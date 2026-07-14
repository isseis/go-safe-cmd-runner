//go:build test
// +build test

package executor

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
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

// WithFdExecDisabled forces the read-only staging fallback even on platforms
// where fd-bound execution is available, so the staging path can be exercised in
// tests on Linux.
func WithFdExecDisabled() Option {
	return func(e *DefaultExecutor) {
		e.fdExecDisabled = true
	}
}

// WithRunAsResolver replaces the default run-as identity resolver for testing.
// The resolver is called during executeWithUserGroup to resolve user/group names
// to a RunAsIdent before building the SysProcAttr.Credential.
func WithRunAsResolver(resolver func(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error)) Option {
	return func(e *DefaultExecutor) {
		e.runAsResolver = resolver
	}
}
