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
				Sponsor: true,
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

	admin := NewAdminHandler(provider)
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
	assert.True(t, len(key) > 3 && key[:3] == "gw-")
	assert.Len(t, key, 35) // gw- + 32 hex chars
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
