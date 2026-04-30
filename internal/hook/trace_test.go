package hook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceRecorder_RecordRequest_StoresOffset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	rec := NewTraceRecorder(&offsetStubWriter{w: &buf}, nil)

	c := &Context{
		GinCtx:         &gin.Context{Request: httptest.NewRequest(http.MethodPost, "/", nil)},
		RequestID:      "req001",
		ClientProtocol: "chat",
		ClientModel:    "gpt-4o",
		IsStream:       true,
	}
	c.StartTime = c.StartTime.UTC()

	rec.RecordRequest(c)
	assert.Equal(t, int64(0), c.traceOffset)
}

func TestTraceRecorder_RecordResponse_WritesIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var dataBuf, idxBuf bytes.Buffer
	rec := NewTraceRecorder(&offsetStubWriter{w: &dataBuf}, &idxBuf)

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	c := &Context{
		GinCtx:          ginCtx,
		RequestID:       "req001",
		ClientModel:     "claude-sonnet-4-6",
		IsStream:        true,
		InputTokens:     100,
		OutputTokens:    50,
		UpstreamResp:    &http.Response{StatusCode: 200},
		UpstreamLatency: 1234 * time.Millisecond,
	}
	c.StartTime = c.StartTime.UTC()
	c.RouteResult = nil // provider will be empty

	// Simulate pipeline: RecordRequest first
	rec.RecordRequest(c)
	offset := c.traceOffset

	// RecordResponse writes index entry
	rec.RecordResponse(c)

	// Verify index was written
	lines := bytes.Split(idxBuf.Bytes(), []byte("\n"))
	var nonEmpty [][]byte
	for _, l := range lines {
		if len(l) > 0 {
			nonEmpty = append(nonEmpty, l)
		}
	}
	require.Len(t, nonEmpty, 1)

	var idx traceIndex
	require.NoError(t, json.Unmarshal(nonEmpty[0], &idx))
	assert.Equal(t, "req001", idx.AisReqID)
	assert.Equal(t, "claude-sonnet-4-6", idx.Model)
	assert.Equal(t, int64(offset), idx.Offset)
	assert.Equal(t, 200, idx.Status)
	assert.Equal(t, int64(1234), idx.LatencyMs)
	assert.Equal(t, int64(100), idx.InputTokens)
	assert.Equal(t, int64(50), idx.OutputTokens)
}

func TestTraceRecorder_NoIndexWhenNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var dataBuf bytes.Buffer
	rec := NewTraceRecorder(&offsetStubWriter{w: &dataBuf}, nil)

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	c := &Context{
		GinCtx:      ginCtx,
		RequestID:   "req001",
		ClientModel: "gpt-4o",
	}
	c.StartTime = c.StartTime.UTC()

	// Should not panic
	rec.RecordRequest(c)
	rec.RecordResponse(c)

	assert.Equal(t, int64(0), c.traceOffset)
}

func TestTraceRecorder_NoopWithNilWriter(t *testing.T) {
	rec := NoopTraceRecorder()
	c := &Context{RequestID: "req001"}
	c.StartTime = c.StartTime.UTC()

	// Should not panic
	rec.RecordRequest(c)
	rec.RecordResponse(c)
	assert.Equal(t, int64(0), c.traceOffset)
}

func TestTraceRecorder_IndexEntryWithProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var dataBuf, idxBuf bytes.Buffer
	rec := NewTraceRecorder(&offsetStubWriter{w: &dataBuf}, &idxBuf)

	w := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(w)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	c := &Context{
		GinCtx:       ginCtx,
		RequestID:    "req002",
		ClientModel:  "gpt-4o",
		SessionID:    "sess-abc",
		InputTokens:  200,
		OutputTokens: 80,
		UpstreamResp: &http.Response{StatusCode: 200},
	}
	c.StartTime = c.StartTime.UTC()
	c.RouteResult = &router.RouteResult{ProviderKey: "anthropic"}
	c.UpstreamLatency = 500 * time.Millisecond

	rec.RecordRequest(c)
	rec.RecordResponse(c)

	var idx traceIndex
	require.NoError(t, json.Unmarshal(idxBuf.Bytes(), &idx))
	assert.Equal(t, "req002", idx.AisReqID)
	assert.Equal(t, "sess-abc", idx.SessionID)
	assert.Equal(t, "gpt-4o", idx.Model)
	assert.Equal(t, "anthropic", idx.Provider)
	assert.Equal(t, int64(500), idx.LatencyMs)
}

// offsetStubWriter simulates WriteWithOffset for testing.
type offsetStubWriter struct {
	w       *bytes.Buffer
	written int
}

func (o *offsetStubWriter) Write(p []byte) (int, error) {
	n, err := o.w.Write(p)
	o.written += n
	return n, err
}

func (o *offsetStubWriter) WriteWithOffset(p []byte) (int, int64, error) {
	offset := int64(o.written)
	n, err := o.w.Write(p)
	o.written += n
	return n, offset, err
}
