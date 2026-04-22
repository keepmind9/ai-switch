package log

import (
	"context"
	"log/slog"
	"os"
)

// SetupDefaultLogger configures the global slog logger to write to a daily-rotating file
// and stderr simultaneously.
func SetupDefaultLogger(dataDir string) error {
	w, err := NewDailyRotateWriter(dataDir, AppLogFilePrefix)
	if err != nil {
		return err
	}

	fileHandler := slog.NewTextHandler(w, nil)
	stderrHandler := slog.NewTextHandler(os.Stderr, nil)
	slog.SetDefault(slog.New(&multiHandler{file: fileHandler, fallback: stderrHandler}))

	return nil
}

// NewLLMLogger creates a dedicated JSON logger for LLM request/response logging.
func NewLLMLogger(dataDir string) (*slog.Logger, error) {
	w, err := NewDailyRotateWriter(dataDir, LLMLogFilePrefix)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewJSONHandler(w, nil)), nil
}

// multiHandler duplicates log records to both file and fallback handlers.
type multiHandler struct {
	file     slog.Handler
	fallback slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return m.file.Enabled(ctx, level) || m.fallback.Enabled(ctx, level)
}

func (m *multiHandler) Handle(ctx context.Context, rec slog.Record) error {
	if err := m.file.Handle(ctx, rec); err != nil {
		return err
	}
	return m.fallback.Handle(ctx, rec)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{file: m.file.WithAttrs(attrs), fallback: m.fallback.WithAttrs(attrs)}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{file: m.file.WithGroup(name), fallback: m.fallback.WithGroup(name)}
}
