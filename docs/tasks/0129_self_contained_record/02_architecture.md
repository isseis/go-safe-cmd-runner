# アーキテクチャ設計書: コマンド Record 完全自己完結化

## 1. 設計概要

### 1.1 設計目標

- `runner` が参照するファイルをコマンドの Record JSON 1ファイルのみにする
- Record ファイル1つで完全な解析情報を提供し、配布・移植を容易にする
- `runner` の `dynamicanalysis.Store` への依存を除去してコードをシンプルにする

### 1.2 設計原則

- **自己完結性**: コマンドの Record は実行時判断に必要な全情報を内包する
- **dedup**: 複数の依存元から参照される共有ライブラリは `path` を主キーとして1エントリに統合する（hash が一致する場合に統合。不一致の場合は致命的エラーとして `record` を中断する）
- **キャッシュの分離**: dynlib-analysis キャッシュは `record` の内部最適化手段として維持し、`runner` からは不可視とする
- **段階的デバッグ**: 通常用途には必要最小限の情報のみ記録し、`-debug-info` 時のみ詳細な由来情報を追記する

## 2. システム構成

### 2.1 全体アーキテクチャ（変更前後）

**変更前**: `runner` が複数ファイルを参照

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    RUN[["runner"]]
    RUN -->|"① 読み込み"| CMD[("command.json\n（参照のみ）")]
    RUN -->|"② 読み込み × N"| DYN[("dynlib-analysis/\n各ライブラリ JSON")]
    RUN -->|"③ 読み込み"| INT[("interpreter.json\n（hash・解析結果を保持）")]

    class CMD,DYN,INT data;
    class RUN process;
```

**変更後**: `runner` は1ファイルのみ参照

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    RUN[["runner"]]
    RUN -->|"① 読み込み（1ファイルのみ）"| CMD[("command.json\n（解析結果を内包）")]
    CACHE[("dynlib-analysis/\n（record の内部キャッシュ）\n※ runner は参照しない")]

    class CMD,CACHE data;
    class RUN process;
    class CMD enhanced;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[("データ")] --- P1["既存コンポーネント"] --- E1["拡張・変更コンポーネント"]
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

    subgraph "internal/fileanalysis"
        SCHEMA["schema.go\nRecord 構造体（拡張）\nDepEntry, ShebangBinaryInfo, DebugInfo 追加"]
    end

    subgraph "cmd/record"
        REC["main.go\ndep 収集・dedup・解析・埋め込みロジック追加\nshebang チェーン解析を shebang_chain に出力"]
    end

    subgraph "internal/dynamicanalysis"
        DYN_STORE["store.go\nrecord 内部キャッシュ（変更なし）\nrunner からは不可視"]
    end

    subgraph "internal/runner/base/security"
        NET_ANA["network_analyzer.go\ndynamicanalysis.Store 依存を除去\nRecord.Deps から解析結果を読む"]
    end

    subgraph "internal/filevalidator"
        VALID["validator.go\nshebang_chain を参照するよう更新"]
    end

    REC -->|"キャッシュ利用"| DYN_STORE
    REC -->|"Record 生成"| SCHEMA
    NET_ANA -->|"Record 参照"| SCHEMA
    VALID -->|"Record 参照"| SCHEMA

    class SCHEMA,DYN_STORE data;
    class VALID,NET_ANA process;
    class REC,SCHEMA enhanced;
```

### 2.3 record コマンドの処理フロー（変更後）

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    CMD_FILE[("実行ファイル\n（ELF / スクリプト）")]
    CMD_FILE --> ANALYZE["コマンド本体解析\n（syscall / symbol / dyn_lib_deps）"]
    ANALYZE --> SHEBANG{"shebang?"}

    SHEBANG -->|"あり"| CHAIN["shebang チェーン解析\n（各バイナリの hash / syscall / symbol / deps）"]
    SHEBANG -->|"なし"| COLLECT

    CHAIN --> COLLECT["全 dyn_lib_deps を収集\n（コマンド + shebang チェーン全バイナリ）"]
    COLLECT --> DEDUP["path+hash で dedup"]

    DEDUP --> ANALYZE_DEP["各ライブラリ解析\n（キャッシュ優先）"]
    ANALYZE_DEP --> CACHE_CHK{"dynlib キャッシュ\nヒット?"}
    CACHE_CHK -->|"ヒット"| USE_CACHE["キャッシュ利用"]
    CACHE_CHK -->|"ミス"| FRESH["新規解析 → キャッシュ保存"]

    USE_CACHE --> BUILD
    FRESH --> BUILD["Record 構築\n（全解析結果を埋め込み）"]
    BUILD --> WRITE[("command.json 出力\n（アトミック書き出し）")]

    class CMD_FILE,WRITE data;
    class ANALYZE,CHAIN,COLLECT,DEDUP process;
    class ANALYZE_DEP,CACHE_CHK,USE_CACHE,FRESH,BUILD enhanced;
