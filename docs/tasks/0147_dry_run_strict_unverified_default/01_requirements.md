# dry-run における未検証成果物の常時 hard fail 化（`-dry-run-fail-unverified` フラグ削除） — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `reviewed` |
| Created | 2026-07-16 |
| Review date | 2026-07-16 |
| Reviewer | Claude Code / Issei Suzuki |
| Comments | コード実測に基づくレビュー実施。影響箇所・影響テストの補完、AC-05 / AC-08 の検証方法の是正を反映。**Q-01〜Q-03 は全件解決済み（§5）**。Q-02 の決定（`file_read_error` / `permission_denied` を環境起因へ変更）により `hash_mismatch` のみが exit 1 となり、F-002 の分類・AC-11・AC-13 を更新。Q-03 の決定により F-004（AC-18 / AC-19）を追加。 |

## 1. 背景と目的

### 背景

タスク 0136（`docs/tasks/0136_runtime_risk_evaluation_enforcement/`）で、dry-run 時に
「この環境では検証できなかった」コマンドを hard fail 扱いにする **オプトインフラグ**
`-dry-run-fail-unverified`（`DryRunOptions.FailOnVerificationUnavailable`）が導入された。
タスク 0146（`docs/tasks/0146_security_hardening/`、F-004）は、フラグ増殖を避けるため
このフラグの対象を「未検証成果物全般（検証に失敗したまま採用された設定／テンプレート
ファイルの内容）」へ拡張し、原因別に終了コードを分離した。

現在の既定挙動（フラグ未指定時）は、未検証成果物が存在しても **注記表示のみで exit 0**
である。この既定の根拠は `docs/tasks/0146_security_hardening/02_architecture.md` §3.4.3 に
以下のように記録されている。

> 既定 exit 0 は「dry-run はプレビューであり実行しない」という前提に基づく。
> 実行経路は非 dry-run で従来どおりフェイルクローズドする。

しかしこの根拠を再検討した結果、既定を exit 0 に据える正当性は乏しいと判断した。

1. **フラグは終了コードのみを変える。** `-dry-run-fail-unverified` は
   `cmd/runner/main.go:407-415` で `DryRunOptions` に渡された後、
   `internal/runner/resource/dryrun_manager.go` の `previewExitCodeLocked()` の分岐に
   しか影響しない。プレビュー本体（コマンド列の展開、ファイル検証、`UNVERIFIED`
   セクションを含むレポート出力）はフラグの有無に関わらず同一である。
2. **したがって「ローカルでプレビューを目視する」用途はフラグ常時有効でも成立する。**
   0136/0146 が既定 exit 0 の根拠として挙げていた「本番ハッシュ DB を持たないローカル
   環境でのプレビュー」「設定ファイル作成中のブートストラップ」は、いずれも人間が出力を
   読む用途であり、終了コードに依存しない。
3. **残る差分は、終了コードだけを見て機械的に成否を判定する自動化のみ。** そこでは
   「未検証成果物がある＝非ゼロ終了」が安全側の既定として望ましい。既定 exit 0 は、
   `UNVERIFIED-TAMPER`（`hash_mismatch`、改ざん兆候）を含む場合ですら CI を緑にしてしまう。

一方で、常時有効化をそのまま適用すると **既存の分類が実態に合わない** 問題が顕在化する。
`hasTamperingSignal()`（`internal/runner/resource/dryrun_manager.go:494-504`）は
`Failure != nil` の全ケースを「改ざん兆候」とみなし exit 1（policy deny）へ倒す。
これはフラグがオプトインである限り影響が限定的だったが、常時有効化すると
**ハッシュ未登録・ハッシュ DB 未整備という環境起因の状況が「ポリシー拒否」として
報告される**ことになり、`skipped_no_validator` のみを環境起因（exit 3）とする現在の
線引きが破綻する。本タスクではこの分類も併せて是正する。

### 目的

1. `-dry-run-fail-unverified` フラグを **削除** し、未検証成果物を採用した dry-run は
   常に非ゼロ終了する挙動を **既定かつ唯一の挙動** にする（F-001）。
2. 未検証理由の **環境起因／改ざん兆候の分離を実態に合わせて再定義** し、常時有効化に
   よってブートストラップ工程が「ポリシー拒否」と誤報されることを防ぐ（F-002）。
