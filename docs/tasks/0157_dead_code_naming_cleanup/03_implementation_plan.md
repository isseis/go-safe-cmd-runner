# 実装計画書: フィルタ未実装・命名と実装の乖離の整理（デッドコード削除含む）

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-26 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連文書

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)
- 要件プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テスト構成ガイド: [test_organization.md](../../dev/developer_guide/test_organization.md)
- パッケージ構成リファレンス: [package_reference.md](../../dev/developer_guide/package_reference.md)

---

## 1. 実装概要

### 1.1 目的

防御を示唆する名前（`Filter`・`EUID`・`changeUserGroup`）と実装のずれを取り除き、本番から到達しないコードを削除する。設計上の判断はすべて [02_architecture.md](02_architecture.md) に記されているため、本書は「どのファイルをどう変えるか」と「どう検証するか」だけを扱う。

### 1.2 実装方針

- **挙動不変を原則とする。** 例外は Phase 3 の一点（passwd エントリを引けない場合の fail-closed → fail-open）のみで、[02_architecture.md](02_architecture.md) §5.4 に記載済みである。
- **Phase 間に依存を作らない。** 4 つの Phase は別パッケージを触る。任意の順序で実施でき、いずれかを見送っても他が成立する（§8.1）。
- **1 Phase = 1 PR。** 各 PR の完了時に `make fmt` → `make test` → `make lint` を実行し、すべて成功した状態でのみマージする。
- **Go ソースに書く識別子・コメント・文字列リテラルはすべて英語とする。** 本書の日本語は計画の記述にのみ用いる。

### 1.3 既存コード調査結果

実装前にリポジトリ全体を調査した。設計書に記載のない事項、および設計書の記載を補正すべき点を以下に挙げる。手当てが不要な箇所は省略した。

#### Phase 1（`environment` / `runner` / `executor` / `config`）

| 対象 | 現状 | 対応 |
|---|---|---|
| `internal/runner/base/environment/filter.go` | `Filter` 型・`NewFilter`・`ParseSystemEnvironment`・`Source` 型・`FilterSystemEnvironment`・`FilterGlobalVariables`・未使用 sentinel 6 個を含む。パッケージコメントは "allowlist-based access control" と述べる | `system_env.go` へ改名し `ParseSystemEnvironment` のみを残す |
| `internal/runner/base/executor/environment.go` | `getSystemEnvironment`（:106-114）を削除すると、`os` import（使用箇所は :108 のみ）と `common` import（使用箇所は :109 のみ）の**両方**が未使用になる。設計書 §3.5 は `os` のみに言及している | 両 import を削除する |
| `internal/runner/runner.go` | `environment` パッケージの参照は `envFilter` フィールド（:69）と `NewFilter` 呼び出し（:323）の 2 箇所のみ。したがって import ごと削除できる | import を削除する |
| `internal/runner/config/expansion.go` | `environment` の参照は他に `IsForbiddenEnvVar`（:326, :807）があるため import は残る | :866 の呼び出しのみ差し替える |
| `cmd/runner/integration_workdir_test.go` | :195 と :630 は `err := r.LoadSystemEnvironment()` で `err` を**宣言**しており、後続の `err = r.Execute(...)`（:201, :636）が同じ変数を再代入している。呼び出しを消すだけでは後続がコンパイルできない | 後続を `err := r.Execute(...)` に変更する |
| `internal/runner/runner_test.go`（6 箇所）/ `cmd/runner/integration_auto_vars_test.go` / `cmd/runner/integration_test_helpers.go` / `internal/runner/e2e_*_test.go` | いずれも `err = ` または `require.NoError(t, r.LoadSystemEnvironment())` の形で、`err` は別の行で宣言済み | 該当行の削除のみで足りる |
| `cmd/runner/integration_workdir_test.go:189` | ヘルパー `executeRunnerWithTimeout` の doc コメントが "executes a runner with LoadSystemEnvironment and ExecuteFiltered" と述べている | 削除に合わせて書き換える |
| `docs/dev/developer_guide/package_reference.md`（:29, :87） | `runner/base/environment/` を "Environment variable processing and filtering" と説明している。設計書 §3.6 が「変更不要」としているのはディレクトリ**一覧**の増減であり、説明文の正確さは別問題である | 説明文を実態（列挙と denylist 判定）に合わせる |
| 存在しないパッケージパス `internal/runner/environment/` を指す文書 | `docs/dev/` に 8 箇所ある（`security-architecture.{ja.,}md` 各 1、`config-inheritance-behavior.{ja.,}md` 各 2、`design-implementation-overview.{ja.,}md` 各 1）。パッケージは `internal/runner/base/environment/` へ移動済みで、Phase 1 後は参照先のファイル名も変わる。設計書 §7.3 と AC-26 が明示しているのは `security-architecture` の 2 件だけである | Phase 1 の PR で 8 箇所すべてを修正する。AC-26 の対象外の 6 箇所を放置すると、AC-26 が防ごうとしている「記述と実装の乖離」が同じ文書群に残ってしまうためである |
| `IsVariableAccessAllowed` | Go ソースでの参照は `filter.go:41` のコメント 1 箇所のみ。他の一致はすべて `docs/tasks/` 配下の過去タスク文書であり、当時の記録として残す | `filter.go` の削除で解消する |
| `ErrGroupNotFound` | `environment`（削除対象）・`internal/runner/runner.go:36`・`internal/runner/cli/filter.go:13` の三重定義。後 2 者は現役 | `environment` 側のみ削除する |
| `Filter` という語 | Go ソース全体では `FilterGroups`（`internal/runner/cli/`）・`FilteredVariables`（`internal/runner/resource/`）・`FilterSyscallsForStorage` の言及・`ExecuteFiltered` が、本タスクとは無関係な用法として存在する | AC-02 の検索式は完全一致の識別子と `environment` パッケージ配下に限定する（§7 参照） |

#### Phase 2（`privilege` / `runnertypes`）

| 対象 | 現状 | 対応 |
|---|---|---|
| `changeUserGroupInternal` の非 dry-run 部（unix.go:569-588） | `m.syscallSetegid` / `m.syscallSeteuid` / EGID ロールバック / `emergencyShutdown` を含む | 削除する |
| `buildUserGroupLogAttrs`（unix.go:460） | 呼び出しは dry-run ログ（:559）と成功ログ（:582）の 2 箇所。後者が消えるため呼び出し元は 1 箇所になる | 関数は残す。呼び出し元が 1 つの非公開ヘルパーは Go では珍しくなく、削除を求める AC もない |
| `performElevation` のエラーラップ文言 `"user/group change failed: %w"`（unix.go:187） | 改名後は「変更」ではなく「解決」の失敗を表す。この文字列は dry-run 失敗時に `Impact.Description` へ現れる | AC-14 の趣旨（"change" を名乗らない）に従い文言を変更する。§2.4 に完全な前後文字列を示す |
| `m.logger.Info("User/group change requested", ...)`（unix.go:487） | `dryRun` 引数を属性に含めている。引数ごと削除されるため両方が成立しなくなる | 文言と属性を書き換える（§2.4） |
| `executionContext` リテラル | 本番 1 箇所（unix.go:145）とテスト 15 箇所。`needsUserGroupChange` を設定するのはテストのリテラル 12 箇所（`unix_privilege_test.go` 10・`identity_linux_test.go` 1・`identity_other_test.go` 1）で、これに加えて `TestPrepareExecution_Success` の表項目 `expectedUserGroupChange`（フィールド宣言 1・値 3・アサーション 1）がある。`originalEUID` / `originalEGID` を設定するのは `unix_privilege_test.go:616-617` の 1 箇所 | フィールド削除に合わせて該当行を削除する。12 のリテラルはいずれも `elevationCtx.Operation` を設定済みであることを確認済みのため、設計書 §3.5 が言う「`elevationCtx.Operation` の設定へ置換」は行削除だけで足りる |
| `TestHandleCleanupAndMetrics_Success`（unix_privilege_test.go:277） | `Operation` は `OperationUserGroupDryRun`、`needsUserGroupChange` は `false`。コメントが「`needsUserGroupChange` が false なので metrics のアサーションは不要」と述べる。operation 直接判定に置き換えると、この文脈では metrics が記録されるようになる（アサーションが無いためテストは通るが、コメントが事実と食い違う） | コメントを書き換える。本番では `prepareExecution` が operation と整合した文脈しか作らないため挙動差はない |
| `TestPerformElevation_Failure/invalid_user_in_dryrun`（unix_privilege_test.go:260） | `assert.Contains(t, err.Error(), "user/group change failed")` | 新しいラップ文言に合わせて更新する |
| `MockPrivilegeManager`（2 実装） | `internal/runner/base/privilege/testutil/mocks.go`（:61, :74）と `internal/runner/resource/normal_manager_test.go`（:81, :86）。`runnertypes.PrivilegeManager` を実装するモックはこの 2 つだけである。後者は testify モックだが `.On("WithUserGroup")` / `.On("IsUserGroupSupported")` の期待値設定はリポジトリ内に存在しない | 両方から 2 メソッドを削除する。期待値設定が無いため他テストへの影響はない |
| `syscall.Seteuid` / `Setegid` の出現箇所 | 非テストコードでは `unix.go:68-69`（注入フィールド初期化）、`unix.go:294`（`Seteuid(0)`）、`unix.go:324`（`Seteuid(m.originalUID)`）の 3 箇所と、`unix.go:116, :121` のコメント。`Setresuid` / `Setresgid` / `Setgroups` の直接呼び出しは無い（`identity_linux.go` は `unix.Getresuid` / `Getresgid` の**読み取り**のみ） | Phase 2 後は :294 と :324 の 2 箇所だけが残る。これを機械的に固定する（§2.4） |
| `ErrInsufficientPrivileges`（unix.go:20） | 返却も比較もされていない未使用 sentinel。AC-05 と同型の劣化だが、要件定義書のスコープは `environment` パッケージの 6 個に限られる | 本タスクでは削除しない（スコープ外の観察として記録する） |
| `TestIsUserGroupSupported`（unix_privilege_test.go:419） | 削除対象 API のテストだが、`privilegeSupported` が `true` / `false` の両方で正しく返ることを固定している**唯一の**テストである。現存する `IsPrivilegedExecutionSupported` の参照はすべて skip ガード（`race_test.go`、`manager_test.go:93`）かモックの期待値設定であり、返り値そのものを検証していない | 削除ではなく `TestIsPrivilegedExecutionSupported` に転用し、検証対象メソッドだけを差し替える |
| `internal/runner/base/executor/executor_usergroup_integration_test.go` / `privileged_test_condition_test.go` | `//go:build integration` かつ `executor/testutil`（`//go:build test \|\| performance`）を import するため、コンパイルには `-tags 'test integration'` の**両方**が必要である。これらをコンパイルする Makefile ターゲットは存在しない（`-tags integration` を使うのは `elfanalyzer-integration-test` と `libccache-integration-test` のみ）。前者は `executor.WithPrivilegeManager(privMgr)` を通じて `runnertypes.PrivilegeManager` に依存する | 各 Step の完了条件に `go vet -tags 'test integration' ./...` を加える。現状このコマンドは成功する（実行して確認済み）ため、Phase 2 のインターフェース変更で壊れれば同じ PR 内で検出できる |

#### Phase 3（`groupmembership`）

