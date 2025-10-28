// Package cmdcommon provides common functionality for command-line tools.
package cmdcommon

import (
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

// Build-time variables (set via ldflags)
var (
	DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes" // fallback default
)

// CreateValidator creates a new file validator with the hybrid hash path getter.
func CreateValidator(hashDir string) (filevalidator.FileValidator, error) {
	return filevalidator.New(&filevalidator.SHA256{}, hashDir)
}
