package common

import (
	"fmt"
	"strings"
)

// ValidatePathIdentifier rejects value when it is empty, ".", or ".." -- the three inputs
// path.Join (used throughout this codebase's GetPath methods to build a request path from a
// caller-supplied identifier) silently resolves into something other than a literal path
// segment: "" collapses away entirely (turning ".../commits/" into ".../commits", the list
// endpoint, not a 404), "." is a no-op (same effect), and ".." removes the previous segment
// entirely (turning ".../downloads/.." into ".../", the parent resource) -- and also rejects any
// value containing a path separator ("/" or "\\"), literal or percent-encoded ("%2f"/"%2F"/
// "%5c"/"%5C"), since every identifier this function guards (a pull request id, a comment or task
// id, a pipeline id, a pipeline step UUID or name, ...) is meant to be exactly one path segment: a
// value carrying an extra separator -- with or without ".." in it -- lets path.Join splice in
// additional segments of the caller's choosing, up to and including escaping the repository's own
// path prefix entirely. A caller-supplied value that legitimately spans two segments (the
// "workspace/repository" form GetRepositoryBySlugOrID accepts for --repository) is validated on
// its own terms by that call site instead of going through this function. Call this at the point
// an identifier argument is accepted, before it ever reaches a GetPath call, naming the argument
// for the error.
func ValidatePathIdentifier(name, value string) error {
	switch value {
	case "":
		return fmt.Errorf("argument %s is missing", name)
	case ".", "..":
		return fmt.Errorf("argument %s is invalid (value: %s)", name, value)
	}
	lowered := strings.ToLower(value)
	if strings.ContainsAny(value, "/\\") || strings.Contains(lowered, "%2f") || strings.Contains(lowered, "%5c") {
		return fmt.Errorf("argument %s is invalid (value: %s)", name, value)
	}
	return nil
}
