package handler

import (
	"fmt"
	"strings"

	"github.com/keepmind9/ai-switch/internal/converter"
)

// ProxyModes tracks which client protocols run in proxy (same-format
// passthrough) mode. Protocols absent from the map use the conversion
// pipeline. A nil/empty map disables proxy mode entirely.
//
// Use ParseProxyModes to build one from the --proxy flag value.
type ProxyModes map[string]bool

// Enabled reports whether the given client protocol runs in proxy mode.
func (p ProxyModes) Enabled(protocol string) bool {
	return p[protocol]
}

// proxyAliases maps user-facing --proxy tokens to internal protocol formats.
var proxyAliases = map[string]string{
	"claude": converter.FormatAnthropic,
	"codex":  converter.FormatResponses,
	"chat":   converter.FormatChat,
}

// AllProxyModes returns a ProxyModes with every client protocol enabled,
// equivalent to --proxy all. Useful for tests.
func AllProxyModes() ProxyModes {
	m := make(ProxyModes, len(proxyAliases))
	for _, proto := range proxyAliases {
		m[proto] = true
	}
	return m
}

// ParseProxyModes parses a comma-separated --proxy value (e.g. "claude,codex",
// "all") into a ProxyModes set. An empty string yields a nil (all-disabled)
// set. The token "all" enables every protocol. Whitespace around tokens is
// trimmed. Unknown tokens are rejected with an error.
func ParseProxyModes(spec string) (ProxyModes, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	modes := make(ProxyModes)
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if tok == "all" {
			return AllProxyModes(), nil
		}
		proto, ok := proxyAliases[tok]
		if !ok {
			return nil, fmt.Errorf("invalid --proxy value %q: must be one of claude, codex, chat, all (comma-separated)", tok)
		}
		modes[proto] = true
	}
	if len(modes) == 0 {
		return nil, nil
	}
	return modes, nil
}
