# 詳細仕様書: エラー本文を役割付きの部分として運び、部分ごとに redaction する

## 0. 本書の位置づけ

- 入力: [01_requirements.md](01_requirements.md)（`approved`）、[02_architecture.md](02_architecture.md)（`approved`）、[design_carryover.md](design_carryover.md)、[detailed_spec_carryover.md](detailed_spec_carryover.md)。
- 優先順位: 02 > 本書 > 申し送り。02 が決めた契約・不変条件・対象の範囲は変えない。本書は、申し送りが詳細仕様書に残した決定を確定し、型・シグネチャ・バイト単位の手順・ガードの判定の仕方・テストの細目を与える。
- 行番号は、特に断らない限り執筆時点のワーキングツリー（アーキテクチャ承認コミット `408b29c5`。コードは `3bb634bd` と同一）で確認したものである。
- 用語は 02 §0 に従う（構造化メッセージ・部分・役割・免除の役割・平らにする・断片・全体の検出範囲・対象の範囲・置換文字列・値全体置換）。
- 実装計画のファイル名は `04_implementation_plan.md` とする（申し送りの論点。Task 0091 の番号付けを採り、`design_carryover.md` と 01 が使う `03_implementation_plan.md` の呼び方は使わない）。
- 本書に未確定の API 名・実装保留のメモは残さない。

### 0.1 申し送りが残した決定（確定結果）

| # | 申し送りの論点 | 確定 |
|---|---|---|
| 1 | 実装計画のファイル名 | `04_implementation_plan.md`（§0） |
| 2 | `errmsg` の型の内部表現 | 申し送りの案を基に、役割の部分と原因の部分を `kind` で区別する（`indent` 欄は置かない）（§2.2） |
| 3 | 構造化メッセージを組み合わせる API | パッケージ関数 `Merge(messages ...Message) Message`。`Message` から部分の列を返す API は置かない（§2.3・§2.6） |
| 4 | 1 回だけ平らにする操作 | `Message.Freeze()`（§2.5） |
| 5 | `ExpandCommand` の `failed to create RuntimeCommand ...` の分類 | 原因は `NewRuntimeCommand` の検証エラーで `ErrUndefinedVariableDetail` を運びえない。02 §3.5.3 に従い前置き全体を 1 つの `Text`（§6.5） |
| 6 | `expansion.go` の can-carry 判定 | `ProcessVars`／`ProcessEnv`／`ExpandString` を通る経路は carry 可。`env_import`・`Runtime*` 作成・`EvalSymlinks` は carry 不可（§6.5） |
| 7 | `%w` を 2 つ持つ書式 | executor に `killAfterCancelError` を宣言（§7.2） |
| 8 | 複合の型の共有 | executor の `errors.Join` 置き換えは `errmsg.Join`／`errmsg.JoinedError`。`cancelledRunError` は中断を宣言する専用の型として別に置く（§5.3・§7.4） |
| 9 | 凍結する位置 | 記録の組み立て（`preExecutionRecordParams`・`HandleExecutionError`）で凍結する（§4.3） |
| 10 | 整形のバイトの検査 | `IndentedCause(error)` のシグネチャをコンパイル時に固定する。字下げを渡す呼び出しはコンパイルできないので、AST の検査は置かない（§9.9） |
| 11 | `Config.Placeholder()` | 残す。本番の呼び出しが `internal/runner/base/security/environment_validation.go:21` にある（§3.5） |
| 12 | `NewConfig` の検証・コンパイル失敗の分岐 | 残す。既定の規則が不正な形に編集されたときの fail-closed の境界である。削除するテストによる `go tool cover -func` の差分は §10.7 に記録する（§3.5） |
| 13 | `contextParts` の名前 | `(*ExecutionError).contextParts` に確定（§4.2・§9.1） |

## 1. 変更対象ファイル一覧

| ファイル | 変更種別 | 概要 |
|---|---|---|
| `internal/errmsg/errmsg.go` | 新規 | 役割・部分・構造化メッセージ・断片・要約文・`Structured`・`Error`・`JoinedError`・平らにする処理・字下げの再現 |
| `internal/errmsg/errmsg_test.go` | 新規 | 平らにする契約、シグネチャの固定、`String()` と `Error()` の一致、nil・ゼロ値 |
| `internal/errmsg/errmsg_guard_test.go` | 新規 | `Const`、免除の役割の位置、`Part` の非公開、部分を返す関数、文言と構造の一致（`//go:build test`） |
| `internal/redaction/ranges.go` | 新規 | `byteRange`、`redactedRanges`、段ごとの置き換え範囲の導出 |
| `internal/redaction/message.go` | 新規 | `RedactMessage`、`redactSegments`、`redactMessageAttribute` |
| `internal/redaction/ranges_test.go` | 新規 | 差分テスト（種とファジング）、幅 0 の範囲、段の重なり |
| `internal/redaction/message_test.go` | 新規 | AC-01〜04・07・36〜38、実行時の検査、性能ベンチマーク |
| `internal/redaction/redaction_guard_test.go` | 新規 | AC-25（`//go:build test`） |
| `internal/redaction/redactor.go` | 変更 | 段の規則を範囲の導出と共有する共通化、`redactLogAttributeWithContext`・`RedactLogAttribute` の分岐、`WithPlaceholder`・`WithAdditionalKeyValuePatterns` の削除 |
| `internal/redaction/value_detector.go` | 変更 | 各規則の置き換え範囲の導出をパッケージ内で共有できる形にする |
| `internal/redaction/errors.go` | 変更 | `ErrMessageFlattenPanic`・`ErrMessageRangeMismatch` |
| `internal/redaction/redactor_test.go` | 変更 | 削除・書き換える既存テスト（§10.7） |
| `internal/logging/pre_execution_error.go` | 変更 | `PreExecutionError.Message` の型、`DetailMessage`、記録と凍結、`errorRecordParams.errorMsg` の型 |
| `internal/logging/execution_error.go` | 変更 | `ExecutionError.Message` の型、`ReportMessage`、`contextParts` |
| `internal/logging/pre_execution_error_test.go` ほか | 変更 | リテラルの機械的な書き換え、原因 1 回のテスト（§10.7） |
| `internal/logging/notification_contract_guard_test.go` | 変更 | `Message` の新しい型に合わせる（必要な範囲のみ） |
| `internal/runner/group_executor.go` | 変更 | `CommandExecutionError` の `Structured`、各ラップの構造化 |
| `internal/runner/group_errors.go` | 変更 | `GroupError`・`GroupErrors` の `Structured` |
| `internal/runner/group_stage.go` | 変更 | `GroupStageError` の `Structured`、段階の定義表の要約文を `Summary` に |
| `internal/runner/runner.go` | 変更 | `cancelledRunError` |
| `internal/runner/group_stage_test.go`・`group_errors_test.go`・`group_executor_test.go`・`runner_test.go`・`multi_group_error_integration_test.go` | 変更 | 型・文言の確認（§10.7） |
| `internal/runner/wrap_guard_test.go` | 新規 | AC-41（`//go:build test`） |
| `internal/runner/config/errors.go` | 変更 | `Level`・`Field` の型と `parts()`、`ErrUndefinedVariableDetail` の `Structured` |
| `internal/runner/config/expansion.go` | 変更 | 引数の型、ラップの構造化 |
| `internal/runner/config/validation.go` | 変更 | `validateVariableName` の引数の型 |
| `internal/runner/config/template_expansion.go` | 変更 | `validateGlobalOnly`・`validateFieldVars` の引数の型 |
| `internal/runner/config/*_test.go` | 変更 | `Level`・`Field` の `String()` 比較（§10.7） |
| `internal/runner/resource/normal_manager.go` | 変更 | ラップの構造化（パスは `Path`） |
| `internal/runner/resource/dryrun_manager.go` | 変更 | ラップの構造化（パスは `Path`） |
| `internal/runner/base/executor/tempdir_manager.go` | 変更 | 一時ディレクトリの 2 つのラップ |
| `internal/runner/base/executor/executor.go` | 変更 | `Validate`・`validatePrivilegedCommand`・`executeNormal`・`executeWithUserGroup` のラップ |
| `internal/runner/base/executor/command_lifecycle.go` | 変更 | `runCommand`・`reportStartFailure`・`superviseCommand`・`killChild`・`killOutcome`、`killAfterCancelError` |
| `internal/runner/base/privilege/errors.go` | 変更 | `(*Error).Error` と `StructuredMessage` |
| `internal/runner/base/privilege/unix.go` | 変更 | `performElevation` のラップ |
| `cmd/runner/main.go` | 変更 | 4 か所の `Err` への付け替え、残り 11 か所の要約文の型 |
| `internal/runner/bootstrap/config.go`・`environment.go` | 変更 | 要約文の型 |
| `internal/runner/runerrors/pre_execution.go` | 変更 | 要約文の型 |
| `cmd/runner/main_test.go` ほか | 変更 | リテラル・原因の確認方法（§10.7） |
| `docs/dev/architecture_design/security-architecture.ja.md`・`.md` | 変更 | 5.4 節（AC-26） |
| `docs/dev/developer_guide/package_reference.md` | 変更 | `internal/errmsg` の追加 |

## 2. `internal/errmsg`（新規）

### 2.1 パッケージ方針

- 標準ライブラリだけを import する末端のパッケージにする（02 §2.2）。`internal/identifier` と同じ位置づけである。
- 役割は、このパッケージの構築関数でしか決められない。`Part` の欄は非公開にする。
- redaction の側は `Role` を読むだけで、`Role` の値を作らない。

### 2.2 型（完全なコード）

```go
package errmsg

import (
    "io/fs"
    "log/slog"
    "strings"
)

// Role declares how redaction treats a segment. The zero value is RoleText.
type Role int

const (
    RoleText Role = iota
    RoleConstant
    RoleIdentifier
    RolePath
)

// causeKind declares how a cause part is flattened. The zero value is a
// plain cause.
type causeKind int

const (
    causePlain causeKind = iota
    causePathError
    causeIndented
)

// partKind declares whether a Part carries a role and text, or a cause.
// The zero value is a role part.
type partKind int

const (
    partRole partKind = iota
    partCause
)

// Part is one element of a Message. Its fields are unexported, so a part
// can only be built by the constructors in this package.
type Part struct {
    kind      partKind
    role      Role
    text      string
    cause     error
    causeKind causeKind
}

// Message is an error body as a sequence of parts.
type Message struct {
    parts []Part
}

// Segment is one flattened element: a role and its unredacted text.
type Segment struct {
    Role Role
    Text string
}

// Segments is the result of flattening a Message.
type Segments []Segment

// Summary is the summary line of a report: a constant or free text only.
type Summary struct {
    part Part
}

// Error is the general-purpose structured error used in place of
// fmt.Errorf on the paths in scope.
type Error struct {
    msg   Message
    cause error
}

// Structured is implemented by errors whose body is a Message. Error()
// must return StructuredMessage().String().
type Structured interface {
    error
    StructuredMessage() Message
}

var _ Structured = (*Error)(nil)
var _ slog.LogValuer = Message{}
```

### 2.3 構築関数・メソッド（確定した API）

```go
func Const(text string) Part       // text must be a constant expression (AST guard)
func Ident(name string) Part
func Path(path string) Part
func Text(text string) Part
func Cause(err error) Part
func PathErrorCause(err error) Part
func IndentedCause(err error) Part

func NewMessage(parts ...Part) Message
func Merge(messages ...Message) Message
func (m Message) String() string
func (m Message) LogValue() slog.Value
func (m Message) Segments() Segments
func (m Message) Freeze() Message

func ConstSummary(text string) Summary // text must be a constant expression (AST guard)
func TextSummary(text string) Summary
func (s Summary) String() string
func (s Summary) Part() Part

func NewError(parts ...Part) *Error // panics unless exactly one part is a cause part with a non-nil cause
func (e *Error) Error() string      // returns e.StructuredMessage().String()
func (e *Error) Unwrap() error
func (e *Error) StructuredMessage() Message

func Join(errs ...error) error // errors.Join semantics with StructuredMessage rendering
type JoinedError struct{ errs []error }
func (e *JoinedError) Error() string
func (e *JoinedError) Unwrap() []error
func (e *JoinedError) StructuredMessage() Message
```

契約（02 §3.1.1 を具体化したもの）:

1. `Const`・`ConstSummary` の引数は定数式でなければならない。型では表せないので §9.3 の AST ガードで固定する。errmsg 自身が作る `Constant`（`PathErrorCause` の `" "`・`": "`、字下げ）は、パッケージ内の文字列リテラルから作る。
2. `NewMessage` は部分をそのまま保持する。`Merge` は各 `Message` の部分を順につないだ 1 つの `Message` を返す。どちらも役割を選ぶ経路にはならない（部分は errmsg の構築関数でしか作れない）。`Message` から部分の列を取り出す公開 API は置かない。
3. `NewError` は、原因の部分がちょうど 1 つで、その原因が nil でないことを要求する。満たさなければ `panic` する。`Unwrap()` はその原因を返す。
4. `Cause(nil)` は panic しない。平らにすると `<nil>`（`fmt` の `%v` と同じ文字列）を 1 つの `Text` の断片にする。`Message` は複数の原因の部分を持てる（`DetailMessage`・`JoinedError`）。
5. `String()` は redaction 前の描画である。`LogValue()` は `String()` を `slog.StringValue` で返す（AC-21）。
6. `Summary` は `Constant` か `Text` の部分だけを持つ。`Part()` はその 1 つの部分を返す。ゼロ値は空文字列を描画する。
7. `Freeze()` は `Segments()` を 1 回だけ評価し、各断片の役割と文字列をそのまま `Part` に移した `Message` を返す。返す `Message` は原因を持たないので、以後の `String()`・`Segments()` は原因の `Error()` を呼ばない。
8. `Text` の断片の値全体置換は断片ごとである（AC-36）。これは redaction の側の規則であり、errmsg は関与しない。

`IndentedCause` のシグネチャは §9.9 のコンパイル時の固定の対象である。

### 2.4 平らにする処理（`Segments`）

`Segments()` は部分の列を次の規則で断片の列にする。

1. `kind == partRole` の部分は、そのまま 1 つの `Segment{Role: p.role, Text: p.text}` にする。役割がどの値でも（ゼロ値・範囲外でも）そのまま移す。
2. `kind == partCause` の部分は `causeKind` で分岐する。`Cause(nil)` は `kind == partCause` なので `Text("")`・`Part{}` と区別でき、平らにすると `<nil>` になる。
   - `causePlain`: 原因が `Structured` を実装していれば `cause.StructuredMessage().Segments()` を展開する。判定は原因の直接の動的な型で行い、`errors.As` で連鎖の奥を探さない（02 §3.1.3。奥の型を探すと途中のラップの文言が消え、`String()` が `Error()` と一致しなくなる）。実装していなければ `Segment{RoleText, cause.Error()}` を 1 つ追加する。原因が nil なら `Segment{RoleText, "<nil>"}` にする。
   - `causePathError`: 原因が `*fs.PathError`（型アサーション）なら、`Text(pe.Op)`・`Const(" ")`・`Path(pe.Path)`・`Const(": ")` と、`pe.Err` の部分（`causePlain` と同じ規則）を展開する。`pe.Err` が nil なら `<nil>` になる（`(*fs.PathError).Error()` は nil で panic するが、この書式は `fmt` の `%v` と同じに扱う。`os` は Err を必ず入れるので実務では起きない）。`*fs.PathError` でなければ `causePlain` と同じにする。この並びは `(*fs.PathError).Error()` と同じ文言になる。
   - `causeIndented`: まず `causePlain` と同じ規則で原因を平らにし、その断片の列に §2.4.1 の整形を施す。役割は変えない。
3. 展開の深さに上限を置かない。自身を原因に持つ連鎖は、変更前に `fmt` の `%v` で表示したときと同じくスタックがあふれる。これは現状のままであり、後退ではない（02 §3.1.3）。

#### 2.4.1 字下げの再現（`IndentedCause`）

`GroupError.Error()`（`internal/runner/group_errors.go:31-34`）は、原因の文言の末尾の `"\r\n"` の並びを除き、残りの改行の後に 2 つの空白を入れる。`IndentedCause` はこの整形を原因の断片の列の上で再現する。

1. 原因を `causePlain` と同じ規則で平らにする。
2. 末尾の除去: 断片を末尾から順に見る。
   - 各断片の `Text` を `strings.TrimRight(text, "\r\n")` した結果で置き換える。
   - 結果が空で、元の `Text` も空なら、その断片はそのまま残し、前の断片に進む（元から空の断片は連結に寄与しない。これが、末尾の空の断片の前にある改行を引き続き除くために要る）。
   - 結果が空で、元の `Text` が空でなければ（改行だけの断片）、その断片を除き、前の断片に進む。
   - 結果が空でなければ、その断片で止める。
