# ネットワークシンボル解析結果のキャッシュ 要件定義書

## 1. 概要

### 1.1 背景

タスク 0069 では ELF バイナリの `.dynsym` セクションを解析し、ネットワーク関連シンボル（`socket`, `connect` 等）の有無を検出する機能を実装した。この解析は現在 `runner` 実行のたびに行われている。

一方、タスク 0070/0072 では静的 ELF バイナリのシステムコール解析結果（`SyscallAnalysis`）を `record` 時に `fileanalysis.Record` へ保存し、`runner` 実行時はストアから読み込む設計が確立されている。

ネットワークシンボル解析にも同様のキャッシュパターンを適用できる：

- **`record` 時**: `.dynsym` を解析してネットワークシンボルの有無と検出シンボルのリストを記録する
- **`runner` 実行時**: 記録済み結果を読み込むだけでよく、`.dynsym` の再解析は不要

ハッシュ検証がネットワークシンボル解析より先に完了し、「バイナリが改ざんされていない」ことが保証された状態であるため、`record` 時の解析結果は信頼できる。

### 1.2 目的

- `runner` 実行時の ELF バイナリ再解析を廃止し、`record` 時の解析結果を再利用する
- `schema.go` のコメント「runner re-runs live binary analysis at runtime and does NOT read this field directly」を解消し、`HasDynamicLoad` の記録を実際に活用する
- ネットワークシンボル検出結果（`HasNetworkSymbols` および `DetectedSymbols`）も記録し、runner 時のログ出力（どのシンボルが検出されたか）を `record` 済み情報から提供する

### 1.3 スコープ

- **対象**: 動的リンクされた ELF バイナリ（`.dynsym` を持つもの）
- **対象外**: 静的 ELF バイナリ（`SyscallAnalysis` ベースの既存フローを維持）
- **対象外**: macOS Mach-O バイナリ（別途検討）。`binaryanalyzer.AnalysisOutput.DynamicLoadSymbols` フィールド追加に伴うビルド維持のための最小限の変更は許容するが、収集ロジックの実装は含まない
- **対象外**: スクリプトファイル
- **対象外**: `commandProfileDefinitions` に登録済みのコマンド（ハードコードリストが優先）

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| `.dynsym` | ELF バイナリの動的シンボルテーブル。動的リンク時に参照される外部シンボルのリストを含む |
| ネットワークシンボル | `socket`, `connect`, `bind` 等のネットワーク操作に関連するシンボル名。`binaryanalyzer.GetNetworkSymbols()` で定義 |
| `dynamic_load` シンボル | `dlopen`, `dlsym`, `dlvsym` 等の実行時ライブラリロードに関連するシンボル。`HasDynamicLoad` として独立して扱う |
| ネットワークシンボル解析 | `.dynsym` のシンボルをネットワークシンボルリストと照合し、ネットワーク関連シンボルの有無を判定する処理 |
| キャッシュ | `record` 時に計算したネットワークシンボル解析結果を `fileanalysis.Record` に保存し、`runner` 実行時に再利用すること |

## 3. 機能要件

### 3.1 `fileanalysis.Record` へのフィールド追加

#### FR-3.1.1: ネットワークシンボル解析結果の保存

`fileanalysis.Record` に `NetworkSymbolAnalysis *NetworkSymbolAnalysisData` フィールドを追加する。`NetworkSymbolAnalysisData` は解析日時・ネットワークシンボルの有無・検出シンボルリスト・dynamic_load シンボルリストを保持する。型定義の詳細は詳細仕様書（[03_detailed_specification.md](03_detailed_specification.md)）を参照。

`HasDynamicLoad bool` は `len(DynamicLoadSymbols) > 0` で導出できるため、独立したフィールドとして持たない。

**注意**: 既存の `HasDynamicLoad bool` フィールド（直接 `Record` に存在するもの）は削除し、`NetworkSymbolAnalysis.DynamicLoadSymbols` に統合する（スキーマ整理）。

#### FR-3.1.2: スキーマバージョンの更新

`fileanalysis.CurrentSchemaVersion` を 3 に更新する（2 → 3: `NetworkSymbolAnalysis` 追加、`HasDynamicLoad` 移動）。

