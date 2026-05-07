# 詳細仕様書: exec syscall による高リスク検出

## 1. 概要

本ドキュメントは、exec syscall による高リスク検出機能の実装詳細を定義する。
アーキテクチャ設計は [02_architecture.md](02_architecture.md) を参照。

## 2. SyscallDefinition の拡張

### 2.1 フィールド追加

**ファイル**: `internal/security/elfanalyzer/syscall_numbers.go`

```go
// SyscallDefinition defines a single syscall.
type SyscallDefinition struct {
    Number    int
    Name      string
    IsNetwork bool
    IsExec    bool  // ★New: true if this syscall can execute a new process image
}
```

### 2.2 SyscallNumberTable インターフェースの拡張

**ファイル**: `internal/security/elfanalyzer/syscall_numbers.go`

```go
// SyscallNumberTable defines the interface for syscall number lookup.
type SyscallNumberTable interface {
    // GetSyscallName returns the syscall name for the given number.
    // Returns empty string if the number is unknown.
    GetSyscallName(number int) string

    // IsNetworkSyscall returns true if the syscall is network-related.
    IsNetworkSyscall(number int) bool

    // GetNetworkSyscalls returns all network-related syscall numbers.
    GetNetworkSyscalls() []int

    // IsExecSyscall returns true if the syscall can execute a new process image.
    IsExecSyscall(number int) bool  // ★New

    // GetExecSyscalls returns all exec-related syscall numbers.
    GetExecSyscalls() []int  // ★New
}
```

## 3. x86_64 syscall テーブルの変更

**ファイル**: `internal/security/elfanalyzer/x86_syscall_numbers.go`
（NOTE: このファイルは生成ファイル。生成スクリプト更新後に再生成すること。本セクションは検証用の期待値を記述する。）

### 3.1 構造体フィールドの追加

```go
type X86_64SyscallTable struct {
    syscalls       map[int]SyscallDefinition
    networkNumbers []int
    execNumbers    []int  // ★New
}
```

コンストラクタ内でのテーブル初期化ループを更新する:

```go
for _, def := range syscalls {
    table.syscalls[def.Number] = def
    if def.IsNetwork {
        table.networkNumbers = append(table.networkNumbers, def.Number)
    }
    if def.IsExec {  // ★New
        table.execNumbers = append(table.execNumbers, def.Number)
    }
}
```

### 3.2 IsExec フラグの設定値

以下のエントリが `IsExec = true` を持つこと。他の全エントリは `IsExec = false` であること。

| 番号 | 名称 | IsNetwork | IsExec |
|------|------|-----------|--------|
| 59 | execve | false | **true** |
| 322 | execveat | false | **true** |

### 3.3 追加メソッド

```go
// IsExecSyscall returns true if the syscall can execute a new process image.
func (t *X86_64SyscallTable) IsExecSyscall(number int) bool {
    if def, ok := t.syscalls[number]; ok {
        return def.IsExec
    }
    return false
}

// GetExecSyscalls returns all exec-related syscall numbers.
// Returns a copy to prevent callers from mutating the internal state.
func (t *X86_64SyscallTable) GetExecSyscalls() []int {
    result := make([]int, len(t.execNumbers))
    copy(result, t.execNumbers)
    return result
}
```

## 4. arm64 syscall テーブルの変更

**ファイル**: `internal/security/elfanalyzer/arm64_syscall_numbers.go`
（NOTE: 生成ファイル。§3 と同じパターンを arm64 に適用する。）

### 4.1 IsExec フラグの設定値

| 番号 | 名称 | IsNetwork | IsExec |
|------|------|-----------|--------|
| 221 | execve | false | **true** |
| 281 | execveat | false | **true** |

### 4.2 追加フィールドとメソッド

§3.1 / §3.3 と同じパターン（`ARM64LinuxSyscallTable` に適用）。

## 5. macOS syscall テーブルの変更

### 5.1 macOSSyscallEntry の拡張

**ファイル**: `internal/libccache/macos_syscall_table.go`

```go
// macOSSyscallEntry is the internal representation of a BSD syscall entry.
type macOSSyscallEntry struct {
    name      string
    isNetwork bool
    isExec    bool  // ★New
}
```

### 5.2 IsExecSyscall メソッドの追加

