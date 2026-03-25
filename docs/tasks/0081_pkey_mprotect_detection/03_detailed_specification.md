# 詳細仕様書: `pkey_mprotect(PROT_EXEC)` 静的検出

## 1. 変更ファイル一覧

| ファイル | 変更種別 | 概要 |
|---|---|---|
| `internal/fileanalysis/schema.go` | 修正 | `CurrentSchemaVersion` 6 → 7、コメント追加 |
| `internal/runner/security/elfanalyzer/syscall_analyzer.go` | 修正 | `evaluateMprotectArgs` → `evaluateMprotectFamilyArgs`、`evalSingleMprotect` 汎化、`analyzeSyscallsInCode` 更新、`maxValidSyscallNumber` コメント更新 |
| `internal/runner/security/elfanalyzer/prot_exec_risk.go` | 修正 | `EvalProtExecRisk` に `pkey_mprotect` 対応追加 |
| `internal/runner/security/elfanalyzer/syscall_analyzer_test.go` | 修正・追加 | `pkey_mprotect` テストケース追加、改名追従 |
| `internal/runner/security/elfanalyzer/prot_exec_risk_test.go` | 修正 | `pkey_mprotect` テストケース追加 |
| `internal/fileanalysis/file_analysis_store_test.go` | 自動追従 | `CurrentSchemaVersion - 1` 参照のため変更不要 |

## 2. `internal/fileanalysis/schema.go`

### 2.1 `CurrentSchemaVersion` 変更

```go
// 変更前（抜粋）
// Version 6 removes is_high_risk from summary and renames high_risk_reasons to analysis_warnings.
// Load returns SchemaVersionMismatchError for records with schema_version != 6.
CurrentSchemaVersion = 6

// 変更後
// Version 6 removes is_high_risk from summary and renames high_risk_reasons to analysis_warnings.
// Version 7 adds pkey_mprotect PROT_EXEC detection.
// Load returns SchemaVersionMismatchError for records with schema_version != 7.
CurrentSchemaVersion = 7
```

## 3. `internal/runner/security/elfanalyzer/syscall_analyzer.go`

### 3.1 `maxValidSyscallNumber` コメント更新

```go
// 変更前
// Current x86_64 Linux syscalls range from 0-288, but we allow up to 500
// to account for future syscall additions and various kernel configurations.

// 変更後
// Current x86_64 Linux syscalls range up to 461 (lsm_list_modules,
// as of the syscall table in this repo), but we allow up to 500
// to account for future syscall additions and various kernel configurations.
```

### 3.2 `evaluateMprotectFamilyArgs`（`evaluateMprotectArgs` から改名・拡張）

#### 変更前のシグネチャ

```go
func (a *SyscallAnalyzer) evaluateMprotectArgs(
    code []byte,
    baseAddr uint64,
    decoder MachineCodeDecoder,
    detectedSyscalls []common.SyscallInfo,
) (*common.SyscallArgEvalResult, uint64)
```

#### 変更後のシグネチャ

ローカル構造体（`syscall_analyzer.go` 内に定義）：

```go
type mprotectFamilyEvalResult struct {
    result   common.SyscallArgEvalResult
    location uint64
}
```

```go
func (a *SyscallAnalyzer) evaluateMprotectFamilyArgs(
    code []byte,
    baseAddr uint64,
    decoder MachineCodeDecoder,
    detectedSyscalls []common.SyscallInfo,
) []mprotectFamilyEvalResult
```

`result` と `location` を1つの構造体にまとめることで、並行スライスによるインデックス不一致を構造的に排除する。

#### ロジック仕様

```
mprotectFamilySyscalls = ["mprotect", "pkey_mprotect"]

evalResults = []

for each syscallName in mprotectFamilySyscalls:
    entries = detectedSyscalls
               で name == syscallName かつ
               DeterminationMethod == "immediate" のもの

    if len(entries) == 0:
        continue

    bestResult = nil
    bestLocation = 0

    for each entry in entries:
        result = evalSingleMprotect(code, baseAddr, decoder, entry, syscallName)
        if bestResult == nil or riskPriority(result.Status) > riskPriority(bestResult.Status):
            bestResult = &result
            bestLocation = entry.Location

    evalResults = append(evalResults, mprotectFamilyEvalResult{
        result:   *bestResult,
        location: bestLocation,
    })

return evalResults
```