| 対象 | 現状 | 対応 |
|---|---|---|
| `os/user` import（manager.go:8） | `getProcessEUID` 以外に `user.LookupId`（:138, :177）で使用 | import は残す |
| `effectiveUID` ローカル変数（manager.go:305, 312, 315, 327） | `getPermissionCheckUID` の戻り値を "effective" と呼んでおり、実 UID または `SUDO_UID` である実態と食い違う。`#nosec G115` の根拠コメントもこの名前を参照している | `permissionCheckUID` に改名する。これにより AC-19 の検証を「`manager.go` に `EUID` という語が現れない」という機械的な検査にできる |
| `getPermissionCheckUID` の sudo 分岐 | 実 UID と `SUDO_UID` を直接読むため、実 UID が 0 のケース（AC-20 の (b)）は root で実行しないとテストできない | 純関数 `resolvePermissionCheckUID(realUID int, sudoUID string)` を切り出す。既存の `parseSudoUID` が「独立してテストするために分離した」と doc に書いている前例に倣う。`getPermissionCheckUID` の外部シグネチャは設計書 §3.3 のとおり変えない |
| passwd エントリ欠如時の挙動（AC-25）**その 1: 残る passwd 依存** | `user.Current()` を消しても、権限判定から passwd 依存が完全に消えるわけではない。`manager.go:138` の `IsUserInGroup` → `user.LookupId` は読み取り経路から、`manager.go:177` の `isUserOnlyGroupMember` → `user.LookupId` は書き込み経路（`CanUserSafelyWriteFile`:237）から、**ファイルがグループ書き込み可能なとき**に呼ばれる。設計書 §5.4 が「変更後は拒否は起きない」と述べるのは、この分岐に入らない通常のファイル（0644・0600 など、グループ書き込みビットが立っていない）についてである | Phase 3 のスコープは UID の取得経路に限る。この調査結果を受けて要件定義書を改訂し、AC-25 の対象を「権限判定に用いる UID の取得」に限定した（2026-07-26、要件定義書の「方針判断の記録」の「D1 M-4 の派生」および対象外節）。`LookupId` 側の fail-closed は [0151](../0151_groupmembership_failclosed/) が意図して導入したものであり、本タスクでは変更しない。グループ書き込み可能なファイルに対する判定が引き続き passwd を必要とすることは、`CHANGELOG.md` と §4.6 に明記する |
| passwd エントリ欠如時の挙動（AC-25）**その 2: テスト手段** | Go の `os/user` は cgo 有無を問わず passwd の探索先を差し替える手段を提供していないため、単体テストの中で「NSS 障害」や「`/etc/passwd` 欠如」を再現することはできない | UID の取得経路から passwd 参照を無くしたことを、実行可能テストと静的検査で確認する（§7） |
| `TestGetPermissionCheckUID`（manager_test.go:599） | 既に存在する。ただし sudo 分岐（実 UID が 0）のケースは root で実行しないと通らず、`t.Skip` される。AC-20 の 3 ケースを決定的に検証してはいない | 新しい重複テストを足さず、この既存テストを純関数 `resolvePermissionCheckUID` のテストで補い、既存テスト自体には具体値のアサーションを追加する |
| `CHANGELOG.md` | `## [Unreleased]` 節が存在しない（直近は `## [1.0.0] - 2026-06-27`） | `## [Unreleased]` 節を新設して AC-25 の挙動変化を記載する |

#### Phase 4（`fileanalysis` / `elfanalyzer`）

| 対象 | 現状 | 対応 |
|---|---|---|
| `make deadcode` | `syscall_store.go` の 3 関数を到達不能として報告する（実行して確認済み） | Phase 4 後にこの 3 行が消えることを確認する |
| `internal/fileanalysis/syscall_store_test.go` | 10 個のテスト関数を含むが、末尾 2 つは**削除対象 API の契約ではなく現役の `Store` の挙動**を検証している。設計書 §3.5 の「ファイル削除」をそのまま実行すると、この 2 つが失われる | **直後の 2 行**のとおり個別に判断する |
| └ `TestStore_ArgEvalResults`（:382） | `ArgEvalResults` の JSON 往復（`omitempty` により nil が nil のまま戻ること）を検証している。同等のカバレッジは `file_analysis_store_test.go` にも `filevalidator` のテストにも存在しない | `Store.Update` / `Store.Load` を直接使う形へ書き換え、`file_analysis_store_test.go` へ移す |
| └ `TestLoad_SchemaVersion17_ReturnsSchemaVersionMismatchError`（:449） | 旧スキーマ版の記録が `SchemaVersionMismatchError` になることを検証している。`file_analysis_store_test.go` に `TestStore_SchemaVersionMismatch`（版 999）・`TestStore_Load_V21RejectedWithSchemaVersionMismatch`（版 21）・`TestStore_Load_V22RejectedWithSchemaVersionMismatch`（版 22）の同等テストが既にある | 冗長なため削除する |
| └ `TestSyscallAnalysisStore_SaveSortsDetectedSyscallsByNumber`（:183）/ `TestSyscallAnalysisStore_GroupingBehavior`（:274） | 実質的に `common.GroupAndSortSyscalls` の挙動（番号昇順・`-1` を末尾・同一番号の Occurrences マージと Location 昇順・空でない Name の優先）を検証している。**`internal/common` にこの関数のテストファイルは存在しない** | `internal/common/syscall_grouping_test.go` を新設し、同じ不変条件を関数の直下で検証する |
| └ 残る 6 個 | `SaveAndLoad` / `HashMismatch` / `NoSyscallAnalysis` / `RecordNotFound` / `AnalysisWarnings` / `UpdatePreservesOtherFields`。前 5 つは削除対象 API の契約そのもの、最後の 1 つは §2.4.3 が「修正しない」と判断した不整合の挙動を固定している。`AnalysisWarnings` の JSON 往復は `file_analysis_store_test.go:214, :234` が既にカバーしている | 削除する |
| `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` | 削除対象は `TestE2E_RecordToRunnerFallbackChain`。doc コメントは :208 から、関数は :215-303。設計書 §3.5 の「:261-290」は関数の一部しか指していない。削除後は `fileanalysis` / `filevalidator` / `binaryanalyzer` の 3 import が未使用になる（`tu` / `exec` / `filepath` / `os` / `runtime` は他テストが使用） | doc コメントを含む関数全体と 3 import を削除する |
| └ 同テストが追加で固定している不変条件 | 削除対象 API の契約に加えて、`StandardELFAnalyzer.convertSyscallResult` が保存済み解析結果から `binaryanalyzer.NetworkDetected` を導くこと（:294-299）も検証している | この不変条件は `internal/security/elfanalyzer/analyzer_test.go::TestStandardELFAnalyzer_SyscallLookup_NetworkDetected`（`make test` で実行される）が引き続きカバーするため、代替テストは不要である |
| ビルドタグ | 同ファイルは `//go:build integration` であり `make test` では**コンパイルされない**。`make elfanalyzer-integration-test` が `-tags integration` で実行する | Phase 4 の完了ゲートに `make elfanalyzer-integration-test` を含める |
| `common.GroupAndSortSyscalls` / `fileanalysis.ErrHashMismatch` | 削除後も `filevalidator/validator.go`（3 箇所）と `elfanalyzer/standard_analyzer.go:301` が使用する | いずれも残す |

---

## 2. 実装ステップ

実施順序は設計書 §8.2 の推奨に従う。Phase 番号は設計書と同じ意味（パッケージ単位）で用い、Step 番号が実施順序を表す。

### 2.1 Step 1 = Phase 4: `fileanalysis` の未使用ストアの削除

**設計参照**: [02_architecture.md](02_architecture.md) §2.4、§3.4

**変更対象ファイル**

- `internal/fileanalysis/syscall_store.go`（削除）
- `internal/fileanalysis/syscall_store_test.go`（削除）
- `internal/fileanalysis/file_analysis_store_test.go`（テスト追加）
- `internal/common/syscall_grouping_test.go`（新規）
- `internal/security/elfanalyzer/syscall_analyzer_integration_test.go`（テスト 1 件削除）

**作業内容**

- [ ] `internal/common/syscall_grouping_test.go` を新規作成し、`TestGroupAndSortSyscalls` を実装する。次のケースを表駆動で網羅する。
  - [ ] 空スライスを渡すと `nil` が返る（境界値）
  - [ ] 番号が昇順に並び替えられる（入力 257, 41, 1, 42 → 1, 41, 42, 257）
  - [ ] 番号 `-1`（番号を特定できなかったエントリ）が末尾に置かれる
  - [ ] 同一番号の複数エントリが 1 つに統合され、`Occurrences` が Location 昇順に並ぶ
  - [ ] 先行エントリの `Name` が空で後続が非空のとき、非空の `Name` が採用される
- [ ] `internal/fileanalysis/file_analysis_store_test.go` に `TestStore_ArgEvalResultsRoundtrip` を追加する。`Store.Update` で `record.SyscallAnalysis` に `ArgEvalResults` を設定し、`Store.Load` で読み戻す。サブテストは 2 つ。
  - [ ] `ArgEvalResults` に 1 件設定した場合、`SyscallName` / `Status` / `Details` が往復後も一致する
  - [ ] `ArgEvalResults` が nil の場合、往復後も nil のままである（`omitempty` の確認）
- [ ] `internal/fileanalysis/syscall_store.go` をファイルごと削除する。
- [ ] `internal/fileanalysis/syscall_store_test.go` をファイルごと削除する。
- [ ] `internal/security/elfanalyzer/syscall_analyzer_integration_test.go` から `TestE2E_RecordToRunnerFallbackChain` を、その doc コメント（"TestE2E_RecordToRunnerFallbackChain tests the full pipeline..." で始まるブロック）ごと削除する。
- [ ] 同ファイルの import から `fileanalysis` / `filevalidator` / `binaryanalyzer` の 3 つを削除する。

**完了条件**

- [ ] `make fmt` → `make test` → `make lint` がすべて成功する
- [ ] `go vet -tags 'test integration' ./...` が成功する（ビルドタグ付きファイルの型検査。§1.3 参照）
- [ ] `make elfanalyzer-integration-test` が成功する（`-tags integration` での実行を含む）
- [ ] `make deadcode` の出力に `internal/fileanalysis/syscall_store.go` の 3 行が現れない
- [ ] `make build` が成功する

### 2.2 Step 2 = Phase 3: `getProcessEUID` の命名と実装の整合

**設計参照**: [02_architecture.md](02_architecture.md) §2.3、§3.3、§5.4

**変更対象ファイル**

- `internal/groupmembership/manager.go`
- `internal/groupmembership/manager_test.go`
- `CHANGELOG.md`

**作業内容: 実装**

- [ ] `getProcessEUID` を `getProcessRealUID` に改名し、本体を `os.Getuid()` に置き換える。`user.Current()` と `strconv.Atoi` による UID 取得を削除する。範囲検査（`< 0` / `> math.MaxUint32` → `ErrUIDOutOfBounds`）は、到達しなくなっても残す。設計書 §2.3.3 のとおり、`CanCurrentUserSafelyReadFile` の `#nosec G115` がこの検査を根拠として挙げているためである。この理由は doc コメントにも明記する。
- [ ] 純関数 `resolvePermissionCheckUID(realUID int, sudoUID string) (int, error)` を新設し、sudo 分岐（実 UID が 0 かつ `SUDO_UID` が非空なら `parseSudoUID`、それ以外は実 UID）をここへ移す。`getPermissionCheckUID` は `getProcessRealUID()` と `os.Getenv("SUDO_UID")` を渡して委譲する形にする。外部から見たシグネチャは変えない。分離の目的は既存の `parseSudoUID` と同じで、root で実行しなくても全分岐をテストできるようにすることである。
- [ ] `CanCurrentUserSafelyReadFile` 内のローカル変数 `effectiveUID` / `effectiveUID32` を `permissionCheckUID` / `permissionCheckUID32` に改名する。参照は :305, :312（コメント）, :315, :327 の 4 箇所すべてを更新する。
- [ ] 下の「作業内容: コメントの書き換え」の 6 項目をすべて適用する。

**作業内容: コメントの書き換え**

`manager.go` から `EUID` / `effective UID` / `effective user` / `effectiveUID` という語をすべて取り除く（変更前は 17 行が一致する。AC-19 は、これらの語が 1 つも現れないことをもって検証する）。以下は完全な前後の文字列である。なお AC-19 の検証式は `EUID` という語を全面的に禁じるため、「実 UID であって実効 UID ではない」という趣旨を書きたい場合も "effective UID" ではなく "real UID (the UID the kernel reports for getuid(2))" のように言い換える。

- [ ] `CanCurrentUserSafelyWriteFile` の doc コメントの `error` 説明。

  前: `//   - error: non-nil if there was an error getting current user info or checking permissions`
  後: `//   - error: non-nil if the process UID could not be determined or the permission check failed`

- [ ] `CanCurrentUserSafelyWriteFile` の本体先頭コメント（:276-278）。

  前:
  ```go
  	// For write operations, use the actual EUID (not SUDO_UID) to verify
  	// that the running process has permission to write to the file.
  	// This is important for hash files that should only be writable by root.
  ```
  後:
  ```go
  	// For write operations, use the process's real UID (not SUDO_UID) to verify
  	// that the running process has permission to write to the file.
  	// This is important for hash files that should only be writable by root.
  ```

- [ ] `CanCurrentUserSafelyReadFile` 内のコメント（:310-315）。

  前:
  ```go
  	// For reads with group-writable permissions: deny only if effective user is NOT in the group
  	// Convert userUID to uint32 for IsUserInGroup call
  	// #nosec G115 -- safe: `effectiveUID` represents a system user ID (UID), which is
  	// non-negative and constrained by the operating system to fit within a 32-bit
  	// unsigned value on supported platforms. It was already validated in getPermissionCheckUID().
  ```
  後:
  ```go
  	// For reads with group-writable permissions: deny only if the permission-check user is NOT in the group
  	// Convert the UID to uint32 for the IsUserInGroup call
  	// #nosec G115 -- safe: `permissionCheckUID` represents a system user ID (UID), which is
  	// non-negative and constrained by the operating system to fit within a 32-bit
  	// unsigned value on supported platforms. It was already validated in getPermissionCheckUID().
  ```

