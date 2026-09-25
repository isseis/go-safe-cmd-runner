# 実装計画書: group 実行前段の失敗の Slack 通知

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-25 |
| Review date | 2026-09-25 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 通知種別定義・共通エンベロープ・表示安全な補間契約: [0172 アーキテクチャ設計書](../0172_slack_notification_message_unification/02_architecture.md)
- 検証エラー通知の既存経路: [0175 アーキテクチャ設計書](../0175_group_verification_error_failed_files/02_architecture.md)
- テストヘルパ配置: [test_organization.md](../../dev/developer_guide/test_organization.md)
- 要件・受け入れ基準プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)

本書の用語は [02_architecture.md](02_architecture.md) の「用語」に従う（実行前段、実行前段の失敗、段階、段階エラー、原因のエラー、段階定義表、Scope の水準、通知レコード、報告出力、記録のみの通知、最終報告）。

---

## 1. 実装の概要

### 1.1 目的

`DefaultGroupExecutor.ExecuteGroup` がコマンド実行を始める前に失敗したとき、失敗した段階を `*GroupStageError` の列挙型フィールドで宣言し、`Runner.executeGroups` がその型から `pre_execution_error` の通知レコードを記録する。これまで Slack に届かなかった 7 か所の失敗（[02_architecture.md](02_architecture.md) §1.1 の #1〜#7）を group ごとに通知する。通知は記録のみとし、報告出力・戻り値・終了コード・既存の検証失敗通知は変えない。設計の詳細は [02_architecture.md](02_architecture.md) §1〜§6 を参照する。

### 1.2 実装方針

1. 段階は型で宣言する。group executor は失敗の発生箇所で段階エラーを作り、`executeGroups` は `errors.AsType[*GroupStageError]` で段階を読む。エラー文字列を検査して段階を選ばない（[02_architecture.md](02_architecture.md) §1.2 原則 1、§3.4）。
2. 段階から `error_type`・Scope の水準・要約文への対応は段階定義表 1 か所に置き、段階不明の汎用行も表の中に持つ（§3.2.3、AC-10）。
3. 段階と Scope の水準の対応は `newGroupStageError` / `newCommandStageError` の 2 つの構築関数で強制する（§3.2.2）。
4. `ExecuteGroup` の出口で、コマンド実行前のエラーが段階エラーを含まなければ `GroupStageUnknown` でラップし（規則 1）、コマンド実行後のエラーには段階エラーを付けない（規則 2）。規則 1・2 は 1 つの非公開の出口関数として実装し、`ExecuteGroup` を named return にして先頭（#1 より前）で登録した deferred 出口から呼ぶ（§3.3.2）。出口の deferred 登録を先頭に置くのは、将来 #1 より前に実行前段の処理が足されたときの失敗も規則 1 で拾うためである。
5. `executeGroups` は段階エラーを含む失敗を、`*verification.Error` の有無より先に宣言された段階で振り分ける。段階が `GroupStageFileVerification` で連鎖に `*verification.Error` を含む失敗だけを既存の経路（0175）に残し、それ以外の段階エラーは新設の記録のみの通知で通知し、`groupErrs` に積む（§3.4、AC-12・AC-13・AC-16）。
6. 記録のみの通知は `logging.NotifyPreExecutionError` として新設し、属性の組み立てとレコードの記録は `HandlePreExecutionError` と共有する（`handleErrorCommon` から切り出す）。新設の `error_type` 定数 4 件を追加する（§3.5、AC-15・AC-17）。
7. #1〜#7 の原因のエラーは発生箇所が現在返しているエラーそのものとし、エラー文言を変えない。唯一の例外は #7 で、AC-07 のために `command dependency verification failed for %q: %w` の接頭辞を原因に加える（§3.3.1・§3.3.4）。
8. Go のソースコメント・識別子・文字列リテラルは英語で書く。

### 1.3 既存コード調査結果

2026-09-25 時点の commit `29b742da`（`docs(0176): Approved the architecture plan`）のコードを読んだ結果を示す。[02_architecture.md](02_architecture.md) が基準とした `ee19df7b` と `29b742da` の差分は `docs/` だけであり（`git diff --stat ee19df7b 29b742da`）、コードの行番号は一致した。

#### group executor の発生箇所（`internal/runner/group_executor.go`）

| # | 場所 | 現状と変更 |
|---|---|---|
| 1 | `ExecuteGroup` の `:155-158`（`config.ExpandGroup`） | `fmt.Errorf("failed to expand group[%s]: %w", ...)` を `newGroupStageError(GroupStageGroupPreparation, groupSpec.Name, ...)` に渡す |
| 2 | `ExecuteGroup` の `:172-175`（`resolveGroupWorkDir`） | `fmt.Errorf("failed to resolve work directory: %w", err)` を `newGroupStageError(GroupStageGroupPreparation, ...)` に渡す |
| 3 | `preExpandCommands`（`:275-309`）の内側 `:296`（`ExpandCommand`）と `:301`（`resolveCommandWorkDir`） | 現在 `ExecuteGroup:194` の外側で付けている `failed to pre-expand commands for group[%s]: ` を内側の原因へ移し、`newCommandStageError(GroupStageCommandPreparation, groupSpec.Name, cmdSpec.Name, ...)` を返す。`ExecuteGroup:193-195` は段階エラーをそのまま返す |
| 4 | `auditGroupDirPermissions`（`:314-356`）の `:340`・`:352` | `newGroupStageError(GroupStageDirPermissionAudit, runnertypes.ExtractGroupName(runtimeGroup), ...)` で返す |
| 5 | `verifyGroupFiles`（`:361-417`）の `:375-378`（`VerifyGroupFiles`） | 現在 `groupName := runnertypes.ExtractGroupName(runtimeGroup)` は `VerifyGroupFiles` の呼び出しより後（`:380`）で宣言されている。この宣言を `input` の組み立て（`:366-369`）より前へ移し、`input` の `Name`（現在 `:367` で `runnertypes.ExtractGroupName(runtimeGroup)` を直接呼んでいる）にも同じ `groupName` を使う。返ったエラーを無条件に `newGroupStageError(GroupStageFileVerification, groupName, err)` でラップする。`*verification.Error` の判定は group executor では行わない（§3.3.1） |
| 6 | `verifyGroupFiles` の `:392-395`（`ResolvePath`） | `fmt.Errorf("command path resolution failed for %q: %w", ...)` を `newCommandStageError(GroupStageCommandVerification, groupName, cmd.Name(), ...)` に渡す |
| 7 | `verifyGroupFiles` の `:407-413`（`VerifyCommandDependencies`） | 直前の `slog.Error("Command dependency verification failed", ...)` は残し、原因を `fmt.Errorf("command dependency verification failed for %q: %w", resolvedPath, depErr)` として `newCommandStageError(GroupStageCommandVerification, groupName, cmd.Name(), ...)` に渡す（§3.3.4） |
| 出口規則 | `ExecuteGroup` の `:146-221` | `executionResult` はコマンド実行に進んだかどうかを示す既存の状態（`:164-170`・`:207-219`）。規則 1・2 を 1 つの非公開の出口関数に実装し、named return と deferred 出口から呼ぶ。deferred 出口は `ExecuteGroup` の先頭（`:147` 付近、#1 の `:155` より前）で登録する。deferred 出口が `executionResult` を参照できるよう、その宣言（現在 `:165`）を deferred 出口の登録より前へ移す。既存の通知 defer（`:166-170`）の処理内容は変えない |
| 名称の出所 | `groupSpec.Name`（#1〜#3）、`runnertypes.ExtractGroupName(runtimeGroup)`（#4 は `:353`、#5〜#7 は `verifyGroupFiles` の先頭へ移した `groupName`。移す前の宣言は `:380`）、`cmd.Name()`（`runnertypes/runtime.go:319-324`） | 同じ設定値 `GroupSpec.Name` に由来する。追加の正規化はしない |
| `CommandExecutionError` | `:27-41` | コマンド実行後の失敗。段階エラーを付けない（規則 2）。変更しない |

#### `executeGroups`（`internal/runner/runner.go`）

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| 段階エラーの分岐 | `:420-437` | `:422-424` のキャンセル判定の後、`:427` の `*verification.Error` 判定の前に、`errors.AsType[*GroupStageError]` の分岐を新設する。段階が `GroupStageFileVerification` で `*verification.Error` を含めば既存の `runerrors.NewVerificationPreExecutionError` 経路（`:431-433`）、それ以外は変換関数で `NotifyPreExecutionError` を呼び `groupErrs` に積む |
| `groupErrs` と戻り値 | `:436`・`:441-442` | 変更しない |
| import | `:14-31` | `common`・`logging`・`resource`・`runerrors`・`verification` は import 済み。新設する変換関数は `group_stage.go` に置くため、`runner.go` の import は増えない |

