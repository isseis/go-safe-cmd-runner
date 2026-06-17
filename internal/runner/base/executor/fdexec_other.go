//go:build !linux

package executor

import (
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
)

// fdExecSupported reports false on non-Linux platforms: /proc/self/fd-based
// execution is unavailable, so the executor uses the read-only staging fallback,
// which copies the verified inode from the held descriptor before exec.
func fdExecSupported() bool { return false }

// fdExecExtraFile is unsupported on non-Linux platforms; callers fall back to
// staging.
func fdExecExtraFile(_ *risktypes.VerifiedIdentity) (string, *os.File, error) {
	return "", nil, ErrFdExecUnsupported
}
