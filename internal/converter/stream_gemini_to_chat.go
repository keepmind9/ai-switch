package converter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

// GeminiToChatState tracks state when converting Gemini SSE to Chat SSE.
type GeminiToChatState struct {
	ID           string
	Model        string
	Created      int64
	Started      bool
	InputTokens  int
	OutputTokens int
	AccText      string
	HasToolCalls bool
	ToolCallSeq  int

	// Pending chunks buffered from a single Gemini response that contained multiple parts.
	pending []*types.ChatStreamResponse
}

// ChatStreamUsage returns token usage for the final Chat SSE usage chunk.
func (s *GeminiToChatState) ChatStreamUsage() (id, model string, input, output int) {
	return s.ID, s.Model, s.InputTokens, s.OutputTokens
}

// ConvertGeminiLineToChat processes a Gemini SSE line and buffers Chat SSE chunks.
// Returns the next buffered chunk, or nil if none available.
// Pass an empty string to drain the next buffered chunk without parsing new input.
// The handler must call this in a loop after each upstream line to emit all chunks.
func ConvertGeminiLineToChat(state *GeminiToChatState, line string) any {
	// Always drain buffered chunks first
	if len(state.pending) > 0 {
		chunk := state.pending[0]
		state.pending = state.pending[1:]
		return chunk
	}

	// Empty line = drain-only request
	if len(line) == 0 {
		return nil
	}

	// Skip event-type lines
	if len(line) > 6 && line[:7] == "event: " {
		return nil
	}

	data := ParseSSEDataLine(line)
	if data == "" {
		return nil
	}

	var gemResp GeminiResponse
	if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
		return nil
	}

	// Extract usage metadata
	if gemResp.UsageMetadata != nil {
		state.InputTokens = gemResp.UsageMetadata.PromptTokenCount
		state.OutputTokens = gemResp.UsageMetadata.CandidatesTokenCount
	}

	// No candidates — usage-only or empty
	if len(gemResp.Candidates) == 0 || gemResp.Candidates[0].Content == nil {
		return nil
	}

	candidate := gemResp.Candidates[0]

	// Initialize state on first data
	if !state.Started {
		state.Started = true
		state.Created = time.Now().Unix()
		state.ID = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}

	// Buffer ALL parts as individual chunks — never return directly
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			state.AccText += part.Text
			state.pending = append(state.pending, &types.ChatStreamResponse{
				ID:      state.ID,
				Object:  "chat.completion.chunk",
				Created: state.Created,
				Model:   state.Model,
				Choices: []types.StreamChoice{
					{Index: 0, Delta: types.ChatMessage{Content: strPtr(part.Text)}, FinishReason: ""},
				},
			})
		}

		if part.FunctionCall != nil {
			state.HasToolCalls = true
			chatIdx := state.ToolCallSeq
			state.ToolCallSeq++
			callID := fmt.Sprintf("call_%d", chatIdx)
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)

			state.pending = append(state.pending, &types.ChatStreamResponse{
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
									ID:       callID,
									Type:     "function",
									Function: types.FunctionCall{Name: part.FunctionCall.Name, Arguments: string(argsJSON)},
								},
							},
						},
						FinishReason: "",
					},
				},
			})
		}
	}

	// Buffer finishReason chunk if present
	if candidate.FinishReason != "" {
		finishReason := "stop"
		if state.HasToolCalls {
			finishReason = "tool_calls"
		}
		state.pending = append(state.pending, &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{}, FinishReason: finishReason},
			},
		})
	}

	// Return first buffered chunk if any
	if len(state.pending) > 0 {
		chunk := state.pending[0]
		state.pending = state.pending[1:]
		return chunk
	}

	return nil
}
