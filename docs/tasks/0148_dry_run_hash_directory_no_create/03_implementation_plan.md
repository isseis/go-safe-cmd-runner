# dry-run でハッシュディレクトリを作成しない（read-only 検証） — 実装計画書

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-17 |
| Review date | 2026-07-18 |
| Reviewer | isseis |
| Comments | - |

## 1. 実装概要

### 1.1 目的

`02_architecture.md` の設計に従い、dry-run 実行時のハッシュディレクトリ構築を「無条件作成
（`os.MkdirAll`、副作用あり）」から「read-only 検証（副作用なし）」へ変更する。具体的には
以下を実装する。

- `internal/fileanalysis.NewStoreReadOnly`: ディレクトリを作成しない `Store` 構築経路。
- `internal/filevalidator.NewReadOnly`: ディレクトリを作成しない `Validator` 構築経路。不在・
  権限不足を「遅延エラー」として保持し、構築自体は成功させる。
- `internal/verification` の dry-run マネージャ生成を `NewReadOnly` へ切り替え、不要になった
  `os.ErrPermission` フォールバック分岐を除去する。
- 忠実な E2E テストの新規追加、およびユーザードキュメントの更新。

本番実行（`NewManagerForProduction`）と `record` コマンドの経路は変更しない。

### 1.2 実装方針

- **既存経路の再利用（DRY）**: `fileanalysis.NewStore` および `filevalidator.New` の「ディレクトリ
  解決後に `Store`/`Validator` を組み立てる」部分ロジックを、新規追加する read-only 経路と共有する。
  重複するフィールド組み立てコードを増やさない。
- **YAGNI**: 新しい未検証理由値・終了コード・設定フラグを追加しない。既存の `ReasonHashDirNotFound`
  / `ReasonPermissionDenied` と `determineFailureReason` の写像をそのまま利用する。
- **最小変更**: dry-run が実際に呼び出す `Validator` のメソッド（`Verify` / `VerifyWithHash` /
  `VerifyAndRead` / `LoadRecord`）のみを遅延エラーで保護する。`SaveRecord` は dry-run から呼ばれず
  `record` 専用の経路（`filevalidator.New`）でのみ使われるため変更しない。

### 1.3 既存コード調査結果

以下は実装着手前にコードベースを調査した結果である。ファイルごとに「現状」「変更内容」を示す。

#### `internal/fileanalysis/file_analysis_store.go`

- 現状: `NewStore`（関数本体 42-68 行目、doc comment は 31-41 行目）は `os.Lstat` で不在を検知すると
  `os.MkdirAll`（49 行目）で作成し、その後 `common.NewResolvedPath` で解決してから `Store` を
  構築する（59-67 行目）。
- 変更: 59-67 行目の「解決してから `Store` を構築する」部分を `newStoreFromExistingDir` という
  非公開ヘルパーへ切り出し、`NewStore` と新設の `NewStoreReadOnly` の両方から呼び出す（DRY）。

#### `internal/filevalidator/validator.go`

- 現状: `New`（関数本体 179-202 行目、doc comment は 174-178 行目）は `fileanalysis.NewStore` の
  呼び出し（183 行目、ここで作成が起きる）→ `common.NewResolvedPath`（189 行目）→
  `newValidator`（206-237 行目、自身も `os.Lstat` で存在確認する）の順に進む。`Verify`（1075-1092 行目）、`VerifyWithHash`（1099-1118 行目）、`VerifyAndRead`
  （1199-1214 行目）、`LoadRecord`（453-463 行目）はいずれも `v.store` に直接アクセスする。
- 変更: `Validator` 構造体（151-172 行目）へ `deferredErr error` フィールドを追加する。新設の
  `NewReadOnly` はディレクトリの `os.Lstat` 結果に応じて、(a) 存在してディレクトリなら
  `fileanalysis.NewStoreReadOnly` を使い `newValidator` で通常どおり構築、(b) 不在
  （`os.IsNotExist`）なら `store` を持たず `deferredErr` に `ErrHashDirNotExist` を包んだエラーを
  設定した `Validator` を返す（構築は成功）、(c) 存在するが非ディレクトリなら `ErrHashPathNotDir`
  で構築失敗、(d) その他の `Lstat` エラー（権限不足等）なら `deferredErr` に生のエラーをそのまま
  設定した `Validator` を返す（構築は成功）。`Verify` / `VerifyWithHash` / `VerifyAndRead` /
  `LoadRecord` の先頭に `deferredErr` チェックを追加し、設定されていれば実ファイルへアクセスせず
  即座に返す。あわせて `HashDirAvailable() bool`（`deferredErr == nil` を返す）を追加し、
  `internal/verification` が `SetHashDirStatus` の入力に使う。

#### `internal/verification/manager.go`

- 現状: `newManagerInternal`（439-530 行目）のうち、
  - 474-494 行目: `opts.fileValidatorEnabled` が真のとき常に `filevalidator.New`（作成を含む）を
    呼び、dry-run かつ `os.ErrPermission` のときだけ `slog.Info` を出してバリデータを nil のまま
    続行するフォールバックがある。
  - 508-527 行目: dry-run のとき `opts.fs.FileExists(hashDir)` を個別に評価し、その結果を
    `resultCollector.SetHashDirStatus` に反映している（`filevalidator.New` の判定とは別プローブ）。
  - `determineFailureReason`（`internal/verification/result_collector.go` 149-173 行目）は
    `filevalidator.ErrHashDirNotExist` → `ReasonHashDirNotFound`、`os.ErrPermission` →
    `ReasonPermissionDenied` を既に写像済みであり、変更不要。
  - `verifyFile`（324 行目〜）、`verifyFileWithHash`（353 行目〜）、
    `readAndVerifyFileWithReadFallback`（392 行目〜）、`verifyDynLibDeps`（593 行目〜）、
    `VerifyCommandShebangInterpreter`（707 行目〜）、`verifyInterpreterHash`（825 行目）は、いずれも
    `m.fileValidator` の `Verify` / `VerifyWithHash` / `VerifyAndRead` / `LoadRecord` を直接呼ぶ
    共通経路であり、個別の分岐追加は不要（`02_architecture.md` §6.2 のとおり）。
- 変更: 474-494 行目を、dry-run のときは `filevalidator.NewReadOnly` を、それ以外は従来どおり
  `filevalidator.New` を呼ぶよう書き換え、`os.ErrPermission` フォールバック分岐を削除する。
  508-527 行目を、`opts.fs.FileExists` による二重プローブをやめ、構築した `*filevalidator.Validator`
  の `HashDirAvailable()` から `SetHashDirStatus` を導出する形へ書き換える。

#### `internal/verification/manager_production.go`

- 現状: `logDryRunManagerCreation`（87-105 行目）は監査ログ用の属性一覧を組み立てるのみで分岐は
  ない。94-96 行目に `"skip_hash_directory_validation", true` と `"file_validator_enabled", true`
  がある。
- 変更（軽微）: 96 行目の直後に `"construction_mode", "read_only"` を追加し、dry-run のバリデータが
  read-only 構築である旨を監査ログに残す。

#### `internal/verification/manager_test.go` / `test_helpers.go`

