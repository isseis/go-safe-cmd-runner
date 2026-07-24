# 実装計画: 検証(verify)と使用(open/exec)間の TOCTOU 残存窓を閉じる

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-22 |
| Review date | 2026-07-22 |
| Reviewer | isseis |
| Comments | - |

---

## 1. 実装の全体像

### 目的

5 つの独立した修正（F-001〜F-005）を実装し、既存の主要経路で徹底されている「検証と使用は同一 fd/同一読み取りから行う」という設計原則の適用漏れを埋める。各修正は fail-closed の設計に従い、正常系（改ざんなし）の record/verify/実行フローに副作用を与えない。

### 実装原則

- **既存機能の回帰防止**: 改ざんがない正常系の成功経路は変更しない。既存テストは全て通過する。
- **YAGNI（必要最小限）**: 完全排除が困難な残存窓（F-006）はコード変更ではなく文書化のみとする。
- **DRY（既存部品の再利用）**: 依存解決は既存の `DynLibAnalyzer` / `LibraryResolver` を再利用し、同等の検証機構を再実装しない。

### 既存コード調査結果

本セクションでは、実装に先立ちコードベース内で既に存在する機構を確認した結果を記述する。

#### F-001（openVerifiedIdentity）の既存機構

- `openVerifiedIdentity` 関数は `internal/runner/base/risk/evaluator.go:570-580` に存在し、現状は `syscall.Open(O_RDONLY|O_CLOEXEC)` のみ実行。ハッシュ照合・fstat・O_NONBLOCK は未実装。
- `allowedPlan` 関数（`evaluator.go:542-553`）は `e.openIdentity(cmd)` を呼び出し、失敗時に `ReasonIdentityUnbound` を返す。新規エラーに対応する分岐が必要。
- `risktypes.VerifiedIdentity` 型は既に FD フィールドを持つ（`risktypes/identity.go`）。
- sentinel error は `errors.Is` で判別するプロジェクト方針に従う（CLAUDE.md 記載）。

#### F-002（AtomicMoveFile）の既存機構

- `atomicMoveFileCore` は `internal/safefileio/safe_file.go:140-179` に存在。
- 現状：SafeOpenFile で検証後、`os.Rename(absSrc, absDst)` で移動。
- `ensureParentDirsNoSymlinks` は既に存在し、親ディレクトリの検証に使用可能。
- 呼び出し元は `internal/runner/base/output/file.go:73` のみ（`safeFS.AtomicMoveFile`）。

#### F-003（SaveRecord と解析の一貫性）の既存機構

- `SaveRecord`（`internal/filevalidator/validator.go:365-387`）は `saveRecordCore` を呼び出す。
- `saveRecordCore`（`internal/filevalidator/validator.go:388-411`）内で shebang・ハッシュ・各解析が別々に実行される。
- 現状の入力形式：各解析器はパス名を受け取る。
  - shebang: `shebang.Parse(filePath, fs)` (filePath と safefileio.FileSystem を受け取る)
  - ハッシュ: `calculateHash(filePath string, ...)` (内部関数)
  - ELF dynlib: `binaryanalyzer.AnalyzeNetworkSymbols`, `elfdynlib.Analyzer.Analyze`
  - Mach-O: `machodylib` 関数群（パス名ベース）
  - ELF syscall: `binaryanalyzer.analyzeELFSyscalls`（パス指定で fopen）
  - Mach-O syscall: `binaryanalyzer.analyzeMachoSyscalls`
- io.ReaderAt インタフェースの利用は既存で確立（Go 標準の `debug/elf.NewFile(io.ReaderAt)` などで活用）。
- `analyzeOneLibrary` は `internal/filevalidator/validator.go:713-776` に存在し、解析対象ファイルを Stat して size チェック後、`DynLibAnalyzer` で解析。

#### F-004（VerifyCommandDynLibDeps の依存解決再実行）の既存機構

- `VerifyCommandDynLibDeps` は `internal/verification/manager.go:604-611` に存在。
- 現状の署名：`func (m *Manager) VerifyCommandDynLibDeps(cmdPath string) error`
- 内部で `m.verifyDynLibDeps(cmdPath)` を呼び出し。
- `DynLibAnalyzer` / `LibraryResolver` は既に `internal/dynlib/elfdynlib/` に存在し、`Analyze` メソッドで RUNPATH/`$ORIGIN` を処理。
- Mach-O 依存解決は `internal/dynlib/machodylib/` に存在。
- モック定義は `internal/verification/testutil/testify_mocks.go:32-33` に存在。
- モック期待は `group_executor_test.go` 等で多数使用（20+ 箇所）。

#### F-005（PathResolver.validateAndCacheCommand の順序変更）の既存機構

- `validateAndCacheCommand` は `internal/verification/path_resolver.go:31-53` に存在。
- 現状：`os.Stat`（symlink 追従）で検証→`filepath.EvalSymlinks` で正規化。
- テストは `path_resolver_test.go` に存在。

#### F-006（verifyInterpreterSymlinkTarget の残余リスク文書化）

- 実装対象は現在の `verifyInterpreterSymlinkTarget`（`internal/verification/manager.go:923-`）ではなく、設計/セキュリティ文書への記述のみ。本タスクで対応済み（02_architecture.md § 5.3）。

---

## 2. 実装順序とマイルストーン

### Phase 1（局所・低リスク）

**範囲**: F-005、F-001。単一関数変更で完結。

**マイルストーン 1**: 解決順序の修正（F-005）
- `validateAndCacheCommand` を EvalSymlinks→Lstat 順へ変更
- 既存テスト全て通過を確認

**マイルストーン 2**: ハッシュ再検証とエラー区別の追加（F-001）
- sentinel error 定義
- 新規 reason code 2 個追加（reason codes 表に登録）
- `openVerifiedIdentity` の実装
- `allowedPlan` の分岐追加
- 既存テスト全て通過を確認

