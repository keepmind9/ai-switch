package types

// ChatErrorResponse models the OpenAI/Chat API error format.
// Used by Chat and Responses APIs.
type ChatErrorResponse struct {
	Error *ChatErrorDetail `json:"error"`
}

type ChatErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

// AnthropicErrorResponse models the Anthropic Messages API error format.
type AnthropicErrorResponse struct {
	Type  string                `json:"type"`
	Error *AnthropicErrorDetail `json:"error"`
}

type AnthropicErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
