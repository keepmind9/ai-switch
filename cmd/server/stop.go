package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
		return fmt.Errorf("%s is not running (PID file not found)", binName)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		return fmt.Errorf("invalid PID file content")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	if err := stopProcess(proc); err != nil {
		removePIDFile(dataDir)
		return fmt.Errorf("process %d not found, cleaned up stale PID file", pid)
	}

	fmt.Printf("%s stopped (PID %d)\n", binName, pid)
	return nil
}
