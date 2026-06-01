package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestBackupName_FormatMatchesRegex(t *testing.T) {
	ts := time.Date(2026, 6, 1, 14, 30, 52, 123456789, time.Local)
	got := backupName("/some/dir/config.yaml", ts)
	// backupName joins dir with the new filename, preserving the absolute prefix.
	assert.Equal(t, "/some/dir/config.yaml.bak.20260601-143052.123456789", got)
	assert.Regexp(t, `^config\.yaml\.bak\.\d{8}-\d{6}\.\d{9}$`, filepath.Base(got))
}

func TestBackupName_WindowsSafeChars(t *testing.T) {
	// The format string must not produce any of these characters on any platform.
	ts := time.Now()
	name := filepath.Base(backupName("/x/config.yaml", ts))
	illegal := `<>:"/\|?*`
	for _, c := range illegal {
		assert.NotContains(t, name, string(c), "backup name contains illegal char %q", c)
	}
}

func TestListBackups_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "server:\n  host: 127.0.0.1\n  port: 12345\n")
	infos, err := ListBackups(path)
	require.NoError(t, err)
	assert.NotNil(t, infos, "should return empty slice, not nil")
	assert.Len(t, infos, 0)
}

func TestListBackups_SortedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	// Create backups with explicit, ascending mtimes (filesystem mtime
	// resolution is OS-dependent; we set it deterministically via Chtimes).
	names := []string{
		"config.yaml.bak.20260101-100000",
		"config.yaml.bak.20260201-100000",
		"config.yaml.bak.20260301-100000",
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, n := range names {
		full := filepath.Join(dir, n)
		require.NoError(t, os.WriteFile(full, []byte("x: 1\n"), 0644))
		require.NoError(t, os.Chtimes(full, base.Add(time.Duration(i)*time.Hour), base.Add(time.Duration(i)*time.Hour)))
	}

	infos, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infos, 3)

	// Newest first.
	assert.Equal(t, names[2], infos[0].Name)
	assert.Equal(t, names[1], infos[1].Name)
	assert.Equal(t, names[0], infos[2].Name)

	// Verify they ARE sorted descending by ModTime.
	assert.True(t, sort.SliceIsSorted(infos, func(i, j int) bool {
		return infos[i].ModTime.After(infos[j].ModTime)
	}))
}

func TestListBackups_MixedLegacyAndNanoFormats_SortedByModTime(t *testing.T) {
	// Even if a legacy (no-nano) backup happens to be created later than a
	// nano one, mtime order is the source of truth. ListBackups must not
	// rely on string comparison.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	// Force distinct, deterministic mtimes: legacy = 1h ago, nano = now.
	legacyPath := filepath.Join(dir, "config.yaml.bak.20260101-100000")
	require.NoError(t, os.WriteFile(legacyPath, []byte("x: 1\n"), 0644))
	past := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(legacyPath, past, past))

	_, err := BackupConfig(path) // newer, nano-suffix
	require.NoError(t, err)
	infos, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infos, 2)

	// Nano one (newest) must be first; legacy second.
	assert.Contains(t, infos[0].Name, ".", "newest entry should be the nano-suffix backup")
	assert.Equal(t, legacyPath, infos[1].Path)
}

func TestListBackups_IgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	// Unrelated files that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("x: 1\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml.bak"), []byte("x: 1\n"), 0644))          // no timestamp
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml.bak.bad"), []byte("x: 1\n"), 0644))      // bad suffix
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml.bak.20260101"), []byte("x: 1\n"), 0644)) // 8 digits, no dash
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x: 1\n"), 0644))

	// One valid one.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml.bak.20260101-100000"), []byte("x: 1\n"), 0644))

	infos, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, "config.yaml.bak.20260101-100000", infos[0].Name)
}

func TestListBackups_NonExistentDir(t *testing.T) {
	infos, err := ListBackups(filepath.Join(t.TempDir(), "nope", "config.yaml"))
	require.NoError(t, err)
	assert.Len(t, infos, 0)
}

func TestBackupConfig_FirstTimeReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Source does not exist yet.
	got, err := BackupConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "", got)

	entries, _ := os.ReadDir(dir)
	assert.Len(t, entries, 0, "no backup file should be created when source missing")
}

func TestBackupConfig_CopiesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	want := "server:\n  host: 1.2.3.4\n  port: 9000\n"
	writeFile(t, path, want)

	got, err := BackupConfig(path)
	require.NoError(t, err)
	require.NotEqual(t, "", got)

	gotBytes, err := os.ReadFile(got)
	require.NoError(t, err)
	assert.Equal(t, want, string(gotBytes))

	// Source file must still be intact.
	srcBytes, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, string(srcBytes))
}

func TestBackupConfig_DistinctTimestampsProduceDistinctNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	// Nanosecond timestamp suffix means back-to-back calls produce different
	// names — no sleep needed.
	first, err := BackupConfig(path)
	require.NoError(t, err)

	second, err := BackupConfig(path)
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestRestoreConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "server:\n  host: 1.1.1.1\n  port: 1111\n"
	writeFile(t, path, original)

	bak, err := BackupConfig(path)
	require.NoError(t, err)

	// Overwrite main with something else.
	writeFile(t, path, "server:\n  host: 9.9.9.9\n  port: 9999\n")

	// Restore.
	bakName := filepath.Base(bak)
	require.NoError(t, RestoreConfig(path, bakName))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestRestoreConfig_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	cases := []string{
		"../etc/passwd",
		"..\\windows\\system32\\config",
		"/absolute/path",
		"subdir/config.yaml.bak.20260101-100000",
		"",
		"config.yaml.bak.NOTATIMESTAMP",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			err := RestoreConfig(path, name)
			assert.Error(t, err, "must reject %q", name)
		})
	}
}

