package elfanalyzer

import (
	"debug/elf"
	"encoding/binary"
	"sort"

	"golang.org/x/arch/arm64/arm64asm"
)

const (
	arm64ReloadSearchWindow     = 8
	arm64HelperSearchWindow     = 15
	arm64SaveSearchWindow       = 15
	arm64PrologueSearchWindow   = 6
	arm64FunctionTailSearchSpan = 24
)

// ARM64GoWrapperResolver implements GoWrapperResolver for arm64 binaries.
type ARM64GoWrapperResolver struct {
	goWrapperBase
	decoder *ARM64Decoder // Shared decoder instance to avoid repeated allocation
}

// NewARM64GoWrapperResolver creates a new ARM64GoWrapperResolver and loads symbols
// from the given ELF file's .gopclntab section.
//
// Returns an error if symbol loading fails (e.g., missing .gopclntab).
// Even on error, the returned resolver is safe to use; it simply has no
// symbols loaded and FindWrapperCalls will return nil.
func NewARM64GoWrapperResolver(elfFile *elf.File) (*ARM64GoWrapperResolver, error) {
	r := newARM64GoWrapperResolver()
	if err := r.loadFromPclntab(elfFile); err != nil {
		return r, err
	}
	r.decoder.SetDataSections(loadARM64DataSections(elfFile))
	r.hasSymbols = len(r.symbols) > 0
	return r, nil
}

// newARM64GoWrapperResolver creates an empty ARM64GoWrapperResolver without loading symbols.
// This is used internally and by tests that set up symbols manually.
func newARM64GoWrapperResolver() *ARM64GoWrapperResolver {
	return &ARM64GoWrapperResolver{
		goWrapperBase: goWrapperBase{
			symbols:      make(map[string]SymbolInfo),
			wrapperAddrs: make(map[uint64]GoSyscallWrapper),
		},
		decoder: NewARM64Decoder(),
	}
}

// FindWrapperCalls implements GoWrapperResolver.
// Scans the code section for BL instructions targeting known Go syscall wrappers,
// then resolves the syscall number from the preceding X0/W0 register assignments.
// On arm64, all instructions are exactly 4 bytes. On decode failure, the scanner
// advances by 4 bytes (InstructionAlignment) to stay aligned.
func (r *ARM64GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int) {
	r.discoverTransparentWrappers(code, baseAddr)
	return r.findWrapperCalls(code, baseAddr, r.decoder)
}

func loadARM64DataSections(elfFile *elf.File) []arm64DataSection {
	sectionNames := []string{".noptrdata", ".rodata", ".data"}
	sections := make([]arm64DataSection, 0, len(sectionNames))
	for _, name := range sectionNames {
		sec := elfFile.Section(name)
		if sec == nil {
			continue
		}
		data, err := sec.Data()
		if err != nil || len(data) == 0 {
			continue
		}
		sections = append(sections, arm64DataSection{Addr: sec.Addr, Data: data})
	}
	return sections
}

func (r *ARM64GoWrapperResolver) discoverTransparentWrappers(code []byte, baseAddr uint64) {
	if len(r.wrapperAddrs) == 0 || len(code) == 0 {
		return
	}

	// totalLookback is the maximum number of instructions that must precede the
	// call candidate for the backward-scan chain to succeed:
	//   reload(8) + helper(15) + save(15) + prologue(6) = 44.
	// arm64FunctionTailSearchSpan is the lookahead needed to find the closing RET.
	// The window holds at most winSize instructions at any one time, so memory is
	// O(1) with respect to binary size instead of O(len(code)/4).
	const totalLookback = arm64ReloadSearchWindow + arm64HelperSearchWindow +
		arm64SaveSearchWindow + arm64PrologueSearchWindow // 44
	const winSize = totalLookback + 1 + arm64FunctionTailSearchSpan // 69

	win := make([]DecodedInstruction, 0, winSize)
	pos := 0

	// Fill the initial window.
	for len(win) < winSize {
		var (
			inst DecodedInstruction
			ok   bool
		)
		inst, pos, ok = r.decodeOneAt(code, baseAddr, pos)
		if !ok {
			break
		}
		win = append(win, inst)
	}

	// Slide: win[totalLookback] is the call candidate; it has totalLookback
	// instructions of lookback history and up to arm64FunctionTailSearchSpan
	// instructions of lookahead for RET detection.
	for len(win) > totalLookback {
		r.addTransparentWrapperFromCall(win, totalLookback)

		// Advance: drop the oldest instruction and append the next decoded one.
		copy(win, win[1:])
		win = win[:len(win)-1]
		var (
			inst DecodedInstruction
			ok   bool
		)
		inst, pos, ok = r.decodeOneAt(code, baseAddr, pos)
		if ok {
			win = append(win, inst)
		}
	}

	r.sortAndDedupWrapperRanges()
}

