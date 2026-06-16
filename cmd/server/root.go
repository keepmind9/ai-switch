package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   binName,
		Short: "AI provider switching proxy",
	}
	// Registered as a persistent flag so every subcommand inherits it; each
	// command reads the value at RunE time via cmd.Flags().GetString("config").
	// Binding a variable here would only snapshot its value at registration
	// time (always ""), silently ignoring -c. See TestConfigFlagReachesCommandRunE.
	rootCmd.PersistentFlags().StringP("config", "c", "", "path to config file")
	rootCmd.AddCommand(
		newServeCmd(),
		newStopCmd(),
		newCheckCmd(),
		newVersionCmd(),
		newAgentCmd(),
		newShortcutCmd(),
		newUpdateCmd(),
		newAdminCmd(),
	)
	return rootCmd
}
