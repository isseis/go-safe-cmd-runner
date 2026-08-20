# 実装計画書: privilege の残 Low 所見（metrics 削除・失敗経路のテスト追加・再入契約の明記）

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-08-20 |
| Review date | - |
| Reviewer | - |
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

`Metrics` 型と5つのメソッド（`RecordElevationSuccess`・`RecordElevationFailure`・`updateSuccessRate`・`GetSnapshot`・`Reset`）のみを含む。他の型はない。ファイル冒頭の `// Package privilege provides metrics collection for privilege operations.` が本 package の package コメントになっているため、削除時に package コメントの行き先を決める必要がある（`manager.go` へ移す）。

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

`race_test.go:235` の `_ = manager.GetMetrics()` は、並行読み取りの対象の1つとして呼ばれている。同じループ内に `GetCurrentUID`・`GetOriginalUID`・`IsPrivilegedExecutionSupported` の3つが残るため、当該行の削除のみでテストの主旨は保たれる。

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

### Phase 1: `privilege.Metrics` の削除（F-001 / AC-01〜AC-06）

**対象ファイル**: `internal/runner/base/privilege/metrics.go`, `metrics_test.go`, `manager.go`, `unix.go`, `testutil/mocks.go`, `unix_privilege_test.go`, `identity_linux_test.go`, `identity_other_test.go`, `race_test.go`

- [ ] **ステップ1**: `internal/runner/base/privilege/metrics.go` を削除する。package コメント `// Package privilege provides metrics collection for privilege operations.` は失われるため、`manager.go` の `package privilege` の直前に `// Package privilege manages elevation to root and restoration of the original privileges for operations that require them.` を追加する。
- [ ] **ステップ2**: `internal/runner/base/privilege/metrics_test.go` を削除する。
- [ ] **ステップ3**: `unix.go` から metrics を取り除く。
  - [ ] `UnixPrivilegeManager.metrics Metrics` フィールドを削除する。
  - [ ] `WithPrivileges` の `m.metrics.RecordElevationFailure(err)` 2箇所を削除する。
  - [ ] `restorePrivilegesAndMetrics` の `m.metrics.RecordElevationSuccess(duration)` を削除する。これにより `if err := m.restorePrivileges(); err != nil { m.emergencyShutdown(...) }` の `else if` 節がなくなる。`needsPrivilegeEscalation` を条件とする2つのブロックは統合せずそのまま残す（§1 の調査結果を参照）。
  - [ ] `GetMetrics` メソッドを削除する。
  - [ ] `executionContext.start` フィールドと、`handleCleanupAndMetrics` 内の `duration` の宣言・計算を削除する。
  - [ ] `restorePrivilegesAndMetrics` のシグネチャから `duration time.Duration` と `panicValue any` を削除し、`(execCtx *executionContext, shutdownContext string)` にする。
  - [ ] `prepareExecution` の `start: time.Now()` の設定を削除する。`time` の import は `Error{Timestamp: time.Now()}` で引き続き使うため残す。
- [ ] **ステップ4**: `unix.go` の関数を改名する。`handleCleanupAndMetrics` → `handleCleanup`、`restorePrivilegesAndMetrics` → `restorePrivilegesAndVerify`。doc コメントの「and metrics recording」に相当する記述も実態に合わせて書き換える（`// handleCleanup recovers from a panic in the callback, restores privileges, and verifies identity.` / `// restorePrivilegesAndVerify restores the original privileges and verifies that no elevated identity leaked.`）。
- [ ] **ステップ5**: `manager.go` の `Manager` インターフェースから `GetMetrics() Metrics` を削除する。
- [ ] **ステップ6**: `testutil/mocks.go` から `MockPrivilegeManager.GetMetrics` とその doc コメントを削除し、未使用になる `privilege` package の import も削除する。
- [ ] **ステップ7**: `race_test.go` の `_ = manager.GetMetrics()`（:235）の行を削除する。
- [ ] **ステップ8**: 主張が消える既存の検証を削除する（§1「既存テストの扱い」の表の4件＋アサーション1件。それぞれ個別に確認する）。
  - [ ] `TestHandleCleanupAndMetrics_Success`
  - [ ] `TestRestorePrivilegesAndMetrics_Success`
  - [ ] `TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalation`
  - [ ] `TestRestorePrivilegesAndMetrics_Failure`
  - [ ] `TestPrepareExecution_Success` 内の `assert.NotZero(t, execCtx.start)`
