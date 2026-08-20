# 実装計画書: privilege の残 Low 所見（metrics 削除・失敗経路の被覆・再入契約の明記）

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
- `panicValue` は metrics 記録以外にも使う（panic 再送出、`shutdownContext` の文言）ため残す。`restorePrivilegesAndMetrics` の `panicValue` 引数は、metrics 削除後は「成功記録の抑止」という唯一の用途を失う。**呼び出し側 `handleCleanup` が `shutdownContext` を組み立てて渡しているため、`panicValue` 引数も削除できる**（§2.1 ステップ3）。
- `escalatePrivileges` の失敗分岐（:270-279）は `*Error` を返す。`restorePrivileges` の失敗分岐（:300-302）は生の syscall エラーを返し、呼び出し側で `emergencyShutdown` に渡される。いずれも既存テストが踏んでいない。

#### `internal/runner/base/privilege/manager.go`（変更）

`Manager` インターフェースの `GetMetrics() Metrics`（:16）を削除する。同インターフェースの `GetCurrentUID`・`GetOriginalUID` も本番の呼び出し元を持たないが、本タスクの対象外（[01_requirements.md](01_requirements.md) 決定事項）。

#### `internal/runner/base/privilege/testutil/mocks.go`（変更）

`MockPrivilegeManager.GetMetrics`（:76-78）を削除する。他に metrics 参照はない。

#### 既存テストの扱い

metrics 削除により**主張が消えるテスト**が4つある。いずれも「metrics に何が記録されたか」以外を検証していないため削除するが、それぞれの不変条件が残存テストで担保されることを以下に示す。

| 削除するテスト | 検証していた不変条件 | 削除後の担保 |
|---|---|---|
| `TestHandleCleanupAndMetrics_Success`（unix_privilege_test.go:156） | 非 panic 時に非ゼロの duration が計算され下位へ渡ること | duration 自体を削除するため不変条件が消滅する |
| `TestRestorePrivilegesAndMetrics_Success`（:218） | 昇格した操作の復元後に成功が記録されること | 「復元が走り `emergencyShutdown` が起きないこと」は `TestRestorePrivilegesAndVerify_SavedSetUnchanged_Passes`（改名後）が `osExit` の未呼び出しで担保する |
| `TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalation`（:250） | 未昇格の操作で成功が記録されないこと | 「未昇格なら復元・検証をしない」は `TestRestorePrivilegesAndVerify_IdentityVerificationSkippedWithoutEscalation`（改名後）が `verifierCalled == false` で担保する |
| `TestRestorePrivilegesAndMetrics_Failure`（:273） | panic 時に成功が記録されないこと。名前に反し復元自体は失敗させていない | 復元失敗の分岐は Phase 2 の新規テスト `TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` が初めて実際に踏む |

`metrics_test.go` は全体を削除する。同ファイルが定義する `ErrTestPrivilegeElevationFailure`・`ErrTestFailure`・`ErrTestError` は同ファイル内でのみ参照されており、他への影響はない（確認済み）。

`race_test.go:235` の `_ = manager.GetMetrics()` は、並行読み取りの対象の1つとして呼ばれている。同じループ内に `GetCurrentUID`・`GetOriginalUID`・`IsPrivilegedExecutionSupported` の3つが残るため、当該行の削除のみでテストの主旨は保たれる。

改名対象の関数を参照するテストは、`unix_privilege_test.go`（16箇所）・`identity_linux_test.go`（3箇所）・`identity_other_test.go`（2箇所）にある。テスト関数名自体も `TestRestorePrivilegesAndMetrics_*` を含むため、併せて改名する。

#### 失敗分岐を踏むための前提（Phase 2）

- `syscall.Seteuid(x)` は、非特権プロセスでは `x` が real・effective・saved のいずれかに一致する場合のみ成功する。テストバイナリは setuid ビットを持たないため3つとも `syscall.Getuid()` に等しく、`0` および `syscall.Getuid() + 1` はどれとも一致しない。失敗時は identity を変更しない。
- 既存の同型のテストが `cmd/runner/startup_privilege_test.go:30-46` にある（`TestDropStartupPrivileges_FailsClosedOnSetegidFailure`）。root skip・実行前後の identity 比較を含めて同じ形に揃える。
- CI は `ubuntu-latest`（GitHub ホストランナー、非 root）、開発コンテナも非 root（uid 1000）である。したがって root skip は保険であり、通常は skip されずに実行される。

