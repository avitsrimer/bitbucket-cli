package common

import (
	"fmt"
	"strings"
)

// ValidatePathIdentifier rejects value when it is empty, or is "." or ".." either literally or
// after decoding any case-insensitive "%2e" percent-encoding it carries -- the two effective
// values path.Join (used throughout this codebase's GetPath methods to build a request path from a
// caller-supplied identifier) silently resolves into something other than a literal path segment:
// "" collapses away entirely (turning ".../commits/" into ".../commits", the list endpoint, not a
// 404), "." is a no-op (same effect), and ".." removes the previous segment entirely (turning
// ".../downloads/.." into ".../", the parent resource). The percent-encoded form matters because
// this codebase's own path.Join never decodes anything, but the resulting request URL is still
// subject to normal URL-path decoding further along the request path, so "%2e%2e" reaches the
// server as ".." just as surely as a literal ".." would. It also rejects any value containing a
// path separator ("/" or "\\"), literal or percent-encoded ("%2f"/"%2F"/"%5c"/"%5C"), since every
// identifier this function guards (a pull request id, a comment or task id, a pipeline id, a
// pipeline step UUID or name, ...) is meant to be exactly one path segment: a value carrying an
// extra separator -- with or without ".." in it -- lets path.Join splice in additional segments of
// the caller's choosing, up to and including escaping the repository's own path prefix entirely. A
// caller-supplied value that legitimately spans two segments (the "workspace/repository" form
// GetRepositoryBySlugOrID accepts for --repository) is validated on its own terms by that call site
// instead of going through this function. Call this at the point an identifier argument is
// accepted, before it ever reaches a GetPath call, naming the argument for the error.
func ValidatePathIdentifier(name, value string) error {
	if value == "" {
		return fmt.Errorf("argument %s is missing", name)
	}
	lowered := strings.ToLower(value)
	if strings.ContainsAny(value, "/\\") || strings.Contains(lowered, "%2f") || strings.Contains(lowered, "%5c") {
		return fmt.Errorf("argument %s is invalid (value: %s)", name, value)
	}
	// Decoding every "%2e" occurrence (value is already known lowercase here) to "." before this
	// comparison is what catches a fully or partially percent-encoded "." or ".." (e.g. "%2e%2e",
	// ".%2e", "%2e."), on top of the literal form the direct comparison already covered. A value
	// that legitimately contains a literal "." elsewhere (e.g. "a%2eb", decoding to "a.b") does not
	// equal either target string, so it is left alone.
	if decoded := strings.ReplaceAll(lowered, "%2e", "."); decoded == "." || decoded == ".." {
		return fmt.Errorf("argument %s is invalid (value: %s)", name, value)
	}
	return nil
}

// ValidatePathRef validates value as a slash-separated ref (a git branch or tag name, as accepted
// by e.g. Bitbucket's diff/patch endpoints alongside a bare commit hash) by applying
// ValidatePathIdentifier's rules to each '/'-delimited segment individually: every segment must be
// present, must not be "." or "..", and must not contain a literal or percent-encoded path
// separator. A single-segment value (no "/" at all) is validated exactly as
// ValidatePathIdentifier would, preserving its error messages for that common case (a bare commit
// hash). This keeps path traversal impossible -- no segment can smuggle in ".." for GetPath's
// underlying path.Join to collapse -- while still accepting a multi-segment ref like "release/1.0"
// or "feature/a/b", which ValidatePathIdentifier's single-segment contract rejects outright by
// disallowing "/" entirely. Call this instead of ValidatePathIdentifier at any call site where the
// argument may legitimately be a ref, not just a bare identifier.
func ValidatePathRef(name, value string) error {
	segments := strings.Split(value, "/")
	if len(segments) == 1 {
		return ValidatePathIdentifier(name, value)
	}
	for _, segment := range segments {
		if err := ValidatePathIdentifier(name, segment); err != nil {
			return fmt.Errorf("argument %s is invalid (value: %s)", name, value)
		}
	}
	return nil
}
