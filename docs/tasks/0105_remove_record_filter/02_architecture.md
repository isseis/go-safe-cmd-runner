# アーキテクチャ設計書: record コマンドのシステムコールフィルタリング削除

## 1. システム概要

### 1.1 アーキテクチャ目標

- `record` コンポーネントからリスク判断ロジックを除去し、関心の分離を徹底する
- `runner` コンポーネントがフィルタリングされていない全システムコール情報を基にリスク判定できるようにする
- macOS BSD syscall テーブルを自動生成化し、手動管理コストを排除する

### 1.2 設計原則

- **関心の分離**: `record` はシステムコールを記録するのみ。フィルタリングは `runner` 側の責務
- **後方互換性**: JSON スキーマは変更しない。`detected_syscalls` の内容が増えるのみ
- **YAGNI**: 既存の構造・インターフェースを活用し、不要な抽象化を追加しない

## 2. 変更前後のアーキテクチャ

### 2.1 変更前: フィルタリングあり

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#ffe6e6,stroke:#cc0000,stroke-width:2px,color:#800000;

    A[("バイナリファイル")] -->|"ELF/Mach-O 解析"| B["buildSyscallData<br>buildMachoSyscallData"]
    B -->|"全システムコール"| C["FilterSyscallsForStorage<br>（IsNetwork または Number==-1 のみ残す）"]
    C -->|"フィルタ済みエントリ"| D[("JSON レコード<br>detected_syscalls")]
    D -->|"ロード"| E["network_analyzer<br>syscallAnalysisHasSVCSignal<br>syscallAnalysisHasNetworkSignal"]
    E -->|"リスク判定"| F["isHighRisk / isNetwork"]

    class A,D data;
    class B,E,F process;
    class C changed;
```

### 2.2 変更後: フィルタリングなし

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    A[("バイナリファイル")] -->|"ELF/Mach-O 解析"| B["buildSyscallData<br>buildMachoSyscallData"]
    B -->|"全システムコール（フィルタなし）"| D[("JSON レコード<br>detected_syscalls")]
    D -->|"ロード"| E["network_analyzer<br>syscallAnalysisHasSVCSignal<br>syscallAnalysisHasNetworkSignal"]
    E -->|"リスク判定"| F["isHighRisk / isNetwork"]

    class A,D data;
    class B,F process;
    class E changed;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[("データ")] --> P1["既存コンポーネント"] --> E1["変更コンポーネント"]
    class D1 data
    class P1 process
    class E1 changed
```

## 3. コンポーネント設計

### 3.1 変更コンポーネント一覧

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef new fill:#f0e6ff,stroke:#7f2fd4,stroke-width:2px,color:#4b0082;

    subgraph "internal/fileanalysis"
        A["syscall_store.go<br>FilterSyscallsForStorage 削除"]
    end

    subgraph "internal/filevalidator"
        B["validator.go<br>buildSyscallData: FilterSyscallsForStorage 呼び出し削除<br>buildMachoSyscallData: FilterSyscallsForStorage 呼び出し削除<br>AnalysisWarnings ロジック修正"]
    end

    subgraph "internal/runner/security"
        C["network_analyzer.go<br>syscallAnalysisHasSVCSignal: Number==-1 条件追加<br>syscallAnalysisHasNetworkSignal: DeterminationMethod 除外削除"]
    end

    subgraph "internal/libccache"
        D["macos_syscall_table.go<br>macOSSyscallEntries 定義削除"]
        E["macos_syscall_numbers.go<br>（新規・自動生成）<br>全 BSD syscall 収録"]
    end

    subgraph "scripts"
        F["generate_syscall_table.py<br>--macos-header オプション追加"]
    end

    subgraph "Makefile"
        G["generate-syscall-tables ターゲット更新<br>macOS SDK ヘッダー対応"]
    end

    class A,B,C,D,F,G changed;
    class E new;
```

### 3.2 `record` パスのデータフロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph ELFパス
        E1[("ELF バイナリ")] --> E2["AnalyzeSyscallsFromELF<br>（libc シンボルマッチ）"]
        E2 -->|"全 syscall エントリ"| E3["buildSyscallData<br>（フィルタリングなし）"]
        E3 --> E4[("detected_syscalls<br>全エントリ記録")]
    end

    subgraph MacOパス
        M1[("Mach-O バイナリ")] --> M2["ScanSyscallInfos<br>（svc #0x80 スキャン）"]
        M1 --> M3["GetSyscallInfos<br>（libSystem シンボルマッチ）"]
        M2 -->|"svc エントリ"| M4["mergeMachoSyscallInfos"]
        M3 -->|"libSystem エントリ"| M4
        M4 -->|"マージ済み全エントリ"| M5["buildMachoSyscallData<br>（フィルタリングなし）"]
        M5 -->|"未解決 svc のみ"| M6{"Number == -1 AND<br>DeterminationMethod ==<br>direct_svc_0x80?"}
        M6 -->|"あり"| M7["AnalysisWarnings<br>警告セット"]
        M6 -->|"なし"| M8["AnalysisWarnings<br>空"]
        M5 --> M9[("detected_syscalls<br>全エントリ記録")]
    end

    class E1,E4,M1,M9 data;
    class E2,M2,M3,M4 process;
    class E3,M5,M6,M7,M8 changed;
```

