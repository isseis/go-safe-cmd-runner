# 実装計画書: privilege の残 Low 所見（metrics 削除・失敗経路のテスト追加・再入契約の明記）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-20 |
| Review date | 2026-08-20 |
| Reviewer | isseis |
| Comments | 変更範囲が privilege package に閉じ新規の設計要素を持たないため、`02_architecture.md` を省略して本書を作成した。設計上の選択は `01_requirements.md` の「決定事項」節にある |

## 1. 実装の概要

### 目的

[01_requirements.md](01_requirements.md) の F-001〜F-004 を実装する。対象は [#977](https://github.com/isseis/go-safe-cmd-runner/issues/977)（A1 L-2・L-3・L-4）と、L-3 の対応に伴い対象物が消滅する L-5 である。

### 実装方針

- **production の identity 変更箇所を増減させない。** 本タスクは `escalatePrivileges` の `syscall.Seteuid(0)` と `restorePrivileges` の `syscall.Seteuid(m.originalUID)` という2つの呼び出し式に一切手を触れない。`identity_mutation_guard_test.go` の allowlist と禁止規則も変更しない。
- **テスト用の注入点を新設しない。** 失敗分岐は、非特権プロセスで実際に `EPERM` を起こして踏む。
- **削除を先、テスト追加を後にする。** Phase 1 で metrics を削除してから Phase 2 のテストを書くことで、新規テストが削除対象の API に依存する事故を防ぐ。
- 各 Phase の完了時に `make fmt` → `make test` → `make lint` を通す。

### 既存コード調査結果

#### `internal/runner/base/privilege/metrics.go`（削除対象）

`Metrics` 型と5つのメソッド（`RecordElevationSuccess`・`RecordElevationFailure`・`updateSuccessRate`・`GetSnapshot`・`Reset`）のみを含む。他の型はない。~~ファイル冒頭の `// Package privilege provides metrics collection for privilege operations.` が本 package の package コメントになっているため、削除時に package コメントの行き先を決める必要がある（`manager.go` へ移す）。~~ **実装時に訂正**: package コメントは `errors.go:1` の `// Package privilege provides secure privilege escalation functionality for command execution.` が提供しており（ファイル名順で `errors.go` が先）、`metrics.go` 冒頭のものは重複である。移設は不要で、`metrics.go` ごと削除すれば足りる。

#### `internal/runner/base/privilege/unix.go`（変更）

- `UnixPrivilegeManager.metrics` フィールド（:36）と `GetMetrics`（:430-433）。
- 記録呼び出し3箇所: `prepareExecution` 失敗時（:98）、`performElevation` 失敗時（:103）、復元成功時（:210）。
- `handleCleanupAndMetrics`（:177）が `duration` を計算し（:190-193）、`restorePrivilegesAndMetrics`（:203）へ渡す。metrics を削除すると `duration` の計算・引数・`executionContext.start`（:124）がすべて不要になる。
- `panicValue` は panic の再送出と `shutdownContext` の文言に使うため `handleCleanupAndMetrics` 側では残る。一方 `restorePrivilegesAndMetrics` 側の `panicValue` は、metrics 削除後は用途が無くなるため引数ごと削除できる（`shutdownContext` は呼び出し側で組み立て済み）。
- `escalatePrivileges` の失敗分岐（:270-279）は `*Error` を返す。`restorePrivileges` の失敗分岐（:300-302）は生の syscall エラーを返し、呼び出し側で `emergencyShutdown` に渡される。いずれも既存テストが踏んでいない。
- `restorePrivilegesAndMetrics` には `needsPrivilegeEscalation` を条件とするブロックが2つ並んでいる（:206 の復元と :218 の identity・saved-set 検証）。metrics 削除後も**統合せず2ブロックのまま残す**。:218 のブロックには防御多重化の根拠を説明する長いコメントが付いており、統合するとコメントの係り先が曖昧になるためである。

#### `internal/runner/base/privilege/manager.go`（変更）

`Manager` インターフェースの `GetMetrics() Metrics`（:16）を削除する。同インターフェースの `GetCurrentUID`・`GetOriginalUID` も本番の呼び出し元を持たないが、本タスクの対象外（[01_requirements.md](01_requirements.md) 決定事項）。

#### `internal/runner/base/privilege/testutil/mocks.go`（変更）

`MockPrivilegeManager.GetMetrics`（:76-78）を削除する。`privilege.Metrics`（:78）は同ファイルにおける `privilege` package の唯一の利用箇所であるため、`privilege` の import（:10）も併せて削除する必要がある。削除後、`privilegetestutil` は `privilege` package に依存しなくなる。

#### 既存テストの扱い

metrics 削除により**主張が消えるテスト**が4つある。いずれも「metrics に何が記録されたか」以外を検証していないため削除するが、それぞれの不変条件が残存テストで担保されることを以下に示す。

| 削除するテスト | 検証していた不変条件 | 削除後の担保 |
|---|---|---|
| `TestHandleCleanupAndMetrics_Success`（unix_privilege_test.go:156） | 非 panic 時に非ゼロの duration が計算され下位へ渡ること | duration 自体を削除するため不変条件が消滅する |
| `TestRestorePrivilegesAndMetrics_Success`（:218） | 昇格した操作の復元後に成功が記録されること | 「復元処理が走り `emergencyShutdown` が起きないこと」は `TestRestorePrivilegesAndVerify_SavedSetUnchanged_Passes`（改名後）が `osExit` の未呼び出しで担保する。なお両テストとも `originalUID` を設定しないため `restorePrivileges` は native root の早期リターンを通り、実際の `seteuid` は走らない。削除するテストも同じ条件であり、失われる検証はない |
| `TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalation`（:250） | 未昇格の操作で成功が記録されないこと | 「未昇格なら復元・検証をしない」は `TestRestorePrivilegesAndVerify_IdentityVerificationSkippedWithoutEscalation`（改名後）が `verifierCalled == false` で担保する |
| `TestRestorePrivilegesAndMetrics_Failure`（:273） | panic 時に成功が記録されないこと。名前に反し復元自体は失敗させていない | 復元失敗の分岐は Phase 2 の新規テスト `TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` が初めて実際に踏む |

加えて `TestPrepareExecution_Success` の `assert.NotZero(t, execCtx.start)`（unix_privilege_test.go:59）も、`start` フィールドの削除により消える。同テストの他のアサーション（operation 判定・saved-set の取り込み）は残る。

`metrics_test.go` は全体を削除する。同ファイルが定義する `ErrTestPrivilegeElevationFailure`・`ErrTestFailure`・`ErrTestError` は同ファイル内でのみ参照されており、他への影響はない（確認済み）。

`race_test.go:235` の `_ = manager.GetMetrics()` は、並行読み取りの対象の1つとして呼ばれている。~~同じループ内に `GetCurrentUID`・`GetOriginalUID`・`IsPrivilegedExecutionSupported` の3つが残るため、当該行の削除のみでテストの主旨は保たれる。~~ **実装時に訂正**: 残る3つは競合しえない（`GetCurrentUID` は `syscall.Geteuid()` を呼ぶだけ、他の2つは構築時に一度だけ書かれるフィールドの読み取り）。`GetMetrics` は RWMutex 下の共有カウンタに触れる唯一の呼び出しであり、これを外すと `TestUnixPrivilegeManager_ThreadSafety` は `-race` で何も報告しえない。したがって当該テストは関数ごと削除する（ステップ7）。

#### 改名・フィールド削除に追従する箇所（実測値）

| パターン | `unix_privilege_test.go` | `identity_linux_test.go` | `identity_other_test.go` |
|---|---|---|---|
| `handleCleanupAndMetrics` / `restorePrivilegesAndMetrics`（大文字始まりのテスト関数名・doc コメント中の言及を含む） | 36 行 | 5 行 | 4 行 |
| `executionContext` リテラル中の `start:` | 10 箇所 | 1 箇所 | 1 箇所 |

重要な点として、この 45 行のうち相当数は**テスト関数名と doc コメント中の言及**であり、取りこぼしてもコンパイルエラーにならない。したがって改名は `rg` の全ヒットを対象に行い、AC で残存ゼロを確認する。

改名対象のテスト関数は次の3つである。

- `TestHandleCleanupAndMetrics_WithError`（unix_privilege_test.go:186）→ `TestHandleCleanup_WithError`
- `TestRestorePrivilegesAndMetrics_IdentityVerificationPassesOnCleanRestore_WithGroundTruth`（identity_linux_test.go:89）→ `TestRestorePrivilegesAndVerify_...`（以下同様に接頭辞のみ置換）
- `TestRestorePrivilegesAndMetrics_SkipsSavedSetCheckOnNonLinux`（identity_other_test.go:33）→ `TestRestorePrivilegesAndVerify_SkipsSavedSetCheckOnNonLinux`

これに加えて `unix_privilege_test.go` 内の `TestRestorePrivilegesAndMetrics_*` 6件（`_Success`・`_NoSuccessWithoutEscalation`・`_Failure` は削除、`_IdentityLeakTriggersShutdown`・`_IdentityVerificationSkippedWithoutEscalation`・`_SavedSetUnchanged_Passes`・`_SavedSetChanged_TriggersShutdown`・`_SavedSetCheckSkipped_NonLinux` は改名）が対象となる。

`identity_linux_test.go`・`identity_other_test.go` は `time` package を `start: time.Now()` でしか使っていないため、`start` の削除に伴い import も削除する。

#### `identity_other_test.go` のビルドタグ

同ファイルのビルド制約は `//go:build !linux && !windows && test` であり、Linux 上の `make test` でも CI（`ubuntu-latest`）でもコンパイルされない。macOS を対象とするワークフロー（`macos-dyld.yml`）は `paths:` により `internal/dynlib/machodylib/**` に限定されている。したがって同ファイルへの改名・フィールド削除の取りこぼしは、通常の `make test` では検出できない。Phase 1 の完了条件に `GOOS=darwin` でのクロスコンパイル検査を入れる。

#### 改名の影響を受ける文書（`docs/tasks/` 以外）

以下の3対（日本語版・英語版）が、改名対象の関数をコード引用または説明文で参照している。

| 文書 | 参照箇所 |
|---|---|
| `docs/dev/architecture_design/security-architecture{.ja,}.md` | :298（コード引用）、:316（説明文）、:319（コード中のコメント） |
| `docs/dev/developer_guide/design-implementation-overview{.ja,}.md` | :112（コード引用） |
| `docs/user/security-risk-assessment{.ja,}.md` | :86（コード引用） |

`security-architecture` の :316 は「`handleCleanupAndMetrics` はパニック回復と**時間計測**を担う」と説明しているが、時間計測は本タスクで削除されるため、名称だけでなく記述内容の修正が必要である。3対はいずれも日英の対訳であり、CLAUDE.md の方針に従い日本語版を先に更新し、英語版は `/mktrans` で反映する。

#### 失敗分岐を踏むための前提（Phase 2）

- `syscall.Seteuid(x)` は、非特権プロセスでは `x` が real・effective・saved のいずれかに一致する場合のみ成功する。テストバイナリは setuid ビットを持たないため3つとも `syscall.Getuid()` に等しく、`0` および `syscall.Getuid() + 1` はどれとも一致しない。
- Go の `syscall.Seteuid` は全スレッドに適用されるが、`runtime.doAllThreadsSyscall` は起動スレッドで最初に実行し、errno が非ゼロならば他スレッドへ伝播させる前に戻る。したがって**失敗時にどのスレッドの identity も変化しない**。cgo 有効時は glibc の setxid が同様に原子的に失敗する。
- 例外として、real UID が 65534 の場合の `+1`（= 65535 = `(uid_t)-1`）や、user namespace で `uid+1` が未マップの場合は `EINVAL` になる。**`EPERM` であることをアサートしない**（エラーが非 nil の `syscall.Errno` であることのみを確認する）ことで、この差に依存しないテストにする。
- 既存の同型のテストが `cmd/runner/startup_privilege_test.go:30-46` にある（`TestDropStartupPrivileges_FailsClosedOnSetegidFailure`）。root skip・実行前後の identity 比較に加え、同ファイルは冒頭（:3-13）に「プロセス全体の状態を触るため `t.Parallel()` を呼んではならない」旨のファイルコメントを持つ。同じ制約が新規テストにも当てはまるため、この注意書きも移植する。
- CI は `ubuntu-latest`（GitHub ホストランナー、非 root、`container:` 指定なし）、`Makefile` のテストターゲットも素の `go test` であり、開発コンテナも非 root（uid 1000）である。したがって root skip は保険であり、通常は skip されずに実行される。

## 2. 実装ステップ

### Phase 1: `privilege.Metrics` の削除と改名（F-001 / AC-01〜AC-06、F-004 のうち AC-21）

**対象ファイル**: `internal/runner/base/privilege/metrics.go`, `metrics_test.go`, `manager.go`, `unix.go`, `testutil/mocks.go`, `unix_privilege_test.go`, `identity_linux_test.go`, `identity_other_test.go`, `race_test.go`, `docs/dev/architecture_design/security-architecture{.ja,}.md`, `docs/dev/developer_guide/design-implementation-overview{.ja,}.md`, `docs/user/security-risk-assessment{.ja,}.md`

- [x] **ステップ1**: `internal/runner/base/privilege/metrics.go` を削除する。~~package コメント `// Package privilege provides metrics collection for privilege operations.` は失われるため、`manager.go` の `package privilege` の直前に `// Package privilege manages elevation to root and restoration of the original privileges for operations that require them.` を追加する。~~ **実装時に前提の誤りが判明したため取りやめた**: `errors.go:1` に `// Package privilege provides secure privilege escalation functionality for command execution.` が既にあり、ファイル名順で `errors.go` が先に来るため package コメントは元からこちらが提供していた。`metrics.go` 冒頭のコメントは package コメントではなく重複であり、移設先は不要である（`manager.go` に足すと `go doc` が2つの要約を出す）。
- [x] **ステップ2**: `internal/runner/base/privilege/metrics_test.go` を削除する。
- [x] **ステップ3**: `unix.go` から metrics を取り除く。
  - [x] `UnixPrivilegeManager.metrics Metrics` フィールドを削除する。
  - [x] `WithPrivileges` の `m.metrics.RecordElevationFailure(err)` 2箇所を削除する。
  - [x] `restorePrivilegesAndMetrics` の `m.metrics.RecordElevationSuccess(duration)` を削除する。これにより `if err := m.restorePrivileges(); err != nil { m.emergencyShutdown(...) }` の `else if` 節がなくなる。`needsPrivilegeEscalation` を条件とする2つのブロックは統合せずそのまま残す（§1 の調査結果を参照）。
  - [x] `GetMetrics` メソッドを削除する。
  - [x] `executionContext.start` フィールドと、`handleCleanupAndMetrics` 内の `duration` の宣言・計算を削除する。
  - [x] `restorePrivilegesAndMetrics` のシグネチャから `duration time.Duration` と `panicValue any` を削除し、`(execCtx *executionContext, shutdownContext string)` にする。
  - [x] `prepareExecution` の `start: time.Now()` の設定を削除する。`time` の import は `Error{Timestamp: time.Now()}` で引き続き使うため残す。
- [x] **ステップ4**: `unix.go` の関数を改名する。`handleCleanupAndMetrics` → `handleCleanup`、`restorePrivilegesAndMetrics` → `restorePrivilegesAndVerify`。doc コメントの「and metrics recording」に相当する記述も実態に合わせて書き換える（`// handleCleanup recovers from a panic in the callback, restores privileges, and verifies identity.` / `// restorePrivilegesAndVerify restores the original privileges and verifies that no elevated identity leaked.`）。
- [x] **ステップ5**: `manager.go` の `Manager` インターフェースから `GetMetrics() Metrics` を削除する。
- [x] **ステップ6**: `testutil/mocks.go` から `MockPrivilegeManager.GetMetrics` とその doc コメントを削除し、未使用になる `privilege` package の import も削除する。
- [x] **ステップ7**: `race_test.go` の `TestUnixPrivilegeManager_ThreadSafety` ごと削除する。§1 は「同じループ内に `GetCurrentUID`・`GetOriginalUID`・`IsPrivilegedExecutionSupported` の3つが残るため、当該行の削除のみでテストの主旨は保たれる」としていたが、これは誤りであった。残る3つのうち `GetCurrentUID` は `syscall.Geteuid()` を呼ぶだけ、他の2つは構築時に一度だけ書かれるフィールドを読むだけであり、`GetMetrics`（RWMutex 下の共有カウンタ）が唯一の競合しうる呼び出しだった。行だけを削ると、このテストは `-race` で何も報告しえない＝主張する理由で失敗できないテストになる（CLAUDE.md）。関数ごと削除しても `go tool cover -func` は全関数で不変であることを確認済み。
- [x] **ステップ8**: 主張が消える既存の検証を削除する（§1「既存テストの扱い」の表の4件＋アサーション1件。それぞれ個別に確認する）。
  - [x] `TestHandleCleanupAndMetrics_Success`
  - [x] `TestRestorePrivilegesAndMetrics_Success`
  - [x] `TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalation`
  - [x] `TestRestorePrivilegesAndMetrics_Failure`
  - [x] `TestPrepareExecution_Success` 内の `assert.NotZero(t, execCtx.start)`
- [x] **ステップ9**: テスト側の `executionContext` リテラルから `start:` を削除する（`unix_privilege_test.go` 10箇所・`identity_linux_test.go` 1箇所・`identity_other_test.go` 1箇所）。`rg -n "start:\s+time\.Now\(\)" internal/runner/base/privilege/` の全ヒットを対象とする。併せて `identity_linux_test.go`・`identity_other_test.go` の `time` import を削除する（`start` が唯一の利用箇所のため）。
- [x] **ステップ10**: 残るテストの呼び出しとテスト関数名を改名後の名前に合わせる。`rg -n "AndMetrics" internal/` の全ヒット（実測 45 行）を対象とし、引数から `panicValue`・`duration` を落とす。改名するテスト関数は §1 の一覧を参照する。doc コメント中の言及も同時に直す。
- [x] **ステップ11**: 削除に着手する前（`origin/main` の状態、すなわちステップ1 の実施前）に `go test -tags test -coverprofile=/tmp/before.out ./internal/runner/base/privilege/` を取得しておき、ステップ1〜10 の完了後に同様の `after.out` を取得して、`go tool cover -func` の差分が「削除した関数の消滅」のみであること、残存関数のカバレッジが低下していないことを確認する（CLAUDE.md「テストの削除は検証を要する主張」）。
- [x] **ステップ12**: 改名の影響を受ける文書3対を、改名と同じ PR で更新する。§1「改名の影響を受ける文書」の表の6ファイルが対象。
  - [x] 日本語版3ファイル（`security-architecture.ja.md`・`design-implementation-overview.ja.md`・`security-risk-assessment.ja.md`）のコード引用と説明文を、改名後の名称に更新する。
  - [x] `security-architecture.ja.md:316` の「パニック回復と時間計測を担い」を、時間計測が無くなった実態に合わせて書き換える。
  - [x] （計画から追加）`security-architecture.ja.md:269` は `UnixPrivilegeManager` の構造体定義を逐語引用しており、削除した `metrics Metrics` フィールドが残っていたため、同行を削除する。同節の他のフィールドが現行コードと一致することも確認する。
  - [x] 日本語版をコミットしたうえで、英語版3ファイルを `/mktrans` で反映する（CLAUDE.md のバイリンガル文書の編集順序）。

**完了条件**:
- `make fmt` → `make test` → `make lint` がすべて通る。
- `GOOS=darwin GOARCH=arm64 go vet -tags test ./internal/runner/base/privilege/` が通る（Linux では未コンパイルの `identity_other_test.go` の追従漏れを検出するため。現行ツリーで exit 0 になることは確認済み）。
- `make deadcode` が本ステップに起因する新規の未使用シンボルを報告しない。
- `rg -n "Metrics" internal/runner/base/privilege/` が0件。
- `rg -n "AndMetrics" docs/` のヒットが、本タスクおよび 0149・0157 の作業文書（`docs/tasks/` 配下の履歴的記述）のみになる。

### PR-1 作成ポイント: metrics removal and helper rename

**対象ステップ**: 1 / 2 / 3 / 4 / 5 / 6 / 7 / 8 / 9 / 10 / 11 / 12

**推奨タイトル**: `refactor(0166): remove privilege.Metrics and rename cleanup helpers`

**レビュー観点**: `restorePrivilegesAndVerify` の制御フロー改変（`else if` 節の除去）が復元失敗時の `emergencyShutdown` 到達条件を変えていないか / 削除したテストの不変条件が残存テストへ引き継がれているか（§1 の表とステップ11 のカバレッジ差分） / `AndMetrics` の改名が `rg` の全ヒットに及び、コンパイルで検出されないテスト関数名・doc コメント・設計文書3対を取りこぼしていないか / Linux でコンパイルされない `identity_other_test.go` の追従が `GOOS=darwin` クロス vet で確認されているか

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ3 が `restorePrivilegesAndVerify` の復元失敗 → `emergencyShutdown` という回復フローの制御構造を書き換える孤立した高リスク手順であり、加えてステップ10・12 の改名追従は CI でコンパイルも lint もされない範囲（`identity_other_test.go`・文書）に及ぶため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] Phase 1 の完了条件（`GOOS=darwin` クロス vet・`make deadcode`・`rg` の残存ゼロ）を満たしたことを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1042）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: 昇格・復元の失敗分岐をテストで踏む（F-002 / AC-07〜AC-13）