3. 上記の破壊的変更を **ユーザードキュメントへ反映** する（F-003）。

### スコープ外

- **dry-run 出力の表示内容の変更**。`UNVERIFIED` / `UNVERIFIED-TAMPER` セクションの
  表示、`security_risk` 注釈（`getSecurityRisk()`、`internal/verification/result_collector.go:189`）、
  ログレベル（`determineLogLevel()`、同 `:176`）は本タスクでは変更しない。本タスクは
  終了コードの決定ロジックに限定する。
- **非 dry-run（実行経路）の挙動変更**。実行経路は従来どおりフェイルクローズドであり、
  本タスクの対象外。
- **リスク評価アルゴリズム自体の変更**。リスクゲートによる policy deny /
  検証不能 deny の判定ロジックは変更しない（終了コードへの写像のみ本タスクの対象）。
- **終了コード体系の再設計**。`0` / `1` / `3` の 3 値体系は維持する。
- **過去タスク文書（0136 / 0146）の改訂**。これらは意思決定の履歴記録であり、
  当時の設計を記述したまま凍結する。本タスクによる上書きは本書および
  `docs/user/` 配下のユーザー文書で行う。

### 前提・依存

- `-dry-run-fail-unverified` は `docs/user/runner_command.md` / `.ja.md` に記載された
  公開フラグである。シェル補完スクリプト等の別経路での参照は存在しない
  （リポジトリ全体の grep で確認済み）。
- 本変更は **破壊的変更（breaking change）** である。既存の CI が当該フラグを
  指定している場合、および未検証成果物がある状態で dry-run の exit 0 に依存している
  場合の双方が影響を受ける。
- 未検証理由（`UnverifiedReason`）は `skipped_no_validator` と
  `verify_failed_<FailureReason>` の 2 形式で、`FailureReason` は 5 値
  （`internal/verification/types.go:110-123`）。
- **ハッシュディレクトリが存在しない場合、runner は起動時に当該ディレクトリを作成する**
  （検証マネージャ初期化時）。実測（`-ldflags` で存在しないハッシュディレクトリを指定し
  dry-run 実行）では、ディレクトリが作成されたうえで各ファイルが
  `verify_failed_hash_file_not_found` となり、`hash_directory_not_found` は発生しなかった。
  `ReasonHashDirNotFound` は `filevalidator.ErrHashDirNotExist`
  （`internal/verification/result_collector.go:162-163`）からのみ生成される。
  この事実は Q-01 および AC-08 の検証方法に影響する（§5 参照）。

## 2. 機能要件

本書の AC は AC-01〜AC-19。

### F-001: `-dry-run-fail-unverified` の削除と strict 挙動の常時有効化

`-dry-run-fail-unverified` フラグおよびそれを伝搬する `DryRunOptions` フィールド・
内部フィールドを削除し、未検証成果物の採用および検証不能 deny を常に非ゼロ終了として
扱う。フラグは互換のための no-op として残さず、完全に削除する（指定された場合は Go の
`flag` パッケージによる未定義フラグエラーで終了する）。

**影響箇所**（リポジトリ全体の grep で確定。doc comment 中の言及も NFR-03 の対象に含む）:

| ファイル | 箇所 | 内容 |
|---|---|---|
| `cmd/runner/main.go` | `:47` | `dryRunFailUnverified` 変数 |
| | `:78` | `flag.BoolVar` 定義 |
| | `:413` | `DryRunOptions` への代入 |
| `internal/runner/resource/types.go` | `:91-114` | `FailOnVerificationUnavailable` フィールドと doc comment |
| | `:126` | `DryRunExitVerificationUnavailable` 定数の doc comment |
| | `:205-212` | `DryRunResult.PreviewExitCode` の doc comment（優先順位の記述） |
| `internal/runner/resource/dryrun_manager.go` | `:51` | `failOnVerificationUnavailable` フィールド宣言 |
| | `:83` | 構造体 doc comment 中のフラグ言及 |
| | `:120` | `opts` から内部フィールドへの伝搬 |
| | `:422-475` | `PreviewExitCode` / `previewExitCodeLocked` の分岐と doc comment |
| | `:494-504` | `hasTamperingSignal()`（F-002 で再定義） |
| `internal/verification/manager.go` | `:425` | doc comment 中の `--dry-run-fail-unverified` 言及 |
| `internal/runner/resource/security_test.go` | `:152`, `:177`, `:272`, `:313-366` | テスト側のフラグ参照（§3 参照） |
| `docs/translation_glossary.md` | `:733` | 変更履歴行（F-003 で扱う） |

