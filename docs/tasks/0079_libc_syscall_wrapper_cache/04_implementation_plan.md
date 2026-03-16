# 実装計画書: libc システムコールラッパー関数キャッシュ

## 進捗サマリー

- [ ] フェーズ 1: 基盤整備（型定義・既存 API 拡張）
- [ ] フェーズ 2: `libccache` パッケージの実装
- [ ] フェーズ 3: `Validator` の統合
- [ ] フェーズ 4: `cmd/record` のリファクタリング
- [ ] フェーズ 5: 統合テスト・最終確認

---

## フェーズ 1: 基盤整備（型定義・既存 API 拡張）

### 1-1. `common.SyscallInfo` への `Source` フィールド追加

- [ ] `internal/common/syscall_types.go` に `Source string \`json:"source,omitempty"\`` を追加する
- [ ] `internal/common/syscall_types_test.go` に `Source` フィールドの JSON `omitempty` 動作テストを追加する
- [ ] `make fmt && make test && make lint` で既存テストがパスすることを確認する

### 1-2. `elfanalyzer.SyscallAnalyzer.AnalyzeSyscallsInRange` の追加

- [ ] `internal/runner/security/elfanalyzer/syscall_analyzer.go` に以下のメソッドを追加する:
  ```go
  func (a *SyscallAnalyzer) AnalyzeSyscallsInRange(
      code []byte,
      sectionBaseAddr uint64,
      startOffset, endOffset int,
      machine elf.Machine,
  ) ([]common.SyscallInfo, error)
  ```
- [ ] `backwardScanForSyscallNumber` への `windowStart` を `max(windowStart, startOffset)` でクランプする処理を実装する
- [ ] `internal/runner/security/elfanalyzer/syscall_analyzer_test.go` に `AnalyzeSyscallsInRange` のテストを追加する:
  - [ ] 正常系: syscall 命令を含む範囲で正しく検出される
  - [ ] 境界チェック: `startOffset` でクランプが効き、隣接バイトが混入しない
  - [ ] 非対応アーキテクチャで `ErrUnsupportedArchitecture` が返る
- [ ] `make fmt && make test && make lint` でパスすることを確認する

### 1-3. `fileanalysis/schema.go` コメント更新

- [ ] `internal/fileanalysis/schema.go` の `SyscallAnalysis` フィールドのコメントを「静的 ELF バイナリ、および libc 経由の syscall が検出された動的 ELF バイナリ」に更新する

---

## フェーズ 2: `libccache` パッケージの実装

### 2-1. `schema.go` の作成

- [ ] `internal/libccache/schema.go` を作成する:
  - [ ] `LibcCacheSchemaVersion = 1` 定数
  - [ ] `LibcCacheFile` 構造体
  - [ ] `WrapperEntry` 構造体

### 2-2. `errors.go` の作成

- [ ] `internal/libccache/errors.go` を作成する:
  - [ ] `ErrLibcFileNotAccessible`
  - [ ] `ErrExportSymbolsFailed`
  - [ ] `ErrCacheWriteFailed`
  - [ ] `SourceLibcSymbolImport = "libc_symbol_import"` 定数

### 2-3. `analyzer.go` の実装

- [ ] `internal/libccache/analyzer.go` を作成する:
  - [ ] `MaxWrapperFunctionSize = 256` 定数
  - [ ] `LibcWrapperAnalyzer` 型と `NewLibcWrapperAnalyzer()` コンストラクタ
  - [ ] `Analyze(libcELFFile *elf.File) ([]WrapperEntry, error)` の実装:
    - [ ] `.text` セクションのデータとベースアドレスの取得
    - [ ] `elf.File.DynamicSymbols()` によるエクスポートシンボルの列挙
    - [ ] `STB_LOCAL`/`SHN_UNDEF`/非 `STT_FUNC` シンボルの除外
    - [ ] サイズフィルタ（256 バイト超を除外）
    - [ ] シンボルアドレスから `.text` セクション内オフセットへの変換
    - [ ] `AnalyzeSyscallsInRange` の呼び出しと結果の検査
    - [ ] `DeterminationMethod == "immediate"` かつ `Number >= 0` かつ全 Number 同一のフィルタ
    - [ ] `WrapperEntry` の収集と `Number` 昇順ソート

