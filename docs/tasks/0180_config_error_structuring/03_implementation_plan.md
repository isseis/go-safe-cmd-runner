# 実装計画書: 設定の展開・検証のエラー型を構造化する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-10-01 |
| Review date | 2026-10-01 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)
- 実装への引き継ぎ: [implementation_handoff.md](implementation_handoff.md)
- 要件・受け入れ基準プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テストヘルパ配置: [test_organization.md](../../dev/developer_guide/test_organization.md)
- セキュリティ設計: [security-architecture.ja.md](../../dev/architecture_design/security-architecture.ja.md)

本書の用語は [02_architecture.md](02_architecture.md) §0 に従う（構造化メッセージ、部分、役割、断片、`Identifier`・`Path`・`Text`・`Constant`、許可位置、引用する描画）。設計の判断と型・箇所ごとの役割は 02（特に §3.1 と付録 A）を参照し、本書では重複して書かない。

---

## 1. 実装の概要

### 1.1 目的

`internal/runner/config` の全エラー型と、`expansion.go`・`template_expansion.go`・`cli/filter.go` のエラー書式を、役割を型で宣言した構造化メッセージとして運ぶ。機密ではない group 名・コマンド名・変数名・システム環境変数の名前・テンプレート名・パラメータ名が原因で、原因の部分全体が `[REDACTED]` になることを防ぐ。秘密の保護は弱めず、`Text` の部分は変更前と同じ扱いを受ける。`Error()` の文言、stderr の `Details:`、通知の種別と構成は変えない。

設計の全体像は [02_architecture.md](02_architecture.md) §1〜§6、変更する観察可能な挙動は 01「変更の効果」を参照する。

### 1.2 実装方針

1. 文言は構造から作る。構造化メッセージを持つ型の `Error()` は `return e.StructuredMessage().String()` の 1 文にする（02 §1.1）。
2. 役割は型で宣言する。描画済みの文字列を解析して役割を決めない。レベルとフィールドは既存の `Level`・`Field` の値として持ち、`parts()` で宣言する（02 §3.2、CLAUDE.md「Declare, don't infer」）。
3. 引用する描画（`%q`）は 02 §3.4 の `errmsg.Quoted` で表す。エスケープの処理は `errmsg` の中に置き、呼び出し側は整形のバイトを渡さない。
4. 対象は型の集合とファイルの単位で決める。型・関数の一覧を保守しない（01 決定事項 5）。
5. 0178 が `Text` のままにしたエラー書式も、本タスクで固定の文言を `Constant`、挿入する名前を 02 §3.1 の役割として宣言し直す（02 §3.5.2）。
6. 新しいパッケージは作らない。0178 の `errmsg` と既存の `Level`・`Field` を使う。
7. 各 Phase の完了時に `make fmt`（Go を変更した場合）・`make test`・`make lint` を通す（AC-24）。
8. Go のソースコメント・識別子・文字列リテラルは英語で書く。

### 1.3 既存コード調査結果

調査基準: コミット `6742b265`（HEAD）。`git diff --stat dc4585de..HEAD -- internal/runner/config internal/runner/cli internal/errmsg` は空であり、02 §0 が挙げる `dc4585de` の行番号はこのコミットでも成り立つ。`internal/errmsg` は Task 0178 で導入され、役割・`Part`・`Message`・`NewError`・`Cause`・`PathErrorCause`・`IndentedCause` を提供する（`internal/errmsg/errmsg.go`）。config 側の `Level`・`Field` 型と `ErrUndefinedVariableDetail` の `StructuredMessage` は 0178 で導入済みである（`errors.go:191-353`・`:438-479`）。

#### 変更対象の現状

| 領域 | 現状（`6742b265`） | 変更 |
|---|---|---|
| `internal/errmsg/errmsg.go` | `Part` は役割付きの断片か原因の 2 種類（`kind`・`role`・`text`・`cause`・`causeKind`）。`appendSegments` は `partCause` 以外を 1 断片として扱う（`:218-242`） | 引用の中の部分の列を持つ新しい部分の種類と `errmsg.Quoted` を追加（02 §3.4） |
| `internal/runner/config/errors.go` | `Error() string` を持つ型は 30。`Level`・`Field` 型と構築関数はある（`:178-353`）。`Level`/`Field` 欄を文字列で持つ型が残る。`ErrUndefinedVariableDetail` だけが構造化済み | 全型に `StructuredMessage` を追加し、`Error()` をそこから作る。`Level`・`Field` 欄を型にする。`Field` に `output_file` のキーと添字なしの形、`hasName` を加える（02 §3.2・§3.3） |
| `internal/runner/config/template_errors.go` | `Error() string` を持つ型は 21。`Field` はすべて `string`（例: `"cmd"`・`"args[0]"`・`"env_vars"`） | 全型に `StructuredMessage` を追加。`Field` 欄を型にし、`GroupName` から group の `Level` を組む（02 §3.2・§3.3） |
| `internal/runner/config/expansion.go` | `fmt.Errorf` は 10 か所（除外 5 関数の `:332`・`:795`・`:1139`・`:1359`・`:1373`・`:1393`・`:1399`・`:1407`・`:1423`・`:1434`）。0178 が前置き全体を `Text` にした `NewError` は 8 か所（`:843`・`:858`・`:931`・`:972`・`:1008`・`:1038`・`:1181`・`:1284`）。`Level`/`Field` を構築する箇所は `error` の欄に `level.String()`・`field.String()` を渡す | 10 か所を 02 §3.1・付録 A の役割で構造化。8 か所を宣言し直す。構築箇所に型をそのまま渡す。`expandCmdAllowed` の `EvalSymlinks` は `PathErrorCause`（02 §3.5.2） |
| `internal/runner/config/validation.go` | `validateVariableName` が `Level`/`Field` の欄に `level.String()`・`field.String()` を渡す（`:151-152`・`:161-162`・`:178-179`）。位置文字列 `fmt.Sprintf("%s.%s", level, field)` は `variable.ValidateVariableNameForScope` に渡す（`:175`） | 欄に型をそのまま渡す。`:175` は残す（02 §3.2） |
| `internal/runner/config/template_expansion.go` | `fmt.Errorf` は 3 か所（`:512`・`:708`・`:1135`）。`Field` を文字列で受ける関数群（`expandSingleArg`・`expandArrayPlaceholder`・`expandOptionalPlaceholder`・`expandStringPlaceholders`・`validateEnvPost`）と、`field == workDirKey` の文字列比較（`:255`）。`fmt.Sprintf("args[%d]", …)`・`fmt.Sprintf("vars.%s", …)`・`fmt.Sprintf("%s[%d]", …)`・`fmt.Sprintf("env_vars[%d]", …)` で欄を組み立てる（`:425`・`:810`・`:854`・`:871`・`:447`） | 欄を `Field` で受け渡す。3 か所の書式を構造化。固定のキーは構築関数から作る（02 §3.2・§3.5） |
| `internal/runner/cli/filter.go` | `errmsg` を import しない。`fmt.Errorf` は 2 か所（`:51`・`:84`）。`checkGroupsExist` の `config == nil` 分岐（`:50-52`）は `%w` を 2 つ持ち、到達しない | `errmsg` を import し、`:84` を構造化。`:50-52` を削除（02 §3.5.3） |
| `internal/errmsg/errmsg_guard_test.go` | `exemptRolePositions` に `expansion.go` の全体と `errors.go` の 3 関数（`:59-75`）。`inScopeExpansionFile`・`inScopeExpansionExclusions`（`:83-91`）。`pathErrorCausePositions` は tempdir のみ（`:79-81`）。`errmsgPartBuilders` は `rolePart`・`causePart`（`:838`） | 許可位置・対象範囲・構築関数を本タスクに合わせる（02 §3.7） |
| `internal/runner/wrap_guard_test.go` | `inScopeWholeFiles` は expansion.go ほか（`:43-48`）。`inScopeFunctions` に `errors.go` の 3 関数（`:57-59`）。`expansionExcludedFunctions`（`:92-98`） | 対象ファイルを追加し、除外と関数単位の指定をなくす（02 §3.7） |

