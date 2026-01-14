//go:build test

package config

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// LoadConfigForTest loads and validates configuration from byte content instead of file path.
// This function is intended for testing purposes only and does not process includes.
//
// For production code, use LoadConfigWithPath which processes includes and merges templates.
//
// Parameters:
//   - content: Raw TOML configuration content as bytes
//
// Returns:
//   - *runnertypes.ConfigSpec: Parsed and validated configuration
//   - error: Any error during parsing or validation
//
// Note: This function does not process includes. Use LoadConfigWithPath for full functionality.
func (l *Loader) LoadConfigForTest(content []byte) (*runnertypes.ConfigSpec, error) {
	return l.loadConfigInternal(content)
}
