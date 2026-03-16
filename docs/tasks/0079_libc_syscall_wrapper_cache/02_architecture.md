# アーキテクチャ設計書: libc システムコールラッパー関数キャッシュ

## 1. システム概要

### 1.1 アーキテクチャ目標

- 動的リンクバイナリが libc 経由で呼び出すシステムコールを、インポートシンボル照合によって補完検出する
- libc 解析結果をキャッシュし、`record` 実行のたびに再解析するコストを避ける
- 既存の `elfanalyzer` Pass 1 ロジックを再利用し、実装の重複を避ける
- 既存の `SyscallAnalysis` フロー（静的バイナリ）への影響を最小化する

### 1.2 設計原則

- **既存活用**: `elfanalyzer.SyscallAnalyzer` の `findSyscallInstructions` を再利用
- **DRY**: `pathencoding` パッケージの既存エンコーディング方式を再利用
- **セキュリティファースト**: 保存順序（キャッシュ先行）によるデータ整合性の保証
- **YAGNI**: x86_64 のみ対応、libc のみ対象の最小実装

## 2. システム構成

### 2.1 全体アーキテクチャ

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    A[("対象バイナリ")] -->|"ファイルパス"| B["Validator.SaveRecord()<br>(store.Update コールバック内で統合)"]
    B -->|"①dynlibAnalyzer.Analyze()"| K["DynLibAnalyzer<br>(既存)"]
    K -->|"dyn_lib_deps"| B
    B -->|"②libc エントリ特定"| C["LibcCacheManager<br>(新規)"]
    C -->|"キャッシュ HIT"| D[("lib-cache/<br>&lt;encoded-libc&gt;")]
    C -->|"キャッシュ MISS"| E["LibcWrapperAnalyzer<br>(新規)"]
    E -->|"libc 解析"| F["SyscallAnalyzer<br>(既存・再利用)"]
    F -->|"syscall 命令検出"| E
    E -->|"syscall_wrappers"| C
    C -->|"②' キャッシュ書き込み"| D
    C -->|"WrapperEntry 一覧"| B
    B -->|"③ImportSymbolMatcher.Match()"| G["ImportSymbolMatcher<br>(新規)"]
    G -->|"libc .dynsym UND シンボル"| H["対象バイナリ .dynsym"]
    G -->|"SyscallInfo (source=libc_symbol_import)"| B
    B -->|"④SyscallAnalyzer.Analyze()"| F
    F -->|"SyscallInfo (source='')"| B
    B -->|"⑤mergeSyscallInfos()"| I["Number で一意化<br>direct 優先 (新規)"]
    I -->|"deduplicated []SyscallInfo"| B
    B -->|"⑥store.Save() ← コールバック後"| J[("hash-dir/<br>&lt;encoded-binary&gt;")]

    class A,D,H,J data;
    class K,F process;
    class B,C,E,G,I enhanced;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[("永続データ")] --> P1["既存コンポーネント"] --> E1["新規 / 変更コンポーネント"]
    class D1 data
    class P1 process
    class E1 enhanced
```

### 2.2 コンポーネント配置

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph "パッケージ構成"

        subgraph "internal/common"
            A["syscall_types.go<br>SyscallInfo に Source フィールド追加"]
        end

        subgraph "internal/libccache (新規パッケージ)"
            B["analyzer.go<br>LibcWrapperAnalyzer"]
            C["cache.go<br>LibcCacheManager"]
            D["matcher.go<br>ImportSymbolMatcher"]
            E["schema.go<br>LibcCacheFile / WrapperEntry"]
        end

        subgraph "internal/runner/security/elfanalyzer"
            F["syscall_analyzer.go<br>findSyscallInstructions<br>（既存・再利用）"]
        end

        subgraph "internal/fileanalysis"
            G["schema.go<br>SyscallAnalysisData コメント更新"]
        end

        subgraph "internal/filevalidator"
            H["validator.go<br>Validator.updateAnalysisRecord() 拡張<br>（libcCacheMgr / syscallAnalyzer フィールド追加）"]
        end

        subgraph "cmd/record"
            I["main.go<br>processFiles 簡略化<br>（analyzeFile() 呼び出し削除）"]
        end
    end

    A --> D
    B --> F
    C --> B
    C --> E
    H --> C
    H --> D
    H --> G
    I --> H

    class A,F,G process;
    class B,C,D,E,H enhanced;
```

### 2.3 データフロー（record コマンド実行時）

`Validator.SaveRecord()` が呼び出す `store.Update()` のコールバック内で、libc キャッシュ・インポートシンボル照合・syscall 解析をすべて実行する。コールバック return 後に `store.Save()` が記録ファイルを書くため、保存順序（キャッシュ先行）が自然に保証される。

