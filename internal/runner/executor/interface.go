package executor

import (
	"context"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Stream names for command output
const (
	// StdoutStream is the name of the standard output stream
	StdoutStream = "stdout"
	// StderrStream is the name of the standard error stream
	StderrStream = "stderr"
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
