# アーキテクチャ設計書: exec syscall による高リスク検出

## 1. システム概要

### 1.1 アーキテクチャ目標

- 既存の syscall 静的解析インフラ（タスク 0070/0072/0097）を再利用し、exec syscall 検出を追加する
- `SyscallDefinition` / `macOSSyscallEntry` に `IsExec` フラグを追加することで、ネットワーク分類と対称な実装を実現する
- `checkSyscallCache` 内で exec signal を検出し、`isHighRisk = true` にマッピングする
- 生成スクリプト（`generate_syscall_table.py`）を更新することで、将来のアーキテクチャ追加時の保守コストを最小化する

### 1.2 設計原則

- **既存活用**: network syscall 検出パターン（`IsNetwork`, `syscallAnalysisHasNetworkSignal`）をそのまま exec 検出に適用する
- **対称性**: exec 関連フィールド名・メソッド名は network 関連の命名規則に従う
- **最小変更**: 新規ファイルの作成は不要。既存ファイルへの追加のみ
- **フェイルクローズ不変**: exec 検出の追加は既存の安全側へ倒す設計に影響しない

### 1.3 スコープと位置づけ

本タスクは既存の syscall 静的解析インフラの拡張であり、解析済みの結果（`SyscallAnalysisResult`）から exec syscall を識別するフラグとロジックを追加する。

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91
    classDef existing fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00
    classDef new fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400

    A[("主バイナリ<br>SyscallAnalysisResult")] --> B["checkSyscallCache<br>(既存)"]
    B -->|"network signal"| C["isNetwork = true<br>(既存)"]
    B -->|"exec signal"| D["isHighRisk = true<br>(新規)"]
    B -->|"svc signal"| E["isHighRisk = true<br>(既存)"]

    F[("dynlib deps<br>SyscallAnalysisData")] --> G["analyzeDepSignals<br>(既存)"]
    G -->|"exec syscall"| D

    H[("shebang interpreter")] --> I["analyzeBinarySignals<br>再帰呼び出し（既存）"]
    I --> B
    I --> G

    class A,F,H data
    class B,C,E,G,I existing
    class D new
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91
    classDef existing fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00
    classDef new fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400

    D1[("Data")] --> S1["Existing Component"] --> S2["New/Enhanced"]
    class D1 data
    class S1 existing
    class S2 new
```

## 2. システム構成

### 2.1 変更対象ファイル一覧

```mermaid
flowchart TD
    classDef existing fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400

    subgraph "internal/security/elfanalyzer"
        A["syscall_numbers.go<br>(拡張: SyscallDefinition.IsExec,<br>SyscallNumberTable.IsExecSyscall)"]
        B["x86_syscall_numbers.go<br>(拡張: IsExec=true for execve/execveat,<br>IsExecSyscall メソッド追加)"]
        C["arm64_syscall_numbers.go<br>(拡張: IsExec=true for execve/execveat,<br>IsExecSyscall メソッド追加)"]
    end

    subgraph "internal/libccache"
        D["macos_syscall_table.go<br>(拡張: macOSSyscallEntry.isExec,<br>IsExecSyscall メソッド追加)"]
        E["macos_syscall_numbers.go<br>(拡張: isExec=true for execve/__mac_execve)"]
    end

    subgraph "internal/runner/base/security"
        F["network_analyzer.go<br>(拡張: syscallTableInterface.IsExecSyscall,<br>syscallAnalysisHasExecSignal,<br>checkSyscallCache 更新,<br>firstExecSyscall,<br>depSignals.execSyscall,<br>analyzeDepSignals 更新,<br>checkDynLibDepsNetwork 更新)"]
    end

    subgraph "scripts"
        G["generate_syscall_table.py<br>(拡張: EXEC_SYSCALL_NAMES,<br>MACOS_EXEC_SYSCALL_NAMES,<br>IsExec フィールド生成)"]
    end

    class A,B,C,D,E,F,G enhanced
```

### 2.2 データ構造の変更

#### 2.2.1 SyscallDefinition（elfanalyzer パッケージ）

```mermaid
classDiagram
    class SyscallDefinition {
        Number int
        Name string
        IsNetwork bool
        IsExec bool  ★New
    }