### Phase 2（依存解決の再実行）

**範囲**: F-004。署名は不変（`envVars` は追加しない。02_architecture.md § 3.4 参照）。

**マイルストーン 3**: 依存解決再実行と集合比較
- VerifyCommandDynLibDeps 内で依存解決を再実行し record 済み集合と比較する実装
- 既存テスト全て通過を確認

### Phase 3（入力経路の拡張）

**範囲**: F-003。各解析器の入力経路拡張・束縛。

**マイルストーン 4**: 共有 fd への束縛
- SaveRecord 起点での安全な open（SafeOpenFile）
- 各解析器への io.ReaderAt 入力対応
- ハッシュ計算・各解析の一貫性
- store 境界でのハッシュキー照合
- 既存テスト全て通過を確認

### Phase 4（fd アンカー移動）

**範囲**: F-002。Linux 固有処理の追加。

**マイルストーン 5**: linkat ベースの fd アンカー移動
- atomicMoveFileCore の実装変更
- 一時リンク生成・クリーンアップ
- 既存テスト全て通過を確認

### Phase 5（文書化と最終検証）

**範囲**: F-006（既に 02_architecture.md で実施済み）。本計画での追跡のみ。

---

## 3. テスト戦略と PR 構成

### 概要

各 AC に対し、正常系（改ざんなし・回帰防止）と異常系（TOCTOU 窓を突く・fail-closed 確認）の双方のテストを実装する。

### 3.1 PR 構成

本実装計画は 5 つの独立した修正（F-001 〜 F-006）を 5 つの PR に分割して実施する。各 PR は green gate（`make test && make lint`）を独立して通過し、内部層（`internal/`）の変更のみで構成される。

| PR | 対象機能 | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | F-005 / F-001 | `validateAndCacheCommand` の `EvalSymlinks`→`Lstat` 順序変更 / `openVerifiedIdentity` のハッシュ検証・`O_NONBLOCK`・`fstat` 追加 / sentinel error と reason code 定義 | frontier-recommended |
| PR-2 | F-004 | `VerifyCommandDynLibDeps`（署名不変）内での依存解決再実行ロジック / グループ実行系の統合テスト | frontier-recommended |
| PR-3 | F-003 | `SaveRecord` の共有 fd 束縛 / `saveRecordCore` で各解析器へ fd 伝播 / `analyzeOneLibrary` でハッシュキー照合 / store 境界でのハッシュ検証 / 各解析器の `io.ReaderAt` 入力対応 | frontier-recommended |
| PR-4 | F-002 | `atomicMoveFileCore` の `linkat` ベース fd アンカー移動 / 一時リンク生成・クリーンアップ / エラー経路の defer 処理 | frontier-recommended |
| PR-5 | F-006 | 02_architecture.md § 5.3 への F-006 残余リスク文書化（既に実施済み） | standard |

**PR 独立性に関する注記**:

- **PR-3（F-003）と PR-2（F-004）の独立性**: F-003 は各解析器の入力経路を拡張（パス名に加え `io.ReaderAt` を受け入れる）するが、既存のパス名ベース呼び出し規約は変更しない。F-004 の `VerifyCommandDynLibDeps` は verify 時にパス指定で `DynLibAnalyzer.Analyze` / `machodylib` を呼び出すため、既存呼び出し規約で動作し、F-003 がなくても green gate をパスする。したがって PR-2 は PR-3 に依存せず独立して実装できる。
- **各 PR の green gate 独立性**: 全ての PR は内部層（`internal/`）のみを変更し、cmd 層への依存がない。各 PR は既存テスト全て通過を確認して green gate をパスする。

### 3.2 テスト戦略

#### F-001 テスト戦略

- **正常系**（AC-03）: 内容一致時に VerifiedIdentity が返る。既存の `openVerifiedIdentityForTest` 等を活用し、回帰防止。
- **異常系（AC-01,04）**:
  - fd 内容を検証済みハッシュと不一致にする。
  - ハッシュ不一致時に `ErrIdentityHashMismatch` を返し、allowedPlan が `ReasonIdentityHashMismatch` を返す。
  - パスを FIFO に差し替えた場合に `ErrIdentityNotRegular` を返す。
- テスト場所: `internal/runner/base/risk/*_test.go`（既存テストに追加）

#### F-002 テスト戦略

- **正常系**（AC-05, AC-06）: 改ざんなしの移動が成功、内容保持。移動先の inode が SafeOpenFile 時点で取得した fd の inode と一致すること（fd アンカー機構が正しく動作していること）を確認する。すべてのテストファイルは `t.TempDir()` で自動的にクリーンアップされる。
- **異常系（AC-06）ソース差し替えによる fail-closed（実装時に AC-05 から再割当て）**:
  - **実装時判明の帰結**（02_architecture.md 3.2 節・01_requirements.md AC-05 参照）: Linux カーネルは `O_TMPFILE` 以外の方法で nlink が 0 になった（完全に unlink 済みの）ファイルを `/proc/self/fd` 経由で再リンクすることを許可しない。したがって、fd 取得後にソースパスを別 inode へ差し替える（検証済み inode の nlink が 0 になる）と、`linkat` 自体が `ENOENT` で失敗し、rename は行われない。当初の設計意図（差し替え後も検証済み inode への到達に成功する）ではなく、「差し替えられた場合は必ず fail-closed で中断する」ことを検証する試験に改めた。
  - 注入手法：SafeOpenFile で fd を取得後、ソースパスを削除して別ファイルを同名で作成し（検証済み inode の nlink を 0 にする）、`moveFileAnchored` を呼び出す。エラーが返り、移動先にファイルが作成されないこと、元ソースパス（差し替え後の内容）が変更されず残ることを確認する。
- **異常系（クリーンアップ検証）**:
  - linkat 後の rename 失敗で defer による一時リンク unlink が実行されること。
  - 一時名の EEXIST エラーが適切に返されること。
