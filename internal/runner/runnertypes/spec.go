// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

// ConfigSpec represents the root configuration structure loaded from TOML file.
// This is an immutable representation of the configuration file and should not be modified after loading.
//
// All fields in this struct correspond directly to TOML file structure.
// For runtime-expanded values, see RuntimeGlobal and RuntimeGroup instead.
type ConfigSpec struct {
	// Version specifies the configuration file version (e.g., "1.0")
	Version string `toml:"version"`

	// Global contains global-level configuration
	Global GlobalSpec `toml:"global"`

	// Groups contains all command groups defined in the configuration
	Groups []GroupSpec `toml:"groups"`
}

// GlobalSpec contains global configuration options loaded from TOML file.
// This is an immutable representation and should not be modified after loading.
//
// For runtime-expanded values (e.g., ExpandedEnv, ExpandedVars), see RuntimeGlobal instead.
type GlobalSpec struct {
	// Execution control
	Timeout           int    `toml:"timeout"`             // Global timeout in seconds (0 = no timeout)
	LogLevel          string `toml:"log_level"`           // Log level: debug, info, warn, error
	SkipStandardPaths bool   `toml:"skip_standard_paths"` // Skip verification for standard system paths
	MaxOutputSize     int64  `toml:"max_output_size"`     // Maximum output size in bytes (0 = unlimited)

	// Security
	VerifyFiles  []string `toml:"verify_files"`  // Files to verify before execution (raw paths)
	EnvAllowlist []string `toml:"env_allowlist"` // Allowed environment variables

	// Variable definitions (raw values, not yet expanded)
	Env     []string `toml:"env"`      // Environment variables in KEY=VALUE format
	FromEnv []string `toml:"from_env"` // System env var imports in internal_name=SYSTEM_VAR format
	Vars    []string `toml:"vars"`     // Internal variables in VAR=value format
}

// GroupSpec represents a command group configuration loaded from TOML file.
// This is an immutable representation and should not be modified after loading.
//
// For runtime-expanded values, see RuntimeGroup instead.
type GroupSpec struct {
	// Basic information
	Name        string `toml:"name"`        // Group name (must be unique within the config)
	Description string `toml:"description"` // Human-readable description
	Priority    int    `toml:"priority"`    // Execution priority (lower number = higher priority)

	// Resource management
	WorkDir string `toml:"workdir"` // Working directory for this group (raw value, not yet expanded)

	// Command definitions
	Commands []CommandSpec `toml:"commands"` // Commands in this group

	// Security
	VerifyFiles  []string `toml:"verify_files"`  // Files to verify for this group (raw paths)
	EnvAllowlist []string `toml:"env_allowlist"` // Group-level environment variable allowlist

	// Variable definitions (raw values, not yet expanded)
	Env     []string `toml:"env"`      // Group-level environment variables in KEY=VALUE format
	FromEnv []string `toml:"from_env"` // System env var imports in internal_name=SYSTEM_VAR format
	Vars    []string `toml:"vars"`     // Group-level internal variables in VAR=value format
}

// CommandSpec represents a single command configuration loaded from TOML file.
// This is an immutable representation and should not be modified after loading.
//
// For runtime-expanded values (e.g., ExpandedCmd, ExpandedArgs), see RuntimeCommand instead.
type CommandSpec struct {
	// Basic information
	Name        string `toml:"name"`        // Command name (must be unique within the group)
	Description string `toml:"description"` // Human-readable description

	// Command definition (raw values, not yet expanded)
	Cmd  string   `toml:"cmd"`  // Command path (may contain variables like %{VAR})
	Args []string `toml:"args"` // Command arguments (may contain variables)

	// Execution settings
	WorkDir      string `toml:"workdir"`        // Working directory for this command (raw value)
	Timeout      int    `toml:"timeout"`        // Command-specific timeout in seconds (overrides global/group)
	RunAsUser    string `toml:"run_as_user"`    // User to execute command as (using seteuid)
	RunAsGroup   string `toml:"run_as_group"`   // Group to execute command as (using setegid)
	MaxRiskLevel string `toml:"max_risk_level"` // Maximum allowed risk level: low, medium, high
	Output       string `toml:"output"`         // Standard output file path for capture

	// Variable definitions (raw values, not yet expanded)
	Env     []string `toml:"env"`      // Command-level environment variables in KEY=VALUE format
	FromEnv []string `toml:"from_env"` // System env var imports in internal_name=SYSTEM_VAR format
	Vars    []string `toml:"vars"`     // Command-level internal variables in VAR=value format
}

// GetMaxRiskLevel parses and returns the maximum risk level for this command.
// Returns RiskLevelUnknown and an error if the risk level string is invalid.
//
// Critical risk level cannot be set in configuration (reserved for internal use only).
func (s *CommandSpec) GetMaxRiskLevel() (RiskLevel, error) {
	return ParseRiskLevel(s.MaxRiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
func (s *CommandSpec) HasUserGroupSpecification() bool {
	return s.RunAsUser != "" || s.RunAsGroup != ""
}
