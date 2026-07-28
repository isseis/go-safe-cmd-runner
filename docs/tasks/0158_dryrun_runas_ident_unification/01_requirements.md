# 要件定義書: dry-run のユーザー・グループ検証を実行時の識別情報解決に統合する

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-27 |
| Review date | 2026-07-27 |
| Reviewer | isseis |
| Comments | - |

## 関連 Issue / 文書

- [#918 [Refactor][Security] dry-run のユーザー・グループ検証を実行時の識別情報解決に統合する（0157 フォローアップ）](https://github.com/isseis/go-safe-cmd-runner/issues/918)
- 親 Issue: [#864 [Security][P5] フィルタ未実装・命名と実装の乖離を整理（デッドコード削除含む）](https://github.com/isseis/go-safe-cmd-runner/issues/864)
- 前タスク: [0157 デッドコード削除・命名整理](../0157_dead_code_naming_cleanup/) — [02_architecture.md](../0157_dead_code_naming_cleanup/02_architecture.md) §2.2.2（乖離の詳細）、§5.6（監査可能性の限界）、§9（将来課題）
- 0149 監査 A1 L-1（`user.Lookup` の二重呼び出し）: [findings/A1_privilege.md](../0149_security_code_smell_audit_fable/findings/A1_privilege.md)

## 背景

0157 は privilege パッケージの到達不能な降格パスを削除し、dry-run 側に残る処理を「ユーザー名・グループ名の解決（検証）」として位置づけた。その際、この検証が実行時検査の**真部分集合**であることを記録し、統合は挙動変更を伴うため別タスク（本タスク）に送っている。

2026-07-27 時点の現物コードで、以下がいずれも未修正であることを確認済みである。

### 1. dry-run と実行時で識別情報の解決が別実装になっている

dry-run のユーザー・グループ解決は単なるログ用の飾りではなく、検証として機能している。[dryrun_manager.go:283-293](../../../internal/runner/resource/dryrun_manager.go) は `privilegeManager.WithPrivileges`（`Operation: OperationUserGroupDryRun`）が返したエラーを受けて、解析結果の `Impact.SecurityRisk` を `high` に引き上げる。

しかしその実装は実行時経路とは別物である。

| | dry-run 経路 | 実行時経路 |
|---|---|---|
| 実装 | `resolveUserGroupForDryRun`（[unix.go:469](../../../internal/runner/base/privilege/unix.go)） | `risktypes.ResolveRunAsIdent`（[runas_ident.go:55-89](../../../internal/runner/base/risktypes/runas_ident.go)） |
| ユーザー名・グループ名の解決 | 行う | 行う |
| 補助グループの列挙 | 行わない | 行う（`u.GroupIds()`） |
| 補助グループ列挙失敗時 | 検知しない | executor が `ErrRunAsIdentityResolution` で実行を拒否（[executor.go:204-213](../../../internal/runner/base/executor/executor.go)） |
| 基準となる識別情報 | `syscall.Geteuid()` / `syscall.Getegid()`（実効 ID） | `risktypes.OriginalExecutionIdentity()`（実 UID / GID を `sync.OnceValue` で一度だけ捕捉） |

したがって補助グループの列挙だけが壊れている構成では、dry-run が `[INFO: User/Group configuration validated]` を出したのちに実行が `ErrRunAsIdentityResolution` で失敗し得る。dry-run が誤った安心を与える状態であり、0157 が挙げた将来課題のうち唯一、実害のある乖離である。

なお、`user.Lookup` の失敗（ユーザー名そのものが解決できない場合）については、リスク評価器がゾーニング判定の入力を組み立てる際にも `ResolveRunAsIdent` を呼び、失敗を `IdentityUnresolved` として扱う経路がある（[evaluator.go:427-432](../../../internal/runner/base/risk/evaluator.go)）。ただしこの経路はゾーニングが有効な場合に限られ、補助グループ列挙の失敗（`Groups == nil`）は検知しない。よって上記の乖離は解消されない。同時に、同一の解決処理がリポジトリ内に 3 か所（privilege / executor / evaluator）存在し、うち 1 つだけが別実装であるという DRY 上の問題も生じている。

**`user.Lookup` の二重呼び出し（0149 監査 A1 L-1）。** `resolveUserGroupForDryRun` は `userName` の解決で `user.Lookup` を呼び（[unix.go:487](../../../internal/runner/base/privilege/unix.go)）、`groupName` が空の場合に primary group を得るためもう一度 `user.Lookup` を呼ぶ（[unix.go:520](../../../internal/runner/base/privilege/unix.go)）。`ResolveRunAsIdent` は 1 回の `user.Lookup` から UID・primary GID・補助グループを取得するため、委譲によりこの重複も解消する。

### 2. dry-run 検証の監査可能性に限界がある

- 検証が `d.privilegeManager != nil && d.privilegeManager.IsPrivilegedExecutionSupported()` で囲まれている（[dryrun_manager.go:274](../../../internal/runner/resource/dryrun_manager.go)）。setuid 設定のない開発機や CI では `WithPrivileges` が呼ばれず、出力は `[WARNING: User/Group privilege management not supported]` のみになる。検証が行われないこと自体は出力に現れるが、検証結果は得られない。一方、ユーザー名・グループ名の解決は `user.Lookup` / `user.LookupGroup` と `u.GroupIds()` のみを用い、特権を必要としない。特権の有無で検証をスキップする根拠がない。
- 失敗内容は `Impact.Description` への文字列連結として現れるのみで（[dryrun_manager.go:288](../../../internal/runner/resource/dryrun_manager.go)）、`slog` の構造化属性にはならない。ログを機械処理する監視には向かない。

### 3. 委譲後に到達不能となる dry-run 専用の特権経路

dry-run の解決を `ResolveRunAsIdent` に委譲すると、`privilegeManager` を経由する必要がなくなる。その結果、`Operation` 種別 `OperationUserGroupDryRun` は本番の呼び出し元を失い、以下がいずれも到達不能となる。

- [config.go:162](../../../internal/runner/base/runnertypes/config.go) の `OperationUserGroupDryRun` 定義
- [unix.go:152](../../../internal/runner/base/privilege/unix.go) `prepareExecution` の `case`
- [unix.go:172-176](../../../internal/runner/base/privilege/unix.go) `performElevation` の dry-run 分岐と `resolveUserGroupForDryRun` 呼び出し
- [unix.go:224-225](../../../internal/runner/base/privilege/unix.go) `restorePrivilegesAndMetrics` の dry-run 専用 metrics 記録分岐
- [testutil/mocks.go:39](../../../internal/runner/base/privilege/testutil/mocks.go) の対応 `case`

0157 の設計原則（到達不能コードを残さない）と一致させるため、本タスクでこれらを削除する（「方針判断の記録」2 参照）。

## 目的

- dry-run のユーザー・グループ検証を、実行時が行う検査と**同一の判定**にする。dry-run が `validated` と報告した構成で実行時が識別情報の解決に失敗することがない状態にする。
- 識別情報の解決を `risktypes.ResolveRunAsIdent` に一元化し、privilege パッケージから独自実装を取り除く（DRY）。
- 検証を特権サポートの有無に依存させず、開発機・CI でも検証結果が得られる状態にする。
- 検証失敗を `slog` の構造化属性として出力し、機械処理可能な監査ログにする。
- 委譲によって到達不能になる dry-run 専用の特権経路を削除する。
- 上記に伴う dry-run の判定結果の変化を、意図した変更として受入基準で固定する。

## スコープ

### 対象（本タスクで対応する）

1. dry-run のユーザー・グループ解決を `risktypes.ResolveRunAsIdent` へ委譲し、補助グループの列挙を含む実行時と同一の検査にする（F-001）。
2. 補助グループ列挙の失敗（`Groups == nil`）に対する fail-closed 判定を dry-run と実行時で共有する（F-001）。
3. 特権サポートの有無に依存せず検証を実行する（F-002）。
4. 検証失敗を `slog` の構造化属性として出力する（F-003）。
5. `resolveUserGroupForDryRun` と、それに付随する dry-run 専用の特権経路（`OperationUserGroupDryRun` を含む）の削除（F-004）。
6. `user.Lookup` の二重呼び出し（0149 監査 A1 L-1）の解消の確認（F-004）。
7. 挙動変更（dry-run の判定結果が変わり得ること）の受入基準の設定（F-005）。
8. 上記に伴うテストの追加・更新・削除、および設計文書の追随（F-005）。

### 対象外（本タスクでは対応しない）

- **dry-run の終了コードへの反映**: 現在、ユーザー・グループ検証の失敗は `Impact.SecurityRisk` を `high` に引き上げるが、`previewExitCode` の判定（`previewPolicyDeny` / `previewVerificationUnavailable`）には影響しない。検証失敗を deny 扱いとして終了コードに反映するかは、dry-run の deny 判定全体の設計に属する別論点であり、本タスクでは現状の扱いを維持する。
- **`Impact.Description` の書式の全面的な見直し**: 本タスクは検証結果に関する記述のみを扱う。他の解析項目の記述形式は変更しない。
- **リスク評価器のゾーニング判定における `IdentityUnresolved` の扱い**: 名前解決失敗をゾーニングでどう扱うかは 0142 系の設計判断であり、本タスクでは変更しない。本タスクが追加するのは dry-run 側の明示的な検証である。
- **`ResolveRunAsIdent` 自体の契約変更**: 補助グループ列挙失敗時に `nil` を返す（エラーを返さない）という現在の契約は変更しない。fail-closed 判定は呼び出し側で行い、その判定を dry-run と実行時で共有する。契約自体をエラー返却に変えるかは、`Groups == nil` を「補助グループなし」と解釈する呼び出し元が将来現れた場合を含めた別検討とする。
- **`privilege` パッケージの他の所見（A1 L-2 / L-3 / L-4）**: 本タスクの対象外。

## 方針判断の記録

Issue #918 が「決める」としていた 2 点について、以下の方針を採る。

### 1. 特権サポートがない環境での検証（スキップ条件の扱い）

**方針: 特権サポートの有無に関わらず、常に解決・検証を行う。**

`ResolveRunAsIdent` は `user.Lookup` / `user.LookupGroup` / `u.GroupIds()` のみを用い、プロセスの識別情報を変更しない。特権を必要としないため、`IsPrivilegedExecutionSupported()` で囲む根拠がない。常に実行することで、setuid 設定のない開発機・CI でも検証結果が得られ、背景 2 の監査可能性の限界が同時に解消する。

`[WARNING: User/Group privilege management not supported]` は「実行時に特権昇格ができない」という別個の事実を伝えるものであり、検証結果とは独立したメッセージとして引き続き出力する（AC-07）。

### 2. `OperationUserGroupDryRun` 経路の扱い

**方針: 本タスクで削除する。**

委譲後、この operation は本番の呼び出し元を持たない（背景 3）。0157 が「到達不能コードを残さない」方針で降格パスを削除した直後に、同じ性質のコードを新たに作ることは避ける。削除に伴い dry-run 時の `RecordElevationSuccess`（metrics）記録が失われるが、dry-run では実際に特権昇格が発生しないため、昇格成功として計上することの意味は薄い。この挙動変更は AC-15 で明示的に受け入れる。

## 機能要件

### F-001: dry-run の識別情報解決を実行時と同一の判定にする

dry-run のユーザー・グループ検証を `risktypes.ResolveRunAsIdent` への委譲に置き換える。補助グループ列挙の失敗（`Groups == nil`）に対する fail-closed 判定は、executor が現在行っているもの（[executor.go:204-213](../../../internal/runner/base/executor/executor.go)）と同一の判定を用いる。同一の判定であることを構造的に保証するため、解決と fail-closed 判定をまとめた単一の関数を両経路が呼ぶ形にする（実装位置は設計文書で決定する）。

**Acceptance Criteria**:

- **AC-01**: `run_as_user` に解決できないユーザー名が指定されたコマンドの dry-run で、検証失敗が報告され `Impact.SecurityRisk` が `high` になる（既存挙動の維持）。
- **AC-02**: `run_as_group` に解決できないグループ名が指定されたコマンドの dry-run で、検証失敗が報告され `Impact.SecurityRisk` が `high` になる（既存挙動の維持）。
- **AC-03**: 補助グループの列挙が失敗する構成（`u.GroupIds()` が失敗し `Groups == nil` となる構成）で、dry-run が検証失敗を報告し `Impact.SecurityRisk` が `high` になる。
- **AC-04**: 同一の `run_as_user` / `run_as_group` の組に対し、dry-run の検証結果（成功/失敗）と実行時の識別情報解決の結果（成功/`ErrRunAsIdentityResolution`）が一致する。少なくとも「解決成功」「ユーザー名解決失敗」「グループ名解決失敗」「補助グループ列挙失敗」の 4 ケースについてテストで示す。
- **AC-05**: `run_as_group` が空で `run_as_user` のみが指定された場合、解決結果の GID はそのユーザーの primary group の GID となる（既存挙動の維持）。
- **AC-06**: `run_as_group` のみが指定された場合、UID は基準識別情報のもの、GID は指定グループのものとなる。基準識別情報は `risktypes.OriginalExecutionIdentity()`（実 UID / GID）であり、`syscall.Geteuid()` / `syscall.Getegid()` は用いない。
- **AC-07**: dry-run の検証はプロセスの識別情報（UID / GID / 補助グループ）を一切変更しない。

### F-002: 特権サポートに依存しない検証

検証を `privilegeManager` の有無・`IsPrivilegedExecutionSupported()` の値から切り離す。

**Acceptance Criteria**:

- **AC-08**: `privilegeManager` が `nil` の場合でも、`run_as_user` / `run_as_group` を持つコマンドの dry-run で検証が実行され、その結果が出力に現れる。
- **AC-09**: `IsPrivilegedExecutionSupported()` が `false` を返す場合でも、検証が実行され、その結果が出力に現れる。
- **AC-10**: 特権実行が利用できない環境（AC-08 / AC-09 の条件）では、検証結果とは別に「特権管理が利用できない」ことを示す警告が引き続き出力される。

### F-003: 構造化ログによる監査可能性

検証失敗の詳細を `slog` の構造化属性として出力する。

**Acceptance Criteria**:

- **AC-11**: 検証が失敗した場合、`slog` のレコードが 1 件出力され、少なくともコマンド名・`run_as_user`・`run_as_group`・失敗理由（error）が個別の属性として含まれる。属性値は文字列連結された 1 個のメッセージに埋め込まれていない。
- **AC-12**: 検証が失敗した場合、`Impact.Description` には検証が失敗したことを示す記述が引き続き含まれる（人間可読な dry-run 出力からの後退がない）。
- **AC-13**: 出力される属性に、環境変数値などの機密値が含まれない。

### F-004: 委譲により不要となるコードの削除

**Acceptance Criteria**:

- **AC-14**: `resolveUserGroupForDryRun` および `buildUserGroupLogAttrs`（同関数専用のヘルパである場合）がリポジトリから削除され、参照が残らない（静的検査）。
- **AC-15**: `OperationUserGroupDryRun` とその分岐（`prepareExecution` の `case`、`performElevation` の dry-run 分岐、`restorePrivilegesAndMetrics` の dry-run 専用 metrics 記録分岐、モックの `case`）が削除され、参照が残らない（静的検査）。これに伴い dry-run 実行時に `RecordElevationSuccess` が記録されなくなる。
- **AC-16**: 削除後、`WithPrivileges` に未知の `Operation` を渡すと `ErrUnsupportedOperationType` が返る（既存の fail-closed 挙動の維持）。
- **AC-17**: dry-run のユーザー・グループ検証において、同一ユーザー名に対する `user.Lookup` の呼び出しが 1 回に減る（0149 監査 A1 L-1 の解消）。
- **AC-18**: privilege パッケージに、識別情報の解決を独自に行うコードが残らない（`user.Lookup` / `user.LookupGroup` による run-as 解決が `risktypes` 側にのみ存在する）（静的検査）。

### F-005: 挙動変更の受入と文書の追随

本タスクは dry-run の判定結果を変え得る。変化の範囲を明示し、テストで固定する。

**Acceptance Criteria**:

- **AC-19**: 補助グループ列挙が失敗する構成において、変更前は `validated` と報告されていた dry-run が、変更後は検証失敗として `high` を報告する（意図した変更であることをテストで固定する）。
- **AC-20**: 上記以外の入力について、dry-run の出力する risk level と終了コードが変更前と一致する（`run_as_user` / `run_as_group` を持たないコマンド、および解決が成功するコマンド）。
- **AC-21**: 変更後も dry-run は一切のコマンドを実行せず、ファイルシステムとプロセス識別情報を変更しない（0147 / 0148 が固定した dry-run の副作用に関する契約の維持）。
- **AC-22**: 0157 の [02_architecture.md](../0157_dead_code_naming_cleanup/02_architecture.md) §2.2.2 / §5.6 / §9 が「別タスクで対応する」と記述している乖離が解消されたことを、本タスクの設計文書に記録する（静的検査）。
- **AC-23**: `docs/dev/architecture_design/` 配下の文書に dry-run のユーザー・グループ検証または `OperationUserGroupDryRun` に関する記述がある場合、変更後の実装に合わせて更新する（日本語版・英語版の双方）（静的検査）。

## 非機能要件

- **セキュリティ**: 本タスクの変更は fail-closed の方向のみである（dry-run が検知する失敗が増える）。dry-run が新たに特権を要求することはなく、プロセスの識別情報を変更する経路を追加しない。
- **性能**: dry-run 1 コマンドあたりの `user.Lookup` 呼び出し回数は増えない（AC-17 により減る）。
- **保守性**: run-as 識別情報の解決実装をリポジトリ内で 1 つにする。
- **後方互換性**: 設定ファイル形式・CLI オプションに変更はない。dry-run 出力のテキストは変わり得る（AC-12 / AC-19）。

## リスク

| リスク | 影響 | 緩和策 |
|---|---|---|
| 補助グループ列挙の失敗を再現するテストが書きにくい | AC-03 / AC-04 / AC-19 が検証できない | 解決関数を注入可能にする（executor は既に `runAsResolver` を注入可能にしている）。実際の OS 状態に依存しないテストとする |
| 検証を常時実行することで、これまで検証されていなかった既存設定が dry-run で `high` を報告し始める | 運用中の設定に対する dry-run 出力が変わる | 変化は「実行時に失敗する構成を dry-run が事前に報告する」方向のみであることを AC-19 / AC-20 で明示し、変更内容として文書化する |
| `OperationUserGroupDryRun` 削除により metrics の系列が消える | dry-run の metrics を参照している運用がある場合に影響 | AC-15 で明示的な受入項目とし、レビューで確認する |
