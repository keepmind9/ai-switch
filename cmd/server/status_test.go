package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withDataDir sets HOME to a temp dir and returns the resolved data dir, so
// each test reads/writes an isolated PID file.
func withDataDir(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	dataDir, err := config.DataDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	return dataDir
}

// seedPIDFile writes the PID file inside dataDir. (Named to avoid colliding
// with the production writePIDFile in serve.go.)
func seedPIDFile(t *testing.T, dataDir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, config.PidFileName), []byte(content), 0o644))
}

func TestRenderStatus_NoPIDFile(t *testing.T) {
	dataDir := withDataDir(t)

	out, running := renderStatus(dataDir, "")
	require.False(t, running)
	assert.Contains(t, out, "is not running")
	assert.Contains(t, out, "Reason:")
	assert.Contains(t, out, "no PID file")
	assert.Contains(t, out, "Logs:")
}

func TestRenderStatus_InvalidPIDFile(t *testing.T) {
	dataDir := withDataDir(t)
	seedPIDFile(t, dataDir, "not-a-number")

	out, running := renderStatus(dataDir, "")
	require.False(t, running)
	assert.Contains(t, out, "is not running")
	assert.Contains(t, out, "invalid PID file")
}

func TestRenderStatus_StalePID(t *testing.T) {
	dataDir := withDataDir(t)
	// A PID high enough that no real process owns it.
	seedPIDFile(t, dataDir, "99999998")

	out, running := renderStatus(dataDir, "")
	require.False(t, running)
	assert.Contains(t, out, "is not running")
	assert.Contains(t, out, "not alive")
	assert.Contains(t, out, "99999998")
}

func TestRenderStatus_RunningCurrentProcess(t *testing.T) {
	dataDir := withDataDir(t)
	// The test process itself is alive, so this exercises the running branch
	// without spawning a child process (works on every OS).
	seedPIDFile(t, dataDir, strconv.Itoa(os.Getpid()))

	out, running := renderStatus(dataDir, "")
	require.True(t, running)
	assert.Contains(t, out, "is running")
	assert.Contains(t, out, "PID:")
	assert.Contains(t, out, strconv.Itoa(os.Getpid()))
}

func TestStatusField_AlignsValues(t *testing.T) {
	// Every key, short or long, must place its value at the same column.
	rows := []string{
		statusField("PID", "1"),
		statusField("Addr", "2"),
		statusField("Config", "3"),
		statusField("Reason", "4"),
		statusField("Logs", "5"),
	}
	want := valueColumn(rows[0])
	require.NotEqual(t, -1, want, "no value column found in %q", rows[0])
	for _, r := range rows[1:] {
		assert.Equal(t, want, valueColumn(r), "value column mismatch in %q", r)
	}
}

// valueColumn returns the index where the value starts in a statusField row,
// i.e. the first non-space byte after the "Key:" prefix. Derived from the
// rendered string so the test catches real misalignment, not a hardcoded
// constant.
func valueColumn(row string) int {
	colon := strings.IndexByte(row, ':')
	if colon < 0 {
		return -1
	}
	i := colon + 1
	for i < len(row) && row[i] == ' ' {
		i++
	}
	return i
}
