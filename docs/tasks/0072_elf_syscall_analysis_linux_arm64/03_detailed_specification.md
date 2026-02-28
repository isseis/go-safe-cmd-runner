# 詳細仕様書: ELF 機械語解析による syscall 静的解析（Linux/arm64 対応）

## 0. 前提と参照

本仕様はタスク 0070 の実装を前提とする。既存のパッケージ構成・型定義・仕様は
[タスク 0070 詳細仕様書](../../tasks/0070_elf_syscall_analysis/03_detailed_specification.md) を参照。
本書では変更・追加される仕様のみを記述する。

受け入れ条件は [01_requirements.md](01_requirements.md) で定義されている。アーキテクチャ設計は
[02_architecture.md](02_architecture.md) を参照。

## 1. パッケージ構成の変更

タスク 0070 で作成したファイルに加え、以下のファイルを変更・追加する。

```
internal/runner/security/elfanalyzer/
    syscall_decoder.go          # 変更: DecodedInstruction 汎用化、MachineCodeDecoder リネーム
    syscall_decoder_test.go     # 変更: メソッド名更新、arm64 デコーダーテスト追加
    syscall_analyzer.go         # 変更: archConfig 導入、ディスパッチロジック追加
    syscall_analyzer_test.go    # 変更: モックのメソッド名変更
    go_wrapper_resolver.go      # 変更: インターフェース化・共通ロジック抽出
    go_wrapper_resolver_test.go # 変更・追加: X86GoWrapperResolver テスト
    arm64_decoder.go            # 新規: ARM64Decoder
    arm64_decoder_test.go       # 新規: ARM64Decoder 単体テスト
    arm64_syscall_numbers.go    # 新規: ARM64LinuxSyscallTable
    arm64_syscall_numbers_test.go # 新規
    arm64_go_wrapper_resolver.go  # 新規: ARM64GoWrapperResolver
    arm64_go_wrapper_resolver_test.go # 新規
    x86_go_wrapper_resolver.go     # 新規: X86GoWrapperResolver（go_wrapper_resolver.go から分割）
    x86_go_wrapper_resolver_test.go # 新規（既存テストを移動）
    syscall_analyzer_integration_test.go # 変更: arm64 統合テスト追加
    testdata/
        arm64_network_program/ # 新規: arm64 テスト用ソースコード
```

## 2. 型定義とインターフェースの変更

### 2.1 DecodedInstruction の汎用化

`syscall_decoder.go` の `DecodedInstruction` 構造体を以下のように変更する。

```go
// DecodedInstruction represents a decoded machine instruction.
// The arch field stores architecture-specific decoded data and is only
// accessed by the corresponding decoder implementation via type assertion.
// External consumers (SyscallAnalyzer, GoWrapperResolver) must not access
// the arch field directly; they use MachineCodeDecoder interface methods or
// decoder-specific helper methods instead.
type DecodedInstruction struct {
    // Offset is the instruction's virtual address.
    Offset uint64

    // Len is the instruction length in bytes.
    Len int

    // Raw contains the raw instruction bytes.
    Raw []byte

    // arch stores architecture-specific decoded instruction.
    // X86Decoder stores x86asm.Inst; ARM64Decoder stores arm64asm.Inst.
    arch any
}
```

**変更点**:
- `Op x86asm.Op` フィールドを削除
- `Args []x86asm.Arg` フィールドを削除
- `arch any` フィールドを追加（unexported）

### 2.2 MachineCodeDecoder インターフェースの汎用化

`syscall_decoder.go` の `MachineCodeDecoder` インターフェースを以下のように更新する。

