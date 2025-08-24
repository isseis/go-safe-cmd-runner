package security

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// PrivilegeEscalationType represents different types of privilege escalation
type PrivilegeEscalationType string

const (
	// PrivilegeEscalationTypeSudo represents sudo-like privilege escalation commands
	PrivilegeEscalationTypeSudo PrivilegeEscalationType = "sudo"
	// PrivilegeEscalationTypeSu represents su-like privilege escalation commands
	PrivilegeEscalationTypeSu PrivilegeEscalationType = "su"
	// PrivilegeEscalationTypeSystemd represents systemd service control commands
	PrivilegeEscalationTypeSystemd PrivilegeEscalationType = "systemd"
	// PrivilegeEscalationTypeService represents legacy service control commands
	PrivilegeEscalationTypeService PrivilegeEscalationType = "service"
	// PrivilegeEscalationTypeOther represents other types of privilege escalation
	PrivilegeEscalationTypeOther PrivilegeEscalationType = "other"
)

// PrivilegeEscalationResult contains the result of privilege escalation analysis
type PrivilegeEscalationResult struct {
	IsPrivilegeEscalation bool
	EscalationType        PrivilegeEscalationType
	RiskLevel             RiskLevel
	RequiredPrivileges    []string
	CommandPath           string
	DetectedPattern       string
	Reason                string
}

// PrivilegeEscalationAnalyzer interface defines methods for privilege escalation analysis
type PrivilegeEscalationAnalyzer interface {
	AnalyzePrivilegeEscalation(ctx context.Context, cmdName string, args []string) (*PrivilegeEscalationResult, error)
	IsPrivilegeEscalationCommand(cmdName string) bool
	GetRequiredPrivileges(cmdName string, args []string) ([]string, error)
}

// PrivilegeCheckInfo contains information about a specific privilege escalation command
type PrivilegeCheckInfo struct {
	EscalationType     PrivilegeEscalationType
	RiskLevel          RiskLevel
	RequiredPrivileges []string
	Reason             string
}

// DefaultPrivilegeEscalationAnalyzer is the default implementation of PrivilegeEscalationAnalyzer
type DefaultPrivilegeEscalationAnalyzer struct {
	logger        *slog.Logger
	commandChecks map[string]*PrivilegeCheckInfo
}

// NewDefaultPrivilegeEscalationAnalyzer creates a new DefaultPrivilegeEscalationAnalyzer
func NewDefaultPrivilegeEscalationAnalyzer(logger *slog.Logger) *DefaultPrivilegeEscalationAnalyzer {
	commandChecks := map[string]*PrivilegeCheckInfo{
		// Sudo-like commands
		"sudo": {
			EscalationType:     PrivilegeEscalationTypeSudo,
			RiskLevel:          RiskLevelHigh,
			RequiredPrivileges: []string{"root"},
			Reason:             "Command requires root privileges for execution",
		},
		"su": {
			EscalationType:     PrivilegeEscalationTypeSu,
			RiskLevel:          RiskLevelHigh,
			RequiredPrivileges: []string{"root"},
			Reason:             "Command requires root privileges for execution",
		},
		"doas": {
			EscalationType:     PrivilegeEscalationTypeSudo,
			RiskLevel:          RiskLevelHigh,
			RequiredPrivileges: []string{"root"},
			Reason:             "Command requires root privileges for execution",
		},
		// Systemd commands
		"systemctl": {
			EscalationType:     PrivilegeEscalationTypeSystemd,
			RiskLevel:          RiskLevelMedium,
			RequiredPrivileges: []string{"systemd"},
			Reason:             "Command can control system services",
		},
		// Service commands
		"service": {
			EscalationType:     PrivilegeEscalationTypeService,
			RiskLevel:          RiskLevelMedium,
			RequiredPrivileges: []string{"service"},
			Reason:             "Command can control system services",
		},
	}

	return &DefaultPrivilegeEscalationAnalyzer{
		logger:        logger,
		commandChecks: commandChecks,
	}
}

