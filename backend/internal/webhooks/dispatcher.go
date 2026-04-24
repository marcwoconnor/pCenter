package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// retrySchedule defines backoff delays between attempts.
// Attempt 1 fires immediately; on failure wait retrySchedule[0], retry;
// then retrySchedule[1]; etc. Total attempts = 1 + len(retrySchedule) = 4.
// Per issue #27: 3 retries after the initial send.
var retrySchedule = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
}

// httpClient is configured with a conservative timeout. Shared across dispatches.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// job is a single delivery task on the queue: an event + the endpoints to fan out to.
type job struct {
	event     Event
	targets   []dispatchTarget
	queuedAt  time.Time
}

type dispatchTarget struct {
	endpointID   string
	url          string
	secretPlain  string
}

// Dispatcher buffers events from the producer and delivers them to endpoints.
// It owns N workers (currently 1; can scale if throughput demands it).
type Dispatcher struct {
	queue    chan job
	quit     chan struct{}
	db       *DB
	logger   *slog.Logger
}

// NewDispatcher creates a dispatcher. Call Start before Enqueue.
func NewDispatcher(db *DB) *Dispatcher {
	return &Dispatcher{
		queue:  make(chan job, 1024),
		quit:   make(chan struct{}),
		db:     db,
		logger: slog.With("component", "webhooks"),
	}
}

// Start launches the worker goroutine.
func (d *Dispatcher) Start(ctx context.Context) {
	go d.run(ctx)
}

// Stop signals the worker to exit. Pending jobs in the queue are dropped.
func (d *Dispatcher) Stop() { close(d.quit) }

func (d *Dispatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.quit:
			return
		case j := <-d.queue:
			for _, t := range j.targets {
				d.deliver(ctx, j.event, t)
			}
		}
	}
}

// Enqueue hands a job to the queue. Non-blocking; drops if queue is full
// (which indicates downstream is catastrophically backed up — better to shed
// than to back-pressure activity logging, which is on a synchronous path).
func (d *Dispatcher) Enqueue(j job) {
	select {
	case d.queue <- j:
	default:
		d.logger.Warn("queue full; dropping webhook delivery", "event", j.event.Event)
	}
}

// deliver sends one event to one endpoint, with retries.
func (d *Dispatcher) deliver(ctx context.Context, e Event, t dispatchTarget) {
	body, err := json.Marshal(e)
	if err != nil {
		d.logger.Error("marshal event", "err", err, "endpoint", t.endpointID)
		return
	}

	attempts := 1 + len(retrySchedule)
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			// Honour ctx cancellation between attempts.
			select {
			case <-ctx.Done():
				return
			case <-time.After(retrySchedule[attempt-1]):
			}
		}
		err := d.attempt(ctx, t, body, e.Timestamp)
		if err == nil {
			d.db.RecordDelivery(t.endpointID, true, time.Now())
			d.logger.Info("webhook delivered",
				"endpoint", t.endpointID, "event", e.Event, "attempts", attempt+1)
			return
		}
		lastErr = err
		d.logger.Warn("webhook attempt failed",
			"endpoint", t.endpointID, "event", e.Event,
			"attempt", attempt+1, "err", err)
	}

	d.db.RecordDelivery(t.endpointID, false, time.Now())
	d.logger.Error("webhook gave up",
		"endpoint", t.endpointID, "event", e.Event,
		"attempts", attempts, "err", lastErr)
}

// attempt performs a single POST. Returns error if the request failed or
// the remote returned a non-2xx status.
func (d *Dispatcher) attempt(ctx context.Context, t dispatchTarget, body []byte, eventTime time.Time) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pCenter-webhook/1")
	req.Header.Set(SignatureHeader, Sign(t.secretPlain, body, eventTime))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Drain to allow keepalive reuse.
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// matches returns true if the endpoint's event filter includes the given event name.
// An empty/nil filter means "all events".
// matches reports whether the given event should be delivered to an endpoint
// whose subscription filter is `filter`.
//
// Filter semantics:
//   - Empty filter → endpoint receives ALL events (unchanged legacy behaviour).
//   - Exact match → case-insensitive equality (unchanged legacy behaviour).
//   - Component wildcards → `*` as a dotted component means "any value at
//     this position." So `vm.*` matches `vm.create` / `vm.delete` but NOT
//     `ct.create`. `*.migrate` matches `vm.migrate` and `ct.migrate`.
//     Multiple wildcards are allowed (`*.*` matches any two-component event).
//
// Wildcard matching is per-component on purpose — a bare `*` does NOT match
// `vm.create` because component counts differ. An operator who wants "all
// events" should either leave the filter empty or list `*.*`, which is the
// honest description of pCenter's current two-component event shape.
func matches(filter []string, event string) bool {
	if len(filter) == 0 {
		return true
	}
	eventParts := strings.Split(event, ".")
	for _, f := range filter {
		if strings.EqualFold(f, event) {
			return true
		}
		if !strings.Contains(f, "*") {
			continue
		}
		filterParts := strings.Split(f, ".")
		if len(filterParts) != len(eventParts) {
			continue
		}
		allMatch := true
		for i, fp := range filterParts {
			if fp == "*" {
				continue
			}
			if !strings.EqualFold(fp, eventParts[i]) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}
