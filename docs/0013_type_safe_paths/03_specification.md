# 型安全なパス検証システム - 詳細仕様書

## 1. 型定義仕様

### 1.1 ValidatedPath型

```go
// ValidatedPath represents a file path that has been validated for security
type ValidatedPath struct {
    // path stores the validated absolute path
    // This field is private to prevent direct manipulation
    path string
}
```

#### 仕様
- **不変性**: 一度作成されたら内容は変更不可
- **検証保証**: 常に有効で安全なパスを保持
- **絶対パス**: 相対パス要素は含まない
- **正規化**: シンボリックリンクは解決済み

#### 制約
- 空文字列は許可しない
- 相対パス要素（`.`, `..`）は含まない
- 存在しないパスも許可（将来作成予定のファイル用）
- プラットフォーム固有の無効文字は含まない

### 1.2 ValidationError型

```go
// PathValidationError represents errors that occur during path validation
type PathValidationError struct {
    Path       string           `json:"path"`
    Reason     ValidationReason `json:"reason"`
    Message    string           `json:"message"`
    Underlying error            `json:"underlying,omitempty"`
}

func (e *PathValidationError) Error() string {
    if e.Underlying != nil {
        return fmt.Sprintf("path validation failed for %q: %s (%v)",
            e.Path, e.Message, e.Underlying)
    }
    return fmt.Sprintf("path validation failed for %q: %s", e.Path, e.Message)
}

func (e *PathValidationError) Unwrap() error {
    return e.Underlying
}
```

### 1.3 ValidationReason列挙型

```go
type ValidationReason int

const (
    ReasonUnknown ValidationReason = iota
    ReasonEmpty
    ReasonTooLong
    ReasonInvalidCharacters
    ReasonNotAbsolute
    ReasonContainsRelativeElements
    ReasonSymlinkLoop
    ReasonNotRegularFile
    ReasonPermissionDenied
    ReasonNotExists
    ReasonNotReadable
    ReasonNotWritable
    ReasonNotExecutable
)

var reasonMessages = map[ValidationReason]string{
    ReasonEmpty:                    "path is empty",
    ReasonTooLong:                  "path exceeds maximum length",
    ReasonInvalidCharacters:        "path contains invalid characters",
    ReasonNotAbsolute:             "path is not absolute",
    ReasonContainsRelativeElements: "path contains relative elements (. or ..)",
    ReasonSymlinkLoop:             "symlink loop detected",
    ReasonNotRegularFile:          "path does not point to a regular file",
    ReasonPermissionDenied:        "permission denied",
    ReasonNotExists:               "path does not exist",
    ReasonNotReadable:             "path is not readable",
    ReasonNotWritable:             "path is not writable",
    ReasonNotExecutable:           "path is not executable",
}
```

## 2. API仕様

### 2.1 パス検証API

#### 2.1.1 基本検証関数

```go
// ValidatePath validates a file path and returns a ValidatedPath
func ValidatePath(path string) (ValidatedPath, error)
```

**仕様**:
- 空パスは `ReasonEmpty` エラー
- 相対パスは `ReasonNotAbsolute` エラー
- 無効文字は `ReasonInvalidCharacters` エラー
- シンボリックリンクは解決される
- 成功時は正規化された絶対パスを返す

**例**:
```go
// 成功例
validated, err := ValidatePath("/tmp/test.txt")
// validated.String() -> "/tmp/test.txt"

// エラー例
_, err := ValidatePath("../test.txt")
// err -> PathValidationError{Reason: ReasonNotAbsolute, ...}
```

#### 2.1.2 オプション付き検証関数

```go
// ValidationOptions specifies additional validation requirements
type ValidationOptions struct {
    RequireExists     bool `json:"require_exists"`
    RequireRegular    bool `json:"require_regular"`
    RequireReadable   bool `json:"require_readable"`
    RequireWritable   bool `json:"require_writable"`
    RequireExecutable bool `json:"require_executable"`
    MaxLength         int  `json:"max_length"`
}

// ValidatePathWithOptions validates a path with additional requirements
func ValidatePathWithOptions(path string, opts ValidationOptions) (ValidatedPath, error)
```

**仕様**:
- `RequireExists`: ファイルの存在を確認
- `RequireRegular`: 通常ファイルであることを確認
- `RequireReadable`: 読み取り権限を確認
- `RequireWritable`: 書き込み権限を確認
- `RequireExecutable`: 実行権限を確認
- `MaxLength`: パス長の上限を指定

### 2.2 ValidatedPathメソッド

#### 2.2.1 基本操作

```go
// String returns the validated path as a string
func (p ValidatedPath) String() string

// IsEmpty returns true if the path is empty (zero value)
func (p ValidatedPath) IsEmpty() bool

// Equals compares two ValidatedPath instances
func (p ValidatedPath) Equals(other ValidatedPath) bool
```

