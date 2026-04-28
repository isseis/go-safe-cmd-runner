# 要件定義書: record コマンドへのデバッグ情報出力フラグ追加

## 1. プロジェクト概要

### 1.1 目的

`record` コマンドに `--debug-info` フラグを追加し、セキュリティ判断に不要なデバッグ情報（`Occurrences`、`DeterminationStats`）を JSON 出力から分離する。デフォルトではデバッグ情報を出力しない。フラグを指定した場合のみ出力する。

### 1.2 背景

`SyscallInfo.Occurrences` および `SyscallAnalysisResultCore.DeterminationStats` は解析アルゴリズムのデバッグに有用な情報（命令アドレス、決定手法の統計など）を含む。しかしセキュリティ判断（`runner` / `verify` による検証）には不要であり、JSON ファイルサイズを増加させるだけとなっている。

現状では両フィールドは `omitempty` タグを持つため、解析結果に値が存在する場合は常に出力される。デバッグ目的でのみ使いたいユーザーが出力を制御する手段がない。

`DeterminationStats` はすでにポインタ型（`*SyscallDeterminationStats`）でオプショナルな扱いになっているが、実際の出力制御はできない。`Occurrences` と合わせて同一フラグで制御することで UI の一貫性を保つ。

### 1.3 スコープ

**対象範囲:**
- `record` コマンドへの `--debug-info` フラグ追加
- `Occurrences` と `DeterminationStats` の保存時における出力制御

**対象外:**
- `runner` / `verify` コマンドの変更
- `record` の内部解析ロジックの変更（`Occurrences` は解析中も引き続き使用する）
- 既存 JSON ファイルのマイグレーション
- `AnalysisWarnings`、`ArgEvalResults` など他のフィールドの出力制御

## 2. 現状分析

### 2.1 対象フィールド

#### `SyscallInfo.Occurrences`

