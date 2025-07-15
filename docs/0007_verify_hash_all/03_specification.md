# 詳細仕様書: ファイル改ざん検出機能の実装

## 1. 概要

本仕様書では、ファイル改ざん検出機能の詳細な実装仕様を定義する。

## 2. データ構造定義

### 2.1 設定ファイル構造拡張

#### 2.1.1 global セクション拡張

```toml
# config.toml
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"
skip_standard_paths = false  # デフォルト: 標準パスも検証する

# 新規追加: 検証対象ファイル
verify_files = [
    "/usr/bin/systemctl",
    "/usr/bin/ls",
    "/etc/ssl/certs/ca-certificates.crt"
]
```

#### 2.1.2 groups セクション拡張

```toml
[[groups]]
name = "system-maintenance"

# 新規追加: グループ固有の検証対象ファイル
verify_files = [
    "/usr/sbin/logrotate",
    "/etc/logrotate.conf"
]

# 既存: コマンド定義（自動的に検証対象になる）
[[groups.commands]]
cmd = "systemctl"        # 相対パス → PATH解決 → /usr/bin/systemctl
args = ["status", "nginx"]

[[groups.commands]]
cmd = "/usr/sbin/nginx"  # 絶対パス → 直接検証
args = ["-t"]
```

### 2.2 Go構造体定義

#### 2.2.1 設定構造体拡張

```go
// internal/runner/runnertypes/types.go
package runnertypes

// 注意: HashFile 構造体は不要になり、配列形式で簡素化

// 既存構造体の拡張
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

// 既存（変更なし）
type Command struct {
    Cmd         string            `toml:"cmd" json:"cmd"`
    Args        []string          `toml:"args" json:"args"`
    Environment map[string]string `toml:"environment" json:"environment"`
    Timeout     int               `toml:"timeout" json:"timeout"`
}

type Config struct {
    Global GlobalConfig   `toml:"global" json:"global"`
    Groups []GroupConfig  `toml:"groups" json:"groups"`
}
```

#### 2.2.2 verification パッケージ拡張

```go
// internal/verification/manager.go
package verification

type Manager struct {
    config       Config
    fs           common.FileSystem
    validator    *filevalidator.Validator
    security     *security.Validator
    pathResolver *PathResolver // 新規追加
}

// 新規構造体
type PathResolver struct {
    pathEnv           string                  // PATH環境変数
    security          *security.Validator     // セキュリティ検証
    cache             map[string]string       // パス解決キャッシュ
    mu                sync.RWMutex           // キャッシュ保護
    skipStandardPaths bool                   // 標準パススキップフラグ
    standardPaths     []string               // スキップ対象標準パス
}

// 新規構造体: 検証結果
type VerificationResult struct {
    TotalFiles    int      `json:"total_files"`
    VerifiedFiles int      `json:"verified_files"`
    FailedFiles   []string `json:"failed_files"`
    SkippedFiles  []string `json:"skipped_files"`
    Duration      time.Duration `json:"duration"`
}

// 新規構造体: ファイル検証詳細
type FileVerificationDetail struct {
    Path           string        `json:"path"`
    ResolvedPath   string        `json:"resolved_path"`
    HashMatched    bool          `json:"hash_matched"`
    ExpectedHash   string        `json:"expected_hash"`
    ActualHash     string        `json:"actual_hash"`
    Error          error         `json:"error,omitempty"`
    Duration       time.Duration `json:"duration"`
}
```

### 2.3 ハッシュファイル命名規則

filevalidator パッケージの命名規則を踏襲：

```
/usr/local/etc/go-safe-cmd-runner/hashes/
├── config.toml.sha256               # 設定ファイル（既存）
├── usr/
│   ├── bin/
│   │   ├── systemctl.sha256         # /usr/bin/systemctl
│   │   └── ls.sha256                # /usr/bin/ls
│   └── sbin/
│       ├── logrotate.sha256         # /usr/sbin/logrotate
│       └── nginx.sha256             # /usr/sbin/nginx
├── etc/
│   ├── ssl/certs/
│   │   └── ca-certificates.crt.sha256  # /etc/ssl/certs/ca-certificates.crt
│   └── logrotate.conf.sha256        # /etc/logrotate.conf
└── manifest.json                   # filevalidator メタデータ
```

**変換ルール:**
- 絶対パス `/usr/bin/ls` → `usr/bin/ls.sha256`
- 先頭の `/` を除去
- `.sha256` 拡張子を追加
- ディレクトリ構造をそのまま維持

## 3. インターフェース仕様

### 3.1 Manager インターフェース拡張

