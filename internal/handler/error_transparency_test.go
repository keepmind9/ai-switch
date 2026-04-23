package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Non-streaming error transparency tests ---

func TestErrorTransparency_ClaudeClient_ChatUpstream_429(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded. Please retry after 60s.","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "Rate limit exceeded. Please retry after 60s.", resp.Error.Message)
	assert.Equal(t, "rate_limit_error", resp.Error.Type)
}

func TestErrorTransparency_ClaudeClient_ChatUpstream_400(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Model not-exist does not exist","type":"invalid_request_error","code":"model_not_found"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "not-exist",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "Model not-exist does not exist", resp.Error.Message)
	assert.Equal(t, "invalid_request_error", resp.Error.Type)
}

func TestErrorTransparency_ChatClient_ChatUpstream_401(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"Invalid API key","type":"authentication_error","code":"invalid_api_key"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "gpt-4o",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid API key")
}

func TestErrorTransparency_ChatClient_AnthropicUpstream_500(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Internal server error", resp.Error.Message)
	assert.Equal(t, "api_error", resp.Error.Type)
}

// --- Streaming error transparency tests ---

// Scenario: Claude client → Chat upstream, upstream returns 200 + JSON error body (not SSE)
func TestErrorTransparency_ClaudeStreaming_NonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":{"message":"Context length exceeded","type":"invalid_request_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "Context length exceeded", resp.Error.Message)
}

// Scenario: Chat client → Anthropic upstream, upstream returns 200 + JSON error body (not SSE)
func TestErrorTransparency_ChatStreaming_NonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Server is overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Server is overloaded", resp.Error.Message)
}

// Scenario: Claude client → Chat upstream, upstream returns SSE with error event as first event
func TestErrorTransparency_ClaudeStreaming_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"error\":{\"message\":\"model not found\",\"type\":\"invalid_request_error\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	// Claude client (anthropic format) → SSE error event flows through SSE stream
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "model not found")
	assert.Contains(t, w.Body.String(), "event: error")
}

// Scenario: Chat client → Anthropic upstream, upstream returns SSE with error event
func TestErrorTransparency_ChatStreaming_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_start\n"))
		w.Write([]byte(fmt.Sprintf("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"test\",\"usage\":{\"input_tokens\":5}}}\n\n")))
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server is overloaded\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "Server is overloaded")
	assert.Contains(t, body, `"error"`)
}

// --- Passthrough streaming error tests ---

// Scenario: Chat client → Chat upstream (passthrough), upstream returns 200 + JSON error body
func TestErrorTransparency_ChatPassthroughStreaming_NonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":{"message":"Model overloaded","type":"server_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	// Should return a proper error, not a malformed SSE stream
	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Model overloaded", resp.Error.Message)
}

// Scenario: Claude client → Anthropic upstream (passthrough), upstream returns 200 + JSON error body
func TestErrorTransparency_ClaudePassthroughStreaming_NonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Server overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "Server overloaded", resp.Error.Message)
}

// --- Upstream non-200 status code in streaming path ---

// Scenario: Claude client → Chat upstream, upstream returns 429 (non-200) in streaming request
func TestErrorTransparency_ClaudeStreaming_Upstream429(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "Rate limit exceeded", resp.Error.Message)
}

// Scenario: Chat client → Anthropic upstream, upstream returns 401 in streaming request
func TestErrorTransparency_ChatStreaming_Upstream401(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "invalid x-api-key", resp.Error.Message)
	assert.Equal(t, "authentication_error", resp.Error.Type)
}

// --- Streaming passthrough with SSE error event ---

// Scenario: Chat client → Chat upstream (passthrough), upstream sends SSE error event mid-stream
func TestErrorTransparency_ChatPassthroughStreaming_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
		w.Write([]byte("data: {\"error\":{\"message\":\"model overloaded\",\"type\":\"server_error\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	body := w.Body.String()
	// Passthrough should forward the error event as-is
	assert.Contains(t, body, "model overloaded")
}

// Scenario: Claude client → Anthropic upstream (passthrough), upstream sends SSE error event
func TestErrorTransparency_ClaudePassthroughStreaming_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message_start\n"))
		w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"test\",\"usage\":{\"input_tokens\":5}}}\n\n"))
		w.Write([]byte("event: error\n"))
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server overloaded\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	body := w.Body.String()
	// Passthrough should forward the error event as-is
	assert.Contains(t, body, "Server overloaded")
}

// --- Responses protocol error tests ---

