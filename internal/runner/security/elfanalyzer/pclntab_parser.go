package elfanalyzer

import (
	"debug/elf"
	"debug/gosym"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// Errors
var (
	ErrNoPclntab                 = errors.New("no .gopclntab section found")
	ErrUnsupportedPclntab        = errors.New("unsupported pclntab format")
	ErrInvalidPclntab            = errors.New("invalid pclntab structure")
	ErrUnsupportedPclntabVersion = errors.New("unsupported pclntab version: only magic 0xfffffff1 (Go 1.20+) is supported")
)

// PclntabFunc represents a function entry in pclntab.
type PclntabFunc struct {
	Name  string
	Entry uint64 // Function entry address
	End   uint64 // Function end address (if available)
}

// ParsePclntab reads the .gopclntab section from an ELF file and extracts
// function information. This works even on stripped binaries because Go
// runtime requires pclntab for stack traces and garbage collection.
//
// For CGO binaries, the .text section contains C runtime startup code before
// the Go runtime functions. This causes pclntab addresses to be offset from
// the actual virtual addresses. ParsePclntab detects and corrects this offset
// using CALL/BL instruction cross-referencing (no .symtab required).
//
// Only pclntab with magic 0xfffffff1 (Go 1.20+, officially supported: Go 1.26)
// is supported. Other versions return ErrUnsupportedPclntabVersion.
func ParsePclntab(elfFile *elf.File) (map[string]PclntabFunc, error) {
	pclntabSection := elfFile.Section(".gopclntab")
	if pclntabSection == nil {
		return nil, ErrNoPclntab
	}

	pclntabData, err := pclntabSection.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read .gopclntab: %w", err)
	}

	if err := checkPclntabVersion(pclntabData, elfFile.ByteOrder); err != nil {
		return nil, err
	}

	// The text start address is required by gosym.NewLineTable.
	// In Go 1.26+, textStart was removed from the pclntab header and must be
	// obtained from the ELF .text section directly.
	var textStart uint64
	textSection := elfFile.Section(".text")
	if textSection != nil {
		textStart = textSection.Addr
	}

	lineTable := gosym.NewLineTable(pclntabData, textStart)
	symTable, err := gosym.NewTable(nil, lineTable)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnsupportedPclntab, err)
	}

	functions := make(map[string]PclntabFunc, len(symTable.Funcs))
	for i := range symTable.Funcs {
		fn := &symTable.Funcs[i]
		functions[fn.Name] = PclntabFunc{
			Name:  fn.Name,
			Entry: fn.Entry,
			End:   fn.End,
		}
	}

	// CGO binaries may have a constant address offset between pclntab entries
	// and actual virtual addresses because C runtime startup code is inserted
	// at the beginning of the .text section. Detect and apply the correction.
	if offset := detectPclntabOffset(elfFile, functions); offset != 0 {
		for name, fn := range functions {
			functions[name] = PclntabFunc{
				Name:  fn.Name,
				Entry: uint64(int64(fn.Entry) + offset), //nolint:gosec // G115: offset is bounded by binary size, no overflow risk
				End:   uint64(int64(fn.End) + offset),   //nolint:gosec // G115: offset is bounded by binary size, no overflow risk
			}
		}
	}

	return functions, nil
}

// checkPclntabVersion verifies that the pclntab magic is supported.
// Only magic = 0xfffffff1 (Go 1.20–1.26, CurrentPCLnTabMagic) is supported.
// Other magic values (e.g. 0xfffffff0 for Go 1.18–1.19) return
// ErrUnsupportedPclntabVersion to prevent incorrect offset application.
//
// Note: Go 1.20–1.25 share the same magic (0xfffffff1) and will pass this
// check. The officially supported version is Go 1.26 (tested), but Go
// 1.20–1.25 binaries may also work in practice.
func checkPclntabVersion(data []byte, byteOrder binary.ByteOrder) error {
	const (
		pclntabMagicSize = 4
		go120magic       = uint32(0xfffffff1) // Go 1.20–1.26 (CurrentPCLnTabMagic)
	)
	if len(data) < pclntabMagicSize {
		return ErrInvalidPclntab
	}
	magic := byteOrder.Uint32(data[0:pclntabMagicSize])
	if magic != go120magic {
		return fmt.Errorf("%w (got magic 0x%x)", ErrUnsupportedPclntabVersion, magic)
	}
	return nil
}

