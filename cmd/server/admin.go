package main

import (
	"github.com/keepmind9/ai-switch/internal/admincli"
	"github.com/spf13/cobra"
)

func newAdminCmd(configPath string) *cobra.Command {
	var urlFlag string

	cmd := &cobra.Command{
		Use:   "admin [command]",
		Short: "Manage ais server via admin API",
		Long: `Manage providers, routes, settings and more via the ais admin API.

Run without a subcommand to enter interactive mode.
Use --url to connect to a remote server.

Examples:
  ais admin provider list
  ais admin route add --key mykey --provider openai --default-model gpt-4o
  ais admin --url http://remote:12345 status`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			baseURL, err := admincli.ResolveAdminURL(configPath, urlFlag)
			if err != nil {
				return err
			}
			client := admincli.NewAdminClient(baseURL)
			admincli.InjectClient(cmd, client)
			return nil
		},
		// No subcommand: enter interactive REPL.
		RunE: func(cmd *cobra.Command, args []string) error {
			return admincli.RunREPL(cmd, nil, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&urlFlag, "url", "", "Server URL (default: from config or http://127.0.0.1:12345)")

	// Add resource subcommands.
	cmd.AddCommand(
		admincli.NewProviderCmd(),
		admincli.NewRouteCmd(),
		admincli.NewSettingsCmd(),
		admincli.NewSystemCmd(),
	)

	return cmd
}
