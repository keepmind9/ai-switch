package converter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
)

// mockSSEWriter collects emitted SSE events for testing.
type mockGeminiSSEWriter struct {
	events []struct {
		EventType string
		Data      any
	}
}

func (m *mockGeminiSSEWriter) WriteEvent(eventType string, data any) {
	m.events = append(m.events, struct {
		EventType string
		Data      any
	}{eventType, data})
}

// --- Gemini → Chat SSE ---

func TestConvertGeminiLineToChat_Text(t *testing.T) {
	state := &GeminiToChatState{Model: "gemini-2.5-pro"}

	// First response with text — emits text delta directly (no separate role chunk)
	gemData := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`
	result := ConvertGeminiLineToChat(state, "data: "+gemData)
	require.NotNil(t, result)
	chunk, ok := result.(*types.ChatStreamResponse)
	require.True(t, ok)
	assert.Equal(t, "Hello", derefStr(chunk.Choices[0].Delta.Content))
	assert.True(t, state.Started)

	// Second response — more text
	result = ConvertGeminiLineToChat(state, "data: "+gemData)
	chunk = result.(*types.ChatStreamResponse)
	assert.Equal(t, "Hello", derefStr(chunk.Choices[0].Delta.Content))
}

func TestConvertGeminiLineToChat_FunctionCall(t *testing.T) {
	state := &GeminiToChatState{Model: "gemini-2.5-pro"}

	// Init
	initData := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi"}]}}]}`
	ConvertGeminiLineToChat(state, "data: "+initData)

	// Function call
	fcData := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}]},"finishReason":"STOP"}]}`
	result := ConvertGeminiLineToChat(state, "data: "+fcData)
	chunk := result.(*types.ChatStreamResponse)
	assert.Equal(t, "get_weather", chunk.Choices[0].Delta.ToolCalls[0].Function.Name)
}

func TestConvertGeminiLineToChat_Usage(t *testing.T) {
	state := &GeminiToChatState{Model: "gemini-2.5-pro"}

	// Init
	ConvertGeminiLineToChat(state, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"x"}]}}]}`)

	// Final with usage
	usageData := `{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	ConvertGeminiLineToChat(state, "data: "+usageData)
	assert.Equal(t, 10, state.InputTokens)
	assert.Equal(t, 5, state.OutputTokens)
}

// --- Gemini → Anthropic SSE ---

func TestConvertGeminiLineToAnthropicSSE_Text(t *testing.T) {
	w := &mockGeminiSSEWriter{}
	state := &GeminiToAnthropicState{Model: "gemini-2.5-pro"}

	gemData := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}]}`
	done := ConvertGeminiLineToAnthropicSSE(w, state, gemData)
	assert.True(t, done)

	// Should have message_start, content_block_start, content_block_delta, content_block_stop, message_delta, message_stop
	assert.True(t, len(w.events) >= 5)

	// Verify message_start
	assert.Equal(t, "message_start", w.events[0].EventType)
	// Verify message_stop at end
	lastEvent := w.events[len(w.events)-1]
	assert.Equal(t, "message_stop", lastEvent.EventType)
}

func TestConvertGeminiLineToAnthropicSSE_FunctionCall(t *testing.T) {
	w := &mockGeminiSSEWriter{}
	state := &GeminiToAnthropicState{Model: "gemini-2.5-pro"}

	gemData := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}]},"finishReason":"STOP"}]}`
	done := ConvertGeminiLineToAnthropicSSE(w, state, gemData)
	assert.True(t, done)
	assert.True(t, state.HasToolUse)
}

// --- Gemini → Responses SSE ---

func TestConvertGeminiLineToResponsesSSE_Text(t *testing.T) {
	w := &mockGeminiSSEWriter{}
	state := &GeminiToResponsesState{Model: "gemini-2.5-pro"}

	gemData := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}]}`
	done := ConvertGeminiLineToResponsesSSE(w, state, gemData)
	assert.True(t, done)

	// Should end with response.completed
	lastEvent := w.events[len(w.events)-1]
	assert.Equal(t, "response.completed", lastEvent.EventType)
}

func TestConvertGeminiLineToResponsesSSE_FunctionCall(t *testing.T) {
	w := &mockGeminiSSEWriter{}
	state := &GeminiToResponsesState{Model: "gemini-2.5-pro"}

	gemData := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"search","args":{"q":"test"}}}]},"finishReason":"STOP"}]}`
	done := ConvertGeminiLineToResponsesSSE(w, state, gemData)
	assert.True(t, done)
}

// --- Round-trip: Chat → Gemini request → Gemini response → Chat response ---

func TestRoundTrip_ChatToGeminiAndBack(t *testing.T) {
	c := NewConverter()

	chatReq := &types.ChatRequest{
		Model: "gemini-2.5-pro",
		Messages: []types.ChatMessage{
			{Role: "system", Content: strPtr("Be helpful")},
			{Role: "user", Content: strPtr("Hello")},
		},
		MaxTokens: 100,
	}

	_, err := c.ChatToGeminiRequest(chatReq)
	require.NoError(t, err)

	// Simulate Gemini response
	gemResp := &GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content:      &GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "Hi there!"}}},
			FinishReason: "STOP",
		}},
		UsageMetadata: &GeminiUsageMeta{PromptTokenCount: 5, CandidatesTokenCount: 3, TotalTokenCount: 8},
	}

	chatResp, err := c.GeminiResponseToChat(gemResp, "gemini-2.5-pro")
	require.NoError(t, err)

	assert.Equal(t, "stop", chatResp.Choices[0].FinishReason)
	assert.Equal(t, "Hi there!", derefStr(chatResp.Choices[0].Message.Content))
	assert.Equal(t, 5, chatResp.Usage.PromptTokens)
	assert.Equal(t, 3, chatResp.Usage.CompletionTokens)
}

// --- Helpers ---

// Verify GeminiRequest JSON serialization
func TestGeminiRequestJSON(t *testing.T) {
	req := &GeminiRequest{
		Contents: []GeminiContent{
			{Role: "user", Parts: []GeminiPart{{Text: "Hello"}}},
		},
		GenerationConfig: &GeminiGenerationConfig{MaxOutputTokens: 1024},
	}
	data, err := json.Marshal(req)
	assert.NoError(t, err)
	assert.True(t, bytes.Contains(data, []byte(`"Hello"`)))
	assert.True(t, bytes.Contains(data, []byte(`"maxOutputTokens":1024`)))
}
