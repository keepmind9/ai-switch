package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/handler"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Request builders ---

func sendAnthropicReq(t *testing.T, url, apiKey string, stream bool) *http.Response {
	t.Helper()
	body := map[string]any{
		"model":      "test-model",
		"max_tokens": 1024,
		"stream":     stream,
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}
	return doPost(t, url+"/v1/messages", apiKey, body)
}

func sendChatReq(t *testing.T, url, apiKey string, stream bool) *http.Response {
	t.Helper()
	body := map[string]any{
		"model":  "test-model",
		"stream": stream,
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	return doPost(t, url+"/v1/chat/completions", apiKey, body)
}

func sendResponsesReq(t *testing.T, url, apiKey string, stream bool) *http.Response {
	t.Helper()
	body := map[string]any{
		"model":  "test-model",
		"stream": stream,
		"input":  "Hello",
	}
	return doPost(t, url+"/v1/responses", apiKey, body)
}

func doPost(t *testing.T, url, apiKey string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// --- Protocol matrix test ---

func TestProtocolMatrix(t *testing.T) {
	clients := []struct {
		name      string
		clientFmt string
		sendReq   func(t *testing.T, url, apiKey string, stream bool) *http.Response
	}{
		{"ClaudeCode_Anthropic", "anthropic", sendAnthropicReq},
		{"OpenCode_Chat", "chat", sendChatReq},
		{"Codex_Responses", "responses", sendResponsesReq},
	}

	upstreams := []string{"chat", "anthropic", "responses"}

	for _, client := range clients {
		for _, upstream := range upstreams {
			for _, stream := range []bool{false, true} {
				name := fmt.Sprintf("%s/to_%s/stream=%v", client.name, upstream, stream)
				t.Run(name, func(t *testing.T) {
					env := setupProtocolTest(t, upstream)
					resp := client.sendReq(t, env.gateway.URL, "test-key", stream)
					defer resp.Body.Close()

					assert.Equal(t, http.StatusOK, resp.StatusCode)

					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)

					if stream {
						assert.True(t, isSSEStream(body), "expected SSE response for streaming request")
						events := parseSSEStream(body)
						assert.True(t, len(events) > 0, "expected SSE events")
						assert.True(t, hasDataContaining(events, mockResponseText),
							"expected mock response in SSE data")
						// [DONE] only in Chat SSE; Anthropic uses message_stop, Responses uses response.completed
						switch client.clientFmt {
						case "chat":
							assert.True(t, hasDone(events), "expected [DONE] in Chat SSE")
						case "anthropic":
							assert.True(t, hasEventWithType(events, "message_stop"), "expected message_stop in Anthropic SSE")
						case "responses":
							assert.True(t, hasEventWithType(events, "response.completed"), "expected response.completed in Responses SSE")
						}
					} else {
						assert.True(t, isJSONResponse(resp), "expected JSON response for non-streaming request")
						assert.Contains(t, string(body), mockResponseText,
							"expected mock response in JSON body")
					}
				})
			}
		}
	}
}

// --- Error scenarios ---

func TestErrorPassthrough(t *testing.T) {
	tests := []struct {
		name      string
		clientFmt string
		upstream  string
		sendReq   func(t *testing.T, url, apiKey string, stream bool) *http.Response
		errField  string
	}{
		{
			name:      "anthropic_client/chat_upstream",
			clientFmt: "anthropic",
			upstream:  "chat",
			sendReq:   sendAnthropicReq,
		},
		{
			name:      "chat_client/anthropic_upstream",
			clientFmt: "chat",
			upstream:  "anthropic",
			sendReq:   sendChatReq,
		},
		{
			name:      "responses_client/chat_upstream",
			clientFmt: "responses",
			upstream:  "chat",
			sendReq:   sendResponsesReq,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock that returns error
			mock := newErrorMockUpstream(tt.upstream, http.StatusBadRequest, "model not found", "invalid_request_error")
			ts := httptest.NewServer(mock)
			defer ts.Close()

			gin.SetMode(gin.TestMode)
			cfg := &config.Config{
				DefaultRoute: "test-route",
				Providers: map[string]config.ProviderConfig{
					"mock": {BaseURL: ts.URL, APIKey: "mock-key", Format: tt.upstream},
				},
				Routes: map[string]config.RouteRule{
					"test-key": {Provider: "mock", DefaultModel: "test-model"},
				},
			}
			provider := config.NewProvider(cfg, "")
			r := router.NewConfigRouter(provider)
			h := handler.NewHandler(provider, nil, r, nil)
			engine := gin.New()
			h.RegisterRoutes(engine)

			gw := httptest.NewServer(engine)
			defer gw.Close()

			resp := tt.sendReq(t, gw.URL, "test-key", false)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), "error")
		})
	}
}

func TestInvalidJSON(t *testing.T) {
	env := setupProtocolTest(t, "chat")

	req, err := http.NewRequest("POST", env.gateway.URL+"/v1/chat/completions", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDefaultRouteFallback(t *testing.T) {
	env := setupProtocolTest(t, "chat")

	// Send without API key — should fall back to default_route
	body := map[string]any{
		"model":    "test-model",
		"messages": []map[string]any{{"role": "user", "content": "Hello"}},
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", env.gateway.URL+"/v1/chat/completions", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCountTokens(t *testing.T) {
	env := setupProtocolTest(t, "chat")

	body := map[string]any{
		"model":    "test-model",
		"messages": []map[string]any{{"role": "user", "content": "Hello world"}},
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", env.gateway.URL+"/v1/messages/count_tokens", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(respBody), "input_tokens")
}

// --- Helpers ---

func isSSEStream(body []byte) bool {
	return bytes.Contains(body, []byte("data:"))
}

func isJSONResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return ct == "application/json" || bytes.Contains([]byte(ct), []byte("application/json"))
}

// errorMockUpstream returns a fixed error response.
type errorMockUpstream struct {
	format    string
	status    int
	message   string
	errorType string
}

func newErrorMockUpstream(format string, status int, message, errorType string) *errorMockUpstream {
	return &errorMockUpstream{format: format, status: status, message: message, errorType: errorType}
}

func (e *errorMockUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(e.status)
	w.Header().Set("Content-Type", "application/json")

	switch e.format {
	case "anthropic":
		json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": e.errorType, "message": e.message},
		})
	default:
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": e.message, "type": e.errorType, "code": e.errorType},
		})
	}
}
