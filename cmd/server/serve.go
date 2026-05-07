package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/handler"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/keepmind9/ai-switch/internal/log"
	"github.com/keepmind9/ai-switch/internal/middleware"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/store"
	"github.com/spf13/cobra"
)

const (
	restartEnvKey   = "AI_SWITCH_RESTART"
	restartAttempts = 20
	restartInterval = 250 * time.Millisecond
)

func newServeCmd(configPath string) *cobra.Command {
	var asDaemon bool

	cmd := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"start"},
		Short:   fmt.Sprintf("Start the %s proxy server", binName),
		RunE: func(_ *cobra.Command, _ []string) error {
			if asDaemon {
				return startDaemon(configPath)
			}
			return runServe(configPath)
		},
	}
	cmd.Flags().BoolVarP(&asDaemon, "daemon", "d", false, "run as background daemon")
	return cmd
}

func runServe(configPath string) error {
	isRestart := os.Getenv(restartEnvKey) == "1"
	os.Unsetenv(restartEnvKey)

	dataDir, err := config.EnsureDataDir()
	if err != nil {
		slog.Warn("failed to create data directory", "error", err)
	}

	if err := log.SetupDefaultLogger(dataDir); err != nil {
		slog.Warn("failed to setup file logger", "error", err)
	}

	llmWriter, err := log.NewLLMWriter(dataDir)
	if err != nil {
		slog.Warn("failed to setup LLM trace writer", "error", err)
	}

	idxWriter, err := log.NewDailyRotateWriter(dataDir, "llm-idx")
	if err != nil {
		slog.Warn("failed to setup LLM index writer", "error", err)
	}

	resolvedPath, err := config.DefaultConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	slog.Info("loading config", "path", resolvedPath)

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
	r.Use(middleware.IPWhitelist(cfg.Server.Host, cfg.Server.AllowedIPs))

	cfgRouter := router.NewConfigRouter(provider)
	traceRecorder := hook.NewTraceRecorder(llmWriter, idxWriter)
	h := handler.NewHandler(provider, usageStore, cfgRouter, traceRecorder)
	h.SyncKeys()

	if usageStore != nil {
		h.RegisterHook(hook.NewUsageHook(usageStore))
	}
	h.RegisterRoutes(r)

	restartCh := make(chan struct{}, 1)
	stopCh := make(chan struct{}, 1)

	adminH := handler.NewAdminHandler(provider, restartCh, stopCh)
	adminGroup := r.Group("/api", middleware.LocalhostOnly())
	adminH.RegisterRoutes(adminGroup)

	traceH := handler.NewTraceHandler(dataDir)
	traceH.RegisterRoutes(adminGroup)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	writePIDFile(dataDir)
	defer removePIDFile(dataDir)

	ln, err := listenWithRetry(addr, isRestart)
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("port %d already in use, edit %s to change the port", cfg.Server.Port, resolvedPath)
		}
		return fmt.Errorf("server error: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting server", "addr", addr, "config", resolvedPath, "data_dir", dataDir)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server error: %w", err)
			return
		}
		errCh <- nil
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
			return nil

		case <-sighup:
			slog.Info("reloading config via SIGHUP")
			if err := provider.Reload(); err != nil {
				slog.Error("failed to reload config", "error", err)
			} else {
				h.SyncKeys()
				slog.Info("config reloaded successfully")
			}

		case <-restartCh:
			slog.Info("restarting server with new config")
			if usageStore != nil {
				usageStore.Close()
			}
			// Spawn new process with retry flag so it retries port binding.
			if err := spawnRestartServer(configPath); err != nil {
				slog.Error("failed to spawn new server", "error", err)
				return err
			}
			// Close listener immediately so new process can bind.
			ln.Close()
			slog.Info("server restarted")
			return nil

		case <-stopCh:
			slog.Info("stopping server via admin API")
			if usageStore != nil {
				usageStore.Close()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				slog.Error("forced shutdown", "error", err)
			}
			slog.Info("server stopped")
			return nil

		case err := <-errCh:
			return err
		}
	}
}

// listenWithRetry binds to addr. On restart, retries up to restartAttempts times
// with restartInterval between attempts to wait for the old process to release the port.
func listenWithRetry(addr string, isRestart bool) (net.Listener, error) {
	attempts := 1
	if isRestart {
		attempts = restartAttempts
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		lastErr = err
		if isRestart && i < attempts-1 {
			slog.Info("waiting for port to be released", "addr", addr, "attempt", i+1)
			time.Sleep(restartInterval)
		}
	}
	return nil, lastErr
}

func startDaemon(configPath string) error {
	dataDir, err := config.EnsureDataDir()
	if err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	pidPath := filepath.Join(dataDir, config.PidFileName)
	if pidData, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil {
			if proc, err := os.FindProcess(pid); err == nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					return fmt.Errorf("%s is already running (PID %d)", binName, pid)
				}
			}
		}
	}

	resolvedPath, _ := config.DefaultConfigPath(configPath)
	displayAddr := "http://localhost:12345"
	if cfg, err := config.Load(resolvedPath); err == nil {
		host := cfg.Server.Host
		if host == "0.0.0.0" {
			host = "localhost"
		}
		displayAddr = fmt.Sprintf("http://%s:%d/ui", host, cfg.Server.Port)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	args := []string{"serve"}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	cmd := exec.Command(execPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	setDaemonSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	fmt.Println("  _   _  _     ___  _       _")
	fmt.Println(" / \\ | || |   / __|| |_    / |")
	fmt.Println("/  |_ | || |__ \\__ \\| ' \\  | |")
	fmt.Println("\\__/ |_||____||___/|_||_| |_|")
	fmt.Println()
	fmt.Printf("  %s started (PID %d)\n", binName, cmd.Process.Pid)
	fmt.Printf("  Config:   %s\n", resolvedPath)
	fmt.Printf("  Data:     %s\n", dataDir)
	fmt.Printf("  Logs:     %s\n", log.LogDir(dataDir))
	fmt.Printf("  Admin UI: %s\n", displayAddr)
	fmt.Println()
	fmt.Printf("  Use '%s stop' to stop the daemon.\n", binName)
	return nil
}

func writePIDFile(dataDir string) {
	pidPath := filepath.Join(dataDir, config.PidFileName)
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func removePIDFile(dataDir string) {
	pidPath := filepath.Join(dataDir, config.PidFileName)
	_ = os.Remove(pidPath)
}

func isAddrInUse(err error) bool {
	return strings.Contains(err.Error(), "address already in use")
}
