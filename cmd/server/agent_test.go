package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests ---

func TestAgentEnvMap(t *testing.T) {
	assert.Equal(t, "ANTHROPIC_BASE_URL", agentEnvMap["claude"].baseURLKey)
	assert.Equal(t, "ANTHROPIC_API_KEY", agentEnvMap["claude"].apiKeyKey)
	assert.Equal(t, "", agentEnvMap["claude"].pathSuffix)
	assert.Equal(t, "OPENAI_BASE_URL", agentEnvMap["codex"].baseURLKey)
	assert.Equal(t, "OPENAI_API_KEY", agentEnvMap["codex"].apiKeyKey)
	assert.Equal(t, "/v1", agentEnvMap["codex"].pathSuffix)
}

func TestSupportedAgents(t *testing.T) {
	s := supportedAgents()
	assert.Contains(t, s, "claude")
	assert.Contains(t, s, "codex")
}

func TestNewAgentCmd(t *testing.T) {
	cmd := newAgentCmd("")
	assert.Contains(t, cmd.Use, "agent")
	assert.NotNil(t, cmd.RunE)
}

func TestAgentCmdValidation(t *testing.T) {
	cmd := newAgentCmd("")
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
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

// --- Scenario: config file does not exist → use defaults ---

func TestRunAgent_NoConfig_UsesDefaults(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)

	// Config does NOT exist — should use default host:port
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

	output, err := captureAgentOutput(t, configPath, "key", "codex", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:12345/v1")
}

// --- Scenario: route_key passed as API key ---

func TestRunAgent_RouteKeyAsApiKey(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "API_KEY=$ANTHROPIC_API_KEY"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	output, err := captureAgentOutput(t, configPath, "my-secret-key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "API_KEY=my-secret-key")
}

// --- Scenario: agent args are passed through ---

func TestRunAgent_ArgsPassedThrough(t *testing.T) {
	_, configPath := setupFakeAgent(t, "codex", `echo "ARGS=$*"`)
	writeConfig(t, configPath, "127.0.0.1", 12345)

	output, err := captureAgentOutput(t, configPath, "key", "codex", []string{"--model", "o4-mini", "--full-auto"})
	require.NoError(t, err)
	assert.Contains(t, output, "ARGS=--model o4-mini --full-auto")
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
	err := runAgent("", "key", "copilot", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported agent")
}

// --- Scenario: agent binary not in PATH ---

func TestRunAgent_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	require.NoError(t, os.Setenv("PATH", tmpDir))

	err := runAgent("", "key", "claude", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in PATH")
}

// --- Scenario: pre-existing env vars are overridden ---

func TestRunAgent_EnvOverride(t *testing.T) {
	_, configPath := setupFakeAgent(t, "claude", `echo "BASE_URL=$ANTHROPIC_BASE_URL"`)
	writeConfig(t, configPath, "127.0.0.1", 19999)

	// Set a pre-existing env var that should be overridden
	os.Setenv("ANTHROPIC_BASE_URL", "http://should-be-overridden:9999")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_BASE_URL") })

	output, err := captureAgentOutput(t, configPath, "key", "claude", nil)
	require.NoError(t, err)
	assert.Contains(t, output, "BASE_URL=http://127.0.0.1:19999")
	assert.NotContains(t, output, "should-be-overridden")
}

// --- Helpers ---

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
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runAgent(configPath, routeKey, agentName, args)

	w.Close()
	os.Stdout = old

	var buf strings.Builder
	io.Copy(&buf, r)
	return buf.String(), err
}
