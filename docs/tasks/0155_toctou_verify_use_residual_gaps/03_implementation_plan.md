# 実装計画: 検証(verify)と使用(open/exec)間の TOCTOU 残存窓を閉じる

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-26 |
| Review date | - |
| Reviewer | - |
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
  - shebang: `shebang.ResolveInterpreter(scriptPath, ...)`
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

### Phase 2（署名変更を伴う）

**範囲**: F-004。インターフェース・モック・呼び出し元・テスト追従。

**マイルストーン 3**: envVars 受け入れと依存解決再実行
- ManagerInterface 署名変更
- VerifyCommandDynLibDeps 実装
- モック定義・期待値の更新（20+ 箇所）
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

## 3. テスト戦略

### 概要

各 AC に対し、**正常系（改ざんなし・回帰防止）**と**異常系（TOCTOU 窓を突く・fail-closed 確認）**の双方のテストを実装する。

### F-001 テスト戦略

- **正常系**（AC-03）: 内容一致時に VerifiedIdentity が返る。既存の `openVerifiedIdentityForTest` 等を活用し、回帰防止。
- **異常系（AC-01,04）**:
  - fd 内容を検証済みハッシュと不一致にする。
  - ハッシュ不一致時に `ErrIdentityHashMismatch` を返し、allowedPlan が `ReasonIdentityHashMismatch` を返す。
  - パスを FIFO に差し替えた場合に `ErrIdentityNotRegular` を返す。
- テスト場所: `internal/runner/base/risk/*_test.go`（既存テストに追加）

### F-002 テスト戦略

- **正常系**（AC-06）: 改ざんなしの移動が成功、内容保持。すべてのテストファイルは `t.TempDir()` で自動的にクリーンアップされる。
- **異常系（AC-05）fd アンカー検証**:
  - 検証後にソースパスを別 inode へ差し替えても、移動されるのが検証済み inode であることを確認（fd アンカー効果）。
  - 注入手法：SafeOpenFile で fd を取得後、ソースパスの内容を別ファイルで置換してから atomicMoveFileCore を呼び出す。移動先に到達したファイルの inode が SafeOpenFile 時点のものと一致することを fstat で確認。
- **異常系（クリーンアップ検証）**:
  - linkat 後の rename 失敗で defer による一時リンク unlink が実行されること。
  - 一時名の EEXIST エラーが適切に返されること。
- テスト場所: `internal/safefileio/*_test.go`（既存テストに追加、すべてのテストは `t.TempDir()` を使用）

### F-003 テスト戦略

- **正常系**（AC-09）: 変更前と同一内容のレコード生成。
- **異常系（AC-07）TOCTOU 注入**:
  - 共有 fd を使用する SaveRecord の実装を検証する。記録中にソースファイルが別 inode へ差し替わった場合でも、記録される ContentHash・解析結果が TOCTOU 発生前の inode 内容に対応することを確認する。
  - 注入手法：テスト用 mock `io.ReaderAt` を作成し、読み取り途中に内容を変更するシミュレーション、または実際のファイルを並行プロセスで置換し、fd が元の内容を読み続けることを確認。
  - 期待動作：共有 fd により、ハッシュと解析結果が必ず同一内容に対応する（別々に open した場合とは異なる）。
- **異常系（AC-08）ハッシュキー照合**:
  - store 境界・analyzeOneLibrary でハッシュキー不一致時に ErrLibraryHashKeyMismatch を返す。
- テスト場所: `internal/filevalidator/*_test.go`・`internal/dynamicanalysis/*_test.go`（既存テストに追加）

### F-004 テスト戦略

- **正常系**（AC-11）: 環境不変時に verify 成功。
- **異常系**（AC-10）: record 後に高優先探索位置へ新規ライブラリ配置→verify 失敗。構造化エラーが soname・新旧パスを保持することを確認。
- テスト場所: `internal/verification/manager_test.go`・`manager_macho_test.go`（既存テストに追加）

### F-005 テスト戦略

