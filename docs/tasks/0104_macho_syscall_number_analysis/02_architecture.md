# アーキテクチャ設計書: Mach-O arm64 syscall 番号解析

## 1. システム概要

### 1.1 アーキテクチャ目標

- `svc #0x80` の存在ではなく syscall 番号の内容でリスク判定することで、正規の Go バイナリ（`record` コマンド自身を含む）の偽陽性を排除する
- `backwardScanX16`・`MacOSSyscallTable` など `libccache` の既存実装を最大限再利用し、重複開発を避ける
- ELF アナライザ（`elfanalyzer` パッケージ）の二パス設計（Pass 1: 直接 svc スキャン、Pass 2: ラッパー呼び出しサイト解析）を Mach-O arm64 向けに移植する
- `DeterminationMethodDirectSVC0x80` / `"direct_svc_0x80"` 判定方式を廃止し、`IsNetwork` フラグベースに統一する
- タスク 0100 の libSystem import 解析を維持しつつ、Mach-O 直接 syscall 番号解析結果と統合する

### 1.2 設計原則

- **既存活用**: `libccache.BackwardScanX16`・`MacOSSyscallTable`・`collectSVCAddresses` を再利用
- **ELF との一貫性**: `DeterminationMethod` 定数・`SyscallAnalysisResultCore` スキーマ・`GoWrapperResolver` 設計パターンを踏襲
- **フェイルセーフ**: 解析エラー時は `AnalysisError` 扱いで実行ブロック
- **YAGNI**: x86_64 Mach-O や非 Go バイナリへの汎用対応は本タスクのスコープ外

## 2. システム構成

### 2.1 全体アーキテクチャ（record 時 / verify 時）

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef removed fill:#ffe0e0,stroke:#cc0000,stroke-width:1px,color:#800000;

    subgraph record["record コマンド（解析・保存）"]
        A[("Mach-O バイナリ")] --> B["Validator.analyzeMachoSyscalls()"]
        B --> C["MachoSyscallNumberAnalyzer"]
        B --> LS["既存 libSystem import 解析"]
        C --> D["Pass 1: X16 後方スキャン"]
        C --> E["Pass 2: スタック ABI 呼び出しサイト解析"]
        D --> F["BackwardScanX16 (libccache)"]
        D --> G["MachoGoStubFilter (pclntab)"]
        E --> H["MachoWrapperResolver (pclntab)"]
        C --> I["MacOSSyscallTable.IsNetworkSyscall()"]
        C --> MM["merge(pass1, pass2, libSystem)"]
        LS --> MM
        MM --> J[("SyscallAnalysisData<br>v16 保存")]
    end

    subgraph verify["verify 時（ネットワーク判定）"]
        K[("SymbolAnalysis / SyscallAnalysis<br>v16 読み込み")] --> L["isNetworkViaBinaryAnalysis()"]
        L --> M{"DetectedSyscalls に<br>IsNetwork=true あり?"}
        M -->|"Yes"| N["true, true"]
        M -->|"No または SyscallAnalysis=nil"| O["SymbolAnalysis 判定へ委譲"]
    end

    J --> K

    class A,J,K data;
    class B,D,E,G,H,I,L,M process;
    class C,F enhanced;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[("データ")] --> P1["既存コンポーネント"] --> E1["新規/変更コンポーネント"]
    class D1 data
    class P1 process
    class E1 enhanced
```

### 2.2 パッケージ構成

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph "変更ファイル"
        subgraph "internal/fileanalysis"
            SA["schema.go<br>CurrentSchemaVersion 15→16"]
        end

        subgraph "internal/libccache"
            LA["macho_analyzer.go<br>backwardScanX16 → BackwardScanX16 (公開)"]
        end

        subgraph "internal/filevalidator"
            VA["validator.go<br>analyzeMachoSyscalls() 拡張"]
        end

        subgraph "internal/runner/security"
            NA["network_analyzer.go<br>syscallAnalysisHasSVCSignal 廃止<br>IsNetwork ベース判定に変更"]
        end
    end

    subgraph "追加ファイル（既存 internal/runner/security/machoanalyzer/）"
        AN["syscall_number_analyzer.go<br>MachoSyscallNumberAnalyzer<br>（Pass1 + Pass2 統合）"]
        P1["pass1_scanner.go<br>scanSVCWithX16()<br>Go スタブ除外 + X16 解析"]
        P2["pass2_scanner.go<br>MachoWrapperResolver<br>スタック ABI 呼び出しサイト解析"]
        PC["pclntab_macho.go<br>ParseMachoPclntab()<br>Mach-O 用 pclntab 読み込み"]
    end

    class SA,LA,VA,NA enhanced;
    class AN,P1,P2,PC enhanced;
```

### 2.3 record 時のデータフロー（二パス解析）

