package router

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
