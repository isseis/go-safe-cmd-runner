# 詳細仕様書: Shebang インタープリタ追跡

## 0. 既存機能活用方針

| 既存コンポーネント | 活用方法 |
|---|---|
| `filevalidator.Validator.SaveRecord(force=true)` | インタープリタバイナリの独立 Record 作成 |
| `filevalidator.Validator.LoadRecord()` | runner 側でのインタープリタ Record 読み取り |
| `fileanalysis.Store.Update()` | スクリプト Record への `ShebangInterpreter` フィールド書き込み |
| `filevalidator.Validator.Verify()` | runner 側でのインタープリタハッシュ検証 |
| `filepath.EvalSymlinks()` | シンボリックリンク解決（DynLibDeps と同一パターン） |
| `exec.LookPath()` | `env` 形式での PATH 解決 |

---

## 1. 実装詳細仕様

### 1.1 パッケージ構成詳細

| パッケージ | ファイル | 変更種別 | 概要 |
|---|---|---|---|
| `internal/shebang` | `parser.go` | **新規** | shebang 解析ロジック |
| `internal/shebang` | `parser_test.go` | **新規** | shebang 解析テスト |
| `internal/shebang` | `errors.go` | **新規** | shebang 固有エラー型 |
| `internal/fileanalysis` | `schema.go` | 変更 | `ShebangInterpreterInfo` 型追加、`Record` フィールド追加、スキーマ v11 |
| `internal/filevalidator` | `validator.go` | 変更 | `updateAnalysisRecord` に shebang 解析フェーズ追加 |
| `internal/verification` | `manager.go` | 変更 | `VerifyCommandShebangInterpreter` メソッド追加 |
| `internal/verification` | `interfaces.go` | 変更 | `ManagerInterface` にメソッド追加 |
| `internal/verification` | `testing/testify_mocks.go` | 変更 | モック更新 |
| `internal/runner` | `group_executor.go` | 変更 | インタープリタ検証ループ追加 |

### 1.2 型定義とインターフェース

#### 1.2.1 `internal/shebang/parser.go`

```go
package shebang

import (
    "bytes"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

const (
    // maxShebangBytes is the maximum number of bytes to read for shebang detection.
    // Matches Linux kernel's BINPRM_BUF_SIZE.
    maxShebangBytes = 256

    // shebangPrefix is the magic bytes for shebang detection.
    shebangPrefix = "#!"
)

// ShebangInfo holds the parsed result of a shebang line.
type ShebangInfo struct {
    // InterpreterPath is the absolute path to the interpreter binary,
    // resolved via filepath.EvalSymlinks.
    // For env form (e.g., "#!/usr/bin/env python3"), this is the resolved
    // path of env (e.g., "/usr/bin/env").
    // For direct form (e.g., "#!/bin/sh"), this is the resolved path of
    // the interpreter (e.g., "/usr/bin/dash" if /bin/sh is a symlink).
    InterpreterPath string

    // CommandName is the command name passed to env (e.g., "python3").
    // Empty for direct form.
    CommandName string

    // ResolvedPath is the resolved absolute path of CommandName via
    // exec.LookPath + filepath.EvalSymlinks.
    // Empty for direct form.
    ResolvedPath string
}
```

#### 1.2.2 `internal/shebang/errors.go`

```go
package shebang

import "errors"

var (
    // ErrShebangLineTooLong is returned when no newline is found within
    // maxShebangBytes (256) bytes.
    ErrShebangLineTooLong = errors.New("shebang line exceeds 256-byte limit")

    // ErrShebangCR is returned when the shebang line contains a carriage
    // return (\r) character.
    ErrShebangCR = errors.New("shebang line contains carriage return")

    // ErrEmptyInterpreterPath is returned when the shebang line has no
    // interpreter path (e.g., "#!\n").
    ErrEmptyInterpreterPath = errors.New("empty interpreter path in shebang")

    // ErrInterpreterNotAbsolute is returned when the interpreter path is
    // not an absolute path (e.g., "#!python3").
    ErrInterpreterNotAbsolute = errors.New("interpreter path is not absolute")

    // ErrMissingEnvCommand is returned when env is used without a command
    // name (e.g., "#!/usr/bin/env\n").
    ErrMissingEnvCommand = errors.New("missing command name after env")

    // ErrEnvFlagNotSupported is returned when the env command has flags
    // (e.g., "#!/usr/bin/env -S python3").
    ErrEnvFlagNotSupported = errors.New("env flags are not supported")

    // ErrEnvAssignmentNotSupported is returned when the env command has
    // environment variable assignments (e.g., "#!/usr/bin/env PYTHONPATH=. python3").
    ErrEnvAssignmentNotSupported = errors.New("env variable assignments are not supported")

    // ErrCommandNotFound is returned when the command name cannot be
    // resolved via PATH (e.g., "#!/usr/bin/env nonexistent_cmd").
    ErrCommandNotFound = errors.New("command not found in PATH")
)
```