#### 2.2.2 パス操作

```go
// Join safely joins the validated path with additional elements
func (p ValidatedPath) Join(elem ...string) (ValidatedPath, error)

// Dir returns the directory portion of the path
func (p ValidatedPath) Dir() ValidatedPath

// Base returns the final element of the path
func (p ValidatedPath) Base() string

// Ext returns the file extension
func (p ValidatedPath) Ext() string
```

**Join仕様**:
- 追加要素は相対パスとして扱う
- 結果は再検証される
- 相対パス要素（`.`, `..`）は拒否される

**例**:
```go
base, _ := ValidatePath("/tmp")
result, err := base.Join("subdir", "file.txt")
// result.String() -> "/tmp/subdir/file.txt"

// エラー例
_, err := base.Join("../escape")
// err -> PathValidationError{Reason: ReasonContainsRelativeElements, ...}
```

#### 2.2.3 ファイル情報

```go
// Stat returns file information for the validated path
func (p ValidatedPath) Stat() (os.FileInfo, error)

// Exists checks if the file exists
func (p ValidatedPath) Exists() bool

// IsRegular checks if the path points to a regular file
func (p ValidatedPath) IsRegular() (bool, error)

// IsReadable checks if the file is readable
func (p ValidatedPath) IsReadable() bool

// IsWritable checks if the file is writable
func (p ValidatedPath) IsWritable() bool

// IsExecutable checks if the file is executable
func (p ValidatedPath) IsExecutable() bool
```

### 2.3 型安全ファイル操作API

#### 2.3.1 読み取り操作

```go
// SafeReadFile reads the content of a validated file path
func SafeReadFile(path ValidatedPath) ([]byte, error)

// SafeOpenFile opens a file at the validated path
func SafeOpenFile(path ValidatedPath, flag int, perm os.FileMode) (*os.File, error)
```

#### 2.3.2 書き込み操作

```go
// SafeWriteFile writes data to a validated file path
func SafeWriteFile(path ValidatedPath, data []byte, perm os.FileMode) error

// SafeWriteFileOverwrite writes data with overwrite capability
func SafeWriteFileOverwrite(path ValidatedPath, data []byte, perm os.FileMode) error
```

### 2.4 互換性API

#### 2.4.1 レガシーサポート

```go
// FromUnsafePath converts an unsafe string path to ValidatedPath
// Deprecated: Use ValidatePath instead
func FromUnsafePath(path string) (ValidatedPath, error)

// ToUnsafePath converts ValidatedPath to string
// Deprecated: Use ValidatedPath.String() instead
func ToUnsafePath(path ValidatedPath) string
```

#### 2.4.2 移行ヘルパー

```go
// MustValidatePath validates a path and panics on error
// Use only for static paths known to be valid
func MustValidatePath(path string) ValidatedPath

// ValidateOrDefault validates a path or returns a default
func ValidateOrDefault(path string, defaultPath ValidatedPath) ValidatedPath
```

## 3. 検証ロジック仕様

### 3.1 検証ステップ

1. **Nullチェック**: 空文字列の検出
2. **長さチェック**: プラットフォーム固有の最大長
3. **文字チェック**: 無効文字の検出
4. **絶対パスチェック**: 相対パスの拒否
5. **正規化**: 絶対パスへの変換
6. **シンボリックリンク解決**: `filepath.EvalSymlinks`
7. **要素チェック**: `.`, `..` の検出
8. **追加検証**: オプション要件の確認

### 3.2 プラットフォーム固有の検証

#### Unix系システム
```go
func validateUnixPath(path string) error {
    // NUL文字の検出
    if strings.Contains(path, "\x00") {
        return &PathValidationError{
            Path:    path,
            Reason:  ReasonInvalidCharacters,
            Message: "path contains NUL character",
        }
    }
    return nil
}
```

#### Windows
```go
func validateWindowsPath(path string) error {
    // 無効文字の検出
    invalidChars := []rune{'<', '>', ':', '"', '|', '?', '*'}
    for _, char := range path {
        for _, invalid := range invalidChars {
            if char == invalid {
                return &PathValidationError{
                    Path:    path,
                    Reason:  ReasonInvalidCharacters,
                    Message: fmt.Sprintf("path contains invalid character: %c", char),
                }
            }
        }
    }
    return nil
}
```

### 3.3 シンボリックリンク処理

