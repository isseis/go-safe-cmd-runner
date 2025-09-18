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
    TempPath     string         // 一時ファイルパス
    TempFile     *os.File       // 一時ファイルハンドル
    Buffer       *bufio.Writer  // バッファライター
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
    EstimatedSize   string        // 推定サイズ（"Unknown"等）
    SecurityRisk    SecurityLevel // セキュリティリスク評価
    MaxSizeLimit    int64         // サイズ制限値
    ErrorMessage    string        // エラーメッセージ（問題がある場合）
}

type SecurityLevel int
const (
    SecurityLevelLow SecurityLevel = iota
    SecurityLevelMedium
    SecurityLevelHigh
    SecurityLevelCritical
)

func (s SecurityLevel) String() string {
    switch s {
    case SecurityLevelLow:
        return "LOW"
    case SecurityLevelMedium:
        return "MEDIUM"
    case SecurityLevelHigh:
        return "HIGH"
    case SecurityLevelCritical:
        return "CRITICAL"
    default:
        return "UNKNOWN"
    }
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
    permissionChecker PermissionChecker
    logger           Logger
}

func NewDefaultOutputCaptureManager(logger Logger) *DefaultOutputCaptureManager {
    fs := NewDefaultExtendedFileSystem()
    return &DefaultOutputCaptureManager{
        pathValidator:    NewDefaultPathValidator(),
        fileManager:      NewDefaultFileManager(fs),
        permissionChecker: NewDefaultPermissionChecker(),
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
    if err := m.permissionChecker.CheckWritePermission(resolvedPath, config.RealUID); err != nil {
        return nil, &OutputCaptureError{
            Type:    ErrorTypePermission,
            Phase:   PhasePreparation,
            Path:    resolvedPath,
            Cause:   err,
            Command: config.CommandName,
        }
    }

    // 3. ディレクトリ作成
    if err := m.fileManager.EnsureDirectory(filepath.Dir(resolvedPath), config.RealUID, config.RealGID); err != nil {
        return nil, &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhasePreparation,
            Path:    resolvedPath,
            Cause:   err,
            Command: config.CommandName,
        }
    }

    // 4. 一時ファイル作成（os.CreateTempを使用して安全に作成）
    tempFile, err := m.fileManager.CreateTempFile(filepath.Dir(resolvedPath), filepath.Base(resolvedPath), config.RealUID, config.RealGID)
    if err != nil {
        return nil, &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhasePreparation,
            Path:    resolvedPath,
            Cause:   err,
            Command: config.CommandName,
        }
    }
    tempPath := tempFile.Name()

    // 5. OutputCapture構造体作成
    capture := &OutputCapture{
        Config:      config,
        OutputPath:  resolvedPath,
        TempPath:    tempPath,
        TempFile:    tempFile,
        Buffer:      bufio.NewWriterSize(tempFile, DefaultBufferSize),
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

    // データ書き込み
    n, err := capture.Buffer.Write(data)
    if err != nil {
        return &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhaseExecution,
            Path:    capture.TempPath,
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
    // 1. ファイルクローズ（bufio.Writerは自動的にフラッシュされる）
    if err := capture.TempFile.Close(); err != nil {
        return &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhaseFinalization,
            Path:    capture.TempPath,
            Cause:   err,
            Command: capture.Config.CommandName,
        }
    }

    // 2. 原子的ファイル移動
    if err := m.fileManager.AtomicMove(capture.TempPath, capture.OutputPath); err != nil {
        os.Remove(capture.TempPath)
        return &OutputCaptureError{
            Type:    ErrorTypeFileSystem,
            Phase:   PhaseFinalization,
            Path:    capture.OutputPath,
            Cause:   err,
            Command: capture.Config.CommandName,
        }
    }

    // 3. ログ記録
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
    var errs []error

    // 1. ファイルクローズ（既にクローズされている場合もある）
    if capture.TempFile != nil {
        if err := capture.TempFile.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
            errs = append(errs, err)
        }
    }

    // 2. 一時ファイル削除
    if capture.TempPath != "" {
        if err := os.Remove(capture.TempPath); err != nil && !os.IsNotExist(err) {
            errs = append(errs, err)
        }
    }

    // エラーがあればまとめて返す
    if len(errs) > 0 {
        return &OutputCaptureError{
            Type:    ErrorTypeCleanup,
            Phase:   PhaseCleanup,
            Path:    capture.TempPath,
            Cause:   fmt.Errorf("cleanup errors: %v", errs),
            Command: capture.Config.CommandName,
        }
    }

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
        analysis.SecurityRisk = SecurityLevelCritical
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
    if err := m.permissionChecker.CheckWritePermission(resolvedPath, config.RealUID); err != nil {
        analysis.WritePermission = false
        if analysis.ErrorMessage == "" {
            analysis.ErrorMessage = fmt.Sprintf("Permission check failed: %v", err)
        }
    } else {
        analysis.WritePermission = true
    }

    // 4. セキュリティリスク評価
    analysis.SecurityRisk = m.evaluateSecurityRisk(resolvedPath)

    // 5. 推定サイズ設定
    analysis.EstimatedSize = "Unknown"

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

    evalPath, err := filepath.EvalSymlinks(filepath.Dir(cleanPath))
    if err != nil && !os.IsNotExist(err) {
        return "", fmt.Errorf("failed to evaluate symlinks: %w", err)
    }

    if evalPath != "" {
        cleanPath = filepath.Join(evalPath, filepath.Base(cleanPath))
    }

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

