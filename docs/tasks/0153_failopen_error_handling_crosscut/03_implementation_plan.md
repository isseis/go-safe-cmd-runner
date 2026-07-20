# 実装計画書: エラー隠蔽による fail-open パターンの横断修正（残件）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-20 |
| Review date | 2026-07-20 |
| Reviewer | isseis |
| Comments | - |

## 関連ドキュメント

- 要件定義書: [01_requirements.md](./01_requirements.md)
- アーキテクチャ設計書: [02_architecture.md](./02_architecture.md)
- Test Organization Guide: [test_organization.md](../../dev/developer_guide/test_organization.md)

---

## 1. 実装概要

### 1.1 目的

6パッケージに横断的に存在する「エラー隠蔽による fail-open」パターンを一貫して fail-closed に修正する。全修正箇所で共通する方針は以下のとおり:

1. `slog.Debug` による無視 → `slog.Warn` へ格上げ
2. エラーを無視して「問題なし」を返す → エラーを呼び出し元に伝播
3. `switch` の `default` なし → `default` 節で fail-closed

### 1.2 実装方針

- 各修正箇所は機能的な依存関係を持たないため、並行実装可能。ただし以下のファイル競合に注意する:
  - Phase 4（C1 F-1）と Phase 6（C2 F-3）は同一ファイル（`standard_analyzer.go`）を変更する。
  - Phase 2（B3 L1: `hasDynamicLibraryDeps`）と Phase 3（B3 M1: `collectVerificationFiles`・`VerifyGroupFiles`）は同一ファイル（`manager.go`）を変更する。異なる関数への変更であり git merge 上は競合しにくいが、同一 PR での並行実装時は確認が推奨される。
- 新規パッケージ `internal/elfmagic` は leaf ユーティリティとして設計され、他パッケージへの依存を持たない。
- テストは各修正箇所の既存テストファイルに追加する。テストヘルパーが必要な場合はパッケージ内 `test_helpers.go`（`//go:build test`）を使用する。
- 全テストは `go test -tags test -v ./...` で実行可能であり、`make test` で全件パスすることを確認する。

### 1.3 既存コード調査結果

| 修正箇所 | 既存実装の状況 | 既存テスト | 必要な変更 |
|---|---|---|---|
| C1 F-1 `lookupSyscallAnalysis` | `standard_analyzer.go:297-332`。`default` 節が `slog.Debug` + `StaticBinary` へ縮退。 | `analyzer_test.go` に `TestStandardELFAnalyzer_SyscallLookup_Status_*` 系テストあり。`default` 節に到達する想定外エラーのケースは未テスト。 | `default` 節の Fail-closed 化、新規 sentinel `ErrSyscallStoreIOError`、既存の `ErrSyscallHashMismatch` が使う sentinel `ErrSyscallHashMismatch` への影響なし、`convertSyscallResult` が使う `ErrSyscallAnalysisHighRisk` への影響なし |
| C2 F-3 ELF top-level | `elfdynlib/analyzer.go:115-127`。`elf.NewFile`・`DynString` 失敗を `nil, nil`（`//nolint:nilerr`）に縮退。 | `analyzer_test.go` に各シナリオのテストあり。非ELFの正常系テストあり。 | マジックチェック導入、`internal/elfmagic` パッケージ新設 |
| C2 F-3 ELF child | `elfdynlib/analyzer.go:207-218`。子パース失敗を `slog.Debug` + `continue`。 | 子パース失敗の fail-open 挙動は未テスト。 | ログレベル格上げ + エラー伝播 |
| C2 F-3 Mach-O child | `machodylib/analyzer.go:215-221`。`parseMachODeps` 失敗を `slog.Debug` + `continue`。 | 子パース失敗の fail-open 挙動は未テスト（darwin ビルドタグ）。 | ログレベル格上げ + エラー伝播 |
| C2 F-5 `HasDynamicLibDeps` | `machodylib/analyzer.go:617-632`。`Seek` 失敗（2箇所）、`io.ReadFull` 失敗を `(false, nil)` に縮退。 | `analyzer_test.go` に `HasDynamicLibDeps` のテストあり。I/O エラーパスは未テスト（darwin ビルドタグ）。 | エラー伝播追加、`io.EOF`/`io.ErrUnexpectedEOF` は正常扱い |
| B3 M1 `collectVerificationFiles` | `verification/manager.go:264-277`。`slog.Warn` + `continue` で無視。戻り値 `map[string]struct{}`。 | `manager_test.go` に `VerifyGroupFiles` 系テストあり。パス解決失敗のケースは未テスト。 | シグネチャ変更（`error` 追加）、呼び出し元 `VerifyGroupFiles` の修正 |
| B3 L1 `hasDynamicLibraryDeps` | `verification/manager.go:711-715`。`DynString` エラーと `len(needed)==0` を同一視し `(false, nil)`。 | `manager_test.go` にテストあり。DynString エラーケースは未テスト。 | エラー分離、B3 M1 とのファイル競合なし（異なる関数） |
| A5 Low-3 `applyBinaryAnalysis` | `evaluator.go:461-477`。`default` 節なし。4 クラスのみ列挙。 | `risktypes/types_test.go` にゼロ値テストあり。`applyBinaryAnalysis` の未知クラス個別テストはなし。 | `default` 節追加、`blockingAssessment("", "")` を使用 |

#### 1.3.1 シンボル検証結果

- `isELFMagic`（`standard_analyzer.go:288`）: 非公開。呼び出し元は `standard_analyzer.go:141` のみ — `elfmagic.Is` に置換可能。
- `elfMagic`（`standard_analyzer.go:24`）: 非公開変数。削除（`elfmagic.Is` が内部で同一のバイト列を使用するため、公開定数としての露出は不要）。
- `elfMagicLen`（`standard_analyzer.go:27`）: 非公開定数。`elfmagic.Len` に置換。
- `elfMagicStr`（`standard_analyzer.go:21`）: 非公開定数。ELF マジックバイト列の文字列表現。置換後は削除。
- 上記シンボルは `standard_analyzer.go:133`（`AnalyzeNetworkSymbols` 内）でも使用されており、すべて置換対象。
- `ErrSyscallAnalysisHighRisk`（`standard_analyzer.go:41`）: 既存の `convertSyscallResult` 内で高リスク検知時に使用。C1 F-1 修正の `default` 節では**使用しない**（新規 `ErrSyscallStoreIOError` を導入）。
- `ErrSyscallHashMismatch`（`errors.go:27`）: `lookupSyscallAnalysis` 内のハッシュ不一致ケースで使用。修正後も変更なし。
- `blockingAssessment`（`evaluator.go:495`）: 非公開関数。A5 Low-3 の `default` 節で使用。
- `BinaryAnalysisClass`（`risktypes/types.go:145-157`）: 現状 4 値（`Uncertain`/`Clean`/`Network`/`HighRisk`）。A5 Low-3 修正後は `default` 節で未知値を捕捉。
- `looksLikeMachO`（`machodylib/analyzer.go`）: `HasDynamicLibDeps` 内で使用。既存参照あり。
- **注意**: `internal/filevalidator/validator.go` にも独自の `elfMagic` / `elfMagicStr` 定義が存在する（`validator.go:1734-1738`、`openELFFile` から参照）。この3つ目の重複は本タスクの対象外（`01_requirements.md` スコープ外）であり、`internal/elfmagic` への統一は行わない。`validator.go` の変更を避けてリスクを最小化するため、設計上の意図的な除外である。

