# ResolvedPath 型安全性強化 要件定義書

## 1. 概要

### 1.1 背景

`common.ResolvedPath` は現在 `type ResolvedPath string` として定義されており、「絶対パス・シンボリックリンク解決済み」であることを型として保証できない。

コードベース上のパスには以下の 3 種類が存在する:

| 種別 | 説明 | 例 |
|------|------|-----|
| a) 任意パス | 絶対・相対どちらもありえる | `loadTemplate(path string)` の引数 |
| b) 絶対パス（未解決） | 絶対パスだがシンボリックリンク未解決 | `filepath.Abs()` の戻り値 |
| c) 絶対パス（解決済み） | 絶対パス・シンボリックリンク解決済み | `filepath.EvalSymlinks()` 適用後 |

ハッシュレコードへの記録・検証など、セキュリティ上 c) が必須の用途がある。しかし現状は以下の問題がある:

- `type ResolvedPath string` は `common.ResolvedPath("/some/path")` という直接型変換がコンパイルを通る
- `NewResolvedPath(path string)` は非 empty チェックのみで EvalSymlinks を行わない
- c) の保証が慣習にとどまり、誤用をコンパイル時に検出できない
- `fileanalysis/syscall_store.go` 等、EvalSymlinks を経ない raw な string から `NewResolvedPath` を呼んでいる箇所が存在する
- `ResolvedPath` 導入後も、呼び出し側に残った `filepath.Abs` / `filepath.EvalSymlinks` が二重正規化を引き起こすおそれがある

### 1.2 採用アプローチ

`ResolvedPath` を unexported フィールドを持つ struct に変更し、パッケージ外からコンストラクタを経ずに有効な値を生成できないようにする。あわせて、既存の呼び出し側に分散している不要な `filepath.Abs` / `filepath.EvalSymlinks` を整理し、パス正規化の責務をコンストラクタ境界へ寄せる。コンストラクタは用途に応じて 2 種類提供する:

- **`NewResolvedPath`**: 既存ファイル用。`filepath.Abs` + `filepath.EvalSymlinks` を内部で実行する。
- **`NewResolvedPathParentOnly`**: 親ディレクトリのみ解決する用途（新規作成・上書き・シンボリックリンク検出を保持した読み込みなど）。親ディレクトリに `filepath.EvalSymlinks` を適用し、ファイル名部分をそのまま結合する。

### 1.3 スコープ

**対象:**
- `common.ResolvedPath` の型定義変更（string → struct）
- `NewResolvedPath` の動作変更（EvalSymlinks を内包）
- `NewResolvedPathParentOnly` の新規追加
- 全呼び出し箇所の移行
- `ResolvedPath` 導入により不要になる `filepath.Abs` / `filepath.EvalSymlinks` / 再ラップ処理の削除

**対象外:**
- `safefileio.SafeReadFile` / `safefileio.SafeWriteFile` 等のシグネチャ変更（別タスク）
- `BinaryAnalyzer.AnalyzeNetworkSymbols` 等の上位インターフェースへの `ResolvedPath` 伝搬（別タスク）

---

## 2. 用語定義

| 用語 | 定義 |
|------|------|
| ResolvedPath | 絶対パス・シンボリックリンク解決済みのファイルパスを表す型。既存ファイルについては `filepath.EvalSymlinks` の結果と一致することが保証される。新規ファイルについては親ディレクトリが `filepath.EvalSymlinks` 済みであることが保証される |
| 任意パス (a) | 絶対・相対のどちらもありえる raw な string パス |
| 絶対未解決パス (b) | `filepath.Abs` 適用済みだが `filepath.EvalSymlinks` 未適用のパス |
| 解決済みパス (c) | `filepath.EvalSymlinks` 適用済みの絶対パス。`ResolvedPath` が表す |
| コンストラクタ | `ResolvedPath` を生成できる唯一の公開手段。`NewResolvedPath` または `NewResolvedPathParentOnly` |

---

## 3. 機能要件

### 3.1 型定義の変更

#### FR-1.1: struct 化

