package router

import (
	"encoding/json"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

var (
	enc     *tiktoken.Tiktoken
	encOnce sync.Once
	encErr  error
)

func getEncoder() (*tiktoken.Tiktoken, error) {
	encOnce.Do(func() {
		enc, encErr = tiktoken.GetEncoding("cl100k_base")
	})
	return enc, encErr
}

// countTokens counts the approximate token count of an Anthropic request body.
// It extracts text from messages, system prompts, and tools.
// Returns 0 if encoding fails or body is invalid JSON.
func countTokens(body []byte) int {
	e, err := getEncoder()
	if err != nil {
		return 0
	}

	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		System json.RawMessage `json:"system"`
		Tools  []struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return 0
	}

	total := 0

	for _, msg := range req.Messages {
		total += countContentTokens(msg.Content, e)
	}

	total += countContentTokens(req.System, e)

	for _, tool := range req.Tools {
		if tool.Name != "" {
			total += len(e.Encode(tool.Name, nil, nil))
		}
		if tool.Description != "" {
			total += len(e.Encode(tool.Description, nil, nil))
		}
		if len(tool.InputSchema) > 0 {
			total += len(e.Encode(string(tool.InputSchema), nil, nil))
		}
	}

	return total
}

func countContentTokens(raw json.RawMessage, e *tiktoken.Tiktoken) int {
	if len(raw) == 0 {
		return 0
	}

	var s string
	if json.Unmarshal(raw, &s) == nil {
		return len(e.Encode(s, nil, nil))
	}

	var blocks []struct {
		Type   string          `json:"type"`
		Text   string          `json:"text"`
		Input  json.RawMessage `json:"input"`
		Result json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return 0
	}

	total := 0
	for _, b := range blocks {
		switch b.Type {
		case "text":
			total += len(e.Encode(b.Text, nil, nil))
		case "tool_use":
			if len(b.Input) > 0 {
				total += len(e.Encode(string(b.Input), nil, nil))
			}
		case "tool_result":
			total += countContentTokens(b.Result, e)
		}
	}
	return total
}
