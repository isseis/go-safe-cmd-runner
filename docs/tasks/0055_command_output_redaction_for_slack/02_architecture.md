# アーキテクチャ設計書：コマンド出力の Slack 通知における機密情報 Redaction

## 1. エグゼクティブサマリー

### 1.1 問題

現在、コマンドの標準出力に含まれる機密情報（パスワード、API トークン、秘密鍵など）が redact されずに Slack に送信されている。これは、`redaction.RedactingHandler` が `slog.KindAny` 型の属性を処理しないため、`[]common.CommandResult` スライス内の出力が redaction をバイパスしてしまうことが原因である。

### 1.2 解決策

**二重の防御層アプローチ**：

1. **主対策（案1）**：`RedactingHandler` を拡張し、`slog.KindAny` 型と `LogValuer` インターフェースを処理できるようにする
2. **補完策（案2）**：`CommandResult` 作成時に stdout/stderr を予め redact する

これにより、**Defense in Depth**（多層防御）の原則に従い、一方の対策が失敗しても他方が保護する。

### 1.3 期待される効果

- すべてのログ出力（Slack、ファイル、コンソール）で一貫した機密情報保護
- 将来追加される構造化ログ型にも自動的に redaction を適用
- コーディングミスによる機密情報漏洩のリスクを最小化

---

## 2. 現状のアーキテクチャ

### 2.1 システム概要

```mermaid
flowchart TB
    subgraph "Command Execution"
        A[Executor] -->|stdout/stderr| B[GroupExecutor]
        B --> C[CommandResult]
    end

    subgraph "Logging Pipeline"
        C -->|slog.Any| D[slog]
        D --> E[RedactingHandler]
        E --> F[MultiHandler]
    end

    subgraph "Output Handlers"
        F --> G[SlackHandler]
        F --> H[FileHandler]
        F --> I[ConsoleHandler]
    end

    G -->|POST| J[(Slack)]
    H -->|write| K[(Log File)]
    I -->|write| L[(Console)]

    style C fill:#ffcccc
    style E fill:#ffcccc
    style G fill:#ffcccc
    style J fill:#ff6666

    classDef vulnerable fill:#ffcccc,stroke:#ff0000,stroke-width:2px
    classDef risk fill:#ff6666,stroke:#ff0000,stroke-width:3px
```

**凡例**：
- 🔴 赤色：脆弱な箇所
- 濃い赤：機密情報が漏洩するリスクのある場所

### 2.2 データフロー（現状）

```mermaid
sequenceDiagram
    participant Executor
    participant GroupExecutor
    participant CommandResult
    participant slog
    participant RedactingHandler
    participant SlackHandler
    participant Slack

    Executor->>Executor: コマンド実行
    Executor->>GroupExecutor: stdout/stderr (生データ)

    GroupExecutor->>CommandResult: 作成 (Output, Stderr)
    Note over CommandResult: ❌ 機密情報が含まれる

    CommandResult->>slog: slog.Any([]CommandResult)
    slog->>RedactingHandler: Handle(record)

    Note over RedactingHandler: ❌ slog.KindAny をスキップ
    RedactingHandler->>RedactingHandler: 他の属性のみ redact

    RedactingHandler->>SlackHandler: record (redact されていない)
    SlackHandler->>SlackHandler: extractCommandResults()
    Note over SlackHandler: LogValue() を呼び出して<br/>Output, Stderr を抽出

    SlackHandler->>Slack: POST (機密情報含む)
    Note over Slack: ⚠️ 機密情報が漏洩
```

### 2.3 問題の根本原因

#### 原因1：RedactingHandler の制限

`redaction.RedactingHandler.RedactLogAttribute()` は以下の型のみ処理：

```go
// 現在の実装（簡略化）
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    switch attr.Value.Kind() {
    case slog.KindString:
        // ✅ 処理される
        return redactString(attr)
    case slog.KindGroup:
        // ✅ 再帰的に処理される
        return redactGroup(attr)
    case slog.KindAny:
        // ❌ 処理されない（スルー）
        return attr
    }
}
```

#### 原因2：LogValuer の遅延評価

`[]common.CommandResult` は `slog.KindAny` として渡されるが、各要素の `LogValue()` メソッドは RedactingHandler を通過した後に SlackHandler で呼び出される：

```
CommandResult (LogValuer) → slog.Any → RedactingHandler (スキップ) → SlackHandler (LogValue 呼び出し)
                                                ↓
                                         redaction されない
```

### 2.4 影響範囲

以下のコンポーネントが影響を受ける：

1. **SlackHandler**：機密情報が Slack に送信される（最も深刻）
2. **FileHandler**：機密情報がログファイルに記録される
3. **ConsoleHandler**：機密情報がコンソールに出力される
4. **将来追加される handler**：同様の問題が発生しうる

---

## 3. 提案するアーキテクチャ

### 3.1 システム概要（改善後）

```mermaid
flowchart TB
    subgraph "Command Execution"
        A[Executor] -->|stdout/stderr| B[GroupExecutor]
        B -->|redact| C[Validator]
        C -->|redacted output| D[CommandResult]
    end

    subgraph "Logging Pipeline"
        D -->|slog.Any| E[slog]
        E --> F[RedactingHandler]
        F -->|process KindAny| G{LogValuer?}
        G -->|Yes| H[Resolve LogValue]
        H -->|Recursively redact| F
        G -->|No| I[MultiHandler]
        F --> I
    end

    subgraph "Output Handlers"
        I --> J[SlackHandler]
        I --> K[FileHandler]
        I --> L[ConsoleHandler]
    end

    J -->|POST| M[(Slack)]
    K -->|write| N[(Log File)]
    L -->|write| O[(Console)]

    style C fill:#ccffcc
    style F fill:#ccffcc
    style G fill:#ccffcc
    style H fill:#ccffcc
    style M fill:#66ff66

    classDef safe fill:#ccffcc,stroke:#00ff00,stroke-width:2px
    classDef protected fill:#66ff66,stroke:#00ff00,stroke-width:3px
```

**凡例**：
- 🟢 緑色：redaction が適用される箇所
- 濃い緑：保護された出力先

### 3.2 データフロー（改善後）