## 2. 実装ステップ

### Phase 1: `privilege.Metrics` の削除（F-001 / AC-01〜AC-06）

**対象ファイル**: `internal/runner/base/privilege/metrics.go`, `metrics_test.go`, `manager.go`, `unix.go`, `testutil/mocks.go`, `unix_privilege_test.go`, `identity_linux_test.go`, `identity_other_test.go`, `race_test.go`

- [ ] **ステップ1**: `internal/runner/base/privilege/metrics.go` を削除する。package コメント `// Package privilege provides metrics collection for privilege operations.` は失われるため、`manager.go` の `package privilege` の直前に `// Package privilege manages elevation to root and restoration of the original privileges for operations that require them.` を追加する。
- [ ] **ステップ2**: `internal/runner/base/privilege/metrics_test.go` を削除する。
- [ ] **ステップ3**: `unix.go` から metrics を取り除く。
  - [ ] `UnixPrivilegeManager.metrics Metrics` フィールドを削除する。
  - [ ] `WithPrivileges` の `m.metrics.RecordElevationFailure(err)` 2箇所を削除する。
  - [ ] `restorePrivilegesAndMetrics` の `m.metrics.RecordElevationSuccess(duration)` を削除する。これにより `if err := m.restorePrivileges(); err != nil { m.emergencyShutdown(...) }` の `else if` 節がなくなる。
  - [ ] `GetMetrics` メソッドを削除する。
  - [ ] `executionContext.start` フィールドと、`handleCleanupAndMetrics` 内の `duration` の宣言・計算を削除する。
  - [ ] `restorePrivilegesAndMetrics` のシグネチャから `duration time.Duration` と `panicValue any` を削除し、`(execCtx *executionContext, shutdownContext string)` にする。`panicValue` は呼び出し側 `handleCleanup` でのみ使われる状態になる。
  - [ ] `prepareExecution` の `start: time.Now()` の設定を削除する。`time` の import は `Error{Timestamp: time.Now()}` で引き続き使うため残す。
- [ ] **ステップ4**: `unix.go` の関数を改名する。`handleCleanupAndMetrics` → `handleCleanup`、`restorePrivilegesAndMetrics` → `restorePrivilegesAndVerify`。doc コメントの「and metrics recording」に相当する記述も実態に合わせて書き換える（`// handleCleanup recovers from a panic in the callback, restores privileges, and verifies identity.` / `// restorePrivilegesAndVerify restores the original privileges and verifies that no elevated identity leaked.`）。
- [ ] **ステップ5**: `manager.go` の `Manager` インターフェースから `GetMetrics() Metrics` を削除する。
- [ ] **ステップ6**: `testutil/mocks.go` から `MockPrivilegeManager.GetMetrics` とその doc コメントを削除する。
- [ ] **ステップ7**: `race_test.go` の `_ = manager.GetMetrics()`（:235）の行を削除する。
- [ ] **ステップ8**: 主張が消える既存テストを削除する（§1「既存テストの扱い」の表の4件。それぞれ個別に確認する）。
  - [ ] `TestHandleCleanupAndMetrics_Success`
  - [ ] `TestRestorePrivilegesAndMetrics_Success`
  - [ ] `TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalation`
  - [ ] `TestRestorePrivilegesAndMetrics_Failure`
- [ ] **ステップ9**: 残るテストの呼び出しとテスト関数名を改名後の名前に合わせる。`rg -n "restorePrivilegesAndMetrics|handleCleanupAndMetrics|RestorePrivilegesAndMetrics|HandleCleanupAndMetrics" internal/` の全ヒットを対象とし、引数から `panicValue`・`duration` を落とす。`TestHandleCleanupAndMetrics_WithError` は `TestHandleCleanup_WithError` とする。
- [ ] **ステップ10**: 削除前に `go test -tags test -coverprofile=/tmp/before.out ./internal/runner/base/privilege/` を、削除後に同様の `after.out` を取得し、`go tool cover -func` の差分が「削除した関数の消滅」のみであること、残存関数のカバレッジが低下していないことを確認する（CLAUDE.md「テストの削除は検証を要する主張」）。

