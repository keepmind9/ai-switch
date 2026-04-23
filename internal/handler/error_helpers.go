package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/keepmind9/ai-switch/internal/converter"
	"github.com/keepmind9/ai-switch/internal/types"
)

// errorStatusMap maps common upstream error types to HTTP status codes.
var errorStatusMap = map[string]int{
	"rate_limit_error":      http.StatusTooManyRequests,
	"authentication_error":  http.StatusUnauthorized,
	"permission_error":      http.StatusForbidden,
	"invalid_request_error": http.StatusBadRequest,
	"overloaded_error":      http.StatusServiceUnavailable,
	"server_error":          http.StatusInternalServerError,
	"api_error":             http.StatusInternalServerError,
}

// errorTypeToStatus maps an upstream error type to an HTTP status code.
// Returns http.StatusBadGateway for unknown types.
func errorTypeToStatus(errType string) int {
	if code, ok := errorStatusMap[errType]; ok {
		return code
	}
	return http.StatusBadGateway
}

// parseUpstreamError extracts error message and type from an upstream error response.
// Tries Chat format first, then Anthropic format.
func parseUpstreamError(body []byte) (message, errType string) {
	// Try Chat/OpenAI format: {"error": {"message": "...", "type": "..."}}
	var chatErr types.ChatErrorResponse
	if err := json.Unmarshal(body, &chatErr); err == nil && chatErr.Error != nil {
		return chatErr.Error.Message, chatErr.Error.Type
	}

	// Try Anthropic format: {"type": "error", "error": {"type": "...", "message": "..."}}
	var anthErr types.AnthropicErrorResponse
	if err := json.Unmarshal(body, &anthErr); err == nil && anthErr.Error != nil {
		return anthErr.Error.Message, anthErr.Error.Type
	}

	// Fallback: return raw body
	return string(body), ""
}

// writeConvertedError forwards an upstream error to the client converted into
// the client's expected error format. respBody must be pre-read by the caller.
func (h *Handler) writeConvertedError(c *gin.Context, resp *http.Response, respBody []byte, clientFormat string) {
	copyUpstreamHeaders(c, resp)

	message, errType := parseUpstreamError(respBody)
	slog.Warn("upstream error", "status", resp.StatusCode, "message", message, "type", errType, "client_format", clientFormat)

	switch clientFormat {
	case "anthropic":
		// Anthropic format: {"type": "error", "error": {"type": "...", "message": "..."}}
		if errType == "" {
			errType = "api_error"
		}
		c.JSON(resp.StatusCode, types.AnthropicErrorResponse{
			Type: "error",
			Error: &types.AnthropicErrorDetail{
				Type:    errType,
				Message: message,
			},
		})
	default:
		// Chat/Responses format: {"error": {"message": "...", "type": "...", "code": "..."}}
		c.JSON(resp.StatusCode, types.ChatErrorResponse{
			Error: &types.ChatErrorDetail{
				Message: message,
				Type:    errType,
				Code:    errType,
			},
		})
	}
}

// isSSEResponse checks if the upstream response is SSE (text/event-stream).
func isSSEResponse(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/event-stream")
}

// isSSEErrorData checks if an SSE data payload contains an error object.
func isSSEErrorData(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "[DONE]" {
		return false
	}
	var raw map[string]any
	if json.Unmarshal([]byte(trimmed), &raw) != nil {
		return false
	}
	_, hasError := raw["error"]
	return hasError
}

// writeStreamErrorJSON writes a JSON error response for a streaming path that
// hasn't started SSE yet (upstream returned non-SSE in a streaming request).
func writeStreamErrorJSON(c *gin.Context, statusCode int, message, errType, clientFormat string) {
	switch clientFormat {
	case "anthropic":
		if errType == "" {
			errType = "api_error"
		}
		c.JSON(statusCode, types.AnthropicErrorResponse{
			Type: "error",
			Error: &types.AnthropicErrorDetail{
				Type:    errType,
				Message: message,
			},
		})
	default:
		c.JSON(statusCode, types.ChatErrorResponse{
			Error: &types.ChatErrorDetail{
				Message: message,
				Type:    errType,
				Code:    errType,
			},
		})
	}
}

// writeSSEErrorToClient writes an error event to the client's SSE stream
// in the appropriate format before the stream closes.
func writeSSEErrorToClient(w converter.SSEWriter, msg, errType, clientFormat string) {
	switch clientFormat {
	case "anthropic":
		if errType == "" {
			errType = "api_error"
		}
		w.WriteEvent("error", map[string]any{
			"type":  "error",
			"error": map[string]any{"type": errType, "message": msg},
		})
	default:
		w.WriteEvent("", map[string]any{
			"error": map[string]any{"message": msg, "type": errType},
		})
	}
}
