# セキュリティ堅牢化（セキュリティリスク評価レポート対応） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-14 |
| Review date | 2026-07-14 |
| Reviewer | isseis |
| Comments | - |

## 0. 本書の位置づけ

本書は [`02_architecture.md`](02_architecture.md)（status: `approved`）で確定した実現機構（HOW）を、
実行可能なタスクへ分解した実装計画書である。要件は [`01_requirements.md`](01_requirements.md)
（AC-01〜AC-22）、検出事項は [`00_security_risk_report.md`](00_security_risk_report.md) を参照する。
フェーズ構成は `02_architecture.md` §8「実装の優先順位」の Phase 0〜4 に従う。

---

## 1. 実装概要

### 1.1 目的

`01_requirements.md` の F-001〜F-007（AC-01〜AC-22）を、`02_architecture.md` の設計に従って実装する。
主な変更範囲は次の 3 面である（`02_architecture.md` §1.2 参照）。

1. 下方委譲面（特権分離）: run-as コマンド実行を `SysProcAttr.Credential` によるカーネル側アトミック設定へ移行（F-001/F-002）。
2. 外部流出面（機微情報）: 値ベースの秘密検出器を Redaction 経路へ接続（F-003）。
3. 信頼の起点（生成と検証）: `record` の権限違反フェイルクローズド化、dry-run 未検証内容の明示区別（F-004/F-005）。

F-006/F-007 はドキュメント専用対応であり、コンポーネントの流れを持たない。

### 1.2 実装原則

`02_architecture.md` §1.1 の設計原則（フェイルクローズド徹底・後方互換の保持・DRY・カーネル側アトミック設定の優先）を実装全体で踏襲する。加えて:

- 各フェーズは独立してグリーンゲート（`make test && make lint`）を満たした状態でマージする。
- 既存テストの期待値変更は「新規追加」ではなく「既存テスト更新」として明示的にタスク化する（挙動反転を伴う箇所があるため）。

### 1.3 既存コード調査結果

実装着手前にコードベースを調査した結果を示す（`mkplan.md` 手順 5）。以降のフェーズ記述はこの調査結果を前提とする。

#### F-001 / F-002（privilege / executor / risk）

- **`internal/runner/base/privilege/unix.go`**（`//go:build !windows`、darwin を含む）:
  - `UnixPrivilegeManager` 構造体（28-42行目）は `syscallSeteuid`/`syscallSetegid`/`identityVerifier` の3つの注入可能フィールドを持つが、これらは `changeUserGroupInternal` 内部（511・515行目）でのみ使用される。`escalatePrivileges`（217-252行目）・`restorePrivileges`（256-273行目）・`restoreUserGroupInternal`（534-545行目）は生の `syscall` パッケージを直接呼んでおり、注入経路を経由しない。新設する suid（saved-set-uid）/sgid（saved-set-gid）捕捉・検証ロジックをテスト可能にするには、これと同方式（注入可能フィールド）で切り出す必要がある。
  - `executionContext` 構造体（92-108行目）は `originalEUID`/`originalEGID` のみを捕捉しており、suid/sgid フィールドは存在しない（新規追加が必要、AC-06）。
  - `changeUserGroupInternal`（414-530行目）は `Setegid`→`Seteuid` のみを呼び、`setgroups(2)` の呼び出しはファイル全体に存在しない（`grep` で確認済み、AC-01 のギャップ）。
  - `OperationUserGroupExecution`/`OperationUserGroupDryRun`/`OperationFileValidation` は `internal/runner/base/runnertypes/config.go`（161-164行目）で定義されている。`prepareExecution`（111-134行目）が `Operation` により `needsPrivilegeEscalation`/`needsUserGroupChange` を設定する分岐であり、`OperationUserGroupExecution`/`OperationUserGroupDryRun` はいずれも `needsUserGroupChange=true` になっている。**実装前に `performElevation`（147行目付近）が `needsUserGroupChange` をどう処理し、`changeUserGroupInternal` が dry-run 経路で実際に identity を変更するのか検証のみなのかを確認すること。** `OperationUserGroupExecution` を「root 昇格のみ・親の identity 変更なし」に変更する際、`OperationUserGroupDryRun` の既存挙動（AC-04 の回帰対象）を壊さないよう、その境界を正確に把握する必要がある。
  - `defaultIdentityVerifier`（56-67行目）は EUID==UID / EGID==GID のみを検証し `ErrIdentityLeak`（26行目）でラップする。suid/sgid 検証の追加先はここ、または `restorePrivilegesAndMetrics`（206行目付近で `needsVerification` 判定）である。
  - 既存テスト: `unix_test.go`（`TestUnixPrivilegeManager_WithUserGroupInternal`, `TestUnixPrivilegeManager_PrivilegeValidation`）、`unix_privilege_test.go`（18件、うち `TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown`/`_IdentityVerificationSkippedForDryRun`/`_IdentityVerificationPassesOnCleanRestore`/`TestDefaultIdentityVerifier`/`TestEmergencyShutdown` が suid/sgid 検証追加の直接対象）、`race_test.go`（並行性、変更不要見込み）、`manager_test.go`。
- **`internal/runner/base/executor/executor.go`**:
  - `DefaultExecutor` 構造体（46-54行目）に `SysProcAttr`/`Credential` の使用は現状ゼロ（grep で確認済み）。`executeWithUserGroup`（134-205行目）は `PrivMgr.WithPrivileges` に委譲するのみで、子プロセスの identity 設定は行っていない。
  - `prepareExecCommand`（360-397行目）が fd-bound exec（`/proc/self/fd`、`ExtraFiles`）と `stageFromFD`（404行目以降、staging fallback）を選択する箇所。`stagedExecMode = 0o500`（27行目で定数宣言）。**`stageFromFD` を全文確認済み**: 明示的な `chown` 呼び出しは存在しない。現行方式では、親プロセスが対象ユーザーへ `seteuid` した後で `os.OpenFile` によりコピーを作成するため、ファイル所有権はカーネルがプロセスの実効 uid に基づき作成時点で暗黙に決定している（明示的な chown 呼び出しの削除は不要）。新方式（Step 1-4 で親が root のまま）では、この暗黙のメカニズムにより自動的に root 所有のコピーになる。コピー先ディレクトリ（`os.MkdirTemp`、既定権限 `0o700` の可能性が高い）の権限は実装時に確認する。
  - `identityChecker`/`defaultIdentityChecker` は `executor_privilege_check_unix.go`（10-27行目）にあり、`ErrPrivilegeLeak` でラップする。これは executor 側の実行後チェックであり、privilege 側の `executionContext` 不変条件検証とは別物（両方維持する）。
  - 既存テスト: `executor_test.go`（`TestDefaultExecutor_ExecuteUserGroupPrivileges` 系8件）、`executor_privilege_check_test.go`（2件）、`executor_fdexec_test.go`（`TestExecute_FdBoundOrStaging` 等、Credential 導入時に fd-bound 経路との相互作用を再検証する対象）。`executor_usergroup_test.go` は現状**存在しない**（`02_architecture.md` の記載は新設想定）。
- **`internal/runner/base/risk/runas_identity.go`**:
  - `resolveRunAsIdent(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error)`（51-85行目）が唯一の解決関数。`originalExecutionIdentity()`（24-38行目）・`supplementaryGroups(u *user.User) []uint32`（90-102行目）も同ファイル。
  - 呼び出し元は `risk/evaluator.go:84`（`resolveRunAs` フィールドへの代入）と `runas_identity_test.go`（6件のテスト関数: `TestResolveRunAsIdent_UserOnly`/`_GroupOnly`/`_UserAndGroup`/`_UnknownUser`/`_UnknownGroup`/`TestOriginalExecutionIdentity`）のみ。**`executor`/`privilege` からの呼び出しは現状ゼロ**（新規配線が必要）。
  - `risktypes.RunAsIdent` は `internal/runner/base/risktypes/operand_zone.go`（62-66行目）に定義済み（`UID/GID uint32`, `Groups []uint32`）。`risktypes` パッケージに `runas_ident.go` は現状**存在しない**（新規ファイル）。
  - インポートグラフ確認: `risk` は `risktypes` に依存、`executor` も既に `risktypes` に依存（`executor.go:6-23`）、`privilege/unix.go` は現状 `risktypes` に依存していない（新規依存を追加するが循環は生じない）。`risk`→`executor`/`privilege` の依存、`executor`/`privilege`→`risk` の依存はいずれも存在しない。よって `resolveRunAsIdent` 等を `risktypes` へ移設しても循環インポートは発生しない（確認済み）。
  - `go.mod:19` に `golang.org/x/sys v0.35.0 // indirect` が存在。`unix.Getresuid`/`Getresgid` を使用すると `go mod tidy` で `indirect` が外れ直接依存になる。
- **suid/sgid 読み取り**: `golang.org/x/sys/unix`（`go.mod` 固定バージョン v0.35.0）で `Getresuid`/`Getresgid` が定義されているのは `syscall_linux.go`・`syscall_freebsd.go`・`syscall_openbsd.go` のみであり、**darwin 版には存在しないことを確認済み**。したがって `identity_linux.go`（`//go:build linux`、実装）と `identity_other.go`（`//go:build !linux && !windows`、no-op）へのファイル分割は必須であり、`02_architecture.md` §3.1.2 の「非 Linux ではガードした no-op」という設計判断と整合する。

#### F-003（redaction）

