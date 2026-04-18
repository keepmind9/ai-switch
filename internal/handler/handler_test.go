package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/keepmind9/llm-gateway/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticRouter is a test router that always returns a fixed result.
type staticRouter struct {
	result *router.RouteResult
}

func (r *staticRouter) Route(_, _ string, _ []byte) (*router.RouteResult, error) {
	return r.result, nil
}

// --- Unit tests for forwardRequest and formatToPath ---

func TestForwardRequest_DefaultPath(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil)
	result := &router.RouteResult{
		BaseURL: ts.URL,
		APIKey:  "test-key",
		Format:  "chat",
	}

	resp, err := h.forwardRequest(result, "/v1/chat/completions", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "/v1/chat/completions", requestedPath)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestForwardRequest_PathOverride(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil)
	result := &router.RouteResult{
		BaseURL: ts.URL,
		Path:    "/proxy/v1/chat/completions",
		APIKey:  "test-key",
		Format:  "chat",
	}

	resp, err := h.forwardRequest(result, "/v1/chat/completions", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "/proxy/v1/chat/completions", requestedPath)
}

func TestForwardRequest_TrailingSlash(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil)
	result := &router.RouteResult{
		BaseURL: ts.URL + "/",
		APIKey:  "test-key",
		Format:  "chat",
	}

	resp, err := h.forwardRequest(result, "/v1/chat/completions", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "/v1/chat/completions", requestedPath)
}

func TestForwardRequest_AnthropicHeaders(t *testing.T) {
	var authHeader, versionHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("x-api-key")
		versionHeader = r.Header.Get("anthropic-version")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil)
	result := &router.RouteResult{
		BaseURL: ts.URL,
		APIKey:  "anth-key",
		Format:  "anthropic",
	}

	resp, err := h.forwardRequest(result, "/v1/messages", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "anth-key", authHeader)
	assert.Equal(t, "2023-06-01", versionHeader)
}

func TestForwardRequest_ChatBearerHeader(t *testing.T) {
	var authHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil)
	result := &router.RouteResult{
		BaseURL: ts.URL,
		APIKey:  "bearer-key",
		Format:  "chat",
	}

	resp, err := h.forwardRequest(result, "/v1/chat/completions", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "Bearer bearer-key", authHeader)
}

func TestForwardRequest_UpstreamError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil)
	result := &router.RouteResult{
		BaseURL: ts.URL,
		APIKey:  "key",
		Format:  "chat",
	}

	resp, err := h.forwardRequest(result, "/v1/chat/completions", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestFormatToPath(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{
		{"chat", "/chat/completions"},
		{"", "/chat/completions"},
		{"anthropic", "/v1/messages"},
		{"responses", "/v1/responses"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatToPath(tt.format))
		})
	}
}

// --- Integration tests for endpoint handlers ---

// setupRouter creates a test Gin engine with the handler wired to a mock upstream.
func setupRouter(t *testing.T, upstreamFormat string, upstreamHandler http.HandlerFunc) (*gin.Engine, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(upstreamHandler)
	t.Cleanup(ts.Close)

	provider := config.NewProvider(&config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			APIKey:  "test-key",
			Format:  upstreamFormat,
			Model:   "test-model",
		},
	}, "")

	r := router.NewConfigRouter(provider)
	h := NewHandler(provider, nil, r)
	engine := gin.New()
	h.RegisterRoutes(engine)

	return engine, ts
}

func doRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// chatUpstreamHandler returns a mock upstream that responds to Chat Completions.
func chatUpstreamHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   req["model"],
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from upstream",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func TestHandleChat_Passthrough(t *testing.T) {
	r, _ := setupRouter(t, "chat", chatUpstreamHandler(t))

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "test-model", resp["model"])

	choices := resp["choices"].([]any)
	require.Len(t, choices, 1)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	assert.Equal(t, "Hello from upstream", msg["content"])
}

