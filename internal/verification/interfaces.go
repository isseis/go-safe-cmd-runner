package verification

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ManagerInterface defines the interface for verification management
// This interface is introduced for testing purposes
type ManagerInterface interface {
	ResolvePath(path string) (string, error)
	VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error)
}

// Ensure Manager implements ManagerInterface
var _ ManagerInterface = (*Manager)(nil)
