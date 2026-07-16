# dry-run における未検証成果物の常時 hard fail 化（`-dry-run-fail-unverified` フラグ削除） — 要件定義書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-16 |
| Review date | 2026-07-16 |
| Reviewer | isseis |
| Comments | |

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

1. **フラグは終了コードのみを変える。** プレビュー本体（コマンド列の展開、ファイル検証、
   `UNVERIFIED` セクションを含むレポート出力）はフラグの有無に関わらず同一であり、
   フラグは終了コードの決定にのみ影響する。
2. **したがって「ローカルでプレビューを目視する」用途はフラグ常時有効でも成立する。**
   0136/0146 が既定 exit 0 の根拠として挙げていた「本番ハッシュ DB を持たないローカル
   環境でのプレビュー」「設定ファイル作成中のブートストラップ」は、いずれも人間が出力を
   読む用途であり、終了コードに依存しない。
3. **残る差分は、終了コードだけを見て機械的に成否を判定する自動化のみ。** そこでは
   「未検証成果物がある＝非ゼロ終了」が安全側の既定として望ましい。既定 exit 0 は、
   `UNVERIFIED-TAMPER`（`hash_mismatch`、改ざん兆候）を含む場合ですら CI を緑にしてしまう。

一方で、常時有効化をそのまま適用すると **既存の分類が実態に合わない** 問題が顕在化する。
現在の実装は、検証失敗の原因を問わず一律に「改ざん兆候」とみなし exit 1（policy deny）へ
倒している。これはフラグがオプトインである限り影響が限定的だったが、常時有効化すると
**ハッシュ未登録・ハッシュ DB 未整備という環境起因の状況が「ポリシー拒否」として
報告される**ことになり、`skipped_no_validator` のみを環境起因（exit 3）とする現在の
線引きが破綻する。本タスクではこの分類も併せて是正する。

### 目的

1. `-dry-run-fail-unverified` フラグを **削除** し、未検証成果物を採用した dry-run は
   常に非ゼロ終了する挙動を **既定かつ唯一の挙動** にする（F-001）。
2. 未検証理由の **環境起因／改ざん兆候の分離を実態に合わせて再定義** し、常時有効化に
   よってブートストラップ工程が「ポリシー拒否」と誤報されることを防ぐ（F-002）。
3. 上記の破壊的変更を **ユーザードキュメントへ反映** する（F-003）。
4. `verify_files`（`global.verify_files` / `groups[].verify_files`）の検証失敗を dry-run の
   終了コードへ反映し、改ざん兆候（`hash_mismatch`）を CI で検知できるようにする（F-005）。
   従来これらの検証失敗は dry-run では終了コードに反映されず exit 0 のままであり、常時
   hard fail 化の目的（改ざん兆候の検知）を部分的にしか満たしていなかった。
5. 非ゼロ終了の根拠（`Failures` / `UNVERIFIED` セクション）を、詳細レベル
   （`-dry-run-detail`）によらず出力し、`summary` 指定時でも終了コードの原因を
   標準出力から追跡できるようにする（F-006）。

### スコープ外

- **dry-run 出力の表示「内容」の変更**。`Failures` / `UNVERIFIED` / `UNVERIFIED-TAMPER`
  セクションの各行の項目、`security_risk` 注釈、マーカー、ログレベルは本タスクでは変更しない。
  ただしこれらのセクションを **どの詳細レベルで表示するか**（表示の可視性）は F-006 で変更する
  （表示内容自体は不変）。
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
  `verify_failed_<FailureReason>` の 2 形式で、`FailureReason` は 5 値ある。
- **ハッシュディレクトリの不在は dry-run では自動的に解消され、本番実行では
  hard fail として扱われる。** 実測（存在しないハッシュディレクトリを指定して
  dry-run 実行）では、ディレクトリが作成されたうえで各ファイルが
  `verify_failed_hash_file_not_found` となり、`hash_directory_not_found` は
  発生しなかった。この事実は Q-01 および AC-08 の検証方法に影響する（§5 参照）。

