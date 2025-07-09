// Package runnertypes defines the core data structures used throughout the command runner.
// It includes types for configuration, commands, and other domain-specific structures.
package runnertypes

import "github.com/isseis/go-safe-cmd-runner/internal/verification"

// Config represents the root configuration structure
type Config struct {
	Version      string                    `toml:"version"`
	Global       GlobalConfig              `toml:"global"`
	Verification verification.Config       `toml:"verification"`
	Templates    map[string]TemplateConfig `toml:"templates"`
	Groups       []CommandGroup            `toml:"groups"`
}

// GlobalConfig contains global configuration options
type GlobalConfig struct {
	Timeout  int    `toml:"timeout"`   // Global timeout in seconds
	WorkDir  string `toml:"workdir"`   // Working directory
	LogLevel string `toml:"log_level"` // Log level (debug, info, warn, error)
}

// TemplateConfig represents a template configuration
type TemplateConfig struct {
	Description string            `toml:"description"`
	Verify      []string          `toml:"verify"`     // Verification rule names
	TempDir     bool              `toml:"temp_dir"`   // Auto-generate temporary directory
	Cleanup     bool              `toml:"cleanup"`    // Auto cleanup
	WorkDir     string            `toml:"workdir"`    // Working directory (supports "auto")
	Env         []string          `toml:"env"`        // Default environment variables
	Privileged  bool              `toml:"privileged"` // Default privileged execution
	Variables   map[string]string `toml:"variables"`  // Template variables
}

// CommandGroup represents a group of related commands with a name
type CommandGroup struct {
	Name        string    `toml:"name"`
	Description string    `toml:"description"`
	Priority    int       `toml:"priority"`
	DependsOn   []string  `toml:"depends_on"`
	Template    string    `toml:"template"` // Template to apply to this group
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
	Privileged  bool     `toml:"privileged"`
	Timeout     int      `toml:"timeout"` // Command-specific timeout (overrides global)
}
