package handler

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/hook"
	"github.com/keepmind9/ai-switch/internal/middleware"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/store"
	"github.com/keepmind9/ai-switch/internal/types"
)

//go:embed all:static
var staticFS embed.FS

const ctxProviderKey = "provider_key"

const (
	maxLogReqBodyLen  = 16384
	maxLogRespBodyLen = 4096
	maxStreamLogLen   = 512
	upstreamTimeout   = 30 * time.Second // connection + first-byte timeout for upstream requests
)

type Handler struct {
	provider   *config.Provider
	converter  *converter.Converter
	client     *http.Client
	usageStore *store.UsageStore
	router     router.Router
	llmLogger  *slog.Logger
	hooks      *hook.Manager
}

func NewHandler(provider *config.Provider, usageStore *store.UsageStore, r router.Router, llmLogger *slog.Logger) *Handler {
	return &Handler{
		provider:  provider,
		converter: converter.NewConverter(),
		client: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: upstreamTimeout,
			},
		},
		usageStore: usageStore,
		router:     r,
		llmLogger:  llmLogger,
		hooks:      hook.NewManager(),
	}
}

// RegisterHook adds a lifecycle hook to the pipeline.
func (h *Handler) RegisterHook(hk hook.Hook) {
	h.hooks.Register(hk)
}

// RegisterRoutes registers all API endpoints.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.POST("/v1/responses", h.handleResponses)
	r.POST("/v1/messages", h.handleAnthropic)
	r.POST("/v1/messages/count_tokens", h.handleCountTokens)
	r.POST("/v1/chat/completions", h.handleChat)
	r.POST("/api/reload", h.handleReload)
	r.GET("/api/status", h.handleAPIStatus)

	if h.usageStore != nil {
		r.GET("/api/stats", h.handleStats)
	}

	// Serve UI (localhost only)
	staticSub, _ := fs.Sub(staticFS, "static")
	serveUI := func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil || !ip.IsLoopback() {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access restricted to localhost"})
			return
		}
		data, err := fs.ReadFile(staticSub, "index.html")
		if err != nil {
			c.String(http.StatusOK, "Frontend not built. Run `make build-all` to build the admin UI.")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}
	r.GET("/favicon.svg", func(c *gin.Context) {
		c.FileFromFS("/favicon.svg", http.FS(staticSub))
	})
	r.GET("/ui", serveUI)
	r.GET("/ui/favicon.svg", func(c *gin.Context) {
		c.FileFromFS("/favicon.svg", http.FS(staticSub))
	})
	r.GET("/ui/assets/*filepath", func(c *gin.Context) {
		ip := net.ParseIP(c.ClientIP())
		if ip == nil || !ip.IsLoopback() {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access restricted to localhost"})
			return
		}
		c.FileFromFS(c.Request.URL.Path[len("/ui"):], http.FS(staticSub))
	})
	// SPA catch-all: any /ui/* path that doesn't match above returns index.html
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/ui") {
			serveUI(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.HEAD("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}

// handleReload reloads configuration from disk.
func (h *Handler) handleReload(c *gin.Context) {
	if err := h.provider.Reload(); err != nil {
		slog.Error("failed to reload config", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "reload_error", "message": err.Error()}})
		return
	}
	slog.Info("config reloaded via API")
	c.JSON(http.StatusOK, gin.H{"status": "reloaded"})
}

