//go:build windows

package main

import (
	"os"
	"os/exec"
)

func setDaemonSysProcAttr(_ *exec.Cmd) {
	// No-op on Windows
}

func setupReloadSignal() chan os.Signal {
	// SIGHUP is not available on Windows
	return nil
}

func stopProcess(proc *os.Process) error {
	return proc.Kill()
}

func spawnRestartServer(configPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"serve"}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	cmd := exec.Command(execPath, args...)
	cmd.Env = append(os.Environ(), "AI_SWITCH_RESTART=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
