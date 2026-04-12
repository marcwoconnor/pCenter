package api

import (
	"encoding/json"
	"net/http"

	"github.com/moconnor/pcenter/internal/tags"
)

// SetTagsService sets the tags service on the handler
func (h *Handler) SetTagsService(t *tags.Service) {
	h.tags = t
}

func (h *Handler) GetTags(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeJSON(w, []tags.Tag{})
		return
	}
	list, err := h.tags.ListTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []tags.Tag{}
	}
	writeJSON(w, list)
}

func (h *Handler) GetTagCategories(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeJSON(w, tags.DefaultCategories)
		return
	}
	cats, err := h.tags.GetCategories(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, cats)
}

func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeError(w, http.StatusServiceUnavailable, "tags not enabled")
		return
	}
	var req tags.CreateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tag, err := h.tags.CreateTag(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, tag)
}

func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeError(w, http.StatusServiceUnavailable, "tags not enabled")
		return
	}
	id := r.PathValue("id")
	var req tags.UpdateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.tags.UpdateTag(r.Context(), id, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "updated"})
}

func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeError(w, http.StatusServiceUnavailable, "tags not enabled")
		return
	}
	id := r.PathValue("id")
	if err := h.tags.DeleteTag(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetAllTagAssignments(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeJSON(w, []tags.TagAssignment{})
		return
	}
	assignments, err := h.tags.GetAllAssignments(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if assignments == nil {
		assignments = []tags.TagAssignment{}
	}
	writeJSON(w, assignments)
}

func (h *Handler) AssignTag(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeError(w, http.StatusServiceUnavailable, "tags not enabled")
		return
	}
	var req tags.AssignTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.tags.AssignTag(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "assigned"})
}

func (h *Handler) UnassignTag(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeError(w, http.StatusServiceUnavailable, "tags not enabled")
		return
	}
	var req tags.UnassignTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.tags.UnassignTag(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) BulkAssignTags(w http.ResponseWriter, r *http.Request) {
	if h.tags == nil {
		writeError(w, http.StatusServiceUnavailable, "tags not enabled")
		return
	}
	var req tags.BulkAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.tags.BulkAssign(r.Context(), req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "assigned"})
}
