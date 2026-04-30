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

const noHook hook.Point = -1

func (h *Handler) tracer() *hook.TraceRecorder {
	if h.trace == nil {
		return hook.NoopTraceRecorder()
	}
	return h.trace
}

// executePipeline runs the unified request pipeline with hook support.
func (h *Handler) executePipeline(c *gin.Context, protocol string, body []byte) {
	ctx := hook.NewContext(c, protocol, body)
	hooks := h.hooks

	type pipelineStep struct {
		name string
		fn   func(ctx *hook.Context) error
		pre  hook.Point
		post hook.Point
	}

	steps := []pipelineStep{
		{"parse", h.stepParse, noHook, hook.BeforeRoute},
		{"route", h.stepRoute, noHook, hook.AfterRoute},
		{"convertReq", h.stepConvertReq, hook.BeforeUpstream, noHook},
		{"forward", h.stepForward, noHook, hook.AfterUpstream},
		{"writeResp", h.stepWriteResp, noHook, hook.AfterResponse},
	}

	h.tracer().RecordRequest(ctx)

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
	case converter.FormatAnthropic:
		var req converter.AnthropicRequest
		if err := json.Unmarshal(ctx.ClientReqBody, &req); err != nil {
			writeBadRequest(ctx.GinCtx, converter.FormatAnthropic, "failed to parse request: "+err.Error())
			return err
		}
		ctx.ClientParsedReq = &req
		ctx.ClientModel = req.Model
		ctx.IsStream = req.Stream
	case converter.FormatResponses:
		var req types.ResponsesRequest
		if err := json.Unmarshal(ctx.ClientReqBody, &req); err != nil {
			writeBadRequest(ctx.GinCtx, converter.FormatResponses, "failed to parse request: "+err.Error())
			return err
		}
		ctx.ClientParsedReq = &req
		ctx.ClientModel = req.Model
		ctx.IsStream = req.Stream
	case converter.FormatChat:
		var req types.ChatRequest
		if err := json.Unmarshal(ctx.ClientReqBody, &req); err != nil {
			writeBadRequest(ctx.GinCtx, converter.FormatChat, "failed to parse request: "+err.Error())
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
		ctx.UpstreamProtocol = converter.FormatChat
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
		if err := h.passthroughConvertReq(ctx); err != nil {
			return err
		}
	} else {
		switch ctx.ClientProtocol {
		case converter.FormatAnthropic:
			if err := h.convertAnthropicReq(ctx); err != nil {
				return err
			}
		case converter.FormatResponses:
			if err := h.convertResponsesReq(ctx); err != nil {
				return err
			}
		case converter.FormatChat:
			if err := h.convertChatReq(ctx); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown client protocol: %s", ctx.ClientProtocol)
		}
	}
	return nil
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
	case converter.FormatChat:
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
	case converter.FormatResponses:
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
	case converter.FormatChat:
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
	case converter.FormatAnthropic:
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
	case converter.FormatAnthropic:
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
	case converter.FormatResponses:
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
	case converter.FormatAnthropic:
		req.Header.Set("x-api-key", ctx.RouteResult.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+ctx.RouteResult.APIKey)
	}

	ctx.UpstreamReqHeader = req.Header
	h.tracer().RecordUpstreamRequest(ctx)

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
		h.tracer().RecordUpstreamResponse(ctx, resp.StatusCode)
		h.writeConvertedError(ctx.GinCtx, resp, respBody, ctx.ClientProtocol)
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	return nil
}

// stepWriteResp writes the upstream response to the client, handling protocol
// conversion for both streaming and non-streaming paths.
func (h *Handler) stepWriteResp(ctx *hook.Context) error {
	defer ctx.UpstreamResp.Body.Close()

	if !ctx.IsStream {
		return h.writeNonStreamResponse(ctx)
	}
	return h.writeStreamResponse(ctx)
}