**ファイル**: `internal/libccache/macos_syscall_table.go`

```go
// IsExecSyscall returns true if the syscall can execute a new process image.
func (t MacOSSyscallTable) IsExecSyscall(number int) bool {
    if e, ok := macOSSyscallEntries[number]; ok {
        return e.isExec
    }
    return false
}
```

`MacOSSyscallTable` はこれにより `network_analyzer.go` の `syscallTableInterface` を満たす。

### 5.3 macOS exec syscall 一覧

**ファイル**: `internal/libccache/macos_syscall_numbers.go`
（NOTE: 生成ファイル。以下が `isExec: true` を持つこと。）

| 番号（BSD prefix 除く）| 名称 | isNetwork | isExec |
|------|------|-----------|--------|
| 59 | execve | false | **true** |
| 380 | \_\_mac\_execve | false | **true** |

## 6. network_analyzer.go の変更

**ファイル**: `internal/runner/base/security/network_analyzer.go`

### 6.1 syscallTableInterface の拡張

```go
// Before:
type syscallTableInterface interface {
    IsNetworkSyscall(number int) bool
}

// After:
type syscallTableInterface interface {
    IsNetworkSyscall(number int) bool
    IsExecSyscall(number int) bool  // ★New
}
```

### 6.2 syscallAnalysisHasExecSignal 関数の追加

`syscallAnalysisHasNetworkSignal` の直後に追加する。

```go
// syscallAnalysisHasExecSignal reports whether the given SyscallAnalysisResult
// contains any detected syscall classified as an exec syscall.
// Resolved svc entries (Number != -1) with exec classification are included.
func syscallAnalysisHasExecSignal(result *fileanalysis.SyscallAnalysisResult, goos string) bool { //nolint:unparam // goos varies by platform (darwin vs linux)
    if result == nil {
        return false
    }
    if len(result.DetectedSyscalls) == 0 {
        return false
    }
    table := syscallTableForArch(goos, result.Architecture)
    if table == nil {
        return false
    }
    for _, s := range result.DetectedSyscalls {
        if s.Number >= 0 && table.IsExecSyscall(s.Number) {
            return true
        }
    }
    return false
}
```

### 6.3 checkSyscallCache の変更

**変更前（抜粋）**:

```go
// Check whether any non-svc detected syscall is a network syscall.
if syscallAnalysisHasNetworkSignal(svcResult, a.goos) {
    slog.Info("SyscallAnalysis cache indicates network syscall",
        "path", cmdPath)
    return true, true, false  // ← handled=true で symbol analysis をスキップ
}
return false, false, false
```

**変更後**:

```go
isNet := syscallAnalysisHasNetworkSignal(svcResult, a.goos)
if isNet {
    slog.Info("SyscallAnalysis cache indicates network syscall",
        "path", cmdPath)
}
isExec := syscallAnalysisHasExecSignal(svcResult, a.goos)
if isExec {
    slog.Warn("SyscallAnalysis cache indicates exec syscall; treating as high risk",
        "path", cmdPath)
}
if isExec {
    // Exec = definitively high risk: caller may skip symbol analysis.
    return true, isNet, true
}
// Network-only or no signal: return handled=false so checkAnalysisCache
// still runs symbol analysis (dlopen detection etc.).
return false, isNet, false
```

**変更の理由**: `handled=true` の意味を「決定的 high-risk（これ以上の分析が不要）」に限定する。exec syscall は `handled=true` とし、呼び出し元が symbol analysis をスキップできる。一方、network-only の場合は `handled=false` とし、`checkAnalysisCache` が symbol analysis も実行して dlopen 等の high-risk シグナルを見落とさないようにする。

### 6.4 checkAnalysisCache の変更

`checkSyscallCache` の `isNet` シグナルを `handled=false` 時にも積算し、symbol analysis を常に実行するよう変更する。

**ファイル**: `internal/runner/base/security/network_analyzer.go`

**変更前（抜粋）**:

```go
// Check SyscallAnalysis cache for svc #0x80 signal (Mach-O arm64).
// This check runs regardless of SymbolAnalysis result so that svc #0x80 always
// escalates hasDynLoad to true.
if handled, isNet, dynLoad := a.checkSyscallCache(cmdPath, contentHash); handled {
    return true, isNet, dynLoad  // ← handled=false のとき isNet を捨てる
}

// No svc #0x80 signal: accumulate network/dynload signals from SymbolAnalysis.
if data != nil {
    output := buildAnalysisOutputFromSymbolData(data)
    isNetwork, hasDynLoad = handleAnalysisOutput(output, cmdPath)
}
return false, isNetwork, hasDynLoad
```

