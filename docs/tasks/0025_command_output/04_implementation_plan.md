# 実装計画書：コマンド出力キャプチャ機能

## 1. 概要 (Overview)

本文書は、go-safe-cmd-runnerにおけるコマンド出力キャプチャ機能の実装計画を定義する。要件定義書、アーキテクチャ設計書、詳細仕様書に基づき、段階的な実装アプローチを採用し、テスト駆動開発（TDD）による確実な実装を行う。

## 2. 実装戦略 (Implementation Strategy)

### 2.1 開発アプローチ
- **テスト駆動開発（TDD）**: 全ての実装において先にテストを作成し、その後実装を行う
- **セキュリティテストの网羅的実施**
- [ ] パストラバーサル攻撃のテスト（PathValidatorでの".."検出）
- [ ] シンボリックリンク攻撃の統合テスト（既存SecurityValidator + 出力キャプチャ連携）
- [ ] 権限昇格攻撃のテスト（不正なファイル権限設定への対策）
- [ ] ディスク容量枯渇攻撃のテスト（出力サイズ制限機能）行う
- **段階的実装**: 機能を4つのフェーズに分けて段階的に実装する
- **既存アーキテクチャとの統合**: 現在のResourceManager/Executor パターンを拡張する
- **セキュリティファースト**: 各段階でセキュリティテストを実装・検証する

### 2.2 品質保証

- 各フェーズ完了時に`make test`、`make lint`を実行
- セキュリティテストの必須実装
- エラーハンドリングの網羅的テスト
- パフォーマンステストの実装

## 3. 実装フェーズ計画

### Phase 1: 基本構造とデータ構造の実装

#### 3.1.1 目標
既存構造体の拡張と新規データ構造の実装を完了し、基本的な出力キャプチャ機能のテストを作成する。

#### 3.1.2 実装項目

**データ構造拡張**
- [x] `Command`構造体にoutputフィールドを追加
- [x] `GlobalConfig`構造体にmax_output_sizeフィールドを追加
- [x] TOML設定の解析機能拡張

**新規データ構造作成**
- [x] `Config`構造体の実装（旧`OutputConfig`、lintエラー修正）
- [x] `Capture`構造体の実装（旧`OutputCapture`、lintエラー修正）
- [x] `Analysis`構造体の実装（旧`OutputAnalysis`、lintエラー修正）
- [x] `CaptureError`エラー型の実装（旧`OutputCaptureError`、lintエラー修正）

**基本インターフェース定義**
- [x] `CaptureManager`インターフェースの定義（旧`OutputCaptureManager`、lintエラー修正）
- [x] `PathValidator`インターフェースの定義（簡素化版）
- [ ] 既存`security.Validator`の拡張（出力ファイル書き込み権限チェック機能追加）
- [x] `FileManager`インターフェース（safefileio活用版）の定義

#### 3.1.3 テスト作成（TDD）

**単体テスト作成**
- [x] `internal/runner/output/types_test.go` - データ構造のテスト
- [x] `internal/runner/output/errors_test.go` - エラー型のテスト
- [x] `internal/runner/config/config_test.go` - 設定解析のテスト

**テスト実行と確認**
- [x] テストを実行して期待通りに失敗することを確認
- [x] 基本構造のテストコミット

#### 3.1.4 実装
- [x] データ構造の実装
- [x] エラー型の実装
- [x] 設定解析機能の実装
- [x] 全テストが通過することを確認

#### 3.1.5 検証
- [x] `make test`実行とパス確認
- [x] `make lint`実行と問題修正（stuttering issues完全修正）
- [x] Phase 1完了コミット

### Phase 2: パス処理とセキュリティ機能の実装

#### 3.2.1 目標
パス検証、セキュリティチェック、権限確認機能を実装し、セキュリティリスクを適切に評価できる基盤を構築する。

#### 3.2.2 実装項目

**パス処理機能**
- [ ] `DefaultPathValidator`の実装（基本的なパス検証のみ）
- [ ] 絶対パス検証機能
- [ ] 相対パス検証機能（WorkDir基準）
- [ ] パストラバーサル防止機能（".."の検出）
- [ ] 注意: 包括的なシンボリックリンク検証は既存SecurityValidator.ValidateDirectoryPermissionsで実施