// writeNonStreamResponse reads the full upstream response, converts if needed, and writes to client.
func (h *Handler) writeNonStreamResponse(ctx *hook.Context) error {
	respBody, _ := io.ReadAll(ctx.UpstreamResp.Body)
	ctx.UpstreamRespBody = respBody
	h.tracer().RecordUpstreamResponse(ctx, ctx.UpstreamResp.StatusCode)

	upstream := ctx.UpstreamProtocol
	client := ctx.ClientProtocol

	if upstream == client {
		copyUpstreamHeaders(ctx.GinCtx, ctx.UpstreamResp)
		ctx.GinCtx.Data(http.StatusOK, "application/json", respBody)
		ctx.ClientRespBody = respBody
		h.setNonStreamTokenUsage(ctx, respBody)
		h.tracer().RecordResponse(ctx)
		return nil
	}

	switch upstream {
	case converter.FormatChat:
		return h.writeNonStreamFromChat(ctx, respBody)
	case converter.FormatAnthropic:
		return h.writeNonStreamFromAnthropic(ctx, respBody)
	case converter.FormatResponses:
		copyUpstreamHeaders(ctx.GinCtx, ctx.UpstreamResp)
		ctx.GinCtx.Data(http.StatusOK, "application/json", respBody)
		ctx.ClientRespBody = respBody
		h.setNonStreamTokenUsage(ctx, respBody)
		h.tracer().RecordResponse(ctx)
		return nil
	}

	return fmt.Errorf("unknown upstream protocol: %s", upstream)
}

func (h *Handler) writeNonStreamFromChat(ctx *hook.Context, respBody []byte) error {
	var chatResp types.ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		writeConversionError(ctx.GinCtx, "failed to parse chat response")
		return err
	}
	ctx.InputTokens = int64(chatResp.Usage.TotalTokens)
	ctx.OutputTokens = int64(chatResp.Usage.CompletionTokens)

	switch ctx.ClientProtocol {
	case converter.FormatAnthropic:
		anthResp, err := h.converter.ChatToAnthropic(&chatResp, ctx.ClientModel, ctx.RouteResult.ThinkTag)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		body, _ := json.Marshal(anthResp)
		ctx.ClientRespBody = body
		ctx.GinCtx.JSON(http.StatusOK, anthResp)
	case converter.FormatResponses:
		responsesResp, err := h.converter.ChatToResponses(&chatResp, ctx.ClientModel, ctx.RouteResult.ThinkTag)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		body, _ := json.Marshal(responsesResp)
		ctx.ClientRespBody = body
		ctx.GinCtx.JSON(http.StatusOK, responsesResp)
	}
	h.tracer().RecordResponse(ctx)
	return nil
}

func (h *Handler) writeNonStreamFromAnthropic(ctx *hook.Context, respBody []byte) error {
	var anthResp converter.AnthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		writeConversionError(ctx.GinCtx, "failed to parse anthropic response")
		return err
	}
	ctx.InputTokens = int64(anthResp.Usage.InputTokens)
	ctx.OutputTokens = int64(anthResp.Usage.OutputTokens)

	switch ctx.ClientProtocol {
	case converter.FormatChat:
		chatResp, err := h.converter.AnthropicResponseToChat(&anthResp)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		body, _ := json.Marshal(chatResp)
		ctx.ClientRespBody = body
		ctx.GinCtx.JSON(http.StatusOK, chatResp)
	case converter.FormatResponses:
		responsesResp, err := h.converter.AnthropicResponseToResponses(&anthResp, ctx.ClientModel, ctx.RouteResult.ThinkTag)
		if err != nil {
			writeConversionError(ctx.GinCtx, err.Error())
			return err
		}
		body, _ := json.Marshal(responsesResp)
		ctx.ClientRespBody = body
		ctx.GinCtx.JSON(http.StatusOK, responsesResp)
	}
	h.tracer().RecordResponse(ctx)
	return nil
}

// setNonStreamTokenUsage extracts token counts from a non-stream JSON response.
func (h *Handler) setNonStreamTokenUsage(ctx *hook.Context, body []byte) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return
	}
	usage, _ := raw["usage"].(map[string]any)
	if usage == nil {
		return
	}
	ctx.InputTokens = toInt64(usage["input_tokens"]) + toInt64(usage["prompt_tokens"])
	ctx.OutputTokens = toInt64(usage["output_tokens"]) + toInt64(usage["completion_tokens"])
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

// writeStreamResponse handles all streaming response paths with protocol conversion.
func (h *Handler) writeStreamResponse(ctx *hook.Context) error {
	upstream := ctx.UpstreamProtocol
	client := ctx.ClientProtocol
	model := ctx.ClientModel
	thinkTag := ctx.RouteResult.ThinkTag

	// Same-protocol passthrough
	if upstream == client {
		content := h.streamPassthrough(ctx.GinCtx, ctx.UpstreamResp, client)
		ctx.UpstreamRespBody = []byte(content)
		h.tracer().RecordUpstreamResponse(ctx, http.StatusOK)
		ctx.ClientRespBody = ctx.UpstreamRespBody
		h.tracer().RecordResponse(ctx)
		return nil
	}

	switch upstream {
	case converter.FormatChat:
		return h.streamFromChat(ctx, model, thinkTag)
	case converter.FormatAnthropic:
		return h.streamFromAnthropic(ctx, model, thinkTag)
	case converter.FormatResponses:
		return h.streamFromResponses(ctx, model, thinkTag)
	}

	return fmt.Errorf("unknown upstream protocol for streaming: %s", upstream)
}

