# 詳細仕様書: ELF 機械語解析による syscall 静的解析

## 0. 既存機能活用方針

この実装では、重複開発を避け既存のインフラを最大限活用します：

- **elfanalyzer パッケージ**: ELF ファイルのオープン、セクション解析、AnalysisOutput 構造体
  - `StandardELFAnalyzer`: 静的バイナリ検出時のフォールバック先として拡張
  - `AnalysisOutput`, `AnalysisResult`: 既存の結果型を再利用
- **filevalidator パッケージ**: ハッシュ計算、パス生成
  - `HybridHashFilePathGetter`: 解析結果ファイルパス生成
  - `SHA256`: ハッシュアルゴリズム
- **safefileio パッケージ**: 安全なファイル I/O
  - シンボリックリンク攻撃への防御

これにより**実装工数を削減**し、**実証済みセキュリティ機能を継承**できます。

## 1. パッケージ構成詳細

```
# 既存コンポーネント（再利用・拡張）
internal/runner/security/elfanalyzer/
    analyzer.go                    # AnalysisResult, AnalysisOutput（既存）
    standard_analyzer.go           # StandardELFAnalyzer（拡張）
    network_symbols.go             # ネットワークシンボル定義（既存）

# 新規コンポーネント
internal/runner/security/elfanalyzer/
    syscall_analyzer.go            # SyscallAnalyzer
    syscall_decoder.go             # MachineCodeDecoder, X86Decoder
    syscall_numbers.go             # SyscallNumberTable, X86_64SyscallTable
    go_wrapper_resolver.go         # GoWrapperResolver
    pclntab_parser.go              # PclntabParser

internal/fileanalysis/
    file_analysis_store.go         # FileAnalysisStore（解析結果ストア層）
    schema.go                      # FileAnalysisRecord, 統合スキーマ定義
    errors.go                      # 解析結果ストア関連エラー
    syscall_store.go               # SyscallAnalysisStore インターフェース実装

# 拡張対象
cmd/record/
    main.go                        # --analyze-syscalls オプション追加

internal/filevalidator/
    validator.go                   # 統合解析結果ストア対応（拡張）

# 既存コンポーネント（依存）
internal/filevalidator/
    hybrid_hash_path_getter.go     # パス生成（既存）
    hash_algo.go                   # SHA256（既存）
```

## 2. 型定義とインターフェース

### 2.1 SyscallAnalyzer

```go
// internal/runner/security/elfanalyzer/syscall_analyzer.go

package elfanalyzer

import (
    "bytes"
    "debug/elf"
    "fmt"
    "log/slog"
)

// SyscallAnalysisResult represents the result of syscall analysis.
type SyscallAnalysisResult struct {
    // DetectedSyscalls contains all detected syscall events with their numbers.
    // This includes both direct syscall instructions (opcode 0F 05) and
    // indirect syscalls via Go wrapper function calls (e.g., syscall.Syscall).
    DetectedSyscalls []SyscallInfo

    // HasUnknownSyscalls indicates whether any syscall number could not be determined.
    HasUnknownSyscalls bool

    // HighRiskReasons explains why the analysis resulted in high risk, if applicable.
    HighRiskReasons []string

    // Summary provides aggregated information about the analysis.
    Summary SyscallSummary
}

// SyscallInfo represents information about a single detected syscall event.
// An event can be either a direct syscall instruction or an indirect syscall
// via a Go wrapper function call.
type SyscallInfo struct {
    // Number is the syscall number (e.g., 41 for socket on x86_64).
    // -1 indicates the number could not be determined.
    Number int

    // Name is the human-readable syscall name (e.g., "socket").
    // Empty if the number is unknown or not in the table.
    Name string

    // IsNetwork indicates whether this syscall is network-related.
    IsNetwork bool

    // Location is the virtual address of the syscall instruction
    // (typically located within the .text section).
    Location uint64

    // DeterminationMethod describes how the syscall number was determined.
    // Possible values:
    // - "immediate"
    // - "go_wrapper"
    // - "unknown" or "unknown:<reason>" (e.g., "unknown:decode_failed",
    //   "unknown:control_flow_boundary", "unknown:indirect_setting",
    //   "unknown:scan_limit_exceeded", "unknown:invalid_offset")
    DeterminationMethod string
}

// SyscallSummary provides aggregated analysis information.
type SyscallSummary struct {
    // HasNetworkSyscalls indicates presence of network-related syscalls.
    HasNetworkSyscalls bool

    // IsHighRisk indicates the analysis could not fully determine network capability.
    IsHighRisk bool

    // TotalDetectedEvents is the count of detected syscall events.
    // This includes both direct syscall instructions and indirect syscalls
    // via Go wrapper function calls.
    TotalDetectedEvents int

    // NetworkSyscallCount is the count of network-related syscall events.
    NetworkSyscallCount int
}

// SyscallAnalyzer analyzes ELF binaries for syscall instructions.
//
// Security Note: This analyzer is designed to work with pre-opened *elf.File
// instances. The caller is responsible for opening files securely using
// safefileio.SafeOpenFile() followed by elf.NewFile(). This design ensures
// TOCTOU safety and symlink attack prevention, consistent with the existing
// StandardELFAnalyzer pattern.
type SyscallAnalyzer struct {
    decoder      MachineCodeDecoder
    syscallTable SyscallNumberTable
    goResolver   *GoWrapperResolver

    // maxBackwardScan is the maximum number of instructions to scan backward
    // from a syscall instruction to find the syscall number.
    maxBackwardScan int
}

// NewSyscallAnalyzer creates a new SyscallAnalyzer with default settings.
func NewSyscallAnalyzer() *SyscallAnalyzer {
    return &SyscallAnalyzer{
        decoder:         NewX86Decoder(),
        syscallTable:    NewX86_64SyscallTable(),
        goResolver:      NewGoWrapperResolver(),
        maxBackwardScan: 50, // Default: scan up to 50 instructions backward
    }
}

// NewSyscallAnalyzerWithConfig creates a SyscallAnalyzer with custom configuration.
func NewSyscallAnalyzerWithConfig(decoder MachineCodeDecoder, table SyscallNumberTable, maxScan int) *SyscallAnalyzer {
    return &SyscallAnalyzer{
        decoder:         decoder,
        syscallTable:    table,
        goResolver:      NewGoWrapperResolver(),
        maxBackwardScan: maxScan,
    }
}

// AnalyzeSyscalls analyzes the given ELF file for syscall instructions.
// Returns SyscallAnalysisResult containing all found syscalls and risk assessment.
//
// Note: This method accepts an *elf.File that has already been opened securely.
// The caller is responsible for using safefileio.SafeOpenFile() to prevent
// symlink attacks and TOCTOU race conditions, then wrapping with elf.NewFile().
// See StandardELFAnalyzer.AnalyzeNetworkSymbols() for the recommended pattern.
func (a *SyscallAnalyzer) AnalyzeSyscallsFromELF(elfFile *elf.File) (*SyscallAnalysisResult, error) {

    // Verify architecture
    if elfFile.Machine != elf.EM_X86_64 {
        return nil, &UnsupportedArchitectureError{
            Machine: elfFile.Machine,
        }
    }

    // Load .text section
    textSection := elfFile.Section(".text")
    if textSection == nil {
        return nil, ErrNoTextSection
    }

    code, err := textSection.Data()
    if err != nil {
        return nil, fmt.Errorf("failed to read .text section: %w", err)
    }

    // Load symbols for Go wrapper resolution
    if err := a.goResolver.LoadSymbols(elfFile); err != nil {
        // Non-fatal: continue without Go wrapper resolution
        // This handles stripped binaries
        slog.Debug("failed to load symbols for Go wrapper resolution",
            slog.String("error", err.Error()))
    }

    // Analyze syscalls
    return a.analyzeSyscallsInCode(code, textSection.Addr)
}

// analyzeSyscallsInCode performs the actual syscall analysis on code bytes.
// This method uses two separate analysis passes:
//   1. Direct syscall instruction analysis (syscall opcode 0F 05)
//   2. Go wrapper call analysis (calls to syscall.Syscall, etc.)
func (a *SyscallAnalyzer) analyzeSyscallsInCode(code []byte, baseAddr uint64) (*SyscallAnalysisResult, error) {
    result := &SyscallAnalysisResult{
        DetectedSyscalls: make([]SyscallInfo, 0),
    }

    // Pass 1: Analyze direct syscall instructions
    syscallLocs := a.findSyscallInstructions(code, baseAddr)
    for _, loc := range syscallLocs {
        info := a.extractSyscallInfo(code, loc, baseAddr)
        result.DetectedSyscalls = append(result.DetectedSyscalls, info)

        if info.Number == -1 {
            result.HasUnknownSyscalls = true
            result.HighRiskReasons = append(result.HighRiskReasons,
                fmt.Sprintf("syscall at 0x%x: number could not be determined (%s)",
                    info.Location, info.DeterminationMethod))
        }

        if info.IsNetwork {
            result.Summary.NetworkSyscallCount++
        }
    }

    // Pass 2: Analyze Go wrapper calls (if symbols are available)
    if a.goResolver != nil && a.goResolver.HasSymbols() {
        wrapperCalls := a.goResolver.FindWrapperCalls(code, baseAddr)
        for _, call := range wrapperCalls {
            info := SyscallInfo{
                Number:              call.SyscallNumber,
                Location:            call.CallSiteAddress,
                DeterminationMethod: "go_wrapper",
            }

            if call.SyscallNumber >= 0 {
                info.Name = a.syscallTable.GetSyscallName(call.SyscallNumber)
                info.IsNetwork = a.syscallTable.IsNetworkSyscall(call.SyscallNumber)
            } else {
                result.HasUnknownSyscalls = true
                result.HighRiskReasons = append(result.HighRiskReasons,
                    fmt.Sprintf("go wrapper call at 0x%x: syscall number could not be determined",
                        call.CallSiteAddress))
            }

            result.DetectedSyscalls = append(result.DetectedSyscalls, info)

            if info.IsNetwork {
                result.Summary.NetworkSyscallCount++
            }
        }
    }

    // Build summary
    result.Summary.TotalDetectedEvents = len(result.DetectedSyscalls)
    result.Summary.HasNetworkSyscalls = result.Summary.NetworkSyscallCount > 0
    result.Summary.IsHighRisk = result.HasUnknownSyscalls

    return result, nil
}

// findSyscallInstructions scans the code for syscall instructions (0F 05).
func (a *SyscallAnalyzer) findSyscallInstructions(code []byte, baseAddr uint64) []uint64 {
    var locations []uint64

    pattern := []byte{0x0F, 0x05}
    if len(code) < len(pattern) {
        return locations
    }

    for i := 0; i <= len(code)-len(pattern); {
        idx := bytes.Index(code[i:], pattern)
        if idx == -1 {
            break
        }
        pos := i + idx
        locations = append(locations, baseAddr+uint64(pos))
        i = pos + 1
    }

    return locations
}

// extractSyscallInfo extracts syscall number by backward scanning.
func (a *SyscallAnalyzer) extractSyscallInfo(code []byte, syscallAddr uint64, baseAddr uint64) SyscallInfo {
    info := SyscallInfo{
        Number:   -1,
        Location: syscallAddr,
    }

    // Calculate offset in code
    offset := int(syscallAddr - baseAddr)
    if offset < 0 || offset >= len(code) {
        info.DeterminationMethod = "unknown:invalid_offset"
        return info
    }

    // Backward scan to find eax/rax modification
    number, method := a.backwardScanForSyscallNumber(code, offset)
    info.Number = number
    info.DeterminationMethod = method

    if number >= 0 {
        info.Name = a.syscallTable.GetSyscallName(number)
        info.IsNetwork = a.syscallTable.IsNetworkSyscall(number)
    }

    return info
}

// backwardScanForSyscallNumber scans backward from syscall instruction
// to find where eax/rax is set.
// Note: This method only handles direct syscall instructions.
// Go wrapper calls are analyzed separately by analyzeGoWrapperCalls.
func (a *SyscallAnalyzer) backwardScanForSyscallNumber(code []byte, syscallOffset int) (int, string) {
    // Performance optimization: Use windowed decoding to avoid re-decoding
    // the entire .text section for each syscall instruction.
    // Window starts from max(0, syscallOffset - maxBackwardScan * maxInstructionLength)
    const maxInstructionLength = 15 // x86_64 maximum instruction length
    windowStart := syscallOffset - (a.maxBackwardScan * maxInstructionLength)
    if windowStart < 0 {
        windowStart = 0
    }

    // Build instruction list by forward decoding within the window
    instructions := a.decodeInstructionsInWindow(code, windowStart, syscallOffset)
    if len(instructions) == 0 {
        return -1, "unknown:decode_failed"
    }

    // Scan backward through decoded instructions
    scanCount := 0
    for i := len(instructions) - 1; i >= 0 && scanCount < a.maxBackwardScan; i-- {
        inst := instructions[i]
        scanCount++

        // Check for control flow instruction (basic block boundary)
        if a.decoder.IsControlFlowInstruction(inst) {
            return -1, "unknown:control_flow_boundary"
        }

        // Check if this instruction modifies eax/rax
        if !a.decoder.ModifiesEAXorRAX(inst) {
            continue
        }

        // Check if it's an immediate move
        if isImm, value := a.decoder.IsImmediateMove(inst); isImm {
            return int(value), "immediate"
        }

        // Non-immediate modification found (register move, memory load, etc.)
        return -1, "unknown:indirect_setting"
    }

    // Reached scan limit without finding eax/rax modification
    return -1, "unknown:scan_limit_exceeded"
}

// decodeInstructionsInWindow decodes instructions within a specified window [startOffset, endOffset).
// This method provides better performance by avoiding unnecessary decoding of the entire code section.
// For large binaries with many syscall instructions, this reduces total decode overhead significantly.
//
// Performance comparison (example: 10MB .text, 100 syscalls):
// - Old approach: 100 × 5MB avg = ~500MB worth of redundant decoding
// - Window approach: 100 × (50 instructions × 15 bytes) = ~75KB of focused decoding
func (a *SyscallAnalyzer) decodeInstructionsInWindow(code []byte, startOffset, endOffset int) []DecodedInstruction {
    var instructions []DecodedInstruction
    pos := startOffset

    for pos < endOffset {
        inst, err := a.decoder.Decode(code[pos:], uint64(pos))
        if err != nil {
            // Skip problematic byte and continue
            pos++
            continue
        }
        instructions = append(instructions, inst)
        pos += inst.Len
    }

    return instructions
}
```

