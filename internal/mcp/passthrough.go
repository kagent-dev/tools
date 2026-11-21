package mcp

import (
	"errors"
	"net/http"
	"os"
	"strings"
)

// TokenPassthroughEnv names the environment variable that turns on bearer-token
// passthrough. When it is "true", every tool that talks to the cluster through a
// CLI authenticates as the caller, with the bearer token of the MCP request,
// instead of with the server's own kubeconfig.
const TokenPassthroughEnv = "TOKEN_PASSTHROUGH"

// ErrBearerTokenRequired is returned when passthrough is on and the request
// carries no bearer token. Falling back to the server's identity would let a
// caller act with credentials that are not theirs, so the call is refused.
var ErrBearerTokenRequired = errors.New("Bearer token required when TOKEN_PASSTHROUGH is true")

// TokenPassthroughEnabled reports whether TOKEN_PASSTHROUGH is "true".
func TokenPassthroughEnabled() bool {
	return os.Getenv(TokenPassthroughEnv) == "true"
}

// PassthroughToken returns the token a cluster-facing CLI call must use, with
// passthrough read from TOKEN_PASSTHROUGH. See ResolvePassthroughToken.
func PassthroughToken(headers http.Header) (string, error) {
	return ResolvePassthroughToken(TokenPassthroughEnabled(), headers)
}

// ResolvePassthroughToken returns the token a cluster-facing CLI call must use:
//
//   - passthrough on and an "Authorization: Bearer <token>" header: the token;
//   - passthrough on and no usable bearer token: ErrBearerTokenRequired;
//   - passthrough off: "" (the configured kubeconfig is used, and any token in
//     the request is ignored).
//
// The token is meant for commands.CommandBuilder.WithToken, which hands it to
// the CLI through a per-call kubeconfig, never on argv.
func ResolvePassthroughToken(passthrough bool, headers http.Header) (string, error) {
	if !passthrough {
		return "", nil
	}
	token := bearerToken(headers)
	if token == "" {
		return "", ErrBearerTokenRequired
	}
	return token, nil
}

// bearerToken returns the token of an "Authorization: Bearer <token>" header,
// or "" when there is none or the header uses another scheme.
func bearerToken(headers http.Header) string {
	if auth := headers.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
