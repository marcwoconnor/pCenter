package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/library"
	"github.com/moconnor/pcenter/internal/pve"
)

// SetLibraryService sets the content library service
func (h *Handler) SetLibraryService(lib *library.Service) {
	h.library = lib
}

// GetLibraryItems lists content library items with optional filters
func (h *Handler) GetLibraryItems(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	q := r.URL.Query()
	filter := library.ListFilter{
		Type:     library.ItemType(q.Get("type")),
		Category: q.Get("category"),
		Cluster:  q.Get("cluster"),
		Search:   q.Get("search"),
		Tag:      q.Get("tag"),
	}

	items, err := h.library.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, items)
}

// GetLibraryItem returns a single content library item
func (h *Handler) GetLibraryItem(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	id := r.PathValue("id")
	item, err := h.library.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	writeJSON(w, item)
}

// CreateLibraryItem adds a new item to the content library
func (h *Handler) CreateLibraryItem(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	var req library.CreateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	item, err := h.library.Create(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "library_create",
			ResourceType: "library",
			ResourceID:   item.ID,
			ResourceName: item.Name,
			Cluster:      req.Cluster,
		})
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, item)
}

// UpdateLibraryItem modifies an existing content library item
func (h *Handler) UpdateLibraryItem(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	id := r.PathValue("id")

	var req library.UpdateItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if err := h.library.Update(r.Context(), id, req); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if h.activity != nil {
		name := ""
		if req.Name != nil {
			name = *req.Name
		}
		h.activity.Log(activity.Entry{
			Action:       "library_update",
			ResourceType: "library",
			ResourceID:   id,
			ResourceName: name,
		})
	}

	writeJSON(w, map[string]string{"message": "updated"})
}

// DeleteLibraryItem removes a content library item
func (h *Handler) DeleteLibraryItem(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	id := r.PathValue("id")

	// Get item first for activity logging
	item, _ := h.library.Get(r.Context(), id)

	if err := h.library.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if h.activity != nil && item != nil {
		h.activity.Log(activity.Entry{
			Action:       "library_delete",
			ResourceType: "library",
			ResourceID:   id,
			ResourceName: item.Name,
			Cluster:      item.Cluster,
		})
	}

	writeJSON(w, map[string]string{"message": "deleted"})
}

// GetLibraryCategories returns all distinct categories
func (h *Handler) GetLibraryCategories(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	cats, err := h.library.GetCategories(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, cats)
}

// DeployLibraryItem deploys a library item to a target cluster/storage
func (h *Handler) DeployLibraryItem(w http.ResponseWriter, r *http.Request) {
	if h.library == nil {
		writeError(w, http.StatusServiceUnavailable, "content library not enabled")
		return
	}

	id := r.PathValue("id")

	item, err := h.library.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	var req library.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	if req.TargetCluster == "" {
		writeError(w, http.StatusBadRequest, "target_cluster is required")
		return
	}
	if req.TargetNode == "" {
		writeError(w, http.StatusBadRequest, "target_node is required")
		return
	}

	// Deploy based on item type
	switch item.Type {
	case library.ItemTypeVMTemplate:
		upid, err := h.deployVMTemplate(r, item, req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if h.activity != nil {
			h.activity.Log(activity.Entry{
				Action:       "library_deploy",
				ResourceType: "library",
				ResourceID:   id,
				ResourceName: item.Name,
				Cluster:      req.TargetCluster,
				Details:      fmt.Sprintf("deployed to %s/%s", req.TargetCluster, req.TargetNode),
				Status:       "started",
			})
		}
		writeJSON(w, map[string]string{"upid": upid, "message": "clone started"})

	case library.ItemTypeISO, library.ItemTypeVZTemplate, library.ItemTypeSnippet, library.ItemTypeOVA:
		writeError(w, http.StatusNotImplemented,
			"file deployment not yet implemented - use storage upload for ISOs/templates")

	default:
		writeError(w, http.StatusBadRequest, "unsupported item type for deploy: "+string(item.Type))
	}
}

// deployVMTemplate clones a VM template to the target cluster/node
func (h *Handler) deployVMTemplate(r *http.Request, item *library.Item, req library.DeployRequest) (string, error) {
	if item.VMID == 0 {
		return "", fmt.Errorf("item has no source VMID")
	}

	client, ok := h.getClient(req.TargetCluster, req.TargetNode)
	if !ok {
		return "", fmt.Errorf("no client for %s/%s", req.TargetCluster, req.TargetNode)
	}

	storage := req.TargetStorage
	if storage == "" {
		storage = item.Storage
	}

	name := req.NewName
	if name == "" {
		name = item.Name + "-deployed"
	}

	upid, err := client.CloneVM(r.Context(), item.VMID, pve.CloneOptions{
		NewID:      req.NewVMID,
		Name:       name,
		TargetNode: req.TargetNode,
		Full:       req.Full,
		Storage:    storage,
	})
	if err != nil {
		return "", fmt.Errorf("clone VM: %w", err)
	}

	return upid, nil
}
