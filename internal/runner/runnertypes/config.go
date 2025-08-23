// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import (
	"errors"
	"fmt"
	"slices"
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
}

// Command represents a single command to be executed
type Command struct {
	Name         string   `toml:"name"`
	Description  string   `toml:"description"`
	Cmd          string   `toml:"cmd"`
	Args         []string `toml:"args"`
	Env          []string `toml:"env"`
	Dir          string   `toml:"dir"`
	Privileged   bool     `toml:"privileged"`
	Timeout      int      `toml:"timeout"`        // Command-specific timeout (overrides global)
	RunAsUser    string   `toml:"run_as_user"`    // User to execute command as (using seteuid)
	RunAsGroup   string   `toml:"run_as_group"`   // Group to execute command as (using setegid)
	MaxRiskLevel string   `toml:"max_risk_level"` // Maximum allowed risk level (low, medium, high)
}

// GetMaxRiskLevel returns the parsed maximum risk level for this command
func (c *Command) GetMaxRiskLevel() (RiskLevel, error) {
	return ParseRiskLevel(c.MaxRiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified
func (c *Command) HasUserGroupSpecification() bool {
	return c.RunAsUser != "" || c.RunAsGroup != ""
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
	// RiskLevelLow indicates commands with minimal security risk
	RiskLevelLow RiskLevel = iota

	// RiskLevelMedium indicates commands with moderate security risk
	RiskLevelMedium

	// RiskLevelHigh indicates commands with high security risk
	RiskLevelHigh
)

// String returns a string representation of RiskLevel
func (r RiskLevel) String() string {
	switch r {
	case RiskLevelLow:
		return "low"
	case RiskLevelMedium:
		return "medium"
	case RiskLevelHigh:
		return "high"
	default:
		return "unknown"
	}
}

// ParseRiskLevel converts a string to RiskLevel
func ParseRiskLevel(s string) (RiskLevel, error) {
	switch s {
	case "low":
		return RiskLevelLow, nil
	case "medium":
		return RiskLevelMedium, nil
	case "high":
		return RiskLevelHigh, nil
	case "":
		return RiskLevelLow, nil // Default to low risk
	default:
		return RiskLevelLow, fmt.Errorf("%w: %s", ErrInvalidRiskLevel, s)
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
}

// Standard privilege errors
var (
	ErrPrivilegedExecutionNotAvailable = fmt.Errorf("privileged execution not available: binary lacks required SUID bit or running as non-root user")
	ErrInvalidRiskLevel                = errors.New("invalid risk level")
)

// PrivilegeManager interface defines methods for privilege management
type PrivilegeManager interface {
	IsPrivilegedExecutionSupported() bool
	WithPrivileges(elevationCtx ElevationContext, fn func() error) error

	// Enhanced privilege management for user/group specification
	WithUserGroup(user, group string, fn func() error) error
	IsUserGroupSupported() bool
}
