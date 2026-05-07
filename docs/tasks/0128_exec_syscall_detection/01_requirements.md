# 要件定義書: exec syscall による高リスク検出

## 1. 概要

### 1.1 背景

現在のリスク評価システムは、以下のパターンを検出した実行ファイルをリスクありとして識別する。

| 検出パターン | リスクレベル |
|---|---|
| network 関連シンボル（socket, connect 等）| medium |
| 動的ロードシンボル（dlopen/dlsym/dlvsym）| high |
| syscall 解析による network syscall | medium |
| syscall 解析による svc #0x80（unresolved） | high |

一方、`execve` / `execveat` 等の exec 系 syscall は現在検出対象外である。

exec 系 syscall を呼び出すバイナリは、任意の別バイナリにプロセスを置き換えることができる。この性質は `dlopen` と同様に「静的解析で把握した内容を実行時に無効化できる」能力であり、リスク判定の観点では high リスクに相当する。さらに、`execl`, `execvp`, `system`, `popen`, `posix_spawn` 等の多様な API はすべて最終的に `execve` / `execveat` syscall に収束するため、syscall レベルで検出することで一網打尽にできる。

### 1.2 目的

syscall 静的解析（タスク 0070/0072/0097）の枠組みを利用して、exec 系 syscall（`execve`, `execveat`）を検出し、当該バイナリを high リスクとして識別する。

### 1.3 スコープ

**対象:**
- ELF バイナリ（x86_64 Linux、arm64 Linux）の syscall 解析結果に含まれる exec syscall の検出
- Mach-O バイナリ（macOS arm64）の syscall 解析結果に含まれる exec syscall の検出
- 主バイナリの syscall 解析結果を通じた検出
- dynlib 依存ライブラリの syscall 解析結果を通じた検出
- shebang インタープリタバイナリの syscall 解析結果を通じた検出（`followShebangChain` の再帰処理により自動的にカバーされる）
- shebang インタープリタが依存する dynlib ライブラリの検出（同上）

**対象外:**
- 静的解析結果に exec syscall が記録されていない場合の実行時動的検出
- exec 呼び出し先バイナリの解析（exec チェーンのトレース）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| exec syscall | プロセスイメージを別の実行ファイルに置き換えるシステムコール。Linux では `execve` (sys 59/221) と `execveat` (sys 322/281)、macOS では `execve` (sys 59) と `__mac_execve` (sys 380) |
| IsExec | syscall がexec 系かどうかを示すフラグ。`SyscallDefinition` 構造体と `macOSSyscallEntry` 構造体に追加するフィールド |
| syscall テーブル | アーキテクチャ別の syscall 番号・名称・分類を管理するデータ構造 |
| high リスク | `RiskLevelHigh` に相当するリスクレベル。`IsNetworkOperation` の返り値 `isHighRisk = true` に対応する |
| exec signal | syscall 解析結果に exec syscall が含まれることを示すシグナル |

## 3. 機能要件

### 3.1 exec syscall の定義

#### FR-3.1.1: exec syscall のデータ定義

`elfanalyzer.SyscallDefinition` 構造体に `IsExec bool` フィールドを追加すること。

#### FR-3.1.2: 対象 syscall 番号

以下の syscall を exec syscall として定義すること。

**Linux x86_64:**

| syscall | 番号 |
|---------|------|
| `execve` | 59 |
| `execveat` | 322 |

**Linux arm64:**

| syscall | 番号 |
|---------|------|
| `execve` | 221 |
| `execveat` | 281 |

**macOS arm64:**

| syscall | 番号（BSD class prefix 0x2000000 を除く） |
|---------|------|
| `execve` | 59 |
| `__mac_execve` | 380 |

#### FR-3.1.3: SyscallNumberTable インターフェースの拡張

`elfanalyzer.SyscallNumberTable` インターフェースに `IsExecSyscall(number int) bool` メソッドを追加すること。`X86_64SyscallTable` および `ARM64LinuxSyscallTable` はこのメソッドを実装すること。

#### FR-3.1.4: MacOSSyscallTable の拡張

`libccache.MacOSSyscallTable` に `IsExecSyscall(number int) bool` メソッドを追加すること。

### 3.2 exec syscall の検出

#### FR-3.2.1: syscallTableInterface の拡張

`network_analyzer.go` 内のパッケージプライベートインターフェース `syscallTableInterface` に `IsExecSyscall(number int) bool` を追加すること。

#### FR-3.2.2: exec signal 検出関数

`network_analyzer.go` に `syscallAnalysisHasExecSignal` 関数を実装すること。`fileanalysis.SyscallAnalysisResult` を受け取り、exec syscall が含まれる場合に `true` を返すこと。

