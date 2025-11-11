// Package variable provides definitions and utilities for automatically generate variables
package variable

import (
	"fmt"
	"os"
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

// DatetimeKey returns the auto variable key for the runner datetime.
func DatetimeKey() string {
	return AutoVarPrefix + AutoVarKeyDatetime
}

// PIDKey returns the auto variable key for the runner PID.
func PIDKey() string {
	return AutoVarPrefix + AutoVarKeyPID
}

// GenerateAutoVars generates automatic variables (__runner_datetime and __runner_pid)
// and returns them as a map. This should be called once at configuration load time.
// The clock parameter allows for time injection for testing; pass time.Now for production use.
func GenerateAutoVars(clock Clock) map[string]string {
	if clock == nil {
		clock = time.Now
	}

	autoVars := make(map[string]string)

	// Generate __runner_datetime in UTC with format YYYYMMDDHHmmSS.msec
	now := clock().UTC()
	autoVars[DatetimeKey()] = now.Format(DatetimeLayout)

	// Generate __runner_pid
	autoVars[PIDKey()] = fmt.Sprintf("%d", os.Getpid())

	return autoVars
}
