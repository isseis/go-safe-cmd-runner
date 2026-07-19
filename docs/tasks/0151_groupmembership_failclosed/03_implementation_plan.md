# 実装計画書: groupmembership グループメンバー列挙 fail-closed 是正

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-19 |
| Review date | 2026-07-19 |
| Reviewer | isseis |
| Comments | - |

本書は [`01_requirements.md`](01_requirements.md)（status: `approved`）と
[`02_architecture.md`](02_architecture.md)（status: `approved`）に基づき、
実装作業を追跡可能なタスクへ分解したものである。設計判断の詳細・Mermaid 図は
`02_architecture.md` を参照し、本書では重複記載しない。

---

## 1. 実装概要

### 1.1 目的

`internal/groupmembership` におけるグループメンバー列挙の fail-open 構造的リスク
（H-1: CGO 版のエラー握りつぶし、M-2: CGO 版・非 CGO 版の意味論不一致、
M-1: `isUserOnlyGroupMember` の特例分岐）を、02_architecture.md の設計に従って解消する。

### 1.2 実装方針

- 02_architecture.md §8 の実装優先順位（Phase 1〜4）にそのまま従う。各 Phase 完了時点で
  `make test`（CGO_ENABLED=1/0 の両方）と `make lint` がグリーンであることを維持する。
- 公開 API（`GetGroupMembers` / `IsUserInGroup` / `CanUserSafelyWriteFile` 等）のシグネチャは
  変更しない。
- C 境界の既存防御パターン（`unsafe.Slice`、`validateGroupMemberCount`、`free_string_array`）
  は踏襲し、重複実装は作らない（DRY）。

### 1.3 既存コード調査結果

対象ファイルと変更点は次の通り。すべて `internal/groupmembership` パッケージ内に閉じる
（新規パッケージは追加しない）。