**Acceptance Criteria**:
- **AC-01**: `-dry-run-fail-unverified` を指定して `runner` を起動すると、未定義フラグ
  として拒否され非ゼロ終了する。フラグが no-op として黙って受理されることはない。
- **AC-02**: `DryRunOptions.FailOnVerificationUnavailable` フィールド、`cmd/runner` の
  `dryRunFailUnverified` 変数、`DryRunResourceManager` の
  `failOnVerificationUnavailable` フィールドがコードベースから削除されている。
- **AC-03**: フラグを指定しない `-dry-run` において、未検証成果物を 1 件以上採用した
  場合、プレビューは非ゼロ終了コードを返す（原因別のコード値は F-002 が定める）。
- **AC-04**: フラグを指定しない `-dry-run` において、リスクゲートによる検証不能 deny
  （`previewVerificationUnavailable`）が発生した場合、`DryRunExitVerificationUnavailable`
  （= 3）を返す。
- **AC-05**: すべてのファイル検証が成功し、かつすべてのコマンドが許可される正常系の
  dry-run は、従来どおり `DryRunExitAllow`（= 0）を返す。dry-run 出力の内容も回帰しない。
- **AC-06**: 非 dry-run（通常実行）経路の挙動および終了コードは本変更の影響を受けない。

### F-002: 未検証理由の環境起因／改ざん兆候の再分類

`hasTamperingSignal()` が `Failure != nil` を一律に改ざん兆候として扱う現行の分類を
改め、`FailureReason` の値ごとに「環境起因（この環境では検証できない／記録が無い）」と
「改ざん兆候（検証を試みて不整合を検出した）」を区別する。

**分類の定義**:

| `UnverifiedReason` | 区分 | 終了コード | 根拠 |
|---|---|---|---|
| `skipped_no_validator` | 環境起因 | `3` | バリデータ未設定。検証が試行されていない |
| `verify_failed_hash_directory_not_found` | 環境起因 | `3` | ハッシュ DB 自体が未整備。検証手段が存在しない |
| `verify_failed_hash_file_not_found` | 環境起因 | `3` | 当該ファイルのハッシュが未登録。ブートストラップ工程で正常に発生する |
| `verify_failed_hash_mismatch` | 改ざん兆候 | `1` | 記録済みハッシュと実体が不一致。**唯一の真の改ざん兆候** |
| `verify_failed_file_read_error` | 環境起因 | `3` | ファイルを読めない。改ざんの積極的な証拠ではない |
| `verify_failed_permission_denied` | 環境起因 | `3` | 権限不足。実行環境の設定に起因する |

すなわち **`hash_mismatch` のみが exit 1、他の全理由は exit 3** となる。

**設計上の注記 1（分類軸）**: 本表の軸は「**検証を試行し、記録との不整合を実際に
検出したか**」である。`hash_mismatch` だけがこれを満たす。他の 4 理由は
「検証できなかった」に留まり、改ざんの積極的な証拠ではないため環境起因とする。
この軸は `getSecurityRisk()`（`internal/verification/result_collector.go:189-200`）や
`determineLogLevel()`（同 `:176-186`）とは**異なる軸**であり、両者の値が一致しなくても
矛盾ではない。`getSecurityRisk()` は「本番実行時にどの程度危険か」を測り、
`hash_file_not_found` / `file_read_error` / `permission_denied` を medium とするが、
これらは本表では環境起因（exit 3）である。`determineLogLevel()` が
`hash_file_not_found` を ERROR とする理由も「本番実行では失敗するから」
（同 `:181` のコメント）であり、改ざん兆候だからではない。

**設計上の注記 2（表示との一致）**: 本分類により、終了コードの改ざん兆候判定は
表示側の `formatUnverifiedMarker()`（`internal/runner/resource/formatter.go:240-246`）と
**完全に一致する**（いずれも `Failure == ReasonHashMismatch` のみを改ざん兆候とみなす）。
その結果、以下が 1 対 1 で対応する一貫したモデルとなり、利用者は dry-run 出力から
終了コードの理由を読み取れる。

