# 要件定義書: redaction 境界の不統一を解消（メッセージ本文・map/slice・監査ログ）

## Document Status

| Item | Value |
|---|---|
| Status | `draft` |
| Created | 2026-07-20 |
| Review date | - |
| Reviewer | - |
| Comments | - |

## 関連 Issue

- [#861 [Security][P2] redaction 境界の不統一を解消（メッセージ本文・map/slice・監査ログ）](https://github.com/isseis/go-safe-cmd-runner/issues/861)
- 関連（解消済み）: D2 H-1（`RedactingHandler.Handle` がメッセージ本文を redact しない）は [#859](https://github.com/isseis/go-safe-cmd-runner/issues/859) で個別対応済み。`internal/redaction/redactor.go:405` の `Handle` は現在 `redactedMessage := r.config.RedactText(record.Message)` を実施しており、本タスクの対象外。
- 詳細所見:
  - [findings/D2_logging_redaction.md](../0149_security_code_smell_audit_fable/findings/D2_logging_redaction.md) M-1, M-3
  - [findings/A7_audit.md](../0149_security_code_smell_audit_fable/findings/A7_audit.md) M-1, M-2, M-3
  - [findings/A4_security.md](../0149_security_code_smell_audit_fable/findings/A4_security.md) M-2
  - 集約サマリ: [99_summary.md](../0149_security_code_smell_audit_fable/99_summary.md)

## 背景

Issue #861 は、`internal/redaction`（RedactingHandler）・`internal/logging`（Slack 送出経路）・`internal/runner/base/audit`（監査ログ）・`internal/runner/base/security`（環境変数サニタイズ）の 4 箇所に分布する redaction 欠落所見が、いずれも同型の根本原因に帰着すると指摘している。

1. **`RedactingHandler` が `slog.Any` 経由の map/struct/slice 要素に再帰しない**（D2 M-1）。この制約は audit パッケージ側でも既知の前提として扱われており（`internal/runner/base/audit/logger.go:286-289` のコメント参照）、`LogRiskProfile` はこの制約を認識した上で `command_args`/`operand_zones` に境界 redaction（`argRedactor.RedactText`）を個別適用して回避している。
2. **`LogRiskProfile` が確立した「境界 redaction」の設計パターンが、同一パッケージ内の他 3 メソッド（`LogUserGroupExecution`/`LogSecurityEvent`/`LogPrivilegeEscalation`）に遡及適用されていない**（A7 M-1〜M-3）。
3. **`SanitizeEnvironmentVariables` がキー名のみで判定し値を見ない**（A4 M-2）ため、キー名が中立な環境変数に格納された秘密情報（PEM、JWT 等）が非 redaction 経路（このメソッド自身の呼び出し元）で漏れる。
4. **Slack webhook URL 自体が credential であるにもかかわらず、送信失敗時のエラーログ（`slog.Any("error", err)`）に平文で残る**（D2 M-3）。これも根本的には (1) の「`slog.Any` の値が文字列化・redaction されずに JSON 直列化される」という同じ制約の一表現である。

これらはいずれも「audit/logging パッケージを通る文字列は必ず redaction される」という単一の不変条件の欠落箇所であり、まとめて解消することで再発防止（新規コード追加時に同型の漏れが生じない構造）を図る。

## 目的

- `RedactingHandler` が `slog.Any` で渡される値（map・struct・スライス要素）に対しても、文字列属性・スライス内 LogValuer と同等の redaction を適用するようにし、「型に依存せず redaction される」という不変条件を確立する。
- `internal/runner/base/audit` の全 4 メソッド（`LogUserGroupExecution`/`LogPrivilegeEscalation`/`LogSecurityEvent`/`LogRiskProfile`）で、ユーザ由来の文字列フィールドに対する境界 redaction の適用を対称にする。
- Slack webhook URL がエラーログ経由で漏れる経路を塞ぐ。
- `SanitizeEnvironmentVariables` がキー名だけでなく値の内容にも基づいて redaction するようにする。
- 各修正が正常系のログ出力内容（redaction 対象外のフィールド）に副作用を与えないことをテストで保証する。

## スコープ

### 対象（本タスクで対応する）

1. **D2 M-1**: `RedactingHandler.processKindAny`（`internal/redaction/redactor.go:522-543`）が map・struct を無加工で通す問題、および `processSlice`（`redactor.go:641-736`）が非 LogValuer な文字列スライス要素を無加工で通す問題。
2. **D2 M-3**: Slack 送信失敗時のエラーログ（`internal/logging/slack_handler.go:875-878` ほか `slog.Any("error", err)` 箇所）に webhook URL が平文で残る問題。
3. **A7 M-1**: `LogUserGroupExecution`（`internal/runner/base/audit/logger.go:96-109`）が失敗時に `stdout`/`stderr` を境界 redaction なしで記録し、`slack_notify=true` を伴って外部送出経路に乗せている問題。
4. **A7 M-2**: 境界 redaction（`argRedactor.RedactText`）の適用が `LogRiskProfile` にのみ実装され、`LogUserGroupExecution`（`command_args`/`expanded_command_args`）・`LogPrivilegeEscalation`（`operation`/`commandName`）・`LogSecurityEvent`（`message`）に適用されていない非対称性。
5. **A7 M-3**: `LogSecurityEvent` の `details map[string]any`（`internal/runner/base/audit/logger.go:171-174`）が (a) 値を redaction せずに `slog.Any` で出力する、(b) キーが無検証でスキーマ予約キー（`severity`/`audit_type`/`slack_notify`/`decision` 等）と衝突し得る、という 2 つの問題。
6. **A4 M-2**: `SanitizeEnvironmentVariables`（`internal/runner/base/security/environment_validation.go:9-26`）がキー名パターンのみで判定し値の内容を検査しない問題、および `sensitiveEnvRegexps` が `upperName` に対して適用されるため利用者定義の小文字カスタムパターンが恒常的に不一致となる問題。

### 対象外（別 Issue・別タスクとする）

- D2 H-1（メッセージ本文の redaction）: [#859](https://github.com/isseis/go-safe-cmd-runner/issues/859) で対応済み。
- D2 M-2（key=value redaction の空白・引用符・JSON 形式カバレッジ不足）、D2 M-4（ValueDetector パターン網羅性: GitHub fine-grained PAT、JWT 等）、D2 M-5（Slack 送信の同期ブロッキング）: #861 の「該当箇所」リストに含まれず、根本原因（`slog.Any` 未再帰）とは独立した所見のため別途検討。
- D2 の Low/Info 所見全般（L-1〜L-6, I-1〜I-6）。
- A7 L-1（severity 判定の二重実装・fail-open）、L-2（`chain[].path` の redaction 非適用）、I-1〜I-5: #861 の「該当箇所」リストに含まれない。特に L-2 は本タスクの根本原因修正（D2 M-1 で `slog.Any` の map 要素が再帰的に redaction されるようになること）により `chain[].path` も自動的に緩和され得るが、`path` は本来 redaction 対象として想定されたフィールドではない（低リスクなパス文字列)ため、明示的な対応は本タスクのスコープ外とする。
- A4 M-1（インタプリタ用コード注入 env 変数が loader-control 拒否リストに未登録）、A4 の Low/Info 所見全般: redaction ではなく実行時ブロック（Reject）に関する所見であり、#861 のスコープ外。

## 現状の問題点（詳細）

### 1. D2 M-1: `slog.Any` の map/struct/スライス要素が redaction を素通りする

`RedactingHandler.processKindAny`（`internal/redaction/redactor.go:522-543`）は LogValuer とスライスのみを処理し、それ以外の型（map・struct 等）はコメント「3. Unsupported type: pass through」の通り無加工で返す。下流の JSON ハンドラは `json.Marshal` で全エクスポートフィールドを直列化するため、`slog.Any("details", map[string]any{"api_key": "..."})` のような呼び出しで秘密情報がそのままログファイルへ書かれる。

`processSlice`（`redactor.go:641-736`）も同様に、LogValuer でない要素（例: `[]string{"password=secret"}` の各要素）を「Non-LogValuer element: keep as-is」（`redactor.go:728-730`）でそのまま通す。同じ文字列が単独の `slog.String` 属性であれば `RedactText` が適用されるのに、スライス要素だと適用されないという非一貫性がある。

### 2. D2 M-3: Slack webhook URL が送信失敗時のエラーログに漏れる

`internal/logging/slack_handler.go:875-878` の `slog.Warn("Failed to send Slack request", slog.Any("error", err), ...)` は、`http.Client.Do` が返す `*url.Error` をそのまま `slog.Any` 属性として渡す。`*url.Error.Error()` は URL 全体（`hooks.slack.com/services/<token>` を含む）を含み、`processKindAny` の現状（前項）では error 値は LogValuer でも slice でもないため無加工で通り、JSON 直列化時に `*url.Error` のエクスポートフィールド（`URL` を含む）がそのまま出力される。webhook URL は Slack 公式に secret 扱いを要求される credential であり、これがログファイル・stderr に平文で残る。

### 3. A7 M-1: `LogUserGroupExecution` が stdout/stderr を境界 redaction なしで記録する

`internal/runner/base/audit/logger.go:100-109` は、コマンド失敗時（`ExitCode != 0`）に `result.Stdout`/`result.Stderr` を `slog.String` でそのまま属性に載せ、`slack_notify=true` を付与して Slack 送出経路に乗せる。`LogRiskProfile` が確立した境界 redaction（`argRedactor.RedactText`、`logger.go:255-261`）はここには適用されておらず、`RedactingHandler` のパターンベース redaction（既知パターンのみ対象）に依存した「ベストエフォート」の防御に留まる。

### 4. A7 M-2: 境界 redaction の適用がメソッド間で非対称

`LogRiskProfile` のみが `argRedactor.RedactText` による境界 redaction を実装しており、他の 3 メソッドは `RedactingHandler` の存在を暗黙の前提にしている。特に `LogUserGroupExecution` の `command_args`/`expanded_command_args`（引数を空白 join した文字列、`logger.go:71,73`）は秘密を含む典型フィールド（`--password=xxx` 等）である。`LogPrivilegeEscalation` の `operation`/`commandName`、`LogSecurityEvent` の `message` も同様にユーザ・呼び出し元由来の文字列であり、境界 redaction が適用されていない。

### 5. A7 M-3: `LogSecurityEvent` の `details` map — redaction バイパスとキー衝突

`internal/runner/base/audit/logger.go:171-174` は `details map[string]any` の各値を `slog.Any(key, value)` でそのまま属性化する。D2 M-1 の制約（map/struct が `processKindAny` を素通りする）により、`RedactingHandler` が有効な production 構成でも `details` 内の秘密は一切マスクされない。また `details` のキーは無検証で属性に追加されるため、`severity`/`audit_type`/`slack_notify`/`decision` 等のスキーマ用キーと重複し得る（slog は重複キーを両方出力し、下流の JSON パーサは多くが後勝ちで解釈するため、監査解析・アラート抑制の判断を汚染し得る）。

### 6. A4 M-2: `SanitizeEnvironmentVariables` が値を検査せずキー名のみで判定する

`internal/runner/base/security/environment_validation.go:9-26` の `SanitizeEnvironmentVariables` は `isSensitiveEnvVar(key)` の結果のみで redaction 要否を決める。値そのものが秘匿情報（秘密鍵 PEM、JWT、AWS シークレット等）であっても、キー名が既知パターン（`PASSWORD`/`TOKEN`/`KEY`/`API` 等）に一致しなければ素通しする。加えて `sensitiveEnvRegexps` は `strings.ToUpper(name)` に対して適用される（`environment_validation.go:36`）ため、利用者が `Config.SensitiveEnvVars` に小文字のカスタムパターンを設定すると恒常的に不一致となる（設定のフットガン）。

## 要件

### F-001: `RedactingHandler` の map/struct 対応

`slog.Any` で渡された値が map または struct（LogValuer でも slice でもない複合型）である場合、`RedactingHandler` はその中身（キー・値、ネストした map/struct/slice を含む）に対しても、文字列属性と同等の redaction（`RedactText` によるキー=値パターン検出・値形式検出、および `IsSensitiveKey` によるキー名判定）を適用したうえで下流ハンドラへ渡す。

**Acceptance Criteria**:
- **AC-01**: `slog.Any("details", map[string]any{"api_key": "secret-value"})` を出力すると、下流ハンドラが受け取る最終的な値（JSON 直列化結果を含む）に `secret-value` の平文が含まれない。
- **AC-02**: `slog.Any("details", map[string]any{"note": "password=hunter2"})` のように、キー名は非機密だが値の内容が機密パターンに一致する場合も、値がマスクされる（キー名ベースの判定だけに依存しない）。
- **AC-03**: ネストした map（`map[string]any{"outer": map[string]any{"token": "..."}}`）でも `maxRedactionDepth` の範囲内で再帰的に redaction が適用される。
- **AC-04a**: struct 値（エクスポートされた文字列フィールドを持つ型）を `slog.Any` で渡した場合、フィールドごとに redaction が適用される（`map[string]any{"api_key": "secret-value"}` 相当の内容を持つ struct を渡すと、下流ハンドラが受け取る最終的な値に `secret-value` の平文が含まれない）。
- **AC-04b**: フィールドごとの redaction が技術的に実装困難な struct 形状（unexported フィールドのみ、循環参照等）に限り、`RedactionFailurePlaceholder` 等の安全なプレースホルダへのフォールバックを許容する。この場合も redaction を経ずに機密文字列が下流へ渡らないことを検証する。
- **AC-05**: 機密パターンを含まない map/struct 値は、redaction 適用後も実質的な内容（キー・値の対応関係）が保たれる（正常系への回帰がない）。

### F-002: `RedactingHandler.processSlice` の非 LogValuer 文字列要素への redaction 適用

`processSlice` が処理する要素のうち、LogValuer でない文字列型要素にも `RedactText`（およびキー名ではなく値のみの判定であるため `IsSensitiveValue` 相当の判定）を適用する。

**Acceptance Criteria**:
- **AC-06**: `slog.Any("args", []string{"--password=hunter2"})` を出力すると、対応するスライス要素中の `hunter2` が下流ハンドラの出力に平文で含まれない。
- **AC-07**: 機密パターンを含まない文字列スライス要素は redaction 適用後も内容が変化しない。
- **AC-08**: 文字列以外の要素（int, bool 等）を含む混在スライスでも、redaction 処理がパニックせず既存の型変換仕様（`[]any` への変換、`processSlice` のドキュメントコメント参照）を維持する。

### F-003: Slack webhook URL のエラーログ漏洩防止

`internal/logging/slack_handler.go` の Slack 送信失敗時のログ出力（リクエスト作成失敗・送信失敗等）で、webhook URL を含み得るエラー値をそのまま `slog.Any` で渡さず、URL を含まない形に要約してから記録する。

**Acceptance Criteria**:
- **AC-09**: webhook URL 疎通不可（DNS 解決失敗・接続拒否等、`*url.Error` を伴う失敗）を発生させた場合に出力されるログに、設定された webhook URL の文字列（トークン部分を含むパス）が平文で含まれない。
- **AC-10**: エラーの種別（タイムアウト、DNS エラー、接続拒否等の判別に資する情報）は URL を除いた形でログに残り、運用上のトラブルシューティング情報が失われない。

### F-004: `LogUserGroupExecution` の境界 redaction 適用

`LogUserGroupExecution` の `stdout`/`stderr`/`command_args`/`expanded_command_args` に、`LogRiskProfile` と同様の境界 redaction（`argRedactor.RedactText`）を適用する。

**Acceptance Criteria**:
- **AC-11**: `RedactingHandler` を経由しない素の `slog.Logger`（`NewAuditLoggerWithCustom` 等のテスト専用コンストラクタで注入）を使って `LogUserGroupExecution` を呼び出し、`result.Stderr` に `password=hunter2` を含めて実行を失敗させた場合、出力される `stderr` 属性の値に `hunter2` が平文で含まれない。
- **AC-12**: 同様の条件で `cmd.Args()`/`cmd.ExpandedArgs` に機密パターンを含む引数を渡した場合、`command_args`/`expanded_command_args` 属性の値がマスクされる。
- **AC-13**: 機密パターンを含まない stdout/stderr/引数は、redaction 適用後も内容が変化しない（正常系の監査ログが読みづらくならない）。

### F-005: `LogPrivilegeEscalation`・`LogSecurityEvent` の境界 redaction 適用

`LogPrivilegeEscalation` の `operation`/`commandName`、`LogSecurityEvent` の `message` に、同様の境界 redaction を適用する。

**Acceptance Criteria**:
- **AC-14**: `RedactingHandler` を経由しない `slog.Logger` を使って `LogPrivilegeEscalation` を呼び出し、`commandName` に機密パターンを含めた場合、出力される `command_name` 属性がマスクされる。
- **AC-15**: 同条件で `LogSecurityEvent` の `message` 引数に機密パターンを含めた場合、出力される `message` 属性がマスクされる。
- **AC-16**: 機密パターンを含まない値は redaction 適用後も内容が変化しない。

### F-006: `LogSecurityEvent` の `details` map の redaction とキー衝突防止

`details map[string]any` の各値に境界 redaction を適用し、かつキーをスキーマ予約キーと衝突しない名前空間（prefix 付与等）に隔離する。

**Acceptance Criteria**:
- **AC-17**: `RedactingHandler` を経由しない `slog.Logger` を使って `LogSecurityEvent` を呼び出し、`details` に機密パターンを含む値（例: `map[string]any{"payload": "api_key=secret123"}`）を渡した場合、出力される対応属性の値に `secret123` が平文で含まれない。
- **AC-18**: `details` に `severity`（または `audit_type`/`slack_notify`/`decision` 等の既存スキーマキー）と同名のキーを渡しても、出力されるログの当該スキーマキーの値が `details` 由来の値で上書き・重複出力されない（キー名が予約名と衝突しない形で出力される）。
- **AC-19**: 機密パターンを含まない `details` の値は redaction 適用後も内容が判別可能な形でログに残る（完全に読めなくなるような過剰隠蔽をしない）。

### F-007: `SanitizeEnvironmentVariables` の値ベース redaction

`SanitizeEnvironmentVariables` はキー名判定に加えて、値の内容に対しても redaction（`RedactText` および/または `ValueDetector` による値形式検出）を適用する。また `sensitiveEnvRegexps` の大文字小文字非対称性を解消する。

**Acceptance Criteria**:
- **AC-20**: キー名が既知の機密パターンに一致しない環境変数（例: `CONFIG_BLOB`）でも、値が機密形式（PEM 秘密鍵ヘッダ、JWT 等、既存の `ValueDetector`/`SensitivePatterns` が検出可能な形式）に一致する場合、`SanitizeEnvironmentVariables` の戻り値でその値がマスクされる。
- **AC-21**: キー名・値ともに機密パターンに一致しない環境変数は、`SanitizeEnvironmentVariables` 適用後も値が変化しない。
- **AC-22**: `Config.SensitiveEnvVars` に小文字のカスタムパターンを設定した場合でも、対応する環境変数キーが正しく機密と判定される（大文字小文字の非対称性が解消される）。

## 非機能要件

- 本タスクの各修正は、`internal/redaction`・`internal/logging`・`internal/runner/base/audit`・`internal/runner/base/security` の既存の公開 API シグネチャ（`RedactingHandler`, `Logger`, `Validator` の各メソッド）を変更しない。呼び出し側のコード変更を要求しない形で redaction の適用範囲のみを拡張する。
- 既存の `maxRedactionDepth`（DoS 防止のための再帰深度制限）の設計方針を、新たに追加する map/struct 再帰処理にも適用する。
- 既存の fail-secure 方針（redaction 失敗時は `RedactionFailurePlaceholder` 等の安全側フォールバックを用いる）を、新規追加する処理経路にも一貫して適用する。

## 参考: 既存の良好な防御層（変更しないもの）

- `LogRiskProfile` の境界 redaction（`argRedactor.RedactText` を `command_args`/`operand_zones` に適用）は既存のまま維持し、他メソッドをこれに合わせる形で拡張する。
- `RedactingHandler` の fail-secure 方針（regex compile 失敗・再帰深度超過・LogValue() panic のいずれも安全側プレースホルダーへ置換）、failureLogger の循環依存検証、Webhook URL 宛先検証の fail-closed 方針は本タスクの対象外であり変更しない。