```mermaid
sequenceDiagram
    participant RC as "record コマンド"
    participant V as "Validator.SaveRecord()"
    participant CB as "store.Update() コールバック"
    participant DA as "DynLibAnalyzer (既存)"
    participant CM as "LibcCacheManager"
    participant WA as "LibcWrapperAnalyzer"
    participant SA as "SyscallAnalyzer (既存)"
    participant IM as "ImportSymbolMatcher"
    participant FS as "ファイルシステム"

    RC->>V: "SaveRecord(filePath, force)"
    V->>V: "calculateHash()"
    V->>CB: "store.Update(コールバック)"
    CB->>DA: "Analyze(filePath)"
    DA-->>CB: "dyn_lib_deps (libc の Path/Hash を含む)"
    CB->>FS: "SafeOpenFile(filePath)"
    FS-->>CB: "secure file handle"
    CB->>CB: "elf.NewFile(handle) → *elf.File"
    alt ELF でない（ErrNotELF）
        CB->>CB: "syscall 解析全体スキップ（SyscallAnalysis = nil）"
    else ELF パース成功
        CB->>CB: "findLibcEntry(dyn_lib_deps)"
        alt libc エントリあり（*LibEntry != nil）
            CB->>CB: "extractUNDSymbols(*elf.File) → importSymbols []string"
            CB->>CM: "GetOrCreate(libcPath, libcHash)"
            CM->>FS: "lib-cache/<encoded-libc> 読み込み"
            alt キャッシュ HIT（schema_version 一致 かつ lib_hash 一致）
                FS-->>CM: "LibcCacheFile"
            else キャッシュ MISS、schema_version 不一致、または lib_hash 不一致
                CM->>FS: "SafeOpenFile(libcPath) → libcELFFile"
                CM->>WA: "Analyze(libcELFFile)"
                WA->>SA: "AnalyzeSyscallsInRange(code, sectionBaseAddr, startOffset, endOffset, machine)"
                SA-->>WA: "[]SyscallInfo (Number, DeterminationMethod)"
                WA->>WA: "サイズフィルタ・immediate/単一Number フィルタ"
                WA-->>CM: "[]WrapperEntry"
                CM->>FS: "lib-cache/<encoded-libc> 書き込み  ← 先行"
            end
            CM-->>CB: "[]WrapperEntry"
            CB->>IM: "Match(importSymbols, wrappers)"
            IM-->>CB: "[]SyscallInfo (Source=libc_symbol_import)"
        else libc エントリなし（静的バイナリ等）
            CB->>CB: "libc キャッシュ処理スキップ（libc 由来 SyscallInfo なし）"
        end
        CB->>SA: "AnalyzeSyscallsFromELF(*elf.File)"
    end
    SA-->>CB: "[]SyscallInfo (Source='', 直接 syscall 命令由来)"
    CB->>CB: "mergeSyscallInfos(libcSyscalls, directSyscalls)<br>→ Number で一意化・direct 優先"
    CB->>CB: "record.DynLibDeps, record.SyscallAnalysis を設定"
    CB-->>V: "return nil"
    V->>FS: "store.Save() → hash-dir/<encoded-binary> 書き込み  ← 後行"
    V-->>RC: "hashFilePath, contentHash, nil"
```

### 2.4 キャッシュ有効性判定フロー

```mermaid
flowchart TD
    A["GetOrCreate(libcPath, libcHash) 呼び出し"] --> B{"lib-cache/<encoded-libc> 存在?"}
    B -->|"存在しない"| E["libc 解析実行"]
    B -->|"存在する"| C["JSON パース"]
    C --> D{"パース成功?"}
    D -->|"失敗（破損）"| E
    D -->|"成功"| F{"schema_version 一致?"}
    F -->|"不一致（スキーマ変更）"| E
    F -->|"一致"| F2{"lib_hash 一致?"}
    F2 -->|"不一致（libc 更新）"| E
    F2 -->|"一致"| G["キャッシュ返却 (HIT)"]
    E --> H["WrapperEntry 生成"]
    H --> I["lib-cache/ 書き込み"]
    I --> J{"書き込み成功?"}
    J -->|"失敗"| K["エラー返却<br>→ record エラー終了"]
    J -->|"成功"| L["キャッシュ返却 (新規生成)"]
```

## 3. コンポーネント設計

### 3.1 新規パッケージ: `internal/libccache`

#### 3.1.1 スキーマ定義 (`schema.go`)

```go
// LibcCacheSchemaVersion はキャッシュファイルの現行スキーマバージョン。
// スキーマの後方互換性を破壊する変更（フィールド削除・型変更・意味変更等）を
// 行う際に値を更新する。不一致時はキャッシュを無効とみなして再解析する。
const LibcCacheSchemaVersion = 1

// LibcCacheFile はキャッシュファイルの JSON スキーマ。
type LibcCacheFile struct {
    SchemaVersion   int            `json:"schema_version"`
    LibPath         string         `json:"lib_path"`
    LibHash         string         `json:"lib_hash"`
    AnalyzedAt      string         `json:"analyzed_at"`
    SyscallWrappers []WrapperEntry `json:"syscall_wrappers"`
}

// WrapperEntry は 1 つのシステムコールラッパー関数を表す。
type WrapperEntry struct {
    Name   string `json:"name"`
    Number int    `json:"number"`
}
```

