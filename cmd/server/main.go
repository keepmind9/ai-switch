package main

import (
	"errors"
	"fmt"
	"os"
)

// errExitCode wraps an exit code for commands that need non-1 exit codes.
type errExitCode struct {
	code int
}

func (e errExitCode) Error() string {
	return fmt.Sprintf("exit code %d", e.code)
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		var eec errExitCode
		if errors.As(err, &eec) {
			os.Exit(eec.code)
		}
		os.Exit(1)
	}
}
