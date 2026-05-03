# 動的リンクライブラリの再帰的システムコール解析 実装計画書

## 進捗状況

- [x] Step 1: スキーマ変更
- [x] Step 2: `binaryanalyzer/syscall_wrapper_libs.go` 新規作成とテスト
- [x] Step 3: `Validator` にキャッシュフィールドと setter 追加
- [x] Step 4: `analyzeOneLibrary()` 実装とユニットテスト
- [x] Step 5: `analyzeLibraries()` 実装とユニットテスト
- [x] Step 6: `updateAnalysisRecord` への統合
- [x] Step 7: runner 側のネットワーク判定拡張
- [x] Step 8: `cmd/record/main.go` の有効化
- [x] Step 9: 統合テスト（AC-1 〜 AC-8）
- [ ] Step 11: AC-9/10/11 対応テスト追加
- [x] Step 10: `make fmt` `make test` `make lint` で品質確認

---

## 各 Step の詳細

### Step 1: スキーマ変更

**対象ファイル**: `internal/fileanalysis/schema.go`

作業内容:
- [x] `CurrentSchemaVersion` を 19 → 20 に変更し、バージョンアップ理由をコメントに追記
- [x] `LibraryAnalysisEntry` 型を追加（`SOName`, `Path`, `SyscallAnalysis`, `SymbolAnalysis` フィールド）
- [x] `Record` 構造体に `LibraryAnalysis []LibraryAnalysisEntry` フィールドを追加
- [x] `SymbolAnalysisData` 構造体に `DetectedLibraryNetworkDeps []string` フィールドを追加
- [x] スキーマバージョンを参照しているテストを 20 に更新

---

### Step 2: `syscall_wrapper_libs.go` 新規作成とテスト

**対象ファイル**:
- 新規: `internal/security/binaryanalyzer/syscall_wrapper_libs.go`
- 新規: `internal/security/binaryanalyzer/syscall_wrapper_libs_test.go`

作業内容:
- [x] `syscallWrapperPrefixes` スライスを定義（`libc`, `libpthread`, `libdl`, `librt`,
  `libgcc_s`, `ld-linux`, `ld-linux-x86-64`, `ld-linux-aarch64`, `linux-vdso`）
- [x] `IsSyscallWrapperLibrary(soname string) bool` 関数を実装（既存の
  `matchesKnownPrefix` を再利用）
- [x] テスト: マッチするケース（`libc.so.6`, `libpthread.so.0`, `ld-linux-x86-64.so.2`,
  `linux-vdso.so.1`）
- [x] テスト: マッチしないケース（`libssl.so.3`, `libcurl.so.4`, `libstdc++.so.6`）
- [x] テスト: 前方一致境界ケース（`libcc.so.1` → `false`, `libcpp.so.1` → `false`）

---

### Step 3: `Validator` にキャッシュフィールドと setter 追加

**対象ファイル**: `internal/filevalidator/validator.go`

作業内容:
- [x] `libraryCacheEntry` 構造体を定義（`entry fileanalysis.LibraryAnalysisEntry`,
  `hasNetwork bool`）
- [x] `Validator` 構造体に `libraryAnalysisCache map[string]libraryCacheEntry` フィールドを追加
- [x] `Validator` 構造体に `libraryAnalysisEnabled bool` フィールドを追加
- [x] `SetLibraryAnalysisEnabled(enabled bool)` メソッドを追加（有効化時にキャッシュを初期化）
- [x] `isKnownVDSO(soname string) bool` プライベートヘルパを追加（`linux-vdso.so.1`,
  `linux-gate.so.1`, `linux-vdso64.so.1`）

---

### Step 4: `analyzeOneLibrary()` 実装とユニットテスト

**対象ファイル**:
- `internal/filevalidator/validator.go`
- 新規: `internal/filevalidator/validator_library_analysis_test.go`（または既存テストファイルへ追記）