```go
func resolveSymlinks(path string) (string, error) {
    resolved, err := filepath.EvalSymlinks(path)
    if err != nil {
        // ファイルが存在しない場合は親ディレクトリまで解決
        if os.IsNotExist(err) {
            dir := filepath.Dir(path)
            resolvedDir, dirErr := filepath.EvalSymlinks(dir)
            if dirErr != nil {
                return "", &PathValidationError{
                    Path:       path,
                    Reason:     ReasonSymlinkLoop,
                    Message:    "failed to resolve symlinks",
                    Underlying: err,
                }
            }
            return filepath.Join(resolvedDir, filepath.Base(path)), nil
        }

        return "", &PathValidationError{
            Path:       path,
            Reason:     ReasonSymlinkLoop,
            Message:    "symlink resolution failed",
            Underlying: err,
        }
    }
    return resolved, nil
}
```

## 4. エラーハンドリング仕様

### 4.1 エラー分類

#### 4.1.1 入力エラー
- `ReasonEmpty`: 空パス
- `ReasonInvalidCharacters`: 無効文字
- `ReasonNotAbsolute`: 相対パス

#### 4.1.2 ファイルシステムエラー
- `ReasonNotExists`: ファイル不存在
- `ReasonPermissionDenied`: 権限不足
- `ReasonSymlinkLoop`: シンボリックリンクループ

#### 4.1.3 システムエラー
- `ReasonTooLong`: パス長超過
- `ReasonNotRegularFile`: 非通常ファイル

### 4.2 エラーメッセージ仕様

```go
func (r ValidationReason) String() string {
    if msg, ok := reasonMessages[r]; ok {
        return msg
    }
    return "unknown validation error"
}

func formatValidationError(path string, reason ValidationReason, underlying error) error {
    return &PathValidationError{
        Path:       path,
        Reason:     reason,
        Message:    reason.String(),
        Underlying: underlying,
    }
}
```

### 4.3 エラー復旧仕様

```go
// RecoverablePath attempts to fix common path issues
func RecoverablePath(path string) (ValidatedPath, error) {
    // 共通の問題の自動修正試行

    // 1. 前後の空白を除去
    path = strings.TrimSpace(path)

    // 2. バックスラッシュをスラッシュに変換（Windows互換性）
    path = filepath.ToSlash(path)

    // 3. 重複スラッシュを除去
    path = filepath.Clean(path)

    // 4. 相対パスを絶対パスに変換
    if !filepath.IsAbs(path) {
        abs, err := filepath.Abs(path)
        if err != nil {
            return ValidatedPath{}, formatValidationError(path, ReasonNotAbsolute, err)
        }
        path = abs
    }

    // 5. 通常の検証を実行
    return ValidatePath(path)
}
```

## 5. パフォーマンス仕様

### 5.1 性能目標

| 操作 | 目標時間 | メモリ使用量 |
|------|----------|--------------|
| 基本検証 | < 100μs | < 256 bytes |
| シンボリックリンク解決 | < 1ms | < 512 bytes |
| パス結合 | < 50μs | < 128 bytes |
| ファイル存在確認 | < 500μs | < 64 bytes |

### 5.2 最適化実装

#### 5.2.1 文字列プール

```go
var pathPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 256)
    },
}

func optimizedPathValidation(path string) (ValidatedPath, error) {
    buf := pathPool.Get().([]byte)
    defer pathPool.Put(buf[:0])

    // バッファを使用した最適化処理
    // ...
}
```

#### 5.2.2 キャッシュ機能

```go
type ValidationCache struct {
    cache map[string]ValidatedPath
    mutex sync.RWMutex
    maxSize int
}

func (c *ValidationCache) Get(path string) (ValidatedPath, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()

    validated, exists := c.cache[path]
    return validated, exists
}
```

### 5.3 ベンチマーク仕様

```go
func BenchmarkPathValidation(b *testing.B) {
    testPaths := []string{
        "/tmp/test.txt",
        "/usr/bin/ls",
        "/var/log/system.log",
        // ... more test paths
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        path := testPaths[i%len(testPaths)]
        _, err := ValidatePath(path)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

## 6. セキュリティ仕様

### 6.1 脅威対策

#### 6.1.1 パストラバーサル攻撃
```go
// 相対パス要素の検出と拒否
func detectPathTraversal(path string) error {
    if strings.Contains(path, "..") {
        return formatValidationError(path, ReasonContainsRelativeElements, nil)
    }

    // 正規化後の再チェック
    clean := filepath.Clean(path)
    if clean != path {
        return formatValidationError(path, ReasonContainsRelativeElements, nil)
    }

    return nil
}
```

#### 6.1.2 シンボリックリンク攻撃
```go
// 解決深度の制限
const maxSymlinkDepth = 8

func safeEvalSymlinks(path string) (string, error) {
    return evalSymlinksWithDepth(path, 0)
}