[internal/common/syscall_types.go:80](../../../internal/common/syscall_types.go#L80)

```go
Occurrences []SyscallOccurrence `json:"occurrences,omitempty"`
```

各 `SyscallOccurrence` には以下が含まれる:
- `Location`: syscall 命令の仮想アドレス
- `DeterminationMethod`: syscall 番号の決定手法
- `DeterminationDetail`: 決定手法の補足情報
- `Source`: 検出方法（直接命令 vs. libc シンボル経由）

`runner` / `verify` はこのフィールドを参照しない。

#### `SyscallAnalysisResultCore.DeterminationStats`

[internal/common/syscall_types.go:112](../../../internal/common/syscall_types.go#L112)

```go
DeterminationStats *SyscallDeterminationStats `json:"determination_stats,omitempty"`
```

syscall 番号解決の各パスの件数カウンタ。`runner` / `verify` はこのフィールドを参照しない。

### 2.2 `Occurrences` の内部利用

`Occurrences` は解析中に広く利用される（アドレス参照、決定手法の判定など）。**解析後・保存前**に除去する必要があり、解析ロジックは変更しない。

主な内部利用箇所:
- [internal/runner/security/elfanalyzer/syscall_analyzer.go:489](../../../internal/runner/security/elfanalyzer/syscall_analyzer.go#L489) — mprotect 解析での `Occurrences[0].DeterminationMethod` 判定
- [internal/runner/security/elfanalyzer/syscall_analyzer.go:510](../../../internal/runner/security/elfanalyzer/syscall_analyzer.go#L510) — mprotect 解析での `Occurrences[0].Location` 参照
- [internal/runner/security/elfanalyzer/syscall_analyzer.go:320](../../../internal/runner/security/elfanalyzer/syscall_analyzer.go#L320) — `DeterminationStats` 集計
- [internal/runner/security/network_analyzer.go:289](../../../internal/runner/security/network_analyzer.go#L289) — `Occurrences` のループ処理
- [internal/runner/security/machoanalyzer/svc_scanner.go:162](../../../internal/runner/security/machoanalyzer/svc_scanner.go#L162) — `Occurrences[0].DeterminationMethod` / `.Source` の書き換え
- [internal/libccache/analyzer.go:66](../../../internal/libccache/analyzer.go#L66) — `Occurrences[0].DeterminationMethod` の判定

## 3. 要件定義

### 3.1 機能要件

#### FR-1: `--debug-info` フラグの追加

`record` コマンドに `--debug-info` フラグを追加する。

**デフォルト（フラグなし）:**
- JSON 出力から `Occurrences` を除去する（各 `SyscallInfo` の `Occurrences` フィールドを nil にして保存）
- JSON 出力から `DeterminationStats` を除去する（`DeterminationStats` フィールドを nil にして保存）

**`--debug-info` 指定時:**
- `Occurrences` を JSON に含める
- `DeterminationStats` を JSON に含める

**優先度**: 必須

#### FR-2: 解析ロジックへの非干渉

`Occurrences` と `DeterminationStats` の除去は**保存時**に行う。解析処理中（elfanalyzer、machoanalyzer、libccache など）はこれらのフィールドを引き続き利用可能とする。

**優先度**: 必須

#### FR-3: `runner` / `verify` への非影響

`runner` と `verify` はこれらのフィールドを参照しないため、変更を加えない。デバッグ情報なしで保存されたレコードの検証動作は変わらない。

**優先度**: 必須

### 3.2 非機能要件

#### NFR-1: 既存 JSON の後方互換性

デバッグ情報を含む既存の JSON ファイルは引き続き有効であること。`--debug-info` なしで再 record した場合はデバッグ情報が除去されるが、これは意図した動作とする。

**優先度**: 必須

#### NFR-2: テスト容易性

フラグの有無による JSON 出力の差異をユニットテストで検証可能であること。

**優先度**: 必須

## 4. ユーザーインターフェース

### 4.1 コマンドラインフラグ

```
--debug-info    Include debug information (Occurrences, DeterminationStats) in output (default: false)
```

### 4.2 使用例

```sh
# デフォルト: デバッグ情報なしで記録
record --hash-dir /path/to/hashdir /usr/bin/myapp

# デバッグ情報を含めて記録
record --debug-info --hash-dir /path/to/hashdir /usr/bin/myapp
```

## 5. 制約条件

### 5.1 技術的制約

- Go 1.23.10 を使用
- 既存の `encoding/json` パッケージを使用
- `Occurrences` の除去は、既存の直接書き込みパス（`filevalidator` 内の `buildSyscallData` / `buildMachoSyscallData`）と libc キャッシュ経由のパス（`SaveSyscallAnalysis`）の両方に適用する

### 5.2 設計上の制約

- フラグは `recordConfig` に追加し、`filevalidator.Validator` にセッター（`SetIncludeDebugInfo(bool)` 相当）を通じて伝搬する既存パターン（`SetSyscallAnalyzer` 等）に従う
- `Occurrences` の除去は保存直前に行い、解析中は除去しない

## 6. 成功基準

### 6.1 機能的成功基準

- [ ] `--debug-info` フラグなしで記録した JSON に `occurrences` フィールドが含まれない
- [ ] `--debug-info` フラグなしで記録した JSON に `determination_stats` フィールドが含まれない
- [ ] `--debug-info` フラグありで記録した JSON に `occurrences` フィールドが含まれる（解析で検出された場合）
- [ ] `--debug-info` フラグありで記録した JSON に `determination_stats` フィールドが含まれる（解析で検出された場合）
- [ ] ELF バイナリ（Linux x86_64、arm64）の両パスでデバッグ情報の除去が機能する
- [ ] Mach-O バイナリ（macOS arm64）のパスでデバッグ情報の除去が機能する
- [ ] libc キャッシュ経由の保存パスでデバッグ情報の除去が機能する
- [ ] `--debug-info` フラグなしで記録したファイルを `verify` で検証できる（動作変化なし）
- [ ] 既存テストがすべて成功する（回帰なし）

### 6.2 品質基準

- [ ] `--debug-info` フラグの有無による JSON 出力差異のユニットテストが実装される
- [ ] `make test` がすべて通過する
- [ ] `make lint` がエラーなしで通過する

## 7. リスクと課題

### 7.1 複数の保存パス

**リスク**: `Occurrences` の書き込みは `filevalidator.Validator` 内の直接パスと、`libccache` 経由の `SaveSyscallAnalysis` パスの 2 系統ある。フラグを片方にしか伝搬しないと出力が不一致になる。

**対策**: フラグの伝搬経路を設計段階で両パス分明確にし、テストでそれぞれ検証する。

### 7.2 フラグを省略した再 record 時の `Occurrences` 消失

**リスク**: デバッグ情報ありで一度 record し、後でフラグなしで再 record すると `Occurrences` が消える。これは意図した動作だが、ユーザーが混乱する可能性がある。

**対策**: 要件として明示的に文書化する（本要件書 NFR-1）。ユーザーマニュアルで言及する。

## 8. 次のステップ

1. アーキテクチャ設計書の作成（02_architecture.ja.md）
2. 詳細仕様書の作成（03_detailed_specification.ja.md）
3. 実装計画書の作成（04_implementation_plan.ja.md）
4. 実装とテスト
