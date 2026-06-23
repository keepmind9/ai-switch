# Codex Remote Compaction

Codex CLI compacts long conversations to stay within context limits. ai-switch
supports both Codex compaction protocols transparently and with **zero
configuration**, on **any** upstream provider — including non-OpenAI providers
(GLM, Anthropic, Gemini, DeepSeek, …) that do not implement compaction natively.

## Why this is needed

OpenAI's compaction returns an `encrypted_content` blob that only OpenAI's
servers can produce and later decrypt. A third-party upstream cannot generate
it, so forwarding a compaction request to a non-OpenAI upstream would leave
Codex with no compaction item — and Codex aborts:

- **v1** (`POST /v1/responses/compact`): parse error (`missing field text`).
- **v2** (an ordinary streaming `POST /v1/responses` whose `input` ends with a
  `{"type":"compaction_trigger"}` marker item):
  `remote compaction v2 expected exactly one compaction output item, got 0`.

ai-switch solves this by **synthesizing** the compaction itself: it summarizes
the conversation with the routed upstream model and returns a self-contained
compaction item that Codex accepts.

## Two trigger protocols

| Protocol | How Codex triggers it | Endpoint |
|---|---|---|
| v1 | Explicit compact call | `POST /v1/responses/compact` |
| v2 | Marker item in an ordinary streaming request | `POST /v1/responses`, `input` ends with `{"type":"compaction_trigger"}` |

Both are detected automatically. No configuration is required.

## How it works

1. **Detect.** v1 is detected by the `/compact` path; v2 is detected by scanning
   the request body's `input` array for a `compaction_trigger` item
   (`converter.HasCompactionTrigger`) before the normal completion pipeline
   runs. A replayed compaction item (`type: "compaction"`) is never mistaken for
   a v2 trigger.
2. **Summarize.** The conversation is flattened to text
   (`converter.ExtractConversationText`) and sent to the routed upstream model
   as a non-streaming summarization request
   (`converter.BuildSummarizationRequest`, max 1024 tokens). The same
   provider/model the original request routes to is reused — no separate
   compaction model is configured.
3. **Synthesize.** The summary is wrapped in an ai-switch "fake compaction"
   payload (see below) and returned in the protocol-correct shape:
   - v1: a `response.compaction` JSON object.
   - v2: a Responses SSE stream with exactly one `compaction` output item and a
     `response.completed` event (`converter.BuildCompactionSSE`). `model` echoes
     the client's requested model and `usage` is zeroed (the summarization
     tokens belong to the upstream call).

### Upstream-native compaction (OpenAI)

If the routed upstream speaks the Responses API natively (`format = responses`),
ai-switch forwards the compaction request as-is (`forwardCompactPassthrough`)
and lets the upstream produce the real `encrypted_content`. Synthesis only
applies to non-native upstreams.

## The fake-compaction closed loop

Because ai-switch cannot produce OpenAI's opaque `encrypted_content`, it uses a
self-describing payload so it can decode it again on the next request:

- **Encode** (`converter.EncodeCompactionPayload`): the summary is serialized as
  `{summary, model, ts}` → JSON → base64, prefixed with `aisw_`.
- **Replay.** When Codex sends the compaction item back in a later request,
  `decodeCompactionInBody` detects the `aisw_` prefix, removes the item from
  `input`, decodes the summary, and prepends it to the request's `instructions`
  field as a `[Conversation Summary]` block.

The compacted context therefore reaches the upstream as ordinary instructions,
which every format (Chat / Anthropic / Gemini) understands. v1 and v2 share the
same closed loop.

## Failure handling

If summarization fails (route error, upstream non-2xx, empty summary, or
conversion error), ai-switch never hangs the client and never fabricates an
empty summary:

- v1 returns an HTTP JSON error.
- v2 emits a Codex-parseable `response.failed` SSE followed by `[DONE]`, so the
  client stream terminates cleanly instead of hanging.

## Observability

v2 compaction requests are traced end-to-end
(`request → upstream_req → upstream_resp → response`) and appear in the Trace
Viewer / llm log like any ordinary request. v1 and the OpenAI passthrough path
are intentionally not traced. To see v2 compaction traces, enable LLM trace
logging (`llm_log_enabled`); see the Trace Viewer docs.

## Configuration

None. Compaction is auto-detected and reuses the request's routed provider and
model. There is no compaction-specific config option.
