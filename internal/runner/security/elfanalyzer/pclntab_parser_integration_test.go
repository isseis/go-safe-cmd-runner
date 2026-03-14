//go:build integration

package elfanalyzer

import (
	"debug/elf"
	"debug/gosym"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cgoParseSrc = `package main

// #include <stdio.h>
import "C"

import "fmt"

func main() {
	C.puts(C.CString("hello"))
	fmt.Println("hello from Go")
}
`

// buildCGOBinary compiles a CGO binary from cgoParseSrc into a temporary directory.
// Returns the path to the compiled binary.
func buildCGOBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	srcFile := filepath.Join(tmpDir, "main.go")
	err := os.WriteFile(srcFile, []byte(cgoParseSrc), 0o600)
	require.NoError(t, err, "write CGO source")

	binFile := filepath.Join(tmpDir, "cgo_binary")
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GOARCH=amd64")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build CGO binary: %s", string(out))

	return binFile
}

// TestParsePclntab_RealCGOBinary_NotStripped verifies that ParsePclntab correctly
// detects the pclntab offset for a real CGO binary (AC-1 / AC-6 regression).
//
// The detected offset is cross-validated against the actual offset derived from
// the .symtab (runtime.text VA - .text section Addr).
func TestParsePclntab_RealCGOBinary_NotStripped(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("CGO offset detection test requires amd64")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available for CGO build")
	}

	binFile := buildCGOBinary(t)

	elfFile, err := elf.Open(binFile)
	require.NoError(t, err)
	defer elfFile.Close()

	// Get true offset from .symtab: runtime.text VA - .text section Addr.
	syms, err := elfFile.Symbols()
	require.NoError(t, err, "not-stripped binary must have .symtab")

	var runtimeTextVA uint64
	for _, s := range syms {
		if s.Name == "runtime.text" {
			runtimeTextVA = s.Value
			break
		}
	}
	require.NotZero(t, runtimeTextVA, "runtime.text not found in .symtab")

	textSec := elfFile.Section(".text")
	require.NotNil(t, textSec, ".text section not found")
	expectedOffset := int64(runtimeTextVA) - int64(textSec.Addr) //nolint:gosec

	// Parse pclntab raw entries (before offset correction) to pass to detectOffsetByCallTargets.
	rawFuncs, err := parsePclntabRaw(elfFile)
	require.NoError(t, err, "parsePclntabRaw should succeed")

	detectedOffset := detectOffsetByCallTargets(elfFile, rawFuncs)

	assert.Equal(t, expectedOffset, detectedOffset,
		"detected offset 0x%x should match symtab offset 0x%x", detectedOffset, expectedOffset)
	assert.Greater(t, detectedOffset, int64(0),
		"CGO binary must have positive offset (C startup code precedes Go text)")
}

// TestParsePclntab_RealCGOBinary_Stripped verifies that ParsePclntab returns
// the same offset for a stripped binary (no .symtab) as for the unstripped one (AC-2 regression).
func TestParsePclntab_RealCGOBinary_Stripped(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("CGO offset detection test requires amd64")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available for CGO build")
	}
	if _, err := exec.LookPath("strip"); err != nil {
		t.Skip("strip not available")
	}

	binFile := buildCGOBinary(t)

	// Get expected offset from the not-stripped binary via .symtab.
	elfNotStripped, err := elf.Open(binFile)
	require.NoError(t, err)
	defer elfNotStripped.Close()

	rawFuncs, err := parsePclntabRaw(elfNotStripped)
	require.NoError(t, err)
	expectedOffset := detectOffsetByCallTargets(elfNotStripped, rawFuncs)
	require.Greater(t, expectedOffset, int64(0), "not-stripped binary must have positive offset")

	// Strip the binary.
	strippedFile := binFile + "_stripped"
	cmd := exec.Command("strip", "-o", strippedFile, binFile)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "strip: %s", string(out))

	// Verify stripped binary: .symtab should be absent.
	elfStripped, err := elf.Open(strippedFile)
	require.NoError(t, err)
	defer elfStripped.Close()

	rawFuncsStripped, err := parsePclntabRaw(elfStripped)
	require.NoError(t, err, "parsePclntabRaw should succeed on stripped binary")

	detectedOffset := detectOffsetByCallTargets(elfStripped, rawFuncsStripped)

	assert.Equal(t, expectedOffset, detectedOffset,
		"stripped binary should detect same offset 0x%x as not-stripped", expectedOffset)
}

// parsePclntabRaw returns uncorrected pclntab function entries (as gosym returns them,
// before any offset correction). Used by integration tests to call detectOffsetByCallTargets directly.
func parsePclntabRaw(elfFile *elf.File) (map[string]PclntabFunc, error) {
	pclntabSection := elfFile.Section(".gopclntab")
	if pclntabSection == nil {
		return nil, ErrNoPclntab
	}

	pclntabData, err := pclntabSection.Data()
	if err != nil {
		return nil, err
	}

	if err := checkPclntabVersion(pclntabData, elfFile.ByteOrder); err != nil {
		return nil, err
	}

	var textStart uint64
	if textSec := elfFile.Section(".text"); textSec != nil {
		textStart = textSec.Addr
	}

	lineTable := gosym.NewLineTable(pclntabData, textStart)
	symTable, err := gosym.NewTable(nil, lineTable)
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
