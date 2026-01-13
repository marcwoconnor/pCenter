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

	"github.com/moconnor/pcenter/internal/api"
	"github.com/moconnor/pcenter/internal/config"
	"github.com/moconnor/pcenter/internal/metrics"
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

	slog.Info("loaded config", "clusters", len(cfg.Clusters), "port", cfg.Server.Port, "drs", cfg.DRS.Enabled)

	// Create state store
	store := state.New()

	// Create multi-cluster poller with DRS
	p := poller.New(store, 5*time.Second, cfg.DRS)

	// Add configured clusters
	for _, clusterCfg := range cfg.Clusters {
		p.AddCluster(clusterCfg)
		slog.Info("configured cluster", "name", clusterCfg.Name, "discovery", clusterCfg.DiscoveryNode)
	}

	// Create WebSocket hub
	hub := api.NewHub(store)
	go hub.Run()

	// Broadcast state changes via WebSocket
	p.OnChange(func() {
		hub.BroadcastState()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.Start(ctx)
	defer p.Stop()

	// Wait for initial discovery and poll
	time.Sleep(2 * time.Second)

	// Create HTTP server
	router, handler := api.NewRouter(store, p, hub, cfg.Server.CORSOrigins)

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