- 現状: `withFileValidatorDisabledInternal()`（`test_helpers.go` 108-112 行目）は
  `opts.fileValidatorEnabled = false` を設定する既存のテスト専用オプションであり、変更不要。
  `TestReadAndVerifyFileWithReadFallback_NoValidator_DryRunRecordsUnverified`
  （`manager_test.go` 992-1028 行目）は `withFileValidatorDisabledInternal()` を使って
  `fileValidator` を明示的に nil にしており、本タスクの変更後も成立する。ただし直前の関数コメント
  （987-991 行目）が「dry-run on a machine where the hash directory is not writable」という、
  本タスク後は成立しなくなる前提を書いている。
- 変更: 987-991 行目のコメントを、nil バリデータは検証機能そのものを無効化した場合
  （テストの `withFileValidatorDisabledInternal` 相当）にのみ生じる旨へ書き換える。実装内容は
  変更しない（コメントのみ）。

#### `internal/verification/manager_production_test.go`

- 現状: `TestProductionNewManagerForDryRun` の `"auto_creates_missing_hash_dir_and_enables_validator"`
  サブテスト（136-155 行目）は、存在しないディレクトリを指定した dry-run マネージャ生成後に
  `os.Stat(nonexistentHashDir)` が成功する（＝自動作成される）ことを明示的に検証している。これは
  本タスクが変更する挙動そのものであり、**このサブテストは本タスク後に失敗する**。調査で確認した
  唯一のこの種の回帰対象である（`internal/verification`・`internal/runner/resource` 配下の他の
  dry-run 関連テストに同種の前提は見つからなかった）。
  `"dry_run_security_audit_logging"` サブテスト（157-178 行目）は `logDryRunManagerCreation` の
  ログ出力を検証しており、`"file_validator_enabled=true"` の検証は変更後も成立するが、新設する
  `"construction_mode=read_only"` 属性は未検証のままである。
  `TestDryRunManagerLogging`（231-253 行目）も同様に `logDryRunManagerCreation()` を直接呼ぶ
  ユニットテストで、新設属性は未検証。
- 変更: `"auto_creates_missing_hash_dir_and_enables_validator"` を「存在しないディレクトリを
  作成せず、かつ read-only 構築でバリデータが初期化される」ことを検証する内容へ書き換える
  （詳細はフェーズ 3 参照）。`"dry_run_security_audit_logging"` と `TestDryRunManagerLogging` へ
  `"construction_mode=read_only"` の検証を追加する。

#### `internal/verification/types.go`

- 現状: `UnverifiedReasonNoValidator` の doc コメント（149-153 行目）が「dry-run on a machine where
  the hash directory is not writable」という、本タスク後は成立しなくなる例を挙げている。
- 変更: コメントを、検証機能そのものを無効化した場合にのみ生じる旨へ書き換える（値
  `"skipped_no_validator"` 自体は変更しない）。

#### `cmd/runner/integration_dryrun_verification_test.go`

- 現状: `TestDryRunE2E_HashDirectoryNotFound` という名前のテストは存在しない（0147 が削除した
  `TestDryRunE2E_HashDirectoryNotFound` は「E2E で再現不能」という当時の前提に基づく削除であり、
  本タスクはその巻き戻しではなく、実態に即した新規追加である）。`setupTempConfig` (23-30 行目)、
  `runDryRunCommand` (34-44 行目)、`newGoRunCmdWithHashDir`
  (`cmd/runner/testutil_ldflags_test.go` 24-45 行目) など、再利用可能な既存ヘルパーがある。
  `newGoRunCmdWithHashDir` は指定した `hashDir` を作成せずそのまま `-ldflags` で埋め込むため、
  「存在しないディレクトリ」を渡すテストにそのまま使える。
- 変更: 新規テスト `TestDryRunE2E_HashDirectoryNotFound` を追加する（詳細はフェーズ 4 参照）。

#### `docs/user/runner_command.md` / `docs/user/runner_command.ja.md`

- 現状: 両ファイルとも 674 行目（EN/JA で行番号一致）が
  `skipped_no_validator` の説明として「ハッシュディレクトリが書き込み不可」という、本タスク後は
  誤りとなる例を挙げている。dry-run がハッシュディレクトリを作成する／作成しないについての記述は
  現状どちらのファイルにも存在しない（新規追加）。`docs/user/dry_run_json_schema.md` /
  `.ja.md` は理由コード一覧に `hash_directory_not_found` / `permission_denied` を既に含んでおり
  修正不要。
- 変更: 674 行目の説明を修正し、「ハッシュディレクトリの扱い（dry-run）」節を新規追加する
  （詳細はフェーズ 5 参照）。

## 2. 実装ステップ

### フェーズ 1: `internal/fileanalysis` — read-only Store

**対象ファイル**: `internal/fileanalysis/file_analysis_store.go`,
`internal/fileanalysis/file_analysis_store_test.go`

- [x] **ステップ 1-1**: `NewStore`（41-68 行目）の 59-67 行目（`common.NewResolvedPath` 呼び出し以降）を
      `newStoreFromExistingDir(analysisDir string, pathGetter common.HashFilePathGetter) (*Store, error)`
      という非公開ヘルパーへ切り出す。`NewStore` はこのヘルパーを呼び出すように書き換える。
- [x] **ステップ 1-2**: `NewStoreReadOnly(analysisDir string, pathGetter common.HashFilePathGetter) (*Store, error)`
      を追加する。`os.Lstat(analysisDir)` を行い、エラーなら
      `fmt.Errorf("failed to access analysis result directory: %w", err)` を返し（`os.MkdirAll`
      は呼ばない）、ディレクトリでなければ `fmt.Errorf("%w: %s", ErrAnalysisDirNotDirectory, analysisDir)`
      を返す。存在してディレクトリなら `newStoreFromExistingDir` を呼んで返す。
- [x] **ステップ 1-3**: `TestNewStoreReadOnly_MissingDirectory_ReturnsErrorWithoutCreating` を追加する。
      `TestNewStore_CreatesDirectory`（340-357 行目）と対になるよう、存在しないパスを渡して
      エラーが返ること、かつ `os.Stat` で当該パスが作成されていないことを確認する。
- [x] **ステップ 1-4**: `TestNewStoreReadOnly_ExistingDirectory` を追加する。`TestNewStore_ExistingDirectory`
      （359-366 行目）と同じ構成（既存の一時ディレクトリを渡して成功する）。
- [x] **ステップ 1-5**: `TestNewStoreReadOnly_NotADirectory` を追加する。`TestNewStore_NotADirectory`
      （368-380 行目）と同じ構成（ファイルパスを渡して `ErrAnalysisDirNotDirectory` を含むエラーが
      返る）。
- [x] **ステップ 1-6**: `TestNewStore_CreatesDirectory` / `TestNewStore_ExistingDirectory` / `TestNewStore_NotADirectory`
      がリファクタリング後も無変更で成立することを `go test -tags test ./internal/fileanalysis/...`
      で確認する（回帰確認）。

### PR-1 作成ポイント: fileanalysis read-only store

**対象ステップ**: 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6

**推奨タイトル**: `feat(0148): add read-only Store construction to internal/fileanalysis`

**レビュー観点**: `newStoreFromExistingDir` への切り出しが `NewStore` の既存挙動を変えていないか / `NewStoreReadOnly` が `os.MkdirAll` を一切呼ばないか / 既存 3 テストが無変更で回帰していないか

