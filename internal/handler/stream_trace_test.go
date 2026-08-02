package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/stretchr/testify/assert"
)

// anthropicSSEFixture is a small but complete Anthropic SSE stream covering
// message_start, a delta, message_delta (usage) and message_stop. It is the
// golden input for byte-diff equivalence checks.
const anthropicSSEFixture = "event: message_start\n" +
	"data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n" +
	"event: content_block_delta\n" +
	"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\n" +
	"event: message_delta\n" +
	"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n" +
	"event: message_stop\n" +
	"data: {\"type\":\"message_stop\"}\n\n"

func newSSETestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, w
}

// TestStreamBodyAsSSE_NoAccumulationWhenTraceDisabled: when tracing is off (the
// common case, llm_log_enabled=false), the full response must NOT be accumulated
// into memory — streamBodyAsSSE returns "" — while the client still receives a
// byte-identical stream and usage is still sniffed.
func TestStreamBodyAsSSE_NoAccumulationWhenTraceDisabled(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil) // trace == nil => noop recorder => disabled

	c, w := newSSETestContext()
	content, inTokens, outTokens, _, _ := h.streamBodyAsSSE(c, bytes.NewReader([]byte(anthropicSSEFixture)), converter.FormatAnthropic)

	assert.Equal(t, "", content, "with trace disabled, full-body accumulation must be skipped")
	assert.Equal(t, anthropicSSEFixture, w.Body.String(), "client output must be byte-identical regardless of trace")
	// usage sniffing still works without accumulation
	assert.Equal(t, int64(10), inTokens, "input tokens must still be sniffed")
	assert.Equal(t, int64(5), outTokens, "output tokens must still be sniffed")
}

// TestStreamBodyAsSSE_AccumulatesWhenTraceEnabled: when tracing is on, the full
// body is accumulated and returned (regression guard for the trace feature).
func TestStreamBodyAsSSE_AccumulatesWhenTraceEnabled(t *testing.T) {
	var traceBuf bytes.Buffer
	trace := hook.NewTraceRecorder(&traceBuf, nil)
	h := NewHandler(nil, nil, nil, trace, nil)

	c, w := newSSETestContext()
	content, _, _, _, _ := h.streamBodyAsSSE(c, bytes.NewReader([]byte(anthropicSSEFixture)), converter.FormatAnthropic)

	assert.Equal(t, anthropicSSEFixture, content, "with trace enabled, full body is accumulated for the trace record")
	assert.Equal(t, anthropicSSEFixture, w.Body.String())
}
