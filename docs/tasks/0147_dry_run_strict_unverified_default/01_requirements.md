# dry-run における未検証成果物の常時 hard fail 化（`-dry-run-fail-unverified` フラグ削除） — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-16 |
| Review date | - |
| Reviewer | - |
| Comments | - |

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
`hasTamperingSignal()`（`internal/runner/resource/dryrun_manager.go:497-504`）は
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

## 2. 機能要件

本書の AC は AC-01〜AC-17。

### F-001: `-dry-run-fail-unverified` の削除と strict 挙動の常時有効化

`-dry-run-fail-unverified` フラグおよびそれを伝搬する `DryRunOptions` フィールド・
内部フィールドを削除し、未検証成果物の採用および検証不能 deny を常に非ゼロ終了として
扱う。フラグは互換のための no-op として残さず、完全に削除する（指定された場合は Go の
`flag` パッケージによる未定義フラグエラーで終了する）。

**影響箇所**:
- `cmd/runner/main.go:47`（`dryRunFailUnverified` 変数）、`:78`（`flag.BoolVar`）、
  `:413`（`DryRunOptions` への代入）
- `internal/runner/resource/types.go:91-114`（`FailOnVerificationUnavailable` フィールドと doc comment）
- `internal/runner/resource/dryrun_manager.go:120`（内部フィールドへの伝搬）、
  `:422-475`（`PreviewExitCode` / `previewExitCodeLocked` の分岐と doc comment）

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
| `verify_failed_hash_mismatch` | 改ざん兆候 | `1` | 記録済みハッシュと実体が不一致。真の改ざん兆候 |
| `verify_failed_file_read_error` | 改ざん兆候 | `1` | 検証を試行して失敗。環境起因と断定できない |
| `verify_failed_permission_denied` | 改ざん兆候 | `1` | 検証を試行して失敗。環境起因と断定できない |

**設計上の注記**: 本分類は既存の `getSecurityRisk()`
（`internal/verification/result_collector.go:189-200`）の重み付け（`hash_mismatch`=high、
`hash_directory_not_found`=low）と整合する。ただし `getSecurityRisk()` は
`hash_file_not_found` を medium とする一方、本表では環境起因（exit 3）に分類する。
これは両者が異なる軸を測っているためである。`getSecurityRisk()` は
「本番実行時にどの程度危険か」を、本表は「この環境で検証できたか否か」を表す。
`determineLogLevel()` が `hash_file_not_found` を ERROR とする理由も
「本番実行では失敗するから」（同 `:181` のコメント）であり、改ざん兆候だからではない。

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
  含む未検証成果物は `DryRunExitPolicyDeny`（= 1）を返す。
- **AC-12**: リスクゲートによる policy deny は最優先で `DryRunExitPolicyDeny`（= 1）を
  返し、未検証成果物や検証不能 deny の有無に影響されない。
- **AC-13**: 環境起因（exit 3 相当）と改ざん兆候（exit 1 相当）が混在する場合、
  `DryRunExitPolicyDeny`（= 1）が優先される。改ざん兆候が環境起因のコードに埋没しない。
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

## 3. 影響を受ける既存テスト

本変更は既存テストの期待値変更を伴う。以下は事前調査で特定した主な回帰対象であり、
網羅的な一覧は `03_implementation_plan.md` で確定させる。

| テスト | 現在の期待 | 変更後の期待 | 対応 AC |
|---|---|---|---|
| `cmd/runner/integration_dryrun_verification_test.go:70` `TestDryRunE2E_HashFilesNotFound` | exit 0 | exit 3（ハッシュ未登録＝環境起因） | AC-09 |
| `cmd/runner/integration_dryrun_verification_test.go:66` 付近（同ファイル先行テスト） | exit 0 | 検証状態に応じて要再確認 | AC-03 / AC-09 |
| `internal/runner/resource/security_test.go:171` "verification unavailable not a failure by default" | `DryRunExitAllow` | `DryRunExitVerificationUnavailable` | AC-04 |
| `internal/runner/resource/security_test.go:272-374` `TestDryRun_UnverifiedContentExitCode` | `failOnVerif` によるテーブル駆動 | フラグ軸を削除し F-002 の分類軸へ再構成 | AC-07〜AC-13 |

## 4. 非機能要件

- **NFR-01**: 本変更はセキュリティ既定値を安全側へ倒すものであり、いかなる設定・
  フラグによっても従来の「未検証でも exit 0」挙動へ戻せてはならない。
- **NFR-02**: `make test` および `make lint` が通ること。
- **NFR-03**: 削除対象のシンボルがコードベースに残存しないこと（デッドコードを残さない）。

## 5. 検討事項（レビュー確認依頼）

- **Q-01**: F-002 の表に含めた `verify_failed_hash_directory_not_found` の環境起因
  （exit 3）分類について。この理由は要件検討時の選択肢提示から漏れていた
  （`internal/runner/resource/types.go:101-104` の doc comment 自体が 5 値中 4 値しか
  列挙しておらず、それを元に選択肢を作成したため）。ハッシュ DB 自体が存在しない
  ケースであり `hash_file_not_found` 以上に環境起因と考えられるため exit 3 としたが、
  レビューでの確認を要する。
- **Q-02**: `file_read_error` / `permission_denied` を改ざん兆候（exit 1）に据え置く点。
  現行挙動の維持であり、かつ「検証を試行して失敗した」ものを安全側に倒す方針とは
  整合するが、`permission_denied` は環境起因の色も濃い。据え置きで問題ないか確認を要する。
