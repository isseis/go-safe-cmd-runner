# 実装計画書: groupmembership の列挙完全性の表明と fail-closed 化

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-26 |
| Review date | 2026-08-27 |
| Reviewer | isseis |
| Comments | - |

## 1. 実装概要

### 1.1 目的

[`01_requirements.md`](01_requirements.md)（status: `approved`）の AC-01〜AC-33 を、[`02_architecture.md`](02_architecture.md)（status: `approved`）が定めた設計に従って実装する。中心となる変更は次の3つである。

1. `getGroupMembers` の戻り値に「返した集合が全メンバーを網羅しているか」を同伴させる。
2. 非 CGO ビルドが、`/etc/nsswitch.conf` の内容とプラットフォームから列挙の完全性を決め、パース不能な行の有無と合成する。
3. `isUserOnlyGroupMember` が「完全」以外を拒否し、拒否の原因と回復手段が呼び出し元まで届くようにする。

設計の根拠・図・型定義は `02_architecture.md` にある。本書はそれを重複させず、着手すべき作業と完了の判定条件だけを並べる。

### 1.2 実装原則

- **設計文書を単一の典拠とする**。型の形・分類規則・エラー文面の方針は `02_architecture.md` の該当節を参照し、本書で再定義しない。
- **Go のコメント・識別子・文字列リテラルはすべて英語で書く**（`.claude/commands/_context.md` の Source-language rule）。本書の説明文は日本語だが、それをそのままコードへ持ち込まない。
- **コミットメッセージは英語で書く**（本リポジトリの既存のコミット履歴に揃える）。AC-21 が求める「テストを無効化して失敗を確認した」記録も英語で書き、`disabled` の語を含める。§3 の AC-21 の静的確認はこの語を数える。
- **各 Phase の完了時に `make fmt` → `make test` → `make lint` を通す**。`make test` と `make lint` はいずれも `CGO_ENABLED=1` と `CGO_ENABLED=0` の2構成で実行される（`Makefile` の `lint` ターゲット 150〜157行、`unit-test` ターゲット 477〜484行で確認済み）。したがって新しいビルドタグ付きファイルの型エラーは、それを導入した Phase で必ず表面化する。
- **テストは主張する理由で失敗できることを確認する**（AC-21）。確認の方法は `02_architecture.md` §7.3 の表を出発点とし、本書が §7.3 に無い5件を追加する（Phase 4 の完了判定条件を参照）。

### 1.3 既存コード調査結果

実装前に対象範囲を調査した。以下は「現状どうなっているか」「何が足りないか」「何を変えるか」の対応である。記載のない箇所は変更を要しない。

#### `internal/groupmembership`（本タスクの中心）

| 対象 | 現状 | 変更内容 |
|---|---|---|
| `membership_nocgo.go:21` `getGroupMembers` | `(gid uint32) ([]string, error)`。ビルドタグ `!cgo` | 戻り値を `(groupEnumeration, error)` にする。分類結果と不正行の記録を合成する |
| `membership_cgo.go:309` `getGroupMembers` | `(gid uint32) ([]string, error)`。ビルドタグ `cgo` | 同上。成功時は常に「完全」を申告する |
| `membership_files.go:22` `findGroupByGID` | 対象 GID に一致した時点で `return` するため、その行より後ろの不正行を見ない。ファイルパスが関数内に固定されており `io.Reader` を受け取らない | 走査を `scanGroupFile(io.Reader, ...)` へ分け、末尾まで読み切って `malformedLines` を返す |
| `membership_files.go:82` `findUsersWithPrimaryGID` | 同様にファイルパス固定。不正行は `slog.Warn` のみで呼び出し元へ伝わらない | 同様に `scanPasswdFile(io.Reader, ...)` へ分ける。既存の `slog.Warn` は残す |
| `manager.go:88` `enumerateGroupMembers` フィールド | 型が `func(gid uint32) ([]string, error)` | `func(gid uint32) (groupEnumeration, error)` にする |
| `manager.go:102` `groupMemberCache` | `members []string` と `expiry` のみ | `enumeration groupEnumeration` と `expiry` にする |
| `manager.go:123` `GetGroupMembers` | キャッシュ参照と列挙を兼ねる | キャッシュ層を `getGroupEnumeration` へ切り出し、`GetGroupMembers` はそこからメンバー集合だけを取り出す薄い包みにする。公開シグネチャは変えない |
| `manager.go:204` `isUserOnlyGroupMember` | 集合の長さと要素だけを見る | 完全性の `switch` を先頭に置く |
| `manager.go:517` `EnsurePermissionCheckUID` | `getPermissionCheckUID` のみを呼び、その失敗時は早期 return する | `precomputeEnumerationEnvironment()` の呼び出しを**先頭に**加える。基準 UID の解決に失敗するホストでも分類の警告が出るようにするため |
| `manager.go:63` 付近の sentinel 定義 | `ErrGroupMemberEnumeration` まで定義済み | 新しい2つを同じ場所に並べて追加する |
| `test_helpers.go:7` `newWithEnumerator` | 引数型が `func(gid uint32) ([]string, error)` | 新しい差し替え点の型に合わせる。呼び出し元は `manager_test.go` の**9箇所**（846・869・888・904・941・948・957・973・987行。`rg -c "newWithEnumerator\(" internal/groupmembership/manager_test.go` で確認済み。2箇所を含むテストが2つあるためテスト関数は7つ） |
| `completeness.go`・`nsswitch.go` | **存在しない** | 新規追加する |

#### `internal/groupmembership` のテスト

| 対象 | 現状 | 変更内容 |
|---|---|---|
| `membership_semantics_test.go:16` `shouldSkipSemanticsTest`、`:42` `nssSources` | テスト専用の分類実装。ビルドタグ `cgo && test` | 削除し、production の `readNsswitchSnapshot`・`classifyNSSCompleteness` を呼ぶ |
| `membership_semantics_test.go:69` `TestShouldSkipSemanticsTest` | 上記に対するテーブルテスト（7行） | `nsswitch_test.go` へ移設し、`classifyNSSCompleteness` に対するテーブルテストへ作り替える |
| `membership_semantics_test.go:167`（`TestGetGroupMembers_CGOAndNoCGOSemanticsMatch` 内） | `getGroupMembers` を2値で受ける | 戻り値の変更に追随する。**Phase 1 で対応しないと `cgo && test` 構成がコンパイルできない** |
| `membership_semantics_test.go:175` `fileExpectedMembers` | `findGroupByGID`・`findUsersWithPrimaryGID` を2値で受ける | 走査関数の戻り値変更に追随する。**変更は Phase 4 で行う**（走査関数のシグネチャが変わるのが Phase 4 のため） |
| `membership_nocgo_test.go:159` `TestFindGroupByGID`（内部の複製は `:216`）、`:258` `TestFindUsersWithPrimaryGID`（複製は `:308`）、`:351` `TestFileReadingErrors`（複製は `:353`・`:367`） | **production の走査関数を呼んでいない。** `testFindGroupByGID`／`testFindUsersWithPrimaryGID` という走査ループの複製をテスト内に持ち、一時ファイルへ適用している。このままでは signature を変えてもテストは従来どおり通り、`malformedLines` は一度も検証されない | 複製を削除し、`scanGroupFile`・`scanPasswdFile` を直接呼ぶ形へ置き換える |
| `membership_common_test.go:55`・`:68` | `getGroupMembers` を2値で受ける。ビルドタグなし（両構成で実行される） | 戻り値の変更に追随する |
| `membership_cgo_test.go:176` | 同上。ビルドタグ `cgo` | 追随に加え、「完全」の申告を検証する |
| `manager_test.go` の `newWithEnumerator` 利用箇所 | 9箇所。いずれも `([]string, error)` を返す関数を注入 | 「完全」を申告する値を返す形に更新し、従来の期待を維持する |
| `manager_test.go:263`・`:390`・`:457` | 実環境の列挙で group-writable の許可を確認する3つのサブテスト（`02_architecture.md` §3.7 で特定済み） | 列挙が不完全と申告された場合に `ErrGroupMemberEnumerationIncomplete` を許容するよう更新する |
| `manager_test.go:1005` `TestSudoUIDAdoptionReporter_Report`、`:1031` `..._ReportsOnlyOnce`、`:1228` `TestSudoUIDAdoptionRecordReachesDefaultLogger` | `tu.NewLogRecorder`（`internal/testutil/handlers.go:41`）で属性を検証する既存のテスト。既定ロガーを差し替えるものは `slog.SetDefault` ＋ `t.Cleanup` を使い `t.Parallel` を呼ばない | **変更しない。** Phase 4 で追加する分類警告のテストは、この3つを雛形として流用する |

#### 呼び出し元パッケージ

| 対象 | 現状 | 変更内容 |
|---|---|---|
| `internal/security/dir_permissions_unix.go:211` | `CanUserSafelyWrite` のエラーを `%v` で整形するため、新しい sentinel が `errors.Is` で辿れない | `%v` を `%w` に改める（Phase 3 に完全な前後の文字列を記す） |
| `internal/security/dir_permissions_unix_test.go` | **存在しない**（`internal/security` にあるのは `dir_permissions_audit_test.go` ほか5ファイル）。`02_architecture.md` §3.10 はこのファイルを「変更」と記しているが、実際には新規作成になる | 新規作成する。ビルドタグは `//go:build !windows && test`（同パッケージの `_test.go` は全て `//go:build test` を持ち、対象の `dir_permissions_unix.go` は `//go:build !windows`。同じ組み合わせの先例は `internal/runner/base/privilege/unix_privilege_test.go`） |
| `internal/safefileio/safe_file.go:1187` `rejectionRule` | 既知の sentinel を列挙する `switch`。新しい2つは `default` の `unknown` に落ちる | 分岐を2つ加える |
| `internal/safefileio/safe_file_test.go:1239` `TestRejectionRule` | 既存のテーブルテスト | 新しい2行を加える |
| `internal/runner/base/output/manager.go:257` | `analysis.ErrorMessage = fmt.Sprintf("Permission check failed: %v", err)` | **変更しない。** `02_architecture.md` §5.5 が挙げる3経路の1つだが、値を dry-run の表示メッセージへ描画するだけで `errors.Is` による判別には使わないため、`%v` のままで支障がない。調査済みであることを記録として残す |
| `internal/runner/base/security` | `GetGroupMembers`・`CanUserSafelyWriteFile` を公開シグネチャ経由で使う（`file_validation.go:336` ほか） | **production コードは変更しない。** ただし実環境の列挙に依存するテストは、Phase 4 の強制実行の結果しだいで更新しうる（§5.4 と AC-04a の検証を参照） |

