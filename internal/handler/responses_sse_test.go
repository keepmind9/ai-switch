package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestConvertResponsesJSONToSSE_BasicText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	respJSON := map[string]any{
		"id": "resp_test123", "object": "response", "created_at": 1234567890,
		"model": "gpt-5.4", "status": "completed",
		"output": []any{
			map[string]any{
				"id": "item_1", "type": "message", "status": "completed", "role": "assistant",
				"content": []any{
					map[string]any{"type": "output_text", "text": "Hello, world!"},
				},
			},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
	body, _ := json.Marshal(respJSON)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &Handler{}
	h.convertResponsesJSONToSSE(c, body)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	output := w.Body.String()
	assert.Contains(t, output, "event: response.created")
	assert.Contains(t, output, "event: response.output_item.added")
	assert.Contains(t, output, "event: response.content_part.added")
	assert.Contains(t, output, "event: response.output_text.delta")
	assert.Contains(t, output, "Hello, world!")
	assert.Contains(t, output, "event: response.output_text.done")
	assert.Contains(t, output, "event: response.content_part.done")
	assert.Contains(t, output, "event: response.output_item.done")
	assert.Contains(t, output, "event: response.completed")

	// Verify response.completed contains status=completed
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, `"type":"response.completed"`) {
			assert.Contains(t, line, `"status":"completed"`)
			break
		}
	}
}

func TestConvertResponsesJSONToSSE_EmptyOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	respJSON := map[string]any{
		"id": "resp_empty", "object": "response", "created_at": 1234567890,
		"model": "test-model", "status": "completed", "output": []any{},
	}
	body, _ := json.Marshal(respJSON)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &Handler{}
	h.convertResponsesJSONToSSE(c, body)

	output := w.Body.String()
	assert.Contains(t, output, "event: response.created")
	assert.Contains(t, output, "event: response.completed")
	// No output item events since output is empty
	assert.NotContains(t, output, "response.output_item.added")
}

func TestConvertResponsesJSONToSSE_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &Handler{}
	h.convertResponsesJSONToSSE(c, []byte("not json"))

	// Should still write SSE headers but no events
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
}

func TestConvertResponsesJSONToSSE_FunctionCall(t *testing.T) {
	gin.SetMode(gin.TestMode)

	respJSON := map[string]any{
		"id": "resp_fc", "object": "response", "created_at": 1234567890,
		"model": "test-model", "status": "completed",
		"output": []any{
			map[string]any{
				"id": "item_fc", "type": "function_call", "status": "completed",
				"name": "get_weather", "call_id": "call_123", "arguments": `{"city":"NYC"}`,
			},
		},
	}
	body, _ := json.Marshal(respJSON)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h := &Handler{}
	h.convertResponsesJSONToSSE(c, body)

	output := w.Body.String()
	assert.Contains(t, output, "event: response.output_item.added")
	assert.Contains(t, output, "event: response.output_item.done")
	assert.Contains(t, output, "event: response.completed")
	// function_call items don't emit content_part events
	assert.NotContains(t, output, "response.content_part.added")
}