---

## 2. 実装手順

### Phase 1: A5 Low-3 — `applyBinaryAnalysis` の `default` 節追加

#### Step 1-1: `default` 節を追加

- [x] **ファイル**: `internal/runner/base/risk/evaluator.go`
- [x] `applyBinaryAnalysis`（461行目付近）の `switch result.Class` に `default` 節を追加する
- [x] `default` 節の内容: `blockingAssessment("", "")` で生成した `risktypes.RiskAssessment` に `result.ReasonCodes` を設定し、`&blocked, nil` を返す
- [x] 実装コードは既存の `Uncertain` ケースと同一パターンに従う（`02_architecture.md` 3.6.2節参照）
- [x] 既存の 4 ケース（`Uncertain`/`HighRisk`/`Network`/`Clean`）は変更しない

**検証**: `make test` の全テストがパスすること。既存の AC-18 のテストは変更不要（`risktypes/types_test.go::TestBinaryAnalysisClass_ZeroValueIsUncertain` はそのまま維持）。

#### Step 1-2: 未知クラスのテストを追加

- [x] **ファイル**: `internal/runner/base/risk/evaluator_test.go`
- [x] `TestApplyClassResult_DefaultBlocksUnknownClass` テストを追加する（実装では switch ロジックを `applyClassResult` に分離し、この関数を直接テストしている。モック `Classify` は不要）
- [x] テスト内容: 未知の `BinaryAnalysisClass` 値（例: `BinaryAnalysisClass(999)`）を `applyClassResult` に渡し、`*risktypes.RiskAssessment`（non-nil Blocking）を返すことを検証
- [x] 正常系の 4 クラス（`Uncertain`/`Clean`/`Network`/`HighRisk`）が従来どおり動作することの確認は、`TestApplyClassResult_KnownClassesUnchanged` で担保

**検証**: `go test -tags test -v ./internal/runner/base/risk/` がパスすること。

### PR-1 作成ポイント: risk evaluator default clause

**対象ステップ**: 1-1 / 1-2

**推奨タイトル**: `feat(0153): add default clause to applyBinaryAnalysis for fail-closed`

**レビュー観点**: `default` 節が既存の 4 クラス（Uncertain/HighRisk/Network/Clean）に影響していないこと / 未知クラスが Blocking を返すこと / `blockingAssessment("", "")` の使用が既存の `Uncertain` ケースと一貫していること

**実装モデル要件**: standard

**判定理由**: 該当トリガーなし（単純な `default` 節追加と単体テストのみ）

- [x] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: B3 L1 — `hasDynamicLibraryDeps` の fail-closed 化

#### Step 2-1: DynString エラーを分離

- [x] **ファイル**: `internal/verification/manager.go`
- [x] `hasDynamicLibraryDeps`（711-715行目）の `if err != nil || len(needed) == 0` を分割する:
  - `if err != nil`: `return false, fmt.Errorf("failed to read DT_NEEDED: %w", err)`
  - `if len(needed) == 0`: `return false, nil`（依存なし・正常）
  - それ以外: `return true, nil`（依存あり）
- [x] 関数の doc comment を更新し、`DynString` エラーが fail-closed であることを明記する

**検証**: `make test` の既存テストがパスすること。

#### Step 2-2: DynString エラーのテストを追加

- [x] **ファイル**: `internal/verification/manager_test.go`
- [x] `TestHasDynamicLibraryDeps_DynStringError` テストを追加した
- [x] テスト内容: DT_NEEDED セクションが破壊された ELF ファイルを作成し、`hasDynamicLibraryDeps` が `(false, err)` を返すことを検証
- [x] DT_NEEDED なし（正常系）が `(false, nil)` を返すことの確認（AC-16）: `TestHasDynamicLibraryDeps_NoDeps` を追加
- [x] DT_NEEDED あり（正常系）が `(true, nil)` を返すことの確認: `TestHasDynamicLibraryDeps_DynamicELF` を追加（既存 `TestVerify_ELFNoDynLibDeps` でも間接的にカバー）

**検証**: `go test -tags test -v ./internal/verification/` がパスすること。

#### Step 2-3: 呼び出し元を通したエラー伝播のテスト（AC-15）

- [x] **ファイル**: `internal/verification/manager_test.go`
- [x] `TestVerifyCommandDynLibDeps_DynStringError` テストを追加した（AC-15）
- [x] テスト内容: DT_NEEDED セクションが破壊された ELF ファイルを作成し、`VerifyCommandDynLibDeps(elfPath)` を呼び出す。戻り値がエラーであること、かつエラーメッセージに原因が含まれることを検証
- [x] このテストは `hasDynamicLibraryDeps` → `verifyDynLibDeps` → `VerifyCommandDynLibDeps` の全呼び出しチェーンを通してエラーが伝播することを確認する

**検証**: `go test -tags test -v ./internal/verification/` がパスすること。

#### Step 2-4: dry-run モードのエラー伝播テスト

- [x] **ファイル**: `internal/verification/manager_test.go`
- [x] `TestVerifyCommandDynLibDeps_DynStringError_DryRun` テストを追加した
- [x] テスト内容: DT_NEEDED セクションが破壊された ELF ファイルを作成し、dry-run モードの `Manager` で `VerifyCommandDynLibDeps` を呼び出す。dry-run でもエラーが返り実行が中断されることを検証する（`02_architecture.md` §5.3 参照: C2 F-5 / B3 L1 は dry-run を中断させるようになる）

**検証**: `go test -tags test -v ./internal/verification/` がパスすること。

### Phase 3: B3 M1 — `collectVerificationFiles` の fail-closed 化

#### Step 3-1: シグネチャ変更と呼び出し元修正