#### 通知側（`internal/logging`）と共通型

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| `ErrorType` 定数 | `internal/logging/pre_execution_error.go:25-47` | 既存 10 件。`ErrorTypeGroupPreparation`・`ErrorTypeGroupDirPermissionViolation`・`ErrorTypeCommandVerification`・`ErrorTypeGroupPreExecution` の 4 件を追加する。`ErrorTypeGroupFileVerification`（`:42-43`）は既存を使う |
| `handleErrorCommon` | `:126-157` | stderr 出力・slog 記録・stdout の `RUN_SUMMARY` を 1 つに持つ。slog 記録部分を非公開関数へ切り出し、`HandlePreExecutionError`・`HandleExecutionError`・新設の `NotifyPreExecutionError` が同じ記録部分を通るようにする。既存 3 出力の内容・順序は変えない |
| `HandlePreExecutionError` | `:162-181` | 変更しない（記録部分は切り出し先を呼ぶ形にする） |
| 記録のみの通知 | なし | `NotifyPreExecutionError(preExecErr *PreExecutionError)` を新設する（§3.5.2）。メッセージは `Pre-execution error notified`。`slack_notify` は既存の `NotificationAttrs` 経由で設定する |
| `PreExecutionError` 型 | `:50-67` | フィールドは変更しない |
| `notification_context.go` | `internal/common/notification_context.go:15-22`・`:45-62` | `ScopeGroup`・`ScopeCommand`・`GroupScope`・`CommandScope` を使う。変更しない |
| `resource.ComponentRunner` | `internal/runner/resource/types.go:376-377` | 既存。変換関数の `Component` はこれに固定する（§3.2.3） |

#### 再利用するテスト・ヘルパ

| 対象 | 位置 | 使い方 |
|---|---|---|
| group executor のテスト補助 | `internal/runner/group_executor_helpers_test.go:12-58` | `NewTestGroupExecutor`・`NewTestGroupExecutorWithConfig` を使う |
| 検証マネージャのモック | `internal/verification/testutil/testify_mocks.go:12-35` | `VerifyGroupFiles`・`ResolvePath`・`VerifyCommandDependencies` の失敗を注入する |
| `MockGroupExecutor` | `internal/runner/test_helpers.go:20-29` | `Runner.Execute` 経由の配線テストで使う |
| ログレコーダ | `internal/testutil/handlers.go:42`・`:188`・`:202`・`:256` | `tu.NewLogRecorder`・`RequireRecord`・`AssertAttrs`・`AssertNotificationContext` を使う |
| 既存の検証分岐テスト | `internal/runner/runner_test.go:2385`・`:2439` | `TestRunner_VerificationErrorCarriesGroupScopeAndCleanMessage`・`TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent` の構成（`MockGroupExecutor` と `RedactingHandler`）を流用する。両テストはモックが生の `*verification.Error` を返すため変更不要（§3.8） |
| 統合テスト補助 | `cmd/runner/integration_test_helpers.go:184-331` | `slackRunSpec`・`runMainWithSlackMock`・`requireSinglePreExecutionError`・`attachmentField`・`stderrDetailsLine` を再利用する。`t.Parallel` を使わない |
| 表示安全の検査 | `internal/logging/slack_handler_test.go:1485`（`assertDisplaySafeProperties`） | AC-18 のビルダー検査で使う |
| 例外出力の捕捉 | `internal/logging/pre_execution_error_test.go:431`（`captureErrorOutput`） | `NotifyPreExecutionError` が報告出力をしないことの検査で使う |

#### 既存の静的ガード（変更せず通すこと）

| ガード | 位置 | 本計画との関係 |
|---|---|---|
| `TestProductionPreExecutionErrorLiteralsCarryNotificationContext` | `internal/logging/notification_contract_guard_test.go:41` | 変換関数の `PreExecutionError` リテラルは `NotificationContext` を設定する |
| `TestProductionErrorLiteralsUseTypedComponent` | 同 `:80` | `Component` を `string(resource.ComponentRunner)` と字句どおり書く（表から読む値にはしない） |
| `TestProductionCodeSetsSlackNotifyOnlyInNotificationAttrs` | 同 `:1094` | `NotifyPreExecutionError` は `NotificationAttrs` 経由で `slack_notify=true` を設定する |
| `TestFiringPointsUseSharedVerificationConstructor` | `internal/runner/runerrors/pre_execution_guard_test.go:58` | `NewVerificationPreExecutionError` の呼び出しは `runner.go`・`cmd/runner/main.go` に 1 件ずつ。変換関数は別関数であり、`FailedFilePaths` を設定しない |
| `TestIdentifierDeclarationCatalog`（宣言サイト表 `declarationSiteCatalog`） | `internal/identifier/identifier_guard_test.go:71-123`・`:145` | 本計画は `identifier.NewIdentifier` の呼び出しを追加しない。`slog.Error("Command dependency verification failed")`（`:83`、`verifyGroupFiles` 内・`"group"`→`groupName`）はそのまま残す |
| `TestRegisteredTypeNamesAppearOnlyInNotificationRegistry` | `internal/logging/notification_contract_guard_test.go:1156` | 追加するのは `error_type` の値であり `message_type` の登録名ではない。衝突しない |

#### 構築経路の扱い（`GroupStageError`）

`GroupStageError` は `internal/runner` の新しいファイル `group_stage.go` に置く（§2.2）。フィールドを非公開にして構築を `newGroupStageError` / `newCommandStageError` に限るのは [02_architecture.md](02_architecture.md) §3.2.2 の決定であり、その不変条件を本計画がどう守るかを次に定める。型を葉パッケージへ移さないのは、`internal/runner` の中だけで使う型であり、`CommandExecutionError`（`group_executor.go:27-41`）も同じパッケージにあるためである。フィールドが非公開なので他パッケージからは構築関数しか使えないが、同一パッケージの本番コードは複合リテラルでも構築できる。コンパイラで強制できないその分を `go/ast` のガード（Phase 2）で列挙して固定する。列挙する構築形は次の 2 つである。

- `GroupStageError` 複合リテラル（値形・ポインタ形・elided 形）。修飾形 `runner.GroupStageError{...}` はリポジトリ全体で、修飾なしの `GroupStageError{...}` は `internal/runner` 直下のファイルでだけ検出し、`group_stage.go` の `newGroupStageError`・`newCommandStageError` の外にあるものを違反とする。関数単位で免除するのは、既存の `TestVerificationErrorLiteralsOnlyInConstructor`（`internal/verification/error_construction_guard_test.go`）の規則 (a) と同じである。
- `internal/runner` 直下（`filepath.Dir(file) == "internal/runner"`、つまり `runner` パッケージ）の本番ファイルにある `.stage`・`.group`・`.command`・`.err` へのセレクタ代入とインクリメント。同じ 2 つの構築関数の外にあるものを違反とする。フィールドは非公開なので `runner` パッケージの外からは代入できず（コンパイルエラーになる）、この範囲に限っても漏れはない。また現時点で `internal/runner` 直下の本番ファイルにこれらの名前のセレクタへの代入は無いため、`go/types` で受け手の型を解決しなくても名前だけの照合で足りる。パッケージの外まで走査すると、`internal/runner/base/executor` の `pc.stage = ...`（`command_lifecycle.go:383`）や `*w.err = nil`（`executor.go:433`）のような無関係な代入に誤反応する。

テストファイルは走査対象に含めない（`identitymutationguard.ProductionGoFilesInRepo` を使う）。この絞り方は既存の `TestVerificationErrorLiteralsOnlyInConstructor` の規則 (b) と同じである。このガードは §3.2.2 の決定を変えず、その実効性を検証する追加である。また、ガード自身の検出器が空振りしないことは、`test_helpers` の形式に倣った認識対象の表テスト（`TestGroupStageConstructionCheckRecognizesForms`）で固定する。

