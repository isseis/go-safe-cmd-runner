# 詳細仕様書：ハイブリッドハッシュファイル名エンコーディング

## 1. 実装概要 (Implementation Overview)

### 1.1. 実装対象コンポーネント

本仕様書は以下のコンポーネントの詳細実装仕様を定義する：

- `SubstitutionHashEscape`: 換字+ダブルエスケープエンコーダー
- `HybridHashFilePathGetter`: ハイブリッド方式のパス生成器
- `MigrationHashFilePathGetter`: 移行サポート機能
- 関連するテストスイートと検証機能

### 1.2. ファイル配置

```
internal/filevalidator/
├── encoding/
│   ├── substitution_hash_escape.go      # メインエンコーダー
│   ├── substitution_hash_escape_test.go # ユニットテスト
│   ├── encoding_result.go               # 結果構造体
│   └── errors.go                        # エラータイプ定義
├── hybrid_hash_file_path_getter.go      # ハイブリッドパス生成器
├── hybrid_hash_file_path_getter_test.go # 統合テスト
├── migration_hash_file_path_getter.go   # 移行サポート
└── benchmark_encoding_test.go           # パフォーマンステスト
```

## 2. SubstitutionHashEscape 詳細仕様

### 2.1. 構造体定義

```go
package encoding

import (
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "strings"
)

// SubstitutionHashEscape implements hybrid substitution + double escape encoding
type SubstitutionHashEscape struct {
    // MaxFilenameLength defines the maximum allowed filename length
    // Default: 250 (NAME_MAX - safety margin)
    MaxFilenameLength int

    // EnableFallback controls whether SHA256 fallback is used for long paths
    // Default: true
    EnableFallback bool

    // HashLength defines the number of characters to use from SHA256 hash
    // Default: 12
    HashLength int
}

// NewSubstitutionHashEscape creates a new encoder with default settings
func NewSubstitutionHashEscape() *SubstitutionHashEscape {
    return &SubstitutionHashEscape{
        MaxFilenameLength: 250,
        EnableFallback:    true,
        HashLength:        12,
    }
}

// NewSubstitutionHashEscapeWithConfig creates encoder with custom configuration
func NewSubstitutionHashEscapeWithConfig(config EncodingConfig) *SubstitutionHashEscape {
    return &SubstitutionHashEscape{
        MaxFilenameLength: config.MaxFilenameLength,
        EnableFallback:    config.EnableFallback,
        HashLength:        config.HashLength,
    }
}
```

### 2.2. エンコーディング実装

#### 2.2.1 基本エンコード関数

```go
// Encode encodes a file path using substitution + double escape method
// Returns the encoded filename (without directory path)
func (e *SubstitutionHashEscape) Encode(path string) string {
    if path == "" {
        return ""
    }

    // Step 1: Substitution (/ ↔ ~)
    substituted := e.substitute(path)

    // Step 2: Double escape (# → #1, / → ##)
    escaped := e.doubleEscape(substituted)

    return escaped
}

// substitute performs character substitution (/ ↔ ~)
func (e *SubstitutionHashEscape) substitute(path string) string {
    var builder strings.Builder
    builder.Grow(len(path))

    for _, char := range path {
        switch char {
        case '/':
            builder.WriteRune('~')
        case '~':
            builder.WriteRune('/')
        default:
            builder.WriteRune(char)
        }
    }

    return builder.String()
}

// doubleEscape performs meta-character double escaping
func (e *SubstitutionHashEscape) doubleEscape(substituted string) string {
    // Replace # → #1 first to avoid interference
    escaped := strings.ReplaceAll(substituted, "#", "#1")
    // Replace / → ##
    escaped = strings.ReplaceAll(escaped, "/", "##")

    return escaped
}
```

#### 2.2.2 デコード実装

