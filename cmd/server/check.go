package main

import (
	"fmt"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/spf13/cobra"
)

func newCheckCmd(configPath string) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate config file without starting the server",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCheck(configPath)
		},
	}
}

func runCheck(configPath string) error {
	resolvedPath, err := config.DefaultConfigPath(configPath)
	if err != nil {
		fmt.Printf("✗ %s\n", err)
		return errExitCode{code: 1}
	}

	fmt.Printf("Checking %s ...\n\n", resolvedPath)

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		fmt.Printf("✗ Parse error: %s\n", err)
		return errExitCode{code: 1}
	}

	result := config.Validate(cfg)

	fmt.Printf("  Providers: %d\n", len(cfg.Providers))
	fmt.Printf("  Routes:    %d\n", len(cfg.Routes))
	if cfg.DefaultRoute != "" {
		fmt.Printf("  Default:   %s\n", cfg.DefaultRoute)
	}
	fmt.Println()

	if len(result.Warnings) > 0 {
		fmt.Println("⚠ Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  - %s\n", w.Message)
		}
		fmt.Println()
	}

	if len(result.Errors) > 0 {
		fmt.Println("✗ Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e.Message)
		}
		fmt.Println()
		fmt.Printf("%d error(s), %d warning(s) found.\n", len(result.Errors), len(result.Warnings))
		return errExitCode{code: 1}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("✓ No errors, %d warning(s).\n", len(result.Warnings))
		return errExitCode{code: 2}
	}

	fmt.Println("✓ Config is valid.")
	return nil
}
