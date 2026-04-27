package elfanalyzer

import (
	"debug/elf"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMachineCodeDecoder is a test mock for MachineCodeDecoder.
type MockMachineCodeDecoder struct {
	decodeFunc func(buf []byte, baseAddr uint64) (DecodedInstruction, error)
}

func (m *MockMachineCodeDecoder) Decode(code []byte, offset uint64) (DecodedInstruction, error) {
	return m.decodeFunc(code, offset)
}

func (m *MockMachineCodeDecoder) IsSyscallInstruction(_ DecodedInstruction) bool {
	return false
}

func (m *MockMachineCodeDecoder) WritesSyscallReg(_ DecodedInstruction) bool {
	return false
}

func (m *MockMachineCodeDecoder) IsSyscallNumImm(_ DecodedInstruction) (bool, int64) {
	return false, 0
}

func (m *MockMachineCodeDecoder) IsControlFlowInstruction(_ DecodedInstruction) bool {
	return false
}

func (m *MockMachineCodeDecoder) InstructionAlignment() int {
	return 1 // default: x86_64 behavior
}

func (m *MockMachineCodeDecoder) MaxInstructionLength() int {
	return maxInstructionLength // default: x86_64 behavior
}

func (m *MockMachineCodeDecoder) GetCallTarget(_ DecodedInstruction, _ uint64) (uint64, bool) {
	return 0, false
}

func (m *MockMachineCodeDecoder) IsFirstArgImm(_ DecodedInstruction) (bool, int64) {
	return false, 0
}

func (m *MockMachineCodeDecoder) ModifiesFirstArg(_ DecodedInstruction) bool {
	return false
}

func (m *MockMachineCodeDecoder) ResolveFirstArgGlobal(_ []DecodedInstruction, _ int) (bool, int64) {
	return false, 0
}

func (m *MockMachineCodeDecoder) ModifiesThirdArg(_ DecodedInstruction) bool {
	return false
}

func (m *MockMachineCodeDecoder) IsThirdArgImm(_ DecodedInstruction) (bool, int64) {
	return false, 0
}

func hasUnknownSyscall(syscalls []SyscallInfo) bool {
	for _, info := range syscalls {
		if info.Number == -1 {
			return true
		}
	}
	return false
}

func TestSyscallAnalyzer_BackwardScan(t *testing.T) {
	tests := []struct {
		name       string
		code       []byte
		wantNumber int
		wantMethod string
	}{
		{
			name: "immediate mov before syscall",
			// mov $0x29, %eax; syscall
			code:       []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05},
			wantNumber: 41,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "immediate with unrelated instruction",
			// mov $0x2a, %eax; mov %rsi, %rdi; syscall
			code:       []byte{0xb8, 0x2a, 0x00, 0x00, 0x00, 0x48, 0x89, 0xf7, 0x0f, 0x05},
			wantNumber: 42,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "register copy from immediate source",
			// mov $0x2a, %edx; mov %edx, %eax; syscall
			code:       []byte{0xba, 0x2a, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: 42,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "register copy from zeroing source",
			// xor %edx, %edx; mov %edx, %eax; syscall
			code:       []byte{0x31, 0xd2, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: 0,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "register copy from r9d immediate source",
			// mov $0xca, %r9d; mov %r9d, %eax; syscall
			code:       []byte{0x41, 0xb9, 0xca, 0x00, 0x00, 0x00, 0x44, 0x89, 0xc8, 0x0f, 0x05},
			wantNumber: 202,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "phase2 predecessor single path via jump",
			// mov $0x2a, %edx; jmp join; nop; nop; join: mov %edx, %eax; syscall
			code:       []byte{0xba, 0x2a, 0x00, 0x00, 0x00, 0xeb, 0x02, 0x90, 0x90, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: 42,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "phase2 predecessor multi-path same value",
			// cmp %ecx,%ecx; jne alt; mov $0x2a,%edx; jmp join; alt: mov $0x2a,%edx; join: mov %edx,%eax; syscall
			code:       []byte{0x39, 0xc9, 0x75, 0x07, 0xba, 0x2a, 0x00, 0x00, 0x00, 0xeb, 0x05, 0xba, 0x2a, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: 42,
			wantMethod: DeterminationMethodImmediate,
		},
		{
			name: "phase2 predecessor multi-path conflicting value",
			// cmp %ecx,%ecx; jne alt; mov $0x2a,%edx; jmp join; alt: mov $0x2b,%edx; join: mov %edx,%eax; syscall
			code:       []byte{0x39, 0xc9, 0x75, 0x07, 0xba, 0x2a, 0x00, 0x00, 0x00, 0xeb, 0x05, 0xba, 0x2b, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: DeterminationMethodUnknownIndirectSetting,
		},
		{
			name: "register move (indirect)",
			// mov %ebx, %eax; syscall
			code:       []byte{0x89, 0xd8, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: DeterminationMethodUnknownIndirectSetting,
		},
		{
			name: "control flow boundary",
			// mov $0x29, %eax; jmp label(+5); syscall
			// When backwardScanForSyscallNumber scans backward from syscall,
			// it encounters jmp first, which creates a control flow boundary.
			code:       []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0xeb, 0x05, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: DeterminationMethodUnknownControlFlowBoundary,
		},
		{
			name: "syscall only (no eax modification)",
			code: []byte{0x0f, 0x05},
			// With only the syscall instruction and no prior instructions,
			// the decode window is empty [0, 0), so decode_failed is returned.
			wantNumber: -1,
			wantMethod: DeterminationMethodUnknownDecodeFailed,
		},
		{
			name: "memory load to eax (indirect)",
			// mov (%rsp), %eax; syscall
			code:       []byte{0x8b, 0x04, 0x24, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: DeterminationMethodUnknownIndirectSetting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyscallAnalyzer()
			cfg := analyzer.archConfigs[elf.EM_X86_64]
			result := analyzer.analyzeSyscallsInCode(tt.code, 0, cfg.decoder, cfg.syscallTable, nil)
			require.Len(t, result.DetectedSyscalls, 1)

			info := result.DetectedSyscalls[0]
			assert.Equal(t, tt.wantNumber, info.Number)
			assert.Equal(t, tt.wantMethod, info.Occurrences[0].DeterminationMethod)
		})
	}
}

func TestSyscallAnalyzer_BackwardScan_HighRisk(t *testing.T) {
	tests := []struct {
		name       string
		code       []byte
		wantMethod string
	}{
		{
			name:       "indirect setting is high risk",
			code:       []byte{0x89, 0xd8, 0x0f, 0x05}, // mov %ebx, %eax; syscall
			wantMethod: DeterminationMethodUnknownIndirectSetting,
		},
		{
			name:       "control flow boundary is high risk",
			code:       []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0xeb, 0x05, 0x0f, 0x05}, // mov $0x29, %eax; jmp label(+5); syscall
			wantMethod: DeterminationMethodUnknownControlFlowBoundary,
		},
		{
			name:       "decode failed is high risk",
			code:       []byte{0x0f, 0x05},
			wantMethod: DeterminationMethodUnknownDecodeFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyscallAnalyzer()
			cfg := analyzer.archConfigs[elf.EM_X86_64]
			result := analyzer.analyzeSyscallsInCode(tt.code, 0, cfg.decoder, cfg.syscallTable, nil)
			require.Len(t, result.DetectedSyscalls, 1)

			info := result.DetectedSyscalls[0]
			assert.Equal(t, -1, info.Number)
			assert.Equal(t, tt.wantMethod, info.Occurrences[0].DeterminationMethod)
			assert.NotEmpty(t, result.AnalysisWarnings)
		})
	}
}

func TestSyscallAnalyzer_NegativeImmediateValue(t *testing.T) {
	// Test that negative immediate values (e.g., 0xffffffff decoded as -1)
	// are rejected and return unknown:indirect_setting, not immediate.
	// This prevents inconsistency where Number=-1 (unknown sentinel) with
	// DeterminationMethodImmediate could occur.
	//
	// mov $0xffffffff, %eax; syscall
	// The immediate 0xffffffff is sign-extended to -1 in a signed int64.
	code := []byte{
		0xb8, 0xff, 0xff, 0xff, 0xff, // mov $0xffffffff, %eax
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)
	require.Len(t, result.DetectedSyscalls, 1)

	info := result.DetectedSyscalls[0]
	// Number should be -1 (unknown), not the negative value itself
	assert.Equal(t, -1, info.Number)
	// Method should be unknown:indirect_setting, not immediate
	assert.Equal(t, DeterminationMethodUnknownIndirectSetting, info.Occurrences[0].DeterminationMethod)
	assert.NotEmpty(t, result.AnalysisWarnings)
}

func TestSyscallAnalyzer_OutOfRangeImmediateValue(t *testing.T) {
	// Test that immediate values outside the valid syscall range
	// are rejected and return unknown:indirect_setting.
	//
	// mov $0x1000, %eax; syscall (0x1000 = 4096, well beyond valid syscall range)
	code := []byte{
		0xb8, 0x00, 0x10, 0x00, 0x00, // mov $0x1000, %eax
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)
	require.Len(t, result.DetectedSyscalls, 1)

	info := result.DetectedSyscalls[0]
	assert.Equal(t, -1, info.Number)
	assert.Equal(t, DeterminationMethodUnknownIndirectSetting, info.Occurrences[0].DeterminationMethod)
	assert.NotEmpty(t, result.AnalysisWarnings)
}

func TestSyscallAnalyzer_MultipleSyscalls(t *testing.T) {
	// mov $0x29, %eax; syscall; mov $0x2a, %eax; syscall
	code := []byte{
		0xb8, 0x29, 0x00, 0x00, 0x00, // mov $0x29, %eax
		0x0f, 0x05, // syscall
		0xb8, 0x2a, 0x00, 0x00, 0x00, // mov $0x2a, %eax
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 2)

	// First syscall: socket (41)
	assert.Equal(t, 41, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "socket", result.DetectedSyscalls[0].Name)
	assert.Equal(t, DeterminationMethodImmediate, result.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)

	// Second syscall: connect (42)
	assert.Equal(t, 42, result.DetectedSyscalls[1].Number)
	assert.Equal(t, "connect", result.DetectedSyscalls[1].Name)
	assert.Equal(t, DeterminationMethodImmediate, result.DetectedSyscalls[1].Occurrences[0].DeterminationMethod)

	assert.Empty(t, result.AnalysisWarnings)
	assert.Empty(t, result.ArgEvalResults)
}

func TestSyscallAnalyzer_NoSyscalls(t *testing.T) {
	// nop; nop; ret
	code := []byte{0x90, 0x90, 0xc3}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	assert.Empty(t, result.DetectedSyscalls)
	assert.Empty(t, result.AnalysisWarnings)
	assert.Empty(t, result.ArgEvalResults)
}

func TestSyscallAnalyzer_NetworkAndNonNetworkSyscalls(t *testing.T) {
	// mov $0x01, %eax; syscall; mov $0x29, %eax; syscall
	code := []byte{
		0xb8, 0x01, 0x00, 0x00, 0x00, // mov $0x01, %eax (write)
		0x0f, 0x05, // syscall
		0xb8, 0x29, 0x00, 0x00, 0x00, // mov $0x29, %eax (socket)
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 2)

	// First: write (non-network)
	assert.Equal(t, 1, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "write", result.DetectedSyscalls[0].Name)

	// Second: socket (network)
	assert.Equal(t, 41, result.DetectedSyscalls[1].Number)
	assert.Equal(t, "socket", result.DetectedSyscalls[1].Name)

	assert.Empty(t, result.AnalysisWarnings)
	assert.Empty(t, result.ArgEvalResults)
}

func TestSyscallAnalyzer_DeterminationDetail_X86(t *testing.T) {
	tests := []struct {
		name       string
		code       []byte
		wantNumber int
		wantMethod string
		wantDetail string
	}{
		{
			name:       "copy chain detail",
			code:       []byte{0xba, 0x2a, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05}, // mov $0x2a,%edx; mov %edx,%eax; syscall
			wantNumber: 42,
			wantMethod: DeterminationMethodImmediate,
			wantDetail: DeterminationDetailX86CopyChain,
		},
		{
			name:       "branch converged detail",
			code:       []byte{0x39, 0xc9, 0x75, 0x07, 0xba, 0x2a, 0x00, 0x00, 0x00, 0xeb, 0x05, 0xba, 0x2a, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: 42,
			wantMethod: DeterminationMethodImmediate,
			wantDetail: DeterminationDetailX86BranchConverged,
		},
		{
			name:       "copy chain unresolved detail",
			code:       []byte{0x39, 0xc9, 0x75, 0x07, 0xba, 0x2a, 0x00, 0x00, 0x00, 0xeb, 0x05, 0xba, 0x2b, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: DeterminationMethodUnknownIndirectSetting,
			wantDetail: DeterminationDetailX86CopyChainUnresolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyscallAnalyzer()
			cfg := analyzer.archConfigs[elf.EM_X86_64]
			result := analyzer.analyzeSyscallsInCode(tt.code, 0, cfg.decoder, cfg.syscallTable, nil)
			require.Len(t, result.DetectedSyscalls, 1)

			occ := result.DetectedSyscalls[0].Occurrences[0]
			assert.Equal(t, tt.wantNumber, result.DetectedSyscalls[0].Number)
			assert.Equal(t, tt.wantMethod, occ.DeterminationMethod)
			assert.Equal(t, tt.wantDetail, occ.DeterminationDetail)
		})
	}
}

func TestSyscallAnalyzer_DeterminationStats(t *testing.T) {
	// sequence:
	// 1) mov $0x2a,%edx; mov %edx,%eax; syscall                -> copy chain immediate
	// 2) cmp %ecx,%ecx; jne alt; mov $0x2b,%edx; ...; syscall  -> branch converged immediate
	// 3) mov %ebx,%eax; syscall                                 -> unknown indirect
	code := []byte{
		0xba, 0x2a, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05,
		0x39, 0xc9, 0x75, 0x07, 0xba, 0x2b, 0x00, 0x00, 0x00, 0xeb, 0x05, 0xba, 0x2b, 0x00, 0x00, 0x00, 0x89, 0xd0, 0x0f, 0x05,
		0x89, 0xd8, 0x0f, 0x05,
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.NotNil(t, result.DeterminationStats)
	assert.Equal(t, 2, result.DeterminationStats.ImmediateTotal)
	assert.Equal(t, 1, result.DeterminationStats.ImmediateViaCopyChain)
	assert.Equal(t, 1, result.DeterminationStats.ImmediateViaBranchConvergence)
	assert.Equal(t, 1, result.DeterminationStats.UnknownIndirectSetting)
}

func TestSyscallAnalyzer_UnknownWarningIncludesDetail(t *testing.T) {
	code := []byte{0x89, 0xd8, 0x0f, 0x05} // mov %ebx, %eax; syscall

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.NotEmpty(t, result.AnalysisWarnings)
	assert.Contains(t, result.AnalysisWarnings[0], "unknown:indirect_setting")
	assert.Contains(t, result.AnalysisWarnings[0], "detail=x86_copy_chain_unresolved")
}

func TestSyscallAnalyzer_BackwardScan_MisalignedWindowRecovery(t *testing.T) {
	prefix := append([]byte{0xeb, 0x00}, make([]byte, 730)...)
	for i := 2; i < len(prefix); i++ {
		prefix[i] = 0x90
	}

	pattern := []byte{
		0xf3, 0x0f, 0x1e, 0xfa,
		0xba, 0xe7, 0x00, 0x00, 0x00,
		0xeb, 0x06,
		0x0f, 0x1f, 0x44, 0x00, 0x00,
		0xf4,
		0x89, 0xd0,
		0x0f, 0x05,
	}
	code := make([]byte, 0, len(prefix)+len(pattern))
	code = append(code, prefix...)
	code = append(code, pattern...)

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 1)
	info := result.DetectedSyscalls[0]
	occ := info.Occurrences[0]
	assert.Equal(t, 231, info.Number)
	assert.Equal(t, "exit_group", info.Name)
	assert.Equal(t, DeterminationMethodImmediate, occ.DeterminationMethod)
	assert.NotEqual(t, DeterminationDetailX86CopyChainUnresolved, occ.DeterminationDetail)
}

func TestSyscallAnalyzer_CopyChain_IgnoresSourceClobberAfterCopy(t *testing.T) {
	// mov $0x2a,%edx; mov %edx,%eax; mov $0x3c,%edx; syscall
	// EDX is clobbered after the copy into EAX, but syscall number in EAX remains 42.
	code := []byte{
		0xba, 0x2a, 0x00, 0x00, 0x00,
		0x89, 0xd0,
		0xba, 0x3c, 0x00, 0x00, 0x00,
		0x0f, 0x05,
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 1)
	info := result.DetectedSyscalls[0]
	occ := info.Occurrences[0]

	assert.Equal(t, 42, info.Number)
	assert.Equal(t, "connect", info.Name)
	assert.Equal(t, DeterminationMethodImmediate, occ.DeterminationMethod)
	assert.Equal(t, DeterminationDetailX86CopyChain, occ.DeterminationDetail)
}

func TestSyscallAnalyzer_MixedKnownAndUnknown(t *testing.T) {
	// mov $0x29, %eax; syscall; mov %ebx, %eax; syscall
	code := []byte{
		0xb8, 0x29, 0x00, 0x00, 0x00, // mov $0x29, %eax (socket)
		0x0f, 0x05, // syscall
		0x89, 0xd8, // mov %ebx, %eax (indirect)
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 2)

	// First: socket (known)
	assert.Equal(t, 41, result.DetectedSyscalls[0].Number)
	assert.Equal(t, DeterminationMethodImmediate, result.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)

	// Second: unknown (indirect)
	assert.Equal(t, -1, result.DetectedSyscalls[1].Number)
	assert.Equal(t, DeterminationMethodUnknownIndirectSetting, result.DetectedSyscalls[1].Occurrences[0].DeterminationMethod)

	assert.NotEmpty(t, result.AnalysisWarnings)
	assert.Empty(t, result.ArgEvalResults)
}

func TestSyscallAnalyzer_WithBaseAddress(t *testing.T) {
	// mov $0x29, %eax; syscall
	code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05}
	baseAddr := uint64(0x401000)

	analyzer := NewSyscallAnalyzer()
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, baseAddr, cfg.decoder, cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 1)
	assert.Equal(t, 41, result.DetectedSyscalls[0].Number)
	assert.Equal(t, baseAddr+5, result.DetectedSyscalls[0].Occurrences[0].Location) // syscall at offset 5
}

func TestSyscallAnalyzer_InvalidOffset(t *testing.T) {
	// Test extractSyscallInfo with invalid offsets
	analyzer := NewSyscallAnalyzer()
	code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05}

	// syscallAddr < baseAddr
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	info := analyzer.extractSyscallInfo(code, 0, 100, cfg.decoder, cfg.syscallTable)
	assert.Equal(t, -1, info.Number)
	assert.Equal(t, DeterminationMethodUnknownInvalidOffset, info.Occurrences[0].DeterminationMethod)

	// syscallAddr beyond code length
	info = analyzer.extractSyscallInfo(code, 200, 0, cfg.decoder, cfg.syscallTable)
	assert.Equal(t, -1, info.Number)
	assert.Equal(t, DeterminationMethodUnknownInvalidOffset, info.Occurrences[0].DeterminationMethod)
}

func TestSyscallAnalyzer_FindSyscallInstructions(t *testing.T) {
	analyzer := NewSyscallAnalyzer()

	tests := []struct {
		name      string
		code      []byte
		baseAddr  uint64
		wantCount int
		wantLocs  []uint64
	}{
		{
			name:      "no syscall instructions",
			code:      []byte{0x90, 0x90, 0xc3}, // nop; nop; ret
			baseAddr:  0,
			wantCount: 0,
		},
		{
			name:      "single syscall",
			code:      []byte{0x0f, 0x05}, // syscall
			baseAddr:  0x1000,
			wantCount: 1,
			wantLocs:  []uint64{0x1000},
		},
		{
			name:      "multiple syscalls",
			code:      []byte{0x0f, 0x05, 0x90, 0x0f, 0x05}, // syscall; nop; syscall
			baseAddr:  0x2000,
			wantCount: 2,
			wantLocs:  []uint64{0x2000, 0x2003},
		},
		{
			name:      "code too short",
			code:      []byte{0x0f},
			baseAddr:  0,
			wantCount: 0,
		},
		{
			name:      "empty code",
			code:      []byte{},
			baseAddr:  0,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := analyzer.archConfigs[elf.EM_X86_64]
			locs, _ := analyzer.findSyscallInstructions(tt.code, tt.baseAddr, cfg.decoder)
			assert.Len(t, locs, tt.wantCount)
			if tt.wantLocs != nil {
				assert.Equal(t, tt.wantLocs, locs)
			}
		})
	}
}

func TestSyscallAnalyzer_DecodeInstructionsInWindow(t *testing.T) {
	analyzer := NewSyscallAnalyzer()

	// mov $0x29, %eax; nop; syscall
	code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x90, 0x0f, 0x05}

	// Decode window [0, 6) - should decode "mov" and "nop" but not "syscall"
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	instructions, decodeFailures := analyzer.decodeWindow(code, 0, 0, 6, cfg.decoder)
	require.Len(t, instructions, 2) // mov (5 bytes) + nop (1 byte)
	assert.Equal(t, 0, decodeFailures)

	assert.Equal(t, uint64(0), instructions[0].Offset)
	assert.Equal(t, 5, instructions[0].Len)
	assert.Equal(t, uint64(5), instructions[1].Offset)
	assert.Equal(t, 1, instructions[1].Len)
}

