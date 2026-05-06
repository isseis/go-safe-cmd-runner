package security

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

type syscallTableInterface interface {
	IsNetworkSyscall(number int) bool
}

func syscallTableForArch(goos, arch string) syscallTableInterface {
	if goos == isec.GosDarwin {
		return libccache.MacOSSyscallTable{}
	}
	return elfanalyzer.SyscallTableForArchitecture(arch)
}

// NetworkAnalyzer provides network operation detection for commands.
type NetworkAnalyzer struct {
	goos             string
	store            fileanalysis.NetworkSymbolStore   // nil means cache disabled
	syscallStore     fileanalysis.SyscallAnalysisStore // nil means svc cache disabled
	depsStore        fileanalysis.DynLibDepsStore      // nil means dynlib check disabled
	libAnalysisStore dynamicanalysis.Store             // nil means dynlib check disabled
}

// NewNetworkAnalyzer creates a NetworkAnalyzer with the given stores.
// Any nil store disables the corresponding analysis.
func NewNetworkAnalyzer(
	goos string,
	symStore fileanalysis.NetworkSymbolStore,
	svcStore fileanalysis.SyscallAnalysisStore,
	depsStore fileanalysis.DynLibDepsStore,
	libAnalysisStore dynamicanalysis.Store,
) *NetworkAnalyzer {
	return &NetworkAnalyzer{
		goos:             isec.RequireGOOS(goos),
		store:            symStore,
		syscallStore:     svcStore,
		depsStore:        depsStore,
		libAnalysisStore: libAnalysisStore,
	}
}

