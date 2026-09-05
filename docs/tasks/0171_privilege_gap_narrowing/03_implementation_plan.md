# 実装計画書: 特権の隙をコマンド実行全体から fork/exec まで縮小する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-03 |
| Review date | 2026-09-03 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義書: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](02_architecture.md)（`approved`）
- Issue: [#1080](https://github.com/isseis/go-safe-cmd-runner/issues/1080)
- 先行タスク: [0170 実装計画書](../0170_excess_synchronization_removal/03_implementation_plan.md)

## 用語

本書は設計文書の用語表（[02_architecture.md](02_architecture.md) 冒頭）をそのまま用いる。
とくに次の語は本書で頻出する。

| 用語 | 意味 |
|---|---|
| 特権の隙 | プロセスの実効 UID が 0 に上がっている区間 |
| 起動区間／kill 区間／後始末区間 | 本タスクが開く3種類の特権の隙（設計文書 §5.2） |
| 出力中継 | 子プロセスの stdout／stderr のパイプを親側で読み `OutputWriter` へ流す部品（`outputPump`） |
| 実行 goroutine | `Execute` を呼んだ goroutine |
| fd-bound 実行／staging フォールバック | 検証済み記述子の束縛方式2種（設計文書の用語表と §3.1 の `execBinding`。staging の位置づけは §3.4） |

---

## 1. 実装の全体像

### 1.1 目的

`run_as_user`／`run_as_group` を伴う実行で、特権の隙をコマンドの実行時間全体から
`chown` と `fork`/`execve` へ縮める。あわせて、隙の中を走る非参加 goroutine のうち
コマンド実行の経路が起こすもの（`os/exec` の出力コピー goroutine 2本、
`exec.CommandContext` の watchdog goroutine 1本）を無くす。

設計の全体像・判断の根拠・図はすべて [02_architecture.md](02_architecture.md) にある。
本書はそれを実行可能な作業単位へ落とし、受け入れ基準との対応を追跡する。

### 1.2 実装方針

1. **設計文書 §8 の Phase 順を守る。** Phase 1（出力中継）と Phase 3（キャンセル・kill）は
   互いに独立だが、本書の分解では Phase 3 が Phase 2 の成果物（`preparedCommand`、
   `superviseCommand`）を前提とするため、実際の順序は 1 → 2 → 3 → 4 → 5 → 6 に固定する。
   Phase 4（隙の縮小）は Phase 1 と Phase 3 の両方が入った後に行う。
2. **Phase ごとに独立した PR へ分ける（Phase 3 と Phase 4 だけは2つに分ける）。**
   問題が出たときは PR 単位で revert する（設計文書 §8「戻し方」）。PR の区切りは §3.2 に示す。
   Phase 3 は、新しい隙を開かない 3-a〜3-c（PR-3）と、キャンセル・kill を実装する
   3-d・3-e（PR-4）に分ける。
   Phase 4 は、挙動を変える 4-a とその挙動テスト 4-b（PR-5）と、静的検査だけを足す
   4-c（PR-6）に分ける。隙の範囲を現状へ戻すときは PR-6 → PR-5 の順に revert する
   （PR-6 の静的検査は PR-5 が作った隙の形を前提にするため、PR-5 だけを戻すと赤くなる）。
   実行時に旧経路へ切り替える設定項目は追加しない。
3. **各 Phase の完了ゲートは `make fmt` → `make test` → `make lint` の全緑とする。**
   Phase 5 は加えて `go test -run '^$' -tags "test integration" ./internal/runner/base/executor/`
   を通し、`integration` タグ付きファイルが同じタグ組み合わせでコンパイルできることを確かめる。
4. **Go のコメント・識別子・文字列リテラルはすべて英語で書く。** 本書の説明文は日本語だが、
   実装へ書き写す文言は英語である。この規約は production コード・テストコードのほか、
   `Makefile` のコメントにも及ぶ（既存の `Makefile` のコメントはすべて英語である）。
5. **テストのパッケージ配置は import 制約から決める。** `executor/testutil`
   （`package executortestutil`）は `executor` パッケージを import しているため、
   `package executor` のテストファイルからは使えない（import cycle になる）。
   `MockOutputWriter`／`MockFileSystem`／`CreateRuntimeCommand` を要するテストは
   `package executor_test` に置き、非公開の型・関数を触るテストは `package executor` に置いて
   同パッケージ内の既存ヘルパー `createTestCommand`（`executor_logging_test.go`）を使う。

### 1.3 既存コード調査結果

#### 変更対象の現状

| 対象 | 現状 | 不足／変更点 |
|---|---|---|
| `internal/runner/base/executor/executor.go` `executeWithUserGroup`（144-237行） | `WithPrivileges` の `fn` の中で `executeCommandWithPath` を呼び、その内側で `execCmd.Run()`／`execCmd.Output()` が走る。昇格時間の計測は `WithPrivileges` 全体を囲む。エラー時は `LogUserGroupExecution` を呼ばずに早期 return する（218-221行） | `fn` の中身を `startPrepared` だけにする。計測を隙ごとに分ける。監査ログが成功時にしか出ないことは変えないので、監査ログを読む検証は正常終了する実行で組む |
| 同 `executeCommandWithPath`（275-388行） | 準備・実行・出力取り込み・結果組み立てを1関数で行う | 廃止し、`prepareCommand`／`startPrepared`／`superviseCommand` の3つへ分ける |
| 同 `prepareExecCommand`（420-463行） | 3経路（fd-bound／staging／resolved path）とも `exec.CommandContext` を使い、`(*exec.Cmd, func(), error)` を返す | `exec.Command` へ替え、`preparedCommand` を返す形へ分割。staging は起動フェーズへ移す |
| 同 `stageFromFD`（481-579行） | `os.MkdirTemp`／`os.Chown`／`os.Chmod`／`syscall.Dup`／`os.NewFile`／`os.OpenFile`／`io.Copy`／`os.RemoveAll`／`(*os.File).Stat`／`(*os.File).Close`／`syscall.Close`／`Logger.Warn` を呼ぶ。ほかに `fmt.Errorf`／`filepath.Base`／`filepath.Join`／`io.NewSectionReader` も呼ぶ。シグネチャは `(stagedPath string, cleanupFn func(), err error)` | 失敗時の後始末の中身は変えない。`Logger.Warn` を2箇所とも取り除き、`cleanupFn` を `func() error` へ変え、close 失敗は `preparedCommand.stagingWarn` で隙の外へ運ぶ（設計文書 §7.2）。「起動区間の内側で呼ばれる」ことを doc コメントへ書く。§7.2 の許可リストは副作用を持つ呼び出しの集合（`os.MkdirTemp` 等）と一致させ、副作用の無い呼び出し（`fmt.Errorf` 等）は静的検査の追跡対象から外す |
| 同 `outputWrapper`（643-676行） | `buffer bytes.Buffer`。構築は構造体リテラルで、production 2箇所（`executor.go:332`／`:333`）とテスト3箇所（`output_wrapper_test.go`） | `buffer` を `boundedBuffer` へ替え、構築を `newOutputWrapper(writer, stream, limit)` に統一する。doc コメントの「os/exec starts one copy goroutine per non-`*os.File` writer」を出力中継の記述へ置き換える |
| `test/security/output_security_test.go`（181行、252行） | コメントが `prepareExecCommand/stageFromFD` を名指ししている | `prepareExecCommand` が無くなるため、コメントを `prepareCommand/stageFromFD` へ直す |
| `internal/runner/base/runnertypes/config.go`（154-166行） | `Operation` は6値（`file_hash_calculation`／`command_execution`／`user_group_execution`／`file_access`／`file_validation`／`health_check`） | `OperationKillAfterCancel`／`OperationStagingCleanup` を追加 |
| `internal/runner/base/privilege/unix.go` `prepareExecution`（154-159行） | `switch` は `OperationUserGroupExecution`／`OperationFileValidation` だけを昇格が要る側へ振り分け、他は `ErrUnsupportedOperationType` で弾く | 上記2値を昇格が要る側へ追加する。追加しないと kill 区間・後始末区間が開けない |
| 同 `WithPrivileges` doc コメント（84-99行） | 「`os/exec` の copy goroutine が euid 0 で走る。未解決の設計課題」と書かれている（91-96行） | 実態に合わせて書き替える（AC-20） |
| `internal/runner/base/audit/logger.go` `PrivilegeMetrics`（50-53行） | `ElevationCount int` と `TotalDuration time.Duration` のみ。ログ属性は `elevation_count` と `total_privilege_duration_ms` | 隙ごとの内訳が採れない。operation 別の内訳を追加する（AC-05／AC-06 の検証に要る）。`TotalDuration` はミリ秒精度で出るため、起動区間（数十マイクロ秒）の比較にはマイクロ秒の属性が要る |
| `internal/runner/base/privilege/testutil/mocks.go` `MockPrivilegeManager`（24-48行） | 失敗の注入口は `ShouldFail bool` の1つだけで、すべての operation が同時に失敗する。再入ガードを持たない。隙の内側を観測する口が無い（`ExecFn` は `fn` を置き換えてしまう） | 3つを追加する（Phase 3）: 隙の内側で `fn` の前後2回呼ばれる `InWindow func(MockWindowPhase)`、operation 別の失敗注入 `FailFor map[runnertypes.Operation]error`、`UnixPrivilegeManager` と同じ再入ガード |
| `Makefile` `GOLINT`（24行）と `.pre-commit-config.yaml` の `golangci-lint` フック | どちらも `--build-tags test` のみ | `//go:build integration` のファイルは lint されない。既存の統合テストも同じ状態なので本タスクでは lint 対象を広げず、代わりにコンパイル検査を Phase 5 の完了ゲートで明示する |

#### 再利用する既存の資産

| 資産 | 場所 | 使い方 |
|---|---|---|
| `canRunPrivilegedIntegrationTest` とその表テスト | `executor/privileged_test_condition_test.go`（タグ無し、`package executor_test`） | 変更しない。同ファイルへ `canRunSetuidModelIntegrationTest` と、narrow interface `skipper` を取る薄いラッパー `requireSetuidModel` を追加し、同じ形の表テストを足す（設計文書 §7.3） |
| `numOpenFDs`／`openVerifiedPlan`／`TestExecute_FdBoundNoLeak`／`TestExecute_FdBoundStartFailureNoLeak` | `executor/executor_fdexec_test.go`（`package executor_test`） | `Start()` 失敗時の記述子リークは既存テストが覆っているので作り直さない。新規テストはキャンセル2経路だけを足し、既存テストと同じ作法（計測前のウォームアップ実行、ループ内での `plan.Close()`）を踏襲する |
| `createTestCommand` | `executor/executor_logging_test.go:19`（`//go:build test`、`package executor`） | `package executor` の新規テストから直接使う。run-as 指定と出力上限を与えられるよう可変長オプションを足して拡張する |
| `MockOutputWriter`／`MockFileSystem`／`CreateRuntimeCommand` | `executor/testutil/`（`package executortestutil`） | `package executor_test` のテストからのみ使う（§1.2.5） |
| `MockPrivilegeManager` | `internal/runner/base/privilege/testutil/mocks.go`（`//go:build test`、`package privilegetestutil`） | 上表のとおり3つの注入口を足して再利用する。新しいモックは作らない |
| `streamRecorder` | `executor/output_wrapper_test.go`（`package executor`、`//go:build test`） | 出力中継のテストからも使う（同一パッケージなので追加のヘルパーは不要） |
| `LogRecorder`／`NewRecordingLogger`／`RequireRecord`／`AssertHasAttrs` | `internal/testutil/handlers.go` | 監査ログとセキュリティログを読むのに使う。属性は `map[string]any` に格納されるため、同名キーの重複は保持されない。よって隙ごとの時間は operation 名を含む一意なキーで出す |
| `audit.NewAuditLoggerWithCustom` | `internal/runner/base/audit/test_helpers.go:13`（`//go:build test`） | 監査ログを `LogRecorder` へ向けるのに使う（`audit.NewAuditLogger` は `slog.Default()` を構築時に掴むため差し替えられない） |
| `WithFdExecDisabled`／`WithExitFunc`／`WithIdentityChecker` | `executor/test_helpers.go`（`//go:build test`） | `WithKillGraceDelay` を同じ形で足す（Phase 3） |
| `elfanalyzer-integration-test`／`libccache-integration-test` | `Makefile`（550-558行） | 新しい統合テストターゲットの雛形にする |

#### 外部前提の確認（典拠つき）

| 前提 | 典拠 | 確認結果 |
|---|---|---|
| `os/exec` は `Cmd.Stdout`／`Cmd.Stderr` が `*os.File` のとき出力コピー goroutine を起こさない | `$(go env GOROOT)/src/os/exec/exec.go` `writerDescriptor` 580-605行 | `*os.File` のときは即 return し、`c.goroutine` に何も足さない。パイプを作る経路（594-605行）でのみ goroutine が積まれる |
| `*os.File` を渡した場合、その記述子は `os/exec` が閉じない | 同 590-592行（`childIOFiles` に追加せず返す） | 親側で閉じる責任が残る。設計文書 §3.2 要点3 のとおり `releaseChildEnds` が要る |
| `Cmd.Output()` の標準エラー出力の上限は前後 32 KiB | 同 1023行 `c.Stderr = &prefixSuffixSaver{N: 32 << 10}`、省略表示は 1186行 `"\n... omitting "` | `boundedBuffer` の既定 `stderrLimit` を `32 << 10` とし、省略表示の文言を合わせる |
| `os.Pipe` の両端は `O_CLOEXEC` である | Go 標準ライブラリの `os.Pipe`（`pipe2(O_CLOEXEC)`） | 設計文書 §3.2 要点4 の不変条件は `os.Pipe` を使う限り自動的に満たされる |
| `.pre-commit-config.yaml` の `go-test` フックは `make` を経由しない | `.pre-commit-config.yaml` の `entry: go test -tags test -v ./...` | `Makefile` の `test` ターゲットへ依存を足しても pre-commit では走らない。フックを別に足す必要がある |
| `repo: local` の pre-commit フックは `id`／`name`／`entry`／`language` が必須 | `.pre-commit-config.yaml` の既存5フックはすべて `name` を持つ | 新フックにも `name` を書く。欠くと設定検証に失敗し、リポジトリ全体のフックが動かなくなる |
| `Makefile` の `ENVSET` は `env -i` で環境を空にする | `Makefile` 83-95行 | `TEST_RUNAS_TARGET_USER` は新ターゲットの中で明示的に転送しなければテストへ届かない |
| `integration` タグ付きテストは `test` タグも要る | `executor/executor_usergroup_integration_test.go` 冒頭コメント（`executor/testutil` が `//go:build test \|\| performance` を持つため） | 実行は `-tags "test integration"` で行う |
| `/tmp` は `nosuid` でマウントされていることがある | systemd の既定 `tmp.mount` は `mode=1777,strictatime,nosuid,nodev` | setuid テストバイナリを `/tmp` に置くと setuid ビットが無視され、全テストが理由も分からずサイレントにスキップする。§4.3 の手順でマウントオプションを確かめる |

#### 挙動への波及と、既存テストへの影響

| 箇所 | 波及 | 対応 |
|---|---|---|
| `executor_test.go::TestExecute_ContextCancellation` | キャンセル済み context での起動は、現在 `exec.CommandContext` が `Start` で `ctx.Err()` を返し、`executeCommandWithPath` は `result` を非 `nil` のまま返す。テストは `assert.NotNil(t, result)` を主張している。新設計では準備フェーズで `ctx.Err()` を検査して早期に戻るため、素直に書くと `result` が `nil` になる | `superviseCommand` を通らない早期リターンでも `&Result{ExitCode: ExitCodeUnknown}` を返し、既存テストを変更せずに緑を保つ。この不変条件を Phase 3 の作業項目に明記する。同じ主張の新規テストは作らない |
| `executor_privilege_check_test.go::TestPrepareExecCommand_CredentialWiring` | `prepareExecCommand` が無くなる | `prepareCommand` と `applyCredential` の組で同じ配線を主張する形へ書き替える |
| `output_wrapper_test.go` の3箇所の構造体リテラル | 構築関数が `newOutputWrapper` に変わる | リテラルを `newOutputWrapper(recorder, StdoutStream, 0)` などへ書き替える。主張の内容は変えない |
| `stagefromfd_test.go::TestStageFromFD_*` | `stageFromFD` のシグネチャが `(stagedPath, cleanupFn func() error, warn error, err error)` へ変わる（複製元記述子の close 失敗を、隙の外へ運ぶために戻り値で返すため）。失敗時の後始末の中身は変わらない | 呼び出し2箇所を新シグネチャへ合わせる（戻り値を1つ読み飛ばす）。ディレクトリを残さないという既存の主張は変えない |
| `Start()` 失敗パスの返るエラー | 旧コードは複製検証記述子／`os.DevNull`／staging ディレクトリの close 失敗を `Logger.Warn` するだけでエラーに含めなかった。新コードは `pc.release()` がそれらを `errors.Join` で返るエラーへ入れる（設計文書 §3.1 の骨格の形） | 二重失敗（`Start()` 失敗と close 失敗の同時）のとき、返るエラーに以前より1つ多い join された cause が含まれうる（新しい失敗は起きず、元の cause は `errors.Is` でたどれる）。§3.1 骨格の帰結であり、§5.6 の1点には含めない |
| 準備フェーズの失敗時の戻り値 | 旧コードは失敗時に close 失敗をその場で `Logger.Warn` していた。新コードでは `release()` が隙の中で走りうるためログできず、失敗を `preparedCommand` へ記録して隙の外で出す | `prepareCommand` は失敗時も非 `nil` の `preparedCommand` をエラーと共に返し、呼び出し元が `logDeferredWarnings` へ渡す。返る `pc` は使用済みで、起動してはならない旨を doc コメントに明記する |
| `test/security/output_security_test.go:181,252` | コメントが `prepareExecCommand` を名指ししている | Phase 2 でコメントを直す |
| `internal/testutil/synccensus/census_guard_test.go` | `capture.go` の `mutex`、`log_line_tracker.go` の `lineCounter`、`error_collector.go` の `mu` の理由文字列が実態と合わなくなる | 3行の `reason` を出力中継の読み取り goroutine を指す文言へ書き替える。出力中継は同期プリミティブを宣言しない（設計文書 §3.2、チャネルで join する）ので行の追加は起きない |
| 0170 実装計画書の検証コマンド3行と完了条件2箇所 | Phase 6 の doc コメント更新で、`unix.go` の `This is an unresolved design issue`（0170 AC-11、`:1471`）、`capture.go` の2つのリテラル（0170 AC-14、`:1475` と本文 `:278`）、`log_line_tracker.go`／`error_collector.go` の `output copy goroutine`（0170 AC-15、`:1478` と本文 `:297`）がいずれも消える | 5箇所すべてに、本タスク（0171）で文言が置き換わった旨の注記と置換後の検証コマンドを併記する（Phase 6）。0170 の他の行は触らない |
| `internal/filevalidator` の `WithPrivileges` 利用（`OperationFileValidation`） | 変更しない（要件定義書のスコープ外） | 対応不要 |
| `test-ci`／`test-ci-cgo1` への新ターゲット追加 | 既存の `executor_usergroup_integration_test.go` も CI でコンパイル・実行されるようになる（CI は非特権なのでスキップする） | 想定どおり。CI で新たにスキップログが出ることを Phase 5 の完了確認で見込んでおく |

---

## 2. 実装ステップ

### Phase 1: 出力中継と `boundedBuffer`（F-001 の土台）

**変更するファイル**: `internal/runner/base/executor/output_pump.go`（新規）、
`internal/runner/base/executor/output_pump_test.go`（新規）、
`internal/runner/base/executor/executor.go`、
`internal/runner/base/executor/output_wrapper_test.go`、
`internal/runner/base/executor/executor_test.go`

- [x] `output_pump.go` に `boundedBuffer` を実装する。`newBoundedBuffer(limit int) *boundedBuffer`、
      `Write(p []byte) (int, error)`（常に `len(p), nil` を返す）、`Bytes() []byte`。
      `limit == 0` は上限なしで `bytes.Buffer` と同じ振る舞いにする。
      省略表示は `os/exec` の `prefixSuffixSaver` と同じく `"\n... omitting %d bytes ...\n"` とする。
- [x] `outputWrapper` の `buffer` の型を `bytes.Buffer` から `boundedBuffer` へ替え、
      構築関数 `newOutputWrapper(writer OutputWriter, stream OutputStream, limit int) *outputWrapper`
      を追加する。`Write`／`GetBuffer`／`GetWriteError` のシグネチャと挙動は変えない。
- [x] `outputWrapper` の doc コメントを書き替える。現在の
      「Each wrapper is written by exactly one goroutine: os/exec starts one copy
      goroutine per non-\*os.File writer, ...」以下の、`os/exec` の copy goroutine と
      `Cmd.WaitDelay` に依拠した記述を、「出力中継の読み取り goroutine が1本ずつ書き、
      `buffer`／`writeErr` はその goroutine の `done` チャネルが値を返した後にだけ読む」
      という不変条件の記述へ置き換える。
- [x] `output_wrapper_test.go` の3箇所の `&outputWrapper{writer: …, stream: …}` を
      `newOutputWrapper(…, …, 0)` へ置き換える（主張は変えない）。
- [x] `output_pump.go` に、テストから差し替えられるパイプ生成の口
      `var pipeFn = os.Pipe` を置く（`newOutputPump` はこれを呼ぶ）。
      パイプ生成失敗の経路をテストから通すために要る。
- [x] `output_pump.go` に `pumpStream` と `outputPump` を実装する。API は設計文書 §3.2 のとおり:
      `newOutputPump(writer OutputWriter, stderrLimit int) (*outputPump, error)`、
      `childFiles()`、`releaseChildEnds() error`（冪等）、`start()`、
      `wait(deadline time.Duration) (stdout, stderr []byte, writeErr error, timedOut bool)`、
      `release() error`。
- [x] `wait` の `deadline` の規約を決めて doc コメントへ書く: **`deadline == 0` は上限なし**
      （kill を経ない通常の経路で使う）。非 0 は kill 後の回収に使う上限である。
      `boundedBuffer` の `limit == 0` と同じ「0 = 制限なし」の規約に揃える。
- [x] 読み取り goroutine が、`io.Copy` の終了時（エラー終了を含む）に自分の読み取り側を閉じ、
      結果を `done` チャネル（バッファ1）へ送るようにする。`WaitGroup` は使わない
      （同期プリミティブを宣言しないため。設計文書 §3.2）。
- [x] `wait` の deadline 経路で、`done` が値を返していない側のバッファと `writeErr` を読まず
      `nil` を返し、`timedOut` を真にする（設計文書 §3.2 要点7）。
- [x] `output_pump.go` にエラー変数 `ErrOutputPipe` を設計文書 §4.1 の文言で定義する。
      設計文書 §4.1 はエラー変数をまとめて挙げているが、この1つだけは `output_pump.go` に置く。
      Phase 1 の時点では `command_lifecycle.go` がまだ無く、この PR 単独で
      グリーンゲートを通す必要があるためである（残りは Phase 2・Phase 3 で定義する）。
- [x] `newOutputPump` のパイプ生成に失敗したときは `ErrOutputPipe` を返し、
      それまでに作った記述子を解放する。
- [x] `executeCommandWithPath` の出力取り込みを出力中継に置き換える。
      `outputWriter != nil` の経路も `nil` の経路も同じ出力中継を通し、
      `stderrLimit` は前者で 0、後者で `32 << 10` とする。
      `execCmd.Stdout`／`Stderr` には `childFiles()` が返す `*os.File` を渡す。
      `outputWriter == nil` の経路では、`Result.Stderr` へ標準エラー出力を載せるのは
      異常終了時だけという `Cmd.Output()` の性質を維持する（設計文書 §4.3）。
- [x] 書き込み側の解放（`releaseChildEnds`）を `Start()` の直後に置き、
      `Start()` の成否によらず必ず通る位置にする。
- [x] `executor.go` から `execCmd.Run()`／`execCmd.Output()` の呼び出しを削除する。
- [x] 出力中継の `wait` が返す `writeErr` を `Execute` の戻り値のエラーへ載せる。
      Phase 1 では「書き込みエラーが出たらそれを返す」ところまでとし、
      `*exec.ExitError` との優先順位と `errors.Join` による併記は Phase 3-d で確定させる。
      この配線が無いと、下の `TestExecute_OutputLimitAbortsRunningChild` は
      子が `SIGPIPE` で死んだ `*exec.ExitError` を受け取って落ちる。

**テスト**（`output_pump_test.go`、`//go:build test`、`package executor`。
このファイルでは記述子の実数を数えない。`numOpenFDs` は `package executor_test` にあり、
`package executor` からは参照できないため）

- [x] `TestBoundedBuffer_UnlimitedBehavesLikeBytesBuffer`: `limit == 0` で入力がそのまま返る。
- [x] `TestBoundedBuffer_KeepsPrefixAndSuffix`: `limit` を超えた入力で、先頭 `limit` バイトと
      末尾 `limit` バイトが残り、中間が省略表示に替わる。境界値として
      「ちょうど `limit`」「`limit`+1」「`2*limit`」「`2*limit`+1」を含める。
- [x] `TestBoundedBuffer_WriteNeverFails`: 上限超過後の `Write` も `(len(p), nil)` を返す。
- [x] `TestOutputPump_SeparatesStreams`: stdout と stderr が別々に `OutputWriter` へ渡り、
      `streamRecorder` の記録で取り違えが起きない。
- [x] `TestOutputPump_WriteErrorPrefersStdout`: 両ストリームで書き込みエラーが起きたとき、
      `wait` が返す `writeErr` は stdout 側である。
- [x] `TestOutputPump_PipeCreationFailureReleasesDescriptors`: `pipeFn` を、2回目の呼び出しで
      失敗する関数へ差し替え（`t.Cleanup` で元へ戻す）、`newOutputPump` が `ErrOutputPipe` を返し、
      1回目に作ったパイプが解放されることを確かめる。
- [x] `TestOutputPump_ReleaseIsIdempotent`: `start()` へ到達しなかった経路で `release()` と
      `releaseChildEnds()` を二重に呼んでもエラーにならない。
- [x] `TestOutputPump_WaitDeadlineDoesNotReadUnfinishedStream`: 読み取りが終わらない側について
      `wait` が `nil` と `timedOut == true` を返す。`-race` 付きで走らせて、
      終わっていない側のバッファを読んでいないことを検出できる形にする。

**テスト**（`executor_test.go`、`package executor_test`。`Execute` から見た互換性と打ち切り挙動）

- [x] `TestExecute_NilOutputWriter_StderrPrefixSuffixBound`: `outputWriter` が `nil` の経路で、
      64 KiB を超える標準エラー出力を出して**異常終了**するコマンドについて、
      `Result.Stderr` に先頭 32 KiB と末尾 32 KiB が残り、中間が
      `\n... omitting N bytes ...\n` に置き換わる（`Cmd.Output()` と同じ規則。設計文書 §4.3）。
- [x] `TestExecute_NilOutputWriter_LargeStderrStillSucceeds`: 同じ経路で、
      64 KiB を超える標準エラー出力を出してから**正常終了**するコマンドが成功したままであり、
      `Result.Stderr` が空である（`Cmd.Output()` は正常終了時に標準エラー出力を載せない）。
- [x] `TestExecute_OutputLimitAbortsRunningChild`: 一定バイト数を超えたら固定のエラーを返す
      `OutputWriter` のスタブ（`executor_test.go` にローカルに置く。`output.Capture` は使わない）を
      渡し、10 秒間出力を出し続けるコマンドが 2 秒以内に打ち切られ、返るエラーから
      そのスタブのエラーを `errors.Is` でたどれることを確かめる。
      **特権を要さないので `make test` で常に走る**（AC-13 の常時実行される証拠。
      run-as を伴う版は Phase 5 の統合テストが担う）。

**完了の目安**: `make fmt` → `make test` → `make lint` が緑。既存の
`executor_test.go`／`executor_fdexec_test.go`／`output_wrapper_test.go` が変更なし（または
上記の構築関数置き換えのみ）で通る。

### PR-1 作成ポイント: output pump and bounded buffer

**対象ステップ**: 1

**推奨タイトル**: `feat(0171): add output pump and bounded buffer for child stdout/stderr`

**レビュー観点**: パイプ記述子の生成から解放までの対称性（`release`／`releaseChildEnds` の冪等性） / `wait` の `deadline == 0` 規約と、未完了ストリームのバッファを読まないこと / `boundedBuffer` の省略表示が `Cmd.Output()` と一致すること / `outputWriter` が `nil` の経路で標準エラー出力の載せ方が変わらないこと / `writeErr` が `Execute` の戻り値へ届くこと（優先順位の確定は 3-d に譲る）

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ 1 は自前のパイプと読み取り goroutine の生存管理という並行処理を導入する隔離された高リスク step（deadline 経路の競合と記述子リーク）であるため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] `make deadcode` が新たな未使用シンボルを報告しない（後続 PR が使うまで未使用のままの記号があるため）
- [x] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: 準備・起動・監督の3フェーズへの分解