**実装モデル要件**: standard

**判定理由**: 既存 `NewStore` のヘルパー切り出しと、既存テストと対になる 3 本の新規テスト追加のみで、未確定の設計判断や競合する実装方針は無い。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 2: `internal/filevalidator` — read-only Validator（遅延エラー）

**対象ファイル**: `internal/filevalidator/validator.go`, `internal/filevalidator/validator_test.go`,
`internal/filevalidator/validator_error_test.go`

- [x] **ステップ 2-1**: `Validator` 構造体（151-172 行目）へ `deferredErr error` フィールドを追加する。
- [x] **ステップ 2-2**: `NewReadOnly(algorithm HashAlgorithm, hashDir string, cfg ValidatorConfig) (*Validator, error)`
      を追加する。実装は次のとおり。
      1. `algorithm == nil` なら `ErrNilAlgorithm` を返す（`New` は `newValidator` 経由でこの
         チェックを行うが、`NewReadOnly` の不在ディレクトリ分岐は `newValidator` を経由しないため、
         関数冒頭で明示的にチェックする）。
      2. `hashFilePathGetter := NewHybridHashFilePathGetter()` を作る。
      3. `info, err := os.Lstat(hashDir)` を行う。
         - `err == nil && info.IsDir()`: `fileanalysis.NewStoreReadOnly(hashDir, hashFilePathGetter)`
           で `store` を構築し、`common.NewResolvedPath(hashDir)` で解決したうえで
           `newValidator(algorithm, resolvedHashDir, hashFilePathGetter, cfg)` を呼び、
           `v.store = store` を設定して返す（`New` の 194-201 行目と同型の組み立て）。
           `NewStoreReadOnly` と `NewResolvedPath` の各呼び出しは、`New` の既存パターン
           （194-201 行目）と同様にエラーを都度チェックし、エラーがあれば `nil, err` を
           そのまま返す。
         - `err == nil && !info.IsDir()`: `fmt.Errorf("%w: %s", ErrHashPathNotDir, hashDir)` を返す
           （構築失敗）。
         - `os.IsNotExist(err)`: `newDeferredValidator(algorithm, hashFilePathGetter, cfg,
           fmt.Errorf("%w: %s", ErrHashDirNotExist, hashDir)), nil` を返す（構築成功、`store` は
           ゼロ値のまま）。
         - それ以外（`Lstat` が権限エラー等で失敗）: `newDeferredValidator(algorithm,
           hashFilePathGetter, cfg, err), nil` を返す（構築成功）。`err` はそのまま保持するため、
           `errors.Is(err, os.ErrPermission)` は `result_collector.go` の `determineFailureReason`
           でそのまま機能する。
- [x] **ステップ 2-3**: `Verify`（1075 行目）、`VerifyWithHash`（1099 行目）、`VerifyAndRead`（1199 行目）、
      `LoadRecord`（453 行目）の各先頭に、次の 2 行を追加する（戻り値の型に応じてゼロ値を調整）。
      ```go
      if v.deferredErr != nil {
          return v.deferredErr
      }
      ```
      （`VerifyWithHash` は `return "", v.deferredErr`、`VerifyAndRead` は
      `return nil, v.deferredErr`、`LoadRecord` は `return nil, v.deferredErr`。）
- [x] **ステップ 2-4**: `HashDirAvailable() bool` を追加する。`return v.deferredErr == nil` のみを返す。
- [x] **ステップ 2-5**: `TestNewReadOnly_MissingDirectory_DoesNotCreateDirectory` を `validator_test.go` の
      `TestNew_CreatesDirectory`（596-617 行目）の近くに追加する。存在しないパスを渡し、
      `NewReadOnly` がエラーなく `*Validator` を返すこと、かつ `os.Stat` で当該パスが作成されて
      いないことを確認する。
- [x] **ステップ 2-6**: `TestNewReadOnly_MissingDirectory_VerifyReturnsErrHashDirNotExist` を追加する。上記で得た
      `*Validator` に対して `Verify`・`VerifyWithHash`・`VerifyAndRead`・`LoadRecord` をそれぞれ
      呼び、いずれも `errors.Is(err, ErrHashDirNotExist)` が真であることを確認する（テーブル駆動で
      4 メソッドをまとめてよい）。
- [x] **ステップ 2-7**: `TestNewReadOnly_ExistingDirectory_VerifiesSuccessfully` を追加する。`TestNew`
      （102-140 行目付近）の「valid」ケースに相当する構成で、既存ディレクトリに対して
      `NewReadOnly` → `SaveRecord`（通常の `New` で作成した別 Validator、または同一
      ディレクトリを対象に `New` で先に記録してから `NewReadOnly` で検証する）→ `Verify` が成功する
      ことを確認する。
- [x] **ステップ 2-8**: `TestNewReadOnly_NotADirectory` を追加する。ファイルパスを渡し、`errors.Is(err, ErrHashPathNotDir)`
      を確認する。
- [x] **ステップ 2-9**: `TestNewReadOnly_NilAlgorithm` を追加する。`algorithm` に `nil` を渡し、
      `errors.Is(err, ErrNilAlgorithm)` を確認する。
- [x] **ステップ 2-10**: `validator_error_test.go`（`//go:build linux || freebsd || openbsd || netbsd`、既存の
      「unreadable directory」サブテスト 126-145 行目と同じ chmod パターンを踏襲）へ
      `TestNewReadOnly_ParentUnreadable_DeferredPermissionError` を追加する。手順:
      1. `tempDir` 配下に `restricted` ディレクトリを作成する。
      2. `restricted` の中に存在しない `hashDir := filepath.Join(restrictedDir, "hashes")` を
         パスとしてのみ用意する（作成しない）。
      3. `os.Chmod(restrictedDir, 0o000)` で親を辿れなくし、`t.Cleanup` で `0o755` に戻す。
      4. `NewReadOnly(&SHA256{}, hashDir, ValidatorConfig{})` を呼び、エラーなく `*Validator` が
         返ることを確認する。
      5. `Verify`（任意のファイルパスでよい）を呼び、`errors.Is(err, os.ErrPermission)` を確認する。
      本テストは `validator_error_test.go` の既存パーミッションテストと同様、実行ユーザーが
      root の場合は `chmod 0o000` によるアクセス拒否が発生せず意味をなさない。既存の
      「unreadable directory」テスト（126-145 行目）も同じ制約を前提としており、本タスクで
      新たに導入する制約ではないため、既存規約に倣い root 検知は追加しない。

### PR-2 作成ポイント: filevalidator read-only validator with deferred errors

**対象ステップ**: 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6 / 2-7 / 2-8 / 2-9 / 2-10

**推奨タイトル**: `feat(0148): add read-only Validator with deferred errors`

**レビュー観点**: `deferredErr` を保持したまま構築成功させる 4 分岐（存在/不在/非ディレクトリ/権限エラー）が `02_architecture.md` §3.1・§5.3 の Q-01〜Q-03 と一致しているか / `Verify`・`VerifyWithHash`・`VerifyAndRead`・`LoadRecord` の 4 メソッド全てで `deferredErr` チェック漏れがないか / chmod を使う権限テストが CI の root 実行下で偽陽性になる既知の制約を正しく踏襲しているか

