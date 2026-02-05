# 詳細仕様書: ELF 機械語解析による syscall 静的解析

## 0. 既存機能活用方針

この実装では、重複開発を避け既存のインフラを最大限活用します：

- **elfanalyzer パッケージ**: ELF ファイルのオープン、セクション解析、AnalysisOutput 構造体
  - `StandardELFAnalyzer`: 静的バイナリ検出時のフォールバック先として拡張
  - `AnalysisOutput`, `AnalysisResult`: 既存の結果型を再利用
- **filevalidator パッケージ**: ハッシュ計算、パス生成
  - `HybridHashFilePathGetter`: キャッシュファイルパス生成
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

internal/cache/
    integrated_cache.go            # IntegratedCacheStore（統合キャッシュ層）
    schema.go                      # CacheEntry, 統合スキーマ定義
    errors.go                      # キャッシュ関連エラー
    syscall_cache.go               # SyscallCache インターフェース実装

# 拡張対象
cmd/record/
    main.go                        # --analyze-syscalls オプション追加

internal/filevalidator/
    validator.go                   # 統合キャッシュ対応（拡張）

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
    "debug/elf"
    "fmt"
)

// SyscallAnalysisResult represents the result of syscall analysis.
type SyscallAnalysisResult struct {
    // DetectedSyscalls contains all syscall instructions found with their numbers.
    DetectedSyscalls []SyscallInfo

    // HasUnknownSyscalls indicates whether any syscall number could not be determined.
    HasUnknownSyscalls bool

    // HighRiskReasons explains why the analysis resulted in high risk, if applicable.
    HighRiskReasons []string

    // Summary provides aggregated information about the analysis.
    Summary SyscallSummary
}

// SyscallInfo represents information about a single syscall instruction.
type SyscallInfo struct {
    // Number is the syscall number (e.g., 41 for socket on x86_64).
    // -1 indicates the number could not be determined.
    Number int

    // Name is the human-readable syscall name (e.g., "socket").
    // Empty if the number is unknown or not in the table.
    Name string

    // IsNetwork indicates whether this syscall is network-related.
    IsNetwork bool

    // Location is the offset within the .text section where the syscall was found.
    Location uint64

    // DeterminationMethod describes how the syscall number was determined.
    // Possible values: "immediate", "go_wrapper", "unknown"
    DeterminationMethod string
}

// SyscallSummary provides aggregated analysis information.
type SyscallSummary struct {
    // HasNetworkSyscalls indicates presence of network-related syscalls.
    HasNetworkSyscalls bool

    // IsHighRisk indicates the analysis could not fully determine network capability.
    IsHighRisk bool

    // TotalSyscallInstructions is the count of syscall instructions found.
    TotalSyscallInstructions int

    // NetworkSyscallCount is the count of network-related syscalls.
    NetworkSyscallCount int
}

// SyscallAnalyzer analyzes ELF binaries for syscall instructions.
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
        maxBackwardScan: 5, // Default: scan up to 5 instructions backward
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
func (a *SyscallAnalyzer) AnalyzeSyscalls(path string) (*SyscallAnalysisResult, error) {
    // Open ELF file
    elfFile, err := elf.Open(path)
    if err != nil {
        return nil, fmt.Errorf("failed to open ELF file: %w", err)
    }
    defer elfFile.Close()

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
    result.Summary.TotalSyscallInstructions = len(result.DetectedSyscalls)
    result.Summary.HasNetworkSyscalls = result.Summary.NetworkSyscallCount > 0
    result.Summary.IsHighRisk = result.HasUnknownSyscalls

    return result, nil
}

