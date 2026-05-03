package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/spf13/cobra"
)

// agentEnvMap maps agent names to their environment variable names.
var agentEnvMap = map[string]struct {
	baseURLKey string
	apiKeyKey  string
	pathSuffix string
	authKeys   []string
}{
	"claude": {
		baseURLKey: "ANTHROPIC_BASE_URL",
		apiKeyKey:  "ANTHROPIC_API_KEY",
		pathSuffix: "",
		authKeys:   []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"},
	},
	"codex": {
		baseURLKey: "OPENAI_BASE_URL",
		apiKeyKey:  "OPENAI_API_KEY",
		pathSuffix: "/v1",
		authKeys:   []string{"OPENAI_API_KEY"},
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
		return fmt.Errorf("agent %q not found in PATH", agentName)
	}

	// Resolve server address: config file > defaults; load failure is fatal
	host := config.DefaultHost
	port := config.DefaultPort

	resolvedPath, _ := config.DefaultConfigPath(configPath)
	if resolvedPath != "" {
		if _, statErr := os.Stat(resolvedPath); statErr == nil {
			// Config file exists — load it, failure is fatal
			cfg, err := config.Load(resolvedPath)
			if err != nil {
				return fmt.Errorf("failed to load config from %s: %w", resolvedPath, err)
			}
			host = cfg.Server.Host
			port = cfg.Server.Port
		}
		// Config file does not exist — use defaults silently
	}

	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%d%s", host, port, envConfig.pathSuffix)

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

	// Inherit current env, filtering out all auth-related keys to avoid duplicates
	overrideKeys := map[string]bool{envConfig.baseURLKey: true}
	for _, k := range envConfig.authKeys {
		overrideKeys[k] = true
	}

	// Use whichever auth key is already set in the environment, fallback to default
	authKey := envConfig.apiKeyKey
	for _, k := range envConfig.authKeys {
		if os.Getenv(k) != "" {
			authKey = k
			break
		}
	}

	baseEnv := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !overrideKeys[k] {
			baseEnv = append(baseEnv, e)
		}
	}
	cmd.Env = append(baseEnv,
		envConfig.baseURLKey+"="+baseURL,
		authKey+"="+routeKey,
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
	sort.Strings(names)
	return strings.Join(names, ", ")
}