**実装モデル要件**: frontier-recommended

**判定理由**: `deferredErr` の早期リターンチェックを `Verify`・`VerifyWithHash`・`VerifyAndRead`・`LoadRecord` の 4 メソッドすべてに一貫して適用しないと、いずれか 1 箇所の漏れが不在ディレクトリでの `nil` store 参照パニックへ直結する、孤立した高リスクロジックである（Risk isolation 該当）。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 3: `internal/verification` — dry-run の read-only 化

**対象ファイル**: `internal/verification/manager.go`, `internal/verification/manager_production.go`,
`internal/verification/manager_test.go`, `internal/verification/manager_production_test.go`,
`internal/verification/types.go`, 新規 `internal/verification/manager_permission_test.go`

- [x] **ステップ 3-1**: `manager.go` 474-494 行目を次の内容へ置き換える。
      ```go
      // Initialize file validator with hybrid hash path getter
      var hashDirAvailable bool
      if opts.fileValidatorEnabled {
          var (
              validator *filevalidator.Validator
              err       error
          )
          if opts.isDryRun {
              // Dry-run performs a read-only construction: a missing or
              // inaccessible hash directory is captured as a deferred error
              // and reported per file via determineFailureReason, instead of
              // being created as a side effect.
              validator, err = filevalidator.NewReadOnly(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
          } else {
              validator, err = filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
          }
          if err != nil {
              return nil, fmt.Errorf("failed to initialize file validator: %w", err)
          }
          manager.fileValidator = validator
          hashDirAvailable = validator.HashDirAvailable()
      }
      ```
      （`errors` パッケージが `os.ErrPermission` 分岐削除後も他用途で使われているか確認し、未使用に
      なった場合のみ import を削除する。）
- [x] **ステップ 3-2**: `manager.go` 508-527 行目（dry-run の `resultCollector` 初期化ブロック）を次の内容へ
      置き換える。`opts.fileValidatorEnabled` が偽の場合（現状はテスト専用の
      `withFileValidatorDisabledInternal()` 経由でのみ到達する）は `hashDirAvailable` が初期化
      されないため、従来どおり `opts.fs.FileExists` による判定を残す。読み取り専用バリデータが
      実際に構築された場合（`fileValidatorEnabled` が真）のみ `HashDirAvailable()` の結果を使う。
      こうすることで、バリデータを無効化したまま dry-run を構築する既存の呼び出し方
      （`opts.fs` にモックを注入するテストを含む）の `HashDirStatus` 判定を変更しない。
      ```go
      // Initialize result collector for dry-run mode
      if opts.isDryRun {
          manager.resultCollector = NewResultCollector(hashDir)

          if opts.fileValidatorEnabled {
              // hashDirAvailable was derived above from the read-only
              // Validator's own Lstat result; reuse it as the single source
              // of truth instead of probing the filesystem a second time.
              manager.resultCollector.SetHashDirStatus(hashDirAvailable)
          } else {
              // File validation is disabled for this manager instance (test-only
              // today), so no Validator was constructed to derive hashDirAvailable
              // from. Fall back to the pre-existing filesystem probe.
              exists, err := opts.fs.FileExists(hashDir)
              switch {
              case err != nil:
                  slog.Info("Unable to check hash directory existence in dry-run mode",
                      "hash_directory", hashDir,
                      "error", err)
                  manager.resultCollector.SetHashDirStatus(false)
              case !exists:
                  slog.Info("Hash directory does not exist in dry-run mode",
                      "hash_directory", hashDir)
                  manager.resultCollector.SetHashDirStatus(false)
              default:
                  manager.resultCollector.SetHashDirStatus(true)
              }
          }
      }
      ```
- [x] **ステップ 3-3**: `manager_test.go` に `TestVerifyConfigFile_DryRun_MissingHashDir` を追加する
      （`TestVerifyConfigFile_DryRun_HashFileNotFound`、1357-1385 行目、と対になる構成）。
      - `tmpDir := tu.SafeTempDir(t)`、`hashDir := filepath.Join(tmpDir, "does-not-exist")`
        （作成しない）、`configFile := createTestFile(t, tmpDir, "config.toml", []byte("test config"))`。
      - `manager := createDryRunManager(t, hashDir)` で構築し、
        `require.NotNil(t, manager.fileValidator)` を確認する（AC-04: バリデータは構成される）。
      - `manager.VerifyAndReadConfigFile(configFile)` を呼び、エラーなく内容が読めることを確認する。
      - `summary := manager.GetVerificationSummary()` を取得し、
        `summary.HashDirStatus.Exists` が `false`、`summary.Failures[0].Reason` が
        `ReasonHashDirNotFound`（`hash_file_not_found` ではないこと）、
        `summary.UnverifiedFiles[0].Reason` が `"verify_failed_hash_directory_not_found"`
        （`"skipped_no_validator"` ではないこと）であることを確認する。
      - `os.Stat(hashDir)` が `os.IsNotExist` を満たすことを確認する（AC-01）。
- [x] **ステップ 3-4**: `manager_test.go` に `TestVerifyGlobalFiles_DryRun_MissingHashDir_RecordsHashDirNotFound` を
      追加する。存在しない `hashDir` で `createDryRunManager` を構築し、`createRuntimeGlobal` で
      作成した `GlobalVerificationInput`（`ExpandedVerifyFiles` に実在する 1 ファイルを含む）を
      `manager.VerifyGlobalFiles` に渡す。戻り値の `err` が `nil`、`result.FailedFiles` に
      当該ファイルが含まれること、`summary.Failures[0].Reason == ReasonHashDirNotFound` を確認する。
      これは `global.verify_files` が通る `verifyFile`（`manager.go` 150 行目）を直接検証する
      （AC-06 前半）。
- [x] **ステップ 3-5**: `manager_test.go` に `TestVerifyGroupFiles_DryRun_MissingHashDir_RecordsHashDirNotFound` を
      追加する。同様に `createRuntimeGroup` を使い、`manager.VerifyGroupFiles` を経由する
      `verifyFileWithHash`（`manager.go` 213 行目）を検証する（AC-06 後半）。
      `groups[].verify_files` はいずれも `readAndVerifyFileWithReadFallback` /
      `verifyFile` / `verifyFileWithHash` のいずれかを通る共通経路であり（`02_architecture.md`
      §6.2）、これら 3 関数を直接対象にした本テストと `TestVerifyConfigFile_DryRun_MissingHashDir`
      を合わせることで、呼び出し元ごとに個別のテストを追加しなくても AC-06 の `verify_files` 部分は
      証明できる。

      **env ファイルについての注記**: AC-06 は env ファイルも同一理由（環境起因）として扱われる
      ことを要求している。しかし、env ファイルの実内容を検証していた
      `internal/verification.VerifyEnvironmentFile` は、タスク 0147
      （`docs/tasks/0147_dry_run_strict_unverified_default/`）の F-007（同要件書 307-322 行目）で
      「env ファイルの実内容を読み込む production 経路自体が存在しないことが確認済み」として
      削除されている（`grep -rn "VerifyEnvironmentFile" internal/ cmd/` はヒットしない）。
      したがって現時点では env ファイルを検証する production
      呼び出し経路が存在せず、AC-06 の env ファイルに関する条項は具体的なテストで実行できない
      （検証対象が存在しないため）。本タスクは `verifyFile` / `verifyFileWithHash` /
      `readAndVerifyFileWithReadFallback` という共通経路そのものを変更しないため、将来 env
      ファイル検証が復活した場合も同じ写像が適用されることは設計上保証されるが、これは本タスクの
      実装によって新たに証明されるものではない。AC-06 の検証は `verify_files`
      （global・group の両方）と、共通経路を実際に使う設定ファイル読み込みに対してのみ行う。
