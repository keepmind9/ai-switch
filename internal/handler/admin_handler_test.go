package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAdminTest(t *testing.T) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		Server:       config.ServerConfig{Host: "0.0.0.0", Port: 12345},
		DefaultRoute: "gw-test",
		Providers: map[string]config.ProviderConfig{
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.minimaxi.com",
				APIKey:  "sk-test-key-12345678",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {
				Provider:     "minimax",
				DefaultModel: "MiniMax-M2.7",
			},
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

	return r, cfgPath
}

func TestListProviders(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].([]any)
	assert.Len(t, data, 1)

	p := data[0].(map[string]any)
	assert.Equal(t, "minimax", p["key"])
	assert.Equal(t, "sk-t****5678", p["api_key"])
	assert.Equal(t, "MiniMax", p["name"])
}

func TestCreateProvider(t *testing.T) {
	r, _ := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"key":      "deepseek",
		"name":     "DeepSeek",
		"base_url": "https://api.deepseek.com",
		"api_key":  "sk-deep-key",
		"format":   "chat",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	r.ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp["data"].([]any), 2)
}

func TestCreateProviderDuplicate(t *testing.T) {
	r, _ := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"key":      "minimax",
		"name":     "Dup",
		"base_url": "https://dup.com",
		"api_key":  "sk-dup",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUpdateProvider(t *testing.T) {
	r, _ := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"name": "MiniMax Updated",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/minimax", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateProviderPreserveAPIKey(t *testing.T) {
	tests := []struct {
		name       string
		payload    map[string]any
		wantAPIKey string
	}{
		{
			name:       "empty api_key preserves existing",
			payload:    map[string]any{"name": "Updated", "api_key": ""},
			wantAPIKey: "sk-test-key-12345678",
		},
		{
			name:       "new api_key overwrites",
			payload:    map[string]any{"name": "Updated", "api_key": "sk-new-key"},
			wantAPIKey: "sk-new-key",
		},
		{
			name:       "omit api_key preserves existing",
			payload:    map[string]any{"name": "Updated"},
			wantAPIKey: "sk-test-key-12345678",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := setupAdminTest(t)

			body, _ := json.Marshal(tc.payload)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/minimax", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			// Verify via GET that api_key is masked (still present)
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
			r.ServeHTTP(w, req)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			data := resp["data"].([]any)
			p := data[0].(map[string]any)
			assert.Contains(t, p["api_key"], "****")
		})
	}
}

func TestGetSettings_IncludeProxyURL(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	_, hasProxyURL := data["proxy_url"]
	assert.True(t, hasProxyURL, "settings response should include proxy_url")
}

func TestUpdateSettings_ProxyURL(t *testing.T) {
	r, cfgPath := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"proxy_url": "socks5://127.0.0.1:1080",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify persisted to config file
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "socks5://127.0.0.1:1080", loaded.Server.ProxyURL)
}

