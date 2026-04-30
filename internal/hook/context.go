package hook

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/router"
)

type Context struct {
	GinCtx    *gin.Context
	RequestID string
	StartTime time.Time

	// Client side
	ClientProtocol  string // "anthropic" | "responses" | "chat"
	ClientReqBody   []byte // original raw request body
	ClientParsedReq any    // parsed struct (AnthropicRequest, ResponsesRequest, ChatRequest)
	ClientModel     string // model name from client request
	SessionID       string // conversation session ID extracted from request metadata

	// Route
	RouteResult *router.RouteResult

	// Upstream side
	UpstreamProtocol  string         // determined by RouteResult.Format
	UpstreamReqBody   []byte         // request body sent to upstream (after conversion)
	UpstreamReqHeader http.Header    // headers sent to upstream
	UpstreamResp      *http.Response // raw response from upstream
	UpstreamRespBody  []byte         // accumulated response body or raw SSE
	UpstreamLatency   time.Duration

	// Client response
	ClientRespBody []byte // response body sent to client (after conversion)

	// Token usage (populated by stepWriteResp)
	InputTokens       int64
	OutputTokens      int64
	CacheReadTokens   int64
	CacheCreateTokens int64

	// Stream
	IsStream    bool
	StreamState any // protocol-specific stream conversion state

	// Hook mutable store
	Extra map[string]any
}

func NewContext(c *gin.Context, protocol string, body []byte) *Context {
	return &Context{
		GinCtx:         c,
		RequestID:      NewRequestID(),
		StartTime:      time.Now(),
		ClientProtocol: protocol,
		ClientReqBody:  body,
		Extra:          make(map[string]any),
	}
}

// NewRequestID generates a globally unique request ID with a datetime prefix.
// Format: YYYYMMDDHHmmss + 6 random hex chars (e.g. "20260430075235a3f1b2c").
func NewRequestID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return time.Now().Format("20060102150405") + hex.EncodeToString(b)
}
