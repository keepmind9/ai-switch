package router

import (
	"encoding/json"
	"fmt"

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

	// No routes configured → fallback to default upstream
	if len(cfg.Routes) == 0 {
		return defaultRoute(cfg), nil
	}

	// Lookup API key in routes
	rule, ok := cfg.Routes[apiKey]
	if !ok {
		return defaultRoute(cfg), nil
	}

	// Lookup provider
	prov, ok := cfg.Providers[rule.Provider]
	if !ok {
		return nil, fmt.Errorf("route references unknown provider %q", rule.Provider)
	}

	model := resolveModel(rule, clientProtocol, body)

	return &RouteResult{
		BaseURL: prov.BaseURL,
		APIKey:  prov.APIKey,
		Format:  prov.Format,
		Model:   model,
		Path:    prov.Path,
	}, nil
}

func defaultRoute(cfg *config.Config) *RouteResult {
	return &RouteResult{
		BaseURL: cfg.Upstream.BaseURL,
		APIKey:  cfg.Upstream.APIKey,
		Format:  cfg.Upstream.Format,
		Model:   cfg.Upstream.Model,
		Path:    cfg.Upstream.Path,
	}
}

func resolveModel(rule config.RouteRule, clientProtocol string, body []byte) string {
	if clientProtocol == "anthropic" && len(rule.SceneMap) > 0 {
		scene := DetectScene(body)
		if mapped, ok := rule.SceneMap[scene]; ok {
			return mapped
		}
		return rule.DefaultModel
	}

	if len(rule.ModelMap) > 0 {
		var req struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &req) == nil && req.Model != "" {
			if mapped, ok := rule.ModelMap[req.Model]; ok {
				return mapped
			}
		}
	}

	return rule.DefaultModel
}
