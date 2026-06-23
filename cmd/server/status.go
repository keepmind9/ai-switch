package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/log"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the running status of the daemon",
		// status is script-driven: silence cobra's "Error: exit code N" and
		// usage block on the normal not-running path. The exit code is still
		// set via errExitCode, which main.go turns into os.Exit.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			return runStatus(configPath)
		},
	}
}

func runStatus(configPath string) error {
	dataDir, err := config.DataDir()
	if err != nil {
		// SilenceErrors is on, so surface this unexpected failure ourselves.
		fmt.Fprintf(os.Stderr, "failed to get data directory: %v\n", err)
		return errExitCode{code: 1}
	}

	out, running := renderStatus(dataDir, configPath)
	fmt.Print(out)
	if !running {
		// Non-zero exit so scripts can branch on the daemon being down.
		return errExitCode{code: 1}
	}
	return nil
}

// renderStatus builds the daemon status report with one piece of information
// per line. When the daemon is not running, the reason and log path are still
// shown to aid diagnosis. The second return value reports whether the daemon
// process is currently alive.
func renderStatus(dataDir, configPath string) (string, bool) {
	logDir := log.LogDir(dataDir)
	pidPath := filepath.Join(dataDir, config.PidFileName)

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		// No PID file: never started (or removed).
		return fmt.Sprintf("%s is not running\n%s\n%s\n",
			binName, statusField("Reason", "no PID file"), statusField("Logs", logDir)), false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return fmt.Sprintf("%s is not running\n%s\n%s\n",
			binName, statusField("Reason", "invalid PID file"), statusField("Logs", logDir)), false
	}

	proc, findErr := os.FindProcess(pid)
	if findErr != nil || !processAlive(proc) {
		return fmt.Sprintf("%s is not running\n%s\n%s\n%s\n",
			binName,
			statusField("Reason", "process not alive (stale PID file)"),
			statusField("PID", strconv.Itoa(pid)),
			statusField("Logs", logDir)), false
	}

	// Running: resolve Addr/Config from the current config (best-effort — the
	// PID file does not record which config the daemon was started with).
	resolved, _ := config.DefaultConfigPath(configPath)
	addr := resolveListenAddr(resolved)

	var b strings.Builder
	fmt.Fprintf(&b, "%s is running\n", binName)
	fmt.Fprintf(&b, "%s\n", statusField("PID", strconv.Itoa(pid)))
	if addr != "" {
		fmt.Fprintf(&b, "%s\n", statusField("Addr", addr))
	}
	if resolved != "" {
		fmt.Fprintf(&b, "%s\n", statusField("Config", resolved))
	}
	fmt.Fprintf(&b, "%s\n", statusField("Logs", logDir))
	return b.String(), true
}

// resolveListenAddr loads the config at resolvedPath and returns "host:port".
// Returns "" if the config cannot be resolved or loaded.
func resolveListenAddr(resolvedPath string) string {
	if resolvedPath == "" {
		return ""
	}
	cfg, err := config.Load(resolvedPath)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
}

// statusField renders a "  Key: value" line, left-aligning keys to a fixed
// width so the values line up across rows.
func statusField(key, value string) string {
	return fmt.Sprintf("  %-8s %s", key+":", value)
}
