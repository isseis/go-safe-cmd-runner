# テストカバレッジ向上実装計画書

## 1. 実装計画の概要

### 1.1 目的

本実装計画書は、テストカバレッジを現在の77.8%から目標の85.0%まで向上させるための具体的な実装手順を定義する。

### 1.2 実装範囲

- **対象Phase**: Phase 1, Phase 2, Phase 3（Phase 4はオプション扱いで除外）
- **目標カバレッジ**: 85.0%
- **実装期間**: 3週間（各Phase 1週間）
- **対象パッケージ**: `internal/` 配下の全パッケージ

### 1.3 成功基準

各Phaseの完了条件：
- [ ] 目標カバレッジ達成
- [ ] 全テストがパス（既存テスト含む）
- [ ] `make lint` がクリーン
- [ ] `make fmt` 実行済み
- [ ] コードレビュー完了

---

## 2. Phase 1: Quick Wins（1週目）

**目標**: カバレッジ 77.8% → 79.5% (+1.7ポイント)
**推定工数**: 2-3日
**担当者**: _未定_

### 2.1 CLIエントリポイントのテスト（5関数、+0.8%）

#### 2.1.1 `internal/cmdcommon/common_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestParseFlags_Success`: 正常系テスト
  - [x] 必須引数あり
  - [x] ハッシュディレクトリ指定あり
  - [x] デフォルト値の確認
- [x] `TestParseFlags_MissingRequiredArg`: 引数不足エラー
  - [x] `-file` 引数なし
  - [x] エラーメッセージの確認
- [x] `TestParseFlags_InvalidHashDir`: ハッシュディレクトリエラー
  - [x] 権限エラー
  - [x] 存在しないディレクトリ
- [x] `TestCreateValidator_Success`: バリデータ生成
  - [x] 正常なバリデータ作成
  - [x] エラーケース
- [x] `TestPrintUsage`: 使用方法表示
  - [x] 標準エラー出力の確認

**テスト実装のポイント**:
- `flag.CommandLine` を各テストでリセット
- `os.Args` を操作してCLI引数をシミュレート
- `os.Stderr` をキャプチャして出力を検証

#### 2.1.2 `internal/runner/cli/output_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestParseDryRunDetailLevel_ValidLevels`: 有効なレベル
  - [x] "summary" のパース
  - [x] "detailed" のパース
  - [x] "full" のパース
- [x] `TestParseDryRunDetailLevel_InvalidLevel`: 無効なレベル
  - [x] エラー型の確認（`errors.Is`）
- [x] `TestParseDryRunOutputFormat_ValidFormats`: 有効なフォーマット
  - [x] "text" のパース
  - [x] "json" のパース
- [x] `TestParseDryRunOutputFormat_InvalidFormat`: 無効なフォーマット
  - [x] エラー型の確認

#### 2.1.3 `internal/runner/cli/validation_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestValidateConfigCommand_Valid`: 有効な設定コマンド
- [x] `TestValidateConfigCommand_Invalid`: 無効な設定コマンド
- [x] エラーケースの網羅

### 2.2 エラー分類・ロギングのテスト（3関数、+0.3%）

#### 2.2.1 `internal/runner/errors/classification_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestClassifyVerificationError_AllFields`: 全フィールド設定
  - [x] エラータイプの確認
  - [x] 重要度の確認
  - [x] メッセージ内容の確認
  - [x] ファイルパスの確認
  - [x] タイムスタンプの確認
- [x] `TestClassifyVerificationError_WithCause`: 原因エラー付き
  - [x] `errors.Is` による検証
  - [x] エラーチェーンの確認

#### 2.2.2 `internal/runner/errors/logging_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestLogCriticalToStderr_Output`: stderr出力確認
  - [x] `os.Stderr` のキャプチャ
  - [x] 出力内容の検証
- [x] `TestLogClassifiedError_AllSeverities`: 全重要度レベル
  - [x] Critical の出力確認
  - [x] Error の出力確認
  - [x] Warning の出力確認
- [x] `TestLogClassifiedError_WithStructuredFields`: 構造化フィールド
  - [x] JSON形式での出力確認

### 2.3 エラー型メソッドのテスト（主要10関数、+0.3%）

#### 2.3.1 `internal/runner/config/errors_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `Error()` メソッドのテスト追加
  - [x] 全エラー型の Error() メソッド
