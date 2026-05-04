package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests ---

func TestBuildCompactionResponse(t *testing.T) {
	input := []any{
		map[string]any{"role": "user", "content": "hello"},
		map[string]any{"role": "assistant", "content": "hi there"},
	}

	encrypted := "aisw_dGVzdA==" // base64 of "test"
	resp := buildCompactionResponse(encrypted, input)

	assert.Equal(t, "response.compaction", resp["object"])
	assert.Contains(t, resp["id"], "resp_compact_")

	output := resp["output"].([]any)
	// First item: retained user message
	userMsg := output[0].(map[string]any)
	assert.Equal(t, "message", userMsg["type"])
	assert.Equal(t, "user", userMsg["role"])
	assert.Equal(t, "completed", userMsg["status"])

	content := userMsg["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "hello", content[0].(map[string]any)["text"])

	// Second item: compaction
	compaction := output[1].(map[string]any)
	assert.Equal(t, "compaction", compaction["type"])
	assert.Equal(t, encrypted, compaction["encrypted_content"])
	assert.True(t, strings.HasPrefix(compaction["encrypted_content"].(string), "aisw_"))
}

func TestBuildCompactionResponse_NonArrayInput(t *testing.T) {
	resp := buildCompactionResponse("aisw_test", "string input")

	assert.Equal(t, "response.compaction", resp["object"])
	output := resp["output"].([]any)
	// Only the compaction item, no user message (string input has no role)
	require.Len(t, output, 1)
	assert.Equal(t, "compaction", output[0].(map[string]any)["type"])
}

func TestBuildCompactionResponse_NoUserMessage(t *testing.T) {
	input := []any{
		map[string]any{"role": "assistant", "content": "hi"},
	}
	resp := buildCompactionResponse("aisw_test", input)

	output := resp["output"].([]any)
	require.Len(t, output, 1)
	assert.Equal(t, "compaction", output[0].(map[string]any)["type"])
}

func TestExtractSummaryFromResponse_Chat(t *testing.T) {
	content := "This is a summary."
	chatResp := types.ChatResponse{
		ID:     "chatcmpl-test",
		Object: "chat.completion",
		Choices: []types.ChatChoice{
			{
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: &content,
				},
			},
		},
	}
	body, err := json.Marshal(chatResp)
	require.NoError(t, err)

	summary := extractSummaryFromResponse(body, converter.FormatChat)
	assert.Equal(t, "This is a summary.", summary)
}

func TestExtractSummaryFromResponse_ChatEmpty(t *testing.T) {
	chatResp := types.ChatResponse{
		ID:     "chatcmpl-test",
		Object: "chat.completion",
		Choices: []types.ChatChoice{
			{
				Message: types.ChatMessage{Role: "assistant"},
			},
		},
	}
	body, _ := json.Marshal(chatResp)
	assert.Equal(t, "", extractSummaryFromResponse(body, converter.FormatChat))
}

func TestExtractSummaryFromResponse_Anthropic(t *testing.T) {
	anthResp := converter.AnthropicResponse{
		Content: []converter.AnthropicContentBlock{
			{Type: "text", Text: "Anthropic summary"},
		},
	}
	body, _ := json.Marshal(anthResp)
	summary := extractSummaryFromResponse(body, converter.FormatAnthropic)
	assert.Equal(t, "Anthropic summary", summary)
}

func TestExtractSummaryFromResponse_Gemini(t *testing.T) {
	gemResp := converter.GeminiResponse{
		Candidates: []converter.GeminiCandidate{
			{
				Content: &converter.GeminiContent{
					Parts: []converter.GeminiPart{{Text: "Gemini summary"}},
				},
			},
		},
	}
	body, _ := json.Marshal(gemResp)
	summary := extractSummaryFromResponse(body, converter.FormatGemini)
	assert.Equal(t, "Gemini summary", summary)
}

func TestExtractSummaryFromResponse_InvalidJSON(t *testing.T) {
	assert.Equal(t, "", extractSummaryFromResponse([]byte("not json"), converter.FormatChat))
	assert.Equal(t, "", extractSummaryFromResponse([]byte("not json"), converter.FormatAnthropic))
	assert.Equal(t, "", extractSummaryFromResponse([]byte("not json"), converter.FormatGemini))
}

