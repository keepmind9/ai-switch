package handler

import (
	"bytes"
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
	h := NewHandler(nil, nil, nil, nil)

	rec := &countingRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	h.streamBodyAsSSE(c, bytes.NewReader([]byte(anthropicSSEFixture)), converter.FormatAnthropic)

	assert.Equal(t, anthropicSSEFixture, rec.Body.String(), "client output must stay byte-identical")
	assert.Equal(t, 4, rec.flushes, "should flush once per SSE event, not per line")
}
