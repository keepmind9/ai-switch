package hook

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/router"
)

const (
	traceTypeRequest      = "request"
	traceTypeUpstreamReq  = "upstream_req"
	traceTypeUpstreamResp = "upstream_resp"
	traceTypeResponse     = "response"

	// traceTimeFormat is RFC3339 with millisecond precision (3 digits).
	traceTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

// TraceRecorder records each pipeline stage as a JSONL line.
type TraceRecorder struct {
	writer      io.Writer // JSONL file writer (DailyRotateWriter)
	indexWriter io.Writer // index file writer (DailyRotateWriter), may be nil
}

func NewTraceRecorder(writer io.Writer, indexWriter io.Writer) *TraceRecorder {
	return &TraceRecorder{
		writer:      writer,
		indexWriter: indexWriter,
	}
}

// NoopTraceRecorder returns a no-op recorder safe to call when tracing is disabled.
func NoopTraceRecorder() *TraceRecorder {
	return &TraceRecorder{}
}

// traceRecord is a single JSONL trace line.
type traceRecord struct {
	Type      string `json:"type"`
	RequestID string `json:"ais_req_id"`
	Time      string `json:"time"`
	SessionID string `json:"session_id,omitempty"`

	// request
	ClientProtocol string            `json:"client_protocol,omitempty"`
	Model          string            `json:"model,omitempty"`
	Stream         bool              `json:"stream,omitempty"`
	Body           string            `json:"body,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`

	// upstream_request
	UpstreamProtocol string `json:"upstream_protocol,omitempty"`
	Provider         string `json:"provider,omitempty"`
	URL              string `json:"url,omitempty"`

	// upstream_response
	Status    int   `json:"status,omitempty"`
	LatencyMs int64 `json:"latency_ms,omitempty"`

	// response
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
}

// traceIndex is a single index entry written once per request (on RecordResponse).
type traceIndex struct {
	AisReqID       string `json:"ais_req_id"`
	Time           string `json:"time"`
	SessionID      string `json:"session_id,omitempty"`
	ClientProtocol string `json:"client_protocol,omitempty"`
	Model          string `json:"model,omitempty"`
	Stream         bool   `json:"stream,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Status         int    `json:"status,omitempty"`
	LatencyMs      int64  `json:"latency_ms,omitempty"`
	InputTokens    int64  `json:"input_tokens,omitempty"`
	OutputTokens   int64  `json:"output_tokens,omitempty"`
	Offset         int64  `json:"offset"`
}

// RecordRequest writes a "request" trace record after stepParse.
func (t *TraceRecorder) RecordRequest(ctx *Context) {
	if t.writer == nil {
		return
	}
	ctx.traceOffset = t.writeRecord(&traceRecord{
		Type:           traceTypeRequest,
		RequestID:      ctx.RequestID,
		Time:           ctx.StartTime.Format(traceTimeFormat),
		SessionID:      ctx.SessionID,
		ClientProtocol: ctx.ClientProtocol,
		Model:          ctx.ClientModel,
		Stream:         ctx.IsStream,
		Body:           redactBody(string(ctx.ClientReqBody)),
		Headers:        headersToMap(ctx.GinCtx.Request.Header),
	})
}

// RecordUpstreamRequest writes an "upstream_request" trace record after stepConvertReq.
func (t *TraceRecorder) RecordUpstreamRequest(ctx *Context) {
	if t.writer == nil {
		return
	}
	if ctx.RouteResult == nil {
		return
	}
	t.writeRecord(&traceRecord{
		Type:             traceTypeUpstreamReq,
		RequestID:        ctx.RequestID,
		Time:             time.Now().Format(traceTimeFormat),
		SessionID:        ctx.SessionID,
		UpstreamProtocol: ctx.UpstreamProtocol,
		Model:            ctx.ClientModel,
		Provider:         ctx.RouteResult.ProviderKey,
		URL:              router.BuildUpstreamURL(ctx.RouteResult.BaseURL, ctx.RouteResult.Path),
		Body:             redactBody(string(ctx.UpstreamReqBody)),
		Headers:          headersToMap(ctx.UpstreamReqHeader),
	})
}

// RecordUpstreamResponse writes an "upstream_response" trace record after stepForward.
func (t *TraceRecorder) RecordUpstreamResponse(ctx *Context, status int) {
	if t.writer == nil {
		return
	}
	rec := &traceRecord{
		Type:      traceTypeUpstreamResp,
		RequestID: ctx.RequestID,
		Time:      time.Now().Format(traceTimeFormat),
		SessionID: ctx.SessionID,
		Status:    status,
		LatencyMs: ctx.UpstreamLatency.Milliseconds(),
		Body:      redactBody(string(ctx.UpstreamRespBody)),
	}
	if ctx.UpstreamResp != nil {
		rec.Headers = headersToMap(ctx.UpstreamResp.Header)
	}
	if ctx.RouteResult != nil {
		rec.Provider = ctx.RouteResult.ProviderKey
		rec.URL = router.BuildUpstreamURL(ctx.RouteResult.BaseURL, ctx.RouteResult.Path)
	}
	t.writeRecord(rec)
}

// RecordResponse writes a "response" trace record after stepWriteResp.
func (t *TraceRecorder) RecordResponse(ctx *Context) {
	if t.writer == nil {
		return
	}
	provider := ""
	if ctx.RouteResult != nil {
		provider = ctx.RouteResult.ProviderKey
	}

	t.writeRecord(&traceRecord{
		Type:         traceTypeResponse,
		RequestID:    ctx.RequestID,
		Time:         time.Now().Format(traceTimeFormat),
		SessionID:    ctx.SessionID,
		Provider:     provider,
		Model:        ctx.ClientModel,
		InputTokens:  ctx.InputTokens,
		OutputTokens: ctx.OutputTokens,
		Body:         redactBody(string(ctx.ClientRespBody)),
		Headers:      headersToMap(ctx.GinCtx.Writer.Header()),
	})

	if t.indexWriter != nil {
		status := 0
		if ctx.UpstreamResp != nil {
			status = ctx.UpstreamResp.StatusCode
		}
		idx := traceIndex{
			AisReqID:       ctx.RequestID,
			Time:           ctx.StartTime.Format(traceTimeFormat),
			SessionID:      ctx.SessionID,
			ClientProtocol: ctx.ClientProtocol,
			Model:          ctx.ClientModel,
			Stream:         ctx.IsStream,
			Provider:       provider,
			Status:         status,
			LatencyMs:      ctx.UpstreamLatency.Milliseconds(),
			InputTokens:    ctx.InputTokens,
			OutputTokens:   ctx.OutputTokens,
			Offset:         ctx.traceOffset,
		}
		data, _ := json.Marshal(idx)
		t.indexWriter.Write(append(data, '\n'))
	}
}

func (t *TraceRecorder) writeRecord(rec *traceRecord) int64 {
	data, err := json.Marshal(rec)
	if err != nil {
		return 0
	}
	buf := append(data, '\n')

	if ow, ok := t.writer.(interface {
		WriteWithOffset(p []byte) (int, int64, error)
	}); ok {
		_, offset, _ := ow.WriteWithOffset(buf)
		return offset
	}
	t.writer.Write(buf)
	return 0
}

// redactBody redacts sensitive keys from a JSON body string (keys shown, values masked).
func redactBody(body string) string {
	// Simple string-based redaction: replaces values of known sensitive keys with partial mask.
	// For each sensitive key, find "key":"value" or "key": "value" and mask the value.
	result := []byte(body)
	for _, key := range sensitiveBodyKeys {
		result = redactKeyValue(result, key)
	}
	return string(result)
}

var sensitiveBodyKeys = []string{"api_key", "apiKey", "authorization", "secret", "password", "token"}

func redactKeyValue(body []byte, key string) []byte {
	quoted := `"` + key + `"`
	quotedBytes := []byte(quoted)
	start := 0
	for {
		idx := indexBytes(body, start, quotedBytes[0])
		if idx < 0 || idx+len(quotedBytes) > len(body) {
			break
		}
		// Check if this is the full key match (next char must be ':')
		if string(body[idx:idx+len(quotedBytes)]) != key {
			start = idx + 1
			continue
		}
		colonIdx := idx + len(quotedBytes)
		if colonIdx >= len(body) || body[colonIdx] != ':' {
			start = idx + 1
			continue
		}
		// Find opening quote of value
		q := colonIdx + 1
		for q < len(body) && (body[q] == ' ' || body[q] == '\t') {
			q++
		}
		if q >= len(body) || body[q] != '"' {
			start = idx + 1
			continue
		}
		// Find closing quote of value, handling escaped characters.
		end := q + 1
		for end < len(body) {
			if body[end] == '\\' {
				end += 2 // skip escape sequence
				continue
			}
			if body[end] == '"' {
				break
			}
			end++
		}
		if end >= len(body) || body[end] != '"' {
			start = idx + 1
			continue
		}
		val := body[q+1 : end]
		masked := redactValue(string(val))
		// Replace in place: keep key and quotes, replace value
		head := body[:q+1]
		tail := body[end+1:]
		body = concat(head, []byte(masked), tail)
		start = len(head) + len(masked) + 1
	}
	return body
}

func redactValue(v string) string {
	if len(v) <= 12 {
		return "***"
	}
	return v[:4] + "***" + v[len(v)-4:]
}

func concat(a, b, c []byte) []byte {
	r := make([]byte, 0, len(a)+len(b)+len(c))
	return append(append(append(r, a...), b...), c...)
}

func indexBytes(s []byte, start int, c byte) int {
	for i := start; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// sensitiveHeaders is the set of header keys that get redacted.
var sensitiveHeaders = map[string]bool{
	"authorization": true,
	"x-api-key":     true,
	"api-key":       true,
	"x-auth-token":  true,
	"secret":        true,
	"cookie":        true,
	"set-cookie":    true,
}

// headersToMap converts http.Header to map[string]string by joining multi-values with comma.
// Sensitive headers are partially masked: first 8 + last 4 characters shown.
func headersToMap(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	m := make(map[string]string, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		if sensitiveHeaders[lk] {
			m[lk] = redactValue(strings.Join(v, ", "))
		} else {
			m[lk] = strings.Join(v, ", ")
		}
	}
	return m
}