- **正常系**（AC-13）: 通常構成で解決済みパスが返りキャッシュされる。
- **異常系**（AC-12）: 解決順序により、検証対象とキャッシュ対象が同一解決結果を指す。
- テスト場所: `internal/verification/*_test.go`（既存テストに追加）

### F-006 テスト戦略

- **静的検証**（AC-14）: 02_architecture.md § 5.3 に (a)(b)(c) が記載されていることを確認。

---

## 4. 実装チェックリスト

### Phase 1: F-005 PathResolver の順序変更

**ファイル**: `internal/verification/path_resolver.go`

- [ ] `validateAndCacheCommand` を EvalSymlinks→Lstat 順へ変更
  - `filepath.EvalSymlinks(path)` を最初に実行し canonical パスを得る
  - canonical パスに対して `os.Lstat`（symlink を追従しない）で存在・regular・実行ビットを検証
  - 解決済みパスをキャッシュ（同じ解決結果を参照）
- [ ] 既存テスト `path_resolver_test.go` が全て通過

### Phase 1: F-001 openVerifiedIdentity のハッシュ再検証

**ファイル**:
- `internal/runner/base/risk/evaluator.go`
- `internal/runner/base/risktypes/reason_codes.go`

- [ ] 新規 sentinel error 定義（`risk` パッケージ）:
  - `var ErrIdentityHashMismatch = errors.New("verified identity content hash mismatch")`
  - `var ErrIdentityNotRegular = errors.New("verified identity is not a regular file")`
- [ ] 新規 reason code 定義（`risktypes`）:
  - `ReasonIdentityHashMismatch = "identity_hash_mismatch"`
  - `ReasonIdentityNotRegular = "identity_not_regular_file"`
  - 両コードを `reasonFamilies` マップに `FamilyRuntimeArgument` として登録
  - `totalReasonCodes` を +2 する（テストで検査）
- [ ] `openVerifiedIdentity` 実装:
  - `syscall.Open(cmd.ExpandedCmd, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)`
  - `fstat` で通常ファイル確認（FC_ISREG）
  - `O_NONBLOCK` を `fcntl` で解除
  - 同じ fd から `io.NewSectionReader` でハッシュ計算（fd offset 進めず）
  - ハッシュ不一致 → `ErrIdentityHashMismatch` return
  - 非通常ファイル → `ErrIdentityNotRegular` return
- [ ] `allowedPlan` に分岐追加:
  - `errors.Is(err, ErrIdentityHashMismatch)` → `ReasonIdentityHashMismatch`
  - `errors.Is(err, ErrIdentityNotRegular)` → `ReasonIdentityNotRegular`
- [ ] 既存テスト全て通過（`reason_codes_test.go::TestReasonFamily_AllCodesAssigned` 含む）

### Phase 2: F-004 VerifyCommandDynLibDeps の envVars 追加

**ファイル**:
- `internal/verification/manager.go`
- `internal/verification/interfaces.go`
- `internal/verification/testutil/testify_mocks.go`
- `internal/runner/*_test.go`（モック期待更新）

**AC-10 に関する設計上の注記**: 02_architecture.md § 3.4 で明示されたとおり、本タスクで再利用する ELF/Mach-O 解決器は環境変数を参照しない（環境非依存）。したがって `envVars` は「VerifyCommandDynLibDeps インターフェース境界で受け入れる」ことで AC-10 の「受け入れ」を満たす。現状の解決結果が環境非依存である点は、設計上の坦直な位置づけであり、環境依存の置換（例：将来 `$LIB`/`$PLATFORM` 対応）が必要になった場合は、別タスクで `Analyze`/`Resolve` 署名変更を伴う拡張を行う。本タスク実装時に AC-10 の「使用する」の解釈を明示すること。

- [ ] `ManagerInterface.VerifyCommandDynLibDeps` 署名変更:
  - 変更前: `VerifyCommandDynLibDeps(cmdPath string) error`
  - 変更後: `VerifyCommandDynLibDeps(cmdPath string, envVars map[string]string) error`