- テスト場所: `internal/safefileio/*_test.go`（既存テストに追加、すべてのテストは `t.TempDir()` を使用）

#### F-003 テスト戦略

- **正常系**（AC-09）: 変更前と同一内容のレコード生成。
- **異常系（AC-07）TOCTOU 注入**:
  - 共有 fd を使用する SaveRecord の実装を検証する。記録中にソースファイルが別 inode へ差し替わった場合でも、記録される ContentHash・解析結果が TOCTOU 発生前の inode 内容に対応することを確認する。
  - 注入手法：テスト用 mock `io.ReaderAt` を作成し、読み取り途中に内容を変更するシミュレーション、または実際のファイルを並行プロセスで置換し、fd が元の内容を読み続けることを確認。
  - 期待動作：共有 fd により、ハッシュと解析結果が必ず同一内容に対応する（別々に open した場合とは異なる）。
- **異常系（AC-08）ハッシュキー照合**:
  - store 境界・analyzeOneLibrary でハッシュキー不一致時に ErrLibraryHashKeyMismatch を返す。
- テスト場所: `internal/filevalidator/*_test.go`・`internal/dynamicanalysis/*_test.go`（既存テストに追加）

#### F-004 テスト戦略

- **正常系**（AC-11）: 環境不変時に verify 成功。
- **異常系**（AC-10）: record 後に高優先探索位置へ新規ライブラリ配置→verify 失敗。構造化エラーが soname・新旧パスを保持することを確認。
- テスト場所: `internal/verification/manager_test.go`・`manager_macho_test.go`（既存テストに追加）

#### F-005 テスト戦略

- **正常系**（AC-13）: 通常構成で解決済みパスが返りキャッシュされる。
- **異常系**（AC-12）: 解決順序により、検証対象とキャッシュ対象が同一解決結果を指す。
- テスト場所: `internal/verification/*_test.go`（既存テストに追加）

#### F-006 テスト戦略

- **静的検証**（AC-14）: 02_architecture.md § 5.3 に (a)(b)(c) が記載されていることを確認。

---

## 4. 実装チェックリスト

### Phase 1: F-005 PathResolver の順序変更

**ファイル**: `internal/verification/path_resolver.go`

- [x] `validateAndCacheCommand` を EvalSymlinks→Lstat 順へ変更
  - `filepath.EvalSymlinks(path)` を最初に実行し canonical パスを得る
  - canonical パスに対して `os.Lstat`（symlink を追従しない）で存在・regular・実行ビットを検証
  - 解決済みパスをキャッシュ（同じ解決結果を参照）
- [x] 既存テスト `path_resolver_test.go` が全て通過

### Phase 1: F-001 openVerifiedIdentity のハッシュ再検証

**ファイル**:
- `internal/runner/base/risk/evaluator.go`
- `internal/runner/base/risktypes/reason_codes.go`

- [x] 新規 sentinel error 定義（`risk` パッケージ）:
  - `var ErrIdentityHashMismatch = errors.New("verified identity content hash mismatch")`
  - `var ErrIdentityNotRegular = errors.New("verified identity is not a regular file")`
- [x] 新規 reason code 定義（`risktypes`）:
  - `ReasonIdentityHashMismatch = "identity_hash_mismatch"`
  - `ReasonIdentityNotRegular = "identity_not_regular_file"`
  - 両コードを `reasonFamilies` マップに `FamilyRuntimeArgument` として登録
  - `totalReasonCodes` を +2 する（テストで検査）
- [x] `openVerifiedIdentity` 実装:
  - `syscall.Open(cmd.ExpandedCmd, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)`
  - `fstat` で通常ファイル確認（FC_ISREG）
  - `O_NONBLOCK` を `fcntl` で解除
  - 同じ fd から `io.NewSectionReader` でハッシュ計算（fd offset 進めず）
  - ハッシュ不一致 → `ErrIdentityHashMismatch` return
  - 非通常ファイル → `ErrIdentityNotRegular` return
- [x] `allowedPlan` に分岐追加:
  - `errors.Is(err, ErrIdentityHashMismatch)` → `ReasonIdentityHashMismatch`
  - `errors.Is(err, ErrIdentityNotRegular)` → `ReasonIdentityNotRegular`
- [x] 既存テスト全て通過（`reason_codes_test.go::TestReasonFamily_AllCodesAssigned` 含む）

### PR-1 作成ポイント: path resolution and identity verification hardening

**対象ステップ**: F-005 / F-001

**推奨タイトル**: `feat(0155): path resolution and identity verification hardening`

**レビュー観点**: `EvalSymlinks` 順序変更の正確性 / ハッシュ検証の実装 / sentinel error の定義と対応付け / reason code の登録と `totalReasonCodes` 更新の確認

**実装モデル要件**: frontier-recommended

**判定理由**: F-001 は file integrity verification（ファイルハッシュ検証）に相当する Conditional trigger にマッチし、AC-01/AC-02 はセキュリティクリティカルな検証ロジックであるため

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/901）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: F-004 VerifyCommandDynLibDeps の依存解決再実行

**ファイル**:
- `internal/verification/manager.go`
- `internal/verification/interfaces.go`

**AC-10 の再検討（envVars を追加しない結論への変更）**: 当初、本フェーズは `VerifyCommandDynLibDeps` に `envVars` を追加し（署名変更）、インターフェース境界で受け入れることで AC-10 の「使用する」を満たす、という設計で実装・レビュー（PR #902）された。しかしレビューで、本タスクの再利用対象である ELF/Mach-O 解決器が環境変数を一切参照しない以上、`envVars` は `Analyze` へ渡されず実質的に未使用の引数になり、これは CLAUDE.md の YAGNI 原則（予定のない将来拡張のために設計しない）に反するとの指摘を受けた。この指摘を受けて AC-10 自体を「`envVars` の受け入れ」を要求しない形に修正し（01_requirements.md 参照）、実装からも `envVars` パラメータを撤去した。将来 `$LIB`/`$PLATFORM` 等の環境依存置換が実際にタスク化された時点で、`Analyze`/`Resolve` および `VerifyCommandDynLibDeps` の署名変更を行う。

