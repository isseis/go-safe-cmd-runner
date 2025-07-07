package executor

import (
	"context"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// CommandExecutor defines the interface for executing commands
type CommandExecutor interface {
	// Execute executes a command with the given environment variables
	Execute(ctx context.Context, cmd runnertypes.Command, env map[string]string) (*Result, error)
	// Validate validates a command without executing it
	Validate(cmd runnertypes.Command) error
}

// Result contains the result of a command execution
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// OutputWriter defines the interface for writing command output
type OutputWriter interface {
	// Write writes the output of a command
	Write(stream string, data []byte) error

	// Close closes the output writer
	Close() error
}

// FileSystem defines the interface for file system operations
type FileSystem interface {
	// CreateTempDir creates a temporary directory
	CreateTempDir(prefix string) (string, error)

	// RemoveAll removes a directory and all its contents
	RemoveAll(path string) error

	// FileExists checks if a file exists
	FileExists(path string) (bool, error)
}

// EnvironmentManager handles environment variable operations
type EnvironmentManager interface {
	// LoadFromFile loads environment variables from a file
	LoadFromFile(path string) (map[string]string, error)

	// Merge merges multiple environment variable maps
	Merge(envs ...map[string]string) map[string]string

	// Resolve resolves environment variable references in a string
	Resolve(s string, env map[string]string) (string, error)
}
