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

**特権マネージャーの初期化プロセス（拡張版）:**

```go
// internal/runner/privilege/unix.go
func newPlatformManager(logger *slog.Logger) Manager {
    originalUID := syscall.Getuid()

    // 特権実行サポートの確認（Native root または Setuid binary）
    privilegeSupported := isPrivilegeExecutionSupported(logger)

    return &UnixPrivilegeManager{
        logger:      logger,
        originalUID: originalUID,
        originalGID: syscall.Getgid(),
        isSetuid:    privilegeSupported,  // ←両方の実行モードをサポート
    }
}

// 2つの実行モードをサポート
func isPrivilegeExecutionSupported(logger *slog.Logger) bool {
    originalUID := syscall.Getuid()
    effectiveUID := syscall.Geteuid()

    // Case 1: Native root実行（実UID・実効UIDともに0）
    if originalUID == 0 && effectiveUID == 0 {
        logger.Info("Privilege execution supported: native root execution")
        return true
    }

    // Case 2: Setuid binary実行（ファイルシステムベース検証）
    return isRootOwnedSetuidBinary(logger)
}

// isRootOwnedSetuidBinary - より堅牢なsetuid検出（root所有も判定）
func isRootOwnedSetuidBinary(logger *slog.Logger) bool {
    execPath, err := os.Executable()
    if err != nil {
        return false
    }

    fileInfo, err := os.Stat(execPath)
    if err != nil {
        return false
    }

    hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

    // root所有権の確認 - setuidが機能するための必須条件
    var isOwnedByRoot bool
    if stat, ok := fileInfo.Sys().(*syscall.Stat_t); ok {
        isOwnedByRoot = stat.Uid == 0
    }

    originalUID := syscall.Getuid()

    // 有効なsetuid環境: setuidビット + root所有権 + 非rootユーザー
    return hasSetuidBit && isOwnedByRoot && originalUID != 0
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
- **2つの実行モード**: Native root実行とSetuid binary実行の両方をサポート
- **適応的権限管理**: 実行環境に応じてseteuid呼び出しの有無を決定
- **堅牢な検証**: setuidバイナリでは3条件（setuidビット・root所有権・非rootユーザー）を確認
- **統一インターフェース**: `FileValidator`インターフェースにより両タイプを透明に扱う
- **運用柔軟性**: rootユーザーによる直接実行も一般ユーザーによるsetuid実行も対応

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

## 9. コマンド検証・実行フロー (拡張版)

### 9.1 概要

ファイル検証に加えて、設定ファイルから読み込まれたコマンドの検証と実行までの完全なフローを解説する。このフローには、コマンドパス解決、バイナリ検証、特権コマンド実行が含まれる。

### 9.2 完全なアーキテクチャ図

```
┌─────────────────┐    ┌──────────────────────┐    ┌─────────────────────┐
│                 │    │                      │    │                     │
│   設定ファイル   │───▶│   検証マネージャー    │───▶│  ファイル検証実行   │
│                 │    │                      │    │                     │
└─────────────────┘    └──────────────────────┘    └─────────────────────┘
                               │                            │
                               ▼                            ▼
                        ┌──────────────────────┐    ┌─────────────────────┐
                        │                      │    │                     │
                        │  特権バリデータ選択  │    │   コマンド検証開始   │
                        │                      │    │                     │
                        └──────────────────────┘    └─────────────────────┘
                               │                            │
                    ┌──────────┴──────────┐                 ▼
                    ▼                     ▼         ┌─────────────────────┐
            ┌───────────────┐     ┌─────────────────────┐ │                     │
            │               │     │                     │ │   パス解決・検証    │
            │ 標準バリデータ │     │ 特権バリデータ      │ │                     │
            │               │     │ (権限昇格付き)      │ └─────────────────────┘
            └───────────────┘     └─────────────────────┘         │
                                                                  ▼
                                                          ┌─────────────────────┐
                                                          │                     │
                                                          │  コマンド実行       │
                                                          │  (特権管理付き)     │
                                                          └─────────────────────┘