// extractClientAPIKey extracts the API key from client request headers.
func extractClientAPIKey(c *gin.Context) string {
	if key := c.GetHeader("x-api-key"); key != "" {
		return key
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func buildUpstreamURL(result *router.RouteResult) string {
	return router.BuildUpstreamURL(result.BaseURL, result.Path)
}

// normalizeInputRoles replaces unsupported roles in input/messages
// and filters out built-in tools that lack name/parameters.
func normalizeInputRoles(raw map[string]any) {
	// Normalize roles in Responses API input array
	if input, ok := raw["input"].([]any); ok {
		for _, item := range input {
			if m, ok := item.(map[string]any); ok {
				if role, _ := m["role"].(string); role != "" {
					m["role"] = converter.NormalizeRole(role)
				}
			}
		}
	}

	// Normalize roles in Anthropic messages array
	if messages, ok := raw["messages"].([]any); ok {
		for _, item := range messages {
			if m, ok := item.(map[string]any); ok {
				if role, _ := m["role"].(string); role != "" {
					m["role"] = converter.NormalizeRole(role)
				}
			}
		}
	}

	// Filter out built-in tools without a name
	if tools, ok := raw["tools"].([]any); ok {
		var filtered []any
		for _, t := range tools {
			if m, ok := t.(map[string]any); ok {
				if name, _ := m["name"].(string); name != "" {
					filtered = append(filtered, m)
				}
			}
		}
		if len(filtered) != len(tools) {
			raw["tools"] = filtered
		}
	}
}

// forwardRequest sends a request to the upstream API and returns the response with latency.
func (h *Handler) forwardRequest(result *router.RouteResult, body []byte) (*http.Response, time.Duration, error) {
	upstreamURL := buildUpstreamURL(result)
	req, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")

	switch result.Format {
	case converter.FormatAnthropic:
		req.Header.Set("x-api-key", result.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+result.APIKey)
	}

	start := time.Now()
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	return resp, time.Since(start), nil
}

// copyUpstreamHeaders forwards upstream response headers to the client,
// preserving headers we explicitly override (Content-Type, etc.).
func copyUpstreamHeaders(c *gin.Context, resp *http.Response) {
	skip := map[string]bool{
		"Content-Type":      true,
		"Content-Length":    true,
		"Transfer-Encoding": true,
		"Cache-Control":     true,
		"Connection":        true,
	}
	for k, vv := range resp.Header {
		if skip[k] {
			continue
		}
		for _, v := range vv {
			c.Header(k, v)
		}
	}
}

// checkUpstreamStreamError checks if upstream response should be treated as error
// before starting SSE stream. Handles two cases:
//  1. Non-SSE response (JSON instead of expected SSE)
//  2. SSE response where the first event is an error
//
// Returns (body, true) if error handled, ("", false) if stream is safe to start.
func checkUpstreamStreamError(c *gin.Context, resp *http.Response, clientFormat string) (string, bool) {
	// Case 1: non-SSE response
	if !isSSEResponse(resp) {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Warn("upstream returned non-SSE response in streaming path",
			"content_type", resp.Header.Get("Content-Type"), "body_len", len(respBody))
		msg, errType := parseUpstreamError(respBody)
		writeStreamErrorJSON(c, resp.StatusCode, msg, errType, clientFormat)
		return string(respBody), true
	}

	// Case 2: peek at first SSE event — only for non-Anthropic clients.
	// Anthropic clients (Claude Code) handle SSE error events natively.
	if clientFormat == converter.FormatAnthropic {
		return "", false
	}

	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, &buf)
	scanner := bufio.NewScanner(tee)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		data := converter.ParseSSEDataLine(scanner.Text())
		if data == "" {
			continue
		}
		if isSSEErrorData(data) {
			msg, errType := parseUpstreamError([]byte(data))
			slog.Warn("SSE error in first upstream event", "message", msg, "type", errType, "client_format", clientFormat)
			writeStreamErrorJSON(c, errorTypeToStatus(errType), msg, errType, clientFormat)
			return buf.String(), true
		}
		break
	}

	// Not an error — restore peeked data so the streaming function can read it
	resp.Body = io.NopCloser(io.MultiReader(&buf, resp.Body))
	return "", false
}

func writeSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
}

// streamChatToClient reads Chat SSE from upstream and converts to the target
// client format using the provided converter function. Returns accumulated upstream content.
// clientFormat is used to format error events in the client's expected format.
func (h *Handler) streamChatToClient(c *gin.Context, resp *http.Response, convertFn func(w converter.SSEWriter, data string) bool, clientFormat string) string {
	copyUpstreamHeaders(c, resp)

	if body, handled := checkUpstreamStreamError(c, resp, clientFormat); handled {
		return body
	}

	writeSSEHeaders(c)

	ginWriter := &ginSSEWriter{c: c}
	flusher, canFlush := c.Writer.(http.Flusher)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buf bytes.Buffer
	done := false
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")
		data := converter.ParseSSEDataLine(line)
		if data == "" {
			continue
		}
		if isSSEErrorData(data) {
			msg, errType := parseUpstreamError([]byte(data))
			slog.Warn("SSE error event from upstream", "message", msg, "type", errType)
			writeSSEErrorToClient(ginWriter, msg, errType, clientFormat)
			break
		}
		if convertFn(ginWriter, data) {
			done = true
			break
		}
		if canFlush {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("SSE scanner error", "error", err)
	}
	if !done {
		convertFn(ginWriter, "[DONE]")
	}
	if canFlush {
		flusher.Flush()
	}
	return buf.String()
}

