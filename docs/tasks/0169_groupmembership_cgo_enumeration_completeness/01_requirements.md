# 要件定義書: CGO ビルドの列挙完全性の判定と、SSSD 環境での fail-open の解消

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-27 |
| Review date | 2026-08-28 |
| Reviewer | isseis |
| Comments | 2026-08-29: AC-22 を「ファイルが変わらないこと」ではなく「振る舞い（skip 条件と比較結果）が変わらないこと」へ改めた。実装中に、同ファイルの `//go:build cgo && test` の `test` タグが無用であることが判明したためである（`_test.go` は production ビルドに入らず、同ファイルは `//go:build test` のシンボルを1つも使わない）。振る舞いに関わらない整理まで禁じる読み方は、AC の意図（意味論一致テストの適用範囲が動かないこと）を超えていた。<br>2026-08-28: 設計レビューの結果を反映して承認。AC-30（`netgroup` 行が判定の対象外であることの明記）と AC-31（`cmd/runner` の起動時確定）を追加し、AC-21 の文言を実装構造に合わせて差し替えた。`/etc/nsswitch.conf` 不在時の扱いは AC-03 のまま据え置く |

## 関連 Issue

- [#1064 CGO ビルドの列挙完全性: SSSD 環境で group-writable の書き込み判定が fail-open](https://github.com/isseis/go-safe-cmd-runner/issues/1064)
- 先行タスク: [0168 groupmembership の列挙完全性の表明と fail-closed 化](../0168_groupmembership_nocgo_enumeration_completeness/01_requirements.md)（非 CGO ビルドを対象とした対の作業。本タスクはその `01_requirements.md:74` が「対象外」として分離した AC-29 の対応先を兼ねる）
- 先行タスク: [0151 groupmembership のグループメンバー列挙 fail-open 是正](../0151_groupmembership_failclosed/01_requirements.md)（列挙 API のエラー時 fail-closed 化と、CGO 版・非 CGO 版の意味論統一）
- 残件一覧: [98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §2 D1 の「（新規）CGO ビルドの列挙完全性」

## 背景

0168 は、非 CGO ビルドの列挙が実際より少ない集合を「成功」として返し、`isUserOnlyGroupMember` を経由して group-writable ファイルへの書き込み許可（fail-open）に到達する経路を塞いだ。判定の要は、`/etc/nsswitch.conf` の `passwd`・`group` 行のソースを `files`・`systemd` の許可リストで分類し、それ以外を「不完全」に倒すことである。

同じシンクに至る経路が CGO ビルドにも別の原因で残っている。[`membership_cgo.go:331`](../../../internal/groupmembership/membership_cgo.go#L331) の `precomputeEnumerationEnvironment()` は空実装であり、その doc コメントは根拠を「libc の NSS lookup は成功時つねに完全な列挙を報告するため、最初の列挙より前に決めるべき環境要因は無い」と述べる。SSSD の2つの設定はこの前提を破る。

### (a) `enumerate = False`（SSSD の既定）

CGO 版 `getGroupMembers` が返す集合は、明示メンバー（`gr_mem`）と、当該 GID をプライマリ GID とするユーザーの和である。後者は `getUsersWithPrimaryGID` が `setpwent`/`getpwent` で全ユーザーを走査して求めるが、`enumerate = False` の下で SSSD はディレクトリ側のユーザーを1件も返さない。libc はエラーを返さないため、[`membership_cgo.go:315,328`](../../../internal/groupmembership/membership_cgo.go#L315) は無条件に `completeVerdict()` を付ける。**追加設定のない既定の SSSD 環境で常に成立する。** AD 統合環境では利用者のプライマリグループが `Domain Users` に揃うのが通例であり、漏れる人数は大きい。

### (b) `ignore_group_members = True`

SSSD が `getgrgid_r()` の `gr_mem` を空で返すようになる。既定は `False` だが、メンバーが数千規模のグループでメンバー解決の負荷を避ける性能チューニング項目として AD 統合環境で広く用いられる。(a) と重なると、メンバー集合はファイル所有者ひとりに縮む。

この経路は、[0168 の `02_architecture.md` §5.4](../0168_groupmembership_nocgo_enumeration_completeness/02_architecture.md) が「libc の `getgrgid_r`・`getpwent` は NSS を経由するため、`/etc/nsswitch.conf` に設定されたすべてのソースを参照する」と述べていた想定の外にある。0168 の PR-6 は同節に (a)・(b) の注記を加えたが、production コードは変更していない。

### 影響

- **書き込み判定（本タスクの主対象）**: [`manager.go:298`](../../../internal/groupmembership/manager.go#L298) `CanUserSafelyWriteFile` は、group-writable なファイルを `isUserOnlyGroupMember`（[`manager.go:223`](../../../internal/groupmembership/manager.go#L223)）が「唯一のメンバーが自分」と言ったときにだけ許可する。これは「グループ → メンバー一覧」を尋ねる方向であり、(a)・(b) が抑止するのはまさにこの方向である。メンバー集合が縮んだ結果、[`manager.go:245`](../../../internal/groupmembership/manager.go#L245) の `switch` は `completenessComplete` を素通りし、`len(members) == 1 && members[0] == user.Username` が成立する。ディレクトリ側に他のメンバーが何人いても書き込みが許可され、`ErrGroupMemberEnumerationIncomplete` は出ない。
- **読み取り判定（影響なし）**: [`manager.go:361`](../../../internal/groupmembership/manager.go#L361) `CanCurrentUserSafelyReadFile` が使う `IsUserInGroup`（[`manager.go:183`](../../../internal/groupmembership/manager.go#L183)）は、プライマリ GID と `userInfo.GroupIds()`（CGO では `getgrouplist(3)` 経由の initgroups）を先に見る。initgroups は「ユーザー → 所属グループ」の方向であり、(a)・(b) の影響を受けない。

### 対象となるバイナリ

公式配布バイナリは [release.yml](../../../.github/workflows/release.yml) がすべて `CGO_ENABLED: 0` でビルドしているため、本件が該当するのは **`CGO_ENABLED=1` でセルフビルドしたバイナリ**である。0168 以前の利用者向け文書は、非 CGO ビルドの制限に対する対処として `CGO_ENABLED=1` でのセルフビルドを推奨していた。すなわち本件は、その推奨に従った利用者が到達する経路である。

## 目的

- CGO ビルドが、環境から確かめられないまま「完全」を申告することをやめる。SSSD・LDAP 等が構成されたホストでは、列挙結果を group-writable ファイルへの書き込み許可の根拠に使わない（fail-closed）。
- 完全性の判定規則を CGO 版・非 CGO 版で1箇所に集約し、`sss` 等のソース名の扱いが両ビルドで一致するようにする。
- 拒否が起きたとき、運用者が原因と回復手段をエラーメッセージから辿れるようにする。**回復手段はビルドごとに異なる**——非 CGO 版の「`CGO_ENABLED=1` でビルドし直す」は CGO 版では成り立たない。
- 読み取り判定（`IsUserInGroup` 経由）と公開 API `GetGroupMembers` の挙動は変えない。

## スコープ

### 対象

1. CGO 版 `precomputeEnumerationEnvironment()` を空実装から `/etc/nsswitch.conf` の分類へ変え、CGO 版 `getGroupMembers` が返す `groupEnumeration` の完全性にその判定を反映すること。
2. `nsswitch.go` の分類器（`classifyNSSCompleteness`・`classifyNSSSources`・`nssSources`・`splitNSSTokens`・`completeNSSSources`）と、プロセス単位で判定を1度だけ確定させる仕組み（`nsswitchVerdict` とその memo、`nssCompletenessReporter`）を、両ビルドが共有する位置へ移すこと。
3. 不完全と判定した場合のエラーメッセージの「事実」と「回復手段」を、ビルドごとに正しい内容にすること。
4. 起動時に、CGO ビルドでもプロセス1回きりの警告を出すこと。`record`・`verify` は既存の `EnsurePermissionCheckUID` 経由で、`runner` は完全性判定の確定だけを行う入口を新たに設けて行う。
5. [`membership_cgo.go:331`](../../../internal/groupmembership/membership_cgo.go#L331) の doc コメント（「libc の NSS lookup は成功時つねに完全」）の是正。0168 の PR-6 が意図的に残した箇所である。
6. 利用者向け文書の更新。[security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) §3 の CGO ビルドに関する「既知の制限」、および `record_command.ja.md`・`verify_command.ja.md` のトラブルシューティング。日本語版を先に更新し、英語版は `/mktrans` で反映する。
7. `CHANGELOG.ja.md`／`CHANGELOG.md` への破壊的変更の記載。
8. 残件一覧（`98_remaining_issues.md` §2 D1）の当該項目の解消。

### 対象外

- **`IsUserInGroup` および読み取り判定（`CanCurrentUserSafelyReadFile`）の変更**。上記「背景」のとおり、この経路は initgroups を用いるため (a)・(b) の影響を受けない。列挙の完全性を読み取り側の判定材料に加えると、影響のない経路に過剰な拒否を持ち込むことになる。
- **公開 API `GetGroupMembers` の戻り値の変更**。0168 の判断（完全性を表す型は package 内に閉じる）をそのまま踏襲する。
- **`internal/runner/base/security` の誤検知の是正**。[`file_validation.go:319`](../../../internal/runner/base/security/file_validation.go#L319) の `isUserInGroup` は `GroupIds()` を使わず `GetGroupMembers` を直接引くため、SSSD 環境では正当なメンバーが「非メンバー」と判定され、[`checkWritePermission`](../../../internal/runner/base/security/file_validation.go#L244) が正当なアクセスを `ErrInvalidFilePermissions` で弾く。安全側に倒れる誤検知であり、本タスクの fail-open とは方向が逆である。原因（`GroupIds()` を使っていないこと）も別であるため分離し、[#1071](https://github.com/isseis/go-safe-cmd-runner/issues/1071) として起票のうえ残件として記録する（AC-28）。
- **SSSD の設定内容そのものの確認**。`/etc/sssd/sssd.conf` は root 専用、`sssctl` にも権限が要るため、`enumerate`・`ignore_group_members` の値を外から確かめる手段は事実上ない。この確認不能性こそが本タスクの前提である。
- **group-writable の緩和そのものの廃止**。UPG（`user:user` の 0664/0775）は Debian/Ubuntu・RHEL とも既定であり、ローカルユーザーだけのホストまで巻き込むため採らない（Issue の案3）。
- **リリースビルドの `CGO_ENABLED` 構成の変更**（[#1067](https://github.com/isseis/go-safe-cmd-runner/issues/1067)）。
- **[Makefile:147-148](../../../Makefile#L147-L148) のコメントの是正**。同コメントは「macOS はグループメンバーシップに CGO を要するため `CGO_ENABLED=0` の構成検査を macOS では行わない」と述べるが、上記「決定事項」のとおり `os/user` は darwin では非 CGO でも Directory Services を引くため、この根拠は `os/user` については成り立たない（成り立つのは本リポジトリ自身の `getGroupMembers` に限られる）。同じコメントを対象とする [#1067](https://github.com/isseis/go-safe-cmd-runner/issues/1067) の守備範囲であるため、本タスクでは触れず申し送りとする。

## 決定事項

以下は本タスクで採る方針として確定させたい事項であり、レビューでの確認を求める。詳細な設計は `02_architecture.md` に記す。

- **Issue の案1（nsswitch 分類を CGO ビルドにも通し、緩和だけを無効化する）を採る。** 案2（`getUsersWithPrimaryGID` の `getpwent` 依存の解消）は、「あるグループをプライマリ GID とする全ユーザー」を NSS から堅牢に引く API が無く、方向を反転させて initgroups を使おうにも全ユーザーの列挙が前提になるため成立しない。案4（警告のみ）は判定が `completeVerdict()` のままで fail-open が残るため単独では不十分だが、案1 に併走させる（AC-14）。

- **CGO 版で「完全」と申告する条件は、非 CGO 版と同一の規則とする。** すなわち `GOOS` が `linux` であり、かつ `/etc/nsswitch.conf` の `passwd`・`group` 両データベースのソースがいずれも `files`・`systemd` のみであること。判定の理由は両ビルドで異なる——非 CGO 版は「このビルドが読めないソース」であるのに対し、CGO 版は「libc は読めるが、網羅的に列挙する保証が無いソース」である——が、**許可リストの中身は一致する**。`files`・`systemd` は列挙が網羅的であることが確かめられ、`sss`・`ldap`・`nis`・`winbind`・`compat`・`db` その他の未知のソース名はいずれも確かめられない。規則が一致する以上、分類器を複製せず共有する。

- **`GOOS` が `linux` 以外の場合、CGO ビルドでも「不完全」と申告する。** これは現在の挙動（macOS の CGO ビルドは常に「完全」）を変える。理由は次のとおりである。
  - `/etc/nsswitch.conf` を持たないプラットフォームでは、ユーザーデータベースの構成を判定する手段が無い。Open Directory が AD にバインドされた macOS には SSSD と同じ部分列挙のリスクがあり、外から確かめられない点も同じである。「確かめられないものを完全と申告しない」という 0168 の方針をここで曲げる根拠が無い。
  - **「macOS ではディレクトリを引くのに CGO が要る」という反論は成り立たない。** Go の `os/user` は darwin だけ build tag が `cgo` ではなく `!osusergo && darwin` であり（[`cgo_lookup_syscall.go`](https://github.com/golang/go/blob/master/src/os/user/cgo_lookup_syscall.go)・[`getgrouplist_syscall.go`](https://github.com/golang/go/blob/master/src/os/user/getgrouplist_syscall.go)）、libSystem を直接 syscall linkage で呼ぶ。`CGO_ENABLED=0` でも macOS の `user.LookupId`・`GroupIds()` は Directory Services を引く。したがって macOS で `CGO_ENABLED=1` が非 CGO ビルドに対して追加するのは、本リポジトリ自身の `getGroupMembers`（`membership_cgo.go`）だけである。
  - **その `getGroupMembers` の消費者のうち、本 AC が影響するのは緩和の側だけである。** 消費者は3つある。(1) `isUserOnlyGroupMember`（本 AC が拒否側へ倒す）、(2) `IsUserInGroup` の最終フォールバック（プライマリ GID と `GroupIds()`＝`getgrouplist` で先に決着するため、darwin では非 CGO でも同じ結論になる）、(3) 公開 `GetGroupMembers` を引く [`file_validation.go:319`](../../../internal/runner/base/security/file_validation.go#L319)。macOS の `/etc/passwd` にはシステムアカウントしか無いため (3) は非 CGO 版で正当なメンバーを取りこぼすが、これは「対象外」に置いた誤検知（AC-28）そのものであり、本 AC が作る問題ではない。
  - 実際の代償は小さい。本 AC は「macOS の CGO ビルドの書き込み緩和を、非 CGO ビルドと同じ状態にする」だけの変更である（0168 の AC-06 が非 CGO の darwin を既に「不完全」としている）。加えて macOS のローカルユーザーのプライマリグループは全員 `staff` であり、UPG は既定ではない。「メンバーが自分ひとりのグループ」が group-writable な保護対象ファイルに付いている構成は稀である。
  - **確認事項**: 代替として「非 linux の CGO ビルドは現状どおり完全と申告する」も採りうる。ただしその場合、緩和の正しさは Open Directory が網羅的なメンバー一覧を返すことに依拠し、それは SSSD と同じく外から確かめられない。過剰拒否を避ける観点からこちらを採る場合は、その旨をレビューで指示されたい。

- **受容する副作用は過剰拒否である。** `ignore_group_members` を設定しておらず `gr_mem` が正しく返っている SSSD 環境も拒否する。さらに、判定はホスト単位であるため、**ローカルの `/etc/group` にしか存在しないグループが付いた group-writable ファイルも拒否される**。影響を受けるのは「ディレクトリ統合ホストで group-writable な保護対象ファイルを扱う」構成に限られ、そもそも推奨されない構成である。

- **拒否時のエラーメッセージの「事実」と「回復手段」はビルドごとに与える。** `incompletenessCause` の語彙（`causeUnsupportedPlatform`・`causeNSSSources`・`causeMalformedLine`）は共有したまま、[`incompleteEnumerationError`](../../../internal/groupmembership/manager.go#L252) が用いる文面を build tag で分ける。非 CGO 版の文面（「このビルドは参照できない」「`CGO_ENABLED=1` でビルドし直せ」）は CGO 版では誤りであり、**そのまま出すと利用者を、既に居る場所へ誘導する**。CGO 版の回復手段は「対象パスの group-writable ビットを外す」である。文面の選択は `cause` による `switch` で行い、`detail` の文字列を検査して分岐しない（CLAUDE.md「Declare, don't infer」）。

- **`causeMalformedLine` は CGO 版では発生しない。** ファイルを直接走査するのは非 CGO 版だけである。CGO 版の文面生成でも `switch` の分岐自体は残し、到達した場合は実装の誤りとして扱う（`default` と同じく拒否側）。

- **判定はプロセス単位で1度だけ確定させ、以後再読しない。** 0168 が非 CGO 版で採った方針（実行の途中で `/etc/nsswitch.conf` が編集されたために判定が変わると、運用者が再現できない拒否になる）をそのまま共有の仕組みとして両ビルドへ広げる。

## 受け入れ基準（Acceptance Criteria）

#### F-001: CGO ビルドが列挙の完全性を環境から判定する

`precomputeEnumerationEnvironment()` の空実装をやめ、CGO 版 `getGroupMembers` の完全性の申告に `/etc/nsswitch.conf` の分類を反映する。

**Acceptance Criteria**:

- **AC-01**: CGO ビルドの `getGroupMembers` は、`GOOS` が `linux` であり、かつ `/etc/nsswitch.conf` の `passwd`・`group` 両データベースのソースが `files` または `systemd` のみである場合に限り「完全」と申告する。
- **AC-02**: CGO ビルドの `getGroupMembers` は、`passwd` または `group` のソースに `sss`・`ldap` その他の許可リスト外の名前が含まれる場合、libc の lookup がエラーを返していなくても「不完全」と申告する。
- **AC-03**: `/etc/nsswitch.conf` が存在しない場合は `files` とみなして「完全」と申告する。ファイルは存在するが読み取りに失敗した場合、`passwd` または `group` の行が存在しない場合、行が重複する場合、角括弧トークンが閉じていない場合はいずれも「不完全」と申告する（非 CGO 版と同一の分類）。
- **AC-04**: CGO ビルドは、`GOOS` が `linux` 以外の場合に「不完全」と申告する（決定事項の「確認事項」に従い、レビューで代替方針が採られた場合はその方針に読み替える）。
- **AC-05**: `isUserOnlyGroupMember` は、CGO ビルドにおいても「不完全」の申告に対しメンバー集合の内容によらず sentinel エラー `ErrGroupMemberEnumerationIncomplete` を返す。ユーザーが集合内の唯一の名前であっても許可しない。
- **AC-06**: メンバーシップキャッシュは CGO ビルドでも完全性をメンバー集合と併せて保持し、キャッシュヒット時にも AC-05 の判定が同じ結果になる。
- **AC-07**: [`membership_cgo.go:331`](../../../internal/groupmembership/membership_cgo.go#L331) の doc コメントが、`precomputeEnumerationEnvironment()` の新しい役割を述べるものに書き換えられている。「libc の NSS lookup は成功時つねに完全」という、SSSD の `enumerate = False`・`ignore_group_members = True` で破れる根拠が残らない。

#### F-002: 判定規則の共有

**Acceptance Criteria**:

- **AC-08**: `/etc/nsswitch.conf` の読み取り・分類と、プロセス単位で判定を確定させる仕組みが、CGO ビルド・非 CGO ビルドの双方からコンパイルされる1つの実装として存在する。ビルドごとに複製された分類器や許可リストが残らない。
- **AC-09**: 許可リスト `completeNSSSources` の内容が両ビルドで同一であり、同じ `/etc/nsswitch.conf` の内容に対して両ビルドが同じ完全性を判定する。この一致がテストで検証される。
- **AC-10**: `nsswitch.go` が持っていた `//go:build !cgo || test` の制約が外れたことにより、非 CGO 専用のシンボル（ファイル走査、`malformedLines` 等）が CGO ビルドへ持ち込まれることがない。`make deadcode` が新たな到達不能コードを報告しない。

#### F-003: 拒否メッセージがビルドに応じた事実と回復手段を示す

**Acceptance Criteria**:

- **AC-11**: CGO ビルドで AC-05 の拒否が起きたとき、エラーメッセージは `user_database_source=nss` と `cause`、および不完全と判定した具体的な理由（`detail`）を含む。
- **AC-12**: CGO ビルドのエラーメッセージは、回復手段として `CGO_ENABLED=1` でのビルドを示さない。示す回復手段は対象パスの group-writable ビットを外すことであり、事実としては「設定されたソースが網羅的に列挙される保証が無い」ことを述べる。
- **AC-13**: 非 CGO ビルドのエラーメッセージは本タスクの前後で変わらない。0168 の AC-18 が定めた文面（`cause` ごとの事実と回復手段）がそのまま維持される。
- **AC-14**: 文面の選択は `cause` に対する `switch` で行われ、`default` および `causeUnspecified` は実装の誤りであることが読み取れるメッセージになる。`detail` の文字列内容によって分岐する箇所が無い。

#### F-004: 起動時の警告

**Acceptance Criteria**:

- **AC-15**: CGO ビルドでも `EnsurePermissionCheckUID` の呼び出しにより、最初の group-writable ファイルに到達する前に完全性の判定が確定する。列挙が不完全と判定された場合、プロセスにつき1回だけ警告が記録される。
- **AC-16**: AC-15 の警告は、非 CGO 版が出力するものと同じメッセージ・同じ属性名（`user_database_source`・`cause`・`detail`）を用いる。`user_database_source` の値はビルドに応じて `nss`／`passwd-file` になる。
- **AC-31**: `cmd/runner` も起動時に完全性判定を確定させる。最初の group-writable な構成要素の判定に到達する前に、列挙が不完全であれば AC-15・AC-16 と同じ警告が記録される。この入口は完全性判定の確定だけを行い、基準UIDの解決を伴わない——`EnsurePermissionCheckUID` を呼ばせると `SUDO_UID` を検証できない場合の失敗が `runner` の起動時に新たに加わるためである。`record`・`verify` の挙動は変わらない。

#### F-005: 影響範囲の限定

**Acceptance Criteria**:

- **AC-17**: `IsUserInGroup` および `CanCurrentUserSafelyReadFile` の判定結果が、列挙結果の完全性によらず本タスクの前後で変わらない。CGO ビルドの読み取り判定が新たに拒否を返すことはない。
- **AC-18**: 公開 API `GetGroupMembers` の戻り値の型と意味が本タスクの前後で変わらない。`internal/runner/base/security` および `internal/safefileio` は無変更である。
- **AC-19**: 列挙が「完全」と判定される環境（`passwd: files systemd` 等）における `CanUserSafelyWriteFile` の判定結果が、CGO ビルドで本タスクの前後で変わらない。world-writable の一律拒否、非所有者の拒否、owner-writable の許可、group-writable かつ唯一のメンバーである場合の許可がいずれも従来どおりである。

#### F-006: テスト

**Acceptance Criteria**:

- **AC-20**: AC-01〜AC-05 を検証するテストが、実行ホストの `/etc/nsswitch.conf` の内容に依存せず、与えられた内容だけから判定できる形で書かれている。CGO ビルドでも、SSSD が構成されたホストを模した内容に対する判定が検証できる。
- **AC-21**: F-001・F-003 の各 AC を検証するテストが、検証対象の分岐を無効化すると失敗する（CLAUDE.md「テストは主張する理由で失敗できること」）。無効化の方法と失敗を確認した旨をコミットメッセージに記す。とくに AC-02 は CGO 版 `getGroupMembers` が完全性判定ではなく `completeVerdict()` を載せるように戻すと、AC-15 は完全性判定を確定させる処理を空実装に戻すと、それぞれ失敗しなければならない。
- **AC-22**: [`membership_semantics_test.go`](../../../internal/groupmembership/membership_semantics_test.go) の `TestGetGroupMembers_CGOAndNoCGOSemanticsMatch` の**振る舞い**が本タスクの前後で変わらない。すなわち、対象とする環境集合（skip 条件）と比較結果が変わらない。同テストが比較するのはメンバー集合であり、完全性の申告を加えたことによって skip 条件が変わることはない。**これはファイルの無変更を求めるものではない**——振る舞いを変えない編集（ビルドタグの整理など）は許す。
- **AC-23**: CGO ビルド・非 CGO ビルドの双方で `make test` と `make lint` が通過する。

#### F-007: 文書への反映

**Acceptance Criteria**:

- **AC-24**: [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) §3 の CGO ビルドに関する記述（現行の「なお CGO ビルドにも既知の制限がある……実際より緩く評価される可能性がある」）が、本タスク後の挙動に更新されている。SSSD/LDAP 等が構成されたホストでは CGO ビルドでも書き込み安全性判定が**拒否される**こと、`files`・`systemd` のみの環境では従来どおり判定できることが読み取れる。#1064 への参照は解消済みとして扱う。
- **AC-25**: `record_command.ja.md`・`verify_command.ja.md` のトラブルシューティングに、CGO ビルドで AC-05 の拒否に遭遇した場合の項目が追加されている。既存の非 CGO ビルド向けの記述（`user_database_source=passwd-file` の例）と混同されない形で、`user_database_source=nss` の例と、回復手段が異なること（`CGO_ENABLED=1` でのビルドは回復手段にならないこと）が示されている。
- **AC-26**: AC-24・AC-25 の英語版が `/mktrans` により反映されている。日本語版を先に作成・コミットする。
- **AC-27**: [CHANGELOG.ja.md](../../../CHANGELOG.ja.md) の「未リリース」→「破壊的変更」に本変更の項目が追加され、その英語版が [CHANGELOG.md](../../../CHANGELOG.md) に `/mktrans` により反映されている。項目からは、拒否が起きる条件（CGO ビルドかつ許可リスト外の NSS ソースが構成されたホスト、および非 linux）、影響を受ける構成、回復手段、切り戻し方法が読み取れる。同節の既存項目の書式（対象範囲を示す見出し、`**影響範囲:**`、アップグレード前に影響有無を判定する手順）に揃える。既存の非 CGO ビルド向けの項目（`CHANGELOG.ja.md:79`）とは別項目とし、両者の関係が読み取れるようにする。
- **AC-28**: [98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §2 D1 の「（新規）CGO ビルドの列挙完全性」が解消済みとして更新され、同節が既に用いている引用ブロック（`> **… について**`）の形式で本タスクと #1064 への参照が記載されている。あわせて、「対象外」節で分離した `internal/runner/base/security` の誤検知が残件として追加されている。
- **AC-29**: `98_remaining_issues.md` の D1 以外の残件の記述が、本タスクの書き換えによって増減していない。
- **AC-30**: `record_command.ja.md`・`verify_command.ja.md`・[security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) の該当箇所に、完全性の判定が `/etc/nsswitch.conf` の `passwd`・`group` の2行だけを見ること、および `netgroup` 行は判定に影響しないことが明記されている。Ubuntu の既定である `netgroup: nis` を見た利用者が、自ホストが該当すると誤認しないことが読み取れる。その英語版が `/mktrans` により反映されている。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが CGO・非 CGO 双方のビルドで `make test` により成功する。
- `make lint` が CGO・非 CGO 双方で警告なく通過する。
- SSSD が構成されたホストで `CGO_ENABLED=1` ビルドを用いた場合、group-writable な保護対象ファイルへの書き込みが、ディレクトリ側のメンバーの有無によらず拒否される。
- 列挙が完全である環境における `CanUserSafelyWriteFile`・`IsUserInGroup`・`CanCurrentUserSafelyReadFile` の外部から観測できる挙動が、本タスクの前後で変わらない。
- 拒否に遭遇した運用者が、エラーメッセージだけから原因と、そのビルドで実際に採れる回復手段に到達できる。`CGO_ENABLED=1` でビルド済みの利用者が「CGO_ENABLED=1 でビルドし直せ」と案内されることがない。
- 0168 の非 CGO 版と本タスクの CGO 版とで、同じ `/etc/nsswitch.conf` に対する完全性の判定が一致することが、コードとテストの双方から追える。
