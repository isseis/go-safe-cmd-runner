# 詳細仕様書: libc システムコールラッパー関数キャッシュ

## 1. ファイル構成

### 1.1 新規作成ファイル

| ファイルパス | 内容 |
|-------------|------|
| `internal/libccache/schema.go` | `LibcCacheFile`, `WrapperEntry`, `LibcCacheSchemaVersion` 定数 |
| `internal/libccache/errors.go` | `ErrLibcFileNotAccessible` 等のエラー型 |
| `internal/libccache/analyzer.go` | `LibcWrapperAnalyzer` |
| `internal/libccache/cache.go` | `LibcCacheManager` |
| `internal/libccache/matcher.go` | `ImportSymbolMatcher` |
| `internal/libccache/analyzer_test.go` | `LibcWrapperAnalyzer` のユニットテスト |
| `internal/libccache/cache_test.go` | `LibcCacheManager` のコンポーネントテスト |
| `internal/libccache/matcher_test.go` | `ImportSymbolMatcher` のユニットテスト |
| `internal/libccache/integration_test.go` | 統合テスト（`//go:build integration`） |

### 1.2 変更ファイル

| ファイルパス | 変更内容 |
|-------------|---------|
| `internal/common/syscall_types.go` | `SyscallInfo` に `Source` フィールド追加 |
| `internal/runner/security/elfanalyzer/syscall_analyzer.go` | `AnalyzeSyscallsInRange` メソッド追加 |
| `internal/fileanalysis/schema.go` | `SyscallAnalysis` コメント更新 |
| `internal/filevalidator/validator.go` | `libcCacheMgr`/`syscallAnalyzer` フィールド追加、`updateAnalysisRecord()` 拡張 |
| `cmd/record/main.go` | `processFiles` から独立した `analyzeFile()` 呼び出しを削除、`syscallAnalysisContext` を廃止 |

---

## 2. `internal/common/syscall_types.go` 変更仕様

### 2.1 `SyscallInfo` への `Source` フィールド追加

```go
type SyscallInfo struct {
    Number              int    `json:"number"`
    Name                string `json:"name,omitempty"`
    IsNetwork           bool   `json:"is_network"`
    Location            uint64 `json:"location"`
    DeterminationMethod string `json:"determination_method"`
    Source              string `json:"source,omitempty"` // 追加
}
```

**`Source` の値:**
- `""` (空文字列・省略): `syscall` 命令から検出（既存動作を維持）
- `"libc_symbol_import"`: libc インポートシンボル照合によって検出

`omitempty` により、既存の `Source` なしエントリは JSON 出力が変わらない。

**テスト方針:**
- 既存テストはすべて変更なしでパスすること（フィールド追加のみ）
- `Source` フィールドが JSON で `omitempty` 動作することを検証するテストを追加

---

## 3. `internal/runner/security/elfanalyzer/syscall_analyzer.go` 変更仕様

### 3.1 `AnalyzeSyscallsInRange` メソッドの追加

```go
// AnalyzeSyscallsInRange は code[startOffset:endOffset] の範囲に含まれる
// syscall 命令を検出し、各命令の syscall 番号を後方スキャンで抽出して返す。
// sectionBaseAddr は code 全体の仮想アドレス起点。
// startOffset/endOffset は code 先頭からのバイトオフセット。
// Go ラッパー解析（Pass 2）は行わない。
// アーキテクチャ非対応の場合は ErrUnsupportedArchitecture を返す。
func (a *SyscallAnalyzer) AnalyzeSyscallsInRange(
    code []byte,
    sectionBaseAddr uint64,
    startOffset, endOffset int,
    machine elf.Machine,
) ([]common.SyscallInfo, error)
```

**処理手順:**

1. `machine` 引数から `a.archConfigs[machine]` を参照してアーキテクチャ設定を取得する。非対応の場合は `&UnsupportedArchitectureError{Machine: machine}` を返す
2. `findSyscallInstructions(code[startOffset:endOffset], sectionBaseAddr+uint64(startOffset), decoder)` を呼び出して syscall 命令の位置を取得する
   - **設計選択**: `code[startOffset:endOffset]` のスライスを渡し、ベースアドレスを `sectionBaseAddr+uint64(startOffset)` にシフトする。これによりスライスの先頭が仮想アドレス上で `sym.Value` に対応し、スライス内オフセットが 0 相対になる
