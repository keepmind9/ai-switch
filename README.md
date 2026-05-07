# ai-switch

[![Go Report Card](https://goreportcard.com/badge/github.com/keepmind9/ai-switch)](https://goreportcard.com/report/github.com/keepmind9/ai-switch) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**English** | [中文](README_zh.md)

A lightweight local proxy that lets any AI CLI tool (Claude Code, Codex CLI, etc.) use third-party LLM APIs through a unified local endpoint.

**One binary, one config, any AI CLI → any LLM API.**

## Features

- **Multi-protocol**: Auto-detects client protocol (Responses API, Anthropic Messages, Chat Completions) and converts transparently
- **Zero-intrusion**: No changes to your CLI config files, just point `base_url` to the local proxy
- **Scene routing**: Route Claude Code requests to different models based on request type (thinking, web search, background tasks)
- **Model mapping**: Map client model names to upstream model names at the route level
- **Cross-provider routing**: Route different scenes to different providers (e.g. thinking → DeepSeek, web search → Zhipu)
- **Hot reload**: Update config without restart (`POST /api/reload` or `kill -HUP`)
- **Admin UI**: Built-in web dashboard for managing providers, routes, viewing usage statistics, and debugging requests
- **Request tracing**: Inspect every request/response pair with raw viewer, Diff view, and TTFB waterfall chart
- **Usage statistics**: Track token usage (input, output, cache) by provider and model with dashboard charts
- **Multi-key fallback**: Automatically switch to fallback API keys on 429/529 rate limiting or service overload
- **Context compaction**: Support compact endpoint for context window management, with LLM-based summarization for non-OpenAI upstreams
- **Lightweight**: Pure Go, single binary, no CGO

## Installation

### One-line install (recommended)

**Linux / macOS:**

```bash
curl -sL https://raw.githubusercontent.com/keepmind9/ai-switch/main/scripts/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/keepmind9/ai-switch/main/scripts/install.ps1 | iex
```

This downloads the latest release for your platform, installs to `~/.local/bin`, and adds it to PATH.

### Build from source

```bash
git clone https://github.com/keepmind9/ai-switch.git
cd ai-switch
make build-all   # build frontend + Go binary (includes Admin UI)
```

> If you don't need the Admin UI, use `make build` instead (Go only, faster).

## Quick Start

### 1. Start the server

```bash
ai-switch serve
```

No config file needed — it auto-creates `~/.ai-switch/config.yaml` with defaults on first run.

### 2. Configure via Admin UI

Open `http://localhost:12345` in your browser to add providers and routes.

### 3. Point your CLI tool

**Claude Code:**

```bash
export ANTHROPIC_BASE_URL=http://localhost:12345
export ANTHROPIC_API_KEY=<route-key>
```

**Codex CLI:**

```toml
[model_providers.proxy]
name = "ai-switch"
base_url = "http://localhost:12345/v1"
api_key = "ais-default"
wire_api = "responses"
```

**Any OpenAI-compatible tool:**

```bash
export OPENAI_BASE_URL=http://localhost:12345/v1
export OPENAI_API_KEY=<route-key>
```

That's it — your CLI tool will now route requests through ai-switch to your configured provider.

## How It Works

```
Claude Code ──→ ai-switch ──→ DeepSeek (chat)
Codex CLI  ──→          ──→ Zhipu    (anthropic)
Any tool   ──→          ──→ Gemini   (gemini)
Any tool   ──→          ──→ MiniMax  (chat)
```

ai-switch sits between your CLI tool and upstream LLM providers. It:
- Detects the client protocol automatically (Anthropic / Responses / Chat)
- Routes requests to the correct provider based on the API key (route key)
- Converts between protocols when needed (e.g. Anthropic → Chat Completions)
- Detects request scenes (thinking, web search, etc.) for smart routing

The route key (`<route-key>` in the example above) serves as both the API key for authentication and the routing identifier.

## Configuration

### Providers

Define your upstream LLM vendor connections:

```yaml
providers:
  deepseek:
    name: "DeepSeek"
    base_url: "https://api.deepseek.com/v1"
    api_key: "${DEEPSEEK_API_KEY}"    # supports ${ENV_VAR} expansion
    format: "chat"                     # chat (default) | responses | anthropic | gemini
    think_tag: "think"                 # optional: strip reasoning tags from responses
    fallback_keys:                     # optional: fallback API keys on 429 rate limiting
      - "${DEEPSEEK_API_KEY_2}"
      - "${DEEPSEEK_API_KEY_3}"
    models:                            # optional: for validation warnings
      - "deepseek-chat"
      - "deepseek-reasoner"
```

### Gemini Provider

Use Google Gemini as upstream:

```yaml
providers:
  google:
    name: "Google Gemini"
    base_url: "https://generativelanguage.googleapis.com"
    api_key: "${GOOGLE_API_KEY}"
    format: "gemini"
```

No `path` needed — ai-switch automatically builds `/v1beta/models/{model}:generateContent`.

### Routes

Routes map API keys to providers and models:

```yaml
routes:
  "ais-default":
    provider: "deepseek"
    default_model: "deepseek-chat"
```

### Scene Map

Route Claude Code requests to different models based on what it's doing:

```yaml
routes:
  "ais-claude":
    provider: "zhipu"
    default_model: "glm-5.1"
    long_context_threshold: 60000
    scene_map:
      default: "glm-5.1"
      think: "glm-5.1"
      websearch: "glm-4.7"
      background: "glm-4.5-air"
      longContext: "glm-5.1"
```

| Scene | Key | Detection |
|-------|-----|-----------|
| Long Context | `longContext` | Token count exceeds `long_context_threshold` |
| Background | `background` | Model name contains "haiku" |
| Web Search | `websearch` | Tools contain `web_search_*` type |
| Thinking | `think` | `thinking` field present |
| Image | `image` | User messages contain image blocks |
| Default | `default` | Fallback |

Priority: `longContext` > `background` > `websearch` > `think` > `image` > `default`

### Default Routes

Control which route is used when a request has no matching API key:

```yaml
default_route: "ais-default"              # global fallback
default_anthropic_route: "ais-zhipu"      # /v1/messages (Claude Code)
default_responses_route: "ais-default"    # /v1/responses (Codex CLI)
default_chat_route: "ais-default"         # /v1/chat/completions
```

**Routing priority:** route key match > protocol-specific default > global `default_route`

All fields are optional. Protocol-specific defaults fall back to `default_route` when not set.

### Log Retention

Control how many days of log files to keep (default: 30):

```yaml
log_retention_days: 7
```

Logs are stored in `~/.ai-switch/logs/`.

### IP Whitelist

When binding to a non-localhost address, restrict access to trusted IPs:

```yaml
server:
  host: "0.0.0.0"
  port: 12345
  allowed_ips:
    - "192.168.1.0/24"
    - "10.0.0.5"
```

Supports CIDR notation and bare IP addresses. When `host` is `127.0.0.1` or `localhost`, the whitelist is ignored (even if configured).

### Model Map

Map client model names to upstream models:

```yaml
routes:
  "ais-default":
    provider: "deepseek"
    default_model: "deepseek-chat"
    model_map:
      "claude-sonnet-4-5": "deepseek-chat"
      "gpt-4o": "deepseek-chat"
```

### Cross-Provider Routing

Use `provider|model` to route to a different provider within the same route:

```yaml
routes:
  "ais-default":
    provider: "minimax"
    default_model: "MiniMax-M2.5"
    scene_map:
      default: "MiniMax-M2.5"
      think: "deepseek|deepseek-chat"
      websearch: "zhipu|glm-4.7"
```

### Model Resolution Priority

1. **ModelMap** — exact model name match (case-insensitive)
2. **SceneMap** — scene detection (Anthropic protocol only)
3. **DefaultModel** — fallback

## CLI

```bash
ai-switch serve                   # Start in foreground
ai-switch serve -d                # Start as background daemon
ai-switch serve -c config.yaml    # Start with custom config
ai-switch stop                    # Stop the background daemon
ai-switch check -c config.yaml    # Validate config without starting
ai-switch version                 # Print version info
ai-switch update                  # Check for updates and download latest version
ai-switch update --apply          # Apply the downloaded update
ai-switch shortcut                # Create desktop shortcuts to start/stop ai-switch
ai-switch agent <route-key> claude # Launch Claude Code via ai-switch
ai-switch agent <route-key> codex  # Launch Codex CLI via ai-switch
```

Running without a subcommand defaults to `serve`:

```bash
ai-switch -c config.yaml          # Same as: ai-switch serve -c config.yaml
```

### Agent Launcher

Launch AI agents with environment variables auto-configured from a route key:

```bash
# Launch Claude Code
ai-switch agent my-route-key claude --continue

# Launch Codex CLI
ai-switch agent my-route-key codex --model o4-mini
```

This auto-configures environment variables and overrides the agent's own config (via `--settings` for Claude, `-c` for Codex) to ensure requests route through ai-switch using the route key. No manual configuration needed.

The route key serves as the API key. Agent args and exit codes are passed through.

### Config validation

```bash
$ ai-switch check -c config.yaml

Checking config.yaml ...

  Providers: 3
  Routes:    3
  Default:   ais-default

✓ Config is valid.
```

Exit codes: `0` = valid, `1` = has errors, `2` = warnings only.

## Admin UI

Open `http://localhost:12345` in your browser for a built-in dashboard to manage providers, routes, view usage statistics, and inspect request traces.

### Traces

Every request is recorded with full request/response details. Click any trace to inspect:

- **Raw viewer**: See the exact request and response payloads
- **Diff view**: Side-by-side comparison of request and response
- **TTFB waterfall**: Visualize time-to-first-byte and upstream latency

### Usage Stats

The stats page shows token usage broken down by provider and model, including cache token metrics, with daily trend charts.

## Build

```bash
make build      # fmt + vet + compile
make build-all  # build frontend + Go binary
make dev        # run in dev mode
make test       # run tests
make clean      # remove binary
```

## License

[MIT](LICENSE)