#### 1.2.3 `internal/fileanalysis/schema.go` — 追加型

```go
// ShebangInterpreterInfo records the interpreter associated with a script file.
// For direct form (e.g., "#!/bin/sh"), only InterpreterPath is set.
// For env form (e.g., "#!/usr/bin/env python3"), all three fields are set.
type ShebangInterpreterInfo struct {
    // InterpreterPath is the shebang interpreter path, symlink-resolved.
    // For direct form: the interpreter itself (e.g., "/usr/bin/dash").
    // For env form: the env binary path (e.g., "/usr/bin/env").
    InterpreterPath string `json:"interpreter_path"`

    // CommandName is the command passed to env (e.g., "python3").
    // Empty for direct form.
    CommandName string `json:"command_name,omitempty"`

    // ResolvedPath is the PATH-resolved absolute path of CommandName,
    // symlink-resolved (e.g., "/usr/bin/python3.11").
    // Empty for direct form.
    ResolvedPath string `json:"resolved_path,omitempty"`
}
```

#### 1.2.4 `internal/fileanalysis/schema.go` — Record 変更

```go
type Record struct {
    SchemaVersion int       `json:"schema_version"`
    FilePath      string    `json:"file_path"`
    ContentHash   string    `json:"content_hash"`
    UpdatedAt     time.Time `json:"updated_at"`

    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
    DynLibDeps      []LibEntry           `json:"dyn_lib_deps,omitempty"`
    SymbolAnalysis  *SymbolAnalysisData  `json:"symbol_analysis,omitempty"`

    // ShebangInterpreter holds interpreter information parsed from the file's
    // shebang line. nil for non-script files (ELF binaries, text files, etc.).
    ShebangInterpreter *ShebangInterpreterInfo `json:"shebang_interpreter,omitempty"`
}
```

#### 1.2.5 スキーマバージョン更新

```go
// Version 11 adds ShebangInterpreter to Record for shebang interpreter tracking.
const CurrentSchemaVersion = 11
```

#### 1.2.6 `internal/verification/interfaces.go` — インターフェース変更

```go
type ManagerInterface interface {
    ResolvePath(path string) (string, error)
    VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error)
    VerifyCommandDynLibDeps(cmdPath string) error
    // VerifyCommandShebangInterpreter verifies the shebang interpreter
    // recorded in the script's Record. envVars is the final environment
    // that will be used to execute the command (needed for env form PATH
    // re-resolution).
    VerifyCommandShebangInterpreter(cmdPath string, envVars map[string]string) error
}
```

### 1.3 実装詳細

#### 1.3.1 `shebang.Parse` 関数

