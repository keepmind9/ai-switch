package converter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HasCompactionTrigger reports whether a Responses API request body's input
// array contains a {"type":"compaction_trigger"} item. Codex remote-compaction
// v2 sends an ordinary POST /v1/responses whose input ends with such a marker
// instead of using the v1 /v1/responses/compact endpoint.
func HasCompactionTrigger(body []byte) bool {
	var raw struct {
		Input []map[string]any `json:"input"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	for _, item := range raw.Input {
		if t, ok := item["type"].(string); ok && t == "compaction_trigger" {
			return true
		}
	}
	return false
}

// BuildCompactionSSE builds a Codex v2 remote-compaction Responses SSE stream
// containing exactly one compaction output item and a response.completed event.
// model echoes the client's requested model. now is the unix timestamp used for
// ids/created_at (passed in so the function is deterministic and testable).
//
// The encrypted_content must be an ai-switch fake compaction payload so that,
// when Codex replays it in a later request, decodeCompactionInBody decodes it
// into the instructions field — identical to the v1 closed loop.
func BuildCompactionSSE(encryptedContent, model string, now int64) string {
	respID := fmt.Sprintf("resp_%d", now)
	itemID := fmt.Sprintf("cmp_%d", now)

	compactionItem := map[string]any{
		"id":                itemID,
		"type":              "compaction",
		"status":            "completed",
		"encrypted_content": encryptedContent,
	}

	var sb strings.Builder
	sb.WriteString(buildCompactionEventLine("response.created", map[string]any{
		"type":            "response.created",
		"sequence_number": 0,
		"response": map[string]any{
			"id":         respID,
			"object":     "response",
			"created_at": now,
			"status":     "in_progress",
			"model":      model,
		},
	}))
	sb.WriteString(buildCompactionEventLine("response.output_item.added", map[string]any{
		"type":            "response.output_item.added",
		"sequence_number": 1,
		"output_index":    0,
		"item": map[string]any{
			"id":     itemID,
			"type":   "compaction",
			"status": "in_progress",
		},
	}))
	sb.WriteString(buildCompactionEventLine("response.output_item.done", map[string]any{
		"type":            "response.output_item.done",
		"sequence_number": 2,
		"output_index":    0,
		"item":            compactionItem,
	}))
	sb.WriteString(buildCompactionEventLine("response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": 3,
		"response": map[string]any{
			"id":         respID,
			"object":     "response",
			"created_at": now,
			"status":     "completed",
			"model":      model,
			"output":     []any{compactionItem},
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
	}))
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

// buildCompactionEventLine renders one SSE event block: "event: <type>\ndata: <json>\n\n".
func buildCompactionEventLine(eventType string, data map[string]any) string {
	b, _ := json.Marshal(data)
	return "event: " + eventType + "\ndata: " + string(b) + "\n\n"
}
