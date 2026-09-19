// Package usage queries coding-plan usage/quota from upstream providers.
//
// Currently only GLM (Zhipu) coding plans are supported. The quota endpoint
// lives on a different path from the chat API but shares the same host and
// API key, so the query URL is derived from the provider's base_url host.
package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// DefaultClient is a plain client for direct usage queries. Callers that need
// provider-specific transport (e.g. proxy routing) must pass their own client
// to Query.
var DefaultClient = &http.Client{Timeout: 10 * time.Second}

// glmHosts are the base_url hosts that expose the GLM coding-plan quota API.
var glmHosts = map[string]bool{
	"open.bigmodel.cn": true, // CN
	"api.z.ai":         true, // global
}

// Supported reports whether the given provider base_url belongs to a vendor
// whose coding-plan usage can be queried.
func Supported(baseURL string) bool {
	host, err := hostOf(baseURL)
	if err != nil {
		return false
	}
	return glmHosts[host]
}

// Info describes a coding-plan usage window.
type Info struct {
	// Utilization is the percentage of the plan quota already used (0-100).
	Utilization float64 `json:"utilization"`
	// ResetsAtMs is the unix timestamp (milliseconds) when the window resets.
	// 0 when no window is active (the plan is waiting to be triggered).
	ResetsAtMs int64 `json:"resets_at_ms"`
	// WindowActive reports whether a usage window is currently running.
	WindowActive bool `json:"window_active"`
}

// glmQuotaResponse mirrors the GLM quota API response. Only the fields we
// consume are modeled.
type glmQuotaResponse struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
	Data    struct {
		Limits []struct {
			Type          string  `json:"type"`
			Percentage    float64 `json:"percentage"`
			NextResetTime *int64  `json:"nextResetTime"`
		} `json:"limits"`
	} `json:"data"`
}

// Query fetches the coding-plan usage for the provider at baseURL using the
// given HTTP client (nil falls back to DefaultClient, which connects directly
// with a 10s timeout). The URL is derived from the base_url origin; apiKey is
// sent verbatim in the Authorization header (GLM expects the raw key, no
// Bearer prefix) after environment-variable expansion.
func Query(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Info, error) {
	origin, err := originOf(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/api/monitor/usage/quota/limit", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", os.ExpandEnv(apiKey))
	req.Header.Set("Accept", "application/json")

	if client == nil {
		client = DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API returned status %d: %s", resp.StatusCode, truncate(body, 200))
	}

	var quota glmQuotaResponse
	if err := json.Unmarshal(body, &quota); err != nil {
		return nil, fmt.Errorf("decode usage response: %w", err)
	}
	if !quota.Success {
		return nil, fmt.Errorf("usage API error: %s", orDefault(quota.Msg, "unknown error"))
	}

	for _, limit := range quota.Data.Limits {
		if limit.Type != "TOKENS_LIMIT" {
			continue
		}
		info := &Info{Utilization: limit.Percentage}
		if limit.NextResetTime != nil {
			info.ResetsAtMs = *limit.NextResetTime
			info.WindowActive = true
		}
		return info, nil
	}
	return nil, fmt.Errorf("no TOKENS_LIMIT found in usage response: %s", truncate(body, 200))
}

// parseBaseURL validates a base_url and returns its scheme and host.
func parseBaseURL(baseURL string) (scheme, host string, err error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("missing scheme or host in %q", baseURL)
	}
	return u.Scheme, u.Host, nil
}

// originOf extracts scheme://host from a base_url.
func originOf(baseURL string) (string, error) {
	scheme, host, err := parseBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	return scheme + "://" + host, nil
}

// hostOf extracts the host portion of a base_url.
func hostOf(baseURL string) (string, error) {
	_, host, err := parseBaseURL(baseURL)
	return host, err
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
