//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"cmp"
	"slices"
)

// CloneOrEmpty returns a copy of the slice or an empty slice if nil.
// This is useful when you need to ensure a non-nil slice is always returned,
// avoiding potential nil pointer issues in downstream code.
//
// Parameters:
//   - slice: input slice to copy (can be nil)
//
// Returns:
//   - []string: a copy of the input slice, or empty slice if input is nil
//
// Example:
//
//	var nilSlice []string
//	result := CloneOrEmpty(nilSlice)
//	// result = []string{} (not nil)
//
//	nonNil := []string{"a", "b"}
//	result = CloneOrEmpty(nonNil)
//	// result = []string{"a", "b"} (a copy)
func CloneOrEmpty(slice []string) []string {
	if slice == nil {
		return []string{}
	}
	return slices.Clone(slice)
}

// SetDifferenceToSlice returns elements in setA that are not in setB.
// The result is sorted for deterministic output.
//
// This is a generic function that works with any comparable type T.
//
// Parameters:
//   - setA: first set (elements to check)
//   - setB: second set (elements to exclude)
//
// Returns:
//   - []T: sorted slice of elements in setA but not in setB
//
// Example:
//
//	setA := map[string]struct{}{"a": {}, "b": {}, "c": {}}
//	setB := map[string]struct{}{"b": {}, "d": {}}
//	result := SetDifferenceToSlice(setA, setB)
//	// result = []string{"a", "c"} (sorted)
func SetDifferenceToSlice[T cmp.Ordered](setA, setB map[T]struct{}) []T {
	var result []T
	for key := range setA {
		if _, exists := setB[key]; !exists {
			result = append(result, key)
		}
	}
	slices.Sort(result)
	return result
}

// SliceToSet converts a slice to a set (map with struct{} values).
// This is useful for creating efficient O(1) lookup structures from slices.
//
// The function is generic and works with any comparable type T.
// Using struct{} as the value type minimizes memory usage (0 bytes per entry).
//
// Parameters:
//   - slice: input slice to convert
//
// Returns:
//   - map[T]struct{}: set representation of the input slice
//
// Example:
//
//	strings := []string{"a", "b", "c", "a"}
//	set := SliceToSet(strings)
//	// set = map[string]struct{}{"a": {}, "b": {}, "c": {}}
//
//	if _, exists := set["b"]; exists {
//	    // "b" is in the set
//	}
func SliceToSet[T comparable](slice []T) map[T]struct{} {
	set := make(map[T]struct{}, len(slice))
	for _, item := range slice {
		set[item] = struct{}{}
	}
	return set
}