| 表示 | `security_risk` | 終了コード |
|---|---|---|
| `UNVERIFIED-TAMPER`（`hash_mismatch`） | high | `1` |
| `UNVERIFIED`（他の全理由） | medium / low | `3` |

**設計上の注記 3（DRY）**: 上記のとおり `hasTamperingSignal()` と
`formatUnverifiedMarker()` は同一の述語を持つことになる。両者は同一パッケージ
（`internal/runner/resource`）にあるため、`UnverifiedFileUsage` 1 件を判定する
非公開ヘルパー（例: `isTamperingSignal(usage)`）へ抽出し、両者から呼び出すこと。
判定が二重定義され将来乖離することを防ぐ（CLAUDE.md の DRY 方針）。これは
リファクタリングであり、表示の挙動は変えない（AC-14 と両立する）。

**AC-14（表示への非波及）の根拠**: `hasTamperingSignal()` の現在の呼び出し元は
`previewExitCodeLocked()` のみである（grep で確認済み）。表示は独立した
`formatUnverifiedMarker()` が決定しており、その判定条件は本タスクで変更しない。
したがって終了コード分類の変更は表示へ波及しない。

**Acceptance Criteria**:
- **AC-07**: `skipped_no_validator` のみを含む未検証成果物は
  `DryRunExitVerificationUnavailable`（= 3）を返す。
- **AC-08**: `verify_failed_hash_directory_not_found` のみを含む未検証成果物は
  `DryRunExitVerificationUnavailable`（= 3）を返す。
- **AC-09**: `verify_failed_hash_file_not_found` のみを含む未検証成果物は
  `DryRunExitVerificationUnavailable`（= 3）を返す。
- **AC-10**: `verify_failed_hash_mismatch` を含む未検証成果物は
  `DryRunExitPolicyDeny`（= 1）を返す。
- **AC-11**: `verify_failed_file_read_error` または `verify_failed_permission_denied` を
  含む未検証成果物は `DryRunExitVerificationUnavailable`（= 3）を返す
  （`hash_mismatch` を伴わない場合）。
- **AC-12**: リスクゲートによる policy deny は最優先で `DryRunExitPolicyDeny`（= 1）を
  返し、未検証成果物や検証不能 deny の有無に影響されない。
- **AC-13**: `hash_mismatch` と環境起因理由（`skipped_no_validator` /
  `hash_directory_not_found` / `hash_file_not_found` / `file_read_error` /
  `permission_denied`）が混在する場合、`DryRunExitPolicyDeny`（= 1）が優先される。
  改ざん兆候が環境起因のコードに埋没しない。
- **AC-14**: 上記の分類変更は終了コードの決定にのみ影響し、dry-run 出力上の
  `UNVERIFIED` / `UNVERIFIED-TAMPER` 表示、`security_risk` 注釈、ログレベルは変更されない。

### F-003: ドキュメント更新

`-dry-run-fail-unverified` の削除と常時 hard fail 化を、ユーザー向けドキュメントへ
反映する。破壊的変更である旨と移行方法を明記する。

**影響箇所**:
- `docs/user/runner_command.md:650-705`（`-dry-run-fail-unverified` 節、終了コード表、
  Use Cases、Usage Examples、Notes）
- `docs/user/runner_command.ja.md:650-705`（同上・日本語版）
- `docs/translation_glossary.md`（当該フラグ由来の用語エントリ）

**Acceptance Criteria**:
- **AC-15**: `docs/user/runner_command.md` および `docs/user/runner_command.ja.md` から
  `-dry-run-fail-unverified` フラグの記述が削除され、dry-run の終了コード表が
  フラグ非依存の記述（`0` / `1` / `3` の意味と F-002 の分類）へ更新されている。
- **AC-16**: 両ドキュメントに、本変更が破壊的変更であること、および当該フラグを
  指定している既存の呼び出しはフラグを除去すれば同一の挙動になることが明記されている。
- **AC-17**: 日本語版と英語版の章構成・記述内容が対応しており、
  `docs/translation_glossary.md` の用語が更新後の記述と整合している。

### F-004: 既存 E2E テストの是正（Q-03 の決定により本タスクで実施）

§3.1 で判明したテスト名と実態の乖離を是正する。これは AC-05 を検証可能にするために
必須であり、かつ現状のテストが「検証成功」を偽って主張している状態を解消する。

