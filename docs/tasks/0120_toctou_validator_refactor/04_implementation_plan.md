# 実装計画書: TOCTOU チェックの実装重複を解消する（フェーズ 1）

## 変更対象の概要

| ファイル | 変更種別 | 変更内容 |
|---|---|---|
| `internal/runner/security/toctou_check.go` | 削除 | `NewValidatorForTOCTOU()` ごと削除 |
| `internal/runner/security/toctou_check_test.go` | 移動・更新 | テストを `internal/security/toctou_test.go` へ移動し、`isec.NewDirectoryPermChecker()` を使用するよう更新後に削除 |
| `internal/security/toctou_test.go` | 新規作成 | `toctou_check_test.go` のテストを移管 |
| `internal/runner/runner.go` | 修正 | `toctouValidator` フィールド型・`WithTOCTOUValidator` 引数型を変更、`isec` import 追加 |
| `internal/runner/group_executor.go` | 修正 | `toctouValidator` フィールド型を変更 |
| `internal/runner/group_executor_options.go` | 修正 | フィールド型・`WithGroupTOCTOUValidator` 引数型を変更、import 入れ替え |
| `cmd/runner/main.go` | 修正 | `runTOCTOUCheck` 戻り値型・`executeRunner` 引数型を変更、`isec.NewDirectoryPermChecker()` に切り替え |
| `internal/runner/group_executor_test.go` | 修正 | `security.NewValidatorForTOCTOU()` を `isec.NewDirectoryPermChecker()` に置き換え |

---

## タスクリスト

### Step 1: `internal/security/toctou_test.go` を新規作成する

`toctou_check_test.go` のテストは `internal/security` パッケージの関数（`CollectTOCTOUCheckDirs`・`RunTOCTOUPermissionCheck`）を検証しており、そのパッケージに置くべきである。また `RunTOCTOUPermissionCheck` のテストは旧来の `runner/security.Validator` ではなく `isec.NewDirectoryPermChecker()` を使用するよう更新する。

- [x] `internal/security/toctou_test.go` を新規作成し、以下のテストを実装する

  | 移植元 | 変更点 |
  |---|---|
  | `TestCollectTOCTOUCheckDirs` | 変更なし（`isec.CollectTOCTOUCheckDirs` を呼ぶだけ） |
  | `TestRunTOCTOUPermissionCheck_NoViolations` | `NewValidator(nil, WithGroupMembership(gm))` → `isec.NewDirectoryPermChecker()` |
  | `TestRunTOCTOUPermissionCheck_ViolationDetected` | 同上 |
  | `TestRunTOCTOUPermissionCheck_MultipleViolations` | 同上 |
  | `TestRunTOCTOUPermissionCheck_EmptyDirs` | 同上 |

  作成後、`make test` でテストが通ることを確認する。

---

### Step 2: `toctou_check.go` と `toctou_check_test.go` を削除する

- [x] `internal/runner/security/toctou_check.go` を削除する
- [x] `internal/runner/security/toctou_check_test.go` を削除する（Step 1 で内容を移管済み）

---

### Step 3: `internal/runner/group_executor_options.go` を修正する

- [x] `groupExecutorOptions.toctouValidator` の型を変更する

  ```go
  // before
  toctouValidator  *security.Validator

  // after
  toctouValidator  isec.DirectoryPermChecker
  ```

- [x] `WithGroupTOCTOUValidator` の引数型を変更する

  ```go
  // before
  func WithGroupTOCTOUValidator(v *security.Validator) GroupExecutorOption {

  // after
  func WithGroupTOCTOUValidator(v isec.DirectoryPermChecker) GroupExecutorOption {
  ```

- [x] import を入れ替える
  - `"github.com/isseis/go-safe-cmd-runner/internal/runner/security"` を削除
  - `isec "github.com/isseis/go-safe-cmd-runner/internal/security"` を追加

---

### Step 4: `internal/runner/group_executor.go` を修正する

- [x] `DefaultGroupExecutor.toctouValidator` の型を変更する

  ```go
  // before
  toctouValidator     *security.Validator

  // after
  toctouValidator     isec.DirectoryPermChecker
  ```

  `security` import はこのファイルで他用途（`security.ValidatorInterface`）にも使用されているため削除しない。
  `isec` import はすでに存在するため追加不要。

---

### Step 5: `internal/runner/runner.go` を修正する

- [x] `runnerOptions.toctouValidator` の型を変更する

  ```go
  // before
  toctouValidator         *security.Validator

  // after
  toctouValidator         isec.DirectoryPermChecker
  ```

- [x] `WithTOCTOUValidator` の引数型を変更する

  ```go
  // before
  func WithTOCTOUValidator(v *security.Validator) Option {

  // after
  func WithTOCTOUValidator(v isec.DirectoryPermChecker) Option {
  ```

