# 実装計画書: 複数 group 失敗時のエラー行への group 帰属の表示

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-26 |
| Review date | 2026-09-26 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 要件・受け入れ基準プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テストヘルパ配置: [test_organization.md](../../dev/developer_guide/test_organization.md)
- セキュリティ設計（§16 出力サイズ制限）: [security-architecture.md](../../dev/architecture_design/security-architecture.md)

本書の用語は [02_architecture.md](02_architecture.md) §0 に従う（外側の context、実行全体の context、実行全体の中断、タイムアウト、出力ファイル）。先頭の窓と省略の印は同 §3.7 で定義する。

---

## 1. 実装の概要

### 1.1 目的

複数 group が失敗したとき、stderr の `Details:` と構造化ログの `error_message` の各行から、その行の group（と command）を判別できるようにする。あわせて、原因の文言の差し替え（`UserFriendlyError`）を廃止し、group の失敗を専用のエラー型で宣言する。コマンドのタイムアウトは group の失敗として扱い、`output_size_limit = 0` は無制限として扱う。負の値は設定の読み込みで拒否し、メモリ上に保持する出力は上限付き（先頭の窓）にし、一部だけ残った秘密鍵のブロックは redaction で隠す。

設計の根拠と個別の決定は [02_architecture.md](02_architecture.md) §1〜§6、変更する観察可能な挙動は同 §4.3 を参照する。実装は同 §8 の 8 段階をそのまま Phase 1〜8 とする。

### 1.2 実装方針

1. 原因の文言は差し替えない。報告に出す原因は常に `Error()` の文言とする（[02_architecture.md](02_architecture.md) §1.1）。
2. group の失敗は専用の型 `GroupError`・`GroupErrors` で宣言し、`executionErrorContext` は失敗の件数で外側の context を決める。`Unwrap() []error` の形では判定しない（§1.1、§3.1〜§3.3）。
3. 実行全体の中断は、エラーの中身ではなく実行全体の context の状態で判定する（§3.2）。
4. 設定の誤り（負の `output_size_limit`）は設定の読み込みで拒否する（§3.6）。
5. メモリ上に保持する出力は、出力ファイルの有無と `output_size_limit` によらず先頭の窓（64 KiB）に限る（§3.7）。
6. 秘密の形の知識は redaction の層に閉じる。executor は PEM の構文を知らないままとする（§3.7 の `internal/redaction` の変更）。
7. Go のソースコメント・識別子・文字列リテラルは英語で書く。
8. 各 Phase の完了時に `make fmt`（Go を変更した Phase）・`make test`・`make lint` を通す（AC-13）。

### 1.3 既存コード調査結果

2026-09-26 時点の commit `f72b5bbd` のコードを読んで確認した。`git log --oneline 8f7f7681..HEAD -- '*.go'` は空であり、[02_architecture.md](02_architecture.md) が基準とした `8f7f7681` の `file:line` は現 HEAD でも有効である。

#### 変更対象の現状

| ファイル | 現状 | 変更 |
|---|---|---|
| `internal/runner/runner.go:406-464` | `executeGroups` は `fmt.Errorf("failed to execute group %s: %w", ...)` で集め（`:436`、`:450`）、1 件ならそのまま、2 件以上なら `errors.Join` で返す（`:457-463`）。エラーの中身が `context.Canceled`・`context.DeadlineExceeded` を含むと直ちにそのエラーを返す（`:422-424`） | `newGroupError`／`newGroupErrors` で返す。中断の判定を実行全体の context の状態に変える |
| `cmd/runner/main.go:729-737` | `executionErrorContext` は `Unwrap() []error` の有無で multi-error を判定し（`:730`）、`*runner.CommandExecutionError` から名前を取る | `*runner.GroupErrors` の件数による判定に置き換える。判定順は §3.3 の 1→2→3 |
| `internal/logging/execution_error.go:20-58` | `UserFriendlyError`・`GetUserFriendlyMessage`・`formatCause` を定義。multi-error を子ごとに分ける | 3 つとも削除する |
| `internal/logging/pre_execution_error.go:93-98`・`:243-276` | `Detail()` と `HandleExecutionError` は原因を `formatCause` で描画する | 原因を `Err.Error()` にする。`handleErrorCommon` の字下げ（`:156-159`）と context の位置は変えない |
| `internal/runner/base/output/errors.go:74-130` | `CaptureError` は公開フィールドを持つ。`GetType`・`GetPath`・`UserMessage` を持つ | フィールドを非公開にし `limit` を加える。構築関数 2 つに限る。サイズ超過の `Error()` を変える。3 メソッドを削除する |
| `internal/runner/base/output/capture.go:37-75` | `WriteOutput` は上限 0 でも比較し、超過エラーをリテラルで作る（`:42-48`、`:61-70`） | `MaxSize == 0` は比較しない。構築関数で作る |
| `internal/runner/config/validation.go:191-222` | `ValidateTimeouts` が負の timeout を拒否する。`output_size_limit` の検証は無い | `ValidateOutputSizeLimits` を加える |
| `internal/runner/config/loader.go:62-94`・`:214-254` | `ValidateTimeouts` は `loadConfigInternal`（`:231`）で呼ばれる。テンプレートの合流はその後の `loadConfigWithIncludes`（`:85-91`） | `ValidateOutputSizeLimits` は合流後の設定全体に対して `loadConfigWithIncludes` で呼ぶ |
| `internal/runner/base/executor/output_pump.go:45-74`・`:222-310` | stdout は上限なし、stderr は引数で渡された上限。`boundedBuffer` は先頭と末尾を保持する | 両ストリームに同じ上限を使う。`boundedBuffer` は先頭だけを保持し、完全な行に切り詰めて省略の印を置く |
| `internal/runner/base/executor/command_lifecycle.go:435-441` | `outputWriter == nil` のときだけ stderr に上限を渡す | 分岐を削除し、両ストリームに同じ定数を渡す |
| `internal/runner/base/executor/executor.go:33-36` | 定数 `nilWriterStderrLimit = 32 << 10` | 定数を `retainedOutputLimit = 64 << 10` にする（改名は下の台帳） |
| `internal/redaction/value_detector.go:34`・`:143-167` | `pemPrivate` は `BEGIN` と `END` の両方があるブロックだけを隠す | 対応する `END` が無い `BEGIN ... PRIVATE KEY` の行から末尾までを隠すパターンを加える |
| `docs/user/toml_config/04_global_level.ja.md`（4.1 timeout、4.8 output_size_limit） | 0 が無制限であること、負の値の拒否、タイムアウト後の後続 group、メモリ保持の上限を書いていない | 記載を追記し、`/mktrans` で英語版に反映する |

#### 削除・改名するシンボルの全出現箇所

`rg` で本番（`internal/`・`cmd/` の `*.go`、`_test.go` を除く）とテストの両方を列挙した（commit `f72b5bbd`）。

| シンボル | 全出現箇所 | 対応 |
|---|---|---|
| `UserFriendlyError` | `internal/logging/execution_error.go:20-27`（定義）、`internal/runner/base/output/errors.go:114`（コメント）、`internal/logging/pre_execution_error_test.go:43`（テストのコメント） | 定義とコメントを削除。テストのコメントは「`UserMessage` を持つ型でも `Error()` を出す」ことを示す文言に変える |
| `GetUserFriendlyMessage` | `internal/logging/execution_error.go:29-36`（定義）、`:54`（`formatCause` 内の呼び出し）、`:41`（コメント） | `formatCause` ごと削除 |
| `UserMessage` | `internal/logging/execution_error.go:24-26`（インタフェースのメソッド）、`:33`（呼び出し）、`internal/runner/base/output/errors.go:113-130`（実装）、`internal/logging/pre_execution_error_test.go:43`・`:48`（`friendlyTestError`） | 本番側は削除。テストの `friendlyTestError` は残し、`UserMessage` があっても使われないことを確かめる |
| `formatCause` | `internal/logging/execution_error.go:38-58`（定義）、`internal/logging/pre_execution_error.go:97`（呼び出し）、`:244`（コメント）、`:259`（呼び出し） | 定義・呼び出しを削除し、コメントを `Error()` に合わせる |
| `GetType`・`GetPath` | `internal/runner/base/output/errors.go:103-111`（定義）のみ。テストを含めて呼び出しは無い | 削除する |
| `nilWriterStderrLimit` | `internal/runner/base/executor/executor.go:33-36`（定義とコメント）、`internal/runner/base/executor/command_lifecycle.go:439`、`internal/runner/base/executor/output_pump_test.go:343`・`:345`（コメントと使用） | 下の改名台帳に従う |
| `Unwrap() []error` の形による判定 | `cmd/runner/main.go:730-732`（削除）、`internal/runner/runner.go:455`（コメント）、`internal/logging/execution_error.go:46`（`formatCause` とともに削除） | 本番に残さない。ガードテストで固定する |

#### 改名台帳

| 旧名 | 新名 | 全出現箇所 | 対応 |
|---|---|---|---|
| `nilWriterStderrLimit` | `retainedOutputLimit` | 上の表の 3 ファイル 5 箇所（`executor.go` は定義とコメントで 2 箇所） | 定義・コメント・使用箇所を同時に置き換える。Phase 3 の完了時に旧名が残っていないことを `rg` で確認する |

`newOutputPump` のシグネチャ変更は改名ではなく引数の削除であり、呼び出し箇所はコンパイラが検出する（[02_architecture.md](02_architecture.md) §3.8 のテスト表）。

#### 構築経路の扱い（`CaptureError`・`GroupError`・`GroupErrors`）

- `CaptureError` は `internal/runner/base/output` のまま置く。本番の構築は `capture.go:43`・`:64` の 2 か所だけであり、他のパッケージからフィールドを読む本番コードもテストも無い（`rg` で確認）。フィールドを非公開にして構築関数 2 つに限れば、パッケージ外からの構築はコンパイラが拒否する。同一パッケージの本番ファイルでは複合リテラルでも構築できるため、`internal/runner/base/output/errors_guard_test.go` の `TestProductionCaptureErrorLiteralsUseConstructors` で次を固定する。複合リテラルの値形・ポインタ形・elided 形・位置指定形（キーなし）は `ProductionGoFilesInRepo` でリポジトリ全体から列挙する。`.typ`・`.path`・`.phase`・`.cause`・`.limit` へのセレクタ代入・インクリメントは、宣言パッケージの本番ファイル（`internal/runner/base/output` 直下）に限って列挙する。フィールド名だけの照合は、同じ名前のフィールドを持つ別の型への代入に誤反応するためである。構築形ごとの変異は §4.4 に置く。
- `GroupError`・`GroupErrors` は `internal/runner` に置く。構築は `executeGroups` が呼ぶ `newGroupError`・`newGroupErrors` と、`test_helpers.go`（`//go:build test`）のテスト用構築関数だけである。`cmd/runner` はアクセサで読むだけなので、パッケージ外からの構築はコンパイラが拒否する。同一パッケージの本番ファイル向けに `internal/runner/group_errors_guard_test.go` の `TestProductionGroupErrorLiteralsUseConstructors` を置く。複合リテラル（値形・ポインタ形・elided 形・位置指定形）は `ProductionGoFilesInRepo` でリポジトリ全体、`.errs`・`.group`・`.command`・`.err` へのセレクタ代入・インクリメントは `internal/runner` 直下の本番ファイルに限って列挙する（0176 の `TestProductionGroupStageErrorLiteralsUseConstructors` と同じ絞り方）。
- 型を葉パッケージへ移す案は採らない。`GroupError` は構築時に `*CommandExecutionError`・`*GroupStageError`（どちらも `internal/runner`）を読むため、分離すると依存が循環する。`CaptureError` は `Capture` と同じパッケージのエラーであり、分離する利点が無い。

