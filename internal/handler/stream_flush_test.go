package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/stretchr/testify/assert"
)

// countingRecorder wraps httptest.ResponseRecorder to count Flush() calls,
// so tests can assert flush granularity (per-event vs per-line) without a real
// socket. This is the performance gate for OPT-1; byte-identity is the
// functional gate (asserted separately and here as well).
type countingRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (c *countingRecorder) Flush() { c.flushes++ }

// TestStreamBodyAsSSE_FlushesOncePerEvent: the fixture has exactly 4 SSE events.
// OPT-1 requires flushing once per event (~4 flushes), not once per line (~12).
// Output must remain byte-identical.
func TestStreamBodyAsSSE_FlushesOncePerEvent(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, false)

	rec := &countingRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h.streamBodyAsSSE(c, bytes.NewReader([]byte(anthropicSSEFixture)), converter.FormatAnthropic)

	assert.Equal(t, anthropicSSEFixture, rec.Body.String(), "client output must stay byte-identical")
	assert.Equal(t, 4, rec.flushes, "should flush once per SSE event, not per line")
}

// TestGinSSEWriter_WriteEvent verifies the inline byte-splice path produces
// output byte-identical to converter.FormatSSEEvent for representative event
// types, including the empty-eventType case used by the Chat error path
// (error_helpers.go). Also pins the exact SSE wire format and per-call flush.
func TestGinSSEWriter_WriteEvent(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      any
	}{
		{
			"content_block_delta",
			"content_block_delta",
			map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "hi"}},
		},
		{
			"error_event",
			"error",
			map[string]any{"type": "error", "error": map[string]any{"type": "api_error", "message": "boom"}},
		},
		{
			"empty_event_type_chat_error",
			"",
			map[string]any{"error": map[string]any{"message": "boom", "type": "server_error"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &countingRecorder{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			w := &ginSSEWriter{c: c}

			w.WriteEvent(tt.eventType, tt.data)

			jsonData, _ := json.Marshal(tt.data)
			// The inline path must stay byte-identical to the converter's public helper.
			assert.Equal(t, converter.FormatSSEEvent(tt.eventType, jsonData), rec.Body.String())
			// And match the exact SSE wire format explicitly.
			assert.Equal(t, "event: "+tt.eventType+"\ndata: "+string(jsonData)+"\n\n", rec.Body.String())
			assert.Equal(t, 1, rec.flushes, "WriteEvent must flush exactly once per call")
		})
	}
}
