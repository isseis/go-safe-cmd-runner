# 詳細仕様書: `mprotect(PROT_EXEC)` 静的検出

## 0. 既存機能活用方針

本タスクはタスク 0070（ELF syscall 静的解析）の拡張であり、既存基盤を最大限に再利用する。

- **`SyscallAnalyzer`**: `backwardScanForSyscallNumber` を汎用化し、`prot` 引数の後方スキャンに流用
- **`MachineCodeDecoder`**: 2メソッドを追加し、第3引数レジスタ（`rdx`/`x2`）への即値設定を検出
- **`SyscallAnalysisResultCore`**: `ArgEvalResults` フィールドを追加し、引数評価結果を格納
- **`fileanalysis.schema`**: スキーマバージョンを v4 → v5 に更新

本体コードでの新規ファイルは `mprotect_risk.go`（リスク判定ヘルパー）のみ。
テストコードでは `mprotect_risk_test.go` を追加する。

## 1. パッケージ構成

```
# 拡張対象
internal/common/
    syscall_types.go               # SyscallArgEvalResult, SyscallArgEvalStatus 追加

internal/runner/security/elfanalyzer/
    syscall_decoder.go             # MachineCodeDecoder に 2 メソッド追加
    x86_decoder.go                 # rdx/edx 対応の実装追加
    arm64_decoder.go               # x2/w2 対応の実装追加
    syscall_analyzer.go            # backwardScanForRegister 汎用化,
                                   # evaluateMprotectArgs 追加
    mprotect_risk.go               # 新規: EvalMprotectRisk ヘルパー

internal/fileanalysis/
    schema.go                      # CurrentSchemaVersion を 5 に更新
```

## 2. 型定義

### 2.1 `SyscallArgEvalStatus` / `SyscallArgEvalResult`

`internal/common/syscall_types.go` に追加する。

```go
// SyscallArgEvalStatus is a typed string for argument evaluation status values.
type SyscallArgEvalStatus string

const (
    // SyscallArgEvalExecConfirmed indicates prot value was obtained
    // and PROT_EXEC flag (0x4) is set.
    SyscallArgEvalExecConfirmed SyscallArgEvalStatus = "exec_confirmed"

    // SyscallArgEvalExecUnknown indicates prot value could not be
    // statically determined.
    SyscallArgEvalExecUnknown SyscallArgEvalStatus = "exec_unknown"

    // SyscallArgEvalExecNotSet indicates prot value was obtained
    // and PROT_EXEC flag (0x4) is NOT set.
    SyscallArgEvalExecNotSet SyscallArgEvalStatus = "exec_not_set"
)

// SyscallArgEvalResult represents the static evaluation result
// of a syscall argument.
type SyscallArgEvalResult struct {
    // SyscallName is the syscall being evaluated (e.g., "mprotect").
    SyscallName string               `json:"syscall_name"`

    // Status is the evaluation outcome.
    Status      SyscallArgEvalStatus  `json:"status"`

    // Details provides supplementary info.
    // For exec_confirmed/exec_not_set: prot value (e.g., "prot=0x5").
    // For exec_unknown: reason (e.g., "scan limit exceeded").
    Details     string               `json:"details,omitempty"`
}
```

**設計判断**: `Status` を `SyscallArgEvalStatus` 型（型付き文字列定数）とすることで、
任意文字列の代入を型レベルで防止する。`string` フィールドとの比較では
`string(SyscallArgEvalExecConfirmed)` のようにキャストが必要だが、
安全性の利点が上回る。

### 2.2 `SyscallAnalysisResultCore` の拡張

`internal/common/syscall_types.go` の `SyscallAnalysisResultCore` に 1 フィールドを追加する。

```go
type SyscallAnalysisResultCore struct {
    // ... existing fields (Architecture, DetectedSyscalls,
    //     HasUnknownSyscalls, HighRiskReasons, Summary) ...

    // ArgEvalResults contains static evaluation results for syscall arguments.
    // Currently used for mprotect PROT_EXEC detection.
    // Only populated when relevant syscalls are detected; otherwise nil.
    ArgEvalResults []SyscallArgEvalResult `json:"arg_eval_results,omitempty"`
}
```

`omitempty` タグにより、`mprotect` 未検出時はフィールドが JSON 出力から省略される。

`SyscallAnalysisData`（`internal/fileanalysis/schema.go`）は
`SyscallAnalysisResultCore` を埋め込んでいるため、`ArgEvalResults` は
自動的に JSON 出力に含まれる。追加の変換コードは不要。

## 3. `MachineCodeDecoder` インターフェースの拡張

`internal/runner/security/elfanalyzer/syscall_decoder.go` に 2 メソッドを追加する。

