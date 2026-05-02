# ADR: ResolvedPath のコンストラクタ制約をセキュリティ境界で実施する方法の決定

## ステータス

採用済み（実装完了）

## コンテキスト

### 背景

`common.ResolvedPath` は struct 化されており、2 つのコンストラクタを持つ。

| コンストラクタ | 動作 | 主な用途 |
|---|---|---|
| `NewResolvedPath` | `filepath.EvalSymlinks` でリーフを含む全パス成分を解決する | 既存ファイルへのアクセス（`filevalidator`、`config/loader` 等） |
| `NewResolvedPathParentOnly` | 親ディレクトリのみ `EvalSymlinks` を適用し、リーフ（ファイル名）は元の文字列のまま保持する | 新規作成・上書き・アトミック移動先等 |

### セキュリティ上の前提

`safefileio` パッケージの各関数（`SafeWriteFile`、`SafeWriteFileOverwrite`、`SafeAtomicMoveFile`）は、リーフのシンボリックリンクを **検知して拒否する** ことを前提に設計されている。この検知は `SafeOpenFile` が `openat2(RESOLVE_NO_SYMLINKS)` を使用することで実現される。

### 問題

これらの関数が受け取る引数の型は `common.ResolvedPath` であり、2 つのコンストラクタのいずれで作成された値も渡すことができる。`NewResolvedPath` で作成した値は、リーフのシンボリックリンクが既に解決されているため、`SafeOpenFile` の `RESOLVE_NO_SYMLINKS` チェックをすり抜けてしまう。

**具体的なシナリオ:**

```
/tmp/link → /real/file  （攻撃者が用意したシンボリックリンク）
```

```go
// NewResolvedPath を使用した場合
srcRP, _ := common.NewResolvedPath("/tmp/link")
// srcRP.path == "/real/file"  （リーフのシンボリックリンクが解決済み）

SafeAtomicMoveFile(srcRP, dstRP, 0o600)
// SafeOpenFile は "/real/file" を直接開く
// openat2(RESOLVE_NO_SYMLINKS) はシンボリックリンクのないパスに対して成功する
// → シンボリックリンク検知が機能しない
```

```go
// NewResolvedPathParentOnly を使用した場合
srcRP, _ := common.NewResolvedPathParentOnly("/tmp/link")
// srcRP.path == "/tmp/link"  （リーフは元の文字列のまま）

SafeAtomicMoveFile(srcRP, dstRP, 0o600)
// SafeOpenFile は "/tmp/link" を openat2(RESOLVE_NO_SYMLINKS) で開こうとする
// → リーフがシンボリックリンクのため ErrIsSymlink が返る  ✓
```

### 各関数に必要なコンストラクタの整理

| 関数 | 引数 | 必要なコンストラクタ | 理由 |
|---|---|---|---|
| `SafeWriteFile` | `filePath` | `NewResolvedPathParentOnly` のみ | リーフのシンボリックリンク検知が必要 |
| `SafeWriteFileOverwrite` | `filePath` | `NewResolvedPathParentOnly` のみ | 同上 |
| `SafeAtomicMoveFile` | `srcPath` | `NewResolvedPathParentOnly` のみ | 同上 |
| `SafeAtomicMoveFile` | `dstPath` | `NewResolvedPathParentOnly` のみ | 移動先はファイルが不在の場合もある |
| `SafeReadFile` | `filePath` | **両方が正当** | `filevalidator`（`NewResolvedPath`）と `fileanalysis`（`NewResolvedPathParentOnly`）の双方から呼ばれる |

`SafeReadFile` は 2 種類のコンストラクタからの呼び出しが意味的にいずれも正当であるため、型レベルの制約を適用できない。

---

## 検討した案

### 案 A: コメントによる明記（採用せず）

`SafeAtomicMoveFile` 等のドキュメントコメントに「`NewResolvedPathParentOnly` で作成したパスを渡すこと」と明記する。

**却下理由:** コンパイル時にも実行時にも誤用を検知できず、コードレビュー時のみが防衛ラインとなる。

