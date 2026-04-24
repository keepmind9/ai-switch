# OpenAI Responses API

> Official docs: https://platform.openai.com/docs/api-reference/responses

## Endpoint

```
POST /v1/responses
```

## Auth

```
Authorization: Bearer <api-key>
```

## Request Fields (project scope)

| Field | Type | Note |
|-------|------|------|
| `model` | string | Model ID |
| `input` | any | Text, array, or structured input |
| `instructions` | string | System-level instructions |
| `max_tokens` | int | Max output tokens |
| `temperature` | float64 | Sampling temperature |
| `top_p` | float64 | Nucleus sampling |
| `stream` | bool | Enable SSE streaming |
| `tools` | []ResponsesTool | Available tools |
| `tool_choice` | any | Tool selection policy |
| `previous_response_id` | string | Chain to prior response |
| `store` | bool | Persist response |
| `metadata` | map | Custom metadata |

## Streaming

SSE events with typed events: `response.output_item.added`, `response.content_part.added`, `response.output_text.delta`, `response.output_text.done`, `response.completed`, etc.

## Tool Use

- Request: `tools[]` with `type`, `name`, `description`, `parameters`
- Response: `FunctionCallBlock` with `call_id`, `name`, `arguments`
