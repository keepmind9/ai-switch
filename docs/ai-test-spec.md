# AI Automated Test Specification

This document enables an AI agent to autonomously execute all test scenarios from `test-checklist.md` against a running ai-switch instance and produce a results report.

## 1. Test Environment

### 1.1 Prerequisites

- ai-switch running at `http://localhost:3456`
- curl available
- jq available (for JSON parsing)

### 1.2 Configuration Reference

The test uses the running server's config. Key mappings:

| Protocol | Default Route | Upstream Format | Model |
|----------|--------------|----------------|-------|
| Chat | glm-anthropic | anthropic | glm-5.1 |
| Anthropic | glm-anthropic | anthropic | glm-5.1 |
| Responses | xaixapi-gpt | chat | gpt-5.4 |

### 1.3 API Key

Use the configured API key for requests. Read from config if needed:
```bash
grep "api_key" ~/.ai-switch/config.yaml | head -1
```

### 1.4 Report Format

Create `docs/test-report.md` with the following structure:

```markdown
# Test Report
**Date**: <auto-filled>
**Commit**: <git rev-parse --short HEAD>

## Summary
- Total: N
- Passed: N
- Failed: N

## Results

| # | Scenario | Status | Details |
|---|----------|--------|---------|
| 1 | ... | PASS/FAIL | ... |
```

For each test case, record:
- **PASS**: response contains expected content, correct format
- **FAIL**: include the actual response body (truncated to 500 chars) and error description

---

## 2. Test Helpers

### 2.1 SSE Stream Validation

For streaming tests, use this pattern to validate SSE output:
```bash
curl -sN -X POST http://localhost:3456/v1/... \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <key>" \
  -d '<body>' \
  --max-time 30 2>/dev/null
```

SSE stream is valid if:
- Response starts with SSE headers: `Content-Type: text/event-stream`
- Contains expected terminal event:
  - Chat: `data: [DONE]`
  - Anthropic: `event: message_stop`
  - Responses: `event: response.completed`
- Contains actual model response text (not empty)

### 2.2 Response Validation Patterns

**Non-streaming Chat**:
```bash
# Check: status 200, valid JSON, choices[0].message.content non-empty
curl -s -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <key>" \
  -d '<body>' | jq '.choices[0].message.content'
```

**Non-streaming Anthropic**:
```bash
# Check: status 200, valid JSON, content[0].text non-empty
curl -s -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: <key>" \
  -H "anthropic-version: 2023-06-01" \
  -d '<body>' | jq '.content[0].text'
```

**Non-streaming Responses**:
```bash
# Check: status 200, valid JSON, output[0].content[0].text non-empty
curl -s -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <key>" \
  -d '<body>' | jq '.output[0].content[0].text'
```

---

## 3. Test Cases

### 3.1 Basic Conversation (Tests 1-6)

#### Test 1: Chat Non-Streaming
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Say hello in one word"}],
    "max_tokens": 50
  }'
```
**Pass criteria**: HTTP 200, `choices[0].message.content` is non-empty string.

#### Test 2: Chat Streaming
```bash
curl -sN -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Say hello in one word"}],
    "max_tokens": 50,
    "stream": true
  }' --max-time 30 2>/dev/null
```
**Pass criteria**: HTTP 200, response is SSE stream, contains `data: [DONE]`, contains text content in delta chunks.

#### Test 3: Responses Non-Streaming
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "gpt-5.4",
    "input": "Say hello in one word"
  }'
```
**Pass criteria**: HTTP 200, `output[0].content[0].text` is non-empty string.

#### Test 4: Responses Streaming
```bash
curl -sN -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "gpt-5.4",
    "input": "Say hello in one word",
    "stream": true
  }' --max-time 30 2>/dev/null
```
**Pass criteria**: HTTP 200, SSE stream with `event: response.completed`, contains text in `response.output_text.delta` events.

#### Test 5: Anthropic Non-Streaming
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test-key" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Say hello in one word"}],
    "max_tokens": 50
  }'
```
**Pass criteria**: HTTP 200, `content[0].text` is non-empty string.

#### Test 6: Anthropic Streaming
```bash
curl -sN -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test-key" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Say hello in one word"}],
    "max_tokens": 50,
    "stream": true
  }' --max-time 30 2>/dev/null
