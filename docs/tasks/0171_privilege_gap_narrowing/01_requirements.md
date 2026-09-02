# 要件定義書: 特権の隙をコマンド実行全体から fork/exec まで縮小する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-02 |
| Review date | 2026-09-02 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#1080 特権の隙をコマンド実行全体から fork/exec まで縮小する](https://github.com/isseis/go-safe-cmd-runner/issues/1080)
- 先行タスク: [0170 単一スレッド前提に照らして過剰な排他制御を棚卸しし、削除する](../0170_excess_synchronization_removal/01_requirements.md)（[#1074](https://github.com/isseis/go-safe-cmd-runner/issues/1074)）。`UnixPrivilegeManager.mu` を削除し、代わりに同期を伴わない再入ガードを置いた。本タスクはその前提を壊さないことを要件に含む
- 参照: [0170 の設計文書](../0170_excess_synchronization_removal/02_architecture.md) §6.2（脅威モデル）、§10.2（特権操作の設計をやり直すとき）

## 背景

`run_as_user`／`run_as_group` を伴うコマンド実行では、**特権の隙（プロセスの実効 UID が 0 である区間）がコマンドの実行時間そのものに等しい**。

[`executor.go:211`](../../../internal/runner/base/executor/executor.go#L211) は `WithPrivileges` に渡す `fn()` の中で `executeCommandWithPath` を呼び、その中の [`execCmd.Run()`](../../../internal/runner/base/executor/executor.go#L340) と [`execCmd.Output()`](../../../internal/runner/base/executor/executor.go#L358) が隙の内側にある。`Run()` は `Start()` と `Wait()` の組であり、`Wait()` はコマンドが終わるまで戻らない。したがって隙は数分から数時間に及びうる。

root が実際に要るのは次の2つだけである。

1. [`stageFromFD`](../../../internal/runner/base/executor/executor.go#L508) の `os.Chown`。検証済み記述子をそのまま exec できない環境で使う staging フォールバックの経路にだけ現れる
2. `execCmd.Start()`。`SysProcAttr.Credential` をカーネルが `execve` の時点で適用するため

`Wait()`、環境変数の組み立て、`os.Open(os.DevNull)`、記述子の複製はいずれも root を要しない。

### 隙の中を走る非参加 goroutine

[`executor.go:333-337`](../../../internal/runner/base/executor/executor.go#L333-L337) は `*os.File` ではない `io.Writer`（`outputWrapper`）を `Stdout`／`Stderr` に渡す。このとき `os/exec` は writer ごとに出力コピー goroutine を1つ起こす。`outputWriter` が `nil` の分岐（`execCmd.Output()`）も同様である。これらは `WithPrivileges` の参加者ではないため、特権マネージャは原理的に保護できない。

本タスクの調査で、**コマンド実行中に走る非参加 goroutine の発生源は2つだけである**ことを確認した。1つは上記の出力コピー goroutine（stdout と stderr で2本）であり、もう1つは [`slack_sender.go`](../../../internal/logging/slack_sender.go) の Slack 送信ワーカーである。production コードの `go` 文はこの送信ワーカーと [`bootstrap/logger.go`](../../../internal/runner/bootstrap/logger.go#L461) の Slack flush の2箇所しかなく、後者は終了処理の中でのみ走るためコマンド実行とは重ならない。

本タスクが取り除けるのは出力コピー goroutine だけである。Slack 送信ワーカーはログ機構の一部であり、隙の外へ追い出すには別の設計が要るため対象外とする（後述の「本タスクの後に残るもの」を参照）。

現時点で被害が出ていないのは、これらの goroutine が既に開いている記述子への書き込みと slog 呼び出ししか行わないためである。これは `Capture` とハンドラ群の現在の実装がたまたまそうであるという事実にすぎず、設計上の不変条件ではない。ハンドラの1つがファイルを開くようになれば、その `open` は気づかれることなく root 権限で行われる。

### 縮小を阻む2つの障害

Issue #1080 は、`WithPrivileges` で囲む範囲を `Start()` までに狭める案（`WithPrivileges` の内側にあった `Wait()` を外側へ出す、入れ子構造の反転）に対して2つの障害を挙げている。

**障害1: タイムアウト時の kill が EPERM になる。** [`prepareExecCommand`](../../../internal/runner/base/executor/executor.go#L421) は `exec.CommandContext` を使っており、その watchdog は context のキャンセル時に `Process.Kill()` を呼ぶ。シグナル送信の権限は、送信側の実 UID または実効 UID が対象の実 UID または保存 set-user-ID と一致することを要求する。setuid バイナリのモデル（実 UID = 起動者、実効 UID = root、子プロセスは `run_as_user`）では、`Wait()` の前に実効 UID を実 UID へ落とすとこの `Kill()` が EPERM で失敗し、タイムアウトと Ctrl-C が子プロセスを停止できなくなる。

**障害2: 特権マネージャの直列化要件が復活する。** `Cmd.Cancel` は watchdog goroutine の上で走る。そこから `WithPrivileges` を呼ぶと2つの goroutine が `WithPrivileges` に入りうることになり、タスク 0170 が「単一 goroutine 前提」として置いた再入ガード（[`unix.go`](../../../internal/runner/base/privilege/unix.go) の `inPrivilegedWindow`、同期を伴わない）の前提が崩れる。

本タスクは、**`exec.CommandContext` を使わず、キャンセルの待機と `Kill()` をコマンドを起動したのと同じ goroutine の上で行う**ことでこの2つを同時に解決する。watchdog goroutine が無くなるので特権操作は単一 goroutine に留まり、障害2 は発生しない。kill のための再昇格は障害1 を解決する。

## 目的

1. **特権の隙の中から、コマンド実行が起こす非参加 goroutine を無くす。** 子プロセスの `Stdout`／`Stderr` に `*os.File` を渡し、`os/exec` に出力コピー goroutine を起こさせない。出力の取り込みは自前で行い、隙が閉じた後に開始する。隙の中に残るのは実行 goroutine と Slack 送信ワーカーだけになる。
2. **隙の長さをコマンドの実行時間から `chown` と `fork`/`execve` へ縮める。** `WithPrivileges` で囲む範囲を `Start()` までとし、`Wait()` を外へ出す。
3. **縮小によって失われかねない2つの性質を維持する。** タイムアウトと Ctrl-C による子プロセスの停止、および出力サイズ上限のストリーミング判定（上限を超えた時点で子を打ち切る挙動）。
4. **特権操作が単一 goroutine の上に留まるというタスク 0170 の前提を壊さない。** 排他制御を復活させずに済ませる。

## スコープ

### 対象

1. 子プロセスの `Stdout`／`Stderr` へ `*os.File` を渡す形への変更と、出力の取り込みの自前実装。
2. 出力サイズ上限のストリーミング判定の維持。
3. `WithPrivileges` で囲む範囲を `Start()` までへ縮小する変更。
4. `exec.CommandContext` の使用をやめ、キャンセルの待機・タイムアウト・`Kill()` を呼び出し元 goroutine で行う形への変更。
5. kill のための最小限の再昇格。
6. [`security-architecture.md`](../../dev/architecture_design/security-architecture.md) のうち、特権の隙の範囲と残存リスクに関する記述の更新。
7. 特権を要する統合テストの整備と、pre-commit からの実行。

### 対象外

- **Slack 送信ワーカーの扱い。** コマンド実行中も生きている非参加 goroutine だが、ログ機構の側の構造であり、隙の外へ追い出すには通知経路そのものの設計が要る。
- **特権操作を別プロセスへ切り出す設計、およびプロセス全体を停止する設計。** 0170 の設計文書 §10.2 が挙げる2つの選択肢は、いずれも本タスクの範囲を超える。本タスクは非参加 goroutine を隙の外へ追い出すことでこの選択を不要にするが、選択そのものを行うわけではない。
- **`OperationFileValidation` の経路。** `internal/filevalidator` 側の `WithPrivileges` の使い方は変更しない。
- **実 UID が 0 の場合の改善。** root による起動や `sudo` 経由では落とすべき特権が無く、`escalatePrivileges` も短絡する。本タスクが改善するのは setuid バイナリのモデルに限られる。
- **並列実行の導入。** コマンドをグループ単位で並列化する話は本タスクでは扱わない。
- **出力サイズ上限の値そのものや、上限を超えたときの報告文言の変更。**

## 用語

| 用語 | 意味 |
|---|---|
| 特権の隙 | プロセスの実効 UID が 0 に上がっている区間。`WithPrivileges` が開いて閉じる |
| 非参加 goroutine | `WithPrivileges` の呼び出しに参加していないのに、隙が開いている間に走っている goroutine |
| fd-bound 実行 | 検証済みの記述子を `/proc/self/fd` 経由でそのまま exec する経路（Linux） |
| staging フォールバック | fd-bound 実行が使えないとき、検証済み記述子の内容を専用の複製へ写して実行する経路。`chown` を要するのはこの経路だけ |
| 実行 goroutine | コマンドの起動から結果の組み立てまでを進める goroutine。`Execute` を呼んだ goroutine と同じ |

## 機能要件

### F-001: 特権の隙の中で非参加 goroutine を作らない

子プロセスの `Stdout`／`Stderr` に `*os.File` を渡すことで、`os/exec` が writer ごとに起こす出力コピー goroutine を無くす。出力の読み取りは自前で行い、特権の隙が閉じてから開始する。

**受け入れ基準**:

- **AC-01**: `run_as_user`／`run_as_group` を伴う実行において、`exec.Cmd` の `Stdout` と `Stderr` に渡される値がいずれも `*os.File` である。`outputWriter` が `nil` の分岐（従来 `execCmd.Output()` を使っていた経路）でも同じである。
- **AC-02**: 子プロセスの出力を読み取る goroutine は、特権の隙が閉じた後にのみ開始される。すなわち `WithPrivileges` に渡す関数が戻るまでの間、コマンド実行の経路が起こした goroutine は実行 goroutine のほかに存在しない（`os/exec` が内部で起こすものを含む）。
- **AC-03**: 実効 UID が 0 である区間の中でコマンド実行の経路が行う操作のうち、ファイルを開くものと `exec` するものは、`chown`（staging フォールバック時のみ）と `Start()` だけである。

### F-002: 特権の隙を chown と fork/execve に限定する

`WithPrivileges` で囲む範囲を `Start()` までに狭め、`Wait()` と出力の取り込みを隙の外へ出す。

**受け入れ基準**:

- **AC-04**: `WithPrivileges` に渡す関数の中で行われるのは、staging フォールバック時の `chown`／`chmod` と `execCmd.Start()` だけである。`Wait()`、出力の取り込み、結果の組み立ては隙の外で行われる。
- **AC-05**: 特権の隙の長さがコマンドの実行時間に依存しない。実行時間の異なる2つのコマンドについて、記録される昇格時間（`ElevationMetrics` の値）に有意な差が出ない。
- **AC-06**: 1回のコマンド実行につき、昇格と復帰の対は1組である（F-003 の kill 経路で追加される1組を除く）。復帰の直後に実効 UID と実 UID の一致を検査する既存の不変条件は変わらない。

### F-003: キャンセルとタイムアウトで子プロセスを確実に停止する

`exec.CommandContext` の使用をやめ、キャンセルの待機と `Kill()` を実行 goroutine の上で行う。`run_as` 実行では kill の直前に最小限の再昇格を行う。

**受け入れ基準**:

- **AC-07**: setuid モデル（実 UID が 0 以外、実効 UID が 0）で `run_as_user` を伴う長時間コマンドを実行し、コマンドのタイムアウトに達したとき、子プロセスが停止し、タイムアウトとして報告される。
- **AC-08**: 同じ条件で SIGINT／SIGTERM によって context がキャンセルされたとき、子プロセスが停止する。
- **AC-09**: kill のための再昇格は kill の実行だけを含み、その直後に実効 UID を復帰させ、実効 UID と実 UID の一致を検査する。
- **AC-10**: `run_as` を伴わない通常実行（カーネルへ渡す資格情報が無い場合）では、kill のための再昇格を行わない。
- **AC-11**: kill 経路は実行 goroutine の上で走り、`WithPrivileges` に2つの goroutine が同時に入ることが無い。再入ガードが発火しない。
- **AC-12**: 子プロセスが終了した後に context がキャンセルされた場合、および context が既にキャンセルされた状態で実行を開始した場合に、実効 UID を上げたまま戻る経路も、記述子を漏らす経路も無い。

### F-004: 出力サイズ上限のストリーミング判定を維持する

子プロセスへ実ファイルを渡す形にしても、上限超過を実行中に検出して子を打ち切る現在の性質を失わない。

**受け入れ基準**:

- **AC-13**: 出力サイズの上限を超える出力を出し続けるコマンドについて、コマンドの終了を待たずに上限超過が検出され、子プロセスが打ち切られる。
- **AC-14**: 上限超過が起きたとき、報告されるエラーは上限超過を表すものであり、子プロセスがパイプの破断で終了したことを表すエラーではない。
- **AC-15**: stdout と stderr の区別、および `OutputWriter` への書き込みが現在と同じ内容・同じ経路で行われる。両ストリームが同一の `OutputWriter` を共有するという現在の前提は変わらない。

### F-005: 外部から観測できる挙動の互換性

セキュリティ姿勢とキャンセル経路の内部構造は変わるが、利用者から見た結果は変わらない。

**受け入れ基準**:

- **AC-16**: 終了コード、標準出力の内容、標準エラー出力の内容、返されるエラーの種別が現在と一致する。
- **AC-17**: fd-bound 実行の経路と staging フォールバックの経路の双方で AC-16 が成立する。
- **AC-18**: dry-run の出力が変わらない。

### F-006: 文書の更新

**受け入れ基準**:

- **AC-19**: [`security-architecture.md`](../../dev/architecture_design/security-architecture.md) のうち、特権の隙の範囲および脅威モデルの残存リスクに関する記述が、本タスク後の実態に合わせて更新されている。とくにタスク 0170 が残存リスクとして記した「特権の隙が開いている間、参加しない goroutine は保護されない」の項が、本タスクの結果を反映した記述に置き換わっている。
- **AC-20**: `WithPrivileges` の doc コメントのうち、出力コピー goroutine が実効 UID 0 で走るという記述が実態に合わせて更新されている。残る未解決の課題（`Start()` 中の露出、および別プロセス化の是非）は、残っていることが分かる形で書かれている。

### F-007: 検証環境の整備

setuid モデルの実挙動は通常のユニットテストでは再現できない。特権を要する統合テストを整備し、pre-commit から実行する。

**受け入れ基準**:

- **AC-21**: 特権を要する検証（AC-07、AC-08、AC-13）が `integration` ビルドタグ付きの統合テストとして存在し、特権が無い環境や対象ユーザーが存在しない環境では、既存の二段階スキップ判定（[`privileged_test_condition_test.go`](../../../internal/runner/base/executor/privileged_test_condition_test.go)）と同じ形で理由を示してスキップする。
- **AC-22**: pre-commit の実行に `integration` タグ付きテストの実行が含まれ、`make` の合成ターゲットからも同じ実行ができる。
- **AC-23**: すべての受け入れ基準について、対応する検証（テストまたは静的な確認）が `03_implementation_plan.md` の追跡表から辿れる。

## 非機能要件

- **性能**: 本タスクは性能を目的としない。パイプの読み取りを自前で行う変更が、コマンド1回あたりの実時間に測れるほどの差を生まないことだけを確認する。差が出た場合は、`fork`/`exec` に要する時間（数十マイクロ秒）と比較した絶対値で判断する。
- **移植性**: fd-bound 実行が使えない環境（非 Linux、およびテストで無効化した場合）でも、staging フォールバックの経路が同じ受け入れ基準を満たす。
- **可読性**: `exec.CommandContext` を自前の待機に置き換えることで失われる意味論（キャンセル済み context での起動、`WaitDelay` を設定していないことに依存する現在の前提）を、コード上で追える形で残す。

## リスク

| リスク | 影響 | 緩和 |
|---|---|---|
| 自前のパイプ管理による記述子の漏洩 | 長時間動作するプロセスで記述子を使い切る | 生成から解放までを1箇所に閉じ、`Start()` が失敗した経路を含めて解放されることをテストで確認する |
| 読み取り側の閉じ忘れによるデッドロック | `Wait()` が戻らず、コマンドがハングする | 親側の書き込み端を `Start()` の直後に閉じることを不変条件とし、実行時間の長いコマンドと大量出力のコマンドの両方でテストする |
| `exec.CommandContext` の意味論の取りこぼし | キャンセルが効かない、または二重に kill する | 置き換え前に現在の意味論（キャンセル済み context、`WaitDelay` が 0 であること、`Wait` が出力コピーの完了を待つこと）を洗い出し、それぞれに対応する検証を置く |
| 再昇格の追加による復帰漏れ | 実効 UID 0 のまま処理が続く | kill 経路も既存の `WithPrivileges` を通し、復帰と識別子の検査を共通の経路で行う |
| 特権が要るテストが開発環境で常にスキップされる | 受け入れ基準が実質的に検証されない | スキップ時に理由を出力する既存の方式を踏襲し、実際に検証した環境と手順を実装計画に記録する |

## 受け入れ基準一覧

| ID | 要約 | 対応する要件 |
|---|---|---|
| AC-01 | `Stdout`／`Stderr` に渡すのは `*os.File` である | F-001 |
| AC-02 | 出力の読み取り goroutine は隙が閉じた後に開始する | F-001 |
| AC-03 | 隙の中でコマンド実行の経路が行う操作は `chown` と `Start()` だけである | F-001 |
| AC-04 | `WithPrivileges` の中は `chown`／`chmod` と `Start()` に限られる | F-002 |
| AC-05 | 隙の長さがコマンドの実行時間に依存しない | F-002 |
| AC-06 | 昇格と復帰の対は1組（kill の分を除く） | F-002 |
| AC-07 | タイムアウトで子プロセスが停止する | F-003 |
| AC-08 | SIGINT／SIGTERM で子プロセスが停止する | F-003 |
| AC-09 | kill の再昇格は kill だけを含み、直後に復帰と検査を行う | F-003 |
| AC-10 | 通常実行では kill の再昇格を行わない | F-003 |
| AC-11 | `WithPrivileges` に2つの goroutine が同時に入らない | F-003 |
| AC-12 | 終了後・キャンセル済みの場合に特権と記述子を漏らさない | F-003 |
| AC-13 | 上限超過を実行中に検出し、子を打ち切る | F-004 |
| AC-14 | 上限超過エラーが破断エラーより優先して報告される | F-004 |
| AC-15 | stdout／stderr の区別と `OutputWriter` への経路が変わらない | F-004 |
| AC-16 | 終了コード・出力・エラー種別が現在と一致する | F-005 |
| AC-17 | fd-bound と staging フォールバックの双方で成立する | F-005 |
| AC-18 | dry-run の出力が変わらない | F-005 |
| AC-19 | `security-architecture.md` の残存リスクが更新されている | F-006 |
| AC-20 | `WithPrivileges` の doc コメントが実態に合っている | F-006 |
| AC-21 | 特権を要する統合テストがあり、条件が揃わない環境ではスキップする | F-007 |
| AC-22 | pre-commit と `make` から統合テストを実行できる | F-007 |
| AC-23 | すべての受け入れ基準が追跡表から辿れる | F-007 |

## 本タスクの後に残るもの

- **Slack 送信ワーカー。** 隙の中を走る非参加 goroutine のうち、本タスクが取り除かない唯一のものである。現在は既に開いている記述子への書き込みと HTTP 送信しか行わないが、これは実装がたまたまそうであるという事実であって不変条件ではない。
- **`Start()` 実行中の露出。** `Start()` の内側でも、`fork` と `execve` の間に親のプロセスは実効 UID 0 である。本タスクは非参加 goroutine をこの区間から追い出すが、区間そのものは消えない。
- **特権操作を別プロセスへ切り出すかどうかの判断。** 0170 の設計文書 §10.2 が挙げる選択は未決のままである。本タスクはその判断の必要性を下げるが、判断を代替しない。
- **実 UID が 0 で起動された場合。** 落とすべき特権が無いため、本タスクの効果は及ばない。