#### 3.2.2 PermissionChecker
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

// checkGroupMembership verifies if a user (by UID) is a member of a group (by GID)
func (c *DefaultPermissionChecker) checkGroupMembership(uid int, gid uint32) error {
    // Get username from UID
    user, err := user.LookupId(strconv.Itoa(uid))
    if err != nil {
        return fmt.Errorf("failed to lookup user with UID %d: %w", uid, err)
    }

    // Get group members
    members, err := c.groupMembership.GetGroupMembers(gid)
    if err != nil {
        return fmt.Errorf("failed to get group members for GID %d: %w", gid, err)
    }

    // Check if user is in group members
    for _, member := range members {
        if member == user.Username {
            return nil
        }
    }

    return fmt.Errorf("user %s (UID %d) is not a member of group with GID %d", user.Username, uid, gid)
}
```

#### 3.2.3 ExtendedFileSystem（common.FileSystemの拡張）
```go
// internal/runner/output/file.go
import (
    "github.com/isseis/go-safe-cmd-runner/internal/common"
)

// ExtendedFileSystem extends common.FileSystem with additional functionality needed for output capture
type ExtendedFileSystem interface {
    common.FileSystem

    // CreateTempFile creates a temporary file with the given pattern
    CreateTempFile(dir, pattern string) (*os.File, error)

    // Stat returns file information
    Stat(path string) (os.FileInfo, error)

    // Chown changes the ownership of the file
    Chown(path string, uid, gid int) error

    // Chmod changes the permissions of the file
    Chmod(path string, mode os.FileMode) error

    // Rename renames (moves) oldpath to newpath
    Rename(oldpath, newpath string) error

    // Open opens a file for reading
    Open(name string) (*os.File, error)

    // OpenFile opens a file with specified flags and permissions
    OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)

    // MkdirAll creates directories recursively
    MkdirAll(path string, perm os.FileMode) error
}

// DefaultExtendedFileSystem implements ExtendedFileSystem
type DefaultExtendedFileSystem struct {
    *common.DefaultFileSystem
}

func NewDefaultExtendedFileSystem() *DefaultExtendedFileSystem {
    return &DefaultExtendedFileSystem{
        DefaultFileSystem: common.NewDefaultFileSystem(),
    }
}

func (fs *DefaultExtendedFileSystem) CreateTempFile(dir, pattern string) (*os.File, error) {
    return os.CreateTemp(dir, pattern)
}

func (fs *DefaultExtendedFileSystem) Stat(path string) (os.FileInfo, error) {
    return os.Stat(path)
}

func (fs *DefaultExtendedFileSystem) Chown(path string, uid, gid int) error {
    return os.Chown(path, uid, gid)
}

func (fs *DefaultExtendedFileSystem) Chmod(path string, mode os.FileMode) error {
    return os.Chmod(path, mode)
}

func (fs *DefaultExtendedFileSystem) Rename(oldpath, newpath string) error {
    return os.Rename(oldpath, newpath)
}

func (fs *DefaultExtendedFileSystem) Open(name string) (*os.File, error) {
    return os.Open(name)
}

func (fs *DefaultExtendedFileSystem) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
    return os.OpenFile(name, flag, perm)
}

func (fs *DefaultExtendedFileSystem) MkdirAll(path string, perm os.FileMode) error {
    return os.MkdirAll(path, perm)
}

#### 3.2.4 FileManager
type FileManager interface {
    EnsureDirectory(path string, uid, gid int) error
    CreateTempFile(dir, prefix string, uid, gid int) (*os.File, error)
    AtomicMove(src, dst string) error
}

type DefaultFileManager struct {
    fs ExtendedFileSystem
}

func NewDefaultFileManager(fs ExtendedFileSystem) *DefaultFileManager {
    return &DefaultFileManager{
        fs: fs,
    }
}

