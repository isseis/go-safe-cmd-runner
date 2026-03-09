package security

import (
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"
)

// gosDarwin is the GOOS value for macOS.
const gosDarwin = "darwin"

// NetworkAnalyzer provides network operation detection for commands.
type NetworkAnalyzer struct {
	binaryAnalyzer binaryanalyzer.BinaryAnalyzer
}

// NewBinaryAnalyzer creates a BinaryAnalyzer appropriate for the current platform.
// On macOS, returns StandardMachOAnalyzer; on Linux and other platforms, returns StandardELFAnalyzer.
func NewBinaryAnalyzer() binaryanalyzer.BinaryAnalyzer {
	switch runtime.GOOS {
	case gosDarwin:
		return machoanalyzer.NewStandardMachOAnalyzer(nil)
	default: // "linux", etc.
		return elfanalyzer.NewStandardELFAnalyzer(nil, nil)
	}
}

// NewNetworkAnalyzer creates a new NetworkAnalyzer.
// On macOS, uses StandardMachOAnalyzer; on Linux and other platforms, uses StandardELFAnalyzer.
func NewNetworkAnalyzer() *NetworkAnalyzer {
	return &NetworkAnalyzer{binaryAnalyzer: NewBinaryAnalyzer()}
}

// IsNetworkOperation checks if the command performs network operations.
// This function considers symbolic links to detect network commands properly.
// Returns (isNetwork, isHighRisk) where isHighRisk indicates symlink depth exceeded.
//
// contentHash is a pre-computed hash in "algo:hex" format (e.g. "sha256:abc123...").
// When non-empty it is forwarded to ELF analysis for static binaries to avoid
// re-reading the binary. Pass empty string when no hash is available.
//
// Detection priority:
// 1. commandProfileDefinitions (hardcoded list) - takes precedence
// 2. ELF .dynsym analysis for unknown commands
// 3. Argument-based detection (URLs, SSH-style addresses)
func (a *NetworkAnalyzer) IsNetworkOperation(cmdName string, args []string, contentHash string) (bool, bool) {
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

	// If not found in profiles, try binary analysis for unknown commands.
	// Binary analysis requires an absolute path (should be resolved by caller via PathResolver).
	// If cmdName is not absolute, skip binary analysis silently.
	if !foundInProfiles && filepath.IsAbs(cmdName) {
		isNet, isHigh := a.isNetworkViaBinaryAnalysis(cmdName, contentHash)
		if isNet || isHigh {
			return isNet, isHigh
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

// isNetworkViaBinaryAnalysis performs binary analysis on the command binary.
// Returns (isNetwork, isHighRisk) where:
//   - isNetwork: true if confirmed network symbols were found or analysis failed (safety)
//   - isHighRisk: true if dynamic load symbols (dlopen/dlsym/dlvsym) were detected
//
// HasDynamicLoad and network detection are independent signals.
// A binary with both dlopen and socket will return (true, true).
//
// IMPORTANT: cmdPath is expected to be an absolute, symlink-resolved path,
// already resolved by the caller (via verification.PathResolver.ResolvePath()).
// This ensures TOCTOU safety and consistency across all security checks.
//
// contentHash is a pre-computed hash in "algo:hex" format that is forwarded to
// the binary analyzer to avoid redundant hashing for static binaries with a
// syscall store configured. Pass empty string when no hash is available.
func (a *NetworkAnalyzer) isNetworkViaBinaryAnalysis(cmdPath string, contentHash string) (isNetwork, isHighRisk bool) {
	// Validate that cmdPath is an absolute path.
	// The caller (EvaluateRisk via group_executor) must have already resolved the path.
	// A non-absolute path here indicates a programming error in the call chain.
	if !filepath.IsAbs(cmdPath) {
		panic("isNetworkViaBinaryAnalysis: cmdPath must be an absolute path, got: " + cmdPath)
	}

	// cmdPath is already symlink-resolved by PathResolver.ResolvePath(),
	// so no need for filepath.EvalSymlinks() here.

	// Perform binary analysis
	output := a.binaryAnalyzer.AnalyzeNetworkSymbols(cmdPath, contentHash)

	if output.HasDynamicLoad {
		isHighRisk = true
		slog.Debug("Binary analysis detected dynamic load symbols",
			"path", cmdPath,
			"symbols", strings.Join(binaryanalyzer.DynamicLoadSymbolNames(), "/"))
	}

	switch output.Result {
	case binaryanalyzer.NetworkDetected:
		slog.Debug("Binary analysis detected network symbols",
			"path", cmdPath,
			"symbols", formatDetectedSymbols(output.DetectedSymbols))
		return true, isHighRisk

	case binaryanalyzer.NoNetworkSymbols:
		slog.Debug("Binary analysis found no network symbols",
			"path", cmdPath)
		return false, isHighRisk

	case binaryanalyzer.NotSupportedBinary:
		// File format is not supported by this analyzer (e.g., ELF analyzer
		// receiving a Mach-O, or Mach-O analyzer receiving an ELF).
		// Assume no network operation, consistent with binary format mismatch handling.
		slog.Debug("Binary analysis: unsupported binary format, assuming no network operation",
			"path", cmdPath)
		return false, isHighRisk

	case binaryanalyzer.StaticBinary:
		// Static binary: cannot determine network capability
		// Return false for now, 2nd step (Task 0070) will handle this
		slog.Debug("Binary analysis: static binary detected, cannot determine network capability",
			"path", cmdPath)
		return false, isHighRisk

	case binaryanalyzer.AnalysisError:
		// Analysis failed: treat as potential network operation for safety
		slog.Warn("Binary analysis failed, treating as potential network operation",
			"path", cmdPath,
			"error", output.Error,
			"reason", "Unable to determine network capability, assuming middle risk for safety")
		return true, isHighRisk

	default:
		// Unknown result: treat as potential network operation for safety
		slog.Warn("Binary analysis returned unknown result",
			"path", cmdPath,
			"result", output.Result)
		return true, isHighRisk
	}
}