#### 文書

| 対象 | 現状 | 変更内容 |
|---|---|---|
| `docs/user/security-risk-assessment.ja.md:340-345` | 「既知の制限（`CGO_ENABLED=0` ビルド）」が、非 CGO ビルドは NSS を参照しないという事実と自己ビルドの検討を述べる | 本タスク後の挙動（拒否される）へ書き換える |
| `CHANGELOG.ja.md:110-112` | **同じ `## [未リリース]` ブロックの `### セキュリティ` 節に、本タスクが解消する制限をそのまま述べた項目が既にある。** 「実際より緩く評価されることがあります」「`CGO_ENABLED=1` でのセルフビルドを検討してください」と書かれており、本タスクの「破壊的変更」項目と同一リリースの中で矛盾する | 本タスク後の挙動に合わせて書き換えるか、「破壊的変更」の新項目に統合して削除する（AC-32）。英語版 `CHANGELOG.md:110-112` の対応箇所も同時に処理する（AC-33）。未リリースの記述であるため履歴として残す必要はない |
| `docs/user/record_command.ja.md`・`verify_command.ja.md` | トラブルシューティング節あり | 新しい拒否への対処を追加する |
| `CHANGELOG.ja.md:8-10` | 「未リリース」→「破壊的変更」に既存項目が5件。書式は `#### <対象>: <見出し>` ＋ `**影響範囲:**` ＋ アップグレード前の判定手順。`verify` のハッシュディレクトリ fail-closed 化の項目が本変更に最も近い先例 | 同じ書式で1項目を追加する |
| `docs/tasks/0149_.../98_remaining_issues.md:45-49` | §2 の「D1（groupmembership）」に L-2（47行）・L-3（48行）が残件として並び、49行に `- → [#976](…) を作成済み。` の参照がある。解消済み所見は `> **<所見名> について**:` の引用ブロックで記す形式（同文書の15・17・19・53・57・59・101行が先例） | 47〜49行を除き、引用ブロックへ置き換える。49行の #976 参照は引用ブロックの中へ畳み込む |
| `docs/tasks/0149_.../findings/D1_groupmembership.md:83-93` | L-2・L-3 の所見本文。追記の先例は2つあり書式が割れている（`A1_privilege.md:57` は `- **対応状況**:`、`B1_safefileio.md:43` は `- 対応状況:`） | `- **対応状況**:`（太字）の形に統一して追記する。所見の原文は書き換えない |

#### 外部前提の検証結果

| 前提 | 典拠 |
|---|---|
| `make test` が `CGO_ENABLED=1`（`-race`）と `CGO_ENABLED=0` の2構成で `-tags test` を付けて走る | `Makefile` の `unit-test` ターゲット（477〜484行）を直接確認 |
| `make lint` が同じ2構成で `--build-tags test` を付けて走る | `Makefile` の `lint` ターゲット（150〜157行）および `GOLINT` 定義（24行）を直接確認 |
| `unused` linter が有効である | `.golangci.yml:13` を直接確認。**`nsswitchVerdict` の配置根拠になっている**（Phase 4 の「ファイルの分担」と `02_architecture.md` §2.2 を参照） |
| `unit-test-cgo0` は `CGO_ENABLED=0` を自ら設定し、`unit-test` と違って `build-test`・`elfanalyzer-testdata-verify` を前提としない | `Makefile` 494〜495行を直接確認。Phase 4 の強制実行ではこの差を承知のうえで使う |
| 本開発コンテナの `/etc/nsswitch.conf` は `passwd: files` / `group: files` であり、分類は「完全」になる | `/etc/nsswitch.conf` を直接確認。**そのため §3.7 の既存テストの破綻は手元では再現しない。** Phase 4 で分類結果を強制した実行を行う |
| 実行ユーザーは非 root（uid 1000、sudo なし）であり、`/etc/nsswitch.conf` を書き換えて検証することはできない | `id` の出力で確認。強制は一時的なソース改変で行う。一方、`chmod 0000` したファイルを非 root が読めないことは利用できるため、読み取り失敗のテストは実ファイルで再現できる |
| 本リポジトリのコミットメッセージは英語である | `git log` の既存コミットで確認。AC-21 の静的確認を英語前提にした根拠 |
| `#` の行頭コメントが nsswitch.conf の文法であること | `/etc/nsswitch.conf` の現物（1〜5行目）で確認 |
| 行末の `#` コメントと、内部に空白を含む角括弧トークン（`[NOTFOUND=return UNAVAIL=continue]`）が文法上ありうること | nsswitch.conf(5) および glibc「Name Service Switch」の記述に基づく。**本コンテナに man ページが無いため現物では確認していない。** `02_architecture.md` §3.2 が設計判断として確定済みであり、本タスクはその挙動をテストで固定する |

## 2. 実装ステップ

Phase の区切りと順序は `02_architecture.md` §8 の実装優先順位に一致させる。Phase 2・3 を Phase 4 より先に置く理由も同節にある。

### Phase 1: 完全性の型と申告（AC-01, AC-02, AC-04, AC-04a）

この時点では非 CGO 版は暫定的に「完全」を申告し、判定側はまだ完全性を読まない。外部から観測できる挙動は変わらない。

**変更するファイル**

- `internal/groupmembership/completeness.go`（新規、ビルドタグなし）
- `internal/groupmembership/membership_nocgo.go`
- `internal/groupmembership/membership_cgo.go`
- `internal/groupmembership/manager.go`
- `internal/groupmembership/test_helpers.go`
- `internal/groupmembership/completeness_test.go`（新規、ビルドタグなし）
- `internal/groupmembership/membership_common_test.go`
- `internal/groupmembership/membership_cgo_test.go`
- `internal/groupmembership/membership_semantics_test.go`
- `internal/groupmembership/manager_test.go`

**作業内容**

- [ ] `completeness.go` に `enumerationCompleteness`（`completenessUnstated` をゼロ値とする3値）と `String()` を定義する。表示名の形は既存の `PermissionCheckUIDPolicy.String()` に倣い、想定外の値を `unknown(N)` で表す
- [ ] `completeness.go` に `incompletenessCause`（`causeUnspecified` をゼロ値とする4値）と `String()` を定義する
- [ ] `completeness.go` に `completenessVerdict`（フィールドはすべて非公開）と構築関数 `completeVerdict()`・`incompleteVerdict(cause, detail)` を定義する
- [ ] `completeness.go` に `combine` を定義する。「1つでも不完全なら不完全」であり、完全へ戻る経路を持たせない
- [ ] `completeness.go` に `groupEnumeration`（`members []string` と `verdict completenessVerdict`）を定義する
- [ ] `membership_cgo.go` の `getGroupMembers` の戻り値を `(groupEnumeration, error)` にし、成功時に `completeVerdict()` を申告する
- [ ] `membership_cgo.go` に `precomputeEnumerationEnvironment()` の CGO 版（何もしない）を定義する
- [ ] `membership_nocgo.go` の `getGroupMembers` の戻り値を `(groupEnumeration, error)` にする。この Phase では暫定的に `completeVerdict()` を申告する。**TODO コメントは置かず、暫定であることを Phase 1 のコミットメッセージに英語で明記する**（Phase 4 でこの暫定値が確実に置き換わることは、Phase 4 の静的確認と強制実行で担保する）
- [ ] `membership_nocgo.go` に `precomputeEnumerationEnvironment()` の非 CGO 版を定義する。この Phase では本体を空にし、Phase 4 で `nsswitchVerdict()` の呼び出しを入れる
- [ ] `manager.go` の `groupMemberCache` を `enumeration groupEnumeration` と `expiry time.Time` に変える
- [ ] `manager.go` の `enumerateGroupMembers` フィールドの型を `func(gid uint32) (groupEnumeration, error)` に変える
- [ ] `manager.go` にキャッシュ層 `getGroupEnumeration(gid uint32) (groupEnumeration, error)` を切り出す。キャッシュ有効期間・失効処理・エラーをキャッシュしない扱いは現行のままにする
- [ ] `manager.go` の `GetGroupMembers` を `getGroupEnumeration` からメンバー集合だけを取り出す包みにする。**シグネチャ `func (gm *GroupMembership) GetGroupMembers(gid uint32) ([]string, error)` は変えない**
- [ ] `test_helpers.go` の `newWithEnumerator` の引数型を `func(gid uint32) (groupEnumeration, error)` に変える
- [ ] `completeness_test.go` に `combine`・構築関数・`String()` のテストを書く。「1つでも不完全なら不完全」、先に評価した側の原因が残ること、`completeVerdict()` が原因を持たないこと、想定外の値が `unknown(N)` で表示されることを検証する
- [ ] `membership_cgo_test.go` に、CGO 版 `getGroupMembers` が成功時に「完全」を申告することを検証するテストを追加する
- [ ] `membership_cgo_test.go:176` の `TestGetGroupMembers_IncludesPrimaryGroupMembers` を新しい戻り値に追随させる
- [ ] `membership_cgo_test.go` の `TestGetGroupMembers_MergedCountExceedsMaximum` を新しい戻り値に追随させる
- [ ] `membership_common_test.go:55` の `TestGetGroupMembers_Common` を新しい戻り値に追随させる
- [ ] `membership_common_test.go:68` の `TestGetGroupMembers_InvalidGID_Common` を新しい戻り値に追随させる。グループ不在時に空集合とエラーなしが返る既存の契約（AC-04）はここで維持される
- [ ] `membership_semantics_test.go:167` の `getGroupMembers` 呼び出しを新しい戻り値に追随させる。**これを行わないと `cgo && test` 構成がコンパイルできない**
- [ ] `manager_test.go` の `newWithEnumerator` を呼ぶ全9箇所を、「完全」を申告する `groupEnumeration` を返す形へ更新する（対象テストは `TestIsUserOnlyGroupMember_NoSpecialCasing`、`TestIsUserOnlyGroupMember_EnumerationError`、`TestCanUserSafelyWriteFile_EnumerationError`、`TestGetGroupMembers_ErrorNotCached`、`TestIsUserInGroup_NoRegressionWithPrimaryMembers`、`TestIsUserInGroup_EnumerationError`、`TestCanCurrentUserSafelyReadFile_EnumerationError` の7つ）

**完了判定条件**