- [ ] **ステップ9**: テスト側の `executionContext` リテラルから `start:` を削除する（`unix_privilege_test.go` 10箇所・`identity_linux_test.go` 1箇所・`identity_other_test.go` 1箇所）。`rg -n "start:\s+time\.Now\(\)" internal/runner/base/privilege/` の全ヒットを対象とする。併せて `identity_linux_test.go`・`identity_other_test.go` の `time` import を削除する（`start` が唯一の利用箇所のため）。
- [ ] **ステップ10**: 残るテストの呼び出しとテスト関数名を改名後の名前に合わせる。`rg -n "AndMetrics" internal/` の全ヒット（実測 45 行）を対象とし、引数から `panicValue`・`duration` を落とす。改名するテスト関数は §1 の一覧を参照する。doc コメント中の言及も同時に直す。
- [ ] **ステップ11**: 削除前に `go test -tags test -coverprofile=/tmp/before.out ./internal/runner/base/privilege/` を、削除後に同様の `after.out` を取得し、`go tool cover -func` の差分が「削除した関数の消滅」のみであること、残存関数のカバレッジが低下していないことを確認する（CLAUDE.md「テストの削除は検証を要する主張」）。

**完了条件**:
- `make fmt` → `make test` → `make lint` がすべて通る。
- `GOOS=darwin GOARCH=arm64 go vet -tags test ./internal/runner/base/privilege/` が通る（Linux では未コンパイルの `identity_other_test.go` の追従漏れを検出するため。現行ツリーで exit 0 になることは確認済み）。
- `make deadcode` が本ステップに起因する新規の未使用シンボルを報告しない。
- `rg -n "Metrics" internal/runner/base/privilege/` が0件。

### Phase 2: 昇格・復元の失敗分岐をテストで踏む（F-002 / AC-07〜AC-13）

**対象ファイル**: `internal/runner/base/privilege/unix_privilege_test.go`

- [ ] **ステップ12**: `unix_privilege_test.go` の冒頭に、`cmd/runner/startup_privilege_test.go:3-13` に倣ったファイルコメントを英語で追加する。内容は、本ファイルの一部のテストがプロセス全体の identity 状態に依存するため `t.Parallel()` を呼んではならないこと、および root で実行すると失敗分岐を踏めず skip されること。
- [ ] **ステップ13**: `TestEscalatePrivileges` にサブテスト `seteuid_failure` を追加する。
  - [ ] `syscall.Getuid() == 0 || syscall.Geteuid() == 0` のとき `t.Skip("running as root: seteuid(0) succeeds and the native-root path returns early, so the failure path cannot be exercised")` とする（real UID が 0 だと `escalatePrivileges` が native root の早期リターンを通り、`seteuid` に到達しないため両方を条件にする）。
  - [ ] `privilegeSupported: true`・`originalUID: syscall.Getuid()` のマネージャを構築し、`escalatePrivileges` を呼ぶ。
  - [ ] 戻り値が `*Error` であることを `errors.AsType[*Error]` で確認し、`Operation`・`CommandName`・`OriginalUID`・`TargetUID`（= 0）が渡した値と一致すること、`SyscallErr` が非 nil の `syscall.Errno` であることを検証する（環境により `EPERM` 以外になり得るため errno 値は固定しない）。
  - [ ] 実行前後で `syscall.Geteuid()`・`syscall.Getegid()` が変化していないことを検証する。
