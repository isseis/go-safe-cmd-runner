# 実装計画書: エラー本文を役割付きの部分として運び、部分ごとに redaction する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-28 |
| Review date | 2026-09-29 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 詳細仕様書: [03_detailed_specification.md](03_detailed_specification.md)
- 設計への引き継ぎ: [design_carryover.md](design_carryover.md)
- 詳細仕様書への申し送り: [detailed_spec_carryover.md](detailed_spec_carryover.md)
- 要件・受け入れ基準プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テストヘルパ配置: [test_organization.md](../../dev/developer_guide/test_organization.md)
- セキュリティ設計: [security-architecture.ja.md](../../dev/architecture_design/security-architecture.ja.md)

本書の用語は [02_architecture.md](02_architecture.md) §0 に従う（構造化メッセージ、部分、役割、免除の役割、平らにする、断片、全体の検出範囲、対象の範囲、置換文字列、値全体置換）。設計の判断は 02、型・シグネチャ・箇所ごとの並び・ガードの判定は 03 を参照し、本書では重複して書かない。

**実装計画のファイル名について**: 本タスクは詳細仕様書を `03_detailed_specification.md` に持つため、[03_detailed_specification.md](03_detailed_specification.md) §0.1 #1 の確定に従い、実装計画を `04_implementation_plan.md` とする（Task 0091 と同じ 4 文書の番号付け）。

---

## 1. 実装の概要

### 1.1 目的

対象のエラー本文を、役割を型で宣言した部分の列（構造化メッセージ）として運び、`RedactingHandler` が部分ごとに役割に応じた redaction を適用してから 1 本の文字列に描画する。これにより、機密ではない group 名・コマンド名・変数名・パスを含むだけで本文全体が `[REDACTED]` になることを防ぐ。秘密の保護は弱めず、構造を持たないエラーは現状と同じ保護を受ける（fail-closed）。`Error()`・stderr・通知の種別と構成は変えない。

設計の全体像は [02_architecture.md](02_architecture.md) §1〜§6、変更する観察可能な挙動は 01「変更の効果」を参照する。

### 1.2 実装方針

1. 役割は `internal/errmsg` の構築関数でしか決められない。`redaction`・通知ビルダー・ログ出力は断片の役割を読むだけにする（02 §1.1、§3.1.1 の契約 1）。
2. 文言は構造から作る。`Structured` を実装する型の `Error()` は `StructuredMessage().String()` を返す 1 文とし、AST ガードで形を固定する（02 §1.1、§4.3）。
3. `RedactText` は変えない。全体の検出範囲は同じ規則を共有する別の関数で求め、結果が `RedactText` と一致することを差分テスト・ファジング・実行時の検査で確かめる（02 §3.2.1）。
4. 値全体置換は `Text` の断片ごとに判定し、部分の境界をまたぐ秘密は 01 の契約（`Identifier` のバイトは残し、それ以外を置換する）で扱う（01 決定事項、02 §3.2.2）。
5. 対象の範囲（AC-41 の検証対象）は 02 §3.8.1 / 03 §9.1 の表を唯一の定義とし、範囲内の `fmt.Errorf`・`errors.Join`・非定数 `errors.New` を AST ガードで拒否する。
6. 実装の手順は 02 §8 / 03 §13 の 8 項目をそのまま Phase 1〜8 とする。
7. 各 Phase の完了時に `make fmt`（Go を変更した場合）・`make test`・`make lint` を通す（AC-27）。
8. Go のソースコメント・識別子・文字列リテラルは英語で書く。

### 1.3 既存コード調査結果

調査基準: コミット `e3f7dfdc`（HEAD）。`git diff --stat 3bb634bd..HEAD -- '*.go' Makefile .golangci.yml` は空である。03 の `file:line` を要所で確認し、見つけた食い違いは下の食い違い表に記録した。`internal/errmsg` は存在せず、本番コードに `errmsg` への参照は無い（確認済み）。`internal/runner/base`・`internal/runner/resource`・`internal/runner/base/privilege` は `3bb634bd` と同一である。

#### 変更対象の現状

| 領域 | 現状 | 変更 |
|---|---|---|
| `internal/errmsg` | 存在しない | 03 §2 の型・構築関数・平らにする処理・`Join`・`Freeze` を新設 |
| `internal/redaction` | `RedactText`（`redactor.go:272-307`）は PEM → `compiled` の key=value → `ValueDetector.Mask` の順。`Mask` は状態を変更しない。`WithPlaceholder`・`WithAdditionalKeyValuePatterns` は本番の呼び出しが無く、テストだけが使う | 範囲を返す `redactedRanges` と `RedactMessage` を追加。`RedactText` は変えない。2 つのオプションを削除 |
| `internal/logging` | `PreExecutionError.Message`・`ExecutionError.Message` は文字列。`errorRecordParams.errorMsg` は文字列、`writeErrorLogRecord` は `slog.String`（`pre_execution_error.go:187`）。`handleErrorCommon`（`:148-174`）が stderr の `Details:` を作る | 2 つの `Message` の型、`DetailMessage`・`ReportMessage`、記録と凍結を 03 §4 のとおりに変更 |
| `internal/runner` | `GroupStageError`・`GroupError`・`GroupErrors`・`CommandExecutionError` は `Structured` 未実装。`group_executor.go` に `fmt.Errorf` が 11 か所。`executeGroups`（`runner.go:406-476`）は `errors.Join(ctxErr, err)`（`:452`）を返す | 4 型の `Structured`、11 か所の構造化、`cancelledRunError`、段階の定義表の `Summary` 化 |
| `internal/runner/config` | `ErrUndefinedVariableDetail.Level`・`Field` は文字列（`errors.go:258-264`）。`Level`・`Field` 型は無い。`expansion.go` の `fmt.Errorf` は 28 か所（うち対象 16 か所、`ExpandWorkDir` 2 か所、除外関数 10 か所） | `Level`・`Field` の型、`ErrUndefinedVariableDetail` の `Structured`、16 か所と `ExpandWorkDir` の構造化 |
| `internal/runner/resource` | 対象 4 関数に `fmt.Errorf` が 16 か所（03 §7.1 の表のとおり） | 16 か所を構造化 |
| `internal/runner/base/executor` | `tempdir_manager.go` の 2 つのラップ（`:77`・`:90`）、`executor.go` の 12 か所、`command_lifecycle.go` の `errors.Join`／`fmt.Errorf` 11 か所 | 03 §7.2 のとおりに構造化。`killAfterCancelError` を新設 |
| `internal/runner/base/privilege` | `(*Error).Error()` は `"privilege operation '%s' failed for command '%s' (uid %d->%d): %v"`（`errors.go:34-37`） | `StructuredMessage` を追加し `Error()` をそこから作る。`performElevation`（`unix.go:217`）を構造化 |
| `cmd/runner` | 原因を `Message` に埋め込む 4 か所（`PreExecutionError{` の行が `main.go:372`・`:383`・`:512`・`:640`、その `Message` の行が `:374`・`:385`・`:514`・`:642`） | 原因を `Err` に移し、`Message` を `ConstSummary` にする。ほか 11 か所の要約文も型を合わせる |
| 文書 | security-architecture の日英、`package_reference.md`、`scripts/verification/check_identifier_exemption_docs.sh` | 03 §11 のとおりに更新・追記 |

#### 削除するシンボルと全出現箇所

`rg` で本番（`internal/`・`cmd/` の `*.go`、`_test.go` を除く）とテストの両方を列挙した（`e3f7dfdc`）。

| シンボル | 全出現箇所 | 対応 |
|---|---|---|
| `WithPlaceholder` | 定義 `internal/redaction/redactor.go:155-159`、コメント `:18`・`:210`、テスト `redactor_test.go:1728`・`:3352`・`:3767`。本番の呼び出しは無い | 定義とコメントを削除。テストは下の削除台帳に従う |
| `WithAdditionalKeyValuePatterns` | 定義 `internal/redaction/redactor.go:169-173`、テスト `redactor_test.go:645`・`:654`・`:1574`・`:1712`・`:1721`。本番の呼び出しは無い | 定義を削除。テストは下の削除台帳に従う |

`Config.Placeholder()`（`redactor.go:64-66`）は残す。本番の呼び出しが `internal/runner/base/security/environment_validation.go:21` にあるためである（03 §3.5）。`NewConfig`（`:200-236`）のパターン検証・コンパイル失敗の分岐も残す（fail-closed の境界。03 §3.5）。

#### テストの削除・書き換え台帳

削除するものは、挙動または型が変わるために更新が要る既存テストである。削除の後、`go tool cover -func` を関数ごとに比較し、差をコミットメッセージに記録する（CLAUDE.md「Deleting a test is a claim that must be checked」）。

削除前の基準値（`e3f7dfdc`、`go tool cover -func`）: `internal/redaction` の合計 87.3%。`WithPlaceholder` 100%・`WithAdditionalKeyValuePatterns` 100%・`NewConfig` 95.0%・`compilePattern` 94.7%・`apply` 100%・`RedactText` 100%・`replaceKeyValueMatches` 93.8%。

| 対象 | 扱い | 理由 |
|---|---|---|
| `redactor_test.go::TestNewConfig_RejectsInvalidPatterns` | 関数ごと削除（subtest `valid addition is accepted and applied`・`invalid addition is rejected`・`placeholder reaches both redaction layers` の 3 つ） | 2 つのオプションを削除すると、関数の全 subtest が対象を失う。パターンの検証そのものは `TestKeyValuePattern_Validate` に残る |
| `redactor_test.go::TestKeyBoundaryGroup_Classification/user-added key is redacted with the loose boundary` | subtest を削除 | 利用者がキーを足す経路が無くなる |
| `redactor_test.go::TestPerformKeyValueRedaction/unknown kind never reaches the redaction path` | 後半（`NewConfig` を通す部分）を削除し、`compilePattern` での拒否だけを残す | 足す経路が無くなる |
| `redactor_test.go::TestKeyBoundaryGroup_Classification/zero value of PatternKind is the key kind` | `applyPattern`（`:48`）で `KeyValuePattern{Literal: "passphrase"}` を直接コンパイルする形に書き換え | ゼロ値が key の規則になることは残す |
| `redactor_test.go::TestRedactText_ValueBasedDetection_BypassWhenNil` | `NewConfig()` の既定の置換文字列に替え、期待値を `password=[REDACTED] value AKIAIOSFODNN7EXAMPLE` に改める | 置換文字列はこのテストの主眼ではない |
| `redactor_test.go::TestNewConfig_WithWebhookHost` の subtest `the placeholder option reaches the configured-host pattern` | subtest を削除 | オプションを確かめるためだけのテスト |

削除の後、`NewConfig` のパターン検証・コンパイル失敗の分岐は既定の規則だけでは届かなくなる。分岐を残す判断（03 §3.5）と、その分の網羅率の差（`NewConfig` の 95.0% からの低下）をコミットメッセージに記録する。`TestNewConfig_RejectsInvalidPatterns` が固定していた「利用者が足した不正な規則を構築時に拒否する」という不変条件は、規則を足す経路の削除とともに無くなる（受け入れる）。残す構築時の拒否の分岐は、既定の規則が不正な形に編集されたときの fail-closed の備えであり、その前提（既定の規則が妥当であること）は `TestDefaultKeyValuePatterns_AreValid` が固定する。`Placeholder()` の網羅率は `environment_validation_test.go:232` が通るので残る。

#### 構築経路と検証の扱い

- `errmsg.Part` は末端のパッケージ `internal/errmsg` に置き、欄を非公開にする。同じパッケージの構築関数だけが部分を作れる。パッケージ外の複合リテラル（値形・ポインタ形・`[]errmsg.Part{{...}}` のような省略形）は `errmsg_guard_test.go`（03 §9.8）で拒否する。この選択により、型・欄の公開形を変えずに構築経路をコンパイル時に限定できる。
- `errmsg.Message`・`Summary`・`JoinedError` は欄が非公開であり、`errmsg` の外ではゼロ値しか作れない。ゼロ値は panic せず空文字列を描画する（03 §2.8）。
- `errmsg.Segment` は欄を公開する。redaction の側が役割を読むためと、AC-38 のテストが範囲外の役割を直接与えるために必要である（03 §2.2、§10.3）。`Role` の値を redaction の側で作らないことは `redaction_guard_test.go` で固定する（03 §9.6）。
- `config.Level`・`Field` は欄を非公開にし、パッケージ内の構築関数で作る。`ExpandWorkDir` の呼び出し側だけが公開の `GroupLevel`・`CommandLevel` を使う（03 §6.1）。`parts()` は非公開で、返すのは `errmsg.Part` であるため、03 §9.8 の「部分を返す非公開の関数は対象の範囲の中にある」の対象になる。
- `cancelledRunError`（`internal/runner`）・`killAfterCancelError`（`internal/runner/base/executor`）はどちらも非公開の型で、構築はそれぞれのパッケージの中だけである。`JoinedError` と同じく `Unwrap() []error` を宣言するが、0177 のガードが禁じるのは形による判定のほうであり両立する（02 §3.4.3、03 §9.7）。

#### 再利用する既存実装・テスト・ヘルパ