**変更後**:

```go
syscallHandled, syscallIsNet, syscallIsHighRisk := a.checkSyscallCache(cmdPath, contentHash)
if syscallHandled {
    // Definitively high risk (svc or exec): early return.
    // Symbol analysis cannot escalate further.
    return true, syscallIsNet, syscallIsHighRisk
}
// Accumulate syscall network signal even when not definitively handled.
isNetwork = syscallIsNet
hasDynLoad = syscallIsHighRisk

// Always run symbol analysis to detect dlopen etc.
// Runs even when a network syscall was detected, preventing dlopen from being missed.
if data != nil {
    output := buildAnalysisOutputFromSymbolData(data)
    symIsNetwork, symIsHighRisk := handleAnalysisOutput(output, cmdPath)
    isNetwork = isNetwork || symIsNetwork
    hasDynLoad = hasDynLoad || symIsHighRisk
}
if isNetwork || hasDynLoad {
    return true, isNetwork, hasDynLoad
}
return false, false, false
```

## 7. dynlib 依存ライブラリの exec 検出

**ファイル**: `internal/runner/base/security/network_analyzer.go`

### 7.1 depSignals 構造体の拡張

```go
// depSignals holds the network/risk signals extracted from one library analysis result.
type depSignals struct {
    dynLoadSymbols  []string
    networkSymbols  []string
    networkSyscall  string
    execSyscall     string  // ★New: name of first detected exec syscall, or ""
    mprotectRisk    common.SyscallArgEvalResult
    hasMprotectRisk bool
}
```

### 7.2 firstExecSyscall 関数の追加

`firstNetworkSyscall` の直後に追加する。

```go
// firstExecSyscall returns the name of the first exec syscall found in
// data using table for classification. Returns "" if none found or inputs are nil.
func firstExecSyscall(table syscallTableInterface, data *fileanalysis.SyscallAnalysisData) string { //nolint:unparam // table varies by platform (darwin vs linux)
    if table == nil || data == nil {
        return ""
    }
    for _, s := range data.DetectedSyscalls {
        if s.Number >= 0 && table.IsExecSyscall(s.Number) {
            return s.Name
        }
    }
    return ""
}
```

### 7.3 analyzeDepSignals の拡張

`result.SyscallAnalysis != nil` ブロック内に exec syscall 検出を追加する。

```go
func (a *NetworkAnalyzer) analyzeDepSignals(result *dynamicanalysis.Result) depSignals {
    var s depSignals
    s.dynLoadSymbols = result.DynamicLoadSymbols()
    if result.SymbolAnalysis != nil {
        s.networkSymbols = result.SymbolAnalysis.DetectedSymbols
    }
    if result.SyscallAnalysis != nil {
        table := syscallTableForArch(a.goos, result.SyscallAnalysis.Architecture)
        s.networkSyscall = firstNetworkSyscall(table, result.SyscallAnalysis)
        s.execSyscall = firstExecSyscall(table, result.SyscallAnalysis)  // ★New
        s.mprotectRisk, s.hasMprotectRisk = elfanalyzer.FirstMprotectRisk(result.SyscallAnalysis.ArgEvalResults)
    }
    return s
}
```

### 7.4 checkDynLibDepsNetwork の拡張

`sigs.hasMprotectRisk` の処理の後に exec signal のハンドリングを追加する。

```go
var (
    dynLoadLog  onceLogger
    networkLog  onceLogger
    mprotectLog onceLogger
    execLog     onceLogger  // ★New
)

// ...（既存の dynLoadLog, networkLog, mprotectLog 処理）...

if sigs.execSyscall != "" {  // ★New
    execLog.log("dynlib analysis detected exec syscall; treating as high risk",
        "cmd_path", cmdPath, "dep_path", dep.Path, "syscall", sigs.execSyscall)
    isHighRisk = true
}
```

## 8. 生成スクリプトの変更

**ファイル**: `scripts/generate_syscall_table.py`