```go
// Decode decodes an encoded filename back to original file path
func (e *SubstitutionHashEscape) Decode(encoded string) (string, error) {
    if encoded == "" {
        return "", nil
    }

    // Check if this is a fallback format (not start with ~)
    if len(encoded) > 0 && encoded[0] != '~' {
        return "", ErrFallbackNotReversible{EncodedName: encoded}
    }

    // Step 1: Reverse double escape (## → /, #1 → #)
    decoded := strings.ReplaceAll(encoded, "##", "/")
    decoded = strings.ReplaceAll(decoded, "#1", "#")

    // Step 2: Reverse substitution (/ ↔ ~)
    result := e.reverseSubstitute(decoded)

    return result, nil
}

// reverseSubstitute reverses the character substitution
func (e *SubstitutionHashEscape) reverseSubstitute(decoded string) string {
    var builder strings.Builder
    builder.Grow(len(decoded))

    for _, char := range decoded {
        switch char {
        case '/':
            builder.WriteRune('~')
        case '~':
            builder.WriteRune('/')
        default:
            builder.WriteRune(char)
        }
    }

    return builder.String()
}
```

#### 2.2.3 ハイブリッド実装（フォールバック対応）

```go
// EncodeWithFallback encodes with automatic fallback to SHA256 for long paths
func (e *SubstitutionHashEscape) EncodeWithFallback(path string) EncodingResult {
    if path == "" {
        return EncodingResult{
            EncodedName:    "",
            IsFallback:     false,
            OriginalLength: 0,
            EncodedLength:  0,
        }
    }

    // Try normal encoding first
    normalEncoded := e.Encode(path)

    // Check length constraint
    if len(normalEncoded) <= e.MaxFilenameLength {
        return EncodingResult{
            EncodedName:    normalEncoded,
            IsFallback:     false,
            OriginalLength: len(path),
            EncodedLength:  len(normalEncoded),
        }
    }

    // Use SHA256 fallback for long paths
    if !e.EnableFallback {
        // Fallback disabled, return error-indicating result
        return EncodingResult{
            EncodedName:    "",
            IsFallback:     false,
            OriginalLength: len(path),
            EncodedLength:  len(normalEncoded),
            Error:          ErrPathTooLong{Path: path, EncodedLength: len(normalEncoded)},
        }
    }

    fallbackEncoded := e.generateSHA256Fallback(path)

    return EncodingResult{
        EncodedName:    fallbackEncoded,
        IsFallback:     true,
        OriginalLength: len(path),
        EncodedLength:  len(fallbackEncoded),
    }
}

// generateSHA256Fallback generates SHA256-based filename for long paths
func (e *SubstitutionHashEscape) generateSHA256Fallback(path string) string {
    hash := sha256.Sum256([]byte(path))
    hashStr := base64.URLEncoding.EncodeToString(hash[:])

    // Use configured hash length, ensure it fits within limits
    hashLength := e.HashLength
    if hashLength > len(hashStr) {
        hashLength = len(hashStr)
    }

    // Format: {hash}.json (hashLength + 5 characters)
    return hashStr[:hashLength] + ".json"
}
```

### 2.3. 分析・デバッグ機能

```go
// AnalyzeEncoding provides detailed analysis of encoding process
func (e *SubstitutionHashEscape) AnalyzeEncoding(path string) EncodingAnalysis {
    result := e.EncodeWithFallback(path)

    analysis := EncodingAnalysis{
        OriginalPath:     path,
        EncodedName:      result.EncodedName,
        IsFallback:       result.IsFallback,
        OriginalLength:   result.OriginalLength,
        EncodedLength:    result.EncodedLength,
        ExpansionRatio:   float64(result.EncodedLength) / float64(result.OriginalLength),
    }

    if !result.IsFallback {
        // Analyze character frequency for normal encoding
        analysis.CharFrequency = e.analyzeCharFrequency(path)
        analysis.EscapeCount = e.countEscapeOperations(path)
    }

    return analysis
}

// analyzeCharFrequency counts character frequency in original path
func (e *SubstitutionHashEscape) analyzeCharFrequency(path string) map[rune]int {
    frequency := make(map[rune]int)

    for _, char := range path {
        frequency[char]++
    }

    return frequency
}

// countEscapeOperations counts the number of escape operations needed
func (e *SubstitutionHashEscape) countEscapeOperations(path string) EscapeOperationCount {
    substituted := e.substitute(path)

    hashCount := strings.Count(substituted, "#")
    slashCount := strings.Count(substituted, "/")

    return EscapeOperationCount{
        HashEscapes:  hashCount,   // # → #1
        SlashEscapes: slashCount,  // / → ##
        TotalEscapes: hashCount + slashCount,
        AddedChars:   hashCount + slashCount, // Each escape adds 1 character
    }
}

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

    return !IsNormalEncoding(string)
}
```

