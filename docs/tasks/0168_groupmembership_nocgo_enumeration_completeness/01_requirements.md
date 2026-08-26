# 要件定義書: groupmembership の列挙完全性の表明と、不完全な列挙を許可根拠にしない fail-closed 化

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-08-25 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#976 [Security][D1 L-2/L-3] groupmembership: 非CGO版がNSSディレクトリ管理メンバーを列挙しない](https://github.com/isseis/go-safe-cmd-runner/issues/976)
- 詳細所見: [docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の L-2・L-3
- 残件一覧: [docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §2 D1
- 先行タスク: [0151 groupmembership のグループメンバー列挙 fail-open 是正](../0151_groupmembership_failclosed/01_requirements.md)（H-1・M-1・M-2 を対応し、L-2・L-3 を明示的に対象外とした）
- 先行タスク: [0160 権限チェック基準 UID の方針明示化](../0160_permission_check_subject_explicit/01_requirements.md)、[0161 SUDO_UID の実在確認と記録](../0161_sudo_uid_validation_and_logging/01_requirements.md)（`user_database_source` 属性を導入した）

## 背景

`internal/groupmembership` は `safefileio` が呼び出す、ハッシュファイル・設定ファイルの読み書き安全性判定の中核である。0149 監査の総評は、この package 最大の構造的リスクを「グループメンバー列挙の失敗・空結果が、`isUserOnlyGroupMember` を経由して書き込み許可（fail-open）に到達すること」と要約し、H-1・M-1・L-2・L-3 の4件が同一のシンクに合流すると指摘した。

0151 はこのうち H-1・M-1・M-2 を解消した。列挙 API がエラーを返した場合は fail-closed になり、CGO 版・非 CGO 版の意味論も「明示メンバー（`gr_mem`）＋当該 GID をプライマリ GID とするユーザーの和集合」に統一された。残る L-2・L-3 は、いずれも列挙 API がエラーを返さないまま結果が実際より少なくなる経路であり、0151 の対象外として残された。

### L-2: 非 CGO ビルドは NSS を参照しないまま、成功として少ない集合を返す

非 CGO ビルドの `getGroupMembers`（[membership_nocgo.go:20-58](../../../internal/groupmembership/membership_nocgo.go#L20-L58)）は `/etc/group` と `/etc/passwd` を直接パースする。LDAP・SSSD 等の NSS ディレクトリサービスでのみ管理されるメンバーはこれらのファイルに現れないため、実際には他のメンバーがいるグループが「メンバー1名」として返る。この結果は `isUserOnlyGroupMember` で「ユーザーが唯一のメンバー」と解釈され、`CanUserSafelyWriteFile` の group-writable 分岐で書き込み許可に至る。

この経路は理論上の可能性ではなく、配布バイナリの既定の経路である。[release.yml:35](../../../.github/workflows/release.yml#L35) はリリース成果物を `CGO_ENABLED: 0` でビルドしており、利用者が入手する `runner`・`record`・`verify` はすべて非 CGO ビルドである。CI（`ci.yml`）は cgo=1／cgo=0 の双方を検査しているが、それは配布バイナリが NSS を参照するかどうかとは無関係である。検査される2つのうち、利用者に届くのは非 CGO ビルドだけだからである。

現状の対応は文書のみである。[docs/user/security-risk-assessment.ja.md:340-345](../../user/security-risk-assessment.ja.md#L340-L345) が「既知の制限」として、配布バイナリが `CGO_ENABLED=0` であること、NSS 管理のメンバーが列挙されないこと、NSS 依存環境では `CGO_ENABLED=1` でのセルフビルドを検討すべきことを記載している。すなわち所見 L-2 の推奨のうち「ドキュメント化する」側は既に済んでおり、未着手なのは「非 CGO 版では group-writable の許可判定を fail-closed に倒す」側である。運用者が制限に気付かずに配布バイナリを NSS 環境で使う場合、文書は防御として働かない。

### L-3: パース不能な行は警告のみで読み飛ばされ、集合が黙って縮む

`findGroupByGID`・`findUsersWithPrimaryGID`（[membership_files.go](../../../internal/groupmembership/membership_files.go)）は、パースできない行を `slog.Warn` で記録して読み飛ばす。この警告出力は所見 L-3 の推奨の前半（「少なくとも `slog.Warn` で不正行を記録する」）に対応するもので、既に実装済みである。

未着手なのは後半、「対象 GID の行がパース不能だった場合はエラーを返すことを検討する」である。ただしこの推奨は文字どおりには実装できない。行がパース不能になる主因は GID フィールドの解析失敗であり、その行がどの GID のものだったかは定義上わからないためである。したがって本タスクは「対象 GID の行か否か」を判定する方向を採らず、「パース不能な行が1行でもあれば、その列挙結果は不完全である」という、より弱いが判定可能な性質に置き換えて扱う。

### 2件は同じ性質の異なる原因である

L-2 も L-3 も、`getGroupMembers` が `(members, nil)` を返しながら `members` が実際のメンバー集合の部分集合でしかない、という同一の状態を作る。呼び出し側はこの2つを区別できず、また区別する必要もない。必要なのは「返された集合が全メンバーを網羅していると信じてよいか」という1つの事実である。現状の戻り値型にはこの事実を表す場所がない。

なお、この事実を必要とするのは、列挙結果を許可の根拠に使う呼び出し元だけである。`isUserOnlyGroupMember` は「メンバーが本人1名だけ」を書き込み許可の根拠に使うため、集合が実際より小さいと誤って許可する。一方 `IsUserInGroup` は「ユーザーがメンバーに含まれる」ことを読み取り許可の根拠に使うため、集合が実際より小さいと誤って拒否する。後者は既に fail-closed 側であり、不完全性を理由に追加で拒否する必要はない。

### 判定ロジックの原型はテストコードに既にある

`/etc/nsswitch.conf` の `passwd`・`group` 行を読み、ソースが `files`／`systemd` 以外を含むかどうかで環境を分類する処理は、[membership_semantics_test.go](../../../internal/groupmembership/membership_semantics_test.go) の `shouldSkipSemanticsTest`／`nssSources` に既に存在する。CGO 版と非 CGO 版の意味論一致テストが、ローカルファイルのみで完結しない環境では成立しないことを判定するために 0151 が導入したものである。本タスクが production コード側で必要とする判定はこれと同一であり、定義が2箇所に分かれないよう production コード側へ移して両者で共有する。

## 目的

- `getGroupMembers` の戻り値に「返した集合が全メンバーを網羅しているか」という事実を持たせ、呼び出し元が推測ではなく型からそれを知れるようにする。
- 不完全な列挙結果が書き込み許可の根拠に使われないようにする（fail-closed）。すなわち、非 CGO ビルドが NSS 環境で動いている場合、およびユーザーデータベースのファイルにパース不能な行がある場合、group-writable ファイルへの書き込みは許可されない。
- 上記の拒否が起きたとき、運用者が原因（どの環境要因により列挙が不完全と判定されたか）と回復手段（`CGO_ENABLED=1` でのセルフビルド、または不正行の修正）をエラーメッセージから辿れるようにする。
- 読み取り判定（`IsUserInGroup` 経由）の挙動は変えない。不完全な列挙は読み取りでは既に拒否側に働くため、追加の制限は過剰である。

## スコープ

### 対象

1. `getGroupMembers`（CGO 版・非 CGO 版の両方）が、メンバー名の集合に加えて列挙の完全性を返すこと。CGO 版は NSS を参照するため常に完全である。
2. 非 CGO ビルドにおける完全性の判定。`/etc/nsswitch.conf` の `passwd`・`group` データベースのソース構成、および対象プラットフォームが Linux であるかに基づく。
3. `findGroupByGID`・`findUsersWithPrimaryGID` がパース不能な行を読み飛ばした事実を、列挙の不完全性として呼び出し元へ伝えること（L-3）。
4. `isUserOnlyGroupMember` が、不完全な列挙結果を許可の根拠に使わずエラーを返すこと。および `CanUserSafelyWriteFile` の group-writable 分岐がそれにより fail-closed になること。
5. メンバーシップキャッシュが完全性をメンバー集合と併せて保持すること。
6. `membership_semantics_test.go` の `shouldSkipSemanticsTest`／`nssSources` を production コード側の実装に置き換え、判定定義を1箇所にすること。
7. 利用者向け文書の更新。`security-risk-assessment.ja.md` の「既知の制限」節（挙動が「緩く評価される場合がある」から「拒否される」に変わる）、および `record_command.ja.md`／`verify_command.ja.md` のトラブルシューティング。日本語版を先に更新し、英語版は `/mktrans` で反映する。
8. 0149 の残件一覧（`98_remaining_issues.md` §2 D1）と findings（`D1_groupmembership.md` の L-2・L-3）への対応状況の反映。

### 対象外

- **`IsUserInGroup` および読み取り判定（`CanCurrentUserSafelyReadFile`）の変更**。上記「背景」のとおり、不完全な列挙はこの経路では既に拒否側に働く。
- **CGO 版における `getpwent` の列挙不完全性**。SSSD は既定で `enumerate = false` であり、その構成では `getpwent` がディレクトリ管理ユーザーを返さない。つまり CGO 版の「プライマリ GID 一致ユーザー」の収集も NSS バックエンドの設定次第で不完全になりうる。これは L-2 と同じ性質の残存リスクだが、`/etc/nsswitch.conf` からは判定できず、バックエンドごとの設定調査を要する。本タスクでは扱わず、新規 Issue として分離する。
- **リリースビルドを `CGO_ENABLED=1` に変更すること**。cgo でのクロスコンパイルには target ごとのツールチェーンが必要であり、リリースワークフローの構成変更は本タスクの範囲を超える。なお [release.yml](../../../.github/workflows/release.yml) は `darwin/arm64` も `CGO_ENABLED: 0` でビルドしている一方、[Makefile:147-148](../../../Makefile#L147-L148) は「macOS はグループメンバーシップに CGO を要するため `CGO_ENABLED=0` を検査しない」としており、両者は整合していない。この不整合の扱いも新規 Issue として分離する。本タスクは、その darwin 非 CGO バイナリが列挙を不完全と申告して fail-closed になることまでを保証する（AC-06）。
- **D1 L-1（`GetGroupMembers` がキャッシュ内部のスライスをそのまま返す）**。キャッシュの構造に触れる点で本タスクと近接するが、原因も影響も別である。残件一覧に残す。
- **D1 L-4（判定 API が `(false, nil)` と `(false, err)` を混在して返す）**。API 形状の統一であり、本タスクが追加する拒否経路もこの既存の形状に従う。残件一覧に残す。
- **`internal/safefileio` 等の呼び出し元パッケージの変更**。本タスクは `(bool, error)` の契約を変えないため、呼び出し元は無変更で新しい拒否を受け取る。

## 決定事項

以下は本タスクで採る方針として確定させたい事項であり、レビューでの確認を求める。詳細な設計は `02_architecture.md` に記す。

- **完全性は列挙結果の型に持たせ、3つの名前付きの値で表す**。`getGroupMembers` が返す値に、集合が網羅的かどうかを表すフィールドを設ける。CLAUDE.md「Declare, don't infer」に従い真偽値は使わず、専用の列挙型に「未申告（ゼロ値）」「完全」「不完全」の3つを定義する。ゼロ値に「不完全」の意味を負わせない理由は次のとおりである。

  - 「実装者が申告を忘れた」状態と「環境を調べた結果として不完全と判定した」状態は、原因も対処も異なる。前者はプログラミングの誤りであり修正すべきものだが、後者は正常な判定結果である。ゼロ値を「不完全」にすると両者が同じ値になり、区別できなくなる。
  - AC-18 は、拒否時のエラーメッセージに不完全と判定した理由を含めることを求めている。不完全の申告には理由が伴うが、申告し忘れには理由がない。ゼロ値を「不完全」とすると、理由を持たない「不完全」という表現不能な状態を型が許すことになる。
  - 安全性はどちらの設計でも同じである。「未申告」も「完全」ではない以上、`switch` の `default` が拒否側に倒れる（AC-03）ため fail-closed になる。3値にすることで失うものはない。

  命名と形は、本リポジトリの既存の列挙型（[`RiskLevelUnknown RiskLevel = iota`](../../../internal/runner/base/runnertypes/config.go#L33)、[`PolicyUnset`](../../../internal/groupmembership/policy.go)）に揃える。

- **公開 API `GetGroupMembers` の戻り値は変えない**。完全性を必要とするのは package 内の `isUserOnlyGroupMember` だけである。package 外の呼び出し元は [file_validation.go:336](../../../internal/runner/base/security/file_validation.go#L336) の1箇所だけであり、そこでの用法は、メンバーに含まれることを根拠に `true` を返す（＝集合が実際より小さいと拒否側に働く）というものである。この用法は上記「背景」の `IsUserInGroup` と同じ理由により、変更を要しない。したがって完全性を表す型は package 内に閉じる。

- **非 CGO ビルドで「完全」と申告する条件**は次の2つをともに満たすこととする。
  1. `GOOS` が `linux` である。
  2. `/etc/nsswitch.conf` の `passwd`・`group` 両データベースのソースが、いずれも `files` または `systemd` のみである。ファイルが存在しない場合は glibc の既定に従い `files` とみなす。ファイルが存在するが読み取りに失敗した場合、または `passwd`・`group` の行が無い場合は「不完全」とする。

- **`systemd` を許可リストに含める根拠と、受容する残存リスク**。`nss-systemd` が提供するのは `DynamicUser=` サービスの一時 UID、`systemd-homed` のユーザー、`systemd-nspawn`／`machined` のマッピングであり、いずれも本ツールが保護対象とするハッシュファイル・設定ファイルのグループを共有する立場になりにくい。一方 Ubuntu の既定 `/etc/nsswitch.conf` は `passwd: files systemd` であり、`systemd` を除外すると主要ターゲット環境の配布バイナリが group-writable 分岐で常に拒否することになる。この判断は `shouldSkipSemanticsTest` が 0151 以来採ってきた区分と同一であり、production コードとテストで同じ定義を共有する。`systemd-homed` ユーザーが group-writable ファイルのグループを共有する構成は残存リスクとして受容し、findings に記録する。
  `compat`・`db`・`ldap`・`sss`・`nis`・`winbind` その他の未知のソース名は、すべて「不完全」とする。とくに `compat` は `+`／`-` エントリ経由で NIS を引き込みうるため、名前が既知であることを理由に許容しない（CLAUDE.md「Reject, don't normalize」）。

- **パース不能な行の扱い**は、行の内容や位置によらず「その列挙は不完全」とする。「対象 GID の行だったか」の判定は上記「背景」のとおり原理的にできない。既存の `slog.Warn` は残す。

- **判定の実装形は、純関数と入出力を分離する**。`/etc/nsswitch.conf` の内容を受け取って分類する関数は副作用を持たず、テストが内容を直接与えられる形とする。既存の `shouldSkipSemanticsTest` のテーブルテストは、この production コード側の関数に対するテストとして移設する。

- **拒否時のエラー**は sentinel エラーとし、メッセージに `user_database_source`（0160/0161 が導入した属性。非 CGO は `passwd-file`、CGO は `nss`）と、不完全と判定した具体的な理由を含める。既存の `SUDO_UID` 関連エラーメッセージが採っている「事実 → 確認事項 → 回復手段」の書式に揃える。

## 受け入れ基準（Acceptance Criteria）

#### F-001: 列挙結果が完全性を申告する

`getGroupMembers` の戻り値に、返した集合が全メンバーを網羅しているかどうかを持たせる。

**Acceptance Criteria**:

- **AC-01**: `getGroupMembers` は、メンバー名の集合に加えて列挙の完全性を返す。完全性は専用の列挙型で表され、「未申告」「完全」「不完全」の3つの名前付きの値を持つ。ゼロ値は「未申告」であり、「完全」でも「不完全」でもない。
- **AC-02**: CGO 版 `getGroupMembers` は、成功時に常に「完全」を返す。
- **AC-03**: 完全性を解釈する側は `switch` で分岐し、「完全」以外（「不完全」「未申告」、および想定外の値）をすべて拒否側に倒す。`default` に到達した値が「完全」として扱われることはない。
- **AC-03a**: 「未申告」は「不完全」と区別して扱われる。「未申告」に到達した場合のエラーは、環境要因による不完全（AC-18）ではなくプログラミングの誤りであることが読み取れるメッセージを持ち、`errors.Is` で AC-15 の sentinel と区別できる。この分岐がテストで実行され、検証される。
- **AC-04**: 0151 が確立した既存の契約が維持される。すなわち、列挙 API がエラーを報告した場合は `(nil, non-nil error)` を返し、指定 GID のグループが存在しない場合は空集合とエラーなしを返す。
- **AC-04a**: 公開 API `GetGroupMembers` の戻り値の型と意味が本タスクの前後で変わらない。`internal/runner/base/security` および `internal/safefileio` は無変更で従来どおり動作する。

#### F-002: 非 CGO ビルドにおける完全性の判定（L-2）

**Acceptance Criteria**:

- **AC-05**: 非 CGO ビルドは、`GOOS` が `linux` であり、かつ `/etc/nsswitch.conf` の `passwd`・`group` 両データベースのソースが `files` または `systemd` のみである場合に限り「完全」と申告する。
- **AC-06**: 非 CGO ビルドは、`GOOS` が `linux` 以外の場合に「不完全」と申告する。これには [release.yml](../../../.github/workflows/release.yml) が `CGO_ENABLED: 0` でビルドする `darwin/arm64` バイナリが含まれる。
- **AC-07**: `/etc/nsswitch.conf` が存在しない場合は `files` とみなして「完全」と申告する。ファイルは存在するが読み取りに失敗した場合、`passwd` または `group` の行が存在しない場合、いずれかのソースが `files`・`systemd` 以外を含む場合は「不完全」と申告する。
- **AC-08**: 未知のソース名（`ldap`・`sss`・`nis`・`winbind`・`db`・`compat` を含む）は「不完全」と判定される。許容は `files`・`systemd` のみの許可リスト方式であり、既知の危険なソース名を列挙するブロックリスト方式ではない。
- **AC-09**: ソース名に付随する動作指定（`[NOTFOUND=continue]` 等の角括弧トークン）はソース名として扱われず、それ自体が「不完全」の判定理由にならない。
- **AC-10**: `/etc/nsswitch.conf` の内容を分類する処理は、ファイルシステムに触れない関数として与えられた内容だけから判定でき、AC-05〜AC-09 の各条件をテーブルテストで検証できる。

#### F-003: パース不能な行の扱い（L-3）

**Acceptance Criteria**:

- **AC-11**: `/etc/group` または `/etc/passwd` の走査中にパース不能な行を1行以上読み飛ばした場合、その列挙結果は「不完全」と申告される。読み飛ばした行が対象 GID のものであったかどうかは問わない。
- **AC-12**: パース不能な行に対する既存の `slog.Warn` 出力（ファイル名・行番号・エラー）は維持される。
- **AC-13**: 空行および `#` で始まる行は不正行として扱われず、「不完全」の判定理由にならない。

#### F-004: 不完全な列挙を書き込み許可の根拠にしない

**Acceptance Criteria**:

- **AC-14**: `isUserOnlyGroupMember` は、列挙結果が「不完全」である場合、メンバー集合の内容によらず sentinel エラーを返す。ユーザーが集合内の唯一の名前であっても許可しない。
- **AC-15**: `CanUserSafelyWriteFile` は、group-writable なファイルについて、列挙結果が「不完全」である場合に `(false, non-nil error)` を返す。エラーは AC-14 の sentinel を `errors.Is` で判別できる。
- **AC-16**: 列挙結果が「完全」である場合の `CanUserSafelyWriteFile` の判定結果は、本タスクの前後で変わらない。world-writable の一律拒否、非所有者の拒否、owner-writable の許可、および group-writable かつ唯一のメンバーである場合の許可がいずれも従来どおりである。
- **AC-17**: `IsUserInGroup` および `CanCurrentUserSafelyReadFile` の判定結果は、列挙結果の完全性によらず本タスクの前後で変わらない。
- **AC-18**: AC-15 のエラーメッセージに、`user_database_source` の値と、不完全と判定した理由（NSS のソース構成によるものか、パース不能な行によるものか）が含まれる。NSS のソース構成による場合は、回復手段として `CGO_ENABLED=1` でのビルドが示される。
- **AC-19**: メンバーシップキャッシュは完全性をメンバー集合と併せて保持し、キャッシュヒット時にも AC-14 の判定が同じ結果になる。完全性の申告がキャッシュを跨いで失われることはない。

#### F-005: 判定定義の一元化とテスト

**Acceptance Criteria**:

- **AC-20**: `membership_semantics_test.go` の `shouldSkipSemanticsTest`・`nssSources` は、production コード側の判定関数を呼ぶ形に置き換えられ、テスト専用の複製実装が残らない。`TestGetGroupMembers_CGOAndNoCGOSemanticsMatch` の skip 条件は本タスクの前後で同じ環境集合を対象とする。
- **AC-21**: F-002・F-003・F-004 の各 AC を検証するテストが、検証対象の分岐を無効化すると失敗する（CLAUDE.md「テストは主張する理由で失敗できること」）。無効化の方法と失敗を確認した旨をコミットメッセージに記す。
- **AC-22**: CGO ビルド・非 CGO ビルドの双方で `make test` と `make lint` が通過する。

#### F-006: 文書への反映

**Acceptance Criteria**:

- **AC-23**: [docs/user/security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) §3 の「既知の制限（`CGO_ENABLED=0` ビルド）」が、本タスク後の挙動に更新されている。NSS 環境の非 CGO ビルドでは group-writable ファイルへの書き込みが「緩く評価される場合がある」のではなく「拒否される」こと、および `files`・`systemd` のみの環境では従来どおり判定できることが読み取れる。
- **AC-24**: `record_command.ja.md`・`verify_command.ja.md` のトラブルシューティングに、AC-15 の拒否に遭遇した場合の項目が追加されている。既存の `user_database_source` に関する記述と重複しない。
- **AC-25**: AC-23・AC-24 の英語版が `/mktrans` により反映されている。日本語版を先に作成・コミットする。
- **AC-26**: [98_remaining_issues.md](../0149_security_code_smell_audit_fable/98_remaining_issues.md) §2 の「D1（groupmembership）」から L-2・L-3 が残件として除かれ、解消したことと本タスク・#976 への参照が、同文書が既に D1 M-3・B3 M2・A1・B1 に用いている引用ブロック（`> **… について**`）の形式で記載されている。L-3 について、所見の推奨（対象 GID の行がパース不能ならエラー）をそのままではなく「不完全性の申告」に置き換えて close したことと、その理由が読み取れる。
- **AC-27**: [findings/D1_groupmembership.md](../0149_security_code_smell_audit_fable/findings/D1_groupmembership.md) の L-2・L-3 に本タスクでの対応結果が追記され、`systemd` を許可リストに含めたことによる残存リスクが記録されている。所見の原文は書き換えず、監査時点の記述として残す。
- **AC-28**: `98_remaining_issues.md` の D1 以外の残件（E1・B2・C1・C2・C3・A3・A7 ほか）の記述が、本タスクの書き換えによって増減していない。
- **AC-29**: 「対象外」節で分離した2件（CGO 版 `getpwent` の列挙不完全性、`release.yml` の darwin 非 CGO ビルドと Makefile の不整合）が Issue として登録され、本要件定義書と `98_remaining_issues.md` から参照できる。

## Success Criteria（要件レベル）

- 上記すべての Acceptance Criteria が実装され、対応するテストが CGO・非 CGO 双方のビルドで `make test` により成功する。
- `make lint` が CGO・非 CGO 双方で警告なく通過する。
- 本タスクの前後で、列挙が完全である環境における `CanUserSafelyWriteFile`・`IsUserInGroup`・`CanCurrentUserSafelyReadFile` の外部から観測できる挙動が変わらない。
- 非 CGO ビルドが NSS 環境で group-writable ファイルへの書き込みを拒否したとき、運用者がエラーメッセージだけから原因と回復手段に到達できる。
- #976 の2件それぞれについて、解消したのか、所見の推奨とは異なる形で close したのかが、コードと監査文書の双方から追える。
