# responses-to-chat-proxy

将 OpenAI Responses API 请求转换为 Chat Completions API 格式的反向代理。

解决 Codex CLI 等工具使用 Responses API，但上游模型服务仅支持 Chat Completions API 的兼容问题。支持任何兼容 OpenAI Chat Completions 格式的上游服务（MiniMax、GLM、DeepSeek 等）。

## 快速开始

```bash
# 修改 config.yaml
# 编辑 config.yaml 中的 upstream 配置为你使用的服务

# 构建
make build

# 运行
./bin/server -c config.yaml
```

## 配置

```yaml
server:
  host: "0.0.0.0"
  port: 8080

upstream:
  base_url: "https://api.minimaxi.com/v1"  # 上游服务地址
  api_key: "${API_KEY}"                     # 支持环境变量
  model: "MiniMax-M2.5"                     # 默认模型
```

切换上游服务只需修改 `base_url`、`api_key` 和 `model`，无需改代码。

## Codex 配置

```toml
model_provider = "codex"
model = "MiniMax-M2.5"
model_reasoning_effort = "high"
disable_response_storage = true
preferred_auth_method = "apikey"

[model_providers.codex]
name = "responses-to-chat-proxy"
base_url = "http://localhost:8080/v1"
api_key = "dummy"  # 不验证
wire_api = "responses"
```

## API 端点

- `POST /v1/responses` - Responses API 请求转换（支持流式和非流式）
- `GET /health` - 健康检查

## 构建

```bash
make build   # fmt + vet + 编译
make lint    # 仅 fmt + vet
make dev     # go run 开发模式
make clean   # 清理构建产物
```
