package handler

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/keepmind9/llm-gateway/internal/converter"
	"github.com/keepmind9/llm-gateway/internal/router"
	"github.com/keepmind9/llm-gateway/internal/store"
	"github.com/keepmind9/llm-gateway/internal/types"
)

//go:embed all:static
var staticFS embed.FS

type Handler struct {
	provider   *config.Provider
	converter  *converter.Converter
	client     *http.Client
	usageStore *store.UsageStore
	router     router.Router
}

func NewHandler(provider *config.Provider, usageStore *store.UsageStore, r router.Router) *Handler {
	return &Handler{
		provider:   provider,
		converter:  converter.NewConverter(),
		client:     &http.Client{},
		usageStore: usageStore,
		router:     r,
	}
}

// RegisterRoutes registers all API endpoints.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.POST("/v1/responses", h.handleResponses)
	r.POST("/v1/messages", h.handleAnthropic)
	r.POST("/v1/chat/completions", h.handleChat)
	r.POST("/api/reload", h.handleReload)
	r.GET("/api/status", h.handleAPIStatus)

	if h.usageStore != nil {
		r.GET("/api/stats", h.handleStats)
	}

	// Serve UI
	staticSub, _ := fs.Sub(staticFS, "static")
	r.GET("/ui", func(c *gin.Context) {
		data, _ := fs.ReadFile(staticSub, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
	r.StaticFS("/ui/assets", http.FS(staticSub))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
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

// forwardRequest sends a request to the upstream API and returns the response.
func (h *Handler) forwardRequest(result *router.RouteResult, path string, body []byte) (*http.Response, error) {
	if result.Path != "" {
		path = result.Path
	}
	upstreamURL := strings.TrimSuffix(result.BaseURL, "/") + path
	req, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	switch result.Format {
	case "anthropic":
		req.Header.Set("x-api-key", result.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+result.APIKey)
	}

	return h.client.Do(req)
}

// writeUpstreamError forwards an upstream error response to the client.
func (h *Handler) writeUpstreamError(c *gin.Context, resp *http.Response) {
	respBody, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", respBody)
}

// streamChatToClient reads Chat SSE from upstream and converts to the target
// client format using the provided converter function.
func (h *Handler) streamChatToClient(c *gin.Context, resp *http.Response, convertFn func(w converter.SSEWriter, data string) bool) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	ginWriter := &ginSSEWriter{c: c}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	done := false
	for scanner.Scan() {
		line := scanner.Text()
		data := converter.ParseSSEDataLine(line)
		if data == "" {
			continue
		}
		if convertFn(ginWriter, data) {
			done = true
			break
		}
	}
	if !done {
		convertFn(ginWriter, "[DONE]")
	}
}

// streamPassthrough forwards upstream SSE directly to the client.
func (h *Handler) streamPassthrough(c *gin.Context, resp *http.Response) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, canFlush := c.Writer.(http.Flusher)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		c.Writer.WriteString(line + "\n")
		if canFlush {
			flusher.Flush()
		}
	}
}

// streamToChatSSE reads upstream SSE (any format) and converts to Chat SSE output.
// convertFn returns a *types.ChatStreamResponse, "[DONE]" string, or nil.
func (h *Handler) streamToChatSSE(c *gin.Context, resp *http.Response, convertFn func(state any, line string) any, initialState any) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, canFlush := c.Writer.(http.Flusher)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		result := convertFn(initialState, line)

		switch v := result.(type) {
		case *types.ChatStreamResponse:
			data, _ := json.Marshal(v)
			c.Writer.WriteString("data: " + string(data) + "\n\n")
		case string:
			if v == "[DONE]" {
				c.Writer.WriteString("data: [DONE]\n\n")
				if canFlush {
					flusher.Flush()
				}
				return
			}
		}

		if canFlush {
			flusher.Flush()
		}
	}
}

