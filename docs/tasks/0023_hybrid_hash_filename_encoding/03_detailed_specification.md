# 詳細仕様書：ハイブリッドハッシュファイル名エンコーディング

## 1. 実装概要 (Implementation Overview)

### 1.1. 実装対象コンポーネント

本仕様書は以下のコンポーネントの詳細実装仕様を定義する：

- `SubstitutionHashEscape`: 換字+ダブルエスケープエンコーダー（シンプル化）
- エラーハンドリングとデータ型
- 関連するテストスイート

**注意**: 分析機能、HybridHashFilePathGetter、MigrationHashFilePathGetterは当面実装しない

### 1.2. ファイル配置

```
internal/filevalidator/
└── encoding/
    ├── substitution_hash_escape.go      # メインエンコーダー
    ├── substitution_hash_escape_test.go # ユニットテスト
    ├── encoding_result.go               # 結果構造体
    └── errors.go                        # エラータイプ定義
```

## 2. SubstitutionHashEscape 詳細仕様

### 2.1. 構造体定義

```go
package encoding

import (
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "path/filepath"
    "strings"
)

const (
    // MaxFilenameLength defines the maximum allowed filename length (NAME_MAX - safety margin)
    MaxFilenameLength = 250
    // HashLength defines the number of characters to use from SHA256 hash
    HashLength = 12
)

// SubstitutionHashEscape implements hybrid substitution + double escape encoding
type SubstitutionHashEscape struct{}

// NewSubstitutionHashEscape creates a new encoder
func NewSubstitutionHashEscape() *SubstitutionHashEscape {
    return &SubstitutionHashEscape{}
}
```

### 2.2. エンコーディング実装

#### 2.2.1 基本エンコード関数

```go
// Encode encodes a file path using substitution + double escape method.
// The path will be converted to an absolute, normalized path.
// Returns the encoded filename (without directory path).
func (e *SubstitutionHashEscape) Encode(path string) (string, error) {
    if path == "" {
        return "", ErrInvalidPath{Path: path, Err: ErrEmptyPath}
    }

    // Ensure path is absolute and canonical
    if !filepath.IsAbs(path) {
        return "", ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
    }
    if filepath.Clean(path) != path {
        return "", ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
    }
    // Single-pass encoding optimization
    return e.encodeOptimized(path), nil
}

// encodeOptimized performs single-pass encoding combining substitution and double escape
func (e *SubstitutionHashEscape) encodeOptimized(path string) string {
    var builder strings.Builder
    // Pre-allocate: typical expansion is minimal, but allow for some escaping
    // Most characters (especially /) don't expand, only ~ and # expand to 2 chars
    builder.Grow(len(path) + len(path)/10) // +10% buffer for typical cases

    for _, char := range path {
        switch char {
        case '/':
            // Combined: / → ~ (substitution step result has no / to double escape)
            builder.WriteRune('~')
        case '~':
            // Combined: ~ → ##
            builder.WriteString("##")
        case '#':
            // Combined: # → #1
            builder.WriteString("#1")
        default:
            // No substitution or escaping needed
            builder.WriteRune(char)
        }
    }

    return builder.String()
}
```

#### 2.2.2 デコード実装

```go
// Decode decodes an encoded filename back to original absolute file path.
// Only absolute paths are supported as inputs during encoding.
func (e *SubstitutionHashEscape) Decode(encoded string) (string, error) {
    if encoded == "" {
        return "", nil
    }

    // Check if this is a fallback format (not start with ~)
    if len(encoded) > 0 && encoded[0] != '~' {
        return "", ErrFallbackNotReversible{EncodedName: encoded}
    }

    // Single-pass decoding optimization
    result := e.decodeOptimized(encoded)

    return result, nil
}

// decodeOptimized performs single-pass decoding combining reverse double escape and substitution
func (e *SubstitutionHashEscape) decodeOptimized(encoded string) string {
    var builder strings.Builder
    // Pre-allocate: decoded result is always <= encoded length
    builder.Grow(len(encoded))

    runes := []rune(encoded)
    for i := 0; i < len(runes); i++ {
        char := runes[i]

        switch char {
        case '#':
            // Check for escape sequences: ## → ~ or #1 → #
            if i+1 < len(runes) {
                next := runes[i+1]
                switch next {
                case '#':
                    // Combined: ## → ~
                    builder.WriteRune('~')
                    i++ // Skip next character
                case '1':
                    // Combined: #1 → #
                    builder.WriteRune('#')
                    i++ // Skip next character
                default:
                    // Single # without escape sequence (shouldn't happen in valid encoding)
                    builder.WriteRune(char)
                }
            } else {
                // Single # at end (shouldn't happen in valid encoding)
                builder.WriteRune(char)
            }
        case '~':
            // ~ → / (reverse substitution)
            builder.WriteRune('/')
        default:
            // No decoding needed
            builder.WriteRune(char)
        }
    }

    return builder.String()
}
```