作業内容:
- [x] `analyzeOneLibrary(lib fileanalysis.LibEntry) (entry *fileanalysis.LibraryAnalysisEntry, hasNetwork bool, warnings []string, err error)` を実装
  - [x] `.dynsym` シンボル解析: `v.binaryAnalyzer.AnalyzeNetworkSymbols(lib.Path, "")` を呼び出し
  - [x] `NetworkDetected` のとき `hasNetwork = true`、`SymbolAnalysisData` を設定
  - [x] `AnalysisError` のとき `warnings` に追記して継続
  - [x] syscall 命令スキャン: `openELFFile` → `AnalyzeSyscallsFromELF`
  - [x] ELF open 失敗は `errNotELF` なら無視、それ以外は `warnings` に追記して継続
  - [x] `ErrUnsupportedArch` は `warnings` に追記せずスキップ
  - [x] network syscall 判定: `GetSyscallTable(elfFile.Machine)` → `IsNetworkSyscall()` で `hasNetwork` を更新
- [x] テスト: network シンボルを持つライブラリ → `hasNetwork = true`、`SymbolAnalysis` に記録
- [x] テスト: network syscall を持つライブラリ → `hasNetwork = true`、`SyscallAnalysis` に記録
- [x] テスト: ネットワーク系なし → `hasNetwork = false`
- [x] テスト: ファイル不在 → `hasNetwork = false`、`warnings` に追記
- [x] テスト: 非 ELF ライブラリ（`.so` だが ELF でない）→ syscall 解析はスキップ
- [x] テスト: `ErrUnsupportedArch` → `warnings` なし、`SyscallAnalysis` nil

---

### Step 5: `analyzeLibraries()` 実装とユニットテスト

**対象ファイル**: `internal/filevalidator/validator.go`、テストファイル

作業内容:
- [x] `analyzeLibraries(record *fileanalysis.Record) error` を実装
  - [x] `!v.libraryAnalysisEnabled` または `len(record.DynLibDeps) == 0` のときは即時 return nil
  - [x] 各 `DynLibDeps` エントリに対してループ:
    - [x] `isKnownVDSO(lib.SOName)` の場合はスキップ
    - [x] `binaryanalyzer.IsSyscallWrapperLibrary(lib.SOName)` の場合はスキップ
    - [x] `v.libraryAnalysisCache[lib.Path]` でキャッシュヒット確認
    - [x] キャッシュミスのとき `analyzeOneLibrary` を呼び出してキャッシュに保存
  - [x] ネットワーク検出ライブラリの SOName を `slices.Sort` してから `DetectedLibraryNetworkDeps` へ設定
  - [x] `record.SymbolAnalysis` が nil のとき新規作成して設定
  - [x] `record.LibraryAnalysis` に全エントリを設定
  - [x] `record.AnalysisWarnings` を `slices.Sort` で整列
- [x] テスト: `libraryAnalysisEnabled = false` → `LibraryAnalysis` nil
- [x] テスト: 空の `DynLibDeps` → `LibraryAnalysis` nil
- [x] テスト: `libc.so.6` と `libssl.so.3` が依存の場合、`libc` はスキップ、`libssl` のみ解析
- [x] テスト: `linux-vdso.so.1` はスキップ
- [x] テスト: 同一ライブラリを 2 回参照（異なるコマンドが同じライブラリに依存）→ `analyzeOneLibrary` は 1 回のみ
- [x] テスト: `SymbolAnalysis` が nil の場合でも `DetectedLibraryNetworkDeps` が設定される

---

### Step 6: `updateAnalysisRecord` への統合

**対象ファイル**: `internal/filevalidator/validator.go`

作業内容:
- [x] `KnownNetworkLibDeps` ブロックの直後、`analyzeELFSyscalls` の直前に
  `v.analyzeLibraries(record)` 呼び出しを追加
- [x] エラーが返ったとき即時 `return err`

---

### Step 7: runner 側のネットワーク判定拡張