**Acceptance Criteria**:
- **AC-18**: `TestDryRunE2E_AllSuccess` が **実際にハッシュを事前記録したうえで**
  dry-run を実行し、`Verified: 2` / `Failed: 0`（未検証成果物なし）となり
  `DryRunExitAllow`（= 0）で終了することを検証する。これにより AC-05 を担保する。
- **AC-19**: `TestDryRunE2E_HashDirectoryNotFound` の乖離が解消されている。同テストは
  `TestDryRunE2E_HashFilesNotFound` とセットアップ・期待が完全に同一であり、かつ
  runner がハッシュディレクトリを自動作成するため E2E で
  `hash_directory_not_found` は再現不能である。したがって**同テストは削除**し、
  `hash_directory_not_found` の検証は AC-08 のユニットテストに一本化する
  （重複テストを残さない＝DRY）。

**実装上の注記**: AC-18 のハッシュ事前記録は、既存の
`filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})`
＋ `SaveRecord()`（`internal/filevalidator/validator.go:245`）で実現できる。
`cmd/runner/integration_security_test.go:461` に同 API の利用例がある。
本要件のレビュー時に、`config.toml` と `/bin/echo` のハッシュを記録した状態で
dry-run が exit 0（`Verified: 2` / `Failed: 0` / ALLOW）となることを実測で確認済み。

## 3. 影響を受ける既存テスト

本変更は既存テストの期待値変更を伴う。**実測（`go run` で E2E テストと同一条件を再現）に
基づき、影響範囲は当初想定より広い**ことが判明した。網羅的な一覧は
`03_implementation_plan.md` で確定させる。

### 3.1 E2E テスト（`cmd/runner/integration_dryrun_verification_test.go`）

`newGoRunCmd()`（`cmd/runner/testutil_ldflags_test.go:27-35`）は **空の一時ディレクトリ**を
既定ハッシュディレクトリとして `-ldflags` 埋め込みする。したがって当該ファイルの
**全 E2E テストがハッシュ未登録状態で動作しており**、実測では各実行が
`verify_failed_hash_file_not_found` の未検証成果物を採用し、かつリスクゲートの
検証不能 deny（`uncertain_unverified_identity`）を伴う。両者とも F-002 では exit 3 に写像
されるため、**同ファイルの exit 0 アサーションは 6 件すべてが exit 3 へ変わる**。

| テスト | 現在の期待 | 変更後の期待 | 対応 AC |
|---|---|---|---|
| `:46` `TestDryRunE2E_HashDirectoryNotFound` | exit 0 | exit 3 | AC-09 / AC-04 |
| `:70` `TestDryRunE2E_HashFilesNotFound` | exit 0 | exit 3 | AC-09 / AC-04 |
| `:94` `TestDryRunE2E_AllSuccess` | exit 0 | exit 3 | AC-09 / AC-04 |
| `:121` `TestDryRunE2E_JSONOutput` | exit 0 | exit 3 | AC-09 / AC-04 |
| `:167` `TestDryRunE2E_MixedResults` | exit 0（「dry-run never fails」コメント付き） | exit 3 | AC-09 / AC-04 |
| `:205` `TestDryRunE2E_NoSideEffects` | exit 0 | exit 3 | AC-09 / AC-04 |
| `:32-41` `runDryRunCommand()` ヘルパー | `require.NoError(t, err, "dry-run should succeed")` | 非ゼロ終了を許容する形へ改修（`:39`） | AC-03 |

**テスト名と実態の乖離**: 実測により以下が判明した。**Q-03 の決定により本タスクで
是正する**（F-004）。

- `TestDryRunE2E_HashDirectoryNotFound` は **ハッシュディレクトリ不在を再現していない**。
  `newGoRunCmd()` が作成する空ディレクトリを使うため、`TestDryRunE2E_HashFilesNotFound`
  と**セットアップも期待も完全に同一**であり、実際に発生する理由は
  `hash_file_not_found` である。→ 削除（AC-19）。
- `TestDryRunE2E_AllSuccess` は **ハッシュを一切記録しておらず**「全検証成功」を
  再現していない（実測: `Verified: 0` / `Failed: 2`）。→ ハッシュ事前記録へ修正（AC-18）。

上表の「変更後の期待」は AC-19 / AC-18 適用**前**の素の挙動を示す。適用後は
`TestDryRunE2E_HashDirectoryNotFound` は消滅し、`TestDryRunE2E_AllSuccess` は
exit 0 のままとなる（ハッシュを記録するため）。