func (h *Handler) streamFromChat(ctx *hook.Context, model, thinkTag string) error {
	switch ctx.ClientProtocol {
	case converter.FormatAnthropic:
		state := &converter.AnthropicStreamState{Model: model, ThinkTag: thinkTag}
		content := h.streamChatToClient(ctx.GinCtx, ctx.UpstreamResp, func(w converter.SSEWriter, data string) bool {
			return converter.ConvertChatChunkToAnthropicSSE(w, state, data)
		}, converter.FormatAnthropic)
		ctx.UpstreamRespBody = []byte(content)
		ctx.InputTokens = int64(state.InputTokens)
		ctx.OutputTokens = int64(state.OutputTokens)

	case converter.FormatResponses:
		state := &converter.ResponsesStreamState{Created: time.Now().Unix(), Model: model, ThinkTag: thinkTag}
		content := h.streamChatToClient(ctx.GinCtx, ctx.UpstreamResp, func(w converter.SSEWriter, data string) bool {
			return converter.ConvertChatChunkToResponsesSSE(w, state, data)
		}, converter.FormatResponses)
		ctx.UpstreamRespBody = []byte(content)
		ctx.InputTokens = int64(state.InputTokens)
		ctx.OutputTokens = int64(state.OutputTokens)
	}
	h.tracer().RecordUpstreamResponse(ctx, http.StatusOK)
	ctx.ClientRespBody = ctx.UpstreamRespBody
	h.tracer().RecordResponse(ctx)
	return nil
}

func (h *Handler) streamFromAnthropic(ctx *hook.Context, model, thinkTag string) error {
	switch ctx.ClientProtocol {
	case converter.FormatChat:
		state := &converter.AnthropicToChatState{}
		content := h.streamToChatSSE(ctx.GinCtx, ctx.UpstreamResp, func(s any, line string) any {
			return converter.ConvertAnthropicLineToChat(s.(*converter.AnthropicToChatState), line)
		}, state)
		ctx.UpstreamRespBody = []byte(content)
		ctx.InputTokens = int64(state.InputTokens)
		ctx.OutputTokens = int64(state.OutputTokens)

	case converter.FormatResponses:
		state := &converter.AnthropicToResponsesState{ThinkTag: thinkTag}
		content := h.streamAnthropicToResponsesSSE(ctx.GinCtx, ctx.UpstreamResp, state)
		ctx.UpstreamRespBody = []byte(content)
		ctx.InputTokens = int64(state.InputTokens)
		ctx.OutputTokens = int64(state.OutputTokens)
	}
	h.tracer().RecordUpstreamResponse(ctx, http.StatusOK)
	h.tracer().RecordResponse(ctx)
	return nil
}

func (h *Handler) streamFromResponses(ctx *hook.Context, model, thinkTag string) error {
	switch ctx.ClientProtocol {
	case converter.FormatChat:
		state := &converter.ResponsesToChatState{}
		content := h.streamToChatSSE(ctx.GinCtx, ctx.UpstreamResp, func(s any, line string) any {
			return converter.ConvertResponsesLineToChat(s.(*converter.ResponsesToChatState), line)
		}, state)
		ctx.UpstreamRespBody = []byte(content)
		ctx.InputTokens = int64(state.InputTokens)
		ctx.OutputTokens = int64(state.OutputTokens)

	case converter.FormatAnthropic:
		state := &converter.ResponsesToAnthropicState{}
		content := h.streamChatToClient(ctx.GinCtx, ctx.UpstreamResp, func(w converter.SSEWriter, data string) bool {
			return converter.ConvertResponsesEventToAnthropicSSE(w, state, data)
		}, converter.FormatAnthropic)
		ctx.UpstreamRespBody = []byte(content)
		ctx.InputTokens = int64(state.InputTokens)
		ctx.OutputTokens = int64(state.OutputTokens)
	}
	h.tracer().RecordUpstreamResponse(ctx, http.StatusOK)
	h.tracer().RecordResponse(ctx)
	return nil
}

func writeConversionError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": message}})
}

func writeUpstreamError(c *gin.Context, message string) {
	c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": message}})
}

func writeBadRequest(c *gin.Context, protocol, message string) {
	switch protocol {
	case converter.FormatAnthropic:
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": message}})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": message}})
	}
}

func writeRouteError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "route_error", "message": message}})
}
