package machoanalyzer

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
)

// svcInstruction is the encoding of "svc #0x80" for arm64 (little-endian).
// ARM64 encoding: 0xD4001001 → bytes [0x01, 0x10, 0x00, 0xD4]
var svcInstruction = []byte{0x01, 0x10, 0x00, 0xD4}

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
	if f.Cpu != macho.CpuArm64 {
		return false, nil
	}

	section := f.Section("__text")
	if section == nil || section.Seg != "__TEXT" {
		return false, nil
	}

	data, err := section.Data()
	if err != nil {
		return false, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
	}

	target := binary.LittleEndian.Uint32(svcInstruction)
	for i := 0; i+4 <= len(data); i += 4 {
		if binary.LittleEndian.Uint32(data[i:i+4]) == target {
			return true, nil
		}
	}
	return false, nil
}
