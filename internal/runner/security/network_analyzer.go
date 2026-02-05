package security

import (
	"log/slog"
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
		if hasNetworkArguments(args) {
			return true, false
		}
		return false, false
	}

	// If not found in profiles, try ELF analysis for unknown commands.
	// ELF analysis requires an absolute path (should be resolved by caller via PathResolver).
	// If cmdName is not absolute, skip ELF analysis silently.
	if !foundInProfiles && filepath.IsAbs(cmdName) {
		if a.isNetworkViaELFAnalysis(cmdName) {
			return true, false
		}
	}

	// Check for network-related arguments in any command
	if hasNetworkArguments(args) {
		return true, false
	}

	return false, false
}

// hasNetworkArguments checks if the arguments contain network indicators.
func hasNetworkArguments(args []string) bool {
	allArgs := strings.Join(args, " ")
	return strings.Contains(allArgs, "://") || // URLs
		containsSSHStyleAddress(args) // SSH-style user@host:path addresses
}

// isNetworkViaELFAnalysis performs ELF .dynsym analysis on the command binary.
// Returns true if the command should be treated as a network operation.
// This includes both confirmed network symbols (NetworkDetected) and
// analysis failures (AnalysisError), which are treated as potential
// network operations for safety (middle risk → RiskLevelMedium).
//
// IMPORTANT: cmdPath is expected to be an absolute, symlink-resolved path,
// already resolved by the caller (via verification.PathResolver.ResolvePath()).
// This ensures TOCTOU safety and consistency across all security checks.
func (a *NetworkAnalyzer) isNetworkViaELFAnalysis(cmdPath string) bool {
	// Validate that cmdPath is an absolute path.
	// The caller (EvaluateRisk via group_executor) must have already resolved the path.
	// A non-absolute path here indicates a programming error in the call chain.
	if !filepath.IsAbs(cmdPath) {
		panic("isNetworkViaELFAnalysis: cmdPath must be an absolute path, got: " + cmdPath)
	}

	// cmdPath is already symlink-resolved by PathResolver.ResolvePath(),
	// so no need for filepath.EvalSymlinks() here.

	// Perform ELF analysis
	output := a.elfAnalyzer.AnalyzeNetworkSymbols(cmdPath)

	switch output.Result {
	case elfanalyzer.NetworkDetected:
		slog.Debug("ELF analysis detected network symbols",
			"path", cmdPath,
			"symbols", formatDetectedSymbols(output.DetectedSymbols))
		return true

	case elfanalyzer.NoNetworkSymbols:
		slog.Debug("ELF analysis found no network symbols",
			"path", cmdPath)
		return false

	case elfanalyzer.NotELFBinary:
		// Non-ELF binaries (scripts, shell commands) can invoke arbitrary commands internally,
		// so they should be treated as potential network operations for safety.
		slog.Debug("ELF analysis: not an ELF binary, treating as potential network operation",
			"path", cmdPath,
			"reason", "Scripts and shell commands can invoke network commands internally")
		return true

	case elfanalyzer.StaticBinary:
		// Static binary: cannot determine network capability
		// Return false for now, 2nd step (Task 0070) will handle this
		slog.Debug("ELF analysis: static binary detected, cannot determine network capability",
			"path", cmdPath)
		return false

	case elfanalyzer.AnalysisError:
		// Analysis failed: treat as potential network operation for safety
		slog.Warn("ELF analysis failed, treating as potential network operation",
			"path", cmdPath,
			"error", output.Error,
			"reason", "Unable to determine network capability, assuming middle risk for safety")
		return true

	default:
		// Unknown result: treat as potential network operation for safety
		slog.Warn("ELF analysis returned unknown result",
			"path", cmdPath,
			"result", output.Result)
		return true
	}
}