// IsNetworkOperation checks if the command performs network operations.
// This function considers symbolic links to detect network commands properly.
// Returns (isNetwork, isHighRisk) where isHighRisk indicates symlink depth exceeded.
//
// contentHash is a pre-computed hash in "algo:hex" format (e.g. "sha256:abc123...").
// Used to verify cache record freshness when store-based binary analysis runs.
//
// Detection priority:
// 1. commandProfileDefinitions (hardcoded list) - takes precedence
// 2. Cache-backed binary analysis for unknown commands (requires store and contentHash)
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
// DynamicLoadSymbols and network detection are independent signals.
// A binary with both dlopen and socket will return (true, true).
//
// IMPORTANT: cmdPath is expected to be an absolute, symlink-resolved path,
// already resolved by the caller (via verification.PathResolver.ResolvePath()).
// This ensures TOCTOU safety and consistency across all security checks.
//
// contentHash is a pre-computed hash in "algo:hex" format. Used to verify cache
// record freshness; when empty or store is nil, analysis is skipped (returning false, false).
func (a *NetworkAnalyzer) isNetworkViaBinaryAnalysis(cmdPath string, contentHash string) (isNetwork, isHighRisk bool) {
	// Validate that cmdPath is an absolute path.
	// The caller (EvaluateRisk via group_executor) must have already resolved the path.
	// A non-absolute path here indicates a programming error in the call chain.
	if !filepath.IsAbs(cmdPath) {
		panic("isNetworkViaBinaryAnalysis: cmdPath must be an absolute path, got: " + cmdPath)
	}

	// cmdPath is already symlink-resolved by PathResolver.ResolvePath(),
	// so no need for filepath.EvalSymlinks() here.

	// Check cache first (when store is configured and contentHash is available).
	// Skip cache lookup when contentHash is empty: the store uses hash to verify
	// record freshness, so an empty hash would always produce ErrHashMismatch even
	// for a valid record. That would misuse ErrHashMismatch (which signals a genuine
	// file-content change) for a mere "no hash available" situation.
	if a.store != nil && contentHash != "" {
		// Load SymbolAnalysis cache.
		// (nil, nil) means no network symbol analysis stored (e.g., not applicable or none detected): fall through to SyscallAnalysis.
		// All other errors are treated as AnalysisError because production always has records.
		data, err := a.store.LoadNetworkSymbolAnalysis(cmdPath, contentHash)
		var symSchemaMismatch *fileanalysis.SchemaVersionMismatchError
		switch {
		case err == nil:
			// data is valid (or nil when analyzed but no symbols found); continue below.
		case errors.Is(err, fileanalysis.ErrHashMismatch):
			slog.Warn("SymbolAnalysis cache hash mismatch; treating as high risk",
				"path", cmdPath)
			return true, true
		case errors.As(err, &symSchemaMismatch):
			slog.Warn("SymbolAnalysis cache has outdated schema; treating as high risk",
				"path", cmdPath,
				"expected_schema", symSchemaMismatch.Expected,
				"actual_schema", symSchemaMismatch.Actual)
			return true, true
		default:
			slog.Warn("SymbolAnalysis cache load failed; treating as high risk",
				"path", cmdPath, "error", err)
			return true, true
		}

		// Check SyscallAnalysis cache for svc #0x80 signal (Mach-O arm64).
		// This check runs regardless of SymbolAnalysis result (NetworkDetected, NoNetworkSymbols,
		// or nil for static binaries) so that svc #0x80 always escalates
		// isHighRisk to true.
		if a.syscallStore != nil {
			svcResult, svcErr := a.syscallStore.LoadSyscallAnalysis(cmdPath, contentHash)
			var svcSchemaMismatch *fileanalysis.SchemaVersionMismatchError
			switch {
			case svcErr == nil:
				if syscallAnalysisHasSVCSignal(svcResult) {
					slog.Warn("SyscallAnalysis cache indicates svc #0x80; treating as high risk",
						"path", cmdPath)
					return true, true
				}
				// Check whether any non-svc detected syscall is a network syscall.
				if syscallAnalysisHasNetworkSignal(svcResult, a.goos) {
					slog.Info("SyscallAnalysis cache indicates network syscall",
						"path", cmdPath)
					return true, false
				}
				// No svc signal and no network signal: fall through to SymbolAnalysis-based decision.

			case errors.Is(svcErr, fileanalysis.ErrHashMismatch):
				slog.Warn("SyscallAnalysis cache hash mismatch; treating as high risk",
					"path", cmdPath)
				return true, true
			case errors.As(svcErr, &svcSchemaMismatch):
				slog.Warn("SyscallAnalysis cache has outdated schema; treating as high risk",
					"path", cmdPath,
					"expected_schema", svcSchemaMismatch.Expected,
					"actual_schema", svcSchemaMismatch.Actual)
				return true, true
			default:
				// ErrRecordNotFound or unexpected error: this must not occur in production.
				// The underlying record exists (SymbolAnalysis succeeded or returned
				// nil for static binary), so a missing SyscallAnalysis record indicates
				// a consistency bug that must be fixed, not silently absorbed.
				panic(fmt.Sprintf("SyscallAnalysis cache inconsistency for %q: %v", cmdPath, svcErr))
			}
		}

		// No svc #0x80 signal: determine result from SymbolAnalysis.
		// data == nil when analyzed but no network symbols found (static binary with no svc).
		// Non-nil: check for network/high-risk signal; fall through to dynlib check if none.
		if data != nil {
			output := buildAnalysisOutputFromSymbolData(data, cmdPath)
			isNet, isHigh := handleAnalysisOutput(output, cmdPath)
			if isNet || isHigh {
				return isNet, isHigh
			}
			// No network signal from binary body: fall through to dynlib check.
		}
	}

	// Additional dynlib analysis: check per-library network signals.
	// Runs when depsStore and libAnalysisStore are both configured and contentHash is available.
	// Fail-closed: any loading error is treated as high risk.
	if a.depsStore != nil && a.libAnalysisStore != nil && contentHash != "" {
		return a.checkDynLibDepsNetwork(cmdPath, contentHash)
	}

	// No store configured or contentHash empty: skip analysis.
	return false, false
}

