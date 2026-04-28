# 詳細仕様書: セキュリティパッケージの再構成

## 1. 前提・スコープ

本文書は `docs/tasks/0113_security_package_restructure/02_architecture.md` の
実装詳細を定義する。対象は `internal/security/` パッケージの新設と
`internal/runner/security` の縮小・再配置。

---

## 2. Phase 1: `internal/security/binaryanalyzer/` の作成

### 2.1 対象ファイル

`internal/runner/security/binaryanalyzer/` の全ファイルを
`internal/security/binaryanalyzer/` へコピーし、
利用側（呼び出し元）のインポートパスを更新する。
このパッケージ自体は runner 依存を持たないため、
コピー先パッケージ内部のコード変更は不要。

**コピー対象:**

- `analyzer.go`
- `doc.go`
- `known_network_libs.go`
- `known_network_libs_test.go`
- `network_symbols.go`
- `network_symbols_test.go`

### 2.2 変更内容

`package binaryanalyzer` の宣言はそのまま維持する。
このパッケージは外部パッケージ（`internal/runner/` など）をインポートしないため、
コード変更は不要。

---

## 3. Phase 2: `internal/security/elfanalyzer/` の作成（コア層）

### 3.1 対象ファイル

`internal/runner/security/elfanalyzer/` のうち、
`internal/runner/` への依存を持たないファイルを `internal/security/elfanalyzer/` へコピーする。

**コピー対象（runner 依存なし）:**

| ファイル | インポート変更 |
|---|---|
| `syscall_analyzer.go` | なし（`internal/common` のみ） |
| `syscall_decoder.go` | なし |
| `syscall_numbers.go` | なし |
| `arm64_decoder.go` | なし |
| `arm64_go_wrapper_resolver.go` | なし |
| `arm64_syscall_numbers.go` | なし |
| `x86_decoder.go` | なし |
| `x86_go_wrapper_resolver.go` | なし |
| `x86_syscall_numbers.go` | なし |
| `go_wrapper_resolver.go` | なし |
| `pclntab_parser.go` | なし（`internal/common`, `safefileio`） |
| `plt_analyzer.go` | なし |
| `mprotect_risk.go` | なし |
| `errors.go` | なし |
| `doc.go` | なし |
| `testing/` ディレクトリ | インポートパス更新 |
| `testdata/` ディレクトリ | コピーのみ |

**コピー対象（テストファイル）:**

| ファイル | インポート変更 |
|---|---|
| `syscall_analyzer_test.go` | インポートパス更新 |
| `syscall_analyzer_integration_test.go` | `binaryanalyzer` インポートパス更新 |
| `arm64_decoder_test.go` | なし |
| `arm64_go_wrapper_resolver_test.go` | なし |
| `arm64_syscall_numbers_test.go` | なし |
| `go_wrapper_resolver_test.go` | なし |
| `pclntab_parser_test.go` 等 | インポートパス更新 |
| `mprotect_risk_test.go` | なし |
| `plt_analyzer_test.go` | なし |

**`internal/runner/security/elfanalyzer/` に残留するファイル:**

| ファイル | 理由 |
|---|---|
| `standard_analyzer.go` | `internal/runner/runnertypes` 依存 |
| `analyzer.go` | `StandardELFAnalyzer` のコンパイル時チェック |
| `analyzer_test.go` | `StandardELFAnalyzer` のテスト |
| `standard_analyzer_fallback_test.go` | `StandardELFAnalyzer` のテスト |
| `analyzer_benchmark_test.go` | ベンチマーク（`StandardELFAnalyzer` 使用） |

### 3.2 `syscall_analyzer_integration_test.go` のインポート更新

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
```

### 3.3 `testing/` サブパッケージの更新

`testing/` 内のファイルのパッケージ宣言と自己参照するインポートパスを更新する。

---

## 4. Phase 3: `internal/security/machoanalyzer/` の作成

### 4.1 対象ファイル

`internal/runner/security/machoanalyzer/` の全ファイルをコピーし、
`binaryanalyzer` のインポートパスを更新する。

**インポート変更:**

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
```

**対象ファイル:**

