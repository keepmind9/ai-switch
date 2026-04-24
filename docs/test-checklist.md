# Test Checklist

Comprehensive test scenarios for ai-switch protocol conversion proxy.

## Basic Conversation

| # | Scenario | Client Protocol | Stream | Notes |
|---|----------|----------------|--------|-------|
| 1 | Basic Q&A | Chat | No | Simple question and answer |
| 2 | Basic Q&A | Chat | Yes | Full streaming output |
| 3 | Basic Q&A | Responses | No | Codex basic Q&A |
| 4 | Basic Q&A | Responses | Yes | Recently fixed: upstream SSE without Content-Type |
| 5 | Basic Q&A | Anthropic | No | Claude Code basic Q&A |
| 6 | Basic Q&A | Anthropic | Yes | Claude Code streaming |

## Tool Use / Function Calling

| # | Scenario | Stream | Notes |
|---|----------|--------|-------|
| 7 | Chat client invokes tool | Yes/No | Send tools param, receive tool_calls response |
| 8 | Chat client returns tool result | Yes/No | Send tool role message, continue conversation |
| 9 | Responses client invokes tool | Yes/No | Codex triggers skill (function_call output) |
| 10 | Responses client returns tool result | Yes/No | function_call_output sent back, continue |
| 11 | Anthropic client invokes tool | Yes/No | Claude Code tool_use block |
| 12 | Anthropic client returns tool result | Yes/No | tool_result sent back, continue |
| 13 | Multi-turn tool invocation | Yes/No | 2+ consecutive tool calls |

## Cross-Protocol Conversion

### Non-Tool-Use

| # | Client | Upstream | Stream | Notes |
|---|--------|----------|--------|-------|
| 14 | Chat | Responses | No | Chat → Responses conversion |
| 15 | Chat | Responses | Yes | Streaming SSE conversion |
| 16 | Chat | Anthropic | No | Chat → Anthropic conversion |
| 17 | Chat | Anthropic | Yes | Anthropic SSE → Chat SSE |
| 18 | Responses | Chat | No | Responses → Chat conversion |
| 19 | Responses | Chat | Yes | Chat SSE → Responses SSE |
| 20 | Responses | Anthropic | No | Responses → Anthropic conversion |
| 21 | Responses | Anthropic | Yes | Anthropic → Responses SSE |
| 22 | Anthropic | Chat | No | Anthropic → Chat conversion |
| 23 | Anthropic | Chat | Yes | Chat SSE → Anthropic SSE |
| 24 | Anthropic | Responses | No | Anthropic → Responses direct conversion |
| 25 | Anthropic | Responses | Yes | Responses → Anthropic SSE |

### With Tool Use

| # | Scenario | Notes |
|---|----------|-------|
| 26 | Codex (Responses) → Chat upstream + tool use | Responses tool → Chat tool_calls |
| 27 | Codex (Responses) → Anthropic upstream + tool use | Responses tool → Anthropic tool_use |
| 28 | Claude Code (Anthropic) → Chat upstream + tool use | Anthropic tool_use → Chat tool_calls |
| 29 | Claude Code (Anthropic) → Responses upstream + tool use | Anthropic tool_use → Responses function_call |
| 30 | Chat client → Responses upstream + tool use | Chat tool_calls → Responses function_call |
| 31 | Chat client → Anthropic upstream + tool use | Chat tool_calls → Anthropic tool_use |

## Error Handling

| # | Scenario | Notes |
|---|----------|-------|
| 32 | Upstream returns 429 | Rate limit, client receives correctly formatted error |
| 33 | Upstream returns 401 | Authentication failure |
| 34 | Upstream model not found | 400/404 error |
| 35 | Upstream timeout | Connection timeout handling |
| 36 | Upstream stream disconnects mid-stream | SSE interruption, client receives reasonable feedback |
| 37 | Upstream returns non-SSE error in streaming path | Recently fixed non-SSE handling |
| 38 | Malformed request | Invalid JSON, missing required fields |

## Edge Cases

| # | Scenario | Notes |
|---|----------|-------|
| 39 | Long context | Large input token count |
| 40 | Empty input / empty messages | Boundary condition |
| 41 | tool_choice parameter | auto / required / none / specific function |
| 42 | Multiple tool definitions | 3+ tools defined simultaneously |
| 43 | Concurrent requests | Multiple requests with different protocols simultaneously |
| 44 | Token usage tracking | Verify usage statistics recorded correctly |

## Priority

- **P0 (Must)**: 1-6 (basic conversation), 7-12 (tool use), 4 (recently fixed streaming Responses)
- **P1 (Important)**: 14-25 (cross-protocol conversion), 26-31 (cross-protocol tool use)
- **P2 (Supplementary)**: 32-44 (error handling and edge cases)