`common.ResolvedPath` を以下のように変更する:

```go
// 変更前
type ResolvedPath string

// 変更後
type ResolvedPath struct {
    path string      // unexported
    mode resolveMode // unexported; set by constructor
}
```

パッケージ外からは `common.ResolvedPath{path: "..."}` という直接初期化がコンパイルエラーになる。ゼロ値（`common.ResolvedPath{}`）のみ生成可能だが、各関数の非 empty チェックで弾かれる。

#### FR-1.2: メソッドの維持

既存の `String() string` メソッドを維持する:

```go
func (p ResolvedPath) String() string { return p.path }
```

#### FR-1.3: 比較可能性と等値演算子の意味

`ResolvedPath` の全フィールドは比較可能な型であるため、`==` / `!=` はコンパイルエラーにならない。ただし `mode` フィールドが追加されたことにより、**同じパス文字列でもコンストラクタが異なる場合は `==` で等しくならない**。

```go
a, _ := NewResolvedPath("/some/file")           // mode = resolveModeFull
b, _ := NewResolvedPathParentOnly("/some/file") // mode = resolveModeParentOnly
a == b // false（path は同じ、mode が異なる）
a.String() == b.String() // true
```

パス文字列のみを比較したい場合は `.String()` を経由する。`ResolvedPath` をマップキーとして使う場合も `map[string]V` に `p.String()` で格納するか、同一コンストラクタ由来の値に限定すること。

#### FR-1.4: ゼロ値の明示

ゼロ値 `ResolvedPath{}` は空文字列を表し、有効な ResolvedPath ではない。`String()` は `""` を返す。

---

### 3.2 コンストラクタの変更・追加

#### FR-2.1: `NewResolvedPath`（既存ファイル用）の動作変更

シグネチャは変えず、内部で `filepath.Abs` と `filepath.EvalSymlinks` を実行するよう変更する:

```go
// NewResolvedPath は既存ファイルのパスを解決して ResolvedPath を生成する。
// filepath.Abs と filepath.EvalSymlinks を内部で呼び出すため、
// ファイルが存在しない場合はエラーを返す。
func NewResolvedPath(path string) (ResolvedPath, error)
```

処理手順:
1. `path` が空文字列の場合は `ErrEmptyPath` を返す
2. `filepath.Abs(path)` で絶対パスに変換する
3. `filepath.EvalSymlinks(absPath)` でシンボリックリンクを解決する（ファイル不存在の場合はここでエラー）
4. `ResolvedPath{path: resolvedPath}` を返す

#### FR-2.2: `NewResolvedPathParentOnly`（新規ファイル用）の追加

新規ファイル（まだ存在しないファイル）のパスを表す `ResolvedPath` を生成するコンストラクタを追加する:

```go
// NewResolvedPathParentOnly は新規作成予定ファイルのパスを解決して ResolvedPath を生成する。
// 親ディレクトリに filepath.EvalSymlinks を適用し、ファイル名を結合する。
// 親ディレクトリが存在しない場合はエラーを返す。
func NewResolvedPathParentOnly(path string) (ResolvedPath, error)
```

処理手順:
1. `path` が空文字列の場合は `ErrEmptyPath` を返す
2. `filepath.Abs(path)` で絶対パスに変換する
3. `filepath.Dir(absPath)` で親ディレクトリを取得する
4. `filepath.EvalSymlinks(parentDir)` で親ディレクトリのシンボリックリンクを解決する（親が存在しない場合はここでエラー）
5. `filepath.Join(resolvedParent, filepath.Base(absPath))` でファイル名を結合する
6. `ResolvedPath{path: joined}` を返す

---

### 3.3 呼び出し箇所の移行

#### FR-3.1: `filevalidator/validator.go:validatePath`

現状は自前で `filepath.Abs` / `filepath.EvalSymlinks` を呼んでから `NewResolvedPath` を呼んでいる。`NewResolvedPath` が Abs + EvalSymlinks を内包するため、この重複処理は削除する。

`validatePath` は `IsRegular` チェック（ドメインロジック）を引き続き担当する。変更後の形:

```go
func validatePath(filePath string) (common.ResolvedPath, error) {
    rp, err := common.NewResolvedPath(filePath) // Abs + EvalSymlinks を内包。空文字は common.ErrEmptyPath を返す
    if err != nil {
        return common.ResolvedPath{}, err
    }
    fi, err := os.Lstat(rp.String())
    if err != nil {
        return common.ResolvedPath{}, err
    }
    if !fi.Mode().IsRegular() {
        return common.ResolvedPath{}, fmt.Errorf("%w: not a regular file: %s", ...)
    }
    return rp, nil
}
```

`validatePath` 内から `filepath.Abs` および `filepath.EvalSymlinks` の直接呼び出しは除去する。これにより、パス解決は `NewResolvedPath` に一元化される。

**空文字チェックの設計判断（二重チェックの削除）**

`validatePath` の先頭にある `filePath == ""` → `safefileio.ErrInvalidFilePath` のチェックは削除する。`NewResolvedPath` が空文字列を `common.ErrEmptyPath` として処理するため冗長であり、エラー生成責務が分散する。

`safefileio.ErrInvalidFilePath` を参照しているのは `internal/filevalidator/validator.go` と `internal/filevalidator/validator_error_test.go` の2ファイルのみであり、いずれも内部コードである。本タスクの中で以下を合わせて実施する:

1. `validatePath` の先頭の空文字チェックを削除する
2. `validator_error_test.go` の "empty path" ケースの期待エラーを `safefileio.ErrInvalidFilePath` → `common.ErrEmptyPath` に変更する

#### FR-3.2: `fileanalysis/syscall_store.go`

`SaveSyscallAnalysis(filePath string, ...)` と `LoadSyscallAnalysis(filePath string, ...)` は内部で `NewResolvedPath(filePath)` を呼んでいる。`NewResolvedPath` が EvalSymlinks を内包することで正しく動作するため、この関数群の前後に追加の `filepath.Abs` / `filepath.EvalSymlinks` を導入しない。

#### FR-3.3: `fileanalysis/network_symbol_store.go`

`LoadNetworkSymbolAnalysis(filePath string, ...)` は内部で `NewResolvedPath(filePath)` を呼んでいる。FR-3.2 と同様、コード変更不要であり、追加の事前正規化も行わない。

#### FR-3.4: `fileanalysis/file_analysis_store.go`

`Store.analysisDir`（record ファイルを格納するディレクトリパス）を `NewStore` 作成時に解決し、`ResolvedPath` で保持する:

```go
type Store struct {
    analysisDir common.ResolvedPath // string から変更
    pathGetter  common.HashFilePathGetter
}

func NewStore(analysisDir string, pathGetter common.HashFilePathGetter) (*Store, error) {
    // ...（既存のディレクトリ存在確認・作成処理）
    resolvedDir, err := common.NewResolvedPath(analysisDir)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve analysis dir: %w", err)
    }
    return &Store{analysisDir: resolvedDir, pathGetter: pathGetter}, nil
}
```

`getRecordPath` が返す string は `<resolvedDir>/<hash>.json` の形式であり、親ディレクトリ（`resolvedDir`）は EvalSymlinks 済みであることが保証される。`getRecordPath` および `HashFilePathGetter` 実装は、この既に解決済みのディレクトリを前提としてパスを組み立て、そこで追加の `filepath.Abs` / `filepath.EvalSymlinks` を行わない。

#### FR-3.5: `common.HashFilePathGetter` インターフェース

`GetHashFilePath(hashDir string, filePath ResolvedPath) (string, error)` の第 1 引数 `hashDir` は、`Store.analysisDir` が `ResolvedPath` 型になることに伴い変更する:

```go
type HashFilePathGetter interface {
    GetHashFilePath(hashDir common.ResolvedPath, filePath ResolvedPath) (string, error)
}
```

実装（`SHA256PathHashGetter`、`HybridHashFilePathGetter`）のシグネチャを合わせて変更する。

この変更の目的は、ハッシュ保存先ディレクトリも「解決済みパス」として扱い、getter 実装側で再度正規化する余地をなくすことにある。