- `analyzer.go`
- `pass1_scanner.go`
- `pass2_scanner.go`
- `pclntab_macho.go`
- `standard_analyzer.go`
- `svc_scanner.go`
- `symbol_normalizer.go`
- `testdata/`
- テストファイル全般

---

## 5. Phase 4: `internal/security/` 基盤ファイルの作成

### 5.1 `internal/security/errors.go`

TOCTOU チェックおよびディレクトリ権限チェックで使用するエラー変数。

```go
package security

import "errors"

var (
    // ErrInvalidDirPermissions is returned when a directory has inappropriate permissions.
    ErrInvalidDirPermissions = errors.New("invalid directory permissions")

    // ErrInsecurePathComponent is returned for insecure path component issues.
    ErrInsecurePathComponent = errors.New("insecure path component")

    // ErrInvalidPath is returned for path structural issues.
    ErrInvalidPath = errors.New("invalid path")
)
```

### 5.2 `internal/security/dir_permissions.go`

ディレクトリ権限チェックの独立実装。`internal/runner/security/file_validation.go` の
`ValidateDirectoryPermissions` および関連ロジックをこのファイルに抽出する。

```go
package security

// DirectoryPermChecker validates directory permissions for TOCTOU safety.
// internal/runner/security.Validator satisfies this interface.
type DirectoryPermChecker interface {
    ValidateDirectoryPermissions(path string) error
}

// dirPermChecker is a standalone implementation using the real OS file system.
type dirPermChecker struct {
    gm *groupmembership.GroupMembership
}

// NewDirectoryPermChecker creates a standalone DirectoryPermChecker
// using real OS file system and group membership checking.
// Used by cmd/record and cmd/verify without depending on internal/runner/security.
func NewDirectoryPermChecker() (DirectoryPermChecker, error) {
    return &dirPermChecker{gm: groupmembership.New()}, nil
}

func (d *dirPermChecker) ValidateDirectoryPermissions(path string) error {
    // Full implementation equivalent to runner/security.Validator.ValidateDirectoryPermissions
    // using os.Lstat directly (no injectable filesystem abstraction needed here).
    ...
}
```

**実装詳細:**

`dirPermChecker.ValidateDirectoryPermissions` の実装は
`internal/runner/security/file_validation.go` の `ValidateDirectoryPermissions` を
ベースとし、`v.fs.Lstat` を `os.Lstat` に置き換え、
`v.config.MaxPathLength` を `DefaultMaxPathLength` 定数で代替する。

関連するヘルパーメソッドも `dir_permissions.go` に移植する:
- `validateCompletePath()`
- `validateDirectoryComponentMode()`
- `validateDirectoryComponentPermissions()`
- `isStickyDirectory()`

定数:
```go
const (
    DefaultMaxPathLength        = 4096
    DefaultDirectoryPermissions = 0o755
    UIDRoot                     = 0
    GIDRoot                     = 0
)
```

### 5.3 `internal/security/toctou.go`

`internal/runner/security/toctou_check.go` から以下を移植する。

```go
package security

import (
    "errors"
    "io/fs"
    "log/slog"
    "path/filepath"
)

// TOCTOUViolation contains information about a TOCTOU permission check violation.
type TOCTOUViolation struct {
    Path string
    Err  error
}

// ResolveAbsPathForTOCTOU normalizes an absolute path for TOCTOU directory collection.
func ResolveAbsPathForTOCTOU(p string) (string, bool) { ... }

// CollectTOCTOUCheckDirs collects directories to check for TOCTOU prevention.
func CollectTOCTOUCheckDirs(verifyFilePaths []string, commandPaths []string, hashDir string) []string { ... }

// RunTOCTOUPermissionCheck checks all collected directories for TOCTOU-exploitable
// permission issues. Takes DirectoryPermChecker interface instead of *Validator.
func RunTOCTOUPermissionCheck(checker DirectoryPermChecker, dirs []string, logger *slog.Logger) []TOCTOUViolation { ... }
```

**変更点:** `RunTOCTOUPermissionCheck` の第 1 引数を `*Validator` から
`DirectoryPermChecker` インタフェースに変更する。

### 5.4 `internal/runner/security/binary_analyzer.go`（残留）

