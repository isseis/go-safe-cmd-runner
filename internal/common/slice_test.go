//go:build test
// +build test

package common

import (
	"testing"
)

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