3. 各 syscall 命令位置について `extractSyscallInfo` を呼び出す（引数 `code` にはスライス `code[startOffset:endOffset]`、`baseAddr` には `sectionBaseAddr+uint64(startOffset)` を使用）
4. **後方スキャン窓のクランプ**: ステップ 2 でスライスを渡すため、後方スキャン窓はスライスの先頭（オフセット 0）より前に出ることはなく、追加のクランプ処理は不要。`backwardScanForSyscallNumber` の既存実装（`windowStart = max(windowStart, 0)`）がスライス先頭のクランプを担う
5. 結果 `[]common.SyscallInfo` を返す（`SyscallInfo.Source` は空文字列のまま）

**`backwardScanForSyscallNumber` のクランプ実装:**

`AnalyzeSyscallsInRange` 専用のヘルパーメソッド `backwardScanForSyscallNumberClamped` を追加する、またはオフセットのクランプを呼び出し前に行う。既存の `backwardScanForSyscallNumber` シグネチャは変更しない。

スライスを渡す設計を採用するため、クランプ処理の概念コードは以下のようになる（スライス内オフセット基準）:

```go
// AnalyzeSyscallsInRange 内の実装（概念コード）
// subCode = code[startOffset:endOffset]、subBase = sectionBaseAddr+uint64(startOffset)
subCode := code[startOffset:endOffset]
subBase := sectionBaseAddr + uint64(startOffset)
syscallLocs, _ := a.findSyscallInstructions(subCode, subBase, decoder)
for _, loc := range syscallLocs {
    // extractSyscallInfo はスライス内オフセットで動作する。
    // backwardScanForSyscallNumber の windowStart は max(windowStart, 0) で
    // クランプされるため、スライス先頭より前のバイトは参照されない。
    info := a.extractSyscallInfo(subCode, loc, subBase, decoder, table)
    results = append(results, info)
}
```

**テスト方針:**
- 正常系: syscall 命令を含む範囲で正しく検出される
- 境界チェック: `startOffset` でクランプが効き、隣接バイトが混入しない
- 非対応アーキテクチャで `ErrUnsupportedArchitecture` が返る

---

## 4. `internal/libccache/schema.go` 仕様

```go
package libccache

// LibcCacheSchemaVersion はキャッシュファイルの現行スキーマバージョン。
// スキーマの後方互換性を破壊する変更時に更新する。
const LibcCacheSchemaVersion = 1

// LibcCacheFile はキャッシュファイルの JSON スキーマ。
type LibcCacheFile struct {
    SchemaVersion   int            `json:"schema_version"`
    LibPath         string         `json:"lib_path"`
    LibHash         string         `json:"lib_hash"`
    AnalyzedAt      string         `json:"analyzed_at"`      // RFC3339UTC 形式
    SyscallWrappers []WrapperEntry `json:"syscall_wrappers"`
}

// WrapperEntry は 1 つのシステムコールラッパー関数を表す。
type WrapperEntry struct {
    Name   string `json:"name"`
    Number int    `json:"number"`
}
```

`SyscallWrappers` は `Number` 昇順でソートして保存する（`sort.Slice` を使用）。

---

## 5. `internal/libccache/errors.go` 仕様

```go
package libccache

import "errors"

var (
    // ErrLibcFileNotAccessible は libc ファイルの読み取り失敗を示す。
    ErrLibcFileNotAccessible = errors.New("libc file not accessible")

    // ErrExportSymbolsFailed は libc エクスポートシンボル取得失敗を示す。
    ErrExportSymbolsFailed = errors.New("failed to get export symbols from libc")

    // ErrCacheWriteFailed はキャッシュファイルの書き込み失敗を示す。
    ErrCacheWriteFailed = errors.New("failed to write libc cache file")
)

// 非対応アーキテクチャのエラーは elfanalyzer.UnsupportedArchitectureError（型エラー）が
// ラップなしで伝播する。呼び出し元は errors.As で検出する:
//
//	var archErr *elfanalyzer.UnsupportedArchitectureError
//	if errors.As(err, &archErr) { ... }

// SourceLibcSymbolImport は SyscallInfo.Source の値。libc インポートシンボル照合由来を示す。
const SourceLibcSymbolImport = "libc_symbol_import"
```

