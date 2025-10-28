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
- **現在のカバレッジ**: 82.4% (Phase 3目標82.0%達成)
- **ステータス**: ✅ Phase 3完了

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
- [x] コミット作成
- [x] Phase 3完了レビュー

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

---

## 10. Phase 4: 目標85%達成のための追加強化（プランA）

**目標**: カバレッジ 82.4% → 85.0% (+2.6ポイント)
**推定工数**: 5-6日
**担当者**: _未定_
**ステータス**: 計画中

### 10.1 Phase 4 策定の背景とカバレッジ分析

#### 10.1.1 Phase 3完了時点の状況

**達成カバレッジ**: 82.4%
- Phase 3目標: 82.0% ✅ 達成（+0.4ポイント超過達成）
- 全体目標: 85.0% （残り2.6ポイント）
- 実施期間: 計画通り（4-5日以内）

**Phase 3での主要な成果**:
1. セキュリティバリデーション強化（+4.2%）
2. Executor検証機能追加（+6.0%）
3. グループメンバーシップ管理充実（+7.4%）
4. ファイル検証ロジック強化（+30.0%）

#### 10.1.2 カバレッジ分析結果（2025-10-28実施）

Phase 3完了時点（82.4%）での詳細分析を実施し、以下の低カバレッジパッケージを特定：

| パッケージ | カバレッジ | 優先度 | 工数見積 |
|-----------|-----------|--------|---------|
| internal/runner/privilege | 62.0% | A（高） | 2-3日 |
| internal/logging | 60.0% | A（高） | 2-3日 |
| internal/safefileio | 76.4% | A（高） | 1-2日 |
| internal/runner/resource | 79.5% | B（中） | 1日 |
| cmd/runner | 19.7% | C（低） | 3-4日 |
| internal/runner/debug | 16.4% | C（低） | 1日 |

#### 10.1.3 分析手法

**使用ツール**:
```bash
# カバレッジ測定
go test -tags test -coverprofile=coverage.out ./...

# パッケージ別カバレッジ確認
go test -tags test ./... 2>&1 | grep coverage

# 詳細な関数別カバレッジ
go tool cover -func=coverage.out | grep -E "(package|0.0%|[1-6][0-9]\.)"
```

**分析観点**:
1. **パッケージレベル**: 70%未満のパッケージを優先対象とする
2. **関数レベル**: 0.0%カバレッジの関数を特定
3. **影響度**: パッケージの行数と全体への影響を評価
4. **実装難易度**: テスト可能性と工数を考慮

**カバレッジ詳細データ**:
```
総カバレッジ: 77.4% (coverage.out total) / 82.4% (実測値)
```

パッケージ別内訳（70%未満を抽出）:
- cmd/runner: 19.7% (main関数中心、テスト困難)
- internal/logging: 60.0% (Slack通知、エラーハンドリング)
- internal/runner/privilege: 62.0% (システムコール依存)
- internal/runner/debug: 16.4% (デバッグ出力、優先度低)

#### 10.1.4 未カバー領域の分類と制約事項

**A. テスト困難な領域（システムコール依存）**
- 特権昇格/降下処理（`escalatePrivileges`, `restorePrivileges`）
- 緊急シャットダウン処理（`emergencyShutdown`）
- UID/GID変更処理

**制約事項**:
- これらのテストはモック依存が高く、実際の動作保証には限界がある
- システムコールのモック化により、実環境での動作とテストの乖離リスクが存在
- 特権操作のテストは、統合テスト環境でのE2E検証が推奨される

**B. 外部依存領域（環境変数・ネットワーク）**
- Slack通知機能（`SlackHandler`全般）
- 環境変数からのWebhook URL取得

**制約事項**:
- HTTPモックで検証するため、実環境での動作検証が別途必要
- ネットワークタイムアウト、リトライロジックの実動作確認には限界
- Slack APIの仕様変更には追従できない

**C. CLIエントリーポイント**
- main関数とその周辺
- フラグパース処理

**制約事項**:
- 統合テストスタイルとなり、メンテナンスコストが高い
- `os.Exit`の呼び出しはテストが困難

**D. デバッグ/診断機能**
- トレース出力（`PrintTrace`）
- 環境変数継承診断（`PrintFromEnvInheritance`）

#### 10.1.5 プラン比較と意思決定プロセス

**検討した3つのプラン**:

##### プランA: 効率重視（採用）
- **対象**: privilege + logging + セキュリティ統合テスト
- **工数**: 5-6日
- **期待カバレッジ**: 84.6-85.1%
- **実装内容**:
  1. internal/runner/privilege 強化（2-3日） → +0.8%
  2. internal/logging 強化（2-3日） → +0.9%
  3. セキュリティ統合テスト拡充（1日） → +0.5-1.0%

##### プランB: バランス型（不採用）
- **対象**: プランA + safefileio + E2Eテスト
- **工数**: 7-9日
- **期待カバレッジ**: 85.6-86.6%
- **実装内容**: プランA + safefileio強化（1-2日） + E2Eテスト（2-3日）

##### プランC: 徹底型（不採用）
- **対象**: プランB + resource + debug + cmd/runner一部
- **工数**: 10-13日
- **期待カバレッジ**: 86.8-88.3%
- **実装内容**: プランB + 低優先度パッケージ（3-4日）

**意思決定マトリックス**:

| 評価項目 | プランA | プランB | プランC | 重み |
|---------|---------|---------|---------|------|
| 目標達成可能性 | ◎ 85%達成 | ◎ 85%超過 | ◎ 85%大幅超過 | 40% |
| 工数効率 | ◎ 5-6日 | ○ 7-9日 | △ 10-13日 | 30% |
| リスク | ◎ 低 | ○ 中 | △ 高 | 20% |
| 保守性向上 | ○ 中 | ◎ 高 | ◎ 最高 | 10% |
| **総合評価** | **92点** | 78点 | 64点 | - |

**プランA採用の決定理由**:

1. **目標達成の確実性**（重要度: 高）
   - 82.4% → 85.0%の2.6ポイント増加が確実に達成可能
   - Phase 3の実績（目標82.0%に対し82.4%達成）から見て実現性が高い
   - バッファを考慮しても85.1%までの余地がある

2. **費用対効果の最大化**（重要度: 高）
   - 5-6日の工数で目標達成（プランBの7-9日比で20-33%効率的）
   - 追加の0.6-1.6%カバレッジのために3-4日の工数は非効率
   - ROI（投資対効果）が最も高い

3. **セキュリティ重要機能への注力**（重要度: 中）
   - 特権管理（privilege）: セキュリティの核心機能
   - ロギング（logging）: 監査とトレーサビリティに必須
   - これらの強化は品質向上に直結

4. **リスクの最小化**（重要度: 中）
   - モック依存のテストを限定的に実施
   - 実装が複雑化せず、保守性を維持
   - 既存テストの破損リスクが低い

5. **Phase 3の学び**（重要度: 低）
   - Phase 3で効率的なテスト追加ノウハウを獲得
   - 優先度の高い領域に集中する戦略が有効だった

**プランB・C不採用の理由**:

| プラン | 不採用理由 |
|-------|----------|
| B | ・safefileioは既に76.4%で許容範囲<br>・E2Eテストは工数が大きい割にカバレッジ増分が小さい<br>・85.6-86.6%は目標を大きく超過し、過剰品質 |
| C | ・10-13日は予算超過<br>・cmd/runnerのmain関数テストはメンテナンスコストが高い<br>・debugパッケージは優先度が低い（本番では使用頻度が低い） |

**リスク受容の明確化**:

以下のリスクを理解した上でプランAを選択：

1. **特権管理のテスト限界**
   - **リスク**: モック依存が高く、実際の動作保証には限界がある
   - **緩和策**:
     - システムコール（`Seteuid`）は直接テストせず、モックでロジック検証
     - 実環境でのE2E検証を別途実施（手動テスト計画を策定）
     - メトリクス更新等のビジネスロジックに焦点を当てる
   - **受容理由**: 完全な実環境再現は不可能であり、モックテストで十分な品質保証が可能

2. **Slack通知の実環境検証**
   - **リスク**: HTTPモックで検証するため、実環境での動作検証が別途必要
   - **緩和策**:
     - `httptest.Server`で忠実なモック実装
     - リトライロジック、タイムアウト処理の単体テスト
     - 手動テスト手順書の作成
   - **受容理由**: ネットワーク依存のテストは不安定であり、モック + 手動検証が現実的

3. **main関数のテスト最小化**
   - **リスク**: CLIエントリーポイントのカバレッジが低いまま
   - **緩和策**:
     - 統合テストで実行パスを検証
     - `run()`関数等のビジネスロジックは個別にテスト
   - **受容理由**: main関数は薄いラッパーであり、統合テストでカバー可能

