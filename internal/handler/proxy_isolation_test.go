package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// protocolRouter routes by client protocol, so one handler can be exercised
// against different upstream formats in a single test (unlike staticRouter,
// which always returns the same result regardless of protocol).
type protocolRouter struct {
	byProtocol map[string]*router.RouteResult
}

func (r *protocolRouter) Route(protocol, _ string, _ []byte) (*router.RouteResult, error) {
	if result, ok := r.byProtocol[protocol]; ok {
		return result, nil
	}
	return nil, fmt.Errorf("no route for protocol %q", protocol)
}

// TestProxyModes_PerProtocolIsolation verifies proxy mode is applied per client
// protocol: with only claude (anthropic) enabled, a Claude Code request is
// byte-passthrough'd to an anthropic upstream, while a Codex request still
// flows through the conversion pipeline to a chat upstream instead of being
// rejected by the proxy same-format guard.
func TestProxyModes_PerProtocolIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var anthGotBody []byte
	anthUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anthGotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}]}`))
	}))
	t.Cleanup(anthUpstream.Close)

	chatCalled := false
	chatUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(chatUpstream.Close)

	rt := &protocolRouter{byProtocol: map[string]*router.RouteResult{
		converter.FormatAnthropic: {ProviderKey: "anth", BaseURL: anthUpstream.URL, Path: "/v1/messages", APIKey: "sk", Format: "anthropic", Model: "claude"},
		converter.FormatResponses: {ProviderKey: "chat", BaseURL: chatUpstream.URL, Path: "/v1/chat/completions", APIKey: "sk", Format: "chat", Model: "gpt-4"},
	}}

	// Only claude in proxy mode; codex (responses) uses the conversion pipeline.
	h := NewHandler(nil, nil, rt, nil, ProxyModes{converter.FormatAnthropic: true})
	engine := gin.New()
	h.RegisterRoutes(engine)

	// Claude Code: anthropic -> anthropic upstream via proxy passthrough.
	w := doRequest(engine, "POST", "/v1/messages",
		`{"model":"claude","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"id":"msg_1"`, "anthropic response forwarded verbatim (proxy on)")
	// Proxy passthrough preserves the request body (bar the model rewrite).
	assert.Contains(t, string(anthGotBody), `"role":"user"`)

	// Codex: responses -> chat upstream via conversion (proxy off for responses).
	w = doRequest(engine, "POST", "/v1/responses",
		`{"model":"gpt-4","stream":false,"input":"hi"}`)
	require.True(t, chatCalled, "chat upstream must be reached (responses converted, not proxy-rejected)")
	assert.NotContains(t, w.Body.String(), "proxy_mode_format_mismatch",
		"responses must not hit the proxy same-format guard")
}
