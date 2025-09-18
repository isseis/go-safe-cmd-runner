# 詳細仕様書：コマンド出力キャプチャ機能

## 1. 概要 (Overview)

本文書は、go-safe-cmd-runnerにおけるコマンド出力キャプチャ機能の詳細な技術仕様を定義する。要件定義書とアーキテクチャ設計書に基づき、実装レベルでの具体的な仕様を記述する。

## 2. データ構造仕様 (Data Structure Specifications)

### 2.1 既存構造体の拡張

#### 2.1.1 Command構造体の拡張
```go
// internal/runner/config/config.go
type Command struct {
    // 既存フィールド
    Name         string   `toml:"name"`
    Description  string   `toml:"description"`
    Cmd          string   `toml:"cmd"`
    Args         []string `toml:"args"`
    WorkDir      string   `toml:"workdir"`
    Env          []string `toml:"env"`
    RunAsUser    string   `toml:"run_as_user"`
    Timeout      int      `toml:"timeout"`

    // 新規追加フィールド
    Output       string   `toml:"output"`  // 標準出力の書き込み先ファイル（optional）
}
```

#### 2.1.2 GlobalConfig構造体の拡張
```go
// internal/runner/config/config.go
type GlobalConfig struct {
    // 既存フィールド
    Timeout           int      `toml:"timeout"`
    WorkDir           string   `toml:"workdir"`
    Env               []string `toml:"env"`

    // 新規追加フィールド
    MaxOutputSize     int64    `toml:"max_output_size"`  // デフォルト出力サイズ制限（bytes、0で無制限）
}
```

### 2.2 新規データ構造

#### 2.2.1 OutputCaptureManager インターフェース
```go
// internal/runner/output/manager.go
type OutputCaptureManager interface {
    // 出力キャプチャの準備（事前検証）
    PrepareOutput(config *OutputConfig) (*OutputCapture, error)

    // ストリーミング出力書き込み
    WriteOutput(capture *OutputCapture, data []byte) error

    // 出力完了と最終化
    FinalizeOutput(capture *OutputCapture) error

    // エラー時のクリーンアップ
    CleanupOutput(capture *OutputCapture) error

    // Dry-Run用の分析
    AnalyzeOutput(config *OutputConfig) (*OutputAnalysis, error)
}
```

#### 2.2.2 OutputConfig構造体
```go
// internal/runner/output/types.go
type OutputConfig struct {
    OutputPath    string    // 出力先パス（Command.Outputの値）
    WorkDir       string    // 作業ディレクトリ（相対パス解決用）
    MaxSize       int64     // 出力サイズ制限
    RealUID       int       // 実UID（ファイル権限設定用）
    RealGID       int       // 実GID（ファイル権限設定用）
    CommandName   string    // コマンド名（ログ用）
}
```

#### 2.2.3 OutputCapture構造体
```go
// internal/runner/output/types.go
type OutputCapture struct {
    Config       *OutputConfig  // 設定情報
    OutputPath   string         // 最終出力先パス（絶対パス）
    Buffer       *bytes.Buffer  // メモリバッファ（一時ファイル不要）
    CurrentSize  int64          // 現在の書き込みサイズ
    StartTime    time.Time      // 開始時刻
    mutex        sync.Mutex     // 並行アクセス制御
}
```

#### 2.2.4 OutputAnalysis構造体（Dry-Run用）
```go
// internal/runner/output/types.go
type OutputAnalysis struct {
    OutputPath      string        // 元の出力先パス
    ResolvedPath    string        // 解決済み絶対パス
    DirectoryExists bool          // ディレクトリ存在確認
    WritePermission bool          // 書き込み権限確認
    SecurityRisk    runnertypes.RiskLevel // セキュリティリスク評価
    MaxSizeLimit    int64         // サイズ制限値
    ErrorMessage    string        // エラーメッセージ（問題がある場合）
}

```

### 2.3 エラー型定義

#### 2.3.1 OutputCaptureError
```go
// internal/runner/output/errors.go
type OutputCaptureError struct {
    Type     ErrorType       // エラータイプ
    Path     string          // 関連パス
    Phase    ExecutionPhase  // 実行フェーズ
    Cause    error           // 根本原因
    Command  string          // コマンド名
}

func (e *OutputCaptureError) Error() string {
    return fmt.Sprintf("output capture error [%s][%s]: %s (path: %s, command: %s)",
        e.Type, e.Phase, e.Cause, e.Path, e.Command)
}

func (e *OutputCaptureError) Unwrap() error {
    return e.Cause
}

type ErrorType int
const (
    ErrorTypePathValidation ErrorType = iota  // パス検証エラー
    ErrorTypePermission                       // 権限エラー
    ErrorTypeFileSystem                       // ファイルシステムエラー
    ErrorTypeSizeLimit                        // サイズ制限エラー
    ErrorTypeCleanup                          // クリーンアップエラー
)

func (e ErrorType) String() string {
    switch e {
    case ErrorTypePathValidation:
        return "PATH_VALIDATION"
    case ErrorTypePermission:
        return "PERMISSION"
    case ErrorTypeFileSystem:
        return "FILE_SYSTEM"
    case ErrorTypeSizeLimit:
        return "SIZE_LIMIT"
    case ErrorTypeCleanup:
        return "CLEANUP"
    default:
        return "UNKNOWN"
    }
}

type ExecutionPhase int
const (
    PhasePreparation ExecutionPhase = iota  // 準備段階
    PhaseExecution                          // 実行段階
    PhaseFinalization                       // 完了段階
    PhaseCleanup                            // クリーンアップ段階
)

func (p ExecutionPhase) String() string {
    switch p {
    case PhasePreparation:
        return "PREPARATION"
    case PhaseExecution:
        return "EXECUTION"
    case PhaseFinalization:
        return "FINALIZATION"
    case PhaseCleanup:
        return "CLEANUP"
    default:
        return "UNKNOWN"
    }
}
```

