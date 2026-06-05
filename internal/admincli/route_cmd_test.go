package admincli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteList_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/routes", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","provider":"openai","default_model":"gpt-4o","disabled":false,"scene_map":null,"model_map":null,"long_context_threshold":0},{"key":"deepseek","provider":"deepseek","default_model":"deepseek-chat","disabled":true,"scene_map":null,"model_map":null,"long_context_threshold":0}]`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"list"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "openai")
	assert.Contains(t, out, "deepseek")
	assert.Contains(t, out, "gpt-4o")
	assert.Contains(t, out, "true")
}

func TestRouteList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[{"key":"openai","provider":"openai","default_model":"gpt-4o","disabled":false}]`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"list", "-o", "json"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `"key": "openai"`)
}

func TestRouteAdd(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/admin/routes", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"myroute","provider":"openai","default_model":"gpt-4o"}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"add", "--key", "myroute", "--provider", "openai", "--default-model", "gpt-4o"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, "myroute", receivedBody["key"])
	assert.Equal(t, "openai", receivedBody["provider"])
	assert.Equal(t, "gpt-4o", receivedBody["default_model"])
}

func TestRouteAdd_Disabled(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"test","provider":"openai"}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"add", "--key", "test", "--provider", "openai", "--disabled"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Equal(t, true, receivedBody["disabled"])
}

func TestRouteUpdate(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/routes/myroute", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"myroute","provider":"deepseek"}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"update", "myroute", "--provider", "deepseek", "--disabled"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, "deepseek", receivedBody["provider"])
	assert.Equal(t, true, receivedBody["disabled"])
	_, hasDefaultModel := receivedBody["default_model"]
	assert.False(t, hasDefaultModel, "default_model should not be sent when not changed")
}

func TestRouteRemove(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/admin/routes/myroute", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{Code: 0, Msg: "ok", Data: json.RawMessage(`null`)})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"remove", "myroute"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `route "myroute" removed`)
}

// --- route default list/set/remove ---

func TestRouteDefaultList_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/status", r.URL.Path)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"server":{"version":"1.0"},"default_route":"openai","default_anthropic_route":"claude","default_responses_route":"","default_chat_route":"","provider_count":3,"route_count":5}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"default", "list"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "TYPE")
	assert.Contains(t, out, "ROUTE KEY")
	assert.Contains(t, out, "openai")
	assert.Contains(t, out, "claude")
}

func TestRouteDefaultList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"server":{},"default_route":"openai","default_anthropic_route":"","default_responses_route":"","default_chat_route":"","provider_count":0,"route_count":0}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"default", "list", "-o", "json"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, `"default_route": "openai"`)
}

func TestRouteDefaultSet(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/default-routes", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"default_route":"openai","default_anthropic_route":"claude"}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"default", "set", "--default", "openai", "--anthropic", "claude"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, "openai", receivedBody["default_route"])
	assert.Equal(t, "claude", receivedBody["default_anthropic_route"])
	_, hasResponses := receivedBody["default_responses_route"]
	assert.False(t, hasResponses, "responses should not be sent when not changed")
}

func TestRouteDefaultRemove(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/default-routes", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"default_route":"","default_anthropic_route":"","default_responses_route":"","default_chat_route":""}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"default", "remove", "--default", "--anthropic"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)

	assert.Equal(t, "", receivedBody["default_route"])
	assert.Equal(t, "", receivedBody["default_anthropic_route"])
	_, hasResponses := receivedBody["default_responses_route"]
	assert.False(t, hasResponses, "responses should not be sent when not flagged")
}

func TestRouteDefaultRemove_NoFlags(t *testing.T) {
	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"default", "remove"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "specify at least one flag")
}

// --- route enable/disable ---

func TestRouteEnable(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/routes/myroute", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"myroute","disabled":false}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"enable", "myroute"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Equal(t, false, receivedBody["disabled"])
}

func TestRouteDisable(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/admin/routes/myroute", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&receivedBody)
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`{"key":"myroute","disabled":true}`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"disable", "myroute"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Equal(t, true, receivedBody["disabled"])
}

func TestRouteEnable_NoArgs(t *testing.T) {
	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"enable"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
}

func TestRouteDisable_NoArgs(t *testing.T) {
	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"disable"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
}

// --- error cases ---

func TestRouteUpdate_NoArgs(t *testing.T) {
	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"update"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestRouteRemove_NoArgs(t *testing.T) {
	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"remove"})
	injectClient(cmd, "http://unused")

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage:")
}

func TestRouteList_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIResponse{Code: 500, Msg: "internal error"})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"list"})
	injectClient(cmd, srv.URL)

	_, err := captureCmdOutput(cmd)
	assert.Error(t, err)
}

func TestRouteList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(APIResponse{
			Code: 0,
			Msg:  "ok",
			Data: json.RawMessage(`[]`),
		})
	}))
	defer srv.Close()

	cmd := NewRouteCmd()
	cmd.SetArgs([]string{"list"})
	injectClient(cmd, srv.URL)

	out, err := captureCmdOutput(cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "KEY")
}
