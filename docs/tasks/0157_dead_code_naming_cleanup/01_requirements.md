# 要件定義書: フィルタ未実装・命名と実装の乖離の整理（デッドコード削除含む）

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-25 |
| Review date | - |
| Reviewer | - |
| Comments | 2026-07-26 に isseis が一度承認したが、[02_architecture.md](02_architecture.md) の設計レビューで判明した次の3点を反映するため draft に戻した（再承認が必要）。(1) AC-15 の文言が `OperationUserGroupExecution` について事実と異なるため修正。(2) Phase 3 の fail-closed → fail-open 変化に対する AC-25 を追加。(3) セキュリティ設計文書の整合に対する F-005 / AC-26 を追加。既存 AC の番号は変更していない。 |

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
- 誤用一歩手前の footgun（allowlist 適用済みに見えて未適用な戻り値、実降格したように見える `WithUserGroup`）を構造的に取り除く。

## スコープ

本タスクは互いに独立な4コンポーネントを扱うため、Phase 単位で分割して進める。各 Phase の設計・実装手順は [02_architecture.md](02_architecture.md) / [03_implementation_plan.md](03_implementation_plan.md) に委ね、本書では「何を達成するか」を Acceptance Criteria として定義する。

### 対象（本タスクで対応する）

1. **Phase 1（A3 F-1〜F-5）**: `internal/runner/base/environment` パッケージの縮退・改名、および `internal/runner/runner.go` / `cmd/runner/main.go` の死にフィールド・死にメソッドの削除。
2. **Phase 2（A1 M-2）**: `internal/runner/base/privilege/unix.go` の到達不能な実降格パス（`Setegid`/`Seteuid` 実行部・EGID ロールバック・`performElevation` のロールバックブロック）の削除と、`WithUserGroup` / `WithPrivileges` の doc コメント整備。
3. **Phase 3（D1 M-4）**: `internal/groupmembership/manager.go` の `getProcessEUID` および `getPermissionCheckUID` の命名・コメントの実装との整合。
4. **Phase 4（C3 F3）**: `internal/fileanalysis/syscall_store.go` の削除（後述「方針判断の記録」参照）。
5. 上記に伴うテストの追加・更新・削除。
6. **セキュリティ設計文書の追随（Phase 1）**: `docs/dev/architecture_design/security-architecture.ja.md` / `.md` の §3 が引用する `Filter` 構造体の記述を、Phase 1 適用後の実装に合わせる（F-005）。

Phase 間に実装上の依存はなく、独立にレビュー・マージ可能とする。

### 対象外（別 Issue・別タスクとする、または本タスクでは対応しない）

