package config

import (
	"embed"
	"log/slog"

	"gopkg.in/yaml.v3"
)

//go:embed provider_presets.yaml
var presetsFS embed.FS

type ProviderPreset struct {
	Key        string `json:"key" yaml:"key"`
	Name       string `json:"name" yaml:"name"`
	BaseURL    string `json:"base_url" yaml:"base_url"`
	Format     string `json:"format" yaml:"format"`
	Icon       string `json:"icon" yaml:"icon"`
	IconColor  string `json:"icon_color" yaml:"icon_color"`
	Category   string `json:"category" yaml:"category"`
	APIKeyURL  string `json:"api_key_url" yaml:"api_key_url"`
	IsPartner  bool   `json:"is_partner" yaml:"is_partner"`
}

type presetsFile struct {
	Presets []ProviderPreset `yaml:"presets"`
}

var ProviderPresets []ProviderPreset

func init() {
	data, err := presetsFS.ReadFile("provider_presets.yaml")
	if err != nil {
		slog.Error("failed to read provider presets", "error", err)
		return
	}

	var pf presetsFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		slog.Error("failed to parse provider presets", "error", err)
		return
	}

	ProviderPresets = pf.Presets
}
