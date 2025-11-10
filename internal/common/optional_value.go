// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// Numeric is a constraint for numeric types that can be used with OptionalValue.
type Numeric interface {
	~int | ~int64
}

// OptionalValue represents an optional configuration value that can be:
// - Unset (nil) - use default or inherit from parent
// - Zero (0) - explicitly set to unlimited/disabled
// - Positive value - explicitly set to a specific value
//
// This type provides type safety and explicit semantics compared to using *T directly.
type OptionalValue[T Numeric] struct {
	value *T
}

// NewOptionalValueFromPtr creates an OptionalValue from an existing pointer.
func NewOptionalValueFromPtr[T Numeric](ptr *T) OptionalValue[T] {
	return OptionalValue[T]{value: ptr}
}

// NewUnsetOptionalValue creates an unset OptionalValue (will use default or inherit from parent).
func NewUnsetOptionalValue[T Numeric]() OptionalValue[T] {
	return OptionalValue[T]{value: nil}
}

// NewUnlimitedOptionalValue creates an OptionalValue with unlimited/disabled setting (0).
func NewUnlimitedOptionalValue[T Numeric]() OptionalValue[T] {
	var zero T
	return OptionalValue[T]{value: &zero}
}

// NewOptionalValue creates an OptionalValue with the specified value.
func NewOptionalValue[T Numeric](value T) OptionalValue[T] {
	return OptionalValue[T]{value: &value}
}

// IsSet returns true if the value has been explicitly set (non-nil).
func (o OptionalValue[T]) IsSet() bool {
	return o.value != nil
}

// IsUnlimited returns true if the value is explicitly set to unlimited/disabled (0).
// Returns false if the value is unset (nil).
func (o OptionalValue[T]) IsUnlimited() bool {
	return o.value != nil && *o.value == 0
}

// Value returns the value.
// Panics if the value is not set (IsSet() == false).
// Callers must check IsSet() before calling Value().
// For unlimited value, returns 0. Callers should use IsUnlimited() to distinguish between unlimited (zero) and set non-zero values.
func (o OptionalValue[T]) Value() T {
	if o.value == nil {
		panic("OptionalValue.Value() called on unset value: use IsSet() to check if the value is set before calling Value()")
	}
	return *o.value
}

// Ptr returns the underlying pointer (can be nil).
// This is useful for serialization or when you need to distinguish between unset and zero.
func (o OptionalValue[T]) Ptr() *T {
	return o.value
}
