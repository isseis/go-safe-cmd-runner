package variable

import (
	"os"
	"strconv"
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
)

// AutoVarProvider provides automatic internal variables
type AutoVarProvider interface {
	// Generate returns all auto internal variables as a map.
	// All keys have the AutoVarPrefix (__runner_).
	Generate() map[string]string
}

// autoVarProvider implements AutoVarProvider
type autoVarProvider struct {
	clock Clock
}

// NewAutoVarProvider creates a new AutoVarProvider.
// If clock is nil, it defaults to time.Now.
func NewAutoVarProvider(clock Clock) AutoVarProvider {
	if clock == nil {
		clock = time.Now
	}
	return &autoVarProvider{
		clock: clock,
	}
}

// Generate returns all auto internal variables as a map.
// This includes:
//   - Internal variables (lowercase): __runner_datetime, __runner_pid
func (p *autoVarProvider) Generate() map[string]string {
	now := p.clock()
	return map[string]string{
		// Internal variables (lowercase format)
		AutoVarPrefix + AutoVarKeyDatetime: now.UTC().Format(DatetimeLayout),
		AutoVarPrefix + AutoVarKeyPID:      strconv.Itoa(os.Getpid()),
	}
}
