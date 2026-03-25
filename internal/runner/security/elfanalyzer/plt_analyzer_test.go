package elfanalyzer

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── ELF header field byte offsets (Elf64_Ehdr, little-endian) ───────────────

const (
	elfHdrTypeOff      = 16
	elfHdrMachineOff   = 18
	elfHdrVersionOff   = 20
	elfHdrShoffOff     = 40
	elfHdrEhsizeOff    = 52
	elfHdrPhentsizeOff = 54
	elfHdrShentsizeOff = 58
	elfHdrShnumOff     = 60
	elfHdrShstrndxOff  = 62

	elfIdentClassOff = 4
	elfIdentDataOff  = 5
	elfIdentVersOff  = 6

	elfShstrndxSelf = 1 // .shstrtab is always at section index 1 in synthetic ELFs
)

// ── Fixed ELF layout sizes ───────────────────────────────────────────────────

const (
	elfHdrSize64  = 64 // sizeof(Elf64_Ehdr)
	elfPhdrSize64 = 56 // sizeof(Elf64_Phdr)
	elfAlignBytes = 8  // section-header table alignment

	// sym64Size is the byte size of a 64-bit ELF symbol table entry
	// (st_name + st_info + st_other + st_shndx + st_value + st_size = 4+1+1+2+8+8 bytes).
	sym64Size = 24
)

// ── VA constants for synthetic test ELF files ───────────────────────────────

const (
	testPLTSecVA uint64 = 0x401000
	testPLTVA    uint64 = 0x401000
	testTextVA   uint64 = 0x402000
)

// ── x86-64 machine code fragments used across tests ─────────────────────────
//
// Layout (each snippet is placed at VA testTextVA = 0x402000):
//
//	offset 0: BA <imm32>  MOV EDX, prot  (5 bytes)
//	offset 5: E8 <rel32>  CALL pltAddr   (5 bytes)
//
// CALL displacement: pltAddr − (textVA + 5 + 5) = 0x401000 − 0x40200A = −0x100A
// Little-endian int32(−0x100A) = F6 EF FF FF
var (
	// movEDX7 sets prot = PROT_READ|PROT_WRITE|PROT_EXEC (0x7); exec_confirmed.
	movEDX7 = []byte{0xBA, 0x07, 0x00, 0x00, 0x00}
	// movEDX3 sets prot = PROT_READ|PROT_WRITE (0x3); exec_not_set.
	movEDX3 = []byte{0xBA, 0x03, 0x00, 0x00, 0x00}
	// callPLT is a direct x86-64 CALL to testPLTSecVA from offset 5 of testTextVA.
	callPLT = []byte{0xE8, 0xF6, 0xEF, 0xFF, 0xFF}
)

// ── arm64 machine code fragments used across tests ───────────────────────────
//
// Layout (each snippet is placed at VA testTextVA = 0x402000):
//
//	offset 0: MOVZ X2, #prot  (4 bytes)
//	offset 4: BL pltAddr      (4 bytes)
//
// BL target: testPLTSecVA = 0x401000; instruction VA = testTextVA + 4 = 0x402004
// PCRel = 0x401000 − 0x402004 = −0x1004; imm26 = −0x1004/4 = −0x401
// BL encoding: 0x94000000 | (−0x401 & 0x3FFFFFF) = 0x97FFFBFF → LE: FF FB FF 97
var (
	// movX2_7 is MOVZ X2, #7 (PROT_READ|PROT_WRITE|PROT_EXEC); exec_confirmed.
	movX2_7 = []byte{0xE2, 0x00, 0x80, 0xD2}
	// movX2_3 is MOVZ X2, #3 (PROT_READ|PROT_WRITE only); exec_not_set.
	movX2_3 = []byte{0x62, 0x00, 0x80, 0xD2}
	// blPLT is an arm64 BL to testPLTSecVA from offset 4 of testTextVA.
	blPLT = []byte{0xFF, 0xFB, 0xFF, 0x97}
)

// ── Synthetic ELF builder ────────────────────────────────────────────────────

// testELFConfig describes a minimal ELF binary to assemble for PLT tests.
type testELFConfig struct {
	machine         elf.Machine // defaults to EM_X86_64 when zero
	funcName        string      // symbol to add to .dynsym / .rela.plt (empty = omit)
	pltSecAddr      uint64      // VA for .plt.sec (0 = omit; takes priority over pltAddr)
	pltAddr         uint64      // VA for .plt   (0 = omit; used only when pltSecAddr == 0)
	textAddr        uint64      // VA for .text  (0 = omit)
	textCode        []byte      // machine code for .text
	relaPLTOverride []byte      // if non-nil, replaces the generated .rela.plt data
}