### 7.1 exec syscall 定数の追加

```python
# ---------------------------------------------------------------------------
# Exec syscall names (architecture-independent names).
# These are the syscalls that can execute a new process image.
# ---------------------------------------------------------------------------
EXEC_SYSCALL_NAMES = {
    "execve",
    "execveat",
}

# macOS-specific exec syscall names.
MACOS_EXEC_SYSCALL_NAMES = {
    "execve",
    "__mac_execve",
}
```

### 7.2 Linux テーブル生成の変更（build_body 関数）

```python
# Before:
is_network = "true" if name in NETWORK_SYSCALL_NAMES else "false"
lines.append(f'\t\t{{{num}, "{name}", {is_network}}},')

# After:
is_network = "true" if name in NETWORK_SYSCALL_NAMES else "false"
is_exec = "true" if name in EXEC_SYSCALL_NAMES else "false"
lines.append(f'\t\t{{{num}, "{name}", {is_network}, {is_exec}}},')
```

また、テーブル初期化ループに exec numbers の追加を反映する（§3.1 参照）。

### 7.3 STRUCT_TEMPLATE の変更

構造体定義テンプレートを更新し、`execNumbers []int` フィールドと `IsExecSyscall` / `GetExecSyscalls` メソッドを生成する。

```python
STRUCT_TEMPLATE = """\
...
// {struct_name} implements SyscallNumberTable for {arch_desc}.
type {struct_name} struct {{
\tsyscalls       map[int]SyscallDefinition
\tnetworkNumbers []int
\texecNumbers    []int
}}
...
// IsExecSyscall returns true if the syscall can execute a new process image.
func (t *{struct_name}) IsExecSyscall(number int) bool {{
\tif def, ok := t.syscalls[number]; ok {{
\t\treturn def.IsExec
\t}}
\treturn false
}}

// GetExecSyscalls returns all exec-related syscall numbers.
// Returns a copy to prevent callers from mutating the internal state.
func (t *{struct_name}) GetExecSyscalls() []int {{
\tresult := make([]int, len(t.execNumbers))
\tcopy(result, t.execNumbers)
\treturn result
}}
"""
```

### 7.4 macOS テーブル生成の変更

`generate_macos` 関数内のエントリ生成を更新する。

```python
# Before:
is_network = "true" if is_macos_network_syscall(name) else "false"
lines.append(f'\t{number}:\t{{name: "{name}", isNetwork: {is_network}}},')

# After:
is_network = "true" if is_macos_network_syscall(name) else "false"
is_exec = "true" if name in MACOS_EXEC_SYSCALL_NAMES else "false"
lines.append(f'\t{number}:\t{{name: "{name}", isNetwork: {is_network}, isExec: {is_exec}}},')
```

## 8. テスト仕様

### 8.1 x86_64 syscall テーブルのテスト

**ファイル**: `internal/security/elfanalyzer/x86_syscall_numbers_test.go`

`TestX86_64SyscallTable_IsExecSyscall` を追加する。

| テストケース | 入力（syscall 番号）| 期待値 |
|---|---|---|
| execve | 59 | true |
| execveat | 322 | true |
| socket（network, not exec）| 41 | false |
| read | 0 | false |
| 存在しない番号 | -1 | false |
| 存在しない番号 | 9999 | false |

`TestX86_64SyscallTable_GetExecSyscalls` を追加する。

| テストケース | 期待値 |
|---|---|
| 返り値に 59（execve）が含まれる | true |
| 返り値に 322（execveat）が含まれる | true |
| 返り値の長さが 2 である | true |
| 返り値が slice のコピーである（内部状態を変更しない）| true |

### 8.2 arm64 syscall テーブルのテスト

**ファイル**: `internal/security/elfanalyzer/arm64_syscall_numbers_test.go`

`TestARM64LinuxSyscallTable_IsExecSyscall` を追加する。

| テストケース | 入力（syscall 番号）| 期待値 |
|---|---|---|
| execve | 221 | true |
| execveat | 281 | true |
| socket（arm64 の番号）| 198 | false |
| read | 63 | false |
| 存在しない番号 | -1 | false |

### 8.3 macOS syscall テーブルのテスト

**ファイル**: `internal/libccache/macos_syscall_table_test.go`

`TestMacOSSyscallTable_IsExecSyscall` を追加する。

