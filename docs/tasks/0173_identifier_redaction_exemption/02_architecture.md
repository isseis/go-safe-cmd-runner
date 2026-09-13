# アーキテクチャ設計書: 識別子の型宣言と値ベース redaction からの免除

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-09-12 |
| Review date | - |
| Reviewer | - |
| Comments | 決定変更（要再承認）: 宣言型 `Identifier` を `internal/common` から leaf パッケージ `internal/identifier` へ移す。`internal/common` をはじめすべての消費者が型の定義パッケージ外になるため、名前を設定する構築（要素を持つ複合リテラル `identifier.Identifier{name: …}` とフィールド代入 `id.name = …`）はコンパイラが拒否する（ゼロ値 `identifier.Identifier{}` は空名として構築できるが、名前は設定できない）。これに伴い、旧設計で guard に追加していた複合リテラル検査・フィールド書き込み検査と対応する AC-19 mutation は削除し、guard は宣言サイト目録（`identifier.NewIdentifier` 呼び出し）と値参照（エイリアス）の拒否に絞る。構築経路をソース走査で列挙する方式はレビュー 3 ラウンドで抜け道（件数を保つ移設・複合リテラル・フィールド代入）が順次見つかったため、コンパイラによる強制へ切り替える（CLAUDE.md「Enforce invariants with the type, not with convention」）。あわせて §5.2 の rollback を、宣言サイトを個別に戻す手順から、その宣言を導入したコミットを revert する手順（同一コミットの目録・固定アサーションごと戻す）へ改めた（§3.4 に同一コミット規約を追加）。 |

本設計書で既存挙動について述べる箇所は、特に断りのない限り commit `88624849`（`docs(0173): Approved the requirements document`）時点のコードで検証した。

## 関連文書

- [01_requirements.md](01_requirements.md) — 本設計が満たす要件と受け入れ基準
- [Task 0172 02_architecture.md](../0172_slack_notification_message_unification/02_architecture.md) — 残余リスク §3.5「表示安全な補間契約」、§5.2「既存の保護との関係」。本タスクはこの残余リスクを置き換える
- [Task 0172 03_implementation_plan.md](../0172_slack_notification_message_unification/03_implementation_plan.md) — 設定検証の決定（commit `8d0667c5`）
- [security-architecture.ja.md](../../dev/architecture_design/security-architecture.ja.md) / [security-architecture.md](../../dev/architecture_design/security-architecture.md) — redaction 層の説明を更新する（AC-15）
- [security-risk-assessment.ja.md](../../user/security-risk-assessment.ja.md) / [security-risk-assessment.md](../../user/security-risk-assessment.md) — Limitations を更新する（AC-16）

## 用語

| 用語 | 意味 |
|---|---|
| 識別子 | group 名またはコマンド名。TOML に人間が書くリテラルであり、変数展開も外部入力も経由しない |
| 値ベース redaction | 文字列属性の値に含まれる機密情報を検出してマスクする処理。実装は key=value 置換（`Config.RedactText` 内の `keyValuePatterns`）、値形式検出（`ValueDetector.Mask`）、値まるごと判定（`IsSensitiveValue`）の 3 層 |
| 免除 | 宣言された識別子に対して値ベース redaction を適用しないこと |
| 宣言型 | 識別子であることを型で示す `identifier.Identifier`。leaf パッケージ `internal/identifier` が定義し、構築は `NewIdentifier` に限られる |
| 宣言サイト | group 名・コマンド名をログ属性値として書く production の各行 |
| Scope | Slack 通知がどの group／command で起きたかを示すフィールド。Task 0172 で導入された |

## 1. 設計の全体像

### 1.1 設計原則

1. 識別子を免除するかどうかは、属性キーでも値の内容でもなく、値の型で宣言する（Declare, don't infer）。production コードが `identifier.Identifier` を構築した値だけを免除する。
2. 免除は値ベース redaction のすべての層に及ぶ。key=value 置換・値形式検出・値まるごと判定のいずれも識別子には適用しない。
3. 自由文（stdout・stderr・コマンド行・引数・環境変数値・message・error 文字列）の redaction は一切弱めない。同じ文字列でも、宣言型でない限り従来どおり redact される。
4. 下流ハンドラには string として正規化して渡す。`RedactingHandler` の免除経路が `identifier.Identifier` を `slog.StringValue` に変換してから転送するため、JSON ハンドラ・text ハンドラ・`SlackHandler`・`message_formatter` の読み取りコードは変更しない。
5. Task 0172 の決定（識別子の中身を設定検証で redaction と照合しない）を維持する。本タスクは表示時に書き換えないことを保証する。
6. 既存の redaction パターン集合（`DefaultSensitivePatterns`・`DefaultKeyValuePatterns`・`ValueDetector`）を変更しない。

### 1.2 概念モデル

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    CFG[("TOML の group 名<br>コマンド名")]
    PROD["identifier.NewIdentifier"]
    FREE["自由文の書き込み"]
    ID["identifier.Identifier"]
    MSG["string 属性"]
    REC[("slog.Record")]
    RH["RedactingHandler"]
    EX["string 正規化"]
    VC["値ベース redaction"]
    OUT["MultiHandler"]

    CFG --> PROD
    PROD --> ID
    FREE --> MSG
    ID --> REC
    MSG --> REC
    REC --> RH
    RH -->|"属性値が identifier.Identifier"| EX
    RH -->|"属性値が identifier.Identifier でない"| VC
    EX --> OUT
    VC --> OUT

    class CFG,REC data
    class FREE,RH,VC,OUT process
    class PROD,EX enhanced
    class ID newpkg
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    L1[("データ")]
    L2["変更しない既存処理"]
    L3["変更・追加する処理"]
    L4["新しく追加する型"]

    class L1 data
    class L2 process
    class L3 enhanced
    class L4 newpkg
```

矢印 A → B は「A が B へデータを渡す、または A を起点として B の処理が始まる」ことを表す。同じ `slog.Record` に識別子と自由文が同居し、`RedactingHandler` は属性ごとに次の順で経路を決める。

1. キー名が機密（`IsSensitiveKey`）に一致する属性は値まるごとマスクする（本図では省略）。本タスクが識別子を載せるキー（`group`・`command`・`command_name`・`name`・`notification_context`）はいずれもこのパターンに一致しないため、識別子にこの処理は適用されない。
2. 属性値が `identifier.Identifier` のときは免除経路（`string 正規化`）へ進み、値に含まれる機密情報をマスクする値ベース redaction を適用しない。
3. それ以外の属性は、値に含まれる機密情報をマスクする値ベース redaction の対象になり、従来どおり redact される。実装は key=value 置換（`Config.RedactText`）、値形式検出（`ValueDetector.Mask`）、値まるごと判定（`SensitivePatterns.IsSensitiveValue`）の 3 層から成る。対象は string だけでなく、再帰の結果として現れる文字列属性を含む。グループ値・`LogValuer` は再帰し、その各文字列属性が同じ 2・3 の判定で振り分けられる。

`identifier.Identifier` を構築しない限り、同じ内容の文字列でも 3 へ進む。これが免除が宣言型だけに掛かることの意味である。`identifier.Identifier` の構築は leaf パッケージ `internal/identifier` の `NewIdentifier` だけが行える（§3.1）。

### 1.3 現在の仕組みと変更後の境界

現在の `RedactingHandler.redactLogAttributeWithContext` は、文字列属性に対して key=value 置換（`Config.RedactText`）を行い、変化が無ければ値まるごと判定（`IsSensitiveValue`）を行う（[`internal/redaction/redactor.go:764`](../../../internal/redaction/redactor.go)、[`internal/redaction/redactor.go:775`](../../../internal/redaction/redactor.go)）。`RedactText` はさらに値形式検出（`ValueDetector.Mask`）を内包する（[`internal/redaction/redactor.go:289`](../../../internal/redaction/redactor.go)）。キー名によるマスク（`IsSensitiveKey`）はこの手前にあり、`group`・`command`・`command_name`・`name`・`notification_context` はいずれも一致しない（[`internal/redaction/sensitive_patterns.go:127`](../../../internal/redaction/sensitive_patterns.go)）。

変更後は、`identifier.Identifier` 型の値が属性に現れたときだけ、値ベース redaction を適用せず string へ正規化して転送する。それ以外の値の扱いは変えない。

#### 既存の単純な案を採らない理由

**キー名による除外は成立しない。** キー `"command"` は、TOML のコマンド名（識別子）と展開済みコマンド行（自由文）の両方に載る。実際に `DefaultExecutor.executeWithUserGroup` は同一関数内で `"command"` に `cmd.Name()` を載せる行（[`internal/runner/base/executor/executor.go:254`](../../../internal/runner/base/executor/executor.go)）と、`cmd.ExpandedCmd` を載せる行（[`internal/runner/base/executor/executor.go:191`](../../../internal/runner/base/executor/executor.go)、[`:198`](../../../internal/runner/base/executor/executor.go)、[`:247`](../../../internal/runner/base/executor/executor.go)、[`:284`](../../../internal/runner/base/executor/executor.go)）を持つ。キー単位の除外は、コマンド名を救うためにコマンド行の redaction を同時に弱めるか、コマンド行を守るためにコマンド名を redact し続けるかのどちらかになり、両立しない。キー `"name"` も group 名（[`internal/runner/group_executor.go:149`](../../../internal/runner/group_executor.go)）、一時ファイル名（[`internal/safefileio/safe_file_linux.go:241`](../../../internal/safefileio/safe_file_linux.go)）、コマンド結果の名前（[`internal/common/logschema.go:120`](../../../internal/common/logschema.go)）という複数の用途で使われる。

**パターン集合の調整では救えない。** `IsSensitiveValue` を語境界付きにすれば `monkey` は救えるが、`AKIA…` 形の値形式一致（`ValueDetector` 経由）と `monkey=` の隣接形（key=value 置換の境界なし代替、[`internal/redaction/redactor.go:491`](../../../internal/redaction/redactor.go)）は救えず、しかも自由文の検出挙動まで変わる。免除は宣言だけに基づかせ、パターン集合は据え置く（AC-12）。

付録 B に、0172 からの経緯を含む決定履歴をまとめる。

## 2. システム構成

### 2.1 全体アーキテクチャ

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    subgraph Producers["宣言サイト（common を import 済み。identifier は新規 import）"]
        RUNNER["internal/runner"]
        RESOURCE["internal/runner/resource"]
        AUDIT["internal/runner/base/audit"]
        VERIFY["internal/verification"]
        LOGSEC["internal/logging"]
        CFGPKG["internal/runner/config"]
    end

    subgraph NewEdges["宣言サイト（identifier を新規 import。common を import しない）"]
        EXECPKG["internal/runner/base/executor"]
        PRIVPKG["internal/runner/base/privilege"]
    end

    COMMON["internal/common<br>notification_context.go<br>logschema.go"]
    IDPKG["internal/identifier"]
    ID["identifier.Identifier"]

    subgraph Redaction["internal/redaction"]
        RH["RedactingHandler"]
        EX["string 正規化"]
        VC["値ベース redaction"]
    end

    RECORD[("slog.Record")]
    DOWN["MultiHandler<br>JSON / text / Slack"]

    RUNNER -.->|"型を利用（新規の辺）"| IDPKG
    RESOURCE -.->|"型を利用（新規の辺）"| IDPKG
    AUDIT -.->|"型を利用（新規の辺）"| IDPKG
    VERIFY -.->|"型を利用（新規の辺）"| IDPKG
    LOGSEC -.->|"型を利用（新規の辺）"| IDPKG
    CFGPKG -.->|"型を利用（新規の辺）"| IDPKG
    EXECPKG -.->|"型を利用（新規の辺）"| IDPKG
    PRIVPKG -.->|"型を利用（新規の辺）"| IDPKG
    COMMON -.->|"型を利用（新規の辺）"| IDPKG
    Redaction -.->|"免除のため型を利用（新規の辺）"| IDPKG
    IDPKG --> ID
    RUNNER --> RECORD
    RESOURCE --> RECORD
    AUDIT --> RECORD
    VERIFY --> RECORD
    LOGSEC --> RECORD
    CFGPKG --> RECORD
    EXECPKG --> RECORD
    PRIVPKG --> RECORD
    ID --> RECORD
    RECORD --> RH
    RH --> EX
    RH --> VC
    EX --> DOWN
    VC --> DOWN

    class RECORD data
    class DOWN process
    class COMMON,RUNNER,RESOURCE,AUDIT,VERIFY,LOGSEC,CFGPKG,EXECPKG,PRIVPKG,Redaction,RH,EX,VC enhanced
    class IDPKG,ID newpkg
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    L1[("データ")]
    L2["変更しない既存処理"]
    L3["変更・追加する処理"]
    L4["新しく追加する型"]

    class L1 data
    class L2 process
    class L3 enhanced
    class L4 newpkg
```

