# safefileio 公開 API の ResolvedPath 対応 要件定義書

## 1. 概要

### 1.1 背景

タスク 0084 で `common.ResolvedPath` が struct 化され、コンストラクタ（`NewResolvedPath` / `NewResolvedPathParentOnly`）を経由しなければ有効な値を生成できなくなった。これにより「絶対パス・親ディレクトリのシンボリックリンク解決済み」であることが型レベルで保証されるようになった。

しかし `safefileio` パッケージの公開 API は依然として `string` を受け取っており、型安全性の恩恵が届いていない：

| 関数 | 現在のシグネチャ |
|------|----------------|
| `SafeReadFile` | `(filePath string) ([]byte, error)` |
| `SafeWriteFile` | `(filePath string, content []byte, perm os.FileMode) error` |
| `SafeWriteFileOverwrite` | `(filePath string, content []byte, perm os.FileMode) error` |
| `SafeAtomicMoveFile` | `(srcPath, dstPath string, requiredPerm os.FileMode) error` |

また、タスク 0084 では `NewResolvedPathParentOnly`（親ディレクトリのみを解決するコンストラクタ）の公開化が YAGNI として先送りされ、`internal/common/test_helpers.go` に非公開のまま残っている。本タスクで `SafeWriteFile` 等が `ResolvedPath` を受け取るために必要となるため、合わせて公開化する。

### 1.2 採用アプローチ

上記 4 関数のシグネチャを `string` から `common.ResolvedPath` に変更する。あわせて内部関数（`WithFS` バリアント、`safeWriteFileCommon`、`safeAtomicMoveFileWithFS`）も同様に変更し、各関数内部の冗長な `filepath.Abs()` 呼び出しを削除する。

`FileSystem` インターフェース（`SafeOpenFile`、`AtomicMoveFile`）は内部インターフェースであり、本タスクでは変更しない。

### 1.3 スコープ

**対象:**
- `common.NewResolvedPathParentOnly` の公開化
- `SafeReadFile` / `SafeWriteFile` / `SafeWriteFileOverwrite` / `SafeAtomicMoveFile` のシグネチャ変更
- 上記に対応する `WithFS` バリアントおよび `safeWriteFileCommon`、`safeAtomicMoveFileWithFS` の変更
- `ResolvedPath` に解決モード（`resolveMode`）を保持し、`IsParentOnly()` で参照できるようにする
- `safeWriteFileCommon` / `safeAtomicMoveFileWithFS` の入口で `NewResolvedPathParentOnly` 由来であることを検証する
- 各関数内部の冗長な `filepath.Abs()` 呼び出しの削除
- 全呼び出し箇所の移行（プロダクションコードおよびテスト）

**対象外:**
- `FileSystem` インターフェース（`SafeOpenFile`、`AtomicMoveFile`）のシグネチャ変更
- `SafeFileManager.MoveToFinal` のシグネチャ変更（`runner/output` パッケージ）
- `safefileio` パッケージ以外のセキュリティロジックの変更
- `SafeReadFile` / `SafeReadFileWithFS` に対する解決モード制約の追加（両コンストラクタが正当であるため）

---

## 2. 用語定義

タスク 0084 の用語定義を継承する。追加定義を以下に示す。

| 用語 | 定義 |
|------|------|
| 親のみ解決済みパス | 親ディレクトリが `filepath.EvalSymlinks` 済みであることが保証されたパス。リーフは元のファイル名をそのまま保持する。ファイル自体の存否は問わない。`NewResolvedPathParentOnly` が返す |
| 完全解決済みパス | ファイル自体も含めて `filepath.EvalSymlinks` 済みのパス。ファイルが存在することが前提。`NewResolvedPath` が返す |

---

## 3. 機能要件

### 3.1 `NewResolvedPathParentOnly` の公開化

#### FR-1.1: 公開コンストラクタの追加

`internal/common/test_helpers.go` の非公開関数 `newResolvedPathForNew` を基に、`internal/common/filesystem.go` に公開コンストラクタ `NewResolvedPathParentOnly` を追加する。

