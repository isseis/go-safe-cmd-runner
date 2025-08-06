// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import (
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
	Cleanup bool   `toml:"cleanup"`  // Auto cleanup
	WorkDir string `toml:"workdir"`  // Working directory

	Commands     []Command `toml:"commands"`
	VerifyFiles  []string  `toml:"verify_files"`  // Files to verify for this group
	EnvAllowlist []string  `toml:"env_allowlist"` // Group-level environment variable allowlist
}

// Command represents a single command to be executed
type Command struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Cmd         string   `toml:"cmd"`
	Args        []string `toml:"args"`
	Env         []string `toml:"env"`
	Dir         string   `toml:"dir"`
	Privileged  bool     `toml:"privileged"`
	Timeout     int      `toml:"timeout"` // Command-specific timeout (overrides global)
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
)

// PrivilegeManager interface defines methods for privilege management
type PrivilegeManager interface {
	IsPrivilegedExecutionSupported() bool
	WithPrivileges(elevationCtx ElevationContext, fn func() error) error
}
