package converter

import (
	stdjson "encoding/json"
	"testing"

	sonic "github.com/bytedance/sonic"
	goccy "github.com/goccy/go-json"
	jsoniter "github.com/json-iterator/go"

	"github.com/keepmind9/ai-switch/internal/types"
)

// jsonLibBench measures raw Marshal/Unmarshal throughput of candidate JSON
// libraries against encoding/json, using payload shapes that mirror the
// streaming hot path: per-line SSE chunk parse into a typed struct, the
// map[string]any sniff used by isSSEErrorData / usage extraction, and struct
// marshal for SSE event output.
//
// All four libraries are already in go.mod (pulled in transitively by gin), so
// this benchmark introduces no new dependency. jsoniter is tested in its
// ConfigCompatibleWithStandardLibrary mode (behavior-equivalent to stdlib),
// which is the safe-drop-in variant; jsoniter's ConfigFastest is faster still.

var jsoniterCompat = jsoniter.ConfigCompatibleWithStandardLibrary

// Representative payload sizes covering the streaming hot path.

// chatDeltaSmall: a typical content delta chunk (~130B).
var chatDeltaSmall = []byte(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"content":"chunk"},"finish_reason":null}]}`)

// usageChunk: a final chunk carrying token usage (~210B).
var usageChunk = []byte(`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":50,"total_tokens":1050,"prompt_tokens_details":{"cached_tokens":800}}}`)

// largeCompleted: a Responses response.completed with a multi-item output
// array (~960B), exercising deeper nesting and larger allocation footprints.
var largeCompleted = []byte(`{"type":"response.completed","response":{"id":"resp_abc","object":"response","created_at":1234567890,"status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Here is a longer assistant response that spans many tokens to exercise deeper JSON nesting and larger allocation footprints during unmarshaling across the candidate libraries being benchmarked today.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"SF\"}"}],"usage":{"input_tokens":1500,"output_tokens":320,"total_tokens":1820,"input_tokens_details":{"cached_tokens":1200},"output_tokens_details":{"reasoning_tokens":80}}}}`)

// marshalStruct is a populated ChatStreamResponse used as the Marshal input.
var marshalStruct = &types.ChatStreamResponse{
	ID:      "chatcmpl-1",
	Object:  "chat.completion.chunk",
	Created: 1234567890,
	Model:   "test-model",
	Choices: []types.StreamChoice{
		{Index: 0, Delta: types.ChatMessage{Role: "assistant", Content: strPtr("Here is a longer assistant response that spans many tokens to exercise deeper JSON nesting and larger allocation footprints.")}, FinishReason: ""},
	},
}

// lib names the candidates in a fixed order.
var libs = []string{"stdlib", "sonic", "jsoniter", "goccy"}

func unmarshalMap(lib string, data []byte) {
	switch lib {
	case "stdlib":
		var m map[string]any
		_ = stdjson.Unmarshal(data, &m)
	case "sonic":
		var m map[string]any
		_ = sonic.Unmarshal(data, &m)
	case "jsoniter":
		var m map[string]any
		_ = jsoniterCompat.Unmarshal(data, &m)
	case "goccy":
		var m map[string]any
		_ = goccy.Unmarshal(data, &m)
	}
}

func unmarshalStruct(lib string, data []byte) {
	switch lib {
	case "stdlib":
		var c types.ChatStreamResponse
		_ = stdjson.Unmarshal(data, &c)
	case "sonic":
		var c types.ChatStreamResponse
		_ = sonic.Unmarshal(data, &c)
	case "jsoniter":
		var c types.ChatStreamResponse
		_ = jsoniterCompat.Unmarshal(data, &c)
	case "goccy":
		var c types.ChatStreamResponse
		_ = goccy.Unmarshal(data, &c)
	}
}

func marshalStructFn(lib string, v *types.ChatStreamResponse) {
	switch lib {
	case "stdlib":
		_, _ = stdjson.Marshal(v)
	case "sonic":
		_, _ = sonic.Marshal(v)
	case "jsoniter":
		_, _ = jsoniterCompat.Marshal(v)
	case "goccy":
		_, _ = goccy.Marshal(v)
	}
}

// BenchmarkJSONLib_UnmarshalMap measures decoding into map[string]any, the
// shape used by per-line error/usage sniffing.
func BenchmarkJSONLib_UnmarshalMap(b *testing.B) {
	cases := []struct {
		name string
		data []byte
	}{
		{"delta", chatDeltaSmall},
		{"usage", usageChunk},
		{"large", largeCompleted},
	}
	for _, c := range cases {
		for _, lib := range libs {
			b.Run(c.name+"/"+lib, func(b *testing.B) {
				unmarshalMap(lib, c.data) // warmup (sonic JIT / codegen cache)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					unmarshalMap(lib, c.data)
				}
			})
		}
	}
}

// BenchmarkJSONLib_UnmarshalStruct measures decoding into a typed struct, the
// shape used by converter chunk parsing.
func BenchmarkJSONLib_UnmarshalStruct(b *testing.B) {
	cases := []struct {
		name string
		data []byte
	}{
		{"delta", chatDeltaSmall},
		{"usage", usageChunk},
		{"large", largeCompleted},
	}
	for _, c := range cases {
		for _, lib := range libs {
			b.Run(c.name+"/"+lib, func(b *testing.B) {
				unmarshalStruct(lib, c.data)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					unmarshalStruct(lib, c.data)
				}
			})
		}
	}
}

// BenchmarkJSONLib_MarshalStruct measures encoding a typed struct, the shape
// used by SSE event output formatting.
func BenchmarkJSONLib_MarshalStruct(b *testing.B) {
	for _, lib := range libs {
		b.Run(lib, func(b *testing.B) {
			marshalStructFn(lib, marshalStruct)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				marshalStructFn(lib, marshalStruct)
			}
		})
	}
}