- [ ] `Manager.VerifyCommandDynLibDeps` 実装:
  - `verifyDynLibDeps(cmdPath, envVars)` 呼び出し（新規引数追加）
  - `envVars` を呼び出し元から受け取り、解決層へ渡す（現状は ELF/Mach-O 解決器が環境非依存のため、実際の解決結果に影響しない）
- [ ] `verifyDynLibDeps` 実装:
  - 依存解決を再実行（`DynLibAnalyzer.Analyze`、`machodylib` 関数に `envVars` を渡す）
  - 記録済み集合と現在の解決集合を比較
  - 不一致時に構造化エラーを返す（soname・新旧パス・シャドーイング段階を含む）
- [ ] モック定義更新（`testutil/testify_mocks.go`）:
  - `On("VerifyCommandDynLibDeps", cmdPath string, envVars map[string]string)` 対応
- [ ] モック期待更新（全 call sites を確認）:
  - 以下のファイルで `.On("VerifyCommandDynLibDeps", mock.Anything)` → `.On("VerifyCommandDynLibDeps", mock.Anything, mock.Anything)` に変更:
    - `internal/runner/group_executor_test.go` (5+ 箇所)
    - `internal/runner/integration_dual_defense_test.go` (4+ 箇所)
    - `internal/runner/e2e_slack_redaction_test.go` (2+ 箇所)
    - `internal/runner/command_output_capture_test.go` (2+ 箇所)
    - `internal/verification/manager_test.go` (1+ 箇所)
    - `internal/verification/manager_macho_test.go` (1+ 箇所)
    - `internal/verification/shebang_chain_verifier_test.go` (1+ 箇所)
    - `internal/verification/testutil/testify_mocks_test.go` (1+ 箇所)
- [ ] `group_executor.go` 呼び出し元の実装変更:
  - `finalEnv` の算出を `VerifyCommandDynLibDeps` 呼び出し前へ移動
  - `m.VerifyCommandDynLibDeps(cmdPath, finalEnv)` に変更
- [ ] 既存テスト全て通過

### Phase 2b（統合テスト）: F-001・F-004 end-to-end 検証

**ファイル**: `internal/runner/group_executor_test.go`・`internal/runner/integration_dual_defense_test.go` 等

- [ ] `TestGroupExecutor_F001_HashMismatchBlocksExecution`:
  - バイナリの `ExpandedCmdContentHash` を変更してから verify・group execute を実行
  - ハッシュ不一致により exec が拒否される（`ReasonIdentityHashMismatch` で Blocking）
- [ ] `TestGroupExecutor_F004_LibraryShadowingBlocksExecution`:
  - record 時点と verify 時点の間に高優先度の依存ライブラリを追加
  - group execute が依存集合不一致で実行を中止する
  - reason code が構造化エラー（soname・新旧パス含む）として記録される

### Phase 3: F-003 SaveRecord の共有 fd 束縛

**ファイル**:
- `internal/filevalidator/validator.go`
- `internal/dynamicanalysis/store.go`、`interfaces.go`
- 各解析器（`shebang`、`binaryanalyzer`、`elfdynlib`、`machodylib`）

- [ ] `SaveRecord` 実装変更:
  - 対象ファイルを `SaveFile` 起点で 1 回だけ安全に open（`SafeOpenFile`）
  - fd を `io.ReaderAt` として全下流へ渡す
- [ ] `saveRecordCore` 実装変更:
  - shebang 解析、ハッシュ計算、各解析へ共有 fd を渡す
- [ ] 各解析器の入力経路拡張:
  - `shebang.ResolveInterpreter`: パス名に加え `io.ReaderAt` 入力対応
  - `binaryanalyzer`: パス名に加え `io.ReaderAt` 入力対応
  - `elfdynlib.Analyzer.Analyze`: `io.ReaderAt` 入力対応
  - `machodylib` 関数: `io.ReaderAt` 入力対応
- [ ] `calculateHash` 実装変更:
  - `io.ReaderAt` から実測ハッシュを計算