**完了条件**:
- `make fmt` → `make test` → `make lint` がすべて通る。
- `make deadcode` が本ステップに起因する新規の未使用シンボルを報告しない。
- `rg -n "Metrics" internal/runner/base/privilege/ internal/runner/resource/ internal/runner/base/executor/` の結果が、executor 側の `audit.PrivilegeMetrics` 関連のみになる。

### Phase 2: 昇格・復元の失敗分岐のテスト被覆（F-002 / AC-07〜AC-13）

**対象ファイル**: `internal/runner/base/privilege/unix_privilege_test.go`

- [ ] **ステップ11**: `TestEscalatePrivileges` にサブテスト `seteuid_failure` を追加する。
  - [ ] `syscall.Geteuid() == 0` のとき `t.Skip("running as root: seteuid(0) succeeds, so the failure path cannot be exercised")` とする。
  - [ ] `privilegeSupported: true`・`originalUID: syscall.Getuid()`（0 以外）のマネージャを構築し、`escalatePrivileges` を呼ぶ。
  - [ ] 戻り値が `*Error` であることを `errors.AsType[*Error]` で確認し、`Operation`・`CommandName`・`OriginalUID`・`TargetUID`（= 0）が渡した値と一致すること、`SyscallErr` が `syscall.EPERM` であることを検証する。
  - [ ] 実行前後で `syscall.Geteuid()`・`syscall.Getegid()` が変化していないことを検証する。
- [ ] **ステップ12**: `TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` を追加する。
  - [ ] `syscall.Geteuid() == 0` のとき `t.Skip("running as root: seteuid to an arbitrary UID succeeds, so the restoration failure path cannot be exercised")` とする。
  - [ ] `originalUID: syscall.Getuid() + 1`（real・effective・saved のいずれとも異なるため `Seteuid` は必ず `EPERM`）、`osExit` に呼び出し記録用の関数、`identityVerifier` に `func() error { return nil }`、`readSavedIDs` に `func() (int, int, error) { return -1, -1, ErrSavedSetNotSupported }` を設定したマネージャを構築する。
  - [ ] `needsPrivilegeEscalation: true`・`originalSUID: -1`・`originalSGID: -1` の `executionContext` で `restorePrivilegesAndVerify` を呼ぶ。
  - [ ] `osExit` が終了コード 1 で呼ばれたことを検証する。`originalUID` の選択理由（なぜ必ず `EPERM` になるか）をコメントで説明する。
  - [ ] 実行前後で `syscall.Geteuid()`・`syscall.Getegid()` が変化していないことを検証する。
- [ ] **ステップ13**: 両テストが主張どおりの理由で失敗できることを確認する。ステップ11 は `escalatePrivileges` の `Seteuid` 失敗時の `return &Error{...}` を `return nil` に一時変更してテストが失敗することを、ステップ12 は `restorePrivilegesAndVerify` の `m.emergencyShutdown(err, shutdownContext)` を一時的にコメントアウトしてテストが失敗することを確認し、確認した旨をコミットメッセージに記す。

**完了条件**:
- `make test` が通り、追加した2つのテストが（非 root 環境で）skip されずに実行される。`go test -tags test -run 'TestEscalatePrivileges|TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown' -v ./internal/runner/base/privilege/` の出力で `--- PASS` を確認する（`--- SKIP` ではないこと）。
- `TestNoUnexpectedIdentityMutationSyscalls` が無変更のまま通る。

### Phase 3: 再入禁止の契約の明記（F-003 / AC-14〜AC-17）

**対象ファイル**: `internal/runner/base/privilege/unix.go`, `internal/runner/base/runnertypes/config.go`

- [ ] **ステップ14**: `UnixPrivilegeManager.WithPrivileges` の doc コメントに、再入禁止の契約と、再入検出を実装しない理由を英語で追記する。既存の doc（`OperationUserGroupExecution` と `OperationFileValidation` の説明）は残し、その後ろに段落を足す。内容は次の3点を含める。
  - [ ] マネージャのミューテックスは `fn` の実行中も保持され続けること。特権状態がプロセス全体に及ぶため、直列化には保持が必要であること。
  - [ ] `fn` の内側から同一マネージャの `WithPrivileges` を呼ぶと自己デッドロックすること。再入しないことは呼び出し側の責務であること。
  - [ ] 再入を検出してエラーを返す実装を置かない理由: 保持中フラグでは自ゴルーチンの再入と他ゴルーチンの正当な待機を区別できず、`TryLock` でも同様であるため。
