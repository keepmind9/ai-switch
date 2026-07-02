package converter

// buildAnthropicUsageMap builds an Anthropic Messages API usage object from
// upstream token counts, enforcing the three-bucket invariant:
//
//	input_tokens + cache_read_input_tokens + cache_creation_input_tokens == promptTokens
//
// Upstream "prompt" totals (OpenAI prompt_tokens, Responses input_tokens,
// Gemini promptTokenCount) are INCLUSIVE of cached entries, whereas Anthropic's
// input_tokens EXCLUDES them — so cache_read and cache_creation are subtracted
// from promptTokens (saturating, never negative). This mirrors cc-switch's
// build_anthropic_usage_json / build_anthropic_usage_from_responses.
//
// cache fields are emitted only when non-zero, matching Anthropic's streaming
// examples and avoiding noise for upstreams without a cache concept (e.g. Gemini).
//
// Why this matters: Claude Code derives its context-window utilization (and the
// auto-compact trigger) from the sum of these three input fields in the
// message_start/message_delta usage. Omitting the cache buckets makes the client
// read the context as near-empty, so auto-compact never fires.
func buildAnthropicUsageMap(promptTokens, outputTokens, cacheRead, cacheCreation int) map[string]any {
	input := promptTokens - cacheRead - cacheCreation
	if input < 0 {
		input = 0
	}
	usage := map[string]any{
		"input_tokens":  input,
		"output_tokens": outputTokens,
	}
	if cacheRead > 0 {
		usage["cache_read_input_tokens"] = cacheRead
	}
	if cacheCreation > 0 {
		usage["cache_creation_input_tokens"] = cacheCreation
	}
	return usage
}

// extractResponsesCacheRead resolves the cached input-token count from a
// Responses API usage object. It prefers the direct cache_read_input_tokens
// field (compatible upstreams) and falls back to the OpenAI-standard
// input_tokens_details.cached_tokens. Mirrors cc-switch extract_cache_read_tokens.
func extractResponsesCacheRead(usage map[string]any) int {
	if c := int(toFloat64(usage["cache_read_input_tokens"])); c > 0 {
		return c
	}
	if details, ok := usage["input_tokens_details"].(map[string]any); ok {
		if c := int(toFloat64(details["cached_tokens"])); c > 0 {
			return c
		}
	}
	return 0
}
