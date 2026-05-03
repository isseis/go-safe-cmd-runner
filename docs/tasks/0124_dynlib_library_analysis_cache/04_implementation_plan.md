# ライブラリ解析結果の共通キャッシュ化 実装計画書

## 進捗状況

- [ ] Step 1: `internal/dynlibcache` パッケージ基盤作成
- [ ] Step 2: `internal/dynlibcache/cache.go` 実装と単体テスト
- [ ] Step 3: `internal/fileanalysis/schema.go` 変更（フィールド削除・バージョン更新）
- [ ] Step 4: `internal/fileanalysis/dyn_lib_deps_store.go` 新規作成と単体テスト
- [ ] Step 5: `internal/filevalidator/validator.go` リファクタリング
- [ ] Step 6: `internal/runner/base/security/network_analyzer.go` 変更
- [ ] Step 7: `cmd/record/main.go` / `cmd/runner/main.go` 配線
- [ ] Step 8: 統合テスト（AC-4, AC-9）
- [ ] Step 9: `make fmt` / `make test` / `make lint` で品質確認

---

## 各 Step の詳細

### Step 1: `internal/dynlibcache` パッケージ基盤作成

**対象ファイル**（すべて新規）:
- `internal/dynlibcache/schema.go`
- `internal/dynlibcache/errors.go`
- `internal/dynlibcache/interfaces.go`

作業内容:

- [ ] `schema.go`: `CacheSchemaVersion = 1` 定数を定義
- [ ] `schema.go`: `LibAnalysisCacheFile` 型を定義（フィールド: `SchemaVersion`, `LibPath`, `LibHash`, `SyscallAnalysis`, `SymbolAnalysis`, `DynamicLoadSymbols`）
- [ ] `schema.go`: `LibAnalysisResult` 型を定義（フィールド: `SyscallAnalysis`, `SymbolAnalysis`, `DynamicLoadSymbols`, `Warnings []string`）
  - `Warnings` は解析中に発生した一時的な警告（キャッシュファイルには保存しない）
- [ ] `errors.go`: `ErrCacheMiss` 変数を定義
- [ ] `interfaces.go`: `LibraryAnalyzer` インタフェースを定義
  ```go
  type LibraryAnalyzer interface {
      Analyze(libPath string) (*LibAnalysisCacheFile, []string, error)
  }
  ```
- [ ] `interfaces.go`: `CacheManagerInterface` インタフェースを定義（`GetOrCreate`, `Get` メソッド）
  ```go
  type CacheManagerInterface interface {
      GetOrCreate(libPath, libHash string) (*LibAnalysisResult, error)
      Get(libPath, libHash string) (*LibAnalysisResult, error)
  }
  ```
  - `GetOrCreate`: record 時に使用（cache miss 時に解析・書き込み）
  - `Get`: runner 時に使用（読み取り専用。miss → `ErrCacheMiss`）

---

### Step 2: `internal/dynlibcache/cache.go` 実装と単体テスト

**対象ファイル**（すべて新規）:
- `internal/dynlibcache/cache.go`
- `internal/dynlibcache/cache_test.go`

**作業内容（実装）:**

- [ ] `CacheManager` 構造体を定義（フィールド: `cacheDir string`, `fs safefileio.FileSystem`, `analyzer LibraryAnalyzer`, `pathEnc *pathencoding.SubstitutionHashEscape`）
- [ ] `NewCacheManager(cacheDir string, fs safefileio.FileSystem, analyzer LibraryAnalyzer) (*CacheManager, error)` を実装
  - `cacheDir` が存在しない場合は `os.MkdirAll` で自動作成（パーミッション `0o755`）
  - `pathencoding.NewSubstitutionHashEscape()` で pathEnc を初期化（`libccache.NewLibcCacheManager` と同じパターン）
- [ ] `(m *CacheManager) GetOrCreate(libPath, libHash string) (*LibAnalysisResult, error)` を実装
  - `pathEnc.Encode(libPath + "#" + libHash)` でキャッシュファイルパスを生成
  - ファイル読み込み → JSON パース（失敗 → Cache Miss）
  - `SchemaVersion` 不一致 → Cache Miss
  - `LibHash` 不一致 → Cache Miss
  - Cache Hit: `LibAnalysisResult` を構築して返す（Warnings は空）
  - Cache Miss: `analyzer.Analyze(libPath)` を呼ぶ → `LibAnalysisCacheFile` を `safefileio` で書き込み → `LibAnalysisResult` を返す
  - 書き込み失敗はエラーとして返す
- [ ] `(m *CacheManager) Get(libPath, libHash string) (*LibAnalysisResult, error)` を実装
  - 読み取り専用。ファイルが存在しない・ハッシュ不一致・スキーマ不一致 → `ErrCacheMiss` を返す
  - 解析を行わない

