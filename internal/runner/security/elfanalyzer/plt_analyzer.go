package elfanalyzer

import (
	"debug/elf"
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// pltEntrySize is the byte size of a single PLT stub for all supported
// architectures (x86_64 and arm64 both use 16-byte PLT entries).
const pltEntrySize = 16

// elf64RelASize is the byte size of a 64-bit ELF relocation-with-addend entry
// (r_offset + r_info + r_addend = 8+8+8 bytes).
const elf64RelASize = 24

// elf64RelASymShift is the bit shift to extract the symbol index from r_info.
// ELF64_R_SYM(i) = (i) >> 32.
const elf64RelASymShift = 32

// errRelaPLTMalformed is returned when .rela.plt data is not a multiple of elf64RelASize.
var errRelaPLTMalformed = errors.New(".rela.plt size is not a multiple of the entry size") //nolint:misspell // SHT_RELA is an ELF standard term, not a typo

// findFuncPLTAddr returns the PLT stub virtual address for the named
// undefined (imported) function in elfFile.
//
// It locates the symbol's index in .dynsym, finds the matching entry in
// .rela.plt, then computes the stub address:
//   - .plt.sec present (IBT-enabled): plt_sec_base + relaIndex * pltEntrySize
//   - .plt only (traditional):        plt_base + (relaIndex+1) * pltEntrySize
//
// Returns (addr, true, nil) on success, (0, false, nil) if the function
// has no PLT entry, or (0, false, err) on a parse error.
func findFuncPLTAddr(elfFile *elf.File, funcName string) (uint64, bool, error) {
	// Step 1: locate funcName in the dynamic symbol table.
	// DynamicSymbols() strips the null entry at index 0, so elfFile symbol
	// index for dynsyms[i] is i+1.
	dynsyms, err := elfFile.DynamicSymbols()
	if err != nil {
		if errors.Is(err, elf.ErrNoSymbols) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("reading .dynsym: %w", err)
	}

	elfSymIdx := -1
	for i, s := range dynsyms {
		if s.Name == funcName && s.Section == elf.SHN_UNDEF {
			elfSymIdx = i + 1
			break
		}
	}
	if elfSymIdx < 0 {
		return 0, false, nil
	}

	// Step 2: scan .rela.plt for the relocation that references elfSymIdx.
	relaSection := elfFile.Section(".rela.plt")
	if relaSection == nil {
		return 0, false, nil
	}
	relaData, err := relaSection.Data()
	if err != nil {
		return 0, false, fmt.Errorf("reading .rela.plt: %w", err)
	}
	if len(relaData)%elf64RelASize != 0 {
		return 0, false, errRelaPLTMalformed
	}

	bo := elfFile.ByteOrder
	relaIdx := -1
	for i := 0; i < len(relaData)/elf64RelASize; i++ {
		entry := relaData[i*elf64RelASize : (i+1)*elf64RelASize]
		rInfo := bo.Uint64(entry[8:16])
		symIdx := int(rInfo >> elf64RelASymShift) //nolint:gosec // G115: rInfo>>32 fits int on all supported platforms
		if symIdx == elfSymIdx {
			relaIdx = i
			break
		}
	}
	if relaIdx < 0 {
		return 0, false, nil
	}

	// Step 3: compute PLT stub address.
	// Prefer .plt.sec (IBT-enabled): stub i maps directly to relaIdx.
	if pltSec := elfFile.Section(".plt.sec"); pltSec != nil {
		return pltSec.Addr + uint64(relaIdx)*pltEntrySize, true, nil //nolint:gosec // G115: relaIdx >= 0
	}
	// Traditional .plt: entry 0 is the resolver; function entries start at index 1.
	if plt := elfFile.Section(".plt"); plt != nil {
		return plt.Addr + uint64(relaIdx+1)*pltEntrySize, true, nil //nolint:gosec // G115: relaIdx+1 >= 1
	}

	return 0, false, nil
}

// EvaluatePLTCallArgs scans the .text section of elfFile for CALL/BL instructions
// targeting funcName's PLT stub, performs a backward scan for the third argument
// register at each call site (rdx on x86_64, x2 on arm64), and returns the
// highest-risk SyscallArgEvalResult across all sites.
//
// The caller convention and the Linux syscall convention place the third argument
// in the same register on both x86_64 (rdx) and arm64 (x2), so the same scan
// applies to calls via the C ABI.
//
// Returns (nil, nil) if funcName has no PLT entry or no call sites are found.
// Returns *UnsupportedArchitectureError (detectable via errors.As) for
// unsupported architectures.
func (a *SyscallAnalyzer) EvaluatePLTCallArgs(elfFile *elf.File, funcName string) (*common.SyscallArgEvalResult, error) {
	cfg, ok := a.archConfigs[elfFile.Machine]
	if !ok {
		return nil, &UnsupportedArchitectureError{Machine: elfFile.Machine}
	}

	// Find the PLT stub address for funcName.
	pltAddr, found, err := findFuncPLTAddr(elfFile, funcName)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	// Load .text section.
	textSection := elfFile.Section(".text")
	if textSection == nil {
		return nil, nil
	}
	code, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("reading .text: %w", err)
	}
	baseAddr := textSection.Addr

	// Scan .text for CALL/BL instructions targeting pltAddr, evaluate
	// the third argument register at each site, and keep the highest-risk result.
	var bestResult *common.SyscallArgEvalResult

	pos := 0
	for pos < len(code) {
		inst, decErr := cfg.decoder.Decode(code[pos:], baseAddr+uint64(pos)) //nolint:gosec // G115: pos >= 0
		if decErr != nil {
			pos += cfg.decoder.InstructionAlignment()
			continue
		}
		if inst.Len <= 0 {
			panic("decoder returned non-positive instruction length without error")
		}

		target, isCall := cfg.decoder.GetCallTarget(inst, inst.Offset)
		if isCall && target == pltAddr {
			// evalSingleMprotect backward-scans from entry.Location for rdx/x2.
			synthetic := common.SyscallInfo{Location: inst.Offset}
			result := a.evalSingleMprotect(code, baseAddr, cfg.decoder, synthetic, funcName)
			if bestResult == nil || riskPriority(result.Status) > riskPriority(bestResult.Status) {
				r := result
				bestResult = &r
			}
		}

		pos += inst.Len
	}

	return bestResult, nil
}
