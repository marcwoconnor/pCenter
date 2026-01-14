package activity

import (
	"log/slog"
	"time"
)

// Service manages activity logging with optional broadcast
type Service struct {
	db        *DB
	onLog     func(Entry)
	retention int // days
}

// NewService creates a new activity service
func NewService(db *DB, retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	return &Service{
		db:        db,
		retention: retentionDays,
	}
}

// OnLog sets a callback to be called when new entries are logged
func (s *Service) OnLog(fn func(Entry)) {
	s.onLog = fn
}

// Log records an activity entry and broadcasts it
func (s *Service) Log(e Entry) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if e.Status == "" {
		e.Status = StatusSuccess
	}

	id, err := s.db.Insert(e)
	if err != nil {
		slog.Error("failed to log activity", "error", err, "action", e.Action)
		return
	}

	e.ID = id
	slog.Info("activity logged",
		"id", id,
		"action", e.Action,
		"resource", e.ResourceType+"/"+e.ResourceID,
		"cluster", e.Cluster,
	)

	if s.onLog != nil {
		s.onLog(e)
	}
}

// Query retrieves activity entries
func (s *Service) Query(params QueryParams) ([]Entry, error) {
	return s.db.Query(params)
}

// StartCleanup starts periodic cleanup of old entries
func (s *Service) StartCleanup() {
	go func() {
		// Run cleanup once at startup
		s.db.Cleanup(s.retention)

		// Then daily
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			s.db.Cleanup(s.retention)
		}
	}()
}

// Close closes the underlying database
func (s *Service) Close() error {
	return s.db.Close()
}
