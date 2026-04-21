package converter

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

// AnthropicToChatState tracks state when converting Anthropic SSE to Chat SSE.
type AnthropicToChatState struct {
	ID           string
	Model        string
	Created      int64
	AccText      string
	StopReason   string
	InputTokens  int
	OutputTokens int
	Started      bool
}

// ConvertAnthropicLineToChat processes a raw Anthropic SSE line and returns
// a ChatStreamResponse chunk, or nil if the line should be skipped.
// Returns the string "[DONE]" when the stream is complete.
func ConvertAnthropicLineToChat(state *AnthropicToChatState, line string) any {
	if strings.HasPrefix(line, "event: ") || line == "" {
		return nil
	}

	data := ParseSSEDataLine(line)
	if data == "" {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil
	}

	eventType, _ := raw["type"].(string)

	switch eventType {
	case "message_start":
		msg, _ := raw["message"].(map[string]any)
		if msg != nil {
			state.ID, _ = msg["id"].(string)
			state.Model, _ = msg["model"].(string)
			if usage, ok := msg["usage"].(map[string]any); ok {
				state.InputTokens = int(toFloat64(usage["input_tokens"]))
			}
		}
		state.Created = time.Now().Unix()
		state.Started = true

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{Role: "assistant"}, FinishReason: ""},
			},
		}

	case "content_block_delta":
		delta, _ := raw["delta"].(map[string]any)
		if delta == nil {
			return nil
		}
		text, _ := delta["text"].(string)
		if text == "" {
			return nil
		}
		state.AccText += text
		state.OutputTokens++

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{Content: text}, FinishReason: ""},
			},
		}

	case "message_delta":
		delta, _ := raw["delta"].(map[string]any)
		reason := ""
		if delta != nil {
			r, _ := delta["stop_reason"].(string)
			reason = anthropicStopToChat(r)
			state.StopReason = reason
		}
		if usage, ok := raw["usage"].(map[string]any); ok {
			state.OutputTokens = int(toFloat64(usage["output_tokens"]))
		}

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{}, FinishReason: reason},
			},
		}

	case "message_stop":
		return "[DONE]"
	}

	return nil
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
