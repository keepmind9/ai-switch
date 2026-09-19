package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupported(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected bool
	}{
		{"bigmodel cn", "https://open.bigmodel.cn/api/anthropic", true},
		{"bigmodel cn chat", "https://open.bigmodel.cn/api/coding/paas/v4", true},
		{"z.ai", "https://api.z.ai/api/anthropic", true},
		{"minimax", "https://api.minimaxi.com/anthropic", false},
		{"openai", "https://api.openai.com/v1", false},
		{"empty", "", false},
		{"invalid url", "://not-a-url", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Supported(tt.baseURL))
		})
	}
}

// newQuotaServer spins up a mock GLM usage API and returns the Authorization
// header it received.
func newQuotaServer(t *testing.T, status int, body string) (*httptest.Server, *string) {
	t.Helper()
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/monitor/usage/quota/limit", r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &authHeader
}

func TestQuery(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		status      int
		wantErr     string
		wantLevel   string
		wantWindows []Window
	}{
		{
			// Captured from the real GLM quota API (lite plan): a TIME_LIMIT
			// (tool-usage cap) entry plus the 5-hour TOKENS_LIMIT window.
			name:      "real lite-plan response",
			body:      `{"code":200,"msg":"ok","data":{"limits":[{"type":"TIME_LIMIT","unit":5,"number":1,"usage":100,"currentValue":13,"remaining":87,"percentage":13,"nextResetTime":1790737832981,"usageDetails":[{"modelCode":"search-prime","usage":10}]},{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":64,"nextResetTime":1789816181131}],"level":"lite"},"success":true}`,
			wantLevel: "lite",
			wantWindows: []Window{
				{Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Utilization: 64, ResetsAtMs: 1789816181131, WindowActive: true},
			},
		},
		{
			name:      "credit plan",
			body:      `{"success":true,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":1,"number":1,"percentage":30,"nextResetTime":null}],"level":"credit"}}`,
			wantLevel: "credit",
			wantWindows: []Window{
				{Type: "CREDIT_LIMIT", Unit: 1, Number: 1, Utilization: 30, ResetsAtMs: 0, WindowActive: false},
			},
		},
		{
			name: "multiple token windows keep api order",
			body: `{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","unit":3,"number":1,"percentage":10,"nextResetTime":123},{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":42,"nextResetTime":1758291600000}]}}`,
			wantWindows: []Window{
				{Type: "TOKENS_LIMIT", Unit: 3, Number: 1, Utilization: 10, ResetsAtMs: 123, WindowActive: true},
				{Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Utilization: 42, ResetsAtMs: 1758291600000, WindowActive: true},
			},
		},
		{
			name:    "api error",
			body:    `{"success":false,"msg":"quota exceeded"}`,
			wantErr: "quota exceeded",
		},
		{
			name:    "only non-window limit types",
			body:    `{"success":true,"data":{"limits":[{"type":"REQUESTS_LIMIT","percentage":10},{"type":"TIME_LIMIT","percentage":5}]}}`,
			wantErr: "no TOKENS_LIMIT or CREDIT_LIMIT",
		},
		{
			name:    "empty limits",
			body:    `{"success":true,"data":{"limits":[]}}`,
			wantErr: "no TOKENS_LIMIT or CREDIT_LIMIT",
		},
		{
			name:    "http error",
			body:    `{"success":true}`,
			status:  http.StatusInternalServerError,
			wantErr: "status 500",
		},
		{
			name:    "malformed json",
			body:    `not-json`,
			wantErr: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			if status == 0 {
				status = http.StatusOK
			}
			srv, authHeader := newQuotaServer(t, status, tt.body)

			info, err := Query(context.Background(), nil, srv.URL, "test-key")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantLevel, info.Level)
			assert.Equal(t, tt.wantWindows, info.Windows)
			// GLM expects the raw API key, no Bearer prefix.
			assert.Equal(t, "test-key", *authHeader)
		})
	}
}

func TestQueryInvalidBaseURL(t *testing.T) {
	_, err := Query(context.Background(), nil, "://not-a-url", "k")
	require.Error(t, err)
}

func TestQueryEnvVarKey(t *testing.T) {
	srv, authHeader := newQuotaServer(t, http.StatusOK,
		`{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","percentage":1,"nextResetTime":1}]}}`)

	t.Setenv("USAGE_TEST_KEY", "expanded-key")

	_, err := Query(context.Background(), nil, srv.URL, "${USAGE_TEST_KEY}")
	require.NoError(t, err)
	assert.Equal(t, "expanded-key", *authHeader)
}

func TestQueryTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := Query(ctx, nil, srv.URL, "k")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "timeout"))
}

func TestInfoMarshal(t *testing.T) {
	info := Info{
		Level: "lite",
		Windows: []Window{
			{Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Utilization: 64, ResetsAtMs: 1758291600000, WindowActive: true},
		},
	}
	b, err := json.Marshal(info)
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"level":"lite","windows":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"utilization":64,"resets_at_ms":1758291600000,"window_active":true}]}`,
		string(b))
}