func TestNewSyscallAnalyzerWithConfig(t *testing.T) {
	decoder := NewX86Decoder()
	table := NewX86_64SyscallTable()

	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 10)
	assert.NotNil(t, analyzer)
	assert.Equal(t, 10, analyzer.maxBackwardScan)
}

func TestSyscallAnalyzer_DecodeStats(t *testing.T) {
	t.Run("decode failures are counted", func(t *testing.T) {
		// 0x06 is invalid in 64-bit mode, causing a decode failure.
		// After skipping it, 0x0f 0x05 (syscall) is found normally.
		code := []byte{
			0x06,       // invalid byte (PUSH ES, illegal in 64-bit mode)
			0x0f, 0x05, // syscall
		}

		analyzer := NewSyscallAnalyzer()
		cfg := analyzer.archConfigs[elf.EM_X86_64]
		result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

		assert.Greater(t, result.DecodeStats.DecodeFailureCount, 0,
			"expected at least one decode failure from invalid instruction byte")
		assert.Equal(t, len(code), result.DecodeStats.TotalBytesAnalyzed,
			"TotalBytesAnalyzed should equal the length of the code section")
	})

	t.Run("no decode failures on valid code", func(t *testing.T) {
		// mov $0x01, %eax; syscall — all valid instructions, no decode failures.
		code := []byte{
			0xb8, 0x01, 0x00, 0x00, 0x00, // mov $0x01, %eax
			0x0f, 0x05, // syscall
		}

		analyzer := NewSyscallAnalyzer()
		cfg := analyzer.archConfigs[elf.EM_X86_64]
		result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

		assert.Equal(t, 0, result.DecodeStats.DecodeFailureCount,
			"expected no decode failures for valid instruction sequence")
		assert.Equal(t, len(code), result.DecodeStats.TotalBytesAnalyzed,
			"TotalBytesAnalyzed should equal the length of the code section")
	})

	t.Run("TotalBytesAnalyzed is set even for empty code", func(t *testing.T) {
		code := []byte{}

		analyzer := NewSyscallAnalyzer()
		cfg := analyzer.archConfigs[elf.EM_X86_64]
		result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)

		assert.Equal(t, 0, result.DecodeStats.DecodeFailureCount)
		assert.Equal(t, 0, result.DecodeStats.TotalBytesAnalyzed)
	})
}

