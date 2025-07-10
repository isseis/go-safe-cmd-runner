// Package verification provides file integrity verification functionality.
package verification

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Config represents the configuration for file verification
type Config struct {
	// Enabled determines if verification is active
	Enabled bool `toml:"enabled" json:"enabled"`

	// HashDirectory is the directory containing hash files
	HashDirectory string `toml:"hash_directory" json:"hash_directory"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("%w", ErrConfigNil)
	}

	if c.Enabled {
		if strings.TrimSpace(c.HashDirectory) == "" {
			return fmt.Errorf("%w", ErrHashDirectoryEmpty)
		}

		// Clean and validate the path
		c.HashDirectory = filepath.Clean(c.HashDirectory)
		if !filepath.IsAbs(c.HashDirectory) {
			return fmt.Errorf("%w: hash directory must be absolute path", ErrHashDirectoryInvalid)
		}
	}

	return nil
}

// IsEnabled returns true if verification is enabled
func (c *Config) IsEnabled() bool {
	return c != nil && c.Enabled
}