- [ ] **ステップ15**: `internal/runner/base/runnertypes/config.go` の `PrivilegeManager` インターフェースの `WithPrivileges`（:194）に、再入が禁止であることを英語の doc コメントで記載する。実装の詳細（ミューテックス保持）ではなく、利用者から見た契約として書く。

**完了条件**:
- `make lint` が通る。
- 追加した doc がすべて英語である。

### Phase 4: 監査文書への反映（F-004 / AC-18〜AC-20）

**対象ファイル**: `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md`, `docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md`

- [ ] **ステップ16**: `98_remaining_issues.md` §2 の A1 の項目を書き換える。L-2・L-3・L-4 を残件一覧から外し、本タスク（0166）と #977 への参照とともに対応結果を記す。L-2 については「所見の推奨（注入フィールドを実経路で使う）とは逆方向で close した」ことと、その根拠（`identity_mutation_guard_test.go` が identity 変更関数の値参照を禁じていること、失敗分岐は EPERM を用いた実テストで被覆したこと）が読み取れる記述にする。L-5 は対象物（`Metrics` 型）の消滅により解消した旨を記す。
- [ ] **ステップ17**: 同じ §2 に、A1 の未対応所見（L-1）が残っていること、および §1・§3 に相当する M-1・I-1〜I-4 の扱いが変わっていないことを確認する。誤って落ちていないことを、書き換え前後の A1 関連記述の差分で確認する。
- [ ] **ステップ18**: `findings/A1_privilege.md` の L-2・L-3・L-4・L-5 の各項目末尾に「対応状況」の1行を追記する。所見の原文（監査時点の記述）は書き換えない。L-3 については、恒偽項そのものは 0157 で解消済みであり、本タスクは残る2点を `Metrics` 型の削除によって解消したことを明記する。

**完了条件**:
- 日本語文書の記述スタイル（太字は構造ラベルのみ、平易な語を優先）に沿っている。
- 英訳対象の文書ではない（`docs/tasks/` 配下の作業文書のため `/mktrans` は不要）。

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 対応 Phase | 成果物 |
|---|---|---|---|
| M1 | metrics の削除完了 | Phase 1 | `Metrics` 型が存在せず、`make test`・`make lint`・`make deadcode` が通る状態 |
| M2 | 失敗分岐の被覆完了 | Phase 2 | 昇格・復元の syscall 失敗分岐と `emergencyShutdown` を踏むテストが非 root 環境で実行される状態 |
| M3 | 契約と文書の整備完了 | Phase 3・4 | 再入禁止が doc に明記され、0149 の残件一覧が本タスクの結果を反映した状態 |

Phase 1 → Phase 2 の順序には依存がある（Phase 2 で追加するテストは改名後の関数名を使う）。Phase 3・4 は Phase 1・2 と独立しており、順序を入れ替えてよい。

PR は M1〜M3 をまとめて1本とする。変更が privilege package に閉じており、レビュー時に「metrics 削除で失われた検証がテスト追加で補われている」ことを一度に見せられるためである。

## 4. テスト戦略

### 単体テスト

- **新規**: 昇格失敗（`escalatePrivileges` の `Seteuid(0)` が `EPERM`）と復元失敗（`restorePrivileges` の `Seteuid` が `EPERM`）の2分岐。いずれも実際に syscall を呼んで失敗させる。モック・注入点は追加しない。
- **削除**: metrics の記録内容のみを検証していた4テストと `metrics_test.go` 全体。§1 の表のとおり、消える不変条件は duration 計算（対象そのものが消滅）のみで、他は残存テストが担保する。
- **維持**: identity 検証・saved-set 検証・panic 経路・並行アクセスの各テストは、改名と引数変更への追従のみを行い、検証内容は変えない。

### 回帰の観点

- 特権昇格・復元の外部から観測できる挙動（`WithPrivileges` の戻り値、`emergencyShutdown` の発火条件、panic の再送出）は変えない。既存テストがこれらを担保する。
- 監査ログの特権指標（`elevation_count`・`total_privilege_duration_ms`）は executor 側の `audit.PrivilegeMetrics` に由来し、本タスクでは触らない。`internal/runner/base/executor` と `internal/runner/base/audit` のテストが無変更で通ることで確認する。

