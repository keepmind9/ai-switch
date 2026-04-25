package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
)

// ModelInfo represents a single model returned by the fetch-models endpoint.
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fetchModelsRequest struct {
	BaseURL string `json:"base_url" binding:"required"`
	APIKey  string `json:"api_key"`
	Key     string `json:"key"`
	Format  string `json:"format" binding:"required"`
}

const fetchModelsTimeout = 15 * time.Second

// fetchModels handles POST /admin/providers/fetch-models.
func (a *AdminHandler) fetchModels(c *gin.Context) {
	var req fetchModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "base_url and format are required"})
		return
	}

	if !config.ValidFormat(req.Format) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid format, must be chat, responses, or anthropic"})
		return
	}

	// Resolve API key: use provided api_key first, fall back to existing provider config
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" && req.Key != "" {
		cfg := a.provider.Get()
		if p, ok := cfg.Providers[req.Key]; ok {
			apiKey = p.APIKey
		}
	}
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_key is required"})
		return
	}

	apiKey = os.ExpandEnv(apiKey)

	// For anthropic format, replace /anthropic in path with /v1 to get models endpoint.
	baseURL := req.BaseURL
	if req.Format == "anthropic" {
		baseURL = strings.Replace(baseURL, "/anthropic", "/v1", 1)
	}

	var models []ModelInfo
	var err error

	models, err = fetchOpenAIModels(baseURL, apiKey)

	if err != nil {
		slog.Warn("failed to fetch models", "base_url", req.BaseURL, "format", req.Format, "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to fetch models: %s (the provider may not support automatic model listing)", userFriendlyErr(err))})
		return
	}

	if len(models) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "provider returned an empty model list (the provider may not support automatic model listing)"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"models": models})
}

// userFriendlyErr returns a user-facing error message.
func userFriendlyErr(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "connection refused") {
		return "could not connect to provider API"
	}
	if strings.Contains(msg, "no such host") {
		return "could not resolve provider hostname"
	}
	if strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "context deadline") {
		return "connection to provider timed out"
	}
	if len(msg) > 200 {
		return msg[:200]
	}
	return msg
}

// fetchOpenAIModels calls GET {baseURL}/models with Bearer auth.
func fetchOpenAIModels(baseURL, apiKey string) ([]ModelInfo, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: fetchModelsTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, d := range result.Data {
		models = append(models, ModelInfo{ID: d.ID, Name: d.ID})
	}

	return dedupAndSortInPlace(models), nil
}

// dedupAndSortInPlace deduplicates by ID and sorts alphabetically.
func dedupAndSortInPlace(models []ModelInfo) []ModelInfo {
	seen := make(map[string]bool, len(models))
	n := 0
	for _, m := range models {
		if !seen[m.ID] {
			seen[m.ID] = true
			models[n] = m
			n++
		}
	}
	models = models[:n]
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models
}