**対象ファイル**: `internal/runner/base/privilege/unix_privilege_test.go`

- [x] **ステップ13**: `unix_privilege_test.go` の冒頭に、`cmd/runner/startup_privilege_test.go:3-13` に倣ったファイルコメントを英語で追加する。内容は、本ファイルの一部のテストがプロセス全体の identity 状態に依存するため `t.Parallel()` を呼んではならないこと、および root で実行すると失敗分岐を踏めず skip されること。
- [x] **ステップ14**: `TestEscalatePrivileges` にサブテスト `seteuid_failure` を追加する。
  - [x] `syscall.Getuid() == 0 || syscall.Geteuid() == 0` のとき `t.Skip("running as root: seteuid(0) succeeds and the native-root path returns early, so the failure path cannot be exercised")` とする（real UID が 0 だと `escalatePrivileges` が native root の早期リターンを通り、`seteuid` に到達しないため両方を条件にする）。
  - [x] `privilegeSupported: true`・`originalUID: syscall.Getuid()` のマネージャを構築し、`escalatePrivileges` を呼ぶ。
  - [x] 戻り値が `*Error` であることを `errors.AsType[*Error]` で確認し、`Operation`・`CommandName`・`OriginalUID`・`TargetUID`（= 0）が渡した値と一致すること、`SyscallErr` が非 nil の `syscall.Errno` であることを検証する（環境により `EPERM` 以外になり得るため errno 値は固定しない）。
  - [x] 実行前後で `syscall.Geteuid()`・`syscall.Getegid()` が変化していないことを検証する。
