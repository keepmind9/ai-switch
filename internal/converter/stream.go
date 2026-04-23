package converter

// SSEWriter abstracts SSE event output, decoupling converters from HTTP frameworks.
type SSEWriter interface {
	WriteEvent(eventType string, data any)
}

// ResponsesStreamState tracks state across SSE chunks for Responses API conversion.
type ResponsesStreamState struct {
	ResponseID   string
	Created      int64
	OutputIndex  int
	ContentIndex int
	ItemID       string
	CreatedSent  bool
	AccText      string
	SeqNum       int
	Model        string
	InputTokens  int
	OutputTokens int
	ThinkTag     string
	TagState     ThinkTagState
}

func (s *ResponsesStreamState) nextSeq() int {
	s.SeqNum++
	return s.SeqNum
}

// AnthropicStreamState tracks state across SSE chunks for Anthropic conversion.
type AnthropicStreamState struct {
	MessageID    string
	Model        string
	BlockIndex   int
	ContentSent  bool
	AccText      string
	InputTokens  int
	OutputTokens int
	ThinkTag     string
	TagState     ThinkTagState
	FinishReason string
	DeltaSent    bool
}

func (s *AnthropicStreamState) nextBlockIndex() int {
	idx := s.BlockIndex
	s.BlockIndex++
	return idx
}