- [ ] `getPermissionCheckUID` の doc コメント（:432-444）。

  前:
  ```go
  // getPermissionCheckUID returns the effective user ID for permission checks.
  // When running under sudo (EUID is 0 and SUDO_UID is set), it returns the original user's UID.
  // Otherwise, it returns the current user's UID.
  ```
  後:
  ```go
  // getPermissionCheckUID returns the user ID to use for permission checks.
  // When running under sudo (the real UID is 0 and SUDO_UID is set), it returns the
  // original user's UID taken from SUDO_UID. Otherwise it returns the process's real UID.
  ```
  および、前: `//   - int: The effective UID to use for permission checks`
  後: `//   - int: The UID to use for permission checks`

- [ ] `getPermissionCheckUID`（改名後は `resolvePermissionCheckUID`）内の sudo 判定インラインコメント。

  前: `	// Check if running under sudo: EUID must be 0 (root) and SUDO_UID must be set`
  後: `	// Check if running under sudo: the real UID must be 0 (root) and SUDO_UID must be set`

- [ ] `getProcessEUID` の doc コメント（:481-489）。

  前:
  ```go
  // getProcessEUID returns the current user's EUID without considering SUDO_UID.
  // This returns the actual EUID of the running process.
  //
  // This function is primarily used for write operations where we want to verify
  // the actual running process has the necessary permissions to write files.
  //
  // Returns:
  //   - int: The current user's EUID
  //   - error: Error if unable to determine the EUID
  ```
  後:
  ```go
  // getProcessRealUID returns the process's real UID, without considering SUDO_UID.
  // It reads the UID from the kernel via os.Getuid() and does not consult the passwd
  // database, so it does not depend on NSS or /etc/passwd.
  //
  // This function is primarily used for write operations where we want to verify
  // the running process has the necessary permissions to write files.
  //
  // The bounds check below cannot fail in practice because os.Getuid() always
  // succeeds and returns a valid UID. It is kept because CanCurrentUserSafelyReadFile
  // suppresses gosec G115 on the uint32 conversion with the stated justification that
  // the value was already validated here; removing the check would remove that ground.
  //
  // Returns:
  //   - int: The process's real UID
  //   - error: ErrUIDOutOfBounds if the UID does not fit in uint32
  ```

**作業内容: テスト**

- [ ] `manager_test.go` の `TestGetProcessEUID`（:692）を `TestGetProcessRealUID` に改名し、内容を次のとおり書き換える。
  - [ ] `os.Getuid()` と同じ値を返し、エラーを返さないことを検証する
  - [ ] `t.Setenv("SUDO_UID", "9999")` を設定しても返り値が `os.Getuid()` のままであることを検証する
- [ ] `TestResolvePermissionCheckUID` を新規追加し、AC-20 の 3 ケースを root 権限なしで決定的に検証する。純関数なので実 UID を引数で与えられる。
  - [ ] (a) `SUDO_UID` が空文字列のとき、渡した実 UID がそのまま返る（実 UID が `0` の場合と `1000` の場合の両方）
  - [ ] (b) 実 UID が `0` かつ `SUDO_UID` が `"1000"` のとき、`1000` が返る
  - [ ] (c) 実 UID が `1000` かつ `SUDO_UID` が `"2000"` のとき、`1000` が返る
  - [ ] 実 UID が `0` かつ `SUDO_UID` が不正値（`"-1"`、`"4294967296"`、`"abc"`）のとき、エラーが返る（エラー経路）
- [ ] 既存の `TestGetPermissionCheckUID`（:599）に、実装の再計算ではなく具体値で判定するサブテストを追加する。新規のテスト関数は追加しない（既存テストと重複するため）。
  - [ ] `t.Setenv("SUDO_UID", "")` のとき、返り値が `os.Getuid()` に等しい
  - [ ] `t.Setenv("SUDO_UID", "9999")` のとき、非 root で実行していれば返り値が `os.Getuid()` に等しく、root で実行していれば `9999` に等しい（実行 UID による分岐を明示し、どちらの環境でも具体値を検証する）
- [ ] `TestCanCurrentUserSafelyWriteFile_UsesRealUID` を新規追加し、実装の再計算ではなく**判定結果そのもの**を検証する。`t.TempDir()` に所有者と権限が既知のファイルを作り、次を確認する。
  - [ ] `0o600`（所有者のみ書き込み可、テスト実行ユーザーが所有）→ `true` が返る
  - [ ] `0o666`（誰でも書き込み可）→ `false` と `ErrFileWorldWritable` が返る（`errors.Is` で判定する）
  - [ ] `0o400`（誰も書き込めない）→ `false` と `ErrFileNotWritable` が返る（境界値・エラー経路）
  - [ ] いずれのケースでも `CanUserSafelyWriteFile(os.Getuid(), ...)` と同じ結果になる（UID の出所が `os.Getuid()` のみであることの確認。ただしこれは補助であり、上の 3 つの具体値のアサーションが主である）

**作業内容: リリースノート**

- [ ] `CHANGELOG.md` の冒頭（`## [1.0.0]` の直前）に `## [Unreleased]` 節と `### Changed` 小節を新設し、次の内容を英語で記載する。
  - [ ] 実 UID に対応する passwd エントリを引けない環境（cgo 有効時の NSS 障害、cgo 無効時の `/etc/passwd` 欠如やエントリ未登録）で、これまで権限チェックがファイルアクセスを拒否して実行そのものが停止していたが、**権限チェックに用いる UID の取得**が passwd を参照しなくなったため実行が継続するようになったこと
  - [ ] これは fail-closed から fail-open への変化であること
  - [ ] 判定に使う UID・GID・パーミッションビットと判定規則は変わらないこと
  - [ ] **グループ書き込み可能なファイル**に対する判定はグループメンバーシップの照会（`user.LookupId`）を行うため、その場合は引き続き passwd エントリを必要とすること（§1.3 参照）

**完了条件**

- [ ] `make fmt` → `make test` → `make lint` がすべて成功する。`make test`（= `unit-test`）は非 Darwin では `CGO_ENABLED=1 -race` と `CGO_ENABLED=0` の 2 回テストを実行するため、設計書 §7.5 が求める cgo 両構成の確認はこの時点で済んでいる
- [ ] `go vet -tags 'test integration' ./...` が成功する
- [ ] マージ前に CI の 2 レグ（`make test-ci-cgo1` / `make test-ci-cgo0`）が成功する

### 2.3 Step 3 = Phase 1: `environment` パッケージの縮退と `Runner` のデッドコード削除

**設計参照**: [02_architecture.md](02_architecture.md) §2.1、§3.1、§4.1、§4.2、§7.3

**変更対象ファイル**

- `internal/runner/base/environment/filter.go` → `system_env.go`
- `internal/runner/base/environment/filter_test.go` → `system_env_test.go`
- `internal/runner/base/environment/filter_benchmark_test.go`（削除）
- `internal/runner/base/executor/environment.go`
- `internal/runner/config/expansion.go`
- `internal/runner/config/expansion_test.go`（テスト追加）
- `internal/runner/runner.go`、`internal/runner/runner_test.go`
- `cmd/runner/main.go`、`cmd/runner/integration_workdir_test.go`、`cmd/runner/integration_auto_vars_test.go`、`cmd/runner/integration_test_helpers.go`
- `internal/runner/e2e_shebang_test.go`、`internal/runner/e2e_dynlib_verification_test.go`
- `docs/dev/architecture_design/security-architecture.ja.md` / `security-architecture.md`
- `docs/dev/architecture_design/config-inheritance-behavior.ja.md` / `config-inheritance-behavior.md`
- `docs/dev/developer_guide/design-implementation-overview.ja.md` / `design-implementation-overview.md`
- `docs/dev/developer_guide/package_reference.md`

**作業内容: `environment` パッケージ**

- [ ] `filter.go` を `system_env.go` に `git mv` する。
- [ ] `system_env.go` から `Filter` 型と `globalAllowlist` フィールドを削除する。
- [ ] `NewFilter` を削除する。
- [ ] `FilterSystemEnvironment` を削除する。
- [ ] `FilterGlobalVariables` を削除する（空変数名の `slog.Warn` 分岐を含む。`common.ParseKeyValue` が空キーを拒否するため到達不能である。設計書 §4.2）。
- [ ] `Source` 型と定数 `SourceSystem` / `SourceEnvFile` を削除する。
- [ ] 未使用 sentinel 6 個（`ErrGroupNotFound` / `ErrVariableNameEmpty` / `ErrInvalidVariableName` / `ErrDangerousVariableValue` / `ErrVariableNotFound` / `ErrVariableNotAllowed`）と、それらを囲む `var` ブロックおよび "Note: ErrMalformedEnvVariable is defined in config package..." コメントを削除する。
- [ ] `ParseSystemEnvironment` をメソッドからパッケージ関数 `func ParseSystemEnvironment() map[string]string` に変更する。本体（`os.Environ()` の走査と `common.ParseKeyValue` による分解）は変更しない。
- [ ] パッケージコメントと `ParseSystemEnvironment` の doc コメントを次のとおり書き換える。AC-02 の検証式が `internal/runner/base/environment/` に `Filter` という語が現れないことを要求するため、書き換えた文中に `Filter` / `filter` を使わないこと。

  前:
  ```go
  // Package environment provides environment variable filtering and management functionality
  // for secure command execution with allowlist-based access control.
  package environment
  ```
  後:
  ```go
  // Package environment enumerates the system environment and decides which variable
  // names must never reach a child process (see denylist.go). It applies no allowlist:
  // the effective allowlist checks live in the config layer (ProcessEnvImport) and the
  // executor layer (BuildProcessEnvironment).
  package environment
  ```

  前:
  ```go
  // ParseSystemEnvironment parses os.Environ() and returns all environment variables as a map.
  // No filtering is applied - use IsVariableAccessAllowed for filtering.
  ```
  後:
  ```go
  // ParseSystemEnvironment returns every entry of os.Environ() that parses as key=value.
  // No access control is applied here; see the package comment for where the allowlist
  // and denylist checks live.
  ```

- [ ] 未使用になる `errors` / `log/slog` の import を削除する（`os` と `common` は残る）。
- [ ] `filter_benchmark_test.go` を削除する。

**作業内容: `environment` パッケージのテスト**

- [ ] `filter_test.go` を `system_env_test.go` に `git mv` し、全面的に書き換える。
- [ ] `TestNewFilter` を削除する（対象 API が消える）。
- [ ] `TestNewFilter_WithAllowlist` を削除する（`globalAllowlist` の存在と要素数のみを検証しており、対象 API が消える）。
- [ ] `TestFilterSystemEnvironment` を削除する。
- [ ] `TestFilterGlobalVariables_SourceSystem` を削除する（allowlist 外の変数が通過することを期待値として固定しており、対象 API が消える）。
- [ ] `TestFilterGlobalVariables_SourceEnvFile` を削除する。
- [ ] `TestFilterGlobalVariables_EmptyVariableName` を削除する（検証対象の分岐が到達不能であり削除される）。
- [ ] `TestFilterGlobalVariables_EmptyValue` を削除する（後述の `TestParseSystemEnvironment` が空値を引き継ぐ）。
- [ ] `TestFilterGlobalVariables_SpecialCharactersInValue` を削除し、値に改行・タブ・引用符を含む検証を `TestParseSystemEnvironment_SpecialCharactersInValue` として新 API 向けに引き継ぐ。
- [ ] `TestFilterGlobalVariables_EmptyMap` を削除する。
- [ ] `TestParseSystemEnvironment` と `TestParseSystemEnvironment_EmptyEnvironment` をパッケージ関数呼び出しに合わせて書き換える（`NewFilter([]string{})` の生成を除去する）。
- [ ] `TestParseSystemEnvironment` に、`=` を含まないエントリが除外されることの検証を追加する。`os.Environ()` に `=` を含まない値を注入することはできないため、`t.Setenv` で通常のエントリを設定したうえで、返り値のキーに空文字列が含まれないことを検証する形にする。
- [ ] 不要になる `runnertypes` import を削除する。

**作業内容: `executor` の重複削除**

- [ ] `internal/runner/base/executor/environment.go` の `getSystemEnvironment` を削除する。
- [ ] `BuildProcessEnvironment` の Step 1（:57）を `systemEnv := environment.ParseSystemEnvironment()` に変更する。
- [ ] 未使用になる `os` と `common` の import を削除する（`environment` / `runnertypes` は残る）。

**作業内容: `config` 層**