// buildTestELF assembles a minimal ELF64 from cfg and returns a parsed *elf.File.
// The file is registered for cleanup when the test ends.
func buildTestELF(t *testing.T, cfg testELFConfig) *elf.File {
	t.Helper()
	if cfg.machine == 0 {
		cfg.machine = elf.EM_X86_64
	}
	f, err := elf.NewFile(bytes.NewReader(assembleELF64(t, cfg)))
	require.NoError(t, err, "elf.NewFile on synthetic ELF")
	t.Cleanup(func() { f.Close() })
	return f
}

// assembleELF64 produces raw bytes of a minimal little-endian ELF64 binary.
//
// Fixed sections (always present):
//
//	0  null
//	1  .shstrtab
//	2  .dynstr
//	3  .dynsym   (SHT_DYNSYM, link→.dynstr)
//	4  .rela.plt (relocation-with-addend, link→.dynsym)
//
// Optional sections are appended in the order: .plt.sec/.plt, .text.
func assembleELF64(t *testing.T, cfg testELFConfig) []byte {
	t.Helper()
	le := binary.LittleEndian

	// ── .dynstr ──────────────────────────────────────────────────────────────
	dynstr := []byte{0}
	funcNameOff := uint32(0)
	if cfg.funcName != "" {
		funcNameOff = uint32(len(dynstr))
		dynstr = append(dynstr, cfg.funcName...)
		dynstr = append(dynstr, 0)
	}

	// ── .dynsym: null entry + optional function symbol ────────────────────────
	var dynsymBuf bytes.Buffer
	dynsymBuf.Write(make([]byte, sym64Size)) // null symbol (all zeros)
	if cfg.funcName != "" {
		type sym64 struct {
			Name  uint32
			Info  uint8
			Other uint8
			Shndx uint16
			Value uint64
			Size  uint64
		}
		require.NoError(t, binary.Write(&dynsymBuf, le, sym64{
			Name: funcNameOff,
			Info: elf.ST_INFO(elf.STB_GLOBAL, elf.STT_FUNC),
			// Shndx=0 → SHN_UNDEF (imported symbol)
		}))
	}

	// ── .rela.plt: one relocation for dynsym index 1 ─────────────────────────
	var relaBuf bytes.Buffer
	if cfg.relaPLTOverride != nil {
		relaBuf.Write(cfg.relaPLTOverride)
	} else if cfg.funcName != "" {
		type rela64 struct {
			Off    uint64
			Info   uint64 // sym<<32 | type
			Addend int64
		}
		require.NoError(t, binary.Write(&relaBuf, le, rela64{
			Off:  0x600000, // placeholder GOT entry address
			Info: (1 << elf64RelASymShift) | uint64(elf.R_X86_64_JMP_SLOT),
		}))
	}

	// ── Section table ────────────────────────────────────────────────────────
	type secEntry struct {
		name    string
		typ     uint32
		flags   uint64
		addr    uint64
		data    []byte
		link    uint32
		info    uint32
		entsize uint64
	}

	pltFlags := uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR)

	// Fixed sections (indices 0–4):
	// .dynsym  link=2 (.dynstr), info=1 (first global sym index)
	// .rela.plt link=3 (.dynsym)
	secs := []secEntry{
		{name: ""}, // 0: null
		{name: ".shstrtab", typ: uint32(elf.SHT_STRTAB)},                                                                      // 1: filled below
		{name: ".dynstr", typ: uint32(elf.SHT_STRTAB), data: dynstr},                                                          // 2
		{name: ".dynsym", typ: uint32(elf.SHT_DYNSYM), data: dynsymBuf.Bytes(), link: 2, info: 1, entsize: uint64(sym64Size)}, // 3
		{name: ".rela.plt", typ: uint32(elf.SHT_RELA), data: relaBuf.Bytes(), link: 3, entsize: uint64(elf64RelASize)},        //nolint:misspell // SHT_RELA is an ELF standard term, not a typo
	}

	if cfg.pltSecAddr > 0 {
		secs = append(secs, secEntry{name: ".plt.sec", typ: uint32(elf.SHT_PROGBITS), flags: pltFlags, addr: cfg.pltSecAddr})
	} else if cfg.pltAddr > 0 {
		secs = append(secs, secEntry{name: ".plt", typ: uint32(elf.SHT_PROGBITS), flags: pltFlags, addr: cfg.pltAddr})
	}
	if cfg.textAddr > 0 && len(cfg.textCode) > 0 {
		secs = append(secs, secEntry{name: ".text", typ: uint32(elf.SHT_PROGBITS), flags: pltFlags, addr: cfg.textAddr, data: cfg.textCode})
	}

	// Build .shstrtab from section names.
	shstrtab := []byte{0}
	nameOff := make([]uint32, len(secs))
	for i, s := range secs {
		if s.name != "" {
			nameOff[i] = uint32(len(shstrtab))
			shstrtab = append(shstrtab, s.name...)
			shstrtab = append(shstrtab, 0)
		}
	}
	secs[elfShstrndxSelf].data = shstrtab

	// ── Lay out the binary: header | section data | padding | shdr table ─────
	var buf bytes.Buffer
	buf.Write(make([]byte, elfHdrSize64)) // ELF header placeholder

	dataOff := make([]uint64, len(secs))
	dataLen := make([]uint64, len(secs))
	for i, s := range secs {
		if len(s.data) > 0 {
			dataOff[i] = uint64(buf.Len())
			dataLen[i] = uint64(len(s.data))
			buf.Write(s.data)
		}
	}

	for buf.Len()%elfAlignBytes != 0 {
		buf.WriteByte(0)
	}
	shoff := uint64(buf.Len())

	type shdr64 struct {
		Name      uint32
		Type      uint32
		Flags     uint64
		Addr      uint64
		Off       uint64
		Size      uint64
		Link      uint32
		Info      uint32
		Addralign uint64
		Entsize   uint64
	}
	for i, s := range secs {
		require.NoError(t, binary.Write(&buf, le, shdr64{
			Name:      nameOff[i],
			Type:      s.typ,
			Flags:     s.flags,
			Addr:      s.addr,
			Off:       dataOff[i],
			Size:      dataLen[i],
			Link:      s.link,
			Info:      s.info,
			Addralign: 1,
			Entsize:   s.entsize,
		}))
	}

	// ── Fill in the ELF header placeholder ───────────────────────────────────
	raw := buf.Bytes()
	copy(raw[0:4], []byte{0x7f, 'E', 'L', 'F'})
	raw[elfIdentClassOff] = uint8(elf.ELFCLASS64)
	raw[elfIdentDataOff] = uint8(elf.ELFDATA2LSB)
	raw[elfIdentVersOff] = uint8(elf.EV_CURRENT)
	le.PutUint16(raw[elfHdrTypeOff:], uint16(elf.ET_EXEC))
	le.PutUint16(raw[elfHdrMachineOff:], uint16(cfg.machine))
	le.PutUint32(raw[elfHdrVersionOff:], uint32(elf.EV_CURRENT))
	le.PutUint64(raw[elfHdrShoffOff:], shoff)
	le.PutUint16(raw[elfHdrEhsizeOff:], elfHdrSize64)
	le.PutUint16(raw[elfHdrPhentsizeOff:], elfPhdrSize64)
	le.PutUint16(raw[elfHdrShentsizeOff:], elfHdrSize64)
	le.PutUint16(raw[elfHdrShnumOff:], uint16(len(secs)))
	le.PutUint16(raw[elfHdrShstrndxOff:], elfShstrndxSelf)

	return raw
}

