# 実装計画書: group 検証エラー通知での失敗ファイル名の表示

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-15 |
| Review date | 2026-09-15 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 出発点: [0172 実装計画書 §10 の follow-up](../0172_slack_notification_message_unification/03_implementation_plan.md#10-次のステップ)
- 表示安全な補間契約と動的な値の一覧: [0172 アーキテクチャ設計書 §3.5](../0172_slack_notification_message_unification/02_architecture.md)
- テストヘルパ配置: [test_organization.md](../../dev/developer_guide/test_organization.md)

本書の用語は [02_architecture.md](02_architecture.md) の「用語」に従う（group 検証エラー、失敗ファイル一覧、収集失敗、`failed_file_paths` 属性、通知ビルダー、共有コンストラクタ、昇順正規化、表示上限、全件表示／部分表示、省略通知、掲載件数 k／省略件数 m）。

---

## 1. 実装の概要

### 1.1 目的

group 検証エラーの Slack 通知に失敗ファイル一覧を表示し、グローバル検証エラーと同じ共有コンストラクタ・同じ構造化属性 `failed_file_paths`・同じ通知ビルダーで描画する。あわせて、収集失敗（コマンドのパス解決失敗）でも解決に失敗した対象を一覧として運び、`verification.Error` の生成と `Details` の昇順正規化を非公開コンストラクタ 1 箇所に集約し、`runerrors` の死んだシンボルと `Component` の生リテラルを整理する。設計の詳細は [02_architecture.md](02_architecture.md) §1〜§4 を参照する。

### 1.2 実装方針

1. 失敗ファイル一覧は `verification.Error.Details` だけから取り、自由文 `Message` へ連結しない（02_architecture.md §1.2 原則 1・3）。
2. `PreExecutionError` の組み立ては `runerrors.NewVerificationPreExecutionError` に一本化し、両発火元はそれを呼ぶだけにする（02_architecture.md §3.2）。両発火元が実際にこれを呼ぶことは `go/ast` の静的ガードで固定する（02_architecture.md §7.9）。
3. `Details` の並びは `manager.go` の非公開コンストラクタでだけ正規化する。発火元もビルダーも並びを変えない（§3.2.3・§3.7）。
4. 表示上限の判定は `common.WithinInterpolationLimit` に問い合わせ、上限値や変換規則をビルダーへ書き写さない（02_architecture.md §3.1）。
5. redaction・通知種別定義・Slack フィールド集合・`error_type`・`verification.Error` の型と `Error()` は変更しない（§3.4）。
6. Phase の順序は 02_architecture.md §8 の実装優先順位（1 → 2a → 2b → 3 → 4 → 5）を保つ。各コンポーネントの単体テストと静的ガードは対応する実装と同じ Phase に置き、§8 Phase 4 には横断的なテスト（統合テスト・redaction 回帰・ベンチマーク）を残す（§3.3 順序の根拠）。
7. Go のソースコメント・識別子・文字列リテラルは英語で書く。
8. `verification.Error` と `PreExecutionError` はどちらもフィールドが公開されており（01 §対象外により型は変えない）、「唯一の生成箇所であること」と「手組みしないこと」を型で強制できない。両者は `go/ast` の静的ガードで固定し、ガードは複合リテラル（値・ポインタ・elided 形）とフィールド代入の各構築形を列挙する（Phase 2b.1・2b.4）。

**02_architecture.md への反映。** 本書の作成時に次の 3 点が 02_architecture.md の記述と異なることが分かったため、02_architecture.md の §3.2.1・§3.6・§5.2・§8 を本書に合わせて修正し、`Status` を `draft` に戻して再レビューを依頼した（修正内容は同書の `Comments` に記録）。

- §8 Phase 4 が挙げるコンポーネント単位のテストを、対応する実装と同じ Phase に置く（§3.3）。
- §5.2 は `internal/logging/notification_test.go` の一覧対応テストに `failed_file_paths` の行を足すとするが、同テストはフィールド見出しの集合を検査しており `failed_file_paths` はフィールドではない。テストは変更せず、0172 アーキテクチャ設計書の表にだけ行を足す（§1.3）。
- `runerrors/` の説明は README に加えて `docs/dev/developer_guide/package_reference.md` にもあるため、そちらも更新する（§1.3）。

### 1.3 既存コード調査結果

2026-09-15 時点の commit `066c9e59`（`docs(0175): Approved the architecture document`）のコードを読んだ結果を示す。行番号は同じ commit のものである。02_architecture.md が `b1d7c81c` 時点で引いた行番号は、以下で再確認した範囲では一致した。

#### `verification.Error` の生成箇所と収集失敗

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| `&Error{...}` 構造体リテラル（本番） | `internal/verification/manager.go:170`（`VerifyGlobalFiles` 検証失敗）、`:198`（`VerifyGroupFiles` 収集失敗）、`:242`（`VerifyGroupFiles` 検証失敗） | この 3 箇所が本番で `Error` を生成する全てである（`rg -n "&Error\{" internal/verification/*.go` の非テスト結果はこの 3 件のみ。テストの `errors_test.go` に 7 件あるが型の単体テストであり対象外）。非公開コンストラクタ `newVerificationError` に集約する |
| `collectVerificationFiles` | `manager.go:260-293` | 解決失敗の 1 件目で `return nil, err` する（`:274-285`）。呼び出し元は `VerifyGroupFiles`（`:195`）とテスト（`manager_test.go:679,699,713,730,765,787,808`）のみ。解決失敗の対象を全て集めて返す形へ変える |
| `collectVerificationFiles` の `slog.Warn` を固定する既存ガード | `internal/identifier/identifier_guard_test.go:122` | `(*Manager).collectVerificationFiles` の中に `slog.Warn("Failed to resolve command path")` が `"group"` → `input.Name` の形でちょうど 1 件あることを固定している。書き換え後も `slog.Warn` は同じ関数・同じ属性のまま残す（別の補助関数へ切り出さない） |
| 収集失敗の `Err` | `manager.go:201` | `fmt.Errorf("failed to collect verification files: %w", err)` でコマンド文字列を包む。パスを含まないセンチネル `ErrGroupVerificationCollectionFailed` に置き換える |
| センチネル | `internal/verification/errors.go:19-22` | `ErrGlobalVerificationFailed`・`ErrGroupVerificationFailed` の隣に `ErrGroupVerificationCollectionFailed` を追加する |
| `Error.Error()` | `errors.go:161-181` | 変更しない（`Details` 分岐は `:171-173`） |
| 解決失敗の既存テスト | `manager_test.go:773-810`（`TestCollectVerificationFiles` の `report_command_with_expansion_error`・`report_command_with_resolution_error`。Phase 2b.1 で旧 `skip_command_with_*` から改名） | `collectedFiles` が nil でエラーが返ることを見ている。新しい返り値（解決に失敗した対象の一覧）に合わせて更新する |
| `Details` の並びを見る既存テスト | なし（`rg -n "Details" internal/verification/manager_test.go` は一致なし） | 3 経路の昇順を新しいテストで固定する |

#### 発火元と `Component` の生リテラル

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| group 発火元 | `internal/runner/runner.go:426-439` | `fmt.Sprintf("Total: ...")` で本文を組み、`Component: "runner"`（`:435`）。共有コンストラクタ呼び出しへ置き換える。`resource` は既に import 済み（`:27`） |
| グローバル発火元 | `cmd/runner/main.go:391-402` | `Message: err.Error()`（`:397`）。`*verification.Error` なら共有コンストラクタ、それ以外は従来どおり |
| `Component` の生リテラル | `cmd/runner/main.go:136,186,199,243`（`"main"`、`PreExecutionError`）、`:692`（`"runner"`、`ExecutionError`）、`internal/runner/runner.go:435`（`"runner"`） | `rg -n 'Component:\s+"' --glob '*.go'` の非テスト結果はこの 6 件と `internal/common/logschema.go:44`（属性キー名 `"component"` であり対象外）のみ。6 件を `string(resource.ComponentMain)` / `string(resource.ComponentRunner)` に置き換える |
| `resource.Component` 定数 | `internal/runner/resource/types.go:372-385` | `ComponentRunner`・`ComponentConfig`・`ComponentVerification`・`ComponentMain`・`ComponentLogging` が既にある。追加不要 |

#### `runerrors` パッケージ

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| 既存シンボル | `internal/runner/runerrors/types.go`（`ErrorSeverity`・`ErrorType`・`ClassifiedError`）、`classification.go`（`ClassifyVerificationError`）、`logging.go`（`LogCriticalToStderr`・`LogClassifiedError`） | `rg -n "runerrors\." --glob '*.go'` でパッケージ外の参照は 0 件。本番の import も 0 件。全て削除する |
| 既存テスト | `classification_test.go`・`logging_test.go` | 削除する |
| パッケージ doc | `types.go:1`（"Package runerrors provides error classification and handling for the runner."） | 共有コンストラクタの責務に書き換える。Phase 2a で `doc.go` を置き、以後もそこに置く（非公開シンボルの削除でパッケージにファイルが無くならないようにする） |
| 文書の記述 | `README.ja.md:146`（「一元化エラー処理」）、`README.md:146`（"Centralized error handling"）、`docs/dev/developer_guide/package_reference.md:42,109`（"Centralized error handling"） | 02_architecture.md は README のみを挙げるが、`package_reference.md` にも同じ記述が 2 箇所ある。3 文書とも更新する |
| `make deadcode` | `Makefile:771` | commit `066c9e59` 時点の出力（`unreachable func` 10 行）に `runerrors` は現れない。本番から import されていないため走査対象に入らないからである。Phase 2b で `runner.go`・`main.go` が import した後は走査対象になる |

#### 通知側（`internal/logging`・`internal/common`）

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| `PreExecErrorAttrs` | `internal/common/logschema.go:37-45` | `ErrorType`・`ErrorMessage`・`Component` の 3 キー。`FailedFilePaths: "failed_file_paths"` を追加する |
| 補間契約 | `internal/common/interpolation.go:38`（`interpolationMaxBytes`）、`:112-114`（`truncateInterpolated` は上限以下なら入力をそのまま返す）、`transformInterpolated(value, truncate bool)` | `WithinInterpolationLimit` を追加する。切り詰めを適用しない変換後の長さで判定する（`transformInterpolated(value, false)` の長さを測る） |
| `PreExecutionError` | `internal/logging/pre_execution_error.go:50-60` | `FailedFilePaths []string` を追加する。`Detail()`（`:75-83`）は変更しない |
| `HandlePreExecutionError` | `pre_execution_error.go:156-168` | `handleErrorCommon`（`:120-151`）の `notificationAttrs` にならい、`FailedFilePaths` が空でないときだけ `failed_file_paths` 属性をレコードに加える |
| `HandleExecutionError` | `pre_execution_error.go:171-180` | `Detail()` と同じ組み立てを重複実装している。[#1156](https://github.com/isseis/go-safe-cmd-runner/issues/1156) を指すコメントを残し、実装は変えない |
| `buildPreExecutionError` | `internal/logging/slack_handler.go:836-866` | `error_message` と `failed_file_paths` から `Error Message` を組み立てる形へ変える。`[]any` のデコード補助は同ファイルに置く |
| `processSlice` | `internal/redaction/redactor.go:1436-1439,1463` | 文字列要素に `RedactText` のみを適用し、`[]any` で返す。変更しない |
| 値全体置換 | `redactor.go:819-823`、パターンは `sensitive_patterns.go:41`、判定は `:131-134` | `KindString` のみ。変更しない |
| 動的な値の一覧テスト | `internal/logging/notification_test.go:206`（`TestNotificationDefinitions_FieldsAreDeclaredInInventory`） | フィールド見出しの集合を検査しており、`Error Message` は既に登録済み。`failed_file_paths` はフィールドではなく `Error Message` の値の材料なので、このテストは変更しない。0172 アーキテクチャ設計書の表（`02_architecture.md:560-580`）にだけ行を追加する（Phase 5） |
| 自由文の役割テスト | `notification_test.go:276-292`（`TestNotificationDefinitions_FieldRolesTransformValues`） | `failed_file_paths` なしのレコードを使う。変更しない |
| ベンチマークの置き場所 | `internal/logging/slack_handler_benchmark_test.go` | 既存のベンチマークファイル。ここに追加する |

#### 静的ガードの再利用

| 対象 | 位置 | 現状と変更 |
|---|---|---|
| `PreExecutionError` リテラルガード | `internal/logging/notification_contract_guard_test.go:39-61`（`TestProductionPreExecutionErrorLiteralsCarryNotificationContext`）、走査は `:120-156` | 本番ファイル全件を `go/ast` で走査し、リテラルが `NotificationContext` を設定することを検査する。新設する 4 つのガード（`verification.Error` 構築ガード・発火元ガード・`runerrors` 公開シンボルガード・`Component` ガード）はこの走査を再利用する |
| 走査補助（非公開） | `notification_contract_guard_test.go:249`（`elidedCompositeLiterals`）、`:269`（`compositeElementType`）、`:286`（`unwrapParen`）、`:299`（`isNamedType`）、`:315`（`parseSource`） | `internal/logging` の `_test.go` に閉じており `runerrors`・`verification` から使えない。`elidedCompositeLiterals` は `compositeElementType`・`unwrapParen` に、`isNamedType` は `unwrapParen` に依存するため、5 つをまとめて `internal/testutil/identitymutationguard/helpers.go` へ移す（§1.5 台帳。Phase 1）。`TestPreExecutionErrorLiteralCheckRecognizesForms`（`:327`）は `checkPreExecutionErrorLiterals` を検査しており、移動後も `internal/logging` に残す |
| 共通走査基盤 | `internal/testutil/identitymutationguard/helpers.go:210`（`ProductionGoFilesInRepo`）、`:244`（`ReadProductionSource`）、`:381`（`ResolveLocalImports`） | そのまま使う |

#### 既存テストへの影響（02_architecture.md §3.5 の再確認）

| テスト | 位置 | 影響 |
|---|---|---|
| `TestRunner_VerificationErrorCarriesGroupScopeAndCleanMessage` | `internal/runner/runner_test.go:2385-2431` | `tu.NewLogRecorder` を直接 `slog.SetDefault` しており RedactingHandler を通らない。既存アサーションは成立し続ける。`FailedFilePaths`・`Component` の検証は RedactingHandler を通す新しいテストで行う |
| `TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope` | `cmd/runner/integration_pre_execution_error_test.go:481-586` | Slack モックサーバー・ハッシュ登録・`bootstrap.SetSlackHandlerFactory`・フラグ退避の手順がここにある。stderr は捕捉していない。group 用の 3 テストとグローバルの拡張はこの手順を共有するため、`cmd/runner/integration_test_helpers.go`（既存、`//go:build test`）へ共通部分を抽出する。stderr の捕捉には同じ `package main` にある既存の `captureStdoutStderr`（計画時点では `cmd/runner/startup_privilege_test.go:120-165`。Phase 4a で `integration_test_helpers.go` へ移した。§1.5）を再利用し、新しい捕捉ヘルパは作らない。同関数は `os.Stdout`・`os.Stderr` をパイプへ差し替え、両パイプを goroutine で並行して読み切り、書き込み側を閉じてから待つため、長い一覧を出す統合テストでもパイプの固定バッファが満杯になって `mainWithExitCode` が止まることがない。`integration_logger_test.go:220-226` の手法は `os.Stderr` の退避と復元だけでパイプを持たず、出力を読めないため使えない。`internal/logging` の `captureErrorOutput`（`pre_execution_error_test.go:428`）は `_test.go` にあり import できない |
| 統合テストのプロセス全体状態 | 同上 `:539-570` | パッケージ変数（`configPath`・`dryRun` など）、`cmdcommon.DefaultHashDirectory`、`slog.Default`、Slack ハンドラファクトリを差し替える。これらのテストは `t.Parallel` を使わない。`dryRun = false` を必ず設定する（dry-run では `VerifyGlobalFiles`・`VerifyGroupFiles` が失敗を `result, nil` で返し（`manager.go:163-165`・`:236-238`）、検証エラーが発火しない） |
| `TestRedactingHandler_SliceStringElementRedaction` | `internal/redaction/redactor_test.go:2884` | 機密要素のマスクと非機密要素の保持を既に固定する。`key` を含む普通のパス要素（`/opt/monkey/data`）が残ることと、同じ文字列が `KindString` 属性では値全体置換されることの対照はまだ無い。サブテストとして追加する |
| `TestVerifyGroupFiles_OldSchema_BlocksExecution` | `internal/verification/manager_test.go:1866` | `Details` 付きの検証失敗を返す。並びは新しいテストで固定するため変更しない |
| `TestVerifyGroupFiles`・`TestVerifyGlobalFiles` | `manager_test.go:567`・`:516` | 収集失敗・昇順のケースは別名の新しいテストとして追加する |

#### 事実として確認した既存挙動

- `DefaultGroupExecutor.verifyGroupFiles`（`internal/runner/group_executor.go:359-376`）は `VerifyGroupFiles` を最初に呼ぶ。設定に存在しない絶対パスのコマンド（例: `/nonexistent/scr-0175-missing`）を置くと、`collectVerificationFiles` の `pathResolver.ResolvePath` が失敗し、収集失敗の `*verification.Error` が `executeGroups` の検証分岐（`runner.go:426`）へ届く。`internal/runner/config` にコマンドの存在を確認する処理は無い（`rg -n "os\.Stat|Lstat|LookPath" internal/runner/config/*.go` の非テスト結果は 0 件）ため、`cmd/runner` の統合テストで収集失敗を起こせる。
- `docsguard`（`internal/testutil/docsguard/docs_guard_test.go:49,78`）は `docs/tasks` の表の列数と Document Status の整合を `make test` で検査する。Phase 5 の 0172 文書への追記（表の行追加・`Comments` の更新）もこの検査を通す。

### 1.4 テストヘルパーの方針

- **`internal/testutil/identitymutationguard/helpers.go`（既存・Classification A）**: `ParseSource`・`IsNamedType`・`ElidedCompositeLiterals`・`UnwrapParen` と、`ElidedCompositeLiterals` が依存する非公開の `compositeElementType` を追加する（`internal/logging` からの移動。§1.5。Phase 1）。`UnwrapParen` は slack_notify ガードの 3 関数も使うため公開名にする。`go/ast` だけに依存し公開 API のみを使うため、この配置でよい。
- **`cmd/runner/integration_test_helpers.go`（既存・`//go:build test`）**: Slack モックサーバーと in-process 実行の共通手順を関数に抽出する（`TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope` から切り出す）。`package main` の非公開フラグ変数を退避するため Classification B である。stderr の捕捉はここに新設せず、同じ `package main` の既存 `captureStdoutStderr`（計画時点では `cmd/runner/startup_privilege_test.go:120-165`。パイプを goroutine で読み切る。Phase 4a で本ファイルへ移した。§1.5）を再利用する。`integration_logger_test.go:220-226` の `os.Stderr` 退避・復元はパイプを持たず出力を読めないため、手本にしない。
- **`internal/logging/test_helpers.go`（既存）**: `failed_file_paths` 付きレコードを RedactingHandler に通してビルダーへ渡す補助が 2 つ以上のテストで必要になれば、ここに置く。1 つのテストだけなら置かない。
- 新しいヘルパーファイルは作らない。

### 1.5 シンボルの移動・削除台帳

| 旧 | 新 | 種別 |
|---|---|---|
| `internal/logging/notification_contract_guard_test.go::parseSource` | `identitymutationguard.ParseSource` | 移動・公開 |
| `internal/logging/notification_contract_guard_test.go::isNamedType` | `identitymutationguard.IsNamedType` | 移動・公開 |
| `internal/logging/notification_contract_guard_test.go::elidedCompositeLiterals` | `identitymutationguard.ElidedCompositeLiterals` | 移動・公開 |
| `internal/logging/notification_contract_guard_test.go::compositeElementType` | `identitymutationguard` 内の非公開 `compositeElementType` | 移動（非公開のまま） |
| `internal/logging/notification_contract_guard_test.go::unwrapParen` | `identitymutationguard.UnwrapParen` | 移動・公開 |
| `runerrors.ErrorSeverity`（と定数 3 件） | （なし） | 削除 |
| `runerrors.ErrorType`（と定数 4 件） | （なし） | 削除 |
| `runerrors.ClassifiedError` | （なし） | 削除 |
| `runerrors.ClassifyVerificationError` | （なし） | 削除 |
| `runerrors.LogCriticalToStderr` | （なし） | 削除 |
| `runerrors.LogClassifiedError` | （なし） | 削除 |
| `internal/runner/runerrors/{types,classification,logging}.go`・`{classification,logging}_test.go` | `internal/runner/runerrors/pre_execution.go`・`pre_execution_test.go`・`pre_execution_guard_test.go`・`doc.go`（新設） | ファイルの削除と新設 |
| `cmd/runner/startup_privilege_test.go::captureStdoutStderr` | `cmd/runner/integration_test_helpers.go::captureStdoutStderr` | 移動（内容は変えない。Phase 4a。`//go:build test` のヘルパファイルから `_test.go` の関数は参照できないため） |

本書の他の箇所・AC 表・横断検索の記述は、すべてこの表に従う。

---

## 2. 実装ステップ

各 Phase の完了時、および各 PR のマージ前に `make fmt`・`make test`・`make lint` を通す（AC-09）。追加・変更した各テストは、検証対象を壊して失敗することを確認し、その旨をコミットメッセージに記す（AC-10、§4.4）。

### Phase 1: 補間契約の述語・属性キー・走査補助の移動

**対象ファイル**: `internal/common/interpolation.go`、`internal/common/interpolation_test.go`、`internal/common/logschema.go`、`internal/testutil/identitymutationguard/helpers.go`、`internal/logging/notification_contract_guard_test.go`

**作業内容**:

- [x] `notification_contract_guard_test.go` の `parseSource`・`isNamedType`・`elidedCompositeLiterals`・`compositeElementType`・`unwrapParen` を `identitymutationguard/helpers.go` へ移す（§1.5 台帳。`parseSource`・`isNamedType`・`elidedCompositeLiterals`・`unwrapParen` は公開名に、`compositeElementType` は非公開のまま。`unwrapParen` は slack_notify ガードの `isStaticallyFalse`・`notificationAttributeKey`・`checkNotificationAttributeUsage` も使うため、`internal/logging` から呼べる必要があり公開名 `UnwrapParen` にする）。`internal/logging` の呼び出しを新名に置き換え、`TestPreExecutionErrorLiteralCheckRecognizesForms` と `TestProductionPreExecutionErrorLiteralsCarryNotificationContext` が引き続き通ることを確認する。Phase 2b の 4 つの静的ガード（AC-18・AC-20・AC-21・AC-22）がこれらを使う。
- [x] `WithinInterpolationLimit(value string) bool` を `interpolation.go` に追加する。自由文の役割の変換（切り詰めなし）を適用した長さが `interpolationMaxBytes` 以下かを返す。02_architecture.md §3.1 の doc コメントを使う。`common.Interpolate` の戻り値は測定に使わない。
- [x] `interpolation_test.go` に `TestWithinInterpolationLimit` を追加する。02_architecture.md §7.1 の 5 行（ちょうど上限、上限 + 1 byte、実体参照化で超える ASCII、制御文字を含むが変換後は上限内、不正な UTF-8 を含むが置換後は上限内）を表駆動で固定する。上限は既存テストと同様に `interpolationMaxBytes` を参照して組む。
- [x] `logschema.go` の `PreExecErrorAttrs` に `FailedFilePaths string` を追加し、値を `"failed_file_paths"` にする。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestWithinInterpolationLimit` が、述語の判定を切り詰め後の長さに変えると失敗することを確認する。

### PR-1 作成ポイント: contract predicate, attribute key, and guard scan helpers

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0175): add the interpolation limit predicate and shared guard scan helpers`

**レビュー観点**: `WithinInterpolationLimit` が自由文の変換後の長さで判定し、`common.Interpolate` の戻り値を測定に使っていないこと／移動した走査補助 5 関数が `identitymutationguard` に揃い、`internal/logging` の既存ガード 2 件が移動後もアサーションと検出結果を変えずに通ること／`PreExecErrorAttrs.FailedFilePaths` のキー値が `"failed_file_paths"` であること／表駆動テストが上限ちょうど・上限 + 1 byte・実体参照化で膨らむ入力を覆うこと

**実装モデル要件**: standard

**判定理由**: 述語の変換規則と判定時点は 02_architecture.md §3.1 に固定済みで、未確定の実装アプローチや高リスク分岐は無い。Conditional checks は、build tag 下の非テストソース `identitymutationguard/helpers.go` を同じタグでコンパイルする項目 1 件にのみ該当する（`make test` は `-tags test` でコンパイルする）。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2a: `runerrors` の死んだシンボルの削除（単独コミット）

**対象ファイル**: `internal/runner/runerrors/*.go`、`README.ja.md`、`README.md`、`docs/dev/developer_guide/package_reference.md`

**作業内容**:

- [x] `internal/runner/runerrors/types.go` を削除する（`ErrorSeverity`・`ErrorType`・`ClassifiedError` と定数）。
- [x] `internal/runner/runerrors/classification.go` を削除する（`ClassifyVerificationError`）。
- [x] `internal/runner/runerrors/logging.go` を削除する（`LogCriticalToStderr`・`LogClassifiedError`）。
- [x] `internal/runner/runerrors/classification_test.go` を削除する。
- [x] `internal/runner/runerrors/logging_test.go` を削除する。
- [x] パッケージ doc を持つ `internal/runner/runerrors/doc.go` を置き、責務を「検証失敗を報告境界の `PreExecutionError` へ変換する」と英語で書く。`doc.go` は以後も残す（パッケージにファイルが 1 つも無い状態を作らず、パッケージ doc の置き場所を固定する）。
- [x] `README.ja.md:146` の `runerrors/` の説明を「検証失敗の報告変換」に更新し、コミットする。その後 `/mktrans` で `README.md:146` へ反映する。
- [x] `docs/dev/developer_guide/package_reference.md:42,109` の `runerrors/` の説明を英語で同じ内容に更新する（英語のみの文書のため直接編集する）。
- [x] 削除前後で `go test -tags test -coverprofile=c.out ./internal/runner/runerrors/ && go tool cover -func=c.out` を実行し、削除前は 3 関数（`ClassifyVerificationError`・`LogCriticalToStderr`・`LogClassifiedError`）、削除後は関数が 1 つも報告されず `total: 0.0%` だけになることを確認してコミットメッセージに記す（commit `066c9e59` で削除前を実行した結果: 関数 3 行 + `total` 1 行。テストファイルの無いパッケージでも `go test -coverprofile` はプロファイルを書き、`cover -func` は `total: 0.0%` を出して終了コード 0 で終わることを同時点で確認した）。

**完了条件**: `go build ./...` と `make test`・`make lint` が通る。`rg -n "runerrors" --glob '!docs/tasks/**'` の結果が README 2 件・`package_reference.md` 2 件・パッケージ自身だけであること（§8）。

### PR-2 作成ポイント: unused runerrors symbol removal

**対象ステップ**: Phase 2a

**推奨タイトル**: `refactor(0175): remove the unused runerrors symbols`

**レビュー観点**: 削除したシンボル（`ClassifiedError`・`ErrorSeverity`・`ErrorType`・`ClassifyVerificationError`・`LogClassifiedError`・`LogCriticalToStderr`）への参照が本番・テスト・`docs/tasks/` 以外の文書に残っていないこと／`doc.go` が残り、パッケージ doc が共有コンストラクタの責務を示すこと／README 日英と `package_reference.md` の説明が更新されていること／`go tool cover -func` の削除前後がコミットメッセージに記録されていること

**実装モデル要件**: standard

**判定理由**: 本番参照の無いシンボルの削除と 3 文書の追従で、設計判断を伴わず、Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2b: 収集失敗・非公開コンストラクタ・共有コンストラクタ・発火元

02_architecture.md §8 Phase 2b を 4 段に分ける。2b.1 → 2b.2 → 2b.3 → 2b.4 の順に依存する。

#### 2b.1 `verification`: 非公開コンストラクタと収集失敗

**対象ファイル**: `internal/verification/manager.go`、`internal/verification/errors.go`、`internal/verification/manager_test.go`、`internal/verification/error_construction_guard_test.go`（新設）

**作業内容**:

- [x] `errors.go:19-22` の隣に `ErrGroupVerificationCollectionFailed = errors.New("failed to collect verification files")` を追加する。
- [x] `manager.go` に非公開コンストラクタ `newVerificationError(op, group string, details []string, total, verified int, sentinel error) *Error` を追加する。`Details` には `details` を昇順に並べたコピーを設定し、`FailedFiles` は `len(details)` とする。`&Error{...}` はこの関数の中にだけ書く。
- [x] `manager.go:170`（グローバル検証失敗）・`:242`（group 検証失敗）の構造体リテラルを `newVerificationError` の呼び出しに置き換える。
- [x] `collectVerificationFiles` を、パス解決に失敗したコマンドを記録して残りの解決を続け、解決済みファイル集合と解決に失敗した対象（`command.ExpandedCmd`）の一覧を返す形に変える。既存の `slog.Warn`（`:279-283`）は対象ごとに残す。検証は 1 件も行わない（fail-closed を維持）。
- [x] `VerifyGroupFiles` の収集失敗分岐（`:196-203`）を、解決に失敗した対象の一覧・`TotalFiles` = 解決済みファイル集合の要素数 + 重複を除いた解決失敗対象の数（解決に失敗した対象は重複を除く）・`VerifiedFiles = 0`・`Err = ErrGroupVerificationCollectionFailed` で `newVerificationError` を呼ぶ形に変える（02_architecture.md §3.7）。
- [x] `manager_test.go` の `TestCollectVerificationFiles` の全呼び出し 7 件（`:679,699,713,730,765,787,808`）を新しい返り値の契約に合わせて更新する。解決済みのケースは解決に失敗した対象の一覧が空であることを確認し、解決失敗のサブテスト 2 件（`:773-810`）は返り値の一覧に解決に失敗した対象が入ることを見る形にする。
- [x] `manager_test.go` に `TestVerifyGroupFiles_CollectionFailureCarriesUnresolvedTargets` を追加する。解決に失敗するコマンド 1 件／複数件で、`Details` が解決失敗の全対象を昇順で持つこと、`TotalFiles`・`FailedFiles`・`VerifiedFiles` の件数、`errors.Is(err, ErrGroupVerificationCollectionFailed)`、`verErr.Err.Error()` に対象名が含まれないことを固定する（AC-17）。
- [x] `manager_test.go` に `TestVerificationErrorDetailsAreSorted` を追加する。グローバル検証失敗・group 検証失敗・group 収集失敗の 3 経路を表駆動にし、map の反復順に依存しない入力（例: `/b`, `/a`, `/c` を含む集合）で `Details` が昇順になることを固定する（AC-19・AC-21）。
- [x] `error_construction_guard_test.go` に `TestVerificationErrorLiteralsOnlyInConstructor` を追加する。`verification.Error` はフィールドが公開されており型では強制できないため（§1.2 原則 8）、ガードが構築形を列挙する。`identitymutationguard.ProductionGoFilesInRepo` で本番ファイル全件を走査し、`ResolveLocalImports` で `verification` の修飾子を解決して、(a) `verification.Error` を指す複合リテラル、すなわち修飾名 `<verification 修飾子>.Error{...}` と、`internal/verification` の本番ファイル内の非修飾 `Error{...}`（値形・ポインタ形・`ElidedCompositeLiterals` が返す elided 形）が `manager.go` の `newVerificationError` の中にだけ現れること（非修飾の `Error` は他パッケージにも同名の型があり、例: `internal/runner/base/privilege/unix.go:317` の `privilege.Error`。修飾名の解決無しに全本番ファイルを走査すると偽陽性になる）、(b) `internal/verification` の本番ファイルに限って、`Details` フィールドへのセレクタ代入（`x.Details = ...`）が `newVerificationError` の外に無いこと、(c) リテラルが 1 件も見つからなければ失敗すること、を固定する（AC-21）。(a) はリポジトリ全体を走査するが、(b) を全本番ファイルへ広げない理由は次のとおりである。`go/ast` だけの走査は型を持たず（`ResolveLocalImports` が与えるのは import の修飾子であり、変数の型ではない）、`.Details` という名前のセレクタ代入は無関係な型のフィールドにも一致する（`internal/filevalidator/validator.go:2063-2065` は別の型の `pltResult.Details` に代入している）。`internal/verification` の中では修飾子なしの `Error` は `verification.Error` にしかならないため、そこに限れば名前だけの一致で足りる。したがって (b) はパッケージ外の代入を見ない点で意図的に不完全であり、これは `TestNotificationContextBuiltOnlyByConstructors`（`internal/logging/notification_contract_guard_test.go`）が走査範囲を絞っているのと同じ割り切りである。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestVerificationErrorDetailsAreSorted` がコンストラクタの並べ替えを外すと 3 経路とも失敗すること、`TestVerificationErrorLiteralsOnlyInConstructor` が §4.4 の構築形ごとの変異（`VerifyGroupFiles` に `&Error{...}` を戻す、値形リテラルを置く、elided 形を置く、`internal/verification` の中（例: `VerifyGroupFiles`）で `newVerificationError` の外に `verErr.Details = ...` を代入する）のそれぞれで失敗することを確認する。

### PR-3 作成ポイント: verification single construction point and collection-failure list

**対象ステップ**: Phase 2b.1

**推奨タイトル**: `feat(0175): centralize verification error construction and list unresolved targets`

**レビュー観点**: `newVerificationError` が `Error` を生成する唯一の場所で、`Details` の昇順コピーと `FailedFiles` をそこでだけ設定すること／収集失敗が解決に失敗した全対象を `Details` に載せ、検証を 1 件も行わず fail-closed を維持すること／`collectVerificationFiles` の `slog.Warn` が同じ関数・同じ属性のままで、既存の `identifier_guard_test.go` の固定を壊さないこと／構築ガードが値・ポインタ・elided 形と `internal/verification` 内の `.Details` 代入を覆うこと

**実装モデル要件**: frontier-recommended

**判定理由**: Phase 2b.1 はファイル検証というセキュリティ境界の fail-closed 収集経路を作り替え、構築形を列挙する `go/ast` ガードを導入する孤立した高リスク・複雑ステップである。fail-closed の判定規則（未解決が 1 件でもあれば検証を実行せず拒否）は変えず、段階的な rollout や保護の raise/lower を伴わないため panel-mode トリガーには該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### 2b.2 `logging`: `FailedFilePaths` と `failed_file_paths` 属性

**対象ファイル**: `internal/logging/pre_execution_error.go`、`internal/logging/pre_execution_error_test.go`

**作業内容**:

- [x] `PreExecutionError` に `FailedFilePaths []string` を追加する（02_architecture.md §3.2 の doc コメントを英語で書く）。
- [x] `HandlePreExecutionError` が、`FailedFilePaths` が空でないときだけ `slog.Any(common.PreExecErrorAttrs.FailedFilePaths, preExecErr.FailedFilePaths)` をレコードに加えるようにする。`Detail()`・stderr・stdout の出力は変えない。
- [x] `HandleExecutionError`（`:171`）に、`Detail()` と同じ組み立てを重複実装している旨と [#1156](https://github.com/isseis/go-safe-cmd-runner/issues/1156) を指すコメントを英語で追加する。
- [x] `pre_execution_error_test.go` に `TestHandlePreExecutionError_FailedFilePaths` を追加する。既存の `captureErrorOutput`（`:428`）と RedactingHandler を通したレコーダで、(a) 一覧ありのとき `failed_file_paths` 属性がレコードにあり stderr・stdout にパスが現れないこと、(b) 一覧なしのとき属性が無くレコードが従来と同じ属性集合であることを固定する（AC-14）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。(a) が属性の追加を外すと失敗し、stderr の検査が `Detail()` にパスを連結すると失敗することを確認する。

#### 2b.3 `runerrors`: 共有コンストラクタ

**対象ファイル**: `internal/runner/runerrors/pre_execution.go`（新設）、`internal/runner/runerrors/pre_execution_test.go`（新設）、`internal/runner/runerrors/pre_execution_guard_test.go`（新設）

**作業内容**:

- [x] `pre_execution.go` に `NewVerificationPreExecutionError(verErr *verification.Error, errType logging.ErrorType, scope common.NotificationContext, runID string) *logging.PreExecutionError` を置く（02_architecture.md §3.2.1 のシグネチャと doc コメント）。`Message` は `errors.Is(verErr.Err, verification.ErrGroupVerificationCollectionFailed)` なら `Collection failed: %d of %d targets unresolved, Error: %v`（`FailedFiles`, `TotalFiles`, `Err`）、それ以外は `Total: %d, Verified: %d, Failed: %d, Error: %v`。`FailedFilePaths` は `slices.Clone(verErr.Details)`、`Component` は `string(resource.ComponentVerification)`、`Err` は nil。
- [x] `pre_execution_test.go` に `TestNewVerificationPreExecutionError` を追加する。02_architecture.md §7.9 の 5 行（group 検証失敗、グローバル検証失敗、収集失敗、`Details` が空、呼び出し後の `verErr.Details` 変更が返り値に影響しないこと）を表駆動で固定する（AC-04・AC-18）。
- [x] `pre_execution_guard_test.go` に `TestRunerrorsExportsOnlyTheSharedConstructor` を追加する。`identitymutationguard.ProductionGoFiles` で `internal/runner/runerrors` の本番ファイルを走査し、公開されたトップレベル宣言が `NewVerificationPreExecutionError` だけであることを固定する（AC-20）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestNewVerificationPreExecutionError` が、`Component` を `"runner"` に変える・`slices.Clone` を外す・収集失敗の分岐を外す、のそれぞれで失敗すること、`TestRunerrorsExportsOnlyTheSharedConstructor` が公開関数を 1 つ足すと失敗することを確認する。

### PR-4 作成ポイント: failed_file_paths attribute and shared constructor

**対象ステップ**: Phase 2b.2 / Phase 2b.3

**推奨タイトル**: `feat(0175): record failed_file_paths and add the shared verification constructor`

**レビュー観点**: `failed_file_paths` 属性が `FailedFilePaths` の空でないときだけ記録され、`Detail()`・stderr・stdout の出力が変わらないこと／共有コンストラクタが `Message` テンプレート 2 種・`Component` = `verification`・`slices.Clone`・`Err` = nil を満たす唯一の組み立て場所になること（発火元が実際にこれを呼ぶことの固定は PR-5 の配線ガード）／`TestRunerrorsExportsOnlyTheSharedConstructor` が `runerrors` の公開トップレベル宣言を `NewVerificationPreExecutionError` だけに固定すること／`TestHandlePreExecutionError_FailedFilePaths` が属性あり・なしの両方を覆うこと

**実装モデル要件**: standard

**判定理由**: フィールドと純関数の追加で、テンプレート・`Component`・複製規則は 02_architecture.md §3.2.1 に固定済み。未確定の実装アプローチや高リスク分岐は無く、Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### 2b.4 発火元の置き換えと `Component` の typed 定数化

**対象ファイル**: `internal/runner/runner.go`、`cmd/runner/main.go`、`internal/runner/runner_test.go`、`internal/logging/notification_contract_guard_test.go`、`internal/runner/runerrors/pre_execution_guard_test.go`

**作業内容**:

- [x] `runner.go:426-439` の検証分岐を、`runerrors.NewVerificationPreExecutionError(verErr, logging.ErrorTypeGroupFileVerification, common.GroupScope(verErr.Group), r.runID)` の結果を `logging.HandlePreExecutionError` に渡す形へ置き換える。`errorMsg` の組み立てと `PreExecutionError` リテラルを削除する。
- [x] `main.go:391-402` を、`errors.AsType[*verification.Error](err)` が成立するなら `runerrors.NewVerificationPreExecutionError(verErr, logging.ErrorTypeFileAccess, common.GlobalScope(), runID)` を返し、それ以外は従来どおり `err.Error()` を `Message` に渡す形へ変える（02_architecture.md §3.2.3）。
- [x] `main.go:136,186,199,243` の `Component: "main"` を `string(resource.ComponentMain)` に、`main.go:692` の `Component: "runner"` を `string(resource.ComponentRunner)` に置き換える（`runner.go:435` は上の置き換えで消える）。
- [x] `notification_contract_guard_test.go` に `TestProductionErrorLiteralsUseTypedComponent` を追加する。既存の走査（`checkPreExecutionErrorLiterals` と同じ手法）で本番の `PreExecutionError`・`ExecutionError` 複合リテラルの `Component` キー値が、ちょうど `string(...)` 変換であり、その被演算子がセレクタ `<resource 修飾子>.Component<Name>` で、修飾子が `identitymutationguard.ResolveLocalImports` により `github.com/isseis/go-safe-cmd-runner/internal/runner/resource` に解決されることを固定する（commit `066c9e59` 時点で生リテラルでない本番の `Component:` 値はすべてこの形である。`rg -n 'Component:' cmd/runner/main.go internal/runner/runner.go`）。それ以外の式形（識別子、`fmt.Sprint(...)`、`string("main")`、他パッケージのセレクタ）はすべて失敗させる。あわせて、`internal/logging` 以外の本番ファイル（`logging` を import するファイルを走査範囲とする）で `PreExecutionError`・`ExecutionError` の値の `.Component` へのセレクタ代入（`pe.Component = ...`）が無いことも検査する。走査範囲を import で絞るこの検査は `TestNotificationContextBuiltOnlyByConstructors` と同じく意図的に不完全である（型情報を持たないため、`logging` を import しないファイルでの代入は見ない）。走査が 1 件もリテラルを見つけないときは失敗させる（AC-22）。
- [x] `pre_execution_guard_test.go` に `TestFiringPointsUseSharedVerificationConstructor` を追加する。`PreExecutionError` はフィールドが公開されており型では強制できないため（§1.2 原則 8）、ガードが構築形を列挙する。`identitymutationguard.ProductionGoFilesInRepo` で本番ファイル全件を走査し、`ResolveLocalImports` で `runerrors`・`logging` の修飾子を解決して、(a) `internal/runner/runner.go` と `cmd/runner/main.go` に `NewVerificationPreExecutionError` の呼び出しが 1 件ずつあり他の本番ファイルには無いこと、(b) `internal/runner/runerrors` 以外の本番ファイルで、`PreExecutionError` 複合リテラル（値・ポインタ・elided 形）が `FailedFilePaths` キーを設定しないこと、(c) `internal/runner/runerrors` 以外の本番ファイルに `.FailedFilePaths` へのセレクタ代入が無いこと、(d) 呼び出しが 1 件も見つからなければ失敗すること、を固定する（AC-18。02_architecture.md §7.9）。
- [x] `runner_test.go` に `TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent` を追加する。既存の `TestRunner_VerificationErrorCarriesGroupScopeAndCleanMessage`（`:2385`）と同じ `MockGroupExecutor` 構成で、レコーダを `redaction.NewRedactingHandler` で包んで `slog.SetDefault` する。(a) `Details` 付きの検証失敗で `failed_file_paths` が `Details` と同じ要素を持ち、`component` が `verification`、`error_message` にパスが含まれないこと、(b) `ErrGroupVerificationCollectionFailed` を `Err` に持つ収集失敗で本文が `Collection failed:` で始まり `failed_file_paths` が対象名を持つこと、を固定する（AC-06 の属性側・AC-17・AC-18）。RedactingHandler を通した後の値は `[]any` になるため、要素を文字列として比較する。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。`TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent` が共有コンストラクタ呼び出しを旧リテラルへ戻すと失敗すること、`TestFiringPointsUseSharedVerificationConstructor` が §4.4 の構築形ごとの変異（`FailedFilePaths` 付きの手組みリテラル、リテラルの後の `pe.FailedFilePaths = ...` 代入）のそれぞれで失敗すること、`TestProductionErrorLiteralsUseTypedComponent` が §4.4 の式形ごとの変異（`Component` を生リテラルに戻す、`string("main")` にする、識別子変数にする、リテラルの後に `pe.Component = ...` を代入する）のそれぞれで失敗することを確認する。`make deadcode` の出力に `internal/runner/runerrors` の行が無いことを確認する。

### Phase 3: 通知ビルダーの描画

**対象ファイル**: `internal/logging/slack_handler.go`、`internal/logging/slack_handler_test.go`

**作業内容**:

- [x] `slack_handler.go` に、`failed_file_paths` 属性の `slog.Value` を `[]string` へデコードする非公開補助を追加する。`[]string` と `[]any`（要素は文字列）を読み、どちらでもない表現・文字列でない要素を含む表現は「一覧なし」として扱う。
- [x] `buildPreExecutionError`（`:836-866`）を、`error_message` の値と一覧から `Error Message` の値を組み立てる形へ変える。本文の骨格・表示形（`strconv.Quote`）・掲載の選択・切り詰めは 02_architecture.md §3.3 の表と選択規則に従う。コードが推論できない制約は 2 つ: 上限値・変換規則をビルダーに書かず `common.WithinInterpolationLimit` だけに問い合わせること、最後の `common.Interpolate(..., InterpolationRoleFreeText)` は既存のまま 1 回だけ通すこと。一覧が空（属性はあるが要素 0 件）のときは `Files:` 節を付けない。
- [x] `slack_handler_test.go` に `TestBuildPreExecutionError_FailedFilePaths` を追加する。レコードは必ず `redaction.NewRedactingHandler` で包んだハンドラを通す（`TestSlackHandler_WithRedactingHandler`（`:1002`）と同じ手法）。02_architecture.md §7.2 の 12 行（一覧なし、区切り文字衝突の 2 集合、`\n` と空白、`"`・`\`・制御文字・不正 UTF-8、丸ごと収まらない長いパスの閉じ引用符と `…`、全件表示、部分表示の k と m、先頭が長く後続が短い一覧、丸ごと 0 件の切り詰めと m = n - 1、実体参照化で膨らむ要素、渡した順の描画、raw の候補が `WithinInterpolationLimit` を満たすこと）を固定する（AC-01・AC-02・AC-03・AC-08）。部分表示の行は、掲載パスが `failed_file_paths` の要素そのものであること（切り詰めでないこと）と `(+m more)` の m が `n - 掲載件数` に等しいことを見る。
- [x] 同ファイルに `TestBuildPreExecutionError_FailedFilePathsMalformedValue` を追加する。`failed_file_paths` に文字列でない要素を含むスライス・非スライス値・要素 0 件のスライス（`[]any{}`）を置いたレコードで、いずれも `Error Message` が `error_message` の値と等しく `Files:` を含まないことを固定する（AC-04 の一覧なし規則の producer 欠陥側）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。表の各行について、対応する分岐（省略通知の組み立て・丸ごと優先の走査・k = 0 の切り詰め・要素の型検査）を外すと当該行が失敗することを確認する（§4.4）。

### PR-5 作成ポイント: shared constructor wiring and failed-file rendering

**対象ステップ**: Phase 2b.4 / Phase 3

**推奨タイトル**: `feat(0175): route verification failures through the shared constructor and render failed files`

**レビュー観点**: group・グローバルの両発火元が共有コンストラクタを呼び、本文・一覧・`Component` を手組みしないこと（`go/ast` ガード 2 件）／`Component` の値が `string(resource.ComponentXxx)` 形に限られ、`"main"`・`"runner"` の生リテラルが残らないこと／記録した `failed_file_paths` が同じ PR のビルダーで `Files:` 節として描画され、`Message` がパスを含まないまま通知からパスが消えないこと（上限判定・切り詰め探索は `common.WithinInterpolationLimit` への問い合わせだけに頼り、丸ごと優先の走査・k = 0 の切り詰め・`(+m more)` の m = n − k が 02_architecture.md §3.3 の表の各行どおり）／テストが必ず `redaction.NewRedactingHandler` を通した `[]any` のレコードを使うこと

**実装モデル要件**: frontier-recommended

**判定理由**: Phase 2b.4 は両発火元の `Component`・`Message` という可視挙動を変えながら構築形を列挙する `go/ast` ガード 2 件を導入し、Phase 3 は予算内掲載の選択・切り詰め探索・省略件数計算を述語への問い合わせだけで行う。いずれも孤立した高リスク・複雑ステップだが、`failed_file_paths` の記録と描画を同一 PR に入れて段階的な raise/lower を作らないため panel-mode トリガーには該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 横断的なテスト・redaction 回帰・ベンチマーク・グローバル回帰

02_architecture.md §8 Phase 4 を 2 段に分ける。4a → 4b の順に依存する。

#### 4a 統合テスト共通ヘルパと group 統合テスト

**対象ファイル**: `cmd/runner/integration_test_helpers.go`、`cmd/runner/integration_pre_execution_error_test.go`

**作業内容**:

- [x] `cmd/runner/integration_test_helpers.go` に、`TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope`（`:481-586`）の共通手順（Slack モックサーバー、設定ファイルのハッシュ登録、`bootstrap.SetSlackHandlerFactory`、フラグ変数の退避、`mainWithExitCode` の実行と `FlushSlackNotifications`）を関数に抽出し、既存テストをそれで書き直す。ヘルパは `dryRun = false` を必ず設定し（§1.3）、これを使うテストは `t.Parallel` を呼ばない。
- [x] stderr の捕捉には、同じ `package main` にある既存の `captureStdoutStderr`（`cmd/runner/startup_privilege_test.go:120-165`）を再利用し、`mainWithExitCode` の実行をその `fn` の中で行う。同関数は `os.Stdout`・`os.Stderr` をパイプへ差し替え、両パイプを goroutine で並行して読み切り、書き込み側を閉じてから待つため、長い一覧を出す統合テスト（`TruncatesLongList` の 2 件）でもパイプの固定バッファが満杯になって `mainWithExitCode` が止まることがない。`captureStdoutStderr` は `_test.go` に置かれていたが、`//go:build test` の `integration_test_helpers.go` から参照すると `-tags test` でのバイナリビルド（dry-run 統合テストが行う）が失敗するため、同関数をそのまま `integration_test_helpers.go` へ移した（内容は変えない）。新しい捕捉ヘルパは作らない（`integration_logger_test.go:220-226` の手法は `os.Stderr` の退避と復元だけでパイプを持たず、出力を読めない）。非対話実行の stderr には `handleErrorCommon` の `  Details:` 行のほかに、構造化ログ行（`failed_file_paths=[...]` を含む）と検証マネージャのファイル単位の `slog.Error` 行も流れる（02_architecture.md §5.2 の残存リスク）。したがって以下の統合テストの「stderr にパスが無い」アサーションは `  Details:` 行だけを対象にし、stderr 全体には広げない。
- [x] `integration_pre_execution_error_test.go` に `TestIntegration_GroupFileVerificationFailureListsFailedFiles` を追加する。group の `verify_files` にハッシュ未登録のファイル 2 件以上（昇順でない名前で作る）を置き、`Error Message` が `Total: N, Verified: N, Failed: N, Error: group file verification failed, Files: ...` の形で各パスを表示形で含むこと、`Component` フィールドが `verification`、Text 行の Scope が `group=<name>`、`Error Message` に `Group: <name>` が無いこと、`error_type` が `group_file_verification_failed`、stderr の `Details:` 行にパスが無いことを固定する（AC-01・AC-03・AC-06・AC-07・AC-14。AC-05 はグローバル経路の基準であり §7 のとおり Phase 4b で検証する）。group のコマンド（`/bin/true` など）もハッシュ対象に入るため、ハッシュを登録するか失敗一覧に含めるかを決めて期待値を組む。
- [x] 同ファイルに `TestIntegration_GroupFileVerificationFailureTruncatesLongList` を追加する。上限を超える件数のハッシュ未登録ファイルで、`Error Message` に `(+m more)` が現れ m が `n - 掲載件数` に等しいことを固定する（AC-02・AC-06）。
- [x] 同ファイルに `TestIntegration_GroupCollectionFailureListsUnresolvedTargets` を追加する。存在しない絶対パスのコマンドを 2 件以上持つ group で、`Error Message` が `Collection failed: 2 of N targets unresolved, Error: failed to collect verification files, Files: ...` の形で対象名を含み `Total:`／`Verified:` を含まないこと、stderr の `Details:` 行に対象名が無いこと、`error_type` が `group_file_verification_failed` のままであることを固定する（AC-17）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。group の統合テスト 3 件が対応する分岐（`Files:` 節の付与・省略通知・収集失敗テンプレート）を外すと失敗することを確認する（§4.4）。

### PR-6 作成ポイント: group integration coverage and shared helper

**対象ステップ**: Phase 4a

**推奨タイトル**: `test(0175): extract the Slack integration helper and cover group verification failures`

**レビュー観点**: 統合テスト共通ヘルパが既存テストと同じ手順（Slack モックサーバー、ハッシュ登録、ファクトリ差し替え、フラグ退避、`mainWithExitCode` と `FlushSlackNotifications`）を保ち、既存テストのアサーションを弱めないこと／プロセス全体の状態を差し替えるため `t.Parallel` を使わず、ヘルパが `dryRun = false` を必ず設定すること／group の統合テスト 3 件（全件・部分・収集失敗）が最終 `Error Message`・`Component` = `verification`・Scope・`error_type`・stderr の `  Details:` 行を観測すること／stderr のパス不在の検査対象が `  Details:` 行に限定されていること

**実装モデル要件**: frontier-required

**判定理由**: Slack モックサーバー・プロセス全体状態の差し替え・stderr パイプ捕捉を伴う統合テスト 3 件と、既存統合テストからの共通ヘルパ抽出を含む heavy integration-test surface で、mkplan.md step 8 の panel-mode トリガーに該当するため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### 4b グローバル回帰・redaction 回帰・ベンチマーク

**対象ファイル**: `internal/redaction/redactor_test.go`、`internal/logging/slack_handler_benchmark_test.go`、`cmd/runner/integration_pre_execution_error_test.go`

**作業内容**:

- [x] `TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope` にアサーションを追加する。`Error Message` が `Total: 1, Verified: 0, Failed: 1, Error: global file verification failed, Files: ...` の形で失敗ファイルのパスを含むこと、`Component` フィールドが `verification`、stderr の `Details:` 行にパスが無いこと、`error_type` と Scope が従来どおりであること（AC-05・AC-14・AC-16）。
- [x] 同ファイルに `TestIntegration_GlobalTargetFileVerificationFailureTruncatesLongList` を追加する。上限を超える件数のグローバル `verify_files` で `(+m more)` が現れることを固定する（AC-05・AC-16）。
- [x] `redactor_test.go` の `TestRedactingHandler_SliceStringElementRedaction`（`:2884`）にサブテスト `KeywordBearingPathElementIsKept` を追加する。`[]string{"/opt/monkey/data", "ghp_" + 36 文字のトークン形式}` のような一覧で、`key` を含むパス要素はそのまま残り、値形式の機密要素は `[REDACTED]` になること、および対照として同じ `/opt/monkey/data` を `slog.String` 属性で渡すと値全体が `[REDACTED]` になることを固定する（AC-15。層の切り分け: 値形式検出だけが動く入力と、値全体置換だけが動く入力を分ける）。
- [x] `slack_handler_benchmark_test.go` に `BenchmarkBuildPreExecutionError_FailedFilePaths` を追加する。RedactingHandler とビルダーを通す end-to-end で、n = 1,000・n = 10,000・4 KiB のパス数件、の 3 サブベンチマークを持つ。実装時に 02_architecture.md §7.7 の基準（n = 10,000 が 100 ms 未満、1 件あたりのコストが n = 1,000 の 3 倍以内）を確認し、数値をコミットメッセージに記す。合否判定はテストに入れない。
  - 実測結果（2026-09-24、linux/arm64）: end-to-end は n = 1,000 で約 37 ms、n = 10,000 で約 375 ms（1 件あたり約 37 µs で線形）で、絶対予算 100 ms を満たさなかった。プロファイルでは約 85% が既存の `processSlice` による要素ごとの `RedactText` だった。redaction は本タスクで変更しない（02_architecture.md §5.4）。ビルダーだけのスケーリングを end-to-end の比では検出できない（線形の redaction が大半を占めるため、ビルダーが O(n²) になっても比は 3 倍に届かない）ので、ビルダー単体の `BenchmarkRenderFailedFiles`（n = 1,000・n = 10,000）を同ファイルに追加した。ビルダー単体は n = 1,000 で約 4.3 ms、n = 10,000 で約 45 ms（1 件あたりの比は約 1.04 倍）で、予算とスケーリングの基準を満たす。
  - 既存経路との比較: グローバル経路は `manager.go` の `"failed_files"` ログ（`[]string`）で同じ要素ごとの redaction をすでに 1 回払っている。ファイル単位の `slog.Error` も失敗ファイルごとに `file` と `error`（パスを含む）の 2 属性を redaction する。一覧の redaction は、検証エラーの報告 1 件ごと（group では失敗した group ごと）に、これと同程度のコストを加える。実行全体の wall time は測定していない。02_architecture.md §7.7 の前提（ハッシュ計算と I/O が 1 回の実行でミリ秒台）は n = 10,000 では当てはまらない。CLAUDE.md「Performance」に従い最適化の仕組みは足さない。PR-7 のレビューでレビュアーが結果を受け入れ、02_architecture.md §7.7 の予算の対象をビルダー単体に変更した（end-to-end は記録のみ）。

**完了条件**: `make fmt`・`make test`・`make lint` が通る。redaction 回帰のサブテストが `processSlice` の要素に値全体置換を足すと失敗すること、グローバルの統合テスト 2 件が `Files:` 節・省略通知・`Component`・`Detail()` へのパス連結を外すと失敗することを確認する（§4.4）。

### PR-7 作成ポイント: global regression, redaction regression, and benchmark

**対象ステップ**: Phase 4b

**推奨タイトル**: `test(0175): cover global verification rendering, redaction regression, and benchmark cost`

**レビュー観点**: グローバルの統合テスト 2 件（既存の拡張・上限超過）が Phase 4a の共通ヘルパを通して `Files:` 節・省略件数・`Component` = `verification`・stderr の `  Details:` 行を観測すること／redaction 回帰が値形式検出だけが動く入力と値全体置換だけが動く入力の対照で層を切り分けていること／ベンチマークの絶対予算と実測値がコミットメッセージに記録されていること

**実装モデル要件**: frontier-required

**判定理由**: Phase 4b もプロセス全体状態を差し替えるグローバル統合テスト 2 件と end-to-end ベンチマークを含む integration-test surface で、同じ panel-mode トリガーに該当するため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 文書の更新

**対象ファイル**: `docs/user/runner_command.ja.md`、`docs/user/runner_command.md`、`docs/tasks/0172_slack_notification_message_unification/02_architecture.md`、`docs/tasks/0172_slack_notification_message_unification/03_implementation_plan.md`

**作業内容**:

- [x] `runner_command.ja.md` の「通知されるメッセージ種別」（`:1476` の `pre_execution_error` 行の周辺）またはその直後に、検証エラー通知の `Error Message` が失敗ファイル一覧を `Files:` 節として表示すること、上限を超えるときは `(+m more)` で省略件数を示すこと、収集失敗では解決に失敗したコマンドを一覧に載せることを追記する。「ファイル検証エラー」（`:1898`）から相互参照する。記述は Phase 4a・4b の統合テストが観測した実際の `Error Message` を典拠にする。
- [x] `runner_command.ja.md` をコミットした後、`/mktrans` で `runner_command.md` へ反映する。
- [x] 0172 アーキテクチャ設計書 §3.5「動的な値の一覧」（`02_architecture.md:560-580`）に `failed_file_paths` の要素（`pre_execution_error` の `Error Message` フィールドの材料、役割は自由文）の行を追加する。editorial correction として扱い、`Status` は `approved` のまま変えず、`Comments` に本タスク（0175）からの追加である旨と決定を変えていない旨を書く（02_architecture.md §5.2）。
- [x] 0172 実装計画書 §10（`03_implementation_plan.md:1746-1753`）の group 検証エラーの follow-up に、0174 と同じ形式で「解消済み（2026-MM-DD 追記）」と本タスクへのリンクを追記する。
- [x] `make verify-docs-checks` と `make test`（docsguard を含む）が通ることを確認する。

**完了条件**: 3 文書の変更が `make test`・`make verify-docs-checks` を通る。

### PR-8 作成ポイント: user documentation and 0172 follow-up

**対象ステップ**: Phase 5

**推奨タイトル**: `docs(0175): document the failed file listing in verification notifications`

**レビュー観点**: `runner_command.ja.md` の記述が Phase 4a・4b の統合テストが観測した実際の `Error Message` と一致すること／`/mktrans` による英語版への反映後も見出し構造が一致すること／0172 アーキテクチャ設計書への 1 行追記が editorial correction として `Comments` に記録され、`Status` を変えていないこと／0172 実装計画書 §10 の follow-up に解消が記録されていること

**実装モデル要件**: standard

**判定理由**: 文書の追記と翻訳が中心で、記述の典拠は Phase 4a・4b の統合テスト出力に固定されており、Conditional checks・panel-mode トリガーのいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 契約の述語と走査補助 | Phase 1 | `TestWithinInterpolationLimit` が green。既存の 2 つのリテラルガードが移動後も green |
| M2: パッケージ整理 | Phase 2a | `runerrors` に本番シンボルが無く、README・`package_reference.md` が更新済み |
| M3: 一覧の生成と伝搬 | Phase 2b | 3 経路の昇順・収集失敗・共有コンストラクタ・両発火元の置き換えと静的ガード 4 件が green。`make deadcode` に `runerrors` の行が無い |
| M4: 描画 | Phase 3 | ビルダーの表駆動テストが green |
| M5: 横断検証 | Phase 4a / Phase 4b | 統合テスト 5 件（新規 4 件 + 既存 1 件の拡張）と redaction 回帰が green。ベンチマークの数値を記録 |
| M6: 文書 | Phase 5 | 利用者向け文書と 0172 の 2 文書が更新済み |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `WithinInterpolationLimit`、`failed_file_paths` キー、走査補助 5 関数の `identitymutationguard` への移動 | standard |
| PR-2 | Phase 2a | `runerrors` の死んだシンボルとテストの削除、パッケージ説明の更新 | standard |
| PR-3 | Phase 2b.1 | 非公開コンストラクタと昇順正規化、収集失敗の一覧化、構築ガード | frontier-recommended |
| PR-4 | Phase 2b.2 / Phase 2b.3 | `failed_file_paths` 属性の記録、共有コンストラクタ、公開シンボルガード | standard |
| PR-5 | Phase 2b.4 / Phase 3 | 両発火元の置き換え、`Component` の typed 定数化、配線ガード 2 件、`buildPreExecutionError` の一覧描画 | frontier-recommended |
| PR-6 | Phase 4a | 統合テスト共通ヘルパの抽出、group 統合テスト 3 件（新規） | frontier-required |
| PR-7 | Phase 4b | グローバル統合テスト 2 件（既存 1 件の拡張 + 新規 1 件）、redaction 回帰、ベンチマーク | frontier-required |
| PR-8 | Phase 5 | 日英の利用者向け文書、0172 の 2 文書への追記 | standard |

PR-3 → PR-4 → PR-5 の順序は Phase 2b.1 → 2b.2 → 2b.3 → 2b.4 の依存（収集失敗のセンチネル、`FailedFilePaths`、共有コンストラクタ）による。PR-5 は Phase 2b.4 と Phase 3 をまとめ、`failed_file_paths` の記録と描画を同じ PR で変更する（記録だけが先行して通知からパスが消える中間状態を作らない）。PR-6 は PR-5 までの実装を前提とし、PR-7 は PR-6 の共通ヘルパを再利用するため PR-6 → PR-7 の順に依存する。PR-8 は PR-7 が観測した表示を記述の典拠にする。

### 3.3 順序の根拠

02_architecture.md §8 の 1 → 2a → 2b → 3 → 4 → 5 を保つ。Phase 1 の述語が無ければ Phase 3 のビルダーは上限を測れず、Phase 2b の属性が無ければ Phase 3 のビルダーは読む値を持たない。Phase 2a は共有コンストラクタの追加と別コミットにする要件（01 §決定事項）のため 2b の前に置く。§8 Phase 4 が挙げるテストのうち、コンポーネント単位のもの（`manager_test.go`・`pre_execution_test.go`・`pre_execution_guard_test.go`・`pre_execution_error_test.go`・`slack_handler_test.go`）は対応する実装と同じ Phase へ移し、実装と検証を同じコミットのレビュー対象にする（静的ガードは、それが固定する置き換えと同じコミットに入ることで、置き換え直後から手組みへの後退を検出する）。静的ガードが依存する走査補助の移動は Phase 1 に置く。Phase 4（4a・4b）には複数コンポーネントを跨ぐもの（統合テスト・redaction 回帰・ベンチマーク）を残す。この配置は 02_architecture.md §8 にも反映済みである（§1.2）。Phase 4a・4b の統合テストは Phase 2b と Phase 3 の両方が無いと最終 `Error Message` を観測できないため、この順でなければならない。Phase 5 は統合テストが観測した実際の表示を文書の典拠にするため最後に置く。

---

## 4. テスト戦略

### 4.1 単体テスト（特権不要・`make test` で常に実行）

- **契約**: `TestWithinInterpolationLimit`（Phase 1）。
- **生成と正規化**: `TestVerificationErrorDetailsAreSorted`・`TestVerifyGroupFiles_CollectionFailureCarriesUnresolvedTargets`・`TestCollectVerificationFiles`（更新）（Phase 2b.1）。
- **属性の記録**: `TestHandlePreExecutionError_FailedFilePaths`（Phase 2b.2）。
- **共有コンストラクタ**: `TestNewVerificationPreExecutionError`（Phase 2b.3）。
- **発火元の配線**: `TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent`（Phase 2b.4）。
- **描画**: `TestBuildPreExecutionError_FailedFilePaths`・`TestBuildPreExecutionError_FailedFilePathsMalformedValue`（Phase 3）。

### 4.2 層の切り分け

- `failed_file_paths` を読むテストは必ず `redaction.NewRedactingHandler` を通したレコードを使う。`processSlice` が `[]string` を `[]any` に変えるため（`redactor.go:1463`）、生のレコードを直接組むテストでは本番の表現を取りこぼす（02_architecture.md §7.2）。
- redaction 回帰（Phase 4b）は、値形式検出だけが動く要素（トークン形式）と、値全体置換だけが動く文字列（`/opt/monkey/data` を `KindString` で渡す）を分け、スライス要素の `/opt/monkey/data` が残ることを両者との対照で示す。
- グローバル発火元が渡す `error_message` にパスが無いことは、`internal/logging` の汎用テストでは検出できない（`main.go` がパス入りの `Message` を渡し続けても通る）。`cmd/runner` の統合テストの stderr 捕捉で固定する（§7.4）。

### 4.3 統合テスト（`cmd/runner`、`make test` に含まれる）

in-process ハンドラ差し替えで Slack モックサーバーへ流し、最終 `Error Message`・`Component`・Scope・stderr を観測する。group 検証失敗（全件・部分）、group 収集失敗、グローバル検証失敗（既存の拡張・部分）の 5 件（Phase 4a / Phase 4b）。

### 4.4 実装時に行うテスト失敗確認（AC-10）

各 Phase の「完了条件」に列挙した変異を実施し、当該テストが失敗することを確認して、コミットメッセージに記す。特に次を落とさない。

| Phase | 変異 | 失敗するテスト |
|---|---|---|
| 1 | 述語を切り詰め後の長さで判定する | `TestWithinInterpolationLimit`（上限 + 1 byte の行） |
| 1 | 移動した `ElidedCompositeLiterals` が elided 形を返さないようにする | 既存の `TestPreExecutionErrorLiteralCheckRecognizesForms` |
| 2b.1 | `newVerificationError` の並べ替えを外す | `TestVerificationErrorDetailsAreSorted`（3 経路すべて） |
| 2b.1 | `collectVerificationFiles` を 1 件目で打ち切る形に戻す | `TestVerifyGroupFiles_CollectionFailureCarriesUnresolvedTargets`（複数件の行） |
| 2b.1 | `VerifyGroupFiles` に `&Error{...}` を戻す／値形 `Error{...}` を置く／elided 形（`[]*Error{{...}}`）を `newVerificationError` の外に置く／`internal/verification` の中（例: `VerifyGroupFiles`）で `newVerificationError` の外に `verErr.Details = ...` を代入する | `TestVerificationErrorLiteralsOnlyInConstructor`（構築形ごとに 1 回ずつ、計 4 回） |
| 2b.2 | 属性の追加を外す／`Detail()` にパスを連結する | `TestHandlePreExecutionError_FailedFilePaths` |
| 2b.3 | `slices.Clone` を外す／`Component` を `runner` にする／収集失敗の分岐を外す | `TestNewVerificationPreExecutionError` |
| 2b.3 | 公開関数を 1 つ足す | `TestRunerrorsExportsOnlyTheSharedConstructor` |
| 2b.4 | `Component` を生リテラル `"main"` に戻す／`string("main")` にする／識別子変数（`component := "main"` の `component`）にする／リテラルの後に `pe.Component = string(resource.ComponentMain)` を代入する | `TestProductionErrorLiteralsUseTypedComponent`（式形ごとに 1 回ずつ、計 4 回） |
| 2b.4 | `runner.go` を `FailedFilePaths` 付きの手組みリテラルに戻す／リテラルの後に `pe.FailedFilePaths = ...` を代入する | `TestFiringPointsUseSharedVerificationConstructor`（構築形ごとに 1 回ずつ） |
| 2b.4 | 共有コンストラクタ呼び出しを旧リテラルへ戻す | `TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent` |
| 3 | 省略通知を付けない／丸ごと優先を切り詰めに変える／k = 0 の切り詰めを外す | `TestBuildPreExecutionError_FailedFilePaths`（部分表示・切り詰めの行） |
| 3 | 要素の型検査を外す／空スライスで `Files:` 節を付ける | `TestBuildPreExecutionError_FailedFilePathsMalformedValue` |
| 4b | `processSlice` の文字列要素に `IsSensitiveValue` の値全体置換を足す | `TestRedactingHandler_SliceStringElementRedaction`（`KeywordBearingPathElementIsKept`） |
| 4a・4b | ビルダーが `Files:` 節を付けない／`main.go` が `err.Error()` を `Message` に戻す | 統合テスト 5 件（後者はグローバルの 2 件の `Details:` 行アサーション） |

### 4.5 性能

`BenchmarkBuildPreExecutionError_FailedFilePaths`（end-to-end）と `BenchmarkRenderFailedFiles`（ビルダー単体）で 02_architecture.md §7.7 の絶対予算とスケーリングを確認し、数値をコミットメッセージに記す（Phase 4b）。単体テストのしきい値にはしない。予算はビルダー単体に課し、end-to-end は記録のみとする（02_architecture.md §7.7。Phase 4b の実測結果を参照）。

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| `collectVerificationFiles` の返り値変更が `VerifyGroupFiles` 以外の呼び出し元に波及する | ビルド失敗または挙動変化 | 呼び出し元は `VerifyGroupFiles` とテストのみ（§1.3）。コンパイルで検出する |
| グローバル発火元の `Message` から `err.Error()` のパスが消え、`error_message` を照合する外部消費者が影響を受ける | 運用スクリプトの不一致 | 02_architecture.md §3.6・§8 Phase 5 の記載どおり利用者向け文書に明記する（Phase 5）。通知本文から失敗ファイルは消えず、`Files:` 節へ移る |
| ビルダーの切り詰め探索が上限値を暗黙に前提にする | `interpolationMaxBytes` を変えたときにビルダーが壊れる | 上限値をビルダーに書かず、`WithinInterpolationLimit` だけに問い合わせる。`TestBuildPreExecutionError_FailedFilePaths` の最終行（raw の候補が述語を満たす）で固定する |
| `cmd/runner` 統合テストで group のコマンド（`/bin/true`）が失敗ファイル一覧に混ざる | 期待値がプラットフォーム依存になる | コマンドのハッシュを登録するか、コマンドを含む形で期待値を組むかを Phase 4a で決めて固定する |
| 完了済みタスク 0172 の承認済み文書へ追記する | プロセス上の懸念 | 決定を変えない editorial correction として `Comments` に記録し、ステータスは変えない（02_architecture.md §5.2） |
| `identitymutationguard` への走査補助の移動で `internal/logging` の既存ガードを壊す | 既存ガードの vacuous pass | `TestPreExecutionErrorLiteralCheckRecognizesForms` が移動後も通ることを Phase 1 の作業に含める |
| `collectVerificationFiles` の書き換えが `internal/identifier/identifier_guard_test.go:122` の固定（同関数内の `slog.Warn` 1 件）を壊す | 既存ガードの失敗 | `slog.Warn` は同じ関数・同じ属性のまま残し、補助関数へ切り出さない（§1.3） |
| `cmd/runner` 統合テストがプロセス全体の状態を差し替える | 並列実行時の干渉、dry-run で失敗が発火しない | `t.Parallel` を使わず、ヘルパが `dryRun = false` を必ず設定する（§1.3） |
| 収集失敗で残りのコマンドの解決を続けることによる `slog.Warn` の増加 | ログ行数の増加 | 対象ごとに 1 行で、件数は group のコマンド数が上限。許容する |
| `.Details` 代入の検査を全本番ファイルへ広げると `internal/filevalidator/validator.go:2063-2065` の `pltResult.Details` 代入に誤反応する | ガードの偽陽性 | 検査 (b) を `internal/verification` の本番ファイルに限定する（Phase 2b.1） |
| 非修飾 `Error{...}` の検査を全本番ファイルへ広げると `internal/runner/base/privilege/unix.go:317` の `privilege.Error` リテラルに誤反応する | ガードの偽陽性 | 検査 (a) を修飾名 `verification.Error` と `internal/verification` 内の非修飾 `Error` に限定する（Phase 2b.1） |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2a。削除の単独コミット。`go tool cover -func` の前後をコミットメッセージに記録）
- [ ] PR-3 マージ済み（対象ステップ: Phase 2b.1）
- [ ] PR-4 マージ済み（対象ステップ: Phase 2b.2 / Phase 2b.3）
- [ ] PR-5 マージ済み（対象ステップ: Phase 2b.4 / Phase 3。`make deadcode` に `runerrors` の行が無い）
- [ ] PR-6 マージ済み（対象ステップ: Phase 4a）
- [ ] PR-7 マージ済み（対象ステップ: Phase 4b。ベンチマークの数値をコミットメッセージに記録）
- [ ] PR-8 マージ済み（対象ステップ: Phase 5。`/mktrans` 済み、0172 の 2 文書に追記済み）
- [ ] すべての AC が §7 の検証で green
- [ ] §4.4 の変異確認をすべて実施し、各コミットメッセージに記録

---

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（ガードテスト・`make` ターゲット・コミット済みスクリプト）、`manual`（PR やデプロイでの観察）を表す。テスト名は `path::TestName` で示す。

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 3、Phase 4a | `test`: `internal/logging/slack_handler_test.go::TestBuildPreExecutionError_FailedFilePaths`（全件表示・表示形の一意性の各行）、`cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupFileVerificationFailureListsFailedFiles` |
| AC-02 | Phase 1、Phase 3、Phase 4a / Phase 4b | `test`: `internal/common/interpolation_test.go::TestWithinInterpolationLimit`、`slack_handler_test.go::TestBuildPreExecutionError_FailedFilePaths`（部分表示・切り詰めの各行）、`integration_pre_execution_error_test.go::TestIntegration_GroupFileVerificationFailureTruncatesLongList` |
| AC-03 | Phase 2b.3、Phase 4a | `test`: `internal/runner/runerrors/pre_execution_test.go::TestNewVerificationPreExecutionError`（`Message` に group 名が無い）、`integration_pre_execution_error_test.go::TestIntegration_GroupFileVerificationFailureListsFailedFiles`（Scope に group 名、`Error Message` に `Group:` 無し）、既存の `internal/runner/runner_test.go::TestRunner_VerificationErrorCarriesGroupScopeAndCleanMessage` |
| AC-04 | Phase 2b.3、Phase 3 | `test`: `pre_execution_test.go::TestNewVerificationPreExecutionError`（`Details` が空の行と 3 テンプレートの行）、`slack_handler_test.go::TestBuildPreExecutionError_FailedFilePaths`（一覧なしの行） |
| AC-05 | Phase 2b.3、Phase 2b.4、Phase 3、Phase 4b | `test`: `pre_execution_test.go::TestNewVerificationPreExecutionError`（グローバルと group が同じテンプレート・同じ `Component`）、`integration_pre_execution_error_test.go::TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope`・`::TestIntegration_GlobalTargetFileVerificationFailureTruncatesLongList` |
| AC-06 | Phase 2b.4、Phase 4a | `test`: `runner_test.go::TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent`（`Runner.Execute` 経由の `failed_file_paths` 属性）、`integration_pre_execution_error_test.go::TestIntegration_GroupFileVerificationFailureListsFailedFiles`・`::TestIntegration_GroupFileVerificationFailureTruncatesLongList`（`mainWithExitCode` → `Runner.Execute` を経由した最終 `Error Message` と省略件数） |
| AC-07 | Phase 4a | `test`: `integration_pre_execution_error_test.go::TestIntegration_GroupFileVerificationFailureListsFailedFiles`・`::TestIntegration_GroupCollectionFailureListsUnresolvedTargets`（`error_type` と Text 行）、既存の `internal/logging/notification_test.go::TestNotificationDefinitions_FieldsAreDeclaredInInventory`（フィールド集合が変わらない） |
| AC-08 | Phase 3 | `test`: `slack_handler_test.go::TestBuildPreExecutionError_FailedFilePaths`（実体参照化で膨らむ要素の行、raw の候補が述語を満たす行）、既存の `notification_test.go::TestNotificationDefinitions_FieldRolesTransformValues` |
| AC-09 | 各 Phase | `static`: `make fmt`・`make test`・`make lint`（各 Phase の完了条件） |
| AC-10 | 各 Phase | `manual`: §4.4 の表に従い変異確認を実施し、コミットメッセージに記録する。`test`: 変異確認の対象は §4.4 の各テスト |
| AC-14 | Phase 2b.2、Phase 4a / Phase 4b | `test`: `internal/logging/pre_execution_error_test.go::TestHandlePreExecutionError_FailedFilePaths`（属性の記録と stderr）、`integration_pre_execution_error_test.go::TestIntegration_GroupFileVerificationFailureListsFailedFiles`・`::TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope`（stderr の `Details:` 行にパスが無い） |
| AC-15 | Phase 4b | `test`: `internal/redaction/redactor_test.go::TestRedactingHandler_SliceStringElementRedaction`（サブテスト `KeywordBearingPathElementIsKept`）、既存の `::TestRedactingHandler_PlainStringIsStillRedacted` |
| AC-16 | Phase 2b.4（`main.go`）、Phase 3、Phase 4b | `test`: `integration_pre_execution_error_test.go::TestIntegration_GlobalTargetFileVerificationFailureUsesGlobalScope`・`::TestIntegration_GlobalTargetFileVerificationFailureTruncatesLongList` |
| AC-17 | Phase 2b.1、Phase 2b.4、Phase 4a | `test`: `internal/verification/manager_test.go::TestVerifyGroupFiles_CollectionFailureCarriesUnresolvedTargets`、`runner_test.go::TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent`（収集失敗の行）、`integration_pre_execution_error_test.go::TestIntegration_GroupCollectionFailureListsUnresolvedTargets` |
| AC-18 | Phase 2b.3、Phase 2b.4 | `test`: `pre_execution_test.go::TestNewVerificationPreExecutionError`、`runner_test.go::TestRunner_VerificationErrorCarriesFailedFilePathsAndComponent`（`Component == verification`）。`static`: `internal/runner/runerrors/pre_execution_guard_test.go::TestFiringPointsUseSharedVerificationConstructor`（複合リテラルとフィールド代入の両構築形） |
| AC-19 | Phase 2b.1、Phase 3 | `test`: `manager_test.go::TestVerificationErrorDetailsAreSorted`（3 経路）、`slack_handler_test.go::TestBuildPreExecutionError_FailedFilePaths`（渡した順に描画する行） |
| AC-20 | Phase 2a、Phase 2b.3 | `static`: `pre_execution_guard_test.go::TestRunerrorsExportsOnlyTheSharedConstructor`、`make deadcode`（`internal/runner/runerrors` の行が無い。Phase 2b 以降）。`manual`: Phase 2a の `go tool cover -func` 前後の記録、README・`package_reference.md` の更新（§8） |
| AC-21 | Phase 2b.1 | `static`: `internal/verification/error_construction_guard_test.go::TestVerificationErrorLiteralsOnlyInConstructor`（複合リテラルの 3 形と `Details` 代入）。`test`: `manager_test.go::TestVerificationErrorDetailsAreSorted`（3 経路。並べ替えを外す変異は §4.4） |
| AC-22 | Phase 2b.4 | `static`: `internal/logging/notification_contract_guard_test.go::TestProductionErrorLiteralsUseTypedComponent` |

---

## 8. 横断検索チェックリスト

`make test`・`make lint` が検出できない残存参照だけを挙げる。§7 の表と重複する項目は置かない。

- [x] 削除した `runerrors` シンボル名（`ClassifiedError`・`ClassifyVerificationError`・`LogClassifiedError`・`LogCriticalToStderr`・`ErrorSeverity`）が `docs/`（`docs/tasks/` 以外）と `README*.md` に残っていないこと。commit `066c9e59` 時点で `rg -n "ClassifiedError|ClassifyVerificationError|LogClassifiedError|LogCriticalToStderr" --glob '!docs/tasks/**' --glob '!internal/runner/runerrors/**' .` は一致なし（実行済み）。
- [x] `runerrors/` の説明「一元化エラー処理」／"Centralized error handling" が `README.ja.md`・`README.md`・`docs/dev/developer_guide/package_reference.md` に残っていないこと（Phase 2a。commit `066c9e59` 時点の該当行は §1.3 に記録）。
- [x] `docs/user/runner_command.ja.md` と `runner_command.md` の見出し構造が一致すること（`scripts/verification/compare_doc_structure.go` を直接実行して確認。見出しの数・レベルは日英で一致し、code block 数の差は本タスク以前からの既存差である。`make verify-docs-checks` は同スクリプトを実行せず、`make verify-docs` の `run_all.sh` も失敗をゲートしないため、確認は情報として行った）。
- [x] `docs/translation_glossary.md` に Phase 5 で新しく使った用語（`Files:` 節、省略件数、収集失敗）の対訳が `/mktrans` により登録されていること。

---

## 9. Success Criteria

- **機能**: AC-01〜AC-08・AC-14〜AC-22 を検証するテスト・ガードが green。
- **品質**: 各 Phase の `make fmt`・`make test`・`make lint` が green。`make deadcode` に `internal/runner/runerrors` の行が無い。§4.4 の変異確認をすべて実施し記録済み。
- **セキュリティ**: `handleErrorCommon` の stderr 出力と `error_message` 属性に失敗ファイルのパスが現れないこと、`failed_file_paths` の要素で値形式の機密がマスクされることがテストで観測される。
- **性能**: ビルダー単体で n = 10,000 の描画が 100 ms 未満で、1 件あたりのコストが n = 1,000 の 3 倍以内。end-to-end は数値を記録する（02_architecture.md §7.7。ベンチマークの数値をコミットメッセージに記録）。
- **文書**: 利用者向け文書（日英）が更新され、0172 アーキテクチャ設計書の動的な値の一覧に行が追加され（`Comments` に記録）、0172 実装計画書 §10 の follow-up に解消が記録されている。

---

## 10. 次のステップ

- 本書は承認済みである。Phase 1（PR-1）から順に実装を開始する。
- Phase 2a は共有コンストラクタの追加と別コミットにし、`go tool cover -func` の前後をコミットメッセージに記す。
- 実装完了後、実チャンネルで group 検証エラーと収集失敗の Slack 表示を確認する（手動。AC の対象外）。