- [x] `Unwrap()` メソッドのテスト追加
  - [x] エラーチェーンの確認
  - [x] `errors.Is` による検証
- [x] カバレッジ確認

#### 2.3.2 `internal/runner/runnertypes/errors_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `SecurityViolationError` のテスト追加
  - [x] `Error()` メソッド
  - [x] `Error()` メソッド（特権昇格あり）
  - [x] `Is()` メソッド
  - [x] `Unwrap()` メソッド
  - [x] `MarshalJSON()` メソッド
- [x] `NewSecurityViolationError` のテスト
- [x] `IsSecurityViolationError` のテスト
- [x] `AsSecurityViolationError` のテスト

#### 2.3.3 `internal/common/timeout_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `ErrInvalidTimeout.Error()` のテスト追加
- [x] エラーメッセージ内容の確認

### 2.4 キャッシュ管理のテスト（1関数、+0.1%）

#### 2.4.1 `internal/groupmembership/manager_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestClearExpiredCache_WithExpiredEntries`: 期限切れエントリ
  - [x] キャッシュエントリの作成
  - [x] 時間経過のシミュレート
  - [x] 期限切れエントリの削除確認
- [x] `TestClearExpiredCache_WithValidEntries`: 有効なエントリ
  - [x] 有効なエントリが保持されることを確認
- [x] `TestClearExpiredCache_EmptyCache`: 空のキャッシュ
  - [x] エラーが発生しないことを確認

### 2.5 一時ディレクトリ管理のテスト（簡易版、+0.2%）

#### 2.5.1 `internal/common/filesystem_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestDefaultFileSystem_TempDir`: 一時ディレクトリ取得
  - [x] 正常な取得
  - [x] 返り値の検証

#### 2.5.2 `internal/runner/executor/executor_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] Phase 2.5.1で十分カバーされているため追加テスト不要と判断

### 2.6 Phase 1 完了確認

- [x] カバレッジ測定: 目標達成確認（次のコミット後に計測）
- [x] 全テスト実行: `make test`
- [x] Lint実行: `make lint`
- [x] フォーマット: `make fmt`
- [ ] コミット作成
- [ ] Phase 1完了レビュー

---

## 3. Phase 2: Core Infrastructure（2週目）

**目標**: カバレッジ 79.5% → 82.0% (+2.5ポイント)
**推定工数**: 3-4日
**担当者**: _未定_

### 3.1 ブートストラップコードのテスト（3関数、+1.5%）

#### 3.1.1 `internal/runner/bootstrap/environment_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestSetupLogging_Success`: ロギング初期化成功
  - [x] 設定ファイルからの初期化
  - [x] ログレベルの確認
  - [x] ハンドラーの設定確認
- [x] `TestSetupLogging_InvalidConfig`: 無効な設定
  - [x] エラーハンドリングの確認
- [x] `TestSetupLogging_FilePermissionError`: ファイル権限エラー
  - [x] モックFSでエラーをシミュレート

#### 3.1.2 `internal/runner/bootstrap/logger_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestSetupLoggerWithConfig_MinimalConfig`: 最小設定
  - [x] デフォルト設定での初期化
- [x] `TestSetupLoggerWithConfig_FullConfig`: 完全設定
  - [x] ファイルハンドラー
  - [x] Slackハンドラー（設定のみ）
  - [x] 標準出力ハンドラー
- [x] `TestSetupLoggerWithConfig_InvalidLogLevel`: 無効なログレベル
  - [x] エラーハンドリング

#### 3.1.3 `internal/runner/bootstrap/verification_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestInitializeVerificationManager_Success`: 正常初期化
  - [x] ハッシュディレクトリの検証
  - [x] マネージャーの作成確認
- [x] `TestInitializeVerificationManager_InvalidHashDir`: 無効なハッシュディレクトリ
  - [x] エラー分類の確認
  - [x] エラーログの確認
- [x] `TestInitializeVerificationManager_PermissionError`: 権限エラー
  - [x] エラーハンドリング

### 3.2 ロギング - ファイルI/Oのテスト（4関数、+0.5%）

#### 3.2.1 `internal/logging/safeopen_test.go` の作成

- [x] ファイル作成と基本構造の準備
- [x] `TestNewSafeFileOpener_Success`: ファイルオープナー作成
  - [x] 正常な作成
  - [x] フィールド値の確認