- `internal/redaction/` に `value_detector.go` は存在しない（新規ファイル）。
- `sensitive_patterns.go` の `IsSensitiveValue`（131-133行目）は `IsSensitiveKey` と同じ `combinedCredentialPattern`（キー名指向の正規表現）を再利用しているだけであり、値フォーマット（`AKIA...`/`ghp_...`/PEM 等）の検出器ではない（F-003 の「現状の問題」の裏付け）。
- **重要な発見**: `redactor.go` の `Config.RedactText`（55-68行目）は `KeyValuePatterns` によるキー=値置換のみを行い、`IsSensitiveValue` を一切呼び出していない。値フォーマット検出（`IsSensitiveValue`）は `RedactLogAttribute`（71-106行目）と `redactLogAttributeWithContext`（429行目以降、Layer 2 の内部実装）からのみ呼ばれている。`02_architecture.md` §3.3.3 の図は「`ValueDetector` → `RedactText`」という統合点を示すが、現状の `RedactText` 自体は値ベース判定を持たない。**実装方針の確定が必要**: 統合先には二つの選択肢がある。(a) `ValueDetector` を `RedactText` に直接統合する方式。Layer 1 の `SanitizeOutputForLogging` は `RedactText` のみを呼ぶため、これが唯一 Layer 1 に届く経路である。(b) 既存の `IsSensitiveValue` 呼び出し箇所を `ValueDetector` に差し替える方式。この場合 Layer 1 には届かない。どちらを選ぶかで、AC-09 の「コマンド引数・stdout/stderr・環境変数値」への適用範囲が変わる。Layer 1（`SanitizeOutputForLogging`、`internal/runner/base/security/logging_security.go:29-52`）は stdout/stderr の生テキストに対して `RedactText` のみを呼ぶため、**`ValueDetector` は `RedactText` 内部に統合する**（(a) を採用し、Layer 1/Layer 2 双方に一括適用する）。
- Layer 1 = `internal/runner/base/security/logging_security.go` の `SanitizeOutputForLogging`（29-46行目）→ `redactSensitivePatterns`（48-52行目）→ `v.redactionConfig.RedactText`。呼び出し元: `internal/runner/group_executor.go:260-261`。
- Layer 2 = `internal/redaction/redactor.go` の `RedactingHandler.Handle`（388-400行目）→ `redactLogAttributeWithContext`（429行目以降、444行目で `RedactText` 呼び出し）。
- Slack 経路: `internal/logging/slack_handler.go` の `SlackHandler.Handle`（225-275行目）自体は redaction を行わず、`internal/runner/bootstrap/logger.go`（178・249行目）で `RedactingHandler` が Slack を含む `multiHandler` をラップしている。したがって `RedactText` への統合のみで Slack 経路にも自動的に伝播する（`slack_handler.go` の変更は不要）。
- 既存テスト: `redactor_test.go`（`TestRedactText_*` 系7件、`TestRedactLogAttribute_*` 系6件、`TestRedactingHandler_*` 系多数）、`sensitive_patterns_test.go`。

#### F-004（verification / dry-run）

- `internal/verification/manager.go` の `readAndVerifyFileWithReadFallback`（422-450行目）に経路1（423-427行目、`fileValidator == nil` → `ResultCollector` に一切記録せず `os.ReadFile`）と経路2（432-447行目、dry-run 検証失敗 → `RecordFailure` 後 `os.ReadFile` 再読込）が確認できた。`newManagerInternal`（454-543行目）の488-507行目で、`os.ErrPermission` かつ dry-run の場合に `fileValidator` が実行全体で nil のまま確定する（経路1が発生する条件）。
- `VerifyEnvironmentFile`（92-124行目）は `verifyFile`（内容を返さない）を呼ぶ別経路であり、106-108行目で dry-run 時の検証失敗を握りつぶして `nil` を返す。env ファイルの実内容読み込みは別箇所（`config` ローダ）で行われるため、本フェーズでの対応要否は実装時に該当箇所を確認して判断する（`02_architecture.md` §3.4.1 の指示どおり）。
- `internal/verification/result_collector.go` の `ResultCollector` 構造体（23-30行目）に `UsedUnverifiedContent` フィールドは存在しない（新規追加）。`internal/verification/types.go`（110-124行目）の `FailureReason` 定数は `ReasonHashDirNotFound`/`ReasonHashFileNotFound`/`ReasonHashMismatch`/`ReasonFileReadError`/`ReasonPermissionDenied`。
- `internal/runner/resource/types.go:97` に `FailOnVerificationUnavailable`、`114`行目に `DryRunExitVerificationUnavailable = 3`（`const` ブロックは103行目から、`DryRunExitAllow = 0`・`DryRunExitPolicyDeny = 1`）。`internal/runner/resource/dryrun_manager.go` の `previewExitCodeLocked()`（421-439行目）が2分岐（policy-deny／verification-unavailable）の終了コード決定ロジックであり、これを改ざん兆候用の policy-deny 分岐へ拡張する対象。
- `internal/runner/resource/formatter.go` の `TextFormatter.writeFileVerification`（134-167行目）が dry-run 出力のテキスト整形箇所。`formatReason`（170-185行目）は `FailureReason` の switch であり、UNVERIFIED を新しい `FailureReason` 値として追加するのではなく、`FileVerificationFailure` または別構造体に `UsedUnverifiedContent`/理由文字列を持たせて表示を追加する（既存 `FailureReason` の意味を変えないため）。
- **`02_architecture.md` の記載訂正（実装時に注意）**: 同文書 §3.4.3 のコメントは回帰対象テストとして `internal/runner/resource/dryrun_manager_test.go` の `FailOnVerificationUnavailable` 関連ケースを挙げているが、実際には `internal/runner/resource/security_test.go` の `TestDryRun_VerificationUnavailableExitCode`（151-176行目、`DryRunExitVerificationUnavailable` を直接参照）が該当テストである。`dryrun_manager_test.go` に `FailOnVerificationUnavailable` を参照するテストは存在しない。本計画のテストタスクは `security_test.go` を対象とする。
- `internal/verification/manager_test.go` の `TestReadAndVerifyFileWithReadFallback_DryRunLogging`（991行目）は現状「dry-run で検証失敗しても `err == nil` で内容が返る」ことを `assert.NoError` で確認しており、`UsedUnverifiedContent` 導入後もこのアサーション自体は変わらない想定だが、新フィールドの検証を追加する必要がある（既存テストの拡張、削除ではない）。

#### F-005（record）

- `cmd/record/main.go`: `hashDirPermissions = 0o750`（27行目）。`RunTOCTOUPermissionCheck` の戻り値は126行目で完全に破棄されている（95-98行目のコメントが現状の意図的な warn-only 挙動を明記）。`mkdirAll` は `parseArgs` 内（212行目）で呼ばれ、TOCTOU チェック（126行目）より前に実行される。`processFiles`（233-261行目）がハッシュ生成ループ。`--force` は `recordConfig.force` を経由して `SaveRecord` に渡されるのみで、権限バイパスとは無関係（245行目）。
- `internal/security/toctou.go` の `RunTOCTOUPermissionCheck(checker, dirs, logger) []TOCTOUViolation`（82-102行目）は既に `TOCTOUViolation{Path, Err}`（11-14行目）のスライスを返しており、シグネチャ変更は不要。`CollectTOCTOUCheckDirs`（33行目、`addWithAncestors` ヘルパーで祖先ディレクトリを含む）も変更不要。
- **既存テストの反転が必要**: `cmd/record/main_test.go` の `TestRunTOCTOU_ContinuesOnWorldWritableDir`（179-201行目）は現状 `assert.Equal(t, 0, exitCode, "record should continue (exit 0) despite world-writable directory")` を明示的にアサートしており、これは F-005 実装後は真逆の挙動（非ゼロ終了・ハッシュ未生成）になる。**このテストは削除ではなく期待値を反転させて存続させる**（同じ入力条件を再利用できるため）。

#### F-006 / F-007（ドキュメント）

- `safefileio.MaxFileSize`（`internal/safefileio/safe_file.go:333-334`、128MB）と `filevalidator` の非公開 `maxFileSize`（`internal/filevalidator/validator.go:1399-1402`、1GB、バイナリ解析専用）は**別の定数**であり、要件書 F-6 の要約は128MBに単純化しているが、`02_architecture.md` §3.6 は両方を正しく併記している。ドキュメント記述では両者を区別して明記する。
- `docs/user/` の言語サフィックス規約は「英語=無印（`*.md`）、日本語=`*.ja.md`」であることを確認（`.en.md` は存在しない）。対象ファイル: `record_command.md`/`.ja.md`、`runner_command.md`/`.ja.md`、`security-risk-assessment.md`/`.ja.md`。
- 128MB・`MaxFileSize` の記述は上記6ファイルのいずれにも存在しない（AC-20 は完全新規）。TOCTOU は `runner_command.*` に既存記載があるが `record` の権限チェックに関する記述はない（AC-16/AC-18/AC-19 は新規）。openat2 は `security-risk-assessment.*` に既存の詳細な記述があり拡張可能（AC-22 は既存セクションの拡張）。`--force`/フェイルクローズド/フェイルクローズは対象ファイルに未記載。
- `docs/translation_glossary.md`: 「フェイルクローズド」（172行目）・「Redaction」（410行目）は既存。「補助グループ」「saved-set-uid」は未登録（追加が必要）。

---

## 2. 実装ステップ

### Phase 0: PoC（fd-bound 実行 + `SysProcAttr.Credential` の相互作用検証）

`02_architecture.md` §3.1.3 の PoC。Phase 1 の実装方式（fd-bound 実行を維持するか、代替機構に切り替えるか）を決定する前提作業であるため、Phase 1 の着手前に完了させる。

#### Step 0-1: PoC 実行環境の準備

**対象**: 実装外（手動検証環境。setuid-root または `sudo` が使える Linux 環境。Docker コンテナ等で代替可）

- [x] Linux 環境（本番想定の kernel）で、setuid-root 相当のプロセスから `run_as` 経由の fd-bound 実行（`/proc/self/fd/<n>` を argv[0] とする `execve`）を模擬する最小再現コードを用意する。
- [x] 当該プロセスが `execve` 直前に `SysProcAttr.Credential`（`Uid`/`Gid`/`Groups`、`NoSetGroups: false`）で非 root へ降格した場合に、`/proc/self/fd/<n>` の解決が `EACCES` にならず成功することを確認する。

#### Step 0-2: PoC 結果の記録と方式確定

**対象ファイル**: `docs/tasks/0146_security_hardening/02_architecture.md`（§3.1.3、承認後文書だが同節が明示的に「PoC 結果をインラインで記録する」としている追記対象）

- [x] PoC が成立した場合: 現行の fd-bound 実行方式を維持する旨と検証手順・結果を §3.1.3 に追記する。
- [x] PoC が不成立だった場合: `02_architecture.md` §3.1.3 に列挙された代替（(1) `execveat(fd, "", AT_EMPTY_PATH)` 相当、(2) `PR_SET_DUMPABLE` 維持、(3) staging fallback への切替）から採用方式を選定し、選定理由とともに追記する。
- [x] 選定結果に応じて Phase 1 の Step 1-6/1-7（後述）の実装方針を確定する。選定内容が (3) staging fallback 以外（本書が事前に想定していない (1) `execveat(fd, "", AT_EMPTY_PATH)` 相当、または (2) `PR_SET_DUMPABLE` 維持）だった場合、Step 1-6/1-7 の記述だけでは実装粒度のタスク分解が不十分になり得るため、Phase 1 の Step 1-5 より後に進む前に本書の該当 Step を補足改訂する。

