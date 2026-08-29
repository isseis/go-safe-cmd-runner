# 実装計画書: CGO ビルドの列挙完全性の判定と、SSSD 環境での fail-open の解消

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-08-28 |
| Review date | 2026-08-29 |
| Reviewer | isseis |
| Comments | - |

## 1. 実装概要

### 1.1 目的

[`02_architecture.md`](02_architecture.md)（status: `approved`）が確定した設計を、[`01_requirements.md`](01_requirements.md) の AC-01〜AC-31 を満たす実装へ落とす手順を定める。作業の実体は次の4つである。

1. `nsswitch.go` からビルドタグを外し、`/etc/nsswitch.conf` の分類と、完全性判定をプロセス単位で確定させる仕組みを、両ビルドが共有する1つの実装にする。
2. CGO 版 `getGroupMembers` が、成功して返るすべての経路にその完全性判定を載せる。
3. 拒否メッセージの「事実」と「回復手段」をビルドごとに分け、CGO ビルドの利用者に `CGO_ENABLED=1` でのビルドを案内しないようにする。
4. `cmd/runner` の起動処理から完全性判定を確定させ、3つのバイナリすべてで警告が拒否に先行するようにする。

設計上の根拠・図・受容した残存リスクは `02_architecture.md` にある。本書はそれを繰り返さず、節番号で参照する。

### 1.2 実装原則

- **既存の仕組みを移設し、新しい機構を足さない。** 分類器・完全性判定の確定・警告レポータ・拒否メッセージの組み立ては 0168 が実装済みである。本タスクで新規に書く production コードは、`incompleteness_advice*.go` の3ファイル（内訳は Phase 3 に示す）と公開の入口 `PrecomputeEnumerationEnvironment` だけである（テスト補助関数とテストは §5 に示す）。
- **移設と挙動変更を同じコミットに混ぜない。** Phase 1 は移設のみで外から観測できる挙動を変えず、Phase 2 で初めて CGO ビルドの挙動が変わる。
- **文面の選択は `cause` に対する `switch` で行い、`detail` の文字列を検査しない**（CLAUDE.md「Declare, don't infer」、AC-14）。
- **Go のコメント・識別子・文字列リテラルはすべて英語で書く。** 本書の日本語は計画の記述であり、実装へそのまま入れるものではない。
- **各テストが主張する理由で失敗できることを実装時に確認する**（AC-21、§5.3）。確認を記したコミットメッセージには英語で `disabled the ...` の語を必ず含める。§3.1 の `AC-21` がこの語で件数を数えるためである。

### 1.3 既存コード調査結果

実装前に `internal/groupmembership`・`cmd/runner`・利用者向け文書を調査した。設計が前提としている状態と食い違う点、および設計が触れていない作業を以下に記す。

#### (a) 設計の前提が成り立つことの確認

| 対象 | 現状 | 本タスクでの扱い |
|---|---|---|
| `internal/groupmembership/nsswitch.go` | `//go:build !cgo \|\| test`。分類器一式と `nssCompletenessReporter` を持つ。参照する外部シンボルは `userDatabaseSource`（両ビルドに定義あり）と `completeness.go` の型のみ | タグを外しても未定義シンボルは生じない。Phase 1 で外す |
| `internal/groupmembership/membership_nocgo.go` | `precomputeEnumerationEnvironment`・`processNSSCompletenessReporter`・`nsswitchVerdictMu`／`nsswitchVerdictResolved`／`nsswitchVerdictValue`・`nsswitchVerdict`・`settleNsswitchVerdict` を持つ | Phase 1 で `nsswitch.go` へ移設する。`getGroupMembers`・`enumerateFromFiles`・`enumerateFromSources` は残す |
| `internal/groupmembership/membership_cgo.go` | `getGroupMembers` が2箇所（グループ不在の分岐と最終の return）で `completeVerdict()` を載せる。`precomputeEnumerationEnvironment` は空実装 | Phase 2 で両方を `nsswitchVerdict()` に置き換え、空実装を削除する |
| `internal/groupmembership/membership_files.go`／`membership_files_nocgo.go` | それぞれ `//go:build !cgo \|\| test`／`//go:build !cgo` | 変更しない。CGO ビルドの production 構成に非 CGO 専用シンボルは入らない（AC-10） |
| `internal/groupmembership/manager.go` | `incompleteEnumerationError` が `cause` の `switch` で `fact`・`remediation` を決め、`CGO_ENABLED=1` を3箇所で挙げる | Phase 3 で `switch` と `var fact, remediation string` の宣言をこの関数から取り除き、文面の決定を `adviseIncompleteness` へ委譲する。**組み立て（`state` の生成と `fmt.Errorf` の書式）は変えない** |
| `internal/groupmembership/test_helpers.go` | `//go:build test`。`newWithEnumerator`・`newWithFixedEnumeration` を持つ | Phase 1 で `resetNsswitchClassification` を追加し、`useNsswitchVerdict` は最初の呼び出し元が現れる Phase 2 で追加する（`unused` 対策。Phase 1 参照） |
| `internal/groupmembership/manager_test.go` | ビルドタグ無し。ただし `newWithFixedEnumeration`（`//go:build test`）を使うため、実際には `-tags test` でのみコンパイルされる | 移設先として使える。`make test` は常に `-tags test` を付ける（`Makefile` の `unit-test`） |

#### (b) 設計が挙げていない、必ず対応が要る既存テスト

**`manager_test.go` の `TestIncompleteEnumerationErrorMessage` は、Phase 3 の後 CGO ビルドで失敗する。** 同テストはビルドタグを持たないため現在も CGO ビルドで実行されており、`causeUnsupportedPlatform`・`causeNSSSources`・`causeMalformedLine` の3ケースで `wantContains` に `"CGO_ENABLED=1"` を含んでいる。Phase 3 で CGO 版の文面から `CGO_ENABLED=1` が消えると、この3ケースが CGO ビルドで落ちる。`02_architecture.md` §7.3 の「更新が必要な既存テスト」はこのテストを挙げていないため、本書で対応を定める（Phase 3、§2.3）。方針は、ビルドに依存する期待値を持つ3ケースをビルド別のテストファイルへ分け、ビルドに依存しない2ケース（`causeUnspecified`・想定外の値）を `manager_test.go` に残すことである。

**`membership_nocgo_test.go` の `TestNsswitchVerdictSettlesOncePerProcess`・`TestNsswitchVerdictAgreesAcrossGoroutines`・`TestNsswitchVerdictReportsWhatItSettled` も移設する。** `02_architecture.md` §7.3 が明示するのは `resetNsswitchClassification` と `TestEnsurePermissionCheckUIDPrecomputesEnvironment` の2件だが、この3件も `nsswitchVerdict`／`settleNsswitchVerdict`／`processNSSCompletenessReporter` だけを対象としており、Phase 1 の移設後は非 CGO 固有ではなくなる。`membership_nocgo_test.go` に残すと、両ビルドが共有するようになった仕組みが CGO ビルドでは一度も検証されない。§7.3 と同じ理由（両ビルドで実行されるようにする）が当てはまるため、あわせて `manager_test.go` へ移す。

#### (c) 設計が期待するテストのうち、既に存在するもの（新規作成しない）

以下はいずれも `manager_test.go`（タグ無し ＝ 両ビルドで実行）にあり、Phase 2 で CGO ビルドの挙動が変わっても既に CGO ビルドで通っているため、本タスクで書き換える必要が無い。ホストの `/etc/nsswitch.conf` に依存しない理由はテストによって2通りある——`newWithFixedEnumeration` で完全性判定を注入するもの（下表の「非依存の理由」が「注入」の行）と、そもそも列挙に到達しないもの（同「非到達」の行）である。両者を混同すると、後者を「注入しているから安全」と誤解したまま、列挙に到達する権限ビットを足してしまう。

| AC | 既存テスト | 非依存の理由 | 検証内容 |
|---|---|---|---|
| AC-05 | `TestCanUserSafelyWriteFile_IncompleteEnumeration` | 注入 | 本人1名のメンバー集合でも「不完全」なら `ErrGroupMemberEnumerationIncomplete` |
| AC-05 | `TestIsUserOnlyGroupMember_Completeness` | 注入 | 完全性の各値に対する `switch` の分岐（`default` を含む） |
| AC-06 | `TestCompletenessSurvivesCache` | 注入 | 同じ GID の2回目（キャッシュヒット）でも同じ拒否。列挙が1回しか呼ばれないことも検証する |
| AC-17 | `TestReadPathIgnoresCompleteness` | 注入 | 「不完全」を注入しても `IsUserInGroup`・`CanCurrentUserSafelyReadFile` の結果が変わらない |
| AC-19 | `TestCanUserSafelyWriteFile_CompleteEnumeration` | 注入 | 「完全」を注入したときの、唯一のメンバーの許可と共有グループの拒否 |
| AC-19 | `TestCanUserSafelyWriteFile` | 非到達 | world-writable の一律拒否・非所有者の拒否・owner-writable の許可。`New()`（実物の列挙器）を使うが、3つのサブテストの権限ビット（`0o644` 等）はいずれも group-writable ではないため `isUserOnlyGroupMember` に到達せず、完全性の値に影響されない |
| AC-16 | `nsswitch_test.go::TestNSSCompletenessReporter_Report` | 注入 | 警告の本文と `user_database_source`・`cause`・`detail` の3属性。`userDatabaseSource` を参照するため、CGO ビルドでは自動的に `nss` を期待する |

したがって Phase 2 で新規に書くテストは、CGO 版 `getGroupMembers` が完全性判定を載せることの直接の検証（AC-01・AC-02）に絞れる。

#### (d) `cmd/runner` の調査結果

- 挿入位置は `cmd/runner/main.go` の `run` 関数内、`bootstrap.SetupLogging(...)` が `nil` を返した直後、`configPath == ""` の検査の前である。`02_architecture.md` §4.4 が定める「ログ設定の後、最初の検証の前」を満たす。
- 順序の検証には、同パッケージに既にある `internal/testutil/identitymutationguard` を再利用できる。`startup_order_guard_test.go` が `Options.Extra`（`ExtraTrackedFunc{ImportPath, FuncName}`）で `flag.Parse` を追跡している形をそのまま使い、`bootstrap.SetupLogging`・`groupmembership.PrecomputeEnumerationEnvironment`・`bootstrap.NewVerificationManager` の3つを追跡して `run` 内の位置を比較する。ソースを読む静的な検査だが、`go test` で実行され呼び出しを外すと失敗するため `test` として扱う。
- `cmd/runner/main.go` の `init` は既に `SetProcessPermissionCheckUIDPolicy` を呼んでいる。ここへは足さない。ログ設定より前であり、警告が出力先に届かない。

#### (e) ビルド・検査コマンドの確認

- `make test`（`unit-test`）は `CGO_ENABLED=1 go test -tags test -race` と `CGO_ENABLED=0 go test -tags test` を順に実行する（macOS では前者のみ）。`make lint` も同じ2構成で `golangci-lint` を回す。したがって AC-23 は `make test`・`make lint` の終了コードで判定できる。
- `make deadcode` は `deadcode ./cmd/record ./cmd/runner ./cmd/verify` を既定タグ（CGO 有効）で実行する。AC-10 の確認に使う。

#### (f) 文書の現状

| ファイル | 現状 | 本タスクでの変更 |
|---|---|---|
| `docs/user/security-risk-assessment.ja.md` §3 | 「なお CGO ビルドにも既知の制限がある……実際より緩く評価される可能性がある」と #1064 への参照（各1件） | AC-24 で書き換え、#1064 への参照を解消済みの記述に改める |
| `docs/user/record_command.ja.md` §5.7・`verify_command.ja.md` の対応節 | 非 CGO ビルド向けの3原因（`nss-sources`・`malformed-line`・`unsupported-platform`）の例と対処表がある。例文はすべて `user_database_source=passwd-file` | AC-25 で CGO ビルド向けの項目を追加する。既存の項目には対象ビルドを明記する |
| 上記3件の `.md`（英語版） | 日本語版に対応する記述がある | AC-26 で `/mktrans` により反映 |
| `CHANGELOG.ja.md`／`CHANGELOG.md` | 「未リリース」→「破壊的変更」に 0168 の項目（非 CGO ビルド）がある。`user_database_source=nss` の記載は無い | AC-27 で別項目を追加する |
| `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` §2 D1 | 「（新規）CGO ビルドの列挙完全性」の箇条書きが1件ある | AC-28 で解消済みの引用ブロックへ置き換え、`internal/runner/base/security` の誤検知を残件として追加する |
| いずれの利用者向け文書 | `netgroup` の記述は0件 | AC-30 で追記する |

---

## 2. 実装ステップ

### Phase 1: 分類の共有化（AC-08, AC-09, AC-10, AC-15, AC-16, AC-07 の一部）

対応する設計: `02_architecture.md` §2.2、§3.2。

> **この Phase は「判定」を変えないが、「起動時の記録」は変える。** 移設と doc コメントの是正しか行わないため、許可/拒否の結論はどのビルドでも本 Phase の前後で変わらない。一方で、`EnsurePermissionCheckUID`（`manager.go`、ビルドタグ無し）は `precomputeEnumerationEnvironment()` を無条件に呼んでおり、CGO ビルドでは現在それが `membership_cgo.go` の空実装に束縛されている。本 Phase で定義が `nsswitch.go` の1つに揃うと、CGO ビルドでもこの呼び出しが分類まで到達する。その結果、**`passwd`・`group` が `files`・`systemd` 以外を含むホスト（および非 linux）では、`record`・`verify` の CGO ビルドが起動時に警告 `This build cannot enumerate every member of a group on this host` を `user_database_source=nss` つきで出すようになる。これは CGO ビルドがこれまで一度も出したことのない記録である。** AC-15・AC-16 が求める挙動そのものであり、本 Phase で実際に有効になるため、その検証（既存の `nsswitch_test.go::TestNSSCompletenessReporter_Report`・`TestNSSCompletenessReporter_ReportsOnlyOnce`。タグ除去により両ビルドで実行される）と警告側の陽性対照（§5.5）も本 Phase に置く。`cmd/runner` への配線と公開の入口だけが Phase 4 に残る。

**変更するファイル**: `internal/groupmembership/nsswitch.go`・`membership_nocgo.go`・`membership_cgo.go`・`completeness.go`・`test_helpers.go`・`nsswitch_test.go`・`membership_nocgo_test.go`・`manager_test.go`

> **`precomputeEnumerationEnvironment` の定義は、移設と同じ Phase で1つに揃える。** `nsswitch.go` のビルドタグを外すと、CGO ビルドでは `nsswitch.go` と `membership_cgo.go` の両方がコンパイルされる。移設だけを行って `membership_cgo.go` の空実装を Phase 2 まで残すと、その間 CGO ビルドは `precomputeEnumerationEnvironment redeclared in this block` でコンパイルできず、本 Phase の完了判定条件（両構成で `make test` が通ること）を満たせない。したがって空実装の削除は本 Phase に含める。これにより AC-07 のうち「誤った根拠を述べた doc コメントを空実装ごと削除する」半分が本 Phase で満たされ、残る半分（`getGroupMembers` の doc コメントの是正）が Phase 2 に残る。

**作業内容**

- [x] `nsswitch.go` の先頭行 `//go:build !cgo || test` を、続く空行ごと削除する。
- [x] `membership_cgo.go` の空実装 `precomputeEnumerationEnvironment`（`func precomputeEnumerationEnvironment() {` とその本体）と、その直上の doc コメント4行（`// precomputeEnumerationEnvironment has nothing to resolve for the cgo build:` から始まる）を削除する。これが AC-07 が是正を求める「libc の NSS lookup は成功時つねに完全」の根拠を述べた記述である。
- [x] `membership_nocgo.go` から次の5つを `nsswitch.go` へ移す。本体は1文字も変えない。
  - [x] `processNSSCompletenessReporter` の変数宣言と doc コメント
  - [x] `nsswitchVerdictMu`・`nsswitchVerdictResolved`・`nsswitchVerdictValue` の `var` ブロックと、その上の「1度だけ確定させる」理由を述べたコメント
  - [x] `nsswitchVerdict`
  - [x] `settleNsswitchVerdict`
  - [x] `precomputeEnumerationEnvironment`（移設後は両ビルドで唯一の定義になる）
