package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderPresets(t *testing.T) {
	assert.NotEmpty(t, ProviderPresets)

	keys := map[string]bool{}
	for _, p := range ProviderPresets {
		assert.NotEmpty(t, p.Key, "preset key should not be empty")
		assert.NotEmpty(t, p.Name, "preset name should not be empty")
		assert.NotEmpty(t, p.BaseURL, "preset base_url should not be empty")
		assert.True(t, validFormats[p.Format], "preset format should be valid: %s", p.Format)
		assert.False(t, keys[p.Key], "duplicate preset key: %s", p.Key)
		keys[p.Key] = true
	}
}