```mermaid
sequenceDiagram
    participant Executor
    participant GroupExecutor
    participant Validator
    participant CommandResult
    participant slog
    participant RedactingHandler
    participant SlackHandler
    participant Slack

    Executor->>Executor: コマンド実行
    Executor->>GroupExecutor: stdout/stderr (生データ)

    rect rgb(200, 255, 200)
        Note over GroupExecutor,Validator: 🔒 補完策（案2）
        GroupExecutor->>Validator: SanitizeOutputForLogging(stdout)
        Validator->>GroupExecutor: redacted stdout
        GroupExecutor->>Validator: SanitizeOutputForLogging(stderr)
        Validator->>GroupExecutor: redacted stderr
    end

    GroupExecutor->>CommandResult: 作成 (redacted Output, Stderr)
    Note over CommandResult: ✅ 既に redact 済み

    CommandResult->>slog: slog.Any([]CommandResult)
    slog->>RedactingHandler: Handle(record)

    rect rgb(200, 255, 200)
        Note over RedactingHandler: 🔒 主対策（案1）
        RedactingHandler->>RedactingHandler: detect slog.KindAny
        RedactingHandler->>RedactingHandler: check LogValuer interface
        RedactingHandler->>RedactingHandler: call LogValue()
        RedactingHandler->>RedactingHandler: recursively redact
    end

    RedactingHandler->>SlackHandler: record (二重に保護済み)
    SlackHandler->>SlackHandler: extractCommandResults()
    Note over SlackHandler: Output, Stderr を抽出

    SlackHandler->>Slack: POST (機密情報なし)
    Note over Slack: ✅ 安全
```

### 3.3 二重防御層の動作

```mermaid
flowchart LR
    A[生の出力] --> B{第1層<br/>作成時 redact}
    B -->|成功| C[Redacted]
    B -->|失敗/忘れ| D[生データ]

    C --> E{第2層<br/>Handler redact}
    D --> E

    E -->|処理| F[Redacted]

    F --> G[安全な出力]

    style B fill:#ccffcc
    style E fill:#ccffcc
    style F fill:#66ff66
    style G fill:#00ff00
```

**仕組み**：
1. **第1層（案2）**：CommandResult 作成時に redact
   - 成功：機密情報が早期に除去される
   - 失敗/忘れ：生データが残る
2. **第2層（案1）**：RedactingHandler で redact
   - 第1層で漏れた機密情報もここでキャッチ
   - すべてのログ出力で自動的に適用
3. **結果**：どちらか一方でも成功すれば安全

---

## 4. 案1：RedactingHandler の拡張

### 4.1 概要

`redaction.RedactingHandler` を拡張し、`slog.KindAny` 型の属性を処理できるようにする。

### 4.2 設計の要点

#### 4.2.1 処理対象の型

1. **LogValuer インターフェース実装型**：
   - `common.CommandResult`
   - `audit.ExecutionResult`（将来的に必要に応じて）
   - その他、`slog.LogValuer` を実装する型

2. **スライス型**：
   - `[]common.CommandResult`
   - `[]slog.LogValuer`（汎用的に対応）

3. **対応しない型**：
   - プリミティブ型（int, bool など）は既存の処理に委ねる
   - 構造体（LogValuer 未実装）はスキップ

#### 4.2.2 処理フロー

```mermaid
flowchart TD
    A[RedactLogAttribute] --> B{"attr.Value.Kind()"}

    B -->|KindString| C[Redact string]
    B -->|KindGroup| D[Redact group recursively]
    B -->|KindAny| E{Check type}
    B -->|Others| F[Return as-is]

    E -->|LogValuer?| G[Call LogValue]
    E -->|Slice?| H[Process each element]
    E -->|Others| F

    G --> I[Recursively redact resolved value]
    H --> J{Element is LogValuer?}

    J -->|Yes| K[Call LogValue on element]
    J -->|No| L[Skip element]

    K --> M[Recursively redact element]

    I --> N{Depth < MAX?}
    M --> N

    N -.->|Yes<br/>⟲ Recursive call| A
    N -->|No| O[Stop recursion<br/>Return partially redacted]

    C --> P[Return redacted attr]
    D --> P
    O --> P
    F --> P
    L --> P

    style E fill:#ffffcc
    style G fill:#ccffcc
    style H fill:#ccffcc
    style I fill:#ccffcc
    style M fill:#ccffcc
    style N fill:#ffeecc
    style A fill:#eef
```

**注記**：
- 図中の破線矢印「Depth < MAX? → RedactLogAttribute」は、再帰深度チェック後に `RedactLogAttribute` を再度呼び出す概念的な再帰を示しています
- 実装では `redactLogAttributeWithContext(attr, context)` のような内部ヘルパー関数を使用し、深度情報を `RedactionContext` で管理します
- 公開 API の `RedactLogAttribute(attr)` は内部で `redactLogAttributeWithContext(attr, RedactionContext{depth: 0})` を呼び出す設計とします

#### 4.2.3 型定義

```go
// 最大再帰深度（無限再帰防止）
const maxRedactionDepth = 10

// RedactLogAttribute の拡張版の疑似コード（型定義のみ）
type RedactionContext struct {
    depth int  // 現在の再帰深度
}
```

### 4.3 アルゴリズム

#### 4.3.1 LogValuer の処理

```
入力: slog.Attr (value.Kind() == slog.KindAny)
出力: redacted slog.Attr

1. value.Any() で値を取得
2. 型アサーションで LogValuer インターフェースをチェック
3. LogValuer の場合:
   a. LogValue() を呼び出して slog.Value を取得
   b. 新しい属性を作成: slog.Attr{Key: key, Value: resolvedValue}
   c. RedactLogAttribute() を再帰的に呼び出し
   d. 再帰深度をチェック（maxRedactionDepth）
4. LogValuer でない場合:
   a. そのまま返す
```

#### 4.3.2 スライスの処理

```
入力: slog.Attr (value.Any() がスライス型)
出力: redacted slog.Attr

1. スライスの型を判定
2. 各要素に対して:
   a. LogValuer インターフェースをチェック
   b. LogValuer の場合:
      i.  LogValue() を呼び出し
      ii. RedactLogAttribute() を再帰的に呼び出し
   c. LogValuer でない場合:
      i. スキップ
3. 処理済みの要素を新しいスライスに格納
4. slog.Any(key, newSlice) で新しい属性を作成して返す
   (注: スライス型を維持し、オブジェクト/マップに変換しない)
```

### 4.4 実装上の考慮事項

#### 4.4.1 パフォーマンス

