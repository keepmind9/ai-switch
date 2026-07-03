package router

import "strings"

// RouteResult holds the resolved routing decision.
type RouteResult struct {
	ProviderKey   string            // config provider key
	BaseURL       string            // upstream base_url
	APIKey        string            // upstream api_key
	Format        string            // upstream format (chat/anthropic/responses)
	Model         string            // resolved model name to send upstream
	Path          string            // upstream API path (resolved from format or overridden by config)
	ThinkTag      string            // optional: strip <tag>...</tag> from responses
	CustomHeaders map[string]string // optional: extra headers sent upstream, overriding forwarded client headers (e.g. User-Agent for UA-gated upstreams)
}

// FormatToPath returns the default upstream API path for a given protocol format.
func FormatToPath(format string) string {
	switch format {
	case "anthropic":
		return "/v1/messages"
	case "responses":
		return "/v1/responses"
	case "gemini":
		return "/v1beta/models/{model}:streamGenerateContent"
	default:
		return "/v1/chat/completions"
	}
}

// GeminiGeneratePath returns the Gemini API path for generateContent.
func GeminiGeneratePath(model string, stream bool) string {
	action := "generateContent"
	if stream {
		return "/v1beta/models/" + model + ":streamGenerateContent?alt=sse"
	}
	return "/v1beta/models/" + model + ":" + action
}

// Router resolves a request to an upstream provider + model.
type Router interface {
	// Route makes a routing decision. clientProtocol is "anthropic"/"responses"/"chat".
	// apiKey is from the client's auth header. body is the raw request JSON.
	Route(clientProtocol, apiKey string, body []byte) (*RouteResult, error)
}

// BuildUpstreamURL concatenates baseURL and apiPath into the full upstream URL.
// It is a pure concatenation (only trims trailing slashes from baseURL) — any
// version-segment dedup must already be applied to apiPath, either by resolvePath
// at routing time or by CoordinatePath for dynamically built paths (e.g. Gemini).
func BuildUpstreamURL(baseURL, apiPath string) string {
	return strings.TrimRight(baseURL, "/") + apiPath
}

// CoordinatePath strips the leading version segment from apiPath when baseURL
// already ends with a version segment, to avoid doubled version segments such as
// "/v3/v1/chat/completions". A version segment matches /v<digits>[beta]
// (e.g. /v1, /v2, /v3, /v1beta). apiPath without a leading version segment is
// returned unchanged, so user-configured custom paths are passed through verbatim.
func CoordinatePath(baseURL, apiPath string) string {
	if !hasVersionSuffix(baseURL) {
		return apiPath
	}
	if pref := versionPrefix(apiPath); pref != "" {
		return apiPath[len(pref):]
	}
	return apiPath
}

// hasVersionSuffix reports whether the last path segment of s is a version
// segment: v<digits> with an optional "beta" suffix (e.g. v1, v3, v1beta).
func hasVersionSuffix(s string) bool {
	s = strings.TrimRight(s, "/")
	i := strings.LastIndexByte(s, '/')
	if i < 0 {
		return false
	}
	return isVersionSegment(s[i+1:])
}

// versionPrefix returns the leading version segment of s (e.g. "/v1", "/v3",
// "/v1beta"), or "" if s does not start with one.
func versionPrefix(s string) string {
	if len(s) == 0 || s[0] != '/' {
		return ""
	}
	end := 1
	for end < len(s) && s[end] != '/' {
		end++
	}
	if isVersionSegment(s[1:end]) {
		return s[:end]
	}
	return ""
}

// isVersionSegment reports whether seg (a single path segment without '/')
// matches the version pattern v<digits>[beta], e.g. "v1", "v3", "v1beta".
func isVersionSegment(seg string) bool {
	if len(seg) < 2 || seg[0] != 'v' {
		return false
	}
	j := 1
	for j < len(seg) && seg[j] >= '0' && seg[j] <= '9' {
		j++
	}
	if j == 1 { // 'v' with no digits
		return false
	}
	rest := seg[j:]
	return rest == "" || rest == "beta"
}