```go
// Parse reads the shebang line from the file at filePath and returns the
// parsed interpreter information. Returns (nil, nil) if the file does not
// start with "#!" (not a script).
//
// Errors:
//   - ErrShebangLineTooLong: no newline within 256 bytes
//   - ErrShebangCR: carriage return found in shebang line
//   - ErrEmptyInterpreterPath: no interpreter path after "#!"
//   - ErrInterpreterNotAbsolute: interpreter path is relative
//   - ErrMissingEnvCommand: env used without command name
//   - ErrEnvFlagNotSupported: env argument starts with "-"
//   - ErrEnvAssignmentNotSupported: env argument contains "="
//   - ErrCommandNotFound: command not found via exec.LookPath
func Parse(filePath string) (*ShebangInfo, error) {
    // 1. Read first maxShebangBytes bytes
    f, err := os.Open(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to open file: %w", err)
    }
    defer f.Close()

    buf := make([]byte, maxShebangBytes)
    n, _ := f.Read(buf)
    buf = buf[:n]

    // 2. Check for "#!" prefix
    if n < 2 || string(buf[:2]) != shebangPrefix {
        return nil, nil // Not a shebang script
    }

    // 3. Find newline
    line := buf[2:] // Skip "#!"
    nlIdx := -1
    for i, b := range line {
        if b == '\n' {
            nlIdx = i
            break
        }
    }
    if nlIdx == -1 {
        return nil, ErrShebangLineTooLong
    }
    line = line[:nlIdx]

    // 4. Check for \r
    if bytes.ContainsRune(line, '\r') {
        return nil, ErrShebangCR
    }

    // 5. Skip leading whitespace and tokenize
    content := strings.TrimLeft(string(line), " \t")
    tokens := strings.Fields(content)
    if len(tokens) == 0 {
        return nil, ErrEmptyInterpreterPath
    }

    interpreterPath := tokens[0]

    // 6. Validate absolute path
    if !filepath.IsAbs(interpreterPath) {
        return nil, fmt.Errorf("%w: %s", ErrInterpreterNotAbsolute, interpreterPath)
    }

    // 7. Resolve symlinks for interpreter path
    resolvedInterpreter, err := filepath.EvalSymlinks(interpreterPath)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve interpreter path %s: %w",
            interpreterPath, err)
    }

    // 8. Check if env form
    baseName := filepath.Base(resolvedInterpreter)
    if baseName == "env" {
        return parseEnvForm(resolvedInterpreter, tokens[1:])
    }

    // 9. Direct form
    return &ShebangInfo{
        InterpreterPath: resolvedInterpreter,
    }, nil
}
```

#### 1.3.2 `shebang.parseEnvForm` 関数

```go
// parseEnvForm handles "#!/usr/bin/env <cmd>" shebangs.
func parseEnvForm(envPath string, args []string) (*ShebangInfo, error) {
    if len(args) == 0 {
        return nil, ErrMissingEnvCommand
    }

    cmdArg := args[0]

    // Check for flags
    if strings.HasPrefix(cmdArg, "-") {
        return nil, fmt.Errorf("%w: %s", ErrEnvFlagNotSupported, cmdArg)
    }

    // Check for variable assignments
    if strings.Contains(cmdArg, "=") {
        return nil, fmt.Errorf("%w: %s", ErrEnvAssignmentNotSupported, cmdArg)
    }

    // Resolve command via PATH
    cmdPath, err := exec.LookPath(cmdArg)
    if err != nil {
        return nil, fmt.Errorf("%w: %s", ErrCommandNotFound, cmdArg)
    }

    // Make absolute if not already
    if !filepath.IsAbs(cmdPath) {
        abs, err := filepath.Abs(cmdPath)
        if err != nil {
            return nil, fmt.Errorf("failed to get absolute path for %s: %w", cmdPath, err)
        }
        cmdPath = abs
    }

    // Resolve symlinks for resolved command path
    resolvedCmd, err := filepath.EvalSymlinks(cmdPath)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve command path %s: %w", cmdPath, err)
    }

    return &ShebangInfo{
        InterpreterPath: envPath,
        CommandName:     cmdArg,
        ResolvedPath:    resolvedCmd,
    }, nil
}
```

#### 1.3.3 `shebang.IsShebangScript` 関数

```go
// IsShebangScript checks if the file at filePath starts with "#!" magic bytes.
// Returns false, nil for files that are too small.
// Returns an error when the file cannot be opened.
func IsShebangScript(filePath string) (bool, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return false, fmt.Errorf("failed to open file: %w", err)
    }
    defer f.Close()

    buf := make([]byte, 2)
    n, err := f.Read(buf)
    if err != nil || n < 2 {
        return false, nil
    }
    return string(buf) == shebangPrefix, nil
}
```

#### 1.3.4 `filevalidator.Validator.SaveRecord` — shebang 事前処理フェーズ

`SaveRecord` は `Store.Update` に入る前に shebang を解析し、インタープリタ Record の作成まで完了させる。これにより、インタープリタ記録失敗時にスクリプト Record だけが先に保存される部分更新を防ぐ。