#### 2.2.3 ハイブリッド実装（フォールバック対応）

```go
// EncodeWithFallback encodes a path with automatic fallback to SHA256 for long paths.
// The path will be converted to an absolute, normalized path.
func (e *SubstitutionHashEscape) EncodeWithFallback(path string) (Result, error) {
    if path == "" {
        return Result{}, ErrInvalidPath{Path: path, Err: ErrEmptyPath}
    }
    // Ensure path is absolute and canonical
    if !filepath.IsAbs(path) {
        return Result{}, ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
    }
    if filepath.Clean(path) != path {
        return Result{}, ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
    }

    // Convert to absolute path first for consistent path handling
    absPath, err := filepath.Abs(path)
    if err != nil {
        return Result{}, err
    }

    // Try normal encoding
    normalEncoded, err := e.Encode(absPath)
    if err != nil {
        return Result{}, err
    }

    // Check length constraint
    if len(normalEncoded) <= MaxFilenameLength {
        return Result{
            EncodedName:    normalEncoded,
            IsFallback:     false,
            OriginalLength: len(absPath),
            EncodedLength:  len(normalEncoded),
        }, nil
    }

    // Use SHA256 fallback for long paths (always enabled)
    fallbackEncoded := e.generateSHA256Fallback(absPath)

    return Result{
        EncodedName:    fallbackEncoded,
        IsFallback:     true,
        OriginalLength: len(absPath),
        EncodedLength:  len(fallbackEncoded),
    }, nil
}

// generateSHA256Fallback generates SHA256-based filename for long paths
func (e *SubstitutionHashEscape) generateSHA256Fallback(path string) string {
    hash := sha256.Sum256([]byte(path))
    hashStr := base64.URLEncoding.EncodeToString(hash[:])

    // Use default hash length, ensure it fits within limits
    hashLength := min(HashLength, len(hashStr))

    // Format: {hash}.json (hashLength + 5 characters)
    return hashStr[:hashLength] + ".json"
}
```

### 2.3. 判定機能

```go
// IsNormalEncoding determines if an encoded filename uses normal encoding
func (e *SubstitutionHashEscape) IsNormalEncoding(encoded string) bool {
    if len(encoded) == 0 {
        return false
    }

    // Normal encoding always starts with ~ (since all full paths start with /)
    return encoded[0] == '~'
}

// IsFallbackEncoding determines if an encoded filename uses SHA256 fallback
func (e *SubstitutionHashEscape) IsFallbackEncoding(encoded string) bool {
    if len(encoded) == 0 {
        return false
    }

    return !e.IsNormalEncoding(encoded)
}
```

## 3. データ構造定義

### 3.1. 結果構造体

```go
// Result represents the result of an encoding operation
type Result struct {
    EncodedName    string // The encoded filename
    IsFallback     bool   // Whether SHA256 fallback was used
    OriginalLength int    // Length of original path
    EncodedLength  int    // Length of encoded filename
}
```

### 3.2. エラータイプ定義

```go
// Static errors for common invalid path cases
var (
    // ErrEmptyPath indicates an empty path was provided
    ErrEmptyPath = errors.New("empty path")
    // ErrNotAbsoluteOrNormalized indicates the path is not absolute or normalized
    ErrNotAbsoluteOrNormalized = errors.New("path is not absolute or normalized")
)

// ErrInvalidPath represents an error for invalid file paths during encoding operations
type ErrInvalidPath struct {
    Path string // The invalid path
    Err  error  // The underlying error, if any
}

func (e ErrInvalidPath) Error() string {
    return fmt.Sprintf("invalid path: %s (error: %v)", e.Path, e.Err)
}

func (e *ErrInvalidPath) Unwrap() error {
    return e.Err
}

// ErrFallbackNotReversible indicates a fallback encoding cannot be decoded
type ErrFallbackNotReversible struct {
    EncodedName string
}

func (e ErrFallbackNotReversible) Error() string {
    return fmt.Sprintf("fallback encoding '%s' cannot be decoded to original path", e.EncodedName)
}
```

## 4. テスト実装仕様

**注意**: HybridHashFilePathGetter と MigrationHashFilePathGetter は当面実装しないため、該当セクションは削除

### 4.1. ユニットテスト