- [ ] `internal/runner/config/expansion.go:866` を `runtime.SystemEnv = environment.ParseSystemEnvironment()` に変更する。
- [ ] `internal/runner/config/expansion_test.go` に `TestExpandGlobal_SystemEnvIncludesAllParsableEntries` を追加する。`t.Setenv` で `EnvAllowed` に含まれない変数を設定し、`ExpandGlobal` の結果の `runtime.SystemEnv` にその変数がキー・値ともに含まれること、および `environment.ParseSystemEnvironment()` の返り値と一致することを検証する（AC-03）。

**作業内容: `Runner` のデッドコード削除**

- [ ] `internal/runner/runner.go` から `envVars` フィールド（:66）を削除する。
- [ ] `envFilter` フィールド（:69）を削除する。
- [ ] `LoadSystemEnvironment` メソッド（:388-397）とその doc コメントを削除する。
- [ ] `NewRunner` 内の `envFilter` 生成（:322-323、"// Create environment filter" コメントを含む）を削除する。
- [ ] `Runner` リテラルの `envVars: make(map[string]string)`（:343）と `envFilter: envFilter`（:346）を削除する。
- [ ] `environment` の import を削除する。
- [ ] `cmd/runner/main.go` の `LoadSystemEnvironment()` 呼び出しブロック（:420-423、"// Load system environment variables" コメントを含む）を削除する。

**作業内容: 呼び出し元テストの更新**

`LoadSystemEnvironment` の呼び出し箇所は `rg -n 'LoadSystemEnvironment' --glob '*.go'` で列挙できる。すべて削除する。行番号ではなくこのパターンで特定する。

- [ ] `internal/runner/runner_test.go:72` の `assert.NotNil(t, runner.envVars)` を削除する（削除しないとコンパイルできない）。
- [ ] `internal/runner/runner_test.go` の 6 箇所の `err = runner.LoadSystemEnvironment()` と直後の `require.NoError` を削除する。あわせて直前の "// Load basic environment" 系コメントも削除する。いずれも `err` は `NewRunner` の行で宣言済みのため、後続行の修正は不要である。
- [ ] `cmd/runner/integration_workdir_test.go` の 2 箇所（ヘルパー `executeRunnerWithTimeout` 内と `TestIntegration_ErrorCleanup` 内）で、`err := r.LoadSystemEnvironment()` と `require.NoError(t, err)` を削除し、**後続の `err = r.Execute(ctx, nil)` を `err := r.Execute(ctx, nil)` に変更する**（`err` の宣言が失われるため）。
- [ ] `cmd/runner/integration_workdir_test.go:189` のヘルパー doc コメントを書き換える。

  前: `// executeRunnerWithTimeout executes a runner with LoadSystemEnvironment and ExecuteFiltered`
  後: `// executeRunnerWithTimeout runs the given runner under a timeout and requires it to succeed.`
- [ ] `cmd/runner/integration_auto_vars_test.go` の `err = r.LoadSystemEnvironment()` と `require.NoError` を削除する。
- [ ] `cmd/runner/integration_test_helpers.go` の `err = r.LoadSystemEnvironment()` と `require.NoError` を削除する。
- [ ] `internal/runner/e2e_shebang_test.go` の `require.NoError(t, r.LoadSystemEnvironment())` を削除する。
- [ ] `internal/runner/e2e_dynlib_verification_test.go` の 2 箇所の `require.NoError(t, r.LoadSystemEnvironment())` を削除する。

**作業内容: ドキュメントの追随**

- [ ] `docs/dev/architecture_design/security-architecture.ja.md` §3「環境変数分離」の「許可リストアーキテクチャ」にある `Filter` 構造体の引用を削除する。削除範囲は**開きフェンス ` ```go `（:166）から閉じフェンス（:172）までのブロック全体**とする（設計書 §7.3 が示す ":167-170" は中身の行だけを指しており、そのまま消すと閉じフェンスが孤立して以降のレンダリングが崩れる）。削除したブロックの代わりに、次の 2 点を記述する（設計書 §1.5 の内容を要約する）。
  - [ ] allowlist を適用しているのは `config.ProcessEnvImport`（`internal/runner/config/expansion.go`）と `executor.BuildProcessEnvironment`（`internal/runner/base/executor/environment.go`）の 2 箇所であること
  - [ ] `internal/runner/base/environment` はシステム環境の列挙（`system_env.go`）と denylist 判定（`denylist.go`）を提供し、allowlist は扱わないこと
- [ ] 同節の残りの記述（3 レベル継承モデル、継承モード）が Phase 1 適用後の実装と矛盾しないことを確認する。矛盾があれば同じ PR で修正する。
- [ ] `docs/dev/architecture_design/config-inheritance-behavior.ja.md` の :41 と :58 が `env_allowlist` 継承の実装として参照する `[filter.go](../../internal/runner/environment/filter.go)` を修正する。このリンクは現時点で既に壊れており（実際のパスは `internal/runner/base/environment/`）、Phase 1 後は参照先のファイル自体が存在しなくなる。継承の実際の実装先（`internal/runner/config/expansion.go` の `ProcessEnvImport` と `internal/runner/base/executor/environment.go` の `BuildProcessEnvironment`）へ差し替える。
- [ ] `docs/dev/developer_guide/design-implementation-overview.ja.md:117` の見出し `#### 5. 環境セキュリティ (`internal/runner/environment/`)` のパスを、実在する `internal/runner/base/environment/` へ修正する。あわせて同節の本文が Phase 1 適用後の責務（システム環境の列挙と denylist 判定）と矛盾しないことを確認し、矛盾があれば修正する。
- [ ] 上記 3 つの日本語版の修正を先にコミットしたうえで、`docs/dev/architecture_design/security-architecture.md`、`config-inheritance-behavior.md`、`docs/dev/developer_guide/design-implementation-overview.md` の同じ箇所へ `/mktrans` で反映する。
- [ ] `docs/dev/developer_guide/package_reference.md` の :29 と :87 の 2 箇所の `environment/` の説明を、`Environment variable processing and filtering` から実態（システム環境の列挙と denylist 判定）を表す英語表現へ書き換える。

**完了条件**

- [ ] `make fmt` → `make test` → `make lint` がすべて成功する
- [ ] `go vet -tags 'test integration' ./...` が成功する
- [ ] `make build` が成功する
- [ ] `cmd/runner` の統合テストと `internal/runner` の E2E テストが成功する（環境変数解決の回帰確認）
- [ ] `make verify-docs` が成功する（文書内リンクの整合）

### 2.4 Step 4 = Phase 2: `privilege` の到達不能な降格パスと本番未使用 API の削除

**設計参照**: [02_architecture.md](02_architecture.md) §2.2、§3.2、§4.3、§5.3、§5.5、§7.2

**変更対象ファイル**

- `internal/runner/base/privilege/unix.go`
- `internal/runner/base/privilege/unix_privilege_test.go`、`unix_test.go`、`identity_linux_test.go`、`identity_other_test.go`
- `internal/runner/base/privilege/identity_mutation_guard_test.go`（新規）
- `internal/runner/base/runnertypes/config.go`
- `internal/runner/base/privilege/testutil/mocks.go`
- `internal/runner/resource/normal_manager_test.go`

**作業内容: 降格パスの削除と改名**

- [ ] `changeUserGroupInternal` を `resolveUserGroupForDryRun(userName, groupName string) error` に改名する。引数 `dryRun` と `originalEGID` を削除する。
- [ ] 同関数から `m.syscallSetegid(targetGID)` の呼び出し、`m.syscallSeteuid(targetUID)` の呼び出し、`Seteuid` 失敗時の EGID ロールバック、`emergencyShutdown(restoreErr, "egid_rollback_failure_after_seteuid_failure")`、および成功ログ（`successLogAttrs` の組み立てと `m.logger.Info("User/group privileges changed successfully", ...)`）を削除する。
- [ ] `if dryRun { ... return nil }` の条件を外し、ログ出力と `return nil` を無条件の後処理にする。
- [ ] 同関数の doc コメントを、ユーザー名・グループ名を解決して dry-run のログを出力するだけであり、プロセスの識別情報（UID・GID・補助グループ）を一切変更しないことを述べる内容に書き換える。
- [ ] `performElevation` から、`needsUserGroupChange` による分岐と `isDryRun` の計算を除き、`execCtx.elevationCtx.Operation == runnertypes.OperationUserGroupDryRun` を直接判定して `resolveUserGroupForDryRun` を呼ぶ形に変更する。
- [ ] 同じブロック内の到達不能なロールバックブロック（`if execCtx.needsPrivilegeEscalation { if restoreErr := m.restorePrivileges(); ... m.emergencyShutdown(restoreErr, "user_group_change_failure") } }`）を削除する。
- [ ] `restorePrivilegesAndMetrics` の metrics 記録条件 `} else if panicValue == nil && (execCtx.needsPrivilegeEscalation || execCtx.needsUserGroupChange) {` を `} else if panicValue == nil && execCtx.elevationCtx.Operation == runnertypes.OperationUserGroupDryRun {` に変更する。dry-run で `RecordElevationSuccess` が記録される挙動は維持する。
- [ ] `executionContext` から `needsUserGroupChange` フィールドとその doc コメント（unix.go:118-123）を削除する。
- [ ] `executionContext` から `originalEUID` / `originalEGID` フィールドを削除し、`prepareExecution` の初期化（:147-148）も削除する。
- [ ] `prepareExecution` の `switch` から 3 箇所の `execCtx.needsUserGroupChange = ...` 代入を削除する。`needsPrivilegeEscalation` の代入は残す。
- [ ] `UnixPrivilegeManager` から `syscallSeteuid` / `syscallSetegid` フィールドとその説明コメントを削除する。
- [ ] `newPlatformManager` から `syscallSeteuid: syscall.Seteuid,` / `syscallSetegid: syscall.Setegid,` の初期化を削除する。

**作業内容: 本番未使用 API の削除**

- [ ] `internal/runner/base/runnertypes/config.go` の `PrivilegeManager` インターフェースから `WithUserGroup(user, group string, fn func() error) error` の宣言、`IsUserGroupSupported() bool` の宣言、および両者を囲む "// Enhanced privilege management for user/group specification" コメントを削除する。`IsPrivilegedExecutionSupported` と `WithPrivileges` は残す。
- [ ] `internal/runner/base/privilege/unix.go` から `WithUserGroup` メソッド（:591-599）を削除する。
- [ ] 同ファイルから `IsUserGroupSupported` メソッド（:601-604）を削除する。
- [ ] `internal/runner/base/privilege/testutil/mocks.go` から `WithUserGroup`（:60-71）を削除する。`WithPrivileges` が `OperationUserGroupExecution` に対して `ElevationCalls` へ記録する文字列は変更しない。
- [ ] 同ファイルから `IsUserGroupSupported`（:73-76）を削除する。
- [ ] `internal/runner/resource/normal_manager_test.go` の `MockPrivilegeManager` から `WithUserGroup`（:81-84）を削除する。
- [ ] 同ファイルから `IsUserGroupSupported`（:86-89）を削除する。

**作業内容: コメント・文字列リテラルの書き換え**

以下は完全な前後の文字列を示す。前の文字列を後の文字列に置き換える。

- [ ] `unix.go` の `executionContext.needsPrivilegeEscalation` の説明。前提が失われるため最終行を差し替える。

  前:
  ```go
  	// needsPrivilegeEscalation indicates whether system-level privilege escalation (setuid to root) is required.
  	// This is needed to gain administrative privileges for operations like file validation or user switching.
  	// When true, escalatePrivileges() will call syscall.Seteuid(0) to become root.
  ```
  後:
  ```go
  	// needsPrivilegeEscalation indicates whether system-level privilege escalation (setuid to root) is required.
  	// This is needed to gain administrative privileges for operations like file validation or user switching.
  	// When true, escalatePrivileges() will call syscall.Seteuid(0) to become root. The parent process never
  	// changes its identity to the target user; the executor applies that via syscall.Credential at execve time.
  ```

- [ ] `restorePrivilegesAndMetrics` 冒頭のコメント。

  前:
  ```go
  	// Note: no branch restores the effective group ID here. The only operation with
  	// needsUserGroupChange=true is OperationUserGroupDryRun (see prepareExecution),
  	// which never actually changes identity (changeUserGroupInternal returns early
  	// in dry-run mode), so there is nothing to restore.
  ```
  後:
  ```go
  	// Note: no branch restores the effective group ID here. OperationUserGroupDryRun only
  	// resolves user/group names (see resolveUserGroupForDryRun) and never changes identity,
  	// so there is nothing to restore.
  ```

- [ ] identity 検証をゲートするコメントの最終文。

  前:
  ```go
  	// Only privilege escalation changes identity in production; the sole needsUserGroupChange
  	// operation is dry-run, which never mutates identity, so escalation alone gates verification.
  ```
  後:
  ```go
  	// Only privilege escalation changes identity; OperationUserGroupDryRun never mutates
  	// identity, so escalation alone gates verification.
  ```

