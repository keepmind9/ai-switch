//go:build !windows

package main

import (
	"fmt"
	"syscall"
)

// execAgentDefault replaces the current process with the agent binary via
// syscall.Exec. The ais process disappears; the agent takes over directly.
// Exit code, signals, and stdio are all inherited transparently.
func execAgentDefault(binary string, args []string, env []string) error {
	err := syscall.Exec(binary, append([]string{binary}, args...), env)
	return fmt.Errorf("failed to exec agent: %w", err)
}