- [x] ~~`ManagerInterface.VerifyCommandDynLibDeps` に `envVars` を追加~~ → 撤回。署名は `VerifyCommandDynLibDeps(cmdPath string) error` のまま変更なし
- [x] `Manager.VerifyCommandDynLibDeps` / `verifyDynLibDeps` 実装（`envVars` を持たない）:
  - 依存解決を再実行: `elfDynLibAnalyzer.Analyze(cmdPath)` を実行し、非 ELF（`nil` 戻り）の場合のみ `machoDynLibAnalyzer.Analyze(cmdPath)` にフォールバックする
  - cmdPath 自身が ELF/Mach-O のいずれでもない場合（例: shebang スクリプト。この場合の `DynLibDeps` はスクリプト自身の解決結果ではなく shebang チェーンのインタプリタ解決結果に由来する）は再解決比較をスキップする
  - 記録済み集合と現在の解決集合を **Path** の集合として比較する（`fileanalysis.LibEntry.SOName` は `json:"-"` でレコードに永続化されないため、`LoadRecord` で読み戻した記録済みエントリは常に `SOName` が空になる。設計時に想定していた soname キーでの比較は成立しないため、実装時に Path キー比較へ変更した）
  - 不一致時に構造化エラー `ErrDynLibDepsResolutionChanged`（`internal/verification/errors.go`）を返す。フィールドは記録済みパス・解決済みパス・soname（soname は解決済みエントリ側にのみ実際の値が入り、記録済みエントリ側は前述の理由で空になる)。設計時に想定していた「シャドーイングした探索段階」は含めない（`elfdynlib`/`machodylib` の `Analyze` は解決元の段階（RUNPATH/ld.so.cache/デフォルトパス等）を呼び出し元へ公開しておらず、追加の計装なしには取得できないため。将来必要になった場合は別タスクで `Analyze` の返り値拡張を検討する）
- [x] `envVars` 追加に伴っていた `testutil/testify_mocks.go`・`internal/runner/*_test.go` のモック期待更新（20+ 箇所）は、署名変更自体を撤回したため不要（当該コミットで元に戻した）
- [x] `group_executor.go` 呼び出し元は元の `m.VerifyCommandDynLibDeps(resolvedPath)` のまま変更なし。`finalEnv` は `VerifyCommandShebangInterpreter`（PATH 再解決に実際に環境変数を使う）のためだけに算出する
- [x] 既存テスト全て通過

### Phase 2b（統合テスト）: F-001・F-004 end-to-end 検証

**ファイル**: `internal/runner/e2e_dynlib_verification_test.go`（新規追加。モックベースの `group_executor_test.go`・`integration_dual_defense_test.go` ではなく、実際の `verification.Manager` と実ファイルを用いた e2e テストとして独立ファイルに配置した）

- [x] `TestGroupExecutor_F001_HashMismatchBlocksExecution`:
  - 設計時の想定（`ExpandedCmdContentHash` を直接改変）ではなく、実行順序を利用した決定的な再現に変更した: グループ内の 1 つめのコマンド（実行時に実ファイルとして実行される）が 2 つめのコマンドのバイナリを実際に上書きし、`ExecuteGroup` の「グループ全体のハッシュ検証（step 7）→ 全コマンド実行（step 8）」という順序により、2 つめのコマンドの fd 再検証時点で内容が record 時と乖離する状況を非同期レースなしに再現する
  - ハッシュ不一致により exec が拒否される（`ReasonIdentityHashMismatch` で Blocking。エラーメッセージに reason code 文字列が含まれることを確認）
- [x] `TestGroupExecutor_F004_LibraryShadowingBlocksExecution`:
  - 設計時の想定（record 時点と verify 時点の間に高優先度の依存ライブラリを実配置）ではなく、記録済み `DynLibDeps` から 1 エントリを `store.Update` で除去する手法に変更した。「live resolver が record 時に存在しなかった依存を発見する」という不一致の形は、高優先探索位置への新規ライブラリ出現と同型であり、システム固有のライブラリ配置や ld.so.cache に依存せず決定的に再現できる
  - group execute が依存集合不一致で実行を中止する（エラーメッセージに `dynamic library dependency resolution changed since record` が含まれることを確認）
  - 構造化エラー自体（soname・新旧パス）は `internal/verification/manager_test.go` の `TestVerifyCommandDynLibDeps_ReExecutesDependencyResolution` で個別に検証済み

### PR-2 作成ポイント: dependency resolution re-execution and interface coordination

**対象ステップ**: F-004

**推奨タイトル**: `feat(0155): dependency resolution re-execution with environment awareness`

**レビュー観点**: 依存解決再実行ロジック / 集合比較（Path キー）の正しさ / 構造化エラーの内容（soname・path） / AC-10 の「envVars を追加しない」という結論の妥当性（YAGNI 判断の再確認）

**実装モデル要件**: frontier-recommended

**判定理由**: 依存解決の再実行と集合比較は search-order shadowing 検出の中核ロジックであり、record/verify 環境差異（benign drift）との切り分けが設計上重要な判断であるため

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/902）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: F-003 SaveRecord の共有 fd 束縛

**ファイル**:
- `internal/filevalidator/validator.go`
- `internal/dynamicanalysis/store.go`、`interfaces.go`
- 各解析器（`shebang`、`binaryanalyzer`、`elfdynlib`、`machodylib`）

- [x] `SaveRecord` 実装変更:
  - 対象ファイルを起点で 1 回だけ安全に open（`SafeOpenFile`）
  - fd（`safefileio.File`、`io.ReaderAt` を満たす）を全下流へ渡す
- [x] `saveRecordCore` 実装変更:
  - shebang 解析、ハッシュ計算、各解析へ共有 fd を渡す