- [ ] **ファイル**: `internal/verification/manager.go`
- [ ] `collectVerificationFiles`（250行目）のシグネチャを `map[string]struct{}` から `(map[string]struct{}, error)` に変更する
- [ ] パス解決失敗時（273行目）に `continue` する代わりに `return nil, fmt.Errorf("...: %w", err)` でエラーを返す
- [ ] ログ出力に構造化フィールド `"reason": "path_resolution_failed"` を追加し、`slog.Warn` は維持する
- [ ] 正常系（全パス解決成功）では `return fileSet, nil` を返す
- [ ] 呼び出し元 `VerifyGroupFiles`（196行目）で戻り値のエラーをチェックし、上位に伝播する。ラップには `Error`（`errors.go:105`）を用いる。`Error` は `Group` フィールドを持つ型であり、`OpError`（`errors.go:80`、`Op`/`Path`/`Err` のみで `Group` フィールドを持たない）とは異なる点に注意。エラーハンドリングは、既存の `FailedFiles > 0` 時の分岐（`manager.go:235`）と同じ型・パターンを踏襲する
- [ ] 具体的なラップ: `return nil, &Error{Op: "group", Group: groupName, Err: fmt.Errorf("failed to collect verification files: %w", err)}`

**検証**: `make test` の既存テストがパスすること。

#### Step 3-2: パス解決失敗のテストを追加

- [ ] **ファイル**: `internal/verification/manager_test.go`
- [ ] `TestVerifyGroupFiles_PathResolutionFailure` テストを追加する
- [ ] テスト内容: パス解決不能なコマンドを含む `GroupVerificationInput` で `VerifyGroupFiles` を呼び出し、`error` が返ること（AC-11, AC-12）
- [ ] パス解決不能な `PathResolver` の注入方法: `test_helpers.go` にモック `PathResolver` を追加するか、既存のモックインフラを利用する
- [ ] `TestVerifyGroupFiles_NormalPathResolution` テストを追加する: 正常にパス解決できるコマンドのみを含むグループで `VerifyGroupFiles` が成功すること（AC-13）
- [ ] `TestVerifyGroupFiles_PathResolutionFailure_DryRun` テストを追加する: パス解決不能なコマンドを含むグループに対し、dry-run モードの `Manager` で `VerifyGroupFiles` を呼び出す。dry-run でもエラーが返り実行が中断されることを検証する（`02_architecture.md` §5.3 参照: B3 M1 は dry-run を中断させるようになる）

**検証**: `go test -tags test -v ./internal/verification/` がパスすること。

### PR-2 作成ポイント: verification error handling fail-closed

**対象ステップ**: 2-1 / 2-2 / 2-3 / 2-4 / 3-1 / 3-2

**推奨タイトル**: `feat(0153): fail-closed error handling in verification manager`

**レビュー観点**: `hasDynamicLibraryDeps` の DynString エラーと `len(needed)==0` が正しく分離されていること / `collectVerificationFiles` のシグネチャ変更が唯一の呼び出し元 `VerifyGroupFiles` に正しく伝播していること / dry-run モードでもエラーが伝播し実行が中断されること / 正常系（パス解決成功、DT_NEEDED なし）にリグレッションがないこと

**実装モデル要件**: standard

**判定理由**: 該当トリガーなし（条件分岐の分離、非公開関数のシグネチャ変更、単体テストのみ。アプローチは確立しており競合する設計選択肢なし）

- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 4: C1 F-1 — `lookupSyscallAnalysis` の fail-closed 化

#### Step 4-1: 新規 sentinel error の追加

- [ ] **ファイル**: `internal/security/elfanalyzer/standard_analyzer.go`
- [ ] `ErrSyscallStoreIOError` sentinel error を `var` ブロック（33行目付近）に追加する: `ErrSyscallStoreIOError = errors.New("syscall analysis store I/O error")`
- [ ] **設計根拠**: `02_architecture.md` 3.1.2節を参照（`ErrSyscallAnalysisHighRisk` との分離理由）

#### Step 4-2: `default` 節の修正

- [ ] **ファイル**: `internal/security/elfanalyzer/standard_analyzer.go`
- [ ] `lookupSyscallAnalysis`（297-332行目）の `default` 節を修正する:
  - `slog.Debug` → `slog.Warn` に格上げ
  - 構造化フィールド `"reason": "store_io_error"` を追加
  - 戻り値を `binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.AnalysisError, Error: fmt.Errorf("%w: %s", ErrSyscallStoreIOError, path)}` に変更する（`StaticBinary` へ縮退しない）
- [ ] `ErrSyscallHashMismatch`・`RecordNotFound` のケースは変更しない（既存の fail-closed / キャッシュ不在フォールバック挙動を維持）
- [ ] `convertSyscallResult` が使う `ErrSyscallAnalysisHighRisk` は変更しない（`02_architecture.md` 3.1.2節参照）

**検証**: `make test` の既存テストがパスすること。

#### Step 4-3: 想定外エラーのテストを追加

- [ ] **ファイル**: `internal/security/elfanalyzer/analyzer_test.go`
- [ ] `TestStandardELFAnalyzer_SyscallLookup_StoreIOError` テストを追加する（AC-01, AC-03）
- [ ] テスト内容: `LoadSyscallAnalysis` が想定外エラー（`io.ErrUnexpectedEOF` 等）を返すモックストアを使用し、`AnalyzeNetworkSymbols` が `AnalysisError` を返し、かつ `errors.Is(err, ErrSyscallStoreIOError)` が `true` であることを検証
- [ ] AC-03 のログレベル検証: テスト開始時に `prev := slog.Default(); defer slog.SetDefault(prev)` で既存のデフォルトロガーを退避し、`slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))` でハンドラーを設定する。テスト後に `slog.Warn` レベル以上のログが出力されたことを確認する（`bytes.Contains(buf.String(), "level=WARN")` などの TextHandler 形式のキーで検証）。**注意: `slog.SetDefault` はグローバル状態を変更するため、このテストは `t.Parallel()` を使用しないこと。または、コンポーネントにカスタム `slog.Logger` を注入する方式を推奨する。**
- [ ] 既存の `TestStandardELFAnalyzer_SyscallLookup_NotFound`（AC-02）と `TestStandardELFAnalyzer_SyscallLookup_HashMismatch` は変更不要（既存挙動維持を確認）

**検証**: `go test -tags test -v ./internal/security/elfanalyzer/` がパスすること。

### PR-3 作成ポイント: syscall store I/O error fail-closed

**対象ステップ**: 4-1 / 4-2 / 4-3

**推奨タイトル**: `feat(0153): fail-closed default case in lookupSyscallAnalysis`

**レビュー観点**: 新規 `ErrSyscallStoreIOError` が既存の `ErrSyscallAnalysisHighRisk` および `ErrSyscallHashMismatch` と正しく区別されていること / `default` 節が `StaticBinary` ではなく `AnalysisError` を返すこと / ログレベルが `slog.Debug` から `slog.Warn` に格上げされ `"reason": "store_io_error"` が付与されていること

**実装モデル要件**: standard