- [ ] **ステップ14**: `TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` を追加する。
  - [ ] `syscall.Getuid() == 0 || syscall.Geteuid() == 0` のとき `t.Skip("running as root: seteuid to an arbitrary UID succeeds, so the restoration failure path cannot be exercised")` とする。
  - [ ] `originalUID: syscall.Getuid() + 1`（real・effective・saved のいずれとも異なるため `Seteuid` は必ず失敗する）、`identityVerifier` に `func() error { return nil }`、`readSavedIDs` に `func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported }` を設定したマネージャを構築する。`osExit` は既存の同種テスト（`:366-371` ほか）に倣い、呼び出しを記録したうえで panic するスタブとし、`assert.PanicsWithValue` で捕捉する。これにより `emergencyShutdown` の後に後続の検証ブロックへ進まないことも同時に固定する。
  - [ ] `needsPrivilegeEscalation: true`・`originalSUID: -1`・`originalSGID: -1` の `executionContext` で `restorePrivilegesAndVerify` を呼ぶ。
  - [ ] `osExit` が終了コード 1 でちょうど1回呼ばれたことを検証する。`originalUID` の選択理由（なぜ必ず失敗するか）をコメントで説明する。
  - [ ] 実行前後で `syscall.Geteuid()`・`syscall.Getegid()` が変化していないことを検証する。
- [ ] **ステップ15**: 両テストが主張どおりの理由で失敗できることを確認する。ステップ13 は `escalatePrivileges` の `Seteuid` 失敗時の `return &Error{...}` を `return nil` に一時変更してテストが失敗することを、ステップ14 は `restorePrivilegesAndVerify` の `m.emergencyShutdown(err, shutdownContext)` を一時的にコメントアウトしてテストが失敗することを確認し、確認した旨をコミットメッセージに記す。

**完了条件**:
- `make test` が通る。
- `go test -tags test -count=1 -run 'TestEscalatePrivileges|TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown' -v ./internal/runner/base/privilege/` の出力に `--- PASS: TestEscalatePrivileges/seteuid_failure` と `--- PASS: TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` が現れる（親テストの PASS ではなくサブテスト行を確認する。`-count=1` を付けてキャッシュされた結果を避ける）。
- `TestNoUnexpectedIdentityMutationSyscalls` が無変更のまま通る（同ガードは `_test.go` を走査対象から除外するため、新規テストが `syscall.Seteuid` を間接的に起こしてもガードには影響しない）。

### Phase 3: 再入禁止の契約の明記（F-003 / AC-14〜AC-17）

**対象ファイル**: `internal/runner/base/privilege/unix.go`, `internal/runner/base/runnertypes/config.go`

- [ ] **ステップ16**: `UnixPrivilegeManager.WithPrivileges` の doc コメントに、再入禁止の契約と、再入検出を実装しない理由を英語で追記する。既存の doc（`OperationUserGroupExecution` と `OperationFileValidation` の説明）は残し、その後ろに段落を足す。内容は次の3点を含める。
  - [ ] マネージャのミューテックスは `fn` の実行中も保持され続けること。特権状態がプロセス全体に及ぶため、直列化には保持が必要であること。
  - [ ] `fn` の内側から同一マネージャの `WithPrivileges` を呼ぶと自己デッドロックすること（reentrant / deadlock の語を用いる）。再入しないことは呼び出し側の責務であること。
  - [ ] 再入を検出してエラーを返す実装を置かない理由: 保持中フラグでは自ゴルーチンの再入と他ゴルーチンの正当な待機を区別できず、`TryLock` でも同様であるため。
- [ ] **ステップ17**: `internal/runner/base/runnertypes/config.go` の `PrivilegeManager` インターフェースの `WithPrivileges`（:194）に、再入が禁止であることを英語の doc コメントで記載する。実装の詳細（ミューテックス保持）ではなく、利用者から見た契約として書く。

**完了条件**:
- `make lint` が通る。
- 追加した doc がすべて英語である。

### Phase 4: 文書への反映（F-004 / AC-18〜AC-21）

**対象ファイル**: `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md`, `docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md`, `docs/dev/architecture_design/security-architecture{.ja,}.md`, `docs/dev/developer_guide/design-implementation-overview{.ja,}.md`, `docs/user/security-risk-assessment{.ja,}.md`