**権限確認機能（既存SecurityValidator拡張）**
- [ ] `security.Validator`のValidateOutputWritePermission実装
- [ ] 既存ValidateDirectoryPermissions活用によるシンボリックリンク攻撃防止
- [ ] ディレクトリ書き込み権限確認（UID/GID固有）
- [ ] ファイル書き込み権限確認（Lstat使用）
- [ ] グループメンバーシップ確認（`groupmembership`パッケージ利用、エラーハンドリング改善）

**セキュリティ機能**
- [ ] セキュリティリスク評価機能
- [ ] 危険なパスパターンの検出
- [ ] システム重要ファイル保護

#### 3.2.3 テスト作成（TDD）

**パス処理テスト**
- [ ] `internal/runner/output/path_test.go` - パス検証のテスト
  - 有効な絶対パス・相対パスのテスト
  - パストラバーサル攻撃のテスト（".."含有パス）
  - エラーケースのテスト
  - 注意: シンボリックリンク攻撃のテストは既存SecurityValidator側で実施済み

**権限確認テスト**
- [ ] `internal/runner/security/file_validation_test.go` - ValidateOutputWritePermission追加テスト
  - 既存ValidateDirectoryPermissions統合テスト
  - 出力ファイル固有の権限テスト（所有者・グループ・その他）
  - UID固有の権限確認テスト
  - エラーハンドリング改善テスト（isUserInGroup等）

**セキュリティテスト**
- [ ] `internal/runner/output/security_test.go` - セキュリティのテスト
  - リスク評価のテスト（evaluateSecurityRisk機能）
  - 危険パターン検出のテスト（ConfigValidator）
  - システムファイル保護のテスト
  - 注意: シンボリックリンク攻撃テストは既存SecurityValidatorで包括的に実施済み

#### 3.2.4 実装
- [ ] `DefaultPathValidator`の実装（基本的なパス検証・正規化のみ）
- [ ] `security.Validator`の`ValidateOutputWritePermission`メソッド実装
- [ ] 既存`ValidateDirectoryPermissions`との統合確認
- [ ] セキュリティリスク評価の実装（`evaluateSecurityRisk`）
- [ ] 全テストが通過することを確認

#### 3.2.5 検証
- [ ] セキュリティテストの実行と検証
- [ ] `make test`実行とパス確認
- [ ] `make lint`実行と問題修正
- [ ] Phase 2完了コミット

### Phase 3: ファイル操作とOutput管理機能の実装

#### 3.3.1 目標
ファイルシステム操作、出力キャプチャ管理、エラーハンドリングを実装し、基本的な出力キャプチャ機能を完成させる。

#### 3.3.2 実装項目

**safefileio活用FileManager実装**
- [ ] `SafeFileManager`の実装
- [ ] ディレクトリ自動作成機能
- [ ] `safefileio.SafeWriteFileOverwrite`を活用した安全なファイル書き込み

**OutputCaptureManager実装**
- [ ] `DefaultOutputCaptureManager`の実装
- [ ] `PrepareOutput`メソッドの実装（メモリバッファ版）
- [ ] `WriteOutput`メソッドの実装（メモリバッファ + サイズ制限）
- [ ] `FinalizeOutput`メソッドの実装（safefileio活用版）
- [ ] `CleanupOutput`メソッドの実装（簡素化版）
- [ ] `AnalyzeOutput`メソッドの実装（Dry-Run用）

#### 3.3.3 テスト作成（TDD）

**SafeFileManagerテスト**
- [ ] `internal/runner/output/file_test.go` - ファイル操作のテスト
  - ディレクトリ作成のテスト
  - safefileio活用の書き込みテスト

**OutputCaptureManagerテスト**
- [ ] `internal/runner/output/manager_test.go` - 管理機能のテスト
  - PrepareOutputのテスト（メモリバッファ版）
  - WriteOutputのテスト（サイズ制限含む）
  - FinalizeOutputのテスト（safefileio活用版）
  - CleanupOutputのテスト（簡素化版）
  - AnalyzeOutputのテスト