```go
// 既存メソッド（変更なし）
func NewManager(config Config) (*Manager, error)
func NewManagerWithFS(config Config, fs common.FileSystem) (*Manager, error)
func (vm *Manager) VerifyConfigFile(configPath string) error
func (vm *Manager) ValidateHashDirectory() error
func (vm *Manager) IsEnabled() bool
func (vm *Manager) GetConfig() Config

// 新規メソッド
func (vm *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) (*VerificationResult, error)
func (vm *Manager) VerifyGroupFiles(groupConfig *runnertypes.GroupConfig) (*VerificationResult, error)
func (vm *Manager) VerifyCommandFile(command string) (*FileVerificationDetail, error)
func (vm *Manager) ResolveCommandPath(command string) (string, error)
```

### 3.2 新規メソッド詳細仕様

#### 3.2.1 VerifyGlobalFiles

```go
func (vm *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) (*VerificationResult, error)
```

**パラメータ:**
- `globalConfig`: global設定（hash_files含む）

**戻り値:**
- `*VerificationResult`: 検証結果詳細
- `error`: 致命的エラー（プロセス終了が必要）

**処理フロー:**
```
1. 有効性チェック
   ├─ 無効時: 何もしない（成功を返す）
   └─ 有効時: 以下の処理を実行

2. ファイル一覧取得
   ├─ globalConfig.VerifyFiles から対象ファイル取得
   └─ 空の場合: 成功を返す

3. 各ファイル検証
   ├─ 標準パススキップチェック
   ├─ filevalidator.Verify() 呼び出し（ハッシュ値比較のみ）
   ├─ パーミッションチェックは行わない（第三者による書き込みを許可）
   ├─ 成功: verified_files カウンタ増加
   └─ 失敗: failed_files に追加、エラー返却

4. 結果集計
   └─ VerificationResult 作成して返却
```

**エラーハンドリング:**
- **1ファイルでも失敗**: エラーを返し、プロセス終了
- **ハッシュファイル不存在**: エラーを返し、プロセス終了
- **ファイル読み込み失敗**: エラーを返し、プロセス終了

#### 3.2.2 VerifyGroupFiles

```go
func (vm *Manager) VerifyGroupFiles(groupConfig *runnertypes.GroupConfig) (*VerificationResult, error)
```

**パラメータ:**
- `groupConfig`: グループ設定（hash_files含む）

**戻り値:**
- `*VerificationResult`: 検証結果詳細
- `error`: エラー（グループスキップが必要）

**処理フロー:**
```
1. 有効性チェック
   ├─ 無効時: 何もしない（成功を返す）
   └─ 有効時: 以下の処理を実行

2. 対象ファイル収集
   ├─ groupConfig.VerifyFiles から明示的対象取得
   ├─ groupConfig.Commands から実行コマンド取得
   ├─ 各コマンドのパス解決（ResolveCommandPath使用）
   └─ 重複排除して統合リスト作成

3. 各ファイル検証
   ├─ 標準パススキップチェック
   ├─ filevalidator.Verify() 呼び出し（ハッシュ値比較のみ）
   ├─ パーミッションチェックは行わない（第三者による書き込みを許可）
   ├─ 成功: verified_files カウンタ増加
   └─ 失敗: failed_files に追加（継続処理）

4. 結果判定
   ├─ 全て成功: nil error
   └─ 1つでも失敗: error 返却（グループスキップ）
```

**エラーハンドリング:**
- **1ファイルでも失敗**: エラーを返すが、プロセスは継続（グループスキップ）
- **ハッシュファイル不存在**: エラーを返し、グループスキップ
- **コマンドパス解決失敗**: 警告ログ、当該コマンドスキップ

#### 3.2.3 VerifyCommandFile

```go
func (vm *Manager) VerifyCommandFile(command string) (*FileVerificationDetail, error)
```

**パラメータ:**
- `command`: コマンド文字列（相対パスまたは絶対パス）

**戻り値:**
- `*FileVerificationDetail`: 検証詳細
- `error`: 検証エラー（コマンドスキップが必要）

**処理フロー:**
```
1. パス解決
   ├─ 絶対パス: そのまま使用
   └─ 相対パス: ResolveCommandPath()で解決

2. 標準パススキップチェック
   ├─ ShouldSkipVerification()で判定
   ├─ スキップ対象: 成功を返す（ログ出力）
   └─ 検証対象: 次の処理に進む

3. ハッシュ検証
   ├─ filevalidator.Verify()呼び出し（ハッシュ値比較のみ）
   ├─ パーミッションチェックは行わない（第三者による書き込みを許可）
   ├─ 成功: FileVerificationDetail作成
   └─ 失敗: エラー詳細を含むFileVerificationDetail作成

4. 結果返却
   ├─ 成功: nil error
   └─ 失敗: エラー返却
```

#### 3.2.4 ResolveCommandPath

```go
func (vm *Manager) ResolveCommandPath(command string) (string, error)
```