## 3. データ構造定義

### 3.1. 結果構造体

```go
// EncodingResult represents the result of an encoding operation
type EncodingResult struct {
    EncodedName    string // The encoded filename
    IsFallback     bool   // Whether SHA256 fallback was used
    OriginalLength int    // Length of original path
    EncodedLength  int    // Length of encoded filename
    Error          error  // Any error that occurred during encoding
}

// EncodingAnalysis provides detailed analysis of encoding process
type EncodingAnalysis struct {
    OriginalPath     string                 // Original file path
    EncodedName      string                 // Encoded filename
    IsFallback       bool                   // Whether fallback was used
    OriginalLength   int                    // Length of original path
    EncodedLength    int                    // Length of encoded name
    ExpansionRatio   float64                // Encoded length / Original length
    CharFrequency    map[rune]int           // Character frequency in original path
    EscapeCount      EscapeOperationCount   // Number of escape operations
}

// EscapeOperationCount tracks escape operation statistics
type EscapeOperationCount struct {
    HashEscapes  int // Number of # → #1 operations
    SlashEscapes int // Number of / → ## operations
    TotalEscapes int // Total escape operations
    AddedChars   int // Total characters added by escaping
}

// EncodingConfig provides configuration for the encoder
type EncodingConfig struct {
    MaxFilenameLength int  `yaml:"max_filename_length"` // Default: 250
    EnableFallback    bool `yaml:"enable_fallback"`     // Default: true
    HashLength        int  `yaml:"hash_length"`         // Default: 12
    LogDetails        bool `yaml:"log_details"`         // Default: false
}
```

### 3.2. エラータイプ定義

```go
// ErrFallbackNotReversible indicates a fallback encoding cannot be decoded
type ErrFallbackNotReversible struct {
    EncodedName string
}

func (e ErrFallbackNotReversible) Error() string {
    return fmt.Sprintf("fallback encoding '%s' cannot be decoded to original path", e.EncodedName)
}

// ErrPathTooLong indicates the encoded path exceeds maximum length
type ErrPathTooLong struct {
    Path          string
    EncodedLength int
    MaxLength     int
}

func (e ErrPathTooLong) Error() string {
    return fmt.Sprintf("encoded path too long: %d characters (max: %d) for path: %s",
        e.EncodedLength, e.MaxLength, e.Path)
}

// ErrInvalidEncodedName indicates the encoded name format is invalid
type ErrInvalidEncodedName struct {
    EncodedName string
    Reason      string
}

func (e ErrInvalidEncodedName) Error() string {
    return fmt.Sprintf("invalid encoded name '%s': %s", e.EncodedName, e.Reason)
}
```

## 4. HybridHashFilePathGetter 実装仕様

### 4.1. 構造体定義

```go
package filevalidator

import (
    "path/filepath"
    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator/encoding"
    "github.com/isseis/go-safe-cmd-runner/internal/common"
)

// HybridHashFilePathGetter implements HashFilePathGetter using hybrid encoding
type HybridHashFilePathGetter struct {
    encoder *encoding.SubstitutionHashEscape
    logger  Logger // For logging fallback usage
}

// NewHybridHashFilePathGetter creates a new hybrid hash file path getter
func NewHybridHashFilePathGetter() *HybridHashFilePathGetter {
    return &HybridHashFilePathGetter{
        encoder: encoding.NewSubstitutionHashEscape(),
        logger:  NewDefaultLogger(),
    }
}

// NewHybridHashFilePathGetterWithConfig creates getter with custom configuration
func NewHybridHashFilePathGetterWithConfig(config encoding.EncodingConfig, logger Logger) *HybridHashFilePathGetter {
    return &HybridHashFilePathGetter{
        encoder: encoding.NewSubstitutionHashEscapeWithConfig(config),
        logger:  logger,
    }
}
```

### 4.2. メインAPI実装

