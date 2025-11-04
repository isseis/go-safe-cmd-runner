//nolint:revive // common is an appropriate name for shared utilities package
package common

import (
	"testing"
)

func TestCloneOrEmpty(t *testing.T) {
	t.Run("returns copy of non-nil slice", func(t *testing.T) {
		input := []string{"a", "b", "c"}
		result := CloneOrEmpty(input)

		// Check length
		if len(result) != len(input) {
			t.Errorf("expected length %d, got %d", len(input), len(result))
		}

		// Check contents
		for i, v := range input {
			if result[i] != v {
				t.Errorf("expected result[%d] = %q, got %q", i, v, result[i])
			}
		}

		// Verify it's a copy (different underlying array)
		input[0] = "modified"
		if result[0] == "modified" {
			t.Error("result should be a copy, not share underlying array")
		}
	})

	t.Run("returns empty slice for nil input", func(t *testing.T) {
		var input []string
		result := CloneOrEmpty(input)

		if result == nil {
			t.Error("expected non-nil slice, got nil")
		}

		if len(result) != 0 {
			t.Errorf("expected empty slice, got length %d", len(result))
		}
	})

	t.Run("returns empty slice for empty input", func(t *testing.T) {
		input := []string{}
		result := CloneOrEmpty(input)

		if result == nil {
			t.Error("expected non-nil slice, got nil")
		}

		if len(result) != 0 {
			t.Errorf("expected empty slice, got length %d", len(result))
		}
	})

	t.Run("preserves all elements", func(t *testing.T) {
		input := []string{"VAR1", "VAR2", "VAR3", "VAR4"}
		result := CloneOrEmpty(input)

		if len(result) != len(input) {
			t.Errorf("expected length %d, got %d", len(input), len(result))
		}

		for i, expected := range input {
			if result[i] != expected {
				t.Errorf("expected result[%d] = %q, got %q", i, expected, result[i])
			}
		}
	})
}

