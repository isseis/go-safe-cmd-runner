# dry-run における未検証成果物の常時 hard fail 化（`-dry-run-fail-unverified` 削除） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-16 |
| Review date | 2026-07-17 |
| Reviewer | isseis |
| Comments | - |

## 0. 本書の位置づけ

本書は [`02_architecture.md`](02_architecture.md)（status: `approved`）で確定した設計を、実行可能な
タスクへ分解した実装計画書である。要件は [`01_requirements.md`](01_requirements.md)
（AC-01〜AC-28、AC-21 は意図的な欠番）を参照する。フェーズ構成は `02_architecture.md` §8
「実装優先順位」の Phase 1〜6 に従う。

F-007（`VerifyEnvironmentFile` の削除、AC-28）は、本計画書の作成に先立ち別コミット
（`b33bb5bc` "feat(0147): remove dead VerifyEnvironmentFile as preparatory cleanup"）で
既に完了している。§1.3 および §7 の AC-28 でその事実を記録し、本書のフェーズには含めない。

## 1. 実装概要

### 1.1 目的

`01_requirements.md` の F-001・F-002・F-003・F-005・F-006（AC-01〜AC-27、AC-21 を除く）を、
`02_architecture.md` の設計に従って実装する。F-004（既存 E2E テストの是正）は独立のフェーズを
持たず、F-001/F-005 に伴うテスト是正として Phase 5 に統合する。主な変更範囲は次の 4 点である
（`02_architecture.md` §1.1 参照）。

1. 改ざん兆候の判定基準を、表示側（`resource/formatter.go`）と終了コード側
   （`resource/dryrun_manager.go`）で共有する 1 つの述語（`internal/verification` の
   `IsTamperingSignal`）へ統一する（F-002）。
2. `-dry-run-fail-unverified` フラグと、それを伝搬する全フィールド・変数を削除し、
   未検証成果物・検証失敗を常時終了コードへ反映する（F-001）。
3. 終了コード判定の入口条件を `UnverifiedFiles` と `Failures` の和集合へ拡張し、
   `verify_files` の検証失敗も終了コードへ反映する（F-005）。
4. `Failures` / `UNVERIFIED` セクションの表示条件から詳細レベルの制約を外し、
   `summary` でも非ゼロ終了の根拠を追跡できるようにする（F-006）。

### 1.2 実装原則

- 各フェーズは独立してグリーンゲート（`_context.md` の "Green gate" 参照:
  `make test && make lint`）を満たした状態でマージする。
- `02_architecture.md` §8 の指示どおり、Phase 2 と Phase 3（終了コード判定の書き換えとフラグ
  削除）は同一 PR 内で完結させる。フラグを残したまま判定だけを変えると、フラグ未指定でも
  常時 hard fail してしまう一時的な不整合状態が生じるためである。
- 既存テストの期待値変更は「新規追加」ではなく「既存テスト更新」として明示的にタスク化する
  （常時 hard fail 化により多くの既存ケースが挙動反転するため、`02_architecture.md` §5.7 の
  一覧を正とする）。
- Go のソースコード（コメント・識別子・文字列リテラル）は英語のみを用いる。

### 1.3 既存コード調査結果

実装着手前にコードベースを調査した結果を示す（`mkplan.md` 手順 5）。以降のフェーズ記述は
この調査結果を前提とする。

#### F-007 は実装済み（事前クリーンアップ、コミット `b33bb5bc`）

- `internal/verification/manager.go` に `VerifyEnvironmentFile` は存在しない
  （`rg -n "VerifyEnvironmentFile" --type go` はリポジトリ全体でヒットなし、確認済み）。
- `internal/verification/manager_test.go` に `TestVerifyEnvironmentFile` は存在しない。
- AC-28 は既に満たされている。本書では新規タスクを設けず、§7 の AC 検証で完了済みとして
  記録する。

#### `internal/verification/types.go`（F-002 の述語追加先）

- `FailureReason`（110-124 行目）は 5 値（`ReasonHashDirNotFound` /
  `ReasonHashFileNotFound` / `ReasonHashMismatch` / `ReasonFileReadError` /
  `ReasonPermissionDenied`）。追加する `IsTamperingSignal` は `ReasonHashMismatch` の
  場合のみ `true` を返せばよい。
- `UnverifiedFileUsage`（159-164 行目）は `Failure *FailureReason`
  （`skipped_no_validator` の場合は `nil`）を持つ。追加する
  `UnverifiedFileUsage.IsTamperingSignal` は `u.Failure != nil && u.Failure.IsTamperingSignal()`
  で表現できる。
- `internal/verification/types_test.go` は存在しない（新規作成）。同パッケージの
  `manager_test.go` はビルドタグなし、`result_collector_test.go` は `//go:build test` ありと
  混在しているが、新設する `types_test.go` は非公開 API に依存しない純粋関数のテストであり
  `test_organization.md` のヘルパーファイル規則の対象外（通常の `_test.go`）である。
  `manager_test.go` の慣習（ビルドタグなし）に合わせる。

#### `internal/runner/resource/formatter.go`（F-002 の委譲先、F-006 の変更対象）

- `formatUnverifiedMarker`（237-246 行目）は `usage.Failure != nil && *usage.Failure ==
  verification.ReasonHashMismatch` を直接判定している。`usage.IsTamperingSignal()` への
  置き換えで出力は変わらない（同じ条件式）。
- `securityRiskForFailureReason`（248-259 行目）は `reason == verification.ReasonHashMismatch`
  を直接判定している。`reason.IsTamperingSignal()` への置き換えで出力は変わらない。
  本関数は `writeFileVerification`（`Failures` セクション、168 行目）と
  `formatUnverifiedMarker` 呼び出し側（192 行目）、および `JSONFormatter`
  （382-419 行目、`newJSONFileVerificationSummary` 経由、482・489 行目）の両方から使われる
  共有関数であり、置き換えは 1 箇所で両方の出力経路に反映される。
- `writeFileVerification`（140-203 行目）は `Failures` セクション（162 行目）と
  `UNVERIFIED` セクション（184 行目）の出力条件にそれぞれ
  `opts.DetailLevel >= DetailLevelDetailed` を含む。F-006 ではこの条件節を削除し、
  スライスが空でないことのみを条件とする。
- `JSONFormatter.FormatResult`（383-419 行目）と `applySummaryFilter`（512-520 行目）は
  `FileVerification` を検出レベルによらず常に出力しており、変更不要（`02_architecture.md`
  §6.3「対象は TextFormatter のみ」の裏付けが取れた）。

#### `internal/runner/resource/dryrun_manager.go`（F-001/F-002/F-005 の中心）

- `failOnVerificationUnavailable`（51 行目）はフィールド。`NewDryRunResourceManagerWithOutput`
  内の初期化（120 行目）で `opts.FailOnVerificationUnavailable` から代入される。
- `previewExitCodeLocked`（451-475 行目）が判定の中心。現状は
  `d.fileVerification.UsedUnverifiedContent` のみを入口条件とし、`Failures` を一切見ない
  （F-005 で解消する差分）。
- `hasTamperingSignal`（499-506 行目）は `u.Failure != nil` のみで判定しており、
  `hash_file_not_found` 等の環境起因も改ざん兆候に含めてしまう（F-002 で是正する差分）。
  新設する `hasFailureTamperingSignal` は同じ形（スライスを走査し述語を適用する非公開関数）で
  `[]verification.FileVerificationFailure` を対象に追加する。
- `SetFileVerification`（488-492 行目）・`FinalizeDryRunResults`（811-822 行目）は変更不要
  （F-005 は `previewExitCodeLocked` 内部の判定のみを変更する）。
- doc comment 更新対象: 79-87 行目（`fileVerification` フィールド）、422-440 行目
  （`PreviewExitCode`）、447-450 行目（`previewExitCodeLocked`）。いずれも
  `--dry-run-fail-unverified` および `docs/tasks/0146_security_hardening/02_architecture.md`
  (3.4.3) への言及を含み、§1.4 の新しい優先順位の説明へ書き換える。

#### `internal/runner/resource/types.go`（F-001 のフィールド削除対象）

- `DryRunOptions.FailOnVerificationUnavailable`（91-114 行目、doc comment 含む）を削除する。
- `DryRunExitVerificationUnavailable`（117-137 行目）の doc comment は 126 行目で
  `FailOnVerificationUnavailable` に直接言及し、130 行目で削除対象の
  `docs/tasks/0146_security_hardening/02_architecture.md` (3.4.3) を参照し、134 行目で
  「the opt-in」としてフラグを間接的に指しており、いずれもフラグ前提の記述である。
  §1.4 の優先順位へ書き換える。定数値（`= 3`）自体は変更しない。
- `DryRunResult.PreviewExitCode`（188-215 行目）の doc comment も同様に
  `FailOnVerificationUnavailable` へ 3 箇所言及しており、書き換える。

#### `cmd/runner/main.go`（F-001 のフラグ削除対象）

