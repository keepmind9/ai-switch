package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatToGeminiRequest_Simple(t *testing.T) {
	c := NewConverter()
	req := &types.ChatRequest{
		Model:    "gemini-2.5-pro",
		Messages: []types.ChatMessage{{Role: "user", Content: strPtr("Hello")}},
	}
	gemReq, err := c.ChatToGeminiRequest(req)
	require.NoError(t, err)

	assert.Equal(t, "user", gemReq.Contents[0].Role)
	assert.Equal(t, "Hello", gemReq.Contents[0].Parts[0].Text)
}

func TestChatToGeminiRequest_WithSystemAndTools(t *testing.T) {
	c := NewConverter()
	req := &types.ChatRequest{
		Model:       "gemini-2.5-pro",
		MaxTokens:   2048,
		Temperature: 0.7,
		TopP:        0.9,
		Messages: []types.ChatMessage{
			{Role: "system", Content: strPtr("You are helpful.")},
			{Role: "user", Content: strPtr("Hello")},
			{Role: "assistant", Content: strPtr("Hi there!"), ToolCalls: []types.ToolCall{
				{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "get_weather", Arguments: `{"city":"SF"}`}},
			}},
			{Role: "tool", ToolCallID: "call_1", Content: strPtr(`{"temp":72}`)},
		},
		Tools: []types.Tool{
			{Type: "function", Function: types.FunctionDef{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}}},
		},
		ToolChoice: "auto",
	}

	gemReq, err := c.ChatToGeminiRequest(req)
	require.NoError(t, err)

	// System instruction
	assert.NotNil(t, gemReq.SystemInstruction)
	assert.Equal(t, "You are helpful.", gemReq.SystemInstruction.Parts[0].Text)

	// User message
	assert.Equal(t, "user", gemReq.Contents[0].Role)
	assert.Equal(t, "Hello", gemReq.Contents[0].Parts[0].Text)

	// Assistant message with tool call
	assert.Equal(t, "model", gemReq.Contents[1].Role)
	assert.Equal(t, "Hi there!", gemReq.Contents[1].Parts[0].Text)
	assert.NotNil(t, gemReq.Contents[1].Parts[1].FunctionCall)
	assert.Equal(t, "get_weather", gemReq.Contents[1].Parts[1].FunctionCall.Name)

	// Tool result
	assert.Equal(t, "user", gemReq.Contents[2].Role)
	assert.NotNil(t, gemReq.Contents[2].Parts[0].FunctionResp)

	// Tools
	require.Len(t, gemReq.Tools, 1)
	assert.Equal(t, "get_weather", gemReq.Tools[0].FunctionDeclarations[0].Name)

	// Tool config
	assert.NotNil(t, gemReq.ToolConfig)
	assert.Equal(t, "AUTO", gemReq.ToolConfig.FunctionCallingConfig.Mode)

	// Generation config
	assert.NotNil(t, gemReq.GenerationConfig)
	assert.Equal(t, 2048, gemReq.GenerationConfig.MaxOutputTokens)
	assert.Equal(t, 0.7, gemReq.GenerationConfig.Temperature)
	assert.Equal(t, 0.9, gemReq.GenerationConfig.TopP)
}

func TestGeminiResponseToChat_Text(t *testing.T) {
	c := NewConverter()
	gemResp := &GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content: &GeminiContent{
				Role:  "model",
				Parts: []GeminiPart{{Text: "Hello from Gemini!"}},
			},
			FinishReason: "STOP",
		}},
		UsageMetadata: &GeminiUsageMeta{PromptTokenCount: 10, CandidatesTokenCount: 5, TotalTokenCount: 15},
	}

	chatResp, err := c.GeminiResponseToChat(gemResp, "gemini-2.5-pro")
	require.NoError(t, err)

	assert.Equal(t, "stop", chatResp.Choices[0].FinishReason)
	assert.Equal(t, "Hello from Gemini!", derefStr(chatResp.Choices[0].Message.Content))
	assert.Equal(t, 10, chatResp.Usage.PromptTokens)
	assert.Equal(t, 5, chatResp.Usage.CompletionTokens)
}

