# safefileio ResolvedPath API 対応 実装計画

## 1. 目的

本計画は、ADR
`docs/dev/resolved-path-symlink-enforcement-adr.ja.md`
で採用された案 C（`ResolvedPath` への解決モード付与と実行時アサーション）を、
`docs/tasks/0085_safefileio_resolved_path_api/01_requirements.md` の受け入れ基準
AC-1 から AC-19 に沿って実装するための作業手順を定義する。

## 2. 実装方針

- 既存の公開 API シグネチャ方針（`ResolvedPath` 受け取り）を維持する
- `ResolvedPath` に内部状態（解決モード）を持たせる
- `safefileio` の書き込み系セキュリティ境界でのみ `ParentOnly` 制約を強制する
- `SafeReadFile` 系には制約を追加しない
- 誤用は `ErrInvalidFilePath` で明示的に失敗させる

## 3. 実装ステップ

### Phase 1: `common.ResolvedPath` の拡張

- [ ] `internal/common/filesystem.go` に `resolveMode` 型を追加
- [ ] `ResolvedPath` に `mode` フィールドを追加
- [ ] `NewResolvedPath` が `resolveModeFull` を設定するよう修正
- [ ] `NewResolvedPathParentOnly` が `resolveModeParentOnly` を設定するよう修正
- [ ] `IsParentOnly() bool` メソッドを追加
- [ ] 既存テストを更新し、必要に応じて新規テストを追加

**`resolveMode` のゼロ値設計:** `resolveModeParentOnly` を非ゼロ値にするため、`iota` の順序を以下のように定義する。

```go
type resolveMode int

const (
    resolveModeUnknown    resolveMode = iota // ゼロ値 (ResolvedPath{} のデフォルト)
    resolveModeFull                          // NewResolvedPath が設定
    resolveModeParentOnly                    // NewResolvedPathParentOnly が設定
)
```

これにより `ResolvedPath{}` のゼロ値は `IsParentOnly() == false` となり、`safeWriteFileCommon` の `IsParentOnly()` チェックで `ErrInvalidFilePath` を返す（AC-5, AC-6, AC-8 を満たす）。

対応受け入れ基準: AC-1, AC-2, AC-3, AC-4, AC-13

### Phase 2: `safefileio` 境界アサーションの追加

- [ ] `internal/safefileio/safe_file.go` の `safeWriteFileCommon` に
      `filePath.IsParentOnly()` チェックを追加
- [ ] `internal/safefileio/safe_file.go` の `safeAtomicMoveFileWithFS` に
      `srcPath.IsParentOnly()` / `dstPath.IsParentOnly()` チェックを追加

**チェック順序（両関数共通）:** 空パスチェックを `IsParentOnly()` チェックより先に行う。

```go
// 1. 空パスチェック（既存）
if absPath == "" {
    return fmt.Errorf("%w: empty path", ErrInvalidFilePath)
}
// 2. IsParentOnly チェック（新規追加）
if !filePath.IsParentOnly() {
    return fmt.Errorf("%w: must be created with NewResolvedPathParentOnly", ErrInvalidFilePath)
}
```

**制約（変更しない箇所）:**
- `SafeReadFile` / `SafeReadFileWithFS` にはモードアサーションを追加しない

対応受け入れ基準: AC-5, AC-6, AC-7, AC-8, AC-9, AC-17

### Phase 3: 呼び出し側とユースケースの整合

> **注:** 以下の 3 項目はすでに実装済み。Phase 1・2 完了後にビルドが通ることを確認するだけでよい。

- [x] `internal/fileanalysis/file_analysis_store.go` の `Load` / `Save` が
      `NewResolvedPathParentOnly` を使用していることを確認（実装済み）
- [x] `internal/filevalidator/validator.go` の `calculateHash` が
      `ResolvedPath` を直接受け取ることを確認（実装済み）
- [x] `internal/runner/config/loader.go` のテスト経路が
      `NewResolvedPath` 経由で `SafeReadFile` を呼ぶことを確認（実装済み）

対応受け入れ基準: AC-10, AC-11, AC-12

### Phase 4: 誤用検知テストの追加

追加先: `internal/safefileio/safe_file_test.go`

- [ ] `NewResolvedPath` 生成値を `SafeWriteFile` に渡したとき
      `ErrInvalidFilePath` になるテストを追加
- [ ] `NewResolvedPath` 生成値を `SafeWriteFileOverwrite` に渡したとき
      `ErrInvalidFilePath` になるテストを追加
- [ ] `NewResolvedPath` 生成値を `SafeAtomicMoveFile` の `srcPath` / `dstPath` に
      渡したとき `ErrInvalidFilePath` になるテストを追加
- [ ] `SafeReadFile` は `NewResolvedPath` / `NewResolvedPathParentOnly` の両方で
      正常動作することを確認

対応受け入れ基準: AC-14, AC-15, AC-16, AC-17

### Phase 5: 品質確認

- [ ] `make fmt` を実行
- [ ] `make test` を実行
- [ ] `make lint` を実行
- [ ] 失敗時は最小修正で再実行

対応受け入れ基準: AC-18, AC-19

## 4. リスクと対策

- リスク: `mode` 追加により `ResolvedPath` のゼロ値挙動が変わる
  - 対策: 空パスチェックを先に維持し、ゼロ値は従来どおり `ErrInvalidFilePath`
- リスク: 既存テストが新しい境界制約で失敗する
  - 対策: テストヘルパーを `NewResolvedPathParentOnly` 前提に統一
- リスク: `SafeReadFile` まで制約を入れて既存ユースケースを破壊する
  - 対策: `SafeReadFile` 系には制約を追加しないことをレビュー観点に固定

## 5. 完了条件

以下を満たしたら完了とする。

- AC-1 から AC-19 がすべて満たされる
- CI 相当の `make test` / `make lint` が成功する
- ADR と要件定義書の記述が実装と一致する
