package runner

import (
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// GroupExecutorOption configures a DefaultGroupExecutor during construction.
type GroupExecutorOption func(*groupExecutorOptions)

// groupExecutorOptions holds internal configuration options for DefaultGroupExecutor.
type groupExecutorOptions struct {
	notificationFunc groupNotificationFunc
	dryRunOptions    *resource.DryRunOptions
	keepTempDirs     bool
	securityLogger   *logging.SecurityLogger
	currentUser      string
}

// defaultGroupExecutorOptions returns a new groupExecutorOptions with default values.
func defaultGroupExecutorOptions() *groupExecutorOptions {
	return &groupExecutorOptions{
		notificationFunc: nil,
		dryRunOptions:    nil, // dry-run disabled
		keepTempDirs:     false,
		securityLogger:   nil,
		currentUser:      "unknown",
	}
}

// WithGroupNotificationFunc sets the notification function.
func WithGroupNotificationFunc(fn groupNotificationFunc) GroupExecutorOption {
	return func(opts *groupExecutorOptions) {
		opts.notificationFunc = fn
	}
}

// WithGroupDryRun enables dry-run mode with the specified options.
func WithGroupDryRun(options *resource.DryRunOptions) GroupExecutorOption {
	return func(opts *groupExecutorOptions) {
		opts.dryRunOptions = options
	}
}

// WithGroupKeepTempDirs controls temporary directory cleanup.
func WithGroupKeepTempDirs(keep bool) GroupExecutorOption {
	return func(opts *groupExecutorOptions) {
		opts.keepTempDirs = keep
	}
}

// WithSecurityLogger sets the security logger for timeout-related security events.
func WithSecurityLogger(logger *logging.SecurityLogger) GroupExecutorOption {
	return func(opts *groupExecutorOptions) {
		opts.securityLogger = logger
	}
}

// WithCurrentUser sets the current user name for security logging.
// This should be obtained from a trusted source (e.g., os/user.Current())
// rather than environment variables which can be spoofed.
func WithCurrentUser(username string) GroupExecutorOption {
	return func(opts *groupExecutorOptions) {
		if username != "" {
			opts.currentUser = username
		}
	}
}