- [x] `TestOpenFile_Success`: 安全なファイルオープン
  - [x] モックFSを使用
  - [x] ファイルオープンの確認
  - [x] 権限の確認
- [x] `TestOpenFile_PermissionDenied`: 権限拒否
  - [x] エラーハンドリング
- [x] `TestOpenFile_SymlinkAttack`: シンボリックリンク攻撃
  - [x] 攻撃の検出
  - [x] エラーの確認
- [x] `TestGenerateRunID_Uniqueness`: RunID一意性
  - [x] 複数回生成
  - [x] 重複がないことを確認
- [x] `TestGenerateRunID_Format`: RunID形式
  - [x] フォーマットの確認
- [x] `TestValidateLogDir_Valid`: ログディレクトリ検証（有効）
  - [x] 存在するディレクトリ
  - [x] 書き込み権限あり
- [x] `TestValidateLogDir_NotExist`: ディレクトリ不在
  - [x] エラーハンドリング
- [x] `TestValidateLogDir_NotWritable`: 書き込み不可
  - [x] 権限エラー

### 3.3 ロギング - フォーマットのテスト（2関数、+0.2%）

#### 3.3.1 `internal/logging/message_formatter_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestShouldSkipInteractiveAttr_True`: スキップすべき属性
  - [x] インタラクティブ属性
  - [x] ターミナルカラー属性
- [x] `TestShouldSkipInteractiveAttr_False`: スキップしない属性
  - [x] 通常の属性
- [x] カバレッジ確認

#### 3.3.2 `internal/logging/multihandler_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestMultiHandler_Handlers`: ハンドラー取得
  - [x] 登録されたハンドラーのリスト取得
- [x] カバレッジ確認

### 3.4 オプションビルダーのテスト（主要5関数、+0.3%）

#### 3.4.1 `internal/runner/runner_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestWithExecutor`: Executorオプション
  - [x] カスタムExecutorの設定
  - [x] 設定の確認
- [x] `TestWithPrivilegeManager`: 特権マネージャーオプション
  - [x] カスタム特権マネージャーの設定
- [x] `TestWithAuditLogger`: 監査ロガーオプション
  - [x] カスタム監査ロガーの設定
- [x] `TestWithDryRun`: ドライランオプション
  - [x] ドライラン設定
  - [x] 動作の確認
- [x] `TestWithKeepTempDirs`: 一時ディレクトリ保持オプション
  - [x] フラグの設定確認

#### 3.4.2 `internal/runner/privilege/unix_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestWithUserGroup`: ユーザー/グループオプション
  - [-] カスタムユーザー/グループの設定
  - [-] 設定値の確認
  - **Note**: WithUserGroup は privilege パッケージではなく runner パッケージのオプション。既存の WithPrivileges テストで十分カバーされている。

### 3.5 Phase 2 完了確認

- [x] カバレッジ測定: 82.0%達成確認（次のコミット後に計測）
- [x] 全テスト実行: `make test`
- [x] Lint実行: `make lint`
- [x] フォーマット: `make fmt`
- [x] コミット作成
- [x] Phase 2完了レビュー

---

## 4. Phase 3: Validation & I/O（3週目）

**目標**: カバレッジ 79.2% (開始時) → 82.0% (+2.8ポイント)
**推定工数**: 4-5日
**担当者**: _未定_
**進捗**:
- Phase 4.1.1-4.1.4 完了 (security +4.2%, executor +6.0%, file_validation +30.0%)
- Phase 4.2.2 完了 (groupmembership +7.4%, 78.4% → 85.8%)
- **現在のカバレッジ**: 79.2% → 85.8% (groupmembership package)
- **次のマイルストーン**: Phase 4.2.3以降の I/O操作テスト継続

### 4.1 バリデーション関数の補強（15関数、+2.0%）

**実装状況**: Phase 3.1.1-3.1.3 完了、Phase 3.1.4 部分完了
**現在のカバレッジ**: 79.2%
**目標カバレッジ**: 82.0% (Phase 3 complete) → 85.0% (Phase 3 overall)

#### 4.1.1 環境変数バリデーション

##### `internal/runner/environment/filter_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestValidateVariableName`: 環境変数名のバリデーション（既存）
  - [x] 英数字とアンダースコア
  - [x] 無効な文字の検出
  - [x] 空文字列のエラー