3. 字下げ: 残った各断片の `Text` を `strings.ReplaceAll(text, "\n", "\n  ")` にする。役割は変えない。
4. 1〜3 の結果の連結は `GroupError.Error()` の整形結果と一致する。`Constant` の断片に字下げが入るのは、errmsg が持つ固定の空白と元の断片の改行だけであり、呼び出し側のバイトは入らない。§9.9 のガードで守る。

例: 断片の列が `[Text("cause\n"), Text("")]` のとき、手順 2 の結果は `[Text("cause"), Text("")]` になり、その連結は `GroupError.Error()` の整形結果と一致する。

### 2.5 `Freeze`

```go
func (m Message) Freeze() Message {
    return NewMessage(partsFromSegments(m.Segments())...)
}
```

`partsFromSegments` は各断片を `Part{role: s.Role, text: s.Text}` にする非公開の補助である。凍結した `Message` は、平らにした結果と同じ文字列を `String()` が返し、`Segments()` も同じ役割と文字列を返す。`Freeze` は errmsg の中にあるので、断片の役割をそのまま部分に移しても、役割を errmsg の外で選ぶ経路にはならない（02 §3.1.1 の契約 1）。

### 2.6 `Merge`

```go
func Merge(messages ...Message) Message {
    parts := make([]Part, 0, len(messages))
    for _, m := range messages {
        parts = append(parts, m.parts...)
    }
    return Message{parts: parts}
}
```

`GroupErrors.StructuredMessage` は `Merge` を使い、各 `GroupError` の構造化メッセージを `NewMessage(Const("\n"))` で区切ってつなぐ（§5.1）。平らにせずにつなぐので、各 `GroupError` が宣言した `Identifier`・`Path` の断片は保たれる。

### 2.7 `Join`

```go
func Join(errs ...error) error {
    kept := make([]error, 0, len(errs))
    for _, err := range errs {
        if err != nil {
            kept = append(kept, err)
        }
    }
    if len(kept) == 0 {
        return nil
    }
    return &JoinedError{errs: kept}
}

func (e *JoinedError) Error() string { return e.StructuredMessage().String() }
func (e *JoinedError) Unwrap() []error { return e.errs }
func (e *JoinedError) StructuredMessage() Message {
    parts := make([]Part, 0, max(0, 2*len(e.errs)-1))
    for i, err := range e.errs {
        if i > 0 {
            parts = append(parts, Const("\n"))
        }
        parts = append(parts, Cause(err))
    }
    return NewMessage(parts...)
}
```

- `errors.Join` と同じく nil の子を除き、すべて nil なら nil を返す。文言は子の `Error()` を改行でつないだもので、`errors.Join` と一致する（AC-18）。
- `Unwrap() []error` を持つので、`errors.Is`・`errors.AsType` はすべての子に届く。
- 子が `Structured` なら平らにするときに構造を展開する。子の `String()` は子の `Error()` と一致する（§9.5 のガード）ので、文言は `errors.Join` と変わらない。
- 具体的な型に `Unwrap() []error` を宣言することは、0177 の形の判定を禁じるガードと両立する（02 §3.4.3）。ガードが禁じているのは、形によって子を扱い分ける判定のほうである。
- ゼロ値の `JoinedError`（`len(e.errs) == 0`）は空文字列を返し、panic しない（§2.8 のゼロ値の契約）。`Join` は空の `JoinedError` を返さないが、公開の型なのでゼロ値を許す。

### 2.8 panic とゼロ値

- `Message`・`Segment`・`Summary` のゼロ値は panic しない。`Message{}` は空文字列、`Summary{}` は空文字列、`Segment{}` は `RoleText` と空文字列を表す。
- `Part` のゼロ値は `role == RoleText`・`text == ""` の部分であり、平らにすると空の `Text` の断片 1 つになる。
- `NewError` の前提を満たさない構築は panic する（呼び出し側の誤り。記録の時点では起きない）。
- `Cause(nil)`・`PathErrorCause(nil)`・`IndentedCause(nil)` は panic しない。`PathErrorCause(nil)` は `Cause(nil)` と同じに扱う。

### 2.9 errmsg のガード

`internal/errmsg/errmsg_guard_test.go`（`//go:build test`）に、§9.2〜§9.9 のガードのうち errmsg に関するものを置く。免除の役割の許可位置は §9.4 の表、`Const` は §9.3、`Part` の非公開と部分を返す関数は §9.8、文言と構造の一致は §9.5、整形は §9.9 である。自己テストは §9.10 に従う。

## 3. `internal/redaction`（変更）

### 3.1 置き換える範囲を返す関数 `redactedRanges`

```go
// byteRange is a half-open byte range [start, end) of the original text.
type byteRange struct {
    start, end int
}

// redactedRanges returns the ranges of text that RedactText replaces, in
// the coordinates of text, sorted and non-overlapping.
func (c *Config) redactedRanges(text string) []byteRange
```

- 前提条件: 規則の集合は既定の規則（と、検証済みの `WithWebhookHost` の規則）に固定し、置換文字列は定数 `DefaultPlaceholder` である。`WithPlaceholder` と `WithAdditionalKeyValuePatterns` は削除する（§3.5）。規則を足したり置換文字列を変えたりすると、置換文字列が後の段の規則に再び一致することがあり、正しさの義務が成り立たなくなる。
- `c.validated` が偽のときはこの関数を呼ばない。`RedactMessage` が手前で抑止する（§3.2）。
- 正しさの義務: 任意の入力について、返した各範囲を 1 つの置換文字列に置き換えた結果が、変更していない `RedactText` の出力と一致すること。§10.2 の差分テストで固定し、`RedactMessage` の実行時の検査を fail-closed の最後の備えとして残す。

#### 3.1.1 段の順序と規則の共有

`RedactText`（`redactor.go:272-307`）と同じ段を同じ順序で適用する。

1. `maskPrivateKeyBlocks`（`pemPrivate` の全体一致、次に `pemPrivateUnterminated` の全体一致。`value_detector.go:183-202`）
2. `compiled` の各規則（`DefaultKeyValuePatterns` の順。`redactor.go:294-296`）
3. `ValueDetector.Mask` の各規則（`awsKeyID` から `jwt` までの順、最後に `webhookHostURL`。`value_detector.go:147-181`）

規則の写しを作らない。`RedactText` の実装（`RedactText`・`compiledPattern.apply`・`replaceKeyValueMatches`・`ValueDetector.Mask`）は変えない（02 §1.1・§3.2.1）。正規表現と置換テンプレートは `Config`・`compiledPattern`・`valueDetectorPatterns` にあるものを参照する。範囲の導出は次で行う。

- `redactor.go` の `compiledPattern` に、その規則の「置き換える範囲」を返す非公開のメソッド（`replacedSpans(text string) []byteRange`）を追加する。実装は `re`・`valueGroups`（keyed-value）と、置換テンプレートが再び出力するグループの添字（§3.1.2）だけを使う。`compilePattern` で置換テンプレートから残すグループの添字を求め、`compiledPattern` に持たせてよい。`apply` は変えない。
- `value_detector.go` に、各規則の「置き換える範囲」を返す非公開の関数を追加する。`valueDetectorPatterns` の正規表現と `Mask` の置換テンプレートと同じ残す範囲を使う。この変更は `internal/redaction` のパッケージの中に留める。`Mask` は変えない。
- 段の順序（PEM、key-value、値形式）は `redactedRanges` の中に書き、`RedactText` の順序と同じにする。`RedactText` の本体は変えないので順序は 2 か所に現れるが、規則そのものは共有する。順序と規則の一致は §10.2 の差分テストと §3.2 の実行時の検査で固定する。
- 範囲の列を 1 つの置換文字列で置き換える非公開の `replaceSpans(text string, spans []byteRange, placeholder string) string` を `redactor.go` に置き、`redactedRanges` の確認と `RedactMessage` の実行時の検査で使う。`RedactText` は使わない。
- `RedactText` の観測できる挙動は変えない。既存の `RedactText` のテストがその pin である。

#### 3.1.2 規則ごとの置き換え範囲

残す範囲は、各段の置換のテンプレートが再び出力する範囲とする。

| 規則 | 置き換える範囲 | 残す範囲 |
|---|---|---|
| `pemPrivate`・`pemPrivateUnterminated`（`value_detector.go:35,41`） | 一致の全体 | なし |
| `PatternKindHeaderValue`（`redactor.go:104-107`） | `${1}${2}${3}` の後ろから一致の末尾まで | グループ 1（キー）・2（区切り）・3（`bearer `/`basic ` の接頭辞。空もありうる） |
| `PatternKindNextToken`（`redactor.go:108-111`） | グループ 1 の後ろから一致の末尾まで | グループ 1（リテラル） |
| `PatternKindKeyedValue`（`redactor.go:112-114`） | 値のサブマッチ（`dqValue`・`sqValue`・`sepValue`・`adjValue` の参加したもの）。空の値では幅 0 | 一致のうち値のサブマッチ以外の全体（境界・キー・区切り・引用符） |
| `awsKeyID`・`githubToken`・`slackToken`（`value_detector.go:27-29`） | 一致の全体 | なし |
| `gcpSAKey`（`:34`） | グループ 1 と 2 の間 | グループ 1（`"private_key_id":"`）・2（`"`） |
| `bearerToken`（`:42`） | グループ 1 の後ろから一致の末尾まで | グループ 1（`Bearer` と空白） |
| `urlCred`（`:43`） | グループ 1 の後ろから一致の末尾の `@` の手前まで | グループ 1（`scheme://`）と、一致の最後の 1 バイト（`@`） |
| `githubPAT`・`slackPrefixToken`（`:47,51`） | 一致の全体 | なし |
| `jwt`（`:58`） | 一致の先頭からグループ 1 の手前まで | グループ 1（トークンの次の 1 文字、または末尾） |
| `webhookHostURL`（`:125`） | グループ 1 の後ろから一致の末尾まで | グループ 1（`https://ホスト[:ポート]/`） |

#### 3.1.3 段の重なりと座標

後の段は、前の段が置換文字列を入れた後の文字列に対して一致を探す（`RedactText` と同じ）。範囲は元の文字列の座標で返す。そのため、現在の描画を「元の範囲」と「置換文字列の断片」の列（ピーステーブル）として持ち、段ごとに次を行う。

1. 現在のピーステーブルを描画した文字列 `T` を得る。各ピースは、元の文字列の半開区間（元ピース）か、前の段が入れた置換文字列とその由来する元の区間（置換ピース）である。初期状態は `[0, len(text))` の元ピース 1 つである。
2. その段の各一致について、§3.1.2 の置き換える範囲（`T` の座標）を得る。置き換える範囲が空なら挿入点（幅 0 の範囲）として扱う。
3. 置き換える範囲を元の座標に写す。範囲が重なる各ピースの由来する元の区間を合併し、その合併を 1 つの置換ピースにする。置換ピースに一部でも重なった置換ピースは、範囲の全体を置き換える（置換文字列は分けない。02 §3.2.1 の「段の重なり」）。残す範囲が置換ピースに重なっても、その置換ピースは置き換えたままにする。
4. それ以外の部分は、ピースを（必要なら分割して）そのまま写す。置換ピースを一部分だけ写すことになった場合は、その部分を元の座標を持たない文字のピースとして写す（元の範囲は置き換わったままとするため。範囲には数えない）。
5. 1〜4 を段の順に繰り返す。最後に残った置換ピースの元の区間を、現れる順に `ranges` とする。ピーステーブルは元の文字列の順序を保つので、`ranges` は昇順で重ならない。挿入点は `start == end` の範囲として返す。
6. 表し方は、一致の数に比例する大きさの区間の列とする。文字列の長さに比例する表は作らない。

#### 3.1.4 幅が 0 の範囲

`password=""` では、値のサブマッチが引用符の間の空区間になり、`RedactText` は `password="[REDACTED]"` を返す（`redactor_test.go:404` が固定している）。`redactedRanges` はこれを `start == end` の範囲（挿入点）として返す。挿入点は捨てずに返し、「sorted and non-overlapping」の定義に含める。同じ位置に挿入点と幅のある範囲があるときの並びは、`start` の昇順、`start` が等しければ `end` の昇順（挿入点が先）とする。`RedactMessage` の描画での扱いは §3.2 の手順 6 と 7 である。

### 3.2 `RedactMessage` と `redactSegments`

```go
// RedactMessage renders m with per-segment redaction and the cross-boundary
// contract, using this Config's rules and replacement string. It returns an
// error when flattening m panics or when the runtime range check fails.
func (c *Config) RedactMessage(m errmsg.Message) (string, error)

// redactSegments applies steps 2-7 below to an already flattened segment list.
func (c *Config) redactSegments(segs errmsg.Segments) (string, error)
```

`RedactMessage` は、`c.validated` が偽なら `(RedactionFailurePlaceholder, nil)` を返す（`RedactText` と同じ抑止）。そうでなければ `m.Segments()` を 1 回だけ呼び、`redactSegments` に渡す。`Segments()` の呼び出しは `recover` で囲み、panic したら `*ErrMessageFlattenPanic`（§3.4）を返す。断片の列を得るのは 1 回だけであり、原因の `Error()` を 2 回呼ばない（02 §3.2.2 (a)）。AC-38 のテストは `redactSegments` に範囲外の役割を持つ断片を直接与える。

`redactSegments` の手順:

1. `S` を各断片の文字列の連結とし、各断片の範囲 `[start_i, end_i)` を求める。`S` と範囲はこの 1 回の断片列だけから作る。
2. 各断片の `out_i` を求める。
   - `RoleIdentifier`・`RoleConstant`: `out_i = seg.Text`。
   - `RolePath`: `out_i = c.RedactText(seg.Text)`。
   - `default`（`RoleText` と、どの役割にも当たらない値）: `r := c.RedactText(seg.Text)` とし、`r == seg.Text` かつ `c.patterns.IsSensitiveValue(seg.Text)` なら `out_i = c.placeholder`、そうでなければ `out_i = r`。値全体置換は断片ごとに判定する（AC-36）。
3. `ranges := c.redactedRanges(S)` を求め、実行時の検査を行う。`ranges` の各範囲を順に 1 つの置換文字列で置き換えた結果（挿入点はその位置に置換文字列を挿入する）が `c.RedactText(S)` と一致しなければ、失敗として `*ErrMessageRangeMismatch`（§3.4）を返す。失敗のときは途中の描画を出さない。
4. 境界の影響を受ける断片を決める。`Identifier` の断片は常に受けない。それ以外の断片 `i` について、`L_i` を手順 2 でその断片が置き換えたバイトの集合とする（値全体置換なら全バイト、`RedactText` の結果が変わったなら `c.redactedRanges(seg.Text)` の範囲のバイト、変わらなければ空。断片の座標から `S` の座標に写して持つ）。断片 `i` が受けるのは、次のいずれかのときである。
   - `ranges` の幅のある範囲 `[s, e)` のうち、断片 `i` のバイトと重なり、かつその範囲の全体が `L_i` に含まれないものが 1 つ以上ある。範囲が断片の外（隣の断片）へ続く場合も、この条件でその断片を影響ありにする。隣の断片のバイトと連結した 1 つの極大の区間（手順 6）にまとめるためである。
   - `ranges` の挿入点 `p` が `start_i <= p <= end_i` を満たし、次の付与規則でその断片に付き、かつ `p` が `L_i` に含まれない（幅のある置き換えの内部でも、同じ位置の挿入点でもない）。`p` が断片の内部（`start_i < p < end_i`）ならその断片に付く。`p` が断片の境界にあるときは、次の断片（`p` が末尾なら直前の断片）が `Identifier` でなければそれに付け、そうでなければ前の断片が `Identifier` でなければそれに付ける。両隣とも `Identifier` なら、その境界に置換文字列を出す（手順 7）。
5. 境界の影響を受けない断片は、`out_i` をそのまま出す。
6. 境界の影響を受ける断片では、次のバイトを「隠すバイト」とする。
   - `ranges` に含まれるバイト（幅のある範囲のバイトと、付与規則でその断片に付いた挿入点）。
   - `L_i` のバイト。`RolePath`・`RoleText`・範囲外の役割の断片にだけ起こりうる。
   - `Identifier` の断片のバイトは隠さない（AC-37）。
   隠すバイトの連続する区間は、極大の区間ごとに 1 つの置換文字列にする。区間は隣り合う境界の影響を受ける断片をまたいでよい（`[a, b)` と `[b, c)` は `[a, c)` にまとめる）。挿入点は、隣接する隠すバイトがあればそれと合わせて 1 つの極大の区間にし、無ければ幅 0 の区間として 1 つの置換文字列を出す。同じ位置の複数の挿入点は 1 つにまとめる。
