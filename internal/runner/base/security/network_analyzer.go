package security

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

type syscallTableInterface interface {
	IsNetworkSyscall(number int) bool
	IsExecSyscall(number int) bool
}

func syscallTableForArch(goos, arch string) syscallTableInterface {
	if goos == isec.GosDarwin {
		return libccache.MacOSSyscallTable{}
	}
	return elfanalyzer.SyscallTableForArchitecture(arch)
}

// RecordStore is an interface for loading precomputed analysis records.
type RecordStore interface {
	LoadRecord(filePath string) (*fileanalysis.Record, error)
}

// AnalysisDeps aggregates analysis stores consumed by NetworkAnalyzer.
// RecordStore provides precomputed analysis records; nil disables record-based checks.
type AnalysisDeps struct {
	RecordStore RecordStore
}

// NetworkAnalyzer provides network operation detection for commands.
type NetworkAnalyzer struct {
	goos string
	deps AnalysisDeps
}

// NewNetworkAnalyzer creates a NetworkAnalyzer with the given analysis dependencies.
// Any nil dependency disables the corresponding analysis.
func NewNetworkAnalyzer(goos string, deps AnalysisDeps) *NetworkAnalyzer {
	return &NetworkAnalyzer{
		goos: isec.RequireGOOS(goos),
		deps: deps,
	}
}

// IsNetworkOperation checks if the command performs network operations.
// This function considers symbolic links to detect network commands properly.
// Returns (isNetwork, isHighRisk, error). isHighRisk is true when any of the
// following conditions hold: symlink depth exceeded; dynamic load symbols
// (dlopen/dlsym) detected in the binary; svc #0x80 syscall detected; or
// exec syscall detected in the binary.
//
// contentHash is a pre-computed hash in "algo:hex" format (e.g. "sha256:abc123...").
// Used to verify that binary analysis is applicable; when empty, binary analysis
// is skipped.
//
// Detection priority:
// 1. commandProfileDefinitions (hardcoded list) - takes precedence
// 2. Record-backed binary analysis for unknown commands (requires RecordStore and contentHash)
// 3. Argument-based detection (URLs, SSH-style addresses)
func (a *NetworkAnalyzer) IsNetworkOperation(cmdName string, args []string, contentHash string) (bool, bool, error) {
	// Extract all possible command names including symlink targets
	commandNames, exceededDepth := extractAllCommandNames(cmdName)

	// If symlink depth exceeded, this is a high risk security concern
	if exceededDepth {
		return false, true, nil
	}

	// Check command profiles for network type using unified profiles
	var conditionalProfile *CommandRiskProfile
	foundInProfiles := false
	for name := range commandNames {
		if profile, exists := commandRiskProfiles[name]; exists {
			foundInProfiles = true
			switch profile.NetworkType {
			case NetworkTypeAlways:
				return true, false, nil
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
				return true, false, nil
			}
		}

		// Check for network-related arguments
		if hasNetworkArguments(args) {
			return true, false, nil
		}
		return false, false, nil
	}

	// If not found in profiles, try binary analysis for unknown commands.
	// Binary analysis requires an absolute path (should be resolved by caller via PathResolver).
	// If cmdName is not absolute, skip binary analysis silently.
	if !foundInProfiles && filepath.IsAbs(cmdName) {
		isNet, hasDynLoad, err := a.analyzeBinarySignals(cmdName, contentHash)
		if err != nil {
			return false, false, err
		}
		if isNet || hasDynLoad {
			return isNet, hasDynLoad, nil
		}
	}

	// Check for network-related arguments in any command
	if hasNetworkArguments(args) {
		return true, false, nil
	}

	return false, false, nil
}

// hasNetworkArguments checks if the arguments contain network indicators.
func hasNetworkArguments(args []string) bool {
	allArgs := strings.Join(args, " ")
	return strings.Contains(allArgs, "://") || // URLs
		containsSSHStyleAddress(args) // SSH-style user@host:path addresses
}