**作業内容（テスト）:**

- [ ] `TestCacheManager_GetOrCreate_Miss`: cache miss → `analyzer.Analyze` が呼ばれ、キャッシュファイルが書き込まれる（AC-1 の永続化側、AC-2）
- [ ] `TestCacheManager_GetOrCreate_Hit`: 2 回目の `GetOrCreate` → `analyzer.Analyze` が呼ばれない（AC-1）
- [ ] `TestCacheManager_GetOrCreate_HashMismatch`: `libHash` 不一致 → 再解析される（AC-2）
- [ ] `TestCacheManager_GetOrCreate_SchemaVersionMismatch`: スキーマ不一致 → 再解析される
- [ ] `TestCacheManager_GetOrCreate_CorruptFile`: JSON パース失敗 → 再解析される（AC-5 の record 側）
- [ ] `TestCacheManager_GetOrCreate_WriteFailure`: 書き込み失敗 → エラーが返る
- [ ] `TestCacheManager_Get_Hit`: キャッシュが存在しハッシュ一致 → 結果が返る
- [ ] `TestCacheManager_Get_Miss`: キャッシュが存在しない → `ErrCacheMiss` が返る
- [ ] `TestCacheManager_Get_HashMismatch`: ハッシュ不一致 → `ErrCacheMiss` が返る

---

### Step 3: `internal/fileanalysis/schema.go` 変更

**対象ファイル**: `internal/fileanalysis/schema.go`

作業内容:

- [ ] `CurrentSchemaVersion` を 20 → 21 に変更し、変更理由をコメントに追記
- [ ] `LibraryAnalysisEntry` 型を削除
- [ ] `Record.LibraryAnalysis []LibraryAnalysisEntry` フィールドを削除
- [ ] `SymbolAnalysisData.DetectedLibraryNetworkDeps []string` フィールドを削除
- [ ] スキーマバージョン 20 を参照している全テストを 21 に更新
- [ ] `LibraryAnalysisEntry` を参照している全コードをコンパイルエラーに任せて特定・修正

---

### Step 4: `internal/fileanalysis/dyn_lib_deps_store.go` 新規作成と単体テスト

**対象ファイル**（すべて新規）:
- `internal/fileanalysis/dyn_lib_deps_store.go`
- `internal/fileanalysis/dyn_lib_deps_store_test.go`

**作業内容（実装）:**

- [ ] `DynLibDepsStore` インタフェースを定義
  - `LoadDynLibDeps(filePath string, contentHash string) ([]LibEntry, error)` メソッド
  - `contentHash` 不一致 → `ErrHashMismatch`
  - レコードが存在しない → `ErrRecordNotFound`
  - DynLibDeps が未記録 → `(nil, nil)`
- [ ] `dynLibDepsStore` 型（非公開）を定義し `DynLibDepsStore` を実装
  - 既存の `networkSymbolStore`（`network_symbol_store.go`）と同じパターンで実装
- [ ] `NewDynLibDepsStore(store *Store) DynLibDepsStore` を定義

**作業内容（テスト）:**

- [ ] `TestDynLibDepsStore_Found`: DynLibDeps が存在しハッシュ一致 → 正しい LibEntry スライスが返る
- [ ] `TestDynLibDepsStore_RecordNotFound`: レコードが存在しない → `ErrRecordNotFound`
- [ ] `TestDynLibDepsStore_HashMismatch`: ハッシュ不一致 → `ErrHashMismatch`
- [ ] `TestDynLibDepsStore_SchemaVersionMismatch`: スキーマバージョン不一致 → `SchemaVersionMismatchError`
- [ ] `TestDynLibDepsStore_EmptyDynLibDeps`: DynLibDeps が空のレコード → `(nil, nil)`

---

### Step 5: `internal/filevalidator/validator.go` リファクタリング

**対象ファイル**:
- `internal/filevalidator/validator.go`
- `internal/filevalidator/validator_library_analysis_test.go`（更新）

**作業内容（実装）:**

- [ ] `libraryCacheEntry` 構造体を削除（`LibraryAnalysisEntry` が削除されるため不要になる）
- [ ] `Validator` 構造体の `libraryAnalysisCache` フィールドを `map[string]*dynlibcache.LibAnalysisResult`（処理済み解析結果）に変更
- [ ] `Validator` 構造体に `libAnalysisCacheManager dynlibcache.CacheManagerInterface` フィールドを追加
- [ ] `SetLibraryAnalysisEnabled` を `SetLibraryAnalysisCacheManager(m dynlibcache.CacheManagerInterface)` に変更
- [ ] `validatorLibraryAnalyzer` 型を新規追加（`filevalidator` パッケージ内）
  - `dynlibcache.LibraryAnalyzer` インタフェースを実装
  - `Analyze(libPath string) (*dynlibcache.LibAnalysisCacheFile, []string, error)` で既存の `analyzeOneLibrary` のロジックを再利用