```go
// substitution_hash_escape_test.go
package encoding

import (
    "strings"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSubstitutionHashEscape_Encode(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    tests := []struct {
        name        string
        input       string
        expected    string
        expectError bool
    }{
        {
            name:        "simple absolute path",
            input:       "/usr/bin/python3",
            expected:    "~usr~bin~python3",
            expectError: false,
        },
        {
            name:        "path with hash character",
            input:       "/home/user#test/file",
            expected:    "~home~user#1test~file",
            expectError: false,
        },
        {
            name:        "path with tilde character",
            input:       "/home/~user/file",
            expected:    "~home~##user~file",
            expectError: false,
        },
        {
            name:        "complex path",
            input:       "/path/with#hash/and~tilde/file",
            expected:    "~path~with#1hash~and##tilde~file",
            expectError: false,
        },
        {
            name:        "empty path",
            input:       "",
            expected:    "",
            expectError: true, // ErrEmptyPath
        },
        {
            name:        "root path",
            input:       "/",
            expected:    "~",
            expectError: false,
        },
        {
            name:        "relative path should error",
            input:       "usr/bin/python3",
            expected:    "",
            expectError: true, // ErrNotAbsoluteOrNormalized
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := encoder.Encode(tt.input)

            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}

func TestSubstitutionHashEscape_Decode(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "simple encoded path",
            input:    "~usr~bin~python3",
            expected: "/usr/bin/python3",
            wantErr:  false,
        },
        {
            name:     "fallback format",
            input:    "AbCdEf123456.json",
            expected: "",
            wantErr:  true,
        },
        {
            name:     "complex encoded path",
            input:    "~path~with#1hash~and##tilde~file",
            expected: "/path/with#hash/and~tilde/file",
            wantErr:  false,
        },
        {
            name:     "empty input",
            input:    "",
            expected: "",
            wantErr:  false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := encoder.Decode(tt.input)

            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}

func TestSubstitutionHashEscape_RoundTrip(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    // Property-based test: encode then decode should return original
    testPaths := []string{
        "/usr/bin/python3",
        "/home/user_name/project_files",
        "/path/with#special/chars~here",
        "/very/deep/nested/directory/structure/file.txt",
        "/",
        "/single",
    }

    for _, originalPath := range testPaths {
        t.Run(originalPath, func(t *testing.T) {
            // Encode
            encoded, err := encoder.Encode(originalPath)
            require.NoError(t, err)

            // Decode
            decoded, err := encoder.Decode(encoded)

            // Verify round-trip
            require.NoError(t, err)
            assert.Equal(t, originalPath, decoded, "Round-trip failed for path: %s", originalPath)
        })
    }
}

func TestSubstitutionHashEscape_NameMaxFallback(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    tests := []struct {
        name         string
        path         string
        wantFallback bool
    }{
        {
            name:         "short path uses normal encoding",
            path:         "/usr/bin/python3",
            wantFallback: false,
        },
        {
            name:         "very long path uses fallback",
            path:         "/" + strings.Repeat("very-long-directory-name", 10) + "/file.txt",
            wantFallback: true,
        },
        {
            name:         "edge case near limit",
            path:         "/" + strings.Repeat("a", 248) + "/f", // Should encode to 251 chars
            wantFallback: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := encoder.EncodeWithFallback(tt.path)
            assert.NoError(t, err)

            assert.Equal(t, tt.wantFallback, result.IsFallback)

            if result.IsFallback {
                // Fallback should not start with `~`
                assert.NotEqual(t, '~', result.EncodedName[0])
                // Fallback should be within length limits
                assert.LessOrEqual(t, len(result.EncodedName), MaxFilenameLength)
            } else {
                // Normal encoding should start with `~` (for full paths)
                assert.Equal(t, '~', result.EncodedName[0])
                // Should be reversible
                decoded, err := encoder.Decode(result.EncodedName)
                assert.NoError(t, err)
                assert.Equal(t, tt.path, decoded)
            }
        })
    }
}
```

### 6.2. プロパティベーステスト