実線の矢印 A → B は「A のデータが B へ流れる」こと、破線の矢印 A ⇢ B は「A が B の型を利用する」というパッケージ依存を表す。

#### 追加されるパッケージ依存辺

本タスクは `internal/identifier` を参照する次の新しい直接依存辺を追加する。`internal/identifier` は内部パッケージを 1 つも import しない leaf パッケージ（新規、production 1 ファイル）なので、逆依存は無く、循環は生じない。

| 新規の辺 | 根拠 |
|---|---|
| `internal/common` → `internal/identifier` | `NotificationContext.LogValue` と `CommandResult(s).LogValue` が宣言型で符号化する。`internal/common` はこれまで `internal/` パッケージを 1 つも import していなかった |
| `internal/redaction` → `internal/identifier` | 免除判定が宣言型を参照する。`internal/redaction` の production コードは現在どの `internal/` パッケージにも依存しない |
| `internal/runner/base/executor` → `internal/identifier` | コマンド名を宣言する。同パッケージの production ファイルは現在 `internal/common` も `internal/identifier` も直接 import しない（直接 import するのはテストと `internal/runner/base/executor/testutil` のみ） |
| `internal/runner/base/privilege` → `internal/identifier` | `command` 属性のコマンド名を宣言する。同パッケージは現在 `internal/common` も `internal/identifier` も import しない |
| `internal/runner`・`internal/runner/resource`・`internal/runner/base/audit`・`internal/runner/config`・`internal/verification`・`internal/logging` → `internal/identifier` | 宣言サイトが `identifier.NewIdentifier` を使う。各パッケージは既に `internal/common` を直接 import しているが、`internal/identifier` へは新しい辺になる |

`internal/redaction` は「宣言型を知らなくても値ベース redaction を適用できる」低レベルな primitive である。`identifier` への依存は primitive の独立性を少し下げるが、免除の判断主体は redaction 側にあり、宣言型は leaf パッケージが持つため、この 1 本は受け入れる。型を leaf パッケージに置くこと自体が、構築経路を `NewIdentifier` にコンパイラで限定するための設計である（旧設計では `internal/common` に置き、guard が構築形を列挙して塞いでいた。§3.1、付録 B）。

### 2.2 コンポーネント配置

| ファイル | 種別 | 責務 | 更新が必要な既存テスト |
|---|---|---|---|
| `internal/identifier/identifier.go` | 新規 | leaf パッケージ。宣言型 `Identifier`、コンストラクタ `NewIdentifier`、参照メソッド `Name`、`String`、`LogValue` を定義する。フィールドは非公開なので、型の定義パッケージ外は名前を設定できず、名前を持つ値を構築できるのは `NewIdentifier` だけである（要素付き複合リテラルとフィールド代入をコンパイラが拒否する） | - |
| `internal/identifier/identifier_test.go` | 新規 | `LogValue` が string を返すこと、`String` が名前を返すこと、ゼロ値と空名の扱いを検証する | - |
| `internal/identifier/identifier_guard_test.go` | 新規（`//go:build test`） | production の `identifier.NewIdentifier` 呼び出し（修飾形と `internal/identifier` 内の非修飾形の両方）が §3.4 の宣言サイト目録（ファイル・関数・囲むログ呼び出し／文・属性キー・引数式・件数）と双方向に一致することを構文木で検証する。目録外の宣言と目録にある宣言の欠落の双方を検出し、`NewIdentifier` の値参照（エイリアス）が 0 件であることも要求する（§7.3） | - |
| `internal/common/notification_context.go` | 変更 | `LogValue` の `group`／`command` を `identifier.Identifier` で符号化する（`internal/identifier` を新規 import する）。`decodeNotificationContextParts` は下位値が `identifier.Identifier` の場合に名前を読む（§3.3） | `internal/common/notification_context_test.go` の符号化期待値 |
| `internal/common/notification_context_test.go` | 変更 | `groupAttr`／`commandAttr` を `identifier.NewIdentifier` で構築し、復号が両形式（生の宣言型と正規化後の string）を受けることを検証する | - |
| `internal/common/logschema.go` | 変更 | `CommandResult.LogValue` と `CommandResults.LogValue` の `name` を `identifier.Identifier` で符号化する（`internal/identifier` を新規 import する） | `internal/common/logschema_test.go` は `Value.String()` 比較のため原則そのまま通る（§7.1） |
| `internal/common/logschema_test.go` | 変更 | 必要なら `identifier.Identifier` を明示する行を足す | - |
| `internal/redaction/redactor.go` | 変更 | `identifier.Identifier` を認識して string へ正規化する免除経路を `RedactLogAttribute`、`redactLogAttributeWithContext`、`processSlice` に追加する（`internal/identifier` を新規 import する） | 免除・対照の新規テスト。既存のパターン集合テストは変更しない |
| `internal/redaction/redactor_test.go` | 変更 | 3 層すべての免除、対照（plain string は redact）、コマンド行維持、`notification_context` 正規化を検証する | - |
| `internal/runner/group_executor.go` | 変更 | `group`／`command` の識別子宣言（§3.4） | `internal/runner/group_executor_test.go`、`group_executor_timeout_test.go` |
| `internal/runner/runner.go` | 変更 | `logGroupExecutionSummary` の `group` 宣言 | グループ集計の属性検査 |
| `internal/runner/config/expansion.go` | 変更 | `resolveAndPrepareCommandSpec` の `command` 宣言 | 該当テスト |
| `internal/runner/base/executor/executor.go` | 変更 | `Execute`・`executeWithUserGroup` のコマンド名 `command` 宣言（展開済みコマンド行は変更しない）。あわせて `internal/identifier` を新規 import する | `executor_logging_test.go` は展開済み行のみで影響なし |
| `internal/runner/base/executor/tempdir_manager.go` | 変更 | `Create` の `group` 宣言 | 該当テスト |
| `internal/runner/base/privilege/unix.go` | 変更 | `WithPrivileges`・`logElevationOutcome` の `command` 宣言。あわせて `internal/identifier` を新規 import する | `unix_privilege_test.go` |
| `internal/runner/base/audit/logger.go` | 変更 | `LogUserGroupExecution` の `command_name`、`LogRiskProfile` の `command_name` を宣言 | `logger_test.go` |
| `internal/runner/resource/normal_manager.go` | 変更 | `ExecuteCommand` の `command` 宣言と、`command_path` に載る group 名の宣言（§3.4） | 該当テスト |
| `internal/runner/resource/dryrun_manager.go` | 変更 | `validateRunAsIdentity`・`evaluateCommandRisk` の `command`／`group` 宣言 | 該当テスト |
| `internal/verification/manager.go` | 変更 | `VerifyGroupFiles`・`collectVerificationFiles` の `group` 宣言 | 該当テスト |
| `internal/logging/security.go` | 変更 | `SecurityLogger` の 4 メソッドの引数を `identifier.Identifier` にする（`internal/identifier` を新規 import する）。`command` の宣言はヘルパー内ではなく呼び出し元で行う | `security_test.go` は 4 メソッドの引数を `identifier.NewIdentifier(…)` で構築するよう更新する（§7.4） |
| `docs/dev/architecture_design/security-architecture.ja.md`・`.md` | 変更 | redaction 層の説明へ識別子免除を追記し、kill switch が無いことと rollback 手順を記す（AC-15、§5.2） | - |
| `docs/user/security-risk-assessment.ja.md`・`.md` | 変更 | Limitations へ AC-14 の帰結を追記する（AC-16） | - |

