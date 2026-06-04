package main

import (
	"fmt"
	"os"
	"os/exec"
)

// execAgentFunc launches the agent binary. Overridden in tests to use child
// process mode (syscall.Exec would replace the test process itself).
var execAgentFunc = execAgentDefault

// execAgentChildProcess runs the agent as a child process. Used on Windows
// (which lacks syscall.Exec) and in all test builds.
func execAgentChildProcess(binary string, args []string, env []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return errExitCode{code: exitErr.ExitCode()}
		}
		return fmt.Errorf("failed to run agent: %w", err)
	}
	return nil
}