// AnalyzePrivilegeEscalation analyzes whether a command involves privilege escalation
func (a *DefaultPrivilegeEscalationAnalyzer) AnalyzePrivilegeEscalation(
	_ context.Context, cmdName string, _ []string,
) (*PrivilegeEscalationResult, error) {
	// Resolve command path
	commandPath, err := a.resolveCommandPath(cmdName)
	if err != nil {
		a.logger.Debug("failed to resolve command path", "command", cmdName, "error", err)
		commandPath = cmdName
	}

	result := &PrivilegeEscalationResult{
		IsPrivilegeEscalation: false,
		EscalationType:        "",
		RiskLevel:             RiskLevelNone,
		RequiredPrivileges:    []string{},
		CommandPath:           commandPath,
		DetectedPattern:       "",
		Reason:                "",
	}

	// Get base command name from path
	baseCommand := filepath.Base(commandPath)

	// Check if this command is in our privilege escalation command map
	if checkInfo, exists := a.commandChecks[baseCommand]; exists {
		result.IsPrivilegeEscalation = true
		result.EscalationType = checkInfo.EscalationType
		result.RiskLevel = checkInfo.RiskLevel
		result.RequiredPrivileges = checkInfo.RequiredPrivileges
		result.DetectedPattern = baseCommand
		result.Reason = checkInfo.Reason

		a.logger.Info("privilege escalation detected",
			"type", result.EscalationType,
			"command", cmdName,
			"path", commandPath,
			"risk_level", result.RiskLevel)

		return result, nil
	}

	a.logger.Debug("no privilege escalation detected",
		"command", cmdName,
		"path", commandPath)

	return result, nil
}

// IsPrivilegeEscalationCommand checks if a command is a privilege escalation command
func (a *DefaultPrivilegeEscalationAnalyzer) IsPrivilegeEscalationCommand(cmdName string) bool {
	baseCommand := filepath.Base(cmdName)
	_, exists := a.commandChecks[baseCommand]
	return exists
}

// GetRequiredPrivileges returns the required privileges for a command
func (a *DefaultPrivilegeEscalationAnalyzer) GetRequiredPrivileges(
	cmdName string, _ []string,
) ([]string, error) {
	baseCommand := filepath.Base(cmdName)

	if checkInfo, exists := a.commandChecks[baseCommand]; exists {
		return checkInfo.RequiredPrivileges, nil
	}

	return []string{}, nil
}

// resolveCommandPath resolves command path, handling symlinks
func (a *DefaultPrivilegeEscalationAnalyzer) resolveCommandPath(cmdName string) (string, error) {
	// If cmdName is already an absolute path, resolve symlinks
	if filepath.IsAbs(cmdName) {
		return a.resolveSymlinks(cmdName)
	}

	// If cmdName contains path separators but isn't absolute, resolve relative path
	if strings.Contains(cmdName, string(filepath.Separator)) {
		absPath, err := filepath.Abs(cmdName)
		if err != nil {
			return "", err
		}
		return a.resolveSymlinks(absPath)
	}

	// Search in PATH
	return a.searchInPath(cmdName)
}

// resolveSymlinks resolves symbolic links up to MaxSymlinkDepth
func (a *DefaultPrivilegeEscalationAnalyzer) resolveSymlinks(path string) (string, error) {
	resolved := path
	for i := 0; i < MaxSymlinkDepth; i++ {
		info, err := os.Lstat(resolved)
		if err != nil {
			return "", err
		}

		if info.Mode()&os.ModeSymlink == 0 {
			// Not a symlink, we're done
			return resolved, nil
		}

		// Read symlink target
		target, err := os.Readlink(resolved)
		if err != nil {
			return "", err
		}

		// If target is relative, make it relative to the symlink's directory
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(resolved), target)
		}

		resolved = target
	}

	return "", ErrSymlinkDepthExceeded
}

// searchInPath searches for command in PATH environment variable
func (a *DefaultPrivilegeEscalationAnalyzer) searchInPath(cmdName string) (string, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return "", ErrInvalidPath
	}

	paths := filepath.SplitList(pathEnv)
	for _, path := range paths {
		if path == "" {
			continue
		}

		fullPath := filepath.Join(path, cmdName)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			return a.resolveSymlinks(fullPath)
		}
	}

	return "", ErrInvalidPath
}