`SyscallWrappers` は `Number` 昇順、同一 `Number` 内では `Name` 昇順の複合キーでソートして保存する（決定論的出力）。

#### 3.1.2 libc エクスポート関数解析 (`analyzer.go`)

```go
// MaxWrapperFunctionSize はラッパー関数として認識する最大サイズ（バイト）。
const MaxWrapperFunctionSize = 256

// LibcWrapperAnalyzer は libc の ELF ファイルを解析し、
// syscall ラッパー関数の一覧を返す。
type LibcWrapperAnalyzer struct {
    analyzer *elfanalyzer.SyscallAnalyzer // AnalyzeSyscallsInRange を呼び出す（§6.2 参照）
}

// Analyze は libcELFFile のエクスポート関数を走査し、
// サイズフィルタと syscall 命令検出を適用して WrapperEntry を返す。
func (a *LibcWrapperAnalyzer) Analyze(libcELFFile *elf.File) ([]WrapperEntry, error)
```

**処理手順:**

1. `.dynsym` セクションからエクスポートシンボル（定義済み、関数型）を列挙する
2. 各シンボルのアドレス・サイズを取得し、`Size > MaxWrapperFunctionSize` のものをスキップする
3. `elfanalyzer.AnalyzeSyscallsInRange(code, sectionBaseAddr, startOffset, endOffset, libcELFFile.Machine)` を呼び出す（後述 §6.2）。この関数が「syscall 命令位置の検出（Pass 1）＋各位置からの後方スキャンによる番号抽出」を一括して行い、`[]SyscallInfo`（`Number`, `DeterminationMethod` を含む）を返す
4. 返された `[]SyscallInfo` から `WrapperEntry.Number` を決定する。採用条件は以下をすべて満たすこと:
   - すべてのエントリの `DeterminationMethod == "immediate"` であること（`unknown:*` や他の方法は拒否）
   - すべてのエントリの `Number` が同一の非負値（`>= 0`）であること
   - いずれかの条件を満たさない場合はその関数をスキップする

   **`immediate` のみを受理する根拠**: `backwardScanForSyscallNumber` の実装において、`Number >= 0` を返す唯一のパスは `DeterminationMethodImmediate` である（`syscall_analyzer.go:449-450`）。現時点では `DeterminationMethod == "immediate"` と `Number >= 0` は等価条件だが、将来の実装変更（新しい決定方法の追加等）によってこの等価性が崩れた際に誤った `WrapperEntry` がキャッシュに混入しないよう、`DeterminationMethod` を明示的にフィルタ条件に含める。
5. 採用した関数を `WrapperEntry` として収集し `Number` 昇順、同一 `Number` 内では `Name` 昇順の複合キーでソートして返す

#### 3.1.3 キャッシュ管理 (`cache.go`)

```go
// LibcCacheManager はライブラリキャッシュの読み書きを管理する。
type LibcCacheManager struct {
    cacheDir string // <hash-dir>/lib-cache/
    fs       safefileio.FileSystem
    analyzer *LibcWrapperAnalyzer
    pathEnc  *pathencoding.SubstitutionHashEscape // 具体型を直接使用（インターフェース不在のため）
}

// GetOrCreate はキャッシュを返すか、存在しない/無効な場合は解析して生成する。
// libcPath: 正規化済みの実体ファイルパス（DynLibDeps.Libs[].Path）
// libcHash: DynLibDeps.Libs[].Hash（"sha256:<hex>" 形式）
// キャッシュ MISS 時にのみ libcPath を SafeOpenFile で open し、解析後に close する。
// キャッシュ HIT 時はファイルを開かない。
func (m *LibcCacheManager) GetOrCreate(libcPath, libcHash string) ([]WrapperEntry, error)
```

キャッシュファイルパス: `<cache-dir>/<pathencoding.Encode(libcPath)>`

`pathencoding` パッケージにはエンコーダーのインターフェース定義が存在しない（`SubstitutionHashEscape` 構造体とそのコンストラクタ `NewSubstitutionHashEscape()` のみ公開）。`LibcCacheManager` はこの具体型を直接保持する。テスト時もインターフェースではなく実装をそのまま使う。

#### 3.1.4 インポートシンボル照合 (`matcher.go`)

```go
// ImportSymbolMatcher は対象バイナリのインポートシンボルとキャッシュを照合する。
type ImportSymbolMatcher struct {
    syscallTable SyscallNumberTable
}

// Match はインポートシンボル一覧とキャッシュを照合し、SyscallInfo を生成する。
// 照合は importSymbols の各要素と WrapperEntry.Name を完全一致で行う。
// バージョンサフィックス（"@@GLIBC_x.y" 等）の除去は不要。
// libc の .dynsym エクスポートもバイナリの .dynsym UND も、
// elf.File が返すシンボル名はバージョン文字列を含まない純粋なシンボル名であるため。
// 重複統合キー: Number。複数の importSymbols（例: "open" と "openat"）が同一の
// syscall 番号にマップされる場合、Name の辞書順で最小のシンボル名を採用して 1 件に絞る。
func (m *ImportSymbolMatcher) Match(
    importSymbols []string, // 対象バイナリの .dynsym UND シンボル名
    wrappers []WrapperEntry,
) []common.SyscallInfo
```