- [ ] `analyzeLibraries()` を変更
  - `libAnalysisCacheManager` が nil → 早期リターン
  - 処理済み解析結果（`map[string]*dynlibcache.LibAnalysisResult`）で同一セッション内重複を防ぎ、warnings 伝播を維持する
  - `libAnalysisCacheManager.GetOrCreate(lib.Path, lib.Hash)` を呼び出す
  - 成功: `result.Warnings` を `record.AnalysisWarnings` に追記
  - 失敗（ファイル不在等の error）: error をそのまま上位へ返す（FR-3.6.2）
  - `record.LibraryAnalysis` / `DetectedLibraryNetworkDeps` への書き込みを削除
- [ ] `analyzeOneLibrary` のライブラリファイル不在処理を warning → error に変更したうえで、`validatorLibraryAnalyzer.Analyze` の内部実装として使う（外部から呼ばれなくなる）（FR-3.6.2、タスク 0123 AC-7 の訂正）
- [ ] `cmd/record/main.go` の `SetLibraryAnalysisEnabled(true)` 呼び出しを `SetLibraryAnalysisCacheManager` に変更

**作業内容（テスト更新）:**

- [ ] `TestAnalyzeLibraries_*` のうち `record.LibraryAnalysis` を検証しているテストを、CacheManager の `GetOrCreate` 呼び出し確認に変更
- [ ] `TestAnalyzeLibraries_disabled` → `cacheManager が nil` ケースに変更
- [ ] `TestAnalyzeLibraries_sessionCache` → 処理済み解析結果により同一パスが 2 回解析されず、warnings が再伝播されることを確認（AC-1 のセッション側）
- [ ] `TestAnalyzeLibraries_excludesWrapperAndVDSO` → wrapper/VDSO が `GetOrCreate` の対象外であることを確認（AC-7）
- [ ] `TestAnalyzeOneLibrary_*` → `validatorLibraryAnalyzer.Analyze` のテストとして維持
- [ ] `TestAnalyzeLibraries_recordHasNoDynLibAnalysisField`: `analyzeLibraries` 後の record JSON に `library_analysis` フィールドが含まれないことを確認（AC-6）
- [ ] `TestAnalyzeLibraries_dynLibDepsPreservedOnCacheHit`: キャッシュヒット時も `record.DynLibDeps` の soname/path/hash が正しく記録されることを確認（AC-3）
- [ ] `TestValidatorLibraryAnalyzer_Analyze_fileTooLarge`: ファイルサイズ 1 GB 超でモック FileSystem から Stat が超過サイズを返すとき、`Analyze()` が警告を返し `SyscallAnalysis` が nil になることを確認（AC-10、タスク 0123 未作成テスト）
- [ ] `TestAnalyzeLibraries_fileTooLargeWarningPropagated`: サイズ超過ライブラリが含まれる DynLibDeps で `analyzeLibraries()` を呼ぶと、`record.AnalysisWarnings` に警告が追記され処理が継続されることを確認（AC-11、タスク 0123 未作成テスト）
- [ ] `TestValidatorLibraryAnalyzer_Analyze_missingFileReturnsError`: ライブラリファイルが存在しないとき `Analyze()` が warning ではなく error を返すことを確認（タスク 0123 `TestAnalyzeOneLibrary_missingFileAddsWarning` の置き換え）
- [ ] `TestAnalyzeLibraries_missingLibFileReturnsError`: 存在しないパスが含まれる DynLibDeps で `analyzeLibraries()` を呼ぶと error が返されることを確認（AC-12、FR-3.6.2）
- [ ] セッション継続の統合テスト: 複数ファイルのバッチ処理で 1 ファイルが `analyzeLibraries` エラーになっても次ファイルの処理が継続されることを確認（AC-12）
- [ ] `TestAnalyzeLibraries_vdsoExcluded`: `linux-vdso.so.1` のみの DynLibDeps で `analyzeLibraries()` を呼ぶと、モック `CacheManagerInterface` の `GetOrCreate` が呼ばれないことを確認（AC-13、タスク 0123 AC-9 の専用テスト未作成）

---

### Step 6: `internal/runner/base/security/network_analyzer.go` 変更

**対象ファイル**:
- `internal/runner/base/security/network_analyzer.go`
- `internal/runner/base/security/network_analyzer_test.go`（更新）

**作業内容（実装）:**

