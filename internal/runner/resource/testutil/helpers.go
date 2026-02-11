//go:build test || performance

// Package testutil provides test helper functions for resource management testing.
package testutil

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// NewNormalResourceManager creates a new NormalResourceManager for normal execution mode
func NewNormalResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) *resource.NormalResourceManager {
	// Delegate to NewNormalResourceManagerWithOutput with nil outputManager and 0 maxOutputSize
	return resource.NewNormalResourceManagerWithOutput(exec, fs, privMgr, nil, 0, logger)
}
