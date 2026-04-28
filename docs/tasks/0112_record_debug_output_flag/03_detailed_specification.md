# 詳細仕様書: record コマンドへのデバッグ情報出力フラグ追加

## 1. 概要

本ドキュメントはアーキテクチャ設計書（`02_architecture.md`）を基に、
各変更ファイルの具体的な実装仕様を定義する。
受け入れ条件（AC）と各テストケースの対応関係も示す。

## 2. 変更ファイル一覧

| ファイル | 変更種別 |
|---------|---------|
| `cmd/record/main.go` | 拡張 |
| `internal/filevalidator/validator.go` | 拡張 |
| `internal/libccache/adapters.go` | 拡張 |
| `internal/filevalidator/validator_test.go` | 拡張 |
| `internal/filevalidator/validator_macho_test.go` | 拡張 |
| `internal/libccache/adapters_test.go` | 拡張 |

## 3. `cmd/record/main.go`

### 3.1 `recordConfig` 構造体

`debugInfo bool` フィールドを追加する。

```go
type recordConfig struct {
    files          []string
    hashDir        string
    force          bool
    usedDeprecated bool
    debugInfo      bool  // 追加
}
```

### 3.2 `parseArgs` 関数

`options` 構造体に `debugInfo bool` を追加し、`--debug-info` フラグを登録する。

```go
options := struct {
    deprecatedFile string
    hashDir        string
    force          bool
    debugInfo      bool  // 追加
}{}

// フラグ登録（既存フラグの後に追加）
fs.BoolVar(&options.debugInfo, "debug-info", false,
    "Include debug information (Occurrences, DeterminationStats) in output")
```

`return &recordConfig{...}` の箇所に `debugInfo: options.debugInfo` を追加する。

### 3.3 `run` 関数

`fv.SetIncludeDebugInfo(cfg.debugInfo)` を、既存のセッター呼び出し群と並べて追加する。

```go
if fv, ok := validator.(*filevalidator.Validator); ok {
    // ...（既存セッター）
    fv.SetIncludeDebugInfo(cfg.debugInfo)  // 追加（SetSyscallAnalyzer 呼び出しの後）
}
```

## 4. `internal/filevalidator/validator.go`

### 4.1 `SyscallAnalyzerInterface`

`AnalyzeSyscallsFromELF` の戻り値に `*common.SyscallDeterminationStats` を追加する。

```go
// AnalyzeSyscallsFromELF analyzes the ELF file for direct syscall instructions.
// Returns detected syscalls, argument evaluation results (e.g., mprotect PROT_EXEC),
// and determination stats for debug use.
// Returns an error wrapping ErrUnsupportedArch (detectable via errors.Is) for
// unsupported architectures.
AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error)
```

### 4.2 `Validator` 構造体

既存フィールドの最後に `includeDebugInfo bool` を追加する（行 153 付近）。

```go
syscallAnalyzer     SyscallAnalyzerInterface
machoSyscallTable   SyscallNumberTable
includeDebugInfo    bool  // 追加
```

### 4.3 `SetIncludeDebugInfo` メソッド（新規）

`SetSyscallAnalyzer` メソッドの直後に追加する。

```go
// SetIncludeDebugInfo controls whether debug information (Occurrences,
// DeterminationStats) is included in saved JSON output.
func (v *Validator) SetIncludeDebugInfo(b bool) {
    v.includeDebugInfo = b
}
```

### 4.4 `stripOccurrences` ヘルパー関数（新規）

`buildSyscallData` の直前に追加する。

```go
// stripOccurrences returns a copy of syscalls with Occurrences set to nil in each entry.
func stripOccurrences(syscalls []common.SyscallInfo) []common.SyscallInfo {
    result := make([]common.SyscallInfo, len(syscalls))
    for i, s := range syscalls {
        result[i] = s
        result[i].Occurrences = nil
    }
    return result
}
```

### 4.5 `buildSyscallData` 関数

シグネチャに `stats *common.SyscallDeterminationStats` と `includeDebugInfo bool` を追加する。
`includeDebugInfo == false` の場合、`stats` を nil にし `Occurrences` を除去する。

```go
func buildSyscallData(
    all []common.SyscallInfo,
    argEvalResults []common.SyscallArgEvalResult,
    machine elf.Machine,
    stats *common.SyscallDeterminationStats,
    includeDebugInfo bool,
) *fileanalysis.SyscallAnalysisData {
    syscalls := all
    if !includeDebugInfo {
        syscalls = stripOccurrences(all)
        stats = nil
    }
    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:       elfArchName(machine),
            DetectedSyscalls:   syscalls,
            ArgEvalResults:     argEvalResults,
            DeterminationStats: stats,
        },
    }
}
```

