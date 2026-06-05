package admincli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
)

const prompt = "ais> "

// RunREPL starts an interactive command loop.
// rootCmd is the admin command tree; each input line is dispatched as a subcommand.
func RunREPL(rootCmd *cobra.Command, in io.Reader, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	fmt.Fprintln(out, "Type 'help' for available commands, 'exit' to quit.")

	rl, err := readline.NewEx(&readline.Config{
		Prompt: prompt,
		Stdin:  io.NopCloser(in),
		Stdout: out,
	})
	if err != nil {
		return err
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			// io.EOF (Ctrl+D) or readline.ErrInterrupt (Ctrl+C)
			fmt.Fprintln(out, "bye.")
			return nil
		}
		line = strings.TrimSpace(line)

		switch {
		case line == "":
			continue
		case line == "exit", line == "quit":
			fmt.Fprintln(out, "bye.")
			return nil
		case line == "help":
			rootCmd.SetOut(out)
			rootCmd.Help()
			continue
		}

		rl.SaveHistory(line)

		// Build a fresh command for each line so state doesn't leak.
		args := strings.Fields(line)
		tmp := cloneCommandTree(rootCmd)
		tmp.SetArgs(args)
		tmp.SetOut(out)
		tmp.SetErr(out)

		if err := tmp.Execute(); err != nil {
			fmt.Fprintf(out, "Error: %s\n", err)
		}
	}
}

// cloneCommandTree shallow-clones the root command and its direct subcommands.
// PersistentPreRunE is preserved so client injection propagates to children.
func cloneCommandTree(src *cobra.Command) *cobra.Command {
	clone := &cobra.Command{
		Use:               src.Use,
		Short:             src.Short,
		RunE:              src.RunE,
		PersistentPreRunE: src.PersistentPreRunE,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	// Preserve context (carries injected client) and flags.
	clone.SetContext(src.Context())
	clone.Flags().AddFlagSet(src.Flags())

	for _, sub := range src.Commands() {
		clone.AddCommand(sub)
	}
	return clone
}
