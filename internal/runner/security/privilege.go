package security

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// PrivilegeEscalationType represents the type of privilege escalation detected
type PrivilegeEscalationType string

const (
	// PrivilegeEscalationSudo represents privilege escalation through sudo command
	PrivilegeEscalationSudo PrivilegeEscalationType = "sudo"
	// PrivilegeEscalationSu represents privilege escalation through su command
	PrivilegeEscalationSu PrivilegeEscalationType = "su"
	// PrivilegeEscalationSystemd represents privilege escalation through systemd commands
	PrivilegeEscalationSystemd PrivilegeEscalationType = "systemd"
	// PrivilegeEscalationService represents privilege escalation through service management commands
	PrivilegeEscalationService PrivilegeEscalationType = "service"
	// PrivilegeEscalationOther represents other types of privilege escalation
	PrivilegeEscalationOther PrivilegeEscalationType = "other"
)

// PrivilegeEscalationResult contains the analysis result of privilege escalation detection
type PrivilegeEscalationResult struct {
	// IsPrivilegeEscalation indicates whether privilege escalation was detected
	IsPrivilegeEscalation bool

	// EscalationType specifies the type of privilege escalation detected
	EscalationType PrivilegeEscalationType

	// RiskLevel indicates the security risk level
	RiskLevel runnertypes.RiskLevel

	// RequiredPrivileges lists the specific privileges required
	RequiredPrivileges []string

	// CommandPath is the resolved absolute path of the command
	CommandPath string

	// DetectedPattern describes the pattern that triggered the detection
	DetectedPattern string

	// Reason provides a human-readable explanation of why this is considered privilege escalation
	Reason string
}

// PrivilegeEscalationAnalyzer interface defines methods for analyzing privilege escalation
type PrivilegeEscalationAnalyzer interface {
	// AnalyzePrivilegeEscalation analyzes a command and its arguments for privilege escalation patterns
	AnalyzePrivilegeEscalation(ctx context.Context, cmdName string, args []string) (*PrivilegeEscalationResult, error)

	// IsPrivilegeEscalationCommand checks if a command is a known privilege escalation command
	IsPrivilegeEscalationCommand(cmdName string) bool

	// GetRequiredPrivileges returns the privileges required by a command
	GetRequiredPrivileges(cmdName string, args []string) ([]string, error)
}

// DefaultPrivilegeEscalationAnalyzer implements PrivilegeEscalationAnalyzer
type DefaultPrivilegeEscalationAnalyzer struct {
	logger          *slog.Logger
	sudoCommands    map[string]bool
	systemCommands  map[string]bool
	serviceCommands map[string]bool
}

// NewDefaultPrivilegeEscalationAnalyzer creates a new instance of DefaultPrivilegeEscalationAnalyzer
func NewDefaultPrivilegeEscalationAnalyzer(logger *slog.Logger) *DefaultPrivilegeEscalationAnalyzer {
	return &DefaultPrivilegeEscalationAnalyzer{
		logger: logger,
		sudoCommands: map[string]bool{
			"sudo":          true,
			"/usr/bin/sudo": true,
			"/bin/sudo":     true,
			"su":            true,
			"/bin/su":       true,
			"/usr/bin/su":   true,
			"doas":          true,
			"/usr/bin/doas": true,
		},
		systemCommands: map[string]bool{
			"systemctl":          true,
			"/bin/systemctl":     true,
			"/usr/bin/systemctl": true,
		},
		serviceCommands: map[string]bool{
			"service":           true,
			"/sbin/service":     true,
			"/usr/sbin/service": true,
		},
	}
}