出口規則 1・2 も [02_architecture.md](02_architecture.md) §3.3.2 が規則だけを定め、実装の機構は定めていない。同書 §7.2 は「段階エラーを返さない失敗を注入する手段を実装計画で定める」としているため、本計画は規則 1・2 を非公開の出口関数 1 つに実装し、その関数を直接呼ぶテストで規則 1・2 を固定する。直接呼ぶテストは、`ExecuteGroup` が最初の `return` より前に deferred 出口を登録し、named return をその出口に通すことまでは示さない。Phase 3 の後は #1〜#7 のどれも段階エラーでないエラーを返さず、段階エラーでないエラーを実行前に注入する手段も無いため、この配線は `go/ast` のガード `TestExecuteGroupRegistersExitDefer`（Phase 3）で構造として固定する。この選択は規則の内容を変えない。

#### 事実として確認した既存挙動

- `config.ExpandGroup` の本番呼び出しは `internal/runner/group_executor.go:155` の 1 か所だけである（`rg -n "ExpandGroup" --glob '*.go' | rg -v _test` の一致は定義とこの 1 件）。group の `vars`・`env`・`verify_files` は実行時に展開されるため、未定義変数の失敗を `Runner.Execute` 経由の統合テストで起こせる。
- `ErrUndefinedVariableDetail` は `Context` に展開前の生テンプレートを埋め込む（`internal/runner/config/expansion.go:128-133`）。複数行の TOML 文字列をテンプレートに使うと、原因の文言に生の改行が入る（§3.7、AC-18 の前提）。ただし group の `vars` の未定義変数では `Context` が空になり、テンプレートは文言に入らない。生の改行を文言に載せる #1 の失敗には group の `env_vars` の値を使う（Phase 3 の実装時に確認）。
- 既存テストの網羅は #1〜#7 で一様でない。`#2` は `TestExecuteGroup_VariableExpansionError`（`group_executor_test.go:1885`）、`#3` は `TestExecuteGroup_ExpandCommandError`（`:1999`）・`TestExecuteGroup_ResolveCommandWorkDirError`（`:2054`）・`TestPreExpandCommands_Error`（`:3014`）、`#4` は `TestWithDirPermAuditor_ReachesGroupExecution`（`runner_test.go:2551`）・`TestAuditGroupDirPermissions_ViolationReturnsError`（`group_executor_test.go:3510`）、`#6` は `TestVerifyGroupFiles_ResolvePathFailure`（`group_executor_test.go:1303`）・`TestVerifyGroupFiles_DynLibResolvePathFailure`（`:3171`）、`#7` は `TestVerifyGroupFiles_ShebangInterpreter_Error`（`:3354`）・`TestVerifyGroupFiles_ShebangInterpreter_UsesEffectiveEnvPATH`（`:3415`）が覆う。`#1`（`ExpandGroup` 失敗）と `#5`（`VerifyGroupFiles` の失敗。`*verification.Error` か否かを問わず）を覆う既存の group executor テストは無い。`#5` の `*verification.Error` の経路は group executor のテストでは無く、生の `*verification.Error` を返すモックを使う runner 境界のテスト（`runner_test.go:2385`・`:2439`）が覆う。いずれのテストも `errors.Is` / `assert.ErrorContains` で判定しており、原因の連鎖が `Unwrap` で保たれるため変更不要（§3.8）。

### 1.4 テストヘルパーの方針

- 新しいクロスパッケージのヘルパ・モックは作らない。既存の `verificationtestutil.MockManager`・`MockGroupExecutor`・`tu.NewLogRecorder`・`runMainWithSlackMock` で足りる。
- `internal/runner/group_stage_test.go` は新規テストファイル（`package runner`）として追加する。`DefaultGroupExecutor` を直接組む必要があるテストは `group_executor_test.go` に置き、新しい `test_helpers*.go` は作らない。
- `internal/logging` のビルダー検査は `slack_handler_test.go` に追加し、既存の `assertDisplaySafeProperties`・`redactedPreExecutionErrorMessage` の手法を再利用する。
- `cmd/runner` の統合テストは `integration_test_helpers.go` の既存ヘルパを使い、新しいファイルは追加しない。ただし現行の `runMainWithSlackMock` は `captureStdoutStderr` の stdout を返さない（`integration_test_helpers.go:280` の `_, stderr := ...`）。AC-15 の `RUN_SUMMARY` 行数を観測するため、`slackRun` に `stdout string` フィールドを足し、`runMainWithSlackMock` が stdout も返すようにする（Phase 4）。既存の呼び出し元はフィールド追加の影響を受けない。

---

## 2. 実装ステップ

各 Phase の完了時、および各 PR のマージ前に `make fmt`・`make test`・`make lint` を通す（AC-21）。追加・変更した各テストは、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（AC-22、§4.4）。

### Phase 1: `logging` の `error_type` と記録のみの通知

**対象ファイル**: `internal/logging/pre_execution_error.go`、`internal/logging/pre_execution_error_test.go`

**作業内容**:

- [x] `pre_execution_error.go` の `ErrorType` 定数に、`ErrorTypeGroupPreparation`（`"group_preparation_failed"`）・`ErrorTypeGroupDirPermissionViolation`（`"group_dir_permission_violation"`）・`ErrorTypeCommandVerification`（`"command_verification_failed"`）・`ErrorTypeGroupPreExecution`（`"group_pre_execution_failed"`）を、[02_architecture.md](02_architecture.md) §3.5.1 の doc コメント（英語）付きで追加する。
- [x] `handleErrorCommon`（`:126-157`）から slog の記録部分を非公開関数へ切り出し、`handleErrorCommon` と新設の `NotifyPreExecutionError` が同じ部分を通るようにする。stderr 出力・slog 記録・stdout の `RUN_SUMMARY` の内容と順序は変えない。
- [x] `NotifyPreExecutionError(preExecErr *PreExecutionError)` を追加する。`NotificationAttrs` で `slack_notify=true` と通知コンテキストを組み立て、`FailedFilePaths` が空でないときだけ属性を加え、レベル ERROR・メッセージ `Pre-execution error notified` で記録する。stderr・stdout には書かない。
- [x] `pre_execution_error_test.go` に `TestNotifyPreExecutionError_RecordsWithoutReport` を追加する。既存の `captureErrorOutput` と `tu.NewLogRecorder` で、(a) stdout・stderr に何も書かないこと、(b) `HandlePreExecutionError` と同じ属性（`error_type`・`error_message`・`component`・`run_id`・通知コンテキスト・`slack_notify`）を記録しメッセージだけが異なること、(c) `FailedFilePaths` 付きのときだけ `failed_file_paths` を加えること、を固定する。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestNotifyPreExecutionError_RecordsWithoutReport` が、stderr 書き出しを足す／`slack_notify` の設定を外す／メッセージを `HandlePreExecutionError` と同一にすると失敗することを確認する。

### PR-1 作成ポイント: pre-execution error types and record-only notification

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0176): add the group pre-execution error types and record-only notification`

**レビュー観点**: 4 つの `error_type` 定数の値と doc コメントが §3.5.1 のとおりであること／`NotifyPreExecutionError` が stderr・stdout に書かず、属性とレベル・メッセージが `HandlePreExecutionError` と共有部分を通ること／`slack_notify=true` が `NotificationAttrs` 経由であること／既存 3 出力の内容と順序が変わっていないこと

**実装モデル要件**: standard

**判定理由**: 定数追加と、既存関数からの記録部分の切り出し・薄い追加関数であり、分岐や外部依存の変更を伴わない。Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: `internal/runner/group_stage.go`（段階エラー・段階定義表・変換関数）

**対象ファイル**: `internal/runner/group_stage.go`（新規）、`internal/runner/group_stage_test.go`（新規）

**作業内容**:

- [x] `group_stage.go` に `GroupStage` 列挙型と `GroupStageUnknown`〜`GroupStageCommandVerification`・非公開の `groupStageCount` を、[02_architecture.md](02_architecture.md) §3.2.1 の doc コメント（英語）付きで定義する。`String()`（§3.2.1 の図）は各段階の Go 名を返し、範囲外の段階には `unknown` を返す。
- [x] 段階定義表を `groupStageCount` 個の要素を持つ配列として定義する。各要素は `error_type`（`logging.ErrorType`）・Scope の水準（`common.NotificationScope`）・要約文（`Message`）を持つ。行の値は §3.2.3 の表のとおりとし、`GroupStageUnknown` の行を汎用行にする。
- [x] `GroupStageError` を、フィールド `stage`・`group`・`command`・`err` を非公開にして定義する。アクセサ `Stage()`・`GroupName()`・`CommandName()`・`Error()`・`Unwrap()` を §3.2.2 のとおり実装する。`Error()` は原因の文言を返し、原因が `nil` のとき固定の文言 `group pre-execution failed` を返す。`Unwrap()` は原因を返す。
- [x] 構築関数 `newGroupStageError(stage GroupStage, group string, err error) *GroupStageError` と `newCommandStageError(stage GroupStage, group, command string, err error) *GroupStageError` を追加する。段階定義表を引いて水準を確認し、表にない段階と水準の食い違い、範囲外の段階、空の group 名・コマンド名、`nil` の原因を panic で拒否する（§3.2.2）。
- [x] 変換関数 `groupStagePreExecutionError(stageErr *GroupStageError, runID string) *logging.PreExecutionError` を追加する。段階を索引として表を引けば表の行、引けなければ汎用行を使い、`Type`・`Message`・`Component`（`string(resource.ComponentRunner)`）・`RunID`・`NotificationContext`（group 水準なら `common.GroupScope(group)`、command 水準なら `common.CommandScope(group, command)`）・`Err`（原因）を設定する。`FailedFilePaths` は設定しない。
- [x] `group_stage_test.go` に次を追加する。
  - `TestGroupStageTableHasARowForEveryStage`: `GroupStageUnknown` から `groupStageCount` の手前までを走査し、すべての段階に行があり、`GroupStageUnknown` 以外が汎用行でないことを検証する（AC-09）。
  - `TestGroupStagePreExecutionErrorMapping`: 各段階について `Type`・`Message`・`Component`・`NotificationContext`・`Err` を検証する。期待値は §3.2.3 の表から書く（AC-01〜AC-07）。
  - `TestGroupStageUnknownAndOutOfRangeUseGenericRow`: 有効な group 名と `nil` でない原因を持ち段階が `GroupStageUnknown` の段階エラーと、範囲外の段階 `GroupStage(99)` のリテラルで作った段階エラーが、汎用行の `Type`・`Message` と有効な group 水準の Scope になることを検証する（AC-10）。
  - `TestGroupStageErrorZeroValueDoesNotPanic`: `GroupStageError{}` の `Error()` が panic せず固定の文言を返し、変換結果が汎用行を使うことを検証する。
  - `TestGroupStagePreExecutionErrorUsesDeclaredStageNotReasonText`: 原因の文言が別の段階を示唆する入力（例: 原因に `verification` や `permission` を含むが段階は `GroupStageGroupPreparation`）でも、`error_type` が宣言された段階に従うことを検証する（AC-09）。
  - `TestGroupStageConstructorsPanicOnInvalidInput`: 水準の不一致・範囲外の段階・空の名前・`nil` の原因で panic することを検証する。
  - `TestGroupStageErrorUnwrapsCause`: `Error()` が原因の文言と一致し、`errors.Is` / `errors.AsType` が原因の連鎖を辿れることを検証する。
  - `TestGroupStageStringCoversEveryStage`: `GroupStageUnknown` から `groupStageCount` の手前までが互いに異なる非空文字列を返し、範囲外の段階が `unknown` を返すことを検証する。
  - `TestProductionGroupStageErrorLiteralsUseConstructors`: `identitymutationguard.ProductionGoFilesInRepo` で本番ファイルを走査し、`group_stage.go` の 2 つの構築関数の外に `GroupStageError` 複合リテラルが無いこと、および `internal/runner` 直下の本番ファイルに、2 つの構築関数の外で `.stage`・`.group`・`.command`・`.err` へのセレクタ代入とインクリメントが無いことを固定する（§1.3 の構築経路の扱い）。走査が空振りしていないことは、`group_stage.go` 自身の中に構築関数が作る `GroupStageError` 複合リテラルが 1 件以上見つかることで確かめ、見つからなければ失敗させる（Phase 2 の時点では `group_stage.go` 以外に構築関数の本番呼び出しが無いため、呼び出しの件数を空振り検出に使わない）。検出器そのものが動くことは `TestGroupStageConstructionCheckRecognizesForms`（値・ポインタ・elided 形、`.stage`・`.group`・`.command`・`.err` の代入とインクリメント、許容形、別パッケージの同名型）で固定する。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestGroupStageTableHasARowForEveryStage` が表の行を 1 つ削ると失敗すること、`TestGroupStageUnknownAndOutOfRangeUseGenericRow` が範囲外の索引を専用行に変えると失敗すること、`TestProductionGroupStageErrorLiteralsUseConstructors` が `group_executor.go` に直接リテラルを置く／`.stage` に代入すると失敗し、`internal/runner/base/executor` の既存の `.stage`・`.err` への代入には反応しない（壊していない状態で `make test` が通る）ことを確認する。

### PR-2 作成ポイント: group stage type, table, and conversion

**対象ステップ**: Phase 2

**推奨タイトル**: `feat(0176): add the group stage type and its definition table`

**レビュー観点**: 段階・`error_type`・Scope の水準・要約文の対応が 1 つの表に閉じ、段階不明の汎用行を含むこと／構築関数が水準の食い違いを拒否し、変換関数が `Component` を `string(resource.ComponentRunner)` の字句で書くこと（既存ガードに通す）／`FailedFilePaths` を設定せず、既存の `runerrors` ガードを壊さないこと／構築形を列挙するガードが値・ポインタ・elided 形とセレクタ代入を覆い、テストファイルを走査対象に含めないこと

**実装モデル要件**: standard

**判定理由**: 型・表・純関数の追加で、対応は [02_architecture.md](02_architecture.md) §3.2 に固定済み。走査ガードは既存の `identitymutationguard` を再利用し、未確定の実装アプローチや高リスク分岐は無い。Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: group executor の段階宣言と出口規則

**対象ファイル**: `internal/runner/group_executor.go`、`internal/runner/group_executor_test.go`

**作業内容**:

- [x] #1（`:155-158`）を `newGroupStageError(GroupStageGroupPreparation, groupSpec.Name, <現在の原因>)` に変える。
- [x] #2（`:172-175`）を `newGroupStageError(GroupStageGroupPreparation, groupSpec.Name, <現在の原因>)` に変える。
- [x] #3 を `preExpandCommands` の内側（`:296`・`:301`）で作り、`failed to pre-expand commands for group[%s]: command[%s] (index %d): ...` を原因に含めた `newCommandStageError(GroupStageCommandPreparation, groupSpec.Name, cmdSpec.Name, ...)` を返す。`ExecuteGroup:193-195` の外側の接頭辞は削除する。
- [x] #4（`auditGroupDirPermissions` の `:340`・`:352`）を `newGroupStageError(GroupStageDirPermissionAudit, runnertypes.ExtractGroupName(runtimeGroup), <現在の原因>)` に変える。`:340` の `errUnhandledCheckSkipReason` は現状到達しないため、その段階宣言を検証するテストは無い（宣言を外しても出口規則 1 で汎用行として通知される）。
- [x] `verifyGroupFiles` の `groupName := runnertypes.ExtractGroupName(runtimeGroup)`（`:380`）を `input` の組み立て（`:366-369`）より前へ移し、`input.Name` にもこの `groupName` を使う。#5〜#7 はこの `groupName` を使う（移さないと #5 の時点で `groupName` が未宣言でコンパイルできない）。
- [x] #5（`verifyGroupFiles` の `:375-378`）を `newGroupStageError(GroupStageFileVerification, groupName, err)` に変える。`*verification.Error` の判定はここでは行わない。
- [x] #6（`:392-395`）を `newCommandStageError(GroupStageCommandVerification, groupName, cmd.Name(), <現在の原因>)` に変える。
- [x] #7（`:407-413`）で、`slog.Error` は残し、原因を `command dependency verification failed for %q: %w` でラップして `newCommandStageError(GroupStageCommandVerification, groupName, cmd.Name(), ...)` を返す（§3.3.4）。
- [x] `ExecuteGroup` の出口規則 1・2 を 1 つの非公開の出口関数に実装し、`ExecuteGroup` を named return と deferred 出口から呼ぶ形にする。出口関数は、エラーが `nil` またはコマンド実行後（`executionResult != nil`）ならそのまま返し、コマンド実行前で `*GroupStageError` を含まなければ `newGroupStageError(GroupStageUnknown, groupSpec.Name, err)` を返す。
- [x] `group_executor_test.go` に `TestExecuteGroup_PreExecutionStageErrors` を追加する。既存テストの手法（`TestExecuteGroup_VariableExpansionError`・`TestExecuteGroup_ExpandCommandError`・`TestExecuteGroup_ResolveCommandWorkDirError`・`TestWithDirPermAuditor_ReachesGroupExecution`・`TestVerifyGroupFiles_DynLibResolvePathFailure`・`TestVerifyGroupFiles_ResolvePathFailure`）を流用し、#1〜#7 をそれぞれ失敗させて `errors.AsType[*GroupStageError]` で段階・group 名・コマンド名と `Error()` を検証する（AC-11・AC-01〜AC-07）。#1・#2 は `GroupStageGroupPreparation`、#3 は `GroupStageCommandPreparation` とコマンド名、#4 は `GroupStageDirPermissionAudit`、#5 は `GroupStageFileVerification`、#6・#7 は `GroupStageCommandVerification` とコマンド名を確認する。#7 は `Error()` にコマンドのパスと原因の両方を含むことも確認する（AC-07）。#3 の `Error()` が接頭辞を二重に含まないことも確認する。
- [x] 同テストに、AC-18 の前提を作る行を追加する。#1（group の `env_vars` の値。§1.3 のとおり `vars` では文言にテンプレートが入らない）を複数行の TOML 文字列をテンプレートとして失敗させ、原因の文言に生の改行が含まれることを確認し、#6 では `ExpandedCmd` に、#7 では `ResolvePath` のモックが返す解決済みパス（#7 の `%q` が引用するのは `resolvedPath` であり `ExpandedCmd` ではない）に改行と `U+202E` を含め、`Error()` のパスが `%q` で引用されることを確認する。
- [x] `group_executor_test.go` に `TestExecuteGroup_PreExecutionExitRules` を追加する。出口関数を直接呼び、段階エラーを含まないコマンド実行前のエラーが `GroupStageUnknown` でラップされることと、コマンド実行後（`executionResult != nil`）のエラーはラップされないことを固定する（AC-10・AC-13）。
- [x] `group_executor_test.go` に `go/ast` のガード `TestExecuteGroupRegistersExitDefer` を追加する。段階エラーでないエラーを実行前に注入する手段が無いため、`ExecuteGroup` が出口関数を通ることを構造で固定する。`identitymutationguard.ReadProductionSource` で `internal/runner/group_executor.go` を読み、`identitymutationguard.ParseSource` で構文解析したうえで、`DefaultGroupExecutor` の `ExecuteGroup` について次を検証する。
  - 結果が名前付きの `error` 1 つとして宣言されていること（named return）。
  - 本体の最上位の文の中に、関数リテラルを `defer` する文があり、その関数リテラルの本体が名前付きの結果に出口関数の呼び出し結果を代入していること（`err = <出口関数>(...)` の形。代入先の識別子が名前付きの結果と一致し、右辺が出口関数の呼び出しであること）。
  - 本体のどこにも、その `defer` 文より前の位置にある `return` 文が無いこと（`ast.Inspect` で本体全体の `ReturnStmt` を集め、位置が `defer` 文より前のものが 0 件であること）。関数リテラルの中の `return` は `ExecuteGroup` の `return` ではないため集めない。`defer` が本体の最初の文であることは要求しない。出口規則の行のとおり、`startTime := time.Now()` と前へ移した `var executionResult` の宣言は `defer` より前に置く。
- [x] 同ファイルに `TestExecuteGroup_CommandExecutionFailureHasNoStageError` を追加する。既存の `TestExecuteGroup_CommandExecutionFailure` の構成を流用し、返ったエラーが `*GroupStageError` を含まないことを固定する（AC-13）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestExecuteGroup_PreExecutionStageErrors` が各段階の宣言を外すと失敗すること、`TestExecuteGroup_PreExecutionExitRules` が出口規則 1 を外す／規則 2 を無効化すると失敗すること、`TestExecuteGroupRegistersExitDefer` が deferred 出口の登録を削除する／`config.ExpandGroup` の失敗時の `return`（#1）より後ろへ移すと失敗することを確認する。