---

## 6. `internal/libccache/analyzer.go` 仕様

### 6.1 定数とシグネチャ

```go
package libccache

import (
    "debug/elf"
    "sort"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// MaxWrapperFunctionSize はラッパー関数として認識する最大サイズ（バイト）。
const MaxWrapperFunctionSize = 256

// LibcWrapperAnalyzer は libc の ELF ファイルを解析し、
// syscall ラッパー関数の一覧を返す。
type LibcWrapperAnalyzer struct {
    syscallAnalyzer *elfanalyzer.SyscallAnalyzer
}

// NewLibcWrapperAnalyzer は LibcWrapperAnalyzer を生成する。
func NewLibcWrapperAnalyzer(syscallAnalyzer *elfanalyzer.SyscallAnalyzer) *LibcWrapperAnalyzer

// Analyze は libcELFFile のエクスポート関数を走査し、
// サイズフィルタと syscall 命令検出を適用して WrapperEntry スライスを返す。
// 返すスライスは Number 昇順でソートされている。
func (a *LibcWrapperAnalyzer) Analyze(libcELFFile *elf.File) ([]WrapperEntry, error)
```

### 6.2 `Analyze` 処理手順

1. `.text` セクションのデータ（`code []byte`）とベースアドレス（`sectionBaseAddr uint64`）を取得する
2. `.dynsym` セクションからエクスポートシンボル（定義済み、関数型、`STT_FUNC`）を列挙する
   - `elf.File.DynamicSymbols()` を呼び出す（`elf.ErrNoSymbols` は空スライスとして扱う）
   - シンボルのバインディングが `STB_LOCAL` のものは除外する（エクスポートシンボルのみ対象）
   - `st_shndx == SHN_UNDEF`（インポートシンボル）は除外する
   - `st_type != STT_FUNC` は除外する
3. 各シンボルについてサイズフィルタを適用する（`sym.Size > MaxWrapperFunctionSize` はスキップ）
4. シンボルのアドレス範囲を `.text` セクション内のオフセットに変換する
   - `startOffset = sym.Value - sectionBaseAddr`
   - `endOffset = startOffset + sym.Size`
   - 範囲が `.text` セクションの範囲外の場合はスキップ
5. `syscallAnalyzer.AnalyzeSyscallsInRange(code, sectionBaseAddr, startOffset, endOffset, libcELFFile.Machine)` を呼び出す
   - `ErrUnsupportedArchitecture` は呼び出し元にそのまま返す（ラップなし）
   - その他のエラーはスキップ（当該関数を無視して続行）
6. 返された `[]SyscallInfo` を検査する
   - 空スライス（syscall 命令なし）の場合はスキップ（当該関数をキャッシュに含めない）
   - すべてのエントリの `DeterminationMethod == "immediate"` かつ `Number >= 0` でなければスキップ
   - すべてのエントリの `Number` が同一でなければスキップ
   - 条件を満たした場合 `WrapperEntry{Name: sym.Name, Number: infos[0].Number}` を収集
7. 収集した `[]WrapperEntry` を `Number` 昇順でソートして返す

### 6.3 テスト仕様

**テスト用 ELF バイナリの構築方法:**

`elfanalyzer` パッケージの既存テスト（`syscall_analyzer_test.go`）と同様に、最小限の ELF バイナリをインメモリ構築してテストする。

| テストケース | 検証内容 | AC |
|-------------|---------|-----|
| syscall 命令を含む関数（≤256B）が検出される | 正常系 | AC-2 |
| 256 バイト超の関数が除外される | サイズフィルタ | AC-2 |
| 複数の異なる syscall 番号を含む関数が除外される | 複数 syscall フィルタ | AC-2 |
| 同一 syscall 番号の syscall 命令を複数持つ関数は採用される | 分岐パスの許容 | AC-2 |
| syscall 命令を含まない関数が除外される | 非ラッパー除外 | AC-2 |
| `WrapperEntry` が `Number` 昇順でソートされている | 決定論的出力 | AC-2 |
| `DeterminationMethod != "immediate"` の関数が除外される | 品質フィルタ | AC-2 |
| 非対応アーキテクチャで `ErrUnsupportedArchitecture` が返る | アーキテクチャ検査 | AC-3 |