**パラメータ:**
- `command`: コマンド名または相対パス

**戻り値:**
- `string`: 解決された絶対パス
- `error`: 解決失敗エラー

**処理フロー:**
```
1. 絶対パスチェック
   ├─ 絶対パス: そのまま返却
   └─ 相対パス: 以下の処理続行

2. キャッシュ確認
   ├─ キャッシュ存在: キャッシュから返却
   └─ キャッシュなし: 以下の処理続行

3. PATH環境変数解決
   ├─ PATH分割（":"区切り）
   ├─ 各ディレクトリのセキュリティ検証
   ├─ 不安全ディレクトリ: スキップ
   ├─ コマンドファイル存在確認
   ├─ 見つかった: キャッシュに保存して返却
   └─ 見つからない: エラー返却

4. セキュリティ検証詳細
   ├─ ディレクトリ権限チェック（security.ValidateDirectoryPermissions）
   ├─ root所有確認
   ├─ 書き込み権限チェック
   └─ 不適切: 警告ログ、ディレクトリスキップ
```

### 3.3 PathResolver 詳細仕様

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

func (pr *PathResolver) Resolve(command string) (string, error) {
    // 絶対パスはそのまま返す
    if filepath.IsAbs(command) {
        return command, nil
    }

    // キャッシュ確認
    pr.mu.RLock()
    if cached, exists := pr.cache[command]; exists {
        pr.mu.RUnlock()
        return cached, nil
    }
    pr.mu.RUnlock()

    // PATH解決
    resolved, err := pr.resolveFromPATH(command)
    if err != nil {
        return "", err
    }

    // キャッシュに保存
    pr.mu.Lock()
    pr.cache[command] = resolved
    pr.mu.Unlock()

    return resolved, nil
}