### PR-3 作成ポイント: declare the stage at every pre-execution failure site

**対象ステップ**: Phase 3

**推奨タイトル**: `feat(0176): declare the failure stage in the group executor`

**レビュー観点**: #1〜#7 の各発生箇所が正しい段階・group 名・コマンド名で段階エラーを返すこと／#3 の接頭辞が原因のエラーへ移り、最終報告の文言に重複が無いこと／#7 の原因だけが `%q` の接頭辞で変わり、`slog.Error` と変更対象外の文言は変わらないこと／出口規則 1 が段階宣言の忘れを汎用段階で拾い、規則 2 がコマンド実行後の失敗に段階を付けないこと／既存の `errors.Is` テストが `Unwrap` で通ること

**実装モデル要件**: frontier-recommended

**判定理由**: 7 か所のエラー生成の書き換え、接頭辞の移動、文言が変わる #7、`ExecuteGroup` の出口制御という可視挙動を持つ複数ステップで、実装モデルの能力が結果を左右しうるため。段階の raise/lower や段階的 rollout は無く panel-mode トリガーには該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: `executeGroups` の振り分け・配線テスト・統合テスト・本文の安全性

**対象ファイル**: `internal/runner/runner.go`、`internal/runner/runner_test.go`、`internal/logging/slack_handler_test.go`、`cmd/runner/integration_pre_execution_error_test.go`

**作業内容**:

- [ ] `runner.go` の `executeGroups`（`:420-437`）で、`:422-424` のキャンセル判定の後、`:427` の `*verification.Error` 判定の前に `errors.AsType[*GroupStageError]` の分岐を追加する。段階が `GroupStageFileVerification` で `errors.AsType[*verification.Error]` が成立すれば既存の `runerrors.NewVerificationPreExecutionError` + `logging.HandlePreExecutionError` 経路（`continue` する）、それ以外は `groupStagePreExecutionError` で変換して `logging.NotifyPreExecutionError` を呼び、`groupErrs` に積む。
- [ ] `runner_test.go` に `TestRunner_PreExecutionStageNotifications` を追加する。`MockGroupExecutor` が各段階の段階エラーを返すようにし、`tu.NewLogRecorder` で `Pre-execution error notified` のレコードが 1 件、`message_type`・`error_type`・通知コンテキスト・`component`・`error_message`（`<要約文>: <原因>`）が期待どおりで、`Execute` がエラーを返すことを検証する（AC-01〜AC-07・AC-16）。段階 `GroupStageUnknown` の行も含める（AC-10）。
- [ ] 同ファイルに `TestRunner_PreExecutionStageNotificationsPerGroup` を追加する。2 つの group がそれぞれ段階エラーで失敗し、レコードが 2 件で各 group の Scope を持つことを検証する（AC-08）。group ごとに異なる段階エラーを返すには、`MockGroupExecutor` の `mock.MatchedBy` で group 名を照合するか、`ExecuteGroup` を `groupSpec.Name` で場合分けする小さなテスト用実装を使う（`mock.Anything` のままでは 1 件しか区別できない）。
- [ ] 同ファイルに `TestRunner_FileVerificationStageKeepsExistingPath` を追加する。`newGroupStageError(GroupStageFileVerification, "backup", verErr)` を返すモックで、既存の `Pre-execution error occurred` のレコードが 1 件だけで本設計のレコードは 0 件、本文・`failed_file_paths`・Scope がラップしない場合と同じであることを検証する（AC-12）。
- [ ] 同ファイルに `TestRunner_StageDispatchPrefersStageOverVerificationError` を追加する。`GroupStageCommandVerification` の段階エラーの原因が `*verification.Error` のときに、`Pre-execution error notified` がちょうど 1 件で `error_type` が `command_verification_failed`・command 水準の Scope になり、既存の `Pre-execution error occurred` は 0 件で `Execute` の戻り値がこのエラーを含むことを検証する（AC-09・§3.4 の 3b）。
- [ ] 同ファイルに `TestRunner_CancellationSkipsStageNotification` を追加する。`context.Canceled`・`context.DeadlineExceeded` をラップした段階エラーで本設計のレコードが 0 件であることを検証する（AC-14）。
- [ ] 同ファイルに `TestRunner_CommandExecutionFailureSkipsStageNotification` を追加する。`*CommandExecutionError` を返すモックで本設計のレコードが 0 件であることを検証する（AC-13）。
- [ ] 同ファイルに `TestRunner_PreExecutionErrorMessageIsRedacted` を追加する。値形式の機密（GitHub トークン形式の文字列）を含むが、`key`・`token` などの語も key=value の形も含まない原因を持つ段階エラーを返すモックを使う。`redaction.NewRedactingHandler` を通したレコーダの `error_message` について、まずその入力が値全体置換の判定にも key=value のパターンにも当たらないことを確認する。そのうえで、機密の部分だけが値形式のプレースホルダに置き換わり、他の部分は残ることを検証する（AC-19）。
- [ ] `internal/logging/slack_handler_test.go` に `TestBuildPreExecutionError_InterpolationContract` を追加する。改行・制御文字・書式制御文字（`U+202E`）を含む `error_message` と、500 byte を超える `error_message` を RedactingHandler 経由のレコードで与え、`buildPreExecutionError` の `Error Message` が 1 行で、制御文字と書式制御文字を含まず、`common.Interpolate` の上限以下であることを `assertDisplaySafeProperties` で検証する（AC-18）。
- [ ] `integration_test_helpers.go` の `slackRun` に `stdout string` を追加し、`runMainWithSlackMock`（`:280`）が `captureStdoutStderr` の stdout も返すようにする（§1.4）。
- [ ] `cmd/runner/integration_pre_execution_error_test.go` に `TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce` を追加する。`runMainWithSlackMock` と、未定義変数を複数行の値で参照する group の `env_vars` を持つ設定（`dryRun=false` 固定。`vars` では原因に改行が入らず AC-18 の 1 行化を検証できないため、§1.3）で #1 を 1 件だけ起こし、`run.stdout` の `RUN_SUMMARY` 行がちょうど 1 行、stderr の `Error:` ブロックがちょうど 1 つ、終了コードが 1、Slack のペイロードが 1 件で `error_type` が `group_preparation_failed`・Scope が `group=<group>`、最終報告のレコード（`Execution error occurred`）が `slack_notify=false` であることを検証する（AC-15・AC-16）。ペイロードの `Error Message` が 1 行であることも確認する（AC-18）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。配線テストが 3b の分岐を 3a の前に置く／段階より先に `*verification.Error` を見ると失敗すること、`TestRunner_FileVerificationStageKeepsExistingPath` が 3a を外すと失敗すること、統合テストが `NotifyPreExecutionError` を `HandlePreExecutionError` に替える（`RUN_SUMMARY` 行が増える）と失敗することを確認する。

