package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/keepmind9/ai-switch/internal/router"
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