生成される `SyscallInfo`:
- `Source`: `"libc_symbol_import"`
- `Location`: `0`
- `DeterminationMethod`: `"immediate"` — キャッシュには `DeterminationMethod == "immediate"` のエントリしか格納されない（`LibcWrapperAnalyzer.Analyze()` のステップ 4 フィルタによる保証）ため、`WrapperEntry` から復元する際に `"immediate"` を設定することは根拠の捏造ではなく、キャッシュスキーマの不変条件の転写である
- `Name`, `Number`, `IsNetwork`: キャッシュ値と syscall テーブルから設定

### 3.2 `common.SyscallInfo` の拡張

```go
// internal/common/syscall_types.go
type SyscallInfo struct {
    Number              int    `json:"number"`
    Name                string `json:"name,omitempty"`
    IsNetwork           bool   `json:"is_network"`
    Location            uint64 `json:"location"`
    DeterminationMethod string `json:"determination_method"`
    Source              string `json:"source,omitempty"` // 追加: "libc_symbol_import" or "" (syscall 命令由来)
}
```

`omitempty` により既存の `Source` なしエントリは JSON 出力が変わらない。

### 3.3 `Validator` のリファクタリング方針

#### 3.3.1 制約の分析

保存順序の要件（libc キャッシュ先行）と `dyn_lib_deps` の生成タイミングには、以下の循環的な依存が存在する。

```
libc キャッシュ書き込みに必要: dyn_lib_deps（libcのPath/Hash）
dyn_lib_deps の生成タイミング: store.Update() コールバック内
store.Update() コールバック: store.Save()（記録ファイル書き込み）の直前
```

現行の `processFiles` は `SaveRecord()` 成功後に `analyzeFile()` を warning 扱いで呼び出している。この構造では「libc キャッシュを記録ファイルより先に書く」という順序保証を実現できない。

#### 3.3.2 採用するリファクタリング方針

**方針: `store.Update()` コールバック内への libc キャッシュ処理と SyscallAnalysis の統合**

`store.Update()` のコールバック実行と `store.Save()`（記録ファイル書き込み）の間には自然な順序がある。この構造を利用し、コールバック内で以下をすべて実行する。

```
store.Update() コールバック開始
  ↓
dynlibAnalyzer.Analyze() → dyn_lib_deps 取得
  ↓
SafeOpenFile() + elf.NewFile() → *elf.File（失敗 = ErrNotELF → 以降スキップ）
  ↓
findLibcEntry(dyn_lib_deps) → *LibEntry（nil = libc なし）
  ↓ nil でない場合のみ
extractUNDSymbols(*elf.File) → importSymbols
LibcCacheManager.GetOrCreate() → lib-cache/ 書き込み  ← キャッシュ先行
ImportSymbolMatcher.Match(importSymbols, wrappers) → libc 由来 SyscallInfo
  ↓ （nil の場合はここまでスキップ）
SyscallAnalyzer.AnalyzeSyscallsFromELF(*elf.File) → 直接 syscall 検出
  ↓
mergeSyscallInfos() → Number で一意化・direct 優先
  ↓
record.DynLibDeps, record.SyscallAnalysis を設定
  ↓
コールバック return nil
  ↓
store.Save() → 記録ファイル書き込み  ← 記録ファイルは必ずキャッシュの後
```

コールバック内でキャッシュ書き込みが先行し、コールバック return 後に `store.Save()` が記録ファイルを書くため、保存順序が自然に保証される。コールバックがエラーを返した場合、`store.Save()` は呼ばれないため記録ファイルは保存されない。

**現行の `analyzeFile()` との差分（仕様変更）**

現行の `cmd/record/main.go` は `SaveRecord()` 成功後に `analyzeFile()` を独立呼び出しし、`ErrNotELF`・`os.ErrNotExist` 以外の失敗（ELF パース失敗、アーキテクチャ非対応等）を **warning のみ** で処理している（`main.go:188-191`）。この場合、記録ファイルはすでに保存された後であり、syscall 解析なしの記録が永続化される。

本設計ではこれを **意図的に変更する**。syscall 解析を `store.Update()` コールバック内に移動することで、失敗時は記録ファイルが保存されない（コールバックがエラーを返し、`store.Save()` が呼ばれない）。これは `dynlibAnalyzer` の既存の扱い（`validator.go:184`）と同じレベルに syscall 解析を引き上げることを意味する。

**根拠**: 目的はリスク評価であり、syscall 解析に失敗したまま記録ファイルを保存すると `SyscallAnalysis` が空のまま永続化され、検証時に過小評価を引き起こす。解析失敗は「記録不能」として扱い、利用者に明示的なエラーを返す方が安全である。

**許容する失敗条件（スキップして処理続行）:**

