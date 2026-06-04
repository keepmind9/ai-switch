//go:build windows

package main

// execAgentDefault falls back to child-process mode on Windows
// (syscall.Exec is not available).
func execAgentDefault(binary string, args []string, env []string) error {
	return execAgentChildProcess(binary, args, env)
}
