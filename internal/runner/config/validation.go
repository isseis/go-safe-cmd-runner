// Package config provides configuration validation for privileged commands.
package config

import (
	"fmt"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// ValidatePrivilegedCommands validates configuration for potential security issues with privileged commands
// It explicitly takes a security validator as a parameter, making dependencies clear
func ValidatePrivilegedCommands(cfg *runnertypes.Config, validator *security.Validator) []ValidationWarning {
	var warnings []ValidationWarning

	for _, group := range cfg.Groups {
		for _, cmd := range group.Commands {
			if cmd.Privileged {
				// Check for potentially dangerous commands
				if validator.IsDangerousPrivilegedCommand(cmd.Cmd) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s]", group.Name, cmd.Name),
						Message:    fmt.Sprintf("Privileged command uses potentially dangerous path: %s", cmd.Cmd),
						Suggestion: "Consider using a safer alternative or additional validation",
					})
				}

				// Check for shell commands
				if validator.IsShellCommand(cmd.Cmd) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s]", group.Name, cmd.Name),
						Message:    "Privileged shell commands require extra caution",
						Suggestion: "Avoid using shell commands with privileges or implement strict argument validation",
					})
				}

				// Check for commands with shell metacharacters in arguments
				if validator.HasShellMetacharacters(cmd.Args) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s].args", group.Name, cmd.Name),
						Message:    "Command arguments contain shell metacharacters - ensure proper escaping",
						Suggestion: "Use absolute paths and avoid shell metacharacters in arguments",
					})
				}

				// Check for relative paths
				if !filepath.IsAbs(cmd.Cmd) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s].cmd", group.Name, cmd.Name),
						Message:    "Privileged command uses relative path - consider using absolute path for security",
						Suggestion: "Use absolute path to prevent PATH-based attacks",
					})
				}
			}
		}
	}

	return warnings
}
