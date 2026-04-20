package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate_AllErrors(t *testing.T) {
	cfg := &Config{
		DefaultRoute: "nonexistent-route",
		Providers: map[string]ProviderConfig{
			"p1": {Name: "P1", BaseURL: "", APIKey: "key1", Format: "chat"},
		},
		Routes: map[string]RouteRule{
			"r1": {
				Provider:     "nonexistent-provider",
				DefaultModel: "model-a",
				SceneMap:     map[string]string{"default": "other-provider:model-x"},
			},
		},
	}

	result := Validate(cfg)

	assert.True(t, result.HasErrors())

	errorMsgs := result.ErrorMessages()
	assert.Contains(t, errorMsgs, `route "r1" references unknown provider "nonexistent-provider"`)
	assert.Contains(t, errorMsgs, `default_route "nonexistent-route" not found in routes`)
	assert.Contains(t, errorMsgs, `provider "p1" has empty base_url`)
	assert.Contains(t, errorMsgs, `route "r1" scene_map key "default" references unknown provider "other-provider"`)
}

func TestValidate_Warnings(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"deepseek": {
				Name:    "DeepSeek",
				BaseURL: "https://api.deepseek.com",
				APIKey:  "key1",
				Format:  "chat",
				Models:  []string{"deepseek-v3"},
			},
			"p2": {
				Name:    "P2",
				BaseURL: "https://api.p2.com",
				APIKey:  "key2",
				Format:  "chat",
				Models:  []string{"model-a"},
			},
		},
		Routes: map[string]RouteRule{
			"r1": {
				Provider:     "deepseek",
				DefaultModel: "deepseek-chat",
				SceneMap: map[string]string{
					"think":  "deepseek:deepseek-chat",
					"codex":  "model-a",
					"search": "p2:model-a",
				},
			},
		},
	}

	result := Validate(cfg)

	assert.False(t, result.HasErrors())
	assert.True(t, result.HasOnlyWarnings())

	warnMsgs := result.WarningMessages()
	assert.Contains(t, warnMsgs, `route "r1" model "deepseek-chat" not found in provider "deepseek" models list`)
	assertContainsSubstring(t, warnMsgs, `route "r1" scene_map key "codex" is not a known scene`)
	assertContainsSubstring(t, warnMsgs, `route "r1" scene_map key "search" is not a known scene`)
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:    "OpenAI",
				BaseURL: "https://api.openai.com",
				APIKey:  "sk-test",
				Format:  "chat",
				Models:  []string{"gpt-4o", "gpt-4o-mini"},
			},
		},
		Routes: map[string]RouteRule{
			"r1": {
				Provider:     "openai",
				DefaultModel: "gpt-4o",
				SceneMap:     map[string]string{"think": "gpt-4o"},
			},
		},
		DefaultRoute: "r1",
	}

	result := Validate(cfg)
	assert.False(t, result.HasErrors())
	assert.False(t, result.HasOnlyWarnings())
	assert.Equal(t, 0, len(result.Errors))
	assert.Equal(t, 0, len(result.Warnings))
}

func TestValidate_EmptyApiKey(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"p1": {
				Name:    "P1",
				BaseURL: "https://api.p1.com",
				APIKey:  "",
				Format:  "chat",
			},
		},
		Routes: map[string]RouteRule{
			"r1": {
				Provider:     "p1",
				DefaultModel: "model-a",
			},
		},
	}

	result := Validate(cfg)
	assert.False(t, result.HasErrors())
	assert.True(t, result.HasOnlyWarnings())
	warnMsgs := result.WarningMessages()
	assert.Contains(t, warnMsgs, `provider "p1" is used by route "r1" but has empty api_key`)
}

func TestValidate_CrossProviderModelRef(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"p1": {
				Name:    "P1",
				BaseURL: "https://api.p1.com",
				APIKey:  "key1",
				Format:  "chat",
				Models:  []string{"model-a"},
			},
			"p2": {
				Name:    "P2",
				BaseURL: "https://api.p2.com",
				APIKey:  "key2",
				Format:  "chat",
				Models:  []string{"model-b"},
			},
		},
		Routes: map[string]RouteRule{
			"r1": {
				Provider:     "p1",
				DefaultModel: "model-a",
				SceneMap:     map[string]string{"think": "p2:model-b"},
				ModelMap:     map[string]string{"gpt-4": "p2:model-b"},
			},
		},
	}

	result := Validate(cfg)
	assert.False(t, result.HasErrors())
	assert.False(t, result.HasOnlyWarnings())
	assert.Equal(t, 0, len(result.Errors))
	assert.Equal(t, 0, len(result.Warnings))
}

func TestValidate_EmptyDefaultModel(t *testing.T) {
	cfg := &Config{
		Providers: map[string]ProviderConfig{
			"p1": {
				Name:    "P1",
				BaseURL: "https://api.p1.com",
				APIKey:  "key1",
				Format:  "chat",
			},
		},
		Routes: map[string]RouteRule{
			"r1": {
				Provider: "p1",
			},
		},
	}

	result := Validate(cfg)
	assert.False(t, result.HasErrors())
	warnMsgs := result.WarningMessages()
	assert.Contains(t, warnMsgs, `route "r1" has empty default_model`)
}

func assertContainsSubstring(t *testing.T, msgs []string, substr string) {
	t.Helper()
	found := false
	for _, m := range msgs {
		if strings.Contains(m, substr) {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find substring %q in messages %v", substr, msgs)
}

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := &Config{}

	result := Validate(cfg)
	assert.False(t, result.HasErrors())
	assert.False(t, result.HasOnlyWarnings())
}
