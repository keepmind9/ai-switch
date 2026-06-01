package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{
		Server:       ServerConfig{Host: "0.0.0.0", Port: 12345},
		DefaultRoute: "gw-test",
		Providers: map[string]ProviderConfig{
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.minimaxi.com",
				APIKey:  "sk-test-key",
				Format:  "chat",
			},
		},
		Routes: map[string]RouteRule{
			"gw-test": {
				Provider:     "minimax",
				DefaultModel: "MiniMax-M2.7",
				SceneMap:     map[string]string{"default": "MiniMax-M2.7"},
				ModelMap:     map[string]string{"gpt-4o": "MiniMax-M2.7"},
			},
		},
	}

	err := WriteConfig(path, original)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "minimax")
	assert.Contains(t, string(data), "gw-test")

	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, original.Server.Host, loaded.Server.Host)
	assert.Equal(t, original.Server.Port, loaded.Server.Port)
	assert.Equal(t, original.DefaultRoute, loaded.DefaultRoute)
	assert.Len(t, loaded.Providers, 1)
	assert.Equal(t, "MiniMax", loaded.Providers["minimax"].Name)
	assert.Len(t, loaded.Routes, 1)
	assert.Equal(t, "minimax", loaded.Routes["gw-test"].Provider)
}

func TestWriteConfig_CreatesBackupOnOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// First write seeds the file.
	cfg1 := &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}
	require.NoError(t, WriteConfig(path, cfg1))

	// Second write should produce exactly one backup.
	cfg2 := &Config{Server: ServerConfig{Host: "0.0.0.0", Port: 22222}}
	require.NoError(t, WriteConfig(path, cfg2))

	infos, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infos, 1, "expected exactly one backup after one overwrite")

	// Backup content must match the FIRST (pre-overwrite) config.
	data, err := os.ReadFile(infos[0].Path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "12345", "backup should preserve the original port")
	assert.NotContains(t, string(data), "22222", "backup must NOT contain the new value")
}

func TestWriteConfig_NoBackupOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))

	infos, err := ListBackups(path)
	require.NoError(t, err)
	assert.Len(t, infos, 0, "first write should not create a backup (no source file existed)")
}

func TestWriteConfig_PrunesOldBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// 15 sequential writes — 14 backups expected without prune, 10 with prune.
	for i := 0; i < 15; i++ {
		require.NoError(t, WriteConfig(path, &Config{
			Server: ServerConfig{Host: "127.0.0.1", Port: 10000 + i},
		}))
	}

	infos, err := ListBackups(path)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(infos), DefaultBackupKeep,
		"backups must be pruned to at most DefaultBackupKeep")

	// Confirm the new (final) config is on disk.
	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 10014, loaded.Server.Port, "final write should be the current config")
}

func TestWriteConfig_FailureLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg1 := &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}
	require.NoError(t, WriteConfig(path, cfg1))

	// Force a write failure: make the config dir read-only AFTER the file
	// exists. On Unix, removing write permission on the dir prevents the
	// os.CreateTemp inside WriteConfig from succeeding.
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based permission test is Unix-only")
	}
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	err := WriteConfig(path, &Config{Server: ServerConfig{Host: "0.0.0.0", Port: 99999}})
	require.Error(t, err, "write to read-only dir must fail")

	// Original file content must be unchanged.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "12345", "original config must be intact after failed write")
	assert.NotContains(t, string(data), "99999", "failed write must not corrupt the file")
}