### 3.3 `evalSingleMprotect` 汎化

#### 変更前のシグネチャ

```go
func (a *SyscallAnalyzer) evalSingleMprotect(
    code []byte,
    baseAddr uint64,
    decoder MachineCodeDecoder,
    entry common.SyscallInfo,
) common.SyscallArgEvalResult
```

#### 変更後のシグネチャ

```go
func (a *SyscallAnalyzer) evalSingleMprotect(
    code []byte,
    baseAddr uint64,
    decoder MachineCodeDecoder,
    entry common.SyscallInfo,
    syscallName string,
) common.SyscallArgEvalResult
```

#### 変更箇所

`SyscallName: "mprotect"` のハードコードをすべて `SyscallName: syscallName` に置き換える。
ロジック本体（`validateSyscallOffset`、`backwardScanForRegister`、`PROT_EXEC` 判定）は変更なし。

### 3.4 `analyzeSyscallsInCode` 更新

#### 変更前（抜粋）

```go
evalResult, evalLocation := a.evaluateMprotectArgs(
    code, baseAddr, decoder, result.DetectedSyscalls,
)
if evalResult != nil {
    result.ArgEvalResults = append(result.ArgEvalResults, *evalResult)

    if EvalProtExecRisk(result.ArgEvalResults) {
        switch evalResult.Status {
        case common.SyscallArgEvalExecConfirmed:
            result.AnalysisWarnings = append(result.AnalysisWarnings,
                fmt.Sprintf("mprotect at 0x%x: PROT_EXEC confirmed (%s)",
                    evalLocation, evalResult.Details))
        case common.SyscallArgEvalExecUnknown:
            result.AnalysisWarnings = append(result.AnalysisWarnings,
                fmt.Sprintf("mprotect at 0x%x: PROT_EXEC could not be ruled out (%s)",
                    evalLocation, evalResult.Details))
        }
    }
}
```

#### 変更後

```go
evalResults := a.evaluateMprotectFamilyArgs(
    code, baseAddr, decoder, result.DetectedSyscalls,
)
for _, er := range evalResults {
    result.ArgEvalResults = append(result.ArgEvalResults, er.result)

    if EvalProtExecRisk([]common.SyscallArgEvalResult{er.result}) {
        switch er.result.Status {
        case common.SyscallArgEvalExecConfirmed:
            result.AnalysisWarnings = append(result.AnalysisWarnings,
                fmt.Sprintf("%s at 0x%x: PROT_EXEC confirmed (%s)",
                    er.result.SyscallName, er.location, er.result.Details))
        case common.SyscallArgEvalExecUnknown:
            result.AnalysisWarnings = append(result.AnalysisWarnings,
                fmt.Sprintf("%s at 0x%x: PROT_EXEC could not be ruled out (%s)",
                    er.result.SyscallName, er.location, er.result.Details))
        }
    }
}
```

**注意点**: `EvalProtExecRisk` にはエントリを1件ずつ渡す。リスク判定は各エントリに対して
独立して行い、エントリ間の干渉を避ける。また `fmt.Sprintf` のフォーマット文字列に
`evalResult.SyscallName` を使うことで `mprotect` / `pkey_mprotect` の両方に対応する。

## 4. `internal/runner/security/elfanalyzer/prot_exec_risk.go`

### 4.1 `EvalProtExecRisk` 拡張

#### 変更前

