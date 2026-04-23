package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the background daemon",
		RunE:  runStop,
	}
}

func runStop(_ *cobra.Command, _ []string) error {
	dataDir, err := config.DataDir()
	if err != nil {
		return fmt.Errorf("failed to get data directory: %w", err)
	}

	pidPath := filepath.Join(dataDir, config.PidFileName)
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("ai-switch is not running (PID file not found)")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return fmt.Errorf("invalid PID file content")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		removePIDFile(dataDir)
		return fmt.Errorf("process %d not found, cleaned up stale PID file", pid)
	}

	fmt.Printf("ai-switch stopped (PID %d)\n", pid)
	return nil
}
