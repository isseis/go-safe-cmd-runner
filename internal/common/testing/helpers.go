//go:build test

package testing

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// ErrInvalidTimeout is returned when an invalid timeout value is encountered
type ErrInvalidTimeout struct {
	Value   any
	Context string
}

func (e ErrInvalidTimeout) Error() string {
	return fmt.Sprintf("invalid timeout value %v in %s", e.Value, e.Context)
}

// NewUnlimitedTimeout creates a Timeout with unlimited execution (no timeout).
func NewUnlimitedTimeout() common.Timeout {
	return common.Timeout{OptionalValue: common.NewOptionalValue[int32](0)}
}

// NewTimeout creates a Timeout with the specified duration in seconds.
// Returns error if seconds is negative or exceeds MaxTimeout.
func NewTimeout(seconds int32) (common.Timeout, error) {
	if seconds < 0 {
		return common.Timeout{}, ErrInvalidTimeout{
			Value:   seconds,
			Context: "timeout cannot be negative",
		}
	}
	if seconds > common.MaxTimeout {
		return common.Timeout{}, ErrInvalidTimeout{
			Value:   seconds,
			Context: "timeout exceeds maximum allowed value",
		}
	}
	return common.Timeout{OptionalValue: common.NewOptionalValue(seconds)}, nil
}

// NewUnsetOutputSizeLimit creates an unset OutputSizeLimit (will use default or inherit from parent).
func NewUnsetOutputSizeLimit() common.OutputSizeLimit {
	return common.OutputSizeLimit{OptionalValue: common.NewUnsetOptionalValue[int64]()}
}

// NewUnlimitedOutputSizeLimit creates an OutputSizeLimit with unlimited output (no limit).
func NewUnlimitedOutputSizeLimit() common.OutputSizeLimit {
	return common.OutputSizeLimit{OptionalValue: common.NewOptionalValue[int64](0)}
}

// Int32Ptr returns a pointer to the given int value.
// This is a convenience function for creating timeout values.
func Int32Ptr(v int32) *int32 {
	return &v
}

// Int64Ptr returns a pointer to the given int64 value.
// This is a convenience function for creating output size limit values.
func Int64Ptr(v int64) *int64 {
	return &v
}

// BoolPtr returns a pointer to the given bool value.
// This is a convenience function for creating pointer values in tests and configuration.
func BoolPtr(v bool) *bool {
	return &v
}