| 条件 | エラー | 扱い |
|------|--------|------|
| 対象バイナリが ELF でない（スクリプト等） | `ErrNotELF` | syscall 解析スキップ、`SyscallAnalysis = nil` で記録保存 |
| libc が動的依存に存在しない（静的バイナリ等） | nil（libc エントリなし） | libc キャッシュ処理スキップ、`SyscallAnalysis` に direct 分のみ設定 |
| アーキテクチャ非対応（x86_64 以外） | `*elfanalyzer.UnsupportedArchitectureError`（`errors.As` で検出） | libc キャッシュ処理スキップ、同上 |

**fatal にする条件（コールバックエラー → 記録ファイル未保存）:**

| 条件 | エラー |
|------|--------|
| `dynlibAnalyzer` 失敗 | `dynamic library analysis failed: ...` |
| libc ファイル読み取り失敗 | `ErrLibcFileNotAccessible` |
| libc エクスポートシンボル取得失敗 | `ErrExportSymbolsFailed` |
| libc キャッシュ書き込み失敗 | `ErrCacheWriteFailed` |
| ELF パース失敗（バイナリが破損等） | ELF ライブラリエラー |

#### 3.3.3 `Validator` への変更範囲

`dynlibAnalyzer.Analyze()` は現行でも `store.Update()` コールバック内で呼ばれているため、libc キャッシュ処理と `SyscallAnalysis` を同コールバック内に加えるのは自然な拡張である。

変更箇所:

- `filevalidator.Validator` に `libcCacheMgr` と `syscallAnalyzer` フィールドを追加する
- `updateAnalysisRecord()` のコールバック内に libc キャッシュ処理・インポートシンボル照合・syscall 解析・`mergeSyscallInfos()` による一意化を追加する
- `cmd/record/main.go` の `processFiles` から独立した `analyzeFile()` 呼び出しを削除する（コールバック内への統合のため）
- `syscallAnalysisContext` は `Validator` に吸収され廃止する

#### 3.3.4 libc 特定ロジック

```go
// findLibcEntry は dyn_lib_deps から libc エントリを返す。
// SOName が "libc.so." で前方一致する最初のエントリを返す。見つからない場合は nil を返す。
func findLibcEntry(deps *fileanalysis.DynLibDepsData) *fileanalysis.LibEntry {
    for i, lib := range deps.Libs {
        if strings.HasPrefix(lib.SOName, "libc.so.") {
            return &deps.Libs[i]
        }
    }
    return nil
}
```

#### 3.3.5 保存順序の保証（コールバック統合後）

```
[store.Update() コールバック内]
  1. dynlibAnalyzer.Analyze() → dyn_lib_deps 取得
  2. SafeOpenFile(filePath) → secure file handle 取得（ELF/非ELF 共通）
  3. elf.NewFile(handle) → *elf.File（失敗 = ErrNotELF → ステップ 4〜7 をスキップ）
  4. findLibcEntry(dyn_lib_deps) → *LibEntry（nil = libc なし → ステップ 5・6 をスキップ）
  5. extractUNDSymbols(*elf.File) → importSymbols []string
  6. LibcCacheManager.GetOrCreate(libcPath, libcHash) → lib-cache/ 書き込み（キャッシュ先行、MISS 時のみ libc open）
  7. ImportSymbolMatcher.Match(importSymbols, wrappers) → libc 由来 SyscallInfo 収集
  8. SyscallAnalyzer.AnalyzeSyscallsFromELF(*elf.File) → 直接 syscall 検出
  9. mergeSyscallInfos(libcSyscalls, directSyscalls) → Number で一意化・direct 優先
  10. record.DynLibDeps = dynLibDeps
  11. record.SyscallAnalysis = syscallData
  12. return nil
[store.Update() コールバック後]
  13. store.Save() → 記録ファイル（hash-dir/）書き込み
```

ステップ 2 の `SafeOpenFile` 失敗・ステップ 6 のキャッシュ処理失敗はコールバックエラーとなり `store.Save()` は呼ばれない。ステップ 3 で `ErrNotELF` が返った場合は syscall 解析全体をスキップし（`SyscallAnalysis = nil`）ステップ 10 以降を継続する。その他のエラーはすべてコールバックからエラーを返して終了する。

### 3.3.6 対象バイナリの ELF オープンと UND シンボル抽出

#### ELF ファイルオープンの責務

`Validator` コールバック内で対象バイナリを 1 回だけ secure open し、得られた `*elf.File` を以降のすべての解析ステップで共有する。これにより TOCTOU を防ぎつつ、ファイルオープンの重複を避ける。

```go
// openELFFile は filePath を SafeOpenFile で開き、ELF としてパースして返す。
// ELF でない場合は elfanalyzer.ErrNotELF を返す。
// 呼び出し元は *elf.File.Close() の責任を持つ。
func openELFFile(fs safefileio.FileSystem, filePath string) (*elf.File, error)
```

この関数は `filevalidator` パッケージ内のパッケージ非公開ヘルパーとして実装する。

#### UND シンボル抽出の責務