### PR-4 作成ポイント: dispatch on the declared stage and observe it end to end

**対象ステップ**: Phase 4

**推奨タイトル**: `feat(0176): notify group pre-execution failures by declared stage`

**レビュー観点**: `executeGroups` の判定順がキャンセル → 段階 → `*verification.Error` で、段階が `GroupStageFileVerification` のときだけ既存経路に残すこと／段階エラーが `groupErrs` に積まれ、戻り値と終了コードが変わらないこと／記録のみの通知が `RUN_SUMMARY` 行と stderr 報告を増やさず、`slack_notify=true` の通知が失敗ごとに 1 件であること／本文が既存の redaction と補間契約を通ること／統合テストがプロセス全体の状態を差し替えるため `t.Parallel` を使わず `dryRun=false` であること

**実装モデル要件**: frontier-recommended

**判定理由**: 通知の振り分けという可視挙動の中核と、Slack モックサーバー・プロセス全体状態の差し替えを伴う統合テストを含むため。段階エラーの記録と振り分けを同一 PR に入れ、段階的な raise/lower を作らないので panel-mode トリガーには該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 利用者向け文書・サンプル・対象環境での確認

**対象ファイル**: `docs/user/runner_command.ja.md`、`docs/user/runner_command.md`、`docs/translation_glossary.md`、`sample/slack-group-notification-test.toml`、`Makefile`、`scripts/verification/check_pre_execution_notification_docs.sh`（新規）

**作業内容**:

- [ ] `runner_command.ja.md` の「通知設定」の「通知されるメッセージ種別」の後に、group 実行前段の失敗が `pre_execution_error` として通知されること、段階ごとの `error_type` 4 件（`group_preparation_failed`・`group_dir_permission_violation`・`command_verification_failed`・`group_pre_execution_failed`）とその意味、`group_file_verification_failed` が使われる場合、Scope の表示（`group=<group>`・`group=<group> command=<command>`）を追記する。記述は実装コードの定数（Phase 1）と Phase 4 の統合テストが観測した `error_type`・Scope を典拠にする（AC-20）。
- [ ] `runner_command.ja.md` をコミットした後、`/mktrans` で `runner_command.md` へ反映する。`/mktrans` の用語登録に従い、新しく使った用語（実行前段、段階、段階エラー、段階定義表、記録のみの通知、汎用行など）を `docs/translation_glossary.md` に登録する（AC-20）。
- [ ] `scripts/verification/check_pre_execution_notification_docs.sh` を追加する。既存の `check_identifier_exemption_docs.sh` と同じ POSIX sh の形で、(a) 4 つの `error_type` の値が `internal/logging/pre_execution_error.go` の定数として存在すること、(b) その 4 値と `pre_execution_error` が日英の `runner_command` に含まれること、を検査する。これで文書の値がコードの値に固定される。各 `error_type` の意味の文は人間のレビューを検証とし、その旨を PR-5 のレビュー観点に残す（AC-20）。`make verify-docs-checks` が `scripts/verification/check_*.sh` を列挙して実行するため、新しい検査は自動で走る。
- [ ] `sample/slack-group-notification-test.toml` に、実行前段で失敗する group（例: 未定義変数を参照する group 変数）を 1 件加える。`Makefile` の `slack-group-notification-test`（`:666-675`）の期待通知の一覧とログの確認項目に、その group の `pre_execution_error` 通知と新しい `error_type` を加える。
- [ ] `make verify-docs-checks` と `make test`（docsguard を含む）が通ることを確認する。
- [ ] `make slack-group-notification-test` で対象環境の表示を確かめ、実行前段の失敗の通知が 1 件届き、見出しが `error_type` であることを確認する（AC-20 の manual）。

**完了条件**: `make verify-docs-checks`・`make test`・`make lint` が通る。`scripts/verification/check_pre_execution_notification_docs.sh` が、日英どちらかから `error_type` の記述を外すと非ゼロで終了することを確認する。

### PR-5 作成ポイント: document the pre-execution notification and verify on the target environment

**対象ステップ**: Phase 5

**推奨タイトル**: `docs(0176): document the group pre-execution failure notification`

**レビュー観点**: 日英の記述が Phase 4 の実装・統合テストが観測した `error_type` と Scope に一致すること／`error_type` の意味の文がコードの段階定義表と矛盾しないこと（スクリプトは値をコードに固定するが意味の文はここでレビューする。§7 の AC-20）／`/mktrans` の用語が `translation_glossary.md` に登録されていること／日英の `runner_command` の見出し構造が一致すること／`check_pre_execution_notification_docs.sh` が `make verify-docs-checks` から実行されること／`sample` と `Makefile` の期待通知が実際に届く通知と一致すること

**実装モデル要件**: standard

**判定理由**: 文書の追記と翻訳、サンプル設定と確認手順の更新が中心で、記述の典拠は Phase 1 の定数と Phase 4 の統合テスト出力に固定されている。Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 通知の部品 | Phase 1 | `NotifyPreExecutionError` と `error_type` 4 件が green |
| M2: 段階の型 | Phase 2 | 段階定義表の網羅テストと構築ガードが green |
| M3: 段階の宣言 | Phase 3 | #1〜#7 の段階テストと出口規則のテストが green。既存の `errors.Is` テストが変更なしで green |
| M4: 通知の配線 | Phase 4 | 配線テスト・本文の安全性・統合テストが green |
| M5: 文書と対象環境 | Phase 5 | 日英の `runner_command` 更新、`make verify-docs-checks` green、`make slack-group-notification-test` で表示確認 |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `error_type` 4 件、`NotifyPreExecutionError`、記録部分の共有 | standard |
| PR-2 | Phase 2 | `GroupStage`・`GroupStageError`・構築関数・段階定義表・変換関数・構築ガード | standard |
| PR-3 | Phase 3 | #1〜#7 の段階宣言、#3 の接頭辞移動、#7 の原因ラップ、出口規則 1・2 | frontier-recommended |
| PR-4 | Phase 4 | `executeGroups` の振り分け、配線テスト、本文の安全性、統合テスト | frontier-recommended |
| PR-5 | Phase 5 | 日英の利用者向け文書、用語集、サンプル設定、`Makefile`、doc 検査スクリプト | standard |

PR-1 → PR-2 → PR-3 → PR-4 の順に依存する。Phase 1 の `error_type` が無ければ Phase 2 の表が作れず、Phase 2 の型が無ければ Phase 3 が段階エラーを返せず、Phase 3 の段階エラーが無ければ Phase 4 の配線テストが組めない。Phase 3 までは通知の挙動が変わらない（段階エラーを作るが `executeGroups` がまだ読まない）。Phase 5 は Phase 4 が観測した実際の表示を記述の典拠にする。

