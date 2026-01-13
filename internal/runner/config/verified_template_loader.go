package config

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// VerifiedTemplateFileLoader loads and verifies template files.
type VerifiedTemplateFileLoader struct {
	manager *verification.Manager
}

// NewVerifiedTemplateFileLoader creates a new VerifiedTemplateFileLoader.
// Panics if manager is nil.
func NewVerifiedTemplateFileLoader(manager *verification.Manager) *VerifiedTemplateFileLoader {
	if manager == nil {
		panic("verification.Manager cannot be nil")
	}
	return &VerifiedTemplateFileLoader{
		manager: manager,
	}
}

// LoadTemplateFile verifies and loads a template file from the given path.
func (l *VerifiedTemplateFileLoader) LoadTemplateFile(path string) (map[string]runnertypes.CommandTemplate, error) {
	// Step 1: Read and verify file content using verification manager
	content, err := l.manager.VerifyAndReadTemplateFile(path)
	if err != nil {
		return nil, err
	}

	// Step 2: Parse content using shared parser
	return ParseTemplateContent(content, path)
}