```

`IsNetwork` フィールドと同じパターンで `IsExec` を追加する。

#### 2.2.2 SyscallNumberTable インターフェース（elfanalyzer パッケージ）

```mermaid
classDiagram
    class SyscallNumberTable {
        <<interface>>
        +GetSyscallName(number int) string
        +IsNetworkSyscall(number int) bool
        +GetNetworkSyscalls() []int
        +IsExecSyscall(number int) bool  ★New
        +GetExecSyscalls() []int         ★New
    }

    class X86_64SyscallTable {
        -syscalls map[int]SyscallDefinition
        -networkNumbers []int
        -execNumbers []int  ★New
        +IsExecSyscall(number int) bool
        +GetExecSyscalls() []int
    }

    class ARM64LinuxSyscallTable {
        -syscalls map[int]SyscallDefinition
        -networkNumbers []int
        -execNumbers []int  ★New
        +IsExecSyscall(number int) bool
        +GetExecSyscalls() []int
    }

    SyscallNumberTable <|.. X86_64SyscallTable
    SyscallNumberTable <|.. ARM64LinuxSyscallTable
```

#### 2.2.3 macOSSyscallEntry と MacOSSyscallTable（libccache パッケージ）

```mermaid
classDiagram
    class macOSSyscallEntry {
        name string
        isNetwork bool
        isExec bool  ★New
    }

    class MacOSSyscallTable {
        +GetSyscallName(number int) string
        +IsNetworkSyscall(number int) bool
        +IsExecSyscall(number int) bool  ★New
    }
```

**注意**: `libccache.SyscallNumberTable` インターフェースは `GetSyscallName` と `IsNetworkSyscall` のみを定義し、`MacOSSyscallTable` の `IsExecSyscall` は `libccache.SyscallNumberTable` の外部に実装する。これは `libccache.SyscallNumberTable` が exec 分類とは無関係な用途（`ImportSymbolMatcher`）で使用されているためである。`MacOSSyscallTable` は `network_analyzer.go` の `syscallTableInterface` を通じて使用される。

### 2.3 detection フローの変更

#### 2.3.1 syscallTableInterface の拡張

```go
// Before:
type syscallTableInterface interface {
    IsNetworkSyscall(number int) bool
}

// After:
type syscallTableInterface interface {
    IsNetworkSyscall(number int) bool
    IsExecSyscall(number int) bool   // ★New
}
```

#### 2.3.2 checkSyscallCache の拡張

```mermaid
flowchart TD
    classDef existing fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00
    classDef new fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400
    classDef decision fill:#fff9e6,stroke:#d4a500,stroke-width:1px,color:#6b5200

    A["LoadSyscallAnalysis"] --> B{"svc #0x80?"}
    B -->|"Yes"| C["handled=true<br>isNetwork=true<br>isHighRisk=true"]
    B -->|"No"| D{"network syscall?"}
    D -->|"Yes"| E["isNetwork = true"]
    D -->|"No"| F["isNetwork = false"]
    E --> G{"exec syscall?"}
    F --> G
    G -->|"Yes"| H["isHighRisk = true"]
    G -->|"No"| I["isHighRisk = false"]
    H --> J{"isNetwork OR isHighRisk?"}
    I --> J
    J -->|"Yes"| K["return handled=true,<br>isNetwork, isHighRisk"]
    J -->|"No"| L["return handled=false"]

    class A,B,C existing
    class D,E,F existing
    class G,H,I new
    class J,K,L existing
```

**設計上の注意**: 既存の実装では `syscallAnalysisHasNetworkSignal` が true の場合に即座に `return true, true, false` していた。exec signal との組み合わせを正しく処理するため、early return を廃止し、両シグナルを評価してから返すように変更する。

## 3. データフロー

### 3.1 事前解析フェーズ（変更なし）

exec syscall の記録は既存の syscall 静的解析（タスク 0070/0072/0097）で行われる。`execve` は `SyscallDefinition.IsExec = true` としてテーブルに定義されているが、解析結果の JSON 形式（`SyscallAnalysisData`）は変更不要である。解析結果には syscall 番号と名称が記録されており、実行時に `IsExecSyscall(number)` でフィルタリングする。

### 3.2 実行時フェーズ

```mermaid
sequenceDiagram
    participant E as "EvaluateRisk"
    participant NA as "NetworkAnalyzer"
    participant CC as "checkSyscallCache"
    participant SS as "SyscallStore"
    participant ST as "SyscallTable"

    E->>NA: IsNetworkOperation(cmdPath, args, hash)
    NA->>NA: analyzeBinarySignals(cmdPath, hash)
    NA->>NA: checkAnalysisCache(cmdPath, hash)
    NA->>CC: checkSyscallCache(cmdPath, hash)
    CC->>SS: LoadSyscallAnalysis(cmdPath, hash)
    SS-->>CC: SyscallAnalysisResult

    Note over CC: svc signal check (既存)
    CC->>CC: syscallAnalysisHasSVCSignal(result)

    Note over CC: network signal check (既存)
    CC->>ST: IsNetworkSyscall(number) per syscall
    ST-->>CC: bool

    Note over CC: exec signal check (新規)
    CC->>CC: syscallAnalysisHasExecSignal(result, table)
    CC->>ST: IsExecSyscall(number) per syscall
    ST-->>CC: bool

    CC-->>NA: (handled, isNetwork, isHighRisk)
    NA-->>E: (isNetwork, isHighRisk, nil)
    E-->>E: RiskLevelHigh (isHighRisk=true の場合)
