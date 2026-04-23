package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const mockResponseText = "MOCK_RESPONSE_OK"

// mockUpstream simulates an upstream LLM provider in chat, anthropic, or responses format.
type mockUpstream struct {
	format string
}

func newMockUpstream(format string) *mockUpstream {
	return &mockUpstream{format: format}
}

func (m *mockUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := decodeBody(r)
	isStreaming, _ := body["stream"].(bool)
	model, _ := body["model"].(string)

	switch m.format {
	case "anthropic":
		if isStreaming {
			m.writeAnthropicSSE(w, model)
		} else {
			m.writeAnthropicJSON(w, model)
		}
	case "responses":
		if isStreaming {
			m.writeResponsesSSE(w, model)
		} else {
			m.writeResponsesJSON(w, model)
		}
	default: // chat
		if isStreaming {
			m.writeChatSSE(w, model)
		} else {
			m.writeChatJSON(w, model)
		}
	}
}

// --- Chat format ---

func (m *mockUpstream) writeChatJSON(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": mockResponseText,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	})
}

func (m *mockUpstream) writeChatSSE(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	// First chunk with role
	writeSSE(w, flusher, "", map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil},
		},
	})

	// Content chunk
	writeSSE(w, flusher, "", map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]any{"content": mockResponseText}, "finish_reason": nil},
		},
	})

	// Usage chunk
	writeSSE(w, flusher, "", map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{},
		"usage": map[string]any{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	})

	// Stop chunk
	writeSSE(w, flusher, "", map[string]any{
		"id":      "chatcmpl-mock",
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"},
		},
	})

	// [DONE]
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// --- Anthropic format ---

func (m *mockUpstream) writeAnthropicJSON(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":    "msg_mock",
		"type":  "message",
		"role":  "assistant",
		"model": model,
		"content": []map[string]any{
			{"type": "text", "text": mockResponseText},
		},
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	})
}

func (m *mockUpstream) writeAnthropicSSE(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	// message_start
	writeSSE(w, flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      "msg_mock",
			"type":    "message",
			"role":    "assistant",
			"model":   model,
			"content": []any{},
			"usage":   map[string]any{"input_tokens": 10, "output_tokens": 0},
		},
	})

	// content_block_start
	writeSSE(w, flusher, "content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})

	// content_block_delta
	writeSSE(w, flusher, "content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "text_delta", "text": mockResponseText},
	})

	// content_block_stop
	writeSSE(w, flusher, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})

	// message_delta
	writeSSE(w, flusher, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": "end_turn",
		},
		"usage": map[string]any{"output_tokens": 5},
	})

	// message_stop
	writeSSE(w, flusher, "message_stop", map[string]any{
		"type": "message_stop",
	})
}

// --- Responses format ---

func (m *mockUpstream) writeResponsesJSON(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":      "resp_mock",
		"object":  "response",
		"created": time.Now().Unix(),
		"model":   model,
		"responses": []map[string]any{
			{
				"id":     "item_mock",
				"object": "response",
				"role":   "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": mockResponseText},
				},
				"status": "completed",
			},
		},
		"usage": map[string]any{
			"input_tokens":  10,
			"output_tokens": 5,
			"total_tokens":  15,
		},
	})
}

func (m *mockUpstream) writeResponsesSSE(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	now := time.Now().Unix()

	// response.created
	writeSSE(w, flusher, "response.created", map[string]any{
		"type":            "response.created",
		"sequence_number": 0,
		"response": map[string]any{
			"id": "resp_mock", "object": "response", "created_at": now,
			"model": model, "status": "in_progress",
		},
	})

	// response.output_item.added
	writeSSE(w, flusher, "response.output_item.added", map[string]any{
		"type": "response.output_item.added", "sequence_number": 1, "output_index": 0,
		"item": map[string]any{"id": "item_mock", "type": "message", "status": "in_progress", "role": "assistant"},
	})

	// response.content_part.added
	writeSSE(w, flusher, "response.content_part.added", map[string]any{
		"type": "response.content_part.added", "sequence_number": 2,
		"output_index": 0, "content_index": 0,
		"part": map[string]any{"type": "output_text", "text": ""},
	})

	// response.output_text.delta
	writeSSE(w, flusher, "response.output_text.delta", map[string]any{
		"type": "response.output_text.delta", "sequence_number": 3,
		"output_index": 0, "content_index": 0, "delta": mockResponseText,
	})

	// response.output_text.done
	writeSSE(w, flusher, "response.output_text.done", map[string]any{
		"type": "response.output_text.done", "sequence_number": 4,
		"output_index": 0, "content_index": 0, "text": mockResponseText,
	})

	// response.content_part.done
	writeSSE(w, flusher, "response.content_part.done", map[string]any{
		"type": "response.content_part.done", "sequence_number": 5,
		"output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": mockResponseText},
	})

	// response.output_item.done
	writeSSE(w, flusher, "response.output_item.done", map[string]any{
		"type": "response.output_item.done", "sequence_number": 6, "output_index": 0,
		"item": map[string]any{"id": "item_mock", "type": "message", "status": "completed", "role": "assistant"},
	})

	// response.completed
	writeSSE(w, flusher, "response.completed", map[string]any{
		"type": "response.completed", "sequence_number": 7,
		"response": map[string]any{
			"id": "resp_mock", "object": "response", "created_at": now,
			"model": model, "status": "completed",
		},
	})
}

// --- Helpers ---

func writeSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, data any) {
	jsonData, _ := json.Marshal(data)
	if eventType != "" {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(jsonData))
	} else {
		fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	}
	flusher.Flush()
}

func decodeBody(r *http.Request) map[string]any {
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	return body
}

// isSSE checks if the response is SSE by Content-Type header.
func isSSE(resp *http.Response) bool {
	return strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
}
