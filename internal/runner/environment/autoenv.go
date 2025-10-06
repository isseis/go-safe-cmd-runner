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
	// AutoEnvPrefix is the prefix for automatically generated environment variables
	AutoEnvPrefix = "__RUNNER_"

	// AutoEnvKeyDatetime is the key for the datetime auto environment variable (without prefix)
	AutoEnvKeyDatetime = "DATETIME"
	// AutoEnvKeyPID is the key for the PID auto environment variable (without prefix)
	AutoEnvKeyPID = "PID"

	// DatetimeLayout is the Go time format for __RUNNER_DATETIME
	// Format: YYYYMMDDHHMM.msec (e.g., "202510051430.123")
	DatetimeLayout = "200601021504.000" // Go time format for YYYYMMDDHHMM.msec
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

// Generate returns all auto environment variables as a map
func (p *autoEnvProvider) Generate() map[string]string {
	return map[string]string{
		AutoEnvPrefix + AutoEnvKeyDatetime: p.generateDateTime(),
		AutoEnvPrefix + AutoEnvKeyPID:      p.generatePID(),
	}
}

// generateDateTime generates the datetime string in YYYYMMDDHHMM.msec format (UTC)
func (p *autoEnvProvider) generateDateTime() string {
	return p.clock().UTC().Format(DatetimeLayout)
}

// generatePID generates the PID string
func (p *autoEnvProvider) generatePID() string {
	return strconv.Itoa(os.Getpid())
}