#### 2.1.1 パフォーマンス最適化: ウィンドウ方式のデコード

**問題**: 各 syscall 命令に対して offset 0 からデコードするナイーブな実装は、大規模バイナリのパフォーマンス低下を招く。

**シナリオ例**:
- `.text` セクションサイズ: 10MB
- syscall 命令数: 100箇所
- 平均 syscall 位置: 開始地点から ~5MB

**パフォーマンスへの影響**:
- **旧方式**: 100 × 5MB ≈ 500MB相当の冗長なデコード処理
- **新方式（ウィンドウ）**: 100 × (50命令 × 15バイト最大) ≈ 75KBの集中デコード
- **改善効果**: デコードバイト数で ~6700倍削減

**実装方針**:

ウィンドウ方式（案 b）を選択した理由:

| 案 | メリット | デメリット | 決定 |
|----|---------|----------|------|
| (a) セクション全体を一度デコード | シンプル、メモリ予測可能 | 初期コスト大、初期 syscall で無駄 | ❌ 却下 |
| (b) ウィンドウ方式 | バランス型パフォーマンス、低メモリ | ロジック若干複雑 | ✅ **採用** |
| (c) .gopclntab 関数境界を利用 | 最も正確なスコープ | DWARF/pclntab 解析必須、複雑 | ❌ 過複雑 |

**ウィンドウ計算**:
```
windowStart = max(0, syscallOffset - maxBackwardScan × maxInstructionLength)
windowEnd   = syscallOffset
```

各パラメータ:
- `maxBackwardScan = 50`（設定可能、コンストラクタのデフォルト値）
- `maxInstructionLength = 15`（x86_64 最大命令長）
- ウィンドウサイズ ≈ 750バイト（通常 20-40 命令程度）

**パフォーマンス検証**:

この最適化は NFR-4.1.2（中規模バイナリ < 5秒）の達成に必須:

| バイナリサイズ | 予想 syscall数 | ウィンドウ方式時間 | ナイーブ方式時間 | 状態 |
|---------------|-----------------|---------------------|-----------------|------|
| < 1MB         | ~10-20         | < 0.1秒            | < 0.5秒        | ✅ 両方OK |
| 1-10MB        | ~50-100        | < 1秒              | ~10-30秒       | ✅ ウィンドウ必須 |
| > 10MB        | ~100-500       | < 5秒              | ~60-300秒      | ✅ ウィンドウ重要 |

### 2.2 MachineCodeDecoder

```go
// internal/runner/security/elfanalyzer/syscall_decoder.go

package elfanalyzer

import (
    "golang.org/x/arch/x86/x86asm"
)

// DecodedInstruction represents a decoded x86_64 instruction.
type DecodedInstruction struct {
    // Offset is the instruction's virtual address (e.g., section base VA plus
    // section-relative offset) corresponding to the first byte of this
    // instruction.
    Offset uint64

    // Len is the instruction length in bytes.
    Len int

    // Op is the instruction opcode (e.g., MOV, SYSCALL).
    Op x86asm.Op

    // Args are the instruction arguments.
    Args []x86asm.Arg

    // Raw contains the raw instruction bytes.
    Raw []byte
}

// MachineCodeDecoder defines the interface for decoding machine code.
type MachineCodeDecoder interface {
    // Decode decodes a single instruction at the given offset.
    Decode(code []byte, offset uint64) (DecodedInstruction, error)

    // IsSyscallInstruction returns true if the instruction is a syscall.
    IsSyscallInstruction(inst DecodedInstruction) bool

    // ModifiesEAXorRAX returns true if the instruction modifies eax or rax.
    ModifiesEAXorRAX(inst DecodedInstruction) bool

    // IsImmediateMove returns (true, value) if the instruction moves an immediate to eax/rax.
    IsImmediateMove(inst DecodedInstruction) (bool, int64)

    // IsControlFlowInstruction returns true if the instruction is a control flow instruction.
    IsControlFlowInstruction(inst DecodedInstruction) bool
}

// X86Decoder implements MachineCodeDecoder for x86_64.
type X86Decoder struct{}

// NewX86Decoder creates a new X86Decoder.
func NewX86Decoder() *X86Decoder {
    return &X86Decoder{}
}

// Decode decodes a single x86_64 instruction.
func (d *X86Decoder) Decode(code []byte, offset uint64) (DecodedInstruction, error) {
    inst, err := x86asm.Decode(code, 64) // 64-bit mode
    if err != nil {
        return DecodedInstruction{}, err
    }

    args := inst.Args[:]
    for len(args) > 0 && args[len(args)-1] == x86asm.None {
        args = args[:len(args)-1]
    }

    decoded := DecodedInstruction{
        Offset: offset,
        Len:    inst.Len,
        Op:     inst.Op,
        Args:   args,
        Raw:    code[:inst.Len],
    }

    return decoded, nil
}

// IsSyscallInstruction checks if the instruction is a syscall.
func (d *X86Decoder) IsSyscallInstruction(inst DecodedInstruction) bool {
    return inst.Op == x86asm.SYSCALL
}

// ModifiesEAXorRAX checks if the instruction modifies eax or rax.
func (d *X86Decoder) ModifiesEAXorRAX(inst DecodedInstruction) bool {
    if len(inst.Args) == 0 {
        return false
    }

    // Check destination register (first argument for most instructions)
    switch arg := inst.Args[0].(type) {
    case x86asm.Reg:
        return arg == x86asm.EAX || arg == x86asm.RAX ||
               arg == x86asm.AX || arg == x86asm.AL
    }

    return false
}

// IsImmediateMove checks if the instruction is a MOV immediate to eax/rax.
func (d *X86Decoder) IsImmediateMove(inst DecodedInstruction) (bool, int64) {
    // Check for MOV instruction
    if inst.Op != x86asm.MOV {
        return false, 0
    }

    // Need at least 2 arguments
    if len(inst.Args) < 2 {
        return false, 0
    }

    // Check destination is eax/rax
    destReg, ok := inst.Args[0].(x86asm.Reg)
    if !ok {
        return false, 0
    }
    if destReg != x86asm.EAX && destReg != x86asm.RAX {
        return false, 0
    }

    // Check source is immediate
    switch src := inst.Args[1].(type) {
    case x86asm.Imm:
        return true, int64(src)
    }

    return false, 0
}

// IsControlFlowInstruction checks if the instruction is a control flow instruction.
func (d *X86Decoder) IsControlFlowInstruction(inst DecodedInstruction) bool {
    switch inst.Op {
    case x86asm.JMP, x86asm.JA, x86asm.JAE, x86asm.JB, x86asm.JBE,
         x86asm.JE, x86asm.JG, x86asm.JGE, x86asm.JL, x86asm.JLE,
         x86asm.JNE, x86asm.JNO, x86asm.JNP, x86asm.JNS, x86asm.JO,
         x86asm.JP, x86asm.JS, x86asm.JCXZ, x86asm.JECXZ, x86asm.JRCXZ,
         x86asm.CALL, x86asm.RET, x86asm.IRET, x86asm.INT,
         x86asm.LOOP, x86asm.LOOPE, x86asm.LOOPNE:
        return true
    }
    return false
}
```