### 4.6 `buildMachoSyscallData` 関数

シグネチャに `includeDebugInfo bool` を追加する。
`includeDebugInfo == false` の場合、`merged` を `stripOccurrences` で処理する。

```go
func buildMachoSyscallData(
    svcEntries []common.SyscallInfo,
    libsysEntries []common.SyscallInfo,
    arch string,
    includeDebugInfo bool,
) *fileanalysis.SyscallAnalysisData {
    merged := mergeMachoSyscallInfos(svcEntries, libsysEntries)

    var warnings []string
    for _, s := range merged {
        if s.Number == -1 {
            for _, occ := range s.Occurrences {
                if occ.DeterminationMethod == common.DeterminationMethodDirectSVC0x80 {
                    warnings = []string{"svc #0x80 detected: syscall number unresolved, direct kernel call bypassing libSystem.dylib"}
                    break
                }
            }
            if len(warnings) > 0 {
                break
            }
        }
    }

    syscalls := merged
    if !includeDebugInfo {
        syscalls = stripOccurrences(merged)
    }

    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     arch,
            AnalysisWarnings: warnings,
            DetectedSyscalls: syscalls,
        },
    }
}
```

**注意**: `warnings` の生成は `stripOccurrences` の前に行う。`Occurrences` は警告判定に使用されるため、除去前に処理する必要がある。

### 4.7 `analyzeELFSyscalls` 関数（`updateAnalysisRecord` 内の ELF 解析部分）

`AnalyzeSyscallsFromELF` の呼び出しを更新し、返された `stats` を `buildSyscallData` へ渡す。

**変更前（行 1112〜1134 付近）:**
```go
if v.syscallAnalyzer != nil {
    detected, evalResults, analyzeErr := v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
    ...
    directSyscalls = detected
    directArgEvalResults = evalResults
}
...
record.SyscallAnalysis = buildSyscallData(allSyscalls, argEvalResults, elfFile.Machine)
```

**変更後:**
```go
var directStats *common.SyscallDeterminationStats
if v.syscallAnalyzer != nil {
    detected, evalResults, stats, analyzeErr := v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
    ...
    directSyscalls = detected
    directArgEvalResults = evalResults
    directStats = stats
}
...
record.SyscallAnalysis = buildSyscallData(allSyscalls, argEvalResults, elfFile.Machine, directStats, v.includeDebugInfo)
```

### 4.8 `analyzeMachoSyscalls` 関数

`buildMachoSyscallData` の呼び出しに `v.includeDebugInfo` を追加する。

**変更前（行 885 付近）:**
```go
record.SyscallAnalysis = buildMachoSyscallData(svcEntries, combinedLibEntries, arch)
```

**変更後:**
```go
record.SyscallAnalysis = buildMachoSyscallData(svcEntries, combinedLibEntries, arch, v.includeDebugInfo)
```

### 4.9 テストの stub 更新

`validator_test.go` 内の stub 実装の `AnalyzeSyscallsFromELF` シグネチャを更新する。

**`stubSyscallAnalyzerReturnsOne`:**
```go
func (s *stubSyscallAnalyzerReturnsOne) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
    return []common.SyscallInfo{{Number: 1, Name: "write"}}, nil, nil, nil
}
```

**`stubSyscallAnalyzerReturnsNone`:**
```go
func (s *stubSyscallAnalyzerReturnsNone) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
    return nil, nil, nil, nil
}
```

## 5. `internal/libccache/adapters.go`

### 5.1 `SyscallAdapter.AnalyzeSyscallsFromELF`

戻り値に `*common.SyscallDeterminationStats` を追加し、elfanalyzer の結果から pass-through する。

```go
func (a *SyscallAdapter) AnalyzeSyscallsFromELF(elfFile *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
    result, err := a.analyzer.AnalyzeSyscallsFromELF(elfFile)
    if err != nil {
        if archErr, ok := errors.AsType[*elfanalyzer.UnsupportedArchitectureError](err); ok {
            return nil, nil, nil, fmt.Errorf("%w: %v", filevalidator.ErrUnsupportedArch, archErr.Machine)
        }
        return nil, nil, nil, err
    }
    if result == nil {
        return nil, nil, nil, nil
    }
    return result.DetectedSyscalls, result.ArgEvalResults, result.DeterminationStats, nil
}
```

## 6. テストと受け入れ条件の対応

### 6.1 新規テスト: `TestRecord_DebugInfo_ELF`

**ファイル**: `internal/filevalidator/validator_test.go`

**目的**: ELF バイナリでの `--debug-info` フラグ有無による `Occurrences` の違いを検証する。