- [ ] `analyzeOneLibrary` 実装変更:
  - 解析用 fd から実測ハッシュを計算
  - `lib.Hash` との照合を追加
  - 不一致時に `ErrLibraryHashKeyMismatch` return
- [ ] `LoadOrAnalyzeAndStore`（store 境界）実装変更:
  - 期待ハッシュ `libHash` と実測ハッシュを照合
  - 不一致時に解析結果を記録せず `ErrLibraryHashKeyMismatch` return
- [ ] 既存テスト全て通過

### Phase 4: F-002 atomicMoveFileCore の fd アンカー移動

**ファイル**: `internal/safefileio/safe_file.go`

- [ ] `atomicMoveFileCore` 実装変更:
  - 検証済み fd から `linkat(/proc/self/fd/<n>, dstdir/tmpname, AT_SYMLINK_FOLLOW)` を実行
  - 一時名からの `os.Rename(tmpname, finalname)` を同一ディレクトリ内で実行
  - 失敗経路で defer による一時リンク unlink
  - ソースパスの unlink（移動完了後）
- [ ] 移動先最終検証の実装
- [ ] 既存テスト全て通過

### Phase 5: F-006 の追跡

- [ ] 02_architecture.md § 5.3 に (a)(b)(c) が記載されていることを確認（本タスク前で既に実施）

---

## 5. 受入基準検証

各 AC の検証方法を以下に記述する。

| AC | 検証タイプ | 検証方法 | 期待結果 |
|---|---|---|---|
| **AC-01** | test | `internal/runner/base/risk/*_test.go` に新規テスト追加。`openVerifiedIdentity` が fd 内容のハッシュを検証済みハッシュと照合し、不一致時に `ErrIdentityHashMismatch` を返すことを確認 | テスト通過 |
| **AC-02** | test | `internal/runner/base/risk/*_test.go` に新規テスト追加。path を FIFO に差し替えた場合に `ErrIdentityNotRegular` を返し、`O_NONBLOCK` が open ブロックを防ぎ、`fstat` による検査が有効であることを確認 | テスト通過 |
| **AC-03** | test | 既存テスト `openVerifiedIdentityForTest` 等のテストが変更前と同じ結果を返すことを確認（回帰防止） | 既存テスト全て通過 |
| **AC-04** | test | `internal/runner/base/risk/*_test.go` に新規テスト追加。`ErrIdentityHashMismatch` / `ErrIdentityNotRegular` が適切に `ReasonIdentityHashMismatch` / `ReasonIdentityNotRegular` に対応付けられ、audit ログに記録されることを確認 | テスト通過 |
| **AC-05** | test | `internal/safefileio/*_test.go` に新規テスト追加。検証後に source path を別 inode へ差し替えても、移動されるのが検証済み inode であること（fd アンカー）を確認 | テスト通過 |
| **AC-06** | test | 既存テスト `AtomicMoveFile` 系のテストが成功し、改ざんなし時の移動が fail-closed 後退を持たないことを確認 | 既存テスト全て通過 |
| **AC-07** | test | `internal/filevalidator/*_test.go` に新規テスト追加。`SaveRecord` で共有 fd が各解析へ束縛され、読み取り後の差し替えモデルで `ContentHash` と解析結果が同一 inode の同一読み取りに対応することを確認 | テスト通過 |
| **AC-08** | test | `internal/filevalidator/*_test.go` と `internal/dynamicanalysis/*_test.go` に新規テスト追加。`analyzeOneLibrary` と `LoadOrAnalyzeAndStore` (store 境界) でハッシュキー不一致時に `ErrLibraryHashKeyMismatch` を返すことを確認 | テスト通過 |
| **AC-09** | test | 既存テスト `SaveRecord` 系のテストが変更前と同じ record を生成すること（回帰防止）を確認 | 既存テスト全て通過 |
| **AC-10** | test | `internal/verification/manager_test.go` に新規テスト追加。`VerifyCommandDynLibDeps` が依存解決を再実行し、record 後に高優先位置に新規ライブラリが置かれた場合に verify が失敗することを確認。構造化エラーが soname・新旧パス・シャドーイング段階を保持することを確認 | テスト通過 |
| **AC-11** | test | `internal/verification/manager_test.go` に新規テスト追加。環境不変時に `VerifyCommandDynLibDeps` が成功することを確認 | テスト通過 |
| **AC-12** | test | `internal/verification/*_test.go` に新規テスト追加。`validateAndCacheCommand` が `EvalSymlinks` を検証より先に実行し、検証対象と キャッシュ対象が同一解決結果を指すことを確認 | テスト通過 |
| **AC-13** | test | 既存テスト `PathResolver` 系のテストが変更前と同じ解決済みパスをキャッシュすること（回帰防止）を確認 | 既存テスト全て通過 |
| **AC-14** | static | `rg -n "完全排除が困難" docs/tasks/0155.../02_architecture.md` で 5.3 節に (a)(b)(c) が記載されていることを確認 | マッチして記載が確認される |

