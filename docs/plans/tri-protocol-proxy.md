# Implementation Plan: LLM Gateway

## Context

**llm-gateway** 是一个轻量级本地代理，让各种 AI CLI 工具（Claude Code、Codex CLI、Cursor 等）通过统一的本地网关使用第三方 LLM API。

**核心定位**: 一个二进制文件，改一行 base URL，任何 AI CLI 都能用第三方 LLM。

**项目目标**:
- 零侵入：不改用户 CLI 配置文件，只改 base URL 指向本地代理
- 轻量：纯 Go 实现，无外部依赖服务
- 多协议：自动识别客户端协议并转换
- 跨平台：兼容 macOS、Linux、Windows
- 开源社区：吸引模型厂商和 token 中转站赞助

**跨平台约束**:
- 路径用 `os.UserHomeDir()` + `filepath.Join()`，不硬编码 `~` 或 `/`
- SQLite 选用纯 Go 实现（`modernc.org/sqlite`），无 CGO 依赖，Windows 直接编译
- 文件操作用 `os` / `path/filepath` 标准库，不依赖 shell 命令

## Architecture: Hub-and-Spoke Conversion

以 Chat Completions 为中心 hub，所有协议通过它中转：

```
Anthropic Messages ←→ Chat Completions ←→ Responses API
```

只需实现 3 对转换器（而非 6 对），间接路径通过链式调用实现。

**Gemini** 通过 Chat Completions 兼容层覆盖，不单独实现转换器。

## Protocol Auto-Detection

根据请求路径自动判断客户端协议，无需额外配置：

| Endpoint | Protocol | Client Example |
|----------|----------|----------------|
| `/v1/responses` | Responses API | Codex CLI |
| `/v1/messages` | Anthropic Messages | Claude Code |
| `/v1/chat/completions` | Chat Completions | 通用 |

上游协议通过 config 的 `upstream.format` 指定，自动路由转换。

## SDK Strategy

优先使用官方 SDK，不自己实现协议细节：

| 功能 | SDK | 包 |
|------|-----|----|
| Chat Completions + Responses API | OpenAI 官方 Go SDK | `github.com/openai/openai-go` |
| Anthropic Messages API | Anthropic 官方 Go SDK | `github.com/anthropics/anthropic-sdk-go` |
| SSE 流式解析 | SDK 内置流式处理 | 同上 |
| 上游请求（auth/retry/错误） | SDK 自动处理 | 同上 |

**用 SDK 的部分：**
- 类型定义 — 直接引用 SDK struct，不自己定义 request/response
- 上游请求 — SDK 处理 auth header、重试、错误解析
- SSE 流式 — SDK 自带流式迭代器，不用手写 scanner

**自己写的部分：**
- 协议转换逻辑 — SDK 之间没有转换函数，这是核心价值
- 路由分发 — 根据路径和配置决定调用哪个 SDK
- 原有 `internal/types` 包可大幅简化甚至移除

## Project Structure

```
internal/
  config/
    config.go              (MODIFY - add format, model_map, providers)
  converter/
    responses_chat.go      (NEW - Responses↔Chat conversion logic)
    anthropic_chat.go      (NEW - Anthropic↔Chat conversion logic)
    converter.go           (MODIFY - dispatcher routing)
  handler/
    handler.go             (MODIFY - shared proxy logic, routing)
    responses.go           (NEW - /v1/responses endpoint)
    anthropic.go           (NEW - /v1/messages endpoint)
    chat.go                (NEW - /v1/chat/completions endpoint)
web/
  static/                  (NEW - embed.FS static files)
    index.html             (sponsor cards + config UI)
cmd/server/
  main.go                  (MODIFY - pass full config to handler)
```

## Phase 1: Foundation

### 1.1 Config - add `format`, `model_map`, `providers`

**File: `internal/config/config.go`**

```go
type Config struct {
    Server   ServerConfig        `mapstructure:"server"`
    Upstream UpstreamConfig      `mapstructure:"upstream"`
    Providers map[string]Provider `mapstructure:"providers"`
}

type UpstreamConfig struct {
    BaseURL  string            `mapstructure:"base_url"`
    APIKey   string            `mapstructure:"api_key"`
    Model    string            `mapstructure:"model"`
    Format   string            `mapstructure:"format"`    // chat | responses | anthropic, default: "chat"
    ModelMap map[string]string `mapstructure:"model_map"` // model name mapping
}

type Provider struct {
    Name     string            `mapstructure:"name"`
    BaseURL  string            `mapstructure:"base_url"`
    APIKey   string            `mapstructure:"api_key"`
    Model    string            `mapstructure:"model"`
    Format   string            `mapstructure:"format"`
    ModelMap map[string]string `mapstructure:"model_map"`
    LogoURL  string            `mapstructure:"logo_url"`
    Sponsor  bool              `mapstructure:"sponsor"`
}
```

- `format` 默认 `"chat"`，校验 `["chat", "responses", "anthropic"]`
- `model_map` 支持客户端模型名 → 上游模型名映射，未匹配则原样透传
- `providers` 预设赞助商配置，UI 页面可一键切换

