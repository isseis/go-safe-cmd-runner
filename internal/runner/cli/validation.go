// Package cli provides command-line interface functionality and validation.
package cli

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Error definitions
var (
	ErrConfigPathRequired     = errors.New("config file path is required")
	ErrInvalidDetailLevel     = errors.New("invalid detail level - valid options are: summary, detailed, full")
	ErrInvalidOutputFormat    = errors.New("invalid output format - valid options are: text, json")
	ErrConfigValidationFailed = errors.New("config validation failed")
)

// ValidateConfigCommand implements config validation CLI command
func ValidateConfigCommand(cfg *runnertypes.Config) error {
	// Validate config
	validator := config.NewConfigValidator()
	result, err := validator.ValidateConfig(cfg)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Generate and display report
	report, err := validator.GenerateValidationReport(result)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Print(report)

	// Exit with appropriate code
	if !result.Valid {
		// Return error to signal exit code 1
		return ErrConfigValidationFailed
	}

	return nil
}
