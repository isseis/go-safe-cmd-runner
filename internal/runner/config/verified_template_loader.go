package config

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// NewVerifiedTemplateFileLoader creates a loader function that verifies files before loading.
// Panics if manager is nil.
func NewVerifiedTemplateFileLoader(manager *verification.Manager) TemplateFileLoaderFunc {
	if manager == nil {
		panic("verification.Manager cannot be nil")
	}

	return func(path string) (map[string]runnertypes.CommandTemplate, error) {
		// Step 1: Read and verify file content using verification manager
		content, err := manager.VerifyAndReadTemplateFile(path)
		if err != nil {
			return nil, err
		}

		// Step 2: Parse content using shared parser
		return ParseTemplateContent(content, path)
	}
}
