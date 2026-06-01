package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DefaultBackupKeep is the default number of timestamped backups to retain.
const DefaultBackupKeep = 10

// backupTimeFormat is locale-independent and Windows-safe.
// Lexicographic order matches chronological order.
const backupTimeFormat = "20060102-150405"

// backupNanoFormat adds a 9-digit nanosecond suffix to guarantee uniqueness
// when BackupConfig is called multiple times within the same second.
// Combined length stays under 32 chars and contains no Windows-illegal chars.
const backupNanoFormat = "20060102-150405.000000000"

// backupNameRegex matches the timestamped backup filename format (with nanos).
// Accepts both second-resolution (legacy) and nanosecond-resolution names.
//
// Note: hardcoded to "config.yaml" because all config files in this project
// use that name. If a different config path is ever supported, this regex
// (and backupName/backupTimeFormat) must be generalized.
var backupNameRegex = regexp.MustCompile(`^config\.yaml\.bak\.(\d{8})-(\d{6})(?:\.(\d{9}))?$`)

// BackupInfo describes a single timestamped backup file.
type BackupInfo struct {
	Name    string    `json:"name"` // basename, e.g. "config.yaml.bak.20260601-143052.123456789"
	Path    string    `json:"path"` // absolute path
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// backupName returns the timestamped backup filename for the given config path.
// The timestamp uses local time plus a nanosecond suffix to guarantee
// uniqueness even when called multiple times within the same second.
func backupName(path string, t time.Time) string {
	base := filepath.Base(path)
	dir := filepath.Dir(path)
	return filepath.Join(dir, fmt.Sprintf("%s.bak.%s", base, t.Format(backupNanoFormat)))
}

// ListBackups returns all timestamped backups of path, sorted newest-first.
// Returns an empty slice (not nil) when no backups exist.
func ListBackups(path string) ([]BackupInfo, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	prefix := base + ".bak."

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("list backups: %w", err)
	}

	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		// Validate the full backup filename against the expected pattern.
		if !backupNameRegex.MatchString(name) {
			continue
		}
		full := filepath.Join(dir, name)
		fi, err := e.Info()
		if err != nil {
			slog.Debug("config: skipping backup with unreadable info", "name", name, "error", err)
			continue
		}
		out = append(out, BackupInfo{
			Name:    name,
			Path:    full,
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}

	// Sort newest-first. Primary key: ModTime (mtime). When mtimes are equal
	// (e.g. files written within the same millisecond), fall back to the
	// filename — the nanosecond suffix always sorts AFTER the empty suffix
	// of the same second, so a nano backup is considered newer than a
	// legacy one from the same second.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		return out[i].Name > out[j].Name
	})
	if out == nil {
		out = []BackupInfo{}
	}
	return out, nil
}

// BackupConfig copies the current file at path to a timestamped backup.
// Returns the absolute path of the new backup, or "" if the source does not
// exist (first-time write). Returns an error only on real I/O failures.
//
// The copy uses io.Copy — never os.Rename — because:
//   - We want to *create a new* timestamped file, not move the existing one.
//   - os.Rename to an existing destination fails on Windows.
//   - io.Copy works identically on Linux, macOS, and Windows.
//
// The backup filename includes nanosecond resolution and is created with
// O_EXCL — if a file with the same name already exists (extremely unlikely
// given nanosecond resolution, but possible on a clock with sub-nanosecond
// resolution or non-monotonic sources), we retry up to 3 times with a fresh
// timestamp before giving up.
//
// The source is read fully into memory so the retry loop can copy from the
// same byte slice each attempt. Config files are small (KB), so this is
// preferable to seeking or re-opening the source.
func BackupConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("open source for backup: %w", err)
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		dstPath := backupName(path, time.Now())
		dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
		if err != nil {
			if os.IsExist(err) {
				lastErr = err
				continue // name collision — retry with new timestamp
			}
			return "", fmt.Errorf("create backup file: %w", err)
		}

		if _, err := dst.Write(data); err != nil {
			dst.Close()
			os.Remove(dstPath)
			return "", fmt.Errorf("copy to backup: %w", err)
		}
		if err := dst.Sync(); err != nil {
			dst.Close()
			os.Remove(dstPath)
			return "", fmt.Errorf("sync backup: %w", err)
		}
		if err := dst.Close(); err != nil {
			os.Remove(dstPath)
			return "", fmt.Errorf("close backup: %w", err)
		}
		return dstPath, nil
	}
	return "", fmt.Errorf("create backup file: exhausted %d attempts after collision: %w", maxAttempts, lastErr)
}

// RestoreConfig copies the named backup back to path. The name must be a
// basename returned by ListBackups — any other value is rejected to prevent
// path traversal.
func RestoreConfig(path, name string) error {
	if name == "" {
		return errors.New("restore: empty backup name")
	}
	// Reject anything that isn't a plain basename.
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("restore: invalid backup name %q", name)
	}
	if !backupNameRegex.MatchString(name) {
		return fmt.Errorf("restore: %q is not a recognized backup filename", name)
	}

	dir := filepath.Dir(path)
	src := filepath.Join(dir, name)

	// Final safety check: resolved source must remain inside dir.
	cleanDir := filepath.Clean(dir) + string(os.PathSeparator)
	cleanSrc := filepath.Clean(src)
	if !strings.HasPrefix(cleanSrc, cleanDir) {
		return fmt.Errorf("restore: path traversal detected for %q", name)
	}

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("restore: backup %q not found: %w", name, err)
	}

	// Back up the current main file before overwriting, so the restore is
	// itself reversible. If the main file exists, this is a hard prerequisite
	// (skipping it would risk total loss on a failed restore). If the main
	// file doesn't exist yet, the pre-restore backup is unnecessary.
	if _, statErr := os.Stat(path); statErr == nil {
		if _, berr := BackupConfig(path); berr != nil {
			return fmt.Errorf("restore: pre-restore backup failed (refusing to overwrite current config): %w", berr)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("restore: stat current config: %w", statErr)
	}

	// Replace via temp + rename so a partial restore never leaves a half-file.
	tmp, err := os.CreateTemp(dir, ".config-restore-*")
	if err != nil {
		return fmt.Errorf("restore: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup if we exit early.
		if err := os.Remove(tmpName); err != nil && !os.IsNotExist(err) {
			slog.Debug("config: failed to clean up restore temp file", "path", tmpName, "error", err)
		}
	}()

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("restore: open backup: %w", err)
	}
	defer in.Close()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return fmt.Errorf("restore: copy: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("restore: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("restore: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("restore: rename: %w", err)
	}
	return nil
}

// PruneBackups keeps at most keep timestamped backups for path, deleting the
// oldest. keep must be >= 0. keep == 0 deletes all backups. Errors deleting
// individual files are logged but do not abort the prune.
func PruneBackups(path string, keep int) error {
	if keep < 0 {
		return fmt.Errorf("prune backups: keep must be >= 0, got %d", keep)
	}
	infos, err := ListBackups(path)
	if err != nil {
		return err
	}
	if len(infos) <= keep {
		return nil
	}
	for _, bi := range infos[keep:] {
		if err := os.Remove(bi.Path); err != nil && !os.IsNotExist(err) {
			slog.Warn("config: prune backup failed", "path", bi.Path, "error", err)
		}
	}
	return nil
}
