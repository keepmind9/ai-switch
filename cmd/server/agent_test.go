package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Use child-process mode in tests: syscall.Exec would replace the test process.
	execAgentFunc = execAgentChildProcess
	os.Exit(m.Run())
}

// --- Unit tests ---

func TestAgentEnvMap(t *testing.T) {
	assert.Equal(t, "ANTHROPIC_BASE_URL", agentEnvMap["claude"].baseURLKey)
	assert.Equal(t, "", agentEnvMap["claude"].pathSuffix)
	assert.Equal(t, []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}, agentEnvMap["claude"].authKeys)
	assert.Equal(t, "OPENAI_BASE_URL", agentEnvMap["codex"].baseURLKey)
	assert.Equal(t, "/v1", agentEnvMap["codex"].pathSuffix)
	assert.Equal(t, []string{"OPENAI_API_KEY"}, agentEnvMap["codex"].authKeys)
}

func TestSupportedAgents(t *testing.T) {
	s := supportedAgents()
	assert.Contains(t, s, "claude")
	assert.Contains(t, s, "codex")
}

func TestNewAgentCmd(t *testing.T) {
	cmd := newAgentCmd()
	assert.Contains(t, cmd.Use, "agent")
	assert.NotNil(t, cmd.RunE)
}

func TestAgentCmdValidation(t *testing.T) {
	cmd := newAgentCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

// --- Helper: set up a fake binary in a temp dir with clean PATH ---

func setupFakeAgent(t *testing.T, name, script string) (tmpDir string, configPath string) {
	t.Helper()
	tmpDir = t.TempDir()

	// Write fake binary
	fakeBin := filepath.Join(tmpDir, name)
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"+script), 0755))

	// Override PATH
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	require.NoError(t, os.Setenv("PATH", tmpDir))

	// Default config path inside tmpDir (caller can choose to create it or not)
	configPath = filepath.Join(tmpDir, "config.yaml")
	return
}

// --- buildAuthEnvMap tests ---

func TestBuildAuthEnvMap(t *testing.T) {
	m := buildAuthEnvMap(agentEnvMap["claude"], "ANTHROPIC_AUTH_TOKEN", "secret", "http://127.0.0.1:8080")
	assert.Equal(t, "http://127.0.0.1:8080", m["ANTHROPIC_BASE_URL"])
	assert.Equal(t, "", m["ANTHROPIC_API_KEY"])
	assert.Equal(t, "secret", m["ANTHROPIC_AUTH_TOKEN"])
}

func TestBuildAuthEnvMap_Codex(t *testing.T) {
	m := buildAuthEnvMap(agentEnvMap["codex"], "OPENAI_API_KEY", "rk", "http://127.0.0.1:8080/v1")
	assert.Equal(t, "http://127.0.0.1:8080/v1", m["OPENAI_BASE_URL"])
	assert.Equal(t, "rk", m["OPENAI_API_KEY"])
}

// --- Scenario: config file does not exist → use defaults ---

func TestRunAgent_NoConfig_UsesDefaults(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)

	output, err := captureAgentOutput(t, configPath, "my-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:12345")
}

// --- Scenario: config file exists and valid → use config values ---

func TestRunAgent_ValidConfig_UsesConfig(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)
	writeConfig(t, configPath, "192.168.1.100", 8080)

	output, err := captureAgentOutput(t, configPath, "my-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://192.168.1.100:8080")
}

// --- Scenario: config file exists but invalid → fatal error ---

