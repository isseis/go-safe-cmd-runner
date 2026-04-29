# 実装計画書: filevalidator から runner 依存を除去する

## 概要

`internal/filevalidator` パッケージから `internal/runner/runnertypes` および
`internal/runner/privilege` への依存を除去する。

本番コードから呼ばれていない特権 API（`VerifyWithPrivileges`、`VerifyAndReadWithPrivileges`、
`VerifyFromHandle`、`PrivilegedFileValidator`）をデッドコードとして削除し、
`cmd/record` / `cmd/verify` が `internal/runner` 以下に依存しない構成にする。

## 進捗チェックリスト

### Phase 1: 前提確認

- [x] 現状の runner 依存を確認: `go list -deps ./cmd/record | grep internal/runner`
- [x] 現状の runner 依存を確認: `go list -deps ./cmd/verify | grep internal/runner`
- [x] ベースラインビルドの確認: `go build ./cmd/record ./cmd/verify ./cmd/runner`
- [x] ベースラインテストの確認: `make test`

### Phase 2: `internal/filevalidator/privileged_file.go` の削除

対象: `PrivilegedFileValidator` 型の全コードをパッケージ外へ影響なく削除する。

- [x] `internal/filevalidator/privileged_file.go` を削除
- [x] `internal/filevalidator/privileged_file_test.go` を削除

### Phase 3: `internal/filevalidator/validator.go` の変更

#### 3-1: FileValidator インターフェースから特権メソッドを除去

- [x] `FileValidator` インターフェースから `VerifyWithPrivileges` のシグネチャを削除
- [x] `FileValidator` インターフェースから `VerifyAndReadWithPrivileges` のシグネチャを削除

#### 3-2: Validator 構造体から privilegedFileValidator を除去

- [x] `Validator` 構造体から `privilegedFileValidator *PrivilegedFileValidator` フィールドを削除
- [x] `newValidator` 関数から `privilegedFileValidator: DefaultPrivilegedFileValidator()` 初期化を削除

#### 3-3: デッドコードとなったメソッドの削除

- [x] `Validator.VerifyWithPrivileges` メソッドを削除（本番コードからの呼び出しなし）
- [x] `Validator.VerifyAndReadWithPrivileges` メソッドを削除（本番コードからの呼び出しなし）
- [x] `Validator.VerifyFromHandle` メソッドを削除（`VerifyWithPrivileges` 削除後は呼び出しなし）

#### 3-4: デッドコードとなったエラー変数と import の削除

- [x] `ErrPrivilegeManagerNotAvailable` エラー変数を削除（削除したメソッド内のみで使用）
- [x] `ErrPrivilegedExecutionNotSupported` エラー変数を削除（削除したメソッド内のみで使用）
- [x] `import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"` を削除

### Phase 4: `internal/filevalidator/validator_test.go` の変更

削除したメソッドのテストを除去する。残るテストは削除後も有効であることを確認する。

- [x] `TestValidator_VerifyWithPrivileges` を削除
- [x] `TestValidator_VerifyWithPrivileges_NoPrivilegeManager` を削除
- [x] `TestValidator_VerifyWithPrivileges_MockPrivilegeManager` を削除
- [x] `TestValidator_VerifyAndReadWithPrivileges` を削除（全サブテスト含む）
- [x] `TestValidator_VerifyFromHandle` を削除
- [x] `TestValidator_VerifyFromHandle_Mismatch` を削除
- [ ] `TestValidator_VerifyAndRead_TOCTOUPrevention` から以下のサブテストを削除:
  - [x] `t.Run("VerifyAndReadWithPrivileges atomic operation", ...)` を削除
  - [-] `t.Run("verify read consistency", ...)` を削除（`VerifyAndReadWithPrivileges` を参照）
- [x] `privtesting` import を削除（削除後、参照がなくなるため）

### Phase 5: `internal/filevalidator/benchmark_test.go` の変更

- [x] `BenchmarkValidator_VerifyFromHandle` を削除
- [x] `BenchmarkOpenFileWithPrivileges` を削除
- [x] `openTestFile` ヘルパー関数を削除（`BenchmarkValidator_VerifyFromHandle` のみが使用）

### Phase 6: `internal/safefileio/safe_file.go` のコメント更新

`io.Seeker` の説明コメントが削除対象の `VerifyFromHandle` を参照しているため更新する。
`io.Seeker` は `openELFFile` でも引き続き使用されるためインターフェース定義は変更しない。

- [x] `io.Seeker` の行コメントを `VerifyFromHandle` への言及から更新する

### Phase 7: `internal/verification/testing/testify_mocks.go` の変更

- [x] `MockFileValidator` 構造体を完全に削除（本番・テストいずれからも参照なし）
- [-] `runnertypes` import を削除（`MockFileValidator` 削除後に参照なし）

### Phase 8: `internal/verification/manager_shebang_test.go` の変更

- [x] `mockFVForShebang` から `VerifyWithPrivileges` スタブ実装を削除
- [x] `mockFVForShebang` から `VerifyAndReadWithPrivileges` スタブ実装を削除
- [x] `runnertypes` import を削除（上記削除後に参照なし）

### Phase 9: 整形・静的解析

- [x] `make fmt` を実行してコードを整形
- [x] `make lint` を実行してリントエラーがないことを確認

### Phase 10: 受け入れ基準の検証

- [x] AC-3: `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功
- [x] AC-1: `go list -deps ./cmd/record | grep internal/runner` が 0 件
- [x] AC-2: `go list -deps ./cmd/verify | grep internal/runner` が 0 件
- [x] AC-4: `make test` が全件パス
- [x] AC-5: execute-only バイナリ検証をカバーする既存の `cmd/runner` / `internal/runner` / `internal/verification` テストを特定して実行する
- [-] AC-5: 既存テストで execute-only バイナリ検証を明示的に確認できない場合は、最小の回帰テストを追加してからそのテストを実行する

## 受け入れ基準との対応

| AC | 基準 | 対応フェーズ |
|----|------|-------------|
| AC-1 | `go list -deps ./cmd/record \| grep internal/runner` が 0 件 | Phase 2–3 |
| AC-2 | `go list -deps ./cmd/verify \| grep internal/runner` が 0 件 | Phase 2–3 |
| AC-3 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功 | Phase 2–8 全体 |
| AC-4 | `make test` が全件パス | Phase 2–8 全体 |
| AC-5 | `cmd/runner` の特権昇格が引き続き動作する | Phase 2–8 と Phase 10 |

## 注意事項

- Phase 2–8 の各フェーズはビルドが通る状態を保ちながら進める
- Phase 3 と Phase 4–8 は同時に変更する（インターフェース変更はコンパイルエラーを引き起こすため）
- `verifyAndReadContent` は `VerifyAndRead`（本番コード）からも呼ばれているため削除しない
- runner パッケージ自身の `internal/runner` への依存は本タスクのスコープ外
