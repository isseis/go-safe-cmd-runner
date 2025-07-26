// Package config provides configuration validation for privileged commands.
package config

import (
	"fmt"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// dangerousCommands is a set of potentially dangerous commands when run with privileges
var dangerousCommands = map[string]struct{}{
	// Shell executables
	"/bin/sh":       {},
	"/bin/bash":     {},
	"/usr/bin/sh":   {},
	"/usr/bin/bash": {},
	"/bin/zsh":      {},
	"/usr/bin/zsh":  {},
	"/bin/csh":      {},
	"/usr/bin/csh":  {},

	// Privilege escalation tools
	"/bin/su":       {},
	"/usr/bin/su":   {},
	"/usr/bin/sudo": {},
	"/usr/bin/doas": {},

	// System administration tools that require careful use
	"/sbin/init":      {},
	"/usr/sbin/init":  {},
	"/bin/rm":         {}, // without argument validation
	"/usr/bin/rm":     {},
	"/bin/dd":         {}, // can be destructive
	"/usr/bin/dd":     {},
	"/bin/mount":      {},
	"/usr/bin/mount":  {},
	"/bin/umount":     {},
	"/usr/bin/umount": {},

	// Package management
	"/usr/bin/apt":     {},
	"/usr/bin/apt-get": {},
	"/usr/bin/yum":     {},
	"/usr/bin/dnf":     {},
	"/usr/bin/rpm":     {},

	// Service management
	"/bin/systemctl":     {},
	"/usr/bin/systemctl": {},
	"/sbin/service":      {},
	"/usr/sbin/service":  {},
}

// shellCommands is a set of shell commands
var shellCommands = map[string]struct{}{
	"/bin/sh":       {},
	"/bin/bash":     {},
	"/usr/bin/sh":   {},
	"/usr/bin/bash": {},
	"/bin/zsh":      {},
	"/usr/bin/zsh":  {},
	"/bin/csh":      {},
	"/usr/bin/csh":  {},
	"/bin/fish":     {},
	"/usr/bin/fish": {},
	"/bin/dash":     {},
	"/usr/bin/dash": {},
}

// shellMetacharacters contains shell metacharacters that require careful handling
var shellMetacharacters = []string{
	";", "&", "|", "&&", "||",
	"$", "`", "$(", "${",
	">", "<", ">>", "<<",
	"*", "?", "[", "]",
	"~", "!",
}

// ValidatePrivilegedCommands validates configuration for potential security issues with privileged commands
func ValidatePrivilegedCommands(cfg *runnertypes.Config) []ValidationWarning {
	var warnings []ValidationWarning

	for _, group := range cfg.Groups {
		for _, cmd := range group.Commands {
			if cmd.Privileged {
				// Check for potentially dangerous commands
				if isDangerousCommand(cmd.Cmd) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s]", group.Name, cmd.Name),
						Message:    fmt.Sprintf("Privileged command uses potentially dangerous path: %s", cmd.Cmd),
						Suggestion: "Consider using a safer alternative or additional validation",
					})
				}

				// Check for shell commands
				if isShellCommand(cmd.Cmd) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s]", group.Name, cmd.Name),
						Message:    "Privileged shell commands require extra caution",
						Suggestion: "Avoid using shell commands with privileges or implement strict argument validation",
					})
				}

				// Check for commands with shell metacharacters in arguments
				if hasShellMetacharacters(cmd.Args) {
					warnings = append(warnings, ValidationWarning{
						Type:       "security",
						Location:   fmt.Sprintf("groups[%s].commands[%s].args", group.Name, cmd.Name),
						Message:    "Command arguments contain shell metacharacters - ensure proper escaping",
						Suggestion: "Use absolute paths and avoid shell metacharacters in arguments",
					})
				}

				// Check for relative paths
				if isRelativePath(cmd.Cmd) {
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

// Global security validator instance for backward compatibility
var globalValidator *security.Validator

// initGlobalValidator initializes the global validator if needed
func initGlobalValidator() {
	if globalValidator == nil {
		var err error
		globalValidator, err = security.NewValidator(nil) // Use default config
		if err != nil {
			// Fallback to legacy implementation if validator creation fails
			return
		}
	}
}

// isDangerousCommand checks if a command path is potentially dangerous when run with privileges
func isDangerousCommand(cmdPath string) bool {
	initGlobalValidator()
	if globalValidator != nil {
		return globalValidator.IsDangerousCommand(cmdPath)
	}
	// Fallback to legacy implementation
	_, exists := dangerousCommands[cmdPath]
	return exists
}

// isShellCommand checks if a command is a shell command
func isShellCommand(cmdPath string) bool {
	initGlobalValidator()
	if globalValidator != nil {
		return globalValidator.IsShellCommand(cmdPath)
	}
	// Fallback to legacy implementation
	_, exists := shellCommands[cmdPath]
	return exists
}

// hasShellMetacharacters checks if any argument contains shell metacharacters
func hasShellMetacharacters(args []string) bool {
	initGlobalValidator()
	if globalValidator != nil {
		return globalValidator.HasShellMetacharacters(args)
	}
	// Fallback to legacy implementation
	for _, arg := range args {
		for _, meta := range shellMetacharacters {
			if strings.Contains(arg, meta) {
				return true
			}
		}
	}
	return false
}

// isRelativePath checks if a path is relative
func isRelativePath(path string) bool {
	initGlobalValidator()
	if globalValidator != nil {
		return globalValidator.IsRelativePath(path)
	}
	// Fallback to legacy implementation
	return !strings.HasPrefix(path, "/")
}
