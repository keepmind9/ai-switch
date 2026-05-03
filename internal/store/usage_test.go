package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) (*UsageStore, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "usage.db")

	s, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	return s, dbPath
}

func TestNewUsageStore_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "subdir", "usage.db")

	s, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s.Close()

	assert.FileExists(t, dbPath)
}

func TestUsageStore_RecordAndQuery(t *testing.T) {
	s, dbPath := newTestStore(t)

	s.AsyncRecord(UsageRecord{
		Provider:            "minimax",
		Model:               "MiniMax-M2.5",
		Date:                "2026-04-18",
		Requests:            1,
		SuccessRequests:     1,
		InputTokens:         200,
		OutputTokens:        100,
		CacheCreationTokens: 30,
		CacheReadTokens:     70,
		TotalTokens:         300,
	})

	s.Close()

	s2, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, "minimax", r.Provider)
	assert.Equal(t, "MiniMax-M2.5", r.Model)
	assert.Equal(t, "2026-04-18", r.Date)
	assert.Equal(t, int64(1), r.Requests)
	assert.Equal(t, int64(1), r.SuccessRequests)
	assert.Equal(t, int64(0), r.ErrorRequests)
	assert.Equal(t, int64(200), r.InputTokens)
	assert.Equal(t, int64(100), r.OutputTokens)
	assert.Equal(t, int64(30), r.CacheCreationTokens)
	assert.Equal(t, int64(70), r.CacheReadTokens)
	assert.Equal(t, int64(300), r.TotalTokens)
}

func TestUsageStore_UpsertAggregation(t *testing.T) {
	s, dbPath := newTestStore(t)

	s.AsyncRecord(UsageRecord{
		Provider: "minimax", Model: "model-a", Date: "2026-04-18",
		Requests: 1, InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
	})
	s.AsyncRecord(UsageRecord{
		Provider: "minimax", Model: "model-a", Date: "2026-04-18",
		Requests: 1, InputTokens: 200, OutputTokens: 80, TotalTokens: 280,
	})

	s.Close()

	s2, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	assert.Equal(t, int64(2), records[0].Requests)
	assert.Equal(t, int64(300), records[0].InputTokens)
	assert.Equal(t, int64(130), records[0].OutputTokens)
	assert.Equal(t, int64(430), records[0].TotalTokens)
}

func TestUsageStore_QueryWithFilters(t *testing.T) {
	s, dbPath := newTestStore(t)

	s.AsyncRecord(UsageRecord{Provider: "a", Model: "m1", Date: "2026-04-16", Requests: 1, InputTokens: 10, OutputTokens: 5, TotalTokens: 15})
	s.AsyncRecord(UsageRecord{Provider: "a", Model: "m2", Date: "2026-04-17", Requests: 1, InputTokens: 20, OutputTokens: 10, TotalTokens: 30})
	s.AsyncRecord(UsageRecord{Provider: "b", Model: "m1", Date: "2026-04-18", Requests: 1, InputTokens: 30, OutputTokens: 15, TotalTokens: 45})

	s.Close()

	s2, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("a", "", "", "")
	require.NoError(t, err)
	assert.Len(t, records, 2)

	records, err = s2.QueryUsage("", "", "2026-04-17", "2026-04-18")
	require.NoError(t, err)
	assert.Len(t, records, 2)

	records, err = s2.QueryUsage("", "m1", "", "")
	require.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestUsageStore_QueryEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewUsageStore(filepath.Join(dir, "usage.db"))
	require.NoError(t, err)
	defer s.Close()

	records, err := s.QueryUsage("", "", "", "")
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestToday(t *testing.T) {
	result := Today()
	assert.Regexp(t, `\d{4}-\d{2}-\d{2}`, result)
}

func TestDefaultDBPath(t *testing.T) {
	path, err := DefaultDBPath()
	require.NoError(t, err)
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, config.DataDirName, config.UsageDBName), path)
}

func TestUsageStore_ErrorRequestsAggregation(t *testing.T) {
	s, dbPath := newTestStore(t)

	s.AsyncRecord(UsageRecord{
		Provider: "deepseek", Model: "deepseek-chat", Date: "2026-05-03",
		Requests: 1, SuccessRequests: 1, InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
	})
	s.AsyncRecord(UsageRecord{
		Provider: "deepseek", Model: "deepseek-chat", Date: "2026-05-03",
		Requests: 1, ErrorRequests: 1,
	})
	s.AsyncRecord(UsageRecord{
		Provider: "deepseek", Model: "deepseek-chat", Date: "2026-05-03",
		Requests: 1, ErrorRequests: 1,
	})

	s.Close()

	s2, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, int64(3), r.Requests)
	assert.Equal(t, int64(1), r.SuccessRequests)
	assert.Equal(t, int64(2), r.ErrorRequests)
	assert.Equal(t, int64(100), r.InputTokens)
}

func TestUsageStore_MigrationFromOldSchema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "usage.db")

	// Create DB with old schema (no success_requests/error_requests)
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE usage (
			provider TEXT NOT NULL, model TEXT NOT NULL, date TEXT NOT NULL,
			requests INTEGER DEFAULT 0, input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0, cache_creation_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0, total_tokens INTEGER DEFAULT 0,
			PRIMARY KEY (provider, model, date)
		)
	`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO usage (provider, model, date, requests, input_tokens, output_tokens, total_tokens)
		VALUES ('deepseek', 'deepseek-chat', '2026-05-01', 5, 1000, 500, 1500)`)
	require.NoError(t, err)
	db.Close()

	// Open with new code - should auto-migrate
	s, err := NewUsageStore(dbPath)
	require.NoError(t, err)

	// Record with new fields should work
	s.AsyncRecord(UsageRecord{
		Provider: "deepseek", Model: "deepseek-chat", Date: "2026-05-01",
		Requests: 1, SuccessRequests: 1, InputTokens: 200, OutputTokens: 100, TotalTokens: 300,
	})
	require.NoError(t, s.Close())

	// Verify data
	s2, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, int64(6), r.Requests)
	assert.Equal(t, int64(1), r.SuccessRequests)
	assert.Equal(t, int64(0), r.ErrorRequests)
	assert.Equal(t, int64(1200), r.InputTokens)
}

func TestUsageStore_ErrorOnlyRecord(t *testing.T) {
	s, dbPath := newTestStore(t)

	s.AsyncRecord(UsageRecord{
		Provider: "zhipu", Model: "glm-4", Date: "2026-05-03",
		Requests: 1, ErrorRequests: 1,
	})

	s.Close()

	s2, err := NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, int64(1), r.Requests)
	assert.Equal(t, int64(0), r.SuccessRequests)
	assert.Equal(t, int64(1), r.ErrorRequests)
	assert.Equal(t, int64(0), r.InputTokens)
	assert.Equal(t, int64(0), r.OutputTokens)
	assert.Equal(t, int64(0), r.TotalTokens)
}