- [ ] `performElevation` のエラーラップ文言。`resolveUserGroupForDryRun` はもう識別情報を変更しないため、"change" を名乗らない文言に改める。

  前: `return fmt.Errorf("user/group change failed: %w", err)`
  後: `return fmt.Errorf("user/group resolution failed: %w", err)`

- [ ] `resolveUserGroupForDryRun` の開始ログ。`dryRun` 引数が消えるため属性も除く。

  前:
  ```go
  	logAttrs := []any{"dry_run", dryRun}
  	if userName != "" {
  ```
  後:
  ```go
  	logAttrs := []any{}
  	if userName != "" {
  ```
  および、前: `m.logger.Info("User/group change requested", logAttrs...)`
  後: `m.logger.Info("Dry-run user/group resolution requested", logAttrs...)`

- [ ] dry-run の結果ログ文言 `"Dry-run mode: would change user/group privileges"` は**変更しない**。この文言は「もし実行すれば何が起きるか」を述べたものであり、実装と矛盾しないためである。

- [ ] `WithPrivileges` の doc コメントを次に置き換える（AC-15 の 3 点を明記する）。

  前: `// WithPrivileges executes a function with elevated privileges using safe privilege escalation`
  後:
  ```go
  // WithPrivileges executes fn under the privilege state required by elevationCtx.Operation.
  //
  // For OperationUserGroupExecution this package only escalates to root; it does not read
  // elevationCtx.RunAsUser or elevationCtx.RunAsGroup. Switching to the target user, and
  // resolving the identity that switch needs, is done by the executor: it builds a
  // syscall.Credential that the kernel applies at execve time when the child process starts.
  // RunAsUser and RunAsGroup are resolved and logged inside this package only for
  // OperationUserGroupDryRun.
  //
  // For OperationFileValidation this package escalates to root and restores afterwards.
  ```

**作業内容: テストの更新**

`needsUserGroupChange` を設定している `executionContext` リテラルは、いずれも同じリテラル内で `elevationCtx.Operation` を設定済みであることを確認済みである。したがって設計書 §3.5 が言う「`elevationCtx.Operation` の設定へ置換」は、実際には行の削除だけで足りる。

- [ ] `unix_privilege_test.go` の `TestPrepareExecution_Success` から、表構造体の `expectedUserGroupChange` フィールド、3 ケースそれぞれの値、および `execCtx.needsUserGroupChange` に対するアサーションを削除する。
- [ ] `TestPerformElevation_Success` の 2 つのサブテストから `needsUserGroupChange: true,` の行を削除する（`Operation` は既に `OperationUserGroupDryRun`）。
- [ ] `TestChangeUserGroupInternal_NotCalledForUserGroupExecution` を `TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity` に改名し、注入フィールド `syscallSeteuid` / `syscallSetegid` への依存を除いて書き換える。`manager.WithUserGroup("testuser", "testgroup", fn)` の呼び出しを、`Operation: runnertypes.OperationUserGroupExecution` / `RunAsUser: "testuser"` / `RunAsGroup: "testgroup"` を設定した `ElevationContext` による `manager.WithPrivileges(...)` の直接呼び出しに置き換える。識別情報が変化しないことは、呼び出し前後の `syscall.Geteuid()` / `syscall.Getegid()` の一致で検証する。
- [ ] `TestChangeUserGroupInternal_NotCalledForUserGroupDryRun` を `TestWithPrivileges_UserGroupDryRunDoesNotChangeIdentity` に改名し、同様に注入フィールドへの依存を除く。呼び出し前後の `syscall.Geteuid()` / `syscall.Getegid()` / `syscall.Getgid()` の一致で検証する。テスト対象ホストに存在するユーザー名を使うため `user.Current()` を用いる現在の構成は維持する。
- [ ] `TestPerformElevation_Failure` の 2 つのサブテストから `needsUserGroupChange` の行を削除し、`invalid_user_in_dryrun` のアサーションを `assert.Contains(t, err.Error(), "user/group resolution failed")` に変更する。
- [ ] `TestHandleCleanupAndMetrics_Success` から `needsUserGroupChange: false,` の行を削除し、末尾のコメントを次のとおり書き換える。前: `// No metrics assertion needed since needsUserGroupChange is false` / `// (metrics are only recorded when user/group changes are needed)`、後: `// No metrics assertion here; this test only verifies that cleanup does not panic.`
- [ ] `TestHandleCleanupAndMetrics_WithError` から `needsUserGroupChange: false,` の行を削除する。
- [ ] `TestRestorePrivilegesAndMetrics_Success` から `needsUserGroupChange: true, // This will trigger success recording` の行を削除し、後続コメント `// When needsUserGroupChange is true, success should be recorded` を `// For OperationUserGroupDryRun, success should be recorded` に書き換える。`Operation` が `OperationUserGroupDryRun` であるため記録される挙動は変わらない。
- [ ] `TestRestorePrivilegesAndMetrics_Failure` から `needsUserGroupChange: false,` の行を削除する。
- [ ] `TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown` から `needsUserGroupChange: false,` / `originalEUID: syscall.Geteuid(),` / `originalEGID: syscall.Getegid(),` の 3 行を削除する（削除しないとコンパイルできない）。`syscall` import が他で使われていなければ削除する。
- [ ] `TestRestorePrivilegesAndMetrics_IdentityVerificationSkippedForDryRun` から `needsUserGroupChange: true,` の行を削除する。
- [ ] `TestRestorePrivilegesAndMetrics_SavedSetUnchanged_Passes` と同ファイル内の残る `executionContext` リテラルから、`needsUserGroupChange` の行があれば削除する（`rg -n 'needsUserGroupChange' internal/runner/base/privilege/` で残件が 0 になることを確認する）。
- [ ] `TestChangeUserGroupInternal_SeteuidFailure_EgidRollbackSuccess` を削除する（検証対象の降格 syscall とロールバックが消える）。
- [ ] `TestChangeUserGroupInternal_SeteuidFailure_EgidRollbackFailure` を削除する（検証対象の `emergencyShutdown` 経路が消える。`emergencyShutdown` 自体の挙動は `TestEmergencyShutdown` が引き続き検証する）。
- [ ] `TestWithUserGroup` を削除する（対象 API ごと削除）。
- [ ] `TestIsUserGroupSupported` を **`TestIsPrivilegedExecutionSupported` に転用する**。関数名を改め、2 つのアサーションの検証対象を `manager.IsUserGroupSupported()` / `managerNoPriv.IsUserGroupSupported()` から `manager.IsPrivilegedExecutionSupported()` / `managerNoPriv.IsPrivilegedExecutionSupported()` に差し替え、コメント（"On Unix systems, user/group should always be supported" / "User/group support depends on privilege support"）を `privilegeSupported` の値がそのまま返ることを述べる内容に書き換える。単純に削除しないのは、`privilegeSupported` が `true` / `false` の両方で正しく返ることを固定しているテストが他に存在しないためである（既存の `IsPrivilegedExecutionSupported` 参照はすべて skip ガードかモックの期待値設定であり、返り値を検証していない。§1.3 参照）。
- [ ] `identity_linux_test.go` の `executionContext` リテラルから `needsUserGroupChange: false,` の行を削除する。
- [ ] `identity_other_test.go` の `executionContext` リテラルから `needsUserGroupChange: false,` の行を削除する。
- [ ] `unix_test.go` の `TestUnixPrivilegeManager_WithUserGroupInternal` を `TestUnixPrivilegeManager_DryRunResolution` に改名する（実体は既に `WithPrivileges` を使っており内容の変更は不要。名前だけが削除済み API を指している）。

**作業内容: テストの追加**

- [ ] `unix_privilege_test.go` に `TestResolveUserGroupForDryRun` を追加し、次を検証する。
  - [ ] 存在しないユーザー名（`"nonexistent_user_xyz123"`）でエラーを返す（エラー経路）
  - [ ] 存在しないグループ名でエラーを返す（エラー経路）
  - [ ] グループ未指定時にユーザーのプライマリグループへフォールバックし、エラーを返さない
  - [ ] ユーザー名・グループ名がともに空のときエラーを返さない（境界値）
  - [ ] いずれのケースでも呼び出し前後で `syscall.Geteuid()` / `syscall.Getegid()` が変化しない
- [ ] `internal/runner/base/privilege/identity_mutation_guard_test.go` を新規作成し、`TestNoUnexpectedIdentityMutationSyscalls` を実装する。ビルドタグは `unix.go` と揃えて `//go:build !windows` とする。設計書 §7.2 が求める静的検査を、`make test` で継続的に実行される形にするためである（非特権 CI では降格の有無を動的に判別できないため、AC-16(c) の担保はこのテストが担う）。
  - [ ] `os.ReadDir(".")` でパッケージディレクトリのファイルを列挙し、`.go` で終わり `_test.go` で終わらないものを `go/parser` の `ParseFile` で解析する。**`parser.ParseDir` は使わない**。`ParseDir` は Go 1.22 で deprecated になっている。本リポジトリの `.golangci.yml` は `staticcheck` を有効にしており、`_test.go` に対する除外に SA1019 を含まないため、使用すると `make lint` が失敗する
  - [ ] 依存は標準ライブラリのみとする。`golang.org/x/tools/go/packages` は使えない（`.golangci.yml` の depguard が `**/internal/**` に対して `$gostd` と明示された少数のモジュールしか許可しておらず、`golang.org/x/tools` は含まれない）
  - [ ] ビルドタグを解釈せずディレクトリ内の全ファイルを走査することを doc コメントに明記する。`identity_linux.go` と `identity_other.go` が同時に解析されるが、いずれも識別情報変更関数を呼ばないため問題にならず、むしろプラットフォームを問わず検査できる点で望ましい
  - [ ] `syscall` または `unix`（`golang.org/x/sys/unix`）パッケージの識別情報変更関数（`Seteuid` / `Setegid` / `Setuid` / `Setgid` / `Setreuid` / `Setregid` / `Setresuid` / `Setresgid` / `Setgroups` / `Setfsuid` / `Setfsgid`）の呼び出しを列挙する
  - [ ] 許可される組み合わせは `escalatePrivileges` 内の `syscall.Seteuid(0)` と `restorePrivileges` 内の `syscall.Seteuid(m.originalUID)` の 2 つのみとし、それ以外を検出したらテストを失敗させる。引数は `go/types` の `ExprString` で文字列化して比較する（この API は deprecated ではない）
  - [ ] 許可された 2 つの呼び出しが実際に存在することも検証する（検査自体が空振りしていないことの確認）

**完了条件**

- [ ] `make fmt` → `make test` → `make lint` がすべて成功する
- [ ] `go vet -tags 'test integration' ./...` が成功する（`internal/runner/base/executor/executor_usergroup_integration_test.go` は `runnertypes.PrivilegeManager` に依存するが、これをコンパイルする Makefile ターゲットが存在しないため。§1.3 参照）
- [ ] `make build` が成功する
- [ ] `rg -n --glob '*.go' -e 'needsUserGroupChange' -e 'syscallSeteuid' -e 'syscallSetegid' -e 'changeUserGroupInternal' -e 'originalEUID' -e 'originalEGID'` の一致件数が 0 である
- [ ] 下の「特権環境での確認」を完了する

**特権環境での確認（設計書 §7.4）**

`make integration-test` はランナーを `--dry-run` なしで 1 回だけ実行するため、これ単体では下の 5 項目のうち 2 項目しか確認できない。また特権経路は setuid バイナリを必要とし、それが無いと出力が `[WARNING: User/Group privilege management not supported]` になって確認が空振りする。したがって次の手順を明示的に実施する。すべての実行に共通する前提として、root へ `sudo` できる環境で行う。なお `GSCR_SLACK_WEBHOOK_URL_SUCCESS` / `GSCR_SLACK_WEBHOOK_URL_ERROR` が未設定の場合、`make integration-test` は警告を出すが停止はしない。

- [ ] 前提: `make build && make setuid` を実行し、`build/runner` が root 所有かつ setuid ビット付きであることを `ls -l build/runner` で確認する（これが無いと以降の確認は空振りする）
- [ ] 前提: 検証用に、root 以外の実在するユーザーを `run_as_user` に指定した確認用 TOML を 1 つ用意する。`sample/comprehensive.toml` の `run_as_user` はすべて `"root"` であり、補助グループの検証が退化するためそのままでは使えない。コマンドは `id -G` のように識別情報を標準出力へ出すものにする
- [ ] 前提: 同じ確認用 TOML の複製を 1 つ作り、`run_as_user` を実在しないユーザー名（例: `nonexistent_user_xyz123`）に書き換える
- [ ] 確認 1: 確認用 TOML を通常実行し、子プロセスの出力が対象ユーザーの UID・GID・補助グループと一致する
- [ ] 確認 2: 同じ実行のログに `Privileges fully restored to original state`（`unix.go` の `restorePrivileges`）が現れ、実行後に EUID と UID が一致する
- [ ] 確認 3: 確認用 TOML を `--dry-run -log-level info` で実行し、出力に `[INFO: User/Group configuration validated]`（`internal/runner/resource/dryrun_manager.go:292`）が現れ、親プロセスの識別情報が変化しない
- [ ] 確認 4: 実在しないユーザーの TOML を `--dry-run -log-level info` で実行し、`[ERROR: User/Group validation failed:` が出力され `SecurityRisk` が `high` に引き上げられる（`dryrun_manager.go:288-290`）
- [ ] 確認 5: `make integration-test` が成功する（既存の統合シナリオ全体の回帰確認）

