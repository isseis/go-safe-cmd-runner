# アーキテクチャ設計書: Privileged コマンド実行機能

## 1. システム概要

### 1.1 アーキテクチャ原則
- **最小権限の原則**: 権限昇格は必要な操作の直前に行い、操作完了後即座に降格
- **セキュリティファースト**: 全ての権限操作は安全性を最優先に設計
- **既存システムとの統合**: 現在のファイル検証システムとの完全な互換性
- **障害時の安全性**: 任意のエラー状況でも権限が適切に復元される
- **型の統一**: runnertypesパッケージで権限管理関連の型を一元管理

### 1.2 システム境界
- **対象プラットフォーム**: Linux/Unix系のみ
- **権限モデル**: setuid root バイナリによる実効UID切り替え
- **統合ポイント**: internal/filevalidator パッケージ
- **実行フロー**: 設定ファイル読み込み → 検証管理 → 特権ファイル操作

## 2. コンポーネント設計

### 2.1 権限管理インターフェース (runnertypes.PrivilegeManager)

```go
// PrivilegeManager interface defines methods for privilege elevation/dropping
type PrivilegeManager interface {
    ElevatePrivileges() error
    DropPrivileges() error
    IsPrivilegedExecutionSupported() bool
    WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error
}

// Operation represents different types of privileged operations
type Operation string

const (
    OperationFileHashCalculation Operation = "file_hash_calculation"
    OperationCommandExecution    Operation = "command_execution"
    OperationFileAccess          Operation = "file_access"
    OperationHealthCheck         Operation = "health_check"
)

// ElevationContext contains context information for privilege elevation
type ElevationContext struct {
    Operation   Operation
    CommandName string
    FilePath    string
    StartTime   time.Time
    OriginalUID int
    TargetUID   int
}
```

**設計ポイント:**
- **型の統一**: runnertypesパッケージで権限関連型を一元管理
- **セキュアな設計**: `WithPrivileges` メソッドで安全な権限管理を保証
- **コンテキスト情報**: 操作種別とメタデータによる詳細な監査ログ
- **プラットフォーム抽象化**: インターフェースによる実装の切り替え

### 2.2 権限管理実装 (privilege.Manager)

```go
// Manager interface for privilege management (extends runnertypes.PrivilegeManager)
type Manager interface {
    runnertypes.PrivilegeManager

    // Additional methods specific to privilege package
    GetCurrentUID() int
    GetOriginalUID() int
    HealthCheck(ctx context.Context) error
    GetHealthStatus(ctx context.Context) HealthStatus
    GetMetrics() Metrics
}
```

**設計ポイント:**
- **インターフェース拡張**: runnertypesを基底として機能拡張
- **プラットフォーム実装**: Unix/Linux用とWindows用の個別実装
- **健全性チェック**: 権限昇格機能の動作確認機能
- **メトリクス収集**: 権限操作の統計情報管理

### 2.3 特権対応ファイル検証 (filevalidator.ValidatorWithPrivileges)

```go
// ValidatorWithPrivileges extends the base Validator with privilege management capabilities
type ValidatorWithPrivileges struct {
    *Validator
    privMgr      runnertypes.PrivilegeManager
    logger       *slog.Logger
    secValidator *security.Validator
}

// RecordWithPrivileges calculates and records file hash with privilege elevation if needed
func (v *ValidatorWithPrivileges) RecordWithPrivileges(
    ctx context.Context,
    filePath string,
    needsPrivileges bool,
    force bool,
) (string, error)

// VerifyWithPrivileges validates file hash with privilege elevation if needed
func (v *ValidatorWithPrivileges) VerifyWithPrivileges(
    ctx context.Context,
    filePath string,
    needsPrivileges bool,
) error
```

**設計ポイント:**
- **コンポジション**: 基本Validatorを埋め込みによる拡張
- **条件付き権限昇格**: needsPrivilegesフラグによる動的な権限管理
- **セキュリティ統合**: security.Validatorとの連携による安全なログ出力
- **統一インターフェース**: FileValidatorインターフェースによる抽象化

## 3. 権限切り替えフロー

### 3.1 ファイル検証フロー

