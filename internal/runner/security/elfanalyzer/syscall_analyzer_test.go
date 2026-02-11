//go:build test

package elfanalyzer

import (
	"testing"

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

func (m *MockMachineCodeDecoder) ModifiesEAXorRAX(_ DecodedInstruction) bool {
	return false
}

func (m *MockMachineCodeDecoder) IsImmediateMove(_ DecodedInstruction) (bool, int64) {
	return false, 0
}

func (m *MockMachineCodeDecoder) IsControlFlowInstruction(_ DecodedInstruction) bool {
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
			wantMethod: "immediate",
		},
		{
			name: "immediate with unrelated instruction",
			// mov $0x2a, %eax; mov %rsi, %rdi; syscall
			code:       []byte{0xb8, 0x2a, 0x00, 0x00, 0x00, 0x48, 0x89, 0xf7, 0x0f, 0x05},
			wantNumber: 42,
			wantMethod: "immediate",
		},
		{
			name: "register move (indirect)",
			// mov %ebx, %eax; syscall
			code:       []byte{0x89, 0xd8, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: "unknown:indirect_setting",
		},
		{
			name: "control flow boundary",
			// mov $0x29, %eax; jmp label(+5); syscall
			// When backwardScanForSyscallNumber scans backward from syscall,
			// it encounters jmp first, which creates a control flow boundary.
			code:       []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0xeb, 0x05, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: "unknown:control_flow_boundary",
		},
		{
			name: "syscall only (no eax modification)",
			code: []byte{0x0f, 0x05},
			// With only the syscall instruction and no prior instructions,
			// the decode window is empty [0, 0), so decode_failed is returned.
			wantNumber: -1,
			wantMethod: "unknown:decode_failed",
		},
		{
			name: "memory load to eax (indirect)",
			// mov (%rsp), %eax; syscall
			code:       []byte{0x8b, 0x04, 0x24, 0x0f, 0x05},
			wantNumber: -1,
			wantMethod: "unknown:indirect_setting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyscallAnalyzer()
			result, err := analyzer.analyzeSyscallsInCode(tt.code, 0)
			require.NoError(t, err)
			require.Len(t, result.DetectedSyscalls, 1)

			info := result.DetectedSyscalls[0]
			assert.Equal(t, tt.wantNumber, info.Number)
			assert.Equal(t, tt.wantMethod, info.DeterminationMethod)
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
			wantMethod: "unknown:indirect_setting",
		},
		{
			name:       "control flow boundary is high risk",
			code:       []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0xeb, 0x05, 0x0f, 0x05}, // mov $0x29, %eax; jmp label(+5); syscall
			wantMethod: "unknown:control_flow_boundary",
		},
		{
			name:       "decode failed is high risk",
			code:       []byte{0x0f, 0x05},
			wantMethod: "unknown:decode_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyscallAnalyzer()
			result, err := analyzer.analyzeSyscallsInCode(tt.code, 0)
			require.NoError(t, err)
			require.Len(t, result.DetectedSyscalls, 1)

			info := result.DetectedSyscalls[0]
			assert.Equal(t, -1, info.Number)
			assert.Equal(t, tt.wantMethod, info.DeterminationMethod)
			assert.True(t, result.HasUnknownSyscalls)
			assert.True(t, result.Summary.IsHighRisk)
			assert.NotEmpty(t, result.HighRiskReasons)
		})
	}
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
	result, err := analyzer.analyzeSyscallsInCode(code, 0)
	require.NoError(t, err)

	require.Len(t, result.DetectedSyscalls, 2)

	// First syscall: socket (41)
	assert.Equal(t, 41, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "socket", result.DetectedSyscalls[0].Name)
	assert.True(t, result.DetectedSyscalls[0].IsNetwork)
	assert.Equal(t, "immediate", result.DetectedSyscalls[0].DeterminationMethod)

	// Second syscall: connect (42)
	assert.Equal(t, 42, result.DetectedSyscalls[1].Number)
	assert.Equal(t, "connect", result.DetectedSyscalls[1].Name)
	assert.True(t, result.DetectedSyscalls[1].IsNetwork)
	assert.Equal(t, "immediate", result.DetectedSyscalls[1].DeterminationMethod)

	// Summary
	assert.Equal(t, 2, result.Summary.TotalDetectedEvents)
	assert.Equal(t, 2, result.Summary.NetworkSyscallCount)
	assert.True(t, result.Summary.HasNetworkSyscalls)
	assert.False(t, result.Summary.IsHighRisk)
	assert.False(t, result.HasUnknownSyscalls)
}

