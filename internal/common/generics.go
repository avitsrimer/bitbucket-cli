package common

import "slices"

// Sort sorts items in place using less as a strict less-than comparator.
//
// less(a, b) reporting true means a sorts before b; a and b comparing equal (less(a,b) and
// less(b,a) both false) is order-preserving, matching slices.SortFunc's stability contract.
func Sort[S ~[]T, T any](items S, less func(a, b T) bool) {
	slices.SortFunc(items, func(a, b T) int {
		if less(a, b) {
			return -1
		} else if less(b, a) {
			return 1
		}
		return 0
	})
}

// Map applies mapper to every item of items and returns the results in a new slice.
//
// A nil or empty items yields a nil result, never an empty non-nil slice: callers that
// JSON-marshal the result with `omitempty`, or distinguish "no items" from "unknown", rely
// on that nil.
func Map[S ~[]T, T any, R any](items S, mapper func(T) R) []R {
	var result []R //nolint:prealloc // must stay nil for an empty/nil items, not empty-non-nil; preallocating with make([]R, 0, len(items)) would break that
	for _, item := range items {
		result = append(result, mapper(item))
	}
	return result
}

// Filter returns the items for which keep reports true, in a new slice.
//
// A nil or empty items, or one with no matches, yields a nil result, never an empty
// non-nil slice: callers that JSON-marshal the result with `omitempty`, or distinguish "no
// items" from "unknown", rely on that nil.
func Filter[S ~[]T, T any](items S, keep func(T) bool) []T {
	var result []T
	for _, item := range items {
		if keep(item) {
			result = append(result, item)
		}
	}
	return result
}
