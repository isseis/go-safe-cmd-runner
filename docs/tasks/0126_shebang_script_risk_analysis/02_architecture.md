# shebang スクリプトのネットワークリスク解析 アーキテクチャ設計書

## 1. 設計目標

- runner がスクリプトを処理する際、インタープリタの JSON レコードも読み込み、そこに記録されたリスクシグナル（`SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps`）を活用する
- `record` 側のコードを変更せず、インタープリタバイナリの既存レコードを再利用する
- インタープリタの `SyscallAnalysis`（svc #0x80・ネットワーク syscall）・共有ライブラリの推移的解析を自動的に活用する
- `analyzeBinarySignals` の再帰呼び出しにより既存の解析ロジックを最大限再利用する

---

## 2. 現状と課題

### 2.1. record コマンドの動作（変更なし）

`SaveRecord(scriptPath)` は shebang スクリプトを処理するとき、インタープリタバイナリの record も自動保存する。

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    SCRIPT[("script.sh")] --> SHEBANG["resolveShebangInfo()"]
    SHEBANG --> SAVE_INTERP["saveInterpreterRecord(interpPath)"]
    SAVE_INTERP --> IREC[("インタープリタ record<br>SymbolAnalysis<br>SyscallAnalysis<br>DynLibDeps")]
    SCRIPT --> SAVE_SCRIPT["saveRecordCore(scriptPath)"]
    SAVE_SCRIPT --> SREC[("script record<br>ShebangInterpreter<br>  .InterpreterPath<br>  .ResolvedPath<br>  ...<br>SymbolAnalysis = nil<br>DynLibDeps = nil")]

    class SCRIPT,IREC,SREC data;
    class SHEBANG,SAVE_INTERP,SAVE_SCRIPT process;
```

インタープリタの record には完全な解析結果が保存されているが、script の record の `SymbolAnalysis` と `DynLibDeps` は `nil` のため runner のリスク判定が機能しない（課題）。

---

## 3. 変更後の runner リスク判定フロー

### 3.1. 全体フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    CMD["IsNetworkOperation(<br>  scriptPath, args, contentHash<br>)"] --> PROF{"commandProfiles<br>一致?"}
    PROF -->|"Yes"| RET1["結果返却"]
    PROF -->|"No"| BINS["analyzeBinarySignals(<br>  scriptPath, contentHash<br>)"]

    BINS --> SCRIPT_SYM["LoadNetworkSymbolAnalysis(scriptPath)<br>→ nil (script 自身はシンボルなし)"]
    BINS --> SCRIPT_SVC["checkSyscallCache(scriptPath)<br>→ nil (script に svc なし)"]
    BINS --> SCRIPT_DYNLIB["checkDynLibDepsNetwork(scriptPath)<br>→ nil (script に DynLibDeps なし)"]

    BINS --> SHEBANG["新規: ShebangInterpreterStore<br>.LoadInterpreterAnalysisPath(<br>  scriptPath, contentHash<br>)"]
    SHEBANG --> IPATH[("interpPath<br>interpContentHash")]
    IPATH --> RECURSE["analyzeBinarySignals(<br>  interpPath, interpContentHash<br>)"]

    RECURSE --> INTERP_SYM["LoadNetworkSymbolAnalysis(interpPath)<br>← インタープリタの SymbolAnalysis"]
    RECURSE --> INTERP_SVC["checkSyscallCache(interpPath)<br>← インタープリタの SyscallAnalysis<br>（svc #0x80 / network syscall）"]
    RECURSE --> INTERP_DYNLIB["checkDynLibDepsNetwork(interpPath)<br>← インタープリタの DynLibDeps<br>（推移的ライブラリ解析）"]

    RECURSE --> OR["OR 結合"]
    BINS --> OR
    OR --> RET2["(isNetwork, isHighRisk)"]

    class CMD,IPATH data;
    class PROF,BINS,SCRIPT_SYM,SCRIPT_SVC,SCRIPT_DYNLIB,SHEBANG,INTERP_SYM,INTERP_SVC,INTERP_DYNLIB,OR process;
    class RECURSE enhanced;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[("データ")] --> P1["既存コンポーネント"] --> E1["新規 / 拡張コンポーネント"]
    class D1 data;
    class P1 process;
    class E1 enhanced;
```

---

## 4. `ShebangInterpreterStore` インターフェース

### 4.1. インターフェース定義

`fileanalysis` パッケージに新規追加:

```go
// ShebangInterpreterStore provides the interpreter binary path and content hash
// for a shebang script, enabling the runner to follow the shebang chain.
type ShebangInterpreterStore interface {
    // LoadInterpreterAnalysisPath returns the effective interpreter binary path
    // and its content hash for the shebang script at scriptPath.
    // scriptContentHash is used to validate freshness of the script's record.
    // Returns ("", "", nil) if the script has no ShebangInterpreter or the
    // interpreter's record is not found.
    LoadInterpreterAnalysisPath(scriptPath, scriptContentHash string) (interpPath, interpContentHash string, err error)
}
```

