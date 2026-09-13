# 実装計画書: 識別子の型宣言と値ベース redaction からの免除

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-13 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連文書

- [01_requirements.md](01_requirements.md) — 受け入れ基準（AC-01〜AC-19）の定義元
- [02_architecture.md](02_architecture.md) — 設計の定義元。本書は設計を再掲せず参照する
- [requirements_process.md](../../dev/developer_guide/requirements_process.md) — 本書の必須節と AC 追跡の規約
- [test_organization.md](../../dev/developer_guide/test_organization.md) — テストヘルパーの配置規約
- [security-architecture.ja.md](../../dev/architecture_design/security-architecture.ja.md) / [security-architecture.md](../../dev/architecture_design/security-architecture.md) — Phase 5 で更新する
- [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) / [security-risk-assessment.md](../../user/security-risk-assessment.md) — Phase 5 で更新する
- [translation_glossary.md](../../translation_glossary.md) — 新用語の英語訳を確認する

## 1. 実装概要

### 1.1 目的

group 名・コマンド名を識別子として型で宣言し、宣言された値だけを値ベース redaction の
3 層（key=value 置換・値形式検出・値まるごと判定）から免除する。自由文の redaction は
弱めず、下流ハンドラには string として正規化して渡す。設計の詳細は 02_architecture.md を
参照する。

### 1.2 実装原則

1. 02_architecture.md の設計に従い、本書では設計判断を作り直さない。設計に無い判断が
   必要になった場合は、実装を止めて 02_architecture.md を先に改訂する。
2. `internal/identifier` は内部パッケージを import しない leaf パッケージとする。
   `Identifier` の構築は、定義パッケージ外ではコンパイラが、定義パッケージ内では
   `identifier_guard_test.go` の AST 制限が `NewIdentifier` に限定する
   （02_architecture.md §3.1、§7.3）。
3. Go のコメント・識別子・文字列リテラルはすべて英語で書く。本書の説明文だけが日本語である。
4. 各 Phase の終わりに `make fmt`（Go を変更した場合）、`make test`、`make lint` を通す。
5. `DefaultSensitivePatterns`・`DefaultKeyValuePatterns`・`ValueDetector` のパターン集合と、
   `internal/redaction/sensitive_patterns.go`・`internal/redaction/value_detector.go` は
   変更しない（AC-12）。
6. 宣言サイトの追加・変更・削除は、`identifier_guard_test.go` の目録（Phase 4）と、
   その宣言に固定する型・挙動アサーションを同じコミットに含める。初回実装では Phase 4
   （4.1〜4.5）を 1 コミットにまとめ、宣言・目録・アサーションを同時に導入する。これにより
   rollback は Phase 4 のコミットの revert を単位にできる（02_architecture.md §5.2）。
   以後の宣言サイトの追加・削除も同じコミット規約に従う。
7. 既存の実装・テスト・ヘルパーを優先して再利用する。とくに構文木の走査は
   `internal/testutil/identitymutationguard` の既存ヘルパーを使い、走査対象ファイルの定義を
   複製しない。

### 1.3 既存コード調査結果

本書の行番号は、指定がない限り commit `b2d2f744` 時点で確認した。02_architecture.md が
既存挙動を検証した commit `88624849` から HEAD まで `internal/` と `cmd/` に差分は無い
（`git diff --stat 88624849..HEAD -- internal cmd` が空であることを確認済み）。既存コードに
手を入れる必要がない領域は省いてある。

#### 宣言サイトの全数

group 名・コマンド名をログ属性値として書く production の宣言サイトは、02_architecture.md
§3.4 の表のとおり **12 ファイル・43 サイト**である。01_requirements.md のスコープ対象 3 が
記す件数と一致する。内訳は `internal/common` が 4、`internal/runner` が 18、
`internal/runner/config`・`internal/runner/base`・`internal/verification` が残りで、`cmd/`
配下には宣言サイトが無い。

目録の出典は 02_architecture.md §3.4 の表とする。本書では 43 行を再掲しない
（「設計を再掲せず参照する」の規約に従う）。Phase 4 の guard は、この表から導いた目録と
走査結果を双方向に照合する。

同じキー `"command"`・`"group"`・`"name"` には、宣言型（`identifier.Identifier`。
02_architecture.md の用語表で定義）にしない値（展開済みコマンド行・解決済みパス・
OS グループ名・一時ファイル名）が混在する。`internal/runner/base/executor/executor.go`
は同一関数内の行 247 に OS グループ名、行 254 にコマンド名を書く。値の式で判断する
（02_architecture.md §3.5 の表が全件を列挙する）。

#### 免除経路の挿入点

02_architecture.md §3.2 の設計を `b2d2f744` のコードで確認した。`internal/redaction/redactor.go`
の挿入点と、判定を置くべき位置は次のとおりである。

| 挿入点 | 現在の位置 | 判定の直後・直前 |
|---|---|---|
| `Config.RedactLogAttribute` | `redactor.go:301` | キー名判定（`:312`）の後、string 判定（`:317`）の前 |
| `RedactingHandler.redactLogAttributeWithContext` | `redactor.go:764` | キー名判定（`:769`）の後、`switch value.Kind()`（`:774`）の前 |
| `RedactingHandler.processSlice` | `redactor.go:1264` | 要素の `slog.LogValuer` 型アサーション（`:1330`）の前。免除要素は `Name()` の string にして `processedElements` へ append する |

`processLogValuer` が解決した値が宣言型の場合は、再帰先の `redactLogAttributeWithContext` の
先頭判定で免除される（再帰は `redactor.go:994`）。`processMap`・`processStruct` は値ごとに
`redactLogAttributeWithContext` へ再帰する（`:1127`・`:1213`）ため追加の挿入点を要しない。

`Config.RedactLogAttribute` を production で呼ぶのは group 再帰の `redactor.go:336` だけで、
production に外部の呼び出し元は無い。`rg -n "\.RedactLogAttribute\(" internal cmd -g '*.go'`
の一致は `redactor_test.go` と `redactor.go:336` のみである（`b2d2f744` で確認）。
この経路でも宣言型を string へ正規化する（02_architecture.md §3.2）。

#### 復号経路と `slog.Value.Resolve()` の挙動

`NotificationContext.LogValue` は `internal/common/notification_context.go:78`、
`decodeNotificationContextParts` は同ファイル `:136` にある。本環境の Go 1.26.3 で
`slog.GroupValue(...).Resolve()` がトップレベルの `LogValuer` だけを解決し、グループの
下位値は `KindLogValuer` のまま残ることを確認した（検証用の一時プログラムを実行し、
`Resolve()` 後の下位値の種別が `KindLogValuer`、`Any()` が返す値の型が
`identifier.Identifier` のままであることを確認）。したがって復号側が下位値の宣言型を
直接読む必要がある（02_architecture.md §3.3）。

テスト補助の `internal/testutil/handlers.go:77` の `LogRecorder` は `a.Value.Any()` を
保存し `Resolve` しないため、宣言した値は `identifier.Identifier` として捕捉される。
`AssertAttrs`（同ファイル `:202`）は `EqualValues` で比較する。この 2 点が、後述の
「既存テストの更新が必要な箇所」の根拠である。

#### `SecurityLogger` の引数型

`internal/logging/security.go` の 4 メソッド（`LogUnlimitedExecution` `:22`・
`LogLongRunningProcess` `:31`・`LogTimeoutExceeded` `:40`・`LogTimeoutConfiguration` `:49`）は
引数 `cmdName string` を取る。production の呼び出し元は `internal/runner/group_executor.go:537`
（`LogUnlimitedExecution`）と `:585`（`LogTimeoutExceeded`）の 2 つだけで、
`LogLongRunningProcess` と `LogTimeoutConfiguration` に production の呼び出し元は無い
（`rg -n "LogUnlimitedExecution|LogLongRunningProcess|LogTimeoutExceeded|LogTimeoutConfiguration"
internal/ cmd/ -g '*.go'` で確認）。`buildCommandDebugLogArgs`
（`internal/runner/group_executor.go:553`）は `cmdName string` を取り、呼び出し元は `:612`
の 1 箇所である。4 メソッドの引数を `identifier.Identifier` にすると、ヘルパー内部で
宣言する形と違い、呼び出し元が `identifier.NewIdentifier(…)` を書くため guard の
引数式照合の対象になる（02_architecture.md §3.4）。ただし production の呼び出し元が
無い `LogLongRunningProcess`・`LogTimeoutConfiguration` は guard の目録に宣言が現れない
ため、メソッドが内部で `cmdName.Name()`（plain string）へ退行していないことを、生値を
捕捉する Phase 4.2 の raw 型アサーションで固定する。

#### 宣言型を付けてはいけないサイト