- [x] 各解析器の入力経路拡張（全 6 つの解析関数対応。**実装時の変更点**: 既存のパス名ベース関数の**シグネチャを変更**するのではなく、同名+`FromReader`サフィックスの**新規関数/メソッドを追加**し、パス名ベース版はその薄いラッパーとした。理由: `AnalyzeNetworkSymbols`（`binaryanalyzer.BinaryAnalyzer`インタフェース）等はテスト側に約 45+ 箇所の直接呼び出しがあり、既存シグネチャ変更は無関係な広範囲の書き換えを強制するため、DRY を保ちつつ影響範囲を最小化する設計とした。SaveRecord の対象ファイル（トップレベル）のみ共有 fd を使い、shebang chain のインタプリタや dynlib 依存先ライブラリ等の別ファイルは従来通りパス名ベースで個別 open する（要件の「対象ファイル」の範囲外）):
   - `shebang.Parse` → `shebang.ParseFromReader` に統合（`Parse` は本フェーズでの唯一の呼び出し元だったため、ラッパーを残さず `ParseFromReader` へ一本化。`IsShebangScript` は他ファイル向けの経路で存置）
  - `binaryanalyzer.BinaryAnalyzer` インタフェースに `AnalyzeNetworkSymbolsFromReader` を追加（elfanalyzer/machoanalyzer 両実装で対応、既存 `AnalyzeNetworkSymbols` は内部で `SafeOpenFile` 後に委譲）
  - `machoanalyzer.ScanSyscallInfos` → `ScanSyscallInfosFromReader` を追加（`analyzeELFSyscalls`/`analyzeMachoSyscalls`/`getMachoAnalysisInfo` も同様に `...FromReader` 版を追加し `validator.go` 内で dispatch）
  - `elfdynlib.DynLibAnalyzer.Analyze` → `AnalyzeFromReader` を追加（トップレベル ELF の open のみ共有 fd 化。再帰的な依存先ライブラリの open は対象外）
  - `machodylib.MachODynLibAnalyzer.Analyze` → `AnalyzeFromReader` を追加（同上）
- [x] ハッシュ計算実装変更:
  - `saveRecordCore` で共有 fd から `v.algorithm.Sum(file)` により実測ハッシュを計算（専用ラッパー関数は導入せず直接呼び出し）
- [x] `analyzeOneLibrary` 実装変更(**レビュー後の修正**: 初回実装ではハッシュ計算用の open と `binaryAnalyzer`/ELF syscall 解析用の open が別々のままで、「解析に用いた読み取りから実測ハッシュを計算する」という要求を満たしていなかった。レビューで指摘され、`file safefileio.File` 引数（nil 許容）を追加し、非 nil の場合はハッシュ計算・`binaryAnalyzer.AnalyzeNetworkSymbolsFromReader`・`openELFFileFromReader` の全てが同一 fd を読むよう修正した):
  - `file == nil`（store 経由なしの直接呼び出し。呼び出し元 `loadOrAnalyzeLibrary`）の場合は `lib.Path` を 1 回だけ open し、以降の全解析で共有
  - `file != nil`（store 経由。呼び出し元は下記 `LoadOrAnalyzeAndStore`）の場合はその fd を共有
  - 共有 fd から実測ハッシュを計算（`lib.Hash` が非空の場合のみ照合。store 経由では `AnalyzeLibrary` の `LibEntry` 構築時に `Hash` を設定しないため、この照合はスキップされ、代わりに `LoadOrAnalyzeAndStore` 側の照合に委ねる）
  - 不一致時に `ErrLibraryHashKeyMismatch`（`filevalidator` パッケージ内で新規定義）を return
- [x] `dynamicanalysis.Analyzer.AnalyzeLibrary` シグネチャ変更(**レビュー後の追加変更**): `AnalyzeLibrary(libPath string)` → `AnalyzeLibrary(file safefileio.File, libPath string)`。実装は `filevalidator.Validator` のみ、呼び出し元は `LoadOrAnalyzeAndStore` のみのため影響範囲は小さい
- [x] `LoadOrAnalyzeAndStore`（store 境界）実装変更:
  - `libPath` を 1 回だけ open し、その fd から実測ハッシュを計算して期待ハッシュ `libHash` と照合
  - 不一致時は `AnalyzeLibrary` を呼ばずに解析結果を記録しない状態で `ErrLibraryHashKeyMismatch`（`dynamicanalysis` パッケージ内で新規定義。`filevalidator` からの import は循環参照になるため、パッケージごとに別個の sentinel error を定義）を return
  - 一致時は同じ fd を `s.analyzer.AnalyzeLibrary(file, libPath)` に渡して解析を実行(ハッシュ照合と解析が同一の読み取りに対応することを保証)
- [x] 既存テスト全て通過（fake なパス/ハッシュに依存していた `dynamicanalysis`/`filevalidator` のテストは、実ファイルと実ハッシュを使うよう更新。加えて `TestAnalyzeOneLibrary_SharesReadForHashAndAnalysis`・`TestStore_LoadOrAnalyzeAndStore_AnalysisReadsHashVerifiedContent` を新規追加し、ハッシュ照合と解析が同一 fd/同一内容に対応することを検証）

### PR-3 作成ポイント: shared fd binding for record content consistency

**対象ステップ**: F-003

**推奨タイトル**: `feat(0155): shared fd binding for record content consistency`

**レビュー観点**: 共有 fd の各解析器への伝播 / ハッシュ計算と解析の一貫性検証 / `io.ReaderAt` の正確な使用方法 / ライブラリハッシュキー照合の実装位置と境界の妥当性

**実装モデル要件**: frontier-recommended

**判定理由**: F-003 は file integrity verification（内容一貫性）に相当し AC-07/AC-08 はセキュリティクリティカルであり、TOCTOU 注入テストによる検証が必須であるため

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/903）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: F-002 atomicMoveFileCore の fd アンカー移動