func TestRestoreConfig_RejectsUnknownName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	err := RestoreConfig(path, "config.yaml.bak.20990101-000000")
	assert.Error(t, err)
}

func TestRestoreConfig_SourceMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	// No backup actually exists.
	err := RestoreConfig(path, "config.yaml.bak.20260101-100000")
	assert.Error(t, err)
}

func TestRestoreConfig_OverwritesMain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "old: 1\n")

	// Pre-create a backup; nanosecond suffix makes names unique.
	bak, err := BackupConfig(path)
	require.NoError(t, err)
	writeFile(t, path, "new: 2\n")

	// Before restore: 1 backup (the original).
	infos, _ := ListBackups(path)
	require.Len(t, infos, 1)

	require.NoError(t, RestoreConfig(path, filepath.Base(bak)))

	// After restore: 2 backups — the restore itself backed up "new" first.
	infos, _ = ListBackups(path)
	assert.Len(t, infos, 2)
}

func TestPruneBackups_KeepsN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")

	// Create 12 backups — nanosecond timestamps guarantee distinct names.
	for i := 0; i < 12; i++ {
		_, err := BackupConfig(path)
		require.NoError(t, err)
	}
	allBefore, _ := ListBackups(path)
	require.Len(t, allBefore, 12)

	// Force a strictly-increasing mtime spread so sort order is deterministic
	// across filesystems (mtime resolution can be coarse on some OSes).
	base := time.Now().Add(-time.Hour)
	for i, bi := range allBefore {
		// allBefore is newest-first, so the LAST element is the oldest.
		// Give the oldest the earliest mtime, the newest the latest.
		ts := base.Add(time.Duration(len(allBefore)-1-i) * time.Second)
		require.NoError(t, os.Chtimes(bi.Path, ts, ts))
	}
	allBefore, _ = ListBackups(path)
	require.Len(t, allBefore, 12)
	// allBefore is newest-first, so the OLDEST two are the last two entries.
	oldestName := allBefore[10].Name
	secondOldestName := allBefore[11].Name

	require.NoError(t, PruneBackups(path, 10))

	infos, _ := ListBackups(path)
	assert.Len(t, infos, 10)

	// All retained names must be distinct.
	seen := make(map[string]bool, 10)
	for _, bi := range infos {
		assert.False(t, seen[bi.Name], "duplicate backup retained: %s", bi.Name)
		seen[bi.Name] = true
	}

	// The two oldest should be gone; the others retained.
	assert.NotContains(t, seen, oldestName, "oldest backup should be pruned")
	assert.NotContains(t, seen, secondOldestName, "second-oldest backup should be pruned")
	for i := 0; i < 10; i++ {
		assert.True(t, seen[allBefore[i].Name], "newer backup should be retained: %s", allBefore[i].Name)
	}
}

func TestPruneBackups_KeepZeroDeletesAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")
	for i := 0; i < 3; i++ {
		_, err := BackupConfig(path)
		require.NoError(t, err)
	}

	require.NoError(t, PruneBackups(path, 0))
	infos, _ := ListBackups(path)
	assert.Len(t, infos, 0)
}

func TestPruneBackups_KeepGreaterThanCount_NoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")
	_, err := BackupConfig(path)
	require.NoError(t, err)

	require.NoError(t, PruneBackups(path, 100))
	infos, _ := ListBackups(path)
	assert.Len(t, infos, 1)
}

func TestPruneBackups_NonExistentDir(t *testing.T) {
	err := PruneBackups(filepath.Join(t.TempDir(), "nope", "config.yaml"), 5)
	require.NoError(t, err) // dir missing is fine — no backups to prune
}

func TestPruneBackups_NegativeKeepReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")
	_, err := BackupConfig(path)
	require.NoError(t, err)

	err = PruneBackups(path, -1)
	require.Error(t, err, "negative keep must not silently delete everything")
	// Backups must remain intact.
	infos, _ := ListBackups(path)
	assert.Len(t, infos, 1, "no backup should be deleted on invalid keep")
}

func TestBackupConfig_RaceFreeOnWindows(t *testing.T) {
	// BackupConfig uses O_EXCL (not O_TRUNC) for the destination, and the
	// source is read into memory once and written per attempt. The full
	// backup flow is exercised repeatedly to surface any Windows-only flake
	// on the dev's machine.
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific smoke test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "x: 1\n")
	for i := 0; i < 50; i++ {
		_, err := BackupConfig(path)
		require.NoError(t, err)
	}
}

// Sanity: BackupConfig surfaces I/O errors instead of silently returning "".
func TestBackupConfig_ReturnsErrorOnIOFailure(t *testing.T) {
	// Force a real I/O error by using a path whose parent is a regular file
	// (cannot be opened as a directory). This is portable across OSes.
	dir := t.TempDir()
	blocking := filepath.Join(dir, "blocker")
	writeFile(t, blocking, "i am a file, not a dir")
	path := filepath.Join(blocking, "config.yaml")

	_, err := BackupConfig(path)
	require.Error(t, err)
	// The error wraps either the source-open step or the backup-create step,
	// depending on which fails first on this OS. Both are real I/O failures.
	assert.True(t,
		strings.Contains(err.Error(), "open source for backup") ||
			strings.Contains(err.Error(), "create backup file"),
		"error should describe a backup I/O step, got: %v", err)
}