// logLLMRequest writes a structured LLM log entry with request/response details.
func (h *Handler) logLLMRequest(model, provider, url string, latency time.Duration, stream bool, reqBody []byte, respBody string) {
	if h.llmLogger == nil {
		return
	}
	maxResp := maxLogRespBodyLen
	if stream {
		maxResp = maxStreamLogLen
	}
	h.llmLogger.Info("llm request",
		slog.String("model", model),
		slog.String("provider", provider),
		slog.String("url", url),
		slog.Int64("latency_ms", latency.Milliseconds()),
		slog.Bool("stream", stream),
		slog.String("request", truncateString(string(reqBody), maxLogReqBodyLen)),
		slog.String("response", truncateString(respBody, maxResp)),
	)
}

// recordStreamUsage records token usage for a streaming response.
func (h *Handler) recordStreamUsage(model, provider string, inputTokens, outputTokens int) {
	if h.usageStore == nil {
		return
	}
	if inputTokens == 0 && outputTokens == 0 {
		slog.Debug("stream usage skipped: zero tokens", "provider", provider, "model", model)
		return
	}
	slog.Debug("recorded stream usage", "provider", provider, "model", model,
		"input", inputTokens, "output", outputTokens)
	h.usageStore.AsyncRecord(store.UsageRecord{
		Provider:     provider,
		Model:        model,
		Date:         store.Today(),
		Requests:     1,
		InputTokens:  int64(inputTokens),
		OutputTokens: int64(outputTokens),
		TotalTokens:  int64(inputTokens + outputTokens),
	})
}

// recordStreamUsageFromState extracts provider from context and records usage.
func (h *Handler) recordStreamUsageFromState(c *gin.Context, model string, inputTokens, outputTokens int) {
	provider, _ := c.Get(ctxProviderKey)
	providerStr, _ := provider.(string)
	h.recordStreamUsage(model, providerStr, inputTokens, outputTokens)
}

// streamPassthrough forwards upstream SSE directly to the client. Returns accumulated upstream content.
func (h *Handler) streamPassthrough(c *gin.Context, resp *http.Response, format string) string {
	copyUpstreamHeaders(c, resp)

	// Upstream may return SSE data without Content-Type header.
	if !isSSEResponse(resp) && resp.StatusCode == http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if looksLikeSSE(respBody) {
			slog.Info("upstream SSE without Content-Type, streaming directly", "body_len", len(respBody))
			return h.streamBodyAsSSE(c, bytes.NewReader(respBody), format)
		} else if !isSSEErrorData(string(respBody)) && format == converter.FormatResponses {
			slog.Info("converting non-SSE Responses JSON to SSE events", "body_len", len(respBody))
			return h.convertResponsesJSONToSSE(c, respBody)
		}
		// Error response — restore body for checkUpstreamStreamError
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
	}

	if body, handled := checkUpstreamStreamError(c, resp, format); handled {
		return body
	}

	return h.streamBodyAsSSE(c, resp.Body, format)
}

// streamBodyAsSSE reads SSE from body and writes it to the client.
func (h *Handler) streamBodyAsSSE(c *gin.Context, body io.Reader, format string) string {
	writeSSEHeaders(c)

	var acc middleware.StreamUsageAccumulator
	provider, _ := c.Get(ctxProviderKey)
	providerStr, _ := provider.(string)

	flusher, canFlush := c.Writer.(http.Flusher)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")
		c.Writer.WriteString(line + "\n")

		data := converter.ParseSSEDataLine(line)
		acc.Sniff(data, format)

		if canFlush {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("SSE scanner error", "error", err)
	}
	h.recordStreamUsage(acc.Model, providerStr, int(acc.InputTokens), int(acc.OutputTokens))
	return buf.String()
}