**ファイル**: `internal/safefileio/safe_file.go`

- [x] `atomicMoveFileCore` 実装変更:
  - 検証済み fd から `linkat(/proc/self/fd/<n>, dstdir/tmpname, AT_SYMLINK_FOLLOW)` を実行
  - 一時名からの `os.Rename(tmpname, finalname)` を同一ディレクトリ内で実行
  - 失敗経路で defer による一時リンク unlink
  - ソースパスの unlink（移動完了後）
  - 実装は `internal/safefileio/safe_file_linux.go`（`moveFileAnchored`/`linkFileToTempName`/`randomTempName`）に配置し、`atomicMoveFileCore`（`safe_file.go`）からプラットフォーム非依存に呼び出す。非 Linux 版は `safe_file_nonlinux.go` で従来の `os.Rename` にフォールバック
  - **実装時判明（02_architecture.md 3.2 節・01_requirements.md AC-05 参照）**: ソースパスが検証後に差し替えられ検証済み inode の nlink が 0 になった場合、Linux カーネルの制約（`O_TMPFILE` 以外の unlink 済みファイルは `/proc/self/fd` 経由で再リンクできない）により `linkat` は `ENOENT` で失敗する。これは fail-closed（AC-06）として扱い、当初 AC-05 が想定した「差し替え後も検証済み inode への到達に成功する」動作は行わない
- [x] 移動先最終検証の実装（既存の `dstFile` 検証ロジックを維持。fd アンカー方式への変更後も move 後の `SafeOpenFile`+`canSafelyAccessFile` による最終検証は不変）
- [x] 既存テスト全て通過

### PR-4 作成ポイント: fd-anchored atomic file movement

**対象ステップ**: F-002

**推奨タイトル**: `feat(0155): fd-anchored atomic file movement with linkat`

**レビュー観点**: `linkat` の安全性とクリーンアップロジック / エラーモード処理（EEXIST/EPERM/ETXTBSY） / 一時リンクリーク防止の defer 実装 / unlink 失敗時のセマンティクス

**実装モデル要件**: frontier-recommended

**判定理由**: F-002 は file path handling（パス検証）と file operation（ファイル移動）に相当し、Linux 固有の複雑な状態遷移と新規エラーモード（`protected_hardlinks`、`EEXIST` による予測可能性への対抗）を含むため

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/905）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: F-006 の追跡

- [x] 02_architecture.md § 5.3 に (a)(b)(c) が記載されていることを確認（本タスク前で既に実施）

### PR-5 作成ポイント: residual risk documentation

**対象ステップ**: F-006

**推奨タイトル**: `docs(0155): document residual TOCTOU risks in interpreter resolution`

**レビュー観点**: 完全排除困難な理由の明確性 / 悪用の前提条件の正確性 / 許容判定の根拠の妥当性

**実装モデル要件**: standard

**判定理由**: コード変更なしの文書化のみであり、残余リスクは 02_architecture.md § 5.3 に既に記載済み

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（https://github.com/isseis/go-safe-cmd-runner/pull/907）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 5. 受入基準検証

各 AC の検証方法を以下に記述する。

| AC | 検証タイプ | 検証方法 | 期待結果 |
|---|---|---|---|
| **AC-01** | test | `internal/runner/base/risk/*_test.go` に新規テスト追加。`openVerifiedIdentity` が fd 内容のハッシュを検証済みハッシュと照合し、不一致時に `ErrIdentityHashMismatch` を返すことを確認 | テスト通過 |
| **AC-02** | test | `internal/runner/base/risk/*_test.go` に新規テスト追加。path を FIFO に差し替えた場合に `ErrIdentityNotRegular` を返し、`O_NONBLOCK` が open ブロックを防ぎ、`fstat` による検査が有効であることを確認 | テスト通過 |
| **AC-03** | test | `internal/runner/base/executor/stagefromfd_test.go` の既存テスト `openVerifiedIdentityForTest` 等のテストが変更前と同じ結果を返すことを確認（回帰防止） | 既存テスト全て通過 |
| **AC-04** | test | `internal/runner/base/risk/*_test.go` に新規テスト追加。`ErrIdentityHashMismatch` / `ErrIdentityNotRegular` が適切に `ReasonIdentityHashMismatch` / `ReasonIdentityNotRegular` に対応付けられ、audit ログに記録されることを確認 | テスト通過 |
| **AC-05** | test | `internal/safefileio/*_test.go` に新規テスト追加。改ざんがない場合に移動先の inode が SafeOpenFile 時点の fd の inode と一致すること（fd アンカー機構）を確認。差し替え時の帰結（fail-closed）は AC-06 で検証（01_requirements.md AC-05 のカーネル制約に関する記述を参照） | テスト通過 |
| **AC-06** | test | `internal/safefileio/*_test.go` に新規テスト追加。検証後に source path を別 inode へ差し替えると `linkat` が `ENOENT` で失敗し、rename が行われずエラーが返る（fail-closed）ことを確認。加えて既存テスト `AtomicMoveFile` 系が成功し、改ざんなし時の移動が後退を持たないことを確認 | テスト通過・既存テスト全て通過 |
| **AC-07** | test | `internal/filevalidator/*_test.go` に新規テスト追加。`SaveRecord` で共有 fd が各解析へ束縛され、読み取り後の差し替えモデルで `ContentHash` と解析結果が同一 inode の同一読み取りに対応することを確認 | テスト通過 |
| **AC-08** | test | `internal/filevalidator/*_test.go` と `internal/dynamicanalysis/*_test.go` に新規テスト追加。`analyzeOneLibrary` と `LoadOrAnalyzeAndStore` (store 境界) でハッシュキー不一致時に `ErrLibraryHashKeyMismatch` を返すことを確認 | テスト通過 |
| **AC-09** | test | 既存テスト `SaveRecord` 系のテストが変更前と同じ record を生成すること（回帰防止）を確認 | 既存テスト全て通過 |
| **AC-10** | test | `internal/verification/manager_test.go` に新規テスト追加。`VerifyCommandDynLibDeps` が依存解決を再実行し、record 後に高優先位置に新規ライブラリが置かれた場合に verify が失敗することを確認。構造化エラーが soname・新旧パス・シャドーイング段階を保持することを確認 | テスト通過 |
| **AC-11** | test | `internal/verification/manager_test.go` に新規テスト追加。環境不変時に `VerifyCommandDynLibDeps` が成功することを確認 | テスト通過 |
| **AC-12** | test | `internal/verification/*_test.go` に新規テスト追加。`validateAndCacheCommand` が `EvalSymlinks` を検証より先に実行し、検証対象と キャッシュ対象が同一解決結果を指すことを確認 | テスト通過 |
| **AC-13** | test | 既存テスト `PathResolver` 系のテストが変更前と同じ解決済みパスをキャッシュすること（回帰防止）を確認 | 既存テスト全て通過 |
| **AC-14** | static | `rg -n "完全排除が困難" docs/tasks/0155_toctou_verify_use_residual_gaps/02_architecture.md` | マッチして記載が確認される |

