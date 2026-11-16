package mcp

import (
	"errors"
	"net/http"
	"os"
	"testing"
)

func TestResolvePassthroughToken(t *testing.T) {
	header := func(kv ...string) http.Header {
		h := http.Header{}
		for i := 0; i+1 < len(kv); i += 2 {
			h.Add(kv[i], kv[i+1])
		}
		return h
	}

	tests := []struct {
		name        string
		passthrough bool
		headers     http.Header
		wantToken   string
		wantErr     error
	}{
		// Passthrough on: the caller's bearer token, or a refusal.
		{name: "on/bearer", passthrough: true, headers: header("Authorization", "Bearer test-token"), wantToken: "test-token"},
		{name: "on/nil headers", passthrough: true, headers: nil, wantErr: ErrBearerTokenRequired},
		{name: "on/no authorization header", passthrough: true, headers: header("X-Other", "value"), wantErr: ErrBearerTokenRequired},
		{name: "on/empty authorization header", passthrough: true, headers: header("Authorization", ""), wantErr: ErrBearerTokenRequired},
		{name: "on/bearer without token", passthrough: true, headers: header("Authorization", "Bearer "), wantErr: ErrBearerTokenRequired},
		{name: "on/bearer without separator", passthrough: true, headers: header("Authorization", "Bearer"), wantErr: ErrBearerTokenRequired},
		{name: "on/basic scheme", passthrough: true, headers: header("Authorization", "Basic dGVzdDp0ZXN0"), wantErr: ErrBearerTokenRequired},
		{name: "on/lower-case scheme", passthrough: true, headers: header("Authorization", "bearer test-token"), wantErr: ErrBearerTokenRequired},
		{name: "on/token without scheme", passthrough: true, headers: header("Authorization", "test-token"), wantErr: ErrBearerTokenRequired},
		{name: "on/first of several values", passthrough: true, headers: header("Authorization", "Bearer first-token", "Authorization", "Bearer second-token"), wantToken: "first-token"},

		// Passthrough off: never a token, whatever the request carries.
		{name: "off/bearer", passthrough: false, headers: header("Authorization", "Bearer test-token")},
		{name: "off/nil headers", passthrough: false, headers: nil},
		{name: "off/no authorization header", passthrough: false, headers: header("X-Other", "value")},
		{name: "off/bearer without token", passthrough: false, headers: header("Authorization", "Bearer ")},
		{name: "off/basic scheme", passthrough: false, headers: header("Authorization", "Basic dGVzdDp0ZXN0")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ResolvePassthroughToken(tt.passthrough, tt.headers)
			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if token != tt.wantToken {
				t.Fatalf("token = %q, want %q", token, tt.wantToken)
			}
			if err != nil && err.Error() != "Bearer token required when TOKEN_PASSTHROUGH is true" {
				t.Fatalf("error text = %q, want the message kubectl tools have always returned", err.Error())
			}
		})
	}
}

func TestTokenPassthroughEnabled(t *testing.T) {
	for _, tt := range []struct {
		value string
		set   bool
		want  bool
	}{
		{value: "true", set: true, want: true},
		{value: "false", set: true, want: false},
		{value: "TRUE", set: true, want: false},
		{value: "1", set: true, want: false},
		{value: "", set: true, want: false},
		{set: false, want: false},
	} {
		name := "unset"
		if tt.set {
			name = "set to " + tt.value
		}
		t.Run(name, func(t *testing.T) {
			// t.Setenv restores the original value after the test, also
			// when the variable is then unset.
			t.Setenv(TokenPassthroughEnv, tt.value)
			if !tt.set {
				if err := os.Unsetenv(TokenPassthroughEnv); err != nil {
					t.Fatal(err)
				}
			}
			if got := TokenPassthroughEnabled(); got != tt.want {
				t.Fatalf("TokenPassthroughEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPassthroughTokenReadsEnvironment(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer test-token")

	t.Setenv(TokenPassthroughEnv, "true")
	if token, err := PassthroughToken(headers); err != nil || token != "test-token" {
		t.Fatalf("passthrough on: got (%q, %v), want (%q, nil)", token, err, "test-token")
	}
	if _, err := PassthroughToken(nil); !errors.Is(err, ErrBearerTokenRequired) {
		t.Fatalf("passthrough on without header: err = %v, want %v", err, ErrBearerTokenRequired)
	}

	t.Setenv(TokenPassthroughEnv, "false")
	if token, err := PassthroughToken(headers); err != nil || token != "" {
		t.Fatalf("passthrough off: got (%q, %v), want (\"\", nil)", token, err)
	}
}
