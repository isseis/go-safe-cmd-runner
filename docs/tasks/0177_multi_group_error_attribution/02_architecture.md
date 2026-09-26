# アーキテクチャ設計書: 複数 group 失敗時のエラー行への group 帰属の表示

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-26 |
| Review date | - |
| Reviewer | - |
| Comments | 2026-09-26: 要件の改訂（F-006 コマンドのタイムアウト、F-007 `output_size_limit = 0`、F-008 設定の検証と出力の保持の上限）に合わせて改訂した。 |

## 0. 前提

- 要件: [`01_requirements.md`](01_requirements.md)（`approved`）
- 本書の現状の記述と `file:line` は、コミット `8f7f7681` のコードを読んで確認したものである（それ以降、Go のコードは変更されていない）。
- 用語（要件と同じ）:
  - 「外側の context」は、`ExecutionError.GroupName`・`CommandName` から作る `(group: ..., command: ...)` の表示を指す。
  - 「実行全体の context」は、`executeGroups` が受け取る `context.Context`（`ctx`）を指す。`cmd/runner/main.go:271` の `signal.NotifyContext` が作り、SIGINT・SIGTERM で取り消される。本番では、これより上位に期限を持つ context は無い。
  - 「実行全体の中断」は、実行全体の context が取り消された状態を指す。「タイムアウト」は、コマンド自身の `timeout` による期限切れを指す。
  - 「出力ファイル」は、コマンドの `output` に指定したファイルを指す。executor は、出力ファイルがあるときだけ出力の書き込み先（`OutputWriter`）を受け取る。

## 1. 設計の全体像

### 1.1 設計原則

- **原因の文言は差し替えない。** 報告に出す原因は常に `Error()` の文言とする。原因の一部を別の文言に置き換える仕組み（`UserFriendlyError`）は削除する。
- **group の失敗は型で宣言する。** `executeGroups` が失敗した group の一覧を専用の型で返す。呼び出し側は、その型が持つ失敗の件数で外側の context を決める。`Unwrap() []error` を持つかどうかという形では判定しない（CLAUDE.md「Declare, don't infer」）。
- **失敗 1 件は複数件の特殊な場合として扱う。** 件数によらず同じ型を返し、1 件用の別の形を持たない。
- **不変条件は型で守る。** 新しい型のフィールドは非公開とし、生成は構築関数だけが行う。構築関数は呼び出し側の誤り（空の一覧・nil の要素・空の group 名・nil の原因）を panic で拒否する（CLAUDE.md「Enforce invariants with the type」「Reject, don't normalize」）。既存の `GroupStageError`（`internal/runner/group_stage.go:113-118`、構築関数 `:150-197`）と同じ形にそろえる。
- **中断は実行全体の context の状態で判定する。** エラーが `context.Canceled`・`context.DeadlineExceeded` を含むかどうかは、どの context が取り消されたかを表さない。実行全体の context の状態は、中断されたかどうかを直接表す。
- **設定の誤りは読み込みで拒否する。** 負の `output_size_limit` は、負の `timeout` と同じく設定の読み込みで拒否する。以後の処理は、上限が 0 以上であることを前提にできる。
- **文言の互換を保つ。** 新しい型の `Error()` は、変更前の戻り値（`fmt.Errorf("failed to execute group %s: %w", ...)` と `errors.Join`）と同じ組み立て方の文言を返す。

### 1.2 概念モデル

`GroupErrors` は 1 回の実行で失敗した group の一覧であり、1 件以上の `GroupError` を持つ。`GroupError` は失敗した 1 つの group を表し、group 名・command 名・原因を持つ。型名は要件の仮称 `GroupErrors` に合わせ、エラー型の `…Error` 接尾辞（`CommandExecutionError`・`GroupStageError`）にそろえた。

```mermaid
classDiagram
    class GroupErrors {
        <<new>>
        -errs []*GroupError
        +Errors() []*GroupError
        +Error() string
        +Unwrap() []error
    }
    class GroupError {
        <<new>>
        -group string
        -command string
        -err error
        +GroupName() string
        +CommandName() string
        +Error() string
        +Unwrap() error
    }
    class CommandExecutionError {
        <<existing>>
        +GroupName string
        +CommandName string
        +Err error
    }
    class GroupStageError {
        <<existing>>
        +CommandName() string
    }
    GroupErrors "1" *-- "1..*" GroupError : holds
    GroupError ..> CommandExecutionError : reads command name
    GroupError ..> GroupStageError : reads command name
```

矢印の意味: `*--` は「保持する」、`..>` は「構築時に原因のチェーンから型で command 名を読み取る」を表す。

Legend: クラス図は色分けを使わない。`<<new>>` は本タスクで追加する型、`<<existing>>` は変更しない既存の型を表す。

## 2. システム構成

### 2.1 全体構成（変更前と変更後）

矢印 A → B は「A の結果（エラー値）を B が受け取る」を表す。ノードは処理を行う関数である。

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    subgraph Before["変更前"]
        B_EG["runner.executeGroups"]
        B_CTX["main.executionErrorContext"]
        B_HEE["logging.HandleExecutionError"]
        B_FC["logging.formatCause"]
        B_OUT[("stderr Details: / error_message")]
        B_EG -->|"ラップしたエラー<br>または errors.Join"| B_CTX
        B_CTX --> B_HEE --> B_FC --> B_OUT
    end

    subgraph After["変更後"]
        A_EG["runner.executeGroups"]
        A_CTX["main.executionErrorContext"]
        A_HEE["logging.HandleExecutionError"]
        A_OUT[("stderr Details: / error_message")]
        A_EG -->|"*GroupErrors"| A_CTX
        A_CTX --> A_HEE --> A_OUT
    end

    class B_EG,B_HEE process
    class B_CTX,B_FC problem
    class A_EG,A_CTX,A_HEE enhanced
    class B_OUT,A_OUT data
```

変更前の `executionErrorContext` は `Unwrap() []error` の有無で判定し、`formatCause` は `UserMessage` で原因を差し替える。変更後は `formatCause` を削除し、`executionErrorContext` は `*GroupErrors` の件数で判定する。

Legend:

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;
    L1[("出力先")]
    L2["変更しない既存の関数"]
    L3["変更する関数"]
    L4["削除または置き換える既存の処理"]
    class L1 data
    class L2 process
    class L3 enhanced
    class L4 problem
```

### 2.2 コンポーネント配置

矢印 A → B は「A が B を import する」を表す。本タスクは新しい import を追加しない。図は本タスクで変更するパッケージの間の既存の依存だけを示す。`internal/logging` は `internal/runner` を import しないままである。

```mermaid
flowchart LR
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    CMD["cmd/runner"]
    RUN["internal/runner"]
    CFG["internal/runner/config"]
    LOG["internal/logging"]
    OUT["internal/runner/base/output"]
    EXE["internal/runner/base/executor"]

    CMD --> RUN
    CMD --> LOG
    RUN --> LOG
    RUN --> CFG
    RUN --> OUT
    OUT --> EXE

    class CMD,RUN,CFG,LOG,OUT,EXE enhanced
```

依存の根拠: `cmd/runner/main.go:733`（`runner.CommandExecutionError`）、`internal/runner/group_stage.go:7`（`logging` の import）、`internal/runner/group_executor.go:20`（`config` の import）、`internal/runner/runner.go:21`（`base/output` の import）、`internal/runner/base/output/capture.go:9`（`base/executor` の import）。

Legend:

```mermaid
flowchart LR
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    L1["本タスクで変更するパッケージ"]
    class L1 enhanced
```

### 2.3 データの流れ（2 つの group が失敗する場合）

```mermaid
sequenceDiagram
    participant EG as runner.executeGroups
    participant GE as GroupExecutor.ExecuteGroup
    participant RUN as main.run
    participant CTX as main.executionErrorContext
    participant MX as main.mainWithExitCode
    participant H as logging.HandleExecutionError

    EG->>GE: group-1
    GE-->>EG: *CommandExecutionError
    EG->>EG: newGroupError("group-1", err)
    EG->>GE: group-2
    GE-->>EG: *CommandExecutionError（原因に *CaptureError）
    EG->>EG: newGroupError("group-2", err)
    EG-->>RUN: *GroupErrors（2 件）
    RUN->>CTX: executionErrorContext(err)
    CTX-->>RUN: "", ""（2 件なので空）
    RUN-->>MX: *ExecutionError（GroupName・CommandName は空）
    MX->>H: HandleExecutionError
```

