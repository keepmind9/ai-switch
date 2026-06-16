package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyCustomHeaders_OverridesForwardedHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("User-Agent", "codex-cli/1.0")

	// Configured value must replace the transparently forwarded client header.
	applyCustomHeaders(req, map[string]string{"User-Agent": "claude-code/1.0.0"})

	assert.Equal(t, "claude-code/1.0.0", req.Header.Get("User-Agent"))
}

func TestApplyCustomHeaders_AddsMultipleHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	applyCustomHeaders(req, map[string]string{
		"User-Agent": "claude-code/1.0.0",
		"X-Title":    "ai-switch",
	})

	assert.Equal(t, "claude-code/1.0.0", req.Header.Get("User-Agent"))
	assert.Equal(t, "ai-switch", req.Header.Get("X-Title"))
}

func TestApplyCustomHeaders_NilAndEmptyIsNoOp(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("User-Agent", "original")

	applyCustomHeaders(req, nil)
	assert.Equal(t, "original", req.Header.Get("User-Agent"))

	applyCustomHeaders(req, map[string]string{})
	assert.Equal(t, "original", req.Header.Get("User-Agent"))
}
