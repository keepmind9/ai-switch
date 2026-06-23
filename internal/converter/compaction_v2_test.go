package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasCompactionTrigger(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "trigger at end of input",
			body: `{"model":"gpt-5.4","input":[{"role":"user","content":"hi"},{"type":"compaction_trigger"}]}`,
			want: true,
		},
		{
			name: "trigger in middle of input",
			body: `{"input":[{"type":"compaction_trigger"},{"role":"user","content":"hi"}]}`,
			want: true,
		},
		{
			name: "no trigger",
			body: `{"input":[{"role":"user","content":"hi"}]}`,
			want: false,
		},
		{
			name: "input missing",
			body: `{"model":"gpt-5.4"}`,
			want: false,
		},
		{
			name: "input not array",
			body: `{"input":"not-an-array"}`,
			want: false,
		},
		{
			name: "invalid json",
			body: `{not json`,
			want: false,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
		{
			name: "only trigger item",
			body: `{"input":[{"type":"compaction_trigger"}]}`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasCompactionTrigger([]byte(tt.body)))
		})
	}
}

func TestBuildCompactionSSE(t *testing.T) {
	encrypted := "aisw_eyJzdW1tYXJ5IjoidGVzdCJ9"
	model := "gpt-5.4"
	sse := BuildCompactionSSE(encrypted, model, 1700000000)

	// Event order: created -> output_item.added -> output_item.done -> completed -> [DONE]
	lines := strings.Split(strings.TrimSpace(sse), "\n")
	var eventTypes []string
	for _, l := range lines {
		if strings.HasPrefix(l, "event: ") {
			eventTypes = append(eventTypes, strings.TrimPrefix(l, "event: "))
		}
	}
	assert.Equal(t, []string{
		"response.created",
		"response.output_item.added",
		"response.output_item.done",
		"response.completed",
	}, eventTypes)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(sse), "data: [DONE]"))

	// Exactly one compaction item in the completed response.
	completed := extractCompletedData(t, sse)
	resp := completed["response"].(map[string]any)
	output := resp["output"].([]any)
	require.Len(t, output, 1)
	item := output[0].(map[string]any)
	assert.Equal(t, "compaction", item["type"])
	assert.Equal(t, encrypted, item["encrypted_content"])

	// Model echoed into the completed response.
	assert.Equal(t, model, resp["model"])

	// IDs share the timestamp suffix.
	assert.Contains(t, item["id"].(string), "cmp_")
	assert.Contains(t, resp["id"].(string), "resp_")
}

func TestBuildCompactionSSE_RoundTripsEncodedPayload(t *testing.T) {
	enc, err := EncodeCompactionPayload(&types.CompactionPayload{Summary: "the summary", Model: "gpt-5.4", TS: 1700000000})
	require.NoError(t, err)
	sse := BuildCompactionSSE(enc, "gpt-5.4", 1700000000)

	completed := extractCompletedData(t, sse)
	item := completed["response"].(map[string]any)["output"].([]any)[0].(map[string]any)
	payload, err := DecodeCompactionPayload(item["encrypted_content"].(string))
	require.NoError(t, err)
	assert.Equal(t, "the summary", payload.Summary)
}

// extractCompletedData parses the response.completed "data:" line of an SSE blob.
func extractCompletedData(t *testing.T, sse string) map[string]any {
	t.Helper()
	for _, l := range strings.Split(sse, "\n") {
		if !strings.HasPrefix(l, "data: ") {
			continue
		}
		data := strings.TrimPrefix(l, "data: ")
		if data == "[DONE]" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(data), &m))
		if m["type"] == "response.completed" {
			return m
		}
	}
	t.Fatalf("no response.completed event in SSE:\n%s", sse)
	return nil
}
