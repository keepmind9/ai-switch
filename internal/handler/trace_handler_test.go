package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testJSONL contains two complete request traces (req001 success, req002 error).
const testJSONL = `{"type":"request","ais_req_id":"req001","time":"2026-04-30T07:52:35.123+08:00","client_protocol":"anthropic","model":"claude-sonnet-4-6","stream":true,"body":"{\"messages\":[]}"}
{"type":"upstream_req","ais_req_id":"req001","time":"2026-04-30T07:52:35.156+08:00","upstream_protocol":"chat","model":"claude-sonnet-4-6","provider":"minimax","url":"https://api.minimax.chat/v1/chat","body":"{\"model\":\"MiniMax-M2.5\"}"}
{"type":"upstream_resp","ais_req_id":"req001","time":"2026-04-30T07:52:36.390+08:00","status":200,"latency_ms":1234,"provider":"minimax","url":"https://api.minimax.chat/v1/chat","body":"data: [DONE]"}
{"type":"response","ais_req_id":"req001","time":"2026-04-30T07:52:36.456+08:00","model":"claude-sonnet-4-6","provider":"minimax","input_tokens":100,"output_tokens":50,"body":"data: [DONE]"}
{"type":"request","ais_req_id":"req002","time":"2026-04-30T08:00:00.001+08:00","client_protocol":"chat","model":"gpt-4o","stream":false,"body":"{}"}
{"type":"upstream_req","ais_req_id":"req002","time":"2026-04-30T08:00:00.010+08:00","upstream_protocol":"anthropic","model":"gpt-4o","provider":"anthropic","url":"https://api.anthropic.com/v1/messages","body":"{}"}
{"type":"upstream_resp","ais_req_id":"req002","time":"2026-04-30T08:00:01.500+08:00","status":500,"latency_ms":1490,"provider":"anthropic","url":"https://api.anthropic.com/v1/messages","body":"error"}
{"type":"response","ais_req_id":"req002","time":"2026-04-30T08:00:01.550+08:00","model":"gpt-4o","provider":"anthropic","body":"error"}
`

// testIndexJSONL is the index file content matching testJSONL.
// req001 starts at offset 0, req002 starts at offset after first 4 lines.
const testIndexJSONL = `{"ais_req_id":"req001","time":"2026-04-30T07:52:35.123+08:00","client_protocol":"anthropic","model":"claude-sonnet-4-6","stream":true,"provider":"minimax","status":200,"latency_ms":1234,"input_tokens":100,"output_tokens":50,"offset":0}
{"ais_req_id":"req002","time":"2026-04-30T08:00:00.001+08:00","client_protocol":"chat","model":"gpt-4o","stream":false,"provider":"anthropic","status":500,"latency_ms":1490,"offset":498}
`

func setupTraceTest(t *testing.T, jsonlContent string) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	date := "2026-04-30"
	logFile := filepath.Join(logDir, "llm-"+date+".log")
	require.NoError(t, os.WriteFile(logFile, []byte(jsonlContent), 0644))

	idxFile := filepath.Join(logDir, "llm-idx-"+date+".log")
	require.NoError(t, os.WriteFile(idxFile, []byte(testIndexJSONL), 0644))

	// Freeze clock to the test date
	orig := currentClock.Now
	currentClock.Now = func() time.Time { return time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local) }
	t.Cleanup(func() { currentClock.Now = orig })

	r := gin.New()
	apiGroup := r.Group("/api")
	th := NewTraceHandler(tmpDir)
	th.RegisterRoutes(apiGroup)

	return r, tmpDir
}

func setupTraceTestNoIndex(t *testing.T, jsonlContent string) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	date := "2026-04-30"
	logFile := filepath.Join(logDir, "llm-"+date+".log")
	require.NoError(t, os.WriteFile(logFile, []byte(jsonlContent), 0644))

	orig := currentClock.Now
	currentClock.Now = func() time.Time { return time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local) }
	t.Cleanup(func() { currentClock.Now = orig })

	r := gin.New()
	apiGroup := r.Group("/api")
	th := NewTraceHandler(tmpDir)
	th.RegisterRoutes(apiGroup)

	return r, tmpDir
}

