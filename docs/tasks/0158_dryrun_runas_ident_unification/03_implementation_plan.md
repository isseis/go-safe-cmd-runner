# 実装計画書: dry-run のユーザー・グループ検証を実行時の識別情報解決に統合する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-28 |
| Review date | 2026-07-28 |
| Reviewer | isseis |
| Comments | - |

## 関連文書

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)
- 要件プロセス: [requirements_process.md](../../dev/developer_guide/requirements_process.md)
- テスト構成ガイド: [test_organization.md](../../dev/developer_guide/test_organization.md)

---

## 1. 実装の全体像

### 1.1 目的

dry-run のユーザー・グループ検証と実行時の識別情報解決を、`risktypes.ResolveRunAsIdentStrict` という 1 つの関数の呼び出しに統合する。あわせて、この委譲によってどこからも呼ばれなくなる dry-run 専用の特権経路を削除し、検証結果を構造化ログに出す。設計の詳細は [02_architecture.md](02_architecture.md) を参照する。本書は設計を繰り返さず、作業手順と検証方法のみを述べる。

### 1.2 実装方針

- 設計書 §8 の Phase 区分をそのまま実装の Phase とする。
- 各 Phase の完了時に `make fmt` → `make test` → `make lint` を実行し、すべて成功した状態でのみ次の Phase に進む。
- Go のコメント・識別子・文字列リテラルは、本番コードとテストコードのいずれも英語で書く。本書の説明文は日本語である。以降の各作業項目でコメント文の趣旨を日本語で示している箇所も、実際に書くのは英語の文である。
- 既存のテストヘルパを優先して使い、新規ヘルパは複数パッケージから使うものだけを `testutil/` に置く。

### 1.3 設計書との差分（承認が必要な 1 点）

設計書 §4.1 は、補助グループ列挙の失敗について「`ErrRunAsIdentityResolution`（包む対象のエラーなし）」を返すと定めている。本計画はこれを次のように変更する。

- **変更内容**: `ResolveRunAsIdentStrict` は、この場合に新しい番兵エラー `ErrRunAsSupplementaryGroupsUnavailable` を `ErrRunAsIdentityResolution` で包んで返す。
- **理由**: 設計書どおり「包まない番兵エラーそのもの」を返すと、dry-run 側が失敗種別（設計書 §4.2 の `failure_kind`）を判定する手段が `err == risktypes.ErrRunAsIdentityResolution` という直接比較しかなくなる。この比較は本リポジトリの `golangci-lint` 設定で有効な `err113` に抵触する（`.golangci.yml` の除外規則は `path: _test\.go` のみで、本番コードは除外されない）。したがって `make lint` が通らない。加えて、判定の根拠が「エラーが包まれていないこと」というコード上で見えにくい性質になるため、将来誰かがこのエラーを包むように変更した時点で、警告も失敗も出ないまま判定だけが誤るようになる。
- **影響範囲**: `errors.Is(err, ErrRunAsIdentityResolution)` が真である点は変わらないため、executor の fail-closed 判定と既存テストの期待は変わらない。増えるのは、`errors.Is` で補助グループ列挙失敗を名指しできることだけである。
- **必要な措置**: 本計画の承認時にこの差分を確認し、承認されたら設計書 §4.1 の該当行と §3.1 の契約記述を更新する（Phase 1 の作業項目に含める）。

### 1.4 既存コード調査結果

実装前に実際のコードを確認した結果を記す。行番号は 2026-07-28 時点のものであり、作業中は記号名で検索して位置を確認する。

**追加が必要なもの**

| 対象 | 現状 | 必要な作業 |
|---|---|---|
| `risktypes.ResolveRunAsIdentStrict` | 存在しない | 新規追加。`internal/runner/base/risktypes/runas_ident.go` に置く |
| `risktypes.RunAsResolver` | 存在しない。同一シグネチャが `executor.go:65`（無名関数型）と `risk/evaluator.go:71`（非公開型 `runAsResolver`）に個別に書かれている | 新規追加。executor と dry-run の 2 か所を新しい型に揃える。`risk` の非公開型は触らない（設計書 §3.1） |
| `DryRunResourceManager` の `runAsResolver` / `logger` フィールド | いずれも存在しない。同型は `slog` をパッケージレベルで直接呼んでいる（`dryrun_manager.go:338`、`906`） | 注入可能なフィールドとして追加。既定値はコンストラクタで設定する |
| リスクレベルを指定できるテスト用評価器 | `internal/runner/resource/test_helpers.go:38-45` の `permissiveTestEvaluator` は `Low` 固定で、リスクレベルを与えられない | `riskLevelTestEvaluator{level runnertypes.RiskLevel}` を同ファイルに追加する（同ファイルの build tag は `//go:build test || performance` であり、両タグでコンパイルできる必要がある） |
| dry-run の識別情報ガードテスト | `risktypes` と `resource` のどちらにも無い | 新規追加。`privilege/identity_mutation_guard_test.go` と `risk/live_identity_guard_test.go` が同種の AST 検査の実装例である |
| dry-run の副作用テスト（プロセス識別情報の不変） | `resource` パッケージに無い（`rg -l "Getgroups" internal/runner/resource/` は 0 件）。現在この不変条件を検査しているのは `privilege/unix_privilege_test.go::TestWithPrivileges_UserGroupDryRunDoesNotChangeIdentity` のみで、これは Phase 4 で削除される | Phase 3 で `resource` に先に用意し、Phase 4 で旧テストを削除する（削除が先行して無防備な中間状態を作らないため） |

**変更が必要なもの**

| 対象 | 現状 | 必要な作業 |
|---|---|---|
| `executor.ErrRunAsIdentityResolution`（`executor.go:37-42`） | `executor` パッケージで定義 | `risktypes` へ移し、`executor` の定義を削除。参照は `executor_usergroup_test.go:119` と `:146` の 2 か所 |
| `executor.executeWithUserGroup` の解決部（`executor.go:185-213`） | リゾルバの nil フォールバックと `Groups == nil` 判定を自前で持つ | `ResolveRunAsIdentStrict` の 1 回の呼び出しに置き換える |
| `executor.WithRunAsResolver`（`test_helpers.go`） | 引数が無名関数型 | 引数型を `risktypes.RunAsResolver` に変更。呼び出し側（`executor_usergroup_test.go` の 3 か所）は関数リテラルを渡しているため、型変更だけで通る |
| `DryRunResourceManager.analyzeCommand` の検証部（`dryrun_manager.go:269-297`） | `privilegeManager` 経由で `OperationUserGroupDryRun` を呼ぶ。失敗時は `SecurityRisk` を単純に `"high"` で上書き | `validateRunAsIdentity` に切り出し、`ResolveRunAsIdentStrict` へ委譲。リスクレベルの引き上げを単調にする |
| `privilege/unix.go` の dry-run 経路 | `resolveUserGroupForDryRun`（`:469-565`）、`buildUserGroupLogAttrs`（`:450-462`）、`prepareExecution` の `case`（`:152-153`）、`performElevation` の分岐（`:172-176`）、`restorePrivilegesAndMetrics` の metrics 分岐（`:224-225`）、doc コメント 3 か所（`:89-90`、`:215-217`、`:231-232`） | すべて削除・書き換え。削除後は `os/user` と `strconv` の import も不要になる（`os` と `syscall` は他で使うため残す） |
| `runnertypes.OperationUserGroupDryRun`（`config.go:162`） | 定義あり | 削除 |
| `privilege/testutil/mocks.go:39-41` | `case runnertypes.OperationUserGroupDryRun` と記録文字列 `"user_group_dry_run:"` | 削除 |

**`OperationUserGroupDryRun` / `user_group_dry_run` の全参照**

`rg -n "OperationUserGroupDryRun" --glob '!docs' .` は 29 件・7 ファイル、`rg -n "user_group_dry_run" --glob '!docs' .` は 4 件・3 ファイルである（`config.go` と `testutil/mocks.go` は両方の検索に該当する）。

| ファイル | `OperationUserGroupDryRun` | `user_group_dry_run` | 対応 |
|---|---|---|---|
| `internal/runner/resource/dryrun_manager.go` | 1（`:277`） | 0 | Phase 3 で委譲により消える |
| `internal/runner/base/runnertypes/config.go` | 1（`:162`） | 1（定数値） | Phase 4 で定義を削除 |
| `internal/runner/base/privilege/unix.go` | 6（コード 3 件: `:152`, `:172`, `:224` / コメント 3 件: `:90`, `:215`, `:231`） | 0 | Phase 4 で削除・書き換え |
| `internal/runner/base/privilege/testutil/mocks.go` | 1（`:39`） | 1（`:40` の記録文字列） | Phase 4 で `case` ごと削除 |
| `internal/runner/base/privilege/unix_privilege_test.go` | 12 | 0 | Phase 4 でテスト単位に整理（下表） |
| `internal/runner/base/privilege/unix_test.go` | 3 | 0 | Phase 4 でファイルごと削除 |
| `internal/runner/base/privilege/manager_test.go` | 5 | 0 | Phase 4 で整理 |
| `internal/runner/resource/usergroup_dryrun_test.go` | 0 | 2（`:45`, `:192` の期待文字列） | Phase 3 で書き換え |

**変更・削除の対象となる既存テスト**

