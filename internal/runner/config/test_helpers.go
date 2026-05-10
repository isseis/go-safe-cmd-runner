//go:build test
// +build test

package config

import (
	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
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

// NewLoaderForTest creates a new config loader with default dependencies for testing.
// This convenience constructor should only be used in test code.
// It allows verificationManager to be nil for tests that don't need verification.
func NewLoaderForTest() *Loader {
	return &Loader{
		fs:              common.NewDefaultFileSystem(),
		verificationMgr: nil,
	}
}

func makeCommand(name string, timeout *int32) runnertypes.CommandSpec {
	return runnertypes.CommandSpec{
		Name:    name,
		Cmd:     "/bin/echo",
		Timeout: timeout,
	}
}

func makeGroup(name string, commands ...runnertypes.CommandSpec) runnertypes.GroupSpec {
	return runnertypes.GroupSpec{
		Name:     name,
		Commands: commands,
	}
}

func makeConfig(globalTimeout *int32, groups ...runnertypes.GroupSpec) *runnertypes.ConfigSpec {
	cfg := &runnertypes.ConfigSpec{
		Groups: groups,
	}
	if globalTimeout != nil {
		cfg.Global.Timeout = globalTimeout
	}
	return cfg
}
