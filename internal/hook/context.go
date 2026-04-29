package hook

import (
	"fmt"
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
	ClientProtocol string // "anthropic" | "responses" | "chat"
	ClientReqBody  []byte // original raw request body
	ClientParsedReq any   // parsed struct (AnthropicRequest, ResponsesRequest, ChatRequest)
	ClientModel    string // model name from client request

	// Route
	RouteResult *router.RouteResult

	// Upstream side
	UpstreamProtocol string         // determined by RouteResult.Format
	UpstreamReqBody  []byte         // request body sent to upstream (after conversion)
	UpstreamResp     *http.Response // raw response from upstream
	UpstreamRespBody []byte         // accumulated response body or raw SSE
	UpstreamLatency  time.Duration

	// Client response
	ClientRespBody []byte // response body sent to client (after conversion)

	// Stream
	IsStream   bool
	StreamState any // protocol-specific stream conversion state

	// Hook mutable store
	Extra map[string]any
}

func NewContext(c *gin.Context, protocol string, body []byte) *Context {
	return &Context{
		GinCtx:         c,
		RequestID:      generateRequestID(),
		StartTime:      time.Now(),
		ClientProtocol: protocol,
		ClientReqBody:  body,
		Extra:          make(map[string]any),
	}
}

func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}