**最適化戦略**：
1. **早期リターン**：LogValuer でない型は即座にスキップ
2. **型キャッシュ**：頻繁に使用される型の判定結果をキャッシュ（将来的な最適化）
3. **再帰深度制限**：無限再帰を防ぎ、スタックオーバーフローを回避

**型判定のパフォーマンス分析**：
- 現状の実装では、KindAny 属性ごとに型判定（type assertion）やリフレクションが
  毎回発生するため、ログ出力頻度が高い場合や複雑な型が多い場合、10% 以内の
  オーバーヘッド目標を達成できない可能性がある。
- Go のリフレクションは比較的高コストであり、特にスライス要素数が多い場合は
  影響が大きくなる。
- そのため、初期実装段階から型判定結果のキャッシュ（型キャッシュ）を導入するか、
  少なくともベンチマーク・プロファイリングを早期に実施し、必要に応じて
  キャッシュを追加することを推奨する。
**パフォーマンス目標**：
- 通常のログ出力（KindAny でない）：オーバーヘッドなし
- KindAny を含むログ出力：10% 以内のオーバーヘッド

#### 4.4.2 エラーハンドリング

```mermaid
flowchart TD
    A[Processing KindAny] --> B{LogValue panic?}
    B -->|Yes| C[Recover from panic]
    B -->|No| D[Continue processing]

    C --> E[Log error]
    E --> F["Return attr with [REDACTION FAILED - OUTPUT SUPPRESSED]"]

    D --> G{Type assertion error?}
    G -->|Yes| H[Skip element]
    G -->|No| I[Process normally]

    H --> J[Continue with next element]
    I --> J
    F --> K[Maintain availability and security]
    J --> K

    style C fill:#ffcccc
    style E fill:#ffcccc
    style F fill:#ff6666
    style H fill:#ffffcc
```

**原則**：
- panic は recover して可用性を維持
- エラーは Slack 以外の出力先（stderr とファイルログ）に記録
  - ログレベル: `slog.Warn`（デバッグと監査のため）
  - 記録内容: エラーの種類、属性キー、スタックトレース（panic の場合）
  - 注記: RedactingHandler 自身の出力は再帰を避けるため、非 RedactingHandler 経路を使用
- 処理に失敗した場合は "[REDACTION FAILED - OUTPUT SUPPRESSED]" で置換（fail-secure）

**再帰深度制限の扱い**：
- 再帰深度が maxRedactionDepth (10) に達した場合:
  - エラーとは扱わない（DoS 攻撃防止が目的のため）
  - **それまでに処理した部分的に redact された値を返す**
  - `slog.Debug` レベルでログ記録（監視・チューニング用）
- 完全に redact されない可能性があるが、DoS 攻撃を防ぐことを優先
- 通常のデータ構造では depth=10 に達することは稀

#### 4.4.3 セキュリティ

**考慮事項**：
1. **DoS 攻撃への対策**：
   - 再帰深度制限により、悪意のある深いネスト構造への対策
   - 処理時間の制限は設けない（ログ出力は同期的）

2. **情報漏洩の防止**：
   - LogValuer の実装に依存するため、適切な実装を保証する必要がある
   - CommandResult.LogValue() は既に適切に実装されている

3. **型安全性**：
   - 型アサーションは常にチェック
   - 予期しない型は安全にスキップ

**重要な実装上の注意**：
- 現在の `redaction.Config.RedactText()` 実装は、正規表現コンパイル失敗時に元のテキストを返す fail-open 動作となっている（`redactor.go` の 110-112, 136-139, 174-177 行）
- この動作は本アーキテクチャで定義した fail-secure 原則と矛盾しており、セキュリティリスクとなる
- 実装時には、正規表現コンパイル失敗時も "[REDACTION FAILED - OUTPUT SUPPRESSED]" を返す fail-secure 動作に修正する必要がある

**移行計画（Migration Plan）**：
1. 影響範囲の調査：`RedactText()` を利用している全ての箇所をリストアップし、fail-open から fail-secure への変更が既存機能や運用に与える影響を評価する。
2. テストケースの追加：正規表現コンパイル失敗時に "[REDACTION FAILED - OUTPUT SUPPRESSED]" が返ることを確認するテストを追加する。
3. 段階的なロールアウト：まず開発環境・ステージング環境で動作確認を行い、問題がなければ本番環境へ適用する。
4. ドキュメント更新：開発者向けガイドラインや API ドキュメントに新しいエラーハンドリング仕様を明記する。
5. 監視・フィードバック：本番適用後、異常なログ出力や運用上の問題が発生していないか監視し、必要に応じて調整する。

**実装上の注意**：
- 既存の `RedactText()` の動作変更は他の箇所に影響する可能性があるため、慎重な影響範囲調査とテストが必要
- 変更後は、正規表現コンパイル失敗時に必ず "[REDACTION FAILED - OUTPUT SUPPRESSED]" を返すこと
- 影響を受ける箇所の開発者に周知徹底すること

---

## 5. 案2：CommandResult 作成時の Redaction

### 5.1 概要

`CommandResult` を作成する際に、stdout/stderr を予め redact する。

### 5.2 設計の要点

#### 5.2.1 処理箇所

`internal/runner/group_executor.go` の `executeAllCommands()` メソッド：

```go
// 現在（簡略化）
cmdResult := common.CommandResult{
    CommandResultFields: common.CommandResultFields{
        Name:     cmdSpec.Name,
        ExitCode: exitCode,
        Output:   stdout,   // ❌ 生データ
        Stderr:   stderr,   // ❌ 生データ
    },
}

// 改善後（疑似コード）
cmdResult := common.CommandResult{
    CommandResultFields: common.CommandResultFields{
        Name:     cmdSpec.Name,
        ExitCode: exitCode,
        Output:   ge.validator.SanitizeOutputForLogging(stdout),  // ✅ redact
        Stderr:   ge.validator.SanitizeOutputForLogging(stderr),  // ✅ redact
    },
}
```

#### 5.2.2 処理フロー

```mermaid
flowchart LR
    A[Executor] -->|stdout, stderr| B[executeAllCommands]
    B --> C[SanitizeOutputForLogging]
    C --> D{Redaction enabled?}
    D -->|Yes| E[Apply redaction patterns]
    D -->|No| F[Return as-is]
    E --> G[Redacted output]
    F --> G
    G --> H[Create CommandResult]
    H --> I[Store in results]

    style C fill:#ccffcc
    style E fill:#ccffcc
    style G fill:#66ff66
```

