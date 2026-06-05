package admincli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAdminURL_ExplicitURL(t *testing.T) {
	tests := []struct {
		name    string
		urlFlag string
		want    string
		wantErr bool
	}{
		{"http", "http://192.168.1.100:12345", "http://192.168.1.100:12345/api", false},
		{"https", "https://remote.example.com", "https://remote.example.com/api", false},
		{"trailing slash", "http://localhost:8080/", "http://localhost:8080/api", false},
		{"empty string", "", "", false},
		{"no scheme", "just-a-host", "", true},
		{"ftp scheme", "ftp://host", "", true},
		{"no host", "http://", "", true},
		{"no host with port", "http://:8080", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.urlFlag == "" {
				return // empty urlFlag falls through to config resolution, tested separately with isolated paths
			}
			got, err := ResolveAdminURL("", tt.urlFlag)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveAdminURL_DefaultConfig(t *testing.T) {
	// Use non-existent path to ensure defaults kick in
	got, err := ResolveAdminURL("/nonexistent/__test_config__.yaml", "")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:12345/api", got)
}

func TestResolveAdminURL_FromConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte("server:\n  host: 0.0.0.0\n  port: 9999\n"), 0644)
	require.NoError(t, err)

	got, err := ResolveAdminURL(cfgPath, "")
	require.NoError(t, err)
	// 0.0.0.0 should be replaced with 127.0.0.1
	assert.Equal(t, "http://127.0.0.1:9999/api", got)
}

func TestResolveAdminURL_ConfigFileBindHost(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte("server:\n  host: 192.168.1.50\n  port: 8080\n"), 0644)
	require.NoError(t, err)

	got, err := ResolveAdminURL(cfgPath, "")
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.50:8080/api", got)
}

func TestResolveAdminURL_ConfigNotFound(t *testing.T) {
	// Non-existent config file should fall back to defaults
	got, err := ResolveAdminURL("/nonexistent/config.yaml", "")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:12345/api", got)
}

func TestResolveAdminURL_URLOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte("server:\n  host: 10.0.0.1\n  port: 9999\n"), 0644)
	require.NoError(t, err)

	got, err := ResolveAdminURL(cfgPath, "http://override:5555")
	require.NoError(t, err)
	assert.Equal(t, "http://override:5555/api", got)
}

func TestResolveAdminURL_InvalidConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte("not: valid: yaml: [broken"), 0644)
	require.NoError(t, err)

	_, err = ResolveAdminURL(cfgPath, "")
	assert.Error(t, err)
}

func init() {
	// Ensure config defaults are set
	_ = config.DefaultHost
	_ = config.DefaultPort
}
