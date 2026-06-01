package types

// OpenAI Responses API Types

type ResponsesRequest struct {
	Model              string          `json:"model"`
	Input              any             `json:"input,omitempty"`
	Instructions       any             `json:"instructions,omitempty"`
	MaxTokens          int             `json:"max_tokens,omitempty"`
	Store              bool            `json:"store,omitempty"`
	Metadata           map[string]any  `json:"metadata,omitempty"`
	TopP               float64         `json:"top_p,omitempty"`
	Temperature        float64         `json:"temperature,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Stream             bool            `json:"stream,omitempty"`
	Tools              []ResponsesTool `json:"tools,omitempty"`
	ToolChoice         any             `json:"tool_choice,omitempty"`
	Truncation         string          `json:"truncation,omitempty"`
	Reasoning          any             `json:"reasoning,omitempty"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls,omitempty"`
}

type ResponsesTool struct {
	Type        string          `json:"type,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  map[string]any  `json:"parameters,omitempty"`
	Tools       []ResponsesTool `json:"tools,omitempty"` // namespace sub-tools (Codex MCP)
}

type ResponsesResponse struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	CreatedAt         int64              `json:"created_at"`
	Model             string             `json:"model"`
	Output            []ResponseItem     `json:"output,omitempty"`
	Usage             *Usage             `json:"usage,omitempty"`
	Status            string             `json:"status,omitempty"`
	Error             *ResponseError     `json:"error,omitempty"`
	IncompleteDetails *IncompleteDetails `json:"incomplete_details,omitempty"`
}

type ResponseError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type IncompleteDetails struct {
	Reason string `json:"reason"`
}

type ResponseItem struct {
	ID        string         `json:"id"`
	Type      string         `json:"type,omitempty"`
	Object    string         `json:"object"`
	Created   int64          `json:"created"`
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content,omitempty"`
	Status    string         `json:"status"`
	CallID    string         `json:"call_id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments string         `json:"arguments,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Usage struct {
	InputTokens         int                  `json:"input_tokens"`
	OutputTokens        int                  `json:"output_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	CacheCreationTokens int                  `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int                  `json:"cache_read_tokens,omitempty"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
}

type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// CompactionPayload is the self-contained data encoded in fake encrypted_content.
type CompactionPayload struct {
	Summary string `json:"summary"`
	Model   string `json:"model"`
	TS      int64  `json:"ts"`
}

// Chat Completions API Types

type ChatRequest struct {
	Model         string         `json:"model"`
	Messages      []ChatMessage  `json:"messages"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   float64        `json:"temperature,omitempty"`
	TopP          float64        `json:"top_p,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	N             int            `json:"n,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
	ToolChoice    any            `json:"tool_choice,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type ChatMessage struct {
	Role             string     `json:"role,omitempty"`
	Content          *string    `json:"content,omitempty"`
	ReasoningContent *string    `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   ChatUsage    `json:"usage"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatUsage struct {
	PromptTokens         int                  `json:"prompt_tokens"`
	CompletionTokens     int                  `json:"completion_tokens"`
	TotalTokens          int                  `json:"total_tokens"`
	PromptTokensDetails  *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	PromptCacheHitTokens int                  `json:"prompt_cache_hit_tokens,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// Stream event types

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type ChatStreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *ChatUsage     `json:"usage,omitempty"`
}

// Tool types for Chat Completions API

type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ToolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
