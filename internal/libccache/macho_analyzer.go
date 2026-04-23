package libccache

import (
	"debug/macho"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/arm64util"
)

// svcMacOSEncoding is the little-endian uint32 encoding of the "svc #0x80" instruction.
// ARM64 encoding: 0xD4001001.
const svcMacOSEncoding = uint32(0xD4001001)

// machoSymTypeDebugMin is the minimum n_type value for debug/stab entries (N_STAB range).
// Symbols with type >= this value are compiler-generated debug entries and should be excluded.
const machoSymTypeDebugMin = uint8(0x20)

// MachoLibSystemAnalyzer analyzes a libsystem_kernel.dylib Mach-O file and returns
// a list of syscall wrapper functions.
type MachoLibSystemAnalyzer struct{}

// Analyze scans exported functions in machoFile and returns WrapperEntry values
// for functions recognized as syscall wrappers.
// For non-arm64 architectures, logs an info message and returns nil, nil.
// The returned slice is sorted by Number and then by Name.
func (a *MachoLibSystemAnalyzer) Analyze(machoFile *macho.File) ([]WrapperEntry, error) {
	if machoFile.Cpu != macho.CpuArm64 {
		slog.Info("Skipping libsystem_kernel.dylib analysis: not arm64",
			"cpu", fmt.Sprintf("%v", machoFile.Cpu))
		return nil, nil
	}

	// Get the __TEXT,__text section.
	textSection := machoFile.Section("__text")
	if textSection == nil || textSection.Seg != "__TEXT" {
		return nil, nil
	}
	code, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
	}
	textBase := textSection.Addr // Virtual address base.

	// Enumerate externally defined symbols from LC_SYMTAB.
	if machoFile.Symtab == nil {
		return nil, nil
	}

	// Sort by address to estimate function sizes.
	syms := filterFunctionSymbols(machoFile.Symtab.Syms)
	sort.Slice(syms, func(i, j int) bool {
		return syms[i].Value < syms[j].Value
	})

	textEnd := textBase + uint64(len(code))
	var entries []WrapperEntry

	for i, sym := range syms {
		// Estimate function size because Mach-O symtab has no st_size equivalent.
		// Skip aliases (syms[j].Value == sym.Value) to avoid zero-length bodies.
		var funcEnd uint64
		for j := i + 1; j < len(syms); j++ {
			if syms[j].Value > sym.Value {
				funcEnd = syms[j].Value
				break
			}
		}
		if funcEnd == 0 {
			funcEnd = textEnd
		}
		// Clamp the inferred end to the __TEXT,__text boundary so the last
		// in-range function is still analyzed even if the next symbol is in a
		// later section.
		if funcEnd > textEnd {
			funcEnd = textEnd
		}
		if sym.Value >= funcEnd || sym.Value < textBase {
			continue
		}
		funcSize := funcEnd - sym.Value

		// Real syscall wrappers are small; skip oversized functions.
		if funcSize > MaxWrapperFunctionSize {
			continue
		}

		// Slice out the function bytes.
		startOff := int(sym.Value - textBase) //nolint:gosec // #nosec G115 -- safe: sym.Value >= textBase verified above
		endOff := int(funcEnd - textBase)     //nolint:gosec // #nosec G115 -- safe: funcEnd <= textEnd verified above
		funcCode := code[startOff:endOff]

		// Detect svc #0x80 and resolve the BSD syscall number by scanning backward for x16 setup.
		number, ok := analyzeWrapperFunction(funcCode)
		if !ok {
			continue
		}
		entries = append(entries, WrapperEntry{Name: sym.Name, Number: number})
	}

	// Sort by Number then by Name for deterministic output.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Number != entries[j].Number {
			return entries[i].Number < entries[j].Number
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// filterFunctionSymbols returns all section-defined symbols from Symtab.
// Local symbols are kept because they are needed for accurate function boundary
// detection: a local function between two exported functions would otherwise
// cause the exported function's size to be overestimated.
// macOS Mach-O symbol type flags:
//
//	N_TYPE = 0x0E (type mask)
//	N_SECT = 0x0E (defined in section)
//	N_UNDF = 0x00 (undefined)
func filterFunctionSymbols(syms []macho.Symbol) []macho.Symbol {
	var result []macho.Symbol
	for _, s := range syms {
		// Exclude undefined symbols (imports).
		if s.Sect == 0 {
			continue
		}
		// Exclude debug symbols (N_STAB: type >= machoSymTypeDebugMin).
		if s.Type >= machoSymTypeDebugMin {
			continue
		}
		result = append(result, s)
	}
	return result
}

// analyzeWrapperFunction analyzes funcCode, which contains one function body,
// and returns a single BSD syscall number. It returns (0, false) if the
// function contains no svc or if multiple distinct syscall numbers are found.
func analyzeWrapperFunction(funcCode []byte) (int, bool) {
	var foundNumbers []int
	const instrLen = 4

	for i := 0; i+instrLen <= len(funcCode); i += instrLen {
		word := binary.LittleEndian.Uint32(funcCode[i:])
		if word != svcMacOSEncoding {
			continue
		}
		// Found svc #0x80. Scan backward to find the immediate loaded into x16.
		num, ok := arm64util.BackwardScanX16(funcCode, i)
		if !ok {
			return 0, false
		}
		foundNumbers = append(foundNumbers, num)
	}

	if len(foundNumbers) == 0 {
		return 0, false
	}

	// A valid syscall wrapper calls exactly one syscall; reject functions with mixed numbers.
	first := foundNumbers[0]
	for _, n := range foundNumbers[1:] {
		if n != first {
			return 0, false
		}
	}
	return first, true
}
