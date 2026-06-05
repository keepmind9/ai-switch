package admincli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/status", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"server":{"host":"0.0.0.0","port":12345},"default_route":"openai","default_anthropic_route":"","default_responses_route":"","default_chat_route":"","provider_count":3,"route_count":5}`),
		})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"status"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "3")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "openai")
}

func TestStatus_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"server":{"host":"0.0.0.0","port":12345},"default_route":"openai","provider_count":3,"route_count":5}`),
		})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"status", "-o", "json"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `"provider_count": 3`)
}

func TestPresetList_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/presets", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","name":"OpenAI","base_url":"https://api.openai.com","format":"chat","category":"major","is_partner":false},{"key":"deepseek","name":"DeepSeek","base_url":"https://api.deepseek.com","format":"chat","category":"major","is_partner":false}]`),
		})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"preset-list"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "openai")
	assert.Contains(t, out, "deepseek")
}

func TestPresetList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","name":"OpenAI","format":"chat"}]`),
		})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"preset-list", "-o", "json"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `"key": "openai"`)
}

func TestRestart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/admin/restart", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"url":"http://localhost:12345/ui"}`),
		})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"restart"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "restarting")
}

func TestStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/admin/stop", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`null`),
		})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"stop"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "stopped")
}

func TestStatus_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{Code: 500, Msg: "internal error"})
	}))
	defer srv.Close()

	cmd := NewSystemCmd()
	cmd.SetArgs([]string{"status"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
}