// analyzeBinarySignals analyzes the binary for network usage and dynamic loading
// by loading its precomputed analysis Record.
// Returns (isNetwork, hasDynLoad, nil) where:
//   - isNetwork: true if network symbols or network syscalls were detected
//   - hasDynLoad: true if dynamic load symbols (dlopen/dlsym/dlvsym) were detected,
//     or exec syscalls, or if the record schema is mismatched (fail-closed)
//
// Returns (false, false, nil) when RecordStore is nil or contentHash is empty.
// Returns (true, true, nil) on schema mismatch (fail-closed).
// Returns (false, false, error) on unexpected load failures.
//
// IMPORTANT: cmdPath must be an absolute path.
// contentHash is a pre-computed hash in "algo:hex" format; when empty, analysis is skipped.
func (a *NetworkAnalyzer) analyzeBinarySignals(cmdPath string, contentHash string) (isNetwork, hasDynLoad bool, err error) {
	if !filepath.IsAbs(cmdPath) {
		panic("analyzeBinarySignals: cmdPath must be an absolute path, got: " + cmdPath)
	}

	if a.deps.RecordStore == nil || contentHash == "" {
		return false, false, nil
	}

	record, loadErr := a.deps.RecordStore.LoadRecord(cmdPath)
	if loadErr != nil {
		if errors.Is(loadErr, fileanalysis.ErrRecordNotFound) {
			return false, false, nil
		}
		var schemaMismatch *fileanalysis.SchemaVersionMismatchError
		if errors.As(loadErr, &schemaMismatch) {
			slog.Warn("Record has outdated schema; treating as high risk",
				"path", cmdPath,
				"expected_schema", schemaMismatch.Expected,
				"actual_schema", schemaMismatch.Actual)
			return true, true, nil
		}
		return false, false, fmt.Errorf("failed to load record for %q: %w", cmdPath, loadErr)
	}

	isNetwork, hasDynLoad = a.analyzeRecordSignals(record, cmdPath)
	if hasDynLoad {
		return isNetwork, hasDynLoad, nil
	}

	// Follow the shebang chain: load and analyze each interpreter's record.
	for _, entry := range record.ShebangChain {
		if entry.Path == "" {
			continue
		}
		interpRecord, interpErr := a.deps.RecordStore.LoadRecord(entry.Path)
		if interpErr != nil {
			if errors.Is(interpErr, fileanalysis.ErrRecordNotFound) {
				continue
			}
			var schemaMismatch *fileanalysis.SchemaVersionMismatchError
			if errors.As(interpErr, &schemaMismatch) {
				slog.Warn("Interpreter record has outdated schema; treating as high risk",
					"path", entry.Path,
					"expected_schema", schemaMismatch.Expected,
					"actual_schema", schemaMismatch.Actual)
				return true, true, nil
			}
			return false, false, fmt.Errorf("failed to load interpreter record for %q: %w", entry.Path, interpErr)
		}
		interpNet, interpHigh := a.analyzeRecordSignals(interpRecord, entry.Path)
		isNetwork = isNetwork || interpNet
		hasDynLoad = hasDynLoad || interpHigh
		if hasDynLoad {
			return isNetwork, hasDynLoad, nil
		}
	}

	return isNetwork, hasDynLoad, nil
}

// analyzeRecordSignals extracts network and high-risk signals from a single record.
func (a *NetworkAnalyzer) analyzeRecordSignals(record *fileanalysis.Record, path string) (isNetwork, hasDynLoad bool) {
	if record.SymbolAnalysis != nil {
		output := buildAnalysisOutputFromSymbolData(record.SymbolAnalysis)
		symIsNet, symHigh := handleAnalysisOutput(output, path)
		isNetwork = isNetwork || symIsNet
		hasDynLoad = hasDynLoad || symHigh
	}

	if syscallAnalysisHasSVCSignal(record.SyscallAnalysis) {
		slog.Warn("SyscallAnalysis indicates svc #0x80; treating as high risk", "path", path)
		return true, true
	}
	if syscallAnalysisHasNetworkSignal(record.SyscallAnalysis, a.goos) {
		slog.Info("SyscallAnalysis indicates network syscall", "path", path)
		isNetwork = true
	}
	if syscallAnalysisHasExecSignal(record.SyscallAnalysis, a.goos) {
		slog.Warn("SyscallAnalysis indicates exec syscall; treating as high risk", "path", path)
		hasDynLoad = true
	}

	return isNetwork, hasDynLoad
}

func buildAnalysisOutputFromSymbolData(data *fileanalysis.SymbolAnalysisData) binaryanalyzer.AnalysisOutput {
	output := binaryanalyzer.AnalysisOutput{
		DetectedSymbols:    convertNetworkSymbolEntries(data.DetectedSymbols),
		DynamicLoadSymbols: convertNetworkSymbolEntries(data.DynamicLoadSymbols),
	}

	// Check if any detected symbol has a network category (socket or dns).
	// non-network symbols do not trigger NetworkDetected.
	hasNetworkSymbol := slices.ContainsFunc(data.DetectedSymbols, binaryanalyzer.IsNetworkSymbolName)

	if hasNetworkSymbol {
		output.Result = binaryanalyzer.NetworkDetected
	} else {
		output.Result = binaryanalyzer.NoNetworkSymbols
	}

	return output
}