// detectPclntabOffset returns the address correction needed for pclntab entries
// in CGO binaries. Uses CALL/BL instruction cross-referencing to detect the
// offset without requiring .symtab (supports stripped binaries).
//
// Returns 0 if no correction is needed (non-CGO binaries) or if detection fails.
func detectPclntabOffset(elfFile *elf.File, pclntabFuncs map[string]PclntabFunc) int64 {
	textSection := elfFile.Section(".text")
	if textSection == nil {
		return 0
	}

	// CALL/BL target cross-reference (Go 1.26+).
	// Only reached after checkPclntabVersion confirms a supported binary.
	// CGO binaries always have a positive offset (C startup code precedes Go
	// text), so negative or zero results indicate detection failure.
	//
	// IMPORTANT: pclntabFuncs must contain the *uncorrected* Entry values as
	// returned by gosym (i.e., before any offset correction is applied).
	// The algorithm computes (CALL target VA) - (pclntab Entry) = offset,
	// which is only valid when pclntabFuncs entries are still offset-shifted.
	// ParsePclntab calls detectPclntabOffset *before* applying the correction,
	// so this invariant is guaranteed by the call order.
	offset := detectOffsetByCallTargets(elfFile, pclntabFuncs)
	if !isValidOffset(offset, textSection.FileSize) {
		return 0
	}
	return offset
}

// isValidOffset checks that offset is a plausible CGO text-start correction.
// A valid offset is strictly positive (distinguishes CGO from non-CGO where offset=0)
// and does not exceed the .text section size.
// Negative offsets are theoretically impossible for CGO binaries (C startup code
// always precedes Go text) and must be rejected to prevent address corruption.
func isValidOffset(offset int64, textFileSize uint64) bool {
	return offset > 0 && uint64(offset) <= textFileSize //nolint:gosec
}

// detectOffsetByCallTargets detects the pclntab address offset in CGO binaries
// by cross-referencing CALL/BL instruction targets with pclntab function entries.
// This method works independently of the pclntab header format and is the sole
// offset detection mechanism. Only called after checkPclntabVersion confirms
// magic = 0xfffffff1 (Go 1.20+, officially supported: Go 1.26).
//
// It scans the first 256 KB of .text for CALL/BL targets, builds a histogram of
// (target - nearestPclntabEntry) differences, and returns the most frequent value
// if it appears at least minVotes times. Returns 0 if detection fails.
//
// PRECONDITION: pclntabFuncs must contain *uncorrected* Entry values (as returned
// by gosym.NewLineTable before any offset correction). The algorithm relies on the
// invariant: CALL_target_VA - pclntab_Entry = C_startup_size (constant per binary).
// If corrected entries were passed, all differences would collapse to 0.
func detectOffsetByCallTargets(
	elfFile *elf.File,
	pclntabFuncs map[string]PclntabFunc,
) int64 {
	const (
		scanLimit = 256 * 1024 // scan first 256 KB of .text
		minVotes  = 3
	)

	textSection := elfFile.Section(".text")
	if textSection == nil {
		return 0
	}

	rawData, err := textSection.Data()
	if err != nil {
		return 0
	}
	data := rawData
	if len(data) > scanLimit {
		data = data[:scanLimit]
	}

	// Build sorted slice of pclntab entry addresses for nearest-neighbor search.
	sortedEntries := make([]uint64, 0, len(pclntabFuncs))
	for _, fn := range pclntabFuncs {
		if fn.Entry != 0 {
			sortedEntries = append(sortedEntries, fn.Entry)
		}
	}
	if len(sortedEntries) == 0 {
		return 0
	}
	sort.Slice(sortedEntries, func(i, j int) bool { return sortedEntries[i] < sortedEntries[j] })

	diffCounts := make(map[int64]int)
	textAddr := textSection.Addr

	switch elfFile.Machine {
	case elf.EM_X86_64:
		collectX86CallDiffs(data, textAddr, sortedEntries, diffCounts)
	case elf.EM_AARCH64:
		collectArm64BLDiffs(data, textAddr, sortedEntries, diffCounts)
	default:
		return 0
	}

	// Find the most frequent difference value.
	var bestDiff int64
	bestCount := 0
	for diff, count := range diffCounts {
		if count > bestCount {
			bestCount = count
			bestDiff = diff
		}
	}

	if bestCount < minVotes {
		return 0
	}
	return bestDiff
}