**成功基準**: `02_architecture.md` §3.1.3 に PoC 結果と採用方式が記録され、Phase 1 の実装方針に矛盾がないこと。

### PR-1 作成ポイント: PoC results and architecture decision record

**対象ステップ**: 0-1 / 0-2

**推奨タイトル**: `docs(0146): record fd-bound execution PoC results in architecture doc`

**レビュー観点**: PoC 手順の再現可能性 / 選定方式が Phase 1 の Step 1-6/1-7 の前提と矛盾しないか / 不成立時の代替方式選定理由の妥当性

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 1: F-001 / F-002 — `SysProcAttr.Credential` によるカーネル側アトミック設定

#### Step 1-1: run-as identity 解決関数を `risktypes` へ移設する

**対象ファイル**:
- 新規: `internal/runner/base/risktypes/runas_ident.go`
- 新規: `internal/runner/base/risktypes/runas_ident_test.go`（移設元テストの移動先）
- 変更: `internal/runner/base/risk/runas_identity.go`（削除、または `risktypes` への薄い re-export は行わず呼び出し元を直接差し替える）
- 変更: `internal/runner/base/risk/runas_identity_test.go`（`risktypes` へ移動したテストを削除）
- 変更: `internal/runner/base/risk/evaluator.go`（84行目、410行目付近の呼び出し元を `risktypes.ResolveRunAsIdent` に差し替え）

- [ ] `resolveRunAsIdent`・`originalExecutionIdentity`・`supplementaryGroups` を `internal/runner/base/risk/runas_identity.go` から `internal/runner/base/risktypes/runas_ident.go` へ移設する。`risk`/`executor`/`privilege` の全パッケージから呼べるよう、移設後は `resolveRunAsIdent` → `ResolveRunAsIdent`、`originalExecutionIdentity` → `OriginalExecutionIdentity` にエクスポートする（`supplementaryGroups` は同パッケージ内専用ヘルパーのため非公開のまま）。
- [ ] **重要（`risk`/`executor` 間の base identity 共有）**: 現状 `risk.NewStandardEvaluator`（`evaluator.go:84,91`）は `originalExecutionIdentity()` をプロセス起動時（特権昇格前）に一度だけ呼び、以降の全コマンドの `resolveRunAs` 呼び出しでその単一のキャッシュ値を `base` として使い回している。移設後の `OriginalExecutionIdentity()` を `executor` が実行のたびに再度呼び出す（＝呼び出し時点で再取得する）実装にしてはならない。呼び出しタイミングが異なると、`risk` の dry-run 判定に使われた `base` と `executor` が実際に `Credential` へ渡す `base` が異なる値になり得る（`02_architecture.md` §3.1.2 が要求する「`risk` と `executor` が同一関数を呼ぶことで同じ集合を返すことを保証する」という DRY の前提が崩れる。同 §5.3 も「捕捉は昇格前に行うことを実装上の不変条件とする」と明記している）。**`OriginalExecutionIdentity()` は `sync.OnceValue`（Go 1.21+ 標準ライブラリ、本プロジェクトの Modern Go Idioms 規約に準拠）でラップし、プロセス内で最初に呼ばれた時点（`risk` の評価器構築時、まだ特権昇格前）の値を全呼び出し元に対して恒久的に返すこと。`executor` はこの共有キャッシュ値を参照するのみとし、独自に `os.Getuid`/`Getgid`/`Getgroups` を再呼び出ししない。**
- [ ] `internal/runner/base/risk/evaluator.go` の呼び出し箇所（`resolveRunAs` フィールドへの代入・呼び出し）を `risktypes.ResolveRunAsIdent` に差し替える。
- [ ] `internal/runner/base/risk/runas_identity_test.go` のテストケースを `risktypes/runas_ident_test.go` に移動する（`risk` パッケージ側のテストファイルは削除、または `risk` 固有の統合的な呼び出しテストのみ残す）。
- [ ] 移設後、`risk`・`risktypes` パッケージが `executor`/`privilege` を新たにインポートしないこと（循環インポートが発生しないこと）を `go build ./...` で確認する。

**成功基準**: `go build ./...` が成功し、`risk` パッケージの既存テスト（5件、移動後は `risktypes` 配下）が全てパスする。

#### Step 1-2: 親プロセスの操作開始時 suid/sgid 捕捉を実装する

**対象ファイル**:
- 新規: `internal/runner/base/privilege/identity_linux.go`（`//go:build linux`）
- 新規: `internal/runner/base/privilege/identity_other.go`（`//go:build !linux && !windows`）
- 新規: `internal/runner/base/privilege/identity_linux_test.go`
- 新規: `internal/runner/base/privilege/identity_other_test.go`

- [ ] `identity_linux.go` に `readSavedIDs() (suid, sgid int, err error)` を実装し、`golang.org/x/sys/unix.Getresuid`/`Getresgid` を呼ぶ（`!windows` 単一ファイルへの簡略化はしない。darwin に同関数が存在しないことを確認済み、既存コード調査結果参照）。
- [ ] `identity_other.go` に同シグネチャの no-op 実装（`suid=0, sgid=0, err=nil` を返す。呼び出し側は「非 Linux では検証を省略する」前提でこの戻り値を扱う）を用意する。
- [ ] `identity_other_test.go` を追加し、no-op 実装が `readSavedIDs()` で常に `(0, 0, nil)` を返すことを検証する（本プロジェクトの開発機は darwin であり、この no-op 経路は実際に到達可能なコードパスであるため）。
- [ ] `go.mod` の `golang.org/x/sys` を `go mod tidy` で直接依存へ昇格させる。

**成功基準**: `identity_linux_test.go` が Linux 上で `readSavedIDs()` の戻り値と `/proc/self/status` の `SUID`/`SGID` 行が一致することを検証し、パスする。

#### Step 1-3: `executionContext` に suid/sgid を追加し、復元後不変条件検証に組み込む

**対象ファイル**: `internal/runner/base/privilege/unix.go`, `internal/runner/base/privilege/unix_privilege_test.go`

- [ ] `executionContext` 構造体（92-108行目）に `originalSUID int`・`originalSGID int` フィールドを追加する。
- [ ] `originalEUID`/`originalEGID` を捕捉している箇所（`prepareExecution`/`performElevation` 付近。実装時に正確な捕捉箇所を特定する）に、Step 1-2 の `readSavedIDs()` 呼び出しを追加し `originalSUID`/`originalSGID` を捕捉する。捕捉失敗時のフェイルクローズド方針（エラー返却か、非 Linux 用 no-op の扱いか）を実装時に確定する。
- [ ] `defaultIdentityVerifier` または `restorePrivilegesAndMetrics`（`needsVerification` 判定箇所）に、復元後の suid/sgid を再取得して `executionContext` の捕捉値と比較する検証を追加する。**suid/sgid は `real UID` と比較してはならず、操作開始時に捕捉した値と比較する**（`02_architecture.md` §3.1.2 の不変条件定義）。
- [ ] 不一致時は既存の `ErrIdentityLeak` でラップし、既存の emergency shutdown（`emergencyShutdown`、276-295行目）経路を再利用する。

**成功基準**: 後述 AC-06/AC-07 のテストがパスする。

### PR-2 作成ポイント: identity capture and invariant verification foundation

**対象ステップ**: 1-1 / 1-2 / 1-3

**推奨タイトル**: `feat(0146): capture saved-set-uid/gid and verify post-restore invariants`

**レビュー観点**: `risk`/`executor` 間で `OriginalExecutionIdentity()` の単一キャッシュが共有される保証（`sync.OnceValue` の適用箇所） / `linux` 以外での no-op 境界（`darwin` で到達可能なコードパスであること） / suid/sgid 検証が `real UID` ではなく操作開始時捕捉値と比較されているか

**注記**: AC-01〜AC-03/AC-06 の網羅的なテストは Step 1-8（PR-5）で追加される。本 PR の時点では `go build ./...` と既存テストの継続パスのみが自動検証の範囲であり、新規追加ロジック（suid/sgid 捕捉・不変条件検証）の正しさはコードレビューに強く依存する点をレビュー時に明示的に確認すること。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### Step 1-4: `OperationUserGroupExecution` を「root 昇格のみ・親の identity 変更なし」に変更する

**対象ファイル**: `internal/runner/base/privilege/unix.go`, `internal/runner/base/privilege/unix_privilege_test.go`

- [ ] 実装前に `performElevation`（147行目付近）と `WithPrivileges` の `needsUserGroupChange` 分岐を精読し、`OperationUserGroupExecution` と `OperationUserGroupDryRun` それぞれで `changeUserGroupInternal` が呼ばれる現状の条件を正確に把握する。
- [ ] `OperationUserGroupExecution` の場合、親プロセスの `changeUserGroupInternal` 呼び出し（対象ユーザーへの実際の identity 切替）を実行経路から外す。root への昇格（`escalatePrivileges`）のみを行う。
- [ ] `OperationUserGroupDryRun` は既存どおり検証・ログのみで identity 変更を行わない状態を維持する（回帰させない、AC-04）。
- [ ] `restoreUserGroupInternal` が `OperationUserGroupExecution` 経路で不要になる場合は、呼び出し箇所を整理する（未使用になった関数・分岐を放置しない）。

- [ ] 注入した `syscallSeteuid`/`syscallSetegid` モックへの呼び出しを検証する新規テスト（`TestChangeUserGroupInternal_NotCalledForUserGroupExecution` 等）を本ステップ内で追加し、`OperationUserGroupExecution` 経路で `changeUserGroupInternal`（対象ユーザーへの実際の identity 切替）が呼ばれないことを検証する。**このテストは Step 1-8 まで先送りしない**（本ステップは Phase 1 中で最もリスクの高い単一変更であり、対応する PR に専用の自動回帰テストを含めることを必須とする）。