// syscallAnalysisHasSVCSignal reports whether the given SyscallAnalysisData
// contains evidence of unresolved svc #0x80 direct syscall usage (high risk).
// Returns true only when any DetectedSyscall has Number == -1 and at least one
// Occurrence with DeterminationMethod == "direct_svc_0x80".
// Resolved svc entries (Number != -1) are not treated as high risk here;
// their network classification is handled by syscallAnalysisHasNetworkSignal.
func syscallAnalysisHasSVCSignal(result *fileanalysis.SyscallAnalysisData) bool {
	if result == nil {
		return false
	}
	for _, s := range result.DetectedSyscalls {
		for _, occ := range s.Occurrences {
			if occ.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 && s.Number == -1 {
				return true
			}
		}
	}
	return false
}

// syscallAnalysisHasNetworkSignal reports whether the given SyscallAnalysisData
// contains any detected syscall classified as a network syscall.
// This includes resolved svc entries (DeterminationMethod == "direct_svc_0x80" AND Number != -1)
// whose network classification is determined by the syscall table lookup.
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisData, goos string) bool {
	if result == nil {
		return false
	}
	if len(result.DetectedSyscalls) == 0 {
		return false
	}
	table := syscallTableForArch(goos, result.Architecture)
	if table == nil {
		return false
	}
	for _, s := range result.DetectedSyscalls {
		if s.Number >= 0 && table.IsNetworkSyscall(s.Number) {
			return true
		}
	}
	return false
}

// syscallAnalysisHasExecSignal reports whether the given SyscallAnalysisData
// contains any detected syscall classified as an exec syscall.
// Resolved svc entries (Number != -1) with exec classification are included.
func syscallAnalysisHasExecSignal(result *fileanalysis.SyscallAnalysisData, goos string) bool {
	if result == nil {
		return false
	}
	if len(result.DetectedSyscalls) == 0 {
		return false
	}
	table := syscallTableForArch(goos, result.Architecture)
	if table == nil {
		return false
	}
	for _, s := range result.DetectedSyscalls {
		if s.Number >= 0 && table.IsExecSyscall(s.Number) {
			return true
		}
	}
	return false
}

// handleAnalysisOutput maps a binaryanalyzer.AnalysisOutput to (isNetwork, isHighRisk).
func handleAnalysisOutput(output binaryanalyzer.AnalysisOutput, cmdPath string) (isNetwork, isHighRisk bool) {
	if len(output.DynamicLoadSymbols) > 0 {
		isHighRisk = true
		slog.Info("Binary analysis detected dynamic load symbols; set risk_level = \"high\" or higher to allow execution",
			"path", cmdPath,
			"symbols", strings.Join(binaryanalyzer.DynamicLoadSymbolNames(), "/"))
	}

	switch output.Result {
	case binaryanalyzer.NetworkDetected:
		slog.Info("Binary analysis detected network symbols; set risk_level = \"medium\" or higher to allow execution", //nolint:gosec // G706: cmdPath is a configured command path from TOML, not arbitrary user input
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
		// Analysis failed: cannot determine network capability, treat as high risk.
		// This includes ErrSyscallHashMismatch (binary changed since record time).
		slog.Warn("Binary analysis failed, treating as high risk",
			"path", cmdPath,
			"error", output.Error)
		return true, true

	default:
		// Unknown result: treat as potential network operation for safety
		slog.Warn("Binary analysis returned unknown result", //nolint:gosec // G706: cmdPath is a configured command path from TOML, not arbitrary user input
			"path", cmdPath,
			"result", output.Result)
		return true, isHighRisk
	}
}

// convertNetworkSymbolEntries converts []string to binaryanalyzer.DetectedSymbol slice.
//
// NOTE: This is the inverse of convertDetectedSymbols in
// internal/filevalidator/validator.go. fileanalysis stores symbol names as
// plain strings, and this
// function derives Category for runner-internal logging and filtering.
func convertNetworkSymbolEntries(entries []string) []binaryanalyzer.DetectedSymbol {
	if len(entries) == 0 {
		return nil
	}
	syms := make([]binaryanalyzer.DetectedSymbol, len(entries))
	for i, e := range entries {
		cat, found := binaryanalyzer.IsNetworkSymbol(e)
		if !found {
			if binaryanalyzer.IsDynamicLoadSymbol(e) {
				cat = binaryanalyzer.CategoryDynamicLoad
			} else {
				cat = binaryanalyzer.CategorySyscallWrapper
			}
		}
		syms[i] = binaryanalyzer.DetectedSymbol{Name: e, Category: string(cat)}
	}
	return syms
}
