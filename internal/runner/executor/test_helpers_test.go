package executor

import (
	"log/slog"
)

// WithFileSystem sets the file system for the executor
func WithFileSystem(fs FileSystem) Option {
	return func(e *DefaultExecutor) {
		e.FS = fs
	}
}

// WithLogger sets the logger for the executor
func WithLogger(logger *slog.Logger) Option {
	return func(e *DefaultExecutor) {
		e.Logger = logger
	}
}

// WithExitFunc replaces os.Exit with a custom function for testing emergency shutdown behavior.
func WithExitFunc(fn func(int)) Option {
	return func(e *DefaultExecutor) {
		e.osExit = fn
	}
}

// WithIdentityChecker replaces the default EUID/EGID checker for testing privilege leak detection.
func WithIdentityChecker(fn func() error) Option {
	return func(e *DefaultExecutor) {
		e.identityChecker = fn
	}
}
