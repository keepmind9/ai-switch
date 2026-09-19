package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUsageTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		Server:       config.ServerConfig{Host: "0.0.0.0", Port: 12345},
		DefaultRoute: "gw-test",
		Providers: map[string]config.ProviderConfig{
			"glm": {
				Name:    "GLM",
				BaseURL: "https://open.bigmodel.cn/api/anthropic",
				APIKey:  "primary-key",
				Format:  "anthropic",
			},
			"glm-fallback": {
				Name:         "GLM Fallback",
				BaseURL:      "https://api.z.ai/api/anthropic",
				APIKey:       "",
				FallbackKeys: []string{"fallback-key"},
				Format:       "anthropic",
			},
			"glm-nokey": {
				Name:    "GLM No Key",
				BaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
				APIKey:  "",
				Format:  "chat",
			},
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.minimaxi.com",
				APIKey:  "sk-test",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {Provider: "glm", DefaultModel: "glm-5.1"},
		},
	}

	require.NoError(t, config.WriteConfig(cfgPath, cfg))

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	provider := config.NewProvider(loaded, cfgPath)

	admin := NewAdminHandler(provider, nil, nil)
	r := gin.New()
	adminGroup := r.Group("/api", func(c *gin.Context) { c.Next() })
	admin.RegisterRoutes(adminGroup)
	return r
}

type usageResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func getUsage(t *testing.T, r *gin.Engine, key string) (*usageResponse, int) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/usage/"+key, nil)
	r.ServeHTTP(w, req)

	var resp usageResponse
	if w.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	}
	return &resp, w.Code
}

func TestGetUsage(t *testing.T) {
	t.Cleanup(func() { queryUsage = usage.Query })

	tests := []struct {
		name       string
		provider   string
		mockQuery  func(ctx context.Context, client *http.Client, baseURL, apiKey string) (*usage.Info, error)
		wantStatus int
		wantCode   int
		wantMsg    string
		checkData  func(t *testing.T, data json.RawMessage)
	}{
		{
			name:     "success",
			provider: "glm",
			mockQuery: func(ctx context.Context, client *http.Client, baseURL, apiKey string) (*usage.Info, error) {
				assert.Equal(t, "https://open.bigmodel.cn/api/anthropic", baseURL)
				assert.Equal(t, "primary-key", apiKey)
				return &usage.Info{Level: "lite", Windows: []usage.Window{{Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Utilization: 78.5, ResetsAtMs: 1758291600000, WindowActive: true}}}, nil
			},
			wantStatus: http.StatusOK,
			wantCode:   CodeSuccess,
			checkData: func(t *testing.T, data json.RawMessage) {
				var d map[string]any
				require.NoError(t, json.Unmarshal(data, &d))
				assert.Equal(t, "glm", d["provider"])
				assert.Equal(t, "lite", d["level"])
				windows, ok := d["windows"].([]any)
				require.True(t, ok)
				require.Len(t, windows, 1)
				w := windows[0].(map[string]any)
				assert.Equal(t, "TOKENS_LIMIT", w["type"])
				assert.Equal(t, 78.5, w["utilization"])
				assert.Equal(t, float64(1758291600000), w["resets_at_ms"])
				assert.Equal(t, true, w["window_active"])
			},
		},
		{
			name:     "uses fallback key when primary empty",
			provider: "glm-fallback",
			mockQuery: func(ctx context.Context, client *http.Client, baseURL, apiKey string) (*usage.Info, error) {
				assert.Equal(t, "fallback-key", apiKey)
				return &usage.Info{Level: "credit", Windows: []usage.Window{{Type: "CREDIT_LIMIT", Unit: 1, Number: 1, Utilization: 0, ResetsAtMs: 0, WindowActive: false}}}, nil
			},
			wantStatus: http.StatusOK,
			wantCode:   CodeSuccess,
		},
		{
			name:       "provider not found",
			provider:   "nope",
			wantStatus: http.StatusNotFound,
			wantCode:   CodeNotFound,
			wantMsg:    "provider not found",
		},
		{
			name:       "unsupported base_url",
			provider:   "minimax",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeBadRequest,
			wantMsg:    "not supported",
		},
		{
			name:       "no api key",
			provider:   "glm-nokey",
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeBadRequest,
			wantMsg:    "no API key",
		},
		{
			name:     "upstream error",
			provider: "glm",
			mockQuery: func(ctx context.Context, client *http.Client, baseURL, apiKey string) (*usage.Info, error) {
				return nil, assert.AnError
			},
			wantStatus: http.StatusBadGateway,
			wantCode:   CodeInternalError,
			wantMsg:    "failed to query usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupUsageTest(t)
			if tt.mockQuery != nil {
				queryUsage = tt.mockQuery
			} else {
				queryUsage = func(ctx context.Context, client *http.Client, baseURL, apiKey string) (*usage.Info, error) {
					t.Fatal("queryUsage should not be called")
					return nil, nil
				}
			}

			resp, status := getUsage(t, r, tt.provider)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantCode, resp.Code)
			if tt.wantMsg != "" {
				assert.Contains(t, resp.Msg, tt.wantMsg)
			}
			if tt.checkData != nil {
				tt.checkData(t, resp.Data)
			}
		})
	}
}

func TestUsageHTTPClient(t *testing.T) {
	tests := []struct {
		name          string
		proxyURL      string
		enableProxy   bool
		wantTransport bool
	}{
		{"proxy enabled with global proxy", "socks5://127.0.0.1:1080", true, true},
		{"proxy enabled but no global proxy", "", true, false},
		{"proxy disabled", "socks5://127.0.0.1:1080", false, false},
		{"no proxy config at all", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Server: config.ServerConfig{ProxyURL: tt.proxyURL}}
			p := config.ProviderConfig{EnableProxy: tt.enableProxy}

			client := usageHTTPClient(cfg, p)

			require.NotNil(t, client)
			assert.Equal(t, usageQueryTimeout, client.Timeout)
			transport, ok := client.Transport.(*http.Transport)
			require.True(t, ok, "expected *http.Transport")
			if tt.wantTransport {
				assert.NotNil(t, transport.Proxy, "expected proxy-routed transport")
			} else {
				assert.Nil(t, transport.Proxy, "expected direct transport without env proxy")
			}
		})
	}
}
