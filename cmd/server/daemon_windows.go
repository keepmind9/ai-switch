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