| 対象 | 位置 | 使い方 |
|---|---|---|
| 本番ファイルの AST 走査 | `internal/testutil/identitymutationguard`（`ProductionGoFilesInRepo`・`ReadProductionSource`・`ParseSource`・`ResolveLocalImports`・`UnwrapParen` など） | 3 つの新しいガードテスト。走査は `internal` と `cmd` を対象にし、`_test.go` と `//go:build test` のファイルを除く（`helpers.go:160-181`・`:215-244`） |
| ガードの自己テストの形 | `internal/runner/group_errors_guard_test.go::TestMultiErrorShapeProbeCheckRecognizesForms`（`:419`） | 検出器に検出すべき形を与える表。各ガードに同じ形で付ける |
| ログレコーダ | `internal/testutil`（`tu.NewCallbackHandler` など） | `redaction`・`logging` のハンドラテスト |
| Slack モック実行 | `cmd/runner/integration_test_helpers.go`（`runMainWithSlackMock`・`slackRunSpec`・`jsonLogRecords`）と `requireSinglePreExecutionError`・`attachmentField` | AC-12・AC-34 の端から端までのテスト |
| 複数 group の帰属テストの土台 | `internal/runner/multi_group_error_integration_test.go`（`captureExecutionErrorReport`・`executionErrorMessage`） | AC-16 のテスト。`captureExecutionErrorReport` は内側のコールバックハンドラを `redaction.NewRedactingHandler` で包み、`Text` の対照値を含む記録にする |
| 中断のテストの土台 | `internal/runner/runner_test.go`（`TestRunner_ExecuteGroupsCanceledChildFailureIncludesContextCanceled` ほか） | AC-31・AC-32 のテスト |
| 一時ディレクトリのテスト | `internal/runner/base/executor/tempdir_manager_test.go` | 2 つのラップの文言のテストを追加 |
| 実行の失敗の経路のテスト | `executor_supervise_test.go:433`（`spent_command_stays_not_started`）・`executor_fdexec_test.go:111`・`executor_lifecycle_test.go:815` | `errmsg.Join` と `killAfterCancelError` の到達性の確認 |
| 補間契約のテスト | `internal/logging/slack_handler_test.go::TestBuildPreExecutionError_InterpolationContract`（`:2241`） | AC-22 の確認（変更しない） |

#### 更新が必要な既存テスト

- `internal/logging/pre_execution_error_test.go`: `Message` に文字列を渡す `PreExecutionError` 16 か所と `ExecutionError` 7 か所を `errmsg.ConstSummary`／`errmsg.TextSummary` に機械的に書き換える。`TestHandleExecutionError_CauseFormatting`・`TestHandleExecutionError_DoesNotNotifySlack`・`TestHandlePreExecutionError_SlackNotification` が記録を `attr.Value.String()` で読むが、`slog.Value.String()` は `LogValuer` を `fmt` 経由で解決するので読み出す文字列は変わらない見込みである。実行して確かめる。
- `cmd/runner/main_test.go`: `Message` のリテラル 1 か所（`:104-109`）。`TestStartupDirPermAudit_CheckerInitFailureReturnsPreExecutionError`（`:717`）は `:730` で `preExec.Message` が原因の文言を含むことを確かめている。原因が `Err` に移るので、`errors.Is` と `Detail()` の確認に変える。`errCheckerUnavailable` は `:473` にある。
- `cmd/runner/integration_slack_flush_test.go`: `Message` のリテラル 1 か所（`:78-84`）。
- `internal/runner/runerrors/pre_execution_guard_test.go`（7 か所）と `internal/logging/notification_contract_guard_test.go`（19 か所 + `ExecutionError` 3 か所）の `PreExecutionError{` は、いずれも自己テストの入力文字列の中のテストフィクスチャであり、実際の複合リテラルではない。`Message` を設定していないので書き換えは不要。実行して変化が無いことを確かめる。
- `internal/runner/config/errors_test.go:71-76`（`TestErrUndefinedVariableDetail_Unwrap`）の `Level: "global"`・`Field: "env"` を `globalLevel()`・`envField()` にする。`ErrUndefinedVariableDetail` の `Level`・`Field` を文字列として比べる既存テストは無い（`rg` で確認）。
- 引数の型が変わる関数を直接呼ぶ既存テストは、引数を構築関数に合わせる。`expansion_unit_test.go` は `Level`・`Field` を文字列で渡していたためコンパイルできなくなる。`Level` は `GroupLevel`・`CommandLevel` しか公開しない（03 §6.1）ので、`globalLevel()` などの非公開の構築関数を使うために `package config` へ移す（`config.` の接頭辞を外す）。`config_test.go`・`validation_test.go`・`template_expansion_validation_test.go` は同じパッケージ内なので、`Level`・`Field` を型にし、`"global"` は `globalLevel()`、`"group[test]"` は `groupLevel("test")`、`"cmd"` は `cmdField()`、`"vars"` は `varsField()` にする。`internal/runner/group_executor_test.go` は `config.ExpandString` を直接呼ぶので、`config.CommandLevel(...)` とゼロ値の `config.Field{}` を渡す（外部パッケージから作れる `Field` はゼロ値だけである）。
- `internal/runner/group_executor_test.go::TestExecuteGroup_PreExecutionStageErrors` の `wantIdentifiers` は、原因が構造化されるとその `Identifier` の断片が増える。`ErrUndefinedVariableDetail` を原因に持つ 4 行（group expansion・group workdir resolution・command expansion・command workdir resolution）の期待値を、`Level.parts` の group/command 名と `VariableName` の分だけ増やす（コメントの「later phase ... adds its own Identifier segments here」のとおり）。
- `internal/runner/group_stage_test.go`: `:33` の `def.message` の非空確認と、`:126`・`:164`・`:183`・`:199` の `Message` 比較を `.String()` にする。テスト内の期待表（`:64-106`）の `message` は文字列のままにし、比較の右辺を `.String()` にする。`:183` は `attr.Value.String()` ではなく `preExecErr.Message` の直接比較である。`:199` は `assert.Equal` なので型が合わないままコンパイルが通り、実行時に落ちる。
- `internal/logging/pre_execution_error_test.go:249`（`assert.Equal(t, "integration test error", preExecErr.Message)`）: `assert.Equal` は型の異なる値を比較できるためコンパイルが通り、実行時に落ちる。`.String()` を付ける。
- `internal/runner/multi_group_error_integration_test.go:137`: `logging.ExecutionError{Message: "error running commands", ...}` はコンパイルエラーになる。`errmsg.ConstSummary` にする。
- 記録を `attr.Value.String()` で読むテスト（`pre_execution_error_test.go`・`multi_group_error_integration_test.go:48` の `executionErrorMessage` など）は、値が `slog.KindLogValuer` になっても文字列の内容が変わらない見込みである。実行して確かめ、変化があれば機械的に合わせる。
- 変えないテスト: `error` 属性の全文が値全体置換を受けることを固定する `redactor_test.go:3947`（`TestRedactingHandler_PlainStringIsStillRedacted` の `error naming a group`）。対象外の属性についてのものである。

#### 03_detailed_specification.md の記載とコードの食い違い

調査で見つけた食い違いと、計画での扱いを示す。決定は変えない。実装時はコードを典拠にする。

| 箇所 | 食い違い | 扱い |
|---|---|---|
| 03 §10.7 | `cmd/runner/main_test.go` の `Message` リテラルは 2 か所ではなく 1 か所（`:104-109`。`cmd/runner` 全体では `integration_slack_flush_test.go` の 1 か所と合わせて 2 か所） | Phase 4 のタスクは実在する 2 か所を書き換える |
| 03 §5.2・§6.4 | `ExpandWorkDir` の呼び出しは `group_executor.go:687`・`:724`（03 の `:686`・`:723` は `level := fmt.Sprintf(...)` の行） | 呼び出し箇所は変えず、引数の型の変更（Phase 6）と同じ Phase で `config.GroupLevel`・`config.CommandLevel` を渡す |
| 03 §13 | 手順 5（runner）に `group_executor.go` の `ExpandWorkDir` 呼び出しの変更が含まれるが、`config.Level` は手順 6 で導入される | Phase の順序は 02 §8 のまま変えず、呼び出しの変更だけを Phase 6 で行う |
| 03 §7.2.2 | `prepareCommand` は `executor.go` ではなく `command_lifecycle.go:339-456` にある | どちらにせよ対象外（`fmt.Errorf` は `:419`） |
| 03 §11.2 | `package_reference.md` に `internal/identifier` の記述が無い | ディレクトリ一覧のアルファベット順の位置と Package Responsibilities に `internal/errmsg` を追記し、`internal/identifier` の記述の有無には依存しない |
| 03 §10.7 | `ErrUndefinedVariableDetail.Level`・`Field` を文字列として比べるテストは無い | `errors_test.go:71-76` のリテラルだけを書き換え、`String()` の期待値テストは新設する |
| 03 §2.9・§9.10 | `identitymutationguard` に自己テスト用の共通ヘルパは無い | 各ガードの自己テストは `group_errors_guard_test.go` の表の形で各ガードファイルに置く |
| 02 §3.8.1・03 §7.1・§9.1 | `(*NormalResourceManager).ValidateOutputPath`・`(*DryRunResourceManager).ValidateOutputPath`（および両者が委譲する `(*DefaultOutputCaptureManager).ValidateOutputPath` と `validateAndResolvePath`）を「2 つのレコードの原因にならない」として対象外にしている。しかし `group_executor.go:520-521` の `output path validation failed: %w`（対象の範囲内）を通って最終の実行エラーの原因になり、`dryrun_manager.go:159` と `base/output/path.go:57`・`:62`・`:91` の `validatePathSecurity`・`validateRelativePath` が出力パスを挿入する（`base/output/manager.go:71`・`:76` のラップは `path validation failed: `・`security validation failed: ` の定数の前置きだけで、パスは挿入しない）。02 §3.8.1 の規則 (ii)（パスを挿入するラップは対象）と矛盾する | **ブロッキングタスク**として扱う。02 の対象の範囲を修正して再承認を得るまで Phase 7・8 を開始しない（Phase 7 の冒頭）。修正では `base/output/path.go` の `validatePathSecurity`・`validateRelativePath` を対象に加え、03 §9.1 の範囲と §9.4 の役割の許可位置にも同じ関数を加えてから承認を得る |
| 03 §3.4 | `ErrMessageFlattenPanic` は `PanicValue any` を持ち、既存の `ErrLogValuePanic`（`errors.go:13-15`）と同じ `%v` の `Error()` にすると panic 値が `ShutdownReporter` の出力（`reporter.go:132` の `%v`）に漏れる。本文を含まないという記述だけでは足りない | **編集上の修正**として Phase 3 で扱った。`PanicValue`・`StackTrace` の欄を設けず、panic 値の型名（`PanicType`）だけを持つ。`Error()` は型名と固定の文言だけを描画する。`TestRedactingHandler_FlattenPanicDoesNotLeakPanicValueToShutdownReport` で固定した |
| 03 §9.4 | 許可位置の表が `expansion.go` を「ファイル全体」としており、02 §3.8.1 が対象の範囲から除く 5 関数（`ProcessEnvImport` など）でも `Identifier`・`Path` を宣言できてしまう。02 §3.8.1 はこの範囲を `Identifier`・`Path` を宣言できる箇所の唯一の定義としている | **編集上の修正**として Phase 1 で扱った。03 §9.4 の表を「ファイル全体（§9.1 の除く関数を除く）」に改め、`errmsg_guard_test.go` の免除の役割の検査は §9.1 の範囲（除く関数を含む）にも入っていることを求める。§9.1 の範囲の表は `errmsg_guard_test.go` が持ち、Phase 8 の `wrap_guard_test.go` と共有する形にまとめる |
| 03 §13 | 手順 4（`Message` のリテラルの書き換え）と手順 7（`cmd/runner`）が、どちらも `cmd/runner` の 4 か所に触れる | `Message` の型の変更により 23 か所のリテラルの書き換えと 4 か所の原因の `Err` への付け替えは Phase 4 で完了させる。4 か所の到達性の検証だけを Phase 7 で行う |

#### 外部前提の確認

- `make test` は `unit-test` を呼び、`CGO_ENABLED=1 go test -tags test -race -p 4 -v ./...` と非 Darwin 向けの `CGO_ENABLED=0` の 2 回を実行する（`Makefile:479-485`・`:547`）。`make lint` は `golangci-lint run --build-tags test`（v2.11.4）を実行する（`Makefile:23-24`・`:150-156`）。`make fmt` は gofumpt を使う（`Makefile:606-607`）。
- `scripts/verification/check_*.sh` は `make verify-docs-checks`（`Makefile:790-799`）が `sh` で自動実行する。新しい検査スクリプトは追加するだけで実行対象になる。
- `identitymutationguard.ProductionGoFilesInRepo` は `internal`・`cmd` を走査し、`_test.go` とテスト用ビルド制約のファイルを除く（`helpers.go:160-181`・`:215-244`）。`internal/errmsg`・`internal/redaction` の新しい本番ファイルは自動で対象に入る。
- `os.MkdirTemp` と `os.Chmod` は失敗時に `*fs.PathError` を直接返す。`PathErrorCause` の型アサーションで分けられる根拠である（03 §2.4、Go 標準ライブラリ）。
- `go tool cover -func` の基準値は上の削除台帳に記録した（`e3f7dfdc` で取得）。
- `make slack-group-notification-test` は `sample/slack-group-notification-test.toml` を実行する（ターゲットは `Makefile:654`、期待通知の説明は `:665-674`）。このファイルは `internal/runner/config` の互換テスト 3 件が読み込むが、いずれも「読み込みが成功し group が 1 つ以上ある」ことだけを確かめる。group の追加は影響しない見込みである。実行して確かめる。既存の `pre_execution_failure_group`（`env_vars` の未定義変数）があり、Makefile の期待通知は 5 件である。追加後は 6 件になる。
- `make verify-docs-checks` は CI からは実行されていない（`.github/workflows/ci.yml` に呼び出しが無く、docs のみの変更は CI の分類器で対象外になる）。AC-26 の機械的な確認は、Phase 8 の完了ゲートと PR レビューでのローカル実行に依る。

