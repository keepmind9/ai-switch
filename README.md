# llm-gateway

A lightweight local LLM gateway proxy that lets any AI CLI tool use third-party LLM APIs through a unified local endpoint.

**One binary, one config, any AI CLI → any LLM API.**

## Features

- **Multi-protocol**: Auto-detects client protocol (Responses API, Anthropic Messages, Chat Completions) and converts transparently
- **Zero-intrusion**: No changes to your CLI config files, just point `base_url` to the local proxy
- **Lightweight**: Pure Go, no external dependencies, single binary
- **Hot reload**: Update config without restart (`POST /api/reload` or `kill -HUP`)
- **Cross-platform**: macOS, Linux, Windows (pure Go SQLite, no CGO)
- **Scene routing**: Route Claude Code requests to different models based on request type (thinking, web search, background tasks)
- **Model mapping**: Map client model names to upstream model names at the route level
- **Multiple providers**: Pre-configure providers for quick switching

## Supported Protocols

| Endpoint | Protocol | Client Example |
|----------|----------|----------------|
| `/v1/responses` | OpenAI Responses API | Codex CLI |
| `/v1/messages` | Anthropic Messages | Claude Code |
| `/v1/chat/completions` | Chat Completions | Generic |

The gateway uses a hub-and-spoke architecture centered on Chat Completions. All protocol conversions route through the hub, supporting indirect paths like Responses → Anthropic or Anthropic → Responses.

## Quick Start

```bash
# Copy and edit config
cp config.example.yaml config.yaml

# Build and run
make build
./bin/server -c config.yaml

# Or run in dev mode
make dev
```

## Configuration

```yaml
server:
  host: "0.0.0.0"
  port: 12345

default_provider: "minimax"

providers:
  minimax:
    name: "MiniMax"
    base_url: "https://api.minimaxi.com"
    api_key: "${MINIMAX_API_KEY}"
    model: "MiniMax-M2.5"
    format: "chat"
    # Optional: strip <think/> blocks from responses
    # think_tag: "think"

routes:
  "gw-my-key":
    provider: "minimax"
    default_model: "MiniMax-M2.5"
    model_map:
      "claude-sonnet-4-5": "MiniMax-M2.5"
      "gpt-4o": "MiniMax-M2.5"
    scene_map:
      default: "MiniMax-M2.5"
      think: "MiniMax-M2.5"
      websearch: "MiniMax-M2.5"
      background: "MiniMax-M2.5"
```

Config loading priority: `-c` flag > `./config.yaml` > `~/.llm-gateway/config.yaml`

> **Note:** `base_url` with `/v1` suffix is auto-stripped on load to prevent double path issues.

### Providers

Providers define upstream LLM vendor information:

```yaml
providers:
  minimax:
    name: "MiniMax"
    base_url: "https://api.minimaxi.com"
    api_key: "${MINIMAX_API_KEY}"
    model: "MiniMax-M2.5"
    format: "chat"          # chat (default) | responses | anthropic
    think_tag: "think"      # optional: strip reasoning tags from responses
```

### Routes

Routes map gateway API keys to providers and models. The map key is the gateway API key that clients send for authentication.

```yaml
routes:
  "gw-my-key":
    provider: "minimax"
    default_model: "MiniMax-M2.5"
```

#### Model Resolution Priority

When a request arrives, the gateway resolves the upstream model in this order:

1. **ModelMap** — exact model name match (all protocols, case-insensitive)
2. **SceneMap** — heuristic scene detection (Anthropic protocol only)
3. **DefaultModel** — fallback

#### Model Map

Map client model names to upstream models. Works for all protocols, case-insensitive:

```yaml
routes:
  "gw-my-key":
    provider: "minimax"
    default_model: "MiniMax-M2.5"
    model_map:
      "claude-sonnet-4-5": "MiniMax-M2.5"
      "gpt-4o": "MiniMax-M2.5"
```

#### Scene Map (Claude Code)

When the gateway receives an Anthropic Messages request (from Claude Code), it can detect the request scene and route to different models. This is useful for optimizing cost and performance across different Claude Code usage patterns.

**Detected scenes:**

| Scene | Key | Detection | Claude Code Usage |
|-------|-----|-----------|-------------------|
| Long Context | `longContext` | Token count exceeds `long_context_threshold` (disabled by default) | Requests with very large context windows |
| Background | `background` | Model name contains "haiku" | Lightweight background tasks (Claude Code uses Haiku internally) |
| Web Search | `websearch` | Tools array contains `web_search_*` type | Web search tool invocations |
| Thinking | `think` | `thinking` field present in request | Plan mode, Think/UltraThink modes, extended reasoning |
| Image | `image` | User messages contain image content blocks | Image analysis tasks |
| Default | `default` | Fallback when no other scene matches | General coding, editing, and conversation |

**Detection priority:** `longContext` > `background` > `websearch` > `think` > `image` > `default`

```yaml
routes:
  "gw-claude":
    provider: "zhipu"
    default_model: "glm-5.1"
    long_context_threshold: 60000
    scene_map:
      default: "glm-5.1"
      think: "glm-5.1"
      websearch: "glm-4.7"
      background: "glm-4.5-air"
      longContext: "glm-5.1"
      image: "glm-4.7"
```

### Upstream Format

The `format` field tells the gateway what protocol the upstream API speaks:

- `chat` (default) — Standard OpenAI Chat Completions compatible
- `responses` — OpenAI Responses API
- `anthropic` — Anthropic Messages API

## Client Setup

> **Important**: You must configure the **full URL** (including path) for your AI CLI tool. The gateway identifies the client protocol by the request path (`/v1/messages`, `/v1/responses`, `/v1/chat/completions`). This is different from tools like `cc-switch` that only need a base URL.

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://localhost:12345/v1/messages
export ANTHROPIC_API_KEY=gw-your-route-key
```

### Codex CLI

```toml
[model_providers.proxy]
name = "llm-gateway"
base_url = "http://localhost:12345/v1"
api_key = "gw-your-route-key"
wire_api = "responses"
```

### Generic Chat Completions

Point any tool's `base_url` to `http://localhost:12345/v1`.

## Admin API

| Endpoint | Description |
|----------|-------------|
| `GET /admin/providers` | List providers |
| `POST /admin/providers` | Create provider |
| `PUT /admin/providers/:key` | Update provider |
| `DELETE /admin/providers/:key` | Delete provider |
| `GET /admin/routes` | List routes |
| `POST /admin/routes` | Create route |
| `PUT /admin/routes/:key` | Update route |
| `DELETE /admin/routes/:key` | Delete route |
| `POST /admin/routes/generate-key` | Generate a gateway API key |
| `GET /admin/presets` | List provider presets |
| `GET /admin/status` | Gateway status |

## Core API Endpoints

| Endpoint | Description |
|----------|-------------|
| `POST /v1/responses` | Responses API (Codex CLI) |
| `POST /v1/messages` | Anthropic Messages (Claude Code) |
| `POST /v1/chat/completions` | Chat Completions (generic) |
| `POST /api/reload` | Hot-reload configuration |
| `GET /health` | Health check |

## Build

```bash
make build   # fmt + vet + compile
make lint    # fmt + vet only
make dev     # go run dev mode
make test    # run tests
make clean   # remove binary
```

## Architecture

```
Client (Responses/Anthropic/Chat)
    ↓
llm-gateway (protocol detection + conversion)
    ↓
Upstream API (any format)
```

Hub-and-Spoke conversion:
- Responses ↔ Chat Completions ↔ Anthropic Messages
- Indirect paths chain through the Chat hub

## License

MIT