- [x] **ステップ 3-6**: 新規ファイル `internal/verification/manager_permission_test.go`
      （`//go:build linux || freebsd || openbsd || netbsd`、`package verification`）を作成し、
      `TestNewManagerInternal_DryRun_HashDirParentUnreadable_RecordsPermissionDenied` を追加する。
      `internal/filevalidator/validator_error_test.go` の「unreadable directory」パターン
      （親を `0o000` にし `t.Cleanup` で戻す）を踏襲し、`newManagerInternal` を
      `withDryRunModeInternal()` ・ `withSkipHashDirectoryValidationInternal()` ・
      `withCreationMode(CreationModeTesting)` ・ `withSecurityLevel(SecurityLevelRelaxed)` で構築、
      対象ファイルを 1 件検証して `summary.Failures[0].Reason == ReasonPermissionDenied` を確認する
      （AC-04・Q-03(d) の固定化）。root 実行時の制約は上記フェーズ 2 のテストと同様。
- [x] **ステップ 3-7**: `manager_production_test.go` の `"auto_creates_missing_hash_dir_and_enables_validator"`
      サブテスト（136-155 行目）を次のとおり書き換える。
      - サブテスト名を `"does_not_create_missing_hash_dir_but_initializes_read_only_validator"`
        に変更する。
      - コメント「`New() now auto-creates the directory, so the validator should be non-nil.`」を
        「`NewReadOnly() succeeds without creating the directory; the validator is still non-nil
        thanks to the deferred-error mechanism.`」へ書き換える。
      - `assert.NotNil(t, manager.fileValidator, ...)` は維持する（read-only 構築でも
        バリデータは構成されるため）。
      - `_, statErr := os.Stat(nonexistentHashDir)` の直後を
        `assert.True(t, os.IsNotExist(statErr), "hash directory must not be auto-created in dry-run mode")`
        へ変更する（従来の `assert.NoError` を反転）。
- [x] **ステップ 3-8**: `manager_test.go` に `TestNewManagerInternal_DryRun_HashPathNotDirectory_HardFails` を追加する。
      ハッシュディレクトリのパスとして通常ファイル（ディレクトリではないパス）を渡した
      `newManagerInternal`（`withDryRunModeInternal()` ・ `withSkipHashDirectoryValidationInternal()`
      を付与）がエラーを返し、`errors.Is(err, filevalidator.ErrHashPathNotDir)` を満たすことを
      確認する。フェーズ 2 の `TestNewReadOnly_NotADirectory` は `filevalidator` 単体での構築失敗を
      検証するが、dry-run マネージャ生成の入口（`newManagerInternal`）でも同じ理由で構築失敗する
      ことを固定化し、`02_architecture.md` §3.2・§5.3 Q-02 が定める「非ディレクトリは現状どおり
      ハードエラー」という不変条件が本タスクの変更後も保たれることを確認する（AC には対応しないが、
      Q-02 の回帰防止として追加する）。

### PR-3 作成ポイント: verification dry-run core wiring to read-only validator

**対象ステップ**: 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6 / 3-7 / 3-8

**推奨タイトル**: `feat(0148): wire dry-run manager to read-only validator`

**レビュー観点**: `os.ErrPermission` フォールバック削除後に `skipped_no_validator` へ落ちる経路が残っていないか / `SetHashDirStatus` の単一情報源化（`HashDirAvailable()` 由来 vs `opts.fs.FileExists` フォールバック）の分岐条件が正しいか / 旧挙動（自動作成）を前提にしていた既存テスト（`"auto_creates_missing_hash_dir_and_enables_validator"`）の書き換えが新挙動と整合しているか / AC-03〜AC-06 を検証する新規テストが `verifyFile`・`verifyFileWithHash`・`readAndVerifyFileWithReadFallback` の各経路を網羅しているか

**実装モデル要件**: frontier-recommended

**判定理由**: 既存の権限フォールバック分岐を削除し `permission_denied` への分類変更（セキュリティ関連の再分類）と `SetHashDirStatus` の情報源変更を同時に行う統合ステップであり、4 つの呼び出し経路（`verifyFile`／`verifyFileWithHash`／`readAndVerifyFileWithReadFallback`／権限エラー経路）へ一貫して適用しないと `skipped_no_validator` への回帰や `HashDirStatus` の不整合を招く。加えて旧挙動を前提にした既存テスト（ステップ 3-7）の書き換えが必須であり、Risk isolation の対象に該当する。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

- [x] **ステップ 3-9**: `manager_production.go` の `logDryRunManagerCreation`（87-105 行目）の 96 行目
      `"file_validator_enabled", true,` の直後に `"construction_mode", "read_only",` を追加する。
- [x] **ステップ 3-10**: `manager_test.go` 987-991 行目のコメントを次のとおり書き換える。
      - Before:
        ```go
        // TestReadAndVerifyFileWithReadFallback_NoValidator_DryRunRecordsUnverified
        // covers fallback path 1: the file validator is nil (dry-run on a machine
        // where the hash directory is not writable) and the file is read directly via
        // os.ReadFile. The summary must mark the content as UNVERIFIED with the
        // skipped_no_validator reason, even though no failure was recorded.
        ```
      - After:
        ```go
        // TestReadAndVerifyFileWithReadFallback_NoValidator_DryRunRecordsUnverified
        // covers fallback path 1: the file validator is nil (verification is
        // explicitly disabled for this manager instance, e.g. via
        // withFileValidatorDisabledInternal in tests) and the file is read
        // directly via os.ReadFile. The summary must mark the content as
        // UNVERIFIED with the skipped_no_validator reason, even though no
        // failure was recorded.
        ```
- [x] **ステップ 3-11**: `types.go` 149-153 行目のコメントを次のとおり書き換える（値は変更しない）。
      - Before:
        ```go
        // UnverifiedReasonNoValidator indicates the file was adopted because no
        // file validator was configured for this manager instance (e.g. dry-run
        // on a machine where the hash directory is not writable).
        UnverifiedReasonNoValidator UnverifiedReason = "skipped_no_validator"
        ```
      - After:
        ```go
        // UnverifiedReasonNoValidator indicates the file was adopted because no
        // file validator was configured for this manager instance (verification
        // itself is disabled; a missing or unreadable hash directory is reported
        // via FailureReason instead, e.g. ReasonHashDirNotFound).
        UnverifiedReasonNoValidator UnverifiedReason = "skipped_no_validator"
        ```
- [x] **ステップ 3-12**: `manager_production_test.go` の `"dry_run_security_audit_logging"` サブテスト
      （157-178 行目）と `TestDryRunManagerLogging`（231-253 行目）の両方へ、
      `assert.Contains(t, logOutput, "construction_mode=read_only")` を追加する。