- [x] 移設に伴い `membership_nocgo.go` の import から `log/slog`・`runtime`・`sync` を外す（`fmt`・`strings` は `enumerateFromSources` が使い続けるため残す）。`nsswitch.go` の import には `runtime`・`sync` を加える（`log/slog` は `nssCompletenessReporter.report` のために既にある）。`make fmt` の後に `make lint` で未使用 import が残らないことを確かめる。
- [x] `nsswitch.go` の `precomputeEnumerationEnvironment` の doc コメントを、移設先で正しい表現へ置き換える。
  - 変更前: `// precomputeEnumerationEnvironment resolves whatever environment facts this build needs before the first enumeration, so that a build unable to enumerate every member says so at startup rather than at the first group-writable file.`
  - 変更後: `// precomputeEnumerationEnvironment settles the completeness verdict before the first enumeration, so that a build that cannot enumerate every member on this host says so at startup rather than at the first group-writable file.`
- [x] **レビュー指摘により、是正の対象を5箇所から7箇所へ広げた。** 追加の2箇所は、いずれも「非 CGO 版だけが読む」前提で書かれており、共有後は CGO ビルドについて誤りになる。
  - [x] `nsswitch.go` の `completeNSSSources`: 是正後の文面は「設定だけから網羅性を確かめられるソース」と述べるが、許可リストにある `systemd` は CGO ビルドではこれを満たさない。`02_architecture.md` §3.2.1 は `nss-systemd` が `getpwent`・`getgrent` に応答し `systemd-homed` のユーザーが動的に現れるため「受容している既知の穴」と明言している。文面を「確かめられる（confirmed）」から「設定を根拠に網羅的とみなす（taken to be exhaustive）」へ改め、`systemd` が確認済みではなく受容した穴であることを段落で加えた。
  - [x] `nsswitch.go` の `classifyNSSCompleteness` 内 `case nsswitchAbsent` の理由コメント: 「設定ファイルが無ければ他のソースを設定する手段が無い」は非 CGO 版にしか当てはまらない。`02_architecture.md` §3.2.1 はこの点に丸1節を割き、CGO では glibc がコンパイル時の既定へフォールバックするため airtight ではなく、それでも「完全」とするのはリスクの釣り合い（3条件の同時成立が事実上無いこと、および `/etc/nsswitch.conf` を持たない最小コンテナを全て拒否する副作用）に基づく判断であると述べている。コメントをその根拠へ差し替えた。
- [x] `nsswitch.go` の `goos != "linux"` の理由コメントを「このビルドが読む形式」から「どちらのビルドも分類できる形式」へ改めた（レビュー指摘、Minor）。分岐はフェイルクローズドであり挙動は変わらない。
- [x] 共有されることで誤りになる doc コメント5箇所を是正する（`02_architecture.md` §2.2 の表）。是正後の共通の意味は「列挙の網羅性を、設定だけから確かめられるソース」であり、この表現はビルドに依存しない。
  - [x] `nsswitch.go` の `completeNSSSources`
    - 変更前: `// completeNSSSources is the allowlist of source names a build reading the`／`// user and group databases from files alone can enumerate exhaustively. It`／`// is an allowlist rather than a list of dangerous names so that a source`／`// this build has never heard of counts against completeness instead of for`／`// it. "compat" is deliberately absent: it pulls NIS entries in through the`／`// "+" and "-" lines, which this build cannot resolve.`
    - 変更後: `// completeNSSSources is the allowlist of source names whose enumeration can`／`// be confirmed exhaustive from the configuration alone, on either build. It`／`// is an allowlist rather than a list of dangerous names so that a source`／`// neither build has heard of counts against completeness instead of for it.`／`// "compat" is deliberately absent: it pulls NIS entries in through the "+"`／`// and "-" lines, and neither build can confirm that those entries are`／`// enumerated in full.`
  - [x] `nsswitch.go` の `classifyNSSCompleteness`
    - 変更前: `// classifyNSSCompleteness decides whether a build that reads the user and`／`// group databases from files alone can enumerate all members, given the`／`// contents of /etc/nsswitch.conf and the target platform. It touches no`／`// files.`
    - 変更後: `// classifyNSSCompleteness decides whether this host's user database`／`// configuration establishes that all members of a group are enumerated,`／`// given the contents of /etc/nsswitch.conf and the target platform. Both`／`// builds share this rule; what differs is why a source fails it, which each`／`// build states in its own advice. It touches no files.`
  - [x] `nsswitch.go` の `classifyNSSSources`
    - 変更前（末尾の1文）: `// can be read as written and names at least one source, and every source`／`// they name must be one this build can enumerate exhaustively.`
    - 変更後: `// can be read as written and names at least one source, and every source`／`// they name must be one whose enumeration can be confirmed exhaustive from`／`// the configuration alone.`
  - [x] `nsswitch.go` の `readNsswitchSnapshotFrom`
    - 変更前（末尾の1文）: `// yields nsswitchReadFailed, because a file that exists but cannot be read`／`// may well name a source this build cannot consult.`
    - 変更後: `// yields nsswitchReadFailed, because a file that exists but cannot be read`／`// may well name a source whose enumeration cannot be confirmed exhaustive.`
  - [x] `completeness.go` の `causeNSSSources`
    - 変更前: `\t// causeNSSSources means /etc/nsswitch.conf configures a source this`／`\t// build cannot enumerate through files alone, or could not be read.`
    - 変更後: `\t// causeNSSSources means /etc/nsswitch.conf configures a source whose`／`\t// enumeration cannot be confirmed exhaustive, or could not be read.`
- [x] `membership_nocgo_test.go` の `resetNsswitchClassification` を `test_helpers.go` へ移す。本体は変えず、doc コメントの末尾に「このプロセス全体で1つの状態を触るため、AC-15・AC-16 の検証は必ずこの関数から始めること」を英語で加える（`02_architecture.md` §7.1 の独立性の要請）。
- [-] `test_helpers.go` への `useNsswitchVerdict(t *testing.T, v completenessVerdict)` の追加は Phase 2 へ送る。**理由**: 本 Phase には呼び出し元が無く、`.golangci.yml` の `unused` が `func useNsswitchVerdict is unused` で `make lint` を失敗させる（本 Phase の完了判定条件に反する）。最初の呼び出し元は Phase 2 の CGO 版 `getGroupMembers` のテストであり、そこで追加する。仕様は変えない: `resetNsswitchClassification` を呼んだうえで `nsswitchVerdictValue` に `v` を、`nsswitchVerdictResolved` に `true` を設定する。両者が触る状態は `nsswitchVerdictMu`・`nsswitchVerdictResolved`・`nsswitchVerdictValue`・`processNSSCompletenessReporter.reported` の4つだけであり、片方だけを操作する経路は作らない。
- [x] `nsswitch_test.go` の先頭行 `//go:build !cgo || test` を、続く空行ごと削除する。テスト本体は変えない。
- [x] `membership_nocgo_test.go` から `manager_test.go` へ次の4つのテストを移す。本体は変えない。
  - [x] `TestNsswitchVerdictSettlesOncePerProcess`
  - [x] `TestNsswitchVerdictAgreesAcrossGoroutines`
  - [x] `TestNsswitchVerdictReportsWhatItSettled`
  - [x] `TestEnsurePermissionCheckUIDPrecomputesEnvironment`（移設に伴い、doc コメント末尾の「It lives here rather than in manager_test.go because the cgo build has no classification to settle.」を削除する。CGO ビルドも確定させるようになったため事実に反する）
- [-] `manager_test.go` へのテーブルテスト `TestClassifyNSSCompletenessAgreesAcrossBuilds` の追加は**取り止めた**（レビュー指摘）。**理由**: 本 Phase は `nsswitch_test.go` からもビルドタグを外しており、既存の `TestClassifyNSSCompleteness`（22行）が両ビルドで実行される。加えて `make test` は元から `-tags test` で走るため、`!cgo || test` は両構成でつねに成立しており、タグ除去の有無に関わらず同テーブルは両ビルドで実行されていた。新テーブルの8行はこの22行の真部分集合であり、主張（`assert.Equal` 2件）も同一で、既存側はさらに `detail` の非空を主張する。さらに「両ビルドの分類が食い違う」状態は、`classifyNSSCompleteness` と `completeNSSSources` がパッケージに1つしか無い以上どのコード変更でも作れず、この主張は反証不能である（§5.3 の `X1`〜`X9` にも対応行が無く、「テストは主張する理由で失敗できること」を満たせない）。AC-09 は既存テスト＋タグ除去＋AC-08 の静的検査（許可リストが1箇所であること）で満たす。
- [-] `manager_test.go` への `TestProcessNSSCompletenessReporterEmitsOncePerProcess` の追加は**取り止めた**（レビュー指摘）。**理由**: 既存の `nsswitch_test.go::TestNSSCompletenessReporter_Report` が同一の属性マップ（`user_database_source`・`cause`・`detail`）・レベル・本文を主張し、`TestNSSCompletenessReporter_ReportsOnlyOnce` が1回限りを主張する。いずれも `-tags test` により元から両ビルドで実行され、`userDatabaseSource` を期待値に用いるため CGO では `nss`、非 CGO では `passwd-file` を要求している（AC-16 はこれで満たされている）。追加しようとしたテストはこの2件を共有インスタンスに対して連結したものにすぎず、`report` を直接呼ぶため `settleNsswitchVerdict` → `justSettled` → `report` の連結（共有インスタンスについて唯一未検証な部分）を踏まない。`X8` の無効化は既存テストも同時に失敗させるため、追加分の固有価値を示さない。削除後、`go tool cover -func` が両構成で関数ごとに不変であることを確認した。

> **`nsswitchVerdict()` 経由で警告を踏ませることはできない。** `nsswitchVerdict` が `report` を呼ぶのは `settleNsswitchVerdict` が `justSettled == true` を返したときだけであり、`useNsswitchVerdict` は `nsswitchVerdictResolved` を `true` にするため、その後の呼び出しは `justSettled == false` になって記録を1件も出さない。`reported` を `false` に戻しても同じである。「確定が記録を駆動する」という連結そのものは、移設した `TestNsswitchVerdictReportsWhatItSettled`（確定した完全性が「完全」でないことと `reported` が真であることが同値であると主張する）が担う。ただし同テストは分類が「完全」になるホストでは `false == false` を確かめるにとどまるため、「不完全」側の陽性確認は §5.5 の強制実行で行う。production コードにテスト専用の差し替え点を足さない方針（`02_architecture.md` §7.1）を守るための割り切りであり、0168 が同テストの doc コメントで既に宣言している限界と同じものである。


**完了判定条件**

- [x] `CGO_ENABLED=1` と `CGO_ENABLED=0` の双方で `make test` が成功する（`make test` が両方を実行する）。
- [x] `make lint` が両構成で成功する。
- [x] `make deadcode` が**新たな**到達不能コードを報告しない（AC-10）。**計画からの相違**: 本条件は当初「出力が本 Phase の前後で変わらない」と書いていたが、実際の出力は1行**減った**（`internal/groupmembership/completeness.go:84:6: unreachable func: incompleteVerdict` が消えた）。`nsswitch.go` のタグ除去により `precomputeEnumerationEnvironment` → `nsswitchVerdict` → `classifyNSSCompleteness` の経路が CGO ビルドの production 構成でつながり、`incompleteVerdict` が到達可能になったためである。これは本 Phase が有効化しようとしている当のものであり、AC-10 の要求（「新たな到達不能コードを報告しない」）は満たしている。したがって判定は「出力の同一性」ではなく「増分が無いこと」で行う。
- [x] `rg -c '^//go:build' internal/groupmembership/nsswitch.go internal/groupmembership/nsswitch_test.go` が一致無し。
- [x] `rg -l 'func precomputeEnumerationEnvironment' internal/groupmembership/` が `nsswitch.go` の1件のみを返す（現状は `membership_nocgo.go`・`membership_cgo.go` の2件）。
- [x] `rg -c 'always report a complete enumeration' internal/groupmembership/` が一致無し（現状 `membership_cgo.go` に 1 件。AC-07 の前半）。
- [x] §5.3 の無効化確認 `X2`・`X7`・`X8` を実施し、コミットメッセージに英語で記す。
- [x] `rg -c '^//go:build' internal/groupmembership/membership_files.go` が `1`（`!cgo || test` を据え置いたこと）。

> **`02_architecture.md` §2.2 が求める「ビルドごとの理由」の置き場について（レビュー指摘、Minor）。** §2.2 は、`nsswitch.go` から外したビルド固有の理由（非 CGO 版は「自力で読めない」、CGO 版は「libc は読めるが網羅性の保証が無い」）を `membership_nocgo.go`・`membership_cgo.go` の doc コメントへ置くとしている。本 Phase は `nsswitch.go` 側の是正のみを行い、両ファイルへの追記は行っていない。CGO 版の理由は Phase 3 の `incompleteness_advice_cgo.go` の文面として利用者向けに現れるため、そちらで満たす。非 CGO 版の理由は `membership_nocgo.go` の `userDatabaseSource` の既存 doc コメント（「Without cgo, os/user parses /etc/passwd only; directory-backed sources such as LDAP or SSSD are not consulted.」）が既に述べており、追加は要らない。§2.2 の要請はこの2つで満たすものとし、Phase 1 での追記は行わない。

### PR-1 作成ポイント: shared nsswitch classification

**対象ステップ**: Phase 1

**推奨タイトル**: `feat(0169): share the nsswitch classification across both builds`

**レビュー観点**: 移設した5つのシンボルの本体が1文字も変わっていないこと（差分が移動と doc コメントの是正に限られること） / ビルドタグ除去後に `precomputeEnumerationEnvironment` の定義が `nsswitch.go` の1つだけになり、両構成でコンパイルが通ること / **CGO ビルドの `record`・`verify` が該当ホストで起動時警告を出すようになること（許可/拒否の判定は変わらない）を確認したか**——本 PR で新たに有効になる唯一の外部から観測できる変化である / **プロセス全体で1つの latch（`nsswitchVerdictMu` と `reported` の CAS）が CGO ビルドで初めて実行経路に入るため、`-race` つきの `CGO_ENABLED=1` 実行が通ること** / 是正した doc コメント5箇所が、どちらのビルドの説明としても正しい表現になっていること / `membership_files.go`・`membership_files_nocgo.go` のタグが据え置かれ、非 CGO 専用シンボルが CGO ビルドへ入っていないこと（AC-10）

**実装モデル要件**: frontier-recommended

**判定理由**: 「孤立した高リスク・複雑なステップ（並行制御）」の枝に該当する。プロセス全体で1つの latch（`nsswitchVerdictMu` と `processNSSCompletenessReporter.reported` の CAS）が、これまで一度も実行してこなかった CGO ビルドの実行経路に入る。しかもそれは `-race` つきで走る構成である。同時に、この活性化によって CGO ビルドの `record`・`verify` が起動時警告を出すようになる（Phase 1 冒頭の注記）——「移設するだけ」では済まない外部から観測できる変化であり、その意味を取り違えたまま「挙動は変わらない」と判断されると、警告の増加が回帰として扱われかねない。あわせて Conditional check の「ビルドタグ下でのコンパイル確認」にも該当する（`useNsswitchVerdict` を `//go:build test` の `test_helpers.go` に置く）。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1073）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: CGO 版の完全性申告（AC-01〜AC-06, AC-07 の残り）

対応する設計: `02_architecture.md` §3.1、§3.4、§6.1。ここで CGO ビルドの挙動が変わる。

**変更するファイル**: `internal/groupmembership/membership_cgo.go`・`membership_cgo_test.go`・`02_architecture.md`

**作業内容**

