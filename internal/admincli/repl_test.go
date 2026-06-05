package admincli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestREPL_Exit(t *testing.T) {
	in := strings.NewReader("exit\n")
	var out bytes.Buffer

	cmd := &cobra.Command{Use: "admin"}
	ctx := context.WithValue(context.Background(), clientKey, &AdminClient{})
	cmd.SetContext(ctx)

	err := RunREPL(cmd, in, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "bye.")
}

func TestREPL_Quit(t *testing.T) {
	in := strings.NewReader("quit\n")
	var out bytes.Buffer

	cmd := &cobra.Command{Use: "admin"}
	cmd.SetContext(context.WithValue(context.Background(), clientKey, &AdminClient{}))

	err := RunREPL(cmd, in, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "bye.")
}

func TestREPL_Help(t *testing.T) {
	in := strings.NewReader("help\nexit\n")
	var out bytes.Buffer

	cmd := &cobra.Command{Use: "admin", Short: "Manage ais server via admin API"}
	cmd.SetContext(context.WithValue(context.Background(), clientKey, &AdminClient{}))

	err := RunREPL(cmd, in, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Manage ais server")
}

func TestREPL_EmptyLine(t *testing.T) {
	in := strings.NewReader("\n\nexit\n")
	var out bytes.Buffer

	cmd := &cobra.Command{Use: "admin"}
	cmd.SetContext(context.WithValue(context.Background(), clientKey, &AdminClient{}))

	err := RunREPL(cmd, in, &out)
	require.NoError(t, err)
	// Should show prompts and exit cleanly
	assert.Contains(t, out.String(), "bye.")
}

func TestREPL_UnknownCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{Code: 0, Msg: "ok", Data: json.RawMessage(`[]`)})
	}))
	defer srv.Close()

	in := strings.NewReader("provider foobar\nexit\n")
	var out bytes.Buffer

	client := NewAdminClient(srv.URL + "/api")
	parent := &cobra.Command{Use: "admin"}
	parent.AddCommand(NewProviderCmd())
	parent.SetContext(context.WithValue(context.Background(), clientKey, client))

	err := RunREPL(parent, in, &out)
	require.NoError(t, err)
	// Cobra shows usage help for unknown subcommands.
	assert.Contains(t, out.String(), "Available Commands:")
}

func TestREPL_DispatchSubcommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","name":"OpenAI","base_url":"https://api.openai.com","format":"chat","models":["gpt-4o"],"enable_proxy":false,"api_key":"sk-***","path":"","logo_url":"","think_tag":""}]`),
		})
	}))
	defer srv.Close()

	in := strings.NewReader("provider list\nexit\n")
	var out bytes.Buffer

	client := NewAdminClient(srv.URL + "/api")
	cmd := NewProviderCmd()
	parent := &cobra.Command{Use: "admin"}
	parent.AddCommand(cmd)
	parent.SetContext(context.WithValue(context.Background(), clientKey, client))

	err := RunREPL(parent, in, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "openai")
}

func TestREPL_MultipleCommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"host":"0.0.0.0","port":12345,"allowed_ips":[],"log_retention_days":7,"proxy_url":""}`),
		})
	}))
	defer srv.Close()

	in := strings.NewReader("settings get\nsettings get -o json\nexit\n")
	var out bytes.Buffer

	client := NewAdminClient(srv.URL + "/api")
	settingsCmd := NewSettingsCmd()
	parent := &cobra.Command{Use: "admin"}
	parent.AddCommand(settingsCmd)
	parent.SetContext(context.WithValue(context.Background(), clientKey, client))

	err := RunREPL(parent, in, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "0.0.0.0")
	assert.Contains(t, out.String(), `"host": "0.0.0.0"`)
}
