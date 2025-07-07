// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

// Config represents the root configuration structure
type Config struct {
	Version string                  `toml:"version"`
	Global  GlobalConfig            `toml:"global"`
	Groups  map[string]CommandGroup `toml:"groups"`
}

// GlobalConfig contains global configuration options
type GlobalConfig struct {
	Timeout  int    `toml:"timeout"`   // Global timeout in seconds
	WorkDir  string `toml:"workdir"`   // Working directory
	LogLevel string `toml:"log_level"` // Log level (debug, info, warn, error)
}

// CommandGroup represents a group of related commands
type CommandGroup struct {
	Description string    `toml:"description"`
	Priority    int       `toml:"priority"`
	DependsOn   []string  `toml:"depends_on"`
	Commands    []Command `toml:"commands"`
}

// Command represents a single command to be executed
type Command struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Cmd         string   `toml:"cmd"`
	Args        []string `toml:"args"`
	Env         []string `toml:"env"`
	Dir         string   `toml:"dir"`
	User        string   `toml:"user"`
	Privileged  bool     `toml:"privileged"`
	Timeout     int      `toml:"timeout"` // Command-specific timeout (overrides global)
}
