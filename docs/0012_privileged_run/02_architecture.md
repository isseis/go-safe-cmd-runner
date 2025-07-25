# アーキテクチャ設計書: Privileged コマンド実行機能

## 1. システム概要

### 1.1 アーキテクチャ原則
- **最小権限の原則**: 権限昇格は必要な操作の直前に行い、操作完了後即座に降格
- **セキュリティファースト**: 全ての権限操作は安全性を最優先に設計
- **既存システムとの統合**: 現在のexecutorインターフェースとの完全な互換性
- **障害時の安全性**: 任意のエラー状況でも権限が適切に復元される

### 1.2 システム境界
- **対象プラットフォーム**: Linux/Unix系のみ
- **権限モデル**: setuid root バイナリによる実効UID切り替え
- **統合ポイント**: internal/runner/executor パッケージ
- **セキュリティ統合**: internal/filevalidator との連携

## 2. コンポーネント設計

### 2.1 権限管理コンポーネント (PrivilegeManager)

```go
// PrivilegeManager manages privilege escalation and restoration
type PrivilegeManager struct {
    originalUID int
    logger      *slog.Logger
}

// WithPrivileges executes a function with escalated privileges
// This is the ONLY public method to ensure safe privilege management
func (pm *PrivilegeManager) WithPrivileges(fn func() error) error

// escalatePrivileges escalates to root privileges (private method)
func (pm *PrivilegeManager) escalatePrivileges() error

// restorePrivileges restores original user privileges (private method)
func (pm *PrivilegeManager) restorePrivileges() error
```

**設計ポイント:**
- **セキュアな設計**: `WithPrivileges` のみを公開し、内部で権限昇格・復帰を保証
- **確実な権限復元**: defer文による自動復元でヒューマンエラーを防止
- **panic安全性**: recover機構で異常終了時も権限復元を保証
- **責任の集約**: 権限管理ロジックを一箇所に集中して管理

### 2.2 特権対応Executor (PrivilegedExecutor)

```go
// PrivilegedExecutor extends the standard executor with privilege management
type PrivilegedExecutor struct {
    baseExecutor     executor.CommandExecutor
    privilegeManager *PrivilegeManager
    fileValidator    *filevalidator.Validator
}

// Execute implements the CommandExecutor interface with privilege support
func (pe *PrivilegedExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*executor.Result, error)
```

**設計ポイント:**
- 既存の `CommandExecutor` インターフェースを完全実装
- コンポジションパターンによる既存executorの拡張
- privilegedフラグに基づく条件付き権限昇格

### 2.3 ハッシュ値計算の権限管理

```go
// PrivilegedHashCalculator handles hash calculation with privilege management
type PrivilegedHashCalculator struct {
    privilegeManager *PrivilegeManager
    baseCalculator   filevalidator.HashCalculator
}

// CalculateHash calculates file hash with privilege escalation if needed
func (phc *PrivilegedHashCalculator) CalculateHash(filePath string, privileged bool) (string, error)
```

**設計ポイント:**
- filevalidatorとの統合
- privilegedコマンドのハッシュ計算時のみ権限昇格
- 計算完了後の即座の権限降格

## 3. 権限切り替えフロー

### 3.1 コマンド実行フロー

```
1. コマンド受信
2. Privilegedフラグ確認
   ├─ false: 標準executorで実行
   └─ true: 特権実行フロー
3. 【特権実行フロー】
   a. ハッシュ値計算
      ├─ seteuid(0)
      ├─ ファイルハッシュ計算
      └─ seteuid(originalUID)
   b. コマンド実行
      ├─ seteuid(0)
      ├─ exec.Command実行
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

### 4.2 安全性保証メカニズム

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

### 5.1 Runner統合

```go
// NewRunner with privileged executor support
func NewRunner(config *runnertypes.Config, options ...Option) (*Runner, error) {
    // 既存の実装を拡張
    var executor executor.CommandExecutor

    if hasPrivilegedCommands(config) {
        privilegeManager := NewPrivilegeManager()
        executor = NewPrivilegedExecutor(
            executor.NewDefaultExecutor(),
            privilegeManager,
            fileValidator)
    } else {
        executor = executor.NewDefaultExecutor()
    }

    return &Runner{
        executor: executor,
        // ... 他のフィールド
    }, nil
}
```

### 5.2 設定互換性

```toml
[[groups.commands]]
name = "mysql_backup"
cmd = "/usr/bin/mysqldump"
args = ["-u", "root", "-p${MYSQL_ROOT_PASSWORD}", "database"]
privileged = true  # この設定で特権実行が有効化
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
