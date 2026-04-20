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

	if len(cfg.Routes) > 0 {
		// Map key is the gateway API key; viper lowercases map keys, so match lowercase.
		if rule, ok := cfg.Routes[strings.ToLower(apiKey)]; ok {
			prov, ok := cfg.Providers[rule.Provider]
			if !ok {
				return nil, fmt.Errorf("route references unknown provider %q", rule.Provider)
			}
			model := resolveModel(rule, clientProtocol, body)
			return providerToResult(prov, model), nil
		}
	}

	// Fallback to default_provider
	dp := cfg.DefaultProviderConfig()
	if dp == nil {
		return nil, fmt.Errorf("no matching route and default_provider not configured")
	}
	return providerToResult(*dp, dp.Model), nil
}

func providerToResult(prov config.ProviderConfig, model string) *RouteResult {
	return &RouteResult{
		BaseURL:  prov.BaseURL,
		APIKey:   prov.APIKey,
		Format:   prov.Format,
		Model:    model,
		Path:     prov.Path,
		ThinkTag: prov.ThinkTag,
	}
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
		scene := DetectScene(body)
		if mapped, ok := rule.SceneMap[scene]; ok {
			return mapped
		}
	}

	// Priority 3: DefaultModel fallback
	return rule.DefaultModel
}