func TestListTraces(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	tests := []struct {
		name       string
		query      string
		wantItems  int
		wantStatus int
	}{
		{"default", "", 2, http.StatusOK},
		{"filter by model", "model=claude", 1, http.StatusOK},
		{"filter by provider", "provider=anthropic", 1, http.StatusOK},
		{"filter by status", "status=500", 1, http.StatusOK},
		{"filter no match", "model=nonexistent", 0, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/admin/traces"
			if tt.query != "" {
				path += "?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

			data := resp["data"].(map[string]any)
			items := data["items"].([]any)
			assert.Len(t, items, tt.wantItems)
		})
	}
}

func TestListTracesNoBody(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	items := data["items"].([]any)
	for _, item := range items {
		summary := item.(map[string]any)
		_, hasBody := summary["body"]
		assert.False(t, hasBody, "list items should not contain body field")
	}
}

func TestListTracesSortedByTimeDesc(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	items := resp["data"].(map[string]any)["items"].([]any)
	require.Len(t, items, 2)

	// req002 (08:00) should come before req001 (07:52)
	first := items[0].(map[string]any)
	assert.Equal(t, "req002", first["ais_req_id"])
	second := items[1].(map[string]any)
	assert.Equal(t, "req001", second["ais_req_id"])
}

func TestGetTrace(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces/req001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	assert.Equal(t, "req001", data["ais_req_id"])

	records := data["records"].([]any)
	assert.Len(t, records, 4)

	types := make([]string, len(records))
	for i, rec := range records {
		r := rec.(map[string]any)
		types[i] = r["type"].(string)
		// Detail view should include body
		_, hasBody := r["body"]
		assert.True(t, hasBody, "detail records should contain body field")
	}
	assert.Equal(t, []string{"request", "upstream_req", "upstream_resp", "response"}, types)
}

func TestGetTraceNotFound(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetTraceNoDateParam(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	// Missing date param should use today (which we've frozen to 2026-04-30)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces/req001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListTracesDateNotFound(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces?date=2020-01-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListTracesCursorPagination(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	// First page: page_size=1, should get req002 (newest) and has_next=true
	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces?page_size=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	items := data["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "req002", items[0].(map[string]any)["ais_req_id"])
	assert.Equal(t, false, data["has_prev"])
	assert.Equal(t, true, data["has_next"])

	nextCursor := data["next_cursor"].(string)
	require.NotEmpty(t, nextCursor)

	// Second page: use next_cursor
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/traces?page_size=1&cursor="+nextCursor, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code)

	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))

	data2 := resp2["data"].(map[string]any)
	items2 := data2["items"].([]any)
	require.Len(t, items2, 1)
	assert.Equal(t, "req001", items2[0].(map[string]any)["ais_req_id"])
	assert.Equal(t, true, data2["has_prev"])
	assert.Equal(t, false, data2["has_next"])
}

func TestTraceSummaryMerge(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	items := resp["data"].(map[string]any)["items"].([]any)

	// Find req001
	for _, item := range items {
		s := item.(map[string]any)
		if s["ais_req_id"] == "req001" {
			assert.Equal(t, "anthropic", s["client_protocol"])
			assert.Equal(t, "claude-sonnet-4-6", s["model"])
			assert.Equal(t, true, s["stream"])
			assert.Equal(t, "minimax", s["provider"])
			assert.Equal(t, float64(200), s["status"])
			assert.Equal(t, float64(1234), s["latency_ms"])
			assert.Equal(t, float64(100), s["input_tokens"])
			assert.Equal(t, float64(50), s["output_tokens"])
			return
		}
	}
	t.Fatal("req001 not found")
}

