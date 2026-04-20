package config

import (
	"fmt"
	"sort"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

var knownScenes = map[string]bool{
	"default":     true,
	"longContext": true,
	"background":  true,
	"websearch":   true,
	"think":       true,
	"image":       true,
}

type ValidationIssue struct {
	Severity Severity
	Message  string
}

type ValidationResult struct {
	Errors   []ValidationIssue
	Warnings []ValidationIssue
}

func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func (r *ValidationResult) HasOnlyWarnings() bool {
	return len(r.Warnings) > 0 && len(r.Errors) == 0
}

func (r *ValidationResult) ErrorMessages() []string {
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.Message
	}
	return msgs
}

func (r *ValidationResult) WarningMessages() []string {
	msgs := make([]string, len(r.Warnings))
	for i, w := range r.Warnings {
		msgs[i] = w.Message
	}
	return msgs
}

func Validate(cfg *Config) *ValidationResult {
	result := &ValidationResult{}

	providerUsedByRoute := make(map[string][]string)
	for routeKey, rule := range cfg.Routes {
		providerUsedByRoute[rule.Provider] = append(providerUsedByRoute[rule.Provider], routeKey)
	}

	for provKey, prov := range cfg.Providers {
		if prov.BaseURL == "" {
			result.Errors = append(result.Errors, ValidationIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("provider %q has empty base_url", provKey),
			})
		}
	}

	for routeKey, rule := range cfg.Routes {
		if _, ok := cfg.Providers[rule.Provider]; !ok {
			result.Errors = append(result.Errors, ValidationIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("route %q references unknown provider %q", routeKey, rule.Provider),
			})
		} else {
			if rule.DefaultModel != "" {
				validateModelRef(result, cfg, rule.Provider, rule.DefaultModel, routeKey)
			}
		}

		if rule.DefaultModel == "" {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("route %q has empty default_model", routeKey),
			})
		}

		for sceneKey, modelValue := range rule.SceneMap {
			if !knownScenes[sceneKey] {
				result.Warnings = append(result.Warnings, ValidationIssue{
					Severity: SeverityWarning,
					Message: fmt.Sprintf(
						"route %q scene_map key %q is not a known scene (valid: %s)",
						routeKey, sceneKey, strings.Join(sortedScenes(), ", "),
					),
				})
			}

			pk, modelName := SplitProviderModel(modelValue, rule.Provider)
			if _, ok := cfg.Providers[pk]; !ok {
				result.Errors = append(result.Errors, ValidationIssue{
					Severity: SeverityError,
					Message:  fmt.Sprintf("route %q scene_map key %q references unknown provider %q", routeKey, sceneKey, pk),
				})
			} else {
				validateModelRef(result, cfg, pk, modelName, routeKey)
			}
		}

		for mapKey, modelValue := range rule.ModelMap {
			pk, modelName := SplitProviderModel(modelValue, rule.Provider)
			if _, ok := cfg.Providers[pk]; !ok {
				result.Errors = append(result.Errors, ValidationIssue{
					Severity: SeverityError,
					Message:  fmt.Sprintf("route %q model_map key %q references unknown provider %q", routeKey, mapKey, pk),
				})
			} else {
				validateModelRef(result, cfg, pk, modelName, routeKey)
			}
		}
	}

	if cfg.DefaultRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultRoute]; !ok {
			result.Errors = append(result.Errors, ValidationIssue{
				Severity: SeverityError,
				Message:  fmt.Sprintf("default_route %q not found in routes", cfg.DefaultRoute),
			})
		}
	}

	for provKey, routeKeys := range providerUsedByRoute {
		prov, ok := cfg.Providers[provKey]
		if ok && prov.APIKey == "" {
			for _, rk := range routeKeys {
				result.Warnings = append(result.Warnings, ValidationIssue{
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("provider %q is used by route %q but has empty api_key", provKey, rk),
				})
			}
		}
	}

	return result
}

func validateModelRef(result *ValidationResult, cfg *Config, provKey, model, routeKey string) {
	prov := cfg.Providers[provKey]
	if len(prov.Models) == 0 {
		return
	}
	for _, m := range prov.Models {
		if m == model {
			return
		}
	}
	result.Warnings = append(result.Warnings, ValidationIssue{
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("route %q model %q not found in provider %q models list", routeKey, model, provKey),
	})
}

func sortedScenes() []string {
	scenes := make([]string, 0, len(knownScenes))
	for s := range knownScenes {
		scenes = append(scenes, s)
	}
	sort.Strings(scenes)
	return scenes
}

// SplitProviderModel splits "provider:model" format. Plain names use defaultProvider.
func SplitProviderModel(value, defaultProvider string) (string, string) {
	if idx := strings.LastIndex(value, ":"); idx > 0 {
		return value[:idx], value[idx+1:]
	}
	return defaultProvider, value
}