- `dryRunFailUnverified bool`（47 行目、`var` ブロック内）。
- `flag.BoolVar(&dryRunFailUnverified, "dry-run-fail-unverified", ...)`（78 行目）。
- `FailOnVerificationUnavailable: dryRunFailUnverified,`（413 行目、`DryRunOptions{}` 初期化内）。
- `DryRunOptions.HashDir` フィールド（412 行目、`cmdcommon.DefaultHashDirectory` を設定）は
  本タスクの変更対象ではない。ただし調査の過程で、**`resource.DryRunOptions.HashDir` は
  production コードのどこからも参照されない dead field** であることが判明した
  （`rg -n "\.HashDir\b"` で該当ヒットなし）。`verification.NewManagerForDryRun` は常に
  `cmdcommon.DefaultHashDirectory` を直接参照しており、`DryRunOptions.HashDir` を経由
  しない。この dead field 自体は要件書のスコープ外（F-001〜F-007 のいずれにも該当しない）
  であり、本タスクでは削除しない。Phase 5 の E2E テスト設計（後述）はこの事実を前提とする。

#### `internal/verification/manager.go`（F-001 の doc comment 更新対象）

- 390 行目の doc comment
  （`readAndVerifyFileWithReadFallback` 直上）が `--dry-run-fail-unverified exit code` に
  言及している（`02_architecture.md` は「425 行目付近」としているが実際は 390 行目）。
  ここを削除対象フラグへの言及を含まない記述へ書き換える。

#### `internal/runner/resource/security_test.go`（既存テストの是正対象、F-002/F-005 の追加対象）

- `unverifiedSummaryFromFailureReason` / `unverifiedSummaryNoValidator` /
  `mergeUnverifiedSummaries`（208-236, 378-389 行目）は F-005 のテストでも再利用できる
  （`UnverifiedFiles` 用のサマリ構築ヘルパー）。`Failures` 用の等価なヘルパーが存在しないため
  新設が必要（後述 Step 3-5 参照）。
- `TestDryRun_AnalysisUnavailableDenyPreview`（129-148 行目）は
  `assert.Equal(t, DryRunExitAllow, mgr.PreviewExitCode())` を期待しているが、常時
  hard fail 化後は `DryRunExitVerificationUnavailable` になる（`02_architecture.md` §5.7 の
  表に明示されている 3 件のうちの 1 件）。
- `TestDryRun_VerificationUnavailableExitCode`（150-186 行目）はテーブル駆動で
  `failOnVerif bool` 列を持つ。フラグ列を削除し、"verification unavailable" ケースを
  1 本化する。
- `TestHasTamperingSignal`（238-270 行目）は `hash_file_not_found` を伴うケースで
  `want: true` を期待しているが、F-002 是正後は `false` になる
  （`hash_mismatch` 以外はすべて `false`）。テーブルの `want` 列を全面的に見直す。
- `TestDryRun_UnverifiedContentExitCode`（272-359 行目）はテーブル駆動で `opts
  *DryRunOptions` に `FailOnVerificationUnavailable` を設定するケースを持つ。フラグ列を
  廃止し、常時有効前提の期待値へ更新する。F-005 のケース（`Failures` のみのサマリ）を
  同じテーブルへ追加する（既存の `summary *verification.FileVerificationSummary` フィールドを
  再利用できる）。
- `TestDryRun_SetFileVerificationNilClears`（361-376 行目）は
  `opts := &DryRunOptions{..., FailOnVerificationUnavailable: true}` を使っている。
  フィールド自体が削除されるためコンパイルエラーになる。フィールド参照を除去する。

#### `internal/runner/resource/formatter_test.go`（F-006 の是正対象）

- `TestTextFormatter_FormatResult_WithFileVerification`（874-936 行目）のサブテスト
  `"Summary level hides failures"`（905-915 行目）は `assert.NotContains(t, output,
  "Failures:")` を期待している。F-006 後は表示されるため、サブテスト名と assertion を
  反転する（AC-25）。
- `TestTextFormatter_WriteFileVerification_UnverifiedHiddenAtSummaryLevel`
  （1207-1243 行目）は `UNVERIFIED` セクションが `summary` で非表示であることを検証している。
  F-006 後は表示されるため、テスト名と assertion を反転する（AC-26）。
- AC-27（正常系 `summary` でセクションが現れないこと）を検証する既存テストは存在しない。
  新規テストを追加する。
- `TestTextFormatter_WriteFileVerification_AllSuccess`（939-969 行目）・
  `TestTextFormatter_WriteFileVerification_UnverifiedSection`（1148-1205 行目）・
  `TestTextFormatter_WriteFileVerification_HashMismatchFailureSecurityRiskInFailures`
  （1248 行目〜）はいずれも `DetailLevelDetailed` を使っており、F-006 による回帰はない
  （変更不要、AC-14/AC-24 の裏付け）。

#### `cmd/runner/integration_dryrun_verification_test.go` / `testutil_ldflags_test.go`（F-004/F-005 の是正対象）

- `newGoRunCmd`（`testutil_ldflags_test.go` 27-35 行目）は、テストごとに
  `tu.SafeTempDir(t)` で生成した一時ディレクトリを `-ldflags -X
  ...cmdcommon.DefaultHashDirectory=<tmp>` で埋め込んでバイナリをビルド・実行している。
  すなわち **この E2E テスト群は実システムパス
  （`/usr/local/etc/go-safe-cmd-runner/hashes`）を一切使わず**、テストごとに隔離された
  一時ハッシュディレクトリを使っている。ただし現状の `newGoRunCmd` はこの一時ディレクトリの
  パスを呼び出し元へ返さないため、AC-18（事前にハッシュを記録してから dry-run を実行する）を
  実現するには、ハッシュディレクトリを呼び出し元が指定・参照できる形へ拡張する必要がある。
  `sudo` や実システムパスへの書き込みは不要（`make hash-integration-test` が使う
  `$(SUDOCMD)` 経由のパターンとは無関係）。
- `runDryRunCommand`（30-43 行目）は `require.NoError(t, err, "dry-run should succeed")` を
  含み、非ゼロ終了をテスト失敗として扱う。この assertion を除去し、終了コードの検証を
  呼び出し側へ委ねる必要がある（`02_architecture.md` §7.2 のとおり）。
- `TestDryRunE2E_HashDirectoryNotFound`（45-67 行目）と
  `TestDryRunE2E_HashFilesNotFound`（69-91 行目）はセットアップ・アサーションが完全に
  同一（ハッシュディレクトリもハッシュファイルも用意しない）。前者を削除する（AC-19）。
- `TestDryRunE2E_AllSuccess`（93-118 行目）はハッシュを記録せずに dry-run を実行し
  `exit 0` を期待している。ハッシュを記録していないため、修正後は `skipped_no_validator`
  または `hash_file_not_found` により `exit 3` になり AC-05 の検証になっていない
  （AC-18 で是正）。
- `TestDryRunE2E_JSONOutput`（120-164 行目）は `cmd.Run()` の戻り値へ直接
  `require.NoError`（141-142 行目）しており、`cmd.Run()` は非ゼロ終了で `*exec.ExitError`
  を返す。ハッシュ未記録のままでは修正後に失敗する。テストの主眼は JSON 構造の検証であり
  終了コードではないため、`TestDryRunE2E_AllSuccess` と同様にハッシュを事前記録して
  `exit 0` を維持する形で是正する。
- `TestDryRunE2E_MixedResults`（166-202 行目）は `err := cmd.Run()` は使わず
  `cmd.CombinedOutput()`（187 行目）だが `require.NoError(t, err, "dry-run should succeed
  even with verification failures")`（188 行目）でラップしている。コメント
  「Verify exit code is 0 (dry-run never fails)」（200 行目）は本タスクの主旨と正面から
  矛盾する。ハッシュを記録しない設定のため `hash_file_not_found`（環境起因）に該当し、
  是正後の期待値は `exit 3` である。
- `TestDryRunE2E_NoSideEffects`（204-285 行目）も `require.NoError(t, err, "dry-run
  should succeed")`（246 行目）と `assert.Equal(t, 0, cmd.ProcessState.ExitCode())`
  （284 行目）を持つ。副作用の不在を検証することが目的であり終了コードの成否は主眼では
  ないため、ハッシュ未記録のまま `exit 3`（環境起因）を期待する形へ是正する
  （`cmd.CombinedOutput()` の戻り値に対する `require.NoError` は削除し、代わりに
  出力が得られたことを確認したうえで終了コードを明示的に検証する）。
- AC-01（未定義フラグの拒否）・AC-20（`verify_files` の `hash_mismatch` E2E）を検証する
  既存テストは存在しない。新規テストを追加する。

## 2. 実装ステップ

### Phase 1: F-002 — 改ざん兆候の共有述語を追加し、表示側を委譲する

#### Step 1-1: `IsTamperingSignal` 述語を追加する

**対象ファイル**: `internal/verification/types.go`

- [ ] `FailureReason` に `IsTamperingSignal() bool` メソッドを追加する。
      `ReasonHashMismatch` の場合のみ `true` を返す（`02_architecture.md` §3.1.2 のシグネチャ）。
- [ ] `UnverifiedFileUsage` に `IsTamperingSignal() bool` メソッドを追加する。
      `u.Failure != nil && u.Failure.IsTamperingSignal()` として実装する
      （`skipped_no_validator` は `Failure == nil` のため常に `false`）。

**成功基準**: `make build` が通る（既存呼び出し元は未変更のためコンパイルは通る）。

#### Step 1-2: 述語のユニットテストを新設する

**対象ファイル**: `internal/verification/types_test.go`（新規）

- [ ] `TestFailureReason_IsTamperingSignal` を追加し、`FailureReason` の 5 値すべてに対し
      戻り値を検証する（`ReasonHashMismatch` のみ `true`、他 4 値は `false`）。