```go
// extractUNDSymbols は elfFile の .dynsym セクションから
// UND（未定義）シンボル名の一覧を返す。
// .dynsym が存在しない（静的バイナリ等）場合は空スライスとエラーなしを返す。
// .dynsym の読み取りエラー（ELF 破損等）はエラーを返す。
func extractUNDSymbols(elfFile *elf.File) ([]string, error)
```

この関数も `filevalidator` パッケージ内のパッケージ非公開ヘルパーとして実装する。`elf.ErrNoSymbols` は「シンボルなし」として空スライスを返す（エラーなし）。それ以外の読み取りエラーはエラーとして返し、呼び出し元のコールバックがエラーとして処理する。

#### `*elf.File` の共有範囲

| 処理 | 同一 `*elf.File` を使用するか |
|------|------------------------------|
| `extractUNDSymbols()` | ○（UND シンボル抽出） |
| `SyscallAnalyzer.AnalyzeSyscallsFromELF()` | ○（直接 syscall 検出） |
| `LibcCacheManager.GetOrCreate()` | ✕（libc は GetOrCreate が内部で safe open/close） |

libc は対象バイナリとは別ファイルであるため、`LibcCacheManager.GetOrCreate()` がキャッシュ MISS 確定後に libc を SafeOpenFile で open し、解析完了後に close する責務を持つ。キャッシュ HIT 時はファイルを開かない。

### 3.4 `fileanalysis/schema.go` のコメント更新

```go
// SyscallAnalysis contains syscall analysis result (optional).
// Present when at least one syscall was detected (via direct syscall instruction
// or libc symbol import). Nil for non-ELF files and ELF binaries with no
// detected syscalls.
SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
```

### 3.5 `mergeSyscallInfos` による一意化仕様

#### 3.5.1 関数シグネチャ

```go
// mergeSyscallInfos は libc 由来と直接 syscall 命令由来の SyscallInfo を統合する。
// 同じ Number を持つエントリは direct syscall（Source == ""）を優先して 1 つに絞る。
// 引数の順序は優先度に影響しない（Source 値で判定する）。
func mergeSyscallInfos(libc, direct []common.SyscallInfo) []common.SyscallInfo
```

#### 3.5.2 一意化ルール

| 条件 | 採用するエントリ |
|------|----------------|
| 同じ `Number` に `Source == ""` と `Source == "libc_symbol_import"` が存在 | `Source == ""`（direct）を採用 |
| 同じ `Number` に `Source == "libc_symbol_import"` のみ存在 | そのエントリを採用 |
| 同じ `Number` に `Source == ""` のみ存在 | そのエントリを採用 |

**`ImportSymbolMatcher` 内部重複との関係**: `Match()` の重複統合キー `Number` は、libc 由来エントリ内の重複（同一 Number を複数シンボルが担う場合、例: `open` と `openat` が共に syscall 257 にマップされる場合）を除去し、`Name` の辞書順で最小のシンボル名を採用して 1 件に絞る。`mergeSyscallInfos` は異なる `Source` 間の重複を除去する。両者は異なるスコープで独立して動作する。

#### 3.5.3 `SyscallAnalysisData` の再計算

一意化後の `[]SyscallInfo` を `DetectedSyscalls` に設定し、`Summary`・`HasUnknownSyscalls`・`HighRiskReasons` をその内容から再計算する。既存の `syscall_analyzer.go` の計算ロジック（`DetectedSyscalls` と `HasUnknownSyscalls` から `Summary` を導出する箇所）を再利用する。

```
mergeSyscallInfos(libcSyscalls, directSyscalls)
  → deduplicated []SyscallInfo
  → SyscallAnalysisData{
        DetectedSyscalls:  deduplicated,
        HasUnknownSyscalls: <directSyscalls 中の Number < 0 エントリの有無>,
        Summary:            <deduplicated から再計算>,
        HighRiskReasons:    <deduplicated から再計算>,
    }
```

`HasUnknownSyscalls` は `Source == ""` の直接 syscall のうち `Number < 0` のものを対象とする。`libc_symbol_import` 由来はキャッシュフィルタ（§3.1.2 ステップ 4）により `Number < 0` を含まないため、対象外とする。

## 4. ファイルシステム構造

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    A[("&lt;hash-dir&gt;/")]
    A --> B["&lt;encoded-mkdir&gt;<br>（/usr/bin/mkdir の記録ファイル）"]
    A --> C["&lt;encoded-libc.so.6&gt;<br>（libc.so.6 の記録ファイル、<br>record に libc を渡した場合）"]
    A --> D["lib-cache/"]
    D --> E["&lt;encoded-libc.so.6&gt;<br>（libc.so.6 のキャッシュファイル）"]

    class A,B,C,D,E data;
```

キャッシュファイルのファイル名エンコードには `internal/filevalidator/pathencoding` の既存実装を使用する。

## 5. エラーハンドリング設計

### 5.1 エラー分類

```mermaid
flowchart TD
    A["libc キャッシュ処理中のエラー"] --> B{"エラー種別"}
    B -->|"キャッシュ破損（JSON パース失敗）"| C["再解析を試みる"]
    C --> D{"再解析成功?"}
    D -->|"成功"| E["処理続行"]
    D -->|"失敗"| F["エラーで終了<br>記録ファイル未保存"]
    B -->|"libc ファイル読み取り失敗"| F
    B -->|"エクスポートシンボル取得失敗"| F
    B -->|"キャッシュ書き込み失敗"| F
    B -->|"非対応アーキテクチャ（x86_64 以外）"| G["libc 解析スキップ<br>libc 由来エントリなしで継続<br>（エラーなし）"]