`NewBinaryAnalyzer()` は `NewStandardELFAnalyzer()`（`runnertypes.PrivilegeManager` 依存）を
利用するため、`internal/runner/security/binary_analyzer.go` に残留させる。

実施する変更は「配置変更」ではなく「依存先更新」のみとする。

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"

// After
"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
"github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
// elfanalyzer は internal/runner/security/elfanalyzer（StandardELFAnalyzer 側）を継続利用
```

**確定方針:**

- `cmd/record` は `internal/runner/security` を `NewBinaryAnalyzer()` 用にのみ継続利用する
- `cmd/record` の `NewSyscallAnalyzer()` は `internal/security/elfanalyzer` へ移行する
- `cmd/record` の TOCTOU 関連は `internal/security` 経由へ移行する

---

## 6. Phase 5: 既存コードのインポートパス更新

### 6.1 `internal/filevalidator/validator.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
"github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
```

`internal/filevalidator/validator_test.go` および `validator_macho_test.go`、
`validator_sort_test.go` のインポートも同様に更新する。

### 6.2 `internal/libccache/adapters.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
```

`adapters_test.go` のインポートも同様に更新する。

### 6.3 `internal/runner/security/syscall_store_adapter.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
```

`elfanalyzer.SyscallAnalysisResult` と `elfanalyzer.SyscallAnalysisStore` は
`internal/security/elfanalyzer/` に移動するため。

### 6.4 `internal/runner/security/elfanalyzer/standard_analyzer.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
```

`SyscallAnalysisResult` や `SyscallAnalysisStore` も `internal/security/elfanalyzer/` から
インポートするよう変更する:

```go
// Add
import secelfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"

// SyscallAnalysisResult and SyscallAnalysisStore come from internal/security/elfanalyzer
```

### 6.5 `internal/runner/security/binary_analyzer.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/machoanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
"github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
// elfanalyzer インポートはそのまま（StandardELFAnalyzer は runner/security/elfanalyzer に残留）
```

### 6.6 `internal/runner/security/toctou_check.go`

`NewValidatorForTOCTOU()` のみ残留し、他の関数（`CollectTOCTOUCheckDirs` 等）は削除する。
`cmd/runner/main.go` が `CollectTOCTOUCheckDirs` 等を呼ぶ箇所は
`internal/security` 経由に変更する。

### 6.7 `cmd/verify/main.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
// After
"github.com/isseis/go-safe-cmd-runner/internal/security"
```

呼び出し箇所:
- `security.NewValidatorForTOCTOU()` → `security.NewDirectoryPermChecker()`
- `security.CollectTOCTOUCheckDirs()` → `security.CollectTOCTOUCheckDirs()`（同名、新パッケージ）
- `security.RunTOCTOUPermissionCheck()` → `security.RunTOCTOUPermissionCheck()`（同名、新パッケージ）

### 6.8 `cmd/record/main.go`

```go
// Before
"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
// After
"github.com/isseis/go-safe-cmd-runner/internal/runner/security"   // NewBinaryAnalyzer() のみ残留
"github.com/isseis/go-safe-cmd-runner/internal/security"
"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
```

呼び出し変更:
- `security.NewValidatorForTOCTOU()` → `isec.NewDirectoryPermChecker()`
- `security.CollectTOCTOUCheckDirs()` → `isec.CollectTOCTOUCheckDirs()`
- `security.RunTOCTOUPermissionCheck()` → `isec.RunTOCTOUPermissionCheck()`
- `elfanalyzer.NewSyscallAnalyzer()` の参照先を
  `internal/runner/security/elfanalyzer` から `internal/security/elfanalyzer` に変更

> **本タスクのスコープ:**
> `cmd/verify` については `internal/runner/security` の依存を完全に解消する。
> `cmd/record` については TOCTOU 関連の `internal/runner/security` 依存を解消し、
> `NewSyscallAnalyzer()` の依存先を `internal/security/elfanalyzer` へ移行する。
> 残留する `internal/runner/security` 依存は `NewBinaryAnalyzer()` のみとする。

### 6.9 `cmd/runner/main.go`

`CollectTOCTOUCheckDirs`・`RunTOCTOUPermissionCheck`・`ResolveAbsPathForTOCTOU` を
`internal/security` 経由に変更する。
`NewValidatorForTOCTOU()` と `*security.Validator` は `internal/runner/security` から引き続き使用する。

```go
// Add
import isec "github.com/isseis/go-safe-cmd-runner/internal/security"

