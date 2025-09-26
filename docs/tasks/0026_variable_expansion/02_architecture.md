# アーキテクチャ設計書: コマンド・引数内環境変数展開機能

## 1. システム概要

### 1.1 アーキテクチャ目標
- 既存システムへの影響最小化
- セキュリティの堅牢性確保
- 実績のある反復制限方式を活用したシンプルな実装
- 直感的な設計による保守性向上

### 1.2 設計原則
- **セキュリティファースト**: allowlist検証を展開前に実施
- **既存活用**: 実績のある反復制限方式の循環参照検出を拡張
- **既存互換性**: 既存設定ファイルの無変更動作
- **直感的実装**: 複雑なDFSではなく理解しやすいアルゴリズム

## 2. システム構成

### 2.1 全体アーキテクチャ

```mermaid
%% Color nodes: data vs process vs enhanced
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    A[(TOML Configuration)] -->|cmd/args| B[Config Parser]
    B --> C[CommandEnvProcessor]
    C --> D[Security Validator]
    D --> E[Command Executor]

    F[(Runtime Environment)] -->|system env vars| K[Environment Variables Processor]
    A -->|local env vars| K
    K --> L[Environment Variables Filter]
    L -.->|allowlist check| C

    %% Assign classes: data vs process vs enhanced nodes
    class A,F data;
    class B,D,E,L process;
    class C,K enhanced;

```

<!-- Legend for colored nodes -->
**凡例（Legend）**

以下の図は、ダイアグラムで使用している形状と色の意味を示します。円柱形（青系）はデータ（configuration / environment）、長方形（オレンジ系）は既存処理（プロセス）、長方形（緑系）は拡張された処理を表します。

```mermaid
%% Legend: data (blue) vs process (orange) vs enhanced (green)
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[(Configuration Data)] --> P1[Existing Component] --> E1[Enhanced Component]
    class D1 data
    class P1 process
    class E1 enhanced
```

### 2.2 コンポーネント配置

```mermaid
%% Component graph with data/process coloring
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    subgraph "内部パッケージ構成"

        subgraph "internal/runner/config"
            E[command.go<br>Command構造体拡張]
        end

        subgraph "internal/runner/environment"
            F["processor.go<br>CommandEnvProcessor (拡張)"]
            L[filter.go<br>既存環境変数フィルタ]
        end

        subgraph "internal/runner/security"
            C[validator.go<br>既存セキュリティ検証]
        end

        subgraph "internal/runner/executor"
            G[executor.go<br.コマンド実行]
        end
    end

    E --> F
    F --> L
    F --> G

    %% Assign classes: treat files that are primarily data as data, others as process
    class E data;
    class F,L,C,G process;

```

### 2.3 データフロー

```mermaid
sequenceDiagram
    participant CF as Config File
    participant CP as Config Parser
    participant VE as CommandEnvProcessor
    participant SV as Security Validator
    participant CE as Command Executor

    CF->>CP: Load TOML config
    CP->>VE: Parse cmd/args with variables
    VE->>VE: Extract variable references
    VE->>SV: Validate against allowlist
    SV-->>VE: Validation result
    alt Validation success
        VE->>VE: Expand variables
        VE->>SV: Validate expanded cmd path
        SV-->>VE: Path validation result
        VE->>CE: Execute with expanded values
    else Validation failure
        VE->>CE: Return error
    end
```

## 3. 詳細設計

### 3.1 コンポーネント設計方針

#### 3.1.1 既存実装を活用したシンプルなアプローチ
**visited mapによる循環参照検出方式を採用**:
- `internal/runner/environment/processor.go` の `resolveVariableReferencesForCommandEnv`
- 既存の反復制限方式から visited map による循環参照検出に変更し、上限を撤廃

#### 3.1.2 CommandEnvProcessor の直接使用
**責務**: 環境変数展開の統合制御

**主要メソッド**:
```go
func (p *CommandEnvProcessor) Expand(
    value string,
    envVars map[string]string,
    group *runnertypes.CommandGroup,
    visited map[string]bool,
) (string, error)
```