- [x] **ステップ15**: `TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` を追加する。
  - [x] `syscall.Getuid() == 0 || syscall.Geteuid() == 0` のとき `t.Skip("running as root: seteuid to an arbitrary UID succeeds, so the restoration failure path cannot be exercised")` とする。
  - [x] `originalUID: syscall.Getuid() + 1`（real・effective・saved のいずれとも異なるため `Seteuid` は必ず失敗する）、`identityVerifier` に `func() error { return nil }`、`readSavedIDs` に `func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported }` を設定したマネージャを構築する。`osExit` は既存の同種テスト（`:366-371` ほか）に倣い、呼び出しを記録したうえで panic するスタブとし、`assert.PanicsWithValue` で捕捉する。これにより `emergencyShutdown` の後に後続の検証ブロックへ進まないことも同時に固定する。
  - [x] `needsPrivilegeEscalation: true`・`originalSUID: -1`・`originalSGID: -1` の `executionContext` で `restorePrivilegesAndVerify` を呼ぶ。
  - [x] `osExit` が終了コード 1 でちょうど1回呼ばれたことを検証する。`originalUID` の選択理由（なぜ必ず失敗するか）をコメントで説明する。
  - [x] 実行前後で `syscall.Geteuid()`・`syscall.Getegid()` が変化していないことを検証する。