---

## 7. `internal/libccache/cache.go` 仕様

### 7.1 型定義

```go
package libccache

import (
    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator/pathencoding"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// LibcCacheManager はライブラリキャッシュの読み書きを管理する。
type LibcCacheManager struct {
    cacheDir string // <hash-dir>/lib-cache/
    fs       safefileio.FileSystem
    analyzer *LibcWrapperAnalyzer
    pathEnc  *pathencoding.SubstitutionHashEscape
}

// NewLibcCacheManager は LibcCacheManager を生成する。
// cacheDir は <hash-dir>/lib-cache/ のパス（存在しない場合は自動作成）。
func NewLibcCacheManager(
    cacheDir string,
    fs safefileio.FileSystem,
    analyzer *LibcWrapperAnalyzer,
) (*LibcCacheManager, error)

// GetOrCreate はキャッシュを返すか、存在しない/無効な場合は解析して生成する。
// libcPath: 正規化済みの実体ファイルパス（DynLibDeps.Libs[].Path）
// libcHash: DynLibDeps.Libs[].Hash（"sha256:<hex>" 形式）
// キャッシュ MISS 時にのみ libcPath を SafeOpenFile で open し、解析後に close する。
// キャッシュ HIT 時はファイルを開かない。
func (m *LibcCacheManager) GetOrCreate(libcPath, libcHash string) ([]WrapperEntry, error)
```

### 7.2 `GetOrCreate` 処理手順

1. キャッシュファイルパスを `cacheDir + "/" + pathEnc.Encode(libcPath)` で生成する
2. キャッシュファイルを読み込む
   - ファイルが存在しない場合 → ステップ 4（MISS）へ
   - JSON パース失敗 → ステップ 4（MISS）へ
   - `cache.SchemaVersion != LibcCacheSchemaVersion` → ステップ 4（MISS）へ
   - `cache.LibHash != libcHash` → ステップ 4（MISS）へ
3. キャッシュ HIT: `cache.SyscallWrappers` を返す
4. キャッシュ MISS:
   - `fs.SafeOpenFile(libcPath, os.O_RDONLY, 0)` で libc を開く（失敗 → `ErrLibcFileNotAccessible` を返す）
   - `elf.NewFile(libcFile)` で ELF としてパースする（失敗 → エラーを返す）
   - `analyzer.Analyze(elfFile)` で解析する（失敗 → エラーを返す、`ErrUnsupportedArchitecture` はそのまま伝播）
   - キャッシュファイルを書き込む（失敗 → `ErrCacheWriteFailed` を返す）
   - `[]WrapperEntry` を返す

**キャッシュファイル書き込み方法:**

セーフライト（アトミック書き込み）は現時点では要求しない。`os.MkdirAll` で `cacheDir` を作成後、`json.Marshal` → `os.WriteFile` で書き込む。

**テスト仕様:**

| テストケース | 検証内容 | AC |
|-------------|---------|-----|
| キャッシュ未存在時に解析・生成される | MISS → 生成 | AC-2, AC-3 |
| ハッシュ一致時にキャッシュが再利用される | HIT | AC-3 |
| ハッシュ不一致時にキャッシュが再生成される | libc 更新時 | AC-3 |
| キャッシュファイルが破損している場合に再解析される | エラー耐性 | AC-3 |
| `schema_version` 不一致時にキャッシュが再生成される | スキーマ変更 | AC-3 |
| `syscall_wrappers` が `number` 昇順でソートされている | 決定論的出力 | AC-2 |
| libc ファイルが読み取れない場合にエラーを返す | fatal ケース | AC-3 |
| キャッシュファイルの書き込みに失敗した場合にエラーを返す | fatal ケース | AC-3 |

---

## 8. `internal/libccache/matcher.go` 仕様

### 8.1 型定義