// handleResponses handles /v1/responses endpoint (Codex CLI, OpenAI Responses API).
func (h *Handler) handleResponses(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to read request body"}})
		return
	}

	var responsesReq types.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to parse request: " + err.Error()}})
		return
	}

	apiKey := extractClientAPIKey(c)
	result, routeErr := h.router.Route("responses", apiKey, body)
	if routeErr != nil {
		slog.Error("route error", "error", routeErr, "api_key", apiKey)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "route_error", "message": routeErr.Error()}})
		return
	}

	slog.Info("responses request", "model", responsesReq.Model, "stream", responsesReq.Stream, "upstream_format", result.Format, "upstream_base", result.BaseURL, "resolved_model", result.Model)

	model := responsesReq.Model
	if model == "" {
		model = result.Model
	}

	isStreaming := responsesReq.Stream

	switch result.Format {
	case "chat", "":
		h.responsesViaChat(c, result, &responsesReq, model, isStreaming)
	case "responses":
		h.passthroughRequest(c, result, body, isStreaming)
	case "anthropic":
		h.responsesViaChatToAnthropic(c, result, &responsesReq, model, isStreaming)
	}
}

// handleAnthropic handles /v1/messages endpoint (Claude Code, Anthropic Messages API).
func (h *Handler) handleAnthropic(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to read request body"}})
		return
	}

	var anthReq converter.AnthropicRequest
	if err := json.Unmarshal(body, &anthReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to parse request: " + err.Error()}})
		return
	}

	apiKey := extractClientAPIKey(c)
	result, routeErr := h.router.Route("anthropic", apiKey, body)
	if routeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "route_error", "message": routeErr.Error()}})
		return
	}

	model := anthReq.Model
	if model == "" {
		model = result.Model
	}
	anthReq.Model = model

	isStreaming := anthReq.Stream

	switch result.Format {
	case "chat", "":
		h.anthropicViaChat(c, result, &anthReq, model, isStreaming)
	case "anthropic":
		h.passthroughRequest(c, result, body, isStreaming)
	case "responses":
		h.anthropicViaChatToResponses(c, result, &anthReq, model, isStreaming)
	}
}

// handleChat handles /v1/chat/completions endpoint (Chat Completions passthrough).
func (h *Handler) handleChat(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to read request body"}})
		return
	}

	var chatReq types.ChatRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "failed to parse request: " + err.Error()}})
		return
	}

	apiKey := extractClientAPIKey(c)
	result, routeErr := h.router.Route("chat", apiKey, body)
	if routeErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "route_error", "message": routeErr.Error()}})
		return
	}

	if chatReq.Model == "" {
		chatReq.Model = result.Model
	}

	isStreaming := chatReq.Stream

	switch result.Format {
	case "chat", "":
		h.passthroughRequest(c, result, body, isStreaming)
	case "anthropic":
		h.chatViaAnthropic(c, result, &chatReq, isStreaming)
	case "responses":
		h.chatViaResponses(c, result, &chatReq, isStreaming)
	}
}

// --- Routing implementations ---

// passthroughRequest forwards the request body directly to upstream.
func (h *Handler) passthroughRequest(c *gin.Context, result *router.RouteResult, body []byte, isStreaming bool) {
	var raw map[string]any
	if json.Unmarshal(body, &raw) == nil && result.Model != "" {
		raw["model"] = result.Model
		body, _ = json.Marshal(raw)
	}

	resp, err := h.forwardRequest(result, formatToPath(result.Format), body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": "failed to call upstream: " + err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}

	if isStreaming {
		h.streamPassthrough(c, resp)
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	c.Data(http.StatusOK, "application/json", respBody)
}

// responsesViaChat converts Responses→Chat, forwards to upstream, converts back.
func (h *Handler) responsesViaChat(c *gin.Context, result *router.RouteResult, req *types.ResponsesRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.ResponsesToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	chatReq.Model = result.Model
	chatReq.Stream = isStreaming

	chatBody, _ := json.Marshal(chatReq)
	resp, err := h.forwardRequest(result, PathChat, chatBody)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}

	if isStreaming {
		state := &converter.ResponsesStreamState{Created: time.Now().Unix(), Model: model}
		h.streamChatToClient(c, resp, func(w converter.SSEWriter, data string) bool {
			return converter.ConvertChatChunkToResponsesSSE(w, state, data)
		})
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	var chatResp types.ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": "failed to parse chat response"}})
		return
	}

	responsesResp, err := h.converter.ChatToResponses(&chatResp, model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, responsesResp)
}

// anthropicViaChat converts Anthropic→Chat, forwards, converts back.
func (h *Handler) anthropicViaChat(c *gin.Context, result *router.RouteResult, req *converter.AnthropicRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.AnthropicToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	chatReq.Model = result.Model
	chatReq.Stream = isStreaming

	chatBody, _ := json.Marshal(chatReq)
	resp, err := h.forwardRequest(result, PathChat, chatBody)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}

	if isStreaming {
		state := &converter.AnthropicStreamState{Model: model}
		h.streamChatToClient(c, resp, func(w converter.SSEWriter, data string) bool {
			return converter.ConvertChatChunkToAnthropicSSE(w, state, data)
		})
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	var chatResp types.ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": "failed to parse chat response"}})
		return
	}

	anthResp, err := h.converter.ChatToAnthropic(&chatResp, model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, anthResp)
}

