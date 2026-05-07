package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// binName is the compiled binary name used in user-facing messages.
const binName = "ais"

var (
	version   = "dev"
	gitCommit = "none"
	buildTime = "unknown"
)

var versionTmpl = `Version:    %s
Git commit: %s
Built:      %s
Go version: %s
OS/Arch:    %s/%s
`

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), versionTmpl, version, gitCommit, buildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
