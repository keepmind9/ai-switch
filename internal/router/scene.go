package router

import (
	"encoding/json"
	"strings"
)

// SceneConfig holds configuration for scene detection.
type SceneConfig struct {
	LongContextThreshold int
}

// DetectScene detects the Claude Code scenario from an Anthropic request body.
// Detection priority (matching cc-router):
// 1. longContext — token count exceeds threshold (> 0 to enable)
// 2. background — model name contains "haiku"
// 3. websearch — tools contain web_search_* type
// 4. think — thinking field present
// 5. image — user messages contain image content blocks
// 6. default — fallback
func DetectScene(body []byte, cfg SceneConfig) string {
	var req struct {
		Thinking any `json:"thinking"`
		Tools    []struct {
			Type string `json:"type"`
		} `json:"tools"`
		Model    string         `json:"model"`
		Messages []sceneMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "default"
	}

	// Priority 1: longContext
	if cfg.LongContextThreshold > 0 {
		tokenCount := countTokens(body)
		if tokenCount > cfg.LongContextThreshold {
			return "longContext"
		}
	}

	// Priority 2: background — haiku model
	if strings.Contains(strings.ToLower(req.Model), "haiku") {
		return "background"
	}

	// Priority 3: websearch
	for _, tool := range req.Tools {
		if strings.HasPrefix(tool.Type, "web_search_") {
			return "websearch"
		}
	}

	// Priority 4: think
	if req.Thinking != nil {
		return "think"
	}

	// Priority 5: image
	if hasImageContent(req.Messages) {
		return "image"
	}

	return "default"
}

type sceneMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func hasImageContent(messages []sceneMessage) bool {
	for _, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		var blocks []struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(msg.Content, &blocks) == nil {
			for _, b := range blocks {
				if b.Type == "image" {
					return true
				}
			}
		}
	}
	return false
}