---

## 6. 受け入れ基準の追跡

### AC-01: openVerifiedIdentity のハッシュ再検証

**実装ステップ**:
- PR-1 での openVerifiedIdentity ハッシュ検証実装

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestOpenVerifiedIdentity_HashMismatchReturnsError`
- 検証内容: fd 内容を検証済みハッシュと不一致にしたとき、`ErrIdentityHashMismatch` を返す

---

### AC-02: 通常ファイル確認と O_NONBLOCK

**実装ステップ**:
- PR-1 での openVerifiedIdentity に fstat と O_NONBLOCK 追加

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestOpenVerifiedIdentity_FIFODetection`（O_NONBLOCK により無期限ブロックしないことも同テスト内で確認）
- 検証内容: path を FIFO に差し替えたとき `ErrIdentityNotRegular` を返す

---

### AC-03: 正常系の回帰防止（openVerifiedIdentity）

**実装ステップ**:
- PR-1 での openVerifiedIdentity ハッシュ検証実装

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestOpenVerifiedIdentity_SuccessReturnsVerifiedIdentity`
- 検証内容: 改ざんなし時に VerifiedIdentity が返る

---

### AC-04: reason code の区別可能性

**実装ステップ**:
- PR-1 での reason code 定義と allowedPlan での分岐追加

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestAllowedPlan_ReturnsDistinctReasonCodes`
- 検証内容: `ReasonIdentityHashMismatch` と `ReasonIdentityNotRegular` が適切に返される

---

### AC-05: fd アンカー移動による同一性保証

**実装ステップ**:
- PR-4 での atomicMoveFileCore 実装変更（`moveFileAnchored`、Linux 実装は `safe_file_linux.go`）

**検証テスト**:
- テスト場所: `internal/safefileio/safe_file_linux_test.go` (新規テスト追加)
- テスト名: `TestMoveFileAnchored_RegressionSuccessfulMove`
- 検証内容: 改ざんがない場合、移動先の inode が SafeOpenFile 時点で取得した fd の inode と一致すること（fd アンカー機構が実際に機能していること）
- **注（実装時判明）**: 「差し替え後も検証済み inode への到達に成功する」という当初の AC-05 試験意図は、Linux カーネルの制約（`O_TMPFILE` 以外の unlink 済みファイルは `/proc/self/fd` 経由で再リンクできない）により実現できないと判明した。差し替え時の挙動は AC-06 のテストで検証する（01_requirements.md AC-05 参照）。

---

### AC-06: 移動失敗時の fail-closed

**実装ステップ**:
- PR-4 での atomicMoveFileCore 実装変更（`moveFileAnchored`）

**検証テスト**:
- テスト場所: `internal/safefileio/safe_file_linux_test.go` (新規テスト追加)
- テスト名: `TestMoveFileAnchored_SourceReplacementFailsClosed`（fd 取得後にソースパスを別 inode へ差し替えると `linkat` が `ENOENT` で失敗し、rename が行われずエラーが返ることを確認）、`TestMoveFileAnchored_RenameFailureCleansUpTemporaryLink`（rename 失敗時に一時ハードリンクがリークしないことを確認）
- 検証内容: 同一性が確認できない場合（ソース差し替え、または rename 失敗）に rename を行わずエラーを返す。改ざんなし時の移動成功は `TestMoveFileAnchored_RegressionSuccessfulMove` で確認（既存パスの回帰なし）

---

### AC-07: 共有 fd への束縛（ハッシュと解析の一貫性）

**実装ステップ**:
- PR-3 での SaveRecord 実装・各解析器の拡張

**検証テスト**:
- テスト場所: `internal/filevalidator/validator_test.go` (新規テスト追加)
- テスト名: `TestSaveRecord_SharedFDBindsHashAndAnalysis`
- 検証内容: 共有 fd から計算されたハッシュと解析結果が同一 inode の同一読み取りに対応

---

### AC-08: ライブラリハッシュキー照合

**実装ステップ**:
- PR-3 での analyzeOneLibrary と store.LoadOrAnalyzeAndStore の実装変更

**検証テスト**:
- テスト場所: `internal/filevalidator/validator_library_analysis_test.go` (新規テスト追加) と `internal/dynamicanalysis/store_test.go` (新規テスト追加)
- テスト名: `TestAnalyzeOneLibrary_HashKeyMismatchError`, `TestStore_LoadOrAnalyzeAndStore_HashKeyMismatchError`, `TestAnalyzeOneLibrary_SharesReadForHashAndAnalysis`, `TestStore_LoadOrAnalyzeAndStore_AnalysisReadsHashVerifiedContent`
- 検証内容: ハッシュキー不一致時に `ErrLibraryHashKeyMismatch` を返す。加えて、ハッシュ照合と解析が同一 fd・同一内容から行われることを検証(前者2つは不一致時のfail-closed、後者2つは一致時に共有read/同一内容を確認)