#### 変更する構築箇所と全出現

`rg` で本番（`internal/`・`cmd/` の `_test.go` を除く `*.go`）を列挙した。行番号は `6742b265`。

| パターン | 全出現箇所 | 対応 |
|---|---|---|
| `fmt.Errorf`（expansion.go） | `ProcessEnvImport:332`・`ProcessEnv:795`・`resolveAndPrepareCommandSpec:1139`・`expandTemplateToSpec:1359`・`:1373`・`:1393`・`:1399`・`:1407`・`:1423`・`:1434` | `errmsg.NewError`／`Constant`・`Quoted`・`Cause` に置き換える |
| `fmt.Errorf`（template_expansion.go） | `validateEnvPre:512`・`validateGlobalOnly:708`・`validateFieldVars:1135` | 同上 |
| `fmt.Errorf`（cli/filter.go） | `checkGroupsExist:51`（削除）・`:84` | `:84` を構造化し、`:50-52` を削除 |
| 前置き全体が `Text` の `NewError`（expansion.go） | `ExpandGlobal:843`・`:858`・`expandCmdAllowed:931`・`:972`・`ExpandGroup:1008`・`:1038`・`expandCommandEnvImport:1181`・`ExpandCommand:1284` | `Constant`・`Ident`・`Text`・`Path` に分け、原因を保つ |
| `Level: level.String()` 系（expansion.go・validation.go） | `expansion.go:194`・`:220`・`:236`・`:248`・`:258`・`:300`・`:314`・`:323`・`:339`・`:426`・`:487`・`:496`・`:539`・`:592`・`:594`・`:595`・`:616`・`:636`・`:646`・`:666`・`:676`・`:688`・`:697`・`:777`・`:786`・`:802`・`:915`・`:980`、`validation.go:151`・`:161`・`:178` | `Level`・`Field` の値（または構築関数）をそのまま渡す。`:915`・`:980` は `groupLevel(...).String()` を型に置き換える |
| `Field: field.String()` 系・文字列リテラルの欄（expansion.go・validation.go・template_expansion.go） | `validation.go:152`・`:162`・`:179`・`expansion.go:195`・`:221`・`:237`・`:249`・`:259`・`:315`・`:324`・`:427`・`:488`・`:803`、`expansion.go:916`・`:981`（`Field: "cmd_allowed"`）、`template_expansion.go:714`・`:1142`・`:1151`・`:572`・`:629`・`:753`・`:765`・`:774`・`:783`・`:887` | 型付き `Field` にする。固定のキーは `cmdField()`・`envField()`・`envImportField()`・`workdirField()`・`varsField()`・`argsFieldNoIndex()`・`envVarsFieldNoIndex()`・`cmdAllowedFieldNoIndex()` から作る |
| `Field` を文字列で受ける関数 | `expandSingleArg`・`expandArrayPlaceholder`・`expandOptionalPlaceholder`・`expandStringPlaceholders`・`validateEnvPre`・`validateEnvPost`（`template_expansion.go:202`・`:247`・`:311`・`:341`・`:487`・`:528`） | 欄を `Field` で受け渡す。呼び出し側（`expansion.go:1371`・`:1405`・`:1421`、`template_expansion.go:426`・`:450`・`:455`・`:467`・`:820`・`:855`・`:872`）も `Field` を渡す |

#### 更新が必要な既存テスト

型・挙動が変わるために更新が要る既存テストである。`make test` のコンパイルエラーで残りを洗い出す。

| テスト | 理由 |
|---|---|
| `internal/runner/config/errors_test.go` | エラー型を文字列の `Level`・`Field` で作っている（`Level:` 17 か所、`Field:` 10 か所）。`TestLevelAndField_StringMatchesLegacyFormat` は新しい `fieldKey`（`output_file`、添字なしの形、`hasName`）を網羅する形に広げる |
| `internal/runner/config/template_expansion_validation_test.go` | `ErrLocalVariableInTemplate`・`ErrUndefinedGlobalVariableInTemplate`（errors.go の型）を文字列の `Field` で作っている（`:409`・`:421`）。errors.go の `Field` を型にする Phase 2 で更新する |
| `internal/runner/config/template_param_expansion_test.go` | `expandSingleArg` に文字列の欄を渡している（`:190`） |
| `internal/runner/config/template_field_constraints_test.go` | `expandSingleArg` に文字列の欄を渡している（`:257`・`:264`） |
| `internal/errmsg/errmsg_guard_test.go` の `TestExemptRoleCallCheckRecognizesForms` | `expansion.go` の除外（`ProcessEnvImport`）で `Ident` が拒否されるケースがある（`:684-687`）。除外をなくすと許容される |
| `internal/runner/wrap_guard_test.go` の `TestScopeCatalogNamesExist` | 除外の一覧（`expansionExcludedFunctions`）の名前の存在を確かめている（`:666-670`） |

`Error()` の文言そのものを確かめる既存テストは変えない。文言は変わらないので、そのまま通ることが AC-14 の確認の一部になる。0178 の `Text` のエラー書式の断片の役割を確かめる既存テストはない（`internal`・`cmd` の `*_test.go` を文言で検索して確認）。

#### 実装への引き継ぎ項目の扱い

- **I-01（レベル・フィールドの値の流入をみるガードの網羅性）**: Phase 5 で、`internal/runner/config` と `internal/runner/cli` の本番コードで、`config.Level`・`config.Field` の値が描画済みの文字列に平らにされる形を検出する静的検査を `internal/runner/config_error_guard_test.go` に置く。対象は、(1) 値に対する `.String()` の呼び出し、(2) `fmt.Sprintf`・`fmt.Errorf`・`fmt.Sprint` 系の引数のうち `%s`・`%v`・`%q` で描画される `config.Level`・`config.Field` の値、の 2 つである。`fmt` はメソッドを暗黙に呼ぶため、(1) だけでは `fmt.Sprintf("%s", level)` を取りこぼす（02 §7.4 の自己テストの例）。許可位置は `validateVariableName` の全体ではなく、`variable.ValidateVariableNameForScope` に渡す位置文字列の式（`validation.go:175` の `fmt.Sprintf("%s.%s", level, field)`）だけに絞る。同関数の他の箇所（`:151-152`・`:161-162`・`:178-179` のエラー型の構築を含む）は許可位置にしない。一時変数へ描画する書き方も、描画の位置で捉える。自己テストで、位置文字列の式は許容し、同じ関数内の他の平ら化（例: 検証の分岐で `errmsg.Text(level.String())` を書く）は拒否することを固定する。
- **I-02（レベル・フィールドの型のガードが認識する名前）**: Phase 5 の同ファイルで、ガードが認識するのは level/field の意味スロットである。すなわち、正確な名前 `Level`・`Field` と、既に使われている `...Level` の規則（`EnvImportLevel`・`VarsLevel`）である。名前が `Field` で終わる欄については、検証されていない名前を保持して `Text` に属する生の値の `string` の欄（`UnknownField string` など）を拒否してはならない。ガードに明示的な例外集合を設けて自己テストで固定するか、規則を実際の level/field の位置スロットに限る。認識する名前の規則は 1 か所で定義し、自己テストで固定する。この規則は `Location` のような別名の欄を捉えない。AC-04 の主たる確認は型ごとのセグメントのテストであり、検査は普通の書き方の誤りを補助的に捉えるものである。

#### ガードの現状と本タスクの変更

