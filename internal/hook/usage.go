package hook

import (
	"log/slog"

	"github.com/keepmind9/ai-switch/internal/store"
)

// NewUsageHook creates an AfterResponse hook that records token usage to the store.
func NewUsageHook(usageStore *store.UsageStore) Hook {
	return Hook{
		Name:  "usage",
		Point: AfterResponse,
		Level: Optional,
		Fn: func(ctx *Context) error {
			if usageStore == nil {
				return nil
			}
			if ctx.InputTokens == 0 && ctx.OutputTokens == 0 {
				return nil
			}

			provider := ""
			if ctx.RouteResult != nil {
				provider = ctx.RouteResult.ProviderKey
			}

			usageStore.AsyncRecord(store.UsageRecord{
				Provider:            provider,
				Model:               ctx.ClientModel,
				Date:                store.Today(),
				Requests:            1,
				InputTokens:         ctx.InputTokens,
				OutputTokens:        ctx.OutputTokens,
				CacheCreationTokens: ctx.CacheCreateTokens,
				CacheReadTokens:     ctx.CacheReadTokens,
				TotalTokens:         ctx.InputTokens + ctx.OutputTokens,
			})

			slog.Debug("recorded usage", "provider", provider, "model", ctx.ClientModel,
				"input", ctx.InputTokens, "output", ctx.OutputTokens,
				"cache_create", ctx.CacheCreateTokens, "cache_read", ctx.CacheReadTokens)
			return nil
		},
	}
}