**主要機能**:
- `${VAR}` 形式の変数展開
- 循環参照検出 (visited map による)
- セキュリティ検証との統合
- エスケープシーケンス処理


### 3.2 既存コンポーネントとの統合

#### 3.2.1 CommandEnvProcessor の機能拡張
**機能拡張点**:
- `${VAR}` 形式の一貫したサポート
- エスケープシーケンス処理 (`\$`, `\\`)
- 循環参照検出 (visited map による再帰的チェック)

**統合機能**:
- Command.Env での `${VAR}` 形式
- cmd/args での `${VAR}` 形式サポート
- 統一されたエラーハンドリング

#### 3.2.2 Security Validator 連携
**既存機能をそのまま活用**:
- `ValidateAllEnvironmentVars` - allowlist 検証
- Command.Env 優先ポリシーの継続
- 既存のセキュリティポリシーを保持

#### 3.2.3 Config Parser 統合
**シンプルな統合方式**:
- Command 構造体処理時に展開処理を挿入
- 既存の処理フローへの影響を最小限に抑制
- エラーハンドリングの一貫性を維持

## 4. セキュリティアーキテクチャ

### 4.1 セキュリティ処理フロー

```mermaid
flowchart TD
    A[Variable Reference Detection] --> B[Environment Filter Processing]
    B --> C{In Command.Env?}
    C -->|Yes| D[Mark as Allowed - Skip Allowlist Check]
    C -->|No| E{In Allowlist?}
    E -->|Yes| F[Mark as Allowed]
    E -->|No| G[Security Error]
    D --> H[Variable Expansion]
    F --> H
    H --> I[Command Path Validation]
    I --> J{Path Valid?}
    J -->|No| K[Path Error]
    J -->|Yes| L[Execute Command]
```

### 4.2 セキュリティレイヤー

1. **入力検証レイヤー**: 変数参照の形式チェック
2. **認可レイヤー**: allowlist / Command.Env検証
3. **展開レイヤー**: 安全な変数展開処理
4. **実行前検証レイヤー**: 展開後の最終検証

### 4.3 攻撃ベクター対策

| 攻撃タイプ | 対策 |
|-----------|------|
| 権限昇格 | allowlist強制、Command.Env優先 |
| 情報漏洩 | 変数アクセス監査、ログマスキング |
| インジェクション | シェル実行禁止、特殊文字エスケープなし |
| 循環参照DoS | 循環参照検出、最大深度制限 |
| パス展開攻撃 | グロブパターンをリテラル扱い、エスケープ機能でリテラル文字列制御 |

## 5. パフォーマンス設計

### 5.1 性能最適化戦略

- **visited mapによる効率的な循環参照検出**: O(1)時間でのアクセスと循環検出
- **シンプルな実装**: 複雑なDFSアルゴリズムを回避し、直感的なvisited map方式を採用
- **最小限のメモリ使用**: visited map + 効率的な文字列操作でメモリ使用量を抑制

### 5.2 パフォーマンス監視ポイント

```go
type ExpansionMetrics struct {
    TotalExpansions   int64
    ExpansionDuration time.Duration
    VariableCount     int
    CacheHitRatio     float64
    ErrorCount        int64
}
```

### 5.3 スケーラビリティ考慮

- **引数数制限**: 最大1000個の引数
- **変数数制限**: 要素あたり最大50個の変数
- **展開深度**: 制限なし（visited mapによる循環参照検出で安全性を保証）
- **メモリ使用量**: visited map + 文字列処理でO(n)のメモリ使用量を維持

## 6. エラーハンドリング設計

### 6.1 エラー階層

```go
type ExpansionError struct {
    Type    ErrorType
    Message string
    Context ErrorContext
}

type ErrorType int
const (
    VariableNotFound ErrorType = iota
    CircularReference
    SecurityViolation
    SyntaxError
    PathValidationError
)
```

### 6.2 エラー処理フロー