```go
func (v *Validator) SaveRecord(filePath string, force bool) (string, string, error) {
    targetPath, err := validatePath(filePath)
    if err != nil {
        return "", "", err
    }

    hash, err := v.calculateHash(targetPath.String())
    if err != nil {
        return "", "", fmt.Errorf("failed to calculate hash: %w", err)
    }

    shebangInfo, err := v.resolveShebangInfo(targetPath.String())
    if err != nil {
        return "", "", err
    }

    if shebangInfo != nil {
        if err := v.recordInterpreter(shebangInfo.InterpreterPath); err != nil {
            return "", "", fmt.Errorf("failed to record interpreter %s: %w",
                shebangInfo.InterpreterPath, err)
        }
        if shebangInfo.ResolvedPath != "" {
            if err := v.recordInterpreter(shebangInfo.ResolvedPath); err != nil {
                return "", "", fmt.Errorf("failed to record resolved command %s: %w",
                    shebangInfo.ResolvedPath, err)
            }
        }
    }

    hashFilePath, err := v.GetHashFilePath(targetPath)
    if err != nil {
        return "", "", err
    }

    contentHash, err := v.updateAnalysisRecord(targetPath, hash, force, shebangInfo)
    if err != nil {
        return "", "", err
    }

    return hashFilePath, contentHash, nil
}

func (v *Validator) resolveShebangInfo(filePath string) (*shebang.ShebangInfo, error) {
    shebangInfo, err := shebang.Parse(filePath)
    if err != nil {
        return nil, fmt.Errorf("shebang analysis failed for %s: %w", filePath, err)
    }
    if shebangInfo == nil {
        return nil, nil
    }

    isShebang, err := shebang.IsShebangScript(shebangInfo.InterpreterPath)
    if err != nil {
        return nil, fmt.Errorf("failed to check interpreter %s: %w",
            shebangInfo.InterpreterPath, err)
    }
    if isShebang {
        return nil, fmt.Errorf("interpreter %s is itself a shebang script: %w",
            shebangInfo.InterpreterPath, shebang.ErrRecursiveShebang)
    }

    if shebangInfo.ResolvedPath != "" {
        isShebang, err = shebang.IsShebangScript(shebangInfo.ResolvedPath)
        if err != nil {
            return nil, fmt.Errorf("failed to check resolved command %s: %w",
                shebangInfo.ResolvedPath, err)
        }
        if isShebang {
            return nil, fmt.Errorf("resolved command %s is itself a shebang script: %w",
                shebangInfo.ResolvedPath, shebang.ErrRecursiveShebang)
        }
    }

    return shebangInfo, nil
}

// recordInterpreter creates an independent Record for an interpreter binary.
// Uses force=true to ensure the record is always updated.
func (v *Validator) recordInterpreter(interpreterPath string) error {
    _, _, err := v.SaveRecord(interpreterPath, true)
    return err
}
```

`updateAnalysisRecord` 側は、引数として渡された `shebangInfo` を `record.ShebangInterpreter` に反映するだけに留める。

```go
func (v *Validator) updateAnalysisRecord(
    filePath common.ResolvedPath,
    hash string,
    force bool,
    shebangInfo *shebang.ShebangInfo,
) (string, error) {
    // Existing hash / dynlib / symbol / syscall analysis logic.

    if shebangInfo != nil {
        record.ShebangInterpreter = &fileanalysis.ShebangInterpreterInfo{
            InterpreterPath: shebangInfo.InterpreterPath,
            CommandName:     shebangInfo.CommandName,
            ResolvedPath:    shebangInfo.ResolvedPath,
        }
    } else {
        record.ShebangInterpreter = nil
    }
}
```

**注意**: `recordInterpreter` は `SaveRecord` を再帰呼び出しするが、インタープリタは shebang スクリプトではないことを `resolveShebangInfo` 内で事前確認しているため、無限再帰は発生しない。

#### 1.3.5 `verification.Manager.VerifyCommandShebangInterpreter`

