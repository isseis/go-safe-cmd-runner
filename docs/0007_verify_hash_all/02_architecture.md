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
   │           │                   │               │              ├─ グループ1検証
   │           │                   │               │              │  ├─ 成功: コマンド実行
   │           │                   │               │              │  └─ 失敗: スキップ
   │           │                   │               │              │
   │           │                   │               │              ├─ グループ2検証
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

# 新規追加: global ハッシュファイル
[[global.hash_files]]
path = "/usr/bin/systemctl"

[[global.hash_files]]
path = "/usr/bin/ls"

[[global.hash_files]]
path = "/etc/ssl/certs/ca-certificates.crt"
```

#### 3.1.2 groups セクション拡張

```toml
[[groups]]
name = "system-maintenance"

# 新規追加: groups ハッシュファイル
[[groups.hash_files]]
path = "/usr/sbin/logrotate"

[[groups.hash_files]]
path = "/etc/logrotate.conf"

# 既存コマンド（自動的に検証対象に追加される）
[[groups.commands]]
cmd = "systemctl"  # PATH解決 → /usr/bin/systemctl → 検証対象
args = ["status", "nginx"]

[[groups.commands]]
cmd = "/usr/sbin/nginx"  # 絶対パス → 直接検証対象
args = ["-t"]
```

### 3.2 新規コンポーネント設計

#### 3.2.1 HashFilesConfig 構造体

```go
// internal/runner/runnertypes/types.go 拡張
type HashFile struct {
    Path string `toml:"path"` // 検証対象ファイルパス
}

type GlobalConfig struct {
    // 既存フィールド...
    Timeout     int         `toml:"timeout"`
    Workdir     string      `toml:"workdir"`
    LogLevel    string      `toml:"log_level"`

    // 新規追加
    HashFiles   []HashFile  `toml:"hash_files"`
    SkipStandardPaths bool  `toml:"skip_standard_paths"`
}

type GroupConfig struct {
    // 既存フィールド...
    Name        string      `toml:"name"`
    Commands    []Command   `toml:"commands"`

    // 新規追加
    HashFiles   []HashFile  `toml:"hash_files"`
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
func (vm *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) error
func (vm *Manager) VerifyGroupFiles(groupConfig *runnertypes.GroupConfig) error
func (vm *Manager) ResolveCommandPath(command string) (string, error)
func (vm *Manager) VerifyCommandFile(command string) error

// 内部メソッド
func (vm *Manager) verifyFileList(files []runnertypes.HashFile) error
func (vm *Manager) resolveRelativePath(path string) (string, error)
func (vm *Manager) validatePATHDirectory(dir string) error
func (vm *Manager) isStandardSystemPath(path string) bool
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
    // 1. 絶対パスの場合はそのまま返す
    if filepath.IsAbs(command) {
        return command, nil
    }

    // 2. PATH環境変数から解決
    for _, dir := range strings.Split(pr.pathEnv, ":") {
        // 3. 各PATHディレクトリのセキュリティ検証
        if err := pr.security.ValidateDirectoryPermissions(dir); err != nil {
            continue // 不安全なディレクトリはスキップ
        }

        // 4. コマンドファイルの存在確認
        fullPath := filepath.Join(dir, command)
        if exists, _ := os.Stat(fullPath); exists != nil {
            return fullPath, nil
        }
    }

    return "", fmt.Errorf("command not found in secure PATH: %s", command)
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
    // 1. 新規: グループファイル検証
    if r.verificationManager != nil {
        if err := r.verificationManager.VerifyGroupFiles(group); err != nil {
            slog.Warn("Group file verification failed, skipping group",
                "group", group.Name,
                "error", err.Error())
            return nil // エラーを返さずスキップ
        }
    }

    // 2. コマンド実行ループ
    for _, command := range group.Commands {
        // 3. 新規: コマンドパス解決と検証
        if r.verificationManager != nil {
            // パス解決
            resolvedPath, err := r.verificationManager.ResolveCommandPath(command.Cmd)
            if err != nil {
                slog.Warn("Command path resolution failed, skipping command",
                    "group", group.Name,
                    "command", command.Cmd,
                    "error", err.Error())
                continue
            }

            // 標準パススキップチェック
            if r.verificationManager.ShouldSkipVerification(resolvedPath) {
                slog.Info("Skipping verification for standard system path",
                    "group", group.Name,
                    "command", command.Cmd,
                    "resolved_path", resolvedPath)
            } else {
                // 検証実行
                if err := r.verificationManager.VerifyCommandFile(resolvedPath); err != nil {
                    slog.Warn("Command file verification failed, skipping command",
                        "group", group.Name,
                        "command", command.Cmd,
                        "resolved_path", resolvedPath,
                        "error", err.Error())
                    continue // コマンドをスキップ
                }
            }
        }

        // 4. 既存: コマンド実行
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

### 4.1 検証段階とエラーハンドリング

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

### 4.2 PATH解決のセキュリティ

```go
// セキュアなPATH解決
func (pr *PathResolver) securePathResolution(command string) (string, error) {
    pathDirs := strings.Split(os.Getenv("PATH"), ":")

    for _, dir := range pathDirs {
        // 1. ディレクトリ権限検証
        if err := pr.security.ValidateDirectoryPermissions(dir); err != nil {
            slog.Warn("Skipping insecure PATH directory",
                "directory", dir,
                "error", err.Error())
            continue
        }

        // 2. コマンドファイル確認
        fullPath := filepath.Join(dir, command)
        if fileExists(fullPath) {
            return fullPath, nil
        }
    }

    return "", fmt.Errorf("command not found in secure PATH directories: %s", command)
}
```

### 4.3 ハッシュファイル保護

既存の設定ファイル検証と同じセキュリティモデルを使用:

```
/usr/local/etc/go-safe-cmd-runner/hashes/
├── config.toml.sha256           # 設定ファイル（既存）
├── usr/
│   ├── bin/
│   │   ├── systemctl.sha256     # global ファイル
│   │   └── ls.sha256            # global ファイル
│   └── sbin/
│       └── logrotate.sha256     # groups ファイル
└── etc/
    ├── ssl/certs/
    │   └── ca-certificates.crt.sha256  # global ファイル
    └── logrotate.conf.sha256    # groups ファイル

全ファイル権限: 644 (root:root)
全ディレクトリ権限: 755 (root:root)
```

## 5. パフォーマンス設計

### 5.1 最適化戦略

- **遅延評価**: groups検証は各グループ実行直前に実行
- **キャッシュ**: 同一ファイルの重複検証を避ける
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
        HashFiles: []runnertypes.HashFile{
            {Path: "/usr/bin/ls"},
            {Path: "/usr/bin/systemctl"},
        },
    }

    // テスト実行
    vm := NewManagerWithFS(config, mockFS)
    err := vm.VerifyGlobalFiles(globalConfig)
    assert.NoError(t, err)
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