- **D1 の他の所見（H-1 / M-1 / L-2 / L-3 等 fail-open 系）**: H-1 / M-1 は [0150](../0150_groupmembership_getgrgid_failclosed/) / [0151](../0151_groupmembership_failclosed/) で対応済み、L-2 / L-3 は [#860](https://github.com/isseis/go-safe-cmd-runner/issues/860) / [0153](../0153_failopen_error_handling_crosscut/) の管理下で未着手。いずれも本タスクの対象外であり、本タスクは D1 M-4（命名と実装の乖離）のみを扱う。
- **D1 M-2（`SUDO_UID` を検証せず権限チェック主体として採用する問題）**: `getPermissionCheckUID` の sudo 分岐そのものの是非は別系統のセキュリティ判断であり、本タスクはコメント・命名の整合に留める。
- **A1 の他の所見（L-1 二重 `user.Lookup`、L-2 昇格・復元での注入フィールド不使用、L-3 metrics の恒偽項、L-4 再入デッドロック）**: #864 の「該当箇所」に含まれない。なお本タスクの AC-13（未使用となった `syscallSeteuid`/`syscallSetegid` の削除）は A1 L-2 の推奨（昇格・復元でも注入フィールドを使う）と方向が逆であるが、L-2 に着手する時点で必要な注入点を改めて設計する方が、未使用フィールドを温存するより健全と判断する。
- **A3 F-6（`ParseSystemEnvironment` が不正形式エントリを無音スキップ）/ F-7（allowlist 判定ロジックの分散）**: #864 の「該当箇所」に含まれず、前者は挙動変更、後者は設計変更を伴うため別途検討。
- **C3 の他の所見（F1, F2, F4 以降の shebang / fileanalysis 所見）**: #864 の「推奨対応」に挙げられているのは F3 のみ。
- **`elfanalyzer.NewStandardELFAnalyzerWithSyscallStore`（`internal/security/elfanalyzer/standard_analyzer.go:70`）の本番未使用**: Phase 4 の調査で判明した隣接デッドコードだが、#864 の該当箇所ではなく、syscall analysis の将来的な有効化方針と併せて判断すべきため本タスクでは削除しない。
- **機能追加・挙動変更全般**: 本タスクは削除・改名・doc 修正に限定する。`environment` パッケージへの allowlist 適用機能の「復活」（A3 F-1 の対応案2）は、`config` 層の `ProcessEnvImport` と二重適用になるため採用しない。<br>ただし Phase 3 には一点だけ挙動変化がある。`user.Current()` をやめることで、passwd エントリを引けない場合にファイルアクセスを拒否していた失敗経路が消える（fail-closed から fail-open への変化）。これは意図した変更として AC-25 で扱う。詳細と受容理由は [02_architecture.md](02_architecture.md) §5.4。
- **`internal/runner/base/environment/denylist.go`（0156 で追加）**: 本タスクでは変更しない。同一パッケージ内の `Filter` 側のみを整理する。

## 方針判断の記録

要件確定にあたり、findings が複数案を併記している2件について、現物コードを確認したうえで以下の方針を採る。

### A1 M-2: 「削除」を採る

findings は「到達不能な実降格パスを削除する」か「実フローを doc コメントに明記する」の二択を挙げているが、本タスクでは**削除を主、doc 明記を従**とする。理由:

- 実降格パスは `Setegid`/`Seteuid` という特権 syscall と `emergencyShutdown`（プロセス即時終了）を含む。到達不能なまま残すと、将来 `prepareExecution` の分岐が1行変わっただけで検証されていない特権コードが有効化される。
- 実際のユーザー切替は executor の `syscall.Credential` に一本化されており（execve 時に原子的に適用されるため、親プロセスの EUID/EGID を書き換える方式より安全）、降格の二重実装を維持する理由がない。YAGNI 原則にも整合する。
- ただし削除だけでは「`WithUserGroup` が何をするのか」が読み取れないままになるため、doc コメントへの実フロー明記（AC-15）も併せて必須とする。

### D1 M-4: 「実 UID セマンティクスを正とし、名前とコメントを実装に合わせる」を採る

findings は「EUID が必要なら `os.Geteuid()` を使う」「実 UID が正しい仕様ならば関数名とコメントを修正する」の二択を挙げている。本タスクでは**後者**（実 UID を正とし、`os.Getuid()` / `syscall.Getuid()` を直接使ってキャッシュ依存だけを排除する）を採る。理由:

- 呼び出し元 `CanCurrentUserSafelyReadFile` / `CanCurrentUserSafelyWriteFile` は `internal/safefileio/safe_file.go:445, 454, 492` から呼ばれ、この経路はファイル検証（`OperationFileValidation` により `syscall.Seteuid(0)` 済み）の内側でも実行され得る。ここで EUID セマンティクスへ切り替えると、昇格中は常に EUID=0 と判定され、さらに `getPermissionCheckUID` の sudo 分岐が発火して `SUDO_UID`（環境変数由来）が権限チェック主体として採用される。これは意図しない緩和（fail-open 方向）であり、命名整合という本タスクの目的に対して副作用が大きすぎる。
- 現状の実 UID 判定は fail-closed 方向（setuid 構成で root 権限の書き込みが不当に拒否され得る）であり、安全側に倒れている。
- したがって本タスクでは判定主体を変えず、`user.Current()` のキャッシュ（特権昇格前後で値が固定される問題）だけを取り除き、名前・コメントを実 UID セマンティクスに揃える。EUID を使うべきか否かの再設計は、上記 safefileio 経路の権限モデル全体を扱う別タスクに委ねる。

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

#### F-002: 到達不能な実降格パスの削除と `WithUserGroup` の doc 整合（A1 M-2、Phase 2）

- **AC-11**: `internal/runner/base/privilege` に、対象ユーザー／グループへ**実際に降格する**コード（`Setegid`/`Seteuid` の呼び出しと、その失敗時の EGID ロールバック・`emergencyShutdown`）が存在しない。
- **AC-12**: `performElevation` から、`needsPrivilegeEscalation && needsUserGroupChange` の同時成立を前提とした到達不能なロールバックブロックが削除されている。
- **AC-13**: AC-11 により未使用となったテスト注入フィールド（`syscallSeteuid` / `syscallSetegid`）と、それに依存していたテストが削除されている。
- **AC-14**: 旧 `changeUserGroupInternal` に相当する処理は、その実態（ユーザー／グループ名の解決と dry-run ログ出力）を表す名前になっており、「変更する（change）」ことを名乗らない。
- **AC-15**: `WithUserGroup` および `WithPrivileges` の doc コメントに、`OperationUserGroupExecution` の実フローとして次の3点が明記されている。(a) privilege パッケージは root への昇格のみを行い、`RunAsUser` / `RunAsGroup` を参照しない。(b) 対象ユーザーへの切り替えと、その前提となる識別情報の解決は executor が行い、子プロセス起動時の `syscall.Credential` により execve 時に適用される。(c) privilege パッケージ内で `RunAsUser` / `RunAsGroup` が解決・ログ出力されるのは `OperationUserGroupDryRun` の場合に限られる。<br>（2026-07-26 修正: 当初の文言は `OperationUserGroupExecution` でも `RunAsUser`・`RunAsGroup` が「解決とログのために渡される」としていたが、`prepareExecution` は同 operation で dry-run 解決を呼ばないため privilege パッケージはこれらを読まない。事実と異なる記述を doc コメントに書かせないよう改めた。詳細は [02_architecture.md](02_architecture.md) §5.5。）
- **AC-16**: 変更後も、(a) `OperationUserGroupExecution` では root 昇格と復元が従来どおり行われ、(b) `OperationUserGroupDryRun` ではユーザー／グループ解決とログ出力のみが行われて識別情報が変化せず、(c) いずれの経路でも親プロセスの EUID/EGID を対象ユーザーへ変更しないことが、テストで確認できる。既存の identity 検証（`ErrIdentityLeak` / saved-set 検査）の挙動も変わらない。

#### F-003: `getProcessEUID` の命名・実装の整合（D1 M-4、Phase 3）

- **AC-17**: `getProcessEUID` という名前の関数が存在しない。同等の機能は「プロセスの実 UID を返す」ことを表す名前で提供され、その doc コメントに EUID を返すという記述が含まれない。
- **AC-18**: 当該関数は `user.Current()` を使わず、実 UID を直接取得する（`os.Getuid()` / `syscall.Getuid()` 相当）。これによりプロセス生存中のキャッシュに起因する値の固定が発生しない。
- **AC-19**: `getPermissionCheckUID` の doc コメント・インラインコメント（sudo 判定条件の説明）および `CanCurrentUserSafelyWriteFile` のコメントが、実 UID による判定であるという実装と一致する。
- **AC-20**: 判定に使われる UID の値は変更前後で同一である。具体的に、(a) sudo なし・`SUDO_UID` 未設定、(b) `SUDO_UID` 設定済みかつ実 UID が 0、(c) `SUDO_UID` 設定済みかつ実 UID が 0 以外、の各ケースで `getPermissionCheckUID` が返す UID が従来と一致することをテストで確認できる。なお、この3ケースはいずれも passwd エントリを引ける前提であり、引けない場合の挙動は AC-25 が扱う。
- **AC-25**: 実 UID に対応する passwd エントリを引けない状況（cgo 有効時の NSS 障害、cgo 無効時の `/etc/passwd` 欠如・エントリ未登録）でも、権限判定がエラーで中断せずに実行され、エントリを引ける場合と同じ判定結果を返すことをテストで確認できる。あわせて、この変更が fail-closed（エラー時にファイルアクセスを拒否）から fail-open（判定を続行）への変化であることがリリースノートに記載されている。

#### F-004: `fileanalysis` の未使用 syscall analysis ストアの削除（C3 F3、Phase 4）

- **AC-21**: `internal/fileanalysis` に `SyscallAnalysisStore` interface、`syscallAnalysisStore` 実装、`NewSyscallAnalysisStore`、`SaveSyscallAnalysis`、および `fileanalysis.SyscallAnalysisResult` 型が存在しない。
- **AC-22**: 削除対象に依存していたテスト（`internal/fileanalysis/syscall_store_test.go` および `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` の該当箇所）が、削除または現役 API のみを使う形に更新されている。
- **AC-23**: 現役の型・機能は影響を受けない。すなわち `fileanalysis.SyscallAnalysisData`（`internal/dynamicanalysis/schema.go`、`internal/runner/base/security/network_analyzer.go` が使用）と、別型である `elfanalyzer.SyscallAnalysisStore` は削除されず、`cmd/record` / `cmd/runner` のビルドと既存テストが通る。
- **AC-24**: （代替条件）レビューの結果ストアを維持する判断となった場合は、`SaveSyscallAnalysis` が既存レコードの `ContentHash` と保存対象の `fileHash` が異なるときに `DynLibDeps` / `ShebangChain` / `SymbolAnalysis` / `AnalysisWarnings` をクリアし、その挙動を検証するテストが存在する。あわせて interface の doc コメント（"Used directly by `cmd/record`"）が実態に合うよう修正されている。AC-21〜AC-23 と AC-24 は排他であり、いずれか一方を満たせばよい。

#### F-005: セキュリティ設計文書の整合（Phase 1）

Phase 1 が削除する `Filter` 構造体は、セキュリティ設計文書が allowlist フィルタとして引用している。削除後もその記述を残すと、本タスクが解消しようとしている「記述と実装の乖離」を、より広く読まれる文書で再生産することになる。

- **AC-26**: `docs/dev/architecture_design/security-architecture.ja.md` および `security-architecture.md` の §3 に、`globalAllowlist` フィールドを持つ `Filter` 構造体の引用が存在せず、同節の記述が Phase 1 適用後の実装（`environment` パッケージはシステム環境の列挙と denylist 判定を提供し、allowlist は扱わない）と整合している。日本語版を先に修正し、英語版はそこから反映する。本 AC は Phase 1 の PR に含める。

## Success Criteria（要件レベル）

- AC-01〜AC-23、AC-25、AC-26（AC-24 を採る場合はその代替を含む）のすべてに対し、実装計画（[03_implementation_plan.md](03_implementation_plan.md)）で具体的なテストまたは静的検証手段（`grep` による不存在確認など）が対応付けられている。
- 本タスクは AC-25 が扱う一点を除き挙動不変のリファクタリングであり、削除対象に直接依存していたテストを除き、既存テストが無修正で pass する。
- Phase 1〜4 のそれぞれが独立してレビュー可能な単位に分かれており、いずれかを見送っても他 Phase の成果が成立する。
- `make fmt` / `make lint` / `make test` がグリーンである。
