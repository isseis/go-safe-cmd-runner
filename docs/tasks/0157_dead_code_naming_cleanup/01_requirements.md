# 要件定義書: フィルタ未実装・命名と実装の乖離の整理（デッドコード削除含む）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-25 |
| Review date | 2026-07-26 |
| Reviewer | isseis |
| Comments | 2026-07-26 に isseis が承認したのち、[03_implementation_plan.md](03_implementation_plan.md) の作成時の調査で AC-25 が現実には達成できない範囲を含むと判明したため draft に戻した（再承認が必要）。`user.Current()` を除いても、group-writable なファイルの判定は `IsUserInGroup` / `isUserOnlyGroupMember` を経由して `user.LookupId` を呼ぶため、passwd エントリなしでは判定が成立しない。AC-25 の対象を「権限判定に用いる UID の取得」に限定し、対象外節と「方針判断の記録」に根拠を追加した。AC の追加・削除・番号変更はない。 |

## 関連 Issue

- [#864 [Security][P5] フィルタ未実装・命名と実装の乖離を整理（デッドコード削除含む）](https://github.com/isseis/go-safe-cmd-runner/issues/864)
- 詳細所見:
  - [findings/A3_environment.md](../0149_security_code_smell_audit_fable/findings/A3_environment.md) F-1, F-2, F-3, F-4, F-5
  - [findings/A1_privilege.md](../0149_security_code_smell_audit_fable/findings/A1_privilege.md) M-2
  - [findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) M-4
  - [findings/C3_shebang_fileanalysis.md](../0149_security_code_smell_audit_fable/findings/C3_shebang_fileanalysis.md) F3
  - 集約サマリ: [99_summary.md](../0149_security_code_smell_audit_fable/99_summary.md)（横断パターン P5）

## 背景

Issue #864 は「『フィルタする』と称して実質フィルタしていない」「関数名・doc コメントと実装が食い違う」「本番から到達しないコードが特権 syscall ごと残置されている」という同型の code smell が複数コンポーネントに分布していると指摘している。いずれも直接の脆弱性ではないが、監査時に「ここで防御が効いている」という誤読を誘発する点で共通しており、YAGNI 原則（CLAUDE.md）に沿った整理が求められる。

2026-07-25 時点の現物コードで、以下4件がいずれも未修正であることを確認済みである。

### 1. A3: `environment.Filter` は allowlist を保持するだけでフィルタしない

`internal/runner/base/environment/filter.go` の `Filter` 型は、`NewFilter(allowList []string)` で受け取った allowlist を `globalAllowlist map[string]struct{}`（filter.go:28）に格納するが、このフィールドを読むコードはリポジトリ全体に存在しない（`filter_test.go:22-24` がフィールドの存在と要素数を確認するのみ）。

`FilterSystemEnvironment`（filter.go:72-76）／`FilterGlobalVariables`（filter.go:81-102）は名前に反して allowlist 照合を一切行わず、実処理は「空の変数名をスキップし、残りをそのままコピーする」だけである。さらに `ParseSystemEnvironment` の doc コメント（filter.go:41）は「use `IsVariableAccessAllowed` for filtering」と案内するが、`IsVariableAccessAllowed` はコードベースのどこにも存在しない。

付随する劣化として以下がある。

- **未使用エラー6個**（filter.go:14-22）: `ErrGroupNotFound` / `ErrVariableNameEmpty` / `ErrInvalidVariableName` / `ErrDangerousVariableValue` / `ErrVariableNotFound` / `ErrVariableNotAllowed` はいずれも本パッケージからもリポジトリ内からも返却・比較されない。とくに `ErrGroupNotFound` は `internal/runner/runner.go:36` と `internal/runner/cli/filter.go:13` にも同一メッセージの別インスタンスがあり、`errors.Is` で相互にマッチしない同名 sentinel が三重に存在する。
- **常に nil の error 戻り値**（filter.go:72, 81）: 両関数は `error` を返すが失敗パスが存在しない。呼び出し側（runner.go:391-394）はエラーを丁寧にラップしており、「ここで危険値検証が行われ失敗し得る」という誤った印象を与える。
- **空ファイル**: `internal/runner/base/environment/filter_benchmark_test.go` は `package environment` の1行のみで、ベンチマークが1つも存在しない。

なお、本パッケージには [0156](../0156_env_denylist_consolidation/) で追加された `denylist.go`（禁止環境変数名の一元判定）が同居しており、こちらは実際に3層から参照される現役コードである。

### 2. A3 F-2: `Runner.LoadSystemEnvironment` は無フィルタの全環境を死にフィールドへ格納する

`internal/runner/runner.go:388-397` の `LoadSystemEnvironment` は doc コメントで「loads and filters system environment variables」と述べるが、実体は上記の無フィルタ処理であり、`r.envVars`（runner.go:66）には機密値を含む全システム環境変数が格納される。

さらに `r.envVars` は代入（runner.go:395）以降どこからも読み出されない。`envFilter` フィールド（runner.go:69）も `LoadSystemEnvironment` からしか使われない。したがって `cmd/runner/main.go:421` の呼び出しは、全環境変数のコピーをプロセス寿命の間メモリに保持するだけの無意味な処理になっている。

一方、`Filter` のうち `ParseSystemEnvironment` は `internal/runner/config/expansion.go:866`（`ExpandGlobal` が `runtime.SystemEnv` を一度だけ構築する箇所）から実際に使われており、これは現役の機能である。

### 3. A1 M-2: `changeUserGroupInternal` の実降格パスが本番到達不能

`internal/runner/base/privilege/unix.go:154-166`（`prepareExecution` の operation 分岐）では、`needsUserGroupChange = true` になるのは `OperationUserGroupDryRun` のみである。dry-run では `changeUserGroupInternal` が `dryRun=true` で呼ばれ unix.go:558-567 で早期リターンするため、以下は本番のどの操作からも到達しない。

- unix.go:569-580 の `m.syscallSetegid` / `m.syscallSeteuid` 実行部、Seteuid 失敗時の EGID ロールバック、およびロールバック失敗時の `emergencyShutdown("egid_rollback_failure_after_seteuid_failure")`。
- unix.go:182-186 の `performElevation` 内ロールバックブロック（`needsPrivilegeEscalation && needsUserGroupChange` が同時に true になる operation が存在しない）。

`syscallSeteuid` / `syscallSetegid` 注入フィールド（unix.go:43-44）も、この到達不能パス以外からは使われていない（`escalatePrivileges` は unix.go:294 で、`restorePrivileges` は unix.go:324 で `syscall.Seteuid` を直接呼ぶ）。

実際のユーザー切替は executor 側で行われている。`internal/runner/base/executor/executor.go:217-222` が `syscall.Credential{Uid, Gid, Groups, NoSetGroups:false}` を構築し、子プロセス起動時に execve で原子的に適用する。すなわち `WithUserGroup`（unix.go:592-599）が実際に行うのは root への昇格のみで、`RunAsUser` / `RunAsGroup` は解決とログのために渡されているに過ぎず、名前と実装が乖離している。既存テスト（`unix_privilege_test.go:142-180`）も「`OperationUserGroupExecution` では `syscallSeteuid`/`syscallSetegid` が呼ばれないこと」を期待値として固定している。

### 4. D1 M-4: `getProcessEUID` は EUID ではなく実 UID を返す

`internal/groupmembership/manager.go:490-500` の `getProcessEUID` は `user.Current()` を用いる。Go 標準 `os/user` の `current()` は `getuid(2)`（**実 UID**）でユーザーを引き、かつ結果をプロセス生存中キャッシュする。したがって:

- 関数名と doc コメント（manager.go:481-482 の "returns the current user's EUID" / "the actual EUID of the running process"）が事実と異なる。
- 呼び出し元 `CanCurrentUserSafelyWriteFile`（manager.go:275-286）のコメント「For write operations, use the actual EUID (not SUDO_UID)」も同様に不正確。
- `getPermissionCheckUID`（manager.go:445-459）の sudo 判定コメント「EUID must be 0 (root) and SUDO_UID must be set」（manager.go:451）も、実際には実 UID による判定である。
- `user.Current()` のキャッシュにより、本プロジェクトの特権昇格／降格の前後で UID が変わっても判定は初回値に固定される。

### 5. C3 F3: `SaveSyscallAnalysis` の不整合レコード生成と、ファイル全体のデッドコード化

`internal/fileanalysis/syscall_store.go:53-71` の `SaveSyscallAnalysis` は `Store.Update` 経由で `record.ContentHash` と `record.SyscallAnalysis` のみを書き換え、`DynLibDeps` / `ShebangChain` / `SymbolAnalysis` / `AnalysisWarnings` は既存値を温存する。ファイル内容が変わった後にこの API 単独で呼ばれると「新しい ContentHash に旧内容由来の解析結果が紐付いた」レコードが生成され、verify 側のハッシュ一致チェックを通過してしまう。

一方で 2026-07-25 時点の grep により、以下がいずれも**本番コードから参照されていない**ことを確認した（参照は `internal/fileanalysis/syscall_store_test.go` および `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` のテストのみ）。

- `fileanalysis.SyscallAnalysisStore`（interface, syscall_store.go:24-36）
- `fileanalysis.SyscallAnalysisResult`（syscall_store.go:13-19。同名の `SyscallAnalysisData` とは別型で、こちらは現役）
- `syscallAnalysisStore` / `NewSyscallAnalysisStore` / `SaveSyscallAnalysis` / `LoadSyscallAnalysis`（syscall_store.go:41-107）

したがって interface の doc コメント「Used directly by `cmd/record` for saving/loading syscall analysis」（syscall_store.go:23）は実態と乖離している。`internal/security/elfanalyzer` が持つ同名 interface `SyscallAnalysisStore`（`elfanalyzer/syscall_store.go`）は**別型**であり、本ファイルの削除の影響を受けない。

## 目的

- 「フィルタ」「EUID」「ユーザー降格」といった**防御を示唆する名前**が、実装が実際に行っていることと一致する状態にする（名前を実装に合わせる、または実装を名前に合わせるかを所見ごとに決定する）。
- 本番から到達しないコード（特権 syscall を含む降格パス、死にフィールド、未使用 sentinel、未使用ストア API、空ファイル）を削除し、監査対象を縮退させる。
- 上記はいずれも**外部から観測可能な挙動を変えないリファクタリング**として行う。挙動変化が避けられない箇所（D1 M-4）は、変化の有無と方向を明示し、テストで固定する。
- 誤用一歩手前の footgun（allowlist 適用済みに見えて未適用な戻り値、実降格したように見える `WithUserGroup`）を構造的に取り除く。`WithUserGroup` については、doc コメントで実態を説明するのではなく API ごと削除する（F-006）。

## スコープ

本タスクは互いに独立な4コンポーネントを扱うため、Phase 単位で分割して進める。各 Phase の設計・実装手順は [02_architecture.md](02_architecture.md) / [03_implementation_plan.md](03_implementation_plan.md) に委ね、本書では「何を達成するか」を Acceptance Criteria として定義する。

### 対象（本タスクで対応する）

1. **Phase 1（A3 F-1〜F-5）**: `internal/runner/base/environment` パッケージの縮退・改名、および `internal/runner/runner.go` / `cmd/runner/main.go` の死にフィールド・死にメソッドの削除。
2. **Phase 2（A1 M-2）**: `internal/runner/base/privilege/unix.go` の到達不能な実降格パス（`Setegid`/`Seteuid` 実行部・EGID ロールバック・`performElevation` のロールバックブロック）の削除、本番呼び出し元を持たない `WithUserGroup` / `IsUserGroupSupported` の `runnertypes.PrivilegeManager` インターフェースからの削除（F-006）、および `WithPrivileges` の doc コメント整備。
3. **Phase 3（D1 M-4）**: `internal/groupmembership/manager.go` の `getProcessEUID` および `getPermissionCheckUID` の命名・コメントの実装との整合。
4. **Phase 4（C3 F3）**: `internal/fileanalysis/syscall_store.go` の削除（後述「方針判断の記録」参照）。
5. 上記に伴うテストの追加・更新・削除。
6. **セキュリティ設計文書の追随（Phase 1）**: `docs/dev/architecture_design/security-architecture.ja.md` / `.md` の §3 が引用する `Filter` 構造体の記述を、Phase 1 適用後の実装に合わせる（F-005）。

Phase 間に実装上の依存はなく、独立にレビュー・マージ可能とする。

### 対象外（別 Issue・別タスクとする、または本タスクでは対応しない）

- **D1 の他の所見（H-1 / M-1 / L-2 / L-3 等 fail-open 系）**: H-1 / M-1 は [0150](../0150_groupmembership_getgrgid_failclosed/) / [0151](../0151_groupmembership_failclosed/) で対応済み、L-2 / L-3 は [#860](https://github.com/isseis/go-safe-cmd-runner/issues/860) / [0153](../0153_failopen_error_handling_crosscut/) の管理下で未着手。いずれも本タスクの対象外であり、本タスクは D1 M-4（命名と実装の乖離）のみを扱う。
- **D1 M-3（`SUDO_UID` を検証せず権限チェック主体として採用する問題）**: `getPermissionCheckUID` の sudo 分岐そのものの是非は別系統のセキュリティ判断であり、本タスクはコメント・命名の整合に留める（[#920](https://github.com/isseis/go-safe-cmd-runner/issues/920)）。（2026-07-26 修正: 当初 "D1 M-2" と記していたが、`SUDO_UID` の所見は D1 M-3 である。D1 M-2 は `getGroupMembers` の CGO / 非 CGO 間の意味論差であり、[0151](../0151_groupmembership_failclosed/) で対応済みのため本タスクの対象外に挙げる必要はない。）
- **A1 の他の所見（L-1 二重 `user.Lookup`、L-2 昇格・復元での注入フィールド不使用、L-3 metrics の恒偽項、L-4 再入デッドロック）**: #864 の「該当箇所」に含まれない。なお本タスクの AC-13（未使用となった `syscallSeteuid`/`syscallSetegid` の削除）は A1 L-2 の推奨（昇格・復元でも注入フィールドを使う）と方向が逆であるが、L-2 に着手する時点で必要な注入点を改めて設計する方が、未使用フィールドを温存するより健全と判断する。
- **A3 F-6（`ParseSystemEnvironment` が不正形式エントリを無音スキップ）/ F-7（allowlist 判定ロジックの分散）**: #864 の「該当箇所」に含まれず、前者は挙動変更、後者は設計変更を伴うため別途検討。
- **C3 の他の所見（F1, F2, F4 以降の shebang / fileanalysis 所見）**: #864 の「推奨対応」に挙げられているのは F3 のみ。
- **`elfanalyzer.NewStandardELFAnalyzerWithSyscallStore`（`internal/security/elfanalyzer/standard_analyzer.go:70`）の本番未使用**: Phase 4 の調査で判明した隣接デッドコードだが、#864 の該当箇所ではなく、syscall analysis の将来的な有効化方針と併せて判断すべきため本タスクでは削除しない。
- **機能追加・挙動変更全般**: 本タスクは削除・改名・doc 修正に限定する。ただし F-006（`WithUserGroup` / `IsUserGroupSupported` のインターフェースからの削除）は API 変更を伴う。本番呼び出し元が存在しないため外部から観測可能な挙動は変わらないと判断し、本タスクの対象に含める。`environment` パッケージへの allowlist 適用機能の「復活」（A3 F-1 の対応案2）は、`config` 層の `ProcessEnvImport` と二重適用になるため採用しない。<br>ただし Phase 3 には一点だけ挙動変化がある。`user.Current()` をやめることで、権限判定に用いる UID の取得時に passwd エントリを引けずファイルアクセスを拒否していた失敗経路が消える（fail-closed から fail-open への変化）。これは意図した変更として AC-25 で扱う。詳細と受容理由は [02_architecture.md](02_architecture.md) §5.4。
- **`internal/runner/base/environment/denylist.go`（0156 で追加）**: 本タスクでは変更しない。同一パッケージ内の `Filter` 側のみを整理する。
- **group-writable なファイルに対するグループメンバーシップ照会の passwd 依存**: `IsUserInGroup`（`manager.go:138`）と `isUserOnlyGroupMember`（`manager.go:177`）は `user.LookupId` を呼ぶため、passwd エントリを引けない環境では従来どおり fail-closed で拒否する。この拒否は [0151](../0151_groupmembership_failclosed/) が意図して導入したものであり、本タスクでは緩めない。理由は「方針判断の記録」の「D1 M-4 の派生」を参照。AC-25 の対象は UID の取得に限る。

## 方針判断の記録

要件確定にあたり、findings が複数案を併記している2件について、現物コードを確認したうえで以下の方針を採る。

### A1 M-2: 「削除」を採る

findings は「到達不能な実降格パスを削除する」か「実フローを doc コメントに明記する」の二択を挙げているが、本タスクでは**削除を主、doc 明記を従**とする。理由:

- 実降格パスは `Setegid`/`Seteuid` という特権 syscall と `emergencyShutdown`（プロセス即時終了）を含む。到達不能なまま残すと、将来 `prepareExecution` の分岐が1行変わっただけで検証されていない特権コードが有効化される。
- 実際のユーザー切替は executor の `syscall.Credential` に一本化されており（execve 時に原子的に適用されるため、親プロセスの EUID/EGID を書き換える方式より安全）、降格の二重実装を維持する理由がない。YAGNI 原則にも整合する。
- ただし削除だけでは「特権昇格の実フローが何をするのか」が読み取れないままになるため、`WithPrivileges` の doc コメントへの実フロー明記（AC-15）も併せて必須とする。なお `WithUserGroup` 自体は F-006 により削除するため、doc コメントの対象から外れる。

### D1 M-4: 「実 UID セマンティクスを正とし、名前とコメントを実装に合わせる」を採る

findings は「EUID が必要なら `os.Geteuid()` を使う」「実 UID が正しい仕様ならば関数名とコメントを修正する」の二択を挙げている。本タスクでは**後者**（実 UID を正とし、`os.Getuid()` / `syscall.Getuid()` を直接使ってキャッシュ依存だけを排除する）を採る。理由:

- 呼び出し元 `CanCurrentUserSafelyReadFile` / `CanCurrentUserSafelyWriteFile` は `internal/safefileio/safe_file.go:445, 454, 492` から呼ばれ、この経路はファイル検証（`OperationFileValidation` により `syscall.Seteuid(0)` 済み）の内側でも実行され得る。ここで EUID セマンティクスへ切り替えると、昇格中は常に EUID=0 と判定され、さらに `getPermissionCheckUID` の sudo 分岐が発火して `SUDO_UID`（環境変数由来）が権限チェック主体として採用される。これは意図しない緩和（fail-open 方向）であり、命名整合という本タスクの目的に対して副作用が大きすぎる。
- 現状の実 UID 判定は fail-closed 方向（setuid 構成で root 権限の書き込みが不当に拒否され得る）であり、安全側に倒れている。
- したがって本タスクでは判定主体を変えず、`user.Current()` のキャッシュ（特権昇格前後で値が固定される問題）だけを取り除き、名前・コメントを実 UID セマンティクスに揃える。EUID を使うべきか否かの再設計は、上記 safefileio 経路の権限モデル全体を扱う別タスクに委ねる。

### D1 M-4 の派生: AC-25 の範囲を「UID の取得」に限定する

`user.Current()` をやめても、権限判定から passwd 依存が完全に消えるわけではない。残るのは次の 2 箇所で、いずれも**ファイルが group-writable（`perm & 0o020 != 0`）のときにしか通らない**分岐である。

- 読み取り経路: `IsUserInGroup`（`manager.go:138`）の `user.LookupId` — 対象 UID のプライマリ GID・補助グループ・ユーザー名を得るために必要。
- 書き込み経路: `isUserOnlyGroupMember`（`manager.go:177`）の `user.LookupId` — 対象 UID のユーザー名をグループのメンバー一覧と照合するために必要。

当初の AC-25 は「権限判定がエラーで中断せずに実行され、エントリを引ける場合と同じ判定結果を返す」ことを求めていたが、この 2 箇所については達成できない。理由を示す。

- **書き込み経路は原理的に答えが出せない。** 「グループ G のメンバーは対象ユーザーただ 1 人か」という問いは、ユーザーデータベースを引かなければ答えが存在しない。メンバーを UID で列挙する案も、`/etc/group` の記載自体が名前であるため passwd 解決を要する。したがって選べるのは「データベースを引けないときに拒否する」か「許可する」かの二択であり、後者は誰がグループに属するか分からないまま group-writable ファイルへの書き込みを許すことになる。
- **その拒否は意図して作られたものである。** [0151](../0151_groupmembership_failclosed/) は、まさにこの `isUserOnlyGroupMember` 周辺の fail-open を fail-closed 化するタスクだった。AC-25 のためにここを緩めるのは、0151 の判断を巻き戻すことになる。
- **読み取り経路も「同じ判定結果」は保証できない。** 自プロセスについてであれば `os.Getgid()` と `os.Getgroups()` でデータベースを引かずに所属を判定できるが、(1) sudo 分岐は `SUDO_UID`（自プロセスとは別ユーザー）を渡し得るため適用できず、(2) カーネルが保持するグループ集合はログイン・exec 時点のスナップショットであり、データベースの現在の設定と食い違い得る。
- **実務上の狙いは限定しても達成される。** ハッシュファイル・設定ファイルが group-writable でない通常の構成（0644・0600 など）では上記の分岐に入らないため、UID 取得の passwd 依存を除くだけで、当初の狙い（passwd エントリを持たない最小構成コンテナでランナーが起動できる）は満たされる。

以上より、AC-25 の対象を「権限判定に用いる UID の取得」に限定し、group-writable なファイルに対するグループメンバーシップの照会は対象外とする。照会が失敗した場合は従来どおり fail-closed で拒否する。

### C3 F3: 「削除」を採る

`SaveSyscallAnalysis` / `NewSyscallAnalysisStore` を含む `internal/fileanalysis/syscall_store.go` 全体に本番参照が存在しないことを確認済みのため、「ContentHash 不一致時に他フィールドをクリアする実装を追加して維持する」案は採らず、ファイルごと削除する（YAGNI）。将来 syscall analysis の永続化が必要になった時点で、`analyzeDynLibDeps` と同様の stale data prevention を備えた形で新規に設計する。ただし、レビューの結果「維持」の判断に変わった場合に備え、AC-24 に代替条件を残す。

## Acceptance Criteria

#### F-001: `environment` パッケージの縮退と Runner 死にコードの削除（A3 F-1〜F-5、Phase 1）

`internal/runner/base/environment` の `Filter` から未使用の allowlist を取り除き、実態（システム環境の列挙）に即した名前へ改める。あわせて `Runner` 側の死にフィールド・死にメソッドと、その唯一の呼び出し元を削除する。

- **AC-01**: `internal/runner/base/environment` に `globalAllowlist` フィールドが存在せず、コンストラクタが allowlist（変数名のスライス）を引数に取らない。
- **AC-02**: `Filter` 型、`FilterSystemEnvironment`、`FilterGlobalVariables` という名前がリポジトリ内に存在しない。システム環境を列挙する機能は、その実態を表す名前（例: 環境の読み取り／列挙を示す名前）で提供される。
- **AC-03**: `internal/runner/config/expansion.go` の `ExpandGlobal` が構築する `runtime.SystemEnv` の内容は、変更前後で同一である（`os.Environ()` の全エントリのうち `key=value` 形式にパースできるものが、キー・値ともに変わらず含まれる）。
- **AC-04**: 存在しない `IsVariableAccessAllowed` を参照する doc コメントがリポジトリ内に存在しない。
- **AC-05**: `environment` パッケージに `ErrGroupNotFound` / `ErrVariableNameEmpty` / `ErrInvalidVariableName` / `ErrDangerousVariableValue` / `ErrVariableNotFound` / `ErrVariableNotAllowed` の6つの sentinel が存在しない（`internal/runner/runner.go` および `internal/runner/cli/filter.go` の `ErrGroupNotFound` は現役のため残す）。
- **AC-06**: 環境列挙 API は、失敗パスを持たない限り `error` を返さないシグネチャである（常に nil を返す `error` 戻り値が存在しない）。
- **AC-07**: `internal/runner/base/environment/filter_benchmark_test.go` が存在しない。
- **AC-08**: `Runner` 構造体に `envVars` フィールドおよび `envFilter` フィールドが存在しない。
- **AC-09**: `Runner.LoadSystemEnvironment` メソッドが存在せず、`cmd/runner/main.go` からの呼び出しも存在しない。削除後も、既存の統合／E2E テスト（`cmd/runner/integration_*_test.go`、`internal/runner/e2e_*_test.go` 等）がコマンド実行時の環境変数解決について従来と同じ結果を得る。
- **AC-10**: [0156](../0156_env_denylist_consolidation/) で追加された `internal/runner/base/environment/denylist.go` の公開 API とその挙動は変更されない（同ファイルの既存テストが無修正で pass する）。

#### F-002: 到達不能な実降格パスの削除と `WithPrivileges` の doc 整合（A1 M-2、Phase 2）

- **AC-11**: `internal/runner/base/privilege` に、対象ユーザー／グループへ**実際に降格する**コード（`Setegid`/`Seteuid` の呼び出しと、その失敗時の EGID ロールバック・`emergencyShutdown`）が存在しない。
- **AC-12**: `performElevation` から、`needsPrivilegeEscalation && needsUserGroupChange` の同時成立を前提とした到達不能なロールバックブロックが削除されている。
- **AC-13**: AC-11 により未使用となったテスト注入フィールド（`syscallSeteuid` / `syscallSetegid`）と、それに依存していたテストが削除されている。
- **AC-14**: 旧 `changeUserGroupInternal` に相当する処理は、その実態（ユーザー／グループ名の解決と dry-run ログ出力）を表す名前になっており、「変更する（change）」ことを名乗らない。
- **AC-15**: `WithPrivileges` の doc コメントに、`OperationUserGroupExecution` の実フローとして次の3点が明記されている。(a) privilege パッケージは root への昇格のみを行い、`RunAsUser` / `RunAsGroup` を参照しない。(b) 対象ユーザーへの切り替えと、その前提となる識別情報の解決は executor が行い、子プロセス起動時の `syscall.Credential` により execve 時に適用される。(c) privilege パッケージ内で `RunAsUser` / `RunAsGroup` が解決・ログ出力されるのは `OperationUserGroupDryRun` の場合に限られる。<br>（2026-07-26 修正: 当初の文言は `OperationUserGroupExecution` でも `RunAsUser`・`RunAsGroup` が「解決とログのために渡される」としていたが、`prepareExecution` は同 operation で dry-run 解決を呼ばないため privilege パッケージはこれらを読まない。事実と異なる記述を doc コメントに書かせないよう改めた。詳細は [02_architecture.md](02_architecture.md) §5.5。）
- **AC-16**: 変更後も、(a) `OperationUserGroupExecution` では root 昇格と復元が従来どおり行われ、(b) `OperationUserGroupDryRun` ではユーザー／グループ解決とログ出力のみが行われて識別情報が変化せず、(c) いずれの経路でも親プロセスの EUID/EGID を対象ユーザーへ変更しないことが、テストで確認できる。既存の identity 検証（`ErrIdentityLeak` / saved-set 検査）の挙動も変わらない。

#### F-003: `getProcessEUID` の命名・実装の整合（D1 M-4、Phase 3）

- **AC-17**: `getProcessEUID` という名前の関数が存在しない。同等の機能は「プロセスの実 UID を返す」ことを表す名前で提供され、その doc コメントに EUID を返すという記述が含まれない。
- **AC-18**: 当該関数は `user.Current()` を使わず、実 UID を直接取得する（`os.Getuid()` / `syscall.Getuid()` 相当）。これによりプロセス生存中のキャッシュに起因する値の固定が発生しない。
- **AC-19**: `getPermissionCheckUID` の doc コメント・インラインコメント（sudo 判定条件の説明）および `CanCurrentUserSafelyWriteFile` のコメントが、実 UID による判定であるという実装と一致する。
- **AC-20**: 判定に使われる UID の値は変更前後で同一である。具体的に、(a) sudo なし・`SUDO_UID` 未設定、(b) `SUDO_UID` 設定済みかつ実 UID が 0、(c) `SUDO_UID` 設定済みかつ実 UID が 0 以外、の各ケースで `getPermissionCheckUID` が返す UID が従来と一致することをテストで確認できる。なお、この3ケースはいずれも passwd エントリを引ける前提であり、引けない場合の挙動は AC-25 が扱う。
- **AC-25**: 権限判定に用いる **UID の取得**が passwd エントリを必要としない。すなわち、実 UID に対応する passwd エントリを引けない状況（cgo 有効時の NSS 障害、cgo 無効時の `/etc/passwd` 欠如・エントリ未登録）でも UID の取得は失敗せず、グループメンバーシップの照会を伴わない権限判定（group-writable でないファイルに対する判定）がエラーで中断せずに実行され、エントリを引ける場合と同じ判定結果を返すことをテストで確認できる。あわせて次の 2 点がリリースノートに記載されている。(a) この変更が fail-closed（エラー時にファイルアクセスを拒否）から fail-open（判定を続行）への変化であること。(b) group-writable なファイルに対する判定はグループメンバーシップの照会を行うため、引き続き passwd エントリを必要とすること。<br>（2026-07-26 修正: 当初の文言は「権限判定が」エラーで中断しないことを求めていたが、group-writable なファイルの判定は `IsUserInGroup` / `isUserOnlyGroupMember` を経由して `user.LookupId` を呼ぶため、passwd エントリなしでは判定そのものが成立しない。範囲を限定した理由は「方針判断の記録」の「D1 M-4 の派生」を参照。）

#### F-004: `fileanalysis` の未使用 syscall analysis ストアの削除（C3 F3、Phase 4）

- **AC-21**: `internal/fileanalysis` に `SyscallAnalysisStore` interface、`syscallAnalysisStore` 実装、`NewSyscallAnalysisStore`、`SaveSyscallAnalysis`、および `fileanalysis.SyscallAnalysisResult` 型が存在しない。
- **AC-22**: 削除対象に依存していたテスト（`internal/fileanalysis/syscall_store_test.go` および `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` の該当箇所）が、削除または現役 API のみを使う形に更新されている。
- **AC-23**: 現役の型・機能は影響を受けない。すなわち `fileanalysis.SyscallAnalysisData`（`internal/dynamicanalysis/schema.go`、`internal/runner/base/security/network_analyzer.go` が使用）と、別型である `elfanalyzer.SyscallAnalysisStore` は削除されず、`cmd/record` / `cmd/runner` のビルドと既存テストが通る。
- **AC-24**: （代替条件）レビューの結果ストアを維持する判断となった場合は、`SaveSyscallAnalysis` が既存レコードの `ContentHash` と保存対象の `fileHash` が異なるときに `DynLibDeps` / `ShebangChain` / `SymbolAnalysis` / `AnalysisWarnings` をクリアし、その挙動を検証するテストが存在する。あわせて interface の doc コメント（"Used directly by `cmd/record`"）が実態に合うよう修正されている。AC-21〜AC-23 と AC-24 は排他であり、いずれか一方を満たせばよい。

#### F-005: セキュリティ設計文書の整合（Phase 1）

Phase 1 が削除する `Filter` 構造体は、セキュリティ設計文書が allowlist フィルタとして引用している。削除後もその記述を残すと、本タスクが解消しようとしている「記述と実装の乖離」を、より広く読まれる文書で再生産することになる。

- **AC-26**: `docs/dev/architecture_design/security-architecture.ja.md` および `security-architecture.md` の §3 に、`globalAllowlist` フィールドを持つ `Filter` 構造体の引用が存在せず、同節の記述が Phase 1 適用後の実装（`environment` パッケージはシステム環境の列挙と denylist 判定を提供し、allowlist は扱わない）と整合している。日本語版を先に修正し、英語版はそこから反映する。本 AC は Phase 1 の PR に含める。

#### F-006: 本番未使用の特権 API のインターフェースからの削除（Phase 2）

`runnertypes.PrivilegeManager` の `WithUserGroup` と `IsUserGroupSupported` は、いずれも本番の呼び出し元を持たない。executor は `WithPrivileges` を直接呼んでおり（`executor.go:236`）、両メソッドの呼び出しはテストのみである。インターフェースのメソッドセットに属するため `make deadcode` では検出されず、静的解析では見つからない位置に残っている。

`WithUserGroup` は「対象ユーザーの権限で実行する」と読める名前でありながら、実体は root へ昇格して `WithPrivileges` へ委譲するだけの薄いラッパである。doc コメントで実態を説明する方針（当初の AC-15）では、誤読を招く API がインターフェース上に残り続ける。本タスクの目的（誤用一歩手前の API 形状を構造的に取り除く）に照らし、削除する。

`IsUserGroupSupported` は `IsPrivilegedExecutionSupported` と同一のフィールド（`privilegeSupported`）を返す重複メソッドであり、これも本番呼び出し元を持たないため併せて削除する。`IsPrivilegedExecutionSupported` は本番で使用されている（`executor.go:162`、`dryrun_manager.go:274`）ため残す。

- **AC-27**: `runnertypes.PrivilegeManager` インターフェース、`UnixPrivilegeManager`、および privilege パッケージのモック（`testutil/mocks.go`、`internal/runner/resource/normal_manager_test.go`）から `WithUserGroup` と `IsUserGroupSupported` が削除されている。リポジトリ内に両メソッドの定義・呼び出しが存在しない（executor の非公開メソッド `executeWithUserGroup` は別物であり、残す）。
- **AC-28**: 削除後も `cmd/runner` / `cmd/record` / `cmd/verify` がビルドでき、既存の特権関連テストが pass する。とくに `OperationUserGroupExecution` を用いた実行経路（executor → `WithPrivileges`）の挙動は変わらない。
- **AC-29**: `IsPrivilegedExecutionSupported` は削除されず、`executor.go` および `dryrun_manager.go` の既存の呼び出しが従来どおり機能する。

## Success Criteria（要件レベル）

- AC-01〜AC-23、AC-25〜AC-29（AC-24 を採る場合はその代替を含む）のすべてに対し、実装計画（[03_implementation_plan.md](03_implementation_plan.md)）で具体的なテストまたは静的検証手段（`grep` による不存在確認など）が対応付けられている。
- 本タスクは AC-25 が扱う一点を除き挙動不変のリファクタリングであり、削除対象に直接依存していたテストを除き、既存テストが無修正で pass する。
- Phase 1〜4 のそれぞれが独立してレビュー可能な単位に分かれており、いずれかを見送っても他 Phase の成果が成立する。
- `make fmt` / `make lint` / `make test` がグリーンである。
