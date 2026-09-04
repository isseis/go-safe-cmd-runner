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

// WithKillGraceDelay replaces the default 5-second killGraceDelay for testing,
// so tests exercising the timeout paths (ErrChildNotReaped, the output pump's
// grandchild-holding-the-pipe wait) do not each cost 5 real seconds.
func WithKillGraceDelay(d time.Duration) Option {
	return func(e *DefaultExecutor) {
		e.killGraceDelay = d
	}
}

// WithWaitFn replaces (*exec.Cmd).Wait for testing. ErrChildNotReaped can only
// occur when Wait does not return, and a SIGKILL'd child is always reaped
// promptly by the real kernel/runtime, so this injection point is the only
// way to exercise that path deterministically (a grandchild holding the pipe
// open does not block Wait -- see killGraceDelay's doc comment).
func WithWaitFn(fn func(*exec.Cmd) error) Option {
	return func(e *DefaultExecutor) {
		e.waitFn = fn
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