`WithPrivileges` で囲む範囲はまだ変えない。外から見える挙動を変えずに構造だけを組み替える。

**変更するファイル**: `internal/runner/base/executor/command_lifecycle.go`（新規）、
`internal/runner/base/executor/executor.go`、
`internal/runner/base/executor/executor_privilege_check_test.go`、
`internal/runner/base/executor/executor_logging_test.go`、
`internal/runner/base/executor/executor_lifecycle_test.go`（新規）、
`internal/runner/base/executor/stagefromfd_test.go`、
`test/security/output_security_test.go`

- [x] `command_lifecycle.go` に `execBinding`（`bindingUnset`／`bindingVerifiedFD`／
      `bindingStagedCopy`／`bindingResolvedPath`）と `killStrategy`（`killUnset`／
      `killDirect`／`killReelevated`）を定義する。零値は宣言し忘れを表し、
      `switch` の `default` は失敗側へ倒す。
- [x] `preparedCommand` を設計文書 §3.1 のフィールド構成で定義する。`stagingRequest` は
      staged path の確定を起動フェーズへ移す Phase 3-d で定義する（この Phase では
      staging は準備フェーズに残るので、使い手がいない）。
- [x] `command_lifecycle.go` に `commandOutcome`（設計文書 §3.3）と、エラー変数
      `ErrExecBindingUnset` を設計文書 §4.1 の文言で定義する。どちらも本 Phase の
      `prepareCommand`（`binding` の `switch` の `default`）と `superviseCommand`
      （待機結果の受け渡し）が使うため、ここで入れる。
- [x] `preparedCommand.release() error` を実装し、準備フェーズが確保した記述子
      （出力中継、複製した検証済み記述子、`os.DevNull`）と staged copy をすべて解放する。
- [x] `prepareCommand` を実装する。`prepareExecCommand` の3経路の選択、`applyCredential`、
      `os.DevNull` の open、作業ディレクトリと環境変数の設定、出力中継の生成、
      `binding`／`kill` の宣言をここへ集める。**この Phase では `exec.CommandContext` を
      使い続ける**。`bindingStagedCopy` の経路も現行どおり `prepareCommand` の中で
      `stageFromFD` を呼び、確定した staged path に対して `exec.CommandContext` を呼ぶ。
      `exec.Command` への切り替えは Phase 3-d で、キャンセル経路の自前実装と同時に行う。
      **この2つを別々の PR に割ると、`exec.CommandContext` が担っていたタイムアウト・
      キャンセル時の kill を誰も行わない状態が PR-2 から PR-4 まで続く。** その間、
      `executor_test.go` の既存2件（`sleep 2` を 100ms のタイムアウトで殺す表の行と
      `TestExecute_ContextCancellation`）が落ち、PR-2 は単独でグリーンゲートを通らない。
      本 Phase の「挙動不変」はこの一点で決まる。
- [x] `startPrepared(pc *preparedCommand) (started bool, err error)` を実装する。
      この Phase の中身は `execCmd.Start()` の呼び出しだけである（staging は
      `prepareCommand` に残る）。`Start()` に成功したら `started` を真にする。
      `stageFromFD` の呼び出しをここへ移すのは Phase 3-d で、`exec.Command` への
      切り替えと同じ PR で行う。
- [x] `superviseCommand(ctx, pc, startupErr) (*Result, error)` を実装する。この Phase では
      まだキャンセル経路を持たず、待機 goroutine の起動・出力中継の `wait`・`Result` の
      組み立てまでを行う。
- [x] `executeCommandWithPath` を削除し、`executeWithUserGroup`／`executeNormal` を
      設計文書 §3.1 の骨格（`started` の判定を含む3分岐）へ組み替える。
      この Phase では `WithPrivileges` が `prepareCommand`＋`startPrepared`＋
      `superviseCommand` の全体を包んだままにする。
- [x] `prepareExecCommand` を削除する。
- [x] `test/security/output_security_test.go:181` と `:252` のコメント中の
      `prepareExecCommand/stageFromFD` を `prepareCommand/stageFromFD` へ直す。
- [x] `stageFromFD` から `Logger.Warn` を2箇所とも取り除く（設計文書 §7.2）。隙の中では
      ログを出さない。記録したい内容は値として隙の外へ運ぶ。
      - `executor.go:488`（staging ディレクトリの削除失敗）: 後始末の関数の型を
        `cleanupFn func()` から `func() error` へ変え、`os.RemoveAll` のエラーを返す。
        この関数を呼ぶのは呼び出し元なので、記録も呼び出し元が行える。
        **あわせて、この1件だけは stderr へも書く**（設計文書 §7.2）。書式は
        `os.Stderr.WriteString(fmt.Sprintf("WARNING: failed to remove staging directory %s: %v\n", dir, rmErr))`。
        隙を閉じる復帰処理が失敗して `emergencyShutdown` が `os.Exit` する経路では、
        戻り値を記録する行に到達しないためである。隙の中から呼ばれるときも外から呼ばれるときも
        区別せずに書く（区別すると後始末の関数が自分のいる隙を引数で受け取ることになる。
        削除の失敗は稀なので、まれに1行重複するほうが安い）。
        doc コメントへ「この書き込みは redaction ハンドラを通らないので、
        秘匿情報を含まないと分かっている値だけを載せる」ことを英語で書く。
      - `executor.go:531`（staging 元の複製記述子の close 失敗）: `stageFromFD` も
        `startPrepared` も隙の内側にいるため戻り値では外へ出せない。
        `preparedCommand` へ `stagingWarn error` フィールドを足してそこへ載せる。
- [x] `preparedCommand` に `stagingWarn error` を足し、その doc コメントへ
      「staging は成功しているのでエラーとしては返さない。隙の中でログを出さないために
      値として運ぶだけである」ことを英語で書く。
- [x] `stagingWarn` と `cleanupFn()` の戻り値を記録する位置は、この Phase では
      **`WithPrivileges` から戻った後**（`executeWithUserGroup`／`executeNormal` の中）に置く。
      この Phase の `WithPrivileges` は準備から監督までを包むので、隙の外で記録できる
      最初の地点がここである。**`startPrepared` の呼び出し元は隙の内側なので、そこへ
      `Logger` の呼び出しを置いてはならない**（設計文書 §7.2。slog ハンドラはファイルを
      開くことが許されており、隙の中で呼べばその `open` が実効 UID 0 で行われる）。
      Phase 4-a では隙が `startPrepared` だけへ縮むので、記録は「隙の直後」という位置づけの
      まま、より早い地点へ移る。
- [x] `stagefromfd_test.go` の `stageFromFD` 呼び出し2箇所（`:66`、`:91`）を、
      `cleanupFn func() error` に合わせて受け方だけ直す。ディレクトリを残さないという
      既存の主張は変えない。
- [x] `stageFromFD` の doc コメントに、この関数が特権の隙の内側で呼ばれることと、
      その結果として staged copy が root 所有になること、および隙の中でログを出さないため
      警告を戻り値で返すことを書き足す（設計文書 §3.4 差分1、§7.2）。
      「起動区間の内側」と限定できるのは呼び出しが `startPrepared` へ移る Phase 3-d 以降なので、
      その1語はそこで足す。
- [x] `executor_logging_test.go` の `createTestCommand` へ可変長オプション引数を足し、
      出力サイズ上限を指定できるようにする（同パッケージの新規テストが使う）。既存の
      2引数呼び出しは変えずに済む形にする。**当初案にあった run-as ユーザー／グループの
      オプション（`withRunAs`）は実装後のレビューで指摘され削除した**: `prepareCommand`
      は run-as の資格情報を `cred` 引数で直接受け取り、`cmd.RunAsUser()`／`RunAsGroup()`
      を読まないため、`TestPrepareCommand_CredentialWiring` に `withRunAs` を足しても
      `prepareCommand` の実際の依存関係を検証したことにならない（cred は別途手で組み立てて
      渡している）。run-as 経由の資格情報解決を検証する必要が生じたら、そのときのテストが
      要る形でオプションを足し直す。
- [x] `TestPrepareExecCommand_CredentialWiring` を `TestPrepareCommand_CredentialWiring` へ
      書き替え、`prepareCommand` が返す `preparedCommand.execCmd` に対して
      `SysProcAttr.Credential` の Uid／Gid／Groups を主張する形にする。
      検査後は `preparedCommand.release()` を `t.Cleanup` で呼ぶ。

**テスト**（`executor_lifecycle_test.go`、`//go:build test`、`package executor`）

- [x] `TestPrepareCommand_ChildStreamsAreOSFiles`: `prepareCommand` が返す `execCmd` の
      `Stdout` と `Stderr` が、`outputWriter` が非 `nil` の場合と `nil` の場合の両方で
      `*os.File` である（型アサーションで確認）。`t.Cleanup` で `release()` を呼ぶ。
      **設計文書 §8 は AC-01 の検証を Phase 1 に置いているが、型アサーションの対象となる
      `prepareCommand` が入るのは本 Phase なので、ここで行う。** Phase 1 の時点では
      `executeCommandWithPath` の内側にしか `exec.Cmd` が無く、外から観測できない。

- [x] `TestStageFromFD_ReportsFailuresWithoutLogging` を足す。`tu.NewRecordingLogger` を
      `DefaultExecutor.Logger` に差し、`stageFromFD` の後始末が失敗する状況
      （既存 `TestStageFromFD_ChownFailure_CleansUpStagingDir` と同じ作り方で
      ディレクトリを先に読み取り専用にする）で、次の3つを主張する。
      - `cleanupFn()` が非 `nil` のエラーを返す（失敗の情報が握り潰されず戻り値で届く）
      - recorder に記録が**1件も無い**（`Logger` は呼ばれていない）
      - stderr に `WARNING: failed to remove staging directory` を含む1行が出る
      3つ目は `os.Stderr` を `os.Pipe` の書き込み側へ一時的に差し替えて読み取り、
      `t.Cleanup` で元へ戻す。`os.Stderr` はパッケージ変数なので、この差し替えを行う
      テストでは `t.Parallel()` を呼ばない（同一パッケージの別テストの出力を横取りするため）。
      stderr への書き込みは `Logger` を経由しないので、2つ目と3つ目は矛盾しない。
      この区別こそが設計文書 §7.2 の要点なので、同じテストで両方を主張して固定する。
      静的検査（Phase 4-c）は「隙から `Logger` へ到達しない」ことを構文で見るのに対し、
      こちらは実際の挙動を見る。