#### FR-3.2.3: checkSyscallCache への統合（主バイナリ）

`NetworkAnalyzer.checkSyscallCache` において exec signal を検出し、exec syscall が含まれる場合に `isHighRisk = true` を返すこと。

network signal と exec signal が同時に存在する場合は、両方のシグナルを組み合わせること（`isNetwork = true` かつ `isHighRisk = true`）。

#### FR-3.2.4: dynlib 依存ライブラリへの exec signal 伝播

`network_analyzer.go` に `firstExecSyscall` 関数を追加すること（既存の `firstNetworkSyscall` と同じパターン）。

`depSignals` 構造体に `execSyscall string` フィールドを追加し、`analyzeDepSignals` において `firstExecSyscall` を呼び出すこと。

`checkDynLibDepsNetwork` において `sigs.execSyscall != ""` の場合に `isHighRisk = true` を返すこと。

#### FR-3.2.5: shebang バイナリおよびその dynlib 依存ライブラリ

shebang インタープリタの exec 検出は `followShebangChain` → `analyzeBinarySignals(interpPath, interpHash)` の再帰呼び出しを通じて自動的にカバーされる。インタープリタの dynlib 依存ライブラリの exec 検出は、同再帰呼び出し内の `checkDynLibDepsNetwork` を通じてカバーされる。FR-3.2.3 および FR-3.2.4 の実装が完了すれば、追加実装は不要である。

### 3.3 リスクマッピング

#### FR-3.3.1: exec signal → high リスク

exec signal が検出された場合、`IsNetworkOperation` の戻り値において `isHighRisk = true` となること。これにより呼び出し元の `EvaluateRisk` は `RiskLevelHigh` を返す。

#### FR-3.3.2: dlopen との等価扱い

exec signal による high リスク判定は、dlopen シンボル検出による high リスク判定と等価に扱うこと（同じ `isHighRisk = true` パス）。

### 3.4 生成スクリプトの更新

#### FR-3.4.1: generate_syscall_table.py の更新

`scripts/generate_syscall_table.py` を以下の観点で更新すること。

- Linux 用: `EXEC_SYSCALL_NAMES` セット（`execve`, `execveat`）を追加し、`SyscallDefinition` の `IsExec` フィールドを生成すること
- macOS 用: `MACOS_EXEC_SYSCALL_NAMES` セット（`execve`, `__mac_execve`）を追加し、`macOSSyscallEntry` の `isExec` フィールドを生成すること

生成コードは既存の `IsNetwork` フィールドの処理パターンに従うこと。

## 4. 非機能要件

### 4.1 パフォーマンス

#### NFR-4.1.1: 実行時オーバーヘッド

exec signal の検出は、既存の network signal 検出と同一のパス（`checkSyscallCache` 内の syscall リスト線形スキャン）で実行すること。追加の I/O やキャッシュ操作は不要であること。

### 4.2 保守性

#### NFR-4.2.1: アーキテクチャ対称性

exec syscall の定義は `NETWORK_SYSCALL_NAMES` と同じ設計パターン（Python のセット定数）で管理し、新アーキテクチャ追加時の拡張が容易な構造とすること。

#### NFR-4.2.2: コード再利用

exec signal 検出ロジックは、既存の `syscallAnalysisHasNetworkSignal` と同一の実装パターンに従い重複を排除すること。

#### NFR-4.2.3: コード言語

実装コード（Go および Python）中に日本語文字列を含めないこと。

### 4.3 テスト可能性

#### NFR-4.3.1: ユニットテスト

各アーキテクチャの syscall テーブルに対して、`IsExecSyscall` の正常動作をテストすること。

#### NFR-4.3.2: network_analyzer テスト

`checkSyscallCache` に対して、主バイナリの exec signal 検出とリスクマッピングをテストすること。

dynlib 依存ライブラリの exec signal 検出（`analyzeDepSignals` 経由）についてもテストすること。

## 5. 受け入れ条件

### AC-1: SyscallDefinition の拡張

- [ ] `elfanalyzer.SyscallDefinition` に `IsExec bool` フィールドが追加されていること
- [ ] `elfanalyzer.SyscallNumberTable` インターフェースに `IsExecSyscall(int) bool` が追加されていること
- [ ] `X86_64SyscallTable.IsExecSyscall` が execve(59) と execveat(322) に対して `true` を返すこと
  - `TestX86_64SyscallTable_IsExecSyscall`
- [ ] `ARM64LinuxSyscallTable.IsExecSyscall` が execve(221) と execveat(281) に対して `true` を返すこと
  - `TestARM64LinuxSyscallTable_IsExecSyscall`
