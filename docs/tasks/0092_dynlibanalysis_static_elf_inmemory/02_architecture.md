# アーキテクチャ設計書: TestAnalyze_StaticELF のインメモリ ELF 生成への移行

## 1. システム概要

### 1.1 アーキテクチャ目標

- 外部ファイル依存の解消
- 既存のインメモリ ELF ビルダー（`buildTestELFWithDeps`）の再利用
- テストの安定実行（GCC・`make` 不要）

### 1.2 設計原則

- **既存活用**: 同テストファイル内に存在する `buildTestELFWithDeps` ヘルパーを再利用する
- **YAGNI**: 新たなヘルパー関数は追加しない。`buildTestELFWithDeps` に `sonames=nil` を渡すことで静的 ELF を表現する
- **最小変更**: 変更対象は `TestAnalyze_StaticELF` 関数本体のみ

## 2. 変更前後の構成比較

### 2.1 変更前: 外部ファイル依存

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    A[("elfanalyzer/testdata/static.elf")] -->|"ファイル存在確認"| B{"ファイルあり?"}
    B -->|"なし"| C["t.Skipf → テストスキップ"]
    B -->|"あり"| D["Analyze(path)"]
    D --> E["assert.Nil(result)"]

    F[("make elfanalyzer-testdata<br>GCC 依存")] -->|"生成"| A

    class A,F data;
    class D,E process;
    class B,C problem;
```

**問題点**

- `make elfanalyzer-testdata` 未実行時はテストがスキップされる（カバレッジ欠落）
- `dynlibanalysis` パッケージが `elfanalyzer` パッケージのテストデータに依存（責務境界違反）

### 2.2 変更後: インメモリ ELF 生成

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    A[("t.TempDir()")] --> B["buildTestELFWithDeps<br>sonames=nil, runpath=&quot;&quot;"]
    B -->|"インメモリ ELF バイト列"| C[("静的 ELF (DT_NEEDED なし)")]
    C --> D["Analyze(path)"]
    D --> E["assert.Nil(result)"]

    class A data;
    class B enhanced;
    class C data;
    class D,E process;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    D1[("設定・環境データ")] --> P1["既存コンポーネント"] --> E1["変更・追加コンポーネント"] --> X1["問題箇所"]
    class D1 data
    class P1 process
    class E1 enhanced
    class X1 problem
```

## 3. コンポーネント設計

### 3.1 対象ファイルと変更範囲

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph "internal/dynlibanalysis/analyzer_test.go"
        H["buildTestELFWithDeps<br>（既存ヘルパー・変更なし）"]
        T["TestAnalyze_StaticELF<br>（変更対象）"]
    end

    subgraph "internal/runner/security/elfanalyzer/testdata/"
        F[("static.elf<br>（参照除去・ファイル自体は残す）")]
    end

    H -->|"再利用"| T
    F -. "参照を除去" .-> T

    class H process;
    class T enhanced;
    class F data;
```

### 3.2 `buildTestELFWithDeps` による静的 ELF の表現

`buildTestELFWithDeps` の引数に `sonames=nil`、`runpath=""` を渡すことで、DT_NEEDED エントリを持たない ELF バイナリが生成される。

生成される ELF の構造:

| セクション/セグメント | 内容 |
|---|---|
| ELF Header | ELF64 LE, ET_DYN, EM_X86_64 |
| PT_LOAD | ファイル全体をカバーするロードセグメント |
| PT_DYNAMIC | .dynamic セクションを指す動的セグメント |
| .dynamic | DT_STRTAB, DT_STRSZ, DT_NULL のみ（DT_NEEDED なし） |
| .dynstr | 空文字列のみ（`\x00`） |
| .shstrtab | セクション名テーブル |

### 3.3 変更前後のコード比較

**変更前**:

```go
func TestAnalyze_StaticELF(t *testing.T) {
    staticELF := "../runner/security/elfanalyzer/testdata/static.elf"
    if _, err := os.Stat(staticELF); err != nil {
        t.Skipf("static.elf testdata not accessible: %v", err)
    }

    a := newTestAnalyzer(t)
    result, err := a.Analyze(staticELF)
    require.NoError(t, err)
    assert.Nil(t, result, "static ELF with no DT_NEEDED should return nil")
}
```

**変更後**:

```go
func TestAnalyze_StaticELF(t *testing.T) {
    tmpDir := t.TempDir()
    // sonames=nil produces an ELF with no DT_NEEDED entries.
    staticELF := buildTestELFWithDeps(t, tmpDir, "static.elf", nil, "")

    a := newTestAnalyzer(t)
    result, err := a.Analyze(staticELF)
    require.NoError(t, err)
    assert.Nil(t, result, "static ELF with no DT_NEEDED should return nil")
}
```

## 4. データフロー

```mermaid
sequenceDiagram
    participant T as "TestAnalyze_StaticELF"
    participant B as "buildTestELFWithDeps"
    participant FS as "t.TempDir()"
    participant A as "DynLibAnalyzer.Analyze()"

    T->>FS: "一時ディレクトリ作成"
    T->>B: "buildTestELFWithDeps(t, tmpDir, 'static.elf', nil, '')"
    B->>B: "ELF バイト列をメモリ上で構築"
    B->>FS: "ELF ファイルを書き込む"
    B-->>T: "ファイルパスを返す"
    T->>A: "Analyze(path)"
    A->>A: "ELF を解析: DT_NEEDED なし"
    A-->>T: "nil, nil"
    T->>T: "assert.Nil(result) → PASS"
```

## 5. 影響範囲

| 対象 | 変更内容 |
|---|---|
| `internal/dynlibanalysis/analyzer_test.go` | `TestAnalyze_StaticELF` のみ書き換え |
| `internal/runner/security/elfanalyzer/testdata/static.elf` | 変更なし（`elfanalyzer` パッケージのテストが引き続き使用） |
| `Makefile` の `elfanalyzer-testdata` ターゲット | 変更なし |
| その他テスト | 変更なし |

## 6. 削除対象の依存関係

| 依存 | 削除理由 |
|---|---|
| `os.Stat` によるファイル存在確認 | インメモリ生成により不要 |
| `t.Skipf` によるスキップ処理 | インメモリ生成により不要 |
| `"../runner/security/elfanalyzer/testdata/static.elf"` パス参照 | パッケージ間依存の解消 |
