# Claude Code Auto-Compact Configuration Guide

## TL;DR

When Claude Code CLI is routed through ai-switch (or any proxy), auto-compact depends on **two independent layers** both being correct:

1. **Usage data layer** — ai-switch must emit the three Anthropic usage buckets (`input_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`) correctly. This is ai-switch's responsibility and is fixed in commit `a99dc6d`.
2. **Threshold layer** — Claude Code's client env vars must be configured so the threshold actually applies to your model. This is the user's responsibility and is documented here.

If auto-compact does not trigger despite context usage clearly exceeding your target percentage, the cause is almost always layer 2 on **Opus 4.8 / Fable 5 / extended-context models**.

---

## Official reference

- Environment variables: <https://code.claude.com/docs/en/env-vars.md>
- Extended context: <https://code.claude.com/docs/en/model-config#extended-context>
- Auto-compaction & costs: <https://code.claude.com/docs/en/costs#reduce-token-usage>

---

## The two relevant environment variables

### `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`

> Set the percentage (1-100) of the auto-compaction window at which auto-compaction triggers. Use lower values like `50` to compact earlier. **This variable only causes earlier compaction when Claude Code compacts proactively: when `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is set, in cloud sessions, and on Sonnet 4.6 and Opus 4.6 without extended context, which compact at the 200K boundary by default. In other cases, such as a local session on Opus 4.8 or any model with extended context, auto-compaction triggers when the conversation reaches the model's context limit.** The override can only lower the threshold, so values above the default have no effect. Applies to both main conversations and subagents.

### `CLAUDE_CODE_AUTO_COMPACT_WINDOW`

> Set the context capacity in tokens used for auto-compaction calculations. Defaults to the model's context window: 200K for standard models or 1M for extended context models. Use a lower value like `500000` on a 1M model to treat the window as 500K for compaction purposes. The value is capped at the model's actual context window. `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` is applied as a percentage of this value. Setting this variable decouples the compaction threshold from the status line's `used_percentage`, which always uses the model's full context window.

---

## The critical rule

`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` does **not** apply universally. It only takes effect in these proactive-compaction scenarios:

| Scenario | `PCT_OVERRIDE` effective alone? | Default compaction point |
| :-- | :--: | :-- |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is set | ✅ Yes | percentage of the configured window |
| Cloud session (`claude --remote`) | ✅ Yes | 200K boundary |
| Sonnet 4.6 / Opus 4.6 **without** extended context | ✅ Yes | 200K boundary |
| **Opus 4.8 / Fable 5 / any model with extended context** (local) | ❌ **No** | the model's full context limit (~200K or 1M) |

> The override can only **lower** the threshold. A value above the model's effective default is a no-op.

**Practical consequence:** on a local Opus 4.8 session, setting only `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=55` does nothing. Auto-compact will wait until the conversation approaches the model's context limit. You must also set `CLAUDE_CODE_AUTO_COMPACT_WINDOW` to bring the threshold down.

---

## Recommended configuration

Pick **one** approach depending on your goal.

### Approach A — Fixed token window (simplest, recommended)

Treat compaction as "compact when context reaches N tokens":

```jsonc
// ~/.claude/settings.json
{
  "env": {
    // Compact when context reaches 160000 tokens (~80% of a 200K window)
    "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "160000"
  }
}
```

- `PCT_OVERRIDE` is **not needed** here (defaults to 100% of the 160K window = compact at 160K).
- Works on every model, including Opus 4.8 and extended-context variants.
- **Why ~80% and not lower?** Each compaction generates a summary that loses some detail and costs an extra request. A higher threshold maximizes usable context and compacts less often, while still leaving a ~20% buffer (40K on a 200K window) for the summary plus a few follow-up turns. Dropping below ~70% compacts too aggressively for most workflows; pushing past ~90% leaves too little room for the summary itself.

### Approach B — Percentage of an explicit window

If you prefer to keep a "full window" mental model and express the threshold as a percentage:

```jsonc
{
  "env": {
    "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "200000",  // base window = 200K
    "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80"       // compact at 80% = 160K
  }
}
```

Setting `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is what makes `PCT_OVERRIDE` effective on Opus 4.8 — see the rule above.

### Common env vars you may already have

| Variable | Effect |
| :-- | :-- |
| `CLAUDE_CODE_DISABLE_1M_CONTEXT=1` | Forces the 200K window. Combined with Approach A, `160000` ≈ 80%. |
| `DISABLE_AUTO_COMPACT=1` | Disables auto-compact entirely (manual `/compact` still works). |
| `DISABLE_COMPACT=1` | Disables **all** compaction, including manual `/compact`. |
| `CLAUDE_CODE_MAX_CONTEXT_TOKENS` | Override the assumed context window — only takes effect when `DISABLE_COMPACT` is also set. |

---

## Applying changes

Claude Code reads env vars at **startup**, from the `env` block of `settings.json`. After editing:

1. Save `~/.claude/settings.json`.
2. **Fully restart Claude Code** (restart the CLI client — restarting ai-switch is not enough).
3. If the current context already exceeds the new threshold, the **next** request will trigger auto-compact.

> Note: when `CLAUDE_CODE_AUTO_COMPACT_WINDOW` is set, the status line's `used_percentage` is **decoupled** from the compaction threshold — the percentage still reflects the model's full context window, so it will not match the threshold at which compaction actually fires. This is expected.

---

## Relationship to ai-switch (the data layer)

Correct threshold configuration is useless if the usage data feeding the calculation is wrong. Claude Code computes context utilization as:

```
context_used = input_tokens + cache_creation_input_tokens + cache_read_input_tokens + output_tokens
used_percentage = context_used / context_window_size
```

The three input buckets are **separate fields** in the Anthropic usage object. ai-switch must translate them correctly from every upstream protocol (OpenAI Chat, Responses, Gemini):

- OpenAI `prompt_tokens` is **inclusive** of cached tokens; Anthropic `input_tokens` **excludes** them (saturating subtraction).
- Missing `cache_read_input_tokens` / `cache_creation_input_tokens` makes `context_used` drastically understated on cached conversations — auto-compact never fires and the status line under-reports.

This conversion is enforced centrally in `internal/converter/anthropic_usage.go` (`buildAnthropicUsageMap`) and applied across all streaming and non-streaming →Anthropic paths (commit `a99dc6d`).

---

## Troubleshooting checklist

When auto-compact does not trigger as expected, verify in this order:

1. **Is ai-switch running the fixed binary?** `ais version` should be `a99dc6d` or later. Check the LLM log (`llm_log_enabled: true`) for a `message_delta` whose `usage` includes `cache_read_input_tokens` (non-zero on cached turns).
2. **Is the threshold effective for your model?** On Opus 4.8 / extended context, `PCT_OVERRIDE` alone is a no-op — set `CLAUDE_CODE_AUTO_COMPACT_WINDOW`.
3. **Did you restart Claude Code after editing `settings.json`?** Env vars are read at startup.
4. **Is `DISABLE_AUTO_COMPACT` / `DISABLE_COMPACT` set?** These override everything.
5. **Does `used_percentage` look right in the status line?** If it is far below your real context size, the usage data layer (step 1) is still broken.
