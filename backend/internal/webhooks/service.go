package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/auth"
)

// Service is the public entry point for webhook management and dispatch.
type Service struct {
	db         *DB
	crypto     *auth.Crypto // optional; nil = store secrets plaintext (dev only)
	dispatcher *Dispatcher
}

// NewService wires up the service. crypto may be nil.
func NewService(db *DB, crypto *auth.Crypto) *Service {
	return &Service{
		db:         db,
		crypto:     crypto,
		dispatcher: NewDispatcher(db),
	}
}

// Start launches the background dispatcher.
func (s *Service) Start(ctx context.Context) {
	s.dispatcher.Start(ctx)
}

// Stop halts the dispatcher and closes the DB.
func (s *Service) Stop() error {
	s.dispatcher.Stop()
	return s.db.Close()
}

// Subscribe returns a callback suitable for activity.Service.OnLog — it
// translates activity entries into webhook events and enqueues them.
func (s *Service) Subscribe() func(activity.Entry) {
	return func(e activity.Entry) {
		evt := Event{
			ID:        uuid.NewString(),
			Timestamp: e.Timestamp,
			Event:     eventName(e),
			Data:      activityData(e),
		}
		s.enqueueMatching(evt)
	}
}

// Dispatch sends a synthetic event to all matching endpoints — used by the
// test-ping handler so admins can verify their receiver without waiting for
// real activity.
func (s *Service) Dispatch(event string, data map[string]any) {
	s.enqueueMatching(Event{
		ID:        uuid.NewString(),
		Timestamp: time.Now(),
		Event:     event,
		Data:      data,
	})
}

// DispatchTo sends a synthetic event to a single endpoint (regardless of its
// event filter) — used by the per-endpoint "Test" button.
func (s *Service) DispatchTo(endpointID string, event string, data map[string]any) error {
	row, err := s.db.Get(endpointID)
	if err != nil {
		return err
	}
	secret, err := s.decryptSecret(row.SecretEncrypted)
	if err != nil {
		return err
	}
	s.dispatcher.Enqueue(job{
		event: Event{
			ID:        uuid.NewString(),
			Timestamp: time.Now(),
			Event:     event,
			Data:      data,
		},
		targets: []dispatchTarget{{
			endpointID:  row.ID,
			url:         row.URL,
			secretPlain: secret,
		}},
		queuedAt: time.Now(),
	})
	return nil
}

func (s *Service) enqueueMatching(evt Event) {
	rows, err := s.db.ListEnabled()
	if err != nil {
		slog.Error("webhooks: list enabled endpoints", "err", err)
		return
	}
	var targets []dispatchTarget
	for _, r := range rows {
		if !matches(r.Events, evt.Event) {
			continue
		}
		secret, err := s.decryptSecret(r.SecretEncrypted)
		if err != nil {
			slog.Warn("webhooks: decrypt secret", "endpoint", r.ID, "err", err)
			continue
		}
		targets = append(targets, dispatchTarget{
			endpointID:  r.ID,
			url:         r.URL,
			secretPlain: secret,
		})
	}
	if len(targets) == 0 {
		return
	}
	s.dispatcher.Enqueue(job{event: evt, targets: targets, queuedAt: time.Now()})
}

// --- CRUD ---

// List returns all endpoints (secret never included).
func (s *Service) List() ([]Endpoint, error) {
	rows, err := s.db.List()
	if err != nil {
		return nil, err
	}
	out := make([]Endpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Endpoint)
	}
	return out, nil
}

// Get returns one endpoint.
func (s *Service) Get(id string) (*Endpoint, error) {
	r, err := s.db.Get(id)
	if err != nil {
		return nil, err
	}
	return &r.Endpoint, nil
}

// Create inserts an endpoint with a freshly generated secret.
// The secret is returned in the response (shown once) and never again.
func (s *Service) Create(req CreateRequest) (*CreateResponse, error) {
	if err := validateRequest(req.Name, req.URL); err != nil {
		return nil, err
	}
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encrypted, err := s.encryptSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}

	now := time.Now()
	row := &endpointRow{
		Endpoint: Endpoint{
			ID:        uuid.NewString(),
			Name:      sanitizeName(req.Name),
			URL:       req.URL,
			Events:    dedupe(req.Events),
			Enabled:   req.Enabled,
			CreatedAt: now,
			UpdatedAt: now,
		},
		SecretEncrypted: encrypted,
	}
	if err := s.db.Insert(row); err != nil {
		return nil, err
	}
	return &CreateResponse{Endpoint: row.Endpoint, Secret: secret}, nil
}

// Update modifies an endpoint (secret remains unchanged).
func (s *Service) Update(id string, req UpdateRequest) (*Endpoint, error) {
	if err := validateRequest(req.Name, req.URL); err != nil {
		return nil, err
	}
	req.Name = sanitizeName(req.Name)
	req.Events = dedupe(req.Events)
	if err := s.db.Update(id, req); err != nil {
		return nil, err
	}
	return s.Get(id)
}

// Delete removes an endpoint.
func (s *Service) Delete(id string) error { return s.db.Delete(id) }

// --- helpers ---

func (s *Service) encryptSecret(plain string) (string, error) {
	if s.crypto == nil {
		return plain, nil
	}
	return s.crypto.Encrypt(plain)
}

func (s *Service) decryptSecret(cipher string) (string, error) {
	if s.crypto == nil {
		return cipher, nil
	}
	return s.crypto.Decrypt(cipher)
}

// generateSecret returns a 32-byte URL-safe secret encoded as hex.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func validateRequest(name, rawURL string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("url must include a host")
	}
	return nil
}

func dedupe(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// eventName converts an activity entry into a dotted webhook event name.
// ResourceType is a short lowercase noun (e.g. "vm", "ct", "folder"); Action
// uses snake_case verbs (e.g. "vm_create") — we strip the resource prefix
// where present so we don't emit redundant names like "vm.vm_create".
func eventName(e activity.Entry) string {
	resource := strings.ToLower(strings.TrimSpace(e.ResourceType))
	action := strings.ToLower(strings.TrimSpace(e.Action))
	if resource == "" {
		resource = "activity"
	}
	// strip resource prefix if action starts with it (e.g. "vm_create" → "create")
	prefix := resource + "_"
	if strings.HasPrefix(action, prefix) {
		action = strings.TrimPrefix(action, prefix)
	}
	if action == "" {
		action = "event"
	}
	return resource + "." + action
}

// activityData flattens the activity entry into the Event.Data map so receivers
// can access native fields without unwrapping.
func activityData(e activity.Entry) map[string]any {
	return map[string]any{
		"id":            e.ID,
		"action":        e.Action,
		"resource_type": e.ResourceType,
		"resource_id":   e.ResourceID,
		"resource_name": e.ResourceName,
		"cluster":       e.Cluster,
		"details":       e.Details,
		"status":        e.Status,
		"timestamp":     e.Timestamp,
	}
}