// convertResponsesJSONToSSE converts a non-streaming Responses API JSON response
// into Responses API SSE events and writes them to the client.
func (h *Handler) convertResponsesJSONToSSE(c *gin.Context, body []byte) string {
	writeSSEHeaders(c)
	w := &ginSSEWriter{c: c}
	flusher, canFlush := c.Writer.(http.Flusher)

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		slog.Error("failed to parse Responses JSON for SSE conversion", "error", err)
		return string(body)
	}

	seq := 0
	respID, _ := resp["id"].(string)
	model, _ := resp["model"].(string)
	createdAt := resp["created_at"]

	w.WriteEvent("response.created", map[string]any{
		"type": "response.created", "sequence_number": seq,
		"response": map[string]any{
			"id": respID, "object": "response", "created_at": createdAt,
			"model": model, "status": "in_progress", "output": []any{},
		},
	})
	seq++

	output, _ := resp["output"].([]any)
	for i, item := range output {
		itemMap, _ := item.(map[string]any)
		itemID, _ := itemMap["id"].(string)
		itemType, _ := itemMap["type"].(string)

		w.WriteEvent("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "sequence_number": seq, "output_index": i,
			"item": item,
		})
		seq++

		if itemType == "message" {
			content, _ := itemMap["content"].([]any)
			for j, part := range content {
				partMap, _ := part.(map[string]any)
				partType, _ := partMap["type"].(string)

				w.WriteEvent("response.content_part.added", map[string]any{
					"type": "response.content_part.added", "sequence_number": seq,
					"output_index": i, "content_index": j, "item_id": itemID, "part": part,
				})
				seq++

				if partType == "output_text" {
					if text, _ := partMap["text"].(string); text != "" {
						w.WriteEvent("response.output_text.delta", map[string]any{
							"type": "response.output_text.delta", "sequence_number": seq,
							"output_index": i, "content_index": j, "item_id": itemID, "delta": text,
						})
						seq++
						w.WriteEvent("response.output_text.done", map[string]any{
							"type": "response.output_text.done", "sequence_number": seq,
							"output_index": i, "content_index": j, "item_id": itemID, "text": text,
						})
						seq++
					}
				}

				w.WriteEvent("response.content_part.done", map[string]any{
					"type": "response.content_part.done", "sequence_number": seq,
					"output_index": i, "content_index": j, "item_id": itemID, "part": part,
				})
				seq++
			}
		}

		w.WriteEvent("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "sequence_number": seq, "output_index": i,
			"item": item,
		})
		seq++
	}

	resp["status"] = "completed"
	w.WriteEvent("response.completed", map[string]any{
		"type": "response.completed", "sequence_number": seq, "response": resp,
	})

	if canFlush {
		flusher.Flush()
	}
	return string(body)
}

// streamToChatSSE reads upstream SSE (any format) and converts to Chat SSE output.
// convertFn returns a *types.ChatStreamResponse, "[DONE]" string, or nil.
// Returns accumulated upstream content.
func (h *Handler) streamToChatSSE(c *gin.Context, resp *http.Response, convertFn func(state any, line string) any, initialState any) string {
	copyUpstreamHeaders(c, resp)

	if body, handled := checkUpstreamStreamError(c, resp, converter.FormatChat); handled {
		return body
	}

	writeSSEHeaders(c)

	flusher, canFlush := c.Writer.(http.Flusher)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")

		data := converter.ParseSSEDataLine(line)
		if data != "" && isSSEErrorData(data) {
			msg, errType := parseUpstreamError([]byte(data))
			slog.Warn("SSE error event from upstream", "message", msg, "type", errType)
			errData, _ := json.Marshal(map[string]any{
				"error": map[string]any{"message": msg, "type": errType},
			})
			c.Writer.WriteString("data: " + string(errData) + "\n\n")
			if canFlush {
				flusher.Flush()
			}
			break
		}

		result := convertFn(initialState, line)

		switch v := result.(type) {
		case *types.ChatStreamResponse:
			data, _ := json.Marshal(v)
			c.Writer.WriteString("data: " + string(data) + "\n\n")
		case string:
			if v == "[DONE]" {
				// Emit final usage chunk if state provides usage data
				if up, ok := initialState.(interface {
					ChatStreamUsage() (string, string, int, int)
				}); ok {
					id, model, in, out := up.ChatStreamUsage()
					if in > 0 || out > 0 {
						usageChunk := &types.ChatStreamResponse{
							ID:      id,
							Object:  "chat.completion.chunk",
							Model:   model,
							Choices: []types.StreamChoice{},
							Usage: &types.ChatUsage{
								PromptTokens:     in,
								CompletionTokens: out,
								TotalTokens:      in + out,
							},
						}
						data, _ := json.Marshal(usageChunk)
						c.Writer.WriteString("data: " + string(data) + "\n\n")
					}
				}
				c.Writer.WriteString("data: [DONE]\n\n")
				if canFlush {
					flusher.Flush()
				}
				return buf.String()
			}
		}

		if canFlush {
			flusher.Flush()
		}
	}
	// Upstream stream ended without [DONE] sentinel (e.g. Responses format).
	c.Writer.WriteString("data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
		c.Writer.WriteString("data: [DONE]\n\n")
		flusher.Flush()
	}
	return buf.String()
}

