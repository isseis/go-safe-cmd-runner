# アーキテクチャ設計書: Record スキーマ v22

## 1. 設計方針

### 1.1 目的

1. リスク判定入力を Record トップレベルへ集約する
2. `deps` と `shebang_chain` を検証専用データへ縮約する
3. `runner` の解析依存を `RecordStore` 単一依存にする

### 1.2 原則

1. Self-contained: 実行時の解析判断は Record 1件で完結させる
2. Single source of risk: ネットワークリスク判定は `syscall_analysis` と `symbol_analysis` のみを参照する
3. Fail-closed: dedup 不整合や shebang 再解決不一致は実行停止とする

## 2. 全体構成

### 2.1 Before / After

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef proc fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef change fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph BEFORE[Before]
        BRUN[runner]
        BREC[(command Record)]
        BDYN[(dynlib-analysis cache)]
        BSHE[(shebang interpreter record)]
        BRUN --> BREC
        BRUN --> BDYN
        BRUN --> BSHE
    end

    subgraph AFTER[After]
        ARUN[runner]
        AREC[(command Record v22)]
        ADYN[(dynlib-analysis cache)]
        ARUN --> AREC
        ADYN -. internal only .-> AREC
    end

    class BREC,BDYN,BSHE,AREC,ADYN data;
    class BRUN,ARUN proc;
    class AREC,ARUN change;
```

### 2.2 コンポーネント責務

| コンポーネント | 主責務 | 非責務 |
|---|---|---|
| `record` | 解析対象全体を解析し、トップレベル解析結果を統合して Record に保存 | 実行時リスク判定 |
| `runner.NetworkAnalyzer` | Record のトップレベル解析結果だけでリスク判定 | dep ごとの再解析、shebang 追跡解析 |
| `verifyShebangChain` | `raw_path` と `command_name` の実行時再解決検証 | リスク判定 |
| `deps` | hash 整合性検証対象の列挙 | リスク判定入力 |

## 3. データモデル

### 3.1 Record v22 論理モデル

```mermaid
flowchart TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef proc fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    REC[(Record v22)]
    SA[(syscall_analysis)]
    SBA[(symbol_analysis)]
    AW[(analysis_warnings)]
    DEPS[(deps: path + hash)]
    SH[(shebang_chain)]
    DBG[(debug.dep_sources)]

    REC --> SA
    REC --> SBA
    REC --> AW
    REC --> DEPS
    REC --> SH
    REC --> DBG

    class REC,SA,SBA,AW,DEPS,SH,DBG data;
```

### 3.2 スキーマ責務分離

1. `syscall_analysis` / `symbol_analysis`
   リスク判定用の統合済みデータ
2. `deps`
   実行時 hash 検証用の参照データ
3. `shebang_chain`
   実行時の再解決整合性検証データ。各エントリの `ref`（絶対パスまたはベア名）を解決して `path` と比較する
4. `analysis_warnings`
   非致命警告の統合ログ
5. `debug.dep_sources`
   `-debug-info` 時のみのトレーサビリティ情報

## 4. 処理フロー

### 4.1 record 側

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef proc fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef gate fill:#fffbe6,stroke:#d48806,stroke-width:1px,color:#8a5a00;

    TGT[(target binary or script)] --> A1[Analyze target binary]
    A1 --> S1{Has shebang}
    S1 -->|yes| A2[Resolve shebang chain binaries]
    S1 -->|no| C1[Collect deps]
    A2 --> C1
    C1 --> D1[Dedup deps by path]
    D1 --> G1{Same path hash mismatch}
    G1 -->|yes| E1[Abort record generation]
    G1 -->|no| A3[Analyze all binaries and dep libs]
    A3 --> M1[Merge and dedup syscall by number]
    M1 --> M2[Merge and dedup symbol by name]
    M2 --> M3[Merge ArgEvalResults with worst-case]
    M3 --> W1[Merge and dedup warnings]
    W1 --> O1[(Write Record v22 atomically)]

    class TGT,O1 data;
    class A1,A2,C1,D1,A3,M1,M2,M3,W1,E1 proc;
    class S1,G1 gate;
```

### 4.2 runner 側

```mermaid
sequenceDiagram
    participant R as runner
    participant RS as RecordStore
    participant V as verifyShebangChain
    participant N as NetworkAnalyzer
    participant OS as OS

    R->>RS: LoadRecord(command)
    RS-->>R: Record v22
    R->>V: Verify(record.shebang_chain)
    V->>OS: EvalSymlinks(ref) when ref is absolute path
    V->>OS: LookPath(ref)+EvalSymlinks when ref is bare name
    V-->>R: ok or error
    R->>N: Analyze(record)
    N->>N: Read only record.syscall_analysis
    N->>N: Read only record.symbol_analysis
    N-->>R: network/high-risk signals
```

## 5. 変更対象設計

### 5.1 record

1. コマンド本体、shebang チェーン各バイナリ、各 dep ライブラリを解析対象とする
2. VDSO と syscall wrapper ライブラリは解析スキップする
3. 結果はトップレベルへ統合し、`deps` には `path` + `hash` のみを出力する
4. `saveInterpreterRecord` は削除する
5. `analysis_warnings` は Record トップレベルに統合する

### 5.2 runner / NetworkAnalyzer

1. `AnalysisDeps` は `RecordStore` のみを保持する
2. `analyzeBinarySignals` は Record ロード後、トップレベル解析結果のみで判定する
3. `checkDepsSignals` を削除する
4. `followShebangChain`（解析目的）を削除する
5. `ErrDepAnalysisNotEmbedded` を削除する

### 5.3 verification.Manager

1. `GetAnalysisDeps` は `AnalysisDeps{RecordStore: m.fileValidator}` を返す
2. `networkSymbolStore` `syscallAnalysisStore` `dynLibDepsStore` `dynlibAnalysisStore` `shebangStore` を削除する

## 6. セキュリティ設計

### 6.1 検出ポイント

```mermaid
flowchart LR
    classDef threat fill:#ffe6e6,stroke:#d62728,stroke-width:1px,color:#7f0000;
    classDef control fill:#e8f5e8,stroke:#2e8b57,stroke-width:1px,color:#006400;

    T1["Shebang symlink tampering"] --> C1["ref is absolute: EvalSymlinks(ref) must equal path"]
    T2["PATH hijack for env shebang"] --> C2["ref is bare name: LookPath(ref)+EvalSymlinks must equal path"]
    T3["Dep binary replacement"] --> C3["Hash verification via deps path/hash"]
    T4["Analysis source divergence"] --> C4["Risk decision uses unified top-level analysis only"]

    class T1,T2,T3,T4 threat;
    class C1,C2,C3,C4 control;
```

### 6.2 エラー境界

1. dedup 中の同一 path hash 不一致は `record` 側で致命エラー
2. shebang 再解決不一致は `runner` 側で致命エラー
3. v21 以下 Record は `SchemaVersionMismatchError`

## 7. 文書整合ルール

1. AC番号は [./01_requirements.md](./01_requirements.md) に一致させる
2. テスト対応表は [./03_detailed_specification.md](./03_detailed_specification.md) と [./04_implementation_plan.md](./04_implementation_plan.md) で同一の削除対象を指す