- [ ] `make fmt` → `make test` → `make lint` がいずれも成功する（2構成とも）
- [ ] `rg -c 'func \(gm \*GroupMembership\) GetGroupMembers\(gid uint32\) \(\[\]string, error\)' internal/groupmembership/manager.go` が `1` を返す
- [ ] `git diff --stat main...HEAD -- internal/runner/base/security ':!*_test.go'` が空である

### Phase 2: 判定側のフェイルクローズド化（AC-03, AC-03a, AC-14, AC-15, AC-16, AC-17, AC-18, AC-19）

**変更するファイル**

- `internal/groupmembership/manager.go`
- `internal/groupmembership/manager_test.go`

**作業内容**

- [ ] `manager.go` の既存 sentinel 定義の並びに `ErrGroupMemberEnumerationIncomplete` を追加する
- [ ] 同じ並びに `ErrGroupMemberCompletenessUnstated` を追加する
- [ ] `isUserOnlyGroupMember` を `getGroupEnumeration` の呼び出しに切り替え、完全性の `switch` を判定より手前に置く。分岐の対応は `02_architecture.md` §3.6 の表に従う。`default` は `ErrGroupMemberCompletenessUnstated` 側へ倒す
- [ ] 不完全時のエラーメッセージ生成を、`incompletenessCause` に対する `switch` で組み立てる。文字列の内容から分岐させない。`user_database_source` の値を必ず含める。文面の方針と原因ごとの回復手段は `02_architecture.md` §4.3 の表に従う
- [ ] 「未申告」のメッセージに `enumerationCompleteness.String()` の値を載せ、環境要因ではなく実装の誤りであることを示す
- [ ] **`completenessVerdict`・`groupEnumeration` を構造体のまま `slog.Any` へ渡さない**（`02_architecture.md` §4.4）。ログに出すのは `user_database_source`・`cause.String()`・`detail` の個別属性とする
- [ ] `manager_test.go` に、`newWithEnumerator` へ「不完全」を申告する値を注入し、`isUserOnlyGroupMember` が本人が唯一の要素であっても拒否することを検証するテストを追加する
- [ ] `manager_test.go` に、「未申告」（`groupEnumeration{}` をそのまま返す）を注入し、`ErrGroupMemberCompletenessUnstated` が返ることと、`ErrGroupMemberEnumerationIncomplete` とは `errors.Is` で区別できることを検証するテストを追加する。未定義の `enumerationCompleteness` 値を注入して `default` 分岐にも到達させる
- [ ] `manager_test.go` に、不完全な列挙で `CanUserSafelyWriteFile` が `(false, non-nil error)` を返し、`errors.Is` で sentinel を判別できることを検証するテストを追加する
- [ ] `manager_test.go` に `TestIncompleteEnumerationErrorMessage` を追加する。`causeUnsupportedPlatform`・`causeNSSSources`・`causeMalformedLine` の各原因について、メッセージに `user_database_source` の値が含まれること、および `causeNSSSources` では回復手段として `CGO_ENABLED=1` が現れることを検証する
- [ ] `manager_test.go` に、キャッシュヒット時（同じ GID を2回呼ぶ）にも完全性が保たれ、同じ拒否になることを検証するテストを追加する。注入する関数にクロージャで呼び出し回数のカウンタを持たせ、2回目が列挙を再実行していないことも併せて検証する
- [ ] `manager_test.go` に、不完全な列挙を注入しても `IsUserInGroup` と `CanCurrentUserSafelyReadFile` の結果が完全な列挙の場合と一致することを検証するテストを追加する
- [ ] `manager_test.go:263` のサブテスト「group writable file - owner only allowed if exclusive group member」を、`ErrGroupMemberEnumerationIncomplete` を許容する形へ更新する
- [ ] `manager_test.go:390` のサブテスト「group_writable_member」を同様に更新する
- [ ] `manager_test.go:457` の `various_permission_combinations` の行「group_read_write」（`0o664`）を同様に更新する

**完了判定条件**

- [ ] `make fmt` → `make test` → `make lint` がいずれも成功する（2構成とも）
- [ ] `02_architecture.md` §7.3 の表のうち「不完全での拒否」「未申告での拒否」「キャッシュを跨ぐ完全性」「読み取り経路の不変」の4行について、分岐を無効化して該当テストが失敗することを確認し、コミットメッセージに英語で記す
- [ ] §7.3 に無い追加の無効化確認: エラーメッセージ生成から `user_database_source` を落とすと `TestIncompleteEnumerationErrorMessage` が失敗する（AC-18）

### Phase 3: 診断可能性（AC-15, AC-18）

拒否がまだ実際には起きない段階で、診断の経路を先に整える。

**変更するファイル**

- `internal/security/dir_permissions_unix.go`
- `internal/security/dir_permissions_unix_test.go`（新規、`//go:build !windows && test`）
- `internal/safefileio/safe_file.go`
- `internal/safefileio/safe_file_test.go`

**作業内容**

- [ ] `dir_permissions_unix.go` の `validateGroupWritePermissions` 内の包装を次のとおり書き換える。
      変更前: `return fmt.Errorf("%w: directory %s failed security validation: %v", ErrInvalidDirPermissions, dirPath, err)`
      変更後: `return fmt.Errorf("%w: directory %s failed security validation: %w", ErrInvalidDirPermissions, dirPath, err)`
- [ ] `dir_permissions_unix_test.go` を新規作成する。ビルドタグは `//go:build !windows && test`（同パッケージの慣行と対象ファイルの `!windows` の両方に合わせる。先例は `internal/runner/base/privilege/unix_privilege_test.go`）
- [ ] 同ファイルに `TestValidateDirectoryPermissionsWithOptions_PropagatesEnumerationSentinel` を書く。`CanUserSafelyWrite` が `ErrGroupMemberEnumerationIncomplete` を返す `DirectoryPermCheckOptions` を与え、`ValidateDirectoryPermissionsWithOptions` の返すエラーから `errors.Is` でその sentinel を辿れることを検証する
- [ ] 同じテストの中で、既存の `ErrInvalidDirPermissions` に対する `errors.Is` が引き続き成立することを併せて検証する
- [ ] `safe_file.go:1187` の `rejectionRule` の `switch` に、`errors.Is(cause, groupmembership.ErrGroupMemberEnumerationIncomplete)` → `"enumeration-incomplete"` の分岐を追加する
- [ ] 同じ `switch` に、`errors.Is(cause, groupmembership.ErrGroupMemberCompletenessUnstated)` → `"completeness-unstated"` の分岐を追加する
- [ ] `safe_file_test.go:1239` の `TestRejectionRule` のテーブルに、上記2つの sentinel に対する行を追加する

**完了判定条件**

- [ ] `make fmt` → `make test` → `make lint` がいずれも成功する（2構成とも）
- [ ] `02_architecture.md` §7.3 の表の「sentinel の伝播」の行について、`%w` を `%v` へ戻すと `dir_permissions_unix_test.go` のテストが失敗することを確認し、コミットメッセージに英語で記す

### Phase 4: 環境の分類と不正行（AC-05〜AC-13, AC-20）

非 CGO 版の申告を、Phase 1 で置いた暫定の「完全」から実際の分類結果へ差し替える。

**変更するファイル**

- `internal/groupmembership/nsswitch.go`（新規、`//go:build !cgo || test`）
- `internal/groupmembership/nsswitch_test.go`（新規、`//go:build !cgo || test`）
- `internal/groupmembership/membership_files.go`
- `internal/groupmembership/membership_nocgo.go`
- `internal/groupmembership/manager.go`
- `internal/groupmembership/membership_nocgo_test.go`
- `internal/groupmembership/membership_semantics_test.go`

**ファイルの分担（`02_architecture.md` §2.2 の改訂を反映）**

プロセス単位の状態を持つものと持たないものを、ビルドタグで分ける。

| 置く先 | 内容 |
|---|---|
| `nsswitch.go`（`!cgo \|\| test`） | `nsswitchState`・`nsswitchSnapshot`、`readNsswitchSnapshotFrom`・`readNsswitchSnapshot`、`nssSources`、`classifyNSSCompleteness`、警告を組み立てる `nssCompletenessReporter` 型。**いずれもプロセス単位の状態を持たない** |
| `membership_nocgo.go`（`!cgo`） | `nsswitchVerdict()`、および `nssCompletenessReporter` の package レベルの共有インスタンス。`enumerateFromFiles`、`precomputeEnumerationEnvironment` |

`nsswitchVerdict()` を `nsswitch.go` に置かないのは、`nsswitch.go` が `!cgo || test` のため CGO 構成でもコンパイルされる一方、その構成には呼び出す production コードが無く、`.golangci.yml:13` で有効な `unused` に報告されうるためである。`//nolint` で伏せない。分類そのものが非 CGO ビルドでしか意味を持たないため、`membership_nocgo.go` へ置くほうが設計としても素直である（`02_architecture.md` §2.2）。

**作業内容**

