// Package common provides shared validation functions used throughout the command runner.
package common

import (
	"fmt"
	"reflect"
)

// ErrInvalidTimeout is returned when an invalid timeout value is encountered
type ErrInvalidTimeout struct {
	Value   interface{}
	Context string
}

func (e ErrInvalidTimeout) Error() string {
	return fmt.Sprintf("invalid timeout value %v in %s", e.Value, e.Context)
}

// ValidateTimeout validates timeout configuration.
// It accepts *int values and validates them according to the timeout specification:
// - nil: valid (unset, will use default)
// - *0: valid (unlimited execution)
// - *N (N>0 && N<=MaxTimeout): valid (N seconds timeout)
// - *N (N<0 || N>MaxTimeout): invalid
func ValidateTimeout(timeout *int, context string) error {
	if timeout == nil {
		// Unset timeout is valid - will use DefaultTimeout
		return nil
	}

	value := *timeout

	// Negative timeouts are invalid
	if value < 0 {
		return ErrInvalidTimeout{
			Value:   value,
			Context: context + " (negative timeouts not allowed)",
		}
	}

	// Zero timeout is valid (unlimited execution)
	if value == 0 {
		return nil
	}

	// Check maximum timeout limit
	if value > MaxTimeout {
		return ErrInvalidTimeout{
			Value:   value,
			Context: context + fmt.Sprintf(" (exceeds maximum timeout of %d seconds)", MaxTimeout),
		}
	}

	// Positive timeout within limits is valid
	return nil
}

// ParseTimeoutValue converts TOML value to *int for timeout configuration.
// It handles the following cases:
// - nil/missing field: returns nil (unset)
// - int/int32/int64: converts to *int with validation
// - other types: returns error
func ParseTimeoutValue(value interface{}) (*int, error) {
	if value == nil {
		// Missing field in TOML - return nil (unset)
		return nil, nil
	}

	// Convert to int based on type
	var result int
	switch v := value.(type) {
	case int:
		result = v
	case int64:
		// TOML often parses integers as int64
		// Check for overflow before conversion
		maxInt := int64(^uint(0) >> 1)
		minInt := ^maxInt
		if v > maxInt || v < minInt {
			return nil, ErrInvalidTimeout{
				Value:   v,
				Context: "TOML timeout field (int overflow)",
			}
		}
		result = int(v)
	case int32:
		result = int(v)
	default:
		// Unsupported type
		return nil, ErrInvalidTimeout{
			Value:   value,
			Context: fmt.Sprintf("TOML timeout field (unsupported type: %s)", reflect.TypeOf(value)),
		}
	}

	// Validate the converted value once
	if err := ValidateTimeout(&result, "TOML timeout field"); err != nil {
		return nil, err
	}

	return &result, nil
}

// IsValidTimeoutValue checks if a given *int is a valid timeout value
// without converting from TOML. This is useful for runtime validation.
func IsValidTimeoutValue(timeout *int) bool {
	return ValidateTimeout(timeout, "timeout validation") == nil
}
