package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/moconnor/pve-agent/internal/client"
	"github.com/moconnor/pve-agent/internal/collector"
	"github.com/moconnor/pve-agent/internal/config"
	"github.com/moconnor/pve-agent/internal/executor"
	"github.com/moconnor/pve-agent/internal/types"
)

func main() {
	configPath := flag.String("config", "/etc/pve-agent/config.yaml", "Path to config file")
	flag.Parse()

	// Setup logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("pve-agent starting", "config", *configPath)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("config loaded",
		"node", cfg.Node.Name,
		"cluster", cfg.Node.Cluster,
		"pcenter_url", cfg.PCenter.URL)

	// Create WebSocket client
	wsClient := client.NewClient(
		cfg.PCenter.URL,
		cfg.PCenter.Token,
		cfg.Node.Name,
		cfg.Node.Cluster,
	)

	// Create SMART collector if enabled. Runs as its own goroutine on a
	// slower interval (default 300s) — see SmartCollector docs.
	var smartColl *collector.SmartCollector
	if cfg.Collection.IncludeSmart {
		smartColl = collector.NewSmartCollector(
			cfg.Node.Name,
			cfg.Node.Cluster,
			time.Duration(cfg.Collection.SmartInterval)*time.Second,
		)
	}

	// Create collector
	coll := collector.NewCollector(cfg, wsClient, smartColl)

	// Create executor for handling commands
	exec := executor.NewExecutor(coll.API())

	// Register command handler
	wsClient.OnMessage(types.MsgTypeCommand, func(data json.RawMessage) {
		var cmd types.CommandData
		if err := json.Unmarshal(data, &cmd); err != nil {
			slog.Error("failed to parse command", "error", err)
			return
		}

		slog.Info("received command", "id", cmd.ID, "action", cmd.Action)

		// Execute and send result
		result := exec.Execute(context.Background(), &cmd)
		wsClient.Send(&types.Message{
			Type:      types.MsgTypeCommandResult,
			Timestamp: time.Now().Unix(),
			Data:      result,
		})
	})

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Connect with retry loop
	go connectWithRetry(ctx, wsClient)

	// Start collector
	go coll.Start(ctx)

	// Start SMART collector if configured
	if smartColl != nil {
		go smartColl.Start(ctx)
	}

	// Wait for shutdown signal
	sig := <-sigCh
	slog.Info("received signal, shutting down", "signal", sig)

	cancel()
	wsClient.Close()

	slog.Info("pve-agent stopped")
}

func connectWithRetry(ctx context.Context, c *client.Client) {
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.Connect(ctx)
		if err == nil {
			backoff = time.Second // Reset on success
			// Block until disconnected
			for c.IsConnected() {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
			slog.Warn("disconnected from pCenter, reconnecting...")
		} else {
			slog.Error("connection failed", "error", err, "retry_in", backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff, max 30s
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}