- [ ] `internal/libccache/analyzer_test.go` を作成する（インメモリ ELF バイナリを使用）:
  - [ ] syscall 命令を含む関数（≤256B）が検出されること
  - [ ] 256 バイト超の関数が除外されること
  - [ ] 複数の異なる syscall 番号を含む関数が除外されること
  - [ ] 同一 syscall 番号の syscall 命令を複数持つ関数は採用されること
  - [ ] syscall 命令を含まない関数が除外されること
  - [ ] `WrapperEntry` が `Number` 昇順でソートされていること
  - [ ] `DeterminationMethod != "immediate"` の関数が除外されること
  - [ ] 非対応アーキテクチャで `ErrUnsupportedArchitecture` が返ること

- [ ] `make fmt && make test && make lint` でパスすることを確認する

### 2-4. `cache.go` の実装

- [ ] `internal/libccache/cache.go` を作成する:
  - [ ] `LibcCacheManager` 型と `NewLibcCacheManager()` コンストラクタ（`lib-cache/` ディレクトリの自動作成含む）
  - [ ] `GetOrCreate(libcPath, libcHash string) ([]WrapperEntry, error)` の実装:
    - [ ] キャッシュファイルパスの生成（`pathencoding.Encode`）
    - [ ] キャッシュファイルの読み込みと有効性判定（3 条件チェック）
    - [ ] キャッシュ MISS 時の libc 解析と書き込み
    - [ ] `ErrLibcFileNotAccessible`/`ErrCacheWriteFailed` エラーハンドリング
    - [ ] `ErrUnsupportedArchitecture` のラップなし伝播

- [ ] `internal/libccache/cache_test.go` を作成する（テンポラリディレクトリを使用）:
  - [ ] キャッシュ未存在時に解析・生成されること
  - [ ] ハッシュ一致時にキャッシュが再利用されること（アナライザーが呼ばれないこと）
  - [ ] ハッシュ不一致時にキャッシュが再生成されること
  - [ ] キャッシュファイルが破損している場合に再解析されること
  - [ ] `schema_version` 不一致時にキャッシュが再生成されること
  - [ ] `syscall_wrappers` が `number` 昇順でソートされていること
  - [ ] libc ファイルが読み取れない場合にエラーを返すこと
  - [ ] キャッシュファイルの書き込みに失敗した場合にエラーを返すこと

- [ ] `make fmt && make test && make lint` でパスすることを確認する

### 2-5. `matcher.go` の実装

- [ ] `internal/libccache/matcher.go` を作成する:
  - [ ] `SyscallNumberTable` インターフェース定義
  - [ ] `ImportSymbolMatcher` 型と `NewImportSymbolMatcher()` コンストラクタ
  - [ ] `Match(importSymbols []string, wrappers []WrapperEntry) []common.SyscallInfo` の実装:
    - [ ] `WrapperEntry` を `Name` キーのマップに変換
    - [ ] インポートシンボルとの完全一致照合
    - [ ] `SyscallInfo` の生成（`Source = "libc_symbol_import"`, `Location = 0`, `DeterminationMethod = "immediate"`）
    - [ ] `Number` 重複の排除

- [ ] `internal/libccache/matcher_test.go` を作成する:
  - [ ] シンボルがキャッシュに存在する場合に `SyscallInfo` が生成されること
  - [ ] シンボルがキャッシュに存在しない場合は無視されること
  - [ ] 生成された `SyscallInfo` の `Source` が `"libc_symbol_import"` であること
  - [ ] 生成された `SyscallInfo` の `Location` が `0` であること
  - [ ] 生成された `SyscallInfo` の `DeterminationMethod` が `"immediate"` であること
  - [ ] 同一 `Number` のエントリが重複しないこと

- [ ] `make fmt && make test && make lint` でパスすることを確認する

---

## フェーズ 3: `Validator` の統合

### 3-1. パッケージ非公開ヘルパー関数の実装