| テスト | 現在の検査内容 | 対応 |
|---|---|---|
| `privilege/unix_privilege_test.go::TestPrepareExecution_Success` | 3 operation の `needsPrivilegeEscalation` を表駆動で確認 | `user_group_dryrun` ケースのみ削除。他の 2 ケースは残す |
| 同 `::TestPerformElevation_Success` | dry-run の 2 サブテストのみで構成 | 関数ごと削除。昇格成功経路は `TestEscalatePrivileges` が引き続き検査する |
| 同 `::TestPerformElevation_Failure` | `privilege_escalation_not_supported` と `invalid_user_in_dryrun` | `invalid_user_in_dryrun` サブテストのみ削除 |
| 同 `::TestWithPrivileges_UserGroupDryRunDoesNotChangeIdentity` | dry-run 前後で euid / egid / gid が変わらないこと | 削除。代替は Phase 3 で `resource` に用意済みとする |
| 同 `::TestHandleCleanupAndMetrics_Success` | dry-run で `ElevationSuccesses == 1` | 昇格を伴う構成に書き換え、かつ panic 復帰と duration 計上という `handleCleanupAndMetrics` 固有の振る舞いを検査する形にする |
| 同 `::TestHandleCleanupAndMetrics_WithError` | panic 時の再送出。operation は dry-run だが検査内容とは無関係 | operation を `OperationFileValidation` に差し替えるのみ |
| 同 `::TestRestorePrivilegesAndMetrics_Success` | dry-run で `ElevationSuccesses == 1` | 昇格を伴う構成に書き換え |
| 同 `::TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalationOrDryRun` | 昇格なし・非 dry-run で success が記録されないこと | 改名と本文の書き換え |
| 同 `::TestRestorePrivilegesAndMetrics_Failure` | panic 時に success が記録されないこと | 昇格ありの構成に書き換え |
| 同 `::TestResolveUserGroupForDryRun` | `resolveUserGroupForDryRun` の直接呼び出し（5 ケース） | 関数ごと削除 |
| 同 `::TestRestorePrivilegesAndMetrics_IdentityVerificationSkippedForDryRun` | dry-run では `identityVerifier` が呼ばれないこと | 削除せず、「昇格を伴わない場合は呼ばれない」検査として改名・書き換え |
| `privilege/unix_test.go`（ファイル全体） | `TestUnixPrivilegeManager_DryRunResolution` と `TestUnixPrivilegeManager_PrivilegeValidation` の 2 関数のみ。いずれも dry-run 専用 | ファイルごと削除（残す関数が無いため） |
| `privilege/manager_test.go::TestManager_WithPrivileges_UserGroup_ValidUser` | `dry_run_mode` と（root 時のみ）`actual_change` | `dry_run_mode` サブテストを削除。`actual_change` は残す |
| 同 `::TestManager_WithPrivileges_UserGroup_InvalidUser` | dry-run 経由の名前解決失敗 2 ケース | 関数ごと削除（同等の検査は `risktypes` と `resource` の新テストが担う） |
| 同 `::TestManager_WithPrivileges_UserGroup_EmptyUserGroup` | dry-run で空の user/group が成功すること | 関数ごと削除 |
| 同 `::TestManager_WithPrivileges_UserGroup_FunctionError` | dry-run でコールバックのエラーが伝播すること | **削除せず書き換える**。削除すると「`WithPrivileges` がコールバックのエラーをそのまま返す」という不変条件を検査するテストがパッケージから無くなる（`TestManager_WithPrivileges_UnsupportedPlatform` は `fn` に到達する前のエラーしか見ておらず、`race_test.go` のコールバックはすべて `nil` を返す） |
| `executor/executor_usergroup_test.go::TestExecuteWithUserGroup_ResolverError_FailsClosed` / `::TestExecuteWithUserGroup_ResolverNilGroups_FailsClosed` | `executor.ErrRunAsIdentityResolution` を参照 | 参照先を `risktypes.ErrRunAsIdentityResolution` に変更。検査内容は維持 |
| `resource/usergroup_dryrun_test.go::TestDryRunResourceManager_UserGroupValidation` | 6 サブテスト。うち 2 件が `ElevationCalls` に `user_group_dry_run:` が記録されることを前提とし（`:45`, `:192`）、1 件が `ElevationCalls` が空であることを前提とし（`:229`）、3 件が旧文言を期待する（`configuration validated` 2 件、`validation failed:` 1 件。`[WARNING: ...]` の 2 件は文字列が変わらない） | Phase 3 で全面的に書き換え |

**変更が不要な箇所**

- `risk.StandardEvaluator`（`evaluator.go:101` の `resolveRunAs` 注入）は設計書 §3.2 のとおり対象外。
- `ElevationContext.RunAsUser` / `RunAsGroup` は executor が監査ログ用に設定し続けるため、フィールドは削除しない（設計書 §3.5）。
- `resource` の `riskLevelHigh` 定数（`dryrun_manager.go:37`）は `:617` と `:678` でも使われており、削除しない。この 2 か所はいずれも新しく構築した `Analysis` 値への代入であり、`evaluateCommandRisk` が設定済みの値を上書きする経路ではない。したがって設計書 §3.4 の単調性の欠陥はこの 2 か所には存在せず、修正対象は `analyzeCommand` の検証部のみである。
- `executor/executor_test.go:39` 付近のコメント（注入リゾルバの契約を説明する記述）は、委譲後も事実と一致するため変更しない。

**外部前提の確認結果**

| 前提 | 確認方法 | 結果 |
|---|---|---|
| Linux + `CGO_ENABLED=1` で `u.GroupIds()` が補助グループを列挙できる | 最小プログラムを `CGO_ENABLED=1 go run` で実行 | 成功（`groupids=[1000] err=<nil>`） |
| Linux + `CGO_ENABLED=0`（リリース構成）で同上 | 同上を `CGO_ENABLED=0` で実行 | 成功（同じ結果） |
| macOS + `CGO_ENABLED=1` | 本環境（Linux コンテナ）では実施不能 | **未確認**。設計書 §5.5 の懸念は残る |
| 最小構成コンテナ（`/etc/group` が最小） | 本環境では実施不能 | **未確認** |
| CI で cgo 有無の両方が回ること | `Makefile` の `unit-test-cgo1` / `unit-test-cgo0` ターゲットを確認 | 両ターゲットが存在し、それぞれ `CGO_ENABLED=1` / `CGO_ENABLED=0` で `-tags test ./...` を実行する |
| `ParseRiskLevel` が `"critical"` を受け付けないこと | `runnertypes/config.go:93-110` を確認 | 受け付けない（`ErrInvalidRiskLevel` を返す）。したがってリスクレベルの比較には専用の変換関数が必要（Phase 3 で追加） |
| `err113` が本番コードにも適用されること | `.golangci.yml` の `linters.enable`（`:28`）と `exclusions`（`:134-141`）を確認 | 有効。除外は `path: _test\.go` のみで、本番コードと `testutil/` 配下の非 `_test.go` ファイルは除外されない（§1.3 の根拠） |
| `make deadcode` が失敗で終了するか | `make deadcode` を実行 | **失敗しない**。終了コード 0 で、既存の到達不能コードを 7 件報告する（`WithRiskEvaluator` / `FamilyOf` / `CommandRiskProfile.BaseRiskLevel` / `ResolveOperandPath` / `WithFileSystem` / `NewStandardELFAnalyzerWithSyscallStore` / `NewSyscallAnalyzerWithConfig`）。したがって「`make deadcode` が成功する」は完了条件として無意味であり、出力の内容で判定する |
| `command-risk-evaluation.{ja.,}md` の対象節が存在すること | `rg -n "^### "` で確認 | 日本語版 `:439`「拒否 / エラー / High 許可の区別（dry-run の失敗時挙動）」、英語版 `:439`「Deny vs Error vs High-allowable (dry-run failure handling)」 |
| Phase 3 で追加する `resource` パッケージ向け識別情報変更禁止ガード（許可リスト空）が、既存コードに未検出の識別情報変更呼び出しを抱えていないこと | `rg -n -e "Seteuid" -e "Setegid" -e "Setuid\(" -e "Setgid\(" -e "Setreuid" -e "Setregid" -e "Setresuid" -e "Setresgid" -e "Setgroups" -e "Setfsuid" -e "Setfsgid" -e "syscall\.Syscall" -e "Capset" -e "Prctl" internal/runner/resource/` を実行 | 0 件。ガード導入時点で `resource` パッケージには対象呼び出しが存在せず、ガードは無関係な理由でグリーンゲートを壊さない |

---

## 2. 実装ステップ

### Phase 1: `risktypes` に共有関数を追加する

**対象ファイル**

- `internal/runner/base/risktypes/runas_ident.go`（変更）
- `internal/runner/base/risktypes/runas_ident_strict_test.go`（新規、`package risktypes_test`、`//go:build test`）
- `docs/tasks/0158_dryrun_runas_ident_unification/02_architecture.md`（§1.3 の差分が承認された場合のみ変更）

**作業内容**

- [x] §1.3 の差分（補助グループ列挙失敗時に番兵エラーを包む）についてレビュアーの承認を得る。承認されたら設計書 §3.1 の契約記述と §4.1 の表（「包む対象のエラーなし」の行）を更新する。承認されない場合は Phase 1 の実装内容を再検討する。
- [x] `runas_ident.go` に型 `RunAsResolver` を追加する（設計書 §3.1 のシグネチャ）。
- [x] `runas_ident.go` に番兵エラーを 2 つ追加する。
  - `ErrRunAsIdentityResolution = errors.New("failed to resolve run-as identity (uid/gid/supplementary groups)")` — 文字列は `executor.go:42` の既存値をそのまま引き継ぐ。
  - `ErrRunAsSupplementaryGroupsUnavailable = errors.New("supplementary groups could not be enumerated")`