// AnalyzePrivilegeEscalation analyzes a command for privilege escalation patterns
func (a *DefaultPrivilegeEscalationAnalyzer) AnalyzePrivilegeEscalation(_ context.Context, cmdName string, _ []string) (*PrivilegeEscalationResult, error) {
	// Resolve command path
	resolvedPath, err := a.resolveCommandPath(cmdName)
	if err != nil {
		a.logger.Debug("Failed to resolve command path", "command", cmdName, "error", err)
		resolvedPath = cmdName // Use original if resolution fails
	}

	result := &PrivilegeEscalationResult{
		CommandPath: resolvedPath,
		RiskLevel:   runnertypes.RiskLevelNone,
	}

	// Check for sudo commands
	if a.isSudoCommand(resolvedPath) {
		result.IsPrivilegeEscalation = true
		result.EscalationType = PrivilegeEscalationSudo
		result.RiskLevel = runnertypes.RiskLevelHigh
		result.DetectedPattern = "sudo_command"
		result.Reason = "Command uses sudo for privilege escalation"
		result.RequiredPrivileges = []string{"root"}

		a.logger.Info("Privilege escalation detected",
			"command", cmdName,
			"type", result.EscalationType,
			"risk_level", result.RiskLevel)
		return result, nil
	}

	// Check for systemctl commands
	if a.isSystemCommand(resolvedPath) {
		result.IsPrivilegeEscalation = true
		result.EscalationType = PrivilegeEscalationSystemd
		result.RiskLevel = runnertypes.RiskLevelMedium
		result.DetectedPattern = "systemctl_command"
		result.Reason = "Command manages system services"
		result.RequiredPrivileges = []string{"systemd"}

		a.logger.Info("Privilege escalation detected",
			"command", cmdName,
			"type", result.EscalationType,
			"risk_level", result.RiskLevel)
		return result, nil
	}

	// Check for service commands
	if a.isServiceCommand(resolvedPath) {
		result.IsPrivilegeEscalation = true
		result.EscalationType = PrivilegeEscalationService
		result.RiskLevel = runnertypes.RiskLevelMedium
		result.DetectedPattern = "service_command"
		result.Reason = "Command manages system services"
		result.RequiredPrivileges = []string{"service"}

		a.logger.Info("Privilege escalation detected",
			"command", cmdName,
			"type", result.EscalationType,
			"risk_level", result.RiskLevel)
		return result, nil
	}

	// If no privilege escalation detected, log debug info
	a.logger.Debug("No privilege escalation detected", "command", cmdName, "resolved_path", resolvedPath)
	return result, nil
}

// IsPrivilegeEscalationCommand checks if a command is a known privilege escalation command
func (a *DefaultPrivilegeEscalationAnalyzer) IsPrivilegeEscalationCommand(cmdName string) bool {
	resolvedPath, err := a.resolveCommandPath(cmdName)
	if err != nil {
		resolvedPath = cmdName
	}

	return a.isSudoCommand(resolvedPath) ||
		a.isSystemCommand(resolvedPath) ||
		a.isServiceCommand(resolvedPath)
}

// GetRequiredPrivileges returns the privileges required by a command
func (a *DefaultPrivilegeEscalationAnalyzer) GetRequiredPrivileges(cmdName string, args []string) ([]string, error) {
	result, err := a.AnalyzePrivilegeEscalation(context.Background(), cmdName, args)
	if err != nil {
		return nil, err
	}

	if result.IsPrivilegeEscalation {
		return result.RequiredPrivileges, nil
	}

	return []string{}, nil
}

// resolveCommandPath resolves the command path, handling relative paths and symlinks
func (a *DefaultPrivilegeEscalationAnalyzer) resolveCommandPath(cmdName string) (string, error) {
	// If already absolute path, clean and return
	if filepath.IsAbs(cmdName) {
		resolved, err := filepath.EvalSymlinks(cmdName)
		if err != nil {
			return filepath.Clean(cmdName), nil // Return cleaned original if symlink resolution fails
		}
		return resolved, nil
	}

	// For relative paths, try to find in PATH
	absPath, err := filepath.Abs(cmdName)
	if err != nil {
		return cmdName, err
	}

	// Check if file exists at absolute path
	if _, err := os.Stat(absPath); err == nil {
		resolved, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			return absPath, nil
		}
		return resolved, nil
	}

	// Return original command name if not found
	return cmdName, nil
}

// isSudoCommand checks if the command is a sudo-related command
func (a *DefaultPrivilegeEscalationAnalyzer) isSudoCommand(cmdPath string) bool {
	// Check exact matches
	if a.sudoCommands[cmdPath] {
		return true
	}

	// Check basename for common sudo commands
	baseName := filepath.Base(cmdPath)
	return a.sudoCommands[baseName]
}

// isSystemCommand checks if the command is a systemctl command
func (a *DefaultPrivilegeEscalationAnalyzer) isSystemCommand(cmdPath string) bool {
	// Check exact matches
	if a.systemCommands[cmdPath] {
		return true
	}

	// Check basename
	baseName := filepath.Base(cmdPath)
	return a.systemCommands[baseName]
}

// isServiceCommand checks if the command is a service command
func (a *DefaultPrivilegeEscalationAnalyzer) isServiceCommand(cmdPath string) bool {
	// Check exact matches
	if a.serviceCommands[cmdPath] {
		return true
	}

	// Check basename
	baseName := filepath.Base(cmdPath)
	return a.serviceCommands[baseName]
}