## 2. 機能要件

本書の AC は AC-01〜AC-27（AC-20〜AC-24 は F-005、AC-25〜AC-27 は F-006 で追加）。

### F-001: `-dry-run-fail-unverified` の削除と strict 挙動の常時有効化

`-dry-run-fail-unverified` フラグおよびそれを伝搬する `DryRunOptions` フィールド・
内部フィールドを削除し、未検証成果物の採用および検証不能 deny を常に非ゼロ終了として
扱う。フラグは互換のための no-op として残さず、完全に削除する（指定された場合は Go の
`flag` パッケージによる未定義フラグエラーで終了する）。

**影響範囲**: フラグ本体（変数・`flag` 定義・`DryRunOptions` への伝搬）、フラグを
保持する構造体フィールドとその doc comment、フラグに言及する他パッケージの doc
comment、フラグに依存するテスト、`docs/translation_glossary.md` の変更履歴が対象。
リポジトリ全体の grep で確定するが、**凍結された過去タスク文書（0136 / 0146 など、
§1「前提・依存」参照）は意図的に除外**しており、そこに残る言及は本タスクの対象外。
削除対象シンボルの具体的なファイル・行番号一覧は `03_implementation_plan.md` で管理する
（NFR-03 により doc comment 中の言及も削除対象に含む）。

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

検証失敗を一律に改ざん兆候として扱う現行の分類を改め、`FailureReason` の値ごとに
「環境起因（この環境では検証できない／記録が無い）」と「改ざん兆候（検証を試みて
不整合を検出した）」を区別する。

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

**分類の根拠**: 本表の軸は「検証を試行し、記録との不整合を実際に検出したか」である。
`hash_mismatch` だけがこれを満たし、他の 4 理由は「検証できなかった」に留まるため
環境起因とする。この軸は、表示側が付与する `security_risk` 注釈やログレベルとは
異なる軸であり、両者の値が一致しなくても矛盾ではない（表示側は本タスクで変更しない。
スコープ外を参照）。また本分類は、表示側で「改ざん兆候」とみなす条件（`hash_mismatch`
のみ）と一致するため、利用者は dry-run 出力の表示と終了コードを一貫した理由として
読み取れる。判定ロジックの実装方法（表示側・終了コード側で共通の判定関数を使う等）は
設計文書で扱う。

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

**影響範囲**: `docs/user/runner_command.md` および `.ja.md` の
`-dry-run-fail-unverified` 節（終了コード表、Use Cases、Usage Examples、Notes）と、
`docs/translation_glossary.md` の当該フラグ由来の用語エントリ。具体的な行範囲は
`03_implementation_plan.md` で管理する。

**Acceptance Criteria**:
- **AC-15**: `docs/user/runner_command.md` および `docs/user/runner_command.ja.md` から
  `-dry-run-fail-unverified` フラグの記述が削除され、dry-run の終了コード表が
  フラグ非依存の記述（`0` / `1` / `3` の意味と F-002 の分類）へ更新されている。
- **AC-16**: 両ドキュメントに、本変更が破壊的変更であること、および当該フラグを
  指定している既存の呼び出しはフラグを除去すれば同一の挙動になることが明記されている。
- **AC-17**: 日本語版と英語版の章構成・記述内容が対応しており、
  `docs/translation_glossary.md` の用語が更新後の記述と整合している。

### F-004: 既存 E2E テストの是正（Q-03 の決定により本タスクで実施）

§3 で判明したテスト名と実態の乖離を是正する。これは AC-05 を検証可能にするために
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

本要件のレビュー時に、対象ファイルのハッシュを事前記録した状態で dry-run が
exit 0（`Verified: 2` / `Failed: 0` / ALLOW）となることを実測で確認済みであり、
AC-18 は実現可能である。具体的な実装方法は `03_implementation_plan.md` で扱う。