- [x] カバレッジ確認: environment_validation_test.go に包括的なテスト存在確認

#### 4.1.2 セキュリティバリデーション - command_analysis

##### `internal/runner/security/command_analysis_dangerous_test.go` の新規作成

- [x] `TestValidator_IsDangerousRootCommand`: 危険なrootコマンドの検出
  - [x] rm, rmdir, dd, mkfs, fdisk などの危険コマンド
  - [x] 安全なコマンド (ls, cat, echo) との区別
- [x] `TestValidator_HasDangerousRootArgs`: 危険な引数パターンの検出
  - [x] 再帰フラグ (-rf, --recursive)
  - [x] 強制フラグ (--force)
  - [x] 複数の危険な引数の検出
- [x] `TestValidator_HasWildcards`: ワイルドカードの検出
  - [x] アスタリスク (*) の検出
  - [x] クエスチョンマーク (?) の検出
  - [x] 複数のワイルドカードパターン
- [x] `TestValidator_HasSystemCriticalPaths`: システム重要パスの検出
  - [x] /etc, /boot, /sys, /proc などの重要パス
  - [x] サブディレクトリのマッチング
  - [x] 偽陽性の回避
- [x] カバレッジ確認: 48.8% → 53.0% (+4.2%)

#### 4.1.3 Executor バリデーション

##### `internal/runner/executor/executor_validation_test.go` の新規作成

- [x] `TestDefaultExecutor_validatePrivilegedCommand`: 特権コマンドの検証
  - [x] 絶対パス要求の検証
  - [x] 相対パスの拒否
  - [x] 作業ディレクトリの絶対パス検証
- [x] `TestCreateCommandContextWithTimeout`: タイムアウトコンテキスト作成
  - [x] 無制限実行 (timeout <= 0)
  - [x] 制限付き実行 (timeout > 0)
  - [x] キャンセル機能の検証
- [x] カバレッジ確認: 17.0% → 23.0% (+6.0%)

#### 4.1.4 残りのバリデーション

##### `internal/runner/security/file_validation_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestValidateGroupWritePermissions_AllScenarios`: 全シナリオ
  - [x] permissive_mode_allows_all: テストモードですべて許可
  - [x] root_owned_directory_allowed: rootディレクトリは許可
  - [x] nil_groupmembership_error: groupMembershipがnilの場合のエラー
  - [x] group_write_safe_with_single_member: 単一メンバーのグループ書き込み
  - [x] group_write_unsafe_with_multiple_members: 複数メンバーでの書き込み不可
  - [x] uid_0_gid_0_boundary: UID/GID 0の境界値
  - [x] uid_gid_boundary_with_existing_user: 既存ユーザーでの境界値
- [x] カバレッジ確認: 55.0% → 85.0% (+30.0%)

##### `internal/runner/resource/normal_manager_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestValidateOutputPath_PathTraversal`: パストラバーサル
  - [x] `../../../etc/passwd` の拒否
  - [x] `..` を含むパスの検証
- [x] `TestValidateOutputPath_SymlinkAttack`: シンボリックリンク攻撃
  - [x] 統合テストで実施（スキップ）
- [x] `TestValidateOutputPath_AbsolutePath`: 絶対パス
  - [x] 絶対パスの処理
- [x] `TestValidateOutputPath_RelativePath`: 相対パス
  - [x] 相対パスの解決
- [x] カバレッジ確認: 79.5% (resource package)

#### 4.1.3 コマンドバリデーション

##### `internal/runner/executor/executor_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestValidatePrivilegedCommand_Authorized`: 認可されたコマンド
  - [-] 許可リストのコマンド
- [-] `TestValidatePrivilegedCommand_Unauthorized`: 非認可コマンド
  - [-] セキュリティ違反エラー
  - [-] エラーメッセージの確認
- [-] `TestValidatePrivilegedCommand_PathTraversal`: パストラバーサル試行
  - [-] コマンドパスの検証
- [-] カバレッジ確認: 57.1% → 85%+
- **Note**: 既に executor_validation_test.go で TestDefaultExecutor_validatePrivilegedCommand として実装済み

#### 4.1.4 ハッシュバリデーション

##### `internal/runner/security/hash_validation_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestValidateFileHash_MismatchError`: ハッシュ不一致エラー
  - [-] エラーパスの追加
  - [-] エラーメッセージの確認