### 3.3 `runner` パスのリスク判定フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    R[("JSON レコード<br>detected_syscalls")] --> S1["syscallAnalysisHasSVCSignal"]
    S1 -->|"Number==-1 AND<br>DeterminationMethod==direct_svc_0x80<br>のエントリが存在？"| S2{{"存在する"}}
    S2 -->|"Yes"| HR["isHighRisk = true"]
    S2 -->|"No"| S3["syscallAnalysisHasNetworkSignal"]
    S3 -->|"IsNetwork==true<br>のエントリが存在？<br>（DeterminationMethod 不問）"| S4{{"存在する"}}
    S4 -->|"Yes"| NW["isNetwork = true"]
    S4 -->|"No"| OK["isHighRisk=false, isNetwork=false"]

    class R data;
    class S1,S3 changed;
    class HR,NW,OK process;
```

## 4. macOS syscall テーブル自動生成設計

### 4.1 ファイル役割分担

| ファイル | 変更種別 | 内容 |
|---------|---------|------|
| `internal/libccache/macos_syscall_numbers.go` | 新規（自動生成） | `macOSSyscallEntries` マップ変数（全 BSD syscall） |
| `internal/libccache/macos_syscall_table.go` | 修正 | `macOSSyscallEntries` 定義を削除。`MacOSSyscallTable` 構造体・メソッド・`networkSyscallWrapperNames` は残す |

### 4.2 生成スクリプトの拡張

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef changed fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    H1[("macOS SDK<br>sys/syscall.h")] -->|"--macos-header"| S["generate_syscall_table.py<br>（拡張）"]
    H2[("Linux x86_64<br>unistd_64.h")] -->|"--x86-header"| S
    H3[("Linux arm64<br>unistd.h")] -->|"--arm64-header"| S
    S -->|"SYS_<name> パース"| G1[("macos_syscall_numbers.go")]
    S -->|"__NR_<name> パース"| G2[("x86_syscall_numbers.go")]
    S -->|"__NR_<name> パース"| G3[("arm64_syscall_numbers.go")]

    class H1,H2,H3,G1,G2,G3 data;
    class S changed;
```

### 4.3 macOS ネットワーク syscall セット

Linux の `NETWORK_SYSCALL_NAMES` から macOS で存在しない名前（`accept4`・`recvmmsg`・`sendmmsg`）を除いたセットを `MACOS_NETWORK_SYSCALL_NAMES` として定義する。

### 4.4 Makefile の更新方針

- `MACOS_SYSCALL_HEADER` 変数を追加（デフォルト: `xcrun --show-sdk-path` 経由で取得）
- `SYSCALL_TABLE_OUTPUTS` に `internal/libccache/macos_syscall_numbers.go` を追加
- macOS SDK ヘッダーが存在しない環境（Linux CI 等）では macOS テーブル生成をスキップし、コミット済みファイルをそのまま使用する

## 5. テスト戦略

### 5.1 影響を受けるテストファイル

| テストファイル | 変更内容 |
|-------------|---------|
| `internal/fileanalysis/syscall_store_test.go` | `FilterSyscallsForStorage` 前提テスト削除または置換 |
| `internal/filevalidator/validator_test.go` | 非ネットワーク・解決済み syscall が保持されるよう更新 |
| `internal/filevalidator/validator_macho_test.go` | 解決済み非ネットワーク svc 保持・未解決 svc のみ警告発出の前提に更新 |
| `internal/runner/security/network_analyzer_test.go` | 未解決 svc のみ high risk・IsNetwork 不問での network signal 判定に更新 |
| `internal/libccache/` テスト | macOS syscall テーブル拡張後の `GetSyscallName`・`IsNetworkSyscall` 動作検証 |

### 5.2 テスト方針

- 各 AC（受け入れ基準）に対して最低 1 つのテストを用意する
- 境界値: 解決済み svc（Number != -1）は高リスク判定しない、未解決 svc（Number == -1）は高リスク判定する
- 後方互換性: 旧レコード（フィルタリング済み）でも runner が正しく動作することを確認する

### 5.3 受け入れ基準と確認方法

| 観点 | 確認方法 |
|---|---|
| AC-1, AC-2, AC-3 | `validator_test.go` / `validator_macho_test.go` の単体テストで `DetectedSyscalls` と `AnalysisWarnings` を確認する |
| AC-4, AC-5 | `network_analyzer_test.go` で未解決 svc と解決済みネットワーク svc の判定分岐を確認する |
| AC-6 | `internal/libccache` 配下のテストと `make generate-syscall-tables` で生成結果と API 挙動を確認する |
| AC-7 | `make test` / `make lint` を実行して既存テストの回帰がないことを確認する |
| AC-8 | `docs/tasks/0104_macho_syscall_number_analysis/03_detailed_specification.md` と `04_implementation_plan.md` の superseded 記述をレビューして整合を確認する |
| NFR-1 | `network_analyzer_test.go` で旧レコード相当のフィルタ済み `DetectedSyscalls` を与え、判定が変わらないことを確認する |

## 6. 変更対象外

- `SymbolAnalysis`（`AnalyzeNetworkSymbols`）のフィルタリングロジック
- JSON スキーマバージョン（`CurrentSchemaVersion`）
- `mergeMachoSyscallInfos` の並べ替えロジック
- `internal/libccache/macos_syscall_table.go` の `MacOSSyscallTable` 構造体・メソッド・`networkSyscallWrapperNames`