### F-005: `verify_files` の検証失敗の終了コード反映

設定ファイル／テンプレートファイル以外にも、`global.verify_files` /
`groups[].verify_files` に列挙されたファイルは dry-run 時にハッシュ検証される。しかし現状
これらの検証失敗は、内容を採用しないため「未検証成果物」
（`FileVerificationSummary.UnverifiedFiles`）ではなく検証失敗
（`FileVerificationSummary.Failures`）にのみ記録され、dry-run の終了コード決定
（`UsedUnverifiedContent` を入口条件とする）に反映されない。その結果、これらのファイルに
`hash_mismatch`（改ざん兆候）があっても dry-run は exit 0 を返す。

`verify_files` は任意の成果物をハッシュで固定するための機構であり、改ざん検知の主要な対象で
ある。本要件は、これらの検証失敗も F-002 と同一の分類軸で終了コードへ反映し、
F-001（常時 hard fail 化）の目的を完全に満たす。

終了コードの判定は検証失敗（`Failures`）の**発生源を問わず一律**に行う。したがって本要件で
新設する判定は、`verify_files` に限らず `Failures` に記録される任意の検証失敗へ自動的に
適用される。

なお env ファイルの検証（`VerifyEnvironmentFile`）も同じ経路（`Failures` への記録）を
使う設計だったが、F-007 のとおり production の呼び出し元が存在しない死んだコードであり
本タスクで削除する。削除後は `Failures` に env コンテキストのエントリを生成する production
経路がそもそも存在しないため、「env コンテキストの `hash_mismatch` を個別に分類する」という
シナリオ自体が起こり得ない。この分類は AC-20 が検証する「`Failures` の `hash_mismatch` は
発生源によらず exit 1 になる」という一般則の特殊ケースに過ぎず、発生し得ないケースを
ユニットテストのフィクスチャで人為的に再現しても実装の正しさに関する追加の保証にはならない
ため、独立した AC としては定義しない（AC-21 は意図的な欠番。理由は本節末尾の「AC 番号に
ついて」を参照）。

**分類**: F-002 の分類表と同一の軸を適用する。すなわち `Failures` に記録された理由のうち
`hash_mismatch` のみを改ざん兆候（exit 1）とし、他の理由（`hash_file_not_found` /
`hash_directory_not_found` / `file_read_error` / `permission_denied`）は環境起因（exit 3）と
する。優先順位は F-002 と統合し、`UnverifiedFiles` と `Failures` の**和集合**に対して判定する
（いずれか一方にでも `hash_mismatch` があれば exit 1）。

**スコープ**: 本要件は dry-run の終了コード決定にのみ影響する。非 dry-run（実行経路）は
従来どおり `verify_files` の検証失敗で hard fail する（本タスクの対象外）。表示側
（`Failures` セクション、`security_risk` 注釈、ログレベル）は変更しない。

**影響範囲（破壊的変更）**: `verify_files` に列挙したファイルのハッシュを未登録・不一致の
まま dry-run を実行していた既存の呼び出しは、exit 0 から exit 3 または exit 1 へ変わる。

**Acceptance Criteria**:
- **AC-20**: `global.verify_files` または `groups[].verify_files` に列挙されたファイルの
  検証が `hash_mismatch` で失敗した場合、dry-run は `DryRunExitPolicyDeny`（= 1）を返す。
- **AC-22**: `verify_files` の `hash_mismatch` 以外の検証失敗（例: `hash_file_not_found`）は、
  `hash_mismatch` を伴わない場合 `DryRunExitVerificationUnavailable`（= 3）を返す。
- **AC-23**: `verify_files` の `hash_mismatch` と、環境起因の未検証成果物・検証失敗が
  混在する場合、`DryRunExitPolicyDeny`（= 1）が優先される。改ざん兆候が環境起因のコードに
  埋没しない。
- **AC-24**: 上記の反映は終了コードの決定にのみ影響し、dry-run 出力上の `Failures` 表示、
  `security_risk` 注釈、ログレベルは変更されない。