### 1.4 テストヘルパーの方針

- 新しいクロスパッケージのヘルパ・モックは作らない。既存の `tu`・`identitymutationguard`・`cmd/runner` の Slack モック・`resourcetestutil`・`executortestutil` で足りる。
- パッケージ内のテスト用の値は、各テストファイルの中で `errmsg.Const`・`errmsg.Ident` などの公開の構築関数から作る。`internal/errmsg` の中のテストは同じパッケージなので非公開の識別子も使える。
- ガードテストは `_test.go` の本番パッケージ内テストとして置き、新しい `test_helpers.go` は作らない。
- 新しいテストファイルは 03 §10.8 の 6 つ（`internal/errmsg/errmsg_test.go`・`errmsg_guard_test.go`、`internal/redaction/ranges_test.go`・`message_test.go`・`redaction_guard_test.go`、`internal/runner/wrap_guard_test.go`）である。`errmsg_guard_test.go`・`redaction_guard_test.go`・`wrap_guard_test.go` は 03 の指定どおり `//go:build test` を付ける。

---

## 2. 実装ステップ

各 Phase の完了時に `make fmt`（Go を変更した場合）・`make test`・`make lint` を通す（AC-27）。追加・変更したテストは、§4.4 の変異で失敗することを確認し、その旨をコミットメッセージに記す。

### Phase 1: `internal/errmsg` の新設

**Files**: `internal/errmsg/errmsg.go`（新規）、`internal/errmsg/errmsg_test.go`（新規）、`internal/errmsg/errmsg_guard_test.go`（新規・`//go:build test`）

- [x] 03 §2.2 の型（`Role`・`Part`・`Message`・`Segment`・`Segments`・`Summary`・`Error`・`Structured`）と 03 §2.3 の構築関数・メソッド（`Const`・`Ident`・`Path`・`Text`・`Cause`・`PathErrorCause`・`IndentedCause`・`NewMessage`・`Merge`・`Freeze`・`ConstSummary`・`TextSummary`・`NewError`・`Join`・`JoinedError`）を実装する。
- [x] 03 §2.4 の平らにする処理（`Structured` の直接の型による展開、構造を持たない原因の `Text`、nil の `<nil>`、`*fs.PathError` の分解、深さの上限なし）と、03 §2.4.1 の字下げの再現を実装する。
- [x] 03 §2.5〜§2.7 の `Freeze`・`Merge`・`Join` を実装する。`NewError` は原因の部分がちょうど 1 つで nil でないことを要求する（03 §2.8）。
- [x] 03 §10.1 のテストを `errmsg_test.go` に置く。深い連鎖、`IndentedCause` の末尾の除去が断片をまたぐ場合、`Freeze` が原因の `Error()` を 1 回だけ評価すること、nil・ゼロ値、`String()` と `Error()` の一致、AC-21 を含める。
- [x] 03 §9.3（`Const` の定数式）・§9.4（免除の役割の位置）・§9.5（文言と構造の一致）・§9.8（`Part` の非公開と部分の流れ）・§9.9（整形のバイト）のガードを `errmsg_guard_test.go` に実装する。§9.4 の許可位置の表は 03 §9.4 をそのまま使う。
- [x] 各ガードに 03 §9.10 の自己テストを付ける（`Const` の式、別名・ドット import、`Part` の複合リテラル、`Error()` の本体の形）。
- [x] `IndentedCause` のシグネチャをコンパイル時に固定する（`var _ func(error) Part = IndentedCause`）。
- [x] errmsg のガードを 03 §9.0 の前提に合わせる。対象のパッケージを `go/types` で型検査して名前・定数・型・メソッドの選択を型検査の結果から得、変種のファイルの検査を加える。
- [x] 部分の流れの検査を、許す形を並べる規則に置き換える（03 §9.8）。`Error`・`StructuredMessage` はシグネチャまで照合し、名前の無い struct 型を指す別名も調べる（03 §9.5）。

**完了条件**: `internal/errmsg` の単体・ガードテストが green。`go test -tags test ./internal/errmsg/...` が通る。`errmsg` の本番ファイルが標準ライブラリだけを import する。

### PR-1 作成ポイント: the structured message package

- **対象ステップ**: Phase 1
- **推奨タイトル**: `feat(0178): add internal/errmsg for role-tagged error bodies`
- **レビュー観点**: 平らにする契約（直接の型でだけ `Structured` を展開すること、構造を持たない原因が `Text` になること、`*fs.PathError` の分解が `(*fs.PathError).Error()` と一致すること）、`IndentedCause` が `GroupError.Error()` の整形と一致すること、`Const`・免除の役割・`Part` の非公開のガードが自己テストで実際に検出すること、ゼロ値と nil が panic しないこと
- **実装モデル要件**: frontier-required
- **判定理由**: 本タスク全体の土台であり、バイト単位の整形と AST ガードの判定は後続のすべての Phase の正しさを決める。設計判断の密度が高く、誤ると広範囲に波及する

- **マージの条件**（レビューの往復を止めるため）:
  - must-fix の指摘が残っていない。
  - 03 §9.0 の対象（通常のコードの誤り）に入る worth-fixing の指摘を直してある。
  - 対象外の指摘には、03 §9.0 を根拠に返信してある。
  - これ以後に出た P2 の指摘のうち、どの Phase も書く予定の無い仮想のコードについてのものは、フォローアップの issue にまとめ、マージを止めない。
- ガードに新しい規則を足したときは、push の前に、その規則の誤検出（正しいコードを拒否する形）と見落としを自分でレビューする。自己テストには、正しいコードが通る行を必ず入れる。
- 自動レビューの方針は `AGENTS.md` の Review guidelines に置く。

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 2: `internal/redaction` の置き換え範囲の関数とオプションの削除

**Files**: `internal/redaction/ranges.go`（新規）、`internal/redaction/ranges_test.go`（新規）、`internal/redaction/redactor.go`・`value_detector.go`（変更）、`internal/redaction/redactor_test.go`（変更）

- [x] `redactor.go` の `compiledPattern` に「残す先頭のグループの数」（`keptGroups`）を持たせ、置き換える範囲を返す非公開のメソッド（`replacedSpans`）を追加する。`compilePattern` は置換テンプレートをこの数から組み立てる（`keptGroupsTemplate`。組み立てた文字列は変更前と同じ）ので、テンプレートと範囲の導出が同じ定義を共有する。`apply` は変えない（03 §3.1.1）。
- [x] `value_detector.go` に、各規則の置き換える範囲を返す非公開の関数を追加する。`valueDetectorPatterns` の正規表現と `Mask` の置換テンプレートと同じ残す範囲を使い、`Mask` は変えない（03 §3.1.1・§3.1.2）。
- [x] `ranges.go` に `byteRange` と `redactedRanges`（PEM → key=value → 値形式の順、ピーステーブルによる段の重なりと座標の写し、幅 0 の範囲）を実装する（03 §3.1.3・§3.1.4）。
- [x] `redactor.go` に `replaceSpans` を追加する（`redactedRanges` の確認と `RedactMessage` の実行時の検査で使う）。
- [x] `WithPlaceholder`（`redactor.go:155-159`）とそのコメントを削除する。`Config.Placeholder()` は残す。
- [x] `WithAdditionalKeyValuePatterns`（`redactor.go:169-173`）を削除する。`NewConfig` の検証・コンパイル失敗の分岐は残す。
- [x] `ranges_test.go` に 03 §10.2 の差分テストと `FuzzRedactedRangesMatchesRedactText` を置く。種には既存の `RedactText` のテストの入力をすべて含め、残す範囲の種類・段の重なり・空の引用の値・`DefaultPlaceholder` が関与しない入力・`WithWebhookHost` の入力を加える。
- [x] §1.3 の削除・書き換え台帳の 6 項目を 1 項目ずつ実施する。
- [x] `go tool cover -func` を関数ごとに比較し、`WithPlaceholder`・`WithAdditionalKeyValuePatterns` の消滅と `NewConfig` の分岐の網羅率の差をコミットメッセージに記録する。

**完了条件**: `TestRedactedRanges_MatchesRedactText` と `FuzzRedactedRangesMatchesRedactText`（短時間の実行でよい）が green。既存の `RedactText` のテストが変更なしで通る。削除したテストの旧い記述が残っていない。02 §8 の手順 2 の「性能の確認」は、`redactedRanges` の呼び出し元が Phase 3 で現れるため、Phase 3 の `BenchmarkRedactMessage` で確かめる（この Phase では `redactedRanges` 単体の性能を測らない）。

### PR-2 作成ポイント: derive the replaced ranges from the shared rules

- **対象ステップ**: Phase 2
- **推奨タイトル**: `feat(0178): add redactedRanges and drop unused redaction options`
- **レビュー観点**: `RedactText` の挙動が変わっていないこと、規則と段の順序が写しになっていないこと、段の重なりと幅 0 の範囲の扱い、差分テストとファジングの種が残す範囲・重なり・挿入点を網羅すること、オプション削除後の網羅率の差が記録されていること
- **実装モデル要件**: frontier-required
- **判定理由**: 秘密の漏れに直結する範囲の導出であり、`RedactText` との一致義務をテストと実行時の検査の両方で負う。設計の正しさがそのまま安全性になる

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 3: `Config.RedactMessage` とハンドラの分岐

**Files**: `internal/redaction/message.go`（新規）、`internal/redaction/message_test.go`（新規）、`internal/redaction/redaction_guard_test.go`（新規・`//go:build test`）、`internal/redaction/errors.go`・`redactor.go`（変更）

- [x] `message.go` に `RedactMessage`・`redactSegments`（03 §3.2 の手順 1〜8。`Config` が `NewConfig` を経ていなければ `RedactionFailurePlaceholder` を返す）を実装する。`RedactMessage` の中で、平らにする処理の panic を回復して `*ErrMessageFlattenPanic` を返し、範囲から作った文字列が `RedactText` の出力と一致するかを検査して一致しなければ `*ErrMessageRangeMismatch` を返す（03 §3.1.1・§3.2）。手順 4 の `L_i` に使う断片ごとの `redactedRanges(seg.Text)` にも同じ検査を掛け、一致しなければ同じく `*ErrMessageRangeMismatch` を返す（fail-closed の追加の備え）。
- [x] `errors.go` に `ErrMessageFlattenPanic`・`ErrMessageRangeMismatch` を追加する（03 §3.4。秘密を含みうる文字列を持たない）。`ErrMessageFlattenPanic` は 03 §3.4 の `PanicValue`・`StackTrace` を `Error()` に描画しない（欄を設けない、または `Error()` を鍵と固定の文言だけにする）。panic 値には秘密が入りうるためであり、§1.3 の食い違い表に 03 §3.4 の編集上の修正として記録する。実装では `PanicValue`・`StackTrace` の欄を設けず、panic 値の型名（`PanicType`）だけを持つ。
- [x] `redactMessageAttribute`（03 §3.3）を実装し、`redactLogAttributeWithContext`（`redactor.go:799`）と公開の `Config.RedactLogAttribute`（`:330`）の両方の、宣言済みの識別子の判定の後・`switch value.Kind()` の前に置く。判定は値の動的な型がちょうど `errmsg.Message` のときだけ行う。
- [x] `RedactingHandler` は失敗を `ErrorCollector` に型付きのエラーとして記録し、値を `RedactionFailurePlaceholder` にする。`Config.RedactLogAttribute` は `collector` に nil を渡し、`slog.KindLogValuer` の値を `RedactionFailurePlaceholder` にする（fail-closed）。
- [x] `message_test.go` に 03 §10.3・§10.4 のテスト（AC-01〜04・07・20・36〜38、挿入点、実行時の検査、`Config` 未検証、ハンドラ、`Config.RedactLogAttribute`）を置く。AC-08 は `TestRedactSegments_ZeroRoleFallsBackToText`（ゼロ値の役割）、AC-38 は `TestRedactSegments_UnknownRoleFallsBackToText`（範囲外の役割）で確かめる。AC-37 の各入力では、先に各断片だけに `RedactText` と値全体置換を適用しても秘密が見えたまま残ることを確かめる。
- [x] `TestRedactMessage_RangeMismatchReportsFailure` の不一致は、パッケージ内のテストが `NewConfig` の後で `cfg.placeholder` を `DefaultPlaceholder` 以外に設定して起こす（`RedactText` は `NewConfig` が規則にコンパイルした `DefaultPlaceholder` を、範囲の描画は `cfg.placeholder` を使うため、意図的に食い違わせられる）。断片ごとの検査は、同じ入力では全体の検査も失敗するので、`TestRedactText_SegmentRangeMismatchReportsFailure` で補助関数を直接確かめる。`TestRedactMessage_FlattenPanicReportsFailure` は、`StructuredMessage()` が panic する型をテスト内に定義し、その panic が `RedactMessage` の平らにする処理の時点で回復されることを確かめる。
- [x] `TestRedactingHandler_FlattenPanicDoesNotLeakPanicValueToShutdownReport` を `message_test.go` に置く。秘密を `PanicValue` に持つ `StructuredMessage()` が panic する型を `RedactingHandler`（`ErrorCollector` 付き）に通し、記録された失敗を `ShutdownReporter` に報告させる。報告の出力と `Failure.Err.Error()` にその秘密が現れないことを確かめ、§1.3 の食い違い表のとおり `Error()` が panic 値とスタックトレースを描画しないことを固定する。
- [x] `BenchmarkRedactMessage` を `message_test.go` に置き、100 group の失敗を連結した数十 KiB の入力で 1 回の描画が 200 ms 以下であることを確かめ、結果（実測値と実行環境）をコミットメッセージに記録する（03 §3.6）。実測は約 100 ms（40,123 バイト・700 断片、linux/arm64・12 CPU・go1.26.3）。当初の予算 10 ms は同じ入力に対する `RedactText` 1 回（約 26 ms）を下回るため、02 §3.2.2・03 §3.6 の予算を 200 ms に改めた（経緯は 02 付録 A）。
- [x] `redaction_guard_test.go` に 03 §9.6 の AC-25 のガードと自己テストを置く。