`*ExecutionError` は `run` が組み立て（`cmd/runner/main.go:692-702`）、`mainWithExitCode` が `HandleExecutionError` を呼ぶ（`:238-239`）。stderr の出力は次のようになる（途中の文言は例）。

```
  Details: error running commands: failed to execute group group-1: command fail-cmd in group group-1 failed: ...
           failed to execute group group-2: command cap-cmd in group group-2 failed: command execution failed: output capture error during execution phase: output size limit exceeded for '/path/to/out' (limit: 1048576 bytes)
```

## 3. コンポーネント設計

### 3.1 `internal/runner`: `GroupError` と `GroupErrors`（新規）

新しいファイル `internal/runner/group_errors.go` に置く。

```go
// GroupError is one group that failed during a run.
type GroupError struct {
	group   string // GroupSpec.Name of the failed group
	command string // command the failure belongs to, or "" when none
	err     error  // the error ExecuteGroup returned
}

func (e *GroupError) GroupName() string
func (e *GroupError) CommandName() string
func (e *GroupError) Error() string
func (e *GroupError) Unwrap() error

// GroupErrors reports every group that failed during a run. It always holds
// at least one GroupError.
type GroupErrors struct {
	errs []*GroupError
}

func (e *GroupErrors) Errors() []*GroupError // a copy
func (e *GroupErrors) Error() string
func (e *GroupErrors) Unwrap() []error

func newGroupError(group string, err error) *GroupError
func newGroupErrors(errs []*GroupError) *GroupErrors
```

- **group 名。** `executeGroups` が実行した `GroupSpec.Name` を渡す。エラーから取り出さない。
- **command 名。** `newGroupError` が原因のチェーンから型で読む。`*CommandExecutionError` があればその `CommandName`、なければ `*GroupStageError` の `CommandName()`、どちらも無ければ空とする。
  - `ExecuteGroup` の本番実装が返すエラーは、この 2 型のどちらかを含む。コマンド実行前の失敗は `groupExecutionExitError` が `*GroupStageError` にそろえる（`internal/runner/group_executor.go:241-249`）。コマンド実行後の失敗は `executeSingleCommand` が `*CommandExecutionError` を返す（`:645-649`、`:668-672`）。
  - どちらも含まないエラー（テストのモックなど）では、command 名は空になる。
- **構築関数の拒否。** `newGroupError` は空の group 名と nil の原因で、`newGroupErrors` は空の一覧と nil の要素で panic する。`newGroupErrors` は受け取ったスライスをコピーして持つ。
- **`Error()`。** `GroupError.Error()` は `fmt` で `failed to execute group <group>: <原因の Error()>` を組み立てる。`GroupErrors.Error()` は各要素の `Error()` を `"\n"` でつなぐ。これは変更前の `fmt.Errorf("failed to execute group %s: %w", ...)` と `errors.Join` と同じ組み立て方である。AC-09 は「同じ原因から変更前の組み立て方で作った文言と一致する」と読む（原因の文言そのものは AC-16 で変わりうる）。
- **`Unwrap() []error`。** 各 `GroupError` を返す。`GroupError.Unwrap()` は原因を返す。これにより `errors.Is`・`errors.AsType` が各 group の原因に届く（AC-10）。`Unwrap() []error` は到達のためだけに持ち、「複数 group の失敗」の判定には使わない。
- **`Errors()` はコピーを返す。** 呼び出し側が一覧を書き換えて不変条件を崩せないようにする。

`cmd/runner` のテストは `*GroupErrors` を作る必要があるので、`internal/runner/test_helpers.go`（`//go:build test`）に次を置く。command 名は引数で直接与える（原因のチェーンから読む処理の検証は `internal/runner` のテストで行う）。

```go
func NewGroupErrorForTest(group, command string, err error) *GroupError
func NewGroupErrorsForTest(errs ...*GroupError) *GroupErrors
```

### 3.2 `internal/runner`: `executeGroups`（変更）

**失敗の集め方。**

- 失敗した group ごとに `newGroupError(group.Name, err)` を作って集める。現状の `fmt.Errorf("failed to execute group %s: %w", ...)`（`internal/runner/runner.go:436`、`:450`）を置き換える。
- 終了時、失敗が 1 件以上なら `newGroupErrors` で返す。失敗が無ければ `nil` を返す。戻り値の型は `error` のままとし、nil の `*GroupErrors` を `error` として返してはならない（non-nil の `error` になるため）。
- 1 件のときに分けて返す処理（`:457-459`）と `errors.Join`（`:463`）は削除する。

**`ExecuteGroup` がエラーを返したときの処理の順序（F-006）。** 次の順に行う（§6.3 に図）。

1. **group ファイル検証の失敗を通知する。** 対象は、現状の分岐で検証の経路（`:441-448`）に入るエラーと同じである。つまり、`*verification.Error` を含み、かつ `*GroupStageError` を含まないか、含んでもその段階がファイル検証である（既存の `isGroupFileVerificationFailure`: `:469-475`）エラーである。ファイル検証以外の段階の `*GroupStageError` は、チェーンに `*verification.Error` を含んでいても段階の経路（3）で扱う（`:426-431` のコメント、Task 0176）。現状と同じく通知して、失敗としては集めない。この通知は中断の判定より前に行う。
   - `verifyGroupFiles` は context を受け取らない（`internal/runner/group_executor.go:215`）ため、現状、このエラーは `context.Canceled` を含まず、中断の分岐（`:422`）に入らない。つまり現状は、中断と重なっても検証の失敗は必ず通知される。判定を context の状態に変えると、検証中に SIGINT・SIGTERM を受けたときに通知が落ちるので、改ざん検知の通知を落とさないよう、通知を先に行ってこれを保つ。
2. **実行全体の中断を判定する。** 実行全体の context が取り消されていれば（`ctx.Err() != nil`）、`errors.Join(ctx.Err(), err)` を返し、残りの group を実行しない（AC-21）。
   - エラーの中身（`errors.Is(err, context.Canceled)`・`context.DeadlineExceeded`: `:422`）では判定しない。この分岐は削除する。
   - `ctx.Err()` を加えるのは、中断を戻り値で宣言するためである。利用者の Ctrl-C や systemd の停止はプロセスグループ全体（systemd では cgroup 全体）に届くので、コマンドの子プロセスも同じシグナルで終わる。子の終了が executor の中断の検出より先に観測されると、`ExecuteGroup` のエラーは `context.Canceled` を含まず「終了コード 130」などになる（`internal/runner/base/executor/command_lifecycle.go:687-693` の `select` はどちらが先に準備できたかに依存する）。`ctx.Err()` を加えれば、`errors.Is(err, context.Canceled)` が常に成り立ち、報告に `context canceled` が出る。`ExecuteGroup` のエラーも到達可能なまま残る。
   - それまでに集めた失敗は報告しない（要件の対象外「実行全体の中断時に集めた失敗を報告すること」、§4.4）。
3. **実行前段の失敗（`*GroupStageError`、1 の対象を除く）を通知し、集める。** 現状の分岐（`:432-436`）のとおり。1 と 3 の振り分けは現状と同じであり、既存のテスト `TestRunner_StageDispatchPrefersStageOverVerificationError`・`TestRunner_FileVerificationStageKeepsExistingPath`（`internal/runner/runner_test.go`）が引き続き通ることで確かめる。中断された実行では 2 で返るので通知しない。この順序は現状と同じである（Task 0176 のテスト `TestRunner_CancellationSkipsStageNotification` が固定している）。
4. **それ以外を集める。** コマンドのタイムアウト（エラーが `context.DeadlineExceeded` を含む）もここに入り、次の group へ進む（AC-19、AC-20）。

**変えないもの。**

- group の間で実行全体の context の取り消しを検出したら、`ctx.Err()` をそのまま返す（`:413-417`）。
- タイムアウトしたコマンドの group では、そのコマンドで group の実行が止まり、`command_group_summary` が通知される（`internal/runner/group_executor.go:282-289`、`:175-180`）。

### 3.3 `cmd/runner`: `executionErrorContext`（変更）

次の順で判定する。