```go
type MachineCodeDecoder interface {
    // ... existing methods ...

    // ModifiesThirdArgRegister returns true if the instruction writes to the
    // third syscall argument register.
    // x86_64: edx/rdx (any write including dl, dx, edx/rdx)
    // arm64:  w2 or x2
    ModifiesThirdArgRegister(inst DecodedInstruction) bool

    // IsImmediateToThirdArgRegister returns (true, value) if the instruction
    // sets the third argument register to a known immediate.
    // x86_64: MOV EDX/RDX, imm  or  XOR EDX, EDX (zeroing idiom)
    // arm64:  MOV W2/X2, #imm
    IsImmediateToThirdArgRegister(inst DecodedInstruction) (bool, int64)
}
```

**メソッド名の選定理由**: `mprotect` に特化せず「第3引数レジスタ」として汎用的に命名する。
x86_64 Linux syscall ABI: `rdi`（第1）→ `rsi`（第2）→ `rdx`（第3）、
arm64 Linux syscall ABI: `x0`（第1）→ `x1`（第2）→ `x2`（第3）。
将来他の syscall の第3引数を評価する際にも再利用可能。

### 3.1 X86Decoder の拡張

`internal/runner/security/elfanalyzer/x86_decoder.go` に実装を追加する。

```go
// ModifiesThirdArgRegister checks if the instruction modifies edx or rdx.
func (d *X86Decoder) ModifiesThirdArgRegister(inst DecodedInstruction) bool {
    x86inst, ok := inst.arch.(x86asm.Inst)
    if !ok {
        return false
    }

    // Trim trailing nil arguments
    args := x86inst.Args[:]
    for len(args) > 0 && args[len(args)-1] == nil {
        args = args[:len(args)-1]
    }
    if len(args) == 0 {
        return false
    }

    // Check destination register (first argument for most instructions)
    if arg, ok := args[0].(x86asm.Reg); ok {
        return arg == x86asm.EDX || arg == x86asm.RDX ||
            arg == x86asm.DX || arg == x86asm.DL
    }

    return false
}

// IsImmediateToThirdArgRegister checks if the instruction sets edx/rdx to a known
// immediate value. Covers MOV EDX/RDX, imm and XOR EDX, EDX (zeroing idiom).
func (d *X86Decoder) IsImmediateToThirdArgRegister(inst DecodedInstruction) (bool, int64) {
    return d.isImmediateToReg(inst, func(reg x86asm.Reg) bool {
        return reg == x86asm.EDX || reg == x86asm.RDX
    })
}
```

**実装の要点**:
- `ModifiesThirdArgRegister` は `ModifiesSyscallNumberRegister` と同じパターン。対象レジスタが `edx/rdx/dx/dl`。
- `IsImmediateToThirdArgRegister` は既存の `isImmediateToReg` ヘルパーを再利用する。MOV と XOR（自己ゼロ化）の両方をカバー。
- `edx` への MOV で上位 32bit は自動ゼロ拡張されるため、`rdx` 全体の値として扱える。

### 3.2 ARM64Decoder の拡張

`internal/runner/security/elfanalyzer/arm64_decoder.go` に実装を追加する。

```go
// ModifiesThirdArgRegister returns true if the instruction writes to
// the arm64 third syscall argument register (W2 or X2).
func (d *ARM64Decoder) ModifiesThirdArgRegister(inst DecodedInstruction) bool {
    a, ok := inst.arch.(arm64asm.Inst)
    if !ok {
        return false
    }
    if a.Args[0] == nil {
        return false
    }
    reg, ok := a.Args[0].(arm64asm.Reg)
    if !ok {
        return false
    }
    return reg == arm64asm.W2 || reg == arm64asm.X2
}

// IsImmediateToThirdArgRegister returns (true, value) if inst sets
// W2 or X2 to a known immediate value.
func (d *ARM64Decoder) IsImmediateToThirdArgRegister(inst DecodedInstruction) (bool, int64) {
    a, ok := inst.arch.(arm64asm.Inst)
    if !ok {
        return false, 0
    }
    if a.Op != arm64asm.MOV {
        return false, 0
    }
    if a.Args[0] == nil || a.Args[1] == nil {
        return false, 0
    }
    reg, ok := a.Args[0].(arm64asm.Reg)
    if !ok || (reg != arm64asm.W2 && reg != arm64asm.X2) {
        return false, 0
    }
    val, ok := arm64ImmValue(a.Args[1])
    return ok, val
}
```

**実装の要点**:
- `IsImmediateToSyscallNumberRegister`（W8/X8 対象）と同じパターン。対象レジスタが W2/X2。
- `arm64ImmValue` ヘルパーを再利用し、`Imm` / `Imm64` の両方を処理する。