func TestHandleChat_InvalidBody(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "POST", "/v1/chat/completions", `not json`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleChat_UpstreamError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"unavailable"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleChat_DefaultModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamModel any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		upstreamModel = req["model"]
		resp := map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"model":   req["model"],
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	provider := config.NewProvider(&config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			APIKey:  "key",
			Format:  "chat",
			Model:   "default-model",
		},
	}, "")

	rt := router.NewConfigRouter(provider)
	h := NewHandler(provider, nil, rt)
	engine := gin.New()
	h.RegisterRoutes(engine)

	w := doRequest(engine, "POST", "/v1/chat/completions", `{
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "default-model", upstreamModel)
}

func TestHandleResponses_ConvertedToChat(t *testing.T) {
	r, _ := setupRouter(t, "chat", chatUpstreamHandler(t))

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-4o",
		"input": "Say hello",
		"stream": false
	}`)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "list", resp["object"])

	responses := resp["responses"].([]any)
	require.Len(t, responses, 1)
	item := responses[0].(map[string]any)
	content := item["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "Hello from upstream", content[0].(map[string]any)["text"])
}

func TestHandleResponses_Passthrough(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp-test",
			"object": "response",
			"model":  "test-model",
			"output": []map[string]any{{"type": "message", "content": "direct"}},
			"usage":  map[string]any{"input_tokens": 5, "output_tokens": 3, "total_tokens": 8},
		})
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-4o",
		"input": "hi"
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "resp-test", resp["id"])
}

func TestHandleResponses_InvalidBody(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "POST", "/v1/responses", `bad json`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleAnthropic_ConvertedToChat(t *testing.T) {
	r, _ := setupRouter(t, "chat", chatUpstreamHandler(t))

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hello"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "message", resp["type"])

	content := resp["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "Hello from upstream", content[0].(map[string]any)["text"])
}

func TestHandleAnthropic_Passthrough(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "msg-test",
			"object":  "response",
			"role":    "assistant",
			"content": []map[string]any{{"type": "text", "text": "direct"}},
			"usage":   map[string]any{"input_tokens": 5, "output_tokens": 3},
		})
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "msg-test", resp["id"])
}

func TestHandleAnthropic_InvalidBody(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "POST", "/v1/messages", `{invalid}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleChat_ConvertedToAnthropic(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "test-model", req["model"])
		assert.Equal(t, float64(1024), req["max_tokens"])

		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg-converted",
			"type":        "message",
			"role":        "assistant",
			"model":       "test-model",
			"content":     []map[string]any{{"type": "text", "text": "Anthropic reply"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "chat.completion", resp["object"])

	choices := resp["choices"].([]any)
	require.Len(t, choices, 1)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	assert.Equal(t, "Anthropic reply", msg["content"])
	assert.Equal(t, "stop", choices[0].(map[string]any)["finish_reason"])
}

func TestHandleChat_ConvertedToResponses(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "test-model", req["model"])

		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp-converted",
			"object": "response",
			"model":  "test-model",
			"output": []map[string]any{{"type": "message", "content": "Responses reply"}},
			"usage":  map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		})
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleHealth(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "GET", "/health", "")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ok", resp["status"])
}

func TestHandleReload(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "POST", "/api/reload", "")

	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
}

func TestHandleAPIStatus(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "GET", "/api/status", "")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)

	upstream := resp["upstream"].(map[string]any)
	assert.Equal(t, "chat", upstream["format"])
	assert.Equal(t, "test-model", upstream["model"])
}

func TestExtractClientAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(req *http.Request)
		expected string
	}{
		{
			name: "x-api-key header",
			setup: func(req *http.Request) {
				req.Header.Set("x-api-key", "gw-zhipu")
			},
			expected: "gw-zhipu",
		},
		{
			name: "bearer token",
			setup: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer gw-deepseek")
			},
			expected: "gw-deepseek",
		},
		{
			name: "x-api-key takes priority",
			setup: func(req *http.Request) {
				req.Header.Set("x-api-key", "gw-zhipu")
				req.Header.Set("Authorization", "Bearer other")
			},
			expected: "gw-zhipu",
		},
		{
			name:     "no auth header",
			setup:    func(req *http.Request) {},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest("POST", "/test", nil)
			tt.setup(req)
			c.Request = req

			result := extractClientAPIKey(c)
			assert.Equal(t, tt.expected, result)
		})
	}
}