```

### 9.3 コマンド検証・実行の詳細フロー

#### Step 7: 設定ファイルからのコマンド読み込み

```go
// internal/runner/config/loader.go
func LoadConfig(configPath string) (*runnertypes.Config, error) {
    // 設定ファイルの検証（前述のファイル検証フロー）
    if err := verificationManager.VerifyConfigFile(configPath); err != nil {
        return nil, fmt.Errorf("config file verification failed: %w", err)
    }

    // TOMLファイルの解析
    config := &runnertypes.Config{}
    if err := toml.DecodeFile(configPath, config); err != nil {
        return nil, fmt.Errorf("failed to decode config: %w", err)
    }

    return config, nil
}
```

#### Step 8: コマンドパス解決と検証

```go
// internal/verification/manager.go
func (m *Manager) VerifyCommandFile(command string) (*FileDetail, error) {
    detail := &FileDetail{
        Path: command,
    }

    start := time.Now()
    defer func() {
        detail.Duration = time.Since(start)
    }()

    // 1. パス解決（PathResolver使用）
    if m.pathResolver == nil {
        detail.Error = ErrPathResolverNotInitialized
        return detail, ErrPathResolverNotInitialized
    }

    resolvedPath, err := m.pathResolver.ResolvePath(command)
    if err != nil {
        detail.Error = err
        return detail, fmt.Errorf("path resolution failed: %w", err)
    }
    detail.ResolvedPath = resolvedPath

    // 2. コマンドセキュリティ検証
    if err := m.pathResolver.ValidateCommand(resolvedPath); err != nil {
        detail.Error = err
        return detail, fmt.Errorf("command validation failed: %w", err)
    }

    // 3. スキップ判定
    if m.shouldSkipVerification(resolvedPath) {
        detail.HashMatched = true
        return detail, nil
    }

    // 4. バイナリハッシュ検証（特権バリデータ使用）
    if err := m.fileValidator.Verify(resolvedPath); err != nil {
        detail.HashMatched = false
        detail.Error = err
        return detail, fmt.Errorf("command file verification failed: %w", err)
    }

    detail.HashMatched = true
    return detail, nil
}
```

#### Step 9: 特権コマンド実行の準備

```go
// internal/runner/executor/executor.go (仮想的な拡張実装)
func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // 1. コマンド構造の検証
    if err := e.Validate(cmd); err != nil {
        return nil, fmt.Errorf("command validation failed: %w", err)
    }

    // 2. 特権コマンドの判定
    if cmd.Privileged {
        return e.executePrivileged(ctx, cmd, envVars)
    }

    return e.executeNormal(ctx, cmd, envVars)
}

