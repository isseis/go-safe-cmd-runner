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
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

// errRelativeCmdPath is returned when Classify receives a non-absolute path.
var errRelativeCmdPath = errors.New("Classify: cmdPath must be an absolute path")

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

// AnalysisEnabled reports whether record-backed binary analysis is configured.
// When false, binary identity cannot be confirmed and the evaluator's identity
// gate denies execution (analysis-disabled is fail-closed, not fail-open).
func (a *NetworkAnalyzer) AnalysisEnabled() bool {
	return a.deps.RecordStore != nil
}

// Classify performs binary-signal analysis only and returns the classification
// together with the machine-readable reason codes for the signals found. Profile
// matching and argument-based network detection are no longer part of this
// function; the evaluator owns those dimensions.
//
// The result is one of four classes (risktypes.BinaryAnalysisResult.Class):
//   - Clean: analysis succeeded and found no dangerous or network signal -> Low
//   - Network: only network signals were found -> Medium
//   - HighRisk: dynamic-load/exec/svc/mprotect signals were found -> High
//   - Uncertain: the binary's signals could not be obtained (missing record,
//     schema mismatch, hash mismatch, unverified identity, analysis disabled) ->
//     the evaluator blocks execution (fail-closed). Data-unavailable cases are
//     never collapsed to Clean.
//
// cmdPath must be an absolute path; contentHash is a pre-computed "algo:hex" hash
// used to confirm the loaded record matches the binary on disk. A genuine,
// unclassifiable record-load I/O failure is returned as a non-nil error so the
// caller aborts with an error rather than a deny.
func (a *NetworkAnalyzer) Classify(cmdPath string, contentHash string) (risktypes.BinaryAnalysisResult, error) {
	if !filepath.IsAbs(cmdPath) {
		return risktypes.BinaryAnalysisResult{}, fmt.Errorf("%w: %q", errRelativeCmdPath, cmdPath)
	}

	if a.deps.RecordStore == nil {
		// Analysis disabled. The evaluator's identity gate normally blocks this
		// earlier; classify defensively as uncertain so a missing gate never
		// falls through to Clean.
		return uncertainResult(risktypes.ReasonAnalysisDisabled), nil
	}

	if contentHash == "" {
		slog.Warn("Binary has no pre-verified hash; treating as uncertain", "path", cmdPath)
		return uncertainResult(risktypes.ReasonUncertainUnverifiedIdentity), nil
	}

	record, loadErr := a.deps.RecordStore.LoadRecord(cmdPath)
	if loadErr != nil {
		if errors.Is(loadErr, fileanalysis.ErrRecordNotFound) {
			// contentHash is non-empty here (checked above), meaning the binary
			// was hash-verified but has no analysis record. Treat as uncertain
			// rather than fail-open: a missing analysis record cannot be
			// distinguished from a deleted or never-generated one.
			slog.Warn("Analysis record not found for hash-verified binary; treating as uncertain",
				"path", cmdPath)
			return uncertainResult(risktypes.ReasonUncertainMissingRecord), nil
		}
		if schemaMismatch, ok := errors.AsType[*fileanalysis.SchemaVersionMismatchError](loadErr); ok {
			slog.Warn("Record has outdated schema; treating as uncertain",
				"path", cmdPath,
				"expected_schema", schemaMismatch.Expected,
				"actual_schema", schemaMismatch.Actual)
			return uncertainResult(risktypes.ReasonUncertainSchemaMismatch), nil
		}
		return risktypes.BinaryAnalysisResult{}, fmt.Errorf("failed to load record for %q: %w", cmdPath, loadErr)
	}

	if record.ContentHash != contentHash {
		slog.Warn("Record content hash mismatch; treating as uncertain",
			"path", cmdPath,
			"expected", contentHash,
			"actual", record.ContentHash)
		return uncertainResult(risktypes.ReasonUncertainHashMismatch), nil
	}

	return a.classifyRecordSignals(record, cmdPath), nil
}

// uncertainResult builds an Uncertain classification carrying the given reason.
func uncertainResult(code risktypes.ReasonCode) risktypes.BinaryAnalysisResult {
	return risktypes.BinaryAnalysisResult{
		Class:       risktypes.BinaryAnalysisUncertain,
		ReasonCodes: []risktypes.ReasonCode{code},
	}
}