- [ ] `nsswitch.go` に `nsswitchState`（`nsswitchUnread` をゼロ値とする4値）と `nsswitchSnapshot` を定義する
- [ ] `nsswitch.go` に `readNsswitchSnapshotFrom(path string) nsswitchSnapshot` を実装し、`readNsswitchSnapshot()` を「`/etc/nsswitch.conf` を渡して呼ぶだけ」の包みにする。`02_architecture.md` §3.2 が宣言する `readNsswitchSnapshot()` のシグネチャはそのまま残る（パス受け取りは非公開の下位関数として追加するだけであり、設計の変更にはあたらない）。**この分離が無いと、読み取り失敗と不在の判別をテストできない**
- [ ] `readNsswitchSnapshotFrom` で、**「存在しない」と見なす条件を `errors.Is(err, fs.ErrNotExist)` に限り**、それ以外の失敗はすべて `nsswitchReadFailed` に落とす
- [ ] `nsswitch.go` に `nssSources(content, database string) []string` を実装する。データベース行のトークン化の前に行末の `#` コメントを切り捨て、角括弧の対応（`[` から対応する `]` まで）を1つのトークンとして扱い内部の空白で分割しない（`02_architecture.md` §3.2）
- [ ] `nsswitch.go` に `classifyNSSCompleteness(snapshot, goos)` を実装する。分類規則は `02_architecture.md` §3.2 の表に上から順で従い、`switch` の `default` を「不完全」へ倒す。**ファイルシステムに触れない**
- [ ] 許可リストを `files`・`systemd` の2つに限る。`compat`・`db`・`ldap`・`sss`・`nis`・`winbind` および未知の名前をすべて「不完全」とする（ブロックリスト方式にしない）
- [ ] **`membership_nocgo.go`** に `nsswitchVerdict()` を実装する。最初の呼び出しで読み取りと分類を行い、以後は同じ結果を返す。`nsswitch.go` には置かない（上記「ファイルの分担」）
- [ ] 警告の生成を `nsswitchVerdict()` から切り離し、既存の `sudoUIDAdoptionReporter`（`manager.go:414`）と同じ形の小さな型 `nssCompletenessReporter` に持たせる。`atomic.Bool` で1回限りを保証し、`report(logger *slog.Logger, v completenessVerdict)` の形で**ロガーを引数で受け取る**。production では package レベルの共有インスタンスを `nsswitchVerdict()` の確定時に呼ぶ。**この形にする理由**: `nsswitchVerdict()` はプロセス単位で latch するうえ、警告は分類が「不完全」のときしか出ない。本コンテナの分類は「完全」で、かつ production へ強制用の seam を足さない方針（§5.3）のため、`nsswitchVerdict()` 越しにテストすると記録が1件も出ないまま素通りする。型を分ければテストが自前のインスタンスと自前のロガーに任意の `incompleteVerdict(...)` を渡せる
- [ ] `nssCompletenessReporter.report` が、分類が「不完全」の場合に `user_database_source`・`cause.String()`・`detail` の3つを**個別の属性として**持つ `slog.Warn` を1件出す。「完全」の場合は何も出さない
- [ ] `membership_nocgo.go` の `precomputeEnumerationEnvironment()` の本体を `nsswitchVerdict()` の呼び出しに差し替える
- [ ] `manager.go` の `EnsurePermissionCheckUID` の**先頭**で `precomputeEnumerationEnvironment()` を呼ぶ。`getPermissionCheckUID` が失敗して早期 return するホストでも警告が出るようにする
- [ ] `membership_files.go` に `malformedLines`（`count int`、`first string`）と、その `verdict()` メソッドを定義する
- [ ] `membership_files.go` の走査を `scanGroupFile(r io.Reader, source string, gid uint32) (*groupEntry, malformedLines, error)` へ切り出す。**対象エントリに一致してもファイル末尾まで読み切る**。一致したエントリは保持する
- [ ] `membership_files.go` の走査を `scanPasswdFile(r io.Reader, source string, gid uint32) ([]string, malformedLines, error)` へ切り出す
- [ ] `findGroupByGID`・`findUsersWithPrimaryGID` を、ファイルを開いて上記へ渡すだけの薄い包みにする。戻り値に `malformedLines` を加える
- [ ] 既存の `slog.Warn`（`membership_files.go:40`・`:101`）の出力内容（属性 `file`・`line`・`error`）を変えない
- [ ] 空行と `#` で始まる行を不正行として数えない現行の扱いを維持する
- [ ] `membership_nocgo.go` に `enumerateFromFiles(gid uint32, nssVerdict completenessVerdict) (groupEnumeration, error)` を実装し、分類結果と不正行の記録を `combine` で合成する。**分類を先に評価する**（両方が不完全な場合は分類の原因が残る）
- [ ] `membership_nocgo.go` の `getGroupMembers` を `return enumerateFromFiles(gid, nsswitchVerdict())` の形にし、Phase 1 で置いた暫定の `completeVerdict()` を取り除く

**テストの作業内容**

- [ ] `nsswitch_test.go` に `TestClassifyNSSCompleteness` を書く。`membership_semantics_test.go:69` の7行を移設したうえで、`02_architecture.md` §3.8 の対応表が挙げる10通りの環境を網羅する（`darwin`／`linux` 以外のその他／不在／読み取り失敗／`files` のみ／`files systemd`／`sss` や `ldap` を含む／`passwd` または `group` の行が無い／角括弧トークン付随／角括弧のみでソース名なし）。ゼロ値 `nsswitchUnread` の行も加え、`nsswitchState` の4値すべてを覆う
- [ ] `nsswitch_test.go` に `TestNSSSources` を書く。行末コメント（`files systemd # local users only` が `["files", "systemd"]` になること）と、内部に空白を含む角括弧トークン（`files [NOTFOUND=return UNAVAIL=continue] systemd` が `["files", "systemd"]` になること）を検証する
- [ ] `nsswitch_test.go` に `TestReadNsswitchSnapshotFrom` を書く。`t.TempDir()` 配下で、(1) 存在しないパス → `nsswitchAbsent`、(2) `chmod 0000` したファイル → `nsswitchReadFailed`（非 root で実行されるため再現できる）、(3) 読める内容のファイル → `nsswitchRead` と内容の一致、の3件を検証する
- [ ] `nsswitch_test.go` に `TestNSSCompletenessReporter_Report` を書く。自前の `nssCompletenessReporter` の値と `tu.NewLogRecorder`（`internal/testutil/handlers.go:41`）のロガーへ、合成した `incompleteVerdict(causeNSSSources, ...)` を渡し、属性が `user_database_source`・`cause`・`detail` の**個別のキー**として記録されることを検証する。雛形は `manager_test.go:1005` の `TestSudoUIDAdoptionReporter_Report`。ロガーを引数で受けるため `t.Parallel` を使ってよい
- [ ] `nsswitch_test.go` に `TestNSSCompletenessReporter_ReportsOnlyOnce` を書く。同じインスタンスへ3回渡して記録が1件であることを検証する。雛形は `manager_test.go:1031` の `TestSudoUIDAdoptionReporter_ReportsOnlyOnce`
- [ ] `nsswitch_test.go` に、分類が「完全」の場合に `report` が何も記録しないことを検証するテストを書く
- [ ] `membership_nocgo_test.go`（`//go:build !cgo`）に、`EnsurePermissionCheckUID()` が `precomputeEnumerationEnvironment()` を経由して分類を評価することを検証するテストを追加する。**`manager_test.go` ではなくこのファイルに置く**——`manager_test.go` はビルドタグを持たず CGO 構成でも走るが、CGO 版の `precomputeEnumerationEnvironment()` は設計上なにもしないため（`02_architecture.md` §2.2）、そこに置くと CGO 構成で意味を失う
- [ ] `membership_semantics_test.go:16` の `shouldSkipSemanticsTest` を削除する
- [ ] `membership_semantics_test.go:42` の `nssSources` を削除する
- [ ] `membership_semantics_test.go:69` の `TestShouldSkipSemanticsTest` を削除する（移設先は `nsswitch_test.go`）
- [ ] `membership_semantics_test.go` の `TestGetGroupMembers_CGOAndNoCGOSemanticsMatch` を、`readNsswitchSnapshot` と `classifyNSSCompleteness` を呼び、結果が「完全」でない場合に skip する形へ書き換える。skip の理由は `incompletenessCause.String()` と `detail` から組み立てる
- [ ] `membership_semantics_test.go:175` の `fileExpectedMembers` を、走査関数の戻り値変更に追随させる
- [ ] `membership_nocgo_test.go:159` の `TestFindGroupByGID` から走査ループの複製 `testFindGroupByGID`（`:216`）を削除し、`scanGroupFile` を直接呼ぶ形に置き換える
- [ ] `membership_nocgo_test.go:258` の `TestFindUsersWithPrimaryGID` から走査ループの複製 `testFindUsersWithPrimaryGID`（`:308`）を削除し、`scanPasswdFile` を直接呼ぶ形に置き換える
- [ ] `membership_nocgo_test.go:351` の `TestFileReadingErrors` を、`scanGroupFile`・`scanPasswdFile` に読み取り中にエラーを返す `io.Reader`（`iotest.ErrReader`）を渡し、`scanner.Err()` の分岐が呼び出し元へ伝わることを検証する形へ書き換える。**現行の「存在しないパスを開く」検証は残さない**——`findGroupByGID`・`findUsersWithPrimaryGID` は `/etc/group`・`/etc/passwd` を固定で開く薄い包みになる。テストが動くホストではこれらのファイルは常に存在するため、open 失敗を誘発できないからである。書き換え後に `go tool cover -func` を実行し、`membership_files.go` の各関数のカバレッジが書き換え前から下がっていないことを確認する
- [ ] `membership_nocgo_test.go` に、不正行を含む内容で `malformedLines` の件数と最初の位置（`file:line`）が記録されることを検証するテストを追加する
- [ ] 同ファイルに、**不正行が対象エントリより後ろにある場合も記録される**ことを検証するテストを追加する。これは「走査を末尾まで読み切る」設計を、打ち切る実装に戻せば失敗する形で固定する
- [ ] 同ファイルに、空行・コメント行が不正行として数えられないことを検証するテストを追加する
- [ ] 同ファイルに、NIS 互換エントリ（`+`・`+@netgroup`・`-username` で始まる行）が不正行として数えられることを検証するテストを追加する
- [ ] 同ファイルに、不正行に対する `slog.Warn` が属性 `file`・`line`・`error` を伴って従来どおり出力されることを検証するテストを追加する。`membership_files.go` はパッケージレベルの `slog.Warn` を呼ぶため、`slog.SetDefault` ＋ `t.Cleanup` で `tu.NewLogRecorder` を差し込み、`t.Parallel` を呼ばない
- [ ] 同ファイルに、`enumerateFromFiles` へ「完全」の分類結果を渡しても不正行があれば「不完全」になること、およびその逆（分類が「不完全」なら不正行が無くても「不完全」）を検証するテストを追加する

**完了判定条件**

- [ ] `make fmt` → `make test` → `make lint` がいずれも成功する（2構成とも）
- [ ] 非 CGO 版の配線が残っていることを静的に確認する: `rg -n 'enumerateFromFiles\(gid, nsswitchVerdict\(\)\)' internal/groupmembership/membership_nocgo.go` が1件一致し、`rg -c 'completeVerdict\(\)' internal/groupmembership/membership_nocgo.go` が `0` を返す（Phase 1 の暫定値が確実に置き換わったこと）
- [ ] **分類結果を強制した実行を、陽性対照つきで行う。** 本開発コンテナの `/etc/nsswitch.conf` は `files` のみであり実行ユーザーは非 root であるため、設定ファイルの書き換えでは強制できない。`classifyNSSCompleteness` が常に `incompleteVerdict(causeNSSSources, ...)` を返すよう一時的にソースを改変し、`make unit-test-cgo0` を実行する。
      **陽性対照**: この状態で `manager_test.go:390` の `group_writable_member` が `ErrGroupMemberEnumerationIncomplete` の経路へ入ることを確認する。**入らない場合は、非 CGO 版 `getGroupMembers` の配線が欠けている証拠として扱い、ゲートを不合格とする**（「何も壊れなかった」を合格としない）。
      あわせて `02_architecture.md` §3.7 が挙げた package 外の4ファイル（`internal/safefileio/safe_file_test.go`、`internal/runner/base/security/file_validation_test.go`、`internal/runner/base/security/destination_zoning_test.go`、`internal/runner/base/security/trusted_gids_linux_test.go`）の結果を確認し、破綻したテストを Phase 4 の作業として更新する。確認後に改変を戻す