// checkDynLibDepsNetwork checks network capability by loading per-library analysis
// results for each dynamic dependency of the given command.
// Fail-closed: any loading failure returns (true, true).
func (a *NetworkAnalyzer) checkDynLibDepsNetwork(cmdPath, contentHash string) (isNetwork, isHighRisk bool) {
	deps, err := a.depsStore.LoadDynLibDeps(cmdPath, contentHash)
	if err != nil {
		var schemaMismatch *fileanalysis.SchemaVersionMismatchError
		switch {
		case errors.Is(err, fileanalysis.ErrHashMismatch):
			slog.Error("DynLibDeps record hash mismatch; treating as high risk", "path", cmdPath)
		case errors.As(err, &schemaMismatch):
			slog.Warn("DynLibDeps record has outdated schema; treating as high risk",
				"path", cmdPath,
				"expected_schema", schemaMismatch.Expected,
				"actual_schema", schemaMismatch.Actual)
		default:
			slog.Error("DynLibDeps record load failed; treating as high risk",
				"path", cmdPath, "error", err)
		}
		return true, true
	}

	if len(deps) == 0 {
		// Static binary or no deps recorded: no dynlib network signal.
		return false, false
	}

	var (
		dynLoadLog  onceLogger
		networkLog  onceLogger
		mprotectLog onceLogger
	)

	for _, dep := range deps {
		// Skip VDSO entries (no real file) and known syscall wrapper libraries.
		if isVDSOEntry(dep.SOName) || binaryanalyzer.IsSyscallWrapperLibrary(dep.SOName) {
			continue
		}

		result, loadErr := a.libAnalysisStore.LoadAnalysis(dep.Path, dep.Hash)
		if loadErr != nil {
			if errors.Is(loadErr, dynamicanalysis.ErrAnalysisNotFound) {
				slog.Warn("dynlib analysis not found; treating as high risk",
					"cmd_path", cmdPath, "dep_path", dep.Path, "dep_hash", dep.Hash)
			} else {
				slog.Error("dynlib analysis load failed; treating as high risk",
					"cmd_path", cmdPath, "dep_path", dep.Path, "error", loadErr)
			}
			return true, true
		}

		if result == nil {
			continue
		}

		sigs := a.analyzeDepSignals(result)

		if len(sigs.dynLoadSymbols) > 0 {
			dynLoadLog.log("dynlib analysis detected dynamic load symbols",
				"cmd_path", cmdPath, "dep_path", dep.Path, "symbols", sigs.dynLoadSymbols)
			isHighRisk = true
		}
		if len(sigs.networkSymbols) > 0 {
			networkLog.log("dynlib analysis detected network symbols",
				"cmd_path", cmdPath, "dep_path", dep.Path, "symbols", sigs.networkSymbols)
			isNetwork = true
		}
		if sigs.networkSyscall != "" {
			networkLog.log("dynlib analysis detected network syscall",
				"cmd_path", cmdPath, "dep_path", dep.Path, "syscall", sigs.networkSyscall)
			isNetwork = true
		}
		if sigs.hasMprotectRisk {
			mprotectLog.log("dynlib analysis detected mprotect-family PROT_EXEC risk",
				"cmd_path", cmdPath, "dep_path", dep.Path,
				"syscall", sigs.mprotectRisk.SyscallName, "status", sigs.mprotectRisk.Status)
			isHighRisk = true
		}
	}

	return isNetwork, isHighRisk
}

// onceLogger emits a slog.Info message at most once.
type onceLogger struct{ logged bool }

func (l *onceLogger) log(msg string, args ...any) {
	if !l.logged {
		slog.Info(msg, args...)
		l.logged = true
	}
}

// depSignals holds the network/risk signals extracted from one library analysis result.
type depSignals struct {
	dynLoadSymbols  []string
	networkSymbols  []string
	networkSyscall  string
	mprotectRisk    common.SyscallArgEvalResult
	hasMprotectRisk bool
}

