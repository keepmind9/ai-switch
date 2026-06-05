package admincli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type ctxKey struct{}

var clientKey ctxKey

// InjectClient stores an AdminClient in the cobra command's context.
func InjectClient(cmd *cobra.Command, client *AdminClient) {
	ctx := context.WithValue(cmd.Context(), clientKey, client)
	cmd.SetContext(ctx)
}

// ClientFromCmd retrieves the AdminClient from the cobra command's context.
// Panics with a clear message if the client was not injected.
func ClientFromCmd(cmd *cobra.Command) *AdminClient {
	val := cmd.Context().Value(clientKey)
	if val == nil {
		panic("admin client not initialized — this is a bug, please report")
	}
	return val.(*AdminClient)
}

// FormatterFromCmd creates a Formatter from the command's --output flag.
func FormatterFromCmd(cmd *cobra.Command) *Formatter {
	s, _ := cmd.Flags().GetString("output")
	if s == "" {
		s = "table"
	}
	f, err := ParseOutputFormat(s)
	if err != nil {
		f = FormatTable
	}
	_ = err
	return &Formatter{Format: f, Out: cmd.OutOrStdout()}
}

// bindOutputFlag adds the --output / -o flag to a command.
func bindOutputFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "table", "Output format: table, json")
}

// joinSlice joins a string slice with comma, returns "-" if empty.
func joinSlice(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ", ")
}

// needMinArgs returns a cobra.PositionalArgs validator that requires at least min args.
func needMinArgs(min int, usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < min {
			return fmt.Errorf("usage: ais admin %s", usage)
		}
		return nil
	}
}