02_architecture.md §3.5 の表の全件を確認した。とくに次の 3 点は誤宣言しやすい。

- `internal/runner/base/executor/executor.go:191,198,247,284,314,327` と
  `command_lifecycle.go:301,572,585,661,701,705,738` の `command` は展開済みコマンド行で
  あり、plain string のままとする。
- `internal/runner/base/executor/executor.go:215,247,254,286` の `group` は OS グループ名で
  あり、コマンド名（`:254` の `command`）と同じ関数に同居する。
- `internal/runner/resource/dryrun_manager.go:236,721` は dry-run 解析マップのキーであり
  ログ属性ではない。`internal/safefileio/safe_file_linux.go:241` の `name` は一時ファイル名で
  あり、ログ属性ではあるが識別子ではない。いずれも変更しない。

#### 既存テストの更新が必要な箇所

`LogRecorder` 系の生値比較と、`SecurityLogger`・`buildCommandDebugLogArgs` の呼び出しが
更新対象である。行番号は `b2d2f744` 時点。

| テスト | 箇所 | 宣言されるキー |
|---|---|---|
| `internal/runner/group_executor_test.go::TestCreateCommandContext_UnlimitedTimeout_SecurityLogging` | `:2473`・`:2486`（期待値） | `command` |
| `internal/runner/group_executor_test.go::TestExecuteGroup_TimeoutExceeded_SecurityLogging` | `:2598`（期待値） | `command` |
| `internal/runner/group_executor_test.go::TestExecuteGroup_MultipleCommands_TimeoutLogging` | `:2674`（期待値） | `command` |
| `internal/runner/group_executor_test.go::TestCommandDebugLogArgs_StdoutTruncation` | `:2880`（呼び出し引数） | `command` |
| `internal/runner/group_executor_timeout_test.go::TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded` | `:94`（`Attrs["command"]` 比較） | `command` |
| `internal/runner/runner_test.go::TestCommandResult_LogValue` | `:2010`・`:2027`・`:2044`（期待値マップ）と `:2062-2070`（`KindString`／`KindInt64` だけを分岐し、`KindLogValuer` が map から落ちる） | `name` |
| `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_ReportsNativeRootOutcome` | `:812`（`AssertAttrs`） | `command` |
| `internal/runner/base/privilege/unix_privilege_test.go::TestLogElevationOutcome` | `:869`（`AssertAttrs`） | `command` |
| `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution` | `:117`（`AssertAttrs`） | `command_name` |
| `internal/logging/security_test.go::TestSecurityLogger_LogMethods` | `:31,45,59,73,86`（呼び出し引数。加えて §4.2 の raw 型アサーションと `RedactingHandler` 通過） | `command` |
| `internal/common/notification_context_test.go` | `:19-25`（`groupAttr`／`commandAttr` ヘルパー）、`:38`、`:161` | `group`／`command` |

更新不要と確認したもの:

- `internal/common/logschema_test.go:72` は `Value.String()` を比較するため、
  `Identifier` の `String()` 実装によりそのまま通る。
- `internal/runner/resource/audit_wiring_test.go` は JSON ハンドラの出力を解析するため、
  免除経路の正規化によりそのまま通る。ただし AC-06・AC-09 の検証のために
  `internal/runner/integration_command_results_test.go::TestCommandResults_E2E_Integration`
  は redaction を発火させる名前を足して拡張する（Phase 4.4）。
- `internal/runner/base/executor/executor_logging_test.go:63,78,113` と
  `internal/runner/base/security/logging_security_test.go` は展開済みコマンド行だけを
  比較するため影響しない。

実装時には、この表を起点に全 `*_test.go` を検索し、宣言型になる値を string と比較する
期待値を洗い出す（02_architecture.md §7.4）。

#### 構文木ガードの再利用先

`internal/testutil/identitymutationguard` の既存ヘルパーを使う。

- `ProductionGoFilesInRepo(t)`（`helpers.go:207`）は `internal/` と `cmd/` の production
  `.go` を列挙する。`//go:build test || performance` のように `test` タグを必須としない
  制約のファイル（`internal/testutil/`、`internal/common/testutil/`、
  `internal/runner/base/executor/testutil/`、`internal/runner/resource/testutil/`、
  `internal/runner/resource/test_helpers.go` など）も production として含む。テスト期待値を
  `identifier.NewIdentifier` で組み立てるコードは `_test.go` に置き、これら `test` タグを
  必須としない非 `_test.go` のファイルには置かない。対象集合の定義は
  `ProductionGoFilesInRepo` に委ね、列挙を本書へ写さない。
- `Options.Extra`（`helpers.go:101`）は修飾形と呼び出し元パッケージ内の非修飾形の両方を
  追跡できる。修飾形の値参照（エイリアス）は `ValueRef` として報告される
  （`helpers.go:111`）が、非修飾形の値参照は報告されない（`helpers.go:341`）。後者は
  guard 自身が AST を走査して拒否する。非修飾形の追跡は宣言パッケージを解決できず名前
  だけで一致するため、`ImportPath` を空にする指定は `internal/identifier` 配下のファイル
  に限定して適用し、他パッケージの同名関数を `NewIdentifier` と誤認しない。修飾形は
  リポジトリ全体に適用する。
- `ResolveLocalImports`（`helpers.go:378`）は import の別名解決に再利用する。
- `ProductionGoFilesInRepo` が返すファイルのうち `internal/identifier` の production
  ファイルも guard の走査対象に含め、`name` フィールドを設定する複合リテラルとフィールド
  代入が `NewIdentifier` の本体内だけに現れることを AST で検証する（02_architecture.md
  §3.1）。複合リテラルは綴られた型名に依存せず、パッケージ内の型エイリアス
  （`type Alias = Identifier`）と定義型（`type Alias Identifier`）の宣言を解決し、
  `Identifier` に由来する型なら `name` を設定するリテラルとして拒否する。これにより、
  `type Alias = Identifier; func FromString(s string) Identifier { return Alias{name: s} }`
  のように綴りを変えただけの構築経路も落ちる。定義パッケージ外を守るコンパイラでは
  塞げないパッケージ内の別コンストラクタ・代入を拒否する。

#### 文書の該当箇所

| 文書 | 該当箇所 | Phase 5 の扱い |
|---|---|---|
| `docs/dev/architecture_design/security-architecture.ja.md` | `### 9. セキュアログと機密データ保護`（`:574` 以降、とくに第 2 層の説明 `:616-635`） | 識別子の型宣言による免除を追記する（AC-15）。kill switch が無いことと、導入コミットの revert による rollback 手順、自由文 error の残余リスクを記す |
| `docs/dev/architecture_design/security-architecture.md` | 対応する `### 9. Secure Logging and Sensitive Data Protection`（`:578` 以降） | 日本語版を `/mktrans` で反映する |
| `docs/user/security-risk-assessment.ja.md` | `### 1. 拡張ログ・監査システム` の「限界」（`:295-299`） | 識別子を redact しない帰結を追記する（AC-14、AC-16） |
| `docs/user/security-risk-assessment.md` | 対応する Limitations（`:299-301`） | 日本語版を `/mktrans` で反映する |

現在の語の出現状況（`b2d2f744` 時点で実行済み）:
`security-architecture.ja.md` に `識別子`・`免除` は 0 件、`security-architecture.md` に
`identifier`・`exempt` は 0 件。`security-risk-assessment.ja.md` の `識別子` は 1 件
（`:297` の「識別子内境界」で、AC-14 の内容ではない）、`.md` の `identifier` も 1 件
（`:301` の同じ箇所）。Task 0172 の承認済み文書は変更しない。

#### 文書内容の検証スクリプト

文書でしか確認できない AC を、文書に書くだけの `rg` コマンドで検証すると、そのコマンドは
実行されないまま陳腐化する。そこで Phase 5 で `scripts/verification/check_identifier_exemption_docs.sh` を
追加し、AC-13〜AC-17 の検証をこの 1 ファイルに集約する。追加したスクリプトは `Makefile` の
`verify-docs`（および `verify-docs-full`）ターゲットから実行し、非ゼロ終了を make の失敗と
して伝播させる。`run_all.sh` は検査結果にかかわらず終了コード 0 を返すため、ターゲット側で
直接実行する。これにより `make verify-docs` が AC-13〜AC-17 の完了ゲートになる。スクリプトは
語ごとに独立した終了コードで判定し、次の組を要求する（AND を要素の並びで表現し、いずれか
1 語の一致で全体を緑にしない）。

