package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moconnor/pcenter/internal/activity"
	"github.com/moconnor/pcenter/internal/agent"
	"github.com/moconnor/pcenter/internal/api"
	"github.com/moconnor/pcenter/internal/auth"
	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/folders"
	"github.com/moconnor/pcenter/internal/inventory"
	"github.com/moconnor/pcenter/internal/metrics"
	"github.com/moconnor/pcenter/internal/migration"
	"github.com/moconnor/pcenter/internal/poller"
	"github.com/moconnor/pcenter/internal/state"
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

	// Create WebSocket hub (browser clients)
	hub := api.NewHub(store)
	go hub.Run()

	// Create agent hub (pve-agents)
	agentHub := agent.NewHub(store)

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
	if cfg.Auth.Enabled {
		authDB, err := auth.Open(cfg.Auth.DatabasePath)
		if err != nil {
			slog.Error("failed to open auth database", "error", err)
			os.Exit(1)
		}
		defer authDB.Close()

		// Initialize crypto (optional - for TOTP secret encryption)
		crypto, err := auth.NewCrypto(cfg.Auth.EncryptionKey)
		if err != nil {
			slog.Error("failed to initialize auth crypto", "error", err)
			os.Exit(1)
		}

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
	activityService.OnLog(func(e activity.Entry) {
		hub.BroadcastActivity(e)
	})
	activityService.StartCleanup()
	handler.SetActivityService(activityService)
	slog.Info("activity logging enabled", "database", cfg.Activity.DatabasePath, "retention_days", cfg.Activity.RetentionDays)

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
