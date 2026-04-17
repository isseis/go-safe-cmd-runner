//go:build test && darwin

// Package machodylibtesting provides test helpers for building synthetic Mach-O
// binaries with LC_LOAD_DYLIB, LC_LOAD_WEAK_DYLIB, and LC_RPATH load commands.
package machodylibtesting

import (
	"debug/macho"
	"encoding/binary"
	"runtime"
)

// Mach-O file format constants used when building synthetic binaries for tests.
const (
	machOMagic64      = uint32(0xFEEDFACF) // MH_MAGIC_64
	fatMagic          = uint32(0xCAFEBABE) // FAT_MAGIC
	lcLoadDylib       = uint32(0x0C)       // LC_LOAD_DYLIB
	lcLoadWeakDylib   = uint32(0x80000018) // LC_LOAD_WEAK_DYLIB
	lcRpath           = uint32(0x8000001C) // LC_RPATH
	machOHeaderSize64 = 32                 // sizeof(mach_header_64)
	fatArchSize       = 20                 // sizeof(fat_arch)
	fatFixedHdrSize   = 8                  // fat_header: magic(4) + nfat_arch(4)
	dylibCmdHdrSize   = 24                 // sizeof(dylib_command) header
	rpathCmdHdrSize   = 12                 // sizeof(rpath_command) header
	mhExecute         = uint32(2)          // MH_EXECUTE filetype
	align4Mask        = 3                  // used in (n+3)&^3 alignment
)

// NativeCPU returns the macho.Cpu type matching the current build architecture.
func NativeCPU() macho.Cpu {
	switch runtime.GOARCH {
	case "arm64":
		return macho.CpuArm64
	default:
		return macho.CpuAmd64
	}
}

// NonNativeCPU returns a macho.Cpu type that does NOT match the current build
// architecture, used to build Fat binary fixtures for slice-selection tests.
func NonNativeCPU() macho.Cpu {
	switch runtime.GOARCH {
	case "arm64":
		return macho.CpuAmd64
	default:
		return macho.CpuArm64
	}
}

// BuildMachOWithDeps builds a 64-bit little-endian Mach-O binary for cpuType
// with the specified strong deps (LC_LOAD_DYLIB), weak deps (LC_LOAD_WEAK_DYLIB),
// and rpath entries (LC_RPATH).
func BuildMachOWithDeps(cpuType macho.Cpu, strongDeps, weakDeps, rpaths []string) []byte {
	var cmds [][]byte
	for _, dep := range strongDeps {
		cmds = append(cmds, buildDylibLoadCmd(dep, false))
	}
	for _, dep := range weakDeps {
		cmds = append(cmds, buildDylibLoadCmd(dep, true))
	}
	for _, rp := range rpaths {
		cmds = append(cmds, buildRpathLoadCmd(rp))
	}

	sizeofcmds := 0
	for _, c := range cmds {
		sizeofcmds += len(c)
	}

	hdr := make([]byte, machOHeaderSize64)
	binary.LittleEndian.PutUint32(hdr[0:4], machOMagic64)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(cpuType)) //nolint:gosec
	binary.LittleEndian.PutUint32(hdr[8:12], 0)              // cpusubtype
	binary.LittleEndian.PutUint32(hdr[12:16], mhExecute)
	binary.LittleEndian.PutUint32(hdr[16:20], uint32(len(cmds)))  //nolint:gosec
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(sizeofcmds)) //nolint:gosec
	binary.LittleEndian.PutUint32(hdr[24:28], 0)                  // flags
	binary.LittleEndian.PutUint32(hdr[28:32], 0)                  // reserved

	out := make([]byte, machOHeaderSize64+sizeofcmds)
	copy(out, hdr)
	off := machOHeaderSize64
	for _, c := range cmds {
		copy(out[off:], c)
		off += len(c)
	}
	return out
}

// BuildFatBinaryFromSlices builds a Fat Mach-O binary whose slices are provided
// as pre-built byte slices. Each slice is placed sequentially after the fat header.
func BuildFatBinaryFromSlices(cpuTypes []macho.Cpu, slices [][]byte) []byte {
	nArch := len(cpuTypes)
	fatHdrSize := fatFixedHdrSize + fatArchSize*nArch

	totalSize := fatHdrSize
	for _, s := range slices {
		totalSize += len(s)
	}

	buf := make([]byte, totalSize)
	binary.BigEndian.PutUint32(buf[0:4], fatMagic)
	binary.BigEndian.PutUint32(buf[4:8], uint32(nArch)) //nolint:gosec

	offset := fatHdrSize
	for i, cpu := range cpuTypes {
		archOff := fatFixedHdrSize + i*fatArchSize
		binary.BigEndian.PutUint32(buf[archOff:archOff+4], uint32(cpu))                //nolint:gosec
		binary.BigEndian.PutUint32(buf[archOff+4:archOff+8], 0)                        // cpusubtype
		binary.BigEndian.PutUint32(buf[archOff+8:archOff+12], uint32(offset))          //nolint:gosec
		binary.BigEndian.PutUint32(buf[archOff+12:archOff+16], uint32(len(slices[i]))) //nolint:gosec
		binary.BigEndian.PutUint32(buf[archOff+16:archOff+20], 0)                      // align
		copy(buf[offset:], slices[i])
		offset += len(slices[i])
	}
	return buf
}

// buildDylibLoadCmd builds a raw LC_LOAD_DYLIB or LC_LOAD_WEAK_DYLIB load command.
// The dylib_command header is 24 bytes; the name string follows immediately after.
func buildDylibLoadCmd(name string, isWeak bool) []byte {
	totalSize := alignTo4(dylibCmdHdrSize + len(name) + 1)
	buf := make([]byte, totalSize)

	cmd := lcLoadDylib
	if isWeak {
		cmd = lcLoadWeakDylib
	}
	binary.LittleEndian.PutUint32(buf[0:4], cmd)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(totalSize)) //nolint:gosec
	binary.LittleEndian.PutUint32(buf[8:12], dylibCmdHdrSize)  // name_offset
	// timestamp, current_version, compat_version at [12:24] default to 0
	copy(buf[dylibCmdHdrSize:], name)
	// null terminator is already zero from make
	return buf
}

// buildRpathLoadCmd builds a raw LC_RPATH load command.
// The rpath_command header is 12 bytes; the path string follows immediately after.
func buildRpathLoadCmd(rp string) []byte {
	totalSize := alignTo4(rpathCmdHdrSize + len(rp) + 1)
	buf := make([]byte, totalSize)

	binary.LittleEndian.PutUint32(buf[0:4], lcRpath)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(totalSize)) //nolint:gosec
	binary.LittleEndian.PutUint32(buf[8:12], rpathCmdHdrSize)  // path_offset
	copy(buf[rpathCmdHdrSize:], rp)
	return buf
}

// alignTo4 rounds n up to the nearest multiple of 4 for Mach-O load command size alignment.
func alignTo4(n int) int {
	return (n + align4Mask) &^ align4Mask
}
