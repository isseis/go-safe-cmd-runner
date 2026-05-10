# Dead Code 削除計画 (Phase 03)

## 0. 背景

PR #695〜#700 にかけてのリファクタリング（test-runtime-reduction、ValidatorConfig 構造体化、modern Go idioms 適用）で取り残された dead code を体系的に洗い出し、安全に削除する計画。

過去の関連タスク: `docs/tasks/0121_deadcode_removal_followup`（Phase 1, 2, 3 完了済み）。

## 1. 調査手段と結果サマリ

### 1.1 実行コマンド

```bash
# プロダクションコードからの到達不能関数
~/go/bin/deadcode -filter github.com/isseis/go-safe-cmd-runner ./cmd/record ./cmd/runner ./cmd/verify

# テスト経路を含めた到達不能関数
~/go/bin/deadcode -tags test -test ./...

# 未使用識別子検査
golangci-lint run --no-config --default=none --enable=unused ./...
~/go/bin/staticcheck -checks=U1000 -tests=false ./...

# grep ベースの参照確認（外部参照の有無）
```

### 1.2 結果サマリ

- `staticcheck U1000` / `golangci-lint unused`: 0 件
  - → 未使用の **unexported** 識別子はなし
- `deadcode`（プロダクション経路）: 6 件（4 シンボル）
  - → プロダクション本体に到達不能だがテストから参照されている関数
- `deadcode -tags test -test`: 19 件
  - → 本当にどこからも呼ばれていない（テストからも呼ばれていない）テスト用ヘルパー
- `grep` で確認した外部参照ゼロの exported 型/変数/定数: 0 件

## 2. カテゴリ分け

### A. プロダクションファイルに置かれているがテスト専用 (4 件)

プロダクションコードの公開 API として残っているが、`cmd/*` から到達不能で、参照元はすべて `_test.go`。テストヘルパーへ移すか、テスト側を再構成する。