leaf パッケージ `internal/identifier` を新設し、既存の `internal/common` と `internal/redaction` の責務は維持する。`cmd/` 配下の宣言サイトは無い。

### 2.3 データフロー

```mermaid
sequenceDiagram
    box rgb(232,245,232) 変更・追加する処理
        participant P as 宣言サイト
        participant R as RedactingHandler
    end
    box rgb(255,241,230) 変更しない既存処理
        participant D as JSON/text/Slack ハンドラ
    end

    P->>R: slog レコード（identifier.Identifier 属性）
    R->>R: キー名マスクを判定
    R->>R: 値が Identifier かを型で判定
    alt Identifier
        R->>R: string へ正規化
    else それ以外
        R->>R: 値ベース redaction を適用
    end
    R->>D: 正規化済みレコード
    D->>D: 元の文字列を描画
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    L1["変更・追加する処理"]
    L2["変更しない既存処理"]

    class L1 enhanced
    class L2 process
```

矢印 A → B は「処理の呼び出し、またはデータの受け渡し」を表し、A → A は「A 自身が行う処理」を表す。参加者を囲む色付きボックスの色は上の凡例に対応する。

### 2.4 副作用の境界

本タスクは新しいフラグやモードを追加しない。既存モードの副作用は次のとおりである。

| モード | 外部への書き込み・削除・ネットワーク送信 | ログの内容 |
|---|---|---|
| 通常 | 本タスクでは変更しない | 識別子が redaction で書き換わらなくなる。他の値の redaction は不変 |
| `--dry-run` | 既存のドライラン契約どおり抑止する。本タスクでは変更しない | 通常と同じ redaction 経路を通るため、識別子の扱いも通常と同じ |
| Slack 通知 | 既存の送信機構を変更しない | Scope が元の識別子を表示する。ただし自由文の error に埋め込まれた名前は残余リスクのまま（§5.2） |

免除は redaction の判定にのみ作用し、ファイル操作・コマンド実行・Slack 送信の有無を変えない。免除を止める専用の実行時スイッチは設けない（§5.2 に運用上の扱いを記す）。

## 3. コンポーネント設計

### 3.1 識別子型

leaf パッケージ `internal/identifier` に次の型を置く。フィールドは非公開であり、型の定義パッケージ外は `Identifier` の非公開フィールドに触れられない。したがって、名前を持つ `Identifier` を組み立てられるのは `NewIdentifier` だけである。`internal/common`（`NotificationContext.LogValue`・`CommandResult(s).LogValue`）も `internal/redaction` も同じく定義パッケージ外なので、`identifier.Identifier{name: …}` の要素付き複合リテラルと `id.name = …` のフィールド代入はコンパイラが拒否する（ゼロ値 `identifier.Identifier{}` は構築できるが空名のままで、名前は設定できない）。構築経路をソース走査で列挙する必要は無く、guard は「どの値が宣言されたか」の目録だけを守ればよい（CLAUDE.md「Enforce invariants with the type, not with convention」）。

```go
type Identifier struct {
    // name は TOML に人間が書いた group 名または command 名。
    name string
}

// NewIdentifier は name を識別子として宣言する。
func NewIdentifier(name string) Identifier

// Name は宣言された名前を返す。
func (i Identifier) Name() string

// String は fmt.Stringer として名前を返す。
func (i Identifier) String() string

// LogValue は識別子を string として符号化する。
func (i Identifier) LogValue() slog.Value
```

型を `internal/common` に置くと、同じパッケージの production コード（`notification_context.go`・`logschema.go`）は非公開フィールドを直接書けるため、guard が構築形（複合リテラル・フィールド代入）を 1 つずつ列挙して塞ぐ必要があった。leaf パッケージへ移すとその列挙が不要になり、guard は目録とエイリアス拒否に絞れる（付録 B）。

`Name()` は、テストおよび名前を明示的に取り出す必要がある箇所（§3.3 の `decodeNotificationContextParts` など）で使う。`String()` は `fmt.Stringer` として、`slog.Value.String()` を直接呼ぶ既存コード向けに置く。`notification_context.go:37` の `var _ slog.LogValuer = GlobalScope()` と同じく、`var _ slog.LogValuer = Identifier{}` のコンパイル時ガードを置く。

**`LogValue` を string にする理由。** `Identifier` は `slog.LogValuer` を実装するため、`slog.Any` に渡すと `slog.Value.Kind()` は `KindLogValuer` になる。`RedactingHandler` は `KindLogValuer` という種別そのものを免除するのではなく、値の具象型が `identifier.Identifier` または `*identifier.Identifier` である場合にだけ免除する（§3.2）。任意の `slog.LogValuer` 実装が免除される経路は無い。redaction を通らない解決系ハンドラ（JSON/text）は `LogValue` を解決して string を受け取る。`LogValue` が string を返すことで、JSON ハンドラと text ハンドラの出力は本タスクの前後で変わらない。一方、テスト補助の捕捉ハンドラ `internal/testutil.LogRecorder` は `a.Value.Any()` を保存し `Resolve` しないため、宣言型をそのまま捕捉する。この差は §7.4 のテスト更新で扱う。

**`String` を併せて実装する理由。** `slog.Value.String()` は `KindLogValuer` の値を解決せず `fmt.Append` で整形する（Go 1.26 の `log/slog` `Value.append`）。`Identifier` が `fmt.Stringer` を実装していれば、`attr.Value.String()` を直接呼ぶ既存コード（例: [`internal/logging/slack_handler.go:717`](../../../internal/logging/slack_handler.go) の `extractFromAttrs`、[`internal/common/logschema_test.go:72`](../../../internal/common/logschema_test.go)）は、redaction を通さない経路でも名前を得る。これにより Slack ハンドラと `message_formatter` の読み取りコードを変更しない（AC-06）。

コンストラクタは正規化も検証もしない。名前の検査は設定境界（Task 0172 の `ValidateIdentifiers`）が担い、表示境界は補間契約が担う。`Identifier` は「これは識別子である」という宣言だけを持つ。構築経路は leaf パッケージがコンパイラで `NewIdentifier` に限定するが、コンストラクタが任意の string を受けるため、どの文字列を宣言したかは呼び出しサイトに依存する。呼び出しサイトを §3.4 に固定し、`identifier_guard_test.go`（§7.3）で目録との双方向の不一致を検出する。

### 3.2 免除経路

`internal/redaction` に、`slog.Value` が宣言型かを判定する非公開ヘルパーを 1 つ置く。判定は値の型だけで行い、キー名・値の内容・値の長さは見ない。`Identifier` のメソッドは値レシーバで定義するため `*Identifier` も `slog.LogValuer` を満たし、`slog.Any` にポインタを渡した場合は `KindLogValuer` の具象型が `*identifier.Identifier` として現れる。したがってヘルパーは `identifier.Identifier` と非 nil の `*identifier.Identifier` を宣言型として認識する。3 つの挿入点はいずれもこの 1 つのヘルパーを使うため、非 nil のポインタ値も同じ判定で免除される。一方、型付き nil の `*identifier.Identifier`（例: `slog.Any("command", (*identifier.Identifier)(nil))`）は免除しない。ヘルパーは `Name()` を呼ばずに false を返し、値は通常経路へ進む。`KindLogValuer` として `processLogValuer` に達すると nil ポインタ経由の `LogValue()` がパニックするが、既存のパニック回復が `RedactionFailurePlaceholder`（[`redactor.go:260`](../../../internal/redaction/redactor.go)）を代入するため、ログ呼び出しはプロセスを落とさず fail-closed の表示になる。

免除を挿入する箇所は次の 3 つである。いずれも「値ベースの変換に入る前」に置く。

