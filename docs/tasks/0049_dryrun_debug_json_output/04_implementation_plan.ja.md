# 実装計画書: Dry-Run Debug情報のJSON出力対応

## 1. ドキュメント概要

### 1.1 目的
本ドキュメントは、dry-runモードでJSON形式を指定した際のデバッグ情報出力機能の実装手順とタスク管理を定義する。各フェーズでの具体的な作業項目、依存関係、完了基準を明確にする。

### 1.2 対象読者
- 実装者
- プロジェクトマネージャー
- レビュアー

### 1.3 関連ドキュメント
- [要件定義書](./01_requirements.ja.md)
- [アーキテクチャ設計書](./02_architecture.ja.md)
- [詳細設計書](./03_detailed_design.ja.md)

## 2. 実装アプローチ

### 2.1 段階的実装戦略

4つのフェーズに分けて段階的に実装する：

1. **Phase 1: データ構造とJSON変換** - 基盤となる型定義とJSON変換
2. **Phase 2: データ収集層** - デバッグ情報の収集ロジック
3. **Phase 3: フォーマット層** - テキスト出力のヘルパー関数
4. **Phase 4: 実行層の統合** - GroupExecutorとResourceManagerの統合

各フェーズは独立してテスト可能で、前のフェーズが完了してから次のフェーズに進む。

### 2.2 実装の原則

- **単一責任の原則**: 各関数は1つの明確な責任を持つ
- **テスト駆動**: 実装前にテストケースを定義
- **段階的なコミット**: 各タスク完了後にコミット
- **レビューの徹底**: 各フェーズ完了後にコードレビュー

## 3. Phase 1: データ構造とJSON変換

### 3.1 目的
新しいデータ構造を定義し、JSON変換をサポートする基盤を作る。

### 3.2 タスク一覧

#### 3.2.1 ResourceType と ResourceOperation の拡張

**ファイル**: `internal/runner/resource/types.go`

- [x] `ResourceTypeGroup` 定数を追加
- [x] `OperationAnalyze` 定数を追加
- [x] 既存の定数リストのコメントを更新

**完了基準**:
- 新しい定数が正しく定義されている
- ドキュメントコメントが追加されている

**見積もり**: 30分

#### 3.2.2 DebugInfo 構造体の追加

**ファイル**: `internal/runner/resource/types.go`

- [x] `DebugInfo` 構造体を定義
- [x] JSONタグ (`omitempty`) を適切に設定
- [x] ドキュメントコメントを追加

**完了基準**:
- 構造体が定義されている
- すべてのフィールドに適切なJSONタグがある
- ドキュメントコメントが詳細に記述されている

**見積もり**: 30分

#### 3.2.3 InheritanceAnalysis 構造体の追加

**ファイル**: `internal/runner/resource/types.go`

- [x] `InheritanceAnalysis` 構造体を定義
- [x] 設定値フィールドを定義 (GlobalEnvImport, GlobalAllowlist, GroupEnvImport, GroupAllowlist)
- [x] 計算値フィールドを定義 (InheritanceMode)
- [x] 差分情報フィールドを定義 (InheritedVariables, RemovedAllowlistVariables, UnavailableEnvImportVariables)
- [x] 各フィールドに適切なJSONタグを設定 (`omitempty` を差分情報フィールドに付与)
- [x] ドキュメントコメントを追加

**完了基準**:
- すべてのフィールドが正しく定義されている
- JSONタグが適切に設定されている
- 各フィールドの用途がコメントで説明されている

**見積もり**: 45分

#### 3.2.4 FinalEnvironment と EnvironmentVariable 構造体の追加

**ファイル**: `internal/runner/resource/types.go`

- [x] `FinalEnvironment` 構造体を定義
- [x] `EnvironmentVariable` 構造体を定義
- [x] JSONタグを適切に設定
- [x] ドキュメントコメントを追加 (特にSource フィールドの値の説明)

**完了基準**:
- 両方の構造体が定義されている
- `Source` フィールドの可能な値がコメントで説明されている
- JSONタグが適切に設定されている

**見積もり**: 30分

#### 3.2.5 ResourceAnalysis 構造体への DebugInfo フィールド追加

**ファイル**: `internal/runner/resource/types.go`

- [x] `ResourceAnalysis` に `DebugInfo *DebugInfo` フィールドを追加
- [x] JSONタグに `omitempty` を設定
- [x] ドキュメントコメントを追加