---

### AC-09: record 正常系の回帰防止

**実装ステップ**:
- PR-3 での SaveRecord 実装・各解析器の拡張

**検証テスト**:
- テスト場所: `internal/filevalidator/validator_shebang_test.go` 等（既存テスト維持）
- テスト名: `TestSaveRecord_ShebangDirect`, `TestSaveRecord_ShebangELF` 等
- 検証内容: 改ざんなし時に record 内容が変わらない

---

### AC-10: verify 時の依存解決再実行

**実装ステップ**:
- PR-2 での VerifyCommandDynLibDeps と verifyDynLibDeps の実装変更

**検証テスト**:
- テスト場所: `internal/verification/manager_test.go` (新規テスト追加)
- テスト名: `TestVerifyCommandDynLibDeps_ReExecutesDependencyResolution`
- 検証内容: 依存解決が再実行され、record 後に高優先位置に新規ライブラリが置かれた場合に verify が失敗する

---

### AC-11: verify 正常系の回帰防止

**実装ステップ**:
- PR-2 での VerifyCommandDynLibDeps と verifyDynLibDeps の実装変更

**検証テスト**:
- テスト場所: `internal/verification/manager_test.go` (既存テスト維持)
- テスト名: `TestVerifyCommandDynLibDeps_SucceedsWhenDependenciesUnchanged`
- 検証内容: 環境不変時に verify が成功する

---

### AC-12: validateAndCacheCommand の順序修正

**実装ステップ**:
- PR-1 での validateAndCacheCommand 順序変更

**検証テスト**:
- テスト場所: `internal/verification/path_resolver_test.go` (新規テスト追加)
- テスト名: `TestPathResolver_ValidateAndCacheCommand` の `validates_and_caches_fully_resolved_path` / `rejects_symlink_resolving_to_non_executable_file` サブテスト
- 検証内容: EvalSymlinks 実行後、解決済みパスに対して Lstat 検証が行われる

---

### AC-13: validateAndCacheCommand 正常系の回帰防止

**実装ステップ**:
- PR-1 での validateAndCacheCommand 順序変更

**検証テスト**:
- テスト場所: `internal/verification/path_resolver_test.go` (既存テスト維持)
- テスト名: `TestPathResolver_ValidateAndCacheCommand` の `successful_validation_and_caching` サブテスト 等
- 検証内容: 改ざんなし時に解決済みパスが返りキャッシュされる

---

### AC-14: F-006 残余リスクの文書化

**実装ステップ**:
- 02_architecture.md § 5.3 への記載（本タスク前で既に実施）

**検証方法**: 静的確認
- コマンド: `rg -n "完全排除が困難である理由" docs/tasks/0155_toctou_verify_use_residual_gaps/02_architecture.md`
- 期待結果: § 5.3 に (a)(b)(c) が記載されている

---

### 6.1 PR 完了トラッキング

各 PR の実装状況を以下で追跡する。

- [x] PR-1 マージ済み（対象機能: F-005 / F-001）
- [x] PR-2 マージ済み（対象機能: F-004）
- [x] PR-3 マージ済み（対象機能: F-003）
- [x] PR-4 マージ済み（対象機能: F-002）
- [x] PR-5 マージ済み（対象機能: F-006）

---

## 7. クロス検索チェックリスト

以下のシンボルの全インスタンスが `make test && make lint` で検査されない可能性があるため、削除・リネーム時にはクロス検索が必要。

| シンボル | 対象ファイル | 検索パターン | 期待結果 |
|---|---|---|---|
| `ErrIdentityHashMismatch` | 全ファイル | `rg -n "ErrIdentityHashMismatch" internal/ cmd/` | allowedPlan での使用のみ（F-001 で追加） |
| `ErrIdentityNotRegular` | 全ファイル | `rg -n "ErrIdentityNotRegular" internal/ cmd/` | allowedPlan での使用のみ（F-001 で追加） |
| `ReasonIdentityHashMismatch` | 全ファイル | `rg -n "ReasonIdentityHashMismatch" internal/` | reason code 定義と allowedPlan での使用（F-001 で追加） |
| `ReasonIdentityNotRegular` | 全ファイル | `rg -n "ReasonIdentityNotRegular" internal/` | reason code 定義と allowedPlan での使用（F-001 で追加） |
| `ErrLibraryHashKeyMismatch` | 全ファイル | `rg -n "ErrLibraryHashKeyMismatch" internal/` | analyzeOneLibrary と store.LoadOrAnalyzeAndStore での定義・使用（F-003 で追加） |
| `linkat` 呼び出し | 全ファイル | `rg -n "linkat" internal/ cmd/` | atomicMoveFileCore での使用のみ（F-002 で追加） |
| `VerifyCommandDynLibDeps` 署名呼び出し | 全ファイル | `rg -n "VerifyCommandDynLibDeps\(" internal/ cmd/ --type go \| grep -v "_test.go"` | group_executor.go のみ（本タスク外の呼び出しなし） |
| `VerifyCommandDynLibDeps` テストモック期待 | テストファイル | `rg -n "\.On\(\"VerifyCommandDynLibDeps" internal/ --type go` | 全ての `.On` 期待が単一引数（`cmdPath` のみ）。`envVars` は受け取らない |

---

## 8. 次のステップ

以下は本実装計画作成後の進行ステップであり、本計画自体の進化に含まれない。

1. 本実装計画（`03_implementation_plan.md`）のレビュー・承認待機
2. Phase 1 以降の実装に従事
3. 各 phase 完了ごとに `make test && make lint` による検証
4. 全 AC の検証テスト完了
5. 実装計画ドキュメント状態を `approved` へ更新
6. 実装ブランチを PR 作成・レビュー・マージ
