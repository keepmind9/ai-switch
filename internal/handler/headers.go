package handler

import "net/http"

// applyCustomHeaders sets each configured header on the request. It is called
// after client headers are forwarded and upstream auth is set, so configured
// values override the transparently forwarded client header (Header.Set
// replaces any same-name header). Used for UA-gated upstreams such as the
// Kimi Coding Plan, which 403s non-whitelisted User-Agents.
//
// Note: because this runs AFTER the credential strip and the format-specific
// auth set, a custom header named "Authorization"/"x-api-key" would override
// the resolved upstream credential. This is intentional — the operator owns
// the provider config, so overriding a credential is self-harm at the same
// privilege level as the configured key.
//
// No-op when headers is nil or empty.
func applyCustomHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}