**カバレッジの所在**:

- **AC-05**（全検証成功で exit 0）: 既存 E2E に有効なカバレッジが無いため、
  F-004 / AC-18 で担保する。
- **AC-08**（`hash_directory_not_found` で exit 3）: 前提・依存に記したとおり、runner が
  ハッシュディレクトリを起動時に作成するため **E2E では再現できない**。
  `UnverifiedFileUsage` を合成する**ユニットテストで検証する**。

### 3.2 ユニットテスト（`internal/runner/resource/security_test.go`）

| テスト | 現在の期待 | 変更後の期待 | 対応 AC |
|---|---|---|---|
| `:128-147` `TestDryRun_AnalysisUnavailableDenyPreview` | `DryRunExitAllow`（`:147`） | `DryRunExitVerificationUnavailable` | AC-04 |
| `:153-185` `TestDryRun_VerificationUnavailableExitCode` の `:171` "verification unavailable not a failure by default" | `DryRunExitAllow` | `DryRunExitVerificationUnavailable` | AC-04 |
| 同 `:172` "verification unavailable escalated to distinct code" | `failOnVerif: true` で exit 3 | フラグ軸を削除（既定で exit 3）。`:171` と重複するため統合 | AC-04 |
| `:272-374` `TestDryRun_UnverifiedContentExitCode` | `failOnVerif` によるテーブル駆動 | フラグ軸を削除し F-002 の分類軸（6 理由 × 混在）へ再構成 | AC-07〜AC-13 |

再構成後のテーブルは、`skipped_no_validator` と `FailureReason` 5 値の計 6 ケースに
加え、AC-13 の混在ケース（`hash_mismatch` + 環境起因理由 → exit 1）を含めること。
`hash_directory_not_found` のケースは E2E で再現不能なため、ここで担保する（AC-08）。

## 4. 非機能要件

- **NFR-01**: 本変更はセキュリティ既定値を安全側へ倒すものであり、いかなる設定・
  フラグによっても従来の「未検証でも exit 0」挙動へ戻せてはならない。
- **NFR-02**: `make test` および `make lint` が通ること。
- **NFR-03**: 削除対象のシンボルがコードベースに残存しないこと（デッドコードを残さない）。

## 5. 検討事項（解決済み）

以下は本書レビュー時の確認依頼事項であり、**2026-07-16 に全件解決した**。決定内容は
既に §2（F-002 / F-004）へ反映済みである。

- **Q-01（解決）**: `verify_failed_hash_directory_not_found` の分類。
  **決定: 環境起因（exit 3）とする。**
  経緯: 本理由は要件検討時の選択肢提示から漏れていた
  （`internal/runner/resource/types.go:101-104` の doc comment 自体が 5 値中 4 値しか
  列挙しておらず、それを元に選択肢を作成したため）。
  調査結果: 実測により、runner はハッシュディレクトリが存在しない場合に**これを
  起動時に作成する**ため、dry-run プレビュー経路で `hash_directory_not_found` は
  発生しないことを確認した（§1 前提・依存）。本分類は**到達不能な経路に対する
  防御的な定義**であり、実挙動への影響は無い。AC-08 はユニットテストで担保する
  （§3.2）。
- **Q-02（解決）**: `file_read_error` / `permission_denied` の分類。
  **決定: 環境起因（exit 3）とする**（当初案の「改ざん兆候（exit 1）据え置き」から
  **変更**）。
  影響: これにより **`hash_mismatch` のみが exit 1** となり、分類が大幅に単純化された。
  副次的効果として、当初レビューで指摘した「表示は素の `UNVERIFIED` なのに exit 1」
  という乖離が**解消**され、終了コードが表示（`UNVERIFIED-TAMPER`）および
  `security_risk`（high）と 1 対 1 で対応する一貫したモデルとなった
  （F-002 設計上の注記 2）。さらに `hasTamperingSignal()` と
  `formatUnverifiedMarker()` の述語が一致するため、共通ヘルパーへの抽出が可能に
  なった（同注記 3）。
- **Q-03（解決）**: `TestDryRunE2E_HashDirectoryNotFound` /
  `TestDryRunE2E_AllSuccess` の名称と実態の乖離。
  **決定: 本タスクで修正する。** F-004（AC-18 / AC-19）として要件化した。