| 関数 | 位置 | 処理と根拠 |
|---|---|---|
| `Config.RedactLogAttribute` | [`redactor.go:301`](../../../internal/redaction/redactor.go) | キー名判定の後、string／group 判定の前に宣言型を見て string へ正規化する。この関数は production では `redactLogAttributeWithContext` と違い group 再帰でしか呼ばれないが、`slog.Attr` を走査する 2 つ目の公開実装であり、免除を両実装で一致させ、テストで固定する |
| `RedactingHandler.redactLogAttributeWithContext` | [`redactor.go:764`](../../../internal/redaction/redactor.go) | キー名判定の後、`switch value.Kind()` の前に宣言型を見る。これが本番の主要経路であり、group の再帰・`processMap`・`processStruct`・`processKindAny` へ至る前に免除する |
| `RedactingHandler.processSlice` | [`redactor.go:1264`](../../../internal/redaction/redactor.go) | 要素を `slog.LogValuer` として解決する前（[`redactor.go:1330`](../../../internal/redaction/redactor.go) の型アサーションの前）にヘルパーで要素の型を見る。ヘルパーは `identifier.Identifier` と非 nil の `*identifier.Identifier` を認識するため、`[]*Identifier` の非 nil 要素も免除される（型付き nil は免除せず通常経路のパニック回復に委ねる。§3.2）。免除した要素は属性経路と同じく `Name()` の string に正規化して `processedElements` へ append する（生の `Identifier` を append すると `[{}]` 描画になる。後述）。スライス要素に対しては `redactor.go:1368` で `LogValue()` が直接呼ばれ、解決済みの string が `redactor.go:1374` の再帰へ渡るため、先頭判定では宣言型を見られない。ここを塞がないと、`[]Identifier` の要素だけが免除を失う |

`processLogValuer` が解決した値が宣言型の場合（`LogValue` が `Identifier` を返すラッパー）も、再帰先の `redactLogAttributeWithContext` の先頭判定で免除される。`processMap`・`processStruct` は値ごとに `redactLogAttributeWithContext` へ再帰するため追加の挿入点を要しない。

`processSlice` の挿入点は、現時点で `[]identifier.Identifier` を構築する production サイトが無いため防御的である。それでも置くのは、redaction の走査のどの経路でも免除が保たれることを型で保証するためであり、`[]Identifier`・`[]*Identifier` を人工的に用意するテストで固定する。これは将来の `[]Identifier` が黙って redact される事故を防ぐ最小の 1 行である。

免除した要素の正規化先は `Name()` の string（`LogValue()` が返す `slog.StringValue(name)` の `String()` と同じ値）であり、属性経路の `slog.StringValue(name)` と一致させる。`processSlice` は要素を `[]any` に詰めて `slog.AnyValue` で返し、下流の JSON ハンドラは `KindAny` をそのまま `json.Marshal` へ渡すため、生の `Identifier` を append すると非公開フィールドしか持たない構造体が `[{}]` と描画され、§1.1 の「下流ハンドラには string として正規化して渡す」と AC-06 に反する。

キー名判定を先に置くのは、機密キーの下に誤って識別子が載った場合にマスクを優先する fail-closed のためである。免除経路は `slog.Attr{Key: key, Value: slog.StringValue(name)}` を返す。後続のハンドラは常に string を受け取る。

### 3.3 `notification_context` の符号化と復元

`NotificationContext.LogValue`（[`internal/common/notification_context.go:78`](../../../internal/common/notification_context.go)）は、`group` と `command` の下位値を `slog.String` から `slog.Any` + `identifier.NewIdentifier` に変える。

| 下位キー | 現在の型 | 変更後の型 | 出力条件 |
|---|---|---|---|
| `scope` | string | string（変更なし） | 常に |
| `group` | string | `identifier.Identifier` | 常に |
| `command` | string | `identifier.Identifier` | 空でないときだけ |

`decodeNotificationContextParts`（[`internal/common/notification_context.go:136`](../../../internal/common/notification_context.go)）は、`group`／`command` の下位値として `KindString` に加え、`value.Any()` が `identifier.Identifier` である値を受け、その `Name()` を名前として読む。`slog.Value.Resolve()` はトップレベルの `LogValuer` だけを解決し、グループの下位値までは再帰しない（Go 1.26 の `log/slog` `Value.Resolve`）ため、復元側で下位値を見る必要があるが、汎用の `Resolve` は使わない。`Resolve` は string を返す任意の `LogValuer` を受け入れてしまい、「符号化が固定する値の型」という `DecodeNotificationContext` の契約を緩める（CLAUDE.md「Reject, don't normalize」）。受け入れるのは `LogValue` が生成する 2 形式だけである。

- 正規化前の生のレコード（`identifier.Identifier` の `LogValuer`）と、`RedactingHandler` が正規化した string のレコードの両方が復号できる。
- `scope` は `LogValue` が常に string で符号化するため、従来どおり `KindString` だけを受ける。
- `scope`・`group`・`command` のいずれかの値が string でも `identifier.Identifier` でもない場合（例: `slog.Int`、`identifier.Identifier` 以外の `LogValuer`）は従来どおり拒否する。
- `display`（[`internal/logging/slack_handler.go:491`](../../../internal/logging/slack_handler.go)）は復号済みの `GroupName()`／`CommandName()` を補間するため、変更しない。Scope 表示契約（AC-11）は保たれる。

これにより、Scope に `monkey` が `monkey` として表示される（AC-01、AC-02）。

### 3.4 宣言サイトの置き換え

group 名・コマンド名を属性値として書く production の経路を `slog.Any(key, identifier.NewIdentifier(name))` に置き換える。同じキー `"command"` でも識別子と自由文が関数内で混在するため、行単位で示す。値はコマンド名・group 名そのものであり、展開済みコマンド行・パスは含まない（それらは §3.5）。`internal/common` の 2 ファイルも型の定義パッケージ外なので、修飾形 `identifier.NewIdentifier` を書く。

| ファイル | 関数 | 行 | キー | 値の式 |
|---|---|---|---|---|
| `internal/common/notification_context.go` | `NotificationContext.LogValue` | 81、84 | `group`／`command` | `c.group`／`c.command` |
| `internal/common/logschema.go` | `CommandResult.LogValue` | 120 | `name` | `c.Name` |
| `internal/common/logschema.go` | `CommandResults.LogValue` | 164 | `name` | `cmd.Name` |
| `internal/runner/group_executor.go` | `ExecuteGroup` | 149、151、218 | `name` | `groupSpec.Name` |
| `internal/runner/group_executor.go` | `executeAllCommands` | 233 | `command` | `runtimeCmd.Spec.Name` |
| `internal/runner/group_executor.go` | `verifyGroupFiles` | 383、408 | `group` | `groupName` |
| `internal/runner/group_executor.go` | `outputDryRunDebugInfo` | 435 | `group` | `groupSpec.Name` |
| `internal/runner/group_executor.go` | `executeCommandInGroup` | 459、512 | `command` | `cmd.Name()` |
| `internal/runner/group_executor.go` | `executeCommandInGroup` | 460 | `group` | `groupSpec.Name` |
| `internal/runner/group_executor.go` | `createCommandContext` | 537、543 | `command` | `cmd.Name()`（537 は `LogUnlimitedExecution` の引数） |
| `internal/runner/group_executor.go` | `executeSingleCommand` | 585 | `command` | `cmd.Name()`（`LogTimeoutExceeded` の引数） |
| `internal/runner/group_executor.go` | `executeSingleCommand` | 594、612、617 | `command` | `cmd.Name()`（612 は `buildCommandDebugLogArgs` の引数） |
| `internal/runner/group_executor.go` | `resolveGroupWorkDir` | 652 | `group` | `runtimeGroup.Spec.Name` |
| `internal/runner/runner.go` | `logGroupExecutionSummary` | 547 | `group` | `groupSpec.Name` |
| `internal/runner/config/expansion.go` | `resolveAndPrepareCommandSpec` | 1101 | `command` | `spec.Name` |
| `internal/runner/base/executor/executor.go` | `Execute` | 160 | `command` | `cmd.Name()` |
| `internal/runner/base/executor/executor.go` | `executeWithUserGroup` | 254 | `command` | `cmd.Name()` |
| `internal/runner/base/executor/tempdir_manager.go` | `Create` | 67、92 | `group` | `m.groupName` |
| `internal/runner/base/privilege/unix.go` | `WithPrivileges` | 123、140 | `command` | `execCtx.elevationCtx.CommandName` |
| `internal/runner/base/privilege/unix.go` | `logElevationOutcome` | 347、352 | `command` | `execCtx.elevationCtx.CommandName` |
| `internal/runner/base/audit/logger.go` | `LogUserGroupExecution` | 80 | `command_name` | `cmd.Name()` |
| `internal/runner/base/audit/logger.go` | `LogRiskProfile` | 145 | `command_name` | `entry.CommandName` |
| `internal/runner/resource/normal_manager.go` | `ExecuteCommand` | 108、137 | `command` | `cmd.Name()` |
| `internal/runner/resource/normal_manager.go` | `ExecuteCommand` | 143 | `command_path` | `group.Name`（値は group 名。キー名は path だが値は識別子） |
| `internal/runner/resource/dryrun_manager.go` | `validateRunAsIdentity` | 303、314 | `command` | `cmd.Name()` |
| `internal/runner/resource/dryrun_manager.go` | `validateRunAsIdentity` | 304、315 | `group` | `groupDisplayName` |
| `internal/runner/resource/dryrun_manager.go` | `evaluateCommandRisk` | 430 | `command` | `cmd.Name()` |
| `internal/verification/manager.go` | `VerifyGroupFiles` | 223 | `group` | `groupName`（`input.Name` のローカル値） |
| `internal/verification/manager.go` | `collectVerificationFiles` | 279 | `group` | `input.Name` |