- [ ] **ステップ18**: `98_remaining_issues.md` §2 の「### A1（privilege）」を書き換える。同節は現在 L-2・L-3・L-4 の3つの箇条書きと #977 への参照行だけで構成されており、3件すべてが本タスクで解消するため、箇条書きは残らない。同文書が D1 M-3・B3 M2 に既に用いている引用ブロック形式（`> **D1 M-3 について**: …`、:15-17）に倣い、`> **A1 L-2/L-3/L-4 について**: …` として次を記す。
  - [ ] L-3 の恒偽項は 0157 で解消済みであり、本タスクは残る2点（未昇格時の失敗計上・指標名と実態の乖離）を `Metrics` 型の削除で解消したこと。同時に L-5（一覧には未掲載）も対象物の消滅により解消したこと。
  - [ ] L-2 を所見の推奨とは逆方向で close したことと、その根拠（`identity_mutation_guard_test.go` が identity 変更関数の値参照を禁じており、注入フィールドの追加はこの不変条件を崩す。失敗分岐は実 `EPERM` を用いたテストで踏んだ）。
  - [ ] L-4 は doc への契約明記で対応し、再入検出は Go では正しく実装できないため見送ったこと。
  - [ ] 本タスク（0166）と #977 への参照。
- [ ] **ステップ19**: ステップ18 の書き換えが A1 以外の残件に波及していないことを、`git diff` で確認する。§1・§2・§3 の D1・B1・D2・E1 ほかの記述に増減がないこと。なお A1 の M-1・L-1・I-1〜I-4 は元から同文書に掲載されていないため、本タスクでは新規追加しない（掲載範囲の見直しは本タスクの対象外）。
- [ ] **ステップ20**: `findings/A1_privilege.md` の L-2・L-3・L-4・L-5 の各項目末尾に「**対応状況**:」で始まる1行を追記する。所見の原文（監査時点の記述）は書き換えない。この追記形式は `findings/*.md` では本タスクが最初の導入となるため、他の findings と揃えるかどうかは後続タスクの判断に委ねる。
- [ ] **ステップ21**: 改名の影響を受ける文書3対を更新する。§1「改名の影響を受ける文書」の表の6ファイルが対象。
  - [ ] 日本語版3ファイル（`security-architecture.ja.md`・`design-implementation-overview.ja.md`・`security-risk-assessment.ja.md`）のコード引用と説明文を、改名後の名称に更新する。
  - [ ] `security-architecture.ja.md:316` の「パニック回復と時間計測を担い」を、時間計測が無くなった実態に合わせて書き換える。
  - [ ] 日本語版をコミットしたうえで、英語版3ファイルを `/mktrans` で反映する（CLAUDE.md のバイリンガル文書の編集順序）。

**完了条件**:
- 日本語文書の記述スタイル（太字は構造ラベルのみ、平易な語を優先）に沿っている。
- `rg -n "AndMetrics" docs/` のヒットが、本タスクおよび 0149・0157 の作業文書（`docs/tasks/` 配下の履歴的記述）のみになる。

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 対応 Phase | 成果物 |
|---|---|---|---|
| M1 | metrics の削除完了 | Phase 1 | `Metrics` 型が存在せず、`make test`・`make lint`・`make deadcode`・darwin クロス vet が通る状態 |
| M2 | 失敗分岐のテスト追加完了 | Phase 2 | 昇格・復元の syscall 失敗分岐と `emergencyShutdown` を踏むテストが非 root 環境で実行される状態 |
| M3 | 契約と文書の整備完了 | Phase 3・4 | 再入禁止が doc に明記され、0149 の残件一覧と設計文書3対が本タスクの結果を反映した状態 |

Phase 1 → Phase 2 の順序には依存がある（Phase 2 で追加するテストは改名後の関数名を使う）。Phase 4 のステップ21（設計文書の改名追従）も Phase 1 の改名確定に依存する。Phase 3 は他と独立している。