// streamAnthropicToResponsesSSE reads Anthropic SSE from resp and writes Responses API SSE to client.
// Returns accumulated upstream content.
func (h *Handler) streamAnthropicToResponsesSSE(c *gin.Context, resp *http.Response, state *converter.AnthropicToResponsesState) string {
	copyUpstreamHeaders(c, resp)

	if body, handled := checkUpstreamStreamError(c, resp, converter.FormatResponses); handled {
		return body
	}

	writeSSEHeaders(c)

	w := &ginSSEWriter{c: c}
	flusher, canFlush := c.Writer.(http.Flusher)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line + "\n")

		data := converter.ParseSSEDataLine(line)
		if data != "" && isSSEErrorData(data) {
			msg, errType := parseUpstreamError([]byte(data))
			slog.Warn("SSE error event from upstream", "message", msg, "type", errType)
			writeSSEErrorToClient(w, msg, errType, converter.FormatResponses)
			if canFlush {
				flusher.Flush()
			}
			return buf.String()
		}

		if converter.ConvertAnthropicLineToResponses(w, state, line) {
			if canFlush {
				flusher.Flush()
			}
			return buf.String()
		}
		if canFlush {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("SSE scanner error", "error", err)
	}
	// Upstream closed without sending message_delta/message_stop — emit response.completed as fallback.
	converter.EmitCompleted(w, state)
	if canFlush {
		flusher.Flush()
	}
	return buf.String()
}

// handleResponses handles /v1/responses endpoint (Codex CLI, OpenAI Responses API).
func (h *Handler) handleResponses(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to read request body"}})
		return
	}
	h.executePipeline(c, converter.FormatResponses, body)
}

// handleAnthropic handles /v1/messages endpoint (Claude Code, Anthropic Messages API).
// handleCountTokens implements POST /v1/messages/count_tokens.
// It counts tokens for an Anthropic-format request body without calling the model.
func (h *Handler) handleCountTokens(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "failed to read request body"}})
		return
	}
	count := router.CountTokens(body)
	c.JSON(http.StatusOK, gin.H{"input_tokens": count})
}

func (h *Handler) handleAnthropic(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "failed to read request body"}})
		return
	}
	h.executePipeline(c, converter.FormatAnthropic, body)
}

// handleChat handles /v1/chat/completions endpoint (Chat Completions passthrough).
func (h *Handler) handleChat(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to read request body"}})
		return
	}
	h.executePipeline(c, converter.FormatChat, body)
}

// Upstream API paths. BuildUpstreamURL handles /v1 deduplication
// ginSSEWriter adapts gin.Context to the SSEWriter interface.
type ginSSEWriter struct {
	c *gin.Context
}

func (g *ginSSEWriter) WriteEvent(eventType string, data any) {
	jsonData, _ := json.Marshal(data)
	g.c.Writer.WriteString(converter.FormatSSEEvent(eventType, jsonData))
	g.c.Writer.Flush()
}

// handleStats returns usage statistics.
func (h *Handler) handleStats(c *gin.Context) {
	provider := c.Query("provider")
	model := c.Query("model")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	records, err := h.usageStore.QueryUsage(provider, model, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "query_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": records})
}

// handleAPIStatus returns current configuration status.
func (h *Handler) handleAPIStatus(c *gin.Context) {
	cfg := h.provider.Get()

	status := gin.H{
		"server": gin.H{
			"host": cfg.Server.Host,
			"port": cfg.Server.Port,
		},
		"default_route":           cfg.DefaultRoute,
		"default_anthropic_route": cfg.DefaultAnthropicRoute,
		"default_responses_route": cfg.DefaultResponsesRoute,
		"default_chat_route":      cfg.DefaultChatRoute,
	}

	providers := make([]gin.H, 0)
	for key, p := range cfg.Providers {
		providers = append(providers, gin.H{
			"key":      key,
			"name":     p.Name,
			"base_url": p.BaseURL,
			"format":   p.Format,
			"sponsor":  p.Sponsor,
			"logo_url": p.LogoURL,
		})
	}
	status["providers"] = providers

	c.JSON(http.StatusOK, status)
}

func truncateString(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "...(truncated)"
}