### 3.3 順序の根拠

[02_architecture.md](02_architecture.md) §8 の 1 → 2 → 3 → 4 → 5 をそのまま保つ。§8 が Phase 4 にまとめるテストのうち、コンポーネント単位のもの（`group_stage_test.go`・`group_executor_test.go`・`pre_execution_error_test.go`・`slack_handler_test.go`・`runner_test.go`）は、それが固定する実装と同じ Phase に置く。これにより、実装と検証を同じ PR のレビュー対象にし、段階宣言の忘れやガードの無効化を導入直後に検出する。Phase 4 には複数コンポーネントを跨ぐ統合テストだけを残す。

---

## 4. テスト戦略

### 4.1 単体テスト

- **通知の部品**: `TestNotifyPreExecutionError_RecordsWithoutReport`（Phase 1、AC-15・AC-17）。
- **段階の型と表**: `TestGroupStageTableHasARowForEveryStage`・`TestGroupStagePreExecutionErrorMapping`・`TestGroupStageUnknownAndOutOfRangeUseGenericRow`・`TestGroupStageErrorZeroValueDoesNotPanic`・`TestGroupStagePreExecutionErrorUsesDeclaredStageNotReasonText`・`TestGroupStageConstructorsPanicOnInvalidInput`・`TestGroupStageErrorUnwrapsCause`（Phase 2、AC-01〜AC-10）。
- **構築ガード**: `TestProductionGroupStageErrorLiteralsUseConstructors`（Phase 2、§1.3）。
- **発生箇所と出口**: `TestExecuteGroup_PreExecutionStageErrors`・`TestExecuteGroup_PreExecutionExitRules`・`TestExecuteGroupRegistersExitDefer`・`TestExecuteGroup_CommandExecutionFailureHasNoStageError`（Phase 3、AC-01〜AC-07・AC-10・AC-11・AC-13・AC-18）。
- **配線と振り分け**: `TestRunner_PreExecutionStageNotifications`・`TestRunner_PreExecutionStageNotificationsPerGroup`・`TestRunner_FileVerificationStageKeepsExistingPath`・`TestRunner_StageDispatchPrefersStageOverVerificationError`・`TestRunner_CancellationSkipsStageNotification`・`TestRunner_CommandExecutionFailureSkipsStageNotification`・`TestRunner_PreExecutionErrorMessageIsRedacted`（Phase 4、AC-08・AC-09・AC-12〜AC-14・AC-16・AC-19）。
- **ビルダーの補間契約**: `TestBuildPreExecutionError_InterpolationContract`（Phase 4、AC-18）。

### 4.2 層の切り分け

- AC-18 は 2 層で確認する。Phase 3 のテストが、#1/#3 の原因の文言に生の改行が入ることと、#6/#7 のパスが `%q` で引用されること（引用が効く経路）を固定する。ビルダーが引用を受けない入力と引用済みの入力の両方を 1 行・上限内に収めることは、`internal/logging` の `TestBuildPreExecutionError_InterpolationContract` が固定する。両者を組み合わせて初めて端から端の性質が成り立つ。
- AC-19 は、`key`・`token`・`=` を含まない値形式の機密を入力にして、値全体置換や key=value 置換ではなく値形式検出だけが働くことを確認する。入力が他の層に当たらないことをテスト内で先に確認する。
- AC-12 は、モックが生の `*verification.Error` を返す既存テスト（`TestRunner_VerificationErrorCarriesGroupScopeAndCleanMessage`・`TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent`）と、`GroupStageFileVerification` でラップした `*verification.Error` を返す新規テストの両方で固定する。前者は分岐 4、後者は分岐 3a を通る。

### 4.3 統合テスト

`cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce` が、`runMainWithSlackMock` で #1 を起こし、stdout の `RUN_SUMMARY` 行数・stderr の `Error:` ブロック数・終了コード・Slack ペイロードの件数と内容・最終報告の `slack_notify=false` を観測する。`runMainWithSlackMock` はプロセス全体の状態を差し替えるため `t.Parallel` を使わない。

### 4.4 実装時に行うテスト失敗確認（AC-22）

各 Phase の「完了条件」に列挙した変異を実施し、当該テストが失敗することを確認して、コミットメッセージに記す。特に次を落とさない。

| Phase | 変異 | 失敗するテスト |
|---|---|---|
| 1 | `NotifyPreExecutionError` に stderr 書き出しを足す／`slack_notify` の設定を外す／メッセージを `HandlePreExecutionError` と同一にする | `TestNotifyPreExecutionError_RecordsWithoutReport` |
| 2 | 段階定義表の行を 1 つ削る／範囲外の索引を専用行に変える | `TestGroupStageTableHasARowForEveryStage`・`TestGroupStageUnknownAndOutOfRangeUseGenericRow` |
| 2 | `group_executor.go` に `&GroupStageError{...}` を置く／値形 `GroupStageError{...}` を置く／elided 形（`[]*GroupStageError{{...}}`）を置く／`.stage`・`.group`・`.command`・`.err` の各フィールドへ代入する（構築形ごとに 1 回ずつ） | `TestProductionGroupStageErrorLiteralsUseConstructors` |
| 3 | 各発生箇所の段階宣言を外す | `TestExecuteGroup_PreExecutionStageErrors`（当該行） |
| 3 | 出口規則 1 を外す／規則 2 を無効化する | `TestExecuteGroup_PreExecutionExitRules` |
| 3 | deferred 出口の登録を削除する／#1 の `return` より後ろへ移す | `TestExecuteGroupRegistersExitDefer` |
| 4 | 3b の分岐を 3a の後ろに置く／段階より先に `*verification.Error` を見る | `TestRunner_StageDispatchPrefersStageOverVerificationError` |
| 4 | 3a を外す | `TestRunner_FileVerificationStageKeepsExistingPath` |
| 4 | `NotifyPreExecutionError` を `HandlePreExecutionError` に替える | `TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce`（`RUN_SUMMARY` 行数） |
| 4 | 変換関数の `NotificationContext` を組み立てない | `TestRunner_PreExecutionStageNotifications`（通知コンテキスト） |
| 5 | 日英どちらかから `error_type` の記述を外す | `scripts/verification/check_pre_execution_notification_docs.sh`（`make verify-docs-checks`） |

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| `ExecuteGroup` の戻り値の連鎖に `*logging.PreExecutionError` が混入する | `main.go:233` の `errors.As` が先に成立し、Slack に 2 件目が届く | 変換した `PreExecutionError` は `executeGroups` の中の通知にだけ使う。統合テストでペイロード 1 件と最終報告 `slack_notify=false` を固定する（§3.4、AC-16） |
| 段階より先に `*verification.Error` を判定する | #6/#7 が `group_file_verification_failed` に誤って振り分けられ、`groupErrs` から外れて終了コードが変わる | 判定順をキャンセル → 段階 → `*verification.Error` に固定し、`TestRunner_StageDispatchPrefersStageOverVerificationError` で固定する（§3.4） |
| #3 の接頭辞の移動で文言が二重になる | 最終報告と通知本文に `failed to pre-expand` が 2 回現れる | `TestExecuteGroup_PreExecutionStageErrors` の #3 で `Error()` を確認する。§1.3 の既存テストは部分一致のため検出できない |
| #7 の文言変更が既存テストや dry-run プレビューを壊す | 既存テストの失敗、`SetDryRunExecutionError` の期待値の不一致 | 既存テストは部分一致で判定している（`internal/runner/e2e_dynlib_verification_test.go:95` など）。§3.3.4 の 1 接頭辞だけを加え、`slog.Error` と終了コードは変えない |
| 変換関数の `Component` を表から読む／生リテラルにする | 既存の `TestProductionErrorLiteralsUseTypedComponent` が失敗する | `Component` は `string(resource.ComponentRunner)` と字句どおり書き、段階の区別は `error_type` が担う（§3.2.3） |
| 構築ガードの走査が広すぎて無関係な型・テストに誤反応する | ガードの偽陽性、テストの vacuous pass | 走査は `identitymutationguard.ProductionGoFilesInRepo` の本番ファイルに限り、対象を `GroupStageError` の複合リテラルと 4 フィールドのセレクタ代入に限定する。セレクタ代入は `internal/runner` 直下のファイルだけで検査する（§1.3） |
| 統合テストがプロセス全体の状態を差し替える | 並列実行時の干渉 | 当該テストは `t.Parallel` を呼ばない。`runMainWithSlackMock` の既存の退避・復元に従う |
| 通知が増えることで高優先度キューが満杯になる | 通知の取りこぼし | 既存の容量と flush の判断を変えない（§5.5）。本計画では容量を変更しない |
| 本文の group 名・コマンド名・パスが値全体置換で `[REDACTED]` になる | 原因が読めない通知 | redaction の範囲は変えない。テストは、値形式検出だけが働く入力で機密の部分だけが置き換わることを固定する。運用上の扱いは §5.2 のとおり |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2。構築ガード green）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3。既存の `errors.Is` テストが変更なしで green）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4。配線テスト・本文の安全性・統合テスト green）
- [ ] PR-5 マージ済み（対象ステップ: Phase 5。`/mktrans` 済み、`make verify-docs-checks` green、`make slack-group-notification-test` 確認済み）
- [ ] すべての AC が §7 の検証で green
- [ ] §4.4 の変異確認をすべて実施し、各コミットメッセージに記録