- [x] `runas_ident.go` に `ResolveRunAsIdentStrict` を追加する。契約は設計書 §3.1 のとおりで、分岐は次の 3 つとする。
  - `resolve == nil` のとき `ResolveRunAsIdent` を使う。
  - `resolve` がエラーを返したとき `fmt.Errorf("%w: %w", ErrRunAsIdentityResolution, err)` を返す。
  - 成功したが `Groups == nil` のとき `fmt.Errorf("%w: %w", ErrRunAsIdentityResolution, ErrRunAsSupplementaryGroupsUnavailable)` を返す。
  - いずれの失敗でも `errors.Is(err, ErrRunAsIdentityResolution)` が真になること、および補助グループ列挙失敗は `errors.Is(err, ErrRunAsSupplementaryGroupsUnavailable)` で名指しできることを doc コメントに書く。
- [x] `errors` を import に追加する。
- [x] 単体テストを追加する。既存の `runas_ident_test.go`（同じ `package risktypes_test`）にある `parseID` ヘルパと主グループ名が引けない場合の `t.Skip` パターンを再利用し、同名のヘルパを再定義しない。
  - [x] `TestResolveRunAsIdentStrict_ResolverError`: リゾルバがエラーを返すと `errors.Is(err, ErrRunAsIdentityResolution)` が真で、元のエラーも `errors.Is` で取り出せる。
  - [x] `TestResolveRunAsIdentStrict_NilGroups`: リゾルバが `Groups == nil` を返すと、`errors.Is(err, ErrRunAsIdentityResolution)` と `errors.Is(err, ErrRunAsSupplementaryGroupsUnavailable)` の双方が真になる。
  - [x] `TestResolveRunAsIdentStrict_Success`: リゾルバの返した `RunAsIdent` がそのまま返る。
  - [x] `TestResolveRunAsIdentStrict_NilResolverUsesDefault`: `resolve` に `nil` を渡すと既定リゾルバが使われる。現在のユーザー名を渡し、UID が `os.Getuid()` と一致することで確認する。
  - [x] `TestResolveRunAsIdentStrict_ArgumentForms`: 設計書 §6.3 の 3 行を表駆動で確認する。既定リゾルバと `OriginalExecutionIdentity()` を使い、(a) ユーザーのみ→ UID・GID がそのユーザーのもの、(b) ユーザーとグループ→ GID が指定グループのもの、(c) グループのみ→ UID は基準識別情報（`OriginalExecutionIdentity()` が返す、プロセス起動時の実 UID / 実 GID / 補助グループ）のまま・GID が指定グループのもの、を検査する。

**完了条件**

- `go test -tags test ./internal/runner/base/risktypes/...` が成功する。
- `make test` と `make lint` が成功する（この時点で `executor` 側にも同名の番兵エラーが残っているが、パッケージが異なるため衝突しない）。

### PR-1 作成ポイント: shared run-as identity resolution in risktypes

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0158): add ResolveRunAsIdentStrict to risktypes`

**レビュー観点**: 番兵エラーのラップ方針（§1.3 の設計差分）が `errors.Is` で意図通り判定できるか / `ResolveRunAsIdentStrict` の 3 分岐が設計書 §3.1 の契約と一致するか / 新規テスト 5 件が成功・各失敗種別・nil リゾルバの既定動作・引数形の 3 パターンを網羅しているか

**実装モデル要件**: standard

**判定理由**: 新規追加のみで既存呼び出し元を変更しないため既存動作への影響がなく、既存コード調査結果に競合する実装アプローチも記載されていない。パネルモードの各トリガー（重い統合テスト・CI・外部リソース面、セキュリティゲート/マイグレーション）にも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: `executor` を共有関数の呼び出しに置き換える

**対象ファイル**

- `internal/runner/base/executor/executor.go`（変更）
- `internal/runner/base/executor/test_helpers.go`（変更）
- `internal/runner/base/executor/executor_usergroup_test.go`（変更）

**作業内容**

- [x] `executor.go` の `ErrRunAsIdentityResolution` の定義と直前の doc コメント（`:37-42`）を削除する。
- [x] `DefaultExecutor.runAsResolver` フィールドの型を `risktypes.RunAsResolver` に変える。末尾のコメント `// injectable for testing; defaults to risktypes.ResolveRunAsIdent` はそのまま残す。
- [x] `executeWithUserGroup` の解決部（`resolver := e.runAsResolver` から `Groups == nil` の判定まで、現行 `:185-213`）を次の形に置き換える。
  - `ResolveRunAsIdentStrict(e.runAsResolver, risktypes.OriginalExecutionIdentity(), cmd.RunAsUser(), cmd.RunAsGroup())` を 1 回呼ぶ。
  - エラー時は `e.Logger.Error("Failed to resolve run-as identity", "error", err, "user", cmd.RunAsUser(), "group", cmd.RunAsGroup())` を出し、受け取ったエラーをそのまま返す（二重にラップしない）。
  - nil フォールバックと `Groups == nil` の判定は `ResolveRunAsIdentStrict` に移るため executor 側から削除する。それを説明していた既存コメント 2 件（`Fall back to the default resolver ...` と `ResolveRunAsIdent silently returns nil Groups ...`）も削除し、委譲先を指す短いコメントに置き換える。
  - ログメッセージ `"Failed to resolve run-as supplementary groups"` は無くなる（設計書 §8.1）。
- [x] `test_helpers.go` の `WithRunAsResolver` の引数型を `risktypes.RunAsResolver` に変える。doc コメントは維持する。
- [x] `executor_usergroup_test.go` の `executor.ErrRunAsIdentityResolution` 2 か所（`:119`, `:146`）を `risktypes.ErrRunAsIdentityResolution` に変える。

**完了条件**

- `rg -n "^var ErrRunAsIdentityResolution" internal/runner/base/executor/` の結果が 0 件（定義が残っていない）。
- `rg -n "executor\.ErrRunAsIdentityResolution" internal/` の結果が 0 件（参照が残っていない）。
- `make test` と `make lint` が成功する。

### PR-2 作成ポイント: executor delegates to the shared resolver

**対象ステップ**: Phase 2

**推奨タイトル**: `feat(0158): delegate executor run-as resolution to risktypes`

**レビュー観点**: `executeWithUserGroup` の新しい 1 回呼び出しが旧・自前ロジック（nil フォールバックと `Groups == nil` 判定）と同じ fail-closed 挙動を保っているか / 削除したログメッセージ（`"Failed to resolve run-as supplementary groups"`）の消失が設計書 §8.1 の想定どおりか / `executor.ErrRunAsIdentityResolution` の参照が全て `risktypes.ErrRunAsIdentityResolution` に置き換わっているか

**実装モデル要件**: standard

**判定理由**: 本 PR は実行時の特権実行経路（`executeWithUserGroup`）に触れるが、PR-1 で単体テスト済みの `ResolveRunAsIdentStrict` の 1 回呼び出しへ置き換えるだけで、executor 側に新しい分岐ロジックを持ち込まない。fail-closed 挙動の回帰は既存の `TestExecuteWithUserGroup_ResolverError_FailsClosed` / `..._ResolverNilGroups_FailsClosed` の参照先変更のみで検査できる。既存コード調査結果に競合アプローチの記載はなく、Conditional checks・パネルモードのいずれのトリガーにも該当しない。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: dry-run の検証を共有関数に委譲する

**対象ファイル**

- `internal/runner/resource/dryrun_manager.go`（変更）
- `internal/runner/resource/test_helpers.go`（変更）
- `internal/runner/resource/usergroup_dryrun_test.go`（変更）
- `internal/runner/resource/identity_mutation_guard_test.go`（新規、`package resource`、`//go:build test`）
- `internal/runner/base/risktypes/identity_mutation_guard_test.go`（新規、`package risktypes`、`//go:build test`）

**作業内容（実装）**

- [x] `DryRunResourceManager` に注入可能なフィールドを 2 つ追加する。
  - `runAsResolver risktypes.RunAsResolver`
  - `logger *slog.Logger`
- [x] `NewDryRunResourceManagerWithOutput` の構造体リテラルで既定値を設定する（`runAsResolver: risktypes.ResolveRunAsIdent`、`logger: slog.Default()`）。コンストラクタの引数は増やさない。`NewDryRunResourceManager` はこの関数に委譲しているため、生成経路はこの 1 か所で足りる。
- [x] 表示用のリスクレベル文字列を `runnertypes.RiskLevel` に戻す非公開関数 `parseDisplayRiskLevel(s string) runnertypes.RiskLevel` を追加する。`runnertypes` の文字列定数 5 種を `switch` で受け、該当しない値と空文字は `RiskLevelUnknown` を返す。`ParseRiskLevel` を使わない理由（`"critical"` を拒否するため内部比較に使えない）をコメントで添える。
- [x] リスクレベルを単調に引き上げる非公開関数 `raiseSecurityRisk(analysis *Analysis, level runnertypes.RiskLevel)` を追加する。現在値を `parseDisplayRiskLevel` で `RiskLevel` に直し、`max` で比較して高い方の `.String()` を `analysis.Impact.SecurityRisk` に設定する。
- [x] 失敗種別を判定する非公開関数 `runAsFailureKind(err error, userName string, base risktypes.RunAsIdent) string` を追加する。判定順序は次のとおりで、根拠をコメントで書く。判定はすべて `errors.Is` / `errors.AsType` で行い、エラー値の直接比較（`==`）は使わない。
  1. `errors.AsType[user.UnknownUserError](err)` が真 → `"user_unknown"`
  2. `errors.AsType[user.UnknownGroupError](err)` が真 → `"group_unknown"`
  3. `errors.Is(err, risktypes.ErrRunAsSupplementaryGroupsUnavailable)` が真 → `userName == "" && base.Groups == nil` のとき `"base_identity_groups_unavailable"`、それ以外は `"supplementary_groups_unavailable"`
  4. それ以外 → `"lookup_error"`