| ガード | 現状（`6742b265`） | 変更 |
|---|---|---|
| `errmsg` の許可位置 | `exemptRolePositions`・`pathErrorCausePositions`・`declaresExemptRole`・`checkExemptRoleCalls`（`errmsg_guard_test.go:59-91`・`:582-624`） | errors.go・template_errors.go・template_expansion.go・cli/filter.go をファイル全体に加え、expansion.go の除外をなくす。`PathErrorCause` に `expandCmdAllowed` を加える |
| `errmsg` の部分の構築 | `errmsgPartBuilders`（`:838`） | 引用の部分を構築する関数を加える。`rolePart` を呼ぶなら `errmsgRoleChoosers["rolePart"]` も更新する |
| wrap guard | `inScopeWholeFiles`・`inScopeFunctions`・`expansionExcludedFunctions`（`wrap_guard_test.go:43-98`） | 対象ファイルを加え、除外と関数単位の指定をなくす |
| エラー型の網羅 | 無い | 新設。対象パッケージで `Error() string` を持つ型が `StructuredMessage() errmsg.Message` を持つことを確かめる（AC-01） |
| レベル・フィールドの型 | 無い | 新設。エラー型の `Level`・`Field` 欄が型付きであることを確かめる（AC-04・AC-21） |
| レベル・フィールドの値の流入 | 無い | 新設。`.String()` と `fmt` の `%s`・`%v`・`%q` で平らにされていないことを確かめる。許可位置は `validateVariableName` の位置文字列の式だけにする（AC-21） |

#### 外部前提の確認

- `make test` は `$(ENVSET) CGO_ENABLED=1 go test -tags test -race -p 4 -v ./...` と非 Darwin 向けの `CGO_ENABLED=0` の 2 回を実行する（`Makefile:479-485`・`:547`）。`ENVSET` は環境変数を空にする `env -i` である（`Makefile:83-95`）。`make lint` は `golangci-lint run --build-tags test` を実行する（`Makefile:23-24`・`:150-156`）。`make fmt` は gofumpt を使う（`Makefile:606-607`）。
- `scripts/verification/check_*.sh` は `make verify-docs-checks`（`Makefile:792-801`）が `sh` で自動実行する。スクリプトは POSIX sh で書く。
- `identitymutationguard.ProductionGoFilesInRepo` は `internal`・`cmd` を走査し、`_test.go` を除く（`identitymutationguard/helpers.go`）。`internal/runner` の新しいガードファイルは `_test.go` なので対象外である（`//go:build test` も付けるが、除外の理由は `_test.go` の接尾辞である）。
- `strconv.Quote` は `%q` と同じ結果を返す（01 §0「引用する描画」）。`errmsg.Quoted` の契約の根拠である。

#### 02 との食い違い

- 02 §3.2 は `Level.String()`・`Field.String()` を残す理由に「`validateVariableName` が `variable.ValidateVariableNameForScope` に渡す位置の文字列」と「テンプレートの警告の文言」を挙げる。しかし `6742b265` では、位置の文字列（`validation.go:175`）以外に `Level`・`Field` を描画する本番の警告は無い（`internal`・`cmd` の `_test.go` を除く `*.go` を検索して確認）。決定は変えない。本計画は `String()` を残し、値の流入の検査の許可位置を `validateVariableName` の位置文字列の式（`validation.go:175`）だけにする。

### 1.4 テストヘルパーの方針

- 新しいクロスパッケージのヘルパ・モックは作らない。既存の `cmd/runner` の Slack モック（`runMainWithSlackMock`・`requireSinglePreExecutionError`・`attachmentField`）と `internal/runner` の `captureExecutionErrorReport` で足りる。
- ガードテストは `_test.go` の本番パッケージ内テストとして置き、新しい `test_helpers.go` は作らない。`internal/runner/config_error_guard_test.go` は `//go:build test` を付け、`wrap_guard_test.go` と同じ `package runner` の非公開の仕組み（`wrapProductionGuardSet`・`wrapTypedPackage`・`declaresStructuredMessage`）を再利用する。
- パッケージ内のテストデータは、`internal/runner/config` の中では非公開の構築関数（`groupLevel()` など）で、外からはゼロ値または公開の `errmsg` の構築関数で作る。

---

## 2. 実装ステップ

各 Phase の完了時に `make fmt`（Go を変更した場合）・`make test`・`make lint` を通す（AC-24）。追加・変更したテストは §4.4 の変異で失敗することを確認し、その旨をコミットメッセージに記す。Phase の順序は 02 §8 の 1〜5 をそのまま使う。

### Phase 1: 基盤（`errmsg.Quoted` と `Field` の拡張）

**Files**: `internal/errmsg/errmsg.go`・`internal/errmsg/errmsg_test.go`・`internal/errmsg/errmsg_guard_test.go`（変更）、`internal/runner/config/errors.go`・`internal/runner/config/errors_test.go`（変更）