func TestSyscallAnalyzer_ScanLimitExceeded(t *testing.T) {
	// 4 nops + syscall with maxBackwardScan=3: the scan processes 3 nops and
	// hits the step limit before reaching the 4th nop (window start).
	// Expected: scan_limit_exceeded.
	code := []byte{
		0x90, 0x90, 0x90, 0x90, // 4 nops (none modify eax)
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzerWithConfig(NewX86Decoder(), NewX86_64SyscallTable(), 3)
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)
	require.Len(t, result.DetectedSyscalls, 1)

	assert.Equal(t, -1, result.DetectedSyscalls[0].Number)
	assert.Equal(t, DeterminationMethodUnknownScanLimitExceeded, result.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)
}

func TestSyscallAnalyzer_WindowExhausted(t *testing.T) {
	// 2 nops + syscall with maxBackwardScan=10: the scan examines all 2 nops
	// without hitting the step limit — the entire window is consumed.
	// Expected: window_exhausted (not scan_limit_exceeded).
	code := []byte{
		0x90, 0x90, // 2 nops (none modify eax)
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzerWithConfig(NewX86Decoder(), NewX86_64SyscallTable(), 10)
	cfg := analyzer.archConfigs[elf.EM_X86_64]
	result := analyzer.analyzeSyscallsInCode(code, 0, cfg.decoder, cfg.syscallTable, nil)
	require.Len(t, result.DetectedSyscalls, 1)

	assert.Equal(t, -1, result.DetectedSyscalls[0].Number)
	assert.Equal(t, DeterminationMethodUnknownWindowExhausted, result.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)
}

func TestSyscallAnalyzer_DecodeInstructionsInWindow_NonPositiveLength(t *testing.T) {
	// Test that decodeWindow panics when decoder returns
	// non-positive instruction lengths, indicating a programming bug.
	code := []byte{0x90, 0x90, 0x90} // 3 nop instructions

	// Create a mock decoder that returns non-positive lengths
	mockDecoder := &MockMachineCodeDecoder{
		decodeFunc: func(_ []byte, baseAddr uint64) (DecodedInstruction, error) {
			// Return instruction with zero length
			return DecodedInstruction{
				Offset: baseAddr,
				Len:    0, // Non-positive length
			}, nil
		},
	}

	analyzer := NewSyscallAnalyzerWithConfig(mockDecoder, NewX86_64SyscallTable(), 50)

	// This should panic because returning Len=0 without error is a programming bug.
	assert.Panics(t, func() {
		analyzer.decodeWindow(code, 0, 0, 3, analyzer.archConfigs[elf.EM_X86_64].decoder)
	}, "expected panic when decoder returns non-positive instruction length")
}

func TestSyscallAnalyzer_DecodeInstructionsInWindow_NegativeLength(t *testing.T) {
	// Test that decodeWindow panics when decoder returns negative lengths.
	code := []byte{0x90, 0x90, 0x90} // 3 nop instructions

	// Create a mock decoder that returns negative lengths
	mockDecoder := &MockMachineCodeDecoder{
		decodeFunc: func(_ []byte, baseAddr uint64) (DecodedInstruction, error) {
			// Return instruction with negative length
			return DecodedInstruction{
				Offset: baseAddr,
				Len:    -1, // Negative length (invalid)
			}, nil
		},
	}

	analyzer := NewSyscallAnalyzerWithConfig(mockDecoder, NewX86_64SyscallTable(), 50)

	// This should panic because returning Len=-1 without error is a programming bug.
	assert.Panics(t, func() {
		analyzer.decodeWindow(code, 0, 0, 3, analyzer.archConfigs[elf.EM_X86_64].decoder)
	}, "expected panic when decoder returns negative instruction length")
}

func TestSyscallAnalyzer_UnsupportedArchitecture(t *testing.T) {
	// AnalyzeSyscallsFromELF should return UnsupportedArchitectureError for
	// architectures that are not registered in archConfigs.
	analyzer := NewSyscallAnalyzer()

	// elf.EM_386 (32-bit x86) is not supported; only x86_64 and arm64 are.
	elfFile := &elf.File{FileHeader: elf.FileHeader{Machine: elf.EM_386}}
	_, err := analyzer.AnalyzeSyscallsFromELF(elfFile)

	require.Error(t, err)
	var unsupportedErr *UnsupportedArchitectureError
	require.ErrorAs(t, err, &unsupportedErr)
	assert.Equal(t, elf.EM_386, unsupportedErr.Machine)
}

func TestSyscallAnalyzer_ARM64AnalysisPath(t *testing.T) {
	// Verify that arm64 syscall analysis is registered in NewSyscallAnalyzer()
	// and produces correct results when called directly via analyzeSyscallsInCode.
	analyzer := NewSyscallAnalyzer()
	arm64Cfg := analyzer.archConfigs[elf.EM_AARCH64]
	require.NotNil(t, arm64Cfg, "arm64 archConfig must be registered by NewSyscallAnalyzer")
	assert.Equal(t, "arm64", arm64Cfg.archName)

	// arm64 machine code: mov x8, #198 (socket syscall number); svc #0
	// svc #0:       {0x01, 0x00, 0x00, 0xD4}
	// mov x8, #198: {0xC8, 0x18, 0x80, 0xD2}
	code := []byte{
		0xC8, 0x18, 0x80, 0xD2, // mov x8, #198 (socket syscall)
		0x01, 0x00, 0x00, 0xD4, // svc #0
	}

	result := analyzer.analyzeSyscallsInCode(code, 0, arm64Cfg.decoder, arm64Cfg.syscallTable, nil)

	require.Len(t, result.DetectedSyscalls, 1)
	assert.Equal(t, 198, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "socket", result.DetectedSyscalls[0].Name)
	assert.Equal(t, DeterminationMethodImmediate, result.DetectedSyscalls[0].Occurrences[0].DeterminationMethod)
}

// TestSyscallAnalyzer_AnalyzeSyscallsInRange tests the AnalyzeSyscallsInRange method.
func TestSyscallAnalyzer_AnalyzeSyscallsInRange(t *testing.T) {
	t.Run("detects syscall in range", func(t *testing.T) {
		// code layout:
		//   [0:5]  = mov $0x29, %eax (unrelated prefix bytes)
		//   [5:12] = mov $0x29, %eax; syscall  (target function)
		// We analyze [5:12] and expect syscall 41 (socket) to be detected.
		code := []byte{
			// prefix: mov $0xFF, %eax (not in range, should not affect result)
			0xb8, 0xFF, 0x00, 0x00, 0x00,
			// target function: mov $0x29, %eax; syscall
			0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05,
		}
		analyzer := NewSyscallAnalyzer()
		infos, err := analyzer.AnalyzeSyscallsInRange(code, 0x400000, 5, 12, elf.EM_X86_64)
		require.NoError(t, err)
		require.Len(t, infos, 1)
		assert.Equal(t, 41, infos[0].Number)
		assert.Equal(t, DeterminationMethodImmediate, infos[0].Occurrences[0].DeterminationMethod)
	})

	t.Run("boundary check: adjacent bytes not mixed in", func(t *testing.T) {
		// code layout:
		//   [0:5]  = mov $0x02, %eax (fork, not in range)
		//   [5:12] = mov $0x29, %eax; syscall  (target function, socket=41)
		// If startOffset clamping works correctly, the backward scan from the syscall
		// at offset 5+5=10 will only see bytes from offset 5 onward (not offset 0).
		code := []byte{
			// adjacent function: mov $0x02, %eax (fork)
			0xb8, 0x02, 0x00, 0x00, 0x00,
			// target function: mov $0x29, %eax; syscall
			0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05,
		}
		analyzer := NewSyscallAnalyzer()
		infos, err := analyzer.AnalyzeSyscallsInRange(code, 0, 5, 12, elf.EM_X86_64)
		require.NoError(t, err)
		require.Len(t, infos, 1)
		// Should detect syscall 41 (socket), not syscall 2 (fork) from adjacent bytes
		assert.Equal(t, 41, infos[0].Number)
	})

	t.Run("unsupported architecture returns UnsupportedArchitectureError", func(t *testing.T) {
		code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05}
		analyzer := NewSyscallAnalyzer()
		_, err := analyzer.AnalyzeSyscallsInRange(code, 0, 0, len(code), elf.EM_386)
		require.Error(t, err)
		var archErr *UnsupportedArchitectureError
		require.ErrorAs(t, err, &archErr)
		assert.Equal(t, elf.EM_386, archErr.Machine)
	})
}

