package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pprof is an opt-in diagnostic surface: it must stay off by default, surface
// bind failures (so an operator is never silently running without profiling),
// and only serve the standard endpoints when enabled.

// Empty addr is the default (disabled) state: no server, no goroutine, and the
// pprof handlers must never be registered onto any mux the main server uses.
func TestStartPprofServer_DisabledWhenAddrEmpty(t *testing.T) {
	srv, err := startPprofServer("")
	assert.Nil(t, srv, "empty addr must not start a pprof server")
	assert.NoError(t, err)
}

// A bind failure (e.g. port already in use) must be returned to the caller
// rather than logged-only inside the goroutine, otherwise the process keeps
// running with the operator believing profiling is active.
func TestStartPprofServer_BindFailureReturnsError(t *testing.T) {
	// Occupy a port, then ask pprof to bind the same one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	srv, err := startPprofServer(ln.Addr().String())
	assert.Nil(t, srv, "bind failure must not return a server")
	require.Error(t, err, "bind failure must surface an error")
}

// The success path returns a usable server; closing it stops the serve goroutine.
func TestStartPprofServer_BindsSuccessfully(t *testing.T) {
	srv, err := startPprofServer("127.0.0.1:0")
	require.NoError(t, err)
	require.NotNil(t, srv)
	defer srv.Close()
}

// When enabled, every standard pprof endpoint must respond. Driving the
// registered mux via httptest (instead of binding a real port) isolates the
// behavior under test from ephemeral port allocation.
func TestRegisterPprofHandlers_ServesStandardEndpoints(t *testing.T) {
	paths := []string{
		"/debug/pprof/",     // index page (also routes /heap, /goroutine, ...)
		"/debug/pprof/heap", // alloc profile, routed through Index
		"/debug/pprof/goroutine",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	}

	mux := http.NewServeMux()
	registerPprofHandlers(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp, err := http.Get(ts.URL + p)
			require.NoError(t, err)
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
