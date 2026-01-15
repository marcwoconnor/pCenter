package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/moconnor/pcenter/internal/agent"
	"github.com/moconnor/pcenter/internal/poller"
	"github.com/moconnor/pcenter/internal/state"
)

// CORSMiddleware adds CORS headers
func CORSMiddleware(origins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]bool)
	for _, o := range origins {
		originSet[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if originSet[origin] || len(origins) == 0 {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// NewRouter creates the HTTP router with all routes
// Returns both the http.Handler and the Handler for configuration
func NewRouter(store *state.Store, p *poller.Poller, hub *Hub, agentHub *agent.Hub, corsOrigins []string) (http.Handler, *Handler) {
	h := NewHandler(store, p)

	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// WebSocket endpoint (browser clients)
	mux.HandleFunc("GET /ws", hub.HandleWebSocket)

	// Agent WebSocket endpoint
	if agentHub != nil {
		mux.HandleFunc("GET /api/agent/ws", agentHub.HandleWebSocket)
		h.SetAgentHub(agentHub)
		mux.HandleFunc("POST /api/agent/command", h.AgentCommand)
		mux.HandleFunc("GET /api/agent/connected", h.GetConnectedAgents)
	}

	// === Global endpoints (across all clusters) ===

	// Summary & clusters
	mux.HandleFunc("GET /api/summary", h.GetSummary)
	mux.HandleFunc("GET /api/clusters", h.GetClusters)

	// Nodes (all clusters)
	mux.HandleFunc("GET /api/nodes", h.GetNodes)

	// VMs (all clusters)
	mux.HandleFunc("GET /api/vms", h.GetVMs)
	mux.HandleFunc("GET /api/vms/{vmid}", h.GetVM)
	mux.HandleFunc("POST /api/vms/{vmid}/{action}", h.VMAction)

	// Containers (all clusters)
	mux.HandleFunc("GET /api/containers", h.GetContainers)
	mux.HandleFunc("GET /api/containers/{vmid}", h.GetContainer)
	mux.HandleFunc("POST /api/containers/{vmid}/{action}", h.ContainerAction)

	// All guests (VMs + containers combined)
	mux.HandleFunc("GET /api/guests", h.GetAllGuests)

	// Storage & Ceph (all clusters)
	mux.HandleFunc("GET /api/storage", h.GetStorage)
	mux.HandleFunc("GET /api/storage/{storage}/content", h.GetStorageContent)
	mux.HandleFunc("POST /api/storage/{storage}/upload", h.UploadToStorage)
	mux.HandleFunc("GET /api/ceph", h.GetCeph)
	mux.HandleFunc("POST /api/ceph/command", h.RunCephCommand)
	mux.HandleFunc("GET /api/smart", h.GetSmart)

	// Migrations & DRS (global)
	mux.HandleFunc("GET /api/migrations", h.GetMigrations)
	mux.HandleFunc("DELETE /api/migrations/{upid}", h.ClearMigration)
	mux.HandleFunc("GET /api/drs/recommendations", h.GetDRSRecommendations)

	// Console - ticket endpoint and websocket proxy (legacy, searches all clusters)
	mux.HandleFunc("GET /api/console/{type}/{vmid}/ticket", h.ConsoleTicket)
	mux.HandleFunc("GET /api/console/{type}/{vmid}/ws", h.ConsoleWebsocket)

	// === Cluster-specific endpoints ===

	// Cluster summary
	mux.HandleFunc("GET /api/clusters/{cluster}/summary", h.GetClusterSummary)

	// Cluster nodes
	mux.HandleFunc("GET /api/clusters/{cluster}/nodes", h.GetClusterNodes)

	// Cluster guests
	mux.HandleFunc("GET /api/clusters/{cluster}/guests", h.GetClusterGuests)

	// Cluster VMs
	mux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/{action}", h.ClusterVMAction)
	mux.HandleFunc("GET /api/clusters/{cluster}/vms/{vmid}/config", h.GetClusterVMConfig)
	mux.HandleFunc("PUT /api/clusters/{cluster}/vms/{vmid}/config", h.UpdateClusterVMConfig)

	// Cluster containers
	mux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/{action}", h.ClusterContainerAction)
	mux.HandleFunc("GET /api/clusters/{cluster}/containers/{vmid}/config", h.GetClusterContainerConfig)
	mux.HandleFunc("PUT /api/clusters/{cluster}/containers/{vmid}/config", h.UpdateClusterContainerConfig)

	// Create VMs and containers
	mux.HandleFunc("GET /api/clusters/{cluster}/nextid", h.GetNextVMID)
	mux.HandleFunc("POST /api/clusters/{cluster}/nodes/{node}/vms", h.CreateClusterVM)
	mux.HandleFunc("POST /api/clusters/{cluster}/nodes/{node}/containers", h.CreateClusterContainer)

	// Cluster HA status
	mux.HandleFunc("GET /api/clusters/{cluster}/ha/status", h.GetClusterHA)

	// Cluster DRS
	mux.HandleFunc("GET /api/clusters/{cluster}/drs/recommendations", h.GetClusterDRS)
	mux.HandleFunc("POST /api/clusters/{cluster}/drs/apply/{id}", h.ApplyDRSRecommendation)
	mux.HandleFunc("DELETE /api/clusters/{cluster}/drs/recommendations/{id}", h.DismissDRSRecommendation)

	// Cluster HA management
	mux.HandleFunc("GET /api/clusters/{cluster}/ha/groups", h.GetHAGroups)
	mux.HandleFunc("POST /api/clusters/{cluster}/ha/{type}/{vmid}/enable", h.EnableHA)
	mux.HandleFunc("DELETE /api/clusters/{cluster}/ha/{type}/{vmid}", h.DisableHA)

	// Cluster Network/SDN
	mux.HandleFunc("GET /api/clusters/{cluster}/network", h.GetClusterNetwork)
	mux.HandleFunc("GET /api/clusters/{cluster}/network/interfaces", h.GetClusterNetworkInterfaces)
	mux.HandleFunc("GET /api/clusters/{cluster}/sdn/zones", h.GetClusterSDNZones)
	mux.HandleFunc("GET /api/clusters/{cluster}/sdn/vnets", h.GetClusterSDNVNets)
	mux.HandleFunc("GET /api/clusters/{cluster}/sdn/subnets", h.GetClusterSDNSubnets)

	// Cluster Maintenance Mode
	mux.HandleFunc("GET /api/clusters/{cluster}/qdevice", h.GetQDeviceStatus)
	mux.HandleFunc("GET /api/clusters/{cluster}/maintenance/{node}/preflight", h.GetMaintenancePreflight)
	mux.HandleFunc("GET /api/clusters/{cluster}/maintenance/{node}/state", h.GetMaintenanceState)
	mux.HandleFunc("POST /api/clusters/{cluster}/maintenance/{node}/enter", h.EnterMaintenanceMode)
	mux.HandleFunc("POST /api/clusters/{cluster}/maintenance/{node}/exit", h.ExitMaintenanceMode)

	// --- Migration endpoints ---

	// Global (searches all clusters by VMID)
	mux.HandleFunc("POST /api/vms/{vmid}/migrate", h.MigrateVM)
	mux.HandleFunc("POST /api/containers/{vmid}/migrate", h.MigrateContainer)

	// Cluster-specific migrations
	mux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/migrate", h.ClusterMigrateVM)
	mux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/migrate", h.ClusterMigrateContainer)

	// Get nodes for migration target selection
	mux.HandleFunc("GET /api/clusters/{cluster}/nodes/migration-targets", h.GetClusterNodesForMigration)

	// --- Metrics endpoints ---
	mux.HandleFunc("GET /api/metrics", h.GetMetrics)
	mux.HandleFunc("GET /api/metrics/node/{node}", h.GetNodeMetrics)
	mux.HandleFunc("GET /api/metrics/vm/{vmid}", h.GetVMMetrics)
	mux.HandleFunc("GET /api/metrics/ct/{vmid}", h.GetContainerMetrics)
	mux.HandleFunc("GET /api/clusters/{cluster}/metrics", h.GetClusterMetrics)

	// --- Activity endpoints ---
	mux.HandleFunc("GET /api/activity", h.GetActivity)

	// --- Folders endpoints ---
	mux.HandleFunc("GET /api/folders/{tree}", h.GetFolderTree)
	mux.HandleFunc("POST /api/folders", h.CreateFolder)
	mux.HandleFunc("PUT /api/folders/{id}", h.RenameFolder)
	mux.HandleFunc("DELETE /api/folders/{id}", h.DeleteFolder)
	mux.HandleFunc("POST /api/folders/{id}/move", h.MoveFolder)
	mux.HandleFunc("POST /api/folders/{id}/members", h.AddFolderMember)
	mux.HandleFunc("DELETE /api/folders/{id}/members", h.RemoveFolderMember)
	mux.HandleFunc("POST /api/resources/move", h.MoveResource)

	// --- Inventory endpoints (datacenter/cluster management) ---

	// Datacenters
	mux.HandleFunc("GET /api/datacenters", h.ListDatacenters)
	mux.HandleFunc("POST /api/datacenters", h.CreateDatacenter)
	mux.HandleFunc("GET /api/datacenters/{id}", h.GetDatacenter)
	mux.HandleFunc("PUT /api/datacenters/{id}", h.UpdateDatacenter)
	mux.HandleFunc("DELETE /api/datacenters/{id}", h.DeleteDatacenter)
	mux.HandleFunc("GET /api/datacenters/tree", h.GetDatacenterTree)

	// Inventory Clusters (configuration, separate from runtime /api/clusters)
	mux.HandleFunc("GET /api/inventory/clusters", h.ListInventoryClusters)
	mux.HandleFunc("POST /api/inventory/clusters", h.CreateInventoryCluster)
	mux.HandleFunc("GET /api/inventory/clusters/{name}", h.GetInventoryCluster)
	mux.HandleFunc("PUT /api/inventory/clusters/{name}", h.UpdateInventoryCluster)
	mux.HandleFunc("DELETE /api/inventory/clusters/{name}", h.DeleteInventoryCluster)
	mux.HandleFunc("POST /api/inventory/clusters/{name}/move", h.MoveClusterToDatacenter)

	// Inventory Hosts (per-cluster)
	mux.HandleFunc("GET /api/inventory/clusters/{name}/hosts", h.ListClusterHosts)
	mux.HandleFunc("POST /api/inventory/clusters/{name}/hosts", h.AddClusterHost)
	mux.HandleFunc("GET /api/inventory/hosts/{id}", h.GetHost)
	mux.HandleFunc("PUT /api/inventory/hosts/{id}", h.UpdateHost)
	mux.HandleFunc("DELETE /api/inventory/hosts/{id}", h.DeleteHost)

	// Serve static files and SPA fallback
	staticDir := "./frontend"
	if dir := os.Getenv("STATIC_DIR"); dir != "" {
		staticDir = dir
	}
	mux.Handle("/", spaHandler(staticDir))

	// Apply CORS middleware
	return CORSMiddleware(corsOrigins)(mux), h
}

// spaHandler serves static files and falls back to index.html for SPA routing
func spaHandler(staticDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		path := filepath.Clean(r.URL.Path)
		if path == "/" {
			path = "/index.html"
		}

		// Build full path
		fullPath := filepath.Join(staticDir, path)

		// Check if file exists
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			// If not found or is directory, serve index.html (SPA fallback)
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}

		// Set proper content types
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		}

		http.ServeFile(w, r, fullPath)
	})
}

// staticFS is unused but kept for reference
var _ fs.FS