### 2.3 SyscallNumberTable

```go
// internal/runner/security/elfanalyzer/syscall_numbers.go

package elfanalyzer

// SyscallNumberTable defines the interface for syscall number lookup.
type SyscallNumberTable interface {
    // GetSyscallName returns the syscall name for the given number.
    // Returns empty string if the number is unknown.
    GetSyscallName(number int) string

    // IsNetworkSyscall returns true if the syscall is network-related.
    IsNetworkSyscall(number int) bool

    // GetNetworkSyscalls returns all network-related syscall numbers.
    GetNetworkSyscalls() []int
}

// SyscallDefinition defines a single syscall.
type SyscallDefinition struct {
    Number      int
    Name        string
    IsNetwork   bool
    Description string
}

// X86_64SyscallTable implements SyscallNumberTable for x86_64 Linux.
type X86_64SyscallTable struct {
    syscalls       map[int]SyscallDefinition
    networkNumbers []int
}

// NewX86_64SyscallTable creates a new syscall table for x86_64 Linux.
func NewX86_64SyscallTable() *X86_64SyscallTable {
    table := &X86_64SyscallTable{
        syscalls: make(map[int]SyscallDefinition),
    }

    // Network-related syscalls (as specified in requirements)
    networkSyscalls := []SyscallDefinition{
        {41, "socket", true, "Create a socket"},
        {42, "connect", true, "Connect to a remote address"},
        {43, "accept", true, "Accept a connection"},
        {288, "accept4", true, "Accept a connection with flags"},
        {44, "sendto", true, "Send data to address"},
        {45, "recvfrom", true, "Receive data from address"},
        {46, "sendmsg", true, "Send message"},
        {47, "recvmsg", true, "Receive message"},
        {49, "bind", true, "Bind to an address"},
        {50, "listen", true, "Listen for connections"},
    }

    for _, def := range networkSyscalls {
        table.syscalls[def.Number] = def
        table.networkNumbers = append(table.networkNumbers, def.Number)
    }

    // Common non-network syscalls (for reference/logging)
    nonNetworkSyscalls := []SyscallDefinition{
        {0, "read", false, "Read from file descriptor"},
        {1, "write", false, "Write to file descriptor"},
        {2, "open", false, "Open file"},
        {3, "close", false, "Close file descriptor"},
        {9, "mmap", false, "Map memory"},
        {11, "munmap", false, "Unmap memory"},
        {12, "brk", false, "Change data segment size"},
        {60, "exit", false, "Terminate process"},
        {231, "exit_group", false, "Terminate all threads"},
    }

    for _, def := range nonNetworkSyscalls {
        table.syscalls[def.Number] = def
    }

    return table
}

// GetSyscallName returns the syscall name for the given number.
func (t *X86_64SyscallTable) GetSyscallName(number int) string {
    if def, ok := t.syscalls[number]; ok {
        return def.Name
    }
    return ""
}

// IsNetworkSyscall returns true if the syscall is network-related.
func (t *X86_64SyscallTable) IsNetworkSyscall(number int) bool {
    if def, ok := t.syscalls[number]; ok {
        return def.IsNetwork
    }
    return false
}

// GetNetworkSyscalls returns all network-related syscall numbers.
func (t *X86_64SyscallTable) GetNetworkSyscalls() []int {
    return t.networkNumbers
}
```

### 2.4 PclntabParser

Go バイナリの `.gopclntab` セクションから関数情報を復元するパーサー。

**バージョン別サポート方針**:

| Go バージョン | pclntab magic | サポートレベル | 失敗時の動作 |
|--------------|---------------|---------------|-------------|
| Go 1.18+ | `0xFFFFFFF0` / `0xFFFFFFF1` | **正式サポート** | エラーを返す（解析バグの可能性） |
| Go 1.16-1.17 | `0xFFFFFFFA` | **ベストエフォート** | `ErrInvalidPclntab` を返し、Go wrapper 解析をスキップ（Pass 1 の直接 syscall 検出は継続） |
| Go 1.2-1.15 | `0xFFFFFFFB` | **ベストエフォート（legacy）** | 同上 |
| 上記以外 | 不明 | **非サポート** | `ErrUnsupportedPclntab` を返す |

本仕様では **Go 1.18+ の pclntab 形式に対して関数名・アドレスの抽出を正式に実装** する。
Go 1.16-1.17 および Go 1.2-1.15 の形式はベストエフォートとし、解析不能時は `ErrInvalidPclntab` を返す。
ベストエフォート対象のバージョンで解析が失敗した場合、SyscallAnalyzer は Go wrapper 解析（Pass 2）をスキップし、
直接 syscall 命令の検出（Pass 1）のみで結果を返す。

