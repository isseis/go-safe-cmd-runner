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
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// ErrDirectSyscall indicates svc #0x80 was found, indicating a direct syscall.
var ErrDirectSyscall = errors.New("direct syscall instruction detected (svc #0x80)")

// ErrNoArm64Slice indicates a Fat binary has no arm64 slice.
var ErrNoArm64Slice = errors.New("no arm64 slice found in Fat binary")

// ErrNotRegularFile indicates the target is not a regular file.
var ErrNotRegularFile = errors.New("not a regular file")

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

// selectMachOFromFat selects the arm64 slice from a Fat binary.
// Returns an error if no arm64 slice is found.
func selectMachOFromFat(fat *macho.FatFile) (*macho.File, error) {
	for _, arch := range fat.Arches {
		if arch.Cpu == macho.CpuArm64 {
			return arch.File, nil
		}
	}
	return nil, ErrNoArm64Slice
}

// parseMachO parses the file as a Mach-O or Fat binary.
// For Fat binaries, selects the arm64 slice.
//
// Returns (*macho.File, io.Closer, nil) on success.
// The caller must call closer.Close() when done with the *macho.File.
// For Fat binaries, closer is the *macho.FatFile; for single Mach-O, closer is the *macho.File itself.
//
// Returns (nil, nil, &AnalysisOutput) when the binary cannot be parsed or arm64 slice is absent.
func (a *StandardMachOAnalyzer) parseMachO(file safefileio.File, magic []byte) (*macho.File, io.Closer, *binaryanalyzer.AnalysisOutput) {
	m := binary.LittleEndian.Uint32(magic)
	if m == fatMagic || m == fatCigam {
		// Fat binary: extract arm64 slice
		fat, err := macho.NewFatFile(file)
		if err != nil {
			output := binaryanalyzer.AnalysisOutput{
				Result: binaryanalyzer.AnalysisError,
				Error:  fmt.Errorf("failed to parse Fat binary: %w", err),
			}
			return nil, nil, &output
		}

		slice, err := selectMachOFromFat(fat)
		if err != nil {
			// No arm64 slice: do not close fat here; caller's defer file.Close() handles cleanup
			output := binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NotSupportedBinary}
			return nil, nil, &output
		}
		// Return fat as closer; slice is valid only while fat is alive
		return slice, fat, nil
	}

	// Single Mach-O
	machOFile, err := macho.NewFile(file)
	if err != nil {
		output := binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to parse Mach-O: %w", err),
		}
		return nil, nil, &output
	}
	return machOFile, machOFile, nil
}

// AnalyzeNetworkSymbols implements binaryanalyzer.BinaryAnalyzer.
//
// Returns:
//   - NetworkDetected: Binary imports network-related symbols
//   - NoNetworkSymbols: No network symbols and no svc #0x80 detected
//   - NotSupportedBinary: File is not in Mach-O format, or is a
//     x86_64-only Fat binary (arm64 slice not found)
//   - AnalysisError: Parse error, or svc #0x80 detected (high risk)
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

	// Detect Fat binary and extract arm64 slice
	machOFile, closer, output := a.parseMachO(file, magic)
	if output != nil {
		return *output
	}
	defer func() {
		if closeErr := closer.Close(); closeErr != nil {
			slog.Warn("error closing Mach-O file", slog.Any("error", closeErr))
		}
	}()

	// Step 5: retrieve imported symbols, normalize, and match against network symbol table
	symbols, err := machOFile.ImportedSymbols()
	if err != nil {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("failed to get imported symbols: %w", err),
		}
	}

	var detected []binaryanalyzer.DetectedSymbol
	for _, sym := range symbols {
		normalized := normalizeSymbolName(sym)
		if cat, found := a.networkSymbols[normalized]; found {
			detected = append(detected, binaryanalyzer.DetectedSymbol{
				Name:     normalized,
				Category: string(cat),
			})
		}
	}

	// Step 6: match found → return NetworkDetected
	if len(detected) > 0 {
		return binaryanalyzer.AnalysisOutput{
			Result:          binaryanalyzer.NetworkDetected,
			DetectedSymbols: detected,
		}
	}

	// Step 7: no match → scan __TEXT,__text section for svc #0x80
	if containsSVCInstruction(machOFile) {
		return binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.AnalysisError,
			Error:  fmt.Errorf("binary analysis: %w", ErrDirectSyscall),
		}
	}

	return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}
}