// analyzeDepSignals extracts all network and risk signals from result.
func (a *NetworkAnalyzer) analyzeDepSignals(result *dynamicanalysis.Result) depSignals {
	var s depSignals
	s.dynLoadSymbols = result.DynamicLoadSymbols()
	if result.SymbolAnalysis != nil {
		s.networkSymbols = result.SymbolAnalysis.DetectedSymbols
	}
	if result.SyscallAnalysis != nil {
		table := syscallTableForArch(a.goos, result.SyscallAnalysis.Architecture)
		s.networkSyscall = firstNetworkSyscall(table, result.SyscallAnalysis)
		s.mprotectRisk, s.hasMprotectRisk = elfanalyzer.FirstMprotectRisk(result.SyscallAnalysis.ArgEvalResults)
	}
	return s
}

// firstNetworkSyscall returns the name of the first network syscall found in
// data using table for classification. Returns "" if none found or inputs are nil.
func firstNetworkSyscall(table syscallTableInterface, data *fileanalysis.SyscallAnalysisData) string {
	if table == nil || data == nil {
		return ""
	}
	for _, s := range data.DetectedSyscalls {
		if s.Number >= 0 && table.IsNetworkSyscall(s.Number) {
			return s.Name
		}
	}
	return ""
}

// isVDSOEntry reports whether soname refers to a Linux virtual DSO that has no
// real file on disk and should be excluded from library analysis.
func isVDSOEntry(soname string) bool {
	switch soname {
	case "linux-vdso.so.1", "linux-gate.so.1", "linux-vdso64.so.1":
		return true
	default:
		return false
	}
}

func buildAnalysisOutputFromSymbolData(data *fileanalysis.SymbolAnalysisData, cmdPath string) binaryanalyzer.AnalysisOutput {
	output := binaryanalyzer.AnalysisOutput{
		DetectedSymbols:    convertNetworkSymbolEntries(data.DetectedSymbols),
		DynamicLoadSymbols: convertNetworkSymbolEntries(data.DynamicLoadSymbols),
	}

	// Check if any detected symbol has a network category (socket, dns, tls, http).
	// non-network symbols do not trigger NetworkDetected.
	hasNetworkSymbol := slices.ContainsFunc(data.DetectedSymbols, binaryanalyzer.IsNetworkSymbolName)

	switch {
	case hasNetworkSymbol:
		output.Result = binaryanalyzer.NetworkDetected
	case len(data.KnownNetworkLibDeps) > 0:
		output.Result = binaryanalyzer.NetworkDetected
		slog.Info( //nolint:gosec // G706: cmdPath is a configured command path from TOML, not arbitrary user input
			"treating binary as network-capable based on known network library dependencies",
			"path", cmdPath,
			"known_network_lib_deps", data.KnownNetworkLibDeps,
		)
	default:
		output.Result = binaryanalyzer.NoNetworkSymbols
	}

	return output
}

// syscallAnalysisHasSVCSignal reports whether the given SyscallAnalysisResult
// contains evidence of unresolved svc #0x80 direct syscall usage (high risk).
// Returns true only when any DetectedSyscall has Number == -1 and at least one
// Occurrence with DeterminationMethod == "direct_svc_0x80".
// Resolved svc entries (Number != -1) are not treated as high risk here;
// their network classification is handled by syscallAnalysisHasNetworkSignal.
func syscallAnalysisHasSVCSignal(result *fileanalysis.SyscallAnalysisResult) bool {
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

// syscallAnalysisHasNetworkSignal reports whether the given SyscallAnalysisResult
// contains any detected syscall classified as a network syscall.
// This includes resolved svc entries (DeterminationMethod == "direct_svc_0x80" AND Number != -1)
// whose network classification is determined by the syscall table lookup.
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisResult, goos string) bool { //nolint:unparam // goos varies by platform (darwin vs linux)
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
