package common

import (
	"slices"
	"strings"
)

// FilterValidArgs filters the valid arguments and keeps only the ones
// that match the toComplete string and that are not present in args
//
// Note: the result is a new slice, the original is not modified
func FilterValidArgs(valid, args []string, toComplete string) []string {
	if toComplete != "" {
		valid = Filter(valid, func(value string) bool {
			return strings.HasPrefix(value, toComplete)
		})
	}
	if len(args) > 0 {
		valid = Filter(valid, func(value string) bool {
			return !slices.Contains(args, value)
		})
	}
	return valid
}