// findSyscallInstructions scans the code for syscall instructions (0F 05).
func (a *SyscallAnalyzer) findSyscallInstructions(code []byte, baseAddr uint64) []uint64 {
    var locations []uint64

    for i := 0; i < len(code)-1; i++ {
        if code[i] == 0x0F && code[i+1] == 0x05 {
            locations = append(locations, baseAddr+uint64(i))
        }
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
    number, method := a.backwardScanForSyscallNumber(code, offset, baseAddr)
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
func (a *SyscallAnalyzer) backwardScanForSyscallNumber(code []byte, syscallOffset int, baseAddr uint64) (int, string) {
    // Build instruction list by forward decoding up to syscall
    instructions := a.decodeInstructionsUpTo(code, syscallOffset)
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

// decodeInstructionsUpTo decodes all instructions from start up to the given offset.
func (a *SyscallAnalyzer) decodeInstructionsUpTo(code []byte, targetOffset int) []DecodedInstruction {
    var instructions []DecodedInstruction
    pos := 0

    for pos < targetOffset {
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

### 2.2 MachineCodeDecoder

```go
// internal/runner/security/elfanalyzer/syscall_decoder.go

package elfanalyzer

import (
    "golang.org/x/arch/x86/x86asm"
)

// DecodedInstruction represents a decoded x86_64 instruction.
type DecodedInstruction struct {
    // Offset is the instruction's offset within the code section.
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

    decoded := DecodedInstruction{
        Offset: offset,
        Len:    inst.Len,
        Op:     inst.Op,
        Args:   inst.Args[:],
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

### 2.4 GoWrapperResolver

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

// GoWrapperResolver resolves Go syscall wrapper calls to determine syscall numbers.
type GoWrapperResolver struct {
    symbols     map[string]SymbolInfo
    wrapperAddrs map[uint64]GoSyscallWrapper
    hasSymbols  bool
}

// NewGoWrapperResolver creates a new GoWrapperResolver.
func NewGoWrapperResolver() *GoWrapperResolver {
    return &GoWrapperResolver{
        symbols:      make(map[string]SymbolInfo),
        wrapperAddrs: make(map[uint64]GoSyscallWrapper),
    }
}

// LoadSymbols loads symbols from the ELF file's .symtab section.
// Returns error if symbol table cannot be read (e.g., stripped binary).
//
// Note: This method only reads .symtab, not .dynsym. The target symbols
// (syscall.Syscall, etc.) are Go runtime internal symbols that exist in
// .symtab of statically-linked binaries. The .dynsym section is for
// dynamic linking and would not contain these internal symbols.
// If .symtab is stripped, Go wrapper analysis cannot proceed (FR-3.1.6).
func (r *GoWrapperResolver) LoadSymbols(elfFile *elf.File) error {
    symbols, err := elfFile.Symbols()
    if err != nil {
        return ErrNoSymbolTable
    }

    for _, sym := range symbols {
        r.symbols[sym.Name] = SymbolInfo{
            Name:    sym.Name,
            Address: sym.Value,
            Size:    sym.Size,
        }

        // Check if this is a known Go wrapper
        for _, wrapper := range knownGoWrappers {
            if strings.Contains(sym.Name, wrapper.Name) {
                r.wrapperAddrs[sym.Value] = wrapper
            }
        }
    }

    r.hasSymbols = len(r.symbols) > 0
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

### 2.5 統合キャッシュ層

統合キャッシュは、ハッシュ検証情報と syscall 解析結果を単一ファイルに格納する。
利用側（filevalidator, elfanalyzer）はそれぞれ自分の関心事のみを扱うインターフェースを通じてアクセスする。

#### 2.5.1 IntegratedCacheStore

```go
// internal/cache/integrated_cache.go

package cache

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/common"
    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// IntegratedCacheStore manages unified cache files containing both
// hash validation and syscall analysis data.
type IntegratedCacheStore struct {
    cacheDir   string
    pathGetter filevalidator.HashFilePathGetter
    fs         safefileio.FileSystem
}

// NewIntegratedCacheStore creates a new IntegratedCacheStore.
func NewIntegratedCacheStore(cacheDir string, pathGetter filevalidator.HashFilePathGetter) (*IntegratedCacheStore, error) {
    // Validate cache directory
    info, err := os.Lstat(cacheDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, fmt.Errorf("cache directory does not exist: %s", cacheDir)
        }
        return nil, fmt.Errorf("failed to access cache directory: %w", err)
    }
    if !info.IsDir() {
        return nil, fmt.Errorf("cache path is not a directory: %s", cacheDir)
    }

    return &IntegratedCacheStore{
        cacheDir:   cacheDir,
        pathGetter: pathGetter,
        fs:         safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
    }, nil
}

// Load loads the cache entry for the given file path.
// Returns ErrCacheNotFound if the cache file does not exist.
func (s *IntegratedCacheStore) Load(filePath string) (*CacheEntry, error) {
    cachePath, err := s.getCachePath(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to get cache path: %w", err)
    }

    data, err := s.fs.SafeReadFile(cachePath)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrCacheNotFound
        }
        return nil, fmt.Errorf("failed to read cache file: %w", err)
    }

    var entry CacheEntry
    if err := json.Unmarshal(data, &entry); err != nil {
        return nil, &CacheCorruptedError{Path: cachePath, Cause: err}
    }

    // Validate schema version
    if entry.SchemaVersion != CurrentSchemaVersion {
        return nil, &SchemaVersionMismatchError{
            Expected: CurrentSchemaVersion,
            Actual:   entry.SchemaVersion,
        }
    }

    return &entry, nil
}

// Save saves the cache entry for the given file path.
// This performs a read-modify-write to preserve existing fields.
func (s *IntegratedCacheStore) Save(filePath string, entry *CacheEntry) error {
    cachePath, err := s.getCachePath(filePath)
    if err != nil {
        return fmt.Errorf("failed to get cache path: %w", err)
    }

    entry.SchemaVersion = CurrentSchemaVersion
    entry.FilePath = filePath
    entry.UpdatedAt = time.Now().UTC()

    data, err := json.MarshalIndent(entry, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal cache entry: %w", err)
    }

    if err := s.fs.SafeWriteFile(cachePath, data, 0o600); err != nil {
        return fmt.Errorf("failed to write cache file: %w", err)
    }

    return nil
}

// Update performs a read-modify-write operation on the cache.
// The updateFn receives the existing entry (or a new empty one if not found)
// and should modify it in place.
func (s *IntegratedCacheStore) Update(filePath string, updateFn func(*CacheEntry) error) error {
    // Try to load existing entry
    entry, err := s.Load(filePath)
    if err != nil {
        if err == ErrCacheNotFound {
            // Create new entry
            entry = &CacheEntry{}
        } else {
            // For other errors (corrupted, schema mismatch), create fresh entry
            entry = &CacheEntry{}
        }
    }

    // Apply update
    if err := updateFn(entry); err != nil {
        return err
    }

    // Save updated entry
    return s.Save(filePath, entry)
}

// getCachePath returns the cache file path for the given file.
func (s *IntegratedCacheStore) getCachePath(filePath string) (string, error) {
    absPath, err := filepath.Abs(filePath)
    if err != nil {
        return "", fmt.Errorf("failed to get absolute path: %w", err)
    }

    resolvedPath, err := common.NewResolvedPath(absPath)
    if err != nil {
        return "", fmt.Errorf("failed to create resolved path: %w", err)
    }

    return s.pathGetter.GetHashFilePath(s.cacheDir, resolvedPath)
}
```

#### 2.5.2 SyscallCache（elfanalyzer 用インターフェース実装）

```go
// internal/cache/syscall_cache.go

package cache

import (
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// SyscallCache provides syscall analysis cache operations.
// This is the interface used by elfanalyzer package.
type SyscallCache struct {
    store *IntegratedCacheStore
}

// NewSyscallCache creates a new SyscallCache backed by IntegratedCacheStore.
func NewSyscallCache(store *IntegratedCacheStore) *SyscallCache {
    return &SyscallCache{store: store}
}

// SaveSyscallAnalysis saves the syscall analysis result.
// This updates only the syscall_analysis field, preserving other fields.
func (c *SyscallCache) SaveSyscallAnalysis(filePath, fileHash string, result *elfanalyzer.SyscallAnalysisResult) error {
    return c.store.Update(filePath, func(entry *CacheEntry) error {
        entry.ContentHash = fileHash
        entry.SyscallAnalysis = &SyscallAnalysisData{
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
func (c *SyscallCache) LoadSyscallAnalysis(filePath, expectedHash string) (*elfanalyzer.SyscallAnalysisResult, bool, error) {
    entry, err := c.store.Load(filePath)
    if err != nil {
        if err == ErrCacheNotFound {
            return nil, false, nil
        }
        return nil, false, err
    }

    // Check hash match
    if entry.ContentHash != expectedHash {
        return nil, false, nil
    }

    // Check if syscall analysis exists
    if entry.SyscallAnalysis == nil {
        return nil, false, nil
    }

    // Convert back to elfanalyzer types
    result := &elfanalyzer.SyscallAnalysisResult{
        DetectedSyscalls:   convertToSyscallInfos(entry.SyscallAnalysis.DetectedSyscalls),
        HasUnknownSyscalls: entry.SyscallAnalysis.HasUnknownSyscalls,
        HighRiskReasons:    entry.SyscallAnalysis.HighRiskReasons,
        Summary:            convertToSummary(entry.SyscallAnalysis.Summary),
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
        HasNetworkSyscalls:       s.HasNetworkSyscalls,
        IsHighRisk:               s.IsHighRisk,
        TotalSyscallInstructions: s.TotalSyscallInstructions,
        NetworkSyscallCount:      s.NetworkSyscallCount,
    }
}

func convertToSummary(d SyscallSummaryData) elfanalyzer.SyscallSummary {
    return elfanalyzer.SyscallSummary{
        HasNetworkSyscalls:       d.HasNetworkSyscalls,
        IsHighRisk:               d.IsHighRisk,
        TotalSyscallInstructions: d.TotalSyscallInstructions,
        NetworkSyscallCount:      d.NetworkSyscallCount,
    }
}
```

### 2.6 統合キャッシュスキーマ

```go
// internal/cache/schema.go

package cache

import (
    "time"
)

const (
    // CurrentSchemaVersion is the current cache schema version.
    // Increment this when making breaking changes to the cache format.
    CurrentSchemaVersion = 1
)

// CacheEntry represents a unified cache entry containing both
// hash validation and syscall analysis data.
type CacheEntry struct {
    // SchemaVersion identifies the cache format version.
    SchemaVersion int `json:"schema_version"`

    // FilePath is the absolute path to the cached file.
    FilePath string `json:"file_path"`

    // ContentHash is the SHA256 hash of the file content.
    // Used by both filevalidator and elfanalyzer for cache validation.
    ContentHash string `json:"content_hash"`

    // UpdatedAt is when the cache was last updated.
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

    // Location is the offset within the .text section.
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

    // TotalSyscallInstructions is the count of syscall instructions found.
    TotalSyscallInstructions int `json:"total_syscalls"`

    // NetworkSyscallCount is the count of network-related syscalls.
    NetworkSyscallCount int `json:"network_syscalls"`
}
```

### 2.7 キャッシュエラー

```go
// internal/cache/errors.go

package cache

import (
    "errors"
    "fmt"
)

// Static errors
var (
    // ErrCacheNotFound indicates the cache file does not exist.
    ErrCacheNotFound = errors.New("cache file not found")
)

// SchemaVersionMismatchError indicates cache schema version mismatch.
type SchemaVersionMismatchError struct {
    Expected int
    Actual   int
}

func (e *SchemaVersionMismatchError) Error() string {
    return fmt.Sprintf("schema version mismatch: expected %d, got %d", e.Expected, e.Actual)
}

// CacheCorruptedError indicates cache file is corrupted.
type CacheCorruptedError struct {
    Path  string
    Cause error
}

func (e *CacheCorruptedError) Error() string {
    return fmt.Sprintf("cache file corrupted at %s: %v", e.Path, e.Cause)
}

func (e *CacheCorruptedError) Unwrap() error {
    return e.Cause
}
```

## 3. StandardELFAnalyzer の拡張

```go
// internal/runner/security/elfanalyzer/standard_analyzer.go
// 既存の StandardELFAnalyzer に syscall キャッシュ参照を追加

// SyscallCache defines the interface for syscall analysis result caching.
// This decouples the analyzer from the concrete cache implementation to avoid circular dependencies.
// The concrete implementation is provided by the internal/cache package.
type SyscallCache interface {
    // LoadSyscallAnalysis loads syscall analysis from cache.
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

    // New: syscall cache for static binary analysis
    syscallCache   SyscallCache
    hashAlgo       filevalidator.HashAlgorithm
}

// NewStandardELFAnalyzerWithSyscallCache creates an analyzer with syscall cache support.
// Uses dependency injection for SyscallCache to avoid circular dependencies.
func NewStandardELFAnalyzerWithSyscallCache(
    fs safefileio.FileSystem,
    privManager runnertypes.PrivilegeManager,
    cache SyscallCache,
) *StandardELFAnalyzer {
    analyzer := NewStandardELFAnalyzer(fs, privManager)

    if cache != nil {
        analyzer.syscallCache = cache
        analyzer.hashAlgo = filevalidator.NewSHA256()
    }

    return analyzer
}

// In AnalyzeNetworkSymbols, add syscall cache lookup for static binaries:
func (a *StandardELFAnalyzer) AnalyzeNetworkSymbols(path string) AnalysisOutput {
    // ... existing code for dynamic binary analysis ...

    // If static binary detected and syscall cache is available
    if !hasDynsym && a.syscallCache != nil {
        result := a.lookupSyscallCache(path)
        if result.Result != StaticBinary {
            return result
        }
    }

    // ... existing fallback to StaticBinary ...
}

// lookupSyscallCache checks the syscall cache for analysis results.
func (a *StandardELFAnalyzer) lookupSyscallCache(path string) AnalysisOutput {
    // Calculate file hash
    hash, err := a.calculateFileHash(path)
    if err != nil {
        slog.Debug("Failed to calculate hash for syscall cache lookup",
            "path", path,
            "error", err)
        return AnalysisOutput{Result: StaticBinary}
    }

    // Load cache
    result, found, err := a.syscallCache.LoadSyscallAnalysis(path, hash)
    if err != nil {
        slog.Debug("Syscall cache lookup error",
            "path", path,
            "error", err)
        return AnalysisOutput{Result: StaticBinary}
    }

    if !found {
        // Cache miss or hash mismatch
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

    return a.hashAlgo.Hash(file)
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

    "github.com/isseis/go-safe-cmd-runner/internal/cache"
    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
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

    if *analyzeSyscalls {
        // Check if file is a static ELF binary
        if isStaticELF(filePath) {
            if err := analyzeSyscallsForFile(filePath, hashDir, pathGetter); err != nil {
                slog.Warn("Syscall analysis failed",
                    "path", filePath,
                    "error", err)
                // Non-fatal: continue with hash recording
            }
        }
    }
}

// isStaticELF checks if the file is a static ELF binary.
func isStaticELF(path string) bool {
    elfFile, err := elf.Open(path)
    if err != nil {
        return false
    }
    defer elfFile.Close()

    // Check for .dynsym section
    dynsym := elfFile.Section(".dynsym")
    return dynsym == nil
}

// analyzeSyscallsForFile performs syscall analysis and saves to integrated cache.
func analyzeSyscallsForFile(path, cacheDir string, pathGetter filevalidator.HashFilePathGetter) error {
    // Create syscall analyzer
    analyzer := elfanalyzer.NewSyscallAnalyzer()

    // Perform analysis
    result, err := analyzer.AnalyzeSyscalls(path)
    if err != nil {
        return fmt.Errorf("analysis failed: %w", err)
    }

    // Calculate file hash
    hashAlgo := filevalidator.NewSHA256()
    file, err := os.Open(path)
    if err != nil {
        return fmt.Errorf("failed to open file for hashing: %w", err)
    }
    defer file.Close()

    hash, err := hashAlgo.Hash(file)
    if err != nil {
        return fmt.Errorf("failed to calculate hash: %w", err)
    }

    // Create integrated cache store and syscall cache
    store, err := cache.NewIntegratedCacheStore(cacheDir, pathGetter)
    if err != nil {
        return fmt.Errorf("failed to create cache store: %w", err)
    }

    syscallCache := cache.NewSyscallCache(store)

    // Save syscall analysis to integrated cache
    if err := syscallCache.SaveSyscallAnalysis(path, hash, result); err != nil {
        return fmt.Errorf("failed to save cache: %w", err)
    }

    // Log summary
    slog.Info("Syscall analysis completed",
        "path", path,
        "total_syscalls", result.Summary.TotalSyscallInstructions,
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

### 6.3 統合キャッシュのユニットテスト

```go
// internal/cache/syscall_cache_test.go

package cache

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

func TestSyscallCache_SaveAndLoad(t *testing.T) {
    tmpDir := t.TempDir()
    pathGetter := filevalidator.NewHybridHashFilePathGetter()
    store, err := NewIntegratedCacheStore(tmpDir, pathGetter)
    require.NoError(t, err)

    cache := NewSyscallCache(store)

    result := &elfanalyzer.SyscallAnalysisResult{
        DetectedSyscalls: []elfanalyzer.SyscallInfo{
            {Number: 41, Name: "socket", IsNetwork: true},
        },
        Summary: elfanalyzer.SyscallSummary{
            HasNetworkSyscalls: true,
            TotalSyscallInstructions: 1,
            NetworkSyscallCount: 1,
        },
    }

    // Save
    err = cache.SaveSyscallAnalysis("/test/path", "sha256:abc123", result)
    require.NoError(t, err)

    // Load with matching hash
    loaded, found, err := cache.LoadSyscallAnalysis("/test/path", "sha256:abc123")
    require.NoError(t, err)
    require.True(t, found)
    assert.Equal(t, result.Summary.HasNetworkSyscalls, loaded.Summary.HasNetworkSyscalls)

    // Load with mismatched hash
    _, found, err = cache.LoadSyscallAnalysis("/test/path", "sha256:different")
    require.NoError(t, err)
    assert.False(t, found)  // Hash mismatch returns found=false, not error
}

func TestIntegratedCacheStore_SchemaVersionMismatch(t *testing.T) {
    tmpDir := t.TempDir()
    pathGetter := filevalidator.NewHybridHashFilePathGetter()
    store, err := NewIntegratedCacheStore(tmpDir, pathGetter)
    require.NoError(t, err)

    // Create cache file with different schema version
    entry := &CacheEntry{
        SchemaVersion: 999, // Future version
        FilePath:      "/test/path",
        ContentHash:   "sha256:abc123",
    }

    data, _ := json.Marshal(entry)
    cachePath := filepath.Join(tmpDir, "~test~path")
    os.WriteFile(cachePath, data, 0o600)

    // Load should fail with schema version mismatch
    _, err = store.Load("/test/path")
    assert.Error(t, err)
    var schemaErr *SchemaVersionMismatchError
    assert.True(t, errors.As(err, &schemaErr))
}

func TestIntegratedCache_PreservesExistingFields(t *testing.T) {
    tmpDir := t.TempDir()
    pathGetter := filevalidator.NewHybridHashFilePathGetter()
    store, err := NewIntegratedCacheStore(tmpDir, pathGetter)
    require.NoError(t, err)

    // First, save a cache entry with just content hash
    entry := &CacheEntry{
        ContentHash: "sha256:abc123",
    }
    err = store.Save("/test/path", entry)
    require.NoError(t, err)

    // Now update with syscall analysis
    cache := NewSyscallCache(store)
    result := &elfanalyzer.SyscallAnalysisResult{
        Summary: elfanalyzer.SyscallSummary{
            HasNetworkSyscalls: true,
        },
    }
    err = cache.SaveSyscallAnalysis("/test/path", "sha256:abc123", result)
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
| AC-4: キャッシュの保存と読み込み | `TestSyscallCache_SaveAndLoad` |
| AC-5: キャッシュの整合性検証 | `TestSyscallCache_SchemaVersionMismatch` |
| AC-6: キャッシュ不在時の安全な動作 | `TestStandardELFAnalyzer_CacheMiss` |
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
- Go 1.16 以前はスタックベース ABI
- 本実装は Go 1.17+ を想定（RAX レジスタを優先的にチェック）

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