- [x] `validateRunAsIdentity(cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, analysis *Analysis)` を追加し、`analyzeCommand` の検証部（現行 `:273-296`）をこの呼び出しに置き換える。処理順は設計書 §6.1 に従う。
  - 基準識別情報として `risktypes.OriginalExecutionIdentity()` を 1 回だけ取得し、`ResolveRunAsIdentStrict` と `runAsFailureKind` の両方に渡す。1 コマンドあたりの解決呼び出しは 1 回とする（AC-17）。
  - 成功時: `analysis.Impact.Description` に ` [INFO: User/Group identity resolution validated]` を追記し、`d.logger.Info("Dry-run run-as identity resolved", ...)` を 1 件出す。属性は `dry_run`(true) / `command` / `group` / `run_as_user` / `run_as_group` / `resolved_uid` / `resolved_gid`。`group` は `group == nil` のとき空文字にする。属性名 `command` は設計書 §4.2 の表に従う（`internal/common/logschema.go` の `command_name` は監査ログを読む側と共有する定数である。一方、ここで出すのは dry-run 専用の記録であるため、属性名は揃えない）。
  - 失敗時: `analysis.Impact.Description` に失敗の記述を追記し、`raiseSecurityRisk(analysis, runnertypes.RiskLevelHigh)` を呼び、`d.logger.Warn("Dry-run run-as identity resolution failed", ...)` を 1 件出す。属性は成功時の前半 5 つに `failure_kind` と `error` を加えたもので、`resolved_uid` / `resolved_gid` は付けない。
  - 検証の後に警告を判定する。`d.privilegeManager == nil` のときは、特権マネージャに問い合わせずに ` [WARNING: User/Group privilege management not supported]` を追記する。`d.privilegeManager != nil` のときは、`!d.privilegeManager.IsPrivilegedExecutionSupported()` が真の場合に同じ文字列を追記する。この文字列は変更前と同一である。
  - 環境変数値やコマンド引数はログ属性に含めない。
- [x] `analyzeCommand` から `runnertypes.ElevationContext` の組み立てと `d.privilegeManager.WithPrivileges` の呼び出しを削除する。`analysis.Parameters["run_as_user"]` / `["run_as_group"]` の設定は現在の位置に残す。
- [x] import に `os/user` を追加する（`errors` / `fmt` / `log/slog` / `runnertypes` は既にある）。

**文言の変更**

追記される 3 種の文字列は設計書 §3.4 の表のとおりである。実装で使う書式は次の 2 つで、変更前後の完全な文字列を示す。

| 位置 | 変更前 | 変更後 |
|---|---|---|
| 検証成功 | `" [INFO: User/Group configuration validated]"` | `" [INFO: User/Group identity resolution validated]"` |
| 検証失敗 | `fmt.Sprintf(" [ERROR: User/Group validation failed: %v]", err)` | `fmt.Sprintf(" [ERROR: User/Group identity resolution failed: %v]", err)` |
| 特権非対応 | `" [WARNING: User/Group privilege management not supported]"` | 変更なし（同一文字列） |

**作業内容（テスト）**

テストは `package resource` にあるため、生成した `manager` のフィールドへ直接代入してリゾルバとロガーを差し替える。**`run_as` 指定を持つすべてのサブテストでスタブリゾルバを注入する。** 既定のリゾルバは実 OS のユーザーデータベースを引くため、`testuser` / `testgroup` という実在しない名前を使う既存サブテストは、注入しなければ結果がホスト依存になる（要件のリスク表「実際の OS 状態に依存しないテストとする」）。

- [x] `test_helpers.go` に `riskLevelTestEvaluator{level runnertypes.RiskLevel}` を追加する。`permissiveTestEvaluator` と同じインタフェースを満たし、指定したレベルの許可プランを返す。同ファイルの build tag（`//go:build test || performance`）の下でコンパイルできることを確認する。
- [x] `TestDryRunResourceManager_UserGroupValidation` の各サブテストを更新する。
  - [x] `valid_user_group_specification`: 成功を返すスタブリゾルバを注入する。`ElevationCalls` への `assert.Contains` を削除し、期待文字列を `[INFO: User/Group identity resolution validated]` に更新する。
  - [x] `invalid_user_group_specification`: `NewFailingMockPrivilegeManager` に依存する構成をやめ、`user.UnknownUserError` を返すスタブリゾルバを注入する。期待文字列を `[ERROR: User/Group identity resolution failed:` に更新し、`SecurityRisk == riskLevelHigh` の期待は維持する（AC-01）。
  - [x] `user_group_not_supported`: 成功を返すスタブリゾルバを注入し、`[INFO: User/Group identity resolution validated]` と `[WARNING: User/Group privilege management not supported]` の**両方**が `Description` に現れることを期待する（AC-09 / AC-10）。
  - [x] `no_privilege_manager`: 同じく成功リゾルバを注入し、両方の文字列が現れることを期待する（AC-08 / AC-10）。
  - [x] `only_user_specified`: 成功リゾルバを注入し、`ElevationCalls` への `assert.Contains` を削除する。リゾルバが `groupName == ""` で呼ばれたことと新しい成功文言を検査する。
  - [x] `no_user_group_specification`: `assert.Empty(t, mockPriv.ElevationCalls)` を「スタブリゾルバが呼ばれていないこと」の検査に置き換える。`User/Group` を含まないことの検査は維持する（AC-20）。
- [x] `TestDryRunResourceManager_GroupNameResolutionFailure` を追加する。`user.UnknownGroupError` を返すリゾルバで、失敗報告と `high` を確認する（AC-02）。
- [x] `TestDryRunResourceManager_SupplementaryGroupsUnavailable` を追加する。`Groups == nil` を返すリゾルバで、失敗が報告され `SecurityRisk` が `high` になることを確認する（AC-03 / AC-19）。変更前は成功扱いだった入力であることを doc コメントに書く。
- [x] `TestDryRunResourceManager_RiskRaiseIsMonotonic` を追加する。`riskLevelTestEvaluator{runnertypes.RiskLevelCritical}` と失敗リゾルバを組み合わせ、`SecurityRisk` が `critical` のまま下がらないことを確認する（AC-20、設計書 §3.4）。
- [x] `TestDryRunResourceManager_ResolverCalledOncePerCommand` を追加する。呼び出し回数を数えるリゾルバを注入し、`run_as` 指定を持つコマンド 1 件の解析でちょうど 1 回だけ呼ばれることを確認する（AC-17）。
- [x] `TestDryRunResourceManager_RunAsIdentityLogAttributes` を追加する。`slog.NewJSONHandler` でバッファに書くロガーとスタブリゾルバを注入し、次を確認する（AC-11 / AC-13）。
  - 成功時: レコードが 1 件で、`dry_run` / `command` / `group` / `run_as_user` / `run_as_group` / `resolved_uid` / `resolved_gid` が個別の JSON キーとして存在する。
  - 失敗時: レコードが 1 件で、`failure_kind` と `error` が個別のキーとして存在し、`failure_kind` の値が期待どおりである。
  - コマンドに環境変数 `GSCR_TEST_SECRET=sentinel-env-value-0158`（他の出力と衝突しない一意な値）を与え、この値が JSON 出力全体に現れないことを確認する。
- [x] `TestRunAsFailureKind` を追加し、4 種の `failure_kind`（`user_unknown` / `group_unknown` / `supplementary_groups_unavailable` / `lookup_error`）と基準識別情報由来の `base_identity_groups_unavailable` を表駆動で確認する。
- [x] `TestParseDisplayRiskLevel` を追加し、5 つの正規の文字列・空文字・未知の文字列を表駆動で確認する。
- [x] `TestDryRunPreservesProcessIdentity` を追加する。`run_as` 指定のあるコマンドの dry-run 前後で `os.Getuid()` / `os.Getgid()` / `os.Getgroups()` が一致することを確認する（AC-07 / AC-21）。Phase 4 で削除する `privilege` 側の同等テストより先にこのテストを用意する。
- [x] `resource/identity_mutation_guard_test.go` を追加する。`privilege/identity_mutation_guard_test.go` と同じ AST 走査方式とし、**パッケージディレクトリ配下の本番 `.go` ファイル全体**を対象にする（単一ファイルに限定しない）。識別情報を変更する関数（`Seteuid` / `Setegid` / `Setuid` / `Setgid` / `Setreuid` / `Setregid` / `Setresuid` / `Setresgid` / `Setgroups` / `Setfsuid` / `Setfsgid` / 生 `Syscall` 系 / `Capset` / `Prctl`）を検査対象とし、呼び出しの許可リストは空とする。1 件でも見つかったら失敗とする。既存ガードとの関係（別パッケージを守る別のガードであること、許可リストが空である理由）を doc コメントに書く。
- [x] `risktypes/identity_mutation_guard_test.go` を追加する。同じ方式で `risktypes` パッケージディレクトリを対象にする。`OriginalExecutionIdentity` が使う `os.Getuid` / `os.Getgid` / `os.Getgroups` は読み取りのみであり、検査対象の変更系関数には含まれない。

**完了条件**

