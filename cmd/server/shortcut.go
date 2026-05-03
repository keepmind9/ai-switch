package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/spf13/cobra"
)

func newShortcutCmd(configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "shortcut",
		Short: "Create desktop shortcuts to start/stop ai-switch",
		RunE: func(_ *cobra.Command, _ []string) error {
			return createShortcuts(configPath)
		},
	}
}

func createShortcuts(configPath string) error {
	resolvedPath, err := config.DefaultConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	host := cfg.Server.Host
	if host == "0.0.0.0" || host == "127.0.0.1" {
		host = "localhost"
	}
	uiURL := fmt.Sprintf("http://%s:%d/ui", host, cfg.Server.Port)

	desktopDir, err := getDesktopDir()
	if err != nil {
		return fmt.Errorf("failed to find desktop directory: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		return createWindowsShortcuts(desktopDir, execPath, uiURL, configPath)
	case "darwin":
		return createDarwinShortcuts(desktopDir, execPath, uiURL, configPath)
	default:
		return createLinuxShortcuts(desktopDir, execPath, uiURL, configPath)
	}
}

func getDesktopDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Desktop"), nil
}

func buildServeArgs(configPath string) string {
	if configPath == "" {
		return "serve -d"
	}
	return fmt.Sprintf("serve -d -c \"%s\"", configPath)
}

func buildStopArgs() string {
	return "stop"
}

func createWindowsShortcuts(desktopDir, execPath, uiURL, configPath string) error {
	// Start shortcut
	startPath := filepath.Join(desktopDir, "AI-Switch.vbs")
	startContent := fmt.Sprintf(`Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s"" %s", 0, False
WScript.Sleep 2000
WshShell.Run "%s"
`, execPath, buildServeArgs(configPath), uiURL)

	if err := os.WriteFile(startPath, []byte(startContent), 0644); err != nil {
		return fmt.Errorf("failed to create start shortcut: %w", err)
	}

	// Stop shortcut
	stopPath := filepath.Join(desktopDir, "Stop-AI-Switch.vbs")
	stopContent := fmt.Sprintf(`Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s"" %s", 0, True
`, execPath, buildStopArgs())

	if err := os.WriteFile(stopPath, []byte(stopContent), 0644); err != nil {
		return fmt.Errorf("failed to create stop shortcut: %w", err)
	}

	fmt.Printf("Desktop shortcuts created:\n  Start: %s\n  Stop:  %s\n", startPath, stopPath)
	return nil
}

func createDarwinShortcuts(desktopDir, execPath, uiURL, configPath string) error {
	// Start shortcut
	startPath := filepath.Join(desktopDir, "AI-Switch.command")
	startContent := fmt.Sprintf(`#!/bin/bash
"%s" %s
sleep 2
open "%s"
`, execPath, buildServeArgs(configPath), uiURL)

	if err := os.WriteFile(startPath, []byte(startContent), 0755); err != nil {
		return fmt.Errorf("failed to create start shortcut: %w", err)
	}

	// Stop shortcut
	stopPath := filepath.Join(desktopDir, "Stop-AI-Switch.command")
	stopContent := fmt.Sprintf(`#!/bin/bash
"%s" %s
`, execPath, buildStopArgs())

	if err := os.WriteFile(stopPath, []byte(stopContent), 0755); err != nil {
		return fmt.Errorf("failed to create stop shortcut: %w", err)
	}

	fmt.Printf("Desktop shortcuts created:\n  Start: %s\n  Stop:  %s\n", startPath, stopPath)
	return nil
}

func createLinuxShortcuts(desktopDir, execPath, uiURL, configPath string) error {
	// Start shortcut
	startPath := filepath.Join(desktopDir, "ai-switch.desktop")
	startContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=AI Switch
Comment=Start ai-switch and open Web UI
Exec=bash -c '%s %s && sleep 2 && xdg-open "%s"'
Terminal=false
`, execPath, buildServeArgs(configPath), uiURL)

	if err := os.WriteFile(startPath, []byte(startContent), 0755); err != nil {
		return fmt.Errorf("failed to create start shortcut: %w", err)
	}

	// Stop shortcut
	stopPath := filepath.Join(desktopDir, "stop-ai-switch.desktop")
	stopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Stop AI Switch
Comment=Stop ai-switch daemon
Exec=%s %s
Terminal=false
`, execPath, buildStopArgs())

	if err := os.WriteFile(stopPath, []byte(stopContent), 0755); err != nil {
		return fmt.Errorf("failed to create stop shortcut: %w", err)
	}

	fmt.Printf("Desktop shortcuts created:\n  Start: %s\n  Stop:  %s\n", startPath, stopPath)
	return nil
}
