// Package risk provides command risk evaluation functionality for the safe command runner.
// It analyzes commands and determines their security risk level based on various patterns and behaviors.
package risk

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Evaluator interface defines methods for evaluating command risk levels
type Evaluator interface {
	EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error)
}

// StandardEvaluator implements risk evaluation using predefined patterns
type StandardEvaluator struct{}

// NewStandardEvaluator creates a new standard risk evaluator
func NewStandardEvaluator() Evaluator {
	return &StandardEvaluator{}
}

// EvaluateRisk analyzes a command and returns its risk level
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error) {
	// Check for privilege escalation commands (critical risk - should be blocked)
	isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmd.Cmd)
	if err != nil {
		return runnertypes.RiskLevelUnknown, err
	}
	if isPrivEsc {
		return runnertypes.RiskLevelCritical, nil
	}

	// Check for destructive file operations
	if isDestructiveFileOperation(cmd.Cmd, cmd.Args) {
		return runnertypes.RiskLevelHigh, nil
	}

	// Check for network operations
	isNetwork, isHighRisk := security.IsNetworkOperation(cmd.Cmd, cmd.Args)
	if isHighRisk {
		return runnertypes.RiskLevelHigh, nil
	}
	if isNetwork {
		return runnertypes.RiskLevelMedium, nil
	}

	// Check for system modification commands
	if isSystemModification(cmd.Cmd, cmd.Args) {
		return runnertypes.RiskLevelMedium, nil
	}

	// Default to low risk for safe commands
	return runnertypes.RiskLevelLow, nil
}

// isDestructiveFileOperation checks if the command performs destructive file operations
func isDestructiveFileOperation(cmd string, args []string) bool {
	destructiveCommands := map[string]bool{
		"rm":     true,
		"rmdir":  true,
		"unlink": true,
		"shred":  true,
		"dd":     true, // Can be dangerous when used incorrectly
	}

	if destructiveCommands[cmd] {
		return true
	}

	// Check for destructive flags in common commands
	if cmd == "find" {
		for i, arg := range args {
			if arg == "-delete" {
				return true
			}
			if arg == "-exec" && i+1 < len(args) {
				// Check if the command following -exec is destructive
				execCmd := args[i+1]
				if destructiveCommands[execCmd] {
					return true
				}
			}
		}
	}

	if cmd == "rsync" {
		for _, arg := range args {
			if arg == "--delete" || arg == "--delete-before" || arg == "--delete-after" {
				return true
			}
		}
	}

	return false
}

// isSystemModification checks if the command modifies system settings
func isSystemModification(cmd string, args []string) bool {
	systemCommands := map[string]bool{
		"systemctl":   true,
		"service":     true,
		"chkconfig":   true,
		"update-rc.d": true,
		"mount":       true,
		"umount":      true,
		"fdisk":       true,
		"parted":      true,
		"mkfs":        true,
		"fsck":        true,
		"crontab":     true,
		"at":          true,
		"batch":       true,
	}

	if systemCommands[cmd] {
		return true
	}

	// Check for package management commands
	packageManagers := map[string]bool{
		"apt":     true,
		"apt-get": true,
		"yum":     true,
		"dnf":     true,
		"zypper":  true,
		"pacman":  true,
		"brew":    true,
		"pip":     true,
		"npm":     true,
		"yarn":    true,
	}

	if packageManagers[cmd] {
		// Only consider install/remove operations as medium risk
		for _, arg := range args {
			if arg == "install" || arg == "remove" || arg == "uninstall" ||
				arg == "upgrade" || arg == "update" {
				return true
			}
		}
	}

	return false
}
