package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSettingsTest(t *testing.T) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		Server:           config.ServerConfig{Host: "127.0.0.1", Port: 18080},
		LogRetentionDays: 30,
		LLMLogEnabled:    true,
		Providers: map[string]config.ProviderConfig{
			"test-provider": {
				Name:    "Test",
				BaseURL: "https://api.test.com",
				APIKey:  "sk-test",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"test-route": {Provider: "test-provider"},
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

func TestGetSettings(t *testing.T) {
	r, _ := setupSettingsTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	assert.Equal(t, "127.0.0.1", data["host"])
	assert.Equal(t, float64(18080), data["port"])
	assert.Equal(t, float64(30), data["log_retention_days"])
	assert.Equal(t, true, data["llm_log_enabled"])
}

func TestUpdateSettingsHost(t *testing.T) {
	r, cfgPath := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"host": "0.0.0.0",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", loaded.Server.Host)
	assert.Equal(t, 18080, loaded.Server.Port)
}

func TestUpdateSettingsPort(t *testing.T) {
	r, cfgPath := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"port": 9090,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 9090, loaded.Server.Port)
	assert.Equal(t, "127.0.0.1", loaded.Server.Host)
}

func TestUpdateSettingsLogRetention(t *testing.T) {
	r, cfgPath := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"log_retention_days": 7,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 7, loaded.LogRetentionDays)
}

func TestUpdateSettingsAllFields(t *testing.T) {
	r, cfgPath := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"host":               "0.0.0.0",
		"port":               3000,
		"log_retention_days": 14,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	assert.Equal(t, "0.0.0.0", data["host"])
	assert.Equal(t, float64(3000), data["port"])
	assert.Equal(t, float64(14), data["log_retention_days"])

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", loaded.Server.Host)
	assert.Equal(t, 3000, loaded.Server.Port)
	assert.Equal(t, 14, loaded.LogRetentionDays)
}

func TestUpdateSettingsInvalidPort(t *testing.T) {
	r, _ := setupSettingsTest(t)

	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"too large port", 65536},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"port": tc.port,
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestUpdateSettingsInvalidLogRetention(t *testing.T) {
	r, _ := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"log_retention_days": 0,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSettingsPreservesProviders(t *testing.T) {
	r, cfgPath := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"port": 9999,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)

	_, exists := loaded.Providers["test-provider"]
	assert.True(t, exists, "providers should be preserved after settings update")

	_, routeExists := loaded.Routes["test-route"]
	assert.True(t, routeExists, "routes should be preserved after settings update")
}

func TestRestartServer(t *testing.T) {
	r, _ := setupSettingsTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/restart", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	url := data["url"].(string)
	assert.Equal(t, "http://127.0.0.1:18080/ui", url)
}

func TestRestartServerWildcardHost(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	cfg := &config.Config{
		Server:           config.ServerConfig{Host: "0.0.0.0", Port: 9090},
		LogRetentionDays: 30,
		Providers:        map[string]config.ProviderConfig{},
		Routes:           map[string]config.RouteRule{},
	}
	require.NoError(t, config.WriteConfig(cfgPath, cfg))

	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	provider := config.NewProvider(loaded, cfgPath)

	admin := NewAdminHandler(provider, nil, nil)
	r := gin.New()
	adminGroup := r.Group("/api", func(c *gin.Context) { c.Next() })
	admin.RegisterRoutes(adminGroup)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/restart", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data := resp["data"].(map[string]any)
	url := data["url"].(string)
	assert.Equal(t, "http://localhost:9090/ui", url)
}

func TestUpdateSettingsLLMLogEnabled(t *testing.T) {
	r, cfgPath := setupSettingsTest(t)

	body, _ := json.Marshal(map[string]any{
		"llm_log_enabled": false,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	assert.Equal(t, false, data["llm_log_enabled"])

	// Must survive the write/read round-trip (disabled flag is not dropped).
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.False(t, loaded.LLMLogEnabled)
}

func TestUpdateSettingsEmptyBody(t *testing.T) {
	r, _ := setupSettingsTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/admin/settings", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