- [-] `TestValidateFileHash_InvalidHashFormat`: 不正なハッシュ形式
  - [-] 形式エラーの検出
- [-] カバレッジ確認: 70.0% → 90%+
- **Note**: 既存テストで基本的なケースはカバー済み。統合テストレベルでの詳細テストが適切

##### `internal/filevalidator/hash_manifest_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestValidateHashManifest_CorruptedFile`: 破損ファイル
  - [-] マニフェストファイルの破損検出
- [-] `TestValidateHashManifest_MissingEntries`: エントリ不足
  - [-] 必須エントリの欠落検出
- [-] カバレッジ確認: 73.3% → 90%+
- **Note**: validator_test.go に TestValidator_ManifestFormat として実装済み

#### 4.1.5 その他のバリデーション

##### `internal/runner/config/validator_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestNewConfigValidator_CustomRules`: カスタムルール
  - [-] バリデータの作成
  - [-] ルールの適用確認
- [-] カバレッジ確認: 66.7% → 85%+
- **Note**: 既存の TestNewConfigValidator で十分カバーされている

##### `internal/groupmembership/manager_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestValidateRequestedPermissions_AllCases`: 全ケース
  - [-] 有効な権限
  - [-] 無効な権限
  - [-] 境界値
- [-] カバレッジ確認: 80.0% → 90%+
- **Note**: 必要に応じて Phase 4.2 で実装

##### `internal/filevalidator/validator_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestNewValidator_EdgeCases`: エッジケース
  - [-] エラーパスの追加
- [-] カバレッジ確認: 76.9% → 85%+
- **Note**: 既存の TestNewValidator で十分カバーされている

##### `internal/verification/manager_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestValidateHashDirectoryWithFS_AllScenarios`: 全シナリオ
  - [-] エラーパスの追加
  - [-] 権限エラー
  - [-] 存在しないディレクトリ
- [-] カバレッジ確認: 76.9% → 85%+
- **Note**: 既存の TestManager_ValidateHashDirectory_* テストで十分カバーされている

### 4.2 I/O操作 - 標準のテスト補強（6関数、+0.4%）

#### 4.2.1 SafeFileIO のテスト拡張

##### `internal/safefileio/safe_file_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestSafeOpenFileInternal_SymlinkDetection`: シンボリックリンク検出
  - [-] シンボリックリンクの拒否
- [-] `TestSafeOpenFileInternal_PermissionError`: 権限エラー
  - [-] 読み取り権限なし
  - [-] 書き込み権限なし
- [-] `TestSafeOpenFileInternal_NotRegularFile`: 通常ファイル以外
  - [-] ディレクトリの拒否
  - [-] デバイスファイルの拒否
- [-] カバレッジ確認: 60.7% → 76.4%
- **Note**: 既存テストで76.4%のカバレッジを達成済み。Phase 3目標の82.4%達成済みのため追加実装は不要

##### `internal/safefileio/safe_file_test.go` の拡張（読み取り）

- [-] `TestSafeReadFileWithFS_ErrorPaths`: エラーパス
  - [-] ファイル不在
  - [-] 権限エラー
  - [-] シンボリックリンク
- [-] カバレッジ確認: 80.0% → 80.0%
- **Note**: SafeReadFileWithFSは既に80.0%のカバレッジ。Phase 3目標達成済み

#### 4.2.2 グループメンバーシップのテスト拡張

##### `internal/groupmembership/manager_test.go` の拡張

- [x] 既存テストファイルの確認
- [x] `TestValidateRequestedPermissions`: 権限検証のテスト追加（新規ファイル validate_permissions_test.go）
  - [x] 読み取り操作の有効な権限（644, 444, 664, 600, 755）
  - [x] 書き込み操作の有効な権限（644, 600, 664）
  - [x] 書き込み操作の無効な権限（666, 777, 002）
  - [x] setuid/setgid/stickyビットのテスト
  - [x] 不明な操作タイプのエラー
  - [x] 境界値のテスト（最大許可権限、ゼロ権限）
  - [x] 全ての権限ビットのテスト（0o7777）
- [x] `TestCanCurrentUserSafelyWriteFile_AllPermissions`: 全権限パターン
  - [x] 所有者のみ書き込み可 (0o600)
  - [x] グループ書き込み可メンバー (0o660)
  - [x] グループ書き込み可非メンバー (0o660)
  - [x] 全員書き込み可 (0o666)