**統合テスト**
- [ ] `internal/runner/output/integration_test.go` - 統合テスト
  - 完全な出力キャプチャフローのテスト
  - エラー時のクリーンアップテスト
  - safefileioセキュリティ機能のテスト

#### 3.3.4 実装
- [ ] `SafeFileManager`の実装
- [ ] `DefaultOutputCaptureManager`の実装
- [ ] 全テストが通過することを確認

#### 3.3.5 検証
- [ ] 統合テストの実行と検証
- [ ] エラーハンドリングテストの実行
- [ ] `make test`実行とパス確認
- [ ] `make lint`実行と問題修正
- [ ] Phase 3完了コミット

### Phase 4: ResourceManager統合とExecutor拡張

#### 3.4.1 目標
既存のResourceManager、Executorとの統合を完了し、実際のコマンド実行での出力キャプチャ機能を実現する。

#### 3.4.2 実装項目

**Executor拡張**
- [ ] `ExecuteConfig`構造体にStdoutWriterフィールドを追加
- [ ] `DefaultCommandExecutor.Execute`メソッドの拡張
- [ ] 標準出力の柔軟な書き込み先対応

**TeeOutputWriter実装**
- [ ] `TeeOutputWriter`の実装
- [ ] Runner標準出力とファイルへの同時出力
- [ ] エラー時の適切な処理

**ResourceManager拡張**
- [ ] `NormalResourceManager`の拡張
- [ ] 出力キャプチャ付きコマンド実行
- [ ] エラー時のクリーンアップ処理
- [ ] `DryRunResourceManager`の拡張
- [ ] Dry-Run時の出力分析機能

#### 3.4.3 テスト作成（TDD）

**Executor拡張テスト**
- [ ] `internal/runner/executor/executor_test.go` - Executor拡張のテスト
  - StdoutWriterを使用したテスト
  - 標準出力とファイル出力の並行テスト

**TeeOutputWriterテスト**
- [ ] `internal/runner/output/writer_test.go` - Writerのテスト
  - Tee機能のテスト
  - エラー時の動作テスト

**ResourceManager統合テスト**
- [ ] `internal/runner/resource/manager_test.go` - ResourceManager統合のテスト
  - 出力キャプチャ付きコマンド実行のテスト
  - エラー時のクリーンアップテスト
  - Dry-Run時の分析テスト

**エンドツーエンドテスト**
- [ ] `internal/runner/runner_test.go` - 完全統合テスト
  - 実際のコマンド実行での出力キャプチャテスト
  - 複数コマンドの連続実行テスト
  - 設定ファイル経由の実行テスト

#### 3.4.4 実装
- [ ] `ExecuteConfig`の拡張
- [ ] `DefaultCommandExecutor`の拡張
- [ ] `TeeOutputWriter`の実装
- [ ] `NormalResourceManager`の拡張
- [ ] `DryRunResourceManager`の拡張
- [ ] 全テストが通過することを確認

#### 3.4.5 検証
- [ ] エンドツーエンドテストの実行と検証
- [ ] 実際のTOMLファイルを使用した動作確認
- [ ] `make test`実行とパス確認
- [ ] `make lint`実行と問題修正
- [ ] Phase 4完了コミット

### Phase 5: 最終統合とドキュメント整備

#### 3.5.1 目標
全機能の統合確認、パフォーマンステスト、セキュリティテストの実行と、実用的なドキュメントの整備を完了する。

#### 3.5.2 実装項目

**統合テストとパフォーマンステスト**
- [ ] 大きな出力でのメモリ使用量テスト
- [ ] 出力サイズ制限の動作確認テスト
- [ ] 並行実行時の動作確認テスト
- [ ] 長時間実行での安定性テスト

**セキュリティテストの網羅的実施**
- [ ] パストラバーサル攻撃のテスト
- [ ] シンボリックリンク攻撃のテスト
- [ ] 権限昇格攻撃のテスト
- [ ] ディスク容量枯渇攻撃のテスト