### 3.3 MockMachineCodeDecoder の更新

`syscall_analyzer_test.go` の `MockMachineCodeDecoder` に 2 メソッドをスタブとして追加する。

```go
func (m *MockMachineCodeDecoder) ModifiesThirdArgRegister(_ DecodedInstruction) bool {
    return false
}

func (m *MockMachineCodeDecoder) IsImmediateToThirdArgRegister(_ DecodedInstruction) (bool, int64) {
    return false, 0
}
```

## 4. `SyscallAnalyzer` の拡張

### 4.1 `backwardScanForRegister` 汎用関数の抽出

既存の `backwardScanForSyscallNumber` のスキャンロジックを汎用関数に抽出する。

```go
// backwardScanForRegister is a generalized backward scan that extracts an
// immediate value from a target register. modifiesReg and isImmediateToReg
// are decoder methods specifying which register to track.
//
// Returns:
//   - value: the immediate value found, or -1 if not found
//   - method: the determination method string describing the result
func (a *SyscallAnalyzer) backwardScanForRegister(
    code []byte,
    baseAddr uint64,
    syscallOffset int,
    decoder MachineCodeDecoder,
    modifiesReg func(DecodedInstruction) bool,
    isImmediateToReg func(DecodedInstruction) (bool, int64),
) (value int64, method string) {
    // Window calculation identical to backwardScanForSyscallNumber
    windowStart := syscallOffset - (a.maxBackwardScan * maxWindowBytesPerInstruction(decoder))
    if windowStart < 0 {
        windowStart = 0
    }

    instructions, _ := a.decodeInstructionsInWindow(
        code, baseAddr, windowStart, syscallOffset, decoder,
    )
    if len(instructions) == 0 {
        return -1, DeterminationMethodUnknownDecodeFailed
    }

    scanCount := 0
    for i := len(instructions) - 1; i >= 0 && scanCount < a.maxBackwardScan; i-- {
        inst := instructions[i]
        scanCount++

        if decoder.IsControlFlowInstruction(inst) {
            return -1, DeterminationMethodUnknownControlFlowBoundary
        }

        if !modifiesReg(inst) {
            continue
        }

        if isImm, val := isImmediateToReg(inst); isImm {
            return val, DeterminationMethodImmediate
        }

        return -1, DeterminationMethodUnknownIndirectSetting
    }

    return -1, DeterminationMethodUnknownScanLimitExceeded
}
```

**既存の `backwardScanForSyscallNumber` の変換**:

```go
// backwardScanForSyscallNumber scans backward from syscall instruction
// to find where the syscall number register is set.
func (a *SyscallAnalyzer) backwardScanForSyscallNumber(
    code []byte, baseAddr uint64, syscallOffset int,
    decoder MachineCodeDecoder,
) (int, string) {
    value, method := a.backwardScanForRegister(
        code, baseAddr, syscallOffset, decoder,
        decoder.ModifiesSyscallNumberRegister,
        decoder.IsImmediateToSyscallNumberRegister,
    )

    if method == DeterminationMethodImmediate {
        // Validate immediate value is a valid syscall number.
        if value >= 0 && value <= maxValidSyscallNumber {
            return int(value), DeterminationMethodImmediate
        }
        return -1, DeterminationMethodUnknownIndirectSetting
    }

    return int(value), method
}
```

**設計判断**:
- `backwardScanForRegister` は syscall 番号検証（`maxValidSyscallNumber`）を行わない。この検証は `backwardScanForSyscallNumber` 固有のロジックであり、`prot` 引数には適用されない。
- `backwardScanForRegister` の戻り値は `int64` とする。`backwardScanForSyscallNumber` は内部で `int` にキャストする。
- 既存の `backwardScanForSyscallNumber` は `backwardScanForRegister` のラッパーとなり、外部インターフェース（引数・戻り値）は変更しない。

### 4.2 `defaultMaxBackwardScan` コメントの一般化

```go
// Before:
// defaultMaxBackwardScan is the default maximum number of instructions to scan
// backward from a syscall instruction to find the syscall number.

// After:
// defaultMaxBackwardScan is the default maximum number of instructions to scan
// backward from a syscall instruction. Applied to both syscall number extraction
// and syscall argument evaluation (e.g., mprotect prot flag).
```

### 4.3 `evaluateMprotectArgs` メソッド

`syscall_analyzer.go` に新規メソッドを追加する。