---

### 案 B: 別型 `ParentOnlyResolvedPath` の導入

```go
type ParentOnlyResolvedPath struct { rp ResolvedPath }
func (p ParentOnlyResolvedPath) String() string             { return p.rp.String() }
func (p ParentOnlyResolvedPath) AsResolvedPath() ResolvedPath { return p.rp }

func NewResolvedPathParentOnly(path string) (ParentOnlyResolvedPath, error) { ... }
```

`SafeWriteFile`、`SafeWriteFileOverwrite`、`SafeAtomicMoveFile` の `dstPath` 引数の型を `ParentOnlyResolvedPath` に変更する。

**Pros:**
- コンパイル時に誤用を防止できる
- 関数シグネチャが意図を自己文書化する

**Cons:**
- `SafeReadFile` は両コンストラクタが正当なため保護対象外（最も呼び出し頻度が高い関数が保護されない）
- `fileanalysis.Store.Load` で `ParentOnlyResolvedPath` → `ResolvedPath` の変換が必要（`.AsResolvedPath()`）となり、型を分けた恩恵が呼び出し元の見通しを悪化させる
- テストの `mustResolvedPath` が返す型の変更に伴い、`SafeReadFile` 呼び出し箇所（6〜8 か所）で変換の追加が必要
- `SafeAtomicMoveFile` の `srcPath` について「要件では `NewResolvedPath` を想定」とされていた仕様との矛盾を先に解消する必要がある
- 変更規模：約 45 行（うち変換ボイラープレートが 6〜8 か所）

---

### 案 C: `resolveMode` フィールドを `ResolvedPath` に追加（採用）

```go
type resolveMode int
const (
    resolveModeFull       resolveMode = iota + 1 // NewResolvedPath が設定。iota+1 にすることでゼロ値（0）がどちらのコンストラクタでも設定されない無効値となる
    resolveModeParentOnly                         // NewResolvedPathParentOnly が設定
)

type ResolvedPath struct {
    path string
    mode resolveMode
}
```

各コンストラクタが `mode` を設定し、`ResolvedPath` に追加する公開メソッド
`IsParentOnly() bool` を通じてセキュリティ境界でアサーションを行う。

```go
// safeAtomicMoveFileWithFS 内
if !srcPath.IsParentOnly() {
    return fmt.Errorf("%w: srcPath must use NewResolvedPathParentOnly", ErrInvalidFilePath)
}
if !dstPath.IsParentOnly() {
    return fmt.Errorf("%w: dstPath must use NewResolvedPathParentOnly", ErrInvalidFilePath)
}
```

同様のアサーションを `safeWriteFileCommon` にも追加することで、`SafeWriteFile`・`SafeWriteFileOverwrite` も一括保護できる。

**Pros:**
- 既存の関数シグネチャを変更しない → 既存呼び出し元への影響ゼロ
- 変換ボイラープレートが不要
- `SafeWriteFile`・`SafeWriteFileOverwrite`・`SafeAtomicMoveFile` を一括保護できる
- `mode` のゼロ値（0）はどちらのコンストラクタでも設定されない無効値となるため、`IsParentOnly()` が `false` を返し書き込み系が `ErrInvalidFilePath` で拒否する（加えて `path` も空文字列のため、空パスチェックも先に発火する）
- 変更規模：約 25 行（既存呼び出し元コードの修正なし）

**Cons:**
- 実行時チェックであり、コンパイル時には誤用を検知できない
- `ResolvedPath` に隠れた状態が増える（外見は同じ型なのに動作が異なる点）
- `mode` フィールドは unexported のため、テストでは振る舞いを通じた間接的な検証となる

---

## 採用した案: 案 C

### 意思決定の根拠

**保護範囲の非対称性:** `SafeReadFile` は `NewResolvedPath` と `NewResolvedPathParentOnly` の両方からの呼び出しが意味的に正当であるため、案 B を採用しても `SafeReadFile` は保護対象外となる。コードベース内で最も多く呼ばれるセキュリティ境界関数が型保護の恩恵を受けられないため、案 B の「コンパイル時保護」という主要な利点が限定的になる。