```go
// VerifyCommandShebangInterpreter verifies the shebang interpreter recorded
// in the script's analysis record. It performs:
//   1. LoadRecord for the command to get ShebangInterpreter info
//   2. For each interpreter binary: verify Record existence + hash
//   3. For env form: re-resolve command_name via PATH and compare
//
// envVars is the final environment that will be used to execute the command.
// It is needed for env form PATH re-resolution (FR-3.3.4).
//
// Returns nil if no ShebangInterpreter is recorded (non-script file).
func (m *Manager) VerifyCommandShebangInterpreter(
    cmdPath string, envVars map[string]string,
) error {
    if m.fileValidator == nil {
        return nil
    }

    // Load the script's record.
    record, err := m.fileValidator.LoadRecord(cmdPath)
    if err != nil {
        if errors.Is(err, fileanalysis.ErrRecordNotFound) {
            return nil // No record: skip
        }
        return fmt.Errorf("failed to load record for shebang verification: %w", err)
    }

    // No shebang interpreter: nothing to verify.
    if record.ShebangInterpreter == nil {
        return nil
    }

    si := record.ShebangInterpreter

    // Verify interpreter binary Record exists and hash matches.
    if err := m.verifyInterpreterHash(si.InterpreterPath); err != nil {
        return fmt.Errorf("interpreter verification failed for %s: %w",
            si.InterpreterPath, err)
    }

    // For env form: verify resolved command and PATH re-resolution.
    if si.CommandName != "" {
        // Verify resolved command's Record and hash.
        if err := m.verifyInterpreterHash(si.ResolvedPath); err != nil {
            return fmt.Errorf("resolved command verification failed for %s: %w",
                si.ResolvedPath, err)
        }

        // Re-resolve command_name using the final environment's PATH.
        if err := m.verifyEnvPathResolution(si.CommandName, si.ResolvedPath, envVars); err != nil {
            return err
        }
    }

    return nil
}

// verifyInterpreterHash verifies that the interpreter binary's Record exists
// and its hash matches the current file.
func (m *Manager) verifyInterpreterHash(interpreterPath string) error {
    // Verify hash using the standard verification flow.
    if err := m.fileValidator.Verify(interpreterPath); err != nil {
        if errors.Is(err, filevalidator.ErrHashFileNotFound) {
            return &ErrInterpreterRecordNotFound{Path: interpreterPath}
        }
        return err
    }
    return nil
}

// verifyEnvPathResolution re-resolves the command name using the execution
// environment's PATH and checks that it matches the recorded resolved_path.
func (m *Manager) verifyEnvPathResolution(
    commandName, recordedResolvedPath string, envVars map[string]string,
) error {
    pathEnv, ok := envVars["PATH"]
    if !ok {
        return fmt.Errorf("PATH not found in environment for env command resolution")
    }

    // Search for the command in the execution environment's PATH.
    resolvedPath, err := lookPathInEnv(commandName, pathEnv)
    if err != nil {
        return fmt.Errorf("failed to resolve %s in execution PATH: %w",
            commandName, err)
    }

    // Resolve symlinks for comparison.
    resolvedPath, err = filepath.EvalSymlinks(resolvedPath)
    if err != nil {
        return fmt.Errorf("failed to resolve symlinks for %s: %w",
            resolvedPath, err)
    }

    if resolvedPath != recordedResolvedPath {
        return &ErrInterpreterPathMismatch{
            CommandName:  commandName,
            RecordedPath: recordedResolvedPath,
            ActualPath:   resolvedPath,
        }
    }

    return nil
}

// lookPathInEnv resolves a command name using the given PATH value.
// This is used instead of exec.LookPath because we need to use the
// execution environment's PATH, not the current process's PATH.
func lookPathInEnv(name, pathEnv string) (string, error) {
    for _, dir := range filepath.SplitList(pathEnv) {
        if dir == "" {
            dir = "."
        }
        candidate := filepath.Join(dir, name)
        info, err := os.Stat(candidate)
        if err != nil {
            continue
        }
        // Check if executable (any execute bit set).
        if info.Mode()&0o111 != 0 {
            return candidate, nil
        }
    }
    return "", fmt.Errorf("command %s not found in PATH", name)
}
```

#### 1.3.6 `verification` パッケージ — エラー型

```go
// ErrInterpreterRecordNotFound indicates that the interpreter binary's
// analysis record does not exist.
type ErrInterpreterRecordNotFound struct {
    Path string
}

func (e *ErrInterpreterRecordNotFound) Error() string {
    return fmt.Sprintf("interpreter record not found: %s", e.Path)
}

// ErrInterpreterPathMismatch indicates that the env-form command resolved
// to a different path in the execution environment than what was recorded.
type ErrInterpreterPathMismatch struct {
    CommandName  string
    RecordedPath string
    ActualPath   string
}

func (e *ErrInterpreterPathMismatch) Error() string {
    return fmt.Sprintf(
        "interpreter path mismatch for %s: recorded %s, actual %s",
        e.CommandName, e.RecordedPath, e.ActualPath,
    )
}
```