**AC 番号について**: AC-21 は **意図的な欠番**（env コンテキストの `hash_mismatch` を
個別分類する AC として検討されたが、F-007 で `VerifyEnvironmentFile` を削除した結果、
当該コンテキストを production から生成する経路が存在しなくなり、AC-20 の一般則で
既に担保されるため独立の AC としては採用しなかった）。要件プロセスの規則
（欠番は許容、既存番号の振り直し禁止）に従い、AC-22 以降の番号はそのまま維持する。

### F-006: 非ゼロ終了の根拠を詳細レベルによらず出力する

現状、dry-run のテキスト出力では、検証失敗（`Failures`）および未検証成果物（`UNVERIFIED` /
`UNVERIFIED-TAMPER`）の各セクションが `-dry-run-detail detailed` / `full` でのみ出力され、
`summary` では省略される。既定は `detailed` のため通常は問題にならないが、`summary` を
明示的に指定した場合、F-001 / F-005 によって dry-run が非ゼロ終了するにもかかわらず、
その根拠（どのファイルが、なぜ）が標準出力に現れない（説明のない拒否）。

F-001 / F-005 の常時 hard fail 化により、`Failures` または `UnverifiedFiles` に要素が
1 件でもあれば dry-run は必ず非ゼロ終了する。すなわちこれらのセクションに載る情報は
すべて終了コードの根拠であり、詳細レベルで省略してよい補助情報ではない。本要件は、
これらのセクションを **詳細レベルによらず出力する**（要素が存在する場合は常に表示する）。

**表示内容の不変性**: 各セクションの行項目・`security_risk` 注釈・マーカーは変更しない
（F-005 のスコープ外と一致）。本要件が変えるのは「どの詳細レベルで表示するか」だけである。
正常系（両セクションとも空）では従来どおり何も表示されず、`summary` の簡潔さは保たれる。

**対象**: テキスト出力（`-dry-run-format text`）。JSON 出力（`-dry-run-format json`）は
現状すでに詳細レベルによらず全データを含むため、変更不要。

**Acceptance Criteria**:
- **AC-25**: `-dry-run-detail summary` かつ `-dry-run-format text` において、`Failures` が
  1 件以上ある場合、その一覧（ファイルパスと理由）が標準出力に現れる。
- **AC-26**: `-dry-run-detail summary` かつ `-dry-run-format text` において、
  `UnverifiedFiles` が 1 件以上ある場合、その一覧（ファイルパス・理由・`UNVERIFIED` /
  `UNVERIFIED-TAMPER` マーカー）が標準出力に現れる。
- **AC-27**: すべての検証が成功した正常系（`Failures` と `UnverifiedFiles` がともに空）の
  `summary` テキスト出力にはこれらのセクションが現れず、従来どおり簡潔なままである
  （回帰しない）。

### F-007: 事前クリーンアップ — 死んだコード `VerifyEnvironmentFile` の削除

`internal/verification/manager.go` の `VerifyEnvironmentFile` は、`-env-file`
コマンドラインオプション向けの検証関数だったが、当該オプションは 2025-09 に削除された
（`9c944c78` "doc: remove text referring obsolete -env-file command line option"）。
オプション削除時にドキュメント（`docs/design-implementation-overview.md` 等）からは
env ファイル検証への言及が除去されたが、コード自体は取り残された。

リポジトリ全体の grep で確認した限り production の呼び出し元は存在しておらず
（テストコードからのみ呼び出されていた）、その唯一の呼び出し元であったテストコードも
本タスクで本関数とともに削除された。またタスク 0146 の調査
（`docs/tasks/0146_security_hardening/03_implementation_plan.md` の該当メモ）でも、
env ファイルの実内容を読み込む production 経路自体が存在しないことが確認済みである。
加えてこの関数は検証のみを行い内容を返さない設計であり、`VerifyAndReadConfigFile` /
`VerifyAndReadTemplateFile` が担う「検証と読み込みを同一 I/O で行うことで TOCTOU を
防ぐ」パターンに従っていない。将来 env ファイル検証を復活させる場合も、このシグネチャを
そのまま再利用するべきではなく、新規に設計し直すべきである。