7. 描画は、断片と極大区間を位置の順にたどって行う。手順 5 の断片は `out_i` を出す。手順 6 の断片は `seg.Text` のうち隠さないバイトをそのまま出し、極大区間の開始位置で 1 つの置換文字列を出して区間のバイトを飛ばす。区間が断片の境界をまたぐときは、開始位置を含む断片でだけ置換文字列を出し、続く断片では区間の残りを飛ばす。両隣とも `Identifier` の断片の間に付いた挿入点は、その境界（次の断片の直前、末尾なら文字列の最後）に置換文字列を出す。`Identifier` の断片のバイトは常にそのまま出す。
8. 手順 5 を採る理由: 境界をまたぐ検出が無い断片まで隠すバイトによる描画を使うと、置換文字列の数が断片単独の `RedactText` の結果と変わることがある（隣り合う 2 つの置き換えが 1 つにまとまるなど）。AC-04・AC-11 は、境界をまたぐ検出が無い断片では断片単独の結果と同じになることを求めている。

手順 4・6・7 の例（規範ではない）:

- `Constant("password=\"")`・`Text("")`・`Constant("\"")` の 3 つの断片。`ranges` は 2 つの `Constant` の境界（空の `Text` の位置）の挿入点 1 つ。`Text` の断片が影響を受け、隠すバイトは無い。出力は `password="[REDACTED]"` で、`RedactText("password=\"\"")` と同じになる。
- `Identifier("AKIAIOSFODNN7")`・`Text("EXAMPLE")`。`ranges` は値形式の一致の全体。`Text` の断片が影響を受け、その全バイトが隠れる。出力は `AKIAIOSFODNN7[REDACTED]`（AC-37 の例）。
- `Identifier("AKIAIOSF")`・`Identifier("ODNN7EXAMPLE")`。`ranges` は 2 つの `Identifier` のバイトにまたがるが、どちらの断片も影響を受けない。出力は元のままで、何も置き換えない（AC-37 の例）。
- `Constant("Bearer ")`・`Text("opaque-credential")`。`ranges` は `Text` のバイトだけ（`Bearer ` は残す範囲）。`Text` が影響を受け、全バイトが隠れる。出力は `Bearer [REDACTED]`（AC-37 の例）。
- `Identifier("AKIAIOSFODNN7")` の直後に `Text("EXAMPLE")` が続き、その次の `Text` にも値形式の一致の続きがある場合、隠すバイトは `Identifier` の前後で分かれ、`Identifier` のバイトはそのまま出る（`…[REDACTED]AKIAIOSFODNN7[REDACTED]…`）。範囲の中で `Identifier` 以外のバイトが連続する区間ごとに 1 つの置換文字列になる。
- `Text("password=secret")` の直後に `Text("suffix")` が続き、全体の検出範囲が値 `secretsuffix` を 1 つの区間として覆う場合。断片 1 は `L_1`（`secret`）に加えて隣の断片へ続く範囲にも触れるので影響を受け、`L_1` と範囲のバイトは連結した 1 つの極大区間になる。出力は `password=[REDACTED]`（置換文字列は 1 つ）。

### 3.3 `RedactingHandler`・`RedactLogAttribute` の分岐

判定と描画は 1 つの補助関数にまとめる。

```go
// redactMessageAttribute returns the attribute to use when value carries
// exactly an errmsg.Message. handled is false when it does not. On failure the
// attribute carries RedactionFailurePlaceholder and collector (when non-nil)
// records the error.
func (c *Config) redactMessageAttribute(key string, value slog.Value, collector ErrorCollector) (slog.Attr, bool)
```

- 分岐の位置: `RedactingHandler.redactLogAttributeWithContext`（`redactor.go:799`）では、宣言済みの識別子の判定（`declaredIdentifier`、`:315`）の後、`switch value.Kind()` の前に置く。`Config.RedactLogAttribute`（`:330`）でも宣言済みの識別子の判定の後に同じ分岐を置く。
- 判定は値の動的な型だけで行う。`value.Any().(errmsg.Message)` が成功したときだけ `handled` を真にする（exact の `errmsg.Message` の分岐は両方の関数に置く）。属性名や値の内容は見ない。
- `RedactingHandler.redactLogAttributeWithContext`: exact の `errmsg.Message` 以外の `LogValuer`（`*errmsg.Message` を含む）は、既存の `processLogValuer` が `LogValue()` を解決し、`String()` の文字列として全体の redaction を受ける（fail-closed）。
- `Config.RedactLogAttribute`: exact の `errmsg.Message` の分岐の後、`slog.KindLogValuer` の値は `RedactionFailurePlaceholder` にする（fail-closed）。この公開の関数は `LogValue()` を解決する経路と、panic の回復・再帰の深さの管理を持たない。`LogValue()` が panic する値や自身を返す値を安全に扱えないので、解決せずに値をそのまま後段へ渡さない。
- 分岐は属性名による判定（`IsSensitiveKey`）より後に行う。機密を示す属性名の下では、構造化メッセージも値ごと置換する（AC-07）。
- `RedactingHandler` は `collector` に `r.errorCollector` を渡す。失敗は既存の `ErrLogValuePanic` と同じく型付きのエラーとして記録され、終了時の報告（`ShutdownReporter`）に現れる。`Config.RedactLogAttribute` は `collector` に nil を渡すので、失敗のときに値を `RedactionFailurePlaceholder` にするだけである（02 §3.2.3）。
- `RedactLogAttribute` は本番のコードから呼ばれていない（`:371` の自身の再帰だけ）が、公開の関数なので分岐を置く。置かないと、この関数に構造化メッセージを渡したときに `LogValue()` の redaction 前の文字列がそのまま出る。
- `RedactingHandler.Handle` と `WithAttrs` は既存の経路で `redactLogAttributeWithContext` を通るので、個別の変更は要らない。後段のハンドラが受け取る `error_message` は文字列になる（AC-20）。

### 3.4 失敗の型

`internal/redaction/errors.go` に 2 つ追加する。

```go
// ErrMessageFlattenPanic reports that flattening an errmsg.Message panicked.
type ErrMessageFlattenPanic struct {
    PanicValue any
    StackTrace string
}

func (e *ErrMessageFlattenPanic) Error() string

// ErrMessageRangeMismatch reports that the ranges from redactedRanges do not
// reproduce RedactText when applied to the message's unredacted rendering.
// It deliberately carries no text: the rendering may contain secrets.
type ErrMessageRangeMismatch struct {
    RangeCount int
}

func (e *ErrMessageRangeMismatch) Error() string
```

どちらも秘密を含みうる文字列を持たない。記録する側（`ErrorCollector`）と終了時の報告の文言に、本文や範囲の内容を出さないためである。

### 3.5 オプションの削除

- `WithPlaceholder`（`redactor.go:155-159`）: 本番の呼び出しは無い。削除する。`Config.placeholder` は `NewConfig` が `DefaultPlaceholder` を入れるだけになり、置換文字列は定数に固定される。
- `Config.Placeholder()`（`:64-66`）: 残す。本番の呼び出しが `internal/runner/base/security/environment_validation.go:21` にあり、`WithPlaceholder` の削除後も使われる。
- `WithAdditionalKeyValuePatterns`（`:169-173`）: 本番の呼び出しは無い。削除する。`Config.keyValuePatterns` は `DefaultKeyValuePatterns()` だけになる。
- `NewConfig`（`:200-236`）のパターンの検証とコンパイルの失敗の分岐は残す。既定の規則が将来不正な形に編集されたときに、fail-open（何にも一致しない規則として素通りさせる）ではなく構築時に拒否する境界である。削除するテストで届かなくなる分岐の `go tool cover -func` の差分は §10.7 に記録する。

### 3.6 性能

- `RedactMessage` は、2 つのレコードの `error_message` にだけ使う。1 回の実行で記録されるのは、group 実行前段の失敗ごとに 1 件と、最終の実行エラー 1 件である。
- 予算は、100 group の失敗を連結した最終の実行エラー（数十 KiB）の描画 1 回につき 10 ms 以下とする（02 §3.2.2）。§10.3 のベンチマークで確かめ、結果を実装計画に記録する。すべてのログ行に掛かる `RedactText` の費用は変えない。

## 4. `internal/logging`（変更）

### 4.1 `PreExecutionError`

```go
type PreExecutionError struct {
    Type                ErrorType
    Message             errmsg.Summary // was string
    Component           string
    RunID               string
    NotificationContext common.NotificationContext
    FailedFilePaths     []string
    Err                 error
}

// DetailMessage returns Message followed by the cause, as a structured message.
func (e *PreExecutionError) DetailMessage() errmsg.Message

// Detail returns DetailMessage().String().
func (e *PreExecutionError) Detail() string
```

- `DetailMessage()`: `Err` が nil なら `NewMessage(e.Message.Part())`、そうでなければ `NewMessage(e.Message.Part(), Const(": "), Cause(e.Err))`。
- `Error()`（`pre_execution_error.go:82-87`）の書式 `%s: %s: %v (component: %s, run_id: %s)` と `%s: %s (component: %s, run_id: %s)` は変えない。`Message` は `Summary` の `String()` を通って同じ文言になる。
- `Is`・`As`・`Unwrap` は変えない。

### 4.2 `ExecutionError`

```go
type ExecutionError struct {
    Message     errmsg.Summary // was string
    Component   string
    RunID       string
    GroupName   string
    CommandName string
    Err         error
}

// ReportMessage returns the message HandleExecutionError reports:
// Message, the group/command context, and the cause.
func (e *ExecutionError) ReportMessage() errmsg.Message

// contextParts returns the group/command context as parts. It is the single
// place that builds the context; ReportMessage and ContextString both use it.
func (e *ExecutionError) contextParts() []errmsg.Part
```

- `contextParts()`: `GroupName` が空でなければ `Const("group: ")`・`Ident(GroupName)`。`CommandName` が空でなければ、前の部分があれば `Const(", ")` を挟んで `Const("command: ")`・`Ident(CommandName)`。どちらも空なら nil。
- `ReportMessage()`: `Message.Part()`。`contextParts()` が空でなければ `Const(" (")`・context の部分・`Const(")")`。`Err` が nil でなければ `Const(": ")`・`Cause(Err)`。
- `ContextString()`: `errmsg.NewMessage(e.contextParts()...).String()`。文言は変更前の `group: %s`・`command: %s` を `", "` でつないだものと同じである。
- `Error()`（`execution_error.go:36-46`）の書式と `Unwrap()` は変えない。

### 4.3 記録と凍結

- `errorRecordParams.errorMsg` の型を `string` から `errmsg.Message` に変える（`pre_execution_error.go:125`）。`writeErrorLogRecord` は `error_message` を `slog.Any(common.PreExecErrorAttrs.ErrorMessage, params.errorMsg)` で記録する（変更前は `slog.String`、`:187`）。値の動的な型は `errmsg.Message` になる。
- 凍結する位置は記録の組み立てとする。`preExecutionRecordParams` は `errorMsg: preExecErr.DetailMessage().Freeze()`（`:215`）を、`HandleExecutionError` は `ReportMessage().Freeze()`（`:243-276`）を渡す。これにより、`HandlePreExecutionError`・`NotifyPreExecutionError`・`HandleExecutionError` のどの報告でも、平らにするのは 1 回だけになる。`handleErrorCommon` と `writeErrorLogRecord` は凍結済みの `Message` を受け取り、`String()` を呼ぶだけである。
- `handleErrorCommon`（`:158-160`）の stderr の `Details:` は、凍結した `Message` の `String()` を `strings.TrimRight(..., "\r\n")` してから、従来どおり行ごとに字下げして出す。redaction を通らないことと文言は変わらない（AC-19）。
- 原因の `Error()` の panic は `Freeze()`（`Segments()`）の時点で起きる。変更前に `preExecutionRecordParams` の `Detail()`（`:215`）や `HandleExecutionError` の組み立てで起きていたのと同じ時点であり、報告は回復しない（02 §4.2）。
- `slog.Value.String()` は `LogValuer` を `fmt` 経由で解決して `Message.String()` を呼ぶので、`error_message` を `attr.Value.String()` で読む既存のテストは文字列の内容が変わらない見込みである。実装時に実行して確かめる（§10.7）。
- 通知ビルダー（`buildPreExecutionError`、`slack_handler.go:841`）は変えない。`RedactingHandler` の後で `error_message` を描画済みの文字列として受け取る（AC-23）。

### 4.4 `PreExecutionError.Message` に入る要約文の分類

本番の 23 か所をすべて機械的に書き換える。定数式は `ConstSummary`、値を含めて作るものは `TextSummary`（`fmt.Sprintf` や `err.Error()` の結果を渡す）。

| # | 箇所 | 変更前の `Message` | 変更後 |
|---|---|---|---|
| 1 | `cmd/runner/main.go:134` `reportStartupPrivilegeFailure` | `fmt.Sprintf("Failed to drop startup privileges: %v", err)` | `TextSummary` |
| 2 | `main.go:184` `main`（`--run-id`） | `fmt.Sprintf("Invalid run ID passed to --run-id: %v (accepted format: %s)", err, ...)` | `TextSummary` |
| 3 | `main.go:197` `main`（既定のハッシュディレクトリ） | `fmt.Sprintf("Invalid default hash directory: must be absolute path, got: %s", ...)` | `TextSummary` |
| 4 | `main.go:241` `mainWithExitCode` の既定の分岐 | `err.Error()` | `TextSummary` |
| 5 | `main.go:259` `parseLogLevel` | `fmt.Sprintf("Invalid log level %q: %v", ...)` | `TextSummary` |
| 6 | `main.go:295` `run`（Webhook 環境変数） | `err.Error()` | `TextSummary` |
| 7 | `main.go:324` `run`（設定ファイルの指定） | `"Config file path is required"` | `ConstSummary` |
| 8 | `main.go:341` `run`（検証マネージャーの初期化） | `fmt.Sprintf("Verification manager initialization failed: %v", err)` | `TextSummary` |
| 9 | `main.go:372` `run`（global の展開） | `fmt.Sprintf("Failed to expand global configuration: %v", err)` | `ConstSummary("Failed to expand global configuration")`・`Err: err` |
| 10 | `main.go:383` `run`（テンプレート検証） | `fmt.Sprintf("Template validation failed: %v", err)` | `ConstSummary("Template validation failed")`・`Err: err` |
| 11 | `main.go:404` `run`（検証の失敗） | `err.Error()` | `TextSummary` |
| 12 | `main.go:500` `auditConfiguredDirPermissions`（未対応の reason） | `fmt.Sprintf("unhandled check skip reason %d for path %s", ...)` | `TextSummary` |
| 13 | `main.go:512` `auditConfiguredDirPermissions`（チェッカーの初期化） | `fmt.Sprintf("directory permission checker initialisation failed: %v", secErr)` | `ConstSummary("directory permission checker initialisation failed")`・`Err: secErr` |
| 14 | `main.go:547` `auditConfiguredDirPermissions`（違反） | `fmt.Sprintf("directory permission audit failed: %d ...", ...)` | `TextSummary` |
| 15 | `main.go:640` `executeRunner`（`--groups`） | `fmt.Sprintf("Invalid groups specified: %v", err)` | `ConstSummary("Invalid groups specified")`・`Err: err` |
| 16 | `bootstrap/config.go:61` | `"Config file path is required"` | `ConstSummary` |
| 17 | `bootstrap/config.go:74` | `"Failed to verify and read the configuration file"`（`Err` あり） | `ConstSummary` |
| 18 | `bootstrap/config.go:94` | `"Failed to load the configuration"`（`Err` あり） | `ConstSummary` |
| 19 | `bootstrap/config.go:106` | `"Invalid slack_allowed_host"`（`Err` あり） | `ConstSummary` |
| 20 | `bootstrap/environment.go:105` | `fmt.Sprintf("Failed to setup logger: %v", err)` | `TextSummary` |
| 21 | `bootstrap/environment.go:144` | `"Slack webhook URL validation failed"`（`Err` あり） | `ConstSummary` |
| 22 | `runerrors/pre_execution.go:30-35` | `fmt.Sprintf("Total: %d, Verified: %d, Failed: %d, Error: %v", ...)` または `Collection failed: ...` | `TextSummary` |
| 23 | `group_stage.go:67-98` の段階の定義表 | `message string` | 表の欄を `summary errmsg.Summary` にし、各行を `ConstSummary("...")` にする。`groupStagePreExecutionError` は `Message: def.summary` |