```mermaid
sequenceDiagram
    participant V as Validator
    participant AN as MachoSyscallNumberAnalyzer
    participant P1 as Pass1Scanner
    participant P2 as MachoWrapperResolver
    participant LC as libccache（BackwardScanX16）
    participant PT as ParseMachoPclntab
    participant ST as MacOSSyscallTable
    participant LS as existing libSystem analyzer
    participant FS as fileanalysis.Store

    V->>AN: Analyze(machoFile)
    AN->>PT: ParseMachoPclntab(machoFile)
    PT-->>AN: map[name]FuncRange（またはエラー）

    AN->>P1: scanSVCWithX16(code, textBase, stubRanges)
    loop svc #0x80 ごと
        P1->>P1: stubRanges に含まれる? → スキップ
        P1->>LC: BackwardScanX16(code, svcOffset)
        LC-->>P1: (syscallNum, ok)
        P1->>ST: IsNetworkSyscall(syscallNum)
        ST-->>P1: isNetwork
    end
    P1-->>AN: []SyscallInfo（Pass 1 結果）

    AN->>P2: FindWrapperCalls(code, textBase, wrapperAddrs)
    loop BL 命令ごと
        P2->>P2: wrapperAddrs に含まれる? → 処理対象
        P2->>P2: 後方スキャン [SP,#8] ストア → xN → 即値
        P2->>ST: IsNetworkSyscall(num)
        ST-->>P2: isNetwork
    end
    P2-->>AN: []SyscallInfo（Pass 2 結果）

    V->>LS: analyzeLibSystem(record, filePath)
    LS-->>V: []SyscallInfo（libSystem import 結果）

    AN-->>V: SyscallAnalysisResult（Pass1 + Pass2）
    V->>V: merge(pass1, pass2, libSystem)
    V->>FS: SaveSyscallAnalysis(path, hash, result)
```

### 2.4 verify 時のデータフロー（判定ロジック変更）

```mermaid
sequenceDiagram
    participant R as runner
    participant NA as NetworkAnalyzer
    participant SS as SyscallAnalysisStore
    participant SA as SyscallAnalysis (v16)

    R->>NA: isNetworkViaBinaryAnalysis(path, hash)
    NA->>SS: LoadSyscallAnalysis(path, hash)
    SS->>SA: Load record (schema v16)

    alt SchemaVersionMismatch（v15 レコード）
        SS-->>NA: SchemaVersionMismatchError
        NA-->>R: true, true（高リスク）
    else SyscallAnalysis == nil
        SS-->>NA: nil, nil
        NA->>NA: SymbolAnalysis 判定へ委譲
    else SyscallAnalysis あり
        SS-->>NA: result
        NA->>NA: DetectedSyscalls に IsNetwork=true あり?
        alt あり（ネットワーク syscall 検出）
            NA-->>R: true, true
        else なし
            NA-->>R: SymbolAnalysis 判定に委譲
        end
    end
```

## 3. コンポーネント設計

### 3.1 MachoSyscallNumberAnalyzer（新規）

`internal/runner/security/machoanalyzer/syscall_number_analyzer.go`（既存 `machoanalyzer` パッケージへの追加）

```
MachoSyscallNumberAnalyzer
├── Analyze(f *macho.File) (*fileanalysis.SyscallAnalysisResult, error)
│   ├── ParseMachoPclntab(f) → funcRanges / wrapperAddrs（gopclntab なければスキップ）
│   ├── collectSVCAddresses(f) → svcAddrs
│   ├── scanSVCWithX16(code, textBase, svcAddrs, stubRanges) → pass1Results
│   ├── FindWrapperCalls(code, textBase, wrapperAddrs) → pass2Results
│   └── merge(pass1Results, pass2Results) → SyscallAnalysisResult
```

ELF の `SyscallAnalyzer` と同等の役割。戻り値は `fileanalysis.SyscallAnalysisResult`（`common.SyscallAnalysisResultCore` 埋め込み）を使用して既存の保存・読み込みパスを変更なしに再利用する。libSystem import 解析結果との最終マージは既存の `Validator.analyzeMachoSyscalls()` 側で行う。

### 3.2 ParseMachoPclntab（新規）

`internal/runner/security/machoanalyzer/pclntab_macho.go`

Mach-O ファイルの `__gopclntab` セクションからデータを読み込み、ELF 版 `parsePclntabFuncsRaw` と同じコアロジック（`gosym.NewLineTable` + `gosym.NewTable`）で関数名 → アドレス範囲マップを構築する。

CGO オフセット補正は ELF 版と同一の `detectPclntabOffset` アルゴリズムを流用（`__TEXT,__text` セクションとの CALL/BL 相互参照）。

