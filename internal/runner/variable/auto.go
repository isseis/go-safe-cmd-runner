// Package variable provides definitions and utilities for automatically generate variables
package variable

import (
	"time"
)

// Clock is a function type that returns the current time.
// This allows for dependency injection of time for testing.
type Clock func() time.Time

const (
	// DatetimeLayout is the Go time format for __runner_datetime
	// Format: YYYYMMDDHHmmSS.msec (e.g., "20251005143025.123")
	DatetimeLayout = "20060102150405.000" // Go time format for YYYYMMDDHHmmSS.msec

	// AutoVarPrefix is the prefix for automatically generated internal variables (lowercase format)
	AutoVarPrefix = "__runner_"

	// AutoVarKeyDatetime is the key for the datetime auto internal variable (without prefix)
	AutoVarKeyDatetime = "datetime"
	// AutoVarKeyPID is the key for the PID auto internal variable (without prefix)
	AutoVarKeyPID = "pid"
	// AutoVarKeyWorkDir is the key for the workdir auto internal variable (without prefix)
	AutoVarKeyWorkDir = "workdir"
)

// WorkDirKey returns the auto variable key used to store the runner
// working directory for a group during execution.
func WorkDirKey() string {
	return AutoVarPrefix + AutoVarKeyWorkDir
}
