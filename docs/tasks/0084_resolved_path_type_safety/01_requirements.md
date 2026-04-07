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

### 1.2 採用アプローチ

`ResolvedPath` を unexported フィールドを持つ struct に変更し、パッケージ外からコンストラクタを経ずに有効な値を生成できないようにする。コンストラクタは用途に応じて 2 種類提供する:

- **`NewResolvedPath`**: 既存ファイル用。`filepath.Abs` + `filepath.EvalSymlinks` を内部で実行する。
- **`NewResolvedPathForNew`**: 新規ファイル用。親ディレクトリに `filepath.EvalSymlinks` を適用し、ファイル名部分を結合する。

### 1.3 スコープ

**対象:**
- `common.ResolvedPath` の型定義変更（string → struct）
- `NewResolvedPath` の動作変更（EvalSymlinks を内包）
- `NewResolvedPathForNew` の新規追加
- 全呼び出し箇所の移行

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
| コンストラクタ | `ResolvedPath` を生成できる唯一の公開手段。`NewResolvedPath` または `NewResolvedPathForNew` |

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
    path string // unexported
}
```

パッケージ外からは `common.ResolvedPath{path: "..."}` という直接初期化がコンパイルエラーになる。ゼロ値（`common.ResolvedPath{}`）のみ生成可能だが、各関数の非 empty チェックで弾かれる。

#### FR-1.2: メソッドの維持

既存の `String() string` メソッドを維持する:

```go
func (p ResolvedPath) String() string { return p.path }
```

#### FR-1.3: 比較可能性の維持

struct の全フィールドが比較可能な型（string）であるため、`==` / `!=` による比較はそのまま維持される。

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

#### FR-2.2: `NewResolvedPathForNew`（新規ファイル用）の追加

新規ファイル（まだ存在しないファイル）のパスを表す `ResolvedPath` を生成するコンストラクタを追加する:

```go
// NewResolvedPathForNew は新規作成予定ファイルのパスを解決して ResolvedPath を生成する。
// 親ディレクトリに filepath.EvalSymlinks を適用し、ファイル名を結合する。
// 親ディレクトリが存在しない場合はエラーを返す。
func NewResolvedPathForNew(path string) (ResolvedPath, error)
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

現状は自前で `filepath.EvalSymlinks` を呼んでから `NewResolvedPath` を呼んでいる。`NewResolvedPath` が EvalSymlinks を内包するため、二重解決になるが動作は同一。

`validatePath` は `IsRegular` チェック（ドメインロジック）を引き続き担当する。変更後の形:

```go
func validatePath(filePath string) (common.ResolvedPath, error) {
    rp, err := common.NewResolvedPath(filePath) // Abs + EvalSymlinks を内包
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

#### FR-3.2: `fileanalysis/syscall_store.go`

`SaveSyscallAnalysis(filePath string, ...)` と `LoadSyscallAnalysis(filePath string, ...)` は内部で `NewResolvedPath(filePath)` を呼んでいる。`NewResolvedPath` が EvalSymlinks を内包することで、コード変更なしに正しく動作するようになる（呼び出し元はすでに解決済みパスを渡している）。

#### FR-3.3: `fileanalysis/network_symbol_store.go`

`LoadNetworkSymbolAnalysis(filePath string, ...)` は内部で `NewResolvedPath(filePath)` を呼んでいる。FR-3.2 と同様、コード変更不要。

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

`getRecordPath` が返す string は `<resolvedDir>/<hash>.json` の形式であり、親ディレクトリ（`resolvedDir`）は EvalSymlinks 済みであることが保証される。`Load` 時（record ファイルが存在する）は `NewResolvedPath(recordPath)` で解決する。`Save`/`Update` 時（record ファイルが未存在の可能性あり）は `NewResolvedPathForNew(recordPath)` で解決する。

#### FR-3.5: `common.HashFilePathGetter` インターフェース

`GetHashFilePath(hashDir string, filePath ResolvedPath) (string, error)` の第 1 引数 `hashDir` は、`Store.analysisDir` が `ResolvedPath` 型になることに伴い変更する:

```go
type HashFilePathGetter interface {
    GetHashFilePath(hashDir common.ResolvedPath, filePath ResolvedPath) (string, error)
}
```

実装（`SHA256PathHashGetter`、`HybridHashFilePathGetter`）のシグネチャを合わせて変更する。

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

`NewResolvedPath` が呼ばれるたびに `filepath.EvalSymlinks`（システムコール）が実行される。既存コードでは多くの箇所で `validatePath` → `NewResolvedPath` という経路を経ており、EvalSymlinks の二重呼び出しが発生する場合がある。これは許容する（ファイル I/O と比べてコストは小さい）。

---

## 4. 受け入れ基準

| ID | 基準 |
|----|------|
| AC-1 | `common.ResolvedPath{path: "/foo"}` および `common.ResolvedPath("/foo")` はパッケージ外でコンパイルエラーになる |
| AC-2 | `common.NewResolvedPath("/nonexistent/path")` は存在しないパスに対してエラーを返す |
| AC-3 | `common.NewResolvedPath("/some/symlink")` はシンボリックリンクを解決した実体パスの `ResolvedPath` を返す |
| AC-4 | `common.NewResolvedPath("")` は `ErrEmptyPath` を返す |
| AC-5 | `common.NewResolvedPath("relative/path")` は絶対パスに変換した上で EvalSymlinks を適用した結果を返す |
| AC-6 | `common.NewResolvedPathForNew("/existing/dir/newfile.txt")` は親ディレクトリが存在する場合に成功し、親ディレクトリが EvalSymlinks 済みのパスを返す |
| AC-7 | `common.NewResolvedPathForNew("/nonexistent/dir/newfile.txt")` は親ディレクトリが存在しない場合にエラーを返す |
| AC-8 | `common.NewResolvedPathForNew("")` は `ErrEmptyPath` を返す |
| AC-9 | `ResolvedPath{}` の `String()` は `""` を返す |
| AC-10 | 2 つの `ResolvedPath` が同じパス文字列を指す場合 `==` で等しいと判定される |
| AC-11 | `make test` が全パッケージでパスする |
| AC-12 | `make lint` がエラーを報告しない |
| AC-13 | `fileanalysis.NewStore` に渡したディレクトリパスがシンボリックリンクを含む場合、内部で EvalSymlinks 済みのパスが使われる |
| AC-14 | `filevalidator.validatePath` が内部で EvalSymlinks を二重実行しても、最終結果が正しい `ResolvedPath` を返す |