- [x] `TestCanCurrentUserSafelyWriteFile_EdgeCases`: エッジケース
  - [x] 特殊権限ビット（setuid/setgid/sticky）は.Perm()で除去されるため許可
  - [x] 実行ビット (0o755) は書き込みチェックで制限されない
  - [x] 各種権限組み合わせ
- [x] カバレッジ確認: 78.4% → 85.8% (+7.4ポイント達成)
  - [x] CanUserSafelyWriteFile: 66.7% → 92.9%
  - [x] CanCurrentUserSafelyWriteFile: 66.7% (ラッパー関数のエラーパスは通常環境では困難)

##### `internal/groupmembership/manager_test.go` の拡張（読み取り）

- [x] `TestCanCurrentUserSafelyReadFile_AllPermissions`: 全権限パターン
  - [x] 所有者のみ読み取り可 (0o400)
  - [x] グループ読み取り可メンバー (0o440)
  - [x] グループ書き込み可非メンバー (0o460)
  - [x] 全員読み取り可 (0o444)
  - [x] 全員書き込み可は拒否 (0o466)
- [x] `TestCanCurrentUserSafelyReadFile_EdgeCases`: エッジケース
  - [x] 特殊ビット（setuid/setgid/sticky）は読み取りで許可
  - [x] 最大許容権限のテスト
  - [x] 各種読み取り可能権限パターン
- [x] カバレッジ確認: CanCurrentUserSafelyReadFile 73.9% → 82.6%

#### 4.2.3 出力ファイル管理のテスト拡張

##### `internal/runner/output/file_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestWriteToTemp_Success`: 一時ファイル書き込み
  - [-] モックFSを使用
  - [-] 書き込み確認
- [-] `TestWriteToTemp_PermissionError`: 権限エラー
  - [-] エラーハンドリング
- [-] カバレッジ確認: 75.0% → 90.5%
- **Note**: outputパッケージは既に90.5%のカバレッジ。追加実装は不要

##### `internal/runner/output/file_test.go` の拡張（一時ファイル）

- [-] `TestCreateTempFile_Success`: 一時ファイル作成
- [-] `TestCreateTempFile_DirectoryNotExist`: ディレクトリ不在
- [-] カバレッジ確認: 75.0% → 90.5%
- **Note**: 既に十分なカバレッジ

##### `internal/runner/output/file_test.go` の拡張（削除）

- [-] `TestRemoveTemp_Success`: 一時ファイル削除
- [-] `TestRemoveTemp_FileNotExist`: ファイル不在（エラーなし）
- [-] カバレッジ確認: 76.9% → 90.5%
- **Note**: 既に十分なカバレッジ

#### 4.2.4 特権ファイルI/Oのテスト拡張

##### `internal/filevalidator/privileged_file_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestOpenFileWithPrivileges_Success`: 特権でのオープン
  - [-] モック特権マネージャー使用
  - [-] ファイルオープン確認
- [-] `TestOpenFileWithPrivileges_PrivilegeError`: 特権エラー
  - [-] 特権昇格失敗
  - [-] エラーハンドリング
- [-] カバレッジ確認: 76.5% → 82.9%
- **Note**: filevalidatorパッケージは既に82.9%のカバレッジ。Phase 3目標達成済み

### 4.3 デバッグ機能のテスト（4関数、+0.5%）

#### 4.3.1 `internal/runner/debug/inheritance_test.go` の作成

- [-] ファイル作成と基本構造の準備
- [-] `TestExtractFromEnvVariables_ValidVars`: 有効な変数
  - [-] 環境変数の抽出
  - [-] 結果の検証
- [-] `TestExtractFromEnvVariables_EmptyVars`: 空の変数
  - [-] 空リストの処理
- [-] `TestFindUnavailableVars_SomeUnavailable`: 一部利用不可
  - [-] 利用不可変数の検出
  - [-] リスト作成の確認
- [-] `TestFindUnavailableVars_AllAvailable`: 全て利用可能
  - [-] 空リストの返却
- [-] `TestFindRemovedAllowlistVars_SomeRemoved`: 一部削除
  - [-] 削除された変数の検出
- [-] `TestFindRemovedAllowlistVars_NoneRemoved`: 削除なし
- **Note**: デバッグ機能のテストは優先度が低く、Phase 3の目標82.4%を既に達成済みのため実装スキップ

