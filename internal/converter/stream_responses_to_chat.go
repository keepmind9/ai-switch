package converter

import (
	"encoding/json"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

// ResponsesToChatState tracks state when converting Responses SSE to Chat SSE.
type ResponsesToChatState struct {
	ID           string
	Model        string
	Created      int64
	AccText      string
	Started      bool
	InputTokens  int
	OutputTokens int
	HasToolCalls bool

	// Tool call tracking: item_id -> call info
	ToolCallItems map[string]*responsesToChatTC
	ToolCallSeq   int
}

type responsesToChatTC struct {
	CallID    string
	Name      string
	Args      string
	ChatIndex int
}

// ChatStreamUsage returns token usage for the final Chat SSE usage chunk.
func (s *ResponsesToChatState) ChatStreamUsage() (id, model string, input, output int) {
	return s.ID, s.Model, s.InputTokens, s.OutputTokens
}

// ConvertResponsesLineToChat processes a raw Responses SSE line and returns
// a ChatStreamResponse chunk, or nil if the line should be skipped.
// Returns the string "[DONE]" when the stream is complete.
func ConvertResponsesLineToChat(state *ResponsesToChatState, line string) any {
	if line == "" {
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
	case "response.created":
		resp, _ := raw["response"].(map[string]any)
		if resp != nil {
			state.ID, _ = resp["id"].(string)
			state.Model, _ = resp["model"].(string)
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

	case "response.output_text.delta":
		delta, _ := raw["delta"].(string)
		if delta == "" {
			return nil
		}
		state.AccText += delta

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{Content: strPtr(delta)}, FinishReason: ""},
			},
		}

	case "response.output_item.added":
		item, _ := raw["item"].(map[string]any)
		if item == nil {
			return nil
		}
		itemType, _ := item["type"].(string)
		if itemType != "function_call" {
			return nil
		}

		itemID, _ := item["id"].(string)
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		state.HasToolCalls = true
		if state.ToolCallItems == nil {
			state.ToolCallItems = make(map[string]*responsesToChatTC)
		}
		chatIdx := state.ToolCallSeq
		state.ToolCallSeq++
		state.ToolCallItems[itemID] = &responsesToChatTC{
			CallID:    callID,
			Name:      name,
			ChatIndex: chatIdx,
		}

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{
					ToolCalls: []types.ToolCall{
						{Index: chatIdx, ID: callID, Type: "function", Function: types.FunctionCall{Name: name}},
					},
				}, FinishReason: ""},
			},
		}

	case "response.function_call_arguments.delta":
		delta, _ := raw["delta"].(string)
		itemID, _ := raw["item_id"].(string)
		if delta == "" || itemID == "" {
			return nil
		}
		tc := state.ToolCallItems[itemID]
		if tc == nil {
			return nil
		}
		tc.Args += delta

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{
					ToolCalls: []types.ToolCall{
						{Index: tc.ChatIndex, Function: types.FunctionCall{Arguments: delta}},
					},
				}, FinishReason: ""},
			},
		}

	case "response.function_call_arguments.done":
		return nil

	case "response.completed":
		if resp, ok := raw["response"].(map[string]any); ok {
			if usage, ok := resp["usage"].(map[string]any); ok {
				state.InputTokens = int(toFloat64(usage["input_tokens"]))
				state.OutputTokens = int(toFloat64(usage["output_tokens"]))
			}
		}
		finishReason := "stop"
		if state.HasToolCalls {
			finishReason = "tool_calls"
		}
		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{}, FinishReason: finishReason},
			},
		}

	case "response.output_item.done", "response.content_part.done",
		"response.output_text.done", "response.content_part.added":
		return nil
	}

	return nil
}