```

## 3. コンポーネント設計

### 3.1 スキーマ設計（fileanalysis.Record の変更）

**変更前後のフィールド対応:**

| 変更前フィールド | 変更後フィールド | 変更内容 |
|-----------------|-----------------|---------|
| `DynLibDeps []LibEntry` | `Deps []DepEntry` | 解析結果（`syscall_analysis`、`symbol_analysis`、`warnings`）を追加 |
| `ShebangInterpreter *ShebangInterpreterInfo` | `ShebangChain []ShebangBinaryInfo` | `content_hash` と解析結果を追加、リスト形式に変更 |
| `AnalysisWarnings []string`（Record レベル） | `DepEntry.Warnings []string`（各エントリ内） | 警告を dep ごとに記録 |
| （なし） | `Debug *DebugInfo` | `-debug-info` 時のみ |

**新規型定義（概要）:**

```go
// DepEntry は単一の依存共有ライブラリを表す。
// Deps リスト内では path+hash をキーとして dedup される。
type DepEntry struct {
    SOName          string
    Path            string
    Hash            string
    SyscallAnalysis *SyscallAnalysisData  // nullable（syscall wrapper は nil）
    SymbolAnalysis  *SymbolAnalysisData   // nullable
    Warnings        []string              // 解析中の非致命的警告
}

// ShebangBinaryInfo は shebang チェーンの1バイナリを表す。
type ShebangBinaryInfo struct {
    RawPath         string  // shebang 行の記述（先頭エントリのみ）
    Path            string  // シンボリックリンク解決済みパス
    CommandName     string  // env 形式の引数名（env バイナリのエントリのみ）
    ContentHash     string
    SyscallAnalysis *SyscallAnalysisData  // nullable
    SymbolAnalysis  *SymbolAnalysisData   // nullable
}

// DebugInfo は -debug-info 時のみ記録されるデバッグ情報。
type DebugInfo struct {
    // DepSources は各 dep の由来バイナリパスのリストを保持する。
    // キー: dep の絶対パス、値: 由来バイナリ絶対パスのリスト
    DepSources map[string][]string
}
```

### 3.2 shebang_chain の JSON 表現例

**直接形式 `#!/bin/bash`:**

```json
"shebang_chain": [
  {
    "raw_path": "/bin/bash",
    "path": "/usr/bin/bash",
    "content_hash": "sha256:...",
    "syscall_analysis": { "architecture": "arm64", "detected_syscalls": [...] },
    "symbol_analysis": { "detected_symbols": [...] }
  }
]
```

**env 形式 `#!/usr/bin/env python3`:**

```json
"shebang_chain": [
  {
    "raw_path": "/usr/bin/env",
    "path": "/usr/bin/env",
    "command_name": "python3",
    "content_hash": "sha256:...",
    "syscall_analysis": null,
    "symbol_analysis": { "detected_symbols": [...] }
  },
  {
    "path": "/usr/bin/python3.12",
    "content_hash": "sha256:...",
    "syscall_analysis": { "architecture": "arm64", "detected_syscalls": [...] },
    "symbol_analysis": { "detected_symbols": [...] }
  }
]
```

### 3.3 debug.dep_sources の JSON 表現例（-debug-info 時のみ）

```json
"debug": {
  "dep_sources": {
    "/usr/lib/aarch64-linux-gnu/libz.so.1.3": [
      "/usr/local/bin/myscript.sh",
      "/usr/bin/python3.12"
    ],
    "/usr/lib/aarch64-linux-gnu/libssl.so.3": [
      "/usr/bin/python3.12"
    ]
  }
}
```

`dep_sources` のキーは dep の絶対パス、値はその dep を依存に持つバイナリの絶対パスのリスト（コマンド自身または shebang チェーンのバイナリ）。

### 3.4 deps の dedup ロジック

`record` コマンドが全バイナリの `dyn_lib_deps` を収集する際に、以下のロジックで dedup する。