```
**Pass criteria**: HTTP 200, SSE stream with `event: message_stop`, contains text in `content_block_delta` events.

---

### 3.2 Tool Use / Function Calling (Tests 7-13)

#### Test 7: Chat Client Invokes Tool
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "What is the weather in Beijing?"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather for a city",
        "parameters": {
          "type": "object",
          "properties": {"city": {"type": "string"}},
          "required": ["city"]
        }
      }
    }],
    "max_tokens": 200
  }'
```
**Pass criteria**: HTTP 200, response has `choices[0].message.tool_calls` array with at least one entry, or `choices[0].message.content` contains weather info (model may choose not to use tool).

#### Test 8: Chat Client Tool Result Roundtrip
```bash
# Step 1: First call with tool_use (use response from Test 7)
# Step 2: Send tool result back
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "glm-5.1",
    "messages": [
      {"role": "user", "content": "What is the weather in Beijing?"},
      {"role": "assistant", "content": null, "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}}]},
      {"role": "tool", "tool_call_id": "call_1", "content": "{\"temperature\": 22, \"condition\": \"sunny\"}"}
    ],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather for a city",
        "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
      }
    }],
    "max_tokens": 200
  }'
```
**Pass criteria**: HTTP 200, model responds with weather summary based on tool result.

#### Test 9: Responses Client Invokes Tool
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "gpt-5.4",
    "input": "What is the weather in Beijing?",
    "tools": [{
      "type": "function",
      "name": "get_weather",
      "description": "Get current weather for a city",
      "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
    }]
  }'
```
**Pass criteria**: HTTP 200, output contains `function_call` type item with `name` and `arguments`, or text response if model chooses not to use tool.

#### Test 10: Responses Client Tool Result Roundtrip
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "gpt-5.4",
    "input": [
      {"role": "user", "content": "What is the weather in Beijing?"},
      {"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"},
      {"type": "function_call_output", "call_id": "call_1", "output": "{\"temperature\": 22, \"condition\": \"sunny\"}"}
    ],
    "tools": [{
      "type": "function",
      "name": "get_weather",
      "description": "Get current weather for a city",
      "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
    }]
  }'
```
**Pass criteria**: HTTP 200, model responds with weather summary.

#### Test 11: Anthropic Client Invokes Tool
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test-key" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "What is the weather in Beijing?"}],
    "tools": [{
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
    }],
    "max_tokens": 200
  }'
```
**Pass criteria**: HTTP 200, response contains `tool_use` content block with `name` and `input`, or text response.

#### Test 12: Anthropic Client Tool Result Roundtrip
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: test-key" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "glm-5.1",
    "messages": [
      {"role": "user", "content": "What is the weather in Beijing?"},
      {"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {"city": "Beijing"}}]},
      {"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_1", "content": "{\"temperature\": 22, \"condition\": \"sunny\"}"}]}
    ],
    "tools": [{
      "name": "get_weather",
      "description": "Get current weather for a city",
      "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
    }],
    "max_tokens": 200
  }'
```
**Pass criteria**: HTTP 200, model responds with weather summary.

#### Test 13: Multi-turn Tool Invocation
```bash
# Send a request that may trigger multiple tool calls, e.g. asking about two cities
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model": "glm-5.1",
    "messages": [{"role": "user", "content": "Compare the weather in Beijing and Shanghai"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get current weather for a city",
        "parameters": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
      }
    }],
    "max_tokens": 300
  }'
```
**Pass criteria**: HTTP 200, response has 2+ tool_calls or model provides comparison.

---

### 3.3 Cross-Protocol Conversion — Basic (Tests 14-25)

These tests verify conversion between different client and upstream formats. The current default config has:
- Chat client → Anthropic upstream (glm-anthropic route)
- Anthropic client → Anthropic upstream (glm-anthropic route)
- Responses client → Chat upstream (xaixapi-gpt route)

To test all combinations, the AI agent should:

1. **Use the existing routes** for default paths (tests 14-25 that match current config)
2. **Note untestable combinations** where no route exists for that client→upstream pair

#### Test Matrix by Current Config