#### 再利用する既存テスト・ヘルパ

| 対象 | 位置 | 使い方 |
|---|---|---|
| ログレコーダ | `internal/testutil`（`tu.NewLogRecorder`・`RequireRecord`・`AssertAttrs`・`FindRecords`） | 構造化ログの `error_message` と Slack レコードの観測 |
| Slack モック実行 | `cmd/runner/integration_test_helpers.go:174-357`（`slackRun`・`runMainWithSlackMock`・`captureStdoutStderr`・`jsonLogRecords`） | cmd/runner の統合テスト。`slackRun` は `stdout` と実行のログディレクトリを既に持つ |
| 実コマンドの実行 | `internal/runner/group_executor_timeout_test.go:36-`（`NewTestGroupExecutorWithConfig`・`resourcetestutil.NewDefaultResourceManager`） | 実際の timeout・出力サイズ超過を起こす統合テストの雛形 |
| グループ実行のモック | `internal/runner/runner_test.go:2607-2647`（`groupFailure`・`executeWithGroupFailures`） | `executeGroups` の配線テスト |
| 構築ガード | `internal/testutil/identitymutationguard`（`ProductionGoFilesInRepo`・`ReadProductionSource`・`ParseSource`） | 本番ファイルだけを走査する AST ガード |
| 設定ローダ | `internal/runner/config/test_helpers.go`（`NewLoaderForTest`）、`loader_includes_test.go` の include テスト | 負の `output_size_limit` の読み込みテスト。`LoadConfigForTest` は include を処理せず `loadConfigWithIncludes` を通らない点に注意する |

#### 更新が必要な既存テスト

[02_architecture.md](02_architecture.md) §3.8 の表を現 HEAD で確認した。すべて存在する。

- `internal/runner/runner_test.go:447-453`（`TestRunner_ExecuteAll_ComplexErrorScenarios` 内の multi-error 判定）
- `cmd/runner/main_test.go:748-790`（`TestExecutionErrorContext`）
- `internal/logging/pre_execution_error_test.go:43-48`・`:70-80`（`TestPreExecutionError_Detail`）、`:609-640`（`TestHandleExecutionError_CauseFormatting`）
- `internal/runner/base/output/errors_test.go:21`（`TestCaptureError`）、`:162`（`TestCaptureErrorInterface`）、`capture_test.go:205`（`.Type` の読み取り）
- `internal/runner/runner_test.go:2831-2842`（`TestRunner_CancellationSkipsStageNotification`。モックが実行全体の context を取り消す形に変える）
- `internal/runner/base/executor/output_pump_test.go:49`（`TestBoundedBuffer_KeepsPrefixAndSuffix`）、`:120`（`TestBoundedBuffer_WriteNeverFails`）、`:339-347`（`TestNewBoundedBuffer_RejectsNegativeLimit`）、`newOutputPump` の呼び出し（`:139`・`:194`・`:220`・`:269`・`:287`・`:302`・`:355`、`executor_lifecycle_test.go:362`）
- `internal/runner/base/executor/executor_test.go:248`（`TestExecute_NilOutputWriter_StderrPrefixSuffixBound`）
- `internal/redaction/redactor_test.go:4086`（`TestDefaultPatternSets_AreUnchanged`。追加するパターン 1 件を期待値に加える）
- `internal/redaction/value_detector_test.go:12`・`:93`・`:123-125`（`BEGIN` の側だけの行を加える。`PUBLIC KEY` を否定する行はそのまま）

テスト関数を削除する予定は無い（`TestBoundedBuffer_KeepsPrefixAndSuffix` と `TestExecute_NilOutputWriter_StderrPrefixSuffixBound` は改名・書き直しで残す）ため、AC-14 の `go tool cover -func` の比較は不要である。削除が生じた場合は AC-14 に従う。

#### 外部前提の確認

- `make test` は `-tags test` で全パッケージを実行する（`Makefile:480`・`:482`）。`internal/runner/test_helpers.go` と `cmd/runner` のテストは同じ `test` タグでビルドされるので、テスト用構築関数を `cmd/runner` のテストから使える。
- `make verify-docs-checks` は `scripts/verification/check_*.sh` を列挙して `sh` で実行し、非ゼロ終了を伝播する（`Makefile:790-800`）。新設の doc 検査スクリプトは自動で実行される。
- `identitymutationguard.ProductionGoFilesInRepo` は `_test.go` と `//go:build test` のファイルを除外する（`helpers.go:154-181`・`:204-244`）。
- `LoadConfigForTest` は `loadConfigInternal` を直接呼び、include を処理しない（`config/test_helpers.go:23-25`）。負の `output_size_limit` の読み込みテストは `LoadConfig` を通す。
- `runMainWithSlackMock` は `dryRun = false` を固定する（`integration_test_helpers.go:280`）。AC-27 の dry-run テストでは `slackRunSpec` に `dryRun` を加えて切り替える。

### 1.4 テストヘルパーの方針

- 新しいクロスパッケージのヘルパ・モックは作らない。既存の `tu`・`resourcetestutil`・`executortestutil`・`identitymutationguard` で足りる。
- パッケージ内のテスト用構築関数は `internal/runner/test_helpers.go`（既存・`//go:build test`）に `NewGroupErrorForTest`・`NewGroupErrorsForTest` を追加する。新しい `test_helpers*.go` は作らない。
- ガードテストは `_test.go` の本番パッケージ内テスト（`internal/runner`・`internal/logging`・`internal/runner/base/output`）として置き、`test_helpers.go` には置かない。
- stderr の捕捉が必要な統合テストは、テストファイル内の小さなローカルヘルパで `os.Pipe` を使う（`group_executor_test.go:1512` と同じ手法）。共通ヘルパファイルは追加しない。

---

## 2. 実装ステップ

各 Phase の完了時に `make fmt`（Go を変更した場合）・`make test`・`make lint` を通す（AC-13）。追加・変更したテストは、当該 Phase の「完了条件」に挙げた変異で失敗することを確認し、その旨をコミットメッセージに記す（AC-14、§4.4）。

### Phase 1: `GroupError`・`GroupErrors` の導入と外側の context の件数判定

**対象ファイル**: `internal/runner/group_errors.go`（新規）、`internal/runner/group_errors_test.go`（新規）、`internal/runner/group_errors_guard_test.go`（新規）、`internal/runner/runner.go`、`internal/runner/runner_test.go`、`internal/runner/test_helpers.go`、`cmd/runner/main.go`、`cmd/runner/main_test.go`、`cmd/runner/integration_attribution_test.go`（新規）

**作業内容**:

- [x] `group_errors.go` に [02_architecture.md](02_architecture.md) §3.1 の `GroupError`・`GroupErrors` を追加する。フィールドは非公開、アクセサは `GroupName()`・`CommandName()`、`GroupError.Error()` は原因の 2 行目以降を字下げし、`GroupErrors.Error()` は各要素を `"\n"` でつなぐ。`Unwrap()` は各 `GroupError`／原因を返す。
- [x] `newGroupError` は原因のチェーンから command 名を型で読み（`*CommandExecutionError` を優先、無ければ `*GroupStageError`）、空の group 名と nil の原因で panic する。`newGroupErrors` は空の一覧と nil の要素で panic し、受け取ったスライスをコピーして持つ。`Errors()` はコピーを返す。
- [x] `executeGroups`（`internal/runner/runner.go:406-464`）を、失敗を `newGroupError(group.Name, err)` で集め、1 件以上なら `newGroupErrors` で返し、0 件なら `nil` を返す形に変える。単一失敗の分岐（`:457-459`）と `errors.Join`（`:463`）を削除する。Phase 1 では中断の判定（`:422-424`）は変えない。
- [x] `executionErrorContext`（`cmd/runner/main.go:729-737`）を §3.3 の順に変える。1. `*runner.GroupErrors` を含めば件数で決める。2. `*runner.CommandExecutionError` を含めばその名前。3. それ以外は空。`Unwrap() []error` の判定（`:730-732`）を削除する。
- [x] `internal/runner/test_helpers.go` に `NewGroupErrorForTest(group, command string, err error) *GroupError` と `NewGroupErrorsForTest(errs ...*GroupError) *GroupErrors` を追加する。
- [x] `internal/runner/group_errors_test.go` に次を追加する。`TestGroupError_ErrorMatchesLegacyAssembly`（原因が 1 行のとき、同じ原因に変更前の組み立て方を適用した文言と一致すること。1 件・2 件）、`TestGroupError_IndentsContinuationLines`、`TestGroupErrors_UnwrapReachesEachCause`、`TestGroupError_ReadsCommandNameFromCause`、`TestGroupErrors_ConstructorsRejectInvalidInput`、`TestGroupErrors_ErrorsReturnsCopy`。
- [x] `internal/runner/runner_test.go` の `TestRunner_ExecuteAll_ComplexErrorScenarios` を、単一失敗が `*GroupErrors`（1 件）で group 名が `GroupSpec.Name` であることの検証に置き換える。`*CommandExecutionError` への到達は残す。`TestRunner_ExecuteGroupsBuildsGroupErrors` を追加し、0 件で `err == nil`、1 件・2 件で `*GroupErrors` を返すことを確かめる。
- [x] `internal/runner/group_errors_guard_test.go` に `TestProductionGroupErrorLiteralsUseConstructors`（複合リテラルの値形・ポインタ形・elided 形・位置指定形を構築関数の外で拒否し、`.errs`・`.group`・`.command`・`.err` への代入・インクリメントは `internal/runner` 直下の本番ファイルだけを走査して拒否する。検出器自身は `TestGroupErrorConstructionCheckRecognizesForms` で各形を固定する）を追加する。`Unwrap() []error` の形の判定を禁じるガードは、`formatCause`（`internal/logging/execution_error.go:46`）が同じ判定を持つため Phase 2 で追加する。
- [x] `cmd/runner/main_test.go` の `TestExecutionErrorContext` を、[02_architecture.md](02_architecture.md) §7.1 の行（`*GroupErrors` 1 件の 4 種、2 件、実行全体の中断、対象外のエラー）に置き換える。
- [x] `cmd/runner/integration_attribution_test.go`（`//go:build test`）に `TestIntegration_SingleGroupStageFailureGetsOuterContext` を追加する。group の展開が失敗する設定を `runMainWithSlackMock` で実行し、stderr の `Details:` の行と構造化ログの `error_message` が §3.3 の外側の context を含む文言であることを確かめる。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestGroupError_IndentsContinuationLines` が字下げを外すと失敗すること、`TestExecutionErrorContext` が判定 1・2 の順序を逆にすると失敗すること、`TestProductionGroupErrorLiteralsUseConstructors` が `runner.go` に各形の直接リテラルを置く／`.group` に代入すると失敗し、`internal/runner/base/executor` の既存の `.stage`・`.err` への代入には反応しないことを確認する。

### PR-1 作成ポイント: the typed group-failure error and count-based outer context

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0177): return typed group failures and derive the outer context from the count`