// TestSyscallAnalyzer_GetSyscallTable tests the GetSyscallTable method.
func TestSyscallAnalyzer_GetSyscallTable(t *testing.T) {
	t.Run("supported architecture returns table and true", func(t *testing.T) {
		analyzer := NewSyscallAnalyzer()
		table, ok := analyzer.GetSyscallTable(elf.EM_X86_64)
		assert.True(t, ok)
		assert.NotNil(t, table)
	})

	t.Run("unsupported architecture returns nil and false", func(t *testing.T) {
		analyzer := NewSyscallAnalyzer()
		table, ok := analyzer.GetSyscallTable(elf.EM_386)
		assert.False(t, ok)
		assert.Nil(t, table)
	})
}

func TestSyscallAnalyzer_EvaluateMprotectArgs(t *testing.T) {
	// Use x86_64 decoder and table for component tests.
	// x86_64 syscall number for mprotect is 10 (0xa).
	decoder := NewX86Decoder()
	table := NewX86_64SyscallTable()
	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 50)

	tests := []struct {
		name          string
		code          []byte
		wantStatus    common.SyscallArgEvalStatus
		wantHasResult bool
	}{
		{
			name: "PROT_EXEC confirmed (64bit rdx)",
			// mov $0xa, %eax; mov $0x7, %rdx; syscall
			// mprotect (10) with prot=0x7 (PROT_READ|PROT_WRITE|PROT_EXEC)
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
				0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecConfirmed,
			wantHasResult: true,
		},
		{
			name: "PROT_EXEC confirmed (32bit edx)",
			// mov $0xa, %eax; mov $0x4, %edx; syscall
			// mprotect with prot=0x4 (PROT_EXEC only)
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
				0xba, 0x04, 0x00, 0x00, 0x00, // mov $0x4, %edx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecConfirmed,
			wantHasResult: true,
		},
		{
			name: "PROT_EXEC not set",
			// mov $0xa, %eax; mov $0x3, %rdx; syscall
			// mprotect with prot=0x3 (PROT_READ|PROT_WRITE)
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
				0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecNotSet,
			wantHasResult: true,
		},
		{
			name: "indirect register setting",
			// mov $0xa, %eax; mov %rsi, %rdx; syscall
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
				0x48, 0x89, 0xf2, // mov %rsi, %rdx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "control flow boundary",
			// jmp is the first instruction; it jumps to mov+syscall block.
			// Backward scan for rdx hits jmp before finding rdx setup.
			// Backward scan for eax finds mov eax first (closer to syscall than jmp).
			//
			// Layout:
			//   offset 0: jmp +5   (2 bytes) → jumps to offset 7 (mov eax)
			//   offset 2-6: 5 nops (dead code, never executed)
			//   offset 7: mov $0xa, %eax  (5 bytes)
			//   offset 12: syscall (2 bytes)
			code: []byte{
				0xeb, 0x05, // jmp +5 (target = offset 7)
				0x90, 0x90, 0x90, 0x90, 0x90, // 5 nops (dead code)
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax (mprotect)
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "mprotect syscall only (no rdx setup in scan range)",
			// mov $0xa, %eax; syscall — mprotect with no preceding rdx assignment.
			// Backward scan reaches start of code without finding any rdx modifier.
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "non-mprotect syscall only",
			// mov $0x01, %eax; syscall (write, not mprotect)
			code: []byte{
				0xb8, 0x01, 0x00, 0x00, 0x00, // mov $0x01, %eax
				0x0f, 0x05, // syscall
			},
			wantStatus:    "",
			wantHasResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run full analysis to get detected syscalls first.
			code := tt.code
			baseAddr := uint64(0x1000)

			// Manually build the detected syscalls list the same way analysis would.
			// We need to analyze the code to get DetectedSyscalls.
			result := analyzer.analyzeSyscallsInCode(code, baseAddr, decoder, table, nil)

			if !tt.wantHasResult {
				// No mprotect, ArgEvalResults should be empty
				assert.Empty(t, result.ArgEvalResults)
				return
			}

			require.NotEmpty(t, result.ArgEvalResults, "expected ArgEvalResults to be populated")
			assert.Equal(t, "mprotect", result.ArgEvalResults[0].SyscallName)
			assert.Equal(t, tt.wantStatus, result.ArgEvalResults[0].Status)
		})
	}
}