- [ ] `NetworkAnalyzer` 構造体に `depsStore fileanalysis.DynLibDepsStore` フィールドを追加
- [ ] `NetworkAnalyzer` 構造体に `libCache dynlibcache.CacheManagerInterface` フィールドを追加
- [ ] `NewNetworkAnalyzerWithLibCache` コンストラクタを追加（`depsStore`, `libCache` を受け取る）
- [ ] `buildAnalysisOutputFromSymbolData` の `DetectedLibraryNetworkDeps` 参照を削除
- [ ] `isNetworkViaBinaryAnalysis()` に DynLibDeps ベースのシグナル集計を追加
  - 既存の `SymbolAnalysisData` 判定の後に実行
  - `depsStore.LoadDynLibDeps(cmdPath, contentHash)` を呼び出す
  - `ErrRecordNotFound` → `(true, true)` 高リスク（fail-closed）
  - `ErrHashMismatch`, `SchemaVersionMismatchError`, その他 → `(true, true)` 高リスク
  - wrapper / VDSO 依存は `record` 側と同じ除外規則でスキップ
  - 各 `dep` に対して `libCache.Get(dep.Path, dep.Hash)` を呼び出す
  - `ErrCacheMiss` またはその他エラー → `(true, true)` 高リスク扱いとしてエラーをログに記録して返す
  - `result.SyscallAnalysis` / `result.SymbolAnalysis` / `result.DynamicLoadSymbols` を runner が評価して `has_network_signal` / `has_dynamic_load_signal` を内部導出

**作業内容（テスト更新）:**

- [ ] `TestIsNetworkViaBinaryAnalysis_DetectedLibraryNetworkDeps` を削除
  （`DetectedLibraryNetworkDeps` フィールドが schema から削除されるため）
- [ ] `TestIsNetworkViaBinaryAnalysis_DynLibDepsNetwork`: DynLibDeps に network シグナルを含む `syscall_analysis` / `symbol_analysis` を持つライブラリ → `(true, false)` が返る（AC-4）
- [ ] `TestIsNetworkViaBinaryAnalysis_DynLibDepsDynamicLoad`: DynLibDeps に `dynamic_load_symbols` を持つライブラリ → `(true, true)` が返る（AC-9）
- [ ] `TestIsNetworkViaBinaryAnalysis_DynLibDepsCacheMiss`: `Get` が `ErrCacheMiss` を返す → `(true, true)` が返る
- [ ] `TestIsNetworkViaBinaryAnalysis_DynLibDepsRecordNotFound`: `LoadDynLibDeps` が `ErrRecordNotFound` → `(true, true)` が返る（fail-closed）
- [ ] `TestIsNetworkViaBinaryAnalysis_DynLibDepsHashMismatch`: `LoadDynLibDeps` が `ErrHashMismatch` → `(true, true)` が返る
- [ ] `TestIsNetworkViaBinaryAnalysis_NilDepsStore`: `depsStore` が nil → 既存の `SymbolAnalysisData` 判定のみ（後方互換）

---

### Step 7: `cmd/record/main.go` / `cmd/runner/main.go` 配線

**対象ファイル**:
- `cmd/record/main.go`
- `cmd/runner/main.go`

作業内容:

- [ ] `cmd/record/main.go`: `dynlibcache.NewCacheManager(cacheDir, fs, analyzer)` を生成
  - `cacheDir` はコマンドライン引数またはデフォルト値（例: hashDir + `/dynlibcache`）から取得
  - `analyzer` は `validatorLibraryAnalyzer` のインスタンス（Step 5 で定義）
  - `v.SetLibraryAnalysisCacheManager(cacheManager)` を呼び出す
- [ ] `cmd/runner/main.go`: `dynlibcache.NewCacheManager(cacheDir, fs, nil)` を生成
  - runner 側では `analyzer` は不要（`Get` のみ使用）
  - `CacheManagerInterface` を `NewNetworkAnalyzerWithLibCache` に渡す
  - `NewDynLibDepsStore(store)` を生成して `NewNetworkAnalyzerWithLibCache` に渡す

---

### Step 8: 統合テスト（AC-4, AC-9）

**対象ファイル**: 既存の統合テストファイルへ追記、または新規テストファイル

対応 AC:

| AC | テスト内容 | テストファイル |
|----|---------|-------------|
| AC-3 | キャッシュヒット時も `record.DynLibDeps` に正しい soname/path/hash が記録される | `validator_test.go` |
| AC-4 | dynlibcache 経由のネットワーク判定が 0123 ベースラインと同等（libssl.so.3 を依存に持つバイナリが network=true になる） | `network_analyzer_test.go` |
| AC-6 | record JSON に `library_analysis` フィールドが含まれない | `validator_test.go` |
| AC-9 | `dynamic_load_symbols` を持つライブラリを依存に持つ場合に runner が内部導出した `has_dynamic_load_signal` により高リスク判定 | `network_analyzer_test.go` |

---

### Step 9: `make fmt` / `make test` / `make lint` で品質確認

- [ ] `make fmt` がエラーなく完了する
- [ ] `go test -tags test -v ./...` がすべて PASS する
- [ ] `make lint` がエラーなく完了する（AC-8）
