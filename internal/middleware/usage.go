package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/llm-gateway/internal/store"
)

// responseCapture wraps gin.ResponseWriter to capture the response body.
type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseCapture) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseCapture) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// UsageMiddleware records API usage statistics via a Gin middleware.
func UsageMiddleware(usageStore *store.UsageStore, provider string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only track API endpoints
		path := c.Request.URL.Path
		if path == "/health" || path == "/api/reload" {
			c.Next()
			return
		}

		wrap := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = wrap

		c.Next()

		// Skip non-200 responses
		if c.Writer.Status() != http.StatusOK {
			return
		}

		// Skip streaming responses (Content-Type is text/event-stream)
		contentType := wrap.Header().Get("Content-Type")
		if contentType == "text/event-stream" {
			return
		}

		usage := extractUsage(wrap.body.Bytes(), provider)
		if usage != nil {
			usageStore.AsyncRecord(*usage)
		}
	}
}

// extractUsage tries to extract usage data from various response formats.
func extractUsage(body []byte, provider string) *store.UsageRecord {
	if len(body) == 0 {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}

	usage, ok := raw["usage"].(map[string]any)
	if !ok {
		return nil
	}

	record := &store.UsageRecord{
		Provider: provider,
		Date:     store.Today(),
		Requests: 1,
	}

	// Extract token counts: Anthropic uses input_tokens/output_tokens,
	// Chat uses prompt_tokens/completion_tokens
	record.InputTokens = int64(toFloat(usage["input_tokens"]))
	record.OutputTokens = int64(toFloat(usage["output_tokens"]))

	if record.InputTokens == 0 {
		record.InputTokens = int64(toFloat(usage["prompt_tokens"]))
	}
	if record.OutputTokens == 0 {
		record.OutputTokens = int64(toFloat(usage["completion_tokens"]))
	}

	record.TotalTokens = int64(toFloat(usage["total_tokens"]))
	if record.TotalTokens == 0 {
		record.TotalTokens = record.InputTokens + record.OutputTokens
	}

	// Cache tokens
	record.CacheCreationTokens = int64(toFloat(usage["cache_creation_input_tokens"]))
	record.CacheReadTokens = int64(toFloat(usage["cache_read_input_tokens"]))

	// Cache tokens from prompt_tokens_details (OpenAI format)
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		record.CacheReadTokens += int64(toFloat(details["cached_tokens"]))
	}

	// Extract model name
	if model, ok := raw["model"].(string); ok {
		record.Model = model
	}

	// Skip if no meaningful data
	if record.InputTokens == 0 && record.OutputTokens == 0 {
		return nil
	}

	slog.Debug("recorded usage", "provider", record.Provider, "model", record.Model,
		"input", record.InputTokens, "output", record.OutputTokens)

	return record
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}
