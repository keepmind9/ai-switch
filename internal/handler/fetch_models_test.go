package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- fetchOpenAIModels ---

func TestFetchOpenAIModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o", "object": "model"},
				{"id": "gpt-4.1", "object": "model"},
				{"id": "o3", "object": "model"},
			},
		})
	}))
	defer server.Close()

	models, err := fetchOpenAIModels(server.URL, "test-key")
	require.NoError(t, err)
	assert.Equal(t, []ModelInfo{
		{ID: "gpt-4.1", Name: "gpt-4.1"},
		{ID: "gpt-4o", Name: "gpt-4o"},
		{ID: "o3", Name: "o3"},
	}, models)
}

func TestFetchOpenAIModels_DeduplicatesAndSorts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o"},
				{"id": "o3"},
				{"id": "gpt-4o"},
			},
		})
	}))
	defer server.Close()

	models, err := fetchOpenAIModels(server.URL, "key")
	require.NoError(t, err)
	assert.Equal(t, []ModelInfo{
		{ID: "gpt-4o", Name: "gpt-4o"},
		{ID: "o3", Name: "o3"},
	}, models)
}

func TestFetchOpenAIModels_Unreachable(t *testing.T) {
	_, err := fetchOpenAIModels("http://127.0.0.1:0", "key")
	assert.Error(t, err)
}

func TestFetchOpenAIModels_EmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	models, err := fetchOpenAIModels(server.URL, "key")
	require.NoError(t, err)
	assert.Empty(t, models)
}

func TestFetchOpenAIModels_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := fetchOpenAIModels(server.URL, "key")
	assert.Error(t, err)
}

func TestFetchOpenAIModels_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	_, err := fetchOpenAIModels(server.URL, "bad-key")
	assert.Error(t, err)
}

// --- HTTP handler tests ---

func newFetchModelsRouter(admin *AdminHandler) *gin.Engine {
	r := gin.New()
	r.POST("/api/admin/providers/fetch-models", admin.fetchModels)
	return r
}

func TestFetchModelsHandler_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o"},
			},
		})
	}))
	defer server.Close()

	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := fmt.Sprintf(`{"base_url":"%s","api_key":"test-key","format":"chat"}`, server.URL)
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	models := resp["models"].([]any)
	assert.Len(t, models, 1)
}

func TestFetchModelsHandler_MissingFields(t *testing.T) {
	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(`{"base_url":"http://x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFetchModelsHandler_MissingAPIKeyAndKey(t *testing.T) {
	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := `{"base_url":"http://x","api_key":"","format":"chat"}`
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "api_key")
}

func TestFetchModelsHandler_ResolvesKeyFromExistingProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer stored-key", r.Header.Get("Authorization"))
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o"},
			},
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"openai": {APIKey: "stored-key"},
		},
	}
	provider := config.NewProvider(cfg, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := fmt.Sprintf(`{"base_url":"%s","api_key":"","key":"openai","format":"chat"}`, server.URL)
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	models := resp["models"].([]any)
	assert.Len(t, models, 1)
}

func TestFetchModelsHandler_KeyNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := fmt.Sprintf(`{"base_url":"%s","api_key":"","key":"nonexist","format":"chat"}`, server.URL)
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFetchModelsHandler_InvalidFormat(t *testing.T) {
	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := `{"base_url":"http://x","api_key":"k","format":"invalid"}`
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFetchModelsHandler_UpstreamError(t *testing.T) {
	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := `{"base_url":"http://127.0.0.1:0","api_key":"k","format":"chat"}`
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestFetchModelsHandler_AnthropicFormatUsesBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-key", r.Header.Get("Authorization"))
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-4-20250514"},
			},
		})
	}))
	defer server.Close()

	provider := config.NewProvider(&config.Config{}, "test.yaml")
	admin := NewAdminHandler(provider)
	r := newFetchModelsRouter(admin)

	body := fmt.Sprintf(`{"base_url":"%s","api_key":"my-key","format":"anthropic"}`, server.URL)
	req := httptest.NewRequest("POST", "/api/admin/providers/fetch-models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	models := resp["models"].([]any)
	assert.Len(t, models, 1)
}

// --- userFriendlyErr ---

func TestUserFriendlyErr_ConnectionRefused(t *testing.T) {
	assert.Equal(t, "could not connect to provider API", userFriendlyErr(errors.New("dial tcp: connection refused")))
}

func TestUserFriendlyErr_NoSuchHost(t *testing.T) {
	assert.Equal(t, "could not resolve provider hostname", userFriendlyErr(errors.New("lookup foo: no such host")))
}

func TestUserFriendlyErr_Timeout(t *testing.T) {
	assert.Equal(t, "connection to provider timed out", userFriendlyErr(errors.New("context deadline exceeded")))
}

func TestUserFriendlyErr_LongMessage(t *testing.T) {
	longMsg := strings.Repeat("x", 300)
	result := userFriendlyErr(errors.New(longMsg))
	assert.Len(t, result, 200)
}
