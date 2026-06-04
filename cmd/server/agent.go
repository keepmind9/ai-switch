package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

// agentEnvConfig holds environment variable names for an agent.
type agentEnvConfig struct {
	baseURLKey string   // env var name for base URL (e.g. "ANTHROPIC_BASE_URL")
	pathSuffix string   // URL path suffix (e.g. "/v1" for Codex)
	authKeys   []string // all known auth env var names for this agent
}

// agentEnvMap maps agent names to their environment variable names.
var agentEnvMap = map[string]agentEnvConfig{
	"claude": {
		baseURLKey: "ANTHROPIC_BASE_URL",
		pathSuffix: "",
		authKeys:   []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"},
	},
	"codex": {
		baseURLKey: "OPENAI_BASE_URL",
		pathSuffix: "/v1",
		authKeys:   []string{"OPENAI_API_KEY"},
	},
}

func newAgentCmd(configPath string) *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "agent [--url <server_url>] <route_key> <agent> [agent_args...]",
		Short: fmt.Sprintf("Launch an AI agent with %s config", binName),
		Long: fmt.Sprintf(`Launch an AI agent (claude/codex) with environment variables
automatically configured from a route key.

The route_key is used as the API key for the agent, and the base URL
is set to the %s server address from the config file.

Use --url to override the server address (e.g. connect to a remote server).

Examples:
  %s agent my-key claude --continue
  %s agent my-key codex --model o4-mini
  %s agent --url http://192.168.1.100:12345 my-key claude
  %s agent --url http://remote:12345 my-key claude --dangerously-skip-permissions`, binName, binName, binName, binName, binName),
		Args:               cobra.MinimumNArgs(2),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle help manually since DisableFlagParsing bypasses cobra's -h/--help
			for _, a := range args {
				if a == "-h" || a == "--help" {
					return cmd.Help()
				}
			}
			// Manually extract --url from args before positionals
			filtered, parsedURL, err := parseAgentURL(args)
			if err != nil {
				return err
			}
			if len(filtered) < 2 {
				return fmt.Errorf("requires at least 2 arguments: <route_key> <agent>")
			}
			return runAgent(configPath, parsedURL, filtered[0], filtered[1], filtered[2:])
		},
	}
	// Stored on cmd for documentation only; actual parsing is manual.
	cmd.Flags().StringVar(&serverURL, "url", "", "Override server URL (e.g. http://host:port)")
	_ = serverURL
	return cmd
}

// parseAgentURL scans args for --url <value> or --url=<value>, removes them,
// and returns the remaining args and the parsed URL (empty string if not set).
// Returns an error if --url is provided without a value.
func parseAgentURL(args []string) (remaining []string, url string, err error) {
	remaining = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--url" {
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("--url requires a value")
			}
			url = args[i+1]
			i++ // skip value
			continue
		}
		if strings.HasPrefix(args[i], "--url=") {
			url = strings.TrimPrefix(args[i], "--url=")
			continue
		}
		remaining = append(remaining, args[i])
	}
	return remaining, url, nil
}