### 1.3 数据目录

运行时数据统一存放在 `~/.llm-gateway/`，首次启动自动创建：

```
~/.llm-gateway/
  config.yaml      # 配置文件
  usage.db         # SQLite 统计数据库
  logs/
    server.log     # 运行日志
```

配置加载优先级：
1. `-c` 参数指定路径（已有）
2. 当前目录 `./config.yaml`
3. 默认 `~/.llm-gateway/config.yaml`

### 1.2 Graceful Shutdown & Config Hot Reload

- `os/signal` 监听 `SIGINT`/`SIGTERM`，优雅关闭：等待 SQLite 写完、in-flight 请求完成
- Config 热重载：`POST /api/reload` 触发，Unix 额外支持 `kill -HUP <pid>`
- In-flight 请求用旧配置完成，新请求用新配置
- 不自动 watch 文件变更，避免跨平台文件监听的复杂性

### 1.3 引入官方 SDK 依赖

```bash
go get github.com/openai/openai-go
go get github.com/anthropics/anthropic-sdk-go
```

类型定义直接引用 SDK struct，移除 `internal/types` 中对应的自定义类型：
- Chat Completions / Responses 类型 → `openai-go` 包
- Anthropic Messages 类型 → `anthropic-sdk-go` 包
- `internal/types/types.go` 仅保留少量共享辅助类型（如 SSEEvent），或完全移除

## Phase 2: Anthropic ↔ Chat Converter

### 2.1 Anthropic → Chat

**File: `internal/converter/anthropic_chat.go`**
- `AnthropicToChat(req)`:
  - `system` (string or array) → system message
  - `messages[].content` (string or content blocks) → flattened text
  - `max_tokens`, `temperature`, `top_p` direct mapping
- `ChatToAnthropic(resp, model)`:
  - `choices[0].message.content` → `[{"type":"text","text":"..."}]`
  - `stop` → `end_turn`, `length` → `max_tokens`

### 2.2 Chat → Anthropic

- `ChatRequestToAnthropic(req)`:
  - System messages → Anthropic `system` field
  - Other messages direct mapping, max_tokens defaults to 4096
- `AnthropicResponseToChat(resp)`:
  - Content blocks joined into single string
  - `end_turn` → `stop`, `max_tokens` → `length`

### 2.3 Existing Responses ↔ Chat

**File: `internal/converter/responses_chat.go`**
- Move existing `ResponsesToChat` and `ChatToResponses` logic

## Phase 3: SSE Streaming Converters

利用 SDK 内置流式迭代器，将现有 handler.go 中的手写 scanner 替换为 SDK stream API。

### 3.1 Chat Stream → Anthropic SSE

**File: `internal/converter/stream_anthropic.go`**

使用 `openai-go` 的 stream iterator 读取 Chat 流式响应，转换为 Anthropic SSE 事件序列：
1. First chunk → `message_start`
2. First text content → `content_block_start` + `content_block_delta`
3. Each delta → `content_block_delta` (text_delta)
4. `finish_reason` → `content_block_stop` + `message_delta` + `message_stop`

### 3.2 Chat Stream → Responses SSE

**File: `internal/converter/stream_responses.go`**

使用 `openai-go` stream iterator，重构现有 handler.go 中的手写 scanner 为 SDK 驱动。

### 3.3 Anthropic Stream → Chat SSE

使用 `anthropic-sdk-go` 的 stream iterator 读取 Anthropic 流式响应，转换为 Chat SSE：
- `message_start` → Chat chunk with role
- `content_block_delta` (text_delta) → Chat delta content
- `message_delta` → Chat finish_reason
- `message_stop` → `[DONE]`

### 3.4 Responses Stream → Chat SSE

使用 `openai-go` 的 Responses stream iterator：
- `response.output_text.delta` → Chat delta content
- `response.completed` → Chat finish_reason + `[DONE]`

## Phase 4: Handler Refactor

### 4.1 Shared proxy logic

**File: `internal/handler/handler.go`**
- `Handler` struct holds `*config.Config`, `*http.Client`
- `forwardRequest(c, upstreamPath, body, isStreaming)` - shared upstream request logic
- `setUpstreamHeaders(req)` - set auth headers based on upstream format
- `streamProxy(c, resp, convertFn)` - generic SSE scanning loop
- `resolveModel(model)` - resolve model name via model_map
- `RegisterRoutes` adds all endpoints including static files

### 4.2 Endpoint handlers

Each handler follows the same pattern:
1. Parse client request
2. Convert to upstream format (or passthrough if same format)
3. Forward to upstream
4. Convert response back (or passthrough)

**File: `internal/handler/responses.go`** - `/v1/responses` endpoint
**File: `internal/handler/anthropic.go`** - `/v1/messages` endpoint
**File: `internal/handler/chat.go`** - `/v1/chat/completions` endpoint

### 4.3 Header handling

| Upstream | Auth Header | Extra Headers |
|----------|------------|---------------|
| chat | `Authorization: Bearer` | - |
| responses | `Authorization: Bearer` | - |
| anthropic | `x-api-key` | `anthropic-version: 2023-06-01` |

