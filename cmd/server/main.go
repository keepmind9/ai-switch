package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/handler"
	"github.com/keepmind9/ai-switch/internal/log"
	"github.com/keepmind9/ai-switch/internal/middleware"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/store"
	"github.com/spf13/cobra"
)

var (
	configPath string
	asDaemon   bool
	version    = "dev"
	gitCommit  = "none"
	buildTime  = "unknown"
)

var versionTmpl = `Version:    %s
	Git commit: %s
	Built:      %s
	Go version: %s
	OS/Arch:    %s/%s
	`

func main() {
	rootCmd := &cobra.Command{
		Use:   "ai-switch",
		Short: "AI provider switching proxy",
	}
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config file")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the gateway server",
		Run:   runServe,
	}
	serveCmd.Flags().BoolVarP(&asDaemon, "daemon", "d", false, "run as background daemon")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the background daemon",
		Run:   runStop,
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Validate config file without starting the server",
		Run:   runCheck,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf(versionTmpl, version, gitCommit, buildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		},
	}

	rootCmd.AddCommand(serveCmd, stopCmd, checkCmd, versionCmd)

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
	if asDaemon {
		startDaemon()
		return
	}

	dataDir, err := config.EnsureDataDir()
	if err != nil {
		slog.Warn("failed to create data directory", "error", err)
	}

	if err := log.SetupDefaultLogger(dataDir); err != nil {
		slog.Warn("failed to setup file logger", "error", err)
	}

	llmLogger, err := log.NewLLMLogger(dataDir)
	if err != nil {
		slog.Warn("failed to setup LLM logger", "error", err)
	}

	resolvedPath, err := config.DefaultConfigPath(configPath)
	if err != nil {
		slog.Error("failed to resolve config path", "error", err)
		os.Exit(1)
	}

	slog.Info("loading config", "path", resolvedPath)

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log.SetRetentionDays(cfg.LogRetentionDays)

	provider := config.NewProvider(cfg, resolvedPath)

	dbPath := filepath.Join(dataDir, config.UsageDBName)
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
	h := handler.NewHandler(provider, usageStore, cfgRouter, llmLogger)
	h.RegisterRoutes(r)

	adminH := handler.NewAdminHandler(provider)
	adminGroup := r.Group("/api", middleware.LocalhostOnly())
	adminH.RegisterRoutes(adminGroup)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Write PID file
	writePIDFile(dataDir)
	defer removePIDFile(dataDir)

	go func() {
		slog.Info("starting server", "addr", addr, "config", resolvedPath, "data_dir", dataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if isAddrInUse(err) {
				slog.Error("port already in use",
					"port", cfg.Server.Port,
					"hint", fmt.Sprintf("edit %s to change the port", resolvedPath),
				)
			} else {
				slog.Error("server error", "error", err)
			}
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sighup := setupReloadSignal()

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

func startDaemon() {
	dataDir, err := config.EnsureDataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create data directory: %s\n", err)
		os.Exit(1)
	}

	pidPath := filepath.Join(dataDir, config.PidFileName)
	if pidData, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					fmt.Fprintf(os.Stderr, "ai-switch is already running (PID %d)\n", pid)
					os.Exit(1)
				}
			}
		}
	}

	resolvedPath, _ := config.DefaultConfigPath(configPath)

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get executable path: %s\n", err)
		os.Exit(1)
	}

	// Build args without -d/--daemon
	args := []string{"serve"}
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-d", "--daemon":
			// skip
		case "-c":
			args = append(args, "-c", os.Args[i+1])
			i++
		case "--config":
			args = append(args, "--config", os.Args[i+1])
			i++
		default:
			args = append(args, os.Args[i])
		}
	}

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	setDaemonSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start daemon: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("  _   _  _     ___  _       _")
	fmt.Println(" / \\ | || |   / __|| |_    / |")
	fmt.Println("/  |_ | || |__ \\__ \\| ' \\  | |")
	fmt.Println("\\__/ |_||____||___/|_||_| |_|")
	fmt.Println()
	fmt.Printf("  ai-switch started (PID %d)\n", cmd.Process.Pid)
	fmt.Printf("  Config:  %s\n", resolvedPath)
	fmt.Printf("  Data:    %s\n", dataDir)
	fmt.Printf("  Logs:    %s\n", log.LogDir(dataDir))
	fmt.Println()
	fmt.Println("  Use 'ai-switch stop' to stop the daemon.")
}

func runStop(_ *cobra.Command, _ []string) {
	dataDir, err := config.DataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get data directory: %s\n", err)
		os.Exit(1)
	}

	pidPath := filepath.Join(dataDir, config.PidFileName)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ai-switch is not running (PID file not found)\n")
		os.Exit(1)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid PID file content\n")
		os.Exit(1)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find process %d: %s\n", pid, err)
		os.Exit(1)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Process may have exited, clean up stale PID file
		removePIDFile(dataDir)
		fmt.Fprintf(os.Stderr, "process %d not found, cleaned up stale PID file\n", pid)
		os.Exit(1)
	}

	fmt.Printf("ai-switch stopped (PID %d)\n", pid)
}

func writePIDFile(dataDir string) {
	pidPath := filepath.Join(dataDir, config.PidFileName)
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func removePIDFile(dataDir string) {
	pidPath := filepath.Join(dataDir, config.PidFileName)
	_ = os.Remove(pidPath)
}

func runCheck(_ *cobra.Command, _ []string) {
	resolvedPath, err := config.DefaultConfigPath(configPath)
	if err != nil {
		fmt.Printf("✗ %s\n", err)
		os.Exit(1)
	}

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

func isAddrInUse(err error) bool {
	return strings.Contains(err.Error(), "address already in use")
}
