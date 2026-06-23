package converter

import "encoding/json"

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