| ファイル | 現状 | 変更内容 |
|---|---|---|
| `internal/groupmembership/membership_cgo.go` | `get_group_members` が NULL 一括返却（未存在・エラー・確保失敗を区別しない）。`getGroupMembers` は `members == nil` の場合に常に `([]string{}, nil)` を返す（[membership_cgo.go:38-43,125-127](../../../internal/groupmembership/membership_cgo.go#L38-L43)）。プライマリメンバー列挙は行わない。 | 02_architecture.md §3.1〜§3.4 の三値契約・ERANGE リトライ・プライマリメンバー列挙 C 関数・Go 側和集合マージを実装する。 |
| `internal/groupmembership/manager.go` | `isUserOnlyGroupMember`（[manager.go:166-197](../../../internal/groupmembership/manager.go#L166-L197)）がプライマリ GID 条件分岐を持つ。`GroupMembership.GetGroupMembers`（[manager.go:85-123](../../../internal/groupmembership/manager.go#L85-L123)）はビルド別パッケージ関数 `getGroupMembers` を直接呼んでおり、テストから失敗を注入できない。 | 02_architecture.md §3.5 の列挙シーム（`enumerateGroupMembers` フィールド）導入、特例分岐削除、`ErrGroupMemberEnumeration` センチネル追加。 |
| `internal/groupmembership/membership_nocgo.go` | `groupEntry`・`parseGroupLine`・`parsePasswdLine`・`findGroupByGID`・`findUsersWithPrimaryGID`（[membership_nocgo.go:60-167](../../../internal/groupmembership/membership_nocgo.go#L60-L167)）が `//go:build !cgo` に閉じており CGO ビルドから参照できない。 | 02_architecture.md §3.6 に従い、これらのヘルパを新規 `membership_files.go`（`//go:build !cgo <code>&#124;&#124;</code> test`）へ移動する。ロジック変更なし（移動のみ）。 |
| `internal/groupmembership/membership_common_test.go` | `TestGetGroupMembers_InvalidGID_Common`（AC-05 の正常系）、`TestGetGroupMembers_Common` | 変更不要（02_architecture.md §7.4 のとおり）。 |
| `internal/groupmembership/membership_nocgo_test.go` | `TestParseGroupLine`・`TestParsePasswdLine` が移動対象シンボルを直接呼ぶ。`TestFindGroupByGID`・`TestFindUsersWithPrimaryGID` はテスト内ローカル実装（`testFindGroupByGID`・`testFindUsersWithPrimaryGID`）を使用するが、これらのローカル実装自体は内部で移動対象の `parseGroupLine`・`parsePasswdLine` を呼んでいる。`TestFileReadingErrors` のみ、ローカル実装がパース処理前に返るため移動対象シンボルに依存しない。 | 変更不要。理由は「移動対象シンボルに依存しない」ことではなく、`!cgo` タグは `!cgo \|\| test` に包含されるため、移動対象シンボルは移動後も `!cgo` ビルドから引き続き参照可能であること。 |
| `internal/groupmembership/manager_test.go` | `isUserOnlyGroupMember` の特例分岐に依存するコメント（macOS `staff` グループスキップ等、[manager_test.go:319-323,347-351,383-387,548-552,560-564](../../../internal/groupmembership/manager_test.go#L319-L323)）。列挙失敗を注入する既存の仕組みはない。 | AC-08/09/10/11/12/13 の新規テストを追加。既存 macOS スキップコメントは 02_architecture.md §7.4 のとおり結論が変わらないため変更不要。 |
| `internal/groupmembership/membership_cgo_test.go` | `TestValidateGroupMemberCount` のみ。ERANGE・確保失敗・プライマリメンバーを注入するテストはない（現行コードに失敗を決定的に起こす手段がないため）。 | AC-01〜AC-04, AC-06 の新規テストを追加。 |
| `internal/groupmembership/membership_semantics_test.go` | 存在しない。 | 新規作成（`//go:build cgo && test`）。AC-07 の意味論等価テスト。 |
| `internal/groupmembership/test_helpers.go` | 存在しない。 | 新規作成（`//go:build test`）。列挙シームを差し替えるテスト専用コンストラクタ `newWithEnumerator`。 |
| `internal/groupmembership/validate_permissions_test.go` | 権限ビット検査（本タスク対象外） | 変更不要。 |
| `internal/runner/base/security/file_validation.go` | `Validator.isUserInGroup`（[file_validation.go:318-346](../../../internal/runner/base/security/file_validation.go#L318-L346)）が `GetGroupMembers` のエラーをそのまま伝播する構造は現行どおり。 | 呼び出し元の変更なし（02_architecture.md §2.1「対象外」のとおり）。AC-13 相当の非回帰は `groupmembership` パッケージ内の `IsUserInGroup` テストで検証し、`file_validation.go` 側の追加テストは不要（構造が同型であることは 02_architecture.md §6.3 で確認済み）。 |

既存資産の再利用状況: `unsafe.Slice`・`validateGroupMemberCount`・`free_string_array` は
そのまま踏襲する。`/etc/group`・`/etc/passwd` のパース処理は新規実装せず、移動のみで
CGO ビルドの意味論等価テストから再利用する。

---

## 2. 実装ステップ

### Phase 1: F-001（H-1、AC-01〜AC-05）— フェイルクローズド化

対応する設計: 02_architecture.md §3.1, §3.2, §3.5（列挙シームの導入部分）, §4.2

**対象ファイル**: `internal/groupmembership/membership_cgo.go`, `internal/groupmembership/manager.go`, `internal/groupmembership/membership_cgo_test.go`

作業内容:

- [x] `membership_cgo.go` の C 関数 `get_group_members` を、02_architecture.md §3.1 の三値契約
      （シグニチャへの `int* err_out`, `size_t buf_initial`, `size_t buf_max` 追加、
      `getgrgid_r` の戻り値の四分岐処理、`gr_mem` が NULL/空でも成功として扱う境界規定）どおりに
      変更する。
  - [x] `s == ERANGE` の場合、**確保済みの旧バッファを `free` してから**バッファを 2 倍に拡大して
        再試行する（旧バッファの解放漏れによるメモリリークを防ぐ）。拡大後のサイズが `buf_max` を
        超える場合は `*err_out = ERANGE` としてエラー終了する（無限リトライ・無制限確保をしない）。
- [x] Go 側に非公開ラッパ `getExplicitGroupMembers(gid uint32) (members []string, found bool, err error)`
      を追加する。C の三値契約を Go の `(members, found, err)` へ写像する（02_architecture.md §3.1）。
      C 側エラーは `fmt.Errorf("%w: gid %d: ...", ErrGroupMemberEnumeration, gid, ...)` で
      ラップする。既存の `validateGroupMemberCount`・`free_string_array` の呼び出しパターン
      （count を検証してから defer 登録する順序）を踏襲する。
- [x] 非公開パッケージ変数を追加する（02_architecture.md §3.2）。
  - [x] `var grBufferInitialSize int`（0 の場合は `sysconf(_SC_GETGR_R_SIZE_MAX)`、
        取得不可なら 16384 を使う）
  - [x] `var grBufferMaxSize = 4 * 1024 * 1024`
  - [x] `getExplicitGroupMembers` は呼び出しのたびにこれらを読み、C 関数へ `buf_initial`/`buf_max`
        として渡す。
- [x] `manager.go` に `ErrGroupMemberEnumeration` センチネルを追加する（ビルドタグなしファイルに
      配置し、CGO・非 CGO 両ビルドから参照可能にする。02_architecture.md §4.2）。
  ```go
  // ErrGroupMemberEnumeration is returned when group member enumeration fails
  // due to NSS errors, buffer limit exhaustion, or memory allocation failure.
  var ErrGroupMemberEnumeration = errors.New("group member enumeration failed")
  ```
- [x] `manager.go` の `GroupMembership` 構造体に非公開フィールド
      `enumerateGroupMembers func(gid uint32) ([]string, error)` を追加し、`New()` で
      パッケージ関数 `getGroupMembers`（ビルド別実装）を設定する（02_architecture.md §3.5）。
- [x] `GroupMembership.GetGroupMembers`（[manager.go:111](../../../internal/groupmembership/manager.go#L111)）
      の呼び出しをパッケージ関数 `getGroupMembers(gid)` から `gm.enumerateGroupMembers(gid)` へ
      変更する。
- [x] この時点の CGO 版 `getGroupMembers(gid uint32) ([]string, error)` は、
      `getExplicitGroupMembers` を呼び出し、`err != nil` なら `(nil, err)`、
      `found == false` なら `([]string{}, nil)`、それ以外は明示メンバーをそのまま返す
      （プライマリメンバー列挙は Phase 2 で追加するため、この時点では未実施）。

テスト:

- [x] `membership_cgo_test.go` に `TestGetExplicitGroupMembers_ERANGERetrySucceeds` を追加する
- [x] `membership_cgo_test.go` に `TestGetExplicitGroupMembers_ERANGERetryExceedsLimit` を追加する
- [x] `membership_cgo_test.go` に `TestGetExplicitGroupMembers_AllocationFailure` を追加する
- [x] `membership_cgo_test.go` に `TestGetExplicitGroupMembers_InvalidGID` を追加する
- [x] 上記に加え、`TestGetGroupMembers_InvalidGID_Common`
      （既存、`membership_common_test.go`、変更なし）の「未存在 → `([]string{}, nil)`」により、
      ラッパー `getGroupMembers` レベルでも同じ区別が保たれることを確認する（AC-01, AC-05）。

完了基準: `make fmt && make test && make lint` が CGO_ENABLED=1/0 の両方でグリーン。
この時点で列挙失敗は `(nil, error)` となるが、`isUserOnlyGroupMember` の特例分岐は
まだ残っているため、共有プライマリグループ環境での fail-open は Phase 3 まで残る
（02_architecture.md §8 のとおり、意図した中間状態）。

> **PR-1 に列挙シームを含める理由**: `GetGroupMembers` が呼び出す CGO 版
> `getGroupMembers` の内部実装を三値契約へ変更するにあたり、`GroupMembership` が
> パッケージ関数を直接呼ぶ経路からフィールド経由の間接呼び出しへ移行する必要がある。
> 列挙シーム（`enumerateGroupMembers` フィールド）の導入は 1 フィールドの追加と
> 1 行の呼び出し変更という機械的な変更であり、三値契約化と同時に適用しないと
> C 側変更後の `getGroupMembers` を `GetGroupMembers` から到達不能になる。
> この結合は構造的に必然であり、分離すると中間コミットで経路が不整合になる。

### PR-1 作成ポイント: CGO fail-closed and enumeration seam

**対象ステップ**: Phase 1（C 関数三値契約化 / ERANGE リトライ / バッファ境界変数 / `ErrGroupMemberEnumeration` センチネル / 列挙シーム導入 / CGO ビルド契約テスト）

**推奨タイトル**: `feat(0151): implement CGO fail-closed three-value contract with ERANGE retry`

**レビュー観点**: C 側 `getgrgid_r` 戻り値の四分岐分類が三値契約どおりであるか / ERANGE リトライ時の旧バッファ解放漏れがないか / `buf_max` 上限到達で無限リトライしないか / Go 側 `getExplicitGroupMembers` が C の三値を正しく写像しているか

**実装モデル要件**: frontier-required

**判定理由**: C 境界の三値契約（`err_out` out-param による未存在・エラー・成功の三分類）は本コードベースで前例のない設計判断であり、C メモリ管理・バッファオーバーフロー防止を含む高リスク変更であるため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 2: F-002（M-2、AC-06, AC-07）— 意味論統一

対応する設計: 02_architecture.md §3.3, §3.4, §3.6

**対象ファイル**: `internal/groupmembership/membership_cgo.go`, `internal/groupmembership/membership_files.go`（新規）, `internal/groupmembership/membership_nocgo.go`, `internal/groupmembership/membership_cgo_test.go`, `internal/groupmembership/membership_semantics_test.go`（新規）

作業内容:

- [x] `membership_cgo.go` に C 関数 `get_users_with_primary_gid` を、02_architecture.md §3.3 の
      契約（`setpwent`→`getpwent`ループ→`endpwent` を 1 回の C 呼び出し内で完結、
      `errno` によるループ終端とエラーの区別、`pw_gid` 一致時の即時 `strdup`、
      `malloc`/`strdup` 失敗時の `ENOMEM`、一致 0 人は「成功・`*count_out = 0`」、
      エラーパスを含め `endpwent` を必ず対で呼ぶ）どおりに実装する。
- [x] パッケージレベルの `sync.Mutex` を `membership_cgo.go` に追加し、
      `get_users_with_primary_gid` の呼び出し全体（C 呼び出し前後、Go 側マージ前まで）を
      シリアライズする。`getGroupMembers`（CGO 版）内部でこのミューテックスを取得・解放し、
      呼び出し元はミューテックスを意識しない。ロック順序は
      「`GroupMembership.cacheMutex` → 列挙ミューテックス」の一方向に固定し、
      コード内コメントで明記する（逆順取得の禁止。02_architecture.md §3.3）。
  - [x] `getGroupMembers` の Go 実装は本パッケージ内で唯一の
        `setpwent`/`getpwent`/`endpwent` 呼び出し元であるという不変条件を、
        関数コメントに明記する（02_architecture.md §3.3, §5.4）。
- [x] Go 側非公開ラッパ `getUsersWithPrimaryGID(gid uint32) ([]string, error)` を追加し、
      三値契約の中間状態（未存在）を持たない二値契約とする。`getExplicitGroupMembers` と同じ
      C 境界の防御パターン（`validateGroupMemberCount` によるカウント検証 → 検証後に
      `defer free_string_array` 登録 → `unsafe.Slice` による変換）を適用する
      （02_architecture.md §1.1 の原則 4「既存資産の再利用」は `get_group_members` に限らず
      すべての C 境界関数に適用される）。
- [x] CGO 版 `getGroupMembers(gid uint32) ([]string, error)` を、02_architecture.md §3.4 の
      契約へ更新する。
  - [x] `getExplicitGroupMembers` がエラーなら `(nil, error)`。
  - [x] `found == false`（未存在）なら `getUsersWithPrimaryGID` を呼ばずに `([], nil)` を返す
        （非 CGO 版の早期リターンに合わせる境界確定。AC-07 の前提）。
  - [x] 存在する場合は `getUsersWithPrimaryGID` を呼び、エラーなら `(nil, error)`。
   - [x] マージ処理を非公開関数 `mergeGroupMembers(explicit, primary []string) ([]string, error)`
          として `membership_cgo.go` に切り出す。明示メンバーとプライマリメンバーを
         `map[string]struct{}` で和集合マージし
        （重複除去、順序保証なし）、マージ後の件数を `validateGroupMemberCount` で再検証する
        （02_architecture.md §5.5「件数上限」。マージにより単独の入力では上限を超えない
        2 つの集合が、合算後に `maxGroupMembers` を超えるケースがあるため、個々の列挙結果と
        マージ後の最終集合の双方を検証する）。関数として切り出すことで、実 NSS では再現困難な
        マージ後件数超過（下記テスト参照）を決定的にテストできる。`getGroupMembers` はこの
        `mergeGroupMembers` を呼び出す薄い実装とする。
- [x] 共有パースヘルパを新規ファイル `membership_files.go`（`//go:build !cgo || test`）へ移動する
      （02_architecture.md §3.6）。ロジック変更は行わない。
  - [x] `groupEntry` 型を移動する。
  - [x] `parseGroupLine` を移動する。
  - [x] `parsePasswdLine` を移動する。
  - [x] `findGroupByGID` を移動する。
  - [x] `findUsersWithPrimaryGID` を移動する。
  - [x] `membership_nocgo.go` からこれら 5 シンボルの定義を削除し、`getGroupMembers` の実装
        （呼び出しロジック）のみを残す（振る舞い不変）。

テスト:

- [x] `membership_cgo_test.go` に `TestGetGroupMembers_IncludesPrimaryGroupMembers` を追加する
      （CGO ビルド）。実行ユーザーのプライマリ GID を `getGroupMembers` で列挙し、結果に
      実行ユーザー名が含まれることを確認する（AC-06）。`user.Current()` が失敗する場合、または
      実行ユーザーのプライマリ GID に対応するグループエントリが存在しない場合は
      `t.Skip` する（02_architecture.md §7.1 の境界ケース。実装の欠陥ではない）。
- [x] `membership_cgo_test.go` に `TestGetGroupMembers_MergedCountExceedsMaximum` を追加する
      （CGO ビルド、AC-06 に付随する境界値テスト）。`getExplicitGroupMembers` と
      `getUsersWithPrimaryGID` はそれぞれ `maxGroupMembers` 以下だが、和集合では
      `maxGroupMembers` を超えるケースを決定的に再現する必要がある。実 NSS でこれを再現するのは
      非現実的である。そのため、`getGroupMembers` 内のマージ処理を独立関数
      `mergeGroupMembers(explicit, primary []string) ([]string, error)` として切り出す
      （`validateGroupMemberCount` をマージ後の件数に適用する処理を含む）。本テストはこの
      独立関数に対して、要素数の合計が `maxGroupMembers` を超える 2 つのスライス（重複なし）を
      渡して `(nil, error)`・`errors.Is(err, ErrGroupMemberCountExceedsMax)` を確認する。
      `getGroupMembers` はこの `mergeGroupMembers` を呼び出すだけの薄い実装とする。
- [x] 新規 `membership_semantics_test.go`（`//go:build cgo && test`）に
      `TestGetGroupMembers_CGOAndNoCGOSemanticsMatch` を追加する（AC-07）。
  - [x] スキップ判定（`/etc/nsswitch.conf` の `passwd`・`group` ソース確認、`runtime.GOOS`
        判定）は、テスト本体に埋め込まず、`membership_semantics_test.go` 内の非公開の純粋関数
        `shouldSkipSemanticsTest(nsswitchContent string, goos string) (skip bool, reason string)`
        として切り出す（分岐が 4 つ以上あるため、`test_organization.md` の
        「CI/変更検知ロジックはテスト可能な形に切り出す」思想に倣う）。
        本体テストはこの関数の戻り値に応じて `t.Skip(reason)` するだけにする。
  - [x] `shouldSkipSemanticsTest` の分岐（`/etc/nsswitch.conf` 不在 → 実行、
        `files`/`systemd` のみ → 実行、`files`・`systemd` 以外のソース（例: `sss`）を含む →
        スキップ、`goos == "darwin"` → 無条件スキップ）を表形式でカバーする単体テスト
        `TestShouldSkipSemanticsTest` を同ファイルに追加する（`cgo && test`。ファイル I/O を
        伴わない純粋関数のため、文字列を直接渡してテストする）。
  - [x] 本体テストは `/etc/group` に現れる全 GID について、CGO 版 `getGroupMembers` の結果と、
        `membership_files.go` のヘルパ（`findGroupByGID` → 存在すれば
        `findUsersWithPrimaryGID` と `groupEntry.members` の和集合）から計算した期待集合とを
        比較する（重複なし・順不同、`assert.ElementsMatch` 相当）。

完了基準: `make fmt && make test && make lint` が CGO_ENABLED=1/0 の両方でグリーン。
この時点で共有プライマリグループ環境の判定は拒否側へ変わる（意図したフェイルクローズド変更、
02_architecture.md §8）。`go test -run '^$' -tags test -race ./internal/groupmembership/...`
（CGO_ENABLED=1）と `go test -run '^$' -tags test ./internal/groupmembership/...`
（CGO_ENABLED=0）の両方で `membership_files.go`・`membership_semantics_test.go` が
問題なくコンパイルされることを確認する（ビルドタグ起因のコンパイルエラーを本 Phase 内で検出する）。

### PR-2 作成ポイント: semantic unification with primary member enumeration

**対象ステップ**: Phase 2（`get_users_with_primary_gid` C 関数実装 / `sync.Mutex` シリアライズ / Go 側和集合マージ / 共有パースヘルパの `membership_files.go` への移動 / 意味論等価テスト追加）

**推奨タイトル**: `feat(0151): unify CGO and non-CGO member semantics with primary GID enumeration`

**レビュー観点**: `getpwent` ループの `errno` による終端・エラー区別が正しいか / `malloc`/`strdup` 失敗が ENOMEM で伝播するか / `endpwent` が全パスで対呼び出しされるか / 和集合マージ後の `validateGroupMemberCount` 再検証が行われているか / ロック順序（キャッシュロック → 列挙ミューテックス）が一方向であるか

**実装モデル要件**: frontier-recommended

**判定理由**: `getpwent` 全走査 + `sync.Mutex` シリアライズは隔離された高リスク・複雑なステップ（並行性、C 呼び出しのスレッド安全性）であり、`getpwent_r` vs `getpwent` の競合アプローチが 02_architecture.md 付録に記録されているため。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 3: F-003（M-1、AC-08〜AC-11）— 特例分岐の削除

対応する設計: 02_architecture.md §3.5, §4.3

**対象ファイル**: `internal/groupmembership/manager.go`, `internal/groupmembership/test_helpers.go`（新規）, `internal/groupmembership/manager_test.go`

作業内容:

- [x] `manager.go` の `isUserOnlyGroupMember`（[manager.go:166-197](../../../internal/groupmembership/manager.go#L166-L197)）を、
      現在のプライマリ GID 条件分岐込みの実装から、次の簡素化後の実装へ全体を置き換える。

  変更前（[manager.go:166-197](../../../internal/groupmembership/manager.go#L166-L197)、要旨）:
  ```go
  func (gm *GroupMembership) isUserOnlyGroupMember(userUID int, groupGID uint32) (bool, error) {
      user, err := user.LookupId(strconv.Itoa(userUID))
      if err != nil {
          return false, fmt.Errorf("failed to lookup user for UID %d: %w", userUID, err)
      }
      userPrimaryGID, err := strconv.ParseUint(user.Gid, 10, 32)
      if err != nil {
          return false, fmt.Errorf("failed to parse user's primary GID %s: %w", user.Gid, err)
      }
      members, err := gm.GetGroupMembers(groupGID)
      if err != nil {
          return false, fmt.Errorf("failed to get group members for GID %d: %w", groupGID, err)
      }
      if uint32(userPrimaryGID) == groupGID {
          if len(members) == 0 {
              return true, nil // No explicit members, user is the only primary group member
          }
          return len(members) == 1 && members[0] == user.Username, nil
      }
      return len(members) == 1 && members[0] == user.Username, nil
  }
  ```

  変更後（02_architecture.md §3.5）:
  ```go
  func (gm *GroupMembership) isUserOnlyGroupMember(userUID int, groupGID uint32) (bool, error) {
      user, err := user.LookupId(strconv.Itoa(userUID))
      if err != nil {
          return false, fmt.Errorf("failed to lookup user for UID %d: %w", userUID, err)
      }
      members, err := gm.GetGroupMembers(groupGID)
      if err != nil {
          return false, fmt.Errorf("failed to get group members for GID %d: %w", groupGID, err)
      }
      return len(members) == 1 && members[0] == user.Username, nil
  }
  ```
  この変更により、`user.Gid` のパース（`userPrimaryGID` の取得）と、それに伴う
  デッドコードとなるエラー分岐が削除される。このエラー分岐は判定の許否に影響しない
  付随的なものであり、除去による判定結果への影響はない（01_requirements.md 「M-1」節）。
- [x] 新規 `test_helpers.go`（`//go:build test`）に、列挙シームを差し替えるテスト専用
      コンストラクタを追加する（`test_organization.md` Classification B: 非公開フィールドへの
      アクセスが必要なため package-internal helper とする）。
  ```go
  // newWithEnumerator creates a GroupMembership whose enumeration function is replaced,
  // for tests that need to inject enumeration successes or failures deterministically.
  func newWithEnumerator(fn func(gid uint32) ([]string, error)) *GroupMembership {
      gm := New()
      gm.enumerateGroupMembers = fn
      return gm
  }
  ```

テスト（いずれも CGO_ENABLED=1/0 の両ビルドで実行される。ビルドタグなしファイルのため）:

- [x] `manager_test.go` に `TestIsUserOnlyGroupMember_NoSpecialCasing` を追加する（AC-08, AC-10）。
      `os/user.Current()` で現在ユーザーの UID・ユーザー名を取得し、表形式で次を確認する。
      `GetGroupMembers` は GID 単位で 30 秒 TTL キャッシュする（[manager.go:85-123](../../../internal/groupmembership/manager.go#L85-L123)、変更なし）。
      そのため、**各表行は `newWithEnumerator` で新規に生成した別々の `GroupMembership` インスタンスを
      使うか、各行ごとに異なる GID を用いる**。同一インスタンス・同一 GID を複数行で使い回すと、
      1 行目の列挙結果がキャッシュされて後続行に誤って再利用され、特例分岐削除の検証にならない点に
      注意する。
      - 列挙結果 `[]`（空集合）→ `false`（**AC-08 の挙動変更点**: 旧実装ではこのケースが
        プライマリグループ判定と組み合わさり `true` になり得た）
      - 列挙結果 `[currentUser.Username]` → `true`（AC-10: 単一メンバーの正常系維持）
      - 列挙結果 `[currentUser.Username, "other-user"]` → `false`
      - 列挙結果 `["other-user"]` → `false`
- [x] `manager_test.go` に `TestIsUserOnlyGroupMember_EnumerationError` を追加する（AC-09）。
      `newWithEnumerator` で列挙関数が `(nil, someErr)` を返すよう固定し、
      `isUserOnlyGroupMember` が `(false, error)` を返すこと、返るエラーが `someErr` を
      `errors.Is` で包含することを確認する。
- [x] `manager_test.go` に `TestCanUserSafelyWriteFile_EnumerationError` を追加する（AC-09）。
      同様に列挙エラーを固定し、`CanUserSafelyWriteFile` の group-writable 分岐
      （所有者かつ `perm&0o020 != 0`）で書き込みが許可されない（`canWrite == false` かつ
      `err != nil`）ことを確認する。
- [x] `manager_test.go` に `TestGetGroupMembers_ErrorNotCached` を追加する（AC-11）。
      `newWithEnumerator` に、1 回目の呼び出しでは `(nil, error)`、2 回目以降は
      `([]string{"user"}, nil)` を返すクロージャ（呼び出し回数をカウントする）を設定する。
      - 1 回目の `GetGroupMembers` 呼び出しがエラーを返すこと、`GetCacheStats().TotalEntries`
        が 0 のままであることを確認する（エラー結果が格納されないこと）。
      - 2 回目の `GetGroupMembers` 呼び出しが成功し、列挙関数が再度呼ばれたこと（合計 2 回）、
        かつ `GetCacheStats().TotalEntries` が 1 になる（成功結果がキャッシュされる）ことを
        確認する。

完了基準: `make fmt && make test && make lint` が CGO_ENABLED=1/0 の両方でグリーン。
Phase 2 完了後は完全列挙環境で `userPrimaryGID == groupGID` かつ `len(members) == 0` が
成立し得ないため、特例分岐削除後も正常系（`TestCanUserSafelyWriteFile` 等の既存テスト）が
回帰なく通過する（02_architecture.md §8 Phase 3）。

### PR-3 作成ポイント: remove special-case branch in isUserOnlyGroupMember

**対象ステップ**: Phase 3（`isUserOnlyGroupMember` 特例分岐削除 / 列挙シームテストヘルパー `newWithEnumerator` 追加 / 判定・キャッシュテスト追加）

**推奨タイトル**: `feat(0151): remove primary-GID special-case in isUserOnlyGroupMember`

**レビュー観点**: 削除されるコードにデッドコードが残存しないか（`userPrimaryGID` 取得・パースの消し忘れ） / 列挙シーム経由テストが空集合 → `false` のフェイルクローズドを正しく確認しているか / エラー結果がキャッシュに格納されず再試行されることを確認しているか

**実装モデル要件**: standard

**判定理由**: 設計判断は 02_architecture.md で確定済みの純粋な Go コード簡素化であり、いずれのトリガーにも該当しないため。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 4: F-004（AC-12, AC-13）— 読み取り経路の確認

対応する設計: 02_architecture.md §6.3

**対象ファイル**: `internal/groupmembership/manager_test.go`

作業内容:

- [ ] Phase 1〜3 で `IsUserInGroup` 自体のロジックは変更していないため、本 Phase は新規テストの
      追加のみで完了する（02_architecture.md §6.3 の分析どおり、`IsUserInGroup` は
      プライマリ GID 一致判定と補助グループ集合判定を列挙結果より先に確定するため、
      AC-06 によるプライマリメンバーの追加は `IsUserInGroup` の判定結果を変えない）。

テスト（両ビルド）:

- [ ] `manager_test.go` に `TestIsUserInGroup_NoRegressionWithPrimaryMembers` を追加する
      （AC-12）。現在ユーザーの UID・プライマリ GID を用い、次を確認する。
      - `IsUserInGroup(currentUID, currentPrimaryGID)` が `true` を返す
        （プライマリ GID 一致判定で確定するため、列挙結果には依存しない）。
      - 実行ユーザーが所属しない GID（例: `99999`）に対しては `false` を返す。
      - `newWithEnumerator` で列挙結果に現在ユーザーを含めた場合と含めない場合の両方で、
        上記いずれの結果も変化しないことを確認する（列挙結果の拡大が判定結果を変えないことの
        直接的な確認）。
- [ ] `manager_test.go` に `TestIsUserInGroup_EnumerationError` を追加する（AC-13）。
      プライマリ GID・補助グループのいずれにも一致しない GID に対し、`newWithEnumerator` で
      列挙エラーを固定し、`IsUserInGroup` が `(false, error)` を返すことを確認する。
- [ ] `manager_test.go` に `TestCanCurrentUserSafelyReadFile_EnumerationError` を追加する
      （AC-13）。列挙エラーを固定した `GroupMembership` に対し、group-writable なファイル
      （現在ユーザーが所属しない GID）の `CanCurrentUserSafelyReadFile` が読み取りを
      許可しない（`canRead == false` かつ `err != nil`）ことを確認する。

完了基準: `make fmt && make test && make lint` が CGO_ENABLED=1/0 の両方でグリーン。
全 AC（AC-01〜AC-13）に対応するテストが揃い、パッケージ全体のテストスイートが通過する。

### PR-4 作成ポイント: read-path regression and fail-closed verification

**対象ステップ**: Phase 4（`IsUserInGroup` 回帰テスト / 読み取り経路フェイルクローズドテスト追加）

**推奨タイトル**: `feat(0151): add read-path regression and fail-closed tests`

**レビュー観点**: プライマリメンバー追加が `IsUserInGroup` の判定結果を変えていないか / 列挙エラー時に読み取り経路が拒否側に倒れることを確認しているか / `CanCurrentUserSafelyReadFile` の group-writable 分岐がエラー伝播を正しく行っているか

**実装モデル要件**: standard

**判定理由**: テスト追加のみの変更であり、いずれのトリガーにも該当しないため。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

02_architecture.md §8 の連鎖（H-1 → M-2 → M-1 → 読み取り経路確認）に従い、Phase 1〜4 を
この順序で実装する。各 Phase の完了時点でテストがグリーンであることをマイルストーンとする。

### 3.1 マイルストーン

| マイルストーン | 内容 | 完了基準 |
|---|---|---|
| M1: Phase 1 完了 | CGO 版の三値契約・ERANGE リトライ・列挙シーム導入 | AC-01〜AC-05 のテストが通過し、`make test && make lint` がグリーン |
| M2: Phase 2 完了 | CGO 版・非 CGO 版の意味論統一 | AC-06, AC-07 のテストが通過し、両ビルドの `-tags test` コンパイルが通る |
| M3: Phase 3 完了 | `isUserOnlyGroupMember` の特例分岐削除 | AC-08〜AC-11 のテストが通過し、既存の正常系テストに回帰がない |
| M4: Phase 4 完了 | 読み取り経路の非回帰確認（本タスク完了） | AC-12, AC-13 のテストが通過し、全 AC のトレーサビリティが確定する |

Phase 間の依存関係: Phase 3 は Phase 2 の完了（完全列挙環境でプライマリメンバーが
列挙結果に現れること）を前提とする。この前提は Phase 3 のセクション内（本書 §2 Phase 3
「完了基準」）に明記し、Phase 番号の入れ替えは行わない。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1（全工程） | C 関数三値契約化、ERANGE リトライ（4 MiB 上限）、`ErrGroupMemberEnumeration` センチネル導入、列挙シーム（`enumerateGroupMembers` フィールド）導入、CGO ビルド契約テスト追加 | frontier-required |
| PR-2 | Phase 2（全工程） | `get_users_with_primary_gid` C 関数実装、`sync.Mutex` シリアライズ、Go 側和集合マージ（`mergeGroupMembers`）、共有パースヘルパの `membership_files.go` への移動、意味論等価テスト（`cgo && test`）追加 | frontier-recommended |
| PR-3 | Phase 3（全工程） | `isUserOnlyGroupMember` 特例分岐削除、列挙シームテストヘルパー `newWithEnumerator` 追加、判定・キャッシュテスト追加 | standard |
| PR-4 | Phase 4（全工程） | `IsUserInGroup` 回帰テスト、読み取り経路フェイルクローズドテスト追加 | standard |

---

## 4. テスト戦略

詳細な設計根拠は 02_architecture.md §7 を参照。本書では実行方法と網羅性のみ記す。

- **単体テスト**: すべて `internal/groupmembership` 内の `*_test.go` に配置する
  （新規ヘルパファイル `test_helpers.go` を除く）。`make test` は Linux で
  CGO_ENABLED=1（`-race` 付き）・CGO_ENABLED=0 の両方を実行するため、両ビルドの検証が
  自動化される。
- **統合テスト**: `membership_semantics_test.go`（`cgo && test`）は実 NSS・実ファイルを
  跨ぐ統合テストであり、files バックエンド環境（CI コンテナ想定）でのみ意味論等価性を
  検証する。ディレクトリ NSS 環境での等価性は 02_architecture.md §5.4 の残存リスクとして
  扱い、本タスクでは検証しない。
- **既存テストへの影響**: 02_architecture.md §7.4 のとおり、旧来の fail-open 挙動を直接
  検証する既存テストは存在しないため、更新すべき既存テストはない。
  `membership_common_test.go`・`membership_nocgo_test.go` は変更不要。
- **バッファ境界変数・列挙シームを差し替えるテストの規約**: `t.Parallel()` を使用しない、
  変更した値は `t.Cleanup` で必ず復元する、これらの変数・シームを差し替えるテストと列挙を
  並行実行するテストを同時に走らせない（02_architecture.md §3.2）。本書 §2 Phase 1・
  Phase 3 の該当テストすべてにこの規約を適用する。

---

## 5. リスク管理

| リスク | 影響 | 軽減策 |
|---|---|---|
| ERANGE・確保失敗の決定的なテスト環境依存性 | CI 環境によって `_SC_GETGR_R_SIZE_MAX` の値やメモリ制限が異なり、テストが不安定になる可能性 | バッファ境界変数をテストから直接差し替えることで、実環境の値に依存せず決定的に失敗を誘発する（02_architecture.md §3.2 で確定済みの設計） |
| AC-07 の意味論等価テストが CI 環境で常時スキップされる | ディレクトリ NSS 環境ではテストがスキップされ、AC-07 の検証が実質的に行われない可能性 | `/etc/nsswitch.conf` のスキップ判定を明示的にログ出力し、CI 環境（files バックエンド想定）では確実に実行されることを Phase 2 完了時に確認する |
| `getpwent` の非スレッドセーフ性に起因する潜在的なデータ競合 | パッケージレベルミューテックスの対象外の経路（`os/user` の内部バッファ共有等）が残存する | 02_architecture.md §5.4 に残存リスクとして明記済み。本タスクのスコープでは解消しない（別 Issue） |
| Phase 3 の特例分岐削除が正常系を壊す | 完全列挙環境の前提が崩れている環境（部分列挙 NSS）で、単一メンバーグループの判定が意図せず変わる | Phase 2 完了後の意味論統一によりこのリスクは限定的（02_architecture.md §8）。既存の `TestCanUserSafelyWriteFile` 系テストを Phase 3 完了時に再実行して回帰を確認する |

---

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: Phase 1 — CGO フェイルクローズド化）
- [ ] PR-2 マージ済み（対象ステップ: Phase 2 — 意味論統一）
- [ ] PR-3 マージ済み（対象ステップ: Phase 3 — 特例分岐削除）
- [ ] PR-4 マージ済み（対象ステップ: Phase 4 — 読み取り経路確認）
- [ ] `make fmt` が全変更ファイルに適用されている
- [ ] `make test`（CGO_ENABLED=1・0 の両方）がグリーンである
- [ ] `make lint` がグリーンである
- [ ] AC-01〜AC-13 のすべてに対応するテストが存在し、通過している
- [ ] 本書 §7 の受け入れ基準検証セクションが完成している

---

## 7. 受け入れ基準の検証（Acceptance Criteria Verification）

| AC | 内容 | 検証方法 | 種別 |
|---|---|---|---|
| AC-01 | 「未存在」と「エラー」の区別 | `internal/groupmembership/membership_cgo_test.go::TestGetExplicitGroupMembers_InvalidGID`（未存在 → `found==false, err==nil`）と `::TestGetExplicitGroupMembers_ERANGERetryExceedsLimit`（エラー → non-nil error）を同一関数 `getExplicitGroupMembers` の 2 状態として対比。加えて `internal/groupmembership/membership_common_test.go::TestGetGroupMembers_InvalidGID_Common`（既存）でラッパー `getGroupMembers` レベルでも区別が保たれることを確認する | test |
| AC-02 | ERANGE リトライ（上限付き） | `internal/groupmembership/membership_cgo_test.go::TestGetExplicitGroupMembers_ERANGERetrySucceeds`（拡大→成功、基準呼び出しとの結果一致を含む）と `::TestGetExplicitGroupMembers_ERANGERetryExceedsLimit`（上限到達→エラー） | test |
| AC-03 | `malloc`/`strdup` 失敗の伝播 | `internal/groupmembership/membership_cgo_test.go::TestGetExplicitGroupMembers_AllocationFailure` | test |
| AC-04 | エラー時に `(nil, non-nil error)` | `internal/groupmembership/membership_cgo_test.go::TestGetExplicitGroupMembers_ERANGERetryExceedsLimit`（`errors.Is(err, ErrGroupMemberEnumeration)` を確認） | test |
| AC-05 | 未存在時に `([]string{}, nil)`（正常系維持） | `internal/groupmembership/membership_common_test.go::TestGetGroupMembers_InvalidGID_Common`（既存、変更なし） | test |
| AC-06 | プライマリメンバーの含有（和集合）、マージ後件数上限の再検証 | `internal/groupmembership/membership_cgo_test.go::TestGetGroupMembers_IncludesPrimaryGroupMembers`（含有確認）と `::TestGetGroupMembers_MergedCountExceedsMaximum`（マージ後件数超過の境界値） | test |
| AC-07 | CGO 版・非 CGO 版の意味論等価性 | `internal/groupmembership/membership_semantics_test.go::TestGetGroupMembers_CGOAndNoCGOSemanticsMatch`（本体）と `::TestShouldSkipSemanticsTest`（スキップ判定ロジックの単体テスト） | test |
| AC-08 | 特例分岐の不在（`len(members)==1 && members[0]==user.Username` のみ） | `internal/groupmembership/manager_test.go::TestIsUserOnlyGroupMember_NoSpecialCasing`（空集合 → `false` を含む表形式テスト） | test |
| AC-09 | 列挙エラー時に `(false, error)`、書き込み不許可 | `internal/groupmembership/manager_test.go::TestIsUserOnlyGroupMember_EnumerationError` と `::TestCanUserSafelyWriteFile_EnumerationError` | test |
| AC-10 | 単一メンバーグループでの `true`（正常系回帰なし） | `internal/groupmembership/manager_test.go::TestIsUserOnlyGroupMember_NoSpecialCasing`（単一メンバーケース）および既存 `TestCanUserSafelyWriteFile` 系テスト（両ビルドで再実行） | test |
| AC-11 | 列挙エラーの非キャッシュ・再試行 | `internal/groupmembership/manager_test.go::TestGetGroupMembers_ErrorNotCached` | test |
| AC-12 | 読み取り経路の非回帰（プライマリメンバー追加の影響なし） | `internal/groupmembership/manager_test.go::TestIsUserInGroup_NoRegressionWithPrimaryMembers` | test |
| AC-13 | 読み取り経路のフェイルクローズド | `internal/groupmembership/manager_test.go::TestIsUserInGroup_EnumerationError` と `::TestCanCurrentUserSafelyReadFile_EnumerationError` | test |

---

## 8. クロスサーチチェックリスト

`make lint`・`make test` では検出できない、シンボル移動・削除に伴う残存参照を確認する。

- [ ] `rg -n "func getGroupMembers\(" internal/groupmembership/` を実行し、
      `membership_cgo.go` と `membership_nocgo.go` の 2 箇所にのみ定義が存在すること
      （ビルドタグによる排他選択が保たれていること）を確認する。
- [ ] `rg -n "groupEntry|parseGroupLine|parsePasswdLine|findGroupByGID|findUsersWithPrimaryGID" internal/groupmembership/membership_nocgo.go`
      を実行し、Phase 2 完了後にこれらのシンボル定義が `membership_nocgo.go` に
      残っていない（`membership_files.go` へ移動済みである）ことを確認する。
- [ ] `rg -n "userPrimaryGID" internal/groupmembership/manager.go` を実行し、
      Phase 3 完了後に `isUserOnlyGroupMember` 内の `userPrimaryGID` 変数への参照が
      残っていないことを確認する（`IsUserInGroup` 内の同名変数は別関数のものであり対象外）。
- [ ] `rg -n "gm\.enumerateGroupMembers|getGroupMembers\(gid\)" internal/groupmembership/manager.go`
      を実行し、`GroupMembership.GetGroupMembers` が `gm.enumerateGroupMembers` 経由で
      列挙関数を呼び出しており、パッケージ関数 `getGroupMembers` を直接呼ぶ経路が
      残っていないことを確認する。

---

## 9. Success Criteria

- **機能的完全性**: AC-01〜AC-13 のすべてに対応する実装とテストが揃っている。
- **品質指標**: `make test`（CGO_ENABLED=1/0 の両方、Linux）と `make lint` がグリーンである。
- **セキュリティ検証**: 02_architecture.md §7.3 のセキュリティテスト（AC-08 の空集合 → `false`、
  AC-09、AC-13 が旧来の fail-open 経路の回帰防止テストとして機能すること）が本書 §7 の
  テストで満たされている。
- **ドキュメント完全性**: 本書 §7 の受け入れ基準検証セクションが全 AC を網羅している。

---

## 10. 次のステップ

- 本書のレビューと `approved` への更新（人間レビュアーによる）。
- 承認後、02_architecture.md §8 の Phase 1〜4 の順に実装を開始する。
- 実装完了後、02_architecture.md §5.3「アップグレード時の恒久的な許可 → 拒否の反転」に
  記載した事前検知手順（保護対象パスの group-writable 構成確認）を運用ドキュメントとして
  周知する（本タスクのスコープ外の運用対応であり、実装完了後の後続タスクとする）。
