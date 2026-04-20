package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripThinkTag(t *testing.T) {
	tests := []struct {
		name string
		text string
		tag  string
		want string
	}{
		{"empty tag", "hello <think x</think world", "", "hello <think x</think world"},
		{"single block", "hello <think\x3ehidden</think\x3e world", "think", "hello  world"},
		{"no tag present", "hello world", "think", "hello world"},
		{"unclosed", "hello <think\x3enever", "think", "hello "},
		{"empty block", "a<think\x3e</think\x3eb", "think", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StripThinkTag(tt.text, tt.tag))
		})
	}
}

func TestFilterChunk(t *testing.T) {
	tests := []struct {
		name   string
		tag    string
		chunks []string
		want   []string
	}{
		{"empty tag", "", []string{"a", "b"}, []string{"a", "b"}},
		{"no tags", "think", []string{"hello", " world"}, []string{"hello", " world"}},
		{"complete in chunk", "think", []string{"hi <think\x3ex</think\x3e bye"}, []string{"hi  bye"}},
		{"inside across chunks", "think", []string{"<think\x3ex", "y", "z</think\x3e hi"}, []string{"", "", " hi"}},
		{"unclosed at end", "think", []string{"hi <think\x3enever"}, []string{"hi "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ThinkTagState{}
			var got []string
			for _, c := range tt.chunks {
				got = append(got, s.FilterChunk(c, tt.tag))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