```go
// internal/runner/security/elfanalyzer/pclntab_parser.go

package elfanalyzer

import (
    "debug/elf"
    "encoding/binary"
    "errors"
    "fmt"
)

// pclntab magic numbers for different Go versions
const (
    pclntabMagicGo12  = 0xFFFFFFFB // Go 1.2 - 1.15
    pclntabMagicGo116 = 0xFFFFFFFA // Go 1.16 - 1.17
    pclntabMagicGo118 = 0xFFFFFFF0 // Go 1.18 - 1.19
    pclntabMagicGo120 = 0xFFFFFFF1 // Go 1.20+
)

// Errors
var (
    ErrNoPclntab          = errors.New("no .gopclntab section found")
    ErrUnsupportedPclntab = errors.New("unsupported pclntab format")
    ErrInvalidPclntab     = errors.New("invalid pclntab structure")
)

// PclntabFunc represents a function entry in pclntab.
type PclntabFunc struct {
    Name    string
    Entry   uint64 // Function entry address
    End     uint64 // Function end address (if available)
}

// PclntabParser parses Go's pclntab to extract function information.
type PclntabParser struct {
    ptrSize    int    // 4 or 8 bytes
    goVersion  string // Detected Go version range
    funcData   []PclntabFunc
}

// NewPclntabParser creates a new PclntabParser.
func NewPclntabParser() *PclntabParser {
    return &PclntabParser{
        funcData: make([]PclntabFunc, 0),
    }
}

// Parse reads the .gopclntab section and extracts function information.
// This works even on stripped binaries because Go runtime requires pclntab
// for stack traces and garbage collection.
func (p *PclntabParser) Parse(elfFile *elf.File) error {
    // Find .gopclntab section
    section := elfFile.Section(".gopclntab")
    if section == nil {
        return ErrNoPclntab
    }

    data, err := section.Data()
    if err != nil {
        return fmt.Errorf("failed to read .gopclntab: %w", err)
    }

    if len(data) < 8 {
        return ErrInvalidPclntab
    }

    // Read magic number (first 4 bytes, little-endian)
    magic := binary.LittleEndian.Uint32(data[0:4])

    switch magic {
    case pclntabMagicGo118, pclntabMagicGo120:
        // Go 1.18+ format - supported
        return p.parseGo118Plus(data)
    case pclntabMagicGo116:
        // Go 1.16-1.17 format - supported with limitations
        return p.parseGo116(data)
    case pclntabMagicGo12:
        // Go 1.2-1.15 format - legacy, limited support
        return p.parseGo12(data)
    default:
        return fmt.Errorf("%w: unknown magic 0x%08X", ErrUnsupportedPclntab, magic)
    }
}

// parseGo118Plus parses pclntab for Go 1.18 and later.
// Reference: https://go.dev/src/runtime/symtab.go
func (p *PclntabParser) parseGo118Plus(data []byte) error {
    if len(data) < 16 {
        return ErrInvalidPclntab
    }

    // Header layout for Go 1.18+:
    // [0:4]   magic
    // [4:5]   padding (0)
    // [5:6]   padding (0)
    // [6:7]   instruction size quantum (1 for x86, 4 for ARM)
    // [7:8]   pointer size (4 or 8)
    // [8:16]  nfunc (number of functions) - uint64 or uint32 depending on arch

    p.ptrSize = int(data[7])
    if p.ptrSize != 4 && p.ptrSize != 8 {
        return fmt.Errorf("%w: invalid pointer size %d", ErrInvalidPclntab, p.ptrSize)
    }

    p.goVersion = "go1.18+"

    // Parse function table
    // The structure varies by Go version, but function entries contain:
    // - entry PC (function start address)
    // - offset to function name in string table
    return p.parseFuncTable(data)
}

// parseGo116 parses pclntab for Go 1.16-1.17.
func (p *PclntabParser) parseGo116(data []byte) error {
    if len(data) < 16 {
        return ErrInvalidPclntab
    }

    p.ptrSize = int(data[7])
    if p.ptrSize != 4 && p.ptrSize != 8 {
        return fmt.Errorf("%w: invalid pointer size %d", ErrInvalidPclntab, p.ptrSize)
    }

    p.goVersion = "go1.16-1.17"
    return p.parseFuncTable(data)
}

// parseGo12 parses pclntab for Go 1.2-1.15 (legacy format).
func (p *PclntabParser) parseGo12(data []byte) error {
    if len(data) < 8 {
        return ErrInvalidPclntab
    }

    // Go 1.2-1.15 header:
    // [0:4]   magic
    // [4:5]   padding
    // [5:6]   padding
    // [6:7]   instruction size quantum
    // [7:8]   pointer size

    p.ptrSize = int(data[7])
    p.goVersion = "go1.2-1.15"
    return p.parseFuncTable(data)
}

// parseFuncTable extracts function entries from the pclntab.
// This implementation targets Go 1.18+ pclntab layout (pcHeader + functab).
// Legacy formats (Go 1.2-1.17) are best-effort and may return ErrInvalidPclntab.
func (p *PclntabParser) parseFuncTable(data []byte) error {
    // pcHeader layout (Go 1.16+)
    // offset 0x00: magic (uint32)
    // offset 0x04: pad1 (byte)
    // offset 0x05: pad2 (byte)
    // offset 0x06: minLC (byte)
    // offset 0x07: ptrSize (byte)
    // offset 0x08: nfunc (uint64/uint32)
    // offset 0x10: nfiles (uint64/uint32)
    // offset 0x18: textStart (uintptr)
    // offset 0x20: funcnameOffset (uintptr)
    // offset 0x28: cuOffset (uintptr)
    // offset 0x30: filetabOffset (uintptr)
    // offset 0x38: pctabOffset (uintptr)
    // offset 0x40: pclntabOffset (uintptr)
    // offset 0x48: ftabOffset (uintptr)

    if p.ptrSize != 4 && p.ptrSize != 8 {
        return fmt.Errorf("%w: invalid pointer size %d", ErrInvalidPclntab, p.ptrSize)
    }

    readPtr := func(off int) (uint64, error) {
        end := off + p.ptrSize
        if off < 0 || end > len(data) {
            return 0, ErrInvalidPclntab
        }
        if p.ptrSize == 4 {
            return uint64(binary.LittleEndian.Uint32(data[off:end])), nil
        }
        return binary.LittleEndian.Uint64(data[off:end]), nil
    }

    if len(data) < 0x50 {
        return ErrInvalidPclntab
    }

    nfunc, err := readPtr(0x08)
    if err != nil {
        return err
    }
    textStart, err := readPtr(0x18)
    if err != nil {
        return err
    }
    funcNameOff, err := readPtr(0x20)
    if err != nil {
        return err
    }
    ftabOff, err := readPtr(0x48)
    if err != nil {
        return err
    }

    // Function table: nfunc+1 entries, each entry is {entryoff uint32, funcoff uint32}
    // entry address = textStart + entryoff
    // funcoff points to _func struct; nameoff is at +4 in _func
    entrySize := 8
    ftabStart := int(ftabOff)
    ftabBytes := int((nfunc + 1) * uint64(entrySize))
    if ftabStart < 0 || ftabStart+ftabBytes > len(data) {
        return ErrInvalidPclntab
    }

    funcs := make([]PclntabFunc, 0, nfunc)
    readUint32 := func(b []byte, off int) (uint32, error) {
        if off < 0 || off+4 > len(b) {
            return 0, ErrInvalidPclntab
        }
        return binary.LittleEndian.Uint32(b[off : off+4]), nil
    }

    for i := uint64(0); i < nfunc; i++ {
        entryOff, err := readUint32(data, ftabStart+int(i)*entrySize)
        if err != nil {
            return err
        }
        funcOff, err := readUint32(data, ftabStart+int(i)*entrySize+4)
        if err != nil {
            return err
        }

        entry := uint64(entryOff) + textStart
        funcDataOff := int(funcOff)
        nameOff32, err := readUint32(data, funcDataOff+4)
        if err != nil {
            return err
        }

        nameStart := int(funcNameOff) + int(nameOff32)
        if nameStart < 0 || nameStart >= len(data) {
            return ErrInvalidPclntab
        }

        // Read null-terminated function name
        nameEnd := nameStart
        for nameEnd < len(data) && data[nameEnd] != 0x00 {
            nameEnd++
        }
        if nameEnd == len(data) {
            return ErrInvalidPclntab
        }
        name := string(data[nameStart:nameEnd])

        // Determine end address from next function entry (if available)
        end := uint64(0)
        if i+1 < nfunc {
            nextEntryOff, err := readUint32(data, ftabStart+int(i+1)*entrySize)
            if err != nil {
                return err
            }
            end = uint64(nextEntryOff) + textStart
        }

        funcs = append(funcs, PclntabFunc{
            Name:  name,
            Entry: entry,
            End:   end,
        })
    }

    p.funcData = funcs
    return nil
}

// GetFunctions returns all parsed function information.
func (p *PclntabParser) GetFunctions() []PclntabFunc {
    return p.funcData
}

// FindFunction finds a function by name.
func (p *PclntabParser) FindFunction(name string) (PclntabFunc, bool) {
    for _, f := range p.funcData {
        if f.Name == name {
            return f, true
        }
    }
    return PclntabFunc{}, false
}

// GetGoVersion returns the detected Go version range.
func (p *PclntabParser) GetGoVersion() string {
    return p.goVersion
}
```

### 2.5 GoWrapperResolver

Go の syscall ラッパー関数を解析し、呼び出し元で syscall 番号を特定。
`.gopclntab` から関数情報を取得する設計であり、
**Go 1.18+ の pclntab 形式であれば strip されたバイナリにも対応**。
Go 1.16-1.17 および Go 1.2-1.15 はベストエフォートとする（詳細は 2.4 節のバージョン別サポート方針を参照）。
pclntab 解析に失敗した場合、GoWrapperResolver はシンボルなしとして動作し、
Pass 2（Go wrapper 解析）がスキップされるが、Pass 1（直接 syscall 検出）には影響しない。

```go
// internal/runner/security/elfanalyzer/go_wrapper_resolver.go

package elfanalyzer

import (
    "debug/elf"
    "strings"

    "golang.org/x/arch/x86/x86asm"
)

// GoSyscallWrapper represents a known Go syscall wrapper function.
type GoSyscallWrapper struct {
    Name            string
    SyscallArgIndex int // Which argument contains the syscall number (0-based)
}

// knownGoWrappers lists standard Go syscall wrapper functions.
var knownGoWrappers = []GoSyscallWrapper{
    {"syscall.Syscall", 0},
    {"syscall.Syscall6", 0},
    {"syscall.RawSyscall", 0},
    {"syscall.RawSyscall6", 0},
    {"runtime.syscall", 0},
    {"runtime.syscall6", 0},
}

// SymbolInfo represents information about a symbol in the ELF file.
type SymbolInfo struct {
    Name    string
    Address uint64
    Size    uint64
}

// WrapperCall represents a call to a Go syscall wrapper function.
type WrapperCall struct {
    // CallSiteAddress is the address of the CALL instruction.
    CallSiteAddress uint64

    // TargetFunction is the name of the wrapper function being called.
    TargetFunction string

    // SyscallNumber is the resolved syscall number, or -1 if unresolved.
    SyscallNumber int

    // Resolved indicates whether the syscall number was successfully determined.
    Resolved bool
}

// GoWrapperResolver resolves Go syscall wrapper calls to determine syscall numbers.
type GoWrapperResolver struct {
    symbols       map[string]SymbolInfo
    wrapperAddrs  map[uint64]GoSyscallWrapper
    hasSymbols    bool
    pclntabParser *PclntabParser
}

// NewGoWrapperResolver creates a new GoWrapperResolver.
func NewGoWrapperResolver() *GoWrapperResolver {
    return &GoWrapperResolver{
        symbols:       make(map[string]SymbolInfo),
        wrapperAddrs:  make(map[uint64]GoSyscallWrapper),
        pclntabParser: NewPclntabParser(),
    }
}

// LoadSymbols loads symbols from the .gopclntab section.
// The pclntab persists even after stripping because Go runtime needs it
// for stack traces and garbage collection.
//
// Returns error if .gopclntab is not available.
func (r *GoWrapperResolver) LoadSymbols(elfFile *elf.File) error {
    if err := r.loadFromPclntab(elfFile); err != nil {
        return err
    }

    r.hasSymbols = len(r.symbols) > 0
    return nil
}

// loadFromPclntab loads symbols from the .gopclntab section.
func (r *GoWrapperResolver) loadFromPclntab(elfFile *elf.File) error {
    if err := r.pclntabParser.Parse(elfFile); err != nil {
        return err
    }

    for _, fn := range r.pclntabParser.GetFunctions() {
        // Calculate size, guarding against missing/zero End to avoid underflow
        size := uint64(0)
        if fn.End > fn.Entry {
            size = fn.End - fn.Entry
        }

        r.symbols[fn.Name] = SymbolInfo{
            Name:    fn.Name,
            Address: fn.Entry,
            Size:    size,
        }

        // Check if this is a known Go wrapper
        for _, wrapper := range knownGoWrappers {
            if strings.Contains(fn.Name, wrapper.Name) {
                r.wrapperAddrs[fn.Entry] = wrapper
            }
        }
    }

    return nil
}

// HasSymbols returns true if symbols were successfully loaded.
func (r *GoWrapperResolver) HasSymbols() bool {
    return r.hasSymbols
}

// FindWrapperCalls scans the code section for calls to known Go syscall wrappers.
// This is a separate analysis pass from direct syscall instruction scanning.
//
// Parameters:
//   - code: the code section bytes
//   - baseAddr: base address of the code section
//
// Returns:
//   - slice of WrapperCall structs containing call site info and resolved syscall numbers
func (r *GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) []WrapperCall {
    if len(r.wrapperAddrs) == 0 {
        return nil
    }

    var results []WrapperCall
    decoder := NewX86Decoder()

    // Decode entire code section and find CALL instructions to known wrappers
    pos := 0
    var recentInstructions []DecodedInstruction
    maxRecentInstructions := 10 // Keep recent instructions for backward scan

    for pos < len(code) {
        inst, err := decoder.Decode(code[pos:], baseAddr+uint64(pos))
        if err != nil {
            pos++
            continue
        }

        // Keep track of recent instructions for backward scanning
        recentInstructions = append(recentInstructions, inst)
        if len(recentInstructions) > maxRecentInstructions {
            recentInstructions = recentInstructions[1:]
        }

        // Check if this is a CALL to a known wrapper
        if inst.Op == x86asm.CALL {
            wrapper, isWrapper := r.resolveWrapper(inst)
            if isWrapper {
                // Found a call to a wrapper, try to resolve the syscall number
                syscallNum := r.resolveSyscallArgument(recentInstructions, wrapper)
                results = append(results, WrapperCall{
                    CallSiteAddress: baseAddr + uint64(pos),
                    TargetFunction:  wrapper.Name,
                    SyscallNumber:   syscallNum,
                    Resolved:        syscallNum >= 0,
                })
            }
        }

        pos += inst.Len
    }

    return results
}

// resolveSyscallArgument analyzes instructions before a wrapper call
// to determine the syscall number argument.
//
// LIMITATION: This implementation ONLY supports Go 1.17+ register-based ABI
// where the first argument (syscall number) is passed in RAX.
// This is a known limited specification:
//   - Go 1.16 and earlier use stack-based ABI (not supported)
//   - Compiler optimizations or unusual wrapper patterns may place the
//     syscall number in a different register or via memory indirection
//   - Calls where the syscall number is not resolved are reported as
//     unknown (SyscallNumber = -1), triggering High Risk classification
//
// The target Go version should be fixed and validated with acceptance
// tests using real Go binaries compiled with the specific Go toolchain.
func (r *GoWrapperResolver) resolveSyscallArgument(recentInstructions []DecodedInstruction, wrapper GoSyscallWrapper) int {
    if len(recentInstructions) < 2 {
        return -1
    }

    // Currently only support arg index 0 (RAX for Go 1.17+ ABI)
    if wrapper.SyscallArgIndex != 0 {
        return -1
    }
    targetReg := x86asm.RAX

    decoder := NewX86Decoder()

    // Scan backward through recent instructions (excluding the CALL itself)
    for i := len(recentInstructions) - 2; i >= 0 && i >= len(recentInstructions)-6; i-- {
        inst := recentInstructions[i]

        // Stop at control flow boundary
        if decoder.IsControlFlowInstruction(inst) {
            break
        }

        // Check for immediate move to target register
        if isImm, value := decoder.IsImmediateMove(inst); isImm {
            if reg, ok := inst.Args[0].(x86asm.Reg); ok && reg == targetReg {
                return int(value)
            }
        }
    }

    return -1
}

// resolveWrapper checks if the instruction is a CALL to a known wrapper
// and returns the wrapper information if found.
func (r *GoWrapperResolver) resolveWrapper(inst DecodedInstruction) (GoSyscallWrapper, bool) {
    if inst.Op != x86asm.CALL {
        return GoSyscallWrapper{}, false
    }

    // Extract call target
    if len(inst.Args) == 0 {
        return GoSyscallWrapper{}, false
    }

    // For direct calls, check if target is a known wrapper
    switch target := inst.Args[0].(type) {
    case x86asm.Rel:
        // Relative call - calculate absolute address
        targetAddr := uint64(int64(inst.Offset) + int64(inst.Len) + int64(target))
        if wrapper, ok := r.wrapperAddrs[targetAddr]; ok {
            return wrapper, true
        }
    }

    return GoSyscallWrapper{}, false
}
```