```go
// MachineCodeDecoder defines the interface for decoding machine code.
// Implementations exist for x86_64 (X86Decoder) and arm64 (ARM64Decoder).
type MachineCodeDecoder interface {
    // Decode decodes a single instruction at the given offset from code.
    // On failure, returns a zero-value DecodedInstruction and an error.
    Decode(code []byte, offset uint64) (DecodedInstruction, error)

    // IsSyscallInstruction returns true if the instruction is a syscall.
    // x86_64: SYSCALL opcode (0F 05)
    // arm64:  SVC #0 (D4000001)
    IsSyscallInstruction(inst DecodedInstruction) bool

    // ModifiesSyscallNumberRegister returns true if the instruction writes
    // to the architecture's syscall number register.
    // x86_64: eax/rax (any write including al, ax, r/eax)
    // arm64:  w8 or x8
    ModifiesSyscallNumberRegister(inst DecodedInstruction) bool

    // IsImmediateToSyscallNumberRegister returns (true, value) if the
    // instruction sets the syscall number register to a known immediate.
    // x86_64: MOV EAX/RAX, imm  or  XOR EAX, EAX (zeroing idiom)
    // arm64:  MOV W8/X8, #imm  (arm64asm normalizes MOVZ to MOV)
    IsImmediateToSyscallNumberRegister(inst DecodedInstruction) (bool, int64)

    // IsControlFlowInstruction returns true if the instruction changes the
    // instruction pointer in a way that may skip over the syscall number setup.
    // x86_64: JMP*, CALL, RET, IRET, INT, LOOP*, Jcc, JCXZ*
    // arm64:  B, BL, BLR, BR, RET, CBZ, CBNZ, TBZ, TBNZ
    IsControlFlowInstruction(inst DecodedInstruction) bool

    // InstructionAlignment returns the number of bytes to skip when a decode
    // failure occurs, and the granularity for instruction boundaries.
    // x86_64: 1 (variable-length instructions, byte-by-byte recovery)
    // arm64:  4 (fixed-length 4-byte instructions)
    InstructionAlignment() int
}
```

**変更点**:
- `ModifiesEAXorRAX` → `ModifiesSyscallNumberRegister` にリネーム
- `IsImmediateMove` → `IsImmediateToSyscallNumberRegister` にリネーム
- `InstructionAlignment() int` を新規追加

### 2.3 X86Decoder の更新

`syscall_decoder.go` の `X86Decoder` を更新する。

`Decode` メソッドの変更点:
- 戻り値の `DecodedInstruction` の `Op`/`Args` フィールドへの設定を除去
- `arch` フィールドに `x86asm.Inst` 値を格納する

```go
func (d *X86Decoder) Decode(code []byte, offset uint64) (DecodedInstruction, error) {
    inst, err := x86asm.Decode(code, x86_64BitMode)
    if err != nil {
        return DecodedInstruction{}, err
    }
    return DecodedInstruction{
        Offset: offset,
        Len:    inst.Len,
        Raw:    code[:inst.Len],
        arch:   inst,
    }, nil
}
```

`X86Decoder` のメソッドリネーム:
- `ModifiesEAXorRAX` → `ModifiesSyscallNumberRegister`
- `IsImmediateMove` → `IsImmediateToSyscallNumberRegister`

新規メソッド:
```go
// InstructionAlignment returns 1 for x86_64 variable-length instructions.
func (d *X86Decoder) InstructionAlignment() int { return 1 }
```

各判定メソッドの内部実装は `inst.arch.(x86asm.Inst)` で型アサーションにより `x86asm.Inst` を取り出して判定する。

#### X86Decoder の GoWrapperResolver 専用メソッド

`X86GoWrapperResolver` が使用するため、`*X86Decoder` に以下のメソッドを追加する。
これらは `MachineCodeDecoder` インターフェースには含めない。

```go
// GetCallTarget returns the target address of a CALL instruction.
// Returns (addr, true) if inst is a CALL with a relative (Rel) operand.
// Returns (0, false) otherwise.
func (d *X86Decoder) GetCallTarget(inst DecodedInstruction, instAddr uint64) (uint64, bool)

// IsImmediateToFirstArgRegister returns (value, true) if the instruction
// sets the first argument register (RAX/EAX for x86_64 Go ABI) to an immediate.
// Returns (0, false) otherwise.
func (d *X86Decoder) IsImmediateToFirstArgRegister(inst DecodedInstruction) (int64, bool)
```

`GetCallTarget` の実装:
- `x86asm.Inst.Args[0]` が `x86asm.Rel` 型であることを確認
- ターゲットアドレス = `instAddr + uint64(inst.Len) + uint64(rel)` として計算
  - `x86asm.Rel` は符号付き 32 ビット相対オフセット

`IsImmediateToFirstArgRegister` の実装:
- 既存 `IsImmediateToSyscallNumberRegister`（旧 `IsImmediateMove`）と同一の判定ロジック
  （x86_64 では第1引数レジスタが RAX/EAX であるため）
- 注: このメソッドと `IsImmediateToSyscallNumberRegister` は内部ヘルパーを共通化する

## 3. GoWrapperResolver のリファクタリング

### 3.1 GoWrapperResolver インターフェース

`go_wrapper_resolver.go` に以下のインターフェースを追加する（新規定義）。

```go
// GoWrapperResolver analyzes indirect syscalls through Go syscall wrapper functions.
type GoWrapperResolver interface {
    // HasSymbols returns true if symbol information is available.
    HasSymbols() bool

    // FindWrapperCalls scans code for calls to known Go syscall wrapper functions.
    // Returns the list of detected wrapper calls and the number of decode failures.
    FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int)

    // IsInsideWrapper returns true if addr is within a known syscall wrapper
    // function body. Used to avoid recursive analysis.
    IsInsideWrapper(addr uint64) bool
}
```