- #9・#10・#13・#15 は、原因を `Message` から `Err` へ移す（01 対象 4）。`Detail()` の文言は `Message + ": " + Err.Error()` になり、変更前の `fmt.Sprintf("...: %v", err)` と同じである（AC-18）。
- #23 の表の欄を `string` のままにして `ConstSummary(def.message)` とすると、引数が定数式でなくなり §9.3 のガードが失敗する。欄の型を `errmsg.Summary` にする。
- `Message` を設定する箇所はこれですべてである。機械的な書き換え以外の変更を加えない。テストのリテラルは §10.7 に従って書き換える。

## 5. `internal/runner`（変更）

### 5.1 構造を引き継ぐエラー型

`GroupStageError`・`GroupError`・`GroupErrors`・`CommandExecutionError` の 4 つの型が `Structured` を実装する。`Error()` はすべて `return e.StructuredMessage().String()` の 1 文にし、§9.5 のガードで形を固定する。`Unwrap` は変更しない。

| 型 | `StructuredMessage()` の部分 |
|---|---|
| `*GroupStageError` | `e.err == nil` なら `NewMessage(Const("group pre-execution failed"))`（`group_stage.go:138-143` と同じ文言）。そうでなければ `NewMessage(Cause(e.err))` |
| `*GroupError` | `NewMessage(Const("failed to execute group "), Ident(e.group), Const(": "), IndentedCause(e.err))` |
| `*GroupErrors` | `e.errs` を、間に `NewMessage(Const("\n"))` を置いて `Merge` でつなぐ |
| `*CommandExecutionError` | `NewMessage(Const("command "), Ident(e.CommandName), Const(" in group "), Ident(e.GroupName), Const(" failed: "), Cause(e.Err))` |

- `CommandExecutionError` の欄は公開なので `Err` が nil の値もありうる。`Cause(nil)` は `<nil>` の `Text` になり、`Error()` は変更前の `%v` と同じ文言を返す（`group_executor.go:35-37`）。
- ゼロ値や nil の欄を持つ値の `Error()` は panic しない。`GroupError` のゼロ値は `failed to execute group : <nil>`、`GroupErrors` のゼロ値は空文字列になる（02 §3.4.1）。
- `GroupErrors.StructuredMessage` は平らにせずに組み合わせるので、各 `GroupError` の `Identifier` の断片は保たれる。`GroupErrors.Error()` の文言は、各 `GroupError.Error()` を `\n` でつないだ変更前の結果と同じである。

### 5.2 `group_executor.go` の各ラップ

`fmt.Errorf`（`%w` の有無によらない）を `errmsg` の構築に置き換える。網羅は §9.2 の AC-41 ガードが担う。次の表は、現在のコードに当てはめた結果である。

| 行 | 関数 | 変更前の書式 | 部分 |
|---|---|---|---|
| 169 | `ExecuteGroup` | `failed to expand group[%s]: %w` | `Const("failed to expand group[")`・`Ident(group)`・`Const("]: ")`・`Cause` |
| 186 | `ExecuteGroup` | `failed to resolve work directory: %w` | `Const("failed to resolve work directory: ")`・`Cause` |
| 317 | `preExpandCommands` | `failed to pre-expand commands for group[%s]: command[%s] (index %d): %w` | `Const("failed to pre-expand commands for group[")`・`Ident(group)`・`Const("]: command[")`・`Ident(command)`・`Const("] (index ")`・`Text(strconv.Itoa(i))`・`Const("): ")`・`Cause` |
| 334 | `preExpandCommands` | `failed to resolve workdir: %w` | `Const("failed to resolve workdir: ")`・`Cause` |
| 374 | `auditGroupDirPermissions` | `%w: %d for path %s` | `Cause(errUnhandledCheckSkipReason)`・`Const(": ")`・`Text(strconv.Itoa(int(reason)))`・`Const(" for path ")`・`Path(p)` |
| 388 | `auditGroupDirPermissions` | `%w for group[%s]: %d directory violation(s) detected; review directory permissions` | `Cause(ErrDirPermViolation)`・`Const(" for group[")`・`Ident(groupName)`・`Const("]: ")`・`Text(strconv.Itoa(len(violations)))`・`Const(" directory violation(s) detected; review directory permissions")` |
| 435 | `verifyGroupFiles` | `command path resolution failed for %q: %w` | `Const("command path resolution failed for ")`・`Path(strconv.Quote(cmd.ExpandedCmd))`・`Const(": ")`・`Cause(resolveErr)` |
| 456 | `verifyGroupFiles` | `command dependency verification failed for %q: %w` | `Const("command dependency verification failed for ")`・`Path(strconv.Quote(resolvedPath))`・`Const(": ")`・`Cause(depErr)` |
| 511 | `executeCommandInGroup` | `resolved environment variables security validation failed: %w` | `Const("resolved environment variables security validation failed: ")`・`Cause` |
| 521 | `executeCommandInGroup` | `output path validation failed: %w` | `Const("output path validation failed: ")`・`Cause` |
| 671 | `executeSingleCommand` | `%w: command %s failed with exit code %d` | `Cause(ErrCommandFailed)`・`Const(": command ")`・`Ident(cmd.Name())`・`Const(" failed with exit code ")`・`Text(strconv.Itoa(result.ExitCode))` |

- 数値（index・件数・終了コード・理由の番号）は `Text` にする。`Constant` は定数式に限るので使えない。数字だけの文字列は key=value・値形式の検出・値全体置換のどれにも当たらないので、出力は変わらない（02 §3.4.2）。
- `%q` で引用したパスは、引用とエスケープを含めた全体を 1 つの `Path` の断片にする。`strconv.Quote` の結果を渡すので `%q` と同じ引用とエスケープになる。
- 先頭の番兵（`ErrCommandFailed`・`ErrDirPermViolation`・`errUnhandledCheckSkipReason`）は `Cause` の部分として表す。番兵は `Structured` を実装しないので、平らにすると `Text` になる。どの番兵も機密を示す語を含まない。
- `ExpandWorkDir` の呼び出し（`:686`・`:723`）は、`fmt.Sprintf("group[%s]", ...)`・`fmt.Sprintf("command[%s]", ...)` の代わりに `config.GroupLevel(...)`・`config.CommandLevel(...)` を渡す（§6.4）。
- 各ラップは `errmsg.NewError(...)` で作り、`newGroupStageError`・`newCommandStageError` の原因か、そのまま返す `error` にする。`NewError` は原因の部分をちょうど 1 つ要求するので、番兵と原因を 2 つ持つ書式はここには無い（該当するのは executor の `killAfterCancelError` だけである。§7.2）。

### 5.3 `cancelledRunError`

`executeGroups` の `errors.Join(ctxErr, err)`（`runner.go:452`）を、中断であることを宣言する専用の型に置き換える。作る箇所はこの 1 か所だけで、`ctxErr` も `err` も nil でないことを確かめた後である（`:440-453`）。

```go
// cancelledRunError is returned when the run's context is done while a group
// failure is being handled. Its text equals errors.Join(ctxErr, err).
type cancelledRunError struct {
    ctxErr error
    err    error
}

func (e *cancelledRunError) Error() string { return e.StructuredMessage().String() }

func (e *cancelledRunError) Unwrap() []error { return []error{e.ctxErr, e.err} }

func (e *cancelledRunError) StructuredMessage() errmsg.Message {
    return errmsg.NewMessage(
        errmsg.Cause(e.ctxErr),
        errmsg.Const("\n"),
        errmsg.Cause(e.err),
    )
}
```

- 構造化メッセージは、中断の原因と失敗した group のエラーを、それぞれ原因の部分として改行でつなぐ。2 つとも nil でないときの `errors.Join` の文言と一致する（AC-32）。
- `Unwrap() []error` を持つので、`errors.Is(err, ctx.Err())` と、失敗した group の原因への `errors.Is`・`errors.AsType` は変更前と同じに届く。
- `cancelledRunError` は `errmsg.JoinedError` と描画が同じだが、専用の型として別に置く。中断であることを型で宣言する（02 §3.4.3）ためと、`ctxErr`・`err` という名の欄で意図を読めるようにするためである。`errmsg.Join` は、`errors.Join` と同じ意味の結合が要る executor の経路（§7.4）で使う。
- 返すエラーの中身（それ以前に集めた group の失敗を含めないこと）は変えない。
- AC-31 は、失敗した group のエラーが宣言する部分についてだけ保証する。`GroupStageError.Error()` は原因の文言だけを返すので、原因が group 名を含まなければ本文に group 名は出ない。この場合に group 名を付け足すと AC-32 に反するので、付け足さない。

## 6. `internal/runner/config`（変更）

### 6.1 `Level` と `Field` の型

`internal/runner/config/errors.go` に置く。ゼロ値は「無し」を表し、空の文字列として描画する。

```go
// levelKind is the kind of a Level. The zero value is levelNone.
type levelKind int

const (
    levelNone levelKind = iota
    levelGlobal
    levelGroup
    levelCommand
    levelTemplate
)

// Level is where a value was being expanded. The zero value is "no level"
// and renders as the empty string.
type Level struct {
    kind levelKind
    name string
}

func globalLevel() Level               { return Level{kind: levelGlobal} }
func groupLevel(name string) Level     { return Level{kind: levelGroup, name: name} }
func commandLevel(name string) Level   { return Level{kind: levelCommand, name: name} }
func templateLevel(name string) Level  { return Level{kind: levelTemplate, name: name} }

// GroupLevel declares a group scope. It is the only constructor exported for
// callers outside this package (ExpandWorkDir's callers).
func GroupLevel(name string) Level { return groupLevel(name) }

// CommandLevel declares a command scope.
func CommandLevel(name string) Level { return commandLevel(name) }

// fieldKey is the configuration field being expanded. The zero value is
// fieldNone.
type fieldKey int

const (
    fieldNone fieldKey = iota
    fieldCmd
    fieldArgs
    fieldEnv
    fieldEnvVars
    fieldEnvImport
    fieldWorkdir
    fieldVerifyFiles
    fieldCmdAllowed
    fieldVars
)

// Field is the configuration field being expanded. The zero value is
// "no field" and renders as the empty string.
type Field struct {
    key      fieldKey
    name     string // variable name, for vars fields only
    index    int
    hasIndex bool
}

func cmdField() Field                  { return Field{key: fieldCmd} }
func envField() Field                  { return Field{key: fieldEnv} }
func envVarsField(index int) Field     { return Field{key: fieldEnvVars, index: index, hasIndex: true} }
func envImportField() Field            { return Field{key: fieldEnvImport} }
func workdirField() Field              { return Field{key: fieldWorkdir} }
func argsField(index int) Field        { return Field{key: fieldArgs, index: index, hasIndex: true} }
func verifyFilesField(index int) Field { return Field{key: fieldVerifyFiles, index: index, hasIndex: true} }
func cmdAllowedField(index int) Field  { return Field{key: fieldCmdAllowed, index: index, hasIndex: true} }
func varsField() Field                 { return Field{key: fieldVars} }
func varField(name string) Field       { return Field{key: fieldVars, name: name} }
func varElementField(name string, index int) Field {
    return Field{key: fieldVars, name: name, index: index, hasIndex: true}
}
```

- `Level.String()` の描画: `""`・`"global"`・`"group[<name>]"`・`"command[<name>]"`・`"template[<name>]"`。種類ごとの `switch` で、`default` は `""` を返す。
- `Field.String()` の描画: `""`・`"cmd"`・`"args[<index>]"`・`"env"`・`"env_vars[<index>]"`・`"env_import"`・`"workdir"`・`"verify_files[<index>]"`・`"cmd_allowed[<index>]"`・`"vars"`・`"vars.<name>"`・`"vars.<name>[<index>]"`。index は `strconv.Itoa` で作る。`hasIndex` が真のときだけ `[<index>]` を続ける。`fieldVars` は `name` が空なら `"vars"`、空でなければ `"vars." + name` を基本にし、`hasIndex` が真なら `"[" + index + "]"` を基本の後ろに続ける（`varElementField("foo", 2)` は `vars.foo[2]` になる）。
- `parts()` は非公開である（02 §3.1.1 の契約 8）。呼び出し側は `config` パッケージの中（`ErrUndefinedVariableDetail.StructuredMessage` と `ExpandWorkDir`）だけである。
- `Level.parts()`: `levelGlobal` は `[]Part{Const("global")}`、`levelGroup` は `Const("group[")`・`Ident(l.name)`・`Const("]")`、`levelCommand` は `Const("command[")` で同じ形、`levelTemplate` は `Const("template[")` で同じ形、`levelNone` は nil。
- `Field.parts()`: キーごとの `switch` で、各文言を `Const` の文字列リテラルから作る。キーの文言を変数から `Const` に渡すと §9.3 のガードが定数式でないとして拒否するためである。`args`・`verify_files`・`cmd_allowed` の index と `vars` の index は `Text(strconv.Itoa(...))`。`vars.<name>` の名前は `Ident(f.name)`（`name` が空なら `Const("vars")` だけ）。`fieldNone` は nil。
- `parts()` の結果は、呼び出し側が `slices.Concat` でほかの部分とつないで `NewMessage`・`NewError` に渡す。
- `Level` の文字列を使うほかのエラー型（`ErrCircularReferenceDetail`・`ErrInvalidVariableNameDetail` など、`Level string`・`Field string` を持つ型）は、`level.String()`・`field.String()` を保持する。これらは #1197 の対象なので構造化しない。

### 6.2 引数の型を変える関数

`level string`・`field string` を受け取る関数のうち、`%{...}` の展開と `ErrUndefinedVariableDetail` の生成に関わるものの引数を `Level`・`Field` に変える。`variableResolver` の `field` も `Field` にする。

| ファイル | 関数 | 変更後の引数 |
|---|---|---|
| `expansion.go` | `variableResolver`（型） | `field Field` |
| `expansion.go` | `ExpandWorkDir` | `(workdir string, expandedVars map[string]string, level Level)` |
| `expansion.go` | `ExpandString` | `(input string, expandedVars map[string]string, level Level, field Field)` |
| `expansion.go` | `resolveAndExpand` | `level Level, field Field` |
| `expansion.go` | `processVarRefs` | `level Level, field Field` |
| `expansion.go` | `ProcessEnvImport` | `level Level` |
| `expansion.go` | `newVarExpander`・`varExpander.level` | `level Level` |
| `expansion.go` | `(*varExpander).expandString` | `field Field` |
| `expansion.go` | `(*varExpander).resolveVariable` | `field Field` |
| `expansion.go` | `ProcessVars` | `level Level` |
| `expansion.go` | `validateAndClassifyVars` | `level Level` |
| `expansion.go` | `validateStringVar`・`validateArrayVar` | `level Level` |
| `expansion.go` | `expandVarsWithLazyResolution` | `level Level` |
| `expansion.go` | `ProcessEnv` | `level Level` |
| `validation.go` | `validateVariableName` | `(varName string, level Level, field Field)` |
| `template_expansion.go` | `validateGlobalOnly` | `(input, templateName string, field Field)` |
| `template_expansion.go` | `validateFieldVars` | `(input, templateName string, fieldName Field, globalVars map[string]string)` |

