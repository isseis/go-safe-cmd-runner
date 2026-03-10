# 実装計画書: ネットワークシンボル解析結果のキャッシュ

## 概要

本ドキュメントは、ネットワークシンボル解析結果のキャッシュ実装進捗を追跡する。
詳細仕様は [03_detailed_specification.md](03_detailed_specification.md) を参照。
アーキテクチャ設計は [02_architecture.md](02_architecture.md) を参照。

## 依存関係

```mermaid
flowchart LR
    classDef todo fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00

    P1["Phase 1<br>型定義 &<br>スキーマ変更"]
    P2["Phase 2<br>アナライザー拡張<br>(DynamicLoadSymbols)"]
    P3["Phase 3<br>record 時の<br>キャッシュ保存"]
    P4["Phase 4<br>runner 時の<br>キャッシュ利用"]
    P5["Phase 5<br>統合テスト &<br>最終確認"]

    P1 --> P2
    P1 --> P3
    P2 --> P3
    P3 --> P4
    P4 --> P5

    class P1,P2,P3,P4,P5 todo
```

**注記**: Phase 1 完了後に Phase 2 と Phase 3 の型定義部分は並行して作業可能だが、
Phase 3 のテストは Phase 2 の `DynamicLoadSymbols` 収集ロジックに依存する。
Phase 4 は Phase 3 完了後に実施する。

## Phase 1: 型定義 & スキーマ変更

`fileanalysis` および `binaryanalyzer` パッケージの型定義を更新し、
全既存テストのビルドが通る状態を維持する。

仕様参照: 詳細仕様書 §1

### 1.1 `binaryanalyzer.AnalysisOutput` の変更

- [ ] `internal/runner/security/binaryanalyzer/analyzer.go` を更新
  - `HasDynamicLoad bool` フィールドを削除
  - `DynamicLoadSymbols []DetectedSymbol` フィールドを追加
  - 仕様: 詳細仕様書 §1.1

### 1.2 `HasDynamicLoad` 参照箇所の修正

- [ ] `HasDynamicLoad` を参照しているコード箇所を `len(DynamicLoadSymbols) > 0` に置換
  - `elfanalyzer/standard_analyzer.go`: `checkDynamicSymbols()` の返り値を修正
  - `machoanalyzer/standard_analyzer.go`: ビルド維持の最小限修正
  - `security/network_analyzer.go`: `output.HasDynamicLoad` の参照を修正
  - `filevalidator/validator.go`: `record.HasDynamicLoad` の参照を修正
  - テストファイルの `HasDynamicLoad` 参照を修正

### 1.3 `fileanalysis` スキーマ変更

- [ ] `internal/fileanalysis/schema.go` を更新
  - `NetworkSymbolAnalysisData` 構造体を追加
  - `DetectedSymbolEntry` 構造体を追加
  - `Record` から `HasDynamicLoad bool` フィールドを削除
  - `Record` に `NetworkSymbolAnalysis *NetworkSymbolAnalysisData` フィールドを追加
  - `CurrentSchemaVersion` を 2 → 3 に更新
  - 仕様: 詳細仕様書 §1.2

### 1.4 `fileanalysis` エラー変数の追加

- [ ] `internal/fileanalysis/errors.go` を更新
  - `ErrNoNetworkSymbolAnalysis` エラー変数を追加
  - 仕様: 詳細仕様書 §1.3

### 1.5 `fileanalysis.NetworkSymbolStore` の実装

- [ ] `internal/fileanalysis/network_symbol_store.go` を新規作成
  - `NetworkSymbolStore` インターフェースを定義
  - `networkSymbolStore` 非公開実装を定義
  - `NewNetworkSymbolStore` ファクトリ関数を定義
  - `syscall_store.go` と同じ adapter パターンを踏襲
  - 仕様: 詳細仕様書 §4

### 1.6 ビルド確認

- [ ] `make build` が成功すること
- [ ] `make test` が成功すること（既存テストの `HasDynamicLoad` 参照修正含む）

## Phase 2: アナライザー拡張（DynamicLoadSymbols 収集）

`elfanalyzer` の `checkDynamicSymbols()` で dynamic_load シンボル名を
収集し `AnalysisOutput.DynamicLoadSymbols` に設定する。

仕様参照: 詳細仕様書 §2

### 2.1 `elfanalyzer/standard_analyzer.go` の変更

- [ ] `checkDynamicSymbols()` を変更
  - `hasDynamicLoad = true` の代わりに `dynamicLoadSyms` スライスに
    `DetectedSymbol{Name, Category:"dynamic_load"}` を収集
  - `DynamicLoadSymbols` フィールドに設定して返す
  - 仕様: 詳細仕様書 §2.1

### 2.2 `elfanalyzer` 単体テストの追加

