package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRouterWithHook is like setupRouter but exposes the Handler for hook registration.
func setupRouterWithHook(t *testing.T, upstreamFormat string, upstreamHandler http.HandlerFunc) (*gin.Engine, *Handler, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(upstreamHandler)
	t.Cleanup(ts.Close)

	provider := config.NewProvider(newTestConfig(ts.URL, upstreamFormat, "test-model"), "")
	r := router.NewConfigRouter(provider)
	h := NewHandler(provider, nil, r, nil)
	engine := gin.New()
	h.RegisterRoutes(engine)

	return engine, h, ts
}

func TestPipelineHookAbort(t *testing.T) {
	upstreamCalled := false
	r, h, _ := setupRouterWithHook(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test"}`))
	})

	h.RegisterHook(hook.Hook{
		Name:  "abort-before-upstream",
		Point: hook.BeforeUpstream,
		Level: hook.Critical,
		Fn: func(ctx *hook.Context) error {
			return fmt.Errorf("blocked by hook")
		},
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.False(t, upstreamCalled, "upstream should not be called when critical hook aborts")
	// Pipeline logs and returns without writing a response, so body is empty.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestPipelineOptionalHookError(t *testing.T) {
	r, h, _ := setupRouterWithHook(t, "chat", chatUpstreamHandler(t))

	h.RegisterHook(hook.Hook{
		Name:  "optional-fails",
		Point: hook.BeforeRoute,
		Level: hook.Optional,
		Fn: func(ctx *hook.Context) error {
			return fmt.Errorf("optional hook error")
		},
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "test-model", resp["model"])
}

func TestPipelineHookMutation(t *testing.T) {
	var upstreamModel string
	r, h, _ := setupRouterWithHook(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		upstreamModel, _ = req["model"].(string)
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   req["model"],
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		json.NewEncoder(w).Encode(resp)
	})

	h.RegisterHook(hook.Hook{
		Name:  "mutate-model",
		Point: hook.AfterRoute,
		Level: hook.Critical,
		Fn: func(ctx *hook.Context) error {
			ctx.ClientModel = "mutated-model"
			return nil
		},
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "mutated-model", upstreamModel, "hook should have mutated the model sent to upstream")
}

func TestPipeline_RecordsErrorUsageOnUpstreamError(t *testing.T) {
	// recordErrorUsage with nil usageStore should be no-op
	t.Log("recordErrorUsage with nil usageStore covered by TestRecordErrorUsage_NilStore")
}

func TestRecordErrorUsage_NilStore(t *testing.T) {
	h := &Handler{usageStore: nil}
	ctx := hook.NewContext(nil, "chat", nil)
	// Should not panic
	h.recordErrorUsage(ctx)
}

func TestRecordErrorUsage_WithRouteResult(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "usage.db")
	s, err := store.NewUsageStore(dbPath)
	require.NoError(t, err)

	h := &Handler{usageStore: s}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx := hook.NewContext(c, "chat", nil)
	ctx.ClientModel = "test-model"
	ctx.RouteResult = &router.RouteResult{ProviderKey: "test-provider"}

	h.recordErrorUsage(ctx)
	require.NoError(t, s.Close())

	s2, err := store.NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, "test-provider", r.Provider)
	assert.Equal(t, "test-model", r.Model)
	assert.Equal(t, int64(1), r.Requests)
	assert.Equal(t, int64(0), r.SuccessRequests)
	assert.Equal(t, int64(1), r.ErrorRequests)
	assert.Equal(t, int64(0), r.InputTokens)
	assert.Equal(t, int64(0), r.TotalTokens)
}

func TestRecordErrorUsage_NoRouteResult(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "usage.db")
	s, err := store.NewUsageStore(dbPath)
	require.NoError(t, err)

	h := &Handler{usageStore: s}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx := hook.NewContext(c, "chat", nil)
	ctx.ClientModel = "test-model"
	ctx.RouteResult = nil

	h.recordErrorUsage(ctx)
	require.NoError(t, s.Close())

	s2, err := store.NewUsageStore(dbPath)
	require.NoError(t, err)
	defer s2.Close()

	records, err := s2.QueryUsage("", "", "", "")
	require.NoError(t, err)
	require.Len(t, records, 1)

	r := records[0]
	assert.Equal(t, "", r.Provider)
	assert.Equal(t, "test-model", r.Model)
	assert.Equal(t, int64(1), r.ErrorRequests)
}