**完了基準**:
- フィールドが追加されている
- 既存のテストが正常にパスする

**見積もり**: 15分

#### 3.2.6 InheritanceMode の JSON 変換メソッド実装

**ファイル**: `internal/runner/runnertypes/config.go`

- [x] `MarshalJSON()` メソッドを実装
- [x] `UnmarshalJSON()` メソッドを実装
- [x] エラーハンドリングを追加 (ErrInvalidInheritanceMode)

**完了基準**:
- 両方のメソッドが実装されている
- 不正な値のハンドリングが適切に行われている

**見積もり**: 45分

#### 3.2.7 InheritanceMode JSON 変換のユニットテスト

**ファイル**: `internal/runner/runnertypes/config_test.go`

- [x] `TestInheritanceMode_MarshalJSON` を実装
  - [x] InheritanceModeInherit -> "inherit"
  - [x] InheritanceModeExplicit -> "explicit"
  - [x] InheritanceModeReject -> "reject"
- [x] `TestInheritanceMode_UnmarshalJSON` を実装
  - [x] "inherit" -> InheritanceModeInherit
  - [x] "explicit" -> InheritanceModeExplicit
  - [x] "reject" -> InheritanceModeReject
  - [x] 不正な値でエラーが返される

**完了基準**:
- すべてのテストケースが実装されている
- すべてのテストがパスする
- カバレッジが100%

**見積もり**: 1時間

#### 3.2.8 Phase 1 の統合確認

- [x] `make test` が成功する
- [x] `make lint` が成功する
- [x] すべての新規構造体が正しくJSON変換される
- [x] 既存のテストに影響がない (回帰テスト)

**完了基準**:
- すべてのチェックがパスする
- コードレビューが完了している

**見積もり**: 30分

### 3.3 Phase 1 の合計見積もり

**合計**: 約5時間

### 3.4 Phase 1 の依存関係

なし（最初のフェーズ）

### 3.5 Phase 1 のマイルストーン

- [x] Phase 1 完了
- [ ] コードレビュー完了
- [x] Phase 1 のコミット作成

## 4. Phase 2: データ収集層

### 4.1 目的
デバッグ情報を収集する関数を実装する。これらの関数がテキスト形式とJSON形式の両方で使用される Single Source of Truth となる。

### 4.2 タスク一覧

#### 4.2.1 collector.go ファイルの作成

**ファイル**: `internal/runner/debug/collector.go` (新規作成)

- [x] ファイルを作成
- [x] パッケージコメントを追加
- [x] 必要なインポートを追加

**完了基準**:
- ファイルが作成されている
- ビルドエラーがない

**見積もり**: 15分

#### 4.2.2 ヘルパー関数の実装

**ファイル**: `internal/runner/debug/collector.go`

- [x] `safeStringSlice()` を実装
- [x] `stringSliceToSet()` を実装
- [x] `setDifference()` を実装
- [x] `extractInternalVarNames()` を実装

**完了基準**:
- すべてのヘルパー関数が実装されている
- 各関数にドキュメントコメントがある
- エッジケース（nil, 空スライス）が適切に処理される

**見積もり**: 1時間

#### 4.2.3 CollectInheritanceAnalysis 関数の実装

**ファイル**: `internal/runner/debug/collector.go`

- [x] 関数シグネチャを定義
- [x] DetailLevelSummary の場合に nil を返す処理を実装
- [x] 設定値フィールドの収集を実装
- [x] 計算値フィールド (InheritanceMode) の設定を実装
- [x] DetailLevelFull の場合の差分情報計算を実装
  - [x] InheritedVariables の計算
  - [x] RemovedAllowlistVariables の計算
  - [x] UnavailableEnvImportVariables の計算
- [x] ドキュメントコメントを追加

**完了基準**:
- 関数が完全に実装されている
- すべてのDetail Levelで正しく動作する
- エラーハンドリングが適切

**見積もり**: 2時間

#### 4.2.4 CollectInheritanceAnalysis のユニットテスト

**ファイル**: `internal/runner/debug/collector_test.go` (新規作成)

- [x] `TestCollectInheritanceAnalysis` を実装
  - [x] DetailLevelSummary: nil を返す
  - [x] DetailLevelDetailed: 基本フィールドのみ
  - [x] DetailLevelFull: すべてのフィールド
  - [x] InheritanceModeInherit の場合
  - [x] InheritanceModeExplicit の場合
  - [x] InheritanceModeReject の場合
  - [x] nil / 空スライスのエッジケース