- [ ] `elfanalyzer/standard_analyzer_test.go` にテストを追加
  - `dlopen` のみを持つ ELF → `DynamicLoadSymbols: [{dlopen, dynamic_load}]`
  - `dlsym` と `dlvsym` を両方持つ ELF → 両シンボルが列挙
  - ネットワークシンボルと `dlopen` の同時検出 → 独立して設定
  - dynamic_load シンボルなし → `DynamicLoadSymbols: nil`
  - 受け入れ条件: AC-2

### 2.3 `machoanalyzer` のビルド維持修正

- [ ] `machoanalyzer/standard_analyzer.go` のビルドが通ることを確認
  - `DynamicLoadSymbols` フィールド追加に伴う最小限の修正のみ
  - 収集ロジックの実装は対象外
  - 仕様: 詳細仕様書 §2.2

### 2.4 テスト確認

- [ ] `make test` が成功すること
- [ ] `make lint` が成功すること

## Phase 3: `record` 時のキャッシュ保存

`filevalidator` の `saveHash` を拡張し、
`AnalyzeNetworkSymbols` の結果を `NetworkSymbolAnalysis` として記録する。

仕様参照: 詳細仕様書 §3

### 3.1 `convertDetectedSymbols` ヘルパー関数の追加

- [ ] `internal/filevalidator/validator.go` にヘルパー関数を追加
  - `binaryanalyzer.DetectedSymbol` → `fileanalysis.DetectedSymbolEntry` の変換
  - 空スライスの場合は `nil` を返す（`omitempty` との整合性）
  - 仕様: 詳細仕様書 §3.2

### 3.2 `saveHash` 関数の変更

- [ ] `internal/filevalidator/validator.go` の `saveHash` を変更
  - `AnalyzeNetworkSymbols` の `Result` で分岐
  - `NetworkDetected` / `NoNetworkSymbols` → `record.NetworkSymbolAnalysis` を設定
  - `StaticBinary` / `NotSupportedBinary` → 記録しない
  - `AnalysisError` → エラーを返す
  - 既存の `HasDynamicLoad` への書き込みを廃止
  - 仕様: 詳細仕様書 §3.1

### 3.3 `record` 拡張のユニットテスト

- [ ] `filevalidator/validator_test.go` にテストを追加
  - ネットワークシンボルありの動的 ELF → `HasNetworkSymbols: true`、
    `DetectedSymbols` に socket を含む
  - ネットワークシンボルなしの動的 ELF → `HasNetworkSymbols: false`、
    `DetectedSymbols` が空
  - `dlopen` のみを持つ動的 ELF → `DynamicLoadSymbols` に dlopen を含む
  - 非 ELF ファイル → `NetworkSymbolAnalysis` が `nil`
  - 静的 ELF バイナリ → `NetworkSymbolAnalysis` が `nil`
  - `AnalysisError` → `record` がエラーを返し記録が保存されない
  - `record --force` で既存の `NetworkSymbolAnalysis` が新しい値で上書きされること
  - 受け入れ条件: AC-2

### 3.4 テスト確認

- [ ] `make test` が成功すること
- [ ] `make lint` が成功すること

## Phase 4: `runner` 時のキャッシュ利用

`security.NetworkAnalyzer` に store を注入し、
`isNetworkViaBinaryAnalysis` でキャッシュを参照するロジックを追加する。

仕様参照: 詳細仕様書 §5

### 4.1 `NetworkAnalyzer` 構造体の拡張

- [ ] `internal/runner/security/network_analyzer.go` を変更
  - `store fileanalysis.NetworkSymbolStore` フィールドを追加
  - `NewNetworkAnalyzerWithStore(store)` コンストラクタを本番コードに追加
  - 仕様: 詳細仕様書 §5.1, §5.2

### 4.2 `convertNetworkSymbolEntries` ヘルパー関数の追加

- [ ] `internal/runner/security/network_analyzer.go` にヘルパーを追加
  - `fileanalysis.DetectedSymbolEntry` → `binaryanalyzer.DetectedSymbol`
    の逆変換
  - 仕様: 詳細仕様書 §5.4

### 4.3 `isNetworkViaBinaryAnalysis` の変更

- [ ] キャッシュ参照ロジックを先頭に追加
  - `store != nil` の場合のみキャッシュを参照
  - キャッシュヒット → `AnalysisOutput` を構築して `handleAnalysisOutput` に渡す
  - キャッシュミス（`ErrNoNetworkSymbolAnalysis`、`ErrHashMismatch`、`ErrRecordNotFound`）→ 従来の実行時解析にフォールバック
  - **`SchemaVersionMismatchError` はフォールバック禁止**。エラーをそのまま呼び出し元に返すこと（要件: AC-4）
  - 仕様: 詳細仕様書 §5.4

### 4.4 store 注入チェーンの実装

- [ ] `internal/runner/risk/evaluator.go` を変更
  - `NewStandardEvaluator()` に `store fileanalysis.NetworkSymbolStore` 引数を追加
  - `security.NewNetworkAnalyzerWithStore(store)` を呼び出す
  - 仕様: 詳細仕様書 §5.3
- [ ] `internal/runner/resource/normal_manager.go` を変更
  - `NewNormalResourceManagerWithOutput()` シグネチャに
    `store fileanalysis.NetworkSymbolStore` 引数を追加
  - `risk.NewStandardEvaluator(store)` に渡す
- [ ] `internal/runner/resource/default_manager.go` を変更
  - `NewDefaultResourceManager()` シグネチャに
    `store fileanalysis.NetworkSymbolStore` 引数を追加
  - `NewNormalResourceManagerWithOutput()` に渡す
- [ ] `internal/runner/runner.go` を変更
  - `createNormalResourceManager()` 内で `fileanalysis.Store` を生成し
    `fileanalysis.NewNetworkSymbolStore(store)` で変換して渡す

### 4.5 テスト用ヘルパーの更新

- [ ] `internal/runner/security/network_analyzer_test_helpers.go` を更新
  - `NewNetworkAnalyzerWithBinaryAnalyzerAndStore` ヘルパーの型を
    `fileanalysis.NetworkSymbolStore` に合わせる
  - 仕様: 詳細仕様書 §5.5

### 4.6 シグネチャ変更に伴うテストファイル修正

- [ ] `NewDefaultResourceManager()` / `NewNormalResourceManagerWithOutput()` /
  `NewStandardEvaluator()` のシグネチャ変更に伴い、呼び出し元テストファイルで
  `nil` を引数に追加

### 4.7 `runner` キャッシュ利用のユニットテスト

- [ ] `security/command_analysis_test.go` にテストを追加
  - キャッシュあり・`HasNetworkSymbols: true` → `NetworkDetected`、
    `BinaryAnalyzer` 未呼出
  - キャッシュあり・`HasNetworkSymbols: false` → `NoNetworkSymbols`、
    `BinaryAnalyzer` 未呼出
  - キャッシュなし（`ErrNoNetworkSymbolAnalysis`） →
    `BinaryAnalyzer.AnalyzeNetworkSymbols()` にフォールバック
  - キャッシュあり・`DynamicLoadSymbols` に `dlopen` を含む →
    `isHighRisk: true`
  - store が `SchemaVersionMismatchError` を返す → エラーを伝播し、フォールバック **しない**
  - 受け入れ条件: AC-3, AC-4

### 4.8 テスト確認

- [ ] `make test` が成功すること
- [ ] `make lint` が成功すること

## Phase 5: 統合テスト & 最終確認

全フェーズの変更を統合テストし、受け入れ条件を検証する。

### 5.1 統合テスト

- [ ] `record` → `runner` の正常フロー
  - キャッシュを利用して正しくネットワーク判定されること
  - 受け入れ条件: AC-3
- [ ] 旧スキーマ（`schema_version: 2`）の記録で実行
  - `SchemaVersionMismatchError` でブロックされること
  - 受け入れ条件: AC-4

### 5.2 既存機能への非影響確認

- [ ] `commandProfileDefinitions` 登録済みコマンドの判定が変更されないこと
  - 受け入れ条件: AC-5
- [ ] 静的 ELF バイナリの `SyscallAnalysis` ベースフローが維持されること
  - 受け入れ条件: AC-5
- [ ] `DynLibDeps` 検証が引き続き動作すること
  - 受け入れ条件: AC-5

### 5.3 全テスト・lint の最終確認

- [ ] `make test` が全テストパスすること
- [ ] `make lint` が警告・エラーなしであること
- [ ] `make fmt` で変更がないこと

## 受け入れ条件とテストの対応

| 受け入れ条件 | 要件 | テスト / 検証箇所 |
|---|---|---|
| AC-1: `fileanalysis.Record` フィールド追加 | FR-3.1.1, FR-3.1.2 | Phase 1（§1.3 型定義、§1.6 ビルド確認） |
| AC-2: `record` コマンドの拡張 | FR-3.2.0, FR-3.4.1, FR-3.4.2 | Phase 2（§2.2 アナライザーテスト）、Phase 3（§3.3 record テスト） |
| AC-3: `runner` 時のキャッシュ利用 | FR-3.5.1, FR-3.5.3 | Phase 4（§4.7 キャッシュ利用テスト）、Phase 5（§5.1 統合テスト） |
| AC-4: スキーマ移行 | FR-3.1.2 | Phase 5（§5.1 旧スキーマテスト） |
| AC-5: 既存機能への非影響 | FR-3.5.2 | Phase 5（§5.2 非影響確認） |
