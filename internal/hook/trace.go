package hook

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/store"
)

const (
	traceTypeRequest      = "request"
	traceTypeUpstreamReq  = "upstream_req"
	traceTypeUpstreamResp = "upstream_resp"
	traceTypeResponse     = "response"

	// traceTimeFormat is RFC3339 with millisecond precision (3 digits).
	traceTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

// TraceRecorder records each pipeline stage as a JSONL line and aggregates token usage.
type TraceRecorder struct {
	writer     io.Writer // JSONL file writer (DailyRotateWriter)
	usageStore *store.UsageStore
}

func NewTraceRecorder(writer io.Writer, usageStore *store.UsageStore) *TraceRecorder {
	return &TraceRecorder{
		writer:     writer,
		usageStore: usageStore,
	}
}

// NoopTraceRecorder returns a no-op recorder safe to call when tracing is disabled.
func NoopTraceRecorder() *TraceRecorder {
	return &TraceRecorder{}
}

// traceRecord is a single JSONL trace line.
type traceRecord struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Time      string `json:"time"`

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

// RecordRequest writes a "request" trace record after stepParse.
func (t *TraceRecorder) RecordRequest(ctx *Context) {
	if t.writer == nil {
		return
	}
	t.writeRecord(&traceRecord{
		Type:           traceTypeRequest,
		RequestID:      ctx.RequestID,
		Time:           ctx.StartTime.Format(traceTimeFormat),
		ClientProtocol: ctx.ClientProtocol,
		Model:          ctx.ClientModel,
		Stream:         ctx.IsStream,
		Body:           string(ctx.ClientReqBody),
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
		UpstreamProtocol: ctx.UpstreamProtocol,
		Model:            ctx.ClientModel,
		Provider:         ctx.RouteResult.ProviderKey,
		URL:              router.BuildUpstreamURL(ctx.RouteResult.BaseURL, ctx.RouteResult.Path),
		Body:             string(ctx.UpstreamReqBody),
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
		Status:    status,
		LatencyMs: ctx.UpstreamLatency.Milliseconds(),
		Body:      string(ctx.UpstreamRespBody),
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
// Also records token usage to the usage store.
func (t *TraceRecorder) RecordResponse(ctx *Context) {
	if t.writer == nil && t.usageStore == nil {
		return
	}
	provider := ""
	if ctx.RouteResult != nil {
		provider = ctx.RouteResult.ProviderKey
	}

	if t.usageStore != nil && (ctx.InputTokens > 0 || ctx.OutputTokens > 0) {
		t.usageStore.AsyncRecord(store.UsageRecord{
			Provider:     provider,
			Model:        ctx.ClientModel,
			Date:         store.Today(),
			Requests:     1,
			InputTokens:  ctx.InputTokens,
			OutputTokens: ctx.OutputTokens,
			TotalTokens:  ctx.InputTokens + ctx.OutputTokens,
		})
	}

	if t.writer == nil {
		return
	}
	t.writeRecord(&traceRecord{
		Type:         traceTypeResponse,
		RequestID:    ctx.RequestID,
		Time:         time.Now().Format(traceTimeFormat),
		Provider:     provider,
		Model:        ctx.ClientModel,
		InputTokens:  ctx.InputTokens,
		OutputTokens: ctx.OutputTokens,
		Body:         string(ctx.ClientRespBody),
		Headers:      headersToMap(ctx.GinCtx.Writer.Header()),
	})
}

func (t *TraceRecorder) writeRecord(rec *traceRecord) {
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	t.writer.Write(append(data, '\n'))
}

// headersToMap converts http.Header to map[string]string by joining multi-values with comma.
func headersToMap(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	m := make(map[string]string, len(h))
	for k, v := range h {
		m[strings.ToLower(k)] = strings.Join(v, ", ")
	}
	return m
}
