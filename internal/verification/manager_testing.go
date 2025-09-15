//go:build test

package verification

import (
	"log/slog"
)

// NewManagerForTest creates a new verification manager for testing with a custom hash directory
// This API allows custom hash directories for testing purposes and uses relaxed security constraints
func NewManagerForTest(hashDir string) (*Manager, error) {
	// Log testing manager creation for audit trail
	slog.Info("Testing verification manager created",
		"api", "NewManagerForTest",
		"hash_directory", hashDir,
		"security_level", "relaxed")

	// Create manager with testing constraints (allows custom hash directory)
	return newManagerInternal(hashDir,
		withCreationMode(CreationModeTesting),
		withSecurityLevel(SecurityLevelRelaxed),
	)
}