func TestErrorTransparency_ResponsesClient_ChatUpstream_400(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid model","type":"invalid_request_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "bad-model",
		"input": "hi",
		"stream": false
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Invalid model", resp.Error.Message)
}

// --- Connection error tests ---

func TestErrorTransparency_UpstreamUnreachable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a config pointing to a non-existent upstream
	provider := newTestConfig("http://127.0.0.1:1", "chat", "test-model")
	p := config.NewProvider(provider, "")
	rt := router.NewConfigRouter(p)
	h := NewHandler(p, nil, rt, nil)
	engine := gin.New()
	h.RegisterRoutes(engine)

	w := doRequest(engine, "POST", "/v1/messages", `{
		"model": "test",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

// --- Table-driven test for all protocol combos ---

func TestErrorTransparency_AllProtocolCombos_NonStreaming(t *testing.T) {
	tests := []struct {
		name           string
		upstreamFormat string
		endpoint       string
		body           string
		upstreamStatus int
		upstreamBody   string
		wantStatus     int
		wantMsg        string
	}{
		{
			name:           "Claude→Chat 429",
			upstreamFormat: "chat",
			endpoint:       "/v1/messages",
			body:           `{"model":"claude-3","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`,
			upstreamStatus: 429,
			upstreamBody:   `{"error":{"message":"rate limited","type":"rate_limit_error"}}`,
			wantStatus:     429,
			wantMsg:        "rate limited",
		},
		{
			name:           "Chat→Anthropic 401",
			upstreamFormat: "anthropic",
			endpoint:       "/v1/chat/completions",
			body:           `{"model":"test","messages":[{"role":"user","content":"hi"}]}`,
			upstreamStatus: 401,
			upstreamBody:   `{"type":"error","error":{"type":"auth_error","message":"bad key"}}`,
			wantStatus:     401,
			wantMsg:        "bad key",
		},
		{
			name:           "Responses→Chat 400",
			upstreamFormat: "chat",
			endpoint:       "/v1/responses",
			body:           `{"model":"test","input":"hi"}`,
			upstreamStatus: 400,
			upstreamBody:   `{"error":{"message":"bad request","type":"invalid_request_error"}}`,
			wantStatus:     400,
			wantMsg:        "bad request",
		},
		{
			name:           "Chat→Responses 403",
			upstreamFormat: "responses",
			endpoint:       "/v1/chat/completions",
			body:           `{"model":"test","messages":[{"role":"user","content":"hi"}]}`,
			upstreamStatus: 403,
			upstreamBody:   `{"error":{"message":"forbidden","type":"permission_error"}}`,
			wantStatus:     403,
			wantMsg:        "forbidden",
		},
		{
			name:           "Responses→Anthropic 500",
			upstreamFormat: "anthropic",
			endpoint:       "/v1/responses",
			body:           `{"model":"test","input":"hi"}`,
			upstreamStatus: 500,
			upstreamBody:   `{"type":"error","error":{"type":"api_error","message":"internal error"}}`,
			wantStatus:     500,
			wantMsg:        "internal error",
		},
		{
			name:           "Claude→Responses 503",
			upstreamFormat: "responses",
			endpoint:       "/v1/messages",
			body:           `{"model":"claude-3","max_tokens":1024,"messages":[{"role":"user","content":"hi"}]}`,
			upstreamStatus: 503,
			upstreamBody:   `{"error":{"message":"unavailable","type":"server_error"}}`,
			wantStatus:     503,
			wantMsg:        "unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := setupRouter(t, tt.upstreamFormat, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.upstreamStatus)
				w.Write([]byte(tt.upstreamBody))
			})

			w := doRequest(r, "POST", tt.endpoint, tt.body)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.wantMsg)
		})
	}
}

// --- Table-driven test for streaming error events ---

func TestErrorTransparency_AllProtocolCombos_StreamingSSEError(t *testing.T) {
	tests := []struct {
		name           string
		upstreamFormat string
		endpoint       string
		body           string
		upstreamSSE    string
		wantMsg        string
	}{
		{
			name:           "Claude→Chat SSE error",
			upstreamFormat: "chat",
			endpoint:       "/v1/messages",
			body:           `{"model":"claude-3","max_tokens":1024,"messages":[{"role":"user","content":"hi"}],"stream":true}`,
			upstreamSSE:    "data: {\"error\":{\"message\":\"model overloaded\",\"type\":\"server_error\"}}\n\n",
			wantMsg:        "model overloaded",
		},
		{
			name:           "Responses→Chat SSE error",
			upstreamFormat: "chat",
			endpoint:       "/v1/responses",
			body:           `{"model":"test","input":"hi","stream":true}`,
			upstreamSSE:    "data: {\"error\":{\"message\":\"rate limited\",\"type\":\"rate_limit_error\"}}\n\n",
			wantMsg:        "rate limited",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := setupRouter(t, tt.upstreamFormat, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write([]byte(tt.upstreamSSE))
			})

			w := doRequest(r, "POST", tt.endpoint, tt.body)

			body := w.Body.String()
			assert.Contains(t, body, tt.wantMsg)
		})
	}
}