`RuntimeCommand.Name()`・`GroupSpec.Name` 等のフィールド型は変えない。宣言はログを書く行で行う。`normal_manager.go:143` はキー `command_path` に group 名を載せており、値の型は識別子である。キー名の是正は本タスクのスコープ外とし、値だけを宣言する（§5.2）。宣言サイトの追加・削除は、`identifier_guard_test.go` の目録（§7.3）と、その宣言に固定する型・挙動アサーション（該当があれば。§5.2）の更新を同じコミットに含める。免除の rollback はこのコミットを単位に戻す（§5.2）。

`SecurityLogger` の 4 メソッド（[`internal/logging/security.go`](../../../internal/logging/security.go)）は引数 `cmdName` の型を `string` から `identifier.Identifier` へ変え、`buildCommandDebugLogArgs`（[`internal/runner/group_executor.go:553`](../../../internal/runner/group_executor.go)）も `cmdName identifier.Identifier` を受ける。宣言（`identifier.NewIdentifier(…)`）は各呼び出し元で行う。実呼び出し元は `LogUnlimitedExecution` が [`group_executor.go:537`](../../../internal/runner/group_executor.go)、`LogTimeoutExceeded` が [`:585`](../../../internal/runner/group_executor.go)、`buildCommandDebugLogArgs` が [`:612`](../../../internal/runner/group_executor.go) である。ヘルパーが内部で `NewIdentifier(cmdName)` を呼ぶ形だと、そのヘルパー 1 箇所が目録に載るだけで、将来の呼び出し元が `cmd.ExpandedCmd` のような自由文を渡しても guard に新しい宣言が見えずに免除されてしまう。引数型を宣言型にすれば呼び出し元が `identifier.NewIdentifier(…)` を書くことが guard の引数式照合の対象になり、目録外の呼び出しはテストで拒否される（「Enforce invariants with the type, not with convention」）。`LogLongRunningProcess` と `LogTimeoutConfiguration` には現時点で production の呼び出し元が無いが、同じく `identifier.Identifier` を受けるため、将来の呼び出し元は呼び出しサイトで宣言し、§7.3 の目録へ追加しなければならない。この型変更に伴い `internal/logging/security_test.go` と `internal/runner/group_executor_test.go:2880` の呼び出しを `identifier.NewIdentifier(…)` を渡す形へ更新する（§7.4）。

### 3.5 宣言型にしないサイト

キー `"command"`・`"group"`・`"name"` に載っていても、展開済みコマンド行・解決済みパス・OS グループ名・プロセス引数・一時ファイル名は識別子ではない。これらは plain string のままとし、値ベース redaction を従来どおり適用する。

| ファイル | 関数 | 行 | キー | 値の式 |
|---|---|---|---|---|
| `internal/runner/group_executor.go` | `verifyGroupFiles` | 409 | `command` | `resolvedPath`（解決済みパス） |
| `internal/runner/base/executor/executor.go` | `executeWithUserGroup` | 191、198、247、284 | `command` | `cmd.ExpandedCmd` |
| `internal/runner/base/executor/executor.go` | `executeNormal` | 314、327 | `command` | `cmd.ExpandedCmd` |
| `internal/runner/base/executor/executor.go` | `executeWithUserGroup` | 215、247、254、286 | `group` | `cmd.RunAsGroup()`（OS グループ名） |
| `internal/runner/base/executor/command_lifecycle.go` | `prepareCommand` | 301 | `command` | `cmdLine`（展開済みコマンド行） |
| `internal/runner/base/executor/command_lifecycle.go` | `logStartWindowRecords` | 572 | `command` | `pc.cmdLine` |
| `internal/runner/base/executor/command_lifecycle.go` | `reportStartFailure` | 585 | `command` | `pc.cmdLine` |
| `internal/runner/base/executor/command_lifecycle.go` | `superviseCommand` | 661、701、705、738 | `command` | `pc.cmdLine` |
| `internal/runner/base/audit/logger.go` | `LogUserGroupExecution` | 81 | `command_path` | `cmd.Cmd()`（未展開パス） |
| `internal/runner/base/audit/logger.go` | `LogUserGroupExecution` | 83 | `expanded_command_path` | `cmd.ExpandedCmd` |
| `internal/runner/resource/normal_manager.go` | `ExecuteCommand` | 138 | `cmd_binary` | `cmd.ExpandedCmd` |
| `internal/runner/resource/dryrun_manager.go` | `analyzeCommand` | 236 | マップキー `command` | `cmd.ExpandedCmd` |
| `internal/runner/resource/dryrun_manager.go` | `analyzeOutput` | 721 | マップキー `command` | `cmd.ExpandedCmd` |
| `internal/verification/manager.go` | `collectVerificationFiles` | 280 | `command` | `command.ExpandedCmd`（§3.4 の行 279 `group` と同じ呼び出し内） |
| `internal/safefileio/safe_file_linux.go` | `moveFileAnchored` | 241 | `name` | `tmpName`（一時ファイル名） |

`executor.go` の行 247 と 254 は同じ関数内でキー `"group"`（OS グループ名）とキー `"command"`（コマンド名）を同時に書くことに注意する。キーではなく値の式で判断する。

### 3.6 `CommandResult` / `CommandResults`

`CommandResultFields.Name` は `string` のままとし、`LogValue` の符号化だけを変える。`CommandResult.LogValue` は `slog.Any(LogFieldName, identifier.NewIdentifier(c.Name))` を返し、`CommandResults.LogValue` も各 `cmd_%d` グループの `name` を同様にする。`internal/logging` の `commandResultInfo` は `CommandResultFields` を埋め込むため、抽出コード（[`internal/logging/slack_handler.go:714`](../../../internal/logging/slack_handler.go)）は変更しない。

## 4. エラーハンドリング設計

### 4.1 エラー分類

| 状況 | 挙動 |
|---|---|
| 識別子の構築 | エラーを返さない。`NewIdentifier` は全域関数であり、名前の検査は設定境界が担う |
| `Identifier` の `LogValue` | パニックしない。`slog.Value` を返すだけ |
| 復号時の `Identifier` 読み取り | `Name()` はフィールドを返すだけでパニックしない。`Identifier` 以外の `LogValuer` は `LogValue` を呼ばずに拒否するため、`LogValuer` のパニックが復号に持ち込まれる経路は無い。既存のエラー契約を変えない |
| 免除判定後の転送 | `RedactingHandler` の既存のパニック回復・失敗時プレースホルダを変更しない |

### 4.2 エラー型

本タスクは新しいエラー型・センチネルエラーを導入しない。`Identifier` の構築は失敗しないため、`errors.Is`／`errors.AsType[T]` で判定すべき新しいエラーは無い。`DecodeNotificationContext` と `RedactingHandler` の既存エラー契約を維持する。

## 5. セキュリティ考慮事項

### 5.1 脅威モデル

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    NAME[("宣言された識別子")]
    LINE[("展開済みコマンド行<br>標準出力・引数")]

    subgraph Leak["過剰免除（漏洩方向）"]
        T1["T1<br>名前に書いた機密が露出"]
        T2["T2<br>コマンド行の誤宣言で漏洩"]
        C1["受容と文書化<br>（名前は設定リテラル）"]
        C2["型で構築を限定<br>+ 宣言サイトの列挙<br>+ guard テスト"]
    end

    subgraph Loss["免除不足（診断性喪失方向）"]
        T3["T3<br>error 中の識別子が redact"]
        C3["残余リスクとして文書化"]
    end

    NAME -->|"名前に機密がある"| T1
    NAME -->|"自由文に連結される"| T3
    LINE -->|"誤って Identifier と宣言される"| T2
    T1 -.->|"対策"| C1
    T2 -.->|"対策"| C2
    T3 -.->|"対策"| C3

    class NAME,LINE data
    class T1,T2,T3 problem
    class C1,C2,C3 enhanced
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    L1[("入力データ")]
    L2["対策"]
    L3["脅威"]

    class L1 data
    class L2 enhanced
    class L3 problem
