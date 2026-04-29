package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

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

// stepConvertReq converts the client request to the upstream protocol format.
func (h *Handler) stepConvertReq(ctx *hook.Context) error {
	if ctx.ClientProtocol == ctx.UpstreamProtocol {
		return h.passthroughConvertReq(ctx)
	}
	switch ctx.ClientProtocol {
	case "anthropic":
		return h.convertAnthropicReq(ctx)
	case "responses":
		return h.convertResponsesReq(ctx)
	case "chat":
		return h.convertChatReq(ctx)
	}
	return fmt.Errorf("unknown client protocol: %s", ctx.ClientProtocol)
}

func (h *Handler) passthroughConvertReq(ctx *hook.Context) error {
	var raw map[string]any
	if err := json.Unmarshal(ctx.ClientReqBody, &raw); err != nil {
		ctx.UpstreamReqBody = ctx.ClientReqBody
		return nil
	}
	if ctx.ClientModel != "" {
		raw["model"] = ctx.ClientModel
	}
	normalizeInputRoles(raw)
	body, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	ctx.UpstreamReqBody = body
	return nil
}

func (h *Handler) convertAnthropicReq(ctx *hook.Context) error {
	req := ctx.ClientParsedReq.(*converter.AnthropicRequest)
	switch ctx.UpstreamProtocol {
	case "chat":
		chatReq, err := h.converter.AnthropicToChat(req)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		chatReq.Model = ctx.ClientModel
		chatReq.Stream = ctx.IsStream
		if ctx.IsStream {
			chatReq.StreamOptions = &types.StreamOptions{IncludeUsage: true}
		}
		body, _ := json.Marshal(chatReq)
		ctx.UpstreamReqBody = body
	case "responses":
		respReq, err := h.converter.AnthropicToResponses(req)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		respReq.Model = ctx.ClientModel
		body, _ := json.Marshal(respReq)
		ctx.UpstreamReqBody = body
	}
	return nil
}

func (h *Handler) convertResponsesReq(ctx *hook.Context) error {
	req := ctx.ClientParsedReq.(*types.ResponsesRequest)
	switch ctx.UpstreamProtocol {
	case "chat":
		chatReq, err := h.converter.ResponsesToChat(req)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		chatReq.Model = ctx.ClientModel
		chatReq.Stream = ctx.IsStream
		if ctx.IsStream {
			chatReq.StreamOptions = &types.StreamOptions{IncludeUsage: true}
		}
		body, _ := json.Marshal(chatReq)
		ctx.UpstreamReqBody = body
	case "anthropic":
		anthReq, err := h.converter.ResponsesToAnthropic(req)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		anthReq.Model = ctx.ClientModel
		anthReq.Stream = ctx.IsStream
		body, _ := json.Marshal(anthReq)
		ctx.UpstreamReqBody = body
	}
	return nil
}

func (h *Handler) convertChatReq(ctx *hook.Context) error {
	req := ctx.ClientParsedReq.(*types.ChatRequest)
	switch ctx.UpstreamProtocol {
	case "anthropic":
		anthReq, err := h.converter.ChatRequestToAnthropic(req)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		anthReq.Stream = ctx.IsStream
		if ctx.ClientModel != "" {
			anthReq.Model = ctx.ClientModel
		}
		body, _ := json.Marshal(anthReq)
		ctx.UpstreamReqBody = body
	case "responses":
		respReq := converter.BuildResponsesFromChat(req, ctx.IsStream)
		respReq.Model = ctx.ClientModel
		body, _ := json.Marshal(respReq)
		ctx.UpstreamReqBody = body
	}
	return nil
}

// stepForward sends the converted request to the upstream API.
func (h *Handler) stepForward(ctx *hook.Context) error {
	upstreamURL := buildUpstreamURL(ctx.RouteResult)
	req, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(ctx.UpstreamReqBody))
	if err != nil {
		writeUpstreamError(ctx.GinCtx, "failed to create request: "+err.Error())
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	switch ctx.UpstreamProtocol {
	case "anthropic":
		req.Header.Set("x-api-key", ctx.RouteResult.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+ctx.RouteResult.APIKey)
	}

	start := time.Now()
	resp, err := h.client.Do(req)
	if err != nil {
		writeUpstreamError(ctx.GinCtx, "failed to call upstream: "+err.Error())
		return err
	}
	ctx.UpstreamLatency = time.Since(start)
	ctx.UpstreamResp = resp

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		ctx.UpstreamRespBody = respBody
		h.logLLMRequest(ctx.ClientModel, ctx.RouteResult.ProviderKey, upstreamURL,
			ctx.UpstreamLatency, ctx.IsStream, ctx.UpstreamReqBody, string(respBody))
		h.writeConvertedError(ctx.GinCtx, resp, respBody, ctx.ClientProtocol)
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	return nil
}

// stepWriteResp — stub to be implemented in Task 6
func (h *Handler) stepWriteResp(ctx *hook.Context) error { return nil }

func writeConversionError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": message}})
}

func writeUpstreamError(c *gin.Context, message string) {
	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": message}})
}

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