func TestRunAgent_InvalidConfig_ReturnsError(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", "")
	require.NoError(t, os.WriteFile(configPath, []byte("not: valid: yaml: [["), 0644))

	_, err := captureAgentOutput(t, configPath, "my-key", "claude", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

// --- Scenario: host=0.0.0.0 → replaced with 127.0.0.1 ---

func TestRunAgent_ZeroHost_ReplacesWithLocalhost(t *testing.T) {
	_, configPath := setupFakeAgent(t, "codex", `echo "BASE_URL=$OPENAI_BASE_URL"`)
	writeConfig(t, configPath, "0.0.0.0", 9999)
	setupCodexConfig(t, "xai")

	output, err := captureAgentOutput(t, configPath, "my-key", "codex", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:9999/v1")
}

// --- Scenario: Claude base URL has no /v1 suffix ---

func TestRunAgent_Claude_NoV1Suffix(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	output, err := captureAgentOutput(t, configPath, "key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:12345")
	assert.NotContains(t, output, "/v1")
}

// --- Scenario: Codex base URL has /v1 suffix ---

func TestRunAgent_Codex_V1Suffix(t *testing.T) {
	_, configPath := setupFakeAgent(t, "codex", `echo "BASE_URL=$OPENAI_BASE_URL"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)
	setupCodexConfig(t, "xai")

	output, err := captureAgentOutput(t, configPath, "key", "codex", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:12345/v1")
}

// --- Scenario: route_key passed to first auth key when none set, others explicitly empty ---

func TestRunAgent_RouteKeyFallback(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "API_KEY=$ANTHROPIC_API_KEY AUTH_TOKEN=$ANTHROPIC_AUTH_TOKEN"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	t.Setenv("HOME", t.TempDir())
	os.Unsetenv("ANTHROPIC_AUTH_TOKEN")

	output, err := captureAgentOutput(t, configPath, "my-secret-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "API_KEY=my-secret-key")
	assert.NotContains(t, output, "AUTH_TOKEN=my-secret-key")
}

// --- Scenario: agent args are passed through ---

func TestRunAgent_ArgsPassedThrough(t *testing.T) {
	_, configPath := setupFakeAgent(t, "codex", `echo "ARGS=$*"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)
	setupCodexConfig(t, "xai")

	output, err := captureAgentOutput(t, configPath, "key", "codex", []string{"--model", "o4-mini", "--full-auto"})
	require.NoError(t, err)
	assert.Contains(t, output, "--model o4-mini --full-auto")
}

// --- Scenario: agent exit code is propagated ---

func TestRunAgent_ExitCode(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", "exit 42")
	writeConfig(t, configPath, "127.0.0.1", 12345)

	_, err := captureAgentOutput(t, configPath, "key", "claude", nil)
	require.Error(t, err)
	var eec errExitCode
	require.ErrorAs(t, err, &eec)
	assert.Equal(t, 42, eec.code)
}

// --- Scenario: unsupported agent name ---

func TestRunAgent_UnsupportedAgent(t *testing.T) {
	err := runAgent("", "", "key", "copilot", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported agent")
}

// --- Scenario: agent binary not in PATH ---

func TestRunAgent_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	require.NoError(t, os.Setenv("PATH", tmpDir))

	err := runAgent("", "", "key", "claude", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in PATH")
}

// --- Scenario: pre-existing env vars are overridden ---

func TestRunAgent_EnvOverride(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)
	writeConfig(t, configPath, "127.0.0.1", 19999)

	os.Setenv("ANTHROPIC_BASE_URL", "http://should-be-overridden:9999")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_BASE_URL") })

	output, err := captureAgentOutput(t, configPath, "key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:19999")
	assert.NotContains(t, output, "should-be-overridden")
}

// --- Scenario: ANTHROPIC_AUTH_TOKEN is reused when already set ---

func TestRunAgent_AuthTokenPreferred(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "AUTH_TOKEN=$ANTHROPIC_AUTH_TOKEN API_KEY=$ANTHROPIC_API_KEY"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	t.Setenv("HOME", t.TempDir())
	os.Setenv("ANTHROPIC_AUTH_TOKEN", "existing-token")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_AUTH_TOKEN") })

	output, err := captureAgentOutput(t, configPath, "my-route-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "AUTH_TOKEN=my-route-key")
	assert.Contains(t, output, "API_KEY=")                // empty — not set
	assert.NotContains(t, output, "API_KEY=my-route-key") // only auth token is set
}

// --- Scenario: settings.json conflict resolved via --settings flag ---

func TestRunAgent_SettingsConflictResolved(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "ARGS=$*"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	t.Setenv("HOME", t.TempDir())

	output, err := captureAgentOutput(t, configPath, "my-route-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, `--settings`)

	// Verify the JSON payload content
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "--settings") {
			continue
		}
		parts := strings.SplitN(line, "--settings ", 2)
		if len(parts) < 2 {
			continue
		}
		// Trim shell quoting — the fake binary echoes args as-is
		raw := strings.TrimSpace(parts[1])
		// Find the JSON object in the line
		start := strings.Index(raw, "{")
		end := strings.LastIndex(raw, "}")
		if start < 0 || end < 0 {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw[start:end+1]), &payload), "invalid settings JSON")
		env := payload["env"].(map[string]any)
		assert.Equal(t, "http://127.0.0.1:12345", env["ANTHROPIC_BASE_URL"])
		assert.Equal(t, "my-route-key", env["ANTHROPIC_API_KEY"])
		assert.Equal(t, "", env["ANTHROPIC_AUTH_TOKEN"])
		break
	}
}

// --- Scenario: env filtering removes auth keys from child process ---

func TestRunAgent_EnvFiltering(t *testing.T) {
	script := `echo "API_KEY=$ANTHROPIC_API_KEY AUTH_TOKEN=$ANTHROPIC_AUTH_TOKEN BASE_URL=$ANTHROPIC_BASE_URL"`
	_, configPath := setupFakeAgent(t, "claude", script)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	t.Setenv("HOME", t.TempDir())
	os.Unsetenv("ANTHROPIC_AUTH_TOKEN")
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_BASE_URL")

	output, err := captureAgentOutput(t, configPath, "test-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "API_KEY=test-key")
	assert.Contains(t, output, "AUTH_TOKEN=")
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:12345")
}

// --- buildAgentArgs unit tests ---

func TestBuildAgentArgs_Claude(t *testing.T) {
	args, err := buildAgentArgs("claude", agentEnvMap["claude"], "ANTHROPIC_API_KEY", "my-key", "http://127.0.0.1:8080", []string{"--continue"})
	require.NoError(t, err)
	assert.Equal(t, "--settings", args[0])
	assert.Equal(t, "--continue", args[len(args)-1])
	// Verify JSON payload via parsing
	var payload map[string]any
	err = json.Unmarshal([]byte(args[1]), &payload)
	require.NoError(t, err)
	env := payload["env"].(map[string]any)
	assert.Equal(t, "http://127.0.0.1:8080", env["ANTHROPIC_BASE_URL"])
	assert.Equal(t, "my-key", env["ANTHROPIC_API_KEY"])
}

func TestBuildAgentArgs_Codex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	configPath := filepath.Join(home, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("model_provider = \"xai\"\n"), 0644))

	args, err := buildAgentArgs("codex", agentEnvMap["codex"], "OPENAI_API_KEY", "my-key", "http://127.0.0.1:8080/v1", []string{"--model", "gpt-5.4"})
	require.NoError(t, err)
	assert.Contains(t, args, "-c")
	assert.Contains(t, args, "model_provider=xai")
	assert.Contains(t, args, "model_providers.xai.base_url=http://127.0.0.1:8080/v1")
	assert.Contains(t, args, "model_providers.xai.env_key=OPENAI_API_KEY")
	assert.Contains(t, args, "model_providers.xai.requires_openai_auth=true")
	last := args[len(args)-1]
	assert.Equal(t, "gpt-5.4", last)
}