func (pr *PathResolver) resolveFromPATH(command string) (string, error) {
    pathDirs := strings.Split(pr.pathEnv, ":")

    for _, dir := range pathDirs {
        // ディレクトリセキュリティ検証
        if err := pr.security.ValidateDirectoryPermissions(dir); err != nil {
            slog.Warn("Skipping insecure PATH directory",
                "directory", dir,
                "error", err.Error())
            continue
        }

        // コマンドファイル確認
        fullPath := filepath.Join(dir, command)
        if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
            // 実行権限確認
            if info.Mode()&0111 == 0 {
                continue // 実行権限なし
            }
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

func (pr *PathResolver) ClearCache() {
    pr.mu.Lock()
    defer pr.mu.Unlock()
    pr.cache = make(map[string]string)
}
```

## 4. エラー仕様

### 4.1 エラー定義拡張

```go
// internal/verification/errors.go
package verification

import "errors"

var (
    // 既存エラー
    ErrVerificationDisabled = errors.New("verification is disabled")
    ErrHashDirectoryEmpty = errors.New("hash directory cannot be empty")
    ErrHashDirectoryInvalid = errors.New("hash directory is invalid")
    ErrConfigNil = errors.New("config cannot be nil")
    ErrSecurityValidatorNotInitialized = errors.New("security validator not initialized")

    // 新規エラー
    ErrGlobalVerificationFailed = errors.New("global file verification failed")
    ErrGroupVerificationFailed = errors.New("group file verification failed")
    ErrCommandNotFound = errors.New("command not found in secure PATH")
    ErrCommandVerificationFailed = errors.New("command file verification failed")
    ErrPathResolutionFailed = errors.New("path resolution failed")
    ErrUnsecurePATHDirectory = errors.New("PATH contains insecure directory")
)

// 構造化エラー拡張
type VerificationError struct {
    Op           string   // operation: "global", "group", "command"
    Phase        string   // "global", "group", "command"
    Path         string   // file path
    Group        string   // group name (group verification only)
    Command      string   // command name (command verification only)
    ExpectedHash string   // expected hash value
    ActualHash   string   // actual hash value
    Err          error    // underlying error
    Details      []string // failed file paths
}

func (e *VerificationError) Error() string {
    switch e.Op {
    case "global":
        return fmt.Sprintf("global verification failed for %s: %v", e.Path, e.Err)
    case "group":
        return fmt.Sprintf("group '%s' verification failed for %s: %v", e.Group, e.Path, e.Err)
    case "command":
        return fmt.Sprintf("command '%s' verification failed for %s: %v", e.Command, e.Path, e.Err)
    default:
        return fmt.Sprintf("verification failed for %s: %v", e.Path, e.Err)
    }
}

func (e *VerificationError) Unwrap() error {
    return e.Err
}
```

### 4.2 エラーハンドリングパターン

#### 4.2.1 global検証失敗

```go
// main.go
if err := verificationManager.VerifyGlobalFiles(&cfg.Global); err != nil {
    var verifyErr *verification.VerificationError
    if errors.As(err, &verifyErr) {
        slog.Error("Global file verification failed",
            "failed_files", verifyErr.Details,
            "error", err.Error())
    }
    return fmt.Errorf("global files verification failed: %w", err)
    // プロセス終了
}
```

#### 4.2.2 groups検証失敗

```go
// runner.go - executeGroup
if err := r.verificationManager.VerifyGroupFiles(group); err != nil {
    var verifyErr *verification.VerificationError
    if errors.As(err, &verifyErr) {
        slog.Warn("Group file verification failed, skipping group",
            "group", group.Name,
            "failed_files", verifyErr.Details,
            "error", err.Error())
    }
    return nil // エラーを返さずグループスキップ
}
```

#### 4.2.3 コマンド検証失敗

```go
// runner.go - executeCommand
if detail, err := r.verificationManager.VerifyCommandFile(command.Cmd); err != nil {
    slog.Warn("Command verification failed, skipping command",
        "group", group.Name,
        "command", command.Cmd,
        "resolved_path", detail.ResolvedPath,
        "error", err.Error())
    continue // コマンドスキップ、次のコマンドへ
}
```

## 5. ログ仕様

### 5.1 ログレベル定義

| レベル | 用途 | 例 |
|--------|------|---|
| DEBUG | 詳細処理ログ | パス解決、ハッシュ計算、キャッシュ操作 |
| INFO | 正常処理 | 検証完了、ファイル数、処理時間 |
| WARN | 警告事象 | グループスキップ、コマンドスキップ、不安全PATH |
| ERROR | エラー事象 | global検証失敗、ファイル不存在、権限エラー |

### 5.2 構造化ログフォーマット

#### 5.2.1 global検証ログ

```go
// 開始ログ
slog.Info("Starting global files verification",
    "total_files", len(globalConfig.HashFiles),
    "hash_directory", vm.config.HashDirectory)

// 成功ログ
slog.Info("Global files verification completed",
    "total_files", result.TotalFiles,
    "verified_files", result.VerifiedFiles,
    "duration_ms", result.Duration.Milliseconds())

// 失敗ログ
slog.Error("Global files verification failed",
    "total_files", result.TotalFiles,
    "verified_files", result.VerifiedFiles,
    "failed_files", result.FailedFiles,
    "duration_ms", result.Duration.Milliseconds())
```

#### 5.2.2 groups検証ログ

```go
// 開始ログ
slog.Info("Starting group files verification",
    "group", group.Name,
    "explicit_files", len(group.HashFiles),
    "command_files", len(commandFiles),
    "total_files", totalFiles)

// 成功ログ
slog.Info("Group files verification completed",
    "group", group.Name,
    "verified_files", result.VerifiedFiles,
    "duration_ms", result.Duration.Milliseconds())

// 失敗ログ（グループスキップ）
slog.Warn("Group files verification failed, skipping group",
    "group", group.Name,
    "verified_files", result.VerifiedFiles,
    "failed_files", result.FailedFiles,
    "duration_ms", result.Duration.Milliseconds())
```

#### 5.2.3 コマンド検証ログ

```go
// パス解決ログ
slog.Debug("Resolving command path",
    "command", command,
    "path_env", pathEnv)

slog.Debug("Command path resolved",
    "command", command,
    "resolved_path", resolvedPath,
    "cache_hit", cacheHit)

// 検証成功ログ
slog.Debug("Command verification successful",
    "command", command,
    "resolved_path", detail.ResolvedPath,
    "hash_matched", detail.HashMatched,
    "duration_ms", detail.Duration.Milliseconds())

// 検証失敗ログ（コマンドスキップ）
slog.Warn("Command verification failed, skipping command",
    "group", group.Name,
    "command", command,
    "resolved_path", detail.ResolvedPath,
    "expected_hash", detail.ExpectedHash,
    "actual_hash", detail.ActualHash,
    "error", detail.Error.Error())
```

#### 5.2.4 PATH解決ログ

```go
// 不安全PATHディレクトリ警告
slog.Warn("Skipping insecure PATH directory",
    "directory", dir,
    "permissions", fmt.Sprintf("%04o", info.Mode().Perm()),
    "owner_uid", stat.Uid,
    "owner_gid", stat.Gid,
    "error", err.Error())

// コマンド発見
slog.Debug("Command found in PATH",
    "command", command,
    "directory", dir,
    "full_path", fullPath,
    "permissions", fmt.Sprintf("%04o", info.Mode().Perm()))

// コマンド未発見
slog.Debug("Command not found in secure PATH directories",
    "command", command,
    "searched_directories", secureDirectories,
    "skipped_directories", insecureDirectories)
```

## 6. パフォーマンス仕様

### 6.1 性能要件

| 項目 | 要件 | 測定方法 |
|------|------|----------|
| global検証時間 | < 200ms (20ファイル) | time measurement |
| group検証時間 | < 100ms/group (10ファイル/group) | time measurement |
| コマンド検証時間 | < 50ms/command | time measurement |
| パス解決時間 | < 10ms/command (キャッシュ使用) | time measurement |
| メモリ使用量増加 | < 10MB | memory profiling |

### 6.2 最適化実装

#### 6.2.1 パス解決キャッシュ

```go
type PathResolver struct {
    cache       map[string]string
    mu          sync.RWMutex
    maxCacheSize int
    hitCount    int64
    missCount   int64
}

func (pr *PathResolver) GetCacheStats() (hitRate float64, size int) {
    pr.mu.RLock()
    defer pr.mu.RUnlock()

    total := pr.hitCount + pr.missCount
    if total == 0 {
        return 0, len(pr.cache)
    }

    return float64(pr.hitCount) / float64(total), len(pr.cache)
}
```

#### 6.2.2 バッチ検証

```go
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
        } else {
            result.VerifiedFiles++
        }
    }

    if len(result.FailedFiles) > 0 {
        return result, &VerificationError{
            Op: "batch",
            Details: result.FailedFiles,
            Err: ErrGroupVerificationFailed,
        }
    }

    return result, nil
}
```

### 6.3 パフォーマンステスト

```go
// internal/verification/manager_bench_test.go
func BenchmarkManager_VerifyGlobalFiles(b *testing.B) {
    // 20ファイルのglobal検証ベンチマーク
    globalConfig := createLargeGlobalConfig(20)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := vm.VerifyGlobalFiles(globalConfig)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkPathResolver_Resolve(b *testing.B) {
    // パス解決ベンチマーク（キャッシュ効果測定）
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

## 7. テスト仕様

### 7.1 ユニットテスト

#### 7.1.1 Manager拡張テスト

```go
// internal/verification/manager_test.go
func TestManager_VerifyGlobalFiles_Success(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // テストファイル作成
    files := []string{"/usr/bin/ls", "/usr/bin/systemctl", "/etc/ssl/cert.pem"}
    for _, file := range files {
        content := fmt.Sprintf("content of %s", file)
        mockFS.AddFile(file, 0755, []byte(content))

        // ハッシュファイル作成
        hash := calculateSHA256([]byte(content))
        hashPath := vm.getHashFilePath(file)
        mockFS.AddFile(hashPath, 0644, []byte(hash))
    }

    // 設定作成
    globalConfig := &runnertypes.GlobalConfig{
        HashFiles: []runnertypes.HashFile{
            {Path: "/usr/bin/ls"},
            {Path: "/usr/bin/systemctl"},
            {Path: "/etc/ssl/cert.pem"},
        },
    }

    // テスト実行
    vm, err := NewManagerWithFS(config, mockFS)
    require.NoError(t, err)

    result, err := vm.VerifyGlobalFiles(globalConfig)
    assert.NoError(t, err)
    assert.Equal(t, 3, result.TotalFiles)
    assert.Equal(t, 3, result.VerifiedFiles)
    assert.Empty(t, result.FailedFiles)
}

func TestManager_VerifyGlobalFiles_HashMismatch(t *testing.T) {
    // ハッシュ不一致テスト（global検証失敗 → プロセス終了）
    mockFS := common.NewMockFileSystem()

    // 正しいファイルと間違ったハッシュを作成
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("correct content"))
    hashPath := vm.getHashFilePath("/usr/bin/ls")
    mockFS.AddFile(hashPath, 0644, []byte("wrong_hash"))

    globalConfig := &runnertypes.GlobalConfig{
        HashFiles: []runnertypes.HashFile{{Path: "/usr/bin/ls"}},
    }

    vm, err := NewManagerWithFS(config, mockFS)
    require.NoError(t, err)

    result, err := vm.VerifyGlobalFiles(globalConfig)
    assert.Error(t, err)
    assert.Equal(t, 1, result.TotalFiles)
    assert.Equal(t, 0, result.VerifiedFiles)
    assert.Contains(t, result.FailedFiles, "/usr/bin/ls")

    var verifyErr *VerificationError
    assert.True(t, errors.As(err, &verifyErr))
    assert.Equal(t, "global", verifyErr.Op)
}

func TestManager_VerifyGroupFiles_Success(t *testing.T) {
    // グループ検証成功テスト
    mockFS := common.NewMockFileSystem()

    // 明示的ファイル + コマンドファイル
    explicitFiles := []string{"/etc/logrotate.conf"}
    commandFiles := []string{"/usr/sbin/logrotate", "/usr/bin/systemctl"}

    allFiles := append(explicitFiles, commandFiles...)
    for _, file := range allFiles {
        content := fmt.Sprintf("content of %s", file)
        mockFS.AddFile(file, 0755, []byte(content))

        hash := calculateSHA256([]byte(content))
        hashPath := vm.getHashFilePath(file)
        mockFS.AddFile(hashPath, 0644, []byte(hash))
    }

    groupConfig := &runnertypes.GroupConfig{
        Name: "test-group",
        HashFiles: []runnertypes.HashFile{{Path: "/etc/logrotate.conf"}},
        Commands: []runnertypes.Command{
            {Cmd: "/usr/sbin/logrotate", Args: []string{"-f"}},
            {Cmd: "systemctl", Args: []string{"status"}}, // 相対パス
        },
    }

    // PATH環境変数設定
    pathResolver := NewPathResolver("/usr/bin:/usr/sbin", vm.security)
    vm.pathResolver = pathResolver

    result, err := vm.VerifyGroupFiles(groupConfig)
    assert.NoError(t, err)
    assert.Equal(t, 3, result.TotalFiles) // 明示的1 + コマンド2
    assert.Equal(t, 3, result.VerifiedFiles)
    assert.Empty(t, result.FailedFiles)
}

func TestManager_VerifyGroupFiles_PartialFailure(t *testing.T) {
    // グループ検証部分失敗テスト（グループスキップ）
    mockFS := common.NewMockFileSystem()

    // 1つ成功、1つ失敗のファイルを作成
    mockFS.AddFile("/usr/bin/good", 0755, []byte("good content"))
    mockFS.AddFile("/usr/bin/bad", 0755, []byte("bad content"))

    // good用正しいハッシュ
    goodHash := calculateSHA256([]byte("good content"))
    mockFS.AddFile(vm.getHashFilePath("/usr/bin/good"), 0644, []byte(goodHash))

    // bad用間違ったハッシュ
    mockFS.AddFile(vm.getHashFilePath("/usr/bin/bad"), 0644, []byte("wrong_hash"))

    groupConfig := &runnertypes.GroupConfig{
        Name: "test-group",
        HashFiles: []runnertypes.HashFile{
            {Path: "/usr/bin/good"},
            {Path: "/usr/bin/bad"},
        },
    }

    result, err := vm.VerifyGroupFiles(groupConfig)
    assert.Error(t, err) // グループスキップのためエラー
    assert.Equal(t, 2, result.TotalFiles)
    assert.Equal(t, 1, result.VerifiedFiles)
    assert.Contains(t, result.FailedFiles, "/usr/bin/bad")

    var verifyErr *VerificationError
    assert.True(t, errors.As(err, &verifyErr))
    assert.Equal(t, "group", verifyErr.Op)
    assert.Equal(t, "test-group", verifyErr.Group)
}
```

#### 7.1.2 PathResolver テスト

```go
// internal/verification/path_resolver_test.go
func TestPathResolver_Resolve_AbsolutePath(t *testing.T) {
    // 絶対パスはそのまま返す
    pathResolver := NewPathResolver("/usr/bin", nil)

    result, err := pathResolver.Resolve("/usr/bin/ls")
    assert.NoError(t, err)
    assert.Equal(t, "/usr/bin/ls", result)
}

func TestPathResolver_Resolve_RelativePath_Success(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // PATH環境変数設定
    mockFS.AddDir("/usr/bin", 0755)
    mockFS.AddDir("/usr/sbin", 0755)
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("ls content"))
    mockFS.AddFile("/usr/sbin/nginx", 0755, []byte("nginx content"))

    security, _ := security.NewValidatorWithFS(security.DefaultConfig(), mockFS)
    pathResolver := NewPathResolver("/usr/bin:/usr/sbin", security)

    // ls コマンド解決
    result, err := pathResolver.Resolve("ls")
    assert.NoError(t, err)
    assert.Equal(t, "/usr/bin/ls", result)

    // nginx コマンド解決
    result, err = pathResolver.Resolve("nginx")
    assert.NoError(t, err)
    assert.Equal(t, "/usr/sbin/nginx", result)
}

func TestPathResolver_Resolve_InsecurePATH(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    // 不安全なディレクトリ（world-writable）
    mockFS.AddDirWithOwner("/unsafe/bin", 0777, 0, 0) // world-writable
    mockFS.AddFile("/unsafe/bin/malicious", 0755, []byte("malicious content"))

    // 安全なディレクトリ
    mockFS.AddDir("/usr/bin", 0755)
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("safe ls content"))

    security, _ := security.NewValidatorWithFS(security.DefaultConfig(), mockFS)
    pathResolver := NewPathResolver("/unsafe/bin:/usr/bin", security)

    // 不安全なパスをスキップして安全なパスから発見
    result, err := pathResolver.Resolve("ls")
    assert.NoError(t, err)
    assert.Equal(t, "/usr/bin/ls", result)

    // 不安全なパスにしかないコマンドは発見されない
    _, err = pathResolver.Resolve("malicious")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "command not found in secure PATH")
}

func TestPathResolver_Resolve_Cache(t *testing.T) {
    mockFS := common.NewMockFileSystem()

    mockFS.AddDir("/usr/bin", 0755)
    mockFS.AddFile("/usr/bin/ls", 0755, []byte("ls content"))

    security, _ := security.NewValidatorWithFS(security.DefaultConfig(), mockFS)
    pathResolver := NewPathResolver("/usr/bin", security)

    // 初回解決
    result1, err := pathResolver.Resolve("ls")
    assert.NoError(t, err)
    assert.Equal(t, "/usr/bin/ls", result1)

    // 2回目（キャッシュから）
    result2, err := pathResolver.Resolve("ls")
    assert.NoError(t, err)
    assert.Equal(t, "/usr/bin/ls", result2)

    // キャッシュ統計確認
    hitRate, size := pathResolver.GetCacheStats()
    assert.Equal(t, 1, size)
    assert.Greater(t, hitRate, 0.0)
}
```

### 7.2 統合テスト

#### 7.2.1 エンドツーエンドテスト

```go
// internal/verification/integration_test.go
func TestEndToEnd_FullVerificationFlow(t *testing.T) {
    // 1. テスト環境セットアップ
    tempDir := t.TempDir()
    configPath := filepath.Join(tempDir, "config.toml")
    hashDir := filepath.Join(tempDir, "hashes")

    // 2. 実際のファイル作成
    testFiles := map[string]string{
        "/usr/bin/ls":      "ls binary content",
        "/usr/sbin/nginx":  "nginx binary content",
        "/etc/config.conf": "config file content",
    }

    for path, content := range testFiles {
        dir := filepath.Dir(path)
        os.MkdirAll(dir, 0755)
        os.WriteFile(path, []byte(content), 0644)
    }

    // 3. 設定ファイル作成
    configContent := `
[global]
timeout = 3600

[[global.hash_files]]
path = "/usr/bin/ls"

[[groups]]
name = "web-server"

[[groups.hash_files]]
path = "/etc/config.conf"

[[groups.commands]]
cmd = "/usr/sbin/nginx"
args = ["-t"]
`
    os.WriteFile(configPath, []byte(configContent), 0644)

    // 4. ハッシュ記録
    recorder := filevalidator.New(hashDir, sha256.New())
    for path := range testFiles {
        err := recorder.Record(path)
        require.NoError(t, err)
    }
    err := recorder.Record(configPath)
    require.NoError(t, err)

    // 5. 検証実行
    config := Config{
        Enabled:       true,
        HashDirectory: hashDir,
    }

    vm, err := NewManager(config)
    require.NoError(t, err)

    // 5.1 設定ファイル検証
    err = vm.VerifyConfigFile(configPath)
    assert.NoError(t, err)

    // 5.2 設定読み込み
    cfgLoader := config.NewLoader()
    cfg, err := cfgLoader.LoadConfig(configPath)
    require.NoError(t, err)

    // 5.3 global検証
    result, err := vm.VerifyGlobalFiles(&cfg.Global)
    assert.NoError(t, err)
    assert.Equal(t, 1, result.VerifiedFiles)

    // 5.4 groups検証
    for _, group := range cfg.Groups {
        result, err := vm.VerifyGroupFiles(&group)
        assert.NoError(t, err)
        assert.Equal(t, 2, result.VerifiedFiles) // hash_files(1) + commands(1)
    }
}

func TestEndToEnd_GlobalFailure_ProcessTermination(t *testing.T) {
    // global検証失敗 → プロセス終了のシミュレーション
    tempDir := t.TempDir()
    configPath := filepath.Join(tempDir, "config.toml")
    hashDir := filepath.Join(tempDir, "hashes")

    // 改ざんされたファイル作成
    os.MkdirAll("/usr/bin", 0755)
    os.WriteFile("/usr/bin/ls", []byte("tampered content"), 0755)

    // 正しいハッシュ記録
    os.MkdirAll(hashDir, 0755)
    originalHash := calculateSHA256([]byte("original content"))
    hashPath := filepath.Join(hashDir, "usr/bin/ls.sha256")
    os.MkdirAll(filepath.Dir(hashPath), 0755)
    os.WriteFile(hashPath, []byte(originalHash), 0644)

    // 設定ファイル作成
    configContent := `
[[global.hash_files]]
path = "/usr/bin/ls"
`
    os.WriteFile(configPath, []byte(configContent), 0644)

    // 検証実行（失敗想定）
    config := Config{
        Enabled:       true,
        HashDirectory: hashDir,
    }

    vm, err := NewManager(config)
    require.NoError(t, err)

    cfgLoader := config.NewLoader()
    cfg, err := cfgLoader.LoadConfig(configPath)
    require.NoError(t, err)

    result, err := vm.VerifyGlobalFiles(&cfg.Global)
    assert.Error(t, err)
    assert.Equal(t, 1, result.TotalFiles)
    assert.Equal(t, 0, result.VerifiedFiles)
    assert.Contains(t, result.FailedFiles, "/usr/bin/ls")

    // メイン関数相当の処理（プロセス終了）
    var verifyErr *VerificationError
    if errors.As(err, &verifyErr) {
        t.Logf("Process would terminate due to global verification failure: %v", verifyErr)
        // 実際にはos.Exit(1)が呼ばれる
    }
}
```

## 8. 互換性仕様

### 8.1 後方互換性

- 既存の設定ファイルに`hash_files`セクションがなくても正常動作
- 検証機能の有効/無効は既存と同じメカニズム（コマンドライン引数・環境変数）
- filevalidatorパッケージの既存APIを変更せず拡張

### 8.2 段階的移行

```toml
# Phase 1: 設定ファイル検証のみ（既存）
[global]
timeout = 3600

# Phase 2: globalファイル検証追加
[global]
timeout = 3600

[[global.hash_files]]
path = "/usr/bin/systemctl"

# Phase 3: groupsファイル検証追加
[[groups]]
name = "web-server"

[[groups.hash_files]]
path = "/etc/nginx/nginx.conf"

[[groups.commands]]
cmd = "nginx"
args = ["-t"]
```

### 8.3 アップグレード手順

```bash
# 1. バイナリ更新
sudo cp ./build/runner /usr/local/bin/go-safe-cmd-runner

# 2. globalファイルのハッシュ記録
sudo ./build/record -file /usr/bin/systemctl -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 3. groupsファイルのハッシュ記録
sudo ./build/record -file /etc/nginx/nginx.conf -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 4. 設定ファイル更新
sudo vi /etc/go-safe-cmd-runner/config.toml  # hash_files追加

# 5. 動作確認
./runner --config /etc/go-safe-cmd-runner/config.toml --dry-run
```

## 9. セキュリティ仕様詳細

### 9.1 PATH解決のセキュリティ

**危険な状況:**
```bash
# 攻撃者がPATHの先頭に書き込み可能ディレクトリを配置
export PATH="/tmp/malicious:/usr/bin:/usr/sbin"
echo '#!/bin/bash
# 悪意のあるコード
rm -rf /important/data' > /tmp/malicious/ls
chmod +x /tmp/malicious/ls
```

**対策実装:**
```go
func (pr *PathResolver) validatePATHSecurity() error {
    pathDirs := strings.Split(pr.pathEnv, ":")
    secureCount := 0

    for _, dir := range pathDirs {
        if err := pr.security.ValidateDirectoryPermissions(dir); err != nil {
            slog.Warn("Insecure PATH directory detected",
                "directory", dir,
                "error", err.Error())
            continue
        }
        secureCount++
    }

    if secureCount == 0 {
        return fmt.Errorf("%w: no secure directories in PATH", ErrUnsecurePATHDirectory)
    }

    return nil
}
```

### 9.2 コマンド実行前検証

```go
func (r *Runner) executeCommand(command *runnertypes.Command) error {
    // 1. コマンド検証（新規追加）
    if r.verificationManager != nil {
        detail, err := r.verificationManager.VerifyCommandFile(command.Cmd)
        if err != nil {
            slog.Warn("Command verification failed, skipping",
                "command", command.Cmd,
                "error", err.Error())
            return nil // スキップ
        }

        slog.Debug("Command verified successfully",
            "command", command.Cmd,
            "verified_path", detail.ResolvedPath)
    }

    // 2. 既存のコマンド実行ロジック
    return r.executor.Execute(command)
}
```

### 9.3 ハッシュファイル保護強化

```go
func (vm *Manager) validateHashFile(hashFilePath string) error {
    // 1. ファイル存在確認
    info, err := vm.fs.Lstat(hashFilePath)
    if err != nil {
        return fmt.Errorf("hash file not found: %w", err)
    }

    // 2. ハッシュファイルの権限確認（設定ファイルと同様）
    if err := vm.security.ValidateFilePermissions(hashFilePath); err != nil {
        return fmt.Errorf("hash file permissions invalid: %w", err)
    }

    // 3. 所有者確認（root所有）
    stat := info.Sys().(*syscall.Stat_t)
    if stat.Uid != 0 {
        return fmt.Errorf("hash file not owned by root: uid=%d", stat.Uid)
    }

    // 注意: 検証対象ファイル自体のパーミッションチェックは行わない
    // 第三者による書き込みを許可し、改ざんはハッシュ値で検出

    return nil
}
```