// decodeOneAt decodes the next valid instruction starting at pos, skipping
// undecodable 4-byte chunks to stay instruction-aligned (ARM64 is fixed-width).
// Returns (instruction, advancedPos, true) on success,
// or (zero, pos, false) when the end of code is reached without a valid instruction.
func (r *ARM64GoWrapperResolver) decodeOneAt(code []byte, baseAddr uint64, pos int) (DecodedInstruction, int, bool) {
	for pos+arm64InstructionLen <= len(code) {
		inst, err := r.decoder.Decode(code[pos:], baseAddr+uint64(pos)) //nolint:gosec // G115: pos bounded by loop condition
		pos += arm64InstructionLen
		if err == nil {
			return inst, pos, true
		}
	}
	return DecodedInstruction{}, pos, false
}

func (r *ARM64GoWrapperResolver) addTransparentWrapperFromCall(insts []DecodedInstruction, callIdx int) {
	target, ok := r.decoder.GetCallTarget(insts[callIdx], insts[callIdx].Offset)
	if !ok {
		return
	}
	wrapperName, ok := r.wrapperAddrs[target]
	if !ok {
		return
	}

	loadIdx, stackOff, ok := findArm64StackReload(insts, callIdx)
	if !ok {
		return
	}
	helperIdx, ok := findArm64HelperCall(insts, loadIdx, r.wrapperAddrs, r.decoder)
	if !ok {
		return
	}
	saveIdx, ok := findArm64StackSave(insts, helperIdx, stackOff)
	if !ok {
		return
	}
	prologueIdx, ok := findArm64Prologue(insts, saveIdx)
	if !ok {
		return
	}

	r.registerTransparentWrapper(insts, prologueIdx, callIdx, wrapperName)
}

func (r *ARM64GoWrapperResolver) registerTransparentWrapper(insts []DecodedInstruction, prologueIdx, callIdx int, wrapperName GoSyscallWrapper) {
	start := insts[prologueIdx].Offset
	if _, exists := r.wrapperAddrs[start]; !exists {
		r.wrapperAddrs[start] = wrapperName
	}

	end := insts[callIdx].Offset + uint64(arm64InstructionLen)
	for j := callIdx + 1; j < len(insts) && j <= callIdx+arm64FunctionTailSearchSpan; j++ {
		a, ok := insts[j].arch.(arm64asm.Inst)
		if ok && a.Op == arm64asm.RET {
			end = insts[j].Offset + uint64(arm64InstructionLen)
			break
		}
	}
	if end > start {
		r.wrapperRanges = append(r.wrapperRanges, wrapperRange{start: start, end: end})
	}
}

func (r *ARM64GoWrapperResolver) sortAndDedupWrapperRanges() {
	if len(r.wrapperRanges) <= 1 {
		return
	}
	sort.Slice(r.wrapperRanges, func(i, j int) bool {
		if r.wrapperRanges[i].start == r.wrapperRanges[j].start {
			return r.wrapperRanges[i].end < r.wrapperRanges[j].end
		}
		return r.wrapperRanges[i].start < r.wrapperRanges[j].start
	})
	dedup := r.wrapperRanges[:0]
	for i := range r.wrapperRanges {
		if len(dedup) == 0 {
			dedup = append(dedup, r.wrapperRanges[i])
			continue
		}
		last := dedup[len(dedup)-1]
		if last.start == r.wrapperRanges[i].start && last.end == r.wrapperRanges[i].end {
			continue
		}
		dedup = append(dedup, r.wrapperRanges[i])
	}
	r.wrapperRanges = dedup
}