#### 1.3.7 `group_executor.go` — インタープリタ検証の呼び出し

`verifyGroupFiles` 内の DynLibDeps 検証ループの後に追加:

```go
// Verify shebang interpreter integrity for each command.
for _, cmd := range runtimeGroup.Commands {
    resolvedPath, resolveErr := ge.verificationManager.ResolvePath(cmd.ExpandedCmd)
    if resolveErr != nil {
        slog.Warn("Path resolution failed during shebang interpreter verification",
            "group", runnertypes.ExtractGroupName(runtimeGroup),
            "command", cmd.ExpandedCmd,
            "error", resolveErr)
        continue
    }
    if siErr := ge.verificationManager.VerifyCommandShebangInterpreter(
        resolvedPath, cmd.ExpandedEnv,
    ); siErr != nil {
        slog.Error("Shebang interpreter verification failed",
            "group", runnertypes.ExtractGroupName(runtimeGroup),
            "command", resolvedPath,
            "error", siErr)
        return siErr
    }
}
```

### 1.4 エラー追加

#### 1.4.1 `internal/shebang/errors.go` — 追加エラー

```go
// ErrRecursiveShebang is returned when an interpreter is itself a shebang script.
var ErrRecursiveShebang = errors.New("interpreter is a shebang script")
```

### 1.5 JSON 出力例

#### 1.5.1 直接形式（`#!/bin/sh`）のスクリプト Record

```json
{
  "schema_version": 11,
  "file_path": "/home/user/scripts/deploy.sh",
  "content_hash": "sha256:abc123...",
  "updated_at": "2026-03-27T12:00:00Z",
  "shebang_interpreter": {
    "interpreter_path": "/usr/bin/dash"
  }
}
```

注: `/bin/sh` → `/usr/bin/dash`（シンボリックリンク解決後）

#### 1.5.2 env 形式（`#!/usr/bin/env python3`）のスクリプト Record

```json
{
  "schema_version": 11,
  "file_path": "/home/user/scripts/process.py",
  "content_hash": "sha256:def456...",
  "updated_at": "2026-03-27T12:00:00Z",
  "shebang_interpreter": {
    "interpreter_path": "/usr/bin/env",
    "command_name": "python3",
    "resolved_path": "/usr/bin/python3.11"
  }
}
```

#### 1.5.3 ELF バイナリの Record（変更なし）

```json
{
  "schema_version": 11,
  "file_path": "/usr/bin/ls",
  "content_hash": "sha256:789abc...",
  "updated_at": "2026-03-27T12:00:00Z",
  "dyn_lib_deps": [
    {"soname": "libc.so.6", "path": "/lib/aarch64-linux-gnu/libc.so.6", "hash": "sha256:..."}
  ]
}
```

`shebang_interpreter` フィールドは `omitempty` で省略される。

#### 1.5.4 インタープリタの独立 Record（`/usr/bin/dash` の例）

```json
{
  "schema_version": 11,
  "file_path": "/usr/bin/dash",
  "content_hash": "sha256:fedcba...",
  "updated_at": "2026-03-27T12:00:00Z",
  "dyn_lib_deps": [
    {"soname": "libc.so.6", "path": "/lib/aarch64-linux-gnu/libc.so.6", "hash": "sha256:..."}
  ],
  "syscall_analysis": { "..." : "..." }
}
```

インタープリタ自体は ELF バイナリなので通常の record と同じ内容が含まれる。`shebang_interpreter` は持たない。

---

## 2. テスト仕様

### 2.1 `shebang.Parse` 単体テスト

