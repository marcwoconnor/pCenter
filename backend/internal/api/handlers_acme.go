package api

import (
	"context"
	"net/http"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/pve"
)

// GetNodeCertificates returns certificate info for a node.
func (h *Handler) GetNodeCertificates(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	node := r.PathValue("node")

	client, ok := h.getClient(cluster, node)
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	certs, err := client.GetNodeCertificates(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if certs == nil {
		certs = []pve.NodeCertificate{}
	}
	writeJSON(w, certs)
}

// anyClusterClient picks any online node's client for cluster-scoped endpoints.
func (h *Handler) anyClusterClient(cluster string) (*pve.Client, bool) {
	cs, ok := h.store.GetCluster(cluster)
	if !ok {
		return nil, false
	}
	for _, n := range cs.GetNodes() {
		if n.Status != "online" {
			continue
		}
		if c, ok := h.getClient(cluster, n.Node); ok {
			return c, true
		}
	}
	return nil, false
}

// ListACMEAccounts returns ACME accounts for a cluster.
func (h *Handler) ListACMEAccounts(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	accounts, err := client.ListACMEAccounts(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if accounts == nil {
		accounts = []pve.ACMEAccount{}
	}
	writeJSON(w, accounts)
}

// ListACMEPlugins returns ACME DNS/standalone plugins for a cluster.
func (h *Handler) ListACMEPlugins(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	plugins, err := client.ListACMEPlugins(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if plugins == nil {
		plugins = []pve.ACMEPlugin{}
	}
	writeJSON(w, plugins)
}

// RenewNodeACMECertificate triggers ACME cert renewal on a node.
func (h *Handler) RenewNodeACMECertificate(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	node := r.PathValue("node")

	client, ok := h.getClient(cluster, node)
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	upid, err := client.RenewNodeACMECertificate(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "renew failed: "+err.Error())
		return
	}

	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action:       "acme_renew",
			ResourceType: "node",
			ResourceID:   node,
			ResourceName: node,
			Cluster:      cluster,
			Status:       "started",
		})
	}

	writeJSON(w, map[string]any{"upid": upid})
}
