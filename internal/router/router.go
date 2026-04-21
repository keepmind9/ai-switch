package router

import "strings"

// RouteResult holds the resolved routing decision.
type RouteResult struct {
	ProviderKey string // config provider key
	BaseURL     string // upstream base_url
	APIKey      string // upstream api_key
	Format      string // upstream format (chat/anthropic/responses)
	Model       string // resolved model name to send upstream
	Path        string // optional path override
	ThinkTag    string // optional: strip <tag>...</tag> from responses
}

// Router resolves a request to an upstream provider + model.
type Router interface {
	// Route makes a routing decision. clientProtocol is "anthropic"/"responses"/"chat".
	// apiKey is from the client's auth header. body is the raw request JSON.
	Route(clientProtocol, apiKey string, body []byte) (*RouteResult, error)
}

// BuildUpstreamURL constructs the full upstream URL from base_url and api_path.
// Handles both cases: base_url with or without /v1 suffix.
func BuildUpstreamURL(baseURL, apiPath string) string {
	base := strings.TrimRight(baseURL, "/")
	// If base_url already ends with /v1, strip it from apiPath to avoid /v1/v1.
	if strings.HasSuffix(base, "/v1") {
		apiPath = strings.TrimPrefix(apiPath, "/v1")
	}
	return base + apiPath
}