```go
// GetHashFilePath implements HashFilePathGetter interface
func (h *HybridHashFilePathGetter) GetHashFilePath(
    hashAlgorithm HashAlgorithm,
    hashDir string,
    filePath common.ResolvedPath) (string, error) {

    // Input validation
    if hashAlgorithm == nil {
        return "", ErrNilAlgorithm
    }

    if hashDir == "" {
        return "", ErrEmptyHashDir
    }

    if filePath.String() == "" {
        return "", ErrEmptyFilePath
    }

    // Encode the file path
    result := h.encoder.EncodeWithFallback(filePath.String())

    if result.Error != nil {
        return "", fmt.Errorf("encoding failed for path '%s': %w", filePath.String(), result.Error)
    }

    // Log fallback usage if enabled
    if result.IsFallback && h.logger != nil {
        h.logger.LogInfo("Long path detected, using SHA256 fallback", map[string]interface{}{
            "original_path":   filePath.String(),
            "original_length": result.OriginalLength,
            "encoded_name":    result.EncodedName,
            "encoded_length":  result.EncodedLength,
        })
    }

    // Combine hash directory and encoded filename
    hashFilePath := filepath.Join(hashDir, result.EncodedName)

    return hashFilePath, nil
}
```

### 4.3. 分析・デバッグAPI

```go
// AnalyzeFilePath provides detailed analysis of file path encoding
func (h *HybridHashFilePathGetter) AnalyzeFilePath(filePath common.ResolvedPath) encoding.EncodingAnalysis {
    return h.encoder.AnalyzeEncoding(filePath.String())
}

// GetEncodingStats returns statistics about encoding efficiency
func (h *HybridHashFilePathGetter) GetEncodingStats(filePaths []common.ResolvedPath) EncodingStats {
    stats := EncodingStats{
        TotalFiles:     len(filePaths),
        NormalEncoded:  0,
        FallbackUsed:   0,
        TotalChars:     0,
        EncodedChars:   0,
    }

    for _, filePath := range filePaths {
        result := h.encoder.EncodeWithFallback(filePath.String())

        stats.TotalChars += result.OriginalLength
        stats.EncodedChars += result.EncodedLength

        if result.IsFallback {
            stats.FallbackUsed++
        } else {
            stats.NormalEncoded++
        }
    }

    if stats.TotalChars > 0 {
        stats.OverallExpansionRatio = float64(stats.EncodedChars) / float64(stats.TotalChars)
    }

    stats.FallbackRate = float64(stats.FallbackUsed) / float64(stats.TotalFiles)

    return stats
}

// EncodingStats represents statistics about encoding efficiency
type EncodingStats struct {
    TotalFiles            int     // Total number of files processed
    NormalEncoded         int     // Number of files using normal encoding
    FallbackUsed          int     // Number of files using fallback
    TotalChars            int     // Total characters in original paths
    EncodedChars          int     // Total characters in encoded names
    OverallExpansionRatio float64 // Overall expansion ratio
    FallbackRate          float64 // Fallback usage rate (0.0 - 1.0)
}
```

## 5. MigrationHashFilePathGetter 実装仕様

### 5.1. 移行サポート実装