```go
package libccache

import (
    "github.com/isseis/go-safe-cmd-runner/internal/common"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// SyscallNumberTable はシステムコール番号テーブルのインターフェース。
// elfanalyzer.SyscallNumberTable と同一の仕様。
type SyscallNumberTable interface {
    GetSyscallName(number int) string
    IsNetworkSyscall(number int) bool
}

// ImportSymbolMatcher は対象バイナリのインポートシンボルとキャッシュを照合する。
type ImportSymbolMatcher struct {
    syscallTable SyscallNumberTable
}

// NewImportSymbolMatcher は ImportSymbolMatcher を生成する。
func NewImportSymbolMatcher(syscallTable SyscallNumberTable) *ImportSymbolMatcher

// Match はインポートシンボル一覧とキャッシュを照合し、SyscallInfo を生成する。
// 重複統合キー: Number
// 同じ Number のエントリが複数生成された場合は WrapperEntry.Name の辞書順で
// 最小のシンボル名を持つ 1 件に絞る。
func (m *ImportSymbolMatcher) Match(
    importSymbols []string,
    wrappers []WrapperEntry,
) []common.SyscallInfo
```

### 8.2 `Match` 処理手順

1. `wrappers` を `Name` をキーとするマップに変換する
2. `importSymbols` の各要素についてマップを引く（完全一致）
3. 一致した場合、`common.SyscallInfo` を生成する:
   - `Number`: `WrapperEntry.Number`
   - `Name`: `syscallTable.GetSyscallName(Number)` （テーブルに存在しない場合は `WrapperEntry.Name` を使用）
   - `IsNetwork`: `syscallTable.IsNetworkSyscall(Number)`
   - `Location`: `0`
   - `DeterminationMethod`: `"immediate"`（キャッシュスキーマの不変条件）
   - `Source`: `"libc_symbol_import"`（`SourceLibcSymbolImport` 定数を使用）
4. 結果から `Number` 重複を排除する（同一 `Number` の場合は `WrapperEntry.Name` の辞書順で最小のシンボル名を持つ 1 件を保持）
5. 結果スライスを返す

**テスト仕様:**

| テストケース | 検証内容 | AC |
|-------------|---------|-----|
| シンボルがキャッシュに存在する場合に `SyscallInfo` が生成される | 正常系 | AC-4 |
| シンボルがキャッシュに存在しない場合は無視される | 非ラッパー除外 | AC-4 |
| 生成された `SyscallInfo` の `Source` が `"libc_symbol_import"` | source 確認 | AC-4 |
| 生成された `SyscallInfo` の `Location` が `0` | アドレス未確定 | AC-4 |
| 生成された `SyscallInfo` の `DeterminationMethod` が `"immediate"` | 品質保証 | AC-4 |
| 同一 `Number` のエントリが重複しない | 重複除外 | AC-4 |

---

## 9. `internal/filevalidator/validator.go` 変更仕様

### 9.1 フィールド追加

```go
type Validator struct {
    // ... 既存フィールド ...

    libcCacheMgr   *libccache.LibcCacheManager  // nil if libc cache is disabled
    syscallAnalyzer *elfanalyzer.SyscallAnalyzer // nil if syscall analysis is disabled
}
```

### 9.2 セッタ追加

```go
// SetLibcCacheManager injects the LibcCacheManager used during record operations.
func (v *Validator) SetLibcCacheManager(m *libccache.LibcCacheManager)

// SetSyscallAnalyzer injects the SyscallAnalyzer used during record operations.
func (v *Validator) SetSyscallAnalyzer(a *elfanalyzer.SyscallAnalyzer)
```

### 9.3 `updateAnalysisRecord()` の拡張

`store.Update()` コールバック内に以下のステップを追加する（既存の `dynlibAnalyzer.Analyze()` 呼び出しの後）。