```go
// property_test.go
//go:build property

package encoding

import (
    "testing"
    "testing/quick"
    "unicode/utf8"
    "github.com/stretchr/testify/assert"
)

func TestProperty_EncodeDecode_Reversibility(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    // Property: For any valid path that results in normal encoding,
    // encode(decode(path)) == path
    property := func(path string) bool {
        // Skip invalid UTF-8 strings
        if !utf8.ValidString(path) {
            return true
        }

        // Skip paths that would use fallback
        result, err := encoder.EncodeWithFallback(path)
        if err != nil || result.IsFallback {
            return true // Skip error and fallback cases for this test
        }

        // Test reversibility
        decoded, err := encoder.Decode(result.EncodedName)
        return err == nil && decoded == path
    }

    err := quick.Check(property, &quick.Config{MaxCount: 1000})
    assert.NoError(t, err, "Reversibility property failed")
}

func TestProperty_Encode_Deterministic(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    // Property: encode(path) should always return the same result
    property := func(path string) bool {
        if !utf8.ValidString(path) {
            return true
        }

        encoded1, err1 := encoder.Encode(path)
        encoded2, err2 := encoder.Encode(path)

        if err1 != nil || err2 != nil {
            return true // Skip error cases
        }

        return encoded1 == encoded2
    }

    err := quick.Check(property, &quick.Config{MaxCount: 1000})
    assert.NoError(t, err, "Deterministic property failed")
}

func TestProperty_Encode_UniqueOutput(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    // Property: Different paths should produce different encoded names
    // (except when fallback is used)
    property := func(path1, path2 string) bool {
        if !utf8.ValidString(path1) || !utf8.ValidString(path2) {
            return true
        }

        if path1 == path2 {
            return true // Same input, same output is expected
        }

        result1, err1 := encoder.EncodeWithFallback(path1)
        result2, err2 := encoder.EncodeWithFallback(path2)

        if err1 != nil || err2 != nil {
            return true // Skip error cases
        }

        // If both use normal encoding, they should be different
        if !result1.IsFallback && !result2.IsFallback {
            return result1.EncodedName != result2.EncodedName
        }

        return true // Skip fallback cases for uniqueness test
    }

    err := quick.Check(property, &quick.Config{MaxCount: 500})
    assert.NoError(t, err, "Uniqueness property failed")
}
```

### 6.3. ベンチマークテスト

```go
// benchmark_encoding_test.go
package encoding

import (
    "strings"
    "testing"
)

func BenchmarkSubstitutionHashEscape_Encode(b *testing.B) {
    encoder := NewSubstitutionHashEscape()
    testPath := "/home/user/project/src/main/java/com/example/service/impl/UserServiceImpl.java"

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = encoder.Encode(testPath)
    }
}

func BenchmarkSubstitutionHashEscape_Decode(b *testing.B) {
    encoder := NewSubstitutionHashEscape()
    testPath := "/home/user/project/src/main/java/com/example/service/impl/UserServiceImpl.java"
    encoded, _ := encoder.Encode(testPath)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = encoder.Decode(encoded)
    }
}

func BenchmarkSubstitutionHashEscape_EncodeWithFallback(b *testing.B) {
    encoder := NewSubstitutionHashEscape()

    benchmarks := []struct {
        name string
        path string
    }{
        {
            name: "short_path",
            path: "/usr/bin/python3",
        },
        {
            name: "medium_path",
            path: "/home/user/project/src/main/java/com/example/service/UserService.java",
        },
        {
            name: "long_path_fallback",
            path: "/" + strings.Repeat("very-long-directory-name", 10) + "/file.txt",
        },
    }

    for _, bm := range benchmarks {
        b.Run(bm.name, func(b *testing.B) {
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _, _ = encoder.EncodeWithFallback(bm.path)
            }
        })
    }
}

func BenchmarkMemoryUsage(b *testing.B) {
    encoder := NewSubstitutionHashEscape()

    // Generate test data
    paths := make([]string, 1000)
    for i := 0; i < 1000; i++ {
        paths[i] = strings.Repeat("/dir", i%10) + "/file.txt"
    }

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        for _, path := range paths {
            _, _ = encoder.EncodeWithFallback(path)
        }
    }
}
```

## 5. エラーハンドリング仕様

**注意**: 統合・移行テストは関連コンポーネントが実装されていないため削除

### 5.1. エラー分類と対応

| エラータイプ | 発生条件 | 対処法 |
|-------------|----------|--------|
| `ErrEmptyPath` | 空パスが入力された | パス入力の検証 |
| `ErrNotAbsoluteOrNormalized` | 絶対・正規化済みでないパス | パスの事前変換 |
| `ErrInvalidPath` | 無効なパス（基本エラーをラップ） | 原因調査とエラー報告 |
| `ErrFallbackNotReversible` | フォールバックファイルのデコード試行 | 不可逆であることを通知 |

## 6. 設定・デプロイメント仕様

### 6.1. 定数定義

```go
const (
    // MaxFilenameLength defines the maximum allowed filename length (NAME_MAX - safety margin)
    MaxFilenameLength = 250
    // HashLength defines the number of characters to use from SHA256 hash
    HashLength = 12
)
```

この詳細仕様書により、シンプル化されたハイブリッドハッシュファイル名エンコーディング方式の実装が可能になります。

**主な簡略化**：
- 分析機能の削除
- HybridHashFilePathGetterとMigrationHashFilePathGetterの削除
- エラーハンドリングの簡素化
- パス検証の厳格化（正規化済み絶対パスのみ受け付け）

コアエンコーディング機能は独立してテスト可能であり、将来必要に応じて上位コンポーネントを追加できます。