**判定理由**: 該当トリガーなし（新規 sentinel error の追加、`default` 節の修正、単体テストのみ。設計判断はアーキテクチャ設計書 §3.1.2 で確定済み）

- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: C2 F-5 — `HasDynamicLibDeps` の fail-closed 化

#### Step 5-1: `Seek` 失敗と `io.ReadFull` 失敗のエラー伝播

- [ ] **ファイル**: `internal/dynlib/machodylib/analyzer.go`
- [ ] `HasDynamicLibDeps`（617-632行目）の単一アーキテクチャ Mach-O パスを修正する:
  - `Seek` 失敗（619-621行目）: `return false, nil` → `return false, fmt.Errorf("failed to seek to start of file: %w", err)` に変更
  - `Seek` 失敗（629-631行目）: `return false, nil` → `return false, fmt.Errorf("failed to seek to start of file: %w", err)` に変更
  - `io.ReadFull` 失敗（624-626行目）:
    - `io.EOF` または `io.ErrUnexpectedEOF` の場合: `return false, nil`（非 Mach-O / ファイルが小さすぎる、正常）
    - それ以外のエラー: `return false, fmt.Errorf("failed to read Mach-O magic: %w", err)` に変更
- [ ] ログ出力は既存実装に存在しないため、追加不要（エラー伝播のみ）

**検証**: darwin 環境では `make test` がパスすること。linux CI では `GOOS=darwin GOARCH=arm64 go test -tags test -c ./internal/dynlib/machodylib/` でクロスコンパイル確認を行うこと。

#### Step 5-2: I/O エラーのテストを追加

- [ ] **ファイル**: `internal/dynlib/machodylib/analyzer_test.go`
- [ ] `TestHasDynamicLibDeps_SeekError` テストを追加する（AC-08）: Seek に失敗するモック `safefileio.File` を使用し、`HasDynamicLibDeps` が `(false, err)` を返すことを検証
- [ ] `TestHasDynamicLibDeps_ReadFullError` テストを追加する（AC-09）: 読み取りに失敗するモックファイル（`io.ErrUnexpectedEOF` 以外のエラーを返す `io.Reader`）を使用し、`HasDynamicLibDeps` が `(false, err)` を返すことを検証
- [ ] `TestHasDynamicLibDeps_ReadFullEOF` テストを追加する（境界値）: `io.EOF` または `io.ErrUnexpectedEOF` で読み取りが終了した場合、`HasDynamicLibDeps` が `(false, nil)` を返すことを検証（ファイルが 4 バイトに満たない非 Mach-O の正常系）
- [ ] モックの注入方法: `HasDynamicLibDeps` は `safefileio.FileSystem` を引数に取るため、`safefileio` の既存モックインフラを利用する

**検証**: `go test -tags test -v ./internal/dynlib/machodylib/` （darwin 環境）がパスすること。linux 環境では `go test -tags test -run '^$' ./internal/dynlib/machodylib/` でコンパイルが通ることを確認する。

#### Step 5-3: 呼び出し元のエラー伝播テスト（AC-10）

- [ ] **ファイル**: `internal/verification/manager_test.go`
- [ ] `TestHasMachODynamicLibraryDeps_ErrorPropagation` テストを追加する（AC-10）
- [ ] テスト内容: I/O エラーを返す `safefileio.FileSystem` モックを使用し、`hasMachODynamicLibraryDeps(path)` が `(false, non-nil err)` を返すことを検証
- [ ] `hasMachODynamicLibraryDeps` は `machodylib.HasDynamicLibDeps(path, m.safeFS)` のラッパーであるため、I/O エラーを返すモック `safeFS` を注入した `Manager` でテストする

**検証**: `go test -tags test -v ./internal/verification/` がパスすること。

### PR-4 作成ポイント: HasDynamicLibDeps I/O error fail-closed

**対象ステップ**: 5-1 / 5-2 / 5-3

**推奨タイトル**: `feat(0153): fail-closed I/O error handling in HasDynamicLibDeps`

**レビュー観点**: `Seek` 失敗と `io.ReadFull` 失敗が適切にエラー伝播されていること / `io.EOF` および `io.ErrUnexpectedEOF` が正常系（非 Mach-O）として扱われていること / 呼び出し元 `hasMachODynamicLibraryDeps` を通してエラーが正しく伝播すること / darwin ビルドタグ付きテストが linux 環境でコンパイル確認可能であること

**実装モデル要件**: standard

**判定理由**: 該当トリガーなし（I/O エラーのエラー伝播追加と単体テストのみ）

- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: C2 F-3 — 子依存パース失敗の fail-closed 化

#### Step 6-1: `internal/elfmagic` パッケージを新設する

- [ ] **ファイル**: `internal/elfmagic/elfmagic.go`（新規）
- [ ] パッケージ名: `elfmagic`
- [ ] 以下の公開 API を定義する:
  - `var magic = []byte("\x7fELF")`（非公開; 呼び出し元は `Len` および `Is()` のみを使用する）
  - `const Len = 4`
  - `func Is(b []byte) bool`: 先頭 `Len` バイトが `magic` と一致するか判定
- [ ] `bytes` パッケージのみに依存し、他パッケージへの依存を持たない

- [ ] **ファイル**: `internal/elfmagic/elfmagic_test.go`（新規）
- [ ] `TestIs` テストを追加する:
  - 正しい ELF マジックバイト列（`\x7fELF...`）が `true` を返すこと
  - 短すぎるバイト列が `false` を返すこと
  - 不正なマジック（Mach-O の `\xcf\xfa\xed\xfe` 等）が `false` を返すこと
  - 空バイト列が `false` を返すこと

**検証**: `go test -tags test -v ./internal/elfmagic/` がパスすること。

#### Step 6-2: `elfanalyzer` 側のリファクタリング

- [ ] **ファイル**: `internal/security/elfanalyzer/standard_analyzer.go`
- [ ] `elfmagic` パッケージをインポートに追加する
- [ ] `elfMagicStr`（21行目）、`elfMagic`（24行目）、`elfMagicLen`（27行目）の定義を削除する
- [ ] `isELFMagic`（288-293行目）の関数定義を削除する
- [ ] `isELFMagic(magic)` の呼び出し（141行目）を `elfmagic.Is(magic)` に置き換える
- [ ] `elfMagicLen` の使用箇所（133行目）を `elfmagic.Len` に置き換える
- [ ] **注意**: `bytes.Equal` は `isELFMagic` 内でのみ使用されていた。`isELFMagic` 削除に伴い `"bytes"` インポートも削除する（`go vet ./internal/security/elfanalyzer/` で unused import 警告が出ないことを確認）
- [ ] **検証**: `go vet ./internal/security/elfanalyzer/` が unused import 警告を出さないこと

**検証**: `go test -tags test -v ./internal/security/elfanalyzer/` がパスすること。既存の `AnalyzeNetworkSymbols` 関連テストすべてがパスすることを確認。

