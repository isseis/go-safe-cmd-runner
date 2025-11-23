// Package cli provides command-line interface functionality and validation.
package cli

import (
	"errors"
)

// Error definitions
var (
	ErrInvalidDetailLevel  = errors.New("invalid detail level - valid options are: summary, detailed, full")
	ErrInvalidOutputFormat = errors.New("invalid output format - valid options are: text, json")
)