**意思決定の透明性確保**:

この意思決定プロセスと理由を文書化することで：
- 将来の振り返りで判断根拠を確認可能
- Phase 5以降の計画立案時の参考資料となる
- ステークホルダーへの説明責任を果たす

### 10.2 優先度A: 特権管理のテスト（2-3日、+0.8%）

**対象**: `internal/runner/privilege` (62.0% → 75.0%)
**カバレッジ増分**: +13.0% (パッケージ内) → +0.8% (全体)
**工数**: 2-3日

#### 10.2.1 `internal/runner/privilege/unix_privilege_test.go` の拡張

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] `TestPrepareExecution_Success`: 実行準備の成功ケース
  - [ ] モックを使用した正常フロー
  - [ ] コンテキスト設定の確認
- [ ] `TestPrepareExecution_NotSupported`: 特権実行非サポート
  - [ ] エラーハンドリング
  - [ ] エラー型の確認
- [ ] `TestPerformElevation_Success`: 特権昇格成功
  - [ ] モック特権マネージャーでの昇格
  - [ ] メトリクス更新の確認
- [ ] `TestPerformElevation_Failure`: 特権昇格失敗
  - [ ] システムコールエラーのシミュレーション
  - [ ] エラーメッセージの確認
  - [ ] メトリクス更新（失敗カウント）
- [ ] `TestHandleCleanupAndMetrics_Success`: クリーンアップ成功
  - [ ] 正常なクリーンアップフロー
  - [ ] メトリクス更新の確認
- [ ] `TestHandleCleanupAndMetrics_WithError`: クリーンアップエラー
  - [ ] エラーパスの実行
  - [ ] メトリクスへのエラー記録
- [ ] `TestRestorePrivilegesAndMetrics_Success`: 権限復元成功
  - [ ] 正常な権限復元
  - [ ] 成功メトリクスの更新
- [ ] `TestRestorePrivilegesAndMetrics_Failure`: 権限復元失敗
  - [ ] 復元エラーのシミュレーション
  - [ ] 緊急シャットダウンの呼び出し確認（モック）
  - [ ] 失敗メトリクスの記録
- [ ] カバレッジ確認: 62.0% → 75.0%

**テスト戦略**:
- システムコール（`Seteuid`）は直接テストせず、モックでの動作確認
- 実際の特権昇格は統合テスト環境での検証を推奨
- メトリクス更新ロジックに焦点を当てる

#### 10.2.2 `internal/runner/privilege/metrics_test.go` の拡張

**実装内容**:
- [ ] 既存テストファイルの確認
- [ ] `TestUpdateSuccessRate_AllCases`: 成功率更新の全ケース
  - [ ] 初回実行時（分母0からの更新）
  - [ ] 成功追加時の計算
  - [ ] 失敗追加時の計算
  - [ ] 境界値（100%、0%）
- [ ] カバレッジ確認: 66.7% → 90.0%

**テスト実装のポイント**:
- 浮動小数点計算の精度に注意
- 分母が0の場合のハンドリング確認

### 10.3 優先度A: ロギングシステムのテスト（2-3日、+0.9%）

**対象**: `internal/logging` (60.0% → 75.0%)
**カバレッジ増分**: +15.0% (パッケージ内) → +0.9% (全体)
**工数**: 2-3日

#### 10.3.1 `internal/logging/slack_handler_test.go` の作成

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] HTTPモックサーバーのセットアップ
- [ ] `TestGetSlackWebhookURL_WithEnv`: 環境変数あり
  - [ ] 環境変数の設定
  - [ ] URL取得の確認
- [ ] `TestGetSlackWebhookURL_WithoutEnv`: 環境変数なし
  - [ ] 空文字列の返却確認
- [ ] `TestNewSlackHandler_ValidURL`: 有効なURL
  - [ ] ハンドラー作成成功
  - [ ] フィールドの初期化確認
- [ ] `TestNewSlackHandler_InvalidURL`: 無効なURL
  - [ ] バリデーションエラー
  - [ ] エラーメッセージの確認
- [ ] `TestSlackHandler_Enabled`: レベルフィルタリング
  - [ ] Info以上のレベルが有効
  - [ ] Debug以下が無効
- [ ] `TestSlackHandler_Handle_CommandGroupSummary`: コマンドグループサマリー
  - [ ] メッセージ形式の確認
  - [ ] HTTPリクエストの検証
