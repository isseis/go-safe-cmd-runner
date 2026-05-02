package bootstrap

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// NewVerificationManager creates a production verification manager.
func NewVerificationManager() (*verification.Manager, error) {
	secConfig := security.DefaultConfig()
	secValidator, err := security.NewValidator(secConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize security validator: %w", err)
	}

	return verification.NewManagerForProduction(secValidator)
}
