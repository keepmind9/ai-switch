package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// UsageRecord represents a single usage data point.
type UsageRecord struct {
	Provider            string `json:"provider"`
	Model               string `json:"model"`
	Date                string `json:"date"`
	Requests            int64  `json:"requests"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
}

// UsageStore provides async SQLite storage for usage statistics.
type UsageStore struct {
	db   *sql.DB
	ch   chan UsageRecord
	done chan struct{}
	wg   sync.WaitGroup
}

// NewUsageStore creates a new UsageStore with the database at the given path.
func NewUsageStore(dbPath string) (*UsageStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	s := &UsageStore{
		db:   db,
		ch:   make(chan UsageRecord, 1024),
		done: make(chan struct{}),
	}

	s.wg.Add(1)
	go s.processLoop()

	return s, nil
}

// AsyncRecord queues a usage record for async writing.
func (s *UsageStore) AsyncRecord(record UsageRecord) {
	select {
	case s.ch <- record:
	default:
		slog.Warn("usage channel full, dropping record", "provider", record.Provider, "model", record.Model)
	}
}

// Close stops the processing loop and closes the database.
func (s *UsageStore) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}

func (s *UsageStore) processLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			// Drain remaining records
			for len(s.ch) > 0 {
				record := <-s.ch
				if err := s.upsert(record); err != nil {
					slog.Error("failed to record usage", "error", err)
				}
			}
			return

		case record := <-s.ch:
			if err := s.upsert(record); err != nil {
				slog.Error("failed to record usage", "error", err)
			}
		}
	}
}

func (s *UsageStore) upsert(record UsageRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO usage (provider, model, date, requests, input_tokens, output_tokens,
		                   cache_creation_tokens, cache_read_tokens, total_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider, model, date) DO UPDATE SET
			requests = requests + excluded.requests,
			input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens,
			cache_creation_tokens = cache_creation_tokens + excluded.cache_creation_tokens,
			cache_read_tokens = cache_read_tokens + excluded.cache_read_tokens,
			total_tokens = total_tokens + excluded.total_tokens
	`, record.Provider, record.Model, record.Date,
		record.Requests, record.InputTokens, record.OutputTokens,
		record.CacheCreationTokens, record.CacheReadTokens, record.TotalTokens)

	return err
}

// QueryUsage retrieves usage records with optional filters.
func (s *UsageStore) QueryUsage(provider, model, startDate, endDate string) ([]UsageRecord, error) {
	query := "SELECT provider, model, date, requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, total_tokens FROM usage WHERE 1=1"
	var args []any

	if provider != "" {
		query += " AND provider = ?"
		args = append(args, provider)
	}
	if model != "" {
		query += " AND model = ?"
		args = append(args, model)
	}
	if startDate != "" {
		query += " AND date >= ?"
		args = append(args, startDate)
	}
	if endDate != "" {
		query += " AND date <= ?"
		args = append(args, endDate)
	}

	query += " ORDER BY date DESC, provider, model"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(&r.Provider, &r.Model, &r.Date, &r.Requests,
			&r.InputTokens, &r.OutputTokens, &r.CacheCreationTokens,
			&r.CacheReadTokens, &r.TotalTokens); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// DefaultDBPath returns the default path for the usage database.
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".llm-gateway", "usage.db"), nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage (
			provider              TEXT NOT NULL,
			model                 TEXT NOT NULL,
			date                  TEXT NOT NULL,
			requests              INTEGER DEFAULT 0,
			input_tokens          INTEGER DEFAULT 0,
			output_tokens         INTEGER DEFAULT 0,
			cache_creation_tokens INTEGER DEFAULT 0,
			cache_read_tokens     INTEGER DEFAULT 0,
			total_tokens          INTEGER DEFAULT 0,
			PRIMARY KEY (provider, model, date)
		)
	`)
	return err
}

// Today returns today's date in YYYY-MM-DD format.
func Today() string {
	return time.Now().Format("2006-01-02")
}
