# 要件定義書: privilege の残 Low 所見（metrics 削除・失敗経路のテスト追加・再入契約の明記）

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-08-20 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#977 [Security][A1 L-2/L-3/L-4] privilege: テスト注入構造・恒偽metrics・再入デッドロック](https://github.com/isseis/go-safe-cmd-runner/issues/977)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md](../0149_security_code_smell_audit_fable/findings/A1_privilege.md) の L-2・L-3・L-4（および本タスクで併せて解消する L-5）
- 残件一覧: [docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §2 A1
- 先行タスク: [0157 デッドコードと命名の整理](../0157_dead_code_naming_cleanup/01_requirements.md)（A1 M-2 を対応し、本タスクの前提を作った）
- 本タスクの対象外として分離した事項: [#1041 [Refactor] resource.Manager.WithPrivileges が本番未使用かつ呼べば必ず失敗する死んだ API](https://github.com/isseis/go-safe-cmd-runner/issues/1041)

## 背景

0149 監査の所見 A1（`internal/runner/base/privilege`）のうち、Medium 2件は 0157 で対応したが、Low 5件のうち L-2・L-3・L-4 が未着手のまま残っている。2026-08-20 時点で現行コードと照合したところ、0157 による前提の変化が大きく、所見をそのまま実装できる状態ではないことが分かった。以下に3件それぞれの現状を示す。

### L-2: 所見の推奨が、0157 以降の設計上の不変条件と正面から衝突する

所見 L-2 は「昇格・復元でもテスト注入フィールド（`syscallSeteuid`／`syscallSetegid`）を使い、復元失敗時の `emergencyShutdown` 経路をモックでテストする」ことを推奨していた。0157 はこれとは逆に、到達不能な実降格パスもろとも両フィールドを削除した（AC-13）。

さらにその後、`internal/runner/base/privilege/identity_mutation_guard_test.go` の `TestNoUnexpectedIdentityMutationSyscalls` が導入されている。この静的ガードは、

- identity を変更する syscall（`Seteuid`・`Setegid`・`Setresuid`・`Setgroups`・`Prctl`・生 `Syscall` 系ほか）の呼び出しを、`escalatePrivileges` の `Seteuid(0)` と `restorePrivileges` の `Seteuid(m.originalUID)` の2箇所に、**関数名・syscall 名・引数式の組**まで固定して allowlist し、
- 加えて、これらの関数を**値として参照すること（構造体フィールドへの代入を明示的な例として挙げている）を禁止**している。値として保持されると、その先の間接呼び出しを AST 走査で追えなくなるためである。

つまり L-2 が推奨する「注入フィールド経由で seteuid を呼ぶ」形は、現在このガードが名指しで禁じている形そのものである。ガードの doc コメントは、この静的検査が「非特権 CI では降格 syscall の EPERM 失敗と未呼び出しを区別できない」という動的テストの限界を埋めるために置かれたことを明記しており、L-2 が問題視したテスト容易性の欠如は、別手段で意図的に補償された状態にある。

一方で、L-2 が指摘した実害そのもの——昇格・復元の syscall 失敗分岐と、そこから到達する `emergencyShutdown` が単体テストで踏まれていない——は現存する。現行テストは `escalatePrivileges` について `not_supported`（`privilegeSupported: false`）と `native_root`（`originalUID == 0`）しか通しておらず、`syscall.Seteuid` が実際に失敗する分岐には入らない。`restorePrivileges` の失敗から `emergencyShutdown` に至る経路も同様である。

ただしこれは注入点を作らなくてもテストで踏める。非特権プロセスでは `Seteuid` に自分の real／effective／saved のいずれとも異なる UID を渡せば `EPERM` で失敗し、そのとき identity は変化しない。0162 が `cmd/runner/startup_privilege_test.go` で採っているのと同じ手法であり、同ファイルは root 実行時に `t.Skip` するガードも備えている。CI（`ubuntu-latest`）も開発コンテナも非 root で動くため、この skip は保険であって通常経路ではない。

### L-3: 恒偽項そのものは 0157 で解消済み。残るのは metrics の意味論と、型の存在価値

所見が指した `restorePrivilegesAndMetrics` の条件式（旧 `unix.go:232`）は、0157 が `needsUserGroupChange` を廃止したことに伴い `} else if panicValue == nil {` へ置き換わっており、恒偽項は現存しない。残っているのは所見の後半2点と、今回新たに確認した1点である。

1. `prepareExecution` の失敗（operation 不正・saved-set 読み取り失敗）が `RecordElevationFailure` として計上される。昇格を試みていないのに「昇格失敗」になる。
2. 記録される `duration` は `fn()`（コマンドの全実行時間）を含むのに、指標名は `AverageElevationTime`・`MaxElevationTime`・`TotalElevationTime` である。名前と実態が乖離している。
3. **`GetMetrics()` には本番の読み取り側が1つも存在しない。** 参照は `privilege.Manager` インターフェースの定義と、テスト・モックのみである。監査ログが実際に使っているのは executor 側の別勘定 `audit.PrivilegeMetrics`（executor が `WithPrivileges` 呼び出しの前後で計測し、`elevation_count` と `total_privilege_duration_ms` として出力する）であり、しかも「`WithPrivileges` 全体の所要時間」という同じものを測っている。

読み手のいない指標の意味論を整えても得るものがないため、本タスクは `privilege.Metrics` 自体を削除する方向を採る。0157 が「本番呼び出し元を持たない `WithUserGroup`／`IsUserGroupSupported` をインターフェースごと削除する」と判断したのと同じ基準であり、CLAUDE.md の YAGNI 原則にも従う。

この削除により、A1 の別の所見 **L-5**（`sync.RWMutex` を含む `Metrics` を値で返す `GetSnapshot`／`GetMetrics` の copylocks フットガン）も同時に解消される。L-5 は #977 の対象にも `98_remaining_issues.md` §2 の一覧にも含まれていない（同節の A1 は L-2・L-3・L-4 の3件のみを挙げる）が、対象物が消えるため findings 側に解消を記録する。

### L-4: 現存する。ただし現在の本番コードに再入経路は存在しない

`WithPrivileges` は先頭で `m.mu` を取り、`fn()` の実行を跨いで保持し続ける。特権操作を直列化するための意図的な設計だが、`fn` の内側から同一マネージャの `WithPrivileges` を呼ぶと自己デッドロックする。この禁止は doc コメントに書かれていない。

本番の呼び出し元は `internal/runner/base/executor/executor.go` の user/group 実行1箇所のみで、その `fn` から特権マネージャへ戻る経路は現状ない（`resource.Manager.WithPrivileges` は本番未使用であり、#1041 として分離した）。したがって現時点でデッドロックは発生しないが、その安全性は「呼び出し元が1つしかない」という偶然に依存している。

所見は「必要なら再入検出（保持中フラグ）でエラー返却にする」ことも挙げているが、Go でこれを正しく実装する手段はない。フラグが立っていても、それが自ゴルーチンの再入なのか他ゴルーチンの正当な待機なのかを、ゴルーチン識別なしに区別できないためである（`TryLock` でも同じ）。本タスクは検出を実装せず、契約を doc に明記したうえで、実装しない理由も同じ場所に残す。

## 目的

- 読み手のいない `privilege.Metrics` を削除し、「昇格を試みていないのに昇格失敗として計上される」「昇格時間という名前で操作全体の時間を測る」という誤った記録セマンティクスを、指標の意味を整える方向ではなく、指標そのものを無くす方向で解消する。
- 昇格・復元の syscall が失敗したときの分岐と、そこから `emergencyShutdown` に至る経路を、新たな注入点を一切導入せずに単体テストで踏めるようにする。`identity_mutation_guard_test.go` が守っている「この package が identity を変更しうる箇所は2つの呼び出し式だけ」という不変条件は維持する。
- `WithPrivileges` が再入不可であることを、実装を読まなくても分かる形で契約として明記し、再入検出を実装しない理由も併せて残す。

## スコープ

### 対象

1. `privilege.Metrics` 型と、`privilege.Manager` インターフェースの `GetMetrics` の削除（L-3・L-5）。
2. 削除に伴って意味を失う `executionContext.start`、および metrics 記録を前提とした関数名の整理（L-3）。
3. `escalatePrivileges`・`restorePrivileges` の syscall 失敗分岐と `emergencyShutdown` 到達を、注入点を追加せずにテストで踏むこと（L-2）。
4. `WithPrivileges` および `runnertypes.PrivilegeManager` の doc への再入禁止の明記（L-4）。
5. 0149 残件一覧（`98_remaining_issues.md` §2 A1）と findings（`A1_privilege.md`）への対応状況の反映、および改名した関数を引用している設計文書3対の更新。

### 対象外

- **L-2 が推奨する「昇格・復元での注入フィールド利用」そのもの**。上記「背景」のとおり、現在の静的ガードが禁じている形であり、採用しない。本タスクは所見の推奨とは逆方向で L-2 を close する。この判断の根拠は要件定義書と `98_remaining_issues.md` に残す。
- **`identity_mutation_guard_test.go` の allowlist・禁止規則の変更**。本タスクは production コードの identity 変更箇所を1つも増減させないため、ガードは無変更で通過するはずである。
- **再入検出の実装**（L-4）。上記「背景」のとおり、Go では正しく実装できない。doc 明記にとどめる。
- **A1 M-1（昇格ウィンドウがプロセス全体・コマンド全実行時間に及ぶ）**。設計そのものの見直しを伴い、L-2〜L-4 とは規模が異なる。残件一覧に残す。
- **A1 L-1（`user.Lookup` の二重呼び出し）**。0157 が該当関数を削除したかどうかを含め、現行コードでの現存確認が別途必要であり、本タスクでは扱わない。
- **A1 I-1（`GetCurrentUID` が effective UID を返す命名の乖離）**。後述の「決定事項」のとおり `GetCurrentUID` 自体は残すため、本タスクでは解消しない。
- **`resource.Manager.WithPrivileges` の削除**（#1041）。resource 層の API 整理であり、privilege package に閉じた本タスクとは影響範囲が異なる。
- **executor 側の `audit.PrivilegeMetrics` の変更**。本タスクは privilege package の内部勘定を削除するだけで、監査ログに出る指標は変更しない。

## 決定事項

本タスクは変更範囲が privilege package に閉じており、新規の設計要素を持たないため、`02_architecture.md` を省略して `03_implementation_plan.md` へ進む。設計上の選択は以下のとおり確定し、詳細は実装計画書に記す。

- **`privilege.Manager` インターフェースの整理範囲**: 本タスクでは `GetMetrics` のみを削除する。`GetCurrentUID`・`GetOriginalUID` も本番の呼び出し元を持たないが、削除は I-1（命名の乖離）の扱いと一体で判断すべき別件であり、スコープを広げない。
- **metrics 削除後の関数名**: `handleCleanupAndMetrics` を `handleCleanup` に、`restorePrivilegesAndMetrics` を `restorePrivilegesAndVerify` に改名する。
- **L-2 のテストの配置**: 既存の `unix_privilege_test.go` に追加する。skip 条件と理由文は `cmd/runner/startup_privilege_test.go` の既存パターンに揃える。
- **`restorePrivileges` を失敗させる UID**: `originalUID` に `syscall.Getuid() + 1` を用いる。非 root・非 setuid のテストバイナリでは real・effective・saved のいずれもが `syscall.Getuid()` に等しいため、この値は3つのどれとも一致せず、`Seteuid` は必ず失敗する。実 UID が 65534 の場合や user namespace で `uid+1` が未マップの場合は errno が `EPERM` ではなく `EINVAL` になるため、テストでは errno の値を固定せず、失敗したことのみを検証する。

## Acceptance Criteria

#### F-001: `privilege.Metrics` の削除

読み手を持たない特権操作の内部勘定を削除する。

**Acceptance Criteria**:
- **AC-01**: `internal/runner/base/privilege` に `Metrics` 型、その記録メソッド（`RecordElevationSuccess`・`RecordElevationFailure`・`updateSuccessRate`・`GetSnapshot`・`Reset`）、および `UnixPrivilegeManager` の metrics フィールドが存在しない。
- **AC-02**: `privilege.Manager` インターフェースに `GetMetrics` が存在せず、`UnixPrivilegeManager` およびテスト用モックにも同メソッドの実装が存在しない。
- **AC-03**: 特権昇格・復元の外部から観測できる挙動が変わらない。すなわち、昇格の成否、`emergencyShutdown` の発火条件、`WithPrivileges` の戻り値、panic の再送出のいずれも本タスク前と同一である。
- **AC-04**: 削除により参照されなくなる `executionContext.start` フィールドと、`restorePrivilegesAndMetrics` に duration を渡すための計算が残らない。
- **AC-05**: 監査ログに出力される特権関連の指標（`elevation_count`・`total_privilege_duration_ms`）が本タスク前と同一である。これらは executor 側の `audit.PrivilegeMetrics` に由来し、本タスクの削除対象ではない。
- **AC-06**: `make deadcode` が、本タスクの削除に起因する新たな未使用シンボルを報告しない。

#### F-002: 昇格・復元の syscall 失敗分岐をテストで踏む

新たな注入点を導入せずに、実際に `EPERM` を起こして失敗分岐を踏む。

**Acceptance Criteria**:
- **AC-07**: `escalatePrivileges` が `syscall.Seteuid(0)` の失敗により `*privilege.Error` を返す経路が、非特権プロセスで実行されるテストで踏まれる。テストは戻り値が `*Error` であることと、その `Operation`・`OriginalUID`・`TargetUID`・`SyscallErr` が呼び出し内容と整合することを検証する。
- **AC-08**: `restorePrivileges` の失敗により `restorePrivilegesAndMetrics`（改名後の名称を含む）が `emergencyShutdown` を呼ぶ経路が、非特権プロセスで実行されるテストで踏まれる。テストは注入済みの `osExit` が終了コード 1 で呼ばれることを検証する。
- **AC-09**: AC-07・AC-08 のテストは、実行後にプロセスの実効 UID・実効 GID が実行前から変化していないことを検証する。
- **AC-10**: AC-07・AC-08 のテストは、root（`syscall.Geteuid() == 0`）で実行された場合、失敗分岐を踏めないことを理由として skip する。skip の理由文から、なぜ root では成立しないのかが読み取れる。
- **AC-11**: `internal/runner/base/privilege` の production コードに、テスト用に syscall を差し替えるためのフィールド・パッケージ変数・関数値が追加されていない。
- **AC-12**: `TestNoUnexpectedIdentityMutationSyscalls` が、その allowlist と禁止規則を一切変更しないまま通過する。
- **AC-13**: AC-07・AC-08 の各テストが、検証対象の分岐を無効化すると失敗する（CLAUDE.md「テストは主張する理由で失敗できること」）。無効化の方法と失敗を確認した旨をコミットメッセージに記す。

#### F-003: 再入禁止の契約の明記

**Acceptance Criteria**:
- **AC-14**: `UnixPrivilegeManager.WithPrivileges` の doc コメントに、(a) `fn` の実行中もミューテックスを保持すること、(b) `fn` の内側から同一マネージャの `WithPrivileges` を呼ぶと自己デッドロックすること、(c) したがって再入は呼び出し側の責務として禁止されること、が記載されている。
- **AC-15**: 同じ doc に、再入検出を実装しない理由（保持中フラグでは自ゴルーチンの再入と他ゴルーチンの正当な待機を区別できず、`TryLock` でも同じであること）が記載されている。
- **AC-16**: `runnertypes.PrivilegeManager` インターフェースの `WithPrivileges` の doc コメントにも、再入が禁止であることが記載されている。インターフェースの利用者が実装を読まずに契約を知れる。
- **AC-17**: 上記の doc はすべて英語で記述される。

#### F-004: 文書への反映

**Acceptance Criteria**:
- **AC-18**: `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` §2 の「A1（privilege）」から、本タスクで解消した L-2・L-3・L-4 の3件が残件として除かれ、解消したことと本タスク・#977 への参照が、同文書が既に D1 M-3・B3 M2 に用いている引用ブロック（`> **… について**`）の形式で記載されている。L-2 については、所見の推奨とは逆方向で close したことと、その根拠（`identity_mutation_guard_test.go` の存在）が読み取れる。
- **AC-19**: `docs/tasks/0149_security_code_smell_audit_fable/findings/A1_privilege.md` の L-2・L-3・L-4・L-5 に、本タスクでの対応結果が追記されている。所見の原文は書き換えず、監査時点の記述として残す。
- **AC-20**: `98_remaining_issues.md` の A1 以外の残件（D1・B1・D2・E1 ほか）の記述が、本タスクの書き換えによって増減していない。
- **AC-21**: `handleCleanupAndMetrics`・`restorePrivilegesAndMetrics` を引用・説明している利用者向け／開発者向け文書が、改名後の名称と実態に更新されている。対象は `docs/dev/architecture_design/security-architecture{.ja,}.md`、`docs/dev/developer_guide/design-implementation-overview{.ja,}.md`、`docs/user/security-risk-assessment{.ja,}.md` の3対である。とくに「パニック回復と時間計測を担う」旨の記述は、時間計測が無くなるため実態に合わせて修正する。日本語版を先に更新し、英語版は `/mktrans` で反映する。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが `make test` で成功する。
- `make lint` が警告なく通過する。
- 本タスクの前後で、`internal/runner/base/privilege` の production コードが identity 変更 syscall を呼びうる箇所が増減していない（`escalatePrivileges` の `Seteuid(0)` と `restorePrivileges` の `Seteuid(m.originalUID)` の2箇所のまま）。
- 特権昇格・復元・emergency shutdown の外部から観測できる挙動が、本タスクの前後で変わらない。
- #977 の3件それぞれについて、解消したのか、所見の推奨とは異なる形で close したのかが、コードと監査文書の双方から追える。