```go
// MigrationHashFilePathGetter supports gradual migration from legacy to hybrid encoding
type MigrationHashFilePathGetter struct {
    hybridGetter HashFilePathGetter // New hybrid implementation
    legacyGetter HashFilePathGetter // Existing SHA256 implementation
    fileSystem   FileSystemInterface
    logger       Logger

    // Migration configuration
    autoMigrate    bool // Whether to automatically migrate found legacy files
    backupOldFiles bool // Whether to backup legacy files before migration
}

// NewMigrationHashFilePathGetter creates a new migration-supporting getter
func NewMigrationHashFilePathGetter(
    hybridGetter HashFilePathGetter,
    legacyGetter HashFilePathGetter,
    fileSystem FileSystemInterface,
    logger Logger) *MigrationHashFilePathGetter {

    return &MigrationHashFilePathGetter{
        hybridGetter:   hybridGetter,
        legacyGetter:   legacyGetter,
        fileSystem:     fileSystem,
        logger:         logger,
        autoMigrate:    false, // Default to manual migration
        backupOldFiles: true,  // Default to backup for safety
    }
}

// GetHashFilePath implements HashFilePathGetter with migration support
func (m *MigrationHashFilePathGetter) GetHashFilePath(
    hashAlgorithm HashAlgorithm,
    hashDir string,
    filePath common.ResolvedPath) (string, error) {

    // Try hybrid approach first
    hybridPath, err := m.hybridGetter.GetHashFilePath(hashAlgorithm, hashDir, filePath)
    if err != nil {
        return "", fmt.Errorf("hybrid getter failed: %w", err)
    }

    // Check if hybrid hash file exists
    if exists, err := m.fileSystem.FileExists(hybridPath); err != nil {
        return "", fmt.Errorf("failed to check hybrid hash file existence: %w", err)
    } else if exists {
        // Hybrid file exists, use it
        return hybridPath, nil
    }

    // Check for legacy hash file
    legacyPath, err := m.legacyGetter.GetHashFilePath(hashAlgorithm, hashDir, filePath)
    if err != nil {
        // Legacy getter failed, but we can still use hybrid path for new files
        m.logger.LogWarning("Legacy getter failed, using hybrid path for new file", map[string]interface{}{
            "file_path":   filePath.String(),
            "hybrid_path": hybridPath,
            "error":       err.Error(),
        })
        return hybridPath, nil
    }

    if exists, err := m.fileSystem.FileExists(legacyPath); err != nil {
        return "", fmt.Errorf("failed to check legacy hash file existence: %w", err)
    } else if exists {
        // Legacy file exists
        m.logger.LogInfo("Legacy hash file found", map[string]interface{}{
            "file_path":   filePath.String(),
            "legacy_path": legacyPath,
            "hybrid_path": hybridPath,
        })

        // Auto-migrate if configured
        if m.autoMigrate {
            if err := m.migrateHashFile(legacyPath, hybridPath); err != nil {
                m.logger.LogError("Failed to auto-migrate hash file", map[string]interface{}{
                    "legacy_path": legacyPath,
                    "hybrid_path": hybridPath,
                    "error":       err.Error(),
                })
                // Return legacy path if migration fails
                return legacyPath, nil
            }
            // Migration successful, return hybrid path
            return hybridPath, nil
        }

        // No auto-migration, return legacy path
        return legacyPath, nil
    }

    // Neither hybrid nor legacy file exists, create new hybrid file
    return hybridPath, nil
}
```

### 5.2. 手動移行機能