```

## 4. コンポーネント設計

### 4.1 検出対象のカバレッジ

| 検出対象 | 実装箇所 | 新規実装要否 |
|---|---|---|
| 主バイナリの exec syscall | `checkSyscallCache` | 要（FR-3.2.3） |
| dynlib 依存ライブラリの exec syscall | `analyzeDepSignals` + `firstExecSyscall` | 要（FR-3.2.4） |
| shebang インタープリタの exec syscall | `followShebangChain` → `analyzeBinarySignals` 再帰 → `checkSyscallCache` | 不要（FR-3.2.3 完了で自動カバー） |
| shebang インタープリタの dynlib 依存の exec syscall | `followShebangChain` → `analyzeBinarySignals` 再帰 → `checkDynLibDepsNetwork` | 不要（FR-3.2.4 完了で自動カバー） |

### 4.2 syscallAnalysisHasExecSignal 関数

`syscallAnalysisHasNetworkSignal` と同じパターンで実装する。

```go
// syscallAnalysisHasExecSignal reports whether the given SyscallAnalysisResult
// contains any detected syscall classified as an exec syscall.
func syscallAnalysisHasExecSignal(result *fileanalysis.SyscallAnalysisResult, goos string) bool {
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

### 4.3 checkSyscallCache の変更

```go
// Before (network signal の early return を廃止):
if syscallAnalysisHasNetworkSignal(svcResult, a.goos) {
    slog.Info("SyscallAnalysis cache indicates network syscall", "path", cmdPath)
    return true, true, false  // ← early return で exec signal を見逃す
}

// After (両シグナルを評価してから返す):
isNet := syscallAnalysisHasNetworkSignal(svcResult, a.goos)
isExec := syscallAnalysisHasExecSignal(svcResult, a.goos)

if isNet {
    slog.Info("SyscallAnalysis cache indicates network syscall", "path", cmdPath)
}
if isExec {
    slog.Warn("SyscallAnalysis cache indicates exec syscall; treating as high risk", "path", cmdPath)
}
if isNet || isExec {
    return true, isNet, isExec
}
return false, false, false
```

### 4.4 dynlib 依存ライブラリの exec 検出

`analyzeDepSignals` に exec syscall 検出を追加する。

```go
// depSignals の拡張:
type depSignals struct {
    dynLoadSymbols  []string
    networkSymbols  []string
    networkSyscall  string
    execSyscall     string  // ★New
    mprotectRisk    common.SyscallArgEvalResult
    hasMprotectRisk bool
}

// analyzeDepSignals の拡張（result.SyscallAnalysis != nil ブロック内）:
s.networkSyscall = firstNetworkSyscall(table, result.SyscallAnalysis)
s.execSyscall = firstExecSyscall(table, result.SyscallAnalysis)  // ★New
```

`checkDynLibDepsNetwork` に exec signal のハンドリングを追加する。

```go
if sigs.execSyscall != "" {
    execLog.log("dynlib analysis detected exec syscall; treating as high risk",
        "cmd_path", cmdPath, "dep_path", dep.Path, "syscall", sigs.execSyscall)
    isHighRisk = true
}
```

`firstExecSyscall` は `firstNetworkSyscall` と同一のパターンで実装し、`table.IsExecSyscall` を使用する。

### 4.5 syscall テーブルの拡張パターン

`X86_64SyscallTable` と `ARM64LinuxSyscallTable` の両方で同一パターンを適用する。

```go
// SyscallDefinition のエントリ例（x86_64）:
{59, "execve", false, true},     // IsNetwork=false, IsExec=true
{322, "execveat", false, true},  // IsNetwork=false, IsExec=true

// テーブル構造体:
type X86_64SyscallTable struct {
    syscalls       map[int]SyscallDefinition
    networkNumbers []int
    execNumbers    []int  // ★New
}

// IsExecSyscall メソッド:
func (t *X86_64SyscallTable) IsExecSyscall(number int) bool {
    if def, ok := t.syscalls[number]; ok {
        return def.IsExec
    }
    return false
}

// GetExecSyscalls メソッド:
func (t *X86_64SyscallTable) GetExecSyscalls() []int {
    result := make([]int, len(t.execNumbers))
    copy(result, t.execNumbers)
    return result
}
```

### 4.6 生成スクリプトの変更

```python
# 追加するセット定数:
EXEC_SYSCALL_NAMES = {
    "execve",
    "execveat",
}

MACOS_EXEC_SYSCALL_NAMES = {
    "execve",
    "__mac_execve",
}

# SyscallDefinition の生成ロジック（build_body 内）:
# Before:
is_network = "true" if name in NETWORK_SYSCALL_NAMES else "false"
lines.append(f'\t\t{{{num}, "{name}", {is_network}}},')

# After:
is_network = "true" if name in NETWORK_SYSCALL_NAMES else "false"
is_exec = "true" if name in EXEC_SYSCALL_NAMES else "false"
lines.append(f'\t\t{{{num}, "{name}", {is_network}, {is_exec}}},')
```

生成テンプレートも更新し、`execNumbers` フィールドの初期化と `IsExecSyscall` / `GetExecSyscalls` メソッドの生成を追加する。

## 5. セキュリティアーキテクチャ

### 5.1 リスク判定マトリクス

| 検出シグナル | isNetwork | isHighRisk | 最終リスクレベル |
|---|---|---|---|
| exec syscall のみ | false | true | High |
| network syscall のみ | true | false | Medium |
| exec + network syscall | true | true | High |
| exec なし、network なし | false | false | Low（他要因なし） |

### 5.2 exec signal の位置づけ

exec signal は `isHighRisk` に分類される。これは dlopen シンボル検出および mprotect PROT_EXEC 検出と同じ論理に基づく：「静的解析で把握した内容を実行時に無効化できる能力を持つ」バイナリは high リスクとして扱う。

```mermaid
flowchart LR
    classDef factor fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00
    classDef result fill:#f5f5f5,stroke:#666,stroke-width:1px,color:#333

    A["dlopen/dlsym シンボル"] -->|"OR"| D["isHighRisk = true"]
    B["mprotect PROT_EXEC"] -->|"OR"| D
    C["exec syscall (execve/execveat)"] -->|"OR"| D

    class A,B,C factor
    class D result
```

### 5.3 スキーマバージョンの変更なし

exec syscall の検出は既存の `SyscallAnalysisData` が保持する `DetectedSyscalls` リストを参照するのみで、保存形式の変更はない。`CurrentSchemaVersion` は変更不要である。

## 6. テスト戦略

### 6.1 テスト階層

```mermaid
flowchart TB
    classDef tier1 fill:#c3f08a,stroke:#333,color:#000
    classDef tier2 fill:#ffd59a,stroke:#333,color:#000
    classDef tier3 fill:#ffb86b,stroke:#333,color:#000

    Tier1["単体テスト<br>syscall テーブルの IsExecSyscall"]:::tier1
    Tier2["コンポーネントテスト<br>syscallAnalysisHasExecSignal<br>checkSyscallCache の組み合わせ"]:::tier2
    Tier3["統合テスト<br>既存テスト全通過"]:::tier3

    Tier1 --> Tier2 --> Tier3
```

### 6.2 テストスコープ

- **単体テスト**: 各 `IsExecSyscall` メソッドが正しい syscall に true/false を返すことの検証
- **コンポーネントテスト**: `syscallAnalysisHasExecSignal` と `checkSyscallCache` に対して、exec only / exec + network / no exec の各パターンをテスト
- **統合テスト**: `make test` で全既存テストがパスすることの確認

### 6.3 既存テストとの干渉

`checkSyscallCache` の network signal early return を修正することで、既存の network-only テストケースが影響を受けないことを確認すること。