```

実線の矢印 入力データ → 脅威 のラベルは、その入力にこの条件が成り立つと脅威が生じることを表す（発生条件）。破線の矢印 脅威 ⇢ 対策 は、その脅威に対応する対策を表す。入力データを上、脅威を中央、対策を下に置き、入力と対策を分けている。`data` は入力データ、`problem` は脅威、`enhanced` は対策である。脅威は方向が逆の 2 系統に分かれる。T1・T2 は免除が過剰に働いて redaction が漏れる方向、T3 は免除が不足して識別子の診断情報が失われる方向である。

| 脅威 | 発生条件 | 影響 | 対策 | 残るリスク | AC |
|---|---|---|---|---|---|
| T1 名前に書いた機密が露出 | 運用者が group 名・コマンド名に機密を書く | その文字列が通知・ログにそのまま出る | 名前は設定リテラルで外部入力経路が無いことを前提に、漏洩の帰結を受容して文書化する。設定境界検査は空名・制御文字・長さを弾き、表示境界の補間契約は Slack の書式としての注入を防ぐ（いずれも機密の秘匿は担わない） | 名前に機密を書いた場合は露出する（AC-14） | AC-14、AC-16 |
| T2 コマンド行の誤宣言 | 実装者が展開済みコマンド行・自由文を `identifier.NewIdentifier` で包む | 値ベース redaction が掛からず、`token=…` や `--password=x` が露出する | 宣言型を leaf パッケージ `internal/identifier` に置き、名前を設定する構築（要素付き複合リテラル・フィールド代入）をコンパイラが拒否する。そのうえで宣言サイトを §3.4 に列挙し、`identifier_guard_test.go` で走査結果と目録を双方向に照合する。目録はファイル・関数・囲むログ呼び出し／文・属性キー・引数式の組に出現数を添えて持ち、目録外の宣言だけでなく、目録にある宣言が走査に見つからない場合（宣言の省略・削除）も失敗させる。`NewIdentifier` の値参照（エイリアス）経由の間接呼び出しも拒否する。`executeWithUserGroup` のように同一関数内でコマンド名 `cmd.Name()` とコマンド行 `cmd.ExpandedCmd` が混在するため、ファイルと関数だけではコマンド行の誤宣言を検出できない。§7.3 の対照テストでコマンド行が redact されることを固定する。この guard が守るのは過剰免除方向（T2）であり、宣言を忘れた新規サイトが redact され続ける T3 方向は対象外である | なし（テストで固定） | AC-08 |
| T3 error 中の識別子が redact | 識別子が error メッセージなどの自由文に連結される | 識別子が `[REDACTED]` になり、どの group／command か判別できない | 型では自由文の部分文字列を宣言できないため対策を設けず、残余リスクとして記録する | error 全文が `[REDACTED]` になりうる | AC-13 |

免除は表示と監査相関（Slack の Scope と `Command` フィールド、監査ログの `command_name`）にだけ作用し、権限判断やコマンド実行を変えない。したがって免除が悪用されても実行権限は拡大しない。

### 5.2 既存の保護との関係と残余リスク

- **Task 0172 との関係。** Task 0172 [`02_architecture.md`](../0172_slack_notification_message_unification/02_architecture.md) §3.5 は「redaction が識別子を書き換え、Scope が `[REDACTED]` になりうる」ことを残余リスクとして受け入れ、`internal/redaction` の適用範囲の見直しを別タスクと明記した。同 §5.2 も「`internal/redaction` の適用範囲は変更しない」と記している。本タスクはその別タスクであり、この残余リスクを置き換える（AC-17）。0172 の承認済み文書は履歴として残す。0172 の設定境界検査（識別子の中身を redaction と照合しない。commit `8d0667c5`）は変更しない（AC-10）。
- **旧挙動を固定する既存テストとその扱い。** 識別子を redact する旧挙動を直接固定するテストは無い。`TestRedactText_AlternativePriority`（[`internal/redaction/redactor_test.go:494`](../../../internal/redaction/redactor_test.go)）の `monkey="a b"` は plain string を `RedactText` に渡すテストであり、本タスク後も有効である。`TestValidateIdentifiers` と `TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted` は AC-10 のとおりそのまま通す。更新が必要なのは、宣言サイトの値を生の捕捉ハンドラで string と比較するテストだけであり、§7.4 に列挙する。
- **識別子の免除は保護の意図的な縮小である。** 設定の名前に機密を書いた場合、その文字列は通知・ログにそのまま出る（AC-14）。名前は TOML に人間が書くリテラルであり、変数展開も外部入力も経由せず、名前に機密を書く経路が現実に無いことは Task 0172 §3.1 が既に確認している。この帰結を利用者向けセキュリティ文書の Limitations へ明記する（AC-16）。
- **自由文の error 文字列に埋め込まれた名前は免除されない（残余リスク）。** 型では部分文字列を宣言できない。`RedactingHandler.Handle` は `record.Message` には `Config.RedactText` のみを適用し、`IsSensitiveValue` による値まるごと判定を行わない（[`redactor.go:722`](../../../internal/redaction/redactor.go)、[`redactor.go:724`](../../../internal/redaction/redactor.go)）。一方、error 属性をはじめとする文字列属性には `RedactText` に続けて値まるごと判定が適用される（[`redactor.go:775`](../../../internal/redaction/redactor.go)、[`redactor.go:785`](../../../internal/redaction/redactor.go)）。したがって `failed to execute group monkey: …` のような error 全文が `[REDACTED]` になりうる。この実害は AC-13 のとおり設計文書に記録し、本タスクのスコープ外とする（01_requirements.md「対象外」）。
- **コマンド行の redaction は維持される。** 展開済みコマンド行は plain string であり、`--password=x` や `token=…` を含めば従来どおり redact される。同名のコマンド名は免除される。この対照がキー名除外を採らない理由そのものであり、AC-08 のテストで固定する。
- **パターン集合を変更しない。** `DefaultSensitivePatterns`・`DefaultKeyValuePatterns`・`ValueDetector` のパターンは変更しない（AC-12）。自由文の検出挙動は不変である。
- **キー `command_path` の誤った値。** `normal_manager.go:143` はキー `command_path` に group 名を載せている。キー名の是正はログスキーマを変えるため本タスクでは行わず、値だけを識別子として免除する。
- **運用上の扱い（kill switch と rollback）。** 免除を止める専用の実行時スイッチは設けない。免除された値は正規化後の string としてそのままログ・通知に現れるため、オンコールは通知に出た名前を TOML と照合すれば「宣言済みで免除された」ことを確認できる。免除は失敗ではないため `RedactingHandler.ErrorCollector` には記録しない。漏洩が疑われる場合は、その宣言を導入したコミットを revert して免除を解除する（型は `internal/identifier` に残る）。宣言サイトの追加・削除は、`identifier_guard_test.go` の目録（§7.3）と、その宣言に固定する型・挙動アサーション（例: `TestNotificationContext_LogValueEncoding`、`TestSlackHandler_IdentifierScopeSurvivesRedaction`、`logschema` の `KindLogValuer` アサーション、`TestLogUserGroupExecution_CommandNameSurvivesRedaction`・`TestCommandResults_E2E_Integration`。該当があれば）の更新を同じコミットに含める規約なので（§3.4）、導入コミットの revert が宣言・目録・アサーションを同時に戻し、CI は緑のままになる。同じコミットに別の宣言サイトが混在し、漏洩したサイトだけを戻す場合は、`git revert -n <commit>` で revert を保留して不要な hunk を戻し、そのコミット内で `make test` を実行して guard と CI の green を確認する。この手順を `security-architecture.ja.md`／`.md` に記す。

### 5.3 Task 0172 の Scope 表示契約

Task 0172 の Scope 表示契約（空名・非表示名は `(scope: invalid)`、それ以外は名前を表示）を変えない（AC-11）。本タスクが変えるのは `NotificationContext.LogValue` の下位値の型だけで、`display`（[`internal/logging/slack_handler.go:491`](../../../internal/logging/slack_handler.go)）と補間契約は変更しない。復号は生の宣言型と正規化後の string の両方を受ける（§3.3）。

### 5.4 他の設計文書のポリシーとの関係

本設計は Task 0163 の送信キュー・ワーカー・リトライ・flush・ドライランの境界を変更しない。Task 0172 §5.2 の「`internal/redaction` の適用範囲は変更しない」という記述は、本タスクが明示的に引き継いだ「別タスクで見直す」という前提である。Task 0172 §3.5/§5.2 の残余リスクを置き換えるという判断の理由は §5.2 に記した。旧挙動を固定する既存テストの扱いも §5.2 と §7.4 に記した。この関係は本節に記す。

## 6. 処理フロー詳細

### 6.1 属性 1 件の redaction 判定フロー

```mermaid
flowchart TD
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    START(["属性 1 件"])
    KEY{"キー名が機密?"}
    ID{"値が identifier.Identifier?"}
    KIND{"値の種別"}
    STR["RedactText<br>key=value 置換 + 値形式検出"]
    WHOLE["IsSensitiveValue<br>値まるごと判定"]
    GROUP["グループを再帰"]
    NORM["string へ正規化"]
    PLACE["[REDACTED]"]

    START --> KEY
    KEY -->|"はい"| PLACE
    KEY -->|"いいえ"| ID
    ID -->|"はい"| NORM
    ID -->|"いいえ"| KIND
    KIND -->|"string"| STR
    STR -->|"変化あり"| DONE(["転送"])
    STR -->|"変化なし"| WHOLE
    WHOLE -->|"一致"| PLACE
    WHOLE -->|"不一致"| DONE
    KIND -->|"group"| GROUP
    KIND -->|"LogValuer/Any"| RESOLVE["解決して再帰"]
    NORM --> DONE
    GROUP --> DONE
    RESOLVE --> DONE
    PLACE --> DONE

    class START,KEY,KIND,STR,WHOLE,GROUP,RESOLVE process
    class ID,NORM enhanced
    class PLACE problem
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef problem fill:#ffe6e6,stroke:#d62728,stroke-width:2px,color:#7b0000;

    L1["既存処理"]
    L2["追加する判定"]
    L3["マスク結果"]

    class L1 process
    class L2 enhanced
    class L3 problem