```go
// protExecFlag is the PROT_EXEC flag value (0x4) used in mprotect syscall.
// See: https://man7.org/linux/man-pages/man2/mprotect.2.html
const protExecFlag = 0x4

// evaluateMprotectArgs evaluates prot argument of mprotect syscall entries.
// It scans detected syscalls for mprotect, performs backward scan for prot
// register (rdx on x86_64, x2 on arm64), and returns the highest-risk
// SyscallArgEvalResult and its corresponding instruction address.
// Returns nil, 0 if no mprotect was detected.
func (a *SyscallAnalyzer) evaluateMprotectArgs(
    code []byte,
    baseAddr uint64,
    decoder MachineCodeDecoder,
    detectedSyscalls []common.SyscallInfo,
) (*common.SyscallArgEvalResult, uint64) {
    // mprotect is "mprotect" in both x86_64 (10) and arm64 (226) tables.
    mprotectName := "mprotect"

    // Collect mprotect entries from detected syscalls.
    // Only consider entries determined by "immediate" method, as those
    // have confirmed syscall numbers.
    var mprotectEntries []common.SyscallInfo
    for _, info := range detectedSyscalls {
        if info.Name == mprotectName &&
            info.DeterminationMethod == DeterminationMethodImmediate {
            mprotectEntries = append(mprotectEntries, info)
        }
    }

    if len(mprotectEntries) == 0 {
        return nil, 0
    }

    // Evaluate each mprotect entry and select the highest risk.
    // Priority: exec_confirmed > exec_unknown > exec_not_set
    var bestResult *common.SyscallArgEvalResult
    var bestLocation uint64

    for _, entry := range mprotectEntries {
        result := a.evalSingleMprotect(code, baseAddr, decoder, entry)

        if bestResult == nil || riskPriority(result.Status) > riskPriority(bestResult.Status) {
            bestResult = &result
            bestLocation = entry.Location
        }
    }

    return bestResult, bestLocation
}

// evalSingleMprotect evaluates the prot argument of a single mprotect entry.
func (a *SyscallAnalyzer) evalSingleMprotect(
    code []byte,
    baseAddr uint64,
    decoder MachineCodeDecoder,
    entry common.SyscallInfo,
) common.SyscallArgEvalResult {
    if entry.Location < baseAddr {
        return common.SyscallArgEvalResult{
            SyscallName: "mprotect",
            Status:      common.SyscallArgEvalExecUnknown,
            Details:     "invalid offset",
        }
    }
    delta := entry.Location - baseAddr
    if delta > uint64(math.MaxInt) || int(delta) > len(code)-2 {
        return common.SyscallArgEvalResult{
            SyscallName: "mprotect",
            Status:      common.SyscallArgEvalExecUnknown,
            Details:     "invalid offset",
        }
    }
    offset := int(delta)

    value, method := a.backwardScanForRegister(
        code, baseAddr, offset, decoder,
        decoder.ModifiesThirdArgRegister,
        decoder.IsImmediateToThirdArgRegister,
    )

    if method == DeterminationMethodImmediate {
        if value&protExecFlag != 0 {
            return common.SyscallArgEvalResult{
                SyscallName: "mprotect",
                Status:      common.SyscallArgEvalExecConfirmed,
                Details:     fmt.Sprintf("prot=0x%x", value),
            }
        }
        return common.SyscallArgEvalResult{
            SyscallName: "mprotect",
            Status:      common.SyscallArgEvalExecNotSet,
            Details:     fmt.Sprintf("prot=0x%x", value),
        }
    }

    // Map determination method to exec_unknown details.
    // Convert internal method constants to stable detail strings.
    details := unknownMethodDetail(method)

    return common.SyscallArgEvalResult{
        SyscallName: "mprotect",
        Status:      common.SyscallArgEvalExecUnknown,
        Details:     details,
    }
}

// riskPriority returns the priority of a SyscallArgEvalStatus.
// Higher value = higher risk.
func riskPriority(status common.SyscallArgEvalStatus) int {
    switch status {
    case common.SyscallArgEvalExecConfirmed:
        return 2
    case common.SyscallArgEvalExecUnknown:
        return 1
    case common.SyscallArgEvalExecNotSet:
        return 0
    default:
        return -1
    }
}

// unknownMethodDetail converts unknown:* determination methods to
// compact, stable detail strings for ArgEvalResults.
func unknownMethodDetail(method string) string {
    switch method {
    case DeterminationMethodUnknownDecodeFailed:
        return "decode failed"
    case DeterminationMethodUnknownControlFlowBoundary:
        return "control flow boundary"
    case DeterminationMethodUnknownIndirectSetting:
        return "indirect register setting"
    case DeterminationMethodUnknownScanLimitExceeded:
        return "scan limit exceeded"
    default:
        return "unknown reason"
    }
}
```

