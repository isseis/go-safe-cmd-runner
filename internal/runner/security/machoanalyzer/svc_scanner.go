package machoanalyzer

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// svcInstruction is the encoding of "svc #0x80" for arm64 (little-endian).
// ARM64 encoding: 0xD4001001 → bytes [0x01, 0x10, 0x00, 0xD4]
var svcInstruction = []byte{0x01, 0x10, 0x00, 0xD4}

// collectSVCAddresses scans the __TEXT,__text section of a Mach-O file
// for svc #0x80 instructions and returns their virtual addresses.
//
// Only processes arm64 binaries (CpuArm64); returns nil, nil for other
// architectures or when no __TEXT,__text section is present.
//
// Returns nil, fmt.Errorf("...") when the section exists but cannot be read —
// the caller must treat this as a scan failure.
func collectSVCAddresses(f *macho.File) ([]uint64, error) {
	if f.Cpu != macho.CpuArm64 {
		return nil, nil
	}

	section := f.Section("__text")
	if section == nil || section.Seg != "__TEXT" {
		return nil, nil
	}

	r := section.Open()
	target := binary.LittleEndian.Uint32(svcInstruction)
	buf := make([]byte, len(svcInstruction))
	var addrs []uint64
	for offset := uint64(0); ; offset += uint64(len(svcInstruction)) {
		if _, err := io.ReadFull(r, buf); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
		}
		if binary.LittleEndian.Uint32(buf) == target {
			addrs = append(addrs, section.Addr+offset)
		}
	}
	if len(addrs) == 0 {
		return nil, nil
	}
	return addrs, nil
}

// containsSVCInstruction scans the __TEXT,__text section of a Mach-O file
// for the svc #0x80 instruction (0xD4001001 in little-endian).
//
// Uses 4-byte aligned scan, exploiting arm64 fixed-width instruction encoding.
// Only processes arm64 binaries (CpuArm64); returns (false, nil) for other
// architectures or when no __TEXT,__text section is present.
//
// Returns (false, err) when the section exists but cannot be read — the caller
// must treat this as a scan failure and propagate AnalysisError rather than
// silently returning NoNetworkSymbols.
//
// Background: Regular macOS binaries (both Go and C) call libSystem.dylib for
// system calls and never contain svc #0x80 directly. Its presence indicates
// a direct kernel call, bypassing libSystem.dylib.
func containsSVCInstruction(f *macho.File) (bool, error) {
	addrs, err := collectSVCAddresses(f)
	return len(addrs) > 0, err
}

// isMachOMagicAll reports whether the first 4 bytes match any recognized
// Mach-O or Fat binary magic number, covering 32-bit, 64-bit, and Fat
// binaries in both native and byte-swapped byte orders.
func isMachOMagicAll(b []byte) bool {
	if len(b) < magicNumberSize {
		return false
	}
	m := binary.LittleEndian.Uint32(b[:magicNumberSize])
	switch m {
	case machoMagic64, machoCigam64, fatMagic, fatCigam,
		machoMagic32, machoCigam32:
		return true
	}
	return false
}

// CollectSVCAddressesFromFile opens the file at filePath using fs, checks
// whether it is a Mach-O binary, and collects virtual addresses of svc #0x80
// instructions from arm64 slices.
//
// For Fat binaries only arm64 slices are scanned; other architecture slices
// are skipped.  Returns nil, nil for non-Mach-O files or when no svc #0x80
// is detected.  Returns an error on I/O failures or Mach-O parse failures.
func CollectSVCAddressesFromFile(filePath string, fs safefileio.FileSystem) ([]uint64, error) {
	f, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Read the first 4 bytes to check the Mach-O magic number.
	// macho.NewFile and macho.NewFatFile accept io.ReaderAt, so they read
	// from absolute offsets and are not affected by the current read position.
	magic := make([]byte, magicNumberSize)
	if _, err := io.ReadFull(f, magic); err != nil {
		// File is shorter than 4 bytes — cannot be Mach-O.
		return nil, nil
	}

	if !isMachOMagicAll(magic) {
		return nil, nil
	}

	m := binary.LittleEndian.Uint32(magic)
	if m == fatMagic || m == fatCigam {
		fat, err := macho.NewFatFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Fat binary: %w", err)
		}
		defer func() {
			if closeErr := fat.Close(); closeErr != nil {
				slog.Warn("error closing Fat Mach-O file during svc scan", slog.Any("error", closeErr))
			}
		}()

		var all []uint64
		for i := range fat.Arches {
			addrs, err := collectSVCAddresses(fat.Arches[i].File)
			if err != nil {
				return nil, err
			}
			all = append(all, addrs...)
		}
		if len(all) == 0 {
			return nil, nil
		}
		return all, nil
	}

	// Single-architecture Mach-O.
	machOFile, err := macho.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Mach-O: %w", err)
	}
	defer func() {
		if closeErr := machOFile.Close(); closeErr != nil {
			slog.Warn("error closing Mach-O file during svc scan", slog.Any("error", closeErr))
		}
	}()

	return collectSVCAddresses(machOFile)
}