- [x] **ステップ16**: 両テストが主張どおりの理由で失敗できることを確認する。ステップ14 は `escalatePrivileges` の `Seteuid` 失敗時の `return &Error{...}` を `return nil` に一時変更してテストが失敗することを、ステップ15 は `restorePrivilegesAndVerify` の `m.emergencyShutdown(err, shutdownContext)` を一時的にコメントアウトしてテストが失敗することを確認し、確認した旨をコミットメッセージに記す。

**完了条件**:
- `make test` が通る。
- `go test -tags test -count=1 -run 'TestEscalatePrivileges|TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown' -v ./internal/runner/base/privilege/` の出力に `--- PASS: TestEscalatePrivileges/seteuid_failure` と `--- PASS: TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` が現れる（親テストの PASS ではなくサブテスト行を確認する。`-count=1` を付けてキャッシュされた結果を避ける）。
- `TestNoUnexpectedIdentityMutationSyscalls` が無変更のまま通る（同ガードは `_test.go` を走査対象から除外するため、新規テストが `syscall.Seteuid` を間接的に起こしてもガードには影響しない）。

### PR-2 作成ポイント: syscall failure-path tests

**対象ステップ**: 13 / 14 / 15 / 16

**推奨タイトル**: `test(0166): cover privilege escalation and restoration syscall failures`

