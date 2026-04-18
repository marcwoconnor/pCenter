package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/pve"
)

type createBackupRequest struct {
	VMIDs    []int  `json:"vmids"`
	Storage  string `json:"storage"`
	Mode     string `json:"mode,omitempty"`     // snapshot, suspend, stop
	Compress string `json:"compress,omitempty"` // 0, gzip, lzo, zstd
	Notes    string `json:"notes,omitempty"`
}

// CreateBackup triggers a vzdump backup on a node for one or more guests.
func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	node := r.PathValue("node")

	var req createBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.VMIDs) == 0 {
		writeError(w, http.StatusBadRequest, "vmids required")
		return
	}
	if req.Storage == "" {
		writeError(w, http.StatusBadRequest, "storage required")
		return
	}

	client, ok := h.getClient(cluster, node)
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	upid, err := client.CreateVzdump(ctx, pve.VzdumpOptions{
		VMIDs:    req.VMIDs,
		Storage:  req.Storage,
		Mode:     req.Mode,
		Compress: req.Compress,
		Notes:    req.Notes,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.activity != nil {
		for _, vmid := range req.VMIDs {
			id := strconv.Itoa(vmid)
			h.activity.Log(activity.Entry{
				Action:       "backup",
				ResourceType: "guest",
				ResourceID:   id,
				ResourceName: id,
				Cluster:      cluster,
				Details:      "storage=" + req.Storage + " mode=" + req.Mode,
				Status:       "started",
			})
		}
	}

	writeJSON(w, map[string]any{"upid": upid})
}
