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
}

func NewHandler(provider *config.Provider, usageStore *store.UsageStore) *Handler {
	return &Handler{
		provider:   provider,
		converter:  converter.NewConverter(),
		client:     &http.Client{},
		usageStore: usageStore,
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

// forwardRequest sends a request to the upstream API and returns the response.
func (h *Handler) forwardRequest(cfg *config.Config, path string, body []byte) (*http.Response, error) {
	upstreamURL := strings.TrimSuffix(cfg.Upstream.BaseURL, "/") + path
	req, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	switch cfg.Upstream.Format {
	case "anthropic":
		req.Header.Set("x-api-key", cfg.Upstream.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+cfg.Upstream.APIKey)
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

	for scanner.Scan() {
		line := scanner.Text()
		data := converter.ParseSSEDataLine(line)
		if data == "" {
			continue
		}
		if done := convertFn(ginWriter, data); done {
			break
		}
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

// handleResponses handles /v1/responses endpoint (Codex CLI, OpenAI Responses API).
func (h *Handler) handleResponses(c *gin.Context) {
	cfg := h.provider.Get()
	upstreamFormat := cfg.Upstream.Format

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

	model := responsesReq.Model
	if model == "" {
		model = cfg.Upstream.Model
	}

	isStreaming := responsesReq.Stream

	switch upstreamFormat {
	case "chat", "":
		h.responsesViaChat(c, cfg, &responsesReq, model, isStreaming)
	case "responses":
		h.passthroughRequest(c, cfg, body, isStreaming)
	case "anthropic":
		// Responses → Chat → Anthropic (chain through hub)
		h.responsesViaChatToAnthropic(c, cfg, &responsesReq, model, isStreaming)
	}
}

// handleAnthropic handles /v1/messages endpoint (Claude Code, Anthropic Messages API).
func (h *Handler) handleAnthropic(c *gin.Context) {
	cfg := h.provider.Get()
	upstreamFormat := cfg.Upstream.Format

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

	model := anthReq.Model
	if model == "" {
		model = cfg.Upstream.Model
	}
	anthReq.Model = model

	isStreaming := anthReq.Stream

	switch upstreamFormat {
	case "chat", "":
		h.anthropicViaChat(c, cfg, &anthReq, model, isStreaming)
	case "anthropic":
		h.passthroughRequest(c, cfg, body, isStreaming)
	case "responses":
		// Anthropic → Chat → Responses (chain through hub)
		h.anthropicViaChatToResponses(c, cfg, &anthReq, model, isStreaming)
	}
}

// handleChat handles /v1/chat/completions endpoint (Chat Completions passthrough).
func (h *Handler) handleChat(c *gin.Context) {
	cfg := h.provider.Get()
	upstreamFormat := cfg.Upstream.Format

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

	if chatReq.Model == "" {
		chatReq.Model = cfg.Upstream.Model
	}
	chatReq.Model = cfg.Upstream.ResolveModel(chatReq.Model)

	isStreaming := chatReq.Stream

	switch upstreamFormat {
	case "chat", "":
		h.passthroughRequest(c, cfg, body, isStreaming)
	case "anthropic":
		h.chatViaAnthropic(c, cfg, &chatReq, isStreaming)
	case "responses":
		h.passthroughRequest(c, cfg, body, isStreaming)
	}
}

// --- Routing implementations ---

// passthroughRequest forwards the request body directly to upstream.
func (h *Handler) passthroughRequest(c *gin.Context, cfg *config.Config, body []byte, isStreaming bool) {
	var parsed struct {
		Model string `json:"model"`
	}
	json.Unmarshal(body, &parsed)
	if parsed.Model != "" {
		newModel := cfg.Upstream.ResolveModel(parsed.Model)
		if newModel != parsed.Model {
			var raw map[string]any
			json.Unmarshal(body, &raw)
			raw["model"] = newModel
			body, _ = json.Marshal(raw)
		}
	}

	resp, err := h.forwardRequest(cfg, h.upstreamPath(cfg), body)
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
func (h *Handler) responsesViaChat(c *gin.Context, cfg *config.Config, req *types.ResponsesRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.ResponsesToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	chatReq.Model = cfg.Upstream.ResolveModel(cfg.Upstream.Model)
	chatReq.Stream = isStreaming

	chatBody, _ := json.Marshal(chatReq)
	resp, err := h.forwardRequest(cfg, "/chat/completions", chatBody)
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
func (h *Handler) anthropicViaChat(c *gin.Context, cfg *config.Config, req *converter.AnthropicRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.AnthropicToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	chatReq.Model = cfg.Upstream.ResolveModel(cfg.Upstream.Model)
	chatReq.Stream = isStreaming

	chatBody, _ := json.Marshal(chatReq)
	resp, err := h.forwardRequest(cfg, "/chat/completions", chatBody)
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
func (h *Handler) chatViaAnthropic(c *gin.Context, cfg *config.Config, chatReq *types.ChatRequest, isStreaming bool) {
	anthReq, err := h.converter.ChatRequestToAnthropic(chatReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	anthReq.Stream = isStreaming
	anthBody, _ := json.Marshal(anthReq)

	resp, err := h.forwardRequest(cfg, "/messages", anthBody)
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
		// TODO: implement Anthropic SSE → Chat SSE conversion (Phase 3.3)
		h.streamPassthrough(c, resp)
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

// responsesViaChatToAnthropic chains Responses→Chat→Anthropic.
func (h *Handler) responsesViaChatToAnthropic(c *gin.Context, cfg *config.Config, req *types.ResponsesRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.ResponsesToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}
	chatReq.Model = model
	chatReq.Stream = false // Chain conversion doesn't support streaming yet
	h.chatViaAnthropic(c, cfg, chatReq, isStreaming)
}

// anthropicViaChatToResponses chains Anthropic→Chat→Responses.
func (h *Handler) anthropicViaChatToResponses(c *gin.Context, cfg *config.Config, req *converter.AnthropicRequest, model string, isStreaming bool) {
	chatReq, err := h.converter.AnthropicToChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}
	chatReq.Model = model
	chatReq.Stream = false // Chain conversion doesn't support streaming yet
	// Forward as Chat to upstream Responses format
	chatBody, _ := json.Marshal(chatReq)
	resp, err := h.forwardRequest(cfg, "/responses", chatBody)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": err.Error()}})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		h.writeUpstreamError(c, resp)
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	c.Data(http.StatusOK, "application/json", respBody)
}

// upstreamPath returns the API path based on upstream format.
func (h *Handler) upstreamPath(cfg *config.Config) string {
	switch cfg.Upstream.Format {
	case "anthropic":
		return "/v1/messages"
	case "responses":
		return "/v1/responses"
	default:
		return "/v1/chat/completions"
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
		"upstream": gin.H{
			"base_url": cfg.Upstream.BaseURL,
			"format":   cfg.Upstream.Format,
			"model":    cfg.Upstream.Model,
		},
		"server": gin.H{
			"host": cfg.Server.Host,
			"port": cfg.Server.Port,
		},
	}

	providers := make([]gin.H, 0)
	for key, p := range cfg.Providers {
		providers = append(providers, gin.H{
			"key":      key,
			"name":     p.Name,
			"base_url": p.BaseURL,
			"model":    p.Model,
			"format":   p.Format,
			"sponsor":  p.Sponsor,
			"logo_url": p.LogoURL,
		})
	}
	status["providers"] = providers

	c.JSON(http.StatusOK, status)
}