**成功基準**: `TestChangeUserGroupInternal_*`（`unix_privilege_test.go`）・`TestRestoreUserGroupInternal` が新しい呼び出し条件に合わせて更新され、パスする。既存の `TestManager_WithPrivileges_UserGroup_ValidUser`（`manager_test.go:110-152`）の `actual_change` サブテストは `manager.GetCurrentUID() == 0` の場合のみ実行され、かつ `err`/`executed` の成否しか検証しておらず、identity 変更の有無を検証しない（通常の CI では実行されず、実行されても本ステップの変更点を検証できない）。**本ステップの回帰防止テストとしては使用しない**。代わりに、上記で本ステップ内に追加する `TestChangeUserGroupInternal_NotCalledForUserGroupExecution` を、`OperationUserGroupExecution` 経路で `changeUserGroupInternal`（対象ユーザーへの実際の identity 切替）が呼ばれないことの回帰防止テストとする。

### PR-3 作成ポイント: OperationUserGroupExecution behavior change

**対象ステップ**: 1-4

**推奨タイトル**: `feat(0146): stop parent identity switch for OperationUserGroupExecution`

**レビュー観点**: `performElevation`/`WithPrivileges` の `needsUserGroupChange` 分岐の解釈が正しいか / `OperationUserGroupDryRun` の既存挙動（AC-04）が回帰していないか / 未使用になった分岐・関数が整理されているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### Step 1-5: `executor` で `SysProcAttr.Credential` を生成し run-as を実行する

**対象ファイル**: `internal/runner/base/executor/executor.go`, 新規 `internal/runner/base/executor/executor_usergroup_test.go`

- [ ] `executeWithUserGroup`（134-205行目）または `prepareExecCommand`（360-397行目）に、`risktypes.OriginalExecutionIdentity()`（Step 1-1 で `sync.OnceValue` 化した共有キャッシュ。`risk` が既に評価器構築時に確定させた値と同一）を `base` として `risktypes.ResolveRunAsIdent(base, userName, groupName)` を呼ぶ処理を追加する。
- [ ] 解決成功時、`execCmd.SysProcAttr.Credential = &syscall.Credential{Uid: ..., Gid: ..., Groups: ..., NoSetGroups: false}` を設定する。
- [ ] 解決失敗（未知ユーザー/グループ、補助グループ列挙不能）時は、コマンドを実行せずエラーを返し非ゼロ終了する（AC-02、フェイルクローズド）。`ErrRunAsIdentityResolution`（`02_architecture.md` §4.2 で定義予定のエラー型）を新設し使用する。
- [ ] 既存の `PrivMgr.WithPrivileges(OperationUserGroupExecution, ...)` 呼び出しは維持し、Step 1-4 で変更した「root 昇格のみ」の意味論と整合させる。

**成功基準**: 後述 AC-01/AC-02/AC-03 のテストがパスする。

#### Step 1-6: fd-bound 実行・staging fallback を新方式に整合させる（Phase 0 の PoC 結果に依存）

**対象ファイル**: `internal/runner/base/executor/executor.go`（`prepareExecCommand`, `stageFromFD`）

- [ ] Phase 0 の PoC 結果（Step 0-2）に従い、fd-bound 実行を維持する場合はそのまま Step 1-5 の `Credential` 設定と組み合わせる。代替方式が選定された場合は、その方式を実装する。
- [ ] Step 1-4 により親プロセスが root のまま `stageFromFD` を実行するようになった結果、コピーが root 所有になることを確認する（既存コード調査結果のとおり明示的な `chown` 呼び出しは存在しないため削除は不要。所有権は Step 1-4 の変更により自動的に切り替わる）。
- [ ] `stagedExecMode` 定数（27行目、現状 `0o500`）を `0o555` に変更する。
- [ ] `stageFromFD` 直上の doc コメント（400-404行目、現状「0700 temp directory」「created 0500」と記載）を、変更後の値（`0o711`/`0o555`）に更新する。
- [ ] staging コピー先ディレクトリ（`os.MkdirTemp` の権限、現状デフォルト `0o700` の可能性）を `0o711` に変更する。

**成功基準**: `executor_fdexec_test.go` の既存テストが新方式でパスし、staging fallback のファイル/ディレクトリ権限を検証する新規テストが追加される。

#### Step 1-7: 検証可能性のためのインタフェース注入を整備する

**対象ファイル**: `internal/runner/base/executor/executor.go`, `internal/runner/base/privilege/unix.go`

- [ ] `risktypes.ResolveRunAsIdent` の呼び出しと `Credential` 生成を、既存の `identityChecker`（executor 側）・`syscallSeteuid`/`syscallSetegid`（privilege 側）と同方式で注入可能にする（テストから期待値を差し替えられる形にする）。
- [ ] Step 1-2 の `readSavedIDs` も同様に注入可能なフィールドとして `UnixPrivilegeManager` に追加する。

**成功基準**: AC-03 のユニットテストがモック実装を注入して uid/gid/補助グループの一致を検証できる。

### PR-4 作成ポイント: executor Credential wiring and fd-bound/staging integration

**対象ステップ**: 1-5 / 1-6 / 1-7

**推奨タイトル**: `feat(0146): wire SysProcAttr.Credential into run-as execution`

**レビュー観点**: `Credential` 生成失敗時のフェイルクローズド経路（コマンド未実行・非ゼロ終了） / fd-bound 実行と `Credential` の相互作用が Phase 0 PoC 結果と整合しているか / staging fallback の権限変更（`0o555`/`0o711`）とドキュメントコメントの整合

**注記**: AC-01〜AC-03 の網羅的なテストは Step 1-8（PR-5）で追加される。本 PR の時点では既存テストの継続パスのみが自動検証の範囲であり、`Credential` フィールド生成ロジックの正しさはコードレビューに強く依存する点をレビュー時に明示的に確認すること。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### Step 1-8: F-001/F-002 の単体テストを追加・更新する

**対象ファイル**:
- `internal/runner/base/executor/executor_usergroup_test.go`（新規）
- `internal/runner/base/privilege/unix_privilege_test.go`（更新・追加）
- `internal/runner/base/risktypes/runas_ident_test.go`（Step 1-1 で移動済み）

- [ ] user-only / group-only / both の3形態それぞれで期待する `{UID,GID,Groups}` が生成されることを検証するテストを追加する（AC-01）。特に group-only で `base`（起動元の元 identity）から補助グループが継承され、起動元固有グループを含まないことを検証する。`RunAsIdent` 構造体（`Groups` スライスを含む）の deep equality 比較では、nil スライスと空スライスの不一致による偽陰性を防ぐため `cmpopts.EquateEmpty()` を使用するか、事前に nil/空を正規化すること。
- [ ] setuid-root 想定（`originalUID != 0`、開始時 suid=0）で、復元後に suid=0 のまま不変条件検証が成功することを検証するテストを追加する（`suid==uid` としないことの回帰防止、AC-06）。
- [ ] suid が開始時と変化したケースで emergency shutdown が呼ばれることを検証するテストを追加する（AC-06）。
- [ ] identity 解決失敗・`Credential` 設定失敗時にコマンドが実行されず非ゼロ終了することを検証するテストを追加する（AC-02）。
- [ ] 既存の `TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown` 等（`unix_privilege_test.go`）を、suid/sgid 検証を含む形に拡張する（AC-07 の回帰防止）。
- [ ] `run_as` を使用しない経路（native root 実行、dry-run）で補助グループ操作が行われないことを検証するテストを追加・確認する（AC-04）。
- [ ] `TestChangeUserGroupInternal_NotCalledForUserGroupExecution` は Step 1-4 で追加済みのため、本ステップでは重複追加しない。

**成功基準**: 追加・更新した全テストがパスし、`go test -tags test ./internal/runner/base/...` がグリーン。

#### Step 1-9: 統合テスト方針の実装（環境依存、タグ付き）

**対象ファイル**: `internal/runner/base/executor/`（`//go:build integration` 等の既存タグ規約に従う。実装時に規約を確認する）、新規 `internal/runner/base/executor/integration_skip.go`（`//go:build integration`）、新規 `internal/runner/base/executor/privileged_test_condition_test.go`

- [ ] スキップ判定を2段階に分離する: (1) ビルドタグ（`//go:build integration`）でコンパイル自体を CI 既定でオフにする、(2) ビルド後の実行時に root/setuid 権限が実際に使えるかを判定する純粋関数 `canRunPrivilegedIntegrationTest(euid int, targetUser string) (ok bool, reason string)` を実装し、ユニットテストで euid が 0 でないケース・対象ユーザーが存在しないケース・条件を満たすケースを検証する。
- [ ] 統合テスト本体は、上記の純粋関数を呼ぶ薄い `t.Skip(reason)` ラッパー経由でのみスキップ判定を行う（判定ロジック自体をテスト内に直接書かない）。
- [ ] 対象ユーザーの入手方法を明記する: 環境変数（例: `TEST_RUNAS_TARGET_USER`）で指定する既存フィクスチャユーザーを前提とし、テスト側でユーザー・グループの作成/削除は行わない（作成が必要な場合は本ステップで作成・削除タスクを追加する）。
- [ ] root/setuid 環境が必要な統合テストを、CI では skip 可能なタグ付きテストとして追加する。対象ユーザーで `id -G` 相当を実行し、補助グループが対象ユーザーのものと一致し root の補助グループ（例: `docker`）を含まないことを確認する（`02_architecture.md` §7.2）。

**成功基準**:
- `go test ./internal/runner/base/executor/...` で `canRunPrivilegedIntegrationTest` のユニットテストがパスする（ビルドタグ不要）。
- `go test -tags integration ./internal/runner/base/executor/...` で、環境がある場合のみ統合テスト本体が手動実行で成功することを確認する（CI では実行時スキップされることを確認）。

### PR-5 作成ポイント: Phase 1 comprehensive tests and integration harness

**対象ステップ**: 1-8 / 1-9

**推奨タイトル**: `test(0146): add SysProcAttr.Credential coverage and privileged integration harness`

**レビュー観点**: AC-01〜AC-07 の全項目がテストで検証されているか / `nil`/空スライスの偽陰性防止（`cmpopts.EquateEmpty()`）が適用されているか / 統合テストのスキップ判定がビルドタグと実行時判定の2段階に正しく分離されているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 2: F-003 — 値ベースの機微情報検出・マスク

#### Step 2-1: `ValueDetector` を新設する

**対象ファイル**: 新規 `internal/redaction/value_detector.go`, 新規 `internal/redaction/value_detector_test.go`

