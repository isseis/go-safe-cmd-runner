package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

// NewValidatorForTOCTOU creates a Validator configured for TOCTOU directory
// permission checks.  It wires in real group membership support so that
// group-writable directories whose group has only one member are not
// incorrectly reported as violations.
func NewValidatorForTOCTOU() (*Validator, error) {
	return NewValidator(nil, WithGroupMembership(groupmembership.New()))
}