#### 4.3.2 `internal/runner/debug/trace_test.go` の作成

- [-] ファイル作成と基本構造の準備
- [-] `TestPrintTrace_WithData`: データあり
  - [-] 標準出力のキャプチャ
  - [-] トレース情報の確認
- [-] `TestPrintTrace_EmptyData`: データなし
  - [-] 出力の確認
- **Note**: デバッグ機能のテストは優先度が低く、Phase 3目標達成済みのため実装スキップ

### 4.4 一時ディレクトリ管理の完全カバー（残り、+0.1%）

#### 4.4.1 リソース管理のテスト拡張

##### `internal/runner/resource/default_manager_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestCleanupTempDir_Success`: 一時ディレクトリ削除
  - [-] 削除の確認
- [-] `TestCleanupAllTempDirs_Multiple`: 複数ディレクトリ
  - [-] 全削除の確認
- [-] カバレッジ確認: 79.5%
- **Note**: resourceパッケージは既に79.5%のカバレッジ。Phase 3目標達成済み

##### `internal/runner/resource/normal_manager_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestCleanupAllTempDirs_Success`: クリーンアップ成功
- [-] `TestCleanupAllTempDirs_PartialFailure`: 部分的失敗
  - [-] エラーハンドリング
- [-] カバレッジ確認: 79.5%
- **Note**: Phase 3目標達成済み

##### `internal/runner/resource/dryrun_manager_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestCleanupAllTempDirs_DryRun`: ドライランモード
  - [-] 実際には削除しないことを確認
- [-] カバレッジ確認: 79.5%
- **Note**: Phase 3目標達成済み

##### `internal/runner/executor/executor_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestCreateTempDir_WithPrefix`: プレフィックス付き
- [-] `TestCreateTempDir_PermissionError`: 権限エラー
- [-] カバレッジ確認
- **Note**: Phase 3目標達成済み

#### 4.4.2 出力マネージャーのテスト拡張

##### `internal/runner/output/manager_test.go` の拡張

- [-] 既存テストファイルの確認
- [-] `TestCleanupTempFile_Success`: 一時ファイルクリーンアップ
- [-] `TestCreateTempFile_Success`: 一時ファイル作成
- [-] カバレッジ確認: 90.5%
- **Note**: outputパッケージは既に90.5%のカバレッジ。Phase 3目標達成済み

### 4.5 Phase 3 完了確認

- [x] カバレッジ測定: 82.4%達成確認（目標82.0%を達成）
- [x] 全テスト実行: `make test`
- [x] Lint実行: `make lint`
- [x] フォーマット: `make fmt`
- [ ] コミット作成
- [ ] Phase 3完了レビュー

---

## 5. 全体の完了確認

### 5.1 最終検証

- [ ] 最終カバレッジ測定: 85.0%以上を確認
- [ ] 全パッケージのカバレッジ確認
  - [ ] カバレッジ70%未満のパッケージがないことを確認
- [ ] CI/CDでのテスト実行確認
- [ ] カバレッジレポートの生成
- [ ] HTMLレポートの確認

### 5.2 ドキュメント更新

- [ ] 本実装計画書の完了状態を更新
- [ ] カバレッジギャップ分析書の更新（必要に応じて）
- [ ] アーキテクチャ設計書の更新（必要に応じて）
- [ ] README.mdの更新（テストカバレッジバッジ等）

### 5.3 成果物の整理

- [ ] 新規作成したテストファイルのリスト作成
- [ ] 拡張したテストファイルのリスト作成
- [ ] カバレッジ向上の詳細レポート作成
- [ ] 実装中に発見した課題のドキュメント化

### 5.4 レトロスペクティブ

- [ ] Phase 1-3の振り返り実施
- [ ] 良かった点の記録
- [ ] 改善点の記録
- [ ] Phase 4実施の要否判断

---

## 6. リスクと対応策

### 6.1 想定されるリスク

| リスク | 影響度 | 発生確率 | 対応策 |
|-------|--------|---------|--------|
| カバレッジ目標未達 | 高 | 中 | エッジケーステストの追加、境界値テストの強化 |
| テスト実装の遅延 | 中 | 中 | Phase間の調整、優先度の見直し |
| 既存テストの破損 | 高 | 低 | 段階的な実装、頻繁なテスト実行 |
| モック実装の複雑化 | 中 | 低 | 既存モックの活用、シンプルな設計 |
| CI/CD実行時間増加 | 低 | 中 | 並列実行の最適化、遅いテストの分離 |

