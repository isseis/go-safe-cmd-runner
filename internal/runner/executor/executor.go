// Package executor provides the core functionality for executing commands
// in a safe and controlled manner. It includes interfaces and implementations
// for command execution, output handling, and environment management.
package executor

import (
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
	Env EnvironmentManager
}

// NewDefaultExecutor creates a new default command executor
func NewDefaultExecutor() CommandExecutor {
	return &DefaultExecutor{
		FS:  &osFileSystem{},
		Out: &consoleOutputWriter{},
		Env: &envManager{},
	}
}

// Execute implements the CommandExecutor interface
func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
	// Validate the command before execution
	if err := e.Validate(cmd); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	// Validate command path to prevent command injection
	if !filepath.IsLocal(cmd.Cmd) && !filepath.IsAbs(cmd.Cmd) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPath, cmd.Cmd)
	}

	// Create the command with explicit path resolution
	path, lookErr := exec.LookPath(cmd.Cmd)
	if lookErr != nil {
		return nil, fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
	}

	// Validate the resolved path is within allowed directories
	if !filepath.IsLocal(path) && !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: resolved path %q is not allowed", ErrInvalidPath, path)
	}

	// Create the command with the resolved path
	// #nosec G204 - The command and arguments are validated before execution
	execCmd := exec.CommandContext(ctx, path, cmd.Args...)

	// Set up working directory
	if cmd.Dir != "" {
		execCmd.Dir = cmd.Dir
	}

	// Set up environment variables
	execCmd.Env = os.Environ()
	for k, v := range envVars {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up output capture
	var stdout, stderr []byte
	var cmdErr error

	if e.Out != nil {
		// If we have an output writer, use it
		execCmd.Stdout = &outputWrapper{writer: e.Out, stream: "stdout"}
		execCmd.Stderr = &outputWrapper{writer: e.Out, stream: "stderr"}

		// Run the command
		cmdErr = execCmd.Run()
	} else {
		// Otherwise, capture output in memory
		stdout, cmdErr = execCmd.Output()
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
		}
	}

	// Prepare the result
	result := &Result{
		ExitCode: execCmd.ProcessState.ExitCode(),
		Stdout:   string(stdout),
		Stderr:   string(stderr),
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
		return fmt.Errorf("%w", ErrEmptyCommand)
	}

	// Check if working directory exists and is accessible
	if cmd.Dir != "" {
		exists, err := e.FS.FileExists(cmd.Dir)
		if err != nil {
			return fmt.Errorf("failed to check directory %s: %w", cmd.Dir, err)
		}
		if !exists {
			return fmt.Errorf("%s: %w", cmd.Dir, ErrDirNotExists)
		}
	}

	// TODO: Add more validation rules

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
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// consoleOutputWriter implements OutputWriter by writing to stdout/stderr
type consoleOutputWriter struct {
	mu sync.Mutex
}

func (w *consoleOutputWriter) Write(stream string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if stream == "stderr" {
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

// outputWrapper is an io.Writer that writes to an OutputWriter
// with a specific stream name
type outputWrapper struct {
	writer OutputWriter
	stream string
}

func (w *outputWrapper) Write(p []byte) (n int, err error) {
	if err := w.writer.Write(w.stream, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// envManager implements EnvironmentManager
type envManager struct{}

func (m *envManager) LoadFromFile(_ string) (map[string]string, error) {
	// TODO: Implement environment variable loading from file
	return map[string]string{}, nil
}

func (m *envManager) Merge(envs ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, env := range envs {
		for k, v := range env {
			result[k] = v
		}
	}
	return result
}

func (m *envManager) Resolve(s string, _ map[string]string) (string, error) {
	// TODO: Implement environment variable resolution
	return s, nil
}