### 3.2 goWrapperBase（共通ベース構造体）

`go_wrapper_resolver.go` に共通ベース構造体を定義する。

```go
// goWrapperBase holds symbol information shared by all GoWrapperResolver
// implementations. Concrete resolvers embed this struct.
type goWrapperBase struct {
    // symbols maps function name to SymbolInfo (address, size).
    symbols map[string]SymbolInfo

    // wrapperAddrs maps start address to GoSyscallWrapper name.
    wrapperAddrs map[uint64]GoSyscallWrapper

    // wrapperRanges is sorted by start address for binary search.
    wrapperRanges []wrapperRange

    // hasSymbols reports whether symbol loading succeeded.
    hasSymbols bool
}

// HasSymbols implements GoWrapperResolver.
func (b *goWrapperBase) HasSymbols() bool { return b.hasSymbols }

// IsInsideWrapper implements GoWrapperResolver.
func (b *goWrapperBase) IsInsideWrapper(addr uint64) bool { ... } // 既存の二分探索ロジック

// loadFromPclntab parses the .gopclntab section and populates symbols,
// wrapperAddrs, and wrapperRanges.
func (b *goWrapperBase) loadFromPclntab(elfFile *elf.File) error { ... } // 既存ロジック
```

既存の `GoWrapperResolver` 構造体が持つフィールド (`symbols`, `wrapperAddrs`, `wrapperRanges`,
`hasSymbols`) と `isInsideWrapper`・`loadFromPclntab` メソッドをこの `goWrapperBase` に移動する。

### 3.3 X86GoWrapperResolver

`x86_go_wrapper_resolver.go` に実装する（既存 `go_wrapper_resolver.go` から移動・リネーム）。

```go
// X86GoWrapperResolver implements GoWrapperResolver for x86_64 binaries.
type X86GoWrapperResolver struct {
    goWrapperBase
    decoder *X86Decoder
}

// NewX86GoWrapperResolver creates an X86GoWrapperResolver from an ELF file.
// Returns an error if the .gopclntab section cannot be parsed.
func NewX86GoWrapperResolver(elfFile *elf.File) (*X86GoWrapperResolver, error)

// FindWrapperCalls implements GoWrapperResolver.
// Scans code for CALL instructions targeting known Go syscall wrappers,
// then resolves the syscall number from the preceding instructions (RAX/EAX).
func (r *X86GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int)
```

`FindWrapperCalls` の内部処理:
1. コードを `decoder.Decode()` で逐次デコード
2. `decoder.GetCallTarget(inst, inst.Offset)` でコールターゲットを取得
3. `r.wrapperAddrs[target]` でラッパー名を確認
4. `decoder.IsImmediateToFirstArgRegister(prev)` で直近命令を逆方向スキャン
5. `WrapperCall` を構築して返す

既存の `GoWrapperResolver` 構造体の `FindWrapperCalls`・`resolveSyscallArgument`・`resolveWrapper`
メソッドのロジックをそのまま移動する。

後方互換のため以下の関数を `go_wrapper_resolver.go` に残す:

```go
// NewGoWrapperResolver creates a new X86GoWrapperResolver.
// Deprecated: Use NewX86GoWrapperResolver directly.
func NewGoWrapperResolver(elfFile *elf.File) (*X86GoWrapperResolver, error) {
    return NewX86GoWrapperResolver(elfFile)
}
```

**注**: `elfanalyzer` は `internal` 傘下のパッケージであり、外部 API 向けの後方互換性を保つ必要はない。
型エイリアス `type GoWrapperResolver = X86GoWrapperResolver` は定義しない。
同一パッケージ内で同名のインターフェース（`type GoWrapperResolver interface`）と型エイリアスを
同時に宣言すると Go コンパイラが `redeclared in this block` エラーを報告するためである。
既存のテストコードは `NewX86GoWrapperResolver()` を直接呼ぶ形に更新する。

### 3.4 ARM64GoWrapperResolver

`arm64_go_wrapper_resolver.go` に実装する（新規）。

