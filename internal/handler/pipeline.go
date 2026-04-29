package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/keepmind9/ai-switch/internal/types"
)

// executePipeline runs the unified request pipeline with hook support.
func (h *Handler) executePipeline(c *gin.Context, protocol string, body []byte) {
	ctx := hook.NewContext(c, protocol, body)
	hooks := hook.NewManager() // Will be replaced with Handler.hooks in Task 7

	type pipelineStep struct {
		name string
		fn   func(ctx *hook.Context) error
		pre  hook.Point
		post hook.Point
	}

	steps := []pipelineStep{
		{"parse", h.stepParse, -1, hook.BeforeRoute},
		{"route", h.stepRoute, -1, hook.AfterRoute},
		{"convertReq", h.stepConvertReq, hook.BeforeUpstream, -1},
		{"forward", h.stepForward, -1, hook.AfterUpstream},
		{"writeResp", h.stepWriteResp, -1, hook.AfterResponse},
	}

	for _, s := range steps {
		if s.pre >= 0 {
			if err := hooks.Fire(ctx, s.pre); err != nil {
				slog.Error("critical hook aborted request", "hook_point", hook.PointName(s.pre), "error", err)
				return
			}
		}
		if err := s.fn(ctx); err != nil {
			return
		}
		if s.post >= 0 {
			if err := hooks.Fire(ctx, s.post); err != nil {
				slog.Error("critical hook aborted request", "hook_point", hook.PointName(s.post), "error", err)
				return
			}
		}
	}
}

func (h *Handler) stepParse(ctx *hook.Context) error {
	switch ctx.ClientProtocol {
	case "anthropic":
		var req converter.AnthropicRequest
		if err := json.Unmarshal(ctx.ClientReqBody, &req); err != nil {
			writeBadRequest(ctx.GinCtx, "anthropic", "failed to parse request: "+err.Error())
			return err
		}
		ctx.ClientParsedReq = &req
		ctx.ClientModel = req.Model
		ctx.IsStream = req.Stream
	case "responses":
		var req types.ResponsesRequest
		if err := json.Unmarshal(ctx.ClientReqBody, &req); err != nil {
			writeBadRequest(ctx.GinCtx, "responses", "failed to parse request: "+err.Error())
			return err
		}
		ctx.ClientParsedReq = &req
		ctx.ClientModel = req.Model
		ctx.IsStream = req.Stream
	case "chat":
		var req types.ChatRequest
		if err := json.Unmarshal(ctx.ClientReqBody, &req); err != nil {
			writeBadRequest(ctx.GinCtx, "chat", "failed to parse request: "+err.Error())
			return err
		}
		ctx.ClientParsedReq = &req
		ctx.ClientModel = req.Model
		ctx.IsStream = req.Stream
	default:
		writeBadRequest(ctx.GinCtx, ctx.ClientProtocol, "unsupported protocol: "+ctx.ClientProtocol)
		return fmt.Errorf("unsupported protocol: %s", ctx.ClientProtocol)
	}
	return nil
}

func (h *Handler) stepRoute(ctx *hook.Context) error {
	apiKey := extractClientAPIKey(ctx.GinCtx)
	result, routeErr := h.router.Route(ctx.ClientProtocol, apiKey, ctx.ClientReqBody)
	if routeErr != nil {
		slog.Error("route error", "error", routeErr, "protocol", ctx.ClientProtocol, "api_key", apiKey)
		writeRouteError(ctx.GinCtx, routeErr.Error())
		return routeErr
	}
	ctx.RouteResult = result
	ctx.UpstreamProtocol = result.Format
	if ctx.UpstreamProtocol == "" {
		ctx.UpstreamProtocol = "chat"
	}
	ctx.GinCtx.Set(ctxProviderKey, result.ProviderKey)

	resolvedModel := result.Model
	if resolvedModel == "" {
		resolvedModel = ctx.ClientModel
	}

	slog.Info(ctx.ClientProtocol+" request",
		"model", ctx.ClientModel,
		"stream", ctx.IsStream,
		"upstream_format", ctx.UpstreamProtocol,
		"upstream_url", buildUpstreamURL(result),
		"resolved_model", resolvedModel,
	)

	ctx.ClientModel = resolvedModel
	return nil
}

// Stubs — to be implemented in Tasks 4-6
func (h *Handler) stepConvertReq(ctx *hook.Context) error { return nil }
func (h *Handler) stepForward(ctx *hook.Context) error     { return nil }
func (h *Handler) stepWriteResp(ctx *hook.Context) error   { return nil }

func writeBadRequest(c *gin.Context, protocol, message string) {
	switch protocol {
	case "anthropic":
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": message}})
	}
}

func writeRouteError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "route_error", "message": message}})
}
