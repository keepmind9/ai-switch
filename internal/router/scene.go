package router

import (
	"encoding/json"
	"strings"
)

// DetectScene detects the Claude Code scenario from an Anthropic request body.
// Detection priority (same as cc-router):
// 1. thinking field present → "think"
// 2. tools array contains web_search tool → "websearch"
// 3. model field contains "haiku" → "background"
// 4. fallback → "default"
func DetectScene(body []byte) string {
	var req struct {
		Thinking any `json:"thinking"`
		Tools    []struct {
			Type string `json:"type"`
		} `json:"tools"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "default"
	}

	// Priority 1: thinking field present
	if req.Thinking != nil {
		return "think"
	}

	// Priority 2: web search tools
	for _, tool := range req.Tools {
		if strings.HasPrefix(tool.Type, "web_search_") {
			return "websearch"
		}
	}

	// Priority 3: haiku model → background task
	if strings.Contains(strings.ToLower(req.Model), "haiku") {
		return "background"
	}

	return "default"
}
