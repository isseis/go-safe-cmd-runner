// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// InheritanceMode represents how environment allowlist inheritance works
type InheritanceMode int

const (
	// InheritanceModeInherit indicates the group inherits from global allowlist
	// This occurs when env_allowlist field is not defined (nil slice)
	InheritanceModeInherit InheritanceMode = iota

	// InheritanceModeExplicit indicates the group uses only its explicit allowlist
	// This occurs when env_allowlist field has values: ["VAR1", "VAR2"]
	InheritanceModeExplicit

	// InheritanceModeReject indicates the group rejects all environment variables
	// This occurs when env_allowlist field is explicitly empty: []
	InheritanceModeReject
)

// LogLevel represents the logging level for the application.
// Valid values: debug, info, warn, error
type LogLevel string

const (
	// LogLevelDebug enables debug-level logging
	LogLevelDebug LogLevel = "debug"

	// LogLevelInfo enables info-level logging (default)
	LogLevelInfo LogLevel = "info"

	// LogLevelWarn enables warning-level logging
	LogLevelWarn LogLevel = "warn"

	// LogLevelError enables error-level logging only
	LogLevelError LogLevel = "error"
)

// ErrInvalidLogLevel is returned when an invalid log level is provided
var ErrInvalidLogLevel = errors.New("invalid log level")

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// This enables validation during TOML parsing.
func (l *LogLevel) UnmarshalText(text []byte) error {
	s := strings.ToLower(string(text))
	switch LogLevel(s) {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		*l = LogLevel(s)
		return nil
	case "":
		// Empty string defaults to info level
		*l = LogLevelInfo
		return nil
	default:
		return fmt.Errorf("%w: %q (must be one of: debug, info, warn, error)", ErrInvalidLogLevel, string(text))
	}
}

// ToSlogLevel converts LogLevel to slog.Level for use with the slog package.
func (l LogLevel) ToSlogLevel() (slog.Level, error) {
	switch strings.ToLower(string(l)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidLogLevel, l)
	}
}

// String returns the string representation of LogLevel.
func (l LogLevel) String() string {
	return string(l)
}

// RiskLevel represents the security risk level of a command
type RiskLevel int

const (
	// RiskLevelUnknown indicates commands whose risk level cannot be determined
	RiskLevelUnknown RiskLevel = iota

	// RiskLevelLow indicates commands with minimal security risk
	RiskLevelLow

	// RiskLevelMedium indicates commands with moderate security risk
	RiskLevelMedium

	// RiskLevelHigh indicates commands with high security risk
	RiskLevelHigh

	// RiskLevelCritical indicates commands that should be blocked (e.g., privilege escalation)
	RiskLevelCritical
)

// Risk level string constants used for string representation and parsing.
const (
	// UnknownRiskLevelString represents an unknown risk level.
	UnknownRiskLevelString = "unknown"
	// LowRiskLevelString represents a low risk level.
	LowRiskLevelString = "low"
	// MediumRiskLevelString represents a medium risk level.
	MediumRiskLevelString = "medium"
	// HighRiskLevelString represents a high risk level.
	HighRiskLevelString = "high"
	// CriticalRiskLevelString represents a critical risk level that blocks execution.
	CriticalRiskLevelString = "critical"
)

// String returns a string representation of RiskLevel
func (r RiskLevel) String() string {
	switch r {
	case RiskLevelUnknown:
		return UnknownRiskLevelString
	case RiskLevelLow:
		return LowRiskLevelString
	case RiskLevelMedium:
		return MediumRiskLevelString
	case RiskLevelHigh:
		return HighRiskLevelString
	case RiskLevelCritical:
		return CriticalRiskLevelString
	default:
		return UnknownRiskLevelString
	}
}

// ParseRiskLevel converts a string to RiskLevel for user configuration
// Critical level is prohibited in user configuration and reserved for internal use
func ParseRiskLevel(s string) (RiskLevel, error) {
	switch s {
	case UnknownRiskLevelString:
		return RiskLevelUnknown, nil
	case LowRiskLevelString:
		return RiskLevelLow, nil
	case MediumRiskLevelString:
		return RiskLevelMedium, nil
	case HighRiskLevelString:
		return RiskLevelHigh, nil
	case CriticalRiskLevelString:
		return RiskLevelUnknown, fmt.Errorf("%w: critical risk level cannot be set in configuration (reserved for internal use only)", ErrInvalidRiskLevel)
	case "":
		return RiskLevelLow, nil // Default to low risk for empty strings
	default:
		return RiskLevelUnknown, fmt.Errorf("%w: %s (supported: low, medium, high)", ErrInvalidRiskLevel, s)
	}
}

// String returns a string representation of InheritanceMode for logging
func (m InheritanceMode) String() string {
	switch m {
	case InheritanceModeInherit:
		return "inherit"
	case InheritanceModeExplicit:
		return "explicit"
	case InheritanceModeReject:
		return "reject"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler interface
// Returns the string representation of InheritanceMode for JSON output
func (m InheritanceMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON implements json.Unmarshaler interface
// Parses string representation of InheritanceMode from JSON
func (m *InheritanceMode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch s {
	case "inherit":
		*m = InheritanceModeInherit
	case "explicit":
		*m = InheritanceModeExplicit
	case "reject":
		*m = InheritanceModeReject
	default:
		return ErrInvalidInheritanceMode
	}

	return nil
}

// Operation represents different types of privileged operations
type Operation string

// Supported privileged operations
const (
	OperationFileHashCalculation Operation = "file_hash_calculation"
	OperationCommandExecution    Operation = "command_execution"
	OperationUserGroupExecution  Operation = "user_group_execution"
	OperationUserGroupDryRun     Operation = "user_group_dry_run"
	OperationFileAccess          Operation = "file_access"
	OperationFileValidation      Operation = "file_validation" // For file integrity validation
	OperationHealthCheck         Operation = "health_check"
)

// ElevationContext contains context information for privilege elevation
type ElevationContext struct {
	Operation   Operation
	CommandName string
	FilePath    string
	OriginalUID int
	TargetUID   int
	// User/group privilege change fields
	RunAsUser  string
	RunAsGroup string
}

// Standard privilege errors
var (
	ErrPrivilegedExecutionNotAvailable  = fmt.Errorf("privileged execution not available: binary lacks required SUID bit or running as non-root user")
	ErrInvalidRiskLevel                 = errors.New("invalid risk level")
	ErrPrivilegeEscalationBlocked       = errors.New("privilege escalation command blocked for security")
	ErrCriticalRiskBlocked              = errors.New("critical risk command execution blocked")
	ErrCommandSecurityViolation         = errors.New("command security violation: risk level too high")
	ErrInvalidEnvironmentVariableFormat = errors.New("invalid environment variable format")
	ErrDuplicateEnvironmentVariable     = errors.New("duplicate environment variable")
	ErrInvalidInheritanceMode           = errors.New("invalid inheritance mode")
)

// PrivilegeManager interface defines methods for privilege management
type PrivilegeManager interface {
	IsPrivilegedExecutionSupported() bool
	WithPrivileges(elevationCtx ElevationContext, fn func() error) error

	// Enhanced privilege management for user/group specification
	WithUserGroup(user, group string, fn func() error) error
	IsUserGroupSupported() bool
}