1. コマンド本体の `dyn_lib_deps` を収集
2. shebang チェーンの各バイナリ（env バイナリを含む）の `dyn_lib_deps` を収集
3. `path` を主キーとして dedup する。同一 path で異なる hash が出現した場合は致命的エラーとして `record` を中断する（どちらのバイナリが実際にロードされるか不明なため、不正なセキュリティポリシー適用を防ぐ）
4. 各ユニークなライブラリについて dynlib-analysis キャッシュを参照し、ヒットしなければ新規解析

### 3.5 NetworkAnalyzer の変更

**変更前:**

```
NetworkAnalyzer.deps.LibAnalysisStore (dynamicanalysis.Store)
  ↓ LoadAnalysis(dep.Path, dep.Hash)
  ↓ *dynamicanalysis.Result
```

**変更後:**

```
NetworkAnalyzer は Record.Deps を直接参照
  ↓ dep.SyscallAnalysis, dep.SymbolAnalysis を読む
```

`NetworkAnalyzer` の `AnalyzerDeps` 構造体から `LibAnalysisStore dynamicanalysis.Store` フィールドを除去する。`checkDynLibDepsNetwork` は `[]fileanalysis.DepEntry` を受け取り、各エントリの解析フィールドを直接参照する。

## 4. エラーハンドリング設計

### 4.1 runner からの ErrAnalysisNotFound 除去

現在、dynlib キャッシュが存在しない場合は `ErrAnalysisNotFound` → 「高リスクフォールバック」の処理が `runner` に存在する。新設計では Record に全解析結果が埋め込まれるため、この処理は不要となる。

Record 自体が存在しない場合は既存の `SchemaVersionMismatchError` / ファイル不存在エラーが適用される（変更なし）。

### 4.2 dedup 時の hash 不一致

同一 path で異なる hash の dep が複数のバイナリから参照された場合:

- `record` コマンドを致命的エラーで中断する
- どちらの hash のバイナリが実際にロードされるか不明であり、いずれかの解析結果を採用することは不正なセキュリティポリシー適用につながるため、fail-closed とする
- Record は生成しない（不整合な Record が `runner` に読み込まれることを防ぐ）

## 5. セキュリティ考慮事項

### 5.1 整合性の維持

- `deps` の各エントリは hash を保持し、`runner` がファイルを使用する前にハッシュ検証を行う（既存の filevalidator の動作を維持）
- `shebang_chain` の各バイナリも `content_hash` を保持し、シンボリックリンクリダイレクト攻撃を検出できる
- Record の書き出しはアトミック（既存の動作を維持）

### 5.2 dynlib キャッシュの信頼性

dynlib-analysis キャッシュは `record` コマンドのみが参照する内部最適化手段であり、`runner` は参照しない。キャッシュが改ざんされても `record` が生成した Record の内容（解析結果）が `runner` の判断基準となるため、実行時のセキュリティには影響しない。キャッシュの改ざんが影響するのは `record` の次回実行時のみであり、hash 不一致により改ざんを検出できる（既存の動作を維持）。

### 5.3 脅威モデル: Record の情報完全性

```mermaid
flowchart TD
    classDef threat fill:#ffe6e6,stroke:#d62728,stroke-width:1px,color:#7f0000;
    classDef counter fill:#e8f5e8,stroke:#2e8b57,stroke-width:1px,color:#006400;
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;

    T1["脅威: 依存ライブラリの差し替え"]
    C1["対策: deps エントリの hash 検証\n（filevalidator による実行時検証）"]

    T2["脅威: shebang インタープリターの\nシンボリックリンク改ざん"]
    C2["対策: shebang_chain の raw_path と\ncontent_hash の突き合わせ\n（既存の検出ロジックを維持）"]

    T3["脅威: dynlib キャッシュの改ざん"]
    C3["対策: キャッシュは record のみが参照\nrunner は Record の埋め込み情報を使用"]

    T1 --> C1
    T2 --> C2
    T3 --> C3

    class T1,T2,T3 threat;
    class C1,C2,C3 counter;
```

## 6. 処理フロー詳細

### 6.1 record コマンドの deps 収集シーケンス

