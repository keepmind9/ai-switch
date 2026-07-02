package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildAnthropicUsageMap(t *testing.T) {
	tests := []struct {
		name                  string
		prompt, output        int
		cacheRead, cacheCreat int
		wantInput             int
		wantRead              int // -1 means field must be absent
		wantCreation          int // -1 means field must be absent
	}{
		{"no cache", 100, 50, 0, 0, 100, -1, -1},
		{"cache_read only", 100, 50, 80, 0, 20, 80, -1},
		{"cache_read + cache_creation", 100, 50, 60, 20, 20, 60, 20},
		{"saturating clamp when cache exceeds prompt", 100, 10, 60, 50, 0, 60, 50},
		{"all zero", 0, 0, 0, 0, 0, -1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := buildAnthropicUsageMap(tt.prompt, tt.output, tt.cacheRead, tt.cacheCreat)
			assert.Equal(t, tt.wantInput, u["input_tokens"])
			assert.Equal(t, tt.output, u["output_tokens"])

			// Three-bucket invariant holds whenever the prompt is large enough
			// to cover both cache buckets (i.e. no saturating clamp).
			if tt.prompt >= tt.cacheRead+tt.cacheCreat {
				read, _ := u["cache_read_input_tokens"].(int)
				creat, _ := u["cache_creation_input_tokens"].(int)
				assert.Equal(t, tt.prompt, tt.wantInput+read+creat, "three-bucket invariant")
			}

			checkUsageField(t, u, "cache_read_input_tokens", tt.wantRead)
			checkUsageField(t, u, "cache_creation_input_tokens", tt.wantCreation)
		})
	}
}

func checkUsageField(t *testing.T, u map[string]any, key string, want int) {
	t.Helper()
	if want < 0 {
		_, ok := u[key]
		assert.False(t, ok, "%s should be absent", key)
		return
	}
	assert.Equal(t, want, u[key], key)
}

func TestExtractResponsesCacheRead(t *testing.T) {
	// Direct field wins over nested details.
	assert.Equal(t, 50, extractResponsesCacheRead(map[string]any{
		"cache_read_input_tokens": 50,
		"input_tokens_details":    map[string]any{"cached_tokens": 30},
	}))
	// Falls back to OpenAI-standard nested field.
	assert.Equal(t, 30, extractResponsesCacheRead(map[string]any{
		"input_tokens_details": map[string]any{"cached_tokens": 30},
	}))
	// Absent everywhere.
	assert.Equal(t, 0, extractResponsesCacheRead(map[string]any{}))
	// Zero is treated as absent.
	assert.Equal(t, 0, extractResponsesCacheRead(map[string]any{
		"input_tokens_details": map[string]any{"cached_tokens": 0},
	}))
}