- [ ] `internal/filevalidator/validator.go` に以下のヘルパー関数を追加する:
  - [ ] `openELFFile(fs safefileio.FileSystem, filePath string) (*elf.File, error)`: SafeOpenFile + elf.NewFile
  - [ ] `extractUNDSymbols(elfFile *elf.File) []string`: `.dynsym` UND シンボルの抽出
  - [ ] `findLibcEntry(deps *fileanalysis.DynLibDepsData) *fileanalysis.LibEntry`: libc エントリの特定
  - [ ] `mergeSyscallInfos(libc, direct []common.SyscallInfo) []common.SyscallInfo`: Number で一意化・direct 優先
  - [ ] `buildSyscallAnalysisData(all []common.SyscallInfo, direct []common.SyscallInfo) *fileanalysis.SyscallAnalysisData`: SyscallAnalysisData の構築

### 3-2. `Validator` へのフィールドとセッタ追加

- [ ] `Validator` 構造体に `libcCacheMgr` と `syscallAnalyzer` フィールドを追加する
- [ ] `SetLibcCacheManager(m *libccache.LibcCacheManager)` を実装する
- [ ] `SetSyscallAnalyzer(a *elfanalyzer.SyscallAnalyzer)` を実装する

### 3-3. `updateAnalysisRecord()` のコールバック拡張

- [ ] `store.Update()` コールバック内に以下を統合する:
  - [ ] `openELFFile()` による ELF オープン（ErrNotELF → スキップ・記録保存）
  - [ ] `findLibcEntry()` による libc エントリ特定
  - [ ] `extractUNDSymbols()` による UND シンボル抽出
  - [ ] `libcCacheMgr.GetOrCreate()` による libc キャッシュ取得（`ErrUnsupportedArchitecture` → スキップ・直接解析へ）
  - [ ] `matcher.Match()` によるインポートシンボル照合
  - [ ] `syscallAnalyzer.AnalyzeSyscallsFromELF()` による直接 syscall 解析（`ErrUnsupportedArchitecture` → スキップ）
  - [ ] `mergeSyscallInfos()` による統合
  - [ ] `buildSyscallAnalysisData()` による `record.SyscallAnalysis` 設定

### 3-4. `Validator` のテスト追加・更新

- [ ] `internal/filevalidator/validator_test.go` に以下のテストを追加する:
  - [ ] libc あり動的バイナリ: libc キャッシュから `SyscallInfo` が生成されること
  - [ ] 直接 syscall と libc import の重複は direct 優先で統合されること
  - [ ] 保存順序: キャッシュが成功した後にのみ記録ファイルが保存されること
  - [ ] キャッシュ失敗時: コールバックがエラーを返し記録ファイルが保存されないこと
  - [ ] `ErrUnsupportedArchitecture` 時: libc 解析をスキップして記録保存が続行すること
  - [ ] 非 ELF ファイル: syscall 解析全体をスキップして記録保存が続行すること

- [ ] 上記のヘルパー関数 (`openELFFile`, `extractUNDSymbols`, `findLibcEntry`, `mergeSyscallInfos`, `buildSyscallAnalysisData`) の単体テストを追加する:
  - [ ] `buildSyscallAnalysisData`: `HasUnknownSyscalls` が `direct` 引数の `Number < 0` エントリの有無から計算されること（libc import 由来の `Number < 0` は対象外）

- [ ] `make fmt && make test && make lint` でパスすることを確認する

---

## フェーズ 4: `cmd/record` のリファクタリング

### 4-1. `syscallAnalysisContext` の廃止

- [ ] `cmd/record/main.go` から `syscallAnalysisContext` 型と `newSyscallAnalysisContext()` を削除する
- [ ] `deps.syscallContextFactory` フィールドを削除する
- [ ] `defaultDeps()` から `syscallContextFactory` を削除する
- [ ] `processFiles()` から `syscallCtx.analyzeFile()` 呼び出しを削除する
- [ ] `run()` から `syscallContextFactory` の呼び出しを削除する

### 4-2. libc キャッシュマネージャーと syscall アナライザーの注入

- [ ] `run()` 内で `filevalidator.Validator` に対して:
  - [ ] `elfanalyzer.NewSyscallAnalyzer()` を生成して `SetSyscallAnalyzer()` で設定する
  - [ ] `libccache.NewLibcCacheManager()` を生成して `SetLibcCacheManager()` で設定する
  - [ ] `lib-cache/` ディレクトリパス（`filepath.Join(cfg.hashDir, "lib-cache")`）を使用する