後方互換性は維持しない。`record` の再実行が必須。

### 3.2 `binaryanalyzer.AnalysisOutput` の拡張

#### FR-3.2.0: `DynamicLoadSymbols` フィールドの追加

`binaryanalyzer.AnalysisOutput` に `DynamicLoadSymbols []DetectedSymbol` フィールドを追加し、`HasDynamicLoad bool` を削除する。スキーマバージョンを上げるため後方互換性は不要。呼び出し元は `len(DynamicLoadSymbols) > 0` で代替する。

**内部実装への波及（実装上の必須変更）:**

現在の `checkDynamicSymbols()` はシンボル名を保持しないため、`elfanalyzer/standard_analyzer.go` で dynamic_load シンボルの収集ロジックを追加する必要がある。`machoanalyzer/standard_analyzer.go` はビルドが通る最小限の変更のみ（収集ロジックの実装は対象外）。実装詳細は詳細仕様書（[03_detailed_specification.md](03_detailed_specification.md)）を参照。

### 3.3 （削除済み）

### 3.4 `record` コマンドの拡張

#### FR-3.4.1: ネットワークシンボル解析の実行と記録

`filevalidator.Validator.Record()` の処理中（`updateAnalysisRecord` 内）で、`BinaryAnalyzer` が設定されている場合に `AnalyzeNetworkSymbols` を呼び出し、`NetworkSymbolAnalysis` を記録する。

- バイナリが非 ELF の場合（`NotSupportedBinary`）は `NetworkSymbolAnalysis` を記録しない
- 静的 ELF バイナリ（`StaticBinary`）の場合は `NetworkSymbolAnalysis` を記録しない（`SyscallAnalysis` ベースのフローを維持）
- 解析エラー（`AnalysisError`）の場合は `record` をエラーで終了し、記録を行わない
- 既存の `HasDynamicLoad bool` フィールドへの書き込みを廃止し、`NetworkSymbolAnalysis.DynamicLoadSymbols` に移行する

#### FR-3.4.2: `--force` フラグとの整合性

`record --force` 実行時は `NetworkSymbolAnalysis` も新しい値で上書きする。

### 3.5 `runner` 実行時のネットワーク判定変更

#### FR-3.5.1: 動的バイナリへのキャッシュ利用

`isNetworkViaBinaryAnalysis` 関数において、対象コマンドが動的 ELF バイナリで `NetworkSymbolAnalysis` が記録済みの場合：

1. `fileanalysis.Store` から `NetworkSymbolAnalysis` を読み込む
2. 読み込んだ結果を `AnalysisOutput` に変換して返す
3. **`.dynsym` の再解析は行わない**

キャッシュが利用できない場合（`NetworkSymbolAnalysis` が `nil`、`AnalysisError` 等）は現行の実行時解析にフォールバックする。スキーマバージョン不一致の場合は `SchemaVersionMismatchError` で実行を拒否し、フォールバックしない。

> **実現層の注記**: `SchemaVersionMismatchError` によるブロックは `isNetworkViaBinaryAnalysis` ではなく `VerifyGroupFiles`（`verifyFileWithHash` → `store.Load`）で実現される。スキーマ不一致のレコードはファイルハッシュ検証の時点で実行前にエラーとなるため、`isNetworkViaBinaryAnalysis` まで到達しない。`isNetworkViaBinaryAnalysis` での追加ブロックは不要。

#### FR-3.5.2: 静的バイナリの既存フロー維持

静的 ELF バイナリは `SyscallAnalysis` ベースの既存フローを維持する。変更なし。

#### FR-3.5.3: ログ出力の維持

`NetworkDetected` 判定時の `slog.Info` ログを維持する。キャッシュ利用時も検出シンボル（`DetectedSymbols`）を `slog.Info` に出力する。

## 4. 非機能要件

### 4.1 パフォーマンス

#### NFR-4.1.1: `runner` 実行時のオーバーヘッド削減

現行実装では毎回 ELF ファイルの `.dynsym` をパースしている。キャッシュ利用後はファイルシステムからの JSON 読み込みのみとなり、バイナリ解析のオーバーヘッドを削減する。