---

## 6. 受け入れ基準の追跡

### AC-01: openVerifiedIdentity のハッシュ再検証

**実装ステップ**:
- Phase 1 での openVerifiedIdentity ハッシュ検証実装

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestOpenVerifiedIdentity_HashMismatchReturnsError`
- 検証内容: fd 内容を検証済みハッシュと不一致にしたとき、`ErrIdentityHashMismatch` を返す

---

### AC-02: 通常ファイル確認と O_NONBLOCK

**実装ステップ**:
- Phase 1 での openVerifiedIdentity に fstat と O_NONBLOCK 追加

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestOpenVerifiedIdentity_FIFODetection`, `TestOpenVerifiedIdentity_NonblockPreventsHang`
- 検証内容: path を FIFO に差し替えたとき `ErrIdentityNotRegular` を返す

---

### AC-03: 正常系の回帰防止（openVerifiedIdentity）

**実装ステップ**:
- Phase 1 での openVerifiedIdentity ハッシュ検証実装

**検証テスト**:
- テスト場所: `internal/runner/base/executor/stagefromfd_test.go`
- テスト名: `TestOpenVerifiedIdentity_SuccessReturnsVerifiedIdentity` (既存テスト維持)
- 検証内容: 改ざんなし時に VerifiedIdentity が返る

---

### AC-04: reason code の区別可能性

**実装ステップ**:
- Phase 1 での reason code 定義と allowedPlan での分岐追加

**検証テスト**:
- テスト場所: `internal/runner/base/risk/evaluator_test.go` (新規テスト追加)
- テスト名: `TestAllowedPlan_ReturnsDistinctReasonCodes`
- 検証内容: `ReasonIdentityHashMismatch` と `ReasonIdentityNotRegular` が適切に返される

---

### AC-05: fd アンカー移動による同一性保証

**実装ステップ**:
- Phase 4 での atomicMoveFileCore 実装変更

**検証テスト**:
- テスト場所: `internal/safefileio/safe_file_test.go` (新規テスト追加)
- テスト名: `TestAtomicMoveFileCore_InodeAnchorAfterSourceReplacement`
- 検証内容: 検証後にソースパスを別 inode へ差し替えても、移動されるのが検証済み inode である

---

### AC-06: 移動失敗時の fail-closed

**実装ステップ**:
- Phase 4 での atomicMoveFileCore 実装変更

**検証テスト**:
- テスト場所: `internal/safefileio/safe_file_test.go` (既存テスト維持)
- テスト名: `TestAtomicMoveFileCore_FailureDoesNotMove` (既存パス)
- 検証内容: 改ざんなし時に移動が成功する

---

### AC-07: 共有 fd への束縛（ハッシュと解析の一貫性）

**実装ステップ**:
- Phase 3 での SaveRecord 実装・各解析器の拡張

**検証テスト**:
- テスト場所: `internal/filevalidator/validator_test.go` (新規テスト追加)
- テスト名: `TestSaveRecord_SharedFDBindsHashAndAnalysis`
- 検証内容: 共有 fd から計算されたハッシュと解析結果が同一 inode の同一読み取りに対応

---