func TestSyscallAnalyzer_MultipleMprotect(t *testing.T) {
	// Use x86_64 decoder and table for component tests.
	// x86_64 syscall number for mprotect is 10 (0xa).
	decoder := NewX86Decoder()
	table := NewX86_64SyscallTable()
	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 50)
	baseAddr := uint64(0x1000)

	t.Run("exec_confirmed + exec_not_set selects exec_confirmed", func(t *testing.T) {
		// Two mprotect calls: one with PROT_EXEC, one without.
		// First: mprotect(prot=0x7) - exec_confirmed
		// Second: mprotect(prot=0x3) - exec_not_set
		code := []byte{
			// First mprotect: prot=0x7 (PROT_EXEC set)
			0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
			0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
			0x0f, 0x05, // syscall
			// Second mprotect: prot=0x3 (no PROT_EXEC)
			0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
			0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
			0x0f, 0x05, // syscall
		}
		result := analyzer.analyzeSyscallsInCode(code, baseAddr, decoder, table, nil)
		require.NotEmpty(t, result.ArgEvalResults)
		assert.Equal(t, common.SyscallArgEvalExecConfirmed, result.ArgEvalResults[0].Status)
		assert.True(t, EvalMprotectRisk(result.ArgEvalResults))
	})

	t.Run("exec_unknown + exec_not_set selects exec_unknown", func(t *testing.T) {
		// First: unknown (indirect setting), Second: exec_not_set
		code := []byte{
			// First mprotect: indirect setting
			0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
			0x48, 0x89, 0xf2, // mov %rsi, %rdx
			0x0f, 0x05, // syscall
			// Second mprotect: prot=0x3 (no PROT_EXEC)
			0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
			0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
			0x0f, 0x05, // syscall
		}
		result := analyzer.analyzeSyscallsInCode(code, baseAddr, decoder, table, nil)
		require.NotEmpty(t, result.ArgEvalResults)
		assert.Equal(t, common.SyscallArgEvalExecUnknown, result.ArgEvalResults[0].Status)
		assert.True(t, EvalMprotectRisk(result.ArgEvalResults))
	})

	t.Run("exec_not_set only does not set high risk", func(t *testing.T) {
		// mprotect with prot=0x3 only (no PROT_EXEC)
		code := []byte{
			0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
			0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
			0x0f, 0x05, // syscall
		}
		result := analyzer.analyzeSyscallsInCode(code, baseAddr, decoder, table, nil)
		require.NotEmpty(t, result.ArgEvalResults)
		assert.Equal(t, common.SyscallArgEvalExecNotSet, result.ArgEvalResults[0].Status)
		assert.False(t, EvalMprotectRisk(result.ArgEvalResults))
	})

	t.Run("exec_not_set with unknown syscall remains high risk", func(t *testing.T) {
		// Unknown syscall (indirect register setting) produces Number==-1 in DetectedSyscalls.
		// Subsequent mprotect exec_not_set must NOT change that fact.
		code := []byte{
			// Unknown syscall: mov %ebx, %eax; syscall (Number==-1)
			0x89, 0xd8, // mov %ebx, %eax
			0x0f, 0x05, // syscall
			// mprotect: mov $0xa, %eax; mov $0x3, %rdx; syscall (exec_not_set)
			0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
			0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
			0x0f, 0x05, // syscall
		}
		result := analyzer.analyzeSyscallsInCode(code, baseAddr, decoder, table, nil)
		require.NotEmpty(t, result.ArgEvalResults)
		assert.Equal(t, common.SyscallArgEvalExecNotSet, result.ArgEvalResults[0].Status)
		// Unknown syscall entry (Number==-1) must remain in DetectedSyscalls.
		assert.True(t, hasUnknownSyscall(result.DetectedSyscalls), "DetectedSyscalls should contain an unknown entry")
	})
}

