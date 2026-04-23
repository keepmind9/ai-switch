package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "ai-switch",
		Short: "AI provider switching proxy",
	}
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config file")
	rootCmd.AddCommand(
		newServeCmd(configPath),
		newStopCmd(),
		newCheckCmd(configPath),
		newVersionCmd(),
	)
	return rootCmd
}
