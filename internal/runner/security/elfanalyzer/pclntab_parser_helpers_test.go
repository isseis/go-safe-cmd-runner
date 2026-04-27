//go:build test || integration

package elfanalyzer

import (
	"debug/elf"
)

// parsePclntabFuncsRaw reads .gopclntab and returns function entries as gosym
// reports them — without any CGO offset correction applied.
// Used by tests that need raw, uncorrected entries to validate the offset
// detection algorithm (detectPclntabOffset, detectOffsetByCallTargets).
//
// Note: when the same function name appears multiple times in pclntab (e.g.
// ABI0 wrapper stubs), only the last entry survives in the returned map.
// Use parsePclntabFuncs when all address ranges are needed.
func parsePclntabFuncsRaw(elfFile *elf.File) (map[string]PclntabFunc, error) {
	symTable, err := newPclntabSymTable(elfFile)
	if err != nil {
		return nil, err
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