### AC-08: ライブラリハッシュキー照合

**実装ステップ**:
- Phase 3 での analyzeOneLibrary と store.LoadOrAnalyzeAndStore の実装変更

**検証テスト**:
- テスト場所: `internal/filevalidator/validator_test.go` (新規テスト追加) と `internal/dynamicanalysis/store_test.go` (新規テスト追加)
- テスト名: `TestAnalyzeOneLibrary_HashKeyMismatchError`, `TestLoadOrAnalyzeAndStore_HashKeyMismatchError`
- 検証内容: ハッシュキー不一致時に `ErrLibraryHashKeyMismatch` を返す

---

### AC-09: record 正常系の回帰防止

**実装ステップ**:
- Phase 3 での SaveRecord 実装・各解析器の拡張

**検証テスト**:
- テスト場所: `internal/filevalidator/validator_shebang_test.go` 等（既存テスト維持）
- テスト名: `TestSaveRecord_ShebangDirect`, `TestSaveRecord_ShebangELF` 等
- 検証内容: 改ざんなし時に record 内容が変わらない

---

### AC-10: verify 時の依存解決再実行

**実装ステップ**:
- Phase 2 での VerifyCommandDynLibDeps と verifyDynLibDeps の実装変更

**検証テスト**:
- テスト場所: `internal/verification/manager_test.go` (新規テスト追加)
- テスト名: `TestVerifyCommandDynLibDeps_ReExecutesDependencyResolution`
- 検証内容: 依存解決が再実行され、record 後に高優先位置に新規ライブラリが置かれた場合に verify が失敗する

---

### AC-11: verify 正常系の回帰防止

**実装ステップ**:
- Phase 2 での VerifyCommandDynLibDeps と verifyDynLibDeps の実装変更

**検証テスト**:
- テスト場所: `internal/verification/manager_test.go` (既存テスト維持)
- テスト名: `TestVerifyCommandDynLibDeps_SucceedsWhenDependenciesUnchanged`
- 検証内容: 環境不変時に verify が成功する

---

### AC-12: validateAndCacheCommand の順序修正

**実装ステップ**:
- Phase 1 での validateAndCacheCommand 順序変更

**検証テスト**:
- テスト場所: `internal/verification/path_resolver_test.go` (新規テスト追加)
- テスト名: `TestValidateAndCacheCommand_EvalSymlinksBeforeValidation`
- 検証内容: EvalSymlinks 実行後、解決済みパスに対して Lstat 検証が行われる

---

### AC-13: validateAndCacheCommand 正常系の回帰防止

**実装ステップ**:
- Phase 1 での validateAndCacheCommand 順序変更

**検証テスト**:
- テスト場所: `internal/verification/path_resolver_test.go` (既存テスト維持)
- テスト名: `TestValidateAndCacheCommand_ReturnsResolvedPath` 等
- 検証内容: 改ざんなし時に解決済みパスが返りキャッシュされる

---

### AC-14: F-006 残余リスクの文書化

**実装ステップ**:
- 02_architecture.md § 5.3 への記載（本タスク前で既に実施）

**検証方法**: 静的確認
- コマンド: `rg -n "完全排除が困難である理由" docs/tasks/0155.../02_architecture.md`
- 期待結果: § 5.3 に (a)(b)(c) が記載されている

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
| `VerifyCommandDynLibDeps` テストモック期待 | テストファイル | `rg -n "\.On\(\"VerifyCommandDynLibDeps" internal/ --type go` | 全ての `.On` 期待が `mock.Anything, mock.Anything` で envVars を受け取る |

---

## 8. 次のステップ

以下は本実装計画作成後の進行ステップであり、本計画自体の進化に含まれない。

1. 本実装計画（`03_implementation_plan.md`）のレビュー・承認待機
2. Phase 1 以降の実装に従事
3. 各 phase 完了ごとに `make test && make lint` による検証
4. 全 AC の検証テスト完了
5. 実装計画ドキュメント状態を `approved` へ更新
6. 実装ブランチを PR 作成・レビュー・マージ