func TestExtractInputTextFromItem(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected string
	}{
		{
			name:     "string content",
			input:    map[string]any{"role": "user", "content": "hello world"},
			expected: "hello world",
		},
		{
			name: "array content",
			input: map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "part1"},
					map[string]any{"type": "text", "text": "part2"},
				},
			},
			expected: "part1\npart2",
		},
		{
			name:     "no content",
			input:    map[string]any{"role": "user"},
			expected: "",
		},
		{
			name:     "nil content",
			input:    map[string]any{"role": "user", "content": nil},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractInputTextFromItem(tt.input))
		})
	}
}

// --- Integration tests ---

func TestHandleCompact_Passthrough(t *testing.T) {
	r, _ := setupRouter(t, "responses", func(w http.ResponseWriter, r *http.Request) {
		// Verify the upstream receives /compact path
		assert.True(t, strings.HasSuffix(r.URL.Path, "/compact"),
			"expected path ending with /compact, got %s", r.URL.Path)

		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp-compact-test",
			"object": "response.compaction",
			"output": []map[string]any{
				{"type": "compaction", "encrypted_content": "real-openai-blob"},
			},
		})
	})

	w := doRequest(r, "POST", "/v1/responses/compact", `{
		"model": "gpt-4o",
		"input": "Summarize this"
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "resp-compact-test", resp["id"])
}

func TestHandleCompact_SimulatedChat(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)

		// Verify summarization prompt structure
		assert.Contains(t, req["model"], "test-model")

		resp := map[string]any{
			"id":      "chatcmpl-summary",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   req["model"],
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Compact summary of the conversation.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	w := doRequest(r, "POST", "/v1/responses/compact", `{
		"model": "gpt-4o",
		"input": [
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi"}
		]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "response.compaction", resp["object"])

	output := resp["output"].([]any)
	require.Len(t, output, 2)

	// First item: retained user message
	userMsg := output[0].(map[string]any)
	assert.Equal(t, "message", userMsg["type"])
	assert.Equal(t, "user", userMsg["role"])

	// Second item: compaction with aisw_ prefix
	compaction := output[1].(map[string]any)
	assert.Equal(t, "compaction", compaction["type"])
	encrypted := compaction["encrypted_content"].(string)
	assert.True(t, strings.HasPrefix(encrypted, "aisw_"))

	// Verify the payload decodes correctly
	payload, err := converter.DecodeCompactionPayload(encrypted)
	require.NoError(t, err)
	assert.Equal(t, "Compact summary of the conversation.", payload.Summary)
}

func TestHandleCompact_InvalidJSON(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "POST", "/v1/responses/compact", `not json`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCompact_EmptyInput(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {})

	w := doRequest(r, "POST", "/v1/responses/compact", `{
		"model": "gpt-4o",
		"input": ""
	}`)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCompact_UpstreamError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	})

	w := doRequest(r, "POST", "/v1/responses/compact", `{
		"model": "gpt-4o",
		"input": [
			{"role": "user", "content": "hello"}
		]
	}`)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestForwardCompactPassthrough_UpstreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/responses/compact", strings.NewReader(`{"model":"gpt-4o"}`))

	result := &router.RouteResult{
		BaseURL: ts.URL,
		APIKey:  "test-key",
		Format:  "responses",
		Path:    "/v1/responses",
	}

	h.forwardCompactPassthrough(c, []byte(`{"model":"gpt-4o"}`), result)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestForwardCompactPassthrough_UpstreamSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp-compact",
			"object": "response.compaction",
			"output": []any{},
		})
	}))
	defer ts.Close()

	h := NewHandler(nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/responses/compact", strings.NewReader(`{}`))

	result := &router.RouteResult{
		BaseURL: ts.URL,
		APIKey:  "test-key",
		Format:  "responses",
		Path:    "/v1/responses",
	}

	h.forwardCompactPassthrough(c, []byte(`{}`), result)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/v1/responses/compact", requestedPath)
}
