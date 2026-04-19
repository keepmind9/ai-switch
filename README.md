# llm-gateway

A lightweight local LLM gateway proxy that lets any AI CLI tool use third-party LLM APIs through a unified local endpoint.

**One binary, one config, any AI CLI → any LLM API.**

## Features

- **Multi-protocol**: Auto-detects client protocol (Responses API, Anthropic Messages, Chat Completions) and converts transparently
- **Zero-intrusion**: No changes to your CLI config files, just point `base_url` to the local proxy
- **Lightweight**: Pure Go, no external dependencies, single binary
- **Hot reload**: Update config without restart (`POST /api/reload` or `kill -HUP`)
- **Cross-platform**: macOS, Linux, Windows (pure Go SQLite, no CGO)
- **Model mapping**: Map client model names to upstream model names
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
    # base_url with /v1 suffix is OK — it will be auto-stripped
    base_url: "https://api.minimaxi.com"
    api_key: "${MINIMAX_API_KEY}"
    model: "MiniMax-M2.5"
    format: "chat"
    model_map:
      "claude-sonnet-4-5": "MiniMax-M2.5"
      "gpt-4o": "MiniMax-M2.5"

routes:
  # Map key IS the gateway API key used by clients
  "gw-my-key":
    provider: "minimax"
    default_model: "MiniMax-M2.5"
```

Config loading priority: `-c` flag > `./config.yaml` > `~/.llm-gateway/config.yaml`

> **Note:** `base_url` with `/v1` suffix is auto-stripped on load to prevent double path issues.

### Routes

The map key in `routes` is the gateway API key that clients send for authentication. When a client sends `Authorization: Bearer <key>`, the gateway looks up the matching route.

### Upstream Format

The `format` field tells the gateway what protocol the upstream API speaks:

- `chat` (default) — Standard OpenAI Chat Completions compatible
- `responses` — OpenAI Responses API
- `anthropic` — Anthropic Messages API

## Client Setup

> **Important**: You must configure the **full URL** (including path) for your AI CLI tool. The gateway identifies the client protocol by the request path (`/v1/messages`, `/v1/responses`, `/v1/chat/completions`). This is different from tools like `cc-switch` that only need a base URL.

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://localhost:12345
export ANTHROPIC_API_KEY=any
```

### Codex CLI

```toml
[model_providers.proxy]
name = "llm-gateway"
base_url = "http://localhost:12345/v1"
api_key = "your-gateway-key"
wire_api = "responses"
```

### Generic Chat Completions

Point any tool's `base_url` to `http://localhost:12345/v1`.

## API Endpoints

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
