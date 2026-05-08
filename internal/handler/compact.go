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

	resp, err := h.httpClientFor(result.ProviderKey).Do(httpReq)
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

	resp, latency, fwdErr := h.forwardRequestWithKeyFallback(result, upstreamBody)
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
	if summary == "" {
		slog.Error("compact summarization returned empty summary", "model", model, "upstream_format", upstreamFormat, "status", resp.StatusCode)
		writeUpstreamError(c, "summarization produced empty result")
		return
	}

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

// decodeCompactionInBody scans a Responses API request body for fake compaction items,
// removes them from input, and prepends the decoded summary to the instructions field.
// Returns the original body unchanged if no fake compaction items are found.
func decodeCompactionInBody(body []byte) []byte {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}

	input, ok := raw["input"].([]any)
	if !ok {
		return body
	}

	modified := false
	var newInput []any
	var summaries []string

	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			newInput = append(newInput, item)
			continue
		}

		itemType, _ := m["type"].(string)
		if itemType != "compaction" {
			newInput = append(newInput, item)
			continue
		}

		encContent, _ := m["encrypted_content"].(string)
		if !converter.IsFakeCompaction(encContent) {
			newInput = append(newInput, item)
			continue
		}

		payload, err := converter.DecodeCompactionPayload(encContent)
		if err != nil {
			slog.Warn("failed to decode fake compaction in request", "error", err)
			newInput = append(newInput, item)
			continue
		}

		summaries = append(summaries, payload.Summary)
		modified = true
	}

	if !modified {
		return body
	}

	// Merge summaries into instructions field so all upstream formats handle it correctly:
	// Chat → system message, Anthropic → system field, Gemini → via Chat conversion.
	if len(summaries) > 0 {
		summaryText := "[Conversation Summary]\n" + strings.Join(summaries, "\n\n")
		existing, _ := raw["instructions"].(string)
		if existing != "" {
			raw["instructions"] = summaryText + "\n\n" + existing
		} else {
			raw["instructions"] = summaryText
		}
	}

	raw["input"] = newInput
	newBody, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return newBody
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