- [ ] `TestUnverifiedFileUsage_IsTamperingSignal` を追加し、以下を検証する。
  - `Failure == nil`（`skipped_no_validator` 相当）→ `false`
  - `Failure` が `ReasonHashMismatch` を指す → `true`
  - `Failure` が `ReasonHashMismatch` 以外（例: `ReasonHashFileNotFound`）を指す → `false`
- [ ] ビルドタグは付与しない（`manager_test.go` の慣習に合わせる。§1.3 参照）。

**成功基準**: `go test -tags test ./internal/verification/...` が通る。

#### Step 1-3: 表示側を述語へ委譲する（出力不変）

**対象ファイル**: `internal/runner/resource/formatter.go`

- [ ] `formatUnverifiedMarker`（237-246 行目）の判定式
      `usage.Failure != nil && *usage.Failure == verification.ReasonHashMismatch` を
      `usage.IsTamperingSignal()` に置き換える。
- [ ] `securityRiskForFailureReason`（248-259 行目）の判定式
      `reason == verification.ReasonHashMismatch` を `reason.IsTamperingSignal()` に
      置き換える。

**成功基準**: `internal/runner/resource/formatter_test.go` の既存テストが無変更のまま通る
（AC-14: 表示内容は回帰しない）。

### PR-1 作成ポイント: shared tampering-signal predicate

**対象ステップ**: 1-1 / 1-2 / 1-3

**推奨タイトル**: `feat(0147): add shared IsTamperingSignal predicate`

**レビュー観点**: 述語の分類基準が仕様（`hash_mismatch` のみ）と一致しているか / 表示側の出力が本当に不変か（既存テストが無変更のまま通ること） / `UnverifiedFileUsage.Failure == nil` の nil 安全性

**実装モデル要件**: standard

**判定理由**: 既存の表示側基準をそのまま述語へ移設する単純なリファクタリングであり、frontier トリガー（未確定設計、パネルモード・トリガー、孤立した高リスクステップ）のいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 2+3: F-001 / F-002 / F-005 — 終了コード判定の書き換えとフラグ削除

`02_architecture.md` §8 の指示により、終了コード判定の書き換え（Phase 2）とフラグ削除
（Phase 3）は同一 PR 内で完結させる。

#### Step 2-1: `hasTamperingSignal` を述語へ委譲し、`hasFailureTamperingSignal` を新設する

**対象ファイル**: `internal/runner/resource/dryrun_manager.go`

- [ ] `hasTamperingSignal`（499-506 行目）の内部判定 `u.Failure != nil` を
      `u.IsTamperingSignal()` に置き換える（F-002 の是正。`hash_file_not_found` 等の
      環境起因を改ざん兆候に含めなくなる）。doc comment（494-498 行目）も
      「Hash mismatch, hash file not found, ... are all tampering-signals here」という
      誤った記述を、`hash_mismatch` のみが該当する旨へ書き換える。
- [ ] `hasFailureTamperingSignal(failures []verification.FileVerificationFailure) bool`
      を新設する。各要素の `failure.Reason.IsTamperingSignal()` を走査し、いずれか `true`
      なら `true` を返す（`hasTamperingSignal` と対になる非公開関数、F-005）。

**成功基準**: `make build` が通る（この時点では未使用関数として残るため `make lint` の
`unused` 系警告が出ないよう Step 2-2 と同一コミット内で使用箇所を追加する）。

#### Step 2-2: `previewExitCodeLocked` を書き換える

**対象ファイル**: `internal/runner/resource/dryrun_manager.go`

- [ ] `previewExitCodeLocked`（451-475 行目）を `02_architecture.md` §3.3 の優先順位で
      書き換える。
  1. `d.previewPolicyDeny` が真 → `DryRunExitPolicyDeny`
  2. `d.fileVerification != nil` かつ（`hasTamperingSignal(d.fileVerification.UnverifiedFiles)`
     または `hasFailureTamperingSignal(d.fileVerification.Failures)`）→
     `DryRunExitPolicyDeny`
  3. `d.fileVerification != nil` かつ（`len(d.fileVerification.UnverifiedFiles) > 0` または
     `len(d.fileVerification.Failures) > 0`）→ `DryRunExitVerificationUnavailable`
  4. `d.previewVerificationUnavailable` が真 → `DryRunExitVerificationUnavailable`
  5. 上記以外 → `DryRunExitAllow`
- [ ] 判定は `d.fileVerification.UsedUnverifiedContent` フラグを条件に含めない（入口条件が
      `UnverifiedFiles`/`Failures` の長さそのものに変わるため）。
- [ ] `d.fileVerification == nil` の場合は 1・4・5 のみで判定する契約を維持する
      （`02_architecture.md` §3.3「ファイル検証サマリが不在の場合の契約」、既存の防御的分岐）。
- [ ] `PreviewExitCode`（422-441 行目）と `previewExitCodeLocked`（447-450 行目）の
      doc comment を、フラグ分岐を含まない §1.4 の優先順位の説明へ書き換える。

**成功基準**: この時点でコンパイルは通るが、`failOnVerificationUnavailable` フィールドが
まだ残っているため未使用引数の警告は出ない。Step 2-3 でフィールドを削除する。

**注意**: この時点で `internal/runner/resource/security_test.go` の一部の既存テスト
（`TestHasTamperingSignal` の `hash_file_not_found` ケースなど、分類基準の変更に依存する
ケース）は失敗しうる。Step 3-4 で期待値を是正するまでの一時的な状態であり、Step 5-2 と同様に
同一 PR（PR-2）内で解消する。

#### Step 2-3: `failOnVerificationUnavailable` フィールドを削除する

**対象ファイル**: `internal/runner/resource/dryrun_manager.go`

- [ ] `failOnVerificationUnavailable bool` フィールド（51 行目）を削除する。
- [ ] `NewDryRunResourceManagerWithOutput` 内の代入
      `failOnVerificationUnavailable: opts.FailOnVerificationUnavailable,`（120 行目）を削除する。
- [ ] `fileVerification` フィールドの doc comment（79-87 行目）から
      `--dry-run-fail-unverified` フラグと `docs/tasks/0146_security_hardening/02_architecture.md`
      (3.4.3) への言及を除去し、`Failures`/`UnverifiedFiles` の和集合を判定材料とする旨へ
      書き換える。

**成功基準**: `make build` が通る。`resource.DryRunOptions.FailOnVerificationUnavailable`
フィールド自体は Step 3-1 まで残るため、この時点で代入行だけを削除しても未使用フィールド
としてビルドエラーにはならないことを確認する。Step 2-3 → Step 3-1 という削除順序で
問題ない。

#### Step 3-1: `DryRunOptions.FailOnVerificationUnavailable` を削除する

**対象ファイル**: `internal/runner/resource/types.go`

- [ ] `FailOnVerificationUnavailable bool` フィールドと doc comment（91-114 行目）を削除する。
- [ ] `DryRunExitVerificationUnavailable` の doc comment（125-137 行目）から
      `FailOnVerificationUnavailable` への言及を除去し、§1.4 の優先順位（環境起因の
      未検証成果物・検証失敗、または検証不能による拒否のいずれかで、かつ改ざん兆候を
      伴わない場合に返る）へ書き換える。
- [ ] `DryRunResult.PreviewExitCode` の doc comment（201-214 行目）も同様に書き換える。

**成功基準**: `make build` が通らなくなる箇所（`cmd/runner/main.go` の
`FailOnVerificationUnavailable: dryRunFailUnverified,`）が Step 3-2 で解消されることを
確認する。

#### Step 3-2: `cmd/runner` のフラグを削除する

**対象ファイル**: `cmd/runner/main.go`

- [ ] `dryRunFailUnverified bool` 変数宣言（47 行目）を削除する。
- [ ] `flag.BoolVar(&dryRunFailUnverified, "dry-run-fail-unverified", ...)`（78 行目）を
      削除する。
- [ ] `DryRunOptions{...}` 初期化内の `FailOnVerificationUnavailable: dryRunFailUnverified,`
      （413 行目）を削除する。

**成功基準**: `make build` が通る。

#### Step 3-3: `internal/verification/manager.go` の doc comment を更新する

**対象ファイル**: `internal/verification/manager.go`

- [ ] 390 行目の doc comment
      `"...(text/json output, --dry-run-fail-unverified exit code) can distinguish
      unverified content from successfully verified content."` から
      `--dry-run-fail-unverified` への言及を除去する（例:
      `"...(text/json output, the dry-run preview exit code) can distinguish unverified
      content from successfully verified content."`）。

**成功基準**: `rg -n "dry-run-fail-unverified" internal/verification/manager.go` が
ヒットなしになる。

#### Step 3-4: `internal/runner/resource/security_test.go` を是正する

**対象ファイル**: `internal/runner/resource/security_test.go`

- [ ] `TestDryRun_AnalysisUnavailableDenyPreview`（129-148 行目）の期待値を
      `DryRunExitAllow` から `DryRunExitVerificationUnavailable` へ変更する（AC-04）。
      あわせてコメント（145-146 行目、「By default a verification-unavailable deny is
      reported as a note...」）を常時反映される旨へ書き換える。
