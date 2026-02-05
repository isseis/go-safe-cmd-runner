package security

import (
	"log/slog"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// NetworkAnalyzer provides network operation detection for commands.
type NetworkAnalyzer struct {
	elfAnalyzer elfanalyzer.ELFAnalyzer
}

// NewNetworkAnalyzer creates a new NetworkAnalyzer with a default StandardELFAnalyzer.
func NewNetworkAnalyzer() *NetworkAnalyzer {
	return &NetworkAnalyzer{elfAnalyzer: elfanalyzer.NewStandardELFAnalyzer(nil, nil)}
}

// IsNetworkOperation checks if the command performs network operations.
// This function considers symbolic links to detect network commands properly.
// Returns (isNetwork, isHighRisk) where isHighRisk indicates symlink depth exceeded.
//
// Detection priority:
// 1. commandProfileDefinitions (hardcoded list) - takes precedence
// 2. ELF .dynsym analysis for unknown commands
// 3. Argument-based detection (URLs, SSH-style addresses)
func (a *NetworkAnalyzer) IsNetworkOperation(cmdName string, args []string) (bool, bool) {
	// Extract all possible command names including symlink targets
	commandNames, exceededDepth := extractAllCommandNames(cmdName)

	// If symlink depth exceeded, this is a high risk security concern
	if exceededDepth {
		return false, true
	}

	// Check command profiles for network type using unified profiles
	var conditionalProfile *CommandRiskProfile
	foundInProfiles := false
	for name := range commandNames {
		if profile, exists := commandRiskProfiles[name]; exists {
			foundInProfiles = true
			switch profile.NetworkType {
			case NetworkTypeAlways:
				return true, false
			case NetworkTypeConditional:
				conditionalProfile = &profile
			}
		}
	}

	if conditionalProfile != nil {
		// Check for network subcommands (e.g., git fetch, git push)
		// Skip command-line options to find the actual subcommand
		if len(conditionalProfile.NetworkSubcommands) > 0 {
			subcommand := findFirstSubcommand(args)
			if subcommand != "" && slices.Contains(conditionalProfile.NetworkSubcommands, subcommand) {
				return true, false
			}
		}

		// Check for network-related arguments
		allArgs := strings.Join(args, " ")
		if strings.Contains(allArgs, "://") || // URLs
			containsSSHStyleAddress(args) { // SSH-style user@host:path addresses
			return true, false
		}
		return false, false
	}

	// If not found in profiles, try ELF analysis for unknown commands
	if !foundInProfiles {
		if a.analyzeELFForNetwork(cmdName) {
			return true, false
		}
	}

	// Check for network-related arguments in any command
	allArgs := strings.Join(args, " ")
	if strings.Contains(allArgs, "://") || // URLs
		containsSSHStyleAddress(args) { // SSH-style user@host:path addresses
		return true, false
	}

	return false, false
}

// analyzeELFForNetwork performs ELF .dynsym analysis on the command binary.
// Returns true if the command should be treated as a network operation.
// This includes both confirmed network symbols (NetworkDetected) and
// analysis failures (AnalysisError), which are treated as potential
// network operations for safety (middle risk → RiskLevelMedium).
func (a *NetworkAnalyzer) analyzeELFForNetwork(cmdName string) bool {
	// Resolve command path
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		// Cannot find command, skip ELF analysis
		slog.Debug("ELF analysis skipped: command not found in PATH",
			"command", cmdName,
			"error", err)
		return false
	}

	// LookPath may return a relative path if PATH contains relative entries.
	// AnalyzeNetworkSymbols requires an absolute path.
	cmdPath, err = filepath.Abs(cmdPath)
	if err != nil {
		slog.Debug("ELF analysis skipped: failed to resolve absolute path",
			"command", cmdName,
			"error", err)
		return false
	}

	// Resolve symlinks to get the actual binary path.
	// This is necessary because safefileio.SafeOpenFile rejects symlinks for security.
	// Standard system paths like /bin/echo -> /usr/bin/echo are legitimate symlinks.
	realPath, err := filepath.EvalSymlinks(cmdPath)
	if err != nil {
		slog.Debug("ELF analysis skipped: failed to resolve symlinks",
			"command", cmdName,
			"path", cmdPath,
			"error", err)
		return false
	}
	cmdPath = realPath

	// Perform ELF analysis
	output := a.elfAnalyzer.AnalyzeNetworkSymbols(cmdPath)

	switch output.Result {
	case elfanalyzer.NetworkDetected:
		slog.Debug("ELF analysis detected network symbols",
			"command", cmdName,
			"path", cmdPath,
			"symbols", formatDetectedSymbols(output.DetectedSymbols))
		return true

	case elfanalyzer.NoNetworkSymbols:
		slog.Debug("ELF analysis found no network symbols",
			"command", cmdName,
			"path", cmdPath)
		return false

	case elfanalyzer.NotELFBinary:
		slog.Debug("ELF analysis skipped: not an ELF binary",
			"command", cmdName,
			"path", cmdPath)
		return false

	case elfanalyzer.StaticBinary:
		// Static binary: cannot determine network capability
		// Return false for now, 2nd step (Task 0070) will handle this
		slog.Debug("ELF analysis: static binary detected, cannot determine network capability",
			"command", cmdName,
			"path", cmdPath)
		return false

	case elfanalyzer.AnalysisError:
		// Analysis failed: treat as potential network operation for safety
		slog.Warn("ELF analysis failed, treating as potential network operation",
			"command", cmdName,
			"path", cmdPath,
			"error", output.Error,
			"reason", "Unable to determine network capability, assuming middle risk for safety")
		return true

	default:
		// Unknown result: treat as potential network operation for safety
		slog.Warn("ELF analysis returned unknown result",
			"command", cmdName,
			"path", cmdPath,
			"result", output.Result)
		return true
	}
}
