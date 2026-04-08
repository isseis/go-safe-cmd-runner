# 実装計画書: ResolvedPath 型安全性強化

## 現状スナップショット

```
ResolvedPath = type alias (string)     ← 今回 struct 化する
NewResolvedPath: empty 判定のみ        ← Abs + EvalSymlinks を追加する
NewResolvedPathForNew: 未実装          ← 新規作成する
HashFilePathGetter.GetHashFilePath:
  引数 hashDir string                  ← ResolvedPath 化する
filevalidator.New: filepath.Abs のみ   ← EvalSymlinks 追加 or コンストラクタ委譲
filevalidator.validatePath:
  Abs + EvalSymlinks + regular 判定    ← コンストラクタ委譲に整理する
fileanalysis.Store.analysisDir: string ← ResolvedPath 化する
```

---

## Phase 1: `common` — 型境界の導入

### Step 1-1: `ResolvedPath` を struct 化し、コンストラクタを整備する

**対象ファイル**
- `internal/common/filesystem.go`

**変更内容**

1. `type ResolvedPath string` → `type ResolvedPath struct { path string }` に変更
2. `NewResolvedPath(path string) (ResolvedPath, error)` を整備
   - empty 判定 → `ErrEmptyPath`
   - `filepath.Abs` → 絶対化
   - `filepath.EvalSymlinks` → シンボリックリンク解決
   - 失敗はそのままエラー返却
3. `NewResolvedPathForNew(path string) (ResolvedPath, error)` を新規追加
   - empty 判定 → `ErrEmptyPath`
   - `filepath.Abs` → 絶対化
   - 親ディレクトリに `filepath.EvalSymlinks` → 解決
   - ファイル名を再結合して返却
4. `String() string` メソッドは維持
5. `MustResolvedPath(path string) ResolvedPath` を test ビルドタグ付きで追加（テスト専用）

**影響確認**
- `ResolvedPath` を `string` として直接使っている箇所はコンパイルエラーになるため、ステップ 1-2 で一括対処する

---

### Step 1-2: `common` 内・`common` 直接依存箇所のコンパイル修正

**対象ファイル**
- `internal/common/filesystem_test.go`
- `internal/common/hash_file_path_getter.go`

**変更内容**

1. `filesystem_test.go`: `NewResolvedPath` のテストを新しいシグネチャに更新し、Abs + EvalSymlinks の期待値を追加
2. `hash_file_path_getter.go`: `GetHashFilePath(hashDir string, ...)` → `GetHashFilePath(hashDir ResolvedPath, ...)` に変更

**確認コマンド**

```
make build
go test -tags test ./internal/common/...
```

---

## Phase 2: `fileanalysis` + `filevalidator` — ストレージ経路の移行

### Step 2-1: `fileanalysis.Store` の `analysisDir` を `ResolvedPath` 化

**対象ファイル**
- `internal/fileanalysis/file_analysis_store.go`

**変更内容**

1. `NewStore(analysisDir string, ...)` → `NewStore(analysisDir common.ResolvedPath, ...)`
2. フィールド `analysisDir string` → `analysisDir common.ResolvedPath`
3. `GetHashFilePath` 呼び出し: `s.analysisDir` を `ResolvedPath` として渡す（Phase 1 で型変更済み）
4. ディレクトリ存在確認 (`os.Lstat`) は `analysisDir.String()` を使用

---

### Step 2-2: `HashFilePathGetter` 実装群の更新

**対象ファイル**
- `internal/filevalidator/sha256_path_hash_getter.go`
- `internal/filevalidator/hybrid_hash_path_getter.go`

**変更内容**

1. `GetHashFilePath(hashDir ResolvedPath, filePath ResolvedPath) (string, error)` に合わせてシグネチャ更新
2. 内部で `hashDir.String()` / `filePath.String()` を使ってパス組み立て
3. `hashDir` / `filePath` が既に解決済みのため、内部での再解決は行わない

---

### Step 2-3: `filevalidator.New` の hashDir 正規化を整理

**対象ファイル**
- `internal/filevalidator/validator.go`

**変更内容**

1. `New()` 内の `filepath.Abs(hashDir)` を `common.NewResolvedPath(hashDir)` に置き換え
   - ただし `hashDir` は新規作成されることがあるため `NewResolvedPathForNew` を使う
2. 取得した `ResolvedPath` を `fileanalysis.NewStore` に渡す

---

### Step 2-4: `filevalidator.validatePath` の正規化を整理

**対象ファイル**
- `internal/filevalidator/validator.go`

**変更内容**

1. `validatePath` 内の `filepath.Abs` + `filepath.EvalSymlinks` を `common.NewResolvedPath` 呼び出し一本に置き換え
2. regular file 判定は `validatePath` のドメイン責務として維持
3. 戻り値 `common.ResolvedPath` はそのまま

**確認コマンド**

```
make build
go test -tags test ./internal/fileanalysis/... ./internal/filevalidator/...
```

---

## Phase 3: 再探索と停止判断

### Step 3-1: 残存箇所の検索

以下のコマンドで未移行箇所を列挙する。

```bash
grep -rn "filepath\.Abs\|filepath\.EvalSymlinks" \
  internal/filevalidator/ internal/fileanalysis/ internal/common/ \
  --include="*.go" | grep -v "_test.go"
```

### Step 3-2: 移行候補の分類

検索結果を以下の 3 区分に分類する。

| 区分 | 判断基準 |
|------|----------|
| **要移行** | security-critical path で `ResolvedPath` 導入で責務境界が明確になる |
| **妥当な境界** | 変更範囲が今回スコープ外（`safefileio` 等）で、次タスクへ切り出すべき |
| **対象外** | テスト専用 / ドキュメントコメント / 解決が不要な用途 |

### Step 3-3: 停止または継続の決定

- **停止する場合**: 「妥当な境界」「対象外」のみが残った場合に停止し、未移行箇所をコメントで明文化する
- **継続する場合**: 「要移行」が残っていれば Phase 2 の手順に準じて追加移行する

---

## 各 Step の完了チェック

```
[ ] Step 1-1: ResolvedPath struct 化、コンストラクタ整備、テスト更新
[ ] Step 1-2: hash_file_path_getter.go シグネチャ更新、コンパイル通過
[ ] Step 2-1: fileanalysis.Store analysisDir を ResolvedPath 化
[ ] Step 2-2: sha256 / hybrid HashFilePathGetter 実装の更新
[ ] Step 2-3: filevalidator.New の hashDir 正規化を NewResolvedPathForNew に委譲
[ ] Step 2-4: filevalidator.validatePath の Abs+EvalSymlinks を NewResolvedPath に委譲
[ ] Step 3-1: 残存箇所の検索実施
[ ] Step 3-2: 移行候補の分類
[ ] Step 3-3: 停止または継続の決定
```

---

## 注意事項

- 各 Step 後に `make build && go test -tags test ./... && make lint` を実行し、グリーンを確認してからコミットする
- `ResolvedPath` を `string` にキャストして直接渡している箇所はコンパイルエラーで検出できるので、型変更後に一括修正する
- `fileanalysis.NewStore` の呼び出し箇所は `filevalidator` 以外にも存在する可能性があるため、Step 2-1 前後でコンパイルエラーを利用して全箇所を把握する