- [ ] `02_architecture.md` §7.3 の表のうち「分類」「読み取り失敗の扱い」「不正行の伝達」「対象エントリより後ろの不正行」「合成」の5行について、分岐を無効化して該当テストが失敗することを確認し、コミットメッセージに英語で記す
- [ ] §7.3 に無い追加の無効化確認を4件行い、同じくコミットメッセージに記す。(1) 行末 `#` の切り捨てを外す → `TestNSSSources` が失敗（AC-09）。(2) 空行・コメント行を不正行として数える → 該当テストが失敗（AC-13）。(3) `membership_files.go` の `slog.Warn` を削る → 警告のテストが失敗（AC-12）。(4) 分類表からプラットフォームの行を削る → `TestClassifyNSSCompleteness` が失敗（AC-06）
- [ ] 警告の属性を `slog.Any("verdict", v)` の1つにまとめると `TestNSSCompletenessReporter_Report` が失敗することを確認する（`02_architecture.md` §4.4 の制約を、破れば落ちる形で固定できていることの確認）

### Phase 5: 文書と監査記録（AC-23〜AC-33）

日本語版を先に作成・コミットし、英語版は `/mktrans` で反映する（`CLAUDE.md`「Translation Guidelines」）。

**変更するファイル**

- `docs/user/security-risk-assessment.ja.md` / `docs/user/security-risk-assessment.md`
- `docs/user/record_command.ja.md` / `docs/user/record_command.md`
- `docs/user/verify_command.ja.md` / `docs/user/verify_command.md`
- `CHANGELOG.ja.md` / `CHANGELOG.md`
- `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md`
- `docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md`
- `docs/tasks/0168_groupmembership_nocgo_enumeration_completeness/01_requirements.md`

**作業内容**

- [ ] **最初に** 「対象外」で分離した2件（CGO 版 `getpwent` の列挙不完全性、`release.yml` の darwin 非 CGO ビルドと `Makefile` の不整合）を GitHub Issue として登録する。以降の文書作業で番号を参照するため先に行う
- [ ] `security-risk-assessment.ja.md:340-345` の「既知の制限（`CGO_ENABLED=0` ビルド）」を書き換える。NSS 環境の非 CGO ビルドでは group-writable ファイルへの書き込みが「緩く評価される場合がある」のではなく「拒否される」こと、および `files`・`systemd` のみの環境では従来どおり判定できることが読み取れるようにする
- [ ] **`CHANGELOG.ja.md:110-112` の「既知の制限: 公式バイナリ（`CGO_ENABLED=0`）はグループメンバーシップで NSS を参照しない」を解消する（AC-32）。** これは同じ `## [未リリース]` ブロックの `### セキュリティ` 節にあり、本タスクが解消する制限を「実際より緩く評価されることがあります」と述べているため、放置すると同一リリースの中で新しい「破壊的変更」項目と矛盾する。新項目へ統合して削除するか、本タスク後の挙動に合わせて書き換える。まだリリースされていない記述であるため、履歴として残す必要はない
- [ ] `record_command.ja.md` のトラブルシューティングに、AC-15 の拒否に遭遇した場合の項目を追加する。原因は NSS 環境・不正行・macOS の3種であり、回復手段がそれぞれ異なるため項目を分けて書く。既存の `user_database_source` に関する記述と重複させない
- [ ] `verify_command.ja.md` のトラブルシューティングに同じ項目を追加する
- [ ] `CHANGELOG.ja.md` の「未リリース」→「破壊的変更」に項目を追加する。書式は同節の既存項目（とくに `verify`: ハッシュディレクトリの権限違反を fail-closed 化）に揃え、見出しで対象範囲を示し、`**影響範囲:**` を設け、アップグレード前に影響有無を判定する手順（`/etc/nsswitch.conf` の `passwd`・`group` 行、`/etc/passwd`・`/etc/group` の不正行の有無、対象パスの group-writable な構成要素の3点）と回復手段・切り戻し方法を添える
- [ ] 上記の日本語版をまとめてコミットする
- [ ] `/mktrans` で `security-risk-assessment.md`・`record_command.md`・`verify_command.md`・`CHANGELOG.md` へ反映する。`CHANGELOG.md:110-112` の英語版の項目 "Known limitation: official binaries (`CGO_ENABLED=0`) do not consult NSS for group membership" も日本語版と同じ扱いで解消する（AC-33）
- [ ] `98_remaining_issues.md` §2「D1（groupmembership）」から L-2（47行）・L-3（48行）の箇条書きと、49行の `- → [#976](…) を作成済み。` を除く
- [ ] 同文書に `> **D1 L-2/L-3 について**:` の引用ブロックを追加する。同文書の既存の引用ブロック（15・17・19・53・57・59・101行）と同じ形式にし、#976 への参照を中へ畳み込む。L-3 について、所見の推奨（対象 GID の行がパース不能ならエラー）をそのままではなく「不完全性の申告」に置き換えて close したことと、その理由（不正行がどの GID のものかは原理的に判定できない）が読み取れるようにする
- [ ] `98_remaining_issues.md` の **§2 の D1 の節に**、分離した2件の Issue を追加する。E1・B2・C1・C2・C3・A3・A7 の各節には触れない（AC-28）
- [ ] `findings/D1_groupmembership.md` の L-2 に `- **対応状況**:` の箇条書きを追記する（`A1_privilege.md:57` の太字形式に統一）。**所見の原文は書き換えない**
- [ ] 同文書の L-3 に同様に追記する。あわせて `systemd` を許可リストに含めたことによる残存リスク（`systemd-homed` のユーザーが保護対象ファイルのグループを共有する構成）を記録する
- [ ] 登録した2件の Issue 番号を `01_requirements.md` の「対象外」節へ追記する

**完了判定条件**

- [ ] §3 の受け入れ基準検証表のうち AC-23〜AC-33 の確認がすべて期待どおりの結果になる
- [ ] `git diff main...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` の差分が、D1 の節と分離した2件の追記だけに収まっている（AC-28）

## 3. 受け入れ基準の検証

各受け入れ基準に対する検証を、`test`（実行可能・誤った挙動で失敗する）、`static`（コマンドの結果が機械的に判定できる）、`manual`（人による確認が必要）で分類する。テスト名は実装時に確定するため、下表の名称を実装の指針とする。

**静的確認コマンドの前提**: 下表の `rg` はすべて既定設定（Rust 正規表現）で実行する。`|` は素で書けば選択、`\|` はリテラルのパイプになる点に注意する。

| AC | 種別 | 実装箇所 | 検証 |
|---|---|---|---|
| AC-01 | test | `completeness.go` | `completeness_test.go::TestCompletenessVerdict_Combine`（1つでも不完全なら不完全、先に評価した原因が残る）、`::TestEnumerationCompleteness_String`（ゼロ値が `unstated`、想定外の値が `unknown(N)`） |
| AC-02 | test | `membership_cgo.go` | `membership_cgo_test.go::TestGetGroupMembers_StatesComplete`（CGO 構成でのみ実行） |
| AC-03 | test | `manager.go` `isUserOnlyGroupMember` | `manager_test.go::TestIsUserOnlyGroupMember_Completeness`。未定義の `enumerationCompleteness` 値で `default` に到達し拒否されることを含む |
| AC-03a | test | 同上 | 同テスト内で `errors.Is(err, ErrGroupMemberCompletenessUnstated)` が真かつ `errors.Is(err, ErrGroupMemberEnumerationIncomplete)` が偽であることを検証する |
| AC-04 | test | `manager.go`・`membership_*.go` | `membership_common_test.go::TestGetGroupMembers_InvalidGID_Common`（グループ不在で空集合・エラーなし）、`manager_test.go::TestIsUserOnlyGroupMember_EnumerationError`（列挙エラーの伝播） |
| AC-04a | static | `manager.go` `GetGroupMembers` | `rg -c 'func \(gm \*GroupMembership\) GetGroupMembers\(gid uint32\) \(\[\]string, error\)' internal/groupmembership/manager.go` が `1`。`git diff --stat main...HEAD -- internal/runner/base/security ':!*_test.go'` が空（production コードは無変更。実環境の列挙に依存するテストの更新は本タスクの挙動変更の当然の帰結であり、この保証の対象外とする） |
| AC-04a | test | 同上 | `CGO_ENABLED=0 go test -tags test ./internal/runner/base/security/...` と `CGO_ENABLED=1 go test -tags test ./internal/runner/base/security/...` の双方が成功する。`internal/safefileio` については、`rejectionRule` への2行追加は診断のためであり判定ロジックを変えない（`02_architecture.md` §4.5）。`safe_file_test.go` の既存の行は変更しない |
| AC-05 | test | `nsswitch.go` `classifyNSSCompleteness` | `nsswitch_test.go::TestClassifyNSSCompleteness`（`linux` かつ `files`／`files systemd` の行が「完全」になる） |
| AC-05 | static | `membership_nocgo.go` `getGroupMembers` | 非 CGO 版の配線: `rg -c 'enumerateFromFiles\(gid, nsswitchVerdict\(\)\)' internal/groupmembership/membership_nocgo.go` が `1`、かつ `rg -c 'completeVerdict\(\)' internal/groupmembership/membership_nocgo.go` が `0`（Phase 1 の暫定値が残っていないこと） |
| AC-05 | manual | 同上 | Phase 4 の陽性対照つき強制実行（分類を「不完全」に固定したとき `manager_test.go:390` が拒否経路へ入ること）。自動テストで代替できないのは、強制の seam を production へ追加しない方針のため |
| AC-06 | test | `classifyNSSCompleteness` | `::TestClassifyNSSCompleteness`（`goos` が `darwin` および `linux` 以外のその他で `causeUnsupportedPlatform` の「不完全」になる行） |
| AC-07 | test | `classifyNSSCompleteness` | 同上（`nsswitchAbsent` が「完全」、`nsswitchReadFailed`・`passwd` 行欠落・`group` 行欠落・ゼロ値 `nsswitchUnread` が「不完全」になる各行） |
| AC-07 | test | `nsswitch.go` `readNsswitchSnapshotFrom` | `nsswitch_test.go::TestReadNsswitchSnapshotFrom`（不在 → `nsswitchAbsent`、`chmod 0000` → `nsswitchReadFailed`、通常 → `nsswitchRead`）。**読み取り失敗を不在と取り違えると「完全」の申告に化けるため、分類表だけでは AC-07 を検証したことにならない** |
| AC-08 | test | `classifyNSSCompleteness` | `::TestClassifyNSSCompleteness`（`ldap`・`sss`・`nis`・`winbind`・`db`・`compat`・未知の名前の各行が「不完全」になる） |
| AC-08 | manual | 同上 | 許可リストの定義箇所を読み、列挙されるソース名が `files` と `systemd` の2つだけであり、危険なソース名を列挙するブロックリストが存在しないことを確認する（`rg` では定義箇所への限定ができないため人が読む） |
| AC-09 | test | `nsswitch.go` `nssSources` | `nsswitch_test.go::TestNSSSources`（行末コメントと内部に空白を含む角括弧トークン）、`::TestClassifyNSSCompleteness`（`group: files [NOTFOUND=continue]` が「完全」、`group: [NOTFOUND=return]` が「不完全」） |
| AC-10 | test | `classifyNSSCompleteness` | `::TestClassifyNSSCompleteness` が `nsswitchSnapshot` を組み立てて渡すだけで全条件を網羅する（ファイルを作らず `t.TempDir` も使わない）。ファイルを使うのは `TestReadNsswitchSnapshotFrom` のみ |
| AC-10 | manual | 同上 | `classifyNSSCompleteness` と `nssSources` の本体を読み、`os.` の呼び出しが無いこと（ファイルに触れるのは `readNsswitchSnapshotFrom` だけであること）を確認する。固定行数の `rg -A` は関数長が変われば隣の関数へはみ出すため使わない |
| AC-11 | test | `membership_files.go` `scanGroupFile`・`scanPasswdFile` | `membership_nocgo_test.go::TestScanGroupFile_MalformedLines`（件数と最初の位置）、`::TestScanGroupFile_MalformedLineAfterMatch`（対象エントリより後ろの不正行）、`::TestScanPasswdFile_MalformedLines`、`::TestScanGroupFile_NISCompatEntries` |
| AC-12 | test | `membership_files.go:40`・`:101` | `membership_nocgo_test.go::TestScanGroupFile_WarnsOnMalformedLine`。`tu.NewLogRecorder` を `slog.SetDefault` ＋ `t.Cleanup` で差し込み、属性 `file`・`line`・`error` を検証する（`t.Parallel` を呼ばない） |
| AC-13 | test | `scanGroupFile`・`scanPasswdFile` | `::TestScanGroupFile_BlankAndCommentLines`（空行と `#` 行が `malformedLines.count` を増やさず、結果が「完全」のままであること） |
| AC-14 | test | `isUserOnlyGroupMember` | `manager_test.go::TestIsUserOnlyGroupMember_Completeness` のうち、メンバー集合が本人1名のみでも「不完全」なら拒否される行 |
| AC-15 | test | `manager.go`・`dir_permissions_unix.go:211` | `manager_test.go::TestCanUserSafelyWriteFile_IncompleteEnumeration`（`(false, non-nil error)` と `errors.Is`）、`internal/security/dir_permissions_unix_test.go::TestValidateDirectoryPermissionsWithOptions_PropagatesEnumerationSentinel`（包装を越えて辿れること） |
| AC-16 | test | `CanUserSafelyWriteFile` | `manager_test.go::TestCanUserSafelyWriteFile`（world-writable・非所有者・owner-writable・group-writable かつ唯一のメンバーの各分岐が従来どおり）。「完全」を注入した状態で既存の期待が変わらないことを確認する |
| AC-17 | test | `IsUserInGroup`・`CanCurrentUserSafelyReadFile` | `manager_test.go::TestReadPathIgnoresCompleteness`（不完全を注入しても結果が完全な場合と一致する） |
| AC-18 | test | `manager.go` のメッセージ生成、`safe_file.go:1187` | `manager_test.go::TestIncompleteEnumerationErrorMessage`（原因ごとに `user_database_source` の値が含まれ、`causeNSSSources` では `CGO_ENABLED=1` が回復手段として現れる）、`internal/safefileio/safe_file_test.go::TestRejectionRule`（新しい2つが `unknown` にならない） |
| AC-18 | test | `nsswitch.go` `nssCompletenessReporter` | `nsswitch_test.go::TestNSSCompletenessReporter_Report`（`user_database_source`・`cause`・`detail` が個別の属性として記録される）、`::TestNSSCompletenessReporter_ReportsOnlyOnce`（1回限り）、`membership_nocgo_test.go::TestEnsurePermissionCheckUIDPrecomputesEnvironment`。**ロガーと verdict を引数で受ける型に切り出すため、実行環境の NSS 構成に依存せずに検証できる** |
| AC-19 | test | `manager.go` `getGroupEnumeration` | `manager_test.go::TestCompletenessSurvivesCache`（同じ GID を2回呼び、2回目も同じ拒否になる。注入関数がクロージャで持つカウンタにより列挙が1回しか実行されないことも検証する） |
| AC-20 | static | `membership_semantics_test.go` | §3.1 の `AC-20` のコマンドが `0` を返す |
| AC-20 | test | `nsswitch_test.go` | `::TestClassifyNSSCompleteness` が `02_architecture.md` §3.8 の対応表の10通りの環境をすべて含む |
| AC-21 | static | 各 Phase のコミット | §3.1 の `AC-21` のコマンドが `3` 以上を返す（Phase 2・3・4 のコミット単位で数える） |
| AC-21 | manual | 同上 | `02_architecture.md` §7.3 の10行に加え、本書が追加する5件（AC-06・AC-09・AC-12・AC-13・AC-18）の無効化確認を実施する（Phase 2・4 の完了判定条件に記載） |
| AC-22 | static | — | `make test` の終了コードが 0（`CGO_ENABLED=1` と `CGO_ENABLED=0` の両方を実行する）。`make lint` の終了コードが 0（同じく両構成） |
| AC-23 | static | `security-risk-assessment.ja.md`・`CHANGELOG.ja.md` | §3.1 の `AC-23` の3つのコマンド。前2つ（現状はいずれも1件）が一致無しになり、3つ目の陽性の裏取り（現状0件）が1件以上になる。**対象文書の現在の表記は「緩く（許可寄りに）評価される場合がある」であり、括弧が入るため `緩く評価` では一致しない。** 部分文字列を短く取ると作業前から一致無しになり検査が働かない |
| AC-23 | manual | 同上 | 書き換えた「既知の制限」節を `nsswitch.go` の分類規則と突き合わせ、`files`・`systemd` のみの環境では従来どおり判定できるという記述が実装と一致することを確認する |
| AC-24 | static | `record_command.ja.md`・`verify_command.ja.md` | `rg -c 'CGO_ENABLED=1' docs/user/record_command.ja.md docs/user/verify_command.ja.md` が両ファイルとも `1` 以上（現状はいずれも0件であることを確認済み） |
| AC-24 | manual | 同上 | 追加した項目の原因3種（NSS 環境・不正行・macOS）と回復手段が、`02_architecture.md` §4.3 の表および実際のエラー文面と一致することを確認する。3種は回復手段が異なるため、1項目にまとめずに分けて書けていることを併せて確認する |
| AC-25 | static | 英語版4ファイル | `rg -c 'CGO_ENABLED=1' docs/user/record_command.md docs/user/verify_command.md` が両ファイルとも `1` 以上、かつ `rg -c 'nsswitch' CHANGELOG.md` が `1` 以上（いずれも現状0件） |
| AC-25 | manual | 同上 | `/mktrans` の出力を日本語版と突き合わせ、「既知の制限」節と CHANGELOG 項目の内容が対応していることを確認する |
| AC-26 | static | `98_remaining_issues.md` | §3.1 の `AC-26` の2つのコマンドが、それぞれ `1` と `0` を返す。**節を限定せずに `L-2`／`L-3` を検索すると A7 節の96行にある無関係な `- **L-2**:` に一致し、それを消すと AC-28 に違反するため、必ず D1 節に限定する** |
| AC-27 | static | `findings/D1_groupmembership.md` | `rg -c '\*\*対応状況\*\*' docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md` が `2` 以上（現状0件）。かつ `rg -c 'systemd' docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md` が `1` 以上 |
| AC-27 | static | 同上 | 所見の原文が保たれていること: §3.1 の `AC-27` のコマンドが `0` を返す（差分に削除行が無い） |
| AC-28 | static | `98_remaining_issues.md` | `git diff main...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` を目視し、E1・D2・A1・B1・B2・C1・C2・C3・A3・A7 の各節に差分行が現れないことを確認する（差分は D1 の節と分離した2件の追記のみ） |
| AC-29 | static | `01_requirements.md`・`98_remaining_issues.md` | Issue 登録後、確定した2つの番号について §3.1 の `AC-29` のコマンドが両ファイルとも `2` を返す（既存の #976 リンクと取り違えないよう、番号を直に指定する） |
| AC-30 | static | `CHANGELOG.ja.md` | `rg -c 'nsswitch' CHANGELOG.ja.md` が `1` 以上（現状0件）。かつ追加した見出し文字列そのものが「破壊的変更」節の中に存在すること |
| AC-30 | manual | 同上 | 追加した項目の「アップグレード前に影響有無を判定する手順」を `files` のみの本コンテナで実際に実行し、影響なしと判定されることを確認する |
| AC-31 | static | `CHANGELOG.md` | `rg -c 'nsswitch' CHANGELOG.md` が `1` 以上（現状0件） |
| AC-32 | static | `CHANGELOG.ja.md` | §3.1 の `AC-23` の2つ目のコマンド（`rg -c '緩く評価' CHANGELOG.ja.md`）が一致無しになる（現状1件）。かつ §3.1 の `AC-32` のコマンドで、矛盾する見出し「既知の制限: 公式バイナリ」が「未リリース」ブロックから消えたことを確認する |
| AC-33 | static | `CHANGELOG.md` | §3.1 の `AC-33` のコマンドが一致無しになる（現状は英語版の見出し "Known limitation: official binaries" が1件）|

### 3.1 検証コマンド集

