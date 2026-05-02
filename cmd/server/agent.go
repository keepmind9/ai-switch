package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/spf13/cobra"
)

// agentEnvMap maps agent names to their environment variable names.
var agentEnvMap = map[string]struct {
	baseURLKey string
	apiKeyKey  string
}{
	"claude": {
		baseURLKey: "ANTHROPIC_BASE_URL",
		apiKeyKey:  "ANTHROPIC_API_KEY",
	},
	"codex": {
		baseURLKey: "OPENAI_BASE_URL",
		apiKeyKey:  "OPENAI_API_KEY",
	},
}

func newAgentCmd(configPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent <route_key> <agent> [agent_args...]",
		Short: "Launch an AI agent with ai-switch config",
		Long: `Launch an AI agent (claude/codex) with environment variables
automatically configured from a route key.

The route_key is used as the API key for the agent, and the base URL
is set to the ai-switch server address from the config file.

Examples:
  ai-switch agent my-key claude --continue
  ai-switch agent my-key codex --model o4-mini
  ai-switch agent my-key claude --dangerously-skip-permissions`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return runAgent(configPath, args[0], args[1], args[2:])
		},
	}
	return cmd
}

func runAgent(configPath, routeKey, agentName string, agentArgs []string) error {
	envConfig, ok := agentEnvMap[agentName]
	if !ok {
		return fmt.Errorf("unsupported agent %q, supported: %s", agentName, supportedAgents())
	}

	// Look up agent binary
	binary, err := exec.LookPath(agentName)
	if err != nil {
		// Try common binary names
		aliases := map[string]string{
			"claude": "claude",
			"codex":  "codex",
		}
		if alias, found := aliases[agentName]; found {
			binary, err = exec.LookPath(alias)
		}
		if err != nil {
			return fmt.Errorf("agent %q not found in PATH", agentName)
		}
	}

	// Load config to get server address
	resolvedPath, err := config.DefaultConfigPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	host := cfg.Server.Host
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)

	slog.Info("launching agent",
		"agent", agentName,
		"route_key", routeKey,
		"base_url", baseURL,
		"binary", binary,
		"args", agentArgs,
	)

	cmd := exec.Command(binary, agentArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Inherit current env, then set agent-specific vars
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		envConfig.baseURLKey+"="+baseURL,
		envConfig.apiKeyKey+"="+routeKey,
	)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return errExitCode{code: exitErr.ExitCode()}
		}
		return fmt.Errorf("failed to run agent: %w", err)
	}

	return nil
}

func supportedAgents() string {
	names := make([]string, 0, len(agentEnvMap))
	for k := range agentEnvMap {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}