func TestGeminiResponseToChat_FunctionCall(t *testing.T) {
	c := NewConverter()
	gemResp := &GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content: &GeminiContent{
				Role: "model",
				Parts: []GeminiPart{{
					FunctionCall: &GeminiFuncCall{
						Name: "get_weather",
						Args: map[string]any{"city": "SF"},
					},
				}},
			},
			FinishReason: "STOP",
		}},
	}

	chatResp, err := c.GeminiResponseToChat(gemResp, "gemini-2.5-pro")
	require.NoError(t, err)

	assert.Equal(t, "tool_calls", chatResp.Choices[0].FinishReason)
	require.Len(t, chatResp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "get_weather", chatResp.Choices[0].Message.ToolCalls[0].Function.Name)

	var args map[string]any
	json.Unmarshal([]byte(chatResp.Choices[0].Message.ToolCalls[0].Function.Arguments), &args)
	assert.Equal(t, "SF", args["city"])
}

func TestResponsesToGeminiRequest(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:        "gemini-2.5-pro",
		Input:        "Hello",
		Instructions: "Be helpful",
	}
	gemReq, err := c.ResponsesToGeminiRequest(req)
	require.NoError(t, err)

	assert.NotNil(t, gemReq.SystemInstruction)
	assert.Equal(t, "Be helpful", gemReq.SystemInstruction.Parts[0].Text)
	assert.Equal(t, "user", gemReq.Contents[0].Role)
	assert.Equal(t, "Hello", gemReq.Contents[0].Parts[0].Text)
}

func TestAnthropicToGeminiRequest(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model:     "gemini-2.5-pro",
		MaxTokens: 4096,
		System:    "You are helpful.",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	gemReq, err := c.AnthropicToGeminiRequest(req)
	require.NoError(t, err)

	assert.NotNil(t, gemReq.SystemInstruction)
	assert.Equal(t, "You are helpful.", gemReq.SystemInstruction.Parts[0].Text)
	assert.Equal(t, "user", gemReq.Contents[0].Role)
	assert.Equal(t, "Hello", gemReq.Contents[0].Parts[0].Text)
}

func TestGeminiResponseToAnthropic(t *testing.T) {
	c := NewConverter()
	gemResp := &GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content: &GeminiContent{
				Role:  "model",
				Parts: []GeminiPart{{Text: "Hi from Gemini!"}},
			},
			FinishReason: "STOP",
		}},
		UsageMetadata: &GeminiUsageMeta{PromptTokenCount: 8, CandidatesTokenCount: 3},
	}

	anthResp, err := c.GeminiResponseToAnthropic(gemResp, "gemini-2.5-pro", "")
	require.NoError(t, err)

	assert.Equal(t, "end_turn", anthResp.StopReason)
	require.Len(t, anthResp.Content, 1)
	assert.Equal(t, "text", anthResp.Content[0].Type)
	assert.Equal(t, "Hi from Gemini!", anthResp.Content[0].Text)
}

func TestGeminiResponseToResponses(t *testing.T) {
	c := NewConverter()
	gemResp := &GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content: &GeminiContent{
				Role:  "model",
				Parts: []GeminiPart{{Text: "Hi from Gemini!"}},
			},
			FinishReason: "STOP",
		}},
		UsageMetadata: &GeminiUsageMeta{PromptTokenCount: 8, CandidatesTokenCount: 3},
	}

	respResp, err := c.GeminiResponseToResponses(gemResp, "gemini-2.5-pro", "")
	require.NoError(t, err)

	assert.Equal(t, "response", respResp.Object)
	require.Len(t, respResp.Responses, 1)
	assert.Equal(t, "message", respResp.Responses[0].Type)
	assert.Equal(t, "completed", respResp.Responses[0].Status)
}
