// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

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
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Priority    int      `toml:"priority"`
	DependsOn   []string `toml:"depends_on"`

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