// --- Verify error format matches what AI CLIs expect ---

func TestErrorTransparency_ClaudeClient_ErrorFormat(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad request","type":"invalid_request_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	// Verify exact structure Claude Code expects
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, "error", raw["type"])

	errObj, ok := raw["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "invalid_request_error", errObj["type"])
	assert.Equal(t, "bad request", errObj["message"])
}

func TestErrorTransparency_ChatClient_ErrorFormat(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	// Verify exact structure Chat CLI expects
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))

	errObj, ok := raw["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "slow down", errObj["message"])
	assert.Equal(t, "rate_limit_error", errObj["type"])
}

// --- Streaming error with non-200 status code from upstream ---

func TestErrorTransparency_ChatPassthroughStreaming_UpstreamNon200(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	// Non-200 should be caught before streaming starts
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "rate limited")
}

func TestErrorTransparency_ClaudePassthroughStreaming_UpstreamNon200(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "overloaded")
}

// --- Helper to check SSE event format ---

func TestErrorTransparency_SSEErrorFormat_ClaudeClient(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"error\":{\"message\":\"model not found\",\"type\":\"invalid_request_error\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	// Claude client (anthropic format) → SSE error event flows through SSE stream
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "model not found")
	assert.Contains(t, w.Body.String(), "event: error")
}

func TestErrorTransparency_SSEErrorFormat_ChatClient(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server overloaded\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "Server overloaded")
	// Chat format error SSE: data: {"error":{"message":"..."}}
	assert.Contains(t, body, `"error"`)
}

// --- Verify headers and content type for error responses ---

func TestErrorTransparency_ContentType_ClaudeError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad","type":"invalid_request_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
}

func TestErrorTransparency_ContentType_ChatError(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
}

// --- GLM-style error format: non-standard {"error":{"code":"1113","message":"..."}}

func TestErrorTransparency_GLM429_ClaudePassthrough(t *testing.T) {
	// Exact reproduction of the real log: GLM returns 429 with code+message format,
	// client is Claude Code expecting Anthropic error format, path is anthropic passthrough.
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"1113","message":"余额不足或无可用资源包,请充值。"},"request_id":"20260423122526f6147f2fafb94c52"}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type, "top-level type must be 'error'")
	assert.Contains(t, resp.Error.Message, "余额不足", "error message must contain upstream message")
	assert.NotEmpty(t, resp.Error.Type, "error type must not be empty")
}

func TestErrorTransparency_GLM429_CodexPassthrough(t *testing.T) {
	// Codex (Responses API) client hitting GLM upstream via responses passthrough.
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"1113","message":"余额不足或无可用资源包,请充值。"},"request_id":"req_123"}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi"
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "response must have error object")
	assert.Contains(t, errObj["message"], "余额不足", "error message must contain upstream message")
}

func TestErrorTransparency_GLM429_ClaudePassthrough_Streaming(t *testing.T) {
	// GLM returns 429 with non-standard format in a streaming request.
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"1113","message":"余额不足或无可用资源包,请充值。"},"request_id":"req_456"}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Contains(t, resp.Error.Message, "余额不足")
}

func TestErrorTransparency_GLM429_CodexPassthrough_Streaming(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":"1113","message":"余额不足或无可用资源包,请充值。"},"request_id":"req_789"}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "余额不足")
}

// --- Edge case: passthrough streaming with upstream returning 200 + JSON error ---

func TestErrorTransparency_ChatPassthroughStreaming_ContentTypeOnError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":{"message":"Model overloaded","type":"server_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	// Should return JSON error, NOT text/event-stream with raw JSON body
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json",
		"passthrough streaming should detect non-SSE upstream and return JSON error")
}

func TestErrorTransparency_ClaudePassthroughStreaming_ContentTypeOnError(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Server overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	// Should return JSON error, NOT text/event-stream with raw JSON body
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json",
		"passthrough streaming should detect non-SSE upstream and return JSON error")
}

