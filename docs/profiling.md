# Live Profiling Guide

Collect CPU, heap, and goroutine profiles from a running `ais` server under
**real traffic**, to locate the next optimization hotspot with representative
payloads rather than a synthetic load generator.

## How it works

`ais serve` accepts an optional `--pprof <addr>` flag that starts a standalone
[pprof](https://pkg.go.dev/net/http/pprof) HTTP server. It runs on its own port
with its own request mux, completely separate from the main proxy engine —
request handling is identical whether or not pprof is enabled, and when the flag
is omitted (the default) no pprof code path is active at all.

This lets you profile the exact process you use every day, against real upstream
providers and real payload shapes.

## Enable

Add the flag to your normal `serve` command — foreground or daemon:

```sh
ais serve --pprof 127.0.0.1:6060            # foreground
ais serve -d --pprof 127.0.0.1:6060         # daemon (daily mode)
```

- The value is the listen address. An empty value (omitting the flag) means
  **off** — the default. pprof never starts unless you ask for it.
- Bind to `127.0.0.1` (loopback) so the endpoints are reachable only from your
  own machine. pprof exposes runtime internals; never point it at an untrusted
  network.
- The flag carries across `--daemon`/`-d` spawns and config-driven restarts: a
  daemon started with `--pprof` keeps the pprof server up, and a restart re-adds
  the same flag to the new process.

## Collect a profile

Point `go tool pprof` at the running server. Replace `30` with the sample
duration in seconds.

| What | Command |
|---|---|
| CPU (30s sample) | `go tool pprof -http=:8080 http://127.0.0.1:6060/debug/pprof/profile?seconds=30` |
| Heap / allocations | `go tool pprof -http=:8080 http://127.0.0.1:6060/debug/pprof/heap` |
| Goroutines | `go tool pprof -http=:8080 http://127.0.0.1:6060/debug/pprof/goroutine` |

`-http=:8080` opens an interactive flame-graph view in your browser. Drop it to
get the command-line `pprof` prompt (`top`, `list <func>`, `web`, ...).

Tips:
- For allocation pressure / GC, open the heap profile and switch the sample to
  `alloc_space` (cumulative allocations) instead of the default `inuse_space`.
- Drive realistic load while sampling — run your normal client traffic through
  the proxy for the full `seconds` window so the sample reflects real usage.

### Mutex / block contention

The mutex and block profiles (`/debug/pprof/mutex`, `/debug/pprof/block`) are
registered but collect nothing by default, because sampling them requires
setting a profiling rate that adds overhead to the hot path. If you need them,
set `runtime.SetMutexProfileFraction(1)` and `runtime.SetBlockProfileRate(...)`
in a custom build; the default server leaves them off to honor the
zero-overhead-when-disabled guarantee.

## Read the result

In the browser view:

- **Flame graph** — box width = share of CPU/allocations. Look for wide boxes
  inside the proxy (under `internal/handler` / `internal/converter`) rather than
  in stdlib leaves you cannot change.
- **Top** — sort by cumulative to find the call subtree that dominates.

Compare profiles taken before and after a change to confirm an optimization
actually moved the needle.