**注記**: 上記コードでは `x86asm.Reg`, `x86asm.Rel`, `x86asm.Mem` などの型を使用しているため、パッケージ冒頭のインポートで `"golang.org/x/arch/x86/x86asm"` を追加する必要がある。

### 2.6 統合解析結果ストア層

統合解析結果ストアは、ハッシュ検証情報と syscall 解析結果を単一ファイルに格納する。
利用側（filevalidator, elfanalyzer）はそれぞれ自分の関心事のみを扱うインターフェースを通じてアクセスする。

**循環依存回避設計**:

`fileanalysis` パッケージは `filevalidator` パッケージの型に依存せず、独自のインターフェースを定義する。
`filevalidator.HybridHashFilePathGetter` は「たまたま同じメソッドを持つ」ため、このインターフェースを満たす。
これにより import cycle を回避しつつ、既存の実装を再利用できる。

```
┌─────────────────────────────────────────────────────────────────────────┐
│ fileanalysis パッケージ                                                  │
│   ┌─────────────────────────────────────────┐                          │
│   │ HashFilePathGetter interface            │                          │
│   │   GetHashFilePath(hash string) string   │                          │
│   └─────────────────────────────────────────┘                          │
│                   △ implements                                          │
└───────────────────│─────────────────────────────────────────────────────┘
                    │
                    │ (型の一致により自動的に実装)
                    │
┌───────────────────│─────────────────────────────────────────────────────┐
│ filevalidator パッケージ                                                 │
│   ┌─────────────────────────────────────────┐                          │
│   │ HybridHashFilePathGetter struct         │                          │
│   │   GetHashFilePath(hash string) string   │                          │
│   └─────────────────────────────────────────┘                          │
└─────────────────────────────────────────────────────────────────────────┘
```

#### 2.6.1 FileAnalysisStore

```go
// internal/fileanalysis/file_analysis_store.go

package fileanalysis

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/common"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// HashFilePathGetter generates file paths from content hashes.
// This interface is defined locally to avoid import cycles with filevalidator.
// filevalidator.HybridHashFilePathGetter implements this interface implicitly
// by having the same method signature.
type HashFilePathGetter interface {
    GetHashFilePath(hash string) string
}

// FileAnalysisStore manages unified file analysis record files containing both
// hash validation and syscall analysis data.
type FileAnalysisStore struct {
    analysisDir string
    pathGetter  HashFilePathGetter
}

// NewFileAnalysisStore creates a new FileAnalysisStore.
func NewFileAnalysisStore(analysisDir string, pathGetter HashFilePathGetter) (*FileAnalysisStore, error) {
    // Validate analysis result directory
    info, err := os.Lstat(analysisDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, fmt.Errorf("analysis result directory does not exist: %s", analysisDir)
        }
        return nil, fmt.Errorf("failed to access analysis result directory: %w", err)
    }
    if !info.IsDir() {
        return nil, fmt.Errorf("analysis result path is not a directory: %s", analysisDir)
    }

    return &FileAnalysisStore{
        analysisDir: analysisDir,
        pathGetter:  pathGetter,
    }, nil
}

// Load loads the analysis record for the given file path.
// Returns ErrRecordNotFound if the analysis record file does not exist.
func (s *FileAnalysisStore) Load(filePath string) (*FileAnalysisRecord, error) {
    recordPath, err := s.getRecordPath(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to get analysis record path: %w", err)
    }

    data, err := safefileio.SafeReadFile(recordPath)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrRecordNotFound
        }
        return nil, fmt.Errorf("failed to read analysis record file: %w", err)
    }

    var record FileAnalysisRecord
    if err := json.Unmarshal(data, &record); err != nil {
        return nil, &RecordCorruptedError{Path: recordPath, Cause: err}
    }

    // Validate schema version
    if record.SchemaVersion != CurrentSchemaVersion {
        return nil, &SchemaVersionMismatchError{
            Expected: CurrentSchemaVersion,
            Actual:   record.SchemaVersion,
        }
    }

    return &record, nil
}

// Save saves the analysis record for the given file path.
// This overwrites the entire record. Use Update for read-modify-write operations.
func (s *FileAnalysisStore) Save(filePath string, record *FileAnalysisRecord) error {
    recordPath, err := s.getRecordPath(filePath)
    if err != nil {
        return fmt.Errorf("failed to get analysis record path: %w", err)
    }

    record.SchemaVersion = CurrentSchemaVersion
    record.FilePath = filePath
    record.UpdatedAt = time.Now().UTC()

    data, err := json.MarshalIndent(record, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal analysis record: %w", err)
    }

    if err := safefileio.SafeWriteFileOverwrite(recordPath, data, 0o600); err != nil {
        return fmt.Errorf("failed to write analysis record file: %w", err)
    }

    return nil
}

// Update performs a read-modify-write operation on the analysis record.
// The updateFn receives the existing record (or a new empty one if not found)
// and should modify it in place.
func (s *FileAnalysisStore) Update(filePath string, updateFn func(*FileAnalysisRecord) error) error {
    // Try to load existing record
    record, err := s.Load(filePath)
    if err != nil {
        if err == ErrRecordNotFound {
            // Create new record
            record = &FileAnalysisRecord{}
        } else {
            // For other errors (corrupted, schema mismatch), create fresh record
            record = &FileAnalysisRecord{}
        }
    }

    // Apply update
    if err := updateFn(record); err != nil {
        return err
    }

    // Save updated record
    return s.Save(filePath, record)
}

// getRecordPath returns the analysis record file path for the given file.
func (s *FileAnalysisStore) getRecordPath(filePath string) (string, error) {
    absPath, err := filepath.Abs(filePath)
    if err != nil {
        return "", fmt.Errorf("failed to get absolute path: %w", err)
    }

    resolvedPath, err := common.NewResolvedPath(absPath)
    if err != nil {
        return "", fmt.Errorf("failed to create resolved path: %w", err)
    }

    return s.pathGetter.GetHashFilePath(s.analysisDir, resolvedPath)
}
```

#### 2.6.2 SyscallAnalysisStore（elfanalyzer 用インターフェース実装）