**完了条件**: `RedactMessage` のテストとベンチマークが green。AC-01〜04・07・36〜38 の各入力で、対象の層だけが反応することを先に確かめている。

実装で確かめた範囲の注記:

- AC-25 の `static` の検証は、`internal/redaction` の本番のコードが役割の値を作らないこと（03 §9.6）に限る。断片の文字列を調べて役割を選び直す分岐（`strings` などによる判定）はガードの対象外であり、レビューで確かめる。通知ビルダー・ログ出力は構造化メッセージを文字列として受け取る（AC-20）ので、役割を読む箇所は `internal/redaction` だけである。
- 03 §3.2 手順 8（境界をまたぐ検出が無い断片は断片単独の結果を出す）は、既定の規則では接する 2 つの範囲が生じないため、影響を受ける描画との違いを入力で観察できない。`coversPoint`・`coversRange` を常に偽にしても現在のテストは通る。
- `redactSegments` の中の予期しない panic（索引の誤りなど）は回復しない。平らにする処理の panic だけを回復する（03 §3.2）。

### PR-3 作成ポイント: render a structured message with per-segment redaction

- **対象ステップ**: Phase 3
- **推奨タイトル**: `feat(0178): redact structured messages per segment`
- **レビュー観点**: `Identifier` の免除と境界をまたぐ契約（AC-37）、`Text` の値全体置換が断片ごとであること（AC-36）、範囲の不一致・平らにする処理の panic が fail-closed に倒れること、panic 値が `ShutdownReporter` の報告に漏れないこと、後段のハンドラが文字列を受け取ること、`Config.RedactLogAttribute` の fail-closed の分岐、ベンチマークの予算
- **実装モデル要件**: frontier-required
- **判定理由**: 部分ごとの redaction と境界をまたぐ置換は本タスクの中心のセキュリティ境界であり、バイト単位の手順と失敗時の扱いを同時に満たす必要がある

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 4: `internal/logging` の構造化メッセージへの移行

**Files**: `internal/logging/pre_execution_error.go`・`execution_error.go`、`internal/runner/group_stage.go`（段階の定義表）、`cmd/runner/main.go`・`internal/runner/bootstrap/config.go`・`environment.go`・`internal/runner/runerrors/pre_execution.go`（`Message` のリテラル）、`.golangci.yml`（`main` の depguard に `internal/errmsg` を追加）、および対応するテスト。`internal/logging/slack_handler.go` は変えない（通知ビルダーは描画済みの文字列を受け取る）

- [x] `PreExecutionError.Message` を `errmsg.Summary` にし、`DetailMessage`・`Detail` を 03 §4.1 のとおりに実装する。`Is`・`As`・`Unwrap` と `Error()` の書式は変えない。
- [x] `ExecutionError.Message` を `errmsg.Summary` にし、`ReportMessage`・`contextParts` を実装する。context の文言は `contextParts` の 1 か所で作り、`ReportMessage` と `ContextString` の両方が使う（03 §4.2）。
- [x] `errorRecordParams.errorMsg` を `errmsg.Message` にし、`writeErrorLogRecord` を `slog.Any` にする。`preExecutionRecordParams` は `DetailMessage().Freeze()`、`HandleExecutionError` は `ReportMessage().Freeze()` を渡し、平らにするのは報告ごとに 1 回にする（03 §4.3）。
- [x] `handleErrorCommon` の stderr の `Details:` は凍結した `Message` の `String()` を使い、文言と redaction なしの挙動を変えない（AC-19）。
- [x] `internal/runner/group_stage.go` の段階の定義表の `message` の型を `errmsg.Summary` にし、各行を `ConstSummary` にする。`groupStagePreExecutionError` は `Message: def.summary` にする（03 §4.4 #23）。
- [x] 03 §4.4 の 23 か所の `Message` のリテラルを機械的に書き換える。定数式は `ConstSummary`、値を含むものは `TextSummary`。`#9`・`#10`・`#13`・`#15` は原因を `Err` に移し、`Message` を固定の文言にする（03 §8 の表）。
- [x] `ExecutionError` の唯一の設定箇所（`cmd/runner/main.go:695`）の `Message` を `errmsg.ConstSummary("error running commands")` にする。
- [x] §1.3「更新が必要な既存テスト」の項目を実施する。`cmd/runner/main_test.go` の `TestStartupDirPermAudit_CheckerInitFailureReturnsPreExecutionError` は `errors.Is(err, errCheckerUnavailable)` と `Detail()` の確認に変える。
- [x] 記録を `attr.Value.String()` で読む既存テストを実行し、読み出す文字列が変わらないことを確かめる。変化があれば機械的に合わせる。
- [x] 呼ぶたびに文言が変わる原因を持つ構造化メッセージで `HandleExecutionError` に報告し、stderr の `Details:` と記録された `error_message` が同じ 1 回の結果から来ること（原因の `Error()` が 1 回だけ呼ばれること）を `TestHandleExecutionError_EvaluatesCauseOnce` で確かめる。
- [x] `TestBuildPreExecutionError_InterpolationContract`（`slack_handler_test.go:2241`、AC-22）に、`PreExecutionError.DetailMessage()` の本文を `RedactingHandler` に通した行を追加し、500 byte を超える本文でも先頭の段階の要約文が残ることを確かめる。

**完了条件**: `internal/logging`・`cmd/runner` のテストが green。この Phase の完了時点で、2 つのレコードの `error_message` が構造化メッセージとして記録され、`RedactMessage` を通る。

### PR-4 作成ポイント: record the two reports as structured messages

- **対象ステップ**: Phase 4
- **推奨タイトル**: `feat(0178): record pre-execution and execution errors as structured messages`
- **レビュー観点**: `Detail()`・`Error()`・stderr の文言が変わっていないこと、原因の `Error()` の評価が報告ごとに 1 回であること、`PreExecutionError.Message` が `Constant` か `Text` のどちらかに限られていること、`cmd/runner` の 4 か所で原因が `Err` に移っても報告の種別と終了コードが変わらないこと、既存テストの書き換えが機械的であること
- **実装モデル要件**: frontier-recommended
- **判定理由**: 型の変更は機械的だが、凍結の位置と原因の到達性、`Message` の分類の網羅が正しさを決める。範囲は広いが設計は確定している

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 5: `internal/runner` のエラー型と `cancelledRunError`

**Files**: `internal/runner/group_errors.go`・`group_stage.go`・`runner.go`・`group_executor.go`（変更）、および対応するテスト

- [x] `GroupStageError`・`GroupError`・`GroupErrors`・`CommandExecutionError` に `Structured` を実装する。部分の並びは 03 §5.1 の表のとおり。`Error()` は `return e.StructuredMessage().String()` の 1 文にする。`GroupErrors` は `Merge` で平らにせずにつなぐ。
- [x] ゼロ値と nil の欄を持つ値の `Error()` が panic しないことを確かめる。
- [x] `group_executor.go` の 11 か所の `fmt.Errorf` を `errmsg` の構築に置き換える。部分の並びは 03 §5.2 の表のとおり。数値は `Text`、`%q` のパスは `strconv.Quote` した全体を `Path`、先頭の番兵は `Cause` にする。
- [x] `executeGroups` の `errors.Join(ctxErr, err)`（`runner.go:452`）を `cancelledRunError`（03 §5.3）に置き換える。この型を作るのは、`ctxErr` と `err` がどちらも nil でないことを確かめた後だけにする。
- [x] `cancelledRunError` の文言が `errors.Join(ctxErr, err)` と同じであること、`errors.Is`・`errors.AsType` が変更前と同じ対象に届くことを確かめる。`executeGroups` が返すエラーの動的な型が `*cancelledRunError` であること（`errors.AsType`）も、Phase 8 の `TestRunner_CancelledRunErrorMessageKeepsIdentifiers` で確かめる。
- [x] 03 §10.5 の各エラー型のテスト（部分の並び、文言が変更前と同じであること、`cancelledRunError` の文言と到達性）を追加する。`GroupErrors` が `Merge` で各 `GroupError` の `Identifier` の断片を保つことは `TestGroupErrors_MergePreservesIdentifierSegments` で確かめる。`group_stage_test.go`・`group_errors_test.go`・`group_executor_test.go`・`runner_test.go`・`multi_group_error_integration_test.go` の既存の型・文言の確認を更新する。

**完了条件**: `internal/runner` のテストが green。`GroupErrors.Error()` の文言が各 `GroupError.Error()` を改行でつないだ変更前の結果と同じである。`cancelledRunError` の文言と到達性のテストが green。

### PR-5 作成ポイント: structured error types in internal/runner

- **対象ステップ**: Phase 5
- **推奨タイトル**: `feat(0178): make runner error types carry structured messages`
- **レビュー観点**: 4 型の `Error()` の文言が変更前と同じであること、`GroupErrors` が `Merge` で識別子の断片を保つこと、11 か所の役割の割り当てが 01 の方針に合うこと、`cancelledRunError` が専用の型であること（形による判定をしないこと）、`errors.Is` の到達性
- **実装モデル要件**: frontier-recommended
- **判定理由**: 文言の維持と構造の引き継ぎが中心で、設計は 03 で確定している。箇所数が多いが機械的である

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 6: `internal/runner/config` の `Level`・`Field` と `ErrUndefinedVariableDetail`

**Files**: `internal/runner/config/errors.go`・`expansion.go`・`validation.go`・`template_expansion.go`（変更）、`internal/runner/group_executor.go`（`ExpandWorkDir` の呼び出し）、および対応するテスト

- [x] `errors.go` に `Level`・`Field` の型、種類、パッケージ内の構築関数、`String()`、`parts()`、公開の `GroupLevel`・`CommandLevel` を実装する。`String()` は変更前に `fmt.Sprintf` で作っていた文字列と同じにする（03 §6.1）。
- [x] `ErrUndefinedVariableDetail` の `Level`・`Field` を型にし、`StructuredMessage` を実装する。参照された変数名・`Chain` の各名前・`Level`・`Field` の中の名前は `Identifier`、`Context` は `Text` にする（03 §6.3）。
- [x] `expansion.go` の各ラップを 03 §6.5 の分類に従って構造化する。`ErrUndefinedVariableDetail` を運びうるラップは名前を `Ident`・固定の文言を `Const`、運びえないラップは前置きの全体を 1 つの `Text` にして `Cause` を続ける。
- [x] `ExpandWorkDir` のシグネチャを `Level` にし、相対パスの拒否のエラーを 03 §6.4 のとおりに構造化する。`group_executor.go:757`・`:794` は `config.GroupLevel(...)`・`config.CommandLevel(...)` を渡す（§1.3 の食い違い表のとおり、ここで行う。行番号は Phase 4・5 の変更でずれた）。
- [x] 03 §6.2 の表の関数の引数を `Level`・`Field` に変える。`variableResolver` の `field` と各関数リテラルの第 2 引数も `Field` にする。`validateVariableName` の `level == "global"` の比較は `level.kind == levelGlobal` にする。
- [x] 文字列の `Level`・`Field` を持つ既存の詳細型（`ErrCircularReferenceDetail` など）には `level.String()`・`field.String()` を渡す。固定のキーは構築関数から作る（03 §6.2 の最後の項）。`cmd_allowed` の重複検出の `Field` は変更前と同じ前置き（index なし）なので文字列のままにする。
- [x] `errors_test.go:71-76` のリテラルを `globalLevel()`・`envField()` に書き換える。`Level`・`Field` の `String()` が変更前の `fmt.Sprintf` の結果と同じであることを確かめるテスト（`TestLevelAndField_StringMatchesLegacyFormat`）をキーごとに追加する。
- [x] `ErrUndefinedVariableDetail` の部分の並び（`Chain` が空・非空の両方）を確かめるテスト（`TestErrUndefinedVariableDetail_StructuredMessage`）を追加する。
- [x] §1.3「更新が必要な既存テスト」のとおり、引数の型が変わる関数を呼ぶ既存テストを更新する。`expansion_unit_test.go` は `package config` へ移し、`group_executor_test.go::TestExecuteGroup_PreExecutionStageErrors` の `wantIdentifiers` に構造化された原因の断片を足す。
- [x] §4.4 の変異で `TestLevelAndField_StringMatchesLegacyFormat`（`Field.String()` の `vars.<name>` の組み立てを変える）と `TestExpandWorkDir_RelativePathError`（相対パス拒否のパスを `Path` から `Ident` に変える）が失敗することを確認した。レビューの指摘を受け、`ExpandWorkDir` の相対パス拒否の役割と文言、`Level`・`Field` の `parts()` と `String()` の一致を固定するテストを追加した。