| テストケース | 入力（syscall 番号）| 期待値 |
|---|---|---|
| execve | 59 | true |
| \_\_mac\_execve | 380 | true |
| socket（macOS）| 97 | false |
| read | 3 | false |
| 存在しない番号 | -1 | false |

### 8.4 syscallAnalysisHasExecSignal のテスト

**ファイル**: `internal/runner/base/security/network_analyzer_test.go`

`TestSyscallAnalysisHasExecSignal` を追加する（ `syscallAnalysisHasNetworkSignal` に対応する既存テストのパターンに従う）。

| テストケース | SyscallAnalysisResult の内容 | 期待値 |
|---|---|---|
| execve を含む | `{Number: 59, Name: "execve"}` (x86\_64) | true |
| execveat を含む | `{Number: 322, Name: "execveat"}` (x86\_64) | true |
| network syscall のみ | `{Number: 41, Name: "socket"}` | false |
| exec syscall なし | write のみ | false |
| DetectedSyscalls が空 | `[]` | false |
| result が nil | nil | false |

### 8.5 checkSyscallCache のテスト

**ファイル**: `internal/runner/base/security/network_analyzer_test.go`

`TestNetworkAnalyzer_ExecSyscallIsHighRisk` を追加する。

| テストケース | SyscallAnalysisResult の内容 | 期待 isNetwork | 期待 isHighRisk |
|---|---|---|---|
| exec syscall のみ（execve）| x86\_64, execve(59) | false | true |
| exec + network | x86\_64, socket(41) + execve(59) | true | true |
| network のみ | x86\_64, socket(41) | true | false |
| exec syscall なし | x86\_64, write(1) | false | false |

**実装上の注意**: これらのテストは `checkSyscallCache` を直接テストするのではなく、`IsNetworkOperation` を通じて統合的にテストする。テスト用のモック `SyscallAnalysisResult` を準備し、`NetworkAnalyzer` に渡す。

### 8.6 dynlib exec 検出のテスト

**ファイル**: `internal/runner/base/security/network_analyzer_test.go`

`TestFirstExecSyscall` を追加する（`firstNetworkSyscall` の既存テストパターンに従う）。

| テストケース | 入力 | 期待値 |
|---|---|---|
| execve を含む SyscallAnalysisData | `{Number: 59, Name: "execve"}` (x86\_64) | `"execve"` |
| network syscall のみ | `{Number: 41, Name: "socket"}` | `""` |
| DetectedSyscalls が空 | `[]` | `""` |
| table が nil | nil | `""` |
| data が nil | nil | `""` |

`TestAnalyzeDepSignals_ExecSyscall` を追加する。

| テストケース | dynamicanalysis.Result の内容 | 期待 execSyscall |
|---|---|---|
| execve を含む SyscallAnalysis | execve(59) | `"execve"` |
| exec syscall なし | write(1) | `""` |
| SyscallAnalysis が nil | nil | `""` |

`TestNetworkAnalyzer_DynLibExecSyscallIsHighRisk` を追加する。

| テストケース | dynlib SyscallAnalysis の内容 | 期待 isNetwork | 期待 isHighRisk |
|---|---|---|---|
| dynlib に execve のみ | x86\_64, execve(59) | false | true |
| dynlib に network + exec | x86\_64, socket(41) + execve(59) | true | true |
| dynlib に exec syscall なし | x86\_64, write(1) | false | false |

### 8.7 既存テストの回帰確認

- `TestSyscallAnalysisHasNetworkSignal` が引き続きパスすること（既存テスト）
- network only のケースで `isHighRisk` が `false` のままであることを確認する

## 9. 変更ファイル一覧と変更種別

### 9.1 実装ファイル