- [ ] 少なくとも次のフォーマットを検出するパターンを実装する（AC-08）: AWS（`AKIA`/`ASIA` 等）、GitHub（`ghp_`/`gho_`/`ghs_` 等）、Slack（`xox` 系）、GCP サービスアカウント、PEM/private key ブロック（`-----BEGIN … PRIVATE KEY-----`）、`Bearer <token>`、URL 埋め込み credential（`scheme://user:pass@host`）。コンパイル済み正規表現パターンは、繰り返しのコンパイル・アロケーションを避けるためパッケージレベル変数としてキャッシュすること。
- [ ] `ValueDetector` に検出対象値をマスクする関数（例: `Mask(text string) string`、既存 `Placeholder` 定数を再利用）を実装する。
- [ ] 高エントロピー文字列ヒューリスティックは実装しない（要件スコープ外、`02_architecture.md` §3.3.2）。

**成功基準**: 各フォーマットの正例・負例を含むテーブル駆動テストがパスする。

#### Step 2-2: `ValueDetector` を `Config.RedactText` へ統合する

**対象ファイル**: `internal/redaction/redactor.go`, `internal/redaction/redactor_test.go`

- [ ] §1.3 F-003 節の調査結論（「`RedactText` 内部に統合する」）に従い、以下の変更を行う。
- [ ] `Config` 構造体（16-25行目）に `ValueDetector` フィールドを追加する。
- [ ] `RedactText`（55-68行目）で、既存のキー=値パターン処理に加え `ValueDetector` による値ベース検出・マスクを適用する。**Layer 1（`SanitizeOutputForLogging`）は `RedactText` のみを呼ぶため、ここへの統合が両層一括適用の唯一の経路である**（既存コード調査結果 F-003 節参照）。
- [ ] `RedactLogAttribute`/`redactLogAttributeWithContext` が既に呼んでいる `IsSensitiveValue` との重複適用を確認し、二重マスキングで問題が生じないことを確認する（`RedactText` を経由後に再度 `IsSensitiveValue` を呼ぶ現在のフローとの整合）。
- [ ] `KindGroup` の再帰処理（`redactLogAttributeWithContext`）内でも `RedactText` 経由でネストグループ内の値がマスクされることを確認する。

**成功基準**: `RedactText` 経由で両層（Layer 1/Layer 2）がマスクすること、ネストグループ内の値も再帰的にマスクされることを検証するテストがパスする（AC-09）。

#### Step 2-3: 既知フォーマット追加パターンを `sensitive_patterns.go` に追加する（必要な場合）

**対象ファイル**: `internal/redaction/sensitive_patterns.go`, `internal/redaction/sensitive_patterns_test.go`

- [ ] Step 2-1 で `ValueDetector` に実装したパターンのうち、既存の `SensitivePatterns` 構造体・関数群と役割が重複するものがないか確認し、重複があれば一本化する（DRY、既存キー名パターンは維持）。

**成功基準**: 既存 `sensitive_patterns_test.go` が回帰しない。

#### Step 2-4: Slack 経路・`--show-sensitive` の確認テストを追加する

**対象ファイル**: `internal/redaction/redactor_test.go` または `internal/logging/slack_handler_test.go`（実装時に既存ファイル構成を確認して配置決定）

- [ ] Slack 送信ペイロードが `RedactingHandler` 経由でマスク済みになることを確認する統合的なテストを追加・確認する（AC-10。既存 `TestRedactingHandler_CommandResults_Integration` 等が対象範囲を含むか確認し、含まない場合は追加する）。
- [ ] `--show-sensitive` 非指定時の既定マスク（AC-11）が `ValueDetector` 追加後も維持されることを確認するテストを追加する。

**成功基準**: AC-09/AC-10/AC-11 の該当テストがパスする。

#### Step 2-5: ユーザー向けドキュメントを更新する（AC-12）

**対象ファイル**: `docs/user/security-risk-assessment.md`, `docs/user/security-risk-assessment.ja.md`（他、Slack 通知設定に関する既存ドキュメントがあれば実装時に特定して追加）

- [ ] 「Slack にコマンド出力を載せる設定は避けるべき」旨を明記する。
- [ ] 値ベースマスキングの適用範囲（コマンド引数・stdout/stderr・環境変数値、Slack 含む全出力先）と限界（未知フォーマット・高エントロピー文字列は取りこぼし得る、`02_architecture.md` §5.3 の残余リスク）を明記する。

**成功基準**: 後述 AC-12 の static 検証がパスする。

### PR-6 作成ポイント: value-based sensitive data detection and masking

**対象ステップ**: 2-1 / 2-2 / 2-3 / 2-4 / 2-5

**推奨タイトル**: `feat(0146): detect and mask sensitive values without key-name context`

**レビュー観点**: 正規表現のパッケージレベルキャッシュ（コンパイルコストの回避） / `RedactText` 統合が Layer 1（`SanitizeOutputForLogging`）と Layer 2（`RedactingHandler`）双方に一括適用されるか / `IsSensitiveValue` との二重マスキングで問題が生じないか / ドキュメント記載の適用範囲・限界が実装と一致しているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 3a: F-005 — record のディレクトリ権限違反フェイルクローズド化

（`02_architecture.md` §8 の Phase 3 は F-005/F-004 を単一フェーズとして扱う。本書では両者に相互依存がないため Step 番号は独立させつつ、Phase ラベルは `02_architecture.md` の Phase 3 に合わせて「3a」「3b」のサブラベルとする。`02_architecture.md` のフェーズ境界・優先順位を変更するものではない。）

#### Step 3-1: TOCTOU 違反検出時に fail-closed する

**対象ファイル**: `cmd/record/main.go`, `cmd/record/main_test.go`

- [ ] `RunTOCTOUPermissionCheck` の戻り値（126行目、現状破棄）を評価し、1件以上の違反があれば違反ディレクトリと具体的な権限問題（是正方法を含む）を ERROR ログに出力する。祖先ディレクトリを辿る TOCTOU 権限チェックでは、非絶対パスの起点（相対パス）を拒否またはスキップし、`.` での早期停止や相対パスの安全ゾーン誤判定を防止すること。
- [ ] 違反検出時は `processFiles`（233行目、ハッシュ生成ループ）を呼ばずに非ゼロ終了する early return を追加する。`mkdirAll`（`parseArgs` 内、212行目）が TOCTOU チェックより前に実行される現状の順序を踏まえ、fail-closed の early return を `processFiles` 呼び出し前に配置する（`02_architecture.md` §3.5「処理順序」）。
- [ ] 権限違反を無視して続行するバイパスフラグ（`--allow-insecure-perms` 等）は追加しない（AC-18）。
- [ ] 既存 `--force` フラグの意味（既存ハッシュファイルの上書き専用）を変更しない。権限違反バイパスとして機能しないことをコード上維持する。

**成功基準**: `TestRunTOCTOU_ContinuesOnWorldWritableDir`（`main_test.go:179-201`）のアサーションを反転させ（非ゼロ終了・ハッシュ未生成を検証）、パスする。

#### Step 3-2: ハッシュディレクトリ作成権限を `0o700` に変更する

**対象ファイル**: `cmd/record/main.go`

- [ ] `hashDirPermissions` 定数（27行目、現状 `0o750`）を `0o700` に変更する。

**成功基準**: 新規作成ディレクトリの権限が `0o700` であることを検証するテストがパスする（AC-17）。

#### Step 3-3: 単体テストを追加・更新する

**対象ファイル**: `cmd/record/main_test.go`

- [ ] `mkdirAll` と TOCTOU チェッカを注入し、違反ありで ERROR ログ・非ゼロ終了・ハッシュ未生成（バイパス手段なし）を検証するテストを追加する（AC-16）。
- [ ] 違反なしでハッシュ生成が継続することを検証するテストを確認する。既存の正常系テスト（`TestProcessFiles_MultipleFiles` 等、`main_test.go`）が世界書込み不可なディレクトリでの生成継続を明示的にアサートしていない場合は、本ステップで新規テストを追加する（「代替可能か確認」で終わらせず、カバーされていなければ必ず追加する）。
- [ ] `--force`（上書き）指定が権限違反バイパスとして機能しないことを検証するテストを追加する（AC-18）。
- [ ] 新規作成時の権限 `0o700` を検証するテストを追加する（AC-17）。

**成功基準**: 上記テストが全てパスし、`go test -tags test ./cmd/record/...` がグリーン。

#### Step 3-4: ユーザー向けドキュメントを更新する（AC-18/AC-19）

**対象ファイル**: `docs/user/record_command.md`, `docs/user/record_command.ja.md`

- [ ] 権限違反を意図的に無視して続行するバイパス手段を設けないこと、既存 `--force` が「既存ハッシュファイルの上書き」専用でありバイパスの意味を持たないことを明記する。
- [ ] 「record は信頼できる管理者権限・クリーンな環境で実行すること」を明記する。
- [ ] 既存 `0o750` 配備からのアップグレード時、ディレクトリ権限が自動的には是正されないこと（`os.MkdirAll` は既存ディレクトリの mode を変更しない）と、手動 `chmod 0700` による是正手順を明記する（`02_architecture.md` §3.5「アップグレード時の挙動」）。

**成功基準**: 後述 AC-18/AC-19 の static 検証がパスする。

### PR-7 作成ポイント: record fail-closed permission enforcement

**対象ステップ**: 3-1 / 3-2 / 3-3 / 3-4

**推奨タイトル**: `fix(0146): fail-closed on TOCTOU permission violations in record`

**レビュー観点**: `TestRunTOCTOU_ContinuesOnWorldWritableDir` の反転が意図どおり非ゼロ終了・ハッシュ未生成を検証しているか / 相対パス起点での祖先ディレクトリ探索が安全ゾーンを誤判定しないか / `--force` が権限バイパスとして機能しないことの検証 / 既存 `0o750` 配備からのアップグレード時の非自動是正がドキュメント化されているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 3b: F-004 — dry-run 未検証内容の明示区別と hard fail

#### Step 4-1: `ResultCollector` に `UsedUnverifiedContent` を追加する

**対象ファイル**: `internal/verification/result_collector.go`, `internal/verification/types.go`, `internal/verification/result_collector_test.go`

- [ ] `ResultCollector`（23-30行目）または `FileVerificationFailure`（`types.go` 127-133行目）に、未検証内容を採用したかを表すフラグと理由区分（`skipped_no_validator` / `verify_failed_<reason>`）を追加する。既存 `FailureReason` 列挙は変更しない（意味が異なるため）。
- [ ] 新しいフラグ・理由区分を記録する `RecordUnverifiedContent`（仮称）等のメソッドを追加する。