- [x] テストヘルパー関数を実装
- [x] テストフィクスチャを作成

**完了基準**:
- すべてのテストケースが実装されている
- すべてのテストがパスする
- コードカバレッジが90%以上

**見積もり**: 2.5時間

#### 4.2.5 CollectFinalEnvironment 関数の実装

**ファイル**: `internal/runner/debug/collector.go`

- [x] 関数シグネチャを定義
- [x] DetailLevelFull 以外で nil を返す処理を実装
- [x] 環境変数マップをループ処理
- [x] センシティブ情報のマスキング処理を実装
- [x] `mapEnvVarSource()` ヘルパー関数を実装
- [x] ドキュメントコメントを追加

**完了基準**:
- 関数が完全に実装されている
- センシティブ情報が適切にマスクされる
- Source フィールドが正しくマッピングされる

**見積もり**: 1.5時間

#### 4.2.6 CollectFinalEnvironment のユニットテスト

**ファイル**: `internal/runner/debug/collector_test.go`

- [x] `TestCollectFinalEnvironment` を実装
  - [x] DetailLevelSummary: nil を返す
  - [x] DetailLevelDetailed: nil を返す
  - [x] DetailLevelFull + showSensitive=true: すべての値を含む
  - [x] DetailLevelFull + showSensitive=false: センシティブ値をマスク
  - [x] 各 Source タイプのテスト
  - [x] 空の環境変数マップのテスト

**完了基準**:
- すべてのテストケースが実装されている
- すべてのテストがパスする
- コードカバレッジが90%以上

**見積もり**: 2時間

#### 4.2.7 Phase 2 の統合確認

- [x] `make test` が成功する
- [x] `make lint` が成功する
- [x] データ収集関数が各Detail Levelで正しく動作する
- [x] 既存のテストに影響がない

**完了基準**:
- すべてのチェックがパスする
- コードレビューが完了している

**見積もり**: 30分

### 4.3 Phase 2 の合計見積もり

**合計**: 約9.5時間

### 4.4 Phase 2 の依存関係

- Phase 1 が完了していること

### 4.5 Phase 2 のマイルストーン

- [x] Phase 2 完了
- [ ] コードレビュー完了
- [x] Phase 2 のコミット作成

## 5. Phase 3: フォーマット層

### 5.1 目的
収集したデバッグ情報をテキスト形式で出力するヘルパー関数を実装する。既存の `PrintFromEnvInheritance` と `PrintFinalEnvironment` のロジックを再利用する。

### 5.2 タスク一覧

#### 5.2.1 formatter.go ファイルの作成

**ファイル**: `internal/runner/debug/formatter.go` (新規作成)

- [x] ファイルを作成
- [x] パッケージコメントを追加
- [x] 必要なインポートを追加

**完了基準**:
- ファイルが作成されている
- ビルドエラーがない

**見積もり**: 15分

#### 5.2.2 フォーマットヘルパー関数の実装

**ファイル**: `internal/runner/debug/formatter.go`

- [x] `formatStringSlice()` を実装
- [x] `formatGroupField()` を実装

**完了基準**:
- ヘルパー関数が実装されている
- 既存の出力形式と一致する

**見積もり**: 30分

#### 5.2.3 FormatInheritanceAnalysisText 関数の実装

**ファイル**: `internal/runner/debug/formatter.go`

- [x] 関数シグネチャを定義
- [x] nil チェックを実装
- [x] ヘッダーセクションの出力
- [x] Global Level セクションの出力
- [x] Group Level セクションの出力
- [x] Inheritance Mode セクションの出力
- [x] 差分情報セクションの出力（存在する場合のみ）
  - [x] Inherited Variables
  - [x] Removed Allowlist Variables
  - [x] Unavailable Env Import Variables
- [x] ドキュメントコメントを追加

**完了基準**:
- 関数が完全に実装されている
- 既存の `PrintFromEnvInheritance` と同じ出力形式
- すべてのDetail Levelで正しく動作する

**見積もり**: 2時間

#### 5.2.4 FormatInheritanceAnalysisText のユニットテスト

**ファイル**: `internal/runner/debug/formatter_test.go` (新規作成)

