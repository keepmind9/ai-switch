package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasCompactionTrigger(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "trigger at end of input",
			body: `{"model":"gpt-5.4","input":[{"role":"user","content":"hi"},{"type":"compaction_trigger"}]}`,
			want: true,
		},
		{
			name: "trigger in middle of input",
			body: `{"input":[{"type":"compaction_trigger"},{"role":"user","content":"hi"}]}`,
			want: true,
		},
		{
			name: "no trigger",
			body: `{"input":[{"role":"user","content":"hi"}]}`,
			want: false,
		},
		{
			name: "input missing",
			body: `{"model":"gpt-5.4"}`,
			want: false,
		},
		{
			name: "input not array",
			body: `{"input":"not-an-array"}`,
			want: false,
		},
		{
			name: "invalid json",
			body: `{not json`,
			want: false,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
		{
			name: "only trigger item",
			body: `{"input":[{"type":"compaction_trigger"}]}`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasCompactionTrigger([]byte(tt.body)))
		})
	}
}