```go
// NewResolvedPathParentOnly は親ディレクトリのみに filepath.EvalSymlinks を適用し、
// ファイル名を結合して ResolvedPath を生成する。
// リーフ（ファイル名部分）は解決しないため、SafeReadFile 等が
// openat2(RESOLVE_NO_SYMLINKS) でリーフのシンボリックリンクを検出できる。
// 親ディレクトリが存在しない場合はエラーを返す。
// ファイル自体の存否は問わない（新規作成・上書き・読み込みいずれにも使用できる）。
func NewResolvedPathParentOnly(path string) (ResolvedPath, error)
```

処理手順:
1. `path` が空文字列の場合は `ErrEmptyPath` を返す
2. `filepath.Abs(path)` で絶対パスに変換する
3. `filepath.Dir(absPath)` で親ディレクトリを取得する
4. `filepath.EvalSymlinks(parentDir)` で親ディレクトリのシンボリックリンクを解決する（親が存在しない場合はエラー）
5. `filepath.Join(resolvedParent, filepath.Base(absPath))` でファイル名を結合する
6. `ResolvedPath{path: joined}` を返す

タスク 0084 要件書 FR-2.2 の仕様と同一。

#### FR-1.2: テストヘルパーの更新

`test_helpers.go` の `newResolvedPathForNew` は `NewResolvedPathParentOnly` を呼び出した上でファイルの非存在チェックを追加する薄いラッパーに変更する。既存の `errPathAlreadyExists` エラーは引き続き test-only スコープで保持する。

```go
func newResolvedPathForNew(path string) (ResolvedPath, error) {
    rp, err := NewResolvedPathParentOnly(path)
    if err != nil {
        return ResolvedPath{}, err
    }
    if _, err := os.Lstat(rp.String()); err == nil {
        return ResolvedPath{}, errPathAlreadyExists
    }
    return rp, nil
}
```

---

### 3.2 `SafeReadFile` のシグネチャ変更

#### FR-2.1: 公開関数

```go
// 変更前
func SafeReadFile(filePath string) ([]byte, error)

// 変更後
func SafeReadFile(filePath common.ResolvedPath) ([]byte, error)
```

#### FR-2.2: 内部関数

```go
// 変更前
func SafeReadFileWithFS(filePath string, fs FileSystem) ([]byte, error)

// 変更後
func SafeReadFileWithFS(filePath common.ResolvedPath, fs FileSystem) ([]byte, error)
```

`SafeReadFileWithFS` 内の `filepath.Abs(filePath)` 呼び出しを削除し、`filePath.String()` を直接 `fs.SafeOpenFile` に渡す。

---

### 3.3 `SafeWriteFile` のシグネチャ変更

#### FR-3.1: 公開関数

```go
// 変更前
func SafeWriteFile(filePath string, content []byte, perm os.FileMode) error

// 変更後
func SafeWriteFile(filePath common.ResolvedPath, content []byte, perm os.FileMode) error
```

呼び出し元は `NewResolvedPathParentOnly` で `ResolvedPath` を生成して渡す。

#### FR-3.2: 内部関数

```go
// 変更前
func safeWriteFileWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) error
func safeWriteFileCommon(filePath string, content []byte, perm os.FileMode, fs FileSystem, flags int) error

// 変更後
func safeWriteFileWithFS(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs FileSystem) error
func safeWriteFileCommon(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs FileSystem, flags int) error
```

`safeWriteFileCommon` 内の `filepath.Abs(filePath)` 呼び出しを削除し、`filePath.String()` を直接使用する。

---

### 3.4 `SafeWriteFileOverwrite` のシグネチャ変更

#### FR-4.1: 公開関数

```go
// 変更前
func SafeWriteFileOverwrite(filePath string, content []byte, perm os.FileMode) error

// 変更後
func SafeWriteFileOverwrite(filePath common.ResolvedPath, content []byte, perm os.FileMode) error
```

呼び出し元は `NewResolvedPathParentOnly` で `ResolvedPath` を生成して渡す。ファイルが既存でも新規でも `NewResolvedPathParentOnly` が適切（FR-1.1 参照）。

#### FR-4.2: 内部関数

```go
// 変更前
func safeWriteFileOverwriteWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) error

// 変更後
func safeWriteFileOverwriteWithFS(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs FileSystem) error
```

---

### 3.5 `SafeAtomicMoveFile` のシグネチャ変更

#### FR-5.1: 公開関数

```go
// 変更前
func SafeAtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error

// 変更後
func SafeAtomicMoveFile(srcPath, dstPath common.ResolvedPath, requiredPerm os.FileMode) error
```