func TestUpdateSettings_ClearProxyURL(t *testing.T) {
	r, cfgPath := setupAdminTest(t)

	// Set proxy first
	body, _ := json.Marshal(map[string]any{"proxy_url": "http://proxy:8080"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Clear proxy
	body2, _ := json.Marshal(map[string]any{"proxy_url": ""})
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "", loaded.Server.ProxyURL)
}

func TestUpdateSettings_InvalidProxyURL(t *testing.T) {
	r, _ := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"proxy_url": "not-a-valid-url",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSettings_ProxyURLUnsupportedScheme(t *testing.T) {
	r, _ := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"proxy_url": "ftp://proxy:21",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateProvider_WithEnableProxy(t *testing.T) {
	r, cfgPath := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"key":          "openai",
		"name":         "OpenAI",
		"base_url":     "https://api.openai.com",
		"api_key":      "sk-test",
		"format":       "chat",
		"enable_proxy": true,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.True(t, loaded.Providers["openai"].EnableProxy)
}

func TestUpdateProvider_EnableProxy(t *testing.T) {
	r, cfgPath := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"enable_proxy": true,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/minimax", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.True(t, loaded.Providers["minimax"].EnableProxy)
}

func TestListProviders_IncludeEnableProxy(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].([]any)
	p := data[0].(map[string]any)
	_, hasEnableProxy := p["enable_proxy"]
	assert.True(t, hasEnableProxy, "provider list response should include enable_proxy")
}

func TestDeleteProvider(t *testing.T) {
	r, cfgPath := setupAdminTest(t)

	// Create a second provider so default_route can be cleared
	body, _ := json.Marshal(map[string]any{
		"key":      "other",
		"name":     "Other",
		"base_url": "https://other.com",
		"api_key":  "sk-other",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Now delete minimax (the default provider)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/admin/providers/minimax", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify provider removed from config file
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	_, exists := loaded.Providers["minimax"]
	assert.False(t, exists)
}

func TestListRoutes(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/routes", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].([]any)
	assert.Len(t, data, 1)

	route := data[0].(map[string]any)
	assert.Equal(t, "gw-test", route["key"])
	assert.Equal(t, "minimax", route["provider"])
}

func TestCreateRoute(t *testing.T) {
	r, _ := setupAdminTest(t)

	body, _ := json.Marshal(map[string]any{
		"key":           "gw-new",
		"provider":      "minimax",
		"default_model": "MiniMax-M2.7",
		"scene_map":     map[string]string{"default": "MiniMax-M2.7"},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateRouteWithLongContextThreshold(t *testing.T) {
	r, cfgPath := setupAdminTest(t)

	// Create a route with long_context_threshold
	threshold := 60000
	body, _ := json.Marshal(map[string]any{
		"key":                    "gw-longctx",
		"provider":               "minimax",
		"default_model":          "MiniMax-M2.7",
		"long_context_threshold": threshold,
		"scene_map": map[string]string{
			"default":     "MiniMax-M2.7",
			"longContext": "MiniMax-M2.7-large",
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify the route was persisted with long_context_threshold
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)

	route := loaded.Routes["gw-longctx"]
	assert.Equal(t, "minimax", route.Provider)
	assert.Equal(t, threshold, route.LongContextThreshold)
	assert.Equal(t, "MiniMax-M2.7-large", route.SceneMap["longcontext"])

	// Update the threshold
	newThreshold := 120000
	updateBody, _ := json.Marshal(map[string]any{
		"long_context_threshold": newThreshold,
	})

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/admin/routes/gw-longctx", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify updated threshold persisted
	loaded, err = config.Load(cfgPath)
	require.NoError(t, err)

	route = loaded.Routes["gw-longctx"]
	assert.Equal(t, newThreshold, route.LongContextThreshold)
	assert.Equal(t, "MiniMax-M2.7-large", route.SceneMap["longcontext"])
}

func TestDeleteRoute(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/routes/gw-test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGenerateKey(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/routes/generate-key", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	key := data["key"].(string)
	assert.True(t, len(key) > 4 && key[:4] == "ais-")
	assert.Len(t, key, 36) // ais- + 32 hex chars
}

func TestListPresets(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/presets", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].([]any)
	assert.NotEmpty(t, data)
}

func TestUpdateDefaultRoutes(t *testing.T) {
	t.Run("set all protocol defaults", func(t *testing.T) {
		r, cfgPath := setupAdminTest(t)

		body, _ := json.Marshal(map[string]any{
			"default_route":           "gw-test",
			"default_anthropic_route": "gw-test",
			"default_responses_route": "gw-test",
			"default_chat_route":      "gw-test",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/admin/default-routes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		loaded, err := config.Load(cfgPath)
		require.NoError(t, err)
		assert.Equal(t, "gw-test", loaded.DefaultRoute)
		assert.Equal(t, "gw-test", loaded.DefaultAnthropicRoute)
		assert.Equal(t, "gw-test", loaded.DefaultResponsesRoute)
		assert.Equal(t, "gw-test", loaded.DefaultChatRoute)
	})

	t.Run("clear protocol defaults", func(t *testing.T) {
		r, _ := setupAdminTest(t)

		body, _ := json.Marshal(map[string]any{
			"default_anthropic_route": "",
			"default_responses_route": "",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/admin/default-routes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("nonexistent route returns error", func(t *testing.T) {
		r, _ := setupAdminTest(t)

		body, _ := json.Marshal(map[string]any{
			"default_anthropic_route": "nonexistent",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/admin/default-routes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("partial update preserves other fields", func(t *testing.T) {
		r, cfgPath := setupAdminTest(t)

		body, _ := json.Marshal(map[string]any{
			"default_route":           "gw-test",
			"default_anthropic_route": "gw-test",
			"default_responses_route": "gw-test",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/admin/default-routes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		body, _ = json.Marshal(map[string]any{
			"default_anthropic_route": "",
		})
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPut, "/api/admin/default-routes", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		loaded, err := config.Load(cfgPath)
		require.NoError(t, err)
		assert.Equal(t, "", loaded.DefaultAnthropicRoute)
		assert.Equal(t, "gw-test", loaded.DefaultRoute)
		assert.Equal(t, "gw-test", loaded.DefaultResponsesRoute)
	})
}

// setupBackupTest seeds the config file twice so exactly one backup exists,
// reusing the existing setupAdminTest engine.
func setupBackupTest(t *testing.T) (*gin.Engine, string) {
	r, cfgPath := setupAdminTest(t)
	// Second write: produces exactly one backup of the first state.
	require.NoError(t, config.WriteConfig(cfgPath, &config.Config{
		Server: config.ServerConfig{Host: "0.0.0.0", Port: 22222},
	}))
	return r, cfgPath
}

func doBackupRequest(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestListConfigBackups_Empty(t *testing.T) {
	r, _ := setupAdminTest(t)

	w := doBackupRequest(t, r, http.MethodGet, "/api/admin/config/backups", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp["data"].([]any), 0)
}

func TestListConfigBackups_Populated(t *testing.T) {
	r, _ := setupBackupTest(t)

	w := doBackupRequest(t, r, http.MethodGet, "/api/admin/config/backups", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 1, "exactly one backup should exist after the second write")
	entry := data[0].(map[string]any)
	assert.Contains(t, entry["name"], "config.yaml.bak.")
	assert.Greater(t, int(entry["size"].(float64)), 0)
}

func TestRestoreConfigBackup_Success(t *testing.T) {
	r, cfgPath := setupBackupTest(t)

	// Pick the only backup from the list endpoint.
	w := doBackupRequest(t, r, http.MethodGet, "/api/admin/config/backups", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	require.Len(t, data, 1)
	name := data[0].(map[string]any)["name"].(string)

	// Restore: main should be the FIRST seeded state (port 12345).
	w = doBackupRequest(t, r, http.MethodPost, "/api/admin/config/backups/restore",
		map[string]any{"name": name})
	require.Equal(t, http.StatusOK, w.Code)

	// Verify disk reflects the restored value.
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 12345, loaded.Server.Port)

	// Verify provider reloads — the in-memory copy must match the disk copy.
	settings := doBackupRequest(t, r, http.MethodGet, "/api/admin/settings", nil)
	require.Equal(t, http.StatusOK, settings.Code)
	var sresp map[string]any
	require.NoError(t, json.Unmarshal(settings.Body.Bytes(), &sresp))
	assert.Equal(t, float64(12345), sresp["data"].(map[string]any)["port"].(float64))
}

func TestRestoreConfigBackup_RejectsInvalidName(t *testing.T) {
	cases := []map[string]any{
		{"name": ""},
		{"name": "../etc/passwd"},
		{"name": "config.yaml.bak.NOTATIMESTAMP"},
		{"name": "config.yaml.bak.20990101-000000"}, // well-formed but absent
	}
	for _, body := range cases {
		t.Run(fmt.Sprintf("%v", body["name"]), func(t *testing.T) {
			r, _ := setupAdminTest(t)
			w := doBackupRequest(t, r, http.MethodPost, "/api/admin/config/backups/restore", body)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestCleanConfigBackups_KeepZero(t *testing.T) {
	r, _ := setupBackupTest(t)

	w := doBackupRequest(t, r, http.MethodPost, "/api/admin/config/backups/clean",
		map[string]any{"keep": 0})
	require.Equal(t, http.StatusOK, w.Code)

	w = doBackupRequest(t, r, http.MethodGet, "/api/admin/config/backups", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp["data"].([]any), 0)
}

func TestCleanConfigBackups_NegativeKeepRejected(t *testing.T) {
	r, _ := setupAdminTest(t)
	w := doBackupRequest(t, r, http.MethodPost, "/api/admin/config/backups/clean",
		map[string]any{"keep": -1})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCleanConfigBackups_MissingKeepRejected(t *testing.T) {
	r, _ := setupAdminTest(t)
	w := doBackupRequest(t, r, http.MethodPost, "/api/admin/config/backups/clean",
		map[string]any{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
