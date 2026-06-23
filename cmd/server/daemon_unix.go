//go:build !windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func setupReloadSignal() chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	return ch
}

func stopProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}

// processAlive reports whether the given process is currently running, using
// signal 0 (no signal is delivered; this is a pure existence/permission check).
func processAlive(proc *os.Process) bool {
	if proc == nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}