```go
// internal/fileanalysis/syscall_store.go

package fileanalysis

import (
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// SyscallAnalysisStore provides syscall analysis storage operations.
// This is a concrete adapter backed by FileAnalysisStore and used by elfanalyzer.
type SyscallAnalysisStore struct {
    store *FileAnalysisStore
}

// NewSyscallAnalysisStore creates a new SyscallAnalysisStore backed by FileAnalysisStore.
func NewSyscallAnalysisStore(store *FileAnalysisStore) *SyscallAnalysisStore {
    return &SyscallAnalysisStore{store: store}
}

// SaveSyscallAnalysis saves the syscall analysis result.
// This updates only the syscall_analysis field, preserving other fields.
func (c *SyscallAnalysisStore) SaveSyscallAnalysis(filePath, fileHash string, result *elfanalyzer.SyscallAnalysisResult) error {
    return c.store.Update(filePath, func(record *FileAnalysisRecord) error {
        record.ContentHash = fileHash
        record.SyscallAnalysis = &SyscallAnalysisData{
            Architecture:       "x86_64",
            AnalyzedAt:         time.Now().UTC(),
            DetectedSyscalls:   convertSyscallInfos(result.DetectedSyscalls),
            HasUnknownSyscalls: result.HasUnknownSyscalls,
            HighRiskReasons:    result.HighRiskReasons,
            Summary:            convertSummary(result.Summary),
        }
        return nil
    })
}

// LoadSyscallAnalysis loads the syscall analysis result.
// Returns (result, true, nil) if found and hash matches.
// Returns (nil, false, nil) if not found or hash mismatch.
// Returns (nil, false, error) on other errors.
func (c *SyscallAnalysisStore) LoadSyscallAnalysis(filePath, expectedHash string) (*elfanalyzer.SyscallAnalysisResult, bool, error) {
    record, err := c.store.Load(filePath)
    if err != nil {
        if err == ErrRecordNotFound {
            return nil, false, nil
        }
        return nil, false, err
    }

    // Check hash match
    if record.ContentHash != expectedHash {
        return nil, false, nil
    }

    // Check if syscall analysis exists
    if record.SyscallAnalysis == nil {
        return nil, false, nil
    }

    // Convert back to elfanalyzer types
    result := &elfanalyzer.SyscallAnalysisResult{
        DetectedSyscalls:   convertToSyscallInfos(record.SyscallAnalysis.DetectedSyscalls),
        HasUnknownSyscalls: record.SyscallAnalysis.HasUnknownSyscalls,
        HighRiskReasons:    record.SyscallAnalysis.HighRiskReasons,
        Summary:            convertToSummary(record.SyscallAnalysis.Summary),
    }

    return result, true, nil
}

// Helper functions for type conversion
func convertSyscallInfos(infos []elfanalyzer.SyscallInfo) []SyscallInfoData {
    result := make([]SyscallInfoData, len(infos))
    for i, info := range infos {
        result[i] = SyscallInfoData{
            Number:              info.Number,
            Name:                info.Name,
            IsNetwork:           info.IsNetwork,
            Location:            info.Location,
            DeterminationMethod: info.DeterminationMethod,
        }
    }
    return result
}

func convertToSyscallInfos(data []SyscallInfoData) []elfanalyzer.SyscallInfo {
    result := make([]elfanalyzer.SyscallInfo, len(data))
    for i, d := range data {
        result[i] = elfanalyzer.SyscallInfo{
            Number:              d.Number,
            Name:                d.Name,
            IsNetwork:           d.IsNetwork,
            Location:            d.Location,
            DeterminationMethod: d.DeterminationMethod,
        }
    }
    return result
}

func convertSummary(s elfanalyzer.SyscallSummary) SyscallSummaryData {
    return SyscallSummaryData{
        HasNetworkSyscalls:  s.HasNetworkSyscalls,
        IsHighRisk:          s.IsHighRisk,
        TotalDetectedEvents: s.TotalDetectedEvents,
        NetworkSyscallCount: s.NetworkSyscallCount,
    }
}

func convertToSummary(d SyscallSummaryData) elfanalyzer.SyscallSummary {
    return elfanalyzer.SyscallSummary{
        HasNetworkSyscalls:  d.HasNetworkSyscalls,
        IsHighRisk:          d.IsHighRisk,
        TotalDetectedEvents: d.TotalDetectedEvents,
        NetworkSyscallCount: d.NetworkSyscallCount,
    }
}
```

### 2.7 統合解析結果スキーマ

```go
// internal/fileanalysis/schema.go

package fileanalysis

import (
    "time"
)

const (
    // CurrentSchemaVersion is the current analysis record schema version.
    // Increment this when making breaking changes to the analysis record format.
    CurrentSchemaVersion = 1
)

// FileAnalysisRecord represents a unified file analysis record containing both
// hash validation and syscall analysis data.
type FileAnalysisRecord struct {
    // SchemaVersion identifies the analysis record format version.
    SchemaVersion int `json:"schema_version"`

    // FilePath is the absolute path to the analyzed file.
    FilePath string `json:"file_path"`

    // ContentHash is the SHA256 hash of the file content in prefixed format.
    // Format: "sha256:<64-char-hex>" (e.g., "sha256:abc123...def789")
    // Note: filevalidator.SHA256.Sum() returns unprefixed hex, so callers
    // must prepend "sha256:" when constructing ContentHash values.
    // Used by both filevalidator and elfanalyzer for validation.
    ContentHash string `json:"content_hash"`

    // UpdatedAt is when the analysis record was last updated.
    UpdatedAt time.Time `json:"updated_at"`

    // SyscallAnalysis contains syscall analysis result (optional).
    // Only present for static ELF binaries that have been analyzed.
    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
}

// SyscallAnalysisData contains syscall analysis information.
type SyscallAnalysisData struct {
    // Architecture is the target architecture (e.g., "x86_64").
    Architecture string `json:"architecture"`

    // AnalyzedAt is when the syscall analysis was performed.
    AnalyzedAt time.Time `json:"analyzed_at"`

    // DetectedSyscalls contains all syscall instructions found.
    DetectedSyscalls []SyscallInfoData `json:"detected_syscalls"`

    // HasUnknownSyscalls indicates if any syscall number could not be determined.
    HasUnknownSyscalls bool `json:"has_unknown_syscalls"`

    // HighRiskReasons explains why the analysis resulted in high risk.
    HighRiskReasons []string `json:"high_risk_reasons,omitempty"`

    // Summary provides aggregated information about the analysis.
    Summary SyscallSummaryData `json:"summary"`
}

// SyscallInfoData represents information about a single syscall instruction.
type SyscallInfoData struct {
    // Number is the syscall number (-1 if unknown).
    Number int `json:"number"`

    // Name is the human-readable syscall name.
    Name string `json:"name,omitempty"`

    // IsNetwork indicates whether this syscall is network-related.
    IsNetwork bool `json:"is_network"`

    // Location is the virtual address of the syscall instruction.
    Location uint64 `json:"location"`

    // DeterminationMethod describes how the syscall number was determined.
    DeterminationMethod string `json:"determination_method"`
}

// SyscallSummaryData provides aggregated analysis information.
type SyscallSummaryData struct {
    // HasNetworkSyscalls indicates presence of network-related syscalls.
    HasNetworkSyscalls bool `json:"has_network_syscalls"`

    // IsHighRisk indicates the analysis could not fully determine network capability.
    IsHighRisk bool `json:"is_high_risk"`

    // TotalDetectedEvents is the count of detected syscall events.
    // This includes both direct syscall instructions and indirect syscalls
    // via Go wrapper function calls.
    TotalDetectedEvents int `json:"total_detected_events"`

    // NetworkSyscallCount is the count of network-related syscall events.
    NetworkSyscallCount int `json:"network_syscalls"`
}
```

### 2.8 解析結果ストアエラー

```go
// internal/fileanalysis/errors.go

package fileanalysis

import (
    "errors"
    "fmt"
)

// Static errors
var (
    // ErrRecordNotFound indicates the analysis record file does not exist.
    ErrRecordNotFound = errors.New("analysis record file not found")
)

// SchemaVersionMismatchError indicates analysis record schema version mismatch.
type SchemaVersionMismatchError struct {
    Expected int
    Actual   int
}

func (e *SchemaVersionMismatchError) Error() string {
    return fmt.Sprintf("schema version mismatch: expected %d, got %d", e.Expected, e.Actual)
}

// RecordCorruptedError indicates analysis record file is corrupted.
type RecordCorruptedError struct {
    Path  string
    Cause error
}

func (e *RecordCorruptedError) Error() string {
    return fmt.Sprintf("analysis record file corrupted at %s: %v", e.Path, e.Cause)
}

func (e *RecordCorruptedError) Unwrap() error {
    return e.Cause
}
```

## 3. StandardELFAnalyzer の拡張

```go
// internal/runner/security/elfanalyzer/standard_analyzer.go
// 既存の StandardELFAnalyzer に syscall 解析結果参照を追加

// SyscallAnalysisStore defines the interface for syscall analysis result storage.
// This decouples the analyzer from the concrete storage implementation to avoid circular dependencies.
// The concrete implementation is provided by the internal/fileanalysis package.
type SyscallAnalysisStore interface {
    // LoadSyscallAnalysis loads syscall analysis from storage.
    // Returns (result, true, nil) if found and hash matches.
    // Returns (nil, false, nil) if not found or hash mismatch.
    // Returns (nil, false, error) on other errors.
    LoadSyscallAnalysis(filePath, expectedHash string) (*SyscallAnalysisResult, bool, error)
}

// StandardELFAnalyzer implements ELFAnalyzer using Go's debug/elf package.
type StandardELFAnalyzer struct {
    fs             safefileio.FileSystem
    networkSymbols map[string]SymbolCategory
    privManager    runnertypes.PrivilegeManager
    pfv            *filevalidator.PrivilegedFileValidator

    // New: syscall analysis store for static binary analysis
    syscallStore   SyscallAnalysisStore
    hashAlgo       filevalidator.HashAlgorithm
}

// NewStandardELFAnalyzerWithSyscallStore creates an analyzer with syscall analysis store support.
// Uses dependency injection for SyscallAnalysisStore to avoid circular dependencies.
func NewStandardELFAnalyzerWithSyscallStore(
    fs safefileio.FileSystem,
    privManager runnertypes.PrivilegeManager,
    store SyscallAnalysisStore,
) *StandardELFAnalyzer {
    analyzer := NewStandardELFAnalyzer(fs, privManager)

    if store != nil {
        analyzer.syscallStore = store
        analyzer.hashAlgo = &filevalidator.SHA256{}
    }

    return analyzer
}

// In AnalyzeNetworkSymbols, add syscall analysis lookup for static binaries:
func (a *StandardELFAnalyzer) AnalyzeNetworkSymbols(path string) AnalysisOutput {
    // ... existing code for dynamic binary analysis ...

    // If static binary detected and syscall analysis store is available
    if !hasDynsym && a.syscallStore != nil {
        result := a.lookupSyscallAnalysis(path)
        if result.Result != StaticBinary {
            return result
        }
    }

    // ... existing fallback to StaticBinary ...
}

// lookupSyscallAnalysis checks the syscall analysis store for analysis results.
func (a *StandardELFAnalyzer) lookupSyscallAnalysis(path string) AnalysisOutput {
    // Calculate file hash
    hash, err := a.calculateFileHash(path)
    if err != nil {
        slog.Debug("Failed to calculate hash for syscall analysis lookup",
            "path", path,
            "error", err)
        return AnalysisOutput{Result: StaticBinary}
    }

    // Load analysis result
    result, found, err := a.syscallStore.LoadSyscallAnalysis(path, hash)
    if err != nil {
        slog.Debug("Syscall analysis lookup error",
            "path", path,
            "error", err)
        return AnalysisOutput{Result: StaticBinary}
    }

    if !found {
        // Result not found or hash mismatch
        return AnalysisOutput{Result: StaticBinary}
    }

    // Convert syscall analysis result to AnalysisOutput
    return a.convertSyscallResult(result)
}

// convertSyscallResult converts SyscallAnalysisResult to AnalysisOutput.
func (a *StandardELFAnalyzer) convertSyscallResult(result *SyscallAnalysisResult) AnalysisOutput {
    if result.Summary.HasNetworkSyscalls {
        // Build detected symbols from syscall info
        var symbols []DetectedSymbol
        for _, info := range result.DetectedSyscalls {
            if info.IsNetwork {
                symbols = append(symbols, DetectedSymbol{
                    Name:     info.Name,
                    Category: "syscall",
                })
            }
        }
        return AnalysisOutput{
            Result:          NetworkDetected,
            DetectedSymbols: symbols,
        }
    }

    if result.Summary.IsHighRisk {
        // High risk: treat as potential network operation
        return AnalysisOutput{
            Result: AnalysisError,
            Error:  fmt.Errorf("syscall analysis high risk: %v", result.HighRiskReasons),
        }
    }

    return AnalysisOutput{Result: NoNetworkSymbols}
}

// calculateFileHash calculates SHA256 hash of the file.
func (a *StandardELFAnalyzer) calculateFileHash(path string) (string, error) {
    file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return "", err
    }
    defer file.Close()

    return a.hashAlgo.Sum(file)
}
```