## 3. インターフェース仕様

### 3.1 OutputCaptureManager実装

#### 3.1.1 DefaultOutputCaptureManager
```go
// internal/runner/output/manager.go
type DefaultOutputCaptureManager struct {
    pathValidator    PathValidator
    fileManager      FileManager
    securityValidator *security.Validator
    logger           Logger
}

func NewDefaultOutputCaptureManager(securityValidator *security.Validator, logger Logger) *DefaultOutputCaptureManager {
    fs := NewDefaultExtendedFileSystem()
    return &DefaultOutputCaptureManager{
        pathValidator:    NewDefaultPathValidator(),
        fileManager:      NewSafeFileManager(fs),
        securityValidator: securityValidator,
        logger:           logger,
    }
}
```

#### 3.1.2 メソッド仕様

##### PrepareOutput
```go
func (m *DefaultOutputCaptureManager) PrepareOutput(config *OutputConfig) (*OutputCapture, error) {
    // 1. パス検証
    resolvedPath, err := m.pathValidator.ValidateAndResolvePath(config.OutputPath, config.WorkDir)
    if err != nil {
        return nil, &OutputCaptureError{
            Type:    ErrorTypePathValidation,
            Phase:   PhasePreparation,
            Path:    config.OutputPath,
            Cause:   err,
            Command: config.CommandName,
        }
    }

    // 2. 権限確認
    if err := m.securityValidator.ValidateOutputWritePermission(resolvedPath, config.RealUID); err != nil {
        return nil, &OutputCaptureError{
            Type:    ErrorTypePermission,
            Phase:   PhasePreparation,
            Path:    resolvedPath,
            Cause:   err,
            Command: config.CommandName,
        }
    }

    // 3. ディレクトリ作成
    if err := m.fileManager.EnsureDirectory(filepath.Dir(resolvedPath)); err != nil {
        return nil, &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhasePreparation,
            Path:    resolvedPath,
            Cause:   err,
            Command: config.CommandName,
        }
    }

    // 4. OutputCapture構造体作成（メモリバッファ使用）
    capture := &OutputCapture{
        Config:      config,
        OutputPath:  resolvedPath,
        Buffer:      &bytes.Buffer{},
        CurrentSize: 0,
        StartTime:   time.Now(),
    }

    return capture, nil
}
```

##### WriteOutput
```go
func (m *DefaultOutputCaptureManager) WriteOutput(capture *OutputCapture, data []byte) error {
    capture.mutex.Lock()
    defer capture.mutex.Unlock()

    // サイズ制限チェック
    newSize := capture.CurrentSize + int64(len(data))
    if capture.Config.MaxSize > 0 && newSize > capture.Config.MaxSize {
        // サイズ制限超過エラー
        return &OutputCaptureError{
            Type:    ErrorTypeSizeLimit,
            Phase:   PhaseExecution,
            Path:    capture.OutputPath,
            Cause:   fmt.Errorf("output size limit exceeded: %d bytes (limit: %d)", newSize, capture.Config.MaxSize),
            Command: capture.Config.CommandName,
        }
    }

    // メモリバッファに書き込み
    n, err := capture.Buffer.Write(data)
    if err != nil {
        return &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhaseExecution,
            Path:    capture.OutputPath,
            Cause:   err,
            Command: capture.Config.CommandName,
        }
    }

    capture.CurrentSize += int64(n)

    return nil
}
```

##### FinalizeOutput
```go
func (m *DefaultOutputCaptureManager) FinalizeOutput(capture *OutputCapture) error {
    // 1. safefileioを使ってバッファ内容をファイルに安全に書き込み
    if err := m.fileManager.WriteToFile(capture.OutputPath, capture.Buffer.Bytes()); err != nil {
        return &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhaseFinalization,
            Path:    capture.OutputPath,
            Cause:   err,
            Command: capture.Config.CommandName,
        }
    }

    // 2. ログ記録
    duration := time.Since(capture.StartTime)
    m.logger.Info("Output capture completed",
        "command", capture.Config.CommandName,
        "output_path", capture.OutputPath,
        "size_bytes", capture.CurrentSize,
        "duration_ms", duration.Milliseconds(),
    )

    return nil
}
```

##### CleanupOutput
```go
func (m *DefaultOutputCaptureManager) CleanupOutput(capture *OutputCapture) error {
    // メモリバッファのクリア（ガベージコレクションに任せる）
    capture.Buffer.Reset()
    capture.CurrentSize = 0

    return nil
}
```