| Test | Client | Upstream | Route | Executable |
|------|--------|----------|-------|-----------|
| 14-15 | Chat | Responses | - | Note: no Chat→Responses route configured |
| 16-17 | Chat | Anthropic | glm-anthropic | Yes |
| 18-19 | Responses | Chat | xaixapi-gpt | Yes |
| 20-21 | Responses | Anthropic | - | Note: no Responses→Anthropic route configured |
| 22-23 | Anthropic | Chat | - | Note: no Anthropic→Chat route configured |
| 24-25 | Anthropic | Responses | - | Note: no Anthropic→Responses route configured |

Tests 16-17 are same as Tests 1-2 (default Chat route goes to Anthropic upstream).
Tests 18-19 are same as Tests 3-4 (default Responses route goes to Chat upstream).

Execute Tests 16-19 using the curl commands from Tests 1-4.

For untestable combinations, create a temporary route via admin API:
```bash
# Example: create a route pointing to Chat upstream for Anthropic client
curl -s -X POST http://localhost:3456/admin/routes \
  -H "Content-Type: application/json" \
  -d '{"provider": "glm-coding-plan-chat", "default_model": "glm-5.1"}'
```

Then test Anthropic client → Chat upstream:
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: <route-key-from-above>" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"Hello"}],"max_tokens":50}'
```

**Pass criteria for all**: HTTP 200, response contains non-empty text in the correct client format.

---

### 3.4 Cross-Protocol Tool Use (Tests 26-31)

Same as Tests 14-25 but with tool definitions included. Execute the tool use curl commands from Tests 7-12 against routes that trigger cross-protocol conversion.

**Key combinations to test**:

| Test | Client | Upstream | How |
|------|--------|----------|-----|
| 26 | Responses (Codex) | Chat | Default xaixapi-gpt route (Chat upstream) — use Test 9 curl |
| 27 | Responses (Codex) | Anthropic | Create route to glm-anthropic — use Test 9 curl with new route key |
| 28 | Anthropic (Claude Code) | Chat | Create route to glm-coding-plan-chat — use Test 11 curl with new route key |
| 29 | Anthropic (Claude Code) | Responses | Create route to xaixapi-gpt — use Test 11 curl with new route key |
| 30 | Chat | Responses | Note: no Chat→Responses route currently |
| 31 | Chat | Anthropic | Default glm-anthropic route — use Test 7 curl |

**Pass criteria**: Tool call/response is correctly translated between protocols. Verify the tool call format matches the client protocol expectations.

---

### 3.5 Error Handling (Tests 32-38)

#### Test 32: Upstream 429 Rate Limit
```bash
# Send rapid-fire requests to trigger rate limit
for i in $(seq 1 5); do
  curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer test-key" \
    -d '{"model":"glm-5.1","messages":[{"role":"user","content":"hi"}],"max_tokens":10}' &
done
wait
```
**Pass criteria**: If 429 received, error JSON is in correct format with error message.

#### Test 33: Authentication Failure
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer invalid-key-12345" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"hi"}],"max_tokens":10}'
```
**Pass criteria**: Non-200 status (401 or equivalent), error JSON returned.

#### Test 34: Model Not Found
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{"model":"nonexistent-model-xyz","messages":[{"role":"user","content":"hi"}],"max_tokens":10}'
```
**Pass criteria**: Non-200 status, error JSON with model-not-found message.

#### Test 35: Upstream Timeout
```bash
# Request with very short timeout
curl -s -w "\nHTTP_STATUS:%{http_code}" --max-time 1 -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"Write a 10000 word essay"}],"max_tokens":9999}'
```
**Pass criteria**: Timeout handled gracefully (curl error or 502/504 from proxy).

#### Test 36: Mid-Stream Disconnect
```bash
# Start streaming and kill connection early
timeout 2 curl -sN -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"Write a long story"}],"max_tokens":9999,"stream":true}' 2>/dev/null || true
```
**Pass criteria**: Partial data received, no crash on server side. Check server logs for errors.

#### Test 37: Non-SSE Error in Streaming Path
```bash
# This tests the recently fixed code path where upstream returns non-SSE in streaming mode
# Use Responses streaming which may hit the Content-Type issue
curl -sN -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{"model":"gpt-5.4","input":"Hello","stream":true}' --max-time 30 2>/dev/null
```
**Pass criteria**: HTTP 200, SSE stream with `event: response.completed`. Check server logs show no "failed to parse" errors.

#### Test 38: Malformed Request
```bash
# Invalid JSON
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d 'not json at all'