---

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（ガードテスト・`make` ターゲット・コミット済みスクリプト）、`manual`（PR やデプロイでの観察）を表す。テスト名は `path::TestName` で示す。

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 3、Phase 4 | `test`: `internal/runner/group_executor_test.go::TestExecuteGroup_PreExecutionStageErrors`（#1 の行）、`internal/runner/runner_test.go::TestRunner_PreExecutionStageNotifications`（`group_preparation_failed`・`group=<group>`） |
| AC-02 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#2 の行）、`TestRunner_PreExecutionStageNotifications` |
| AC-03 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#3 の行。`GroupStageCommandPreparation`・コマンド名）、`TestRunner_PreExecutionStageNotifications`（`group=<group> command=<command>`） |
| AC-04 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#4 の行）、`TestRunner_PreExecutionStageNotifications`（`group_dir_permission_violation`） |
| AC-05 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#5 の行。`GroupStageFileVerification`）、`TestRunner_PreExecutionStageNotifications`（`group_file_verification_failed`） |
| AC-06 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#6 の行。`Error()` に解決できなかったコマンドのパス）、`TestRunner_PreExecutionStageNotifications`（`command_verification_failed`） |
| AC-07 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#7 の行。コマンドのパスと原因の両方、`%q` の引用）、`TestRunner_PreExecutionStageNotifications` |
| AC-08 | Phase 4 | `test`: `TestRunner_PreExecutionStageNotificationsPerGroup` |
| AC-09 | Phase 2、Phase 4 | `test`: `group_stage_test.go::TestGroupStagePreExecutionErrorUsesDeclaredStageNotReasonText`、`TestRunner_StageDispatchPrefersStageOverVerificationError`（段階が `*verification.Error` より先） |
| AC-10 | Phase 2、Phase 3、Phase 4 | `test`: `group_stage_test.go::TestGroupStageUnknownAndOutOfRangeUseGenericRow`・`TestGroupStageErrorZeroValueDoesNotPanic`、`group_executor_test.go::TestExecuteGroup_PreExecutionExitRules`（規則 1）、`runner_test.go::TestRunner_PreExecutionStageNotifications`（`GroupStageUnknown` の行）。`static`: `group_executor_test.go::TestExecuteGroupRegistersExitDefer`（最初の `return` より前に deferred 出口を登録し、named return を出口関数に通すこと） |
| AC-11 | Phase 3 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（#1〜#7 の全行） |
| AC-12 | Phase 4 | `test`: `TestRunner_FileVerificationStageKeepsExistingPath`、既存の `TestRunner_VerificationErrorCarriesGroupScopeAndCleanMessage`・`TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent` |
| AC-13 | Phase 3、Phase 4 | `test`: `group_executor_test.go::TestExecuteGroup_CommandExecutionFailureHasNoStageError`・`TestExecuteGroup_PreExecutionExitRules`（規則 2）、`runner_test.go::TestRunner_CommandExecutionFailureSkipsStageNotification` |
| AC-14 | Phase 4 | `test`: `TestRunner_CancellationSkipsStageNotification` |
| AC-15 | Phase 1、Phase 4 | `test`: `internal/logging/pre_execution_error_test.go::TestNotifyPreExecutionError_RecordsWithoutReport`（報告出力なし）、`cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce`（`RUN_SUMMARY` 1 行・`Error:` 1 つ） |
| AC-16 | Phase 4 | `test`: `TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce`（終了コード・ペイロード 1 件・`slack_notify=false`）、`TestRunner_PreExecutionStageNotifications`（`Execute` がエラーを返す） |
| AC-17 | Phase 1、Phase 2 | `static`: `internal/logging/notification_contract_guard_test.go::TestProductionCodeSetsSlackNotifyOnlyInNotificationAttrs`・`TestProductionPreExecutionErrorLiteralsCarryNotificationContext`・`TestProductionErrorLiteralsUseTypedComponent`、`internal/runner/runerrors/pre_execution_guard_test.go::TestFiringPointsUseSharedVerificationConstructor`。`test`: `TestNotifyPreExecutionError_RecordsWithoutReport` |
| AC-18 | Phase 3、Phase 4 | `test`: `TestExecuteGroup_PreExecutionStageErrors`（改行を含む原因・`%q` の引用）、`internal/logging/slack_handler_test.go::TestBuildPreExecutionError_InterpolationContract`、`TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce`（ペイロードの `Error Message` が 1 行） |
| AC-19 | Phase 4 | `test`: `TestRunner_PreExecutionErrorMessageIsRedacted`（値形式検出だけが働く入力） |
| AC-20 | Phase 5 | `static`: `make verify-docs-checks`（`scripts/verification/check_pre_execution_notification_docs.sh`）。`manual`: `make slack-group-notification-test` で表示確認 |
| AC-21 | 各 Phase | `static`: 各 Phase の `make fmt`・`make test`・`make lint` |
| AC-22 | 各 Phase | `manual`: §4.4 の表に従い変異確認を実施し、コミットメッセージに記録する。`test`: 変異確認の対象は §4.4 の各テスト |

---

## 8. 横断検索チェックリスト

`make test`・`make lint` が検出できない残存参照だけを挙げる。§7 の表と重複する項目は置かない。

- [ ] `docs/translation_glossary.md` に、Phase 5 で新しく使った用語（実行前段、段階、段階エラー、段階定義表、記録のみの通知、汎用行）の対訳が `/mktrans` により登録されていること。
- [ ] `docs/user/runner_command.ja.md` と `runner_command.md` の見出し構造が一致すること（`/mktrans` の反映後に `make verify-docs` を実行して確認する。`run_all.sh` は構造比較の結果を報告する）。
- [ ] `sample/slack-group-notification-test.toml` と `Makefile` の期待通知の一覧が、Phase 5 で追加した実行前段の失敗の group と一致すること。
- [ ] `docs/dev/architecture_design/security-architecture.md` と `docs/dev/developer_guide/package_reference.md` に、新しい `error_type` や `group_stage.go` の記述を足して不整合を作っていないこと（本書の対象外であり、変更しない）。

---

## 9. Success Criteria

- **機能**: AC-01〜AC-08・AC-10〜AC-16・AC-18〜AC-20 を検証するテスト・スクリプトが green。AC-09 が段階による振り分けと `GroupStageUnknown` の汎用通知で green。
- **品質**: 各 Phase の `make fmt`・`make test`・`make lint` が green。§4.4 の変異確認をすべて実施し記録済み。既存の `errors.Is` テストと静的ガードが変更なしで green。
- **セキュリティ**: 通知の `error_message` が `RedactingHandler` の redaction を受け、値形式検出だけが働く入力でも機密の部分だけが置換されること、ビルダーの `Error Message` が 1 行・制御文字なし・上限以下であることがテストで観測される。段階不明でも通知が落ちない。
- **一貫性**: `message_type` の登録（3 種別）・`pre_execution_error` のビルダーとフィールド集合・共通エンベロープ・エラー文言（#7 の 1 接頭辞を除く）が変わっていない。
- **文書**: `runner_command.ja.md` に group 実行前段の通知・`error_type` 4 件・Scope が記載され、`runner_command.md` に `/mktrans` で反映され、`make verify-docs-checks` が green。

---

## 10. 次のステップ

- 本書が承認されたら、Phase 1（PR-1）から順に実装を開始する。
- 実装完了後、実チャンネルで group 実行前段の失敗の Slack 表示を確認する（手動。AC-20 の manual）。
- 本文中の group 名・コマンド名・パスを構造化属性へ分ける改善（§3.7）と、値全体置換の誤検出を減らす改善（§5.2）は別タスクの候補とする。