### 5.3 既存インフラの活用

#### 5.3.1 security.Validator

既に `internal/runner/security/logging_security.go` に実装済み：

```go
// SanitizeOutputForLogging sanitizes command output for safe logging
func (v *Validator) SanitizeOutputForLogging(output string) string {
    if output == "" {
        return ""
    }

    // Redact sensitive information if enabled
    if v.config.LoggingOptions.RedactSensitiveInfo {
        output = v.redactSensitivePatterns(output)
    }

    // Truncate stdout if configured
    if v.config.LoggingOptions.TruncateStdout &&
       v.config.LoggingOptions.MaxStdoutLength > 0 &&
       len(output) > v.config.LoggingOptions.MaxStdoutLength {
        output = output[:v.config.LoggingOptions.MaxStdoutLength] +
                 "...[truncated for security]"
    }

    return output
}
```

**機能**：
- 機密情報パターンの redaction
- 出力の長さ制限（オプション）
- 設定に基づく動作の切り替え

#### 5.3.2 redaction.Config

`security.Validator` 内部で使用される `redaction.Config`：

```
redaction.Config
├── Placeholder: "[REDACTED]"
├── Patterns: SensitivePatterns
│   ├── Credential patterns (password, token, secret, etc.)
│   └── Environment variable patterns
└── KeyValuePatterns
    ├── "password", "token", "secret"
    ├── "Bearer ", "Basic "
    └── "Authorization: "
```

**プレースホルダーの使い分け**:

| プレースホルダー | 値 | 使用場面 | 例 |
|---------------|-----|---------|-----|
| `Placeholder` | `"[REDACTED]"` | 正常な redaction（すべての用途） | `password=[REDACTED]` |
| エラー用プレースホルダー | `"[REDACTION FAILED - OUTPUT SUPPRESSED]"` | redaction 処理自体の失敗時（fail-secure） | 処理エラー時の全体置換 |

**重要な区別**:
- **正常な redaction**: 機密情報パターンが検出され、正常に置換された場合 → `Placeholder` (`"[REDACTED]"`)
- **redaction の失敗**: redaction 処理中に panic や予期しないエラーが発生した場合 → `"[REDACTION FAILED - OUTPUT SUPPRESSED]"`
  - 失敗時は機密情報が含まれる可能性があるため、出力全体を安全なプレースホルダーで置換する（FR-2.3 fail-secure 原則)
  - この動作は「セクション 4.4.2 エラーハンドリング」および「セクション 5.4.2 エラーハンドリング」で詳述

**設計上の簡素化**:
- 以前の設計では `LogPlaceholder` (`"***"`) と `TextPlaceholder` (`"[REDACTED]"`) の2種類を使い分けていたが、実用上の利点がないため `Placeholder` に統一
- `"[REDACTED]"` の利点:
  - より明示的で視認性が高い
  - ログ検索が容易（すべての redaction を一括検索可能）
  - 設定と実装がシンプルになる

### 5.4 実装上の考慮事項

#### 5.4.1 GroupExecutor への Validator の注入

現在、`DefaultGroupExecutor` は `security.ValidatorInterface` を持っているが、`SanitizeOutputForLogging()` メソッドはインターフェースに含まれていない可能性がある。

**対応オプション**：

**オプション A：インターフェースを拡張してメソッドを追加**（推奨）
```go
type ValidatorInterface interface {
    // 既存のメソッド...
    SanitizeOutputForLogging(output string) string
}
```

**オプション B：具体型として扱う（型アサーション）**
```go
// GroupExecutor 内で型アサーション
if validator, ok := ge.validator.(*security.Validator); ok {
    stdout = validator.SanitizeOutputForLogging(stdout)
    stderr = validator.SanitizeOutputForLogging(stderr)
}
```

**推奨理由（オプション A）**：
1. **疎結合の維持**：インターフェースを通じて依存することで、具体的な実装に依存しない設計を維持
2. **テスト容易性**：モックの実装が容易になり、単体テストで `SanitizeOutputForLogging` の動作を検証しやすい
3. **型安全性**：コンパイル時に型チェックが行われ、メソッドの存在が保証される
4. **保守性**：将来的に別の Validator 実装を差し替える際も、インターフェースに従っていれば安全に置き換え可能
5. **設計の一貫性**：既存のコードベースがインターフェースベースの設計を採用しているため、この方針に従う

**注記**：オプション B は実装が簡単ですが、型アサーションが失敗した場合の処理が必要であり、テストが複雑になります。詳細は「セクション 7.2.1 ValidatorInterface の拡張」も参照してください。

#### 5.4.2 エラーハンドリング

```mermaid
flowchart TD
    A[SanitizeOutputForLogging] --> B{Redaction error?}
    B -->|Yes| C[Log warning]
    B -->|No| D[Return redacted output]

    C --> E["Return [REDACTION FAILED - OUTPUT SUPPRESSED]"]

    D --> F[Safe output]
    E --> F

    F --> G[Continue execution]

    style C fill:#ffffcc
    style E fill:#ff6666
    style F fill:#66ff66
```

**原則**：
- redaction が失敗しても、エラーを返さない
- セキュリティを優先し、失敗時は "[REDACTION FAILED - OUTPUT SUPPRESSED]" で置換（fail-secure）
- 失敗は stderr とファイルログに記録（FR-2.4：Slack 以外の出力先）
  - ログレベル: `slog.Warn`（デバッグと監査のため）
  - 記録内容: エラーの詳細、コマンド名、失敗した出力の種類（stdout/stderr）

#### 5.4.3 パフォーマンス

**最適化**：
- redaction は CommandResult 作成時の1回だけ
- ログ処理のオーバーヘッドなし
- 正規表現はコンパイル済みのものを使用（`redaction.Config` 内で保持）

---

## 6. 統合アーキテクチャ

### 6.1 全体フロー