- [ ] `TestDryRun_VerificationUnavailableExitCode`（150-186 行目）のテーブルから
      `failOnVerif bool` 列を削除し、「verification unavailable not a failure by default」
      ケースと「verification unavailable escalated to distinct code」ケースを、常時
      `DryRunExitVerificationUnavailable` を期待する 1 ケースへ統合する。
      `opts := &DryRunOptions{DetailLevel: DetailLevelDetailed, FailOnVerificationUnavailable:
      tt.failOnVerif}` から `FailOnVerificationUnavailable` フィールド参照を除去する。
- [ ] `TestHasTamperingSignal`（238-270 行目）のテーブルを是正する。
  - `{"one tampering signal", ..., Failure: &mismatch}, true` は変更なし。
  - `{"environment cause mixed with tampering signal", ...Failure: &notFound}, true` の
    `notFound`（`ReasonHashFileNotFound`）は改ざん兆候ではなくなるため、この行を
    「混在」ケースとして維持するなら `mismatch` を含む要素を追加する必要がある。
    F-002 是正後の意味に合わせてテーブル全体を書き直す。具体的には以下のケースを持たせる。
    - `nil usages` → `false`
    - `empty usages` → `false`
    - `only environment cause`（`Failure: nil` または `Failure: &notFound`）→ `false`
    - `one tampering signal`（`Failure: &mismatch`）→ `true`
    - `environment cause mixed with a genuine tampering signal`
      （`Failure: &notFound` の要素と `Failure: &mismatch` の要素の両方を含む）→ `true`
  - 関数直上の doc comment（238-241 行目）を新しい基準（`hash_mismatch` のみ）へ書き換える。
- [ ] `TestDryRun_UnverifiedContentExitCode`（272-359 行目、`unverifiedSummaryFromFailureReason`
      等のヘルパーは 208-236, 378-389 行目）のテーブルから `opts *DryRunOptions` への
      `FailOnVerificationUnavailable` 設定を除去し、常時有効前提の期待値へ更新する。
  - `"environment cause unverified, flag off"` → 名称を `"environment cause unverified"`
    へ変更し、期待値を `DryRunExitAllow` から `DryRunExitVerificationUnavailable` へ変更する。
  - `"tampering signal unverified, flag off"` → 名称を `"tampering signal unverified"` へ
    変更し、期待値を `DryRunExitAllow` から `DryRunExitPolicyDeny` へ変更する
    （F-002 是正後、`hash_mismatch` は常に exit 1）。
  - `"environment cause unverified, flag on"` ケースを削除する。上記の名称変更後、
    `unverifiedSummaryNoValidator("/etc/app/cfg.toml", "config")` を使うサマリ・
    `RiskLevelLow` の assessment・`DryRunExitVerificationUnavailable` の期待値のいずれも
    `"environment cause unverified"`（旧 `flag off`）と完全に一致するため、`opts` から
    `FailOnVerificationUnavailable: true` を除去するだけでは重複テストが残ってしまう
    （`mkplan2.md` の「不要な重複テストを追加しない」原則に反する）。
  - `"tampering signal unverified, flag on"` ケースも同様の理由で削除する。
    `unverifiedSummaryFromFailureReason("/etc/app/cfg.toml", "config",
    verification.ReasonHashMismatch)` を使うサマリ・`RiskLevelLow` の assessment・
    `DryRunExitPolicyDeny` の期待値のいずれも `"tampering signal unverified"`（旧
    `flag off`）と完全に一致するため重複になる。
  - `"policy deny dominates unverified tampering"` ケースは `RiskLevelHigh` の assessment を
    使っており他のケースと重複しないため維持する。`opts` から
    `FailOnVerificationUnavailable: true` を除去する（期待値 `DryRunExitPolicyDeny` は
    変更不要）。
  - `"mixed unverified, flag on, tampering dominates"` は `opts` から
    `FailOnVerificationUnavailable: true` を除去するだけでは不十分である。このケースは
    `unverifiedSummaryFromFailureReason("/etc/app/tmpl.toml", "template",
    verification.ReasonHashFileNotFound)` を「改ざん」側の要素として使っているが、F-002
    是正後は `ReasonHashFileNotFound` は環境起因（`IsTamperingSignal() == false`）になる
    ため、このサマリは実際には改ざん兆候を一切含まなくなり、期待値
    `DryRunExitPolicyDeny` は誤りになる（是正しなければ Step 3-5 の前提が崩れ、AC-13 を
    実際には検証しないテストになる）。`verification.ReasonHashFileNotFound` を
    `verification.ReasonHashMismatch` へ変更し、「環境起因の要素と真の改ざん兆候の要素が
    混在する場合に改ざん兆候が優先される」という AC-13 の意図を正しく検証する形へ
    修正した上で、`FailOnVerificationUnavailable: true` を除去する。期待値
    `DryRunExitPolicyDeny` は維持する。
- [ ] `TestDryRun_SetFileVerificationNilClears`（361-376 行目）の
      `opts := &DryRunOptions{DetailLevel: DetailLevelDetailed, FailOnVerificationUnavailable:
      true}` から `FailOnVerificationUnavailable: true` を除去する。

**成功基準**: `go test -tags test ./internal/runner/resource/...` がこの時点で通る
（Step 3-5 の新規テスト追加前）。

#### Step 3-5: `Failures` 由来の終了コードのユニットテストを追加する（F-005）

**対象ファイル**: `internal/runner/resource/security_test.go`

- [ ] `failuresOnlySummaryFromReason(path, context string, reason verification.FailureReason)
      *verification.FileVerificationSummary` ヘルパーを追加する。`Failures` に単一要素を持ち
      `UnverifiedFiles` は空、`UsedUnverifiedContent: false` のサマリを返す
      （既存の `unverifiedSummaryFromFailureReason` と対になるヘルパー）。
- [ ] `TestDryRun_UnverifiedContentExitCode` のテーブルへ以下のケースを追加する。
  - `"verify_files hash_mismatch alone"`: `failuresOnlySummaryFromReason(...,
    verification.ReasonHashMismatch)` → 期待値 `DryRunExitPolicyDeny`（AC-20）。
  - `"verify_files hash_file_not_found alone"`: `failuresOnlySummaryFromReason(...,
    verification.ReasonHashFileNotFound)` → 期待値 `DryRunExitVerificationUnavailable`
    （AC-22）。
  - `"verify_files hash_mismatch mixed with environment-cause unverified content"`:
    `Failures` の `hash_mismatch` と `UnverifiedFiles` の `skipped_no_validator` を混在させた
    サマリ（`mergeUnverifiedSummaries` を拡張するか、新規ヘルパーで `Failures` と
    `UnverifiedFiles` の両方を持つサマリを組み立てる）→ 期待値 `DryRunExitPolicyDeny`
    （AC-23、改ざん兆候が環境起因のコードに埋没しないことを検証）。

**成功基準**: `go test -tags test -run TestDryRun_UnverifiedContentExitCode -v
./internal/runner/resource/...` で全ケースが通る。

### PR-2 作成ポイント: exit-code judgment rewrite and flag removal

**対象ステップ**: 2-1 / 2-2 / 2-3 / 3-1 / 3-2 / 3-3 / 3-4 / 3-5

**推奨タイトル**: `feat(0147)!: remove -dry-run-fail-unverified and hard-fail unverified content`