```go
// MigrateHashFile migrates a single hash file from legacy to hybrid format
func (m *MigrationHashFilePathGetter) MigrateHashFile(legacyPath, hybridPath string) error {
    return m.migrateHashFile(legacyPath, hybridPath)
}

// migrateHashFile performs the actual migration
func (m *MigrationHashFilePathGetter) migrateHashFile(legacyPath, hybridPath string) error {
    // Read legacy hash file content
    content, err := m.fileSystem.ReadFile(legacyPath)
    if err != nil {
        return fmt.Errorf("failed to read legacy hash file '%s': %w", legacyPath, err)
    }

    // Backup legacy file if configured
    if m.backupOldFiles {
        backupPath := legacyPath + ".backup"
        if err := m.fileSystem.CopyFile(legacyPath, backupPath); err != nil {
            m.logger.LogWarning("Failed to create backup", map[string]interface{}{
                "legacy_path": legacyPath,
                "backup_path": backupPath,
                "error":       err.Error(),
            })
            // Continue with migration even if backup fails
        }
    }

    // Ensure hybrid directory exists
    hybridDir := filepath.Dir(hybridPath)
    if err := m.fileSystem.MkdirAll(hybridDir, 0755); err != nil {
        return fmt.Errorf("failed to create hybrid directory '%s': %w", hybridDir, err)
    }

    // Write content to hybrid location
    if err := m.fileSystem.WriteFile(hybridPath, content, 0644); err != nil {
        return fmt.Errorf("failed to write hybrid hash file '%s': %w", hybridPath, err)
    }

    // Remove legacy file after successful migration
    if err := m.fileSystem.RemoveFile(legacyPath); err != nil {
        m.logger.LogWarning("Failed to remove legacy file after migration", map[string]interface{}{
            "legacy_path": legacyPath,
            "error":       err.Error(),
        })
        // Don't fail migration if cleanup fails
    }

    m.logger.LogInfo("Successfully migrated hash file", map[string]interface{}{
        "legacy_path": legacyPath,
        "hybrid_path": hybridPath,
    })

    return nil
}

// BatchMigrate migrates multiple hash files in batch
func (m *MigrationHashFilePathGetter) BatchMigrate(
    hashAlgorithm HashAlgorithm,
    hashDir string,
    filePaths []common.ResolvedPath,
    batchSize int) BatchMigrationResult {

    result := BatchMigrationResult{
        TotalFiles:      len(filePaths),
        ProcessedFiles:  0,
        MigratedFiles:   0,
        SkippedFiles:    0,
        FailedFiles:     0,
        Errors:          make([]error, 0),
    }

    for i, filePath := range filePaths {
        // Process in batches to avoid overwhelming the system
        if i > 0 && i%batchSize == 0 {
            m.logger.LogInfo("Batch migration progress", map[string]interface{}{
                "processed": i,
                "total":     len(filePaths),
                "migrated":  result.MigratedFiles,
                "failed":    result.FailedFiles,
            })
        }

        result.ProcessedFiles++

        // Get paths for both legacy and hybrid
        legacyPath, legacyErr := m.legacyGetter.GetHashFilePath(hashAlgorithm, hashDir, filePath)
        hybridPath, hybridErr := m.hybridGetter.GetHashFilePath(hashAlgorithm, hashDir, filePath)

        if legacyErr != nil || hybridErr != nil {
            result.FailedFiles++
            result.Errors = append(result.Errors, fmt.Errorf("path generation failed for %s", filePath.String()))
            continue
        }

        // Check if migration is needed
        legacyExists, _ := m.fileSystem.FileExists(legacyPath)
        hybridExists, _ := m.fileSystem.FileExists(hybridPath)

        if !legacyExists {
            result.SkippedFiles++ // No legacy file to migrate
            continue
        }

        if hybridExists {
            result.SkippedFiles++ // Hybrid file already exists
            continue
        }

        // Perform migration
        if err := m.migrateHashFile(legacyPath, hybridPath); err != nil {
            result.FailedFiles++
            result.Errors = append(result.Errors, fmt.Errorf("migration failed for %s: %w", filePath.String(), err))
        } else {
            result.MigratedFiles++
        }
    }

    return result
}

// BatchMigrationResult represents the result of a batch migration operation
type BatchMigrationResult struct {
    TotalFiles     int     // Total number of files processed
    ProcessedFiles int     // Number of files processed so far
    MigratedFiles  int     // Number of files successfully migrated
    SkippedFiles   int     // Number of files skipped (no migration needed)
    FailedFiles    int     // Number of files that failed to migrate
    Errors         []error // List of errors encountered
}
```

## 6. テスト実装仕様

