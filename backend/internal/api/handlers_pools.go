package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/pve"
)

// ListPools returns resource pools for a cluster.
func (h *Handler) ListPools(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	pools, err := client.ListPools(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pools == nil {
		pools = []pve.Pool{}
	}
	writeJSON(w, pools)
}

// GetPool returns pool details including members.
func (h *Handler) GetPool(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	poolID := r.PathValue("poolid")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	detail, err := client.GetPool(ctx, poolID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail.Members == nil {
		detail.Members = []pve.PoolMember{}
	}
	writeJSON(w, detail)
}

type createPoolRequest struct {
	PoolID  string `json:"poolid"`
	Comment string `json:"comment,omitempty"`
}

// CreatePool creates a new resource pool.
func (h *Handler) CreatePool(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	var req createPoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PoolID == "" {
		writeError(w, http.StatusBadRequest, "poolid required")
		return
	}

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.CreatePool(ctx, req.PoolID, req.Comment); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "pool_create", ResourceType: "pool", ResourceID: req.PoolID,
			ResourceName: req.PoolID, Cluster: cluster, Status: "done",
		})
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"status": "created"})
}

type updatePoolRequest struct {
	Comment string   `json:"comment,omitempty"`
	VMs     []string `json:"vms,omitempty"`     // VMIDs to add (or remove if delete=true)
	Storage []string `json:"storage,omitempty"` // Storage IDs to add (or remove)
	Delete  bool     `json:"delete,omitempty"`  // If true, the listed members are REMOVED instead of added
}

// UpdatePool modifies a pool's comment or members.
func (h *Handler) UpdatePool(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	poolID := r.PathValue("poolid")
	var req updatePoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.UpdatePool(ctx, poolID, req.Comment, req.VMs, req.Storage, req.Delete); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "updated"})
}

// DeletePool removes an empty pool.
func (h *Handler) DeletePool(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	poolID := r.PathValue("poolid")

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.DeletePool(ctx, poolID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "pool_delete", ResourceType: "pool", ResourceID: poolID,
			ResourceName: poolID, Cluster: cluster, Status: "done",
		})
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}