- [x] `isec "github.com/isseis/go-safe-cmd-runner/internal/security"` import を追加する

  `security` import はこのファイルで他用途（`validator *security.Validator` 等）にも使用されているため削除しない。

---

### Step 6: `cmd/runner/main.go` を修正する

- [x] `runTOCTOUCheck` の戻り値型を変更する

  ```go
  // before
  func runTOCTOUCheck(...) (*security.Validator, error) {

  // after
  func runTOCTOUCheck(...) (isec.DirectoryPermChecker, error) {
  ```

- [x] `security.NewValidatorForTOCTOU()` を `isec.NewDirectoryPermChecker()` に置き換える

  ```go
  // before
  secValidator, secErr := security.NewValidatorForTOCTOU()
  if secErr != nil {
      // NewValidatorForTOCTOU only fails when a regex literal in DefaultConfig
      // is invalid — a programming error that cannot be recovered at runtime.
      panic(fmt.Sprintf("security validator initialisation failed (invalid built-in regex pattern): %v", secErr))
  }

  // after
  secValidator, secErr := isec.NewDirectoryPermChecker()
  if secErr != nil {
      // NewDirectoryPermChecker only fails when standalone checker setup fails,
      // which is not recoverable in this startup path.
      panic(fmt.Sprintf("security validator initialisation failed: %v", secErr))
  }
  ```

- [x] `executeRunner` の引数型を変更する

  ```go
  // before
  func executeRunner(..., secValidator *security.Validator) error {

  // after
  func executeRunner(..., secValidator isec.DirectoryPermChecker) error {
  ```

- [x] `"github.com/isseis/go-safe-cmd-runner/internal/runner/security"` import を削除する

  このファイルでは `security.NewValidatorForTOCTOU()` と型参照のみが `runner/security` を使用している。
  他に `runner/security` を参照している箇所がないことを確認してから削除する。

---

### Step 7: `internal/runner/group_executor_test.go` を修正する

- [x] `security.NewValidatorForTOCTOU()` を `isec.NewDirectoryPermChecker()` に置き換える（3 箇所）

  対象関数：
  - `TestRunGroupTOCTOUCheck_SecureDir`
  - `TestRunGroupTOCTOUCheck_ViolationReturnsError`
  - `TestRunGroupTOCTOUCheck_RelativePathsSkipped`

- [x] import を入れ替える
  - `"github.com/isseis/go-safe-cmd-runner/internal/runner/security"` を削除
  - `isec "github.com/isseis/go-safe-cmd-runner/internal/security"` を追加

  `runner/security` がこのテストファイルで他に使用されていないことを確認してから削除する。

---

### Step 8: ビルド・テスト・lint の確認

- [x] `go build ./cmd/record ./cmd/verify ./cmd/runner` が成功することを確認する（AC-4）
- [x] `make test` が全件パスすることを確認する（AC-5）
- [x] `make lint` がエラーなしで完了することを確認する（AC-6）

---

## 補足

### テストカバレッジ

#### `internal/security/toctou_test.go`（新規作成）

`CollectTOCTOUCheckDirs` と `RunTOCTOUPermissionCheck` のユニットテスト。
これらは `internal/security` パッケージが提供する関数であるため、同パッケージで検証するのが適切。

| テスト名 | 検証内容 |
|---|---|
| `TestCollectTOCTOUCheckDirs` | ディレクトリ収集・祖先ディレクトリの列挙・重複除去 |
| `TestRunTOCTOUPermissionCheck_NoViolations` | 安全なディレクトリで violations が空であること |
| `TestRunTOCTOUPermissionCheck_ViolationDetected` | world-writable ディレクトリが検出されること |
| `TestRunTOCTOUPermissionCheck_MultipleViolations` | 複数の violations が全件返されること |
| `TestRunTOCTOUPermissionCheck_EmptyDirs` | ディレクトリ一覧が空のとき violations が空であること |

#### `group_executor_test.go`（既存テストを更新）

`runGroupTOCTOUCheck` の統合的な動作を検証する。型の変更のみで、ロジックは変わらない。新規テストは不要。

| テスト名 | 検証内容 |
|---|---|
| `TestRunGroupTOCTOUCheck_NoValidator` | `toctouValidator` が nil のとき no-op になること |
| `TestRunGroupTOCTOUCheck_SecureDir` | 安全なディレクトリでエラーが発生しないこと |
| `TestRunGroupTOCTOUCheck_ViolationReturnsError` | world-writable ディレクトリで `ErrTOCTOUViolation` が返ること |
| `TestRunGroupTOCTOUCheck_RelativePathsSkipped` | 相対パスがスキップされること |

### `NewDirectoryPermChecker()` のエラー返却について

現在の実装（`dir_permissions_unix.go`）は常に `nil` を返す。
`error` 返却は将来の拡張（Windows 対応など）に備えた型シグネチャであるため、
`cmd/record/main.go` と同様に panic で対処する。
