package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/moconnor/pcenter/internal/agent"
	"github.com/moconnor/pcenter/internal/auth"
	"github.com/moconnor/pcenter/internal/poller"
	"github.com/moconnor/pcenter/internal/state"
)

// CORSMiddleware adds CORS headers.
//
// SECURITY: When origins is empty, NO cross-origin requests are allowed.
// This is intentional — the safe default is to deny all CORS requests
// unless the admin explicitly configures allowed origins in config.yaml
// under server.cors_origins. Previously, an empty list allowed ALL origins,
// which defeated CORS protection entirely on default deployments.
//
// Same-origin requests (no Origin header) are always allowed because
// browsers only send Origin on cross-origin requests.
func CORSMiddleware(origins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]bool)
	for _, o := range origins {
		originSet[o] = true
	}

	if len(origins) == 0 {
		slog.Warn("no CORS origins configured - cross-origin requests will be blocked. " +
			"Set server.cors_origins in config.yaml if you need cross-origin access")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Only set CORS headers if origin is explicitly allowed.
			// Empty origins list = deny all cross-origin (safe default).
			// Same-origin requests have no Origin header and pass through normally.
			if origin != "" && originSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

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
func NewRouter(store *state.Store, p *poller.Poller, hub *Hub, agentHub *agent.Hub, authSvc *auth.Service, corsOrigins []string) (http.Handler, *Handler) {
	h := NewHandler(store, p, corsOrigins)

	mux := http.NewServeMux()

	// Health check (always public)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// Auth endpoints (public)
	if authSvc != nil {
		authHandlers := auth.NewHandlers(authSvc)

		// Public auth endpoints
		mux.HandleFunc("POST /api/auth/login", authHandlers.HandleLogin)
		mux.HandleFunc("POST /api/auth/register", authHandlers.HandleRegister)
		mux.HandleFunc("GET /api/auth/user-count", authHandlers.HandleUserCount)
		mux.HandleFunc("GET /api/auth/check", authHandlers.HandleCheckAuth)

		// Protected auth endpoints (need session but not full auth)
		mux.Handle("POST /api/auth/verify-totp", authSvc.RequireAuth(http.HandlerFunc(authHandlers.HandleVerifyTOTP)))
		mux.Handle("POST /api/auth/logout", authSvc.RequireAuth(http.HandlerFunc(authHandlers.HandleLogout)))
		mux.Handle("GET /api/auth/me", authSvc.RequireAuth(http.HandlerFunc(authHandlers.HandleMe)))
		mux.Handle("PUT /api/auth/password", authSvc.RequireAuth(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleChangePassword))))

		// TOTP management (requires full auth)
		mux.Handle("POST /api/auth/totp/setup", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleTOTPSetup)))))
		mux.Handle("POST /api/auth/totp/verify-setup", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleTOTPVerifySetup)))))
		mux.Handle("DELETE /api/auth/totp", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleTOTPDisable)))))

		// Session management
		mux.Handle("GET /api/auth/sessions", authSvc.RequireAuth(authSvc.RequireFullAuth(http.HandlerFunc(authHandlers.HandleListSessions))))
		mux.Handle("DELETE /api/auth/sessions/{id}", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleRevokeSession)))))

		// Admin-only endpoints
		mux.Handle("GET /api/users", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.RequireAdmin(http.HandlerFunc(authHandlers.HandleListUsers)))))
		mux.Handle("POST /api/users", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.RequireAdmin(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleCreateUser))))))
		mux.Handle("DELETE /api/users/{id}", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.RequireAdmin(authSvc.CSRFProtection(http.HandlerFunc(authHandlers.HandleDeleteUser))))))
		mux.Handle("GET /api/auth/events", authSvc.RequireAuth(authSvc.RequireFullAuth(authSvc.RequireAdmin(http.HandlerFunc(authHandlers.HandleListEvents)))))

		// Wrap WebSocket with auth
		mux.Handle("GET /ws", authSvc.AuthenticatedWebSocket(hub.HandleWebSocket))
	} else {
		// Auth disabled - WebSocket without auth
		mux.HandleFunc("GET /ws", hub.HandleWebSocket)
	}

	// Agent WebSocket endpoint (uses its own auth via API token)
	if agentHub != nil {
		mux.HandleFunc("GET /api/agent/ws", agentHub.HandleWebSocket)
		h.SetAgentHub(agentHub)
		mux.HandleFunc("POST /api/agent/command", h.AgentCommand)
		mux.HandleFunc("GET /api/agent/connected", h.GetConnectedAgents)
	}

	// === Protected API endpoints ===
	// Build a protected mux for all API routes
	protectedMux := http.NewServeMux()

	// === Global endpoints (across all clusters) ===

	// Summary & clusters
	protectedMux.HandleFunc("GET /api/summary", h.GetSummary)
	protectedMux.HandleFunc("GET /api/clusters", h.GetClusters)

	// Nodes (all clusters)
	protectedMux.HandleFunc("GET /api/nodes", h.GetNodes)

	// VMs (all clusters)
	protectedMux.HandleFunc("GET /api/vms", h.GetVMs)
	protectedMux.HandleFunc("GET /api/vms/{vmid}", h.GetVM)
	protectedMux.HandleFunc("POST /api/vms/{vmid}/{action}", h.VMAction)

	// Containers (all clusters)
	protectedMux.HandleFunc("GET /api/containers", h.GetContainers)
	protectedMux.HandleFunc("GET /api/containers/{vmid}", h.GetContainer)
	protectedMux.HandleFunc("POST /api/containers/{vmid}/{action}", h.ContainerAction)

	// All guests (VMs + containers combined)
	protectedMux.HandleFunc("GET /api/guests", h.GetAllGuests)

	// Storage & Ceph (all clusters)
	protectedMux.HandleFunc("GET /api/storage", h.GetStorage)
	protectedMux.HandleFunc("GET /api/storage/{storage}/content", h.GetStorageContent)
	protectedMux.HandleFunc("POST /api/storage/{storage}/upload", h.UploadToStorage)
	protectedMux.HandleFunc("GET /api/ceph", h.GetCeph)
	protectedMux.HandleFunc("POST /api/ceph/command", h.RunCephCommand)
	protectedMux.HandleFunc("GET /api/smart", h.GetSmart)

	// Migrations & DRS (global)
	protectedMux.HandleFunc("GET /api/migrations", h.GetMigrations)
	protectedMux.HandleFunc("DELETE /api/migrations/{upid}", h.ClearMigration)
	protectedMux.HandleFunc("GET /api/drs/recommendations", h.GetDRSRecommendations)

	// Console - ticket endpoint and websocket proxy (legacy, searches all clusters)
	protectedMux.HandleFunc("GET /api/console/{type}/{vmid}/ticket", h.ConsoleTicket)
	protectedMux.HandleFunc("GET /api/console/{type}/{vmid}/ws", h.ConsoleWebsocket)

	// === Cluster-specific endpoints ===

	// Cluster summary
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/summary", h.GetClusterSummary)

	// Cluster nodes
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/nodes", h.GetClusterNodes)

	// Cluster guests
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/guests", h.GetClusterGuests)

	// Cluster VMs
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/{action}", h.ClusterVMAction)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/vms/{vmid}/config", h.GetClusterVMConfig)
	protectedMux.HandleFunc("PUT /api/clusters/{cluster}/vms/{vmid}/config", h.UpdateClusterVMConfig)

	// Cluster containers
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/{action}", h.ClusterContainerAction)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/containers/{vmid}/config", h.GetClusterContainerConfig)
	protectedMux.HandleFunc("PUT /api/clusters/{cluster}/containers/{vmid}/config", h.UpdateClusterContainerConfig)

	// VM Snapshots
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/vms/{vmid}/snapshots", h.GetVMSnapshots)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/snapshots", h.CreateVMSnapshot)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/snapshots/{snapname}/rollback", h.RollbackVMSnapshot)
	protectedMux.HandleFunc("DELETE /api/clusters/{cluster}/vms/{vmid}/snapshots/{snapname}", h.DeleteVMSnapshot)

	// Container Snapshots
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/containers/{vmid}/snapshots", h.GetContainerSnapshots)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/snapshots", h.CreateContainerSnapshot)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/snapshots/{snapname}/rollback", h.RollbackContainerSnapshot)
	protectedMux.HandleFunc("DELETE /api/clusters/{cluster}/containers/{vmid}/snapshots/{snapname}", h.DeleteContainerSnapshot)

	// Create VMs and containers
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/nextid", h.GetNextVMID)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/nodes/{node}/vms", h.CreateClusterVM)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/nodes/{node}/containers", h.CreateClusterContainer)

	// Delete VMs and containers
	protectedMux.HandleFunc("DELETE /api/clusters/{cluster}/vms/{vmid}", h.DeleteClusterVM)
	protectedMux.HandleFunc("DELETE /api/clusters/{cluster}/containers/{vmid}", h.DeleteClusterContainer)

	// Cluster HA status
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/ha/status", h.GetClusterHA)

	// Cluster DRS
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/drs/recommendations", h.GetClusterDRS)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/drs/apply/{id}", h.ApplyDRSRecommendation)
	protectedMux.HandleFunc("DELETE /api/clusters/{cluster}/drs/recommendations/{id}", h.DismissDRSRecommendation)

	// Cluster HA management
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/ha/groups", h.GetHAGroups)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/ha/{type}/{vmid}/enable", h.EnableHA)
	protectedMux.HandleFunc("DELETE /api/clusters/{cluster}/ha/{type}/{vmid}", h.DisableHA)

	// Cluster Network/SDN
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/network", h.GetClusterNetwork)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/network/interfaces", h.GetClusterNetworkInterfaces)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/sdn/zones", h.GetClusterSDNZones)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/sdn/vnets", h.GetClusterSDNVNets)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/sdn/subnets", h.GetClusterSDNSubnets)

	// Cluster Maintenance Mode
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/qdevice", h.GetQDeviceStatus)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/maintenance/{node}/preflight", h.GetMaintenancePreflight)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/maintenance/{node}/state", h.GetMaintenanceState)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/maintenance/{node}/enter", h.EnterMaintenanceMode)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/maintenance/{node}/exit", h.ExitMaintenanceMode)

	// --- Migration endpoints ---

	// Global (searches all clusters by VMID)
	protectedMux.HandleFunc("POST /api/vms/{vmid}/migrate", h.MigrateVM)
	protectedMux.HandleFunc("POST /api/containers/{vmid}/migrate", h.MigrateContainer)

	// Cluster-specific migrations
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/migrate", h.ClusterMigrateVM)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/migrate", h.ClusterMigrateContainer)

	// Clone operations
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/vms/{vmid}/clone", h.CloneVM)
	protectedMux.HandleFunc("POST /api/clusters/{cluster}/containers/{vmid}/clone", h.CloneContainer)

	// Get nodes for migration target selection
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/nodes/migration-targets", h.GetClusterNodesForMigration)

	// --- Metrics endpoints ---
	protectedMux.HandleFunc("GET /api/metrics", h.GetMetrics)
	protectedMux.HandleFunc("GET /api/metrics/node/{node}", h.GetNodeMetrics)
	protectedMux.HandleFunc("GET /api/metrics/vm/{vmid}", h.GetVMMetrics)
	protectedMux.HandleFunc("GET /api/metrics/ct/{vmid}", h.GetContainerMetrics)
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/metrics", h.GetClusterMetrics)

	// --- Task status endpoint ---
	protectedMux.HandleFunc("GET /api/clusters/{cluster}/tasks/{upid}", h.GetTaskStatus)

	// --- Activity endpoints ---
	protectedMux.HandleFunc("GET /api/activity", h.GetActivity)

	// --- Folders endpoints ---
	protectedMux.HandleFunc("GET /api/folders/{tree}", h.GetFolderTree)
	protectedMux.HandleFunc("POST /api/folders", h.CreateFolder)
	protectedMux.HandleFunc("PUT /api/folders/{id}", h.RenameFolder)
	protectedMux.HandleFunc("DELETE /api/folders/{id}", h.DeleteFolder)
	protectedMux.HandleFunc("POST /api/folders/{id}/move", h.MoveFolder)
	protectedMux.HandleFunc("POST /api/folders/{id}/members", h.AddFolderMember)
	protectedMux.HandleFunc("DELETE /api/folders/{id}/members", h.RemoveFolderMember)
	protectedMux.HandleFunc("POST /api/resources/move", h.MoveResource)

	// --- Content Library endpoints ---
	protectedMux.HandleFunc("GET /api/library", h.GetLibraryItems)
	protectedMux.HandleFunc("GET /api/library/categories", h.GetLibraryCategories)
	protectedMux.HandleFunc("GET /api/library/{id}", h.GetLibraryItem)
	protectedMux.HandleFunc("POST /api/library", h.CreateLibraryItem)
	protectedMux.HandleFunc("PUT /api/library/{id}", h.UpdateLibraryItem)
	protectedMux.HandleFunc("DELETE /api/library/{id}", h.DeleteLibraryItem)
	protectedMux.HandleFunc("POST /api/library/{id}/deploy", h.DeployLibraryItem)

	// --- Inventory endpoints (datacenter/cluster management) ---

	// Datacenters
	protectedMux.HandleFunc("GET /api/datacenters", h.ListDatacenters)
	protectedMux.HandleFunc("POST /api/datacenters", h.CreateDatacenter)
	protectedMux.HandleFunc("GET /api/datacenters/{id}", h.GetDatacenter)
	protectedMux.HandleFunc("PUT /api/datacenters/{id}", h.UpdateDatacenter)
	protectedMux.HandleFunc("DELETE /api/datacenters/{id}", h.DeleteDatacenter)
	protectedMux.HandleFunc("GET /api/datacenters/tree", h.GetDatacenterTree)
	protectedMux.HandleFunc("POST /api/datacenters/{id}/hosts", h.AddDatacenterHost)

	// Inventory Clusters (configuration, separate from runtime /api/clusters)
	protectedMux.HandleFunc("GET /api/inventory/clusters", h.ListInventoryClusters)
	protectedMux.HandleFunc("POST /api/inventory/clusters", h.CreateInventoryCluster)
	protectedMux.HandleFunc("GET /api/inventory/clusters/{name}", h.GetInventoryCluster)
	protectedMux.HandleFunc("PUT /api/inventory/clusters/{name}", h.UpdateInventoryCluster)
	protectedMux.HandleFunc("DELETE /api/inventory/clusters/{name}", h.DeleteInventoryCluster)
	protectedMux.HandleFunc("POST /api/inventory/clusters/{name}/move", h.MoveClusterToDatacenter)

	// Inventory Hosts (per-cluster)
	protectedMux.HandleFunc("GET /api/inventory/clusters/{name}/hosts", h.ListClusterHosts)
	protectedMux.HandleFunc("POST /api/inventory/clusters/{name}/hosts", h.AddClusterHost)
	protectedMux.HandleFunc("GET /api/inventory/hosts/{id}", h.GetHost)
	protectedMux.HandleFunc("PUT /api/inventory/hosts/{id}", h.UpdateHost)
	protectedMux.HandleFunc("DELETE /api/inventory/hosts/{id}", h.DeleteHost)
	protectedMux.HandleFunc("POST /api/inventory/hosts/{id}/activate", h.ActivateHost)
	protectedMux.HandleFunc("POST /api/inventory/hosts/{id}/setup-ssh", h.SetupHostSSH)
	protectedMux.HandleFunc("POST /api/inventory/hosts/{id}/deploy-agent", h.DeployAgent)

	// Host connection testing
	protectedMux.HandleFunc("POST /api/inventory/test-connection", h.TestHostConnection)

	// Wrap protected routes with auth middleware (if auth is enabled)
	if authSvc != nil {
		mux.Handle("/api/", authSvc.RequireAuth(authSvc.RequireFullAuth(protectedMux)))
	} else {
		// No auth - register routes directly
		mux.Handle("/api/", protectedMux)
	}

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
