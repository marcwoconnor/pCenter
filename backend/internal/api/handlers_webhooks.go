package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/moconnor/pcenter/internal/webhooks"
)

// GetWebhooks lists all outbound webhook endpoints.
func (h *Handler) GetWebhooks(w http.ResponseWriter, r *http.Request) {
	if h.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhooks disabled")
		return
	}
	list, err := h.webhooks.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []webhooks.Endpoint{}
	}
	writeJSON(w, list)
}

// CreateWebhook creates a new endpoint. The response includes the one-time secret.
func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhooks disabled")
		return
	}
	var req webhooks.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resp, err := h.webhooks.Create(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, resp)
}

// UpdateWebhook modifies an endpoint (name/url/events/enabled).
func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhooks disabled")
		return
	}
	id := r.PathValue("id")
	var req webhooks.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ep, err := h.webhooks.Update(id, req)
	if err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, ep)
}

// DeleteWebhook removes an endpoint.
func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhooks disabled")
		return
	}
	id := r.PathValue("id")
	if err := h.webhooks.Delete(id); err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestWebhook fires a synthetic `webhook.test` event at one endpoint.
// The delivery goes through the normal dispatcher (with retries), so admins
// see the same behaviour real events will have.
func (h *Handler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhooks disabled")
		return
	}
	id := r.PathValue("id")
	data := map[string]any{
		"message": "This is a test event from pCenter. If you're reading it, signature verification worked.",
	}
	if err := h.webhooks.DispatchTo(id, "webhook.test", data); err != nil {
		if errors.Is(err, webhooks.ErrNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "queued"})
}
