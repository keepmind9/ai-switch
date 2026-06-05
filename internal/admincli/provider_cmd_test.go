package admincli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderList_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/providers", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","name":"OpenAI","base_url":"https://api.openai.com","format":"chat","models":["gpt-4o","gpt-4o-mini"],"enable_proxy":false,"api_key":"sk-***","path":"","logo_url":"","think_tag":""}]`),
		})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"list"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "openai")
	assert.Contains(t, out, "gpt-4o, gpt-4o-mini")
}

func TestProviderList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","name":"OpenAI"}]`),
		})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"list", "-o", "json"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `"key": "openai"`)
}

func TestProviderAdd(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"openai","auto_route_created":true}`),
		})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"add", "--key", "openai", "--name", "OpenAI", "--base-url", "https://api.openai.com", "--api-key", "sk-test", "--model", "gpt-4o"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, "openai", receivedBody["key"])
	assert.Equal(t, "OpenAI", receivedBody["name"])
	assert.Equal(t, "chat", receivedBody["format"]) // default
}

func TestProviderAdd_CustomFormat(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{Code: 0, Msg: "ok", Data: json.RawMessage(`{}`)})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"add", "--key", "zhipu", "--name", "Zhipu", "--base-url", "https://open.bigmodel.cn", "--api-key", "key", "--format", "anthropic"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", receivedBody["format"])
}

func TestProviderUpdate(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/providers/openai", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"openai","name":"OpenAI Pro"}`),
		})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"update", "openai", "--name", "OpenAI Pro", "--enable-proxy"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, "OpenAI Pro", receivedBody["name"])
	assert.Equal(t, true, receivedBody["enable_proxy"])
	_, hasBaseURL := receivedBody["base_url"]
	assert.False(t, hasBaseURL, "base_url should not be sent when not changed")
}

func TestProviderUpdate_Models(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{Code: 0, Msg: "ok", Data: json.RawMessage(`{}`)})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"update", "openai", "--model", "gpt-4o", "--model", "o3"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	models, ok := receivedBody["models"].([]any)
	require.True(t, ok)
	assert.Equal(t, "gpt-4o", models[0])
	assert.Equal(t, "o3", models[1])
}

func TestProviderRemove(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/admin/providers/openai", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{Code: 0, Msg: "ok", Data: json.RawMessage(`null`)})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"remove", "openai"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `provider "openai" removed`)
}

func TestProviderRemove_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Code: 40400, Msg: "provider not found"})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"remove", "nonexist"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestProviderUpdate_NoArgs(t *testing.T) {
	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"update"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestProviderRemove_NoArgs(t *testing.T) {
	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"remove"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestProviderList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[]`),
		})
	}))
	defer srv.Close()

	cmd := NewProviderCmd()
	cmd.SetArgs([]string{"list"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "KEY")
}
