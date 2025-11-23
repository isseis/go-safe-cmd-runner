package config

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Static error definitions
var (
	// ErrNegativeTimeout indicates that a timeout value is negative
	ErrNegativeTimeout = errors.New("timeout must not be negative")
)

// ValidateTimeouts validates that all timeout values in the configuration are non-negative.
// It checks both global timeout and command-level timeouts.
// Returns an error if any timeout is negative.
func ValidateTimeouts(cfg *runnertypes.ConfigSpec) error {
	// Check global timeout
	if cfg.Global.Timeout != nil && *cfg.Global.Timeout < 0 {
		return fmt.Errorf("%w: global timeout got %d", ErrNegativeTimeout, *cfg.Global.Timeout)
	}

	// Check command-level timeouts
	for groupIdx, group := range cfg.Groups {
		for cmdIdx, cmd := range group.Commands {
			if cmd.Timeout != nil && *cmd.Timeout < 0 {
				return fmt.Errorf("%w: command '%s' in group '%s' (groups[%d].commands[%d]) got %d",
					ErrNegativeTimeout, cmd.Name, group.Name, groupIdx, cmdIdx, *cmd.Timeout)
			}
		}
	}

	return nil
}
