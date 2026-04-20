package router

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keepmind9/llm-gateway/internal/config"
)

// ConfigRouter implements Router using config-based routing rules.
type ConfigRouter struct {
	provider *config.Provider
}

// NewConfigRouter creates a new ConfigRouter.
func NewConfigRouter(provider *config.Provider) *ConfigRouter {
	return &ConfigRouter{provider: provider}
}

// Route resolves a request to an upstream provider + model.
func (r *ConfigRouter) Route(clientProtocol, apiKey string, body []byte) (*RouteResult, error) {
	cfg := r.provider.Get()

	// 1. Try to find route by API key
	if len(cfg.Routes) > 0 {
		if rule, ok := cfg.Routes[strings.ToLower(apiKey)]; ok {
			return r.resolveRoute(cfg, rule, clientProtocol, body)
		}
	}

	// 2. Fallback to default_route
	dr := cfg.DefaultRouteConfig()
	if dr == nil {
		return nil, fmt.Errorf("no matching route and default_route not configured")
	}
	return r.resolveRoute(cfg, *dr, clientProtocol, body)
}

func (r *ConfigRouter) resolveRoute(cfg *config.Config, rule config.RouteRule, clientProtocol string, body []byte) (*RouteResult, error) {
	modelValue := resolveModel(rule, clientProtocol, body)
	providerKey, modelName := parseProviderModel(modelValue, rule.Provider)

	prov, ok := cfg.Providers[providerKey]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerKey)
	}

	return &RouteResult{
		BaseURL:  prov.BaseURL,
		APIKey:   prov.APIKey,
		Format:   prov.Format,
		Model:    modelName,
		Path:     prov.Path,
		ThinkTag: prov.ThinkTag,
	}, nil
}

// parseProviderModel splits a "provider:model" value. Plain model names
// use defaultProvider. Uses LastIndex for safety with edge cases.
func parseProviderModel(value, defaultProvider string) (provider string, model string) {
	if idx := strings.LastIndex(value, ":"); idx > 0 {
		return value[:idx], value[idx+1:]
	}
	return defaultProvider, value
}

func resolveModel(rule config.RouteRule, clientProtocol string, body []byte) string {
	// Priority 1: ModelMap — exact model name match (all protocols, case-insensitive)
	if len(rule.ModelMap) > 0 {
		var req struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &req) == nil && req.Model != "" {
			lower := strings.ToLower(req.Model)
			for k, v := range rule.ModelMap {
				if strings.ToLower(k) == lower {
					return v
				}
			}
		}
	}

	// Priority 2: SceneMap — heuristic scene detection (anthropic protocol only)
	if clientProtocol == "anthropic" && len(rule.SceneMap) > 0 {
		scene := DetectScene(body, SceneConfig{
			LongContextThreshold: rule.LongContextThreshold,
		})
		if mapped, ok := rule.SceneMap[scene]; ok {
			return mapped
		}
	}

	// Priority 3: DefaultModel fallback
	return rule.DefaultModel
}