当初 F-005 では、env コンテキストの `Failures` を個別に分類する AC-21 を防御的に定義する
案があったが、本関数を削除すると env コンテキストの `Failures` エントリを生成する
production 経路がそもそも存在しなくなるため、その AC は「起こり得ないケースを検証する」
ものになり不要と判断した（F-005 の「AC 番号について」を参照。AC-21 は意図的な欠番）。
NFR-03（デッドコードを残さない）を満たすため、本関数の削除自体は本タスクの一部として
実施する。

**Acceptance Criteria**:
- **AC-28**: `internal/verification/manager.go` から `VerifyEnvironmentFile` が削除されて
  いる。あわせて `internal/verification/manager_test.go` の
  `TestVerifyEnvironmentFile` が削除されている。`make build` および `make test` が
  通ることをもって、他に呼び出し元が存在しないことを確認する。

## 3. 影響を受ける既存テスト

本変更は既存の E2E テスト（`cmd/runner/integration_dryrun_verification_test.go`）と
ユニットテスト（`internal/runner/resource/security_test.go`）の期待値変更を伴う。
実測により、影響範囲は当初想定より広いことが判明した。具体的なテスト単位の
変更内容・対応 AC の一覧は `03_implementation_plan.md` で管理する。

実測で判明した特筆すべき事実（F-004 の根拠）:

- 既存 E2E テストの一部は、ハッシュディレクトリが未登録の状態で実行されており、
  結果として `TestDryRunE2E_HashDirectoryNotFound` は実際には
  `hash_directory_not_found` を再現しておらず、`TestDryRunE2E_HashFilesNotFound` と
  セットアップ・期待が完全に重複している。→ 重複テストとして削除する（AC-19）。
- `TestDryRunE2E_AllSuccess` はハッシュを事前記録しておらず、名前が示す「全検証成功」
  を実際には再現していない。→ ハッシュ事前記録へ修正する（AC-18）。
- **AC-05**（全検証成功で exit 0）と **AC-08**（`hash_directory_not_found` で exit 3）は、
  既存 E2E に有効なカバレッジが無い。AC-05 は F-004（AC-18 の修正）で担保し、AC-08 は
  runner がハッシュディレクトリを起動時に自動作成するため E2E では再現不能であり、
  ユニットテストで担保する。

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
  経緯: 本理由は要件検討時の選択肢提示から漏れていた。
  調査結果: 実測により、runner はハッシュディレクトリが存在しない場合に**これを
  起動時に作成する**ため、dry-run プレビュー経路で `hash_directory_not_found` は
  発生しないことを確認した（§1 前提・依存）。本分類は**到達不能な経路に対する
  防御的な定義**であり、実挙動への影響は無い。AC-08 はユニットテストで担保する
  （§3 参照）。
- **Q-02（解決）**: `file_read_error` / `permission_denied` の分類。
  **決定: 環境起因（exit 3）とする**（当初案の「改ざん兆候（exit 1）据え置き」から
  **変更**）。
  影響: これにより **`hash_mismatch` のみが exit 1** となり、分類が大幅に単純化された。
  副次的効果として、当初レビューで指摘した「表示は素の `UNVERIFIED` なのに exit 1」
  という乖離が**解消**され、終了コードが表示（`UNVERIFIED-TAMPER`）および
  `security_risk`（high）と 1 対 1 で対応する一貫したモデルとなった。
- **Q-03（解決）**: `TestDryRunE2E_HashDirectoryNotFound` /
  `TestDryRunE2E_AllSuccess` の名称と実態の乖離。
  **決定: 本タスクで修正する。** F-004（AC-18 / AC-19）として要件化した。
