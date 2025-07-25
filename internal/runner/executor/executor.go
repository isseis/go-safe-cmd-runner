// Package executor provides the core functionality for executing commands
// in a safe and controlled manner. It includes interfaces and implementations
// for command execution, output handling, and environment management.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Error definitions
var (
	ErrEmptyCommand = errors.New("command cannot be empty")
	ErrDirNotExists = errors.New("directory does not exist")
	ErrInvalidPath  = errors.New("invalid command path")
)

// DefaultExecutor is the default implementation of CommandExecutor
type DefaultExecutor struct {
	FS  FileSystem
	Out OutputWriter
}

// NewDefaultExecutor creates a new default command executor
func NewDefaultExecutor() CommandExecutor {
	return &DefaultExecutor{
		FS:  &osFileSystem{},
		Out: &consoleOutputWriter{},
	}
}

// Execute implements the CommandExecutor interface
func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
	// Validate the command before execution
	if err := e.Validate(cmd); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	// Resolve the command path
	path, lookErr := exec.LookPath(cmd.Cmd)
	if lookErr != nil {
		return nil, fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
	}

	// Create the command with the resolved path
	// #nosec G204 - The command and arguments are validated before execution
	execCmd := exec.CommandContext(ctx, path, cmd.Args...)

	// Set up working directory
	if cmd.Dir != "" {
		execCmd.Dir = cmd.Dir
	}

	// Set up environment variables
	// Only use the filtered environment variables provided in envVars
	// This ensures allowlist filtering is properly enforced
	execCmd.Env = make([]string, 0, len(envVars))
	for k, v := range envVars {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up output capture
	var stdout, stderr []byte
	var cmdErr error

	if e.Out != nil {
		// Create buffered wrappers that both capture output and write to OutputWriter
		stdoutWrapper := &outputWrapper{writer: e.Out, stream: StdoutStream}
		stderrWrapper := &outputWrapper{writer: e.Out, stream: StderrStream}

		execCmd.Stdout = stdoutWrapper
		execCmd.Stderr = stderrWrapper

		// Run the command
		cmdErr = execCmd.Run()

		// Get the captured output
		stdout = stdoutWrapper.GetBuffer()
		stderr = stderrWrapper.GetBuffer()
	} else {
		// Otherwise, capture output in memory
		stdout, cmdErr = execCmd.Output()
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
		}
	}

	// Prepare the result
	result := &Result{
		Stdout: string(stdout),
		Stderr: string(stderr),
	}
	if execCmd.ProcessState != nil {
		result.ExitCode = execCmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = ExitCodeUnknown // Use constant for unknown exit code
	}

	if cmdErr != nil {
		return result, fmt.Errorf("command execution failed: %w", cmdErr)
	}

	return result, nil
}

// Validate implements the CommandExecutor interface
func (e *DefaultExecutor) Validate(cmd runnertypes.Command) error {
	// Check if command is empty
	if cmd.Cmd == "" {
		return ErrEmptyCommand
	}

	// Validate command path to prevent command injection and ensure proper format
	if !filepath.IsLocal(cmd.Cmd) && !filepath.IsAbs(cmd.Cmd) {
		return fmt.Errorf("%w: command path must be local or absolute: %s", ErrInvalidPath, cmd.Cmd)
	}
	if filepath.Clean(cmd.Cmd) != cmd.Cmd {
		return fmt.Errorf("%w: command path contains relative path components ('.' or '..'): %s", ErrInvalidPath, cmd.Cmd)
	}

	// Check if working directory exists and is accessible
	if cmd.Dir != "" {
		exists, err := e.FS.FileExists(cmd.Dir)
		if err != nil {
			return fmt.Errorf("failed to check directory %s: %w", cmd.Dir, err)
		}
		if !exists {
			return fmt.Errorf("working directory %q does not exist: %w", cmd.Dir, ErrDirNotExists)
		}
	}

	return nil
}

// osFileSystem implements FileSystem using the standard os package
type osFileSystem struct{}

func (fs *osFileSystem) CreateTempDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

func (fs *osFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *osFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// consoleOutputWriter implements OutputWriter by writing to stdout/stderr
type consoleOutputWriter struct {
	mu sync.Mutex
}

func (w *consoleOutputWriter) Write(stream string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if stream == StderrStream {
		_, err := os.Stderr.Write(data)
		return err
	}
	_, err := os.Stdout.Write(data)
	return err
}

func (w *consoleOutputWriter) Close() error {
	// Nothing to close for console output
	return nil
}

// outputWrapper is an io.Writer that both captures output in a buffer
// and writes to an OutputWriter with a specific stream name
type outputWrapper struct {
	writer OutputWriter
	stream string
	buffer bytes.Buffer
	mu     sync.Mutex
}

func (w *outputWrapper) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write to buffer for capturing
	w.buffer.Write(p)

	// Also write to the OutputWriter
	if w.writer != nil {
		if err := w.writer.Write(w.stream, p); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (w *outputWrapper) GetBuffer() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Bytes()
}
