# 詳細仕様書: StandardELFAnalyzer の internal/security/elfanalyzer への移動

## 1. ファイル変更一覧

| 操作 | ファイルパス |
|------|-------------|
| 新規作成 | `internal/security/elfanalyzer/privileged_opener.go` |
| 移動・変更 | `internal/runner/security/elfanalyzer/standard_analyzer.go` → `internal/security/elfanalyzer/standard_analyzer.go` |
| 新規作成 | `internal/runner/security/elfanalyzer/privileged_opener_impl.go` |
| 変更 | `internal/runner/security/binary_analyzer.go` |
| 変更 | `internal/runner/security/syscall_store_adapter.go` |
| 削除 | `internal/runner/security/elfanalyzer/analyzer.go` |
| 削除 | `internal/runner/security/elfanalyzer/standard_analyzer.go` |
| 移動 | `internal/runner/security/elfanalyzer/analyzer_test.go` → `internal/security/elfanalyzer/` |
| 移動 | `internal/runner/security/elfanalyzer/analyzer_benchmark_test.go` → `internal/security/elfanalyzer/` |
| 移動 | `internal/runner/security/elfanalyzer/standard_analyzer_fallback_test.go` → `internal/security/elfanalyzer/` |
| 削除 | `internal/runner/security/elfanalyzer/testdata/`（`internal/security/elfanalyzer/testdata/` と重複） |

## 2. 新規ファイル

### 2.1 `internal/security/elfanalyzer/privileged_opener.go`

```go
package elfanalyzer

import "github.com/isseis/go-safe-cmd-runner/internal/safefileio"

// PrivilegedFileOpener はアクセス権不足時に特権昇格でファイルを開く手段を提供する。
// nil の場合は特権昇格を行わず、os.ErrPermission をそのまま返す。
type PrivilegedFileOpener interface {
	OpenWithPrivileges(path string) (safefileio.File, error)
}
```

### 2.2 `internal/runner/security/elfanalyzer/privileged_opener_impl.go`

```go
package elfanalyzer

import (
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	secelfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

type privilegedFileOpenerImpl struct {
	pfv         *filevalidator.PrivilegedFileValidator
	privManager runnertypes.PrivilegeManager
}

// NewPrivilegedFileOpener は secelfanalyzer.PrivilegedFileOpener の runner 実装を生成する。
// privManager が nil の場合は特権昇格なし（os.ErrPermission をそのまま返す）。
func NewPrivilegedFileOpener(
	fs safefileio.FileSystem,
	privManager runnertypes.PrivilegeManager,
) secelfanalyzer.PrivilegedFileOpener {
	return &privilegedFileOpenerImpl{
		pfv:         filevalidator.NewPrivilegedFileValidator(fs),
		privManager: privManager,
	}
}

// OpenWithPrivileges implements secelfanalyzer.PrivilegedFileOpener.
func (o *privilegedFileOpenerImpl) OpenWithPrivileges(path string) (safefileio.File, error) {
	return o.pfv.OpenFileWithPrivileges(path, o.privManager)
}
```

## 3. 既存ファイルの変更

### 3.1 `internal/security/elfanalyzer/standard_analyzer.go`（新規作成・移動）

`internal/runner/security/elfanalyzer/standard_analyzer.go` の内容を以下の差分でこのパスに移動する。

#### 変更点一覧