**完了条件**: `internal/runner/config` のテストが green。`ErrUndefinedVariableDetail.Error()` の文言が変更前と同じである。`go test -tags test ./internal/runner/config/... ./internal/runner/...` が通る。

### PR-6 作成ポイント: type the expansion level and field

- **対象ステップ**: Phase 6
- **推奨タイトル**: `feat(0178): declare expansion levels and fields as types`
- **レビュー観点**: `String()` が変更前の文言と同じであること、`parts()` が構築時に役割を宣言すること（組み立て済みの文字列を解析しないこと）、`ErrUndefinedVariableDetail` を運びうるか運びえないかの分類が 03 §6.5 のとおりであること、引数の型の変更が全関数に及んでいること、`ErrUndefinedVariableDetail` の `Unwrap()` が変わっていないこと
- **実装モデル要件**: frontier-recommended
- **判定理由**: 変更範囲が広い機械的な型変更だが、`expansion.go` の運びうるかどうかの分類は経路をたどる判断を伴う。設計は 03 で確定している

- [x] `make test && make lint` が green であることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた

### Phase 7: コマンドの実行の経路と `cmd/runner`

**Files**: `internal/runner/resource/normal_manager.go`・`dryrun_manager.go`、`internal/runner/base/output/path.go`（`validatePathSecurity`・`validateRelativePath` のパスを挿入する 3 か所）・`manager.go`（上の修正の再承認後。`validateAndResolvePath` などのパス検証）、`internal/runner/base/executor/tempdir_manager.go`・`executor.go`・`command_lifecycle.go`、`internal/runner/base/privilege/errors.go`・`unix.go`、`cmd/runner/main_test.go`（到達性テストの追加）、および対応するテスト。`cmd/runner/main.go` ほかの `Message` のリテラルと 4 か所の原因の付け替えは Phase 4 で完了している（§1.3 の食い違い表を参照）

- [ ] **【ブロッキング】** 02 §3.8.1 の `ValidateOutputPath` の除外を修正する。出力パスの検証の失敗は `group_executor.go:520-521` の `output path validation failed: %w` を通って最終の実行エラーの原因になり、`base/output/path.go:57`・`:62`・`:91` の `validatePathSecurity`・`validateRelativePath` が出力パスを挿入する（`manager.go:71`・`:76` のラップは定数の前置きだけで、パスを挿入しない。§1.3 の食い違い表）。02 の対象の範囲に `(*NormalResourceManager).ValidateOutputPath`・`(*DryRunResourceManager).ValidateOutputPath`・`(*DefaultOutputCaptureManager).ValidateOutputPath`・`(*DefaultPathValidator).ValidateAndResolvePath` とその先の `validatePathSecurity`・`validateRelativePath`（および `validateAndResolvePath`）を加え、出力パスを `Path` として宣言できるようにする修正を提案し、レビュアーの再承認を得る。03 §9.1 の範囲と §9.4 の役割の許可位置に同じ関数を加え、§7.1 の resource の表にも `ValidateOutputPath` 系を加えてから合わせる。**再承認が完了するまで、この Phase の resource の実装と Phase 8 の AC-41 のガードを開始しない。**
- [ ] `internal/runner/resource` の対象の箇所（16 か所と、上の修正で加わる `ValidateOutputPath` 系）を 03 §7.1 の表と修正後の範囲のとおりに構造化する。`CreateTempDir`・`CleanupTempDir`・`CleanupAllTempDirs`・`UpdateCommandDebugInfo` は変えない。
- [ ] `internal/runner/base/output/path.go` の `validatePathSecurity`・`validateRelativePath` の 3 か所（`:57`・`:62`・`:91`）の挿入するパスを `Path`、番兵と固定の文言を `Const`、原因を `Cause` として構造化する。`manager.go:71`・`:76` の定数の前置きのラップも、原因が運ぶ `Path` の断片を保つように構造化する（再承認後）。
- [ ] `tempdir_manager.go` の 2 つのラップを `PathErrorCause` にし、前置きは既存の文言（`failed to create temporary directory: `・`failed to set permissions on temporary directory: `）を保つ。`os.MkdirTemp` と `os.Chmod` の失敗をそれぞれ別に起こして文言を確かめる（03 §7.2.1）。
- [ ] `executor.go` の 12 か所を 03 §7.2.2 のとおりに構造化する。パスは `Path`、固定の文言は `Const`。`stageFromFD`・`prepareCommand`・`output_pump.go`・`fdexec_linux.go` の `fmt.Errorf` は変えない。
- [ ] `command_lifecycle.go` の `runCommand`・`reportStartFailure`・`superviseCommand` の `errors.Join` を `errmsg.Join` にし、`killChild`・`killOutcome` の 2 つの `%w` の書式を `killAfterCancelError`（03 §7.2.3）にする。`:736`・`:789`・`:931` は 03 の表のとおりに構造化する。`rankedError`・`release`・`startPrepared` は変えない。
- [ ] `privilege/errors.go` の `(*Error).Error()` を `StructuredMessage()` から作るようにし、`unix.go:217` の `fmt.Errorf` を構造化する（03 §7.3）。`WithPrivileges`・`escalatePrivileges` は変えない。
- [ ] 03 §10.5 の各エラー型のテスト（ラップごとの文言、到達性、`killAfterCancelError` の 2 経路、`privilege.Error`）を追加・更新する。`killAfterCancelError` は `TestKillAfterCancelError_TextAndReachability` で確かめる。一時ディレクトリの 2 つのラップは `TestTempDirManager_Create_WrapTexts` で確かめる。`base/output/path.go` の 2 関数と `manager.go` の 2 つのラップは `TestPathValidationWraps_KeepInsertedPath` で、文言が変更前と同じであり、出力パスが `Path` の断片として残ることを確かめる（再承認後）。`TestRunCommand_ChildStateTransitions/spent_command_stays_not_started`・`TestExecute_FdBoundStartFailureNoLeak`・`TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow` が green のままであることを確かめる。
- [ ] `cmd/runner/main_test.go` に表駆動の `TestPreExecutionCauseReachability` を追加する。global の展開・テンプレート検証・ディレクトリ権限チェッカーの初期化・`--groups` の各失敗について、`PreExecutionError.Err` に付け替えた原因へ `errors.Is`・`errors.AsType` が届くことと、`Detail()` の文言が変更前と同じであることを確かめる。`run`・`executeRunner` はパッケージ内テストからフラグと設定ファイルを与えて直接呼ぶ。既存の `TestStartupDirPermAudit_CheckerInitFailureReturnsPreExecutionError` の seam をチェッカーの初期化の行に使う。
- [ ] 数値の役割（index・終了コード・件数・理由の番号）が `Text` であり、`Const` を使っていないことを確かめる。

**完了条件**: 対象の経路のテストが green。ラップごとの `Error()` の文言が変更前と同じである。`errors.Is`・`errors.AsType` が変更前と同じ対象に届く。

### PR-7 作成ポイント: structure the command-execution path and cmd/runner

- **対象ステップ**: Phase 7
- **推奨タイトル**: `feat(0178): structure command-execution errors and verify entrypoint reachability`
- **レビュー観点**: 権限の昇格・子プロセスの監督の経路で `errors.Is` の到達性が落ちていないこと、`killAfterCancelError` が 2 つの原因の両方に届くこと、`privilege.Error` の `CommandName` が `Identifier` であること、`cmd/runner` の 4 か所で付け替えた原因に届くこと、一時ディレクトリの 2 つのラップの文言と `*fs.PathError` の分解、02 の対象の範囲の修正が再承認され、`base/output/path.go` の `validatePathSecurity`・`validateRelativePath` が範囲と役割の許可位置に入っていること
- **実装モデル要件**: frontier-recommended
- **判定理由**: 経路が広く、既存の到達性テストとの整合が必要。設計は 03 で確定しているが、複合の型の置き換えは注意を要する

- [ ] `make test && make lint` が green であることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた

### Phase 8: AC-41 のガード、例示のシナリオのテスト、文書

**Files**: `internal/runner/wrap_guard_test.go`（新規・`//go:build test`）、`cmd/runner/integration_pre_execution_error_test.go`・`internal/runner/multi_group_error_integration_test.go`・`internal/runner/runner_test.go`（テスト追加）、`docs/dev/architecture_design/security-architecture.ja.md`・`.md`、`docs/dev/developer_guide/package_reference.md`、`docs/user/security-risk-assessment.ja.md`・`.md`、`scripts/verification/check_structured_message_redaction_docs.sh`（新規）と `scripts/verification/check_structured_message_redaction_docs_selftest.sh`（新規）、`sample/slack-group-notification-test.toml`・`Makefile`（手動確認用）

