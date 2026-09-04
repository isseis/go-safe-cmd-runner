//go:build test

package executor

import (
	"log/slog"
	"os/exec"
	"time"

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
func WithRunAsResolver(resolver risktypes.RunAsResolver) Option {
	return func(e *DefaultExecutor) {
		e.runAsResolver = resolver
	}
}

// WithKillGraceDelay overrides the default killGraceDelay for testing, so
// tests that exercise the reap-timeout or output-drain-timeout paths do not
// have to wait out the production default.
func WithKillGraceDelay(d time.Duration) Option {
	return func(e *DefaultExecutor) {
		e.killGraceDelay = d
	}
}

// WithWaitFn replaces execCmd.Wait() in the wait goroutine for testing. See
// DefaultExecutor.waitFn for why this is the only way to exercise
// ErrChildNotReaped deterministically.
func WithWaitFn(fn func(*exec.Cmd) error) Option {
	return func(e *DefaultExecutor) {
		e.waitFn = fn
	}
}