PR は M1〜M3 をまとめて1本とする。変更が privilege package とその参照文書に閉じており、レビュー時に「metrics 削除で失われた検証がテスト追加で補われている」ことを一度に見せられるためである。ただしステップ21 の英語版反映は、日本語版のコミット後に `/mktrans` で行うため、同一 PR 内の別コミットとする。

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
| root 環境（CI 設定の変更、コンテナでの実行）で Phase 2 のテストが常に skip され、踏んだつもりで踏めていない | 中 | 完了条件でサブテスト行が `--- PASS` であることを確認する。CI（`ubuntu-latest`、`container:` 指定なし）・開発コンテナがいずれも非 root であることは確認済み。ファイルコメントにも root CI ではこの検証が失われる旨を記す |
| Phase 2 のテストが実際にプロセスの identity を変えてしまう | 高 | 失敗する UID のみを渡す。Go の `doAllThreadsSyscall` は起動スレッドの errno が非ゼロなら他スレッドへ伝播させずに戻るため、失敗時に identity は変化しない。加えて各テストが実行前後の実効 UID・GID の一致を検証するため、万一変化した場合はテストが失敗して検知できる |
| 失敗時の errno が環境により `EPERM` 以外になる（uid 65534 の `+1`、user namespace の未マップ uid） | 低 | errno 値を固定せず、非 nil の `syscall.Errno` であることのみを検証する |
| metrics 削除で、既存テストが暗黙に担保していた挙動が落ちる | 中 | §1 の表で不変条件ごとに引き受け先を明示し、ステップ11 でカバレッジの差分を確認する |
| 改名の取りこぼしが、テスト関数名・doc コメント・設計文書に残る（いずれもコンパイルエラーにならない） | 中 | ステップ10・21 で `rg -n "AndMetrics"` の全ヒットを対象とし、AC-04・AC-21 で残存ゼロを確認する |
| Linux でコンパイルされない `identity_other_test.go` の追従漏れ | 中 | Phase 1 完了条件の `GOOS=darwin` クロス vet |

## 6. 実装チェックリスト

### Phase 1: metrics の削除
- [ ] ステップ1: `metrics.go` の削除と package コメントの移設
- [ ] ステップ2: `metrics_test.go` の削除
- [ ] ステップ3: `unix.go` からの metrics 除去
- [ ] ステップ4: `unix.go` の関数改名
- [ ] ステップ5: `manager.go` の `GetMetrics` 削除
- [ ] ステップ6: `testutil/mocks.go` の `GetMetrics` と import の削除
- [ ] ステップ7: `race_test.go` の該当行削除
- [ ] ステップ8: 主張が消える4テストとアサーション1件の削除
- [ ] ステップ9: テスト側 `start:` の削除と不要 import の除去
- [ ] ステップ10: 残存テストの改名・引数追従
- [ ] ステップ11: カバレッジ差分の確認

### Phase 2: 失敗分岐をテストで踏む
- [ ] ステップ12: ファイルコメント（`t.Parallel` 禁止・root skip）の追加
- [ ] ステップ13: 昇格失敗テストの追加
- [ ] ステップ14: 復元失敗テストの追加
- [ ] ステップ15: 両テストが理由どおりに失敗できることの確認

### Phase 3: 再入禁止の明記
- [ ] ステップ16: `WithPrivileges` の doc 追記
- [ ] ステップ17: `runnertypes.PrivilegeManager` の doc 追記

### Phase 4: 文書への反映
- [ ] ステップ18: `98_remaining_issues.md` §2 A1 の書き換え
- [ ] ステップ19: 他の残件に波及していないことの確認
- [ ] ステップ20: `findings/A1_privilege.md` への対応状況の追記
- [ ] ステップ21: 設計文書3対の改名追従と `/mktrans`

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
| AC-13 | manual | ステップ15 の手順で各分岐を無効化してテストが失敗することを確認し、コミットメッセージに記載する。AC-07・AC-08 の `test` 検証を補完するものであり、単独の検証手段ではない |
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

- 本計画書のレビューと `approved` への更新（レビュー後）。
- 実装（Phase 1 → Phase 4）と PR 作成。PR 説明に、L-2 を所見の推奨とは逆方向で close した理由を記載する。
- マージ後に #977 を close する。#1041（`resource.Manager.WithPrivileges` の削除）は本タスクとは独立に扱う。