##### AnalyzeOutput
```go
func (m *DefaultOutputCaptureManager) AnalyzeOutput(config *OutputConfig) (*OutputAnalysis, error) {
    analysis := &OutputAnalysis{
        OutputPath:   config.OutputPath,
        MaxSizeLimit: config.MaxSize,
    }

    // 1. パス解決
    resolvedPath, err := m.pathValidator.ValidateAndResolvePath(config.OutputPath, config.WorkDir)
    if err != nil {
        analysis.ErrorMessage = fmt.Sprintf("Path validation failed: %v", err)
        analysis.SecurityRisk = runnertypes.RiskLevelCritical
        return analysis, nil
    }
    analysis.ResolvedPath = resolvedPath

    // 2. ディレクトリ存在確認
    dir := filepath.Dir(resolvedPath)
    if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
        analysis.DirectoryExists = true
    } else {
        analysis.DirectoryExists = false
    }

    // 3. 権限確認
    if err := m.securityValidator.ValidateOutputWritePermission(resolvedPath, config.RealUID); err != nil {
        analysis.WritePermission = false
        if analysis.ErrorMessage == "" {
            analysis.ErrorMessage = fmt.Sprintf("Permission check failed: %v", err)
        }
    } else {
        analysis.WritePermission = true
    }

    // 4. セキュリティリスク評価
    analysis.SecurityRisk = m.evaluateSecurityRisk(resolvedPath, config.WorkDir)

    return analysis, nil
}
```

### 3.2 補助インターフェース

#### 3.2.1 PathValidator
```go
// internal/runner/output/path.go
type PathValidator interface {
    ValidateAndResolvePath(outputPath, workDir string) (string, error)
}

type DefaultPathValidator struct{}

func NewDefaultPathValidator() *DefaultPathValidator {
    return &DefaultPathValidator{}
}

func (v *DefaultPathValidator) ValidateAndResolvePath(outputPath, workDir string) (string, error) {
    if outputPath == "" {
        return "", fmt.Errorf("output path is empty")
    }

    if filepath.IsAbs(outputPath) {
        return v.validateAbsolutePath(outputPath)
    }

    return v.validateRelativePath(outputPath, workDir)
}

func (v *DefaultPathValidator) validateAbsolutePath(path string) (string, error) {
    if strings.Contains(path, "..") {
        return "", fmt.Errorf("path traversal detected in absolute path: %s", path)
    }

    cleanPath := filepath.Clean(path)
    return cleanPath, nil
}

func (v *DefaultPathValidator) validateRelativePath(path, workDir string) (string, error) {
    if strings.Contains(path, "..") {
        return "", fmt.Errorf("path traversal detected in relative path: %s", path)
    }

    if workDir == "" {
        return "", fmt.Errorf("work directory is required for relative path")
    }

    fullPath := filepath.Join(workDir, path)
    cleanPath := filepath.Clean(fullPath)

    cleanWorkDir := filepath.Clean(workDir)
    if !strings.HasPrefix(cleanPath, cleanWorkDir) {
        return "", fmt.Errorf("relative path escapes work directory: %s", path)
    }

    return cleanPath, nil
}
```

#### 3.2.2 SecurityValidator拡張（出力ファイル書き込み権限チェック）

既存の `internal/runner/security/file_validation.go` を拡張して、出力キャプチャ用の書き込み権限チェック機能を追加します：

