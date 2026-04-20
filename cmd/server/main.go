package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/keepmind9/llm-gateway/internal/handler"
	"github.com/keepmind9/llm-gateway/internal/middleware"
	"github.com/keepmind9/llm-gateway/internal/router"
	"github.com/keepmind9/llm-gateway/internal/store"
	"github.com/spf13/cobra"
)

var configPath string

func main() {
	rootCmd := &cobra.Command{
		Use:   "llm-gateway",
		Short: "LLM API Gateway",
	}
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "path to config file")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the gateway server",
		Run:   runServe,
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Validate config file without starting the server",
		Run:   runCheck,
	}

	rootCmd.AddCommand(serveCmd, checkCmd)

	// Default to serve when no subcommand is given
	if len(os.Args) == 1 || (len(os.Args) > 1 && os.Args[1][0] == '-') {
		args := append([]string{"serve"}, os.Args[1:]...)
		rootCmd.SetArgs(args)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(_ *cobra.Command, _ []string) {
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

	dbPath := filepath.Join(dataDir, "usage.db")
	usageStore, err := store.NewUsageStore(dbPath)
	if err != nil {
		slog.Warn("failed to open usage database, stats disabled", "error", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	if usageStore != nil {
		r.Use(middleware.UsageMiddleware(usageStore))
	}

	cfgRouter := router.NewConfigRouter(provider)
	h := handler.NewHandler(provider, usageStore, cfgRouter)
	h.RegisterRoutes(r)

	adminH := handler.NewAdminHandler(provider)
	adminGroup := r.Group("/api", middleware.LocalhostOnly())
	adminH.RegisterRoutes(adminGroup)

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

func runCheck(_ *cobra.Command, _ []string) {
	resolvedPath := config.DefaultConfigPath(configPath)

	fmt.Printf("Checking %s ...\n\n", resolvedPath)

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		fmt.Printf("✗ Parse error: %s\n", err)
		os.Exit(1)
	}

	result := config.Validate(cfg)

	fmt.Printf("  Providers: %d\n", len(cfg.Providers))
	fmt.Printf("  Routes:    %d\n", len(cfg.Routes))
	if cfg.DefaultRoute != "" {
		fmt.Printf("  Default:   %s\n", cfg.DefaultRoute)
	}
	fmt.Println()

	if len(result.Warnings) > 0 {
		fmt.Println("⚠ Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  - %s\n", w.Message)
		}
		fmt.Println()
	}

	if len(result.Errors) > 0 {
		fmt.Println("✗ Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e.Message)
		}
		fmt.Println()
		fmt.Printf("%d error(s), %d warning(s) found.\n", len(result.Errors), len(result.Warnings))
		os.Exit(1)
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("✓ No errors, %d warning(s).\n", len(result.Warnings))
		os.Exit(2)
	}

	fmt.Println("✓ Config is valid.")
}