## 4. record コマンドの拡張

```go
// cmd/record/main.go に追加するオプションと処理

import (
    "debug/elf"
    "flag"
    "fmt"
    "log/slog"
    "os"

    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// Command line flags
var (
    analyzeSyscalls = flag.Bool("analyze-syscalls", false, "Analyze syscalls for static ELF binaries")
)

// In main function, after hash recording:
func main() {
    // ... existing hash recording logic ...

    // Note: hashDir is determined from args or config in the actual implementation
    // e.g. defined along with recordDir

    // Create secure file system instance
    fs := safefileio.NewSecureFileSystem()

    if *analyzeSyscalls {
        // Check if file is a static ELF binary
        if isStaticELF(filePath, fs) {
            if err := analyzeSyscallsForFile(filePath, hashDir, pathGetter, fs); err != nil {
                slog.Warn("Syscall analysis failed",
                    "path", filePath,
                    "error", err)
                // Non-fatal: continue with hash recording
            }
        }
    }
}

// isStaticELF checks if the file is a static ELF binary.
// Uses safefileio for secure file access.
func isStaticELF(path string, fs safefileio.FileSystem) bool {
    // Open file securely to prevent symlink attacks
    file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return false
    }
    defer file.Close()

    elfFile, err := elf.NewFile(file)
    if err != nil {
        return false
    }
    defer elfFile.Close()

    // Check for .dynsym section
    dynsym := elfFile.Section(".dynsym")
    return dynsym == nil
}

// analyzeSyscallsForFile performs syscall analysis and saves to file analysis store.
// Uses safefileio for secure file access to prevent TOCTOU and symlink attacks.
func analyzeSyscallsForFile(path, analysisDir string, pathGetter fileanalysis.HashFilePathGetter, fs safefileio.FileSystem) error {
    // Open file securely
    file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return fmt.Errorf("failed to open file securely: %w", err)
    }
    defer file.Close()

    // Parse ELF from secure file handle
    elfFile, err := elf.NewFile(file)
    if err != nil {
        return fmt.Errorf("failed to parse ELF: %w", err)
    }
    defer elfFile.Close()

    // Create syscall analyzer and perform analysis
    analyzer := elfanalyzer.NewSyscallAnalyzer()
    result, err := analyzer.AnalyzeSyscallsFromELF(elfFile)
    if err != nil {
        return fmt.Errorf("analysis failed: %w", err)
    }

    // Rewind file for hash calculation
    if _, err := file.Seek(0, 0); err != nil {
        return fmt.Errorf("failed to rewind file: %w", err)
    }

    // Calculate file hash
    hashAlgo := &filevalidator.SHA256{}
    rawHash, err := hashAlgo.Sum(file)
    if err != nil {
        return fmt.Errorf("failed to calculate hash: %w", err)
    }
    // Add algorithm prefix for ContentHash format
    contentHash := fmt.Sprintf("%s:%s", hashAlgo.Name(), rawHash)

    // Create file analysis store
    store, err := fileanalysis.NewFileAnalysisStore(analysisDir, pathGetter)
    if err != nil {
        return fmt.Errorf("failed to create file analysis store: %w", err)
    }

    analysisStore := fileanalysis.NewSyscallAnalysisStore(store)

    // Save syscall analysis to file analysis store
    if err := analysisStore.SaveSyscallAnalysis(path, contentHash, result); err != nil {
        return fmt.Errorf("failed to save analysis result: %w", err)
    }

    // Log summary
    slog.Info("Syscall analysis completed",
        "path", path,
        "total_detected_events", result.Summary.TotalDetectedEvents,
        "network_syscalls", result.Summary.NetworkSyscallCount,
        "high_risk", result.Summary.IsHighRisk)

    return nil
}
```

## 5. エラー定義

```go
// internal/runner/security/elfanalyzer/errors.go

package elfanalyzer

import (
    "debug/elf"
    "errors"
    "fmt"
)

// Static errors
var (
    // ErrNoTextSection indicates the ELF file has no .text section.
    ErrNoTextSection = errors.New("ELF file has no .text section")

    // ErrNoSymbolTable indicates the ELF file has no symbol table.
    ErrNoSymbolTable = errors.New("ELF file has no symbol table (possibly stripped)")
)

// UnsupportedArchitectureError indicates the ELF architecture is not supported.
type UnsupportedArchitectureError struct {
    Machine elf.Machine
}

func (e *UnsupportedArchitectureError) Error() string {
    return fmt.Sprintf("unsupported ELF architecture: %s", e.Machine)
}
```

## 6. テスト仕様

### 6.1 MachineCodeDecoder のユニットテスト

```go
// internal/runner/security/elfanalyzer/syscall_decoder_test.go

package elfanalyzer

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestX86Decoder_IsImmediateMove(t *testing.T) {
    decoder := NewX86Decoder()

    tests := []struct {
        name     string
        code     []byte
        wantImm  bool
        wantVal  int64
    }{
        {
            name:    "mov $0x29, %eax",
            code:    []byte{0xb8, 0x29, 0x00, 0x00, 0x00},
            wantImm: true,
            wantVal: 41, // socket syscall
        },
        {
            name:    "mov $0x2a, %eax",
            code:    []byte{0xb8, 0x2a, 0x00, 0x00, 0x00},
            wantImm: true,
            wantVal: 42, // connect syscall
        },
        {
            name:    "mov %ebx, %eax (register move)",
            code:    []byte{0x89, 0xd8},
            wantImm: false,
            wantVal: 0,
        },
        {
            name:    "mov (%rsp), %eax (memory load)",
            code:    []byte{0x8b, 0x04, 0x24},
            wantImm: false,
            wantVal: 0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            inst, err := decoder.Decode(tt.code, 0)
            require.NoError(t, err)

            gotImm, gotVal := decoder.IsImmediateMove(inst)
            assert.Equal(t, tt.wantImm, gotImm)
            if tt.wantImm {
                assert.Equal(t, tt.wantVal, gotVal)
            }
        })
    }
}

func TestX86Decoder_IsControlFlowInstruction(t *testing.T) {
    decoder := NewX86Decoder()

    tests := []struct {
        name string
        code []byte
        want bool
    }{
        {"jmp", []byte{0xeb, 0x00}, true},
        {"call", []byte{0xe8, 0x00, 0x00, 0x00, 0x00}, true},
        {"ret", []byte{0xc3}, true},
        {"mov", []byte{0xb8, 0x00, 0x00, 0x00, 0x00}, false},
        {"nop", []byte{0x90}, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            inst, err := decoder.Decode(tt.code, 0)
            require.NoError(t, err)
            assert.Equal(t, tt.want, decoder.IsControlFlowInstruction(inst))
        })
    }
}
```

### 6.2 SyscallAnalyzer のユニットテスト

```go
// internal/runner/security/elfanalyzer/syscall_analyzer_test.go

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
            // jmp label; mov $0x29, %eax; syscall (jmp creates boundary)
            code:       []byte{0xeb, 0x05, 0xb8, 0x29, 0x00, 0x00, 0x00, 0x0f, 0x05},
            wantNumber: -1,
            wantMethod: "unknown:control_flow_boundary",
        },
        {
            name: "syscall only (no eax modification)",
            code:       []byte{0x0f, 0x05},
            wantNumber: -1,
            wantMethod: "unknown:scan_limit_exceeded",
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
```

### 6.3 統合解析結果ストアのユニットテスト

