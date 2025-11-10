// Package runnertypes defines the core data structures used throughout the command runner.
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
	// Timeout specifies the default timeout for all commands
	// nil: use DefaultTimeout (60 seconds)
	// 0: unlimited execution (no timeout)
	// Positive value: timeout after N seconds
	Timeout             *int     `toml:"timeout"`               // Global timeout in seconds (nil=default 60s, 0=unlimited)
	LogLevel            LogLevel `toml:"log_level"`             // Log level: debug, info, warn, error
	VerifyStandardPaths *bool    `toml:"verify_standard_paths"` // Verify files in standard system paths (nil=default true)
	OutputSizeLimit     *int64   `toml:"output_size_limit"`     // Maximum output size in bytes (nil=use default, 0=unlimited) - raw value for TOML unmarshaling

	// Security
	VerifyFiles []string `toml:"verify_files"` // Files to verify before execution (raw paths)
	EnvAllowed  []string `toml:"env_allowed"`  // Allowed environment variables

	// Variable definitions (raw values, not yet expanded)
	EnvVars   []string `toml:"env_vars"`   // Environment variables in KEY=VALUE format
	EnvImport []string `toml:"env_import"` // System env var imports in internal_name=SYSTEM_VAR format
	Vars      []string `toml:"vars"`       // Internal variables in VAR=value format
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
	VerifyFiles []string `toml:"verify_files"` // Files to verify for this group (raw paths)
	EnvAllowed  []string `toml:"env_allowed"`  // Group-level environment variable allowlist

	// Variable definitions (raw values, not yet expanded)
	EnvVars   []string `toml:"env_vars"`   // Group-level environment variables in KEY=VALUE format
	EnvImport []string `toml:"env_import"` // System env var imports in internal_name=SYSTEM_VAR format
	Vars      []string `toml:"vars"`       // Group-level internal variables in VAR=value format
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
	WorkDir string `toml:"workdir"` // Working directory for this command (raw value)
	// Timeout specifies command-specific timeout
	// nil: inherit from parent (group or global)
	// 0: unlimited execution (no timeout)
	// Positive value: timeout after N seconds
	Timeout         *int   `toml:"timeout"`           // Command-specific timeout in seconds (nil=inherit, 0=unlimited)
	OutputSizeLimit *int64 `toml:"output_size_limit"` // Command-specific output size limit in bytes (nil=inherit from global, 0=unlimited) - raw value for TOML unmarshaling
	RunAsUser       string `toml:"run_as_user"`       // User to execute command as (using seteuid)
	RunAsGroup      string `toml:"run_as_group"`      // Group to execute command as (using setegid)
	RiskLevel       string `toml:"risk_level"`        // Maximum allowed risk level: low, medium, high
	OutputFile      string `toml:"output_file"`       // Standard output file path for capture

	// Variable definitions (raw values, not yet expanded)
	EnvVars   []string `toml:"env_vars"`   // Command-level environment variables in KEY=VALUE format
	EnvImport []string `toml:"env_import"` // System env var imports in internal_name=SYSTEM_VAR format
	Vars      []string `toml:"vars"`       // Command-level internal variables in VAR=value format
}

// GetRiskLevel parses and returns the maximum risk level for this command.
// Returns RiskLevelUnknown and an error if the risk level string is invalid.
//
// Critical risk level cannot be set in configuration (reserved for internal use only).
func (s *CommandSpec) GetRiskLevel() (RiskLevel, error) {
	return ParseRiskLevel(s.RiskLevel)
}

// HasUserGroupSpecification returns true if either run_as_user or run_as_group is specified.
func (s *CommandSpec) HasUserGroupSpecification() bool {
	return s.RunAsUser != "" || s.RunAsGroup != ""
}