1. `err` が `*runner.GroupErrors` を含むとき、要素が 1 件ならその `GroupName()`・`CommandName()` を返し、2 件以上なら空を返す（AC-05、AC-08、AC-15）。
2. 含まないとき、`*runner.CommandExecutionError` を含めばその group 名・command 名を返す。
3. どちらでもなければ空を返す。

1 を 2 より先に判定しなければならない。`*GroupErrors` は `Unwrap() []error` で各原因に届くので、2 件以上の失敗でも `errors.AsType[*runner.CommandExecutionError]` は先頭の失敗に一致してしまう。

2 が残るのは、`executeGroups` が `*GroupErrors` を経由せずにエラーを返す経路、つまり実行全体の中断（§3.2 の 2）があるためである。コマンドの実行中に中断されると、戻り値は `errors.Join(ctx.Err(), <*CommandExecutionError を含むエラー>)` になり、2 はその group 名・command 名を返す。変更前も、この経路のエラーには外側の context が付いていた（`cmd/runner/main.go:733-735`）。2 が `*GroupStageError` を読まないのも現状と同じである。

コマンドのタイムアウトは、§3.2 の変更により `*GroupErrors` の要素になるので、1 で判定される。タイムアウトが 1 件だけのとき、1 はそのコマンドの group 名・command 名を返す。これは変更前に 2 が返していた値と同じである（AC-22）。タイムアウトのエラーが `*CommandExecutionError` を含むことは次による。

- `executeSingleCommand` は各コマンドを、コマンドの `timeout` を期限とする context で実行する（`internal/runner/group_executor.go:574-589`）。
- 期限切れのとき、executor は `ctx.Err()` をコマンドのエラーに `errors.Join` で加える（`internal/runner/base/executor/command_lifecycle.go:890-891`）。
- それを `*CommandExecutionError` がラップする（`group_executor.go:645-649`）。

`Unwrap() []error` の有無による判定（`cmd/runner/main.go:730-732`）は削除する。

### 3.4 `internal/logging`: 原因の差し替えの削除（変更）

- `UserFriendlyError`（`internal/logging/execution_error.go:22-27`）・`GetUserFriendlyMessage`（`:31-36`）・`formatCause`（`:45-58`）を削除する。
- `PreExecutionError.Detail()`（`internal/logging/pre_execution_error.go:93-98`）と `HandleExecutionError`（`:243-276`）は、原因として `Err.Error()` をそのまま使う。
- `HandleExecutionError` の「外側の context を `Message` の直後、原因の前に置く」順序と、`handleErrorCommon` の複数行の字下げ（`:156-159`）は変えない。
- 複数 group の失敗では、`GroupErrors.Error()` が各 group の文言を改行でつなぐ。各 group の文言の**先頭の行**は `failed to execute group <group>: ` で始まる（AC-01、AC-02）。1 つの group の原因が複数行のときの扱いは §4.4 を参照。

`PreExecutionError.Detail()` の文言への影響は次のとおりである。

- `UserMessage` を持つ唯一の型 `*output.CaptureError` を作るのは `Capture.WriteOutput`（`internal/runner/base/output/capture.go:43`、`:64`）だけである。これはコマンド実行中に出力を書くときだけ呼ばれる（`Capture.Write`: `:32-34`）。したがって、実行前エラーの原因にこの型は入らない。
- `formatCause` は `UserMessage` の差し替えのほかに、トップレベルの `Unwrap() []error` を子ごとに分けていた。`fmt.Errorf` に `%w` が 2 つ以上あるエラーもこれに当たる。このとき分けた結果には、`fmt.Errorf` のエラー書式の文言が含まれない。削除後は `Error()` の文言になり、エラー書式の文言が残る。`PreExecutionError.Err` の設定箇所のうち確認したもの（`internal/runner/bootstrap/config.go:82`、`:100`、`:112`、`internal/runner/bootstrap/environment.go:148`、`internal/runner/group_stage.go:210`）には、そのようなエラーは無い。そのような原因が今後現れても、文言は情報が増える方向に変わるだけである。

### 3.5 `internal/runner/base/output`: `CaptureError` と `Capture.WriteOutput`（変更）

```go
type CaptureError struct {
	Type  ErrorType
	Path  string
	Phase ExecutionPhase
	Cause error
	Limit int64 // size limit in bytes; meaningful only when Type is ErrorTypeSizeLimit
}

// newSizeLimitError builds the size-limit error. Its Cause is always
// ErrOutputSizeExceeded. It panics when limit is not positive.
func newSizeLimitError(path string, limit int64) *CaptureError
```

- **上限 0 は無制限として扱う（F-007）。** `Capture.WriteOutput` は `MaxSize` が 0 のとき、サイズを比べずに書き込む（AC-23、AC-24）。0 を無制限とする定義（`internal/common/output_size_limit_type.go:21-25`）と、無制限のとき上限 0 を渡す `NormalResourceManager`（`internal/runner/resource/normal_manager.go:244-249`）に合わせる。
- **`Limit` と構築関数を追加する。** `Capture.WriteOutput` のサイズ超過（`capture.go:43-48`）は `newSizeLimitError(c.OutputPath, c.MaxSize)` で作る。構築関数が `Type`・`Phase`（`PhaseExecution`）・`Cause`（`ErrOutputSizeExceeded`）を決めるので、サイズ超過の `Cause` が常にセンチネルであることを 1 箇所で保証する。`newSizeLimitError` は上限値が 0 以下なら panic する（AC-25）。本番でこの panic に届く入力は無い。
  - 上限 0 では、上の項目のとおり比較しないので呼ばれない。
  - 負の上限は、設定の読み込みで拒否される（§3.6）。
  - 本番で `Capture` を作るのは `DefaultOutputCaptureManager.PrepareOutput`（`internal/runner/base/output/manager.go:98-106`、`:122-130`）だけで、その `maxSize` は検証済みの設定から来る（`normal_manager.go:244-251`）。`Capture` はフィールドが公開された構造体なので、テストが負の `MaxSize` で作ることはできる。panic の影響は §4.5 を参照。
- **`Error()` は `Type` で分ける。** `ErrorTypeSizeLimit` のときだけ `output capture error during <phase>: output size limit exceeded for '<path>' (limit: <Limit> bytes)` とし、`Cause` の文言を付けない。それ以外の種類は現状の文言（`errors.go:83-96`）のままとする（AC-16、AC-18）。
  - `Cause` を付けない理由: サイズ超過の `Cause` はセンチネル `ErrOutputSizeExceeded` であり、`output size limit exceeded` という同じ事実を表す。`errors.Is` で判定するために持っているだけで、文言としての情報はない。
  - 分岐は宣言された `Type`（列挙型）で行い、`Cause` の文言を比べない（CLAUDE.md「Declare, don't infer」）。
  - 文言に `output size limit exceeded for '<path>'` を残すのは、変更前に stderr に出ていた `UserMessage` の文言（`errors.go:118`）と同じ部分文字列を保ち、その文言を検索している運用に配慮するためである（§4.3）。