### PR-4 作成ポイント: dry-run audit log construction-mode field and stale-comment cleanup

**対象ステップ**: 3-9 / 3-10 / 3-11 / 3-12

**推奨タイトル**: `feat(0148): add construction_mode audit log field for dry-run validator`

**レビュー観点**: `"construction_mode", "read_only"` の追加位置が既存の属性順序・命名規則と整合しているか / 書き換えたコメント（`manager_test.go`・`types.go`）が実装（PR-3 の read-only 化）後の実態と一致しているか / 監査ログのアサーション追加がログ属性名の変更に追従しているか

**実装モデル要件**: standard

**判定理由**: 監査ログへの属性追加とコメント文言の修正のみであり、PR-3 の挙動変更それ自体には依存しない独立した軽微な変更で、未確定の設計判断は無い。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 4: 忠実な E2E テストの新規追加

**対象ファイル**: `cmd/runner/integration_dryrun_verification_test.go`,
`internal/verification/manager.go`

- [x] **ステップ 4-1**: `TestDryRunE2E_HashDirectoryNotFound` を追加する。
      - `hashDir := filepath.Join(tu.SafeTempDir(t), "does-not-exist")`（作成しない）。
      - `configContent` は他の E2E テスト（56-78 行目）と同様、`/bin/echo` を実行する 1 グループの
        最小構成。
      - `configFile := setupTempConfig(t, configContent)`。
      - `cmd := newGoRunCmdWithHashDir(t, hashDir, "-config", configFile, "-dry-run",
        "-dry-run-detail", "full", "-dry-run-format", "json", "-log-level", "error")` で実行し、
        stdout を JSON として `TestDryRunE2E_JSONOutput`（116-162 行目）と同じ
        `FileVerification *verification.FileVerificationSummary` 構造へデコードする。
      - `assert.False(t, result.FileVerification.HashDirStatus.Exists)`。
      - `result.FileVerification.Failures` に `Reason == verification.ReasonHashDirNotFound` の
        エントリが 1 件以上含まれることを確認する（AC-03）。
      - `result.FileVerification.UnverifiedFiles` に
        `Reason == "verify_failed_hash_directory_not_found"` のエントリが 1 件以上含まれることを
        確認する（AC-03）。
      - `assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())`
        （AC-05）。
      - `_, statErr := os.Stat(hashDir)` の後 `assert.True(t, os.IsNotExist(statErr))`
        （AC-01。実行後にディレクトリが作成されていないことを確認する）。
      - dry-run はエラー終了（終了コード 3）するため、コマンド実行時に `*exec.ExitError` が
        返る。`TestDryRunE2E_HashFilesNotFound`（55-78 行目）と同様、この戻り値に対して
        `require.NoError` は行わず、`cmd.ProcessState.ExitCode()` の値のみをアサートする。
- [x] **ステップ 4-2**（PR-3 実装と PR-5 E2E テストの間の発覚した逸脱への追従）:
      ステップ 4-1 の E2E テストを実行したところ、dry-run プレビューは `verifyFile`/
      `verifyFileWithHash`/`readAndVerifyFileWithReadFallback` の共通経路で `ReasonHashDirNotFound`
      を記録できるが、その後 `internal/runner/group_executor.go` のコマンドごとの
      `VerifyCommandDynLibDeps`（`internal/verification/manager.go` `verifyDynLibDeps`）と
      `VerifyCommandShebangInterpreter` が `m.fileValidator.LoadRecord(cmdPath)` を呼び、
      `filevalidator.ErrHashDirNotExist`（`NewReadOnly` が保持する遅延エラー）を
      `fmt.Errorf("failed to load record for ... verification: %w", err)` でラップして返す。
      グループエグゼキュータがこのエラーを fatal として返すため、dry-run がプレビューを出力
      せず終了コード 1（`system_error`）で終了する。これは `02_architecture.md` §5.3 Q-03
      が「dry-run は中断せずプレビューを継続する」と定める挙動、および
      §3.1 が「`verifyDynLibDeps` / `VerifyCommandShebangInterpreter` / `verifyInterpreterHash`
      も `deferredErr` 機構の恩恵を受け、一貫した扱いになる」と定める挙動に対する逸脱で
      ある。本ステップで以下を実装し、計画を記述に合わせて是正する。
      1. `internal/verification/manager.go` `verifyDynLibDeps`（`LoadRecord` 直後）に、
         `errors.Is(err, filevalidator.ErrHashDirNotExist)` の場合に `return nil` を追加
         する（`ErrRecordNotFound` と同じ「no record available」扱い）。「Old schema」
         分岐および最終 `return fmt.Errorf(...)` の前（既存 `ErrRecordNotFound` 分岐の直後）
         に挿入する。`NewReadOnly` の不在および権限系の遅延エラーは dry-run 専用経路でのみ
         発生し、いずれも per-file 検証が `ReasonHashDirNotFound`/`ReasonPermissionDenied`
         として既に忠実に報告しているため、ここで追加ログを出さずに沈黙させてもドライランの
         プレビュー出力に欠落は生じない（設計上の判断、§3.1 を満たす）。
      2. `internal/verification/manager.go` `VerifyCommandShebangInterpreter`
         （`LoadRecord` 直後）にも同様に `errors.Is(err, filevalidator.ErrHashDirNotExist)`
         分岐を追加する。`ErrRecordNotFound` 分岐の直後、既存 `SchemaVersionMismatchError`
         分岐の前に挿入する。
      3. `verifyInterpreterHash` は変更不要。理由は「`Verify` の戻り値の種類」ではなく
         「到達可能性」にある。`verifyInterpreterHash` の 3 つの呼び出し箇所
         （`VerifyCommandShebangInterpreter` 内）はいずれも、上記 2. でガードした
         `LoadRecord` の**成功後**にのみ実行される。ハッシュディレクトリ不在時は
         `LoadRecord` が `ErrHashDirNotExist`（`NewReadOnly` の `deferredErr`）を返し、
         追加した分岐が `return nil` するため、`verifyInterpreterHash` には到達しない。
         したがって `verifyInterpreterHash` 内の `m.fileValidator.Verify` が不在由来の
         `ErrHashDirNotExist` を観測することは実運用上あり得ず、ここへ分岐を追加しても
         デッドコードになる（YAGNI、§4 の重複回避方針と整合）。

### PR-5 作成ポイント: faithful dry-run E2E test for missing hash directory

**対象ステップ**: 4-1 / 4-2

**推奨タイトル**: `test(0148): add faithful E2E test for missing hash directory in dry-run`

**レビュー観点**: `TestDryRunE2E_JSONOutput` など既存 E2E テストと同じヘルパー（`setupTempConfig` / `newGoRunCmdWithHashDir`）を正しく再利用しているか / AC-01・AC-03・AC-05 の 3 つのアサーションが 1 テスト内で漏れなく検証されているか / 実行後にハッシュディレクトリが作成されていないことの確認が確実に行われているか

**実装モデル要件**: standard

**判定理由**: 既存の `TestDryRunE2E_JSONOutput` 等と同じ確立済みのヘルパー・パターンを踏襲するのみで、未確定の設計判断は無い。

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### フェーズ 5: ユーザードキュメント更新