- [x] `TestFormatInheritanceAnalysisText` を実装
  - [x] nil の場合: 何も出力しない
  - [x] 基本フィールドのみの場合
  - [x] すべてのフィールドがある場合
  - [x] 各継承モードでの出力
  - [x] 出力内容を既存の `PrintFromEnvInheritance` と比較

**完了基準**:
- すべてのテストケースが実装されている
- すべてのテストがパスする
- 既存の出力形式と一致している

**見積もり**: 2時間

#### 5.2.5 FormatFinalEnvironmentText 関数の実装

**ファイル**: `internal/runner/debug/formatter.go`

- [x] 関数シグネチャを定義
- [x] nil チェックを実装
- [x] ヘッダーの出力
- [x] 変数名のソート
- [x] 各環境変数の出力
- [x] マスクされた値の処理
- [x] ドキュメントコメントを追加

**完了基準**:
- 関数が完全に実装されている
- 既存の `PrintFinalEnvironment` と同じ出力形式
- 変数が適切にソートされている

**見積もり**: 1時間

#### 5.2.6 FormatFinalEnvironmentText のユニットテスト

**ファイル**: `internal/runner/debug/formatter_test.go`

- [x] `TestFormatFinalEnvironmentText` を実装
  - [x] nil の場合: 何も出力しない
  - [x] 空の環境変数の場合
  - [x] 複数の環境変数がある場合
  - [x] マスクされた変数がある場合
  - [x] 各 Source タイプでの出力
  - [x] 出力内容を既存の `PrintFinalEnvironment` と比較

**完了基準**:
- すべてのテストケースが実装されている
- すべてのテストがパスする
- 既存の出力形式と一致している

**見積もり**: 1.5時間

#### 5.2.7 既存関数との出力比較テスト

**ファイル**: `internal/runner/debug/formatter_test.go`

- [x] `TestFormatConsistency` を実装
  - [x] 同じ入力で既存関数と新関数の出力を比較
  - [x] 各Detail Levelでの一貫性を検証

**完了基準**:
- 既存関数と新関数の出力が一致する
- すべてのDetail Levelで一貫性がある

**見積もり**: 1時間

#### 5.2.8 Phase 3 の統合確認

- [x] `make test` が成功する
- [x] `make lint` が成功する
- [x] フォーマット関数が正しく動作する
- [x] 既存のテストに影響がない

**完了基準**:
- すべてのチェックがパスする
- コードレビューが完了している

**見積もり**: 30分

### 5.3 Phase 3 の合計見積もり

**合計**: 約8.5時間

### 5.4 Phase 3 の依存関係

- Phase 2 が完了していること

### 5.5 Phase 3 のマイルストーン

- [x] Phase 3 完了
- [ ] コードレビュー完了
- [x] Phase 3 のコミット作成

## 6. Phase 4: 実行層の統合

### 6.1 目的
GroupExecutor と ResourceManager を修正し、データ収集関数とフォーマット関数を統合する。

### 6.2 タスク一覧

#### 6.2.1 ResourceManager への新メソッド追加

**ファイル**: `internal/runner/resource/manager.go`

- [x] `RecordGroupAnalysis()` メソッドを実装
  - [x] 引数の検証
  - [x] ResourceAnalysis の作成
  - [x] DebugInfo の設定
  - [x] リストへの追加
  - [x] エラーハンドリング
- [x] `UpdateLastCommandDebugInfo()` メソッドを実装
  - [x] 最後のコマンドを検索
  - [x] DebugInfo のマージ
  - [x] エラーハンドリング
- [x] ドキュメントコメントを追加

**完了基準**:
- 両方のメソッドが実装されている
- エラーケースが適切に処理される
- スレッドセーフである

**見積もり**: 2時間

#### 6.2.2 ResourceManager メソッドのユニットテスト

**ファイル**: `internal/runner/resource/manager_test.go`

- [x] `TestRecordGroupAnalysis` を実装
  - [x] 正常ケース
  - [x] nil manager
  - [x] ResourceAnalyses への追加を検証
- [x] `TestUpdateLastCommandDebugInfo` を実装
  - [x] 正常ケース
  - [x] コマンドが存在しない場合
  - [x] DebugInfo のマージを検証

**完了基準**:
- すべてのテストケースが実装されている
- すべてのテストがパスする
- エッジケースがカバーされている

**見積もり**: 2時間

#### 6.2.3 GroupExecutor への dryRunFormat フィールド追加

