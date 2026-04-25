package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/pvecluster"
)

// SetPveClusterManager injects the cluster-formation orchestrator.
func (h *Handler) SetPveClusterManager(m *pvecluster.Manager) {
	h.pveClusterMgr = m
}

// --- Preflight ---

// pveClusterPreflightRequest is the JSON body shape.
type pveClusterPreflightRequest struct {
	ClusterName   string   `json:"cluster_name"`
	FounderHostID string   `json:"founder_host_id"`
	JoinerHostIDs []string `json:"joiner_host_ids"`
}

// PveClusterPreflight runs read-only probes against the target hosts to
// surface any blockers before the user submits the actual create request.
func (h *Handler) PveClusterPreflight(w http.ResponseWriter, r *http.Request) {
	if h.pveClusterMgr == nil || h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster formation not enabled")
		return
	}
	var req pveClusterPreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	resp, err := pvecluster.Preflight(ctx, h.inventory, pvecluster.PreflightRequest{
		ClusterName:   req.ClusterName,
		FounderHostID: req.FounderHostID,
		JoinerHostIDs: req.JoinerHostIDs,
	}, func(host *inventory.InventoryHost) *pve.Client {
		if host.TokenID == "" || host.TokenSecret == "" {
			return nil
		}
		c := pve.NewClientFromClusterConfig(config.ClusterConfig{
			Name:          "preflight:" + host.ID,
			DiscoveryNode: host.Address,
			TokenID:       host.TokenID,
			TokenSecret:   host.TokenSecret,
			Insecure:      host.Insecure,
		})
		if host.NodeName != "" {
			c.SetNodeName(host.NodeName)
		}
		return c
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, resp)
}

// --- Create ---

// pveClusterCreateRequest is the JSON body shape for kicking off a job.
type pveClusterCreateRequest struct {
	ClusterName     string `json:"cluster_name"`
	DatacenterID    string `json:"datacenter_id"`
	FounderHostID   string `json:"founder_host_id"`
	FounderPassword string `json:"founder_password"`
	FounderLink0    string `json:"founder_link0,omitempty"`
	Joiners         []struct {
		HostID   string `json:"host_id"`
		Password string `json:"password"`
		Link0    string `json:"link0,omitempty"`
	} `json:"joiners"`
}

// CreatePveCluster starts the orchestration job and returns the job ID.
func (h *Handler) CreatePveCluster(w http.ResponseWriter, r *http.Request) {
	if h.pveClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster formation not enabled")
		return
	}
	var req pveClusterCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	joiners := make([]pvecluster.JoinerSpec, 0, len(req.Joiners))
	for _, j := range req.Joiners {
		joiners = append(joiners, pvecluster.JoinerSpec{
			HostID:   j.HostID,
			Password: j.Password,
			Link0:    j.Link0,
		})
	}

	// StartJob validates state; give it a short ctx for the validation reads.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	jobID, err := h.pveClusterMgr.StartJob(ctx, pvecluster.StartJobRequest{
		ClusterName:     req.ClusterName,
		DatacenterID:    req.DatacenterID,
		FounderHostID:   req.FounderHostID,
		FounderPassword: req.FounderPassword,
		FounderLink0:    req.FounderLink0,
		Joiners:         joiners,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"job_id": jobID})
}

// --- Poll ---

// --- Join existing cluster ---

// pveClusterJoinPreflightRequest is the body shape for join-mode preflight.
type pveClusterJoinPreflightRequest struct {
	ClusterID     string   `json:"cluster_id"`
	JoinerHostIDs []string `json:"joiner_host_ids"`
}

// PveClusterJoinPreflight runs read-only checks against the joiners and
// validates their PVE major version matches the existing cluster's members.
func (h *Handler) PveClusterJoinPreflight(w http.ResponseWriter, r *http.Request) {
	if h.pveClusterMgr == nil || h.inventory == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster formation not enabled")
		return
	}
	var req pveClusterJoinPreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	resp, err := pvecluster.JoinHostsPreflight(ctx, h.inventory, pvecluster.JoinHostsPreflightRequest{
		ClusterID:     req.ClusterID,
		JoinerHostIDs: req.JoinerHostIDs,
	}, func(host *inventory.InventoryHost) *pve.Client {
		if host.TokenID == "" || host.TokenSecret == "" {
			return nil
		}
		c := pve.NewClientFromClusterConfig(config.ClusterConfig{
			Name:          "preflight-join:" + host.ID,
			DiscoveryNode: host.Address,
			TokenID:       host.TokenID,
			TokenSecret:   host.TokenSecret,
			Insecure:      host.Insecure,
		})
		if host.NodeName != "" {
			c.SetNodeName(host.NodeName)
		}
		return c
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, resp)
}

// pveClusterJoinRequest is the body for kicking off a join-existing job.
type pveClusterJoinRequest struct {
	ClusterID string `json:"cluster_id"`
	Joiners   []struct {
		HostID   string `json:"host_id"`
		Password string `json:"password"`
		Link0    string `json:"link0,omitempty"`
	} `json:"joiners"`
}

// JoinPveCluster starts a job that runs `pvecm add` for each joiner against
// an already-existing PVE cluster pcenter is managing.
func (h *Handler) JoinPveCluster(w http.ResponseWriter, r *http.Request) {
	if h.pveClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster formation not enabled")
		return
	}
	var req pveClusterJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	joiners := make([]pvecluster.JoinerSpec, 0, len(req.Joiners))
	for _, j := range req.Joiners {
		joiners = append(joiners, pvecluster.JoinerSpec{
			HostID:   j.HostID,
			Password: j.Password,
			Link0:    j.Link0,
		})
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	jobID, err := h.pveClusterMgr.StartJoinHostsJob(ctx, pvecluster.StartJoinHostsJobRequest{
		ClusterID: req.ClusterID,
		Joiners:   joiners,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"job_id": jobID})
}

// GetPveClusterJob returns the current snapshot of a job, or 404 if unknown
// (pcenter may have been restarted — frontend handles the 404 by suggesting a
// manual verification + rescan).
func (h *Handler) GetPveClusterJob(w http.ResponseWriter, r *http.Request) {
	if h.pveClusterMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster formation not enabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "job id is required")
		return
	}
	snap, ok := h.pveClusterMgr.GetJob(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found — pcenter may have restarted")
		return
	}
	writeJSON(w, snap)
}