**設計判断**:
- `mprotect` の検出にはエントリの `Name` フィールド（既に syscall 番号テーブルから解決済み）を使用する。これにより x86_64 と arm64 で異なる syscall 番号（10 と 226）をハードコードする必要がない。
- `DeterminationMethodImmediate` のエントリのみを対象とする。`unknown:*` メソッドのエントリは syscall 番号自体が確定していないため、`mprotect` であるかどうか不明。
- `backwardScanForSyscallNumber` の検証ロジック（`maxValidSyscallNumber`）は `prot` 引数には適用しない。`prot` は任意の整数値を取り得る。

### 4.4 `analyzeSyscallsInCode` への統合

`analyzeSyscallsInCode` の末尾（Summary 構築の前）に mprotect 引数評価ステップを追加する。

```go
func (a *SyscallAnalyzer) analyzeSyscallsInCode(
    code []byte, baseAddr uint64,
    decoder MachineCodeDecoder, table SyscallNumberTable,
    goResolver GoWrapperResolver,
) *SyscallAnalysisResult {
    // ... existing Pass 1 and Pass 2 code ...

    // Evaluate mprotect prot argument (after Pass 1 and Pass 2)
    evalResult, evalLocation := a.evaluateMprotectArgs(
        code, baseAddr, decoder, result.DetectedSyscalls,
    )
    if evalResult != nil {
        result.ArgEvalResults = append(result.ArgEvalResults, *evalResult)

        if EvalMprotectRisk(result.ArgEvalResults) {
            result.Summary.IsHighRisk = true

            // Add high risk reason message
            switch evalResult.Status {
            case common.SyscallArgEvalExecConfirmed:
                result.HighRiskReasons = append(result.HighRiskReasons,
                    fmt.Sprintf("mprotect at 0x%x: PROT_EXEC confirmed (%s)",
                        evalLocation, evalResult.Details))
            case common.SyscallArgEvalExecUnknown:
                result.HighRiskReasons = append(result.HighRiskReasons,
                    fmt.Sprintf("mprotect at 0x%x: PROT_EXEC could not be ruled out (%s)",
                        evalLocation, evalResult.Details))
            }
        }
    }

    // Build summary (existing code)
    result.Summary.TotalDetectedEvents = len(result.DetectedSyscalls)
    result.Summary.HasNetworkSyscalls = result.Summary.NetworkSyscallCount > 0
    result.Summary.IsHighRisk = result.Summary.IsHighRisk || result.HasUnknownSyscalls

    return result
}
```

**変更ポイント**:
- Summary の `IsHighRisk` 設定を OR 条件に変更: `result.Summary.IsHighRisk = result.Summary.IsHighRisk || result.HasUnknownSyscalls`。これにより mprotect 由来の `IsHighRisk = true` が `HasUnknownSyscalls = false` の場合に上書きされることを防ぐ。
- 既存の `result.Summary.IsHighRisk = result.HasUnknownSyscalls` は `result.Summary.IsHighRisk = result.Summary.IsHighRisk || result.HasUnknownSyscalls` に変更する。

## 5. リスク判定ヘルパー

### 5.1 `EvalMprotectRisk` 関数

`internal/runner/security/elfanalyzer/mprotect_risk.go` を新規作成する。

```go
package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// EvalMprotectRisk evaluates ArgEvalResults for mprotect-related risk.
// Returns true if IsHighRisk should be set based on mprotect detection.
//
// Mapping rules (from requirements §5.1):
//   - exec_confirmed → true
//   - exec_unknown   → true
//   - exec_not_set   → false
//   - no mprotect entries → false
func EvalMprotectRisk(argEvalResults []common.SyscallArgEvalResult) bool {
    for _, r := range argEvalResults {
        if r.SyscallName != "mprotect" {
            continue
        }
        switch r.Status {
        case common.SyscallArgEvalExecConfirmed,
            common.SyscallArgEvalExecUnknown:
            return true
        }
    }
    return false
}
```

**配置場所の根拠**: `elfanalyzer` パッケージ内に配置する。
`security` パッケージは既に `elfanalyzer` を import しているため、`security` に置くと
`elfanalyzer → security → elfanalyzer` の循環依存が発生する。
`EvalMprotectRisk` の唯一の呼び出し元は `analyzeSyscallsInCode`（同パッケージ）であるため、
パッケージ境界を越える必要がない。

## 6. スキーマバージョンの更新

`internal/fileanalysis/schema.go` の `CurrentSchemaVersion` を `4` → `5` に更新する。

