# 特権バリデータ・マネージャーを使用した実行フロー解説

## 1. 概要

本文書では、go-safe-cmd-runnerにおいて特権バリデータ（ValidatorWithPrivileges）と特権マネージャー（PrivilegeManager）を使用した場合の、設定ファイル読み込みからファイル検証・ハッシュ操作までの詳細な実行フローを解説する。

## 2. アーキテクチャ概要

```
┌─────────────────┐    ┌──────────────────────┐    ┌─────────────────────┐
│                 │    │                      │    │                     │
│   設定ファイル   │───▶│   検証マネージャー    │───▶│  ファイル検証実行   │
│                 │    │                      │    │                     │
└─────────────────┘    └──────────────────────┘    └─────────────────────┘
                               │
                               ▼
                        ┌──────────────────────┐
                        │                      │
                        │  特権バリデータ選択  │
                        │                      │
                        └──────────────────────┘
                               │
                    ┌──────────┴──────────┐
                    ▼                     ▼
            ┌───────────────┐     ┌─────────────────────┐
            │               │     │                     │
            │ 標準バリデータ │     │ 特権バリデータ      │
            │               │     │ (権限昇格付き)      │
            └───────────────┘     └─────────────────────┘
```

## 3. 主要コンポーネント

### 3.1 設定読み込みコンポーネント
- **場所**: `internal/verification/manager.go`
- **役割**: 検証対象ファイルの設定と特権マネージャーの設定

### 3.2 検証マネージャー
- **場所**: `internal/verification/manager.go`
- **役割**: 適切なバリデータの選択と設定

### 3.3 ファイルバリデータ
- **基本**: `internal/filevalidator/validator.go`
- **特権拡張**: `internal/filevalidator/privileged_validator.go`

### 3.4 権限マネージャー
- **インターフェース**: `internal/runner/runnertypes/config.go`
- **実装**: `internal/runner/privilege/unix.go`

## 4. 詳細実行フロー

### 4.1 初期化フェーズ

#### Step 1: 検証マネージャーの作成

```go
// internal/verification/manager.go
func NewManagerWithOpts(hashDir string, options ...Option) (*Manager, error) {
    // デフォルトオプション適用
    opts := newOptions()
    for _, option := range options {
        option(opts)
    }

    manager := &Manager{
        hashDir:          hashDir,
        fs:               opts.fs,
        privilegeManager: opts.privilegeManager,  // ←特権マネージャー設定
    }
```

#### Step 2: バリデータの選択と初期化

```go
// ファイルバリデータの初期化ロジック
if opts.fileValidatorEnabled {
    var err error

    if opts.privilegeManager != nil {
        // 特権マネージャーが利用可能 → 特権バリデータを作成
        logger := slog.Default()
        manager.fileValidator, err = filevalidator.NewValidatorWithPrivileges(
            &filevalidator.SHA256{},
            hashDir,
            opts.privilegeManager,
            logger
        )
    } else {
        // 標準バリデータを作成
        manager.fileValidator, err = filevalidator.New(
            &filevalidator.SHA256{},
            hashDir
        )
    }

    if err != nil {
        return nil, fmt.Errorf("failed to initialize file validator: %w", err)
    }
}
```

**重要なポイント:**
- 特権マネージャーの有無により動的にバリデータ種別を決定
- `FileValidator`インターフェースにより両タイプが統一的に扱われる
- セキュリティとパフォーマンスのバランスを考慮した設計

### 4.2 ファイル検証実行フェーズ

#### Step 3: ファイル検証の実行

```go
// internal/verification/manager.go
func (m *Manager) VerifyConfigFile(configPath string) error {
    // ハッシュディレクトリの検証
    if err := m.ValidateHashDirectory(); err != nil {
        return &Error{
            Op:   "ValidateHashDirectory",
            Path: m.hashDir,
            Err:  err,
        }
    }

    // ファイルハッシュ検証（インターフェース経由）
    if err := m.fileValidator.Verify(configPath); err != nil {
        slog.Error("Config file verification failed",
            "config_path", configPath,
            "error", err)
        return &Error{
            Op:   "VerifyHash",
            Path: configPath,
            Err:  err,
        }
    }

    return nil
}
```

### 4.3 特権バリデータでの権限昇格フロー

#### Step 4: 特権が必要な場合の詳細フロー

特権バリデータが使用される場合の内部動作：

```go
// internal/filevalidator/privileged_validator.go
func (v *ValidatorWithPrivileges) VerifyWithPrivileges(
    ctx context.Context,
    filePath string,
    needsPrivileges bool,
) error {
    return v.executeWithPrivilegesIfNeeded(
        ctx,
        filePath,
        needsPrivileges,
        runnertypes.OperationFileHashCalculation,
        "file_hash_verify",
        func() error { return v.Verify(filePath) },  // ←基本バリデータメソッド呼び出し
        "File hash verified with privileges",
        "file hash verification",
        map[string]any{},
    )
}
```

#### Step 5: 権限昇格実行の詳細