### 4-3. `cmd/record` のテスト更新

- [ ] `cmd/record` のテストを更新して `syscallAnalysisContext` 依存を除去する:
  - [ ] `deps.syscallContextFactory` フィールドへの参照をすべて削除する
  - [ ] `processFiles` から `syscallCtx.analyzeFile()` 呼び出しを削除したことで、`SyscallAnalysis` が `Validator` 経由で正しく設定されることを `mock Validator` または統合テストで確認する
  - [ ] `run()` が `SetSyscallAnalyzer` / `SetLibcCacheManager` を呼び出すことを `deps` 差し替えで確認する（または既存の結合テストがカバーしていることを確認する）
- [ ] `make fmt && make test && make lint` でパスすることを確認する

---

## フェーズ 5: 統合テスト・最終確認

### 5-1. 統合テストの作成

- [ ] `internal/libccache/integration_test.go` を作成する（`//go:build integration`）:
  - [ ] GCC が利用できない環境では `t.Skip()` でスキップ
  - [ ] テスト用 C プログラム（mkdir syscall を呼ぶ最小プログラム）をオンデマンドコンパイル
  - [ ] コンパイルしたバイナリを `record` した際に `mkdir` syscall（番号 83）が `DetectedSyscalls` に含まれることを確認
  - [ ] `source: "libc_symbol_import"` の `SyscallInfo` が存在することを確認
  - [ ] `Location` が `0` であることを確認
  - [ ] `lib-cache/` 以下にキャッシュファイルが生成されることを確認
  - [ ] 2 回目の record 実行でキャッシュが再利用される（libc を再解析しない）ことを確認

### 5-2. 受け入れ条件の最終確認

- [ ] **AC-1**: `common.SyscallInfo` に `Source` フィールドが追加されていること、既存テストがパスすること
- [ ] **AC-2**: `record` 実行時にキャッシュファイルが `lib-cache/` 以下に生成されること、各フィールドが仕様通りであること、`number` 昇順ソートされていること、サイズフィルタと複数 syscall フィルタが機能していること
- [ ] **AC-3**: キャッシュ HIT/MISS・再生成・破損時の動作、保存順序（キャッシュ先行）、エラー時の記録ファイル未保存、非対応アーキテクチャでの継続
- [ ] **AC-4**: GCC でビルドした専用バイナリを `record` した際に `mkdir`（番号 83）が検出されること、`source: "libc_symbol_import"`・`location: 0`・重複なし・direct 優先
- [ ] **AC-5**: 静的 ELF バイナリの既存フローが変わらないこと、`make test` 全体がパスすること

### 5-3. 最終ビルド・テスト

- [ ] `make build` でビルドが成功することを確認する
- [ ] `make test` で全テストがパスすることを確認する
- [ ] `make lint` でリント警告がないことを確認する
- [ ] `go test -tags integration -v ./internal/libccache/...` で統合テストがパスすることを確認する（GCC が利用可能な環境）

---

## 作業順序の根拠

```
フェーズ 1（基盤）
  → フェーズ 2（libccache パッケージ）
    → フェーズ 3（Validator 統合）
      → フェーズ 4（cmd/record リファクタリング）
        → フェーズ 5（統合テスト・確認）
```

- フェーズ 1 で `SyscallInfo.Source` フィールドと `AnalyzeSyscallsInRange` を追加することで、フェーズ 2 の `libccache` パッケージが依存する API が揃う
- フェーズ 2 が完了してから `Validator` に統合することで、各コンポーネントを独立してテストできる
- フェーズ 4 の `cmd/record` リファクタリング（`syscallAnalysisContext` の廃止）は、フェーズ 3 の `Validator` 統合が完了してから行う
- フェーズ 5 の統合テストは、すべての実装が揃った段階で end-to-end の動作を確認する

## 注意事項

- 各フェーズの末尾で `make fmt && make test && make lint` を実行し、回帰が発生していないことを確認する
- `//go:build test` タグが必要なファイルは既存のパターン（`syscall_analyzer_integration_test.go` 等）に倣う
- `git commit` は実施しない（ユーザーが明示的に要求した場合のみ）
- ツール呼び出しはシーケンシャルに行う（1 ツール → 結果確認 → 次のツール）