```go
// ARM64GoWrapperResolver implements GoWrapperResolver for arm64 binaries.
type ARM64GoWrapperResolver struct {
    goWrapperBase
    decoder *ARM64Decoder
}

// NewARM64GoWrapperResolver creates an ARM64GoWrapperResolver from an ELF file.
// Returns an error if the .gopclntab section cannot be parsed.
func NewARM64GoWrapperResolver(elfFile *elf.File) (*ARM64GoWrapperResolver, error)

// FindWrapperCalls implements GoWrapperResolver.
// Scans code for BL instructions targeting known Go syscall wrappers,
// then resolves the syscall number from the preceding instructions (X0/W0).
func (r *ARM64GoWrapperResolver) FindWrapperCalls(code []byte, baseAddr uint64) ([]WrapperCall, int)
```

`FindWrapperCalls` の内部処理:
1. コードを `decoder.Decode()` で 4 バイト単位で逐次デコード
2. `decoder.GetCallTarget(inst, inst.Offset)` で `BL` 命令のターゲットアドレスを取得
3. `r.wrapperAddrs[target]` でラッパー名を確認
4. 直近命令を逆方向スキャンして `decoder.IsImmediateToFirstArgRegister(prev)` で `X0`/`W0`
   への即値設定を検出
5. `WrapperCall` を構築して返す

x86_64 との処理の対称性を保つため、定数 `maxBackwardScanSteps` / `maxRecentInstructionsToKeep` /
`minRecentInstructionsForScan` は `goWrapperBase` またはパッケージ共通定数として共有する。

## 4. ARM64Decoder の実装

`arm64_decoder.go` に実装する（新規）。

### 4.1 型定義

```go
package elfanalyzer

import "golang.org/x/arch/arm64/arm64asm"

// ARM64Decoder implements MachineCodeDecoder for arm64.
type ARM64Decoder struct{}

// NewARM64Decoder creates a new ARM64Decoder.
func NewARM64Decoder() *ARM64Decoder { return &ARM64Decoder{} }
```

### 4.2 Decode

```go
// Decode decodes a single arm64 instruction (always 4 bytes).
func (d *ARM64Decoder) Decode(code []byte, offset uint64) (DecodedInstruction, error)
```

- `arm64asm.Decode(code)` を呼び出す
- 成功時: `Len = 4`、`arch = inst`（`arm64asm.Inst` 値）として `DecodedInstruction` を返す
- 失敗時: エラーを返す

### 4.3 IsSyscallInstruction

```go
func (d *ARM64Decoder) IsSyscallInstruction(inst DecodedInstruction) bool
```

判定条件:
1. `inst.arch.(arm64asm.Inst).Op == arm64asm.SVC`
2. かつ `Args[0]` が `arm64asm.Imm` 型で値が `0`

arm64 の `svc #0` のエンコード: `0xD4000001`（リトルエンディアンで `[01 00 00 D4]`）

### 4.4 ModifiesSyscallNumberRegister

```go
func (d *ARM64Decoder) ModifiesSyscallNumberRegister(inst DecodedInstruction) bool
```

判定条件（いずれかを満たす場合 true）:
- `Args[0]` が `arm64asm.W8`（32 ビットビュー）
- `Args[0]` が `arm64asm.X8`（64 ビットビュー）

注: Go コンパイラのバージョン・最適化により `W8`/`X8` どちらも使われる場合があるため両方を許容する。

### 4.5 IsImmediateToSyscallNumberRegister

```go
func (d *ARM64Decoder) IsImmediateToSyscallNumberRegister(inst DecodedInstruction) (bool, int64)
```

判定条件:
1. `Op == arm64asm.MOV`（`arm64asm` は `MOVZ` を `MOV` に正規化する）
2. かつ `Args[0]` が `arm64asm.W8` または `arm64asm.X8`
3. かつ `Args[1]` が即値型

即値の取り出し:
- `Args[1]` が `arm64asm.Imm` の場合: `int64(imm.Imm)` を返す
- `Args[1]` が `arm64asm.Imm64` の場合: `int64(imm.Imm)` を返す
  （arm64asm の `Imm64` は `uint64`; syscall 番号として有効な範囲 0〜500 ならば符号なし整数として扱う）

### 4.6 IsControlFlowInstruction

```go
func (d *ARM64Decoder) IsControlFlowInstruction(inst DecodedInstruction) bool
```

以下の Op を制御フロー命令として扱う:
`arm64asm.B`, `arm64asm.BL`, `arm64asm.BLR`, `arm64asm.BR`, `arm64asm.RET`,
`arm64asm.CBZ`, `arm64asm.CBNZ`, `arm64asm.TBZ`, `arm64asm.TBNZ`

### 4.7 InstructionAlignment

```go
func (d *ARM64Decoder) InstructionAlignment() int { return 4 }
```

### 4.8 ARM64Decoder の GoWrapperResolver 専用メソッド