| 変更箇所 | 変更前 | 変更後 |
|---------|--------|--------|
| パッケージ宣言 | `package elfanalyzer` | `package elfanalyzer`（同じ） |
| フィールド `privManager` | `privManager runnertypes.PrivilegeManager` | 削除 |
| フィールド `pfv` | `pfv *filevalidator.PrivilegedFileValidator` | 削除 |
| フィールド（新規） | なし | `opener PrivilegedFileOpener` |
| コンストラクタ引数 | `privManager runnertypes.PrivilegeManager` | `opener PrivilegedFileOpener` |
| コンストラクタ本体 | `privManager: privManager, pfv: filevalidator.NewPrivilegedFileValidator(fs)` | `opener: opener` |
| 特権オープン条件 | `a.privManager != nil` | `a.opener != nil` |
| 特権オープン呼び出し | `a.pfv.OpenFileWithPrivileges(path, a.privManager)` | `a.opener.OpenWithPrivileges(path)` |
| 型エイリアス | `type SyscallAnalysisStore = secelfanalyzer.SyscallAnalysisStore` | 削除（自パッケージで直接定義済み） |
| import `runnertypes` | あり | 削除 |
| import `filevalidator` | あり | 削除 |
| import `secelfanalyzer` | あり（aliasとして） | 不要（自パッケージのため削除） |

#### 構造体定義（移動後）

```go
type StandardELFAnalyzer struct {
	fs             safefileio.FileSystem
	networkSymbols map[string]binaryanalyzer.SymbolCategory
	opener         PrivilegedFileOpener  // optional, for execute-only binaries

	// syscallStore は静的バイナリ解析のためのオプションの syscall 解析ストア。
	syscallStore SyscallAnalysisStore
}
```

#### コンストラクタ（移動後）

```go
func NewStandardELFAnalyzer(fs safefileio.FileSystem, opener PrivilegedFileOpener) *StandardELFAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardELFAnalyzer{
		fs:             fs,
		networkSymbols: binaryanalyzer.GetNetworkSymbols(),
		opener:         opener,
	}
}

func NewStandardELFAnalyzerWithSymbols(fs safefileio.FileSystem, opener PrivilegedFileOpener, symbols map[string]binaryanalyzer.SymbolCategory) *StandardELFAnalyzer {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &StandardELFAnalyzer{
		fs:             fs,
		networkSymbols: symbols,
		opener:         opener,
	}
}

func NewStandardELFAnalyzerWithSyscallStore(
	fs safefileio.FileSystem,
	opener PrivilegedFileOpener,
	store SyscallAnalysisStore,
) *StandardELFAnalyzer {
	analyzer := NewStandardELFAnalyzer(fs, opener)
	if store != nil {
		analyzer.syscallStore = store
	}
	return analyzer
}
```

#### `AnalyzeNetworkSymbols` の特権オープン箇所（移動後）

```go
// 変更前
if errors.Is(err, os.ErrPermission) && a.privManager != nil {
    file, err = a.pfv.OpenFileWithPrivileges(path, a.privManager)
    ...
}

// 変更後
if errors.Is(err, os.ErrPermission) && a.opener != nil {
    file, err = a.opener.OpenWithPrivileges(path)
    ...
}
```

#### コンパイル時インターフェースチェックの移動

`internal/runner/security/elfanalyzer/analyzer.go` にあった以下の宣言を
`internal/security/elfanalyzer/standard_analyzer.go`（または `analyzer.go`）に移動する。

```go
// Compile-time check: StandardELFAnalyzer implements binaryanalyzer.BinaryAnalyzer.
var _ binaryanalyzer.BinaryAnalyzer = (*StandardELFAnalyzer)(nil)
```

### 3.2 `internal/runner/security/binary_analyzer.go`

```go
// 変更前の import
import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
)

// 変更後の import
import (
    "github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
    secelfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/security/machoanalyzer"
)
```

```go
// 変更前の呼び出し
return elfanalyzer.NewStandardELFAnalyzer(nil, nil)

// 変更後の呼び出し
return secelfanalyzer.NewStandardELFAnalyzer(nil, nil)
```

### 3.3 `internal/runner/security/syscall_store_adapter.go`

インポートと型宣言の変更は不要（すでに `internal/security/elfanalyzer` を参照している）。
コメントのみ更新する。

```go
// 変更前のコメント
// The returned value can be passed directly to elfanalyzer.NewStandardELFAnalyzerWithSyscallStore.

// 変更後のコメント
// The returned value can be passed directly to secelfanalyzer.NewStandardELFAnalyzerWithSyscallStore.
```

