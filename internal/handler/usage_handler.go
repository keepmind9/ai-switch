package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/usage"
)

// usageQueryTimeout bounds a single usage query.
const usageQueryTimeout = 10 * time.Second

// queryUsage is swappable for tests.
var queryUsage = usage.Query

// getUsage handles GET /admin/usage/:key — queries the coding-plan usage
// window of the given provider (currently GLM only).
func (a *AdminHandler) getUsage(c *gin.Context) {
	key := c.Param("key")

	cfg := a.provider.Get()
	p, ok := cfg.Providers[key]
	if !ok {
		sendFail(c, http.StatusNotFound, CodeNotFound, "provider not found")
		return
	}

	if !usage.Supported(p.BaseURL) {
		sendFail(c, http.StatusBadRequest, CodeBadRequest, "usage query is not supported for this provider's base_url")
		return
	}

	// Prefer the primary key; fall back to the first fallback key.
	apiKey := p.APIKey
	if apiKey == "" && len(p.FallbackKeys) > 0 {
		apiKey = p.FallbackKeys[0]
	}
	if apiKey == "" {
		sendFail(c, http.StatusBadRequest, CodeBadRequest, "provider has no API key configured")
		return
	}

	info, err := queryUsage(c.Request.Context(), usageHTTPClient(cfg, p), p.BaseURL, apiKey)
	if err != nil {
		slog.Warn("failed to query provider usage", "provider", key, "error", err)
		sendFail(c, http.StatusBadGateway, CodeInternalError, "failed to query usage: "+err.Error())
		return
	}

	sendOK(c, gin.H{
		"provider": key,
		"level":    info.Level,
		"windows":  info.Windows,
	})
}

// Usage-query clients are cached so repeated lookups reuse the transport's
// connection pool instead of accumulating idle TLS sockets (an idle transport
// lingers ~90s). The direct client never changes; the proxy client is rebuilt
// only when server.proxy_url changes.
var (
	usageClientsMu   sync.Mutex
	usageDirectOnce  sync.Once
	usageDirect      *http.Client
	usageProxyURL    string
	usageProxyClient *http.Client
)

// usageHTTPClient returns an HTTP client for usage queries. It mirrors the
// proxy handler's transport policy: providers with enable_proxy route through
// the global server.proxy_url, everything else connects directly via
// newUpstreamTransport(nil) — deliberately NOT inheriting HTTP(S)_PROXY env
// vars, because ai-switch manages proxy selection itself.
func usageHTTPClient(cfg *config.Config, p config.ProviderConfig) *http.Client {
	if !p.EnableProxy || cfg.Server.ProxyURL == "" {
		usageDirectOnce.Do(func() {
			usageDirect = &http.Client{
				Timeout:   usageQueryTimeout,
				Transport: newUpstreamTransport(nil),
			}
		})
		return usageDirect
	}

	usageClientsMu.Lock()
	defer usageClientsMu.Unlock()
	if usageProxyClient != nil && usageProxyURL == cfg.Server.ProxyURL {
		return usageProxyClient
	}

	proxyURL, err := url.Parse(cfg.Server.ProxyURL)
	if err != nil {
		slog.Error("invalid proxy URL, usage query falls back to direct", "proxy_url", cfg.Server.ProxyURL, "error", err)
		usageDirectOnce.Do(func() {
			usageDirect = &http.Client{
				Timeout:   usageQueryTimeout,
				Transport: newUpstreamTransport(nil),
			}
		})
		return usageDirect
	}

	// Drop the old proxy transport's idle connections before replacing it.
	if usageProxyClient != nil {
		if t, ok := usageProxyClient.Transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
	}
	usageProxyClient = &http.Client{
		Timeout:   usageQueryTimeout,
		Transport: newUpstreamTransport(http.ProxyURL(proxyURL)),
	}
	usageProxyURL = cfg.Server.ProxyURL
	return usageProxyClient
}