```

矢印 A → B は「A の判定結果に応じて B へ進む」ことを表す。`Identifier` はキー名判定の後、種別判定の前に short-circuit するため、値ベース redaction のどの層も実行されない。

### 6.2 書き込みから表示までの伝播

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    SRC["宣言サイト"]
    ID["identifier.Identifier"]
    REC[("slog.Record")]
    RH["RedactingHandler"]
    STR[("string 属性")]
    NCTX["NotificationContext.LogValue"]
    CR["CommandResult.LogValue"]
    SCOPE["Slack Scope 表示"]
    JSON[("JSON ログ")]

    SRC --> ID
    ID --> REC
    NCTX --> REC
    CR --> REC
    REC --> RH
    RH --> STR
    STR --> SCOPE
    STR --> JSON

    class REC,STR,JSON data
    class SCOPE process
    class SRC,RH,NCTX,CR enhanced
    class ID newpkg
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef newpkg fill:#ffe8f5,stroke:#d946ef,stroke-width:2px,color:#701a75;

    L1[("データ")]
    L2["変更しない既存処理"]
    L3["変更・追加する処理"]
    L4["新しく追加する型"]

    class L1 data
    class L2 process
    class L3 enhanced
    class L4 newpkg
```

矢印 A → B は「A が B へ値を渡す、または A が B を生成する」ことを表す。

## 7. テスト戦略

### 7.1 単体テスト

| 対象 | 検証内容 |
|---|---|
| `internal/identifier/identifier_test.go` | `NewIdentifier(name).Name()` と `.String()` が名前を返す。`LogValue()` が `KindString` で名前を返す。ゼロ値の `Identifier{}` は空名として扱われる |
| `internal/redaction/redactor_test.go` | 免除（AC-01〜AC-04）と対照（AC-05、AC-08）。`Config.RedactLogAttribute` と `RedactingHandler` の双方を通す |
| `internal/common/notification_context_test.go` | `LogValue` の下位値が `identifier.Identifier` であること、`DecodeNotificationContext` が生の宣言型と正規化後の string の両方を復号すること（AC-06、AC-11） |
| `internal/common/logschema_test.go` | `CommandResult.LogValue`／`CommandResults.LogValue` の `name` が宣言型で符号化され、`Value.String()` が名前を返すこと（AC-06、AC-09） |

免除のテストは次の 3 層それぞれの一致形を用意する（AC-04）。最後の行は免除の適用範囲ではなく、キー名マスクとの優先順位（fail-closed）を固定する（AC-08）。

| 層 | 識別子の例 | 対照（plain string） |
|---|---|---|
| key=value 置換 | `backup --password=x` | 同じ文字列を `slog.String("command", …)` で載せると `password` 以降が redact される |
| 値形式検出 | `AKIAIOSFODNN7EXAMPLE`、`ghp_` + 36 文字、`github_pat_` + 30 文字 | 同じ文字列を plain string で載せると `[REDACTED]` になる |
| 値まるごと判定 | `monkey`、`rotate_api_key`、`keyboard` | 同じ文字列を plain string で載せると `[REDACTED]` になる |
| キー名マスク（優先順位） | キー `password` に `slog.Any("password", identifier.NewIdentifier("monkey"))` を載せる | 同じキーの plain string と同じく値は `[REDACTED]` のまま（§3.2 のとおりキー名判定が免除判定より先。fail-closed、AC-08） |

対照ケースは、免除が plain string へ漏れていないことを免除ケースと対にして検証する。免除側は識別子の文字列がそのまま現れること、対照側は同じ文字列が plain string では値ベース redaction により redact されることを、同じテスト関数内でそれぞれアサートする。検証対象の挙動を壊したときにテストが失敗することの確認は、CLAUDE.md「Every test must be able to fail for its stated reason」と AC-19 のとおり実装時に行い、その結果を実装コミットメッセージに記す。

### 7.2 統合テスト

- `internal/runner/integration_command_results_test.go` と同種の経路で、`RedactingHandler` → JSON ハンドラを通し、`CommandResult` の `name` が redaction 後も元の文字列として出力されることを確認する（AC-06）。
- `internal/logging/slack_handler_test.go` の既存テストで、Scope が `monkey` を表示し、`(scope: invalid)` にならないことを確認する（AC-01、AC-02、AC-11）。既存の `TestSlackHandler_UserGroupCommandFailure`・`TestSlackHandler_InvalidNotificationContext` を基準に、識別子を含む入力のケースを足す。
- 監査ログの `command_name` と `command_group_summary` のコマンド一覧の名前が残ることを確認する（AC-09）。

### 7.3 セキュリティテスト

- stdout・stderr・展開済みコマンド行・引数・環境変数値・message・error 文字列の redaction が本タスクの前後で変わらないこと（AC-05）。`--password=x`、`token=…`、`Bearer …`、AWS/GitHub/Slack トークン形を、識別子と同じテスト入力集合で固定する。
- 同じキー `"command"` に、コマンド名（宣言型）とコマンド行（plain string）を載せ、後者だけが redact されること（AC-08）。
- 宣言サイトの限定。`internal/identifier/identifier_guard_test.go` が、既存の `internal/testutil/identitymutationguard` で production の Go ファイルを走査し、`identifier.NewIdentifier` の呼び出しと §3.4 の宣言サイト目録を双方向に照合する。目録はファイル・関数・囲むログ呼び出し／文・属性キー・引数式の組をキーとして出現数とともに持ち、`NewIdentifier` に渡された引数をソースから復元した式（例: `cmd.Name()`）と照合する。走査結果に目録の外の呼び出しが現れれば失敗し、逆に目録のエントリが走査結果に現れなければ（宣言の省略・削除）失敗する。出現数と囲み文脈を要するのは、§3.4 に同一の `(ファイル, 関数, 引数式)` の組が複数回現れ、件数を保つ移設でも検出できるようにするためである。例えば `ExecuteGroup` の行 149・151・218 は 3 件とも `groupSpec.Name` であり、`executeSingleCommand` の `cmd.Name()` 4 件は囲むログ呼び出し（`LogTimeoutExceeded`・`buildCommandDebugLogArgs`・2 つの `slog.Error`）で区別する。行番号は編集でずれるため合否判定には使わない。これにより、宣言の付け忘れや削除を局所的な統合テストに頼らず検出でき、AC-07 の「すべての宣言サイトが列挙どおりに変換されている」ことを guard が固定する。走査は `Options.Extra` に `ExtraTrackedFunc` を 2 件渡し、他パッケージからのパッケージ修飾呼び出し（`ImportPath: "github.com/isseis/go-safe-cmd-runner/internal/identifier"`、`FuncName: "NewIdentifier"`）と、`internal/identifier` 内の非修飾呼び出し（`ImportPath` を空にし `FuncName: "NewIdentifier"`）の両方に一致させる。呼び出しの照合に加えて、`NewIdentifier` の値参照（エイリアス）を拒否する。`makeID := identifier.NewIdentifier` のように束縛して `makeID(cmd.ExpandedCmd)` と呼ぶと、呼び出しサイト比較には何も現れずに免除が掛かるため、束縛自体を失敗させる。修飾形の値参照は `identitymutationguard` が `ValueRef` として報告するので（[`helpers.go:105-114`](../../../internal/testutil/identitymutationguard/helpers.go)）、guard は `NewIdentifier` の `ValueRef` が 0 件であることを要求する。非修飾形の値参照は同ヘルパーが報告しない（`Options.Extra` の空 `ImportPath` は呼び出しサイトだけに一致する）ため、guard 自身が production ファイルの AST を走査し、`func NewIdentifier` の宣言名と直接呼び出しの被呼び出しを除いて、値位置に残る `NewIdentifier` 識別子を失敗させる。構築経路そのもの（`identifier.Identifier{name: …}` の複合リテラルと `id.name = …` のフィールド代入）は leaf パッケージ外ではコンパイラが拒否するため guard の対象外である。guard が守るのは過剰免除方向（T2）であり、宣言を忘れた新規サイトが redact され続ける T3 方向は対象外である。`ProductionGoFilesInRepo` は `//go:build test || performance` のようにタグ `test` を必須としない制約のファイル（`internal/testutil`、`internal/runner/base/executor/testutil`）も production として走査する。テストの期待値を `identifier.NewIdentifier` で組み立てるコードは `_test.go` に置き、これらの補助パッケージには置かない。
- 設定検証が識別子の中身を redaction と照合しないこと。既存の [`TestValidateIdentifiers`](../../../internal/runner/config/validation_test.go) と [`TestE2E_PreExecutionError_RedactionRewrittenNamesAreAccepted`](../../../cmd/runner/integration_pre_execution_error_test.go) をそのまま通す（AC-10）。
- `DefaultSensitivePatterns`・`DefaultKeyValuePatterns`・`ValueDetector` のパターン集合が変更されていないこと（AC-12）。該当ファイルを変更しないことと、既存のパターンテストが通ることで確認する。
- `[]Identifier` と `[]*Identifier` の要素が免除されること（§3.2 の挿入点を固定する。人工的な `[]identifier.Identifier`・`[]*identifier.Identifier` を用意し、下流ハンドラが描画した JSON／text に名前が string として現れることを検証する。生の `Identifier` を `KindAny` のまま流すと `[{}]` と描画されるため、この描画結果の検証で正規化漏れを捉える）。
- 型付き nil の `*identifier.Identifier` がパニックせず fail-closed になること（§3.2）。`slog.Any("command", (*identifier.Identifier)(nil))` を直接属性として載せたときに、ヘルパーが免除せず（`Name()` を呼ばず false を返し）、既存の `processLogValuer` のパニック回復が代入する `RedactionFailurePlaceholder`（[`redactor.go:260`](../../../internal/redaction/redactor.go)）が出力に現れることを検証する。識別子名でも panic でもないことをアサートする。

### 7.4 既存テストの更新