```

### 5.2 エラー型

`internal/libccache/errors.go` に以下を定義する:

| エラー変数 | 説明 |
|-----------|------|
| `ErrLibcFileNotAccessible` | libc ファイルの読み取り失敗 |
| `ErrExportSymbolsFailed` | エクスポートシンボル取得失敗 |
| `ErrCacheWriteFailed` | キャッシュファイルの書き込み失敗 |

非対応アーキテクチャのエラーは `elfanalyzer.UnsupportedArchitectureError`（型エラー）が `libccache` パッケージを通じてラップなしで伝播する。`libccache` に独自のセンチネル変数は定義しない。

`UnsupportedArchitectureError` は `AnalyzeSyscallsInRange` で発生し、`LibcWrapperAnalyzer.Analyze()` → `LibcCacheManager.GetOrCreate()` とラップなしで伝播する。Validator コールバックが `errors.As(err, new(*elfanalyzer.UnsupportedArchitectureError))` で検知してスキップする（唯一の継続パス）。詳細な伝播経路は §6.2 参照。

## 6. 依存関係

### 6.1 内部パッケージ依存関係

```mermaid
graph LR
    classDef new fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef existing fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    A["cmd/record"]
    B["internal/libccache"]
    C["internal/runner/security/elfanalyzer"]
    D["internal/fileanalysis"]
    E["internal/common"]
    F["internal/filevalidator/pathencoding"]
    G["internal/safefileio"]
    H["internal/filevalidator"]

    A --> H
    A --> D
    H --> B
    H --> C
    B --> C
    B --> E
    B --> F
    B --> G
    C --> E
    D --> E

    class B new;
    class A,C,D,E,F,G,H existing;
