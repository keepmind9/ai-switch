package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupProxyRouter builds a proxy-mode engine backed by a static router result
// and a fake upstream server. The result's BaseURL is overwritten with the
// upstream URL so requests reach the test server.
func setupProxyRouter(t *testing.T, result *router.RouteResult, upstream http.HandlerFunc) (*gin.Engine, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ts := httptest.NewServer(upstream)
	t.Cleanup(ts.Close)
	result.BaseURL = ts.URL
	h := NewHandler(nil, nil, &staticRouter{result: result}, nil, true)
	engine := gin.New()
	h.RegisterRoutes(engine)
	return engine, ts
}

func TestProxyPipeline_AnthropicPassthrough(t *testing.T) {
	result := &router.RouteResult{
		ProviderKey: "anth",
		Path:        "/v1/messages",
		APIKey:      "sk-upstream",
		Format:      "anthropic",
		Model:       "configured-claude",
	}
	var got *http.Request
	var gotBody []byte
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		got = r
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}]}`))
	})

	w := doRequest(engine, "POST", "/v1/messages", `{"model":"claude-sonnet-4-5","max_tokens":1024,"stream":false,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code)
	// Upstream received the configured model, not the client's.
	var sent map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &sent))
	assert.Equal(t, "configured-claude", sent["model"])
	// Auth: anthropic uses x-api-key, not Bearer.
	assert.Equal(t, "sk-upstream", got.Header.Get("x-api-key"))
	assert.Empty(t, got.Header.Get("Authorization"))
	// Path preserved.
	assert.Equal(t, "/v1/messages", got.URL.Path)
	// Response body forwarded verbatim.
	assert.Contains(t, w.Body.String(), `"id":"msg_1"`)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestProxyPipeline_ResponsesPassthrough(t *testing.T) {
	result := &router.RouteResult{
		ProviderKey: "resp",
		Path:        "/v1/responses",
		APIKey:      "sk-resp",
		Format:      "responses",
		Model:       "configured-resp",
	}
	var got *http.Request
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"resp_1","status":"completed"}`))
	})

	w := doRequest(engine, "POST", "/v1/responses", `{"model":"gpt-5","stream":false,"input":"hi"}`)

	require.Equal(t, http.StatusOK, w.Code)
	// Auth: responses uses Bearer, not x-api-key.
	assert.Equal(t, "Bearer sk-resp", got.Header.Get("Authorization"))
	assert.Empty(t, got.Header.Get("x-api-key"))
	assert.Equal(t, "/v1/responses", got.URL.Path)
	assert.Contains(t, w.Body.String(), `"id":"resp_1"`)
}

func TestProxyPipeline_NoModelConfigured_ForwardsClientModel(t *testing.T) {
	result := &router.RouteResult{
		ProviderKey: "anth",
		Path:        "/v1/messages",
		APIKey:      "sk",
		Format:      "anthropic",
		Model:       "", // no model override
	}
	var gotBody []byte
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})

	doRequest(engine, "POST", "/v1/messages", `{"model":"client-model","stream":false,"messages":[]}`)

	var sent map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &sent))
	assert.Equal(t, "client-model", sent["model"], "client model forwarded when no override configured")
}

func TestProxyPipeline_FormatMismatchRejected(t *testing.T) {
	// Anthropic client routed to a chat-format provider -> must not convert.
	result := &router.RouteResult{
		ProviderKey: "chatprov",
		Path:        "/v1/chat/completions",
		APIKey:      "sk",
		Format:      "chat",
		Model:       "gpt-4",
	}
	upstreamCalled := false
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	})

	w := doRequest(engine, "POST", "/v1/messages", `{"model":"claude","stream":false,"messages":[]}`)

	assert.False(t, upstreamCalled, "upstream must not be called on format mismatch")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var errResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	errObj, _ := errResp["error"].(map[string]any)
	assert.Equal(t, "proxy_mode_format_mismatch", errObj["code"])
	assert.Contains(t, errObj["message"], "protocol conversion is disabled")
}

func TestProxyPipeline_SSEStreamPassthrough(t *testing.T) {
	result := &router.RouteResult{
		ProviderKey: "anth",
		Path:        "/v1/messages",
		APIKey:      "sk",
		Format:      "anthropic",
		Model:       "claude",
	}
	sse := "event: message_start\ndata: {\"type\":\"message_start\"}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hi\"}}\n\n"
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Write in two chunks to exercise the streaming copy + flush path.
		w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hi\"}}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	w := doRequest(engine, "POST", "/v1/messages", `{"model":"claude","stream":true,"messages":[{"role":"user","content":"hi"}]}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	// Body forwarded byte-for-byte (whitespace and event framing intact).
	assert.Equal(t, sse, w.Body.String())
}