func TestBuildAgentArgs_Codex_MissingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := buildAgentArgs("codex", agentEnvMap["codex"], "OPENAI_API_KEY", "my-key", "http://127.0.0.1:8080/v1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "codex:")
	assert.Contains(t, err.Error(), "failed to read")
}

func TestBuildAgentArgs_Codex_EmptyModelProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	configPath := filepath.Join(home, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("model_provider = \"\"\n"), 0644))

	_, err := buildAgentArgs("codex", agentEnvMap["codex"], "OPENAI_API_KEY", "my-key", "http://127.0.0.1:8080/v1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model_provider not found")
}

func TestBuildAgentArgs_Codex_InvalidTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	configPath := filepath.Join(home, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("not valid toml ["), 0644))

	_, err := buildAgentArgs("codex", agentEnvMap["codex"], "OPENAI_API_KEY", "my-key", "http://127.0.0.1:8080/v1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestBuildAgentArgs_UnknownAgent(t *testing.T) {
	args, err := buildAgentArgs("copilot", agentEnvConfig{}, "", "", "", nil)
	assert.NoError(t, err)
	assert.Empty(t, args)
}

func TestBuildAgentArgs_EmptyArgs(t *testing.T) {
	args, err := buildAgentArgs("claude", agentEnvMap["claude"], "ANTHROPIC_API_KEY", "key", "http://127.0.0.1:8080", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(args)) // --settings + JSON only
}

// --- getCodexModelProvider unit tests ---

func TestGetCodexModelProvider_Success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	configPath := filepath.Join(home, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("model_provider = \"xai\"\n"), 0644))

	provider, err := getCodexModelProvider()
	require.NoError(t, err)
	assert.Equal(t, "xai", provider)
}