**レビュー観点**: 渡す UID が real・effective・saved のいずれとも一致せず、`Seteuid` が必ず失敗すること（識別子の選び方とコメントの説明） / テストがプロセスの identity を実際に変えていないこと（実行前後の実効 UID・GID 比較と `t.Parallel()` 不使用） / root skip の条件が `Getuid`・`Geteuid` の両方を見ており、理由文から成立しない理由が読めること / ステップ16 の「分岐を無効化するとテストが落ちる」確認がコミットメッセージに記されていること / PR 説明に L-2 を所見の推奨とは逆方向で close した理由が書かれているか（ステップ19 が監査文書に書く文面を正とし、そこから写す）

**実装モデル要件**: frontier-recommended

**判定理由**: ステップ15 が `emergencyShutdown` への到達（回復フロー）とプロセス全体の identity 状態に触る孤立した高リスク手順であるため。追加するのは同一 package の単体テスト2件のみで、統合テスト・CI・外部リソースの面を持たないため frontier-required には該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] Phase 2 の完了条件（サブテスト行が `--- PASS` であること・`TestNoUnexpectedIdentityMutationSyscalls` の無変更通過）を満たしたことを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1043）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: 再入禁止の契約の明記（F-003 / AC-14〜AC-17）

**対象ファイル**: `internal/runner/base/privilege/unix.go`, `internal/runner/base/runnertypes/config.go`

- [x] **ステップ17**: `UnixPrivilegeManager.WithPrivileges` の doc コメントに、再入禁止の契約と、再入検出を実装しない理由を英語で追記する。既存の doc（`OperationUserGroupExecution` と `OperationFileValidation` の説明）は残し、その後ろに段落を足す。内容は次の3点を含める。
  - [x] マネージャのミューテックスは `fn` の実行中も保持され続けること。特権状態がプロセス全体に及ぶため、直列化には保持が必要であること。
  - [x] `fn` の内側から同一マネージャの `WithPrivileges` を呼ぶと自己デッドロックすること（reentrant / deadlock の語を用いる）。再入しないことは呼び出し側の責務であること。
  - [x] 再入を検出してエラーを返す実装を置かない理由: 保持中フラグでは自ゴルーチンの再入と他ゴルーチンの正当な待機を区別できず、`TryLock` でも同様であるため。
- [x] **ステップ18**: `internal/runner/base/runnertypes/config.go` の `PrivilegeManager` インターフェースの `WithPrivileges`（:194）に、再入が禁止であることを英語の doc コメントで記載する。実装の詳細（ミューテックス保持）ではなく、利用者から見た契約として書く。

**完了条件**:
- `make lint` が通る。
- 追加した doc がすべて英語である。

### PR-3 作成ポイント: non-reentrant contract documentation

**対象ステップ**: 17 / 18

**推奨タイトル**: `docs(0166): document the non-reentrant contract of WithPrivileges`

**レビュー観点**: AC-14 の3点（ミューテックス保持・自己デッドロック・呼び出し側の責務）と AC-15（`TryLock` でも区別できない理由）が doc に揃っているか / インターフェース側（`runnertypes`）が実装詳細ではなく利用者から見た契約として書かれているか / 追記した doc がすべて英語であるか

**実装モデル要件**: standard

**判定理由**: doc コメントの追記のみでコード挙動を変えず、`既存コード調査結果` にも競合方針がないため、いずれのトリガーにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] Phase 3 の完了条件（追記した doc がすべて英語であること）を満たしたことを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1044）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 0149 の監査記録への反映（F-004 / AC-18〜AC-20）

**対象ファイル**: `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md`, `docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md`