# Missing required field
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{"model":"glm-5.1"}'
```
**Pass criteria**: HTTP 400, error JSON with descriptive message.

---

### 3.6 Edge Cases (Tests 39-44)

#### Test 39: Long Context
```bash
# Generate a large input
LARGE_INPUT=$(python3 -c "print('hello ' * 2000)")
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d "{\"model\":\"glm-5.1\",\"messages\":[{\"role\":\"user\",\"content\":\"$LARGE_INPUT\"}],\"max_tokens\":50}"
```
**Pass criteria**: HTTP 200 with response, or graceful error if exceeds context limit.

#### Test 40: Empty Input
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{"model":"gpt-5.4","input":""}'
```
**Pass criteria**: Error response or model handles gracefully.

#### Test 41: tool_choice Parameter
```bash
# tool_choice=required should force tool use
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model":"glm-5.1",
    "messages":[{"role":"user","content":"Hello"}],
    "tools":[{"type":"function","function":{"name":"greet","description":"Greet user","parameters":{"type":"object","properties":{"name":{"type":"string"}}}}}],
    "tool_choice":"required",
    "max_tokens":100
  }'
```
**Pass criteria**: Response contains tool_calls (forced by `tool_choice: required`).

#### Test 42: Multiple Tool Definitions
```bash
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test-key" \
  -d '{
    "model":"glm-5.1",
    "messages":[{"role":"user","content":"What is the weather and time in Beijing?"}],
    "tools":[
      {"type":"function","function":{"name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}},
      {"type":"function","function":{"name":"get_time","description":"Get current time","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}},
      {"type":"function","function":{"name":"get_news","description":"Get latest news","parameters":{"type":"object","properties":{"topic":{"type":"string"}},"required":["topic"]}}}
    ],
    "max_tokens":300
  }'
```
**Pass criteria**: HTTP 200, model may call one or more tools.

#### Test 43: Concurrent Requests
```bash
# Send 3 concurrent requests with different protocols
curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" -H "Authorization: Bearer test-key" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"Say hi"}],"max_tokens":20}' &

curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/responses \
  -H "Content-Type: application/json" -H "Authorization: Bearer test-key" \
  -d '{"model":"gpt-5.4","input":"Say hi"}' &

curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" -H "x-api-key: test-key" -H "anthropic-version: 2023-06-01" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"Say hi"}],"max_tokens":20}' &

wait
```
**Pass criteria**: All 3 requests return HTTP 200 with valid responses.

#### Test 44: Token Usage Tracking
```bash
# Non-streaming
curl -s -X POST http://localhost:3456/v1/chat/completions \
  -H "Content-Type: application/json" -H "Authorization: Bearer test-key" \
  -d '{"model":"glm-5.1","messages":[{"role":"user","content":"Hello"}],"max_tokens":20}' \
  | jq '.usage'

# Check stats endpoint
curl -s http://localhost:3456/admin/stats | jq '.'
```
**Pass criteria**: `usage` field has non-zero `total_tokens`. Stats endpoint returns usage data.

---

## 4. Execution Instructions for AI Agent

### Step 1: Verify Environment
```bash
curl -s http://localhost:3456/admin/providers | jq '.[0].name'
# Should return provider name, not error
```

### Step 2: Execute Tests Sequentially
Run each test case in order. For each test:
1. Execute the curl command
2. Capture HTTP status code and response body
3. Evaluate pass criteria
4. Record result in the report

**Important**:
- Use `--max-time 30` for all streaming tests to avoid hanging
- For Tests 7-13 (tool use), the model may or may not choose to use the tool. Both outcomes can be PASS as long as the response format is correct.
- For Tests 26-31 (cross-protocol tool use), create temporary routes as needed using the admin API.
- Clean up temporary routes after testing.

### Step 3: Generate Report
After all tests complete, write results to `docs/test-report.md` using the format in section 1.4.

### Step 4: Cleanup
Remove any temporary routes created during testing.
