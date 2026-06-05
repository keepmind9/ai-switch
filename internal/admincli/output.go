package admincli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// OutputFormat controls how data is rendered.
type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatJSON  OutputFormat = "json"
)

// ParseOutputFormat validates and returns an OutputFormat.
func ParseOutputFormat(s string) (OutputFormat, error) {
	switch OutputFormat(s) {
	case FormatTable, FormatJSON:
		return OutputFormat(s), nil
	default:
		return "", fmt.Errorf("unsupported output format %q (use table or json)", s)
	}
}

// Formatter renders data in the configured output format.
type Formatter struct {
	Format OutputFormat
	Out    io.Writer
}

// PrintTable prints a header + rows as a column-aligned table.
func (f *Formatter) PrintTable(headers []string, rows [][]string) error {
	if f.Format == FormatJSON {
		return f.PrintJSON(rowsToMaps(headers, rows))
	}

	w := tabwriter.NewWriter(f.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

// PrintJSON pretty-prints v as indented JSON.
func (f *Formatter) PrintJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f.Out, string(b))
	return err
}

// PrintMessage prints a plain text message.
func (f *Formatter) PrintMessage(msg string) error {
	_, err := fmt.Fprintln(f.Out, msg)
	return err
}

// rowsToMaps converts rows + headers into a slice of maps for JSON output.
func rowsToMaps(headers []string, rows [][]string) []map[string]string {
	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		m := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		result = append(result, m)
	}
	return result
}
