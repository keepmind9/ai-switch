package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Error conversion integration tests ---

func TestHandleResponses_Upstream429_ConvertedFromChat(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded. Please retry after 60s.","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-4o",
		"input": "hi",
		"stream": false
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Rate limit exceeded. Please retry after 60s.", resp.Error.Message)
	assert.Equal(t, "rate_limit_error", resp.Error.Type)
}

func TestHandleResponses_UpstreamModelNotFound_ConvertedFromChat(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Model gpt-5 does not exist","type":"invalid_request_error","code":"model_not_found"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-5",
		"input": "hi",
		"stream": false
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Model gpt-5 does not exist", resp.Error.Message)
}

func TestHandleAnthropic_Upstream429_ConvertedFromChat(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
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
	assert.Equal(t, "Rate limit exceeded", resp.Error.Message)
	assert.Equal(t, "rate_limit_error", resp.Error.Type)
}

func TestHandleAnthropic_UpstreamError_ConvertedFromAnthropic(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Anthropic API is temporarily overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	// Same-format passthrough: raw body forwarded as-is
	assert.Contains(t, w.Body.String(), "Anthropic API is temporarily overloaded")
}

func TestHandleChat_UpstreamError_ConvertedFromAnthropic(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "invalid x-api-key", resp.Error.Message)
	assert.Equal(t, "authentication_error", resp.Error.Type)
}

func TestHandleChat_Upstream429_ConvertedFromResponses(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Requests per minute limit reached","type":"rate_limit_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Requests per minute limit reached", resp.Error.Message)
}

func TestHandleResponses_UpstreamError_ConvertedFromAnthropic(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": false
	}`)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Internal server error", resp.Error.Message)
}

func TestHandleAnthropic_UpstreamError_ConvertedFromResponses(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"Access denied","type":"permission_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/messages", `{
		"model": "claude-3-sonnet",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp types.AnthropicErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Type)
	assert.Equal(t, "Access denied", resp.Error.Message)
}

// --- Streaming error tests ---

func TestHandleResponses_StreamingNonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"error":{"message":"Model overloaded","type":"server_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	// Should get a JSON error response, not a hung SSE stream
	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Model overloaded", resp.Error.Message)
}

func TestHandleAnthropic_StreamingNonSSEError(t *testing.T) {
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

func TestHandleChat_StreamingNonSSEError(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Too many requests"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}],
		"stream": true
	}`)

	var resp types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Too many requests", resp.Error.Message)
}

func TestHandleResponses_StreamingSSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"error\":{\"message\":\"model not found\",\"type\":\"invalid_request_error\"}}\n\n"))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "test-model",
		"input": "hi",
		"stream": true
	}`)

	body := w.Body.String()
	assert.Contains(t, body, "model not found")
}

// --- Passthrough error (same format, no conversion needed) ---

func TestHandleChat_UpstreamError_Passthrough(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	})

	w := doRequest(r, "POST", "/v1/chat/completions", `{
		"model": "test-model",
		"messages": [{"role": "user", "content": "hi"}]
	}`)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "rate limited")
}

// --- Unit tests for writeConvertedError body consumption fix ---

func TestWriteConvertedError_BodyConsumedByLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstreamBody := []byte(`{"error":{"message":"429 error","type":"rate_limit_error"}}`)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write(upstreamBody)
	}))
	defer upstream.Close()

	resp, err := http.Get(upstream.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &Handler{}
	h.writeConvertedError(c, resp, respBody, "chat")

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	var chatErr types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &chatErr))
	assert.Equal(t, "429 error", chatErr.Error.Message)
}

func TestWriteConvertedError_EmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	resp := &http.Response{StatusCode: 429, Header: http.Header{}}

	h := &Handler{}
	h.writeConvertedError(c, resp, []byte{}, "chat")

	assert.Equal(t, 429, w.Code)
	var chatErr types.ChatErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &chatErr))
	assert.Equal(t, "", chatErr.Error.Message)
}

// --- Test SSE error detection with streaming Chat SSE ---

func TestHandleChat_StreamingSSEErrorEvent(t *testing.T) {
	r, _ := setupRouter(t, "anthropic", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Send normal event then error event
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
}
