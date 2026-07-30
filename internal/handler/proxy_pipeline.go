package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/keepmind9/ai-switch/internal/store"
)

// executeProxyPipeline handles a request in proxy mode: same-format pure
// passthrough with NO protocol conversion. It reuses routing and key-fallback
// but bypasses the converter entirely - the request body is forwarded with
// only the top-level "model" field rewritten, and the response is streamed
// back to the client byte-for-byte (no SSE parsing, no usage sniffing).
//
// This path is selected only when the --proxy flag is on; the regular
// conversion pipeline in executePipeline is untouched otherwise.
func (h *Handler) executeProxyPipeline(c *gin.Context, protocol string, body []byte) {
	ctx := hook.NewContext(c, protocol, body)

	// 1. Parse: only the top-level model + stream flag are needed.
	model, isStream, err := parseProxyRequest(body)
	if err != nil {
		writeBadRequest(c, protocol, "failed to parse request: "+err.Error())
		return
	}
	ctx.ClientModel = model
	ctx.IsStream = isStream
	h.tracer().RecordRequest(ctx)

	// 2. Route: reuse the existing router (api-key routes, scene/model_map,
	// protocol-specific default_route, key fallback all still apply).
	apiKey := extractClientAPIKey(c)
	result, routeErr := h.router.Route(protocol, apiKey, body)
	if routeErr != nil {
		slog.Error("proxy route error", "error", routeErr, "protocol", protocol, "api_key", apiKey)
		writeRouteError(c, routeErr.Error())
		return
	}
	ctx.RouteResult = result
	// Resolve empty format to chat, matching router.FormatToPath("") and
	// handleCompact. An empty-format provider is a chat upstream, so the
	// same-format guard treats it as chat - not as "matches any protocol".
	upstreamFormat := result.Format
	if upstreamFormat == "" {
		upstreamFormat = converter.FormatChat
	}
	ctx.UpstreamProtocol = upstreamFormat
	c.Set(ctxProviderKey, result.ProviderKey)

	resolvedModel := result.Model
	if resolvedModel == "" {
		resolvedModel = model
	}
	ctx.ClientModel = resolvedModel

	// 3. Same-format guard: conversion is disabled in proxy mode. A provider
	// whose resolved format differs from the client protocol is a config error
	// - fail fast instead of silently forwarding a mismatched request (e.g. an
	// anthropic client routed to an empty-format provider whose path resolves
	// to /v1/chat/completions).
	if upstreamFormat != protocol {
		writeProxyFormatMismatch(c, protocol, result.ProviderKey, upstreamFormat)
		return
	}

	// 4. Rewrite request body: replace only the top-level "model" field,
	// preserving every other byte (no role normalization, no stream_options).
	upstreamBody, err := rewriteModel(body, resolvedModel)
	if err != nil {
		writeBadRequest(c, protocol, "failed to rewrite model: "+err.Error())
		return
	}
	ctx.UpstreamReqBody = upstreamBody

	// 5. Forward + 6. Write response (with key fallback on 429).
	upstreamURL := buildUpstreamURL(result)
	slog.Info(protocol+" request (proxy)",
		"model", resolvedModel,
		"stream", isStream,
		"upstream_url", upstreamURL,
	)
	if err := h.proxyForward(ctx, upstreamURL); err != nil {
		slog.Warn("proxy forward failed", "error", err)
	}
}