func TestSyscallAnalyzer_NoSyscalls(t *testing.T) {
	// nop; nop; ret
	code := []byte{0x90, 0x90, 0xc3}

	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.analyzeSyscallsInCode(code, 0)
	require.NoError(t, err)

	assert.Empty(t, result.DetectedSyscalls)
	assert.Equal(t, 0, result.Summary.TotalDetectedEvents)
	assert.Equal(t, 0, result.Summary.NetworkSyscallCount)
	assert.False(t, result.Summary.HasNetworkSyscalls)
	assert.False(t, result.Summary.IsHighRisk)
	assert.False(t, result.HasUnknownSyscalls)
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
	result, err := analyzer.analyzeSyscallsInCode(code, 0)
	require.NoError(t, err)

	require.Len(t, result.DetectedSyscalls, 2)

	// First: write (non-network)
	assert.Equal(t, 1, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "write", result.DetectedSyscalls[0].Name)
	assert.False(t, result.DetectedSyscalls[0].IsNetwork)

	// Second: socket (network)
	assert.Equal(t, 41, result.DetectedSyscalls[1].Number)
	assert.Equal(t, "socket", result.DetectedSyscalls[1].Name)
	assert.True(t, result.DetectedSyscalls[1].IsNetwork)

	assert.Equal(t, 2, result.Summary.TotalDetectedEvents)
	assert.Equal(t, 1, result.Summary.NetworkSyscallCount)
	assert.True(t, result.Summary.HasNetworkSyscalls)
	assert.False(t, result.Summary.IsHighRisk)
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
	result, err := analyzer.analyzeSyscallsInCode(code, 0)
	require.NoError(t, err)

	require.Len(t, result.DetectedSyscalls, 2)

	// First: socket (known)
	assert.Equal(t, 41, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "immediate", result.DetectedSyscalls[0].DeterminationMethod)

	// Second: unknown (indirect)
	assert.Equal(t, -1, result.DetectedSyscalls[1].Number)
	assert.Equal(t, "unknown:indirect_setting", result.DetectedSyscalls[1].DeterminationMethod)

	// Overall result should be high risk because of unknown syscall
	assert.True(t, result.HasUnknownSyscalls)
	assert.True(t, result.Summary.IsHighRisk)
	assert.Equal(t, 1, result.Summary.NetworkSyscallCount)
}

func TestSyscallAnalyzer_WithBaseAddress(t *testing.T) {
	// mov $0x29, %eax; syscall
	code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05}
	baseAddr := uint64(0x401000)

	analyzer := NewSyscallAnalyzer()
	result, err := analyzer.analyzeSyscallsInCode(code, baseAddr)
	require.NoError(t, err)

	require.Len(t, result.DetectedSyscalls, 1)
	assert.Equal(t, 41, result.DetectedSyscalls[0].Number)
	assert.Equal(t, baseAddr+5, result.DetectedSyscalls[0].Location) // syscall at offset 5
}

func TestSyscallAnalyzer_InvalidOffset(t *testing.T) {
	// Test extractSyscallInfo with invalid offsets
	analyzer := NewSyscallAnalyzer()
	code := []byte{0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05}

	// syscallAddr < baseAddr
	info := analyzer.extractSyscallInfo(code, 0, 100)
	assert.Equal(t, -1, info.Number)
	assert.Equal(t, DeterminationMethodUnknownInvalidOffset, info.DeterminationMethod)

	// syscallAddr beyond code length
	info = analyzer.extractSyscallInfo(code, 200, 0)
	assert.Equal(t, -1, info.Number)
	assert.Equal(t, DeterminationMethodUnknownInvalidOffset, info.DeterminationMethod)
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
			locs := analyzer.findSyscallInstructions(tt.code, tt.baseAddr)
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
	instructions := analyzer.decodeInstructionsInWindow(code, 0, 0, 6)
	require.Len(t, instructions, 2) // mov (5 bytes) + nop (1 byte)

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

func TestSyscallAnalyzer_ScanLimitExceeded(t *testing.T) {
	// Create a code sequence with many non-eax instructions followed by syscall.
	// Use maxBackwardScan = 3 to make the test small.
	// nop; nop; nop; nop; syscall
	code := []byte{
		0x90, 0x90, 0x90, 0x90, // 4 nops (none modify eax)
		0x0f, 0x05, // syscall
	}

	analyzer := NewSyscallAnalyzerWithConfig(NewX86Decoder(), NewX86_64SyscallTable(), 3)
	result, err := analyzer.analyzeSyscallsInCode(code, 0)
	require.NoError(t, err)
	require.Len(t, result.DetectedSyscalls, 1)

	assert.Equal(t, -1, result.DetectedSyscalls[0].Number)
	assert.Equal(t, "unknown:scan_limit_exceeded", result.DetectedSyscalls[0].DeterminationMethod)
	assert.True(t, result.Summary.IsHighRisk)
}

func TestSyscallAnalyzer_DecodeInstructionsInWindow_NonPositiveLength(t *testing.T) {
	// Test that decodeInstructionsInWindow guards against decoders returning
	// non-positive instruction lengths, which could cause infinite loops.
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

	// This should not hang (infinite loop) despite the decoder returning Len=0.
	// The decodeInstructionsInWindow method should skip problematic bytes.
	instructions := analyzer.decodeInstructionsInWindow(code, 0, 0, 3)

	// We expect it to skip all bytes because the mock decoder always returns Len=0.
	// With the guard in place, it increments pos and continues, eventually exiting.
	assert.Len(t, instructions, 0)
}

func TestSyscallAnalyzer_DecodeInstructionsInWindow_NegativeLength(t *testing.T) {
	// Test that decodeInstructionsInWindow handles negative instruction lengths.
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

	// This should not hang despite the decoder returning Len=-1.
	instructions := analyzer.decodeInstructionsInWindow(code, 0, 0, 3)

	// We expect it to skip all bytes and return empty.
	assert.Len(t, instructions, 0)
}
