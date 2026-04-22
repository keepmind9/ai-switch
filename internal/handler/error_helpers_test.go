package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestParseUpstreamError_ChatFormat(t *testing.T) {
	body, _ := json.Marshal(types.ChatErrorResponse{
		Error: &types.ChatErrorDetail{
			Message: "rate limit exceeded",
			Type:    "rate_limit_error",
			Code:    "rate_limit_exceeded",
		},
	})
	msg, errType := parseUpstreamError(body)
	assert.Equal(t, "rate limit exceeded", msg)
	assert.Equal(t, "rate_limit_error", errType)
}

func TestParseUpstreamError_AnthropicFormat(t *testing.T) {
	body, _ := json.Marshal(types.AnthropicErrorResponse{
		Type: "error",
		Error: &types.AnthropicErrorDetail{
			Type:    "not_found_error",
			Message: "model: unknown-model",
		},
	})
	msg, errType := parseUpstreamError(body)
	assert.Equal(t, "model: unknown-model", msg)
	assert.Equal(t, "not_found_error", errType)
}

func TestParseUpstreamError_UnknownFormat(t *testing.T) {
	msg, errType := parseUpstreamError([]byte("plain text error"))
	assert.Equal(t, "plain text error", msg)
	assert.Empty(t, errType)
}

func TestWriteConvertedError_ChatClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Simulate Anthropic upstream returning an error
	upstreamBody, _ := json.Marshal(types.AnthropicErrorResponse{
		Type: "error",
		Error: &types.AnthropicErrorDetail{
			Type:    "rate_limit_error",
			Message: "Too many requests",
		},
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write(upstreamBody)
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	resp, _ := http.NewRequest("POST", upstream.URL, nil)
	c.Request = resp
	upstreamResp, _ := http.DefaultClient.Do(resp)

	h := &Handler{}
	h.writeConvertedError(c, upstreamResp, "chat")

	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	var chatErr types.ChatErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &chatErr)
	assert.NoError(t, err)
	assert.Equal(t, "Too many requests", chatErr.Error.Message)
	assert.Equal(t, "rate_limit_error", chatErr.Error.Type)
}

func TestWriteConvertedError_AnthropicClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Simulate Chat upstream returning an error
	upstreamBody, _ := json.Marshal(types.ChatErrorResponse{
		Error: &types.ChatErrorDetail{
			Message: "model not found",
			Type:    "invalid_request_error",
		},
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(upstreamBody)
	}))
	defer upstream.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	resp, _ := http.NewRequest("POST", upstream.URL, nil)
	c.Request = resp
	upstreamResp, _ := http.DefaultClient.Do(resp)

	h := &Handler{}
	h.writeConvertedError(c, upstreamResp, "anthropic")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var anthErr types.AnthropicErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &anthErr)
	assert.NoError(t, err)
	assert.Equal(t, "error", anthErr.Type)
	assert.Equal(t, "model not found", anthErr.Error.Message)
	assert.Equal(t, "invalid_request_error", anthErr.Error.Type)
}

func TestIsSSEResponse(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expected    bool
	}{
		{"SSE", "text/event-stream", true},
		{"SSE with charset", "text/event-stream; charset=utf-8", true},
		{"JSON", "application/json", false},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{"Content-Type": {tt.contentType}}}
			assert.Equal(t, tt.expected, isSSEResponse(resp))
		})
	}
}

func TestIsSSEErrorData(t *testing.T) {
	assert.True(t, isSSEErrorData(`{"error":{"message":"rate limited"}}`))
	assert.True(t, isSSEErrorData(`{"type":"error","error":{"message":"model not found"}}`))
	assert.False(t, isSSEErrorData(`{"type":"message_start","message":{}}`))
	assert.False(t, isSSEErrorData("[DONE]"))
	assert.False(t, isSSEErrorData(""))
}