- [x] **ステップ19**: `98_remaining_issues.md` §2 の「### A1（privilege）」を書き換える。同節は現在 L-2・L-3・L-4 の3つの箇条書きと #977 への参照行だけで構成されており、3件すべてが本タスクで解消するため、箇条書きは残らない。同文書が D1 M-3・B3 M2 に既に用いている引用ブロック形式（`> **D1 M-3 について**: …`、:15-17）に倣い、`> **A1 L-2/L-3/L-4 について**: …` として次を記す。
  - [x] L-3 の恒偽項は 0157 で解消済みであり、本タスクは残る2点（未昇格時の失敗計上・指標名と実態の乖離）を `Metrics` 型の削除で解消したこと。同時に L-5（一覧には未掲載）も対象物の消滅により解消したこと。
  - [x] L-2 を所見の推奨とは逆方向で close したことと、その根拠（`identity_mutation_guard_test.go` が identity 変更関数の値参照を禁じており、注入フィールドの追加はこの不変条件を崩す。失敗分岐は実 `EPERM` を用いたテストで踏んだ）。
  - [x] L-4 は doc への契約明記で対応し、再入検出は Go では正しく実装できないため見送ったこと。
  - [x] 本タスク（0166）と #977 への参照。
- [x] **ステップ20**: ステップ19 の書き換えが A1 以外の残件に波及していないことを、`git diff` で確認する。§1・§2・§3 の D1・B1・D2・E1 ほかの記述に増減がないこと。なお A1 の M-1・L-1・I-1〜I-4 は元から同文書に掲載されていないため、本タスクでは新規追加しない（掲載範囲の見直しは本タスクの対象外）。
- [x] **ステップ21**: `findings/A1_privilege.md` の L-2・L-3・L-4・L-5 の各項目末尾に「**対応状況**:」で始まる1行を追記する。所見の原文（監査時点の記述）は書き換えない。この追記形式は `findings/*.md` では本タスクが最初の導入となるため、他の findings と揃えるかどうかは後続タスクの判断に委ねる。

**完了条件**:
- 日本語文書の記述スタイル（太字は構造ラベルのみ、平易な語を優先）に沿っている。
- ステップ20 の `git diff` で、A1 節の外に変更行がない。

### PR-4 作成ポイント: 0149 audit record update

**対象ステップ**: 19 / 20 / 21

**推奨タイトル**: `docs(0166): record A1 L-2/L-3/L-4 resolution in the 0149 audit`

**レビュー観点**: L-2 を所見の推奨とは逆方向で close した根拠が引用ブロックから読み取れるか（AC-18） / 書き換えが A1 節に閉じ、他の残件の記述を増減させていないか（AC-20・ステップ20 の `git diff` 確認） / findings への追記が所見の原文を書き換えず、L-2・L-3・L-4・L-5 の4件すべてに入っているか（AC-19）

**実装モデル要件**: standard

**判定理由**: 既存の記載形式（D1 M-3 の引用ブロック）に倣う文書更新のみで、未確定の方針・高リスク手順・panel-mode トリガーのいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] Phase 4 の完了条件（ステップ20 の `git diff` で A1 節の外に変更行がないこと）を満たしたことを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 内容 | 対応 Phase | 成果物 |
|---|---|---|---|
| M1 | metrics の削除完了 | Phase 1 | `Metrics` 型が存在せず、`make test`・`make lint`・`make deadcode`・darwin クロス vet が通る状態 |
| M2 | 失敗分岐のテスト追加完了 | Phase 2 | 昇格・復元の syscall 失敗分岐と `emergencyShutdown` を踏むテストが非 root 環境で実行される状態 |
| M3 | 契約と監査記録の整備完了 | Phase 3・4 | 再入禁止が doc に明記され、0149 の残件一覧と findings が本タスクの結果を反映した状態 |

Phase 1 → Phase 2 の順序には依存がある（Phase 2 で追加するテストは改名後の関数名を使う）。Phase 3・Phase 4 は Phase 1・2 と独立している。

### 3.2 PR 構成

PR は Phase と一対一に対応させ、4本に分ける（PR-N ≡ Phase N）。分け方の根拠は次のとおりである。

- **Phase 1 を分割しない理由**: 削除と改名は技術的にはそれぞれ単独でもビルドが通るが、分けると中間状態で `AndMetrics` の名前が残り、Phase 1 の完了条件である「`rg -n "AndMetrics"` の残存ゼロ」が成立しない。改名は文書3対（ステップ12）まで含めて一度に確定させる。
- **Phase 2 を単独にする理由**: 実 syscall でプロセス全体の identity 状態を触る唯一の変更であり、削除・改名の大量の差分と混ぜずにレビューさせる。
- **Phase 3・Phase 4 を分ける理由**: いずれも文書のみだが、Phase 3 は Go の doc コメントに書く API の契約、Phase 4 は 0149 監査の記録であり、レビューする人と観点が異なる。

なお PR-1 が既存テストを削除しても、削除対象はいずれも metrics への記録内容だけを検証していたテストであり、`restorePrivileges` の失敗分岐は元から未カバーである（§1「既存テストの扱い」）。したがって PR-2 の新規テストは PR-1 で失われた検証の穴埋めではなく、純粋な追加カバレッジである。PR-1 と PR-2 の間の状態でカバレッジが後退する分岐はない。

依存関係: PR-2 は PR-1 の改名確定に依存する。PR-3・PR-4 は独立であり、PR-1 と並行に出してよい。

ステップ12 の英語版反映は、日本語版のコミット後に `/mktrans` で行うため、PR-1 内の別コミットとする。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1 / 2 / 3 / 4 / 5 / 6 / 7 / 8 / 9 / 10 / 11 / 12 | `privilege.Metrics` とその参照の削除、`handleCleanup`・`restorePrivilegesAndVerify` への改名、既存テストとカバレッジ差分の確認、改名を引用する文書3対の追従 | frontier-recommended |
| PR-2 | 13 / 14 / 15 / 16 | 昇格・復元の syscall 失敗分岐を実 `EPERM` で踏むテストの追加と、`t.Parallel` 禁止・root skip のファイルコメント | frontier-recommended |
| PR-3 | 17 / 18 | `WithPrivileges` と `runnertypes.PrivilegeManager` への再入禁止契約の doc 追記 | standard |
| PR-4 | 19 / 20 / 21 | 0149 の残件一覧・findings への対応状況の反映 | standard |