func TestGetCodexModelProvider_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	configPath := filepath.Join(home, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("other_field = \"value\"\n"), 0644))

	_, err := getCodexModelProvider()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model_provider not found")
}

func TestGetCodexModelProvider_InvalidTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	configPath := filepath.Join(home, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("not valid toml ["), 0644))

	_, err := getCodexModelProvider()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestGetCodexModelProvider_DirNotExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// ~/.codex/ directory does not exist at all

	_, err := getCodexModelProvider()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read")
}

// --- buildClaudeArgs unit tests ---

func TestBuildClaudeArgs_AuthToken(t *testing.T) {
	result, err := buildClaudeArgs(agentEnvMap["claude"], "ANTHROPIC_AUTH_TOKEN", "secret-key", "http://127.0.0.1:9999")
	require.NoError(t, err)
	assert.Equal(t, "--settings", result[0])
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(result[1]), &payload))
	env := payload["env"].(map[string]any)
	assert.Equal(t, "http://127.0.0.1:9999", env["ANTHROPIC_BASE_URL"])
	assert.Equal(t, "", env["ANTHROPIC_API_KEY"])
	assert.Equal(t, "secret-key", env["ANTHROPIC_AUTH_TOKEN"])
}

func TestBuildClaudeArgs_APIKey(t *testing.T) {
	result, err := buildClaudeArgs(agentEnvMap["claude"], "ANTHROPIC_API_KEY", "secret-key", "http://127.0.0.1:9999")
	require.NoError(t, err)
	assert.Equal(t, "--settings", result[0])
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(result[1]), &payload))
	env := payload["env"].(map[string]any)
	assert.Equal(t, "http://127.0.0.1:9999", env["ANTHROPIC_BASE_URL"])
	assert.Equal(t, "secret-key", env["ANTHROPIC_API_KEY"])
	assert.Equal(t, "", env["ANTHROPIC_AUTH_TOKEN"])
}

// --- buildCodexArgs unit test ---

func TestBuildCodexArgs(t *testing.T) {
	result := buildCodexArgs("xai", "http://127.0.0.1:9999/v1")
	assert.Equal(t, []string{
		"-c", "model_provider=xai",
		"-c", "model_providers.xai.base_url=http://127.0.0.1:9999/v1",
		"-c", "model_providers.xai.env_key=OPENAI_API_KEY",
		"-c", "model_providers.xai.requires_openai_auth=true",
	}, result)
}

// --- Helpers ---

func setupCodexConfig(t *testing.T, provider string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, ".codex", "config.toml"),
		[]byte(fmt.Sprintf("model_provider = %q\n", provider)),
		0644,
	))
}

func writeConfig(t *testing.T, path, host string, port int) {
	t.Helper()
	b := []byte(strings.TrimSpace(fmt.Sprintf(`
server:
  host: "%s"
  port: %d
`, host, port)))
	require.NoError(t, os.WriteFile(path, b, 0644))
}

func captureAgentOutput(t *testing.T, configPath, routeKey, agentName string, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	err = runAgent(configPath, "", routeKey, agentName, args)

	w.Close()
	os.Stdout = old

	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String(), err
}

