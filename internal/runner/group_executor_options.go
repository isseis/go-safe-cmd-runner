package runner

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// GroupExecutorOption configures a DefaultGroupExecutor during construction.
type GroupExecutorOption func(*groupExecutorOptions)

// groupExecutorOptions holds internal configuration options for DefaultGroupExecutor.
type groupExecutorOptions struct {
	notificationFunc groupNotificationFunc
	dryRunOptions    *resource.DryRunOptions
	keepTempDirs     bool
}

// defaultGroupExecutorOptions returns a new groupExecutorOptions with default values.
func defaultGroupExecutorOptions() *groupExecutorOptions {
	return &groupExecutorOptions{
		notificationFunc: nil,
		dryRunOptions:    nil, // dry-run disabled
		keepTempDirs:     false,
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
