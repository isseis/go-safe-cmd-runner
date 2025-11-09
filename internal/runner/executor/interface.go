package executor

import (
	"context"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Exit code constants
const (
	// ExitCodeUnknown is used when the process state is not available
	// to determine the actual exit code.
	ExitCodeUnknown = -1
)

// OutputStream represents the type of output stream (stdout or stderr)
type OutputStream int

// Output stream constants
const (
	// StdoutStream represents the standard output stream
	StdoutStream OutputStream = iota
	// StderrStream represents the standard error stream
	StderrStream
)

// String returns the string representation of the output stream
func (s OutputStream) String() string {
	switch s {
	case StdoutStream:
		return "stdout"
	case StderrStream:
		return "stderr"
	default:
		return "unknown"
	}
}

// CommandExecutor defines the interface for executing commands
type CommandExecutor interface {
	// Execute executes a command with custom output writer.
	//
	// OutputWriter lifecycle and ownership:
	// - outputWriter may be nil, in which case output is handled internally
	// - If provided, the caller owns the outputWriter and is responsible for calling Close()
	// - Execute will NOT call Close() on the outputWriter
	// - Write() calls are made during command execution as output is generated (streamed)
	// - The outputWriter must remain valid for the entire duration of command execution
	// - Multiple concurrent Write() calls may occur, so implementations must be thread-safe
	Execute(ctx context.Context, cmd *runnertypes.RuntimeCommand, env map[string]string, outputWriter OutputWriter) (*Result, error)
	// Validate validates a command without executing it
	Validate(cmd *runnertypes.RuntimeCommand) error
}

// Result contains the result of a command execution
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// OutputWriter defines the interface for writing command output.
//
// Implementations must be thread-safe as Write() may be called concurrently
// from multiple goroutines handling stdout and stderr streams.
//
// Lifecycle contract:
// - Write() calls occur during command execution as output is generated
// - Close() must be called by the owner when output writing is complete
// - Close() should be idempotent (safe to call multiple times)
// - After Close() is called, Write() should return an error
type OutputWriter interface {
	// Write writes output data from the specified stream.
	// stream will be either StdoutStream or StderrStream.
	// data contains the raw output bytes and may include partial lines.
	// Returns an error if writing fails or if the writer has been closed.
	Write(stream OutputStream, data []byte) error

	// Close closes the output writer and releases any resources.
	// Must be idempotent and thread-safe.
	// After Close() is called, subsequent Write() calls should return an error.
	Close() error
}

// FileSystem defines the interface for file system operations
type FileSystem interface {
	// CreateTempDir creates a temporary directory with the given prefix
	CreateTempDir(dir string, prefix string) (string, error)

	// RemoveAll removes a directory and all its contents
	RemoveAll(path string) error

	// FileExists checks if a file exists
	FileExists(path string) (bool, error)
}