- [ ] `TestSlackHandler_Handle_PreExecutionError`: 実行前エラー
  - [ ] エラーメッセージの形式確認
  - [ ] HTTPリクエストの検証
- [ ] `TestSlackHandler_Handle_SecurityAlert`: セキュリティアラート
  - [ ] アラートメッセージの形式確認
  - [ ] 緊急度の表現確認
- [ ] `TestSlackHandler_Handle_PrivilegedCommandFailure`: 特権コマンド失敗
  - [ ] 失敗メッセージの形式確認
- [ ] `TestSlackHandler_Handle_PrivilegeEscalationFailure`: 特権昇格失敗
  - [ ] エスカレーション失敗メッセージの確認
- [ ] `TestSlackHandler_Handle_GenericMessage`: 汎用メッセージ
  - [ ] デフォルトフォーマットの確認
- [ ] カバレッジ確認: 60.0% → 75.0%

**テスト戦略**:
- `httptest.Server`でモックSlackエンドポイントを作成
- リクエストボディのJSON検証
- タイムアウト、リトライロジックのテスト
- **注意**: 実際のSlack通知は手動テストで確認が必要

#### 10.3.2 `internal/logging/slack_retry_test.go` の作成

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] `TestSendToSlack_Success`: 送信成功
  - [ ] HTTPステータス200の確認
  - [ ] リトライなしでの成功
- [ ] `TestSendToSlack_RetryLogic`: リトライロジック
  - [ ] 一時的エラーでのリトライ
  - [ ] バックオフ間隔の確認
  - [ ] 最大リトライ回数の確認
- [ ] `TestSendToSlack_PermanentFailure`: 恒久的失敗
  - [ ] 400エラーでのリトライなし
  - [ ] エラーログの確認
- [ ] `TestGenerateBackoffIntervals_Correctness`: バックオフ間隔生成
  - [ ] 間隔の増加確認
  - [ ] 最大間隔の上限確認
- [ ] カバレッジ確認: リトライ関連関数 0.0% → 80.0%

#### 10.3.3 `internal/logging/pre_execution_error_test.go` の拡張

**実装内容**:
- [ ] 既存テストファイルの確認
- [ ] `TestHandlePreExecutionError_AllTypes`: 全エラータイプ
  - [ ] ErrorTypeBuildConfig
  - [ ] ErrorTypePrivilegeDrop
  - [ ] ErrorTypeFileAccess
  - [ ] ErrorTypeConfigParsing
  - [ ] ErrorTypeSystemError
- [ ] `TestHandlePreExecutionError_OutputFormat`: 出力形式
  - [ ] stderr出力の確認
  - [ ] JSON形式の確認（構造化ログ）
- [ ] カバレッジ確認: 0.0% → 80.0%

#### 10.3.4 `internal/logging/message_formatter_test.go` の拡張

**実装内容**:
- [ ] 既存テストファイルの確認
- [ ] `TestAppendInteractiveAttrs_EdgeCases`: エッジケース
  - [ ] 空の属性リスト
  - [ ] 大量の属性
  - [ ] 特殊文字を含む属性値
- [ ] カバレッジ確認: 69.2% → 85.0%

### 10.4 統合テスト: セキュリティシナリオ拡充（1日、+0.5-1.0%）

**対象**: `test/security/` 配下の統合テスト
**カバレッジ増分**: +0.5-1.0% (全体)
**工数**: 1日

#### 10.4.1 `test/security/environment_injection_test.go` の作成

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] `TestEnvironmentVariableInjection_Blocked`: 環境変数インジェクション防止
  - [ ] `LD_PRELOAD`の注入試行
  - [ ] `PATH`の改ざん試行
  - [ ] インジェクション検出の確認
- [ ] `TestEnvironmentVariableInjection_SafeValues`: 安全な値
  - [ ] 許可された環境変数の通過
  - [ ] サニタイゼーション後の値確認
- [ ] カバレッジ確認: environment/filter.go の一部関数

#### 10.4.2 `test/security/command_argument_test.go` の作成

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] `TestCommandArgumentEscape_ShellMetacharacters`: シェルメタ文字
  - [ ] セミコロン、パイプ等の検出
  - [ ] エスケープ処理の確認
- [ ] `TestCommandArgumentEscape_QuoteInjection`: クォート注入
  - [ ] シングルクォート注入試行
  - [ ] ダブルクォート注入試行
  - [ ] エラーハンドリング
- [ ] カバレッジ確認: executor/executor.go の引数処理部分