```

### 6.2 `elfanalyzer` からの再利用

`LibcWrapperAnalyzer` が `WrapperEntry.Number` を埋めるには、「syscall 命令の位置検出（`findSyscallInstructions`）」だけでなく、「各位置からの後方スキャンによる番号抽出（`extractSyscallInfo` → `backwardScanForSyscallNumber`）」まで必要である。現行の `AnalyzeSyscallsFromELF` はファイル全体の `.text` セクションを対象とするため、関数単位の部分範囲への適用には向かない。

このため、任意のバイト範囲に対して「位置検出 + 番号抽出」を一括実行する新しいエクスポート関数を `elfanalyzer` パッケージに追加する。

**追加するエクスポート API:**

```go
// AnalyzeSyscallsInRange は code[startOffset:endOffset] の範囲に含まれる
// syscall 命令を検出し、各命令の syscall 番号を後方スキャンで抽出して返す。
// sectionBaseAddr は code 全体の仮想アドレス起点。
// startOffset/endOffset は code 先頭からのバイトオフセット。
// Go ラッパー解析（Pass 2）は行わない。
// アーキテクチャ非対応の場合は *UnsupportedArchitectureError を返す（errors.As で検出）。
func (a *SyscallAnalyzer) AnalyzeSyscallsInRange(
    code []byte,
    sectionBaseAddr uint64,
    startOffset, endOffset int,
    machine elf.Machine,
) ([]common.SyscallInfo, error)
```

`libccache.LibcWrapperAnalyzer` はこの関数を呼び出して `[]SyscallInfo` を取得し、`Number` の一意性を検査して `WrapperEntry` を生成する。

**後方スキャン窓の下限クランプ（境界チェック要件）:**

`backwardScanForSyscallNumber` は syscall 命令位置から最大 `maxBackwardScan * maxInstructionLength`（= 50 × 15 = 750）バイト後方まで `decodeInstructionsInWindow` でデコードする。`.text` セクション全体を対象とする `AnalyzeSyscallsFromELF` ではこの窓が隣接する別関数のバイトを取り込むことはないが、`AnalyzeSyscallsInRange` では `startOffset` より前に隣接する別の libc 関数が存在する。

窓の下限が `startOffset` を下回ると、隣接関数のバイトが後方スキャンの候補命令に混入し、無関係な命令を「syscall 番号設定命令」と誤認識する可能性がある（パニックは起きないが正確性の問題）。

このため `AnalyzeSyscallsInRange` の実装では、`backwardScanForSyscallNumber` へ渡す `windowStart` を `max(windowStart, startOffset)` でクランプする。これにより後方スキャンは常に `startOffset` 以降に限定され、他の関数のバイトが混入しない。

```
// windowStart のクランプ（AnalyzeSyscallsInRange 内での処理）
windowStart := syscallOffset - (maxBackwardScan * maxWindowBytesPerInstruction)
if windowStart < startOffset {
    windowStart = startOffset  // ← 部分範囲の先頭より手前に出ない
}
```

クランプにより後方スキャン可能なバイト数が減少する場合（関数が 750 バイト未満で syscall 命令が先頭付近にある場合）、`Number` が解決できずに `DeterminationMethodUnknownScanLimitExceeded` や `DeterminationMethodUnknownDecodeFailed` が返ることがある。`LibcWrapperAnalyzer.Analyze()` はこれらを `DeterminationMethod != "immediate"` として §3.1.2 ステップ 4 のフィルタで除外するため、キャッシュへの混入は防がれる。

`*UnsupportedArchitectureError` の伝播経路（各層でラップせずそのまま返す。最終的に Validator コールバックが `errors.As` で検出する）:

```
AnalyzeSyscallsInRange()  →  LibcWrapperAnalyzer.Analyze()  →  LibcCacheManager.GetOrCreate()  →  呼び出し元（Validator コールバック）
```

`LibcWrapperAnalyzer.Analyze()` は `AnalyzeSyscallsInRange` から受け取った `*UnsupportedArchitectureError` をラップせずそのまま返す。`LibcCacheManager.GetOrCreate()` も同様にそのまま返す。呼び出し元の Validator コールバックが `errors.As(err, new(*elfanalyzer.UnsupportedArchitectureError))` で検知し、libc キャッシュ処理をスキップして処理を継続する（§5.2 参照）。

再利用方法の選択肢:

| 案 | 公開する API | メリット | デメリット |
|----|------------|---------|-----------|
| A | `FindSyscallInstructions` のみ（位置だけ） | 変更が小さい | `Number` を求める処理が未定義のまま残る |
| **B** | `AnalyzeSyscallsInRange`（位置検出 + 番号抽出） | `libccache` 側に番号抽出ロジックの重複が生じない | `elfanalyzer` の API 拡張が必要 |
| C | `LibcWrapperAnalyzer` を `elfanalyzer` パッケージ内に配置 | 内部関数に直接アクセス可能 | パッケージが肥大化 |

**採用: 案 B** — `AnalyzeSyscallsInRange` を `SyscallAnalyzer` のメソッドとして追加し、`libccache.LibcWrapperAnalyzer` から呼び出す。内部では既存の `findSyscallInstructions` + `extractSyscallInfo` を呼ぶだけであり、新規ロジックの追加は不要。

## 7. テスト戦略

### 7.1 テスト階層

```mermaid
flowchart TB
    classDef tier1 fill:#ffb86b,stroke:#333,color:#000;
    classDef tier2 fill:#ffd59a,stroke:#333,color:#000;
    classDef tier3 fill:#c3f08a,stroke:#333,color:#000;

    Tier1["統合テスト（integration ビルドタグ）<br>GCC でビルドした動的リンクバイナリを record<br>→ mkdir syscall が DetectedSyscalls に存在することを確認"]:::tier1
    Tier2["コンポーネントテスト<br>LibcCacheManager のキャッシュ HIT/MISS<br>ImportSymbolMatcher の照合ロジック"]:::tier2
    Tier3["ユニットテスト<br>LibcWrapperAnalyzer のサイズフィルタ<br>複数syscall番号フィルタ<br>WrapperEntry ソート順"]:::tier3

    Tier3 --> Tier2 --> Tier1
```

### 7.2 テストデータ

`LibcWrapperAnalyzer` のユニットテストは、`elfanalyzer/testing` の既存パターンに倣い、最小限の ELF バイナリをインメモリ構築する。統合テストは GCC でコンパイルした動的リンクバイナリを使用し、GCC 非利用環境では `t.Skip()` でスキップする。

## 8. 非機能設計

### 8.1 パフォーマンス

- libc 解析はキャッシュ MISS 時のみ実行（初回または libc 更新時）
- 通常の `record` 実行: キャッシュファイル読み込み（< 1ms）+ ハッシュ照合 + JSON パースのみ
- サイズフィルタ（256 バイト超除外）により解析対象を約 2/3 に削減

### 8.2 スキーマ互換性

- `SyscallInfo.Source` は `omitempty` のため、既存の記録ファイルは正常に読み込める
- `CurrentSchemaVersion`（記録ファイル側）の変更は不要（`01_requirements.md` §4.3 参照）
- `LibcCacheFile.SchemaVersion` はキャッシュファイル専用のバージョン管理フィールド。`LibcCacheFile` のフィールド削除・型変更・意味変更など後方互換性を破壊する変更を行う際に `LibcCacheSchemaVersion` を更新する。`GetOrCreate()` は `schema_version` が `LibcCacheSchemaVersion` と一致しない場合、キャッシュを無効とみなして再解析する。新規フィールドの追加のみで既存フィールドの意味が変わらない場合はバージョンを上げなくてよい。

### 8.3 保守性

- `MaxWrapperFunctionSize = 256` を名前付き定数として定義（変更容易）
- `source` 値 `"libc_symbol_import"` を定数として定義
- ARM64 拡張時は `LibcWrapperAnalyzer` にアーキテクチャ別のデコーダーを追加するだけで対応可能