- [x] 02 §3.4 の `errmsg.Quoted(parts ...Part) Part` を実装する。中の部分の列を持つ新しい部分の種類を加え、`Part.appendSegments`（`:218`）に、引用符を `Constant` の断片に、中の断片を役割を保ったまま `strconv.Quote` の規則でエスケープした断片の列に展開する分岐を加える。エスケープのしかたが全体の `strconv.Quote` と一致しない場合は引用の全体を 1 つの `Text` にする（fail-closed）。中の部分に原因があれば panic する。
- [x] `errmsgPartBuilders` に引用の部分を構築する関数（`Quoted` 自身か補助関数）を加える。`Quoted` が引用符のために `rolePart` を呼ぶなら `errmsgRoleChoosers["rolePart"]` にも加える。`TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg`・`TestProductionExemptRoleCallsAreInAllowedPositions` と各自己テストを更新する（AC-20・AC-22）。
- [x] `errmsg_test.go` に `Quoted` のテストを置く。`"`・`\`・非 ASCII の文字・不正な UTF-8 のバイト列・断片の境目で分かれる文字を含む入力で、描画が `strconv.Quote` と一致すること。境目で分かれる入力では引用の全体が 1 つの `Text` の断片になること。中の断片の役割が保たれること。原因の部分を渡すと panic すること（AC-14）。
- [x] `errors.go` の `Field` に 02 §3.2 の拡張を加える。`output_file` のキー、添字なしの形の `args`・`env_vars`・`cmd_allowed`、`vars` の名前の有無を表す独立の欄。`String()` と `parts()` の両方にキーと形を足し、添字と名前の有無で描画を切り替える。
- [x] `errors_test.go` の `TestLevelAndField_StringMatchesLegacyFormat` を、`fieldKey` のすべての値と添字・名前の有無を反復する形に広げる（手で書いたキーの一覧を使わない）。期待値は変更前の `fmt.Sprintf` を再現して組み立て、`parts()` をつないだ文字列が `String()` と一致すること、`errmsg.Quoted(f.parts()...)` の描画が `strconv.Quote(f.String())` と一致することも確かめる（AC-04・AC-14）。
- [x] 空文字列の `vars` のキーの扱いを固定する。`varField("")`・`varElementField("", i)` が、変更前の `fmt.Sprintf("vars.%s", "")`・`fmt.Sprintf("vars.%s[%d]", "", i)` と同じ `vars.`・`vars.[i]` を描画することをテストで確かめる（`hasName` を独立に持つ理由。AC-14）。

**完了条件**: `go test -tags test ./internal/errmsg/... ./internal/runner/config/...` が green。`errmsg` の本番ファイルが標準ライブラリだけを import する。`Field` の既存の描画（名前が空でない場合、および添字を持つ場合）が変わっていない。

### PR-1 作成ポイント: quoted rendering and the field extension

- **対象ステップ**: Phase 1
- **推奨タイトル**: `feat(0180): add errmsg.Quoted and extend the config Field`
- **レビュー観点**: `Quoted` の描画が `strconv.Quote` とバイト単位で一致すること、境目で分かれる入力の fail-closed、原因を拒否する panic、`Field` の新しいキーと形が既存の描画を変えないこと、許可位置と構築関数のガードが新しい構築関数を正しく扱うこと
- **実装モデル要件**: frontier-required
- **判定理由**: エスケープと断片の境目の扱いが本タスクの文言一致（AC-14）の土台であり、誤ると広範囲のエラー文言が変わる

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 2: 変数の展開

**Files**: `internal/runner/config/errors.go`・`expansion.go`・`validation.go`・`template_expansion.go`（変更、後者は 2 型の構築箇所のみ）、`internal/runner/config/errors_test.go`・`expansion_test.go`・`template_expansion_validation_test.go`（変更）、`internal/errmsg/errmsg_guard_test.go`・`internal/runner/wrap_guard_test.go`（変更）、`cmd/runner/integration_pre_execution_error_test.go`（変更）

- [x] 02 §3.3 のとおり、`errors.go` で `Error() string` を持つ型（`ErrUndefinedVariableDetail` を除く）に `StructuredMessage() errmsg.Message` を加え、`Error()` を `return e.StructuredMessage().String()` にする。部分の並びと役割は 02 付録 A による。`Unwrap()`・`Is()` は変えない（AC-02・AC-03・AC-05・AC-14・AC-15）。
- [x] `errors.go` のエラー型の `Level`・`Field`・`EnvImportLevel`・`VarsLevel` 欄を、02 §3.2 のとおり型 `Level`・`Field` にする。
- [x] `expansion.go`・`validation.go` の構築箇所で、`Level`・`Field` を型のまま（または構築関数から）渡す。`:175` の位置文字列は残す。`template_expansion.go` の `ErrLocalVariableInTemplate`・`ErrUndefinedGlobalVariableInTemplate` の `Field` の欄も型にする（`:714`・`:1142`・`:1151`）。
- [x] `expansion.go` の 10 か所の `fmt.Errorf` を 02 §3.5.2 と付録 A の役割で構造化する。8 か所の前置き全体が `Text` の `NewError` を宣言し直す。`expandCmdAllowed` の `EvalSymlinks` の失敗は `errmsg.PathErrorCause` にする。`ErrForbiddenEnvVar` などのセンチネルエラーは `errmsg.Cause` として同じ位置に置く（AC-06・AC-07・AC-14・AC-15）。
- [x] `errors_test.go` に `TestExpandCmdAllowed_ResolvePathCauseKeepsPath` を加え、`EvalSymlinks` の失敗の原因で、パスが `Path` の断片として残ることと、組み立てた `Error()` が変更前と同じであることを確かめる（AC-07・AC-14）。
- [x] `errmsg_guard_test.go` を更新する。`exemptRolePositions` に `errors.go` をファイル全体として加え、`inScopeExpansionFile`・`inScopeExpansionExclusions` と `declaresExemptRole` の例外をなくす。`pathErrorCausePositions` に `expansion.go` の `expandCmdAllowed` を加える。`PathErrorCause` の違反の文言を直す。`TestExemptRoleCallCheckRecognizesForms` の除外のケースを、除外が無くなった後の形に直す（AC-20・AC-22）。
- [x] `wrap_guard_test.go` を更新する。`inScopeWholeFiles` に `errors.go` を加え、`inScopeFunctions` から `errors.go` の 3 関数を外す。`TestScopeCatalogNamesExist`・`TestWrapCheckRecognizesForms` の関連を直す（AC-06・AC-20）。
- [x] `errors_test.go` の 27 か所の文字列の `Level`・`Field` を型付きの値に書き換える。`template_expansion_validation_test.go` の `ErrLocalVariableInTemplate`・`ErrUndefinedGlobalVariableInTemplate`（errors.go の型）の文字列の `Field`（`:409`・`:421`）も、本 Phase で型付きの値に書き換える（書き換えないとパッケージのテストがコンパイルできない）。
- [x] `expansion_test.go` に `TestExpansionWrapSites_StructuredMessage` を加え、`expansion.go` のエラー書式の各箇所を実行する。固定の文言が `Constant`、名前が 02 §3.1 の役割、原因が保たれること、**組み立てた `Error()` が変更前の `fmt.Sprintf` を再現した文字列と一致すること**、`errors.Is`・`errors.AsType` が変更前と同じ対象に届くことを確かめる。`errmsg.NewError` の panic が起きないこともこの実行で確かめる（AC-07・AC-14・AC-15）。変更前の文言の比較では、以前 `%q` を使っていた各箇所について、少なくとも 1 つは `"` と `\` を含む値を実行することを義務とする。これにより、引用・識別子を `errmsg.Quoted` を通さずに描画する実装が、ヘルパ単体のテストだけでなく箇所ごとの比較でも捉えられる（検証の義務であり、テストコードの構造は問わない）。
- [x] `errors_test.go` に `TestErrorTypes_StructuredMessageSegments`（各型の `StructuredMessage().Segments()` が 02 付録 A と一致すること）と `TestErrorTypes_ErrorMessageMatchesLegacyFormat`（各型の `Error()` が変更前の `fmt.Sprintf` の結果と一致すること）を加える（AC-02・AC-03・AC-05・AC-14）。変更前の文言の比較では、以前 `%q` を使っていた各エラー型について、少なくとも 1 つは `"` と `\` を含む値を実行することを義務とし、引用・識別子を `errmsg.Quoted` を通さずに描画する実装を捉えられるようにする（検証の義務であり、テストコードの構造は問わない）。
- [x] 02 §3.7 に従い、`errmsg_guard_test.go` の `TestExemptRoleCallCheckRecognizesForms` のうち `inScopeExpansionFile` を定数として使うケース（`:680-682`）を、パスを直接書く形に直す（AC-22）。
- [x] `cmd/runner/integration_pre_execution_error_test.go` に、group の展開（`env_import` の allowlist 違反）の Slack の `Error Message` にシステム環境変数の名前・変数名・group 名が出ることを確かめるテストを加える（AC-09）。global の `vars` の循環参照で経路の変数名が出ることを確かめるテストを加える（AC-10）。

**完了条件**: `internal/runner/config`・`internal/runner`・`cmd/runner` のテストが green。`ErrUndefinedVariableDetail` を含む errors.go の各型の `Error()` の文言が変更前と同じである。AC-09・AC-10 のテストが green。

### PR-2 作成ポイント: structure the variable-expansion errors

- **対象ステップ**: Phase 2
- **推奨タイトル**: `feat(0180): structure config variable-expansion errors`
- **レビュー観点**: 各型の `Error()` の文言が変更前と同じであること、`Level`・`Field` の型変更が全構築箇所に及ぶこと、`expansion.go` の運びうる原因の構造が保たれること、`PathErrorCause` の位置、許可位置と wrap guard の範囲が 02 §3.7 と一致すること
- **実装モデル要件**: frontier-recommended
- **判定理由**: 変更範囲が広い機械的な型変更だが、原因の到達性と役割の割り当ての網羅が正しさを決める

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた

### Phase 3: テンプレート

**Files**: `internal/runner/config/template_errors.go`・`template_expansion.go`・`expansion.go`（`expandSingleArg` の呼び出し）（変更）、`internal/runner/config/template_errors_test.go`・`template_expansion_test.go`・`template_param_expansion_test.go`・`template_field_constraints_test.go`（変更）、`internal/errmsg/errmsg_guard_test.go`・`internal/runner/wrap_guard_test.go`（変更）、`cmd/runner/integration_pre_execution_error_test.go`（変更）

- [x] 02 §3.3 のとおり、`template_errors.go` の全型に `StructuredMessage` を加え、`Error()` をそこから作る。部分の並びと役割は 02 付録 A による（AC-02・AC-03・AC-14）。
- [x] `template_errors.go` の `Field` 欄を型 `Field` にする。`ErrTemplateFieldConflict`・`ErrMissingRequiredField` の `group[<name>]` は `GroupName` から group の `Level` の `parts()` で描画する（02 §3.2）。
- [x] `template_expansion.go` の `template_errors.go` の型の構築箇所で、`Field` を型のまま渡す。`fmt.Sprintf` で欄を組み立てている箇所（`:425`・`:447`・`:810`・`:854`・`:871`）を `Field` の構築関数に置き換える。
- [x] `expandSingleArg`・`expandArrayPlaceholder`・`expandOptionalPlaceholder`・`expandStringPlaceholders`・`validateEnvPre`・`validateEnvPost` の欄を `Field` で受け渡す。`field == workDirKey` の文字列比較（`:255`）を `Field` のキーで判定する。`expansion.go` の `expandSingleArg` 呼び出し（`:1371`・`:1405`・`:1421`）も欄を `Field` にする。
- [x] `template_expansion.go` の 3 か所の `fmt.Errorf` を 02 §3.1・付録 A の役割で構造化する。`:708`・`:1135` の拒否された参照名は `Text`、`:512` はキーと原因を宣言する（AC-06・AC-07・AC-14・AC-15）。`:512` は、それを踏む KEY を持つ入力が先の入力全体のパースで失敗するため到達しない防御的な分岐である（テストのコメントに明記）。
- [x] `errmsg_guard_test.go` の `exemptRolePositions` に `template_errors.go`・`template_expansion.go` をファイル全体として加える。`wrap_guard_test.go` の `inScopeWholeFiles` にも両ファイルを加える（AC-06・AC-20）。
- [x] `template_errors_test.go` に `TestTemplateErrorTypes_StructuredMessageSegments`（各型のセグメントが 02 付録 A と一致すること）と `TestTemplateErrorTypes_ErrorMessageMatchesLegacyFormat`（各型の `Error()` が変更前の `fmt.Sprintf` の結果と一致すること）を加える。`template_param_expansion_test.go`・`template_field_constraints_test.go` の欄を型に合わせる（AC-02・AC-03・AC-14）。変更前の文言の比較では、以前 `%q` を使っていた各エラー型について、少なくとも 1 つは `"` と `\` を含む値を実行することを義務とする（検証の義務であり、テストコードの構造は問わない）。
- [x] `template_expansion_test.go` に `TestTemplateExpansionWrapSites_StructuredMessage` を加え、`template_expansion.go` の 3 か所のエラー書式を実行する。固定の文言が `Constant`、名前が 02 §3.1 の役割、**組み立てた `Error()` が変更前の `fmt.Sprintf` を再現した文字列と一致すること**、`errors.Is`・`errors.AsType` の到達性を確かめる（AC-07・AC-14・AC-15）。変更前の文言の比較では、以前 `%q` を使っていた各箇所について、少なくとも 1 つは `"` と `\` を含む値を実行することを義務とする（検証の義務であり、テストコードの構造は問わない）。
- [x] `cmd/runner/integration_pre_execution_error_test.go` に、`ValidateAllTemplates` の失敗でテンプレート名と変数名が出ることを確かめるテスト（AC-11）と、コマンドの展開でテンプレートの展開が失敗しテンプレート名・パラメータ名・コマンド名・group 名が出ることを確かめるテスト（AC-13）を加える。

**完了条件**: `internal/runner/config`・`cmd/runner` のテストが green。`template_errors.go` の各型の `Error()` の文言が変更前と同じである。AC-11・AC-13 のテストが green。

### PR-3 作成ポイント: structure the template errors

- **対象ステップ**: Phase 3
- **推奨タイトル**: `feat(0180): structure config template errors`
- **レビュー観点**: 各型の `Error()` の文言が変更前と同じであること、`Field` の型変更が全構築箇所と `expandSingleArg` の呼び出しに及ぶこと、`workdir` の判定が `Field` のキーであること、wrap guard の範囲が 02 §3.7 と一致すること
- **実装モデル要件**: frontier-recommended
- **判定理由**: 型変更が広く、テンプレートの欄の組み立てを宣言に置き換える判断を伴う

- [x] `make test && make lint` が green であることを確認した
- [x] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた

### Phase 4: `--groups`

**Files**: `internal/runner/cli/filter.go`・`internal/runner/cli/filter_test.go`（変更）、`internal/errmsg/errmsg_guard_test.go`・`internal/runner/wrap_guard_test.go`（変更）、`cmd/runner/integration_pre_execution_error_test.go`・`cmd/runner/integration_test_helpers.go`（変更）

- [ ] 02 §3.5.3 のとおり、`filter.go` の `checkGroupsExist` の `config == nil` 分岐（`:50-52`）を削除する。
- [ ] 存在しない group 名のエラー（`:84-85`）を `errmsg.NewError` で作る。センチネルエラー `ErrGroupNotFound` を原因、指定された名前と定義済みの group 名の各要素を `Identifier`、`%v` の括弧・区切り・固定の文言を `Constant` として宣言する。一覧の順序は変えない（AC-08）。
- [ ] `errmsg_guard_test.go` の `exemptRolePositions` に `cli/filter.go` をファイル全体として加える。`wrap_guard_test.go` の `inScopeWholeFiles` にも `cli/filter.go` を加える（AC-06・AC-20）。
- [ ] `filter_test.go` に `TestFilterGroups_GroupNotFoundStructuredMessage` を加え、存在しない group 名で返すエラーが `errmsg.Structured` を実装し、指定した名前と定義済みの group 名が `Identifier` で、`errors.Is(err, ErrGroupNotFound)` が成り立つことを確かめる。定義済みの group 名の一覧は順序に依らずに確かめる（02 §5.5）。`nil` config の結果が変わらないことを既存のテスト（`:121-125`）で確かめる（AC-08・AC-15）。
- [ ] `cmd/runner/integration_pre_execution_error_test.go` に、`--groups` に存在しない名前を指定し指定した名前と定義済みの group 名が出ることを確かめるテストを加える（AC-12）。
- [ ] `cmd/runner/integration_test_helpers.go` のハーネスを拡張し、シナリオが `--groups` を渡せるようにする。`slackRunSpec` に group 名の値（`groups`）を加え、`runMainWithSlackMock` が現在 `groups` を `""` にリセットしている箇所へその値を配線する。これが無いと AC-12 のシナリオは `cli.FilterGroups` に到達せず、テストは主張した理由で失敗できない。

**完了条件**: `internal/runner/cli`・`cmd/runner` のテストが green。AC-08・AC-12 のテストが green。

### PR-4 作成ポイント: structure the --groups error

- **対象ステップ**: Phase 4
- **推奨タイトル**: `feat(0180): structure the --groups group-not-found error`
- **レビュー観点**: `errors.Is(err, ErrGroupNotFound)` の到達性、指定された名前と定義済みの group 名の役割、削除した到達不能な分岐の扱い、一覧の順序を変えないこと
- **実装モデル要件**: standard
- **判定理由**: 変更範囲が 1 ファイルに閉じ、設計は 02 §3.5.3 で確定している

- [ ] `make test && make lint` が green であることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた

### Phase 5: 仕上げ（ガードの新設、保護のテスト、文書）

**Files**: `internal/runner/config_error_guard_test.go`（新規・`//go:build test`）、`internal/runner/config/security_redaction_test.go`（新規）、`internal/logging`・`internal/runner`・`cmd/runner` の既存テスト（追加）、`docs/dev/architecture_design/security-architecture.ja.md`・`.md`、`docs/tasks/0178_structured_error_message_redaction/03_detailed_specification.md`、`scripts/verification/check_structured_message_redaction_docs.sh`・`..._selftest.sh`