```
1. 設定ファイル読み込み
2. 検証マネージャー初期化
   ├─ PrivilegeManager が利用可能？
   │  ├─ Yes: ValidatorWithPrivilegesを作成
   │  └─ No:  標準Validatorを作成
   └─ FileValidatorインターフェースで統一
3. 【特権ファイル検証フロー】
   a. ファイルハッシュ記録
      ├─ ElevationContext(OperationFileHashCalculation)
      ├─ seteuid(0)
      ├─ ファイル読み込み・ハッシュ計算
      ├─ ハッシュファイル書き込み
      └─ seteuid(originalUID)
   b. ファイルハッシュ検証
      ├─ ElevationContext(OperationFileHashCalculation)
      ├─ seteuid(0)
      ├─ ファイル読み込み・ハッシュ計算
      ├─ 保存済みハッシュと比較
      └─ seteuid(originalUID)
4. 結果返却
```

### 3.2 エラー処理フロー

```
任意のポイントでエラー発生
├─ defer文による権限復元実行
├─ panic発生時のrecover処理
├─ エラーログ記録
└─ 上位レイヤーへエラー伝播
```

## 4. セキュリティ設計

### 4.1 権限昇格の最小化

| 操作 | 昇格前 | 昇格中 | 昇格後 |
|------|--------|--------|--------|
| ハッシュ計算 | 一般ユーザー | root | 一般ユーザー |
| コマンド実行 | 一般ユーザー | root | 一般ユーザー |
| その他処理 | 一般ユーザー | - | 一般ユーザー |

### 4.2 特権実行サポートの拡張

**2つの特権実行モードをサポート**：

#### 4.2.1 Native Root実行
rootユーザーによる直接実行では、setuidビットに関係なく特権コマンドを実行可能：

```go
// isPrivilegeExecutionSupported checks for both native root and setuid execution
func isPrivilegeExecutionSupported(logger *slog.Logger) bool {
    originalUID := syscall.Getuid()
    effectiveUID := syscall.Geteuid()

    // Case 1: Native root execution (both real and effective UID are 0)
    if originalUID == 0 && effectiveUID == 0 {
        logger.Info("Privilege execution supported: native root execution")
        return true
    }

    // Case 2: Setuid binary execution
    return isSetuidBinary(logger)
}
```

#### 4.2.2 Setuid Binary実行

**ファイルシステムベース検証**により、より堅牢なsetuid検出を実現：

```go
// isSetuidBinary checks if the current binary has the setuid bit set
// This provides more robust detection than checking runtime UID/EUID which
// can be altered by previous seteuid() calls
func isSetuidBinary(logger *slog.Logger) bool {
    // Get the path to the current executable
    execPath, err := os.Executable()
    if err != nil {
        return false
    }

    // Get file information and check setuid bit
    fileInfo, err := os.Stat(execPath)
    if err != nil {
        return false
    }

    hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

    // Check root ownership - essential for setuid to work
    var isOwnedByRoot bool
    if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
        isOwnedByRoot = stat.Uid == 0
    }

    originalUID := syscall.Getuid()

    // Valid setuid scenario: setuid bit + root ownership + non-root real UID
    return hasSetuidBit && isOwnedByRoot && originalUID != 0
}
```

**実行モード比較:**

| 実行モード | 実行ユーザー | setuidビット | 権限昇格方法 | ログメッセージ |
|-----------|-------------|-------------|-------------|-------------|
| Native Root | root (UID=0) | 不要 | seteuid不要 | "Native root execution - no privilege escalation needed" |
| Setuid Binary | 一般ユーザー | 必要 + root所有 | seteuid(0) → seteuid(originalUID) | "Privileges elevated" → "Privileges restored" |

**従来方式との比較:**
- **従来**: `effectiveUID == 0 && originalUID != 0` （実行時UID比較のみ）
- **改善後**: Native root + Setuid binaryの両方をサポート
- **利点**:
  - rootユーザーによる直接実行をサポート
  - setuidバイナリでの堅牢な検証
  - 実行環境に応じた適切な権限管理

### 4.3 安全性保証メカニズム

