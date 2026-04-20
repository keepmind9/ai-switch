package converter

import "strings"

// StripThinkTag removes all <{tag}>...</{tag}> blocks from text.
// Returns text unchanged if tag is empty.
func StripThinkTag(text, tag string) string {
	if tag == "" {
		return text
	}
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	for {
		i := strings.Index(text, open)
		if i < 0 {
			return text
		}
		j := strings.Index(text[i:], close)
		if j < 0 {
			return text[:i]
		}
		text = text[:i] + text[i+j+len(close):]
	}
}

// ThinkTagState tracks whether we are inside a think tag across stream chunks.
type ThinkTagState struct {
	inside bool
}

// FilterChunk strips think-tag content from a streaming chunk.
// Tags are assumed to be complete within a chunk boundary (common case).
func (s *ThinkTagState) FilterChunk(chunk, tag string) string {
	if tag == "" {
		return chunk
	}
	open := "<" + tag + ">"
	close := "</" + tag + ">"

	if s.inside {
		i := strings.Index(chunk, close)
		if i < 0 {
			return ""
		}
		s.inside = false
		chunk = chunk[i+len(close):]
	}

	for {
		i := strings.Index(chunk, open)
		if i < 0 {
			return chunk
		}
		j := strings.Index(chunk[i:], close)
		if j < 0 {
			s.inside = true
			return chunk[:i]
		}
		chunk = chunk[:i] + chunk[i+j+len(close):]
	}
}
