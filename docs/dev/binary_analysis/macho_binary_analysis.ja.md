# Mach-O バイナリ解析の技術詳細

本ドキュメントは、macOS 向け Mach-O バイナリ解析の技術的な動作仕様と設計判断を記録する。

ELF バイナリ解析は `internal/security/elfanalyzer/` が担うが、
Mach-O バイナリ解析は以下の 3 つのパッケージに分散して実装されている。

| パッケージ | 役割 |
|---|---|
| `internal/security/machoanalyzer/` | ネットワークシンボル検出 + arm64 限定 2-pass syscall 解析 |
| `internal/dynlib/machodylib/` | `LC_LOAD_DYLIB` 再帰解析（依存ライブラリのハッシュ記録） |
| `internal/libccache/` Mach-O 側 | `libsystem_kernel.dylib` の syscall ラッパーシンボルキャッシュ |

## 1. ネットワークシンボル検出

### 1.1 目的と対応アーキテクチャ

`StandardMachOAnalyzer` は `binaryanalyzer.BinaryAnalyzer` インタフェースを実装し、
Mach-O バイナリが socket / connect / bind 等のネットワーク関連シンボルをインポートしているかを判定する。
単一アーキテクチャ Mach-O およびFat バイナリの両方に対応し、シンボル検出は全アーキテクチャで機能する。

### 1.2 シンボル検出の仕組み

macOS の Mach-O バイナリは原則として 2 レベル名前空間（two-level namespace）を使用するため、
各シンボルの `Desc` フィールドにライブラリオーディナル（bits[15:8]）が格納されている。

**通常パス（Symtab あり）**:

1. `Dysymtab` で未定義シンボル（undefined external）の範囲を取得する。
2. 各シンボルの `Desc` フィールドからライブラリオーディナルを抽出し、
   `LC_LOAD_DYLIB` 一覧と照合して `libSystem` 由来のシンボルかを判定する。
3. `libSystem` 由来のシンボルを `binaryanalyzer.GetNetworkSymbols()` と照合してネットワーク分類を付与する。

```
libSystem 由来の判定条件:
  1. ordinal が範囲内で、対応ライブラリが libSystem.B.dylib または libsystem_*.dylib
  2. または フラット名前空間の場合（全シンボルの ordinal が 0）かつ
     インポートライブラリに libSystem が含まれる
```

**フォールバックパス（Symtab なし）**:

`Symtab` が存在しない stripped バイナリの場合、`ImportedLibraries()` に `libSystem`
が含まれれば `ImportedSymbols()` で全インポートシンボルを取得し検出する。

### 1.3 Fat バイナリの扱い

Fat バイナリは全スライス（全アーキテクチャ）を解析し、最も深刻なリスクを報告する。
これにより、arm64 スライスは安全でも x86_64 スライスにネットワーク機能がある場合の
迂回攻撃（cross-architecture security bypass）を防止する。

```
重大度の優先順位: NetworkDetected > AnalysisError > NoNetworkSymbols

いずれかのスライスが NetworkDetected → 全体を NetworkDetected として報告
```

## 2. syscall 静的解析（arm64 限定）

### 2.1 macOS の syscall 規約

macOS の arm64 バイナリは BSD syscall 規約を使用する。
ELF arm64 の Linux ABI と以下の点が異なる。

| 項目 | macOS (Mach-O) | Linux (ELF) |
|---|---|---|
| syscall 命令 | `SVC #0x80` (0xD4001001) | `SVC #0` (0xD4000001) |
| syscall 番号レジスタ | **X16** | W8/X8 |
| Go ラッパー引数渡し | **スタック** (`[SP, #8]`) | レジスタ (X0) |

### 2.2 解析フロー概要

```
ScanSyscallInfos (svc_scanner.go)
  ↓
  Fat binary の場合: 全 arm64 スライスに対して analyzeArm64Slice を実行
  単一 Mach-O の場合: analyzeArm64Slice を実行
  ↓
  analyzeArm64Slice
    ├── ParseMachoPclntab → Go 関数アドレス範囲を取得
    ├── buildStubRanges  → 既知ラッパー関数の範囲を構築
    ├── Pass 1: collectSVCAddresses + scanSVCWithX16
    └── Pass 2: buildWrapperAddrs + scanGoWrapperCalls
```

### 2.3 pclntab の解析

`ParseMachoPclntab`（`pclntab_macho.go`）は Mach-O 内の `.gopclntab` に相当するセクションを
解析し、Go 関数名とエントリ/終了アドレスのマップ（`map[string]MachoPclntabFunc`）を返す。

このマップから `knownMachoSyscallImpls`（`syscall.Syscall`、`syscall.RawSyscall` 等）の
アドレス範囲を `buildStubRanges` で構築する。ラッパー関数内部の `SVC #0x80` は
Pass 1 / Pass 2 の両方でスキップされる。

