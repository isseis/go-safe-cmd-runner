// Package common provides shared validation functions used throughout the command runner.
package common

import (
	"fmt"
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