| 対象ファイル | 要求する語 |
|---|---|
| `docs/dev/architecture_design/security-architecture.ja.md` | `識別子`、`免除`、`NewIdentifier` |
| `docs/dev/architecture_design/security-architecture.md` | `identifier`、`exempt`、`NewIdentifier` |
| `docs/user/security-risk-assessment.ja.md` | `識別子`、`免除` |
| `docs/user/security-risk-assessment.md` | `identifier`、`exempt` |
| `docs/tasks/0173_identifier_redaction_exemption/02_architecture.md` | `残余リスク`、`record.Message`（AC-13 の記録が残っていること） |
| `docs/tasks/0173_identifier_redaction_exemption/01_requirements.md` | `0172`、`置き換え`（AC-17 の参照） |

`identifier`／`exempt` の検査は語幹で照合する（`exemption`・`exempted` を含む）。
`security-risk-assessment` の 2 文書は現在 `識別子`／`identifier` が 1 件だけ別文脈で
一致するため、`免除`／`exempt` の追加が無ければスクリプトは失敗する。実装前の現時点でも
同スクリプトは失敗する（新しい語が未記載のため）。語だけで内容の正しさまでは保証できない
ため、Phase 5 では追加した段落を読んで、識別子が免除されることと、名前に機密を書いた場合の
帰結が書かれていることを確認する（`manual`）。

#### `internal/identifier` の新規性

`internal/identifier` を参照する既存コードは無く、同名パッケージも存在しない。新設する
leaf パッケージは `log/slog` だけを import し、02_architecture.md §2.1 の追加依存辺は
いずれも非循環である。

### 1.4 テストヘルパーの方針

`docs/dev/developer_guide/test_organization.md` に照らし、新しいテストヘルパーファイルは
必要としない。

- 構文木ガードは既存の `internal/testutil/identitymutationguard` を再利用し、目録の照合
  ロジックとカタログは `internal/identifier/identifier_guard_test.go`（`//go:build test` を
  持つ `_test.go`）に置く。新しい `testutil/` パッケージや `test_helpers.go` は作らない。
- 免除のテストは `internal/redaction/redactor_test.go`（package `redaction` の内部テスト）に
  追加する。テスト専用の小さなヘルパーが要る場合も同ファイル内に置く。
- `identifier.NewIdentifier` で期待値を組み立てるコードは `_test.go` に置く。
  `internal/testutil/`・`internal/common/testutil/`・`internal/runner/resource/testutil/` など、
  `test` タグを必須としないビルド制約を持ち guard が production として走査する非 `_test.go`
  のファイルには置かない。対象集合の定義は `ProductionGoFilesInRepo` に委ねる。

## 2. 実装ステップ

### Phase 1: 宣言型 `internal/identifier` の新設

**対象ファイル**: `internal/identifier/identifier.go`（新規）、
`internal/identifier/identifier_test.go`（新規）

- [ ] `internal/identifier/identifier.go` に leaf パッケージを追加する。`Identifier`
      （非公開フィールド `name string`）、`NewIdentifier(name string) Identifier`、
      `Name() string`、`String() string`、`LogValue() slog.Value`（string を返す）を定義し、
      `var _ slog.LogValuer = Identifier{}` のコンパイル時ガードを置く。02_architecture.md
      §3.1 のコード片は形を示すためのものである。コード片のコメントも含め、実装する
      コメント・識別子・文字列リテラルはすべて英語で書く（`.claude/commands/_context.md`
      の Source-language rule）。
- [ ] `internal/identifier/identifier_test.go` に、`NewIdentifier(name).Name()` と
      `.String()` が名前を返すこと、`LogValue()` が `KindString` で名前を返すこと、ゼロ値
      `Identifier{}` が空名として扱われることを検証するテーブルテストを書く。
- [ ] `internal/identifier` が内部パッケージを import しないことを確認する。`go list
      -deps ./internal/identifier` は対象パッケージ自身を必ず含むため、返り値をそのまま
      「標準ライブラリだけ」と比較しない。`go list -f '{{join .Imports "\n"}}'
      ./internal/identifier` で直接 import を列挙し、または `-deps` の出力から対象
      パッケージを除外して、標準ライブラリ以外が無いこと（leaf パッケージの条件）を
      確認する。
- [ ] AC-19 の確認: `LogValue` を名前ではなく固定文字列へ変えるなど、テストが検証対象と
      する挙動を一時的に壊して `internal/identifier/identifier_test.go` が失敗することを
      確認し、結果をコミットメッセージに記す。

**完了条件**: `make fmt`・`make test`・`make lint` が通り、`Identifier` の単体テストが通る。

### Phase 2: `internal/redaction` への免除経路の追加

**対象ファイル**: `internal/redaction/redactor.go`、`internal/redaction/redactor_test.go`

- [ ] `redactor.go` に、`slog.Value` が宣言型（`identifier.Identifier` または非 nil の
      `*identifier.Identifier`）かを判定する非公開ヘルパーを 1 つ追加する。値の型だけで判定し、
      キー名・値の内容・長さは見ない。型付き nil の `*identifier.Identifier` は false を返す
      （02_architecture.md §3.2）。
- [ ] `Config.RedactLogAttribute` のキー名判定の後、string／group 判定の前に、宣言型を
      `slog.StringValue(name)` へ正規化する分岐を追加する。
- [ ] `RedactingHandler.redactLogAttributeWithContext` のキー名判定の後、
      `switch value.Kind()` の前に、宣言型を `slog.StringValue(name)` にして返す分岐を
      追加する。キー名判定を先に置く fail-closed の順序を守る。
- [ ] `RedactingHandler.processSlice` の要素ループで、`slog.LogValuer` 型アサーションの前に
      ヘルパーで要素の型を見る。免除した要素は `Name()` の string にして
      `processedElements` へ append する。
- [ ] `redactor_test.go` に次のテストを追加する。免除の入力集合は 02_architecture.md §7.1・
      §7.3 が定義する。各テストは、免除側で名前がそのまま現れることに加え、下流へ渡った
      値の種別が string であることを検証する。
  - `TestRedactLogAttribute_IdentifierExemption`（Config 実装。group 再帰内の宣言型が string
    へ正規化され、同じ group 内の plain string は従来どおり redact される）
  - `TestRedactingHandler_IdentifierExemption`（3 層それぞれの一致形を、同じ文字列を
    plain string に載せた対照と対にして検証する。宣言型の値はそのまま現れ、plain string の
    対照は redact される）
  - `TestRedactingHandler_IdentifierInGroupAttribute`（手で組んだ group の下位値が免除される）
  - `TestRedactingHandler_PlainStringIsStillRedacted`（前項までの対照では網羅できない自由文
    （`token=…`・`Bearer …`・message・error 属性）を plain string で固定する。同じ `command`
    キーにコマンド名（宣言型）と展開済みコマンド行（plain string）を載せ、後者だけが
    redact されることを含む）
  - `TestRedactingHandler_IdentifierSliceElements`（`[]identifier.Identifier` と
    `[]*identifier.Identifier` の非 nil 要素。下流ハンドラの描画に名前が string として現れ、
    `[{}]` にならないことを検証する）
  - `TestRedactingHandler_TypedNilIdentifierFailsClosed`（型付き nil がパニックせず
    `RedactionFailurePlaceholder` になる）
  - `TestRedactingHandler_SensitiveKeyMaskPrecedesExemption`（機密キーに宣言型を載せても
    値は `[REDACTED]` のまま）
- [ ] `redactor_test.go` に `TestDefaultPatternSets_AreUnchanged` を追加し、AC-12 の集合
      不変を検出可能にする。`DefaultKeyValuePatterns()` の `Literal` と `Kind` の集合と件数、
      `DefaultSensitivePatterns()` の `AllowedEnvVars` の集合と `combinedCredentialPattern`・
      `combinedEnvVarPattern` の正規表現ソース、`valueDetectorPatterns` の各正規表現ソースを
      固定し、追加・削除・置換のいずれでも失敗させる。
- [ ] `internal/redaction/sensitive_patterns.go` と `internal/redaction/value_detector.go` を
      変更しない（AC-12）。
- [ ] AC-19 の確認: 挿入点ごとに、外したときに失敗するテストを対応づけて確認する。
      `Config.RedactLogAttribute` を外すと `TestRedactLogAttribute_IdentifierExemption`、
      `redactLogAttributeWithContext` を外すと `TestRedactingHandler_IdentifierExemption`、
      `processSlice` を外すと `TestRedactingHandler_IdentifierSliceElements` が失敗する。
      キー名判定より先に免除すると `TestRedactingHandler_SensitiveKeyMaskPrecedesExemption`
      が失敗する。破壊と復元の対象・テスト名をコミットメッセージに記す。

**完了条件**: 追加テストが通り、既存のパターンテストが変更なく通る。

### Phase 3: `NotificationContext` の復号受理

**対象ファイル**: `internal/common/notification_context.go`、
`internal/common/notification_context_test.go`

- [ ] `decodeNotificationContextParts`（`notification_context.go:136`）の `group`・`command` を、
      `KindString` に加えて `value.Any()` が `identifier.Identifier` である値も受けるように
      する。汎用の `Value.Resolve` は使わず、`scope` は従来どおり `KindString` だけを受ける。
      どちらでもない値（`slog.Int`、宣言型以外の `LogValuer`）は従来どおり拒否する。
      この Phase では `NotificationContext.LogValue` の符号化を変えない。宣言を伴う符号化の
      変更は、guard・目録・アサーションを同じコミットに含めるため Phase 4 で行う
      （02_architecture.md §3.4、§5.2、§8.1）。
- [ ] `TestDecodeNotificationContext_Validity` に、`group`／`command` の下位値が
      `identifier.Identifier` である行を足す。期待値は
      `slog.Any(NotificationContextAttrs.Group, identifier.NewIdentifier(…))` の形でその場に
      組み立て、正規化後の string 経路（既存の `groupAttr`／`commandAttr`）と生の宣言型経路の
      両方を検証する。`TestNotificationContext_LogValueEncoding` は符号化が変わる Phase 4 まで
      変更しない。
- [ ] `TestDecodeNotificationContext_Validity` に、`group`／`command` の下位値が
      `identifier.Identifier` 以外の `LogValuer` である行を足し、`ErrInvalidNotificationContext`
      になることを検証する。これは設計が汎用の `Resolve` を採らない根拠（02_architecture.md
      §3.3）を固定する行であり、文字列を受ける任意の `LogValuer` を通す実装を落とす。
- [ ] AC-19 の確認: 復号の宣言型受理を外すと `TestDecodeNotificationContext_Validity` の
      宣言型行が失敗することを確認し、コミットメッセージに記す。

**完了条件**: `internal/common`・`internal/logging`・`internal/redaction` のテストが通る。

### Phase 4: 宣言サイトの置き換えと構文木ガード

#### 4.1 `CommandResult` / `CommandResults`

**対象ファイル**: `internal/common/logschema.go`、`internal/common/logschema_test.go`

- [ ] `CommandResult.LogValue`（`logschema.go:118`）の `name` を
      `slog.Any(LogFieldName, identifier.NewIdentifier(c.Name))` にする。
- [ ] `CommandResults.LogValue`（`logschema.go:147`）の各 `cmd_%d` グループの `name` も同様に
      する。`CommandResultFields.Name` の型は string のまま変えない。
- [ ] `logschema_test.go` に、`name` の `Kind` が `KindLogValuer` で `Value.Any()` が
      `identifier.Identifier` であること、`Value.String()` が名前を返すことを検証する行を
      足す。

#### 4.2 `SecurityLogger` の引数型

**対象ファイル**: `internal/logging/security.go`、`internal/runner/group_executor.go`、
`internal/logging/security_test.go`、`internal/runner/group_executor_test.go`

- [ ] `SecurityLogger` の 4 メソッドの引数 `cmdName string` を
      `cmdName identifier.Identifier` に変え、`internal/identifier` を import する。
- [ ] `buildCommandDebugLogArgs`（`group_executor.go:553`）の引数を
      `cmdName identifier.Identifier` に変える。
- [ ] 呼び出し元で宣言する。`group_executor.go:537`（`LogUnlimitedExecution`）、`:585`
      （`LogTimeoutExceeded`）、`:612`（`buildCommandDebugLogArgs`）を
      `identifier.NewIdentifier(cmd.Name())` に置き換える。
- [ ] `security_test.go` の 5 箇所の呼び出しと `group_executor_test.go:2880` の呼び出しで
      `identifier.NewIdentifier(…)` を渡すよう更新する。
- [ ] `security_test.go::TestSecurityLogger_LogMethods` は呼び出し引数の更新に留めず、
      `LogRecorder`（`internal/testutil/handlers.go`）で生値を捕捉し、4 メソッドが出力する
      `command` 属性の `Value.Any()` が `identifier.Identifier` であることを
      `RecordSnapshot.AssertAttrs` で固定する（`LogRecorder` は `Resolve` しないため、
      両形式を同じ string に解決する JSON ハンドラでは区別できない生の型を比較できる）。
      `LogLongRunningProcess` と `LogTimeoutConfiguration` の両分岐（無制限・有限）には
      production の呼び出し元が無く、guard の目録も呼び出し元に書かれた宣言しか見ないため、
      このアサーションが無いとメソッド内部で `cmdName.Name()` へ退行しても検出できない。
      併せて redaction を発火させる名前（`rotate_api_key` など）で 4 メソッドを
      `RedactingHandler` に通し、名前が redact されずに現れることも検証する。

#### 4.3 残りの宣言サイト

**対象ファイル**: `internal/common/notification_context.go`、
`internal/runner/group_executor.go`、`internal/runner/runner.go`、
`internal/runner/config/expansion.go`、`internal/runner/base/executor/executor.go`、
`internal/runner/base/executor/tempdir_manager.go`、`internal/runner/base/privilege/unix.go`、
`internal/runner/base/audit/logger.go`、`internal/runner/resource/normal_manager.go`、
`internal/runner/resource/dryrun_manager.go`、`internal/verification/manager.go`

- [ ] `NotificationContext.LogValue`（`notification_context.go:78`）の `group`・`command` の
      下位値を `slog.String` から `slog.Any` + `identifier.NewIdentifier` に変える。
      `command` は空でないときだけ載せる現行の出力条件を守る（02_architecture.md §3.3）。
      この符号化は Phase 4.5 の guard が守る宣言サイトの 1 つであり、guard の目録・
      アサーションと同じコミットに含める（02_architecture.md §3.4、§5.2）。この符号化の
      AC-19 mutation（下位値を一時的に `slog.String` へ戻す）は Phase 4.4 に記す。
- [ ] 02_architecture.md §3.4 の表の各行を `slog.Any(key, identifier.NewIdentifier(値の式))`
      または可変長引数への `identifier.NewIdentifier(値の式)` へ置き換える。`slog.Attr` を
      取る位置は `slog.Any` を使う。`internal/identifier` を各パッケージで import する。
- [ ] `internal/runner/base/executor/executor.go` は行 247 の OS グループ名
      （`cmd.RunAsGroup()`）と行 254 のコマンド名（`cmd.Name()`）を混同しない。行 191・198・
      314・327 の展開済みコマンド行を宣言型にしない。
- [ ] `internal/verification/manager.go:279-280` は `group` だけを宣言型にし、同じ呼び出しの
      `command`（`command.ExpandedCmd`）は plain string のまま残す。
- [ ] `internal/runner/resource/normal_manager.go:143` はキー `command_path` の値
      （`group.Name`）だけを宣言型にする。キー名の是正はしない（02_architecture.md §5.2）。
- [ ] 02_architecture.md §3.5 の表の行を変更しない。

#### 4.4 既存テストの更新と統合テストの追加

**対象ファイル**: §1.3「既存テストの更新が必要な箇所」の表に挙げたファイル、
`internal/runner/integration_command_results_test.go`、`internal/logging/slack_handler_test.go`、
`internal/runner/base/audit/logger_test.go`、`internal/common/notification_context_test.go`、
`internal/logging/notification_context_test.go`

- [ ] `group_executor_test.go` の 4 つの期待値（`:2473`・`:2486`・`:2598`・`:2674`）と
      呼び出し 1 箇所（`:2880`）を `identifier.NewIdentifier(…)` を用いる形へ更新する。
      `TestCommandDebugLogArgs_StdoutTruncation` は呼び出し側の更新だけでは
      `buildCommandDebugLogArgs` が型付きの値を返すことを確認できないため、戻り値の
      `command` 要素が `identifier.Identifier` であることもアサートする。
- [ ] AC-19 の確認: `buildCommandDebugLogArgs`（`group_executor.go`）の `command` に
      一時的に `cmdName.Name()`（plain string）を載せ、
      `TestCommandDebugLogArgs_StdoutTruncation` が失敗することを確認して復元する。
      破壊と復元の対象・テスト名をコミットメッセージに記す。
- [ ] `group_executor_timeout_test.go:94` の比較を
      `identifier.NewIdentifier("sleeps-past-its-timeout")` との比較へ更新する。
- [ ] `runner_test.go::TestCommandResult_LogValue` の期待値マップ（`:2010`・`:2027`・`:2044`）
      と属性変換（`:2062-2070`）を更新し、`KindLogValuer` の分岐で `name` が
      `identifier.Identifier` であることを検証する。
- [ ] `unix_privilege_test.go:812`・`:869` の `command` 期待値を
      `identifier.NewIdentifier("test-command")` にする。
- [ ] `audit/logger_test.go:117` の `command_name` 期待値を
      `identifier.NewIdentifier(tt.cmd.Name())` にする。
- [ ] `security_test.go` の呼び出しと §4.2 の raw 型アサーションを追加する。
- [ ] `internal/common/notification_context_test.go` の `groupAttr`／`commandAttr` ヘルパーを
      `identifier.NewIdentifier` で構築する形に直し、`TestNotificationContext_LogValueEncoding`
      の期待値を宣言型へ更新する。復号が生の宣言型と正規化後の string の両形式を受けることの
      検証は Phase 3 の `TestDecodeNotificationContext_Validity` が担う。
- [ ] `internal/logging/notification_context_test.go` の
      `TestRedactingHandler_ResolvesNotificationContextLogValue` に `monkey` を group、
      `rotate_api_key` を command とするケースを足す。RedactingHandler を通した後の復号が
      元の名前を返すことを検証する（AC-01、AC-02）。
- [ ] AC-19 の確認: `NotificationContext.LogValue`（`notification_context.go`）の
      `group`・`command` の下位値を一時的に `slog.String` へ戻し、
      `TestRedactingHandler_ResolvesNotificationContextLogValue` の `monkey` ケースが
      失敗することを確認して復元する。テストケースを削除しても、そのケースが挙動を
      検証しなくなるだけで同じ失敗は確認できないため、production の符号化を壊す。破壊と
      復元の対象・テスト名をコミットメッセージに記す。
- [ ] `internal/logging/slack_handler_test.go` に
      `TestSlackHandler_IdentifierScopeSurvivesRedaction` を追加する。
      `TestSlackHandler_WithRedactingHandler` と同じ配線（RedactingHandler →
      SlackHandler → モックサーバー）で group `monkey`・command `rotate_api_key` の通知を
      送り、Text 行に `group=monkey command=rotate_api_key` が現れ、`(scope: invalid)` に
      ならないこと、Scope・Command フィールドが `[REDACTED]` でないことを検証する
      （AC-01、AC-02）。
- [ ] `internal/runner/integration_command_results_test.go::TestCommandResults_E2E_Integration`
      を拡張し、redaction を発火させる名前（`rotate_api_key` など）を `CommandResults` に
      載せて、JSON の `name` が元の文字列のままであること、同じ文字列が output／stderr に
      現れた場合は redact されることを検証する（AC-06、AC-09）。
- [ ] `internal/logging/slack_handler_test.go::TestSlackHandler_WithRedactingHandler` を拡張し、
      `command_group_summary` のコマンド一覧の名前（redaction を発火させる名前）が
      RedactingHandler 通過後も Slack のコマンドフィールドに残ることを検証する（AC-09）。
- [ ] `internal/runner/base/audit/logger_test.go` に
      `TestLogUserGroupExecution_CommandNameSurvivesRedaction` を追加する。JSON ハンドラを
      RedactingHandler でラップし、redaction を発火させるコマンド名で
      `LogUserGroupExecution` を呼び、出力 JSON の `command_name` が元の名前のままである
      ことを検証する（AC-09）。既存の `TestLogger_LogUserGroupExecution` は生値捕捉の
      期待値更新に留め、免除後の表示はこの新テストが担う。
- [ ] `internal/runner/config/validation.go` を変更しない。`TestValidateIdentifiers` と
      `TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted` をこのコミットで実行し、
      設定境界の挙動が変わっていないことを確認する（AC-10）。
- [ ] 全 `*_test.go` を検索し、宣言型として捕捉される値を string と比較する期待値が
      残っていないことを確認する。追加の更新が生じた場合は §1.3 の表を更新する。

#### 4.5 構文木ガード

**対象ファイル**: `internal/identifier/identifier_guard_test.go`（新規、`//go:build test`）

- [ ] `identifier_guard_test.go` に `TestIdentifierDeclarationCatalog` を追加する。
      02_architecture.md §3.4 の表から導いた目録を持ち、これはファイル・関数・
      「囲むログ呼び出し／文」・「属性キー」・「引数式」の組と出現数から成る。走査結果に
      目録の外の宣言があれば失敗し、目録のエントリが走査結果に無ければ（宣言の省略・
      削除）も失敗する。
- [ ] `Options.Extra` に修飾形（`ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/identifier"`、
      `FuncName: "NewIdentifier"`）と非修飾形（`ImportPath` を空にする）の 2 件を渡す。
      ファイル列挙は `ProductionGoFilesInRepo`、import 解決は `ResolveLocalImports` を
      再利用する。修飾形はリポジトリ全体に適用する。非修飾形は宣言パッケージを解決できず
      名前だけで一致するため、`internal/identifier` 配下のファイルを走査するときにだけ
      有効化する。他パッケージが自前の同名 `NewIdentifier` を定義・呼び出しても、目録外の
      宣言として失敗させない。`identitymutationguard` は組み込みの syscall 関数も併せて
      報告するため、目録との照合前に呼び出しと値参照を `NewIdentifier` に限定して絞り込む。
      対照テストで syscall の呼び出しが混ざっていても検査が失敗しないことを確認する。
- [ ] 目録の組のうち「囲むログ呼び出し／文」と「引数式」は、`identitymutationguard` の
      `CallSite` が持たないため、guard 自身がファイルを再解析して復元する。呼び出し位置から
      囲む文を求め、`[]any` に詰めて後続の `slog` 呼び出しへ渡す形（`group_executor.go:594`・
      `:617`）では、消費側の `slog` 呼び出しまで辿って文脈を定める。引数式はソースから
      復元する。
- [ ] `NewIdentifier` の値参照（エイリアス）を拒否する。修飾形は
      `identitymutationguard` が報告する `ValueRef` のうち `NewIdentifier` のものが 0 件で
      あることを要求し、非修飾形は `internal/identifier` 配下のファイルだけを対象に
      guard 自身が AST を走査し、`func NewIdentifier` の宣言名と直接呼び出しの被呼び出しを
      除いて、値位置に `NewIdentifier` 識別子が残っていれば失敗させる。
- [ ] `internal/identifier` の production ファイルを AST 走査し、`Identifier` の `name`
      フィールドを設定する複合リテラルとフィールド代入（構造体変数の `id.name = …` と
      ポインタ変数の `p.name = …` の両方）が `NewIdentifier` の本体内だけに現れることを
      要求する。複合リテラルは綴られた型名が `Identifier` かに依存せず、パッケージ内の型
      エイリアス（`type Alias = Identifier`）と定義型（`type Alias Identifier`）の宣言を
      解決し、`Identifier` に由来する型なら `name` を設定するリテラルとして拒否する。これに
      より `type Alias = Identifier; func FromString(s string) Identifier { return Alias{name: s} }`
      のように別名を経由した構築経路も落ちる。`FromString` のような別コンストラクタや
      フィールド代入がパッケージ内に現れれば失敗させる（02_architecture.md §3.1、§7.3）。
      定義パッケージ外はコンパイラが拒否するため、この検査は `internal/identifier` に
      限定する。
- [ ] 対照テスト `TestIdentifierDeclarationCatalog_Control` を置く。合成ソースに対して、
      次の 9 入力で検査の合否を確認する（CLAUDE.md 「Every test must be able to fail for
      its stated reason」）。
  - 目録外の宣言を 1 件足した入力
  - 目録にある宣言を 1 件欠いた入力
  - `makeID := identifier.NewIdentifier` の形の修飾エイリアス
  - 同じ組の件数を保ったまま、囲むログ呼び出しを別のものへ移した入力
  - `internal/identifier` 内に `Identifier{name: "free text"}` を返す別コンストラクタを
    足した入力
  - `internal/identifier` 内に `makeID := NewIdentifier` の形の非修飾エイリアスを足した
    入力。修飾形の `ValueRef` 報告には現れないため、guard 自身の AST 走査が拒否することを
    確認する
  - `internal/identifier` 内に `Identifier.name` へフィールド代入する別コンストラクタを
    足した入力。`var id Identifier; id.name = s; return id` と、ポインタ変数形
    `id := &Identifier{}; id.name = s; return *id` の両方を含める
  - `internal/identifier` 内に型エイリアス・定義型を介して `name` を設定する別コンストラクタを
    足した入力。`type Alias = Identifier; func FromString(s string) Identifier { return Alias{name: s} }`
    と、定義型 `type Alias Identifier; func FromString(s string) Identifier { return Identifier(Alias{name: s}) }`
    を置き、綴られた型名ではなく `Identifier` に由来する型として拒否されることを確認する
  - `internal/identifier` 以外のパッケージが自前の `func NewIdentifier(s string) string`
    を定義して呼び出す入力。非修飾形の追跡を `internal/identifier` に限定しているため、
    この入力では検査が失敗しない（同名関数を誤検知しない）ことを確認する
- [ ] AC-19 の確認: 宣言サイトを 1 件削除すると guard が失敗すること、展開済みコマンド行を
      `identifier.NewIdentifier` で包むと guard が失敗すること、`internal/identifier` 内に
      複合リテラル・フィールド代入・型エイリアス／定義型で `name` を設定する別コンストラクタを
      足すと guard が失敗することを確認し、コミットメッセージに記す。

**分岐と証拠の対応。** Phase 4 が主張する分岐・変換ごとに、その分岐を欠いた実装では
通らない証拠を次の表で固定する。guard の対照入力は
`TestIdentifierDeclarationCatalog_Control` が、production mutation は Phase 4.4 の
AC-19 確認が担う。

| 分岐・変換 | 失敗させる証拠 | 対応するテスト |
|---|---|---|
| 修飾エイリアス `makeID := identifier.NewIdentifier` の拒否 | 対照入力: 修飾エイリアスを足した合成ソース | `TestIdentifierDeclarationCatalog_Control` |
| 非修飾エイリアス `makeID := NewIdentifier` の拒否（`internal/identifier` 内） | 対照入力: 非修飾エイリアスを足した合成ソース | 同上 |
| 複合リテラル `Identifier{name: …}` の拒否（`internal/identifier` 内） | 対照入力: `Identifier{name: "free text"}` を返す別コンストラクタ | 同上 |
| フィールド代入 `id.name = …` の拒否（`internal/identifier` 内。ポインタ変数形を含む） | 対照入力: フィールド代入する別コンストラクタ | 同上 |
| 型エイリアス・定義型を介した `name` 設定の拒否（`internal/identifier` 内） | 対照入力: `type Alias = Identifier`／`type Alias Identifier` の `Alias{name: …}` を返す別コンストラクタ | 同上 |
| 非修飾形 `NewIdentifier` 追跡の `internal/identifier` 限定 | 対照入力: 他パッケージが自前の同名 `NewIdentifier` を定義しても検査が失敗しない（同名衝突の誤検知防止） | 同上 |
| `SecurityLogger` 4 メソッドの `command` 属性の生型 | 4 メソッド（`LogTimeoutConfiguration` は両分岐）の `command` を `LogRecorder.AssertAttrs` で `identifier.Identifier` と比較し、`RedactingHandler` 通過後も名前が残ることを確認する | `TestSecurityLogger_LogMethods` |
| `NotificationContext.LogValue` の `group`／`command` 符号化 | AC-19 mutation: 下位値を一時的に `slog.String` へ戻す | `TestRedactingHandler_ResolvesNotificationContextLogValue`（`monkey` ケース） |
| `buildCommandDebugLogArgs` の `cmdName` の宣言型変換 | AC-19 mutation: 戻り値に `cmdName.Name()` を載せる | `TestCommandDebugLogArgs_StdoutTruncation` |

**完了条件**: `make test` が通り、AC-07〜AC-09 のテストと guard が通る。

### Phase 5: 文書の更新と翻訳

**対象ファイル**: §1.3「文書の該当箇所」の 4 文書、`docs/translation_glossary.md`、
`Makefile`（`verify-docs`／`verify-docs-full` ターゲット）

- [ ] `docs/dev/architecture_design/security-architecture.ja.md` の
      「セキュアログと機密データ保護」の第 2 層の説明に、group 名・コマンド名が宣言型
      （`identifier.Identifier`）で免除されること、免除は 3 層すべてに及び、自由文は
      免除されないこと、専用の実行時スイッチが無く rollback は導入コミットの revert で
      行うことを追記する。**追記は `識別子` と `免除` の語を用いる**（用語を統一する）。
- [ ] `docs/user/security-risk-assessment.ja.md` の「限界」に、識別子として宣言された
      group 名・コマンド名は値ベース redaction の対象外であること、設定の名前に機密を
      書いた場合は通知・ログにそのまま出ることを追記する（AC-14、AC-16）。**追記は
      `識別子` と `免除` の語を用いる**。
- [ ] 日本語版 2 文書のコミット後、`/mktrans` で
      `security-architecture.md`・`security-risk-assessment.md` へ反映する
      （`identifier` と `exempt` の語を用いる）。
- [ ] `docs/translation_glossary.md` に `識別子` → `identifier`、`免除` → `exemption` が
      未登録なら追加する。
- [ ] `scripts/verification/check_identifier_exemption_docs.sh` を追加する。§1.3「文書内容の
      検証スクリプト」の表の語をファイルごとに独立して検査し、1 語でも欠ければ非ゼロで
      終了する。`exempt` は語幹で照合し、`exemption`・`exempted` を含める。
- [ ] `Makefile` の `verify-docs` と `verify-docs-full` ターゲットに
      `@./scripts/verification/check_identifier_exemption_docs.sh` を追加し、スクリプトの
      非ゼロ終了を make の失敗として伝播させる。`run_all.sh` は検査結果にかかわらず終了
      コード 0 を返すため、`make verify-docs` はターゲット側で実行したスクリプトの結果で
      失敗する。これにより AC-13〜AC-17 が Phase 6 のゲートと将来の CI で実際に強制される
      （配線を怠ると、スクリプトを追加しても呼び出されず文書の退行を検出できない）。
- [ ] 同スクリプトを Phase 5 の完了時に実行し、すべての語が一致することを確認する。
      `make verify-docs` も実行し、日本語版と英語版の構造が一致することと、スクリプトが
      `make verify-docs` から実行され、失敗時に make が失敗することを確認する。
- [ ] Task 0172 の承認済み文書（`docs/tasks/0172_slack_notification_message_unification/`）
      を変更しない。本タスクが Task 0172 の残余リスクを置き換えたことが、本タスクの
      文書と Phase 5 の更新文書から参照できることを確認する（AC-17）。
- [ ] 追加した段落を読み、識別子が免除されることと、名前に機密を書いた場合の帰結が
      書かれていることを確認する（スクリプトの語一致だけでは内容の正しさまでは保証
      できないため）。

**完了条件**: `scripts/verification/check_identifier_exemption_docs.sh` が成功し、
AC-13〜AC-17 の検証が通り、`make verify-docs` がスクリプトを含めて成功する
（スクリプトを失敗させると `make verify-docs` も失敗することを確認する）。

### Phase 6: 全体検証と AC-19 の確認

- [ ] 最終コミットで `make test`・`make lint`・`make verify-docs` を通す（AC-18）。
- [ ] `git log` で各コミットメッセージに AC-19 の確認記録（壊した対象と、失敗を確認した
      テスト名）が含まれることを確認する。
- [ ] `make deadcode` を実行し、新しい到達不能コードの報告が無いことを確認する。
- [ ] 本計画書のチェックボックスを実装の進捗に合わせて更新し、§6 と §7 の結果を記録する。

## 3. 実装順序とマイルストーン

| マイルストーン | 含む Phase | 成果物 | 判定 |
|---|---|---|---|
| M1: 宣言型 | Phase 1 | `internal/identifier` の型と単体テスト | 型のテストが緑。leaf 条件を確認 |
| M2: 免除 | Phase 2 | 3 挿入点、redaction テスト、パターン集合の固定テスト | AC-03・AC-04・AC-05・AC-12 が緑。AC-08 はテスト側が緑（guard 側は M4） |
| M3: 復号受理 | Phase 3 | `NotificationContext` の復号受理と復号テスト | AC-11 が緑。符号化を伴う AC-01・AC-02 は M4 で確認する |
| M4: 宣言と guard | Phase 4 | 12 ファイルの宣言サイト（`NotificationContext.LogValue` の符号化を含む）、既存テスト更新、`identifier_guard_test.go` | AC-01・AC-02・AC-06・AC-07・AC-08・AC-09 が緑。AC-10 の既存テストが無変更で通る。`make test` が緑 |
| M5: 文書 | Phase 5 | 日英 4 文書、用語集、検証スクリプト、`verify-docs` ターゲットへの配線 | AC-13〜AC-17 が緑。`make verify-docs` がスクリプトの失敗を伝播する |
| M6: 全体検証 | Phase 6 | 全体の green gate、AC-19 の記録 | AC-18・AC-19 が緑 |

Phase 1 を最初に置く理由、Phase 2 を Phase 3 より先に置く理由、Phase 4 を最後の実装
Phase に置く理由は 02_architecture.md §8.2 のとおりである。

## 4. テスト戦略

テストの観点は 02_architecture.md §7 が定義する。本節では、その観点を実装作業へ割り当てる
方針だけを記す。

### 4.1 単体テスト

- **新規**: `internal/identifier/identifier_test.go`（Phase 1）、
  `internal/redaction/redactor_test.go` の免除・対照テストと `TestDefaultPatternSets_AreUnchanged`
  （Phase 2）、`internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_CommandNameSurvivesRedaction`
  （Phase 4）、`internal/identifier/identifier_guard_test.go`（Phase 4）。
- **拡張**: `internal/common/notification_context_test.go`（復号の両形式・宣言型以外の
  `LogValuer` の拒否は Phase 3、符号化期待値は Phase 4）、`internal/common/logschema_test.go`
  （`name` の宣言型符号化、Phase 4）、`internal/logging/notification_context_test.go`
  （`monkey`／`rotate_api_key` の往復、Phase 4）、`internal/logging/slack_handler_test.go`
  （Scope 表示、`command_group_summary` の
  コマンド一覧）、`internal/runner/integration_command_results_test.go`（JSON 出力での
  名前残存）。
- **更新**: §1.3「既存テストの更新が必要な箇所」の表のテストを `identifier.NewIdentifier`
  基準へ移す。

免除のテストは 3 層それぞれの一致形を、同じ文字列を plain string に載せた対照と対にして
書く（02_architecture.md §7.1）。対照ケースは免除が plain string へ漏れていないことを、
免除ケースと同じテスト関数内で検証する。

### 4.2 統合テスト

- RedactingHandler → JSON ハンドラで、`CommandResult` の `name` が redaction 後も元の
  文字列として出力されること（`internal/runner/integration_command_results_test.go`）。
- RedactingHandler → SlackHandler で、Scope が `monkey`・`rotate_api_key` を表示し、
  `(scope: invalid)` にならないこと
  （`internal/logging/slack_handler_test.go::TestSlackHandler_IdentifierScopeSurvivesRedaction`）。
- `command_group_summary` のコマンド一覧の名前が RedactingHandler 通過後も残ること
  （`internal/logging/slack_handler_test.go::TestSlackHandler_WithRedactingHandler` の拡張）。
- 監査ログの `command_name` が redaction 後も元の名前であること
  （`internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_CommandNameSurvivesRedaction`）。

### 4.3 セキュリティテスト

02_architecture.md §7.3 の項目を Phase 2 と Phase 4 に割り当てる。とくに次を固定する。

- `--password=x`・`token=…`・`Bearer …`・AWS/GitHub/Slack トークン形が、識別子と同じ
  テスト入力集合で plain string として redact されること（AC-05）。
- 同じキー `"command"` にコマンド名（宣言型）とコマンド行（plain string）を載せ、後者
  だけが redact されること（AC-08）。
- `[]Identifier`・`[]*Identifier` の要素が免除されること（§3.2 の挿入点）。
- 型付き nil の `*identifier.Identifier` がパニックせず fail-closed になること。
- 宣言サイトの限定を `identifier_guard_test.go` が固定すること（AC-07）。

### 4.4 後方互換テスト

- 設定検証が識別子の中身を redaction と照合しないことは、`TestValidateIdentifiers` と
  `TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted` をそのまま通すことで
  確認する（AC-10）。
- パターン集合の既存テスト（`internal/redaction/sensitive_patterns_test.go`、
  `internal/redaction/value_detector_test.go`）をそのまま通す（AC-12）。
- Slack の Scope 表示契約と補間契約の既存テスト
  （`TestSlackHandler_InvalidNotificationContext`、`TestSlackHandler_IdentifierIsNotTruncated`
  ほか）をそのまま通す（AC-11）。
- ドライラン・通常モードの既存テストが通ること（副作用の境界は 02_architecture.md §2.4）。

## 5. リスク管理

### 5.1 技術リスク

| リスク | 影響 | 対応 |
|---|---|---|
| 宣言サイトの置き換えが 1 件でも漏れると、その名前は redaction で書き換わり続ける（02_architecture.md §5.1 の T3 方向） | 通知・監査で一部の名前が `[REDACTED]` のまま残る | 置き換えは 02_architecture.md §3.4 の表に従い、Phase 4 で全 43 件を処理する。guard の目録は表と双方向に照合するため、表にある宣言の欠落も検出される |
| guard の目録が実装の移設に追随できず、件数だけ保った移動を見逃す | 過剰免除（02_architecture.md §5.1 の T2）の検出漏れ | 目録に「囲むログ呼び出し／文」・「属性キー」・「引数式」と出現数を持たせ、同一の組が複数回現れる場合も件数で検出する（02_architecture.md §7.3）。`TestIdentifierDeclarationCatalog_Control` が、件数を保った移動と `NewIdentifier` のエイリアスを拒否することを固定する |
| `LogRecorder` 系の生値比較の更新漏れ | `make test` が失敗するが、原因の特定に時間がかかる | §1.3 の表を起点に全 `*_test.go` を検索する。コンパイルエラーになる型変更（`SecurityLogger`）は別に扱う |
| 免除経路の挿入順を誤り、キー名判定より先に免除してしまう | 機密キーの下の値が免除され、漏洩方向の縮小が崩れる | キー名判定を先に置く。`TestRedactingHandler_SensitiveKeyMaskPrecedesExemption` で固定する |
| `processSlice` で生の `Identifier` を append する | JSON に `[{}]` と描画され、§1.1 の string 正規化と AC-06 に反する | `Name()` の string に正規化する。`TestRedactingHandler_IdentifierSliceElements` が下流の描画を検証する |
| 文書の語句検証（`識別子`／`免除`、`identifier`／`exempt`）が表現の揺れで落ちる | Phase 5 の完了判定が止まる | `scripts/verification/check_identifier_exemption_docs.sh` の語を Phase 5 のタスクで指定し、用語集にも登録する。落ちた場合は語句検証と本文のどちらを直すかを判断し、判断理由をコミットメッセージに記す |
| パターン集合をうっかり変更する | 自由文の検出挙動が変わる（AC-12 違反） | Phase 2 の対象ファイルを `redactor.go` に限定し、`TestDefaultPatternSets_AreUnchanged` で集合と件数を固定する。既存のパターンテストを変更しない |

### 5.2 スケジュールリスク

| リスク | 影響 | 緩衝策 |
|---|---|---|
| Phase 4 の差分が大きくレビューが滞留する | M4 が遅れる | 宣言・目録・アサーションを同一コミットに保つ規約（§1.2）のため Phase 4 は 1 コミットとし、レビューは 4.1〜4.5 の小見出し単位で依頼する。4.3 は表の機械的な適用に留める |
| guard の目録作成に時間がかかる | Phase 4 が遅れる | 目録の出典を 02_architecture.md §3.4 の表に固定し、本書では再掲しない。43 件・12 ファイルの件数を要件と突き合わせて進捗を測る |
| 英語版の反映が Phase 5 内に収まらない | M5 が遅れる | 日本語版と英語版を別コミットに分け、日本語版だけを先に確定できる形にする |

## 6. 実装チェックリスト

- [ ] Phase 1: `internal/identifier` の型・単体テスト・leaf 条件確認
- [ ] Phase 2: 3 挿入点・免除／対照テスト・パターン集合の固定テスト
- [ ] Phase 3: `NotificationContext` の復号受理と復号テスト
- [ ] Phase 4.1: `CommandResult`／`CommandResults` の宣言
- [ ] Phase 4.2: `SecurityLogger` の引数型と 3 呼び出し元
- [ ] Phase 4.3: 02_architecture.md §3.4 の残り宣言サイト（12 ファイル・43 件。`NotificationContext.LogValue` の符号化を含む）
- [ ] Phase 4.4: 既存テストの更新、統合テストの追加、AC-10 の既存テスト確認、全 `*_test.go` の再検索
- [ ] Phase 4.5: `TestIdentifierDeclarationCatalog`・`name` フィールドのパッケージ内 AST 制限・対照テスト
- [ ] Phase 5: 日英 4 文書の更新、用語集、`check_identifier_exemption_docs.sh`、`make verify-docs`
- [ ] Phase 6: 全体の green gate、AC-19 の記録確認、`make deadcode`
- [ ] 全 Phase: 各コミットで `make test`・`make lint` が緑

## 7. 受け入れ基準の検証

各行の「種別」は `test`（実行可能で、挙動を壊すと失敗する）、`static`（guard テスト、
`make` ターゲット、または Phase 5 で追加する検証スクリプト）、`manual`（PR、コミット
メッセージ、追加した段落の読解での観察）を表す。文書の語句検証は、文書内にしか無い
コマンドではなく `scripts/verification/check_identifier_exemption_docs.sh` に集約し、
Phase 5 の完了ゲートで実行する。スクリプトは `verify-docs`／`verify-docs-full` ターゲット
から実行され、非ゼロ終了は `make verify-docs` の失敗として伝播する。このスクリプトの語は
`b2d2f744` 時点の `rg` と同じ組であり、実装前は新しい語が未記載のため失敗する。

| AC | 種別 | 検証（実行する成果物） | 実装 Phase |
|---|---|---|---|
| AC-01 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_IdentifierScopeSurvivesRedaction`（Text 行に `group=monkey`）と `internal/logging/notification_context_test.go::TestRedactingHandler_ResolvesNotificationContextLogValue`（`monkey` ケース） | Phase 4 |
| AC-02 | test | 同上の `rotate_api_key` ケース（`command=rotate_api_key`） | Phase 4 |
| AC-03 | test | `internal/redaction/redactor_test.go::TestRedactingHandler_IdentifierExemption` の値形式層（02_architecture.md §7.1 の表の 2 行目の入力） | Phase 2 |
| AC-04 | test | 同テストの 3 層（key=value 置換・値形式検出・値まるごと判定） | Phase 2 |
| AC-05 | test | `internal/redaction/redactor_test.go::TestRedactingHandler_PlainStringIsStillRedacted`（識別子と同じ入力集合の plain string が redact される）、`TestRedactingHandler_Handle_MessageRedaction`、`TestRedactingHandler_ErrorValue` | Phase 2 |
| AC-06 | test | `internal/runner/integration_command_results_test.go::TestCommandResults_E2E_Integration`（拡張後。JSON 出力で `name` が元の string、同じ文字列は output／stderr で redact）、`internal/common/logschema_test.go::TestCommandResults_LogValue`（`KindLogValuer` ＋ `Value.String()`）、`internal/logging/slack_handler_test.go::TestSlackHandler_IdentifierScopeSurvivesRedaction`（Slack 読み取り） | Phase 4 |
| AC-07 | static | `internal/identifier/identifier_guard_test.go::TestIdentifierDeclarationCatalog`（目録と走査結果の双方向照合。12 ファイル・43 件） | Phase 4.5 |
| AC-08 | test + static | test: `internal/redaction/redactor_test.go::TestRedactingHandler_PlainStringIsStillRedacted`（同じ `command` キーのコマンド名とコマンド行）。static: 同上の guard（§3.5 の値を包むと目録不一致で失敗し、`internal/identifier` 内に `name` を設定する別コンストラクタを足すと AST 制限で失敗する） | Phase 2・4.5 |
| AC-09 | test | `internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_CommandNameSurvivesRedaction`（新規。redaction を発火させる名前の `command_name` が残る）と `internal/logging/slack_handler_test.go::TestSlackHandler_WithRedactingHandler`（拡張後。`command_group_summary` のコマンド一覧の名前が残る） | Phase 4 |
| AC-10 | test | `internal/runner/config/validation_test.go::TestValidateIdentifiers` と `cmd/runner/integration_pre_execution_error_test.go::TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted`。Phase 4.4 と Phase 6 で実行する | Phase 4.4・6 |
| AC-11 | test | `internal/logging/slack_handler_test.go::TestSlackHandler_InvalidNotificationContext`（無変更で表示契約を固定）と `internal/common/notification_context_test.go::TestDecodeNotificationContext_Validity`（宣言型と正規化後の string の両形式を受ける更新後の行） | Phase 3 |
| AC-12 | test | `internal/redaction/redactor_test.go::TestDefaultPatternSets_AreUnchanged`（新規。`DefaultKeyValuePatterns` の集合と件数、`DefaultSensitivePatterns` の `AllowedEnvVars` の集合と結合正規表現のソース、`valueDetectorPatterns` の各正規表現ソースを固定）と、`internal/redaction/sensitive_patterns_test.go`・`internal/redaction/value_detector_test.go` の既存テストが無変更で通ること | Phase 2 |
| AC-13 | static | `scripts/verification/check_identifier_exemption_docs.sh`（`docs/tasks/0173_identifier_redaction_exemption/02_architecture.md` の `残余リスク` と `record.Message` を検査。`b2d2f744` で一致を確認済み） | 設計済み |
| AC-14 | static | 同上のスクリプト（`docs/user/security-risk-assessment.ja.md` の `識別子` と `免除` を検査。`識別子` の既存 1 件は別文脈のため、`免除` が加わらなければ失敗する）。加えて追加段落を読み、AC-14 の内容を確認する（manual） | Phase 5 |
| AC-15 | static | 同上のスクリプト（`security-architecture.ja.md` の `識別子`・`免除`・`NewIdentifier`、`security-architecture.md` の `identifier`・`exempt`・`NewIdentifier` を検査。実装前はいずれも 0 件） | Phase 5 |
| AC-16 | static | 同上のスクリプト（`docs/user/security-risk-assessment.md` の `identifier` と `exempt` を検査）と `make verify-docs` | Phase 5 |
| AC-17 | static | 同上のスクリプト（`01_requirements.md` の `0172` と `置き換え` を検査）と、Task 0172 の承認済み文書に差分が無いこと（Phase 6 で `git diff` を確認する manual） | 設計済み・6 |
| AC-18 | static | 各 Phase のコミットで `make test` と `make lint`（`make` ターゲット） | 全 Phase |
| AC-19 | test + manual | Phase 1〜4 のタスクに列挙した mutation（実装を一時的に壊し、対応するテストの失敗を確認して復元）を `make test` 後の状態で行う。test: 各 AC 行が指すテスト。manual: `git log -1 --format=%B <sha>` に壊した対象と失敗したテスト名が含まれることを Phase 6 で確認する | Phase 1〜4・6 |

補足: AC-13・AC-17 は設計文書が `approved` である時点で内容が固定されているため、`rg` を
`b2d2f744` で実行して一致を確認し、その組をスクリプトの検査に含める。AC-13 の
`02_architecture.md` と AC-17 の `01_requirements.md` は実装中も変更しないため、スクリプトの
この 2 行は導入時から成功する。AC-14〜AC-16 の行は Phase 5 まで失敗する。

## 8. 横断検索チェックリスト

`make lint` と `make test` が検出できない項目だけを挙げる。シンボルの残留参照は
`identifier_guard_test.go` とコンパイラが検出するため、ここには含めない。

- [ ] `docs/translation_glossary.md` に `識別子` → `identifier`、`免除` → `exemption` が
      登録されているか確認する。未登録なら Phase 5 で追加し、日英文書の訳語が一致する
      ことを確認する。
- [ ] `docs/tasks/0172_slack_notification_message_unification/` の承認済み文書に差分が
      無いことを `git diff` で確認する（履歴として残す。AC-17）。
- [ ] Phase 5 の日英文書について `make verify-docs` を実行し、構造比較のレポート
      （`build/verification-reports/structure_comparison_report.txt`）に見出し構造の差分が
      無いことを確認する。`run_all.sh` は検査結果にかかわらず終了コード 0 を返すため、
      構造比較は終了コードだけでは判定しない。一方、`verify-docs`／`verify-docs-full`
      ターゲットに追加する `check_identifier_exemption_docs.sh` の失敗は make の終了
      コードへ伝播するため、AC-13〜AC-17 の語句検証は `make verify-docs` の成否で判定する。

## 9. 成功基準

### 9.1 機能の完成度

- 宣言された識別子（Scope と構造化された名前属性）が、内容にかかわらず通知・ログで元の
  文字列のまま表示される。
- group 名・コマンド名をログ属性値として書く production の全経路（12 ファイル・43 サイト）
  が宣言型を使い、guard が目録との双方向一致を固定する。

### 9.2 品質

- `make test`・`make lint` が各コミットで通り、`make verify-docs` の構造比較レポートに
  日英文書の見出し差分が無い。
- 新規・更新テストが AC-01〜AC-11 を覆い、AC-19 の mutation 確認が記録されている。

### 9.3 セキュリティ

- 自由文・コマンド行・引数・環境変数値の redaction が弱まっていない（AC-05、AC-08）。
- キー名判定が免除判定より先に働く（fail-closed）。
- パターン集合が変更されていない（AC-12）。
- 免除の適用範囲と残余リスクが文書化されている（AC-13〜AC-16）。

### 9.4 文書

- `security-architecture.ja.md` / `.md` と `security-risk-assessment.ja.md` / `.md` が
  更新され、Task 0172 の残余リスクを置き換えたことが追跡できる（AC-15〜AC-17）。

## 10. 次のステップ

1. 本計画書を人間がレビューし、`03_implementation_plan.md` のステータスを `approved` に
   する（`draft` のままでは実装を開始しない。requirements_process.md §0）。
2. `/mkplan2` で PR 境界と実装モデル要件を挿入する。
3. `/runplan` で Phase 1 から実装する。各 Phase の完了条件と AC-19 の記録を守る。
4. Phase 5 の日本語版コミット後、`/mktrans` で英語版へ反映する。