**変換ボイラープレートのトレードオフ:** 案 B では `fileanalysis.Store.Load`（`SafeReadFile` への引数）や各テストの `SafeReadFile` 呼び出し箇所（6〜8 か所）で `.AsResolvedPath()` 変換が必要となる。型を分けることで生じるこの追加コストが、コンパイル時保護の恩恵を上回ると判断した。

**YAGNI 原則との整合:** 現時点で `SafeWriteFile`・`SafeAtomicMoveFile` の本番呼び出し元は少数（`fileanalysis` の 2 か所のみ）であり、いずれも正しく `NewResolvedPathParentOnly` を使用している。誤用が実際に発生した場合、実行時エラーとして即座に検知できる。

**ゼロ値の安全性:** `resolveModeFull = iota + 1` とすることで `mode` のゼロ値（0）はどちらのコンストラクタでも設定されない無効値となる。`IsParentOnly()` はゼロ値に対して `false` を返すため、`ResolvedPath{}` をそのまま書き込み系に渡すと `ErrInvalidFilePath` で拒否される。また `ResolvedPath{}` は `path` も空文字列のため、空パスチェック（`absPath == ""` → `ErrInvalidFilePath`）が先に発火する場合もある。いずれにせよゼロ値は安全に拒否される。

### 旧仕様の上書き

`docs/tasks/0085_safefileio_resolved_path_api/01_requirements.md`（FR-5.1、FR-6.4）では、`SafeAtomicMoveFile` の `srcPath` について「移動元ファイルは存在するため `NewResolvedPath` を使用する」と規定していた。

この仕様はセキュリティ上誤りであった。`NewResolvedPath` でリーフのシンボリックリンクを事前に解決してしまうと、`SafeOpenFile` の `openat2(RESOLVE_NO_SYMLINKS)` チェックがシンボリックリンクのないパスを受け取ることになり、検知が機能しなくなる（本 ADR の「問題」セクション参照）。ファイルの存在有無は関係なく、`srcPath` においてもリーフのシンボリックリンク検知を維持する必要がある。

**本 ADR は旧仕様を上書きし、`SafeAtomicMoveFile` の `srcPath` にも `NewResolvedPathParentOnly` を使用することを定める。** 現在の実装（テストの `mustResolvedPath` が `NewResolvedPathParentOnly` を使用している点）はすでに正しい挙動を反映している。

### 実装範囲

1. `internal/common/filesystem.go`
   - `resolveMode` 型の追加
   - `ResolvedPath` へ `mode` フィールドを追加
   - `NewResolvedPath`: `resolveModeFull` を設定
   - `NewResolvedPathParentOnly`: `resolveModeParentOnly` を設定
    - `IsParentOnly() bool` メソッドを追加（`mode` の直接参照を外部パッケージへ露出しない）

2. `internal/safefileio/safe_file.go`
    - `safeAtomicMoveFileWithFS`: `srcPath.IsParentOnly()` および `dstPath.IsParentOnly()` のアサーション追加
    - `safeWriteFileCommon`: `filePath.IsParentOnly()` のアサーション追加

   - `SafeReadFile`・`SafeReadFileWithFS`: モードアサーション追加なし（両コンストラクタが正当なため）

3. テストの追加
    - `NewResolvedPath` で作成した `ResolvedPath` を `SafeWriteFile`・`SafeWriteFileOverwrite`・`SafeAtomicMoveFile` に渡した場合に `ErrInvalidFilePath` が返ることを検証

### 将来の方針

将来的に `SafeWriteFile` 系の本番呼び出し元が増加し、誤用リスクが高まった時点で案 B（別型導入）へ移行できる。ただし、移行時には案 C で追加した `mode` フィールドとすべてのアサーションを削除したうえで新しい型を導入する必要があるため、単純な追加変更にはならない。