// getCodexModelProvider reads model_provider from the codex config file.
func getCodexModelProvider() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	var doc struct {
		ModelProvider string `toml:"model_provider"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", configPath, err)
	}
	if doc.ModelProvider == "" {
		return "", fmt.Errorf("model_provider not found in %s", configPath)
	}
	return doc.ModelProvider, nil
}

// buildAuthEnvMap returns a map of env var names to values: base URL is set,
// the chosen authKey gets routeKey, and all other auth keys are set to empty.
func buildAuthEnvMap(envConfig agentEnvConfig, authKey, routeKey, baseURL string) map[string]string {
	m := map[string]string{envConfig.baseURLKey: baseURL}
	for _, k := range envConfig.authKeys {
		if k == authKey {
			m[k] = routeKey
		} else {
			m[k] = ""
		}
	}
	return m
}

func runAgent(configPath, serverURL, routeKey, agentName string, agentArgs []string) error {
	envConfig, ok := agentEnvMap[agentName]
	if !ok {
		return fmt.Errorf("unsupported agent %q, supported: %s", agentName, supportedAgents())
	}
	if len(envConfig.authKeys) == 0 {
		return fmt.Errorf("agent %q has no auth keys configured", agentName)
	}

	// Validate --url early, before expensive operations like binary lookup
	if serverURL != "" {
		parsed, err := url.Parse(serverURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return fmt.Errorf("invalid --url value %q: must be a valid http/https URL", serverURL)
		}
	}

	// Look up agent binary
	binary, err := exec.LookPath(agentName)
	if err != nil {
		return fmt.Errorf("agent %q not found in PATH", agentName)
	}

	// Resolve server address: --url flag > config file > defaults
	var baseURL string
	if serverURL != "" {
		baseURL = strings.TrimRight(serverURL, "/") + envConfig.pathSuffix
	} else {
		host := config.DefaultHost
		port := config.DefaultPort

		resolvedPath, _ := config.DefaultConfigPath(configPath)
		if resolvedPath != "" {
			if _, statErr := os.Stat(resolvedPath); statErr == nil {
				cfg, err := config.Load(resolvedPath)
				if err != nil {
					return fmt.Errorf("failed to load config from %s: %w", resolvedPath, err)
				}
				host = cfg.Server.Host
				port = cfg.Server.Port
			}
		}

		if host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		baseURL = fmt.Sprintf("http://%s:%d%s", host, port, envConfig.pathSuffix)
	}

	// Pick auth key: prefer one already set in env, fallback to first in list.
	authKey := envConfig.authKeys[0]
	for _, k := range envConfig.authKeys {
		if os.Getenv(k) != "" {
			authKey = k
			break
		}
	}

	slog.Info("launching agent",
		"agent", agentName,
		"route_key", routeKey,
		"base_url", baseURL,
		"auth_key", authKey,
		"binary", binary,
		"args", agentArgs,
	)

	// Build command args: prepend --settings JSON if agent supports it
	finalArgs, err := buildAgentArgs(agentName, envConfig, authKey, routeKey, baseURL, agentArgs)
	if err != nil {
		return err
	}
	// Build environment: inherit current env, filtering out all auth and base URL keys
	filterKeys := map[string]bool{envConfig.baseURLKey: true}
	for _, k := range envConfig.authKeys {
		filterKeys[k] = true
	}

	baseEnv := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !filterKeys[k] {
			baseEnv = append(baseEnv, e)
		}
	}

	envMap := buildAuthEnvMap(envConfig, authKey, routeKey, baseURL)
	envOverrides := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envOverrides = append(envOverrides, k+"="+v)
	}

	return execAgentFunc(binary, finalArgs, append(baseEnv, envOverrides...))
}

// buildAgentArgs prepends config override arguments so the agent uses the
// route_key for routing in ai-switch, not the key from its own config files.
// Claude uses --settings (see buildClaudeArgs), Codex uses -c (see buildCodexArgs).
func buildAgentArgs(agentName string, envConfig agentEnvConfig, authKey, routeKey, baseURL string, agentArgs []string) ([]string, error) {
	args := []string{}

	if agentName == "claude" {
		claudeArgs, err := buildClaudeArgs(envConfig, authKey, routeKey, baseURL)
		if err != nil {
			return nil, err
		}
		args = append(args, claudeArgs...)
	} else if agentName == "codex" {
		providerName, err := getCodexModelProvider()
		if err != nil {
			return nil, fmt.Errorf("codex: %w", err)
		}
		args = append(args, buildCodexArgs(providerName, baseURL)...)
	}

	return append(args, agentArgs...), nil
}

// buildClaudeArgs uses --settings to override ~/.claude/settings.json.
//
// Problem: Claude reads ~/.claude/settings.json and injects env vars (e.g.
// ANTHROPIC_AUTH_TOKEN or ANTHROPIC_API_KEY). When both env vars are set,
// Claude warns "both token and API key set" and may not use our route_key.
// Solution: --settings '{"env":{"ANTHROPIC_BASE_URL":"...","ANTHROPIC_API_KEY":"<route_key>"}}'
// takes precedence over settings.json, eliminating the warning.
func buildClaudeArgs(envConfig agentEnvConfig, authKey, routeKey, baseURL string) ([]string, error) {
	env := buildAuthEnvMap(envConfig, authKey, routeKey, baseURL)
	settingsJSON, err := json.Marshal(map[string]interface{}{"env": env})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}
	return []string{"--settings", string(settingsJSON)}, nil
}

// buildCodexArgs uses -c to override ~/.codex/config.toml.
//
// Problem: When requires_openai_auth=false, Codex sends no Authorization header.
// ai-switch's extractClientAPIKey reads Authorization: Bearer or x-api-key,
// so routing fails (falls back to default route).
// Solution: Three -c flags are needed:
//   - model_providers.<name>.base_url: points codex at ai-switch
//   - model_providers.<name>.env_key=OPENAI_API_KEY: tells codex which env var to read
//   - model_providers.<name>.requires_openai_auth=true: enables Authorization: Bearer header
//
// The actual key value is passed via the OPENAI_API_KEY env var (set in runAgent).
func buildCodexArgs(providerName, baseURL string) []string {
	baseURLPath := "model_providers." + providerName + ".base_url"
	return []string{
		"-c", "model_provider=" + providerName,
		"-c", baseURLPath + "=" + baseURL,
		"-c", "model_providers." + providerName + ".env_key=OPENAI_API_KEY",
		"-c", "model_providers." + providerName + ".requires_openai_auth=true",
	}
}

func supportedAgents() string {
	names := make([]string, 0, len(agentEnvMap))
	for k := range agentEnvMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