### 2.4 Pass 1: SVC #0x80 スキャンと X16 逆方向スキャン

`collectSVCAddresses` は `__TEXT,__text` セクションを 4 バイト単位で走査し、
`SVC #0x80`（バイト列 `0x01 0x10 0x00 0xD4`）の仮想アドレスを列挙する。

`scanSVCWithX16` は各アドレスに対して以下の処理を行う。

1. アドレスが既知ラッパー関数内部であればスキップ（Pass 2 が担当する）。
2. `arm64util.BackwardScanX16` で `SVC #0x80` の直前命令列を後向きスキャンし、
   X16 への即値ロードを探す。
3. 即値が見つかれば `DeterminationMethod = "immediate"` で記録する。
4. 見つからなければ `DeterminationMethod = "direct_svc_0x80"` で記録する
   （番号不明だが直接 syscall を確認したことを示す High Risk シグナル）。

```asm
; Pass 1 が検出する典型的なパターン（非 Go バイナリ）
MOV  X16, #4            ; syscall 番号 4 (write) を X16 にロード
SVC  #0x80              ; BSD syscall を呼び出す
```

### 2.5 Pass 2: Go ラッパー呼び出しとスタック ABI

Go の `syscall.Syscall` / `syscall.RawSyscall` 等は NOSPLIT アセンブリスタブであり、
呼び出し規約として**スタックベース ABI** を使用する。呼び出し元は trap 番号を
`SP+8`（`trap+0(FP)`）に格納してから BL 命令でラッパーを呼び出す。

`scanGoWrapperCalls` は以下の手順で処理する。

1. `__TEXT,__text` を 4 バイト単位で走査し、BL 命令のターゲットアドレスを計算する。
2. ターゲットが既知ラッパーアドレスに一致するかを確認する。
   1 段階のトランポリン（`B <wrapper>`）も解決する。
3. BL 命令自体がラッパー関数内部であればスキップする（内部呼び出しは対象外）。
4. `arm64util.BackwardScanStackTrap` で BL 直前の命令列を後向きスキャンし、
   `[SP, #8]` への書き込み（STR/STP）を探してその値の X レジスタへの
   即値ロードを解決する。

```go
// Go 呼び出し側が生成する典型的なパターン
// trap 番号を SP+8 に格納してから syscall.Syscall を呼び出す
MOV  X0, #4             // trap 番号（write）
STR  X0, [SP, #8]       // trap+0(FP) に格納
...
BL   _syscall.Syscall   // Go ラッパーを呼び出す
```

**ELF arm64 との相違**: ELF arm64 の Go ラッパー（`syscall.Syscall` 等）は
レジスタベース ABI を使用し、第 1 引数 (syscall 番号) は X0 レジスタで渡す。
Mach-O では NOSPLIT スタブのため `[SP, #8]` を使用する。

## 3. 動的ライブラリ依存解析

### 3.1 BFS による再帰解析

`MachODynLibAnalyzer.Analyze`（`machodylib/analyzer.go`）は
`LC_LOAD_DYLIB` および `LC_LOAD_WEAK_DYLIB` の依存関係を幅優先探索（BFS）で再帰的に解決する。

```
Analyze(binaryPath)
  ↓ キュー初期化: バイナリの直接依存を enqueue
  ↓ BFS ループ（最大深さ: MaxRecursionDepth = 20）
    ├── install name → 実パス変換
    │     @rpath         → LC_RPATH エントリを参照
    │     @loader_path   → 当該 Mach-O のディレクトリ
    │     @executable_path → バイナリのディレクトリ
    ├── IsDyldSharedCacheLib → dyld 共有キャッシュなら除外（後述）
    ├── SHA256 ハッシュ計算
    ├── LibEntry（SOName, Path, Hash）を追記
    └── 解決済みライブラリの依存をさらに enqueue
```

### 3.2 dyld 共有キャッシュの除外

macOS の多くのシステムライブラリ（`libSystem.B.dylib` など）は
個別ファイルとしてディスクに存在せず、**dyld 共有キャッシュ**として一体化されている。
このため、ハッシュ検証の対象から除外する必要がある。

`IsDyldSharedCacheLib`（`dyld_extractor_darwin.go`）は以下のパスパターンで
キャッシュライブラリを識別する。

- `/usr/lib/` 配下のライブラリ
- `/System/Library/Frameworks/` 配下のフレームワーク
- その他 Apple が dyld 共有キャッシュに含めるパス

除外されたライブラリは `LibEntry` に含まれず、runner 実行時のハッシュ検証対象にもならない。

### 3.3 runner 実行時の検証

記録済みの `DynLibDeps` が存在する場合、runner の `VerifyCommandDynLibDeps` は
各ライブラリのハッシュを実際のファイルと照合する。
動的リンクバイナリで `DynLibDeps` が未記録の場合は再 record を要求する
（`ErrDynLibDepsRequired`）。