```go
func (pm *PrivilegeManager) WithPrivileges(fn func() error) error {
    // 権限昇格
    if err := pm.escalatePrivileges(); err != nil {
        return fmt.Errorf("privilege escalation failed: %w", err)
    }

    // 単一のdefer文で権限復帰とpanic処理を統合
    defer func() {
        var panicValue interface{}
        var context string

        // panic検出
        if r := recover(); r != nil {
            panicValue = r
            context = fmt.Sprintf("after panic: %v", r)
            pm.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
                "panic", r,
                "original_uid", pm.originalUID)
        } else {
            context = "normal execution"
        }

        // 権限復帰実行（常に実行される）
        if err := pm.restorePrivileges(); err != nil {
            // 権限復帰失敗は致命的セキュリティリスク - 即座に終了
            pm.emergencyShutdown(err, context)
        }

        // panic再発生（必要な場合のみ）
        if panicValue != nil {
            panic(panicValue)
        }
    }()

    return fn()
}

// emergencyShutdown handles critical privilege restoration failures
func (pm *PrivilegeManager) emergencyShutdown(restoreErr error, context string) {
    // 詳細なエラー情報を記録（複数の出力先に確実に記録）
    criticalMsg := fmt.Sprintf("CRITICAL SECURITY FAILURE: Privilege restoration failed during %s", context)

    // 構造化ログに記録
    pm.logger.Error(criticalMsg,
        "error", restoreErr,
        "original_uid", pm.originalUID,
        "current_uid", os.Getuid(),
        "current_euid", os.Geteuid(),
        "timestamp", time.Now().UTC(),
        "process_id", os.Getpid(),
    )

    // システムログにも記録（rsyslog等による外部転送対応）
    syslog.Err(fmt.Sprintf("%s: %v (PID: %d, UID: %d->%d)",
        criticalMsg, restoreErr, os.Getpid(), pm.originalUID, os.Geteuid()))

    // 標準エラー出力にも記録（最後の手段）
    fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", criticalMsg, restoreErr)

    // 即座にプロセス終了（defer処理をスキップ）
    os.Exit(1)
}
```

### 4.3 監査とログ

```go
// 権限操作のログ記録
slog.Info("Privilege escalation started",
    "command", cmd.Name,
    "original_uid", originalUID)
slog.Info("Privilege restoration completed",
    "command", cmd.Name,
    "duration_ms", duration.Milliseconds())
```

## 5. インターフェース設計

### 5.1 検証マネージャー統合

```go
// NewManagerWithOpts with privilege support
func NewManagerWithOpts(hashDir string, options ...Option) (*Manager, error) {
    opts := newOptions()
    for _, option := range options {
        option(opts)
    }

    manager := &Manager{
        hashDir:          hashDir,
        fs:               opts.fs,
        privilegeManager: opts.privilegeManager,  // 特権マネージャー設定
    }

    // バリデータの動的選択
    if opts.fileValidatorEnabled {
        var err error

        if opts.privilegeManager != nil {
            // 特権バリデータを作成
            logger := slog.Default()
            manager.fileValidator, err = filevalidator.NewValidatorWithPrivileges(
                &filevalidator.SHA256{}, hashDir, opts.privilegeManager, logger)
        } else {
            // 標準バリデータを作成
            manager.fileValidator, err = filevalidator.New(&filevalidator.SHA256{}, hashDir)
        }

        if err != nil {
            return nil, fmt.Errorf("failed to initialize file validator: %w", err)
        }
    }

    return manager, nil
}
```

### 5.2 使用例

```go
// 特権マネージャー付きでの初期化
privMgr := privilege.NewManager(slog.Default())
manager, err := verification.NewManagerWithOpts(
    "/etc/hashes",
    verification.WithPrivilegeManager(privMgr),
)

// ファイル検証実行（権限昇格は自動判定）
err = manager.VerifyConfigFile("/etc/sensitive-config.toml")
```

## 6. エラー処理戦略

### 6.1 エラー分類

```go
var (
    ErrPrivilegeEscalationFailed  = errors.New("privilege escalation failed")
    ErrPrivilegeRestorationFailed = errors.New("privilege restoration failed")
    ErrPrivilegedHashFailed       = errors.New("privileged hash calculation failed")
    ErrPrivilegedExecutionFailed  = errors.New("privileged command execution failed")
)
```

### 6.2 障害時の動作

| エラー種類 | 動作 |
|------------|------|
| 権限昇格失敗 | コマンド実行中止、グループ実行中止、次グループ継続 |
| 権限復元失敗 | **即座にプロセス終了（os.Exit(1)）** - セキュリティ最優先 |
| ハッシュ計算失敗 | コマンド実行中止、グループ実行中止 |
| 特権コマンド実行失敗 | 標準エラーハンドリング |

