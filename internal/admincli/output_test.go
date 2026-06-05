package admincli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    OutputFormat
		wantErr bool
	}{
		{"table", FormatTable, false},
		{"json", FormatJSON, false},
		{"", FormatTable, true},
		{"wide", "", true},
		{"csv", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseOutputFormat(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatter_PrintTable(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Format: FormatTable, Out: &buf}

	headers := []string{"KEY", "NAME", "FORMAT"}
	rows := [][]string{
		{"openai", "OpenAI", "chat"},
		{"deepseek", "DeepSeek", "chat"},
	}

	err := f.PrintTable(headers, rows)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "KEY")
	assert.Contains(t, output, "openai")
	assert.Contains(t, output, "DeepSeek")
	// Verify tab alignment (each column separated by tabs)
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	assert.Len(t, lines, 3) // header + 2 rows
}

func TestFormatter_PrintTable_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Format: FormatJSON, Out: &buf}

	headers := []string{"KEY", "NAME"}
	rows := [][]string{
		{"openai", "OpenAI"},
		{"deepseek", "DeepSeek"},
	}

	err := f.PrintTable(headers, rows)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"KEY": "openai"`)
	assert.Contains(t, output, `"NAME": "DeepSeek"`)
}

func TestFormatter_PrintTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Format: FormatTable, Out: &buf}

	err := f.PrintTable([]string{"KEY", "NAME"}, nil)
	require.NoError(t, err)

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	assert.Len(t, lines, 1) // header only
}

func TestFormatter_PrintJSON(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Format: FormatJSON, Out: &buf}

	data := map[string]string{"key": "openai", "name": "OpenAI"}
	err := f.PrintJSON(data)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), `"key": "openai"`)
}

func TestFormatter_PrintMessage(t *testing.T) {
	var buf bytes.Buffer
	f := &Formatter{Format: FormatTable, Out: &buf}

	err := f.PrintMessage("provider openai created")
	require.NoError(t, err)
	assert.Equal(t, "provider openai created\n", buf.String())
}

func TestRowsToMaps(t *testing.T) {
	headers := []string{"A", "B", "C"}
	rows := [][]string{
		{"1", "2", "3"},
		{"x", "y"},
	}

	result := rowsToMaps(headers, rows)
	assert.Len(t, result, 2)
	assert.Equal(t, "1", result[0]["A"])
	assert.Equal(t, "2", result[0]["B"])
	assert.Equal(t, "3", result[0]["C"])
	// Short row: missing column gets empty string
	assert.Equal(t, "", result[1]["C"])
}