```mermaid
flowchart TD
    A[Error Occurs] --> B{Error Type}
    B -->|Security| C[Log Security Event]
    B -->|Syntax| D[Log Syntax Error]
    B -->|Circular Ref| E[Log Circular Reference]
    B -->|Not Found| F[Log Variable Error]

    C --> G[Abort Group Execution]
    D --> G
    E --> G
    F --> G

    G --> H[Continue Other Groups]
```

## 7. 監視・運用設計

### 7.1 ログ出力設計

**デバッグレベル**:
- 変数参照の検出結果
- allowlist検証の詳細
- 展開前後の値比較

**エラーレベル**:
- セキュリティ違反
- 循環参照検出
- パス検証失敗

**統計レベル**:
- 処理時間統計
- 変数展開回数
- エラー発生率

### 7.2 運用監視項目

| メトリクス | 説明 | 閾値例 |
|----------|------|-------|
| 展開処理時間 | 1要素あたりの処理時間 | >1ms |
| エラー率 | 展開失敗の割合 | >5% |
| メモリ使用量 | 展開処理時のメモリ | >2倍 |
| セキュリティ違反 | allowlist違反回数 | >0 |

## 8. テスト戦略

### 8.1 テスト階層

```mermaid
flowchart TB
    classDef tier1 fill:#ffb86b,stroke:#333,color:#000;
    classDef tier2 fill:#ffd59a,stroke:#333,color:#000;
    classDef tier3 fill:#c3f08a,stroke:#333,color:#000;

    Tier1["統合テスト"]:::tier1
    Tier2["コンポーネントテスト"]:::tier2
    Tier3["単体テスト"]:::tier3

    Tier3 --> Tier2 --> Tier1
```

### 8.2 テストカテゴリ

**単体テスト**:
- Variable Parser: 全パターンの変数形式
- Security Validator: allowlist検証ロジック
- Circular Reference Detector: 循環参照パターン

**コンポーネントテスト**:
- Variable Expander: 統合展開処理
- エラーハンドリング: 異常系パターン

**統合テスト**:
- 実際のコマンド実行
- 既存機能との互換性
- パフォーマンステスト

### 8.3 テストデータ設計

**正常系**:
- 基本的な変数展開
- 複数変数展開
- ネスト変数展開

**異常系**:
- 存在しない変数参照
- 循環参照
- allowlist違反
- 不正な変数形式

## 9. 段階的実装計画

### 9.1 Phase 1: 既存コードの拡張
- [ ] Environment Processor での `${VAR}` 形式一貫性確保
- [ ] 反復制限を visited map による循環参照検出に変更
- [ ] 既存テストケースの拡張
- [ ] 基本的な単体テスト

### 9.2 Phase 2: cmd/args 統合
- [ ] 拡張された Variable Expander の実装
- [ ] Config Parser での cmd/args 展開統合
- [ ] セキュリティ検証の統合
- [ ] 統合テストの実装

### 9.3 Phase 3: 最終化
- [ ] パフォーマンス検証と最適化
- [ ] 包括的テストケース
- [ ] ドキュメント更新
- [ ] パフォーマンスベンチマーク

## 10. 依存関係とリスク

### 10.1 外部依存関係
- **Go標準ライブラリ**: `os`, `strings`, `regexp`
- **既存内部パッケージ**: `internal/runner/environment`, `internal/runner/config`
- **新規依存関係**: なし（要件に従い最小限）

### 10.2 アーキテクチャリスク

| リスク | 影響度 | 対策 |
|-------|-------|-----|
| 既存コード影響 | 高 | インターフェース設計、段階的統合 |
| 性能劣化 | 中 | ベンチマーク測定、最適化実装 |
| セキュリティホール | 高 | 多層防御、包括的テスト |
| 複雑性増大 | 中 | 関心の分離、明確な責務分担 |

### 10.3 技術的課題

**変数展開の複雑性**:
- ネスト展開の処理順序
- 循環参照検出のアルゴリズム効率性
- `${VAR}` 形式のみによる明確な変数境界の確保

**セキュリティとパフォーマンスの両立**:
- 検証処理のオーバーヘッド
- メモリ使用量の制御

**既存システムとの統合**:
- 設定ファイル処理フローの変更点
- エラー処理の一貫性確保