- [x] `var pwentMutex sync.Mutex` に付いている doc コメントを分割する。現在は `getGroupMembers` を説明する段落と `pwentMutex` を説明する段落が1つのコメントブロックにまとまっており、`getGroupMembers` 自身には doc コメントが無い。
  - [x] `var pwentMutex` に残すコメント: `// pwentMutex serialises all setpwent/getpwent/endpwent calls within this`／`// package. It is held inside getUsersWithPrimaryGID.`／`// Lock ordering: GroupMembership.cacheMutex -> nsswitchVerdictMu -> pwentMutex.`／`// Reverse acquisition is forbidden.`（ロック順序に `nsswitchVerdictMu` を加える。`02_architecture.md` §2.2）
  - [x] `func getGroupMembers` に新たに付ける doc コメント: `// getGroupMembers returns all members of a group given its GID, together`／`// with what this host's user database configuration says about whether`／`// libc enumerates them exhaustively.`／`//`／`// The member set is the union of explicit members (gr_mem) and users whose`／`// primary GID matches the requested GID. This matches the non-CGO`／`// implementation semantics. The completeness is the verdict settled for`／`// this process: libc resolves every configured source, but a source such as`／`// sss gives no guarantee that what it returns is every member.`
- [x] `getGroupMembers` の本体で、完全性判定の取得を関数の先頭に置く（`verdict := nsswitchVerdict()`）。`02_architecture.md` §6.1 のとおり、成功して返るすべての経路が同じ値を載せることをコードの形から読めるようにするためである。
- [x] `getGroupMembers` のグループ不在の分岐を、`return groupEnumeration{members: []string{}, verdict: completeVerdict()}, nil` から `return groupEnumeration{members: []string{}, verdict: verdict}, nil` へ変える。
- [x] `getGroupMembers` の最終の return を、`return groupEnumeration{members: merged, verdict: completeVerdict()}, nil` から `return groupEnumeration{members: merged, verdict: verdict}, nil` へ変える。
- [x] `membership_cgo_test.go` の `TestGetGroupMembers_StatesComplete` を改める（`02_architecture.md` §7.3）。
  - [x] 関数名を `TestGetGroupMembers_StatesTheHostVerdict` へ改める。「無条件に完全」を固定する名前が残ると、主張と名前が食い違う。
  - [x] 主張を `assert.Equal(t, completeVerdict(), enumeration.verdict)` から、期待値を `classifyNSSCompleteness(readNsswitchSnapshot(), runtime.GOOS)` から得る形へ改める。分類の定義をテスト側に複製しないための形であり、`membership_semantics_test.go` が既に採っている書き方に倣う。
  - [x] doc コメントを、このテストがホストの分類との一致を主張するものであることを述べる文へ書き換える。
  - [x] **このテストを AC-01・AC-02 の根拠として数えない。** `files` のみを構成したホスト（本コンテナと GitHub-hosted の Ubuntu ランナーがこれに当たる）では期待値が `completeVerdict()` に等しくなるため、`getGroupMembers` を元の `completeVerdict()` 固定へ戻しても緑のままであり、そのホストでは失敗しえない。失敗しうるのは分類が「完全」にならないホストだけである。AC-01・AC-02 を担うのは、非ホスト由来の完全性判定を植える `TestGetGroupMembers_CarriesTheSettledVerdict` のほうである。`02_architecture.md` §7.3 が本テストの維持（削除ではなく更新）を指示しているため残すが、位置づけはホスト整合性の確認に限る。
- [x] `test_helpers.go` に `useNsswitchVerdict(t *testing.T, v completenessVerdict)` を追加する（Phase 1 から繰り延べた分。仕様は Phase 1 の該当行に記す）。**計画からの相違**: 本 Phase で最初の呼び出し元が現れるため `unused` に掛からない、と書いていたが、実際には掛かった。呼び出し元は `//go:build cgo && test` のテストだけであり、`make lint` の `CGO_ENABLED=0` 側の実行（`Makefile` の `lint` は2構成で回す）はそのファイルを読まないため `func useNsswitchVerdict is unused` を報告する。置き場は §5.2 の指定どおり `test_helpers.go` のまま変えず、理由を述べた `//nolint:unused` を doc コメントに添えて解消した。`cgo && test` のヘルパファイルへ分けなかったのは、両ビルドのテストが同じ手段で完全性判定を固定するという §7.1 の形を崩さないためである。
- [x] `membership_cgo_test.go` に AC-01・AC-02 を検証するテスト `TestGetGroupMembers_CarriesTheSettledVerdict` を追加する。`useNsswitchVerdict` で完全性判定を固定し、グループが存在する GID（`getCurrentUserGID(t)`）と存在しない GID（`manager_test.go` が既に定義している `unrelatedGID`。リテラルの `99999` を書かない）の双方で、固定した値がそのまま `enumeration.verdict` に現れることを検証する。固定する値は少なくとも次の3つとする。
  - [x] `completeVerdict()`（AC-01）
  - [x] `incompleteVerdict(causeNSSSources, "passwd: sss")`（AC-02。SSSD が構成されたホストを模した値であり、libc がエラーを返していない状況で「不完全」になることを踏む）
  - [x] `incompleteVerdict(causeUnsupportedPlatform, "goos=darwin")`（AC-04 が linux 以外で載る値。分類器側の AC-04 は `nsswitch_test.go::TestClassifyNSSCompleteness` が踏む）
  - [x] このテストはプロセス全体の状態を触るため `t.Parallel()` を宣言しない。
- [-] `membership_cgo_test.go` の先頭のビルドタグを `//go:build cgo` から `//go:build cgo && test` へ改める件は**取り止めた（利用者指摘。2026-08-29）**。いったん改めたが元へ戻し、`incompleteness_advice_{cgo,nocgo}_test.go` にも `&& test` を付けないこととした。**理由**: `_test.go` は production ビルドに入らないため、`test` タグに production からの除外という役目は無い（それが必要なのは `_test.go` でない `test_helpers.go` の側である）。「`test_helpers.go` のシンボルを使うから」という当初の理由も成り立たない——本パッケージのテストは `-tags test` 無しでは**そもそもコンパイルできない**。`manager_test.go`（タグ無し）が全ファイル `//go:build test` の `internal/testutil` を import しており、タグ無しの `go test` は `build constraints exclude all Go files in internal/testutil` で失敗する。`Makefile` の `unit-test-cgo1`・`unit-test-cgo0` と `make lint` はいずれも `-tags test` を渡すため、付けても外しても到達範囲は変わらない（両構成での実行を確認済み）。**むしろ付けないほうが安全である**——将来 `manager_test.go` の `testutil` 依存が無くなった場合、タグが付いていればタグ無し実行でこれらのファイルは**黙って実行対象から外れる**が、付いていなければ未定義シンボルのコンパイルエラーとして表面化する。パッケージ内の多数派（`manager_test.go`・`nsswitch_test.go` ほか、いずれも `test_helpers.go` のシンボルを使う）も `test` タグを付けていない。**`membership_semantics_test.go` の `//go:build cgo && test` は据え置く**——同ファイルは AC-22 が本タスクでの無変更を求めている。同ファイルのタグ整理は別タスクとする。
- [x] `02_architecture.md` §3.2.1 の「/etc/nsswitch.conf が存在しない場合」に、glibc の既定構成テーブル（upstream の `nss/nss_database.c`）を読んだ結果を追記する（同節と §7.5 が求める確認）。結果が同節の記述と食い違った場合にのみ、AC-03 の改訂を提案する。

**完了判定条件**

- [x] `make test`・`make lint` が両構成で成功する。
- [x] `rg -c 'completeVerdict\(\)' internal/groupmembership/membership_cgo.go` が一致無し（現状 2 件）。
- [x] `rg -c 'nsswitchVerdict\(\)' internal/groupmembership/membership_cgo.go` が `1`。
- [x] `rg -c '^func TestGetGroupMembers_CarriesTheSettledVerdict\(' internal/groupmembership/membership_cgo_test.go` が `1`（新しいテストが実在すること。§3 の AC-20 の静的確認が空振りしないための前提）。
- [x] §5.3 の無効化確認 `X1`・`X5` を実施し、コミットメッセージに英語で記す。

> **Phase 2 で `cacheMutex` を保持したままログハンドラを呼ぶ経路が CGO ビルドにも生じる（レビュー指摘）。** `manager.go` の `getGroupEnumeration` は `cacheMutex.Lock()` を保持したまま `gm.enumerateGroupMembers(gid)` を呼ぶ。本 Phase で CGO 版 `getGroupMembers` が `nsswitchVerdict()` を呼ぶようになると、その中の `processNSSCompletenessReporter.report(slog.Default(), verdict)` が `cacheMutex`（再入不可の `sync.RWMutex`）の下で実行される。ハンドラは任意のコードであり、`safefileio` 経由で `CanCurrentUserSafelyWriteFile` → `getGroupEnumeration` へ戻るものがあれば自己デッドロックする。`nsswitchVerdict` の既存コメントは「記録はロックの外で出す」と述べているが、その対象は `nsswitchVerdictMu` だけであり `cacheMutex` を名指ししていない。なお本経路は非 CGO ビルドには既に存在する（`getGroupMembers` が `nsswitchVerdict()` を呼ぶため）ので、本 Phase が新設する欠陥ではなく CGO ビルドへ拡げるものである。通常の起動では `precomputeEnumerationEnvironment()` が先に確定させるため `justSettled` が偽になり `report` は呼ばれないが、その入口を通らない利用（ライブラリとしての利用、テスト）では成立しうる。本 Phase で次の2点を行う。ロック順序の注記の更新（`cacheMutex` → `nsswitchVerdictMu` → `pwentMutex`）とは別の問題であり、順序ではなく「ロック下で任意のコードを呼ぶこと」を扱う。

**対処は Phase 4 の作り替えに委ね、本 Phase では回避策を作らない**（2026-08-29 決定。下記「latch の作り替え」を参照）。遅延確定をやめて起動時の1回に固定すれば `report` は起動時＝ロックの外でしか走らなくなり、この経路自体が消えるためである。回避策を先に入れると、Phase 4 でそれを捨てることになる。

- [x] `nsswitchVerdict` のコメントの「a lock that is not reentrant」を、`nsswitchVerdictMu` と `cacheMutex` の双方を名指しする表現へ改める。Phase 4 までの暫定の記録であり、作り替えの際にコメントごと消える。
- [x] Phase 2〜3 の間はこの経路が生きていることを受容する。**踏むには「`GroupMembership` へ戻る `slog` ハンドラ」が要るが、そのようなハンドラは現在存在しない**（production の goroutine は Slack 送信の2箇所のみで、いずれも群所属を参照しない）。

### Phase 3: 拒否メッセージのビルド別化（AC-11〜AC-14）

対応する設計: `02_architecture.md` §3.3、§4.3。

**変更するファイル**: `internal/groupmembership/manager.go`、新規 `incompleteness_advice.go`・`incompleteness_advice_cgo.go`・`incompleteness_advice_nocgo.go`、`manager_test.go`、新規 `incompleteness_advice_cgo_test.go`・`incompleteness_advice_nocgo_test.go`

**作業内容**

- [x] `incompleteness_advice.go`（ビルドタグ無し）を新規追加する。
  - [x] `incompletenessAdvice` 型（非公開フィールド `fact string`・`remediation string`）
  - [x] `implementationDefectAdvice(what string) incompletenessAdvice`。`fact` に `what` を、`remediation` に `"report this as a defect in the enumeration implementation"` を入れて返す。この文字列は現行 `manager.go` の `causeUnspecified`・`default` の `remediation` と同一であり、変えない。
- [x] `incompleteness_advice_nocgo.go`（`//go:build !cgo`）を新規追加し、`adviseIncompleteness(cause incompletenessCause) incompletenessAdvice` を置く。文面は現行 `manager.go` の `switch` から**1文字も変えずに**移す（AC-13）。
  - [x] `causeUnsupportedPlatform`: fact `"this build cannot enumerate all members of a group on this platform"`、remediation `"rebuild with CGO_ENABLED=1 so that group members are resolved through the platform's own user database via libc"`
  - [x] `causeNSSSources`: fact `"/etc/nsswitch.conf names a user database source this build cannot consult, or could not be read"`、remediation `"check the passwd and group lines of /etc/nsswitch.conf, then rebuild with CGO_ENABLED=1 so that the configured sources are consulted"`
  - [x] `causeMalformedLine`: fact `"a line of the user database files could not be parsed and was skipped, so the members listed there are unknown"`、remediation `"check the reported line: correct it if its format is wrong, or, if it is a NIS compatibility entry (a line starting with + or -), rebuild with CGO_ENABLED=1"`
  - [x] `causeUnspecified`: `implementationDefectAdvice("the enumeration was judged incomplete but recorded no cause")`
  - [x] `default`: `implementationDefectAdvice("the enumeration was judged incomplete for a cause this build does not recognize")`
  - [x] 現行 `manager.go` の `causeUnsupportedPlatform` の枝に付いている説明コメント（`// The platforms this reaches are the ones with no /etc/nsswitch.conf,` から始まる3行）も一緒に移す。
- [x] `incompleteness_advice_cgo.go`（`//go:build cgo`）を新規追加し、同名の `adviseIncompleteness` を置く。回復手段に `CGO_ENABLED` を含めない（AC-12）。
  - [x] `causeUnsupportedPlatform`: fact `"this platform offers no way to determine how its user database is configured, so a group's member list cannot be confirmed to cover every member"`、remediation `"clear the group-writable bit on the path (chmod g-w)"`
  - [x] `causeNSSSources`: fact `"/etc/nsswitch.conf does not establish that every member of a group is enumerated: a source it names gives no guarantee of exhaustive enumeration (SSSD returns no directory users under enumerate = False, and no explicit members under ignore_group_members = True), a line it needs is missing or could not be read as written, or the file could not be read; the detail says which"`、remediation `"clear the group-writable bit on the path (chmod g-w), or configure the passwd and group lines with only sources whose enumeration is exhaustive (files, systemd)"`。**計画からの相違（レビュー指摘により実装時に2点変えた。いずれも `02_architecture.md` §4.3 の表に近づける向きである）**: (1) remediation に `only` を入れた——`のみ` は §4.3 の表にあるが計画の文字列から落ちていた。無いと「`sss` の隣に `files` を足す」という読み方が成り立つが、分類器は名指しされたソースが**すべて**許可リストにあることを求めるため、その対処では拒否が解けず利用者を空回りさせる。(2) fact に「行の不在」と「どれに当たるかは `detail` が示す」を入れた——どちらも §4.3 の表にあり、計画の文字列だけが落としていた
    - `fact` を広く書くのは、この原因が指定ソース起因だけでなく行の重複・角括弧の未閉じ・行の不在・ソース名の不在・ファイル読み取り失敗のいずれでも付くためである（`02_architecture.md` §4.3）。粒度は `detail` が担う。
  - [x] `causeMalformedLine`: `implementationDefectAdvice("a cause only a build that scans the user database files directly can produce was reported")`。この枝を残すのは `switch` の網羅性を保つためであり、到達した場合は環境の問題ではなく実装の誤りとして `default` と同じ拒否側に倒す（AC-14）。
  - [x] `causeUnspecified`・`default`: 非 CGO 版と同じ `implementationDefectAdvice` の呼び出しを置く。
