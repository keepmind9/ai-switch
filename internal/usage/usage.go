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
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// DefaultClient is a plain client for direct usage queries. Its transport
// deliberately has no Proxy hook, so it does NOT inherit HTTP(S)_PROXY from
// the environment — ai-switch manages proxy selection itself. Callers that
// need provider-specific transport (e.g. proxy routing) must pass their own
// client to Query.
var DefaultClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: directTransport(),
}

// directTransport builds a transport that connects directly (no environment
// proxy inheritance) with sane timeouts for a single usage query.
func directTransport() *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
}

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

// Window describes one quota window of a coding plan.
type Window struct {
	// Type is the raw GLM limit type: TOKENS_LIMIT (token quota) or
	// CREDIT_LIMIT (credit-based plans).
	Type string `json:"type"`
	// Unit and Number identify the window size (e.g. unit=3, number=5 is the
	// 5-hour window). They are passed through verbatim since the unit
	// enumeration is not publicly documented.
	Unit   int `json:"unit"`
	Number int `json:"number"`
	// Utilization is the percentage of the window quota already used (0-100).
	Utilization float64 `json:"utilization"`
	// ResetsAtMs is the unix timestamp (milliseconds) when the window resets.
	// 0 when no window is active (the plan is waiting to be triggered).
	ResetsAtMs int64 `json:"resets_at_ms"`
	// WindowActive reports whether the window is currently running.
	WindowActive bool `json:"window_active"`
}

// Info describes a coding plan's usage: its level plus all quota windows.
type Info struct {
	// Level is the plan tier reported by the API (e.g. "lite", "pro").
	Level string `json:"level"`
	// PlanResetsAtMs is the unix timestamp (milliseconds) when the coding
	// plan itself resets (e.g. the monthly reset), taken from the TIME_LIMIT
	// entry. 0 when the API does not report one.
	PlanResetsAtMs int64 `json:"plan_resets_at_ms"`
	// Windows holds every TOKENS_LIMIT / CREDIT_LIMIT entry, in API order.
	// TIME_LIMIT entries (tool-usage caps) are not plan windows; only their
	// reset time is captured as PlanResetsAtMs.
	Windows []Window `json:"windows"`
}

// windowTypes are the GLM limit types that represent plan quota windows.
var windowTypes = map[string]bool{
	"TOKENS_LIMIT": true,
	"CREDIT_LIMIT": true,
}

// glmQuotaResponse mirrors the GLM quota API response. Only the fields we
// consume are modeled.
type glmQuotaResponse struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
	Data    struct {
		Limits []struct {
			Type          string  `json:"type"`
			Unit          int     `json:"unit"`
			Number        int     `json:"number"`
			Percentage    float64 `json:"percentage"`
			NextResetTime *int64  `json:"nextResetTime"`
		} `json:"limits"`
		Level string `json:"level"`
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

	info := &Info{Level: quota.Data.Level}
	for _, limit := range quota.Data.Limits {
		// TIME_LIMIT is the plan-level (e.g. monthly) reset — capture its
		// reset time but don't render it as a quota window.
		if limit.Type == "TIME_LIMIT" {
			if limit.NextResetTime != nil {
				info.PlanResetsAtMs = *limit.NextResetTime
			}
			continue
		}
		if !windowTypes[limit.Type] {
			continue
		}
		w := Window{
			Type:        limit.Type,
			Unit:        limit.Unit,
			Number:      limit.Number,
			Utilization: limit.Percentage,
		}
		if limit.NextResetTime != nil {
			w.ResetsAtMs = *limit.NextResetTime
			w.WindowActive = true
		}
		info.Windows = append(info.Windows, w)
	}
	if len(info.Windows) == 0 {
		return nil, fmt.Errorf("no TOKENS_LIMIT or CREDIT_LIMIT found in usage response: %s", truncate(body, 200))
	}
	return info, nil
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
