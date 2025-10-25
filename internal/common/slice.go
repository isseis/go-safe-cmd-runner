//nolint:revive // "common" is an appropriate name for shared utilities package
package common

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
