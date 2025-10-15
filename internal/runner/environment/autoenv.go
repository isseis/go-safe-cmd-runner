package environment

import (
	"os"
	"strconv"
	"time"
)

// Clock is a function type that returns the current time.
// This allows for dependency injection of time for testing.
type Clock func() time.Time

const (
	// AutoEnvPrefix is the prefix for automatically generated environment variables (uppercase format)
	AutoEnvPrefix = "__RUNNER_"

	// AutoEnvKeyDatetime is the key for the datetime auto environment variable (without prefix)
	AutoEnvKeyDatetime = "DATETIME"
	// AutoEnvKeyPID is the key for the PID auto environment variable (without prefix)
	AutoEnvKeyPID = "PID"

	// DatetimeLayout is the Go time format for __RUNNER_DATETIME and __runner_datetime
	// Format: YYYYMMDDHHmmSS.msec (e.g., "20251005143025.123")
	DatetimeLayout = "20060102150405.000" // Go time format for YYYYMMDDHHmmSS.msec

	// AutoVarPrefix is the prefix for automatically generated internal variables (lowercase format)
	AutoVarPrefix = "__runner_"

	// AutoVarKeyDatetime is the key for the datetime auto internal variable (without prefix)
	AutoVarKeyDatetime = "datetime"
	// AutoVarKeyPID is the key for the PID auto internal variable (without prefix)
	AutoVarKeyPID = "pid"
)

// AutoEnvProvider provides automatic environment variables
type AutoEnvProvider interface {
	// Generate returns all auto environment variables as a map.
	// All keys have the AutoEnvPrefix (__RUNNER_).
	Generate() map[string]string
}

// autoEnvProvider implements AutoEnvProvider
type autoEnvProvider struct {
	clock Clock
}

// NewAutoEnvProvider creates a new AutoEnvProvider.
// If clock is nil, it defaults to time.Now.
func NewAutoEnvProvider(clock Clock) AutoEnvProvider {
	if clock == nil {
		clock = time.Now
	}
	return &autoEnvProvider{
		clock: clock,
	}
}

// Generate returns all auto environment variables and internal variables as a map.
// This includes both:
//   - Environment variables (uppercase): __RUNNER_DATETIME, __RUNNER_PID
//   - Internal variables (lowercase): __runner_datetime, __runner_pid
func (p *autoEnvProvider) Generate() map[string]string {
	now := p.clock()
	return map[string]string{
		// Environment variables (uppercase format)
		AutoEnvPrefix + AutoEnvKeyDatetime: now.UTC().Format(DatetimeLayout),
		AutoEnvPrefix + AutoEnvKeyPID:      strconv.Itoa(os.Getpid()),
		// Internal variables (lowercase format)
		AutoVarPrefix + AutoVarKeyDatetime: now.UTC().Format(DatetimeLayout),
		AutoVarPrefix + AutoVarKeyPID:      strconv.Itoa(os.Getpid()),
	}
}
