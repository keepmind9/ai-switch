package handler

import (
	"testing"

	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProxyModes(t *testing.T) {
	cases := []struct {
		name      string
		spec      string
		wantErr   bool
		isNil     bool
		anthropic bool
		responses bool
		chat      bool
	}{
		{"empty", "", false, true, false, false, false},
		{"claude", "claude", false, false, true, false, false},
		{"codex", "codex", false, false, false, true, false},
		{"chat", "chat", false, false, false, false, true},
		{"all", "all", false, false, true, true, true},
		{"claude,codex", "claude,codex", false, false, true, true, false},
		{"spaces around tokens", "claude, codex", false, false, true, true, false},
		{"surrounding whitespace", " codex , chat ", false, false, false, true, true},
		{"only commas", ",,", false, true, false, false, false},
		{"codex then all short-circuits", "codex,all", false, false, true, true, true},
		{"all then claude", "all,claude", false, false, true, true, true},
		{"unknown token", "foo", true, false, false, false, false},
		{"mixed valid and unknown", "claude,foo", true, false, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			modes, err := ParseProxyModes(c.spec)
			if c.wantErr {
				assert.Error(t, err)
				assert.Nil(t, modes)
				return
			}
			require.NoError(t, err)
			if c.isNil {
				assert.Nil(t, modes)
				return
			}
			require.NotNil(t, modes)
			assert.Equal(t, c.anthropic, modes.Enabled(converter.FormatAnthropic))
			assert.Equal(t, c.responses, modes.Enabled(converter.FormatResponses))
			assert.Equal(t, c.chat, modes.Enabled(converter.FormatChat))
		})
	}
}

func TestAllProxyModes(t *testing.T) {
	modes := AllProxyModes()
	assert.True(t, modes.Enabled(converter.FormatAnthropic))
	assert.True(t, modes.Enabled(converter.FormatResponses))
	assert.True(t, modes.Enabled(converter.FormatChat))
}

func TestProxyModes_EmptyDisabled(t *testing.T) {
	// A nil set (no --proxy) disables proxy mode for every protocol.
	var modes ProxyModes
	assert.False(t, modes.Enabled(converter.FormatAnthropic))
	assert.False(t, modes.Enabled(converter.FormatResponses))
	assert.False(t, modes.Enabled(converter.FormatChat))
}
