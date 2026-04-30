package handler

import (
	"encoding/json"
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

func setupTraceTest(t *testing.T, jsonlContent string) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0755))

	date := "2026-04-30"
	logFile := filepath.Join(logDir, "llm-"+date+".log")
	require.NoError(t, os.WriteFile(logFile, []byte(jsonlContent), 0644))

	// Create another date file to test listDates
	otherDate := "2026-04-29"
	otherFile := filepath.Join(logDir, "llm-"+otherDate+".log")
	require.NoError(t, os.WriteFile(otherFile, []byte(""), 0644))

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

const testJSONL = `{"type":"request","ais_req_id":"req001","time":"2026-04-30T07:52:35.123+08:00","client_protocol":"anthropic","model":"claude-sonnet-4-6","stream":true,"body":"{\"messages\":[]}"}
{"type":"upstream_req","ais_req_id":"req001","time":"2026-04-30T07:52:35.156+08:00","upstream_protocol":"chat","model":"claude-sonnet-4-6","provider":"minimax","url":"https://api.minimax.chat/v1/chat","body":"{\"model\":\"MiniMax-M2.5\"}"}
{"type":"upstream_resp","ais_req_id":"req001","time":"2026-04-30T07:52:36.390+08:00","status":200,"latency_ms":1234,"provider":"minimax","url":"https://api.minimax.chat/v1/chat","body":"data: [DONE]"}
{"type":"response","ais_req_id":"req001","time":"2026-04-30T07:52:36.456+08:00","model":"claude-sonnet-4-6","provider":"minimax","input_tokens":100,"output_tokens":50,"body":"data: [DONE]"}
{"type":"request","ais_req_id":"req002","time":"2026-04-30T08:00:00.001+08:00","client_protocol":"chat","model":"gpt-4o","stream":false,"body":"{}"}
{"type":"upstream_req","ais_req_id":"req002","time":"2026-04-30T08:00:00.010+08:00","upstream_protocol":"anthropic","model":"gpt-4o","provider":"anthropic","url":"https://api.anthropic.com/v1/messages","body":"{}"}
{"type":"upstream_resp","ais_req_id":"req002","time":"2026-04-30T08:00:01.500+08:00","status":500,"latency_ms":1490,"provider":"anthropic","url":"https://api.anthropic.com/v1/messages","body":"error"}
{"type":"response","ais_req_id":"req002","time":"2026-04-30T08:00:01.550+08:00","model":"gpt-4o","provider":"anthropic","body":"error"}
`

func TestListTraces(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	tests := []struct {
		name       string
		query      string
		wantTotal  int
		wantStatus int
	}{
		{"default", "", 2, http.StatusOK},
		{"filter by model", "model=claude", 1, http.StatusOK},
		{"filter by provider", "provider=anthropic", 1, http.StatusOK},
		{"filter by status", "status=500", 1, http.StatusOK},
		{"filter no match", "model=nonexistent", 0, http.StatusOK},
		{"page 2", "page=2&page_size=1", 2, http.StatusOK},
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
			assert.Equal(t, tt.wantTotal, int(data["total"].(float64)))
			assert.Len(t, items, min(tt.wantTotal, int(data["page_size"].(float64))))
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

func TestListDates(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces/dates", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	dates := resp["data"].([]any)
	require.Len(t, dates, 2)
	// Sorted descending
	assert.Equal(t, "2026-04-30", dates[0].(string))
	assert.Equal(t, "2026-04-29", dates[1].(string))
}

func TestListTracesDateNotFound(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces?date=2020-01-01", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListTracesPagination(t *testing.T) {
	r, _ := setupTraceTest(t, testJSONL)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/traces?page=2&page_size=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	assert.Equal(t, float64(2), data["total"])
	assert.Equal(t, float64(2), data["page"])
	assert.Equal(t, float64(1), data["page_size"])

	items := data["items"].([]any)
	assert.Len(t, items, 1)
}

func TestTraceSummaryMerge(t *testing.T) {
	// Verify that summary fields are correctly merged from different record types
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