- [ ] `internal/runner/config_error_guard_test.go` に、02 §3.7 の「エラー型の網羅」の検査を実装する。対象パッケージ（`internal/runner/config`）で宣言され `Error() string` を持つすべての型が、`StructuredMessage() errmsg.Message` を持つことを確かめる。型の別名は指す型として確かめる。対象の型の一覧は保守しない（AC-01）。
- [ ] 同ファイルに、02 §3.7 の「レベル・フィールドの型」の検査を実装する。level/field の意味スロット、すなわち正確な名前 `Level`・`Field` と既に使われている `...Level` の規則（`EnvImportLevel`・`VarsLevel`）が型 `Level`・`Field` であることを確かめる。名前が `Field` で終わる欄でも、検証されていない名前を保持して `Text` に属する生の値の `string` の欄（`UnknownField string` など）は拒否しない。明示的な例外集合を設けて自己テストで固定するか、規則を実際の level/field の位置スロットに限る。認識する名前の規則は 1 か所で定義し、自己テストで固定する（I-02、AC-04・AC-21）。
- [ ] 同ファイルに、02 §3.7 の「レベル・フィールドの値の流入」の検査を実装する。`internal/runner/config` と `internal/runner/cli` の本番コードで、`config.Level`・`config.Field` の値が (1) `.String()` で描画される形と、(2) `fmt.Sprintf`・`fmt.Errorf`・`fmt.Sprint` 系の引数で `%s`・`%v`・`%q` として描画される形を検出する。許可位置は `validateVariableName` の全体ではなく、`variable.ValidateVariableNameForScope` に渡す位置文字列の式（`validation.go:175`）だけとし、同関数の他の箇所（エラー型の構築を含む）も含めてそれ以外を拒否する（I-01、AC-21）。(1) だけでは `fmt.Sprintf("%s", level)` を取りこぼす。
- [ ] 3 つの検査に、対象の実装を壊すと失敗することを示す自己テストを付ける。変異の例: `StructuredMessage` を持たないエラー型を加える、あるいは `Level` 欄を `string` に戻す、`Ident` の引数に `Level.String()` を渡す、`Ident` の引数に `fmt.Sprintf("%s", level)` を渡す、許可位置の外で `.String()` を呼ぶ。値の流入の検査では、位置文字列の式は許容し、`validateVariableName` の他の分岐で `errmsg.Text(level.String())` を書くと拒否されることも確かめる（AC-22）。
- [ ] `internal/runner/config/security_redaction_test.go` に、02 §7.3 の保護のテストを置く。`Text` として宣言した部分（拒否された名前・生の設定値）に値全体置換だけが反応する入力を与え、その部分が置換文字列になること（AC-18）。`Identifier` 以外の部分に値形式の検出だけが反応する値（GitHub トークン形式）を含む `env` のエントリやテンプレートの入力文字列が、変更後もマスクされること（AC-19）。
- [ ] `cmd/runner/integration_pre_execution_error_test.go` に `TestIntegration_PreExecutionConfigErrors_OutputContract` を加え、AC-16・AC-17 が名指しする producer をすべて駆動する。Phase 2〜4 で作った config エラーのシナリオ（`ExpandGlobal`・`ExpandGroup`・`ValidateAllTemplates`・`cli.FilterGroups`）に加え、AC-13 の producer であるコマンド・テンプレートの展開（`ExpandCommand`）を対象に含め、次の両方を確かめる。(1) stderr の `  Details:` のブロック全体（`stderrDetailsBlock`。missing-groups の `Available groups:` 行のような継続行を含む）の文言が変更前と同じであること（AC-16）。(2) 通知の件数・Scope・フィールドの構成が変更前と同じであること（`run.payloads`）と、`message_type`・`error_type` が変更前と同じであること（`jsonLogRecords` で読む JSON ログの属性）（AC-17）。
- [ ] `docs/dev/architecture_design/security-architecture.ja.md` の「識別子の型宣言による免除」に、01 AC-23 の内容を追記する。システム環境変数の名前・テンプレート名・パラメータ名を `Identifier` とすること、名前の検証で拒否された名前を `Text` とすること、`--groups` で指定された名前・存在しないテンプレートへの参照名・重複して定義されたテンプレート名を `Identifier` とすること、誤って秘密を名前の位置に書いた場合の保護の境界（`ErrTemplateContainsNameField` のテンプレート名を含む）を記す（AC-23）。
- [ ] `docs/dev/architecture_design/security-architecture.md` を `/mktrans` で日本語版と同じ内容に反映する。用語集に不足があれば登録する（AC-23）。
- [ ] `docs/tasks/0178_structured_error_message_redaction/03_detailed_specification.md` の対象の範囲の記述（`expansion.go` の除外、config の `*...Detail` 型を範囲外とする例外）に、本タスクで範囲に入ったことを注記する（02 §3.6）。
- [ ] `scripts/verification/check_structured_message_redaction_docs.sh` に、追記した語（システム環境変数の名前・テンプレート名・パラメータ名・`--groups`・拒否された名前・保護の境界）を日英それぞれの必須語として加え、`check_structured_message_redaction_docs_selftest.sh` のフィクスチャを更新する。`make verify-docs-checks` を実行して green を確かめる（AC-23）。
- [ ] 最終の `make test`・`make lint` を通す（AC-24）。