```go
const (
    // CurrentSchemaVersion is the current analysis record schema version.
    // ...
    // Version 5 adds ArgEvalResults for syscall argument evaluation
    // (mprotect PROT_EXEC detection).
    // Load returns SchemaVersionMismatchError for records with schema_version != 5.
    CurrentSchemaVersion = 5
)
```

`ArgEvalResults` は `SyscallAnalysisResultCore` のフィールドとして追加済み（§2.2）であり、
`SyscallAnalysisData` は `SyscallAnalysisResultCore` を埋め込んでいるため、追加のコード変更は不要。

## 7. 解析結果ファイル形式（v5）

```json
{
  "schema_version": 5,
  "file_path": "/usr/local/bin/myapp",
  "content_hash": "sha256:abc123...",
  "updated_at": "2025-02-05T10:30:00Z",
  "syscall_analysis": {
    "architecture": "x86_64",
    "analyzed_at": "2025-02-05T10:30:00Z",
    "detected_syscalls": [
      {"number": 10, "name": "mprotect", "is_network": false,
       "location": 8192, "determination_method": "immediate"},
      {"number": 1, "name": "write", "is_network": false,
       "location": 4256, "determination_method": "immediate"}
    ],
    "arg_eval_results": [
      {
        "syscall_name": "mprotect",
        "status": "exec_confirmed",
        "details": "prot=0x5"
      }
    ],
    "has_unknown_syscalls": false,
    "high_risk_reasons": [
      "mprotect at 0x2000: PROT_EXEC confirmed (prot=0x5)"
    ],
    "summary": {
      "has_network_syscalls": false,
      "is_high_risk": true,
      "total_detected_events": 2,
      "network_syscall_count": 0
    }
  }
}
```

`mprotect` 未検出時の例:

```json
{
  "schema_version": 5,
  "syscall_analysis": {
    "detected_syscalls": [
      {"number": 1, "name": "write", "is_network": false,
       "location": 4256, "determination_method": "immediate"}
    ],
    "has_unknown_syscalls": false,
    "summary": {
      "has_network_syscalls": false,
      "is_high_risk": false,
      "total_detected_events": 1,
      "network_syscall_count": 0
    }
  }
}
```

`arg_eval_results` フィールドは `omitempty` により省略される。

## 8. テスト仕様

### 8.1 X86Decoder 単体テスト

`x86_decoder_test.go` に追加するテストケース。

```go
func TestX86Decoder_ModifiesThirdArgRegister(t *testing.T) {
    // Test cases:
    // - mov $0x7, %edx  → true
    // - mov $0x7, %rdx  → true
    // - mov $0x7, %eax  → false
    // - mov %rsi, %rdx  → true
    // - nop             → false
}

func TestX86Decoder_IsImmediateToThirdArgRegister(t *testing.T) {
    // Test cases:
    // - mov $0x7, %rdx  → (true, 7)     [64bit]
    // - mov $0x4, %edx  → (true, 4)     [32bit]
    // - mov $0x3, %rdx  → (true, 3)
    // - mov %rsi, %rdx  → (false, 0)    [register move]
    // - xor %edx, %edx  → (true, 0)    [zeroing idiom]
    // - mov $0x7, %eax  → (false, 0)   [wrong register]
}
```

**バイト列パターン**:

| パターン | x86_64 バイト列 | 意味 |
|----------|----------------|------|
| `mov $0x7, %edx` | `BA 07 00 00 00` | 32bit 即値 MOV |
| `mov $0x4, %edx` | `BA 04 00 00 00` | 32bit 即値 MOV |
| `mov $0x3, %rdx` | `48 C7 C2 03 00 00 00` | 64bit 即値 MOV |
| `mov %rsi, %rdx` | `48 89 F2` | レジスタ間コピー |
| `xor %edx, %edx` | `31 D2` | 自己ゼロ化 |

### 8.2 ARM64Decoder 単体テスト

`arm64_decoder_test.go` に追加するテストケース。

```go
func TestARM64Decoder_ModifiesThirdArgRegister(t *testing.T) {
    // Test cases:
    // - mov x2, #0x7      → true
    // - mov w2, #0x7      → true
    // - mov x8, #0x7      → false
    // - mov x2, x1        → true  (register move modifies x2)
}

func TestARM64Decoder_IsImmediateToThirdArgRegister(t *testing.T) {
    // Test cases:
    // - mov x2, #0x7       → (true, 7)
    // - mov w2, #0x3       → (true, 3)
    // - mov x2, x1         → (false, 0) [register move]
    // - mov x8, #0x7       → (false, 0) [wrong register]
}
```

**ARM64 エンコーディング**:

| パターン | arm64 エンコーディング | 意味 |
|----------|----------------------|------|
| `mov x2, #0x7` | MOVZ X2, #0x7 | 即値 MOV（64bit） |
| `mov w2, #0x3` | MOVZ W2, #0x3 | 即値 MOV（32bit） |
| `mov x2, x1` | ORR X2, XZR, X1 | レジスタ間コピー（MOV エイリアス） |

### 8.3 `evaluateMprotectArgs` コンポーネントテスト

`syscall_analyzer_test.go` に追加するテストケース。
実際の x86_64 バイト列を使用して、`mprotect` の prot 引数評価を検証する。

```go
func TestSyscallAnalyzer_EvaluateMprotectArgs(t *testing.T) {
    tests := []struct {
        name           string
        code           []byte
        wantStatus     common.SyscallArgEvalStatus
        wantHasResult  bool
    }{
        {
            name: "PROT_EXEC confirmed (64bit rdx)",
            // mov $0xa, %eax; mov $0x7, %rdx; syscall
            // mprotect with prot=0x7 (PROT_READ|PROT_WRITE|PROT_EXEC)
            code: []byte{
                0xb8, 0x0a, 0x00, 0x00, 0x00,             // mov $0xa, %eax
                0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
                0x0f, 0x05,                                 // syscall
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
                0xba, 0x04, 0x00, 0x00, 0x00,  // mov $0x4, %edx
                0x0f, 0x05,                     // syscall
            },
            wantStatus:    common.SyscallArgEvalExecConfirmed,
            wantHasResult: true,
        },
        {
            name: "PROT_EXEC not set",
            // mov $0xa, %eax; mov $0x3, %rdx; syscall
            // mprotect with prot=0x3 (PROT_READ|PROT_WRITE)
            code: []byte{
                0xb8, 0x0a, 0x00, 0x00, 0x00,             // mov $0xa, %eax
                0x48, 0xc7, 0xc2, 0x03, 0x00, 0x00, 0x00, // mov $0x3, %rdx
                0x0f, 0x05,                                 // syscall
            },
            wantStatus:    common.SyscallArgEvalExecNotSet,
            wantHasResult: true,
        },
        {
            name: "indirect register setting",
            // mov $0xa, %eax; mov %rsi, %rdx; syscall
            code: []byte{
                0xb8, 0x0a, 0x00, 0x00, 0x00, // mov $0xa, %eax
                0x48, 0x89, 0xf2,              // mov %rsi, %rdx
                0x0f, 0x05,                     // syscall
            },
            wantStatus:    common.SyscallArgEvalExecUnknown,
            wantHasResult: true,
        },
        {
            name: "control flow boundary",
            // mov $0xa, %eax; jmp +7; mov $0x7, %rdx; syscall
            code: []byte{
                0xb8, 0x0a, 0x00, 0x00, 0x00,             // mov $0xa, %eax
                0xeb, 0x07,                                 // jmp +7
                0x48, 0xc7, 0xc2, 0x07, 0x00, 0x00, 0x00, // mov $0x7, %rdx
                0x0f, 0x05,                                 // syscall
            },
            wantStatus:    common.SyscallArgEvalExecUnknown,
            wantHasResult: true,
        },
        {
            name: "non-mprotect syscall only",
            // mov $0x01, %eax; syscall  (write, not mprotect)
            code: []byte{
                0xb8, 0x01, 0x00, 0x00, 0x00, // mov $0x01, %eax
                0x0f, 0x05,                     // syscall
            },
            wantStatus:    "",
            wantHasResult: false,
        },
    }
    // ... test implementation ...
}
```

### 8.4 複数 `mprotect` の集約テスト

```go
func TestSyscallAnalyzer_MultipleMprotect(t *testing.T) {
    // Test: exec_confirmed + exec_not_set → exec_confirmed が選択される
    // Test: exec_unknown + exec_not_set → exec_unknown が選択される
    // Test: exec_not_set のみ → exec_not_set
}
```

### 8.5 `EvalMprotectRisk` テスト

`mprotect_risk_test.go` を新規作成する。

```go
func TestEvalMprotectRisk(t *testing.T) {
    tests := []struct {
        name string
        args []common.SyscallArgEvalResult
        want bool
    }{
        {
            name: "exec_confirmed returns true",
            args: []common.SyscallArgEvalResult{
                {SyscallName: "mprotect", Status: common.SyscallArgEvalExecConfirmed},
            },
            want: true,
        },
        {
            name: "exec_unknown returns true",
            args: []common.SyscallArgEvalResult{
                {SyscallName: "mprotect", Status: common.SyscallArgEvalExecUnknown},
            },
            want: true,
        },
        {
            name: "exec_not_set returns false",
            args: []common.SyscallArgEvalResult{
                {SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
            },
            want: false,
        },
        {
            name: "empty list returns false",
            args: nil,
            want: false,
        },
        {
            name: "non-mprotect entry ignored",
            args: []common.SyscallArgEvalResult{
                {SyscallName: "mmap", Status: common.SyscallArgEvalExecConfirmed},
            },
            want: false,
        },
    }
    // ... test implementation ...
}
```

