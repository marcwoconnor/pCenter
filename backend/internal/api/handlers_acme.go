package api

import (
	"context"
	"encoding/json"
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

// ListACMEDirectories returns published ACME directories (LE, ZeroSSL, etc).
func (h *Handler) ListACMEDirectories(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	dirs, err := client.ListACMEDirectories(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dirs == nil {
		dirs = []pve.ACMEDirectory{}
	}
	writeJSON(w, dirs)
}

// GetACMETOSURL returns the ToS URL for a directory.
func (h *Handler) GetACMETOSURL(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	directoryURL := r.URL.Query().Get("directory")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	tos, err := client.GetACMETOSURL(ctx, directoryURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"tos": tos})
}

// ListACMEChallengeSchemas returns all available plugin schemas.
func (h *Handler) ListACMEChallengeSchemas(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	schemas, err := client.ListACMEChallengeSchemas(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if schemas == nil {
		schemas = []pve.ACMEChallengeSchema{}
	}
	writeJSON(w, schemas)
}

// createACMEAccountRequest is the JSON body for account creation.
type createACMEAccountRequest struct {
	Name      string `json:"name"`
	Contact   string `json:"contact"`
	Directory string `json:"directory,omitempty"`
	TOSURL    string `json:"tos_url,omitempty"`
}

// CreateACMEAccount registers a new ACME account.
func (h *Handler) CreateACMEAccount(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	var req createACMEAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Contact == "" {
		writeError(w, http.StatusBadRequest, "name and contact required")
		return
	}

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	upid, err := client.CreateACMEAccount(ctx, req.Name, req.Contact, req.Directory, req.TOSURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "acme_account_create", ResourceType: "acme_account", ResourceID: req.Name,
			ResourceName: req.Name, Cluster: cluster, Status: "started",
		})
	}
	writeJSON(w, map[string]any{"upid": upid})
}

// updateACMEAccountRequest is the JSON body for account updates.
type updateACMEAccountRequest struct {
	Contact string `json:"contact"`
}

// UpdateACMEAccount updates an account contact.
func (h *Handler) UpdateACMEAccount(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	name := r.PathValue("name")
	var req updateACMEAccountRequest
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

	upid, err := client.UpdateACMEAccount(ctx, name, req.Contact)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"upid": upid})
}

// DeleteACMEAccount removes an ACME account.
func (h *Handler) DeleteACMEAccount(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	name := r.PathValue("name")

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	upid, err := client.DeleteACMEAccount(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "acme_account_delete", ResourceType: "acme_account", ResourceID: name,
			ResourceName: name, Cluster: cluster, Status: "done",
		})
	}
	writeJSON(w, map[string]any{"upid": upid})
}

// createACMEPluginRequest is the JSON body for plugin creation.
type createACMEPluginRequest struct {
	ID   string            `json:"id"`
	Type string            `json:"type"` // "dns" or "standalone"
	API  string            `json:"api,omitempty"`
	Data map[string]string `json:"data,omitempty"`
}

// CreateACMEPlugin creates a new challenge plugin.
func (h *Handler) CreateACMEPlugin(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	var req createACMEPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ID == "" || req.Type == "" {
		writeError(w, http.StatusBadRequest, "id and type required")
		return
	}

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.CreateACMEPlugin(ctx, req.ID, req.Type, req.API, req.Data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "acme_plugin_create", ResourceType: "acme_plugin", ResourceID: req.ID,
			ResourceName: req.ID, Cluster: cluster, Status: "done",
		})
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"status": "created"})
}

// updateACMEPluginRequest for plugin data updates.
type updateACMEPluginRequest struct {
	Data map[string]string `json:"data"`
}

// UpdateACMEPlugin replaces a plugin's data fields.
func (h *Handler) UpdateACMEPlugin(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	id := r.PathValue("id")
	var req updateACMEPluginRequest
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

	if err := client.UpdateACMEPlugin(ctx, id, req.Data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "updated"})
}

// DeleteACMEPlugin removes a plugin.
func (h *Handler) DeleteACMEPlugin(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	id := r.PathValue("id")

	client, ok := h.anyClusterClient(cluster)
	if !ok {
		writeError(w, http.StatusNotFound, "no online node in cluster")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.DeleteACMEPlugin(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "acme_plugin_delete", ResourceType: "acme_plugin", ResourceID: id,
			ResourceName: id, Cluster: cluster, Status: "done",
		})
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// GetNodeACMEDomains returns the node's parsed ACME domain config.
func (h *Handler) GetNodeACMEDomains(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	node := r.PathValue("node")

	client, ok := h.getClient(cluster, node)
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	domains, err := client.GetNodeACMEDomains(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if domains == nil {
		domains = []pve.NodeACMEDomain{}
	}
	writeJSON(w, domains)
}

// setNodeACMEDomainsRequest is the JSON body for domain updates.
type setNodeACMEDomainsRequest struct {
	Domains []pve.NodeACMEDomain `json:"domains"`
}

// SetNodeACMEDomains replaces the node's ACME domain config.
func (h *Handler) SetNodeACMEDomains(w http.ResponseWriter, r *http.Request) {
	cluster := r.PathValue("cluster")
	node := r.PathValue("node")
	var req setNodeACMEDomainsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	client, ok := h.getClient(cluster, node)
	if !ok {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := client.SetNodeACMEDomains(ctx, req.Domains); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.activity != nil {
		h.activity.Log(activity.Entry{
			Action: "acme_domains_set", ResourceType: "node", ResourceID: node,
			ResourceName: node, Cluster: cluster, Status: "done",
		})
	}
	writeJSON(w, map[string]string{"status": "updated"})
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