func TestSetDifferenceToSlice(t *testing.T) {
	t.Run("returns elements in setA not in setB", func(t *testing.T) {
		setA := map[string]struct{}{
			"a": {},
			"b": {},
			"c": {},
		}
		setB := map[string]struct{}{
			"b": {},
		}

		result := SetDifferenceToSlice(setA, setB)

		expected := []string{"a", "c"}
		if len(result) != len(expected) {
			t.Errorf("expected length %d, got %d", len(expected), len(result))
		}

		for i, v := range expected {
			if result[i] != v {
				t.Errorf("expected result[%d] = %q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("returns empty slice when setA is subset of setB", func(t *testing.T) {
		setA := map[string]struct{}{
			"a": {},
			"b": {},
		}
		setB := map[string]struct{}{
			"a": {},
			"b": {},
			"c": {},
		}

		result := SetDifferenceToSlice(setA, setB)

		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})

	t.Run("returns all elements when setB is empty", func(t *testing.T) {
		setA := map[string]struct{}{
			"a": {},
			"b": {},
			"c": {},
		}
		setB := map[string]struct{}{}

		result := SetDifferenceToSlice(setA, setB)

		expected := []string{"a", "b", "c"}
		if len(result) != len(expected) {
			t.Errorf("expected length %d, got %d", len(expected), len(result))
		}

		for i, v := range expected {
			if result[i] != v {
				t.Errorf("expected result[%d] = %q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("returns sorted results", func(t *testing.T) {
		setA := map[string]struct{}{
			"zebra": {},
			"apple": {},
			"mango": {},
		}
		setB := map[string]struct{}{}

		result := SetDifferenceToSlice(setA, setB)

		expected := []string{"apple", "mango", "zebra"}
		if len(result) != len(expected) {
			t.Errorf("expected length %d, got %d", len(expected), len(result))
		}

		for i, v := range expected {
			if result[i] != v {
				t.Errorf("expected result[%d] = %q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("works with integer sets", func(t *testing.T) {
		setA := map[int]struct{}{
			1: {},
			2: {},
			3: {},
			4: {},
		}
		setB := map[int]struct{}{
			2: {},
			4: {},
		}

		result := SetDifferenceToSlice(setA, setB)

		expected := []int{1, 3}
		if len(result) != len(expected) {
			t.Errorf("expected length %d, got %d", len(expected), len(result))
		}

		for i, v := range expected {
			if result[i] != v {
				t.Errorf("expected result[%d] = %d, got %d", i, v, result[i])
			}
		}
	})

	t.Run("handles empty setA", func(t *testing.T) {
		setA := map[string]struct{}{}
		setB := map[string]struct{}{
			"a": {},
			"b": {},
		}

		result := SetDifferenceToSlice(setA, setB)

		if len(result) != 0 {
			t.Errorf("expected empty result, got %v", result)
		}
	})
}

func TestSliceToSet(t *testing.T) {
	t.Run("converts string slice to set", func(t *testing.T) {
		input := []string{"apple", "banana", "cherry"}
		result := SliceToSet(input)

		if len(result) != 3 {
			t.Errorf("expected set size 3, got %d", len(result))
		}

		for _, item := range input {
			if _, exists := result[item]; !exists {
				t.Errorf("expected %q to be in set", item)
			}
		}
	})

	t.Run("removes duplicates", func(t *testing.T) {
		input := []string{"a", "b", "a", "c", "b"}
		result := SliceToSet(input)

		if len(result) != 3 {
			t.Errorf("expected set size 3 (duplicates removed), got %d", len(result))
		}

		expected := map[string]struct{}{"a": {}, "b": {}, "c": {}}
		for key := range expected {
			if _, exists := result[key]; !exists {
				t.Errorf("expected %q to be in set", key)
			}
		}
	})

	t.Run("handles empty slice", func(t *testing.T) {
		input := []string{}
		result := SliceToSet(input)

		if len(result) != 0 {
			t.Errorf("expected empty set, got size %d", len(result))
		}
	})

	t.Run("handles nil slice", func(t *testing.T) {
		var input []string
		result := SliceToSet(input)

		if len(result) != 0 {
			t.Errorf("expected empty set for nil slice, got size %d", len(result))
		}
	})

	t.Run("works with integers", func(t *testing.T) {
		input := []int{1, 2, 3, 2, 1}
		result := SliceToSet(input)

		if len(result) != 3 {
			t.Errorf("expected set size 3, got %d", len(result))
		}

		for _, num := range []int{1, 2, 3} {
			if _, exists := result[num]; !exists {
				t.Errorf("expected %d to be in set", num)
			}
		}
	})

	t.Run("works with custom comparable type", func(t *testing.T) {
		type Status int
		const (
			Pending Status = iota
			Active
			Completed
		)

		input := []Status{Pending, Active, Completed, Active}
		result := SliceToSet(input)

		if len(result) != 3 {
			t.Errorf("expected set size 3, got %d", len(result))
		}

		for _, status := range []Status{Pending, Active, Completed} {
			if _, exists := result[status]; !exists {
				t.Errorf("expected status %d to be in set", status)
			}
		}
	})

	t.Run("preserves all unique elements", func(t *testing.T) {
		input := []string{"VAR1", "VAR2", "VAR3", "VAR1"}
		result := SliceToSet(input)

		if len(result) != 3 {
			t.Errorf("expected set size 3, got %d", len(result))
		}

		// Verify all unique elements are present
		expected := []string{"VAR1", "VAR2", "VAR3"}
		for _, item := range expected {
			if _, exists := result[item]; !exists {
				t.Errorf("expected %q to be in set", item)
			}
		}
	})

	t.Run("large slice performance", func(t *testing.T) {
		// Create a large slice
		input := make([]int, 10000)
		for i := range input {
			input[i] = i
		}

		result := SliceToSet(input)

		if len(result) != 10000 {
			t.Errorf("expected set size 10000, got %d", len(result))
		}

		// Verify a few random elements
		for _, num := range []int{0, 5000, 9999} {
			if _, exists := result[num]; !exists {
				t.Errorf("expected %d to be in set", num)
			}
		}
	})
}
