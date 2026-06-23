package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/hook"
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

func TestHandleCompact_Passthrough_CustomHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		assert.True(t, strings.HasSuffix(r.URL.Path, "/compact"))
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp-compact-ua",
			"object": "response.compaction",
			"output": []map[string]any{{"type": "compaction", "encrypted_content": "blob"}},
		})
	}))
	t.Cleanup(ts.Close)

	cfg := &config.Config{
		DefaultRoute: "gw-default",
		Providers: map[string]config.ProviderConfig{
			"default": {
				BaseURL:       ts.URL,
				APIKey:        "test-key",
				Format:        "responses",
				CustomHeaders: map[string]string{"User-Agent": "claude-code/1.0.0"},
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-default": {Provider: "default", DefaultModel: "test-model"},
		},
	}
	provider := config.NewProvider(cfg, "")
	rtr := router.NewConfigRouter(provider)
	h := NewHandler(provider, nil, rtr, nil)
	engine := gin.New()
	h.RegisterRoutes(engine)

	w := doRequest(engine, "POST", "/v1/responses/compact", `{"model":"gpt-4o","input":"Summarize"}`)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "claude-code/1.0.0", gotUA, "custom User-Agent should reach upstream on native-compact path")
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

func TestDecodeCompactionInBody_NoCompaction(t *testing.T) {
	original := []byte(`{"model":"gpt-4","input":"hello"}`)
	result := decodeCompactionInBody(original)
	assert.Equal(t, string(original), string(result))
}

func TestDecodeCompactionInBody_WithFakeCompaction(t *testing.T) {
	// aisw_eyJzdW1tYXJ5IjoidGVzdCBzdW1tYXJ5IiwibW9kZWwiOiJncHQtNCIsInRzIjoxMjM0fQ==
	// decodes to {"summary":"test summary","model":"gpt-4","ts":1234}
	input := []byte(`{
		"model": "gpt-4",
		"input": [
			{"type": "compaction", "encrypted_content": "aisw_eyJzdW1tYXJ5IjoidGVzdCBzdW1tYXJ5IiwibW9kZWwiOiJncHQtNCIsInRzIjoxMjM0fQ=="},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "continue"}]}
		]
	}`)

	result := decodeCompactionInBody(input)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(result, &raw))

	// Compaction item removed from input, summary merged into instructions
	inputArr := raw["input"].([]any)
	require.Len(t, inputArr, 1)
	assert.Equal(t, "message", inputArr[0].(map[string]any)["type"])
	assert.Equal(t, "user", inputArr[0].(map[string]any)["role"])

	instructions, _ := raw["instructions"].(string)
	assert.Contains(t, instructions, "[Conversation Summary]")
	assert.Contains(t, instructions, "test summary")
}

func TestDecodeCompactionInBody_MergesWithExistingInstructions(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4",
		"instructions": "You are a helpful assistant.",
		"input": [
			{"type": "compaction", "encrypted_content": "aisw_eyJzdW1tYXJ5IjoidGVzdCBzdW1tYXJ5IiwibW9kZWwiOiJncHQtNCIsInRzIjoxMjM0fQ=="}
		]
	}`)

	result := decodeCompactionInBody(input)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(result, &raw))

	instructions, _ := raw["instructions"].(string)
	assert.Contains(t, instructions, "[Conversation Summary]")
	assert.Contains(t, instructions, "test summary")
	assert.Contains(t, instructions, "You are a helpful assistant.")
	// Summary should come first
	assert.True(t, strings.HasPrefix(instructions, "[Conversation Summary]"))
}

func TestDecodeCompactionInBody_RealCompaction_Unchanged(t *testing.T) {
	input := []byte(`{"model":"gpt-4","input":[{"type":"compaction","encrypted_content":"gAAAAABpM0Yj-real-blob"}]}`)
	result := decodeCompactionInBody(input)
	assert.Equal(t, string(input), string(result))
}

func TestDecodeCompactionInBody_NormalRequest_Unchanged(t *testing.T) {
	input := []byte(`{"model":"gpt-4","input":[{"type":"message","role":"user","content":"hello"}]}`)
	result := decodeCompactionInBody(input)
	assert.Equal(t, string(input), string(result))
}

func TestDecodeCompactionInBody_StringInput_Unchanged(t *testing.T) {
	input := []byte(`{"model":"gpt-4","input":"just a string"}`)
	result := decodeCompactionInBody(input)
	assert.Equal(t, string(input), string(result))
}

func TestHandleSimulatedCompactV2_Success(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":     "chatcmpl-summary",
			"object": "chat.completion",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "V2 summary."}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-5.4",
		"input": [
			{"role": "user", "content": "hello"},
			{"type": "compaction_trigger"}
		]
	}`)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "event: response.created")
	assert.Contains(t, body, "event: response.output_item.added")
	assert.Contains(t, body, "event: response.output_item.done")
	assert.Contains(t, body, "event: response.completed")
	assert.Contains(t, body, "data: [DONE]")
	// The compaction item type appears in added/done/completed events.
	assert.Contains(t, body, `"type":"compaction"`)
	// Model is resolved from the router result (test-model), not the client request.
	assert.Contains(t, body, `"model":"test-model"`)
	// Exactly one compaction output_item.done (terminal state for the item).
	assert.Equal(t, 1, strings.Count(body, "event: response.output_item.done"))
	// No message item — v2 emits only a compaction item.
	assert.NotContains(t, body, `"type":"message"`)
}