- [ ] 03 §9.2 の AC-41 のガードを `wrap_guard_test.go` に実装する。対象の範囲（ファイル全体・関数の単位・除く関数）をテストの中の表として持ち、§9.1 の関数名とファイル名が実際のコードに見つかることも確かめる。`cancelledRunError`・`killAfterCancelError`・`privilege.Error` の `StructuredMessage` の宣言も確かめる。自己テストを付ける。範囲の定義は 02 §3.8.1 の修正の再承認後の内容に合わせる（Phase 7 の冒頭）。
- [ ] AC-12 のテストを `cmd/runner/integration_pre_execution_error_test.go` に追加する。名前が値全体置換の引き金になる語（`password`・`token` など。以下「語」）を含む group（`token-rotate`）の `vars` で定義する変数（`token_file`）が、語を含む未定義の変数（`api_key`）を参照する設定を使い、`runMainWithSlackMock` と `requireSinglePreExecutionError`・`attachmentField` で Slack の `Error Message` まで通す。group 名・定義側の変数名 `token_file`・参照された変数名 `api_key` の 3 つが `Error Message` に残ることを確かめる。先に、識別子以外の本文だけでは値全体置換が起きないことを確かめる。
- [ ] AC-34 のテストを同ファイルに追加する。global の `vars` が未定義の `api_key` を参照する設定で、`Error Message` に `Failed to expand global configuration` と `api_key` が出ることを確かめる。
- [ ] AC-16 のテストを `internal/runner/multi_group_error_integration_test.go` に追加する。`captureExecutionErrorReport` は、内側のコールバックハンドラを `redaction.NewRedactingHandler`（`redaction.DefaultConfig()`）で包み、報告する `ExecutionError.Message` を呼び出し側から受け取るようにする。既存の 4 つの呼び出しは `errmsg.ConstSummary("error running commands")` を渡し、AC-16 は `Text` としてだけ置換される語を含む `errmsg.TextSummary`（対照値）を渡す。2 つの group でコマンドが 0 以外の終了コードで失敗し、一方の group 名が語を含むとき、`error_message` に両方の group のエラーが宣言する `Identifier` の部分が置き換えられずに出ることと、対照値が置換文字列になることを確かめる（終了コードのエラー書式はパスを宣言しないため、このシナリオで `Path` の断片は現れない）。
- [ ] AC-31 のテスト `TestRunner_CancelledRunErrorMessageKeepsIdentifiers` を `internal/runner/runner_test.go` に追加する。`executeGroups` の経路で context を中断してから group を失敗させ、返したエラーの動的な型が `*cancelledRunError` であること（`errors.AsType`）を確かめる。続いて、返した `*cancelledRunError` を `captureExecutionErrorReport` に `Err` として渡し、`logging.ExecutionError`（`Message` は呼び出し側が渡し、`Component`・`RunID` はヘルパーの既存の値、`GroupName`・`CommandName` は `cmd/runner` の `executionErrorContext` が返す値（このテストでは失敗した group の `*CommandExecutionError` の `GroupName`・`CommandName`）、`Err` は返したエラー）として `logging.HandleExecutionError` に通す。これで、記録は AC-16 と同じく `redaction.NewRedactingHandler`（`redaction.DefaultConfig()`）を横断した後の `error_message` になる。`Message` には `Text` としてだけ置換される語を含む `errmsg.TextSummary`（対照値）を渡し、対照値が置換文字列になり、`error_message` に中断の原因の文言（`context canceled`）と失敗した group のエラーが宣言する `Identifier`・`Path` の部分が残ることを確かめる（失敗した group のエラーは、`CommandExecutionError` の原因に `Path` を宣言する `errmsg` の構築を置くなど、`Identifier` と `Path` の両方を宣言する形にする）。文言と到達性は Phase 5 の `TestCancelledRunError_TextAndReachability` が担う。
- [ ] `docs/dev/architecture_design/security-architecture.ja.md` の「識別子の型宣言による免除」の節全体（`:636-660`）を 03 §11.1 の内容に合わせて書き換える。旧称「値まるごと判定」は節内の `:640`・`:649`・`:650` にもあるので、行範囲ではなく節全体を対象にし、用語集の「値全体置換」にそろえる。`error` 属性・`record.Message` は変更前と同じ扱いであることを残す。
- [ ] 同節の戻し方に、戻す単位を書く。構造化メッセージの記録を使う変更（Phase 4）は、後の Phase のガード・テスト・文書と結び付いている。そのため、PR-4〜PR-8 をまとめて戻す。個別に戻す場合は `git revert -n` を使い、戻した記録の変更に合わせて `wrap_guard_test.go`・AC-12/AC-34 のテスト・日英の文書を同じコミットで整合させてから、`make test`・`make lint` を通す。実行時のスイッチが無く、`RedactText` を変えないのでほかのログの redaction は影響を受けないことを記す（02 §5.4、既存の #1136 の戻し方の段落と同じ形）。
- [ ] `security-architecture.md` の対応する節を `/mktrans` で日本語版と同じ内容に反映する（用語集の登録を含む）。
- [ ] `docs/user/security-risk-assessment.ja.md`:301・`docs/user/security-risk-assessment.md`:305 の識別子の免除の段落を更新する。普通の `error` 文字列・`record.Message` は値全体置換で全文が置換されうるままであることと、構造化 `error_message` では宣言された `Identifier` の断片が値全体置換を受けずに残ることとを区別して書き、`error` 属性・`record.Message` についての説明として正確にする。日本語版を先に直し、英語版は `/mktrans` で反映する。両ファイルの必須語を次の検査スクリプトに加える。
- [ ] `scripts/verification/check_structured_message_redaction_docs.sh` を追加する。本スクリプトと次の自己テストは strict POSIX sh で書く（`#!/bin/sh`、`[[ ]]`・配列・`local` を使わず、`CDPATH= cd -- "$dir"` を使う）。既存の `scripts/verification/check_*.sh` は `make verify-docs-checks` が `sh` で実行する（§1.3 の外部前提）。日英それぞれについて、構造化メッセージ・役割・`Path` を値全体置換の対象外とする境界・構造を持たないエラーが `Text` であること・旧称を使っていないこと・security-risk-assessment の免除の記述が構造化 `error_message` を「免除されない」と読ませないことを検査し、`package_reference.md` に `errmsg/` と `internal/errmsg` の責務の記述があることも検査する。参照するルートは環境変数で上書きできるようにし、既定はリポジトリのルートにする。既存の `check_identifier_exemption_docs.sh` の必須語（識別子・免除・NewIdentifier・identifier・exempt）が書き換え後も残ることを確かめ、必要なら同スクリプトのアンカーを更新する。
- [ ] `scripts/verification/check_structured_message_redaction_docs_selftest.sh` を追加する（自己テストも同じ strict POSIX sh の規約で書く）。検査対象の語をすべて含むフィクスチャでは終了コード 0、1 語を欠くフィクスチャでは非ゼロになることを確かめ、検査が空振りしないことを固定する（`make verify-docs-checks` は `check_*.sh` を列挙して実行するので、この自己テストも自動で走る）。
- [ ] `docs/dev/developer_guide/package_reference.md` のディレクトリ一覧に `errmsg/` を、Package Responsibilities に `internal/errmsg` の責務を追記する（§1.3 の食い違い表のとおり、`internal/identifier` の記述の有無に依存しない）。追記は上の検査スクリプトで確かめる。
- [ ] `make verify-docs-checks` を実行し green を確かめる。`make verify-docs` も実行し、本タスクで追加した記述に起因する報告が無いことを確かめる。
- [ ] `sample/slack-group-notification-test.toml` と `Makefile`（`:654-674`）を更新する。既存の `pre_execution_failure_group`（`env_vars` の未定義変数）を残し、group 名が語を含み `vars` の未定義変数で group 実行前段に失敗する group を追加して、期待通知を 5 件から 6 件に更新する（2 つの `pre_execution_error` の違いを説明に書く）。`internal/runner/config` の互換テスト 3 件が green のままであることを確かめる。
- [ ] `make slack-group-notification-test` を実行し、Slack の `Error Message` に要約文と group 名・変数名が出ることを確かめる（手動。結果をコミットメッセージに記録する）。
- [ ] 【突き合わせ】 本タスク以前からある「識別子の免除」の説明を引き継いだ成果物を横断して監査し、次の 3 つを同じ変更の中で更新する。(1) 02 §3.8.1 の対象の範囲と 03 §9.1・§9.4 の役割の許可位置の表に `internal/runner/base/output/path.go` のパスを挿入する `validatePathSecurity`・`validateRelativePath` を加える（Phase 7 の冒頭のブロッキングタスクの再承認に含める）。(2) `internal/runner/multi_group_error_integration_test.go` の `captureExecutionErrorReport` を `redaction.NewRedactingHandler` 経由にし、`Text` としてだけ置換される対照値を加える。(3) `docs/user/security-risk-assessment.ja.md`・`.md` の免除の記述を、構造化 `error_message` の `Identifier` の断片が残る新しい挙動に合わせる。3 つとも本タスク以前の免除の説明を引き継いだ成果物であり、1 つだけ直すと食い違うため同時に更新する。日英の文書の一致は `make verify-docs-checks` で機械的に確かめる。
- [ ] 最後に `make test`・`make lint` を通す。

**完了条件**: AC-41 のガードと自己テストが green。例示のシナリオのテストが green。`make verify-docs-checks`・`make test`・`make lint` が green。`make slack-group-notification-test` の結果が記録されている。

### PR-8 作成ポイント: scope guard, end-to-end scenarios, and documentation

- **対象ステップ**: Phase 8
- **推奨タイトル**: `feat(0178): guard the in-scope wraps and document the boundary`
- **レビュー観点**: AC-41 のガードの範囲が 03 §9.1 と一致し、関数やファイルの名前が消えたことを検出すること、例示のシナリオがエラーの発生元から Slack の組み立て・記録までを通すこと、日英の文書が同じ内容であること、検査スクリプトが `make verify-docs-checks` から実行されること、Slack の手動確認の結果
- **実装モデル要件**: frontier-recommended
- **判定理由**: ガードの範囲の定義と、端から端までのシナリオの組み立てに設計の理解が要る。文書は内容が確定している

- [ ] `make test && make lint` が green であることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 構造化メッセージの土台 | Phase 1 | `internal/errmsg` の単体・ガードテストが green。平らにする契約と整形が固定されている |
| M2: 範囲の導出と部分ごとの redaction | Phase 2、Phase 3 | 差分テスト・ファジング・`RedactMessage` のテストが green。ベンチマークが予算内 |
| M3: 記録の移行 | Phase 4 | 2 つのレコードが構造化メッセージとして記録され、文言と stderr が変わらない |
| M4: 対象のエラーの構造化 | Phase 5、Phase 6、Phase 7 | 対象の範囲のラップが `errmsg` の構築になり、`Error()` の文言と到達性が変わらない |
| M5: 網羅と文書 | Phase 8 | AC-41 のガード、例示のシナリオのテスト、日英の文書、`make verify-docs-checks`・`make test`・`make lint` が green |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `internal/errmsg`、errmsg のガード | frontier-required |
| PR-2 | Phase 2 | `redactedRanges`、オプションの削除、差分テストとファジング | frontier-required |
| PR-3 | Phase 3 | `RedactMessage`・ハンドラの分岐・失敗の型・AC-25 のガード | frontier-required |
| PR-4 | Phase 4 | `PreExecutionError`・`ExecutionError`・記録と凍結・`Message` のリテラル | frontier-recommended |
| PR-5 | Phase 5 | runner の 4 型・`group_executor.go` の 11 か所・`cancelledRunError` | frontier-recommended |
| PR-6 | Phase 6 | `Level`・`Field`・`ErrUndefinedVariableDetail`・`expansion.go`・`ExpandWorkDir` | frontier-recommended |
| PR-7 | Phase 7 | resource・`base/output`（再承認後）・executor・privilege・`cmd/runner` の到達性テスト | frontier-recommended |
| PR-8 | Phase 8 | AC-41 のガード・例示のシナリオ・日英の文書・検査スクリプト | frontier-recommended |

### 3.3 順序の根拠

[02_architecture.md](02_architecture.md) §8 の優先順位 1〜8 をそのまま保つ。1〜3 はそれだけで既存の出力を変えない（構造化メッセージを記録する箇所がまだ無く、`RedactText` も挙動を変えない）。4 以降で、記録が構造化メッセージになる。Phase 3 は Phase 1・2 に依存する。Phase 4 は Phase 1・3 に依存する。Phase 5 の `GroupError` などの型は Phase 4 の `Message` の型に依存する。Phase 6 は Phase 1 に依存し、`ExpandWorkDir` の呼び出しの変更だけをここで行う（03 §13 との食い違いは §1.3 の食い違い表のとおり）。Phase 7 は Phase 4 の `PreExecutionError` の型に依存する。`cmd/runner` の実装は `Message` の型の変更のために Phase 4 で完了させ、Phase 7 では到達性の検証だけを行う（§1.3 の食い違い表のとおり）。Phase 7 の resource の実装と Phase 8 は、02 §3.8.1 の `ValidateOutputPath` の除外と `base/output/path.go` のパスを挿入する関数の欠落の修正の再承認を前提とする（Phase 7 の冒頭のブロッキングタスク）。Phase 8 の AC-41 のガードは Phase 5〜7 の対象の範囲の変更がすべて入った後でなければ green にならない。

---

## 4. テスト戦略

### 4.1 単体テスト

[03_detailed_specification.md](03_detailed_specification.md) §10.1〜§10.5 の各項目を、次のテストファイルで実装する。検証内容・入力・期待値は 03 を参照する（本書では重複しない）。

- `internal/errmsg/errmsg_test.go`: 平らにする契約、`String()` と `Error()` の一致、`IndentedCause`、深い連鎖、`Freeze`、`Join`、`errmsg.Error` の構築と到達性、nil・ゼロ値、AC-08、AC-21（03 §10.1）。
- `internal/errmsg/errmsg_guard_test.go`: AC-24（03 §9.3）、免除の役割の位置（§9.4）、文言と構造の一致（§9.5）、`Part` の非公開と部分の流れ（§9.8）、整形のバイト（§9.9）と、それぞれの自己テスト（§9.10）。
- `internal/redaction/ranges_test.go`: `redactedRanges` の差分テスト（既存の `RedactText` のテストの入力をすべて種にする）と `FuzzRedactedRangesMatchesRedactText`、幅 0 の範囲、段の重なり（03 §10.2）。
- `internal/redaction/message_test.go`: AC-01〜04・07・36〜38、挿入点、実行時の検査、`Config` 未検証、ハンドラ（AC-20）、`Config.RedactLogAttribute`、`TestRedactingHandler_FlattenPanicDoesNotLeakPanicValueToShutdownReport`（失敗の報告に panic 値が現れないこと）、`BenchmarkRedactMessage`（03 §10.3・§10.4）。
- `internal/redaction/redaction_guard_test.go`: AC-25（03 §9.6）と自己テスト。
- `internal/runner/wrap_guard_test.go`: AC-41（03 §9.2）と自己テスト。
- 各エラー型（03 §10.5）: `internal/logging`・`internal/runner`・`internal/runner/config`・`internal/runner/resource`・`internal/runner/base/executor`・`internal/runner/base/privilege` の既存テストファイルに、部分の並びと文言の確認を追加する。

### 4.2 統合テスト

AC-12・AC-16・AC-31・AC-34 の例示のシナリオは、エラーの発生元から `RedactingHandler` を通った後のレコードまでを通す（01「テストの入力についての制約」、03 §10.6）。

- AC-12・AC-34: `cmd/runner/integration_pre_execution_error_test.go`。`runMainWithSlackMock` で起動し、Slack のメッセージ組み立て（`buildPreExecutionError`）までを通して `Error Message` を確かめる。`vars` の中の未定義変数を使う。
- AC-16: `internal/runner/multi_group_error_integration_test.go`。`captureExecutionErrorReport` が内側のコールバックハンドラを `redaction.NewRedactingHandler` で包み、2 つの group の失敗を `HandleExecutionError` に通して `error_message` を確かめる。`Identifier` が置き換えられずに残ることと、`Text` の対照値が置換文字列になることの両方を確かめる。
- AC-31: `internal/runner/runner_test.go`。`executeGroups` の経路で context を中断してから group を失敗させ、返した `*cancelledRunError` を AC-16 と同じ `captureExecutionErrorReport`（内側のコールバックハンドラは `redaction.NewRedactingHandler` で包まれる）経由で `logging.HandleExecutionError` に通し、`error_message` を確かめる。`Identifier`・`Path` が置き換えられずに残ることと、`Text` の対照値が置換文字列になることの両方を確かめる。

### 4.3 層の切り分け