**ファイル**: `internal/runner/group_executor.go`

- [x] `dryRunFormat` フィールドを追加
- [x] コンストラクタを修正
- [x] 既存のコードへの影響を確認

**完了基準**:
- フィールドが追加されている
- コンストラクタが正しく動作する
- 既存のテストがパスする

**見積もり**: 30分

#### 6.2.4 GroupExecutor のグループレベル出力の修正

**ファイル**: `internal/runner/group_executor.go`

- [x] 既存の `PrintFromEnvInheritance` 呼び出しを特定
- [x] `CollectInheritanceAnalysis` 呼び出しに変更
- [x] 出力形式による条件分岐を実装
  - [x] JSON形式: `RecordGroupAnalysis` を呼び出し
  - [x] TEXT形式: `FormatInheritanceAnalysisText` を呼び出し
- [x] エラーハンドリングを追加

**完了基準**:
- 両方の出力形式で正しく動作する
- エラーが適切に処理される
- 既存のテキスト出力が維持される

**見積もり**: 1.5時間

#### 6.2.5 GroupExecutor のコマンドレベル出力の修正

**ファイル**: `internal/runner/group_executor.go`

- [x] 既存の `PrintFinalEnvironment` 呼び出しを特定
- [x] `CollectFinalEnvironment` 呼び出しに変更
- [x] 出力形式による条件分岐を実装
  - [x] JSON形式: `UpdateLastCommandDebugInfo` を呼び出し
  - [x] TEXT形式: `FormatFinalEnvironmentText` を呼び出し
- [x] エラーハンドリングを追加

**完了基準**:
- 両方の出力形式で正しく動作する
- エラーが適切に処理される
- 既存のテキスト出力が維持される

**見積もり**: 1.5時間

#### 6.2.6 main.go での dryRunFormat の伝搬

**ファイル**: `cmd/runner/main.go`

- [x] `GroupExecutor` の作成箇所を特定
- [x] `dryRunFormat` パラメータを追加
- [x] フラグからの値の取得を確認

**完了基準**:
- `dryRunFormat` が正しく伝搬される
- ビルドエラーがない

**見積もり**: 30分

#### 6.2.7 統合テスト: JSON出力の検証

**ファイル**: `cmd/runner/dry_run_integration_test.go`

- [x] `TestDryRunJSONOutput_WithDebugInfo` を実装
  - [x] テスト設定ファイルの作成
  - [x] JSON形式でdry-runを実行
  - [x] JSON出力のパースを検証
  - [x] グループレベルのDebugInfo を検証
  - [x] コマンドレベルのDebugInfo を検証

**完了基準**:
- テストが実装されている
- テストがパスする
- JSON出力が有効である

**見積もり**: 2.5時間

#### 6.2.8 統合テスト: Detail Level別の検証

**ファイル**: `cmd/runner/dry_run_integration_test.go`

- [x] `TestDryRunJSONOutput_DetailLevels` を実装
  - [x] DetailLevelSummary: debug_info なし
  - [x] DetailLevelDetailed: 基本情報のみ
  - [x] DetailLevelFull: すべての情報

**完了基準**:
- すべてのDetail Levelでテストがパスする
- 出力内容が仕様と一致する

**見積もり**: 2時間

#### 6.2.9 回帰テスト: テキスト出力の検証

**ファイル**: `cmd/runner/dry_run_integration_test.go`

- [x] `TestDryRunTextOutput_Unchanged` を実装
  - [x] テキスト形式でdry-runを実行
  - [x] 既存の出力形式が維持されていることを確認

**完了基準**:
- テキスト出力が変更されていない
- 既存の動作が維持されている

**見積もり**: 1時間

#### 6.2.10 実際の設定ファイルでの動作確認

- [x] サンプル設定ファイルを作成 (testdata/dry_run_debug_test.toml)
- [x] 各Detail Levelで実行（統合テストに含まれる）
- [x] JSON出力を `jq` でパース（統合テストで検証）
- [x] テキスト出力を目視確認（統合テストで検証）
- [x] エラーケースの確認（センシティブマスキングテストに含まれる）

**完了基準**:
- すべての設定で正しく動作する
- JSON出力が有効である
- エラーが適切に処理される

**見積もり**: 1.5時間

#### 6.2.11 Phase 4 の統合確認