// classifyRecordSignals maps the signals in a verified record to one of the
// Clean/Network/HighRisk classes (Uncertain is decided before this point) and
// collects a distinct reason code per signal found.
func (a *NetworkAnalyzer) classifyRecordSignals(record *fileanalysis.Record, path string) risktypes.BinaryAnalysisResult {
	var codes []risktypes.ReasonCode
	isNetwork := false
	isHighRisk := false

	if record.SymbolAnalysis != nil {
		output := buildAnalysisOutputFromSymbolData(record.SymbolAnalysis)
		symIsNet, symHigh := handleAnalysisOutput(output, path)
		if symHigh {
			isHighRisk = true
			codes = append(codes, risktypes.ReasonBinaryAnalysisDynamicLoad)
		}
		if symIsNet {
			isNetwork = true
		}
	}

	if syscallAnalysisHasSVCSignal(record.SyscallAnalysis) {
		slog.Warn("SyscallAnalysis indicates svc #0x80; treating as high risk", "path", path)
		isHighRisk = true
		codes = append(codes, risktypes.ReasonBinaryAnalysisSVC)
	}
	if syscallAnalysisHasNetworkSignal(record.SyscallAnalysis, a.goos) {
		slog.Info("SyscallAnalysis indicates network syscall", "path", path)
		isNetwork = true
	}
	if syscallAnalysisHasExecSignal(record.SyscallAnalysis, a.goos) {
		slog.Warn("SyscallAnalysis indicates exec syscall; treating as high risk", "path", path)
		isHighRisk = true
		codes = append(codes, risktypes.ReasonBinaryAnalysisExec)
	}
	if syscallAnalysisHasMprotectExecSignal(record.SyscallAnalysis) {
		slog.Warn("SyscallAnalysis indicates mprotect-family PROT_EXEC; treating as high risk", "path", path)
		isHighRisk = true
		codes = append(codes, risktypes.ReasonBinaryAnalysisMprotectExec)
	}

	switch {
	case isHighRisk:
		// Retain the network signal alongside the high-risk codes so audit and
		// callers do not lose the fact that the binary also performs network I/O.
		if isNetwork {
			codes = append(codes, risktypes.ReasonBinaryAnalysisNetwork)
		}
		return risktypes.BinaryAnalysisResult{Class: risktypes.BinaryAnalysisHighRisk, ReasonCodes: codes}
	case isNetwork:
		return risktypes.BinaryAnalysisResult{
			Class:       risktypes.BinaryAnalysisNetwork,
			ReasonCodes: []risktypes.ReasonCode{risktypes.ReasonBinaryAnalysisNetwork},
		}
	default:
		return risktypes.BinaryAnalysisResult{Class: risktypes.BinaryAnalysisClean}
	}
}

// hasNetworkArguments checks if the arguments contain network indicators.
func hasNetworkArguments(args []string) bool {
	allArgs := strings.Join(args, " ")
	return strings.Contains(allArgs, "://") || // URLs
		containsSSHStyleAddress(args) // SSH-style user@host:path addresses
}

func buildAnalysisOutputFromSymbolData(data *fileanalysis.SymbolAnalysisData) binaryanalyzer.AnalysisOutput {
	output := binaryanalyzer.AnalysisOutput{
		DetectedSymbols:    convertNetworkSymbolEntries(data.DetectedSymbols),
		DynamicLoadSymbols: convertNetworkSymbolEntries(data.DynamicLoadSymbols),
	}

	// Check if any detected symbol has a network category (socket or dns).
	// non-network symbols do not trigger NetworkDetected.
	hasNetworkSymbol := slices.ContainsFunc(data.DetectedSymbols, func(s fileanalysis.DetectedSymbol) bool {
		return binaryanalyzer.IsNetworkSymbolName(s.Name)
	})

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

// syscallAnalysisHasMprotectExecSignal reports whether SyscallAnalysisData
// contains mprotect-family ArgEvalResults with PROT_EXEC confirmed or unknown.
// exec_unknown is treated as high risk because the static analyzer could not
// determine the prot argument, so PROT_EXEC cannot be ruled out (fail-closed).
func syscallAnalysisHasMprotectExecSignal(result *fileanalysis.SyscallAnalysisData) bool {
	if result == nil {
		return false
	}
	if len(result.ArgEvalResults) == 0 {
		return false
	}

	for _, r := range result.ArgEvalResults {
		if slices.Contains(elfanalyzer.MprotectFamilyNames, r.SyscallName) &&
			r.Status != common.SyscallArgEvalExecNotSet {
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
		// Unknown result: treat as high risk (fail-closed) to match the
		// AnalysisError / schema-mismatch / missing-record behavior.
		slog.Warn("Binary analysis returned unknown result", //nolint:gosec // G706: cmdPath is a configured command path from TOML, not arbitrary user input
			"path", cmdPath,
			"result", output.Result)
		return true, true
	}
}

// convertNetworkSymbolEntries converts fileanalysis.DetectedSymbol slice to binaryanalyzer.DetectedSymbol slice.
// Category is derived from the symbol name for runner-internal logging and filtering.
//
// NOTE: This is the inverse of convertDetectedSymbols in
// internal/filevalidator/validator.go.
func convertNetworkSymbolEntries(entries []fileanalysis.DetectedSymbol) []binaryanalyzer.DetectedSymbol {
	if len(entries) == 0 {
		return nil
	}
	syms := make([]binaryanalyzer.DetectedSymbol, len(entries))
	for i, e := range entries {
		cat, found := binaryanalyzer.IsNetworkSymbol(e.Name)
		if !found {
			if binaryanalyzer.IsDynamicLoadSymbol(e.Name) {
				cat = binaryanalyzer.CategoryDynamicLoad
			} else {
				cat = binaryanalyzer.CategorySyscallWrapper
			}
		}
		syms[i] = binaryanalyzer.DetectedSymbol{Name: e.Name, Category: string(cat)}
	}
	return syms
}
