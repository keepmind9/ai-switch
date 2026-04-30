package handler

import (
	"encoding/json"
	"github.com/keepmind9/ai-switch/internal/util"
)

// streamUsageAccumulator extracts token usage from SSE data lines during streaming.
type streamUsageAccumulator struct {
	InputTokens  int64
	OutputTokens int64
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
			}
		}
	case "message_delta":
		if usage, ok := raw["usage"].(map[string]any); ok {
			a.OutputTokens = int64(util.ToFloat64(usage["output_tokens"]))
			if in := int64(util.ToFloat64(usage["input_tokens"])); in > 0 {
				a.InputTokens = in
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
	}
}