// Change
isec.CollectTOCTOUCheckDirs(...)
isec.RunTOCTOUPermissionCheck(secValidator, ...)  // *Validator satisfies DirectoryPermChecker
isec.ResolveAbsPathForTOCTOU(...)
```

---

## 7. Phase 6: 旧ファイルの削除

以下のファイル・ディレクトリを削除する。

### 7.1 削除するサブパッケージ

- `internal/runner/security/binaryanalyzer/` — `internal/security/binaryanalyzer/` に移動済み
- `internal/runner/security/machoanalyzer/` — `internal/security/machoanalyzer/` に移動済み

### 7.2 `internal/runner/security/elfanalyzer/` から削除するファイル

移動対象ファイル（Phase 3 でコピー済み）を削除する。
残留するファイル: `standard_analyzer.go`・`analyzer.go`・各テストファイル。

### 7.3 `internal/runner/security/toctou_check.go` の整理

`NewValidatorForTOCTOU()` のみ残し、移動済みの関数・型を削除する。

---

## 8. Phase 7: `internal/runner/security` の `ValidateDirectoryPermissions` 更新

`Validator.ValidateDirectoryPermissions()` は `internal/security.DirectoryPermChecker` を
自動的に満たす（メソッドシグネチャ変更なし）。`Validator` 自身の実装は変更しない。

`internal/security.RunTOCTOUPermissionCheck()` に `*Validator` を渡す場合、
Go の構造的型付けにより自動的に `DirectoryPermChecker` として機能する。

---

## 9. 受け入れ基準の検証計画

### AC-1: `internal/security` が `internal/runner/` をインポートしない

**検証方法:**
```sh
go list -deps github.com/isseis/go-safe-cmd-runner/internal/security/... \
  | grep internal/runner
# 期待結果: 出力なし
```

ただし `binary_analyzer.go` は `internal/runner/security/elfanalyzer` を参照するため
`internal/security/` には置かない（Phase 4 で確認）。

### AC-2: `cmd/verify` が `internal/runner/security` をインポートしない

**検証方法:**
```sh
go list -deps github.com/isseis/go-safe-cmd-runner/cmd/verify \
  | grep internal/runner/security
# 期待結果: 出力なし
```

この確認は `cmd/verify` 向けの受け入れ基準であり、`cmd/record` には適用しない。
`cmd/record` は本タスクでは既知の例外として
`NewBinaryAnalyzer()` 用に `internal/runner/security` 依存を保持する。

### AC-3: `internal/filevalidator` が `internal/runner/security` をインポートしない

**検証方法:**
```sh
go list -deps github.com/isseis/go-safe-cmd-runner/internal/filevalidator \
  | grep internal/runner/security
# 期待結果: 出力なし
```

### AC-4: `internal/libccache` が `internal/runner/security` をインポートしない

**検証方法:**
```sh
go list -deps github.com/isseis/go-safe-cmd-runner/internal/libccache \
  | grep internal/runner/security
# 期待結果: 出力なし
```

### AC-5: 全テスト合格

```sh
make test
```

### AC-6: lint 合格

```sh
make lint
```

---

## 10. 既知の制限事項と将来タスク

### 10.1 `cmd/record` の部分的な残留依存

`cmd/record` は `NewBinaryAnalyzer()` のために
`internal/runner/security` を引き続きインポートする。
`NewSyscallAnalyzer()` は `internal/security/elfanalyzer` へ移行し、
`internal/runner/security/elfanalyzer` 依存は解消する。

**完全解消の条件:**
`runnertypes.PrivilegeManager` インタフェースを `internal/common` または
`internal/privilege` に移動し、`elfanalyzer/standard_analyzer.go` と
`filevalidator/privileged_file.go` を更新する別タスクが必要。

### 10.2 `internal/runner/security/elfanalyzer/` の残留

`standard_analyzer.go`（と `analyzer.go`）は runner 固有のため残留する。
将来、`PrivilegeManager` の移動後に `elfanalyzer/` 全体を `internal/security/elfanalyzer/` へ
統合できる。