// ── Tests for findFuncPLTAddr ────────────────────────────────────────────────

// TestFindFuncPLTAddr verifies that findFuncPLTAddr resolves the correct PLT
// stub address for imported functions from .dynsym and .rela.plt.
func TestFindFuncPLTAddr(t *testing.T) {
	t.Run("plt_sec_present_uses_pltSecAddr", func(t *testing.T) {
		// IBT-enabled binary: .plt.sec present; stub i = pltSecAddr + i*pltEntrySize
		f := buildTestELF(t, testELFConfig{
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
		})
		addr, found, err := findFuncPLTAddr(f, "mprotect")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, testPLTSecVA, addr) // relaIdx=0 → pltSecAddr + 0*16
	})

	t.Run("plt_only_uses_plt_plus_entry_offset", func(t *testing.T) {
		// Traditional binary: .plt only; stub i = pltAddr + (i+1)*pltEntrySize
		f := buildTestELF(t, testELFConfig{
			funcName: "mprotect",
			pltAddr:  testPLTVA,
		})
		addr, found, err := findFuncPLTAddr(f, "mprotect")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, testPLTVA+pltEntrySize, addr) // relaIdx=0 → pltVA + 1*16
	})

	t.Run("function_absent_from_dynsym_returns_not_found", func(t *testing.T) {
		// "mprotect" is in .dynsym but we query for an unrelated name.
		f := buildTestELF(t, testELFConfig{
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
		})
		addr, found, err := findFuncPLTAddr(f, "write")
		require.NoError(t, err)
		assert.False(t, found)
		assert.Zero(t, addr)
	})

	t.Run("empty_dynsym_returns_not_found", func(t *testing.T) {
		// No funcName → .dynsym contains only the null symbol.
		f := buildTestELF(t, testELFConfig{
			pltSecAddr: testPLTSecVA,
		})
		addr, found, err := findFuncPLTAddr(f, "mprotect")
		require.NoError(t, err)
		assert.False(t, found)
		assert.Zero(t, addr)
	})

	t.Run("no_plt_section_returns_not_found", func(t *testing.T) {
		// Symbol is in .dynsym and .rela.plt, but neither .plt.sec nor .plt exists.
		f := buildTestELF(t, testELFConfig{
			funcName: "mprotect",
			// pltSecAddr and pltAddr both zero → no PLT section
		})
		addr, found, err := findFuncPLTAddr(f, "mprotect")
		require.NoError(t, err)
		assert.False(t, found)
		assert.Zero(t, addr)
	})

	t.Run("truncated_rela_plt_returns_error", func(t *testing.T) { //nolint:misspell // SHT_RELA is an ELF standard term, not a typo
		// .rela.plt size is not a multiple of elf64RelASize → parse error.
		f := buildTestELF(t, testELFConfig{
			funcName:        "mprotect",
			pltSecAddr:      testPLTSecVA,
			relaPLTOverride: make([]byte, elf64RelASize-1), // one byte short
		})
		_, found, err := findFuncPLTAddr(f, "mprotect")
		require.Error(t, err)
		assert.False(t, found)
	})
}