`__gopclntab` セクションが存在しない場合は `ErrNoPclntab` を返し、呼び出し側は Pass 1/Pass 2 の除外・解決なしで継続する。

### 3.3 Pass 1 スキャン（新規）

`internal/runner/security/machoanalyzer/pass1_scanner.go`

1. `collectSVCAddresses(f)` で `svc #0x80` アドレスを列挙（既存）
2. pclntab 由来の `stubRanges`（known Go stubs の関数アドレス範囲集合）に含まれるアドレスをスキップ（`isInsideRange` で判定）
3. 残りの `svc #0x80` 各アドレスについて `libccache.BackwardScanX16(code, svcOffset)` を呼び出す
4. 成功: `DeterminationMethod = "immediate"`、BSD prefix 除去済み syscall 番号、`MacOSSyscallTable.IsNetworkSyscall()` でネットワーク判定
5. 失敗（`ok == false`）: `DeterminationMethod = "unknown:indirect_setting"`、`Number = -1`、`IsNetwork = false`

主要な内部関数: `scanSVCWithX16(svcAddrs, code, textBase, stubRanges, table) []SyscallInfo`

`libccache.BackwardScanX16` を呼ぶには `backwardScanX16` を公開名（`BackwardScanX16`）に変更する。用途は内部パッケージ間の再利用であり、外部公開 API としての互換性保証までは不要とする。

Pass 1 と Pass 2 の結果は排他的である：Pass 1 はスタブアドレス範囲**外**の `svc #0x80` のみを解析し、Pass 2 はスタブ範囲**内**関数の呼び出しサイトを解析するため、同一アドレスに対して両 Pass が結果を生成することはない。マージ時の重複排除は不要。

### 3.4 Pass 2 スキャン（新規）

`internal/runner/security/machoanalyzer/pass2_scanner.go`

ELF の `ARM64GoWrapperResolver` に対応する `MachoWrapperResolver` を実装する。

- pclntab から `knownGoWrappers`（`syscall.Syscall` 等）のアドレスを解決し `wrapperAddrs` マップを構築
- ラッパー関数のアドレス範囲を `wrapperRanges`（ソート済み）として保持し、ラッパー内からの再帰 BL を除外（`IsInsideWrapper` で判定）
- `__TEXT,__text` を走査して `BL target` 命令を検出し、`target ∈ wrapperAddrs` であれば呼び出しサイトとして処理
- 呼び出しサイトから後方スキャン（最大 `defaultMaxBackwardScan` 命令）:
  1. `STP xN, ..., [SP, #8]` または `STR xN, [SP, #8]` を検出 → xN を特定
  2. xN への即値設定 `MOVZ xN, #imm` / `MOVZ xN, #hi, LSL#16` + `MOVK xN, #lo` を検出 → syscall 番号を取得
- 制御フロー命令（`B`/`BL`/`RET`/`CBZ` 等）を後方スキャンの境界とする
- 解決成功: `DeterminationMethod = "go_wrapper"`
- 解決失敗: `DeterminationMethod = "unknown:indirect_setting"`、`Number = -1`

ELF 版との違い: ELF arm64 は第1引数が X0 レジスタ（レジスタ ABI）だが、macOS arm64 では旧スタック ABI により第1引数は `[SP, #8]`。このオフセットは調査対象の Go arm64 Mach-O スタブで確認された呼び出し規約に基づく。

Pass 2 の解析対象（`knownGoWrappers`）は Pass 1 の除外対象（`knownMachoSyscallImpls`）のサブセットではなく、`runtime.syscall` 等の Go バージョン依存スタブが追加的に含まれる場合がある。Pass 2 はラッパーの呼び出し側を解析するため、`runtime.syscall` が内部で `svc #0x80` を発行するとしても Pass 2 は呼び出しサイトを対象とする。

### 3.5 libccache 変更

`backwardScanX16` → `BackwardScanX16` に改名（公開）。

変更はシグネチャと命名のみ。同パッケージ内の呼び出し箇所（`analyzeWrapperFunction`）を合わせて更新する。

### 3.6 network_analyzer.go 変更

`syscallAnalysisHasSVCSignal`（`"direct_svc_0x80"` を検出する関数）を削除する。

`SyscallAnalysis` の参照ロジックを以下に変更する：

| 条件 | 変更前 | 変更後 |
|------|--------|--------|
| `DetectedSyscalls` に `IsNetwork=true` あり | N/A | `true, true` |
| `DetectedSyscalls` に `IsNetwork=true` なし | N/A | `SymbolAnalysis` 判定へ委譲 |
| `DeterminationMethod = "direct_svc_0x80"` のエントリあり | `true, true` | **削除（廃止）** |
| `SyscallAnalysis == nil`（`nil, nil` 返却） | `false, false` | `SymbolAnalysis` 判定へ委譲 |