// parseProxyRequest extracts just the model name and stream flag from the
// request body. These fields are common across anthropic/responses/chat
// formats, so a single lightweight parse covers all client protocols.
func parseProxyRequest(body []byte) (model string, isStream bool, err error) {
	var req struct {
		Model  string `json:"model"`
		Stream any    `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", false, err
	}
	if b, ok := req.Stream.(bool); ok {
		isStream = b
	}
	return req.Model, isStream, nil
}

// rewriteModel returns body with the top-level "model" field replaced by
// model. When model is empty the body is returned verbatim (no parse). All
// other fields are preserved as json.RawMessage so unknown/future fields and
// nesting are forwarded untouched.
func rewriteModel(body []byte, model string) ([]byte, error) {
	if model == "" {
		return body, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	m, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	raw["model"] = m
	return json.Marshal(raw)
}

// proxyForward sends the rewritten request upstream, applying key fallback on
// 429, then streams the response back byte-for-byte via proxyWriteResponse.
func (h *Handler) proxyForward(ctx *hook.Context, upstreamURL string) error {
	// Let Go's transport own compression: dropping the client's Accept-Encoding
	// makes the transport add gzip automatically, decompress the response, and
	// strip Content-Encoding from resp.Header. resp.Body is therefore always
	// plain bytes, and copyProxyResponseHeaders forwards the remaining
	// end-to-end headers verbatim.
	ctx.GinCtx.Request.Header.Del("Accept-Encoding")

	providerKey := ctx.RouteResult.ProviderKey

	// No fallback keys: single attempt, no retry.
	if !h.keyMgr.HasFallbackKeys(providerKey) {
		resp, err := h.sendUpstreamRequest(ctx, upstreamURL, ctx.RouteResult.APIKey)
		if err != nil {
			// sendUpstreamRequest already wrote the error response,
			// recorded error usage, and traced the response on transport
			// failure. Returning here avoids double-counting (matches stepForward).
			return err
		}
		return h.proxyWriteResponse(ctx, resp)
	}

	// Fallback keys available: rotate on 429.
	triedKeys := make(map[string]bool)
	for {
		apiKey, ok := h.keyMgr.GetKey(providerKey)
		if !ok {
			writeUpstreamError(ctx.GinCtx, "all API keys are in cooldown due to rate limiting")
			h.recordErrorUsage(ctx)
			h.tracer().RecordResponse(ctx)
			return fmt.Errorf("all API keys rate-limited for provider %q", providerKey)
		}
		if triedKeys[apiKey] {
			break
		}
		triedKeys[apiKey] = true
		ctx.RouteResult.APIKey = apiKey
		resp, err := h.sendUpstreamRequest(ctx, upstreamURL, apiKey)
		if err != nil {
			return err
		}
		if isRateLimited(resp.StatusCode) {
			resp.Body.Close()
			cooled := h.keyMgr.Mark429(providerKey, apiKey)
			slog.Warn("upstream rate limited (proxy)",
				"status", resp.StatusCode,
				"provider", providerKey, "cooled_down", cooled,
				"tried", len(triedKeys))
			continue
		}
		h.keyMgr.ResetKey(providerKey, apiKey)
		return h.proxyWriteResponse(ctx, resp)
	}

	writeUpstreamError(ctx.GinCtx, "upstream rate limit exceeded")
	h.recordErrorUsage(ctx)
	h.tracer().RecordResponse(ctx)
	return fmt.Errorf("upstream rate-limited for provider %q", providerKey)
}

// proxyWriteResponse streams the upstream response to the client verbatim:
// status code, end-to-end headers (minus hop-by-hop), and raw body bytes. The
// body is never buffered or parsed, so SSE flows are flushed as they arrive.
func (h *Handler) proxyWriteResponse(ctx *hook.Context, resp *http.Response) error {
	defer resp.Body.Close()
	ctx.UpstreamLatency = time.Since(h.requestStartTime(ctx))
	ctx.UpstreamResp = resp

	copyProxyResponseHeaders(ctx.GinCtx, resp)
	ctx.GinCtx.Status(resp.StatusCode)

	flusher, canFlush := ctx.GinCtx.Writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := ctx.GinCtx.Writer.Write(buf[:n]); werr != nil {
				return werr
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Warn("proxy response read error", "error", err)
			}
			break
		}
	}

	// Record usage: request count only. Token counts are unavailable because
	// the response body is not parsed in proxy mode.
	h.recordProxyUsage(ctx, resp.StatusCode)
	h.tracer().RecordUpstreamResponse(ctx, resp.StatusCode)
	h.tracer().RecordResponse(ctx)
	return nil
}

// recordProxyUsage records a per-request usage entry (tokens are zero in proxy
// mode since the body is forwarded unread).
func (h *Handler) recordProxyUsage(ctx *hook.Context, statusCode int) {
	if h.usageStore == nil {
		return
	}
	provider := ""
	if ctx.RouteResult != nil {
		provider = ctx.RouteResult.ProviderKey
	}
	rec := store.UsageRecord{
		Provider: provider,
		Model:    ctx.ClientModel,
		Date:     store.Today(),
		Requests: 1,
	}
	if statusCode >= 400 {
		rec.ErrorRequests = 1
	}
	h.usageStore.AsyncRecord(rec)
}

// writeProxyFormatMismatch reports a proxy-mode config error: the routed
// provider's format does not match the client protocol, and conversion is
// disabled.
func writeProxyFormatMismatch(c *gin.Context, clientProtocol, providerKey, providerFormat string) {
	msg := fmt.Sprintf(
		"proxy mode requires same-format upstream: provider %q has format %q, client protocol is %q; protocol conversion is disabled",
		providerKey, providerFormat, clientProtocol,
	)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{"code": "proxy_mode_format_mismatch", "message": msg},
	})
}