**レビュー観点**: `previewExitCodeLocked` の優先順位が §1.4 の 5 段階と完全に一致するか / フラグ削除とロジック書き換えが同一 PR 内で完結し一時的な不整合状態を生まないか / `TestHasTamperingSignal` 等の既存テストの期待値是正が新しい分類基準を正しく反映しているか / `TestDryRun_UnverifiedContentExitCode` の `Failures` 由来の新規ケースが AC-20・AC-22・AC-23 を正しく検証するか

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` 手順 8 のパネルモード・トリガー「security-gate / migration plan（simultaneous behavior raises and lowers, many test updates）」に一致する。改ざん兆候の分類基準が狭まる（`hash_file_not_found` 等が exit 1 → exit 3 へ緩和）と同時に、未検証成果物・検証失敗の既定挙動が常時 hard fail 化する（exit 0 → 非ゼロへ強化）という相反する変更が同一 PR で同時に発生し、`security_test.go` 全体にわたる既存テストの期待値変更を伴う（§5.2 でも本 PR が最もリスクが高いと明記されている）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 4: F-006 — 非ゼロ終了の根拠を詳細レベルによらず出力する

#### Step 4-1: `writeFileVerification` の表示条件を是正する

**対象ファイル**: `internal/runner/resource/formatter.go`

- [ ] `Failures` セクションの出力条件（162 行目）
      `if len(summary.Failures) > 0 && opts.DetailLevel >= DetailLevelDetailed {` から
      `&& opts.DetailLevel >= DetailLevelDetailed` を削除する。
- [ ] `UNVERIFIED` セクションの出力条件（184 行目）
      `if summary.UsedUnverifiedContent && len(summary.UnverifiedFiles) > 0 &&
      opts.DetailLevel >= DetailLevelDetailed {` から `&& opts.DetailLevel >=
      DetailLevelDetailed` を削除する（`summary.UsedUnverifiedContent` と
      `len(summary.UnverifiedFiles) > 0` の条件は維持する）。
- [ ] 関数直上の doc comment（133-139 行目）に「詳細レベルによらず常に表示する」旨を追記する。

**成功基準**: `make build` が通る。

#### Step 4-2: `formatter_test.go` を是正する

**対象ファイル**: `internal/runner/resource/formatter_test.go`

- [ ] `TestTextFormatter_FormatResult_WithFileVerification` のサブテスト
      `"Summary level hides failures"`（905-915 行目）を
      `"Summary level shows failures"` へ改名し、assertion を反転する。
      `assert.NotContains(t, output, "Failures:")` /
      `assert.NotContains(t, output, "/usr/bin/suspicious")` を、それぞれ
      `assert.Contains(t, output, "Failures:")` /
      `assert.Contains(t, output, "/usr/bin/suspicious")` へ変更する（AC-25）。
- [ ] `TestTextFormatter_WriteFileVerification_UnverifiedHiddenAtSummaryLevel`
      （1207-1243 行目）を `TestTextFormatter_WriteFileVerification_UnverifiedShownAtSummaryLevel`
      へ改名し、関数直上のコメント（1207-1209 行目）と assertion を反転する。
      `assert.NotContains(t, output, "UNVERIFIED (content adopted without successful hash
      verification):")` / `assert.NotContains(t, output, "/etc/app/config.toml")` を、
      それぞれ `assert.Contains` へ変更する（AC-26）。
- [ ] `TestTextFormatter_WriteFileVerification_SummaryLevelEmptySectionsHidden` を新規追加する。
      `Failures` と `UnverifiedFiles` の両方が空の `FileVerificationSummary` を
      `DetailLevelSummary` で整形し、`"Failures:"` と
      `"UNVERIFIED (content adopted without successful hash verification):"` のいずれも
      出力に含まれないことを検証する（AC-27、正常系の簡潔さが回帰しないことの防止テスト）。

**成功基準**: `go test -tags test -run TestTextFormatter_.*FileVerification -v
./internal/runner/resource/...` が全て通る。

### PR-3 作成ポイント: detail-level-independent failure/unverified sections

**対象ステップ**: 4-1 / 4-2

**推奨タイトル**: `feat(0147): show Failures and UNVERIFIED sections regardless of detail level`

**レビュー観点**: `Failures`/`UNVERIFIED` セクションの出力条件から `DetailLevelDetailed` の制約が正しく除去されているか / 反転したテスト（`"Summary level shows failures"` 等）が意図通りの挙動を検証しているか / AC-27（正常系で空セクションが出ない）の新規テストが両方の非表示条件をカバーしているか

**実装モデル要件**: standard

**判定理由**: 出力条件のブール式から 1 条件を取り除くだけの局所的な変更であり、frontier トリガー（未確定設計、パネルモード・トリガー、孤立した高リスクステップ）のいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 5: F-001 / F-004 / F-005 — E2E テストの是正

#### Step 5-1: `newGoRunCmd` からハッシュディレクトリを取り出せるようにする

**対象ファイル**: `cmd/runner/testutil_ldflags_test.go`

- [ ] `newGoRunCmdWithHashDir(t *testing.T, hashDir string, appArgs ...string) *exec.Cmd`
      を新設する。現行の `newGoRunCmd` の本体（`hashDirLDFlags(hashDir)` の組み立てから
      `exec.Command` の生成まで）をこの関数へ移し、`hashDir` を引数として受け取る形にする。
- [ ] `newGoRunCmd` は `newGoRunCmdWithHashDir(t, tu.SafeTempDir(t), appArgs...)` を呼ぶ
      薄いラッパーへ変更する。既存の呼び出し元（`cmd/runner` パッケージ内の他の
      `*_test.go`）は挙動不変のため修正不要。

**成功基準**: `make build` が通り、既存の `newGoRunCmd` 呼び出し元のテストが無変更のまま通る。

#### Step 5-2: `runDryRunCommand` ヘルパーを是正する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] `runDryRunCommand`（30-43 行目）から `require.NoError(t, err, "dry-run should
      succeed")`（41 行目）を削除する。戻り値のシグネチャ（`(*exec.Cmd, []byte)`）は
      変更しない。呼び出し側で `cmd.ProcessState.ExitCode()` を明示的に検証する形へ統一する。

**成功基準**: この時点では `runDryRunCommand` を呼ぶテストの一部が失敗しうる
（後続ステップで各テストの期待値を是正するため一時的に許容する。同一 PR 内で解消する）。

#### Step 5-3: `TestDryRunE2E_HashDirectoryNotFound` を削除する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] `TestDryRunE2E_HashDirectoryNotFound`（45-67 行目）を削除する（AC-19。
      `TestDryRunE2E_HashFilesNotFound` と完全に重複しており、
      `hash_directory_not_found` の検証は Phase 1/2 のユニットテストが担保する）。

**成功基準**: `rg -n "TestDryRunE2E_HashDirectoryNotFound"` がヒットなしになる。

#### Step 5-4: `TestDryRunE2E_HashFilesNotFound` の期待値を是正する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] ハッシュを記録しない設定のまま、終了コード検証を
      `assert.Equal(t, 0, cmd.ProcessState.ExitCode())`（66-67 行目相当）から
      `assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())`
      へ変更する（`internal/runner/resource` パッケージを import する）。
      `hash_file_not_found` は環境起因のため exit 3 となる。

**成功基準**: `go test -tags test -run TestDryRunE2E_HashFilesNotFound -v ./cmd/runner/...`
が通る。

#### Step 5-5: `TestDryRunE2E_AllSuccess` をハッシュ事前記録に修正する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] テスト内で `hashDir := tu.SafeTempDir(t)` を生成する。
- [ ] `filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})`
      で `Validator` を構築し、`validator.SaveRecord(configFile, false)` で対象の
      `config.toml` のハッシュを記録する（`cmd/runner/integration_security_test.go` の
      既存の使用パターンを踏襲する）。
- [ ] `newGoRunCmdWithHashDir(t, hashDir, "-config", configFile, "-dry-run", "-dry-run-detail",
      "summary", "-dry-run-format", "text")` でコマンドを構築する。
- [ ] 出力に `"Verified: 1"` および `"Failed: 0"` が含まれること、
      `cmd.ProcessState.ExitCode()` が `resource.DryRunExitAllow`（= 0）であることを検証する
      （AC-18・AC-05）。件数は対象ファイル数に応じて実測で確認し、要件書の「Verified: 2」は
      実装時の実際の検証対象ファイル数（config のみか、config + template か）に応じて
      補正する。

**成功基準**: `go test -tags test -run TestDryRunE2E_AllSuccess -v ./cmd/runner/...` が通る。

#### Step 5-6: `TestDryRunE2E_JSONOutput` をハッシュ事前記録に修正する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] Step 5-5 と同じ要領で `hashDir` を生成し、`configFile` のハッシュを事前記録した上で
      `newGoRunCmdWithHashDir` を使うよう変更する（テストの主眼は JSON 構造の検証であり、
      終了コードを 0 に保つことで既存の検証ロジックを変更せずに済ませる）。

**成功基準**: `go test -tags test -run TestDryRunE2E_JSONOutput -v ./cmd/runner/...` が通る。

#### Step 5-7: `TestDryRunE2E_MixedResults` の期待値を是正する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] `require.NoError(t, err, "dry-run should succeed even with verification failures")`
      （188 行目）を削除する。
- [ ] コメント「Verify exit code is 0 (dry-run never fails)」（200 行目）を削除し、
      `assert.Equal(t, 0, cmd.ProcessState.ExitCode())`（201 行目）を
      `assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())`
      へ変更する（ハッシュ未記録のため `hash_file_not_found` による環境起因、exit 3）。

**成功基準**: `go test -tags test -run TestDryRunE2E_MixedResults -v ./cmd/runner/...` が通る。

#### Step 5-8: `TestDryRunE2E_NoSideEffects` の期待値を是正する

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] `output, err := cmd.CombinedOutput()`（242 行目）に続く
      `require.NoError(t, err, "dry-run should succeed")`（246 行目）を削除する
      （非ゼロ終了時、`CombinedOutput()` の `err` は `*exec.ExitError` になるため）。
- [ ] 末尾の `assert.Equal(t, 0, cmd.ProcessState.ExitCode())`（284 行目）を
      `assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())`
      へ変更する（ハッシュ未記録のため exit 3。テストの目的である副作用の不在検証
      （269-281 行目）は変更しない）。

**成功基準**: `go test -tags test -run TestDryRunE2E_NoSideEffects -v ./cmd/runner/...` が通る。

### PR-4 作成ポイント: E2E test helper and existing-test realignment

**対象ステップ**: 5-1 / 5-2 / 5-3 / 5-4 / 5-5 / 5-6 / 5-7 / 5-8

**推奨タイトル**: `test(0147): realign dry-run E2E tests with hard-fail default`

**レビュー観点**: 各 E2E テストの期待終了コードがハッシュ記録有無と §1.4 の分類に対応しているか / `newGoRunCmdWithHashDir` への移行で既存の `newGoRunCmd` 呼び出し元の挙動が変わっていないか / `TestDryRunE2E_HashDirectoryNotFound` の削除が本当に重複排除であり検証漏れを生まないか / Step 5-2 で `runDryRunCommand` の `require.NoError` を外した後、同一 PR 内の Step 5-3〜5-8 で全呼び出し元の期待値是正が漏れなく完了しているか

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` 手順 8 のパネルモード・トリガー「heavy integration-test / CI / external-resource surface」に一致する。8 ステップにわたり実バイナリのビルド・実行、テストごとに隔離された一時ハッシュディレクトリの構築、6 件の既存 E2E シナリオの期待値是正を伴う。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

#### Step 5-9: AC-01 の E2E テストを追加する（未定義フラグの拒否）

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] `TestDryRunE2E_RemovedFlagRejected` を新規追加する。
      `newGoRunCmd(t, "-config", configFile, "-dry-run", "-dry-run-fail-unverified")`
      （設定ファイルは任意の最小構成でよい）を実行し、`cmd.CombinedOutput()` の `err` が
      非 nil であること、`cmd.ProcessState.ExitCode()` が非ゼロであること、出力に
      Go の `flag` パッケージが出す未定義フラグのエラーメッセージ
      （`"flag provided but not defined"`）が含まれることを検証する（AC-01）。

**成功基準**: `go test -tags test -run TestDryRunE2E_RemovedFlagRejected -v ./cmd/runner/...`
が通る。

#### Step 5-10: AC-20 の E2E テストを追加する（`verify_files` の `hash_mismatch`）

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`

- [ ] `TestDryRunE2E_VerifyFilesHashMismatch` を新規追加する。
  - `hashDir := tu.SafeTempDir(t)` を生成する。
  - `global.verify_files`（または `groups[].verify_files`）に列挙する対象ファイル
    （例: `target.txt`）を用意し、初期内容でハッシュを記録した後、内容を書き換えて
    ハッシュ不一致を発生させる。
  - `config.toml` 自体のハッシュは正しく記録し、終了コードが `verify_files` の改ざんのみに
    起因することを明確にする（要件書 F-005「影響範囲」の記述どおり）。
  - `newGoRunCmdWithHashDir` で dry-run を実行し、`cmd.ProcessState.ExitCode()` が
    `resource.DryRunExitPolicyDeny`（= 1）であることを検証する（AC-20）。

**成功基準**: `go test -tags test -run TestDryRunE2E_VerifyFilesHashMismatch -v
./cmd/runner/...` が通る。

### PR-5 作成ポイント: new AC-01/AC-20 E2E coverage

**対象ステップ**: 5-9 / 5-10

**推奨タイトル**: `test(0147): add E2E coverage for removed-flag rejection and verify_files tampering`

**レビュー観点**: AC-01 の新規 E2E が実際に「未定義フラグ」エラーメッセージ・非ゼロ終了を検証しているか / AC-20 の新規 E2E が `config.toml` 自体は正しく検証させたうえで `verify_files` のみの改ざんを再現できているか / PR-4 で導入した `newGoRunCmdWithHashDir` を正しく再利用しているか

**実装モデル要件**: frontier-required

**判定理由**: `mkplan.md` 手順 8 のパネルモード・トリガー「heavy integration-test / CI / external-resource surface」に一致する。実バイナリのビルド・実行と一時ハッシュディレクトリの構築を伴う新規 E2E シナリオの追加である。PR-4 から分離したのは、既存テストの期待値是正（PR-4）と新規カバレッジの追加（本 PR）が `newGoRunCmdWithHashDir` 共有以外に依存関係を持たず、それぞれ独立にレビュー可能なため（Small over large 原則）。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

### Phase 6: F-003 / F-005 / F-006 — ユーザードキュメントと用語集の更新

#### Step 6-1: `docs/user/runner_command.md` を更新する

**対象ファイル**: `docs/user/runner_command.md`

- [ ] `#### \`-dry-run-fail-unverified\``（650 行目）節を削除し、同じ位置（§3.2
      「Execution Mode Control」内、`-dry-run` 節の直後）へ、フラグ非依存の新しい節
      （見出し例: `#### Dry-run Exit Codes and Unverified Content`）を追加する。
- [ ] 新節には以下を含める。
  - dry-run の終了コード表（`0` / `1` / `3`）を、フラグ分岐のない記述へ更新する。
    `1` は「ポリシー拒否、または `hash_mismatch` を含む未検証成果物・検証失敗」、
    `3` は「`hash_mismatch` を伴わない未検証成果物・検証失敗、または検証不能による拒否」と
    説明する（F-002/F-005 の分類軸）。
  - `verify_files` の検証失敗も終了コードへ反映される旨（F-005）。
  - `-dry-run-detail summary` でも `Failures`/`UNVERIFIED` セクションが表示される旨
    （F-006）。
  - 破壊的変更である旨と移行方法（AC-16）を明記する。具体的には、
    「`-dry-run-fail-unverified` を指定していた既存の呼び出しは、フラグを除去すれば
    同一の挙動になる」ことと、「フラグを指定していなかった呼び出しも、ハッシュ DB が
    未整備であれば exit 0 から exit 3 へ変わりうる」ことの両方を記載する
    （`02_architecture.md` §10「影響範囲に関する注記」）。
  - 運用規則（`02_architecture.md` §5.4）: dry-run の非ゼロ終了はすべて失敗として扱うこと、
    exit 3 を成功として許容しないこと。
- [ ] 節内のコマンド例（旧 680, 703 行目付近）から `-dry-run-fail-unverified` を除去する。
- [ ] 追記した終了コード表の `0`/`1`/`3` の説明を、`02_architecture.md` §1.4 の分類表・
      優先順位（1. ポリシー拒否 → 1、2. 改ざん兆候 → 1、3. 環境起因のみ → 3、
      4. 検証不能による拒否 → 3、5. 上記以外 → 0）と 1 行ずつ突き合わせ、`rg` では
      検出できない意味的な誤り（例: 1 と 3 の説明の取り違え）が無いことを手動で確認する
      （AC-16 は文言の存在確認だけでなく内容の正しさを担保する）。

**成功基準**: `rg -n "dry-run-fail-unverified" docs/user/runner_command.md` がヒットなしに
なり、かつ上記の手動突き合わせで誤りが見つからない。

#### Step 6-2: `docs/user/runner_command.ja.md` を更新する

**対象ファイル**: `docs/user/runner_command.ja.md`

- [ ] Step 6-1 と同一の変更を日本語版へ適用する（`#### \`-dry-run-fail-unverified\``
      節、650 行目、を同じ位置の新節へ置き換える）。章構成・記述内容が英語版と対応するよう
      維持する（AC-17）。
- [ ] Step 6-1 と同様に、追記した終了コード表の説明を `02_architecture.md` §1.4 と
      1 行ずつ突き合わせて手動確認する。加えて、英語版（Step 6-1）と日本語版の該当節を
      左右に並べて読み比べ、章構成だけでなく記述内容（終了コード表・移行方法・運用規則の
      三点）が実質的に対応していることを確認する（AC-17）。

**成功基準**: `rg -n "dry-run-fail-unverified" docs/user/runner_command.ja.md` がヒットなしに
なり、かつ上記の手動突き合わせ・読み比べで齟齬が見つからない。

#### Step 6-3: `docs/translation_glossary.md` に新規用語を追加する

**対象ファイル**: `docs/translation_glossary.md`

- [ ] `### T` セクション（533 行目「改ざん」の付近）に
      `| 改ざん兆候 | tampering signal | hash_mismatch のみが該当する、検証を試行し不整合を
      検出した状態。環境起因と対比される |` を追加する。
- [ ] `### U` セクション（562 行目「未検証」の付近）に
      `| 未検証成果物 | unverified artifact | ハッシュ検証に成功しないまま dry-run
      プレビューが内容を採用した設定／テンプレートファイル。UnverifiedFileUsage に対応 |`
      を追加する。
- [ ] 更新履歴（733 行目以降）に本タスクの用語追加行を追記する
      （例: `| 2026-07-16 | dry-run 常時 hard fail 化（Task 0147）関連の用語を追加
      (tampering signal, unverified artifact) |`）。733 行目の既存の変更履歴行
      （`-dry-run-fail-unverified` フラグ関連の用語追加）はそのまま履歴として残す。

**成功基準**: `rg -n "改ざん兆候|未検証成果物" docs/translation_glossary.md` が §6-3 で
追加した行にヒットする。

### PR-6 作成ポイント: user documentation and glossary updates

**対象ステップ**: 6-1 / 6-2 / 6-3

**推奨タイトル**: `docs(0147): update runner_command docs and glossary for hard-fail default`

**レビュー観点**: 終了コード表（0/1/3）の説明が §1.4 の優先順位と 1 行ずつ一致しているか / 破壊的変更の移行方法がフラグ指定あり・なし双方のケースを網羅しているか / 日英版の章構成・記述内容が対応しているか（AC-17） / 用語集の新規用語（改ざん兆候・未検証成果物）が正しく追加されているか

**実装モデル要件**: standard

**判定理由**: ドキュメントの記述追加・置換のみで実装ロジックの変更を伴わず、frontier トリガー（未確定設計、パネルモード・トリガー、孤立した高リスクステップ）のいずれにも該当しない。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

| マイルストーン | 内容 | 対応 PR |
|---|---|---|
| M1 | 改ざん兆候の共有述語が表示側・終了コード側の両方から参照可能になる（出力は不変） | PR-1 |
| M2 | `-dry-run-fail-unverified` が削除され、未検証成果物・検証失敗（`verify_files` を含む）が常時終了コードへ反映される | PR-2 |
| M3 | `summary` 出力でも非ゼロ終了の根拠が追跡できる | PR-3 |
| M4 | 既存 E2E テストが新しい挙動を正しく検証する | PR-4 |
| M5 | AC-01・AC-20 の新規 E2E カバレッジが揃う | PR-5 |
| M6 | ユーザードキュメント・用語集が新しい挙動と整合する | PR-6 |

Phase 1（M1）を先に完了させることで、表示側の出力が不変であることを確認したうえで
終了コード側の判定を寄せられる（`02_architecture.md` §8）。Phase 2/3（M2）は同一 PR 内で
完結させる（フラグ削除と判定書き換えを分離すると一時的な不整合が生じるため）。PR-3（M3）は
`previewExitCodeLocked` の判定ロジックに依存せず、フォーマッタの表示条件のみを変更するため
PR-2 と並行して着手できる（クリティカルパス上の直列化は不要）。ただし PR-4・PR-5（M4・M5）は
E2E テストの期待終了コードが PR-2 の新しい判定ロジックに直接依存するため PR-2 の完了後に
着手する。PR-6（M6）はドキュメント中の終了コード表が §1.4 の優先順位を記述するため、
PR-2 の完了後に着手する。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 / 1-3 | 改ざん兆候の共有述語 `IsTamperingSignal` を追加し、表示側（`formatter.go`）を委譲する（出力不変） | standard |
| PR-2 | 2-1 / 2-2 / 2-3 / 3-1 / 3-2 / 3-3 / 3-4 / 3-5 | `previewExitCodeLocked` を新しい優先順位へ書き換え、`-dry-run-fail-unverified` フラグと関連フィールドを削除し、既存ユニットテストの期待値を是正する | frontier-required |
| PR-3 | 4-1 / 4-2 | `Failures`/`UNVERIFIED` セクションの出力条件から詳細レベルの制約を除去し、`summary` でも表示されるようにする | standard |
| PR-4 | 5-1 / 5-2 / 5-3 / 5-4 / 5-5 / 5-6 / 5-7 / 5-8 | E2E テストヘルパーを整備し、既存 E2E テストケースを新しい終了コード挙動へ是正する | frontier-required |
| PR-5 | 5-9 / 5-10 | AC-01・AC-20 の新規 E2E テストを追加する | frontier-required |
| PR-6 | 6-1 / 6-2 / 6-3 | ユーザードキュメント（日英）と用語集を新しい挙動へ更新する | standard |

## 4. テスト戦略

`02_architecture.md` §7 を実装可能な粒度へ具体化したものが本書 §2 の各 Step である。
新規追加・変更するテストの一覧は以下のとおり（詳細は §2 の各 Step および §7 の AC 検証を
参照。ここでは重複を避けるため要点のみ記す）。

- **ユニットテスト（述語）**: `internal/verification/types_test.go`（新規、Step 1-2）。
- **ユニットテスト（終了コード）**: `internal/runner/resource/security_test.go`
  （既存テスト是正 Step 3-4、新規ケース追加 Step 3-5）。
- **ユニットテスト（表示の詳細レベル非依存化）**: `internal/runner/resource/formatter_test.go`
  （既存テスト是正・新規テスト追加 Step 4-2）。
- **E2E テスト**: `cmd/runner/integration_dryrun_verification_test.go`
  （既存テスト是正 Step 5-3〜5-8、新規テスト追加 Step 5-9〜5-10）。
- **静的検証**: `docs/user/runner_command.md` / `.ja.md` / `docs/translation_glossary.md`
  への `rg` 検証（Step 6-1〜6-3、および §8 の横断検索チェックリスト）。
- **回帰テスト**: AC-06（非 dry-run 経路の不変性）は専用テストを追加せず、`make test` に
  よる既存の通常実行系テストスイート全体が無変更のまま通ることをもって担保する
  （`02_architecture.md` §7.4）。

## 5. リスク管理

### 5.1 技術リスク

- **リスク**: `TestDryRunE2E_AllSuccess`/`TestDryRunE2E_JSONOutput`（Step 5-5・5-6）で
  ハッシュを事前記録する検証対象ファイル数が、要件書が言及する「Verified: 2」と一致しない
  可能性がある（config ファイルのみか、config + template かは実装時のサンプル設定次第）。
  - **軽減策**: Step 5-5 で実測してから assertion の期待件数を確定する。件数の断定は
    テストコード内の実測値に委ね、本書では「対象ファイルすべてのハッシュを記録し
    Failed: 0 で exit 0 になること」を成功基準とする。
- **リスク**: Phase 2+3（PR-2）はフラグ削除と判定書き換えを同一 PR に含むため、変更範囲が
  大きく、レビューが長引く可能性がある。
  - **軽減策**: Step を 2-1〜2-3（判定側）と 3-1〜3-5（フラグ側・テスト側）に細分しており、
    コミット単位でのレビューは可能。PR 分割はしない（`02_architecture.md` §8 の明示的指示）。
- **リスク**: `internal/runner/resource/security_test.go` の `TestHasTamperingSignal`
  （Step 3-4）は既存のテーブルの `want` 列を大きく書き直すため、既存の正常系ケース
  （`hash_mismatch` のみ `true`）を誤って崩す可能性がある。
  - **軽減策**: Step 1-2 で追加する `types_test.go` の述語テストが同じ分類基準を独立に
    保証するため、`hasTamperingSignal`（`resource` パッケージ側）の意図せぬ劣化は
    `go test ./...` の他のケース（`TestDryRun_UnverifiedContentExitCode` 等）でも検出される。

### 5.2 スケジュールリスク

- 本タスクは 6 PR（PR-1〜PR-6）に分割されており、各 PR は独立してグリーンゲートを
  満たした状態でマージできる粒度に設計されている。PR-2 が最もリスクが高く見積もりの
  不確実性が大きいため、PR-2 着手前に PR-1 のレビューフィードバックを踏まえて見積もりを
  再確認する。

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3）
- [ ] PR-2 マージ済み（対象ステップ: 2-1 / 2-2 / 2-3 / 3-1 / 3-2 / 3-3 / 3-4 / 3-5）
- [ ] PR-3 マージ済み（対象ステップ: 4-1 / 4-2）
- [ ] PR-4 マージ済み（対象ステップ: 5-1 〜 5-8）
- [ ] PR-5 マージ済み（対象ステップ: 5-9 / 5-10）
- [ ] PR-6 マージ済み（対象ステップ: 6-1 / 6-2 / 6-3）
- [ ] 全 PR で `make fmt && make test && make lint` がグリーン
- [ ] 本書「7. 受け入れ基準の検証」の全 AC（AC-01〜AC-28、AC-21 を除く）が検証済み

### 6.1 成功基準（総合）

`requirements_process.md` が求める "Success Criteria"（機能完全性・品質指標・セキュリティ
検証・ドキュメント完全性）を、本書では上記チェックリストと §7 の AC 検証表が合わせて満たす。

- **機能完全性**: AC-01〜AC-28（AC-21 を除く）が全て §7 の検証（`test`/`static`）を
  通過している。
- **品質**: 全 Phase で `make fmt && make test && make lint` がグリーンであり、
  §1.3・§2 に列挙した既存テストの期待値更新が反映されている。
- **セキュリティ検証**: NFR-01（無効化手段の不在）が §7・§8 の静的検証で確認されている。
  F-001/F-002/F-005 に関わる AC-01〜AC-13・AC-20・AC-22・AC-23 の `test` 種別の検証が
  全てパスしている。
- **ドキュメント完全性**: AC-15〜AC-17 の `static` 検証が完了している。

## 7. 受け入れ基準の検証

各 AC について、検証種別（`test`: 自動テスト / `static`: 静的検証コマンド）を付す。

**AC-01**: `-dry-run-fail-unverified` を指定して `runner` を起動すると、未定義フラグとして
拒否され非ゼロ終了する。
- 種別: `test`
- テスト: `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_RemovedFlagRejected`
  （Step 5-9）

**AC-02**: `DryRunOptions.FailOnVerificationUnavailable`・`dryRunFailUnverified`・
`failOnVerificationUnavailable` がコードベースから削除されている。
- 種別: `static`
- 検証コマンド: `rg -n "FailOnVerificationUnavailable|dryRunFailUnverified|failOnVerificationUnavailable" --type go`
- 期待結果: ヒットなし

**AC-03**: フラグを指定しない `-dry-run` で未検証成果物を 1 件以上採用した場合、非ゼロ終了
コードを返す。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  （Step 3-4。「environment cause unverified」「tampering signal unverified」ケース）

**AC-04**: リスクゲートによる検証不能 deny が発生した場合、`DryRunExitVerificationUnavailable`
（= 3）を返す。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_VerificationUnavailableExitCode`
  （Step 3-4）、`TestDryRun_AnalysisUnavailableDenyPreview`（Step 3-4）

**AC-05**: 全ファイル検証成功かつ全コマンド許可の正常系 dry-run は `DryRunExitAllow`（= 0）を
返し、出力内容も回帰しない。
- 種別: `test`
- テスト: `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_AllSuccess`
  （Step 5-5）

**AC-06**: 非 dry-run 経路の挙動・終了コードは本変更の影響を受けない。
- 種別: `test`
- テスト: 専用テストは追加しない（`02_architecture.md` §7.4）。本タスクが変更する
  `previewExitCodeLocked`・`DryRunOptions`・`cmd/runner` の dry-run 分岐はいずれも
  非 dry-run 実行経路から到達不能であるため、既存の非 dry-run 実行系テストが無変更のまま
  通ることで間接的に担保する。具体的には `internal/runner/runner_test.go::TestRunner_ExecuteGroup`
  ／`TestRunner_ExecuteAll`（実コマンド実行、Execute 系の主要経路）と、
  `cmd/runner/integration_security_test.go` の非 dry-run 実行系テスト
  （`TestSecureExecutionFlow` 等）を `make test` で無変更のまま通すことを確認する。

**AC-07**: `skipped_no_validator` のみを含む未検証成果物は exit 3 を返す。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「environment cause unverified」ケース（Step 3-4）

**AC-08**: `verify_failed_hash_directory_not_found` のみを含む未検証成果物は exit 3 を返す。
- 種別: `test`
- テスト: `internal/verification/types_test.go::TestFailureReason_IsTamperingSignal`
  （Step 1-2、`ReasonHashDirNotFound` → `false` のケース）+
  `internal/runner/resource/security_test.go::TestHasTamperingSignal`（Step 3-4）。
  E2E では再現不能なため（`02_architecture.md` §1.5.1）ユニットテストで担保する。

**AC-09**: `verify_failed_hash_file_not_found` のみを含む未検証成果物は exit 3 を返す。
- 種別: `test`
- テスト: `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_HashFilesNotFound`
  （Step 5-4）+ `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`

**AC-10**: `verify_failed_hash_mismatch` を含む未検証成果物は exit 1 を返す。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「tampering signal unverified」ケース（Step 3-4）

**AC-11**: `verify_failed_file_read_error` または `verify_failed_permission_denied` を含み
`hash_mismatch` を伴わない未検証成果物は exit 3 を返す。
- 種別: `test`
- テスト: `internal/verification/types_test.go::TestFailureReason_IsTamperingSignal`
  （Step 1-2、`ReasonFileReadError`/`ReasonPermissionDenied` → `false` のケース）

**AC-12**: リスクゲートによる policy deny は最優先で exit 1 を返し、他の事象に影響されない。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「policy deny dominates unverified tampering」ケース（既存、Step 3-4 で opts 修正のみ）

**AC-13**: `hash_mismatch` と環境起因理由が混在する場合、exit 1 が優先される。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「mixed unverified, flag on, tampering dominates」ケース（既存、Step 3-4 で
  `ReasonHashFileNotFound` → `ReasonHashMismatch` へ是正した上で `opts` を修正）+
  `TestHasTamperingSignal` の「environment cause mixed with a genuine tampering signal」
  ケース（Step 3-4）

**AC-14**: 分類変更は終了コードのみに影響し、`UNVERIFIED`/`UNVERIFIED-TAMPER` 表示・
`security_risk` 注釈・ログレベルは変更されない。
- 種別: `test`
- テスト: `internal/runner/resource/formatter_test.go` の既存テスト
  （`TestTextFormatter_WriteFileVerification_UnverifiedSection` 等、Step 1-3 で無変更のまま
  通ることを確認）

**AC-15**: 両ユーザードキュメントから `-dry-run-fail-unverified` の記述が削除され、終了コード
表がフラグ非依存の記述へ更新されている。
- 種別: `static`
- 検証コマンド: `rg -n "dry-run-fail-unverified" docs/user/runner_command.md
  docs/user/runner_command.ja.md`
- 期待結果: ヒットなし（Step 6-1・6-2）

**AC-16**: 両ドキュメントに破壊的変更である旨、および既存呼び出しの移行方法が明記されている。
- 種別: `static` + `manual`
- 検証コマンド: `rg -n "breaking change|破壊的変更" docs/user/runner_command.md
  docs/user/runner_command.ja.md`
- 期待結果: Step 6-1・6-2 で追記した記述がヒットする
- 手動確認: Step 6-1・6-2 で追記した終了コード表・移行方法の記述内容を
  `02_architecture.md` §1.4 の分類表・優先順位と 1 行ずつ突き合わせ、`rg` では検出でき
  ない意味的な誤り（`0`/`1`/`3` の説明の取り違え等）が無いことを確認する。

**AC-17**: 日英の章構成・記述内容が対応しており、用語集が整合している。
- 種別: `static` + `manual`
- 検証コマンド: `rg -n "^#### " docs/user/runner_command.md docs/user/runner_command.ja.md`
  で見出し数・出現順が一致することを確認し、`rg -n "改ざん兆候|未検証成果物"
  docs/translation_glossary.md` が Step 6-3 の追加行にヒットすることを確認する
- 手動確認: 英語版と日本語版の該当節を読み比べ、章構成だけでなく記述内容（終了コード表・
  移行方法・運用規則）が実質的に対応していることを確認する。

**AC-18**: `TestDryRunE2E_AllSuccess` が実際にハッシュを事前記録した上で dry-run を実行し、
`Verified` > 0 / `Failed: 0` / exit 0 を検証する。
- 種別: `test`
- テスト: `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_AllSuccess`
  （Step 5-5）

**AC-19**: `TestDryRunE2E_HashDirectoryNotFound` の重複が解消されている。
- 種別: `static`
- 検証コマンド: `rg -n "TestDryRunE2E_HashDirectoryNotFound" --type go`
- 期待結果: ヒットなし（Step 5-3 で削除済み）

**AC-20**: `verify_files` の検証が `hash_mismatch` で失敗した場合、exit 1 を返す。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「verify_files hash_mismatch alone」ケース（Step 3-5）+
  `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_VerifyFilesHashMismatch`
  （Step 5-10）

**AC-22**: `verify_files` の `hash_mismatch` 以外の検証失敗は、`hash_mismatch` を伴わない
場合 exit 3 を返す。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「verify_files hash_file_not_found alone」ケース（Step 3-5）

**AC-23**: `verify_files` の `hash_mismatch` と環境起因の未検証成果物・検証失敗が混在する
場合、exit 1 が優先される。
- 種別: `test`
- テスト: `internal/runner/resource/security_test.go::TestDryRun_UnverifiedContentExitCode`
  の「verify_files hash_mismatch mixed with environment-cause unverified content」ケース
  （Step 3-5）

**AC-24**: 反映は終了コードのみに影響し、`Failures` 表示・`security_risk` 注釈・ログレベルは
変更されない。
- 種別: `test`
- テスト: `internal/runner/resource/formatter_test.go` の既存テスト
  （`TestTextFormatter_WriteFileVerification_HashMismatchFailureSecurityRiskInFailures` 等、
  Step 1-3・4-1 で無変更のまま通ることを確認）

**AC-25**: `-dry-run-detail summary` かつテキスト出力で `Failures` が 1 件以上ある場合、
一覧が標準出力に現れる。
- 種別: `test`
- テスト: `internal/runner/resource/formatter_test.go::TestTextFormatter_FormatResult_WithFileVerification`
  の `"Summary level shows failures"` サブテスト（Step 4-2）

**AC-26**: `-dry-run-detail summary` かつテキスト出力で `UnverifiedFiles` が 1 件以上ある
場合、一覧が標準出力に現れる。
- 種別: `test`
- テスト: `internal/runner/resource/formatter_test.go::TestTextFormatter_WriteFileVerification_UnverifiedShownAtSummaryLevel`
  （Step 4-2）

**AC-27**: `Failures` と `UnverifiedFiles` がともに空の正常系 `summary` テキスト出力には
これらのセクションが現れない。
- 種別: `test`
- テスト: `internal/runner/resource/formatter_test.go::TestTextFormatter_WriteFileVerification_SummaryLevelEmptySectionsHidden`
  （Step 4-2、新規）

**AC-28**: `VerifyEnvironmentFile` と `TestVerifyEnvironmentFile` が削除されている。
- 種別: `static`
- 検証コマンド: `rg -n "VerifyEnvironmentFile" --type go`
- 期待結果: ヒットなし。**コミット `b33bb5bc`（本計画書作成前の事前クリーンアップ）で
  既に完了済み**（§1.3 参照）。本タスクの新規作業は不要。

---

## 8. 横断検索チェックリスト

`make lint`/`make test` では検出できない項目のみを対象とする（AC 検証表と重複する `rg`
コマンドはここに含めない）。

- [ ] `docs/tasks/0146_security_hardening/02_architecture.md` (3.4.3) への参照のうち、
      削除したフラグの exit コード分岐を根拠として引用している箇所が本文コード（`internal/`
      配下）から消えていることを確認する: `rg -n "0146_security_hardening.*3\.4\.3"
      internal/runner/resource/dryrun_manager.go internal/runner/resource/types.go` —
      期待結果: ヒットなし（`internal/runner/resource/formatter.go` の 138 行目付近の
      参照は改ざんマーカーの設計根拠であり F-002 で変更しないため対象外）。
- [ ] `hasTamperingSignal` の doc comment に残る「Hash mismatch, hash file not found,
      file read error, and permission denied are all tampering-signals here」という
      誤った説明が残存していないことを確認する:
      `rg -n "are all tampering-signals" internal/runner/resource/dryrun_manager.go` —
      期待結果: ヒットなし（Step 2-1 で書き換え済み）。

---

## 9. 次のステップ

本計画書のレビューが完了し `approved` になった後、`runplan` の手順に従って Phase 1 から
実装に着手する。各フェーズ完了時にグリーンゲート（`make test && make lint`）を確認し、
本書「6. 実装チェックリスト」を更新する。
