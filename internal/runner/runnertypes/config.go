// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import (
	"errors"
	"fmt"
	"slices"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Config represents the root configuration structure
type Config struct {
	Version string         `toml:"version"`
	Global  GlobalConfig   `toml:"global"`
	Groups  []CommandGroup `toml:"groups"`
}

// GlobalConfig contains global configuration options
type GlobalConfig struct {
	Timeout           int      `toml:"timeout"`             // Global timeout in seconds
	WorkDir           string   `toml:"workdir"`             // Working directory
	LogLevel          string   `toml:"log_level"`           // Log level (debug, info, warn, error)
	VerifyFiles       []string `toml:"verify_files"`        // Files to verify at global level
	SkipStandardPaths bool     `toml:"skip_standard_paths"` // Skip verification for standard system paths
	EnvAllowlist      []string `toml:"env_allowlist"`       // Global environment variable allowlist
	MaxOutputSize     int64    `toml:"max_output_size"`     // Default output size limit in bytes
	Env               []string `toml:"env"`                 // Global environment variables (KEY=VALUE format)

	// ExpandedVerifyFiles contains the verify_files paths with environment variable substitutions applied.
	// It is the expanded version of the VerifyFiles field, populated during configuration loading
	// and used during file verification to avoid re-expanding for each verification.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedVerifyFiles []string `toml:"-"`

	// ExpandedEnv contains the global environment variables with all variable substitutions applied.
	// It is the expanded version of the Env field, populated during configuration loading
	// and used during command execution to avoid re-expanding Global.Env for each command.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedEnv map[string]string `toml:"-"`
}

// CommandGroup represents a group of related commands with a name
type CommandGroup struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Priority    int    `toml:"priority"`

	// Fields for resource management
	TempDir bool   `toml:"temp_dir"` // Auto-generate temporary directory
	WorkDir string `toml:"workdir"`  // Working directory

	Commands     []Command `toml:"commands"`
	VerifyFiles  []string  `toml:"verify_files"`  // Files to verify for this group
	EnvAllowlist []string  `toml:"env_allowlist"` // Group-level environment variable allowlist
	Env          []string  `toml:"env"`           // Group-level environment variables (KEY=VALUE format)

	// ExpandedVerifyFiles contains the verify_files paths with environment variable substitutions applied.
	// It is the expanded version of the VerifyFiles field, populated during configuration loading
	// and used during file verification to avoid re-expanding for each verification.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedVerifyFiles []string `toml:"-"`

	// ExpandedEnv contains the group environment variables with all variable substitutions applied.
	// It is the expanded version of the Env field, populated during configuration loading
	// and used during command execution to avoid re-expanding Group.Env for each command.
	// The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedEnv map[string]string `toml:"-"`
}

// Command represents a single command to be executed
type Command struct {
	Name         string   `toml:"name"`
	Description  string   `toml:"description"`
	Cmd          string   `toml:"cmd"`
	Args         []string `toml:"args"`
	Env          []string `toml:"env"`
	Dir          string   `toml:"dir"`
	Timeout      int      `toml:"timeout"`        // Command-specific timeout (overrides global)
	RunAsUser    string   `toml:"run_as_user"`    // User to execute command as (using seteuid)
	RunAsGroup   string   `toml:"run_as_group"`   // Group to execute command as (using setegid)
	MaxRiskLevel string   `toml:"max_risk_level"` // Maximum allowed risk level (low, medium, high)
	Output       string   `toml:"output"`         // Standard output file path for capture

	// ExpandedCmd contains the command path with environment variable substitutions applied.
	// It is the expanded version of the Cmd field, populated during configuration loading
	// (Phase 1) and used during command execution (Phase 2) to avoid re-expanding Command.Cmd
	// for each execution. The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedCmd string `toml:"-"`

	// ExpandedArgs contains the command arguments with environment variable substitutions applied.
	// It is the expanded version of the Args field, populated during configuration loading
	// (Phase 1) and used during command execution (Phase 2) to avoid re-expanding Command.Args
	// for each execution. The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedArgs []string `toml:"-"`

	// ExpandedEnv contains the environment variables with all variable substitutions applied.
	// It is the expanded version of the Env field, populated during configuration loading
	// (Phase 1) and used during command execution (Phase 2) to avoid re-expanding Command.Env
	// for each execution. The toml:"-" tag prevents this field from being set via TOML configuration.
	ExpandedEnv map[string]string `toml:"-"`
}

// GetMaxRiskLevel returns the parsed maximum risk level for this command
func (c *Command) GetMaxRiskLevel() (RiskLevel, error) {
	return ParseRiskLevel(c.MaxRiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified
func (c *Command) HasUserGroupSpecification() bool {
	return c.RunAsUser != "" || c.RunAsGroup != ""
}

// BuildEnvironmentMap builds a map of environment variables from the command's Env slice.
// This is used for variable expansion processing.
func (c *Command) BuildEnvironmentMap() (map[string]string, error) {
	env := make(map[string]string)

	for _, envVar := range c.Env {
		key, value, ok := common.ParseEnvVariable(envVar)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrInvalidEnvironmentVariableFormat, envVar)
		}
		if _, exists := env[key]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateEnvironmentVariable, key)
		}
		env[key] = value
	}

	return env, nil
}

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

// AllowlistResolution contains resolved allowlist information for debugging and logging
type AllowlistResolution struct {
	Mode            InheritanceMode
	GroupAllowlist  []string
	GlobalAllowlist []string
	EffectiveList   []string // The actual allowlist being used
	GroupName       string   // For logging context
}

// IsAllowed checks if a variable is allowed based on the resolved allowlist
func (r *AllowlistResolution) IsAllowed(variable string) bool {
	switch r.Mode {
	case InheritanceModeReject:
		return false
	case InheritanceModeExplicit:
		return slices.Contains(r.GroupAllowlist, variable)
	case InheritanceModeInherit:
		return slices.Contains(r.GlobalAllowlist, variable)
	default:
		return false
	}
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
)

// PrivilegeManager interface defines methods for privilege management
type PrivilegeManager interface {
	IsPrivilegedExecutionSupported() bool
	WithPrivileges(elevationCtx ElevationContext, fn func() error) error

	// Enhanced privilege management for user/group specification
	WithUserGroup(user, group string, fn func() error) error
	IsUserGroupSupported() bool
}