### 3.7 スキーマバージョン

`internal/fileanalysis/schema.go` の `CurrentSchemaVersion` を `15` → `16` に変更する。

v15 レコード読み込み時は既存の `SchemaVersionMismatchError` が返却され、`network_analyzer.go` の `case errors.As(err, &svcSchemaMismatch)` ブランチが `true, true` を返す（既存のフェイルセーフ動作）。

## 4. セキュリティアーキテクチャ

### 4.1 フェイルセーフ設計

```mermaid
flowchart TD
    A["svc #0x80 検出"] --> B{"既知 Go スタブ<br>アドレス範囲内?"}
    B -->|"Yes (Pass 1 除外)"| C["Pass 2 で呼び出しサイト解析"]
    B -->|"No"| D["BackwardScanX16"]
    D -->|"ok=true"| E["syscall 番号確定<br>DeterminationMethod=immediate"]
    D -->|"ok=false"| F["Number=-1<br>DeterminationMethod=unknown:indirect_setting"]
    C -->|"即値確定"| G["syscall 番号確定<br>DeterminationMethod=go_wrapper"]
    C -->|"未確定"| F
    E --> H["MacOSSyscallTable.IsNetworkSyscall()"]
    G --> H
    F --> I["IsNetwork=false<br>（リスクなし）"]
    H -->|"true"| J["IsNetwork=true"]
    H -->|"false"| K["IsNetwork=false"]
```

- `Number=-1` かつ `IsNetwork=false` は偽陰性のリスクがあるが、これらは Go runtime スタブ内の `svc #0x80` に由来し、Pass 2 でネットワーク syscall の呼び出しサイトが別途補足される
- 解析エラー（Mach-O パース失敗等）は `AnalysisError` として伝播し、`network_analyzer.go` が `true, true` を返す

### 4.2 ELF との一貫性

| 項目 | ELF arm64 | Mach-O arm64（本タスク後） |
|------|-----------|--------------------------|
| 検出命令 | `svc #0` | `svc #0x80` |
| syscall 番号レジスタ | X8 | X16 |
| Pass 1 後方スキャン関数 | `arm64BackwardScan` | `BackwardScanX16`（libccache） |
| Pass 2 第1引数 | X0（レジスタ ABI） | `[SP, #8]`（旧スタック ABI） |
| `DeterminationMethod` 定数 | `elfanalyzer` パッケージ定義 | 同定数を使用（`immediate` / `go_wrapper` / `unknown:*`） |
| pclntab 読み込み | `elfanalyzer.ParsePclntab` | `machoanalyzer.ParseMachoPclntab`（新規） |

## 5. パフォーマンス設計

Pass 1・Pass 2 はいずれも `__TEXT,__text` セクションを 4 バイト単位で一方向スキャンする（O(N)、N = コードサイズ）。pclntab の読み込みは解析開始時に一度だけ実施する。

ELF 版と同等のスキャン方式であり、`record` コマンドの処理時間への影響は軽微と見込む。

## 6. 依存関係と影響範囲

```mermaid
graph LR
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef new fill:#fff3cd,stroke:#ffc107,stroke-width:2px,color:#856404;
    classDef unchanged fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    ELF["elfanalyzer<br>（変更なし）"] --- COMMON["common/syscall_types.go<br>（変更なし）"]
    COMMON --- FSA["fileanalysis/schema.go<br>v15→v16"]
    FSA --- STORE["fileanalysis/syscall_store.go<br>（変更なし）"]
    STORE --- NA["network_analyzer.go<br>判定ロジック変更"]
    LC["libccache/macho_analyzer.go<br>BackwardScanX16 公開"] --- P1["machoanalyzer/pass1_scanner.go<br>（新規）"]
    LC2["libccache/MacOSSyscallTable<br>（変更なし）"] --- P1
    LC2 --- P2["machoanalyzer/pass2_scanner.go<br>（新規）"]
    P1 --- ANALY["machoanalyzer/<br>syscall_number_analyzer.go<br>（新規）"]
    P2 --- ANALY
    PT["machoanalyzer/pclntab_macho.go<br>（新規）"] --- ANALY
    ANALY --- FVAL["filevalidator/validator.go<br>analyzeMachoSyscalls 拡張"]

    class FSA,NA,FVAL,LC changed;
    class ANALY,P1,P2,PT new;
    class ELF,COMMON,STORE,LC2 unchanged;
```

**影響を受けないコンポーネント:**
- `elfanalyzer` パッケージ（変更なし）
- `common/syscall_types.go`（`DeterminationMethodDirectSVC0x80` 定数は互換性のため残す）
- `fileanalysis/syscall_store.go`・`SyscallAnalysisStore` インターフェース（変更なし）
- `libccache/MacOSSyscallTable`（変更なし）