```go
func TestRecord_DebugInfo_ELF(t *testing.T) {
    // stub が Occurrences 付きで SyscallInfo を返す
    // includeDebugInfo=false → Occurrences が nil であること
    // includeDebugInfo=true  → Occurrences が保持されること
}
```

**対応 AC**: AC-1（`occurrences` フィールドがデフォルトで含まれない）、AC-3（`--debug-info` 時に含まれる）

### 6.2 新規テスト: `TestRecord_DebugInfo_DeterminationStats`

**ファイル**: `internal/filevalidator/validator_test.go`

**目的**: `DeterminationStats` の制御を検証する。

```go
func TestRecord_DebugInfo_DeterminationStats(t *testing.T) {
    // stub が DeterminationStats 付きで結果を返す
    // includeDebugInfo=false → determination_stats が nil であること
    // includeDebugInfo=true  → determination_stats が保持されること
}
```

**対応 AC**: AC-2（`determination_stats` がデフォルトで含まれない）、AC-4（`--debug-info` 時に含まれる）

### 6.3 新規テスト: `TestBuildSyscallData_DebugInfo` / `TestBuildMachoSyscallData_DebugInfo`

**ファイル**: `internal/filevalidator/validator_test.go` / `validator_macho_test.go`

**目的**: `buildSyscallData` / `buildMachoSyscallData` のユニットテストとして、
フラグ有無による出力差異を検証する。

**対応 AC**: AC-5（ELF パス）、AC-6（Mach-O パス）

### 6.4 既存テスト: シグネチャ変更による呼び出し更新

#### `buildSyscallData` の既存テスト（`validator_test.go`）

`TestBuildSyscallAnalysisData` 内の `buildSyscallData` 直接呼び出し（行 1328, 1338, 1347 付近）に
`nil, true` を追加する（既存テストはデバッグ情報ありの動作を確認する）。

```go
data := buildSyscallData(all, nil, elf.EM_X86_64, nil, true)
```

#### `buildMachoSyscallData` の既存テスト（`validator_macho_test.go`）

`TestBuildMachoSyscallData_SVCOnly` 等の直接呼び出し（行 334, 601, 740, 755, 769, 787, 797 付近）に
`true` を追加する。

```go
result := buildMachoSyscallData(svcEntries, nil, "arm64", true)
```

#### `internal/libccache/adapters_test.go` の既存テスト

`SyscallAdapter.AnalyzeSyscallsFromELF` の戻り値シグネチャ変更に追従して、
stub とアサーションを更新する。

**意図**: `DeterminationStats` の pass-through と既存のエラーハンドリングが
維持されることを確認し、AC-7 / AC-9 の回帰を防ぐ。

### 6.5 テストで使用する stub 設計

`Occurrences` と `DeterminationStats` を返す stub を追加する。

```go
// stubSyscallAnalyzerWithDebugInfo は Occurrences と DeterminationStats を返す stub
type stubSyscallAnalyzerWithDebugInfo struct{}

func (s *stubSyscallAnalyzerWithDebugInfo) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
    return []common.SyscallInfo{
        {
            Number: 1,
            Name:   "write",
            Occurrences: []common.SyscallOccurrence{
                {Location: 0x1000, DeterminationMethod: elfanalyzer.DeterminationMethodImmediate},
            },
        },
    }, nil, &common.SyscallDeterminationStats{ImmediateTotal: 1}, nil
}
```

## 7. 受け入れ条件（AC）チェックリスト

要件定義書の成功基準と実装の対応:

| AC | 内容 | 実装箇所 |
|----|------|---------|
| AC-1 | デフォルト時に `occurrences` が含まれない | `buildSyscallData`（`includeDebugInfo=false`） |
| AC-2 | デフォルト時に `determination_stats` が含まれない | `buildSyscallData`（`stats=nil` で保存） |
| AC-3 | `--debug-info` 時に `occurrences` が含まれる | `buildSyscallData`（`includeDebugInfo=true`） |
| AC-4 | `--debug-info` 時に `determination_stats` が含まれる | `SyscallAdapter` pass-through + `buildSyscallData` |
| AC-5 | ELF バイナリで除去が機能する | `analyzeELFSyscalls` → `buildSyscallData` |
| AC-6 | Mach-O バイナリで除去が機能する | `analyzeMachoSyscalls` → `buildMachoSyscallData` |
| AC-7 | libc キャッシュ経由でも除去が機能する | `buildSyscallData`（libc 由来 syscall も `all` に含まれる） |
| AC-8 | デバッグ情報なしで記録したファイルを `verify` で検証できる | `verify` コマンドは `Occurrences` を参照しない（変更なし） |
| AC-9 | 既存テストが成功する | 既存テストのシグネチャ追従更新で回帰を防ぐ |
