# 実装計画書: filevalidator から runner 依存を除去する

## 概要

`internal/filevalidator` パッケージから `internal/runner/runnertypes` および
`internal/runner/privilege` への依存を除去する。

本番コードから呼ばれていない特権 API（`VerifyWithPrivileges`、`VerifyAndReadWithPrivileges`、
`VerifyFromHandle`、`PrivilegedFileValidator`）をデッドコードとして削除し、
`cmd/record` / `cmd/verify` が `internal/runner` 以下に依存しない構成にする。

## 進捗チェックリスト

### Phase 1: 前提確認

- [ ] 現状の runner 依存を確認: `go list -deps ./cmd/record | grep internal/runner`
- [ ] 現状の runner 依存を確認: `go list -deps ./cmd/verify | grep internal/runner`
- [ ] ベースラインビルドの確認: `go build ./cmd/record ./cmd/verify ./cmd/runner`
- [ ] ベースラインテストの確認: `make test`

### Phase 2: `internal/filevalidator/privileged_file.go` の削除

対象: `PrivilegedFileValidator` 型の全コードをパッケージ外へ影響なく削除する。

- [ ] `internal/filevalidator/privileged_file.go` を削除
- [ ] `internal/filevalidator/privileged_file_test.go` を削除

### Phase 3: `internal/filevalidator/validator.go` の変更

#### 3-1: FileValidator インターフェースから特権メソッドを除去

- [ ] `FileValidator` インターフェースから `VerifyWithPrivileges` のシグネチャを削除
- [ ] `FileValidator` インターフェースから `VerifyAndReadWithPrivileges` のシグネチャを削除

#### 3-2: Validator 構造体から privilegedFileValidator を除去

- [ ] `Validator` 構造体から `privilegedFileValidator *PrivilegedFileValidator` フィールドを削除
- [ ] `newValidator` 関数から `privilegedFileValidator: DefaultPrivilegedFileValidator()` 初期化を削除

#### 3-3: デッドコードとなったメソッドの削除

- [ ] `Validator.VerifyWithPrivileges` メソッドを削除（本番コードからの呼び出しなし）
- [ ] `Validator.VerifyAndReadWithPrivileges` メソッドを削除（本番コードからの呼び出しなし）
- [ ] `Validator.VerifyFromHandle` メソッドを削除（`VerifyWithPrivileges` 削除後は呼び出しなし）

#### 3-4: デッドコードとなったエラー変数と import の削除

- [ ] `ErrPrivilegeManagerNotAvailable` エラー変数を削除（削除したメソッド内のみで使用）
- [ ] `ErrPrivilegedExecutionNotSupported` エラー変数を削除（削除したメソッド内のみで使用）
- [ ] `import "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"` を削除

### Phase 4: `internal/filevalidator/validator_test.go` の変更

削除したメソッドのテストを除去する。残るテストは削除後も有効であることを確認する。

- [ ] `TestValidator_VerifyWithPrivileges` を削除
- [ ] `TestValidator_VerifyWithPrivileges_NoPrivilegeManager` を削除
- [ ] `TestValidator_VerifyWithPrivileges_MockPrivilegeManager` を削除
- [ ] `TestValidator_VerifyAndReadWithPrivileges` を削除（全サブテスト含む）
- [ ] `TestValidator_VerifyFromHandle` を削除
- [ ] `TestValidator_VerifyFromHandle_Mismatch` を削除
- [ ] `TestValidator_VerifyAndRead_TOCTOUPrevention` から以下のサブテストを削除:
  - [ ] `t.Run("VerifyAndReadWithPrivileges atomic operation", ...)` を削除
  - [ ] `t.Run("verify read consistency", ...)` を削除（`VerifyAndReadWithPrivileges` を参照）
- [ ] `privtesting` import を削除（削除後、参照がなくなるため）

### Phase 5: `internal/filevalidator/benchmark_test.go` の変更

- [ ] `BenchmarkValidator_VerifyFromHandle` を削除
- [ ] `BenchmarkOpenFileWithPrivileges` を削除
- [ ] `openTestFile` ヘルパー関数を削除（`BenchmarkValidator_VerifyFromHandle` のみが使用）

### Phase 6: `internal/safefileio/safe_file.go` のコメント更新

`io.Seeker` の説明コメントが削除済み `VerifyFromHandle` を参照しているため更新する。
`io.Seeker` は `openELFFile` でも引き続き使用されるためインターフェース定義は変更しない。

- [ ] `io.Seeker` の行コメントを `VerifyFromHandle` への言及から更新する

### Phase 7: `internal/verification/testing/testify_mocks.go` の変更

- [ ] `MockFileValidator` 構造体を完全に削除（本番・テストいずれからも参照なし）
- [ ] `runnertypes` import を削除（`MockFileValidator` 削除後に参照なし）

### Phase 8: `internal/verification/manager_shebang_test.go` の変更

- [ ] `mockFVForShebang` から `VerifyWithPrivileges` スタブ実装を削除
- [ ] `mockFVForShebang` から `VerifyAndReadWithPrivileges` スタブ実装を削除
- [ ] `runnertypes` import を削除（上記削除後に参照なし）

### Phase 9: 整形・静的解析

- [ ] `make fmt` を実行してコードを整形
- [ ] `make lint` を実行してリントエラーがないことを確認

### Phase 10: 受け入れ基準の検証

- [ ] AC-3: `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功
- [ ] AC-1: `go list -deps ./cmd/record | grep internal/runner` が 0 件
- [ ] AC-2: `go list -deps ./cmd/verify | grep internal/runner` が 0 件
- [ ] AC-4: `make test` が全件パス
- [ ] AC-5: `go list -deps ./cmd/runner | grep internal/runner` が引き続き存在する（runner 側の特権昇格は維持）

## 受け入れ基準との対応

| AC | 基準 | 対応フェーズ |
|----|------|-------------|
| AC-1 | `go list -deps ./cmd/record \| grep internal/runner` が 0 件 | Phase 2–3 |
| AC-2 | `go list -deps ./cmd/verify \| grep internal/runner` が 0 件 | Phase 2–3 |
| AC-3 | `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功 | Phase 2–8 全体 |
| AC-4 | `make test` が全件パス | Phase 2–8 全体 |
| AC-5 | `cmd/runner` の特権昇格が引き続き動作する | Phase 2–3（filevalidator への変更は runner 側の実装に影響しない） |

## 注意事項

- Phase 2–8 の各フェーズはビルドが通る状態を保ちながら進める
- Phase 3 と Phase 4–8 は同時に変更する（インターフェース変更はコンパイルエラーを引き起こすため）
- `verifyAndReadContent` は `VerifyAndRead`（本番コード）からも呼ばれているため削除しない
- runner パッケージ自身の `internal/runner` への依存は本タスクのスコープ外
