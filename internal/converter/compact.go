package converter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keepmind9/ai-switch/internal/types"
)

const compactionPrefix = "aisw_"

// IsFakeCompaction returns true if the encrypted_content was produced by ai-switch.
func IsFakeCompaction(encryptedContent string) bool {
	return strings.HasPrefix(encryptedContent, compactionPrefix)
}

// EncodeCompactionPayload serializes a CompactionPayload into a self-contained encrypted_content string.
func EncodeCompactionPayload(payload *types.CompactionPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return compactionPrefix + base64.StdEncoding.EncodeToString(data), nil
}

// DecodeCompactionPayload decodes an ai-switch fake encrypted_content back to the payload.
func DecodeCompactionPayload(encoded string) (*types.CompactionPayload, error) {
	if !IsFakeCompaction(encoded) {
		return nil, fmt.Errorf("not a fake compaction payload")
	}
	data, err := base64.StdEncoding.DecodeString(encoded[len(compactionPrefix):])
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}
	var payload types.CompactionPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("json decode failed: %w", err)
	}
	return &payload, nil
}

// ExtractConversationText formats Responses API input items into a readable conversation string.
// Compaction items are skipped.
func ExtractConversationText(input any) string {
	if input == nil {
		return ""
	}
	switch v := input.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			switch val := item.(type) {
			case string:
				if val != "" {
					parts = append(parts, "[user]: "+val)
				}
			case map[string]any:
				itemType, _ := val["type"].(string)
				if itemType == "compaction" {
					continue
				}
				role, _ := val["role"].(string)
				text := extractInputTextMessage(val)
				if text != "" && role != "" {
					parts = append(parts, fmt.Sprintf("[%s]: %s", role, text))
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

const summarizationSystemPrompt = `You are a conversation compactor. Summarize the conversation below into a compact but complete summary. Preserve:
- Key decisions and rationale
- Code changes made (file paths, what changed)
- Current task status and any pending items
- Important context needed to continue the conversation

Be factual and specific. Omit pleasantries and repetition.`

// BuildSummarizationRequest creates a Chat API request for LLM-based summarization.
func BuildSummarizationRequest(conversation, model string) *types.ChatRequest {
	return &types.ChatRequest{
		Model:     model,
		Stream:    false,
		MaxTokens: 1024,
		Messages: []types.ChatMessage{
			{Role: "system", Content: strPtr(summarizationSystemPrompt)},
			{Role: "user", Content: strPtr(conversation)},
		},
	}
}