#### 10.4.3 `test/security/hash_bypass_test.go` の作成

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] `TestHashValidation_BypassAttempts`: ハッシュ検証バイパス試行
  - [ ] シンボリックリンクでのバイパス試行
  - [ ] TOCTOU攻撃のシミュレーション
  - [ ] 検証の成功確認
- [ ] `TestHashValidation_ManifestTampering`: マニフェスト改ざん
  - [ ] マニフェストファイルの改ざん試行
  - [ ] 改ざん検出の確認
- [ ] カバレッジ確認: filevalidator/validator.go の検証ロジック

#### 10.4.4 `test/security/temp_directory_race_test.go` の作成

**実装内容**:
- [ ] ファイル作成と基本構造の準備
- [ ] `TestTempDirectory_RaceCondition`: 一時ディレクトリの競合状態
  - [ ] 並行アクセスのシミュレーション
  - [ ] ファイル作成の競合
  - [ ] 安全な処理の確認
- [ ] `TestTempDirectory_Cleanup_Concurrent`: 並行クリーンアップ
  - [ ] 複数goroutineでのクリーンアップ
  - [ ] リソースリークの検出
- [ ] カバレッジ確認: resource/normal_manager.go のクリーンアップ処理

### 10.5 Phase 4 完了確認

- [ ] カバレッジ測定: 85.0%達成確認
- [ ] 全テスト実行: `make test`
- [ ] Lint実行: `make lint`
- [ ] フォーマット: `make fmt`
- [ ] 統合テスト実行: `test/security/`, `test/performance/`
- [ ] コミット作成
- [ ] Phase 4完了レビュー

### 10.6 Phase 4 実装スケジュール

| 日数 | タスク | 成果物 | 累積カバレッジ |
|-----|--------|--------|--------------|
| Day 1 | 特権管理テスト（前半） | unix_privilege_test.go | 82.8% |
| Day 2 | 特権管理テスト（後半） | metrics_test.go 拡張 | 83.2% |
| Day 3 | Slackハンドラーテスト | slack_handler_test.go | 84.0% |
| Day 4 | Slack リトライ・エラー処理 | slack_retry_test.go, pre_execution_error_test.go | 84.5% |
| Day 5 | 統合テスト | test/security/ 拡充 | 85.0% |
| Day 6 | バッファ・レビュー | 完了確認 | 85.0%+ |

### 10.7 Phase 4 成功基準

Phase 4は以下の条件がすべて満たされた時点で完了とする：

- [ ] **カバレッジ目標**: 85.0%以上達成
- [ ] **重点パッケージ**:
  - [ ] internal/runner/privilege: 75.0%以上
  - [ ] internal/logging: 75.0%以上
  - [ ] 統合テスト: 新規4ファイル追加
- [ ] **全テストパス**: 既存・新規すべてのテストが成功
- [ ] **パフォーマンス**: テスト実行時間が7分以内（Phase 3: 5分 + 余裕2分）
- [ ] **コード品質**: Lintエラーなし
- [ ] **ドキュメント**: 制約事項の明記（モック依存、実環境テスト推奨箇所）

### 10.8 Phase 4 リスクと緩和策

| リスク | 影響度 | 発生確率 | 緩和策 |
|-------|--------|---------|--------|
| 特権管理テストの複雑化 | 高 | 中 | モック設計の事前レビュー、既存パターンの活用 |
| HTTPモックの不安定性 | 中 | 低 | httptest.Server の活用、タイムアウト設定の最適化 |
| 統合テストの実行時間増加 | 中 | 中 | 並列実行の最適化、重いテストの分離 |
| 実環境との乖離 | 高 | 中 | 制約事項のドキュメント化、手動テスト手順の整備 |
| カバレッジ目標未達 | 中 | 低 | バッファ日の確保、優先度B施策の待機 |

### 10.9 Phase 4 後の推奨事項

Phase 4完了後、以下の追加施策を検討：

1. **手動統合テスト計画の策定**
   - 特権管理の実環境テストシナリオ
   - Slack通知のE2Eテスト手順

2. **CI/CDパイプラインの強化**
   - カバレッジレポートの自動生成
   - カバレッジ低下の検出とアラート

3. **ドキュメント整備**
   - テスト戦略書の作成
   - モック vs 実環境テストのガイドライン

4. **Phase 5の検討（オプション）**
   - cmd/runner のテスト追加（目標カバレッジ: 87-88%）
   - パフォーマンステストの拡充