---

## 3. 実装順序とマイルストーン

Phase 間に実装上の依存はない（設計書 §8.1）。以下の順序は依存ではなく、レビュー負荷とリスクに基づく推奨（設計書 §8.2）である。1 Phase = 1 PR とする（設計書 §8.3）。

| マイルストーン | 内容 | 成果物 | リスク |
|---|---|---|---|
| M1 | Step 1（Phase 4: `fileanalysis`） | `syscall_store.go` の削除、`common.GroupAndSortSyscalls` のテスト新設、`ArgEvalResults` 往復テストの移設 | 最小。`make deadcode` が到達不能であることを機械的に裏づけている |
| M2 | Step 2（Phase 3: `groupmembership`） | `getProcessRealUID`、`resolvePermissionCheckUID`、AC-20 の 3 ケーステスト、`CHANGELOG.md` の `[Unreleased]` 節 | 中。fail-open 化の合意が前提（設計書 §5.4） |
| M3 | Step 3（Phase 1: `environment` / `runner` / `executor`） | `system_env.go`、`Runner` のデッドコード削除、セキュリティ設計文書と `package_reference.md` の追随 | 中。変更範囲が広くテストの更新点が多い |
| M4 | Step 4（Phase 2: `privilege`） | 降格パスの削除、`WithUserGroup` / `IsUserGroupSupported` の削除、識別情報変更 syscall のガードテスト | 最大。特権管理コードに触れるため、§7.4 の特権環境確認をマージ条件とする |

各マイルストーンは独立してレビュー・マージできる。いずれかを見送っても他のマイルストーンの成果は成立する。

---

## 4. テスト戦略

設計書 §7 の方針に従う。本節は「どのテストをどう書くか」に絞る。

### 4.1 新規テスト

| テスト | 場所 | 目的 |
|---|---|---|
| `TestGroupAndSortSyscalls` | `internal/common/syscall_grouping_test.go` | Phase 4 で削除するテストが検証していた統合・並び替えの不変条件を、関数の直下で保持する |
| `TestStore_ArgEvalResultsRoundtrip` | `internal/fileanalysis/file_analysis_store_test.go` | `ArgEvalResults` の JSON 往復（nil が nil のまま戻ること）を現役 API で保持する |
| `TestResolvePermissionCheckUID` | `internal/groupmembership/manager_test.go` | AC-20 の 3 ケースを root 権限なしで決定的に検証する。不正な `SUDO_UID` のエラー経路も含む |
| `TestCanCurrentUserSafelyWriteFile_UsesRealUID` | `internal/groupmembership/manager_test.go` | 既知の所有者・権限を持つファイルに対する判定結果そのものを検証し、UID の出所が `os.Getuid()` のみであることを固定する（AC-25） |
| `TestExpandGlobal_SystemEnvIncludesAllParsableEntries` | `internal/runner/config/expansion_test.go` | `runtime.SystemEnv` に allowlist 外の変数も含まれることを固定する（AC-03） |
| `TestResolveUserGroupForDryRun` | `internal/runner/base/privilege/unix_privilege_test.go` | 解決の成功・失敗と、識別情報を変更しないことを検証する |
| `TestNoUnexpectedIdentityMutationSyscalls` | `internal/runner/base/privilege/identity_mutation_guard_test.go` | 識別情報を変更する syscall が許可された 2 箇所以外に現れないことを AST 解析で検証する（AC-16(c)） |

### 4.2 既存テストの拡張

新規テスト関数を作らず既存テストへ追記するのは次の 1 件である。`internal/groupmembership/manager_test.go::TestGetPermissionCheckUID`（:599）に、具体値で判定するサブテストを追加する（§2.2）。同関数の委譲を検証する新規テストは作らない。既存テストと重複するうえ、実装本体を再計算するだけのアサーションになり、誤った実装でも通ってしまうためである。

### 4.3 テストヘルパーの追加

新規のテストヘルパーファイルは追加しない。既存の `internal/testutil`（`tu.SafeTempDir`）と `internal/runner/base/privilege/testutil`（`MockPrivilegeManager`、`//go:build test`）をそのまま使う。`internal/runner/base/privilege/testutil/mocks.go` は既存ファイルからのメソッド削除のみであり、[test_organization.md](../../dev/developer_guide/test_organization.md) の分類 A（`testutil/mocks.go`）と命名規則（`package privilegetestutil`）に変更はない。

### 4.4 回帰確認

削除対象に直接依存していたテスト（§2 の各 Step で列挙）を除き、既存テストが無修正で通ることが、回帰が起きていないことの最も強い裏づけになる。とくに次を無修正での成功として確認する。

- `internal/runner/base/environment/denylist_test.go` の 4 テスト（AC-10）
- `internal/runner/base/executor/environment_test.go` の 16 テスト（`getSystemEnvironment` 置換の回帰、設計書 §2.1.3）
- `internal/runner/base/executor/executor_usergroup_test.go` の 4 テスト（AC-28）
- `internal/runner/base/privilege/manager_test.go::TestManager_WithPrivileges_UserGroup_ValidUser`、`::TestManager_WithPrivileges_UserGroup_InvalidUser`、`::TestManager_WithPrivileges_UserGroup_EmptyUserGroup`、`::TestManager_WithPrivileges_UserGroup_FunctionError`（実マネージャに対する dry-run 経路の回帰、AC-16(b)）
- `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_UserGroupValidation`（`[INFO: User/Group configuration validated]` の出力経路を非特権環境で固定する。AC-16(b) と設計書 §2.2.5 のモックへの影響の双方を確認できる）

### 4.5 ビルド構成をまたぐ確認

- Phase 3 の cgo 有効・無効の両構成での確認は `make test` で足りる。`unit-test`（Makefile:454-461）が非 Darwin では `CGO_ENABLED=1 -race` と `CGO_ENABLED=0` の 2 回テストを実行するためである。マージ前に CI の 2 レグ（`make test-ci-cgo1` / `make test-ci-cgo0`）が成功していることも確認する。
- Phase 4 が触る `syscall_analyzer_integration_test.go` は `//go:build integration` であり `make test` ではコンパイルされない。`make elfanalyzer-integration-test`（`-tags integration`）を Phase 4 の完了ゲートに含める。
- ビルドタグ付きのテストファイルには、どの Makefile ターゲットからもコンパイルされないものがある（`internal/runner/base/executor/executor_usergroup_integration_test.go` と `privileged_test_condition_test.go` は `-tags 'test integration'` の両方を要する）。全 Step の完了条件に `go vet -tags 'test integration' ./...` を含め、型・シグネチャの不整合をその PR 内で検出する。
- Phase 2 の `identity_mutation_guard_test.go` は `//go:build !windows` とし、`make test`（`-tags test`）で実行される。

### 4.6 テストで担保できない範囲

- **AC-16(c)（降格しないこと）を動的テストで担保できない。** 非特権 CI では降格 syscall が呼ばれても `EPERM` で拒否されて識別情報が変わらないため、テスト側では異常を検出できない（設計書 §7.1）。`TestNoUnexpectedIdentityMutationSyscalls` による AST 解析と、§2.4 の特権環境確認で補う。
- **AC-25 の「passwd エントリを引けない状況」を単体テストで再現できない。** Go の `os/user` は passwd の探索先を差し替える手段を提供していない。UID の取得経路から passwd 参照を無くしたことを、実行可能テスト（`TestCanCurrentUserSafelyWriteFile_UsesRealUID`）と静的検査（`manager.go` に `user.Current()` が現れないこと）で確認する。
- **グループ書き込み可能なファイルの判定に残る passwd 依存は AC-25 の対象外である。** ファイルがグループ書き込み可能な場合、判定は `IsUserInGroup` / `isUserOnlyGroupMember` を経由して `user.LookupId` を呼ぶため、passwd エントリを引けなければ従来どおり fail-closed で拒否する（§1.3）。Phase 3 が取り除くのは UID の取得に伴う passwd 依存だけであり、要件定義書の改訂により AC-25 の範囲もそこに限定された。この限界は `CHANGELOG.md` にも明記する。

---

## 5. リスク管理

| リスク | 影響 | 緩和 |
|---|---|---|
| Phase 2 で特権復元の経路を壊す | 特権が残留したままコールバックが実行される | `escalatePrivileges` / `restorePrivileges` / `handleCleanupAndMetrics` に手を入れない。identity 検証と saved-set 検査の分岐は `needsPrivilegeEscalation` でゲートされたまま変えない（設計書 §5.3）。§2.4 の特権環境確認 5 項目をマージ条件とする |
| 特権環境確認が setuid バイナリ不在で空振りする | 降格・復元の確認をしたつもりで、実際には `[WARNING: User/Group privilege management not supported]` しか見ていない | §2.4 の前提として `make build && make setuid` と `ls -l build/runner` による確認を明示する |
| Phase 2 のインターフェース変更がビルドタグ付きファイルを壊す | `-tags 'test integration'` でしかコンパイルされないファイルの破損が、後続 PR や CI まで発覚しない | 全 Step の完了条件に `go vet -tags 'test integration' ./...` を含める |
| 真偽値フィールド `needsUserGroupChange` の廃止で dry-run 判定を取りこぼす | dry-run でユーザー・グループ検証が行われなくなり、設定不備が検出されない | `performElevation` と `restorePrivilegesAndMetrics` の両方で同じ `Operation` 判定に置き換える。片方だけの置換を防ぐため、完了条件に `needsUserGroupChange` の一致件数 0 を含める |
| Phase 4 のテスト削除でカバレッジが失われる | `GroupAndSortSyscalls` と `ArgEvalResults` 往復の回帰が検出できなくなる | §1.3 で調査したとおり、代替テストを新設・移設する。削除と追加を同一 PR で行う |
| Phase 3 の fail-open 化が運用に影響する | passwd エントリ欠如環境で、これまで停止していた実行が継続する | `CHANGELOG.md` の `[Unreleased]` に記載する。判定に使う UID・規則は変わらないことを併記する（設計書 §5.4） |
| AC-25 を「passwd 依存が完全に消えた」と読み違える | 実際にはグループ書き込み可能なファイルの判定で `user.LookupId` が残り、記載が事実と食い違う | 要件定義書の AC-25 を「UID の取得」に限定する改訂を行った。§1.3 と §4.6 に残る依存を明記し、`CHANGELOG.md` の文言も同じ範囲に揃える。静的検査は `user.Current(` だけでなく `os/user` の呼び出し全体を列挙する形にする（§7） |
| `performElevation` のエラー文言変更が dry-run 出力に現れる | dry-run の `Impact.Description` の文字列が変わる | 対象は dry-run のユーザー・グループ解決失敗時のみ。既存アサーション（`unix_privilege_test.go`）を同一 PR で更新する。運用者が読む出力の意味は変わらない（「変更に失敗」→「解決に失敗」で、実装に即した表現になる） |
| ドキュメント修正の英語版取りこぼし | セキュリティ設計文書の日英で記述が食い違う | 日本語版を先にコミットし、英語版は `/mktrans` で反映する。Phase 1 の完了条件に `make verify-docs` を含める |

---

## 6. 実装チェックリスト

### Step 1 = Phase 4: `fileanalysis`

- [ ] `internal/common/syscall_grouping_test.go` の新設（5 ケース）
- [ ] `TestStore_ArgEvalResultsRoundtrip` の追加（2 サブテスト）
- [ ] `internal/fileanalysis/syscall_store.go` の削除
- [ ] `internal/fileanalysis/syscall_store_test.go` の削除
- [ ] `TestE2E_RecordToRunnerFallbackChain` と 3 import の削除
- [ ] `make fmt` / `make test` / `make lint` / `make build` / `make elfanalyzer-integration-test` / `go vet -tags 'test integration' ./...` の成功
- [ ] `make deadcode` から該当 3 行が消えたことの確認

### Step 2 = Phase 3: `groupmembership`