呼び出し元での `ResolvedPath` 生成方法:
- `srcPath`: `NewResolvedPathParentOnly` を使用する（リーフのシンボリックリンク検知を維持するため）
- `dstPath`: 移動先ファイルは存在しない場合があるため `NewResolvedPathParentOnly` を使用する

#### FR-5.2: 内部関数

```go
// 変更前
func safeAtomicMoveFileWithFS(srcPath, dstPath string, requiredPerm os.FileMode, fs FileSystem) error

// 変更後
func safeAtomicMoveFileWithFS(srcPath, dstPath common.ResolvedPath, requiredPerm os.FileMode, fs FileSystem) error
```

`safeAtomicMoveFileWithFS` 内の `filepath.Abs(srcPath)` / `filepath.Abs(dstPath)` 呼び出しを削除し、`srcPath.String()` / `dstPath.String()` を直接使用する。

---

### 3.6 呼び出し箇所の移行

#### FR-6.1: `internal/filevalidator/validator.go`

**`calculateHash`**

```go
// 変更前
func (v *Validator) calculateHash(filePath string) (string, error) {
    content, err := safefileio.SafeReadFile(filePath)
    ...
}
// 呼び出し元: v.calculateHash(targetPath.String())

// 変更後
func (v *Validator) calculateHash(filePath common.ResolvedPath) (string, error) {
    content, err := safefileio.SafeReadFile(filePath)
    ...
}
// 呼び出し元: v.calculateHash(targetPath)
```

**`VerifyAndRead`**

```go
// 変更前
content, err := safefileio.SafeReadFile(targetPath.String())

// 変更後
content, err := safefileio.SafeReadFile(targetPath)
```

#### FR-6.2: `internal/fileanalysis/file_analysis_store.go`

**`Load`（`SafeReadFile` 呼び出し）**

```go
// 変更前
data, err := safefileio.SafeReadFile(recordPath)  // recordPath は string

// 変更後
// NewResolvedPathParentOnly（親のみ解決）を使用する。
// NewResolvedPath（完全解決）を使うとリーフのシンボリックリンクが先に解決され、
// SafeReadFile の openat2(RESOLVE_NO_SYMLINKS) によるシンボリックリンク検出が
// 機能しなくなるため不可。
resolvedRecordPath, err := common.NewResolvedPathParentOnly(recordPath)
if err != nil {
    return nil, fmt.Errorf("failed to resolve record path: %w", err)
}
data, err := safefileio.SafeReadFile(resolvedRecordPath)
```

**`Save`（`SafeWriteFileOverwrite` 呼び出し）**

```go
// 変更前
err = safefileio.SafeWriteFileOverwrite(recordPath, data, filePermission)  // recordPath は string

// 変更後
resolvedRecordPath, err := common.NewResolvedPathParentOnly(recordPath)
if err != nil {
    return fmt.Errorf("failed to resolve record path: %w", err)
}
err = safefileio.SafeWriteFileOverwrite(resolvedRecordPath, data, filePermission)
```

#### FR-6.3: `internal/runner/config/loader.go`

```go
// 変更前
content, err = safefileio.SafeReadFile(path)  // テスト専用パス、path は string

// 変更後
resolvedPath, err := common.NewResolvedPath(path)
if err != nil {
    return nil, fmt.Errorf("failed to resolve template path: %w", err)
}
content, err = safefileio.SafeReadFile(resolvedPath)
```

#### FR-6.4: テスト群

`SafeReadFile`、`SafeWriteFile`、`SafeWriteFileOverwrite`、`SafeAtomicMoveFile` を直接呼ぶテストは、`ResolvedPath` を生成して渡すよう変更する。

- 新規作成・上書き・読み込み（リーフのシンボリックリンク検出を保持）: `NewResolvedPathParentOnly` または `newResolvedPathForNew`（test-only、存在チェックあり）
- 既存ファイル（シンボリックリンクを追跡して実体パスを得る場合）: `NewResolvedPath`
- `SafeAtomicMoveFile` の srcPath: `NewResolvedPathParentOnly`、dstPath: `NewResolvedPathParentOnly`

`WithFS` バリアント（`safeWriteFileWithFS` 等）を直接呼ぶテストも同様に変更する。

---

### 3.7 コンストラクタ制約の実行時強制（ADR 反映）