```mermaid
flowchart TB
    subgraph "Command Execution Layer"
        A[Executor.Execute] -->|Result| B[GroupExecutor]
    end

    subgraph "Business Logic Layer"
        B -->|stdout, stderr| C{Apply Case 2}
        C -->|Sanitize| D[Validator]
        D -->|Redacted| E[CommandResult]
        C -->|Original| E
    end

    subgraph "Logging Infrastructure Layer"
        E -->|slog.Any| F[slog]
        F --> G[RedactingHandler]
    end

    subgraph "RedactingHandler Processing"
        G --> H{Apply Case 1}
        H -->|Detect KindAny| I[Process LogValuer]
        I -->|Resolve| J[Recursively Redact]
        J --> K[Redacted Attributes]
        H -->|Other types| K
    end

    subgraph "Output Layer"
        K --> L[MultiHandler]
        L --> M[SlackHandler]
        L --> N[FileHandler]
        L --> O[ConsoleHandler]
    end

    M --> P[(Slack)]
    N --> Q[(Log Files)]
    O --> R[(Console)]

    style C fill:#ccffcc
    style D fill:#ccffcc
    style H fill:#ccffcc
    style I fill:#ccffcc
    style J fill:#ccffcc
    style P fill:#66ff66
    style Q fill:#66ff66
    style R fill:#66ff66

    classDef layer1 fill:#e1f5ff
    classDef layer2 fill:#fff9c4
    classDef layer3 fill:#f3e5f5
    class A,B layer1
    class C,D,E layer2
    class F,G,H,I,J,K layer3
```

### 6.2 レイヤー間の責任分離

| レイヤー | 責任 | Redaction の役割 |
|---------|------|-----------------|
| Command Execution | コマンドの実行、結果の取得 | なし |
| Business Logic | CommandResult の作成、データの整形 | 案2：早期 redaction（補完） |
| Logging Infrastructure | ログレコードの処理、属性の redaction | 案1：最終防衛線 |
| Output | ログの出力先への配信 | なし（既に安全な状態） |

### 6.3 失敗シナリオと対応

#### シナリオ1：案2の実装を忘れた場合

```mermaid
flowchart LR
    A[生データ] -->|案2 未実装| B[CommandResult<br/>機密情報含む]
    B --> C[slog]
    C --> D[RedactingHandler]
    D -->|案1 動作| E[機密情報 redact]
    E --> F[安全な出力]

    style B fill:#ffcccc
    style D fill:#ccffcc
    style F fill:#66ff66
```

**結果**：案1により保護される（✅ 安全）

#### シナリオ2：案1のバグで LogValuer 処理が失敗

```mermaid
flowchart LR
    A[生データ] -->|案2 動作| B[CommandResult<br/>既に redact 済み]
    B --> C[slog]
    C --> D[RedactingHandler]
    D -->|案1 失敗| E[redact 済みデータ<br/>そのまま通過]
    E --> F[安全な出力]

    style B fill:#ccffcc
    style D fill:#ffcccc
    style F fill:#66ff66
```

**結果**：案2により保護される（✅ 安全）

#### シナリオ3：両方失敗（最悪ケース）

```mermaid
flowchart LR
    A[生データ] -->|案2 失敗| B[CommandResult<br/>機密情報含む]
    B --> C[slog]
    C --> D[RedactingHandler]
    D -->|案1 失敗| E[機密情報含む]
    E --> F[漏洩]

    style B fill:#ffcccc
    style D fill:#ffcccc
    style E fill:#ff6666
    style F fill:#ff0000
```

**結果**：漏洩（❌ 危険）

**対策**：
- 十分なテストカバレッジ（90% 以上）
- エンドツーエンドのセキュリティテスト
- コードレビューでの確認

---

## 7. インターフェース設計

### 7.1 RedactingHandler の拡張

#### 7.1.1 既存のインターフェース

```go
// slog.Handler interface (変更なし)
type Handler interface {
    Enabled(ctx context.Context, level Level) bool
    Handle(ctx context.Context, r Record) error
    WithAttrs(attrs []Attr) Handler
    WithGroup(name string) Handler
}
```

#### 7.1.2 内部メソッドの拡張

```go
// Config.RedactLogAttribute の拡張（型定義のみ）
type RedactionContext struct {
    depth int
}

// 疑似コード：詳細は詳細設計書で定義
func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr
func (c *Config) redactLogAttributeWithContext(attr slog.Attr, ctx RedactionContext) slog.Attr
func (c *Config) processLogValuer(key string, value slog.Value, ctx RedactionContext) (slog.Attr, error)
func (c *Config) processSlice(key string, value slog.Value, ctx RedactionContext) (slog.Attr, error)
```

### 7.2 GroupExecutor の変更

#### 7.2.1 ValidatorInterface の拡張

現在の `security.ValidatorInterface` に `SanitizeOutputForLogging` メソッドが含まれているか確認が必要。含まれていない場合、インターフェースを拡張する必要があります。

**推奨アプローチ（オプション A）**：

```go
type ValidatorInterface interface {
    // 既存のメソッド...
    SanitizeOutputForLogging(output string) string
}
```

このアプローチの詳細な理由と代替案については、**セクション 5.4.1「GroupExecutor への Validator の注入」**を参照してください。そこでは、以下の点について詳しく説明しています：

- オプション A（インターフェース拡張）の推奨理由
- オプション B（型アサーション）との比較
- 疎結合の維持とテスト容易性の観点からの評価

**実装の影響範囲**：
- `internal/runner/security/validator.go`：インターフェース定義の更新
- `internal/runner/group_executor.go`：インターフェース経由でメソッドを呼び出す
- テストコード：モックの実装を更新

---

## 8. 設定とカスタマイズ

### 8.1 Redaction の有効化/無効化

既存の `security.Config` を使用：

```go
type LoggingOptions struct {
    RedactSensitiveInfo    bool  // Redaction の有効/無効
    IncludeErrorDetails    bool  // エラー詳細の出力
    TruncateStdout         bool  // 出力の切り詰め
    MaxStdoutLength        int   // 最大出力長
    MaxErrorMessageLength  int   // 最大エラーメッセージ長
}
```

**デフォルト**：
- `RedactSensitiveInfo`: `true`（有効）
- その他：プロジェクトの既定値に従う

### 8.2 Redaction パターンのカスタマイズ

既存の `redaction.Config` を使用：

```go
type Config struct {
    Placeholder      string              // プレースホルダー（"[REDACTED]"）
    Patterns         *SensitivePatterns  // 機密情報パターン
    KeyValuePatterns []string            // key=value パターン
}
```

**拡張性**：
- 将来的にカスタムパターンの追加をサポート可能
- 現時点では `DefaultSensitivePatterns()` を使用

### 8.3 再帰深度の設定

```go
// 定数として定義（詳細設計書で決定）
const maxRedactionDepth = 10
```

**カスタマイズ**：
- 現時点では固定値
- 将来的に設定可能にする余地を残す