func TestSyscallAnalyzer_EvaluateMprotectArgs_ARM64(t *testing.T) {
	// arm64 mprotect syscall number is 226 (0xe2).
	// Syscall instruction: svc #0  = 01 00 00 D4
	// Syscall number register: x8
	//   mov x8, #226 = MOVZ X8, #226 = 48 1C 80 D2
	// Third argument register: x2
	//   mov x2, #7   = MOVZ X2, #7   = E2 00 80 D2
	//   mov x2, #3   = MOVZ X2, #3   = 62 00 80 D2
	//   mov x2, x1   = ORR  X2,XZR,X1 = E2 03 01 AA (register move)
	// Branch: b +N words = N*4 bytes forward from PC
	//   b +5 words (20 bytes): 05 00 00 14
	decoder := NewARM64Decoder()
	table := NewARM64LinuxSyscallTable()
	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 50)
	baseAddr := uint64(0x1000)

	tests := []struct {
		name          string
		code          []byte
		wantStatus    common.SyscallArgEvalStatus
		wantHasResult bool
		wantHighRisk  bool
	}{
		{
			name: "exec_confirmed (mov x2, #7)",
			// mov x8, #226; mov x2, #7; svc #0
			// mprotect (226) with prot=0x7 (PROT_READ|PROT_WRITE|PROT_EXEC)
			code: []byte{
				0x48, 0x1C, 0x80, 0xD2, // mov x8, #226
				0xE2, 0x00, 0x80, 0xD2, // mov x2, #7
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecConfirmed,
			wantHasResult: true,
			wantHighRisk:  true,
		},
		{
			name: "exec_not_set (mov x2, #3)",
			// mov x8, #226; mov x2, #3; svc #0
			// mprotect with prot=0x3 (PROT_READ|PROT_WRITE, no PROT_EXEC)
			code: []byte{
				0x48, 0x1C, 0x80, 0xD2, // mov x8, #226
				0x62, 0x00, 0x80, 0xD2, // mov x2, #3
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecNotSet,
			wantHasResult: true,
			wantHighRisk:  false,
		},
		{
			name: "exec_unknown (register move: mov x2, x1)",
			// mov x8, #226; mov x2, x1; svc #0 — indirect prot setting
			code: []byte{
				0x48, 0x1C, 0x80, 0xD2, // mov x8, #226
				0xE2, 0x03, 0x01, 0xAA, // mov x2, x1
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
			wantHighRisk:  true,
		},
		{
			name: "exec_unknown (mprotect syscall only, no x2 setup in scan range)",
			// mov x8, #226; svc #0 — no x2 assignment before syscall.
			// Backward scan reaches the start of code without finding any x2 modifier.
			code: []byte{
				0x48, 0x1C, 0x80, 0xD2, // mov x8, #226
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
			wantHighRisk:  true,
		},
		{
			name: "exec_unknown (control flow boundary)",
			// b +5 words (lands on svc #0 at offset 20); nop; nop; nop; mov x8, #226; svc #0
			// Backward scan for x2 from svc passes over mov x8 and nops,
			// then hits the branch instruction at offset 0 and stops → exec_unknown.
			//
			// Layout (each instruction is 4 bytes):
			//   offset  0: b +5 words (20 bytes) → jumps to offset 20 (svc), skipping everything
			//   offset  4: nop
			//   offset  8: nop
			//   offset 12: nop
			//   offset 16: mov x8, #226
			//   offset 20: svc #0
			code: []byte{
				0x05, 0x00, 0x00, 0x14, // b +5 (20 bytes forward, to svc)
				0x1F, 0x20, 0x03, 0xD5, // nop
				0x1F, 0x20, 0x03, 0xD5, // nop
				0x1F, 0x20, 0x03, 0xD5, // nop
				0x48, 0x1C, 0x80, 0xD2, // mov x8, #226
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
			wantHighRisk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.analyzeSyscallsInCode(tt.code, baseAddr, decoder, table, nil)

			if !tt.wantHasResult {
				assert.Empty(t, result.ArgEvalResults)
				return
			}

			require.NotEmpty(t, result.ArgEvalResults, "expected ArgEvalResults to be populated")
			assert.Equal(t, "mprotect", result.ArgEvalResults[0].SyscallName)
			assert.Equal(t, tt.wantStatus, result.ArgEvalResults[0].Status)
			gotHighRisk := hasUnknownSyscall(result.DetectedSyscalls) || EvalMprotectRisk(result.ArgEvalResults)
			assert.Equal(t, tt.wantHighRisk, gotHighRisk)
		})
	}
}

func TestSyscallAnalyzer_EvaluatePkeyMprotectArgs(t *testing.T) {
	// x86_64 syscall number for pkey_mprotect is 329 (0x149).
	// Syscall instruction: syscall = 0F 05
	// Syscall number register: eax/rax
	//   mov eax, 329 → 0xb8 0x49 0x01 0x00 0x00 (imm32 form required since 329 > 255)
	// Third argument register: rdx / edx (same as mprotect)
	//   mov $0x7, %rdx  → 0x48 0xc7 0xc2 0x07 0x00 0x00 0x00
	//   mov $0x4, %edx  → 0xba 0x04 0x00 0x00 0x00
	//   mov $0x3, %rdx  → 0x48 0xc7 0xc2 0x03 0x00 0x00 0x00
	//   mov %rsi, %rdx  → 0x48 0x89 0xf2
	decoder := NewX86Decoder()
	table := NewX86_64SyscallTable()
	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 50)

	tests := []struct {
		name          string
		code          []byte
		wantStatus    common.SyscallArgEvalStatus
		wantHasResult bool
	}{
		{
			name: "PROT_EXEC confirmed (64bit rdx)",
			// mov eax, 329; mov $0x7, %rdx; syscall
			// pkey_mprotect (329) with prot=0x7 (PROT_READ|PROT_WRITE|PROT_EXEC)
			code: []byte{
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329
				0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecConfirmed,
			wantHasResult: true,
		},
		{
			name: "PROT_EXEC confirmed (32bit edx)",
			// mov eax, 329; mov $0x4, %edx; syscall
			// pkey_mprotect with prot=0x4 (PROT_EXEC only)
			code: []byte{
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329
				0xba, 0x04, 0x00, 0x00, 0x00, // mov $0x4, %edx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecConfirmed,
			wantHasResult: true,
		},
		{
			name: "PROT_EXEC not set",
			// mov eax, 329; mov $0x3, %rdx; syscall
			// pkey_mprotect with prot=0x3 (PROT_READ|PROT_WRITE)
			code: []byte{
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329
				0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecNotSet,
			wantHasResult: true,
		},
		{
			name: "indirect register setting",
			// mov eax, 329; mov %rsi, %rdx; syscall
			code: []byte{
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329
				0x48, 0x89, 0xf2, // mov %rsi, %rdx
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "pkey_mprotect syscall only (no rdx setup in scan range)",
			// mov eax, 329; syscall — no preceding rdx assignment.
			code: []byte{
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "control flow boundary",
			// jmp is the first instruction; it jumps to mov+syscall block.
			// Backward scan for rdx hits jmp before finding rdx setup.
			//
			// Layout:
			//   offset 0: jmp +5   (2 bytes) → jumps to offset 7 (mov eax)
			//   offset 2-6: 5 nops (dead code)
			//   offset 7: mov eax, 329  (5 bytes)
			//   offset 12: syscall (2 bytes)
			code: []byte{
				0xeb, 0x05, // jmp +5 (target = offset 7)
				0x90, 0x90, 0x90, 0x90, 0x90, // 5 nops (dead code)
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329 (pkey_mprotect)
				0x0f, 0x05, // syscall
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "non-pkey_mprotect syscall only",
			// mov $0x0a, %eax; syscall (mprotect=10, not pkey_mprotect)
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0x0a, %eax
				0x0f, 0x05, // syscall
			},
			wantStatus:    "",
			wantHasResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseAddr := uint64(0x1000)
			result := analyzer.analyzeSyscallsInCode(tt.code, baseAddr, decoder, table, nil)

			if !tt.wantHasResult {
				// No pkey_mprotect, ArgEvalResults should have no pkey_mprotect entry
				for _, r := range result.ArgEvalResults {
					assert.NotEqual(t, "pkey_mprotect", r.SyscallName,
						"expected no pkey_mprotect entry in ArgEvalResults")
				}
				return
			}

			var pkeyResult *common.SyscallArgEvalResult
			for i := range result.ArgEvalResults {
				if result.ArgEvalResults[i].SyscallName == "pkey_mprotect" {
					pkeyResult = &result.ArgEvalResults[i]
					break
				}
			}
			require.NotNil(t, pkeyResult, "expected pkey_mprotect entry in ArgEvalResults")
			assert.Equal(t, "pkey_mprotect", pkeyResult.SyscallName)
			assert.Equal(t, tt.wantStatus, pkeyResult.Status)
		})
	}
}

func TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64(t *testing.T) {
	// arm64 pkey_mprotect syscall number is 288 (0x120).
	// Syscall instruction: svc #0  = 01 00 00 D4
	// Syscall number register: x8
	//   mov x8, #288 = MOVZ X8, #288 = 08 24 80 D2
	// Third argument register: x2
	//   mov x2, #7   = MOVZ X2, #7   = E2 00 80 D2
	//   mov x2, #3   = MOVZ X2, #3   = 62 00 80 D2
	//   mov x2, x1   = ORR  X2,XZR,X1 = E2 03 01 AA (register move)
	// Branch: b +N words
	//   b +5 words (20 bytes): 05 00 00 14
	decoder := NewARM64Decoder()
	table := NewARM64LinuxSyscallTable()
	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 50)
	baseAddr := uint64(0x1000)

	tests := []struct {
		name          string
		code          []byte
		wantStatus    common.SyscallArgEvalStatus
		wantHasResult bool
	}{
		{
			name: "exec_confirmed (mov x2, #7)",
			// mov x8, #288; mov x2, #7; svc #0
			// pkey_mprotect (288) with prot=0x7 (PROT_READ|PROT_WRITE|PROT_EXEC)
			code: []byte{
				0x08, 0x24, 0x80, 0xD2, // mov x8, #288
				0xE2, 0x00, 0x80, 0xD2, // mov x2, #7
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecConfirmed,
			wantHasResult: true,
		},
		{
			name: "exec_not_set (mov x2, #3)",
			// mov x8, #288; mov x2, #3; svc #0
			// pkey_mprotect with prot=0x3 (PROT_READ|PROT_WRITE)
			code: []byte{
				0x08, 0x24, 0x80, 0xD2, // mov x8, #288
				0x62, 0x00, 0x80, 0xD2, // mov x2, #3
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecNotSet,
			wantHasResult: true,
		},
		{
			name: "exec_unknown (indirect register setting)",
			// mov x8, #288; mov x2, x1; svc #0 — indirect prot setting
			code: []byte{
				0x08, 0x24, 0x80, 0xD2, // mov x8, #288
				0xE2, 0x03, 0x01, 0xAA, // mov x2, x1
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "exec_unknown (pkey_mprotect syscall only, no x2 setup in scan range)",
			// mov x8, #288; svc #0 — no x2 assignment before syscall.
			code: []byte{
				0x08, 0x24, 0x80, 0xD2, // mov x8, #288
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
		{
			name: "exec_unknown (control flow boundary)",
			// b +5 words (lands on svc #0 at offset 20); nop×3; mov x8, #288; svc #0
			// Backward scan for x2 from svc -> hits branch at offset 0 -> exec_unknown.
			//
			// Layout (each instruction is 4 bytes):
			//   offset  0: b +5 words (20 bytes) → jumps to offset 20 (svc)
			//   offset  4: nop
			//   offset  8: nop
			//   offset 12: nop
			//   offset 16: mov x8, #288
			//   offset 20: svc #0
			code: []byte{
				0x05, 0x00, 0x00, 0x14, // b +5 (20 bytes forward, to svc)
				0x1F, 0x20, 0x03, 0xD5, // nop
				0x1F, 0x20, 0x03, 0xD5, // nop
				0x1F, 0x20, 0x03, 0xD5, // nop
				0x08, 0x24, 0x80, 0xD2, // mov x8, #288
				0x01, 0x00, 0x00, 0xD4, // svc #0
			},
			wantStatus:    common.SyscallArgEvalExecUnknown,
			wantHasResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.analyzeSyscallsInCode(tt.code, baseAddr, decoder, table, nil)

			if !tt.wantHasResult {
				assert.Empty(t, result.ArgEvalResults)
				return
			}

			var pkeyResult *common.SyscallArgEvalResult
			for i := range result.ArgEvalResults {
				if result.ArgEvalResults[i].SyscallName == "pkey_mprotect" {
					pkeyResult = &result.ArgEvalResults[i]
					break
				}
			}
			require.NotNil(t, pkeyResult, "expected pkey_mprotect entry in ArgEvalResults")
			assert.Equal(t, "pkey_mprotect", pkeyResult.SyscallName)
			assert.Equal(t, tt.wantStatus, pkeyResult.Status)
		})
	}
}

func TestSyscallAnalyzer_MprotectAndPkeyMprotect(t *testing.T) {
	// x86_64 syscall numbers:
	//   mprotect (10):      0xb8 0x0a 0x00 0x00 0x00 (mov eax, 10)
	//   pkey_mprotect (329): 0xb8 0x49 0x01 0x00 0x00 (mov eax, 329)
	// rdx setup:
	//   mov $0x7, %rdx  → 0x48 0xc7 0xc2 0x07 0x00 0x00 0x00
	//   mov $0x3, %rdx  → 0x48 0xc7 0xc2 0x03 0x00 0x00 0x00
	// syscall → 0x0f 0x05
	decoder := NewX86Decoder()
	table := NewX86_64SyscallTable()
	analyzer := NewSyscallAnalyzerWithConfig(decoder, table, 50)
	baseAddr := uint64(0x1000)

	tests := []struct {
		name        string
		code        []byte
		wantEntries []common.SyscallArgEvalResult // minimal fields to check (SyscallName + Status)
	}{
		{
			name: "both detected: exec_confirmed + exec_confirmed",
			// mprotect(prot=0x7) then pkey_mprotect(prot=0x7)
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov eax, 10 (mprotect)
				0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
				0x0f, 0x05, // syscall (mprotect)
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329 (pkey_mprotect)
				0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
				0x0f, 0x05, // syscall (pkey_mprotect)
			},
			wantEntries: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecConfirmed},
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecConfirmed},
			},
		},
		{
			name: "both detected: exec_not_set + exec_unknown",
			// mprotect(prot=0x3) then pkey_mprotect with indirect rdx setup.
			// Backward scan for pkey_mprotect stops at mov %rsi, %rdx (indirect) → exec_unknown.
			// Backward scan for mprotect finds mov $0x3, %rdx → exec_not_set.
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov eax, 10 (mprotect)
				0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
				0x0f, 0x05, // syscall (mprotect)
				0x48, 0x89, 0xf2, // mov %rsi, %rdx (indirect → pkey_mprotect will be exec_unknown)
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329 (pkey_mprotect)
				0x0f, 0x05, // syscall (pkey_mprotect)
			},
			wantEntries: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecUnknown},
			},
		},
		{
			name: "only mprotect detected",
			// mprotect only, no pkey_mprotect
			code: []byte{
				0xb8, 0x0a, 0x00, 0x00, 0x00, // mov eax, 10 (mprotect)
				0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
				0x0f, 0x05, // syscall (mprotect)
			},
			wantEntries: []common.SyscallArgEvalResult{
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecConfirmed},
			},
		},
		{
			name: "only pkey_mprotect detected",
			// pkey_mprotect only, no mprotect
			code: []byte{
				0xb8, 0x49, 0x01, 0x00, 0x00, // mov eax, 329 (pkey_mprotect)
				0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
				0x0f, 0x05, // syscall (pkey_mprotect)
			},
			wantEntries: []common.SyscallArgEvalResult{
				{SyscallName: "pkey_mprotect", Status: common.SyscallArgEvalExecConfirmed},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.analyzeSyscallsInCode(tt.code, baseAddr, decoder, table, nil)

			assert.Equal(t, len(tt.wantEntries), len(result.ArgEvalResults),
				"ArgEvalResults length mismatch")

			// Build a map for order-independent comparison
			resultMap := make(map[string]common.SyscallArgEvalStatus)
			for _, r := range result.ArgEvalResults {
				resultMap[r.SyscallName] = r.Status
			}

			for _, want := range tt.wantEntries {
				gotStatus, ok := resultMap[want.SyscallName]
				assert.True(t, ok, "expected entry with SyscallName=%q", want.SyscallName)
				if ok {
					assert.Equal(t, want.Status, gotStatus,
						"status mismatch for SyscallName=%q", want.SyscallName)
				}
			}
		})
	}
}