| ファイル | 変更種別 | 概要 |
|---|---|---|
| `internal/security/elfanalyzer/syscall_numbers.go` | 変更 | `SyscallDefinition.IsExec`, `SyscallNumberTable` インターフェース拡張 |
| `internal/security/elfanalyzer/x86_syscall_numbers.go` | 再生成 | `execNumbers`, `IsExecSyscall`, `GetExecSyscalls`, execve/execveat に `IsExec=true` |
| `internal/security/elfanalyzer/arm64_syscall_numbers.go` | 再生成 | 同上（arm64 番号）|
| `internal/security/elfanalyzer/x86_syscall_numbers_test.go` | 変更 | `TestX86_64SyscallTable_IsExecSyscall`, `TestX86_64SyscallTable_GetExecSyscalls` 追加 |
| `internal/security/elfanalyzer/arm64_syscall_numbers_test.go` | 変更 | `TestARM64LinuxSyscallTable_IsExecSyscall` 追加 |
| `internal/libccache/macos_syscall_table.go` | 変更 | `macOSSyscallEntry.isExec`, `MacOSSyscallTable.IsExecSyscall` 追加 |
| `internal/libccache/macos_syscall_numbers.go` | 再生成 | execve/\_\_mac\_execve に `isExec: true` |
| `internal/libccache/macos_syscall_table_test.go` | 変更 | `TestMacOSSyscallTable_IsExecSyscall` 追加 |
| `internal/runner/base/security/network_analyzer.go` | 変更 | `syscallTableInterface` 拡張, `syscallAnalysisHasExecSignal`, `checkSyscallCache` 更新（handled=true を exec/SVC のみに限定）, `checkAnalysisCache` 更新（syscall isNet を積算・symbol analysis を常時実行）, `firstExecSyscall`, `depSignals.execSyscall`, `analyzeDepSignals` 更新, `checkDynLibDepsNetwork` 更新 |
| `internal/runner/base/security/network_analyzer_test.go` | 変更 | `TestSyscallAnalysisHasExecSignal`, `TestNetworkAnalyzer_ExecSyscallIsHighRisk`, `TestFirstExecSyscall`, `TestAnalyzeDepSignals_ExecSyscall`, `TestNetworkAnalyzer_DynLibExecSyscallIsHighRisk` 追加 |
| `scripts/generate_syscall_table.py` | 変更 | `EXEC_SYSCALL_NAMES`, `MACOS_EXEC_SYSCALL_NAMES`, `IsExec` フィールド生成 |

### 9.2 ドキュメントファイル

| ファイル | 変更種別 | 概要 |
|---|---|---|
| `docs/user/toml_config/06_command_level.md` | 変更 | 「Risk Assessment Mechanism」に exec syscall 検出を追記 |
| `docs/user/toml_config/06_command_level.ja.md` | 変更 | 同上、日本語版 |
| `docs/dev/architecture_design/security-architecture.md` | 変更 | syscall 解析の説明・Security Guarantees・Threat Model に exec syscall 検出を追記 |
| `docs/dev/architecture_design/security-architecture.ja.md` | 変更 | 同上、日本語版 |

## 11. ドキュメント変更仕様

### 11.1 `docs/user/toml_config/06_command_level.md`

「Risk Assessment Mechanism」セクションの番号付きリストに項目を追加する。

**変更前:**
```
go-safe-cmd-runner automatically assesses command risk from the following elements:

1. **Command Type**: Dangerous commands like rm, chmod, chown
2. **Argument Patterns**: Recursive deletion (-rf), forced execution (-f), etc.
3. **Privilege Escalation**: Use of run_as_user, run_as_group
4. **Network Access**: Network commands like curl, wget
```

**変更後:**
```
go-safe-cmd-runner automatically assesses command risk from the following elements:

1. **Command Type**: Dangerous commands like rm, chmod, chown
2. **Argument Patterns**: Recursive deletion (-rf), forced execution (-f), etc.
3. **Privilege Escalation**: Use of run_as_user, run_as_group
4. **Network Access**: Network commands like curl, wget
5. **Binary Analysis**: Binaries detected with exec syscalls (execve/execveat) via
   static syscall analysis are automatically classified as high risk. Binaries with
   dynamic loading symbols (dlopen/dlsym) or mprotect+PROT_EXEC patterns are also
   classified as high risk.
```

### 11.2 `docs/user/toml_config/06_command_level.ja.md`

「リスク評価の仕組み」セクションの番号付きリストに項目を追加する。

**変更前:**
```
go-safe-cmd-runner は以下の要素からコマンドのリスクを自動評価します:

1. **コマンドの種類**: rm, chmod, chown などの危険なコマンド
2. **引数パターン**: 再帰削除(-rf)、強制実行(-f)など
3. **権限昇格**: run_as_user, run_as_group の使用
4. **ネットワークアクセス**: curl, wget などのネットワークコマンド
```