**完了条件**: 3 つの新しいガードと自己テストが green。保護のテストが green。`make verify-docs-checks`・`make test`・`make lint` が green。日英の文書が同じ内容である。

### PR-5 作成ポイント: guards, protection tests, and documentation

- **対象ステップ**: Phase 5
- **推奨タイトル**: `feat(0180): guard config error declarations and document the boundary`
- **レビュー観点**: エラー型の網羅の検査がパッケージの型宣言を走査すること、レベル・フィールドの型と値の流入の検査が I-01・I-02 の取りこぼしを防ぐこと、各検査の自己テストが空振りしないこと、保護のテストが層を切り分ける入力を使っていること、日英の文書が同じ内容であること
- **実装モデル要件**: frontier-recommended
- **判定理由**: 3 つの検査の対象の決め方と、層を切り分ける保護のテストの組み立てに設計の理解が要る

- [ ] `make test && make lint` が green であることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 完了条件 |
|---|---|---|
| M1: 基盤 | Phase 1 | `errmsg.Quoted` と拡張した `Field` のテストが green。`Field` の既存の描画が変わっていない |
| M2: 変数の展開 | Phase 2 | errors.go と expansion.go の対象が構造化され、`Error()` の文言と到達性が変わらない。AC-09・AC-10 が green |
| M3: テンプレート | Phase 3 | template_errors.go と template_expansion.go の対象が構造化され、文言と到達性が変わらない。AC-11・AC-13 が green |
| M4: `--groups` | Phase 4 | `cli.FilterGroups` のエラーが構造化され、`errors.Is` の到達性が変わらない。AC-08・AC-12 が green |
| M5: 網羅と文書 | Phase 5 | 3 つの新しいガード、保護のテスト、日英の文書、`make verify-docs-checks`・`make test`・`make lint` が green |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `errmsg.Quoted`、`Field` の拡張 | frontier-required |
| PR-2 | Phase 2 | errors.go の型と `expansion.go` のエラー書式・変数の展開 | frontier-recommended |
| PR-3 | Phase 3 | `template_errors.go` の型・`template_expansion.go` のエラー書式 | frontier-recommended |
| PR-4 | Phase 4 | `cli.FilterGroups` | standard |
| PR-5 | Phase 5 | 3 つのガード・保護のテスト・日英の文書 | frontier-recommended |

### 3.3 順序の根拠

02 §8 の順序 1〜5 をそのまま保つ。Phase 2 以降は Phase 1 の `Field`・`Quoted` に依存する。Phase 2 と Phase 3 はファイルを分けて段階的に wrap guard の範囲を広げる（ファイル全体を範囲に加えるのは、そのファイルの型をすべて構造化した段階である。02 §8）。Phase 4 は独立だが、`pathErrorCausePositions`・`exemptRolePositions` の更新を Phase 2・3 と分けるため後ろに置く。Phase 5 の 3 つのガードは、errors.go・template_errors.go・cli/filter.go の型と欄がすべて整った後でなければ green にならない。

---

## 4. テスト戦略

01「テストの入力についての制約」と 02 §7 に従う。層ごとの効果を確かめるテストは、1 つの層だけが反応する入力を使い、ほかの層だけでは入力が変わらないことを先に確かめる。`Identifier` の免除を示すテストは、同じメッセージに同じ文字列の `Text` の対照の部分を置く。

### 4.1 単体テスト