- [x] `manager.go` の `incompleteEnumerationError` を、文面の決定を委譲する形へ改める。`var fact, remediation string` の宣言と `switch verdict.cause { ... }` の全体を削除し、`advice := adviseIncompleteness(verdict.cause)` の1行に置き換える。`fact`・`remediation` の参照は `advice.fact`・`advice.remediation` に読み替える。宣言を残すと `make lint` が未使用として報告する。`state` の組み立てと `fmt.Errorf` の書式（`"cannot confirm the members of group GID %d: %s (%s); %s: %w"`）はそのまま残す。doc コメントの「The cause selects both the fact and the remediation; neither is chosen by inspecting the detail text.」も残し、委譲先を指す1文を加える。
- [x] `manager_test.go` の `TestIncompleteEnumerationErrorMessage` を分割する（§1.3(b)）。
  - [x] `manager_test.go` に残すのは、ビルドに依存しない2ケース（`causeUnspecified`・`causeOutOfRange`）のみとする。この2ケースの `wantContains` に `"CGO_ENABLED=1"` は含まれない。関数名は変えない。
  - [x] `incompleteness_advice_nocgo_test.go`（`//go:build !cgo && test`）を新規追加し、`TestAdviseIncompleteness_NoCGO` を置く。**計画からの相違（利用者指摘。2026-08-29。CGO 版と同じ理由）**: 当初は現行 `TestIncompleteEnumerationErrorMessage` の `wantContains`／`wantNotContains` をそのまま引き継ぐ形で書いたが、これは production の文面のコピーであり、文面を改善しただけでテストが赤くなる。**AC-13 は「移設で文面が変わっていないこと」を求める一回限りの要件であり、その検証は §3.1 の `rg -cF`（静的、`6` を返す）が担う。**同じリテラルを3箇所目としてテストに置いても、AC-13 の検証は強くならず、将来の正当な改善だけを妨げる。テストは性質を検証する形（AC-13 の test 行に列挙）へ改めた。**元のテストが捕らえた欠陥は保たれている**——`wantContains` が捕らえた「回復手段が `CGO_ENABLED=1` を案内しなくなる」は同名の検査が、`wantNotContains` が捕らえた「無関係な `/etc/nsswitch.conf` を案内する」は `namesNsswitch` の真偽両方向の検査が捕らえる。実際に5種の変異（枝の取り違え・cgo 案内の消失・`NIS` 注意の消失・誤ったファイルの名指し・advice の非伝播）で失敗すること、および3つの fact の正当な書き換えでは失敗しないことを確認した。
  - [x] `incompleteness_advice_cgo_test.go`（`//go:build cgo && test`）を新規追加し、`TestAdviseIncompleteness_CGO` を置く。検証内容は次の3点。**計画からの相違（利用者指摘。2026-08-29）**: レビュー指摘を受けて一度は CGO 版の文面を全文照合する形にしたが、これは取り止め、性質の検証へ改めた。**理由**: CGO 版の文面は凍結対象ではない（凍結を求めるのは非 CGO 版に対する AC-13 のみ）。全文照合は本文のコピーであり、文面を改善しただけで——欠陥が無いのに——テストが赤くなる。しかも直し方は期待値へのコピー&ペーストであり、内容を考えずに期待値を書き換える習慣を促す。代わりに「`CGO_ENABLED` を含まない」「ホスト起因の原因は `chmod g-w` を案内する」「実装の誤りを指す原因は `implementationDefectAdvice` の回復手段を返す」「原因ごとの advice が互いに異なる」「関数が返した文字列がメッセージに現れる」を検証する。**レビュー指摘が捕らえたかった欠陥は保たれている**——枝の取り違え（`causeNSSSources` が `causeUnsupportedPlatform` の advice を返す）は advice の相互相違が捕らえ、組み立てが advice を落とす欠陥は「関数の戻り値がメッセージに現れる」が捕らえる。実際に4種の変異（枝の取り違え・advice の非伝播・`CGO_ENABLED` の再混入・実装誤り側の枝がホスト向け案内を返す）で失敗すること、および文面の正当な書き換えでは失敗しないことを確認した。Phase 5 が文書へ転記する文面の一致は §5.4 の生成・突き合わせ手順が担う。
    - [x] `causeUnsupportedPlatform`・`causeNSSSources` の `fact`・`remediation` のいずれにも `"CGO_ENABLED"` が現れないこと、および `remediation` に `"chmod g-w"` が現れること（AC-12）
    - [x] `causeMalformedLine`・`causeUnspecified`・`causeOutOfRange` が実装の誤りを示す文面（`"defect"`）を返すこと（AC-14）
    - [x] `incompleteEnumerationError(unrelatedGID, incompleteVerdict(causeNSSSources, "passwd: sss"))` のメッセージが `"user_database_source=nss"`・`"cause=nss-sources"`・`"detail=passwd: sss"` を含み、`ErrGroupMemberEnumerationIncomplete` で包まれていること（AC-11）
  - [x] `causeOutOfRange` は現在 `manager_test.go` に定義されている。両ビルド別テストから参照するため、定義は `manager_test.go`（タグ無し）にそのまま残す。新規の2ファイルに `&& test` を付けるのは、参照する `unrelatedGID`・`causeOutOfRange` を持つ `manager_test.go` が `newWithFixedEnumeration`（`//go:build test`）に依存し、`-tags test` でなければコンパイルできないためである。Phase 2 で `membership_cgo_test.go` を `//go:build cgo && test` に改めるのと同じ理由であり、パッケージ内で判断を揃える。

**完了判定条件**

- [x] `make test`・`make lint` が両構成で成功する。
- [x] `rg -c 'CGO_ENABLED' internal/groupmembership/manager.go internal/groupmembership/incompleteness_advice.go internal/groupmembership/incompleteness_advice_cgo.go` が一致無し。
- [x] `rg -c 'CGO_ENABLED=1' internal/groupmembership/incompleteness_advice_nocgo.go` が `3`。
- [x] `rg -l 'func adviseIncompleteness' internal/groupmembership/` が `incompleteness_advice_cgo.go` と `incompleteness_advice_nocgo.go` の2件のみを返す。
- [x] §3.1 の `AC-14` のコマンドが一致無し（`detail` の内容で分岐していないこと）。
- [x] `rg --files internal/groupmembership | rg -c 'incompleteness_advice(_cgo|_nocgo)?\.go'` が `3`（3ファイルが実在すること。存在しないファイルに対する `rg` は終了コード 2 を返し、これを「一致無し」と取り違えると上の各静的確認が空振りする）。
- [x] §5.3 の無効化確認 `X6` を実施し、コミットメッセージに英語で記す。

### PR-2 作成ポイント: cgo fail-closed enumeration with build-specific advice

**対象ステップ**: Phase 2 / Phase 3

**推奨タイトル**: `feat(0169): fail closed on cgo builds and advise the right remediation`