**権限復元失敗の緊急対応:**
- 複数の出力先への詳細ログ記録（構造化ログ + syslog + stderr）
- 現在のUID/EUID状態の記録
- defer処理をバイパスして即座に終了
- 一時ファイル等の残存よりもセキュリティを優先

## 7. パフォーマンス考慮事項

### 7.1 権限切り替えオーバーヘッド
- seteuid システムコール: 通常数マイクロ秒
- 権限昇格は必要最小限の期間のみ
- privileged=false のコマンドには影響なし

### 7.2 最適化戦略
- 連続するprivilegedコマンドでも個別に権限切り替え実行
- ログ出力の非同期化
- エラー時の早期return

## 8. テスト戦略

### 8.1 単体テスト

```go
// 権限管理のテスト（WithPrivilegesメソッドに集約）
func TestPrivilegeManager_WithPrivileges_Success(t *testing.T)
func TestPrivilegeManager_WithPrivileges_EscalationFailure(t *testing.T)
func TestPrivilegeManager_WithPrivileges_PanicRecovery(t *testing.T)
func TestPrivilegeManager_WithPrivileges_RestoreFailure_EmergencyShutdown(t *testing.T)
func TestPrivilegeManager_EmergencyShutdown_LoggingBehavior(t *testing.T)

// 特権実行のテスト
func TestPrivilegedExecutor_Execute(t *testing.T)
func TestPrivilegedExecutor_ErrorHandling(t *testing.T)

// 緊急終了テストの注意事項
// - os.Exit(1)をテストするためにはプロセス分離が必要
// - testdata/やintegration_test.goでサブプロセステストを実装
// - ログ出力の検証にはbuffer interceptorを使用
```

### 8.2 統合テスト

```go
// End-to-endテスト
func TestPrivilegedCommandExecution(t *testing.T)
func TestMixedPrivilegedAndNormalCommands(t *testing.T)
func TestPrivilegeFailureRecovery(t *testing.T)
```

### 8.3 セキュリティテスト

- 権限昇格後の確実な復元確認
- エラー時の権限状態検証
- 不正な権限操作の防止確認

## 9. 運用考慮事項

### 9.1 デプロイメント要件

```bash
# バイナリにsetuidビット設定
sudo chown root:root runner
sudo chmod u+s runner
```

### 9.2 監視とアラート

- 権限昇格の頻度監視
- **権限復元失敗の即時アラート（最高優先度）**
- 異常な権限操作パターンの検出
- プロセス異常終了（exit code 1）の監視
- syslogレベルでの重大エラー転送設定

### 9.3 セキュリティガイドライン

- privilegedコマンドの定期レビュー
- ログの外部システム転送
- filevalidatorによるバイナリ整合性確認

## 10. 実装順序

### 10.1 Phase 1: 基盤実装
- [ ] PrivilegeManager の実装
- [ ] 基本的な権限切り替え機能
- [ ] エラーハンドリングフレームワーク

### 10.2 Phase 2: Executor統合
- [ ] PrivilegedExecutor の実装
- [ ] 既存executorとの統合
- [ ] filevalidator連携

### 10.3 Phase 3: Runner統合
- [ ] Runner での特権executor使用
- [ ] 設定ベースの機能切り替え
- [ ] 包括的なテスト実装

### 10.4 Phase 4: 運用機能
- [ ] 監査ログの実装
- [ ] パフォーマンス最適化
- [ ] ドキュメント整備

## 11. リスク軽減策

### 11.1 技術リスク
- **権限復元失敗**: emergencyShutdownによる即座の終了で継続実行リスクを排除
- **セキュリティ脆弱性**: 権限管理の単一責任化とfilevalidatorによる検証
- **権限昇格の悪用**: 非公開メソッドによる直接アクセス防止
- **ログ記録失敗**: 複数出力先（構造化ログ + syslog + stderr）による冗長化
- **互換性問題**: 段階的導入と既存テストの継続実行

### 11.2 運用リスク
- **設定ミス**: 明確な文書化と設定例の提供
- **監査要件**: 詳細なログ記録と外部転送機能
- **パフォーマンス影響**: 非privilegedコマンドへの影響最小化

## 12. 承認

本アーキテクチャ設計に基づき、privilegedコマンド実行機能の詳細設計と実装を進める。

承認者: [アーキテクト]
承認日: [承認日]