- AC-37 は `redactSegments` のテストで、検出の種類ごとに「各断片だけに `RedactText` と値全体置換を適用しても秘密が見えたまま残る」ことを先に確かめてから、境界をまたぐ検出の結果を確かめる。
- AC-07 はハンドラのテストで確かめる。`RedactMessage` の単体テストでは、機密を示す属性名の下の値ごと置換は再現できないためである。
- AC-38 は `redactSegments` に範囲外の役割を持つ `errmsg.Segment` を直接与えて確かめる（`Segment` の欄が公開である理由）。
- AC-11 は、定数式の `Message` の部分が書き換えられず、原因の部分が `RedactText(原因の Error())` と同じ結果になることを `RedactMessage` のテストで確かめる。
- AC-32 は `cancelledRunError` の単体テスト（文言と到達性）と、`executeGroups` の経路のテスト（返すエラーの中身）の 2 層で確かめる。
- AC-41 は `wrap_guard_test.go`（範囲の網羅）と、例示のシナリオのテスト・各エラー型のテスト（実際の宣言）の 2 層で確かめる。
- AC-16 は、1 つの記録の中に残る `Identifier`（group 名）と置換される `Text` の対照値を並べる。対照値が置換されることは記録が `redaction.NewRedactingHandler` を通ったことの証拠になり、識別子が残ることは部分ごとの描画の証拠になる。どちらか片方だけでは層を切り分けられない。
- AC-31 も同じく、1 つの記録の中に残る `Identifier`・`Path` と置換される `Text` の対照値を並べる。`executeGroups` が返す `*cancelledRunError` の構造化メッセージを直接読むだけでは、本番の報告の境界（`logging.HandleExecutionError` が `errmsg.Message` として記録し、`RedactingHandler` が `RedactMessage` で描画する）が役割の情報を落とす変更に気づけない。返したエラーを `captureExecutionErrorReport` に通し、対照値が置換されることと `Identifier`・`Path` が残ることの両方を同じ記録で確かめることで、記録の境界と redaction を横断したことを切り分ける。

### 4.4 実装時に行うテスト失敗確認

各 Phase の完了時に、次の変異を入れて当該テストが失敗することを確認し、コミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。§4.1 のテストは、この確認を済ませてからコミットする。

| Phase | 変異 | 失敗するテスト |
|---|---|---|
| 1 | 平らにする処理で `errors.As` を使い、奥の `Structured` も探す | `TestMessage_SegmentsFollowFlatteningContracts` |
| 1 | 深さの上限を仮に戻す | `TestMessage_DeepChainMatchesFmtErrorfChain` |
| 1 | `IndentedCause` の末尾の改行の除去を外す | `TestIndentedCause_MatchesGroupErrorFormatting` |
| 1 | `Const` に変数を渡す本番コードを一時的に置く | `TestProductionConstCallsUseConstantExpressions` |
| 1 | 許可位置の表に無い別パッケージの関数（例: `internal/logging/slack_handler.go`）に `errmsg.Ident` の呼び出しを置く | `TestProductionExemptRoleCallsAreInAllowedPositions` |
| 1 | `errmsg` の外で `errmsg.Part{}` を本番ファイルに置く | `TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg` |
| 1 | 構造化メッセージを実装する型の `Error()` を 2 文にする | `TestProductionStructuredErrorsRenderFromTheirMessage` |
| 2 | 残す範囲の 1 つ（例: `Bearer ` の前置き）を置き換える範囲に含める | `TestRedactedRanges_MatchesRedactText`・`FuzzRedactedRangesMatchesRedactText` |
| 2 | 幅 0 の範囲を捨てる | `TestRedactedRanges_ZeroWidthRange` |
| 2 | 前の段の置換文字列に重なる範囲で、置換文字列の全体を取り込まないようにする | `TestRedactedRanges_StageOverlap` |
| 3 | `Identifier` の断片にも値全体置換を適用する | `TestRedactMessage_IdentifierSegmentsAreExempt` |
| 3 | 値全体置換を本文全体で 1 回判定する | `TestRedactMessage_WholeValueIsPerSegment` |
| 3 | 境界をまたぐ検出で `Identifier` のバイトも隠す | `TestRedactMessage_CrossBoundaryDetection` |
| 3 | 既定の分岐で `Text` の全段を適用しない | `TestRedactSegments_UnknownRoleFallsBackToText` |
| 3 | `redactedRanges` の結果の検査を外す | `TestRedactMessage_RangeMismatchReportsFailure` |
| 3 | 平らにする処理の `recover` を外す | `TestRedactMessage_FlattenPanicReportsFailure` |
| 3 | `ErrMessageFlattenPanic.Error()` が `PanicValue` を描画する（既存の `ErrLogValuePanic` と同じ `%v` にする） | `TestRedactingHandler_FlattenPanicDoesNotLeakPanicValueToShutdownReport` |
| 3 | redaction の本番コードで `errmsg.Role(99)` の変換を行う、または `Segment.Role` に代入する（`case` での参照は許されるため、参照ではなく値の作成を変異させる） | `TestProductionRedactionDoesNotConstructRoles` |
| 4 | `HandleExecutionError` で `Freeze()` を外し、stderr と記録がそれぞれ `ReportMessage()` を評価する | `TestHandleExecutionError_EvaluatesCauseOnce` |
| 5 | `GroupErrors` を `Merge` ではなく平らにして作る | `TestGroupErrors_MergePreservesIdentifierSegments` |
| 6 | `Field.String()` の `vars.<name>` の組み立てを変える | `TestLevelAndField_StringMatchesLegacyFormat` |
| 6 | `ExpandWorkDir` の相対パス拒否で、パスを `Path` ではなく `Ident` にする | `TestExpandWorkDir_RelativePathError` |
| 7 | `killAfterCancelError` を `Cause` だけの 1 原因にする | `TestKillAfterCancelError_TextAndReachability`（2 経路の両方） |
| 7 | 一時ディレクトリの 2 つ目のラップの前置きを 1 つ目と同じにする | `TestTempDirManager_Create_WrapTexts`（`os.Chmod` の行） |
| 7 | `cmd/runner` の 4 か所のうち 1 か所で原因を `Err` に移さず `Message` に残す | `cmd/runner/main_test.go::TestPreExecutionCauseReachability`（Phase 7 で追加する。Phase 4 の完了時点ではこのテストはまだ無い） |
| 8 | `executeGroups` が `cancelledRunError` の代わりに `errors.Join(ctxErr, err)` を返す | `TestRunner_CancelledRunErrorMessageKeepsIdentifiers`（返すエラーの動的な型が `*cancelledRunError` であることの確認） |
| 8 | `logging.HandleExecutionError` が `execErr.ReportMessage().Freeze()` ではなく、原因を連結した redaction 前の 1 本の文字列を `error_message` に記録する | `TestRunner_CancelledRunErrorMessageKeepsIdentifiers`（全文が値全体置換で置換文字列になり、中断の原因の文言と `Identifier`・`Path` が消えることの確認）・`TestRunner_MultiGroupSensitiveNamesSurvive` |
| 8 | 対象の範囲の中の関数に `fmt.Errorf` を一時的に置く | `TestInScopeWrapsUseStructuredErrors` |
| 8 | 明示の一覧の型から `StructuredMessage` を外す | `TestInScopeErrorTypesDeclareStructuredMessage` |
| 8 | 日英どちらかから検査対象の語を外す | `scripts/verification/check_structured_message_redaction_docs.sh`（`make verify-docs-checks`） |

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| 範囲を返す関数と `RedactText` の規則が食い違い、秘密が漏れる | セキュリティの後退 | 規則と段の順序を共有し、写しを作らない。既存のテストの入力の全体を種にした差分テストとファジング、`RedactMessage` の実行時の検査（不一致は `RedactionFailurePlaceholder`）の 3 層で固定する |
| 部分の境界をまたぐ検出の描画が `RedactText` と異なる | 秘密の漏れ、または本文の過剰な置換 | `Identifier` のバイトだけを残す契約を AC-37 のテストで固定し、どの入力でも断片単独では秘密が見えたまま残ることを先に確かめる |
| `Error()` の文言が構造の再現の誤りで変わる | AC-18 違反、既存の運用の混乱 | `Structured` の `Error()` を 1 文に固定するガード（§9.5）と、各エラー型の文言のテスト（§10.5）。`IndentedCause` の整形は `GroupError.Error()` と比較する（03 §2.4.1・§10.1） |
| 02 §3.8.1 の `ValidateOutputPath` の除外と `base/output/path.go` のパスを挿入する関数の欠落が規則 (ii) と矛盾し、出力パスを含む原因が過剰に置換される | タスクの目的（本文が消えない）が一部達成されない | Phase 7 の冒頭のブロッキングタスクで 02・03 の範囲と役割の許可位置の修正と再承認を得る。再承認まで Phase 7 の resource の実装と Phase 8 のガードを開始しない |
| `make verify-docs-checks` が CI から実行されない | 後からの編集で文書の検査が黙って壊れうる | 本タスクの範囲では Phase 8 の完了ゲートと PR レビューでローカルに実行する。CI への組み込みは本タスクの対象外として、§1.3 の外部前提に記録する |
| 深い連鎖・自己参照の連鎖でスタックがあふれる | プロセスの異常終了 | 変更前の `fmt` の `%v` と同じ挙動であり後退ではない（02 §3.1.3）。深さの上限を置かない理由をテスト（深い連鎖の一致）で固定する |
| 失敗の記録（`ErrMessageFlattenPanic`・`ErrMessageRangeMismatch`）が本文や panic 値を含む | 秘密の漏れ | `ErrMessageRangeMismatch` は本文・範囲の内容を持たない。`ErrMessageFlattenPanic` も `PanicValue`・スタックトレースを `Error()` に描画しない（03 §3.4 の編集上の修正。§1.3 の食い違い表）。`TestRedactingHandler_FlattenPanicDoesNotLeakPanicValueToShutdownReport` で報告の出力を固定する |
| 既存テストの削除で確認が減る | 退行の見逃し | 削除の前後で `go tool cover -func` を関数ごとに比較し、差をコミットメッセージに記録する。差分テストの種には既存の `RedactText` のテストの入力の全体を含める |
| `Config.RedactLogAttribute` の fail-closed 化が既存の呼び出しに影響する | 公開関数の挙動の変化 | 本番の呼び出しは `:371` の自身の再帰だけである（確認済み）。テストで新しい挙動を固定する |
| Phase 8 まで AC-41 のガードが無いため、途中の Phase で範囲外のラップが混入する | 網羅の漏れ | Phase 5〜7 の各完了時に、対象のファイルのラップの一覧を 03 の表と突き合わせることを PR レビューで確認する。Phase 8 のガードが最終的な網羅を担う |
| `sample/slack-group-notification-test.toml` の変更が互換テストに影響する | CI の失敗 | 互換テスト 3 件が読み込みの成否と group の非空だけを確かめることを確認済み。Phase 8 で `internal/runner/config` のテストを実行して確かめる |
| 日英の文書の内容が食い違う | AC-26 違反 | 日本語版を先に更新し、`/mktrans` で反映する。検査スクリプトで両方の必須語を機械的に確かめ、内容の一致は PR レビューで確認する |
| Slack の手動確認が実行環境に依存する | 検証の未実施 | `make slack-group-notification-test` は webhook の環境変数が無いと警告のうえ実行される。実行できない場合は、その旨と代替（E2E テストの `Error Message` の確認）をコミットメッセージに記録する |

---

## 6. 実装チェックリスト

- [ ] 02 §3.8.1 の `ValidateOutputPath` の除外と `base/output/path.go` のパスを挿入する関数の欠落の修正が再承認され、03 §9.1・§9.4 に同じ関数が入っている（Phase 7・8 の前提）
- [ ] PR-1 マージ済み（対象ステップ: Phase 1。`internal/errmsg` とそのガードが green）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2。差分テストとファジングが green、網羅率の差を記録済み）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3。`RedactMessage` のテストとベンチマークが green）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4。2 つのレコードが構造化メッセージになり、stderr と文言が不変）
- [ ] PR-5 マージ済み（対象ステップ: Phase 5。runner の 4 型と `cancelledRunError` が green）
- [ ] PR-6 マージ済み（対象ステップ: Phase 6。`Level`・`Field` と `ErrUndefinedVariableDetail` が green）
- [ ] PR-7 マージ済み（対象ステップ: Phase 7。コマンドの実行の経路と `cmd/runner` が green）
- [ ] PR-8 マージ済み（対象ステップ: Phase 8。AC-41 のガード・例示のシナリオ・日英の文書・`make verify-docs-checks` が green）
- [ ] すべての AC が §7 の検証で green
- [ ] §4.4 の変異確認をすべて実施し、各コミットメッセージに記録
- [ ] `make slack-group-notification-test` の結果を記録

