package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDailyRotateWriter_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "nested", "logs")
	w, err := NewDailyRotateWriter(dir, "test")
	require.NoError(t, err)
	defer w.Close()

	assert.DirExists(t, filepath.Join(dir, LogSubDir))
}

func TestDailyRotateWriter_WritesToFile(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewDailyRotateWriter(tmpDir, "test")
	require.NoError(t, err)

	n, err := w.Write([]byte("hello world\n"))
	require.NoError(t, err)
	assert.Equal(t, 12, n)
	w.Close()

	today := time.Now().Format("2006-01-02")
	expected := filepath.Join(tmpDir, LogSubDir, "test-"+today+".log")
	data, err := os.ReadFile(expected)
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", string(data))
}

func TestDailyRotateWriter_MultipleWrites(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewDailyRotateWriter(tmpDir, "test")
	require.NoError(t, err)

	w.Write([]byte("line1\n"))
	w.Write([]byte("line2\n"))
	w.Write([]byte("line3\n"))
	w.Close()

	today := time.Now().Format("2006-01-02")
	expected := filepath.Join(tmpDir, LogSubDir, "test-"+today+".log")
	data, err := os.ReadFile(expected)
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2\nline3\n", string(data))
}

func TestDailyRotateWriter_DifferentPrefixes(t *testing.T) {
	tmpDir := t.TempDir()

	w1, err := NewDailyRotateWriter(tmpDir, "llm")
	require.NoError(t, err)
	w1.Write([]byte("llm log\n"))
	w1.Close()

	w2, err := NewDailyRotateWriter(tmpDir, "app")
	require.NoError(t, err)
	w2.Write([]byte("app log\n"))
	w2.Close()

	today := time.Now().Format("2006-01-02")
	llmData, err := os.ReadFile(filepath.Join(tmpDir, LogSubDir, "llm-"+today+".log"))
	require.NoError(t, err)
	assert.Equal(t, "llm log\n", string(llmData))

	appData, err := os.ReadFile(filepath.Join(tmpDir, LogSubDir, "app-"+today+".log"))
	require.NoError(t, err)
	assert.Equal(t, "app log\n", string(appData))
}

func TestDailyRotateWriter_RemovesOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, LogSubDir)
	require.NoError(t, os.MkdirAll(logDir, 0755))

	cutoff := time.Now().AddDate(0, 0, -logRetentionDays)
	oldDate := cutoff.Add(-24 * time.Hour).Format("2006-01-02")
	recentDate := time.Now().Format("2006-01-02")

	oldFile := filepath.Join(logDir, "test-"+oldDate+".log")
	recentFile := filepath.Join(logDir, "test-"+recentDate+".log")
	require.NoError(t, os.WriteFile(oldFile, []byte("old"), 0644))
	require.NoError(t, os.WriteFile(recentFile, []byte("recent"), 0644))

	w, err := NewDailyRotateWriter(tmpDir, "test")
	require.NoError(t, err)
	w.Write([]byte("new\n"))
	w.Close()

	// Wait for async cleanup goroutine to finish
	time.Sleep(100 * time.Millisecond)

	assert.NoFileExists(t, oldFile)
	assert.FileExists(t, recentFile)
}

func TestDailyRotateWriter_DoesNotRemoveOtherPrefixes(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, LogSubDir)
	require.NoError(t, os.MkdirAll(logDir, 0755))

	oldDate := time.Now().AddDate(0, 0, -(logRetentionDays + 5)).Format("2006-01-02")
	otherFile := filepath.Join(logDir, "other-"+oldDate+".log")
	require.NoError(t, os.WriteFile(otherFile, []byte("keep"), 0644))

	w, err := NewDailyRotateWriter(tmpDir, "test")
	require.NoError(t, err)
	w.Write([]byte("data\n"))
	w.Close()

	time.Sleep(100 * time.Millisecond)
	assert.FileExists(t, otherFile)
}

func TestDailyRotateWriter_Appends(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, LogSubDir)
	require.NoError(t, os.MkdirAll(logDir, 0755))

	today := time.Now().Format("2006-01-02")
	existing := filepath.Join(logDir, "test-"+today+".log")
	require.NoError(t, os.WriteFile(existing, []byte("existing\n"), 0644))

	w, err := NewDailyRotateWriter(tmpDir, "test")
	require.NoError(t, err)
	w.Write([]byte("appended\n"))
	w.Close()

	data, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Equal(t, "existing\nappended\n", string(data))
}

func TestLogDir(t *testing.T) {
	assert.Equal(t, filepath.Join("/data", LogSubDir), LogDir("/data"))
}

func TestListLogFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, LogSubDir)
	require.NoError(t, os.MkdirAll(logDir, 0755))

	today := time.Now().Format("2006-01-02")
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "llm-"+today+".log"), []byte("a"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "app-"+today+".log"), []byte("b"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "other.txt"), []byte("c"), 0644))

	llmFiles, err := ListLogFiles(tmpDir, "llm")
	require.NoError(t, err)
	assert.Len(t, llmFiles, 1)
	assert.Equal(t, "llm-"+today+".log", llmFiles[0].Name())

	appFiles, err := ListLogFiles(tmpDir, "app")
	require.NoError(t, err)
	assert.Len(t, appFiles, 1)

	otherFiles, err := ListLogFiles(tmpDir, "nonexistent")
	require.NoError(t, err)
	assert.Len(t, otherFiles, 0)
}

func TestDailyRotateWriter_WriteWithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewDailyRotateWriter(tmpDir, "test")
	require.NoError(t, err)
	defer w.Close()

	n, off1, err := w.WriteWithOffset([]byte("first\n"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, int64(0), off1)

	n, off2, err := w.WriteWithOffset([]byte("second\n"))
	require.NoError(t, err)
	assert.Equal(t, 7, n)
	assert.Equal(t, int64(6), off2)

	n, off3, err := w.WriteWithOffset([]byte("third\n"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, int64(13), off3)

	// Verify file content
	today := time.Now().Format("2006-01-02")
	data, err := os.ReadFile(filepath.Join(tmpDir, LogSubDir, "test-"+today+".log"))
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond\nthird\n", string(data))
}
