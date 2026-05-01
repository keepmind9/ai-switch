package handler

import (
	"encoding/json"
	"github.com/keepmind9/ai-switch/internal/util"
)

// streamUsageAccumulator extracts token usage from SSE data lines during streaming.
type streamUsageAccumulator struct {
	InputTokens       int64
	OutputTokens      int64
	CacheCreateTokens int64
	CacheReadTokens   int64
}

func (a *streamUsageAccumulator) sniff(data string, format string) {
	if data == "" || data == "[DONE]" {
		return
	}

	var raw map[string]any
	if json.Unmarshal([]byte(data), &raw) != nil {
		return
	}

	switch format {
	case "anthropic":
		a.sniffAnthropic(raw)
	case "responses":
		a.sniffResponses(raw)
	default:
		a.sniffChat(raw)
	}
}

func (a *streamUsageAccumulator) sniffAnthropic(raw map[string]any) {
	eventType, _ := raw["type"].(string)
	switch eventType {
	case "message_start":
		if msg, ok := raw["message"].(map[string]any); ok {
			if usage, ok := msg["usage"].(map[string]any); ok {
				a.InputTokens = int64(util.ToFloat64(usage["input_tokens"]))
				a.CacheCreateTokens = int64(util.ToFloat64(usage["cache_creation_input_tokens"]))
				a.CacheReadTokens = int64(util.ToFloat64(usage["cache_read_input_tokens"]))
			}
		}
	case "message_delta":
		if usage, ok := raw["usage"].(map[string]any); ok {
			a.OutputTokens = int64(util.ToFloat64(usage["output_tokens"]))
			if in := int64(util.ToFloat64(usage["input_tokens"])); in > 0 {
				a.InputTokens = in
			}
			if c := int64(util.ToFloat64(usage["cache_creation_input_tokens"])); c > 0 {
				a.CacheCreateTokens = c
			}
			if c := int64(util.ToFloat64(usage["cache_read_input_tokens"])); c > 0 {
				a.CacheReadTokens = c
			}
		}
	}
}

func (a *streamUsageAccumulator) sniffResponses(raw map[string]any) {
	eventType, _ := raw["type"].(string)
	if eventType != "response.completed" {
		return
	}
	if resp, ok := raw["response"].(map[string]any); ok {
		if usage, ok := resp["usage"].(map[string]any); ok {
			a.InputTokens = int64(util.ToFloat64(usage["input_tokens"]))
			a.OutputTokens = int64(util.ToFloat64(usage["output_tokens"]))
			a.CacheCreateTokens = int64(util.ToFloat64(usage["cache_creation_input_tokens"]))
			a.CacheReadTokens = int64(util.ToFloat64(usage["cache_read_input_tokens"]))
		}
	}
}

func (a *streamUsageAccumulator) sniffChat(raw map[string]any) {
	if usage, ok := raw["usage"].(map[string]any); ok {
		if in := int64(util.ToFloat64(usage["prompt_tokens"])); in > 0 {
			a.InputTokens = in
		}
		if out := int64(util.ToFloat64(usage["completion_tokens"])); out > 0 {
			a.OutputTokens = out
		}
		// OpenAI format: prompt_tokens_details.cached_tokens
		if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
			a.CacheReadTokens = int64(util.ToFloat64(details["cached_tokens"]))
		}
		// DeepSeek/MiniMax format: prompt_cache_hit_tokens
		if hit := int64(util.ToFloat64(usage["prompt_cache_hit_tokens"])); hit > 0 {
			a.CacheReadTokens = hit
		}
	}
}