```go
// GetCallTarget returns the target address of a BL instruction.
// Returns (addr, true) if inst is BL with a PCRel operand.
// Returns (0, false) otherwise.
func (d *ARM64Decoder) GetCallTarget(inst DecodedInstruction, instAddr uint64) (uint64, bool)

// IsImmediateToFirstArgRegister returns (value, true) if the instruction
// sets the first argument register (X0 or W0 for arm64 Go ABI) to an immediate.
// Returns (0, false) otherwise.
func (d *ARM64Decoder) IsImmediateToFirstArgRegister(inst DecodedInstruction) (int64, bool)
```

`GetCallTarget` の実装:
- `Op == arm64asm.BL` を確認
- `Args[0]` が `arm64asm.PCRel` 型: ターゲットアドレス = `instAddr + uint64(inst.Len) + uint64(pcrel)`
  - `arm64asm.PCRel` は `int64` 型の相対オフセット; 実際には命令アドレス自体からのオフセット
  - **注意**: `arm64asm.PCRel` の意味を `arm64asm` パッケージのドキュメントで確認し、
    `BL` 命令のアドレス + オフセット として計算する

`IsImmediateToFirstArgRegister` の実装:
- `Op == arm64asm.MOV`
- かつ `Args[0]` が `arm64asm.X0` または `arm64asm.W0`
- かつ `Args[1]` が `arm64asm.Imm` または `arm64asm.Imm64`

## 5. ARM64LinuxSyscallTable の実装

`arm64_syscall_numbers.go` に実装する（新規）。

### 5.1 型定義

```go
package elfanalyzer

// ARM64LinuxSyscallTable implements SyscallNumberTable for arm64 Linux.
type ARM64LinuxSyscallTable struct {
    syscalls       map[int]SyscallDefinition
    networkNumbers []int
}

// NewARM64LinuxSyscallTable creates a new ARM64LinuxSyscallTable.
func NewARM64LinuxSyscallTable() *ARM64LinuxSyscallTable
```

### 5.2 定義するネットワーク syscall

```
socket:     198  recvfrom: 207
socketpair: 199  sendmsg:  211
bind:       200  recvmsg:  212
listen:     201  accept4:  242
accept:     202  recvmmsg: 243
connect:    203  sendmmsg: 269
sendto:     206
```

合計 13 syscall（要件 FR-3.1.5 の全項目をカバー）。

### 5.3 メソッド実装

`SyscallNumberTable` インターフェースの 3 メソッドを実装する。実装は
`X86_64SyscallTable` と同一パターン。

## 6. SyscallAnalyzer の変更

### 6.1 archConfig 構造体

`syscall_analyzer.go` に内部型を追加する。

```go
// archConfig holds architecture-specific components for syscall analysis.
type archConfig struct {
    decoder      MachineCodeDecoder
    syscallTable SyscallNumberTable
    archName     string
    // newGoWrapperResolver creates a GoWrapperResolver for the given ELF file.
    newGoWrapperResolver func(*elf.File) (GoWrapperResolver, error)
}
```

### 6.2 SyscallAnalyzer フィールド変更

```go
// SyscallAnalyzer performs static syscall analysis on ELF binaries.
type SyscallAnalyzer struct {
    // archConfigs maps ELF machine type to architecture-specific components.
    archConfigs map[elf.Machine]*archConfig

    // maxBackwardScan is the maximum number of instructions to scan backward
    // from a syscall instruction to find the syscall number.
    maxBackwardScan int
}
```

既存の `decoder MachineCodeDecoder`・`syscallTable SyscallNumberTable` フィールドを削除し、
`archConfigs` に置き換える。

### 6.3 コンストラクタ

```go
// NewSyscallAnalyzer creates a SyscallAnalyzer with x86_64 and arm64 support.
func NewSyscallAnalyzer() *SyscallAnalyzer {
    a := &SyscallAnalyzer{
        archConfigs:     make(map[elf.Machine]*archConfig),
        maxBackwardScan: defaultMaxBackwardScan,
    }
    x86Dec := NewX86Decoder()
    a.archConfigs[elf.EM_X86_64] = &archConfig{
        decoder:      x86Dec,
        syscallTable: NewX86_64SyscallTable(),
        archName:     "x86_64",
        newGoWrapperResolver: func(f *elf.File) (GoWrapperResolver, error) {
            return NewX86GoWrapperResolver(f)
        },
    }
    arm64Dec := NewARM64Decoder()
    a.archConfigs[elf.EM_AARCH64] = &archConfig{
        decoder:      arm64Dec,
        syscallTable: NewARM64LinuxSyscallTable(),
        archName:     "arm64",
        newGoWrapperResolver: func(f *elf.File) (GoWrapperResolver, error) {
            return NewARM64GoWrapperResolver(f)
        },
    }
    return a
}

// NewSyscallAnalyzerWithConfig creates a SyscallAnalyzer with custom components.
// The decoder and table are registered for EM_X86_64 (default test architecture).
// Used primarily for testing with mock decoder/table.
func NewSyscallAnalyzerWithConfig(
    decoder MachineCodeDecoder,
    table SyscallNumberTable,
    maxScan int,
) *SyscallAnalyzer {
    a := &SyscallAnalyzer{
        archConfigs:     make(map[elf.Machine]*archConfig),
        maxBackwardScan: maxScan,
    }
    a.archConfigs[elf.EM_X86_64] = &archConfig{
        decoder:      decoder,
        syscallTable: table,
        archName:     "x86_64",
        newGoWrapperResolver: func(f *elf.File) (GoWrapperResolver, error) {
            return NewX86GoWrapperResolver(f)
        },
    }
    return a
}
```