### 6.1. ユニットテスト

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
        name     string
        input    string
        expected string
    }{
        {
            name:     "simple path",
            input:    "/usr/bin/python3",
            expected: "~usr~bin~python3",
        },
        {
            name:     "path with hash character",
            input:    "/home/user#test/file",
            expected: "~home~user#1test~file",
        },
        {
            name:     "path with tilde character",
            input:    "/home/~user/file",
            expected: "~home~/user~file",
        },
        {
            name:     "complex path",
            input:    "/path/with#hash/and~tilde/file",
            expected: "~path~with#1#1ash~and/tilde~file",
        },
        {
            name:     "empty path",
            input:    "",
            expected: "",
        },
        {
            name:     "root path",
            input:    "/",
            expected: "~",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := encoder.Encode(tt.input)
            assert.Equal(t, tt.expected, result)
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
            input:    "~path~with#1#1ash~and/tilde~file",
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
            encoded := encoder.Encode(originalPath)

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
            result := encoder.EncodeWithFallback(tt.path)

            assert.Equal(t, tt.wantFallback, result.IsFallback)

            if result.IsFallback {
                // Fallback should not start with ~
                assert.NotEqual(t, '~', result.EncodedName[0])
                // Fallback should be within length limits
                assert.LessOrEqual(t, len(result.EncodedName), encoder.MaxFilenameLength)
            } else {
                // Normal encoding should start with ~ (for full paths)
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
        result := encoder.EncodeWithFallback(path)
        if result.IsFallback {
            return true // Skip fallback cases for this test
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

        encoded1 := encoder.Encode(path)
        encoded2 := encoder.Encode(path)

        return encoded1 == encoded2
    }

    err := quick.Check(property, &quick.Config{MaxCount: 1000})
    assert.NoError(t, err, "Deterministic property failed")
}

func TestProperty_Encode_UniqueOutput(t *testing.T) {
    encoder := NewSubstitutionHashEscape()

    // Property: Different paths should produce different encoded names
    // (except when fallback is used)
    seenEncodings := make(map[string]string)

    property := func(path1, path2 string) bool {
        if !utf8.ValidString(path1) || !utf8.ValidString(path2) {
            return true
        }

        if path1 == path2 {
            return true // Same input, same output is expected
        }

        result1 := encoder.EncodeWithFallback(path1)
        result2 := encoder.EncodeWithFallback(path2)

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
        _ = encoder.Encode(testPath)
    }
}

func BenchmarkSubstitutionHashEscape_Decode(b *testing.B) {
    encoder := NewSubstitutionHashEscape()
    testPath := "/home/user/project/src/main/java/com/example/service/impl/UserServiceImpl.java"
    encoded := encoder.Encode(testPath)

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
                _ = encoder.EncodeWithFallback(bm.path)
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
            _ = encoder.EncodeWithFallback(path)
        }
    }
}
```

## 7. 統合・移行テスト

### 7.1. Validator統合テスト

```go
// integration_test.go
func TestValidator_WithHybridEncoding(t *testing.T) {
    // Setup test environment
    tempDir := t.TempDir()
    hashDir := filepath.Join(tempDir, "hashes")

    hybridGetter := NewHybridHashFilePathGetter()
    validator := NewValidator(hybridGetter, hashDir, NewSHA256Algorithm())

    // Test file validation with hybrid encoding
    testFile := filepath.Join(tempDir, "test.txt")
    testContent := []byte("test content")

    err := os.WriteFile(testFile, testContent, 0644)
    require.NoError(t, err)

    filePath := common.NewResolvedPath(testFile)

    // Record hash
    err = validator.RecordHash(filePath)
    require.NoError(t, err)

    // Verify the hash file was created with hybrid encoding
    hashFilePath, err := hybridGetter.GetHashFilePath(validator.algorithm, hashDir, filePath)
    require.NoError(t, err)

    // Check that hash file exists and uses expected encoding
    assert.FileExists(t, hashFilePath)

    // Hash file name should start with ~ for normal encoding (assuming short path)
    hashFileName := filepath.Base(hashFilePath)
    assert.Equal(t, '~', hashFileName[0])

    // Validate hash
    isValid, err := validator.ValidateHash(filePath)
    require.NoError(t, err)
    assert.True(t, isValid)
}
```

### 7.2. 移行機能テスト

```go
func TestMigrationHashFilePathGetter(t *testing.T) {
    tempDir := t.TempDir()
    hashDir := filepath.Join(tempDir, "hashes")

    // Setup mock file system
    mockFS := NewMockFileSystem()
    logger := NewTestLogger()

    // Create legacy and hybrid getters
    legacyGetter := NewLegacyHashFilePathGetter() // Existing implementation
    hybridGetter := NewHybridHashFilePathGetter()
    migrationGetter := NewMigrationHashFilePathGetter(hybridGetter, legacyGetter, mockFS, logger)

    testPath := common.NewResolvedPath("/usr/bin/python3")
    algorithm := NewSHA256Algorithm()

    // Setup: create legacy hash file
    legacyHashPath, err := legacyGetter.GetHashFilePath(algorithm, hashDir, testPath)
    require.NoError(t, err)

    legacyContent := []byte(`{"hash": "abc123", "algorithm": "SHA256"}`)
    mockFS.WriteFile(legacyHashPath, legacyContent, 0644)

    // Test: migration getter should find legacy file
    foundPath, err := migrationGetter.GetHashFilePath(algorithm, hashDir, testPath)
    require.NoError(t, err)
    assert.Equal(t, legacyHashPath, foundPath)

    // Test: enable auto-migration
    migrationGetter.autoMigrate = true

    foundPath, err = migrationGetter.GetHashFilePath(algorithm, hashDir, testPath)
    require.NoError(t, err)

    // Should return hybrid path after migration
    expectedHybridPath, _ := hybridGetter.GetHashFilePath(algorithm, hashDir, testPath)
    assert.Equal(t, expectedHybridPath, foundPath)

    // Verify migration occurred
    hybridExists, _ := mockFS.FileExists(expectedHybridPath)
    assert.True(t, hybridExists)

    legacyExists, _ := mockFS.FileExists(legacyHashPath)
    assert.False(t, legacyExists) // Should be removed after migration
}
```

## 8. エラーハンドリング仕様

### 8.1. エラー分類と対応

| エラータイプ | 発生条件 | 対処法 |
|-------------|----------|--------|
| `ErrPathTooLong` | エンコード後がNAME_MAX超過かつフォールバック無効 | フォールバック有効化またはパス短縮 |
| `ErrFallbackNotReversible` | フォールバックファイルのデコード試行 | 不可逆であることを通知 |
| `ErrInvalidEncodedName` | 不正なエンコードファイル名 | 入力検証とエラー報告 |
| `ErrNilAlgorithm` | ハッシュアルゴリズムがnil | アルゴリズム設定確認 |
| `ErrEmptyHashDir` | ハッシュディレクトリが空 | 設定ファイル確認 |

### 8.2. ログ出力仕様

```go
// Logger interface for structured logging
type Logger interface {
    LogDebug(message string, fields map[string]interface{})
    LogInfo(message string, fields map[string]interface{})
    LogWarning(message string, fields map[string]interface{})
    LogError(message string, fields map[string]interface{})
}

