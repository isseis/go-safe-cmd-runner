package expansion

import (
	"context"
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
)

// VariableExpander is a simple expansion interface for cmd/args
type VariableExpander interface {
	// Expand uses existing iterative approach for simple expansion
	Expand(ctx context.Context, text string, env map[string]string, allowlist []string) (string, error)
	ExpandAll(ctx context.Context, texts []string, env map[string]string, allowlist []string) ([]string, error)
}

// VariableParser handles both $VAR and ${VAR} formats
type VariableParser interface {
	// ReplaceVariables extends existing regex to support both formats
	ReplaceVariables(text string, resolver VariableResolver) (string, error)
}

// VariableResolver provides variable resolution interface
type VariableResolver interface {
	ResolveVariable(name string) (string, error)
}

// Metrics provides minimal metrics
type Metrics struct {
	TotalExpansions int64
	VariableCount   int
	ErrorCount      int64
	MaxIterations   int // Maximum iterations for iterative approach
}

// Error definitions - reuse existing Environment Package errors
var (
	// Reuse existing ErrCircularReference
	ErrCircularReference = environment.ErrCircularReference

	// Reuse existing Security errors
	ErrVariableNotAllowed = environment.ErrVariableNotAllowed
	ErrVariableNotFound   = environment.ErrVariableNotFound
)

// ErrGenericExpansion is a generic expansion error
var ErrGenericExpansion = errors.New("expansion error")

// NewExpansionError creates a simple error factory
func NewExpansionError(message string, cause error) error {
	if cause != nil {
		return fmt.Errorf("%s: %w", message, cause)
	}
	return fmt.Errorf("%w: %s", ErrGenericExpansion, message)
}

// IsCircularReferenceError checks for circular reference using errors.Is
func IsCircularReferenceError(err error) bool {
	return errors.Is(err, ErrCircularReference)
}

// IsSecurityViolationError checks for security violations
func IsSecurityViolationError(err error) bool {
	return errors.Is(err, ErrVariableNotAllowed)
}

// IsVariableNotFoundError checks for variable not found
func IsVariableNotFoundError(err error) bool {
	return errors.Is(err, ErrVariableNotFound)
}