**変更後:**
```
go-safe-cmd-runner は以下の要素からコマンドのリスクを自動評価します:

1. **コマンドの種類**: rm, chmod, chown などの危険なコマンド
2. **引数パターン**: 再帰削除(-rf)、強制実行(-f)など
3. **権限昇格**: run_as_user, run_as_group の使用
4. **ネットワークアクセス**: curl, wget などのネットワークコマンド
5. **バイナリ解析**: 静的 syscall 解析により exec syscall（execve/execveat）の使用が
   検出されたバイナリは自動的に高リスクに分類される。動的ロードシンボル（dlopen/dlsym）や
   mprotect+PROT_EXEC パターンも同様に高リスクとして分類される。
```

### 11.3 `docs/dev/architecture_design/security-architecture.md`

**変更箇所 1: syscall analysis の説明（§2 "Analysis content"）**

`- **syscall analysis** ...` の bullet 末尾に追記する。

> Additionally, detects exec syscalls (execve/execveat) that replace the process image with an arbitrary binary; at runner execution time, this is classified as a high-risk signal alongside mprotect+PROT_EXEC.

**変更箇所 2: Security Guarantees（§2）**

既存リストに項目を追加する。

> - Detection of exec syscall usage (execve/execveat) that allows process image replacement with arbitrary executables; classified as high risk at runner execution time

**変更箇所 3: Threat Model "Dangerous Binary Execution"**

Threats リストに追加:
> - Process image replacement via exec syscalls (execve/execveat), enabling execution of arbitrary binaries that may have network capabilities or other dangerous behaviors

Countermeasures リストに追加:
> - Detection of exec syscall usage (execve/execveat) at runner execution time, automatically classifying such binaries as high risk

### 11.4 `docs/dev/architecture_design/security-architecture.ja.md`

§11.3 と同内容を日本語で対応する箇所に適用する。

**変更箇所 1: syscall 解析の説明（syscall analysis bullet 末尾）**

> また、プロセスイメージを任意のバイナリに置き換える exec syscall（execve/execveat）の使用を検出する。runner 実行時に mprotect+PROT_EXEC と同様の高リスクシグナルとして分類される。

**変更箇所 2: Security Guarantees リスト**

> - exec syscall（execve/execveat）の使用検出（任意の実行ファイルへのプロセス置換を可能にする）；runner 実行時に高リスクとして分類

**変更箇所 3: Threat Model "危険なバイナリの実行" Threats**

> - exec syscall（execve/execveat）によるプロセスイメージの置換。ネットワーク機能や他の危険な動作を持つ任意のバイナリの実行が可能になる

**変更箇所 4: Countermeasures**

> - runner 実行時における exec syscall（execve/execveat）使用の検出。該当バイナリは自動的に高リスクに分類される

## 10. 実装上の注意点

### 10.1 スキーマバージョンの変更不要

exec syscall の検出は既存の `SyscallAnalysisData.DetectedSyscalls` を参照するのみ。保存形式の変更がないため `CurrentSchemaVersion` の変更は不要である。

### 10.2 dynlib 依存ライブラリへの非適用

`checkDynLibDepsNetwork` は dynlib 依存ライブラリの network / dynload シグナルを集約するが、本タスクでは dynlib の exec syscall 検出は対象外とする。将来タスクとして別途対応する。

### 10.3 `GetNetworkSyscalls` との対称性

`GetExecSyscalls` は `GetNetworkSyscalls` と同じ実装パターン（内部スライスのコピーを返す）に従う。

### 10.4 `syscallTableInterface` の拡張影響

`syscallTableInterface` に `IsExecSyscall` を追加すると、このインターフェースを満たす全ての型が `IsExecSyscall` を実装する必要がある。現在の実装では以下の型がこのインターフェースを満たす:
- `elfanalyzer.X86_64SyscallTable`
- `elfanalyzer.ARM64LinuxSyscallTable`
- `libccache.MacOSSyscallTable`

`syscallTableInterface` のモック実装はテストコード中に存在しない。`syscallTableForArch` は常に上記の具体型を返すため、各具体型への実装追加で対応が完結する。インターフェース追加後にビルドエラーが発生した場合は、上記の具体型への実装漏れを確認すること。