### 8.6 スキーマ v5 往復テスト

`internal/fileanalysis/` の既存テストに v5 スキーマの保存・読み込み往復テストを追加する。

```go
func TestStore_SchemaV5_ArgEvalResults(t *testing.T) {
    // 1. ArgEvalResults 付きの Record を保存
    // 2. 読み込んで ArgEvalResults が正しく復元されることを検証
    // 3. ArgEvalResults が nil の Record を保存・読み込みし、
    //    フィールドが省略されることを検証
}
```

### 8.7 統合テスト

- `make test` で既存の全テストがパスすること（AC-5）
- 既存の `Summary.HasNetworkSyscalls` の結果が変わらないこと（AC-5）

## 9. 受け入れ条件とテストのマッピング

| 受け入れ条件 | テスト | 仕様書セクション |
|------------|--------|----------------|
| AC-1: `mprotect` の識別 | `TestSyscallAnalyzer_EvaluateMprotectArgs` | §4.3 |
| AC-2: `prot` 引数の取得 | `TestSyscallAnalyzer_EvaluateMprotectArgs` | §4.1, §4.3 |
| AC-3: `PROT_EXEC` フラグの判定 | `TestSyscallAnalyzer_EvaluateMprotectArgs` | §4.3 |
| AC-4: 解析結果の保存・読み込み | `TestStore_SchemaV5_ArgEvalResults` | §6, §7 |
| AC-5: 既存機能への非影響 | `make test` 全パス | §4.4 |
| AC-6: 複数 `mprotect` の集約 | `TestSyscallAnalyzer_MultipleMprotect` | §4.3 |
| AC-7: リスク判定への反映 | `TestEvalMprotectRisk`, `TestSyscallAnalyzer_EvaluateMprotectArgs` | §4.4, §5.1 |

## 10. `convertSyscallResult` への影響

`standard_analyzer.go` の `convertSyscallResult` は既存ロジックで
`result.Summary.IsHighRisk` を参照する。`mprotect` 検出結果は
`analyzeSyscallsInCode` 内で `Summary.IsHighRisk` に反映済みであるため、
`convertSyscallResult` への変更は不要。

ただし、以下のコメントを更新する:

**`syscall_analyzer.go`**（Summary 構築ブロックのコメント）:
```go
// Before:
// - IsHighRisk: true if HasUnknownSyscalls (any syscall number could not be determined)

// After:
// - IsHighRisk: true if HasUnknownSyscalls or mprotect PROT_EXEC risk detected
```

**`standard_analyzer.go`**（`convertSyscallResult` の docコメント）:
```go
// Before:
//   - IsHighRisk: true if any syscall number could not be determined

// After:
//   - IsHighRisk: true if any syscall number could not be determined
//     or mprotect PROT_EXEC risk was detected
```

ストアから読み出した結果（`SyscallAnalysisData` → `SyscallAnalysisResult` 変換）でも
`Summary.IsHighRisk` は保存済みの値がそのまま使用されるため、追加の判定処理は不要。

## 11. 実装上の注意点

### 11.1 `backwardScanForRegister` のリファクタリング安全性

`backwardScanForSyscallNumber` を `backwardScanForRegister` のラッパーに変換する際、
既存テスト（`TestSyscallAnalyzer_BackwardScan`）が全てパスすることを確認する。
特に以下の動作が保持されることを検証:

- 即値範囲の検証（`maxValidSyscallNumber`）
- 制御フロー境界での打ち切り
- スキャン上限での打ち切り
- 間接設定の検出

### 11.2 追加 import

`syscall_analyzer.go` に追加 import は不要（`unknownMethodDetail` で
`DeterminationMethod` を変換するため `strings.TrimPrefix` を使用しない）。
`fmt`、`math` は既存 import を利用する。

### 11.3 `IsHighRisk` の OR 条件

`analyzeSyscallsInCode` の Summary 構築で `IsHighRisk` を設定する箇所を、
代入（`=`）から OR 代入（`= ... ||`）に変更する。これにより:

1. `mprotect` 評価で `IsHighRisk = true` が設定された後、
   `HasUnknownSyscalls = false` であっても `IsHighRisk = true` が維持される
2. 逆に `HasUnknownSyscalls = true` の場合も、`mprotect` 評価の結果に関わらず
   `IsHighRisk = true` が維持される
