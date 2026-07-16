# dry-run でハッシュディレクトリを作成しない（read-only 検証） — 要件定義書

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

タスク 0147（`docs/tasks/0147_dry_run_strict_unverified_default/`）の要件書 §1「前提・依存」に、
次の実測事実が記録されている。

> ハッシュディレクトリの不在は dry-run では自動的に解消され、本番実行では hard fail として
> 扱われる。実測（存在しないハッシュディレクトリを指定して dry-run 実行）では、ディレクトリが
> 作成されたうえで各ファイルが `verify_failed_hash_file_not_found` となり、
> `hash_directory_not_found` は発生しなかった。

dry-run は「プレビューであり本番環境に副作用を及ぼさない」モードであるにもかかわらず、
**存在しないハッシュディレクトリを作成する**という副作用を持つ。これはモードの原則に反する。

#### 不一致の原因

この挙動は「dry-run が意図的にディレクトリを作る」設計ではなく、**存在チェックのガードを
飛ばした結果、バリデータ構築時の無条件 `os.MkdirAll` に素通りする**ために生じている。

- **本番実行**（`verification.NewManagerForProduction`）は `skipHashDirectoryValidation` を
  設定しないため、`validateHashDirectoryWithFS` がディレクトリの存在を検査し、不在なら
  `filevalidator.New`（＝ディレクトリ作成を含む）に到達する前に hard fail する。
- **dry-run**（`verification.NewManagerForDryRun`）は環境に既存のハッシュ DB を要求しないため
  同ガードをスキップする。その結果、`filevalidator.New` → `fileanalysis.NewStore` の無条件
  `os.MkdirAll` に素通りし、ディレクトリが作成される。

作成後のディレクトリは空であるため、各ファイルの検証は本来の「ディレクトリが無い」
（`hash_directory_not_found`）ではなく「当該ファイルのハッシュが無い」
（`hash_file_not_found`）へ**格下げ**される。すなわち副作用に加えて、未検証理由の**誤ラベル**も
発生している。

### 目的

1. dry-run が存在しないハッシュディレクトリを**作成しない**ようにする（副作用の除去、F-001）。
2. ハッシュディレクトリが不在の場合、検証対象ファイルを**忠実に**
   `verify_failed_hash_directory_not_found`（環境起因）として報告する。`hash_file_not_found`
   への格下げ、および `skipped_no_validator` への丸め込みを行わない（F-002）。
3. 上記の挙動変更を**ユーザードキュメントへ反映**する（F-003）。

### 分類と終了コードの前提（0147 からの継承）

未検証理由から終了コードへの写像はタスク 0147 が定義済みであり、本タスクは**その写像を
変更しない**。関係する行のみ再掲する。

| 未検証理由 | 区分 | 終了コード |
|---|---|---|
| `verify_failed_hash_directory_not_found` | 環境起因 | `3` |
| `verify_failed_hash_file_not_found` | 環境起因 | `3` |
| `skipped_no_validator` | 環境起因 | `3` |

3 者はいずれも環境起因（exit 3）であるため、**本タスクによって dry-run の終了コードは変化
しない**。本タスクが解消するのは (a) ディレクトリ作成という副作用、(b) 報告される未検証理由が
実態（ディレクトリ不在）と一致しないこと、の 2 点である。

### スコープ外

- **終了コードの決定ロジックの変更**。0147 が定めた未検証理由 → 終了コードの写像は変更しない。
- **本番実行（非 dry-run）経路の挙動変更**。本番は従来どおり、不在ハッシュディレクトリで
  hard fail する。
- **`record` コマンドの挙動変更**。`record` はハッシュ記録の作成が本務であり、ディレクトリの
  自動作成（`filevalidator.New` の現挙動）を従来どおり維持する。
- **0147 のタスク文書（要件・設計・実装計画）の改訂**。0147 は意思決定の履歴として、当時の
  実測（自動作成される）を記述したまま凍結する。現行挙動の記述は本書および `docs/user/` 配下が
  担う。

### 前提・依存

- 本タスクは 0147 の**後続**であり、0147 が先にマージされることを前提とする。0147 は
  「dry-run はハッシュディレクトリを自動作成する」実測を前提に、`hash_directory_not_found` を
  「E2E で再現不能な防御的定義」として扱い、重複する E2E テスト
  `TestDryRunE2E_HashDirectoryNotFound` を削除する（0147 AC-19）。本タスクは同経路を
  **再現可能**にするため、当該理由の忠実な E2E テストを新規に追加する（削除の巻き戻しではなく、
  実態に合致した新規テスト）。
- ハッシュディレクトリの作成は `internal/fileanalysis`（`NewStore` の `os.MkdirAll`）で行われ、
  `internal/filevalidator`（`New`）を経由して `internal/verification` の dry-run マネージャ生成
  （`NewManagerForDryRun`）から到達する。3 パッケージが関係する。