```go
func (v *ValidatorWithPrivileges) executeWithPrivilegesIfNeeded(
    ctx context.Context,
    filePath string,
    needsPrivileges bool,
    operation runnertypes.Operation,
    commandName string,
    action func() error,
    successMsg string,
    failureMsg string,
    logFields map[string]any,
) error {
    var err error
    wasPrivileged := false

    // 権限昇格の判定と実行
    if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
        elevationCtx := runnertypes.ElevationContext{
            Operation:   operation,
            CommandName: commandName,
            FilePath:    filePath,
        }
        err = v.privMgr.WithPrivileges(ctx, elevationCtx, action)  // ←実際の権限昇格実行
        wasPrivileged = true
    } else {
        err = action()  // ←通常権限での実行
    }

    // 安全なログ出力（セキュリティフィルタ付き）
    logArgs := []any{"file_path", filePath}
    safeFields := v.secValidator.CreateSafeLogFields(logFields)
    if err != nil {
        safeFields["error"] = v.secValidator.SanitizeErrorForLogging(err)
    }

    // 結果ログ出力
    for k, v := range safeFields {
        logArgs = append(logArgs, k, v)
    }

    if err != nil {
        v.logger.Error(failureMsg, logArgs...)
        if wasPrivileged {
            return fmt.Errorf("privileged %s failed: %w", failureMsg, err)
        }
        return fmt.Errorf("%s failed: %w", failureMsg, err)
    }

    // 成功ログ
    if wasPrivileged {
        v.logger.Info(successMsg, logArgs...)
    } else {
        v.logger.Debug(successMsg, logArgs...)
    }

    return nil
}
```

### 4.4 権限マネージャーでの低レベル権限管理

#### Step 6: setuid/seteuid による権限切り替え

```go
// internal/runner/privilege/unix.go
func (m *UnixPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    return m.withPrivilegesInternal(ctx, elevationCtx, fn)
}

func (m *UnixPrivilegeManager) withPrivilegesInternal(ctx context.Context, elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    // 権限昇格前の安全性チェック
    m.mu.Lock()
    defer m.mu.Unlock()

    if !m.isSetuid {
        return ErrPrivilegedExecutionNotAvailable
    }

    // 1. 権限昇格実行
    if err := syscall.Seteuid(0); err != nil {
        return &Error{
            Operation:   elevationCtx.Operation,
            CommandName: elevationCtx.CommandName,
            OriginalUID: m.originalUID,
            TargetUID:   0,
            SyscallErr:  err,
        }
    }

    // 2. defer による確実な権限復帰設定
    defer func() {
        if restoreErr := syscall.Seteuid(m.originalUID); restoreErr != nil {
            // 権限復帰失敗は致命的セキュリティ問題 → 緊急停止
            m.emergencyShutdown(restoreErr, "privilege restoration failed")
        }
    }()

    // 3. 権限昇格状態での実際の処理実行
    return fn()
}
```

## 5. 実際の使用例

### 5.1 設定ファイル読み込み時の実行例

```bash
# setuid rootバイナリとして実行
sudo chown root:root /usr/local/bin/go-safe-cmd-runner
sudo chmod u+s /usr/local/bin/go-safe-cmd-runner

# 一般ユーザーとして実行
$ /usr/local/bin/go-safe-cmd-runner verify -file /etc/sensitive-config.toml
```

### 5.2 ログ出力例

#### 権限昇格あり（特権が必要なファイル）

```
2025/07/27 23:56:46 INFO File hash verified with privileges file_path=/etc/sensitive-config.toml
```

#### 権限昇格なし（通常ファイル）

```
2025/07/27 23:56:46 DEBUG File hash verified with privileges file_path=/home/user/config.toml
```

#### 権限昇格失敗

```
2025/07/27 23:56:46 ERROR file hash verification file_path=/etc/sensitive-config.toml error="[error details redacted for security]"
```

## 6. セキュリティ考慮事項

### 6.1 権限昇格の最小化

- **期間**: ファイル操作の必要最小限の時間のみ
- **対象**: 特権が明示的に要求された操作のみ
- **範囲**: 個別のファイル操作単位で制御

### 6.2 障害時の安全性

- **権限復帰失敗**: `emergencyShutdown()` による即座のプロセス終了
- **例外発生**: `defer` による確実な権限復帰
- **ログ保護**: センシティブ情報のサニタイズ処理

### 6.3 監査証跡

- **操作記録**: 全ての権限昇格操作をログ記録
- **エラー追跡**: 権限関連エラーの詳細記録
- **アクセス制御**: ファイルアクセスパターンの可視化

## 7. 設定例

### 7.1 検証マネージャーの特権付き初期化

```go
// 特権マネージャー付きでの初期化例
func initializeWithPrivileges() (*verification.Manager, error) {
    privMgr := privilege.NewManager(slog.Default())

    return verification.NewManagerWithOpts(
        "/etc/hashes",
        verification.WithPrivilegeManager(privMgr),
    )
}
```

### 7.2 実行時フラグによる制御

```go
// 実行時の特権制御例
func processFile(manager *verification.Manager, filePath string, needsPrivileges bool) error {
    if validator, ok := manager.FileValidator.(*filevalidator.ValidatorWithPrivileges); ok {
        // 特権バリデータが利用可能
        return validator.VerifyWithPrivileges(
            context.Background(),
            filePath,
            needsPrivileges,  // ←動的な権限制御
        )
    }

    // 標準バリデータでの検証
    return manager.FileValidator.Verify(filePath)
}
```

## 8. 結論

特権バリデータ・マネージャーシステムにより、以下が実現される：

1. **動的権限管理**: 必要時のみの権限昇格
2. **統一インターフェース**: 特権/非特権処理の透明性
3. **セキュリティ強化**: 最小権限原則と確実な権限復帰
4. **監査可能性**: 全操作の詳細なログ記録
5. **堅牢性**: 障害時の安全な停止機構

この設計により、セキュリティを損なうことなく、システムファイルへの安全なアクセスが可能となる。