#### FR-7.1: `ResolvedPath` への解決モード追加

`internal/common/filesystem.go` の `ResolvedPath` に、どのコンストラクタ経由で生成されたかを保持する解決モードを追加する。

- `NewResolvedPath` は `resolveModeFull` を設定する
- `NewResolvedPathParentOnly` は `resolveModeParentOnly` を設定する
- `ResolvedPath` に `IsParentOnly() bool` を追加し、外部パッケージ（`safefileio`）はこのメソッド経由で判定する

`mode` フィールドは unexported とし、パッケージ外から直接参照させない。

#### FR-7.2: `safefileio` のセキュリティ境界でアサーション

`internal/safefileio/safe_file.go` で以下の実行時チェックを追加する。

- `safeWriteFileCommon`: `filePath.IsParentOnly()` が `false` の場合は `ErrInvalidFilePath` を返す
- `safeAtomicMoveFileWithFS`: `srcPath.IsParentOnly()` または `dstPath.IsParentOnly()` が `false` の場合は `ErrInvalidFilePath` を返す

`SafeReadFile` / `SafeReadFileWithFS` にはモードアサーションを追加しない。

#### FR-7.3: 誤用検知テストの追加

`NewResolvedPath` で生成した `ResolvedPath` を以下に渡した場合、`ErrInvalidFilePath` を返すことをテストで検証する。

- `SafeWriteFile`
- `SafeWriteFileOverwrite`
- `SafeAtomicMoveFile`（`srcPath` または `dstPath`）

---

## 4. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `common.NewResolvedPathParentOnly("/existing/dir/newfile.txt")` は親ディレクトリが存在する場合に成功し、親ディレクトリが EvalSymlinks 済みのパスを返す |
| AC-2 | `common.NewResolvedPathParentOnly("/nonexistent/dir/newfile.txt")` は親ディレクトリが存在しない場合にエラーを返す |
| AC-3 | `common.NewResolvedPathParentOnly("")` は `ErrEmptyPath` を返す |
| AC-4 | `common.NewResolvedPathParentOnly("/existing/dir/existingfile.txt")` はファイルが既に存在していても成功する（ファイルの存否を問わない） |
| AC-5 | `SafeWriteFile` に `common.ResolvedPath{}` （ゼロ値）を渡した場合にエラーを返す |
| AC-6 | `SafeWriteFileOverwrite` に `common.ResolvedPath{}` を渡した場合にエラーを返す |
| AC-7 | `SafeReadFile` に `common.ResolvedPath{}` を渡した場合にエラーを返す |
| AC-8 | `SafeAtomicMoveFile` に `common.ResolvedPath{}` を渡した場合にエラーを返す |
| AC-9 | `safeWriteFileCommon` / `safeAtomicMoveFileWithFS` / `SafeReadFileWithFS` の内部で `filepath.Abs()` を直接呼び出す箇所がない |
| AC-10 | `internal/fileanalysis/file_analysis_store.go` の `Load` / `Save` が `ResolvedPath` を介して `SafeReadFile` / `SafeWriteFileOverwrite` を呼ぶ |
| AC-11 | `internal/filevalidator/validator.go` の `calculateHash` が `ResolvedPath` を受け取り、`SafeReadFile` に直接渡す |
| AC-12 | `internal/runner/config/loader.go` のテスト専用パスが `NewResolvedPath` を介して `SafeReadFile` を呼ぶ |
| AC-13 | `ResolvedPath` が解決モードを保持し、`IsParentOnly()` で判定できる |
| AC-14 | `NewResolvedPath` で作成した値を `SafeWriteFile` に渡すと `ErrInvalidFilePath` を返す |
| AC-15 | `NewResolvedPath` で作成した値を `SafeWriteFileOverwrite` に渡すと `ErrInvalidFilePath` を返す |
| AC-16 | `NewResolvedPath` で作成した値を `SafeAtomicMoveFile`（`srcPath` または `dstPath`）に渡すと `ErrInvalidFilePath` を返す |
| AC-17 | `SafeReadFile` / `SafeReadFileWithFS` は `NewResolvedPath` / `NewResolvedPathParentOnly` の両方を受け入れる |
| AC-18 | `make test` が全パッケージでパスする |
| AC-19 | `make lint` がエラーを報告しない |