- `rg -n "privilegeManager\.WithPrivileges" internal/runner/resource/dryrun_manager.go` の結果が 0 件（`WithPrivileges` は `resource` の各マネージャ自身のメソッドとしても定義されているため、パッケージ全体を対象にした検索は使わない）。
- `go test -tags test ./internal/runner/resource/...` が成功する。
- `make test` と `make lint` が成功する。

### PR-3 作成ポイント: dry-run validation delegates to the shared resolver

**対象ステップ**: Phase 3

**推奨タイトル**: `feat(0158): delegate dry-run run-as validation to risktypes`

**レビュー観点**: `raiseSecurityRisk` の単調性（`max` による引き上げのみで下げないこと）が設計書 §3.4 と一致するか / `runAsFailureKind` の判定順序が `errors.Is`/`errors.AsType` のみで直接比較（`==`）を使っていないか / 補助グループ列挙失敗（変更前は成功扱い）が意図どおり失敗扱いに変わる回帰があるか（AC-03/AC-19）/ 構造化ログの属性に環境変数やコマンド引数が含まれていないか（秘密情報漏洩防止、AC-13）/ 6 件の既存サブテスト書き換えと 10 件の新規テストが個別に意図した不変条件を検査しているか

**実装モデル要件**: frontier-required

**判定理由**: `SecurityRisk` を fail-closed 方向に引き上げるリスク評価ロジックの変更であり `mkplan.md` step 8 のパネルモード・トリガーが挙げる「セキュリティゲート」に該当し、加えて既存 6 サブテストの書き換えと新規 10 テスト（`TestDryRunResourceManager_{GroupNameResolutionFailure, SupplementaryGroupsUnavailable, RiskRaiseIsMonotonic, ResolverCalledOncePerCommand, RunAsIdentityLogAttributes}` / `TestRunAsFailureKind` / `TestParseDisplayRiskLevel` / `TestDryRunPreservesProcessIdentity` / 識別情報ガード 2 件）の追加という「many test updates」の水準に達している。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: `privilege` / `runnertypes` から dry-run 専用経路を削除する

**前提**: Phase 3 が完了していること。Phase 3 より先に行うと dry-run が `OperationUserGroupDryRun` を使えなくなり、中間状態が壊れる（設計書 §8）。また Phase 3 で `resource` 側の識別情報不変テストが用意済みであることを確認してから、`privilege` 側の同等テストを削除する。

**対象ファイル**

- `internal/runner/base/privilege/unix.go`（変更）
- `internal/runner/base/runnertypes/config.go`（変更）
- `internal/runner/base/privilege/testutil/mocks.go`（変更）
- `internal/runner/base/privilege/unix_privilege_test.go` / `manager_test.go`（変更）
- `internal/runner/base/privilege/unix_test.go`（削除）

**作業内容（本体）**

- [ ] `unix.go` から `resolveUserGroupForDryRun` を削除する。
- [ ] `unix.go` から `buildUserGroupLogAttrs` を削除する（同関数専用のヘルパである）。
- [ ] `prepareExecution` の `case runnertypes.OperationUserGroupDryRun: execCtx.needsPrivilegeEscalation = false` を削除する。
- [ ] `performElevation` の先頭にある dry-run 分岐を削除する。
- [ ] `restorePrivilegesAndMetrics` の `else if panicValue == nil && execCtx.elevationCtx.Operation == runnertypes.OperationUserGroupDryRun { ... }` を、枝ごと削除する（設計書 §3.5）。
- [ ] `os/user` と `strconv` の import を削除する。`os` と `syscall` は他で使うため残す。
- [ ] `runnertypes/config.go` から `OperationUserGroupDryRun` の定義行を削除する。
- [ ] `privilege/testutil/mocks.go` の `case runnertypes.OperationUserGroupDryRun:` とその記録行（`"user_group_dry_run:"` を組み立てる行）を削除する。

**作業内容（doc コメントの書き換え）**

削除後は存在しない `Operation` を説明する文になるため、語を消すだけでなく変更後の事実を述べる形に書き直す。3 か所とも英語で書く。

- [ ] `WithPrivileges` の doc コメント（`unix.go:89-90`）: 現在の 2 行 `// RunAsUser and RunAsGroup are resolved and logged inside this package only for` / `// OperationUserGroupDryRun.` を削除し、「この package は `RunAsUser` / `RunAsGroup` を一切解決しない。解決は `risktypes.ResolveRunAsIdentStrict` が行い、この package が受け付ける `Operation` はいずれも root への昇格を伴う」という趣旨の 2 行に置き換える。
- [ ] `restorePrivilegesAndMetrics` 冒頭のコメント（`unix.go:215-217`）: 現在の 3 行（`// Note: no branch restores the effective group ID here.` から `// so there is nothing to restore.` まで）を削除し、「実効グループ ID を復元する枝が無いのは、この package が実効 UID の昇格と復元しか行わないためである」という趣旨に置き換える。
- [ ] saved-set 検査前のコメント（`unix.go:228-232`）: `OperationUserGroupDryRun` に言及する 2 行（`:231-232`）を削除し、「識別情報を変えるのは昇格だけなので、昇格の有無が検証のゲートになる」という趣旨に置き換える。同じ段落の `after every non-dry-run privilege operation` という表現も、dry-run 経路が無くなったことに合わせて `after every privilege operation that escalated` の趣旨に直す。

**作業内容（テストの整理）**

`&UnixPrivilegeManager{}` と `&executionContext{}` を直接組み立てるテストを「昇格あり」に書き換える場合、`restorePrivilegesAndMetrics` の防御的検査（`unix.go:233-262`）がすべて動く点に注意する。この検査は `identityVerifier` を呼び、`originalSUID >= 0` のとき saved-set の読み直しを行う。`executionContext` を構造体リテラルで組み立てると `originalSUID` が 0 になるため、この条件を通過してしまう。その結果、読み直した実環境の saved-set（例: 1000）と一致せず、`emergencyShutdown` が走る。したがって**書き換える 4 件すべてで次の構成を共通に用いる**（`TestRestorePrivilegesAndMetrics_SavedSetUnchanged_Passes` と同じ形）。

```
manager: privilegeSupported: true, originalUID: 0,
         identityVerifier: func() error { return nil },
         osExit: func(int) { t.Fatal("emergencyShutdown called unexpectedly") }
execCtx: Operation: OperationFileValidation, needsPrivilegeEscalation: true,
         originalSUID: -1, originalSGID: -1, start: time.Now()
```

`originalUID: 0` を指定すると root で動作している扱いになり、実際の `seteuid` 呼び出しを回避できる。`originalSUID: -1` は saved-set 検査を構造的にスキップする。

- [ ] `unix_privilege_test.go::TestPrepareExecution_Success` の `user_group_dryrun` ケースを削除する。
- [ ] `unix_privilege_test.go::TestPerformElevation_Success` を削除する。
- [ ] `unix_privilege_test.go::TestPerformElevation_Failure` の `invalid_user_in_dryrun` サブテストを削除する。
- [ ] `unix_privilege_test.go::TestWithPrivileges_UserGroupDryRunDoesNotChangeIdentity` を削除する（代替は Phase 3 の `TestDryRunPreservesProcessIdentity`）。
- [ ] `unix_privilege_test.go::TestHandleCleanupAndMetrics_Success` を上記の共通構成に書き換える。`ElevationSuccesses == 1` に加え、`handleCleanupAndMetrics` 固有の振る舞い（panic が無いときに duration が計上され、`restorePrivilegesAndMetrics` に非ゼロの duration が渡ること）を検査対象に含め、`TestRestorePrivilegesAndMetrics_Success` との重複をなくす。コメントから dry-run への言及を落とす。
- [ ] `unix_privilege_test.go::TestHandleCleanupAndMetrics_WithError` の `Operation` を `OperationFileValidation` に差し替える（panic 再送出の検査内容は変えない。`needsPrivilegeEscalation` は `false` のままでよく、共通構成の適用は不要である）。
- [ ] `unix_privilege_test.go::TestRestorePrivilegesAndMetrics_Success` を上記の共通構成に書き換え、`ElevationSuccesses == 1` を維持する。
- [ ] `unix_privilege_test.go::TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalationOrDryRun` を `TestRestorePrivilegesAndMetrics_NoSuccessWithoutEscalation` に改名する。doc コメントとアサーションのメッセージから dry-run への言及を落とし、「昇格を伴わない operation では success を記録しない」という説明に書き換える（英語で書く）。
- [ ] `unix_privilege_test.go::TestRestorePrivilegesAndMetrics_Failure` を上記の共通構成に書き換え（ただし panic 値を渡す）、panic 時に success が記録されないという期待を維持する。
- [ ] `unix_privilege_test.go::TestResolveUserGroupForDryRun` を削除する。この関数だけが使っていた import（`bytes` / `strconv` など）が不要になれば削除する。
- [ ] `unix_privilege_test.go::TestRestorePrivilegesAndMetrics_IdentityVerificationSkippedForDryRun` を `TestRestorePrivilegesAndMetrics_IdentityVerificationSkippedWithoutEscalation` に改名し、`Operation` を `OperationFileValidation`・`needsPrivilegeEscalation` を `false` にして、`identityVerifier` が呼ばれないことの検査を維持する（削除すると「識別情報の検証が昇格の有無でゲートされる」という不変条件を検査するテストが無くなる）。doc コメントを英語で書き換える。
- [ ] `internal/runner/base/privilege/unix_test.go` をファイルごと削除する。含まれる 2 関数（`TestUnixPrivilegeManager_DryRunResolution` / `TestUnixPrivilegeManager_PrivilegeValidation`）はいずれも dry-run 専用であり、他へ移す検査は無い。
- [ ] `manager_test.go::TestManager_WithPrivileges_UserGroup_ValidUser` の `dry_run_mode` サブテストを削除する（root 時のみ走る `actual_change` は残す）。
- [ ] `manager_test.go::TestManager_WithPrivileges_UserGroup_InvalidUser` を削除する。
- [ ] `manager_test.go::TestManager_WithPrivileges_UserGroup_EmptyUserGroup` を削除する。
- [ ] `manager_test.go::TestManager_WithPrivileges_UserGroup_FunctionError` を書き換える（削除しない）。上記の共通構成の manager を用い、`Operation` を `OperationFileValidation` にして `WithPrivileges` を呼び、コールバックが返したエラーがそのまま返ること（`assert.Equal(expectedErr, err)`）を検査する。この不変条件を検査するテストは他に無い。
- [ ] 上記の削除で未使用になったヘルパ（`manager_test.go` の `getCurrentUser` / `getCurrentGroup` など）を確認し、参照が無くなったものは削除する。