// executePrivileged handles privileged command execution
func (e *DefaultExecutor) executePrivileged(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // 特権マネージャーの可用性確認
    if e.PrivMgr == nil {
        return nil, fmt.Errorf("privileged execution requested but no privilege manager available")
    }

    if !e.PrivMgr.IsPrivilegedExecutionSupported() {
        return nil, fmt.Errorf("privileged execution not supported on this system")
    }

    // パス解決（特権付き）
    var resolvedPath string
    pathResolutionCtx := runnertypes.ElevationContext{
        Operation:   runnertypes.OperationFileAccess,
        CommandName: cmd.Name,
        FilePath:    cmd.Cmd,
    }

    err := e.PrivMgr.WithPrivileges(ctx, pathResolutionCtx, func() error {
        path, lookErr := exec.LookPath(cmd.Cmd)
        if lookErr != nil {
            return fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
        }
        resolvedPath = path
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("failed to resolve command path with privileges: %w", err)
    }

    // コマンド実行（特権付き）
    executionCtx := runnertypes.ElevationContext{
        Operation:   runnertypes.OperationCommandExecution,
        CommandName: cmd.Name,
        FilePath:    resolvedPath,
    }

    var result *Result
    err = e.PrivMgr.WithPrivileges(ctx, executionCtx, func() error {
        var execErr error
        result, execErr = e.executeCommandWithPath(ctx, resolvedPath, cmd, envVars)
        return execErr
    })

    if err != nil {
        return nil, fmt.Errorf("privileged command execution failed: %w", err)
    }

    return result, nil
}
```

### 9.4 統合実行フロー

#### Step 10: 完全な実行シーケンス

```
1. 設定ファイル読み込み要求
   ├─ 検証マネージャー初期化
   ├─ 特権マネージャー設定確認
   └─ バリデータ種別決定

2. 設定ファイル検証
   ├─ ハッシュディレクトリ検証
   ├─ ファイルハッシュ検証
   │  ├─ [特権必要時] seteuid(0)
   │  ├─ ファイル読み込み・ハッシュ計算
   │  └─ [特権必要時] seteuid(originalUID)
   └─ 設定ファイル解析

3. コマンドグループ処理
   ├─ グループ内コマンド列挙
   ├─ 依存関係解決
   └─ コマンド順次実行

4. 【個別コマンド実行フロー】
   a. コマンド前処理
      ├─ コマンド構造検証
      ├─ 特権フラグ確認
      └─ 実行方式決定

   b. コマンドバイナリ検証
      ├─ パス解決
      │  ├─ [特権コマンド] ElevationContext(OperationFileAccess)
      │  ├─ [特権コマンド] seteuid(0)
      │  ├─ exec.LookPath実行
      │  └─ [特権コマンド] seteuid(originalUID)
      ├─ セキュリティ検証
      ├─ ハッシュ検証
      │  ├─ [特権必要時] seteuid(0)
      │  ├─ バイナリハッシュ計算・比較
      │  └─ [特権必要時] seteuid(originalUID)
      └─ 実行可否判定

   c. コマンド実行
      ├─ 環境変数設定
      ├─ 作業ディレクトリ設定
      ├─ [特権コマンド] ElevationContext(OperationCommandExecution)
      ├─ [特権コマンド] seteuid(0)
      ├─ exec.CommandContext実行
      ├─ [特権コマンド] seteuid(originalUID)
      └─ 結果収集

   d. 後処理
      ├─ 実行結果ログ記録
      ├─ 権限操作監査ログ
      └─ エラーハンドリング

5. 実行完了・結果報告
```

### 9.5 実際の使用例（コマンド実行まで）

#### 設定ファイル例

```toml
# /etc/runner/config.toml
version = "1.0"

[global]
timeout = 300
workdir = "/tmp"
verify_files = ["/etc/runner/config.toml"]

[[groups]]
name = "system-maintenance"
description = "System maintenance commands requiring root privileges"

  [[groups.commands]]
  name = "mysql_backup"
  description = "Backup MySQL database as root user"
  cmd = "/usr/bin/mysqldump"
  args = ["-u", "root", "-p${MYSQL_ROOT_PASSWORD}", "--single-transaction", "production"]
  privileged = true
  timeout = 1800

  [[groups.commands]]
  name = "log_cleanup"
  description = "Clean old log files"
  cmd = "/bin/find"
  args = ["/var/log", "-name", "*.log.old", "-delete"]
  privileged = true

  [[groups.commands]]
  name = "disk_usage_check"
  description = "Check disk usage (non-privileged)"
  cmd = "/bin/df"
  args = ["-h"]
  privileged = false
```

#### 実行例とログ出力

```bash
# setuid rootバイナリとして実行
$ /usr/local/bin/go-safe-cmd-runner run -config /etc/runner/config.toml -group system-maintenance
```

**ログ出力例：**

```
# 1. 設定ファイル検証
2025/07/27 23:56:46 INFO File hash verified with privileges file_path=/etc/runner/config.toml

# 2. コマンドバイナリ検証
2025/07/27 23:56:46 INFO File hash verified with privileges file_path=/usr/bin/mysqldump
2025/07/27 23:56:46 INFO File hash verified with privileges file_path=/bin/find
2025/07/27 23:56:46 DEBUG File hash verified file_path=/bin/df

# 3. 特権コマンド実行
2025/07/27 23:56:46 INFO Privilege elevation started operation=command_execution command=mysql_backup file_path=/usr/bin/mysqldump original_uid=1000 target_uid=0
2025/07/27 23:56:48 INFO Privileged command executed successfully command=mysql_backup exit_code=0 execution_duration_ms=2340 elevation_count=2 total_privilege_duration_ms=45
2025/07/27 23:56:48 INFO Privilege restoration completed command=mysql_backup duration_ms=2

2025/07/27 23:56:48 INFO Privilege elevation started operation=command_execution command=log_cleanup file_path=/bin/find original_uid=1000 target_uid=0
2025/07/27 23:56:48 INFO Privileged command executed successfully command=log_cleanup exit_code=0 execution_duration_ms=120 elevation_count=2 total_privilege_duration_ms=8
2025/07/27 23:56:48 INFO Privilege restoration completed command=log_cleanup duration_ms=1

# 4. 通常コマンド実行
2025/07/27 23:56:48 INFO Command executed successfully command=disk_usage_check exit_code=0 execution_duration_ms=45
```

### 9.6 セキュリティ考慮事項（拡張版）

#### 9.6.1 コマンド実行時の権限管理

1. **コマンドパス解決**: 特権コマンドのパス解決は権限昇格下で実行
2. **バイナリ検証**: 実行前に必ずバイナリのハッシュ検証を実施
3. **実行時権限制御**: コマンド実行時のみ権限昇格、完了後即座に復帰
4. **環境変数隔離**: 特権実行時の環境変数は厳格に制御

#### 9.6.2 監査とコンプライアンス

```go
// 包括的な監査ログ例
type AuditEntry struct {
    Timestamp        time.Time `json:"timestamp"`
    AuditType        string    `json:"audit_type"`
    CommandName      string    `json:"command_name"`
    CommandPath      string    `json:"command_path"`
    CommandArgs      []string  `json:"command_args"`
    Privileged       bool      `json:"privileged"`
    UserID           int       `json:"user_id"`
    OriginalUID      int       `json:"original_uid"`
    ElevationCount   int       `json:"elevation_count"`
    ExecutionTime    int64     `json:"execution_time_ms"`
    PrivilegeTime    int64     `json:"privilege_time_ms"`
    ExitCode         int       `json:"exit_code"`
    Success          bool      `json:"success"`
    ErrorMessage     string    `json:"error_message,omitempty"`
}
```

### 9.7 エラーハンドリング（完全版）

#### 9.7.1 段階的エラー処理

```
Error Recovery Strategy:
├─ Configuration Phase
│  ├─ Config File Not Found → Exit with clear error
│  ├─ Config File Verification Failed → Security alert, exit
│  └─ Config Parse Error → Configuration error report
│
├─ Command Resolution Phase
│  ├─ Command Not Found → Skip command, log warning
│  ├─ Path Resolution Failed → Security check failure
│  └─ Binary Verification Failed → Security alert, halt execution
│
├─ Privilege Management Phase
│  ├─ Privilege Escalation Failed → Skip privileged commands
│  ├─ Privilege Restoration Failed → Emergency shutdown
│  └─ Platform Not Supported → Graceful degradation
│
└─ Command Execution Phase
   ├─ Command Timeout → Kill process, log timeout
   ├─ Command Exit Error → Log failure, continue with next
   └─ Unexpected Termination → Log crash, attempt cleanup
```

### 9.8 パフォーマンス最適化

#### 9.8.1 権限昇格の最適化

- **最小権限期間**: 各操作で独立した権限昇格（バッチ処理なし）
- **並列実行制限**: 特権操作は常にシリアル実行
- **キャッシュ活用**: バイナリハッシュの一時キャッシュ（セキュリティ範囲内）

#### 9.8.2 メトリクス収集

```go
type ExecutionMetrics struct {
    TotalCommands        int           `json:"total_commands"`
    PrivilegedCommands   int           `json:"privileged_commands"`
    SuccessfulCommands   int           `json:"successful_commands"`
    FailedCommands       int           `json:"failed_commands"`
    TotalExecutionTime   time.Duration `json:"total_execution_time"`
    TotalPrivilegeTime   time.Duration `json:"total_privilege_time"`
    AverageElevationTime time.Duration `json:"average_elevation_time"`
    SecurityViolations   int           `json:"security_violations"`
}
```

## 10. 結論（拡張版）

特権バリデータ・マネージャーシステムにより、設定ファイル読み込みからコマンド実行まで、以下の包括的な機能が実現される：

### 10.1 セキュリティ機能
1. **設定ファイル整合性**: 改ざん検出による信頼性確保
2. **バイナリ検証**: 実行前のコマンドバイナリ整合性確認
3. **動的権限管理**: 必要最小限の期間のみの権限昇格
4. **包括的監査**: 全段階での詳細ログ記録

### 10.2 運用機能
1. **統一インターフェース**: 特権/非特権処理の透明性
2. **柔軟な設定**: コマンド単位での権限制御
3. **堅牢なエラー処理**: 障害時の安全な停止・復旧機構
4. **性能最適化**: 権限昇格オーバーヘッドの最小化

### 10.3 コンプライアンス機能
1. **詳細監査証跡**: 規制要件に対応した完全なログ記録
2. **セキュリティアラート**: 権限異常の即座の検出・通知
3. **変更追跡**: 設定・バイナリの変更履歴管理
4. **アクセス制御**: 最小権限原則の厳格な実装

この包括的な設計により、エンタープライズ環境においても安全で監査可能なシステム管理タスクの自動化が実現される。