## 4. 削除ファイル

### 4.1 `internal/runner/security/elfanalyzer/analyzer.go`

内容（コンパイル時チェックのみ）を `internal/security/elfanalyzer` 側に移動後、このファイルを削除する。

### 4.2 `internal/runner/security/elfanalyzer/standard_analyzer.go`

`internal/security/elfanalyzer/standard_analyzer.go` に内容を移動後、このファイルを削除する。

### 4.3 `internal/runner/security/elfanalyzer/testdata/`

`internal/security/elfanalyzer/testdata/` と同一内容のため削除する。
削除前に両ディレクトリのファイルが一致することを確認する。

## 5. テストファイルの移動と変更

### 5.1 `analyzer_test.go`

**移動元**: `internal/runner/security/elfanalyzer/analyzer_test.go`
**移動先**: `internal/security/elfanalyzer/analyzer_test.go`

#### ビルドタグ・パッケージ宣言

変更なし（`//go:build test && linux`、`package elfanalyzer` を維持）。

#### インポート変更

| 変更 | 詳細 |
|------|------|
| 削除 | `secelfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"` |

`secelfanalyzer.` プレフィックスを持つ参照はすべて自パッケージの直接参照に変更する。

| 変更前 | 変更後 |
|-------|--------|
| `secelfanalyzer.SyscallAnalysisResult` | `SyscallAnalysisResult` |
| `secelfanalyzer.SyscallInfo` | `SyscallInfo` |
| `secelfanalyzer.ErrSyscallHashMismatch` | `ErrSyscallHashMismatch` |

`elfanalyzertesting` のインポートパスは変更なし（同一パッケージ内の testing サブパッケージ）。

#### モック型のインターフェース変更

移動後のテストでは `SyscallAnalysisStore` が自パッケージに存在するため参照は変更なし。

```go
// 変更なし（自パッケージ参照のため）
type mockSyscallAnalysisStore struct {
    result *SyscallAnalysisResult  // secelfanalyzer. プレフィックスを除去
    err    error
    expectedHash string
}

func (m *mockSyscallAnalysisStore) LoadSyscallAnalysis(_ string, expectedHash string) (*SyscallAnalysisResult, error) {
    ...
}
```

#### `testdataDir` パスの変更

```go
// 変更前（runner package での相対パス）
testdataDir := "testdata"

// 変更後（security package での相対パス）
testdataDir := "testdata"  // パスは同一のため変更なし
```

### 5.2 `analyzer_benchmark_test.go`

**移動元**: `internal/runner/security/elfanalyzer/analyzer_benchmark_test.go`
**移動先**: `internal/security/elfanalyzer/analyzer_benchmark_test.go`

変更なし（パッケージ宣言 `package elfanalyzer` を維持。`NewStandardELFAnalyzer` は同一パッケージのため import 変更不要）。

### 5.3 `standard_analyzer_fallback_test.go`

**移動元**: `internal/runner/security/elfanalyzer/standard_analyzer_fallback_test.go`
**移動先**: `internal/security/elfanalyzer/standard_analyzer_fallback_test.go`

変更なし（パッケージ宣言 `package elfanalyzer` を維持。プライベート関数 `isLibcLibrary`、`categorizeELFSymbol` は移動先の `standard_analyzer.go` に存在する）。

## 6. 受け入れ基準の検証方法

各実装ステップ完了後、以下のコマンドで受け入れ基準を確認する。

| AC | 検証コマンド |
|----|-------------|
| AC-1 | `go list -deps ./cmd/record \| grep internal/runner/security/elfanalyzer`（0件を確認） |
| AC-2 | `go list -deps ./cmd/verify \| grep internal/runner/security`（0件を確認） |
| AC-3 | `go build ./cmd/record ./cmd/verify ./cmd/runner` |
| AC-4 | `make test` |
| AC-5 | `cmd/runner` の統合テスト（`integration_cmd_allowed_security_test.go` 等）がパス |