## 4. テスト戦略

### 単体テスト

- **新規**: 昇格失敗（`escalatePrivileges` の `Seteuid(0)` が失敗）と復元失敗（`restorePrivileges` の `Seteuid` が失敗）の2分岐。いずれも実際に syscall を呼んで失敗させる。モック・注入点は追加しない。
- **削除**: metrics の記録内容のみを検証していた4テストと `metrics_test.go` 全体、および `execCtx.start` のアサーション1件。§1 の表のとおり、消える不変条件は duration 計算（対象そのものが消滅）のみで、他は残存テストが担保する。
- **維持**: identity 検証・saved-set 検証・panic 経路・並行アクセスの各テストは、改名と引数変更への追従のみを行い、検証内容は変えない。

### 回帰の観点

- 特権昇格・復元の外部から観測できる挙動（`WithPrivileges` の戻り値、`emergencyShutdown` の発火条件、panic の再送出）は変えない。既存テストがこれらを担保する。
- 監査ログの特権指標（`elevation_count`・`total_privilege_duration_ms`）は executor 側の `audit.PrivilegeMetrics` に由来し、本タスクでは触らない。`internal/runner/base/executor` と `internal/runner/base/audit` を無変更に保つことで確認する。

### 静的検査

- `TestNoUnexpectedIdentityMutationSyscalls` を無変更で通す。これが本タスクの中心的な安全確認であり、production の identity 変更箇所が増減していないことを機械的に示す。
- `GOOS=darwin` でのクロス vet により、Linux ではコンパイルされない `identity_other_test.go` の追従漏れを検出する。

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| root 環境（CI 設定の変更、コンテナでの実行）で PR-2 のテストが常に skip され、踏んだつもりで踏めていない | 中 | 完了条件でサブテスト行が `--- PASS` であることを確認する。CI（`ubuntu-latest`、`container:` 指定なし）・開発コンテナがいずれも非 root であることは確認済み。ファイルコメントにも root CI ではこの検証が失われる旨を記す |
| PR-2 のテストが実際にプロセスの identity を変えてしまう | 高 | 失敗する UID のみを渡す。Go の `doAllThreadsSyscall` は起動スレッドの errno が非ゼロなら他スレッドへ伝播させずに戻るため、失敗時に identity は変化しない。加えて各テストが実行前後の実効 UID・GID の一致を検証するため、万一変化した場合はテストが失敗して検知できる |
| 失敗時の errno が環境により `EPERM` 以外になる（uid 65534 の `+1`、user namespace の未マップ uid） | 低 | errno 値を固定せず、非 nil の `syscall.Errno` であることのみを検証する |
| metrics 削除で、既存テストが暗黙に担保していた挙動が落ちる | 中 | §1 の表で不変条件ごとに引き受け先を明示し、PR-1 のステップ11 でカバレッジの差分を確認する |
| 改名の取りこぼしが、テスト関数名・doc コメント・設計文書に残る（いずれもコンパイルエラーにならない） | 中 | PR-1 のステップ10・12 で `rg -n "AndMetrics"` の全ヒットを対象とし、AC-04・AC-21 で残存ゼロを確認する |
| Linux でコンパイルされない `identity_other_test.go` の追従漏れ | 中 | PR-1（Phase 1）完了条件の `GOOS=darwin` クロス vet |

## 6. 実装チェックリスト

### PR-1: metrics の削除と改名（Phase 1）
- [x] ステップ1: `metrics.go` の削除と package コメントの移設
- [x] ステップ2: `metrics_test.go` の削除
- [x] ステップ3: `unix.go` からの metrics 除去
- [x] ステップ4: `unix.go` の関数改名
- [x] ステップ5: `manager.go` の `GetMetrics` 削除
- [x] ステップ6: `testutil/mocks.go` の `GetMetrics` と import の削除
- [x] ステップ7: `race_test.go` の該当行削除
- [x] ステップ8: 主張が消える4テストとアサーション1件の削除
- [x] ステップ9: テスト側 `start:` の削除と不要 import の除去
- [x] ステップ10: 残存テストの改名・引数追従
- [x] ステップ11: カバレッジ差分の確認
- [x] ステップ12: 改名を引用する文書3対の追従と `/mktrans`

### PR-2: 失敗分岐をテストで踏む（Phase 2）
- [x] ステップ13: ファイルコメント（`t.Parallel` 禁止・root skip）の追加
- [x] ステップ14: 昇格失敗テストの追加
- [x] ステップ15: 復元失敗テストの追加
- [x] ステップ16: 両テストが理由どおりに失敗できることの確認

### PR-3: 再入禁止の明記（Phase 3）
- [x] ステップ17: `WithPrivileges` の doc 追記
- [x] ステップ18: `runnertypes.PrivilegeManager` の doc 追記

### PR-4: 0149 の監査記録への反映（Phase 4）
- [x] ステップ19: `98_remaining_issues.md` §2 A1 の書き換え
- [x] ステップ20: 他の残件に波及していないことの確認
- [x] ステップ21: `findings/A1_privilege.md` への対応状況の追記

### PR のマージ状況
- [ ] PR-1 マージ済み（対象ステップ: 1 / 2 / 3 / 4 / 5 / 6 / 7 / 8 / 9 / 10 / 11 / 12）
- [ ] PR-2 マージ済み（対象ステップ: 13 / 14 / 15 / 16）
- [ ] PR-3 マージ済み（対象ステップ: 17 / 18）
- [ ] PR-4 マージ済み（対象ステップ: 19 / 20 / 21）

### 全体
- [ ] `make fmt` → `make test` → `make lint` がすべて通る
- [ ] `make deadcode` が新規の未使用シンボルを報告しない
- [ ] すべての AC が §7 の方法で検証済み

## 7. Acceptance Criteria 検証

`git diff` を用いる検証は、コミット後でも意味を持つよう `origin/main...HEAD`（マージベースからの差分）を対象とする。

