// Package redact removes credentials from command arguments and free text
// before they reach logs, span attributes, span status or tool results.
//
// It is a safety net, not the primary control: credentials should never be put
// on a command line in the first place (see internal/commands, which passes
// bearer tokens to CLIs through a per-call kubeconfig instead of --token). The
// functions here make sure that, if a credential is ever added to an argument
// list or an error message, the text that leaves the process does not carry it.
package redact

import (
	"regexp"
	"strings"
)

// Placeholder replaces every redacted value.
const Placeholder = "<REDACTED>"

// sensitiveFlags take a credential (or a path to one) as their value, either as
// the next argument ("--token X") or inline ("--token=X").
var sensitiveFlags = []string{
	"--token",
	"--kube-token",
	"--password",
	"--client-key",
}

// assignmentPattern matches key=value assignments whose key names a credential,
// such as "--set db.password=x" or "aws_secret=x". The value starts after "=".
var assignmentPattern = regexp.MustCompile(`(?i)(token|password|secret)=`)

// stringFlagPattern matches a sensitive flag and its value in free text,
// separated by "=" or whitespace.
var stringFlagPattern = regexp.MustCompile(`(--(?:token|kube-token|password|client-key))(=|\s+)\S+`)

// stringAssignmentPattern matches a credential assignment in free text.
var stringAssignmentPattern = regexp.MustCompile(`(?i)((?:token|password|secret)=)\S+`)

// bearerPattern matches an HTTP bearer credential in free text.
var bearerPattern = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)

// Args returns a copy of args with credential values replaced by Placeholder.
// The value following --token, --kube-token, --password and --client-key is
// redacted, as is the inline form (--token=X), and the value of any argument
// containing an assignment whose key matches token, password or secret
// (case-insensitive). A nil or empty slice yields nil.
func Args(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out); i++ {
		arg := out[i]
		if isSensitiveFlag(arg) {
			if i+1 < len(out) {
				out[i+1] = Placeholder
				i++ // the value has been handled
			}
			continue
		}
		if flag, ok := sensitiveInlineFlag(arg); ok {
			out[i] = flag + "=" + Placeholder
			continue
		}
		if loc := assignmentPattern.FindStringIndex(arg); loc != nil {
			out[i] = arg[:loc[1]] + Placeholder
		}
	}
	return out
}

// String returns s with credentials replaced by Placeholder. It covers the same
// flags and assignments as Args, written as text (for example a command line
// joined with spaces, or an error message that quotes one), and bearer
// credentials ("Bearer X").
func String(s string) string {
	if s == "" {
		return s
	}
	s = stringFlagPattern.ReplaceAllString(s, "${1}${2}"+Placeholder)
	s = stringAssignmentPattern.ReplaceAllString(s, "${1}"+Placeholder)
	s = bearerPattern.ReplaceAllString(s, "${1}"+Placeholder)
	return s
}

// Error wraps err so that its message is redacted with String. It returns nil
// for a nil error. The original error stays reachable through errors.Unwrap.
func Error(err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{msg: String(err.Error()), cause: err}
}

type redactedError struct {
	msg   string
	cause error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.cause }

func isSensitiveFlag(arg string) bool {
	for _, f := range sensitiveFlags {
		if arg == f {
			return true
		}
	}
	return false
}

func sensitiveInlineFlag(arg string) (string, bool) {
	for _, f := range sensitiveFlags {
		if strings.HasPrefix(arg, f+"=") {
			return f, true
		}
	}
	return "", false
}