---

## 9. テスト戦略

### 9.1 テストレベル

```mermaid
flowchart TB
    A[Unit Tests] --> B[Integration Tests]
    B --> C[E2E Tests]

    A -->|RedactingHandler| A1[KindAny processing]
    A -->|RedactingHandler| A2[LogValuer resolution]
    A -->|RedactingHandler| A3[Slice processing]
    A -->|RedactingHandler| A4[Recursion depth]
    A -->|GroupExecutor| A5[CommandResult creation]

    B -->|Full pipeline| B1[slog → Handler → Output]
    B -->|Validator| B2[SanitizeOutputForLogging]

    C -->|Real scenario| C1[Command execution → Slack]
    C -->|Security| C2[Known patterns redacted]

    style A fill:#e1f5ff
    style B fill:#fff9c4
    style C fill:#f3e5f5
```

### 9.2 テストシナリオ

#### 9.2.1 Unit Tests（RedactingHandler）

| テストケース | 入力 | 期待される出力 | 目的 |
|------------|------|--------------|------|
| LogValuer single | CommandResult with password | Redacted CommandResult | LogValuer 処理の検証 |
| LogValuer slice | []CommandResult | Redacted slice | スライス処理の検証 |
| Non-LogValuer | Any other type | Pass through | 未対応型のスキップ |
| Deep recursion | Nested structures (depth=11) | Stop at depth 10, return partially redacted | 再帰深度制限の検証（エラー扱いしない） |
| Panic handling | LogValue() panics | Recover and log | パニックからの復旧 |
| Nil LogValue | LogValue() returns nil or empty | Handle gracefully | nil 値の堅牢性検証 |
| Empty slice | []CommandResult (empty) | Pass through empty | 空スライスの処理 |
| Mixed slice | Slice with LogValuer + non-LogValuer | Process only LogValuer elements | 混在型スライスの処理 |
| FR-2.4: Failure logging | LogValue() panics | slog.Warn to stderr/file (not Slack) | 失敗ログが Slack 以外に記録されることを検証 |

#### 9.2.2 Integration Tests

| テストケース | シナリオ | 検証項目 |
|------------|---------|---------|
| Full pipeline | Command → Log → Handler | エンドツーエンドの redaction |
| Multi-handler | Same log to Slack + File + Console | 一貫した redaction |
| Validator integration | SanitizeOutputForLogging in GroupExecutor | 案2の動作確認 |

#### 9.2.3 E2E Tests

| テストケース | シナリオ | 検証項目 |
|------------|---------|---------|
| Real command output | Execute command with API key in output | Slack に送信されるデータを検証 |
| Known patterns | Test all default sensitive patterns | パターンマッチングの網羅性 |
| Performance | Large output (10MB) | パフォーマンス劣化の確認 |

### 9.3 テストデータ

**機密情報パターンの例**：

```
password=secretpass123
api_key=abc123xyz789
Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9
Authorization: Basic dXNlcjpwYXNz
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
token=ghp_1234567890abcdefghijklmnopqrstuvwxyz
```

**期待される出力**：

```
password=[REDACTED]
api_key=[REDACTED]
Bearer [REDACTED]
Authorization: Basic [REDACTED]
AWS_SECRET_ACCESS_KEY=[REDACTED]
token=[REDACTED]
```

---

## 10. パフォーマンス考慮事項

### 10.1 パフォーマンス目標

| メトリクス | 目標 | 測定方法 |
|----------|------|---------|
| RedactingHandler のオーバーヘッド | 10% 以内 | ベンチマークテスト |
| ログ出力レイテンシ | 100ms 以内 | 統合テスト |
| メモリ使用量増加 | 5% 以内 | プロファイリング |
| CPU 使用率増加 | 5% 以内 | プロファイリング |

### 10.2 最適化戦略

#### 10.2.1 型チェックの最適化

```mermaid
flowchart LR
    A[Attribute] --> B{Fast path?}
    B -->|KindString| C[String redaction]
    B -->|KindGroup| D[Group redaction]
    B -->|KindAny| E[Type check]
    B -->|Others| F[Pass through]

    E --> G{LogValuer?}
    G -->|Yes| H[Process]
    G -->|No| F

    style B fill:#ccffcc
    style G fill:#ffffcc
```

**最適化**：
- 最も頻繁な型（KindString, KindGroup）は早期にチェック
- KindAny は最後にチェック（頻度が低い）
- 型アサーションはキャッシュ可能（将来的な最適化）

#### 10.2.2 正規表現の最適化

既に `redaction.Config` 内でコンパイル済みの正規表現を保持：

```go
type SensitivePatterns struct {
    combinedCredentialPattern *regexp.Regexp  // 事前コンパイル済み
    combinedEnvVarPattern     *regexp.Regexp  // 事前コンパイル済み
}
```

**利点**：
- パターンマッチングのたびにコンパイルしない
- 複数のパターンを1つの正規表現に結合

### 10.3 ベンチマーク

**測定項目**：

```go
// 疑似コード
func BenchmarkRedactingHandler_String(b *testing.B) { /* ... */ }
func BenchmarkRedactingHandler_Group(b *testing.B) { /* ... */ }
func BenchmarkRedactingHandler_LogValuer(b *testing.B) { /* ... */ }
func BenchmarkRedactingHandler_Slice(b *testing.B) { /* ... */ }
```

**目標**：
- String/Group：既存と同等（オーバーヘッドなし）
- LogValuer/Slice：既存の String 処理の 2倍以内

---

## 11. セキュリティ考慮事項

### 11.1 脅威モデル

#### 脅威1：機密情報の漏洩（現在の問題）

**攻撃シナリオ**：
1. コマンド出力に API キーが含まれる
2. Redaction が適用されない
3. Slack チャネルに投稿される
4. チャネルメンバー全員が閲覧可能

**対策**：
- 案1 + 案2 による二重の防御層

#### 脅威2：Redaction のバイパス

**攻撃シナリオ**：
1. 攻撃者が意図的に redaction をバイパスしようとする
2. 特殊な文字エンコーディングやフォーマットを使用

**対策**：
- 正規表現パターンは大文字小文字を区別しない（`(?i)`）
- key=value パターンは柔軟にマッチング
- 将来的にはより高度なパターンマッチングを検討

#### 脅威3：DoS 攻撃

