package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/keepmind9/llm-gateway/internal/converter"
	"github.com/keepmind9/llm-gateway/internal/types"
)

type Handler struct {
	provider  *config.Provider
	converter *converter.Converter
	client    *http.Client
}

func NewHandler(provider *config.Provider) *Handler {
	return &Handler{
		provider:  provider,
		converter: converter.NewConverter(),
		client:    &http.Client{},
	}
}

func (h *Handler) Responses(c *gin.Context) {
	cfg := h.provider.Get()

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

	chatReq, err := h.converter.ResponsesToChat(&responsesReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": err.Error()}})
		return
	}

	chatReq.Model = cfg.Upstream.ResolveModel(cfg.Upstream.Model)
	chatReq.Stream = isStreaming

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "conversion_error", "message": "failed to marshal chat request"}})
		return
	}

	upstreamURL := strings.TrimSuffix(cfg.Upstream.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", upstreamURL, bytes.NewReader(chatBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "upstream_error", "message": "failed to create upstream request"}})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Upstream.APIKey)

	resp, err := h.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "upstream_error", "message": "failed to call upstream: " + err.Error()}})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	if isStreaming {
		h.handleStreamingResponse(c, resp, model)
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "upstream_error", "message": "failed to read upstream response"}})
		return
	}

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

type StreamState struct {
	ResponseID   string
	Created      int64
	OutputIndex  int
	ContentIndex int
	ItemID       string
	ItemType     string
	CreatedSent  bool
	AccText      string
	SeqNum       int
	Model        string
}

func (h *Handler) handleStreamingResponse(c *gin.Context, resp *http.Response, model string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	state := &StreamState{
		Created: time.Now().Unix(),
		Model:   model,
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			data := line[6:]
			if data == "[DONE]" {
				break
			}

			var chunk types.ChatStreamResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if !state.CreatedSent {
				state.CreatedSent = true
				state.ResponseID = chunk.ID
				h.sendSSE(c, "response.created", map[string]any{
					"type":            "response.created",
					"sequence_number": state.nextSeq(),
					"response": map[string]any{
						"id":         chunk.ID,
						"object":     "response",
						"created_at": state.Created,
						"model":      model,
						"status":     "in_progress",
						"output":     []any{},
						"usage":      nil,
					},
				})
			}

			for _, choice := range chunk.Choices {
				if state.ItemID == "" {
					state.ItemID = fmt.Sprintf("item_%d", time.Now().UnixNano())
					state.ItemType = "message"
					state.OutputIndex = 0
					state.ContentIndex = 0

					h.sendSSE(c, "response.output_item.added", map[string]any{
						"type":            "response.output_item.added",
						"sequence_number": state.nextSeq(),
						"output_index":    0,
						"item": map[string]any{
							"id":      state.ItemID,
							"type":    "message",
							"status":  "in_progress",
							"role":    "assistant",
							"content": []any{},
						},
					})

					h.sendSSE(c, "response.content_part.added", map[string]any{
						"type":            "response.content_part.added",
						"sequence_number": state.nextSeq(),
						"output_index":    0,
						"content_index":   0,
						"item_id":         state.ItemID,
						"part": map[string]any{
							"type": "output_text",
							"text": "",
						},
					})
				}

				if choice.Delta.Content != "" {
					state.AccText += choice.Delta.Content
					h.sendSSE(c, "response.output_text.delta", map[string]any{
						"type":            "response.output_text.delta",
						"sequence_number": state.nextSeq(),
						"output_index":    state.OutputIndex,
						"content_index":   state.ContentIndex,
						"item_id":         state.ItemID,
						"delta":           choice.Delta.Content,
					})
				}

				if choice.FinishReason != "" && choice.FinishReason != "null" {
					h.sendSSE(c, "response.output_text.done", map[string]any{
						"type":            "response.output_text.done",
						"sequence_number": state.nextSeq(),
						"output_index":    state.OutputIndex,
						"content_index":   state.ContentIndex,
						"item_id":         state.ItemID,
						"text":            state.AccText,
					})

					h.sendSSE(c, "response.content_part.done", map[string]any{
						"type":            "response.content_part.done",
						"sequence_number": state.nextSeq(),
						"output_index":    state.OutputIndex,
						"content_index":   state.ContentIndex,
						"item_id":         state.ItemID,
						"part": map[string]any{
							"type": "output_text",
							"text": state.AccText,
						},
					})

					h.sendSSE(c, "response.output_item.done", map[string]any{
						"type":            "response.output_item.done",
						"sequence_number": state.nextSeq(),
						"output_index":    state.OutputIndex,
						"item": map[string]any{
							"id":     state.ItemID,
							"type":   "message",
							"status": "completed",
							"role":   "assistant",
							"content": []map[string]any{
								{
									"type": "output_text",
									"text": state.AccText,
								},
							},
						},
					})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && state.CreatedSent {
		return
	}

	if !state.CreatedSent {
		return
	}

	h.sendSSE(c, "response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": state.nextSeq(),
		"response": map[string]any{
			"id":         state.ResponseID,
			"object":     "response",
			"created_at": state.Created,
			"model":      state.Model,
			"status":     "completed",
			"output": []map[string]any{
				{
					"id":     state.ItemID,
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []map[string]any{
						{
							"type": "output_text",
							"text": state.AccText,
						},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
	})
}

func (s *StreamState) nextSeq() int {
	s.SeqNum++
	return s.SeqNum
}

func (h *Handler) sendSSE(c *gin.Context, eventType string, data map[string]any) {
	jsonData, _ := json.Marshal(data)
	c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonData)))
	c.Writer.Flush()
}

func (h *Handler) handleReload(c *gin.Context) {
	if err := h.provider.Reload(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "reload_error", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "reloaded"})
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.POST("/v1/responses", h.Responses)
	r.POST("/api/reload", h.handleReload)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