- `HasVariableReference` は `processVarRefs(input, detector, Level{}, Field{}, ...)` を渡す。
- `variableResolver` を実装する関数リテラル（`HasVariableReference`・`resolveAndExpand`・`(*varExpander).expandString`・`(*varExpander).resolveVariable`・`validateGlobalOnly`・`validateFieldVars` の中）の第 2 引数の型も `Field` にする。
- `validateVariableName` は `level == "global"` の比較を `level.kind == levelGlobal` に変える。`location := fmt.Sprintf("%s.%s", level, field)` は `String()` を通って同じ文字列になる。
- 展開の呼び出しに渡す文字列を構築関数に置き換える: `"global"` は `globalLevel()`、`fmt.Sprintf("group[%s]", name)` は `groupLevel(name)`、`fmt.Sprintf("command[%s]", name)` は `commandLevel(name)`、`"workdir"` は `workdirField()`、`"cmd"` は `cmdField()`、`"env"` は `envField()`、`"env_import"` は `envImportField()`、`fmt.Sprintf("args[%d]", i)` は `argsField(i)`、`fmt.Sprintf("verify_files[%d]", i)` は `verifyFilesField(i)`、`fmt.Sprintf("cmd_allowed[%d]", i)` は `cmdAllowedField(i)`、`"vars"` は `varsField()`。`expandVarsWithLazyResolution` の `fmt.Sprintf("vars.%s", varName)`・`fmt.Sprintf("vars.%s[%d]", varName, i)`（`:724`・`:740`）は `varField(varName)`・`varElementField(varName, i)` にする。
- `template_expansion.go` の `processVarRefs` 呼び出し（`:686`・`:1114`）は `templateLevel(templateName)` と、呼び出し元が作った `Field` を渡す。呼び出し元は `cmdField()`・`argsField(i)`・`envVarsField(i)`・`workdirField()` を渡す。テンプレートの `${...}` だけを扱う関数（`expandSingleArg`・`expandArrayPlaceholder` など）の `field string` は変えない。
- 文字列の `Level`・`Field` を持つ既存の詳細型（`ErrInvalidEscapeSequenceDetail`・`ErrUnclosedVariableReferenceDetail`・`ErrInvalidVariableNameDetail`・`ErrCircularReferenceDetail`・`ErrMaxRecursionDepthExceededDetail`・`ErrArrayVariableInStringContextDetail`・`ErrUnsupportedTypeDetail`・`ErrTooManyVariablesDetail`・`ErrEnvImportVarsConflictDetail`・`ErrDuplicateVariableDefinitionDetail`・`ErrDuplicatePathDetail`・`ErrDuplicateResolvedPathDetail` など）には、`level.String()`・`field.String()` を渡す。固定のキーは構築関数から作る（`cmdField()`・`envField()`・`envImportField()`・`workdirField()`・`argsField(i)`・`verifyFilesField(i)`・`cmdAllowedField(i)`・`varsField()`・`varField(name)`・`varElementField(name, i)`）。`validateVariableName(varName, level, "vars")`（`expansion.go:573`）の `"vars"` は `varsField()` にする。`groupLevel(groupName).String()` は `fmt.Sprintf("group[%s]", groupName)` の置き換えである。`ErrUndefinedVariableDetail` だけが型付きの `Level`・`Field` を持つ。

### 6.3 `ErrUndefinedVariableDetail`

```go
type ErrUndefinedVariableDetail struct {
    Level        Level // was string
    Field        Field // was string
    VariableName string
    Context      string
    Chain        []string
}

func (e *ErrUndefinedVariableDetail) Error() string // returns e.StructuredMessage().String()
func (e *ErrUndefinedVariableDetail) StructuredMessage() errmsg.Message
func (e *ErrUndefinedVariableDetail) Unwrap() error // ErrUndefinedVariable, unchanged
```

`StructuredMessage()` の部分:

1. `Const("undefined variable in ")`
2. `e.Level.parts()`
3. `Const(".")`
4. `e.Field.parts()`
5. `Const(": '")`・`Ident(e.VariableName)`・`Const("' (context: ")`・`Text(e.Context)`・`Const(")")`
6. `Chain` が空でなければ `Const(" (expansion path: ")`・各名前を `Ident`、間に `Const(" -> ")`・`Const(")")`

変更前の文言は `internal/runner/config/errors.go:266-272` のとおりで、一致する。参照された変数名（`VariableName`）・展開経路（`Chain`）の各変数名・`Level`・`Field` の中の名前は `Identifier`、生のテンプレート（`Context`）は `Text` である。

### 6.4 `ExpandWorkDir`

```go
func ExpandWorkDir(workdir string, expandedVars map[string]string, level Level) (string, error)
```

- 変数の展開の失敗（`expansion.go:58`）: `NewError(Const("failed to expand workdir: "), Cause(err))`。
- 相対パスの拒否（`:63-64`）: `NewError(slices.Concat(level.parts(), []errmsg.Part{Const(": "), Cause(ErrInvalidWorkDir), Const(": "), Path(strconv.Quote(expanded)), Const(" (relative paths are not allowed for security reasons)")})...)`。
- 呼び出し側（`group_executor.go:686`・`:723`）は `config.GroupLevel(runtimeGroup.Spec.Name)`・`config.CommandLevel(runtimeCmd.Spec.Name)` を渡す。

### 6.5 `expansion.go` の各ラップ

`expansion.go` はファイル全体を対象の範囲に入れ、02 §3.8.1 で除く関数だけを除く（§9.1）。分類の規則は 02 §3.5.3 のとおりである。

- 原因が `ErrUndefinedVariableDetail` を運びうるラップ: 名前を `Ident`、固定の文言を `Const` として宣言する。
- 原因が `ErrUndefinedVariableDetail` を運びえないラップ: 前置きの全体を 1 つの `Text`（`errmsg.Text(fmt.Sprintf(...))`）にして `Cause(err)` を続ける。`Ident`・`Path`・`Const` は使わない。原因の構造は保つ。

`ErrUndefinedVariableDetail` を運びうるのは、`ProcessVars`（`expandVarsWithLazyResolution` → `varExpander`）、`ProcessEnv`（`ExpandString`）、`expandCmdAllowed`（`ExpandString`）、`ExpandWorkDir`（`ExpandString`）、`expandCommandFields`（`ExpandString`）を通る経路である。`ProcessEnvImport`・`NewRuntimeGlobal`・`NewRuntimeGroup`・`NewRuntimeCommand`・`filepath.EvalSymlinks` は運びえない。

| 行 | 関数 | 分類 | 部分 |
|---|---|---|---|
| 832 | `ExpandGlobal` | 不可 | `Text(fmt.Sprintf("failed to create RuntimeGlobal: "))`・`Cause` |
| 847 | `ExpandGlobal` | 不可 | `Text(fmt.Sprintf("failed to process global env_import: "))`・`Cause` |
| 857 | `ExpandGlobal` | 可 | `Const("failed to process global vars: ")`・`Cause` |
| 865 | `ExpandGlobal` | 可（`ProcessEnv` の `env`） | `Const("failed to process global env: ")`・`Cause` |
| 919 | `expandCmdAllowed` | 不可 | `Text(fmt.Sprintf("group[%s] cmd_allowed[%d]: ", groupName, i))`・`Cause(ErrEmptyPath)` |
| 925 | `expandCmdAllowed` | 可 | `Const("group[")`・`Ident(groupName)`・`Const("] cmd_allowed[")`・`Text(strconv.Itoa(i))`・`Const("] '")`・`Text(rawPath)`・`Const("': ")`・`Cause` |
| 948 | `expandCmdAllowed` | 不可 | `Text(fmt.Sprintf("group[%s] cmd_allowed[%d] '%s': failed to resolve path: ", groupName, i, expanded))`・`Cause` |
| 982 | `ExpandGroup` | 不可 | `Text(fmt.Sprintf("failed to create RuntimeGroup: "))`・`Cause` |
| 1011 | `ExpandGroup` | 不可 | `Text(fmt.Sprintf("failed to process group[%s] env_import: ", spec.Name))`・`Cause` |
| 1023 | `ExpandGroup` | 可 | `Const("failed to process group[")`・`Ident(spec.Name)`・`Const("] vars: ")`・`Cause` |
| 1031 | `ExpandGroup` | 可 | `Const("failed to process group[")`・`Ident(spec.Name)`・`Const("] env: ")`・`Cause` |
| 1052 | `ExpandGroup` | 可（`expandCmdAllowed`） | `Const("failed to expand cmd_allowed for group[")`・`Ident(spec.Name)`・`Const("]: ")`・`Cause` |
| 1136 | `expandCommandEnvImport` | 不可 | `Text(fmt.Sprintf("failed to process command[%s] env_import: ", spec.Name))`・`Cause` |
| 1159 | `expandCommandVars` | 可 | `Const("failed to process command[")`・`Ident(spec.Name)`・`Const("] vars: ")`・`Cause` |
| 1194 | `expandCommandFields` | 可 | `Const("failed to process command[")`・`Ident(spec.Name)`・`Const("] env: ")`・`Cause` |
| 1226 | `ExpandCommand` | 不可 | `Text(fmt.Sprintf("failed to create RuntimeCommand for command[%s]: ", workingSpec.Name))`・`Cause` |

- `:1226` の原因は `NewRuntimeCommand`（`expansion.go:1224`）の検証エラーであり、`ErrUndefinedVariableDetail` を運びえない。したがって 02 §3.5.3 の「運びえないラップ」に当たり、前置きの全体を 1 つの `Text` にする。コマンド名を `Identifier` にする案（design_carryover.md）は採らない。
- `:919`・`:948` は `fmt.Errorf` の引数に group 名・index・パスを含むが、原因が carry 不可なので前置き全体が `Text` になる。パスを `Path` として宣言するのは、02 §3.5.3 の規則（運びえないラップでは `Identifier`・`Path`・`Constant` を使わない）を優先する。
- `ProcessEnvImport`・`ProcessEnv`・`resolveAndPrepareCommandSpec`・`ApplyTemplateInheritance`・`expandTemplateToSpec` の内側の `fmt.Errorf` は変更しない（除外関数。§9.1）。ただし引数の型は §6.2 に従って変える。
- `ErrUndefinedVariableDetail` を作る箇所（`:128`・`:425`）は、`Level`・`Field` の型付きの値をそのまま入れる。それ以外の詳細型を作る箇所は §6.2 の最後の項に従う。

### 6.6 `template_expansion.go` の変更

`validateGlobalOnly`（`:666`）・`validateFieldVars`（`:1081`）の `field` の型を `Field` に変える。`processVarRefs` に渡す `fmt.Sprintf("template[%s]", templateName)` は `templateLevel(templateName)` にする。エラー型（`ErrLocalVariableInTemplate` など）の `Field string` には `field.String()` を渡す。`field` を `%q` で出す箇所（`:708`）は、`Field` の `String()` が使われるように `field.String()` を渡す。

## 7. コマンドの実行経路（変更）

### 7.1 `internal/runner/resource`

`(*NormalResourceManager).ExecuteCommand`・`executeCommandWithOutput`、`(*DryRunResourceManager).ExecuteCommand`・`evaluateCommandRisk` の `fmt.Errorf` を `errmsg` の構築に置き換える。

| 行 | 関数 | 変更前の書式 | 部分 |
|---|---|---|---|
| 85 | `ExecuteCommand` | `command validation failed: %w` | `Const("command validation failed: ")`・`Cause` |
| 89 | `ExecuteCommand` | `command group validation failed: %w` | `Const("command group validation failed: ")`・`Cause` |
| 102 | `ExecuteCommand` | `risk evaluation failed: %w` | `Const("risk evaluation failed: ")`・`Cause` |
| 120 | `ExecuteCommand` | `invalid risk_level configuration: %w` | `Const("invalid risk_level configuration: ")`・`Cause` |
| 147 | `ExecuteCommand` | `%w: command %s denied (reason: %s)` | `Cause(runnertypes.ErrCommandSecurityViolation)`・`Const(": command ")`・`Path(cmd.ExpandedCmd)`・`Const(" denied (reason: ")`・`Text(string(plan.Assessment.BlockingReason))`・`Const(")")` |
| 150 | `ExecuteCommand` | `%w: command %s (effective risk: %s) exceeds maximum allowed risk level (%s)` | `Cause(...)`・`Const(": command ")`・`Path(cmd.ExpandedCmd)`・`Const(" (effective risk: ")`・`Text(effectiveRisk.String())`・`Const(") exceeds maximum allowed risk level (")`・`Text(maxAllowedRisk.String())`・`Const(")")` |
| 253 | `executeCommandWithOutput` | `output capture preparation failed: %w` | `Const("output capture preparation failed: ")`・`Cause` |
| 264 | `executeCommandWithOutput` | `failed to close output capture: %w` | `Const("failed to close output capture: ")`・`Cause(closeErr)` |
| 266 | `executeCommandWithOutput` | `%w; and also failed to close output capture: %v` | `Cause(err)`・`Const("; and also failed to close output capture: ")`・`Text(closeErr.Error())` |
| 290 | `executeCommandWithOutput` | `output capture finalization failed: %w` | `Const("output capture finalization failed: ")`・`Cause` |
| 175 | `(*DryRunResourceManager).ExecuteCommand` | `command validation failed: %w` | `Const("command validation failed: ")`・`Cause` |
| 179 | `(*DryRunResourceManager).ExecuteCommand` | `command group validation failed: %w` | `Const("command group validation failed: ")`・`Cause` |
| 185 | `(*DryRunResourceManager).ExecuteCommand` | `command analysis failed: %w` | `Const("command analysis failed: ")`・`Cause` |
| 416 | `evaluateCommandRisk` | `failed to resolve command path '%s': %w. This typically occurs ...` | `Const("failed to resolve command path '")`・`Path(cmd.ExpandedCmd)`・`Const("': ")`・`Cause(err)`・`Const(". This typically occurs if the command is not found in the system PATH or there are permission issues preventing access")` |
| 427 | `evaluateCommandRisk` | `security analysis failed for command '%s': %w` | `Const("security analysis failed for command '")`・`Path(cmd.ExpandedCmd)`・`Const("': ")`・`Cause` |
| 441 | `evaluateCommandRisk` | `invalid risk_level configuration for command '%s': %w` | `Const("invalid risk_level configuration for command '")`・`Path(cmd.ExpandedCmd)`・`Const("': ")`・`Cause` |

- `:266` の `closeErr` は変更前も `%v` で入れていてラップしていないので、`Text(closeErr.Error())` にしても到達性は変わらない。
- `:147`・`:150` の `cmd.ExpandedCmd` はパスなので `Path`。`BlockingReason`・危険度の文字列は `Text`。
- `executeCommandInternal` はエラーを組み立てないので変えない。
- `NormalResourceManager.CreateTempDir`（`:325`）・`CleanupTempDir`（`:337`）・`CleanupAllTempDirs`（`:362`）・`DryRunResourceManager.ValidateOutputPath`（`:159`）・`UpdateCommandDebugInfo`（`:923-945`）は、02 §3.6.2 の「対象にしない」に従い変えない。

### 7.2 `internal/runner/base/executor`

#### 7.2.1 一時ディレクトリ（`tempdir_manager.go`）

`(*DefaultTempDirManager).Create` の 2 つのラップを `PathErrorCause` にする。前置きは既存の文言を保つ。

| 行 | 変更前 | 部分 |
|---|---|---|
| 77 | `failed to create temporary directory: %w`（`os.MkdirTemp`） | `Const("failed to create temporary directory: ")`・`PathErrorCause(err)` |
| 90 | `failed to set permissions on temporary directory: %w`（`os.Chmod`） | `Const("failed to set permissions on temporary directory: ")`・`PathErrorCause(err)` |

- 一時ディレクトリのパスは group 名を含む（`scr-<group>-`、`:74`）。group 名が語を含んでも、パスの断片は `Path` なので値全体置換を受けない。操作（`mkdir` など）と OS のエラーは `Text` になる。
- `Cleanup`（`:116`）はログに出すだけで 2 つのレコードの原因にならないので変えない。

#### 7.2.2 検証と実行（`executor.go`）

`(*DefaultExecutor).Validate`・`validatePrivilegedCommand`・`executeNormal`・`executeWithUserGroup` の `fmt.Errorf` を `errmsg` の構築に置き換える。コマンドパスと作業ディレクトリのパスは `Path`、固定の文言は `Const`。

| 行 | 関数 | 変更前の書式 | 部分 |
|---|---|---|---|
| 613 | `Validate` | `%w: command path must be local or absolute: %s` | `Cause(ErrInvalidPath)`・`Const(": command path must be local or absolute: ")`・`Path(cmd.ExpandedCmd)` |
| 616 | `Validate` | `%w: command path contains relative path components ('.' or '..'): %s` | `Cause(ErrInvalidPath)`・`Const(": command path contains relative path components ('.' or '..'): ")`・`Path(cmd.ExpandedCmd)` |
| 623 | `Validate` | `failed to check directory %s: %w` | `Const("failed to check directory ")`・`Path(cmd.EffectiveWorkDir)`・`Const(": ")`・`Cause(err)` |
| 626 | `Validate` | `%w: %s` | `Cause(ErrDirNotExists)`・`Const(": ")`・`Path(cmd.EffectiveWorkDir)` |
| 715 | `validatePrivilegedCommand` | `%w: privileged commands must use absolute paths: %s` | `Cause(ErrPrivilegedCmdSecurity)`・`Const(": privileged commands must use absolute paths: ")`・`Path(cmd.ExpandedCmd)` |
| 720 | `validatePrivilegedCommand` | `%w: privileged commands must use absolute working directory paths: %s` | `Cause(ErrPrivilegedCmdSecurity)`・`Const(": privileged commands must use absolute working directory paths: ")`・`Path(cmd.EffectiveWorkDir)` |
| 344 | `executeNormal` | `command validation failed: %w` | `Const("command validation failed: ")`・`Cause` |
| 357 | `executeNormal` | `%w: %s` | `Cause(ErrPathNotAbsolute)`・`Const(": ")`・`Path(cmd.ExpandedCmd)` |
| 195 | `executeWithUserGroup` | `command validation failed: %w` | `Const("command validation failed: ")`・`Cause` |
| 202 | `executeWithUserGroup` | `privileged command security validation failed: %w` | `Const("privileged command security validation failed: ")`・`Cause` |
| 254 | `executeWithUserGroup` | `user/group privilege execution failed: %w` | `Const("user/group privilege execution failed: ")`・`Cause` |
| 305 | `executeWithUserGroup` | `user/group privilege execution failed: %w` | `Const("user/group privilege execution failed: ")`・`Cause` |

