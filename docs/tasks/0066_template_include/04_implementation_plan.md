# 実装計画書: テンプレートファイルのInclude機能

## 概要

本ドキュメントは、テンプレートファイルのInclude機能の実装進捗を追跡する。

## Phase 1: 基本構造（完了）

- [x] `ConfigSpec` に `Includes` フィールドを追加
- [x] `TemplateFileSpec` 構造体を作成
- [x] `TemplateSource` 構造体を作成
- [x] エラー型を定義

## Phase 2: コアコンポーネント（完了）

- [x] `PathResolver` の実装とテスト
- [x] `TemplateFileLoader` の実装とテスト
- [x] `TemplateMerger` の実装とテスト

## Phase 3: Loader 統合（完了）

- [x] `LoadConfig` の変更（DisallowUnknownFields対応）
- [x] `processIncludes` の実装
- [x] baseDir 決定の設計と実装

## Phase 4: Verification 統合（完了）

- [x] `Verification Manager` の拡張
- [x] `runner` での include ファイル検証

## Phase 5: テストとドキュメント（完了）

- [x] 統合テストの作成
- [x] E2Eテストの作成
- [x] ユーザードキュメントの作成
- [x] サンプル設定ファイルの作成

## Phase 6: 受け入れ基準の検証

### F-006の受け入れ基準検証

#### AC-1: メイン設定ファイルのハッシュ検証

- [x] テスト: メイン設定ファイルのハッシュが検証されることを確認
  - テストファイル: `internal/runner/config/loader_verification_test.go`
  - テスト関数: `TestLoadConfig_MainConfigHashVerification`
- 実装箇所: `verification/manager.go` の `VerifyAndReadConfigFile`
- 検証方法: 既存の検証機構が動作していることを確認

#### AC-2: includeされた全てのテンプレートファイルのハッシュ検証

- [x] テスト: 単一includeファイルのハッシュ検証
  - テスト関数: `TestLoadConfig_SingleIncludeFileHashVerification`
- [x] テスト: 複数includeファイルのハッシュ検証
  - テスト関数: `TestLoadConfig_MultipleIncludeFilesHashVerification`
- 実装箇所: `verification/manager.go` の `VerifyAndReadTemplateFile`
- 検証方法: `VerifyAndReadTemplateFile` が全includeファイルに対して呼ばれることを確認

#### AC-3: ハッシュ検証失敗時のエラー処理

- [x] テスト: いずれかのファイルのハッシュ検証が失敗した場合、実行前にエラーが返されることを確認
  - テスト関数: `TestLoadConfig_HashMismatchReturnsError`
- 実装箇所: `verification.Manager` のエラーハンドリング
- 検証方法: 改ざんされたファイルで実行し、エラーが返されることを確認

#### AC-4: ハッシュ未記録時のエラー処理

- [x] テスト: テンプレートファイルのハッシュが記録されていない場合、実行前にエラーが返されることを確認
  - テスト関数: `TestLoadConfig_MissingHashReturnsError`
- 実装箇所: `verification.Manager` のエラーハンドリング
- 検証方法: ハッシュファイルが存在しない状態で実行し、エラーが返されることを確認

#### AC-5: 改ざん検知

- [x] テスト: テンプレートファイルが改ざんされた場合、実行前にエラーが返されることを確認
  - テスト関数: `TestLoadConfig_TamperedFileDetection`
- 実装箇所: `verification.Manager` のハッシュ比較
- 検証方法: 記録後にファイル内容を変更し、エラーが返されることを確認

### セキュリティ要件の検証

#### SEC-1: TOCTOU対策

- [x] テスト: ファイル検証と読み込みが原子的に行われることを確認
  - テスト関数: `TestVerifyAndReadTemplateFile_AtomicOperation`
- 実装箇所: `verification.Manager.VerifyAndReadTemplateFile`
- 検証方法: 1回のファイル読み込みでハッシュ検証とコンテンツ取得が完了することを確認

#### SEC-2: 検証なしでの直接読み込みの防止

- [x] テスト: 本番環境で検証付きの読み込みが使用されることの確認
  - テスト関数: `TestLoadConfig_ProductionPathUsesVerification`
- [x] テスト: `NewLoader` が nil の verificationManager を拒否することの確認
  - テスト関数: `TestNewLoader_RequiresVerificationManager`
- 実装箇所: `config/loader.go` の `NewLoader`
- 検証方法: 本番コードパスで必ず `VerifyAndReadTemplateFile` が使用されることを確認
