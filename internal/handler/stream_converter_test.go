package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
)

// newSSEResponse builds a minimal streaming *http.Response whose Body is the
// given SSE bytes, so converter streaming functions can be driven directly
// without a real upstream connection.
func newSSEResponse(body string) *http.Response {
	r := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
	r.Header.Set("Content-Type", "text/event-stream")
	return r
}

// newCountingSSEContext returns a gin test context backed by a flush-counting
// recorder, so converter tests can assert flush granularity (OPT-4 part 2).
func newCountingSSEContext() (*gin.Context, *countingRecorder) {
	rec := &countingRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c, rec
}

// --- OPT-4 part 1: trace-gated accumulation (same contract as OPT-2) ---
// Converter streaming functions accumulate the upstream body only to feed the
// trace recorder (ctx.UpstreamRespBody). When tracing is off (the common case,
// llm_log_enabled=false), accumulation must be skipped to avoid per-line
// allocation/GC on the hot path.

func TestStreamChatToClient_NoAccumulationWhenTraceDisabled(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil) // trace disabled
	c, _ := newSSETestContext()

	resp := newSSEResponse("data: hello\n\ndata: [DONE]\n\n")
	convertFn := func(w converter.SSEWriter, data string) bool { return data == "[DONE]" }

	content := h.streamChatToClient(c, resp, convertFn, converter.FormatAnthropic)
	assert.Equal(t, "", content, "trace disabled: upstream accumulation must be skipped")
}

func TestStreamToChatSSE_NoAccumulationWhenTraceDisabled(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil) // trace disabled
	c, _ := newSSETestContext()

	resp := newSSEResponse("data: hello\n\ndata: [DONE]\n\n")
	convertFn := func(s any, line string) any {
		if converter.ParseSSEDataLine(line) == "[DONE]" {
			return "[DONE]"
		}
		return nil
	}

	content := h.streamToChatSSE(c, resp, convertFn, nil)
	assert.Equal(t, "", content, "trace disabled: upstream accumulation must be skipped")
}

// --- OPT-4 part 2: remove no-op flushes on the conversion path ---

// ginSSEWriter.WriteEvent already flushes per event. The outer per-line flush
// in streamChatToClient is therefore redundant; removing it leaves one flush
// per event (from WriteEvent) plus the single final flush after the loop.
// Client output stays byte-identical.
func TestStreamChatToClient_OmitsRedundantFlushes(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil)
	c, rec := newCountingSSEContext()

	// 2 chunks each emit one event; [DONE] ends the stream without writing.
	resp := newSSEResponse("data: c1\n\ndata: c2\n\ndata: [DONE]\n\n")
	convertFn := func(w converter.SSEWriter, data string) bool {
		if data == "[DONE]" {
			return true
		}
		w.WriteEvent("content_block_delta", map[string]any{"text": data})
		return false
	}

	content := h.streamChatToClient(c, resp, convertFn, converter.FormatAnthropic)

	assert.Equal(t, "", content, "trace disabled: no accumulation")
	assert.Contains(t, rec.Body.String(), `"c1"`)
	assert.Contains(t, rec.Body.String(), `"c2"`)
	// After OPT-4: 2 events (WriteEvent flushes) + 1 final flush = 3.
	// Before OPT-4: each event flushed twice (WriteEvent + redundant outer) + final = 5.
	assert.Equal(t, 3, rec.flushes, "outer per-line flush is redundant with WriteEvent")
}

// streamToChatSSE writes via c.Writer directly (no auto-flush). OPT-4 makes it
// flush only when the converter actually produced output this iteration, so
// input lines that yield no event (blank lines, or a state-only "skip" line)
// no longer trigger a no-op flush.
func TestStreamToChatSSE_OmitsNoOpFlushesWhenNoOutput(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil)
	c, rec := newCountingSSEContext()

	// c1 produces a chunk; "skip" yields nil (no output); [DONE] ends.
	resp := newSSEResponse("data: c1\n\ndata: skip\n\ndata: [DONE]\n\n")
	convertFn := func(s any, line string) any {
		switch d := converter.ParseSSEDataLine(line); d {
		case "[DONE]":
			return "[DONE]"
		case "skip", "":
			return nil
		default:
			return &types.ChatStreamResponse{ID: d}
		}
	}

	content := h.streamToChatSSE(c, resp, convertFn, nil)

	assert.Equal(t, "", content, "trace disabled: no accumulation")
	assert.Contains(t, rec.Body.String(), `"c1"`)
	assert.Contains(t, rec.Body.String(), `data: [DONE]`)
	// After OPT-4: c1 (output) + [DONE] (output) = 2 flushes; "skip" and blank
	// lines produce no output → no flush.
	// Before OPT-4: every input line flushed (c1, blank, skip, blank, [DONE]) = 5.
	assert.Equal(t, 2, rec.flushes, "input lines producing no output must not flush")
}