### 4.2 セキュリティ

#### NFR-4.2.1: ハッシュ検証との順序保証

`runner` 実行時、`NetworkSymbolAnalysis` の読み込みは必ずハッシュ検証（`VerifyGroupFiles`）の完了後に行う。これにより、「バイナリが改ざんされていない」ことを確認した上で `record` 時の解析結果を信頼する。

#### NFR-4.2.2: フォールバック時のセキュリティ維持

キャッシュが利用できない場合のフォールバック（実行時解析）は現行と同等のセキュリティレベルを維持する。

### 4.3 後方互換性

本タスクでは後方互換性を維持しない。`schema_version: 2` 以前の記録ファイルは `SchemaVersionMismatchError` で拒否される。`record` の再実行が必須。

### 4.4 保守性

#### NFR-4.4.1: `SyscallAnalysis` との対称性

`SyscallAnalysis` の保存・読み込みパターン（`fileanalysis.Store` を介した `record` 時保存 / `runner` 時読み込み）と同じ構造を `NetworkSymbolAnalysis` に適用する。実装の一貫性を保つ。

**`NetworkSymbolStore` に Save メソッドを持たない設計意図**:
`SyscallAnalysis` の保存は `filevalidator` の外側（`cmd/record/main.go`）から `SyscallAnalysisStore.SaveSyscallAnalysis()` を呼ぶアーキテクチャのため、Store に Save が必要だった。一方 `NetworkSymbolAnalysis` の保存は既存の `filevalidator.updateAnalysisRecord()` 内で `BinaryAnalyzer` を呼ぶフローの延長として完結し、`filevalidator` 外から Save を呼ぶ経路がない。このため `NetworkSymbolStore` は Load 専用インターフェースとし、Save は `filevalidator` パッケージ内に閉じる。

## 5. 受け入れ条件

### AC-1: `fileanalysis.Record` フィールド追加

- [x] `NetworkSymbolAnalysisData` 型が定義されていること
- [x] `fileanalysis.Record` に `NetworkSymbolAnalysis *NetworkSymbolAnalysisData` フィールドが追加されていること
- [x] 既存の `HasDynamicLoad bool` フィールドが `Record` から削除され、`NetworkSymbolAnalysis.DynamicLoadSymbols` に統合されていること
- [x] `CurrentSchemaVersion` が 3 に更新されていること

### AC-2: `record` コマンドの拡張

- [x] 動的 ELF バイナリで `NetworkSymbolAnalysis` が記録されること
- [x] `NetworkSymbolAnalysis.HasNetworkSymbols` が正しく設定されること
- [x] `NetworkSymbolAnalysis.DetectedSymbols` にネットワーク関連シンボルが列挙されること
- [x] `NetworkSymbolAnalysis.DynamicLoadSymbols` に検出された `dlopen`/`dlsym`/`dlvsym` が列挙されること
- [x] `NetworkSymbolAnalysis.AnalyzedAt` が記録されること
- [x] 非 ELF ファイルでは `NetworkSymbolAnalysis` が記録されないこと
- [x] 静的 ELF バイナリでは `NetworkSymbolAnalysis` が記録されないこと
- [x] `AnalysisError` 時に `record` がエラーで終了し記録が保存されないこと
- [x] `record --force` で `NetworkSymbolAnalysis` が更新されること

### AC-3: `runner` 実行時のキャッシュ利用

- [x] `NetworkSymbolAnalysis` が記録済みの動的 ELF バイナリで、`runner` 実行時に `.dynsym` の再解析が行われないこと
- [x] キャッシュ利用時に `NetworkDetected` が正しく判定されること（`HasNetworkSymbols: true` → `NetworkDetected`）
- [x] キャッシュ利用時に `isHighRisk`（`HasDynamicLoad` 相当）が `DynamicLoadSymbols` から正しく導出されること
- [x] `NetworkSymbolAnalysis` が未記録の場合に実行時解析にフォールバックすること
- [x] `slog.Debug` ログにキャッシュ利用時も `DetectedSymbols` が出力されること

### AC-4: スキーマ移行