**レビュー観点**: 完全性判定が、成功して返るすべての経路（グループ不在の分岐を含む）に載っていること / `completeVerdict()` の固定値が `membership_cgo.go` から消えていること / 非 CGO 版の6つの文面（事実3・回復手段3）が1文字も変わっていないこと（§3.1 の `AC-13` が `6` を返すこと） / CGO 版の文面に `CGO_ENABLED` が現れず、回復手段が group-writable ビットの除去になっていること / 分岐が `cause` に対する `switch` だけで行われ `detail` を検査していないこと（AC-14） / 「不完全」を固定した実行で CGO ビルドが実際に拒否経路へ入り、そのとき出る文面が CGO 版のものであることを確認したか（§5.5） / glibc 既定構成テーブルの確認結果が `02_architecture.md` §3.2.1 に追記され、AC-03 の前提と食い違っていないこと

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` step 8 のパネルモード・トリガのうち「セキュリティゲート／移行」に該当する。CGO ビルドの書き込み判定を fail-open から fail-closed へ反転させ、利用者が従来できていた操作が拒否されるようになる。加えて glibc 既定構成テーブルの一次情報確認は結果が未知であり、内容しだいで AC-03 の改訂を提案するという前例のない判断が生じうる。

> **Phase 2 と Phase 3 を1つの PR にまとめる理由。** 2つを別々の PR にすると、その間 `main` は「CGO ビルドで拒否するが、非 CGO 版の文面で `CGO_ENABLED=1` でのビルドを勧める」状態になる。既に `CGO_ENABLED=1` でビルドしている利用者を、既に居る場所へ誘導する案内であり、AC-12 が無くそうとしているもの、および §9「運用への案内」が成功基準に挙げているものそのものである。Phase 3 は Phase 2 に依存しない（`adviseIncompleteness` の3層分割は単独でグリーンになる）ため、順序を入れ替えれば窓は塞げるが、それには承認済みの `02_architecture.md` §8 の Phase 順序の改訂が要る。Phase の順序を保ったまま窓を無くせる本まとめを採る。両 Phase とも `internal/groupmembership`1パッケージに閉じており、まとめても1つの関心——「CGO ビルドを fail-closed にし、その理由と回復手段を正しく伝える」——に収まる。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/1075）
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: 起動時の警告の `cmd/runner` への配線（AC-31）

対応する設計: `02_architecture.md` §4.4。**`record`・`verify` の起動時警告は Phase 1 の移設で既に成立しており、AC-15・AC-16 はそこで検証済みである**（Phase 1 冒頭の注記）。本 Phase に残るのは、`cmd/runner` にも同じ確定を起こすための公開の入口の追加と配線、およびその呼び出し位置の検証（AC-31）だけである。他の Phase と独立しているため、単独の PR として切り出せる。

**変更するファイル**: `internal/groupmembership/nsswitch.go`・`manager_test.go`・`cmd/runner/main.go`・`cmd/runner/startup_order_guard_test.go`

**作業内容**

- [ ] `nsswitch.go` に公開の入口を追加する。
  ```go
  // PrecomputeEnumerationEnvironment settles the completeness verdict for
  // this process, so that a build that cannot enumerate every member on this
  // host says so at startup rather than at the first group-writable path.
  // It resolves no UID and returns no error; EnsurePermissionCheckUID calls
  // it too, so record and verify need no change.
  func PrecomputeEnumerationEnvironment() {
  	precomputeEnumerationEnvironment()
  }
  ```
- [ ] `cmd/runner/main.go` の `run` 関数で、`bootstrap.SetupLogging(...)` が `nil` を返した直後に `groupmembership.PrecomputeEnumerationEnvironment()` を呼ぶ。呼び出しの上に、位置の理由（ログ設定の後でなければ警告が出力先に届かず、最初の検証の前でなければ拒否に先行しない）を述べる英語のコメントを置く。
- [ ] `manager_test.go` に `TestPrecomputeEnumerationEnvironmentSettlesTheVerdict` を追加する（AC-31 のうちパッケージ側）。`resetNsswitchClassification` から始め、`PrecomputeEnumerationEnvironment()` を呼んだ後に `nsswitchVerdictResolved` が真であることを検証する。`t.Parallel()` は宣言しない。
- [ ] `cmd/runner/startup_order_guard_test.go` に `TestEnumerationEnvironmentPrecomputeOrder` を追加する（AC-31 のうち呼び出し位置）。既存の `startupOrderOptions` に倣い、次の3つを追跡する `Options` を作って `identitymutationguard.FindRefsWithOptions(t, ".", ...)` を呼び、`run` 内の位置を比較する。
  - [ ] `{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap", FuncName: "SetupLogging"}`
  - [ ] `{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/groupmembership", FuncName: "PrecomputeEnumerationEnvironment"}`
  - [ ] `{ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap", FuncName: "NewVerificationManager"}`
  - [ ] 既存の `onlyCallSite` を使い、3件がいずれもちょうど1件であること（空振りしないこと）と、位置が `SetupLogging` < `PrecomputeEnumerationEnvironment` < `NewVerificationManager` の順であることを検証する。
  - [ ] 既存の `TestStartupPrivilegeDropOrder` が持つ「control」サブテストに倣い、順序を入れ替えたソース文字列を `identitymutationguard.RefsInSourceWithOptions` に与えて、走査が実際に順序を見ていることを確かめるサブテストを置く。

- [ ] **latch を作り替える（2026-08-29 決定。mutex を外し、起動時の1回に固定する）。** 本 Phase で3つのバイナリすべてが起動時に確定させるようになるため、ここが厳密化の自然な置き場である。Phase 3 以前に入れると、`cmd/runner` がまだ確定させないため group-writable なパスを全部拒否してしまう。
  - [ ] 根拠: 現在のフローに `nsswitchVerdict()` を並行に呼ぶ経路は無い。production の `go`／`wg.Go` は Slack 送信の2箇所（`internal/logging/slack_sender.go`・`internal/runner/bootstrap/logger.go`）だけで、いずれも群所属を参照しない。コマンド実行は逐次である。したがって `nsswitchVerdictMu` は何も守っていない。
  - [ ] `nsswitchVerdictMu`・`nsswitchVerdictResolved`・`settleNsswitchVerdict`（と `justSettled`）を削除する。`nsswitchVerdictValue` は「起動時に goroutine が存在しないうちに1回だけ書き、以後は読むだけ」の変数にし、その旨を英語のコメントで宣言する。
  - [ ] `precomputeEnumerationEnvironment` が分類と `report` の両方を行い、`nsswitchVerdict()` は `nsswitchVerdictValue` を返すだけにする。「記録はロックの外で出す」というコメントは、ロックが無くなるため削除する。
  - [ ] **未確定のまま列挙へ到達した場合は拒否する。** 追加のコードは不要である: ゼロ値 `completenessUnstated` が `manager.go` の `switch` の `default` に落ち、`unstatedCompletenessError` で拒否される。この「起動時に確定していなければ拒否」が成り立つことをテストで固定する。
  - [ ] `TestNsswitchVerdictAgreesAcrossGoroutines` を削除する（守るべき並行性が無くなるため）。削除後に `go tool cover -func` が両構成で関数ごとに不変であることを確認する。
  - [ ] `resetNsswitchClassification`・`useNsswitchVerdict` を、mutex を使わない形へ書き換える。
  - [ ] `02_architecture.md` の §2.2（ロック順序の注記）・§3.2（`settleNsswitchVerdict` を含む3シンボルの提示）・§7.1 を、作り替え後の姿へ改訂する。改訂は本 Phase の実装と同じコミットで行う。
  - [ ] §5.3 に無効化確認の行を足す: 起動時の確定を外した状態で、「未確定なら拒否される」ことを主張するテストが失敗すること。

**完了判定条件**

- [ ] `make test`・`make lint` が両構成で成功する。
- [ ] `rg -c 'PrecomputeEnumerationEnvironment\(\)' cmd/runner/main.go` が `1`。
- [ ] `rg -c 'PrecomputeEnumerationEnvironment' cmd/record/main.go cmd/verify/main.go` が一致無し（`record`・`verify` は `EnsurePermissionCheckUID` 経由のままであること）。
- [ ] `make deadcode` が `PrecomputeEnumerationEnvironment` を到達不能として報告しない。
- [ ] §5.3 の無効化確認 `X3`・`X4`・`X9` を実施し、コミットメッセージに英語で記す。

### PR-3 作成ポイント: startup warning entry point and runner wiring

**対象ステップ**: Phase 4

**推奨タイトル**: `feat(0169): settle the completeness verdict at runner startup`

**レビュー観点**: `PrecomputeEnumerationEnvironment()` の呼び出しが `run` 内でログ設定の後・最初の検証の前にあること（AC-31） / 順序ガードが追跡する3つの呼び出しをちょうど1件ずつ解決し、空振りしないこと（`onlyCallSite` と control サブテスト） / `record`・`verify` が `EnsurePermissionCheckUID` 経由のままで、新たな起動時失敗条件が増えていないこと

**実装モデル要件**: frontier-recommended

**判定理由**: 「孤立した高リスク・複雑なステップ」の枝に該当する。`nsswitchVerdict` が記録を出すのは latch が `false → true` に遷移した回だけであり、この状態遷移を取り違えると、書いたテストが**通るのに何も証明しない**（`useNsswitchVerdict` が latch を確定させるため `justSettled` が偽になり、記録が0件になる。Phase 1 の注記）。同じ罠を実際に一度踏んでいる。加えて本タスクで唯一 `cmd/runner` に配線を入れる Phase であり、順序の主張は AST 走査でしか確かめられない。**Conditional checks には該当しない**——新規の非 `_test.go` ソースはビルドタグを持たず（`nsswitch.go`・`cmd/runner/main.go` はいずれもタグ無し）、`identitymutationguard` の API 確認は cleanup/close/logout のライフサイクル呼び出しではなく、プロセス内のパッケージ変数は「外部の長寿命な状態」ではない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: 文書と残件一覧（AC-24〜AC-30）

対応する設計: `02_architecture.md` §5.5。日本語版を先に更新・コミットし、英語版は `/mktrans` で反映する。

**変更するファイル**: `docs/user/security-risk-assessment.ja.md`・`docs/user/record_command.ja.md`・`docs/user/verify_command.ja.md`・`CHANGELOG.ja.md`・`docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md`、および `/mktrans` による英語版4件

**作業内容**

- [ ] `docs/user/security-risk-assessment.ja.md` §3 の段落「なお CGO ビルドにも既知の制限がある……」を書き換える（AC-24）。
  - [ ] SSSD/LDAP 等が構成されたホストでは CGO ビルドでも書き込み安全性判定が拒否されること。既存の「この場合は CGO ビルドでも書き込み安全性判定が実際より緩く評価される可能性がある」を、`CGO ビルドでも書き込み安全性判定が拒否される` という表現を含む文へ置き換える（§3.1 の `AC-24` がこの表現を検査する）。
  - [ ] `files`・`systemd` のみの環境では従来どおり判定できること。
  - [ ] `GOOS` が `linux` 以外の CGO ビルドも「不完全」と判定されること。
  - [ ] #1064 への参照を、本タスクで解消済みである旨の記述に改める。
  - [ ] 完全性の判定が見るのは `/etc/nsswitch.conf` の `passwd`・`group` の2行だけであり、`netgroup` 行は判定に影響しないこと（AC-30）。**`netgroup` 行は判定に影響しません** という文をそのまま含める（§3.1 の `AC-30` がこの表現を検査する）。Ubuntu の既定である `netgroup: nis` を見て自ホストが該当すると誤認しないよう、理由（ネットグループは GID を持たず、`getgrgid_r` が返すことも `st_gid` に現れることもない）を1文添える。
- [ ] `docs/user/record_command.ja.md` §5.7「group-writable なファイルの書き込みが拒否される（列挙不完全）」を更新する（AC-25、AC-30）。
  - [ ] 既存の3つのエラー例の直前に、その例が非 CGO ビルド（`user_database_source=passwd-file`）のものであることを明記する。
  - [ ] CGO ビルドの例を追加する。`user_database_source=nss` のメッセージ全文を、Phase 3 で確定した `fact`・`remediation` から実際の出力どおりに転記する。
  - [ ] 対処法の表に「対象ビルド」の列を加えるか、CGO ビルド用の表を分けて置く。CGO ビルドの回復手段は「対象パスの group-writable ビットを外す」「`passwd`・`group` の両行を `files`・`systemd` のみで構成する」の2つであり、**`CGO_ENABLED=1` でのビルドは回復手段にならない**ことを明記する。
  - [ ] CGO ビルドでは `cause=malformed-line` が発生しないことを1文で述べる。
  - [ ] `runner` の実行前検証（`internal/security` のディレクトリ権限検査）での拒否は `slog` の構造化された記録を持たず、どのディレクトリで拒否されたかはエラー本文から読む必要があることを手順として書く（`02_architecture.md` §4.1）。
  - [ ] `record` が途中で拒否された場合の復旧手順——ハッシュディレクトリの内容と対象一覧を突き合わせてどこまで書けたかを確認し、回復手段を適用したうえで `record` を再実行する——を書く（`02_architecture.md` §5.5）。
  - [ ] 「事前の検知」の節に、事前確認には `verify` を用いること、`record` は警告を出しても実行を止めずハッシュファイルの書き込みへ進むため事前確認に使わないことを書く。
  - [ ] 判定が `passwd`・`group` の2行だけを見ること、および `netgroup` 行が判定に影響しないことを明記する（AC-30）。
- [ ] `docs/user/verify_command.ja.md` §5.8「ハッシュディレクトリの書き込み安全性判定が拒否される（列挙不完全）」に、上記のうち `record` 固有の復旧手順を除く同じ更新を施す（AC-25、AC-30）。**§5.7「SUDO_UID の実在確認に失敗する」ではない**——そちらも `user_database_source` を扱うため取り違えやすいが、列挙の完全性による拒否を扱うのは §5.8 である。`verify` は読み取りのみを行い、起動時に完全性判定を確定させるため事前確認に使えることを書く。
- [ ] `CHANGELOG.ja.md` の「未リリース」→「破壊的変更」に新項目を追加する（AC-27）。既存の 0168 の項目（`#### groupmembership: 非CGOビルドで列挙不完全な環境の group-writable 書き込みをfail-closed化`）とは別項目とし、その直後に置く。
  - [ ] 見出しで対象範囲（CGO ビルド ＝ セルフビルドしたバイナリ）を示す。見出しは `#### \`groupmembership\`: CGO_ENABLED=1 ビルドで列挙不完全な環境の group-writable 書き込みをfail-closed化` とする（0168 の既存見出し「非CGOビルドで…」と機械的に区別するため、`CGO_ENABLED=1` の語を必ず含める。§3.1 の `AC-27` がこれを検査する）。公式配布バイナリはすべて `CGO_ENABLED: 0` でビルドされるため影響を受けないことを明記する。
  - [ ] `**影響範囲:**` に拒否が起きる条件2つ（`GOOS` が `linux` 以外、または `passwd`・`group` 行が `files`・`systemd` 以外を含む／行の形が読めない／読み取りに失敗する。かつ判定対象に `isTrustedGroup` の免除に当たらない group-writable な構成要素が含まれる）を書く。
  - [ ] 0168 の項目と対をなすこと（同じ完全性判定を非 CGO ビルドと CGO ビルドの双方へ適用したこと）を明記する。
  - [ ] アップグレード前に影響有無を判定する手順を、0168 の項目と同じ書式（`grep -E '^(passwd|group):' /etc/nsswitch.conf` と、対象パスの構成要素を根まで辿る `while` ループ）で書く。`grep` を `passwd`・`group` に限定してよい理由（`netgroup` 行は判定に影響しない）を1文添える（AC-30）。
  - [ ] 回復手段（group-writable ビットを外す／両行を `files`・`systemd` のみにする）と、`CGO_ENABLED=1` でのビルドが回復手段にならないことを書く。
  - [ ] 切り戻し方法（`CGO_ENABLED=0` でのビルドし直しでは回避できず、本変更を含まないバージョンを使うほかない。設定やハッシュファイルの形式は変わらないため追加の作業は要らない）を書く。
- [ ] `docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` §2 D1 を更新する（AC-28）。
  - [ ] 「（新規）CGO ビルドの列挙完全性」の箇条書き（`- **（新規）CGO ビルドの列挙完全性**:` から `→ [#1064]…を作成済み。` までの8行）を削除する。**直後の「（新規）release.yml の darwin 非 CGO ビルドと Makefile の想定の不整合」の箇条書きは残す**（#1067 の守備範囲であり、本タスクの対象外）。
  - [ ] 同節が既に用いている引用ブロックの書式で、本タスクと #1064 への参照を含む解消済みの記述を追加する。引用ブロックの見出しは `> **（新規）CGO ビルドの列挙完全性 について**:` とする（§3.1 の `AC-28` がこの表現を検査する）。
  - [ ] 「対象外」で分離した `internal/runner/base/security` の誤検知（`file_validation.go` の `isUserInGroup` が `GroupIds()` を使わず `GetGroupMembers` を直接引くため、SSSD 環境で正当なメンバーが「非メンバー」と判定される）を、[#1071](https://github.com/isseis/go-safe-cmd-runner/issues/1071) への参照つきで残件として箇条書きで追加する。
  - [ ] D1 以外の節（E1・D2・A1・B1・B2・C1・C2・C3・A3・A7）に差分行が現れないことを確認する（AC-29）。
- [ ] ここまでを日本語版としてコミットする。
- [ ] `/mktrans` で英語版4件（`security-risk-assessment.md`・`record_command.md`・`verify_command.md`・`CHANGELOG.md`）へ反映する（AC-26、AC-30）。反映にあたり次の2点を守る。
  - [ ] `CHANGELOG.md` の新しい見出しにも `CGO_ENABLED=1` の語をそのまま残す。§3.1 の `AC-27` が英語版にも同じ検査をかけるためであり、0168 の既存見出し "on non-CGO builds" と機械的に区別する必要があるためでもある。「on CGO-enabled builds」のような言い換えは、訳として正しくても検査を落とす。
  - [ ] `security-risk-assessment.md` から、書き換え前の記述の英語版（`evaluated more permissively than it actually is even on a CGO build`）が消えていること。§3.1 の `AC-26` がこれを反映漏れの検出に使う。

**完了判定条件**

- [ ] §3 の AC-24〜AC-30 の各静的検査が期待どおりの結果を返す。
- [ ] 追加した CGO ビルド向けのエラーメッセージ例が、Phase 3 で実装した文面と一字一句一致する（§5.4 の突き合わせ手順）。
- [ ] `make verify-docs` が成功する。文書に対する検査はこの target が担う——`make test` は `unit-test`（`go test` の2構成実行）だけであり、`verify-docs` を依存に持たない。
- [ ] `make test`・`make lint` が両構成で成功する（文書のみの変更であるため回帰の確認としてのみ行う）。

### PR-4 作成ポイント: user documentation and remaining-issue records

**対象ステップ**: Phase 5

**推奨タイトル**: `docs(0169): document the cgo denial and its remediation`

**レビュー観点**: 追加した CGO ビルド向けのエラーメッセージ例が、PR-3 で実装した文面と一字一句一致すること（§5.4 の突き合わせ手順） / 既存の非 CGO ビルド向けの記述と対象ビルドが区別でき、回復手段が混同されないこと / `CGO_ENABLED=1` でのビルドが CGO ビルドの回復手段として案内されていないこと（AC-12 と整合） / 日本語版を先にコミットし英語版を `/mktrans` で反映した順序になっていること / `98_remaining_issues.md` の差分が D1 節に収まっていること（AC-29）

**実装モデル要件**: standard

**判定理由**: frontier のトリガに該当しない。文書のみの変更で production コードを触らず、転記元の文面は PR-3 で確定済みである。突き合わせの手順は §5.4 が、静的検査は §3・§3.1 が定めている。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 受け入れ基準の検証

各受け入れ基準に対する検証を、`test`（実行可能・誤った挙動で失敗する）・`static`（コマンドの結果が機械的に判定できる）・`manual`（人による確認が必要）で分類する。テスト名は実装時に確定するため、下表の名称を実装の指針とする。

**静的確認コマンドの前提**: 下表の `rg` はすべて既定設定（Rust 正規表現）で実行する。パイプを含むコマンドは §3.1 にまとめ、下表からは AC 番号で参照する。加えて次の3点に注意する。

- `rg -c` は一致が0件のとき何も出力せず終了コード **1** を返す（`0` とは表示しない）。以下で「一致無しで合格」と書いた検査はこの状態を指す。
- **終了コード 2 は失敗である。** 対象ファイルが存在しないとき `rg` は `No such file or directory` を出して 2 を返す。終了コードだけで判定すると「一致無し」と区別できず、まだ作っていないファイルに対する検査が空振りしたまま合格に見える。本タスクで新規追加するファイル（`incompleteness_advice*.go`）と新規追加するテスト関数を対象とする検査には、**実在を確かめる検査を必ず併記する**（Phase 2・Phase 3 の完了判定条件に置いた）。
- 複数のパスを渡した `rg -c` は `path:count` の形で出力し、**一致0件のファイルは行ごと出力しない**。したがって「N ファイルすべて `1` 以上」は、値だけでなく**出力行数が対象ファイル数と一致すること**で判定する。

| AC | 種別 | 実装箇所 | 検証 |
|---|---|---|---|
| AC-01 | test | `membership_cgo.go` `getGroupMembers` | `internal/groupmembership/membership_cgo_test.go::TestGetGroupMembers_CarriesTheSettledVerdict`（`completeVerdict()` を固定した場合。グループが存在する GID と存在しない GID の両方） |
| AC-01 | test | `nsswitch.go` `classifyNSSCompleteness` | `internal/groupmembership/nsswitch_test.go::TestClassifyNSSCompleteness`（`linux` かつ `files`／`files systemd` が「完全」）。Phase 1 のタグ除去により CGO ビルドでも実行される |
| AC-02 | test | 同上 | `::TestGetGroupMembers_CarriesTheSettledVerdict`（`incompleteVerdict(causeNSSSources, "passwd: sss")` を固定した場合。libc がエラーを返していない状況で「不完全」が返ることを踏む）、`nsswitch_test.go::TestClassifyNSSCompleteness`（`sss`・`ldap` 等が「不完全」になる各行） |
| AC-02 | static | 同上 | `rg -c 'nsswitchVerdict\(\)' internal/groupmembership/membership_cgo.go` が `1`、かつ `rg -c 'completeVerdict\(\)' internal/groupmembership/membership_cgo.go` が一致無し（現状 2 件）。CGO 版が固定値ではなく完全性判定を載せていること |
| AC-03 | test | `classifyNSSCompleteness` | `nsswitch_test.go::TestClassifyNSSCompleteness`（`nsswitchAbsent` が「完全」、`nsswitchReadFailed`・`passwd` 行欠落・`group` 行欠落・行の重複・角括弧未閉じ・ゼロ値 `nsswitchUnread` が「不完全」の各行） |
| AC-03 | test | `readNsswitchSnapshotFrom` | `nsswitch_test.go::TestReadNsswitchSnapshotFrom`（不在 → `nsswitchAbsent`、`chmod 0000` → `nsswitchReadFailed`、通常 → `nsswitchRead`）。読み取り失敗を不在と取り違えると「完全」の申告に化けるため、分類表だけでは足りない |
| AC-04 | test | `classifyNSSCompleteness` | `nsswitch_test.go::TestClassifyNSSCompleteness`（`goos` が `darwin` および `linux` 以外のその他で `causeUnsupportedPlatform` の「不完全」になる行）。Phase 1 のタグ除去により CGO ビルドでも実行される |
| AC-05 | test | `manager.go` `isUserOnlyGroupMember` | `manager_test.go::TestCanUserSafelyWriteFile_IncompleteEnumeration`（既存。メンバー集合が本人1名でも `ErrGroupMemberEnumerationIncomplete`）、`::TestIsUserOnlyGroupMember_Completeness`（既存）。いずれもタグ無しのため CGO ビルドで実行される |
| AC-06 | test | `manager.go` `getGroupEnumeration` | `manager_test.go::TestCompletenessSurvivesCache`（既存。同じ GID を2回引き、2回目も同じ拒否になる。列挙が1回しか呼ばれないこともクロージャのカウンタで検証する） |
| AC-07 | static | `membership_cgo.go`（Phase 1 で削除） | `rg -c 'always report a complete enumeration' internal/groupmembership/` が一致無し（現状 `membership_cgo.go` に 1 件）。かつ `rg -l 'func precomputeEnumerationEnvironment' internal/groupmembership/` が `nsswitch.go` の1件のみを返す（現状は `membership_nocgo.go`・`membership_cgo.go` の2件）。**「2件を返せば合格」と書いてはならない**——それは今日の木の状態そのものであり、何もしなくても合格する |
| AC-07 | manual | 同上 | `membership_cgo.go` の `getGroupMembers` の doc コメントと `nsswitch.go` の `precomputeEnumerationEnvironment` の doc コメントを読み、完全性の申告について述べており、かつ「libc の lookup が成功すれば完全」という根拠がどこにも残っていないことを確認する |
| AC-08 | static | `nsswitch.go` | `rg -c '^//go:build' internal/groupmembership/nsswitch.go` が一致無し（現状 1 件）。かつ `rg -l 'completeNSSSources = map' internal/groupmembership/` が `nsswitch.go` の1件のみを返す（許可リストが複製されていないこと） |
| AC-08 | static | `membership_nocgo.go` | §3.1 の `AC-08` のコマンドが一致無し（移設漏れが無いこと。現状 3 件） |
| AC-09 | test | `classifyNSSCompleteness` | `nsswitch_test.go::TestClassifyNSSCompleteness`。Phase 1 のタグ除去により、同じ22行の期待値テーブルが両ビルドで実行される。**当初は `manager_test.go` に専用の一致テーブルを置く計画だったが取り止めた**（レビュー指摘。Phase 1 の該当行に理由を記す） |
| AC-09 | static | — | `make test` が `CGO_ENABLED=1`・`CGO_ENABLED=0` の双方で成功する（同一の期待値テーブルが両構成で通ること） |
| AC-10 | static | `membership_files.go`・`membership_files_nocgo.go` | `rg -c '^//go:build !cgo \|\| test' internal/groupmembership/membership_files.go` が `1`、`rg -c '^//go:build !cgo$' internal/groupmembership/membership_files_nocgo.go` が `1`（非 CGO 専用シンボルのタグが据え置かれていること） |
| AC-10 | static | — | `make deadcode` の出力が本タスクの前後で変わらない。§3.1 の `AC-10` のコマンドで差分を取る |
| AC-11 | test | `manager.go` `incompleteEnumerationError` | `internal/groupmembership/incompleteness_advice_cgo_test.go::TestAdviseIncompleteness_CGO` の `incompleteEnumerationError` を通すサブテスト（`user_database_source=nss`・`cause=nss-sources`・`detail=passwd: sss` を含み、`ErrGroupMemberEnumerationIncomplete` で包まれていること） |
| AC-12 | test | `incompleteness_advice_cgo.go` `adviseIncompleteness` | `::TestAdviseIncompleteness_CGO`。次の4点を検証する。(1) どの原因の `fact`・`remediation` にも `"CGO_ENABLED"` が現れない、(2) ホスト起因の原因は `"chmod g-w"` を案内し、実装の誤りを指す原因は `implementationDefectAdvice` の回復手段を返す、(3) **原因ごとの advice が互いに異なる**、(4) `adviseIncompleteness` が返した文字列そのものが組み立て後のメッセージに現れる。**CGO 版の文面は全文照合しない**——後述 |
| AC-12 | static | 同上 | §3.1 の `AC-12` のコマンドで3ファイルの実在（`3`）を確かめたうえで、`rg -c 'CGO_ENABLED' internal/groupmembership/incompleteness_advice_cgo.go internal/groupmembership/incompleteness_advice.go internal/groupmembership/manager.go` が一致無し。**実在の確認を先に行わないと、ファイル未作成による終了コード 2 を「一致無し」と取り違える** |
| AC-13 | test | `incompleteness_advice_nocgo.go` | `internal/groupmembership/incompleteness_advice_nocgo_test.go::TestAdviseIncompleteness_NoCGO`。**文面の全文照合は行わない**（下記「計画からの相違」）。検証するのは性質——ホストが生む3原因はいずれも `CGO_ENABLED=1` でのビルドを案内すること、`causeNSSSources` だけが `/etc/nsswitch.conf` を名指すこと、`causeMalformedLine` の回復手段が `NIS` に触れること、原因ごとの advice が互いに異なること、`adviseIncompleteness` が返した文字列がメッセージに現れること |
| AC-13 | static | 同上 | §3.1 の `AC-13` のコマンドが `6` を返す。移設前の `manager.go` の**事実3つと回復手段3つ**が、移設後は `incompleteness_advice_nocgo.go` に一字一句そのまま存在すること。**回復手段だけを照合してはならない**——既存の `TestIncompleteEnumerationErrorMessage` の `wantContains` も事実の全文を主張していないため、事実を書き換えるとどの検査にも掛からない |
| AC-14 | test | 両ビルドの `adviseIncompleteness` | `::TestAdviseIncompleteness_CGO` の `causeUnspecified`・`causeMalformedLine` の各行と「cause outside the defined range」のサブテスト（実装の誤りを指す文面が返ること）、および `manager_test.go::TestIncompleteEnumerationErrorMessage` の2ケース（非 CGO 版の `causeUnspecified`・`default` を `incompleteEnumerationError` 経由で踏む）。**`TestAdviseIncompleteness_NoCGO` はここには挙げない**——同テストの行はホストが生む3原因だけであり、`causeUnspecified`・`causeOutOfRange` の行を持たない |
| AC-14 | static | `incompleteness_advice*.go` | §3.1 の `AC-14` のコマンドが一致無し（`detail` の内容で分岐していないこと） |
| AC-15 | test | `manager.go` `EnsurePermissionCheckUID` | `manager_test.go::TestEnsurePermissionCheckUIDPrecomputesEnvironment`（Phase 1 で `membership_nocgo_test.go` から移設。移設後は CGO ビルドでも実行される） |
| AC-15 | test | `nsswitch.go` `nssCompletenessReporter` | `nsswitch_test.go::TestNSSCompletenessReporter_ReportsOnlyOnce`（既存。「不完全」に対し記録が1件だけ出ること）。**当初は共有インスタンスを対象とする専用テストを足す計画だったが取り止めた**（レビュー指摘。Phase 1 の該当行に理由を記す） |
| AC-15 | manual | `nsswitch.go` `nsswitchVerdict` | §5.5 の強制実行。`/etc/nsswitch.conf` を `passwd: sss` に書き換えたホストで `record` を1回起動し、警告が実際に1度だけ出ることを確かめる。`TestNsswitchVerdictReportsWhatItSettled` は分類が「完全」になるホストでは「記録が無いこと」を確かめるにとどまり陽性方向を踏まないため、この強制実行で補う |
| AC-16 | test | `nssCompletenessReporter.report` | `nsswitch_test.go::TestNSSCompletenessReporter_Report`（既存。メッセージ本文と `user_database_source`・`cause`・`detail` の3属性。Phase 1 のタグ除去により CGO ビルドでも実行され、`userDatabaseSource` を期待値に使うためビルドごとに `nss`／`passwd-file` を要求する） |
| AC-17 | test | `IsUserInGroup`・`CanCurrentUserSafelyReadFile` | `manager_test.go::TestReadPathIgnoresCompleteness`（既存。「不完全」を注入しても結果が「完全」の場合と一致する） |
| AC-18 | static | `manager.go` `GetGroupMembers` | `rg -c 'func \(gm \*GroupMembership\) GetGroupMembers\(gid uint32\) \(\[\]string, error\)' internal/groupmembership/manager.go` が `1`（シグネチャが変わっていないこと） |
| AC-18 | static | `internal/runner/base/security`・`internal/safefileio` | §3.1 の `AC-18` のコマンド（両パッケージの production コードに差分が無いこと）。実環境の列挙に依存するテストの更新は本タスクの挙動変更の当然の帰結であり、この保証の対象外とする |
| AC-19 | test | `CanUserSafelyWriteFile` | `manager_test.go::TestCanUserSafelyWriteFile_CompleteEnumeration`（既存。「完全」を注入し、唯一のメンバーの許可と共有グループの拒否）、`::TestCanUserSafelyWriteFile`（既存。world-writable の一律拒否・非所有者の拒否・owner-writable の許可。`New()` を使うが権限ビットが group-writable でないため列挙に到達しない。§1.3(c)） |
| AC-20 | test | `useNsswitchVerdict` を用いる各テスト | `membership_cgo_test.go::TestGetGroupMembers_CarriesTheSettledVerdict` が `useNsswitchVerdict` で完全性判定を固定し、実行ホストの `/etc/nsswitch.conf` を読まずに判定を検証する |
| AC-20 | static | `nsswitch_test.go`・`membership_cgo_test.go` | §3.1 の `AC-20` のコマンド。2つの関数の**実在を確かめてから**、その本体にファイルを開く呼び出しが無いことを確認する。**この検査が言うのは対象の2関数についてだけである**——`membership_cgo_test.go::TestGetGroupMembers_StatesTheHostVerdict` は期待値を得るために `readNsswitchSnapshot()` を呼ぶため実行ホストの `/etc/nsswitch.conf` を読む。同テストはホストの分類との一致を確かめるものであり、完全性判定を固定して検証する側ではない。**実在の確認は省けない**——`awk` が関数を見つけられなければ空の入力が `rg` に渡り、テストを1行も書いていない状態で「一致無し」＝合格になる |
| AC-21 | static | PR-1〜PR-3 の各ブランチ | §3.1 の `AC-21` のコマンドが `1` 以上を返す。**PR ごとに main へマージして次を新しいブランチで始めるため、`main..HEAD` には当該ブランチ分しか含まれない。** 各 Phase の完了判定条件で個別に確認する。PR-4（Phase 5）は §5.3 に担当行を持たないため対象外である |
| AC-21 | manual | 同上 | §5.3 の `X1`〜`X9` の無効化確認を実施し、対応するテストが実際に失敗することを確かめる。各行の担当 Phase は同表の「実施する Phase」列が定める |
| AC-22 | static | `membership_semantics_test.go` | §3.1 の `AC-22` のコマンド（同ファイルに差分が無いこと） |
| AC-22 | test | 同上 | `membership_semantics_test.go::TestGetGroupMembers_CGOAndNoCGOSemanticsMatch` が CGO ビルドで従来どおり通過する（skip 条件も比較結果も変わらない） |
| AC-23 | static | — | `make test` の終了コードが 0（`CGO_ENABLED=1` と `CGO_ENABLED=0` の両方を実行する）。`make lint` の終了コードが 0（同じく両構成） |
| AC-24 | static | `security-risk-assessment.ja.md` | `rg -c '緩く評価される可能性がある' docs/user/security-risk-assessment.ja.md` が一致無し（現状 1 件）。かつ §3.1 の `AC-24` のコマンド（書き換え後の節に「拒否」が入ったことの陽性の裏取り）が `1` 以上 |
| AC-24 | manual | 同上 | 書き換えた段落を `nsswitch.go` の分類規則と突き合わせ、`files`・`systemd` のみの環境では従来どおり判定できるという記述が実装と一致することを確認する |
| AC-25 | static | `record_command.ja.md`・`verify_command.ja.md` | `rg -c 'user_database_source=nss, cause=nss-sources' docs/user/record_command.ja.md docs/user/verify_command.ja.md` が両ファイルとも `1` 以上（現状はいずれも0件） |
| AC-25 | manual | 同上 | 追加した CGO ビルド向けの例文を、Phase 3 で実装した `fact`・`remediation` と1文字ずつ突き合わせる（§5.4）。既存の非 CGO ビルド向けの例と対象ビルドが区別できること、回復手段が異なることが読み取れることをあわせて確認する |
| AC-26 | static | 英語版4ファイル | `rg -c 'user_database_source=nss, cause=nss-sources' docs/user/record_command.md docs/user/verify_command.md` が両ファイルとも `1` 以上、かつ `rg -c 'user_database_source=nss' CHANGELOG.md` が `1` 以上（現状0件）、かつ `rg -cF 'evaluated more permissively than it actually is even on a CGO build' docs/user/security-risk-assessment.md` が一致無し（現状 1 件。日本語版の書き換えが英語版へ届いたことの検出）。**`rg -c 'nsswitch' CHANGELOG.md` は 0168 が既に3件入れているため検査にならない** |
| AC-26 | manual | 同上 | `/mktrans` の出力を日本語版と突き合わせ、更新した各節が対応していることを確認する |
| AC-27 | static | `CHANGELOG.ja.md`・`CHANGELOG.md` | §3.1 の `AC-27` のコマンド。「未リリース」→「破壊的変更」の範囲内に、CGO ビルドを対象とする新しい見出しが1件あること（日本語版・英語版とも）。かつ `rg -c 'user_database_source=nss' CHANGELOG.ja.md` が `1` 以上（現状0件） |
| AC-27 | manual | 同上 | 追加した項目の「アップグレード前に影響有無を判定する手順」を本コンテナ（`files` 構成）で実際に実行し、影響なしと判定されることを確認する。書式が同節の既存項目（見出し・`**影響範囲:**`・判定手順）に揃っていること、および 0168 の項目との関係が読み取れることをあわせて確認する |
| AC-28 | static | `98_remaining_issues.md` | §3.1 の `AC-28` の2つのコマンド。D1 節に限定して「（新規）CGO ビルドの列挙完全性」の箇条書きが消えたこと（一致無し）と、引用ブロックが1件追加されたこと（`1`）。かつ `rg -c 'issues/1071' docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` が `1` 以上 |
| AC-29 | static | 同上 | §3.1 の `AC-29` のコマンド。D1 節を除いた全文が `main` と `HEAD` で一致すること（`diff` の出力が空）。**diff のハンクヘッダから節名を読む方法は使えない**——本リポジトリの `.gitattributes` は git-crypt の2行のみで markdown 用の `xfuncname` を定義しておらず、既定のヒューリスティックは `#` 見出しを関数文脈として拾わないため、ハンクヘッダは常に空になり、どの節を編集しても検査が素通りする |
| AC-29 | manual | 同上 | `git diff main...HEAD -- docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md` を目視し、E1・D2・A1・B1・B2・C1・C2・C3・A3・A7 の各節に差分行が現れないことを確認する |
| AC-30 | static | 利用者向け文書 日本語版3件 | `rg -cF 'netgroup 行は判定に影響しません' docs/user/record_command.ja.md docs/user/verify_command.ja.md docs/user/security-risk-assessment.ja.md` が3ファイルとも `1` 以上、かつ出力が3行（現状はいずれも0件）。**`rg -c 'netgroup'` のような語の有無だけの検査にしない**——「`netgroup` 行も判定の対象である」と正反対のことを書いた文書でも合格してしまう |
| AC-30 | static | 利用者向け文書 英語版3件 | `rg -c 'netgroup' docs/user/record_command.md docs/user/verify_command.md docs/user/security-risk-assessment.md` が3ファイルとも `1` 以上、かつ出力が3行（現状はいずれも0件）。英語版は `/mktrans` の訳語が事前に確定しないため語の有無で取り、内容は下の manual で確かめる |
| AC-30 | manual | 同上 | 追加した記述を日本語版・英語版とも読み、「判定が見るのは `passwd`・`group` の2行だけである」ことと「`netgroup` 行は判定に影響しない」ことの両方が明示されており、Ubuntu の既定を見た利用者が誤認しない書き方になっていることを確認する |
| AC-31 | test | `cmd/runner/main.go` `run` | `cmd/runner/startup_order_guard_test.go::TestEnumerationEnvironmentPrecomputeOrder`（`SetupLogging` < `PrecomputeEnumerationEnvironment` < `NewVerificationManager` の順であること。3件がちょうど1件ずつであることを要求して空振りを防ぐ。順序を入れ替えたソースで走査が実際に順序を見ていることを確かめる control サブテストを含む） |
| AC-31 | test | `nsswitch.go` `PrecomputeEnumerationEnvironment` | `manager_test.go::TestPrecomputeEnumerationEnvironmentSettlesTheVerdict`（公開の入口を呼んだ後に完全性判定が確定していること）。**公開の入口を実際に呼ぶテストはこれだけである**——警告側のテストは共有レポータを直接の対象とするため、入口が消えても失敗しない |
| AC-31 | static | `cmd/record`・`cmd/verify` | `rg -c 'PrecomputeEnumerationEnvironment' cmd/record/main.go cmd/verify/main.go` が一致無し（`record`・`verify` の挙動が変わらないこと。両者は `EnsurePermissionCheckUID` 経由のまま） |

### 3.1 検証コマンド集

パイプ（`|`）を含むコマンドは、markdown の表のセルに直接書くと表の桁区切りと衝突する。エスケープして `\|` と書けば表は崩れないが、そのまま貼り付けると正規表現では選択ではなくリテラルのパイプに、シェルではパイプラインではなくただの引数になり、**いずれも検査が意図どおりに働かない**。そのためパイプを含むものは以下にまとめる。上の表からは AC 番号で参照する。

```bash
# AC-08: 完全性判定を確定させる仕組みの移設漏れが無いこと。一致無しで合格（現状 3 件）。
#        `\|` と書くとリテラルのパイプを探すことになり、作業前でも一致無しになって検査が無効化される。
rg -c 'func (nsswitchVerdict|settleNsswitchVerdict|precomputeEnumerationEnvironment)\(' \
  internal/groupmembership/membership_nocgo.go

# AC-10: make deadcode の出力が本タスクの前後で変わらないこと。差分が空で合格。
#        基準は必ず「コミット済みの main」から取る。git stash で作業ツリーを退避して
#        比べる方法は使えない: 本計画は Phase ごとにコミットして PR を出すため、
#        測る時点では変更が既にコミット済みであり、stash は何も退避しない。
#        その結果 before と after が同一になり、検査が常に合格してしまう。
#        （さらに、退避するものが無いと git stash push は非ゼロ終了するため、
#        `&&` で繋いだ before 側の生成そのものが走らない。）
git worktree add /tmp/dc-base main
( cd /tmp/dc-base && make deadcode ) > /tmp/deadcode-before.txt 2>&1
make deadcode > /tmp/deadcode-after.txt 2>&1
diff /tmp/deadcode-before.txt /tmp/deadcode-after.txt
git worktree remove /tmp/dc-base

# AC-13: 非 CGO 版の文面が、移設後も一字一句そのまま存在すること。6 を返せば合格。
#        事実3つと回復手段3つの両方を照合する。回復手段だけを見ると、事実を書き換えても
#        どの検査にも掛からない（既存の TestIncompleteEnumerationErrorMessage の
#        wantContains も事実の全文は主張していない）。
#        文字列は全体を照合する。部分文字列を短く取ると、書き換えられていても一致する。
rg -cF \
  -e 'this build cannot enumerate all members of a group on this platform' \
  -e 'rebuild with CGO_ENABLED=1 so that group members are resolved through the platform'\''s own user database via libc' \
  -e '/etc/nsswitch.conf names a user database source this build cannot consult, or could not be read' \
  -e 'check the passwd and group lines of /etc/nsswitch.conf, then rebuild with CGO_ENABLED=1 so that the configured sources are consulted' \
  -e 'a line of the user database files could not be parsed and was skipped, so the members listed there are unknown' \
  -e 'check the reported line: correct it if its format is wrong, or, if it is a NIS compatibility entry (a line starting with + or -), rebuild with CGO_ENABLED=1' \
  internal/groupmembership/incompleteness_advice_nocgo.go

# AC-12 / AC-14 の前提: 3ファイルが実在すること。3 を返せば合格。
#        これを先に確かめないと、以下の「一致無し」系の検査が空振りする。
#        存在しないファイルに対する rg は No such file or directory を出して
#        終了コード 2 を返し、終了コードだけを見ると「一致無し」(1) と区別できない。
rg --files internal/groupmembership | rg -c 'incompleteness_advice(_cgo|_nocgo)?\.go'

# AC-14: 文面の選択が detail の文字列内容で分岐していないこと。一致無しで合格。
#        `\|` と書くとリテラルのパイプを探すことになり、作業前でも一致無しになって検査が無効化される。
rg -n 'strings\.(Contains|HasPrefix|HasSuffix|Index)' \
  internal/groupmembership/incompleteness_advice.go \
  internal/groupmembership/incompleteness_advice_cgo.go \
  internal/groupmembership/incompleteness_advice_nocgo.go

# AC-18: 対象2パッケージの production コードが無変更であること。出力が空で合格。
git diff --stat main...HEAD -- internal/runner/base/security internal/safefileio ':!*_test.go'

# AC-20 の前提: 対象の2関数が実在すること。いずれも 1 で合格。
#        awk が関数を見つけられないと空の入力が rg に渡り、テストを1行も書いていない
#        状態で「一致無し」＝合格になる。実在の確認は省けない。
rg -c '^func TestClassifyNSSCompleteness\(' internal/groupmembership/nsswitch_test.go
rg -c '^func TestGetGroupMembers_CarriesTheSettledVerdict\(' internal/groupmembership/membership_cgo_test.go

# AC-20: 完全性判定を検証するテストがファイルに触れていないこと。いずれも一致無しで合格。
#        関数長が変われば固定行数の -A は隣の関数へはみ出すため、awk で関数本体に限る。
#        `os\.` と広く取ってはいけない。TestClassifyNSSCompleteness は
#        `err: os.ErrPermission` で読み取り失敗の snapshot を組み立てており、
#        これはファイルに触れる呼び出しではなく sentinel 値である。作業前から
#        1 件一致してしまい、検査が働かない。ファイルを開く関数名で限定する。
awk '/^func TestClassifyNSSCompleteness\(/{f=1} f&&/^}$/{print;f=0} f' \
  internal/groupmembership/nsswitch_test.go \
  | rg -c 'os\.(ReadFile|Open|OpenFile|WriteFile|Create|Stat|Remove)|t\.TempDir'
awk '/^func TestGetGroupMembers_CarriesTheSettledVerdict\(/{f=1} f&&/^}$/{print;f=0} f' \
  internal/groupmembership/membership_cgo_test.go \
  | rg -c 'os\.(ReadFile|Open|OpenFile|WriteFile|Create|Stat|Remove)|t\.TempDir'

# AC-21: 作業中のブランチにある「無効化確認を記録したコミット」の件数。1 以上で合格。
#        PR ごとに main へマージするため、main..HEAD は当該ブランチ分しか含まない。
#        マージ後に走らせても 0 になるので、必ず PR を出す前のブランチ上で実行する。
#        --no-show-signature は必須。本リポジトリは log.showSignature=true のため、
#        付けないと署名検証行が %H の出力に混ざり、git show が「invalid object name」で失敗する。
#        範囲は二点の main..HEAD にする。三点の main...HEAD は対称差であり main 側の
#        無関係なコミットも数えて誤って合格する。文言も 'disabled the' に固定する。
git log --no-show-signature --format='%H' main..HEAD | while read -r h; do
  git show -s --no-show-signature --format=%B "$h" | rg -qi 'disabled the' && echo "$h"
done | wc -l

# AC-22: 意味論一致テストが本タスクで変更されていないこと。出力が空で合格。
git diff main...HEAD -- internal/groupmembership/membership_semantics_test.go

# AC-24: 書き換え後に「CGO ビルドでも拒否される」と述べていること（陽性の裏取り）。1 で合格。
#        `rg -A 20 '既知の制限' | rg -c 'CGO ビルド'` のような広い取り方は、非 CGO ビルド
#        向けの既存の段落に作業前から3件一致し、検査が働かない。書き換え後の一文を
#        そのまま照合する（Phase 5 でこの表現に定める。現状0件）。
rg -cF 'CGO ビルドでも書き込み安全性判定が拒否される' \
  docs/user/security-risk-assessment.ja.md

# AC-27: 「未リリース」ブロックの中に CGO ビルドを対象とする新しい見出しがあること。
#        範囲を「未リリース」から次のリリース見出し（## [1.1.1] など）までに限る。
#        限定しないと、将来この見出しが過去のリリース節へ引用された場合に誤検出する。
#        日本語版・英語版とも 1 で合格（現状はいずれも0件）。
#        `CGO` だけで取ってはいけない。0168 の既存の見出し（「非CGOビルドで…」／
#        "on non-CGO builds"）に作業前から一致し、検査が働かない。新しい見出しは
#        Phase 5 で `CGO_ENABLED=1` を含む形に定めるため、その文字列で限定する。
awk '/^## \[未リリース\]/{f=1;next} /^## \[/{f=0} f' CHANGELOG.ja.md \
  | rg -c '^#### .*CGO_ENABLED=1'
awk '/^## \[Unreleased\]/{f=1;next} /^## \[/{f=0} f' CHANGELOG.md \
  | rg -c '^#### .*CGO_ENABLED=1'

# AC-28: D1 節に限定して、解消した箇条書きが消えたこと。一致無しで合格。
#        節を限定しないと、他節の無関係な行に一致しうる。
awk '/^### D1（groupmembership）/{f=1;next} /^### /{f=0} f' \
  docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md \
  | rg -c '（新規）CGO ビルドの列挙完全性'

# AC-28: 解消済みを述べる引用ブロックが1件追加されたこと。1 で合格。
awk '/^### D1（groupmembership）/{f=1;next} /^### /{f=0} f' \
  docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md \
  | rg -c '^> \*\*.*CGO ビルドの列挙完全性 について\*\*'

# AC-29: 残件一覧の差分が D1 節に限られること。diff の出力が空で合格。
#        D1 節を取り除いた全文を main と HEAD から取り出して比べる。
#        diff のハンクヘッダ（@@ ... @@ の後ろ）から節名を読む方法は使えない。
#        本リポジトリの .gitattributes は git-crypt の2行のみで markdown 用の
#        xfuncname を定義しておらず、git 既定のヒューリスティックは `#` 見出しを
#        関数文脈として拾わない。ハンクヘッダは常に空になるため、どの節を編集しても
#        検査が素通りする（AC-29 が捕まえたい失敗そのもの）。
#        awk の分割子は AC-28 の2つの検査と同じものを使う（`!f` で D1 節を捨てる）。
extract() {
  git show "$1:docs/tasks/0149_security_code_smell_audit_fable/98_remaining_issues.md" \
    | awk '/^### D1（groupmembership）/{f=1;next} /^### /{f=0} !f'
}
diff <(extract main) <(extract HEAD)

# AC-29 の陰性対照: 上の検査が実際に働くことを1度確かめる。
#        E1 節の任意の1行をわざと書き換えて再実行し、diff が空でなくなることを見る。
#        確認後は必ずその変更を戻す。
```

---

## 4. 実装順序とマイルストーン

| マイルストーン | 対応 Phase | 成果物 | 完了の定義 |
|---|---|---|---|
| M1: 分類の共有化 | Phase 1 | ビルドタグを外した `nsswitch.go`、移設された完全性判定の確定の仕組み（定義は1つだけ）、共有される `test_helpers.go` の補助関数 | AC-08・AC-09・AC-10・AC-15・AC-16 の検証が通る。許可/拒否の判定は変わらないが、CGO ビルドの `record`・`verify` が起動時警告を出すようになる。両構成で `make test`・`make lint`・`make deadcode` が通る |
| M2: CGO 版の申告 | Phase 2 | 完全性判定を全成功経路に載せた CGO 版 `getGroupMembers`、是正した doc コメント | AC-01〜AC-06 と AC-07 の残り半分の検証が通る。SSSD を模した完全性判定を固定したとき、CGO ビルドで拒否経路に入る |
| M3: 拒否メッセージ | Phase 3 | `incompleteness_advice*.go` の3ファイルと、委譲する形になった `incompleteEnumerationError` | AC-11〜AC-14 の検証が通り、非 CGO 版の文面が1文字も変わっていない |
| M4: 起動時の警告の配線 | Phase 4 | 公開の入口 `PrecomputeEnumerationEnvironment` と、`cmd/runner` からの呼び出し | AC-31 の検証が通る。3つのバイナリすべてで警告が拒否に先行する（`record`・`verify` は M1 で成立済み） |
| M5: 文書 | Phase 5 | 利用者向け文書3件・`CHANGELOG` の日本語版と英語版、更新した残件一覧 | AC-24〜AC-30 の検証が通る |

**PR の切り方**: M1〜M3 は同じパッケージの連続した変更であり、順に積む。M4 は他と独立しているため単独の PR として切り出せる（`02_architecture.md` §8 Phase 4）。M5 は M3 で文面が確定した後でなければ書けない——利用者向け文書に転記するエラーメッセージ例が確定しないためである。

### 4.1 PR 構成

Phase を単位に PR を切る。ただし **Phase 2 と Phase 3 は1つの PR にまとめる**——分けると、その間 `main` が「CGO ビルドで拒否しつつ、非 CGO 版の文面で `CGO_ENABLED=1` でのビルドを勧める」という、AC-12 が無くそうとしている状態になるためである（PR-2 の注記）。どの PR も後続 Phase のスタブ無しで `make test`・`make lint` を両構成で通せる。

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | Phase 1 | `nsswitch.go` のビルドタグ除去、完全性判定の確定の仕組みの移設、CGO 版の空実装の削除、doc コメント5箇所の是正、テスト補助関数の集約。**CGO ビルドの起動時警告がここで有効になる**（判定は変わらない） | frontier-recommended |
| PR-2 | Phase 2 / Phase 3 | CGO 版 `getGroupMembers` が完全性判定を全成功経路に載せる（fail-closed への反転）。あわせて `incompleteness_advice*.go` の3ファイルを追加し、`incompleteEnumerationError` を委譲化する。glibc 既定構成の確認 | frontier-required |
| PR-3 | Phase 4 | 公開の入口 `PrecomputeEnumerationEnvironment` の追加と `cmd/runner` からの呼び出し、呼び出し順序の検証 | frontier-recommended |
| PR-4 | Phase 5 | 利用者向け文書3件・`CHANGELOG` の日本語版と英語版、残件一覧の更新 | standard |

**`internal/` が `cmd/` に先行していること**: `cmd/` を変更するのは PR-3 だけであり、その依存先である `PrecomputeEnumerationEnvironment` は同じ PR で `internal/groupmembership/nsswitch.go` に追加される。公開の入口と唯一の呼び出し元を分割すると、先行 PR に呼び出し元の無いエクスポート関数が残って `make deadcode` が落ちるため、1つの PR にまとめる。

**Phase 順序が設計と一致していること**: 本書の Phase 1〜5 は `02_architecture.md` §8 の Phase 1〜5 と名称・順序が一致する。順序の前提（Phase 3 の文面確定が Phase 5 の前提であること、Phase 1 の移設が Phase 4 の `record`・`verify` 側の成立条件であること）は各 Phase の記述内で述べており、番号の振り直しは行っていない。

対応 AC は1点だけ設計と異なる。**AC-07 を Phase 1 と Phase 2 に分けた**——設計 §8 は AC-07 を Phase 2 に置いているが、AC-07 が求める是正のうち「誤った根拠を述べた doc コメントを空実装ごと削除する」半分は、`nsswitch.go` のビルドタグを外すのと同じ Phase で行わなければ CGO ビルドが `precomputeEnumerationEnvironment` の重複定義でコンパイルできない（Phase 1 冒頭の注記）。分割は設計の意図（同一の関数を1つだけ置く。§3.2）に沿うものであり、Phase の順序も担当範囲も変えていない。

---

## 5. テスト戦略

### 5.1 単体テストの方針

- **実行ホストへの非依存**（AC-20）: 完全性判定に依存するテストは `test_helpers.go` の `useNsswitchVerdict` で判定を固定する。差し替え点はこの1つに絞り、CGO 版に非 CGO 版の `enumerateFromFiles` と同型の内側関数は置かない（`02_architecture.md` §3.1、§7.1）。
- **テストの独立性**: `useNsswitchVerdict`・`resetNsswitchClassification` が触るのはプロセス全体で1つの状態であるため、これらを用いるテストは `t.Parallel()` を宣言しない。また `processNSSCompletenessReporter.reported` は「プロセスにつき1回」を保証するフラグであり、先に走った任意のテストが消費しうる。AC-15・AC-16 のテストが「警告が出なかった」ことを黙って見逃すことを防ぐため、これらは必ず `resetNsswitchClassification` から始める。この要請は両補助関数の doc コメントに英語で記す。
- **重複の回避**: §1.3(c) の既存テストが既に両ビルドで AC-05・AC-06・AC-16・AC-17・AC-19 を踏んでいるため、同じ内容を CGO 専用テストとして書き足さない。新規に書くのは、既存テストが届かない範囲——CGO 版 `getGroupMembers` の申告（AC-01・AC-02）、両ビルドの分類の一致（AC-09）、CGO 版の文面（AC-11・AC-12・AC-14）、`PrecomputeEnumerationEnvironment`（AC-31）——に限る。
- **境界値と `default`**: `causeOutOfRange`・`completenessOutOfRange` は既に `manager_test.go` に定義されている。両ビルドの `adviseIncompleteness` のテストはこれを用いて `default` の枝を踏む。フェイルクローズドの成立条件（`02_architecture.md` §5.2）の各点は、対応する `switch` の `default` を含むテーブルテストで踏む。

### 5.2 テストヘルパの配置

追加する補助関数はいずれも非公開の状態（`nsswitchVerdictMu` ほか）を触るため、`docs/dev/developer_guide/test_organization.md` の Classification B に当たる。`testutil/` サブディレクトリは公開 API だけを使うヘルパのための置き場であり、本タスクの補助関数は該当しない。

**置き場は既存の `internal/groupmembership/test_helpers.go`（`//go:build test`、`package groupmembership`）とし、新しいヘルパファイルは作らない。** 同ガイドはカテゴリが複数ある場合に `test_helpers_<category>.go` を認めており、本パッケージには既に `test_helpers_policy.go` があるため、`test_helpers_nsswitch.go` を作る選択肢もある。それを採らないのは、`useNsswitchVerdict`・`resetNsswitchClassification` が既に `test_helpers.go` にある `newWithFixedEnumeration`・`newWithEnumerator` と同じ関心——1回の列挙が申告する完全性をテストから決めること——を扱い、同じテストの中で並べて使われるためである。`test_helpers_policy.go` が別ファイルなのは、基準UIDポリシーという別の関心を扱い、かつ公開 API を持つためである。`02_architecture.md` §2.2・§7.1 も `test_helpers.go` を指定している。

### 5.3 テストが理由どおりに失敗できることの確認（AC-21）

`02_architecture.md` §7.2 の表に従い、各 Phase の実装時に無効化を施して対応するテストが失敗することを確かめ、無効化の方法と結果をコミットメッセージに記す。**コミットメッセージは英語で書き、`disabled the ...` の語を必ず含める**（§3.1 の `AC-21` がこの語で件数を数えるため）。

各行に `X1`〜`X9` の名前を与える（残件一覧の節名 `D1` と紛れないよう `D` を避ける）。行番号で参照すると表の編集で参照がずれるため、**各 Phase の完了判定条件はこの名前で参照する**。

| 名前 | 無効化する内容 | 失敗するはずのテスト | 実施する Phase |
|---|---|---|---|
| `X1` | CGO 版 `getGroupMembers` が `nsswitchVerdict()` ではなく `completeVerdict()` を載せるようにする | `membership_cgo_test.go::TestGetGroupMembers_CarriesTheSettledVerdict` | Phase 2 |
| `X2` | `precomputeEnumerationEnvironment` を空実装に戻す | `manager_test.go::TestEnsurePermissionCheckUIDPrecomputesEnvironment` | Phase 1 |
| `X9` | 同上（公開の入口が加わった後に、あらためて確認する） | `manager_test.go::TestPrecomputeEnumerationEnvironmentSettlesTheVerdict` | Phase 4 |
| `X3` | `cmd/runner` の `run` から `PrecomputeEnumerationEnvironment()` の呼び出しを外す | `cmd/runner/startup_order_guard_test.go::TestEnumerationEnvironmentPrecomputeOrder` | Phase 4 |
| `X4` | `PrecomputeEnumerationEnvironment()` の呼び出しを `NewVerificationManager` の後ろへ移す | 同上（順序の比較が失敗する） | Phase 4 |
| `X5` | `isUserOnlyGroupMember` の `completenessIncomplete` の枝を削り `default` へ倒す | `manager_test.go::TestCanUserSafelyWriteFile_IncompleteEnumeration`（返る sentinel が変わる） | Phase 2 |
| `X6` | CGO 版 `adviseIncompleteness` の文面を非 CGO 版のものに差し替える | `incompleteness_advice_cgo_test.go::TestAdviseIncompleteness_CGO` | Phase 3 |
| `X7` | `classifyNSSCompleteness` の `goos != "linux"` の分岐を削る | `nsswitch_test.go::TestClassifyNSSCompleteness`（Phase 1 のタグ除去により CGO ビルドでも実行されることを、あわせて確かめる） | Phase 1 |
| `X8` | `nssCompletenessReporter.report` の `CompareAndSwap` による1回限りの抑止を外す（`r.reported.Store(true)` へ差し替える） | `nsswitch_test.go::TestNSSCompletenessReporter_ReportsOnlyOnce`（記録が3件になる） | Phase 1 |

### 5.4 新規に書く文書の突き合わせ

Phase 5 で利用者向け文書に転記するエラーメッセージ例は、Phase 3 で実装した文面から生成した実物と一致していなければならない。目視の転記では取りこぼすため、次の手順で突き合わせる。

- [ ] CGO ビルドで、`incompleteEnumerationError` が生成するメッセージを1件出力する使い捨てのテスト（`t.Log`）を実行し、その出力をそのまま `record_command.ja.md`・`verify_command.ja.md` へ貼る。
- [ ] `CHANGELOG.ja.md` に書いた「アップグレード前に影響有無を判定する手順」のコマンドを本コンテナで実際に実行し、終了コードと出力が説明どおりであることを確かめる（AC-27 の manual 検証）。
- [ ] 対処法の表が CGO ビルドと非 CGO ビルドで別々の行になっていること、および両者の回復手段が実際に異なる（`CGO_ENABLED=1` でのビルドが CGO ビルドの回復手段に含まれない）ことを確かめる。

### 5.5 ビルド構成の網羅（AC-23）

- `make test` は `CGO_ENABLED=1`（`-race` つき）と `CGO_ENABLED=0` を順に実行する。`make lint` も同じ2構成で回る。両者の終了コードが 0 であることを各 Phase の完了判定条件とする。
- **開発環境が `files` 構成である場合、新しい拒否も新しい警告も手元では現れない。** 次の2つを必ず行う。
  - [x] **拒否側（Phase 2 で実施）**: `useNsswitchVerdict` で完全性判定を「不完全」に固定した状態でのテスト実行。拒否側の経路を実際に踏む（`02_architecture.md` §7.5）。あわせて、そのとき出る文面が CGO 版のもの（`CGO_ENABLED` を含まない）であることを確かめる。**実施状況**: いずれも実施済み。経路の踏破は Phase 2（`CanUserSafelyWriteFile` が `user_database_source=nss` つきで拒否）、文面の確認は Phase 3（同じ経路で出るメッセージが `chmod g-w` を勧め、`CGO_ENABLED` を含まないことを確認）。
  - [ ] **警告側の陽性対照（強制実行。Phase 1 で実施）。** 警告は Phase 1 で有効になるため、この確認も Phase 1 で行う。開発コンテナの `/etc/nsswitch.conf` の `passwd` 行を一時的に `passwd: sss` へ書き換え、`CGO_ENABLED=1` でビルドした `record` を1回起動して、`This build cannot enumerate every member of a group on this host` が `user_database_source=nss` つきで**1度だけ**出ることを目視する。同じ手順を `CGO_ENABLED=0` のビルドでも行い、`user_database_source=passwd-file` になることを確かめる。確認後は `/etc/nsswitch.conf` を必ず元に戻す。**この手順を省くと、警告が出る側の経路はどのテストでも踏まれない**——`TestNsswitchVerdictReportsWhatItSettled` は分類が「完全」になるホストでは「記録が無いこと」しか確かめず、`TestNSSCompletenessReporter_Report`・`TestNSSCompletenessReporter_ReportsOnlyOnce` はレポータを直接呼ぶため確定からの連結を通らないためである（Phase 1 の注記）。
- `make deadcode` を Phase 1 と Phase 4 の完了時に実行し、`nsswitch.go` のタグ除去と公開の入口の追加によって到達不能コードが生じていないことを確かめる。

### 5.6 後方互換性の確認

- 公開 API `GetGroupMembers` のシグネチャと意味は変えない（AC-18）。`internal/runner/base/security`・`internal/safefileio` の production コードは無変更であり、§3.1 の `AC-18` のコマンドで差分が無いことを確かめる。
- 「完全」と判定される環境における `CanUserSafelyWriteFile` の結果は変わらない（AC-19）。既存の `manager_test.go` のテーブルテストがこれを固定している。
- 読み取り判定（`IsUserInGroup`・`CanCurrentUserSafelyReadFile`）は完全性を読まない（AC-17）。既存の `TestReadPathIgnoresCompleteness` がこれを固定している。

---

## 6. リスク管理

### 6.1 技術的リスク

| リスク | 影響 | 対策 |
|---|---|---|
| `nsswitch.go` のタグ除去により、CGO ビルドで `unused` や `deadcode` が新たに何かを報告する | Phase 1 で `make lint`・`make deadcode` が落ちる | **Phase 1 の時点で解消済みである。** 0168 がタグを付けていた理由は「CGO ビルドに `nsswitchVerdict` を呼ぶ production コードが無いこと」だが、Phase 1 で `precomputeEnumerationEnvironment` の定義が `nsswitch.go` の1つに揃うと、`cmd/record`・`cmd/verify` → `EnsurePermissionCheckUID`（`manager.go`、タグ無し）→ `precomputeEnumerationEnvironment` → `nsswitchVerdict` → 分類器一式、という到達経路が CGO ビルドにも成立する。したがって Phase 1 単独で lint・deadcode の完了判定条件を満たせる。満たせない場合は移設が未完（`membership_cgo.go` の空実装が残っている等）であり、先へ進まずそこを直す |
| `TestIncompleteEnumerationErrorMessage` の分割漏れ | Phase 3 で CGO ビルドの `make test` が落ちる | §1.3(b) で特定済み。Phase 3 の作業項目に明示的に含めた。分割後、`rg -c 'CGO_ENABLED' internal/groupmembership/manager_test.go` が一致無しになることで確かめる |
| 開発環境が `files` 構成のため、新しい拒否経路が手元で踏まれない | 実装の誤りが CI やユーザ環境まで露見しない | §5.5 のとおり `useNsswitchVerdict` で「不完全」を固定した実行を必須とする。AC-21 の無効化確認も同じ役割を果たす |
| `identitymutationguard` の `Options.Extra` が期待どおりに `run` 内の呼び出しを拾わない | AC-31 の順序テストが空振りする | 既存の `onlyCallSite` は一致が1件でないと失敗するため、空振りはテストの失敗として現れる。加えて順序を入れ替えたソースに対する control サブテストを置き、走査が実際に順序を見ていることを確かめる |
| glibc の既定構成の確認結果が `02_architecture.md` §3.2.1 の記述と食い違う | AC-03 の前提が崩れる | Phase 2 で確認を行い、食い違った場合はその時点で AC-03 の改訂を提案し、レビューを経てから先へ進む。実装を先に進めない |
| CGO ビルドの新しい文面が長く、既存のログ整形と衝突する | 出力が読みにくくなる | 文面は `fmt.Errorf` の書式に載るだけであり、`slog` の属性には入らない。既存の非 CGO 版の文面も同程度の長さであり、新たな制約は生じない |

### 6.2 スケジュール上のリスク

| リスク | 影響 | 対策 |
|---|---|---|
| Phase 5 の `/mktrans` が Phase 3 の文面確定を待つ | 文書作業を先行させられない | M5 の完了の定義を M3 の後に置いた。文面の転記を伴わない部分（AC-30 の `netgroup` の記述、`98_remaining_issues.md` の更新）は先行して**書き始められる**が、PR は分割せず PR-4 に一括して載せる。文書の変更を2つの PR に分けると、同一リリース内で利用者向け文書と `CHANGELOG` の記述が食い違う窓ができるためである |
| glibc 既定構成の一次情報の確認に時間がかかる | Phase 2 が止まる | 確認対象は upstream の `nss/nss_database.c` の既定テーブル1箇所であり、範囲は限定されている。結果が §3.2.1 の記述と一致する限り、追加の作業は §3.2.1 への追記のみである |

---

## 7. 実装チェックリスト

- [ ] **PR-1** マージ済み（対象ステップ: Phase 1 — 分類の共有化。AC-08, AC-09, AC-10, AC-15, AC-16, AC-07 の一部）
- [ ] **PR-2** マージ済み（対象ステップ: Phase 2 / Phase 3 — CGO 版の完全性申告と拒否メッセージのビルド別化。AC-01〜AC-06, AC-07 の残り, AC-11〜AC-14）
- [ ] **PR-3** マージ済み（対象ステップ: Phase 4 — 起動時の警告の `cmd/runner` への配線。AC-31）
- [ ] **PR-4** マージ済み（対象ステップ: Phase 5 — 文書と残件一覧。AC-24〜AC-30）
- [ ] **全体**: §3 の受け入れ基準検証表の全行が期待どおりの結果になる
- [ ] **全体**: §5.3 の `X1`〜`X9` の無効化確認を、各行の「実施する Phase」に従って実施し、`disabled the ...` を含む英語のコミットメッセージに記した（件数の確認は PR-1〜PR-3 の各ブランチ上で行う。マージ後の `main..HEAD` では数えられない）
- [ ] **全体**: AC-23 の `make test`・`make lint` が2構成で通過した
- [ ] **全体**: §5.5 の強制実行を両ビルドで実施した（警告側は PR-1、拒否側は PR-2）

---

## 8. 横断検索チェックリスト

`make lint` と `make test` が検出できない項目に限る。§3 の受け入れ基準検証表に既にあるコマンドは重複させない。

- [ ] `rg -n 'files alone|this build cannot consult|this build can enumerate exhaustively|this build cannot resolve' internal/groupmembership/nsswitch.go internal/groupmembership/completeness.go` が一致無し（共有されることで誤りになる doc コメントの言い回しが残っていない。現状は `nsswitch.go` に5行・`completeness.go` に1行が一致し、Phase 1 が是正する5箇所すべてを覆う）。**選択肢を `files alone` と `this build cannot consult` の2つに縮めない**——それでは `classifyNSSSources`（`this build can enumerate exhaustively`）と `completeNSSSources` の末尾（`this build cannot resolve`）を取りこぼし、是正漏れを検出できない。**日本語の候補も混ぜない**——対象は production の Go ファイルであり英語しか書かれないため、日本語の枝は永久に一致せず、読み手には「日本語が入りうる」という誤った示唆になる
- [ ] `rg -n 'cacheMutex -> pwentMutex' internal/groupmembership/` が一致無し（ロック順序の注記が更新されている）
- [ ] `rg -n 'the cgo build has no classification to settle' internal/groupmembership/` が一致無し（移設したテストの doc コメントから、事実でなくなった1文が消えている）
- [ ] `rg -n 'resetNsswitchClassification' internal/groupmembership/` の一致がすべて `test_helpers.go` の定義か、それを呼ぶテストである（`membership_nocgo_test.go` に定義が残っていない）
- [ ] `rg -n '列挙の完全性|完全性判定|未申告|分類' docs/tasks/0169_groupmembership_cgo_enumeration_completeness/` の用語が `02_architecture.md`「用語」節の定義と一致している（「判定」「完全性判定」「分類」の使い分けが崩れていない）
- [ ] `rg -cF 'evaluated more permissively than it actually is even on a CGO build' docs/user/security-risk-assessment.md` が一致無し（現状 1 件。`/mktrans` の反映漏れの検出）。**日本語の見出し語で英語版を検索しない**——同ファイルの当該箇所は "CGO builds have a known limitation too" であり、`既知の制限` は反映の前後いずれでも一致しないため、検査として働かない

---

## 9. 成功基準

- **機能の完成度**: `01_requirements.md` の AC-01〜AC-31 がすべて実装され、§3 の検証表の全行が期待どおりの結果になる。
- **品質**: `make test` と `make lint` が `CGO_ENABLED=1`・`CGO_ENABLED=0` の双方で通過する（AC-23）。新規追加した各テストが §5.3 の方法で無効化すると失敗する（AC-21）。`make deadcode` が新たな到達不能コードを報告しない（AC-10）。
- **セキュリティ**: SSSD・LDAP 等が構成されたホストで `CGO_ENABLED=1` ビルドを用いた場合、group-writable な保護対象ファイルへの書き込みが、ディレクトリ側のメンバーの有無によらず拒否される。完全性判定を「不完全」に固定した状態での実行により、この経路が実際に踏まれることを確かめる。
- **後方互換性**: 列挙が「完全」と判定される環境における `CanUserSafelyWriteFile`・`IsUserInGroup`・`CanCurrentUserSafelyReadFile` の外部から観測できる挙動が本タスクの前後で変わらない（AC-17〜AC-19）。`internal/runner/base/security`・`internal/safefileio` の production コードは無変更である。
- **運用への案内**: 拒否に遭遇した運用者が、エラーメッセージだけから原因と、そのビルドで実際に採れる回復手段に到達できる。`CGO_ENABLED=1` でビルド済みの利用者が「`CGO_ENABLED=1` でビルドし直せ」と案内されることがない（AC-12）。
- **文書**: 利用者向け文書3点と `CHANGELOG` の日本語版・英語版が更新され、CGO ビルドと非 CGO ビルドの案内が混同されない形になっている。0149 の残件一覧の D1 が更新され、分離した #1071 が残件として記録されている。
- **追跡可能性**: 同じ `/etc/nsswitch.conf` に対する完全性の判定が両ビルドで一致することが、コード（1箇所の分類器）とテスト（ビルドタグを持たない期待値テーブル）の双方から追える。

---

## 10. 次のステップ

- [ ] 本計画書のレビューを受け、status を `approved` へ更新する
- [ ] `approved` 後に Phase 1 から実装へ着手する
- [ ] 実装中は各作業項目のチェックボックスをその都度更新する
- [x] Phase 2 で確認した glibc の既定構成の結果を `02_architecture.md` §3.2.1 へ追記する
- [ ] Phase 5 完了後、`02_architecture.md` §9 が挙げる拡張候補（「完全」と判定した場合の `slog.Debug` 記録、不完全時の列挙の短絡、`initgroups` 行の分類）を別タスクとして検討する
