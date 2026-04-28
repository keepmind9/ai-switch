package converter

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/keepmind9/ai-switch/internal/util"
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

	// Tool use tracking: Anthropic block index → tool block info
	ToolBlocks      map[int]*anthropicToolBlock
	ToolCallCounter int
}

type anthropicToolBlock struct {
	ChatIndex int
	ID        string
	Name      string
}

// ChatStreamUsage returns token usage for the final Chat SSE usage chunk.
func (s *AnthropicToChatState) ChatStreamUsage() (id, model string, input, output int) {
	return s.ID, s.Model, s.InputTokens, s.OutputTokens
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

	case "content_block_start":
		block, _ := raw["content_block"].(map[string]any)
		if block == nil {
			return nil
		}
		blockType, _ := block["type"].(string)
		blockIdx := int(toFloat64(raw["index"]))

		if blockType == "tool_use" {
			if state.ToolBlocks == nil {
				state.ToolBlocks = make(map[int]*anthropicToolBlock)
			}
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			chatIdx := state.ToolCallCounter
			state.ToolCallCounter++

			state.ToolBlocks[blockIdx] = &anthropicToolBlock{
				ChatIndex: chatIdx,
				ID:        id,
				Name:      name,
			}

			return &types.ChatStreamResponse{
				ID:      state.ID,
				Object:  "chat.completion.chunk",
				Created: state.Created,
				Model:   state.Model,
				Choices: []types.StreamChoice{
					{
						Index: 0,
						Delta: types.ChatMessage{
							ToolCalls: []types.ToolCall{
								{
									Index:    chatIdx,
									ID:       id,
									Type:     "function",
									Function: types.FunctionCall{Name: name},
								},
							},
						},
						FinishReason: "",
					},
				},
			}
		}
		return nil

	case "content_block_delta":
		delta, _ := raw["delta"].(map[string]any)
		if delta == nil {
			return nil
		}
		deltaType, _ := delta["type"].(string)

		if deltaType == "input_json_delta" {
			blockIdx := int(toFloat64(raw["index"]))
			partialJSON, _ := delta["partial_json"].(string)
			tb := state.ToolBlocks[blockIdx]
			if tb == nil {
				return nil
			}

			return &types.ChatStreamResponse{
				ID:      state.ID,
				Object:  "chat.completion.chunk",
				Created: state.Created,
				Model:   state.Model,
				Choices: []types.StreamChoice{
					{
						Index: 0,
						Delta: types.ChatMessage{
							ToolCalls: []types.ToolCall{
								{
									Index:    tb.ChatIndex,
									Function: types.FunctionCall{Arguments: partialJSON},
								},
							},
						},
						FinishReason: "",
					},
				},
			}
		}

		// text delta
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
				{Index: 0, Delta: types.ChatMessage{Content: strPtr(text)}, FinishReason: ""},
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
			if in := int(toFloat64(usage["input_tokens"])); in > 0 {
				state.InputTokens = in
			}
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
	return util.ToFloat64(v)
}