- `ErrEmptyCommand` をそのまま返す分岐（`:608`・`:710`・`:349`・`:207`）と、`ErrNoPrivilegeManager`・`ErrUserGroupPrivilegeUnsupported` をそのまま返す分岐（`:184`・`:189`）は、番兵をそのまま返すので変更しない。
- `prepareCommand`（`:419`）・`stageFromFD`・`output_pump.go`・`fdexec_linux.go` は、02 §3.8.1 の範囲外なので `fmt.Errorf` のままにする。`prepareCommand` が返すエラーは、範囲内の `executeNormal`・`executeWithUserGroup` のラップを通って 2 つのレコードの原因になるが、その原因は OS・標準ライブラリのエラーか executor の番兵であり、`Structured` を運ばない。`Text` として平らになり、変更前と同じ保護を受ける。

#### 7.2.3 子プロセスの監督（`command_lifecycle.go`）

`runCommand`・`reportStartFailure`・`superviseCommand`・`killChild`・`killOutcome` の `errors.Join` を `errmsg.Join` に、`fmt.Errorf` を `errmsg` の構築に置き換える。

| 行 | 関数 | 変更前 | 変更後 |
|---|---|---|---|
| 589 | `runCommand`（`!opened`） | `errors.Join(elevErr, closeErr, fdErr, pc.release())` | `errmsg.Join(elevErr, closeErr, fdErr, pc.release())` |
| 591 | `runCommand`（`!started`） | `errors.Join(elevErr, closeErr, fdErr)` | `errmsg.Join(elevErr, closeErr, fdErr)`（`reportStartFailure` に渡す） |
| 593 | `runCommand`（既定） | `errors.Join(elevErr, closeErr)` | `errmsg.Join(elevErr, closeErr)`（`superviseCommand` に渡す） |
| 619 | `reportStartFailure` | `errors.Join(startErr, pc.release())` | `errmsg.Join(startErr, pc.release())` |
| 625 | `reportStartFailure` | `command execution failed: %w` | `Const("command execution failed: ")`・`Cause(combinedErr)` |
| 736 | `superviseCommand` | `%w: pid=%d`（`ErrChildNotReaped`） | `Cause(ErrChildNotReaped)`・`Const(": pid=")`・`Text(strconv.Itoa(pid))` |
| 782 | `superviseCommand` | `errors.Join(rankedError(outcome), outcome.killErr, notReapedErr, startupErr)` | `errmsg.Join(...)` |
| 789 | `superviseCommand` | `command execution failed: %w` | `Const("command execution failed: ")`・`Cause(cmdErr)` |
| 906 | `killChild`（`ErrNoPrivilegeManager`） | `%w: pid=%d: %w`（`ErrKillAfterCancel`, pid, `ErrNoPrivilegeManager`） | `&killAfterCancelError{pid: pid, err: ErrNoPrivilegeManager}` |
| 931 | `killChild`（`ErrKillStrategyUnset`） | `%w: pid=%d` | `Cause(ErrKillStrategyUnset)`・`Const(": pid=")`・`Text(strconv.Itoa(pid))` |
| 942 | `killOutcome` | `%w: pid=%d: %w`（`ErrKillAfterCancel`, pid, err） | `&killAfterCancelError{pid: pid, err: err}` |

`ErrKillAfterCancel` と子プロセスのエラーを両方 `errors.Is` で届かせる必要があるため、`command_lifecycle.go` に複合の型を宣言する。

```go
// killAfterCancelError reports a kill attempted after cancellation whose
// underlying cause must stay reachable together with ErrKillAfterCancel.
// Its text equals fmt.Errorf("%w: pid=%d: %w", ErrKillAfterCancel, pid, err).
type killAfterCancelError struct {
    pid int
    err error
}

func (e *killAfterCancelError) Error() string { return e.StructuredMessage().String() }

func (e *killAfterCancelError) Unwrap() []error { return []error{ErrKillAfterCancel, e.err} }

func (e *killAfterCancelError) StructuredMessage() errmsg.Message {
    return errmsg.NewMessage(
        errmsg.Cause(ErrKillAfterCancel),
        errmsg.Const(": pid="),
        errmsg.Text(strconv.Itoa(e.pid)),
        errmsg.Const(": "),
        errmsg.Cause(e.err),
    )
}
```

- `rankedError`（`:880-889`）・`(*preparedCommand).release`（`:278`）・`startPrepared`（`:498`）は範囲外なので `errors.Join` のままにする。
- `removeStagedCopy`（`:795-852`）の失敗は `pc.stagingWindowErr` に記録してログに出すだけで、返すエラーに入らない（`superviseCommand` は `:756` で戻り値を捨てる）。対象にしない。
- `runCommand` の 3 つの `errmsg.Join` は、すべて nil なら nil を返す点も `errors.Join` と同じである。

### 7.3 `internal/runner/base/privilege`

`(*Error).Error()` を `return e.StructuredMessage().String()` にし、`StructuredMessage()` を追加する。`Unwrap()` は `SyscallErr` を返す変更前のままにする。

```go
func (e *Error) StructuredMessage() errmsg.Message {
    return errmsg.NewMessage(
        errmsg.Const("privilege operation '"),
        errmsg.Text(string(e.Operation)),
        errmsg.Const("' failed for command '"),
        errmsg.Ident(e.CommandName),
        errmsg.Const("' (uid "),
        errmsg.Text(strconv.Itoa(e.OriginalUID)),
        errmsg.Const("->"),
        errmsg.Text(strconv.Itoa(e.TargetUID)),
        errmsg.Const("): "),
        errmsg.Cause(e.SyscallErr),
    )
}
```

- 変更前の書式は `privilege operation '%s' failed for command '%s' (uid %d->%d): %v`（`errors.go:34-37`）。`CommandName` は `Identifier`、操作と uid、システムコールのエラーは `Text` である。`Timestamp` は描画しない。
- `SyscallErr` は変更前も `%v` で入れているが、`Unwrap()` が `SyscallErr` を返すので `Cause` にしても到達性は変わらない。nil の `SyscallErr` は `<nil>` になり `%v` と同じである。
- `(*UnixPrivilegeManager).performElevation` の `fmt.Errorf("privilege escalation failed: %w", err)`（`unix.go:217`）を `errmsg.NewError(Const("privilege escalation failed: "), Cause(err))` にする。
- `WithPrivileges`（`unix.go:105-143`）は `performElevation` のエラーをそのまま返す（`:132-134`）。ラップしないので範囲に入れない。`escalatePrivileges`（`:295-330`）は `&Error{...}` を作って返すだけで、`ErrPrivilegedExecutionNotAvailable` のラップ（`:302`）は `Structured` を運ばず名前も挿入しないので範囲に入れない。

### 7.4 複合の型と `errmsg.Join` の使い分け

- executor の `errors.Join` の置き換えは `errmsg.Join`（`JoinedError`）を使う。子の文言を改行でつなぐ `errors.Join` と同じ意味の結合であり、子が `Structured` なら構造を保つ。
- 中断の最終エラーは `cancelledRunError`（§5.3）にする。中断であることを型で宣言するためである。
- `privilege.Error` の `SyscallErr`、`runCommand` の複合、`superviseCommand` の複合、`killAfterCancelError` の `err` は、いずれも `Cause` の部分として運ぶ。`errors.Is`・`errors.AsType` が変更前と同じ対象に届くことをテストで確かめる（§10.6）。
- #1196・#1197 でこの経路のどれかの型が `Structured` を実装したら、範囲の規則 (i) に当たる箇所が変わるので、分類を確かめ直す。

## 8. `cmd/runner`（変更）

原因を `Message` に埋め込んでいる 4 か所を、原因を `Err` に移し、`Message` を定数式の要約文にする（01 対象 4、§4.4 の #9・#10・#13・#15）。

| 関数 | `Message` | `Err` |
|---|---|---|
| `run`（global の展開、`:372-378`） | `errmsg.ConstSummary("Failed to expand global configuration")` | `err` |
| `run`（テンプレート検証、`:383-389`） | `errmsg.ConstSummary("Template validation failed")` | `err` |
| `auditConfiguredDirPermissions`（`:512-518`） | `errmsg.ConstSummary("directory permission checker initialisation failed")` | `secErr` |
| `executeRunner`（`--groups`、`:640-646`） | `errmsg.ConstSummary("Invalid groups specified")` | `err` |

- `Detail()` の文言は `Message + ": " + Err.Error()` になり、変更前の `fmt.Sprintf("…: %v", err)` と同じである（AC-18）。
- 移した原因は `PreExecutionError.Unwrap()` を通じて `errors.Is`・`errors.AsType` で届くようになる。01 が認めた到達性の追加である。
- 報告の種別と終了コードは変わらない。`mainWithExitCode`（`:219-252`）の分岐の順は `dryRunPreviewExit`・`SilentExitError`・`PreExecutionError`・`ExecutionError` であり、付け替えた原因（`config.ExpandGlobal`・`config.ValidateAllTemplates`・`newPermChecker`・`cli.FilterGroups` のエラー）は `dryRunPreviewExit`・`SilentExitError` を含まないので、4 か所とも従来どおり `PreExecutionError` として報告され、終了コードは 1 のままである。
- global の展開の原因は §6.5 で構造化されるので、AC-34 の場面では、`Message` の固定の文言と参照された変数名が置き換えられずに出る。
- `ExecutionError` の構築（`executeRunner`、`:695-702`）は `Message: errmsg.ConstSummary("error running commands")` にする。`ExecutionError` の唯一の設定箇所である。
- `cmd/runner` の 4 か所は §9.1 の範囲の検証ではなく、AC-34 のシナリオと §10.6 の単体テストで確かめる。

## 9. AC-41 のガードと AST ガード

### 9.0 errmsg のガードの前提

§9.3〜§9.5・§9.8 のガード（`internal/errmsg/errmsg_guard_test.go`）に共通する前提を定める。

- 目的: このリポジトリで開発する人と AI エージェントが通常のコードを書くときに、誤って免除の役割を宣言したり、`Error()` の文言と構造化メッセージをずらしたりすることを検出する。
- 対象外: ガードを避けるために書かれたコード。例として、`Part` を `any` などのインターフェースの値に入れて外へ渡し、型アサーションで取り出すこと、`reflect`・`unsafe` で欄を書き換えることがある。これらはコードレビューで扱う。ガードの不足をこの種の書き方で指摘されたときは、この節を根拠に対象外とする。
- 解析の方法: ガードは対象のパッケージを `go/types` で型検査し、名前の解決・定数かどうか・式と宣言の型・メソッドの集合（埋め込み・別名・昇格の深さを含む）を型検査の結果から得る。構文から型やスコープを推測しない。
- 対象のパッケージ: errmsg と、本番のファイルの import をたどって errmsg に届くパッケージである。import はどのビルドのファイルのものも数える。
- ビルドの変種: 型検査は、テストを実行する環境のビルド（GOOS と cgo の設定）が選ぶファイルで行う。サポートするビルド（linux/amd64・linux/arm64・darwin/arm64 のそれぞれで cgo の有無）の間で採否が変わるファイルを、変種のファイルと呼ぶ。どのビルドでも検査の結果が同じになるように、対象のパッケージの変種のファイルは次のことをしてはならない。
  - errmsg を import する。
  - 型を宣言する。
  - `Error`・`StructuredMessage` という名前のメソッドを宣言する。
- `//go:build !windows` のように、サポートするどのビルドにも含まれるファイルは変種のファイルではない。現在の変種のファイル（`fdexec_linux.go`・`identity_linux.go`・`trusted_gids_darwin.go` など）は、いずれも上の 3 つをしていない。

### 9.1 対象の範囲（ガードが持つ唯一の定義）

02 §3.8.1 の表を、ガードが読む形に確定する。範囲は「ファイル全体」または「関数・メソッド」の単位で持ち、ファイルと関数の名前の両方が実際のコードに見つかることを確かめる（名前を変えたり関数を消したりしたときに、警告なく検証から外れないようにする）。`contextParts` は `(*ExecutionError).contextParts` に確定する（§0.1）。

| パッケージ | ファイル全体 | 関数・メソッドの単位 |
|---|---|---|
| `internal/runner` | `group_executor.go`・`group_stage.go`・`group_errors.go` | `runner.go` の `(*Runner).Execute`・`(*Runner).ExecuteGroup`・`(*Runner).executeGroups` |
| `internal/runner/config` | `expansion.go` | `errors.go` の `(*ErrUndefinedVariableDetail).StructuredMessage`・`(Level).parts`・`(Field).parts` |
| `internal/runner/resource` | — | `normal_manager.go` の `(*NormalResourceManager).ExecuteCommand`・`executeCommandWithOutput`。`dryrun_manager.go` の `(*DryRunResourceManager).ExecuteCommand`・`evaluateCommandRisk` |
| `internal/runner/base/executor` | — | `tempdir_manager.go` の `(*DefaultTempDirManager).Create`。`executor.go` の `(*DefaultExecutor).Validate`・`validatePrivilegedCommand`・`executeNormal`・`executeWithUserGroup`。`command_lifecycle.go` の `runCommand`・`reportStartFailure`・`superviseCommand`・`killChild`・`killOutcome` |
| `internal/runner/base/privilege` | — | `errors.go` の `(*Error).StructuredMessage`。`unix.go` の `(*UnixPrivilegeManager).performElevation` |
| `internal/logging` | — | `pre_execution_error.go` の `(*PreExecutionError).DetailMessage`。`execution_error.go` の `(*ExecutionError).ReportMessage`・`(*ExecutionError).contextParts` |

`expansion.go` から除く関数（ガードは名前の存在も確かめる）。

| 除く関数 | 理由 |
|---|---|
| `ProcessEnvImport` | `env_import` の拒否のエラーを作る関数であり、01 の対象外（#1197）。`ErrUndefinedVariableDetail` はこの関数を通らない |
| `ProcessEnv` | `env_vars` の拒否のエラーを作る。`ErrUndefinedVariableDetail` はこの関数をそのまま通るだけであり、ラップしない |
| `resolveAndPrepareCommandSpec`・`ApplyTemplateInheritance`・`expandTemplateToSpec` | テンプレートのエラーを作る関数であり、01 の対象外（#1197） |

- ファイル全体を対象にするのは、group の実行と展開の経路の関数がそのファイルに集まっており、関数を分けたり加えたりしても検証から漏れないようにするためである。`runner.go`・`resource`・`executor` は対象外の経路の関数を多く含むので、関数の単位で指定する。
- 範囲に関数やファイルを加えると、その中のラップもガードの対象になり、`Identifier`・`Path` を宣言できる箇所にもなる（02 §3.8.1）。
- `internal/runner/config` の範囲で、原因が `Structured` を運びえないラップは、前置きの全体を 1 つの `Text` にする（§6.5）。これは範囲の外に出ることを意味しない。ガードの no-`fmt.Errorf` は適用される。

### 9.2 ラップの検査（AC-41）