- **`Cause` はそのまま持つ。** `errors.Is(err, output.ErrOutputSizeExceeded)` が成り立ち続ける（AC-17）。これを使うテストは `test/performance/output_capture_test.go:154` と `internal/runner/base/executor/executor_privilege_gap_integration_test.go:683` である。
- **`UserMessage`（`errors.go:115-130`）・`GetType`（`:104-106`）・`GetPath`（`:109-111`）を削除する**（AC-06、AC-18）。本番コードからの呼び出しは、`GetUserFriendlyMessage` 経由の `UserMessage` だけである。
- **フィールドは公開のままとする。** `CaptureError` はすべてのフィールドが公開された値型で、テストはリテラルで作っている（`errors_test.go`）。フィールドを非公開にすれば `Limit` の不変条件を型で守れるが、使われていない種類と段階を整理する [#1180](https://github.com/isseis/go-safe-cmd-runner/issues/1180) で形を変えるときに合わせて行う。本タスクでは本番の生成箇所を構築関数に寄せ、AC-25 は本番の経路について保証する。サイズ超過のテストの値は、リテラルではなく `newSizeLimitError` で作る。

`CaptureError` は本番では常にポインタで作られる（`capture.go:43`、`:64`）。原因への到達を確かめるテストは `errors.AsType[*output.CaptureError]` を使う。

使われていない `DefaultOutputCaptureManager.WriteOutput`（`manager.go:136-143`）にも「0 は無制限、超えたら失敗」の判定があり、別のセンチネル `ErrOutputSizeLimitExceeded` を返す。本番から呼ばれないので本タスクでは変えず、重複の整理は [#1181](https://github.com/isseis/go-safe-cmd-runner/issues/1181) で扱う。

### 3.6 `internal/runner/config`: 負の `output_size_limit` の拒否（変更）

```go
// ErrNegativeOutputSizeLimit indicates that an output_size_limit value is negative.
var ErrNegativeOutputSizeLimit error

// ValidateOutputSizeLimits validates that all output_size_limit values in the
// configuration are non-negative.
func ValidateOutputSizeLimits(cfg *runnertypes.ConfigSpec) error
```

- `ValidateTimeouts`（`internal/runner/config/validation.go:188-222`）と同じ形で、グローバル（`GlobalSpec.OutputSizeLimit`）・テンプレート（`CommandTemplate.OutputSizeLimit`）・コマンド（`CommandSpec.OutputSizeLimit`）の負の値をすべて集め、`ErrNegativeOutputSizeLimit` をラップした 1 つのエラーで返す。各項目は値と設定箇所（テンプレート名、group 名・コマンド名と添字）を含む（AC-27）。
- **取り込んだテンプレートを含めて検証する。** `ValidateTimeouts` は、主の設定ファイルだけを読む `loadConfigInternal` の中で呼ばれる（`internal/runner/config/loader.go:231-233`）。`includes` で取り込んだテンプレートのファイルは `ParseTemplateContent`（`internal/runner/config/template_loader.go:22-44`）で読まれた後、値の検証を受けないまま `mergeTemplates` で `cfg.CommandTemplates` に合流する（`loader.go:62-93`）。そのため `ValidateOutputSizeLimits` は、`ValidateTimeouts` の隣ではなく、`loadConfigWithIncludes` でテンプレートを合流した後の設定全体に対して呼ぶ。テンプレート名は合流時に重複が拒否されるので、設定箇所はテンプレート名で一意に示せる。
  - 取り込んだテンプレートの負の `timeout` も同じ理由で `ValidateTimeouts` を通らず、`createCommandContext` の panic（`internal/runner/group_executor.go:575-578`）に届く。これは本タスクの要件の対象外の既存の不具合なので、別 issue で扱う。
- 設定の読み込みは dry-run の分岐より前（`cmd/runner/main.go:350`）なので、dry-run でも拒否される。読み込みのエラーは、既存の設定の読み込みエラーと同じく実行前エラーとして報告され、どの group も実行されない。
- 同じ値を、検証を行わない `common.NewOutputSizeLimitFromPtr`（`internal/common/output_size_limit_type.go:33`）が上限に変換する点は変えない。検証済みの設定だけがこの変換に届くためである。

### 3.7 `internal/runner/base/executor`: 出力の保持の上限（変更）

- 出力ファイルがあるとき（`OutputWriter` が nil でないとき）、stdout と stderr の両方を、メモリ上では上限付き（先頭と末尾を保持し、省略した量を示す）で保持する（AC-28、AC-29）。上限には、出力ファイルが無いときの stderr の上限 `nilWriterStderrLimit`（32 KiB、`internal/runner/base/executor/executor.go:33-36`）と同じ値を使い、定数は 1 つにまとめて用途に合う名前にする。
- 出力ファイルが無いときは現状のまま（stdout は上限なし、stderr は 32 KiB）とする（要件の対象外）。
- 保持の仕組みは既存の `boundedBuffer`（`internal/runner/base/executor/output_pump.go:245-310`）をそのまま使う。先頭と末尾を保持し、間に `... omitting N bytes ...` を入れる。`newOutputPump`（`:49`）が stdout の上限も引数で受け取るようにし、`command_lifecycle.go:437-441` が出力ファイルの有無で両方の上限を決める。
- **stderr も上限付きにする理由。** 出力ファイルがあるとき、executor は stdout と stderr の両方を同じ `OutputWriter`（出力ファイル）へ書き（`output_pump.go:64-70`）、現状は stderr もメモリ上で上限なしに保持する（`command_lifecycle.go:437-441`）。stdout だけに上限を設けても、`output_size_limit = 0` のコマンドが stderr に大量に書けば、runner のメモリ使用量は出力の大きさに比例する。要件の Success Criteria（「runner のメモリ使用量が出力の大きさに比例しない」）を満たすには両方に上限が要る。出力の全体は出力ファイルにある。
- **出力ファイルへの書き込みは変えない。** 上限はメモリ上の保持だけにかかり、`OutputWriter` にはすべてのバイトを渡す（`executor.go:678-693`）。

保持した出力を使う箇所と、変わる点は次のとおりである。いずれも全体を必要としない。

| 使う箇所 | 変更前 | 変更後 |
|---|---|---|
| デバッグログ `Command execution result` の `stdout`（`internal/runner/group_executor.go:602-603`） | 切り詰めて記録 | 変わらない |
| デバッグログ `Command execution result` の `stderr`（`group_executor.go:605-606`） | 全体（最大 `output_size_limit`） | 先頭と末尾の 32 KiB ずつ |
| Slack の `command_group_summary` の出力（`internal/logging/slack_handler.go:21` の stdout 1000 文字、`:22` の stderr 500 文字で切り詰め） | 切り詰めて表示 | 変わらない（先頭側で切り詰められる） |
| `Command failed`・`Command failed with non-zero exit code` の構造化ログの `stderr`（`group_executor.go:639-643`、`:659-665`） | 全体（最大 `output_size_limit`） | 先頭と末尾の 32 KiB ずつ |
| executor の `Command execution failed` のログの `stderr`（`internal/runner/base/executor/command_lifecycle.go:789-793`） | 全体（最大 `output_size_limit`） | 先頭と末尾の 32 KiB ずつ |
| `run_as_user`/`run_as_group` 付きコマンドの失敗の監査ログの `stdout`・`stderr`（`internal/runner/base/audit/logger.go:120-121`）と、その Slack 通知（`user_group_command_failure`、`slack_handler.go:1033-1063`） | 監査ログは全体（最大 `output_size_limit`）、Slack は切り詰めて表示 | 監査ログは先頭と末尾の 32 KiB ずつ、Slack は変わらない |

### 3.8 コンポーネントの責務と変更ファイル

| ファイル | 変更 | 責務 |
|---|---|---|
| `internal/runner/group_errors.go` | 新規 | `GroupError`・`GroupErrors` と構築関数 |
| `internal/runner/runner.go` | 変更 | `executeGroups` が `*GroupErrors` を返す。中断の判定を実行全体の context の状態で行う |
| `internal/runner/test_helpers.go` | 変更 | `NewGroupErrorForTest`・`NewGroupErrorsForTest`（`test` ビルドタグ） |
| `cmd/runner/main.go` | 変更 | `executionErrorContext` を件数による判定にする |
| `internal/logging/execution_error.go` | 変更 | `UserFriendlyError`・`GetUserFriendlyMessage`・`formatCause` を削除 |
| `internal/logging/pre_execution_error.go` | 変更 | `Detail()`・`HandleExecutionError` が `Err.Error()` を使う |
| `internal/runner/base/output/errors.go` | 変更 | `Limit`・`newSizeLimitError` の追加、サイズ超過の `Error()`、`UserMessage`・`GetType`・`GetPath` の削除 |
| `internal/runner/base/output/capture.go` | 変更 | 上限 0 を無制限として扱い、サイズ超過を `newSizeLimitError` で作る |
| `internal/runner/config/errors.go` | 変更 | `ErrNegativeOutputSizeLimit` の追加 |
| `internal/runner/config/validation.go` | 変更 | `ValidateOutputSizeLimits` の追加 |
| `internal/runner/config/loader.go` | 変更 | テンプレートを合流した後に `ValidateOutputSizeLimits` を呼ぶ |
| `internal/runner/base/executor/output_pump.go` | 変更 | `newOutputPump` が stdout の上限も受け取る |
| `internal/runner/base/executor/command_lifecycle.go` | 変更 | 出力ファイルの有無で stdout・stderr の上限を決める |
| `internal/runner/base/executor/executor.go` | 変更 | 保持の上限の定数をまとめる |
| `docs/user/toml_config/04_global_level.ja.md` | 変更 | 「4.8 output_size_limit」に 0 が無制限であること・負の値は読み込みで拒否されること・メモリ上の保持の上限を追記（AC-26）。「4.1 timeout」の「動作の詳細」に、タイムアウト後も後続の group が実行されること・プロセスが残りうることを追記（AC-30） |
| `docs/user/toml_config/04_global_level.md` | 変更 | 上記を `/mktrans` で反映 |

変更する挙動を検証している既存のテスト（更新が必要）:

| テスト | 現在の検証内容 | 対応 |
|---|---|---|
| `internal/runner/runner_test.go:447-453` | 失敗 1 件が multi-error でないこと、`*CommandExecutionError` に届くこと | `*GroupErrors`（1 件）であることの検証に置き換える。`*CommandExecutionError` への到達は残す |
| `cmd/runner/main_test.go:745-790` `TestExecutionErrorContext` | `errors.Join` とラップしたエラーで外側の context を決めること | 入力を `*GroupErrors` に置き換え、§7.1 の行を加える |
| `internal/logging/pre_execution_error_test.go:43-48`、`:70-80` | `Detail()` が `UserMessage` を優先すること、`errors.Join` の子を分けること | `friendlyTestError`（`Error()` と `UserMessage()` が異なる型）は残し、期待値を `Error()` の文言に反転する |
| `internal/logging/pre_execution_error_test.go:609-640` `TestHandleExecutionError_CauseFormatting` | `HandleExecutionError` が `UserMessage` を使うこと | 同上。期待値を `Error()` の文言に反転する |
| `internal/runner/base/output/errors_test.go:74-86` | サイズ超過の旧文言 | 新しい文言と `Limit` に更新し、値は `newSizeLimitError` で作る |
| `internal/runner/runner_test.go:2831-2842` `TestRunner_CancellationSkipsStageNotification` | モックが `context.Canceled`・`DeadlineExceeded` を原因に持つ段階のエラーを返すと、実行前段の通知をしないこと | モックのエラーを返すときに実行全体の context を取り消すように変える。取り消さない場合は通知されることを別の行で検証する |
| `internal/runner/base/executor/output_pump_test.go:343-345` | 出力ファイルがあるとき stderr の上限が 0（上限なし）であること | 出力ファイルがあるときの stdout・stderr の上限の検証に更新する |
| `newOutputPump` を呼ぶテスト（`output_pump_test.go:139`、`:194`、`:220`、`:269`、`:287`、`:302`、`:355`、`executor_lifecycle_test.go:362`） | 現在の引数で出力ポンプを作ること | 引数の追加に合わせて更新する（コンパイラが検出する） |

`internal/runner/runner_test.go:660-735` の `TestRunner_CommandTimeoutBehavior` は `t.Skip` で常に飛ばされる（`:661`）ため、変更の検証には使わない。

`friendlyTestError` を残すのは、`UserMessage` を持つ型が無くなると「`Error()` をそのまま使う」ことを他の表示と区別できる入力が無くなり、テストが失敗しえなくなるためである（CLAUDE.md「Every test must be able to fail for its stated reason」）。テストを削除する場合は、AC-14 に従い `go tool cover -func` の結果を関数単位で比べる。

## 4. エラーハンドリング設計

### 4.1 エラー型

新しいエラー型は §3.1 の `GroupError`・`GroupErrors` の 2 つである。新しいセンチネルは `config.ErrNegativeOutputSizeLimit`（§3.6）の 1 つである。

### 4.2 文言の設計

| 状況 | `Details:` の文言 |
|---|---|
| 失敗 1 件（`*CommandExecutionError`、`CaptureError` を含まない） | `error running commands (group: g, command: c): failed to execute group g: command c in group g failed: ...`（変更前と同じ、AC-11） |
| 失敗 1 件（`*GroupStageError`、group レベル） | `error running commands (group: g): failed to execute group g: ...`（外側の context が新たに付く、AC-15） |
| 失敗 1 件（`*GroupStageError`、command レベル） | `error running commands (group: g, command: c): failed to execute group g: ...`（同上） |
| 失敗 2 件以上 | `error running commands: failed to execute group g1: ...` の後に、各 group の文言が `failed to execute group gN: ...` で続く（AC-01、AC-02、AC-05） |
| 出力サイズ超過を含む原因 | 末尾が `output capture error during execution phase: output size limit exceeded for '<path>' (limit: <N> bytes)`（AC-16） |
| 書き込み失敗（`ErrorTypeFileSystem`）を含む原因 | 末尾が `output capture error during execution phase: filesystem error for '<path>': <OS のエラー>`（変更前は `UserMessage` で OS のエラーが落ちていた、AC-03） |
| コマンドのタイムアウト | 失敗の 1 件として扱われる（上の行と同じ形）。1 件だけなら外側の context は変更前と同じ（AC-22） |
| 実行全体の中断 | 先頭の行が `context canceled`、続く行が中断時の `ExecuteGroup` のエラー（§3.2 の 2）。外側の context は §3.3 の 2 で決まる |
| 負の `output_size_limit` | 設定の読み込みエラー。`ErrNegativeOutputSizeLimit` の文言に、値と設定箇所が続く（AC-27） |

構造化ログの `error_message` は同じ文言である（AC-04）。ただし §5.2 の redaction を受ける。

### 4.3 変更前との互換

運用が挙動やエラー文言に依存している場合、影響を受けうる変更は次のとおりである。リポジトリ内にこれらの文言を検索するものは無い（`grep` で確認）。

| 変更 | 変更前 | 変更後 |
|---|---|---|
| `CaptureError` を含む失敗の `Details:` | `error running commands (group: g, command: c): output size limit exceeded for '<path>'` | 原因のチェーン全体（§4.2） |
| サイズ超過の `Error()`（`Command failed` などの slog の `error` 属性にも出る） | `... size limit exceeded for '<path>': output size limit exceeded` | `... output size limit exceeded for '<path>' (limit: <N> bytes)` |
| 失敗 1 件が `*GroupStageError` のときの `Details:` | `error running commands: ...` | `error running commands (group: g[, command: c]): ...` |
| コマンドのタイムアウト後 | 残りの group を実行せず、先の失敗を報告しない | 残りの group を実行し、すべての失敗を報告する |
| 1 回の実行にかかる時間 | タイムアウトが起きた時点で実行が終わる | タイムアウト後も残りの group を実行するので、最長で全コマンドの `timeout` の合計まで延びうる。cron・systemd でタイムアウトを実行時間の上限として使っている運用は、実行の重なりや `TimeoutStartSec` を見直す必要がありうる |
| 実行全体の中断時の報告 | 中断の検出の仕方によって `context canceled` の場合と、コマンドの失敗（終了コード 130 など）だけの場合がある | 常に `context canceled` を含む |
| group ファイル検証中の中断 | 検証の失敗を通知し、次の group の前の判定で `context canceled` だけを返す。最後の group だった場合は nil を返し、中断された実行が成功として報告されていた | 検証の失敗を通知し、`context canceled` と検証の失敗を合わせて返す。検証の失敗は通知と報告の両方に出る |
| `output_size_limit = 0` のコマンド | 出力を 1 バイトでも書くと、その時点でサイズ超過として失敗する（出力しないコマンドは成功する） | 出力の大きさによらず成功する |
| 負の `output_size_limit` を含む設定 | 読み込みは成功する。該当するコマンドは、出力を 1 バイトでも書くとその時点でサイズ超過として失敗する（出力しないコマンドは成功する） | 読み込みで拒否され、どの group も実行されない（dry-run を含む） |
| 出力ファイルを指定したコマンドのメモリ上の出力 | 全体を保持（最大 `output_size_limit`）。`Command failed` の `stderr` と監査ログに全体が出る | 先頭と末尾の 32 KiB ずつを保持（§3.7） |

`output size limit exceeded for '<path>'` という部分文字列は、変更前の stderr・`error_message` にも変更後にも現れる。

### 4.4 既知の制限

- **実行全体の中断では、集めた失敗が報告に出ない。** §3.2 のとおり、実行全体の context が取り消されると、`executeGroups` は集めた失敗を捨てて返す。これは要件の対象外（「実行全体の中断時に集めた失敗を報告すること」）である。
- **タイムアウトしたコマンドのプロセスが残りうる。** executor はタイムアウトのとき直接の子プロセスだけを kill し、プロセスグループへの kill は行わない（`internal/runner/base/executor` に `Setpgid`・`Kill(-pgid)` は無い）。子を kill・回収できなかった場合は `ErrKillAfterCancel`・`ErrChildNotReaped` がエラーに加わる（`command_lifecycle.go:741-742`、`:912`、`:948`）。いずれの場合も、残ったプロセスと後続の group が並行して動きうる。要件の決定事項「タイムアウト後に残るプロセスは受け入れる」により、これを受け入れ、利用者向け文書に記載する（AC-30）。
- **1 つの group の原因が複数行のとき、続きの行は `failed to execute group` で始まらない。** executor はコマンドのエラーに kill の失敗などを `errors.Join` で加えることがある（`command_lifecycle.go:788`）。そのとき `GroupError.Error()` は複数行になり、続きの行は `handleErrorCommon` によって他の group の行と同じ字下げで出る。帰属は、各 group の文言の先頭の行で判別する。続きの行を `GroupError.Error()` で字下げすると `Error()` の文言が変わり、AC-09 に反するので行わない。
- **出力ファイルを指定しないコマンドの stdout は、メモリ上で上限なく保持される。** 要件の対象外であり、現状のままとする。
- **dry-run の出力の分析は、上限を常に 0 と表示する。** `AnalyzeOutput` は `MaxSizeLimit` を設定しない（`internal/runner/base/output/manager.go:232` 以降）ため、dry-run の `max_size_limit` は常に 0 である（`internal/runner/resource/dryrun_manager.go:746`）。0 が無制限を意味すると利用者向け文書に明記した後は、dry-run が常に表示するこの 0 も無制限を意味すると読めてしまう。本タスクでは変えず、別 issue で扱う。

### 4.5 失敗時の扱い

- **`GroupError`・`GroupErrors` の構築関数の panic** は呼び出し側の誤りにだけ起きる。`executeGroups` は、`ExecuteGroup` が nil でないエラーを返したときだけ `newGroupError` を呼ぶ。group 名が空でないことは設定の読み込みで検証済みである（`internal/runner/config/validation.go:50-52`、`internal/runner/config/loader.go:236` から呼ばれる）。
- **`newSizeLimitError` の panic** も呼び出し側の誤りにだけ起きる（§3.5）。これは出力ポンプの読み取りの goroutine（`Capture.Write` → `WriteOutput`）で起きるため、発生すると回復できずプロセスが終わる。defer が走らないため、`RUN_SUMMARY` 行は出ず、出力の一時ファイルが残り、子プロセスが残りうる。本番の入力では届かないことを §3.5 と §3.6 で保証したうえで、この扱いを選ぶ。
- **中断の判定と競合。** コマンドが別の理由で失敗した直後に SIGINT・SIGTERM を受けた場合、`ExecuteGroup` から戻った時点で実行全体の context が取り消されていれば中断として扱う。利用者が中断を求めた以上、残りの group を実行しないのが正しい。戻った後に取り消された場合は、次の group の前の判定（`internal/runner/runner.go:413-417`）で中断する。

## 5. セキュリティ考慮事項

### 5.1 出力先

| 出力先 | 変更の影響 |
|---|---|
| stderr の `Details:` | `handleErrorCommon` が直接書く（`internal/logging/pre_execution_error.go:148-174`）。redaction は通らず、これは変更前と同じである。`CaptureError` を含む失敗では、変更前は `UserMessage` に隠れていた次の情報が新たに出る。書き込み失敗の OS のエラー、サイズ超過の上限値、原因のチェーン全体（`ErrKillAfterCancel`・`ErrChildNotReaped` など、別の uid でプロセスが残っている可能性を示すエラーを含む: `internal/runner/base/executor/command_lifecycle.go:788`）。最後のものは、利用者が気付くべき事象が隠れなくなるという改善である。同じエラーは変更前から `Command failed` の構造化ログ（`internal/runner/group_executor.go:639-643` の `error` 属性）に出ている |
| 構造化ログの `error_message` | `RedactingHandler` を通る点は変わらない（§5.2） |
| 構造化ログ・監査ログの `stdout`・`stderr` | 出力ファイルを指定したコマンドでは、先頭と末尾の 32 KiB ずつになる（§3.7）。省略は redaction（`SanitizeOutputForLogging`・`RedactText`）より前に起きるので、先頭と末尾の境目にかかった機密の値は途中で切れ、値の形の検出に一致しなくなることがある。また `key=` とその値の間に省略の印が入ると、値が key と結び付かなくなる。出力ファイルが無いときの stderr（32 KiB の上限）は現状でも同じ扱いであり、新しい種類の問題ではない。出力ファイルには省略の無い全体が書かれ、redaction の対象ではない点は変わらない |
| Slack | 実行エラーのレコードは `slack_notify=false` のまま（`internal/logging/pre_execution_error.go:270`）で、Slack へは送らない（AC-12）。実行前エラーの Slack 通知の本文（`Detail()`）は、§3.4 のとおり `CaptureError` による変化を受けない。group ファイル検証の失敗の通知は、実行全体の中断と重なっても行う（§3.2 の 1） |

### 5.2 redaction による帰属の喪失

`error_message` は自由文として値の redaction を受け、文言の一部が機密の形に一致すると、全体が `[REDACTED]` になりうる（`docs/dev/architecture_design/security-architecture.md:644`）。本タスクで、この影響を受ける範囲は広がる。

- `CaptureError` を含む失敗は、変更前は短い `UserMessage`（group 名・command 名を含まない）だった。変更後は原因のチェーン全体（group 名・command 名を含む）になる。group 名や command 名が機密の形に一致すると、変更前は読めた `error_message` が `[REDACTED]` になる。
- 複数 group の失敗では 1 つのレコードに全 group の文言が入るので、1 つの group の文言が一致すると全 group の帰属が構造化ログから読めなくなる。この点は変更前の `errors.Join` でも同じである。

stderr には redaction 前の文言が出るので、帰属は stderr で判別できる。redaction の規則は本タスクでは変えない。

### 5.3 脅威モデル

新しい入力経路・権限の変更・外部への送信は追加しない。変わるのは次の点である。

- **エラー文言の組み立てと、stderr・構造化ログに出る情報の範囲**（§5.1、§5.2）。
- **コマンドのタイムアウト後も後続の group が実行される（F-006）。** コマンドが 0 以外の終了コードで失敗した場合と比べると、次のとおりである。
  - 後続の group が先の group の結果に依存する設定では、先の group がタイムアウトしても後続の group が動く。0 以外の終了コードでも同じであり（`internal/runner/runner.go:449-450`）、この点は新しくない。
  - 0 以外の終了コードでは直接の子は回収済みだが、タイムアウトでは子や孫のプロセスが残りうる（§4.4）。残ったプロセス（`run_as_user` で別の uid のことがある）と後続の group が並行して動きうる。これは新しい種類の重なりであり、要件の決定事項により受け入れ、利用者向け文書に記載する（AC-30）。
  - なお、各 group の実行前の検証（ファイル検証・権限監査）は、タイムアウトの有無によらず group ごとに行われる。
- **`output_size_limit = 0` のコマンドは、出力ファイルに無制限に書く（F-007）。** 利用者が明示的に指定した値の定義どおりの挙動であり、出力先のディスクの消費は利用者の設定の責任範囲になる。runner のメモリ使用量は、§3.7 の上限により出力の大きさに比例しない。
- **負の `output_size_limit` は設定の読み込みで拒否される（§3.6）。**

脅威モデル図は N/A とする。

## 6. 処理フローの詳細

### 6.1 `executionErrorContext` の判定

矢印は判定の順序を表す。

```mermaid
flowchart TD
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    S(["err"]) --> Q1{"*runner.GroupErrors<br>を含むか"}
    Q1 -->|"はい"| Q2{"要素の件数"}
    Q2 -->|"1 件"| R1["その要素の<br>GroupName / CommandName"]
    Q2 -->|"2 件以上"| R2["空"]
    Q1 -->|"いいえ<br>（実行全体の中断）"| Q3{"*runner.CommandExecutionError<br>を含むか"}
    Q3 -->|"はい"| R3["その GroupName / CommandName"]
    Q3 -->|"いいえ"| R4["空"]

    class Q1,Q2,R1,R2 enhanced
    class Q3,R3,R4 process
```

Legend:

```mermaid
flowchart LR
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    L1["本タスクで追加する判定"]
    L2["既存の判定（実行全体の中断の経路のために残す）"]
    class L1 enhanced
    class L2 process
```

### 6.2 出力サイズ超過の原因の組み立て

矢印 A → B は「A が返したエラーを B が受け取り、必要ならラップして上へ返す」を表す。矢印のラベルは受け渡すエラーである。

```mermaid
flowchart TD
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    W["output.Capture.WriteOutput"] -->|"*CaptureError"| P["executor.outputPump"]
    P --> RE["executor.rankedError"]
    RE -->|"command execution failed: %w"| SC["runner.executeSingleCommand"]
    SC -->|"*CommandExecutionError"| EG["runner.executeGroups"]
    EG -->|"*GroupError を要素とする *GroupErrors"| MAIN["main.run"]

    class W,P,EG,MAIN enhanced
    class RE,SC process
```

出力ポンプは最初の書き込みエラーを保持し（`internal/runner/base/executor/output_pump.go:139-172`）、`rankedError` は書き込みエラーを最優先する（`command_lifecycle.go:886-895`）。`command execution failed: %w` のラップは `:795` による。出力ポンプは、メモリ上の保持の上限（§3.7）のために変更する。

Legend:

```mermaid
flowchart LR
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    L1["本タスクで変更する関数"]
    L2["変更しない関数"]
    class L1 enhanced
    class L2 process
```

### 6.3 `executeGroups` のエラー処理

`ExecuteGroup` がエラーを返したときの処理を示す。矢印は処理の順序を表す。

```mermaid
flowchart TD
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    E(["ExecuteGroup のエラー"]) --> Q0{"group ファイル検証の<br>失敗か（§3.2 の 1）"}
    Q0 -->|"はい"| N0["検証の失敗を通知する"]
    N0 --> Q1
    Q0 -->|"いいえ"| Q1{"実行全体の context は<br>取り消されているか"}
    Q1 -->|"はい"| R1["ctx.Err() と合わせて返す<br>（残りの group は実行しない）"]
    Q1 -->|"いいえ"| Q9{"検証の失敗として<br>通知済みか"}
    Q9 -->|"はい"| R9["集めずに次の group へ"]
    Q9 -->|"いいえ"| Q2{"*GroupStageError<br>（ファイル検証を除く）か"}
    Q2 -->|"はい"| R2["実行前段の失敗を通知し<br>GroupError として集める"]
    Q2 -->|"いいえ"| R4["GroupError として集める<br>（タイムアウトを含む）"]

    class Q0,N0,Q1,R1,Q9,R2,R4 enhanced
    class Q2,R9 process
```

Legend:

```mermaid
flowchart LR
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    L1["本タスクで変更する判定・処理（順序の変更を含む）"]
    L2["変更しない判定・処理"]
    class L1 enhanced
    class L2 process
```

## 7. テスト戦略

### 7.1 単体テスト

- **`GroupError`・`GroupErrors`（`internal/runner`）**
  - `Error()` が、同じ原因から `fmt.Errorf("failed to execute group %s: %w", ...)`・`errors.Join(...)` で作った値の `Error()` と一致すること（1 件・2 件、AC-09）。期待値をリテラルで書かず、変更前の組み立て方で作った値と比べる。
  - `errors.Is`・`errors.AsType[*CommandExecutionError]`・`errors.AsType[*output.CaptureError]` が 1 件・2 件の各原因に届くこと（AC-10）。
  - command 名が `*CommandExecutionError`・`*GroupStageError`（command レベル）から読まれ、どちらも無ければ空になること。
  - 構築関数が空の group 名・nil の原因・空の一覧・nil の要素で panic すること。`newGroupErrors` に渡したスライスや `Errors()` の戻り値を書き換えても、保持している一覧が変わらないこと。
- **`executeGroups`（`internal/runner`）**: `MockGroupExecutor` で次を確かめる。
  - 0 件・1 件・2 件の失敗。0 件で `nil`（`err == nil` が成り立つ）、1 件以上で `*GroupErrors` を返し、group 名が `GroupSpec.Name` であること（AC-07）。
  - 実行全体の context を取り消さずに、group-1 が `context.DeadlineExceeded` を含む `*CommandExecutionError` を返すと、group-2 が実行され、戻り値が group-1 の要素を含む `*GroupErrors` で、`errors.Is(err, context.DeadlineExceeded)` が成り立つこと（AC-19）。group-1 が 0 以外の終了コードの失敗、group-2 がタイムアウトのとき、両方が要素になること（AC-20 の前提）。
  - group-1 のモックが実行全体の context を取り消してから `context.Canceled` を含まないエラーを返すと、group-2 が実行されず、戻り値が `*GroupErrors` でなく、`errors.Is(err, context.Canceled)` が成り立ち、モックのエラーにも届くこと（AC-21。エラーの中身ではなく context の状態で判定していることを確かめる）。
  - group-1 のモックが実行全体の context を取り消してから `*verification.Error` を返すと、group ファイル検証の失敗の通知が記録されること（§3.2 の 1）。
- **`executionErrorContext`（`cmd/runner`）**: 次の行を持つ表にする（AC-05、AC-08、AC-11、AC-15、AC-22）。
  - `*GroupErrors` 1 件（`*CommandExecutionError`）
  - `*GroupErrors` 1 件（group レベルの `*GroupStageError`、command 名は空）
  - `*GroupErrors` 1 件（command レベルの `*GroupStageError`）
  - `*GroupErrors` 1 件（タイムアウトの `*CommandExecutionError`。変更前と同じ group 名・command 名になること）
  - `*GroupErrors` 2 件（空になること。§3.3 の判定順を逆にすると失敗することを確かめる）
  - 実行全体の中断（`errors.Join(context.Canceled, <*CommandExecutionError を含むエラー>)`）
  - どれでもないエラー
- **`CaptureError`・`Capture`（`internal/runner/base/output`）**
  - サイズ超過の文言が段階・パス・上限値を含み、`size limit exceeded` を 1 回だけ含むこと。`errors.Is(err, ErrOutputSizeExceeded)` が成り立つこと。他の種類の文言が変わらないこと（AC-16〜AC-18）。
  - `Capture.WriteOutput` のサイズ超過が `Limit` に `MaxSize` を設定し、`Cause` が `ErrOutputSizeExceeded` であること。
  - `MaxSize` が 0 の `Capture.WriteOutput` が、大きなデータでもエラーを返さないこと（AC-23）。
  - `newSizeLimitError` が 0 と負の上限値で panic すること（AC-25）。
- **`ValidateOutputSizeLimits`（`internal/runner/config`）**: グローバル・テンプレート・コマンドの負の値がそれぞれ `ErrNegativeOutputSizeLimit` で拒否され、エラーに値と設定箇所が含まれること。0 と正の値と未指定は受け入れること。`Loader.LoadConfig` が、主の設定ファイルの負の値と、`includes` で取り込んだテンプレートのファイルの負の値の両方を拒否すること（AC-27）。
- **出力ポンプ（`internal/runner/base/executor`）**: 出力ファイルがあるとき、上限を超える stdout・stderr を書くと、`OutputWriter` にはすべてのバイトが渡り、保持される出力は上限付きで先頭と末尾と省略の印を含むこと（AC-28、AC-29）。出力ファイルが無いときの上限は変わらないこと。
- **`logging`**
  - `HandleExecutionError` と `Detail()` が、`friendlyTestError` について `UserMessage()` ではなく `Error()` の文言を出すこと（AC-06）。
  - `HandleExecutionError` に `ErrorTypeFileSystem` の `*CaptureError`（`Cause` あり）を含む原因を渡すと、`Details:` に `Cause` の文言が出ること（AC-03）。

### 7.2 統合テスト

- `internal/runner` の既存の出力キャプチャのテスト（`output_capture_integration_test.go`、`runner_test.go` の `TestRunner_OutputCaptureErrorScenarios`）の形で、group-1 はコマンド失敗、group-2 は出力サイズ超過となる設定を実行する。得られたエラーを `HandleExecutionError` に渡し、stderr と `error_message` で AC-01、AC-02、AC-04、AC-05 を確かめる。group 名・command 名には redaction に一致しない名前を使い、`error_message` が `[REDACTED]` にならずに帰属を含むことも確かめる。
- 1 件の失敗（`*CommandExecutionError`）で、stderr と `error_message` が変更前と同じであること（AC-11）。
- **AC-20**: 実際のタイムアウトのエラーの形は、実際の executor でタイムアウトさせる既存のテスト `internal/runner/group_executor_timeout_test.go` の `TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded` と同じ仕組みで作る。これに、エラーが `*CommandExecutionError` と `context.DeadlineExceeded` の両方を含むことの確認を加える。そのうえで、group-1 は 0 以外の終了コード、group-2 はその形のタイムアウトのエラーとなる `executeGroups` の結果を `HandleExecutionError` に渡し、`Details:` に両方の group の行が出ることを確かめる。
- **AC-24、AC-28**: `output_size_limit = 0` と出力ファイルを指定したコマンドを実際に実行し、32 KiB を超える出力を書かせる。出力サイズ超過で失敗せずに完了し、出力ファイルに全出力が書かれ、結果の stdout（`ExecutionResult.Stdout`）が上限付きで省略の印を含むことを確かめる（既存の出力キャプチャの統合テストの形）。
- **AC-27（dry-run）**: 負の `output_size_limit` を含む設定で dry-run を行うと、設定の読み込みで拒否されることを確かめる。

### 7.3 既存挙動の維持

- 実行エラーのレコードの `slack_notify` が `false` のままであること。既存テスト（`internal/logging/pre_execution_error_test.go` の `slack_notify` の検証）を残す（AC-12）。
- 終了コードと `RUN_SUMMARY` 行は `HandleExecutionError` → `handleErrorCommon` の経路で決まり、本タスクはこの経路を変えない（`internal/logging/pre_execution_error.go:148-174`）。既存の `RUN_SUMMARY` のテストが引き続き通ることで確かめる（AC-12）。

### 7.4 静的な確認

`grep` で次を確かめる。

- `UserFriendlyError`・`GetUserFriendlyMessage`・`UserMessage`・`GetType`・`GetPath`・`formatCause` が本番コードに無いこと（AC-06、AC-18）。
- `Unwrap() []error` を型アサーションで判定する箇所が本番コードに無いこと（AC-08）。
- 本番コード全体で `errors.Is(..., context.Canceled)`・`errors.Is(..., context.DeadlineExceeded)` の使用箇所を列挙し、中断を決める分岐が無いこと（AC-21）。残る使用箇所は中断を決めないものに限られる。現状では、タイムアウトのセキュリティログ（`internal/runner/group_executor.go:629`）と Slack 送信の再試行（`internal/logging/slack_sender.go:530`）である。
- 利用者向け文書の記載（AC-26、AC-30）。

## 8. 実装の優先順位

各段階でビルドとテストが通る順にする（AC-13）。挙動の修正と文言の整理は別のコミットにする。

1. **`GroupErrors` の導入**: `group_errors.go`、`executeGroups`、`executionErrorContext`、テスト用の構築関数、関連テスト。この段階で、失敗 1 件の `*GroupStageError` に外側の context が付く（AC-15）。原因の文言（`UserMessage` の差し替えを含む）はまだ変わらない。
2. **`UserFriendlyError` の削除**: `logging` の 3 つの関数、`CaptureError.UserMessage`、関連テスト。ここで AC-01〜AC-04 が成り立つ。
3. **出力の保持の上限（AC-28、AC-29）**: 出力ポンプと `command_lifecycle.go`、関連テスト。5 より前に行い、上限 0 を無制限にした時点でメモリ使用量が出力に比例する状態を作らない。
4. **負の `output_size_limit` の拒否（AC-27）**: `ValidateOutputSizeLimits`、関連テスト。
5. **`output_size_limit = 0` の修正（AC-23、AC-24）**: `Capture.WriteOutput` の上限 0、関連テスト。3 の後に行う。
6. **`CaptureError` の整理（AC-16〜AC-18、AC-25）**: `Limit`・`newSizeLimitError` の追加、サイズ超過の文言、`GetType`・`GetPath` の削除、関連テスト。`newSizeLimitError` が 0 以下を拒否するので、4 と 5 の後に行う。
7. **タイムアウトと中断の扱いの変更（AC-19〜AC-22）**: `executeGroups` の処理の順序と中断の判定、関連テスト。1 の後であればよい。
8. **利用者向け文書（AC-26、AC-30）**: 日本語版を更新してコミットし、英語版を `/mktrans` で反映する。

2 と 3〜6 の順は入れ替えてよい。2 を 6 より先にすると、その間はサイズ超過の文言に同じ事実が 2 回出るが、情報は欠けない。

## 9. 将来の拡張性

- `GroupErrors` は失敗した group ごとに group 名・command 名を型で持つ。Slack で group ごとの失敗理由を知らせる改善（`01_requirements.md`「検討して採らなかった案」）を行う場合も、エラー文字列を解析せずにこの型から情報を得られる。
- 実行全体の中断で集めた失敗が捨てられる点（§4.4）を直すときも、捨てずに `GroupErrors` と中断のエラーを合わせて返す形で扱える。
- 出力の保持の上限は定数 1 つにまとめるので、出力ファイルを指定しないコマンドの stdout に上限を設ける改善（§4.4）でも同じ定数を使える。
- `CaptureError` の使われていない種類と段階の整理は [#1180](https://github.com/isseis/go-safe-cmd-runner/issues/1180)、サイズ超過のセンチネルと判定の重複は [#1181](https://github.com/isseis/go-safe-cmd-runner/issues/1181) で扱う。

## 10. 受け入れ基準との対応

| AC | 設計の該当箇所 | テスト |
|---|---|---|
| AC-01, AC-02 | §3.2、§3.4、§4.2 | §7.2 |
| AC-03 | §3.4、§4.2 | §7.1（`logging`） |
| AC-04 | §4.2、§5.2 | §7.2 |
| AC-05 | §3.3、§6.1 | §7.1、§7.2 |
| AC-06 | §3.4、§3.5 | §7.1（`logging`）、§7.4 |
| AC-07 | §3.1、§3.2 | §7.1（`executeGroups`） |
| AC-08 | §3.3、§6.1 | §7.1、§7.4 |
| AC-09, AC-10 | §3.1 | §7.1（`GroupError`・`GroupErrors`） |
| AC-11 | §3.3、§4.2 | §7.1、§7.2 |
| AC-12 | §5.1、§7.3 | §7.3 |
| AC-13 | §8 | 各コミットの `make test`・`make lint` |
| AC-14 | §3.8 | コミットメッセージ |
| AC-15 | §3.3、§4.2 | §7.1（`executionErrorContext`） |
| AC-16, AC-17, AC-18 | §3.5 | §7.1（`CaptureError`・`Capture`）、§7.4 |
| AC-19 | §3.2、§6.3 | §7.1（`executeGroups`） |
| AC-20 | §3.2、§6.3 | §7.1（`executeGroups`）、§7.2 |
| AC-21 | §3.2、§4.5、§6.3 | §7.1（`executeGroups`）、§7.4 |
| AC-22 | §3.3 | §7.1（`executionErrorContext`） |
| AC-23, AC-24 | §3.5 | §7.1（`CaptureError`・`Capture`）、§7.2 |
| AC-25 | §3.5、§4.5 | §7.1（`CaptureError`・`Capture`） |
| AC-26 | §3.8 | §7.4 |
| AC-27 | §3.6 | §7.1（`ValidateOutputSizeLimits`）、§7.2 |
| AC-28, AC-29 | §3.7 | §7.1（出力ポンプ） |
| AC-30 | §3.8、§4.4 | §7.4 |

## 付録 A. 他の設計文書との関係

> `docs/tasks/0176_group_pre_execution_failure_notification/02_architecture.md:412` は「`Detail()` は原因の連鎖に `UserFriendlyError` があるとき、その文言に置き換える」と、当時の挙動を説明している。本タスクで `UserFriendlyError` は無くなる。ただし、0176 の設計判断（実行前段の原因に `CaptureError` は現れないので、通知の文言に影響しない）は変わらない。完了したタスクの文書なので書き換えない。