### 静的検査

- `TestNoUnexpectedIdentityMutationSyscalls` を無変更で通す。これが本タスクの中心的な安全確認であり、production の identity 変更箇所が増減していないことを機械的に示す。

## 5. リスク管理

| リスク | 影響 | 対策 |
|---|---|---|
| root 環境（CI 設定の変更、コンテナでの実行）で Phase 2 のテストが常に skip され、被覆したつもりで被覆できていない | 中 | 完了条件に「`--- SKIP` ではなく `--- PASS` であること」を明示する。CI（`ubuntu-latest`）・開発コンテナがいずれも非 root であることは確認済み |
| Phase 2 のテストが実際にプロセスの identity を変えてしまう | 高 | `EPERM` で失敗する UID のみを渡す。加えて各テストが実行前後の実効 UID・GID の一致を検証するため、万一変化した場合はテストが失敗して検知できる |
| metrics 削除で、既存テストが暗黙に担保していた挙動が落ちる | 中 | §1 の表で不変条件ごとに引き受け先を明示し、ステップ10 でカバレッジの差分を確認する |
| 改名の取りこぼしでテストの意図と名前が乖離する | 低 | ステップ9 で `rg` の全ヒットを対象とする。取りこぼしはコンパイルエラーになる（テスト関数名の取りこぼしのみコンパイルを通るため、AC 検証で `rg` を用いる） |

## 6. 実装チェックリスト

### Phase 1: metrics の削除
- [ ] ステップ1: `metrics.go` の削除と package コメントの移設
- [ ] ステップ2: `metrics_test.go` の削除
- [ ] ステップ3: `unix.go` からの metrics 除去
- [ ] ステップ4: `unix.go` の関数改名
- [ ] ステップ5: `manager.go` の `GetMetrics` 削除
- [ ] ステップ6: `testutil/mocks.go` の `GetMetrics` 削除
- [ ] ステップ7: `race_test.go` の該当行削除
- [ ] ステップ8: 主張が消える4テストの削除
- [ ] ステップ9: 残存テストの改名・引数追従
- [ ] ステップ10: カバレッジ差分の確認

### Phase 2: 失敗分岐の被覆
- [ ] ステップ11: 昇格失敗テストの追加
- [ ] ステップ12: 復元失敗テストの追加
- [ ] ステップ13: 両テストが理由どおりに失敗できることの確認

### Phase 3: 再入禁止の明記
- [ ] ステップ14: `WithPrivileges` の doc 追記
- [ ] ステップ15: `runnertypes.PrivilegeManager` の doc 追記

### Phase 4: 監査文書への反映
- [ ] ステップ16: `98_remaining_issues.md` §2 A1 の書き換え
- [ ] ステップ17: 未対応所見が落ちていないことの確認
- [ ] ステップ18: `findings/A1_privilege.md` への対応状況の追記

### 全体
- [ ] `make fmt` → `make test` → `make lint` がすべて通る
- [ ] `make deadcode` が新規の未使用シンボルを報告しない
- [ ] すべての AC が §7 の方法で検証済み

## 7. Acceptance Criteria 検証