**対象ファイル**: `docs/user/runner_command.md`, `docs/user/runner_command.ja.md`

- [ ] **ステップ 5-1**: `docs/user/runner_command.md` 674 行目を書き換える。
      - Before:
        `- *Environment cause* (no validator configured, e.g., hash directory not writable): reason `skipped_no_validator`.`
      - After:
        ``- *Environment cause* (no validator configured for this manager instance): reason `skipped_no_validator`. A missing or unreadable hash directory is reported as `verify_failed_hash_directory_not_found` or `verify_failed_permission_denied` instead (see below), not `skipped_no_validator`.``
- [ ] **ステップ 5-2**: `docs/user/runner_command.ja.md` 674 行目を書き換える。
      - Before:
        `- *環境起因*（バリデータ未設定、例：ハッシュディレクトリが書き込み不可）: 理由 `skipped_no_validator`。`
      - After:
        ``- *環境起因*（このマネージャインスタンスでバリデータ自体が未設定）: 理由 `skipped_no_validator`。ハッシュディレクトリが不在または読み取り不可の場合は、`skipped_no_validator` ではなく後述の `verify_failed_hash_directory_not_found` や `verify_failed_permission_denied` として報告されます。``
- [ ] **ステップ 5-3**: `docs/user/runner_command.md` の「`verify_files` failures」段落（679-681 行目）の直後、
      「**Syntax**」（683 行目）の直前に、次の見出しと本文を追加する。
      ```markdown
      **Hash directory handling (dry-run)**

      Dry-run never creates the hash directory, even when it does not exist. In
      that case dry-run performs a read-only check only; every file requiring
      verification is reported with the environment-cause reason
      `hash_directory_not_found` (`verify_failed_hash_directory_not_found` for
      content adopted without verification), and the exit code is `3`. This
      applies uniformly to the configuration file, templates, `verify_files`
      entries, and env files. Only the `record` command and production
      execution create the hash directory automatically.
      ```
- [ ] **ステップ 5-4**: `docs/user/runner_command.ja.md` の「`verify_files` の検証失敗」段落（679-681 行目）の直後、
      「**文法**」（683 行目）の直前に、次の見出しと本文を追加する。
      ```markdown
      **ハッシュディレクトリの扱い（dry-run）**

      dry-run はハッシュディレクトリを作成しません。設定されたハッシュディレクトリが存在しない
      場合、dry-run は read-only な確認のみを行い作成を試みません。検証が必要な各ファイルは
      環境起因の理由 `hash_directory_not_found`（採用したが未検証のコンテンツは
      `verify_failed_hash_directory_not_found`）として報告され、終了コードは `3` になります。
      この扱いは設定ファイル・テンプレート・`verify_files`・env ファイルのすべてに共通です。
      ハッシュディレクトリを自動作成するのは `record` コマンドおよび本番実行のみです。
      ```
- [ ] **ステップ 5-5**: 上記追加後、両ファイルの新設見出しが同じ相対位置（`verify_files` の失敗段落の直後、
      Syntax/文法見出しの直前）にあることを目視確認する。

### PR-6 作成ポイント: user documentation for dry-run hash directory handling

**対象ステップ**: 5-1 / 5-2 / 5-3 / 5-4 / 5-5

**推奨タイトル**: `docs(0148): document dry-run hash directory read-only handling`

**レビュー観点**: 英語版・日本語版の追加段落が内容として対応しているか（訳抜け・訳過剰が無いか）/ 674 行目の書き換えが `skipped_no_validator` と `hash_directory_not_found`/`permission_denied` の区別を正確に説明しているか / 新設見出しの挿入位置が両ファイルで一致しているか

**実装モデル要件**: standard

**判定理由**: テキストのみの変更であり、静的検証（`rg` による見出し存在確認と目視レビュー）で十分に検証できる。未確定の設計判断は無い。

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

## 3. 実装順序とマイルストーン

`02_architecture.md` §8 の優先順位（`fileanalysis` → `filevalidator` → `verification` → E2E →
ドキュメント）に従う。下位パッケージへの依存があるため、この順序を変更しない。

| マイルストーン | 内容 | 完了条件 |
|---|---|---|
| M1 | フェーズ 1 完了 | `go test -tags test ./internal/fileanalysis/...` が通る |
| M2 | フェーズ 2 完了 | `go test -tags test ./internal/filevalidator/...` が通る |
| M3 | フェーズ 3 完了 | `go test -tags test ./internal/verification/...` が通る |
| M4 | フェーズ 4 完了 | `go test -tags test ./cmd/runner/...` が通る |
| M5 | フェーズ 5 完了 | ドキュメント差分のレビュー完了 |
| M6 | 全体完了 | `make test && make lint` が通る（グリーンゲート） |

上記マイルストーンはフェーズ完了の目安であり、PR 単位の分割・実装モデル要件・グリーンゲート
条件は下記 §3.2 を正とする（フェーズ 3 は PR-3・PR-4 の 2 本に分割されるため、M3 の
`go test -tags test ./internal/verification/...` は両 PR マージ後に満たされる条件である）。

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6 | `internal/fileanalysis` に `NewStoreReadOnly` を追加（作成しない Store 構築） | standard |
| PR-2 | 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6 / 2-7 / 2-8 / 2-9 / 2-10 | `internal/filevalidator` に遅延エラー状態を持つ `NewReadOnly` を追加 | frontier-recommended |
| PR-3 | 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6 / 3-7 / 3-8 | `internal/verification` の dry-run マネージャ生成を `NewReadOnly` へ切り替え、権限フォールバックを除去 | frontier-recommended |
| PR-4 | 3-9 / 3-10 / 3-11 / 3-12 | 監査ログへ `construction_mode` 属性を追加、関連コメントの陳腐化を解消 | standard |
| PR-5 | 4-1 / 4-2 | 不在ハッシュディレクトリの忠実な E2E テストを新規追加、および `verifyDynLibDeps`/`VerifyCommandShebangInterpreter` の遅延エラーソフトフェイル | standard |
| PR-6 | 5-1 / 5-2 / 5-3 / 5-4 / 5-5 | `docs/user/runner_command.md` / `.ja.md` の dry-run 挙動記述を更新 | standard |

## 4. テスト戦略

- **単体テスト**: フェーズ 1・2・3 で追加する各テストが、`02_architecture.md` §7.1 の単体テスト
  観点（不在時に作成しない・不在/権限不足で `Verify` 系が忠実なエラーを返す・存在時は従来どおり
  成功する）をすべて満たす。
- **統合テスト（E2E）**: フェーズ 4 の `TestDryRunE2E_HashDirectoryNotFound` で、実バイナリ経由の
  終了コードとディレクトリ非作成を検証する。既存の `TestDryRunE2E_AllSuccess` /
  `TestDryRunE2E_NoSideEffects` / `TestDryRunE2E_HashFilesNotFound` は無変更のまま回帰確認する。
- **セキュリティテスト**: `02_architecture.md` §7.3 のとおり、本番実行の hard fail
  （`manager_production_test.go::TestProductionNewManager/"production_constraints_validation"`、
  既存・無変更）と `record` コマンドの自動作成
  （`cmd/record/main_test.go::TestHashDirPermissions_0o700`、既存・無変更）が本タスクの変更後も
  成立することを確認する（AC-09、新規テスト不要）。