| AC | 種別 | 検証方法 |
|---|---|---|
| AC-01 | static | `rg -n "Metrics" internal/runner/base/privilege/` が0件（型名・メソッド名・`metrics` フィールドの型注記をまとめて捕捉する）。加えて `test ! -e internal/runner/base/privilege/metrics.go` |
| AC-02 | static | `rg -n "GetMetrics" internal/ cmd/` が0件 |
| AC-03 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity`、`::TestHandleCleanup_WithError`、`::TestRestorePrivilegesAndVerify_IdentityLeakTriggersShutdown`、`::TestRestorePrivilegesAndVerify_SavedSetChanged_TriggersShutdown`、`internal/runner/base/privilege/manager_test.go::TestManager_WithPrivileges_UserGroup_FunctionError` が、改名への追従以外は無変更の検証内容で通る |
| AC-04 | static | `rg -n "execCtx\.start|start:\s+time\.Now\(\)|duration" internal/runner/base/privilege/` が0件（production・テストの両方を対象とする）。加えて `rg -n "AndMetrics" internal/` が0件 |
| AC-05 | static + test | `git diff --exit-code origin/main...HEAD -- internal/runner/base/audit/ internal/runner/base/executor/executor.go` が差分なし。加えて `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution` が無変更で通る |
| AC-06 | static | `make deadcode` の出力に `internal/runner/base/privilege` 由来の新規項目がない（実施前後の出力を比較する。現行ツリーでは同 package の項目は0件） |
| AC-07 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestEscalatePrivileges/seteuid_failure` |
| AC-08 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` |
| AC-09 | test | 上記2テスト内の実効 UID・GID の実行前後比較アサーション |
| AC-10 | static | `rg -n -A 3 "func TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown|seteuid_failure" internal/runner/base/privilege/unix_privilege_test.go` に `t.Skip` と root である旨の理由文が現れる。root 環境での実行確認は行わない（root では `IsPrivilegedExecutionSupported` の判定が変わり同 package の他テストの前提も崩れるため） |
| AC-11 | static | `internal/runner/base/privilege/identity_mutation_guard_test.go::TestNoUnexpectedIdentityMutationSyscalls` が通る（identity 変更関数の値参照・フィールド代入を検出して失敗させる）。加えて `rg -n "func\(uid int\) error|func\(gid int\) error" -g '!*_test.go' internal/runner/base/privilege/` が0件 |
| AC-12 | static | `git diff --exit-code origin/main...HEAD -- internal/runner/base/privilege/identity_mutation_guard_test.go` が差分なし、かつ `go test -tags test -count=1 -run TestNoUnexpectedIdentityMutationSyscalls ./internal/runner/base/privilege/` が PASS |
| AC-13 | manual | ステップ16 の手順で各分岐を無効化してテストが失敗することを確認し、コミットメッセージに記載する。AC-07・AC-08 の `test` 検証を補完するものであり、単独の検証手段ではない |
| AC-14 | static | `rg -n -B 25 "func \(m \*UnixPrivilegeManager\) WithPrivileges" internal/runner/base/privilege/unix.go`（doc コメントは関数シグネチャの手前にあるため後方向に見る）に `reentr`・`deadlock`・`mutex` の各語が現れる |
| AC-15 | static | 同じ出力に `TryLock` と、ゴルーチンを区別できない旨の記述が現れる |
| AC-16 | static | `rg -n -B 20 "WithPrivileges\(elevationCtx ElevationContext" internal/runner/base/runnertypes/config.go` に `reentr` が現れる |
| AC-17 | static | `rg -n "[ぁ-んァ-ヶ一-龠]" internal/runner/base/privilege/ internal/runner/base/runnertypes/config.go` が0件（`_test.go` も対象に含める。現行ツリーで0件であることを確認済み） |
| AC-18 | static | `98_remaining_issues.md` §2 の「A1（privilege）」に L-2・L-3・L-4 の箇条書きが残っておらず、`rg -n "A1 L-2/L-3/L-4 について" docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` が1件ヒットし、その引用ブロックに `0166`・`#977`・`identity_mutation_guard_test.go` の語が含まれる |
| AC-19 | static | `rg -n "対応状況" docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md` が L-2・L-3・L-4・L-5 の4箇所でヒットする |
| AC-20 | static | `git diff origin/main...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` の変更行が、`### A1（privilege）` 節と、そこへ追加する引用ブロックの範囲内に収まっている（他節への変更行が0であること） |
| AC-21 | static | `rg -n "handleCleanupAndMetrics|restorePrivilegesAndMetrics" docs/dev/ docs/user/` が0件。加えて `rg -n "時間計測|timing" docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md` の結果に、`handleCleanup` の責務としての記述が残っていない |

## 8. Success Criteria

- 上記すべての AC が §7 の方法で検証済みである。
- `make test` と `make lint` が通り、`GOOS=darwin` でのクロス vet も通る。
- `TestNoUnexpectedIdentityMutationSyscalls` が無変更のまま通り、production の identity 変更箇所が `escalatePrivileges` の `Seteuid(0)` と `restorePrivileges` の `Seteuid(m.originalUID)` の2箇所のままである。
- 特権昇格・復元・emergency shutdown の外部から観測できる挙動が本タスクの前後で変わらない。
- #977 の3件それぞれについて、解消したのか所見の推奨とは異なる形で close したのかが、コードと監査文書の双方から追える。

## 9. 次のステップ

- PR-1 → PR-2 の順に実装し、各 PR をマージする（PR-3・PR-4 は §3.2 のとおり独立であり、PR-1 と並行に出してよい）。PR-2 の説明に、L-2 を所見の推奨とは逆方向で close した理由（`identity_mutation_guard_test.go` の不変条件と、注入点を使わずに失敗分岐を踏んだこと）を記載する。
- PR-4 のマージ後に #977 を close する。#1041（`resource.Manager.WithPrivileges` の削除）は本タスクとは独立に扱う。