// ── Tests for EvaluatePLTCallArgs ────────────────────────────────────────────

// TestEvaluatePLTCallArgs verifies that EvaluatePLTCallArgs correctly evaluates
// the mprotect prot argument at PLT call sites in synthetic x86-64 and arm64
// binaries.
func TestEvaluatePLTCallArgs(t *testing.T) {
	t.Run("exec_confirmed_prot_has_exec_bit", func(t *testing.T) {
		// MOV EDX, 7 (PROT_READ|PROT_WRITE|PROT_EXEC); CALL mprotect PLT stub
		code := append(append([]byte(nil), movEDX7...), callPLT...)
		f := buildTestELF(t, testELFConfig{
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
			textAddr:   testTextVA,
			textCode:   code,
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, common.SyscallArgEvalExecConfirmed, result.Status)
	})

	t.Run("exec_not_set_prot_lacks_exec_bit", func(t *testing.T) {
		// MOV EDX, 3 (PROT_READ|PROT_WRITE only); CALL mprotect PLT stub
		code := append(append([]byte(nil), movEDX3...), callPLT...)
		f := buildTestELF(t, testELFConfig{
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
			textAddr:   testTextVA,
			textCode:   code,
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, common.SyscallArgEvalExecNotSet, result.Status)
	})

	t.Run("no_call_to_plt_returns_nil", func(t *testing.T) {
		// .text has a MOV EDX but no CALL targeting the PLT stub.
		f := buildTestELF(t, testELFConfig{
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
			textAddr:   testTextVA,
			textCode:   movEDX7, // no CALL instruction
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("no_plt_entry_returns_nil", func(t *testing.T) {
		// mprotect is absent from .dynsym → findFuncPLTAddr returns not-found.
		f := buildTestELF(t, testELFConfig{
			// no funcName → no PLT entry
			pltSecAddr: testPLTSecVA,
			textAddr:   testTextVA,
			textCode:   append(append([]byte(nil), movEDX7...), callPLT...),
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("unsupported_architecture_returns_error", func(t *testing.T) {
		f := buildTestELF(t, testELFConfig{
			machine:  elf.EM_ARM, // not supported by SyscallAnalyzer
			funcName: "mprotect",
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		assert.Nil(t, result)
		var archErr *UnsupportedArchitectureError
		assert.True(t, errors.As(err, &archErr))
	})

	t.Run("arm64_exec_confirmed_prot_has_exec_bit", func(t *testing.T) {
		// MOVZ X2, #7 (PROT_READ|PROT_WRITE|PROT_EXEC); BL mprotect PLT stub
		code := append(append([]byte(nil), movX2_7...), blPLT...)
		f := buildTestELF(t, testELFConfig{
			machine:    elf.EM_AARCH64,
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
			textAddr:   testTextVA,
			textCode:   code,
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, common.SyscallArgEvalExecConfirmed, result.Status)
	})

	t.Run("arm64_exec_not_set_prot_lacks_exec_bit", func(t *testing.T) {
		// MOVZ X2, #3 (PROT_READ|PROT_WRITE only); BL mprotect PLT stub
		code := append(append([]byte(nil), movX2_3...), blPLT...)
		f := buildTestELF(t, testELFConfig{
			machine:    elf.EM_AARCH64,
			funcName:   "mprotect",
			pltSecAddr: testPLTSecVA,
			textAddr:   testTextVA,
			textCode:   code,
		})
		result, err := NewSyscallAnalyzer().EvaluatePLTCallArgs(f, "mprotect")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, common.SyscallArgEvalExecNotSet, result.Status)
	})
}