```go
// internal/runner/security/file_validation.go (拡張)

// ValidateOutputWritePermission validates write permission for output file creation
// This method is specifically designed for output capture functionality
func (v *Validator) ValidateOutputWritePermission(outputPath string, realUID int) error {
    if outputPath == "" {
        return fmt.Errorf("%w: empty output path", ErrInvalidPath)
    }

    // Ensure absolute path
    if !filepath.IsAbs(outputPath) {
        return fmt.Errorf("%w: output path must be absolute, got: %s", ErrInvalidPath, outputPath)
    }

    cleanPath := filepath.Clean(outputPath)
    dir := filepath.Dir(cleanPath)

    // Validate directory write permission
    if err := v.validateOutputDirectoryWritePermission(dir, realUID); err != nil {
        return fmt.Errorf("directory write permission check failed: %w", err)
    }

    // If file exists, validate file write permission
    if fileInfo, err := v.fs.Stat(cleanPath); err == nil {
        if err := v.validateOutputFileWritePermission(cleanPath, fileInfo, realUID); err != nil {
            return fmt.Errorf("file write permission check failed: %w", err)
        }
    } else if !os.IsNotExist(err) {
        return fmt.Errorf("failed to stat output file %s: %w", cleanPath, err)
    }

    return nil
}

// validateOutputDirectoryWritePermission checks if the user can write to the directory
func (v *Validator) validateOutputDirectoryWritePermission(dirPath string, realUID int) error {
    stat, err := v.fs.Stat(dirPath)
    if err != nil {
        if os.IsNotExist(err) {
            // Directory doesn't exist, check parent recursively
            parent := filepath.Dir(dirPath)
            if parent != dirPath {
                return v.validateOutputDirectoryWritePermission(parent, realUID)
            }
        }
        return fmt.Errorf("failed to stat directory %s: %w", dirPath, err)
    }

    if !stat.IsDir() {
        return fmt.Errorf("%w: %s is not a directory", ErrInvalidDirPermissions, dirPath)
    }

    return v.checkWritePermission(dirPath, stat, realUID)
}

// validateOutputFileWritePermission checks if the user can write to the existing file
func (v *Validator) validateOutputFileWritePermission(filePath string, fileInfo os.FileInfo, realUID int) error {
    if !fileInfo.Mode().IsRegular() {
        return fmt.Errorf("%w: %s is not a regular file", ErrInvalidFilePermissions, filePath)
    }

    return v.checkWritePermission(filePath, fileInfo, realUID)
}

// checkWritePermission performs the actual permission check for a given UID
func (v *Validator) checkWritePermission(path string, stat os.FileInfo, realUID int) error {
    sysstat, ok := stat.Sys().(*syscall.Stat_t)
    if !ok {
        return fmt.Errorf("%w: failed to get system info for %s", ErrInvalidFilePermissions, path)
    }

    // Check owner permissions
    if int(sysstat.Uid) == realUID {
        if stat.Mode()&0200 != 0 {
            return nil // Owner has write permission
        }
        return fmt.Errorf("%w: owner write permission denied for %s", ErrInvalidFilePermissions, path)
    }

    // Check group permissions
    if stat.Mode()&0020 != 0 {
        if v.isUserInGroup(realUID, sysstat.Gid) {
            return nil // User is in group and group has write permission
        }
    }

    // Check other permissions
    if stat.Mode()&0002 != 0 {
        return nil // Others have write permission
    }

    return fmt.Errorf("%w: write permission denied for %s", ErrInvalidFilePermissions, path)
}

// isUserInGroup checks if a user (by UID) is a member of a group (by GID)
// This is a simplified version that checks primary group and supplementary groups
func (v *Validator) isUserInGroup(uid int, gid uint32) bool {
    // Get user information
    user, err := user.LookupId(strconv.Itoa(uid))
    if err != nil {
        slog.Error("Failed to lookup user", "uid", uid, "error", err)
        return false
    }

    // Check primary group
    userGid, err := strconv.Atoi(user.Gid)
    if err == nil && uint32(userGid) == gid {
        return true
    }

    // Check supplementary groups using groupmembership
    if v.groupMembership != nil {
        members, err := v.groupMembership.GetGroupMembers(gid)
        if err == nil {
            for _, member := range members {
                if member == user.Username {
                    return true
                }
            }
        }
    }

    return false
}
```
```go
// internal/runner/output/permission.go
import (
    "fmt"
    "os"
    "os/user"
    "path/filepath"
    "strconv"
    "syscall"

    "github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

type PermissionChecker interface {
    CheckWritePermission(path string, uid int) error
}

type DefaultPermissionChecker struct {
    groupMembership *groupmembership.GroupMembership
}

func NewDefaultPermissionChecker() *DefaultPermissionChecker {
    return &DefaultPermissionChecker{
        groupMembership: groupmembership.New(),
    }
}

func (c *DefaultPermissionChecker) CheckWritePermission(path string, uid int) error {
    dir := filepath.Dir(path)

    if err := c.checkDirectoryWritePermission(dir, uid); err != nil {
        return err
    }

    if stat, err := os.Stat(path); err == nil {
        return c.checkFileWritePermission(path, stat, uid)
    }

    return nil
}

func (c *DefaultPermissionChecker) checkDirectoryWritePermission(dir string, uid int) error {
    stat, err := os.Stat(dir)
    if err != nil {
        if os.IsNotExist(err) {
            parent := filepath.Dir(dir)
            if parent != dir {
                return c.checkDirectoryWritePermission(parent, uid)
            }
        }
        return fmt.Errorf("failed to stat directory %s: %w", dir, err)
    }

    sysstat := stat.Sys().(*syscall.Stat_t)

    // Check owner permissions
    if int(sysstat.Uid) == uid {
        if stat.Mode()&0200 != 0 {
            return nil
        }
        return fmt.Errorf("owner write permission denied for directory: %s", dir)
    }

    // Check group permissions with proper membership verification
    if stat.Mode()&0020 != 0 {
        if err := c.checkGroupMembership(uid, uint32(sysstat.Gid)); err == nil {
            return nil
        }
    }

    // Check other permissions
    if stat.Mode()&0002 != 0 {
        return nil
    }

    return fmt.Errorf("write permission denied for directory: %s", dir)
}

func (c *DefaultPermissionChecker) checkFileWritePermission(path string, stat os.FileInfo, uid int) error {
    sysstat := stat.Sys().(*syscall.Stat_t)

    // Check owner permissions
    if int(sysstat.Uid) == uid && stat.Mode()&0200 != 0 {
        return nil
    }

    // Check group permissions with proper membership verification
    if stat.Mode()&0020 != 0 {
        if err := c.checkGroupMembership(uid, uint32(sysstat.Gid)); err == nil {
            return nil
        }
    }

    // Check other permissions
    if stat.Mode()&0002 != 0 {
        return nil
    }

    return fmt.Errorf("write permission denied for file: %s", path)
}

#### 3.2.3 safefileio.FileSystemの活用
出力キャプチャ機能では、既存の`safefileio.FileSystem`を活用してセキュリティを確保します：

```go
// internal/runner/output/file.go
import (
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

#### 3.2.4 FileManager（safefileio活用版）
```go
type FileManager interface {
    EnsureDirectory(path string) error
    CreateSecureTempFile(dir, prefix string) (*os.File, error)
    WriteToFile(path string, content []byte) error
}

type SafeFileManager struct {
    fs safefileio.FileSystem
}

func NewSafeFileManager() *SafeFileManager {
    return &SafeFileManager{
        fs: safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
    }
}

func (f *SafeFileManager) EnsureDirectory(path string) error {
    if err := os.MkdirAll(path, 0755); err != nil {
        return fmt.Errorf("failed to create directory %s: %w", path, err)
    }
    return nil
}

func (f *SafeFileManager) CreateSecureTempFile(dir, prefix string) (*os.File, error) {
    pattern := prefix + "_*.tmp"
    return os.CreateTemp(dir, pattern)
}

func (f *SafeFileManager) WriteToFile(path string, content []byte) error {
    // safefileio.SafeWriteFileOverwriteを使用して安全にファイル書き込み
    return safefileio.SafeWriteFileOverwrite(path, content, 0600)
}
```

## 4. 定数とパラメータ仕様

### 4.1 デフォルト値定数
```go
// internal/runner/output/constants.go
const (
    // Buffer settings
    DefaultBufferSize = 64 * 1024     // 64KB buffer
    MaxBufferSize     = 1024 * 1024   // 1MB maximum buffer

    // Size limits
    DefaultMaxOutputSize = 10 * 1024 * 1024   // 10MB default limit
    AbsoluteMaxSize      = 100 * 1024 * 1024  // 100MB absolute limit

    // File permissions
    TempFileMode   = 0600   // Temporary file permissions
    OutputFileMode = 0600   // Output file permissions
    DirectoryMode  = 0755   // Directory permissions

    // Temporary file patterns
    TempSuffix     = ".tmp"
    TempPattern    = "_*.tmp"
)
```

### 4.2 設定検証ルール
```go
// internal/runner/output/validation.go
type ConfigValidator struct{}

func (v *ConfigValidator) ValidateGlobalConfig(config *GlobalConfig) error {
    // MaxOutputSize検証
    if config.MaxOutputSize < 0 {
        return fmt.Errorf("max_output_size cannot be negative: %d", config.MaxOutputSize)
    }

    if config.MaxOutputSize > AbsoluteMaxSize {
        return fmt.Errorf("max_output_size exceeds absolute limit: %d > %d",
            config.MaxOutputSize, AbsoluteMaxSize)
    }

    return nil
}

func (v *ConfigValidator) ValidateCommand(cmd *Command) error {
    // Output フィールド検証
    if cmd.Output != "" {
        // 基本的なパス検証
        if strings.TrimSpace(cmd.Output) == "" {
            return fmt.Errorf("output path cannot be whitespace only")
        }

        // 危険なパターンの検出
        dangerousPatterns := []string{
            "/dev/", "/proc/", "/sys/",
            "passwd", "shadow", "sudoers",
        }

        outputLower := strings.ToLower(cmd.Output)
        for _, pattern := range dangerousPatterns {
            if strings.Contains(outputLower, pattern) {
                return fmt.Errorf("output path contains dangerous pattern: %s", pattern)
            }
        }
    }

    return nil
}
```

## 5. ResourceManager統合仕様

### 5.1 NormalResourceManager拡張

```go
// internal/runner/resource/manager.go (拡張)
type NormalResourceManager struct {
    // 既存フィールド
    executor         executor.CommandExecutor
    pathResolver     PathResolver
    privilegeManager runnertypes.PrivilegeManager

    // 新規追加フィールド
    outputManager    output.OutputCaptureManager
    maxOutputSize    int64
    logger           Logger
}

func NewNormalResourceManager(
    executor executor.CommandExecutor,
    pathResolver PathResolver,
    privilegeManager runnertypes.PrivilegeManager,
    outputManager output.OutputCaptureManager,
    maxOutputSize int64,
    logger Logger,
) *NormalResourceManager {
    return &NormalResourceManager{
        executor:         executor,
        pathResolver:     pathResolver,
        privilegeManager: privilegeManager,
        outputManager:    outputManager,
        maxOutputSize:    maxOutputSize,
        logger:           logger,
    }
}
```

### 5.2 ExecuteCommand拡張

```go
func (m *NormalResourceManager) ExecuteCommand(
    ctx context.Context,
    cmd *config.Command,
    workDir string,
    env []string,
) (*runnertypes.CommandResult, error) {
    // 出力キャプチャ準備
    var capture *output.OutputCapture
    var outputConfig *output.OutputConfig

    if cmd.Output != "" {
        // UID/GID取得
        realUID := os.Getuid()
        realGID := os.Getgid()

        outputConfig = &output.OutputConfig{
            OutputPath:  cmd.Output,
            WorkDir:     workDir,
            MaxSize:     m.maxOutputSize,
            RealUID:     realUID,
            RealGID:     realGID,
            CommandName: cmd.Name,
        }

        var err error
        capture, err = m.outputManager.PrepareOutput(outputConfig)
        if err != nil {
            return nil, fmt.Errorf("failed to prepare output capture: %w", err)
        }

        // エラー時のクリーンアップを確保
        defer func() {
            if capture != nil {
                m.outputManager.CleanupOutput(capture)
            }
        }()
    }

    // コマンド実行
    result, err := m.executeCommandWithCapture(ctx, cmd, workDir, env, capture)

    // 出力キャプチャ完了処理
    if capture != nil {
        if err == nil {
            // 成功時：出力を最終化
            if finalizeErr := m.outputManager.FinalizeOutput(capture); finalizeErr != nil {
                m.logger.Error("Failed to finalize output capture", "error", finalizeErr)
                // 実行は成功したが出力保存に失敗した場合のエラーハンドリング
                return result, fmt.Errorf("command succeeded but output capture failed: %w", finalizeErr)
            }
            capture = nil // クリーンアップ不要
        }
        // 失敗時：deferでクリーンアップが実行される
    }

    return result, err
}

func (m *NormalResourceManager) executeCommandWithCapture(
    ctx context.Context,
    cmd *config.Command,
    workDir string,
    env []string,
    capture *output.OutputCapture,
) (*runnertypes.CommandResult, error) {

    // 既存のExecutor呼び出しロジックを拡張
    execConfig := &executor.ExecuteConfig{
        Name:      cmd.Name,
        Cmd:       cmd.Cmd,
        Args:      cmd.Args,
        WorkDir:   workDir,
        Env:       env,
        RunAsUser: cmd.RunAsUser,
        Timeout:   cmd.Timeout,
    }

    // 標準出力は常にRunner標準出力に書き込み、output指定時はファイルにもTee
    if capture != nil {
        // Tee機能付きWriterを使用（Runner標準出力 + ファイル出力）
        execConfig.StdoutWriter = NewTeeOutputWriter(
            os.Stdout,  // Runner標準出力
            capture,    // ファイル出力先
            m.outputManager,
        )
    } else {
        // 通常通りRunner標準出力のみ
        execConfig.StdoutWriter = os.Stdout
    }

    return m.executor.Execute(ctx, execConfig)
}
```

### 5.3 TeeOutputWriter（Tee機能付きWriter）

```go
// internal/runner/output/writer.go
type TeeOutputWriter struct {
    capture        *OutputCapture
    manager        OutputCaptureManager
    originalWriter io.Writer  // Runner標準出力（常に書き込み）
}

func NewTeeOutputWriter(originalWriter io.Writer, capture *OutputCapture, manager OutputCaptureManager) *TeeOutputWriter {
    return &TeeOutputWriter{
        capture:        capture,
        manager:        manager,
        originalWriter: originalWriter,
    }
}

func (w *TeeOutputWriter) Write(data []byte) (int, error) {
    // 常にRunner標準出力に書き込み
    if _, err := w.originalWriter.Write(data); err != nil {
        // 標準出力への書き込み失敗は致命的エラー
        return 0, fmt.Errorf("failed to write to stdout: %w", err)
    }

    // ファイルへの書き込み（output指定時のみ）
    if w.capture != nil {
        if err := w.manager.WriteOutput(w.capture, data); err != nil {
            // ファイル書き込み失敗時はプロセスを停止
            return 0, fmt.Errorf("failed to write to output file: %w", err)
        }
    }

    return len(data), nil
}
```

### 5.4 DryRunResourceManager拡張

```go
// internal/runner/resource/dryrun.go (拡張)
func (m *DryRunResourceManager) ExecuteCommand(
    ctx context.Context,
    cmd *config.Command,
    workDir string,
    env []string,
) (*runnertypes.CommandResult, error) {

    // 既存のDry-Run処理
    result := &runnertypes.CommandResult{
        Name:     cmd.Name,
        Success:  true,
        ExitCode: 0,
        Output:   fmt.Sprintf("[DRY-RUN] Would execute: %s %v", cmd.Cmd, cmd.Args),
    }

    // 出力キャプチャ分析
    if cmd.Output != "" {
        analysis, err := m.analyzeOutputCapture(cmd, workDir)
        if err != nil {
            m.logger.Warn("Output capture analysis failed", "error", err)
        } else {
            // DryRunResultに出力分析を追加
            m.addOutputAnalysisToResult(result, analysis)
        }
    }

    return result, nil
}

func (m *DryRunResourceManager) analyzeOutputCapture(cmd *config.Command, workDir string) (*output.OutputAnalysis, error) {
    realUID := os.Getuid()
    realGID := os.Getgid()

    outputConfig := &output.OutputConfig{
        OutputPath:  cmd.Output,
        WorkDir:     workDir,
        MaxSize:     m.maxOutputSize,
        RealUID:     realUID,
        RealGID:     realGID,
        CommandName: cmd.Name,
    }

    return m.outputManager.AnalyzeOutput(outputConfig)
}
```

## 6. Executor統合仕様

### 6.1 ExecuteConfig拡張

```go
// internal/runner/executor/types.go (拡張)
type ExecuteConfig struct {
    // 既存フィールド
    Name      string
    Cmd       string
    Args      []string
    WorkDir   string
    Env       []string
    RunAsUser string
    Timeout   int

    // 新規追加フィールド
    StdoutWriter io.Writer  // 標準出力の書き込み先（nilの場合は従来通り）
}
```

### 6.2 Execute実装拡張

```go
// internal/runner/executor/executor.go (拡張)
func (e *DefaultCommandExecutor) Execute(ctx context.Context, config *ExecuteConfig) (*runnertypes.CommandResult, error) {
    // 既存のコマンド準備処理...

    cmd := exec.CommandContext(ctx, config.Cmd, config.Args...)

    // 標準出力の設定
    var stdoutBuf bytes.Buffer
    if config.StdoutWriter != nil {
        // 指定されたWriterとバッファの両方に書き込み
        // （TeeOutputWriterが既にRunner標準出力とファイルへのTeeを処理）
        cmd.Stdout = io.MultiWriter(config.StdoutWriter, &stdoutBuf)
    } else {
        // 従来通りバッファのみ
        cmd.Stdout = &stdoutBuf
    }

    // 標準エラーは常にバッファに書き込み（キャプチャ対象外）
    var stderrBuf bytes.Buffer
    cmd.Stderr = &stderrBuf

    // 既存の実行処理...
    err := cmd.Run()

    // 結果構築
    result := &runnertypes.CommandResult{
        Name:     config.Name,
        Success:  err == nil,
        ExitCode: cmd.ProcessState.ExitCode(),
        Output:   stdoutBuf.String(),
        Error:    stderrBuf.String(),
    }

    return result, err
}
```

## 7. ユーティリティ関数仕様

### 7.1 セキュリティリスク評価

```go
// internal/runner/output/security.go
import (
    "os/user"
    "path/filepath"
    "strings"
)

func (m *DefaultOutputCaptureManager) evaluateSecurityRisk(path, workDir string) runnertypes.RiskLevel {
    pathLower := strings.ToLower(path)

    // Critical: システム重要ファイル
    criticalPatterns := []string{
        "/etc/passwd", "/etc/shadow", "/etc/sudoers",
        "/boot/", "/sys/", "/proc/",
        "authorized_keys", "id_rsa", "id_ed25519",
    }

    for _, pattern := range criticalPatterns {
        if strings.Contains(pathLower, pattern) {
            return runnertypes.RiskLevelCritical
        }
    }

    // High: システムディレクトリ
    highPatterns := []string{
        "/etc/", "/var/log/", "/usr/bin/", "/usr/sbin/",
        ".ssh/", ".gnupg/",
    }

    for _, pattern := range highPatterns {
        if strings.Contains(pathLower, pattern) {
            return runnertypes.RiskLevelHigh
        }
    }

    // Low: WorkDir内のファイル
    if workDir != "" {
        cleanWorkDir := filepath.Clean(workDir)
        cleanPath := filepath.Clean(path)
        if strings.HasPrefix(cleanPath, cleanWorkDir) {
            return runnertypes.RiskLevelLow
        }
    }

    // Low: 現在ユーザーのホームディレクトリ内
    if currentUser, err := user.Current(); err == nil {
        homeDir := currentUser.HomeDir
        cleanHomePath := filepath.Clean(homeDir)
        cleanPath := filepath.Clean(path)
        if strings.HasPrefix(cleanPath, cleanHomePath) {
            return runnertypes.RiskLevelLow
        }
    }

    // Medium: その他の場所
    return runnertypes.RiskLevelMedium
}
```

### 7.2 一時ファイル命名規則

一時ファイルの命名は`os.CreateTemp`に委譲する：

```go
// パターン例: "output_file_20241215_120345_123456.tmp"
pattern := prefix + "_*.tmp"
file, err := os.CreateTemp(dir, pattern)
// os.CreateTempが自動的にランダムな文字列を生成して安全なファイル名を作成
```

このアプローチにより：
- 一意性が保証される
- セキュリティが確保される（0600権限で作成）
- レースコンディションが回避される
- システム標準の実装を利用できる

## 8. テスト仕様

### 8.1 単体テスト対象

```go
// Test files structure:
// internal/runner/output/manager_test.go
// internal/runner/output/path_test.go
// internal/runner/output/permission_test.go
// internal/runner/output/file_test.go
// internal/runner/output/security_test.go
```

### 8.2 主要テストケース

#### 8.2.1 パス検証テスト
```go
func TestPathValidator_ValidateAndResolvePath(t *testing.T) {
    tests := []struct {
        name       string
        outputPath string
        workDir    string
        wantErr    bool
        errType    string
    }{
        {
            name:       "valid_absolute_path",
            outputPath: "/tmp/output.txt",
            workDir:    "/home/user",
            wantErr:    false,
        },
        {
            name:       "valid_relative_path",
            outputPath: "output/result.txt",
            workDir:    "/home/user/project",
            wantErr:    false,
        },
        {
            name:       "path_traversal_absolute",
            outputPath: "/tmp/../etc/passwd",
            workDir:    "/home/user",
            wantErr:    true,
            errType:    "path traversal detected",
        },
        {
            name:       "path_traversal_relative",
            outputPath: "../../../etc/passwd",
            workDir:    "/home/user/project",
            wantErr:    true,
            errType:    "path traversal detected",
        },
        {
            name:       "empty_path",
            outputPath: "",
            workDir:    "/home/user",
            wantErr:    true,
            errType:    "output path is empty",
        },
        // 追加のテストケース...
    }

    validator := NewDefaultPathValidator()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := validator.ValidateAndResolvePath(tt.outputPath, tt.workDir)

            if tt.wantErr {
                assert.Error(t, err)
                if tt.errType != "" {
                    assert.Contains(t, err.Error(), tt.errType)
                }
            } else {
                assert.NoError(t, err)
                assert.NotEmpty(t, result)
                assert.True(t, filepath.IsAbs(result))
            }
        })
    }
}
```

#### 8.2.2 出力キャプチャテスト
```go
func TestOutputCaptureManager_Integration(t *testing.T) {
    tempDir := t.TempDir()
    outputPath := filepath.Join(tempDir, "test_output.txt")

    logger := &MockLogger{}
    manager := NewDefaultOutputCaptureManager(logger)

    config := &OutputConfig{
        OutputPath:  outputPath,
        WorkDir:     tempDir,
        MaxSize:     1024 * 1024, // 1MB
        RealUID:     os.Getuid(),
        RealGID:     os.Getgid(),
        CommandName: "test_command",
    }

    // Prepare（os.CreateTempを使用した一時ファイル作成）
    capture, err := manager.PrepareOutput(config)
    require.NoError(t, err)
    require.NotNil(t, capture)

    // 一時ファイルが適切な権限で作成されていることを確認
    tempStat, err := os.Stat(capture.TempPath)
    require.NoError(t, err)
    assert.Equal(t, os.FileMode(0600), tempStat.Mode().Perm())

    // Write
    testData := []byte("test output data\n")
    err = manager.WriteOutput(capture, testData)
    require.NoError(t, err)

    // Finalize
    err = manager.FinalizeOutput(capture)
    require.NoError(t, err)

    // Verify
    content, err := os.ReadFile(outputPath)
    require.NoError(t, err)
    assert.Equal(t, testData, content)

    // Check permissions（一時ファイルの権限が継承されている）
    stat, err := os.Stat(outputPath)
    require.NoError(t, err)
    assert.Equal(t, os.FileMode(0600), stat.Mode().Perm())

    // 一時ファイルが削除されていることを確認
    _, err = os.Stat(capture.TempPath)
    assert.True(t, os.IsNotExist(err), "temporary file should be cleaned up")
}
```

### 8.3 セキュリティテスト

```go
func TestSecurityValidation(t *testing.T) {
    tests := []struct {
        name         string
        path         string
        expectRisk   runnertypes.RiskLevel
    }{
        {
            name:       "critical_passwd_file",
            path:       "/etc/passwd",
            expectRisk: runnertypes.RiskLevelCritical,
        },
        {
            name:       "high_etc_directory",
            path:       "/etc/myconfig.conf",
            expectRisk: runnertypes.RiskLevelHigh,
        },
        {
            name:       "low_home_directory",
            path:       "/home/user/output.txt",
            expectRisk: runnertypes.RiskLevelLow,
        },
    }

    manager := NewDefaultOutputCaptureManager(&MockLogger{})

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            risk := manager.evaluateSecurityRisk(tt.path)
            assert.Equal(t, tt.expectRisk, risk)
        })
    }
}

func TestSecurityValidator_OutputWritePermission(t *testing.T) {
    // Create a temporary file with specific permissions
    tempFile, err := os.CreateTemp("", "test_output_*")
    require.NoError(t, err)
    defer os.Remove(tempFile.Name())
    defer tempFile.Close()

    // Set up security validator
    config := &ValidationConfig{
        MaxPathLength: 4096,
        RequiredFilePermissions: 0600,
    }
    validator, err := NewSecurityValidator(config, nil)
    require.NoError(t, err)

    // Get current user info
    currentUser, err := user.Current()
    require.NoError(t, err)

    currentUID, err := strconv.Atoi(currentUser.Uid)
    require.NoError(t, err)

    // Test permission check for output file writing
    err = validator.ValidateOutputWritePermission(tempFile.Name(), currentUID)

    // The result depends on actual group membership, so we mainly test that it doesn't panic
    // and returns a reasonable result
    if err != nil {
        t.Logf("Permission check failed (expected if user not in group): %v", err)
    } else {
        t.Log("Permission check passed (user has write access)")
    }
}
```

## 9. エラーハンドリング実装詳細

### 9.1 エラーラッピング戦略

```go
// エラーの階層化と詳細情報の保持
func wrapOutputError(errType ErrorType, phase ExecutionPhase, path, command string, cause error) error {
    return &OutputCaptureError{
        Type:    errType,
        Phase:   phase,
        Path:    path,
        Command: command,
        Cause:   cause,
    }
}

// エラーの判定と処理
func handleOutputError(err error) {
    var outputErr *OutputCaptureError
    if errors.As(err, &outputErr) {
        switch outputErr.Type {
        case ErrorTypeSizeLimit:
            // サイズ制限エラーの場合はプロセス強制終了
            // 既にWriteOutputでエラーが返されているため、
            // 呼び出し側でプロセス終了処理を実行
        case ErrorTypePathValidation:
            // パス検証エラーの場合はコマンド実行前に中止
        case ErrorTypePermission:
            // 権限エラーの場合は適切なログとエラー返却
        }
    }
}
```

### 9.2 リソースリーク防止

```go
// defer文を使用したリソース管理
func (m *NormalResourceManager) ExecuteCommand(...) (*runnertypes.CommandResult, error) {
    var capture *output.OutputCapture

    if cmd.Output != "" {
        capture, err := m.outputManager.PrepareOutput(outputConfig)
        if err != nil {
            return nil, err
        }

        // 確実なクリーンアップ
        defer func() {
            if capture != nil {
                if cleanupErr := m.outputManager.CleanupOutput(capture); cleanupErr != nil {
                    m.logger.Error("Failed to cleanup output capture", "error", cleanupErr)
                }
            }
        }()
    }

    // 実行処理...

    if capture != nil && err == nil {
        // 成功時のファイナライズ
        if finalizeErr := m.outputManager.FinalizeOutput(capture); finalizeErr != nil {
            return result, finalizeErr
        }
        capture = nil // deferでのクリーンアップをスキップ
    }

    return result, err
}
```

この詳細仕様書により、要件定義書とアーキテクチャ設計書で定義された機能を実装レベルで具体化し、安全で効率的なコマンド出力キャプチャ機能を実現できます。
