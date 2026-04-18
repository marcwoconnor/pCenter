package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/alarms"
	"github.com/moconnor/pcenter/internal/drs"
	"github.com/moconnor/pcenter/internal/agent"
	"github.com/moconnor/pcenter/internal/api"
	"github.com/moconnor/pcenter/internal/auth"
	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/folders"
	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/library"
	"github.com/moconnor/pcenter/internal/metrics"
	"github.com/moconnor/pcenter/internal/migration"
	"github.com/moconnor/pcenter/internal/poller"
	"github.com/moconnor/pcenter/internal/pve"
	"github.com/moconnor/pcenter/internal/rbac"
	"github.com/moconnor/pcenter/internal/scheduler"
	"github.com/moconnor/pcenter/internal/state"
	"github.com/moconnor/pcenter/internal/tags"
	"github.com/moconnor/pcenter/internal/updater"
	"github.com/moconnor/pcenter/internal/webhooks"
)

func main() {
	// Flags
	configPath := flag.String("config", "config.yaml", "path to config file")
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	// Logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("loaded config", "clusters", len(cfg.Clusters), "port", cfg.Server.Port, "drs", cfg.DRS.Enabled, "poller", cfg.Poller.Enabled)

	// Create state store
	store := state.New()

	// Create WebSocket hub (browser clients) with origin checking
	hub := api.NewHub(store, cfg.Server.CORSOrigins)
	go hub.Run()

	// Create agent hub (pve-agents) with pre-shared auth token
	agentHub := agent.NewHub(store, cfg.Agent.AuthToken)

	// Broadcast state changes via WebSocket
	broadcastFn := func() {
		hub.BroadcastState()
	}
	agentHub.OnChange(broadcastFn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start agent hub cleanup loop (for stale commands)
	go agentHub.StartCleanupLoop(ctx)

	// Create poller (started later after inventory is loaded)
	var p *poller.Poller
	if cfg.Poller.Enabled {
		p = poller.New(store, 5*time.Second, cfg.DRS)
		p.OnChange(broadcastFn)
	} else {
		slog.Info("poller disabled - running in agent-only mode")
	}

	// Initialize auth service
	var authSvc *auth.Service
	var authCrypto *auth.Crypto // shared with webhooks for secret encryption
	if cfg.Auth.Enabled {
		authDB, err := auth.Open(cfg.Auth.DatabasePath)
		if err != nil {
			slog.Error("failed to open auth database", "error", err)
			os.Exit(1)
		}
		defer authDB.Close()

		// Initialize crypto for TOTP secret encryption
		// Auto-generate key if not configured
		if cfg.Auth.EncryptionKey == "" {
			key, genErr := auth.GenerateKey()
			if genErr != nil {
				slog.Error("failed to generate encryption key", "error", genErr)
				os.Exit(1)
			}
			if err := persistEncryptionKey(key); err != nil {
				slog.Warn("could not persist encryption key to env file — set PCENTER_ENCRYPTION_KEY manually", "error", err)
			} else {
				slog.Info("auto-generated TOTP encryption key and saved to env file")
			}
			cfg.Auth.EncryptionKey = key
		}

		crypto, err := auth.NewCrypto(cfg.Auth.EncryptionKey)
		if err != nil {
			slog.Error("failed to initialize auth crypto", "error", err)
			os.Exit(1)
		}
		authCrypto = crypto

		// Build auth config from app config
		authCfg := auth.Config{
			Enabled:       cfg.Auth.Enabled,
			DatabasePath:  cfg.Auth.DatabasePath,
			EncryptionKey: cfg.Auth.EncryptionKey,
			Session: auth.SessionConfig{
				DurationHours:    cfg.Auth.Session.DurationHours,
				IdleTimeoutHours: cfg.Auth.Session.IdleTimeoutHours,
				CookieSecure:     cfg.Auth.Session.CookieSecure,
				CookieDomain:     cfg.Auth.Session.CookieDomain,
			},
			Lockout: auth.LockoutConfig{
				MaxAttempts:    cfg.Auth.Lockout.MaxAttempts,
				LockoutMinutes: cfg.Auth.Lockout.LockoutMinutes,
				Progressive:    cfg.Auth.Lockout.Progressive,
			},
			TOTP: auth.TOTPConfig{
				Enabled:       cfg.Auth.TOTP.Enabled,
				Required:      cfg.Auth.TOTP.Required,
				Issuer:        cfg.Auth.TOTP.Issuer,
				RecoveryCodes: cfg.Auth.TOTP.RecoveryCodes,
				TrustIPHours:  cfg.Auth.TOTP.TrustIPHours,
			},
			RateLimit: auth.RateLimitConfig{
				RequestsPerMinute: cfg.Auth.RateLimit.RequestsPerMinute,
			},
		}

		authSvc = auth.NewService(authDB, crypto, authCfg)

		// Migrate any existing plaintext TOTP secrets to encrypted
		migrated, migErr := authDB.MigrateTOTPSecrets(crypto)
		if migErr != nil {
			slog.Error("failed to migrate TOTP secrets", "error", migErr)
			os.Exit(1)
		}
		if migrated > 0 {
			slog.Info("encrypted existing plaintext TOTP secrets", "count", migrated)
		}

		// Check if we need first-user setup
		userCount, err := authSvc.UserCount(ctx)
		if err != nil {
			slog.Error("failed to check user count", "error", err)
			os.Exit(1)
		}
		if userCount == 0 {
			slog.Info("auth enabled - no users found, first registration will create admin")
		} else {
			slog.Info("auth enabled", "users", userCount, "database", cfg.Auth.DatabasePath)
		}

		// Start session cleanup goroutine
		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					authSvc.CleanupExpiredSessions(ctx)
				}
			}
		}()
	} else {
		slog.Info("auth disabled - all endpoints are public")
	}

	// Create HTTP server
	router, handler := api.NewRouter(store, p, hub, agentHub, authSvc, cfg.Server.CORSOrigins)

	// Set clusters config for on-demand client creation (agent-only mode)
	handler.SetClusters(cfg.Clusters)

	// Set broadcast callback for handler (migrations, etc.)
	handler.SetOnChange(broadcastFn)

	// Initialize metrics if enabled
	var metricsDB *metrics.DB
	if cfg.Metrics.Enabled {
		var err error
		metricsDB, err = metrics.Open(cfg.Metrics.DatabasePath)
		if err != nil {
			slog.Error("failed to open metrics database", "error", err)
			os.Exit(1)
		}
		defer metricsDB.Close()

		// Set up metrics services
		queryService := metrics.NewQueryService(metricsDB)
		handler.SetMetricsService(queryService)

		// Start metrics collector
		collector := metrics.NewCollector(store, metricsDB, cfg.Metrics.CollectionInterval)
		go collector.Start(ctx)

		// Start rollup service
		retention := metrics.RetentionConfig{
			RawHours:     cfg.Metrics.Retention.RawHours,
			HourlyDays:   cfg.Metrics.Retention.HourlyDays,
			DailyDays:    cfg.Metrics.Retention.DailyDays,
			WeeklyMonths: cfg.Metrics.Retention.WeeklyMonths,
		}
		rollupService := metrics.NewRollupService(metricsDB, retention)
		go rollupService.Start(ctx)

		slog.Info("metrics enabled", "database", cfg.Metrics.DatabasePath, "interval", cfg.Metrics.CollectionInterval)
	}

	// Initialize folders database (always enabled)
	foldersDB, err := folders.Open(cfg.Folders.DatabasePath)
	if err != nil {
		slog.Error("failed to open folders database", "error", err)
		os.Exit(1)
	}
	defer foldersDB.Close()

	foldersService := folders.NewService(foldersDB)
	handler.SetFoldersService(foldersService)
	slog.Info("folders enabled", "database", cfg.Folders.DatabasePath)

	// Initialize inventory database (datacenter/cluster management)
	inventoryDB, err := inventory.Open(cfg.Inventory.DatabasePath)
	if err != nil {
		slog.Error("failed to open inventory database", "error", err)
		os.Exit(1)
	}
	defer inventoryDB.Close()

	inventoryService := inventory.NewService(inventoryDB)
	handler.SetInventoryService(inventoryService)
	handler.SetSecrets(cfg.ClusterSecrets)
	handler.SetConfig(cfg)

	// Migrate clusters from config.yaml to inventory DB (one-time)
	if err := migrateConfigClusters(ctx, inventoryService, cfg); err != nil {
		slog.Error("failed to migrate clusters", "error", err)
		// Non-fatal - continue with existing inventory
	}

	slog.Info("inventory enabled", "database", cfg.Inventory.DatabasePath)

	// Initialize RBAC
	rbacDBPath := "data/rbac.db"
	rbacDB, err := rbac.OpenDB(rbacDBPath)
	if err != nil {
		slog.Error("failed to open RBAC database", "error", err)
		os.Exit(1)
	}
	defer rbacDB.Close()

	rbacResolver := rbac.NewStateResolver(store, inventoryService)
	rbacService := rbac.NewService(rbacDB, rbacResolver)
	handler.SetRBACService(rbacService)
	slog.Info("RBAC enabled", "database", rbacDBPath)

	// Initialize update checker
	updateChecker := updater.NewChecker("marcwoconnor/pCenter", 6*time.Hour)
	go updateChecker.Start(ctx)
	handler.SetUpdateChecker(updateChecker)
	slog.Info("update checker enabled", "version", updater.Version)

	// Initialize content library
	if cfg.Library.Enabled {
		libraryDB, err := library.Open(cfg.Library.DatabasePath)
		if err != nil {
			slog.Error("failed to open library database", "error", err)
			os.Exit(1)
		}
		defer libraryDB.Close()

		libraryService := library.NewService(libraryDB, store)
		handler.SetLibraryService(libraryService)
		slog.Info("content library enabled", "database", cfg.Library.DatabasePath)
	}

	// Tags
	tagsDB, err := tags.Open(cfg.Tags.DatabasePath)
	if err != nil {
		slog.Error("failed to open tags database", "error", err)
		os.Exit(1)
	}
	defer tagsDB.Close()
	tagsService := tags.NewService(tagsDB)
	handler.SetTagsService(tagsService)
	slog.Info("tags enabled", "database", cfg.Tags.DatabasePath)

	// DRS Rules
	rulesDB, err := drs.OpenRulesDB(cfg.DRSRules.DatabasePath)
	if err != nil {
		slog.Error("failed to open DRS rules database", "error", err)
		os.Exit(1)
	}
	defer rulesDB.Close()
	if p != nil {
		p.SetDRSRulesDB(rulesDB)
	}
	handler.SetDRSRulesDB(rulesDB)
	slog.Info("DRS rules enabled", "database", cfg.DRSRules.DatabasePath)

	// Alarms
	var alarmService *alarms.Service
	if cfg.Alarms.Enabled {
		alarmsDB, err := alarms.Open(cfg.Alarms.DatabasePath)
		if err != nil {
			slog.Error("failed to open alarms database", "error", err)
			os.Exit(1)
		}
		defer alarmsDB.Close()

		alarmService = alarms.NewService(alarmsDB, store, cfg.Alarms.EvalInterval, func() {
			hub.BroadcastState()
		})
		handler.SetAlarmsService(alarmService)
		hub.SetAlarmsService(alarmService)

		// Seed defaults on first run
		alarmsDB.SeedDefaults(ctx)

		slog.Info("alarms enabled", "database", cfg.Alarms.DatabasePath, "interval", cfg.Alarms.EvalInterval)
	}

	// Load clusters from inventory and start poller
	if cfg.Poller.Enabled && p != nil {
		clusterConfigs, err := inventoryService.GetClusterConfigs(ctx, cfg.ClusterSecrets)
		if err != nil {
			slog.Error("failed to load cluster configs", "error", err)
			os.Exit(1)
		}

		for _, clusterCfg := range clusterConfigs {
			p.AddCluster(clusterCfg)
			slog.Info("configured cluster", "name", clusterCfg.Name, "discovery", clusterCfg.DiscoveryNode)
		}

		p.Start(ctx)
		defer p.Stop()

		// Wait for initial discovery and poll
		time.Sleep(2 * time.Second)
	}

	// Initialize activity logging
	activityDB, err := activity.OpenDB(cfg.Activity.DatabasePath)
	if err != nil {
		slog.Error("failed to open activity database", "error", err)
		os.Exit(1)
	}
	defer activityDB.Close()

	activityService := activity.NewService(activityDB, cfg.Activity.RetentionDays)
	activityService.StartCleanup()
	handler.SetActivityService(activityService)
	slog.Info("activity logging enabled", "database", cfg.Activity.DatabasePath, "retention_days", cfg.Activity.RetentionDays)

	// Webhooks: translate activity entries into outbound POSTs
	var webhookSubscribe func(activity.Entry)
	webhooksDB, err := webhooks.Open(cfg.Webhooks.DatabasePath)
	if err != nil {
		slog.Error("failed to open webhooks database", "error", err)
		os.Exit(1)
	}
	defer webhooksDB.Close()
	webhooksSvc := webhooks.NewService(webhooksDB, authCrypto)
	webhooksSvc.Start(ctx)
	handler.SetWebhooksService(webhooksSvc)
	webhookSubscribe = webhooksSvc.Subscribe()
	slog.Info("webhooks enabled", "database", cfg.Webhooks.DatabasePath)

	// Fan activity entries out to both the WebSocket broadcast and webhooks.
	// OnLog is a single-subscriber slot, so we bundle fanout in one closure.
	activityService.OnLog(func(e activity.Entry) {
		hub.BroadcastActivity(e)
		if webhookSubscribe != nil {
			webhookSubscribe(e)
		}
	})

	// Start migration monitor (tracks task status for active migrations)
	migrationMonitor := migration.NewMonitor(store, cfg.Clusters, 3*time.Second)
	migrationMonitor.OnChange(broadcastFn)
	migrationMonitor.SetActivity(activityService)
	go migrationMonitor.Start(ctx)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start alarm evaluator
	if alarmService != nil {
		go alarmService.Evaluator.Run(ctx)
	}

	// Initialize scheduler
	schedulerDB, err := scheduler.Open("data/scheduler.db")
	if err != nil {
		slog.Error("failed to open scheduler database", "error", err)
		os.Exit(1)
	}
	defer schedulerDB.Close()

	// Action function: tries agent first, falls back to poller
	scheduleAction := func(sctx context.Context, cluster, node, action string, params map[string]interface{}) (string, error) {
		// Cluster-wide actions: bypass per-node agent/poller lookup
		if action == "cluster_acme_renew" {
			return clusterACMERenew(sctx, p, cluster)
		}

		// Try agent
		if agentHub != nil {
			agents := agentHub.GetConnectedAgents()
			key := cluster + "/" + node
			for _, a := range agents {
				if a == key {
					cmdID := fmt.Sprintf("sched-%d-%s", time.Now().UnixNano(), action)
					cmd := &agent.CommandData{ID: cmdID, Action: action, Params: params}
					resultCh, err := agentHub.SendCommand(cluster, node, cmd)
					if err == nil {
						select {
						case result := <-resultCh:
							if result.Success {
								return result.UPID, nil
							}
							return "", fmt.Errorf("agent: %s", result.Error)
						case <-sctx.Done():
							return "", fmt.Errorf("timeout")
						}
					}
					break
				}
			}
		}
		// Fallback: use poller client directly
		if p != nil {
			clients := p.GetClusterClients(cluster)
			if client, ok := clients[node]; ok {
				vmid := 0
				if v, ok := params["vmid"].(int); ok {
					vmid = v
				} else if v, ok := params["vmid"].(float64); ok {
					vmid = int(v)
				}
				switch action {
				case "vm_start":
					return client.StartVM(sctx, vmid)
				case "vm_stop":
					return client.StopVM(sctx, vmid)
				case "vm_shutdown":
					return client.ShutdownVM(sctx, vmid)
				case "ct_start":
					return client.StartContainer(sctx, vmid)
				case "ct_stop":
					return client.StopContainer(sctx, vmid)
				case "ct_shutdown":
					return client.ShutdownContainer(sctx, vmid)
				case "vm_snapshot_rotate":
					return rotateSnapshots(sctx, client, true, vmid, params)
				case "ct_snapshot_rotate":
					return rotateSnapshots(sctx, client, false, vmid, params)
				case "vm_backup", "ct_backup":
					storage, _ := params["storage"].(string)
					if storage == "" {
						return "", fmt.Errorf("backup task requires 'storage' param")
					}
					mode, _ := params["mode"].(string)
					compress, _ := params["compress"].(string)
					return client.CreateVzdump(sctx, pve.VzdumpOptions{
						VMIDs: []int{vmid}, Storage: storage, Mode: mode, Compress: compress,
					})
				default:
					return "", fmt.Errorf("unsupported action for poller fallback: %s", action)
				}
			}
		}
		return "", fmt.Errorf("no agent or poller client for %s/%s", cluster, node)
	}

	// Node resolver: finds which node a VM/CT is on
	nodeResolver := func(cluster string, targetType scheduler.TargetType, targetID int) string {
		cs, ok := store.GetCluster(cluster)
		if !ok {
			return ""
		}
		if targetType == scheduler.TargetVM {
			if vm, ok := cs.GetVM(targetID); ok {
				return vm.Node
			}
		} else {
			if ct, ok := cs.GetContainer(targetID); ok {
				return ct.Node
			}
		}
		return ""
	}

	schedulerService := scheduler.NewService(schedulerDB, scheduleAction, nodeResolver)
	handler.SetSchedulerService(schedulerService)
	go schedulerService.Start(ctx)
	slog.Info("scheduler enabled", "database", "data/scheduler.db")

	// Start server in goroutine
	go func() {
		slog.Info("starting server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Print summary after first poll
	time.Sleep(500 * time.Millisecond)
	gs := store.GetGlobalSummary()
	slog.Info("cluster state loaded",
		"clusters", len(gs.Clusters),
		"nodes", fmt.Sprintf("%d/%d online", gs.Total.OnlineNodes, gs.Total.TotalNodes),
		"vms", fmt.Sprintf("%d/%d running", gs.Total.RunningVMs, gs.Total.TotalVMs),
		"containers", fmt.Sprintf("%d/%d running", gs.Total.RunningCTs, gs.Total.TotalContainers),
	)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	slog.Info("goodbye!")
}

// migrateConfigClusters imports clusters from config.yaml into inventory DB (one-time migration)
func migrateConfigClusters(ctx context.Context, svc *inventory.Service, cfg *config.Config) error {
	// Check if inventory already has clusters
	count, err := svc.ClusterCount(ctx)
	if err != nil {
		return fmt.Errorf("check cluster count: %w", err)
	}
	if count > 0 {
		return nil // Already have clusters, skip migration
	}

	// Check if legacy clusters exist in config
	if len(cfg.Clusters) == 0 {
		return nil // Nothing to migrate
	}

	slog.Info("migrating clusters from config.yaml", "count", len(cfg.Clusters))

	// Create "Default" datacenter
	dc, err := svc.CreateDatacenter(ctx, inventory.CreateDatacenterRequest{
		Name:        "Default",
		Description: "Auto-created during migration from config.yaml",
	})
	if err != nil {
		return fmt.Errorf("create default datacenter: %w", err)
	}
	slog.Info("created default datacenter", "id", dc.ID)

	// Import each cluster with its host
	for _, legacy := range cfg.Clusters {
		cluster, err := svc.CreateCluster(ctx, inventory.CreateClusterRequest{
			Name:         legacy.Name,
			DatacenterID: &dc.ID,
		})
		if err != nil {
			slog.Warn("failed to migrate cluster", "name", legacy.Name, "error", err)
			continue
		}

		// Add the host from legacy config
		host, err := svc.AddHost(ctx, cluster.ID, inventory.AddHostRequest{
			Address:  legacy.DiscoveryNode,
			TokenID:  legacy.TokenID,
			Insecure: legacy.Insecure,
		})
		if err != nil {
			slog.Warn("failed to add host during migration", "cluster", legacy.Name, "error", err)
		} else {
			// Mark host as online and cluster as active (since it was working before)
			svc.SetHostStatus(ctx, host.ID, inventory.HostStatusOnline, "", "")
			svc.SetClusterStatus(ctx, cluster.ID, inventory.ClusterStatusActive)
		}

		slog.Info("migrated cluster", "name", legacy.Name, "datacenter", dc.Name)
	}

	return nil
}

// persistEncryptionKey writes the auto-generated encryption key to the env file
// Tries /etc/pcenter/env first (deb package), then /opt/pcenter/.env (manual install)
func persistEncryptionKey(key string) error {
	envPaths := []string{"/etc/pcenter/env", "/opt/pcenter/.env"}
	envLine := fmt.Sprintf("PCENTER_ENCRYPTION_KEY=%s", key)

	for _, path := range envPaths {
		// Check if file exists or directory exists
		dir := path[:strings.LastIndex(path, "/")]
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		// Read existing content to avoid duplicates
		existing := ""
		if data, err := os.ReadFile(path); err == nil {
			existing = string(data)
			// Check if key already set
			scanner := bufio.NewScanner(strings.NewReader(existing))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "PCENTER_ENCRYPTION_KEY=") {
					return nil // already set
				}
			}
		}

		// Append the key
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			continue // try next path
		}
		defer f.Close()

		if existing != "" && !strings.HasSuffix(existing, "\n") {
			envLine = "\n" + envLine
		}
		if _, err := fmt.Fprintln(f, envLine); err != nil {
			continue
		}

		slog.Info("encryption key saved", "path", path)
		return nil
	}

	return fmt.Errorf("no writable env file found (tried %v)", envPaths)
}