| テストケース | 入力 | 期待結果 | 受け入れ基準 |
|---|---|---|---|
| 直接形式 `#!/bin/sh` | `#!/bin/sh\necho hello` | `InterpreterPath` = `/bin/sh` の EvalSymlinks 結果 | AC-1 |
| 引数付き `#!/bin/bash -e` | `#!/bin/bash -e\nset -x` | `InterpreterPath` = `/bin/bash` の EvalSymlinks 結果 | AC-2 |
| env 形式 `#!/usr/bin/env python3` | `#!/usr/bin/env python3\nimport sys` | `InterpreterPath` = `/usr/bin/env` の EvalSymlinks 結果, `CommandName` = `python3`, `ResolvedPath` = LookPath 結果 | AC-3 |
| `#!` 直後にスペース | `#! /bin/sh\n` | `InterpreterPath` = `/bin/sh` の EvalSymlinks 結果 | AC-21 |
| 非 shebang ファイル | `ELF...` (ELF magic) | `nil, nil` | AC-10, AC-11 |
| 空インタープリタ | `#!\n` | `ErrEmptyInterpreterPath` | AC-22 |
| 空白のみ | `#!  \n` | `ErrEmptyInterpreterPath` | AC-22 |
| 非絶対パス | `#!python3\n` | `ErrInterpreterNotAbsolute` | AC-14 |
| env コマンドなし | `#!/usr/bin/env\n` | `ErrMissingEnvCommand` | AC-15 |
| env フラグ | `#!/usr/bin/env -S python3\n` | `ErrEnvFlagNotSupported` | AC-12 |
| env 変数代入 | `#!/usr/bin/env PYTHONPATH=. python3\n` | `ErrEnvAssignmentNotSupported` | AC-24 |
| PATH 解決不可 | `#!/usr/bin/env nonexistent_cmd\n` | `ErrCommandNotFound` | AC-16 |
| 256 バイト超 | `#!/bin/sh` + 250 文字 + no newline | `ErrShebangLineTooLong` | AC-17 |
| CR 含む | `#!/bin/sh\r\n` | `ErrShebangCR` | 要件 FR-3.1.4 |

### 2.2 `shebang.IsShebangScript` 単体テスト

| テストケース | 入力 | 期待結果 |
|---|---|---|
| shebang ファイル | `#!/bin/sh\n` | `true, nil` |
| ELF ファイル | `\x7fELF...` | `false, nil` |
| テキストファイル | `hello world\n` | `false, nil` |
| 空ファイル | `` | `false, nil` |
| 1 バイトファイル | `#` | `false, nil` |

### 2.3 `filevalidator` コンポーネントテスト

| テストケース | 概要 | 受け入れ基準 |
|---|---|---|
| スクリプト record で ShebangInterpreter 設定 | `#!/bin/sh` スクリプトの record 後、Record に `shebang_interpreter` あり | AC-4, AC-6 |
| env 形式で 3 フィールド設定 | `#!/usr/bin/env python3` スクリプトの record 後、3 フィールドすべて設定 | AC-5 |
| インタープリタ独立 Record 作成 | `#!/bin/sh` スクリプトの record 後、`/bin/sh`（解決先）の Record も存在 | AC-1 |
| env 形式で 2 つの独立 Record | `#!/usr/bin/env python3` の record 後、env と python3 の Record 存在 | AC-3 |
| 再帰 shebang 検出 | インタープリタが shebang スクリプトの場合にエラー | AC-13 |
| ELF バイナリは shebang 解析なし | ELF ファイルの record で ShebangInterpreter = nil | AC-10 |
| テキストファイルは shebang 解析なし | shebang なしのテキストファイルの record で ShebangInterpreter = nil | AC-11 |
| シンボリックリンク解決 | `/bin/sh` → `/usr/bin/dash` の場合 `interpreter_path` が後者 | AC-23 |

### 2.4 `verification.Manager` コンポーネントテスト

| テストケース | 概要 | 受け入れ基準 |
|---|---|---|
| インタープリタ Record 不在 | Record は存在するが interpreter_path の独立 Record がない | AC-7 |
| インタープリタハッシュ不一致 | interpreter_path のハッシュが変化 | AC-8 |
| env パス再解決不一致 | PATH 操作で python3 が別パスに解決 | AC-9 |
| 正常系: 直接形式 | すべてのチェックをパス | — |
| 正常系: env 形式 | すべてのチェックをパス | — |
| ShebangInterpreter nil | ELF バイナリなど非スクリプト → スキップ | AC-10 |
| スキーマ v10 Record | v10 の Record が SchemaVersionMismatchError | AC-18 |
| 最終環境の PATH で再解決 | 設定適用後の環境で PATH 解決 | AC-19 |
| command_name で再解決 | shebang 再解析ではなく Record の command_name を使用 | AC-20 |

### 2.5 受け入れ基準とテストの対応表