**成功基準**: 新規フィールド・メソッドを検証する単体テストがパスする。

#### Step 4-2: 経路1（検証器 nil）に記録呼び出しを追加する

**対象ファイル**: `internal/verification/manager.go`, `internal/verification/manager_test.go`

- [ ] `readAndVerifyFileWithReadFallback`（422-450行目）の423-427行目（`fileValidator == nil` 経路）に、Step 4-1 のフラグを `skipped_no_validator` として記録する呼び出しを追加する（現状この経路は `ResultCollector` に一切記録しない）。

**成功基準**: 経路1でも `UsedUnverifiedContent` が記録されることを検証する新規テストがパスする（AC-13）。

#### Step 4-3: 経路2（検証失敗）の記録に理由区分を追加する

**対象ファイル**: `internal/verification/manager.go`, `internal/verification/manager_test.go`

- [ ] `readAndVerifyFileWithReadFallback` の432-447行目（dry-run 検証失敗経路）で、既存の `RecordFailure` 呼び出しに加え、Step 4-1 のフラグを `verify_failed_<reason>`（`reason` は既存 `FailureReason`）として記録する。
- [ ] `TestReadAndVerifyFileWithReadFallback_DryRunLogging`（991行目）を拡張し、`UsedUnverifiedContent` とその理由区分が正しく設定されることを検証する（既存の `assert.NoError` によるコンテンツ返却の検証は維持する）。

**成功基準**: AC-13 の該当テストがパスする。

#### Step 4-4: env ファイル経路の扱いを確認する

**対象ファイル**: 実装時に特定（`internal/runner/config` の `SafeReadFile` 呼び出し箇所等）

- [ ] `VerifyEnvironmentFile`（`manager.go:92-124`）以外で env ファイルの実内容を読み込む箇所（`config` ローダ）が、同種のフォールバック（検証なしで読み込む経路）を持つかを確認する。
- [ ] 持つ場合は、Step 4-1〜4-3 と同じ `UsedUnverifiedContent` 記録を適用する。持たない場合は、その旨を本ステップの完了メモとして記録する（`02_architecture.md` §3.4.1 の実装時確認事項）。

**成功基準**: env 経路の扱いが確定し、必要な記録処理が実装されているか、不要である理由が明確であること。

### PR-8 作成ポイント: unverified content tracking in verification results

**対象ステップ**: 4-1 / 4-2 / 4-3 / 4-4

**推奨タイトル**: `feat(0146): record unverified content usage in verification results`

**レビュー観点**: 経路1（`fileValidator == nil`）が従来記録していなかった箇所への記録追加漏れがないか / 既存 `FailureReason` 列挙の意味を変えていないか / `TestReadAndVerifyFileWithReadFallback_DryRunLogging` の既存アサーション（`assert.NoError`）が維持されているか / env ファイル経路の要否判断が明確に記録されているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### Step 4-5: dry-run 出力（text/json）に UNVERIFIED 表示を追加する

**対象ファイル**: `internal/runner/resource/formatter.go`（`TextFormatter.writeFileVerification` 134-167行目、`JSONFormatter.FormatResult` 323行目付近）

- [ ] text 形式で、UNVERIFIED として採用されたファイルを検証済みと区別して表示し、理由区分（検証スキップ／検証失敗）を併記する。
- [ ] json 形式でも同様の情報を出力する。
- [ ] ハッシュ不一致（`ReasonHashMismatch`）は `security_risk: high` として強調表示する（既存 `getSecurityRisk`、`result_collector.go:145-156` の分類を利用）。

**成功基準**: text/json 双方の出力に UNVERIFIED マーカーが含まれることを検証するテストがパスする（AC-13）。

#### Step 4-6: `--dry-run-fail-unverified` の対象を未検証成果物全般へ拡張し、終了コードを分離する

**対象ファイル**: `internal/runner/resource/dryrun_manager.go`, `internal/runner/resource/types.go`, `internal/runner/resource/security_test.go`

- [ ] `previewExitCodeLocked()`（421-439行目）に、未検証内容（経路1/経路2）の採用有無を判定する分岐を追加する。
- [ ] 環境起因（検証不能・スキップ）は既存 `DryRunExitVerificationUnavailable`（= 3）を再利用する。
- [ ] 改ざん兆候（検証失敗、ハッシュ不一致等）は既存の policy-deny 終了コード（`DryRunExitPolicyDeny`）を再利用し、`DryRunExitVerificationUnavailable` に埋没させない（`02_architecture.md` §3.4.3 の対応表）。
- [ ] `FailOnVerificationUnavailable`（`types.go:97`）の doc コメントを、対象拡張後の意味に更新する。
- [ ] 既定（フラグなし）では dry-run が継続し終了コード0であることを回帰確認する（AC-15）。
- [ ] `02_architecture.md` §3.4.3 は回帰対象として「`cmd/runner` の該当フラグ挙動テスト」を挙げているが、実装時点で `cmd/runner/*_test.go` に `--dry-run-fail-unverified`/`FailOnVerificationUnavailable`/`DryRunExitVerificationUnavailable` を参照するテストは存在しない（`rg` で確認済み）。該当テストが存在しないことを実装時に再確認し、存在しなければ本ステップの対象外とする（存在する場合のみ追加で更新する）。

**成功基準**: `security_test.go` の `TestDryRun_VerificationUnavailableExitCode`（151-176行目）を拡張し、未検証成果物全般（経由1/経路2）を対象にした終了コード分岐を検証する。既存の `TestDryRun_AnalysisUnavailableDenyPreview`・`TestDryRun_DenyVsHardError` が回帰しないことを確認する。

#### Step 4-7: ドキュメントを更新する（AC-14）

**対象ファイル**: `docs/user/runner_command.md`, `docs/user/runner_command.ja.md`

- [ ] `--dry-run-fail-unverified` の既定挙動（未指定時は継続・終了コード0）と、CI 用途での推奨運用（未検証を hard fail にする用途での使用）をドキュメント化する。
- [ ] 環境起因（検証不能）と改ざん兆候（検証失敗）で終了コードが異なることを明記する。

**成功基準**: 後述 AC-14 の static 検証がパスする。

### PR-9 作成ポイント: dry-run UNVERIFIED display and exit code separation

**対象ステップ**: 4-5 / 4-6 / 4-7

**推奨タイトル**: `feat(0146): distinguish unverified dry-run content and split exit codes`

**レビュー観点**: 環境起因（`DryRunExitVerificationUnavailable`）と改ざん兆候（`DryRunExitPolicyDeny`）の終了コードが混同されていないか / 既定（フラグなし）で dry-run が終了コード0のまま継続する回帰がないか（AC-15） / text/json 双方の出力形式で UNVERIFIED 表示が一致しているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 4: F-006 / F-007 — 前提・限界のドキュメント化

`02_architecture.md` §8 の Phase 4 と同一（Step 番号は独立して 5-x を用いる）。Phase 0〜3b の後に実施する。

#### Step 5-1: F-006 のファイルサイズ上限をドキュメント化する

**対象ファイル**: `docs/user/security-risk-assessment.md`, `docs/user/security-risk-assessment.ja.md`（他、実装時に適切な既存セクションを特定）

- [ ] `safefileio.MaxFileSize`（128MB）とその根拠（メモリ枯渇対策）、大容量バイナリを検証・解析できない可用性上の制約を明記する（AC-20）。
- [ ] `filevalidator` の内部 `maxFileSize`（1GB、バイナリ解析専用）が `safefileio.MaxFileSize` とは別の制限であることを明確に区別して記述する（既存コード調査結果参照。両者を混同しない）。
- [ ] 閾値の設定可能化／ハッシュ・解析の上限分離について、本タスクでは実装しないと結論した理由（`02_architecture.md` §3.6 の(1)〜(3)）を記録する（AC-21。設計文書への記録は `02_architecture.md` に既に存在するため、ユーザードキュメントには結論の要約を記載する）。

**成功基準**: AC-20/AC-21 の static 検証がパスする。

#### Step 5-2: F-007 の non-Linux TOCTOU 残余ウィンドウをドキュメント化する

**対象ファイル**: `docs/user/security-risk-assessment.md`, `docs/user/security-risk-assessment.ja.md`

- [ ] 本番ターゲットが Linux + openat2 前提であることを明記する（既存の openat2 記述セクションを拡張する）。
- [ ] non-Linux（openat2 非対応）環境には `safeOpenFileFallback` の二段階チェックでも原理的に残る極短の TOCTOU 競合ウィンドウが存在することを明記する。
- [ ] macOS 等を開発・限定用途に限る運用ガイドを明記する。

**成功基準**: AC-22 の static 検証がパスする。

#### Step 5-3: 翻訳グロッサリを更新する

**対象ファイル**: `docs/translation_glossary.md`

- [ ] 「補助グループ」→「supplementary group」を追加する。
- [ ] 「saved-set-uid」（英語表記のまま、または訳語）のエントリを追加する。

**成功基準**: `rg -n "補助グループ|saved-set-uid" docs/translation_glossary.md` で両エントリがヒットする。

### PR-10 作成ポイント: F-006/F-007 documentation of assumptions and limitations

**対象ステップ**: 5-1 / 5-2 / 5-3

**推奨タイトル**: `docs(0146): document file size limits and non-Linux TOCTOU residual risk`

**レビュー観点**: 128MB（`safefileio.MaxFileSize`）と1GB（`filevalidator` 非公開 `maxFileSize`）の記述が混同されず区別されているか / non-Linux 環境の残余 TOCTOU ウィンドウの説明が `safeOpenFileFallback` の実装と整合しているか / 翻訳グロッサリの新規エントリが日英両方の文書に反映されているか

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

`02_architecture.md` §8 の優先順位に従い、次の順序で実装する。各マイルストーンはグリーンゲート（`make test && make lint`）達成をもって完了とする。

| マイルストーン | 対応フェーズ | 成果物 |
|---|---|---|
| M0 | Phase 0 | PoC 結果が `02_architecture.md` §3.1.3 に記録され、Phase 1 の実装方針が確定 |
| M1 | Phase 1 | `SysProcAttr.Credential` による run-as 実行。F-001/F-002（AC-01〜AC-07）実装完了 |
| M2 | Phase 2 | `ValueDetector` 追加・`RedactText` 統合。F-003（AC-08〜AC-12）実装完了 |
| M3a | Phase 3a | record の fail-closed 化・`0o700`。F-005（AC-16〜AC-19）実装完了 |
| M3b | Phase 3b | dry-run UNVERIFIED 明示・hard fail 終了コード分離。F-004（AC-13〜AC-15）実装完了 |
| M4 | Phase 4 | F-006/F-007 ドキュメント化（AC-20〜AC-22）完了 |