#### Step 6-3: ELF `Analyze` のトップレベル修正

- [ ] **ファイル**: `internal/dynlib/elfdynlib/analyzer.go`
- [ ] `elfmagic` パッケージをインポートに追加する
- [ ] `Analyze` 関数（100-127行目）を修正する:
  - `SafeOpenFile` 後、`io.ReadFull(file, magic[:])` で ELF マジックを読み取る（`magic := make([]byte, elfmagic.Len)`）
    - `io.ReadFull` が `io.EOF` または `io.ErrUnexpectedEOF` を返した場合: 非 ELF（ファイルが小さすぎる）→ `return nil, nil`
    - `io.ReadFull` がそれ以外の I/O エラーを返した場合: `return nil, fmt.Errorf("failed to read ELF magic: %w", err)`（fail-closed）
  - `elfmagic.Is(magic)` が `false` の場合: 非 ELF → `return nil, nil`（既存の正常系挙動を維持、スクリプト等はエラーにしない）
  - `elfmagic.Is(magic)` が `true` の場合: `file.Seek(0, io.SeekStart)` でファイルポインタを先頭に戻す
    - `Seek` エラーの場合: `return nil, fmt.Errorf("failed to seek to start of file: %w", err)`（fail-closed）
    - `Seek` 成功時: `elf.NewFile(file)` を試行
    - `elf.NewFile` 失敗時: `return nil, fmt.Errorf("failed to parse ELF binary: %w", err)`（fail-closed）
    - `elf.NewFile` 成功時: `DynString(DT_NEEDED)` を試行
      - `DynString` 失敗時: `return nil, fmt.Errorf("failed to read DT_NEEDED: %w", err)`（fail-closed）
      - `DynString` 成功 + `len(needed) == 0`: `return nil, nil`（依存なし・正常）
      - `DynString` 成功 + `len(needed) > 0`: 既存の BFS 処理に進む
- [ ] 既存の `//nolint:nilerr` コメント（118行目、126行目）を削除する
- [ ] **検証**: `grep -r 'nolint:nilerr' internal/dynlib/elfdynlib/` が空を返すこと。`make lint` が警告なしでパスすること
- [ ] `SafeOpenFile` が返す `File` インタフェースは `io.Seeker` を埋め込んでいるため、`file.(io.Seeker)` の型アサーションは冗長である。型アサーションを削除し、`file.Seek` を直接呼び出すように簡略化する（`02_architecture.md` 9.1節 パフォーマンスの項、`internal/safefileio/safe_file.go` の `File` インタフェース定義を参照）

**検証**: `go test -tags test -v ./internal/dynlib/elfdynlib/` がパスすること。

#### Step 6-4: ELF トップレベルのテストを追加

- [ ] **ファイル**: `internal/dynlib/elfdynlib/analyzer_test.go`（ビルドタグ: `linux`）
- [ ] `TestAnalyze_NonELFFile` テストを追加する（AC-05 一部）: 非ELFファイル（プレーンテキスト等）の解析が `(nil, nil)` を返すこと
- [ ] `TestAnalyze_ELFMagicMatchButParseFailure` テストを追加する（AC-05）: ELF マジックを持つがパース不能なファイルの解析がエラーを返すこと
- [ ] `TestAnalyze_ELFMagicMatchButDynStringError` テストを追加する（AC-05）: ELF マジックを持ちパース可能だが DT_NEEDED セクションが破壊されたファイルの解析がエラーを返すこと
- [ ] 正常系のリグレッション確認（AC-07）: 既存の `TestAnalyze_*` テストがすべてパスすることを確認する

**検証**: `go test -tags test -v ./internal/dynlib/elfdynlib/` がパスすること。

#### Step 6-5: ELF 子依存パース失敗の修正

- [ ] **ファイル**: `internal/dynlib/elfdynlib/analyzer.go`
- [ ] BFS traversal 内の子依存パース失敗（207-218行目）を修正する:
  - `slog.Debug` → `slog.Warn` に格上げ
  - 構造化フィールド `"reason": "child_parse_error"` を追加
  - `continue` → `return nil, err`（fail-closed、解析全体を失敗させる）
  - `ErrDTRPATHNotSupported` のケースは変更しない（既存どおりエラー伝播）

**検証**: `go test -tags test -v ./internal/dynlib/elfdynlib/` がパスすること。

#### Step 6-6: ELF 子依存パース失敗のテストを追加

- [ ] **ファイル**: `internal/dynlib/elfdynlib/analyzer_test.go`（ビルドタグ: `linux`）
- [ ] `TestAnalyze_ChildParseFailure` テストを追加する（AC-04）: BFS traversal 中にパース不能な子 ELF に遭遇した場合、`Analyze` がエラーを返すことを検証
- [ ] テストセットアップ: 既存の `buildTestELFWithDeps` ヘルパー（`elfdynlib/analyzer_test.go` 内、非公開）を使用して依存ツリーを構築し、孫ライブラリを破壊してパース失敗を引き起こす。孫ライブラリの破壊は ELF ヘッダの書き換えにより行う
- [ ] エラーメッセージに失敗した子ライブラリのパスが含まれることの検証（AC-04 の blast radius 対応）

**検証**: `go test -tags test -v ./internal/dynlib/elfdynlib/` がパスすること。

#### Step 6-7: Mach-O 子依存パース失敗の修正

- [ ] **ファイル**: `internal/dynlib/machodylib/analyzer.go`
- [ ] BFS traversal 内の `parseMachODeps` 失敗（215-221行目）を修正する:
  - `slog.Debug` → `slog.Warn` に格上げ
  - 構造化フィールド `"reason": "child_parse_error"` を追加
  - `continue` → `return nil, nil, parseErr`（fail-closed、解析全体を失敗させる。`Analyze` の戻り値シグネチャは `([]LibEntry, []AnalysisWarning, error)`）

**検証**: `go test -tags test -v ./internal/dynlib/machodylib/`（darwin 環境）がパスすること。

#### Step 6-8: Mach-O 子依存パース失敗のテストを追加

- [ ] **ファイル**: `internal/dynlib/machodylib/analyzer_test.go`（ビルドタグ: `darwin`）
- [ ] `TestAnalyze_ChildParseFailure` テストを追加する（AC-06）: BFS traversal 中にパース不能な子 Mach-O に遭遇した場合、`Analyze` がエラーを返すことを検証
- [ ] 制約: darwin ビルドタグが必要なため、CI では macos 環境でのみ実行される。linux 環境ではコンパイル確認のみ。

**検証**: `go test -tags test -v ./internal/dynlib/machodylib/`（darwin 環境）がパスすること。

#### Step 6-9: 統合テスト（正常系リグレッション確認）

