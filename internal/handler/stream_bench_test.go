package handler

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
)

// benchSink* prevent the compiler from eliminating the per-line result as dead
// code in the micro-benchmark below.
var (
	benchSinkString string
	benchSinkBytes  []byte
)

// buildBenchmarkSSE constructs a representative Anthropic SSE stream with the
// given number of content_block_delta events plus the surrounding lifecycle
// events (message_start, content_block_start/stop, message_delta with usage,
// message_stop). Delta lines intentionally omit "usage" so the sniff pre-filter
// is exercised on the hot path.
func buildBenchmarkSSE(numDeltas int) []byte {
	var b bytes.Buffer
	w := func(s string) { b.WriteString(s) }
	w("event: message_start\n")
	w("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100}}}\n\n")
	w("event: content_block_start\n")
	w("data: {\"type\":\"content_block_start\",\"index\":0}\n\n")
	for i := 0; i < numDeltas; i++ {
		w("event: content_block_delta\n")
		w("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"chunk\"}}\n\n")
	}
	w("event: content_block_stop\n")
	w("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	w("event: message_delta\n")
	w("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":200}}\n\n")
	w("event: message_stop\n")
	w("data: {\"type\":\"message_stop\"}\n\n")
	return b.Bytes()
}

// BenchmarkStreamBodyAsSSE measures the same-protocol passthrough hot path at
// three stream sizes. It is the baseline for streaming optimizations: compare
// allocs/op and ns/op before and after changes. Per-iteration setup (context +
// recorder + request) is excluded via StopTimer so the numbers reflect
// streamBodyAsSSE itself.
func BenchmarkStreamBodyAsSSE(b *testing.B) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil, nil, nil)

	for _, numDeltas := range []int{50, 500, 2000} {
		body := buildBenchmarkSSE(numDeltas)
		b.Run(fmt.Sprintf("deltas=%d", numDeltas), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				b.StartTimer()
				h.streamBodyAsSSE(c, bytes.NewReader(body), converter.FormatAnthropic)
			}
		})
	}
}

// BenchmarkScannerTextVsBytes isolates the per-line cost of scanner.Text() vs
// scanner.Bytes() on the same SSE body. Text() copies each line into a new
// string (heap alloc per line); Bytes() returns a slice into the scanner's
// buffer (zero alloc, valid until the next Scan). The gap between the two is the
// upper-bound savings available to streamBodyAsSSE if it switches to Bytes().
func BenchmarkScannerTextVsBytes(b *testing.B) {
	body := buildBenchmarkSSE(500)
	b.Run("Text", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s := bufio.NewScanner(bytes.NewReader(body))
			s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for s.Scan() {
				benchSinkString = s.Text()
			}
		}
	})
	b.Run("Bytes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s := bufio.NewScanner(bytes.NewReader(body))
			s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for s.Scan() {
				benchSinkBytes = s.Bytes()
			}
		}
	})
}
