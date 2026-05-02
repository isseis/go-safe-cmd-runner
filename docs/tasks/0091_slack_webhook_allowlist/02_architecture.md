# アーキテクチャ設計書: Slack webhook URL ホスト allowlist

## 1. システム概要

### 1.1 アーキテクチャ目標

- SSRF・情報漏洩リスクの排除: 環境変数が改ざんされても任意ホストへの送信を防止する
- TOML によるポリシー管理: 許可ホストはハッシュ検証済み TOML で管理し、改ざんを検出可能にする
- 起動フローの整合性: TOML 読み込み後に Slack ハンドラを初期化し、許可ホストの確実な適用を保証する
- 既存検証の維持: 既存の HTTPS スキーム・ホスト名存在チェックを除去しない

### 1.2 設計原則

- **セキュリティファースト**: TOML 由来の許可ホストで URL を検証してから Slack ハンドラを生成する
- **最小変更**: 既存の起動フローの骨格を維持しつつ Slack 初期化のみを後段に移動する
- **明示的設定**: デフォルト許可ホストを持たず、利用者が TOML に明示した場合のみ Slack 通知を有効化する
- **単一ホスト**: 成功・エラー両 URL は同一ホストを使用することを前提とし、許可ホストは単一文字列で管理する

---

## 2. 起動フローの変更

### 2.1 現状の起動フロー (問題あり)

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    E[("環境変数<br>GSCR_SLACK_WEBHOOK_URL_*")] --> A
    A["ValidateSlackWebhookEnv()"] --> B["SetupLogging()<br>(Slack ハンドラ含む)"]
    B --> C["LoadAndPrepareConfig()"]
    C --> D[("TOML<br>slack_allowed_host")]

    class E data;
    class D data;
    class A,C process;
    class B problem;
```

> 問題: Slack ハンドラ生成 (B) が TOML 読み込み (C→D) より前に実行されるため、許可ホストを参照できない。

### 2.2 変更後の起動フロー

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    E[("環境変数<br>GSCR_SLACK_WEBHOOK_URL_*")] --> A
    A["ValidateSlackWebhookEnv()"] --> B

    subgraph Phase1["Phase 1: TOML 読み込み前"]
        B["SetupLogging()<br>(コンソール・ファイルのみ)"]
    end

    B --> C["LoadAndPrepareConfig()"]
    C --> D[("TOML<br>slack_allowed_host")]
    D --> F

    subgraph Phase2["Phase 2: TOML 読み込み後"]
        F["SetupSlackLogging()<br>(ホスト検証 + Slack ハンドラ追加)"]
    end

    class E,D data;
    class A,B,C process;
    class F enhanced;
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

---

## 3. コンポーネント構成

### 3.1 変更対象パッケージ

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph "cmd/runner"
        MAIN["main.go<br>起動フロー変更<br>(Phase 1/2 分割)"]
    end

    subgraph "internal/runner/runnertypes"
        SPEC["spec.go<br>GlobalSpec.SlackAllowedHost 追加"]
    end

    subgraph "internal/runner/bootstrap"
        ENV["environment.go<br>SetupLoggingOptions.SlackAllowedHost 追加<br>SetupSlackLogging() 新規追加"]
        LOG["logger.go<br>SlackLoggerConfig 新規追加 (AllowedHost を保持)<br>Slack ハンドラ生成を AddSlackHandlers に移動"]
    end

    subgraph "internal/logging"
        SH["slack_handler.go<br>SlackHandlerOptions.AllowedHost 追加<br>validateWebhookURL シグネチャ変更"]
    end

    MAIN --> ENV
    MAIN --> SPEC
    ENV --> LOG
    LOG --> SH

    class MAIN,SPEC,ENV,LOG,SH enhanced;
```

### 3.2 許可ホスト伝播経路

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;

    A[("TOML<br>global.slack_allowed_host")] -->|"GlobalSpec<br>.SlackAllowedHost"| B["LoadAndPrepareConfig()"]
    B -->|"SetupLoggingOptions<br>.SlackAllowedHost"| C["SetupSlackLogging()"]
    C -->|"SlackLoggerConfig<br>.AllowedHost"| D["AddSlackHandlers()"]
    D -->|"SlackHandlerOptions<br>.AllowedHost"| E["NewSlackHandler()"]
    E -->|"allowedHost"| F["validateWebhookURL()"]

    class A data;
    class B,C,D,E,F process;
```

---

## 4. 詳細設計

### 4.1 Phase 1: `SetupLogging` の変更

`SetupLogging` はコンソールハンドラ・ファイルハンドラのみを初期化し、Slack ハンドラを生成しない。`SetupLoggingOptions` および `LoggerConfig` から `SlackWebhookURLSuccess/Error` フィールドを**削除**し、コンパイルレベルで Slack URL を受け付けなくする。

```mermaid
sequenceDiagram
    participant M as main.go
    participant E as environment.go
    participant L as logger.go

    M->>E: SetupLogging(opts)
    Note over E: Slack URL フィールドなし<br>(SetupLoggingOptions から削除済み)
    E->>L: SetupLoggerWithConfig(config)
    L->>L: コンソールハンドラ生成
    L->>L: ファイルハンドラ生成 (LogDir が設定されている場合)
    Note over L: Slack ハンドラは生成しない
    L-->>E: nil (success)
    E-->>M: nil (success)