パイプ（`|`）を含むコマンドは、markdown の表のセルに直接書くと表の桁区切りと衝突する。エスケープして `\|` と書けば表は崩れないが、そのまま貼り付けると正規表現では選択ではなくリテラルのパイプに、シェルではパイプラインではなくただの引数になり、**いずれも検査が意図どおりに働かない**。そのためパイプを含むものは以下にまとめる。上の表からは AC 番号で参照する。

なお `rg -c` は一致が0件のとき何も出力せず終了コード 1 を返す（`0` とは表示しない）。以下で「一致無しで合格」と書いた検査は、この「無出力・終了コード 1」を指す。

```bash
# AC-23: 本タスクが解消する「緩い評価」の記述が両文書から消えたこと。いずれも一致無しで合格。
#        対象文書の実際の表記は「緩く（許可寄りに）評価される場合がある」であり、
#        `緩く評価` と短く取ると括弧のせいで作業前から一致せず、検査が働かない。
rg -c '緩く（許可寄りに）評価' docs/user/security-risk-assessment.ja.md   # 現状 1 → 0 件へ
rg -c '緩く評価' CHANGELOG.ja.md                                          # 現状 1 → 0 件へ

# AC-23: 書き換え後に「拒否」が既知の制限の節へ入ったこと（陽性の裏取り）。1 以上で合格。
rg -A 12 '既知の制限' docs/user/security-risk-assessment.ja.md | rg -c '拒否'

# AC-32: 矛盾する既存項目が「未リリース」ブロックから消えたこと。一致無しで合格。
#        範囲を「未リリース」から次のリリース見出し（## [1.1.1] など）までに限る。
#        限定しないと、将来この見出しが過去のリリース節へ引用された場合に誤検出する。
awk '/^## \[未リリース\]/{f=1;next} /^## \[/{f=0} f' CHANGELOG.ja.md \
  | rg -c '^#### 既知の制限: 公式バイナリ'

# AC-33: 同じ確認を英語版に対して行う。一致無しで合格。
awk '/^## \[Unreleased\]/{f=1;next} /^## \[/{f=0} f' CHANGELOG.md \
  | rg -c '^#### Known limitation: official binaries'

# AC-20: テスト専用の複製実装が残っていないこと。0 を返せば合格。
#        `\|` と書くとリテラルのパイプを探すことになり、作業前でも 0 を返して検査が無効化される。
rg -c 'shouldSkipSemanticsTest|func nssSources' \
  internal/groupmembership/membership_semantics_test.go

# AC-21: 無効化確認を記録したコミットの「件数」。3 以上で合格。
#        `rg -c` をストリームに掛けると一致した「行数」になり、1コミットでも条件を満たしてしまう。
#        --no-show-signature は必須。本リポジトリは log.showSignature=true のため、
#        付けないと署名検証行が %H の出力に混ざり、git show が「invalid object name」で失敗する。
#        範囲は二点の main..HEAD にする。git log の三点 main...HEAD は対称差であり
#        main 側のコミットも含むため、main が進むと 'disabled' を含む無関係な既存コミット
#        （例: "build: stop disabling modernize's newexpr analyzer"）を数えて誤って合格する。
#        文言も 'disabled the' に固定し、無関係な件名に一致しないようにする。
git log --no-show-signature --format='%H' main..HEAD | while read -r h; do
  git show -s --no-show-signature --format=%B "$h" | rg -qi 'disabled the' && echo "$h"
done | wc -l

# AC-26: 引用ブロックが1件あること。1 を返せば合格。
rg -c 'D1 L-2/L-3 について' \
  docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md

# AC-26: D1 節に限定して L-2・L-3 の箇条書きが消えたこと。0 を返せば合格。
#        節を限定しないと A7 節の無関係な L-2 に一致する（消すと AC-28 違反）。
awk '/^### D1（groupmembership）/{f=1;next} /^### /{f=0} f' \
  docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md \
  | rg -c '^- \*\*L-[23]\*\*'

# AC-27: 所見の原文が書き換えられていないこと（削除行が無い）。0 を返せば合格。
#        `^---` を除くのは diff のファイルヘッダを数えないため。
git diff main...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/findings/D1_groupmembership.md \
  | rg '^-' | rg -v '^---' | rg -c '\S'

# AC-29: 分離した2件の Issue 番号が両文書から参照できること。両ファイルとも 2 で合格。
#        N1・N2 は登録後に確定した実際の番号へ置き換える。
rg -c "issues/(N1|N2)" \
  docs/tasks/0168_groupmembership_nocgo_enumeration_completeness/01_requirements.md \
  docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md
```

## 4. 実装順序とマイルストーン

| マイルストーン | 対応 Phase | 成果物 | 完了の定義 |
|---|---|---|---|
| M1: 完全性が型として存在する | Phase 1 | `completeness.go`、新しい戻り値・キャッシュ・差し替え点 | 外部から観測できる挙動が変わらないまま、2構成で `make test` と `make lint` が通る |
| M2: 不完全が拒否になる | Phase 2 | `isUserOnlyGroupMember` の `switch`、sentinel 2つ、エラーメッセージ | 差し替え点へ不完全・未申告を注入したテストが拒否を確認する |
| M3: 拒否が診断できる | Phase 3 | `%w` への修正、`rejectionRule` の2分岐、`dir_permissions_unix_test.go` | sentinel が `internal/security` と `internal/safefileio` の両境界を越えて辿れる |
| M4: 実環境の条件で拒否に至る | Phase 4 | `nsswitch.go`、走査の分離と `malformedLines`、複製実装の削除、分類警告 | 陽性対照つきの強制実行を含め、2構成で `make test` と `make lint` が通る |
| M5: 変更が文書と監査記録に反映される | Phase 5 | 利用者向け文書、`CHANGELOG`（新項目と既存の矛盾項目の解消）、0149 の残件一覧と findings、分離した2件の Issue | §3 の AC-23〜AC-31 の確認がすべて期待どおりになる |

Phase 1〜4 はこの順に依存する。Phase 5 は Phase 4 の完了後に着手する（文面が実装のエラーメッセージと一致している必要があるため）。

## 5. テスト戦略

### 5.1 単体テストの方針

- **実行環境の NSS 構成に依存させない。** 不完全・未申告の経路へは、列挙の差し替え点 `newWithEnumerator` に任意の完全性を持つ値を返させて到達する。列挙全体の合成へは、`enumerateFromFiles` に分類結果を引数で直接渡して到達する。実環境に依存するのは意味論一致テストのみであり、それは従来どおり skip 条件を持つ。
- **読み取りの分岐だけは実ファイルで検証する。** `readNsswitchSnapshotFrom` は `t.TempDir()` 配下のファイルを対象とし、不在・`chmod 0000` による読み取り失敗・正常読み取りの3件を区別する。これは分類のテーブルテストでは到達できない分岐であり、かつ取り違えると「完全」の誤申告に直結する（`02_architecture.md` §3.2）。
- **走査のテストは production の関数を呼ぶ。** `scanGroupFile`・`scanPasswdFile` が `io.Reader` を受け取る形になるため、テストは `strings.NewReader` で内容を直接与えられる。読み取り中のエラーは `iotest.ErrReader` で与える。既存の走査ループの複製（`membership_nocgo_test.go` の `testFindGroupByGID`／`testFindUsersWithPrimaryGID`）は削除する。
- **境界値と誤り経路を含める。** 分類では `nsswitchState` の4値すべて（ゼロ値の「未読」を含む）、ソース名が1つも残らない行、`passwd`・`group` のいずれか一方だけが欠ける行を対象とする。走査では不正行が対象エントリより前・後の双方、空行・コメント行、NIS 互換エントリを対象とする。判定では完全・不完全・未申告に加えて、`switch` の `default` に到達する未定義の値を対象とする。
- **ログの検証は既存の仕組みを使う。** `tu.NewLogRecorder`（`internal/testutil/handlers.go:41`）を使い、`manager_test.go:1005`・`:1031`・`:1228` を雛形とする。分類警告は `nssCompletenessReporter` がロガーを引数で受けるため、自前のロガーを渡して `t.Parallel` を使ってよい。一方、不正行の警告（`membership_files.go` のパッケージレベルの `slog.Warn`）を検証する場合は `slog.SetDefault` ＋ `t.Cleanup` で復元し、`t.Parallel` を呼ばない。
- **重複を作らない。** `enumerationCompleteness` の各値が期待どおりの拒否になることは `manager_test.go` の判定テストが担い、`completeness_test.go` は合成・構築・表示に限る。`nssSources` の角括弧・コメントの扱いは `TestNSSSources` が担い、`TestClassifyNSSCompleteness` は分類規則の各行に限る。

### 5.2 ビルド構成の網羅

`make test` と `make lint` は `CGO_ENABLED=1` と `CGO_ENABLED=0` の双方で実行される（`Makefile` で確認済み）。新規ファイルのビルドタグは次のとおりとする。

| ファイル | ビルドタグ | 実行される構成 | 根拠 |
|---|---|---|---|
| `completeness.go`・`completeness_test.go` | なし | 両方 | 両ビルドが同じ型を使う |
| `nsswitch.go`・`nsswitch_test.go` | `//go:build !cgo \|\| test` | 両方（`make test`・`make lint` は常に `test` タグを付けるため） | `02_architecture.md` §2.2。同じタグの `membership_files.go` に倣う |
| `internal/security/dir_permissions_unix_test.go` | `//go:build !windows && test` | 両方 | 同パッケージの `_test.go` は全て `//go:build test`、対象の `dir_permissions_unix.go` は `//go:build !windows`。同じ組み合わせの先例は `internal/runner/base/privilege/unix_privilege_test.go` |

`membership_nocgo_test.go`（`//go:build !cgo`）に追加する走査のテストは `CGO_ENABLED=0` の実行でのみ走る。走査の挙動は cgo の有無で変わらないため、この範囲で十分である。この配置は `02_architecture.md` §3.10 の割り当てに従う。

### 5.3 分類結果を強制した実行と陽性対照

`02_architecture.md` §7.5 が求める「非 CGO ビルドかつ分類が『不完全』となる状態でのテスト実行」を Phase 4 の完了判定条件として実施する。本コンテナは `/etc/nsswitch.conf` が `files` のみであり実行ユーザーが非 root であるため、設定ファイルの書き換えでは強制できない。`classifyNSSCompleteness` の戻り値を一時的にソース上で固定する方法を採る。これは AC-21 の無効化確認と同じ手法であり、追加の仕組みを要さない。

