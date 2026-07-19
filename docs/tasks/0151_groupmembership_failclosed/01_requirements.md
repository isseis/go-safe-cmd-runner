# 要件定義書: groupmembership のグループメンバー列挙 fail-open 是正

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-19 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#858 [Security][H-1] groupmembership: getGroupMembers のエラー握りつぶしによる fail-open](https://github.com/isseis/go-safe-cmd-runner/issues/858)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の H-1, M-1, M-2
- 横断パターン: [#860 [Security][P1] エラー処理の縮退による fail-open パターンの横断修正](https://github.com/isseis/go-safe-cmd-runner/issues/860)（本タスクは groupmembership 分の H-1/M-1/M-2 を解消する。#860 が挙げる L-2/L-3 や他パッケージ分は本タスクの対象外）

## 背景

`internal/groupmembership` は `safefileio` 等が呼び出す、ハッシュファイル・設定ファイルの安全な読み書き判定に使われるセキュリティクリティカルなコンポーネントである。監査（D1_groupmembership.md）により、グループメンバー列挙の失敗系・意味論不一致が一貫して「メンバー 0 人」に縮退し、`isUserOnlyGroupMember` を経由して group-writable ファイルへの書き込み許可（fail-open）に到達する構造的リスクが指摘された。本タスクはこのうち以下 3 件をまとめて是正する。

- **H-1**: CGO 版 `getGroupMembers`（`membership_cgo.go`）が `getgrgid_r` のエラー（ERANGE・NSS 障害・EINTR 等）と「グループが存在しない」を区別せず、いずれも `([]string{}, nil)` として返している。
- **M-1**: `isUserOnlyGroupMember`（`manager.go:185-193`）が「プライマリグループについて明示メンバーが 0 人ならユーザーが唯一のメンバー」と仮定しており、複数ユーザーが同一プライマリグループを共有する環境（LDAP 等）で成立しない。
- **M-2**: CGO 版（`gr_mem` のみ）と非 CGO 版（明示メンバー + プライマリ GID 一致ユーザーの和集合）とで `getGroupMembers` の意味論が異なり、`CGO_ENABLED` の設定次第でセキュリティ判定結果が変わる。

3 件は監査の総評で「同一の fail-open 構造的リスクに合流する」とされており、以下の関係で連鎖的に是正できる。ただし各件を修正する変更は独立しているため、要件・受け入れ基準としては H-1／M-2／M-1 を分離して定義する。

- **H-1（fail-closed 化）**: CGO 版 C 関数で「見つからない」と「エラー」を区別し、エラー時は必ず non-nil error を Go 側へ伝播させる。これが H-1 を直接是正する中核変更である（意味論統一とは独立）。
- **M-2（意味論統一）**: CGO 版の列挙結果を非 CGO 版と同じ意味論（明示メンバー + プライマリ GID 一致ユーザーの和集合）に揃える。これにより CGO 版でもプライマリメンバーが列挙結果に含まれる。
- **M-1（特例分岐の削除）**: M-2 によって「プライマリグループの唯一のメンバーは列挙結果に本人 1 名として現れる」ことが両ビルドで保証されるため、`isUserOnlyGroupMember` の特例分岐（「明示メンバー 0 人 → 唯一のメンバー」）は不要になり削除できる。削除しても正常系が壊れないのは、H-1 の fail-closed 化により「列挙失敗が空集合に化ける」経路が閉じているためである（＝空集合はもはや失敗を意味しない）。

## 目的

- グループメンバー列挙が失敗した場合に、書き込み安全性判定が「安全（fail-open）」側に縮退しないことを保証する（fail-closed）。
- CGO 版・非 CGO 版で `getGroupMembers` が返す集合の意味論を一致させ、ビルド構成によってセキュリティ判定結果が変わらないようにする。
- `isUserOnlyGroupMember` が「メンバー列挙失敗・空結果」を許可根拠に使わないようにする。

## スコープ

### 対象（本タスクで対応する）

- `internal/groupmembership/membership_cgo.go`: C 関数 `get_group_members` と Go 関数 `getGroupMembers`（CGO ビルド版）。
  - 「グループが見つからない」と「エラー」を区別し、エラー時は必ず non-nil error を返す。
  - `getgrgid_r` が `ERANGE` を返した場合のバッファ拡大リトライ。
  - `malloc`/`strdup` 失敗時のエラー伝播。
  - 返却するメンバー集合を非 CGO 版と同じ意味論（明示メンバー + プライマリ GID 一致ユーザーの和集合）に揃える。
- `internal/groupmembership/manager.go`: `isUserOnlyGroupMember` の「明示メンバー 0 人 → 唯一のメンバー」特例分岐の削除、および `groupmembership` パッケージ内の呼び出し元（`isUserOnlyGroupMember`／`IsUserInGroup` 経由の書き込み・読み取り判定）がエラー時に fail-closed になることの確認。
- 上記変更に対する CGO 版・非 CGO 版共通のユニット/意味論テスト。
- 旧来の fail-open 挙動（列挙失敗を空集合・エラーなしとして扱う）に依存する既存テストがあれば、新しい fail-closed 契約に合わせて更新する。なお、既存の `membership_common_test.go` は「存在しないグループ → 空集合・エラーなし」を検証しており（AC-05 で維持する正常系）、これは fail-closed 契約と両立するため変更不要である。

### 対象外（別タスク・別 Issue とする）

- M-3（`SUDO_UID` の無検証信頼）, M-4（`getProcessEUID` の命名/実装乖離）: 別 Issue で対応。
- L-1〜L-4, I-1, I-2: 別 Issue またはバックログで対応。
- `internal/security`, `internal/runner/base/security` など呼び出し元パッケージ内の仕様変更（本タスクは `groupmembership` パッケージ内の修正に閉じる。呼び出し元は現行の `(bool, error)` 契約を変えずに利用できる想定）。

## 現状の問題点（詳細）

1. **エラーと「見つからない」の混同（H-1）**: C 側 `get_group_members` は「グループが `/etc/group`（または NSS）に存在しない」「`getgrgid_r` のエラー（ERANGE・NSS/LDAP 障害・EINTR 等）」「`malloc`/`strdup` 失敗」のすべてを NULL 返却で表現しており、Go 側から区別できない（`membership_cgo.go:38-43, 52-57, 61-71`）。
2. **ERANGE の未リトライ（H-1）**: `getgrgid_r` は `sysconf(_SC_GETGR_R_SIZE_MAX)` で確保した固定バッファを 1 回使うのみで、バッファ不足（ERANGE）時にリトライしない。メンバー数の多いグループで現実的に発生し得る。
3. **プライマリグループの誤った空集合仮定（M-1）**: `isUserOnlyGroupMember` は「明示メンバー 0 人 → プライマリグループの唯一のメンバー」とみなす。共有プライマリグループ環境（例: 伝統的な `users` GID や LDAP で配布される共通プライマリ GID）では、`/etc/group` に他ユーザーが列挙されないため誤って許可される。
4. **ビルド依存の意味論不一致（M-2）**: 非 CGO 版はプライマリ GID 一致ユーザーを `/etc/passwd` から収集して明示メンバーと合算するのに対し、CGO 版は `gr_mem`（明示メンバーのみ）しか返さない。同一システムでも `CGO_ENABLED` によって `isUserOnlyGroupMember` の結果が変わる。

## 受け入れ基準（Acceptance Criteria）

#### F-001: CGO 版 `getGroupMembers` の fail-closed 化（H-1）

- **AC-01**: C 側は `getgrgid_r` が「グループが見つからない」（`result == NULL` かつ `s == 0`）と「エラー」（`s != 0`）を区別して Go 側に伝達する。
- **AC-02**: `getgrgid_r` が `ERANGE`（`s == ERANGE`）を返した場合、バッファサイズを拡大して再試行する。再試行には上限を設け、上限到達時はエラーとして扱う。
- **AC-03**: `malloc`/`strdup` の失敗はエラーとして Go 側に伝達される（グループが見つからない場合と区別可能である）。
- **AC-04**: Go 関数 `getGroupMembers`（CGO 版）は、C 側がエラーを報告した場合に `(nil, non-nil error)` を返す。
- **AC-05**: Go 関数 `getGroupMembers`（CGO 版）は、指定 GID のグループが存在しない場合（エラーではない）に `([]string{}, nil)` を返す（既存の正常系動作を維持する）。

#### F-002: CGO 版・非 CGO 版の意味論統一（M-2）

- **AC-06**: CGO 版 `getGroupMembers` は、明示メンバー（`gr_mem`）に加えて、指定 GID をプライマリ GID とするユーザーも結果に含める（和集合）。指定 GID を自身のプライマリ GID とする呼び出しユーザー本人も、この和集合に含まれる。
- **AC-07**: 同一の `/etc/group`・`/etc/passwd` 相当の入力に対し、CGO 版と非 CGO 版の `getGroupMembers` が同じメンバー集合（重複なし・順不同）を返すことをテストで確認できる。

#### F-003: `isUserOnlyGroupMember` の fail-closed 化（M-1）

- **AC-08**: `isUserOnlyGroupMember` は「明示メンバー 0 人 → ユーザーが唯一のメンバー」という特例分岐を持たない（F-002 によりプライマリメンバーが列挙結果に含まれるため、特例が不要になる）。これにより、`isUserOnlyGroupMember` 内のプライマリ GID 条件分岐（`manager.go` の `if uint32(userPrimaryGID) == groupGID` ブロック）を完全に削除し、単純なメンバー数とユーザー名の一致判定（`len(members) == 1 && members[0] == user.Username`）のみに簡素化する。本 AC は CGO 版・非 CGO 版の両ビルドで成立する。
- **AC-09**: `GetGroupMembers` がエラーを返した場合、`isUserOnlyGroupMember` は `(false, error)` を返し、`CanUserSafelyWriteFile` の group-writable 分岐は書き込みを許可しない（fail-closed）。
- **AC-10**: 単一ユーザーのみが所属するグループ（プライマリ・明示メンバーいずれの形でも）に対しては、従来どおり `isUserOnlyGroupMember` が `true` を返す（fail-closed 化・特例分岐削除による正常系の回帰がないことを、CGO 版・非 CGO 版の両ビルドで確認する）。
- **AC-11**: `GetGroupMembers` が列挙エラーを返した場合、その結果（空集合・エラー）はキャッシュに格納されず、後続の呼び出しは列挙を再試行する（一時的な失敗が最大 TTL の間キャッシュされて fail-open/誤判定が固定化しないこと）。

#### F-004: 読み取り経路への非回帰・fail-closed 波及の確認

`getGroupMembers` は書き込み判定（`isUserOnlyGroupMember`）だけでなく、読み取り判定経路（`IsUserInGroup` → `CanCurrentUserSafelyReadFile`）からも `GetGroupMembers` 経由で利用される。本タスクの CGO 版意味論変更（AC-06）と fail-closed 化（AC-04）がこれらに回帰・波及しないことを確認する。

- **AC-12**: CGO 版 `getGroupMembers` にプライマリメンバーを追加しても（AC-06）、`IsUserInGroup` の判定結果に回帰がない（本来メンバーであるユーザーは引き続き `true`、非メンバーは引き続き `false`）。
- **AC-13**: `GetGroupMembers` が列挙エラーを返した場合、`IsUserInGroup` はエラーを返し、`CanCurrentUserSafelyReadFile` の group-writable 分岐は読み取りを許可しない（読み取り経路も fail-closed）。

## 非機能要件

- 既存の `GroupMembership` のキャッシュ機構（30 秒 TTL）の挙動は変更しない。
- パフォーマンス: ERANGE リトライによるバッファ拡大は妥当な上限（例: 数 MB 程度）を設け、無限リトライやメモリ枯渇を招かないこと。
- 既存の公開 API シグネチャ（`GetGroupMembers`, `IsUserInGroup`, `CanUserSafelyWriteFile` 等）を変更しない。

## 参考

- 良好な既存実装パターン: `membership_cgo.go` の `unsafe.Slice` 使用、count の境界検証（`validateGroupMemberCount`）は踏襲する。

## 残存する留意点（設計フェーズで判断）

- **意味論統一の境界ケース（AC-07 関連）**: 非 CGO 版は「グループが `/etc/group` に存在しない」場合、プライマリ GID 一致ユーザーの列挙（`findUsersWithPrimaryGID`）を行わずに早期リターンで空集合を返す（`membership_nocgo.go:24-26`）。CGO 版でグループ検索元（NSS）と passwd 相当の入力が食い違うと、両実装の等価性（AC-07）が崩れうる。AC-07 は「同一の `/etc/group`・`/etc/passwd` 相当の入力」を前提とするため通常は問題にならないが、この前提と境界挙動は 02_architecture.md で明示的に扱うこと。
- **NSS 非可視メンバー（L-2）は本タスクの対象外**: 非 CGO 版はローカルファイルのみを参照するため、LDAP/SSSD 等でディレクトリ管理されるメンバーは見えない。H-1 の fail-closed 化はあくまで「列挙 API がエラーを返した場合」を対象とし、「NSS を経由しないため他メンバーが 0 人に見える」問題（L-2）は解消しない。この残存リスクは別 Issue（#860 の L-2）で扱う。
- **ERANGE リトライ上限の具体値**: AC-02 の上限到達＝エラー（fail-closed）は満たすが、具体的な上限バイト数・拡大戦略（倍々等）は 02_architecture.md で決定する。