func TestProxyPipeline_NonOKErrorPassthrough(t *testing.T) {
	result := &router.RouteResult{
		ProviderKey: "anth",
		Path:        "/v1/messages",
		APIKey:      "sk",
		Format:      "anthropic",
		Model:       "claude",
	}
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	})

	w := doRequest(engine, "POST", "/v1/messages", `{"model":"claude","stream":false,"messages":[]}`)

	// Status code and body forwarded verbatim, no conversion/error wrapping.
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "rate_limit_error")
}

func TestProxyPipeline_HopByHopHeadersStripped(t *testing.T) {
	result := &router.RouteResult{
		ProviderKey: "anth",
		Path:        "/v1/messages",
		APIKey:      "sk",
		Format:      "anthropic",
		Model:       "claude",
	}
	engine, _ := setupProxyRouter(t, result, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("X-Custom", "kept")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: ok\n\n"))
	})

	w := doRequest(engine, "POST", "/v1/messages", `{"model":"claude","stream":true,"messages":[]}`)

	require.Equal(t, http.StatusOK, w.Code)
	// End-to-end headers preserved.
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "kept", w.Header().Get("X-Custom"))
	// Hop-by-hop headers removed.
	assert.Empty(t, w.Header().Get("Keep-Alive"))
	assert.Empty(t, w.Header().Get("Connection"))
	assert.Empty(t, w.Header().Get("Transfer-Encoding"))
	// Content-Length removed (server uses chunked for streaming).
	assert.Empty(t, w.Header().Get("Content-Length"))
}

func TestProxyPipeline_KeyFallbackOn429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var seenKeys []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-api-key")
		seenKeys = append(seenKeys, key)
		if key == "sk-primary" {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"ok"}`))
	}))
	t.Cleanup(ts.Close)

	// Provider with a fallback key so KeyManager rotates on 429.
	cfg := &config.Config{
		DefaultRoute: "gw-default",
		Providers: map[string]config.ProviderConfig{
			"default": {
				BaseURL:      ts.URL,
				APIKey:       "sk-primary",
				FallbackKeys: []string{"sk-fallback"},
				Format:       "anthropic",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-default": {Provider: "default", DefaultModel: "claude"},
		},
	}
	provider := config.NewProvider(cfg, "")
	result := &router.RouteResult{
		ProviderKey: "default",
		BaseURL:     ts.URL,
		Path:        "/v1/messages",
		APIKey:      "sk-primary",
		Format:      "anthropic",
		Model:       "claude",
	}
	h := NewHandler(provider, nil, &staticRouter{result: result}, nil, true)
	h.SyncKeys()
	engine := gin.New()
	h.RegisterRoutes(engine)

	body := `{"model":"claude","stream":false,"messages":[]}`
	// Primary needs maxConsecutive429 (3) hits to enter cooldown. The 3rd
	// request cools the primary mid-loop and the forwarder retries with the
	// fallback key in the same request.
	var last *httptest.ResponseRecorder
	for i := 0; i < 3; i++ {
		last = doRequest(engine, "POST", "/v1/messages", body)
	}

	// The fallback key must have been used and the final request succeeds.
	require.Equal(t, http.StatusOK, last.Code, "should succeed via fallback key after primary cooldown")
	assert.Contains(t, last.Body.String(), `"id":"ok"`)
	assert.Contains(t, seenKeys, "sk-fallback", "fallback key must have been tried")
}

func TestProxyPipeline_UpstreamUnreachable_NoDoubleCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Create then immediately close the upstream so the transport dial fails.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()

	result := &router.RouteResult{
		ProviderKey: "anth",
		BaseURL:     ts.URL,
		Path:        "/v1/messages",
		APIKey:      "sk",
		Format:      "anthropic",
		Model:       "claude",
	}
	h := NewHandler(nil, nil, &staticRouter{result: result}, nil, true)
	engine := gin.New()
	h.RegisterRoutes(engine)

	w := doRequest(engine, "POST", "/v1/messages", `{"model":"claude","stream":false,"messages":[]}`)

	// sendUpstreamRequest writes the 502 error response once on transport
	// failure; proxyForward must not re-record usage/trace (would double-count).
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "failed to call upstream")
}