```
[コールバック内の追加処理（既存の dynlibAnalyzer 呼び出しの後）]

// ステップ A: 対象バイナリを ELF としてオープン
elfFile, err := openELFFile(safefileio.NewFileSystem(...), filePath.String())
if errors.Is(err, elfanalyzer.ErrNotELF) {
    // 非 ELF ファイル: syscall 解析全体をスキップして記録を保存
    return nil
}
if err != nil {
    return fmt.Errorf("failed to open ELF file: %w", err)
}
defer elfFile.Close()

var libcSyscalls []common.SyscallInfo

// ステップ B: libc エントリの特定と libc キャッシュ処理
if v.libcCacheMgr != nil && record.DynLibDeps != nil {
    libcEntry := findLibcEntry(record.DynLibDeps)
    if libcEntry != nil {
        // UND シンボルの抽出
        importSymbols := extractUNDSymbols(elfFile)

        // libc キャッシュの取得または生成
        wrappers, err := v.libcCacheMgr.GetOrCreate(libcEntry.Path, libcEntry.Hash)
        if err != nil {
            var archErr *elfanalyzer.UnsupportedArchitectureError
            if errors.As(err, &archErr) {
                // 非対応アーキテクチャ: libc キャッシュ処理をスキップして続行
                goto directSyscallAnalysis
            }
            return fmt.Errorf("libc cache error: %w", err)
        }

        // インポートシンボル照合
        matcher := libccache.NewImportSymbolMatcher(elfanalyzer.NewX86_64SyscallTable())
        libcSyscalls = matcher.Match(importSymbols, wrappers)
    }
}

directSyscallAnalysis:
// ステップ C: 直接 syscall 命令解析
var directSyscalls []common.SyscallInfo
if v.syscallAnalyzer != nil {
    result, err := v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
    var archErr *elfanalyzer.UnsupportedArchitectureError
    if err != nil && !errors.As(err, &archErr) {
        return fmt.Errorf("syscall analysis failed: %w", err)
    }
    if result != nil {
        directSyscalls = result.DetectedSyscalls
    }
}

// ステップ D: マージと SyscallAnalysis の設定
allSyscalls := mergeSyscallInfos(libcSyscalls, directSyscalls)
if len(allSyscalls) > 0 {
    record.SyscallAnalysis = buildSyscallAnalysisData(allSyscalls, directSyscalls)
}
```

**注意**: `goto` は Go では制限があるため、実装では `goto` の代わりにフラグ変数か早期 return を使う。アーキテクチャ非対応の場合は libc キャッシュ処理をスキップし、直接 syscall 解析も `ErrUnsupportedArchitecture` をスキップして空の結果で続行する。

### 9.4 パッケージ非公開ヘルパー関数

```go
// openELFFile は filePath を SafeOpenFile で開き、ELF としてパースして返す。
// ELF でない場合は elfanalyzer.ErrNotELF を返す。
// 呼び出し元は返された *elf.File.Close() の責任を持つ。
func openELFFile(fs safefileio.FileSystem, filePath string) (*elf.File, error)

// extractUNDSymbols は elfFile の .dynsym セクションから
// UND（未定義）シンボル名の一覧を返す。
// .dynsym が存在しない場合は空スライスを返す（エラーなし）。
// .dynsym の読み取りエラーは空スライスを返す（エラーなし）。
func extractUNDSymbols(elfFile *elf.File) []string

// findLibcEntry は dyn_lib_deps から libc エントリを返す。
// SOName が "libc.so." で前方一致する最初のエントリを返す。
// 見つからない場合は nil を返す。
func findLibcEntry(deps *fileanalysis.DynLibDepsData) *fileanalysis.LibEntry

// mergeSyscallInfos は libc 由来と直接 syscall 命令由来の SyscallInfo を統合する。
// 同じ Number を持つエントリは direct（Source == ""）を優先して 1 つに絞る。
func mergeSyscallInfos(libc, direct []common.SyscallInfo) []common.SyscallInfo

// buildSyscallAnalysisData は SyscallInfo スライスから SyscallAnalysisData を構築する。
// HasUnknownSyscalls は direct 由来 (Source == "") の Number < 0 エントリの有無から計算する。
func buildSyscallAnalysisData(all []common.SyscallInfo, direct []common.SyscallInfo) *fileanalysis.SyscallAnalysisData
```

### 9.5 `mergeSyscallInfos` の一意化ルール詳細

```
入力:
  libc = []SyscallInfo{Source="libc_symbol_import", ...}
  direct = []SyscallInfo{Source="", ...}

一意化キー: Number

ルール:
  同じ Number に Source=="" と Source=="libc_symbol_import" が存在 → Source=="" を採用
  同じ Number に Source=="libc_symbol_import" のみ存在 → そのエントリを採用
  同じ Number に Source=="" のみ存在 → そのエントリを採用
```