**完了条件**

- `rg -n -e "OperationUserGroupDryRun" -e "user_group_dry_run" --glob '!docs/tasks/**' .` の結果が 0 件。
- `rg -n -e "resolveUserGroupForDryRun" -e "buildUserGroupLogAttrs" --glob '!docs/tasks/**' .` の結果が 0 件。
- `make deadcode` の出力に `resolveUserGroupForDryRun` / `buildUserGroupLogAttrs` が現れず、報告件数がベースラインの 7 件から増えていない（このコマンドは終了コードで失敗を伝えないため、出力の内容で判定する。§1.4 の外部前提の表を参照）。
- `make test` と `make lint` が成功する。

### PR-4 作成ポイント: remove the dry-run-only privilege path

**対象ステップ**: Phase 4

**推奨タイトル**: `refactor(0158): remove dry-run-only privilege paths after delegation`

**レビュー観点**: 書き換えた 4 件の `unix_privilege_test.go` テストが共通構成（`originalUID: 0` / `originalSUID: -1` / `identityVerifier` / `osExit`）を漏れなく適用し `emergencyShutdown` の誤爆を避けているか / `TestManager_WithPrivileges_UserGroup_FunctionError` のようにコールバックエラー伝播やゲート条件を検査する不変条件が削除ではなく書き換えで維持されているか / `OperationUserGroupDryRun` と `user_group_dry_run` の全参照（本体・doc コメント・テスト）が本当に消えたか（AC-14/AC-15）/ `make deadcode` の報告件数がベースラインの 7 件のまま増えていないか

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` step 8 のパネルモード・トリガーが挙げる「セキュリティゲート/マイグレーション」に該当する。`performElevation` / `restorePrivilegesAndMetrics` という特権昇格・復元の中核経路から dry-run 専用分岐を除去する変更であり、加えて 18 項目のテスト整理（既存 4 件の書き換えは構成を誤ると `emergencyShutdown` を誤発火させる）という「many test updates」の水準に達している。本体の削除とテスト整理をさらに 2 PR へ分割することはできない（`resolveUserGroupForDryRun` 等を削除すると、それを参照する `unix_privilege_test.go` の該当テストがコンパイルできなくなるため、同一 PR に含める必要がある）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 一致テスト・静的検査・文書の追随

**前提**: Phase 2 / 3 / 4 が完了していること。

**対象ファイル**

- `internal/runner/base/risktypes/testutil/mocks.go`（新規、`package risktypestestutil`、`//go:build test`）
- `internal/runner/base/risktypes/testutil/helpers.go`（新規、`package risktypestestutil`、`//go:build test`）
- `internal/runner/base/executor/executor_usergroup_test.go`（変更）
- `internal/runner/resource/usergroup_dryrun_test.go`（変更）
- `docs/dev/architecture_design/command-risk-evaluation.ja.md` / `command-risk-evaluation.md`（変更）
- `docs/translation_glossary.md`（変更）

**作業内容（AC-04 の一致テスト）**

- [ ] `risktypes/testutil/` を新規に作る。テスト構成ガイドの Classification A（複数パッケージから使い、公開 API のみを使う）に該当する。ガイドのファイル分類に従い 2 ファイルに分ける。
  - `mocks.go`: 4 種のスタブ `risktypes.RunAsResolver`（成功 / `user.UnknownUserError` / `user.UnknownGroupError` / `Groups == nil`）。エラー値はパッケージレベルの `var` として宣言する（このファイルは `_test.go` ではないため `.golangci.yml` の `err113` 除外が効かず、関数本体での `errors.New` は指摘される）。
  - `helpers.go`: 型 `RunAsResolutionCase`（`Name string` / `UserName string` / `GroupName string` / `Resolver risktypes.RunAsResolver` / `WantFailure bool`）と、設計書 §7.2 の 4 ケースを返す `RunAsResolutionCases() []RunAsResolutionCase`。
- [ ] `executor_usergroup_test.go` に `TestExecuteWithUserGroup_SharedResolutionCases` を追加する。各ケースのリゾルバを `WithRunAsResolver` で注入し、次を検査する。
  - `WantFailure` が真: `assert.ErrorIs(err, risktypes.ErrRunAsIdentityResolution)` かつ `mockPriv.ElevationCalls` が空。
  - `WantFailure` が偽: `assert.NotErrorIs(err, risktypes.ErrRunAsIdentityResolution)` かつ `mockPriv.ElevationCalls` に `user_group_change:` で始まる記録がある。実際のプロセス起動は環境によって成功も失敗もするため、終了状態は意図的に検査しない（既存の `executor_usergroup_test.go:78-81` のコメントと同じ扱い）。
- [ ] `usergroup_dryrun_test.go` に `TestDryRunResourceManager_SharedResolutionCases` を追加する。同じケース表を読み、`WantFailure` が真のケースでは `SecurityRisk` が `high` 以上かつ `Description` に `identity resolution failed` が含まれること、偽のケースでは `identity resolution validated` が含まれることを検査する。
- [ ] 両テストの doc コメントに、担当範囲を英語で書く。Phase 3 で追加した個別テストが文言・リスクレベル・ログ属性を担当し、この一致テストは dry-run と実行時の判定が一致することだけを担当する。また、固定するのは識別情報の解決に関する一致だけであり、実行時のみの前提条件（設計書 §1.3）は対象外である。

**作業内容（文書の追随）**

- [ ] `docs/dev/architecture_design/command-risk-evaluation.ja.md` の「拒否 / エラー / High 許可の区別（dry-run の失敗時挙動）」節（`:439`）の末尾に注記を追加する。記載する内容は次の 3 点とする。
  1. run-as 識別情報の検証失敗は、この方針の例外として `Impact.SecurityRisk` を `high` へ引き上げる形で表示される。
  2. deny 予告にも終了コードにも反映されない。
  3. 解消は dry-run の deny 判定全体の設計に属する別論点であり、経緯は本タスクの設計書 §5.7 にある。
- [ ] 追記した注記を設計書 §5.7 と読み合わせ、上記 3 点がすべて含まれ、かつ §5.7 と矛盾しないことを確認する（存在検索だけでは内容の正しさを確かめられないため）。
- [ ] `docs/translation_glossary.md` に「run-as 識別情報 / run-as identity」の対訳を追加する（既存の項目を確認し、既にあれば追加しない）。
- [ ] 日本語版を先にコミットしたうえで、`/mktrans` により `command-risk-evaluation.md` の "Deny vs Error vs High-allowable (dry-run failure handling)" 節（`:439`）へ反映する（CLAUDE.md の翻訳ワークフロー）。反映後、日本語版と英語版で節構成と段落数が一致することを目視で確認する。

**完了条件**

- `make test`、`make lint` が成功する。
- 「6. 受入基準の検証」の静的検査コマンドがすべて期待どおりの結果になる。
- `make unit-test-cgo1` と `make unit-test-cgo0` の両方が成功する。

### PR-5 作成ポイント: shared resolution cases and doc follow-up

**対象ステップ**: Phase 5

**推奨タイトル**: `test(0158): add shared run-as resolution cases and doc updates`

**レビュー観点**: `risktypes/testutil` の 4 スタブケースが executor（実行時）と resource（dry-run）の両方から見て等価な入力を表しているか / `TestExecuteWithUserGroup_SharedResolutionCases` と `TestDryRunResourceManager_SharedResolutionCases` が実際に判定結果の一致だけを検査し Phase 3 の個別テストと重複していないか / `command-risk-evaluation.ja.md` への追記が設計書 §5.7 の 3 点（例外の内容・終了コード非反映・別論点である旨）を過不足なく含むか / 英語版への反映が `/mktrans` のワークフロー（日本語版を先にコミット）に従っているか

**実装モデル要件**: standard

**判定理由**: `risktypes/testutil` の新規 `mocks.go`/`helpers.go` は `//go:build test` の非 `_test.go` ファイルであり Conditional checks の「ビルドタグ配下でのみコンパイルされる新規非テストソース」に該当するが、該当する Conditional checks は 1 件のみで frontier-recommended の閾値（2 件以上）に届かない。既存コード調査結果に競合アプローチの記載もなく、パネルモードのトリガーにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン

| マイルストーン | 含む Phase | 成果物 | 完了判定 |
|---|---|---|---|
| M1: 共有関数の成立 | Phase 1 | `ResolveRunAsIdentStrict` とその単体テスト | `make test` / `make lint` が成功する。AC-05 / AC-06 のテストが通る |
| M2: 実行時経路の委譲 | Phase 2 | `executor` が共有関数を呼ぶ状態 | `rg -n "^var ErrRunAsIdentityResolution" internal/runner/base/executor/` と `rg -n "executor\.ErrRunAsIdentityResolution" internal/` がいずれも 0 件 |
| M3: dry-run 経路の委譲 | Phase 3 | dry-run の検証・構造化ログ・単調なリスク引き上げ・識別情報ガード | `rg -n "privilegeManager\.WithPrivileges" internal/runner/resource/dryrun_manager.go` が 0 件。AC-01〜AC-03 / AC-07〜AC-13 / AC-17 / AC-19〜AC-21 のテストが通る |
| M4: 到達不能コードの削除 | Phase 4 | dry-run 専用の特権経路が無い状態 | AC-14 / AC-15 / AC-18 の静的検査が期待どおり。`make deadcode` の報告が 7 件のベースラインから増えていない |
| M5: 一致の固定と文書の追随 | Phase 5 | 共有ケース表・一致テスト・文書の注記 | AC-04 / AC-22 / AC-23 が満たされる |

Phase 2 と Phase 3 は互いに独立で、順序を入れ替えてもよい。Phase 4 は Phase 3 の完了後に行う（設計書 §8）。設計書 §7.3 が求める識別情報ガードとプロセス識別情報の不変テストは、`privilege` 側の同等テストを削除する Phase 4 より前（Phase 3）に置く。これは M4 の PR が「不変条件を誰も検査していない状態」でマージ可能になることを避けるためである。

### 3.2 PR 構成

各マイルストーン（Phase）はそのまま 1 つの PR に対応する。本書のステップは「ステップ X-Y」形式では区切られておらず Phase 単位が実装の最小単位であるため、`対象ステップ` は Phase 番号で表す。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `risktypes.ResolveRunAsIdentStrict` と `RunAsResolver` / 番兵エラー 2 件の新規追加 | standard |
| PR-2 | Phase 2 | `executor` の自前解決ロジックを `ResolveRunAsIdentStrict` の呼び出しへ置き換え | standard |
| PR-3 | Phase 3 | dry-run の検証を共有関数へ委譲し、構造化ログと単調なリスク引き上げを追加 | frontier-required |
| PR-4 | Phase 4 | `privilege` / `runnertypes` から dry-run 専用の特権経路を削除 | frontier-required |
| PR-5 | Phase 5 | 実行時・dry-run 共有の一致テストケース表、および文書の追随 | standard |

---

## 4. テスト戦略

設計書 §7 が定めるテスト方針をそのまま実装対象とする。本節は、そこに書かれていない実装上の判断のみを記す。

- **注入方針**: 補助グループ列挙の失敗は実 OS では再現しにくいため、注入した `RunAsResolver` で再現する。加えて、`run_as` 指定を持つテストはすべてリゾルバを注入し、実 OS のユーザーデータベースに依存させない。
- **層の分担**: Phase 3 で追加する `resource` 個別テストが `Description` の文言・リスクレベル・ログ属性を担当し、Phase 5 の共有ケース表テストは dry-run と実行時の判定の一致だけを担当する。同じ 4 ケースを 2 層で扱うが、検査対象は重複しない。
- **境界値**: 補助関数（`parseDisplayRiskLevel` / `raiseSecurityRisk` / `runAsFailureKind`）は、空文字・未知の文字列・基準識別情報由来の失敗を含めて表駆動で検査する。
- **後方互換性**: `run_as` 指定を持たないコマンドの出力が変わらないことを既存サブテスト `no_user_group_specification` で固定する。設定ファイル形式と CLI オプションは変更しないため、これらの互換性テストは追加しない。
- **ビルド構成**: `u.GroupIds()` の挙動がビルド構成に依存するため（設計書 §5.5）、Phase 5 の完了時に `make unit-test-cgo1` と `make unit-test-cgo0` の両方を実行し、結果を「8. 成功基準」に記録する。macOS と最小構成コンテナは本環境で確認できないため、未確認であることを明示する。

---

## 5. リスク管理

### 5.1 技術的リスク

| リスク | 影響 | 緩和策 |
|---|---|---|
| Phase 4 のテスト整理で、削除したテストが持っていた不変条件が失われる | 昇格経路やエラー伝播の回帰を検知できなくなる | §1.4 の表で 1 件ずつ「削除」「書き換え」を決める。エラー伝播（`TestManager_WithPrivileges_UserGroup_FunctionError`）、識別情報検証のゲート（`..._IdentityVerificationSkipped...`）、dry-run の識別情報不変（`TestDryRunPreservesProcessIdentity`）はいずれも削除せず移設・書き換えとする |
| 構造体リテラルで組み立てたテストを「昇格あり」に変えると、防御的検査が実環境の saved-set を読んで `emergencyShutdown` に落ちる | Phase 4 のテストが環境によって失敗し、結果が安定しなくなる | Phase 4 に共通構成（`originalUID: 0` / `originalSUID: -1` / `identityVerifier` / `osExit` を必ず設定する）を明記し、書き換える 4 件すべてに同じ構成を適用する |
| §1.3 の設計差分が承認されない | Phase 1 の実装内容が変わる | Phase 1 の最初の作業項目として承認確認を置き、承認されない場合は着手前に再検討する |
| macOS と最小構成コンテナでの `u.GroupIds()` の挙動が未確認 | その環境で dry-run が `run_as` 指定のある全コマンドを検証失敗と報告し得る | 未確認であることを本書と設計書 §5.5 に明記する。実行時経路が既に同じ依存を持つため、dry-run の報告は実行時の結果と一致する（設計書 §5.5） |
| 英語版文書の翻訳漏れ・内容のずれ | AC-23 が満たされない | 日本語版を先にコミットし、`/mktrans` で英語版へ反映する手順と、反映後の節構成の目視確認を Phase 5 の作業項目として明記する |

### 5.2 スケジュール上のリスク

| リスク | 影響 | 緩和策 |
|---|---|---|
| §1.3 の設計差分の承認待ちで Phase 1 が着手できない | 全 Phase が後ろにずれる | 差分は 1 点のみで、影響範囲も番兵エラー 1 個の追加に限られる。承認依頼を本計画のレビューと同時に出す |
| Phase 4 のテスト整理（18 項目）が想定より膨らむ | M4 が遅れる | Phase 4 は本体の削除（8 項目）とテストの整理（18 項目）を分けて進められる。本体の削除とコメント書き換えを先に済ませ、テスト整理はテスト単位で刻んで進める |
| macOS 環境での確認手段が最後まで得られない | M5 の完了判定が保留になる | 未確認であることを完了の妨げとはせず、本書と PR 説明にその旨を記載したうえで完了とする（実行時経路が既に同じ依存を持つため、新たなリスクを持ち込まない） |

---

## 6. 受入基準の検証

「種別」は `test`（実行可能なテスト）/ `static`（`rg` またはコンパイル）/ `manual`（人手の確認）を表す。