**完了の目安**: 既存テストがすべて緑。外から見える挙動（終了コード、標準出力、
標準エラー出力、エラー種別）が変わらない。

### PR-2 作成ポイント: command lifecycle decomposition

**対象ステップ**: 2

**推奨タイトル**: `feat(0171): split command execution into prepare, start and supervise phases`

**レビュー観点**: 外から見える挙動（終了コード・標準出力・標準エラー出力・エラー種別）が変わらないこと / `preparedCommand.release()` が準備フェーズの確保した資源をすべて覆うこと / `stageFromFD` の2つの警告が握り潰されず戻り値と `stagingWarn` で運ばれること / redaction を通らない `os.Stderr` への直書きが staging ディレクトリの削除失敗の1件に限られ、その制約が doc コメントに書かれていること / `bindingStagedCopy` で `exec.Command` を使わない理由が実装に反映されていること

**実装モデル要件**: standard

**判定理由**: 挙動を変えない構造の組み替えであり、未踏の設計判断・パネルモードの引き金（重い統合テスト／CI／外部資源の面、security-gate／移行）・approach 未確定・隔離された高リスク step のいずれにも当たらないため。隙の中のログ出力の全廃とredaction を通らない stderr 書き込みは設計文書 §7.2 で判断済みで、実装は1箇所に限られる。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] `make deadcode` が新たな未使用シンボルを報告しない（後続 PR が使うまで未使用のままの記号があるため）
- [x] §8 の `executeCommandWithPath`／`prepareExecCommand` の横断検索が 0 件を返す
      （`exec.CommandContext` はこの PR ではまだ残る。その検索は PR-4 で行う）。
      検索中に見つかった2箇所の古いコメント（`executor_privilege_check_test.go` は
      `prepareCommand` を、`output_pump.go` は `prepareCommand`＋`startPrepared`＋
      `superviseCommand` を指すよう修正した）
- [x] §8 の新規型名（`boundedBuffer`／`outputPump`／`preparedCommand`／`killStrategy`／`execBinding`）が executor パッケージの外へ漏れていない（マッチ 0 件）
- [x] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: `exec.CommandContext` の置き換えとキャンセル・kill（F-003）

**前提**: 本 Phase は Phase 2 が作る `command_lifecycle.go`／`preparedCommand`／
`superviseCommand` の上に載る。Phase 2 より前には入れられない。

**変更するファイル（3-a〜3-c）**: `internal/runner/base/runnertypes/config.go`、
`internal/runner/base/privilege/unix.go`、
`internal/runner/base/privilege/testutil/mocks.go`、
`internal/runner/base/privilege/unix_privilege_test.go`、
`internal/runner/base/audit/logger.go`、
`internal/runner/base/executor/command_lifecycle.go`、
`internal/runner/base/executor/executor.go`、
`internal/runner/base/executor/test_helpers.go`

#### 3-a. 型と定数

- [x] `runnertypes.Operation` に `OperationKillAfterCancel Operation = "kill_after_cancel"` を追加する。
- [x] `privilege/unix.go` の `prepareExecution` の `switch` の
      `case runnertypes.OperationUserGroupExecution, runnertypes.OperationFileValidation:` へ
      `runnertypes.OperationKillAfterCancel` を足す。
- [x] `command_lifecycle.go` にエラー変数 `ErrKillStrategyUnset`／`ErrKillAfterCancel`／
      `ErrChildNotReaped` を設計文書 §4.1 の文言で定義する。設計文書 §4.1 の残り2つは
      先に入っている（`ErrOutputPipe` は Phase 1、`ErrExecBindingUnset` は Phase 2）。
      いずれも本 Phase のキャンセル・kill 経路が最初の利用者である。
- [x] `killGraceDelay` を `DefaultExecutor` の非公開フィールドにし、`NewDefaultExecutor` の
      既定値を `5 * time.Second` とする。この上限は**別々の2つの待機**に同じ値を使うので、
      その2つを混同しないよう doc コメントへ書き分ける。
      (1) `Wait()` が返るまでの上限。超過は「子を回収できなかった」であり `ErrChildNotReaped`
      になる。(2) 出力中継が読み切るまでの上限。`Stdout`／`Stderr` が `*os.File` になった後
      （Phase 1）、`Wait()` は copy goroutine を待たないので、**パイプの書き込み側を握ったまま
      残る孫プロセスが延ばすのは (2) であって (1) ではない**。(2) の超過は「出力を読み切れなかった」
      であり、終了コードは `Wait()` から分かるのでエラーにはしない。
- [x] `test_helpers.go`（`//go:build test`）へ `WithKillGraceDelay(d time.Duration) Option` を足す。
      既存の `WithFdExecDisabled`／`WithExitFunc` と同じ形にする。これが無いと
      待機を打ち切る経路を通すテストが1本あたり5秒かかる。
- [x] `test_helpers.go` へ `WithWaitFn(fn func(*exec.Cmd) error) Option` を足す。待機 goroutine は
      この関数が非 `nil` ならこれを、既定では `execCmd.Wait()` を呼ぶ。`ErrChildNotReaped` は
      「`Wait()` が返らない」ことでしか起きず、`SIGKILL` を受けた子は実機では必ず回収されるため、
      この注入口が無いとこの経路を決定的に通せない（孫プロセスにパイプを握らせても
      `Wait()` は返る。上の `killGraceDelay` の doc コメント参照）。

#### 3-b. 監査メトリクス

- [x] `audit.PrivilegeMetrics` に隙ごとの内訳を追加する:
      `ByOperation map[runnertypes.Operation]time.Duration`。`ElevationCount` と
      `TotalDuration` は残し、意味も変えない。
- [x] `audit.Logger.LogUserGroupExecution` が `ByOperation` の各要素を、operation 名を含む
      一意なキーで出すようにする（キー: `"privilege_duration_" + string(op) + "_us"`、
      値: `slog.Int64` のマイクロ秒）。ミリ秒では起動区間（数十マイクロ秒）が 0 に潰れて
      AC-05 を検証できないため、マイクロ秒で出す。キーは operation 名でソートした順に足す。
- [x] `executeWithUserGroup` の計測を隙ごとに分け、隙を1つ閉じるたびに `ElevationCount` を
      1増やし、`ByOperation` へ経過時間を加算する。この時点で実際に開く隙は
      `user_group_execution`（まだ `prepareCommand` から `superviseCommand` までを包む）1つだけで、
      `ByOperation` にもこのキーしか現れない。`kill_after_cancel` は 3-d が kill 経路を配線した
      時点から、`staging_cleanup` はその隙を作る 4-a から、同じ枠へ加算されるようになる。
      値そのものが意味を持つのは隙が縮んだ後（AC-05 の検証は 4-a より後の統合テスト）であり、
      本ステップが固定するのは内訳の出し方だけである。

#### 3-c. モックの拡張

- [x] `privilege/testutil/mocks.go` の `MockPrivilegeManager` へ次の3つを足す。
      既存の `Supported`／`ElevationCalls`／`ShouldFail`／`ExecFn` の意味は変えない。
      - `InWindow func(phase MockWindowPhase)`: 隙の内側で**2回**呼ばれる。
        `MockWindowPhaseBeforeFn` は `fn` を呼ぶ直前、`MockWindowPhaseAfterFn` は
        `fn` が戻った直後で、どちらもまだ隙は開いている。`MockWindowPhase` は同パッケージの
        列挙型とし、零値は `MockWindowPhaseUnset`（宣言し忘れを表す）とする。
        隙が開いている**その瞬間の状態**（goroutine 集合、子プロセスの生死、フラグ）を
        採るためのもので、`fn` の中で何が呼ばれたかは観測できない
        （呼び出し集合の主張は §7.2 の静的検査が担う）。
        **`fn` の後の1回が要る理由**: `fn` の実行中に生まれたものは `fn` の前の標本に現れない。
        `Stdout`／`Stderr` を `*os.File` から別の writer へ戻す回帰が起きると、
        `os/exec` は copy goroutine を `Start()` の**中**で起こすので、前の標本しか採らない
        AC-02／AC-04 のテストは緑のまま通ってしまう。
      - `FailFor map[runnertypes.Operation]error`: operation 別の失敗注入。
        `ShouldFail` は全 operation を同時に失敗させるため、起動区間を成功させて
        kill 区間だけを失敗させる経路が作れない。
      - `UnixPrivilegeManager` と同じ再入ガード: 隙が開いている間の再入を検知して
        `privilege.ErrReentrantPrivilegeCall`（`privilege/errors.go:22`）を**そのまま返す**。
        モック専用のセンチネルエラーは作らない。作ると AC-11 が、本番コードのどの経路も
        返しえないエラーを主張することになる。import の循環は起きない:
        `privilegetestutil` が今 import しているのは `context`／`errors`／`runnertypes` だけで、
        `privilege` パッケージ側は `privilege/testutil` を import していない
        （`unix_privilege_test.go` などの同パッケージ内テストも含めて）。
        ガードが無いと AC-11 の「再入ガードが発火しない」という主張が無条件に通ってしまう。
- [x] `unix_privilege_test.go` の `ErrUnsupportedOperationType` を主張している表へ、
      `OperationKillAfterCancel` が**弾かれない**ことを示す行を足す。

**完了の目安（3-a〜3-c）**: 新しい operation・エラー変数・監査属性・モックの注入口が入り、
既存テストが変更なしで緑。`make test`／`make lint`／`make deadcode` が緑。
この時点では新しい隙はまだ開かない（`OperationKillAfterCancel` を実際に使うのは 3-d）。

### PR-3 作成ポイント: operation types, audit metrics and privilege mocks

**対象ステップ**: 3-a / 3-b / 3-c

**推奨タイトル**: `feat(0171): add kill operation type, per-operation privilege metrics and mock hooks`

**レビュー観点**: `OperationKillAfterCancel` を昇格が要る側へ足す `switch` の変更が、他の operation の扱いを変えていないこと / 監査属性のキー（`privilege_duration_<op>_us`）が一意で、マイクロ秒で出ること / `MockPrivilegeManager` の再入ガードが `UnixPrivilegeManager` と同じ意味であり、`ShouldFail`／`ExecFn` の既存の意味を変えていないこと / この PR の時点では新しい隙が開かないこと（`ByOperation` に現れるのは `user_group_execution` だけ）

**実装モデル要件**: standard

