package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- httpClientFor tests ---

func TestHttpClientFor_NoProxyURL(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"test": {EnableProxy: true},
		},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client := h.httpClientFor("test")
	assert.Equal(t, h.client, client, "should return default client when no proxy_url configured")
}

func TestHttpClientFor_ProviderNotEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{ProxyURL: "socks5://127.0.0.1:1080"},
		Providers: map[string]config.ProviderConfig{
			"test": {EnableProxy: false},
		},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client := h.httpClientFor("test")
	assert.Equal(t, h.client, client, "should return default client when provider has enable_proxy=false")
}

func TestHttpClientFor_ProviderEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{ProxyURL: "http://127.0.0.1:8080"},
		Providers: map[string]config.ProviderConfig{
			"test": {EnableProxy: true},
		},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client := h.httpClientFor("test")
	assert.NotNil(t, client)
	assert.NotEqual(t, h.client, client, "should return proxy client when provider has enable_proxy=true")
}

func TestHttpClientFor_ProviderNotFound(t *testing.T) {
	cfg := &config.Config{
		Server:    config.ServerConfig{ProxyURL: "http://127.0.0.1:8080"},
		Providers: map[string]config.ProviderConfig{},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client := h.httpClientFor("nonexistent")
	assert.Equal(t, h.client, client, "should return default client for unknown provider")
}

func TestHttpClientFor_CachesProxyClient(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{ProxyURL: "http://127.0.0.1:8080"},
		Providers: map[string]config.ProviderConfig{
			"test": {EnableProxy: true},
		},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client1 := h.httpClientFor("test")
	client2 := h.httpClientFor("test")
	assert.Same(t, client1, client2, "should cache and reuse the same proxy client")
}

func TestGetProxyClient_ProxyURLChangeRebuildsClient(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{ProxyURL: "http://127.0.0.1:8080"},
		Providers: map[string]config.ProviderConfig{
			"test": {EnableProxy: true},
		},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client1 := h.httpClientFor("test")
	require.NotNil(t, client1)

	// Simulate proxy URL change via new provider
	cfg.Server.ProxyURL = "http://127.0.0.1:9090"
	provider2 := config.NewProvider(cfg, "")
	h.provider = provider2

	client2 := h.httpClientFor("test")
	require.NotNil(t, client2)
	assert.NotSame(t, client1, client2, "should create new client when proxy URL changes")
}

func TestGetProxyClient_InvalidURL(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{ProxyURL: "://invalid"},
	}
	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	client := h.getProxyClient("://invalid")
	assert.Equal(t, h.client, client, "should fall back to default client on invalid proxy URL")
}

func TestForwardRequest_UsesProxy(t *testing.T) {
	var gotProxy string
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProxy = r.URL.String()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer proxyServer.Close()

	// Target upstream that records if it was called directly
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{ProxyURL: proxyServer.URL},
		Providers: map[string]config.ProviderConfig{
			"test": {
				BaseURL:     upstream.URL,
				APIKey:      "test-key",
				Format:      "chat",
				EnableProxy: true,
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {Provider: "test", DefaultModel: "gpt-4"},
		},
		DefaultRoute: "gw-test",
	}

	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{
		result: &router.RouteResult{
			ProviderKey: "test",
			BaseURL:     upstream.URL,
			Path:        "/v1/chat/completions",
			APIKey:      "test-key",
			Format:      "chat",
			Model:       "gpt-4",
		},
	}, nil)

	_, _, err := h.forwardRequest(context.Background(), &router.RouteResult{
		ProviderKey: "test",
		BaseURL:     upstream.URL,
		Path:        "/v1/chat/completions",
		APIKey:      "test-key",
		Format:      "chat",
	}, []byte(`{"model":"gpt-4","messages":[]}`))

	require.NoError(t, err)
	assert.False(t, upstreamCalled, "upstream should NOT be called directly")
	assert.Contains(t, gotProxy, "chat/completions", "request should go through proxy")
}

func TestForwardRequest_NoProxy(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"test": {
				BaseURL: upstream.URL,
				APIKey:  "test-key",
				Format:  "chat",
			},
		},
	}

	provider := config.NewProvider(cfg, "")
	h := NewHandler(provider, nil, &staticRouter{}, nil)

	_, _, err := h.forwardRequest(context.Background(), &router.RouteResult{
		ProviderKey: "test",
		BaseURL:     upstream.URL,
		Path:        "/v1/chat/completions",
		APIKey:      "test-key",
		Format:      "chat",
	}, []byte(`{"model":"gpt-4","messages":[]}`))

	require.NoError(t, err)
	assert.True(t, upstreamCalled, "upstream should be called directly without proxy")
}
