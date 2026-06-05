package admincli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminClient_Get_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/admin/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"key":"val"}}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	data, err := c.Get(context.Background(), "/test")
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"val"}`, string(data))
}

func TestAdminClient_Post_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"code":0,"msg":"created","data":{"id":1}}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	data, err := c.Post(context.Background(), "/items", map[string]string{"name": "test"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1}`, string(data))
}

func TestAdminClient_Put_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"updated":true}}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	data, err := c.Put(context.Background(), "/items/1", map[string]string{"name": "new"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"updated":true}`, string(data))
}

func TestAdminClient_Delete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"code":0,"msg":"ok","data":null}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	data, err := c.Delete(context.Background(), "/items/1")
	require.NoError(t, err)
	assert.Equal(t, "null", string(data))
}

func TestAdminClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"code":40400,"msg":"provider \"x\" not found","data":null}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	_, err := c.Get(context.Background(), "/providers/x")
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 40400, apiErr.Code)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Contains(t, apiErr.Msg, "not found")
}

func TestAdminClient_ConnectionError(t *testing.T) {
	c := NewAdminClient("http://127.0.0.1:1/api")
	_, err := c.Get(context.Background(), "/test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect")
}

func TestAdminClient_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	_, err := c.Get(context.Background(), "/test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

func TestAdminClient_NilBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"code":0,"msg":"ok","data":[]}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	data, err := c.Get(context.Background(), "/list")
	require.NoError(t, err)
	assert.Equal(t, "[]", string(data))
}

func TestAdminClient_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		select {}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewAdminClient(srv.URL + "/api")
	_, err := c.Get(ctx, "/test")
	require.Error(t, err)
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{Code: 40400, Msg: "not found"}
	assert.Equal(t, "API error 40400: not found", err.Error())
}

func TestAdminClient_ConflictError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `{"code":40900,"msg":"provider \"openai\" already exists","data":null}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	_, err := c.Post(context.Background(), "/providers", map[string]string{"key": "openai"})
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 40900, apiErr.Code)
	assert.Contains(t, apiErr.Msg, "already exists")
}

// Round-trip test: verify full request/response cycle with realistic provider data.
func TestAdminClient_FullRoundTrip(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path

		if r.Body != nil && r.ContentLength > 0 {
			json.NewDecoder(r.Body).Decode(&gotBody)
		}

		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"code":0,"msg":"created","data":{"key":"myprov","name":"Test","auto_route_created":true}}`)
	}))
	defer srv.Close()

	c := NewAdminClient(srv.URL + "/api")
	data, err := c.Post(context.Background(), "/providers", map[string]any{
		"key":      "myprov",
		"name":     "Test",
		"base_url": "https://api.test.com",
		"api_key":  "sk-test",
		"format":   "chat",
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/admin/providers", gotPath)
	assert.Equal(t, "myprov", gotBody["key"])
	assert.Equal(t, "sk-test", gotBody["api_key"])

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	assert.Equal(t, "myprov", result["key"])
	assert.Equal(t, true, result["auto_route_created"])
}
