package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardRequest_DefaultPath(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer ts.Close()

	h := NewHandler(nil, nil)
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			APIKey:  "test-key",
			Format:  "chat",
		},
	}

	resp, err := h.forwardRequest(cfg, "/v1/chat/completions", []byte(`{}`))
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

	h := NewHandler(nil, nil)
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			Path:    "/proxy/v1/chat/completions",
			APIKey:  "test-key",
			Format:  "chat",
		},
	}

	resp, err := h.forwardRequest(cfg, "/v1/chat/completions", []byte(`{}`))
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

	h := NewHandler(nil, nil)
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL + "/",
			APIKey:  "test-key",
			Format:  "chat",
		},
	}

	resp, err := h.forwardRequest(cfg, "/v1/chat/completions", []byte(`{}`))
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

	h := NewHandler(nil, nil)
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			APIKey:  "anth-key",
			Format:  "anthropic",
		},
	}

	resp, err := h.forwardRequest(cfg, "/v1/messages", []byte(`{}`))
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

	h := NewHandler(nil, nil)
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			APIKey:  "bearer-key",
			Format:  "chat",
		},
	}

	resp, err := h.forwardRequest(cfg, "/v1/chat/completions", []byte(`{}`))
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

	h := NewHandler(nil, nil)
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: ts.URL,
			APIKey:  "key",
			Format:  "chat",
		},
	}

	resp, err := h.forwardRequest(cfg, "/v1/chat/completions", []byte(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestUpstreamPath(t *testing.T) {
	h := NewHandler(nil, nil)

	tests := []struct {
		format   string
		expected string
	}{
		{"chat", "/v1/chat/completions"},
		{"", "/v1/chat/completions"},
		{"anthropic", "/v1/messages"},
		{"responses", "/v1/responses"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			cfg := &config.Config{
				Upstream: config.UpstreamConfig{Format: tt.format},
			}
			assert.Equal(t, tt.expected, h.upstreamPath(cfg))
		})
	}
}