```mermaid
sequenceDiagram
    participant R as record コマンド
    participant E as Binary Analyzer
    participant S as Shebang Parser
    participant C as dynlib-analysis キャッシュ
    participant FS as ファイルシステム

    R->>E: コマンド本体解析
    E-->>R: syscall_analysis, symbol_analysis, dyn_lib_deps

    alt shebang スクリプト
        R->>S: shebang 行解析
        S-->>R: インタープリター情報（path, command_name 等）
        loop shebang チェーンの各バイナリ
            R->>E: バイナリ解析（hash + syscall + symbol + deps）
            E-->>R: ShebangBinaryInfo + dyn_lib_deps
        end
    end

    R->>R: 全 dyn_lib_deps を収集・dedup（path+hash キー）

    loop 各ユニーク dep
        R->>C: LoadAnalysis(path, hash)
        alt キャッシュヒット
            C-->>R: 解析結果
        else キャッシュミス
            R->>E: ライブラリ解析
            E-->>R: 解析結果
            R->>C: SaveResult(path, hash, result)
        end
    end

    R->>FS: Record JSON 書き出し（アトミック）
```

### 6.2 runner の NetworkAnalyzer 処理フロー（変更後）

```mermaid
sequenceDiagram
    participant RUN as runner
    participant FS as ファイルシステム
    participant NA as NetworkAnalyzer

    RUN->>FS: Record 読み込み（1ファイルのみ）
    FS-->>RUN: Record（Deps, ShebangChain を内包）

    RUN->>NA: CheckAnalysisCache(record)

    NA->>NA: checkSyscallCache（コマンド本体の SyscallAnalysis）
    NA->>NA: checkSymbolAnalysisCache（コマンド本体の SymbolAnalysis）

    loop Record.Deps の各エントリ
        NA->>NA: analyzeDepSignals(dep.SyscallAnalysis, dep.SymbolAnalysis)
    end

    loop Record.ShebangChain の各エントリ
        NA->>NA: analyzeChainBinarySignals(entry.SyscallAnalysis, entry.SymbolAnalysis)
    end

    NA-->>RUN: (isNetwork, isHighRisk)
```

## 7. テスト戦略

### 7.1 ユニットテスト

- `DepEntry` の JSON シリアライズ・デシリアライズ（`syscall_analysis` null / 非 null、`warnings` あり / なし）
- dedup ロジック（同一 path+hash → 統合、同一 path 異なる hash → 致命的エラーで中断）
- `ShebangBinaryInfo` の直接形式・env 形式の JSON 表現
- `DebugInfo` の `-debug-info` あり / なしでの生成（`omitempty` 動作）

### 7.2 統合テスト

- `record` コマンドが ELF バイナリの `deps` を埋め込んだ Record を生成する（AC-1〜4 検証）
- `record` コマンドが直接形式 shebang の `shebang_chain`（1エントリ）を生成する（AC-2 検証）
- `record` コマンドが env 形式 shebang の `shebang_chain`（2エントリ）を生成する（AC-3 検証）
- `runner` が dynlib-analysis キャッシュなしで Record のみから正しく動作する（F-003 AC-3 検証）
- スキーマバージョンミスマッチ時に `SchemaVersionMismatchError` が返される（F-005 AC-2 検証）

### 7.3 後方互換性テスト

- 旧バージョン（v21 以前）の Record を読み込んだ際に `SchemaVersionMismatchError` が返される
- `record` 再実行による旧 Record の上書きが正常に動作する

## 8. 実装の優先順位

### Phase 1: スキーマ定義
`fileanalysis/schema.go` に `DepEntry`、`ShebangBinaryInfo`、`DebugInfo` を追加し、`CurrentSchemaVersion` をインクリメント。既存フィールドを削除・置換。旧バージョン Record に対するエラー動作テストを追加。

### Phase 2: record コマンド
deps 収集・dedup・解析・埋め込みロジックを実装。shebang チェーン解析を `shebang_chain` に出力。`-debug-info` 時の `dep_sources` 生成を追加。

### Phase 3: runner の更新
`NetworkAnalyzer` から `dynamicanalysis.Store` 依存を除去し、`Record.Deps` を直接参照するよう変更。`filevalidator` の `ShebangChain` 参照を更新。

### Phase 4: 検証
全テストのパス、リンターパス、エンドツーエンド動作確認。

## 9. 将来の拡張性

- **推移的依存の解析**: 将来的に共有ライブラリが依存するライブラリ（推移的依存）も `deps` に追加する拡張が可能。`DepEntry` の構造はそのまま利用できる（現在は直接依存のみ）
- **多段 shebang チェーン**: `shebang_chain` はリスト構造のため、インタープリター自体がスクリプトである場合への対応が可能（現在はスコープ外）
- **並列 record**: dedup 後の各ライブラリ解析を並列実行する最適化が可能。現在のスキーマ設計は並列化に対応できる