// chatViaAnthropic converts Chat→Anthropic, forwards, converts back.
func (h *Handler) chatViaAnthropic(c *gin.Context, result *router.RouteResult, chatReq *types.ChatRequest, isStreaming bool) {
	anthReq, err := h.converter.ChatRequestToAnthropic(chatReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	anthReq.Stream = isStreaming
	anthBody, _ := json.Marshal(anthReq)

	resp, err := h.forwardRequest(result, PathMessages, anthBody)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}

	if isStreaming {
		state := &converter.AnthropicToChatState{}
		h.streamToChatSSE(c, resp, func(s any, line string) any {
			return converter.ConvertAnthropicLineToChat(s.(*converter.AnthropicToChatState), line)
		}, state)
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	var anthResp converter.AnthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": "failed to parse anthropic response"}})
		return
	}

	chatResp, err := h.converter.AnthropicResponseToChat(&anthResp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, chatResp)
}

// chatViaResponses converts Chat→Responses, forwards, converts back.
func (h *Handler) chatViaResponses(c *gin.Context, result *router.RouteResult, chatReq *types.ChatRequest, isStreaming bool) {
	respReq := converter.BuildResponsesFromChat(chatReq, isStreaming)
	respReq.Model = result.Model

	respBody, _ := json.Marshal(respReq)
	resp, err := h.forwardRequest(result, PathResponses, respBody)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}

	if isStreaming {
		state := &converter.ResponsesToChatState{}
		h.streamToChatSSE(c, resp, func(s any, line string) any {
			return converter.ConvertResponsesLineToChat(s.(*converter.ResponsesToChatState), line)
		}, state)
		return
	}

	respData, _ := io.ReadAll(resp.Body)
	c.Data(http.StatusOK, "application/json", respData)
}

// responsesViaChatToAnthropic chains Responses→Chat→Anthropic.
func (h *Handler) responsesViaChatToAnthropic(c *gin.Context, result *router.RouteResult, req *types.ResponsesRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.ResponsesToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}
	chatReq.Model = model
	chatReq.Stream = isStreaming
	h.chatViaAnthropic(c, result, chatReq, isStreaming)
}

// anthropicViaChatToResponses chains Anthropic→Chat→Responses.
func (h *Handler) anthropicViaChatToResponses(c *gin.Context, result *router.RouteResult, req *converter.AnthropicRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.AnthropicToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}
	chatReq.Model = model
	chatReq.Stream = isStreaming

	chatBody, _ := json.Marshal(chatReq)
	resp, err := h.forwardRequest(result, PathResponses, chatBody)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}

	if isStreaming {
		state := &converter.ResponsesToChatState{}
		h.streamToChatSSE(c, resp, func(s any, line string) any {
			return converter.ConvertResponsesLineToChat(s.(*converter.ResponsesToChatState), line)
		}, state)
		return
	}

	respBody, _ := io.ReadAll(resp.Body)
	c.Data(http.StatusOK, "application/json", respBody)
}

// Upstream API paths. Convention: all paths include /v1 prefix.
// base_url should NOT include /v1 — the gateway appends these paths.
const (
	PathChat       = "/v1/chat/completions"
	PathMessages   = "/v1/messages"
	PathResponses  = "/v1/responses"
)

// formatToPath returns the upstream API path based on format.
func formatToPath(format string) string {
	switch format {
	case "anthropic":
		return PathMessages
	case "responses":
		return PathResponses
	default:
		return PathChat
	}
}

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
		"default_provider": cfg.DefaultProvider,
	}

	providers := make([]gin.H, 0)
	for key, p := range cfg.Providers {
		providers = append(providers, gin.H{
			"key":       key,
			"name":      p.Name,
			"base_url":  p.BaseURL,
			"model":     p.Model,
			"format":    p.Format,
			"sponsor":   p.Sponsor,
			"logo_url":  p.LogoURL,
		})
	}
	status["providers"] = providers

	c.JSON(http.StatusOK, status)
}