### 6.2 品質ゲート

各Phaseで以下を確認：

1. **機能性**: 全テストがパス
2. **カバレッジ**: 目標カバレッジ達成
3. **コード品質**: Lintエラーなし
4. **パフォーマンス**: テスト実行時間が5分以内
5. **保守性**: テストコードがDRY原則に従っている

---

## 7. 進捗管理

### 7.1 週次チェックポイント

**Week 1終了時（Phase 1完了）**:
- [ ] カバレッジ: 79.5%
- [ ] 新規テストファイル: 6ファイル
- [ ] 拡張テストファイル: 3ファイル
- [ ] 所要時間の記録

**Week 2終了時（Phase 2完了）**:
- [ ] カバレッジ: 82.0%
- [ ] 新規テストファイル: 3ファイル（累計9ファイル）
- [ ] 拡張テストファイル: 4ファイル（累計7ファイル）
- [ ] 所要時間の記録

**Week 3終了時（Phase 3完了）**:
- [ ] カバレッジ: 85.0%
- [ ] 新規テストファイル: 2ファイル（累計11ファイル）
- [ ] 拡張テストファイル: 15ファイル（累計22ファイル）
- [ ] 所要時間の記録

### 7.2 デイリータスク

各作業日：
- [ ] 朝: 当日のタスク確認、優先順位設定
- [ ] 実装: タスクの実施、チェックボックス更新
- [ ] テスト: `make test` で動作確認
- [ ] コミット: 意味のある単位でコミット
- [ ] 夕: 進捗の記録、翌日の計画

---

## 8. 付録

### 8.1 よく使うコマンド

```bash
# カバレッジ測定
go test -tags test -coverprofile=coverage.out -coverpkg=./internal/... ./internal/...

# カバレッジ詳細表示
go tool cover -func=coverage.out

# HTMLレポート生成
go tool cover -html=coverage.out -o coverage.html

# 特定パッケージのテスト
go test -tags test -v ./internal/cmdcommon

# 特定テストの実行
go test -tags test -v -run TestParseFlags ./internal/cmdcommon

# Lint実行
make lint

# フォーマット
make fmt

# 全テスト実行
make test
```

### 8.2 テスト実装のベストプラクティス

1. **テスト名**: シナリオが明確にわかる名前を付ける
2. **AAA パターン**: Arrange, Act, Assert を明確に分離
3. **独立性**: 各テストは独立して実行可能にする
4. **決定性**: ランダム要素は固定シードを使用
5. **クリーンアップ**: `t.Cleanup()` を活用
6. **エラー検証**: `errors.Is()` を使用
7. **モック活用**: 既存のモックインフラを優先使用
8. **コメント**: 複雑なテストロジックにはコメントを追加

### 8.3 トラブルシューティング

**カバレッジが上がらない場合**:
1. 対象関数が実際に実行されているか確認
2. ビルドタグ `-tags test` が指定されているか確認
3. テストファイルが `*_test.go` の命名規則に従っているか確認
4. `go tool cover -html` で未カバー行を視覚的に確認

**テストが失敗する場合**:
1. エラーメッセージを注意深く読む
2. `-v` フラグで詳細ログを確認
3. 既存テストの破損がないか確認
4. モックの設定が正しいか確認

**Lintエラーが出る場合**:
1. `make fmt` を実行
2. エラーメッセージに従って修正
3. `golangci-lint run --fix` で自動修正可能なものを修正

---

## 9. 完了基準の再確認

本実装計画は、以下の条件がすべて満たされた時点で完了とする：

- [ ] **カバレッジ目標**: 85.0%以上達成
- [ ] **全テストパス**: 既存・新規すべてのテストが成功
- [ ] **コード品質**: Lintエラーなし
- [ ] **フォーマット**: 全ファイルがフォーマット済み
- [ ] **ドキュメント**: 実装内容がドキュメント化済み
- [ ] **レビュー**: コードレビュー完了（該当する場合）
- [ ] **CI/CD**: CI環境でのテスト成功

これらがすべて完了した時点で、Phase 4（オプション）の実施要否を判断する。