func TestHandleSimulatedCompactV2_UpstreamError(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"upstream down"}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-5.4",
		"input": [{"role":"user","content":"hi"},{"type":"compaction_trigger"}]
	}`)

	// Codex must get a parseable terminal SSE, not a bare HTTP error.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "event: response.failed")
	assert.Contains(t, w.Body.String(), "data: [DONE]")
}

func TestHandleResponses_NotCompactionPassesThrough(t *testing.T) {
	r, _ := setupRouter(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"object": "chat.completion",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "normal reply"}, "finish_reason": "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Ordinary request with NO compaction_trigger must NOT be hijacked by v2.
	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-5.4",
		"input": [{"role":"user","content":"hi"}]
	}`)
	assert.NotContains(t, w.Body.String(), "compaction")
}

// setupRouterWithTrace is like setupRouter but wires a real TraceRecorder writing
// to an in-memory buffer, so v2 compaction trace records can be asserted.
func setupRouterWithTrace(t *testing.T, upstreamFormat string, upstreamHandler http.HandlerFunc) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	ts := httptest.NewServer(upstreamHandler)
	t.Cleanup(ts.Close)

	provider := config.NewProvider(newTestConfig(ts.URL, upstreamFormat, "test-model"), "")
	r := router.NewConfigRouter(provider)
	buf := &bytes.Buffer{}
	trace := hook.NewTraceRecorder(buf, nil)
	h := NewHandler(provider, nil, r, trace)
	engine := gin.New()
	h.RegisterRoutes(engine)
	return engine, buf
}

// parseTraceRecords splits the JSONL trace buffer into a slice of records.
func parseTraceRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	recs := make([]map[string]any, 0)
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &rec))
		recs = append(recs, rec)
	}
	return recs
}

func TestHandleSimulatedCompactV2_TraceRecords(t *testing.T) {
	r, buf := setupRouterWithTrace(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-summary",
			"object": "chat.completion",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "V2 summary."}, "finish_reason": "stop"},
			},
		})
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-5.4",
		"input": [{"role":"user","content":"hi"},{"type":"compaction_trigger"}]
	}`)
	assert.Equal(t, http.StatusOK, w.Code)

	recs := parseTraceRecords(t, buf)
	require.Len(t, recs, 4, "expected request→upstream_req→upstream_resp→response")

	typeSeq := []string{
		recs[0]["type"].(string),
		recs[1]["type"].(string),
		recs[2]["type"].(string),
		recs[3]["type"].(string),
	}
	assert.Equal(t, []string{"request", "upstream_req", "upstream_resp", "response"}, typeSeq)

	// Request record carries the original compaction_trigger body + client protocol.
	assert.Equal(t, "responses", recs[0]["client_protocol"])
	assert.Contains(t, recs[0]["body"], "compaction_trigger")

	// Upstream request is the synthesized summarization call to the routed format.
	assert.Equal(t, "chat", recs[1]["upstream_protocol"])

	// Response record carries the synthesized compaction SSE (the compaction item
	// recurs across the added/done/completed events). The "exactly one output item"
	// invariant is covered by converter.BuildCompactionSSE's own tests.
	respBody := recs[3]["body"].(string)
	assert.Contains(t, respBody, "response.completed")
	assert.Contains(t, respBody, `"type":"compaction"`)
}

func TestHandleSimulatedCompactV2_TraceOnUpstreamError(t *testing.T) {
	r, buf := setupRouterWithTrace(t, "chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"overloaded"}`))
	})

	w := doRequest(r, "POST", "/v1/responses", `{
		"model": "gpt-5.4",
		"input": [{"role":"user","content":"hi"},{"type":"compaction_trigger"}]
	}`)

	// Codex must get a parseable terminal SSE; trace must still cover all 4 stages.
	assert.Equal(t, http.StatusOK, w.Code)
	recs := parseTraceRecords(t, buf)
	require.Len(t, recs, 4)

	assert.Equal(t, "upstream_resp", recs[2]["type"])
	assert.EqualValues(t, http.StatusServiceUnavailable, recs[2]["status"])

	assert.Equal(t, "response", recs[3]["type"])
	assert.Contains(t, recs[3]["body"], "response.failed")
}
