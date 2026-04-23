# AI Full-Chain Integration Test Guide

This document describes how an AI agent (Claude Code, Copilot CLI, etc.) can automatically perform end-to-end integration testing of ai-switch with real upstream providers and real CLI tools.

## Prerequisites

### 1. Fill in Provider Credentials

Edit `tests/e2e/testdata/config.real.yaml` — replace all `<FILL>` placeholders:

```yaml
providers:
  minimax-chat:
    base_url: "https://api.minimaxi.com/v1"
    api_key: "${MINIMAX_API_KEY}"
    format: "chat"
    models: ["MiniMax-M2.5"]

  minimax-anthropic:
    base_url: "https://api.minimaxi.com/anthropic"
    api_key: "${MINIMAX_API_KEY}"
    format: "anthropic"
    models: ["MiniMax-M2.5"]

  glm-chat:
    base_url: "https://open.bigmodel.cn/api/paas/v1"
    api_key: "${GLM_API_KEY}"
    format: "chat"
    models: ["glm-4-plus"]

  glm-anthropic:
    base_url: "https://open.bigmodel.cn/api/anthropic"
    api_key: "${GLM_API_KEY}"
    format: "anthropic"
    models: ["glm-4-plus"]

routes:
  "e2e-minimax-chat":
    provider: "minimax-chat"
    default_model: "MiniMax-M2.5"
  "e2e-minimax-anthropic":
    provider: "minimax-anthropic"
    default_model: "MiniMax-M2.5"
  "e2e-glm-chat":
    provider: "glm-chat"
    default_model: "glm-4-plus"
  "e2e-glm-anthropic":
    provider: "glm-anthropic"
    default_model: "glm-4-plus"
```

Set environment variables:

```bash
export MINIMAX_API_KEY="<your-key>"
export GLM_API_KEY="<your-key>"
```

### 2. CLI Tools Installation

At least one of:

| CLI | Install Check | Protocol |
|-----|--------------|----------|
| Claude Code | `which claude` | Anthropic `/v1/messages` |
| Codex | `which codex` | Responses `/v1/responses` |
| OpenCode | `which opencode` | Chat `/v1/chat/completions` |

Tests auto-skip if a CLI is not installed.

## Test Execution Steps

### Step 1: Build ai-switch

```bash
cd /data/app/workspace/me/ai-switch
go build -o /tmp/ai-switch-e2e ./cmd/server
```

Verify: `/tmp/ai-switch-e2e version` should print version info.

### Step 2: Start ai-switch with test config

```bash
# Pick an available port
E2E_PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()")

# Update config port
sed -i "s/port: 0/port: $E2E_PORT/" tests/e2e/testdata/config.real.yaml

# Start server
/tmp/ai-switch-e2e -c tests/e2e/testdata/config.real.yaml &
SERVER_PID=$!
```

Wait for startup:

```bash
for i in $(seq 1 20); do
  curl -sf http://127.0.0.1:$E2E_PORT/health && break
  sleep 0.5
done
```

Verify: `curl http://127.0.0.1:$E2E_PORT/health` returns 200.

### Step 3: Run CLI Tests

For each installed CLI, run the test matrix against all 4 providers.

#### Claude Code (Anthropic protocol → 4 providers)

```bash
# Test against each route key
for ROUTE in e2e-minimax-chat e2e-minimax-anthropic e2e-glm-chat e2e-glm-anthropic; do
  echo "=== Claude Code → $ROUTE ==="
  ANTHROPIC_API_KEY="$ROUTE" \
  ANTHROPIC_BASE_URL="http://127.0.0.1:$E2E_PORT" \
  claude -p "Reply with exactly one word: PONG" \
    --dangerously-skip-permissions \
    --output-format json 2>/dev/null | head -5
  echo ""
done
```

**Pass criteria**: Each output contains "PONG" (case-insensitive).

#### Codex (Responses protocol → 4 providers)

