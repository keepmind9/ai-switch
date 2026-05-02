package converter

import "strings"

// GeminiStreamState tracks state when converting between Gemini SSE and other SSE formats.
type GeminiStreamState struct {
	Model        string
	InputTokens  int
	OutputTokens int
	Started      bool
	Done         bool
	// Tool call accumulation for Gemini → Chat: index → accumulated args
	ToolCallArgs map[int]*strings.Builder
	ToolCallIDs  map[int]string
	ToolCallSeq  int
	AccText      string
}

func newGeminiStreamState() *GeminiStreamState {
	return &GeminiStreamState{
		ToolCallArgs: make(map[int]*strings.Builder),
		ToolCallIDs:  make(map[int]string),
	}
}