**この実行には陽性対照を付ける。** 「何も壊れなかった」を合格とすると、非 CGO 版 `getGroupMembers` の配線が欠けている場合（Phase 1 の暫定の `completeVerdict()` が残っている場合）でも合格してしまい、ゲート自体が「主張する理由で失敗できない」ものになる。分類を「不完全」に固定したとき `manager_test.go:390` の `group_writable_member` が拒否経路へ入ることを確認し、入らなければ不合格とする。

### 5.4 後方互換性の確認

- 公開 API `GetGroupMembers` のシグネチャが変わらないことを静的に確認する（§3 の AC-04a）。
- `internal/runner/base/security` の **production コード**は変更しない。同パッケージのテストのうち実環境の列挙に依存するものは、Phase 4 の強制実行で破綻が判明した場合に `ErrGroupMemberEnumerationIncomplete` を許容する形へ更新する。これは本タスクの挙動変更の当然の帰結であり、AC-04a が保証する「無変更」の対象は production コードとする。
- 完全性が「完全」である場合の `CanUserSafelyWriteFile`・`IsUserInGroup`・`CanCurrentUserSafelyReadFile` の判定結果が変わらないことを、既存テストの期待値を維持したまま確認する（AC-16・AC-17）。

## 6. リスク管理

### 6.1 技術的リスク

| リスク | 影響 | 対策 |
|---|---|---|
| `nsswitchVerdict()` は CGO 構成では呼び出し元を持たないため、`!cgo \|\| test` の `nsswitch.go` に置くと `unused`（`.golangci.yml:13` で有効）に報告されうる | `make lint` の CGO 構成が失敗する | `02_architecture.md` §2.2 の改訂により、`nsswitchVerdict()` と警告レポータの共有インスタンスを `membership_nocgo.go`（`!cgo`）へ置くことで、リスクの原因そのものを取り除いた。`nsswitch.go` に残るのは状態を持たない関数と型だけになる |
| Phase 1 が非 CGO 版に暫定の `completeVerdict()` を置くため、Phase 4 でその差し替えを忘れても、注入側でテストしている全テストが通ってしまう | 本タスクが閉じようとしているフェイルオープンがそのまま残る | §3 の AC-05 に配線の静的確認（`enumerateFromFiles(gid, nsswitchVerdict())` が存在し `completeVerdict()` が残っていないこと）を置き、Phase 4 の強制実行に陽性対照を付ける（§5.3） |
| `readNsswitchSnapshot()` がパスを受け取らないため、`fs.ErrNotExist` と他の失敗の判別をテストできない。この判別を誤ると読み取り失敗が「完全」の申告に化ける | AC-07 の中核が未検証のまま完了扱いになる | 非公開の `readNsswitchSnapshotFrom(path)` へ分離し、`t.TempDir` と `chmod 0000` で3分岐を検証する（Phase 4） |
| 本コンテナの分類が「完全」であるため、`02_architecture.md` §3.7 が挙げた既存テストの破綻が手元で再現しない | CI で初めて破綻が現れ、Phase 4 の PR が差し戻される | Phase 4 の完了判定条件として、陽性対照つきの強制実行を必須にする（§5.3） |
| `membership_nocgo_test.go` の走査ループの複製を残したまま signature だけ変えると、テストは通るが `malformedLines` が一度も検証されない | AC-11〜AC-13 が実質的に未検証のまま完了扱いになる | Phase 4 の作業項目で複製の削除を個別のチェックボックスにし、AC-21 の無効化確認（「不正行の伝達」「対象エントリより後ろの不正行」）で実効性を担保する |
| `completenessVerdict`・`groupEnumeration` は全フィールドが非公開のため、`slog.Any` で渡すと `internal/redaction` の構造体走査が内容ごと `RedactionFailurePlaceholder` に置き換える | 拒否の原因と詳細がログから完全に失われ、`02_architecture.md` §1.1 原則6 に反する | Phase 2 の作業項目に個別のチェックボックスとして明記し、Phase 4 で属性を `slog.Any` にまとめると警告のテストが失敗することを確認する（破れば落ちる形で固定する） |
| 分類がプロセス単位で1回だけ確定する（latch する）ため、`nsswitchVerdict()` 越しに警告を検証しようとすると、先に走ったテストが latch を消費して後続が1件も観測できない。加えて本コンテナの分類は「完全」であり警告自体が出ない | 警告のテストが記録0件のまま素通りし、「主張する理由で失敗できない」テストになる | 警告の生成を `nssCompletenessReporter` へ切り出し、ロガーと `completenessVerdict` を引数で受ける形にする（Phase 4）。テストは自前のインスタンスへ合成した `incompleteVerdict(...)` を渡すため、latch にも実行環境にも依存しない。既存の `sudoUIDAdoptionReporter` と同じ形であり、新しい仕組みを持ち込まない |
| `internal/security/dir_permissions_unix_test.go` が存在しないため、`02_architecture.md` §3.10 の「変更」という記載どおりに進めると作業が漏れる | AC-15 の呼び出し元境界の検証が実装されない | §1.3 と Phase 3 の作業項目で「新規作成」とビルドタグを明記した |
| `runner` の利用者向け文書には拒否への対処が追加されない。`EnsurePermissionCheckUID`（起動時の警告）を呼ぶのは `record`・`verify` のみであり、`runner` では `internal/security` のディレクトリ権限検査が実行前検証の段階で失敗して実行全体が中断する（`02_architecture.md` §5.5） | `runner` の運用者が最も厳しい失敗の仕方をするのに、警告もトラブルシューティング項目も得られない | AC-24 が対象を `record`・`verify` に限っているため本タスクでは範囲外とする。`CHANGELOG` の「破壊的変更」項目には `runner` への影響を明記し、`runner` 文書への追加は残件として記録する |

### 6.2 スケジュール上のリスク

| リスク | 対策 |
|---|---|
| `nsswitchVerdict()` の配置変更に伴い、`02_architecture.md` §2.2・§3.2・§3.10・§4.4 が更新済みである。実装がこの分担から外れると、設計文書と実装が食い違ったまま進む | Phase 4 の「ファイルの分担」の表を実装前に確認し、`make lint` を2構成で通す |
| Phase 4 で package 外の4ファイルに想定外の破綻が見つかり、作業量が増える | 強制実行を Phase 4 の早い段階で行い、破綻の有無を把握する。破綻した場合はその更新も Phase 4 の作業に含める |
| Phase 5 の Issue 登録が外部依存（GitHub）であり、番号確定まで文書を確定できない | Issue 登録を Phase 5 の最初の作業とし、番号が出てから `01_requirements.md` と `98_remaining_issues.md` へ書き戻す |

## 7. 実装チェックリスト

- [ ] **Phase 1**: 完全性の型と申告（AC-01, AC-02, AC-04, AC-04a）
- [ ] **Phase 2**: 判定側のフェイルクローズド化（AC-03, AC-03a, AC-14〜AC-19）
- [ ] **Phase 3**: 診断可能性（AC-15, AC-18）
- [ ] **Phase 4**: 環境の分類と不正行（AC-05〜AC-13, AC-20）
- [ ] **Phase 5**: 文書と監査記録（AC-23〜AC-33）
- [ ] **全体**: §3 の受け入れ基準検証表の全行が期待どおりの結果になる
- [ ] **全体**: AC-21 の無効化確認を `02_architecture.md` §7.3 の10行と本書が追加する5件について実施し、コミットメッセージに英語で記した
- [ ] **全体**: AC-22 の `make test`・`make lint` が2構成で通過した

## 8. 横断検索チェックリスト

`make lint` と `make test` が検出できない項目に限る。§3 の受け入れ基準検証表に既にあるコマンドは重複させない。

- [ ] `rg -c 'testFindGroupByGID|testFindUsersWithPrimaryGID' internal/groupmembership/membership_nocgo_test.go` が `0`（走査ループの複製が残っていない）
- [ ] `rg -n 'shouldSkipSemanticsTest' -g '!docs/**'` が0件一致（削除した関数への参照がコードとコメントに残っていない）
- [ ] `rg -n 'groupMemberCache\{' --type go` の一致がすべて新しいフィールド構成を使っている
- [ ] `rg -n '既知の制限' docs/user/security-risk-assessment.md` の英語版の見出しが日本語版の書き換えに追随している（`/mktrans` の反映漏れの検出）
- [ ] `rg -n '列挙の完全性|完全性判定|未申告' docs/tasks/0168_groupmembership_nocgo_enumeration_completeness/` の用語が `02_architecture.md`「用語」節の定義と一致している

## 9. 成功基準

- **機能の完成度**: `01_requirements.md` の AC-01〜AC-33 がすべて実装され、§3 の検証表の全行が期待どおりの結果になる。
- **品質**: `make test` と `make lint` が `CGO_ENABLED=1`・`CGO_ENABLED=0` の双方で通過する（AC-22）。新規追加した各テストが、`02_architecture.md` §7.3 と本書 Phase 2・4 が挙げる方法で無効化すると失敗する（AC-21）。
- **セキュリティ**: 不完全な列挙が書き込み許可へ到達する経路が残らない。ゼロ値・`switch` の `default`・`combine` の3点がいずれも拒否側に倒れることをテストで固定し、非 CGO 版の配線が実際に分類結果を使っていることを静的確認と陽性対照で担保する。
- **後方互換性**: 完全性が「完全」である環境において、`CanUserSafelyWriteFile`・`IsUserInGroup`・`CanCurrentUserSafelyReadFile` の外部から観測できる挙動が本タスクの前後で変わらない。`internal/runner/base/security` の production コードは無変更のまま動作する。
- **文書**: 利用者向け文書3点と `CHANGELOG` の日本語版・英語版が更新され、同一リリース内で矛盾する記述が残らない。0149 の残件一覧と findings が更新され、分離した2件が Issue として登録されている。
- **追跡可能性**: #976 の L-2・L-3 それぞれについて、解消したのか所見の推奨とは異なる形で close したのかが、コードと監査文書の双方から追える。

## 10. 次のステップ

- [ ] 本計画書のレビューを受け、status を `approved` へ更新する
- [ ] `approved` 後に Phase 1 から実装へ着手する
- [ ] 実装中は各作業項目のチェックボックスをその都度更新する
- [ ] Phase 5 完了後、`98_remaining_issues.md` に残る D1 L-1・L-4 と、`runner` 文書への拒否対処の追加を、別タスクとして検討する