#### FR-3.6: テスト群

`NewResolvedPath(someTempPath)` を呼んでいるテストは `NewResolvedPath` が EvalSymlinks を内包することで、macOS の `/tmp` → `/private/tmp` 解決が自動で行われ正確性が上がる。テスト上のファイルはすべて `t.TempDir()` 配下（存在する）であるため、エラーは発生しない。

struct 化に伴い、`ResolvedPath` をマップのキーや型アサーションで使用しているテストがあれば確認・修正する。

---

### 3.4 非機能要件

#### NFR-1: 後方互換性

`common.ResolvedPath` の型定義変更はコンパイル時の破壊的変更となる（型変換 `common.ResolvedPath(someString)` が使えなくなる）。本リファクタリングは単一コミットで全呼び出し箇所を一括移行し、ビルド失敗が残存しないことを保証する。

#### NFR-2: ゼロ値の安全性

`ResolvedPath{}` のゼロ値を受け取った関数は `path == ""` の判定でエラーを返すことが既存コードで保証されている。ゼロ値の使用は `var rp common.ResolvedPath` 等の未初期化宣言に限定される。

#### NFR-3: パフォーマンス

`NewResolvedPath` / `NewResolvedPathParentOnly` は必要な正規化を内部で実行する。一方で、移行対象の呼び出し箇所では、同一パスに対する冗長な `filepath.Abs` / `filepath.EvalSymlinks` / `NewResolvedPath` の重複適用を残してはならない。少なくとも `filevalidator.validatePath` と `fileanalysis.Store` 周辺では、正規化が 1 回に集約されていることをコードレビューで確認できる状態にする。

---

## 4. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `common.ResolvedPath{path: "/foo"}` および `common.ResolvedPath("/foo")` はパッケージ外でコンパイルエラーになる |
| AC-2 | `common.NewResolvedPath("/nonexistent/path")` は存在しないパスに対してエラーを返す |
| AC-3 | `common.NewResolvedPath("/some/symlink")` はシンボリックリンクを解決した実体パスの `ResolvedPath` を返す |
| AC-4 | `common.NewResolvedPath("")` は `ErrEmptyPath` を返す |
| AC-5 | `common.NewResolvedPath("relative/path")` は絶対パスに変換した上で EvalSymlinks を適用した結果を返す |
| AC-6 | `common.NewResolvedPathParentOnly("/existing/dir/newfile.txt")` は親ディレクトリが存在する場合に成功し、親ディレクトリが EvalSymlinks 済みのパスを返す |
| AC-7 | `common.NewResolvedPathParentOnly("/nonexistent/dir/newfile.txt")` は親ディレクトリが存在しない場合にエラーを返す |
| AC-8 | `common.NewResolvedPathParentOnly("")` は `ErrEmptyPath` を返す |
| AC-9 | `ResolvedPath{}` の `String()` は `""` を返す |
| AC-10 | 同一コンストラクタで生成した 2 つの `ResolvedPath` が同じパス文字列を指す場合 `==` で等しいと判定される。異なるコンストラクタ（`NewResolvedPath` と `NewResolvedPathParentOnly`）で同じパス文字列を生成した場合は `==` で等しくならないが、`.String()` による比較では等しい |
| AC-11 | `make test` が全パッケージでパスする |
| AC-12 | `make lint` がエラーを報告しない |
| AC-13 | `fileanalysis.NewStore` に渡したディレクトリパスがシンボリックリンクを含む場合、内部で EvalSymlinks 済みのパスが使われる |
| AC-14 | `filevalidator.validatePath` は `filepath.Abs` / `filepath.EvalSymlinks` を直接呼ばず、`common.NewResolvedPath` に正規化を一元化した上で正しい `ResolvedPath` を返す |
| AC-15 | `fileanalysis.Store` および `HashFilePathGetter` 実装は、`analysisDir` を `ResolvedPath` として保持・受け渡しし、追加の `filepath.Abs` / `filepath.EvalSymlinks` を行わない |