**攻撃シナリオ**：
1. 攻撃者が深くネストした構造を送信
2. RedactingHandler が無限再帰に陥る
3. スタックオーバーフローまたはメモリ枯渇

**対策**：
- 再帰深度制限（maxRedactionDepth = 10）
  - 深度制限到達時は部分的に redact された値を返す（エラー扱いしない）
  - DoS 攻撃を防ぎつつ、可用性を維持
  - 深度制限到達は `slog.Debug` で記録
- panic からの recover
  - 予期しない panic は `slog.Warn` で記録し、"[REDACTION FAILED - OUTPUT SUPPRESSED]" を返す

### 11.2 セキュリティテスト

#### 11.2.1 既知のパターンのテスト

すべての既知の機密情報パターンが redact されることを検証：

```
✅ password=xxx
✅ token=xxx
✅ api_key=xxx
✅ Bearer xxx
✅ Basic xxx
✅ Authorization: xxx
✅ AWS keys
✅ GCP keys
✅ GitHub tokens
```

#### 11.2.2 エッジケースのテスト

```
✅ Mixed case (PaSsWoRd=xxx)
✅ Special characters (password="secret@123")
✅ Multiline output
✅ Empty values
✅ Very long values (> 10KB)
```

### 11.3 監査とロギング

**redaction の監査**：
- Redaction が適用された場合、debug レベルでログ出力（オプション）
- Redaction に失敗した場合、warning レベルでログ出力
- パフォーマンスへの影響を最小限にするため、デフォルトでは無効

**FR-2.4：失敗時のログ記録要件**：
- Redaction の失敗は、**Slack 以外の出力先**に記録すること（必須）
  - 出力先: stderr、ファイルログ、監査ログ
  - 目的: デバッグ、インシデント分析、セキュリティ監査
- ログ記録の実装方法:
  - RedactingHandler: 内部エラーログを非 RedactingHandler 経路で出力
  - GroupExecutor: `slog.Warn()` で失敗を記録（通常のログパイプライン経由）
  - 注意: RedactingHandler のエラーログは再帰を避けるため、MultiHandler の構成に注意が必要

---

## 12. 運用考慮事項

### 12.1 デプロイメント

#### 12.1.1 段階的ロールアウト

```mermaid
flowchart LR
    A[Phase 1:<br/>案2のみ] -->|検証| B[Phase 2:<br/>案1追加]
    B -->|検証| C[Phase 3:<br/>本番環境]

    style A fill:#fff9c4
    style B fill:#ccffcc
    style C fill:#66ff66
```

**Phase 1：案2（CommandResult 作成時の redaction）**

実装内容：
- GroupExecutor で CommandResult 作成時に `SanitizeOutputForLogging()` を呼び出す
- ValidatorInterface の拡張（必要に応じて）

検証基準：
- ✅ 単体テスト：`SanitizeOutputForLogging()` が正しく呼び出されることを確認
- ✅ 統合テスト：以下の機密情報パターンが Slack に送信されないことを確認
  - **安全なテスト方法**：
    - テスト専用の Slack チャネルまたはワークスペースを使用
    - モックした SlackHandler を使用し、実際の HTTP リクエストをキャプチャして検証
    - 本番用の Slack Webhook URL を絶対に使用しない
  - **検証パターン**：
    - `password=xxx` → `password=[REDACTED]`
    - `Bearer xxx` → `Bearer [REDACTED]`
    - `api_key=xxx` → `api_key=[REDACTED]`
  - **検証方法**：
    - モックエンドポイント経由でペイロードを検査
    - テスト用 Slack チャネルに送信された実際のメッセージを確認
- ✅ パフォーマンス：既存のベンチマークテストがすべてパス
- ✅ 既存機能：すべての既存テストがパス（回帰なし）

完了判断：上記の検証基準がすべて満たされ、コードレビューが完了

**Phase 2：案1（RedactingHandler の拡張）**

実装内容：
- `RedactLogAttribute()` を拡張し、`slog.KindAny` を処理
- LogValuer インターフェースの解決
- スライス型の処理

検証基準：
- ✅ 単体テスト：以下のケースで redaction が動作することを確認
  - LogValuer 型の redaction
  - スライス型の redaction
  - 再帰深度制限（depth=10）
  - nil 値の処理
  - panic からの復旧
- ✅ 統合テスト：Phase 1 で redaction を「無効化」しても、Phase 2 の機能だけで機密情報が保護されることを確認
  - **安全なテスト環境**：Phase 1 同様、テスト専用 Slack チャネルまたはモックエンドポイントを使用
- ✅ パフォーマンス：
  - RedactingHandler のオーバーヘッドが 10% 以内
  - ログ出力レイテンシが 100ms 以内
- ✅ E2E テスト：実際のコマンド実行→Slack 送信フローで機密情報が redact されることを確認
  - **重要**：本番 Slack チャネルではなく、必ずテスト専用環境で実施

完了判断：上記の検証基準がすべて満たされ、パフォーマンス目標を達成

**Phase 3：本番環境へのデプロイ**

デプロイ準備：
- すべてのテストがパス（カバレッジ 90% 以上）
- パフォーマンスが許容範囲内
- ドキュメントが完成（詳細設計書、実装計画書、運用手順書）
- ロールバック計画の策定

検証基準（本番環境）：
- ✅ デプロイ前の最終確認：
  - 本番 Slack Webhook URL が設定されていることを確認
  - テスト用 Webhook URL が削除されていることを確認
  - Redaction が有効化されていることを確認（`RedactSensitiveInfo: true`）
- ✅ 初期監視期間（24時間）：以下のメトリクスが正常範囲内
  - Redaction エラー率 < 0.1%
  - ログ出力レイテンシ < 100ms
  - CPU 使用率増加 < 5%
  - メモリ使用量増加 < 5%