**設定検証機能の実装**
- [ ] `ConfigValidator`の実装
- [ ] GlobalConfig検証
- [ ] Command検証
- [ ] 設定ファイルの事前検証

#### 3.5.3 テスト作成と実行

**パフォーマンステスト**
- [ ] `test/performance/output_capture_test.go` - パフォーマンステスト
  - 大容量出力のテスト
  - メモリ使用量のテスト
  - 並行実行のテスト

**セキュリティテスト**
- [ ] `test/security/output_security_test.go` - 出力キャプチャ固有のセキュリティテスト
  - 出力ファイル攻撃シナリオのテスト
  - SecurityValidatorとの統合セキュリティ検証
  - サイズ制限等の出力キャプチャ固有のセキュリティ要件検証

**設定検証テスト**
- [ ] `internal/runner/output/validation_test.go` - 設定検証のテスト
  - 不正設定の検出テスト
  - エラーメッセージの確認

#### 3.5.4 実装
- [ ] パフォーマンステストの実装
- [ ] 出力キャプチャ固有のセキュリティテスト実装
- [ ] 既存SecurityValidatorとの統合セキュリティテスト
- [ ] `ConfigValidator`の実装
- [ ] 全テストが通過することを確認

#### 3.5.5 サンプルと使用例の作成

**設定ファイルサンプル**
- [ ] `sample/output_capture_basic.toml` - 基本的な使用例
- [ ] `sample/output_capture_advanced.toml` - 高度な設定例
- [ ] `sample/output_capture_security.toml` - セキュリティ重視の設定例

**実行サンプル**
- [ ] 基本的な出力キャプチャの動作確認
- [ ] Dry-Run機能の動作確認
- [ ] エラーケースの動作確認

#### 3.5.6 最終検証
- [ ] 全機能の動作確認
- [ ] 要件定義書の全項目の実装確認
- [ ] セキュリティ要件の全項目の検証
- [ ] `make test`でのテスト完全パス
- [ ] `make lint`での問題ゼロ確認
- [ ] Phase 5完了コミット

## 4. ファイル構成計画

### 4.1 新規作成ファイル

```
internal/runner/output/
├── constants.go          # 定数定義
├── errors.go            # エラー型定義
├── file.go              # SafeFileManager（safefileio活用）
├── manager.go           # DefaultOutputCaptureManager
├── path.go              # DefaultPathValidator（簡素化版）
├── security.go          # セキュリティリスク評価
├── types.go             # データ構造定義
├── validation.go        # 設定検証
└── writer.go            # TeeOutputWriter

internal/runner/security/  # 既存ディレクトリに追加
└── file_validation.go   # ValidateOutputWritePermission追加（既存ValidateDirectoryPermissionsでシンボリックリンク検証済み）

internal/runner/output/
├── constants_test.go
├── errors_test.go
├── file_test.go
├── integration_test.go
├── manager_test.go
├── path_test.go
├── security_test.go      # evaluateSecurityRisk等の出力キャプチャ固有のセキュリティ機能
├── types_test.go
├── validation_test.go
└── writer_test.go

internal/runner/security/   # 既存ディレクトリに追加
└── file_validation_test.go # ValidateOutputWritePermissionの追加テスト

test/performance/
└── output_capture_test.go

test/security/
└── output_security_test.go

sample/
├── output_capture_basic.toml
├── output_capture_advanced.toml
└── output_capture_security.toml
```

### 4.2 修正対象ファイル

```
internal/runner/config/config.go      # Command/GlobalConfig拡張
internal/runner/resource/manager.go   # NormalResourceManager拡張
internal/runner/resource/dryrun.go    # DryRunResourceManager拡張
internal/runner/executor/types.go     # ExecuteConfig拡張
internal/runner/executor/executor.go  # DefaultCommandExecutor拡張

# 対応するテストファイル
internal/runner/config/config_test.go
internal/runner/resource/manager_test.go
internal/runner/resource/dryrun_test.go
internal/runner/executor/executor_test.go
```

## 5. 依存関係と前提条件

