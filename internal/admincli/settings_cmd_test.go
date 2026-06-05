package admincli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// injectClient sets up a cobra command with an AdminClient for the given server URL.
func injectClient(cmd *cobra.Command, srvURL string) {
	client := NewAdminClient(srvURL + "/api")
	ctx := context.WithValue(context.Background(), clientKey, client)
	cmd.SetContext(ctx)
}

// captureCmdOutput runs cmd.Execute() and captures stdout output.
func captureCmdOutput(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	// Also set output on all subcommands
	for _, sub := range cmd.Commands() {
		sub.SetOut(&buf)
	}
	err := cmd.Execute()
	return buf.String(), err
}

func TestSettingsGet_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/settings", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"host":"0.0.0.0","port":12345,"allowed_ips":[],"log_retention_days":7,"proxy_url":""}`),
		})
	}))
	defer srv.Close()

	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"get"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "0.0.0.0")
	assert.Contains(t, out, "12345")
}

func TestSettingsGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"host":"127.0.0.1","port":8080,"allowed_ips":["10.0.0.0/8"],"log_retention_days":30,"proxy_url":"socks5://proxy:1080"}`),
		})
	}))
	defer srv.Close()

	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"get", "-o", "json"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `"port": 8080`)
	assert.Contains(t, out, `"proxy_url": "socks5://proxy:1080"`)
}

func TestSettingsUpdate_Partial(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/settings", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"host":"0.0.0.0","port":8080,"allowed_ips":[],"log_retention_days":7,"proxy_url":"socks5://p:1080"}`),
		})
	}))
	defer srv.Close()

	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"update", "--port", "8080", "--proxy-url", "socks5://p:1080"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, float64(8080), receivedBody["port"])
	assert.Equal(t, "socks5://p:1080", receivedBody["proxy_url"])
	_, hasHost := receivedBody["host"]
	assert.False(t, hasHost, "host should not be sent when not changed")
}

func TestSettingsUpdate_AllowedIPs(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{Code: 0, Msg: "ok", Data: json.RawMessage(`{}`)})
	}))
	defer srv.Close()

	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"update", "--allowed-ips", "10.0.0.0/8,192.168.0.0/16"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	ips, ok := receivedBody["allowed_ips"].([]any)
	require.True(t, ok)
	assert.Equal(t, "10.0.0.0/8", ips[0])
	assert.Equal(t, "192.168.0.0/16", ips[1])
}

func TestSettingsUpdate_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Code: 400, Msg: "port must be between 1 and 65535"})
	}))
	defer srv.Close()

	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"update", "--port", "0"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port must be between 1 and 65535")
}

func TestSettingsUpdate_NoFlags(t *testing.T) {
	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"update"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "specify at least one setting to update")
}

func TestSettingsGet_ConnectionError(t *testing.T) {
	cmd := NewSettingsCmd()
	cmd.SetArgs([]string{"get"})
	// Use unreachable port
	client := NewAdminClient("http://127.0.0.1:1/api")
	ctx := context.WithValue(context.Background(), clientKey, client)
	cmd.SetContext(ctx)

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect")
}