- **重複回避**: `verifyDynLibDeps` / `VerifyCommandShebangInterpreter` / `verifyInterpreterHash`
  （コマンドバイナリの dynlib・shebang 検証経路）も `deferredErr` 機構の恩恵を受け、不在
  ハッシュディレクトリでパニックせず一貫した扱いになるが、これらは `01_requirements.md` の
  AC-01〜AC-10 のいずれにも含まれないため、本タスクでは専用テストを追加しない（YAGNI）。

## 5. リスク管理

| リスク | 内容 | 対応 |
|---|---|---|
| 既存テストの回帰見落とし | `manager_production_test.go` の `"auto_creates_missing_hash_dir_and_enables_validator"` のように、旧挙動（自動作成）を前提にアサーションしている既存テストが他にも存在する可能性 | フェーズ 3 完了時に `go test -tags test ./internal/... ./cmd/...` を全体実行し、想定外の失敗がないことを確認する |
| 権限系テストの環境依存 | `chmod 0o000` を用いる新規テスト（フェーズ 2・3）は、root 権限で実行される CI 環境ではアクセス拒否が発生せず意味をなさない | 既存の `validator_error_test.go` の「unreadable directory」テストと同じ制約であり新規リスクではない。CI が root 実行の場合にこれらのテストが偽陽性（誤って成功）になる点は許容し、本タスクのスコープでは対応しない（既存規約の踏襲） |
| `Manager.fs`（`common.FileSystem`）とドライラン用ディレクトリ判定の乖離 | `SetHashDirStatus` の判定元を `opts.fs.FileExists` から `validator.HashDirAvailable()` に変更するため、テストで `opts.fs` にモックを注入していても実際のディスク状態が使われる | 調査の結果、`isDryRun` かつ `fileValidatorEnabled` を両方満たしつつ `opts.fs` のモックとディスク実態を意図的に乖離させて `SetHashDirStatus` を検証している既存テストは無いことを確認済み（`manager_test.go` 全体を `withFSInternal` で検索し、該当テストはいずれも `withFileValidatorDisabledInternal` を併用していた） |

## 6. 実装チェックリスト

- [ ] PR-1 マージ済み（対象ステップ: 1-1 / 1-2 / 1-3 / 1-4 / 1-5 / 1-6）
- [ ] PR-2 マージ済み（対象ステップ: 2-1 / 2-2 / 2-3 / 2-4 / 2-5 / 2-6 / 2-7 / 2-8 / 2-9 / 2-10）
- [ ] PR-3 マージ済み（対象ステップ: 3-1 / 3-2 / 3-3 / 3-4 / 3-5 / 3-6 / 3-7 / 3-8）
- [ ] PR-4 マージ済み（対象ステップ: 3-9 / 3-10 / 3-11 / 3-12）
- [ ] PR-5 マージ済み（対象ステップ: 4-1 / 4-2）
- [ ] PR-6 マージ済み（対象ステップ: 5-1 / 5-2 / 5-3 / 5-4 / 5-5）
- [ ] `make fmt` を実行し、フォーマット差分がない
- [ ] `make test` が通る
- [ ] `make lint` が通る
- [ ] 下記「受け入れ基準の検証」の全項目を満たしている

## 7. 受け入れ基準の検証

| AC | 内容 | 検証方法 | 種別 |
|---|---|---|---|
| AC-01 | 実行後に不在パスが作成されていない | `internal/fileanalysis/file_analysis_store_test.go::TestNewStoreReadOnly_MissingDirectory_ReturnsErrorWithoutCreating`、`internal/filevalidator/validator_test.go::TestNewReadOnly_MissingDirectory_DoesNotCreateDirectory`、`internal/verification/manager_test.go::TestVerifyConfigFile_DryRun_MissingHashDir`、`cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_HashDirectoryNotFound` | test |
| AC-02 | 正常系（記録済み）の非回帰 | `internal/filevalidator/validator_test.go::TestNewReadOnly_ExistingDirectory_VerifiesSuccessfully`（新規）、`cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_AllSuccess`（既存・回帰確認） | test |
| AC-03 | 不在時は `hash_directory_not_found`（格下げなし） | `internal/filevalidator/validator_test.go::TestNewReadOnly_MissingDirectory_VerifyReturnsErrHashDirNotExist`、`internal/verification/manager_test.go::TestVerifyConfigFile_DryRun_MissingHashDir`、`cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_HashDirectoryNotFound` | test |
| AC-04 | `skipped_no_validator` にならない（バリデータは構成される） | `internal/verification/manager_test.go::TestVerifyConfigFile_DryRun_MissingHashDir`（`manager.fileValidator` の非 nil と `UnverifiedFiles[0].Reason` の値を確認）、`internal/verification/manager_permission_test.go::TestNewManagerInternal_DryRun_HashDirParentUnreadable_RecordsPermissionDenied` | test |
| AC-05 | 終了コード 3 | `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_HashDirectoryNotFound` | test |
| AC-06 | `verify_files`・env ファイルも環境起因として扱われる | `internal/verification/manager_test.go::TestVerifyGlobalFiles_DryRun_MissingHashDir_RecordsHashDirNotFound`、`internal/verification/manager_test.go::TestVerifyGroupFiles_DryRun_MissingHashDir_RecordsHashDirNotFound`（`verify_files` 部分を網羅）。env ファイルについては、タスク 0147 の F-007 で `VerifyEnvironmentFile` が削除されて以降、env ファイルの実内容を検証する production 経路自体が存在しない（フェーズ 3 の該当タスク参照）。本タスクは `verifyFile`/`verifyFileWithHash`/`readAndVerifyFileWithReadFallback` という共通経路自体を変更しないため、将来 env ファイル検証が復活した場合に同じ写像が適用されることは設計上保証されるが、これは具体的なテストで実行できる主張ではない | test（`verify_files`）／該当経路なしのため対象外（env ファイル） |
| AC-07 | ドキュメントに非作成・`hash_directory_not_found`（exit 3）が記載されている | `rg -n "Hash directory handling \(dry-run\)" docs/user/runner_command.md`（1 件ヒット）、`rg -n "ハッシュディレクトリの扱い（dry-run）" docs/user/runner_command.ja.md`（1 件ヒット） | static |
| AC-08 | 日本語版と英語版の記述が対応している | 上記 static チェックで両ファイルに新設見出しが存在することを確認したうえで、`git diff` 上で両ファイルの追加段落を読み合わせ、内容が一致することをレビュー時に確認する | static + manual |
| AC-09 | 本番・`record` の不変性 | `internal/verification/manager_production_test.go::TestProductionNewManager`（`"production_constraints_validation"` サブテスト、既存・無変更）、`cmd/record/main_test.go::TestHashDirPermissions_0o700`（既存・無変更） | test |
| AC-10 | 不在ケースの忠実な E2E | `cmd/runner/integration_dryrun_verification_test.go::TestDryRunE2E_HashDirectoryNotFound` | test |

## 8. 次のステップ

- 本実装計画のレビューと承認（`draft` → `approved`）。
- 承認後、フェーズ 1 から順に実装に着手する。