- [ ] `getProcessRealUID` への改名と `os.Getuid()` への変更（範囲検査を残す理由を doc に明記）
- [ ] `resolvePermissionCheckUID` の切り出しと `getPermissionCheckUID` の委譲
- [ ] コメントの書き換え 6 項目（§2.2「作業内容: コメントの書き換え」）
- [ ] `effectiveUID` → `permissionCheckUID` の改名（4 箇所）
- [ ] `TestGetProcessRealUID` への改名と書き換え
- [ ] `TestResolvePermissionCheckUID` の追加（3 ケース + エラー経路）
- [ ] 既存 `TestGetPermissionCheckUID` への具体値サブテストの追加
- [ ] `TestCanCurrentUserSafelyWriteFile_UsesRealUID` の追加（3 つの権限パターン）
- [ ] `CHANGELOG.md` の `[Unreleased]` 節の新設（残る passwd 依存の明記を含む）
- [ ] `make fmt` / `make test` / `make lint` / `go vet -tags 'test integration' ./...` の成功、および CI 2 レグの成功

### Step 3 = Phase 1: `environment` / `runner` / `executor`

- [ ] `filter.go` → `system_env.go` の改名と縮退
- [ ] `filter_test.go` → `system_env_test.go` の改名と書き換え（削除 9 件・書き換え 3 件）
- [ ] `filter_benchmark_test.go` の削除
- [ ] `executor.getSystemEnvironment` の削除と置換、import 2 件の削除
- [ ] `expansion.go:866` の置換と `TestExpandGlobal_SystemEnvIncludesAllParsableEntries` の追加
- [ ] `Runner` の `envVars` / `envFilter` / `LoadSystemEnvironment` の削除と import の削除
- [ ] `cmd/runner/main.go` の呼び出し削除
- [ ] 呼び出し元テスト 12 箇所の更新（`err :=` への変更 2 箇所を含む）
- [ ] `security-architecture.ja.md`（コードブロック :166-172 全体）・`config-inheritance-behavior.ja.md`（:41, :58）・`design-implementation-overview.ja.md`（:117）の修正 → コミット → `/mktrans` で英語版 3 ファイルへ反映
- [ ] `package_reference.md` の 2 箇所の説明修正
- [ ] `make fmt` / `make test` / `make lint` / `make build` / `make verify-docs` / `go vet -tags 'test integration' ./...` の成功

### Step 4 = Phase 2: `privilege`

- [ ] `resolveUserGroupForDryRun` への改名と降格処理の削除
- [ ] `performElevation` のロールバックブロック削除と `Operation` 直接判定への置換
- [ ] `restorePrivilegesAndMetrics` の metrics 条件の置換
- [ ] `executionContext` からの 3 フィールド削除と注入フィールド 2 個の削除
- [ ] `WithUserGroup` / `IsUserGroupSupported` の削除（インターフェース・実装・モック 2 つ）
- [ ] コメント・文字列リテラル 7 箇所の書き換え（§2.4）
- [ ] `WithPrivileges` の doc コメント整備（AC-15 の 3 点）
- [ ] テスト更新 15 件・削除 3 件・改名 4 件（`TestIsUserGroupSupported` は削除ではなく `TestIsPrivilegedExecutionSupported` へ転用）
- [ ] `TestResolveUserGroupForDryRun` の追加
- [ ] `identity_mutation_guard_test.go` の新設（`parser.ParseFile` を使い `ParseDir` は使わない）
- [ ] `make fmt` / `make test` / `make lint` / `make build` / `go vet -tags 'test integration' ./...` の成功
- [ ] 特権環境での確認 5 項目（§2.4、前提の `make setuid` と確認用 TOML 2 本の用意を含む）

---

## 7. 受入基準の検証

各 AC を、それを検証する手段に対応づける。種別は `test`（実行可能・誤った挙動で失敗する）、`static`（`rg` / コンパイル / `make deadcode`）、`manual`（特権環境やレビューでの観察）である。`manual` は補助であり、`test` または `static` を代替しない。

以降の `rg` は、明示がない限りリポジトリルートで実行する。過去タスクの記録は `docs/tasks/` に残るため、Go の識別子に関する検索は `--glob '*.go'` で Go ソースに限定する。

### Phase 1

| AC | 種別 | 検証手段 | 期待結果 |
|---|---|---|---|
| AC-01 | static | `rg -n --glob '*.go' -e '\bglobalAllowlist\b' -e '\bNewFilter\b'` | 一致 0 件 |
| AC-02 | static | `rg -n --glob '*.go' -e '\bFilterSystemEnvironment\b' -e '\bFilterGlobalVariables\b' -e 'environment\.Filter\b' -e '\bSourceEnvFile\b'` および `rg -n '\bFilter\b' internal/runner/base/environment/` | いずれも一致 0 件 |
| AC-02 | test | `internal/runner/base/environment/system_env_test.go::TestParseSystemEnvironment` | 成功（列挙機能が実態を表す名前のパッケージ関数として提供されている） |
| AC-03 | test | `internal/runner/config/expansion_test.go::TestExpandGlobal_SystemEnvIncludesAllParsableEntries` | 成功（`runtime.SystemEnv` が allowlist 外の変数もキー・値ともに含み、`environment.ParseSystemEnvironment()` と一致する） |
| AC-04 | static | `rg -n --glob '*.go' 'IsVariableAccessAllowed'` | 一致 0 件 |
| AC-05 | static | `rg -n --glob '*.go' -e 'ErrVariableNameEmpty' -e 'ErrInvalidVariableName' -e 'ErrDangerousVariableValue' -e 'ErrVariableNotFound' -e 'ErrVariableNotAllowed'` | 一致 0 件 |
| AC-05 | static | `rg -n --glob '*.go' 'ErrGroupNotFound'` | 一致が `internal/runner/runner.go:36` の定義、`internal/runner/cli/filter.go` の定義と参照、`internal/runner/cli/filter_test.go` の参照だけになり、`internal/runner/base/environment/` の一致が 0 件 |
| AC-06 | static | `rg -n 'map\[string\]string, error\)' internal/runner/base/environment/` | 一致 0 件（変更前は `filter.go:72` と `:81` の 2 件が一致する） |
| AC-06 | static | `rg -n 'func .*error\)' internal/runner/base/environment/system_env.go` | 一致 0 件（`error` を返す関数が存在しない） |
| AC-06 | static | `rg -n '^func ParseSystemEnvironment' internal/runner/base/environment/system_env.go` | `func ParseSystemEnvironment() map[string]string {` の 1 件のみ |
| AC-07 | static | `test ! -e internal/runner/base/environment/filter_benchmark_test.go && echo absent` | `absent` が出力される |
| AC-08 | static | `rg -n -e '\benvVars\b' -e '\benvFilter\b' internal/runner/runner.go` | 一致 0 件 |
| AC-09 | static | `rg -n --glob '*.go' 'LoadSystemEnvironment'` | 一致 0 件 |
| AC-09 | test | `cmd/runner/integration_workdir_test.go::TestIntegration_TempDirHandling`、`::TestIntegration_ErrorCleanup`、`::TestIntegration_MultipleGroups`、`cmd/runner/integration_auto_vars_test.go::TestIntegration_AutoVariables`、`internal/runner/e2e_shebang_test.go::TestIntegration_ShebangChainRunnerExecution`、`internal/runner/e2e_dynlib_verification_test.go::TestGroupExecutor_F001_HashMismatchBlocksExecution`、`::TestGroupExecutor_F004_LibraryShadowingBlocksExecution` | すべて成功（呼び出し削除後もコマンド実行時の環境変数解決が従来と同じ結果になる） |
| AC-10 | test | `internal/runner/base/environment/denylist_test.go::TestIsForbiddenEnvVar_Prefix`、`::TestIsForbiddenEnvVar_Exact`、`::TestIsForbiddenEnvVar_NonMatch`、`::TestIsForbiddenEnvVar_CaseSensitive` | 4 件すべて**無修正**で成功 |
| AC-10 | static | `git diff --stat internal/runner/base/environment/denylist.go internal/runner/base/environment/denylist_test.go` | 差分 0 行 |
| AC-26 | static | `rg -n -e 'globalAllowlist' -e 'type Filter struct' -e 'internal/runner/environment/filter\.go' docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md` | 一致 0 件 |
| AC-26 | static | `rg -c -F -e 'ProcessEnvImport' -e 'BuildProcessEnvironment' -e 'denylist.go' docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md` | 両ファイルとも 3 語すべてで 1 件以上（allowlist の実際の適用箇所 2 つと denylist 判定に言及している） |
| AC-26 | static | `rg -n 'internal/runner/environment/' docs/dev/` | 一致 0 件（存在しないパッケージパスを指す記述が残っていない。変更前は 8 件が一致する: `security-architecture.{ja.,}md` 各 1、`config-inheritance-behavior.{ja.,}md` 各 2、`design-implementation-overview.{ja.,}md` 各 1） |
| AC-26 | manual | 差し替えた §3 の記述を、設計書 §1.5 が示す 2 箇所の実装（`internal/runner/config/expansion.go` の `ProcessEnvImport`、`internal/runner/base/executor/environment.go` の `BuildProcessEnvironment`）と読み合わせる | 記述された挙動が実装と一致する |

### Phase 2