## Phase 5: Converter Dispatcher

**File: `internal/converter/converter.go`**
- `ConvertRequest(clientFormat, body)` → (upstreamPath, upstreamBody, model, isStreaming)
- `ConvertResponse(clientFormat, model, body)` → convertedBody
- Route based on `upstreamFormat` and `clientFormat`

Indirect paths chain through Chat:
- Responses → Anthropic = Responses → Chat → Anthropic
- Anthropic → Responses = Anthropic → Chat → Responses

## Phase 6: Config & Docs

### 6.1 Config example

```yaml
server:
  host: "0.0.0.0"
  port: 12345

upstream:
  base_url: "https://api.minimaxi.com/v1"
  api_key: "${API_KEY}"
  model: "MiniMax-M2.5"
  format: "chat"
  model_map:
    "claude-sonnet-4-5": "MiniMax-M2.5"
    "gpt-4o": "MiniMax-M2.5"

providers:
  minimax:
    name: "MiniMax"
    base_url: "https://api.minimaxi.com/v1"
    api_key: "${MINIMAX_API_KEY}"
    model: "MiniMax-M2.5"
    format: "chat"
    logo_url: "https://..."
    sponsor: true
  deepseek:
    name: "DeepSeek"
    base_url: "https://api.deepseek.com/v1"
    api_key: "${DEEPSEEK_API_KEY}"
    model: "deepseek-chat"
    format: "chat"
    logo_url: "https://..."
    sponsor: true
```

### 6.2 README update

项目介绍、slogan、三协议使用示例、赞助商说明。

## Phase 7: Usage Statistics

### 7.1 Gin middleware 零侵入统计

**File: `internal/middleware/usage.go`**

```go
func UsageMiddleware(store *UsageStore) gin.HandlerFunc {
    return func(c *gin.Context) {
        wrap := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
        c.Writer = wrap

        c.Next()

        usage := extractUsage(wrap.body.Bytes())
        if usage != nil {
            store.AsyncRecord(provider, model, usage)
        }
    }
}
```

- 零侵入：不改 converter、handler、SDK 任何代码
- 协议无关：不管哪个 SDK，响应里都有 usage 字段，统一提取
- 异步写入：通过 channel，不阻塞请求

### 7.2 SQLite 存储（纯 Go，无 CGO）

依赖：`modernc.org/sqlite`

```sql
CREATE TABLE usage (
    provider              TEXT,
    model                 TEXT,
    date                  TEXT,
    requests              INTEGER DEFAULT 0,
    input_tokens          INTEGER DEFAULT 0,
    output_tokens         INTEGER DEFAULT 0,
    cache_creation_tokens INTEGER DEFAULT 0,
    cache_read_tokens     INTEGER DEFAULT 0,
    total_tokens          INTEGER DEFAULT 0,
    PRIMARY KEY (provider, model, date)
);
```

- 按天聚合 UPSERT，不存逐条记录
- 数据量 = 厂商数 × 模型数 × 天数，跑一年也就几千行
- Cache token 来源：Anthropic `cache_creation_input_tokens` / `cache_read_input_tokens`，OpenAI `prompt_tokens_details.cached_tokens`

### 7.3 API 端点

- `GET /api/stats` — 按 provider/model/时间范围查询统计
- `GET /api/stats?group=daily` — 按天粒度
- `GET /api/stats?group=monthly` — 按月汇总

## Phase 8: Sponsor UI Page

### 7.1 Static page via embed.FS

**File: `web/static/index.html`**

纯静态单页 HTML，Go 编译时嵌入：
- 顶部：项目 logo + slogan "One binary, one config, any AI CLI → any LLM API"
- 赞助商卡片：logo、名称、支持的模型列表、点击复制配置命令
- 用量统计面板：按 provider/model 展示 token 用量、cache 命中率、趋势图
- 配置状态：显示当前活跃上游和模型
- 可选：简易表单编辑 config（修改后写回 YAML，重启生效）

### 8.2 Handler integration

**File: `internal/handler/handler.go`**
- `RegisterRoutes` 添加 `/ui` 和 `/ui/assets/*` 路由，serve embed.FS
- `/api/status` 返回当前配置状态 JSON
- `/api/providers` 返回赞助商列表 JSON

## Verification

1. `make build` - fmt + vet + compile passes
2. Start proxy with `format: "chat"`, test:
   - `curl /v1/responses` (Codex → Chat upstream) - existing test
   - `curl /v1/messages` (Claude Code → Chat upstream) - new test
   - `curl /v1/chat/completions` (passthrough) - new test
3. Start proxy with `format: "anthropic"`, test `/v1/responses` and `/v1/chat/completions`
4. Verify SSE streaming for each path with `curl -N`
5. Model mapping: send request with mapped model name, verify upstream receives correct model
6. Usage stats: send requests, verify SQLite records updated, `GET /api/stats` returns data
7. UI page: `open http://localhost:12345/ui` shows sponsor cards, usage stats, and config