| AC ID | 基準 | テスト箇所 |
|---|---|---|
| AC-1 | `#!/bin/sh` → `/bin/sh` の独立 Record | `filevalidator` テスト |
| AC-2 | `#!/bin/bash -e` → `/bin/bash` の独立 Record | `shebang.Parse` テスト + `filevalidator` テスト |
| AC-3 | `#!/usr/bin/env python3` → env + python3 の独立 Record | `filevalidator` テスト |
| AC-4 | `shebang_interpreter` フィールド保存 | `filevalidator` テスト |
| AC-5 | env 形式の 3 フィールド | `shebang.Parse` テスト + `filevalidator` テスト |
| AC-6 | 直接形式で `command_name` / `resolved_path` 省略 | `shebang.Parse` テスト + JSON 出力テスト |
| AC-7 | インタープリタ未 record で runner エラー | `verification.Manager` テスト |
| AC-8 | インタープリタハッシュ変化で検証エラー | `verification.Manager` テスト |
| AC-9 | env 形式で PATH 変化時エラー | `verification.Manager` テスト |
| AC-10 | ELF record で動作変わらず | `shebang.Parse` テスト（nil 返却）+ `filevalidator` テスト |
| AC-11 | テキスト record で動作変わらず | `shebang.Parse` テスト（nil 返却） |
| AC-12 | `env -S` でエラー | `shebang.Parse` テスト |
| AC-13 | 再帰 shebang でエラー | `filevalidator` テスト |
| AC-14 | 非絶対パスでエラー | `shebang.Parse` テスト |
| AC-15 | env のみでエラー | `shebang.Parse` テスト |
| AC-16 | 解決不可コマンドでエラー | `shebang.Parse` テスト |
| AC-17 | 256 バイト超でエラー | `shebang.Parse` テスト |
| AC-18 | v10 Record で SchemaVersionMismatchError | `verification.Manager` テスト |
| AC-19 | 最終環境の PATH で再解決 | `verification.Manager` テスト |
| AC-20 | command_name で再解決 | `verification.Manager` テスト |
| AC-21 | `#!` 直後スペース許容 | `shebang.Parse` テスト |
| AC-22 | 空インタープリタでエラー | `shebang.Parse` テスト |
| AC-23 | シンボリックリンク解決 | `shebang.Parse` テスト + `filevalidator` テスト |
| AC-24 | env 変数代入でエラー | `shebang.Parse` テスト |

---

## 3. 実装チェックリスト

### Phase 1: shebang パーサー

- [ ] `internal/shebang/errors.go` — エラー型定義
- [ ] `internal/shebang/parser.go` — `Parse`, `IsShebangScript`, `parseEnvForm`
- [ ] `internal/shebang/parser_test.go` — 単体テスト（全受け入れ基準カバー）

### Phase 2: スキーマ変更

- [ ] `internal/fileanalysis/schema.go` — `ShebangInterpreterInfo` 型追加
- [ ] `internal/fileanalysis/schema.go` — `Record` に `ShebangInterpreter` フィールド追加
- [ ] `internal/fileanalysis/schema.go` — `CurrentSchemaVersion` 10 → 11

### Phase 3: record 時ロジック

- [ ] `internal/filevalidator/validator.go` — `updateAnalysisRecord` に shebang 解析フェーズ追加
- [ ] `internal/filevalidator/validator.go` — `SaveRecord` にインタープリタ独立 Record 作成追加
- [ ] `internal/filevalidator/validator.go` — `recordInterpreter` ヘルパー
- [ ] `internal/filevalidator/validator_test.go` — コンポーネントテスト

### Phase 4: runner 時ロジック

- [ ] `internal/verification/manager.go` — `VerifyCommandShebangInterpreter` 実装
- [ ] `internal/verification/manager.go` — `verifyInterpreterHash`, `verifyEnvPathResolution`, `lookPathInEnv` 実装
- [ ] `internal/verification/manager.go` — エラー型 `ErrInterpreterRecordNotFound`, `ErrInterpreterPathMismatch`
- [ ] `internal/verification/interfaces.go` — `ManagerInterface` 更新
- [ ] `internal/verification/testing/testify_mocks.go` — モック更新
- [ ] `internal/verification/manager_test.go` — コンポーネントテスト

### Phase 5: group executor 統合

- [ ] `internal/runner/group_executor.go` — インタープリタ検証ループ追加
- [ ] `internal/runner/group_executor_test.go` — 統合テスト

### Phase 6: 最終検証

- [ ] `make test` — 全テスト GREEN
- [ ] `make fmt` — フォーマット差分なし
- [ ] `make lint` — lint エラーなし