**対象ファイル**: `internal/runner/base/security/network_analyzer.go`

作業内容:
- [x] `isNetworkViaBinaryAnalysis` 内の条件を変更:
  `if hasNetworkSymbol || len(data.KnownNetworkLibDeps) > 0 || len(data.DetectedLibraryNetworkDeps) > 0`
- [x] `DetectedLibraryNetworkDeps` が原因でネットワーク有りと判定した場合のログ出力を追加
- [x] テスト: `DetectedLibraryNetworkDeps` 非空のとき `(true, false)` を返すことを確認

---

### Step 8: `cmd/record/main.go` の有効化

**対象ファイル**: `cmd/record/main.go`

作業内容:
- [x] `v.SetSyscallAnalyzer(...)` 呼び出しの直後に `v.SetLibraryAnalysisEnabled(true)` を追加

---

### Step 9: 統合テスト（AC-1 〜 AC-8）

統合テストは既存のパターン（`cmd/runner/integration_*_test.go`）に倣い、
実際のバイナリまたはテスト用 ELF を使用する。

| テスト | 対応 AC | 確認内容 |
|--------|--------|---------|
| アプリケーションライブラリのネットワークシンボル検出 | AC-1 | `libssl` 等が依存にある場合、`LibraryAnalysis` に記録される |
| アプリケーションライブラリの syscall 検出 | AC-2 | `SyscallAnalysis` に network syscall が記録される |
| `libc.so.6` 除外 | AC-3 | `LibraryAnalysis` に `libc.so.6` が含まれない |
| `ld-linux` 除外 | AC-4 | `LibraryAnalysis` に `ld-linux` 系が含まれない |
| runner ネットワーク判定 | AC-5 | `DetectedLibraryNetworkDeps` 非空でネットワーク有りと判定 |
| セッションキャッシュ | AC-6 | 同一ライブラリの重複解析が発生しない |
| エラー耐性 | AC-7 | ライブラリファイル不在でも記録が完了する |
| 既存機能の回帰 | AC-8 | 既存の `.dynsym` / syscall 解析結果に変化なし |

---

### Step 10: 品質確認

- [x] `make fmt` — gofumpt によるフォーマット
- [x] `make test` — 全テスト通過
- [x] `make lint` — golangci-lint エラーなし

---

### Step 11: AC-9/10/11 対応テスト追加

**対象ファイル**: `internal/filevalidator/validator_library_analysis_test.go`、`internal/security/binaryanalyzer/syscall_wrapper_libs_test.go`

作業内容:
- [ ] `TestAnalyzeLibraries_vdsoExcluded`: `linux-vdso.so.1` を DynLibDeps に含めて実行し、`LibraryAnalysis` に含まれないことを確認（AC-9）
- [ ] `TestAnalyzeLibraries_fileTooLarge`: ファイルサイズが `maxLibraryFileSize` を超えるモックを設定し、解析スキップと `AnalysisWarnings` 追記を確認（AC-10）
- [ ] `analyzeOneLibrary` の冒頭にファイルサイズチェックを実装（詳細仕様 4.5 節）
- [ ] `TestIsSyscallWrapperLibrary_prefixBoundary_libcc`: `libcc.so.1` が `false` を返すことを追加確認（AC-11）
- [ ] `make fmt` / `go test -tags test -v ./...` / `make lint` で品質確認

---

## 依存関係と実施順序

```
Step 1 (スキーマ) → Step 2 (wrapper libs) → Step 3 (Validator fields)
→ Step 4 (analyzeOneLibrary) → Step 5 (analyzeLibraries)
→ Step 6 (updateAnalysisRecord 統合) → Step 7 (runner)
→ Step 8 (main.go) → Step 9 (統合テスト) → Step 10 (品質確認)
```

Step 1 〜 Step 3 は並行して作業可能。
Step 4 は Step 1 と Step 3 の完了後に着手する。
Step 7 は Step 1 の完了後に着手可能（Step 4〜6 と並行可）。