| AC | 種別 | 検証内容 |
|---|---|---|
| AC-01 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_UserGroupValidation`（サブテスト `invalid_user_group_specification`）。解決できないユーザー名で検証失敗が報告され `SecurityRisk == "high"` になる |
| AC-02 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_GroupNameResolutionFailure` |
| AC-03 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_SupplementaryGroupsUnavailable` および `internal/runner/base/risktypes/runas_ident_strict_test.go::TestResolveRunAsIdentStrict_NilGroups` |
| AC-04 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_SharedResolutionCases` と `internal/runner/base/executor/executor_usergroup_test.go::TestExecuteWithUserGroup_SharedResolutionCases`（共有ケース表 `risktypestestutil.RunAsResolutionCases()` の 4 ケース） |
| AC-05 | test | `internal/runner/base/risktypes/runas_ident_strict_test.go::TestResolveRunAsIdentStrict_ArgumentForms`（ユーザーのみ指定の行） |
| AC-06 | test | `internal/runner/base/risktypes/runas_ident_strict_test.go::TestResolveRunAsIdentStrict_ArgumentForms`（グループのみ指定の行。UID が `OriginalExecutionIdentity()` 由来であることを確認する） |
| AC-06 | static | `rg -c -e "syscall\.Geteuid" -e "syscall\.Getegid" internal/runner/base/privilege/unix.go` の結果が `5`（変更前は `9`）。実効 ID を基準にしていた旧 dry-run 実装の 4 行（`:499`, `:548`, `:560`, `:561`）が消え、昇格・復元と識別情報検査で使う 5 行（`:74`, `:77`, `:354`, `:361`, `:414`）だけが残ることを示す |
| AC-07 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunPreservesProcessIdentity`（dry-run 前後で `os.Getuid` / `os.Getgid` / `os.Getgroups` が一致） |
| AC-07 | static | `internal/runner/resource/identity_mutation_guard_test.go::TestResourcePackageDoesNotMutateIdentity` と `internal/runner/base/risktypes/identity_mutation_guard_test.go::TestRisktypesPackageDoesNotMutateIdentity`（AST 走査。許可リストは空） |
| AC-08 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_UserGroupValidation`（サブテスト `no_privilege_manager`）。検証結果が出力に現れる |
| AC-09 | test | 同上（サブテスト `user_group_not_supported`） |
| AC-10 | test | 同上（`no_privilege_manager` と `user_group_not_supported` の両サブテストで、`[INFO: User/Group identity resolution validated]` と `[WARNING: User/Group privilege management not supported]` の双方が `Description` に含まれることを確認する） |
| AC-11 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_RunAsIdentityLogAttributes`（失敗時のレコードが 1 件で、`command` / `run_as_user` / `run_as_group` / `error` / `failure_kind` が個別の JSON キーとして存在する） |
| AC-12 | test | 同 `::TestDryRunResourceManager_UserGroupValidation`（`invalid_user_group_specification` が `Description` に `[ERROR: User/Group identity resolution failed:` を含むことを確認） |
| AC-13 | test | 同 `::TestDryRunResourceManager_RunAsIdentityLogAttributes`（コマンドに与えた `GSCR_TEST_SECRET=sentinel-env-value-0158` が JSON 出力全体に現れないことを確認） |
| AC-14 | static | `rg -n -e "resolveUserGroupForDryRun" -e "buildUserGroupLogAttrs" --glob '!docs/tasks/**' .` の結果が 0 件 |
| AC-15 | static | `rg -n -e "OperationUserGroupDryRun" -e "user_group_dry_run" --glob '!docs/tasks/**' .` の結果が 0 件（コメント中の参照を含む） |
| AC-15 | manual | dry-run 時に `RecordElevationSuccess` が記録されなくなること（metrics 系列の消滅）を PR 説明に明記し、レビューで確認する（要件のリスク表）。上の static 検査を置き換えるものではない |
| AC-16 | test | `internal/runner/base/privilege/unix_privilege_test.go::TestPrepareExecution_NotSupported`（既存、無修正）。未知の `Operation` に `ErrUnsupportedOperationType` が返る |
| AC-17 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_ResolverCalledOncePerCommand`（`run_as` 指定を持つコマンド 1 件の解析でリゾルバがちょうど 1 回呼ばれる） |
| AC-17 | static | `rg -c "user\.Lookup\(" internal/runner/base/risktypes/runas_ident.go` の結果が `1`（1 回の `user.Lookup` から UID・主 GID・補助グループを得ていることの回帰ガード）。加えて `rg -n "user\.Lookup" internal/runner/base/privilege/unix.go` が 0 件（旧実装の二重呼び出しが消えたこと） |
| AC-18 | static | `rg -n -e "user\.Lookup" -e "user\.LookupGroup" -e "user\.LookupGroupId" internal/runner/base/privilege --glob '!*_test.go'` の結果が 0 件 |
| AC-19 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_SupplementaryGroupsUnavailable`（変更前は成功扱いだった入力が `high` として報告される） |
| AC-20 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunResourceManager_UserGroupValidation`（サブテスト `no_user_group_specification`）、同 `::TestDryRunResourceManager_RiskRaiseIsMonotonic`、同 `::TestParseDisplayRiskLevel` |
| AC-21 | test | `internal/runner/resource/usergroup_dryrun_test.go::TestDryRunPreservesProcessIdentity` と AC-07 の 2 つのガードテスト。加えて `make test` 全体が成功すること（既存の dry-run 副作用テストの維持） |
| AC-22 | static | `rg -n "0157 の \[02_architecture.md\].*§5.6 が挙げた 2 つの限界" docs/tasks/0158_dryrun_runas_ident_unification/02_architecture.md` が 1 件を返す（設計書 §5.6 の該当文。単なる「0157」の出現数ではなく、解消を記録した文そのものを指す） |
| AC-22 | manual | 設計書 §5.6 の記述を 0157 の `02_architecture.md` §2.2.2 / §5.6 / §9 と読み合わせ、「別タスクで対応する」とされた乖離が実際に解消されたことが記録されていることを確認する |
| AC-23 | static | `rg -n "run-as 識別情報の検証" docs/dev/architecture_design/command-risk-evaluation.ja.md` が 1 件以上、`rg -n "run-as identity" docs/dev/architecture_design/command-risk-evaluation.md` が 1 件以上を返す（英語版は変更前 0 件であるため、この検索は変更の有無を判別できる） |
| AC-23 | manual | 追記した注記が設計書 §5.7 の 3 点（例外の内容・終了コードに反映されないこと・解消は別論点であること）をすべて含み、日本語版と英語版で節構成が一致することを確認する |

---

## 7. 実装チェックリスト

以下は PR 単位で整理している。PR と Phase は 1 対 1 で対応する（§3.2 参照）。

### PR-1（対象ステップ: Phase 1）

- [x] §1.3 の設計差分の承認確認と設計書 §3.1 / §4.1 の更新
- [x] `RunAsResolver` の追加
- [x] 番兵エラー 2 件の追加
- [x] `ResolveRunAsIdentStrict` の追加
- [x] 単体テスト 5 件の追加
- [x] `make fmt` / `make test` / `make lint`
- [ ] PR-1 マージ済み（対象ステップ: Phase 1）

### PR-2（対象ステップ: Phase 2）

- [x] `executor.ErrRunAsIdentityResolution` の削除
- [x] `runAsResolver` フィールドの型変更
- [x] `executeWithUserGroup` の委譲
- [x] `WithRunAsResolver` の型変更
- [x] 既存テストの参照先変更（2 か所）
- [x] `make fmt` / `make test` / `make lint`
- [ ] PR-2 マージ済み（対象ステップ: Phase 2）

### PR-3（対象ステップ: Phase 3）

- [ ] `runAsResolver` / `logger` フィールドの追加と既定値の設定
- [ ] `parseDisplayRiskLevel` / `raiseSecurityRisk` / `runAsFailureKind` の追加
- [ ] `validateRunAsIdentity` の追加と `analyzeCommand` の置き換え
- [ ] 出力文言の更新
- [ ] `riskLevelTestEvaluator` の追加
- [ ] 既存 6 サブテストの更新（全サブテストへのリゾルバ注入を含む）
- [ ] 新規テスト 10 件の追加（識別情報ガード 2 件と `TestDryRunPreservesProcessIdentity` を含む）
- [ ] `make fmt` / `make test` / `make lint`
- [ ] PR-3 マージ済み（対象ステップ: Phase 3）

### PR-4（対象ステップ: Phase 4）

- [ ] 本体の削除 8 項目
- [ ] doc コメント 3 か所の書き換え
- [ ] テスト整理 18 項目（共通構成の適用漏れがないこと）
- [ ] `make fmt` / `make test` / `make lint` / `make deadcode`（出力の内容で判定）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4）

### PR-5（対象ステップ: Phase 5）

- [ ] `risktypes/testutil/mocks.go` と `helpers.go` の追加
- [ ] 一致テスト 2 件の追加
- [ ] 文書の注記（日本語版 → 英語版）と用語集の更新
- [ ] 静的検査コマンドの実行と結果の記録
- [ ] `make unit-test-cgo1` / `make unit-test-cgo0`
- [ ] PR-5 マージ済み（対象ステップ: Phase 5）

### 横断検索（`make lint` / `make test` では検知できない項目）

- [ ] 旧文言の残存確認: `rg -n -e "User/Group configuration validated" -e "User/Group validation failed" --glob '!docs/tasks/**' .` が 0 件
- [ ] 旧ログ属性名の残存確認: `rg -n -e "target_uid" -e "current_euid" internal/runner/base/privilege/` の結果が `unix.go` の `emergencyShutdown`（`current_euid`）1 件のみ。`target_uid` は残らない（`internal/common/logschema.go` の同名定数は監査ログ用であり本タスクの対象外のため、検索範囲を `privilege` パッケージに限定する）
- [ ] 用語の一致確認: 本書と設計書で「run-as 識別情報」「基準識別情報」「fail-closed 判定」の語が同じ意味で使われていること（目視）

---

## 8. 成功基準

### 機能面

- [ ] dry-run と実行時が `risktypes.ResolveRunAsIdentStrict` のみを通じて識別情報を解決する。
- [ ] 補助グループ列挙の失敗を dry-run が検証失敗として報告する。
- [ ] 特権サポートの有無に関わらず dry-run の検証が実行される。
- [ ] 検証の成否が `slog` の構造化レコードとして 1 件出力される。

### 品質面

- [ ] `make test` と `make lint` が成功する。
- [ ] `make deadcode` の報告がベースラインの 7 件から増えず、削除した 2 関数が出力に現れない。
- [ ] 「6. 受入基準の検証」の全 AC が満たされる。
- [ ] `make unit-test-cgo1` と `make unit-test-cgo0` の両方が成功する（結果を PR 説明に記載する）。

### セキュリティ面

- [ ] dry-run がプロセスの識別情報を変更しないことを、実行時テストと AST ガードの双方で確認する。
- [ ] 判定結果の変化が fail-closed 方向のみであることを AC-19 / AC-20 のテストで固定する。

### 文書面

- [ ] `command-risk-evaluation.ja.md` と `command-risk-evaluation.md` に設計書 §5.7 の例外が注記される。
- [ ] `docs/translation_glossary.md` に「run-as 識別情報 / run-as identity」の対訳がある。
- [ ] 未確認の環境依存（macOS、最小構成コンテナ）が本書に明記されている。

---

## 9. 次のステップ

- 実装完了後、マイルストーン単位で PR を作成する（M2 / M3 / M4 / M5 が自然な区切りである）。
- 設計書 §9 に挙がっている将来課題（`ResolveRunAsIdent` の契約変更、検証失敗の終了コードへの反映、解決結果の共有、`run_id` の伝播、リゾルバ型の統一）は本タスクの対象外であり、必要になった時点で別タスクとして起票する。
- 本タスクで未確認のまま残る環境（macOS、最小構成コンテナ）での `u.GroupIds()` の挙動は、該当環境が利用可能になった時点で確認する。