- [x] `make test` が成功する
- [x] `make lint` が成功する
- [x] `make build` が成功する
- [x] すべての統合テストがパスする
- [x] 既存のテストに影響がない（回帰なし）

**完了基準**:
- すべてのチェックがパスする
- コードレビューが完了している

**見積もり**: 1時間

### 6.3 Phase 4 の合計見積もり

**合計**: 約16時間

### 6.4 Phase 4 の依存関係

- Phase 1, 2, 3 が完了していること

### 6.5 Phase 4 のマイルストーン

- [x] Phase 4 完了
- [ ] コードレビュー完了
- [x] Phase 4 のコミット作成

## 7. ドキュメント更新

### 7.1 タスク一覧

#### 7.1.1 ユーザーマニュアルの更新

**ファイル**: `docs/user/dry_run.md` または新規作成

- [ ] JSON出力機能の説明を追加
- [ ] Detail Level別の出力例を追加
- [ ] 使用例を追加
- [ ] センシティブ情報のマスキングについて説明

**完了基準**:
- ドキュメントが完全で分かりやすい
- すべての機能がカバーされている

**見積もり**: 2時間

#### 7.1.2 JSON Schema のドキュメント化

**ファイル**: `docs/user/json_schema.md` (新規作成) または既存ドキュメントに追加

- [ ] `DebugInfo` の説明
- [ ] `InheritanceAnalysis` の説明
- [ ] `FinalEnvironment` の説明
- [ ] `EnvironmentVariable` の説明
- [ ] 各フィールドの説明とサンプル

**完了基準**:
- JSON Schemaが完全に文書化されている
- サンプルが含まれている

**見積もり**: 2時間

#### 7.1.3 README.md の更新

**ファイル**: `README.md`

- [ ] JSON出力機能の追加を記載
- [ ] 簡単な使用例を追加

**完了基準**:
- 新機能が適切に紹介されている

**見積もり**: 30分

#### 7.1.4 CHANGELOG の更新

**ファイル**: `CHANGELOG.md` (存在する場合)

- [ ] 新機能として記載
- [ ] 変更内容のサマリーを追加

**完了基準**:
- CHANGELOGが更新されている

**見積もり**: 15分

### 7.2 ドキュメント更新の合計見積もり

**合計**: 約4.75時間

### 7.3 ドキュメント更新の依存関係

- Phase 4 が完了していること

## 8. 最終確認とリリース準備

### 8.1 タスク一覧

#### 8.1.1 全体テストの実行

- [ ] すべてのユニットテストが成功
- [ ] すべての統合テストが成功
- [ ] すべての回帰テストが成功
- [ ] `make test` が成功
- [ ] `make lint` が成功
- [ ] `make build` が成功

**完了基準**:
- すべてのテストがパスする

**見積もり**: 30分

#### 8.1.2 パフォーマンステスト

- [ ] JSON出力のオーバーヘッドを計測
- [ ] メモリ使用量を計測
- [ ] 大規模設定ファイルでの動作確認

**完了基準**:
- パフォーマンス目標を達成している（10%以内のオーバーヘッド）

**見積もり**: 1時間

#### 8.1.3 最終コードレビュー

- [ ] すべてのコードがレビュー済み
- [ ] レビューコメントが反映済み
- [ ] コーディング規約に準拠

**完了基準**:
- コードレビューが承認されている

**見積もり**: 2時間

#### 8.1.4 リリースノートの作成

- [ ] 新機能の説明
- [ ] 使用方法の概要
- [ ] 既存機能への影響（なし）
- [ ] 既知の制限事項（あれば）

**完了基準**:
- リリースノートが完成している

**見積もり**: 1時間

### 8.2 最終確認の合計見積もり

**合計**: 約4.5時間

## 9. 実装スケジュールのサマリー

### 9.1 フェーズ別の見積もり

| フェーズ | 内容 | 見積もり時間 | 依存関係 |
|---------|------|-------------|---------|
| Phase 1 | データ構造とJSON変換 | 5時間 | なし |
| Phase 2 | データ収集層 | 9.5時間 | Phase 1 |
| Phase 3 | フォーマット層 | 8.5時間 | Phase 2 |
| Phase 4 | 実行層の統合 | 16時間 | Phase 1-3 |
| ドキュメント | ユーザーマニュアル等 | 4.75時間 | Phase 4 |
| 最終確認 | テスト・レビュー | 4.5時間 | すべて |
| **合計** | | **48.25時間** | |