- [ ] **ファイル**: 既存テストの再実行で確認する（新規ファイル追加不要）
- [ ] AC-07 の検証: `go test -tags test -v ./internal/dynlib/elfdynlib/ ./internal/dynlib/machodylib/` の既存テストがすべてパスすることを確認する
- [ ] 多階層依存、循環依存を持つ実バイナリに対する record/verify の正常系テストも、既存テストでカバーされていること

**検証**: `make test` の全テストがパスすること。

### PR-5 作成ポイント: dynlib parse failure fail-closed with shared ELF magic

**対象ステップ**: 6-1 / 6-2 / 6-3 / 6-4 / 6-5 / 6-6 / 6-7 / 6-8 / 6-9

**推奨タイトル**: `feat(0153): fail-closed dynlib parse failure with internal/elfmagic`

**レビュー観点**: `internal/elfmagic` パッケージが他パッケージへの依存を持たない leaf ユーティリティであること / `elfanalyzer` 側の `isELFMagic` 等の削除が既存の `AnalyzeNetworkSymbols` の挙動に影響しないこと / ELF `Analyze` のトップレベル修正でマジック不一致（非ELF）が正常系として `(nil, nil)` を返すこと / BFS traversal 中の子依存パース失敗が解析全体の失敗として伝播すること

**実装モデル要件**: standard

**判定理由**: 該当トリガーなし。BFS traversal の blast radius はアーキテクチャ設計書 §3.2.3 で分析・緩和策（エラーメッセージへの子ライブラリパス明示）が策定済み。コード変更自体は error propagation と ELF magic check の範囲に留まり、並行性制御・リカバリフロー・状態機械のような実装複雑性を伴わない

- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

---

## 3. 実装順序とマイルストーン

### 3.1 マイルストーン定義

| マイルストーン | 成果物 | 完了基準 |
|---|---|---|
| M1: A5 Low-3 | Phase 1 実装 + テスト | `go test -tags test -v ./internal/runner/base/risk/` パス |
| M2: B3 L1 + M1 | Phase 2 + Phase 3 実装 + テスト | `go test -tags test -v ./internal/verification/` パス |
| M3: C1 F-1 | Phase 4 実装 + テスト | `go test -tags test -v ./internal/security/elfanalyzer/` パス |
| M4: C2 F-5 | Phase 5 実装 + テスト | `go test -tags test ./internal/dynlib/machodylib/` パス（darwin） |
| M5: C2 F-3 | Phase 6 実装 + テスト | `make test` 全件パス |

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 1-1 / 1-2 | `applyBinaryAnalysis` の `switch` に `default` 節を追加し未知クラスを Blocking 化 | standard |
| PR-2 | 2-1 / 2-2 / 2-3 / 2-4 / 3-1 / 3-2 | `hasDynamicLibraryDeps` の DynString エラー分離、`collectVerificationFiles` のシグネチャ変更とエラー伝播 | standard |
| PR-3 | 4-1 / 4-2 / 4-3 | `lookupSyscallAnalysis` の `default` 節を fail-closed 化、新規 sentinel `ErrSyscallStoreIOError` 追加 | standard |
| PR-4 | 5-1 / 5-2 / 5-3 | `HasDynamicLibDeps` の `Seek`/`io.ReadFull` エラーを fail-closed 化 | standard |
| PR-5 | 6-1 / 6-2 / 6-3 / 6-4 / 6-5 / 6-6 / 6-7 / 6-8 / 6-9 | `internal/elfmagic` 新設、`elfanalyzer` リファクタリング、ELF/Mach-O 子依存パース失敗の fail-closed 化 | standard |

### 3.3 推奨実装順序

Phase 1（A5 Low-3）から着手し、変更量・リスクの小さい順に進める。Phase 6（C2 F-3）を最後にすることで、Phase 4 の `standard_analyzer.go` 変更を先に確定させる。

Phase 4 と Phase 6 を別ブランチで並行実装する場合、`standard_analyzer.go` のマージ競合に注意する（`02_architecture.md` 8.1節参照）。

---

## 4. テスト戦略

### 4.1 単体テスト戦略

| 修正箇所 | テストファイル | テスト関数 | 対応 AC |
|---|---|---|---|
| C1 F-1 | `internal/security/elfanalyzer/analyzer_test.go` | `TestStandardELFAnalyzer_SyscallLookup_StoreIOError` | AC-01, AC-03 |
| C2 F-3 (ELF top) | `internal/dynlib/elfdynlib/analyzer_test.go` | `TestAnalyze_NonELFFile`, `TestAnalyze_ELFMagicMatchButParseFailure`, `TestAnalyze_ELFMagicMatchButDynStringError` | AC-05 |
| C2 F-3 (ELF child) | `internal/dynlib/elfdynlib/analyzer_test.go` | `TestAnalyze_ChildParseFailure` | AC-04 |
| C2 F-3 (Mach-O child) | `internal/dynlib/machodylib/analyzer_test.go` | `TestAnalyze_ChildParseFailure` | AC-06 |
| C2 F-5 | `internal/dynlib/machodylib/analyzer_test.go` | `TestHasDynamicLibDeps_SeekError`, `TestHasDynamicLibDeps_ReadFullError` | AC-08, AC-09 |
| B3 M1 | `internal/verification/manager_test.go` | `TestVerifyGroupFiles_PathResolutionFailure`, `TestVerifyGroupFiles_NormalPathResolution` | AC-11, AC-12, AC-13 |
| B3 L1 | `internal/verification/manager_test.go` | `TestHasDynamicLibraryDeps_DynStringError` | AC-14 |
| B3 L1 (caller) | `internal/verification/manager_test.go` | `TestVerifyCommandDynLibDeps_DynStringError` | AC-15 |
| B3 L1 (dry-run) | `internal/verification/manager_test.go` | `TestVerifyCommandDynLibDeps_DynStringError_DryRun` | dry-run 中断検証 |
| B3 M1 (dry-run) | `internal/verification/manager_test.go` | `TestVerifyGroupFiles_PathResolutionFailure_DryRun` | dry-run 中断検証 |
| A5 Low-3 | `internal/runner/base/risk/evaluator_test.go` | `TestApplyBinaryAnalysis_DefaultBlocksUnknownClass` | AC-17 |
| elfmagic 新設 | `internal/elfmagic/elfmagic_test.go` | `TestIs` | AC-05 の一部（マジック判定の正しさ） |

### 4.2 既存テストのリグレッション確認

すべての正常系 AC（AC-02, AC-07, AC-10, AC-13, AC-16, AC-18）は、既存テストでカバーされている。これらの AC の検証は以下の既存テストのパスで確認する:

| AC | 既存テスト |
|---|---|
| AC-02 | `TestStandardELFAnalyzer_SyscallLookup_NotFound`, `TestDynamicELF_SyscallFallback_NotRecorded` |
| AC-07 | `internal/dynlib/elfdynlib/analyzer_test.go` の既存テスト全件、`internal/dynlib/machodylib/analyzer_test.go` の既存テスト全件 |
| AC-13 | `TestVerifyGroupFiles` 系の既存テスト全件 |
| AC-16 | `TestVerify_ELFNoDynLibDeps`（`DT_NEEDED` 読み取り後の動的依存なし ELF）、`TestVerify_NonELFNoDynLibDeps`（非 ELF が `(false, nil)` を返す） |
| AC-18 | `TestBinaryAnalysisClass_ZeroValueIsUncertain`、`evaluator_test.go` の既存 `applyBinaryAnalysis` 関連テスト |

> AC-10 は新規テスト（Step 5-3 `TestHasMachODynamicLibraryDeps_ErrorPropagation`）で検証するため、本表には含めていない。

### 4.3 テストヘルパー方針

- C1 F-1: 既存の `mockSyscallAnalysisStore`（`analyzer_test.go` 内、非公開）を流用する。新規のテストヘルパーは不要。
- C2 F-3 (ELF): 既存の `buildTestELFWithDeps` ヘルパー（`elfdynlib/analyzer_test.go` 内、非公開）を流用・拡張する。壊れた ELF の生成には新たに `buildBrokenELFWithDeps` のようなヘルパーを同ファイルに追加する。
- B3 M1: パス解決失敗を注入するために、`test_helpers.go`（`//go:build test`）にモック `PathResolver` を追加する必要がある。`VerifyGroupFiles` の dry-run モードテストでは `WithDryRunMode()` TestOption を使用する。
- B3 L1: DT_NEEDED セクション破壊 ELF の生成には、`elfdynlib/analyzer_test.go` の ELF ビルダーの手法を参考に、`manager_test.go` 内に専用ヘルパーを追加する。または破壊 ELF を事前生成してテストフィクスチャとして使用する。B3 L1 はエラー伝播のみで構造化ログの追加は行わない（`slog.Warn` を追加してもログファイルへの出力であり、エラー伝播ですでに上位でログ出力されるため、二重ログを避ける設計判断）
- その他: `elfmagic` のテストは自明なテーブルテストであり、ヘルパー不要。

---

## 5. 受入基準検証（Acceptance Criteria Verification）

| AC | 検証種別 | 検証場所 / コマンド | 合格条件 |
|---|---|---|---|
| AC-01 | test | `internal/security/elfanalyzer/analyzer_test.go::TestStandardELFAnalyzer_SyscallLookup_StoreIOError` | 想定外エラー時に `AnalysisError` が返ること、`errors.Is(err, ErrSyscallStoreIOError)` が true |
| AC-02 | test | `internal/security/elfanalyzer/analyzer_test.go::TestStandardELFAnalyzer_SyscallLookup_NotFound`（既存） | `RecordNotFound` 時に `StaticBinary` が返ること |
| AC-03 | test | AC-01 と同じテスト内で検証 | テスト内でログをキャプチャし、`WARN` レベル以上のログが出力されたことを確認する（Step 4-3 参照） |
| AC-04 | test | `internal/dynlib/elfdynlib/analyzer_test.go::TestAnalyze_ChildParseFailure` | 子 ELF パース失敗がエラーとして伝播すること |
| AC-05 | test | `internal/dynlib/elfdynlib/analyzer_test.go::TestAnalyze_NonELFFile`、`TestAnalyze_ELFMagicMatchButParseFailure`、`TestAnalyze_ELFMagicMatchButDynStringError` | 非ELF は `(nil, nil)`、ELF マジックあり + パース失敗はエラー |
| AC-05 (magic) | test | `internal/elfmagic/elfmagic_test.go::TestIs` | 正しい ELF マジック判定が行われること |
| AC-06 | test | `internal/dynlib/machodylib/analyzer_test.go::TestAnalyze_ChildParseFailure` | 子 Mach-O パース失敗がエラーとして伝播すること |
| AC-07 | test | `make test` 全件パス | 既存の正常系テストがすべてパスすること |
| AC-08 | test | `internal/dynlib/machodylib/analyzer_test.go::TestHasDynamicLibDeps_SeekError` | `Seek` 失敗時に `(false, non-nil err)` が返ること |
| AC-09 | test | `internal/dynlib/machodylib/analyzer_test.go::TestHasDynamicLibDeps_ReadFullError` | `io.ReadFull` 失敗（非EOF）時に `(false, non-nil err)` が返ること |
| AC-10 | test | `internal/verification/manager_test.go::TestHasMachODynamicLibraryDeps_ErrorPropagation`（新規追加） | `hasMachODynamicLibraryDeps` が、I/O エラーを返す `HasDynamicLibDeps` のエラーを上位に伝播し `(false, non-nil err)` を返すこと |
| AC-11 | test | `internal/verification/manager_test.go::TestVerifyGroupFiles_PathResolutionFailure` | パス解決失敗時に `VerifyGroupFiles` がエラーを返すこと |
| AC-12 | static | `sed -n '/func (m \*Manager) collectVerificationFiles/,/^}/p' internal/verification/manager.go | grep continue` — `collectVerificationFiles` 内のパス解決失敗経路に `continue` が存在しないこと（`return error` に置き換わっている）。`VerifyGroupFiles` が当該ファイルの検証をスキップするコードパスがないことをコードレビューで確認 | `collectVerificationFiles` のシグネチャに `error` が追加されているため、呼び出し元 `VerifyGroupFiles` は戻り値エラーをチェックせざるを得ず（コンパイルエラー）、fail-open の窓は物理的に存在しない。これは静的保証（コンパイラ強制）であり、`test` ではなく `static` として検証する |
| AC-13 | test | `internal/verification/manager_test.go::TestVerifyGroupFiles_NormalPathResolution`（または既存の `TestVerifyGroupFiles_*` 系の拡張） | 正常にパス解決できるコマンドのみを含むグループで `VerifyGroupFiles` が成功すること。`collectVerificationFiles` 単体ではなく `VerifyGroupFiles` の公開 API レベルで検証する |
| AC-14 | test | `internal/verification/manager_test.go::TestHasDynamicLibraryDeps_DynStringError` | `DynString` エラー時に `(false, non-nil err)` が返ること |
| AC-15 | test | `internal/verification/manager_test.go::TestVerifyCommandDynLibDeps_DynStringError`（新規追加） | `hasDynamicLibraryDeps` の DynString エラーが `verifyDynLibDeps` → `VerifyCommandDynLibDeps` 経由で呼び出し元にエラーとして伝播すること |
| AC-16 | test | `internal/verification/manager_test.go` の既存テスト | DT_NEEDED なし（正常系）が `(false, nil)` を返すこと |
| AC-17 | test | `internal/runner/base/risk/evaluator_test.go::TestApplyBinaryAnalysis_DefaultBlocksUnknownClass` | 未知クラスが `*risktypes.RiskAssessment`（non-nil Blocking）を返すこと |
| AC-18 | test | `make test` の `internal/runner/base/risk/` 内テスト全件パス | 既存の 4 クラスの挙動が変更されないこと（ゼロ値テスト `TestBinaryAnalysisClass_ZeroValueIsUncertain` も変更不要） |