M1（F-001/F-002）が最優先である理由は `01_requirements.md` §4 の推奨順序（setuid-root 配備での実害が最大）に基づく。M3a（F-005）と M3b（F-004）は `02_architecture.md` の Phase 3 で同一フェーズとして扱われている（相互依存がないため本書では Step 番号・マイルストーンを 3a/3b に分けて独立追跡するが、`02_architecture.md` のフェーズ境界は変更しない）。

上記のマイルストーン表（M0〜M4）は、進捗を機能単位で大まかに説明するための表であり、「このPRがマージ済みか」を正確に判定する実務上の完了管理には用いない。実際にどこまで完了したかは §3.2 の PR 構成表・§6 のチェックリストを正とする。例えば M1 は PR-2〜PR-5 の4 PR にまたがるため、PR-2〜4 がマージ済みでも PR-5（テスト）が未マージなら「M1 完了」とは扱わない。M1 の完了は PR-5 のマージをもって確定する。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 |
|---|---|---|
| PR-1 | 0-1 / 0-2 | PoC 実行・結果を `02_architecture.md` §3.1.3 に記録し採用方式を確定 |
| PR-2 | 1-1 / 1-2 / 1-3 | run-as identity 解決を `risktypes` へ移設、suid/sgid 捕捉、`executionContext` 不変条件検証に組み込み |
| PR-3 | 1-4 | `OperationUserGroupExecution` を「root 昇格のみ」に変更（親プロセスの identity 変更を廃止） |
| PR-4 | 1-5 / 1-6 / 1-7 | `executor` で `SysProcAttr.Credential` を生成、fd-bound/staging fallback の権限整合、テスト注入インタフェース整備 |
| PR-5 | 1-8 / 1-9 | F-001/F-002 の単体テスト網羅、root/setuid 環境向け統合テストのスキップ判定基盤 |
| PR-6 | 2-1 / 2-2 / 2-3 / 2-4 / 2-5 | `ValueDetector` 新設、`RedactText` への統合、Slack 経路確認、ユーザードキュメント更新 |
| PR-7 | 3-1 / 3-2 / 3-3 / 3-4 | record の TOCTOU 違反検出時 fail-closed 化、ハッシュディレクトリ権限 `0o700`、ドキュメント更新 |
| PR-8 | 4-1 / 4-2 / 4-3 / 4-4 | `ResultCollector` に `UsedUnverifiedContent` 追加、経路1/経路2への記録組み込み、env 経路の扱い確定 |
| PR-9 | 4-5 / 4-6 / 4-7 | dry-run 出力の UNVERIFIED 表示、hard fail 終了コード分離、ドキュメント更新 |
| PR-10 | 5-1 / 5-2 / 5-3 | F-006/F-007 のファイルサイズ上限・non-Linux TOCTOU 残余ウィンドウのドキュメント化、翻訳グロッサリ更新 |

---

## 4. テスト戦略

### 4.1 単体テスト

`02_architecture.md` §7.1 の方針に従う。詳細は各 Step の成功基準を参照。権限系（F-001/F-002）は既存の `syscallSeteuid`/`syscallSetegid`/`identityChecker` と同方式の注入可能インタフェースで環境非依存のユニットテストを実現する（Step 1-7）。

### 4.2 統合テスト

run-as の実 uid/gid/補助グループ検証は root/setuid 環境が必要なため、CI では skip 可能なタグ付き統合テストとする（Step 1-9、`02_architecture.md` §7.2）。

### 4.3 既存テストへの影響（回帰確認が必要な箇所の一覧）

以下は挙動変更に伴い、新規追加ではなく既存アサーションの更新が必要なテストである。

| テスト | ファイル | 変更内容 |
|---|---|---|
| `TestRunTOCTOU_ContinuesOnWorldWritableDir` | `cmd/record/main_test.go:179-201` | 現状「exit 0」を「非ゼロ終了・ハッシュ未生成」に反転 |
| `TestReadAndVerifyFileWithReadFallback_DryRunLogging` | `internal/verification/manager_test.go:991` | `UsedUnverifiedContent` の検証を追加（既存アサーションは維持） |
| `TestChangeUserGroupInternal_*` 系 | `internal/runner/base/privilege/unix_privilege_test.go` | `OperationUserGroupExecution` 経路での呼び出し条件変更に合わせて更新 |
| `TestRestoreUserGroupInternal` | `internal/runner/base/privilege/unix_privilege_test.go` | 同上 |
| `TestDryRun_VerificationUnavailableExitCode` | `internal/runner/resource/security_test.go:151-176` | 未検証成果物全般を対象にした分岐を追加 |
| `TestExecute_FdBoundOrStaging` 等 | `internal/runner/base/executor/executor_fdexec_test.go` | `Credential` 設定との組み合わせで再検証 |

### 4.4 バックワード互換性テスト

native root 実行・`run_as` 未使用の経路、正常系 dry-run 出力について、Phase 1/Phase 3b の変更後も既存挙動が回帰しないことを Step 1-8（AC-04）・Step 4-6（AC-15）で検証する。

---

## 5. リスク管理

### 5.1 技術リスク

| リスク | 影響 | 緩和策 |
|---|---|---|
| Phase 0 の PoC で fd-bound 実行が `Credential` と非互換と判明する | Phase 1 のスケジュールに影響し、代替実装（`execveat`/`PR_SET_DUMPABLE`/staging）の追加実装が必要になる | Phase 0 を Phase 1 の先頭に配置し、影響範囲を早期に確定する（Step 0-2） |
| `performElevation`/`WithPrivileges` の `needsUserGroupChange` 分岐の現状挙動が調査で完全に特定できていない | Step 1-4 の変更範囲を誤り、`OperationUserGroupDryRun` を回帰させる可能性 | Step 1-4 冒頭に精読タスクを明示し、既存テスト（`TestManager_WithPrivileges_UserGroup_*`）で回帰を検出する |

### 5.2 スケジュールリスク

Phase 0（PoC）は setuid-root または root 権限を持つ Linux 環境を要するため、実装者のローカル環境で完結しない可能性がある。Docker コンテナ等での代替を許容し、それでも困難な場合は staging fallback を暫定的な既定方式として採用し、Phase 1 完了後に PoC を追試する運用も許容する（ただし `02_architecture.md` §3.1.3 の記録更新は必須）。

staging fallback を暫定採用する場合の Phase 1 実装方針（Step 1-6 代替手順）:
1. fd-bound 実行（`/proc/self/fd/<n>` 経由の `execve`）をスキップし、全ケースで `stageFromFD` による staging fallback を使用する（`prepareExecCommand` の分岐を短絡するフラグを導入するか、暫定的に fd-bound 経路を無効化する）。
2. Step 1-6 の本来のタスク（fd-bound + `Credential` の相互作用検証、`stagedExecMode` の `0o555` への変更、ディレクトリ権限 `0o711` への変更）は、Phase 1 の他ステップと同時に実施する（staging fallback 経路の権限変更は fd-bound の成否に関わらず必要であるため）。
3. PoC 追試で fd-bound 実行が `Credential` と互換であると確認できた場合、後続タスク（本書の改訂時に Step 番号を振り直す）として fd-bound 経路を再有効化し、その時点で `Credential` との組み合わせテストを追加する。

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: 0-1 / 0-2）
- [ ] PR-2 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3）
- [ ] PR-3 マージ済み（対象ステップ: 1-4）
- [ ] PR-4 マージ済み（対象ステップ: 1-5 / 1-6 / 1-7）
- [ ] PR-5 マージ済み（対象ステップ: 1-8 / 1-9）
- [ ] PR-6 マージ済み（対象ステップ: 2-1 / 2-2 / 2-3 / 2-4 / 2-5）
- [ ] PR-7 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3 / 3-4）
- [ ] PR-8 マージ済み（対象ステップ: 4-1 / 4-2 / 4-3 / 4-4）
- [ ] PR-9 マージ済み（対象ステップ: 4-5 / 4-6 / 4-7）
- [ ] PR-10 マージ済み（対象ステップ: 5-1 / 5-2 / 5-3）
- [ ] 全 PR で `make fmt && make test && make lint` がグリーン
- [ ] 本書「7. 受け入れ基準の検証」の全 AC が検証済み

### 6.1 成功基準（総合）

`requirements_process.md` が求める "Success Criteria"（機能完全性・品質指標・セキュリティ検証・ドキュメント完全性）を、本書では上記チェックリストと §7 の AC 検証表が合わせて満たす。総合的な達成条件は次のとおり。

- **機能完全性**: AC-01〜AC-22 が全て §7 の検証（`test`/`static`/`manual`）を通過している。
- **品質**: 全 Phase で `make fmt && make test && make lint` がグリーンであり、§4.3 に列挙した既存テストの期待値更新が反映されている。
- **セキュリティ検証**: F-001/F-002（権限分離）・F-005（信頼の起点）に関わる AC-01〜AC-07・AC-16〜AC-19 の `test` 種別の検証が全てパスしている（フェイルクローズド原則の回帰がないことを含む）。
- **ドキュメント完全性**: AC-12・AC-14・AC-18・AC-19・AC-20・AC-21・AC-22 の `static`+`manual` 検証が全て完了している。

---

## 7. 受け入れ基準の検証

各 AC について、検証種別（`test`: 自動テスト / `static`: 静的検証コマンド / `manual`: 手動確認）を付す。ドキュメント専用 AC（AC-12・AC-14 のドキュメント部分・AC-18 のドキュメント部分・AC-19・AC-20・AC-21・AC-22）の `static` 検証は文言の存在確認（`rg`）に留まり、記述内容が実装の実値と一致しているかまでは確認しない。したがって各該当 AC には `manual` 検証（最終差分の時点でドキュメント記載値を実装の定数・デフォルト値と突き合わせる確認）を併記し、`static` 検証のみで完了としない。