**判定理由**: 型・定数・監査属性・テスト用注入口の追加のみで、frontier のいずれの引き金にも当たらないため。追加した operation が実際に隙を開くのは PR-4 であり、その PR を frontier-required としている。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] `make deadcode` が新たな未使用シンボルを報告しない（後続 PR が使うまで未使用のままの記号があるため）
- [x] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [x] PR を作成した（[#1097](https://github.com/isseis/go-safe-cmd-runner/pull/1097)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3（続き）: キャンセルと kill の実装

**変更するファイル（3-d・3-e）**: `internal/runner/base/executor/command_lifecycle.go`、
`internal/runner/base/executor/executor.go`、
`internal/runner/base/executor/executor_lifecycle_test.go`、
`internal/runner/base/executor/executor_fdexec_test.go`、
`internal/runner/group_executor_test.go`

#### 3-d. 監督フェーズのキャンセル経路

- [x] `prepareCommand` の `exec.CommandContext` を `exec.Command` へ替える。Phase 2 から
      持ち越した最後の1点であり、ここから先はキャンセル・タイムアウト時の kill を
      `superviseCommand` が自前で行う。**この置き換えと下の `select` の実装は同じ PR に入れる**
      （片方だけを入れると子を殺す担い手がいなくなる。Phase 2 の該当項目を参照）。
- [x] `command_lifecycle.go` へ `stagingRequest` を設計文書 §3.1 のフィールド構成で定義し、
      `bindingStagedCopy` の経路の `stageFromFD` 呼び出しを `prepareCommand` から
      `startPrepared` へ移す。この経路の `prepareCommand` は `exec.Cmd` を構造体リテラルで
      組み立て、`stagingRequest` を埋めるだけにする（`exec.Command` は `LookPath` を走らせ、
      まだ存在しない staged path に対して `Cmd.Err` を立ててしまうため）。
      `startPrepared` は `stageFromFD` を呼んで `execCmd.Path` を確定させてから
      `execCmd.Start()` を呼ぶ。この移動は 4-a が隙を `startPrepared` だけへ縮めたとき、
      staged copy が実効 UID 0 で作られる（＝起動者に差し替えられない）ために要る
      （設計文書 §3.4 差分1）。
- [x] `superviseCommand` に `select { case <-ctx.Done(): … case res := <-waitCh: … }` を実装する。
      待機 goroutine は `execCmd.Wait()` だけを呼び、結果をバッファ1のチャネルへ送る。
- [x] kill 対象の PID は `Start()` が成功した直後（起動区間の内側）に `execCmd.Process.Pid` から
      控え、kill 区間の前後を通じてこの控えた値だけを使う。`reaped == false` になった後は
      `execCmd` のどのフィールドにも触れない（`ProcessState`、`Process` を含む）。
- [x] `pc.kill == killReelevated` のとき、`Process.Kill()` の呼び出しだけを
      `WithPrivileges`（`Operation: OperationKillAfterCancel`）で包む。
      `killDirect` のときは包まずに呼ぶ。`killUnset` は `ErrKillStrategyUnset` で失敗する。
- [x] kill したこと、対象 PID、選ばれた `killStrategy` を `Info` で記録する。
      記録は隙の外で行う（設計文書 §7.2）。
- [x] `Kill()` が `os.ErrProcessDone` を返した場合はエラーとして扱わない。
- [x] kill 後の回収（`Wait()` の完了）を `killGraceDelay` で打ち切る。超過時は出力中継の
      読み取り側を閉じ、`ErrChildNotReaped` に PID を添えて `Error` で記録し、
      `Result.ExitCode` を `ExitCodeUnknown` にする。
- [x] 子を回収**できた**後の出力中継の `wait(killGraceDelay)` が `timedOut` を返した場合
      （パイプの書き込み側を握ったまま残る孫プロセスがいる場合）は、エラーにせず
      `Logger.Warn` で記録して打ち切る。終了コードと `*exec.ExitError` は `Wait()` から
      得られているので `ExitCodeUnknown` にはしない。記録は隙の外で行う。
      **`ErrChildNotReaped` とは別の事象である**（前者は `Wait()` が返らないこと、
      後者は出力を読み切れないこと）。
- [x] kill の `WithPrivileges` が失敗したときは `ErrKillAfterCancel` に PID を添えて返し、
      待機は `killGraceDelay` で打ち切る。
- [x] kill 区間の計測を 3-b の枠（`ByOperation` の `kill_after_cancel`）へ届ける。
      監査メトリクスを持つのは `executeWithUserGroup` だけなので、`superviseCommand` は
      閉じた隙を `preparedCommand.privilegeWindows` に記録するにとどめ、
      `executeWithUserGroup` が `WithPrivileges` から戻った後にまとめて畳み込む。
      4-a が開く後始末区間も同じ枠に載る。
- [x] エラーの優先順位を設計文書 §4.2 のとおり実装する。書き込みエラー（stdout 優先）を最優先、
      次にキャンセル由来のエラー、最後に `Wait()` のエラー。
      `ErrKillAfterCancel`／`ErrChildNotReaped` は順位の外に置き、`errors.Join` で必ず併記する。
- [x] キャンセル由来の kill では、`ctx.Err()` と `Wait()` のエラーの両方を `errors.Is` で
      たどれる形にして返す（設計文書 §4.2 順位2、§5.6）。
- [x] `prepareCommand` の最後（起動区間を開く前）に `ctx.Err()` を検査し、非 `nil` なら
      `release()` してそのエラーを返す。**このとき呼び出し元は `&Result{ExitCode: ExitCodeUnknown}` を
      非 `nil` で返す**（既存の `TestExecute_ContextCancellation` が `result != nil` を主張しているため）。
      実装では、この扱いを準備フェーズの失敗**全体**に広げた。「キャンセルされたときだけ」を
      区別するには呼び出し元が戻り値のエラーを調べるしかなく、"Declare, don't infer" に反する。
      準備フェーズが失敗した以上どの経路でも終了コードは存在しないので、
      `ExitCodeUnknown` の置き場は同じである。
- [x] `superviseCommand` の doc コメントへ、設計文書 §4.3 の対応表のうちコード上の判断に直結する
      3点を英語で要約して残す（非機能要件「可読性」）: キャンセル済み context を準備フェーズで
      弾くこと、`os.ErrProcessDone` をエラーとしないこと、`killGraceDelay` を設けた理由が
      `os/exec` の `WaitDelay` 既定（上限なし）と意図的に異なることの3点。

#### 3-e. テスト

既存の `executor_lifecycle_test.go` の
`TestExecute_PrepareFailureRecordsCarriedWarnings` は
`TestExecute_CancelledContextReleasesAndRecordsWarnings` へ置き換える。前者は
「staging が済んだ後に準備フェーズが失敗する」状況を作って警告の持ち出しを主張していたが、
3-d が staging を起動フェーズへ移したのでその状況自体が作れなくなった。後者は同じ主張を、
3-d が新しく足した準備フェーズ最後の失敗点（`ctx.Err()` の検査）で行う。
`pipeFn` の差し替えで出力中継の読み取り側を裏から閉じ、`release()` を失敗させて
その警告が記録されることと、キャンセル済み context が起動前に弾かれることを同時に主張する。

`executor_supervise_test.go`（`//go:build test`、`package executor`。当初は
`executor_lifecycle_test.go` へ足す計画だったが、監督フェーズのテストだけで
同ファイルと同程度の分量になるため別ファイルへ分けた）:

- [x] `TestSupervise_TimeoutJoinsContextAndWaitErrors`: タイムアウトで殺した実行の戻り値から
      `context.DeadlineExceeded` と `Wait()` のエラーの両方を `errors.Is` でたどれる。
- [x] `TestSupervise_CancelKillsChild`: `context.WithCancel` のキャンセルで子が停止し、
      `context.Canceled` をたどれる。
- [x] `TestSupervise_KillOpensExactlyOneReelevation`: `run_as` 実行のキャンセルで、
      `MockPrivilegeManager.ElevationCalls` に `kill_after_cancel` がちょうど1回現れ、
      kill 区間の `MockWindowPhaseBeforeFn` の時点で子プロセスがまだ生きており、
      `MockWindowPhaseAfterFn` の時点では生きていない（＝ kill はこの隙の中で起きる）。
      隙の中の呼び出し集合そのものは静的検査（`G`）が主張するので、ここでは重ねない。
      本テスト群は `Execute` ではなく `prepareCommand`／`startPrepared`／`superviseCommand` を
      直接呼び、`pc.kill` を手で `killReelevated` にする。非特権のテストは本物の `run_as` の子を
      起動できない（`SysProcAttr.Credential` の `setgroups` が `CAP_SETGID` を要る）ためで、
      本物の資格情報は Phase 5 の特権付き統合テストが担う。
      なお `MockWindowPhaseAfterFn` の時点の「生きていない」は、`SIGKILL` の配送が非同期な以上
      1回の標本では言えないので、隙を開いたまま子の消滅を待って判定する
      （待てたこと自体が「kill はこの隙の中で起きた」の証拠になる）。
- [x] `TestSupervise_NormalExecutionDoesNotReelevate`: `run_as` を伴わない実行のキャンセルで、
      `WithPrivileges` が一度も呼ばれない（`ElevationCalls` が空）。
- [x] `TestSupervise_KillRunsOnExecutingGoroutine`: 起動区間と kill 区間の
      `MockWindowPhaseBeforeFn` で
      `runtime.Stack(buf, false)` の goroutine ヘッダ行（`goroutine N [`）を採り、
      両者の N が一致すること、および `privilege.ErrReentrantPrivilegeCall` が返らないことを
      主張する。後者はモックの再入ガード（3-c）が入って初めて意味を持つ。
      起動区間はテスト自身が開く（4-a が縮めた後の形）。本番の `executeWithUserGroup` は
      この Phase ではまだ全体を包んでいるので、そこから来た run-as の kill は
      **kill 区間を開かず直接シグナルを送る**（`preparedCommand.supervisedInsideStartWindow`）。
      入れ子の `WithPrivileges` は再入ガードが `fn` を呼ぶ前に弾くため、開こうとすると
      子に一切シグナルが届かず、`main` の `exec.CommandContext` の watchdog が
      隙の内側（実効 UID 0）で殺せていた挙動に対する退行になる。
      隙の内側なので実効 UID は既に 0 であり、直接の `Kill()` で足りる。
      このフィールドは 4-a が隙を縮めた時点で削除する。
- [x] `TestSupervise_ProcessAlreadyDoneIsNotAnError`: 子が終了した後に context が
      キャンセルされた場合、`os.ErrProcessDone` がエラーとして返らない。
      競争に勝つのではなく競争そのものを消す作りにした: テストが自分で `Wait()` して子を回収し、
      `startupErr` で kill 経路を強制するので、`Process.Kill()` は必ず終了済みの子を見る。
- [x] `TestSupervise_ChildNotReapedReportsUnknownExitCode`: `WithKillGraceDelay(50ms)` と
      `WithWaitFn`（テストが解放するまで戻らない `Wait`）を与え、`ErrChildNotReaped` が返り、
      `Result.ExitCode` が `ExitCodeUnknown` であることを確かめる。注入した `Wait` は
      `t.Cleanup` で必ず解放する。**孫プロセスにパイプを握らせる作り方はこの経路を通さない**:
      `Stdout`／`Stderr` が `*os.File` なので `Wait()` は copy goroutine を待たず、
      殺された子はすぐ回収される。
- [x] `TestSupervise_GrandchildHoldingPipeDoesNotBlockCompletion`: `WithKillGraceDelay(50ms)` を
      与え、子が起こした孫プロセスがパイプの書き込み側を保持したまま残る状況
      （`/bin/sh -c 'sleep 30 & exec sleep 30'`）でキャンセルし、`Execute` が孫の寿命（30秒）を
      待たずに戻ることと、出力を読み切れなかった旨の記録が1件出ることを確かめる。
      孫プロセスはテストの後始末で確実に殺す（`t.Cleanup`。PID は孫自身に一時ファイルへ書かせる）。
      **`Result.ExitCode` が `ExitCodeUnknown` ではないことは主張できない**:
      `ExitCodeUnknown` は `-1` であり、`SIGKILL` で死んだ子の `ProcessState.ExitCode()` も `-1` で、
      両者は値として区別できない。代わりに、終了状態が `Wait()` 由来であることを
      `*exec.ExitError` がたどれることで示し、`ErrChildNotReaped` の経路と別物であることを
      `NotErrorIs` で主張する。
- [x] `TestSupervise_SizeLimitErrorOutranksExitError`: 書き込みエラー（出力上限超過を模した
      センチネルエラー）が起き、子が `SIGPIPE` で異常終了する状況で、返るエラーの主因がセンチネルエラーで
      あり、`*exec.ExitError` が主因として現れないことを主張する（AC-14 の本題）。
- [x] `TestSupervise_KillFailureIsJoinedWithWriteError`: `FailFor` で
      `OperationKillAfterCancel` だけを失敗させ、出力上限超過と kill 失敗が同時に起きたとき、
      センチネルエラーと `ErrKillAfterCancel` の**両方**を `errors.Is` でたどれる。
- [x] `TestStartPrepared_ReleaseFailureStillKillsChild`: `Start()` 成功後、
      `pc.pump` の書き込み側を先に閉じておいて `releaseChildEnds` を失敗させ
      （同一パッケージなので非公開フィールドへ到達できる）、子が kill・回収され、
      そのエラーが結果に現れることを確かめる。**失敗させる手段は `os.ErrClosed` ではない**:
      `closeUnlessClosed` は `os.ErrClosed` を冪等な成功として飲むので、
      `syscall.Close` で記述子を裏から閉じ `EBADF` を起こす
      （`TestPreparedCommand_ReleaseRecordsPumpFailure` と同じ手口）。

レビューを受けて次の3本を追加した（いずれも計画には無かったが、3-d が新しく足した
コードのうちテストの無い部分を覆う）。

- [x] `TestKillChild_RecordsOnlyWindowsThatOpened`: 監査メトリクスへ届く隙の記録が、
      **実際に開いた隙だけ**であること。`WithPrivileges` が `fn` に入る前に失敗した場合
      （再入・operation 不正・昇格失敗）に記録すると、監査ログに実在しない昇格が並び、
      その監査ログこそが AC-06／AC-09 を検証する典拠なので害がある。
      開いたかどうかは隙の内側で立てるフラグで宣言する（エラーからの推測にしない）。
- [x] `TestKillChild_InsideStartWindowSignalsDirectly`: 起動区間の内側から来た
      `killReelevated` の kill が、隙を開かずに直接シグナルを送ること
      （`ElevationCalls` が空で、子が止まる）。上記の退行を止める主張である。
- [x] `TestKillChild_RejectsUndeclaredAndUnavailableStrategies`: `killUnset` と
      特権マネージャ不在の2つの fail-secure 経路で、シグナルを送らずに理由を返すこと。
      `prepareCommand` が唯一の構築子である限り到達しないが、守っている `switch` が
      特権の境界に載っているため主張する（設計原則5）。
- [x] `TestSupervise_UnreapedChildDoesNotSpendASecondDrainDeadline`: 回収を諦めたときは
      出力中継の読み取り側を閉じてから読み切りへ入り、`killGraceDelay` を2回続けて
      使わないこと（設計文書 §6.1 の「読み取り側を閉じ」）。
      回収できない子こそがパイプの書き込み側を握っているので、待っても出力は来ない。

あわせて、kill の `Info` 記録は kill が**成功したとき**だけ出し、
`ctx` のキャンセル由来か起動フェーズの失敗由来かを文面で分ける
（失敗した kill を「殺した」と記録すると、直後の `Error` 記録と食い違う）。

`executor_fdexec_test.go`（`package executor_test`）:

- [x] `TestExecute_NoLeakOnCancellationPaths` を足す。既存の
      `TestExecute_FdBoundNoLeak`／`TestExecute_FdBoundStartFailureNoLeak` と同じ作法を守る:
      計測前に1回ウォームアップ実行を行ってから `before := numOpenFDs(t)` を採り、
      ループの各回で `require.NoError(t, plan.Close())` を呼ぶ。
      対象は (a) キャンセル済み context、(b) 実行中のキャンセル（＝ kill 経路）の2経路だけとし、
      20 回反復して `after <= before+1` を主張する。
      **当初 (b) は「子の終了後のキャンセル」としていたが、これに替えた**:
      その瞬間を決定的に作れないうえ、資源の解放経路は正常終了と同じで
      既存の `TestExecute_FdBoundNoLeak` が覆っている。本 Phase が新しく足すのは kill 経路である。
      **`Start()` 失敗の経路は既存 `TestExecute_FdBoundStartFailureNoLeak` が覆っているので作らない。**
      **特権側の主張はここには置かない。** このファイルはビルドタグ無しで `make test` から
      非特権ユーザーとして走るため、`os.Geteuid() == os.Getuid()` は executor が何をしても
      無条件に成り立ち、主張する理由で落ちられない（CLAUDE.md）。AC-12 の特権側は
      setuid モデルで走る Phase 5-b の統合テストが担う。

`internal/runner/group_executor_timeout_test.go`（`//go:build test`、`package runner`。
`group_executor_test.go` はビルドタグを持たないが、このテストは `//go:build test` の
`resource/testutil` などを要るため別ファイルにした）:

- [x] `TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded` を足す。実 executor
      （モックのリソースマネージャではなく `executor.NewDefaultExecutor`）を通して
      1秒タイムアウトで `sleep 10` を実行し、`tu.NewRecordingLogger` に
      `LogTimeoutExceeded` の記録が現れることを主張する。
      **本タスクで唯一意図して変える挙動（設計文書 §5.6）の効果は、`executor.Execute` の
      戻り値だけでは観測できない。** `LogTimeoutExceeded` は `group_executor.go:584-585` の
      `errors.Is(err, context.DeadlineExceeded)` に掛かっており、現在は成立しないため
      事実上呼ばれない。既存の `group_executor_test.go` のタイムアウトテストは
      `context.DeadlineExceeded` を返すモックを使っており、実 executor を通らないので
      この変更の効果を証明しない。

**完了の目安（3-d・3-e）**: 上記テストが緑。通常実行のタイムアウトが働く。`make test`／`make lint` が緑。

### PR-4 作成ポイント: cancel and kill supervision

**対象ステップ**: 3-d / 3-e

**推奨タイトル**: `feat(0171): replace exec.CommandContext with explicit cancel and kill handling`

**レビュー観点**: kill 区間が `Process.Kill()` 1回だけを包み、`killDirect`／`killReelevated` の宣言どおりに分岐すること / `reaped == false` の後に `execCmd` のどのフィールドにも触れないこと / エラーの優先順位と `errors.Join` による併記が設計文書 §4.2 と一致すること / `killGraceDelay` の2つの用途（`Wait()` の打ち切り＝`ErrChildNotReaped` と `ExitCodeUnknown`／
出力中継の読み切りの打ち切り＝記録のみ）が分かれていること / `FailFor` と再入ガードが AC-11／AC-14 の主張を空洞化させないこと

**実装モデル要件**: frontier-required

**判定理由**: ステップ 3-d が新しい特権昇格の隙（kill 区間）を実際に開く security-gate step であり、同時に `select` による待機・kill・回収打ち切りという状態機械と並行処理を持つ隔離された高リスク step でもあるため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [x] PR を作成した（[#1100](https://github.com/isseis/go-safe-cmd-runner/pull/1100)）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 特権の隙の縮小と staging の位置づけ変更（F-002）

**変更するファイル（4-a・4-b）**: `internal/runner/base/executor/executor.go`、
`internal/runner/base/executor/command_lifecycle.go`、
`internal/runner/base/runnertypes/config.go`、
`internal/runner/base/privilege/unix.go`、
`internal/runner/base/executor/executor_lifecycle_test.go`

#### 4-a. 隙の縮小

- [x] `runnertypes.Operation` に `OperationStagingCleanup Operation = "staging_cleanup"` を追加し、
      `prepareExecution` の昇格が要る側の `case` へ足す。
- [x] `executeWithUserGroup` の `WithPrivileges` の `fn` を `startPrepared(pc)` の呼び出しだけにする。
      あわせて `preparedCommand.supervisedInsideStartWindow` とその代入、
      `killChild` の直接 kill の分岐、`TestKillChild_InsideStartWindowSignalsDirectly` を削除する
      （3-d が入れた暫定措置であり、隙が縮んだ時点で kill 区間を開くのが正しくなる）。
      `prepareCommand` は隙の前、`superviseCommand` は隙の後に置く。
- [x] `executeNormal` は `startPrepared` を `WithPrivileges` で包まずにそのまま呼ぶ。
- [x] `releaseChildEnds()` と複製した検証済み記述子の解放を、`WithPrivileges` から戻った直後
      （隙の外）に置く。`Start()` の成否によらず必ず通る1箇所にする（設計文書 §3.1 の骨格）。
- [x] staged copy の削除を子プロセスの終了後（および回収を諦めた後）へ移す。
      `binding == bindingStagedCopy` かつ `cred != nil` のときだけ、
      `WithPrivileges`（`Operation: OperationStagingCleanup`）で包んで `os.RemoveAll` を呼ぶ。
      それ以外（通常実行）は隙を開かずに削除する（設計文書 §3.4 差分2）。
- [x] `ErrChildNotReaped` の経路でも staged copy を削除する（設計文書 §6.1）。
- [x] 後始末区間の計測を `ByOperation` へ足す（キー `staging_cleanup`）。3-b で入れた
      計測の枠へ、この Phase で新しく開く隙の分を加える。
- [x] `Start()` が失敗した経路では、staged copy の削除は起動区間の中で行う（設計文書 §6.3）。
- [x] `preparedCommand.stagingWarn` の記録位置を確定する: `WithPrivileges` から戻った直後
      （隙の外、`releaseChildEnds` と同じ位置）で、非 `nil` なら `Logger.Warn` で記録する。
- [x] `binding == bindingStagedCopy` のとき、staged copy のパスを起動区間が閉じた直後に
      `Logger.Debug` で記録する（設計文書 §6.1）。復帰失敗による `emergencyShutdown` は
      この記録より後に起きるため、その経路で `$TMPDIR` に残った複製の在処が追える。
      記録は隙の外で行うので、隙の中の操作は増えない。
- [x] `cleanupFn()` の戻り値の記録位置を確定する: 後始末区間を閉じた後、および隙を開かない
      通常実行の削除の後で、非 `nil` なら `Logger.Warn` で記録する。
      `Start()` 失敗時の削除（起動区間の内側）の戻り値も、隙を出てから記録する。
- [x] `privilege/unix.go` の `WithPrivileges` が隙の内側で出している `Debug` 2件
      （「Executing privileged operation callback」「Privileged operation callback completed」）を
      隙の外へ出す。1件目は昇格の前、2件目は復帰の後に置く。本タスクの carry-out の仕組みは
      すべて「隙の中では何もログしない」を前提にしており、呼び出される側がその前提を
      内側から崩していた。あわせて AC-05 が測る隙の長さからハンドラ2回分が外れる。
- [x] 複製した検証済み記述子の解放の失敗は、`superviseCommand` へ渡す `startupErr` に
      混ぜない。書き込み側が閉じないと読み取り側が EOF に達しないので実行を止める必要があるが、
      `Start()` が既に子へ複製し終えた記述子にはその帰結が無く、混ぜると正常に走っている
      コマンドを SIGKILL して失敗として報告することになる。何も走っていない2経路
      （隙が開かなかった／`Start()` 失敗）では従来どおり戻り値のエラーに含める。
- [x] `ErrStartPhaseNotRun` を足す。起動フェーズを走らせずエラーも返さない `startWindow` は
      拒否する。そうしないと `Result` が `nil`、エラーも `nil` で返り、`executeWithUserGroup`
      の監査ブロックがそれを参照する。本リポジトリの `PrivilegeManager` はこの挙動を取らないが、
      ガードは取りうる場所に置いて fail-secure にする。
- [x] `outputPump.start` の doc コメントを直す。「隙が閉じた後に呼ばれるのは事実である」は
      起動区間についてのみ成り立ち、kill 区間と後始末区間は読み取り goroutine が生きたまま
      開く。設計文書 §5.3 が受け入れている残存リスクであることを明示して参照させる。
- [x] `escalatePrivileges` が `seteuid(0)` の直後に出す `Info`（「Privileges elevated」）は
      移さず、残存リスクとして設計文書 §5.3 に記す。この記録の意味は「いつ昇格したか」であり、
      復帰の後へ動かすと監査の時系列が実際と食い違う。あわせて、静的検査（Phase 4-c）が
      固定するのは executor の隙の本体だけであることを同節に明記する。
- [x] `preparedCommand.stagingWindowErr` を足す。後始末が**試みられる前に**止まった理由
      （昇格の拒否、`stagingCleanupStrategy` 未宣言）を記録する。`stagingCleanupErr`
      （`os.RemoveAll` 自身の失敗）とは別にするのは、`release()` の隙なしの再試行が返す
      EACCES だけを読んだ運用者が「昇格は行われた」と誤読するのを防ぐためである。
- [x] `WithPrivileges` の中で復帰に失敗したとき、`emergencyShutdown` によりプロセスが即座に
      終わるため子プロセスと staged copy が残る旨を、`startPrepared` の doc コメントへ書く。
      あわせて、この経路では `stagingWarn` と後始末の戻り値が記録されないまま失われることも書く
      （記録は隙を出てから行うため。`emergencyShutdown` 自身は CRITICAL を記録する）。

#### 4-b. テスト（`executor_lifecycle_test.go`）

- [x] `TestStartPrepared_NoGoroutineInsideWindow` を足す（AC-02）。次の点を守る。
      - 標本を**3つ**採る: 隙を開く直前（基準）、`InWindow` の `MockWindowPhaseBeforeFn`、
        `InWindow` の `MockWindowPhaseAfterFn`。いずれも `runtime.Stack(buf, true)` を使う。
        **`MockWindowPhaseAfterFn` の標本が本題である**: `Start()` の中で起きた goroutine は
        `MockWindowPhaseBeforeFn` の標本には現れないので、前だけを採ると
        `*os.File` への切り替えを revert しても緑のままになる。
        `buf` は `runtime.Stack` の戻り値が
        `len(buf)` 未満になるまで倍々に伸ばす（既定サイズだと黙って切り詰められ、
        差分が空に見える偽の緑になる）。
      - 比較は **goroutine ID**（各ブロック冒頭の `goroutine N [` から採る N）の集合で行う。
        文字列全体の比較にすると、状態文字列（`[running]`／`[chan receive]`）の変化だけで
        差分が出る。隙の中で採った2つの標本の**どちらか**に基準集合に無い ID が現れたら失敗とする。
      - **executor パッケージのフレームで絞り込まない**（設計文書 §7.1）。`os/exec` の
        copy goroutine は `io.Copy`／`internal/poll` のフレームしか持たないため、
        絞ると `*os.File` への切り替えを revert してもテストが緑のままになる。
      - 除外する goroutine は、先頭フレームで同定した**リテラルの一覧**としてテスト内に書き、
        1件ずつ理由をコメントする。対象は Go ランタイム（`runtime.gcBgMarkWorker`、
        `runtime.bgsweep`、`runtime.bgscavenge`、`runtime.runfinq`）、`testing` パッケージ、
        `-race` 有効時に現れる race detector の goroutine、およびログ機構の Slack 送信ワーカー。
      - `make test` は `-race` 付きと無し（CGO_ENABLED=0）の両方で走るので、除外一覧は
        両方の条件で成立させる。
- [x] `TestExecute_SingleElevationPairPerRun` を足す（AC-06）。**正常終了する**実行で組む。
      当初は監査メトリクスの `ElevationCount`／`ByOperation` を主張する計画だったが、
      特権の無いテストでは run-as 実行を最後まで走らせられない（`SysProcAttr.Credential` の
      `setgroups` が CAP_SETGID を要求し、`Start()` が EPERM で失敗する）ため、
      同じ事実を観測できる次の3点に置き換えた。このテストが通る成功経路では、昇格と復帰の対は
      `WithPrivileges` の呼び出しと1対1であり、`ElevationCount` はその回数と一致する
      （昇格を断られた呼び出しはどの区間でも `ElevationCount` に数えないが、モックの
      呼び出し一覧には残るため、失敗経路では両者はずれる）。
      - fd-bound 実行: 特権管理器の呼び出しが起動区間の1件だけ、`pc.privilegeWindows` が空。
      - staging フォールバック（`WithFdExecDisabled`）: 呼び出しが
        `user_group_execution` と `staging_cleanup` の2件、`pc.privilegeWindows` に
        後始末区間が1件記録され、staged copy が実行後に消えている。
      - `prepareCommand` が資格の有無から `stagingCleanupStrategy` を宣言すること
        （run-as なら `cleanupElevated`、通常実行なら `cleanupDirect`）。
      監査メトリクスへの畳み込み自体は `pc.privilegeWindows` を走査するだけの処理であり、
      特権のある環境での主張は Phase 5 の統合テスト（AC-05）が担う。
- [x] `TestStartPrepared_WaitAndPumpRunOutsideWindow` を足す（AC-04 の挙動側）。
      「`start()` が呼ばれたか」は `outputPump` に既にある非公開フラグ `started`
      （二重呼び出しの拒否に使っている）がそのまま使えるので、新しい旗は足さない。それを使い、
      `InWindow` の**2つの phase のどちらの時点でも**そのフラグが偽であること、および
      待機 goroutine がまだ起動していないことを主張する。`MockWindowPhaseAfterFn` の側が
      要るのは、`fn`（＝`startPrepared`）の中で `pump.start()` を呼ぶ回帰が
      `MockWindowPhaseBeforeFn` の標本では見えないためである。静的検査（`G`）が
      「隙の中から何を呼べるか」を固定するのに対し、こちらは「隙が閉じるまで
      `Wait()` と出力の取り込みが始まらない」という時間順序を主張する。
- [x] `TestExecute_ShebangScriptRunsUnderStagingFallback` を足す（AC-17）。
      `WithFdExecDisabled` の下でシェバンつきスクリプトが実行でき、標準出力が一致する
      （staged copy を `Start()` 直後に削除していないことの検査）。
- [x] `TestRemoveStagedCopy_NormalExecutionDoesNotElevate` を足す。通常実行の staging では
      後始末区間を開かないこと（設計文書 §3.4 差分2 の「条件を広く採ってはならない」側）。
      kill 側の `TestSupervise_NormalExecutionDoesNotReelevate` に対応する。
- [x] `TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow` を足す。`Start()` 失敗時の
      削除が**隙が閉じる前に**済んでいることを `InWindow` の `MockWindowPhaseAfterFn` から
      主張する。実行後に「消えている」ことを見るだけでは、特権の無いテストでは
      `release()` の再試行が消してしまうため、削除を外しても緑のままになる。
- [x] `TestRemoveStagedCopy_RejectsUndeclaredAndUnavailableStrategies` を足す。
      `stagingCleanupStrategy` 未宣言と、`cleanupElevated` かつ特権管理器が無い場合の
      2つの fail-secure 分岐。戻り値が呼び出し側で捨てられるため、理由が
      `stagingWindowErr` に記録されることまで主張する。

**完了の目安（4-a・4-b）**: 上記テストが緑。隙の縮小後も既存テストが変わらず通る。
`make test`／`make lint` が緑。

### PR-5 作成ポイント: privilege window narrowing

**対象ステップ**: 4-a / 4-b

**推奨タイトル**: `feat(0171): narrow the privilege window to the start phase`

**レビュー観点**: `WithPrivileges` の `fn` が `startPrepared` の呼び出しだけになっていること / `releaseChildEnds` と検証済み記述子の解放が `Start()` の成否によらず隙の外の1箇所を通ること / staged copy の削除が3経路（正常終了・`ErrChildNotReaped`・`Start()` 失敗）すべてで行われ、`cred != nil` のときだけ後始末区間を開くこと / `stagingWarn`／`cleanupFn()` の戻り値／staged copy のパスを記録する位置がすべて隙の外にあること

**実装モデル要件**: frontier-required

**判定理由**: ステップ 4-a は特権昇格の範囲そのものを定義し直し、後始末区間（`OperationStagingCleanup`）を新たに開く security-gate step であり、誤ると実効 UID 0 のまま処理が続くため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [x] PR を作成した（[#1101](https://github.com/isseis/go-safe-cmd-runner/pull/1101)）
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4（続き）: 隙の中から到達する呼び出しの静的検査

**変更するファイル（4-c）**: `internal/runner/base/executor/privileged_window_guard_test.go`（新規）、
`internal/runner/base/executor/testdata/`（新規）

#### 4-c. 静的検査（`privileged_window_guard_test.go`、`//go:build test`、`package executor`）

既存の [`identity_mutation_guard_test.go`](../../../internal/runner/base/privilege/identity_mutation_guard_test.go)
は**denylist 方式**（追跡対象を `Seteuid` など17個の識別子に限り、その呼び出しサイトを許可リストと
照合する）であり、呼び出しグラフを辿らない。本検査は「隙の中から到達する呼び出しの集合」を見るので
到達解析が要り、既存 guard より難しい。方式を決めずに「リスト外は失敗」とすると、
`stageFromFD` の `fmt.Errorf`／`filepath.Base`／`filepath.Join`／`io.NewSectionReader` が
リスト外になって初日から赤くなる。そこで次のとおり範囲を先に固定する。

- [ ] **追跡対象を決める。** 追跡するのは「副作用を持ちうる呼び出し」だけとし、
      具体的には `os`／`syscall`／`io`／`os/exec` パッケージの関数呼び出しと、
      `*os.File`／`*os.Process`／`*exec.Cmd` のメソッド呼び出し、および `Logger` のメソッド呼び出しに限る。
      `fmt`／`filepath`／`errors`／`strconv` など副作用の無いパッケージの呼び出しは追跡しない。
      この線引きの理由（許可リストが実装の写しではなく「隙の中で何に触れるか」の宣言であること）を
      テストファイルの doc コメントへ英語で書く。
- [ ] **到達解析の範囲を決める。** `WithPrivileges` へ渡す関数リテラルを起点に、
      **同一パッケージ内の関数・メソッドだけ**を辿る（`startPrepared` → `stageFromFD` の2段が実際の深さ）。
      パッケージ外へ出る呼び出し（`os.MkdirTemp` など）はそこで葉として扱い、
      許可リストと突き合わせる。この境界をテストファイルの doc コメントへ書く。
- [ ] **レシーバ型の同定手段を決める。** `(*os.File).Stat` と `(*os.Process).Kill` は
      レシーバの型が分からないと同定できないため、`go/parser` に加えて標準ライブラリの
      `go/types`（`go/importer` 経由）で型情報を取る。`golang.org/x/tools` は `go.mod` に無く、
      本タスクで新しい依存は増やさない。
- [ ] **許可リストを書く。** 設計文書 §7.2 の表をそのまま写す。例外は無い。
      `stageFromFD` は同一パッケージの関数なので**葉ではなく辿る対象**である。設計文書 §7.2 の
      表との対応を保つために名前は残すが、許可リストと突き合わせるのはその先の葉である。
      - 起動区間: `(*exec.Cmd).Start`、`stageFromFD`（辿る）、`os.MkdirTemp`、`syscall.Dup`、
        `os.NewFile`、`os.OpenFile`、`io.Copy`、`os.Chmod`、`os.Chown`、`os.RemoveAll`、
        `(*os.File).Stat`、`(*os.File).Close`、`syscall.Close`、`(*os.File).WriteString`
      - kill 区間: `(*os.Process).Kill` のみ
      - 後始末区間: `os.RemoveAll` と `(*os.File).WriteString`
- [ ] **`Logger` の呼び出しは3つの隙すべてで禁じる。** `Logger` のメソッドは追跡対象に
      含めたうえで、許可リストには1つも載せない（設計文書 §7.2）。Phase 2 で `stageFromFD` から
      `Logger.Warn` を取り除いてあるので、この規則は例外なしに成立する。
      `(*os.File).WriteString` を許すのは stderr への最後の手段の記録のためであり、
      ハンドラ経由のログ出力を許すものではない。この区別（パスを開く／既に開いた fd へ書く）を
      テストファイルの doc コメントへ英語で書く。
      なお `io.Copy` が既に許可リストにあるため、「開いている記述子へ書ける」という能力の階級は
      `(*os.File).WriteString` の追加によって増えない。
- [ ] **フィールドへの代入は対象外とする。** `bindingStagedCopy` で起動区間が行う
      `execCmd.Path` への代入は呼び出しではないため検査しない（設計文書 §7.2）。
- [ ] **negative self-test を置く。** `testdata/` に、隙の中で許可リスト外の呼び出しを行う
      小さな Go ソースを2種類置き、検査関数がどちらも**拒否する**ことを主張するサブテストを書く。
      1つは許可リストに無いファイル操作（例: `os.Remove`）、もう1つは隙の中からの
      ログ出力（例: `Logger.Warn`）である。後者を入れるのは、ログ禁止が許可リストの
      「載せなかった」という不在によって表現されており、不在は壊れても目に見えないためである。
      これが無いと、到達解析が壊れて何も見つけなくなっても検査は緑のままになる
      （CLAUDE.md「テストは主張する理由で落ちられなければならない」）。
- [ ] `TestPrivilegeWindowAllowedCalls` を、`start_window`／`kill_window`／`cleanup_window`／
      `rejects_unlisted_call`／`rejects_logging_in_window`（後の2つが negative self-test）の
      5サブテストで構成する。

**完了の目安（4-c）**: 静的検査と negative self-test が緑。`make test`／`make lint` が緑。

### PR-6 作成ポイント: static guard for privilege windows

**対象ステップ**: 4-c

**推奨タイトル**: `test(0171): add static guard for calls reachable inside the privilege windows`

**レビュー観点**: 追跡対象・到達解析の範囲・レシーバ型の同定手段という3つの前提がテストの doc コメントに書かれていること / 許可リストが設計文書 §7.2 の表と一致し、`Logger` のメソッドを1つも含まないこと / negative self-test 2種（許可リスト外の呼び出し・隙の中のログ出力）が実際に拒否されること / `golang.org/x/tools` を足さず `go/parser` と `go/types` だけで型解決していること

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ 4-c は既存の `identity_mutation_guard_test.go`（denylist 方式・呼び出しグラフを辿らない）に前例が無い到達解析つきの検査であり、隔離された複雑な step にあたるため。追跡対象・解析範囲・型解決手段は 4-c 自身が先に固定しているので未踏の設計判断は残っておらず、テストだけを足す PR で挙動は変わらないため frontier-required には上げない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 統合テストと実行経路の整備（F-007）

**変更するファイル**: `internal/runner/base/executor/privileged_test_condition_test.go`、
`internal/runner/base/executor/executor_privilege_gap_integration_test.go`（新規）、
`Makefile`、`.pre-commit-config.yaml`

#### 5-a. スキップ判定

- [ ] `privileged_test_condition_test.go` へ純関数
      `canRunSetuidModelIntegrationTest(uid, euid int, targetUser string) (ok bool, reason string)`
      を追加する。`canRunPrivilegedIntegrationTest` は変更しない（既存の補助グループ統合テストが
      `sudo`（実 UID = 0）前提で使っているため。設計文書 §7.3）。
      新しい述語は `canRunPrivilegedIntegrationTest(euid, targetUser)` の判定に加えて
      `uid != 0` を要求する。
- [ ] 同ファイルへ薄いラッパー `requireSetuidModel(t skipper)` を追加する。`skipper` は
      同ファイルに置く narrow interface
      `type skipper interface { Helper(); Skipf(format string, args ...any) }` であり、
      `*testing.T` はそのまま満たす。**引数を `*testing.T` にすると、下の
      `TestRequireSetuidModel_ReadsDocumentedEnvVar` がスタブを渡せずコンパイルできない。**
      中身は `os.Getuid()`／`os.Geteuid()`／`os.Getenv("TEST_RUNAS_TARGET_USER")` を読んで
      純関数へ渡し、偽なら理由つきで `t.Skip` するだけ。
      **判定を4つのテストへ書き写さない。** 書き写すと、後でヘルパーへ括り出したときに
      AC-21 の件数チェックが赤くなり、環境変数名を1箇所間違えても全テストがサイレントにスキップする。
- [ ] `TestCanRunSetuidModelIntegrationTest` を同ファイルへ足す。既存の
      `TestCanRunPrivilegedIntegrationTest` と同じ表形式で、
      `real_uid_is_root`／`not_root_euid`／`no_target_user_configured`／
      `target_user_does_not_exist`／`conditions_satisfied` を網羅する。
- [ ] `TestRequireSetuidModel_ReadsDocumentedEnvVar` を足す。`t.Setenv` で
      `TEST_RUNAS_TARGET_USER` に存在しないユーザー名を設定し、`requireSetuidModel` が
      その名前を含む理由でスキップすることを（`skipper` を満たす小さなスタブを渡し、
      `Skipf` に渡された書式と引数を記録して）確かめる。環境変数名の取り違えは、
      全テストがサイレントにスキップするという最悪の失敗モードを生むので、明示的に固定する。

#### 5-b. 統合テスト

- [ ] `executor_privilege_gap_integration_test.go` を新規に作る
      （`//go:build integration`、`package executor_test`）。冒頭コメントに
      `-tags "test integration"` が要る理由（`executor/testutil` が `test` タグを持つ）を書く。
- [ ] 各テストの冒頭で `requireSetuidModel(t)` を呼ぶ（4本すべて）。
- [ ] 監査ログの受け口を共通のヘルパーへ括る: `tu.NewRecordingLogger()` で
      `*slog.Logger` と `*tu.LogRecorder` を作り、`audit.NewAuditLoggerWithCustom(logger)` を
      `executor.WithAuditLogger` へ渡す。`audit.NewAuditLogger` は構築時に `slog.Default()` を
      掴んでしまい差し替えられないので、この経路を使う。
- [ ] `TestPrivilegeGap_StartWindowIndependentOfCommandDuration`（AC-05）: 1秒休止と5秒休止の
      2コマンドを実行し、監査ログの `privilege_duration_user_group_execution_us` を比べる。
      判定は絶対値で行う: **両者とも 5,000 マイクロ秒（5ms）未満**であり、かつ
      **両者の差が 2,000 マイクロ秒未満**であること。
      設計文書 §1.4／§5.2 は起動区間を `fork`／`execve` の時間（数十マイクロ秒）と見積もっており、
      5ms はその 100 倍以上の余裕を見た上限である。ここを 100ms のように緩めると、
      出力中継の起動や `Wait()` が隙の中へ戻る回帰を見逃す。
- [ ] `TestPrivilegeGap_TimeoutKillsChild`（AC-07）: `run_as_user` 付きの長時間コマンドが
      タイムアウトで停止し、戻り値から `context.DeadlineExceeded` をたどれる。
- [ ] `TestPrivilegeGap_CancelKillsChild`（AC-08）: 同じ条件で `context.CancelFunc` による
      キャンセル（SIGINT／SIGTERM 相当）により子が停止する。停止の確認は、
      `/proc/<pid>` が消えることではなく `Execute` が制限時間内に戻ることで行う。
- [ ] `TestPrivilegeGap_TimeoutKillsChild` と `TestPrivilegeGap_CancelKillsChild` の両方で、
      `Execute` の前後に `os.Geteuid()` を採り、値が一致することを併せて主張する
      （AC-12 の特権側）。setuid モデルでは実効 UID は 0、実 UID は起動者なので、
      非特権環境で成り立つ `os.Geteuid() == os.Getuid()` とは逆の関係になる。ここで見るのは
      「実効 UID が `Execute` の呼び出しをまたいで変わらないこと」、すなわち起動区間・
      kill 区間・後始末区間のいずれからも復帰していることである。
- [ ] `TestPrivilegeGap_OutputLimitAbortsRunningChild`（AC-13、run-as 版）: `MaxSize` を
      小さくした `output.Capture` を渡し、上限を超え続ける出力を出す `run_as` コマンドが、
      コマンドの終了を待たずに打ち切られる。非特権版は Phase 1 の
      `TestExecute_OutputLimitAbortsRunningChild` が常時実行される形で覆っている。

#### 5-c. 実行経路

- [ ] `Makefile` に `executor-privileged-integration-test` ターゲットを追加する。内容は
      既存 `integration-test`（598-603行）と同じシェル展開形で環境変数を転送する:
      `$(ENVSET) TEST_RUNAS_TARGET_USER="$$TEST_RUNAS_TARGET_USER" CGO_ENABLED=1 $(GOTEST) -tags "test integration" -v ./internal/runner/base/executor/`。
      `ENVSET` が `env -i` で環境を空にするため、この明示的な転送が要る。
      `.PHONY` 行へターゲット名を足す。
- [ ] 同ターゲットを `test-ci`、`test-ci-cgo1` の依存へ足す。
- [ ] `Makefile` のターゲット一覧コメント（458-470行付近）へ新ターゲットの説明を**英語で**足す。
      次の3点を書く: (1) `TEST_RUNAS_TARGET_USER` が未設定なら空文字が渡り、テストは
      `no target user configured` の理由でスキップする（呼び出し方は
      `TEST_RUNAS_TARGET_USER=<user> make executor-privileged-integration-test`）。
      (2) 特権と対象ユーザーが揃わない環境ではスキップし、このターゲットが主張するのは
      `integration` タグ付きファイルがコンパイルでき、スキップ判定が働くことまでである。
      (3) `//go:build integration` のファイルは `make lint` の対象外である
      （`GOLINT` も pre-commit の `golangci-lint` も `--build-tags test` だけを渡す）。
      型・シグネチャの誤りはこのターゲットのコンパイルで捕まえる。
- [ ] `.pre-commit-config.yaml` に、このターゲットを呼ぶフックを足す。
      必須フィールドを欠かさない: `id: executor-privileged-integration-test`、
      `name: executor privileged integration test`、
      `entry: make executor-privileged-integration-test`、`language: system`、
      `types: [go]`、`pass_filenames: false`。
      `repo: local` のフックは `id`／`name`／`entry`／`language` が必須で、`name` を欠くと
      pre-commit の設定検証に失敗し、リポジトリ全体のフックが動かなくなる。
      既存の `go-test` フックは `make` を経由せず `-tags test` だけで走るため、
      このフックが無いと `integration` タグ付きファイルは pre-commit でコンパイルすらされない。
- [ ] 新ターゲットは `-race` を付けない（既存の `elfanalyzer-integration-test` と同じ）。
      設計文書 §3.2 要点7 の競合検出は `-race` 付きで走る `output_pump_test.go` が担い、
      統合テストは検出しないことを `Makefile` のコメントへ書く。

**完了の目安**: `go test -run '^$' -tags "test integration" ./internal/runner/base/executor/`
がコンパイルを通る。特権のある環境で AC-05／AC-07／AC-08／AC-13 が緑（§4.3 の手順で確認）。
非特権環境では理由つきでスキップし、pre-commit が緑のままである。

### PR-7 作成ポイント: privileged integration tests and execution paths

**対象ステップ**: 5-a / 5-b / 5-c

**推奨タイトル**: `test(0171): add privileged integration tests and their execution paths`

**レビュー観点**: スキップ判定が `requireSetuidModel` の1箇所にあり、4本のテストへ書き写されていないこと / `ENVSET` が `env -i` で環境を空にする下で `TEST_RUNAS_TARGET_USER` が転送されること / 非特権環境では理由つきでスキップし、`test-ci` と pre-commit が緑のままであること / `.pre-commit-config.yaml` の必須フィールド（`name` を含む）が揃っていること

**実装モデル要件**: frontier-required

**判定理由**: ステップ 5-b／5-c は setuid テストバイナリという外部資源の面と、`Makefile` の CI 合成ターゲットおよび pre-commit フックを触る面を持ち、パネルモードの引き金「重い統合テスト／CI／外部資源の面」に当たるため。スキップ判定を誤ると全テストがサイレントにスキップし、無検証のまま緑になる。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] `go test -run '^$' -tags "test integration" ./internal/runner/base/executor/` が終了コード 0（`make lint`／`make test` は `integration` タグ付きファイルをコンパイルしないため、グリーンゲートだけでは足りない）
- [ ] `make -n executor-privileged-integration-test` と `pre-commit validate-config .pre-commit-config.yaml` が §7 AC-22 の期待どおり
- [ ] §4.3 の実行手順を特権のある環境で走らせ、`--- SKIP` が無いことを確認して結果を §4.3 へ追記した
- [ ] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: 文書と doc コメントの更新（F-006）

**変更するファイル**: `internal/runner/base/privilege/unix.go`、
`internal/runner/base/output/capture.go`、`internal/redaction/error_collector.go`、
`internal/logging/log_line_tracker.go`、`internal/testutil/synccensus/census_guard_test.go`、
`docs/dev/architecture_design/security-architecture.ja.md`、
`docs/dev/architecture_design/security-architecture.md`、
`docs/user/security-risk-assessment.ja.md`、`docs/user/security-risk-assessment.md`、
`docs/translation_glossary.md`、
`docs/tasks/0170_excess_synchronization_removal/03_implementation_plan.md`

#### 6-a. Go の doc コメント

- [ ] `privilege/unix.go` の `WithPrivileges` doc コメントの次の段落（91-96行）を置き換える（AC-20）。
      **各語が行をまたがないよう改行位置を守る**（§7 の検証コマンドが行単位で照合するため）。

      変更前:
      ```
      // The window is not serialized: while it's open, the process-wide euid is
      // raised for every goroutine, including os/exec's copy goroutines for
      // non-*os.File writers. This is an unresolved design issue -- those copy
      // goroutines already run at euid 0 on every privileged command, without any
      // parallel execution feature -- and fixing it needs a separate design, not a
      // lock here.
      ```

      変更後:
      ```
      // The window is not serialized: while it's open, the process-wide euid is
      // raised for every goroutine. The executor no longer leaves any of its own
      // goroutines running inside the start window: it hands *os.File pipe ends to
      // exec.Cmd and reads them only after the window closes. What stays exposed is
      // the logging side's Slack send worker, the fork/execve window inside Start
      // itself, and the kill and staging-cleanup windows, which open while the
      // output-pump reader goroutines are alive. Removing those needs a separate
      // design -- pausing the pump around the window, or moving privileged
      // operations into a separate process -- not a lock here.
      ```
- [ ] `output/capture.go` の `Capture` doc コメントの次の段落を置き換える。

      変更前:
      ```
      // os/exec starts one goroutine per writer when Cmd.Stdout/Cmd.Stderr is not
      // an *os.File, and stdout and stderr wrappers share this Capture: the
      // executor gives both the stdoutWrapper and the stderrWrapper the same
      // OutputWriter, so the two per-writer goroutines can call WriteOutput on this
      // Capture concurrently. mutex protects the fields those goroutines contend
      // on.
      ```

      変更後:
      ```
      // The executor's output pump starts one reader goroutine per stream, and the
      // stdout and stderr wrappers share this Capture: the executor gives both the
      // same OutputWriter, so the two output-pump reader goroutines can call
      // WriteOutput on this Capture concurrently. mutex protects the fields those
      // goroutines contend on.
      ```
- [ ] `redaction/error_collector.go` の `InMemoryErrorCollector` doc コメントの
      `output copy goroutine os/exec starts for a non-*os.File Cmd.Stdout/Cmd.Stderr, so
      RecordFailure` の前半を（同じ言い回しが `log_line_tracker.go` にもあるので、
      `RecordFailure` を含む側がこのファイルである）
      `output-pump reader goroutine the executor starts for the child's stdout/stderr pipes` へ
      書き替える。`mu` が要ることは変わらない。
- [ ] `logging/log_line_tracker.go` の `DefaultLogLineTracker` doc コメントを同じ理由で
      同じ言い回しへ書き替える。`atomic.Int64` が要ることは変わらない。
- [ ] `synccensus/census_guard_test.go` の3行の `reason` を書き替える。
      - `capture.go` / `mutex`: `"the executor's output-pump reader goroutines for stdout and stderr share this Capture"`
      - `log_line_tracker.go` / `lineCounter`: `"incremented from the executor's output-pump reader goroutine"`
      - `error_collector.go` / `mu`: `"reached from the executor's output-pump reader goroutine through the redacting handler"`
      出力中継は同期プリミティブを宣言しない（チャネルで join する）ので、行の追加は起きない。

#### 6-b. 設計・利用者向け文書

- [ ] `security-architecture.ja.md`（1196行付近）の「特権管理」節の残存リスク1件目を置き換える（AC-19）。

      変更前:
      ```
      - 特権の隙が開いている間、参加しない goroutine は保護されない。これは未解決の設計課題である
      ```

      変更後:
      ```
      - 特権の隙は、コマンド実行の経路では起動区間（`fork`／`execve`。staging フォールバックでは
        検証済みバイナリの複製も含む）まで縮まった。この区間で走る非参加 goroutine は
        ログ機構の Slack 送信ワーカーだけである
      - キャンセル時の kill 区間と、staging フォールバックの後始末区間は、出力読み取りの
        goroutine が生きている間に開く。どちらも例外的な場合にだけ開き、中で行うのは
        `kill(2)` 1回または自分が作ったディレクトリ1つの削除だけである。これは受け入れた残存リスクである
      ```
- [ ] 同節の残存リスク2件目（再入ガードが同期を伴わない旨）は維持する。本タスクは前提を
      強めこそすれ変えないため、文言を変えない。
- [ ] `security-risk-assessment.ja.md`（99-101行付近）の残存リスクを置き換える（AC-19）。

      変更前:
      ```
      - 特権の隙が開いている間、プロセス全体の実効 UID が上がる。`WithPrivileges` に参加しない goroutine も
        その実効 UID で走るため保護されない。これは未解決の設計課題である
      ```

      変更後:
      ```
      - 特権の隙が開いている間は、プロセス全体の実効 UID が上がる。ただし隙は
        起動区間（子プロセスを起こす `fork`／`execve` の一瞬）まで縮まっており、
        コマンドの実行時間には比例しない
      - 起動区間の中で走る、隙に参加しない goroutine はログ通知（Slack 送信）の処理だけである。
        キャンセル時のプロセス停止と、一時複製の後始末のときにだけ開く短い隙では、
        出力を読み取る処理も動いている。これは受け入れた残存リスクである
      ```
- [ ] 上の置換後テキストで導入する語のうち、`security-architecture.ja.md` の読者に
      前提が無い「起動区間」は、同ファイルの初出箇所に「（子プロセスを起こす `fork`／`execve` の区間）」を
      添えて説明する。`security-risk-assessment.ja.md` の側は上の変更後テキストが既に説明を含む。
- [ ] `security-architecture.md` と `security-risk-assessment.md` を `/mktrans` で
      日本語版から反映する（CLAUDE.md の翻訳方針: 日本語版を先にコミットしてから翻訳する）。
- [ ] `docs/translation_glossary.md` に「出力中継」「起動区間」「kill 区間」「後始末区間」の
      訳語が登録されているか確認し、未登録なら `/mktrans` の手順に従って追加する。

#### 6-c. 先行タスク 0170 の追跡表の陳腐化対応

Phase 6-a の doc コメント更新で、0170 実装計画書の**3つの検証コマンドと2つの完了条件**が
古くなる。5箇所すべてに、0171 で文言が置き換わった旨の注記と置換後の検証コマンドを併記する。
0170 の他の行は変更しない。

- [ ] `03_implementation_plan.md:1471`（0170 AC-11）: `This is an unresolved design issue` が
      `unix.go` から消えるため、期待値 3 が 2 になる。注記と、置換後の検証コマンド
      `rg -F -c 'The window is not serialized' internal/runner/base/privilege/unix.go`（期待値 1）
      および `rg -F -c 'raised for every goroutine' internal/runner/base/privilege/unix.go`（期待値 1）を併記する。
- [ ] `03_implementation_plan.md:1475`（0170 AC-14）: `capture.go` の2つのリテラルが消える。
      注記と、置換後の検証コマンド
      `rg -F -c "the executor's output-pump reader goroutines for stdout and stderr share this Capture" internal/runner/base/output/capture.go`（期待値 1）を併記する。
- [ ] `03_implementation_plan.md:1478`（0170 AC-15）: `output copy goroutine` が消える。
      注記と、置換後の検証コマンド
      `rg -F -c "the executor's output-pump reader goroutine" internal/logging/log_line_tracker.go internal/redaction/error_collector.go`（期待値 各ファイル 1 件以上）を併記する。
- [ ] `03_implementation_plan.md:278` 付近（0170 Step 1-2 の完了条件、`capture.go` の2リテラル）:
      同じ注記を足す。
- [ ] `03_implementation_plan.md:297` 付近（0170 Step 1-3 の完了条件、`output copy goroutine`）:
      同じ注記を足す。

**完了の目安**: §7 の AC-19／AC-20 の検証コマンドが期待どおりの結果を返す。
`make test`（`synccensus` の guard test を含む）が緑。

### PR-8 作成ポイント: documentation and doc comments

**対象ステップ**: 6-a / 6-b / 6-c

**推奨タイトル**: `docs(0171): update security documents and doc comments for the narrowed window`

**レビュー観点**: §7 の AC-19／AC-20 の `rg` 検証が、旧文言の不在と新文言の存在の両方で期待どおりになること / 日本語版を先にコミットし、英語版は `/mktrans` で反映していること / 0170 実装計画書の5箇所が注記の追加にとどまり、歴史的記述を書き換えていないこと / `synccensus` の guard test の3つの理由文字列が出力中継の実態と一致すること

**実装モデル要件**: standard

**判定理由**: 文言の置き換えと追跡表への注記のみで、未踏の設計判断・パネルモードの引き金・approach 未確定・隔離された高リスク step のいずれにも当たらないため。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] §8 の文言に関する横断検索（`output copy goroutine` の不在、置換後文言の存在、0170 追跡表の `0171` 5件以上）が期待どおり
- [ ] §4.4 の性能測定を全変更が入った状態で行い、結果を §4.4 へ追記した
- [ ] この PR が追加したテストについて §4.2 の該当行（仕組みを外すと落ちること）を確認し、コミットメッセージに記した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 成果物 | 判定 |
|---|---|---|---|
| M1: 出力経路の自前化 | Phase 1 | `output_pump.go`、`boundedBuffer`、`*os.File` への切り替え | 既存テストが緑。AC-13（非特権版）／AC-15／AC-16 のテストが通る |
| M2: 構造の分解 | Phase 2 | `command_lifecycle.go`、3フェーズ分解 | 外から見える挙動が変わらない。AC-01 のテストが通る |
| M3: キャンセル経路の自前化 | Phase 3 | `select` によるキャンセル待機、kill 区間、`OperationKillAfterCancel`、監査メトリクスの内訳 | AC-07／AC-10〜AC-12／AC-14 のテストが通る |
| M4: 隙の縮小 | Phase 4 | 起動区間だけを `WithPrivileges` で包む形、後始末区間、静的検査 | AC-02〜AC-04／AC-06／AC-17 のテストと静的検査が通る |
| M5: 検証環境 | Phase 5 | 統合テスト、`Makefile` ターゲット、pre-commit フック | AC-05／AC-08／AC-13（run-as 版）／AC-21／AC-22 |
| M6: 文書 | Phase 6 | 4文書・5つの doc コメント・0170 の5箇所の更新 | AC-19／AC-20 |

順序は 1 → 2 → 3 → 4 → 5 → 6 に固定する。設計文書 §8 は Phase 1 と Phase 3 を
「独立でどちらを先に入れてもよい」としているが、本書の分解では Phase 3 が Phase 2 の
`preparedCommand`／`superviseCommand` の上に載るため、Phase 3 は Phase 2 の後にしか入らない。
Phase 4 は Phase 1 と Phase 3 の両方が入ってから行う（設計文書 §8）。

AC-01 の検証位置は設計文書 §8 の Phase 1 から Phase 2 へ移してある。理由は Phase 2 の
`TestPrepareCommand_ChildStreamsAreOSFiles` の項目に書いた（Phase 1 の時点では
`exec.Cmd` が `executeCommandWithPath` の内側にしか無く、外から観測できない）。

### 3.2 PR 構成

PR の区切りは §2 の各ステップの直後に「PR-N 作成ポイント」として埋め込んである。
各 PR は単独でグリーンゲート（`make test && make lint`）を通り、後続ステップのスタブを要さない。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1 | `output_pump.go`（`boundedBuffer`／`outputPump`／`ErrOutputPipe`）の新設と、出力取り込みの自前化 | frontier-recommended |
| PR-2 | 2 | `command_lifecycle.go` の新設。準備・起動・監督の3フェーズへの分解（挙動は変えない） | standard |
| PR-3 | 3-a / 3-b / 3-c | `OperationKillAfterCancel` とエラー変数、監査メトリクスの内訳、`MockPrivilegeManager` の注入口 | standard |
| PR-4 | 3-d / 3-e | `exec.CommandContext` の置き換え。キャンセル・kill 区間・回収打ち切りとそのテスト | frontier-required |
| PR-5 | 4-a / 4-b | `WithPrivileges` の範囲を `startPrepared` へ縮小。後始末区間と staged copy の削除位置の変更 | frontier-required |
| PR-6 | 4-c | `privileged_window_guard_test.go` の新設（隙から到達する呼び出しの静的検査） | frontier-recommended |
| PR-7 | 5-a / 5-b / 5-c | 特権つき統合テスト、スキップ判定、`Makefile` ターゲット、pre-commit フック | frontier-required |
| PR-8 | 6-a / 6-b / 6-c | doc コメント5箇所、設計・利用者向け文書4本、0170 追跡表の5箇所 | standard |

Phase 3 と Phase 4 だけを2つに割った理由は次のとおり。

- **Phase 3。** 3-d（キャンセル・kill）は本タスクで唯一の状態機械であり、並行処理と
  新しい特権の隙を同時に持つ。型・定数・監査属性・モックの注入口（3-a〜3-c）と混ぜると、
  監査属性のキーの形を読む人が kill の状態機械を読み飛ばせない。3-a〜3-c は
  新しい隙を開かないので、単独で入れても挙動は変わらない。
- **Phase 4。** PR-5 が挙動（隙の範囲）を変え、PR-6 はテストだけを足す。到達解析つきの
  静的検査は、隙の縮小そのものとは別の観点（解析の正しさと許可リストの忠実さ）で読まれる。
  PR-5 の時点でも 4-b の挙動テスト（AC-02／AC-04 の挙動側／AC-06／AC-17）が縮小を検証する。

### 3.3 PR に割り当てた §2 以外の作業

§2 のステップに現れない作業も、宙に浮かないよう PR へ割り当てる。

| 作業 | 割り当て先 |
|---|---|
| §4.2「テストが主張する理由で失敗できることの確認」 | 各 PR。その PR が追加したテストの行を、その PR で確認する（各 PR 作成ポイントのチェックリスト） |
| §4.3 統合テストの実行手順（setuid モデルでの実行と結果の追記） | PR-7 |
| §4.4 性能の確認 | PR-8（全変更が入った状態で測るため） |
| §8 の `executeCommandWithPath`／`prepareExecCommand` の残存参照検索 | PR-2（この2つが消えるのは Phase 2） |
| §8 の `exec.CommandContext` の残存参照検索 | PR-4（`exec.Command` へ替わるのは Phase 3-d） |
| §8 の文言に関する検索と 0170 追跡表の検索 | PR-8 |
| §8 の新規型名が executor パッケージの外へ漏れていないことの検索 | PR-2（PR-1 が入れた `boundedBuffer`／`outputPump` も併せて確認する） |

---

## 4. テスト戦略

### 4.1 単体テスト

設計文書 §7.1 の表を作業へ落としたものが Phase 1〜4 のテスト項目である。方針は次のとおり。

- **境界値**: `boundedBuffer` は `limit` ちょうど／`limit`+1／`2*limit`／`2*limit`+1 を含める。
  出力上限は「上限直下で成功」「上限超過で打ち切り」の両方を置く。
- **エラー経路**: パイプ生成失敗（`pipeFn` の差し替え）、`Start()` 失敗、
  `releaseChildEnds` 失敗（書き込み側を先に閉じる）、kill 失敗（`FailFor`）、
  `Wait()` が返らないことによる `killGraceDelay` 超過（`WithKillGraceDelay` ＋ `WithWaitFn`）、
  出力中継が読み切れないことによる同超過（`WithKillGraceDelay` ＋ パイプを握る孫プロセス）、
  キャンセル済み context を、それぞれ独立したテストで覆う。注入口はすべて Phase 1／3 で用意する。
- **層の切り分け**: AC-02 の goroutine 検査は、executor のフレームで絞らず goroutine ID の
  集合を基準に採る（設計文書 §7.1）。絞ると `Stdout`／`Stderr` を `*os.File` へ替える変更を
  revert しても緑のままになり、テストが主張する理由で落ちなくなる。
- **重複の回避**: 次の3つは既存テストが覆っているので作り直さない。
  - `Start()` 失敗時の記述子リーク → `executor_fdexec_test.go::TestExecute_FdBoundStartFailureNoLeak`
  - キャンセル済み context で `Result` が非 `nil` かつ `context.Canceled` → `executor_test.go::TestExecute_ContextCancellation`
  - `stageFromFD` の失敗時後始末 → `stagefromfd_test.go::TestStageFromFD_*`
  `output_wrapper_test.go` の stdout／stderr 分離とエラー保持も既存の主張を再利用し、
  出力中継のテストは「中継が `outputWrapper` を正しく駆動すること」だけを見る。
- **特権を要さない証拠を必ず1本持つ**: AC-13 は特権と無関係の性質なので、
  非特権で常時走る `TestExecute_OutputLimitAbortsRunningChild`（Phase 1）を主たる証拠とし、
  統合テストは run-as 版の追加確認とする。統合テストだけに置くと、CI でも pre-commit でも
  開発機でもスキップされ、実質的に無検証になる。

### 4.2 テストが主張する理由で失敗できることの確認

設計文書 §7.6 の表の各行について、対象の仕組みを外して落ちることを確かめる。
表の内容は再掲せず、行の識別子だけを挙げる。加えて、§7.6 に無い AC と
**すべての `static` 検査**についても同じ確認を行う（`static` 検査は「変更前に期待どおり
失敗し、変更後に成功する」ことを両方確かめて初めて意味を持つ）。確認したことはコミットメッセージへ書く。

設計文書 §7.6 の各行:

- [ ] AC-01 / AC-02 / AC-06・AC-09 / AC-07 / AC-12 / AC-13・AC-14 / AC-16（`errors.Join`） /
      AC-16（stderr の上限） / AC-17 / §3.1 の起動後解放 / §3.2 要点7

§7.6 に無いぶん:

- [ ] AC-03・AC-04: `privileged_window_guard_test.go` の negative self-test（`testdata/` の
      許可リスト外呼び出し）が実際に拒否されること。到達解析を1段に浅くすると
      `stageFromFD` の中の呼び出しが見えなくなり、この self-test が緑のまま通ってしまうことも確かめる。
- [ ] AC-05: `Wait()` を起動区間の内側へ戻すと、`privilege_duration_user_group_execution_us` が
      閾値（5,000 µs）を超えてテストが落ちる。
- [ ] AC-10: `killStrategy` の宣言を無視して常に `killReelevated` にすると、
      `TestSupervise_NormalExecutionDoesNotReelevate` が落ちる。
- [x] AC-02・AC-04（`fn` の後の標本）: `InWindow` の `MockWindowPhaseAfterFn` の呼び出しを
      外すと、`Stdout`／`Stderr` を `*os.File` 以外の writer へ戻す revert を
      `TestStartPrepared_NoGoroutineInsideWindow` が捕まえられなくなることを確かめる
      （`os/exec` の copy goroutine は `Start()` の中で起きるので、`fn` の前の標本には現れない）。
      確認: `*os.File` を包む writer へ戻すと `MockWindowPhaseAfterFn` の標本にだけ
      `os/exec` の goroutine が2つ現れて落ちる（`fn` の前の標本は差分なし）。
- [x] 検証済み記述子（PR-5 で追加）: 解放の失敗を `superviseCommand` の `startupErr` へ
      混ぜると `TestRunCommand_VerifiedFDCloseFailureDoesNotKillChild` が落ちる。
      `ErrStartPhaseNotRun` のガードを外すと
      `TestRunCommand_StartWindowThatRunsNothingIsRejected` が落ちる。いずれも確認済み。
- [x] 後始末区間（PR-5 で追加）: `cleanupDirect` の分岐を昇格側へ倒すと
      `TestRemoveStagedCopy_NormalExecutionDoesNotElevate` が落ちる。`Start()` 失敗時の
      隙の中の削除を消すと `TestStartPrepared_StartFailureRemovesStagedCopyInsideWindow` が
      落ちる。`stagingWindowErr` への記録を消すと
      `TestRemoveStagedCopy_RejectsUndeclaredAndUnavailableStrategies` が落ちる。いずれも確認済み。
- [ ] AC-11: モックの再入ガード（Phase 3-c）を外すと、kill を別 goroutine から呼ぶ実装に
      戻しても緑のままになることを確かめる（＝ガードが入っていて初めて主張が成立する）。
- [ ] AC-15: stdout 用と stderr 用の `outputWrapper` を取り違えて渡すと落ちる。
- [ ] AC-03・AC-04（`Logger` 禁止）: `stageFromFD` に `Logger.Warn` を1行戻すと、
      `G::TestPrivilegeWindowAllowedCalls` と
      `TestStageFromFD_ReportsFailuresWithoutLogging` の両方が落ちる。
      `testdata/` の `rejects_logging_in_window` も、許可リストに `Logger` を1つ載せると
      通ってしまうことを確かめる（＝この self-test が不在を守れていること）。
- [ ] AC-03・AC-04（stderr の最後の手段）: 後始末の `os.Stderr.WriteString` を消すと、
      `TestStageFromFD_ReportsFailuresWithoutLogging` の3つ目の主張が落ちる。
      逆に `(*os.File).WriteString` を許可リストから外すと
      `G::TestPrivilegeWindowAllowedCalls` が落ちることも確かめ、
      許可リストのこの1行が実装に効いていることを固定する。
- [ ] AC-18: `dryrun_manager.go` へ `d.executor.Execute(...)` の呼び出しを1行足すと、
      AC-18 の `static` 検査が 1 件を返して落ちる。
- [ ] AC-19 / AC-20 の各 `static` 検査: 置換前のファイルに対して実行し、期待と異なる結果
      （旧文言の検査は 1 件以上、新文言の検査は 0 件）になることを確かめてから、置換後に再実行する。
- [ ] AC-21: `requireSetuidModel` が読む環境変数名を1文字変えると
      `TestRequireSetuidModel_ReadsDocumentedEnvVar` が落ちる。
- [ ] AC-22: `.pre-commit-config.yaml` から `name` を外すと pre-commit の設定検証が失敗する
      ことを確かめる（`pre-commit validate-config`）。

### 4.3 統合テストの実行手順（AC-21、AC-22）

setuid モデル（実 UID ≠ 0、実効 UID = 0）を作るには、root 所有で setuid ビットを立てた
テストバイナリを非 root ユーザーから起動する。次の3点に注意する。

- **置き場が `nosuid` だと setuid ビットが無視される。** systemd の既定 `tmp.mount` は
  `nosuid` を付けるため、`/tmp` に置くと実効 UID が 0 にならず、全テストが理由も分からず
  スキップし、実行者は「終了コード 0 だから検証できた」と誤認する。置き場のマウントオプションを
  必ず確かめる。
- **バイナリを消す。** root 所有・setuid のテストバイナリを置きっぱなしにすると、
  そのバイナリは自パッケージの `run_as` 実行を任意に行えるため、ローカル権限昇格の踏み台になる。
  実行のたびに使い捨てのディレクトリを作り、最後に消す。
- **スキップは失敗と見なす。** スキップされたのに「確認した」と記録するのが、
  要件定義書がリスクに挙げた「特権が要るテストが常にスキップされる」の実際の姿である。
  出力に `--- SKIP` が無いことを確かめる。
- **テストバイナリの終了コードをパイプで捨てない。** `set -eu` の下でも
  `cmd | tee out.txt` の状態はパイプの**最後**（`tee`）のものになるので、テストバイナリが
  panic しても落ちても打ち切られない。`pipefail` は POSIX の `sh` にあるとは限らないため、
  下の手順ではパイプを使わずファイルへ落としてから `cat` する。

```sh
set -eu

# 1. 対象ユーザー（フィクスチャ）を用意する。root 以外であること。冪等に書く。
id -u scr-runas-fixture >/dev/null 2>&1 || sudo useradd -m scr-runas-fixture

# 2. 使い捨ての置き場を作り、nosuid でないことを確かめる。
d=$(mktemp -d "${TMPDIR:-/var/tmp}/scr-setuid.XXXXXX")
trap 'sudo rm -rf "$d"' EXIT
if findmnt -no OPTIONS -T "$d" | tr ',' '\n' | grep -qx nosuid; then
  echo "FATAL: $d is on a nosuid mount; the setuid model cannot be exercised here" >&2
  exit 1
fi

# 3. テストバイナリを作る（-c は単一パッケージ指定でのみ使える）。
go test -tags "test integration" -c -o "$d/executor.test" ./internal/runner/base/executor/

# 4. root 所有・setuid にする。
sudo chown root:root "$d/executor.test"
sudo chmod u+s "$d/executor.test"

# 5. 非 root ユーザーとして起動する（実 UID = 起動者、実効 UID = 0）。
# パイプを使わない（パイプの状態は tee のものになり、テストバイナリの失敗が消える）。
if ! TEST_RUNAS_TARGET_USER=scr-runas-fixture "$d/executor.test" \
  -test.v -test.run 'TestPrivilegeGap_' > "$d/out.txt" 2>&1; then
  cat "$d/out.txt"
  echo "FATAL: the test binary exited non-zero" >&2
  exit 1
fi
cat "$d/out.txt"

# 6. スキップされていないことを確かめる。
grep -q -- '--- SKIP' "$d/out.txt" && { echo "FATAL: tests were skipped, nothing was verified" >&2; exit 1; }
for t in StartWindowIndependentOfCommandDuration TimeoutKillsChild CancelKillsChild OutputLimitAbortsRunningChild; do
  grep -q -- "--- PASS: TestPrivilegeGap_$t" "$d/out.txt" || { echo "FATAL: TestPrivilegeGap_$t did not pass" >&2; exit 1; }
done
echo "OK: all four privileged criteria verified"
```

- [ ] 上記手順を実際に走らせ、AC-05／AC-07／AC-08／AC-13 が緑（スキップではない）に
      なることを確認する。確認した環境（OS、カーネル、Go のバージョン、置き場のマウントオプション）を
      本節へ追記する。
- [ ] `make executor-privileged-integration-test` が、上の条件が揃わない環境では
      理由つきでスキップし、終了コード 0 で終わることを確認する。

`make executor-privileged-integration-test` は `go test` を直接呼ぶため、setuid バイナリの
経路は通らない。条件が揃わない環境ではスキップするので、このターゲットが pre-commit で
主張するのは「`integration` タグ付きファイルがコンパイルでき、スキップ判定が働くこと」までである。
この限界を `Makefile` のコメントへ書く（Phase 5-c）。

### 4.4 性能の確認（非機能要件）

- [ ] `/bin/true` 相当の短いコマンドを 200 回繰り返し、変更前（`main`）と変更後で実時間の
      中央値を測り、その差を**絶対値**で記録する。判断の基準は `fork`／`exec` に要する
      数十マイクロ秒との比較であり、相対的な増減では判断しない（CLAUDE.md の性能方針）。
- [ ] 測定結果（測定環境、回数、中央値、差）を本節へ追記する。差が数十マイクロ秒の
      オーダーに収まる場合は「実時間に差は出ない」と結論して閉じ、機構は追加しない。

### 4.5 後方互換性

AC-16／AC-17／AC-18 が後方互換性の検証にあたる。意図して変えるのは
「タイムアウト・キャンセルで殺したときのエラーから `context.DeadlineExceeded` をたどれるように
する」1点だけである（設計文書 §5.6。同 §8 は「§5.6 の2点」と書いているが、§5.6 の本文は
1点しか挙げておらず、もう1点（標準エラー出力の切り詰め方）は §5.6 自身が
「変更点から外した」と記している。本書は §5.6 本文の1点に従う）。
dry-run は `DefaultExecutor.Execute` へ到達しないため、本タスクの変更の影響を受けない
（設計文書 §5.7）。

---

## 5. リスク管理

| リスク | 影響 | 緩和 | 対応する作業 |
|---|---|---|---|
| 自前のパイプ管理による記述子の漏洩 | 長時間動作するプロセスで記述子を使い切る | 生成から解放までを `outputPump` と `preparedCommand.release()` に閉じ、`Start()` 失敗経路（既存テスト）とキャンセル2経路（新規テスト）を `numOpenFDs` で計数する | Phase 1、Phase 3 の `TestExecute_NoLeakOnCancellationPaths` |
| 読み取り側の閉じ忘れによるデッドロック | `Wait()` が戻らずコマンドがハングする | 書き込み側の解放を隙の外の必ず通る1箇所に置き、長時間コマンドと大量出力の両方でテストする | Phase 1、Phase 3 |
| `exec.CommandContext` の意味論の取りこぼし | キャンセルが効かない、または二重に kill する | 設計文書 §4.3 の対応表の各行に検証を割り当て、コード上の判断に直結する3点は `superviseCommand` の doc コメントへ残す | Phase 3-d のテスト項目と doc コメント項目 |
| 再昇格の追加による復帰漏れ | 実効 UID 0 のまま処理が続く | kill 経路も既存の `WithPrivileges` を通し、復帰と識別子検査を共通経路で行う。`Execute` 末尾の `identityChecker` は変更しない | Phase 3 |
| 特権が要るテストが開発環境で常にスキップされる | 受け入れ基準が実質的に検証されない | (1) 特権と無関係な AC-13 は非特権テストを主たる証拠にする。(2) 環境変数名の取り違えを `TestRequireSetuidModel_ReadsDocumentedEnvVar` で固定する。(3) §4.3 の手順が `--- SKIP` を失敗として扱う | Phase 1、Phase 5、§4.3 |
| 静的検査の許可リストが実装から離れる／検査が空洞化する | AC-03／AC-04 の主張が空洞化する | 許可リストを go/ast + go/types の guard test に置き、追跡対象・到達範囲・型解決手段を先に固定する。negative self-test で検査自体が落ちられることを確かめる | Phase 4-c |
| 起動区間の中で `os.MkdirTemp` が走る（staging フォールバック時） | 「隙の中でファイルを開かない」という性質が staging 経路では成り立たない | 設計文書 §3.4 差分1 は、この配置を意図して選んでいる。複製を隙の外で作ると staged copy が起動者所有になり、差し替え可能になるためである。本タスクはこの判断を変えない。許可リストを `stageFromFD` が実際に呼ぶものだけに固定し、範囲が広がらないようにする | Phase 4-c の許可リスト |
| 隙の中のログ出力が、ハンドラ経由で将来 `open` を行いうる | slog ハンドラはファイルを開くことが許されており、隙の中で呼べばその `open` が euid 0 で行われる。出力コピー goroutine について本タスクが取り除いたのと同じ危険 | 隙の中の `Logger` 呼び出しを全廃する（設計文書 §7.2）。`stageFromFD` の2つの警告は戻り値と `preparedCommand.stagingWarn` で隙の外へ運ぶ。許可リストに `Logger` を1つも載せず、negative self-test で「隙の中のログ出力を拒否すること」を確かめる | Phase 2、Phase 4-a、Phase 4-c |
| 隙を出る前に `emergencyShutdown` やクラッシュでプロセスが死ぬと、運び出した記録が失われる | `$TMPDIR` に残った root 所有の複製の在処が誰にも分からなくなる | 失われると困る1件（staging ディレクトリの削除失敗）だけ、既に開いている stderr の記述子へも書く。パスを開かないので §7.2 の禁止と両立する。あわせて staged copy のパスを起動区間の直後に `Debug` で記録し、`emergencyShutdown` の経路でも在処が追えるようにする | Phase 2、Phase 4-a |
| stderr への直書きが redaction ハンドラを通らない | 秘匿情報を書けば、そのまま起動者の選んだ宛先へ出る | 載せてよいのは秘匿情報を含まないと分かっている値だけ、という制約を doc コメントに書く。実際に載せるのは staging ディレクトリのパスと errno だけである | Phase 2 |
| 隙の外へ運んだ警告を、呼び出し元が記録し忘れる | staging の失敗が誰にも見えなくなる | 通常のエラー伝播にしたので、握り潰せば `errcheck`（`.golangci.yml:9` で有効）が検出する。汎用のログバッファを置かないのはこのためで、バッファ方式の `flush` 忘れは lint でも静的検査でも捕まらない | Phase 2、`make lint` |
| Phase 4 の同時変更が大きく、切り分けが難しい | 障害時の revert 単位が粗くなる | Phase を独立した PR に分け、Phase 4 は挙動を変える PR-5 と静的検査だけの PR-6 に分ける。PR-6 → PR-5 の順に戻せば隙の範囲が現状へ戻る | §3.2 の PR 構成 |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: 1）
- [ ] PR-2 マージ済み（対象ステップ: 2）
- [ ] PR-3 マージ済み（対象ステップ: 3-a / 3-b / 3-c）
- [ ] PR-4 マージ済み（対象ステップ: 3-d / 3-e）
- [ ] PR-5 マージ済み（対象ステップ: 4-a / 4-b）
- [ ] PR-6 マージ済み（対象ステップ: 4-c）
- [ ] PR-7 マージ済み（対象ステップ: 5-a / 5-b / 5-c）
- [ ] PR-8 マージ済み（対象ステップ: 6-a / 6-b / 6-c）
- [ ] §4.2 のすべての項目について、仕組みを外すとテストが落ちることを確認した
- [ ] §4.3 の実行手順を実環境で走らせ、スキップされていないことを確認し、結果を追記した
- [ ] §4.4 の性能測定を行い、結果を追記した
- [ ] §7 の受け入れ基準検証のすべての項目が期待どおりの結果を返す
- [ ] §8 の横断検索チェックリストがすべて期待どおりの結果を返す

---

## 7. 受け入れ基準の検証

種別は `test`（実行可能。挙動が誤っていれば落ちる）／`static`（`rg` または静的検査）／
`manual`（実環境での観測）で示す。パスは省略のため次の別名を使う。

- `L` = `internal/runner/base/executor/executor_lifecycle_test.go`
- `S` = `internal/runner/base/executor/executor_supervise_test.go`
- `P` = `internal/runner/base/executor/output_pump_test.go`
- `G` = `internal/runner/base/executor/privileged_window_guard_test.go`
- `I` = `internal/runner/base/executor/executor_privilege_gap_integration_test.go`
- `F` = `internal/runner/base/executor/executor_fdexec_test.go`
- `C` = `internal/runner/base/executor/privileged_test_condition_test.go`
- `E` = `internal/runner/base/executor/executor_test.go`
- `W` = `internal/runner/base/executor/output_wrapper_test.go`

`static` の各コマンドは、§4.2 のとおり**変更前に期待と異なる結果を返すこと**も確かめてから確定する。

### AC-01: `Stdout`／`Stderr` に渡すのは `*os.File`

- 種別: `test`
- 検証: `L::TestPrepareCommand_ChildStreamsAreOSFiles`
- 期待: `outputWriter` が非 `nil`／`nil` の両サブテストで `execCmd.Stdout` と `execCmd.Stderr` の
  `*os.File` への型アサーションが成功する
- 実装: Phase 1（`newOutputPump`／`childFiles`）、Phase 2（`prepareCommand`）

### AC-02: 読み取り goroutine は隙が閉じた後に開始する

- 種別: `test`
- 検証: `L::TestStartPrepared_NoGoroutineInsideWindow`
- 期待: 隙の内側で採った2つの標本（`fn` の直前と `fn` の直後）の goroutine ID の集合が、
  どちらも隙を開く直前の基準集合の部分集合である（除外一覧はテスト内にリテラルで列挙する）
- 実装: Phase 4-a（`WithPrivileges` の `fn` を `startPrepared` だけにする）

### AC-03: 隙の中の操作は `chown` と `Start()` だけ

- 種別: `test`（go/ast + go/types による静的検査を `go test` から実行する）
- 検証: `G::TestPrivilegeWindowAllowedCalls`（サブテスト `start_window`、
  `rejects_unlisted_call`、`rejects_logging_in_window`）
- 期待: 起動区間から到達する追跡対象の呼び出しが Phase 4-c の許可リストに収まり、
  `Logger` へは到達しない。`testdata/` の許可リスト外呼び出しと、隙の中のログ出力は
  どちらも拒否される
- 種別: `test`（失敗の情報が握り潰されず、かつ `Logger` を経由せずに届くこと）
- 検証: `TestStageFromFD_ReportsFailuresWithoutLogging`
- 期待: 後始末が失敗したとき `cleanupFn()` が非 `nil` のエラーを返し、
  `Logger` の記録が1件も出ず、stderr に警告が1行出る
- 実装: Phase 2（`Logger.Warn` の除去、戻り値化、stderr への最後の手段の記録）、
  Phase 4-c（静的検査）

### AC-04: `WithPrivileges` の中は `chown`／`chmod` と `Start()` に限られる

- 種別: `test`
- 検証: `G::TestPrivilegeWindowAllowedCalls`（`start_window`）、
  `L::TestStartPrepared_WaitAndPumpRunOutsideWindow`
- 期待: 静的検査が通り、かつ `InWindow` の2つの phase のどちらの時点でも待機 goroutine が
  起動しておらず出力中継の `start()` も呼ばれていないこと（`Wait()`・出力の取り込み・`Result` の組み立てが
  隙の外で起きる）。隙の中にログ出力は無い（AC-03 と同じ静的検査が担う）
- 実装: Phase 2、Phase 4-a、Phase 4-b、Phase 4-c

### AC-05: 隙の長さがコマンドの実行時間に依存しない

- 種別: `test`
- 検証: `I::TestPrivilegeGap_StartWindowIndependentOfCommandDuration`
- 期待: 1秒／5秒のコマンドとも `privilege_duration_user_group_execution_us` が 5,000 未満、
  かつ両者の差が 2,000 未満
- 実装: Phase 3-b（監査メトリクスの内訳）、Phase 4-a（隙の縮小）

### AC-06: 昇格と復帰の対は1組（kill の分を除く）

- 種別: `test`
- 検証: `L::TestExecute_SingleElevationPairPerRun`（正常終了する実行で組む）
- 期待: fd-bound 実行で特権管理器の呼び出しが起動区間の1件のみ。staging フォールバックで
  `user_group_execution` と `staging_cleanup` の2件、後者が `pc.privilegeWindows` に記録される。
  この成功経路では昇格と復帰の対は `WithPrivileges` の呼び出しと1対1で、`ElevationCount` は
  その回数と一致する。
  監査メトリクスの `ElevationCount`／`ByOperation` を直接主張しないのは、特権の無いテストでは
  run-as 実行を最後まで走らせられないためである（Phase 4-b の該当ステップ参照）
- 種別: `test`（復帰直後の識別子検査が変わらないこと）
- 検証: 既存 `internal/runner/base/executor/executor_privilege_check_test.go::TestExecute_PrivilegeLeakCausesExit`、
  同 `::TestExecute_NoPrivilegeLeakDoesNotCallExit`
- 期待: 既存の主張が変更なしで緑（`identityChecker` の位置と意味を変えていないこと）

### AC-07: タイムアウトで子プロセスが停止する

- 種別: `test`
- 検証: `I::TestPrivilegeGap_TimeoutKillsChild`、`S::TestSupervise_TimeoutJoinsContextAndWaitErrors`、
  `internal/runner/group_executor_timeout_test.go::TestExecuteSingleCommand_TimeoutLogsTimeoutExceeded`
- 期待: `Execute` がタイムアウト直後に戻り、戻り値から `context.DeadlineExceeded` と
  `Wait()` のエラーの両方を `errors.Is` でたどれる。実 executor を通した実行で
  `LogTimeoutExceeded` の記録が現れる（設計文書 §7.3 が AC-07 に課す2点目）
- 実装: Phase 3-d（エラー合成）

### AC-08: SIGINT／SIGTERM で子プロセスが停止する

- 種別: `test`
- 検証: `I::TestPrivilegeGap_CancelKillsChild`、`S::TestSupervise_CancelKillsChild`
- 期待: キャンセル後、`Execute` が `killGraceDelay` 以内に戻り `context.Canceled` をたどれる
- 実装: Phase 3-d

### AC-09: kill の再昇格は kill だけを含み、直後に復帰と検査を行う

- 種別: `test`
- 検証: `S::TestSupervise_KillOpensExactlyOneReelevation`、
  `S::TestKillChild_RecordsOnlyWindowsThatOpened`（監査メトリクスへ届くのは実際に開いた隙だけ）、
  `S::TestKillChild_RejectsUndeclaredAndUnavailableStrategies`（`killUnset` と
  特権マネージャ不在の fail-secure 側）、
  `G::TestPrivilegeWindowAllowedCalls`（サブテスト `kill_window`）
- 期待: `ElevationCalls` に `kill_after_cancel` がちょうど1回現れ、
  kill 区間の `MockWindowPhaseBeforeFn` の時点で
  子プロセスがまだ生きている。静的検査で kill 区間から到達する追跡対象の呼び出しが
  `(*os.Process).Kill` だけである
- 実装: Phase 3-d、Phase 4-c

### AC-10: 通常実行では kill の再昇格を行わない

- 種別: `test`
- 検証: `S::TestSupervise_NormalExecutionDoesNotReelevate`
- 期待: `run_as` を伴わない実行のキャンセルで `MockPrivilegeManager.ElevationCalls` が空
- 実装: Phase 3-d（`killStrategy` による宣言）

### AC-11: `WithPrivileges` に2つの goroutine が同時に入らない

- 種別: `test`
- 検証: `S::TestSupervise_KillRunsOnExecutingGoroutine`
- 期待: 起動区間と kill 区間の `MockWindowPhaseBeforeFn` で採った goroutine ID が一致し、
  `privilege.ErrReentrantPrivilegeCall` が返らない（モック側の再入ガードは Phase 3-c で入れる。
  ガードが無いとこの主張は無条件に通る）
- 実装: Phase 3-c、Phase 3-d

### AC-12: 終了後・キャンセル済みの場合に特権と記述子を漏らさない

- 種別: `test`（記述子と特権の両方）
- 検証（記述子）: `F::TestExecute_NoLeakOnCancellationPaths`（キャンセル済み context／
  実行中のキャンセル＝ kill 経路）、既存 `F::TestExecute_FdBoundStartFailureNoLeak`（`Start()` 失敗）、
  既存 `E::TestExecute_ContextCancellation`（キャンセル済み context で `Result` が非 `nil`）
- 期待: 3経路とも反復で記述子が増えない（`numOpenFDs` の差が 1 以下）
- 検証（特権）: `I::TestPrivilegeGap_TimeoutKillsChild`、`I::TestPrivilegeGap_CancelKillsChild`
- 期待: `Execute` の前後で `os.Geteuid()` が変わらない。**非特権で走るテストへは置かない**:
  そこでは実効 UID と実 UID が executor の挙動によらず等しく、主張が理由をもって落ちられない
- 実装: Phase 2（`release()`）、Phase 3-d（キャンセル済み context の早期リターン）

### AC-13: 上限超過を実行中に検出し、子を打ち切る

- 種別: `test`（非特権。`make test` で常時実行される）
- 検証: `E::TestExecute_OutputLimitAbortsRunningChild`
- 期待: 10 秒間出力を出し続けるコマンドが 2 秒以内に打ち切られ、返るエラーから
  書き込みエラーのセンチネルエラーを `errors.Is` でたどれる
- 種別: `test`（run-as 版。特権環境でのみ実行）
- 検証: `I::TestPrivilegeGap_OutputLimitAbortsRunningChild`
- 期待: `output.Capture` の上限を超え続ける `run_as` コマンドが、終了を待たずに打ち切られる
- 実装: Phase 1（読み取り goroutine が書き込みエラー時に読み取り側を閉じる）

### AC-14: 上限超過エラーが破断エラーより優先して報告される

- 種別: `test`
- 検証: `S::TestSupervise_SizeLimitErrorOutranksExitError`、
  `P::TestOutputPump_WriteErrorPrefersStdout`、
  `S::TestSupervise_KillFailureIsJoinedWithWriteError`
- 期待: 子が `SIGPIPE` で異常終了しても、返るエラーの主因は書き込みエラーであり
  `*exec.ExitError` は主因として現れない。stdout 側の書き込みエラーが stderr 側より優先される。
  kill 失敗が同時に起きたときは `ErrKillAfterCancel` も併記され、両方を `errors.Is` でたどれる
- 実装: Phase 3-d（エラーの優先順位）

### AC-15: stdout／stderr の区別と `OutputWriter` への経路が変わらない

- 種別: `test`
- 検証: `P::TestOutputPump_SeparatesStreams`、既存 `W::TestOutputWrapper_SeparatesStdoutAndStderr`
- 期待: `streamRecorder` の記録がストリームごとに正しく、両ストリームが同一の
  `OutputWriter` を共有したままである
- 実装: Phase 1

### AC-16: 終了コード・出力・エラー種別が現在と一致する

- 種別: `test`
- 検証: 既存 `E::TestExecute_Success`、`E::TestExecute_Failure`、
  `E::TestDefaultExecutor_UserGroupPrivileges_StderrCapture`、`E::TestExecute_ContextCancellation`
  （いずれも変更なしで緑）、新規 `E::TestExecute_NilOutputWriter_StderrPrefixSuffixBound`、
  `E::TestExecute_NilOutputWriter_LargeStderrStillSucceeds`
- 期待: 既存4件が無変更で通る。`outputWriter` が `nil` の経路で前後 32 KiB の保持と
  省略表示が `Cmd.Output()` と一致し、大量の標準エラー出力を出して正常終了するコマンドが
  成功したままである
- 実装: Phase 1（`boundedBuffer`）

### AC-17: fd-bound と staging フォールバックの双方で成立する

- 種別: `test`
- 検証: `L::TestExecute_ShebangScriptRunsUnderStagingFallback`、既存 `F::TestExecute_FdBoundOrStaging`
- 期待: `WithFdExecDisabled` の下でシェバンつきスクリプトが実行でき標準出力が一致する。
  fd-bound／staging 両経路で終了コードと出力が一致する
- 実装: Phase 4-a（staged copy の削除を子の終了後へ移す）

### AC-18: dry-run の出力が変わらない

- 種別: `test`
- 検証: 既存 `internal/runner/resource/dryrun_manager_test.go::TestDryRunResourceManager_ExecuteCommand`、
  `internal/runner/resource/integration_test.go::TestDryRunExecutionPath`（いずれも変更なしで緑）
- 期待: dry-run の `Result` が `[DRY-RUN]` を含み、子プロセスを起こさない
- 種別: `static`（dry-run が executor へ到達しないこと）
- 検証:
  ```sh
  rg -n -e 'executor\.Execute\(' -e '\.executor\.Execute' internal/runner/resource/dryrun_manager.go
  ```
- 期待: マッチ 0 件（`DefaultExecutor.Execute` を呼ばないため、本タスクの変更が dry-run に波及しない）

### AC-19: `security-architecture.md` の残存リスクが更新されている

旧文言の不在（4ファイル、それぞれ現行の実テキストから採ったリテラルを使う。
いずれも変更前は 1 件を返すことを確認済み）:

- 種別: `static`
- 検証:
  ```sh
  rg -F -c '特権の隙が開いている間、参加しない goroutine は保護されない' docs/dev/architecture_design/security-architecture.ja.md
  rg -F -c 'その実効 UID で走るため保護されない' docs/user/security-risk-assessment.ja.md
  rg -F -c 'goroutines that do not participate in it are not protected' docs/dev/architecture_design/security-architecture.md
  rg -F -c 'also run with that effective UID and are therefore not protected' docs/user/security-risk-assessment.md
  ```
- 期待: 4本とも 0 件（`rg -c` はマッチが無いと終了コード 1 を返し何も出力しない）

新文言の存在（日本語版2ファイル。ground truth は Phase 6-b の変更後ブロック）:

- 種別: `static`
- 検証:
  ```sh
  rg -F -c '起動区間' docs/dev/architecture_design/security-architecture.ja.md
  rg -F -c 'Slack 送信ワーカー' docs/dev/architecture_design/security-architecture.ja.md
  rg -F -c '起動区間' docs/user/security-risk-assessment.ja.md
  rg -F -c 'コマンドの実行時間には比例しない' docs/user/security-risk-assessment.ja.md
  ```
- 期待: 4本とも 1 件以上
- 種別: `manual`（英語版が日本語版の内容に対応していること）
- 検証: `/mktrans` 実行後、`security-architecture.md` と `security-risk-assessment.md` の
  該当箇所を日本語版と並べて読み、段落の対応と内容の一致を確認する
- 補足: 上の `static` 4本＋4本が主たる証拠であり、この `manual` はそれを補完する

### AC-20: `WithPrivileges` の doc コメントが実態に合っている

- 種別: `static`（旧文言の不在）
- 検証:
  ```sh
  rg -F -c "including os/exec's copy goroutines for" internal/runner/base/privilege/unix.go
  ```
- 期待: 0 件
- 種別: `static`（残る未解決の課題が書かれていること）
- 検証: 語ごとに独立して数える。Phase 6-a の変更後ブロックで各語が1行に収まることを
  確認済みである（`rg` は行単位で照合するため、行をまたぐ語は一致しない）。
  ```sh
  rg -F -c 'Slack send worker' internal/runner/base/privilege/unix.go
  rg -F -c 'fork/execve window inside Start' internal/runner/base/privilege/unix.go
  rg -F -c 'kill and staging-cleanup windows' internal/runner/base/privilege/unix.go
  rg -F -c 'moving privileged' internal/runner/base/privilege/unix.go
  ```
- 期待: 4本とも 1 件

### AC-21: 特権を要する統合テストがあり、条件が揃わない環境ではスキップする

- 種別: `static`（`integration` タグ付きファイルが `test` タグと併せてコンパイルできる）
- 検証:
  ```sh
  go test -run '^$' -tags "test integration" ./internal/runner/base/executor/
  ```
- 期待: 終了コード 0（`//go:build integration` のファイルは `make lint` の対象外なので、
  型・シグネチャの誤りはこのコンパイルでしか捕まらない）
- 種別: `test`（スキップ判定が既存と同じ形で理由を示す）
- 検証: `C::TestCanRunSetuidModelIntegrationTest`、`C::TestRequireSetuidModel_ReadsDocumentedEnvVar`
- 期待: 表の各行で `ok` と理由が期待どおり。ラッパーが `TEST_RUNAS_TARGET_USER` を読み、
  その値を含む理由でスキップする
- 種別: `static`（4本すべてがスキップ判定を通る）
- 検証:
  ```sh
  rg -F -c 'requireSetuidModel(t)' internal/runner/base/executor/executor_privilege_gap_integration_test.go
  ```
- 期待: 4 件（AC-05／AC-07／AC-08／AC-13 の各テストが1件ずつ呼ぶ）

### AC-22: pre-commit と `make` から統合テストを実行できる

- 種別: `static`（ターゲットが定義され、正しいタグと環境変数の転送を含む）
- 検証:
  ```sh
  make -n executor-privileged-integration-test
  ```
- 期待: 出力に `-tags "test integration"` と `TEST_RUNAS_TARGET_USER=` の両方が現れ、終了コード 0
- 種別: `static`（CI 合成ターゲットから走る）
- 検証:
  ```sh
  rg -n '^test-ci:.*executor-privileged-integration-test' Makefile
  rg -n '^test-ci-cgo1:.*executor-privileged-integration-test' Makefile
  ```
- 期待: 2本ともマッチ 1 件
- 種別: `static`（pre-commit フックが定義され、設定として妥当である）
- 検証:
  ```sh
  rg -F -c 'make executor-privileged-integration-test' .pre-commit-config.yaml
  pre-commit validate-config .pre-commit-config.yaml
  ```
- 期待: 前者 1 件、後者 終了コード 0（`name` の欠落など必須フィールドの漏れを検出する）
- 種別: `test`（ターゲットが実際に走り、スキップして緑になる）
- 検証:
  ```sh
  make executor-privileged-integration-test
  ```
- 期待: 非特権環境で終了コード 0。出力に `TestPrivilegeGap_` のスキップ理由が現れる
  （ターゲットが実際にテストを起動していることの証拠。`make -n` は印字するだけで実行しない）

### AC-23: すべての受け入れ基準が追跡表から辿れる

- 種別: `static`
- 検証: AC-01 から AC-23 まで、本節に見出しがあり、かつ `test` または `static` の
  検証を少なくとも1つ持つことを確かめる。
  ```sh
  plan=docs/tasks/0171_privilege_gap_narrowing/03_implementation_plan.md
  awk '/^### AC-/{ac=$2; has[ac]=0}
       /^- 種別: `(test|static)`/{if(ac!="") has[ac]=1}
       END{for(i=1;i<=23;i++){k=sprintf("AC-%02d:",i); if(!(k in has)) print "missing " k; else if(has[k]==0) print "no test/static for " k}}' "$plan"
  ```
- 期待: 出力が空

---

## 8. 横断検索チェックリスト

`make lint` と `make test` では検出できない残存参照と文言の整合だけを挙げる。
§7 の検証と重複する `rg` は挙げない。検索範囲は `internal/`／`cmd/`／`test/`／`Makefile` に絞る
（`docs/tasks/` の過去タスクの記録は歴史的事実なので書き換えない。ただし 0170 の追跡表だけは
Phase 6-c で注記を足す）。

- [ ] 削除した `executeCommandWithPath` の残存参照:
      ```sh
      rg -n 'executeCommandWithPath' internal/ cmd/ test/ Makefile
      ```
      期待: マッチ 0 件（コメント・テスト名を含む）
- [ ] 削除した `prepareExecCommand` の残存参照:
      ```sh
      rg -n 'prepareExecCommand' internal/ cmd/ test/ Makefile
      ```
      期待: マッチ 0 件（`test/security/output_security_test.go:181,252` のコメントを含む）
- [ ] `exec.CommandContext` を使い切っていないこと:
      ```sh
      rg -n 'exec\.CommandContext' internal/runner/base/executor/
      ```
      期待: マッチ 0 件
- [ ] 旧文言 `output copy goroutine` の一掃:
      ```sh
      rg -n -F 'output copy goroutine' internal/ cmd/
      ```
      期待: マッチ 0 件（`census_guard_test.go` の理由文字列を含む）
- [ ] 置換後の文言が実際に入っていること:
      ```sh
      rg -F -c "output-pump reader goroutine" internal/logging/log_line_tracker.go internal/redaction/error_collector.go internal/runner/base/output/capture.go internal/testutil/synccensus/census_guard_test.go
      ```
      期待: 各ファイル 1 件以上
- [ ] 0170 の追跡表の陳腐化対応が入っていること:
      ```sh
      rg -n -F '0171' docs/tasks/0170_excess_synchronization_removal/03_implementation_plan.md
      ```
      期待: 5 件以上（Phase 6-c の5箇所）
- [ ] 新規の型名が executor パッケージの外へ漏れておらず、他パッケージに同名の宣言も無いこと:
      ```sh
      rg -n -e 'boundedBuffer' -e 'outputPump' -e 'preparedCommand' -e 'killStrategy' -e 'execBinding' --glob '*.go' internal/ cmd/ | rg -v '^internal/runner/base/executor/'
      ```
      期待: マッチ 0 件

---

## 9. 完了基準

### 機能の完成度

- [ ] 受け入れ基準 AC-01〜AC-23 のすべてについて、§7 の検証が期待どおりの結果を返す
- [ ] 設計文書 §5.6 が挙げる1点を除き、外から見える挙動が変わらない
- [ ] 実行時に旧経路へ切り替える設定項目を追加していない

### 品質

- [ ] `make fmt` → `make test` → `make lint` が緑
- [ ] `go test -run '^$' -tags "test integration" ./internal/runner/base/executor/` がコンパイルを通る
- [ ] `make deadcode` が新たな未使用シンボルを報告しない
- [ ] §4.2 のすべての項目について「仕組みを外すと落ちる」ことを確認し、コミットメッセージに記した
- [ ] 新規テストは `-race` 付き（CGO_ENABLED=1）と無し（CGO_ENABLED=0）の両方で緑

### セキュリティ

- [ ] `privileged_window_guard_test.go` の許可リストが設計文書 §7.2 の表と一致し、
      `Logger` のメソッドを1つも含まず、negative self-test が許可リスト外の呼び出しと
      隙の中のログ出力の両方を拒否する
- [ ] 3つの隙のいずれからも `Logger` へ到達しない。`stageFromFD` の2つの警告は
      戻り値と `preparedCommand.stagingWarn` で隙の外へ運ばれ、記録されている
- [ ] 隙の中の stderr への書き込みは、staging ディレクトリの削除失敗の1件だけである。
      その doc コメントに、redaction を通らないので秘匿情報を載せない旨が書かれている
- [ ] staged copy のパスが起動区間の直後に `Debug` で記録され、`emergencyShutdown` の
      経路でも `$TMPDIR` に残った複製の在処が追える
- [ ] 3つの隙がそれぞれ専用の `Operation` で開き、監査ログから区別できる
- [ ] `Execute` 末尾の識別子検査（`identityChecker`）の位置と意味を変えていない
- [ ] §4.3 の手順で作った setuid テストバイナリを、実行後に削除した

### 文書

- [ ] `security-architecture.ja.md`／`security-risk-assessment.ja.md` を更新し、
      英語版へ `/mktrans` で反映した
- [ ] 5つの doc コメント（`WithPrivileges`／`Capture`／`InMemoryErrorCollector`／
      `DefaultLogLineTracker`／`superviseCommand`）と `outputWrapper`／`stageFromFD`／
      `startPrepared` の doc コメントを更新した
- [ ] 0170 実装計画書の5箇所へ注記と置換後の検証コマンドを併記した
- [ ] §4.3（統合テストの実行手順）と §4.4（性能測定）に実測結果を追記した

---

## 10. 次のステップ

本書は `approved` である。実装は次の順で進める。

- [ ] §3.2 の PR 構成に従い、PR-1 から順に実装する。1つの PR を1つのブランチで作り、
      マージしてから次の PR のブランチへ切り替える

実装完了後に残るもの（設計文書 §9、要件定義書「本タスクの後に残るもの」）:

- kill 区間と後始末区間から読み取り goroutine と待機 goroutine を追い出す設計
- Slack 送信ワーカーを隙の外へ出す設計
- 特権操作を別プロセスへ切り出すかどうかの判断（0170 設計文書 §10.2）
- staging の行き先（`$TMPDIR`）を信頼できるディレクトリへ固定するかどうかの判断（設計文書 §5.4）
- `//go:build integration` のファイルを lint 対象に含めるかどうかの判断
  （現状は既存の統合テストも含めて lint されない）