| # | ファイル:行 | シンボル | 唯一の利用元 |
|---|---|---|---|
| A-1 | [internal/fileanalysis/syscall_store.go:47](internal/fileanalysis/syscall_store.go#L47) | `NewSyscallAnalysisStore` + 非公開型 `syscallAnalysisStore` (実装メソッド `SaveSyscallAnalysis`, `LoadSyscallAnalysis`) | `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` のみ |
| A-2 | [internal/runner/base/security/validator.go:79](internal/runner/base/security/validator.go#L79) | `WithFileSystem(fs)` Option | `internal/runner/base/security/*_test.go` のみ |
| A-3 | [internal/security/elfanalyzer/standard_analyzer.go:76](internal/security/elfanalyzer/standard_analyzer.go#L76) | `NewStandardELFAnalyzerWithSyscallStore` | `internal/security/elfanalyzer/analyzer_test.go` のみ |
| A-4 | [internal/security/elfanalyzer/syscall_analyzer.go:177](internal/security/elfanalyzer/syscall_analyzer.go#L177) | `NewSyscallAnalyzerWithConfig` | `internal/security/elfanalyzer/syscall_analyzer_test.go` のみ |

注意:
- A-1 のうち `SyscallAnalysisStore` インターフェースは `standard_analyzer.go` から参照されているため残す。
- 削除ではなく `//go:build test` 付き `test_helpers.go` への移動が妥当。

### B. 完全に未参照のテストヘルパー (19 件 / 13 シンボル)

| # | ファイル:行 | シンボル | 状態 |
|---|---|---|---|
| B-1 | [internal/common/test_helpers.go](internal/common/test_helpers.go) | ファイル全体（`BoolPtr`, `NewUnlimitedTimeout`, `NewTimeout`, `NewUnsetOutputSizeLimit`, `NewUnlimitedOutputSizeLimit`, `Int32Ptr`, `Int64Ptr`, `NewUnlimitedOptionalValue`, `ErrInvalidTimeout`） | `internal/common/testutil/helpers.go` に移行済み。**全関数が外部参照ゼロ**（grep で `common.Foo(` をチェック済み） |
| B-2 | [internal/common/testutil/helpers.go:21-57](internal/common/testutil/helpers.go#L21-L57) | `ErrInvalidTimeout`, `NewUnlimitedTimeout`, `NewTimeout`, `NewUnlimitedOutputSizeLimit`, `BoolPtr` | `commontesting.Foo(` の grep でゼロ件。新しい testutil でも未使用 |
| B-3 | [internal/runner/bootstrap/test_helpers.go:17](internal/runner/bootstrap/test_helpers.go#L17) | `InitializeVerificationManagerForTest` | ファイル全体が外部参照ゼロ |
| B-4 | [internal/runner/testutil/helpers.go:17,28,39,45](internal/runner/testutil/helpers.go) | `SetupTestEnv`, `SetupSafeTestEnv`, `SetupFailedMockExecution`, `TestGroupExecutorConfig` | `internal/runner/test_helpers.go` 内の小文字版 (`setupTestEnv` など) が利用されており、`runnertesting.Foo` 形式の参照ゼロ |
| B-5 | [internal/runner/group_executor_test.go:63](internal/runner/group_executor_test.go#L63) | `WithEffectiveWorkDir` | テストファイル内のみで定義され外部参照ゼロ |
| B-6 | [internal/runner/base/executor/testutil/mocks.go:77,82,87](internal/runner/base/executor/testutil/mocks.go) | `NewMockFileSystem`, `NewMockFileSystemWithPaths`, `NewMockOutputWriter` | `executortesting.Foo` の grep でゼロ件 |
| B-7 | [internal/runner/base/executor/testutil/helpers.go:61,69,91](internal/runner/base/executor/testutil/helpers.go) | `WithExpandedCmd`, `WithExpandedArgs`, `WithEffectiveWorkDir` | `executortesting.Foo` の grep でゼロ件 |
| B-8 | [internal/runner/base/privilege/testutil/mocks.go:117](internal/runner/base/privilege/testutil/mocks.go#L117) | `NewMockPrivilegeManagerWithExecFn` | 外部参照ゼロ |
| B-9 | [internal/verification/test_helpers.go:22](internal/verification/test_helpers.go#L22) | `WithFS` | パッケージ内 `withFSInternal` への置換が完了しており未参照 |

## 3. 削除フェーズ計画

各フェーズで `make fmt` → `go build ./...` → `make test` → `make lint` を実行。

### Phase 1: テストヘルパーの完全削除（Category B）

責務が単一で副作用が小さいので最初に実施。

#### Phase 1a: `internal/common/test_helpers.go` 全削除 + testutil の未使用関数削除
- [x] [internal/common/test_helpers.go](internal/common/test_helpers.go) を削除
- [x] [internal/common/testutil/helpers.go](internal/common/testutil/helpers.go) から `ErrInvalidTimeout`, `NewUnlimitedTimeout`, `NewTimeout`, `NewUnlimitedOutputSizeLimit`, `BoolPtr` を削除（残す: `Int32Ptr`, `Int64Ptr`, `NewUnsetOutputSizeLimit`, `StringPtr`, `StringPtrOrNil`, `SafeTempDir`, `WriteExecutableFile`）
- [x] `goimports`/`gofumpt` で未使用 import (`fmt` など) を整理

#### Phase 1b: `internal/runner/bootstrap/test_helpers.go` 削除
- [x] [internal/runner/bootstrap/test_helpers.go](internal/runner/bootstrap/test_helpers.go) を削除（中身が `InitializeVerificationManagerForTest` 1 関数のみ）

#### Phase 1c: `internal/runner/testutil/helpers.go` の未使用 export を削除
- [x] `SetupTestEnv`, `SetupSafeTestEnv`, `SetupFailedMockExecution`, `TestGroupExecutorConfig` を削除
- [x] 必要に応じて未使用 import (`executor`, `runnertypes`, `security`, `verification`, `mock`) を整理

#### Phase 1d: testutil/mocks ・ helpers の未使用 export を削除
- [x] [internal/runner/base/executor/testutil/mocks.go](internal/runner/base/executor/testutil/mocks.go) から `NewMockFileSystem`, `NewMockFileSystemWithPaths`, `NewMockOutputWriter` 削除
- [x] [internal/runner/base/executor/testutil/helpers.go](internal/runner/base/executor/testutil/helpers.go) から `WithExpandedCmd`, `WithExpandedArgs`, `WithEffectiveWorkDir` 削除（コメントの "WithEffectiveWorkDir で上書き" など参照箇所も修正）
- [x] [internal/runner/base/privilege/testutil/mocks.go](internal/runner/base/privilege/testutil/mocks.go) から `NewMockPrivilegeManagerWithExecFn` 削除

#### Phase 1e: `internal/verification/test_helpers.go: WithFS` 削除
- [ ] [internal/verification/test_helpers.go:22-26](internal/verification/test_helpers.go#L22-L26) 削除（同ファイル内 `withFSInternal` が代替済み）

#### Phase 1f: `internal/runner/group_executor_test.go: WithEffectiveWorkDir` 削除
- [ ] [internal/runner/group_executor_test.go:62-66](internal/runner/group_executor_test.go#L62-L66) を削除

### Phase 2: プロダクションファイル中のテスト専用 API の整理（Category A）

選択肢2つ。原則 (b) を採る:
- (a) 削除して、既存テストを書き換えてプロダクション API のみで成立させる
- (b) `//go:build test` 付き `test_helpers.go` に切り出して、本番バイナリから除外する

#### Phase 2a: `WithFileSystem` を test_helpers に移動
- [ ] [internal/runner/base/security/validator.go:77-83](internal/runner/base/security/validator.go#L77-L83) を削除
- [ ] [internal/runner/base/security/test_helpers.go](internal/runner/base/security/test_helpers.go) に `WithFileSystem` を移動（`//go:build test`）

#### Phase 2b: `NewStandardELFAnalyzerWithSyscallStore` / `NewSyscallAnalyzerWithConfig` を test_helpers に移動
- [ ] [internal/security/elfanalyzer/standard_analyzer.go:76-87](internal/security/elfanalyzer/standard_analyzer.go#L76-L87) を `internal/security/elfanalyzer/test_helpers.go`（新規 / `//go:build test`）に移動
- [ ] [internal/security/elfanalyzer/syscall_analyzer.go:177](internal/security/elfanalyzer/syscall_analyzer.go#L177) も同ファイルに移動

#### Phase 2c: `NewSyscallAnalysisStore` / `syscallAnalysisStore` impl を test_helpers に移動
- [ ] [internal/fileanalysis/syscall_store.go:38-107](internal/fileanalysis/syscall_store.go) のうち `syscallAnalysisStore` 構造体・メソッド・コンストラクタを `internal/fileanalysis/test_helpers.go`（新規 / `//go:build test`）に移動
- [ ] インターフェース `SyscallAnalysisStore`（17-36行）と関連型は本体に残す
- [ ] テストファイル `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` の import は変更不要（同パッケージ内移動なので）

## 4. 検証手順

各フェーズ完了時に下記を実行:

```bash
make fmt
go build ./...
make test
make lint
~/go/bin/deadcode ./cmd/record ./cmd/runner ./cmd/verify        # production deadcode が減ること
~/go/bin/deadcode -tags test -test ./...                       # 該当項目が消えていること
```

## 5. リスクと留意点

- **A カテゴリの移動先について**: `//go:build test` ファイルへ移動した場合、`make build`（テストタグなし）でテストヘルパーを参照しないことを確認する。`go build ./...` だけでは検出できないため、`go build -tags=test ./...` も実行する。
- **A-1 の `SyscallAnalysisStore` インターフェース**: 本番経路で `standard_analyzer.go` から参照されているため、インターフェース自体は移動禁止。実装 `syscallAnalysisStore` のみ移動する。
- **インポート整合性**: 移動後に元ファイルの未使用 import が残らないように `goimports` で確認する。
- **`Int32Ptr`/`Int64Ptr` 維持**: `commontesting.Int32Ptr`/`Int64Ptr` は多数のテストから利用中なので削除しないこと。

## 6. PR 分割方針

- **PR 1**: Phase 1a〜1f まとめて（テストヘルパー削除のみ。本番に影響なし、レビューしやすい）
- **PR 2**: Phase 2a〜2c（プロダクションファイルから test_helpers への移動。慎重にレビュー）

各 PR は単一の責務で、リバート可能な大きさを保つ。

## 7. 進捗ログ

- 実施日:
- ブランチ:
- 実行コマンド:
- 結果サマリ:
- 課題:
- 次アクション:
