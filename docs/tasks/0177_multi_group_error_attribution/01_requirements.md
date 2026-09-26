# 要件定義書: 複数 group 失敗時のエラー行への group 帰属の表示

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-25 |
| Review date | - |
| Reviewer | - |
| Comments | 2026-09-26: 方針を変更した。各行に context を付ける表示側の変更をやめ、`UserFriendlyError` を削除して原因の文言を常にそのまま出すことで帰属を保つ。専用のエラー型は、`Unwrap() []error` の形による判定を置き換える役割に絞った。同日、対象外としていた `CaptureError.Error()` の文言の整理と `GetType`・`GetPath` の削除を対象に含めた（AC-16〜AC-18）。新たに対象外とした 2 件は #1180・#1181 として起票した。 |

## 関連 Issue

- [#1179](https://github.com/isseis/go-safe-cmd-runner/issues/1179) 複数 group 失敗時に UserMessage で表示される group エラーの帰属 group が分からない
- 派生元: [#1153](https://github.com/isseis/go-safe-cmd-runner/issues/1153) / PR #1176（全 group のエラーを返す変更、multi-error 時の context 抑止、`formatCause`）
- 関連: [#1177](https://github.com/isseis/go-safe-cmd-runner/issues/1177) / PR #1178（複数行エラーの stderr 表示）

## 背景

### 現在の流れ

group の失敗は次の順に扱われる。

1. [`Runner.executeGroups`](../../../internal/runner/runner.go) は、失敗した group のエラーを `failed to execute group <name>: %w` でラップして集める。失敗が 1 件ならそのまま返し、2 件以上なら `errors.Join` で返す。`*verification.Error` による group ファイル検証の失敗は別経路で通知され、集める対象に入らない。
2. [`cmd/runner/main.go`](../../../cmd/runner/main.go) の `executionErrorContext` は、戻り値が `Unwrap() []error` を持つ（multi-error である）とき group 名・command 名を空にする。そうでなければ `*runner.CommandExecutionError` から group 名・command 名を取り出す。これらを `logging.ExecutionError` の `GroupName`・`CommandName` に設定する。
3. [`logging.HandleExecutionError`](../../../internal/logging/pre_execution_error.go) は `Message (group: ..., command: ...): <cause>` を組み立て、stderr の `Details:` と構造化ログの `error_message` に出す。cause は [`formatCause`](../../../internal/logging/execution_error.go) が作る。`formatCause` は multi-error を子ごとに分け、各子を `UserFriendlyError.UserMessage()` があればそれに、なければ `Error()` の文言にして、1 行ずつ並べる。`PreExecutionError.Detail` も同じ `formatCause` を使う。

### 問題

`UserFriendlyError` を含む子（実装しているのは出力キャプチャの失敗 `output.CaptureError` だけ）は、`formatCause` で `UserMessage` に置き換わる。このとき `failed to execute group <name>: command <cmd> in group <name> failed:` の部分が落ちる。multi-error では外側の context も空なので、その行の group 名・command 名はどこにも出ない。

例（group-1 がコマンド失敗、group-2 が出力サイズ超過）:

```
  Details: error running commands: failed to execute group group-1: command fail-cmd in group group-1 failed: ...
           output size limit exceeded for '/path/to/out'
```

2 行目がどの group・command のエラーかは、slog の他のレコードを突き合わせないと分からない。

### `UserFriendlyError` はもう必要ない

`UserFriendlyError` は 2025-11-10（`6f88d77e`）に導入された。当時の `HandleExecutionError` は `Message` と context だけを出し、原因（`Err`）を報告に出していなかった。出力サイズ超過の理由を報告に出すための特別扱いを一般化したのが `UserFriendlyError` であり、当時は原因を報告に出す唯一の経路だった。

現在は、`UserMessage` が無いときも原因の `Error()` がそのまま報告に出る（2026-07-21 `f215efec` 以降）。導入時の理由は成り立っておらず、残っている役割は原因の文言を差し替えることだけである。この差し替えには次の問題がある。

- **外側の文脈が消える。** エラーのチェーンの途中にある `UserMessage` が全体を置き換えるため、group 名・command 名を含むラップの文言が落ちる。本 issue の症状の原因はこれである。
- **原因が消える。** 例えば `ErrorTypeFileSystem` の `UserMessage` は `filesystem error for '<path>'` だけで、`Cause`（例: `no space left on device`）を含まない。`Error()` には含まれる。
- **実装が 1 つしかない。** 汎用のインタフェースを維持する価値が薄い（CLAUDE.md「YAGNI」、および「Enforce invariants with the type」の、使われていない拡張点を残さない方針）。

`UserFriendlyError` を削除して原因の `Error()` を常にそのまま出せば、各行は `failed to execute group <name>: ...` で始まり、帰属が分かる。

### 複数 group の失敗を形で判定している

`executionErrorContext` と `formatCause` は、どちらも「`Unwrap() []error` を持つか」という形で「複数 group が失敗した」と判定している。`errors.Join` は汎用の仕組みであり、この判定は「複数 group の失敗」を宣言していない（CLAUDE.md「Declare, don't infer」）。

`UserFriendlyError` を削除すると、`formatCause` が子ごとに分ける必要はなくなる。`errors.Join` の `Error()` は子の文言を改行で並べたもので、分けた結果と同じになるためである。残る判定は `executionErrorContext` のものだけで、これは runner が型で宣言すれば置き換えられる。

### `GroupName` の利用箇所

`ExecutionError.GroupName`・`CommandName` の利用箇所は `ContextString` だけである。`ContextString` は stderr の `Details:` と構造化ログの `error_message` に使われる。`HandleExecutionError` の構造化ログレコードは `slack_notify=false` 固定であり、Slack 通知には使われない（Slack の `group=` 表示は別の `NotificationContext` による）。

## 目的

- 複数 group が失敗したとき、報告の各行から、その行がどの group（と command）の失敗かを判別できるようにする。
- 原因の文言を差し替えず、報告に原因の情報をすべて残す。
- 「group の失敗」を専用のエラー型で宣言し、`Unwrap() []error` の形による判定をやめる。失敗 1 件は複数件の特殊な場合として同じ型で表す。

## スコープ

### 対象

1. `UserFriendlyError` インタフェース・`GetUserFriendlyMessage`・`CaptureError.UserMessage` を削除する。
2. 実行エラー（`HandleExecutionError`）と実行前エラー（`PreExecutionError.Detail`）の報告で、原因は常に `Error()` の文言をそのまま出す。`formatCause` は不要になるので削除する。
3. `executeGroups` が、1 件以上の group の失敗を、件数によらず、失敗 group ごとに group 名・command 名（分かるとき）・原因を持つ専用のエラー型で返す。
4. `executionErrorContext` の multi-error 判定を、この型が持つ失敗の件数による判定に置き換える。
5. 出力サイズ超過の `CaptureError.Error()` の文言から、同じ事実の繰り返しをなくし、超過した上限値を載せる。
6. 本番コードで使われていない `CaptureError.GetType`・`GetPath` を削除する。

### 対象外

- **出力サイズ超過以外の `CaptureError`。** 本番コードが生成するのは出力サイズ超過のほかに `ErrorTypeFileSystem`（`Cause` は OS のエラー）だけで、その文言に繰り返しはない。本番で生成されない `ErrorType`・`ExecutionPhase` の整理は [#1180](https://github.com/isseis/go-safe-cmd-runner/issues/1180) で扱う。
- **`output.ErrOutputSizeLimitExceeded`（`manager.go`）と `ErrOutputSizeExceeded` の重複。** 同じ文言の 2 つのセンチネルがある。前者を返す `DefaultOutputCaptureManager.WriteOutput` は本番コードから呼ばれていない。本 issue とは独立しているので [#1181](https://github.com/isseis/go-safe-cmd-runner/issues/1181) で扱う。
- **各行に `(group: ..., command: ...)` を付ける表示。** 原因の文言が `failed to execute group <name>` を含むので不要である。
- **`ExecutionError.GroupName`・`CommandName` に複数の group を持たせること。** 決定事項「複数失敗時の `GroupName` は空のまま」を参照。
- **Slack 通知。** 実行エラーの構造化ログレコードは `slack_notify=false` のままとし、通知の内容・件数は変えない。`command_group_summary` に失敗理由を載せる改善は別 issue で扱う（決定事項「検討して採らなかった案」を参照）。
- **dry-run の実行エラー記録（`SetDryRunExecutionError`）。** `Error()` の文言を使っており、その文言は変えない（AC-09）ので影響しない。
- **group ファイル検証の失敗（`*verification.Error`）とコンテキストのキャンセル。** 現状どおり、集める対象に入らない（検証失敗）・即座に返す（キャンセル）。

## 決定事項

### `UserFriendlyError` を削除し、原因の文言を差し替えない

報告に出す原因は、常にエラーの `Error()` の文言とする。原因の一部を別の文言に差し替える仕組みは持たない。理由は背景「`UserFriendlyError` はもう必要ない」のとおりである。差し替えをやめることで、ラップの文言（group 名・command 名）と原因の詳細（`Cause`）の両方が報告に残る。

この変更は `PreExecutionError.Detail` にも及ぶ。実行前エラーの原因のチェーンに `CaptureError` が入る経路は現状ないので、実行前エラーの報告の文言は変わらない。

### 出力サイズ超過の文言を整える

`UserMessage` を削除すると、出力サイズ超過は `Error()` の文言で報告される。現状の文言は次のとおりで、同じ事実を 2 回述べ、超過した上限値を含まない。

```
output capture error during execution phase: size limit exceeded for '<path>': output size limit exceeded
```

`Type`（`size limit exceeded`）と `Cause`（センチネル `ErrOutputSizeExceeded` の `output size limit exceeded`）が同じ事実を表しているためである。出力サイズ超過の `Error()` は、段階・パス・上限値を含み、超過の事実を 1 回だけ述べる文言にする。上限値は、`UserMessage` が出していた情報（超過とパス）に加えて、利用者が設定を直すために必要な情報である。文言の具体的な形と、上限値の持たせ方は設計段階で決める。

`errors.Is(err, output.ErrOutputSizeExceeded)` は引き続き成り立つようにする。テスト（`test/performance`、`executor_privilege_gap_integration_test.go`）がこれで出力サイズ超過を判定している。

### 使われていない `GetType`・`GetPath` を削除する

`CaptureError.GetType`・`GetPath` は、`UserFriendlyError` の前身である `CaptureErrorInterface`（出力サイズ超過を package への依存なしに検出するためのインタフェース）のために追加された。`CaptureErrorInterface` は `6f88d77e` で削除され、以後この 2 つを呼ぶ本番コードはない。`UserFriendlyError` と同じ仕組みの名残なので、あわせて削除する。

### group の失敗は件数によらず専用の型で表す

`executeGroups` は、group の失敗が 1 件以上あるとき、専用の型（以下、仮に `GroupErrors`）を返す。型は失敗 group ごとに次を持つ。

| 項目 | 由来 |
|---|---|
| group 名 | `executeGroups` が実行した `GroupSpec.Name`（エラーから取り出さない） |
| command 名 | 原因のチェーンにある型（`*CommandExecutionError`、command レベルの段階の `*GroupStageError`）が宣言している command 名。無ければ空 |
| 原因 | `ExecuteGroup` が返したエラー |

失敗 1 件のときに別の形（ラップしたエラーをそのまま返す）を使わない。1 件と複数件で形を分けると、呼び出し側は再び「どちらの形か」で分岐することになり、`Unwrap() []error` の形による判定と同じ問題が残る。1 件と複数件の違いは、型が持つ失敗の件数で表す。

- `executionErrorContext` は、失敗が 1 件ならその失敗の group 名・command 名を外側の context に設定し、2 件以上なら空にする。
- 失敗 1 件のとき、group 名は `GroupSpec.Name` から必ず得られる。このため、現状は外側の context が空になる `*CommandExecutionError` 以外の単一失敗（例: group の展開の失敗）にも、外側の context が付くようになる（AC-15）。帰属を型から得ることの自然な結果であり、情報が増えるだけなので受け入れる。

型名・フィールドの形は設計段階で決める。

### 複数失敗時の `GroupName` は空のまま

`ExecutionError.GroupName`・`CommandName` は、複数 group の失敗では従来どおり空にする。一覧を持たせる案は取らない。理由は次のとおり。

- これらのフィールドの利用箇所は `ContextString`（stderr・`error_message`）だけで、Slack 通知には使われない。
- 帰属は各行の原因の文言（`failed to execute group <name>: ...`）に現れるので、外側の context に一覧を並べると同じ情報が 2 回出る。

### `Error()` の文言と `errors.Is`・`errors.AsType` の到達性は変えない

新しい型の `Error()` は、変更前の戻り値と同じ文言を返す。失敗 1 件なら `failed to execute group <name>: ...` の 1 行、2 件以上ならそれを改行で並べたもの（`errors.Join` の文言）である。報告の各行と、dry-run の実行エラー記録など `Error()` の文言を使う箇所の出力を変えないためである。また、`errors.Is`・`errors.AsType` が各 group の原因に届くことを保つ。

### 検討して採らなかった案

| 案 | 採らなかった理由 |
|---|---|
| `UserFriendlyError` を残し、専用の型から各行に `(group: ..., command: ...)` を付ける | 原因の差し替えで `Cause` が消える問題が残る。`logging` が runner の型を知るためのインタフェースと表示の分岐が要り、変更範囲が大きい。`UserFriendlyError` を削除すれば、原因の文言がすでに group 名を含む |
| group のラップ用エラーに `UserFriendlyError` を実装させ、`UserMessage` に group 名・command 名を含める | `UserMessage` の意味（特別な文言がなければ空を返す）を変えてしまう。外側の context と重複しないよう特別扱いが要る。`Unwrap() []error` の形による判定も残る |
| 失敗した時点で group ごとに stderr へ報告する | stderr の `Error:` ブロックと stdout の `RUN_SUMMARY` 行が増える。Task 0176 の「新しい経路は `RUN_SUMMARY` 行と stderr 報告を増やさない」という決定に反する |
| group ごとにエラー本文を Slack へ通知する（`command_group_summary` に失敗理由を載せる、または新しい通知種別を作る） | 本 issue の対象である stderr と構造化ログの帰属は解決しない。Slack には group ごとの通知がすでにあり、欠けているのは帰属ではなく失敗の理由（`groupExecutionResult.errorMsg` は設定されているが送られていない）である。これは通知のフィールド集合と、Slack へ出す本文の redaction に関わる別の問題なので、別 issue で扱う |

## 受け入れ基準（Acceptance Criteria）

#### F-001: 原因の文言を差し替えない

**Acceptance Criteria**:
- **AC-01**: group-1 がコマンド実行で失敗し、group-2 が出力サイズ超過（`output.CaptureError`）で失敗したとき、stderr の `Details:` に、group-2 の行として `failed to execute group group-2: command <group-2 の失敗 command> in group group-2 failed: ` に続けて `CaptureError.Error()` の文言が出る。
- **AC-02**: AC-01 と同じ実行で、`Details:` の各行はそれぞれ `failed to execute group <group>: ` で始まる（1 行目は `error running commands: ` の後）。
- **AC-03**: `Cause` を持つ `CaptureError` による失敗では、その `Cause` の文言が報告に出る。
- **AC-04**: 構造化ログの `error_message` にも、AC-01〜AC-03 と同じ文言が出る。
- **AC-05**: 複数 group の失敗では、外側の context（`error running commands (group: ..., command: ...)`）は付かない（`ExecutionError.GroupName`・`CommandName` は空）。
- **AC-06**: `UserFriendlyError`・`GetUserFriendlyMessage`・`CaptureError.UserMessage` は本番コードに存在しない。`PreExecutionError.Detail` と `HandleExecutionError` は原因の `Error()` の文言をそのまま使う。

#### F-002: 型による宣言

**Acceptance Criteria**:
- **AC-07**: `executeGroups` は、1 件以上の group が失敗したとき、件数によらず、失敗 group ごとに group 名・command 名・原因を持つ専用の型を返し、group 名は `GroupSpec.Name` から設定される。失敗が無いときは nil を返す。
- **AC-08**: `executionErrorContext` は、専用の型が持つ失敗の件数で外側の context を決める。本番コードに、`Unwrap() []error` の有無で複数 group の失敗を判定する分岐、およびエラー文字列から group 名・command 名を取り出す処理がない。

#### F-003: 既存挙動の維持

**Acceptance Criteria**:
- **AC-09**: 失敗が 1 件のときも 2 件以上のときも、戻り値の `Error()` の文言は変更前と同じである。
- **AC-10**: 失敗が 1 件のときも 2 件以上のときも、`errors.Is`・`errors.AsType` が各 group の原因（`*CommandExecutionError`、`output.CaptureError` など）に届く。
- **AC-11**: 失敗が 1 group だけで、原因が `*CommandExecutionError` であり `CaptureError` を含まないとき、stderr の `Details:`・構造化ログの `error_message`・`ExecutionError.GroupName`・`CommandName` は変更前と同じである。
- **AC-12**: 実行エラーの構造化ログレコードは `slack_notify=false` のままで、Slack 通知の件数・内容は変わらない。プロセスの終了コードと `RUN_SUMMARY` 行も変わらない。
- **AC-15**: 失敗が 1 group だけで、原因が `*CommandExecutionError` ではないとき（例: group の展開の失敗）、外側の context に `group: <group>` が付く。command レベルの段階の `*GroupStageError` では `command: <command>` も付く。Details の原因の文言は変更前と同じである。

#### F-004: `CaptureError` の整理

**Acceptance Criteria**:
- **AC-16**: 出力サイズ超過の `CaptureError.Error()` の文言は、段階・出力先のパス・上限値を含み、「size limit exceeded」に当たる語句を 1 回だけ含む。
- **AC-17**: 出力サイズ超過の失敗で、`errors.Is(err, output.ErrOutputSizeExceeded)` が成り立つ。
- **AC-18**: `CaptureError.GetType`・`GetPath` は存在しない。出力サイズ超過以外の種類の `CaptureError.Error()` の文言は変更前と同じである。

#### F-005: 全体の健全性

**Acceptance Criteria**:
- **AC-13**: 各コミットの時点で `make fmt`（Go を変更した場合）・`make test`・`make lint` が通る。
- **AC-14**: 追加・変更したテストが、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。テストを削除した場合は、`go tool cover -func` の結果が関数単位で変わらないこと（削除した関数を除く）を確認し、その旨を記す。

## Success Criteria（要件レベル）

- 複数 group が失敗したとき、stderr と構造化ログの各行から、その行の group（と command）を他のレコードと突き合わせずに判別できる。
- 報告に出る原因から、ラップの文言と原因の詳細が失われない。
- 出力サイズ超過の報告は、上限値を含み、同じ事実を繰り返さない。
- group の失敗が件数によらず同じ型で宣言され、`errors.Join` の形からの推測がなくなる。
- `Error()` の文言、Slack 通知、終了コードは変わらない。
