// Package cmdcommon provides common functionality for command-line tools.
package cmdcommon

import (
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

// Build-time variables (set via ldflags)
var (
	DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes" // fallback default
)

// CreateReadOnlyValidator creates a validator that never creates the hash
// directory. A missing or inaccessible directory is reported through
// *filevalidator.Validator.HashDirError rather than failing construction.
func CreateReadOnlyValidator(hashDir string) (*filevalidator.Validator, error) {
	return filevalidator.NewReadOnly(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
}
