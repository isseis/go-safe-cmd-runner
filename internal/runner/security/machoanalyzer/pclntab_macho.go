package machoanalyzer

import (
	"debug/gosym"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
)

// machoTextSegName is the Mach-O segment name for the executable text segment.
const machoTextSegName = "__TEXT"

// Errors returned by ParseMachoPclntab.
var (
	// ErrNoPclntab is returned when no __gopclntab section is found.
	// This typically means a stripped binary or a non-Go binary.
	// Callers must continue Pass 1 and Pass 2 without exclusion/resolution.
	ErrNoPclntab = errors.New("no __gopclntab section found")

	// ErrUnsupportedPclntabVersion is returned when the pclntab magic is not
	// 0xfffffff1 (Go 1.20+). Only Go 1.20+ (magic 0xfffffff1) is supported.
	ErrUnsupportedPclntabVersion = errors.New("unsupported pclntab version: only magic 0xfffffff1 (Go 1.20+) is supported")

	// ErrInvalidPclntab is returned when the pclntab data is too short to
	// contain a valid magic number.
	ErrInvalidPclntab = errors.New("invalid pclntab data")
)

// MachoPclntabFunc holds the address range of a function extracted from
// the Mach-O __gopclntab section.
type MachoPclntabFunc struct {
	Name  string
	Entry uint64
	End   uint64
}

// funcRange represents a contiguous address range [start, end).
// Used by both Pass 1 (stubRanges) and Pass 2 (wrapperRanges).
type funcRange struct {
	start uint64
	end   uint64
}

// isInsideRange reports whether addr falls within any range in ranges.
// ranges must be sorted by start for binary search (O(log n)).
func isInsideRange(addr uint64, ranges []funcRange) bool {
	// Binary search: find the last range with start <= addr.
	n := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].start > addr
	})
	// n is the first index with start > addr, so the candidate is n-1.
	if n == 0 {
		return false
	}
	r := ranges[n-1]
	return addr >= r.start && addr < r.end
}

// ParseMachoPclntab reads the __gopclntab section from a Mach-O file and
// returns a map from function name to address range.
//
// Returns ErrNoPclntab when no __gopclntab section exists (stripped binary or
// non-Go binary). Callers must continue Pass 1 and Pass 2 without exclusion/
// resolution in that case.
//
// Only pclntab magic 0xfffffff1 (Go 1.20+) is supported; other versions
// return ErrUnsupportedPclntabVersion.
//
// For CGO binaries, a constant address offset may exist between pclntab entries
// and actual virtual addresses because C runtime startup code is inserted at the
// beginning of __TEXT,__text. ParseMachoPclntab detects and corrects this offset.
func ParseMachoPclntab(f *macho.File) (map[string]MachoPclntabFunc, error) {
	functions, err := parseMachoPclntabFuncsRaw(f)
	if err != nil {
		return nil, err
	}

	// CGO binaries may have a constant address offset between pclntab entries
	// and actual virtual addresses. Detect and apply the correction.
	if offset := detectMachoPclntabOffset(f, functions); offset != 0 {
		for name, fn := range functions {
			functions[name] = MachoPclntabFunc{
				Name:  fn.Name,
				Entry: fn.Entry + uint64(offset), //nolint:gosec // G115: offset verified positive in detectMachoPclntabOffset
				End:   fn.End + uint64(offset),   //nolint:gosec // G115: offset verified positive in detectMachoPclntabOffset
			}
		}
	}

	return functions, nil
}

// parseMachoPclntabFuncsRaw reads __gopclntab and returns function entries as
// gosym reports them — without any CGO offset correction applied.
func parseMachoPclntabFuncsRaw(f *macho.File) (map[string]MachoPclntabFunc, error) {
	section := f.Section("__gopclntab")
	if section == nil {
		return nil, ErrNoPclntab
	}

	data, err := section.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read __gopclntab: %w", err)
	}

	if err := checkMachoPclntabVersion(data); err != nil {
		return nil, err
	}

	// The text start address is required by gosym.NewLineTable.
	var textStart uint64
	textSection := f.Section("__text")
	if textSection != nil && textSection.Seg == machoTextSegName {
		textStart = textSection.Addr
	}

	lineTable := gosym.NewLineTable(data, textStart)
	symTable, err := gosym.NewTable(nil, lineTable)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedPclntabVersion, err)
	}

	functions := make(map[string]MachoPclntabFunc, len(symTable.Funcs))
	for i := range symTable.Funcs {
		fn := &symTable.Funcs[i]
		functions[fn.Name] = MachoPclntabFunc{
			Name:  fn.Name,
			Entry: fn.Entry,
			End:   fn.End,
		}
	}
	return functions, nil
}

