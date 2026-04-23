# ai-switch

[![Go Report Card](https://goreportcard.com/badge/github.com/keepmind9/ai-switch)](https://goreportcard.com/report/github.com/keepmind9/ai-switch) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**English** | [中文](README_zh.md)

一个轻量级本地代理，让任何 AI CLI 工具（Claude Code、Codex CLI 等）通过统一的本地端点使用第三方 LLM API。

**一个二进制，一个配置，任意 AI CLI → 任意 LLM API。**

## 特性

- **多协议自动转换**：自动检测客户端协议（Responses API、Anthropic Messages、Chat Completions）并透明转换
- **零侵入**：无需修改 CLI 工具的配置文件，只需将 `base_url` 指向本地代理
- **场景路由**：根据请求类型（思考、联网搜索、后台任务）将 Claude Code 的请求路由到不同模型
- **模型映射**：在路由层将客户端模型名映射为上游模型名
- **跨 Provider 路由**：不同场景路由到不同 Provider（如思考 → DeepSeek，联网 → 智谱）
- **热重载**：无需重启即可更新配置（`POST /api/reload` 或 `kill -HUP`）
- **管理面板**：内置 Web 管理界面，可视化管理 Provider 和 Route
- **轻量**：纯 Go 实现，单二进制，无 CGO 依赖

## 安装

### 一键安装（推荐）

```bash
# Linux / macOS
curl -sL https://raw.githubusercontent.com/keepmind9/ai-switch/main/scripts/install.sh | bash

# Windows (PowerShell)
irm https://raw.githubusercontent.com/keepmind9/ai-switch/main/scripts/install.ps1 | iex
```

自动下载最新版本，安装到 `~/.local/bin`，并添加到 PATH。

### 从源码构建

```bash
git clone https://github.com/keepmind9/ai-switch.git
cd ai-switch
make build-all   # 构建前端 + Go 二进制（包含管理面板）
```

> 如不需要管理面板，可使用 `make build`（仅构建 Go 二进制，速度更快）。

## 快速开始

### 1. 启动服务

```bash
ai-switch serve
```

无需配置文件——首次运行自动创建 `~/.ai-switch/config.yaml`。

### 2. 通过管理面板配置

在浏览器打开 `http://localhost:12345`，添加 Provider 和 Route。

### 3. 配置你的 CLI 工具

**Claude Code：**

```bash
export ANTHROPIC_BASE_URL=http://localhost:12345
export ANTHROPIC_API_KEY=<route-key>
```

**Codex CLI：**

```toml
[model_providers.proxy]
name = "ai-switch"
base_url = "http://localhost:12345/v1"
api_key = "ais-default"
wire_api = "responses"
```

**任何 OpenAI 兼容工具：**

```bash
export OPENAI_BASE_URL=http://localhost:12345/v1
export OPENAI_API_KEY=<route-key>
```

完成！你的 CLI 工具将通过 ai-switch 路由到你配置的 Provider。

## 工作原理

```
Claude Code ──→ ai-switch ──→ DeepSeek (chat)
Codex CLI  ──→          ──→ 智谱    (anthropic)
任意工具    ──→          ──→ MiniMax  (chat)
```

ai-switch 位于你的 CLI 工具和上游 LLM Provider 之间，它会：

- 自动检测客户端协议（Anthropic / Responses / Chat）
- 根据 API Key（路由 Key）将请求路由到正确的 Provider
- 在需要时进行协议转换（如 Anthropic → Chat Completions）
- 检测请求场景（思考、联网搜索等）实现智能路由

路由 Key（上例中的 `<route-key>`）同时用作认证的 API Key 和路由标识。

## 配置说明

### Provider

定义上游 LLM 服务商连接：

```yaml
providers:
  deepseek:
    name: "DeepSeek"
    base_url: "https://api.deepseek.com/v1"
    api_key: "${DEEPSEEK_API_KEY}"    # 支持 ${ENV_VAR} 环境变量展开
    format: "chat"                     # chat（默认）| responses | anthropic
    think_tag: "think"                 # 可选：去除响应中的推理标签
    models:                            # 可选：用于配置校验警告
      - "deepseek-chat"
      - "deepseek-reasoner"
```

### Route

Route 将 API Key 映射到 Provider 和模型：

```yaml
routes:
  "ais-default":
    provider: "deepseek"
    default_model: "deepseek-chat"
```

### 场景映射（Scene Map）

根据 Claude Code 正在执行的操作类型路由到不同模型：

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

| 场景 | Key | 检测方式 |
|------|-----|---------|
| 长上下文 | `longContext` | Token 数超过 `long_context_threshold` |
| 后台任务 | `background` | 模型名包含 "haiku" |
| 联网搜索 | `websearch` | Tools 包含 `web_search_*` 类型 |
| 思考 | `think` | 请求包含 `thinking` 字段 |
| 图片 | `image` | 用户消息包含图片内容 |
| 默认 | `default` | 兜底 |

优先级：`longContext` > `background` > `websearch` > `think` > `image` > `default`

### 默认路由（Default Routes）

控制请求没有匹配 API Key 时使用哪条路由：

```yaml
default_route: "ais-default"              # 全局兜底
default_anthropic_route: "ais-zhipu"      # /v1/messages（Claude Code）
default_responses_route: "ais-default"    # /v1/responses（Codex CLI）
default_chat_route: "ais-default"         # /v1/chat/completions
```

**路由优先级：** route key 匹配 > 协议级默认 > 全局 `default_route`

所有字段均可选。未设置的协议级默认会回退到 `default_route`。

### 日志保留（Log Retention）

控制日志文件保留天数（默认 30 天）：

```yaml
log_retention_days: 7
```

日志文件存储在 `~/.ai-switch/logs/`。

### 模型映射（Model Map）

将客户端模型名映射为上游模型：

```yaml
routes:
  "ais-default":
    provider: "deepseek"
    default_model: "deepseek-chat"
    model_map:
      "claude-sonnet-4-5": "deepseek-chat"
      "gpt-4o": "deepseek-chat"
```

### 跨 Provider 路由

使用 `provider:model` 格式在同一 Route 中路由到其他 Provider：

```yaml
routes:
  "ais-default":
    provider: "minimax"
    default_model: "MiniMax-M2.5"
    scene_map:
      default: "MiniMax-M2.5"
      think: "deepseek:deepseek-chat"
      websearch: "zhipu:glm-4.7"
```

### 模型解析优先级

1. **ModelMap** — 精确模型名匹配（不区分大小写）
2. **SceneMap** — 场景检测（仅 Anthropic 协议）
3. **DefaultModel** — 兜底

## CLI 命令

```bash
ai-switch serve                   # 前台启动
ai-switch serve -d                # 后台守护进程启动
ai-switch serve -c config.yaml    # 指定配置文件启动
ai-switch stop                    # 停止后台守护进程
ai-switch check -c config.yaml    # 校验配置文件
ai-switch version                 # 查看版本信息
```

不带子命令时默认执行 `serve`：

```bash
ai-switch -c config.yaml          # 等同于：ai-switch serve -c config.yaml
```

### 配置校验

```bash
$ ai-switch check -c config.yaml

Checking config.yaml ...

  Providers: 3
  Routes:    3
  Default:   ais-default

✓ Config is valid.
```

退出码：`0` = 有效，`1` = 有错误，`2` = 仅警告。

## 管理面板

在浏览器打开 `http://localhost:12345`，使用内置管理面板管理 Provider、Route，以及查看用量统计。

## 构建

```bash
make build      # 格式化 + 静态检查 + 编译
make build-all  # 构建前端 + Go 二进制
make dev        # 开发模式运行
make test       # 运行测试
make clean      # 清理构建产物
```

### Release 构建

```bash
./scripts/release.sh           # 自动检测 git tag 作为版本号
./scripts/release.sh v0.1.0    # 指定版本号
```

产物输出到 `dist/` 目录，包含跨平台二进制和 SHA256 校验文件。

## 许可证

[MIT](LICENSE)