### 9.2 推奨スケジュール

1日あたり4-6時間の実装時間を想定した場合：

- **Week 1 (Day 1-2)**: Phase 1 完了
- **Week 1 (Day 3-5)**: Phase 2 完了
- **Week 2 (Day 1-3)**: Phase 3 完了
- **Week 2 (Day 4-5) + Week 3 (Day 1-3)**: Phase 4 完了
- **Week 3 (Day 4)**: ドキュメント更新
- **Week 3 (Day 5)**: 最終確認とリリース準備

**合計期間**: 約3週間

## 10. リスク管理

### 10.1 潜在的なリスク

| リスク | 影響 | 確率 | 対策 |
|--------|------|------|------|
| 既存コードとの統合が複雑 | 中 | 中 | Phase 4を細かいタスクに分割 |
| テキスト形式との一貫性が取れない | 高 | 低 | Phase 3で徹底的にテスト |
| パフォーマンスの低下 | 中 | 低 | Detail Levelでの早期リターンを実装 |
| センシティブ情報の漏洩 | 高 | 低 | デフォルトでマスク、徹底的なテスト |
| JSON Schema の後方互換性 | 中 | 低 | `omitempty` タグの使用、既存テストで確認 |

### 10.2 リスク軽減策

1. **段階的な実装**: 各フェーズを独立してテスト
2. **頻繁なコミット**: 問題発生時にロールバック可能
3. **徹底的なテスト**: ユニットテスト、統合テスト、回帰テスト
4. **コードレビュー**: 各フェーズ完了後にレビュー
5. **ドキュメント**: 実装と並行してドキュメント更新

## 11. 成功基準

プロジェクト完了時の成功基準：

- [ ] すべてのテストがパスする
- [ ] `--dry-run --dry-run-format json` で有効なJSONが出力される
- [ ] `jq` などのJSONパーサーで処理可能
- [ ] テキスト形式の出力が変更されていない
- [ ] すべてのDetail Levelで正しく動作する
- [ ] パフォーマンス目標を達成している（10%以内のオーバーヘッド）
- [ ] センシティブ情報が適切にマスクされる
- [ ] ドキュメントが完全で分かりやすい
- [ ] コードレビューが承認されている

## 12. 実装の進捗管理

各タスクのチェックボックスを使用して進捗を管理する。

### 12.1 進捗の記録方法

- [ ] 未着手: チェックボックスが空
- [x] 完了: チェックボックスにチェックを入れる

### 12.2 フェーズ完了の報告

各フェーズ完了時に：
1. すべてのタスクのチェックボックスを確認
2. テストがすべてパスすることを確認
3. コードレビューを依頼
4. マイルストーンのチェックボックスにチェックを入れる
5. 次のフェーズに進む

## 13. 補足情報

### 13.1 開発環境

- Go 1.23.10
- 必要なツール: `make`, `golangci-lint`, `jq`

### 13.2 テスト実行コマンド

```bash
# すべてのテストを実行
make test

# 特定のパッケージのテストを実行
go test -tags test -v ./internal/runner/debug/...

# カバレッジを確認
go test -tags test -v -coverprofile=coverage.out ./internal/runner/debug/...
go tool cover -html=coverage.out

# Linterを実行
make lint
```

### 13.3 デバッグ用のコマンド

```bash
# テキスト形式でdry-run
./build/runner --config test.toml --dry-run --dry-run-format text --dry-run-detail full

# JSON形式でdry-run
./build/runner --config test.toml --dry-run --dry-run-format json --dry-run-detail full

# JSON出力をjqでパース
./build/runner --config test.toml --dry-run --dry-run-format json --dry-run-detail full | jq .

# debug_infoフィールドのみ抽出
./build/runner --config test.toml --dry-run --dry-run-format json --dry-run-detail full | jq '.resource_analyses[] | select(.debug_info != null) | .debug_info'
```

### 13.4 問い合わせ先

実装中に質問や問題が発生した場合：
- アーキテクチャに関する質問: [アーキテクチャ設計書](./02_architecture.ja.md) を参照
- 詳細な仕様に関する質問: [詳細設計書](./03_detailed_design.ja.md) を参照
- 要件に関する質問: [要件定義書](./01_requirements.ja.md) を参照

---

**ドキュメント作成日**: 2025-10-30
**最終更新日**: 2025-10-30
**バージョン**: 1.0
