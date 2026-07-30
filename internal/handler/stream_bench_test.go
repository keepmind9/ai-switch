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
	benchSinkBool   bool
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
	h := NewHandler(nil, nil, nil, nil, false)

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

// buildChatSSE constructs a representative OpenAI Chat-format SSE stream with
// the given number of content delta chunks plus a final [DONE]. The first
// chunk carries role:"assistant" so ConvertChatChunkToAnthropicSSE emits the
// message_start lifecycle event, mirroring real upstream traffic.
func buildChatSSE(numDeltas int) []byte {
	var b bytes.Buffer
	b.WriteString("data: {\"id\":\"chatcmpl-1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n")
	for i := 0; i < numDeltas; i++ {
		b.WriteString("data: {\"id\":\"chatcmpl-1\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"chunk\"},\"finish_reason\":null}]}\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.Bytes()
}

// BenchmarkStreamChatToClient measures the Chat->Anthropic conversion hot path
// with a REAL convertFn (ConvertChatChunkToAnthropicSSE), so the alloc share of
// the scanner/string boundary can be judged against the converter's own json
// parse + event-formatting cost. Compare allocs/op before vs after the
// zero-copy Bytes switch.
func BenchmarkStreamChatToClient(b *testing.B) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil, nil, nil, false) // trace disabled
	body := buildChatSSE(500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c, _ := newSSETestContext()
		resp := newSSEResponse(string(body))
		state := &converter.AnthropicStreamState{Model: "m"}
		convertFn := func(w converter.SSEWriter, data []byte) bool {
			return converter.ConvertChatChunkToAnthropicSSE(w, state, data)
		}
		b.StartTimer()
		h.streamChatToClient(c, resp, convertFn, converter.FormatAnthropic)
	}
}

// BenchmarkPerLineCost isolates the per-data-line allocation cost of the two
// json.Unmarshal sites on the conversion path: isSSEErrorData (error detection)
// and the converter itself (ConvertChatChunkToAnthropicSSE, steady-state content
// delta after message_start is initialized). The split shows whether the
// redundant error-detection parse is a real share of the 34205 allocs/op baseline.
func BenchmarkPerLineCost(b *testing.B) {
	delta := []byte(`{"id":"chatcmpl-1","model":"m","choices":[{"index":0,"delta":{"content":"chunk"},"finish_reason":null}]}`)
	roleChunk := `{"id":"chatcmpl-1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`

	b.Run("isSSEErrorData", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchSinkBool = isSSEErrorData(delta)
		}
	})

	b.Run("convertFn_delta", func(b *testing.B) {
		gin.SetMode(gin.TestMode)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			c, _ := newSSETestContext()
			w := &ginSSEWriter{c: c}
			state := &converter.AnthropicStreamState{Model: "m"}
			converter.ConvertChatChunkToAnthropicSSE(w, state, []byte(roleChunk)) // init message_start
			b.StartTimer()
			benchSinkBool = converter.ConvertChatChunkToAnthropicSSE(w, state, delta)
		}
	})
}