**レビュー観点**: `GroupError`・`GroupErrors` の不変条件が構築関数に閉じ、`Error()` の字下げ規則が §3.1 のとおりであること／`executeGroups` が 0 件で nil、1 件以上で `*GroupErrors` を返し、単一失敗の分岐と `errors.Join` が本番から消えていること／`executionErrorContext` の判定順が `*GroupErrors` → `*CommandExecutionError` → 空であること／構築ガードが値・ポインタ・elided 形とセレクタ代入を覆い、テストファイルを走査しないこと／原因の文言（`UserMessage` の差し替えを含む）がまだ変わっていないこと

**実装モデル要件**: frontier-recommended

**判定理由**: 新しいエラー型とその不変条件、返り値の型の変更、複数箇所をまたぐ呼び出し側の切替、構築の AST ガードを含む。段階的な raise/lower は無く panel-mode トリガーには該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: `UserFriendlyError` の削除（原因の差し替えの廃止）

**対象ファイル**: `internal/logging/execution_error.go`、`internal/logging/pre_execution_error.go`、`internal/logging/pre_execution_error_test.go`、`internal/logging/execution_error_guard_test.go`（新規）、`internal/runner/group_errors_guard_test.go`（Phase 1 で新規作成）、`internal/runner/base/output/errors.go`、`internal/runner/multi_group_error_integration_test.go`（新規）、`cmd/runner/integration_attribution_test.go`（Phase 1 で新規作成）

**作業内容**:

- [x] `internal/logging/execution_error.go` から `UserFriendlyError`・`GetUserFriendlyMessage`・`formatCause` を、その doc コメントごと削除する。
- [x] `PreExecutionError.Detail()`（`:93-98`）と `HandleExecutionError`（`:243-276`）の原因の描画を `e.Err.Error()` に変える。`handleErrorCommon` の複数行の字下げ（`:156-159`）と、外側の context を `Message` の直後・原因の前に置くという順序は変えない。
- [x] `CaptureError.UserMessage`（`internal/runner/base/output/errors.go:113-130`）を削除する（`GetType`・`GetPath` は Phase 6）。
- [x] `internal/logging/execution_error_guard_test.go` に `TestProductionCodeHasNoUserFriendlyError` を追加し、本番ファイルに `UserFriendlyError`・`GetUserFriendlyMessage`・`UserMessage`・`formatCause` が現れないことを固定する。
- [x] `internal/runner/group_errors_guard_test.go` に `TestProductionCodeDoesNotProbeMultiErrorShape`（本番ファイルに `Unwrap() []error` を型アサーションで調べる分岐が無いこと）を追加する。Phase 1 の `executionErrorContext` の置き換えと、本 Phase の `formatCause` の削除で、本番の該当箇所が無くなった後に置く。
- [x] `internal/logging/pre_execution_error_test.go` の `friendlyTestError` は残し、`UserMessage` を持っていても `Error()` が使われることを示すコメントに変える。`TestPreExecutionError_Detail` と `TestHandleExecutionError_CauseFormatting` の期待値を `Error()` の文言に反転する。
- [x] `internal/runner/multi_group_error_integration_test.go` を追加する。`TestRunner_MultiGroupFailureAttribution` は、group-1 が 0 以外の終了コード、group-2 が小さな `output_size_limit` と出力ファイルによるサイズ超過になる設定を実際の executor と resource manager を使い（`group_executor_timeout_test.go` の構成で `executeSingleCommand` により各エラーを作り、`executeGroups` には `MockGroupExecutor` で両方を返させて集約させる）、得たエラーを `logging.HandleExecutionError` に渡す。stderr はテストファイル内のローカルヘルパで `os.Pipe` により捕捉し、`error_message` は `tu.NewCallbackHandler` で記録して観測する（AC-01・AC-02・AC-04）。`TestHandleExecutionError_FilesystemCaptureErrorKeepsCause` は、閉じたファイルハンドルを持つ `output.Capture` の `WriteOutput` で実物の `ErrorTypeFileSystem` の `*CaptureError` を作り（`MaxSize` は書き込むデータより大きい正の値にする。Phase 5 より前は `MaxSize` が 0 だとサイズの比較が先に働き、`ErrorTypeSizeLimit` になる）、`Cause` の文言が `Details:` に出ることを確かめる（AC-03）。`TestRunner_SingleCommandFailureReportUnchanged` は、1 group・1 コマンドの失敗で `Details:`・`error_message` が変更前の文言のままであることを確かめる（AC-11）。
- [x] `cmd/runner/integration_attribution_test.go` に `TestIntegration_SingleCommandFailureKeepsOuterContext` を追加し、コマンドレベルの失敗（コマンドの `env_vars` の未定義変数。ハッシュ検証が有効なため記録の無い実行ファイルは実行できず、ここではコマンド実行前の失敗を使う。実コマンドの `*CommandExecutionError` の文言は `TestRunner_SingleCommandFailureReportUnchanged` が固定する）で `GroupName`・`CommandName` を含む外側の context が `Details:` と `error_message` に出ること、終了コードが 1 で `RUN_SUMMARY` 行が 1 行（失敗の status）であることを確かめる。あわせて `TestIntegration_MultiGroupFailureHasNoOuterContext` を追加し、2 group の失敗では外側の context が付かず、各行が `failed to execute group <group>: ` で始まることを確かめる（AC-05。終了コードの対応付けは AC-11・AC-12・AC-35・AC-36 の共通の証拠にも使う）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestPreExecutionError_Detail` が `Detail()` を `UserMessage` 優先に戻すと失敗すること、`TestHandleExecutionError_CauseFormatting` が同様に失敗すること、`TestProductionCodeHasNoUserFriendlyError` が本番のコメントに旧名を戻すと失敗すること、`TestProductionCodeDoesNotProbeMultiErrorShape` が `executionErrorContext` または `formatCause` の旧判定を戻すと失敗することを確認する。

### PR-2 作成ポイント: remove the cause-substitution interface

**対象ステップ**: Phase 2

**推奨タイトル**: `refactor(0177): report the raw cause and remove UserFriendlyError`

**レビュー観点**: `Detail()` と `HandleExecutionError` が `Error()` の文言だけを使い、`handleErrorCommon` の字下げと context の位置が変わっていないこと／`CaptureError.UserMessage` が本番に残っていないこと／複数 group の失敗で各行が `failed to execute group <group>: ` で始まり、外側の context が付かないこと（AC-01・AC-02・AC-05）／`ErrorTypeFileSystem` の `Cause` が報告に出ること（AC-03）／単一 `*CommandExecutionError` 失敗の文言が変わっていないこと（AC-11）

**実装モデル要件**: frontier-recommended

**判定理由**: 報告文言の中核を変え、実コマンドを動かす統合テストとガードテストを伴う。`UserMessage` が消えることでテストの失敗可能性を保つ工夫（`friendlyTestError` の維持）が必要であり、実装モデルの能力が結果に影響しうる。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: 秘密鍵のブロックの検出と、メモリ上の出力の保持の上限

**対象ファイル**: `internal/redaction/value_detector.go`、`internal/redaction/value_detector_test.go`、`internal/redaction/redactor_test.go`、`internal/runner/base/executor/output_pump.go`、`internal/runner/base/executor/command_lifecycle.go`、`internal/runner/base/executor/executor.go`、`internal/runner/base/executor/output_pump_test.go`、`internal/runner/base/executor/executor_test.go`、`internal/runner/base/executor/executor_lifecycle_test.go`、`internal/runner/output_retention_integration_test.go`（新規）

**作業内容**:

- [ ] `internal/redaction/value_detector.go` に、対応する `END` の行が無い `BEGIN ... PRIVATE KEY` の行からテキストの末尾までを隠すパターン `pemPrivateUnterminated` を加える。`Mask` では `pemPrivate` を適用した後に適用し、placeholder は既定のものを使う。パターンの doc コメントに、先頭の窓で `END` が失われたブロックを隠す目的と、隠しすぎる側に倒すことを書く。
- [ ] `internal/redaction/value_detector_test.go` の `TestValueDetector_Mask_PositiveCases` に `BEGIN` の側だけの行（先行する行・本文の行・省略の印の並びを含む）を加え、`TestValueDetector_Mask_NegativeCases` の `PUBLIC KEY` と完全なブロックの否定・肯定はそのまま通ることを確かめる。追加する行は、まず同じ入力を変更前の `pemPrivate` だけに通して一致しないことを確かめてから、検査に加える（CLAUDE.md「A layered path needs inputs only one layer can handle」）。
- [ ] `internal/redaction/redactor_test.go:4086` の `TestDefaultPatternSets_AreUnchanged` の期待値に、追加するパターン 1 件を加える。`pemPrivate` の文字列は変えない。`TestRedactText_ValueBasedDetection` はそのまま通ることを確かめる。
- [ ] `internal/runner/base/executor/executor.go` の定数を `nilWriterStderrLimit`（32 KiB）から `retainedOutputLimit`（64 KiB）に改名し、コメントを先頭の窓の説明に書き換える（改名台帳）。
- [ ] `newOutputPump` を、上限を引数に取らず両ストリームに `retainedOutputLimit` を使う形に変える。`boundedBuffer` を先頭だけの保持に変え、`Bytes()` は、省略が無ければ保持したバイトをそのまま返す。省略があれば、先頭の窓を最後の改行まで（改行を含む）に切り詰め、その後ろに省略の印を置く（切り詰めと印の規則は §3.7 のとおり）。先頭の窓に改行が無ければ省略の印だけを返す。
- [ ] `output_pump.go` の印の文字列は現状と同じ `\n... omitting N bytes ...\n` とし、`N` は出力全体のバイト数から、残った完全な行のバイト数を引いたものとする（`omissionMarkerCapacity` をそのまま使える形に保つ）。旧い `os/exec` の prefix/suffix 規則を説明するコメントを削除・書き換える。上限 0 を無制限とする `boundedBuffer` の分岐と `TestBoundedBuffer_UnlimitedBehavesLikeBytesBuffer` は残す（本番の呼び出し元は `retainedOutputLimit` だけであり、本番に無制限の経路は無い）。
- [ ] `command_lifecycle.go`（`:435-441`）の `outputWriter == nil` による上限の分岐を削除する。
- [ ] `output_pump_test.go` の `TestBoundedBuffer_KeepsPrefixAndSuffix` を `TestBoundedBuffer_KeepsCompletePrefixLines` に改名し、先頭の窓に改行がある行・無い行、短い先頭行の後に長い行が続く行を持つ表に書き直す。`TestBoundedBuffer_WriteNeverFails`・`TestNewBoundedBuffer_RejectsNegativeLimit`（許容する上限の一覧）と `newOutputPump` の呼び出しを新しい形に合わせる。
- [ ] `executor_test.go` の `TestExecute_NilOutputWriter_StderrPrefixSuffixBound` を `TestExecute_NilOutputWriter_BoundedStderrIsPrefixOnly` に改名し、改行を含まない巨大 stderr の期待値を省略の印だけにする。`TestExecute_NilOutputWriter_StdoutBoundedOnSuccess`（正常終了でも `Result.Stdout` が上限付きで省略の印を含むこと）と `TestExecute_OutputWriterReceivesAllBytes`（出力ファイルがあるとき、全バイトが `OutputWriter` に渡ること）を追加する。
- [ ] `output_pump_test.go` に `TestBoundedBuffer_LineBoundaryCutLeavesNoPartialSecret` を追加する。値の形の検出の各パターン（`pemPrivate`・`githubToken`・`bearerToken`・`gcpSAKey` と、1 行の `urlCred`）に一致する秘密の行を並べたコーパスを作る。コーパスを `boundedBuffer` 自身に書いて `Bytes()` で取り出し（行の途中では切らない）、`RedactText` に通して、秘密の本体が redaction されずに残らないことを確かめる。コーパス全体を切らずに `RedactText` に通すと各秘密が隠れることも確かめ、コーパスが検出に一致する入力であることを示す。切る規則を完全な行への切り詰めから外す（行の途中で切る）と失敗することも確認する（§4.4）。
- [ ] `internal/runner/output_retention_integration_test.go`（`package runner`）に `TestOutputRetention_SlackAndDebugFieldsFromBoundedOutput` を追加する。実際の group executor での実行を 2 つに分ける。(a) 64 KiB を超える stdout を書くコマンドを終了コード 0 で実行する（デバッグログと `command_group_summary` の記録は正常終了でも出る）。`tu.NewCallbackHandler` で記録した `Command execution result` の記録から、デバッグログの `stdout` を確かめる。`command_group_summary` の記録は `Runner.logGroupExecutionSummary`（`NewRunner` が `WithGroupNotificationFunc` で配線する。`runner.go:373`）が出すので、group executor を `WithGroupNotificationFunc(r.logGroupExecutionSummary)` で組み立て、その記録から出力の欄を確かめる。(b) 0 以外の終了コードで終わり 64 KiB を超える stdout を書くコマンドを別に実行し、executor の `Result`（stdout は失敗時も保持され、監査の記録は失敗の経路 `internal/runner/base/executor/executor.go:290` で書かれる）を `audit.NewAuditLoggerWithCustom(logger).LogUserGroupExecution`（`internal/runner/base/audit/test_helpers.go`。`NewAuditLogger` は `slog.Default()` に書くため記録を受け取れない）に渡して得た記録からは、`user_group_command_failure` の `Output` の欄を確かめる（`LogUserGroupExecution` は終了コードが 0 以外のときだけ stdout・stderr と通知属性を記録するため、0 以外の終了コードの実行を使う）。Slack の欄は、`logging.NewSlackHandler` を `Synchronous: true`・`httptest.NewTLSServer` の `HTTPClient` で組み立て、`Handle` に記録を渡してペイロードを観測する（AC-29。`user_group_command_failure` と `command_group_summary` は別の記録なので、それぞれの記録を渡す）。ハンドラは生成時に `t.Cleanup` で閉じる。先頭の窓に改行が無い出力・切り詰め位置より短い完全な行がある出力・残った行が切り詰め位置より長い出力の 3 種を表に持つ。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`pemPrivateUnterminated` の適用を外すと `TestValueDetector_Mask_PositiveCases`（`BEGIN` の側だけの行）と `TestBoundedBuffer_LineBoundaryCutLeavesNoPartialSecret` が失敗すること、完全な行への切り詰めを外すと `TestBoundedBuffer_KeepsCompletePrefixLines` と `TestBoundedBuffer_LineBoundaryCutLeavesNoPartialSecret` が失敗すること、省略の印のバイト数から切り詰め分を除くと `TestBoundedBuffer_KeepsCompletePrefixLines` が失敗することを確認する。

### PR-3 作成ポイント: bound retained output to complete prefix lines and mask a lone PEM BEGIN

**対象ステップ**: Phase 3

**推奨タイトル**: `fix(0177): bound retained output and mask an unterminated PEM block`

**レビュー観点**: redaction の追加規則が `pemPrivate` の後だけに適用され、完全なブロックと `PUBLIC KEY` の扱いが変わっていないこと／`boundedBuffer` が末尾を保持せず、先頭の窓を完全な行に切り詰め、省略の印のバイト数が保持しなかった全バイトを数えること／出力ファイルへの書き込みが全バイトのまま変わっていないこと／上限が出力ファイルの有無によらず 1 つの定数になっていること／新しい doc コメントが英語で、旧い prefix/suffix の説明が残っていないこと／`nilWriterStderrLimit` の旧名が残っていないこと／性質テストが行の途中で切れた断片を検出すること

**実装モデル要件**: frontier-required

**判定理由**: セキュリティに直結する redaction の規則追加と、メモリ保持の仕組みの置き換えを同時に行い、境目をまたぐ値の漏れを性質テストで抑える必要がある。Conditional checks に挙げた「行の途中で切れた断片」の扱いと、redaction を適用する順序が正しさを左右する。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 負の `output_size_limit` の読み込み時の拒否

**対象ファイル**: `internal/runner/config/errors.go`、`internal/runner/config/validation.go`、`internal/runner/config/loader.go`、`internal/runner/config/validation_test.go`、`internal/runner/config/loader_includes_test.go`、`cmd/runner/integration_test_helpers.go`、`cmd/runner/integration_attribution_test.go`

**作業内容**:

- [ ] `config/errors.go` に `ErrNegativeOutputSizeLimit` を追加する。
- [ ] `config/validation.go` に `ValidateOutputSizeLimits(cfg *runnertypes.ConfigSpec) error` を追加する。`ValidateTimeouts`（`:191-222`）と同じ形で、グローバル・テンプレート・コマンドの負の値をすべて集め、値と設定箇所（テンプレート名、group 名・コマンド名と添字）を含む 1 つのエラーで返す。0・正の値・未指定は受け入れる。
- [ ] `config/loader.go` の `loadConfigWithIncludes` で、`mergeTemplates` によるテンプレートの合流（`:85-91`）の後に `ValidateOutputSizeLimits(cfg)` を呼ぶ。include で取り込んだテンプレートの負の値もここで拒否される。`loadConfigInternal` には追加しない（取り込んだテンプレートがまだ合流していないため）。
- [ ] `validation_test.go` に `TestValidateOutputSizeLimits` を追加し、グローバル・テンプレート・コマンドの負の値、0、正の値、未指定を表で確認する。
- [ ] `loader_includes_test.go` に `TestLoadConfig_NegativeOutputSizeLimitValidation` を追加し、`LoadConfig` を通して主の設定ファイルの負の値と、`includes` で取り込んだテンプレートのファイルの負の値を拒否することを確かめる。`LoadConfigForTest` は include を処理しないため使わない。
- [ ] `cmd/runner/integration_test_helpers.go` の `slackRunSpec` に `dryRun` フィールドを加え、`runMainWithSlackMock` の `dryRun = false` 固定（`:280`）をこの値に変える（既定は false とし、既存の呼び出し元の挙動を変えない）。`cmd/runner/integration_attribution_test.go` に `TestIntegration_NegativeOutputSizeLimitRejectedInDryRun` を追加し、負の `output_size_limit` を含む設定の dry-run が終了コード 1 で、stderr と `error_message` に値と設定箇所を含む読み込みエラー（`output_size_limit` の拒否）を出すことを確かめる（AC-27。dry-run ではもともと group が実行されないため、「どの group も実行されない」は判定に使わない）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`ValidateOutputSizeLimits` の呼び出しを外すと `TestLoadConfig_NegativeOutputSizeLimitValidation` の主設定ファイルの行が失敗すること、テンプレートの検査を外すと include の行が失敗することを確認する。

### PR-4 作成ポイント: reject a negative output_size_limit at load time

**対象ステップ**: Phase 4

**推奨タイトル**: `feat(0177): reject negative output_size_limit while loading the configuration`

**レビュー観点**: `ValidateOutputSizeLimits` がグローバル・テンプレート・コマンドの負の値をすべて集め、値と設定箇所を含むこと／呼び出しがテンプレートの合流後で、include の値も検査されること／0・正の値・未指定が拒否されないこと／dry-run でも読み込みで拒否されること（AC-27）／`slackRunSpec` の `dryRun` 追加が既存の呼び出し元の挙動を変えないこと

**実装モデル要件**: standard

**判定理由**: `ValidateTimeouts` と同じ形の検証関数と、その呼び出し位置の追加であり、検査内容は要件と設計（§3.6）に固定されている。テストは表の追加が中心で、未確定の実装判断が無い。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: `output_size_limit = 0` を無制限として扱う

**対象ファイル**: `internal/runner/base/output/capture.go`、`internal/runner/base/output/capture_test.go`、`internal/runner/output_capture_integration_test.go`

**作業内容**:

- [ ] `Capture.WriteOutput`（`capture.go:37-75`）を、`MaxSize == 0` のときサイズを比べずに書き込む形に変える（[02_architecture.md](02_architecture.md) §3.5）。
- [ ] `capture_test.go` の `TestCapture_WriteOutput` に、`MaxSize` が 0 のとき大きなデータでもエラーを返さない行を加える（AC-23）。
- [ ] `internal/runner/output_capture_integration_test.go` に `TestRunner_ZeroOutputSizeLimitIntegration` を追加する。`output_size_limit = 0` と出力ファイルを指定したコマンドに 64 KiB を超える出力を書かせ、出力サイズ超過で失敗せず、出力ファイルに全出力が書かれ、結果の stdout が上限付きで省略の印を含むことを確かめる（AC-24）。Phase 3 の後に行う。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`MaxSize == 0` の判定を外すと `TestCapture_WriteOutput` の 0 の行と `TestRunner_ZeroOutputSizeLimitIntegration` が失敗することを確認する。

### PR-5 作成ポイント: treat output_size_limit = 0 as unlimited

**対象ステップ**: Phase 5

**推奨タイトル**: `fix(0177): treat output_size_limit = 0 as unlimited`

**レビュー観点**: `WriteOutput` が上限 0 で比較せず、正の上限では変更前どおり超過を拒否すること／`common.OutputSizeLimit` の 0 の定義、および `NormalResourceManager` が上限 0 を渡す挙動に一致すること／実コマンドで出力ファイルに全出力が残り、結果の stdout が上限付きであること（AC-24）／出力ファイルが無い場合の挙動を変えていないこと

**実装モデル要件**: standard

**判定理由**: 1 つの条件分岐の追加と、実コマンドの統合テスト 1 件である。設計は §3.5 に固定され、他コンポーネントへの波及はない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: `CaptureError` の整理

**対象ファイル**: `internal/runner/base/output/errors.go`、`internal/runner/base/output/capture.go`、`internal/runner/base/output/errors_test.go`、`internal/runner/base/output/capture_test.go`、`internal/runner/base/output/errors_guard_test.go`（新規）

**作業内容**:

- [ ] `CaptureError` のフィールドを非公開（`typ`・`path`・`phase`・`cause`・`limit`）にし、メソッドの受け手をポインタにそろえる。`Error()` は宣言された `typ` で分岐し、`ErrorTypeSizeLimit` のときだけ段階・パス・上限値を含む文言を返して原因の文言を付けない。それ以外の種類は変更前と同じ文言を返す（AC-16・AC-18）。
- [ ] 構築関数 `newSizeLimitError(path string, limit int64) *CaptureError`（種類 `ErrorTypeSizeLimit`・段階 `PhaseExecution`・原因 `ErrOutputSizeExceeded` を固定し、`limit` が 0 以下で panic する）と `newFileSystemError(path string, cause error) *CaptureError`（種類 `ErrorTypeFileSystem`・段階 `PhaseExecution`）を追加する。`Unwrap()` は引き続き原因を返し、`errors.Is(err, ErrOutputSizeExceeded)` が成り立つ。
- [ ] `capture.go` のサイズ超過を `newSizeLimitError(c.OutputPath, c.MaxSize)`、書き込み失敗を `newFileSystemError(c.OutputPath, err)` に変える。
- [ ] `GetType`・`GetPath` を削除する（`UserMessage` は Phase 2 で削除済み）。パッケージ外にアクセサを加えない。
- [ ] `errors_test.go` の `TestCaptureError`・`TestCaptureErrorInterface` を構築関数で作り直す。サイズ超過は新しい文言と上限値、書き込み失敗は `newFileSystemError` を使う。構築関数の無い種類（`ErrorTypePathValidation`・`ErrorTypePermission`・`ErrorTypeCleanup`）の行は、このテストファイル内のテスト専用の構築関数で作り、文言が変わらないことを確かめる（AC-18）。`TestNewSizeLimitErrorPanicsOnNonPositiveLimit` を追加する（AC-25）。
- [ ] `capture_test.go:205` の `.Type` の読み取りを非公開フィールドの読み取りに更新する（同じパッケージなので読める）。
- [ ] `errors_guard_test.go` に `TestProductionCaptureErrorLiteralsUseConstructors`（複合リテラルの値形・ポインタ形・elided 形・位置指定形を `ProductionGoFilesInRepo` で拒否し、`.typ`・`.path`・`.phase`・`.cause`・`.limit` の代入・インクリメントは `internal/runner/base/output` 直下の本番ファイルに限って拒否する。検出器自身は `TestCaptureErrorConstructionCheckRecognizesForms` で各形を固定する）と `TestCaptureErrorHasNoLegacyAccessors`（本番ファイルに `CaptureError` の `GetType`・`GetPath` の宣言が無いこと）を追加する。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`newSizeLimitError` の panic を外すと `TestNewSizeLimitErrorPanicsOnNonPositiveLimit` が失敗すること、`errors.go` に直接リテラルを置く／`.limit` に代入すると `TestProductionCaptureErrorLiteralsUseConstructors` が失敗すること、`GetType` を戻すと `TestCaptureErrorHasNoLegacyAccessors` が失敗することを確認する。

### PR-6 作成ポイント: make CaptureError constructible only through its constructors

**対象ステップ**: Phase 6

**推奨タイトル**: `refactor(0177): encapsulate CaptureError and add the size-limit value to its message`

**レビュー観点**: サイズ超過の文言が段階・パス・上限値を含み「size limit exceeded」に当たる語句を 1 回だけ含むこと（AC-16）／`errors.Is(err, ErrOutputSizeExceeded)` と `errors.AsType[*output.CaptureError]` が成り立つこと（AC-17）／他の種類の文言が変更前と同じであること（AC-18）／構築関数が上限 0 以下を拒否し、パッケージ外からの構築がコンパイラで拒否されること（AC-25）／`GetType`・`GetPath` が本番にもテストにも残っていないこと

**実装モデル要件**: frontier-recommended

**判定理由**: 公開フィールドから非公開フィールドへの変更、文言の変更、構築関数 2 つの追加と、同一パッケージの構築経路を列挙する AST ガードを伴う。テストの書き直しが複数ファイルに及ぶ。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 7: コマンドのタイムアウトと実行全体の中断の扱い

**対象ファイル**: `internal/runner/runner.go`、`internal/runner/runner_test.go`、`internal/runner/group_errors_guard_test.go`、`internal/runner/multi_group_error_integration_test.go`、`cmd/runner/main_test.go`

**作業内容**:

- [ ] `executeGroups` を [02_architecture.md](02_architecture.md) §3.2 と §6.3 の順に変える。1. group ファイル検証の失敗を先に通知する。対象は §3.2 の 1 が定義するとおり、`*verification.Error` を含み、かつ `*GroupStageError` を含まないか、含んでもその段階がファイル検証であるエラー（現状の分岐で検証の経路 `:441-448` に入るエラーと同じ）である。2. 実行全体の context が取り消されていれば、検証の経路では `ctx.Err()` だけを、それ以外は `errors.Join(ctx.Err(), err)` を返して残りの group を実行しない。3. 1 で通知した検証失敗は集めずに次の group へ進む。4. 実行前段の `*GroupStageError` は通知して `GroupError` として集める。5. それ以外（コマンドのタイムアウトを含む）は `GroupError` として集める。エラーの中身が `context.Canceled`・`context.DeadlineExceeded` を含むかによる分岐（`:422-424`）を削除する。
- [ ] `group_errors_guard_test.go` に `TestExecuteGroupsDoesNotBranchOnCancellationCause` を追加し、`executeGroups` の関数本体が `context.Canceled`・`context.DeadlineExceeded` を参照しないことを固定する。
- [ ] `runner_test.go` の `TestRunner_CancellationSkipsStageNotification` を、モックが実行全体の context を取り消してから段階エラーを返す形に変える。同じ構成で、取り消さない場合は通知されることを別の行で確かめる。
- [ ] `runner_test.go` に `TestRunner_ExecuteGroupsCollectsCommandTimeout`（実行全体の context を取り消さず group-1 が `context.DeadlineExceeded` を含む `*CommandExecutionError` を返すと group-2 が実行され、戻り値が `*GroupErrors` で `errors.Is(err, context.DeadlineExceeded)` が成り立つ。AC-19）、`TestRunner_ExecuteGroupsCollectsFailureThenTimeout`（group-1 の 0 以外の終了コードと group-2 のタイムアウトが両方要素になる。AC-20 と AC-22 の前提）、`TestRunner_ExecuteGroupsStopsOnRunContextCancellation`（AC-21）、`TestRunner_ExecuteGroupsReturnsCancellationOnLastGroupVerificationFailure`（AC-35）、`TestRunner_ExecuteGroupsCanceledChildFailureIncludesContextCanceled`（AC-36）を追加する。実行全体の context を取り消すモックはテストファイル内の小さな `GroupExecutor` 実装で用意する。
- [ ] `runner_test.go` に `TestRunner_CommandTimeoutNotifiesSubsequentGroups` を追加する。`WithGroupNotificationFunc` で通知を記録し、group-1 のコマンドを自身の `timeout` でタイムアウトさせ、group-2 を正常に実行させて、両方の `command_group_summary` の通知が記録されることを確かめる（AC-32）。
- [ ] `multi_group_error_integration_test.go` に `TestRunner_TimeoutAttributionIntegration` を追加する。`TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded` と同じ仕組みで得たタイムアウトのエラーが `*CommandExecutionError` と `context.DeadlineExceeded` の両方を含むことを確かめたうえで、group-1 の 0 以外の終了コードと group-2 のタイムアウトを並べた `executeGroups` の結果を `logging.HandleExecutionError` に渡し、`Details:` に両 group の行が出ることを確かめる（AC-20）。
- [ ] `cmd/runner/main_test.go` の `TestExecutionErrorContext` に、タイムアウト 1 件の行と、先の失敗＋タイムアウトの 2 件の行を加える（AC-22）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。中断の判定をエラーの中身に戻すと `TestRunner_ExecuteGroupsCollectsCommandTimeout` と `TestExecuteGroupsDoesNotBranchOnCancellationCause` が失敗すること、検証失敗の通知を中断判定の後ろへ移すと `TestRunner_ExecuteGroupsReturnsCancellationOnLastGroupVerificationFailure` が失敗すること、`errors.Join(ctx.Err(), err)` を `err` だけにすると `TestRunner_ExecuteGroupsCanceledChildFailureIncludesContextCanceled` が失敗することを確認する。

### PR-7 作成ポイント: classify a command timeout as a group failure and stop by context state

**対象ステップ**: Phase 7

**推奨タイトル**: `fix(0177): collect command timeouts as group failures and stop on context state`

**レビュー観点**: `ExecuteGroup` のエラーの処理順が §3.2 と §6.3 のとおりで、検証失敗の通知と中断の判定と段階の振り分けが正しいこと／エラーの中身による中断判定が本番に残っていないこと（AC-21）／コマンドのタイムアウトが `GroupError` として集まり、後続 group が実行されること（AC-19・AC-20）／最後の group の検証失敗・子プロセスの先終了と中断が重なったときも nil でない `context.Canceled` を含むエラーを返すこと（AC-35・AC-36）／タイムアウト後に後続 group の通知が行われること（AC-32）／`TestRunner_CommandTimeoutBehavior` は `t.Skip` のままにすること

**実装モデル要件**: frontier-required

**判定理由**: 実行の制御フローの順序を変え、中断の意味を「エラーの中身」から「実行全体の context」へ変える。既存テストの前提（モックのキャンセル方法）も変わり、実 executor のタイムアウトと通知を組み合わせた検証が必要である。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 8: 利用者向け文書の更新と検証スクリプト

**対象ファイル**: `docs/user/toml_config/04_global_level.ja.md`、`docs/user/toml_config/04_global_level.md`、`docs/translation_glossary.md`、`scripts/verification/check_output_limit_timeout_docs.sh`（新規）

**作業内容**:

- [ ] `04_global_level.ja.md` の「4.8 output_size_limit」に次を追記する。第 1 に、0 が無制限を表すこと、負の値は設定の読み込みで拒否されること（AC-26）。第 2 に、メモリ上に保持する出力には一定の上限（先頭の完全な行と省略の印）があり、上限を超えた出力の全体が必要なら出力ファイルを指定すること。第 3 に、出力ファイルに出力の全体が残るのは、コマンドが成功して出力が `output_size_limit` に収まったとき（または上限が 0 のとき）であること（AC-31）。0 が無制限である記述の典拠は Phase 5 の `TestCapture_WriteOutput` と `TestRunner_ZeroOutputSizeLimitIntegration`、負の値の拒否の典拠は Phase 4 の `TestValidateOutputSizeLimits`、保持の上限の典拠は Phase 3 の `TestBoundedBuffer_KeepsCompletePrefixLines` とする。
- [ ] 同書の「4.1 timeout」の「動作の詳細」に、コマンドのタイムアウト後も後続の group が実行されること、既知の制限として、タイムアウトしたコマンドのプロセス（孫プロセスを含む）が残りうることを追記する（AC-30）。典拠は Phase 7 の `TestRunner_CommandTimeoutNotifiesSubsequentGroups` と [02_architecture.md](02_architecture.md) §4.4 とする。
- [ ] 日本語版をコミットした後、`/mktrans` で `04_global_level.md` に反映し、新しく使った用語を `docs/translation_glossary.md` に登録する。
- [ ] `scripts/verification/check_output_limit_timeout_docs.sh` を、既存の `check_pre_execution_notification_docs.sh` と同じ POSIX sh の形で追加する。日英の両方について、(a) 0 が無制限であることを述べる文、(b) 負の値が読み込みで拒否されることを述べる文、(c) タイムアウト後も後続の group が実行されることを述べる文、(d) タイムアウトしたプロセスが残りうることを述べる文、(e) メモリ上に保持する出力の上限と出力ファイルの指定を述べる文、のアンカー文字列が存在することを検査する。`make verify-docs-checks` が `check_*.sh` を列挙して実行するため、追加の登録は不要である。
- [ ] `make verify-docs-checks`・`make test`・`make lint` が通ることを確認する。
- [ ] `make verify-docs` を実行し、日英の見出し構造の比較が問題を報告しないことを確認する（横断検索チェックリスト）。

**完了条件**: `make verify-docs-checks`・`make test`・`make lint` が通る。日英どちらかから (a)〜(e) のアンカーを外すと `check_output_limit_timeout_docs.sh` が非ゼロで終了することを確認する。

### PR-8 作成ポイント: document the timeout and output-size behavior

**対象ステップ**: Phase 8

**推奨タイトル**: `docs(0177): document timeout continuation and the output-size semantics`

**レビュー観点**: 日英の記述が Phase 3〜7 の実装とテストが観測した挙動に一致すること（0 が無制限、負の値の拒否、タイムアウト後の後続 group、残りうるプロセス、メモリ上の保持の上限と出力ファイル）／`check_output_limit_timeout_docs.sh` のアンカーが日英の実際の文と一致し、`make verify-docs-checks` から実行されること／`/mktrans` の用語が `translation_glossary.md` に登録されていること／日英の `04_global_level` の見出し構造が一致すること

**実装モデル要件**: standard

**判定理由**: 文書の追記と翻訳、アンカーを値として固定する検査スクリプトの追加である。記述の典拠は Phase 3〜7 のテストに固定されており、未確定の実装判断が無い。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 型による宣言 | Phase 1 | `GroupErrors` の単体・配線・ガードテストが green。単一の段階失敗に外側の context が付く |
| M2: 原因の差し替えの廃止 | Phase 2 | 複数 group の失敗の各行が帰属を示し、単一失敗の文言が変わらない |
| M3: 出力の保持と秘密の検出 | Phase 3 | redaction の性質テストと保持の上限のテストが green。旧い prefix/suffix の記述が残らない |
| M4: 設定と上限の修正 | Phase 4、Phase 5 | 負の値の拒否と 0 の無制限が単体・統合・dry-run のテストで green |
| M5: `CaptureError` の整理 | Phase 6 | 構築ガードと文言のテストが green。本番の構築が構築関数 2 つに限られる |
| M6: タイムアウトと中断 | Phase 7 | タイムアウトの収集・後続 group の実行・中断の判定のテストが green |
| M7: 文書 | Phase 8 | 日英の更新、`make verify-docs-checks` green、`make verify-docs` が問題を報告しない |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `GroupError`・`GroupErrors`、`executeGroups` の返り値、`executionErrorContext`、テスト用構築関数、構築の AST ガード | frontier-recommended |
| PR-2 | Phase 2 | `UserFriendlyError` の削除、`Detail()` と `HandleExecutionError` の原因、`Unwrap() []error` の判定を禁じるガード、複数 group の統合テスト | frontier-recommended |
| PR-3 | Phase 3 | `pemPrivateUnterminated`、`boundedBuffer` の先頭だけの保持、定数の改名、性質テスト、Slack・デバッグログの統合テスト | frontier-required |
| PR-4 | Phase 4 | `ValidateOutputSizeLimits` と `ErrNegativeOutputSizeLimit`、合流後の呼び出し、dry-run の統合テスト | standard |
| PR-5 | Phase 5 | `WriteOutput` の上限 0、0 の統合テスト | standard |
| PR-6 | Phase 6 | `CaptureError` の非公開化と構築関数、サイズ超過の文言、構築ガード | frontier-recommended |
| PR-7 | Phase 7 | `executeGroups` の処理順と中断判定、タイムアウトの収集、通知・統合テスト | frontier-required |
| PR-8 | Phase 8 | 日英の利用者向け文書、用語集、doc 検査スクリプト | standard |

### 3.3 順序の根拠

[02_architecture.md](02_architecture.md) §8 の優先順位 1〜8 をそのまま保つ。依存関係は次のとおりである。Phase 2 の統合テストは Phase 1 の `*GroupErrors` を前提にする。Phase 3 の redaction を保持の上限より先に入れないと、`BEGIN` の側だけが残る抜けを作る。Phase 5 の「0 を無制限」は Phase 3 の保持の上限の後に行い、Phase 6 の構築関数は Phase 4・5 の後に行う（上限が常に正であることが前提のため）。Phase 7 は Phase 1 の後であればよく、Phase 8 は Phase 3〜7 が観測した挙動を記述の典拠にする。§8 の「Phase 2 と 3〜6 は入れ替えてよい」はそのまま適用可能である。

---

## 4. テスト戦略

### 4.1 単体テスト

[02_architecture.md](02_architecture.md) §7.1 の各項目を、次のテストで実装する。検証内容・入力・期待値は同 §7.1 を参照する（本書では重複しない）。

- `internal/runner/group_errors_test.go`: `TestGroupError_ErrorMatchesLegacyAssembly`・`TestGroupError_IndentsContinuationLines`・`TestGroupErrors_UnwrapReachesEachCause`・`TestGroupError_ReadsCommandNameFromCause`・`TestGroupErrors_ConstructorsRejectInvalidInput`・`TestGroupErrors_ErrorsReturnsCopy`（Phase 1、AC-07〜AC-10・AC-33）。
- `internal/runner/runner_test.go`: `TestRunner_ExecuteGroupsBuildsGroupErrors`（Phase 1、AC-07）、`TestRunner_ExecuteGroupsCollectsCommandTimeout`・`TestRunner_ExecuteGroupsCollectsFailureThenTimeout`・`TestRunner_ExecuteGroupsStopsOnRunContextCancellation`・`TestRunner_ExecuteGroupsReturnsCancellationOnLastGroupVerificationFailure`・`TestRunner_ExecuteGroupsCanceledChildFailureIncludesContextCanceled`（Phase 7、AC-19〜AC-22・AC-35・AC-36）、`TestRunner_CommandTimeoutNotifiesSubsequentGroups`（Phase 7、AC-32）。
- `cmd/runner/main_test.go::TestExecutionErrorContext`（Phase 1・7、AC-05・AC-08・AC-11・AC-15・AC-22）。
- `internal/logging/pre_execution_error_test.go`（Phase 2、AC-06）。
- `internal/runner/base/output`（Phase 5・6、AC-16〜AC-18・AC-23・AC-25）。
- `internal/runner/config`（Phase 4、AC-27）。
- `internal/runner/base/executor`（Phase 3、AC-28・AC-29）。
- `internal/redaction`（Phase 3、AC-34）。

### 4.2 統合テスト

- `internal/runner/multi_group_error_integration_test.go`: 複数 group の帰属（AC-01・AC-02・AC-04）、ファイルシステム起因の `CaptureError` の `Cause`（AC-03）、単一 `*CommandExecutionError` 失敗の文言の維持（AC-11）、タイムアウトの帰属（AC-20）。
- `internal/runner/output_capture_integration_test.go::TestRunner_ZeroOutputSizeLimitIntegration`（AC-24）。
- `internal/runner/output_retention_integration_test.go::TestOutputRetention_SlackAndDebugFieldsFromBoundedOutput`（AC-29）。
- `cmd/runner/integration_attribution_test.go`: 単一の段階失敗の外側の context（AC-15）、単一のコマンド失敗の外側の context（AC-11）、負の `output_size_limit` の dry-run（AC-27）。

### 4.3 層の切り分け

- AC-01・AC-02 は `executeGroups` のテスト（`TestRunner_MultiGroupFailureAttribution`）で 2 group の実行から観測する。外側の context の有無を決めるのは `cmd/runner` の `executionErrorContext` であり、AC-05 は `cmd/runner` の `TestExecutionErrorContext` だけで確認する（`internal/runner` の統合テストは `ExecutionError` を自ら組み立てるため、この判定を検証できない）。
- AC-03 は、閉じたファイルハンドルを持つ `output.Capture` の `WriteOutput` で作った実物の `*CaptureError` を原因に含むチェーンを `HandleExecutionError` に渡して確かめる。`internal/logging` のテストは `internal/runner/base/output` を import できない（`logging` から `output` への依存は循環する）ため、`internal/runner` の統合テストに置く。
- AC-06 は本番コードのガードテストと、`friendlyTestError` を使う `logging` の単体テストの 2 層で確認する。`UserMessage` を持つ型が無くなると「`Error()` をそのまま使う」ことを他の表示と区別できる入力が無くなるため、テスト型は残す。
- AC-28・AC-29 は `boundedBuffer` の単体テスト（保持量と印）と、executor・Slack・デバッグログの統合テスト（利用者に見える欄）の 2 層で確認する。
- AC-34 は `ValueDetector.Mask` の単体テストと、`boundedBuffer` の切り詰めと `RedactText` を組み合わせた性質テストの 2 層で確認する。

### 4.4 実装時に行うテスト失敗確認（AC-14）

各 Phase の「完了条件」に挙げた変異を実施し、当該テストが失敗することを確認して、コミットメッセージに記す。特に次を落とさない。

| Phase | 変異 | 失敗するテスト |
|---|---|---|
| 1 | `GroupError.Error()` の字下げを外す | `TestGroupError_IndentsContinuationLines` |
| 1 | 構築関数の panic 検査を外す | `TestGroupErrors_ConstructorsRejectInvalidInput` |
| 1 | `executionErrorContext` の判定 1・2 を逆にする | `TestExecutionErrorContext`（2 件の行） |
| 1 | `GroupError`・`GroupErrors` の複合リテラルを本番に置く（値形・ポインタ形・elided 形・位置指定形の各形で 1 回ずつ） | `TestProductionGroupErrorLiteralsUseConstructors` |
| 1 | `.errs`・`.group`・`.command`・`.err` の各フィールドへ代入する | `TestProductionGroupErrorLiteralsUseConstructors` |
| 2 | `Detail()` を `UserMessage` 優先に戻す | `TestPreExecutionError_Detail` |
| 2 | `HandleExecutionError` を `UserMessage` 優先に戻す | `TestHandleExecutionError_CauseFormatting` |
| 2 | 本番のコメントに旧いシンボル名を戻す | `TestProductionCodeHasNoUserFriendlyError` |
| 2 | `Unwrap() []error` の型アサーションを本番に戻す | `TestProductionCodeDoesNotProbeMultiErrorShape` |
| 3 | `pemPrivateUnterminated` の適用を外す | `TestValueDetector_Mask_PositiveCases`（`BEGIN` の側だけの行）・`TestBoundedBuffer_LineBoundaryCutLeavesNoPartialSecret` |
| 3 | `boundedBuffer.Bytes()` の完全な行への切り詰めを外す（行の途中で切る） | `TestBoundedBuffer_KeepsCompletePrefixLines`・`TestBoundedBuffer_LineBoundaryCutLeavesNoPartialSecret` |
| 3 | 省略の印のバイト数から切り詰め分を除く | `TestBoundedBuffer_KeepsCompletePrefixLines` |
| 4 | `ValidateOutputSizeLimits` の呼び出しを外す | `TestLoadConfig_NegativeOutputSizeLimitValidation`（主設定ファイルの行） |
| 4 | テンプレートの検査を外す | `TestLoadConfig_NegativeOutputSizeLimitValidation`（include の行） |
| 5 | `MaxSize == 0` の判定を外す | `TestCapture_WriteOutput`（0 の行）・`TestRunner_ZeroOutputSizeLimitIntegration` |
| 6 | `newSizeLimitError` の panic 検査を外す | `TestNewSizeLimitErrorPanicsOnNonPositiveLimit` |
| 6 | `CaptureError` の複合リテラルを本番に置く（値形・ポインタ形・elided 形・位置指定形の各形で 1 回ずつ） | `TestProductionCaptureErrorLiteralsUseConstructors` |
| 6 | `.typ`・`.path`・`.phase`・`.cause`・`.limit` の各フィールドへ代入する | `TestProductionCaptureErrorLiteralsUseConstructors` |
| 6 | `GetType`・`GetPath` を戻す | `TestCaptureErrorHasNoLegacyAccessors` |
| 7 | 中断の判定をエラーの中身に戻す | `TestRunner_ExecuteGroupsCollectsCommandTimeout`・`TestExecuteGroupsDoesNotBranchOnCancellationCause` |
| 7 | 検証失敗の通知を中断の判定の後ろへ移す | `TestRunner_ExecuteGroupsReturnsCancellationOnLastGroupVerificationFailure` |
| 7 | `errors.Join(ctx.Err(), err)` を `err` だけにする | `TestRunner_ExecuteGroupsCanceledChildFailureIncludesContextCanceled` |
| 8 | 日英どちらかから (a)〜(e) のアンカーを外す | `scripts/verification/check_output_limit_timeout_docs.sh`（`make verify-docs-checks`） |

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| `GroupErrors` が `Unwrap() []error` を持つため、他の `errors.AsType[*CommandExecutionError]` の利用箇所が複数失敗の先頭に一致する | 誤った group 名・command 名の表示 | `executionErrorContext` の判定順を `*GroupErrors` → `*CommandExecutionError` に固定し、`TestExecutionErrorContext` の 2 件の行で固定する |
| Phase 7 の順序変更で、中断時に検証失敗の通知が落ちる | 改ざん検知の通知漏れ | 通知を中断の判定より前に置き、`TestRunner_ExecuteGroupsReturnsCancellationOnLastGroupVerificationFailure` と既存の `TestRunner_FileVerificationStageKeepsExistingPath` で固定する |
| 保持の上限で、上限を超えた stderr の末尾（失敗の理由）がログと通知から消える | 障害解析に使える情報が減る | [01_requirements.md](01_requirements.md) の決定事項「コマンドの出力はメモリ上で常に上限付きにする」で受け入れ、利用者向け文書（AC-30・AC-31）と [02_architecture.md](02_architecture.md) §4.4 に記載する。末尾の保持は #1186 で扱う |
| `output_size_limit = 0` の無制限化で runner のメモリ使用量が出力に比例する | 可用性 | Phase 3 の保持の上限を先に入れる。出力ファイルへの書き込み量は利用者の設定の責任範囲とする |
| `newSizeLimitError` の panic が出力ポンプの goroutine で起きる | プロセスの異常終了、`RUN_SUMMARY` 行と一時ファイルの後始末が失われる | 上限 0 の無制限（Phase 5）と負の値の拒否（Phase 4）で本番の入力から panic を届かせない。構築関数の panic と入力の網羅をテストで固定する（[02_architecture.md](02_architecture.md) §4.5） |
| 構築ガードの走査が広すぎて無関係な型・テストに誤反応する | ガードの偽陽性、テストの空振り | 走査は `identitymutationguard.ProductionGoFilesInRepo` の本番ファイルに限り、対象を当該型の複合リテラルとそのフィールドへの代入に限定する。検出器自身のテスト（`TestGroupErrorConstructionCheckRecognizesForms`・`TestCaptureErrorConstructionCheckRecognizesForms`）で形を固定する |
| redaction の `pemPrivateUnterminated` が `BEGIN ... PRIVATE KEY` 以降を隠しすぎる | 秘密でない出力の可読性低下 | 秘密鍵を出すより安全な側に倒す決定（[02_architecture.md](02_architecture.md) §3.7）に従う。`PUBLIC KEY` と完全なブロックの扱いを否定・肯定のテストで固定する |
| 上限を超えた出力の Slack の欄の内容が変わる | 既存の表示の変化 | 変化は [02_architecture.md](02_architecture.md) §4.3 で宣言済み。`TestOutputRetention_SlackAndDebugFieldsFromBoundedOutput` で 3 つの場合（印だけ・印が途中に現れる・変更前と同じ）を固定する |
| doc 検査スクリプトのアンカーが翻訳の言い回しに依存する | 翻訳更新で検査が壊れる | アンカーは日英それぞれの確定した文から取り、スクリプトと文書を同じ Phase で更新する。語の意味の妥当性は PR レビューで確認する |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1。構築の AST ガード green）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2。複数 group の帰属の統合テスト green）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3。redaction の性質テストと保持の上限のテスト green）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4。dry-run の統合テスト green）
- [ ] PR-5 マージ済み（対象ステップ: Phase 5。0 の統合テスト green）
- [ ] PR-6 マージ済み（対象ステップ: Phase 6。構築ガード green）
- [ ] PR-7 マージ済み（対象ステップ: Phase 7。タイムアウト・中断・通知のテスト green）
- [ ] PR-8 マージ済み（対象ステップ: Phase 8。`/mktrans` 済み、`make verify-docs-checks` green）
- [ ] すべての AC が §7 の検証で green
- [ ] §4.4 の変異確認をすべて実施し、各コミットメッセージに記録

---

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（ガードテスト・`make` ターゲット・コミット済みスクリプト）、`manual`（PR やデプロイでの観察）を表す。テスト名は `path::TestName` で示す。

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 1、Phase 2 | `test`: `internal/runner/multi_group_error_integration_test.go::TestRunner_MultiGroupFailureAttribution`（group-2 の行が `failed to execute group group-2: command <command> in group group-2 failed: ` に続けて `CaptureError.Error()` を含むこと） |
| AC-02 | Phase 1、Phase 2 | `test`: `TestRunner_MultiGroupFailureAttribution`（`Details:` の各行が `failed to execute group ` で始まること） |
| AC-03 | Phase 2 | `test`: `internal/runner/multi_group_error_integration_test.go::TestHandleExecutionError_FilesystemCaptureErrorKeepsCause` |
| AC-04 | Phase 2 | `test`: `TestRunner_MultiGroupFailureAttribution`（`error_message` が `Details:` と同じ文言であること） |
| AC-05 | Phase 1、Phase 2 | `test`: `cmd/runner/main_test.go::TestExecutionErrorContext`（2 件の行が空を返すこと）、`cmd/runner/integration_attribution_test.go::TestIntegration_MultiGroupFailureHasNoOuterContext`（2 group の失敗の end-to-end） |
| AC-06 | Phase 2 | `test`: `internal/logging/pre_execution_error_test.go::TestPreExecutionError_Detail`・`TestHandleExecutionError_CauseFormatting`。`static`: `internal/logging/execution_error_guard_test.go::TestProductionCodeHasNoUserFriendlyError` |
| AC-07 | Phase 1 | `test`: `internal/runner/group_errors_test.go::TestGroupError_ReadsCommandNameFromCause`、`internal/runner/runner_test.go::TestRunner_ExecuteGroupsBuildsGroupErrors`（0 件で nil、1 件以上で `GroupSpec.Name` を持つ `*GroupErrors`） |
| AC-08 | Phase 1、Phase 2 | `test`: `TestExecutionErrorContext`。`static`: `internal/runner/group_errors_guard_test.go::TestProductionCodeDoesNotProbeMultiErrorShape` |
| AC-09 | Phase 1 | `test`: `group_errors_test.go::TestGroupError_ErrorMatchesLegacyAssembly` |
| AC-10 | Phase 1 | `test`: `group_errors_test.go::TestGroupErrors_UnwrapReachesEachCause` |
| AC-11 | Phase 1、Phase 2 | `test`: `TestExecutionErrorContext`（1 件の行）、`TestRunner_SingleCommandFailureReportUnchanged`、`cmd/runner/integration_attribution_test.go::TestIntegration_SingleCommandFailureKeepsOuterContext` |
| AC-12 | Phase 2、Phase 7 | `test`: `internal/logging/pre_execution_error_test.go::TestHandleExecutionError_DoesNotNotifySlack`、`cmd/runner/integration_attribution_test.go::TestIntegration_SingleCommandFailureKeepsOuterContext`（終了コード 1 と失敗の `RUN_SUMMARY` 行。同じプロセス境界の証拠を AC-35・AC-36 にも使う）、`TestRunner_CommandTimeoutNotifiesSubsequentGroups`（後続 group の通知の増加が AC-32 の範囲に限られること） |
| AC-13 | 各 Phase | `static`: 各 Phase の `make fmt`（Go を変更した場合）・`make test`・`make lint` |
| AC-14 | 各 Phase | `test`: §4.4 の各変異を入れたときに失敗する各テスト（実装者が変異を入れ、失敗を確認する）。`manual`: 実施結果をコミットメッセージに記録する |
| AC-15 | Phase 1 | `test`: `TestExecutionErrorContext`（group レベル・command レベルの `*GroupStageError` の行）、`cmd/runner/integration_attribution_test.go::TestIntegration_SingleGroupStageFailureGetsOuterContext` |
| AC-16 | Phase 6 | `test`: `internal/runner/base/output/errors_test.go::TestCaptureError`（サイズ超過の行） |
| AC-17 | Phase 5、Phase 6 | `test`: `TestCaptureError`、`internal/runner/base/output/capture_test.go::TestCapture_WriteOutput` |
| AC-18 | Phase 6 | `test`: `TestCaptureError`（構築関数の無い種類の行）。`static`: `internal/runner/base/output/errors_guard_test.go::TestCaptureErrorHasNoLegacyAccessors` |
| AC-19 | Phase 7 | `test`: `internal/runner/runner_test.go::TestRunner_ExecuteGroupsCollectsCommandTimeout` |
| AC-20 | Phase 7 | `test`: `TestRunner_ExecuteGroupsCollectsFailureThenTimeout`、`multi_group_error_integration_test.go::TestRunner_TimeoutAttributionIntegration` |
| AC-21 | Phase 7 | `test`: `TestRunner_ExecuteGroupsStopsOnRunContextCancellation`。`static`: `group_errors_guard_test.go::TestExecuteGroupsDoesNotBranchOnCancellationCause` |
| AC-22 | Phase 1、Phase 7 | `test`: `TestExecutionErrorContext`（タイムアウト 1 件と 2 件の行）、`TestRunner_ExecuteGroupsCollectsFailureThenTimeout` |
| AC-23 | Phase 5 | `test`: `TestCapture_WriteOutput`（`MaxSize` 0 の行） |
| AC-24 | Phase 5 | `test`: `internal/runner/output_capture_integration_test.go::TestRunner_ZeroOutputSizeLimitIntegration` |
| AC-25 | Phase 6 | `test`: `errors_test.go::TestNewSizeLimitErrorPanicsOnNonPositiveLimit`。`static`: `errors_guard_test.go::TestProductionCaptureErrorLiteralsUseConstructors`（同一パッケージの構築経路の網羅。パッケージ外はコンパイラが拒否する） |
| AC-26 | Phase 8 | `static`: `make verify-docs-checks`（`scripts/verification/check_output_limit_timeout_docs.sh` が日英の 0 の無制限と負の値の拒否の記述を検査する）。`manual`: 記載した各文を、Phase 5 の `TestRunner_ZeroOutputSizeLimitIntegration` と Phase 4 の `TestLoadConfig_NegativeOutputSizeLimitValidation` の内容と突き合わせてレビューする |
| AC-27 | Phase 4 | `test`: `internal/runner/config/validation_test.go::TestValidateOutputSizeLimits`、`internal/runner/config/loader_includes_test.go::TestLoadConfig_NegativeOutputSizeLimitValidation`、`cmd/runner/integration_attribution_test.go::TestIntegration_NegativeOutputSizeLimitRejectedInDryRun` |
| AC-28 | Phase 3 | `test`: `internal/runner/base/executor/output_pump_test.go::TestBoundedBuffer_KeepsCompletePrefixLines`（保持量が上限を超えないこと）、`internal/runner/base/executor/executor_test.go::TestExecute_OutputWriterReceivesAllBytes`・`TestExecute_NilOutputWriter_BoundedStderrIsPrefixOnly`・`TestExecute_NilOutputWriter_StdoutBoundedOnSuccess` |
| AC-29 | Phase 3 | `test`: `TestBoundedBuffer_KeepsCompletePrefixLines`（完全な行への切り詰めと省略の印のバイト数）、`TestExecute_NilOutputWriter_BoundedStderrIsPrefixOnly`、`internal/runner/output_retention_integration_test.go::TestOutputRetention_SlackAndDebugFieldsFromBoundedOutput` |
| AC-30 | Phase 8 | `static`: `make verify-docs-checks`（`check_output_limit_timeout_docs.sh` が日英の timeout 節の記述を検査する）。`manual`: 記載した各文を、Phase 7 の `TestRunner_CommandTimeoutNotifiesSubsequentGroups` と [02_architecture.md](02_architecture.md) §4.4 の内容と突き合わせてレビューする |
| AC-31 | Phase 8 | `static`: `make verify-docs-checks`（`check_output_limit_timeout_docs.sh` が日英の保持の上限と出力ファイルの記述を検査する）。`manual`: 記載した各文を、Phase 3 の `TestBoundedBuffer_KeepsCompletePrefixLines`・`TestExecute_OutputWriterReceivesAllBytes` と [02_architecture.md](02_architecture.md) §3.7 の内容と突き合わせてレビューする |
| AC-32 | Phase 7 | `test`: `internal/runner/runner_test.go::TestRunner_CommandTimeoutNotifiesSubsequentGroups` |
| AC-33 | Phase 1 | `test`: `group_errors_test.go::TestGroupError_IndentsContinuationLines` |
| AC-34 | Phase 3 | `test`: `internal/redaction/value_detector_test.go::TestValueDetector_Mask_PositiveCases`（`BEGIN` の側だけの行）・`TestValueDetector_Mask_NegativeCases`、`internal/redaction/redactor_test.go::TestDefaultPatternSets_AreUnchanged`、`internal/runner/base/executor/output_pump_test.go::TestBoundedBuffer_LineBoundaryCutLeavesNoPartialSecret` |
| AC-35 | Phase 7 | `test`: `TestRunner_ExecuteGroupsReturnsCancellationOnLastGroupVerificationFailure`（変更前は nil が返る行）。終了コード 1 への対応付けは `cmd/runner/integration_attribution_test.go::TestIntegration_SingleCommandFailureKeepsOuterContext`（nil でないエラーが終了コード 1 と失敗の `RUN_SUMMARY` 行になること）が担う |
| AC-36 | Phase 7 | `test`: `TestRunner_ExecuteGroupsCanceledChildFailureIncludesContextCanceled`。終了コード 1 への対応付けは AC-35 と同じテストが担う |

---

## 8. 横断検索チェックリスト

`make test`・`make lint` が検出できない残存参照・用語の整合だけを挙げる。§7 の表と重複する項目は置かない。

- [ ] `nilWriterStderrLimit` の旧名が、本番コード・テスト・コメントのどこにも残っていないこと（改名台帳。`make test` は識別子の参照だけを検出し、コメント中の旧名は検出しない）。
- [ ] `docs/dev/architecture_design/security-architecture.md` と `security-architecture.ja.md` の §16（出力サイズ制限）が、0 を無制限とする記述と矛盾しないこと。同節は既に「Unlimited: Can disable limit by setting value to 0」としており、変更は不要と見込む（確認して、必要なら別タスクとして記録する）。
- [ ] 本番コードの `errors.Is(..., context.Canceled)`・`errors.Is(..., context.DeadlineExceeded)` の使用箇所を列挙し、残る箇所が中断の判定に使われていないことを記録する（[02_architecture.md](02_architecture.md) §7.4。現状はタイムアウトのセキュリティログ `internal/runner/group_executor.go:629` と Slack 送信の再試行 `internal/logging/slack_sender.go:530` の 2 箇所）。`TestExecuteGroupsDoesNotBranchOnCancellationCause` は `executeGroups` の本体だけを固定するため、この列挙は手作業で行う。
- [ ] `docs/translation_glossary.md` に、Phase 8 で新しく使った用語（先頭の窓、省略の印など）の対訳が `/mktrans` により登録されていること。
- [ ] 日英の `docs/user/toml_config/04_global_level.ja.md` と `04_global_level.md` の見出し構造が一致すること（`/mktrans` の反映後に `make verify-docs` を実行して確認する）。
- [ ] `docs/dev/developer_guide/package_reference.md` に、`boundedBuffer`・`CaptureError`・`GroupErrors` の旧い説明が無いこと（現状の記述を確認し、直接矛盾する記述があれば同じ Phase で直す。無ければ変更しない）。

---

## 9. Success Criteria

- **機能**: AC-01〜AC-12・AC-15〜AC-36 を検証するテスト・スクリプトが green。0 件の失敗で `nil`、1 件以上で `*GroupErrors` を返し、外側の context が件数で決まる。
- **品質**: 各 Phase の `make fmt`・`make test`・`make lint` が green。§4.4 の変異確認をすべて実施し記録済み。既存の `errors.Is`・`errors.AsType` の到達性のテストと静的ガードが green。
- **セキュリティ**: メモリ上の出力が上限付きになり、省略の境目をまたぐ redaction の性質テストが green。`BEGIN` の側だけの秘密鍵のブロックが `RedactText`・`SanitizeOutputForLogging` を通るすべての出力先で隠れる。実行エラーのレコードは `slack_notify=false` のままである（AC-12）。
- **一貫性**: 実行エラーの `Error()` の文言（原因が 1 行の group と単一 `*CommandExecutionError` の失敗）と、終了コード（実行全体の中断を除く）が変わっていない。観察できる挙動の変化は [02_architecture.md](02_architecture.md) §4.3 に挙げたものに限られる。
- **文書**: 日英の `04_global_level` に、0 の無制限・負の値の拒否・タイムアウト後の後続 group・残りうるプロセス・保持の上限と出力ファイルが記載され、`make verify-docs-checks` が green。

---

## 10. 次のステップ

- 本書が承認されたら、Phase 1（PR-1）から順に実装を開始する。
- 実装完了後、PR-8 の文書を対象環境で確認する（manual。0 の無制限とタイムアウト後の後続 group の表示）。
- 上限を超えた出力の末尾の保持は #1186、`CaptureError` の種類と段階の整理は #1180、サイズ超過のセンチネルの重複は #1181、取り込んだテンプレートの負の `timeout` は #1182、`output_size_limit` の利用者向け文書のその他の食い違いは #1183、dry-run の上限表示は #1184 で扱う。
