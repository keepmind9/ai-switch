package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHandler_UpstreamTransportConfigured locks in the upstream HTTP client
// transport configuration: HTTP/2 enabled, a healthy idle-connection pool, the
// original ResponseHeaderTimeout preserved, and (critically) NO environment
// proxy forwarding.
func TestNewHandler_UpstreamTransportConfigured(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil)
	tr, ok := h.client.Transport.(*http.Transport)
	require.True(t, ok, "upstream client must use *http.Transport")

	assert.True(t, tr.ForceAttemptHTTP2, "ForceAttemptHTTP2 must be true so h2 is negotiated via ALPN")
	assert.Nil(t, tr.Proxy, "Proxy must be nil; ai-switch manages proxy via its own getProxyClient")
	assert.Equal(t, upstreamTimeout, tr.ResponseHeaderTimeout, "ResponseHeaderTimeout must be preserved")
	assert.Greater(t, tr.MaxIdleConnsPerHost, 2, "MaxIdleConnsPerHost must exceed the default of 2")
}

// TestNewHandler_UpstreamTransportIgnoresProxyEnv is a regression guard: even
// with HTTP_PROXY set, the default upstream client must connect directly.
// ai-switch owns proxy handling (getProxyClient) and must not inherit net/http's
// ProxyFromEnvironment, which would silently reroute traffic.
func TestNewHandler_UpstreamTransportIgnoresProxyEnv(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Point the env proxy at a closed port; if honored, the request would fail.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("http_proxy", "http://127.0.0.1:9")

	h := NewHandler(nil, nil, nil, nil, nil)
	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	require.NoError(t, err)

	resp, err := h.client.Do(req)
	require.NoError(t, err, "request must succeed (env proxy must be ignored)")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