| AC | 種別 | 検証方法 |
|---|---|---|
| AC-01 | static | `rg -n "type Metrics|RecordElevationSuccess|RecordElevationFailure|updateSuccessRate|GetSnapshot|func \(m \*Metrics\)" internal/runner/base/privilege/` が0件。加えて `test -e internal/runner/base/privilege/metrics.go` が失敗する |
| AC-02 | static | `rg -n "GetMetrics" internal/ cmd/` が0件 |
| AC-03 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity`、`::TestHandleCleanup_WithError`、`::TestRestorePrivilegesAndVerify_IdentityLeakTriggersShutdown`、`::TestRestorePrivilegesAndVerify_SavedSetChanged_TriggersShutdown`、`internal/runner/base/privilege/manager_test.go::TestManager_WithPrivileges_UserGroup_FunctionError` が無変更の検証内容で通る |
| AC-04 | static | `rg -n "execCtx\.start|start:\s+time\.Now\(\)|duration" internal/runner/base/privilege/unix.go` が0件 |
| AC-05 | static + test | `git diff --exit-code internal/runner/base/audit/ internal/runner/base/executor/executor.go` が差分なし（指標の生成元に手を入れていない）。加えて `internal/runner/base/audit/logger_test.go::TestLogger_LogUserGroupExecution` が無変更で通る（`audit.PrivilegeMetrics{ElevationCount: 1, ...}` を入力とする監査ログ出力を検証している） |
| AC-06 | static | `make deadcode` の出力に `internal/runner/base/privilege` 由来の新規項目がない（実施前後の出力を比較する） |
| AC-07 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestEscalatePrivileges/seteuid_failure` |
| AC-08 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown` |
| AC-09 | test | 上記2テスト内の実効 UID・GID の実行前後比較アサーション |
| AC-10 | test | 上記2テストの `t.Skip` 分岐。`sudo -n go test -tags test -run 'TestEscalatePrivileges|TestRestorePrivilegesAndVerify_RestoreFailureTriggersShutdown' -v ./internal/runner/base/privilege/` を実行できる環境では skip されること、非 root では `--- PASS` になることを確認する |
| AC-11 | static | `internal/runner/base/privilege/identity_mutation_guard_test.go::TestNoUnexpectedIdentityMutationSyscalls` が通る（identity 変更関数の値参照・フィールド代入を検出して失敗させる）。加えて `rg -n "func\(uid int\) error|func\(gid int\) error" internal/runner/base/privilege/*.go`（`_test.go` を除く）が0件 |
| AC-12 | static | `git diff --exit-code internal/runner/base/privilege/identity_mutation_guard_test.go` が差分なし、かつ `go test -tags test -run TestNoUnexpectedIdentityMutationSyscalls ./internal/runner/base/privilege/` が PASS |
| AC-13 | manual | ステップ13 の手順で各分岐を無効化してテストが失敗することを確認し、コミットメッセージに記載する（AC-07・AC-08 の test 検証を補完する） |
| AC-14 | static | `rg -n -A 20 "func \(m \*UnixPrivilegeManager\) WithPrivileges" internal/runner/base/privilege/unix.go` の直前 doc に `reentr`（reentrant/reentrancy）・`deadlock`・`mutex` の各語が現れる |
| AC-15 | static | 同じ doc に `TryLock` と、ゴルーチンを区別できない旨の記述が現れる |
| AC-16 | static | `rg -n -B 10 "WithPrivileges\(elevationCtx ElevationContext" internal/runner/base/runnertypes/config.go` の doc に `reentr` が現れる |
| AC-17 | static | `rg -n "[ぁ-んァ-ヶ一-龠]" internal/runner/base/privilege/*.go internal/runner/base/runnertypes/config.go` が0件 |
| AC-18 | static | `rg -n "A1" docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` の §2 該当箇所に L-2・L-3・L-4 が残件として列挙されておらず、0166 と #977 への参照がある |
| AC-19 | static | `rg -n "対応状況" docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md` が L-2・L-3・L-4・L-5 の4箇所でヒットする |
| AC-20 | manual | ステップ17 の差分確認。A1 の M-1・L-1 と I-1〜I-4 の記述が、本タスクの書き換えで削除されていないこと |

## 8. Success Criteria

- 上記すべての AC が §7 の方法で検証済みである。
- `make test` と `make lint` が通る。
- `TestNoUnexpectedIdentityMutationSyscalls` が無変更のまま通り、production の identity 変更箇所が `escalatePrivileges` の `Seteuid(0)` と `restorePrivileges` の `Seteuid(m.originalUID)` の2箇所のままである。
- 特権昇格・復元・emergency shutdown の外部から観測できる挙動が本タスクの前後で変わらない。
- #977 の3件それぞれについて、解消したのか所見の推奨とは異なる形で close したのかが、コードと監査文書の双方から追える。

## 9. 次のステップ

- 本計画書のレビューと `approved` への更新（レビュー後）。
- 実装（Phase 1 → Phase 4）と PR 作成。PR 説明に、L-2 を所見の推奨とは逆方向で close した理由を記載する。
- マージ後に #977 を close する。#1041（`resource.Manager.WithPrivileges` の削除）は本タスクとは独立に扱う。
