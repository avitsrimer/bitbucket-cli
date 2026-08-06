package common

import "fmt"

// ValidatePathIdentifier rejects value when it is empty, ".", or ".." -- the three inputs
// path.Join (used throughout this codebase's GetPath methods to build a request path from a
// caller-supplied identifier) silently resolves into something other than a literal path
// segment: "" collapses away entirely (turning ".../commits/" into ".../commits", the list
// endpoint, not a 404), "." is a no-op (same effect), and ".." removes the previous segment
// entirely (turning ".../downloads/.." into ".../", the parent resource). url.PathEscape handles
// every other character a caller might pass (including "/", "%", "?", "#") but leaves these three
// untouched, since they are valid path segments syntactically -- just never the ones the caller
// meant. Call this at the point an identifier argument is accepted, before it ever reaches a
// GetPath call, naming the argument for the error.
func ValidatePathIdentifier(name, value string) error {
	switch value {
	case "":
		return fmt.Errorf("argument %s is missing", name)
	case ".", "..":
		return fmt.Errorf("argument %s is invalid (value: %s)", name, value)
	}
	return nil
}