```

### 4.2 Phase 2: `SetupSlackLogging` の新規追加

TOML 読み込み後に呼び出す新関数。ホスト検証を実施してから Slack ハンドラを既存ロガーに追加する。

```mermaid
sequenceDiagram
    participant M as main.go
    participant E as environment.go
    participant L as logger.go
    participant S as slack_handler.go

    M->>E: SetupSlackLogging(slackConfig, opts)
    Note over E: opts.SlackAllowedHost = cfg.Global.SlackAllowedHost
    E->>L: AddSlackHandlers(config)
    Note over L: config.AllowedHost = opts.SlackAllowedHost

    alt successURL が設定されている場合
        L->>S: NewSlackHandler(SlackHandlerOptions{AllowedHost: ...})
        S->>S: validateWebhookURL(url, allowedHost)
        alt ホスト検証失敗
            S-->>L: ErrInvalidWebhookURL
            L-->>E: error
            E-->>M: PreExecutionError{Type: ErrorTypeConfigParsing}
        else 検証成功
            S-->>L: *SlackHandler
        end
    end

    alt errorURL が設定されている場合
        L->>S: NewSlackHandler(SlackHandlerOptions{AllowedHost: ...})
        S->>S: validateWebhookURL(url, allowedHost)
    end

    L->>L: Slack ハンドラを既存 MultiHandler に追加して再構築
    L->>L: slog.SetDefault(新ロガー)
    L-->>E: nil (success)
    E-->>M: nil (success)
```

### 4.3 `validateWebhookURL` の変更

```mermaid
flowchart TD
    A["validateWebhookURL(url, allowedHost)"] --> B{"url が空?"}
    B -->|"Yes"| ERR1["ErrInvalidWebhookURL<br>(empty URL)"]
    B -->|"No"| C["url.Parse(url)"]
    C --> D{"パース失敗?"}
    D -->|"Yes"| ERR2["ErrInvalidWebhookURL<br>(parse error)"]
    D -->|"No"| E{"scheme != 'https'?"}
    E -->|"Yes"| ERR3["ErrInvalidWebhookURL<br>(not HTTPS)"]
    E -->|"No"| F{"Host が空?"}
    F -->|"Yes"| ERR4["ErrInvalidWebhookURL<br>(empty host)"]
    F -->|"No"| G["hostname = 正規化(url のホスト名)"]
    G --> H{"hostname == 正規化(allowedHost)?"}
    H -->|"No"| ERR5["ErrInvalidWebhookURL<br>(host not allowed)"]
    H -->|"Yes"| OK["nil (success)"]
```

---

## 5. エラーハンドリング設計

### 5.1 エラー分類

| 状況 | エラー型 | `PreExecutionError.Type` |
|------|----------|--------------------------|
| Slack URL なし (`GSCR_SLACK_WEBHOOK_URL_*` 未設定) | — (エラーなし、サイレントに無効) | — |
| SUCCESS のみ設定、ERROR なし | `ErrSuccessWithoutError` | `ErrorTypeConfigParsing` (既存) |
| URL が HTTPS でない | `ErrInvalidWebhookURL` | `ErrorTypeConfigParsing` |
| URL のホストが許可ホストと不一致 | `ErrInvalidWebhookURL` | `ErrorTypeConfigParsing` |
| 許可ホストが未設定 かつ URL が設定されている | `ErrInvalidWebhookURL` | `ErrorTypeConfigParsing` |

### 5.2 エラーメッセージ例

```
Error: invalid webhook URL: host not allowed: evil.example.com (allowed: hooks.slack.com)
```

---

## 6. テスト戦略

### 6.1 テスト階層

```mermaid
flowchart TB
    classDef tier1 fill:#ffb86b,stroke:#333,color:#000;
    classDef tier2 fill:#ffd59a,stroke:#333,color:#000;
    classDef tier3 fill:#c3f08a,stroke:#333,color:#000;

    Tier1["統合テスト<br>起動フロー全体 (AC-L2-20)"]:::tier1
    Tier2["コンポーネントテスト<br>SetupSlackLogging / AddSlackHandlers (AC-L2-19)"]:::tier2
    Tier3["単体テスト<br>validateWebhookURL (AC-L2-13〜18)"]:::tier3

    Tier3 --> Tier2 --> Tier1
```

### 6.2 各 AC とテスト対象の対応

| AC | テスト対象 | パッケージ |
|----|-----------|------------|
| AC-L2-13 | `validateWebhookURL` — 許可ホスト未設定 | `internal/logging` |
| AC-L2-14 | `validateWebhookURL` — ホスト不一致 | `internal/logging` |
| AC-L2-15 | `validateWebhookURL` — ホスト一致 | `internal/logging` |
| AC-L2-16 | `validateWebhookURL` — 大文字/小文字 | `internal/logging` |
| AC-L2-17 | `validateWebhookURL` — ポート番号付き URL | `internal/logging` |
| AC-L2-18 | `validateWebhookURL` — 既存 HTTPS/host チェック | `internal/logging` |
| AC-L2-19 | `SetupSlackLogging` — 許可ホスト伝播 | `internal/runner/bootstrap` |
| AC-L2-20 | 起動フロー — 許可ホスト未設定で起動失敗 | `internal/runner/bootstrap` |