- §9.1 の範囲の中に、`fmt.Errorf`（`%w` の有無によらない）・`errors.Join`・定数式でない引数の `errors.New` の呼び出しが無いこと。ラップせずに `err` をそのまま返すことは許す。
- 範囲の中のファイル全体の単位に属する型（レシーバがそのファイルにある型）が `Unwrap` を宣言するなら、同じ型が `StructuredMessage` も宣言すること（ファイル全体の単位の検査は残す）。
- 関数の単位の範囲では、次の明示の一覧の型が `StructuredMessage` を宣言すること: `runner.cancelledRunError`・`executor.killAfterCancelError`・`privilege.Error`。これらの型は、範囲の関数から到達する原因をラップするが、ファイル全体の単位のファイルに属さないため、上のファイル全体の検査では拾われない。ガードは、列挙した各型がコードに実在し、`StructuredMessage` を宣言することを確かめる。
- 次の例外の型は、`Unwrap` を宣言しても `Structured` を実装しなくてよい: `logging.PreExecutionError`・`logging.ExecutionError`（記録の原因を `DetailMessage`・`ReportMessage` で組み立てる）と、`internal/runner/config/errors.go` の `*...Detail` のエラー型（範囲外。#1197）。
- ガードは `internal/runner/wrap_guard_test.go`（`//go:build test`）に置く。
- 実装の指針: `identitymutationguard.ProductionGoFilesInRepo`・`ReadProductionSource`・`ParseSource`・`ResolveLocalImports` を使い、範囲の定義（ファイル全体・関数の単位・除く関数）をテストの中の表として持つ。関数の単位は、`*ast.FuncDecl` のレシーバ型名と関数名で照合する。範囲の全ファイルを調べ、`fmt.Errorf`・`errors.Join`・`errors.New` の呼び出し位置が範囲の中にあるかで判定する。

### 9.3 `Const` の検査（AC-24）

- 本番のコードの `errmsg.Const`・`errmsg.ConstSummary` の呼び出しの引数が、文字列リテラル、同じパッケージの `const` 宣言に解決できる名前、またはそれらを `+` でつないだ式であること。
- 名前は型検査で解決する。同じパッケージの定数（関数の中で宣言した定数を含む）に解決される名前だけを認め、それ以外は拒否する（fail-closed）。変種のファイル（§9.0）で宣言された定数も認めない。変種ごとに定数かどうかが変わりうるためである。
- 関数の呼び出し以外の使い方（`f := errmsg.Const` など）は拒否する。
- errmsg 自身の `Const` 呼び出し（`PathErrorCause` の区切り、`Const` を使う構築関数の内部）は、文字列リテラルだけなのでそのまま通る。
- ガードは `internal/errmsg/errmsg_guard_test.go` に置く。

### 9.4 免除の役割の検査

- `errmsg.Ident`・`errmsg.Path` の呼び出しは、§9.1 の範囲の中の位置でだけ行う。位置は「ファイルと関数」で決め、関数やメソッドの名前の一致では決めない。呼び出し以外の使い方（関数値としての参照）は拒否する。
- `errmsg` パッケージ自身の中の非修飾の呼び出し（`Ident(...)`・`Path(...)`）は、構築関数の実装と平らにする処理の中のものなので対象外とする。ガードは、`errmsg` の外から呼ばれる `errmsg.Ident`・`errmsg.Path` のセレクタ式を調べる。
- `errmsg.PathErrorCause` の呼び出しは `(*DefaultTempDirManager).Create` の中だけで行う。
- ガードは `internal/errmsg/errmsg_guard_test.go` に置き、`errmsg` の関数は名前ではなく型検査で解決する（別名 import・ドット import も同じに扱う）。
- 許可位置の表:

| ファイル | 関数・メソッド |
|---|---|
| `internal/runner/group_executor.go`・`group_stage.go`・`group_errors.go` | ファイル全体 |
| `internal/runner/config/expansion.go` | ファイル全体（§9.1 の除く関数を除く） |
| `internal/runner/config/errors.go` | `(*ErrUndefinedVariableDetail).StructuredMessage`・`(Level).parts`・`(Field).parts` |
| `internal/logging/pre_execution_error.go`・`execution_error.go` | `(*PreExecutionError).DetailMessage`・`(*ExecutionError).ReportMessage`・`(*ExecutionError).contextParts` |
| `internal/runner/base/privilege/errors.go` | `(*Error).StructuredMessage` |
| `internal/runner/base/executor/executor.go` | `(*DefaultExecutor).Validate`・`validatePrivilegedCommand`・`executeNormal`・`executeWithUserGroup` |
| `internal/runner/resource/normal_manager.go` | `(*NormalResourceManager).ExecuteCommand` |
| `internal/runner/resource/dryrun_manager.go` | `evaluateCommandRisk` |

### 9.5 文言と構造の一致（1.1 節）

- 対象のパッケージで宣言されたインターフェース以外の型のうち、`errmsg.Message` を返す `StructuredMessage` を持つもの（宣言か、埋め込みによる昇格）を調べる。`Error` と `StructuredMessage` がどのメソッドに解決されるかは、型検査のメソッドの選択（埋め込みの経路）で求める。
- その型の `Error()` は、その型が宣言するもので本体が `return <受け手>.StructuredMessage().String()` の 1 文だけであるか、`StructuredMessage` と同じ埋め込みの経路から選ばれるものであること。`errmsg.Error` もこの形で書く。
- `StructuredMessage` を宣言する型は `Error()` も宣言すること。
- 型の一覧は保守しない。
- ガードは `internal/errmsg/errmsg_guard_test.go` に置く。

### 9.6 役割を選ぶ処理の禁止（AC-25）

- `internal/redaction` の本番のコードが、`errmsg.Role` の値を作らないこと。具体的には、`errmsg.RoleText` などの定数の参照、`errmsg.Role(...)` の変換、`Segment.Role` への代入が無いこと。`switch` の `case` や比較での参照は読むだけなので許す。
- ガードは `internal/redaction/redaction_guard_test.go`（`//go:build test`）に置く。`errmsg` の定数の参照を import のパスで解決する。

### 9.7 形による判定の禁止（AC-33）

- 既存の `TestProductionCodeDoesNotProbeMultiErrorShape`（`internal/runner/group_errors_guard_test.go:335`）が、`internal/errmsg` と `internal/redaction` を含む本番のコード全体を調べている。新しい `JoinedError`・`cancelledRunError`・`killAfterCancelError` が `Unwrap() []error` を宣言することは許され、それを形で判定する分岐を書くことが拒否される。
- ガードは既存のファイルを変えずにそのまま使う。`internal/errmsg`・`internal/redaction` の新しいファイルは `ProductionGoFilesInRepo` の走査対象に自動で入る。

### 9.8 `Part` の非公開と部分を返す関数（02 §3.1.1 の契約 8）

- `errmsg.Part` の欄がすべて非公開であり、`errmsg` の外で `Part` の複合リテラル（`errmsg.Part{...}` と、`[]errmsg.Part{{...}}` のような省略形）を作っていないこと。
- `errmsg` の外に、結果の型が `errmsg.Part` を含む（`[]errmsg.Part` や、`Part` を欄に持つ型などの複合も含む）公開の関数・メソッドが無いこと。結果に `errmsg.Part` を含む非公開の関数・メソッドは §9.1 の範囲の中にあること。
- パッケージレベルの変数も同じ規則に従う。変数の型は型検査で得る。非公開の変数は、§9.1 の範囲のうちファイル全体を対象とする位置にあること。
- 結果の型が型引数であれば、その制約が許す型に `errmsg.Part` を含むかで判定する。
- 宣言の型が `Part` を含まないインターフェースである値に `Part` を入れて渡すことは、§9.0 の対象外である。
- 自己テストには、公開の `Parts()` メソッドを持つ型を与えて検出されることを確かめる。
- ガードは `internal/errmsg/errmsg_guard_test.go` に置く。

### 9.9 整形のバイトの検査

- 呼び出し側が渡すバイトが、整形として `Identifier`・`Path`・`Constant` の断片に入らないこと。
- `IndentedCause` は `error` を 1 つだけ受け取る。このシグネチャを `internal/errmsg/errmsg_test.go` のコンパイル時の固定（`var _ func(error) Part = IndentedCause`）で固定する。引数を足した呼び出しはコンパイルできず、関数値にしてもシグネチャは同じなので、AST の検査は置かない。字下げの文字列は errmsg が持つ定数であり、呼び出し側からは渡せない。
- `Const`・`ConstSummary` の引数は §9.3 で定数式に限る。`Ident`・`Path` は宣言の位置を §9.4 で限る。この 3 つで、呼び出し側のバイトが免除の役割の断片に入る経路は塞がる。
- ガードは `internal/errmsg/errmsg_guard_test.go` に置く。

### 9.10 自己テスト

各ガードに、検出すべき形を与えて検出されることを確かめる自己テストを付ける（既存の `TestMultiErrorShapeProbeCheckRecognizesForms` と同じ形）。ガードが何も見ない状態のまま通ることを防ぐためである。

- `Const` の検査: リテラル・定数名・`+` の式・解決できない名前・非呼び出しの参照を並べる。
- 免除の役割の検査: 範囲内の呼び出し・範囲外の呼び出し・別名 import・ドット import・関数値としての参照を並べる。
- `Part` の検査: `errmsg.Part{}`・`[]errmsg.Part{{}}`・公開の `Parts()` メソッドを持つ型・別パッケージの `Part` 風の型を並べる。
- 文言と構造の一致: `Error()` が 2 文の型・違う式を返す型・`*errmsg.Error` を埋め込んで `Error()` を宣言する型を並べる。
- 変種のファイルの検査（§9.0）: 変種のファイルでの errmsg の import・型の宣言・`Error` の宣言と、変種のファイルの定数を `Const` に渡す形を並べる。サポートするすべてのビルドに含まれるファイル（`//go:build !windows`）を変種のファイルとして扱わないことも確かめる。
- ラップの検査: 範囲内の `fmt.Errorf`・`errors.Join`・非定数 `errors.New`・範囲外の `fmt.Errorf` を並べる。範囲の関数名と除外関数名がコードに見つかることも確かめる。型の検査では、明示の一覧の型が `StructuredMessage` を欠く形を検出し、例外の型（`PreExecutionError`・`ExecutionError`・config の `*Detail` 型）を検出しないことを確かめる。
- AC-25: `errmsg.Role(99)` の変換・`errmsg.RoleText` の参照・`Segment.Role` への代入・`case errmsg.RolePath` を並べる。

## 10. テスト

### 10.1 `internal/errmsg`

- 平らにする契約: `Structured` の展開（直接の型だけで判定し、奥の `Structured` を探さないこと）、構造を持たない原因が `Text` になること（AC-09・AC-10）、nil の原因が `<nil>` になること、`*fs.PathError` の分解が `(*fs.PathError).Error()` と一致すること、分けない `*fs.PathError` が 1 つの `Text` になること。
- 役割の部分と原因の部分の区別: `NewMessage(Cause(nil))` が `<nil>` に、`NewMessage(Text(""))` と `Message{}` が空文字列になること。
- `String()` と `Error()` の一致: `Structured` を実装する型の `Error()` が `StructuredMessage().String()` と一致すること（§9.5 のガードと対）。
- `IndentedCause`: 末尾の除去が断片をまたぐ場合（末尾の断片が改行だけ、`\r\n` の並び）、途中の断片の改行、`Identifier`・`Path` の断片の中の改行、原因が構造を持たない場合。どれも `GroupError.Error()` と比べる。字下げの引数が API に無いことは、シグネチャのコンパイル時の固定で確かめる。
- 深い連鎖: 16 段より深い構造化エラーの連鎖の `String()` が、同じ形の `fmt.Errorf` の `%w` の連鎖の `Error()` と一致すること。深さの上限を仮に戻すとこのテストが失敗することを確かめる。
- `errmsg.Error`: 原因をちょうど 1 つラップすること、`errors.Is`・`errors.AsType` が原因に届くこと、原因の部分が 1 つでない・原因が nil の構築が panic すること。
- `Join`: 文言が `errors.Join` と一致すること、nil の子を除くこと、すべて nil なら nil を返すこと、`errors.Is` がすべての子に届くこと。
- `Freeze`: 凍結の前後で `String()`・`Segments()` が一致すること、凍結後は原因の `Error()` が呼ばれないこと（呼ぶたびに文言が変わる原因で確かめる）。
- AC-08: 役割を明示しない部分・範囲外の役割を持つ部分が `Text` として描画されること（`redactSegments` 側の AC-38 と対）。
- AC-21: `RedactingHandler` を通らない標準のハンドラ（`slog.NewTextHandler` など）に `errmsg.Message` の属性を渡し、出力が `String()` と同じ文字列になること。
- ゼロ値: 各型のゼロ値の `String()`・`Error()` が panic しないこと。

### 10.2 `redactedRanges` の差分テスト

- 正しさの義務: `ranges := c.redactedRanges(text)` の各範囲を置換文字列で置き換えた結果が `c.RedactText(text)` と一致すること。
- 種（テーブル）: 既存の `RedactText` のテストの入力の全体を種にする。`redactor_test.go` の `TestRedactText_*` と `value_detector_test.go` の `TestValueDetector_Mask_*` が種の出所である。加えて次を必ず含める。
  - 残す範囲の種類: 前置き（`Bearer `・key と区切り・`"private_key_id":"`）、後ろのグループ（`gcpSAKey` の `${2}`・`jwt` の `${1}`）、`urlCred` の `@`。
  - 段の重なり: 後の段が前の段の置換文字列に重なる入力（例: `password=-----BEGIN ...`、`Bearer ` の直後に値形式の値）。
  - 空の引用の値: `password=""`・`"password":""`・`password=''`（幅 0 の範囲）。
  - `DefaultPlaceholder` がどの規則の一致にも関与しないこと: PEM ブロックの直後に `[0-9A-Z]{16}` の文字列を続けた入力と、PEM ブロックの直後にほかの規則の前置き（`Bearer ` など）を続けた入力。
  - `WithWebhookHost`: 設定したホストの URL を置換文字列の隣に置いた入力（`https://hooks.example.com/[REDACTED]`・`[REDACTED]https://hooks.example.com/x`）。この規則は `\bhttps://` から一致を始め、パスの文字の集合は `[` を含まない（`value_detector.go:125` の `compileWebhookHostPattern`）。一致が `[REDACTED]` の中で始まることも終わることもない。
- ファジング: `FuzzRedactedRangesMatchesRedactText` を置き、上記の種を `f.Add` する。ファジングの入力でも `RedactText` との一致を確かめる。
- 幅 0 の範囲: `password=""` が `start == end` の範囲を 1 つ返すこと。同じ位置の挿入点と幅のある範囲の並び（`start` 昇順、`start` が等しければ `end` 昇順）。
- 段の重なり: 後の段の置き換える範囲が前の段の置換文字列と重なるとき、前の段の置換文字列に当たる元の範囲の全体が 1 つの範囲に含まれること。残す範囲が重なっても置換文字列は分けないこと。
- 表し方: 一致の数に比例する区間の列であること（文字列の長さに比例する表を作らないことは、入力の長さを大きくしても `ranges` の要素数が一致の数に留まることで確かめる）。

### 10.3 `RedactMessage`

- AC-01: `Identifier` の断片が、語（`api_key`・`monkey-test`）・key=value の形・値形式の検出のいずれでも書き換えられずに描画されること。
- AC-02・AC-03: `Path` の断片が語だけでは書き換えられず、`RedactText` が反応する値ではマスクされること。
- AC-04: `Text` の断片が `RedactText` の結果になり、反応せず値全体置換に当たればその断片だけが置換文字列になり、同じ本文の他の断片が残ること。
- AC-07: 機密を示す属性名の下では、構造化メッセージが値ごと置換されること（ハンドラのテスト）。
- AC-36: `Identifier` の断片だけが語を含み、`Text` の断片が語も形式も含まない本文で、`Text` の断片が書き換えられないこと。
- AC-37: 検出の種類（key=value、`Bearer `・`Basic ` の次の語、`Authorization` のヘッダ値、値形式の検出）ごとに 1 回ずつ確かめる。各入力は、接頭辞または key と値を別の断片に分ける。値形式の検出の場合の 1 つは、値形式に当たる値を `Identifier` の断片と `Text` の断片に分ける。どの入力でも、各部分だけに `RedactText` と値全体置換を適用しても秘密が見えたまま残ることを先に確かめる（01「テストの入力についての制約」）。§3.2 の手順 4・6・7 の例（`AKIAIOSFODNN7` + `EXAMPLE`、`AKIAIOSF` + `ODNN7EXAMPLE`、`Bearer ` + `opaque-credential`、`API_KEY` + `=` + 値）を表で固定する。断片の値が全体の一致で隣の断片へ延びる場合（`Text("password=secret")` + `Text("suffix")`）に、置換文字列が 1 つにまとまることも確かめる。
- AC-38: `redactSegments` に `errmsg.Segment{Role: errmsg.Role(99), Text: ...}` を直接与える。値全体置換だけが反応する入力を使い、その断片が置換文字列になること。
- 挿入点: `Constant("password=\"")`・`Text("")`・`Constant("\"")` の 3 つの断片で、出力が `RedactText("password=\"\"")` と同じ `password="[REDACTED]"` になること。
- 実行時の検査: `redactedRanges` を差し替えて不一致を起こすテスト（テスト内で範囲を改変する）で `*ErrMessageRangeMismatch` が返ること。平らにする処理が panic する原因（`StructuredMessage()` が panic する型）で `*ErrMessageFlattenPanic` が返ること。どちらも `error_message` が `RedactionFailurePlaceholder` になり、`ErrorCollector` に記録されること（ハンドラのテスト）。
- `Config` が `NewConfig` を経ていない場合に `RedactionFailurePlaceholder` を返すこと。
- 性能: 100 group の失敗を連結した最終の実行エラー（数十 KiB）を入力にした `BenchmarkRedactMessage` を `BenchmarkRedactText`（`redactor_test.go:3569`）に並べて置き、1 回の描画が 10 ms 以下であることを確かめる（§3.6）。

