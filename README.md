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

upstream:
  base_url: "https://api.minimaxi.com/v1"
  api_key: "${API_KEY}"
  model: "MiniMax-M2.5"
  format: "chat"        # chat | responses | anthropic
  model_map:
    "claude-sonnet-4-5": "MiniMax-M2.5"
    "gpt-4o": "MiniMax-M2.5"

providers:
  deepseek:
    name: "DeepSeek"
    base_url: "https://api.deepseek.com/v1"
    api_key: "${DEEPSEEK_API_KEY}"
    model: "deepseek-chat"
    format: "chat"
    sponsor: true
```

Config loading priority: `-c` flag > `./config.yaml` > `~/.llm-gateway/config.yaml`

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
api_key = "any"
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