- エラー型ごと（`errors_test.go`・`template_errors_test.go`）: `StructuredMessage().Segments()` の役割が 02 付録 A と一致すること、`Error()` が変更前の `fmt.Sprintf` の結果と一致すること（AC-02・AC-03・AC-05・AC-14）。変更前の文言はテストの中で再現して比べる。以前 `%q` を使っていた各エラー型の比較では、`"` と `\` を含む値を少なくとも 1 つ実行し、引用・識別子を `errmsg.Quoted` を通さずに描画する実装を捉える（検証の義務）。
- `errmsg.Quoted`（`errmsg_test.go`）: 02 §7.1 の入力と契約（AC-14）。
- `Field`（`errors_test.go`）: `fieldKey` のすべての値と添字・名前の有無を反復し、`String()`・`parts()`・`strconv.Quote` の一致（AC-04・AC-14）。
- エラー書式（`expansion_test.go`・`template_expansion_test.go`）: `TestExpansionWrapSites_StructuredMessage`・`TestTemplateExpansionWrapSites_StructuredMessage` で各箇所を実行し、固定の文言が `Constant`、名前が 02 §3.1 の役割、組み立てた `Error()` が変更前と同じであること、`errors.Is`・`errors.AsType` が変更前と同じ原因に届くこと（AC-07・AC-14・AC-15）。以前 `%q` を使っていた各箇所の比較では、`"` と `\` を含む値を少なくとも 1 つ実行する（検証の義務）。
- `cli.FilterGroups`（`filter_test.go`）: `errmsg.Structured`・役割・`errors.Is`（AC-08）。
- 保護（`security_redaction_test.go`）: 02 §7.3（AC-18・AC-19）。

### 4.2 統合テスト

- AC-09〜AC-13 の例示のシナリオを、エラーの発生元（`ExpandGlobal`・`ExpandGroup`・`ValidateAllTemplates`・`cli.FilterGroups`・group executor）から Slack のメッセージの組み立てまで通す。`cmd/runner/integration_pre_execution_error_test.go` の `runMainWithSlackMock`・`requireSinglePreExecutionError`・`attachmentField` を使う。0178 のシナリオテストと同じ仕組みを使う（02 §7.2）。
- AC-16・AC-17: 同じシナリオで、stderr の `Details:` の文言と、通知の件数・`message_type`・`error_type`・Scope・Slack のフィールド構成が変更前と同じであることを確かめる。

### 4.3 ガードのテスト

- 3 つの新しい検査（`config_error_guard_test.go`）と、`errmsg_guard_test.go`・`wrap_guard_test.go` の更新した検査に、自己テストを付ける（AC-22）。

### 4.4 実装時に行うテスト失敗確認

各 Phase の完了時に、次の変異を入れて当該テストが失敗することを確認し、コミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。

| Phase | 変異 | 失敗するテスト |
|---|---|---|
| 1 | `Quoted` のエスケープを `strconv.Quote` と別の規則にする | `errmsg_test.go::TestQuoted_MatchesStrconvQuote` |
| 1 | 断片の境目で分かれる入力の fail-closed を外す | `errmsg_test.go::TestQuoted_SplitRuneFallsBackToText` |
| 1 | `Field` の添字なしの形が添字を描画するようにする | `errors_test.go::TestLevelAndField_StringMatchesLegacyFormat` |
| 2 | エラー型の 1 つの `StructuredMessage` の断片の役割を変える | `errors_test.go::TestErrorTypes_StructuredMessageSegments` |
| 2 | `expansion.go` の 1 か所のエラー書式の `Constant` の文言を 1 文字変える | `expansion_test.go::TestExpansionWrapSites_StructuredMessage` |
| 2 | `expansion.go` の 1 か所に `fmt.Errorf` を戻す | `wrap_guard_test.go::TestInScopeWrapsUseStructuredErrors` |
| 2 | 許可位置の外で `Ident` を呼ぶ | `errmsg_guard_test.go::TestProductionExemptRoleCallsAreInAllowedPositions` |
| 2 | `expandCmdAllowed` の原因で `PathErrorCause` を `Cause` にする | `errors_test.go::TestExpandCmdAllowed_ResolvePathCauseKeepsPath` |
| 3 | template_errors.go の 1 つの `StructuredMessage` の断片の役割を変える | `template_errors_test.go::TestTemplateErrorTypes_StructuredMessageSegments` |
| 3 | `template_expansion.go` の 1 か所のエラー書式の `Constant` の文言を 1 文字変える | `template_expansion_test.go::TestTemplateExpansionWrapSites_StructuredMessage` |
| 3 | `expandArrayPlaceholder` の workdir の拒否を外す | `template_field_constraints_test.go::TestTemplateFieldConstraints`（`workdir: ${@param} rejected` のケース） |
| 4 | `cli.FilterGroups` の存在しない名前を `Text` にする | `filter_test.go::TestFilterGroups_GroupNotFoundStructuredMessage` |
| 5 | `StructuredMessage` を持たないエラー型を加える | `config_error_guard_test.go::TestConfigErrorTypesDeclareStructuredMessage` |
| 5 | エラー型の `Level` 欄を `string` に戻す | `config_error_guard_test.go::TestConfigErrorLevelAndFieldTypesAreTyped` |
| 5 | `Ident` の引数に `Level.String()` を渡す | `config_error_guard_test.go::TestProductionDoesNotFlattenLevelOrField` |
| 5 | `Ident` の引数に `fmt.Sprintf("%s", level)` を渡す | `config_error_guard_test.go::TestProductionDoesNotFlattenLevelOrField` |
| 5 | `validateVariableName` の位置文字列の式以外（例: 検証の分岐）で `level.String()` を `errmsg.Text` に渡す | `config_error_guard_test.go::TestProductionDoesNotFlattenLevelOrField`（許可位置の自己テスト） |
| 5 | `Text` の部分にも値全体置換を免除する | `security_redaction_test.go::TestTextSegmentsAreWholeValueReplaced` |
| 5 | 日英どちらかから必須語を外す | `make verify-docs-checks`（`check_structured_message_redaction_docs.sh`） |

---

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| `errmsg.Quoted` のエスケープと断片の境目の扱いを誤り、`Error()` の文言が変わる | AC-14 違反 | 02 §3.4 の契約を `Quoted` 単体のテストで固定し、各型の `Error()` を変更前の `fmt.Sprintf` と比べる |
| `Level`・`Field` の型変更が全構築箇所に及ばず、コンパイルエラーが残る | ビルド不能 | 型を変える Phase の完了時に `make test` を通し、残りをコンパイルエラーで洗い出す |
| `Level`・`Field` が `.String()` や `fmt` の `%s`・`%v`・`%q` で平らにされ、役割を失う | AC-21 違反 | Phase 5 の値の流入の検査と自己テストで固定する。`validation.go` の位置文字列だけを許可位置にする |
| 許可位置をファイル全体に広げすぎ、誤った宣言が通る | セキュリティの後退 | 許可位置は、エラー型か設定の展開・`--groups` の検証のエラーを作るファイルに限る（02 §3.7）。役割は型ごとのテストで固定する |
| wrap guard の範囲を段階的に広げる順序を誤り、途中の Phase で green にならない | CI の失敗 | ファイル全体を範囲に加えるのはそのファイルの型をすべて構造化した段階にする（02 §8） |
| 日英の文書の内容が食い違う | AC-23 違反 | 日本語版を先に更新し、`/mktrans` で反映する。検査スクリプトの必須語で両方を機械的に確かめる |
| 到達不能な `checkGroupsExist` の分岐を消し、`nil` config の結果が変わる | 退行 | `FilterGroups` が先に `nil` を返すことを既存のテスト（`filter_test.go:121-125`）で確かめる |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1。`errmsg.Quoted` と `Field` の拡張が green）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2。errors.go と expansion.go のエラーが構造化され、文言と到達性が不変）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3。template_errors.go と template_expansion.go のエラーが構造化）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4。`cli.FilterGroups` が構造化）
- [ ] PR-5 マージ済み（対象ステップ: Phase 5。3 つのガード・保護のテスト・日英の文書・`make verify-docs-checks` が green）
- [ ] すべての AC が §7 の検証で green
- [ ] §4.4 の変異確認をすべて実施し、各コミットメッセージに記録

---

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（ガードテスト・`make` ターゲット・コミット済みスクリプト）、`manual`（PR やデプロイでの観察）を表す。テスト名は `path::TestName` で示す。本書で新設するテストは、Phase の完了時に §4.4 の変異で失敗することを確認する。

