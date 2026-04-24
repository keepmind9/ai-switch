# Anthropic Messages API

> Official docs: https://docs.anthropic.com/en/api/messages

## Endpoint

```
POST /v1/messages
```

## Auth

```
x-api-key: <api-key>
anthropic-version: 2023-06-01
```

## Request Fields (project scope)

| Field | Type | Note |
|-------|------|------|
| `model` | string | Model ID |
| `messages` | []AnthropicMessage | Conversation history |
| `system` | any | String or structured system prompt |
| `max_tokens` | int | Max output tokens |
| `temperature` | float64 | Sampling temperature |
| `top_p` | float64 | Nucleus sampling |
| `stream` | bool | Enable SSE streaming |
| `tools` | []AnthropicTool | Available tools |
| `tool_choice` | any | Tool selection policy |
| `metadata` | map | Custom metadata |

## Streaming

SSE events with typed events: `message_start`, `content_block_start`, `content_block_delta`, `content_block_stop`, `message_delta`, `message_stop`.

## Tool Use

- Request: `tools[]` with `name`, `description`, `input_schema`
- Response: `tool_use` content block with `id`, `name`, `input`
- Follow-up: `tool_result` content block with `tool_use_id`, `content`

## Token Counting

```
POST /v1/messages/count_tokens
```

Local endpoint for counting tokens without calling upstream.
