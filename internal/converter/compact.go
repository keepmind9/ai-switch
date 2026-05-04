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