| AC | 実装タスク | 検証（種別 / アーティファクト） |
|---|---|---|
| AC-01 | Phase 2・3・5 | `static`: `internal/runner/config_error_guard_test.go::TestConfigErrorTypesDeclareStructuredMessage` と自己テスト |
| AC-02 | Phase 2・3 | `test`: `internal/runner/config/errors_test.go::TestErrorTypes_StructuredMessageSegments`・`internal/runner/config/template_errors_test.go::TestTemplateErrorTypes_StructuredMessageSegments` |
| AC-03 | Phase 2・3 | `test`: 同じ 2 つの表駆動テストの `Text` の期待値 |
| AC-04 | Phase 1・2・3・5 | `static`: `internal/runner/config_error_guard_test.go::TestConfigErrorLevelAndFieldTypesAreTyped` と自己テスト。`test`: `internal/runner/config/errors_test.go::TestLevelAndField_StringMatchesLegacyFormat` |
| AC-05 | Phase 2・3 | `test`: `internal/runner/config/errors_test.go::TestErrorTypes_StructuredMessageSegments`（原因を持つ型の `Identifier` が外側を通しても残ること） |
| AC-06 | Phase 2・3・4 | `static`: `internal/runner/wrap_guard_test.go::TestInScopeWrapsUseStructuredErrors` |
| AC-07 | Phase 2・3 | `test`: `internal/runner/config/expansion_test.go::TestExpansionWrapSites_StructuredMessage`・`internal/runner/config/template_expansion_test.go::TestTemplateExpansionWrapSites_StructuredMessage` |
| AC-08 | Phase 4 | `test`: `internal/runner/cli/filter_test.go::TestFilterGroups_GroupNotFoundStructuredMessage` |
| AC-09 | Phase 2 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupEnvImportAllowlistIdentifiersSurviveRedaction` |
| AC-10 | Phase 2 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GlobalCircularReferenceIdentifiersSurviveRedaction` |
| AC-11 | Phase 3 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_TemplateValidationIdentifiersSurviveRedaction` |
| AC-12 | Phase 4 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_GroupsFlagMissingIdentifiersSurviveRedaction` |
| AC-13 | Phase 3 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_CommandTemplateExpansionIdentifiersSurviveRedaction` |
| AC-14 | Phase 1・2・3・4 | `test`: `internal/errmsg/errmsg_test.go::TestQuoted_MatchesStrconvQuote`・`internal/runner/config/errors_test.go::TestErrorTypes_ErrorMessageMatchesLegacyFormat`・`internal/runner/config/template_errors_test.go::TestTemplateErrorTypes_ErrorMessageMatchesLegacyFormat`・`TestExpansionWrapSites_StructuredMessage`・`TestTemplateExpansionWrapSites_StructuredMessage`。`--groups` のエラーは定義済み group 名の一覧が map の反復順であり（02 §5.5）、一覧の部分は順序に依らずに確かめる（バイト一致の対象は残りの固定の部分） |
| AC-15 | Phase 2・3・4 | `test`: `TestExpansionWrapSites_StructuredMessage`・`TestTemplateExpansionWrapSites_StructuredMessage`・`filter_test.go::TestFilterGroups_GroupNotFoundStructuredMessage` の `errors.Is`・`errors.AsType` の確認 |
| AC-16 | Phase 5 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_PreExecutionConfigErrors_OutputContract`（config エラーのシナリオで `stderrDetailsBlock` により Details ブロック全体の文言を比較する。コマンド・テンプレートの展開のシナリオも含む） |
| AC-17 | Phase 5 | `test`: `cmd/runner/integration_pre_execution_error_test.go::TestIntegration_PreExecutionConfigErrors_OutputContract`（config エラーのシナリオで `run.payloads` の件数・Scope・フィールドの構成と、`jsonLogRecords` で読む `message_type`・`error_type` を比較する。コマンド・テンプレートの展開のシナリオも含む） |
| AC-18 | Phase 5 | `test`: `internal/runner/config/security_redaction_test.go::TestTextSegmentsAreWholeValueReplaced` |
| AC-19 | Phase 5 | `test`: `internal/runner/config/security_redaction_test.go::TestNonIdentifierSegmentsMaskValueFormats` |
| AC-20 | Phase 2・3・4・5 | `static`: `internal/errmsg/errmsg_guard_test.go::TestProductionExemptRoleCallsAreInAllowedPositions`・`TestProductionPartFieldsAreUnexportedAndUnbuiltOutsideErrmsg` と `internal/runner/wrap_guard_test.go::TestInScopeWrapsUseStructuredErrors`・`TestInScopeErrorTypesDeclareStructuredMessage` と各自己テスト |
| AC-21 | Phase 5 | `static`: `internal/runner/config_error_guard_test.go::TestProductionDoesNotFlattenLevelOrField`・`TestConfigErrorLevelAndFieldTypesAreTyped` と各自己テスト |
| AC-22 | Phase 5 | `static`: 3 つの新しいガードと更新したガードの各自己テスト（§4.3） |
| AC-23 | Phase 5 | `static`: `make verify-docs-checks`（`scripts/verification/check_structured_message_redaction_docs.sh` と `..._selftest.sh`）。`manual`: 日英の内容を突き合わせてレビューする |
| AC-24 | 各 Phase | `static`: 各 Phase の `make fmt`（Go を変更した場合）・`make test`・`make lint` |

---

## 8. 横断検索チェックリスト

`make test`・`make lint` が検出できない残存参照・用語の整合だけを挙げる。§7 の表と重複する項目は置かない。

- [ ] 削除・改名したガードの一覧に載る名前（`inScopeExpansionFile`・`inScopeExpansionExclusions`・`expansionExcludedFunctions`）が、コメントを含めて本番コード・テストに残っていないこと（`rg -n "inScopeExpansion|expansionExcludedFunctions" internal cmd`）。
- [ ] `docs/tasks/0178_structured_error_message_redaction/03_detailed_specification.md` の `expansion.go` の除外と config の `*...Detail` 型を範囲外とする記述に、本タスクで範囲に入ったことを示す注記があること。
- [ ] `docs/translation_glossary.md` に、Phase 5 で新しく使う用語の対訳があること。既存の「構造化メッセージ」「断片」「役割」「値全体置換」「識別子」「免除」で足りる見込みである。`/mktrans` の結果を確認する。

---

## 9. Success Criteria

- **機能**: AC-01〜AC-24 を検証するテスト・ガード・`make` ターゲットが green。config のエラー型と対象のエラー書式が構造化メッセージとして運ばれ、`Identifier` の部分が値ベース redaction を免除される。
- **品質**: 各 Phase の `make fmt`・`make test`・`make lint` が green。§4.4 の変異確認をすべて実施し記録済み。
- **セキュリティ**: 名前の検証で拒否された名前・生の設定値・パラメータの値に由来しうる文字列は `Text` のままで、変更前と同じ保護を受ける。`Path` の部分は key=value 置換と値形式の検出を受け、値全体置換を受けない。保護の境界（名前の位置の秘密・パスに展開された秘密・付随的な保護の減少）は 02 §5.1〜§5.2 のとおり受け入れる。
- **一貫性**: 対象のエラーの `Error()`・stderr の `Details:` の文言が変更前と同じである。通知の件数・`message_type`・`error_type`・Scope・Slack のフィールド構成が変わらない。`errors.Is`・`errors.AsType` の到達性が変わらない。
- **文書**: security-architecture の日英に、新しい免除の対象と `Text` の境界が記載され、`make verify-docs-checks` が green である。

---

## 10. 次のステップ

- 本書が承認されたら、Phase 1（PR-1）から順に実装を開始する。
- 実装の完了後は、§7 の全 AC と §4.4 の変異確認の記録をレビューする。
- 設定の読み込み時の検証のエラー書式（`validation.go`・`loader.go`・`template_loader.go`）は本タスクの対象外のままである。構造化するときは、wrap guard の範囲にそのファイルを加える。