func (f *DefaultFileManager) EnsureDirectory(path string, uid, gid int) error {
    isDir, err := f.fs.IsDir(path)
    if err == nil && isDir {
        return nil
    }

    exists, err := f.fs.FileExists(path)
    if err == nil && exists && !isDir {
        return fmt.Errorf("path exists but is not a directory: %s", path)
    }

    if err := f.fs.MkdirAll(path, 0755); err != nil {
        return fmt.Errorf("failed to create directory %s: %w", path, err)
    }

    if err := f.fs.Chown(path, uid, gid); err != nil {
        return fmt.Errorf("failed to set ownership for directory %s: %w", path, err)
    }

    return nil
}

func (f *DefaultFileManager) CreateTempFile(dir, prefix string, uid, gid int) (*os.File, error) {
    pattern := prefix + "_*.tmp"
    file, err := f.fs.CreateTempFile(dir, pattern)
    if err != nil {
        return nil, fmt.Errorf("failed to create temp file in %s with pattern %s: %w", dir, pattern, err)
    }

    tempPath := file.Name()

    if err := f.fs.Chown(tempPath, uid, gid); err != nil {
        file.Close()
        f.fs.Remove(tempPath)
        return nil, fmt.Errorf("failed to set ownership for temp file %s: %w", tempPath, err)
    }

    return file, nil
}

func (f *DefaultFileManager) AtomicMove(src, dst string) error {
    if err := f.fs.Rename(src, dst); err != nil {
        if errors.Is(err, syscall.EXDEV) {
            return f.copyAndRemove(src, dst)
        }
        return fmt.Errorf("failed to move file from %s to %s: %w", src, dst, err)
    }

    return nil
}

func (f *DefaultFileManager) copyAndRemove(src, dst string) error {
    srcStat, err := f.fs.Stat(src)
    if err != nil {
        return err
    }

    srcFile, err := f.fs.Open(src)
    if err != nil {
        return err
    }
    defer srcFile.Close()

    dstFile, err := f.fs.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcStat.Mode().Perm())
    if err != nil {
        return err
    }
    defer dstFile.Close()

    srcSysStat := srcStat.Sys().(*syscall.Stat_t)
    if err := f.fs.Chown(dst, int(srcSysStat.Uid), int(srcSysStat.Gid)); err != nil {
        f.fs.Remove(dst)
        return err
    }

    if _, err := io.Copy(dstFile, srcFile); err != nil {
        f.fs.Remove(dst)
        return err
    }

    if err := dstFile.Sync(); err != nil {
        f.fs.Remove(dst)
        return err
    }

    return f.fs.Remove(src)
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
func (m *DefaultOutputCaptureManager) evaluateSecurityRisk(path string) SecurityLevel {
    pathLower := strings.ToLower(path)

    // Critical: システム重要ファイル
    criticalPatterns := []string{
        "/etc/passwd", "/etc/shadow", "/etc/sudoers",
        "/boot/", "/sys/", "/proc/",
        "authorized_keys", "id_rsa", "id_ed25519",
    }

    for _, pattern := range criticalPatterns {
        if strings.Contains(pathLower, pattern) {
            return SecurityLevelCritical
        }
    }

    // High: システムディレクトリ
    highPatterns := []string{
        "/etc/", "/var/log/", "/usr/bin/", "/usr/sbin/",
        ".ssh/", ".gnupg/",
    }

    for _, pattern := range highPatterns {
        if strings.Contains(pathLower, pattern) {
            return SecurityLevelHigh
        }
    }

    // Medium: ホームディレクトリ外
    if !strings.HasPrefix(path, os.Getenv("HOME")) {
        return SecurityLevelMedium
    }

    // Low: ホームディレクトリ内の通常ファイル
    return SecurityLevelLow
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
        expectRisk   SecurityLevel
    }{
        {
            name:       "critical_passwd_file",
            path:       "/etc/passwd",
            expectRisk: SecurityLevelCritical,
        },
        {
            name:       "high_etc_directory",
            path:       "/etc/myconfig.conf",
            expectRisk: SecurityLevelHigh,
        },
        {
            name:       "low_home_directory",
            path:       "/home/user/output.txt",
            expectRisk: SecurityLevelLow,
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

func TestPermissionChecker_GroupMembership(t *testing.T) {
    // Create a temporary file with specific group permissions
    tempFile, err := os.CreateTemp("", "test_group_*")
    require.NoError(t, err)
    defer os.Remove(tempFile.Name())
    defer tempFile.Close()

    // Set group write permission
    err = os.Chmod(tempFile.Name(), 0620) // owner rw-, group -w-, other ---
    require.NoError(t, err)

    checker := NewDefaultPermissionChecker()

    // Get current user info
    currentUser, err := user.Current()
    require.NoError(t, err)

    currentUID, err := strconv.Atoi(currentUser.Uid)
    require.NoError(t, err)

    // Test permission check (this will use groupmembership to verify actual membership)
    err = checker.CheckWritePermission(tempFile.Name(), currentUID)

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
