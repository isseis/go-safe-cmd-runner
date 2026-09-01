# 要件定義書: 単一スレッド前提に照らして過剰な排他制御を棚卸しし、削除する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-09-01 |
| Review date | 2026-09-01 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue

- [#1074 単一スレッド前提に照らして過剰な排他制御を棚卸しし、削除する](https://github.com/isseis/go-safe-cmd-runner/issues/1074)
- 先行タスク: [0169 CGO ビルドの列挙完全性](../0169_groupmembership_cgo_enumeration_completeness/01_requirements.md)（Phase 4 で `nsswitch.go` の完全性判定 latch を同じ理由で削除済み。本タスクはその続きにあたる）
- 関連: [#1071](https://github.com/isseis/go-safe-cmd-runner/issues/1071)（`internal/runner/base/security` の誤検知。本タスクとは無関係）

## 背景

本プロジェクトは基本的に単一スレッド実行である。にもかかわらず production コードには
`sync.Mutex`／`sync.RWMutex`／`atomic.*` が広く置かれており、その多くは実行時に何も守っ
ていない。守る対象の無いロックは、読み手に「ここは並行に触られる」という誤った前提を与え、
本当に並行な箇所（Slack 送信ワーカーなど）との区別を失わせる。YAGNI に基づいて削除する。

将来 runner のコマンド実行をグループ単位で並列化する可能性はあり、そのための排他制御は
許容する。しかし「いつか並列化するかもしれない」を根拠に現在到達しないロックを残すことは、
本タスクでは採らない。並列化を実施する時点で、守るべき対象を特定したうえで置き直す。

### 並行性の発生源は3つあり、3つ目が見落としやすい

自前の `go` 文は production コードに2箇所しかない。

1. [`internal/logging/slack_sender.go:257`](../../../internal/logging/slack_sender.go#L257) — `go sd.run()`（Slack 送信ワーカー）
2. [`internal/runner/bootstrap/logger.go:457`](../../../internal/runner/bootstrap/logger.go#L457) — `sync.WaitGroup` による Slack ハンドラの並列 flush

3つ目は `os/exec` が起こす出力コピー goroutine である。
[`executor.go:333-337`](../../../internal/runner/base/executor/executor.go#L333-L337) は
`execCmd.Stdout`／`execCmd.Stderr` に `*os.File` ではない `io.Writer`（`outputWrapper`）を
渡すため、`os/exec` は writer ごとに goroutine を1つ起こす。さらに
[`output/capture.go:44`](../../../internal/runner/base/output/capture.go#L44) は書き込み経路の
中で `c.Logger.Error(...)` を呼ぶため、**slog ハンドラがこの goroutine 上で走りうる**。

したがって「自前の `go` が2つだけだから単一スレッド」という判断は誤りである。とくに
**slog ハンドラの実装が触る状態はすべて並行アクセスを受けうる**。以下の分類はこれを踏まえる。

### Issue 記載の調査結果に対する差分

Issue #1074 の表を現時点の HEAD に対して再確認した結果、次の差分があった。行番号は HEAD の
値であり、Issue 記載のものとは一部ずれている。

- **Issue の表に無い削除候補が1件ある。**
  [`internal/groupmembership/nsswitch.go:278`](../../../internal/groupmembership/nsswitch.go#L278)
  の `nssCompletenessReporter.reported atomic.Bool`。Issue が削除候補に挙げた
  `sudoUIDAdoptionReporter.reported`（`manager.go:473`）と同種の「起動時に1回だけ記録する」
  ガードであり、同じ理由で削除対象になる。
- **Issue の表に無い維持対象が2件ある。いずれも slog ハンドラの経路上にある。**
  - [`internal/logging/log_line_tracker.go:22`](../../../internal/logging/log_line_tracker.go#L22)
    の `lineCounter atomic.Int64`。`interactive_handler.go:155` が `GetCurrentLine()` を呼ぶ。
  - [`internal/redaction/error_collector.go:18`](../../../internal/redaction/error_collector.go#L18)
    の `mu sync.RWMutex`。`RedactingHandler` が `redactor.go:815,828` で `RecordFailure` を呼ぶ。
- **削除は対応する並行テストの削除を伴う。** Issue はこれに触れていないが、削除候補のいくつか
  には「並行に呼んでも壊れないこと」を主張するテストがある（詳細は F-004）。ロックを外したまま
  テストを残すと `-race` が正しく失敗するため、テストの扱いを AC で決めておく必要がある。

## 目的

- 単一スレッドで到達し、守る対象が存在しない排他制御を production コードから取り除く。
- 実際に並行アクセスがある箇所については、**なぜ必要かを doc コメントで明記する**。とくに
  `output/capture.go` は「`os/exec` が writer ごとに goroutine を起こす」という非自明な事実に
  依存しているため、書いておかないと次の棚卸しで誤って削除される。
- `privilege/unix.go` の `mu` については、削除と同時に「特権窓の直列化と非参加 goroutine の
  保護は未解決の設計課題である」ことを明記し、不十分なガードが与える誤った安心を取り除く。
- 外部から観測できる挙動を変えない。本タスクは純粋なリファクタリングである。

## スコープ

### 対象

1. 下表「削除候補」の10件の排他制御・アトミック変数の削除。
2. `privilege/unix.go` の `mu` の削除と、`WithPrivileges` の doc コメントへの未解決課題の明記。
3. 下表「維持対象」への根拠 doc コメントの追加。
4. 削除に伴って主張が成り立たなくなる並行テストの整理。

### 対象外

- **並列実行の実装そのもの。** 本タスクは削除だけを行う。
- **「維持対象」および「判断が要るもの」の削除。**
- **[`internal/groupmembership/membership_cgo.go:302`](../../../internal/groupmembership/membership_cgo.go#L302)
  の `pwentMutex`。** `setpwent`／`getpwent`／`endpwent` は libc のプロセス全体のカーソルであり、
  外すと将来並行に呼んだ際に**エラーではなく黙って誤った列挙結果**になる。「並列化するなら
  mutex を足す」では済まず設計そのものの検討が要るため、並列化を実際に検討する時点で扱う。
- **`internal/testutil/handlers.go` の `mu`。** テスト用ヘルパであり、並行テストが使う。
- **利用者向け文書・CHANGELOG の更新。** 外部から観測できる挙動が変わらないため不要。

### 削除候補（単一スレッドで到達し、守る対象が無い）

| # | 箇所 | 対象 | 根拠 |
|---|---|---|---|
| D1 | [`executor.go:640`](../../../internal/runner/base/executor/executor.go#L640) | `outputWrapper.mu` | `stdoutWrapper`／`stderrWrapper` は**別インスタンス**で、それぞれ `os/exec` の goroutine 1つだけが書く。同一インスタンスを2つの goroutine が触る経路が無い。`buffer`／`writeErr` の後続の読み出しは `cmd.Wait()` の後であり、`Wait` が happens-before を確立する |
| D2 | [`manager.go:90`](../../../internal/groupmembership/manager.go#L90) | `cacheMutex sync.RWMutex` | メンバーシップキャッシュ。コマンド実行は逐次で、goroutine から到達しない |
| D3 | [`manager.go:473`](../../../internal/groupmembership/manager.go#L473) | `sudoUIDAdoptionReporter.reported atomic.Bool` | SUDO_UID 採用レポータ。起動時に1回 |
| D4 | [`manager.go:504`](../../../internal/groupmembership/manager.go#L504) | `sudoUIDExistenceMemo.mu sync.Mutex` | 同上。single-flight の必要が無い |
| D5 | [`nsswitch.go:278`](../../../internal/groupmembership/nsswitch.go#L278) | `nssCompletenessReporter.reported atomic.Bool` | 列挙完全性レポータ。`precomputeEnumerationEnvironment` から起動時に1回（Issue の表に無い追加分） |
| D6 | [`policy.go:68`](../../../internal/groupmembership/policy.go#L68) | `processPermissionCheckUIDPolicy atomic.Int32` | プロセス全体の方針。起動時に設定し以後読むだけ |
| D7 | [`path_resolver.go:17`](../../../internal/verification/path_resolver.go#L17) | `mu sync.RWMutex` | パス解決キャッシュ |
| D8 | [`result_collector.go:24`](../../../internal/verification/result_collector.go#L24) | `mu sync.Mutex` | dry-run の結果収集 |
| D9 | [`normal_manager.go:42`](../../../internal/runner/resource/normal_manager.go#L42)<br>[`dryrun_manager.go:91`](../../../internal/runner/resource/dryrun_manager.go#L91) | `mu sync.RWMutex` | `tempDirs` の管理（通常版・dry-run 版） |
| D10 | [`types.go:33`](../../../internal/runner/base/risktypes/types.go#L33) | `VerifiedFD.closed atomic.Bool` | close の冪等性。単一スレッドなら平の `bool` で足りる |
| D11 | [`unix.go:36`](../../../internal/runner/base/privilege/unix.go#L36) | `UnixPrivilegeManager.mu sync.Mutex` | 後述 §「`privilege/unix.go` の `mu` について」 |

### 維持対象（実際に並行アクセスがある）

| # | 箇所 | 対象 | 理由 |
|---|---|---|---|
| K1 | [`slack_sender.go`](../../../internal/logging/slack_sender.go) | `sd.mu` ほか（`sync.Once`・`WaitGroup`・`atomic.Int64` 群） | `go sd.run()` ワーカーとの間で本当に並行 |
| K2 | [`output/capture.go:21`](../../../internal/runner/base/output/capture.go#L21) | `mutex sync.Mutex` | **stdout と stderr の2つの `outputWrapper` が同一の `OutputWriter` を共有する**（`executor.go:333-334` の `writer: outputWriter`）。`os/exec` の2つの goroutine が同じ `Capture` に書くため必須 |
| K3 | [`bootstrap/logger.go:457`](../../../internal/runner/bootstrap/logger.go#L457) | `sync.WaitGroup` | Slack flush の並列化そのもの |
| K4 | [`log_line_tracker.go:22`](../../../internal/logging/log_line_tracker.go#L22) | `lineCounter atomic.Int64` | slog ハンドラ（`interactive_handler.go`）の状態。K2 の経路上で `Capture` が `Logger.Error` を呼ぶため、出力コピー goroutine 上で走りうる（追加分） |
| K5 | [`redaction/error_collector.go:18`](../../../internal/redaction/error_collector.go#L18) | `mu sync.RWMutex` | 同上。`RedactingHandler` が失敗を記録する経路（追加分） |
| K6 | [`fdexec_linux.go:19`](../../../internal/runner/base/executor/fdexec_linux.go#L19)<br>[`runas_ident.go:29`](../../../internal/runner/base/risktypes/runas_ident.go#L29) | `sync.OnceValue` | 排他制御ではなくメモ化。とくに `OriginalExecutionIdentity` は「最初の権限変更より前に捕捉する」正しさの要請を担う。手書きの遅延初期化に置き換えると悪化する |
| K7 | [`testutil/handlers.go:33`](../../../internal/testutil/handlers.go#L33) | `mu` | テスト用ヘルパ。並行テストが使う |

## 決定事項

以下は本タスクで採る方針として確定させたい事項であり、レビューでの確認を求める。

### `privilege/unix.go` の `mu` について

Issue #1074 は当初これを「判断が要るもの」に分類したのち、**削除候補**へ移している。本タスクは
その判断を踏襲する。決め手は YAGNI ではなく、**不十分なガードが誤った安心を与えること**である。

`mu` が達成するのは `WithPrivileges` 同士の相互排他だけである。これは必要条件ではある（無ければ、
A の `handleCleanup` が B の `fn()` の足元で euid を落とし、B が特権を持たないまま走ってしまう、
権限の意図しない喪失（silent privilege loss）が起きる）。しかし次の3点で、並列実行の安全機構としては成立しない。

1. **非参加者を守れない。** ロックを取るのは `WithPrivileges` の呼び出し側だけである。特権窓が
   開いている間、並行して走る無関係な処理はすべて root として実行される。本コードベースには
   実際にそのような goroutine があり、それが `os/exec` の出力コピー goroutine である。euid は
   プロセス全体であり、`mu` はこれを原理的に防げない。
2. **粒度が守る対象と合っていない。** `mu` はインスタンス単位、euid はプロセス単位である。
   [`runner.go:181`](../../../internal/runner/runner.go#L181) に第2インスタンスを作る経路がある
   （現行は [`cmd/runner/main.go:539`](../../../cmd/runner/main.go#L539) が注入するため発火しない）。
   インスタンスが2つできれば、`mu` は互いを直列化しない。
3. **並列化の利益が出ない。** 特権を要するコマンドはすべてこの1つのロックで直列化されるため、
   そこでは並列にならない。

単一スレッドの現状では実行時に何も守っていない。唯一ありうる価値は再入検知（`fn()` が
`WithPrivileges` を再帰呼び出しするとデッドロックする）だが、デッドロックは検知手段として
劣悪であり、意図された設計でもない。

**削除の条件**: 削除と同時に、「特権窓の直列化と、非参加 goroutine の保護は未解決の設計課題で
ある」ことを `WithPrivileges` の doc コメントに明記する（AC-11）。`mu` を残すと「特権まわりは
並行安全」と読めてしまい、本当に必要な設計（特権操作中はプロセス全体を止める／特権操作を別
プロセスへ切り出す）の検討を先送りさせる。

**`pwentMutex` はこれと性質が異なり、維持のままとする。** あちらは参加者が `getpwent` 系の
呼び出しの範囲内に限られるため、参加者間の相互排他で十分になる。非参加者が状態を壊す経路が無い点が
決定的な違いである。

### `VerifiedFD.closed`（D10）について

現行の doc コメントは「Close is idempotent and safe for concurrent use」「The atomic swap
ensures syscall.Close runs for exactly one caller, avoiding a double-close race (CWE-1341)」と
述べる。これは**表明された契約**であり、削除は契約の変更を伴う。したがって平の `bool` へ
落とすと同時に、doc コメントから並行安全性の主張を取り除き、冪等性（同一 goroutine からの
二重 close が安全であること）だけを述べる形に改める（AC-10）。契約を残したまま実装だけ弱める
ことは許さない。

### 削除に伴うテストの扱い

`-race` つきの `make test` は、削除候補が実は並行だった場合を捕まえる主要な安全網である。
しかし削除候補の一部には「並行に呼んでも壊れないこと」を主張する既存テストがあり、ロックを
外すとこれらは正しく `-race` で失敗する。テストを残すことはできない。

CLAUDE.md は「テストの削除は検証を要する主張である」と定める。よって削除するテストごとに、
**その関数が主張していた非並行の性質（冪等性・キャッシュの一貫性・memo のヒットなど）を
逐次的なテストとして残すか、既存の逐次テストが同じ関数を覆っていることを確認する**。
`go tool cover -func` を関数単位で比較し、カバレッジが落ちていないことを確かめる（AC-13）。

対象となる既存の並行テストは次のとおりである。

| 対象 | テスト |
|---|---|
| D2 | `internal/groupmembership/manager_test.go:1360`（キャッシュへの並行アクセス） |
| D4 | `internal/groupmembership/manager_test.go:1432,1445`（memo の single-flight） |
| D6 | `internal/groupmembership/policy_test.go:87`（方針設定の競合） |
| D8 | `internal/verification/result_collector_test.go:118` |
| D10 | `internal/runner/base/risktypes/types_test.go:105`（並行 close） |
| D11 | `internal/runner/base/privilege/race_test.go`（ファイル全体、3箇所） |

K2 を覆う `internal/runner/base/output/capture_test.go:364` は維持対象のテストであり、削除しない。

### 進め方

削除候補は1件ずつ、独立したコミットで外す（revert 可能な粒度）。各削除で `make test`
（`CGO_ENABLED=1` は `-race` つき、`CGO_ENABLED=0` は素）と `make lint` を両構成で通す。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 過剰な排他制御の削除

「削除候補」の表に挙げた D1〜D11 を production コードから取り除く。

**Acceptance Criteria**:

- **AC-01**: D1〜D11 の各対象がフィールド定義もろとも production コードから消えており、
  その `Lock`／`Unlock`／`RLock`／`RUnlock`／`Load`／`Store`／`Swap`／`CompareAndSwap` の
  呼び出しも残らない。
- **AC-02**: D1〜D11 の削除がそれぞれ独立したコミットになっており、1件ずつ revert できる。
  各コミットのメッセージに、その箇所が単一スレッドでしか到達しない根拠（削除候補表の「根拠」
  欄に相当する内容）が記されている。
- **AC-03**: 削除の対象は `internal/` と `cmd/` の production コード（`_test.go` を除く）に
  限られる。`internal/testutil/handlers.go`（K7）は変更されない。
- **AC-04**: D6（`processPermissionCheckUIDPolicy`）の削除後も、
  `SetProcessPermissionCheckUIDPolicy` の外部から観測できる契約が変わらない。すなわち、同じ値の
  再設定は no-op で `nil` を返し、異なる値の設定は `ErrPermissionCheckUIDPolicyConflict` を返して
  格納値を変えず、`PolicyUnset` および不正値は `ErrInvalidPermissionCheckUIDPolicy` を返す。
- **AC-05**: D3・D5（2つのレポータ）の削除後も、それぞれの警告がプロセスにつき1回だけ記録される
  という挙動が変わらない。2回目以降の `report` 呼び出しは何も記録しない。
- **AC-06**: D4（`sudoUIDExistenceMemo`）の削除後も、確認済み UID の再問い合わせが起きず、
  失敗した確認は毎回再問い合わせされるという挙動が変わらない。
- **AC-07**: D2（メンバーシップキャッシュ）・D7（パス解決キャッシュ）の削除後も、キャッシュヒット
  時と未ヒット時の返り値が本タスクの前後で変わらない。
- **AC-08**: D9（`tempDirs` 管理）の削除後も、一時ディレクトリの登録・解放・クリーンアップの
  外部から観測できる挙動が通常版・dry-run 版とも変わらない。
- **AC-09**: D1（`outputWrapper.mu`）の削除後も、`GetBuffer` が返す内容と `GetWriteError` が返す
  最初のエラーが本タスクの前後で変わらない。標準出力と標準エラー出力が取り違えられない。
- **AC-10**: D10（`VerifiedFD.closed`）の削除後、`Close` は同一 goroutine からの二重呼び出しに対して
  `syscall.Close` を1回だけ実行し、nil レシーバに対して `nil` を返す。doc コメントから
  「safe for concurrent use」および CWE-1341 の二重 close レースに関する主張が取り除かれ、
  冪等性だけを述べるものになっている。

#### F-002: `WithPrivileges` の未解決課題の明記

**Acceptance Criteria**:

- **AC-11**: `WithPrivileges` の doc コメントに、(a) 特権窓の直列化が保証されないこと、
  (b) 特権窓が開いている間はプロセス全体の euid が上がるため、`WithPrivileges` に参加しない
  goroutine（`os/exec` の出力コピー goroutine を含む）も特権下で走ること、(c) これらが未解決の
  設計課題であり、並列実行を導入する際には別途の設計が要ることが記されている。
- **AC-12**: 削除後の `privilege` パッケージの doc コメント・コード内コメントに、
  「並行安全である」「並行呼び出しから保護されている」と読める記述が残らない。

#### F-003: 維持対象への根拠の明記

**Acceptance Criteria**:

- **AC-14**: K2（`output/capture.go`）の `mutex` に、必要な理由が doc コメントとして記されている。
  少なくとも (a) `os/exec` は `*os.File` でない writer ごとに goroutine を1つ起こすこと、
  (b) stdout 用と stderr 用の2つの `outputWrapper` が同一の `Capture` を共有すること、の2点が
  読み取れる。
- **AC-15**: K1・K3・K4・K5 の各対象に、それを触りうる並行の経路（Slack 送信ワーカー、Slack flush、
  slog ハンドラが出力コピー goroutine 上で走りうること）が doc コメントとして記されている。
- **AC-16**: K6 の 2箇所について、これが排他制御ではなくメモ化であること、および
  `OriginalExecutionIdentity` は「最初の権限変更より前に捕捉する」正しさの要請を担うため
  手書きの遅延初期化に置き換えてはならないことが doc コメントから読み取れる。
- **AC-17**: `pwentMutex` の doc コメントに、`setpwent`／`getpwent`／`endpwent` が libc の
  プロセス全体のカーソルであること、および外すと将来の並行呼び出しでエラーではなく黙って誤った
  列挙結果になることが記されている。あわせて、本タスクの棚卸しで意図的に維持したことが読み取れる。

#### F-004: テストの整理と被覆の維持

**Acceptance Criteria**:

- **AC-13**: 「削除に伴うテストの扱い」の表に挙げた各テストについて、削除した場合はその関数が
  主張していた非並行の性質を覆う逐次テストが存在する。削除の前後で `go tool cover -func` の
  出力を関数単位で比較し、カバレッジが落ちた関数が無いことを確認したうえで、その旨をコミットメッセージ
  に記す。
- **AC-18**: `internal/runner/base/output/capture_test.go` の並行テストは削除されず、
  `-race` つきで引き続き通過する。
- **AC-19**: AC-04〜AC-10 を検証するテストが、検証対象の挙動を壊すと失敗する（CLAUDE.md
  「テストは主張する理由で失敗できること」）。壊し方と失敗を確認した旨をコミットメッセージに記す。

#### F-005: 全体の健全性

**Acceptance Criteria**:

- **AC-20**: 各削除コミットの時点で、`CGO_ENABLED=1`（`-race` つき）と `CGO_ENABLED=0` の双方で
  `make test` が通過する。`-race` の警告が1件も出ない。
- **AC-21**: 各削除コミットの時点で、`CGO_ENABLED=1`・`CGO_ENABLED=0` の双方で `make lint` が
  通過する。
- **AC-22**: `make deadcode` が本タスクの前後で新たな到達不能コードを報告しない。
- **AC-23**: 本タスクの完了時点で、`internal/` と `cmd/` の production コードに残る
  `sync.Mutex`／`sync.RWMutex`／`sync.Once*`／`sync.WaitGroup`／`atomic.*` が、「維持対象」の表と
  「対象外」に挙げた `pwentMutex` の集合に一致する。この一致が、production ファイルを走査して
  検証される（並行リストを持たず、ソース集合を直接走査する）。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が満たされ、対応するテストが CGO・非 CGO 双方のビルドで
  `make test` により成功する。
- 外部から観測できる挙動が本タスクの前後で変わらない。CLI の出力、エラーの種類、ログの内容、
  権限判定の結果のいずれも変化しない。
- 削除候補の各件が独立して revert できる。ある削除が誤りだったと後に判明した場合、その1件だけを
  戻せる。
- production コードに残る排他制御をコードから読んだ読み手が、それぞれについて「どの goroutine と
  どの goroutine の間で並行なのか」を doc コメントから答えられる。
- 特権まわりを次に触る者が、`WithPrivileges` が並行安全ではないこと、およびそれが未解決の設計
  課題であることを doc コメントから知る。