func evalSymlinksWithDepth(path string, depth int) (string, error) {
    if depth > maxSymlinkDepth {
        return "", formatValidationError(path, ReasonSymlinkLoop, nil)
    }

    // シンボリックリンク解決の実装
    // ...
}
```

### 6.2 監査ログ仕様

```go
type SecurityEvent struct {
    Timestamp time.Time           `json:"timestamp"`
    EventType SecurityEventType   `json:"event_type"`
    Path      string              `json:"path"`
    Reason    ValidationReason    `json:"reason,omitempty"`
    UserID    string              `json:"user_id,omitempty"`
    ProcessID int                 `json:"process_id,omitempty"`
}

type SecurityEventType string

const (
    EventValidationFailed SecurityEventType = "validation_failed"
    EventSuspiciousPath   SecurityEventType = "suspicious_path"
    EventAccessDenied     SecurityEventType = "access_denied"
)
```

## 7. テスト仕様

### 7.1 単体テスト

```go
func TestValidatePath(t *testing.T) {
    tests := []struct {
        name        string
        input       string
        expectError bool
        errorReason ValidationReason
    }{
        {
            name:        "valid absolute path",
            input:       "/tmp/test.txt",
            expectError: false,
        },
        {
            name:        "empty path",
            input:       "",
            expectError: true,
            errorReason: ReasonEmpty,
        },
        {
            name:        "relative path",
            input:       "test.txt",
            expectError: true,
            errorReason: ReasonNotAbsolute,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := ValidatePath(tt.input)

            if tt.expectError {
                assert.Error(t, err)
                var pathErr *PathValidationError
                assert.True(t, errors.As(err, &pathErr))
                assert.Equal(t, tt.errorReason, pathErr.Reason)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.input, result.String())
            }
        })
    }
}
```

### 7.2 統合テスト

```go
func TestFileOperationsWithValidatedPaths(t *testing.T) {
    // 一時ディレクトリの作成
    tmpDir := t.TempDir()

    // テストファイルの作成
    testFile := filepath.Join(tmpDir, "test.txt")
    err := os.WriteFile(testFile, []byte("test content"), 0644)
    require.NoError(t, err)

    // ValidatedPathでの操作テスト
    validated, err := ValidatePath(testFile)
    require.NoError(t, err)

    // ファイル読み取りテスト
    content, err := SafeReadFile(validated)
    require.NoError(t, err)
    assert.Equal(t, "test content", string(content))
}
```

### 7.3 セキュリティテスト

```go
func TestSecurityVulnerabilities(t *testing.T) {
    maliciousPaths := []string{
        "../../../etc/passwd",
        "/tmp/../../../etc/passwd",
        "..\\..\\..\\windows\\system32\\config\\sam",
        "/proc/self/mem",
        "\x00/tmp/test.txt",
    }

    for _, path := range maliciousPaths {
        t.Run(fmt.Sprintf("malicious_path_%s", path), func(t *testing.T) {
            _, err := ValidatePath(path)
            assert.Error(t, err, "malicious path should be rejected: %s", path)
        })
    }
}
```

## 8. 移行仕様

### 8.1 段階的移行計画

#### Phase 1: 型定義と基本機能
- [ ] `ValidatedPath` 型の定義
- [ ] 基本検証関数の実装
- [ ] 単体テストの作成

#### Phase 2: 拡張機能
- [ ] オプション付き検証の実装
- [ ] パス操作メソッドの追加
- [ ] 統合テストの作成

#### Phase 3: 型安全ファイル操作
- [ ] `SafeReadFile`, `SafeWriteFile` の実装
- [ ] 既存 `safefileio` パッケージとの統合
- [ ] パフォーマンステストの実施

#### Phase 4: 既存コードの更新
- [ ] `filevalidator` パッケージの型安全化
- [ ] `executor` パッケージの更新
- [ ] 後方互換性の確保

### 8.2 移行ツール

```go
// 自動移行ツール用の解析関数
func AnalyzeCodeForMigration(packagePath string) (*MigrationReport, error) {
    // Go ASTを使用してstring型のファイルパス使用箇所を検出
    // ValidatedPath型への移行候補を特定
    // 移行の優先度と影響範囲を分析
}

type MigrationReport struct {
    TotalOccurrences int
    SafeToMigrate    []Location
    RequiresReview   []Location
    Blockers         []Location
}
```

## 9. 互換性仕様

### 9.1 Go バージョン互換性
- 最小要求: Go 1.19
- 推奨バージョン: Go 1.21+
- テスト対象: Go 1.19, 1.20, 1.21, 1.22

### 9.2 プラットフォーム互換性
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)
- FreeBSD (amd64)

### 9.3 依存関係
- 標準ライブラリのみ使用
- 外部依存なし
- CGO不要
