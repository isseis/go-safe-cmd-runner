# 動的ライブラリ解析結果ストア導入 アーキテクチャ設計書

## 1. 設計目標

- ライブラリ解析結果を実行ファイルレコードから分離し、ライブラリ単位の動的ライブラリ解析結果ストアへ集約する
- runner は実行時に動的ライブラリ解析結果を読み取り、syscall_analysis / symbol_analysis / dynamic_load_symbols からリスク判定を導出する
- 解析結果ストアは libc-cache と独立したスキーマ・保存先・用途で実装する
- DynLibDeps に記録済みのハッシュを解析結果取得キーに利用し、ライブラリファイルの二重読み取りを避ける

---

## 2. 用語規則

- record 文脈での再利用戦略に限ってキャッシュという語を使う
- runner 文脈では常に動的ライブラリ解析結果という語を使う
- 実装パッケージ名は internal/dynlibanalysisstore を最終形とする

---

## 3. 全体フロー

### 3.1 record フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    BIN[(実行ファイル ELF)] --> DEPS[analyzeDynLibDeps 依存ライブラリ解決]
    DEPS --> DYNLIB[(record.DynLibDeps soname/path/hash)]
    DYNLIB --> LIBANA[analyzeLibraries]
    LIBANA --> FILTER{wrapper / VDSO?}
    FILTER -->|Yes| SKIP[スキップ]
    FILTER -->|No| MEM{メモリキャッシュ Hit?}
    MEM -->|Hit| RESULT[(解析結果)]
    MEM -->|Miss| STORE{解析結果ストア Hit?}
    STORE -->|Hit| RESULT
    STORE -->|Not Found| ANALYZE[ライブラリ解析]
    ANALYZE --> WRITE[解析結果ストアへ保存]
    WRITE --> RESULT
    DYNLIB --> RECORD[(record JSON DynLibDepsのみ)]

    class BIN,DYNLIB,RESULT,RECORD data;
    class DEPS,LIBANA,FILTER,MEM,STORE,ANALYZE process;
    class WRITE enhanced;
```

### 3.2 runner フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    JSON[(record JSON)] --> VERIFY[VerifyCommandDynLibDeps ハッシュ検証]
    VERIFY -->|NG| STOP1[エラー停止]
    VERIFY -->|OK| ISNET[IsNetworkOperation]

    ISNET --> BASE[バイナリ本体シグナル取得]
    ISNET --> DEPS[DynLibDeps取得]
    DEPS --> EACH[各 DynLibDep]
    EACH --> LOAD[解析結果読込 path + hash]
    LOAD -->|Not Found| STOP2[エラー停止]
    LOAD -->|Found| SIG[(syscall/symbol/dynamic_load)]

    BASE --> DECIDE[ネットワーク/高リスク判定]
    SIG --> DECIDE
    DECIDE --> OUT[判定結果]

    class JSON,SIG,OUT data;
    class VERIFY,ISNET,BASE,DEPS,EACH,LOAD,DECIDE process;
    class STOP1,STOP2 enhanced;
```

---

## 4. コンポーネント設計

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph storepkg[internal/dynlibanalysisstore]
        SCHEMA[schema.go 解析結果スキーマ]
        STORE[cache.go 実体は解析結果ストア]
        ERR[errors.go]
    end

    subgraph validator[internal/filevalidator]
        V[record側 解析と保存]
    end

    subgraph runner[internal/runner/base/security]
        R[runner側 読取と判定]
    end

    V -->|LoadOrAnalyzeAndStore| STORE
    R -->|LoadAnalysis| STORE

    class SCHEMA data;
    class V,R process;
    class STORE,ERR enhanced;
```

---

## 5. record 側設計

### 5.1 責務

- 解析対象ライブラリを抽出する
- 解析結果ストアに結果があれば再利用する
- 解析結果がなければ解析して保存する
- レコードには DynLibDeps のみを書き込む

### 5.2 再解析回避キャッシュ

record のみ、同一実行中のメモリキャッシュを利用する。

- 目的: 重複ライブラリ再解析の回避
- キー: libPath + # + libHash
- runner ではこのキャッシュ戦略を持ち込まない

---

## 6. runner 側設計

### 6.1 責務

- DynLibDeps と整合する動的ライブラリ解析結果を読み込む
- 読み込んだ解析結果から判定値を導出する
- 解析結果が取得できない場合は fail-closed で停止する

### 6.2 判定入力

runner が使う入力は以下の 3 つのみとする。

- syscall_analysis
- symbol_analysis
- dynamic_load_symbols

### 6.3 解析結果未取得時の挙動

runner 実行時に必要な解析結果が取得できない場合は、record 未実行、保存失敗、
破損などを示すためエラー停止する。

---

## 7. エラー処理方針

| 発生フェーズ | 状況 | 対応 |
|---|---|---|
| record 時 | Analyze がファイル不在を検出 | error を返し、当該実行ファイルのレコードは不出力。セッションは次ファイルへ継続 |
| record 時 | Analyze が 1 GB 超過を検出 | error を返し、当該実行ファイルのレコードは不出力。セッションは次ファイルへ継続 |
| record 時 | 解析結果読込失敗（破損等） | 警告を記録し再解析して継続 |
| record 時 | 解析結果保存失敗 | 警告を記録し継続（runner 時は解析結果未取得エラーになり得る） |
| runner 時 | 解析結果未取得 | エラー停止 |
| runner 時 | DynLibDeps ハッシュ不一致 | VerifyCommandDynLibDeps で先にエラー停止 |
| runner 時 | スキーマ不一致 | エラー停止（record 再実行が必要） |

### 7.1 ファイル不在とサイズ超過の統一処理

ファイル不在（FR-3.6.2）とサイズ超過（FR-3.6.1）は、ともに Analyze が error を返し、
analyzeLibraries が上位へ伝播する。

- 当該実行ファイルのレコードは書き込まれない
- record セッションは次の実行ファイル処理を継続する