### 5.1 外部依存関係
- `github.com/isseis/go-safe-cmd-runner/internal/groupmembership` - グループメンバーシップ確認
- `github.com/isseis/go-safe-cmd-runner/internal/safefileio` - 安全なファイル操作

### 5.2 内部依存関係
- 既存のResourceManager/Executor パターン
- 既存の設定システム（TOML）
- 既存のLogger実装
- **重要**: 既存`security.Validator.ValidateDirectoryPermissions`（包括的シンボリックリンク検証機能を提供）

### 5.3 実装順序の制約
1. Phase 1のデータ構造が完了してからPhase 2以降に進む
2. Phase 3のOutputCaptureManagerが完了してからPhase 4に進む
3. 各フェーズでテストが完全にパスしてから次のフェーズに進む

## 6. リスク管理

### 6.1 技術リスク

**R1: 既存コードとの統合問題**
- リスク: 既存のResourceManager/Executorとの統合で予期しない動作
- 対策: 段階的統合、豊富な統合テスト

**R2: パフォーマンス影響**
- リスク: 出力キャプチャによる性能劣化
- 対策: ストリーミング処理、パフォーマンステスト

**R3: メモリリーク**
- リスク: 大きな出力でのメモリ消費
- 対策: バッファサイズ制限、リソース管理

### 6.2 セキュリティリスク

**R4: パストラバーサル攻撃とシンボリックリンク攻撃**
- リスク: 設定ファイル経由での不正ファイルアクセス
- 対策:
  - パストラバーサル: PathValidatorでの".."検出
  - シンボリックリンク攻撃: 既存SecurityValidator.ValidateDirectoryPermissionsによる包括的防止
  - safefileioによるTOCTOU攻撃防止

**R5: 権限昇格**
- リスク: ファイル権限設定の不備
- 対策: safefileio標準の0600権限固定、権限分離の徹底

## 7. 品質保証計画

### 7.1 テスト戦略
- **単体テスト**: 各コンポーネントの個別テスト
- **統合テスト**: コンポーネント間の連携テスト
- **セキュリティテスト**: 攻撃シナリオのテスト
- **パフォーマンステスト**: 性能・メモリ使用量のテスト

### 7.2 コードレビュー要件
- 各フェーズ完了時にコードレビューを実施
- セキュリティ観点での重点レビュー
- エラーハンドリングの網羅性確認

### 7.3 受け入れ基準
- 全テストがパスすること
- `make lint`でエラーがないこと
- 要件定義書の全項目が実装されていること
- セキュリティテストが全て成功すること

## 8. スケジュール目安
git
### 8.1 各フェーズの目安時間

- **Phase 1** (基本構造): 2-3日
- **Phase 2** (パス処理・セキュリティ): 3-4日
- **Phase 3** (ファイル操作・Output管理): 4-5日
- **Phase 4** (ResourceManager統合): 3-4日
- **Phase 5** (最終統合・ドキュメント): 2-3日

**合計**: 14-19日

### 8.2 マイルストーン

- **Week 1 End**: Phase 1-2完了
- **Week 2 End**: Phase 3-4完了
- **Week 3 End**: Phase 5完了、機能リリース

## 9. 成功基準

### 9.1 機能要件
- [ ] outputフィールドが設定されたコマンドの標準出力がファイルに保存される
- [ ] Runner標準出力への表示は常に実行される（Tee機能）
- [ ] 絶対パス・相対パス両方で正しく動作する
- [ ] ファイル権限が0600で設定される
- [ ] パストラバーサル攻撃が防がれる
- [ ] 出力サイズ制限が機能する

### 9.2 非機能要件
- [ ] メモリ使用量が出力サイズに比例しない（ストリーミング処理）
- [ ] 大きな出力（10MB以上）でも安定動作
- [ ] エラー時の適切なクリーンアップ
- [ ] Dry-Runモードでの分析機能

### 9.3 品質要件
- [ ] テストカバレッジ90%以上
- [ ] 全セキュリティテストの成功
- [ ] 全パフォーマンステストの成功
- [ ] lintエラーゼロ

この実装計画書に従って段階的に実装を進めることで、要件を満たす安全で効率的なコマンド出力キャプチャ機能を実現できます。
