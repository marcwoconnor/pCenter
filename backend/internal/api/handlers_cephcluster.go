package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/moconnor/pcenter/internal/cephcluster"
	"github.com/moconnor/pcenter/internal/pve"
)

// SetCephClusterManager injects the Ceph install/destroy orchestrator. When
// not called, the install + destroy endpoints return 503.
func (h *Handler) SetCephClusterManager(m *cephcluster.Manager) {
	h.cephClusterMgr = m
}

// cephInstallPreflightRequest is the JSON body for the preflight endpoint.
type cephInstallPreflightRequest struct {
	Nodes []string `json:"nodes"`
}

// PreflightCephInstall checks each target node and returns a structured
// report the install wizard renders before letting the user submit.
func (h *Handler) PreflightCephInstall(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	if h.cephClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Ceph install orchestration not enabled")
		return
	}
	if h.poller == nil {
		writeError(w, http.StatusServiceUnavailable, "no cluster connection available (agent-only mode)")
		return
	}

	var req cephInstallPreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Nodes) == 0 {
		writeError(w, http.StatusBadRequest, "at least one node is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	clients := h.poller.GetClusterClients(clusterName)
	resp := cephcluster.RunInstallPreflight(ctx, func(node string) (*pve.Client, bool) {
		c, ok := clients[node]
		return c, ok
	}, req.Nodes)

	writeJSON(w, resp)
}

// cephInstallRequest is the JSON body for StartCephInstall.
type cephInstallRequest struct {
	Nodes          []string `json:"nodes"`
	Network        string   `json:"network"`
	ClusterNetwork string   `json:"cluster_network,omitempty"`
	PoolSize       int      `json:"pool_size,omitempty"`
	MinSize        int      `json:"min_size,omitempty"`
}

// StartCephInstall kicks off an install Job and returns the job ID. The
// orchestration runs in a background goroutine; clients poll
// GET /api/clusters/{cluster}/ceph/jobs/{job_id} for progress.
func (h *Handler) StartCephInstall(w http.ResponseWriter, r *http.Request) {
	clusterName := r.PathValue("cluster")
	if h.cephClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Ceph install orchestration not enabled")
		return
	}
	var req cephInstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	jobID, err := h.cephClusterMgr.StartInstall(r.Context(), cephcluster.InstallRequest{
		Cluster:        clusterName,
		Nodes:          req.Nodes,
		Network:        req.Network,
		ClusterNetwork: req.ClusterNetwork,
		PoolSize:       req.PoolSize,
		MinSize:        req.MinSize,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"job_id": jobID})
}

// GetCephJob returns a snapshot of a Ceph install/destroy job. 404 when
// unknown — callers should fall back to the topology view (the operation
// either landed or pcenter restarted and lost the job state).
func (h *Handler) GetCephJob(w http.ResponseWriter, r *http.Request) {
	if h.cephClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Ceph install orchestration not enabled")
		return
	}
	jobID := r.PathValue("job_id")
	snap, ok := h.cephClusterMgr.GetJob(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found (pcenter may have restarted; check the topology view)")
		return
	}
	writeJSON(w, snap)
}

// ListCephJobs returns all known Ceph install/destroy jobs (running +
// recently completed), sorted newest-first.
func (h *Handler) ListCephJobs(w http.ResponseWriter, r *http.Request) {
	if h.cephClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Ceph install orchestration not enabled")
		return
	}
	writeJSON(w, h.cephClusterMgr.ListJobs())
}