func findArm64StackReload(insts []DecodedInstruction, callIdx int) (int, int64, bool) {
	start := max(callIdx-arm64ReloadSearchWindow, 0)
	for i := callIdx - 1; i >= start; i-- {
		a, ok := insts[i].arch.(arm64asm.Inst)
		if !ok || a.Op != arm64asm.LDR || a.Args[0] == nil || a.Args[1] == nil {
			continue
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.X0) {
			continue
		}
		mem, ok := a.Args[1].(arm64asm.MemImmediate)
		if !ok || mem.Mode != arm64asm.AddrOffset || mem.Base != arm64asm.RegSP(arm64asm.SP) {
			continue
		}
		off, ok := arm64UnsignedOffsetFromEnc(binary.LittleEndian.Uint32(insts[i].Raw), true)
		if !ok {
			continue
		}
		return i, int64(off), true //nolint:gosec // G115: max off = 4095<<3 = 32760, safely fits int64
	}
	return -1, 0, false
}

func findArm64HelperCall(insts []DecodedInstruction, idx int, wrapperAddrs map[uint64]GoSyscallWrapper, decoder *ARM64Decoder) (int, bool) {
	start := max(idx-arm64HelperSearchWindow, 0)
	for i := idx - 1; i >= start; i-- {
		target, ok := decoder.GetCallTarget(insts[i], insts[i].Offset)
		if !ok {
			continue
		}
		if _, isWrapper := wrapperAddrs[target]; isWrapper {
			continue
		}
		return i, true
	}
	return -1, false
}

func findArm64StackSave(insts []DecodedInstruction, helperIdx int, stackOff int64) (int, bool) {
	start := max(helperIdx-arm64SaveSearchWindow, 0)
	for i := helperIdx - 1; i >= start; i-- {
		a, ok := insts[i].arch.(arm64asm.Inst)
		if !ok || a.Op != arm64asm.STR || a.Args[0] == nil || a.Args[1] == nil {
			continue
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.X0) {
			continue
		}
		mem, ok := a.Args[1].(arm64asm.MemImmediate)
		if !ok || mem.Mode != arm64asm.AddrOffset || mem.Base != arm64asm.RegSP(arm64asm.SP) {
			continue
		}
		off, ok := arm64UnsignedOffsetFromEnc(binary.LittleEndian.Uint32(insts[i].Raw), true)
		if !ok {
			continue
		}
		if int64(off) == stackOff { //nolint:gosec // G115: max off = 4095<<3 = 32760, safely fits int64
			return i, true
		}
	}
	return -1, false
}

func findArm64Prologue(insts []DecodedInstruction, saveIdx int) (int, bool) {
	start := max(saveIdx-arm64PrologueSearchWindow, 0)
	for i := saveIdx - 1; i >= start; i-- {
		a, ok := insts[i].arch.(arm64asm.Inst)
		if !ok || a.Op != arm64asm.STR || a.Args[0] == nil || a.Args[1] == nil {
			continue
		}
		if !arm64MatchesReg(a.Args[0], arm64asm.X30) {
			continue
		}
		mem, ok := a.Args[1].(arm64asm.MemImmediate)
		if !ok || mem.Mode != arm64asm.AddrPreIndex || mem.Base != arm64asm.RegSP(arm64asm.SP) {
			continue
		}
		return i, true
	}
	return -1, false
}

// GetWrapperAddresses returns all known wrapper function addresses.
// This is primarily useful for testing.
func (r *ARM64GoWrapperResolver) GetWrapperAddresses() map[uint64]GoSyscallWrapper {
	return r.wrapperAddrs
}

// GetSymbols returns all loaded symbols.
// This is primarily useful for testing.
func (r *ARM64GoWrapperResolver) GetSymbols() map[string]SymbolInfo {
	return r.symbols
}