- [x] `schema_version: 2` 以前の記録ファイルで `runner` 実行時に `VerifyGroupFiles` が group verification failed（`ErrGroupVerificationFailed` を内包する `verification.Error`）を返し、実行前に停止すること
- [x] 既存のテストがすべてパスすること

### AC-5: 既存機能への非影響

- [x] `commandProfileDefinitions` 登録済みコマンドの判定ロジックが変更されないこと
- [x] 静的 ELF バイナリの `SyscallAnalysis` ベースのフローが維持されること
- [x] `DynLibDeps` 検証が引き続き動作すること

## 6. テスト方針

### 6.1 `record` 拡張のユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| ネットワークシンボルありの動的 ELF | `HasNetworkSymbols: true`、`DetectedSymbols` に検出シンボルが含まれること |
| ネットワークシンボルなしの動的 ELF | `HasNetworkSymbols: false`、`DetectedSymbols` が空であること |
| `dlopen` のみを持つ動的 ELF | `DynamicLoadSymbols` に `dlopen` が含まれ、`HasNetworkSymbols: false` となること |
| 非 ELF ファイル | `NetworkSymbolAnalysis` が `nil` であること |
| 静的 ELF バイナリ | `NetworkSymbolAnalysis` が `nil` であること |
| `AnalysisError` | `record` がエラーで終了し記録が保存されないこと |

### 6.2 アナライザーレベルのユニットテスト（`elfanalyzer`）

FR-3.2.0 の内部実装変更（`checkDynamicSymbols()` でのシンボル名収集）を検証する。`machoanalyzer` は本タスクのスコープ外のためテスト追加不要。

**対象ファイル:** `elfanalyzer/analyzer_test.go`（既存の `HasDynamicLoad` テストの置換と新規追加）

| テストケース | 検証内容 |
|-------------|---------|
| `dlopen` のみを持つ ELF | `AnalysisOutput.DynamicLoadSymbols` に `{Name:"dlopen", Category:"dynamic_load"}` が含まれること |
| `dlsym` と `dlvsym` を両方持つ ELF | `DynamicLoadSymbols` に両シンボルが列挙されること |
| ネットワークシンボルと `dlopen` を同時に持つ ELF | `DetectedSymbols` と `DynamicLoadSymbols` が独立して正しく設定されること |
| dynamic_load シンボルを持たない ELF | `DynamicLoadSymbols` が空スライス（または `nil`）であること |

### 6.3 `runner` キャッシュ利用のユニットテスト

| テストケース | 検証内容 |
|-------------|---------|
| キャッシュあり・ネットワークシンボルあり | `NetworkDetected` が返され、`.dynsym` 再解析なし |
| キャッシュあり・ネットワークシンボルなし | `NoNetworkSymbols` が返され、`.dynsym` 再解析なし |
| キャッシュなし（未記録） | 実行時解析にフォールバックすること |
| キャッシュあり・`DynamicLoadSymbols` に `dlopen` を含む | `isHighRisk: true` が返されること |

### 6.4 統合テスト

| テストケース | 検証内容 |
|-------------|---------|
| `record` → `runner` の正常フロー | キャッシュを利用して正しく判定されること |
| 旧スキーマ（`schema_version: 2`）の記録で実行 | `SchemaVersionMismatchError` でブロックされること |

## 7. 先行タスクとの関係

| 項目 | タスク 0069 | タスク 0070/0072 | タスク 0074 | 本タスク（0076）|
|------|------------|-----------------|------------|----------------|
| 解析手法 | `.dynsym` シンボル解析 | 機械語 syscall 解析 | `DT_NEEDED` 解決 | `.dynsym` 解析結果のキャッシュ |
| 実行タイミング | 実行時（毎回） | `record` 時保存・`runner` 時読み込み | `record` 時保存・`runner` 時検証 | `record` 時保存・`runner` 時読み込み |
| キャッシュ先 | なし | `SyscallAnalysis` フィールド | `DynLibDeps` フィールド | `NetworkSymbolAnalysis` フィールド |
| 目的 | ネットワーク使用検出 | 静的バイナリの syscall 検出 | 依存ライブラリ整合性保証 | `runner` 時の再解析廃止・ログ改善 |