| AC | 種別 | 検証手段 | 期待結果 |
|---|---|---|---|
| AC-11 | test | `internal/runner/base/privilege/identity_mutation_guard_test.go::TestNoUnexpectedIdentityMutationSyscalls` | 成功（識別情報を変更する syscall の呼び出しが `escalatePrivileges` 内の `syscall.Seteuid(0)` と `restorePrivileges` 内の `syscall.Seteuid(m.originalUID)` の 2 つに限られる） |
| AC-11 | static | `rg -n --glob '!*_test.go' -e 'syscall\.Setegid' -e 'Setresuid' -e 'Setresgid' -e 'egid_rollback_failure_after_seteuid_failure' internal/runner/base/privilege/` | 一致 0 件 |
| AC-12 | static | `rg -n --glob '*.go' 'user_group_change_failure'` | 一致 0 件 |
| AC-12 | static | `rg -n --glob '*.go' 'needsUserGroupChange'` | 一致 0 件（真偽値フィールドごと廃止されたため、それを前提とする分岐も存在しない） |
| AC-13 | static | `rg -n --glob '*.go' -e 'syscallSeteuid' -e 'syscallSetegid'` | 一致 0 件 |
| AC-14 | static | `rg -n --glob '*.go' 'changeUserGroupInternal'` | 一致 0 件 |
| AC-14 | static | `rg -n 'func \(m \*UnixPrivilegeManager\) resolveUserGroupForDryRun' internal/runner/base/privilege/unix.go` | 1 件一致（実態を表す名前で提供されている） |
| AC-14 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestResolveUserGroupForDryRun` | 成功（解決とログ出力のみを行い、識別情報を変更しない） |
| AC-15 | static | `sed -n '/^\/\/ WithPrivileges executes fn/,/^func (m \*UnixPrivilegeManager) WithPrivileges/p' internal/runner/base/privilege/unix.go \| rg -c -F -e 'only escalates to root' -e 'does not read' -e 'syscall.Credential' -e 'execve' -e 'OperationUserGroupDryRun'` | 5 語すべてで 1 件以上。すなわち doc コメントに (a) root への昇格のみで `RunAsUser` / `RunAsGroup` を読まないこと、(b) 対象ユーザーへの切り替えと識別情報の解決は executor が行い `syscall.Credential` として execve 時に適用されること、(c) privilege パッケージ内での解決・ログ出力は `OperationUserGroupDryRun` に限られること、の 3 点が現れる |
| AC-15 | manual | 上記 doc コメント全文を設計書 §5.5 の 3 項目と読み合わせる | 3 項目が過不足なく、かつ実装と矛盾なく書かれている |
| AC-16 (a) | test | `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_UserGroupExecutionDoesNotChangeIdentity`、`::TestEscalatePrivileges`、`::TestRestorePrivilegesAndMetrics_SavedSetUnchanged_Passes` | 成功（root 昇格と復元が従来どおり行われる） |
| AC-16 (b) | test | `internal/runner/base/privilege/unix_privilege_test.go::TestWithPrivileges_UserGroupDryRunDoesNotChangeIdentity`、`internal/runner/base/privilege/unix_test.go::TestUnixPrivilegeManager_DryRunResolution`、`internal/runner/base/privilege/manager_test.go::TestManager_WithPrivileges_UserGroup_ValidUser`（無修正）、`internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_UserGroupValidation`（無修正） | すべて成功（解決とログ出力のみが行われ、識別情報が変化せず、`[INFO: User/Group configuration validated]` の出力経路も変わらない） |
| AC-16 (c) | test | `internal/runner/base/privilege/identity_mutation_guard_test.go::TestNoUnexpectedIdentityMutationSyscalls` | 成功（対象ユーザーへ EUID/EGID を変更する呼び出しが存在しない） |
| AC-16 (c) | manual | §2.4「特権環境での確認」の 5 項目 | 5 項目すべてを確認 |
| AC-16 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown`、`::TestDefaultIdentityVerifier`、`internal/runner/base/privilege/identity_linux_test.go` / `identity_other_test.go` の saved-set 検査テスト | すべて成功（`ErrIdentityLeak` と saved-set 検査の挙動が変わらない） |
| AC-27 | static | `rg -n --glob '*.go' -e '\bWithUserGroup\b' -e '\bIsUserGroupSupported\b'` | 一致 0 件（`executeWithUserGroup` は `\b` 境界により一致しない） |
| AC-27 | static | `rg -n 'executeWithUserGroup' internal/runner/base/executor/` | 一致あり（executor の非公開メソッドは残っている） |
| AC-28 | static | `make build` | 成功（`cmd/runner` / `cmd/record` / `cmd/verify` がビルドできる） |
| AC-28 | test | `internal/runner/base/executor/executor_usergroup_test.go::TestExecuteWithUserGroup_ResolverArgs_ThreeForms`、`::TestExecuteWithUserGroup_ResolverError_FailsClosed`、`::TestExecuteWithUserGroup_ResolverNilGroups_FailsClosed`、`::TestExecuteWithUserGroup_NoRunAs_ResolverNotInvoked` | 4 件すべて**無修正**で成功（executor → `WithPrivileges` の実行経路が変わらない） |
| AC-29 | static | `rg -n 'IsPrivilegedExecutionSupported' internal/runner/base/executor/executor.go internal/runner/resource/dryrun_manager.go internal/runner/base/runnertypes/config.go` | 3 ファイルすべてで一致あり（宣言と 2 つの呼び出しが残っている） |
| AC-29 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestIsPrivilegedExecutionSupported`（旧 `TestIsUserGroupSupported` の転用） | 成功（`privilegeSupported` が `true` / `false` の両方で正しく返る） |
| AC-29 | test | `internal/runner/base/privilege/manager_test.go::TestManager_WithPrivileges_UnsupportedPlatform`、`internal/runner/resource/dryrun_manager_test.go`（`IsPrivilegedExecutionSupported` のモック期待値を設定している 2 箇所） | 無修正で成功（executor / dry-run manager の分岐が従来どおり機能する） |

### Phase 3

| AC | 種別 | 検証手段 | 期待結果 |
|---|---|---|---|
| AC-17 | static | `rg -n --glob '*.go' 'getProcessEUID'` | 一致 0 件 |
| AC-17 | static | `rg -n -A12 '^// getProcessRealUID returns' internal/groupmembership/manager.go` | doc コメントに `EUID` という語が現れず、実 UID を返すことが記されている |
| AC-17 | test | `internal/groupmembership/manager_test.go::TestGetProcessRealUID` | 成功（`os.Getuid()` と同じ値を返し、`SUDO_UID` の設定で変わらない） |
| AC-18 | static | `rg -n 'user\.Current\(' internal/groupmembership/manager.go` | 一致 0 件 |
| AC-18 | static | `rg -n 'os\.Getuid\(\)' internal/groupmembership/manager.go` | `getProcessRealUID` 内の 1 件一致 |
| AC-18 | test | `internal/groupmembership/manager_test.go::TestGetProcessRealUID` | 成功（キャッシュに起因する固定が起こらないこと。値はカーネルから毎回取得される） |
| AC-19 | static | `rg -n -e 'EUID' -e 'effective user' -e 'effective UID' -e 'effectiveUID' internal/groupmembership/manager.go` | 一致 0 件（変更前は 17 行が一致する） |
| AC-19 | manual | `getPermissionCheckUID` の doc・インラインコメントと `CanCurrentUserSafelyWriteFile` のコメントを実装と読み合わせる | 実 UID による判定であるという記述が実装と一致する |
| AC-20 | test | `internal/groupmembership/manager_test.go::TestResolvePermissionCheckUID` | 3 ケース (a)(b)(c) すべてで期待どおりの UID が返り、不正な `SUDO_UID` でエラーが返る |
| AC-20 | test | `internal/groupmembership/manager_test.go::TestGetPermissionCheckUID`（既存テストへサブテストを追加） | `SUDO_UID` が空なら `os.Getuid()`、`SUDO_UID` が `"9999"` なら非 root で `os.Getuid()`・root で `9999` という具体値が返る |
| AC-25 | test | `internal/groupmembership/manager_test.go::TestCanCurrentUserSafelyWriteFile_UsesRealUID` | `0o600` → `true`、`0o666` → `ErrFileWorldWritable`、`0o400` → `ErrFileNotWritable`。UID の取得が失敗し得ないため判定が中断しない |
| AC-25 | static | `rg -n -e 'user\.Current\(' -e 'user\.Lookup\(' -e 'user\.LookupId\(' internal/groupmembership/manager.go` | 一致が `IsUserInGroup`（:138）と `isUserOnlyGroupMember`（:177）の `user.LookupId` の 2 件のみで、`user.Current(` の一致が 0 件（UID の取得経路から passwd 依存が消えたこと）。残る 2 件はグループ書き込み可能なファイルの判定でのみ通る経路であり、要件定義書の対象外節により AC-25 の範囲外である（§1.3、§4.6） |
| AC-25 | static | `rg -n -A10 '^## \[Unreleased\]' CHANGELOG.md` | fail-closed から fail-open への変化、判定に使う UID・規則が変わらないこと、およびグループ書き込み可能なファイルの判定では passwd エントリが引き続き必要であることの 3 点が記載されている |
| AC-25 | manual | `make test-ci-cgo1` と `make test-ci-cgo0` の両方を実行する | 両構成で成功する |

### Phase 4

| AC | 種別 | 検証手段 | 期待結果 |
|---|---|---|---|
| AC-21 | static | `test ! -e internal/fileanalysis/syscall_store.go && echo absent` | `absent` が出力される |
| AC-21 | static | `rg -n --glob '*.go' -e 'fileanalysis\.SyscallAnalysisStore' -e 'fileanalysis\.SyscallAnalysisResult' -e 'NewSyscallAnalysisStore' -e 'syscallAnalysisStore' -e 'SaveSyscallAnalysis'` | 一致 0 件 |
| AC-21 | static | `make deadcode` | 出力に `internal/fileanalysis/syscall_store.go` の行が現れない |
| AC-22 | static | `test ! -e internal/fileanalysis/syscall_store_test.go && echo absent` | `absent` が出力される |
| AC-22 | static | `rg -n 'TestE2E_RecordToRunnerFallbackChain' internal/security/elfanalyzer/` | 一致 0 件 |
| AC-22 | test | `internal/common/syscall_grouping_test.go::TestGroupAndSortSyscalls`、`internal/fileanalysis/file_analysis_store_test.go::TestStore_ArgEvalResultsRoundtrip` | 成功（削除したテストが検証していた不変条件が現役 API で保持されている） |
| AC-23 | static | `rg -c --glob '*.go' 'SyscallAnalysisData'`、`rg -n 'type SyscallAnalysisStore interface' internal/security/elfanalyzer/syscall_store.go`、`rg -c --glob '*.go' 'NewStandardELFAnalyzerWithSyscallStore'` | 3 つとも 1 件以上一致する（現役の `fileanalysis.SyscallAnalysisData`、別型である `elfanalyzer.SyscallAnalysisStore` の宣言、および `NewStandardELFAnalyzerWithSyscallStore` が残っている）。`elfanalyzer` 内では自パッケージの型を修飾せずに参照するため、`elfanalyzer.SyscallAnalysisStore` という文字列では検索できない点に注意する |
| AC-23 | static | `make build` | 成功（`cmd/record` / `cmd/runner` / `cmd/verify` がビルドできる） |
| AC-23 | test | `internal/filevalidator`、`internal/runner/base/security`、`internal/dynamicanalysis`、`internal/security/elfanalyzer` の既存テスト（`make test`）および `make elfanalyzer-integration-test` | すべて成功 |
| AC-24 | static | 本 AC は AC-21〜AC-23 の**代替条件**であり、本計画は削除（AC-21〜AC-23）を採る。理由は設計書 §2.4.3 に記載済み | 検証不要（AC-21〜AC-23 の充足で置き換わる） |

---

## 8. 横断検索チェックリスト

`make lint` と `make test` では検出できない残存参照と記述の不整合のみを挙げる。§7 の AC 検証表に含まれる検索式は重複させない。

rg は Rust の正規表現構文を用いるため、`\|` は選択ではなく**リテラルのパイプ文字**として解釈される。複数語を探すときは必ず `-e` を並べる形にすること。

- [ ] `rg -n --glob '*.go' '\bSource\b' internal/runner/base/environment/` — 削除した `Source` 型の残存参照が 0 件であること
- [ ] `rg -n --glob '*.go' 'ErrMalformedEnvVariable'` — `environment` パッケージのコメントで参照していた `config` 側の sentinel が現役であり、コメント削除により孤立した記述が残っていないこと
- [ ] `rg -n 'filter\.go' docs/dev/ README.md README.ja.md` — 一致 0 件であること。変更前は 6 件（`security-architecture.{ja.,}md` 各 1、`config-inheritance-behavior.{ja.,}md` 各 2）が一致し、いずれも §2.3 で修正対象としている（`docs/tasks/` の過去タスク記録は当時の状態として残すため検索対象に含めない）
- [ ] `rg -n 'filter_benchmark_test' docs/dev/` — 削除したファイルへの参照が残っていないこと
- [ ] `rg -n 'environment/.*filtering' docs/dev/developer_guide/package_reference.md` — 一致 0 件であること。変更前は :29 と :87 の 2 件（`environment/`: "Environment variable processing and filtering"）が一致する
- [ ] `rg -n 'privilege/unix\.go' docs/dev/architecture_design/security-architecture.ja.md docs/dev/architecture_design/security-architecture.md` — §5「特権管理」の構造体引用は設計書 §7.3 の判断に従い本タスクでは修正しない。Phase 2 で不正確さが増すことを確認し、[#919](https://github.com/isseis/go-safe-cmd-runner/issues/919) に追記すること
- [ ] `rg -n -e 'getProcessEUID' -e 'SyscallAnalysisStore' -e 'WithUserGroup' docs/translation_glossary.md` — 用語集に削除対象の識別子が登録されていないこと（現状は未登録）
- [ ] `rg -n --glob '*_test.go' -e 'AC-M1-4' -e 'AC-M1-5'` — 一致 0 件であること。変更前は `internal/runner/base/privilege/unix_privilege_test.go:508` と `:540` の 2 件が一致し、いずれも削除対象のテストの doc コメントである

---

## 9. 成功基準

- [ ] AC-01〜AC-23、AC-25〜AC-29 のすべてに対し、§7 の検証手段が実行され期待結果を満たしている（AC-24 は AC-21〜AC-23 を採るため対象外）
- [ ] 各 AC に少なくとも 1 つの `test` または `static` の検証が対応している
- [ ] `make fmt` / `make test` / `make lint` がグリーンである
- [ ] `make build` が成功する
- [ ] `go vet -tags 'test integration' ./...` が成功する
- [ ] `make deadcode` の出力から `internal/fileanalysis/syscall_store.go` の 3 行が消えている
- [ ] `make test-ci-cgo1` / `make test-ci-cgo0` / `make elfanalyzer-integration-test` が成功する
- [ ] Phase 2 について §2.4 の特権環境確認 5 項目が完了している
- [ ] Phase 1〜4 のそれぞれが独立してレビュー可能な PR に分かれており、いずれかを見送っても他の成果が成立する
- [ ] 削除対象に直接依存していたテスト（§2 の各 Step で列挙）を除き、既存テストが無修正で pass している
- [ ] `CHANGELOG.md` の `[Unreleased]` に Phase 3 の挙動変化と、グループ書き込み可能なファイルに残る passwd 依存が記載されている
- [ ] Phase 1 が触る 3 組のバイリンガル文書（`security-architecture`、`config-inheritance-behavior`、`design-implementation-overview`）について、日本語版と英語版の記述が一致している

---

## 10. 残作業

- [ ] 本実装計画書のレビューと `approved` への更新（レビュー後に実装コードの作成を開始する）
- [ ] Step 1〜4 の実装と、各 Step の PR 作成・レビュー・マージ
- [ ] Phase 2 マージ後に、設計書 §9 の検討事項として記録済みの後続タスクへ進捗を反映する
  - dry-run と実行時で別実装になっている識別情報の解決（[#918](https://github.com/isseis/go-safe-cmd-runner/issues/918)）
  - `security-architecture` §5「特権管理」の全面更新（[#919](https://github.com/isseis/go-safe-cmd-runner/issues/919)）