### 6.4 AnalyzeSyscallsFromELF のアーキテクチャディスパッチ

```go
func (a *SyscallAnalyzer) AnalyzeSyscallsFromELF(elfFile *elf.File) (*SyscallAnalysisResult, error) {
    // Look up arch config
    cfg, ok := a.archConfigs[elfFile.Machine]
    if !ok {
        return nil, &UnsupportedArchitectureError{Machine: elfFile.Machine}
    }

    // Load .text section
    text := elfFile.Section(".text")
    if text == nil {
        return nil, ErrNoTextSection
    }
    code, err := text.Data()
    if err != nil {
        return nil, fmt.Errorf("reading .text section: %w", err)
    }

    // Create GoWrapperResolver
    goResolver, err := cfg.newGoWrapperResolver(elfFile)
    if err != nil {
        slog.Warn("GoWrapperResolver init failed", "arch", cfg.archName, "err", err)
        // Continue without wrapper analysis; use a no-op resolver
        goResolver = newNoopGoWrapperResolver()
    }

    return a.analyzeSyscallsInCode(code, text.Addr, cfg.decoder, cfg.syscallTable, goResolver), nil
}
```

**注**: `noopGoWrapperResolver` は `HasSymbols()=false`, `FindWrapperCalls()=(nil, 0)`,
`IsInsideWrapper()=false` を返す内部型。

### 6.5 analyzeSyscallsInCode のシグネチャ変更

```go
// Before
func (a *SyscallAnalyzer) analyzeSyscallsInCode(
    code []byte, baseAddr uint64,
    goResolver *GoWrapperResolver,
) *SyscallAnalysisResult

// After
func (a *SyscallAnalyzer) analyzeSyscallsInCode(
    code []byte, baseAddr uint64,
    decoder MachineCodeDecoder,
    table SyscallNumberTable,
    goResolver GoWrapperResolver,
) *SyscallAnalysisResult
```

内部の `findSyscallInstructions`、`backwardScanForSyscallNumber`、`decodeInstructionsInWindow` も
同様にデコーダーをパラメータとして受け取るよう変更する。

### 6.6 findSyscallInstructions の変更

バイトパターンマッチ（`code[pos] == 0x0F && code[pos+1] == 0x05`）を
`decoder.IsSyscallInstruction(inst)` に置き換える。

```go
// Before
if inst.Len == 2 && pos+1 < len(code) && code[pos] == 0x0F && code[pos+1] == 0x05 {
    locations = append(locations, inst.Offset)
}

// After
if decoder.IsSyscallInstruction(inst) {
    locations = append(locations, inst.Offset)
}
```

デコード失敗時のスキップバイト数を `decoder.InstructionAlignment()` から取得する。

### 6.7 backwardScanForSyscallNumber / decodeInstructionsInWindow の変更

ウィンドウサイズの計算式を変更する:

```go
// Before (x86_64 fixed)
windowSize := a.maxBackwardScan * maxInstructionLength

// After (architecture-aware)
windowSize := a.maxBackwardScan * decoder.InstructionAlignment() * maxInstructionLengthFactor
```

`maxInstructionLengthFactor` の定義:
- x86_64: `InstructionAlignment() = 1`, ウィンドウ計算に `maxInstructionLength = 15` を掛けるため
  実質的に変わらない
- arm64: `InstructionAlignment() = 4`, `maxInstructionLengthFactor = 1`（固定長のため追加係数不要）

**実装上の簡略化**: ウィンドウサイズ計算を以下のように統一する:

```go
windowSize := a.maxBackwardScan * maxWindowBytesPerInstruction(decoder)

func maxWindowBytesPerInstruction(decoder MachineCodeDecoder) int {
    align := decoder.InstructionAlignment()
    if align == 1 {
        return maxInstructionLength // 15 for x86_64
    }
    return align // 4 for arm64
}
```

`backwardScanForSyscallNumber` 内での `ModifiesEAXorRAX`/`IsImmediateMove` の呼び出しを
`ModifiesSyscallNumberRegister`/`IsImmediateToSyscallNumberRegister` にリネームする。

### 6.8 解析結果の Architecture フィールド

`analyzeSyscallsInCode` 内で `result.Architecture = cfg.archName` をセットする。
（`cfg` は `analyzeSyscallsInCode` の呼び出し側 `AnalyzeSyscallsFromELF` で参照可能）

`analyzeSyscallsInCode` のシグネチャに `archName string` を追加するか、返却後に設定する。
**設計選択**: `AnalyzeSyscallsFromELF` で `analyzeSyscallsInCode` の戻り値に対して
`result.Architecture = cfg.archName` をセットするシンプルな実装を採用する。

## 7. MockMachineCodeDecoder の更新

`syscall_analyzer_test.go` の `MockMachineCodeDecoder` を更新する。

```go
type MockMachineCodeDecoder struct {
    // ... existing fields ...
}

// Renamed methods
func (m *MockMachineCodeDecoder) ModifiesSyscallNumberRegister(inst DecodedInstruction) bool { ... }
func (m *MockMachineCodeDecoder) IsImmediateToSyscallNumberRegister(inst DecodedInstruction) (bool, int64) { ... }

// New method
func (m *MockMachineCodeDecoder) InstructionAlignment() int {
    if m.Alignment != 0 {
        return m.Alignment
    }
    return 1 // default: x86_64 behavior
}
```

## 8. テスト仕様

### 8.1 ARM64Decoder 単体テスト（`arm64_decoder_test.go`）

**テストデータ（arm64 バイト列）**:

| テスト名 | バイト列 | 期待される Op |
|---|---|---|
| `svc #0` | `[0x01, 0x00, 0x00, 0xD4]` | `arm64asm.SVC` (Imm=0) |
| `mov w8, #198` | `[0x08, 0x18, 0x83, 0x52]` | `arm64asm.MOV` (W8, #198) |
| `mov x8, #42` | `[0x48, 0x05, 0x80, 0xD2]` | `arm64asm.MOV` (X8, #42) |
| `bl #offset` | 適切なエンコード | `arm64asm.BL` |
| `ret` | `[0xC0, 0x03, 0x5F, 0xD6]` | `arm64asm.RET` |
| `b #offset` | 適切なエンコード | `arm64asm.B` |

**テストケース**:
- `TestARM64Decoder_Decode`: 上記各バイト列のデコード確認
- `TestARM64Decoder_IsSyscallInstruction`: `svc #0` のみ true、他は false
- `TestARM64Decoder_ModifiesSyscallNumberRegister`: `mov w8, #198`・`mov x8, #42` は true
- `TestARM64Decoder_IsImmediateToSyscallNumberRegister`: 即値取り出し確認
- `TestARM64Decoder_IsControlFlowInstruction`: RET, B は true、SVC, MOV は false
- `TestARM64Decoder_InstructionAlignment`: 4 を返すことを確認
- `TestARM64Decoder_GetCallTarget`: BL + PCRel からターゲットアドレスを計算
- `TestARM64Decoder_IsImmediateToFirstArgRegister`: X0/W0 への即値設定を検出

受け入れ条件: AC-1 (FR-3.1.1, FR-3.1.2, FR-3.1.3)

### 8.2 ARM64LinuxSyscallTable 単体テスト（`arm64_syscall_numbers_test.go`）

- `TestARM64LinuxSyscallTable_GetSyscallName`: 198 → "socket"、999 → "" 等
- `TestARM64LinuxSyscallTable_IsNetworkSyscall`: 全 13 ネットワーク syscall が true
- `TestARM64LinuxSyscallTable_GetNetworkSyscalls`: 13 エントリを返すこと

受け入れ条件: AC-2 (FR-3.1.5)

### 8.3 ARM64GoWrapperResolver 単体テスト（`arm64_go_wrapper_resolver_test.go`）

- `TestARM64GoWrapperResolver_FindWrapperCalls_ImmediateX0`:
  `mov x0, #198 / bl <wrapper_addr>` パターンで `socket` が解决されること
- `TestARM64GoWrapperResolver_FindWrapperCalls_Unresolved`:
  即値設定なしの BL は `Resolved=false` になること

受け入れ条件: AC-3 (FR-3.1.6, FR-3.2.4)

### 8.4 SyscallAnalyzer コンポーネントテスト（`syscall_analyzer_test.go` 追加）

- `TestSyscallAnalyzer_UnsupportedArchitecture`: `elf.EM_386` 等のサポート外アーキテクチャで
  `UnsupportedArchitectureError` が返ること（受け入れ条件: AC-4）
- `TestSyscallAnalyzer_ARM64WithMockDecoder`: arm64 モックデコーダーで解析が実行されること

### 8.5 X86Decoder 回帰テスト（`syscall_decoder_test.go` 変更）

既存テストのメソッド名を更新する:
- `TestX86Decoder_ModifiesEAXorRAX` → `TestX86Decoder_ModifiesSyscallNumberRegister`
- `TestX86Decoder_IsImmediateMove` → `TestX86Decoder_IsImmediateToSyscallNumberRegister`
- `InstructionAlignment` メソッドが `1` を返すテストを追加

### 8.6 統合テスト（`syscall_analyzer_integration_test.go` 追加）

`GOOS=linux GOARCH=arm64 go build` でクロスコンパイルした Go バイナリを解析し、
ネットワーク syscall（`socket`=198 等）が検出されることを確認する。

**テストデータ生成**:
```
testdata/arm64_network_program/main.go    # net.Dial 等を使うシンプルな Go プログラム
testdata/arm64_network_program/binary     # クロスコンパイル済バイナリ（事前生成・コミット）
```

クロスコンパイルコマンド:
```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
    -o testdata/arm64_network_program/binary \
    ./testdata/arm64_network_program/
```

**テストケース**:
- `TestSyscallAnalyzer_IntegrationARM64_NetworkSyscalls`:
  arm64 バイナリを解析し、`socket`（198）を含むネットワーク syscall が検出されること
- `TestSyscallAnalyzer_IntegrationARM64_Architecture`:
  解析結果の `Architecture` フィールドが `"arm64"` であること

受け入れ条件: AC-5 (FR-3.1.1〜FR-3.1.6, FR-3.2.1〜FR-3.2.3)

## 9. エラー処理

### 9.1 UnsupportedArchitectureError

既存の `UnsupportedArchitectureError` 型は変更なし。

```go
// UnsupportedArchitectureError is returned when the ELF binary's architecture
// is not supported by any registered archConfig.
type UnsupportedArchitectureError struct {
    Machine elf.Machine
}

func (e *UnsupportedArchitectureError) Error() string {
    return fmt.Sprintf("unsupported architecture: %v", e.Machine)
}
```

`AnalyzeSyscallsFromELF` は `archConfigs` にエントリがない `elf.Machine` に対して
この型のエラーを返す。

### 9.2 GoWrapperResolver 初期化失敗

`.gopclntab` セクションが存在しない（静的リンクされていない）バイナリや、パースに失敗した場合:
- `NewARM64GoWrapperResolver` / `NewX86GoWrapperResolver` はエラーを返す
- `AnalyzeSyscallsFromELF` 側でエラーをキャッチし、警告ログを出力した上で
  `noopGoWrapperResolver` を使用して解析を継続する
- 既存の `NewX86GoWrapperResolver`（旧 `NewGoWrapperResolver`）と同一の動作とする

## 10. 依存関係の追加

`go.mod` に `golang.org/x/arch` が既に追加されている（タスク 0070）。
`arm64asm` パッケージは同一モジュール内に存在するため、追加のモジュール変更は不要。

確認コマンド:
```bash
grep "golang.org/x/arch" go.mod
```

`arm64_decoder.go` に以下の import を追加する:
```go
import "golang.org/x/arch/arm64/arm64asm"
```

## 11. 後方互換性

### 変更なし（後方互換を維持するもの）
- `SyscallAnalysisResult`、`SyscallInfo`、`SyscallSummary` の JSON スキーマ
- `fileanalysis` パッケージのインターフェース
- `NewSyscallAnalyzerWithConfig` のシグネチャ
- `StandardELFAnalyzer` の呼び出し側インターフェース
- `NewGoWrapperResolver`（関数エイリアスとして維持）

### 変更あり（既存コードの更新が必要なもの）
- `MachineCodeDecoder` インターフェース実装（メソッド名変更）
  → モック実装 `MockMachineCodeDecoder` を更新する
- `DecodedInstruction` の `Op`/`Args` フィールドへの直接アクセス
  → `go_wrapper_resolver.go` 内のアクセスを `X86Decoder` 専用メソッド経由に変更する
- `analyzeSyscallsInCode` のシグネチャ（内部メソッドのため外部への影響なし）
