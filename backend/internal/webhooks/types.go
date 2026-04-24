package webhooks

import "time"

// Endpoint is an outbound webhook destination.
type Endpoint struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	Events              []string  `json:"events"` // empty/nil = all events
	Enabled             bool      `json:"enabled"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	LastFiredAt         time.Time `json:"last_fired_at,omitempty"`
	LastStatus          string    `json:"last_status,omitempty"`          // "success", "failure", "auto_disabled", or ""
	ConsecutiveFailures int       `json:"consecutive_failures,omitempty"` // reset to 0 on any successful delivery
}

// CreateRequest is the payload for creating an endpoint.
// Secret is server-generated and returned once in CreateResponse.
type CreateRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

// CreateResponse returns the new endpoint plus the one-time secret.
type CreateResponse struct {
	Endpoint Endpoint `json:"endpoint"`
	Secret   string   `json:"secret"` // shown once, receiver uses to verify signatures
}

// UpdateRequest mirrors CreateRequest but without secret rotation (separate action).
type UpdateRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

// Event is the envelope POSTed to receivers.
type Event struct {
	ID        string         `json:"id"`        // unique per delivery attempt chain
	Timestamp time.Time      `json:"timestamp"` // when pCenter generated the event
	Event     string         `json:"event"`     // e.g. "vm.create", "ct.migrate"
	Data      map[string]any `json:"data"`      // event-specific payload
}