// collectX86CallDiffs scans data for x86_64 CALL rel32 (opcode 0xE8) instructions
// and accumulates (target_VA - nearest_pclntab_entry) differences in diffCounts.
func collectX86CallDiffs(data []byte, textAddr uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	const (
		x86CallOpcode    = byte(0xe8)
		x86CallInstrSize = 5 // opcode(1) + rel32(4)
	)
	for i := 0; i < len(data); {
		if data[i] == x86CallOpcode && i+x86CallInstrSize <= len(data) {
			rel := int32(binary.LittleEndian.Uint32(data[i+1 : i+x86CallInstrSize]))       //nolint:gosec // G115: uint32 to int32 for sign-extended relative offset
			target := textAddr + uint64(i) + uint64(x86CallInstrSize) + uint64(int64(rel)) //nolint:gosec // G115: result bounded by address space
			recordDiff(target, sortedEntries, diffCounts)
			i += x86CallInstrSize
			continue
		}
		i++
	}
}

// collectArm64BLDiffs scans data for arm64 BL instructions (bits[31:26] == 0b100101)
// and accumulates (target_VA - nearest_pclntab_entry) differences in diffCounts.
func collectArm64BLDiffs(data []byte, textAddr uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	const (
		arm64InstrSize   = 4
		arm64BLOpcode    = uint32(0b100101) // bits[31:26] of BL instruction
		arm64BLImmMask   = uint32(0x03ffffff)
		arm64BLOpcShift  = 26
		arm64BLSignShift = 6 // shift amount to sign-extend 26-bit immediate to 32-bit
	)
	for i := 0; i+arm64InstrSize <= len(data); i += arm64InstrSize {
		instr := binary.LittleEndian.Uint32(data[i : i+arm64InstrSize])
		if instr>>arm64BLOpcShift == arm64BLOpcode {
			imm26 := int32(instr&arm64BLImmMask) << arm64BLSignShift >> arm64BLSignShift // sign-extend 26-bit
			target := textAddr + uint64(i) + uint64(int64(imm26)*arm64InstrSize)         //nolint:gosec // G115: result bounded by address space
			recordDiff(target, sortedEntries, diffCounts)
		}
	}
}

// recordDiff finds the nearest pclntab entry to target and records the difference
// in diffCounts when the absolute difference is within 0x1000 bytes.
//
// PRECONDITION: sortedEntries must not contain address 0 (enforced by the
// caller which filters fn.Entry != 0). findNearest returns 0 to signal "no
// entry close enough", relying on this invariant to avoid ambiguity.
func recordDiff(target uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	nearest := findNearest(sortedEntries, target)
	if nearest == 0 {
		return
	}
	diff := int64(target) - int64(nearest) //nolint:gosec // G115: addresses are valid ELF virtual addresses
	const maxDiff = int64(0x1000)
	if diff > -maxDiff && diff < maxDiff {
		diffCounts[diff]++
	}
}

// findNearest returns the nearest pclntab entry address to target using binary search.
// sortedEntries must be sorted in ascending order.
// Returns 0 if no entry is within maxDistance — 0 is safe as a sentinel because
// the caller (recordDiff) guarantees sortedEntries contains no address 0.
func findNearest(sortedEntries []uint64, target uint64) uint64 {
	const maxDistance = 0x1000
	n := len(sortedEntries)
	if n == 0 {
		return 0
	}

	// Find insertion point.
	idx := sort.Search(n, func(i int) bool { return sortedEntries[i] >= target })

	var best uint64
	bestDist := uint64(maxDistance + 1)

	// Check candidate at idx and idx-1.
	for _, i := range []int{idx - 1, idx} {
		if i < 0 || i >= n {
			continue
		}
		var dist uint64
		if sortedEntries[i] >= target {
			dist = sortedEntries[i] - target
		} else {
			dist = target - sortedEntries[i]
		}
		if dist < bestDist {
			bestDist = dist
			best = sortedEntries[i]
		}
	}

	if bestDist > maxDistance {
		return 0
	}
	return best
}
