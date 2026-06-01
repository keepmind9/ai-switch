package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WriteConfig marshals the config and writes it to the given file path atomically.
// Before the atomic rename, the existing file (if any) is copied to a
// timestamped backup via BackupConfig. A backup failure is logged but does
// not block the write — atomicity of the new config is more important than
// preservation of the backup. On a successful write, PruneBackups is called
// to keep at most DefaultBackupKeep backups.
func WriteConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Back up the existing file (if any) before the atomic rename. We do
	// this AFTER writing the temp file so a backup failure cannot lose the
	// new data, and the temp file already exists on disk in case we crash.
	if _, berr := BackupConfig(path); berr != nil {
		slog.Warn("config: pre-write backup failed", "path", path, "error", berr)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Prune old backups. Best-effort; failure here is non-fatal.
	if err := PruneBackups(path, DefaultBackupKeep); err != nil {
		slog.Warn("config: prune backups failed", "path", path, "error", err)
	}

	return nil
}