- [ ] 上記以外の syscall 番号に対して `IsExecSyscall` が `false` を返すこと
  - `TestX86_64SyscallTable_IsExecSyscall`, `TestARM64LinuxSyscallTable_IsExecSyscall`

### AC-2: MacOSSyscallTable の拡張

- [ ] `libccache.MacOSSyscallTable` に `IsExecSyscall` メソッドが追加されていること
- [ ] `MacOSSyscallTable.IsExecSyscall` が execve(59) と \_\_mac\_execve(380) に対して `true` を返すこと
  - `TestMacOSSyscallTable_IsExecSyscall`
- [ ] 上記以外の syscall 番号に対して `IsExecSyscall` が `false` を返すこと
  - `TestMacOSSyscallTable_IsExecSyscall`

### AC-3: exec signal 検出

- [ ] exec syscall を含む `SyscallAnalysisResult` に対して `syscallAnalysisHasExecSignal` が `true` を返すこと
  - `TestSyscallAnalysisHasExecSignal`
- [ ] exec syscall を含まない `SyscallAnalysisResult` に対して `false` を返すこと
  - `TestSyscallAnalysisHasExecSignal`
- [ ] `SyscallAnalysisResult` が `nil` の場合に `false` を返すこと
  - `TestSyscallAnalysisHasExecSignal`

### AC-4: checkSyscallCache のリスクマッピング（主バイナリ）

以下はいずれも `TestNetworkAnalyzer_ExecSyscallIsHighRisk` のサブケースとして実装する。

- [ ] syscall 解析結果に exec syscall のみが含まれる場合、`IsNetworkOperation` が `(isNetwork=false, isHighRisk=true)` を返すこと
- [ ] syscall 解析結果に network syscall と exec syscall が両方含まれる場合、`(isNetwork=true, isHighRisk=true)` を返すこと
- [ ] syscall 解析結果に exec syscall が含まれない場合、exec 検出による `isHighRisk` への影響がないこと

### AC-7: dynlib 依存ライブラリの exec signal 伝播

- [ ] `firstExecSyscall` 関数が exec syscall を含む `SyscallAnalysisData` に対して syscall 名を返すこと
  - `TestFirstExecSyscall`
- [ ] dynlib 解析結果に exec syscall が含まれる場合、`analyzeDepSignals` の `execSyscall` フィールドが非空であること
  - `TestAnalyzeDepSignals_ExecSyscall`
- [ ] dynlib 依存ライブラリに exec syscall が含まれる場合、`IsNetworkOperation` が `(isHighRisk=true)` を返すこと
  - `TestNetworkAnalyzer_DynLibExecSyscallIsHighRisk`

### AC-5: 生成スクリプトの更新

- [ ] `generate_syscall_table.py` に `EXEC_SYSCALL_NAMES` セットが定義されていること
- [ ] 生成される Go コードに `IsExec` フィールドが含まれること
- [ ] `make generate-syscall-tables` を実行した場合に、既存の生成済みファイルと内容が一致すること

### AC-6: 既存機能への非影響

- [ ] 既存のテストがすべてパスすること（`make test`）
- [ ] `IsNetworkSyscall` の判定結果が変わらないこと
- [ ] `commandProfileDefinitions` に含まれるコマンドのリスク判定が変わらないこと

## 6. テスト方針

### 6.1 ユニットテスト

#### syscall テーブルのテスト

各アーキテクチャ別テストファイルに `IsExecSyscall` のテストを追加する。

`TestX86_64SyscallTable_IsExecSyscall`:
- execve(59) → true
- execveat(322) → true
- socket(41) → false（network syscall は exec ではない）
- read(0) → false
- 存在しない番号（-1, 999） → false

`TestARM64LinuxSyscallTable_IsExecSyscall`:
- execve(221) → true
- execveat(281) → true
- socket(198) → false
- 存在しない番号 → false

`TestMacOSSyscallTable_IsExecSyscall`:
- execve(59) → true
- \_\_mac\_execve(380) → true
- socket(97) → false
- 存在しない番号 → false

#### network_analyzer のテスト

`TestSyscallAnalysisHasExecSignal` を `network_analyzer_test.go` に追加する。
`TestNetworkAnalyzer_ExecSyscallIsHighRisk` および関連テストを追加する。

dynlib 依存ライブラリの exec 検出テストを追加する。

- `TestFirstExecSyscall`
- `TestAnalyzeDepSignals_ExecSyscall`
- `TestNetworkAnalyzer_DynLibExecSyscallIsHighRisk`

### 6.2 既存テストの維持

- `make test` で全テストがパスすること（AC-6）
- `make lint` でリンターが全てパスすること