**AC-01**: run-as で起動されたコマンドの補助グループ集合が対象ユーザーの初期補助グループ集合と一致し、起動元プロセスの補助グループを1つも引き継がない。
- 種別: `test`
- テスト: `internal/runner/base/executor/executor_usergroup_test.go`（Step 1-8 で追加。user-only/group-only/both の3形態）

**AC-02**: 補助グループの決定・設定失敗時、コマンドを実行せずフェイルクローズする。
- 種別: `test`
- テスト: `internal/runner/base/executor/executor_usergroup_test.go`（Step 1-8）

**AC-03**: 補助グループ再設定が uid/gid 切替と整合し、検証可能な形で確認できる。
- 種別: `test`
- テスト: `internal/runner/base/executor/executor_usergroup_test.go`（注入可能インタフェース経由、Step 1-7/1-8）

**AC-04**: `run_as` 未使用経路（native root、dry-run）で補助グループ操作を行わず既存挙動を回帰させない。
- 種別: `test`
- テスト: Step 1-8 で追加する新規テスト。`OperationUserGroupDryRun` および `run_as` 未指定時に、注入した `syscallSeteuid`/`syscallSetegid`/`Credential` 生成モックが一切呼ばれないことを検証する（既存 `TestManager_WithPrivileges_UnsupportedPlatform` は `OperationHealthCheck` の非対応プラットフォームエラーを検証するテストであり本 AC とは無関係のため、回帰防止テストとしては使用しない）。

**AC-05**: run-as コマンド実行区間で親プロセスの saved-set-uid が0のまま子プロセスの実行が行われない。
- 種別: `test`
- テスト: `internal/runner/base/privilege/unix_privilege_test.go`（Step 1-8）。注入した `syscallSeteuid`/`syscallSetegid` モックが `OperationUserGroupExecution` 経路で root（uid/gid 0、昇格のみ）以外の値では呼ばれないこと（解決済みの対象 uid/gid では一度も呼ばれないこと）をモック呼び出し履歴で検証する。

**AC-06**: 復元後の不変条件チェックが suid も検証対象に含み、逸脱時に emergency shutdown する。
- 種別: `test`
- テスト: `internal/runner/base/privilege/unix_privilege_test.go`（Step 1-3, 1-8。setuid-root 想定での成功ケースと逸脱ケース双方）

**AC-07**: 既存の EUID==UID/EGID==GID 不変条件チェックと復元失敗時の即時停止が維持される。
- 種別: `test`
- テスト: `internal/runner/base/privilege/unix_privilege_test.go::TestRestorePrivilegesAndMetrics_IdentityLeakTriggersShutdown`（既存、Step 1-3 の変更後も回帰しないことを確認）

**AC-08**: 既知フォーマットの秘密値がキー名を伴わなくても値として検出・マスクされる。
- 種別: `test`
- テスト: `internal/redaction/value_detector_test.go`（Step 2-1。AWS/GitHub/Slack/GCP/PEM/Bearer/URL credential の正例・負例）

**AC-09**: 値ベース検出がコマンド引数・stdout/stderr・環境変数値、少なくとも Slack 通知へ載る内容に適用される。
- 種別: `test`
- テスト: `internal/redaction/redactor_test.go`（Step 2-2。`RedactText` 経由での Layer1/Layer2 双方の適用、ネストグループ内の値の再帰マスク）

**AC-10**: Slack 送信ペイロードが送信前に値ベースマスキングを必ず通す。
- 種別: `test`
- テスト: `internal/redaction/redactor_test.go` または `internal/logging/slack_handler_test.go`（Step 2-4）

**AC-11**: `--show-sensitive` 指定時のみ平文化、既定はマスク。
- 種別: `test`
- テスト: Step 2-4 で追加する既定マスク確認テスト

**AC-12**: Slack 出力回避の推奨と値ベースマスキングの適用範囲・限界がドキュメントに明記される。
- 種別: `static` + `manual`
- 検証コマンド: `rg -n "値ベース|value-based masking|Slack.*出力" docs/user/security-risk-assessment.ja.md docs/user/security-risk-assessment.md`
- 期待結果: Step 2-5 で追記した記述がヒットする
- 手動確認: 記載した適用範囲（コマンド引数・stdout/stderr・環境変数値・Slack）と限界（未知フォーマット・高エントロピー文字列の取りこぼし）が、Step 2-1〜2-2 で実装した `ValueDetector` の実際の対応フォーマット・適用箇所と一致することを確認する。

**AC-13**: dry-run で検証失敗しフォールバック読み込みした内容が UNVERIFIED として区別表示される。
- 種別: `test`
- テスト: `internal/verification/manager_test.go`（Step 4-2, 4-3。経路1・経路2 双方）+ `internal/runner/resource/formatter_test.go`（Step 4-5、実装時に既存ファイル名を確認）

**AC-14**: 未検証を hard fail にするオプションが提供され、既定挙動と CI 用途の推奨運用がドキュメント化される。
- 種別: `test` + `static`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_VerificationUnavailableExitCode`（Step 4-6）
- 検証コマンド: `rg -n "dry-run-fail-unverified" docs/user/runner_command.ja.md docs/user/runner_command.md`
- 期待結果: Step 4-7 で追記した既定挙動・CI 推奨運用の記述がヒットする
- 手動確認: ドキュメントに記載した終了コード（環境起因=3、改ざん兆候=policy-deny）が `internal/runner/resource/types.go` の `DryRunExitVerificationUnavailable`/`DryRunExitPolicyDeny` の実際の値と一致することを確認する。

**AC-15**: 正常系 dry-run 出力・非 dry-run 実行の挙動が回帰しない。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go`（Step 4-6 の既存ケース `TestDryRun_AnalysisUnavailableDenyPreview`/`TestDryRun_DenyVsHardError` が回帰しないことを確認）

**AC-16**: record 実行時、権限違反検出で無条件非ゼロ終了しハッシュを生成・保存しない。違反ディレクトリと権限問題を ERROR ログに出力する。
- 種別: `test`
- テスト: `cmd/record/main_test.go::TestRunTOCTOU_ContinuesOnWorldWritableDir`（Step 3-1 で反転）+ Step 3-3 の新規テスト

**AC-17**: ハッシュディレクトリが world/group-writable を許さない権限（`0o700` 相当）で作成される。
- 種別: `test`
- テスト: `cmd/record/main_test.go`（Step 3-3）

**AC-18**: 権限違反バイパス手段を設けない。既存 `--force` はバイパスの意味を持たないことをドキュメントに明記する。
- 種別: `test` + `static`
- テスト: `cmd/record/main_test.go`（Step 3-3、`--force` 指定でもバイパスされないことを検証）
- 検証コマンド: `rg -n "force" docs/user/record_command.ja.md docs/user/record_command.md`
- 期待結果: Step 3-4 で追記した「`--force` は上書き専用でありバイパスではない」旨の記述がヒットする
- 手動確認: `cmd/record/main.go` の `--force` 実装（`recordConfig.force` の使用箇所）を最終差分で読み、ドキュメント記載どおり上書き専用でありバイパス経路を持たないことを確認する。

**AC-19**: 「record は信頼できる管理者権限・クリーンな環境で実行すること」がドキュメントに明記される。
- 種別: `static`
- 検証コマンド: `rg -n "信頼できる管理者権限|クリーンな環境" docs/user/record_command.ja.md`
- 期待結果: Step 3-4 で追記した記述がヒットする

**AC-20**: 128MB のファイルサイズ上限とその根拠、可用性上の制約がドキュメントに明記される。
- 種別: `static` + `manual`
- 検証コマンド: `rg -n "128\s*MB|128MB" docs/user/security-risk-assessment.ja.md docs/user/security-risk-assessment.md`
- 期待結果: Step 5-1 で追記した記述がヒットする
- 手動確認: 記載する128MBが `internal/safefileio/safe_file.go` の `MaxFileSize` 定数と一致し、`filevalidator` の別の1GB上限（`internal/filevalidator/validator.go` の非公開 `maxFileSize`）と混同されずに区別して記述されていることを確認する（既存コード調査結果参照）。

**AC-21**: 閾値の設定可能化／上限分離の可否検討結果が設計文書に記録される。
- 種別: `static`
- 検証コマンド: `rg -n "本タスクでは実装せず" docs/tasks/0146_security_hardening/02_architecture.md`
- 期待結果: `02_architecture.md` §3.6 に既に記録済み（本ステップは実装計画としてはユーザードキュメントへの結論要約の追記のみ）。`rg -n "閾値の設定可能化|上限分離" docs/user/security-risk-assessment.ja.md` でユーザー向け要約もヒットすることを確認する

**AC-22**: 本番ターゲットが Linux + openat2 前提であり、non-Linux 環境の TOCTOU 残余ウィンドウと macOS 等の開発・限定用途がドキュメントに明記される。
- 種別: `static` + `manual`
- 検証コマンド: `rg -n "TOCTOU.*残余|残余.*TOCTOU|開発・限定用途" docs/user/security-risk-assessment.ja.md`
- 期待結果: Step 5-2 で追記した記述がヒットする
- 手動確認: 追記内容が `internal/safefileio/safe_file.go` の `safeOpenFileFallback` の実際の二段階チェック実装（既存の openat2 記述セクションが参照している実装）と整合していることを確認する。

---

## 8. 横断検索チェックリスト

`make lint`/`make test` では検出できない項目のみを対象とする（AC 検証表と重複する `rg` コマンドはここに含めない）。

- [ ] `resolveRunAsIdent`（小文字開始、移設前の名前）への残存参照がないことを確認する: `rg -n "resolveRunAsIdent" --type go` — 期待結果: ヒットなし（`risktypes.ResolveRunAsIdent` に統一されていること）。
- [ ] `internal/runner/base/risk/runas_identity.go`・`runas_identity_test.go` が削除され、残存参照がないことを確認する: `rg -n "risk/runas_identity" --type go`。
- [ ] `hashDirPermissions`・`0o750` の残存参照が `cmd/record/main.go` 以外にないことを確認する: `rg -n "0o750" cmd/record/`。
- [ ] `stagedExecMode`（`0o500` → `0o555` 変更後）の残存する `0o500` 参照がないことを確認する: `rg -n "0o500" internal/runner/base/executor/`。

---

## 9. 次のステップ

本計画書のレビューが完了し `approved` になった後、`runplan` の手順に従って Phase 0 から実装に着手する。各フェーズ完了時にグリーンゲート（`make test && make lint`）を確認し、本書「6. 実装チェックリスト」を更新する。
