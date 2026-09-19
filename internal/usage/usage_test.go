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
		name         string
		body         string
		status       int
		wantErr      string
		wantUtil     float64
		wantActive   bool
		wantResetsAt int64
	}{
		{
			name:         "active window",
			body:         `{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","percentage":78.5,"nextResetTime":1758291600000}]}}`,
			wantUtil:     78.5,
			wantActive:   true,
			wantResetsAt: 1758291600000,
		},
		{
			name:         "window waiting to trigger",
			body:         `{"success":true,"data":{"limits":[{"type":"TOKENS_LIMIT","percentage":0,"nextResetTime":null}]}}`,
			wantUtil:     0,
			wantActive:   false,
			wantResetsAt: 0,
		},
		{
			name:         "skips other limit types",
			body:         `{"success":true,"data":{"limits":[{"type":"REQUESTS_LIMIT","percentage":10,"nextResetTime":123},{"type":"TOKENS_LIMIT","percentage":42,"nextResetTime":1758291600000}]}}`,
			wantUtil:     42,
			wantActive:   true,
			wantResetsAt: 1758291600000,
		},
		{
			name:    "api error",
			body:    `{"success":false,"msg":"quota exceeded"}`,
			wantErr: "quota exceeded",
		},
		{
			name:    "no tokens limit",
			body:    `{"success":true,"data":{"limits":[{"type":"REQUESTS_LIMIT","percentage":10}]}}`,
			wantErr: "no TOKENS_LIMIT",
		},
		{
			name:    "empty limits",
			body:    `{"success":true,"data":{"limits":[]}}`,
			wantErr: "no TOKENS_LIMIT",
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
			assert.Equal(t, tt.wantUtil, info.Utilization)
			assert.Equal(t, tt.wantActive, info.WindowActive)
			assert.Equal(t, tt.wantResetsAt, info.ResetsAtMs)
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
	info := Info{Utilization: 50, ResetsAtMs: 1758291600000, WindowActive: true}
	b, err := json.Marshal(info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"utilization":50,"resets_at_ms":1758291600000,"window_active":true}`, string(b))
}
