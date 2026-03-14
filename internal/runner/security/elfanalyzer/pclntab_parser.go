package elfanalyzer

import (
	"debug/elf"
	"debug/gosym"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
	functions, err := parsePclntabFuncsRaw(elfFile)
	if err != nil {
		return nil, err
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

// parsePclntabFuncsRaw reads .gopclntab and returns function entries as gosym
// reports them — without any CGO offset correction applied.
// This is the shared core used by ParsePclntab (which then corrects the offset)
// and by tests that need the raw, uncorrected entries to validate the offset
// detection algorithm directly.
func parsePclntabFuncsRaw(elfFile *elf.File) (map[string]PclntabFunc, error) {
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

	// CALL/BL target cross-reference (Go 1.20+, magic 0xfffffff1).
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
	// A valid offset is strictly positive (distinguishes CGO from non-CGO where offset=0)
	// and does not exceed the .text section size.
	// Negative offsets are theoretically impossible for CGO binaries (C startup code
	// always precedes Go text) and must be rejected to prevent address corruption.
	if offset <= 0 || uint64(offset) > textSection.FileSize { //nolint:gosec
		return 0
	}
	return offset
}

// detectOffsetByCallTargets detects the pclntab address offset in CGO binaries
// by cross-referencing CALL/BL instruction targets with pclntab function entries.
// This method works independently of the pclntab header format and is the sole
// offset detection mechanism. Only called after checkPclntabVersion confirms
// magic = 0xfffffff1 (Go 1.20+, officially supported: Go 1.26).
//
// It scans the first 256 KB of .text for CALL/BL targets, builds a histogram of
// (target - E) for all pclntab entries E within [target - maxOffset, target],
// and returns the most frequent value if it appears at least minVotes times.
// Returns 0 if detection fails.
//
// This window exact-match approach is reliable for real Go binaries where
// functions are typically 0x20–0x320 bytes apart. The nearest-neighbor approach
// (comparing only the single closest entry) fails in dense layouts because the
// closest entry is rarely the callee, scattering votes across many diff values.
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

	sr := io.NewSectionReader(textSection, 0, int64(scanLimit)) //nolint:gosec // G115: scanLimit is a small positive constant
	data, err := io.ReadAll(sr)
	if err != nil {
		return 0
	}

	// Build sorted slice of pclntab entry addresses for window-based diff counting.
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
	// Require a unique winner: if two diffs share the top vote count, detection
	// is ambiguous and we return 0 rather than making a non-deterministic choice
	// (Go map iteration order is randomized, so "first winner wins" would be
	// unpredictable across runs).
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

// collectX86CallDiffs scans data for x86_64 CALL rel32 (opcode 0xE8) instructions
// and accumulates (target_VA - E) differences in diffCounts for all pclntab entries E
// within [target - maxOffset, target].
//
// x86_64 is a variable-length instruction set, so this scanner advances one
// byte at a time when the current byte is not 0xE8. This means bytes inside
// multi-byte operands that happen to equal 0xE8 may be misidentified as CALL
// instructions, producing spurious targets. Such false positives introduce
// noise into diffCounts but are absorbed by the histogram voting: a spurious
// target is unlikely to produce the same difference as the true offset, so it
// will not accumulate enough votes to reach minVotes.
func collectX86CallDiffs(data []byte, textAddr uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	const (
		x86CallOpcode    = byte(0xe8)
		x86CallInstrSize = 5 // opcode(1) + rel32(4)
	)
	for i := 0; i < len(data); {
		if data[i] == x86CallOpcode && i+x86CallInstrSize <= len(data) {
			rel := int32(binary.LittleEndian.Uint32(data[i+1 : i+x86CallInstrSize]))       //nolint:gosec // G115: uint32 to int32 for sign-extended relative offset
			target := textAddr + uint64(i) + uint64(x86CallInstrSize) + uint64(int64(rel)) //nolint:gosec // G115: result bounded by address space
			collectWindowDiffs(target, sortedEntries, diffCounts)
			i += x86CallInstrSize
			continue
		}
		i++
	}
}

// collectArm64BLDiffs scans data for arm64 BL instructions (bits[31:26] == 0b100101)
// and accumulates (target_VA - E) differences in diffCounts for all pclntab entries E
// within [target - maxOffset, target].
func collectArm64BLDiffs(data []byte, textAddr uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	const (
		arm64InstrSize   = 4
		arm64BLOpcode    = uint32(0b100101) // bits[31:26] of BL instruction
		arm64BLImmMask   = uint32(0x03ffffff)
		arm64BLOpcShift  = 26
		arm64BLImmBits   = 26                  // width of the imm26 field
		arm64BLSignShift = 32 - arm64BLImmBits // = 6; shift to sign-extend imm26 to int32 via <<N>>N
	)
	for i := 0; i+arm64InstrSize <= len(data); i += arm64InstrSize {
		instr := binary.LittleEndian.Uint32(data[i : i+arm64InstrSize])
		if instr>>arm64BLOpcShift == arm64BLOpcode {
			imm26Raw := instr & arm64BLImmMask
			imm26 := int32(imm26Raw<<arm64BLSignShift) >> arm64BLSignShift       //nolint:gosec // G115: safe, imm26Raw is masked to 26 bits; shift clears high bits before conversion
			target := textAddr + uint64(i) + uint64(int64(imm26)*arm64InstrSize) //nolint:gosec // G115: result bounded by address space
			collectWindowDiffs(target, sortedEntries, diffCounts)
		}
	}
}

// collectWindowDiffs records (target - E) for all pclntab entries E
// in the range [target - maxOffset, target].
//
// Rationale: in a CGO binary, every CALL to a Go function satisfies
//
//	target = rawEntry + offset  =>  target - rawEntry = offset  (exact)
//
// so the correct offset accumulates votes from all Go-function calls.
// Noise (calls to non-Go targets, or wrong-entry pairs) produces scattered
// diffs and cannot match the vote count of the true offset.
//
// maxOffset is the upper bound for C startup code size (8 KB is generous).
const maxOffset = int64(0x2000)

func collectWindowDiffs(target uint64, sortedEntries []uint64, diffCounts map[int64]int) {
	lo := uint64(0)
	if target > uint64(maxOffset) {
		lo = target - uint64(maxOffset)
	}
	// Binary search: find first index where sortedEntries[i] >= lo
	idxLo := sort.Search(len(sortedEntries), func(i int) bool {
		return sortedEntries[i] >= lo
	})
	for i := idxLo; i < len(sortedEntries) && sortedEntries[i] <= target; i++ {
		// diff is in [0, maxOffset], so the cast to int64 is safe
		diff := int64(target - sortedEntries[i]) //nolint:gosec // G115: subtraction result bounded by maxOffset (0x2000), fits in int64
		diffCounts[diff]++
	}
}
