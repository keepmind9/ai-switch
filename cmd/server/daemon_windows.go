//go:build windows

package main

import (
	"bytes"
	"fmt"
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

// processAlive reports whether the given process is currently running. Windows
// has no POSIX signal-0 equivalent, so we shell out to tasklist (present on
// desktop Windows; may be absent on Server Core / minimal images) filtered by
// PID. If tasklist is unavailable or fails, this returns false.
func processAlive(proc *os.Process) bool {
	if proc == nil {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", proc.Pid), "/NH").Output()
	if err != nil {
		return false
	}
	return bytes.Contains(out, []byte(fmt.Sprintf("%d", proc.Pid)))
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