宣言サイトや helper の引数型を変えると、redaction を通さず生の属性を捕捉して名前を string と比較するテスト、および helper に string を直接渡しているテストが失敗する。`internal/testutil.LogRecorder` は `a.Value.Any()` を保存し `Resolve` しないため、型を宣言した値は `identifier.Identifier` として捕捉される。次のテストを `identifier.NewIdentifier(…)` との比較または引数渡しへ更新する。

| テスト | 箇所 | 宣言されるキー |
|---|---|---|
| `internal/runner/group_executor_test.go` | タイムアウト・無制限実行の属性検査 | `command` |
| `internal/runner/group_executor_test.go:2880` | `buildCommandDebugLogArgs` の呼び出し引数 | `command` |
| `internal/runner/group_executor_timeout_test.go` | `Attrs["command"]` の比較 | `command` |
| `internal/runner/runner_test.go::TestCommandResult_LogValue` | `attr.Value.Kind()` を `KindString`／`KindInt64` だけで分岐するため（`:2062-2070`）、`name` が `KindLogValuer` になると map から落ちて失敗する。`KindLogValuer` と `Value.Any()` が `identifier.Identifier` であることを検証する行へ更新する | `name` |
| `internal/runner/base/privilege/unix_privilege_test.go` | 昇格結果の属性検査 | `command` |
| `internal/runner/base/audit/logger_test.go` | `command_name` の期待値 | `command_name` |
| `internal/logging/security_test.go` | `SecurityLogger` 4 メソッドの呼び出し引数 | `command` |
| `internal/common/notification_context_test.go` | `LogValue` の符号化期待値 | `group`／`command` |

`Value.String()` を比較するテスト（例: `internal/common/logschema_test.go:72`）と、JSON ハンドラの出力を解析するテスト（例: `internal/runner/resource/audit_wiring_test.go:85`）は、`String` の実装と免除経路の正規化により変更を要さない。実装時に全 `*_test.go` を検索し、宣言型を比較する期待値を洗い出す。

## 8. 実装優先順位

### 8.1 フェーズ分割

各フェーズの完了条件は、`make fmt`・`make test`・`make lint` が通ることである（AC-18 の各コミット時点の green gate）。加えてフェーズ固有の条件を次に示す。

| フェーズ | 内容 | 追加の完了条件 |
|---|---|---|
| Phase 1 | `internal/identifier/identifier.go` と `identifier_test.go` を追加する | `Identifier` の単体テストが通る。leaf パッケージは `internal/` パッケージを import しない |
| Phase 2 | `internal/redaction` に免除経路を追加し、免除・対照・コマンド行維持・`[]Identifier` のテストを書く | 新規テストが通り、AC-19 の確認結果（検証対象を壊したときのテストの挙動）をコミットメッセージに記す |
| Phase 3 | `NotificationContext.LogValue` を宣言型で符号化し、`decodeNotificationContextParts` に `identifier.Identifier` の受理を追加する | `notification_context_test.go` が通る |
| Phase 4 | 宣言サイト（§3.4）を `identifier.NewIdentifier` へ置き換え、`CommandResult`／`CommandResults` を更新し、既存テストを更新し、`identifier_guard_test.go` を追加する | `make test` が通り、AC-07〜AC-09 のテストと guard が通る |
| Phase 5 | `security-architecture.ja.md`・`.md`、`security-risk-assessment.ja.md`・`.md` を更新し、Task 0172 への相互参照を確認する | AC-15〜AC-17 の static 検証が通る |
| Phase 6 | 全体の green gate を再実行し、コミットメッセージに AC-19 の確認を記す | `make test`・`make lint` |

日本語版の文書を先にコミットし、英語版は `/mktrans` で反映する。

### 8.2 実装順の根拠

Phase 1 を最初に置くのは、宣言型が Phase 2・3・4 すべての前提だからである。Phase 2 を Phase 3 より先に置くのは、免除が既存の `slog.String` 属性の扱いを変えないことを、`NotificationContext` の符号化変更と分離して確認できるためである。Phase 4 は宣言サイトの数が多く、Phase 2・3 で判定が固まってから機械的に進める。

## 9. 将来の拡張性

- 新しい設定由来の識別子をログへ載せるときは、そのサイトで `identifier.NewIdentifier` を使い、`identifier_guard_test.go` の目録へファイル・関数・囲むログ呼び出し／文・属性キー・引数式の組と出現数を追加する。宣言を削除するときは目録からも取り除く（取り除かなければ、目録にある宣言が走査に見つからず guard が失敗する）。`identifier.NewIdentifier` を変数へ束縛して間接に呼び出すこと（エイリアス）は guard が拒否するため、宣言は常に呼び出しサイトで直接行う。`internal/redaction` にサイト固有の分岐を足す必要はない。
- `IsSensitiveValue` の語境界化や `ValueDetector` のパターン調整を将来行っても、宣言済みの識別子は免除のままである。免除はパターン集合に依存しない。
- ファイルパス・OS グループ名・一時ファイル名など、識別子でない値は引き続き plain string とし、redaction を適用する。免除の対象を広げる場合は、その値が本当に「人間が設定に書く識別子」かを先に確認する。
- `Identifier` の構築は leaf パッケージ `internal/identifier` の `NewIdentifier` にコンパイラで限定されている。消費者側に識別子構築の都合（別の生成規則など）が生じた場合は、消費者へ型を戻すのではなく、leaf パッケージにコンストラクタを追加し、guard の目録を更新する。

## 付録 A: Acceptance Criteria と設計の対応

| AC | 設計の対応 |
|---|---|
| AC-01、AC-02 | §3.1、§3.3、§7.1、§7.2 |
| AC-03、AC-04 | §3.2、§7.1 |
| AC-05 | §1.1、§3.5、§7.3 |
| AC-06 | §1.1、§3.1、§3.6、§7.1、§7.2 |
| AC-07 | §3.4 |
| AC-08 | §1.3、§3.5、§5.2、§7.1、§7.3 |
| AC-09 | §3.4、§3.6、§7.2 |
| AC-10 | §1.1、§5.2、§7.3 |
| AC-11 | §3.3、§5.3 |
| AC-12 | §1.1、§5.2、§7.3 |
| AC-13 | §5.2 |
| AC-14 | §5.2、§2.2（`security-risk-assessment` の更新） |
| AC-15 | §2.2、§5.2、Phase 5 |
| AC-16 | §2.2、§5.2、Phase 5 |
| AC-17 | §5.2、§5.4、Phase 5 |
| AC-18 | §8.1 |
| AC-19 | §7.1、§8.1 |

## 付録 B: 決定履歴

- **キー名除外・パターン調整を採らない理由**は §1.3 に記した。両者とも自由文の redaction を弱めるか識別子を救えないため、型宣言を採る。
- **0172 からの経緯。** Task 0172 §3.5 は、redaction が識別子を書き換える挙動を残余リスクとして受け入れ、`internal/redaction` の適用範囲の見直しを別タスクとした。0172 §5.2 は「`internal/redaction` の適用範囲は変更しない」と明記し、識別子をキー単位で除外する案を、Slack 以外のマスクも同時に弱めるとして退けた。本タスクはその別タスクとして、キー名ではなく型で免除する。
- **`internal/redaction` から `internal/identifier` への依存を許容する理由**は §2.1 と §5.4 に記した。免除の判定主体は redaction 側にあり、宣言型は leaf パッケージが持つ。`internal/identifier` は内部パッケージを import しないため、identifier への辺はどれも非循環である。
- **`Identifier` を leaf パッケージ `internal/identifier` に置く理由。** 旧設計は `internal/common` に置き、`internal/redaction` の guard が複合リテラル・フィールド代入を列挙して構築経路を塞いでいた。この方式はレビューで抜け道（件数を保つ移設・複合リテラル・`Identifier.name` への代入）が順次見つかり、列挙が網羅できないことが確認された。型を定義パッケージ外から触れない leaf パッケージへ移すと、名前を設定する構築をコンパイラが拒否するため、guard の構築形検査と対応する AC-19 mutation を削除できる（CLAUDE.md「Enforce invariants with the type, not with convention」）。`internal/common` の 2 エンコーダも定義パッケージ外になり、修飾形 `identifier.NewIdentifier` を使う。
- **`Identifier` に `LogValue` と `String` の両方を持たせる理由**は §3.1 に記した。前者は redaction 非経由の解決系ハンドラ向け、後者は `slog.Value.String()` を直接呼ぶ既存コード向けである。
- **`RedactText` ではなく属性レベルで免除する理由。** `RedactText` は string を受け取るため、呼び出し時点で型情報が失われる。免除は `slog.Attr` を扱う層で行う必要がある。
- **`CommandResultFields.Name` を型変更せず `LogValue` で宣言する理由。** フィールド型を変えると構築サイト全体と抽出コードへ波及する。要件の対象は `LogValue` の符号化であり、フィールドは string のまま宣言サイトだけを変える。
- **免除に対する専用の kill switch を作らない理由。** 免除は失敗ではなく表示方針であり、実行時スイッチを足すと「redaction が効いているかどうか」が環境依存になる。rollback はその宣言を導入したコミットの revert（宣言・目録・固定アサーションをまとめて戻す）とし、§5.2 に記した。
- **rollback を導入コミット単位にする理由。** 免除状態は宣言サイト・guard の目録・型／挙動アサーションの 3 つが同時に固定しており、宣言だけを戻すと残り 2 つが CI を落とす。3 つを人手で列挙して直す手順は更新漏れを生むため、同一コミットに含める規約（§3.4）とその revert を rollback の単位にした（§5.2）。
