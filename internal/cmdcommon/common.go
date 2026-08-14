// Package cmdcommon provides common functionality for command-line tools.
package cmdcommon

import (
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/security"
)

// Build-time variables (set via ldflags)
var (
	DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes" // fallback default
)

// NewDirectoryPermChecker builds the directory permission checker the CLI
// commands share. It delegates to security.NewDirectoryPermChecker and never
// panics; callers that need to substitute the checker (for example to exercise
// its failure path in a test) do so at their own injection point rather than
// through this helper.
func NewDirectoryPermChecker() (security.DirectoryPermChecker, error) {
	return security.NewDirectoryPermChecker()
}

// CreateValidator creates a new file validator with the hybrid hash path getter.
func CreateValidator(hashDir string) (filevalidator.FileValidator, error) {
	return filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
}

// CreateReadOnlyValidator creates a validator that never creates the hash
// directory. A missing or inaccessible directory is reported through
// *filevalidator.Validator.HashDirError rather than failing construction.
func CreateReadOnlyValidator(hashDir string) (*filevalidator.Validator, error) {
	return filevalidator.NewReadOnly(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
}
