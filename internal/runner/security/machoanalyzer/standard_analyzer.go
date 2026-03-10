package machoanalyzer

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
)

// ErrDirectSyscall indicates svc #0x80 was found, indicating a direct syscall.
var ErrDirectSyscall = errors.New("direct syscall instruction detected (svc #0x80)")

// ErrNotRegularFile indicates the target is not a regular file.
var ErrNotRegularFile = errors.New("not a regular file")

// ErrFileTooLarge indicates the file exceeds the maximum size for analysis.
var ErrFileTooLarge = errors.New("file too large")

// maxFileSize is the maximum file size for Mach-O analysis (1 GB).
const maxFileSize = 1 << 30

// magicNumberSize is the number of bytes in a Mach-O magic number.
const magicNumberSize = 4

// Mach-O magic numbers (see <mach-o/loader.h>)
const (
	machoMagic64 = 0xFEEDFACF // 64-bit Mach-O (native endian)
	machoCigam64 = 0xCFFAEDFE // 64-bit Mach-O (byte-swapped)
	fatMagic     = 0xCAFEBABE // Fat binary
	fatCigam     = 0xBEBAFECA // Fat binary (byte-swapped)
)

// isMachOMagic returns true if the first 4 bytes match any Mach-O or Fat binary magic.
func isMachOMagic(b []byte) bool {
	if len(b) < magicNumberSize {
		return false
	}
	magic := binary.LittleEndian.Uint32(b[:magicNumberSize])
	switch magic {
	case machoMagic64, machoCigam64, fatMagic, fatCigam:
		return true
	}
	return false
}

// analyzeSlice performs symbol and svc #0x80 analysis on a single *macho.File.
// Returns the AnalysisOutput for that slice.
func (a *StandardMachOAnalyzer) analyzeSlice(f *macho.File) binaryanalyzer.AnalysisOutput {
	symbols, err := f.ImportedSymbols()
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to get imported symbols: %w", err),
		}
	}

	var detected []binaryanalyzer.DetectedSymbol
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol
	for _, sym := range symbols {
		normalized := normalizeSymbolName(sym)
		if cat, found := a.networkSymbols[normalized]; found {
			detected = append(detected, binaryanalyzer.DetectedSymbol{
				Name:     normalized,
				Category: string(cat),
			})
		}
		if binaryanalyzer.IsDynamicLoadSymbol(normalized) {
			dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
				Name:     normalized,
				Category: "dynamic_load",
			})
		}
	}

	if len(detected) > 0 {
		return binaryanalyzer.AnalysisOutput{
			Result:             binaryanalyzer.NetworkDetected,
			DetectedSymbols:    detected,
			DynamicLoadSymbols: dynamicLoadSyms,
		}
	}

	hasSVC, err := containsSVCInstruction(f)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("svc scan failed: %w", err),
		}
	}
	if hasSVC {
		return binaryanalyzer.AnalysisOutput{
			Result:             binaryanalyzer.AnalysisError,
			DynamicLoadSymbols: dynamicLoadSyms,
			Error:              fmt.Errorf("binary analysis: %w", ErrDirectSyscall),
		}
	}

	return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols, DynamicLoadSymbols: dynamicLoadSyms}
}

// analyzeAllFatSlices analyzes every slice in a Fat binary and returns the most
// severe result found across all slices. This prevents an attacker from hiding a
// malicious slice behind a benign one (e.g., a clean arm64 slice concealing a
// network-capable x86_64 slice).
//
// Severity order (highest to lowest): NetworkDetected > AnalysisError > NoNetworkSymbols.
// NotSupportedBinary is returned only when no slice could be analyzed.
func (a *StandardMachOAnalyzer) analyzeAllFatSlices(fat *macho.FatFile) binaryanalyzer.AnalysisOutput {
	var worstError binaryanalyzer.AnalysisOutput
	analyzedAny := false
	var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

	for i := range fat.Arches {
		slice := fat.Arches[i].File
		result := a.analyzeSlice(slice)

		dynamicLoadSyms = append(dynamicLoadSyms, result.DynamicLoadSymbols...)

		switch result.Result {
		case binaryanalyzer.NetworkDetected:
			// Highest severity — return immediately (preserve DynamicLoadSymbols).
			result.DynamicLoadSymbols = dynamicLoadSyms
			return result
		case binaryanalyzer.AnalysisError:
			// Record but keep scanning; a later slice might be NetworkDetected.
			worstError = result
			analyzedAny = true
		case binaryanalyzer.NoNetworkSymbols:
			analyzedAny = true
		}
		// NotSupportedBinary or other: skip (don't count as analyzed)
	}

	if !analyzedAny {
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NotSupportedBinary}
	}

	if worstError.Result == binaryanalyzer.AnalysisError {
		worstError.DynamicLoadSymbols = dynamicLoadSyms
		return worstError
	}

	return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols, DynamicLoadSymbols: dynamicLoadSyms}
}

// AnalyzeNetworkSymbols implements binaryanalyzer.BinaryAnalyzer.
//
// For Fat binaries, all slices are analyzed. The binary is flagged if any slice
// contains network symbols or direct syscalls, preventing cross-architecture
// security bypasses.
//
// Returns:
//   - NetworkDetected: any slice imports network-related symbols
//   - NoNetworkSymbols: all slices are clean (no network symbols, no svc #0x80)
//   - NotSupportedBinary: not a Mach-O file, or Fat binary with no analyzable slices
//   - AnalysisError: parse error, or svc #0x80 detected in any slice (high risk)
func (a *StandardMachOAnalyzer) AnalyzeNetworkSymbols(path string, _ string) binaryanalyzer.AnalysisOutput {
	// Step 1: open file safely via safefileio
	file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to open file: %w", err),
		}
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Warn("error closing file during Mach-O analysis", slog.Any("error", closeErr))
		}
	}()

	// Step 2: verify regular file (reject directories, symlinks, etc.)
	fileInfo, err := file.Stat()
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to stat file: %w", err),
		}
	}
	if !fileInfo.Mode().IsRegular() {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NotSupportedBinary,
			Error:  fmt.Errorf("%w: %s", ErrNotRegularFile, fileInfo.Mode()),
		}
	}
	if fileInfo.Size() > maxFileSize {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("%w: %d bytes (max %d)", ErrFileTooLarge, fileInfo.Size(), maxFileSize),
		}
	}

	// Step 3: read first 4 bytes and check Mach-O / Fat magic
	magic := make([]byte, magicNumberSize)
	if _, err := io.ReadFull(file, magic); err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to read magic: %w", err),
		}
	}
	if !isMachOMagic(magic) {
		return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NotSupportedBinary}
	}

	// Step 4: seek back to start for macho.NewFile / macho.NewFatFile
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to seek: %w", err),
		}
	}

	// Step 5: dispatch on binary type
	m := binary.LittleEndian.Uint32(magic)
	if m == fatMagic || m == fatCigam {
		fat, err := macho.NewFatFile(file)
		if err != nil {
			return binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("failed to parse Fat binary: %w", err),
			}
		}
		defer func() {
			if closeErr := fat.Close(); closeErr != nil {
				slog.Warn("error closing Fat Mach-O file", slog.Any("error", closeErr))
			}
		}()
		return a.analyzeAllFatSlices(fat)
	}

	// Single-arch Mach-O
	machOFile, err := macho.NewFile(file)
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to parse Mach-O: %w", err),
		}
	}
	defer func() {
		if closeErr := machOFile.Close(); closeErr != nil {
			slog.Warn("error closing Mach-O file", slog.Any("error", closeErr))
		}
	}()

	return a.analyzeSlice(machOFile)
}
