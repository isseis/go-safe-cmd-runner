package machoanalyzer

import (
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// determinationMethodDirectSVC0x80 is the method string used for unresolved
// svc #0x80 entries detected by Pass 1. It preserves the high-risk signal that
// a direct kernel call was found, even when the syscall number could not be resolved.
const determinationMethodDirectSVC0x80 = common.DeterminationMethodDirectSVC0x80

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

// noopSyscallTable is a SyscallNumberTable that returns empty results.
// Used when no table is provided to ScanSyscallInfos.
type noopSyscallTable struct{}

func (noopSyscallTable) GetSyscallName(_ int) string { return "" }
func (noopSyscallTable) IsNetworkSyscall(_ int) bool { return false }

// analyzeArm64Slice performs Pass 1 and Pass 2 analysis on a single arm64 Mach-O slice.
//
// Pass 1: scans svc #0x80 addresses, excludes known Go stub bodies, and resolves
// the X16 syscall number via backward scan.
// Pass 2: scans BL calls to known Go syscall wrapper addresses and resolves the
// trap argument from the preceding write to the stack slot [SP, #8] (old stack ABI).
//
// Returns:
//   - directSVCInfos: Pass 1 results (actual svc #0x80 instructions in user code).
//   - wrapperCallInfos: Pass 2 results (BL calls to Go syscall wrappers from user code).
//
// Returns nil, nil, nil for non-arm64 slices or when no __TEXT,__text section exists.
func analyzeArm64Slice(f *macho.File, table SyscallNumberTable) (directSVCInfos, wrapperCallInfos []common.SyscallInfo, err error) {
	if f.Cpu != macho.CpuArm64 {
		return nil, nil, nil
	}

	section := f.Section("__text")
	if section == nil || section.Seg != "__TEXT" {
		return nil, nil, nil
	}

	svcAddrs, err := collectSVCAddresses(f)
	if err != nil {
		return nil, nil, err
	}

	// Read the full __text section for backward scanning.
	r := io.NewSectionReader(section, 0, int64(section.Size)) //nolint:gosec // G115: section.Size is a Mach-O field
	code, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
	}
	textBase := section.Addr

	// Parse pclntab to obtain Go function address ranges.
	// ErrNoPclntab is expected for non-Go or stripped binaries; continue without exclusion.
	// ErrUnsupportedPclntabVersion means a Go binary with an older pclntab format; continue
	// without exclusion/resolution rather than failing the entire analysis.
	// Other errors (I/O failures, corrupt data) are propagated to the caller.
	funcs, pclntabErr := ParseMachoPclntab(f)
	if pclntabErr != nil {
		if !errors.Is(pclntabErr, ErrNoPclntab) && !errors.Is(pclntabErr, ErrUnsupportedPclntabVersion) && !errors.Is(pclntabErr, ErrInvalidPclntab) {
			return nil, nil, fmt.Errorf("failed to parse pclntab: %w", pclntabErr)
		}
		funcs = nil
	}

	stubRanges := buildStubRanges(funcs)

	// Pass 1: resolve X16 via backward scan for each svc #0x80 outside stub bodies.
	if len(svcAddrs) > 0 {
		pass1 := scanSVCWithX16(svcAddrs, code, textBase, stubRanges, table)
		for _, info := range pass1 {
			if info.Number == -1 {
				// Unresolved svc — preserve the "direct svc detected" signal.
				info.Occurrences[0].DeterminationMethod = determinationMethodDirectSVC0x80
				info.Occurrences[0].Source = determinationMethodDirectSVC0x80
			}
			directSVCInfos = append(directSVCInfos, info)
		}
	}

	// Pass 2: resolve Go wrapper call sites.
	wrapperAddrs := buildWrapperAddrs(funcs)
	wrapperCallInfos = scanGoWrapperCalls(code, textBase, wrapperAddrs, stubRanges, table)

	return directSVCInfos, wrapperCallInfos, nil
}

// ScanSyscallInfos opens the Mach-O file at filePath and performs Pass 1 and
// Pass 2 syscall analysis on all arm64 slices.
//
// Pass 1 results (directSVCInfos) contain entries from actual svc #0x80
// instructions found outside known Go stub bodies.  Entries with a resolved
// syscall number carry DeterminationMethod="immediate"; unresolved entries
// carry DeterminationMethod="direct_svc_0x80".
//
// Pass 2 results (wrapperCallInfos) contain entries from BL instructions that
// target known Go syscall wrapper functions.  Resolved entries carry
// DeterminationMethod="go_wrapper"; unresolved entries carry
// DeterminationMethod="unknown:indirect_setting".
//
// Both slices are nil for non-Mach-O files or when no relevant instructions
// are found.  table may be nil, in which case syscall names and network flags
// are left empty.
func ScanSyscallInfos(filePath string, fs safefileio.FileSystem, table SyscallNumberTable) (directSVCInfos, wrapperCallInfos []common.SyscallInfo, err error) {
	if table == nil {
		table = noopSyscallTable{}
	}

	f, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, magicNumberSize)
	if _, err := io.ReadFull(f, magic); err != nil {
		return nil, nil, nil
	}
	if !isMachOMagicAll(magic) {
		return nil, nil, nil
	}

	m := binary.LittleEndian.Uint32(magic)
	if m == fatMagic || m == fatCigam {
		fat, err := macho.NewFatFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse Fat binary: %w", err)
		}
		defer func() {
			if closeErr := fat.Close(); closeErr != nil {
				slog.Warn("error closing Fat Mach-O file during syscall scan", slog.Any("error", closeErr))
			}
		}()

		for i := range fat.Arches {
			d, w, sliceErr := analyzeArm64Slice(fat.Arches[i].File, table)
			if sliceErr != nil {
				return nil, nil, sliceErr
			}
			directSVCInfos = append(directSVCInfos, d...)
			wrapperCallInfos = append(wrapperCallInfos, w...)
		}
		return directSVCInfos, wrapperCallInfos, nil
	}

	machOFile, err := macho.NewFile(f)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse Mach-O: %w", err)
	}
	defer func() {
		if closeErr := machOFile.Close(); closeErr != nil {
			slog.Warn("error closing Mach-O file during syscall scan", slog.Any("error", closeErr))
		}
	}()

	return analyzeArm64Slice(machOFile, table)
}

// ScanSVCAddrs opens the file at filePath using fs, checks whether it is a
// Mach-O binary, and returns the virtual addresses of svc #0x80 instructions
// found in arm64 slices.
//
// For Fat binaries only arm64 slices are scanned; other architecture slices
// are skipped.  Returns nil, nil for non-Mach-O files or when no svc #0x80
// is detected.  Returns an error on I/O failures or Mach-O parse failures.
func ScanSVCAddrs(filePath string, fs safefileio.FileSystem) ([]uint64, error) {
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