```bash
for ROUTE in e2e-minimax-chat e2e-minimax-anthropic e2e-glm-chat e2e-glm-anthropic; do
  echo "=== Codex → $ROUTE ==="
  OPENAI_API_KEY="$ROUTE" \
  OPENAI_BASE_URL="http://127.0.0.1:$E2E_PORT/v1" \
  codex exec "Reply with exactly one word: PONG" --full-auto 2>&1 | head -5
  echo ""
done
```

**Pass criteria**: Each output contains "PONG".

#### OpenCode (Chat protocol → 4 providers)

OpenCode uses a config file. Generate it:

```bash
cat > /tmp/opencode-e2e-config.json <<EOF
{
  "provider": "openai",
  "baseURL": "http://127.0.0.1:$E2E_PORT/v1",
  "apiKey": "ROUTE_KEY_HERE"
}
EOF
```

Then for each route:

```bash
for ROUTE in e2e-minimax-chat e2e-minimax-anthropic e2e-glm-chat e2e-glm-anthropic; do
  echo "=== OpenCode → $ROUTE ==="
  # Update apiKey in config
  sed -i "s/ROUTE_KEY_HERE/$ROUTE/" /tmp/opencode-e2e-config.json
  echo "Reply with exactly one word: PONG" | opencode --config /tmp/opencode-e2e-config.json 2>&1 | head -5
  # Reset for next iteration
  sed -i "s/$ROUTE/ROUTE_KEY_HERE/" /tmp/opencode-e2e-config.json
  echo ""
done
```

**Pass criteria**: Each output contains "PONG".

### Step 4: Cleanup

```bash
kill $SERVER_PID 2>/dev/null
# Restore config port
sed -i "s/port: $E2E_PORT/port: 0/" tests/e2e/testdata/config.real.yaml
```

## Full Test Matrix

| CLI | Protocol | Provider | Route Key | Conversion |
|-----|----------|----------|-----------|------------|
| Claude Code | Anthropic | MiniMax Chat | e2e-minimax-chat | Anthropic→Chat |
| Claude Code | Anthropic | MiniMax Anthropic | e2e-minimax-anthropic | Anthropic→Anthropic (passthrough) |
| Claude Code | Anthropic | GLM Chat | e2e-glm-chat | Anthropic→Chat |
| Claude Code | Anthropic | GLM Anthropic | e2e-glm-anthropic | Anthropic→Anthropic (passthrough) |
| Codex | Responses | MiniMax Chat | e2e-minimax-chat | Responses→Chat |
| Codex | Responses | MiniMax Anthropic | e2e-minimax-anthropic | Responses→Anthropic |
| Codex | Responses | GLM Chat | e2e-glm-chat | Responses→Chat |
| Codex | Responses | GLM Anthropic | e2e-glm-anthropic | Responses→Anthropic |
| OpenCode | Chat | MiniMax Chat | e2e-minimax-chat | Chat→Chat (passthrough) |
| OpenCode | Chat | MiniMax Anthropic | e2e-minimax-anthropic | Chat→Anthropic |
| OpenCode | Chat | GLM Chat | e2e-glm-chat | Chat→Chat (passthrough) |
| OpenCode | Chat | GLM Anthropic | e2e-glm-anthropic | Chat→Anthropic |

Total: 3 CLIs × 4 providers = **12 test scenarios**.

## Failure Diagnosis

### ai-switch won't start
- Check port conflict: `ss -tlnp | grep $E2E_PORT`
- Check config syntax: `/tmp/ai-switch-e2e check -c tests/e2e/testdata/config.real.yaml`
- Check logs in terminal output

### CLI connection refused
- Verify ai-switch is running: `curl http://127.0.0.1:$E2E_PORT/health`
- Verify `BASE_URL` includes scheme (`http://`) and correct port
- For Codex: `OPENAI_BASE_URL` should NOT include `/v1` at the end (Codex adds it)

### CLI returns error
- Check ai-switch logs for upstream errors
- Verify API keys are valid
- Verify model names match provider support
- Test upstream directly: `curl -H "Authorization: Bearer $API_KEY" <upstream_url>/models`

### Cross-protocol conversion issues
- Run Mode A protocol tests first: `go test ./tests/e2e/ -v -run TestProtocolMatrix -short`
- These isolate conversion logic without real CLIs or upstreams