---

## 6. 実装チェックリスト

### Phase 1: A5 Low-3
- [x] Step 1-1: `default` 節を追加
- [x] Step 1-2: 未知クラスのテストを追加

### PR-1 作成ポイント: risk evaluator default clause
- [x] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [x] PR を作成した
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 2: B3 L1
- [x] Step 2-1: `hasDynamicLibraryDeps` の DynString エラー分離
- [x] Step 2-2: DynString エラーのテスト追加
- [x] Step 2-3: 呼び出し元を通したエラー伝播のテスト（AC-15）
- [x] Step 2-4: dry-run モードのエラー伝播テスト

### PR-2 作成ポイント: verification error handling fail-closed
- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 3: B3 M1
- [ ] Step 3-1: `collectVerificationFiles` シグネチャ変更と呼び出し元修正
- [ ] Step 3-2: パス解決失敗テスト追加、正常系テスト追加
- [ ] Step 3-3: dry-run モードのエラー伝播テスト

### Phase 4: C1 F-1
- [ ] Step 4-1: `ErrSyscallStoreIOError` sentinel 追加
- [ ] Step 4-2: `default` 節の修正
- [ ] Step 4-3: 想定外エラーテスト追加

### PR-3 作成ポイント: syscall store I/O error fail-closed
- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 5: C2 F-5
- [ ] Step 5-1: `HasDynamicLibDeps` の I/O エラー伝播
- [ ] Step 5-2: Seek エラー・ReadFull エラー・ReadFull EOF テスト追加
- [ ] Step 5-3: 呼び出し元のエラー伝播テスト（AC-10）

### PR-4 作成ポイント: HasDynamicLibDeps I/O error fail-closed
- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### Phase 6: C2 F-3
- [ ] Step 6-1: `internal/elfmagic` パッケージ新設
- [ ] Step 6-2: `elfanalyzer` 側の `isELFMagic` 等リファクタリング
- [ ] Step 6-3: ELF `Analyze` トップレベル修正
- [ ] Step 6-4: ELF トップレベルのテスト追加
- [ ] Step 6-5: ELF 子依存パース失敗の修正
- [ ] Step 6-6: ELF 子依存パース失敗テスト追加
- [ ] Step 6-7: Mach-O 子依存パース失敗の修正
- [ ] Step 6-8: Mach-O 子依存パース失敗テスト追加
- [ ] Step 6-9: 統合テスト（正常系リグレッション確認）

### PR-5 作成ポイント: dynlib parse failure fail-closed with shared ELF magic
- [ ] グリーンゲート（`make test && make lint`）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 全体
- [ ] `make fmt` 実行
- [ ] `make test` 全件パス
- [ ] `make lint` 警告なし
- [ ] `make deadcode` 警告なし

---

## 7. リスク管理

### 7.1 技術的リスク

| リスク | 影響度 | 緩和策 |
|---|---|---|
| Phase 4 と Phase 6 のファイル競合（`standard_analyzer.go`） | 中 | Phase 4 を先に完了させる。並行実装時はマージ後の解決を明示的に行う |
| PR-2 と PR-4 のファイル競合（`verification/manager_test.go`） | 低 | 両 PR ともテスト関数の追加であり、異なる位置への追加のためマージ競合の可能性は低い。並行実装時は念のため確認する |
| C2 F-3 BFS traversal の blast radius（1つの破壊ライブラリで全依存記録失敗） | 高 | エラーメッセージに失敗した子ライブラリのパスを明示する（`02_architecture.md` 3.2.3節）。デプロイ前に修正後バイナリで実際の record をテストする |
| B3 M1 のシグネチャ変更による呼び出し元への影響漏れ | 低 | `collectVerificationFiles` は非公開関数であり、呼び出し元は `VerifyGroupFiles` 内の 1 箇所のみ。コンパイルエラーで捕捉可能 |
| darwin ビルドタグ付きテスト（Mach-O 系）の CI カバレッジ | 中 | テストは `darwin` ビルドタグを付与。linux CI ではコンパイル確認のみ（`-run '^$'`）。実動作確認は macos ランナーで行う |
| dry-run モードの挙動変更（C2 F-5 / B3 L1 / B3 M1） | 中 | `02_architecture.md` 5.3節の dry-run 影響分析に従い、デプロイ前に修正後バイナリで dry-run 確認を実施 |

### 7.2 スケジュールリスク

- 全フェーズが機能的に独立しているため、並行実装が可能。ただし同一ファイルを変更する Phase 4/6 は順次実行が安全。
- Phase 6 は変更量が最大（新規パッケージ作成＋2ファイル修正＋3テストファイル追加）であり、十分なテスト時間を確保する。

---

## 8. 成功基準

### 8.1 機能完全性

- [ ] 全18件の AC がテストまたは既存テストで検証済み
- [ ] `make test` が全件パス（linux + darwin）
- [ ] `make lint` が警告なし
- [ ] `make deadcode` が警告なし（`isELFMagic` 削除後のデッドコードなし）

### 8.2 品質基準

- [ ] 新規テストはエラー条件（CI で到達可能な全エラーパス）をカバーしている
- [ ] 既存テストにリグレッションがない
- [ ] 新規コードは既存のコード規約に従っている

### 8.3 セキュリティ検証要件

- [ ] C1 F-1: ストア I/O エラーが fail-closed（`AnalysisError`）であること
- [ ] C2 F-3: 子依存パース失敗が解析全体の失敗になること
- [ ] C2 F-5: `HasDynamicLibDeps` の I/O エラーが fail-closed であること
- [ ] B3 M1: パス解決失敗が `VerifyGroupFiles` 全体の失敗になること
- [ ] B3 L1: `DynString` エラーが fail-closed であること
- [ ] A5 Low-3: 未知クラスが Blocking になること

---

## 9. 次のステップ

本実装計画のレビュー完了後、Phase 1 から順に実装を開始する。全フェーズ完了後、`02_architecture.md` 5.4節のロールアウト手順に従い、修正後バイナリで以下の確認を実施する:

1. 実際の運用構成に対する dry-run 実行（想定外の中断がないことの確認）
2. record バイナリの動作確認（破壊ライブラリを含む環境でのエラーハンドリング確認）
3. 構造化ログ（`"reason"` フィールド）の出力確認