---

## 10. `cmd/record/main.go` 変更仕様

### 10.1 削除する処理

- `syscallAnalysisContext` 型と関連メソッドを削除する
- `deps.syscallContextFactory` フィールドを削除する
- `processFiles` から `syscallCtx.analyzeFile()` 呼び出しを削除する
- `run()` から `syscallContextFactory` の呼び出しを削除する

### 10.2 追加する処理

`run()` 内で `filevalidator.Validator` に対して以下を設定する:

```go
if fv, ok := validator.(*filevalidator.Validator); ok {
    if d.dynlibAnalyzerFactory != nil {
        fv.SetDynLibAnalyzer(d.dynlibAnalyzerFactory())
    }
    fv.SetBinaryAnalyzer(security.NewBinaryAnalyzer())

    // 追加: libc キャッシュマネージャーと syscall アナライザーの設定
    syscallAnalyzer := elfanalyzer.NewSyscallAnalyzer()
    fv.SetSyscallAnalyzer(syscallAnalyzer)

    libcCacheDir := filepath.Join(cfg.hashDir, "lib-cache")
    libcCacheMgr, err := libccache.NewLibcCacheManager(
        libcCacheDir,
        safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
        libccache.NewLibcWrapperAnalyzer(syscallAnalyzer),
    )
    if err != nil {
        fmt.Fprintf(stderr, "Error: Failed to initialize libc cache: %v\n", err)
        return 1
    }
    fv.SetLibcCacheManager(libcCacheMgr)
}
```

---

## 11. `internal/fileanalysis/schema.go` 変更仕様

`SyscallAnalysis` フィールドのコメントを以下のように更新する:

```go
// SyscallAnalysis contains syscall analysis result (optional).
// Present when at least one syscall was detected (via direct syscall instruction
// or libc symbol import). Nil for non-ELF files and ELF binaries with no
// detected syscalls.
SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
```

---

## 12. 統合テスト仕様

**ファイルパス:** `internal/libccache/integration_test.go`

**ビルドタグ:** `//go:build integration`

**前提条件:** GCC が利用可能、x86_64 アーキテクチャ

```c
// テスト用 C プログラム（mkdir syscall を呼ぶ最小プログラム）
#include <sys/stat.h>
int main() { mkdir("/tmp/test_libccache", 0755); return 0; }
// コンパイル: gcc -o test_mkdir.elf test_mkdir.c （動的リンク、デフォルト）
```

| テストケース | 検証内容 | AC |
|-------------|---------|-----|
| GCC でビルドした動的リンクバイナリを record した際に `mkdir` syscall（番号 83）が `DetectedSyscalls` に含まれる | エンドツーエンド | AC-4 |
| `source: "libc_symbol_import"` の `SyscallInfo` が存在する | source フィールド確認 | AC-4 |
| `Location` が `0` である | アドレス未確定 | AC-4 |
| libc キャッシュファイルが `lib-cache/` 以下に生成される | キャッシュ生成 | AC-2 |
| 2 回目の record 実行でキャッシュが再利用される（libc を再解析しない） | キャッシュ HIT | AC-3 |

---

## 13. 受け入れ条件とテストの対応表

| AC | 対応するテストファイル | テストケース |
|----|----------------------|-------------|
| AC-1 | `internal/common/syscall_types_test.go` | `Source` フィールドの JSON `omitempty` 動作 |
| AC-2 | `internal/libccache/analyzer_test.go` | サイズフィルタ、複数 syscall フィルタ、ソート順 |
| AC-2 | `internal/libccache/cache_test.go` | キャッシュ生成、syscall_wrappers ソート |
| AC-3 | `internal/libccache/cache_test.go` | HIT/MISS/再生成/破損/書き込み失敗 |
| AC-3 | `internal/filevalidator/validator_test.go` | 保存順序、アーキテクチャ非対応 |
| AC-4 | `internal/libccache/matcher_test.go` | 照合、source/location/重複除外 |
| AC-4 | `internal/libccache/integration_test.go` | mkdir syscall エンドツーエンド |
| AC-5 | `make test` 全体 | 既存テストの非回帰 |