// --- Edge case: Responses client → Anthropic upstream with SSE error event ---

func TestErrorTransparency_ResponsesStreaming_AnthropicUpstream_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server overloaded\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "Server overloaded",
		"Responses client should see error from Anthropic upstream SSE")
}

// --- Edge case: Claude client → Responses upstream with SSE error event ---

func TestErrorTransparency_ClaudeStreaming_ResponsesUpstream_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Server overloaded\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "Server overloaded",
		"Claude client should see error from Responses upstream SSE")
}

// --- Edge case: Responses client → Anthropic upstream, non-SSE error (200 + JSON) ---

func TestErrorTransparency_ResponsesStreaming_AnthropicUpstream_NonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Server overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	// Should get a proper JSON error, not a malformed SSE stream
	body := w.Body.String()
	assert.Contains(t, body, "Server overloaded")
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json",
		"non-SSE error in streaming path should return JSON")
}

// --- Edge case: Chat client → Responses upstream with SSE error event ---

func TestErrorTransparency_ChatStreaming_ResponsesUpstream_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: response.created\n"))
		w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\n\n"))
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"model overloaded\",\"type\":\"server_error\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "model overloaded",
		"Chat client should see error from Responses upstream SSE")
}

// --- Codex (Responses) passthrough streaming error tests ---

func TestErrorTransparency_CodexPassthroughStreaming_NonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":{"message":"Model overloaded","type":"server_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	assert.Contains(t, w.Header().Get("Content-Type"), "application/json",
		"Codex passthrough streaming should detect non-SSE upstream")
	assert.Contains(t, w.Body.String(), "Model overloaded")
}

func TestErrorTransparency_CodexPassthroughStreaming_SSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: response.created\n"))
		w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\"}}\n\n"))
		w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"rate limited\",\"type\":\"rate_limit_error\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "rate limited",
		"Codex passthrough should forward SSE error event")
}

func TestErrorTransparency_CodexPassthroughStreaming_UpstreamNon200(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "rate limited")
}

// --- SSE first event error → status code mapping ---

func TestErrorTransparency_SSEFirstEventError_StatusMapping(t *testing.T) {
	tests := []struct {
		name           string
		upstreamFormat string
		endpoint       string
		body           string
		upstreamSSE    string
		wantStatus     int
		wantMsg        string
	}{
		{
			name:           "overloaded → 503 (Codex client)",
			upstreamFormat: "anthropic",
			endpoint:       "/v1/responses",
			body:           `{"model":"test","input":"hi","stream":true}`,
			upstreamSSE:    "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"server overloaded\"}}\n\n",
			wantStatus:     http.StatusServiceUnavailable,
			wantMsg:        "server overloaded",
		},
		{
			name:           "auth_error → 401 (Chat client)",
			upstreamFormat: "anthropic",
			endpoint:       "/v1/chat/completions",
			body:           `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":true}`,
			upstreamSSE:    "data: {\"type\":\"error\",\"error\":{\"type\":\"authentication_error\",\"message\":\"invalid key\"}}\n\n",
			wantStatus:     http.StatusUnauthorized,
			wantMsg:        "invalid key",
		},
		{
			name:           "unknown error → 502 (Codex passthrough)",
			upstreamFormat: "responses",
			endpoint:       "/v1/responses",
			body:           `{"model":"test","input":"hi","stream":true}`,
			upstreamSSE:    "data: {\"error\":{\"message\":\"余额不足\",\"code\":\"1113\"}}\n\n",
			wantStatus:     http.StatusBadGateway,
			wantMsg:        "余额不足",
		},
		{
			name:           "server_error → 500 (Chat client)",
			upstreamFormat: "chat",
			endpoint:       "/v1/chat/completions",
			body:           `{"model":"test","messages":[{"role":"user","content":"hi"}],"stream":true}`,
			upstreamSSE:    "data: {\"error\":{\"message\":\"internal error\",\"type\":\"server_error\"}}\n\n",
			wantStatus:     http.StatusInternalServerError,
			wantMsg:        "internal error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := setupRouter(t, tt.upstreamFormat, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write([]byte(tt.upstreamSSE))
			})

			w := doRequest(r, "POST", tt.endpoint, tt.body)

			assert.Equal(t, tt.wantStatus, w.Code, "status code should map from error type")
			assert.Contains(t, w.Body.String(), tt.wantMsg, "response should contain error message")
		})
	}
}