### 10.4 ハンドラ

- 構造化メッセージの属性が、`RedactingHandler` の後段のハンドラに文字列として渡ること（AC-20）。
- 属性名が機密を示す場合に値ごと置換されること（AC-07）。
- 失敗が `ErrorCollector` に記録されること。
- `Config.RedactLogAttribute`（公開の関数）に構造化メッセージを渡しても、redaction 前の文字列がそのまま出ないこと。
- `Config.RedactLogAttribute` に `*errmsg.Message`・`slog.Any` で包んだ別の `LogValuer`・panic する `LogValuer` を渡すと、`RedactionFailurePlaceholder` になること。

### 10.5 各エラー型

- 構造化メッセージの役割の並び（§5.1・§6.3・§7.1〜§7.3 の表）。各断片の役割と文字列を直接確かめる。
- `Error()` の文言が変更前と同じであること。各ラップについて、代表的な値で文言を固定する（group_executor.go の 11 か所、expansion.go の 16 か所、resource の 16 か所、executor.go の 12 か所、tempdir_manager.go の 2 か所、command_lifecycle.go の 11 か所、privilege の 3 か所、logging の `DetailMessage`・`ReportMessage`）。
- ゼロ値の `Error()` が panic しないこと。
- `Level`・`Field` の `String()` が変更前に `fmt.Sprintf` で作っていた文字列と一致すること（キーごとに 1 つ）。
- `ErrUndefinedVariableDetail` の部分の並び（`Chain` が空・非空の両方）。
- 一時ディレクトリ: 2 つのラップのそれぞれで文言を確かめる。`os.Chmod` の失敗は `os.MkdirTemp` の失敗とは別に起こして確かめる（原因の `*fs.PathError` の `Op`・`Path`・`Err` を直接作れるなら、FS のモックで起こす）。
- `cancelledRunError`: 文言が `errors.Join(ctxErr, err)` と同じであること、`errors.Is(err, context.Canceled)` と失敗した group の原因への到達性（AC-32）。
- `killAfterCancelError`: 文言が変更前の `fmt.Errorf` と同じであること、`errors.Is` が `ErrKillAfterCancel` と子のエラーの両方に届くこと。`killChild` の no-privilege-manager の経路と `killOutcome` の経路の両方で確かめる。
- `(*privilege.Error).StructuredMessage` の部分の並びと、`errors.Is`・`errors.AsType` の到達性（`SyscallErr`・番兵）。
- `errmsg.Join` を使った `runCommand`・`reportStartFailure`・`superviseCommand` の文言が、変更前の `errors.Join` と同じであること。2 つの呼び出し元の経路のそれぞれで、`errors.Is` が変更前と同じ対象に届くこと。
  - `pc.spent` の経路（`command_lifecycle.go:557`、`ErrPreparedCommandSpent`）。既存の `TestRunCommand_ChildStateTransitions/spent_command_stays_not_started`（`executor_supervise_test.go:433`）が `errors.Is` を確かめている。
  - 開始の失敗の経路（`:591`）。既存の `TestExecute_FdBoundStartFailureNoLeak`（`executor_fdexec_test.go:111`）と `TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow`（`executor_lifecycle_test.go:815`）が通る。
- AC-41: 検出される語を含むコマンド名が、`error_message` で置き換えられずに出ること。入力は `Identifier` の免除だけが効くものにする。先に、コマンド名を `Text` として redaction すると置き換えられることを確かめる（値全体置換だけが反応する語を使い、key=value・値形式の検出は反応しない形にする）。

### 10.6 統合テストと端から端までの確認

- AC-12・AC-16・AC-31・AC-34 の例示のシナリオは、エラーの発生元から `RedactingHandler` を通った後のレコードまでを通す（01「テストの入力についての制約」）。Slack に届くレコード（AC-12・AC-34）は、Slack のメッセージ組み立てまでを通す。AC-12・AC-34 は `vars` の中の未定義変数を使う。
  - `vars` の経路を通るシナリオの端から端までのテストは、`vars` の中の未定義変数を使う。定義側の変数名（`Field` の中の名前）が語を含む場合は、その名前以外の本文だけでは値全体置換が起きないことを先に確かめる。
- AC-31・AC-32 の中断のシナリオ: `executeGroups` の経路で、コンテキストを中断してから group を失敗させる。`cancelledRunError` の文言と到達性、`error_message` に中断の原因と失敗した group の原因が出ることを確かめる。
- AC-34: global の展開で未定義の変数 `api_key` を参照し、Slack の `Error Message` に `Failed to expand global configuration` と変数名 `api_key` が出ること。
- AC-19・AC-22・AC-23: 既存のテストが変更なしで通ることで確かめる。stderr の `Details:`、Slack の補間契約、通知の構成。
- 記録の凍結: 呼ぶたびに文言が変わる原因を持つ構造化メッセージで報告し、stderr の `Details:` と記録された `error_message` の redaction 前の文字列が同じ 1 回の結果から来ること（原因の `Error()` が 1 回だけ呼ばれること）を確かめる。
- `make slack-group-notification-test` の場面に、group 名が語を含む group 実行前段の失敗を加え、Slack の表示を確かめる（手動・半自動）。

### 10.7 既存テストの更新

- 変わる既存のテスト（挙動や型が変わるので更新が要る）:
  - `PreExecutionError`・`ExecutionError` のリテラルで `Message` に文字列を渡すテスト（`internal/logging/pre_execution_error_test.go` 16、`internal/runner/runerrors/pre_execution_guard_test.go` 7、`internal/logging/notification_contract_guard_test.go`、`cmd/runner/main_test.go` 2、`cmd/runner/integration_slack_flush_test.go` 1 など）。書き換えは機械的で、確かめる内容は変わらない。`Message: "..."` は `Message: errmsg.ConstSummary("...")`（固定の文言）または `Message: errmsg.TextSummary("...")`（値を含むもの）にする。
  - `cmd/runner/main_test.go:730`: `preExec.Message` が原因の文言を含むことを確かめている。原因が `Err` に移るので、`assert.ErrorIs(t, err, errCheckerUnavailable)` と `preExec.Detail()` の確認に変える。
  - `ErrUndefinedVariableDetail` の `Level`・`Field` を文字列として比べる `internal/runner/config` のテスト。`.String()` で比べる。`ErrUndefinedVariableDetail{Level: "global"}` のようなリテラルは `Level: globalLevel()` にする。
  - `internal/runner/group_stage_test.go`: `def.message` の非空確認と `got.Message` の比較を `.String()` にする（`:33`・`:126`・`:164`・`:183`）。テスト内の期待表（`:64-106`）の `message` は文字列のままにし、比較の右辺を `.String()` にする。
- 確認が要る既存のテスト: 記録された `error_message` を `attr.Value.String()` で読むテスト（`internal/logging/pre_execution_error_test.go:594`・`:658`・`:998`、`internal/runner/multi_group_error_integration_test.go:48`、`internal/runner/group_stage_test.go:183` など）。値は `slog.KindLogValuer` になるが、`slog.Value.String()` は `fmt` を通して `Message.String()` を呼ぶので、読み出す文字列は変わらない見込みである。実装のときに実行して確かめる。
- 変わらない既存のテスト: `error` 属性の全文が値全体置換を受けることを固定するテスト（`internal/redaction/redactor_test.go:3947` の `failed to execute group monkey`）。対象外の属性（01 対象外「`error_message` 以外の属性」）についてのものである。
- `withPlaceholder` を使う 3 か所（`redactor_test.go:1728`・`:3352`・`:3767`）の扱い:
  - `:1728`（`placeholder reaches both redaction layers`）と `:3767`（`the placeholder option reaches the configured-host pattern`）の subtest は、オプションを確かめるためだけのテストなので削除する。
  - `:3352`（`TestRedactText_ValueBasedDetection_BypassWhenNil`）は、置換文字列は付随的な使い方なので `NewConfig()` の既定の置換文字列に替え、期待値を `password=[REDACTED] value AKIAIOSFODNN7EXAMPLE` に改める。
  - 削除の後、`go tool cover -func` の結果が関数ごとに変わらないことを確かめる。`WithPlaceholder` 自体の行は消える。`Placeholder()` は `environment_validation.go:21` と `environment_validation_test.go:232` が通るので、被覆は残る。
- `WithAdditionalKeyValuePatterns` を使う 5 か所（`redactor_test.go:645`・`:654`・`:1574`・`:1712`・`:1721`）の扱い:
  - `:645`（`user-added key is redacted with the loose boundary`）は、利用者が足すキーの境界を固定するテストで、足す経路が無くなるので削除する。
  - `:654`（`zero value of PatternKind is the key kind`）は、`applyPattern(t, KeyValuePattern{Literal: "passphrase"}, DefaultPlaceholder, "passphrase: xyz")` が `passphrase: [REDACTED]` になる形に書き換える（`applyPattern` は `redactor_test.go:48`）。
  - `:1574`（`TestPerformKeyValueRedaction/unknown kind never reaches the redaction path`）は、`NewConfig` を通す後半を削除し、`compilePattern` での拒否（`:1571`）だけを残す。
  - `:1712`・`:1721`（`TestNewConfig_RejectsInvalidPatterns` の 2 つの subtest）は、足す経路の検証なので削除する。`WithPlaceholder` の subtest（`:1728`）も削除するので、関数ごと消える。パターンの検証そのものは `TestKeyValuePattern_Validate` に残る。
  - 削除の後、`go tool cover -func` の結果が関数ごとに変わらないことを確かめる。`NewConfig` の中の、パターンの検証とコンパイルが失敗したときに返す分岐は、既定の規則だけでは届かなくなる。これらの分岐は §3.5 のとおり残し、`NewConfig` の網羅率の差をこの判断として実装計画に記録する。
- `internal/logging/notification_contract_guard_test.go`: `PreExecutionError` のリテラルを AST で調べる。`Message` の値が `errmsg` の構築関数の呼び出しになっても、`NotificationContext` と `Component` の検査は変わらない見込みである。実行して変化を確認し、必要なら機械的に合わせる。
- 既存テストの削除・書き換えの後、`go tool cover -func` を関数ごとに比較し、`WithPlaceholder` と `TestNewConfig_RejectsInvalidPatterns` の消滅以外に差が無いことを確かめる（CLAUDE.md「Deleting a test is a claim that must be checked」）。

### 10.8 新しいテストファイル

- `internal/errmsg/errmsg_test.go`: §10.1。
- `internal/errmsg/errmsg_guard_test.go`: §9.3〜§9.5・§9.8・§9.9・§9.10。
- `internal/redaction/ranges_test.go`: §10.2。
- `internal/redaction/message_test.go`: §10.3・§10.4。
- `internal/redaction/redaction_guard_test.go`: §9.6・§9.10。
- `internal/runner/wrap_guard_test.go`: §9.2・§9.10。

## 11. 文書（AC-26）とパッケージ一覧

### 11.1 `security-architecture.ja.md`・`.md`

`docs/dev/architecture_design/security-architecture.ja.md` の `:645-651`（「識別子の型宣言による免除」の末尾の段落）を、本設計に合わせて書き換える。書き換える内容は次のとおりである。

- 構造化メッセージの役割ごとの redaction（`Constant`・`Identifier`・`Path`・`Text`）と、`Path` を値全体置換の対象外とする境界。
- 構造を持たないエラーが `Text` として扱われること（fail-closed）。
- `Identifier` の免除が変数名にも及ぶこと（5.2 節の R2。`%{AKIA…}` のような名前もそのまま出る）。
- 全体の検出範囲が、名前やパスの後ろの固定の文言や原因の一部を置き換えることがあること（6.2 節）。
- 用語を用語集の「値全体置換」にそろえる（旧称「値まるごと判定」）。
- 戻し方: 実行時のスイッチは無い。戻す場合は、構造化メッセージを記録に使う変更を含む PR を revert する。`RedactText` は変えないので、ほかのログの redaction は revert の影響を受けない。

英語版（`security-architecture.md`）は `/mktrans` で日本語版と同じ内容を反映する。既存の記述（免除は宣言された値だけに掛かり、message・error 文字列に連結された識別子は免除の対象外である）のうち、`error_message` の構造化メッセージで宣言された断片についての記述を本設計に合わせ、`error` 属性・`record.Message` は変更前と同じ扱いであることを残す。

### 11.2 `package_reference.md`

`docs/dev/developer_guide/package_reference.md` のディレクトリ構造（`internal/` の一覧）に `errmsg/` を追加し、Package Responsibilities に `internal/errmsg` の責務（役割付きの部分の列としてエラー本文を運び、部分ごとの redaction の単位を与える末端のパッケージ）を追記する。`internal/identifier` の記述の並びに置く。

## 12. AC 対応

02 §10 の対応表が規範である。本節は、本書で具体化した主な箇所との対応を補う。

| AC | 本書の箇所 |
|---|---|
| AC-01・AC-02・AC-03・AC-04・AC-07・AC-08・AC-36・AC-37・AC-38 | §3.2・§3.3・§10.3・§10.4 |
| AC-09・AC-10・AC-11 | §2.4・§3.2 手順 5・§10.1・§10.3 |
| AC-12・AC-16・AC-31・AC-34 | §5.3・§6.3・§8・§10.6 |
| AC-18 | §2.4.1・§4.1・§4.2・§5.1・§5.2・§6.4・§6.5・§7・§8・§10.5 |
| AC-19・AC-22・AC-23 | §4.3・§10.6 |
| AC-20・AC-21 | §2.3・§3.3・§10.1・§10.4 |
| AC-24 | §9.3 |
| AC-25 | §9.6 |
| AC-26 | §11.1 |
| AC-27 | `make test`・`make lint`（§13 の各段階で実行する） |
| AC-32 | §5.3・§10.5 |
| AC-33 | §2.7・§9.7 |
| AC-35 | §4.4 #22 |
| AC-41 | §9.1・§9.2・§10.5 |

## 13. 実装順序

02 §8 の順序を、本書のファイル単位に落とす。1〜3 はそれだけで既存の出力を変えない（構造化メッセージを記録する箇所がまだ無く、`RedactText` も挙動を変えない）。4 から後で、記録が構造化メッセージになる。

1. `internal/errmsg`: 型・構築関数・平らにする処理・`IndentedCause`・`Join`・`Freeze`・`errmsg_test.go`・`errmsg_guard_test.go`。
2. `internal/redaction` の範囲を返す関数: `span` の共通化、`redactedRanges`、差分テストとファジング、`WithPlaceholder`・`WithAdditionalKeyValuePatterns` の削除、被覆の確認、性能の確認。
3. `Config.RedactMessage`・`redactSegments`・`redactMessageAttribute`・失敗の型・ハンドラの分岐・`redaction_guard_test.go`。
4. `internal/logging`: `PreExecutionError`・`ExecutionError`・`DetailMessage`・`ReportMessage`・`contextParts`、記録と凍結、`Message` のリテラルの書き換え（§4.4）、既存テストの更新。この段階で記録が構造化メッセージになる。
5. `internal/runner`: 4 つのエラー型の `Structured`、`group_executor.go` のラップ、`cancelledRunError`、段階の定義表。
6. `internal/runner/config`: `Level`・`Field`、`ErrUndefinedVariableDetail`、`expansion.go` のラップ、`ExpandWorkDir`、`validation.go`・`template_expansion.go` の引数の型。
7. `internal/runner/resource`・`internal/runner/base/executor`・`internal/runner/base/privilege`・`cmd/runner`。
8. AC-41 のガード、例示のシナリオのテスト、文書（§11）、`make test`・`make lint`。

各段階で `make fmt`・`make test`・`make lint` を通す。4 以降の各段階では、変更した箇所の `Error()` の文言が変更前と同じであることを §10.5 のテストで確かめる。
