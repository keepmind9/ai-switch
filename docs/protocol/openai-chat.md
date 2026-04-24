# OpenAI Chat Completions API

> Official docs: https://platform.openai.com/docs/api-reference/chat

## Endpoint

```
POST /v1/chat/completions
```

## Auth

```
Authorization: Bearer <api-key>
```

## Request Fields (project scope)

| Field | Type | Note |
|-------|------|------|
| `model` | string | Model ID |
| `messages` | []ChatMessage | Conversation history |
| `max_tokens` | int | Max output tokens |
| `temperature` | float64 | Sampling temperature |
| `top_p` | float64 | Nucleus sampling |
| `stream` | bool | Enable SSE streaming |
| `stream_options.include_usage` | bool | Include usage in final chunk |
| `tools` | []Tool | Function tools |
| `tool_choice` | any | Tool selection policy |

## Streaming

SSE events with `data: [DONE]` terminator. Each chunk is a `ChatStreamResponse` with delta content in `choices[].delta`.

## Tool Use

- Request: `tools[].function` with `name`, `description`, `parameters`
- Response: `choices[].message.tool_calls[]` with `id`, `function.name`, `function.arguments`