// checkMachoPclntabVersion verifies that the pclntab magic is supported.
// Only magic 0xfffffff1 (Go 1.20+) is supported.
func checkMachoPclntabVersion(data []byte) error {
	const (
		pclntabMagicSize = 4
		go120Magic       = uint32(0xfffffff1) // Go 1.20+ (CurrentPCLnTabMagic)
	)
	if len(data) < pclntabMagicSize {
		return ErrInvalidPclntab
	}
	// Mach-O arm64 is always little-endian.
	magic := binary.LittleEndian.Uint32(data[0:pclntabMagicSize])
	if magic != go120Magic {
		return fmt.Errorf("%w (got magic 0x%x)", ErrUnsupportedPclntabVersion, magic)
	}
	return nil
}

// detectMachoPclntabOffset returns the address correction needed for pclntab entries
// in CGO binaries. Uses BL instruction cross-referencing to detect the offset
// without requiring a symbol table (supports stripped binaries).
//
// Returns 0 if no correction is needed (non-CGO binaries) or if detection fails.
func detectMachoPclntabOffset(f *macho.File, pclntabFuncs map[string]MachoPclntabFunc) int64 {
	textSection := f.Section("__text")
	if textSection == nil || textSection.Seg != machoTextSegName {
		return 0
	}

	offset := detectMachoOffsetByBLTargets(f, pclntabFuncs)
	// A valid offset is strictly positive (C startup code always precedes Go text)
	// and does not exceed the __text section size.
	if offset <= 0 || uint64(offset) > textSection.Size { //nolint:gosec
		return 0
	}
	return offset
}

// detectMachoOffsetByBLTargets detects the pclntab address offset in CGO binaries
// by cross-referencing BL instruction targets with pclntab function entries.
// Algorithm mirrors elfanalyzer.detectOffsetByCallTargets.
func detectMachoOffsetByBLTargets(
	f *macho.File,
	pclntabFuncs map[string]MachoPclntabFunc,
) int64 {
	const (
		scanLimit = 256 * 1024 // scan first 256 KB of __text
		minVotes  = 3
	)

	textSection := f.Section("__text")
	if textSection == nil || textSection.Seg != machoTextSegName {
		return 0
	}

	sr := io.NewSectionReader(textSection, 0, int64(scanLimit)) //nolint:gosec
	data, err := io.ReadAll(sr)
	if err != nil {
		return 0
	}

	// Build sorted slice of pclntab entry addresses.
	sortedEntries := make([]uint64, 0, len(pclntabFuncs))
	for _, fn := range pclntabFuncs {
		if fn.Entry != 0 {
			sortedEntries = append(sortedEntries, fn.Entry)
		}
	}
	if len(sortedEntries) == 0 {
		return 0
	}
	slices.Sort(sortedEntries)

	diffCounts := make(map[int64]int)
	textAddr := textSection.Addr

	collectMachoArm64BLDiffs(data, textAddr, sortedEntries, diffCounts)

	// Find the most frequent difference value with a unique winner.
	var bestDiff int64
	bestCount := 0
	tied := false
	for diff, count := range diffCounts {
		if count > bestCount {
			bestCount = count
			bestDiff = diff
			tied = false
		} else if count == bestCount {
			tied = true
		}
	}

	if bestCount < minVotes || tied {
		return 0
	}
	return bestDiff
}

// collectMachoArm64BLDiffs scans data for arm64 BL instructions and accumulates
// (target_VA - E) differences for all pclntab entries E within the window.
func collectMachoArm64BLDiffs(data []byte, textAddr uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	const (
		arm64InstrSize   = 4
		arm64BLOpcode    = uint32(0b100101) // bits[31:26] of BL instruction
		arm64BLImmMask   = uint32(0x03ffffff)
		arm64BLOpcShift  = 26
		arm64BLImmBits   = 26
		arm64BLSignShift = 32 - arm64BLImmBits // = 6
	)
	for i := 0; i+arm64InstrSize <= len(data); i += arm64InstrSize {
		instr := binary.LittleEndian.Uint32(data[i : i+arm64InstrSize])
		if instr>>arm64BLOpcShift == arm64BLOpcode {
			imm26Raw := instr & arm64BLImmMask
			imm26 := int32(imm26Raw<<arm64BLSignShift) >> arm64BLSignShift       //nolint:gosec // G115: safe, imm26Raw is masked to 26 bits
			target := textAddr + uint64(i) + uint64(int64(imm26)*arm64InstrSize) //nolint:gosec // G115: result bounded by address space
			collectMachoWindowDiffs(target, sortedEntries, diffCounts)
		}
	}
}

// maxMachoOffset is the upper bound for C startup code size (8 KB).
const maxMachoOffset = int64(0x2000)

// collectMachoWindowDiffs records (target - E) for all pclntab entries E
// in the range [target - maxMachoOffset, target].
func collectMachoWindowDiffs(target uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	lo := uint64(0)
	if target > uint64(maxMachoOffset) {
		lo = target - uint64(maxMachoOffset)
	}
	idxLo := sort.Search(len(sortedEntries), func(i int) bool {
		return sortedEntries[i] >= lo
	})
	for i := idxLo; i < len(sortedEntries) && sortedEntries[i] <= target; i++ {
		diff := int64(target - sortedEntries[i]) //nolint:gosec // G115: subtraction result bounded by maxMachoOffset
		diffCounts[diff]++
	}
}