dyld 共有キャッシュのライブラリは記録されていないため、その変更は検証対象外となる。
これは macOS のシステムアップデートでキャッシュが入れ替わっても
runner が起動できるようにするための設計上の意図的な除外である。

## 4. libsystem_kernel.dylib ラッパーキャッシュ

### 4.1 役割

macOS の `syscall.Syscall` 等の Go ラッパーは最終的に `libsystem_kernel.dylib`
内の `_syscallname` 関数（例: `_write`、`_socket`）を BL で呼び出す。
これらの関数は `SVC #0x80` を含む syscall ラッパーである。

`MachoLibSystemAnalyzer`（`libccache/macho_analyzer.go`）は
`libsystem_kernel.dylib` の `__TEXT,__text` セクションをスキャンし、
`SVC #0x80` を含む関数を `WrapperEntry{Name, Number}` として列挙する。

### 4.2 キャッシュの仕組み

`MachoLibSystemCacheManager`（`libccache/macho_cache.go`）は
解析結果を record コマンドのハッシュディレクトリ配下にキャッシュする。
キャッシュのキーはライブラリパスと SHA256 ハッシュであり、
ライブラリが更新されるとキャッシュミスが発生して再解析を行う。

これにより `libsystem_kernel.dylib` の解析コスト（全関数の SVC スキャン）を
最初の record 時のみに抑えられる。

### 4.3 libccache との関係

ELF 側の `LibcCacheManager`（Linux glibc 向け）と Mach-O 側の `MachoLibSystemCacheManager` は
同じキャッシュディレクトリ・同じスキーマ（`LibcCacheFile`）を共有する。
ただし解析対象ライブラリと arm64 限定という制約の点で、実装は独立している。

## 5. ELF arm64 との主な相違点

| 項目 | Mach-O (machoanalyzer) | ELF (elfanalyzer) |
|---|---|---|
| syscall 命令 | `SVC #0x80` (0xD4001001) | `SVC #0` (0xD4000001) |
| syscall 番号レジスタ | X16 | W8/X8 |
| Go ラッパー引数 | スタック `[SP, #8]` | レジスタ X0 |
| pclntab 解析 | `ParseMachoPclntab` (Mach-O 専用) | `parsePclntabFuncs` (.gopclntab ELF セクション) |
| 動的ライブラリ解析 | `machodylib` (LC_LOAD_DYLIB + dyld キャッシュ除外) | `elfdynlib` (DT_NEEDED + ldconfig キャッシュ) |
| libc キャッシュ | `MachoLibSystemCacheManager` (libsystem_kernel.dylib) | `LibcCacheManager` (glibc) |
| Fat binary | 全スライスを解析、最悪ケースを報告 | 非対応（ELF は単一アーキテクチャ） |
| トランポリン解決 | 1 段階 B スタブを解決 | なし |

## 6. 設計判断の根拠

### 6.1 machoanalyzer を elfanalyzer と独立したパッケージにした理由

ELF と Mach-O はバイナリ形式・syscall 規約・ライブラリシステムが根本的に異なるため、
共有できるコードが少なく、統合するとかえって複雑になる。
また、macOS 向け機能は Darwin 固有の API（`debug/macho`、dyld キャッシュ）に依存するため、
ELF 解析と分離することでビルドタグなしに両プラットフォームをサポートできる。

### 6.2 dyld 共有キャッシュを除外する理由

dyld 共有キャッシュは macOS のシステムアップデートで更新されるが、
個別ファイルとしては存在しないため SHA256 ハッシュを計算できない。
また、dyld 共有キャッシュの内容はユーザー空間から変更できないため、
整合性検証の対象とする実益が薄い。
一方、非キャッシュライブラリ（`@rpath` 経由のカスタムフレームワーク等）は
攻撃者が置き換える可能性があるため、ハッシュ検証の対象とする。

### 6.3 Pass 2 がスタック ABI を使用する理由

Go の `syscall.Syscall` 等の NOSPLIT アセンブリスタブは Arm64 の Go レジスタ ABI
（Go 1.17 以降）ではなく、旧来のスタック ABI を維持している。
これは NOSPLIT スタブが goroutine スタックをスイッチしないため、
Go ランタイムのスタック管理との互換性を保つためである。
したがって Pass 2 の逆方向スキャンは X0 ではなく `[SP, #8]` を追跡する必要がある。

### 6.4 Fat binary の全スライス解析

悪意ある開発者が arm64 スライスを安全に見せかけつつ、x86_64 スライスに
ネットワーク機能や危険な syscall を仕込む可能性がある（cross-slice attack）。
全スライスを解析して最悪ケースを報告することでこの迂回を防止する。