// Example log entries
/*
INFO: Fallback encoding used
{
  "timestamp": "2025-09-16T10:00:00Z",
  "level": "INFO",
  "component": "HybridHashFilePathGetter",
  "message": "Long path detected, using SHA256 fallback",
  "original_path": "/very/long/path/...",
  "original_length": 280,
  "encoded_name": "AbCdEf123...",
  "encoded_length": 43
}

WARN: Migration needed
{
  "timestamp": "2025-09-16T10:01:00Z",
  "level": "WARN",
  "component": "MigrationHashFilePathGetter",
  "message": "Legacy hash file found, consider migration",
  "file_path": "/usr/bin/python3",
  "legacy_path": "/hashes/abc123def456.json",
  "hybrid_path": "/hashes/~usr~bin~python3"
}
*/
```

## 9. 設定・デプロイメント仕様

### 9.1. 設定ファイル

```yaml
# config/encoding.yml
encoding:
  # Hybrid encoding settings
  hybrid:
    enabled: true
    max_filename_length: 250  # NAME_MAX - safety margin
    enable_fallback: true
    hash_length: 42          # SHA256 hash characters to use
    log_fallback_usage: true
    log_details: false       # Enable detailed debug logging

  # Migration settings
  migration:
    enabled: true
    auto_migrate: false      # Automatic migration (use with caution)
    backup_old_files: true   # Backup before migration
    migration_batch_size: 1000

  # Performance settings
  performance:
    enable_metrics: true
    sample_rate: 0.1         # Metrics sampling rate
    memory_limit_mb: 100     # Memory usage limit for batch operations
    enable_pooling: true     # Enable string builder pooling
```

### 9.2. 初期化コード

```go
// Initialize hybrid encoding system
func InitializeHybridEncoding(config EncodingConfig) (*HybridHashFilePathGetter, error) {
    // Validate configuration
    if err := validateEncodingConfig(config); err != nil {
        return nil, fmt.Errorf("invalid encoding configuration: %w", err)
    }

    // Create logger
    logger := NewProductionLogger()

    // Create hybrid getter
    hybridGetter := NewHybridHashFilePathGetterWithConfig(config, logger)

    // Setup metrics if enabled
    if config.Performance.EnableMetrics {
        metrics := NewEncodingMetrics(config.Performance.SampleRate)
        hybridGetter.SetMetrics(metrics)
    }

    return hybridGetter, nil
}

// validateEncodingConfig validates the encoding configuration
func validateEncodingConfig(config EncodingConfig) error {
    if config.MaxFilenameLength <= 0 || config.MaxFilenameLength > 255 {
        return fmt.Errorf("max_filename_length must be between 1 and 255, got: %d", config.MaxFilenameLength)
    }

    if config.HashLength <= 0 || config.HashLength > 64 {
        return fmt.Errorf("hash_length must be between 1 and 64, got: %d", config.HashLength)
    }

    return nil
}
```

この詳細仕様書により、ADRで決定されたハイブリッドハッシュファイル名エンコーディング方式の完全な実装が可能になります。各コンポーネントは独立してテスト可能であり、段階的な移行と運用監視も適切にサポートされています。
