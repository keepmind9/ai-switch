package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadMapFields verifies viper/mapstructure can decode map[string]string
// fields (custom_headers, scene_map, model_map) from YAML.
func TestLoadMapFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	yamlContent := `
providers:
  kimi:
    name: Kimi
    base_url: https://api.kimi.com/coding
    api_key: k
    format: anthropic
    custom_headers:
      User-Agent: claude-code/1.0.0
routes:
  r1:
    provider: kimi
    default_model: m
    scene_map:
      default: m
    model_map:
      gpt-4o: m
`
	require.NoError(t, os.WriteFile(p, []byte(yamlContent), 0o644))
	cfg, err := Load(p)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"User-Agent": "claude-code/1.0.0"}, cfg.Providers["kimi"].CustomHeaders, "custom_headers must load")
	assert.Equal(t, map[string]string{"default": "m"}, cfg.Routes["r1"].SceneMap, "scene_map must load")
	assert.Equal(t, map[string]string{"gpt-4o": "m"}, cfg.Routes["r1"].ModelMap, "model_map must load")
}