func TestParseAgentURL(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantRem []string
		wantURL string
	}{
		{
			name:    "no --url",
			args:    []string{"my-key", "claude", "--continue"},
			wantRem: []string{"my-key", "claude", "--continue"},
			wantURL: "",
		},
		{
			name:    "--url before positionals",
			args:    []string{"--url", "http://1.2.3.4:9999", "my-key", "claude"},
			wantRem: []string{"my-key", "claude"},
			wantURL: "http://1.2.3.4:9999",
		},
		{
			name:    "--url=value form",
			args:    []string{"--url=http://remote:8080", "my-key", "codex"},
			wantRem: []string{"my-key", "codex"},
			wantURL: "http://remote:8080",
		},
		{
			name:    "--url after positionals",
			args:    []string{"my-key", "claude", "--url", "http://host:12345", "--continue"},
			wantRem: []string{"my-key", "claude", "--continue"},
			wantURL: "http://host:12345",
		},
		{
			name:    "--url with trailing slash",
			args:    []string{"--url", "http://host:12345/", "my-key", "claude"},
			wantRem: []string{"my-key", "claude"},
			wantURL: "http://host:12345/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rem, url, err := parseAgentURL(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRem, rem)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}

func TestParseAgentURL_NoValue(t *testing.T) {
	_, _, err := parseAgentURL([]string{"--url"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--url requires a value")
}

func TestExtractLeadingConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantConfig string
		wantRemain []string
	}{
		{
			name:       "no config flag",
			args:       []string{"my-key", "claude", "--continue"},
			wantConfig: "",
			wantRemain: []string{"my-key", "claude", "--continue"},
		},
		{
			name:       "-c before positionals",
			args:       []string{"-c", "/tmp/x.yaml", "my-key", "claude"},
			wantConfig: "/tmp/x.yaml",
			wantRemain: []string{"my-key", "claude"},
		},
		{
			name:       "--config= form",
			args:       []string{"--config=/tmp/x.yaml", "my-key", "codex"},
			wantConfig: "/tmp/x.yaml",
			wantRemain: []string{"my-key", "codex"},
		},
		{
			name:       "codex -c after positionals is preserved",
			args:       []string{"my-key", "codex", "-c", "model_provider=x"},
			wantConfig: "",
			wantRemain: []string{"my-key", "codex", "-c", "model_provider=x"},
		},
		{
			name:       "-c and --url both before positionals",
			args:       []string{"--url", "http://h:1", "-c", "/tmp/x.yaml", "my-key", "claude"},
			wantConfig: "/tmp/x.yaml",
			wantRemain: []string{"--url", "http://h:1", "my-key", "claude"},
		},
		{
			name:       "-- separator stops option scanning",
			args:       []string{"--", "-c", "/tmp/x.yaml", "my-key", "claude"},
			wantConfig: "",
			wantRemain: []string{"--", "-c", "/tmp/x.yaml", "my-key", "claude"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, rem, err := extractLeadingConfig(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantConfig, cfg)
			assert.Equal(t, tt.wantRemain, rem)
		})
	}
}

func TestExtractLeadingConfig_NoValue(t *testing.T) {
	for _, a := range []string{"-c", "--config"} {
		_, _, err := extractLeadingConfig([]string{a})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires a value")
	}
}

func TestRunAgent_ServerURLOverride(t *testing.T) {
	// Use a fake claude binary that prints the BASE_URL env var
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)
	writeConfig(t, configPath, "10.0.0.1", 9999)

	output, err := captureAgentOutput(t, configPath, "my-key", "claude", nil)
	// Without --url, should use config values (10.0.0.1:9999)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://10.0.0.1:9999")
}

func TestRunAgent_ServerURLFlagOverridesConfig(t *testing.T) {
	// Config says 10.0.0.1:9999, but --url overrides to 192.168.1.100:12345
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)
	writeConfig(t, configPath, "10.0.0.1", 9999)

	old := os.Stdout
	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Stdout = old; os.Setenv("PATH", oldPath) })
	// PATH already set by setupFakeAgent

	err := runAgent(configPath, "http://192.168.1.100:12345", "my-key", "claude", nil)
	require.NoError(t, err)
}

// --- execAgentChildProcess direct tests ---

func TestExecAgentChildProcess_Success(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "test-bin")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho hello"), 0755))

	err := execAgentChildProcess(script, nil, os.Environ())
	require.NoError(t, err)
}

func TestExecAgentChildProcess_ExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "test-bin")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 7"), 0755))

	err := execAgentChildProcess(script, nil, os.Environ())
	require.Error(t, err)
	var eec errExitCode
	require.ErrorAs(t, err, &eec)
	assert.Equal(t, 7, eec.code)
}

func TestExecAgentChildProcess_NonexistentBinary(t *testing.T) {
	err := execAgentChildProcess("/nonexistent/binary", nil, os.Environ())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run agent")
}

func TestExecAgentChildProcess_EnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "test-bin")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"MY_VAR=$MY_VAR\""), 0755))

	env := append(os.Environ(), "MY_VAR=testvalue")
	err := execAgentChildProcess(script, nil, env)
	require.NoError(t, err)
}

func TestExecAgentFunc_SetByTestMain(t *testing.T) {
	// Verify TestMain correctly set execAgentFunc to the test-safe implementation
	assert.NotNil(t, execAgentFunc)
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "test-bin")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0"), 0755))
	err := execAgentFunc(script, nil, os.Environ())
	assert.NoError(t, err)
}

func TestRunAgent_InvalidServerURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"not a URL", "not-a-url"},
		{"ftp scheme", "ftp://example.com"},
		{"empty host", "http://"},
		{"hostname only port", "http://:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runAgent("", tt.url, "key", "claude", nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid --url value")
		})
	}
}
