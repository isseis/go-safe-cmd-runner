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

### Step 1-2: `common` 内・全パッケージのコンパイル修正

`ResolvedPath` の struct 化と `GetHashFilePath` の `hashDir` 型変更は、コンパイルエラーで全影響箇所を特定できる。以下はあらかじめ判明している修正対象の一覧。

#### (a) `common` パッケージ本体

**対象ファイル**: `internal/common/hash_file_path_getter.go`

- `GetHashFilePath(hashDir string, filePath ResolvedPath)` → `GetHashFilePath(hashDir ResolvedPath, filePath ResolvedPath)` に変更

#### (b) `common` のテスト

**対象ファイル**: `internal/common/filesystem_test.go`

- `NewResolvedPath` のテストを新しいシグネチャ（Abs + EvalSymlinks 込み）に更新

#### (c) `GetHashFilePath` の実装（本体）

**対象ファイル**
- `internal/filevalidator/sha256_path_hash_getter.go`
- `internal/filevalidator/hybrid_hash_path_getter.go`

- `hashDir string` → `hashDir common.ResolvedPath` に変更
- 内部で `hashDir.String()` を使ってパス組み立て

#### (d) `GetHashFilePath` のモック実装（テスト）

**対象ファイル**
- `internal/fileanalysis/file_analysis_store_test.go`（`mockPathGetter.GetHashFilePath`）
- `internal/filevalidator/validator_test.go`（`collidingHashFilePathGetter.GetHashFilePath`）

- `hashDir string` → `hashDir common.ResolvedPath` に合わせてシグネチャ更新

#### (e) `GetHashFilePath` のテスト呼び出し側

**対象ファイル**
- `internal/filevalidator/sha256_path_hash_getter_test.go`
- `internal/filevalidator/hybrid_hash_path_getter_test.go`
- `internal/verification/manager_test.go`
- `internal/fileanalysis/network_symbol_store_test.go`

- `hashDir` を `string` リテラルで渡している箇所を `common.NewResolvedPath(...)` または `MustResolvedPath(...)` に変更

#### (f) `common.ResolvedPath(someString)` の直接型変換

**対象ファイル**（struct 化でコンパイルエラーになる箇所）
- `internal/filevalidator/validator_test.go`（7 箇所）
- `internal/fileanalysis/file_analysis_store_test.go`（多数）
- `internal/fileanalysis/syscall_store_test.go`
- `internal/fileanalysis/network_symbol_store_test.go`
- `internal/runner/security/command_analysis_test.go`

- すべて `common.MustResolvedPath(someString)`（テスト専用コンストラクタ）に置き換える

**確認コマンド**

```
make build
go test -tags test ./internal/common/... ./internal/fileanalysis/... ./internal/filevalidator/... ./internal/verification/... ./internal/runner/security/...
```

---

## Phase 2: `fileanalysis` + `filevalidator` — ストレージ経路の移行

### Step 2-1: `fileanalysis.Store` の `analysisDir` を `ResolvedPath` 化

**対象ファイル**
- `internal/fileanalysis/file_analysis_store.go`

**変更内容**

1. フィールドを `analysisDir string` → `analysisDir common.ResolvedPath` に変更
2. `NewStore(analysisDir string, ...)` シグネチャは **変えない**（要件 FR-3.4 / AC-13 に従い、raw string を受けて内部で正規化する）
3. `NewStore` 内でディレクトリ存在確認・`MkdirAll` を行った後、`common.NewResolvedPath(analysisDir)` を呼んでフィールドに格納する
   - ディレクトリが存在しない場合は `MkdirAll` で作成してから `NewResolvedPath` を呼ぶ
   - ディレクトリが symlink でも `EvalSymlinks` が正しく解決する
4. `GetHashFilePath` 呼び出し: `s.analysisDir` を `ResolvedPath` として渡す（Phase 1 で型変更済み）
5. ディレクトリ存在確認 (`os.Lstat`) の文字列操作は引き続き raw string を使用（`NewResolvedPath` 呼び出し前）

> **注意**: 計画書の旧 Step 2-3 では `filevalidator.New` から `NewResolvedPathForNew` を使って解決済みパスを `NewStore` に渡す案を記載していたが、これは要件と矛盾するため廃止した。`NewStore` が自前で正規化を完結させる。

---

### Step 2-2: `HashFilePathGetter` 実装群の更新

Step 1-2 (c)(d)(e) で実施済み。本 Step はスキップする。

---

### Step 2-3: `filevalidator.New` の hashDir 正規化を削除

**対象ファイル**
- `internal/filevalidator/validator.go`

**変更内容**

1. `New()` 内の `filepath.Abs(hashDir)` を削除する
   - `NewStore` が内部で `NewResolvedPath` を呼んで正規化するため、呼び出し元での事前正規化は冗長かつ二重適用になる
2. `NewStore(hashDir, hashFilePathGetter)` に raw string をそのまま渡す
   - `NewResolvedPathForNew` は使わない（hashDir は既存ディレクトリ前提であり、`NewResolvedPath` が適切）

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
[ ] Step 1-1: ResolvedPath struct 化、コンストラクタ整備
[ ] Step 1-2 (a): hash_file_path_getter.go の hashDir を ResolvedPath 化
[ ] Step 1-2 (b): filesystem_test.go の NewResolvedPath テスト更新
[ ] Step 1-2 (c): sha256 / hybrid GetHashFilePath 実装の hashDir 型変更
[ ] Step 1-2 (d): mockPathGetter / collidingHashFilePathGetter のモック更新
[ ] Step 1-2 (e): sha256/hybrid/manager/network_symbol テストの hashDir 渡し方更新
[ ] Step 1-2 (f): 全パッケージの ResolvedPath(string) 直接変換を MustResolvedPath に置換
[ ] Step 1-2 確認: make build && go test -tags test ./... が通る
[ ] Step 2-1: fileanalysis.Store の analysisDir を ResolvedPath 化（NewStore は string シグネチャ維持）
[ ] Step 2-2: スキップ（Step 1-2 で完了）
[ ] Step 2-3: filevalidator.New の filepath.Abs 削除（NewStore が内部で完結）
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
