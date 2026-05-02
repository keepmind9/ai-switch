package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentEnvMap(t *testing.T) {
	assert.Equal(t, "ANTHROPIC_BASE_URL", agentEnvMap["claude"].baseURLKey)
	assert.Equal(t, "ANTHROPIC_API_KEY", agentEnvMap["claude"].apiKeyKey)
	assert.Equal(t, "OPENAI_BASE_URL", agentEnvMap["codex"].baseURLKey)
	assert.Equal(t, "OPENAI_API_KEY", agentEnvMap["codex"].apiKeyKey)
}

func TestSupportedAgents(t *testing.T) {
	s := supportedAgents()
	assert.Contains(t, s, "claude")
	assert.Contains(t, s, "codex")
}

func TestRunAgentUnsupported(t *testing.T) {
	err := runAgent("", "test-key", "nonexistent-agent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported agent")
}

func TestRunAgentBinaryNotFound(t *testing.T) {
	// Save and restore PATH
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	// Set PATH to empty dir
	tmpDir := t.TempDir()
	os.Setenv("PATH", tmpDir)

	err := runAgent("", "test-key", "claude", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in PATH")
}

func TestRunAgentWithConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configContent := `
server:
  host: "127.0.0.1"
  port: 19999
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Create a fake claude binary that prints env vars
	fakeBin := filepath.Join(tmpDir, "claude")
	fakeScript := `#!/bin/sh
echo "BASE_URL=$ANTHROPIC_BASE_URL"
echo "API_KEY=$ANTHROPIC_API_KEY"
`
	err = os.WriteFile(fakeBin, []byte(fakeScript), 0755)
	require.NoError(t, err)

	// Save and restore PATH
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", tmpDir)

	// Run agent
	err = runAgent(configPath, "my-route-key", "claude", nil)
	require.NoError(t, err)
}

func TestRunAgentPassesArgs(t *testing.T) {
	tmpDir := t.TempDir()
	configContent := `
server:
  host: "127.0.0.1"
  port: 19999
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte(configContent), 0644)

	// Create a fake codex binary that prints its args
	fakeBin := filepath.Join(tmpDir, "codex")
	fakeScript := `#!/bin/sh
echo "ARGS=$*"
echo "BASE_URL=$OPENAI_BASE_URL"
echo "API_KEY=$OPENAI_API_KEY"
`
	os.WriteFile(fakeBin, []byte(fakeScript), 0755)

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", tmpDir)

	err := runAgent(configPath, "test-key", "codex", []string{"--model", "o4-mini"})
	require.NoError(t, err)
}

func TestRunAgentExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	configContent := `
server:
  host: "127.0.0.1"
  port: 19999
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte(configContent), 0644)

	// Create a fake claude binary that exits with code 42
	fakeBin := filepath.Join(tmpDir, "claude")
	fakeScript := `#!/bin/sh
exit 42
`
	os.WriteFile(fakeBin, []byte(fakeScript), 0755)

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })
	os.Setenv("PATH", tmpDir)

	err := runAgent(configPath, "test-key", "claude", nil)
	require.Error(t, err)
	var eec errExitCode
	require.ErrorAs(t, err, &eec)
	assert.Equal(t, 42, eec.code)
}

func TestNewAgentCmd(t *testing.T) {
	cmd := newAgentCmd("")
	assert.Contains(t, cmd.Use, "agent")
	assert.NotNil(t, cmd.RunE)
}

func TestAgentCmdValidation(t *testing.T) {
	cmd := newAgentCmd("")
	// No args should fail
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
}