```go
// EvalProtExecRisk evaluates ArgEvalResults for mprotect-related risk.
// Returns true if mprotect-derived risk exists (used for AnalysisWarnings
// entries and risk derivation in convertSyscallResult).
//
// Mapping rules:
//   - exec_confirmed → true
//   - exec_unknown   → true
//   - exec_not_set   → false
//   - no mprotect entries → false
func EvalProtExecRisk(argEvalResults []common.SyscallArgEvalResult) bool {
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

#### 変更後

```go
// EvalProtExecRisk evaluates ArgEvalResults for mprotect-family risk.
// Covers both mprotect and pkey_mprotect syscalls.
// Returns true if PROT_EXEC risk exists (used for AnalysisWarnings
// entries and risk derivation in convertSyscallResult).
//
// Mapping rules:
//   - exec_confirmed → true
//   - exec_unknown   → true
//   - exec_not_set   → false
//   - no mprotect/pkey_mprotect entries → false
func EvalProtExecRisk(argEvalResults []common.SyscallArgEvalResult) bool {
    for _, r := range argEvalResults {
        if r.SyscallName != "mprotect" && r.SyscallName != "pkey_mprotect" {
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

## 5. テスト仕様

### 5.1 `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs`（x86_64）

`syscall_analyzer_test.go` に追加。テスト構造はタスク 0078 の
`TestSyscallAnalyzer_EvaluateMprotectArgs` に準拠する。

#### バイト列構成（x86_64）

syscall 番号 329（`pkey_mprotect`）は以下のバイト列で表現する。

```
# pkey_mprotect のバイト列（x86_64）
mov eax, 329  → 0xb8 0x49 0x01 0x00 0x00
syscall       → 0x0f 0x05
```

**テストケース一覧**

| テスト名 | バイト列構成 | 期待 SyscallName | 期待 Status | 期待 Details |
|---|---|---|---|---|
| `PROT_EXEC confirmed (64bit rdx)` | `mov $0x7, %rdx` + syscall 329 | `pkey_mprotect` | `exec_confirmed` | `prot=0x7` |
| `PROT_EXEC confirmed (32bit edx)` | `mov $0x4, %edx` + syscall 329 | `pkey_mprotect` | `exec_confirmed` | `prot=0x4` |
| `PROT_EXEC not set` | `mov $0x3, %rdx` + syscall 329 | `pkey_mprotect` | `exec_not_set` | `prot=0x3` |
| `indirect register setting` | `mov %rsi, %rdx` + syscall 329 | `pkey_mprotect` | `exec_unknown` | `indirect register setting` |
| `pkey_mprotect syscall only` | syscall 329 のみ（スキャン範囲内に rdx 変更なし） | `pkey_mprotect` | `exec_unknown` | `window exhausted`（短いコード片では全命令を走査し尽くすため確定的） |
| `control flow boundary` | `jmp` + `mov $0x7, %rdx` + syscall 329（jmp が間に入る構成） | `pkey_mprotect` | `exec_unknown` | `control flow boundary` |
| `non-pkey_mprotect syscall only` | syscall 10（mprotect）のみ | — | （ArgEvalResults に pkey_mprotect エントリなし） | — |

### 5.2 `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64`（arm64）

#### バイト列構成（arm64）

syscall 番号 288（`pkey_mprotect`）は以下のバイト列で表現する。

```
# pkey_mprotect のバイト列（arm64）
mov x8, #288  → MOVZ X8, #288  （エンコーディング: 0x08 0x24 0x80 0xd2）
svc #0        → 0x01 0x00 0x00 0xd4
```

エンコーディングの根拠（288 = 0x0120 = 2^8 + 2^5）：
- bits[31:24] = 0xD2（MOVZ 64bit プレフィックス）
- bits[23:16] = 0x80（hw=0 固定、imm16 の上位8ビットがすべて 0）
- bits[15:8]  = 0x24（bit13=imm16[8]=1, bit10=imm16[5]=1 → 0010 0100）
- bits[7:0]   = 0x08（Rd=8=01000 → 0000 1000）
- リトルエンディアン格納: 0x08 0x24 0x80 0xD2

参考: 既存テストで確認済みの `mov x8, #226`（mprotect）= `0x48 0x1C 0x80 0xD2`

**テストケース一覧**

| テスト名 | バイト列構成 | 期待 Status |
|---|---|---|
| `exec_confirmed (mov x2, #7)` | `mov x2, #0x7` + syscall 288 | `exec_confirmed` |
| `exec_not_set (mov x2, #3)` | `mov x2, #0x3` + syscall 288 | `exec_not_set` |
| `exec_unknown (indirect register setting)` | `mov x2, x1` + syscall 288 | `exec_unknown` |
| `exec_unknown (pkey_mprotect syscall only, no x2 setup in scan range)` | syscall 288 のみ | `exec_unknown` |
| `exec_unknown (control flow boundary)` | `b` + `mov x2, #0x7` + syscall 288 | `exec_unknown` |

### 5.3 `TestSyscallAnalyzer_MprotectAndPkeyMprotect`

`mprotect` と `pkey_mprotect` が同一コードに共存する場合のテスト。

**テストケース一覧**

| テスト名 | 入力 | 期待 ArgEvalResults |
|---|---|---|
| `both detected: exec_confirmed + exec_confirmed` | syscall 10 (`mov rdx, $0x7`) + syscall 329 (`mov rdx, $0x7`) | 2件（mprotect: exec_confirmed, pkey_mprotect: exec_confirmed） |
| `both detected: exec_not_set + exec_unknown` | syscall 10 (`mov rdx, $0x3`) + syscall 329 のみ | 2件（mprotect: exec_not_set, pkey_mprotect: exec_unknown） |
| `only mprotect detected` | syscall 10 のみ | 1件（mprotect のみ） |
| `only pkey_mprotect detected` | syscall 329 のみ | 1件（pkey_mprotect のみ） |

### 5.4 `TestEvalProtExecRisk`（追加ケース）

`prot_exec_risk_test.go` の既存テストに以下を追加する。

| テスト名 | 入力 | 期待値 |
|---|---|---|
| `pkey_mprotect exec_confirmed → true` | `[{pkey_mprotect, exec_confirmed}]` | `true` |
| `pkey_mprotect exec_unknown → true` | `[{pkey_mprotect, exec_unknown}]` | `true` |
| `pkey_mprotect exec_not_set → false` | `[{pkey_mprotect, exec_not_set}]` | `false` |
| `mprotect exec_not_set + pkey_mprotect exec_unknown → true` | `[{mprotect, exec_not_set}, {pkey_mprotect, exec_unknown}]` | `true` |
| `both exec_not_set → false` | `[{mprotect, exec_not_set}, {pkey_mprotect, exec_not_set}]` | `false` |

### 5.5 スキーマバージョンテスト

既存の `TestStore_SchemaVersionMismatch` は `CurrentSchemaVersion - 1` を使用しているため
変更不要。バージョン 7 への更新後も自動的にバージョン 6 との不一致を検証する。

## 6. 受け入れ条件と対応テストの照合

| AC | 対応テスト |
|---|---|
| AC-1: pkey_mprotect の識別（x86_64: 329） | `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs` |
| AC-1: pkey_mprotect の識別（arm64: 288） | `TestSyscallAnalyzer_EvaluatePkeyMprotectArgs_ARM64` |
| AC-2: rdx/x2 後方スキャン | 上記テストの `exec_confirmed` ケース |
| AC-2: 制御フロー境界 | 上記テストの `control flow boundary` ケース |
| AC-2: スキャン上限 | 上記テストの `syscall only` ケース |
| AC-3: exec_confirmed 判定 | `PROT_EXEC confirmed` ケース |
| AC-3: exec_not_set 判定 | `PROT_EXEC not set` ケース |
| AC-3: exec_unknown 判定 | `indirect`, `control flow`, `syscall only` ケース |
| AC-4: SyscallName = "pkey_mprotect" | 全 `EvaluatePkeyMprotectArgs` テストケースの SyscallName 検証 |
| AC-4: mprotect + pkey_mprotect 共存 | `TestSyscallAnalyzer_MprotectAndPkeyMprotect` |
| AC-4: 未検出時エントリなし | `non-pkey_mprotect syscall only` ケース |
| AC-5: EvalProtExecRisk exec_confirmed → true | `TestEvalProtExecRisk` 追加ケース |
| AC-5: EvalProtExecRisk exec_unknown → true | `TestEvalProtExecRisk` 追加ケース |
| AC-5: EvalProtExecRisk exec_not_set → false | `TestEvalProtExecRisk` 追加ケース |
| AC-6: CurrentSchemaVersion = 7 | `TestStore_SchemaVersionMismatch` |
| AC-7: 既存 mprotect テスト通過 | `TestSyscallAnalyzer_EvaluateMprotectArgs*`、`TestSyscallAnalyzer_MultipleMprotect` |
| AC-7: 全テスト通過 | `make test` |
