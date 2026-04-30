package log

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	LogSubDir        = "logs"
	LLMLogFilePrefix = "llm"
	AppLogFilePrefix = "app"
)

// DefaultLogRetentionDays is the default number of days to keep log files.
const DefaultLogRetentionDays = 30

// logRetentionDays is the configured number of days to keep log files.
var logRetentionDays = DefaultLogRetentionDays

// SetRetentionDays configures how many days of log files to keep.
// Must be called before creating any DailyRotateWriter.
func SetRetentionDays(days int) {
	if days > 0 {
		logRetentionDays = days
	}
}

// DailyRotateWriter writes log output to date-named files and rotates daily.
// File naming: {prefix}-{YYYY-MM-DD}.log
type DailyRotateWriter struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	file    *os.File
	current string // current date string "YYYY-MM-DD"
}

// NewDailyRotateWriter creates a writer that rotates files daily under dir/LogSubDir/.
func NewDailyRotateWriter(dir, prefix string) (*DailyRotateWriter, error) {
	logDir := filepath.Join(dir, LogSubDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	w := &DailyRotateWriter{dir: logDir, prefix: prefix}
	if err := w.rotate(time.Now()); err != nil {
		return nil, err
	}
	return w, nil
}

// Write implements io.Writer. It rotates the file when the date changes.
func (w *DailyRotateWriter) Write(p []byte) (int, error) {
	n, _, err := w.WriteWithOffset(p)
	return n, err
}

// WriteWithOffset writes p and returns the file offset before the write.
// The offset is the byte position of p in the current day's log file.
func (w *DailyRotateWriter) WriteWithOffset(p []byte) (n int, offset int64, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if today != w.current {
		if err := w.rotate(time.Now()); err != nil {
			return 0, 0, err
		}
	}
	offset, _ = w.file.Seek(0, io.SeekCurrent)
	n, err = w.file.Write(p)
	return
}

// Close closes the current log file.
func (w *DailyRotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *DailyRotateWriter) rotate(now time.Time) error {
	dateStr := now.Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.log", w.prefix, dateStr)
	path := filepath.Join(w.dir, filename)

	if w.file != nil {
		_ = w.file.Close()
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	w.file = f
	w.current = dateStr

	go w.removeOldLogs(now)

	return nil
}

func (w *DailyRotateWriter) removeOldLogs(now time.Time) {
	cutoff := now.AddDate(0, 0, -logRetentionDays).Format("2006-01-02")

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	prefix := w.prefix + "-"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		date := strings.TrimPrefix(entry.Name(), prefix)
		date = strings.TrimSuffix(date, ".log")
		if date < cutoff {
			_ = os.Remove(filepath.Join(w.dir, entry.Name()))
		}
	}
}

// LogDir returns the full path to the log subdirectory.
func LogDir(dataDir string) string {
	return filepath.Join(dataDir, LogSubDir)
}

// ListLogFiles returns log file names matching the given prefix, sorted by date descending.
func ListLogFiles(dataDir, prefix string) ([]fs.FileInfo, error) {
	dir := LogDir(dataDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	prefixStr := prefix + "-"
	var infos []fs.FileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefixStr) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos, nil
}