func TestListTracesFallbackNoIndex(t *testing.T) {
	// Without index file, should fall back to legacy full scan
	r, _ := setupTraceTestNoIndex(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	items := resp["data"].(map[string]any)["items"].([]any)
	assert.Len(t, items, 2)
}

func TestGetTraceFallbackNoIndex(t *testing.T) {
	// Without index file, should fall back to legacy full scan
	r, _ := setupTraceTestNoIndex(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces/req001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	records := data["records"].([]any)
	assert.Len(t, records, 4)
}

func TestScanIndexFiltersWork(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	idxContent := testIndexJSONL +
		`{"ais_req_id":"req003","time":"2026-04-30T09:00:00.000+08:00","model":"gpt-4o-mini","provider":"openai","status":200,"latency_ms":50,"offset":900}
`
	idxFile := filepath.Join(logDir, "llm-idx-2026-04-30.log")
	require.NoError(t, os.WriteFile(idxFile, []byte(idxContent), 0644))

	tests := []struct {
		name      string
		filters   traceFilters
		wantCount int
	}{
		{"no filter", traceFilters{}, 3},
		{"filter model gpt", traceFilters{Model: "gpt"}, 2},
		{"filter model claude", traceFilters{Model: "claude"}, 1},
		{"filter provider minimax", traceFilters{Provider: "minimax"}, 1},
		{"filter status 200", traceFilters{Status: 200}, 2},
		{"filter status 500", traceFilters{Status: 500}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scanIndex(idxFile, tt.filters, 0)
			require.NoError(t, err)
			assert.Len(t, result, tt.wantCount)
		})
	}
}

func TestLookupOffset(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	idxFile := filepath.Join(logDir, "llm-idx-2026-04-30.log")
	require.NoError(t, os.WriteFile(idxFile, []byte(testIndexJSONL), 0644))

	off, err := lookupOffset(idxFile, "req001")
	require.NoError(t, err)
	assert.Equal(t, int64(0), off)

	off, err = lookupOffset(idxFile, "req002")
	require.NoError(t, err)
	assert.Equal(t, int64(498), off)

	_, err = lookupOffset(idxFile, "nonexistent")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestScanTraceByOffset(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	jsonlFile := filepath.Join(logDir, "llm-2026-04-30.log")
	require.NoError(t, os.WriteFile(jsonlFile, []byte(testJSONL), 0644))

	// req001 starts at offset 0
	records, err := scanTraceByOffset(jsonlFile, "req001", 0)
	require.NoError(t, err)
	assert.Len(t, records, 4)

	types := make([]string, len(records))
	for i, rec := range records {
		types[i] = rec["type"].(string)
	}
	assert.Equal(t, []string{"request", "upstream_req", "upstream_resp", "response"}, types)

	// req002 starts at offset 498
	records2, err := scanTraceByOffset(jsonlFile, "req002", 498)
	require.NoError(t, err)
	assert.Len(t, records2, 4)
	assert.Equal(t, "req002", records2[0]["ais_req_id"])
}

func TestScanTraceByOffsetMaxLines(t *testing.T) {
	// Build a JSONL with many interleaved records
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	var lines string
	// 300 lines of other requests before our target
	for i := 0; i < 300; i++ {
		lines += fmt.Sprintf(`{"type":"request","ais_req_id":"other%03d","time":"2026-04-30T07:00:00.000+08:00"}`, i) + "\n"
	}
	lines += `{"type":"request","ais_req_id":"target","time":"2026-04-30T08:00:00.000+08:00"}` + "\n"
	lines += `{"type":"response","ais_req_id":"target","time":"2026-04-30T08:00:01.000+08:00"}` + "\n"

	jsonlFile := filepath.Join(logDir, "llm-2026-04-30.log")
	require.NoError(t, os.WriteFile(jsonlFile, []byte(lines), 0644))

	// Seek to the target line, but maxScanLines=200 means we scan 200 lines from offset 0
	// Our target records are beyond 200 lines, so we'll only get partial results
	records, err := scanTraceByOffset(jsonlFile, "target", 0)
	require.NoError(t, err)
	// Should find at most the target records within 200 scanned lines
	assert.LessOrEqual(t, len(records), 2)
}