- 未検証理由 `hash_directory_not_found` は `internal/verification` に既存の `FailureReason`
  （`ReasonHashDirNotFound`）として定義済みであり、`filevalidator.ErrHashDirNotExist` から
  `determineFailureReason` で写像される。本タスクは新しい理由値を追加しない。

## 2. 機能要件

本書の AC は AC-01〜AC-09。

### F-001: dry-run はハッシュディレクトリを作成しない

dry-run 実行時、指定されたハッシュディレクトリが存在しない場合でも、これを作成しない。
dry-run は検証結果を読むだけであり、ハッシュ DB へ書き込む必要がないため、read-only な
検証で足りる。

**Acceptance Criteria**:
- **AC-01**: 存在しないパスをハッシュディレクトリとして dry-run を実行しても、実行後に当該
  パスが作成されていない（ディレクトリもファイルも生成されない）。
- **AC-02**: ハッシュディレクトリが存在し、対象ファイルのハッシュが記録済みの dry-run は、
  従来どおり検証に成功する（read-only 化によって既存の正常系が回帰しない）。

### F-002: ディレクトリ不在時は `hash_directory_not_found` として忠実に報告

dry-run でハッシュディレクトリが不在の場合、検証対象ファイル（設定ファイル・テンプレート
ファイル・`verify_files`・env ファイル）の未検証理由を
`verify_failed_hash_directory_not_found` として報告する。

**Acceptance Criteria**:
- **AC-03**: ハッシュディレクトリが不在の dry-run では、検証対象の各ファイルの未検証理由が
  `hash_directory_not_found` になる（`hash_file_not_found` へ格下げされない）。
- **AC-04**: 同状況で、未検証理由は `skipped_no_validator` にならない（バリデータは構成された
  うえで「ディレクトリが無い」と報告する）。
- **AC-05**: 同状況の dry-run の終了コードは `DryRunExitVerificationUnavailable`（= 3）である
  （0147 の分類に従い、環境起因のため）。
- **AC-06**: 同状況で、`verify_files`（`global.verify_files` / `groups[].verify_files`）および
  env ファイルの検証失敗も `hash_directory_not_found`（環境起因）として扱われ、`hash_mismatch`
  を伴わない限り exit 1 にはならない（0147 F-005 の分類軸と整合する）。

### F-003: ドキュメント更新

dry-run がハッシュディレクトリを作成しないこと、および不在時の未検証理由が
`hash_directory_not_found`（exit 3）であることを、ユーザー向けドキュメントへ反映する。

**影響範囲**: `docs/user/runner_command.md` および `.ja.md` の dry-run / 終了コードに関する節。
具体的な行範囲は `03_implementation_plan.md` で管理する。

**Acceptance Criteria**:
- **AC-07**: `docs/user/runner_command.md` および `docs/user/runner_command.ja.md` に、dry-run が
  存在しないハッシュディレクトリを作成しないこと、および不在時の未検証理由が
  `hash_directory_not_found`（exit 3）であることが記載されている。
- **AC-08**: 日本語版と英語版の当該記述が対応している。

### F-004: 本番実行経路と `record` コマンドの不変性

本変更は dry-run の検証経路のみを対象とし、本番実行および `record` コマンドの挙動を変更しない。

**Acceptance Criteria**:
- **AC-09**: 本番実行（非 dry-run）で不在のハッシュディレクトリを指定した場合、従来どおり
  hard fail する。また `record` コマンドは従来どおり、不在のハッシュディレクトリを自動作成する。

## 3. 非機能要件

- **NFR-01**: 本変更はセキュリティ既定値を安全側へ倒すもの（副作用の除去・分類の忠実化）で
  あり、いかなる設定・フラグによっても「dry-run がディレクトリを作成する」旧挙動へ戻せて
  はならない。
- **NFR-02**: `make test` および `make lint` が通ること。
- **NFR-03**: read-only 化に伴い不要となるコード（例: dry-run 専用のディレクトリ作成
  フォールバック）を残さないこと（デッドコードを残さない）。ただし `record` コマンドが使う
  作成モード（`filevalidator.New`）は温存する。

## 4. 検討事項

以下は設計フェーズ（`02_architecture.md`）で確定する論点である。

- **Q-01**: read-only 時のパス解決。不在ディレクトリに対する `common.NewResolvedPath` は解決に
  失敗する。read-only の `Store` 構築時にパス解決を遅延させるか、raw パスを保持するかを設計で
  確定する。
- **Q-02**: ハッシュディレクトリのパスが「存在するがディレクトリではない」場合の dry-run での
  扱い（`hash_directory_not_found` として報告するか、ハードエラーとするか）。本タスクの主眼で
  ある「不在」ケースとは別のエッジであり、現挙動（`ErrHashPathNotDir` / `ErrAnalysisDirNotDirectory`）
  を踏まえて設計で確定する。
- **Q-03**: read-only 化により、ハッシュディレクトリが存在するが読み取り権限が無い場合の理由が
  `permission_denied`（exit 3）となる。現行の dry-run 専用フォールバック
  （権限エラー時にバリデータを nil 化して続行）の要否を設計で再評価する。
