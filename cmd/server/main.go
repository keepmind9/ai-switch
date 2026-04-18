package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/keepmind9/llm-gateway/internal/handler"
	"github.com/keepmind9/llm-gateway/internal/middleware"
	"github.com/keepmind9/llm-gateway/internal/router"
	"github.com/keepmind9/llm-gateway/internal/store"
)

var (
	configPath string
	showHelp   bool
)

func init() {
	flag.StringVar(&configPath, "c", "config.yaml", "path to config file")
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.BoolVar(&showHelp, "h", false, "show help")
	flag.BoolVar(&showHelp, "help", false, "show help")
}

func main() {
	flag.Parse()

	if showHelp {
		fmt.Fprintf(os.Stdout, "Usage: %s [options]\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}

	dataDir, err := config.EnsureDataDir()
	if err != nil {
		slog.Warn("failed to create data directory", "error", err)
	}

	resolvedPath := config.DefaultConfigPath(configPath)

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	provider := config.NewProvider(cfg, resolvedPath)

	// Initialize usage store
	dbPath := filepath.Join(dataDir, "usage.db")
	usageStore, err := store.NewUsageStore(dbPath)
	if err != nil {
		slog.Warn("failed to open usage database, stats disabled", "error", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Usage tracking middleware
	if usageStore != nil {
		upstreamName := extractProviderName(cfg)
		r.Use(middleware.UsageMiddleware(usageStore, upstreamName))
	}

	cfgRouter := router.NewConfigRouter(provider)
	h := handler.NewHandler(provider, usageStore, cfgRouter)
	h.RegisterRoutes(r)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		slog.Info("starting server", "addr", addr, "data_dir", dataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)

	for {
		select {
		case sig := <-quit:
			slog.Info("shutting down gracefully", "signal", sig)
			if usageStore != nil {
				usageStore.Close()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				slog.Error("forced shutdown", "error", err)
			}
			slog.Info("server exited")
			return

		case <-sighup:
			slog.Info("reloading config via SIGHUP")
			if err := provider.Reload(); err != nil {
				slog.Error("failed to reload config", "error", err)
			} else {
				slog.Info("config reloaded successfully")
			}
		}
	}
}

func extractProviderName(cfg *config.Config) string {
	if dp := cfg.DefaultProviderConfig(); dp != nil && dp.BaseURL != "" {
		return extractHost(dp.BaseURL)
	}
	return "unknown"
}

func extractHost(url string) string {
	// Simple extraction: remove scheme and path
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	if idx := strings.Index(url, "/"); idx > 0 {
		url = url[:idx]
	}
	return url
}