```go
// internal/fileanalysis/syscall_store_test.go

package fileanalysis

import (
    "encoding/json"
    "errors"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

func TestSyscallAnalysisStore_SaveAndLoad(t *testing.T) {
    tmpDir := t.TempDir()
    pathGetter := filevalidator.NewHybridHashFilePathGetter()
    store, err := NewFileAnalysisStore(tmpDir, pathGetter)
    require.NoError(t, err)

    analysisStore := NewSyscallAnalysisStore(store)

    result := &elfanalyzer.SyscallAnalysisResult{
        DetectedSyscalls: []elfanalyzer.SyscallInfo{
            {Number: 41, Name: "socket", IsNetwork: true},
        },
        Summary: elfanalyzer.SyscallSummary{
            HasNetworkSyscalls:  true,
            TotalDetectedEvents: 1,
            NetworkSyscallCount: 1,
        },
    }

    // Save
    err = analysisStore.SaveSyscallAnalysis("/test/path", "sha256:abc123", result)
    require.NoError(t, err)

    // Load with matching hash
    loaded, found, err := analysisStore.LoadSyscallAnalysis("/test/path", "sha256:abc123")
    require.NoError(t, err)
    require.True(t, found)
    assert.Equal(t, result.Summary.HasNetworkSyscalls, loaded.Summary.HasNetworkSyscalls)

    // Load with mismatched hash
    _, found, err = analysisStore.LoadSyscallAnalysis("/test/path", "sha256:different")
    require.NoError(t, err)
    assert.False(t, found)  // Hash mismatch returns found=false, not error
}

func TestFileAnalysisStore_SchemaVersionMismatch(t *testing.T) {
    tmpDir := t.TempDir()
    pathGetter := filevalidator.NewHybridHashFilePathGetter()
    store, err := NewFileAnalysisStore(tmpDir, pathGetter)
    require.NoError(t, err)

    // Create analysis record file with different schema version
    record := &FileAnalysisRecord{
        SchemaVersion: 999, // Future version
        FilePath:      "/test/path",
        ContentHash:   "sha256:abc123",
    }

    data, _ := json.Marshal(record)
    recordPath := filepath.Join(tmpDir, "~test~path")
    os.WriteFile(recordPath, data, 0o600)

    // Load should fail with schema version mismatch
    _, err = store.Load("/test/path")
    assert.Error(t, err)
    var schemaErr *SchemaVersionMismatchError
    assert.True(t, errors.As(err, &schemaErr))
}

func TestFileAnalysisStore_PreservesExistingFields(t *testing.T) {
    tmpDir := t.TempDir()
    pathGetter := filevalidator.NewHybridHashFilePathGetter()
    store, err := NewFileAnalysisStore(tmpDir, pathGetter)
    require.NoError(t, err)

    // First, save an analysis record with just content hash
    record := &FileAnalysisRecord{
        ContentHash: "sha256:abc123",
    }
    err = store.Save("/test/path", record)
    require.NoError(t, err)

    // Now update with syscall analysis
    analysisStore := NewSyscallAnalysisStore(store)
    result := &elfanalyzer.SyscallAnalysisResult{
        Summary: elfanalyzer.SyscallSummary{
            HasNetworkSyscalls: true,
        },
    }
    err = analysisStore.SaveSyscallAnalysis("/test/path", "sha256:abc123", result)
    require.NoError(t, err)

    // Verify both fields are present
    loaded, err := store.Load("/test/path")
    require.NoError(t, err)
    assert.Equal(t, "sha256:abc123", loaded.ContentHash)
    assert.NotNil(t, loaded.SyscallAnalysis)
    assert.True(t, loaded.SyscallAnalysis.Summary.HasNetworkSyscalls)
}
```

### 6.4 統合テスト

```go
// internal/runner/security/elfanalyzer/syscall_analyzer_integration_test.go

//go:build integration

package elfanalyzer

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSyscallAnalyzer_RealBinary(t *testing.T) {
    // Skip if gcc not available
    if _, err := exec.LookPath("gcc"); err != nil {
        t.Skip("gcc not available")
    }

    // Create test C program
    src := `
#include <sys/socket.h>
int main() {
    socket(AF_INET, SOCK_STREAM, 0);
    return 0;
}
`
    tmpDir := t.TempDir()
    srcFile := filepath.Join(tmpDir, "test.c")
    binFile := filepath.Join(tmpDir, "test")

    require.NoError(t, os.WriteFile(srcFile, []byte(src), 0o644))

    // Compile with static linking
    cmd := exec.Command("gcc", "-static", "-o", binFile, srcFile)
    require.NoError(t, cmd.Run())

    // Analyze
    analyzer := NewSyscallAnalyzer()
    result, err := analyzer.AnalyzeSyscalls(binFile)
    require.NoError(t, err)

    // Verify network syscall detected
    assert.True(t, result.Summary.HasNetworkSyscalls)
    assert.Greater(t, result.Summary.NetworkSyscallCount, 0)

    // Verify socket syscall found
    found := false
    for _, info := range result.DetectedSyscalls {
        if info.Name == "socket" {
            found = true
            break
        }
    }
    assert.True(t, found, "socket syscall should be detected")
}
```

## 7. 受け入れ条件とテストのマッピング

| 受け入れ条件 | テスト |
|------------|--------|
| AC-1: syscall 命令の検出 | `TestSyscallAnalyzer_BackwardScan` |
| AC-2: ネットワーク関連 syscall の判定 | `TestX86_64SyscallTable_IsNetworkSyscall` |
| AC-3: 間接設定の high risk 判定 | `TestSyscallAnalyzer_BackwardScan` (indirect case) |
| AC-4: 解析結果の保存と読み込み | `TestSyscallAnalysisStore_SaveAndLoad` |
| AC-5: 解析結果の整合性検証 | `TestFileAnalysisStore_SchemaVersionMismatch` |
| AC-6: 解析結果不在時の安全な動作 | `TestStandardELFAnalyzer_ResultNotFound` |
| AC-7: 非 ELF ファイルのエラーハンドリング | `TestSyscallAnalyzer_NonELF` |
| AC-8: フォールバックチェーンの統合 | `TestNetworkAnalyzer_FallbackChain` |
| AC-9: 既存機能への非影響 | 既存テストの維持 |
| AC-10: Go syscall ラッパーの解決 | `TestGoWrapperResolver_Resolve` |

## 8. 実装上の注意点

### 8.1 x86asm パッケージの制約

- `golang.org/x/arch/x86/x86asm` は Intel/AT&T 両方の構文をサポート
- 64-bit モードのデコードには第2引数に `64` を指定
- 不正な命令バイト列ではエラーを返す（エラーハンドリング必須）

### 8.2 逆方向スキャンの実装

- 前方デコードで命令リストを構築し、そのリストを逆順に走査
- 直接逆方向デコードは可変長命令のため困難
- スキャン幅は設定可能にし、テストで調整可能に

### 8.3 Go ABI の考慮

- Go 1.17 以降はレジスタベース ABI（RAX で第1引数）
- **本実装は RAX レジスタのみ対応の限定仕様**であり、Go 1.17+ のレジスタベース ABI を前提とする
- Go 1.16 以前のスタックベース ABI には非対応（Go wrapper 解析が機能しない）
- コンパイラ最適化やインライン化により RAX 以外のレジスタ経由で syscall 番号が設定されるケースでは、番号未解決（unknown）として High Risk に分類される
- **推奨**: 対象 Go バージョンを固定し（例: Go 1.21, 1.22）、受け入れテストで実際のバイナリを用いて ABI 前提が正しいことを実証すること

### 8.4 パフォーマンス最適化

- `.text` セクションのみを解析（他のセクションは無視）
- 大規模バイナリでは進捗表示を検討
- syscall 命令のバイトパターン検索は線形スキャン（O(n)）

### 8.5 デコード失敗時の動作

命令デコードに失敗した場合、1バイトスキップして次の位置からデコードを再試行する（`pos++`）。

**設計上の考慮事項**:

1. **x86_64 の可変長命令**: 命令長が1〜15バイトと可変のため、「次の正しい命令境界」を確実に見つける方法がない
2. **実用上の影響**: `.text` セクションは通常ほぼ全てが有効な命令で構成されており、デコード失敗は稀。発生しても数バイト後に正常な命令境界に「再同期」する
3. **安全側への設計**: 検出できない場合は High Risk として扱うため（FR-3.1.4）、多少の見落としがあっても安全側に倒れる

**制限事項**:

- デコード失敗後の誤検出リスク: 偶然 `0F 05` パターンがデータ領域内に現れた場合、誤って syscall 命令として検出する可能性がある
- 命令境界のずれ: 不正確なアラインメントで再開すると、本来の命令を見落とす可能性がある

これらの制限は、実装の複雑さとのトレードオフとして許容している。より堅牢なアプローチ（シンボルテーブルを使った関数境界の特定など）は将来の改善課題とする。

**ログ出力要件**:

デコード失敗の発生状況を可視化するため、以下のログ出力を実装すること：

- デコード失敗が発生した場合、`slog.Debug` でログを出力する
  - 出力項目: ファイルパス、失敗位置（オフセット）、失敗したバイト列（先頭数バイト）
- 解析完了時に、デコード失敗の総数をサマリとして `slog.Debug` で出力する
  - 出力項目: ファイルパス、デコード失敗回数、解析した総バイト数

これにより、デコード失敗が多発するバイナリの調査が可能となり、必要に応じて解析ロジックの改善や対象バイナリの手動検証を行える。

## 9. 実装後タスク

### 9.1 開発者ドキュメントの更新

実装完了後、`docs/development/` 配下に以下の内容を文書化すること：

#### 9.1.1 x86_64 命令デコードの技術詳細

- **デコード失敗時の動作**: 命令デコードに失敗した場合、1バイトスキップして次の位置からデコードを再試行する。これは x86_64 の可変長命令（1〜15バイト）に起因する制約であり、「次の正しい命令境界」を確実に見つける方法がないための設計判断である。
- **再同期メカニズム**: `.text` セクションは通常ほぼ全てが有効な命令で構成されているため、デコード失敗後も数バイト以内で正常な命令境界に再同期する。
- **誤検出リスク**: 命令境界がずれた状態で `0F 05` パターンがデータ領域内に偶然現れた場合、誤って syscall 命令として検出する可能性がある。

#### 9.1.2 設計判断の根拠

- **デコード失敗を High Risk としない理由**:
  1. Pass 1 の解析対象は直接 syscall 命令であり、デコード失敗は syscall 命令自体の検出には影響しにくい（syscall 命令は `0F 05` の 2バイト固定）
  2. デコード失敗が多発するケースは稀であり、過度に High Risk 判定を行うと実用性が低下する
  3. Pass 2（Go ラッパー解析）でのデコード失敗も、必ずしも syscall ラッパー呼び出しの見落としを意味しない

- **安全側への設計原則との整合性**: 本設計では「検出できない syscall 番号」を High Risk とする（FR-3.1.4）。デコード失敗は「syscall 番号を検出できない」ケースとは異なり、「命令自体を認識できない」ケースである。syscall 命令が正常にデコードされた場合に番号が不明であれば High Risk とし、デコード自体の失敗は別問題として扱う。
