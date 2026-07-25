package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
)

// registerPprofHandlers installs the standard pprof endpoints onto mux.
//
// It mirrors the package-level registration that `import _ "net/http/pprof"`
// would perform against http.DefaultServeMux, but on a caller-supplied mux so
// the main server's mux is never touched. The Index handler also serves the
// named profiles (heap, goroutine, mutex, ...) under /debug/pprof/<name>.
func registerPprofHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

// startPprofServer launches a standalone pprof HTTP server on addr.
//
// It collects CPU/heap/goroutine/mutex/block profiles of the running server
// under real traffic. The server runs on its own port with its own ServeMux,
// fully separate from the main gin engine, so request serving is identical
// whether or not pprof is enabled. When pprof is off (addr == ""), this does
// nothing and no handler is ever registered.
//
// The listen socket is bound synchronously so a port conflict is returned to
// the caller rather than lost inside the server goroutine — otherwise the
// process would keep running with the operator believing profiling is active.
//
// Returns (nil, nil) when disabled, (nil, err) on bind failure. The returned
// server should be Shutdown on process exit; if left running it dies with the
// process anyway.
func startPprofServer(addr string) (*http.Server, error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("pprof listen %s: %w", addr, err)
	}
	mux := http.NewServeMux()
	registerPprofHandlers(mux)

	srv := &http.Server{Handler: mux}
	go func() {
		slog.Info("starting pprof server", "addr", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("pprof server stopped", "error", err)
		}
	}()
	return srv, nil
}