### 4.2. 処理フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    START["LoadInterpreterAnalysisPath(<br>  scriptPath, scriptContentHash<br>)"] --> LOAD_S["v.store.Load(scriptPath)"]
    LOAD_S --> NOT_FOUND{"ErrRecordNotFound?"}
    NOT_FOUND -->|"Yes"| RET_ERR0["ErrRecordNotFound を返す<br>（呼び出し元で panic）"]
    NOT_FOUND -->|"No"| ERR1{"他のエラー?"}
    ERR1 -->|"Yes"| RET_ERR1["error を返す"]
    ERR1 -->|"No"| HASH_CHK{"scriptRecord.ContentHash<br>== scriptContentHash?"}
    HASH_CHK -->|"No"| RET_MISMATCH["ErrHashMismatch を返す<br>（整合性異常 → 実行中止）"]
    HASH_CHK -->|"Yes"| SI{"ShebangInterpreter<br>nil?"}
    SI -->|"Yes"| RET_NIL2["return ('', '', nil)"]
    SI -->|"No"| IPATH["interpPath 決定<br>(ResolvedPath 優先)"]
    IPATH --> LOAD_I["v.store.Load(interpPath)"]
    LOAD_I --> NOT_FOUND2{"ErrRecordNotFound?"}
    NOT_FOUND2 -->|"Yes"| RET_ERR3["error を返す<br>（整合性異常 → 実行中止）"]
    NOT_FOUND2 -->|"No"| ERR2{"他のエラー?"}
    ERR2 -->|"Yes"| RET_ERR2["error を返す"]
    ERR2 -->|"No"| HASH_EMPTY{"interpRecord.ContentHash<br>空?"}
    HASH_EMPTY -->|"Yes"| RET_ERR4["error を返す<br>（整合性異常 → 実行中止）"]
    HASH_EMPTY -->|"No"| RET_OK["return (interpPath,<br>  interpRecord.ContentHash,<br>  nil)"]

    class START,RET_NIL2,RET_OK data;
    class LOAD_S,NOT_FOUND,ERR1,HASH_CHK,SI,IPATH,LOAD_I,NOT_FOUND2,ERR2,HASH_EMPTY process;
```

---

## 5. インタープリタパス決定ロジック

| shebang 形式 | 使用するパス | 根拠 |
|-------------|-------------|------|
| `#!/bin/bash` (direct 形式) | `ShebangInterpreter.InterpreterPath` | シンボルリンク解決済みのインタープリタバイナリパス |
| `#!/usr/bin/env python3` (env 形式) | `ShebangInterpreter.ResolvedPath` | `env` ではなく実際に実行される `python3` のパス |

---

## 6. `analyzeBinarySignals` の拡張

### 6.1. 拡張箇所

既存の解析（`SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps`）の実行後、`shebangStore` が設定されている場合に shebang チェーン追跡を追加する。

```
analyzeBinarySignals(cmdPath, contentHash):
  [既存] SymbolAnalysis キャッシュ参照
  [既存] SyscallAnalysis キャッシュ参照（svc #0x80）
  [既存] DynLibDeps 推移的解析
  [新規] if shebangStore != nil && contentHash != "":
           interpPath, interpHash = shebangStore.LoadInterpreterAnalysisPath(cmdPath, contentHash)
           if interpPath != "" && interpHash != "":
               interpNet, interpHigh = analyzeBinarySignals(interpPath, interpHash)
               isNetwork |= interpNet
               hasDynLoad |= interpHigh
```

### 6.2. 再帰呼び出しが安全な理由

- インタープリタは常にネイティブバイナリ（ELF/Mach-O）であり、`record` でのシェバン再帰チェック（`ErrRecursiveShebang`）により保証される
- インタープリタのレコードには `ShebangInterpreter = nil`（バイナリのため）が格納されるため、再帰呼び出し先では shebang チェーン追跡がスキップされる（最大 1 段の再帰）

---

## 7. `NetworkAnalyzer` への注入

```go
// NetworkAnalyzer にフィールドを追加
type NetworkAnalyzer struct {
    goos             string
    store            fileanalysis.NetworkSymbolStore
    syscallStore     fileanalysis.SyscallAnalysisStore
    depsStore        fileanalysis.DynLibDepsStore
    libAnalysisStore dynamicanalysis.Store
    shebangStore     fileanalysis.ShebangInterpreterStore  // 新規（nil で無効化）
}
```

`NewNetworkAnalyzer` の引数に `shebangStore fileanalysis.ShebangInterpreterStore` を追加し、DI で注入する。

---

## 8. record 側を変更しない理由

- インタープリタバイナリの `record` 実行時に `SymbolAnalysis`・`SyscallAnalysis`・`DynLibDeps` はすでに保存される
- runner 側でインタープリタの JSON を読み込むことで、あらゆるリスクシグナルを自動的に取得できる
- record 側にデータを複製すると整合性の問題（インタープリタが更新された場合のキャッシュ陳腐化）が発生する可能性があるが、本設計では常に最新のインタープリタ記録を参照する
