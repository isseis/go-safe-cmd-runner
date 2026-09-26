# 要件定義書: 複数 group 失敗時のエラー行への group 帰属の表示

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-25 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#1179](https://github.com/isseis/go-safe-cmd-runner/issues/1179) 複数 group 失敗時に UserMessage で表示される group エラーの帰属 group が分からない
- 派生元: [#1153](https://github.com/isseis/go-safe-cmd-runner/issues/1153) / PR #1176（全 group のエラーを返す変更、multi-error 時の context 抑止、`formatCause`）
- 関連: [#1177](https://github.com/isseis/go-safe-cmd-runner/issues/1177) / PR #1178（複数行エラーの stderr 表示）

## 背景

### 現在の流れ

group の失敗は次の順に扱われる。

1. [`Runner.executeGroups`](../../../internal/runner/runner.go) は、失敗した group のエラーを `failed to execute group <name>: %w` でラップして集める。失敗が 1 件ならそのまま返し、2 件以上なら `errors.Join` で返す。`*verification.Error` による group ファイル検証の失敗は別経路で通知され、集める対象に入らない。
2. [`cmd/runner/main.go`](../../../cmd/runner/main.go) の `executionErrorContext` は、戻り値が `Unwrap() []error` を持つ（multi-error である）とき group 名・command 名を空にする。そうでなければ `*runner.CommandExecutionError` から group 名・command 名を取り出す。これらを `logging.ExecutionError` の `GroupName`・`CommandName` に設定する。
3. [`logging.HandleExecutionError`](../../../internal/logging/pre_execution_error.go) は `Message (group: ..., command: ...): <cause>` を組み立て、stderr の `Details:` と構造化ログの `error_message` に出す。cause は [`formatCause`](../../../internal/logging/execution_error.go) が作る。`formatCause` は multi-error を子ごとに分け、各子を `UserFriendlyError.UserMessage()` があればそれに、なければ `Error()` の文言にして、1 行ずつ並べる。

### 問題

`UserFriendlyError` を含む子（現状は出力キャプチャの失敗 `output.CaptureError` だけ）は、`formatCause` で `UserMessage` に置き換わる。このとき `failed to execute group <name>: command <cmd> in group <name> failed:` の部分が落ちる。multi-error では外側の context も空なので、その行の group 名・command 名はどこにも出ない。

例（group-1 がコマンド失敗、group-2 が出力サイズ超過）:

```
  Details: error running commands: failed to execute group group-1: command fail-cmd in group group-1 failed: ...
           output size limit exceeded for '/path/to/out'
```

2 行目がどの group・command のエラーかは、slog の他のレコードを突き合わせないと分からない。失敗が 1 group だけのときは外側の context（`(group: ..., command: ...)`）が付くので、この問題は起きない。

### 型ではなく形で判定している

`executionErrorContext` と `formatCause` は、どちらも「`Unwrap() []error` を持つか」という形で「複数 group が失敗した」と判定している。`errors.Join` は他の場所でも使われる汎用の仕組みであり、この判定は「複数 group の失敗」を宣言していない。`logging` 側でエラー文字列から group 名を取り出すことも、CLAUDE.md「Declare, don't infer」に反するため取らない。group 名・command 名は runner が型で渡す必要がある。

### `GroupName` の利用箇所

`ExecutionError.GroupName`・`CommandName` の利用箇所は `ContextString` だけである。`ContextString` は stderr の `Details:` と構造化ログの `error_message` に使われる。`HandleExecutionError` の構造化ログレコードは `slack_notify=false` 固定であり、Slack 通知には使われない（Slack の `group=` 表示は別の `NotificationContext` による）。

## 目的

- 複数 group が失敗したとき、報告の各行から、その行がどの group（と、分かるときは command）の失敗かを判別できるようにする。
- 「複数 group の失敗」を専用のエラー型で宣言し、`Unwrap() []error` の形による判定をやめる。
- 失敗が 1 件か複数件かを同じ型で表し、1 件を複数件の特殊な場合として扱う。
- 失敗が 1 group だけのときの報告は、帰属が分からなかった場合（AC-15）を除いて変えない。

## スコープ

### 対象

1. `executeGroups` が、1 件以上の group の失敗を、件数によらず、失敗 group ごとに group 名・command 名（分かるとき）・原因を持つ専用のエラー型で返す。
2. `executionErrorContext` の multi-error 判定を、この型の判定に置き換える。
3. `HandleExecutionError` の報告（stderr の `Details:`・構造化ログの `error_message`）で、この型の各失敗を 1 行ずつ、group 名・command 名を付けて出す。原因の文言は従来どおり `UserMessage` があればそれ、なければ生の文言とする。
4. `logging` は runner の型に依存せず、`logging` 側で宣言した型またはインタフェースで group 名・command 名を受け取る（`runner` は `logging` を import しているため、逆向きの依存は作れない）。

### 対象外

- **失敗が 1 group だけのときの報告の文言。** 外側の context で帰属が分かるので、Details の文言は変えない（外側の context が付くようになる場合は AC-15）。
- **`ExecutionError.GroupName`・`CommandName` に複数の group を持たせること。** 決定事項「複数失敗時の `GroupName` は空のまま」を参照。
- **Slack 通知。** 実行エラーの構造化ログレコードは `slack_notify=false` のままとし、通知の内容・件数は変えない。`command_group_summary` に失敗理由を載せる改善は別 issue で扱う（決定事項「検討して採らなかった案」を参照）。
- **dry-run の実行エラー記録（`SetDryRunExecutionError`）。** `Error()` の文言を使っており、その文言は変えない（AC-09）ので影響しない。
- **`PreExecutionError.Detail` の表示。** `formatCause` を共有するが、`executeGroups` の戻り値は `PreExecutionError` の原因にならないので、表示は変わらない。
- **group ファイル検証の失敗（`*verification.Error`）とコンテキストのキャンセル。** 現状どおり、集める対象に入らない（検証失敗）・即座に返す（キャンセル）。

## 決定事項

### 複数 group の失敗は専用の型で宣言する

`executeGroups` は、group の失敗が 1 件以上あるとき、件数によらず専用の型（以下、仮に `GroupErrors`）を返す。型は失敗 group ごとに次を持つ。

| 項目 | 由来 |
|---|---|
| group 名 | `executeGroups` が実行した `GroupSpec.Name`（エラーから取り出さない） |
| command 名 | 原因のチェーンにある型（`*CommandExecutionError`、command レベルの段階の `*GroupStageError`）が宣言している command 名。無ければ空 |
| 原因 | `ExecuteGroup` が返したエラー |

型名・フィールドの形は設計段階で決める。

### 1 件は複数件の特殊な場合として同じ型で表す

失敗 1 件のときに別の形（ラップしたエラーをそのまま返す）を使わない。1 件と複数件で形を分けると、呼び出し側は再び「どちらの形か」で分岐することになり、`Unwrap() []error` の形による判定と同じ問題が残る。1 件と複数件の違いは、型が持つ失敗の件数で表す。

- `executionErrorContext` は、失敗が 1 件ならその失敗の group 名・command 名を外側の context に設定し、2 件以上なら空にする。
- 報告の各行に context を付けるのは 2 件以上のときだけとする。1 件のときは外側の context がすでに帰属を示すので、行には付けない（同じ情報を 2 回出さないため）。
- 失敗 1 件のとき、group 名は `GroupSpec.Name` から必ず得られる。このため、現状は外側の context が空になる `*CommandExecutionError` 以外の単一失敗（例: group の展開の失敗）にも、外側の context が付くようになる（AC-15）。これは帰属を型から得ることの自然な結果であり、情報が増えるだけなので受け入れる。

### 検討して採らなかった案

| 案 | 採らなかった理由 |
|---|---|
| group のラップ用エラーに `UserFriendlyError` を実装させ、`UserMessage` に group 名・command 名を含める | `UserMessage` の意味（特別な文言がなければ空を返す）を変えてしまう。外側の context と重複しないよう特別扱いが要る。`Unwrap() []error` の形による判定も残る |
| 失敗した時点で group ごとに stderr へ報告する | stderr の `Error:` ブロックと stdout の `RUN_SUMMARY` 行が増える。Task 0176 の「新しい経路は `RUN_SUMMARY` 行と stderr 報告を増やさない」という決定に反する |
| group ごとにエラー本文を Slack へ通知する（`command_group_summary` に失敗理由を載せる、または新しい通知種別を作る） | 本 issue の対象である stderr と構造化ログの帰属は解決しない。Slack には group ごとの通知がすでにあり、欠けているのは帰属ではなく失敗の理由（`groupExecutionResult.errorMsg` は設定されているが送られていない）である。これは通知のフィールド集合と、Slack へ出す本文の redaction に関わる別の問題なので、別 issue で扱う |

### 表示の各行に context を付ける

複数 group が失敗したとき、`Details:` の各行は `(group: <group>, command: <command>): <原因の文言>` の形にする。command 名が無いときは `(group: <group>): <原因の文言>` とする。括弧の中は既存の `ContextString` と同じ書式にそろえる。原因の文言は、`UserMessage` があればそれ、なければ原因の生の文言とする。

生の文言の行も同じ形にそろえる。行ごとに形が違うと、読み手が行の種類を見分ける必要が生じるためである。このとき `failed to execute group <name>: ` のラップの文言は行に含めない（context と重複するため）。原因自体の文言（例: `command <cmd> in group <group> failed: ...`）はそのまま残す。

### 複数失敗時の `GroupName` は空のまま

`ExecutionError.GroupName`・`CommandName` は、複数 group の失敗では従来どおり空にする。一覧を持たせる案は取らない。理由は次のとおり。

- これらのフィールドの利用箇所は `ContextString`（stderr・`error_message`）だけで、Slack 通知には使われない。
- 帰属は各行の context で表示されるので、外側の context に一覧を並べると同じ情報が 2 回出る。

### `Error()` の文言と `errors.Is`・`errors.AsType` の到達性は変えない

新しい型の `Error()` は、変更前の戻り値と同じ文言を返す。失敗 1 件なら `failed to execute group <name>: ...` の 1 行、2 件以上ならそれを改行で並べたもの（`errors.Join` の文言）である。dry-run の実行エラー記録など、`Error()` の文言を使う箇所の出力を変えないためである。また、`errors.Is`・`errors.AsType` が各 group の原因に届くことを保つ。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 複数 group 失敗時の各行の帰属表示

**Acceptance Criteria**:
- **AC-01**: group-1 がコマンド実行で失敗し、group-2 が出力サイズ超過（`output.CaptureError`）で失敗したとき、stderr の `Details:` に、group-2 の行として `(group: group-2, command: <group-2 の失敗 command>): ` に続けて `CaptureError` の `UserMessage` が出る。
- **AC-02**: AC-01 と同じ実行で、group-1 の行は `(group: group-1, command: <group-1 の失敗 command>): ` に続けて原因の生の文言が出て、`failed to execute group group-1: ` を含まない。
- **AC-03**: command に帰属しない失敗（例: group の展開の失敗）を含む複数 group の失敗では、その group の行は `(group: <group>): ` で始まる。
- **AC-04**: 構造化ログの `error_message` にも、AC-01〜AC-03 と同じ各行の文言が出る。
- **AC-05**: 複数 group の失敗では、外側の context（`error running commands (group: ..., command: ...)`）は付かない（`ExecutionError.GroupName`・`CommandName` は空）。

- **AC-15**: 失敗が 1 group だけで、原因が `*CommandExecutionError` ではないとき（例: group の展開の失敗）、外側の context に `group: <group>` が付く。command レベルの段階の `*GroupStageError` では `command: <command>` も付く。Details の原因の文言は変更前と同じである。

#### F-002: 型による宣言

**Acceptance Criteria**:
- **AC-06**: `executeGroups` は、1 件以上の group が失敗したとき、件数によらず、失敗 group ごとに group 名・command 名・原因を持つ専用の型を返し、group 名は `GroupSpec.Name` から設定される。
- **AC-07**: `executionErrorContext` と `formatCause` は、複数 group の失敗を専用の型（または `logging` 側で宣言したインタフェース）で判定する。本番コードに、`Unwrap() []error` の有無で複数 group の失敗を判定する分岐、およびエラー文字列から group 名・command 名を取り出す処理がない。
- **AC-08**: 専用の型ではない `errors.Join` の値を `formatCause` に渡したときは、現状どおり子ごとに 1 行ずつ、context を付けずに出る。

#### F-003: 既存挙動の維持

**Acceptance Criteria**:
- **AC-09**: 失敗が 1 件のときも 2 件以上のときも、戻り値の `Error()` の文言は変更前と同じである。
- **AC-10**: 失敗が 1 件のときも 2 件以上のときも、`errors.Is`・`errors.AsType` が各 group の原因（`*CommandExecutionError`、`output.CaptureError` など）に届く。
- **AC-11**: 失敗が 1 group だけで、原因が `*CommandExecutionError` のとき、stderr の `Details:`・構造化ログの `error_message`・`ExecutionError.GroupName`・`CommandName` は変更前と同じである。
- **AC-12**: 実行エラーの構造化ログレコードは `slack_notify=false` のままで、Slack 通知の件数・内容は変わらない。プロセスの終了コードと `RUN_SUMMARY` 行も変わらない。

#### F-004: 全体の健全性

**Acceptance Criteria**:
- **AC-13**: 各コミットの時点で `make fmt`（Go を変更した場合）・`make test`・`make lint` が通る。
- **AC-14**: 追加・変更したテストが、検証対象の挙動を壊すと失敗することを確認し、その旨をコミットメッセージに記す（CLAUDE.md「Every test must be able to fail for its stated reason」）。

## Success Criteria（要件レベル）

- 複数 group が失敗したとき、stderr と構造化ログの各行から、その行の group（と command）を他のレコードと突き合わせずに判別できる。
- 「複数 group の失敗」が型で宣言され、`errors.Join` の形からの推測がなくなる。
- 失敗が 1 件でも複数件でも同じ型で表され、1 件は複数件の特殊な場合として扱われる。
- 失敗が 1 group だけのときの報告（外側の context が増える場合を除く）、`Error()` の文言、Slack 通知、終了コードは変わらない。