---

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（ガードテスト・`make` ターゲット・コミット済みスクリプト）、`manual`（PR やデプロイでの観察）を表す。テスト名は `path::TestName` で示す。本書で新設するテストは、Phase の完了時に §4.4 の変異で失敗することを確認する。

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 1、Phase 3 | `test`: `internal/redaction/message_test.go::TestRedactMessage_IdentifierSegmentsAreExempt`（語・key=value の形・値形式の検出の 3 種の入力） |
| AC-02 | Phase 1、Phase 3 | `test`: `TestRedactMessage_PathSegmentsFollowRedactTextOnly`（語だけでは書き換えられないこと） |
| AC-03 | Phase 1、Phase 3 | `test`: `TestRedactMessage_PathSegmentsFollowRedactTextOnly`（`RedactText` が反応する値はマスクされること） |
| AC-04 | Phase 3 | `test`: `TestRedactMessage_TextSegmentsFollowRedactTextAndWholeValue`（同じ本文の他の断片が残ること） |
| AC-07 | Phase 3 | `test`: `TestRedactingHandler_SensitiveKeyReplacesMessage`（機密を示す属性名の下で値ごと置換） |
| AC-08 | Phase 1、Phase 3 | `test`: `internal/errmsg/errmsg_test.go::TestMessage_ZeroValuesAndUndefinedRoles`、`internal/redaction/message_test.go::TestRedactSegments_ZeroRoleFallsBackToText`（ゼロ値の役割）と `TestRedactSegments_UnknownRoleFallsBackToText`（範囲外の役割、AC-38） |
| AC-09 | Phase 1、Phase 3 | `test`: `internal/errmsg/errmsg_test.go::TestMessage_SegmentsFollowFlatteningContracts`（構造を持たない原因が 1 つの `Text` になること）、`internal/redaction/message_test.go::TestRedactMessage_UnstructuredCauseWholeValueReplaced` |
| AC-10 | Phase 1、Phase 3 | `test`: `TestMessage_SegmentsFollowFlatteningContracts`（外側の宣言された部分と内側の `Text`）、各エラー型のテスト |
| AC-11 | Phase 3、Phase 4 | `test`: `internal/redaction/message_test.go::TestRedactMessage_OutOfScopeBodyMatchesRedactText`（定数式の `Message` の部分が書き換えられないこと、原因の部分が `RedactText` の結果と一致すること） |
| AC-12 | Phase 4、Phase 5、Phase 6、Phase 8 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupVarsUndefinedVariableIdentifiersSurviveRedaction`（Slack の `Error Message` まで通す） |
| AC-16 | Phase 5、Phase 8 | `test`: `internal/runner/multi_group_error_integration_test.go::TestRunner_MultiGroupSensitiveNamesSurvive`（記録を `redaction.NewRedactingHandler` に通し、両方の group のエラーが宣言する `Identifier` の部分が置き換えられずに出ること、および `Text` の対照値が置換文字列になること。終了コードのエラー書式は `Path` を宣言しないため、このシナリオで `Path` は現れない） |
| AC-18 | Phase 1、Phase 4〜7 | `test`: `internal/errmsg/errmsg_test.go::TestMessage_StringEqualsErrorForStructuredTypes`・`TestMessage_DeepChainMatchesFmtErrorfChain`・`TestIndentedCause_MatchesGroupErrorFormatting`、`internal/logging/pre_execution_error_test.go::TestPreExecutionError_DetailMessage`・`TestExecutionError_ReportMessage`、`internal/runner/config/errors_test.go::TestLevelAndField_StringMatchesLegacyFormat`、各エラー型の文言のテスト、`cmd/runner/main_test.go::TestStartupDirPermAudit_CheckerInitFailureReturnsPreExecutionError`・`TestPreExecutionCauseReachability`（4 か所で原因が `Err` に移っても `Detail()` の文言が変わらず、付け替えた原因に `errors.Is`・`errors.AsType` が届くこと）。`static`: `internal/errmsg/errmsg_guard_test.go::TestProductionStructuredErrorsRenderFromTheirMessage` |
| AC-19 | Phase 4、Phase 8 | `test`: `internal/logging/pre_execution_error_test.go::TestHandleExecutionError_WithWrappedError`・`TestHandleExecutionError_CauseFormatting`（最終報告の stderr の `Details:` の完全一致）、`internal/runner/multi_group_error_integration_test.go` の `Details:` の比較が変更なしで通る。`TestHandlePreExecutionError_AllTypes` は前段の失敗の stderr の補助的な固定として残る。`manual`: 既存の `cmd/runner` の統合テストの stderr の確認 |
| AC-20 | Phase 3、Phase 4 | `test`: `internal/redaction/message_test.go::TestRedactingHandler_MessageAttributeBecomesString`（後段のハンドラが文字列を受け取ること） |
| AC-21 | Phase 1 | `test`: `internal/errmsg/errmsg_test.go::TestMessage_LogValueReturnsUnredactedString`（`slog.NewTextHandler` などの出力が `String()` と一致すること） |
| AC-22 | Phase 4、Phase 8 | `test`: `internal/logging/slack_handler_test.go::TestBuildPreExecutionError_InterpolationContract` と、構造化メッセージの本文を渡す追加の行（要約文が 500 byte の上限でも先頭に残ること）。既存のテストが変更なしで通る |
| AC-23 | Phase 3、Phase 4 | `test`: `internal/logging/pre_execution_error_test.go::TestHandleExecutionError_DoesNotNotifySlack`・`TestHandlePreExecutionError_SlackNotification`、`cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupPreparationFailureNotifiesAndReportsOnce`（通知の件数・`message_type`・Scope）。`static`: `internal/logging/notification_contract_guard_test.go`（変更なしで通る） |
| AC-24 | Phase 1、Phase 4〜7 | `static`: `internal/errmsg/errmsg_guard_test.go::TestProductionConstCallsUseConstantExpressions` と自己テスト。`test`: 各エラー型の部分の並びのテスト（`Const` の断片が期待どおりであること） |
| AC-25 | Phase 3 | `static`: `internal/redaction/redaction_guard_test.go::TestProductionRedactionDoesNotConstructRoles` と自己テスト |
| AC-26 | Phase 8 | `static`: `make verify-docs-checks`（`scripts/verification/check_structured_message_redaction_docs.sh` が security-architecture と security-risk-assessment の日英の必須語・免除の記述の区別・旧称の不在、および `package_reference.md` の `errmsg/` を検査し、`check_structured_message_redaction_docs_selftest.sh` が検査の空振りを防ぐ）。`manual`: 日英の内容を突き合わせてレビューする |
| AC-27 | 各 Phase | `static`: 各 Phase の `make fmt`（Go を変更した場合）・`make test`・`make lint` |
| AC-31 | Phase 5、Phase 8 | `test`: `internal/runner/runner_test.go::TestRunner_CancelledRunErrorMessageKeepsIdentifiers`（返したエラーの動的な型が `*cancelledRunError` であること、返したエラーを `captureExecutionErrorReport` 経由で `logging.HandleExecutionError` と `redaction.NewRedactingHandler` に通した後の `error_message` に中断の原因の文言と失敗した group が宣言する `Identifier`・`Path` の部分が出ること、`Text` の対照値が置換文字列になること） |
| AC-32 | Phase 5 | `test`: `internal/runner/runner_test.go::TestCancelledRunError_TextAndReachability`（文言が `errors.Join` と同じ、`errors.Is(err, ctx.Err())`、失敗した group の原因への到達）。`test`: `TestRunner_CancelledRunErrorMessageKeepsIdentifiers`（返すエラーの中身） |
| AC-33 | Phase 5、Phase 8 | `static`: `internal/runner/group_errors_guard_test.go::TestProductionCodeDoesNotProbeMultiErrorShape`（変更なしで通る）、`internal/runner/wrap_guard_test.go::TestInScopeErrorTypesDeclareStructuredMessage`（`cancelledRunError` が `StructuredMessage` を宣言すること） |
| AC-34 | Phase 6、Phase 7、Phase 8 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GlobalExpansionUndefinedVariableIdentifiersSurviveRedaction`（Slack の `Error Message` に固定の文言と `api_key` が出ること） |
| AC-35 | Phase 3、Phase 4 | `test`: `internal/redaction/message_test.go::TestRedactMessage_TextSummaryWholeValueReplaced`（`TextSummary` の部分が値全体置換だけに反応する入力で置換文字列になること）、`internal/logging/pre_execution_error_test.go::TestPreExecutionError_DetailMessage`（値を含む `Message` が `Text` として扱われること） |
| AC-36 | Phase 3 | `test`: `TestRedactMessage_WholeValueIsPerSegment`（`Identifier` だけが語を含む本文で `Text` の断片が書き換えられないこと） |
| AC-37 | Phase 3 | `test`: `TestRedactMessage_CrossBoundaryDetection`（検出の種類ごとに、各断片単独では秘密が見えたまま残ること、`Identifier` のバイトが残ること、`Identifier` 以外の連続区間ごとに 1 つの置換文字列になること） |
| AC-38 | Phase 3 | `test`: `TestRedactSegments_UnknownRoleFallsBackToText`（`redactSegments` に `errmsg.Segment{Role: errmsg.Role(99), ...}` を直接与える） |
| AC-41 | Phase 4〜8 | `static`: `internal/runner/wrap_guard_test.go::TestInScopeWrapsUseStructuredErrors`・`TestInScopeErrorTypesDeclareStructuredMessage`・`TestScopeCatalogNamesExist` と自己テスト。`test`: 各エラー型の部分の並びのテスト（対象の経路の宣言。`(*PreExecutionError).DetailMessage`・`(*ExecutionError).ReportMessage` は Phase 4） |

---

## 8. 横断検索チェックリスト

`make test`・`make lint` が検出できない残存参照・用語の整合だけを挙げる。§7 の表と重複する項目は置かない。

- [ ] 削除した `WithPlaceholder`・`WithAdditionalKeyValuePatterns` の名前が、本番コード・テスト・コメントのどこにも残っていないこと。Go のソースだけを対象にする（`rg -n '\bWithPlaceholder\b|\bWithAdditionalKeyValuePatterns\b' internal cmd`）。設計文書（02・03・本書）と過去のタスクの文書には、決定の記録として旧名が残る。`TestOptionalParameter_EnvKeyWithPlaceholder` は語の途中に現れるので、単語境界で除外される。`make test` は参照だけを検出し、コメントの旧名は検出しない。
- [ ] 旧称「値まるごと判定」と "whole-value detection" が `docs/dev/architecture_design/security-architecture.ja.md` と `.md` に残っていないこと（`docs/translation_glossary.md:658` の旧称の注記は除く）。`docs/user/security-risk-assessment.ja.md:301`・`.md:305` の識別子の免除の記述は Phase 8 の文書タスクで更新し、(1) `error` 属性・`record.Message` についての説明として正確であること、(2) 構造化メッセージの `error_message` では宣言された `Identifier` の断片が値全体置換を受けずに残り、普通の `error` 文字列・`record.Message` は従来どおり値全体置換で全文が置換されうること、の 2 点を満たすこと。英語版は `/mktrans` で反映し、`check_structured_message_redaction_docs.sh` の必須語と `make verify-docs-checks` で機械的に確かめる。
- [ ] `docs/translation_glossary.md` に、Phase 8 で新しく使う用語（構造化メッセージ = structured message、断片 = segment、役割 = role など）の対訳が `/mktrans` により登録されていること。値全体置換 = whole-value replacement は登録済みである（`:658`）。
- [ ] `internal/runner/config` の `Level`・`Field` と同名の型が他のパッケージにあり、import の別名が必要になっていないこと（`rg -n "type Level |type Field "`。config の中には既存の同名型は無い）。
- [ ] 0176 の `error_message` の本文が値全体置換で `[REDACTED]` になることを固定するテストが無いこと（`rg` で確認済み。3 つの `REDACTED` と `error_message` を含むテストはいずれも識別子の免除または key=value を確かめている）。新しく加えたテストがこの逆を固定していないことを確認する。

---

## 9. Success Criteria

- **機能**: AC-01〜AC-04・AC-07〜AC-12・AC-16・AC-18〜AC-27・AC-31〜AC-38・AC-41 を検証するテスト・ガード・`make` ターゲットが green。`error_message` は構造化メッセージとして記録され、`RedactMessage` を通った文字列として後段のハンドラに渡る。
- **品質**: 各 Phase の `make fmt`・`make test`・`make lint` が green。§4.4 の変異確認をすべて実施し記録済み。`go tool cover -func` の差が削除台帳の範囲に収まっている。
- **セキュリティ**: key=value・`Bearer `・`Basic ` の次の語・値形式の検出・属性名による判定でマスクされていた値が、構造化メッセージでもマスクされる。値全体置換を免れるのは型で役割を宣言した断片だけであり、`Identifier` の免除と `Path` の値全体置換の対象外は承認済みの境界である（02 §5.1）。範囲を返す関数の不一致は fail-closed に倒れる。
- **一貫性**: 対象のエラーの `Error()`・`PreExecutionError.Detail()`・stderr の文言が変更前と同じである。通知の件数・`message_type`・`error_type`・Scope・Slack のフィールド構成が変わらない。
- **文書**: security-architecture の日英に、構造化メッセージの役割ごとの redaction、`Path` の境界、構造を持たないエラーの扱いが記載され、security-risk-assessment の日英の免除の記述が構造化 `error_message` の `Identifier` の断片を正しく説明し、`make verify-docs-checks` が green。`package_reference.md` に `internal/errmsg` がある。

---

## 10. 次のステップ

- 本書が承認されたら、Phase 1（PR-1）から順に実装を開始する。Phase 7 の冒頭のブロッキングタスク（02 §3.8.1 の `ValidateOutputPath` の除外と `base/output/path.go` のパスを挿入する関数の欠落の修正の再承認）を、Phase 7 の実装開始前に完了する。
- 実装の完了後は、§7 の全 AC と §4.4 の変異確認の記録をレビューし、Phase 8 の手動確認（Slack）の結果を PR に記録する。
- スコープの外にあるものは、既存の Issue で扱う: dynlib・shebang 検証のエラー型の構造化は #1196、設定の展開・検証のエラー型の構造化は #1197。