- ✅ セキュリティ検証：Slack チャネルのログを自動スキャンと限定的な目視確認で検証
  - **検証プロセス**：
    1. **自動スキャン（最優先）**：
       - Slack API を使用して対象期間（過去1週間）のメッセージをエクスポート
       - 検証スクリプトで以下をチェック：
         - `[REDACTED]` プレースホルダーの存在確認
         - `[REDACTION FAILED - OUTPUT SUPPRESSED]` の出現回数（あれば調査）
         - 機密情報パターンのスキャン（例：`password=`, `Bearer `, `api_key=`, AWS キー形式など）
       - スキャン結果をレポートとして出力
    2. **限定的な目視確認（自動スキャンで問題検出時のみ）**：
       - 自動スキャンで疑わしいメッセージが検出された場合のみ実施
       - 検出されたメッセージのみを目視で精査（全体の目視確認は行わない）
    3. **アクセス制限と監査**：
       - 検証作業は適切な権限とセキュリティクリアランスを持つ担当者のみが実施
       - 検証作業の開始・終了・担当者を監査ログに記録
  - **検証スクリプト要件**：
    - 言語：Python または Go
    - 機能：
      - Slack API 経由でのメッセージエクスポート（`conversations.history` API）
      - 正規表現ベースの機密情報パターンマッチング
      - レポート生成（検出結果、統計情報、問題のあるメッセージID）
    - セキュリティ：
      - エクスポートしたメッセージは暗号化してローカルに一時保存
      - 検証完了後は自動的に削除
      - スクリプト実行ログを監査証跡として保存
  - **重要な注意事項**：
    - 本番 Slack チャネルの実データを扱うため、検証担当者は適切な権限とセキュリティクリアランスを持つこと
    - 自動スキャンの結果、問題が検出されなければ目視確認は不要
    - 検出された問題があれば、速やかに関係者に報告し、必要に応じて緊急対応を実施
- ✅ 安定性確認（1週間）：上記メトリクスが継続的に正常範囲内

完了判断：1週間の安定稼働を確認し、問題が発生しないことを確認

#### 12.1.2 ロールバック計画

```mermaid
flowchart TD
    A[デプロイ] --> B{問題発生?}
    B -->|No| C[継続運用]
    B -->|Yes| D{重大度?}

    D -->|High| E[即座にロールバック]
    D -->|Medium| F[案1を無効化]
    D -->|Low| G[修正して再デプロイ]

    E --> H[旧バージョンに戻す]
    F --> I[案2のみで運用]

    style E fill:#ffcccc
    style H fill:#ff6666
```

### 12.2 監視とアラート

#### 12.2.1 メトリクス

| メトリクス | 正常範囲 | アラート条件 |
|----------|---------|------------|
| Redaction エラー率 | < 0.1% | > 1% |
| ログ出力レイテンシ | < 100ms | > 200ms |
| CPU 使用率増加 | < 5% | > 10% |
| メモリ使用量増加 | < 5% | > 10% |

#### 12.2.2 ログ出力

```go
// Redaction の成功をログ出力（debug レベル、オプション）
slog.Debug("Redaction applied",
    "attribute_key", key,
    "value_type", valueType,
    "redacted", true)

// 再帰深度制限到達（debug レベル、監視・チューニング用）
slog.Debug("Recursion depth limit reached - returning partially redacted value",
    "attribute_key", key,
    "depth", maxRedactionDepth,
    "note", "This is not an error - DoS prevention measure")

// Redaction の失敗（warning レベル、FR-2.4 必須）
// 注意: このログは Slack 以外（stderr、ファイル、監査ログ）に出力される
slog.Warn("Redaction failed - using safe placeholder",
    "attribute_key", key,
    "error", err,
    "placeholder", "[REDACTION FAILED - OUTPUT SUPPRESSED]",
    "output_destination", "stderr, file, audit")
```

### 12.3 トラブルシューティング

#### 12.3.1 一般的な問題

| 問題 | 原因 | 対処法 |
|-----|------|--------|
| Redaction が適用されない | 設定の誤り | `RedactSensitiveInfo` を確認 |
| パフォーマンス低下 | 大量のログ出力 | ログレベルを調整 |
| panic | LogValue() の実装バグ | recover により継続、該当の LogValuer を修正 |

#### 12.3.2 デバッグ手順

1. **ログレベルを DEBUG に変更**：
   ```
   LOG_LEVEL=debug ./runner ...
   ```

2. **Redaction の動作を確認**：
   ```
   grep "Redaction" /var/log/runner/*.json
   ```

3. **パフォーマンスプロファイリング**：
   ```
   go tool pprof http://localhost:6060/debug/pprof/profile
   ```

---

## 13. 今後の拡張

### 13.1 短期的な拡張（3-6ヶ月）

1. **カスタム redaction パターンのサポート**：
   - ユーザーが独自のパターンを追加できる
   - 設定ファイル（TOML）でパターンを定義

2. **Redaction の監査ログ**：
   - どの機密情報が redact されたかを記録
   - セキュリティ監査に使用

3. **パフォーマンスの最適化**：
   - 型チェックのキャッシュ
   - より効率的な正規表現パターン

### 13.2 中期的な拡張（6-12ヶ月）

1. **機械学習ベースの機密情報検出**：
   - パターンマッチングだけでなく、ML モデルを使用
   - より高度な検出精度

2. **リアルタイムストリーミング出力の redaction**：
   - 現在は出力完了後に redaction
   - ストリーミング中に redaction を適用

3. **他の通知システムへの対応**：
   - Email, PagerDuty, など
   - 統一的な redaction 機能の提供

### 13.3 長期的な拡張（12ヶ月以上）

1. **プラガブル redaction アーキテクチャ**：
   - カスタム redactor の実装をサポート
   - 組織固有のポリシーに対応

2. **コンプライアンス対応**：
   - GDPR, HIPAA など
   - 監査証跡の自動生成

---

## 14. 結論

### 14.1 アーキテクチャの利点

1. **Defense in Depth**：二重の防御層により、高い安全性を確保
2. **包括的な保護**：すべてのログ出力で一貫した redaction
3. **将来性**：新しい構造化ログ型にも自動的に対応
4. **保守性**：既存のアーキテクチャとの整合性、明確な責任分離

### 14.2 トレードオフ

| 項目 | トレードオフ | 選択 |
|-----|-----------|------|
| パフォーマンス vs セキュリティ | 若干のオーバーヘッド vs 機密情報保護 | セキュリティを優先 |
| シンプル vs 包括性 | 案2のみ vs 案1+案2 | 包括性を優先 |
| 早期リリース vs 完全性 | 段階的実装 vs 一括実装 | 段階的実装を選択 |

### 14.3 次のステップ

1. **詳細設計書の作成**：
   - 具体的な実装方法
   - コードレベルの設計
   - テストケースの詳細

2. **実装計画書の作成**：
   - マイルストーンの詳細
   - タスクの分解
   - リソースの割り当て

3. **プロトタイプの実装**：
   - 案1の基本実装
   - パフォーマンステスト
   - フィードバックの収集
