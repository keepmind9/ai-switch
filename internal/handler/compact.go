package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/router"
	"github.com/keepmind9/ai-switch/internal/types"
)

// handleCompact handles POST /v1/responses/compact.
// For OpenAI upstream: transparent passthrough.
// For non-OpenAI upstream: LLM-based summarization.
func (h *Handler) handleCompact(c *gin.Context) {
	body, err := readBody(c)
	if err != nil {
		status := http.StatusBadRequest
		if len(body) > maxRequestBodyBytes {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": gin.H{"code": "invalid_request", "message": err.Error()}})
		return
	}

	var req types.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "invalid JSON: " + err.Error()}})
		return
	}

	apiKey := extractClientAPIKey(c)
	result, routeErr := h.router.Route(converter.FormatResponses, apiKey, body)
	if routeErr != nil {
		slog.Error("compact route error", "error", routeErr)
		writeRouteError(c, routeErr.Error())
		return
	}

	upstreamFormat := result.Format
	if upstreamFormat == "" {
		upstreamFormat = converter.FormatChat
	}

	// OpenAI transparent passthrough
	if upstreamFormat == converter.FormatResponses {
		h.forwardCompactPassthrough(c, body, result)
		return
	}

	// Non-OpenAI: simulated compact via LLM summarization
	h.handleSimulatedCompact(c, &req, result, upstreamFormat)
}

// forwardCompactPassthrough forwards the compact request directly to OpenAI upstream.
func (h *Handler) forwardCompactPassthrough(c *gin.Context, body []byte, result *router.RouteResult) {
	compactPath := strings.TrimSuffix(result.Path, "/") + "/compact"
	upstreamURL := router.BuildUpstreamURL(result.BaseURL, compactPath)

	httpReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", upstreamURL, bytes.NewReader(body))
	if err != nil {
		writeUpstreamError(c, "failed to create request: "+err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+result.APIKey)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		writeUpstreamError(c, "failed to call upstream: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", respBody)
}

// handleSimulatedCompact performs LLM-based summarization for non-OpenAI upstreams.
func (h *Handler) handleSimulatedCompact(c *gin.Context, req *types.ResponsesRequest, result *router.RouteResult, upstreamFormat string) {
	model := result.Model
	if model == "" {
		model = req.Model
	}

	conversation := converter.ExtractConversationText(req.Input)
	if conversation == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "empty input"}})
		return
	}

	chatReq := converter.BuildSummarizationRequest(conversation, model)

	var upstreamBody []byte
	var err error
	switch upstreamFormat {
	case converter.FormatChat:
		upstreamBody, err = json.Marshal(chatReq)
	case converter.FormatAnthropic:
		anthReq, convErr := h.converter.ChatRequestToAnthropic(chatReq)
		if convErr != nil {
			writeConversionError(c, convErr.Error())
			return
		}
		anthReq.Stream = false
		upstreamBody, err = json.Marshal(anthReq)
	case converter.FormatGemini:
		gemReq, convErr := h.converter.ChatToGeminiRequest(chatReq)
		if convErr != nil {
			writeConversionError(c, convErr.Error())
			return
		}
		upstreamBody, err = json.Marshal(gemReq)
	default:
		writeBadRequest(c, converter.FormatResponses, "unsupported upstream format: "+upstreamFormat)
		return
	}
	if err != nil {
		writeConversionError(c, err.Error())
		return
	}

	resp, latency, fwdErr := h.forwardRequest(result, upstreamBody)
	if fwdErr != nil {
		writeUpstreamError(c, "summarization request failed: "+fwdErr.Error())
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		slog.Error("compact summarization upstream error", "status", resp.StatusCode, "body", string(respBody))
		writeUpstreamError(c, fmt.Sprintf("summarization upstream returned %d", resp.StatusCode))
		return
	}

	summary := extractSummaryFromResponse(respBody, upstreamFormat)

	slog.Info("compact summarization complete",
		"model", model, "latency", latency, "summary_len", len(summary))

	payload := &types.CompactionPayload{
		Summary: summary,
		Model:   model,
		TS:      time.Now().Unix(),
	}
	encrypted, encErr := converter.EncodeCompactionPayload(payload)
	if encErr != nil {
		writeConversionError(c, encErr.Error())
		return
	}

	compactionResp := buildCompactionResponse(encrypted, req.Input)
	c.JSON(http.StatusOK, compactionResp)
}

// extractSummaryFromResponse extracts text content from an upstream response.
func extractSummaryFromResponse(body []byte, format string) string {
	switch format {
	case converter.FormatChat:
		var resp types.ChatResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return ""
		}
		for _, ch := range resp.Choices {
			if ch.Message.Content != nil && *ch.Message.Content != "" {
				return *ch.Message.Content
			}
		}
	case converter.FormatAnthropic:
		var resp converter.AnthropicResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return ""
		}
		for _, block := range resp.Content {
			if block.Type == "text" && block.Text != "" {
				return block.Text
			}
		}
	case converter.FormatGemini:
		var resp converter.GeminiResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return ""
		}
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.Text != "" {
						return part.Text
					}
				}
			}
		}
	}
	return ""
}

// buildCompactionResponse constructs a response.compaction JSON matching the OpenAI format.
func buildCompactionResponse(encryptedContent string, input any) map[string]any {
	now := time.Now().Unix()
	output := []any{}

	if items, ok := input.([]any); ok {
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role == "user" {
				output = append(output, map[string]any{
					"id":     fmt.Sprintf("msg_%d", now),
					"type":   "message",
					"status": "completed",
					"role":   "user",
					"content": []any{
						map[string]any{"type": "input_text", "text": extractInputTextFromItem(m)},
					},
				})
				break
			}
		}
	}

	output = append(output, map[string]any{
		"id":                fmt.Sprintf("cmp_%d", now),
		"type":              "compaction",
		"encrypted_content": encryptedContent,
	})

	return map[string]any{
		"id":         fmt.Sprintf("resp_compact_%d", now),
		"object":     "response.compaction",
		"created_at": now,
		"output":     output,
	}
}

// extractInputTextFromItem extracts text from a message item's content.
func extractInputTextFromItem(m map[string]any) string {
	content, ok := m["content"]
	if !ok {
		return ""
	}
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, item := range c {
			if cm, ok := item.(map[string]any); ok {
				if text, ok := cm["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
