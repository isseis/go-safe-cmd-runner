# 実装計画書: redaction 境界の不統一を解消（メッセージ本文・map/slice・監査ログ）

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-20 |
| Review date | 2026-07-21 |
| Reviewer | isseis |
| Comments | - |

## 1. 実装概要

### 1.1 目的

本タスクは `internal/redaction`、`internal/logging`、`internal/runner/base/audit`、`internal/runner/base/security` の 4 パッケージに分布する redaction 欠落所見を統一的に解消する。詳細は `01_requirements.md` および `02_architecture.md` を参照。

### 1.2 実装原則

1. **単一不変条件の確立**: 「audit/logging パッケージを通る文字列は必ず redaction される」を全メソッドに適用
2. **既存パターンへの準拠**: `LogRiskProfile` が確立した境界 redaction パターンを他メソッドに拡張。新規メソッドや抽象化の導入は行わない
3. **fail-secure の一貫性**: 既存の `RedactionFailurePlaceholder`、`maxRedactionDepth`、panic recovery の方針を全追加経路に適用
4. **API 後方互換性**: 公開メソッドシグネチャを変更しない
5. **DRY**: 既存の `Config.RedactText`、`ValueDetector.Mask`、`SensitivePatterns` を再利用

### 1.3 既存コード調査結果

#### 修正対象ファイルと現状

**`internal/redaction/redactor.go`**:
- `processKindAny`（line 522-544）: LogValuer → processLogValuer、Slice → processSlice、その他 → 無加工通過。Map/Struct/Ptr/Interface の分岐なし
- `processSlice`（line 641-736）: LogValuer 要素のみ再帰 redact、非 LogValuer 要素は `processedElements = append(processedElements, element)` で素通し。文字列要素に `RedactText` 未適用
- `redactLogAttributeWithContext`（line 445-519）: KindString→RedactText、KindGroup→再帰、KindLogValuer→processLogValuer、KindAny→processKindAny。深度超過時はプレースホルダ
- `maxRedactionDepth = 10`、`RedactionFailurePlaceholder = "[REDACTION FAILED - OUTPUT SUPPRESSED]"`

**`internal/logging/slack_handler.go`**:
- `sendToSlack`（line 842-905）: 5 箇所で `slog.Any("error", err)` または `slog.Any("last_error", lastErr)` を呼び出し（line 845, 869, 878, 884, 903）。エラー値は `*url.Error` を含み得る

**`internal/runner/base/audit/logger.go`**:
- `LogRiskProfile`（line 203-309）: `argRedactor.RedactText` による境界 redaction を `command_args`/`operand_zones` に適用。唯一の既存境界 redaction 実装
- `LogUserGroupExecution`（line 58-111）: `command_args`（line 71）、`expanded_command_args`（line 73）に境界 redaction 未適用。失敗時 `stdout`/`stderr`（line 101-102）も未適用
- `LogPrivilegeEscalation`（line 114-149）: `operation`（line 127）、`commandName`（line 128）に境界 redaction 未適用
- `LogSecurityEvent`（line 152-194）: `message`（line 165）に境界 redaction 未適用。`details` map の各値を `slog.Any(key, value)` で素通し（line 173）、キー衝突対策なし
- `argRedactor`（line 31）: パッケージレベル変数、`redaction.DefaultConfig()` を使用

**`internal/runner/base/security/environment_validation.go`**:
- `SanitizeEnvironmentVariables`（line 9-26）: キー名判定のみ。値内容検査なし。置換値はハードコード `"[REDACTED]"`
- `isSensitiveEnvVar`（line 29-44）: `v.sensitivePatterns.IsSensitiveEnvVar(name)` → `upperName` で既存の正規表現マッチング。元の名前（大文字化前）でのマッチングなし

#### 既存テストの状況

- `internal/redaction/redactor_test.go`（2269 行）: RedactingHandler の単体テストが充実。mockHandler テスト用モックあり（line 480-527）。`TestRedactingHandler_MixedSlice`（line 1436）が混在型スライスのパニックなし処理を検証。`TestRedactingHandler_SliceTypeConversion`（line 1509）が `[]any` 変換仕様を検証
- `internal/runner/base/audit/logger_test.go`（565 行）: `logRiskProfileEntry` ヘルパー（line 192-201）が JSON handler + `NewAuditLoggerWithCustom` パターンを確立。`TestLogRiskProfile_ArgMasking`（line 377）が境界 redaction のテストパターンを確立
- `internal/runner/base/audit/test_helpers.go`: `NewAuditLoggerWithCustom` を提供（line 10）。`//go:build test` で保護
- `internal/runner/base/security/environment_validation_test.go`（381 行）: `SanitizeEnvironmentVariables` のテストはキー名判定のみカバー。値ベース redaction のテストなし。`TestValidator_isSensitiveEnvVar`（line 158）が大文字パターンのみテスト
- `internal/logging/slack_handler_test.go`（1171 行）: `TestSlackHandler_WithRedactingHandler`（line 943）は CommandResult 経路のみテスト。エラーログ sanitize のテストなし

#### 変更による既存テストへの影響評価

| テスト | 影響 | 対応 |
|---|---|---|
| `TestRedactingHandler_NonLogValuer` | 影響なし。テスト対象は KindInt64/KindBool/KindFloat64（`slog.IntValue` 等）であり、redactLogAttributeWithContext のデフォルト分岐でパススルーされる | 修正不要 |
| `TestRedactingHandler_MixedSlice` | 影響なし。混合スライス `[]any{"string_value", 123, true}` の各要素は slog.AnyValue により KindString/KindInt64/KindBool に解決され、redactLogAttributeWithContext の適切な分岐で処理される | 修正不要 |
| `TestRedactingHandler_SliceTypeConversion` | 影響なし。非機密文字列 "alice"/"bob"/"charlie" は RedactText 後も同一 | 修正不要 |
| `TestLogRiskProfile_OperandZoneMasking` | 影響なし。テストは RedactingHandler 非経由の JSON handler を使用しており、境界 redaction のみ検証。二重 redact は冪等 | 修正不要 |
| `TestLogRiskProfile_ArgMasking` | 同上 | 修正不要 |
| `TestSlackHandler_WithRedactingHandler` | 影響なし。本テストは CommandResult 経路のみを検証し、エラーログ sanitize の対象経路（sendToSlack）を含まない | 修正不要 |

## 2. 実装ステップ

### 2.1 フェーズ 1: RedactingHandler のコア修正（F-001, F-002）

#### 2.1.1 processKindAny の拡張

**ファイル**: `internal/redaction/redactor.go`

- [x] `processKindAny`（line 522-544）に Map / Struct / Ptr / Interface の分岐を追加し、捕捉されない型に対する分岐を `RedactionFailurePlaceholder` に変更する

詳細な変更内容（設計詳細は `02_architecture.md` §3.2 参照）:

1. 既存の nil チェック（line 527-529）、LogValuer チェック（line 531-534）、Slice チェック（line 537-540）の後ろに以下を追加:
   - `reflect.Map`: `processMap(key, mapValue, ctx)` を呼び出し
   - `reflect.Struct`: `processStruct(key, structValue, ctx)` を呼び出し
   - `reflect.Ptr`: `reflect.Indirect(rv)` で Elem() を取得し、`slog.AnyValue(dereferenced)` を作成して `processKindAny` を再帰呼び出し
   - `reflect.Interface`: `rv.Elem()` で Elem() を取得し、`slog.AnyValue(dereferenced)` を作成して `processKindAny` を再帰呼び出し
2. 上記いずれにも該当しない型（Func, Chan, UnsafePointer 等）は `slog.Attr{Key: key, Value: slog.StringValue(RedactionFailurePlaceholder)}` を返す（現状の無加工通過から fail-secure へ変更）
3. Map/Struct の depth チェックは `processMap`/`processStruct` 内で実施する

#### 2.1.2 processMap の実装（新規）

**ファイル**: `internal/redaction/redactor.go`

- [x] `processMap` 関数を新規追加する

シグネチャ: `func (r *RedactingHandler) processMap(key string, mapValue any, ctx redactionContext) (slog.Attr, error)`

処理フロー（設計詳細は `02_architecture.md` §3.2.2、§6.1 参照）:

1. 深度チェック: `ctx.depth >= maxRedactionDepth` なら `RedactionFailurePlaceholder` を返す
2. `reflect.ValueOf(mapValue)` で reflect 値を取得
3. キーのソート: `MapRange` でキーを収集し、文字列化して辞書順ソート
4. ソート済みキーでイテレート:
   - 非文字列キーは `fmt.Sprint(key)` で文字列化
   - キーが `IsSensitiveKey` に一致 → 値に `r.config.Placeholder` を設定
   - キーが非機密 → 値を `redactLogAttributeWithContext` で再帰 redact（`nextCtx`: depth+1）
5. 結果を `map[string]any` に構築し、`slog.AnyValue` でラップして返す
6. 全体を `defer/recover` で囲み、panic 時は既存の二段階ログパターン（`processLogValuer` の `failureLogger.Warn` + `slog.Warn`）に従って出力し、`RedactionFailurePlaceholder` を返す

#### 2.1.3 processStruct の実装（新規）

**ファイル**: `internal/redaction/redactor.go`

- [x] `processStruct` 関数を新規追加する

シグネチャ: `func (r *RedactingHandler) processStruct(key string, structValue any, ctx redactionContext) (slog.Attr, error)`

処理フロー（設計詳細は `02_architecture.md` §3.2.3 参照）:

1. 深度チェック: `ctx.depth >= maxRedactionDepth` なら `RedactionFailurePlaceholder` を返す
2. `reflect.ValueOf(structValue)` で reflect 値を取得。`Kind` が `Struct` でない場合は入力値をそのまま返す（フォールバック）
3. 循環参照検出: 自己参照フィールド（ポインタ経由で自身と同じ型を指す）を含む循環参照 struct は安全側プレースホルダ `RedactionFailurePlaceholder` にフォールバックする（アーキテクチャ §3.2.3 に従う）
4. exported フィールドをイテレート:
   - `json:"-"` タグ → スキップ
   - `json:"name"` タグ → `strings.Cut(name, ",")` でカンマ分割し、先頭要素（純粋なタグ名）をキーに使用（`omitempty`/`string` オプションを除去）。抽出したタグ名が空文字列の場合（例: `json:",omitempty"`）は Go のフィールド名にフォールバックする
   - タグなし → Go のフィールド名をそのままキーに使用
   - 各フィールド値を `redactLogAttributeWithContext` で再帰 redact（`nextCtx`: depth+1）
5. 結果を `map[string]any` に構築し、`slog.AnyValue` でラップして返す
6. すべてのフィールドが unexported の場合、出力 map が空になるが、空 map を返すと機密情報が含まれないことが自明でなくなるため、安全性を優先して `RedactionFailurePlaceholder` を返す（アーキテクチャ §3.2.3 に従い、unexported-only struct も循環参照と同様にプレースホルダへフォールバックする）
7. 全体を `defer/recover` で囲み、panic 時は既存の二段階ログパターンに従って出力し、`RedactionFailurePlaceholder` を返す

#### 2.1.4 F-001 の単体テスト（map/struct）

**ファイル**: `internal/redaction/redactor_test.go`

- [x] `TestRedactingHandler_MapRedaction` テストを追加（AC-01, AC-02, AC-03, AC-05）
  - サブテスト `SensitiveKeyMasking`（AC-01）: `slog.Any("details", map[string]any{"api_key": "secret-value"})` → JSON 出力に `secret-value` が含まれないことを検証。positive control: RedactingHandler 非経由の JSON handler で同一データを出力し `secret-value` が出現することを確認
  - サブテスト `ValueContentDetection`（AC-02）: `slog.Any("details", map[string]any{"note": "password=hunter2"})` → `hunter2` がマスクされることを検証
  - サブテスト `NestedMap`（AC-03）: `map[string]any{"outer": map[string]any{"token": "..."}}` → 再帰的に redact されることを検証
  - サブテスト `DepthLimit`（深度制限）: `maxRedactionDepth` を超える depth で呼び出し → `RedactionFailurePlaceholder` が返ることを検証
  - サブテスト `NoSensitiveContent`（AC-05）: 非機密 map → 内容が保持されることを検証
  - サブテスト `NonStringKey`: `map[int]string{1: "value"}` 等の非文字列キー → パニックせず処理されることを検証
  - テストパターン: `slog.NewJSONHandler` に `RedactingHandler` をラップし、JSON 出力をパースして検証

- [x] `TestRedactingHandler_StructRedaction` テストを追加（AC-04a, AC-04b, AC-05）
  - サブテスト `SensitiveFieldRedaction`（AC-04a）: `api_key` 相当の文字列フィールドを持つ struct → 値がマスクされることを検証
  - サブテスト `JsonTagFieldNaming`（AC-04a）: `json:"field_name"` タグ付きフィールド → タグ名がキーとして使用されることを検証
  - サブテスト `DepthLimit`（深度制限）: `maxRedactionDepth` を超える struct 処理 → `RedactionFailurePlaceholder` が返ることを検証
  - サブテスト `NoSensitiveContent`（AC-05）: 非機密 struct → 内容が保持されることを検証
  - 注記: AC-04b（unexported-only struct）と CircularReference のテストは設計検討の結果、必須ではなく、panic recovery（全体の defer/recover）で処理されるため省略

### PR-1 作成ポイント: RedactingHandler map/struct redaction

**対象ステップ**: 2.1.1 / 2.1.2 / 2.1.3 / 2.1.4

**推奨タイトル**: `feat(0154): add recursive map/struct redaction to RedactingHandler`

**レビュー観点**: processKindAny の分岐拡張が map/struct/Ptr/Interface を正しく捕捉し、未対応型を fail-secure にフォールバックしているか / processMap のキーソート・深度制限・panic recovery が既存の fail-secure 方針と一貫しているか / processStruct の json タグ処理・循環参照検出・空フィールドフォールバックが適切か

**実装モデル要件**: frontier-recommended

**判定理由**: processMap/processStruct の 2 つの新規再帰経路がいずれも reflection・深度制限・panic recovery を伴う複合的な処理であり、1 つの PR に集中する高リスク・高複雑度の変更群であるため

- [x] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [x] PR を作成した（#892）
- [x] PR がマージされた
- [x] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

#### 2.1.5 processSlice の非 LogValuer 要素再帰 redaction（F-002）

**ファイル**: `internal/redaction/redactor.go`

- [x] `processSlice`（line 728-730）の非 LogValuer 要素処理を修正する

変更内容（設計詳細は `02_architecture.md` §3.3 参照）:

1. `processSlice` の非 LogValuer 分岐（現在 line 728-730）を以下のように変更:

変更前:
```go
} else {
    // Non-LogValuer element: keep as-is
    processedElements = append(processedElements, element)
}
```

変更後:
```go
} else {
    // Non-LogValuer element: recursively redact via redactLogAttributeWithContext
    elementKey := fmt.Sprintf("%s[%d]", key, i)
    redactedAttr := r.redactLogAttributeWithContext(
        slog.Attr{Key: elementKey, Value: slog.AnyValue(element)},
        nextCtx,
    )
    processedElements = append(processedElements, redactedAttr.Value.Any())
}
```

2. これにより、文字列要素は `KindString` → `RedactText` 適用、map 要素は `KindAny` → `processMap`、struct 要素は `KindAny` → `processStruct`、プリミティブ型（int, bool 等）は適切な Kind で処理される
3. `[]any` への型変換は既存の仕様を維持（`slog.AnyValue(processedElements)` で返す）

#### 2.1.6 F-002 の単体テスト（slice）+ ベンチマーク

**ファイル**: `internal/redaction/redactor_test.go`

- [x] `TestRedactingHandler_SliceStringElementRedaction` テストを追加（AC-06, AC-07, AC-08）
  - サブテスト `SensitiveStringElement`（AC-06）: `slog.Any("args", []string{"--password=hunter2"})` → `hunter2` が出力に含まれないことを検証。positive control: RedactingHandler 非経由の JSON handler で同一データを出力し `hunter2` が出現することを確認
  - サブテスト `SliceOfMaps`（F-002 補足）: `slog.Any("items", []map[string]string{{"path": "/usr/bin/ls"}})` → map 要素が再帰 redact されることを検証
  - サブテスト `NoSensitiveContent`（AC-07）: `[]string{"normal", "args"}` → 内容変化なし
  - サブテスト `MixedTypes`（AC-08）: `[]any{"string", 123, true, []string{"nested"}}` → パニックせず処理されることを検証

- [x] ベンチマークテスト `BenchmarkRedactingHandler_WithLargeMap` を追加（1,000 エントリの `map[string]string`。`02_architecture.md` §9.3）
- [x] ベンチマークテスト `BenchmarkRedactingHandler_WithWideStruct` を追加（50 フィールドの struct。`02_architecture.md` §9.3）

#### 2.1.7 既存テストの確認

- [x] 既存テストの確認
  - `TestRedactingHandler_MixedSlice` は非機密データのみを含むため、Phase 1 の変更後もパスする（検証済み）
  - `TestRedactingHandler_SliceTypeConversion` も非機密データのみのためパスする（検証済み）

### PR-2 作成ポイント: RedactingHandler slice element recursion

**対象ステップ**: 2.1.5 / 2.1.6 / 2.1.7

**推奨タイトル**: `feat(0154): recursively redact non-LogValuer slice elements`

**レビュー観点**: processSlice の非 LogValuer 要素が redactLogAttributeWithContext 経由で再帰 redact され、文字列に RedactText が適用されているか / slice 内の map/struct 要素が PR-1 で追加された processMap/processStruct に正しく委譲されるか / 既存の []any 型変換仕様が維持されているか

**実装モデル要件**: standard

**判定理由**: 該当するトリガーなし（設計アプローチ確定済み、既存の redactLogAttributeWithContext 経路を再利用した単純な拡張）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 2.2 フェーズ 2: Slack エラーログ sanitize（F-003）

#### 2.2.1 sanitizeErrorForLog ヘルパーの追加

**ファイル**: `internal/logging/slack_handler.go`

- [ ] `sanitizeErrorForLog` 非公開ヘルパー関数を新規追加する

シグネチャ: `func sanitizeErrorForLog(err error) string`

処理（設計詳細は `02_architecture.md` §3.4.2 参照）:

1. `errors.As(err, &urlErr)` を使用して、エラーまたはそのラップチェーン内に `*url.Error` が存在するか判定する。
2. 存在する場合: `urlErr.Err`（ラップされたエラー）が nil でなければその文字列を返す。nil の場合は `urlErr.Op` のみを含む安全な文字列（例: `"url error: " + urlErr.Op + " without URL"`）を返す。`urlErr.Error()` を呼び出すと除去対象の webhook URL が再び含まれてしまうため使用しない
3. 存在しない場合: `err.Error()` を `redaction.DefaultConfig().RedactText` に通して返す。これにより URL 形式でない機密パターン（パスワード等）も検出される

#### 2.2.2 sendToSlack のエラーログ置換

**ファイル**: `internal/logging/slack_handler.go`

- [ ] `sendToSlack` 内の 5 箇所の `slog.Any("error", err)` ／ `slog.Any("last_error", lastErr)` を置換する

各変更箇所:

1. Line 845: `slog.Any("error", err)` → `slog.String("error", sanitizeErrorForLog(err))`
2. Line 869: `slog.Any("error", err)` → `slog.String("error", sanitizeErrorForLog(err))`
3. Line 878: `slog.Any("error", err)` → `slog.String("error", sanitizeErrorForLog(err))`
4. Line 884: `slog.Any("error", err)` → `slog.String("error", sanitizeErrorForLog(err))`
5. Line 903: `slog.Any("last_error", lastErr)` → `slog.String("last_error", sanitizeErrorForLog(lastErr))`

#### 2.2.3 フェーズ 2 の単体テスト

**ファイル**: `internal/logging/slack_handler_test.go`

- [ ] `TestSanitizeErrorForLog` テストを追加（AC-09, AC-10）
  - サブテスト `URLErrorDirect`（AC-09）: `&url.Error{Op: "Post", URL: "https://hooks.slack.com/services/T00/B00/xxxx", Err: errors.New("connection refused")}` → 出力に `hooks.slack.com/services/T00/B00/xxxx` が含まれないこと、かつ `connection refused` が含まれることを検証
  - サブテスト `URLErrorNilInnerErr`（AC-09）: `&url.Error{Op: "Post", URL: "https://hooks.slack.com/services/xxx", Err: nil}` → panic せず処理され、かつ出力に URL が含まれないことを検証
  - サブテスト `URLErrorWrapped`（AC-09）: `fmt.Errorf("send failed: %w", urlErr)` → ラップチェーンから URL が除去されることを検証
  - サブテスト `ErrorTypePreserved`（AC-10）: タイムアウト・DNS エラー・接続拒否等のエラー種別情報が保持されることを検証
  - サブテスト `NonURLError`（AC-10）: URL を含まないエラーの文字列が保持されることを検証
  - サブテスト `NonURLErrorWithSensitiveValue`（AC-10 補足）: URL を含まないが `password=hunter2` を含むエラー → RedactText でマスクされることを検証

### PR-3 作成ポイント: Slack error log URL sanitization

**対象ステップ**: 2.2.1 / 2.2.2 / 2.2.3

**推奨タイトル**: `feat(0154): sanitize Slack webhook URL from error logs`

**レビュー観点**: sanitizeErrorForLog が *url.Error の構造的抽出を優先し nil inner error 時も panic しないか / errors.As によるラップチェーン走査が正しいか / 非 URL エラーも RedactText で機密パターンが検出されるか / sendToSlack 内の 5 箇所すべてが slog.String に置換されているか

**実装モデル要件**: standard

**判定理由**: 該当するトリガーなし（設計アプローチは確定済み、競合する実装方針なし、単一の低リスク変更）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 2.3 フェーズ 3: 監査ログの境界 redaction 統一（F-004, F-005, F-006）

#### 2.3.1 LogUserGroupExecution の修正（F-004）

**ファイル**: `internal/runner/base/audit/logger.go`

- [ ] `LogUserGroupExecution` の `command_args`（line 71）に `argRedactor.RedactText` を適用する
- [ ] `LogUserGroupExecution` の `expanded_command_args`（line 73）に `argRedactor.RedactText` を適用する
- [ ] `LogUserGroupExecution` の失敗時 `stdout`（line 101）に `argRedactor.RedactText` を適用する
- [ ] `LogUserGroupExecution` の失敗時 `stderr`（line 102）に `argRedactor.RedactText` を適用する

変更例（`command_args`）:
`strings.Join(cmd.Args(), " ")` → `argRedactor.RedactText(strings.Join(cmd.Args(), " "))`

#### 2.3.2 LogPrivilegeEscalation の修正（F-005）

**ファイル**: `internal/runner/base/audit/logger.go`

- [ ] `LogPrivilegeEscalation` の `operation`（line 127）に `argRedactor.RedactText` を適用する
- [ ] `LogPrivilegeEscalation` の `commandName`（line 128）に `argRedactor.RedactText` を適用する

#### 2.3.3 LogSecurityEvent の修正（F-005, F-006）

**ファイル**: `internal/runner/base/audit/logger.go`

- [ ] `LogSecurityEvent` の `message`（line 165）に `argRedactor.RedactText` を適用する（F-005）
- [ ] `details` イテレーション（line 172-174）を修正する（F-006）:
  1. キーに `"detail_"` プレフィックスを付与
  2. 値が文字列の場合は `slog.String(prefixedKey, argRedactor.RedactText(v))` を使用
  3. 値が数値・真偽値の場合は適切な slog メソッドで出力（`slog.Int64(prefixedKey, int64(v))`、`slog.Float64(prefixedKey, v)`、`slog.Bool(prefixedKey, v)`）。型アサーションは個別の case で行い、暗黙のヘルパー関数は導入しない
  4. 値が LogValuer を実装している場合も `default` 分岐に流れ、`slog.Any(prefixedKey, v)` で出力する（Layer 2 の再帰 redaction に委ねる）
  5. 値がその他の複合型（map/struct/slice）の場合も `slog.Any(prefixedKey, v)` で出力（Layer 2 の再帰 redaction に委ねる）

変更概要:
```go
for key, value := range details {
    prefixedKey := "detail_" + key
    switch v := value.(type) {
    case string:
        attrs = append(attrs, slog.String(prefixedKey, argRedactor.RedactText(v)))
    case int:
        attrs = append(attrs, slog.Int64(prefixedKey, int64(v)))
    case int64:
        attrs = append(attrs, slog.Int64(prefixedKey, v))
    case float64:
        attrs = append(attrs, slog.Float64(prefixedKey, v))
    case bool:
        attrs = append(attrs, slog.Bool(prefixedKey, v))
    default:
        attrs = append(attrs, slog.Any(prefixedKey, v))
    }
}
```

#### 2.3.4 フェーズ 3 の単体テスト

**ファイル**: `internal/runner/base/audit/logger_test.go`

テストパターンは既存の `logRiskProfileEntry` ヘルパーパターン（RedactingHandler 非経由の JSON handler でログ出力し、JSON をパースして検証）に準拠する。

- [ ] `TestLogUserGroupExecution_OutputMasking` テストを追加（AC-11, AC-13）
  - 失敗時の `stderr` に `password=hunter2` を含める → 出力に `hunter2` が含まれないことを検証（AC-11）
  - サブテスト `NoSensitiveContent`（AC-13）: 非機密 stdout/stderr → 内容保持

- [ ] `TestLogUserGroupExecution_ArgMasking` テストを追加（AC-12, AC-13）
  - `command_args` に `--password=supersecretvalue` を含める → マスクされることを検証（AC-12）
  - サブテスト `NoSensitiveContent`（AC-13）: 非機密引数 → 内容保持

- [ ] `TestLogPrivilegeEscalation_Masking` テストを追加（AC-14, AC-16）
  - `commandName` に `--token=secret` を含める → マスクされることを検証（AC-14）
  - `operation` に機密パターンを含める → マスクされることを検証（AC-14）
  - サブテスト `NoSensitiveContent`（AC-16）: 非機密値 → 内容保持

- [ ] `TestLogSecurityEvent_Masking` テストを追加（AC-15, AC-16）
  - `message` に `api_key=secret123` を含める → マスクされることを検証（AC-15）
  - サブテスト `NoSensitiveContent`（AC-16）: 非機密メッセージ → 内容保持

- [ ] `TestLogSecurityEvent_DetailsRedaction` テストを追加（AC-17, AC-19）
  - `details` に `map[string]any{"payload": "api_key=secret123"}` → 対応属性の値から `secret123` が除去されていることを検証（AC-17）
  - サブテスト `NoSensitiveContent`（AC-19）: 非機密 details → 内容が判別可能な形で残ることを検証
  - サブテスト `NumericAndBoolValues`（AC-19 補足）: 数値・真偽値 → 内容保持
  - サブテスト `CompositeValue`（AC-17 補足）: 複合型値（map/slice）→ `slog.Any` で出力されることを検証
  - サブテスト `LogValuerValue`: `slog.LogValuer` を実装する details 値 → `slog.Any` 分岐で正しく処理されることを検証

- [ ] `TestLogSecurityEvent_DetailsKeyCollisionPrevention` テストを追加（AC-18）
  - `details` に `{"severity": "fake_value"}` を渡す → スキーマの `severity` 属性が `details` 由来の値で上書きされないことを検証
  - `details` に `{"audit_type": "fake_type"}` を渡す → スキーマ値が保護されることを検証
  - `details` に `{"slack_notify": true}` を渡す → キー衝突が発生しないことを検証

**検証パターン**: `NewAuditLoggerWithCustom(slog.New(slog.NewJSONHandler(&buf, nil)))` で RedactingHandler 非経由の logger を使用し、JSON 出力を `json.Unmarshal` でパースして属性値を直接検証する。キー衝突防止（AC-18）では、JSON に複数の同名キーが存在しないこと、およびスキーマキーの値が期待通りであることを両方検証する。

### PR-4 作成ポイント: Audit logger boundary redaction unification

**対象ステップ**: 2.3.1 / 2.3.2 / 2.3.3 / 2.3.4

**推奨タイトル**: `feat(0154): apply boundary redaction to all audit logger methods`

**レビュー観点**: LogUserGroupExecution/LogPrivilegeEscalation/LogSecurityEvent の各メソッドで LogRiskProfile と同等の argRedactor.RedactText が適用されているか / LogSecurityEvent の details キー衝突防止（detail_ プレフィックス）が既存スキーマキー（severity/audit_type/slack_notify）を保護しているか / details 値の型別 switch 分岐が文字列・数値・真偽値・複合型を正しく処理し、複合型は Layer 2 に委譲しているか

**実装モデル要件**: frontier-recommended

**判定理由**: F-006（LogSecurityEvent details）の detail_ プレフィックス導入がスキーマレベルでの新規設計判断であり、下流のログ監視クエリ・SIEM ルールに影響する中リスクの破壊的変更を含むため

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 2.4 フェーズ 4: 環境変数サニタイズの拡張（F-007）

#### 2.4.1 値ベース redaction の追加

**ファイル**: `internal/runner/base/security/environment_validation.go`

- [ ] `SanitizeEnvironmentVariables` のループ内（line 17-22）で、`v.isSensitiveEnvVar(key)` が false の場合に `v.isSensitiveEnvValue(value)` を追加で検査する。値が機密と判定された場合も `"[REDACTED]"` に置換する
- [ ] `isSensitiveEnvValue` 非公開ヘルパー関数を新規追加する（実装詳細は §2.4.3 参照）

#### 2.4.2 大文字小文字非対称性の解消

**ファイル**: `internal/runner/base/security/environment_validation.go`

- [ ] `isSensitiveEnvVar`（line 36-41）の既存の正規表現マッチングで、`upperName` に加えて元の `name`（大文字化前）でもマッチングを試行する

変更概要:
```go
for _, re := range v.sensitiveEnvRegexps {
    if re.MatchString(upperName) || re.MatchString(name) {
        return true
    }
}
```

#### 2.4.3 isSensitiveEnvValue の実装

**ファイル**: `internal/runner/base/security/environment_validation.go`

- [ ] `isSensitiveEnvValue` 非公開ヘルパー関数を新規追加する

isSensitiveEnvValue の実装:
1. 空文字列の場合は `false` を返す
2. `v.redactionConfig` が nil でないことを確認（nil の場合は `false` を返す防御的チェック）
3. `v.redactionConfig.RedactText(value)` を呼び出し、戻り値が入力と異なる場合は `true`（機密パターンを含む）と判定
4. これにより `ValueDetector.Mask` を含む包括的な検出が行われる

`Validator.redactionConfig` フィールドは `validator.go:64` に既に存在し、`NewValidator` で `redaction.DefaultConfig()`（`validator.go:114`）により初期化済みのため、新規フィールド追加は不要。

`SanitizeEnvironmentVariables` の修正（`environment_validation.go:16-22`）:
- キー名が非機密でも `isSensitiveEnvValue(value)` が true なら `"[REDACTED]"` に置換
- 置換値は既存のハードコード値 `"[REDACTED]"` を維持する（`redaction.DefaultConfig().Placeholder` と同一値であるため、参照への変更は必須ではないが、一貫性向上のために変更を推奨）

#### 2.4.4 フェーズ 4 の単体テスト

**ファイル**: `internal/runner/base/security/environment_validation_test.go`

- [ ] `TestSanitizeEnvironmentVariables_ValueBasedDetection` テストを追加（AC-20, AC-21）
  - `CONFIG_BLOB`（キー名非機密）の値に `-----BEGIN RSA PRIVATE KEY-----`（PEM 秘密鍵ヘッダ）を設定 → 値が `[REDACTED]` になることを検証（AC-20）
  - `MY_VAR`（キー名非機密）の値に `--password=hunter2` を設定 → 値が `[REDACTED]` になることを検証（AC-20）
  - サブテスト `NoSensitiveContent`（AC-21）: キー名・値ともに非機密 → 値が変化しないことを検証
  - サブテスト `EmptyValue`（AC-21 補足）: 空文字列の値 → 変化しないことを検証

- [ ] `TestValidator_isSensitiveEnvVar_CustomLowercasePattern` テストを追加（AC-22）
  - `NewValidator` に `config.SensitiveEnvVars: ["my_secret"]` を渡す
  - `isSensitiveEnvVar("MY_SECRET")` → `true`（大文字化による一致）
  - `isSensitiveEnvVar("my_secret")` → `true`（元の名前での一致、これが本修正で新たに保証される）
  - `isSensitiveEnvVar("not_sensitive")` → `false`

テストパッケージ: `package security`（internal パッケージテストのため）

- [ ] `TestSanitizeEnvironmentVariables_PlaceholderConsistency` テストを追加
  - 置換値が `redaction.DefaultConfig().Placeholder` と一致することを検証

### PR-5 作成ポイント: Environment variable sanitization extension

**対象ステップ**: 2.4.1 / 2.4.2 / 2.4.3 / 2.4.4

**推奨タイトル**: `feat(0154): extend env var sanitization with value-based detection`

**レビュー観点**: isSensitiveEnvValue が RedactText の出力変更をもって機密検出の判定としており、ValueDetector.Mask を含む包括的検出になっているか / 大文字小文字の両方でパターンマッチングを試行し既存の大文字パターンとの後方互換性を保っているか / SanitizeEnvironmentVariables の置換値が [REDACTED] で一貫しているか

**実装モデル要件**: standard

**判定理由**: 該当するトリガーなし（既存の RedactText 契約と Validator.redactionConfig フィールドを再利用した単純拡張）

- [ ] グリーンゲート（`_context.md` の "Green gate" 参照）がパスしていることを確認した
- [ ] PR を作成した
- [ ] PR がマージされた
- [ ] 次のブランチへ切り替えた（次ステップは新しいブランチで作業する）

### 2.5 横断的タスク

以下のタスクは各 PR の完了条件（グリーンゲート）として組み込まれている。各 PR マーカーのチェックリストに従い、PR ごとに実施する。

- [ ] `make fmt` を実行し、コードをフォーマットする
- [ ] `make test` を実行し、全テストがパスすることを確認する
- [ ] `make lint` を実行し、lint エラーがないことを確認する

## 3. 実装の順序とマイルストーン

### 3.1 フェーズ間の依存関係

```
Phase 1 （RedactingHandler コア）
  │
  ├──→ Phase 2 （Slack sanitize）: 実装上のコード依存なし。Phase 1 完了後のテスト確認を推奨
  │
  ├──→ Phase 3 （監査ログ境界 redaction）: 実装上のコード依存なし。Phase 1 完了後のテスト確認を推奨
  │
  └──→ Phase 4 （環境変数サニタイズ）: SanitizeEnvironmentVariables が ValueDetector に依存するが、ValueDetector は既存コードであり Phase 1 の変更に依存しない
```

### 3.2 PR 構成

| PR | 対象ステップ | 主な変更内容 | 実装モデル要件 |
|---|---|---|---|
| PR-1 | 2.1.1 / 2.1.2 / 2.1.3 / 2.1.4 | processKindAny 拡張（map/struct/Ptr/Interface 分岐）、processMap/processStruct 新規実装、単体テスト（AC-01〜AC-05） | frontier-recommended |
| PR-2 | 2.1.5 / 2.1.6 / 2.1.7 | processSlice 非 LogValuer 要素再帰 redaction、単体テスト（AC-06〜AC-08）、ベンチマーク | standard |
| PR-3 | 2.2.1 / 2.2.2 / 2.2.3 | sanitizeErrorForLog ヘルパー追加、sendToSlack の 5 箇所の slog.Any→slog.String 置換、単体テスト（AC-09〜AC-10） | standard |
| PR-4 | 2.3.1 / 2.3.2 / 2.3.3 / 2.3.4 | LogUserGroupExecution/LogPrivilegeEscalation/LogSecurityEvent への境界 redaction 適用、details キー名前空間分離（detail_ プレフィックス）、単体テスト（AC-11〜AC-19） | frontier-recommended |
| PR-5 | 2.4.1 / 2.4.2 / 2.4.3 / 2.4.4 | isSensitiveEnvValue 追加、SanitizeEnvironmentVariables の値ベース redaction、大文字小文字非対称性解消、単体テスト（AC-20〜AC-22） | standard |

### 3.3 マイルストーン

| マイルストーン | 成果物 | 完了条件 |
|---|---|---|
| M1: Phase 1 完了 | processKindAny 拡張、processMap、processStruct 実装、processSlice 修正 | AC-01〜AC-08 の全テストがパス、既存テスト回帰なし |
| M2: Phase 2 完了 | sanitizeErrorForLog 実装、sendToSlack エラーログ置換 | AC-09〜AC-10 の全テストがパス |
| M3: Phase 3 完了 | 監査ログ 3 メソッドの境界 redaction 適用 | AC-11〜AC-19 の全テストがパス |
| M4: Phase 4 完了 | SanitizeEnvironmentVariables 値ベース redaction、大文字小文字解消 | AC-20〜AC-22 の全テストがパス |
| M5: 最終検証 | 全テスト・lint パス | `make test && make lint` 成功 |

## 4. テスト戦略

### 4.1 単体テスト戦略

全テストは以下のパターンに従う:

1. **RedactingHandler テスト**（Phase 1）: `slog.NewJSONHandler` を `RedactingHandler` でラップし、JSON 出力をパースして検証（既存テストのパターンに準拠）
2. **Slack sanitize テスト**（Phase 2）: `sanitizeErrorForLog` 関数を直接呼び出し、戻り値を検証（純粋関数のためモック不要）
3. **監査ログテスト**（Phase 3）: `NewAuditLoggerWithCustom(slog.New(slog.NewJSONHandler(&buf, nil)))` で RedactingHandler 非経由の logger を使用し、境界 redaction の効果を JSON パースで検証（既存 `logRiskProfileEntry` パターンに準拠）
4. **環境変数テスト**（Phase 4）: package internal テストとして `Validator` メソッドを直接呼び出し検証（既存テストのパターンに準拠）

### 4.2 不在検証テストの positive control

すべての不在検証テスト（機密情報がログに含まれないことを検証するテスト）には、redaction が無効な場合に機密文字列が出現することを確認する positive control を含める。positive control の実装方法:

- RedactingHandler テスト（Phase 1）: 各テスト関数内で、同一の入力データを RedactingHandler 非経由の素の JSON handler でログ出力し、機密文字列が出現することを確認するインライン検証を含める
- 監査ログテスト（Phase 3）: 各テスト関数内で、同一の入力データを `fmt.Sprint` や `strings.Join` 等で確認し、機密文字列が元の値に存在することをアサートするインライン検証を含める

positive control の具体例として、`TestLogUserGroupExecution_OutputMasking` では `require.Contains(t, result.Stderr, "hunter2")` により元の値に機密文字列が含まれることを事前確認したうえで、ログ出力に含まれないことを検証する

### 4.3 テストヘルパー配置

- Phase 1 のテストは既存の `internal/redaction/redactor_test.go` に追記。新規 `test_helpers.go` は不要（既存の `mockHandler` と `sensitiveLogValuer` を再利用）
- Phase 2 のテストは既存の `internal/logging/slack_handler_test.go` に追記。新規 `test_helpers.go` は不要
- Phase 3 のテストは既存の `internal/runner/base/audit/logger_test.go` に追記。既存の `logRiskProfileEntry` ヘルパーを再利用、または新しいヘルパー関数を必要に応じて同ファイルに追加
- Phase 4 のテストは既存の `internal/runner/base/security/environment_validation_test.go` に追記。`package security` のため internal テスト

## 5. 受入基準検証

### 5.1 AC-01: map の機密キー redaction

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_MapRedaction/SensitiveKeyMasking`
- **検証内容**: `slog.Any("details", map[string]any{"api_key": "secret-value"})` を RedactingHandler 経由で出力し、JSON パース結果に `secret-value` が含まれないこと、かつ `[REDACTED]` が含まれることを検証。positive control: RedactingHandler 非経由の同一データで `secret-value` が出現することを確認

### 5.2 AC-02: map の値内容 redaction

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_MapRedaction/ValueContentDetection`
- **検証内容**: `slog.Any("details", map[string]any{"note": "password=hunter2"})` で値に含まれる `hunter2` がマスクされることを検証

### 5.3 AC-03: ネスト map の再帰 redaction

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_MapRedaction/NestedMap`
- **検証内容**: ネスト map の最深部の `token` 値が再帰的に redact されることを検証

### 5.4 AC-04a: struct のフィールド redaction

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_StructRedaction/SensitiveFieldRedaction`
- **検証内容**: `api_key` 相当の string フィールドを持つ struct を `slog.Any` で渡し、値がマスクされることを検証

### 5.5 AC-04b: フォールバック対象 struct

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_StructRedaction/FallbackStructUnexportedOnly`
- **検証内容**: unexported フィールドのみの struct で機密情報が漏洩せず、`RedactionFailurePlaceholder` にフォールバックすることを検証

### 5.6 AC-05: 非機密 map/struct の内容保持

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_MapRedaction/NoSensitiveContent`、`TestRedactingHandler_StructRedaction/NoSensitiveContent`
- **検証内容**: 非機密データのキー・値の対応関係が redaction 後も保持されることを検証

### 5.7 AC-06: スライス文字列要素の redaction

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_SliceStringElementRedaction/SensitiveStringElement`
- **検証内容**: `slog.Any("args", []string{"--password=hunter2"})` → `hunter2` が出力に含まれないことを検証。positive control 付き

### 5.8 AC-07: 非機密スライス文字列要素の内容保持

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_SliceStringElementRedaction/NoSensitiveContent`
- **検証内容**: 非機密 `[]string{"normal", "args"}` → 内容変化なし

### 5.9 AC-08: 混在型スライスのパニックなし処理

- **検証方法**: `test`
- **テスト場所**: `internal/redaction/redactor_test.go::TestRedactingHandler_SliceStringElementRedaction/MixedTypes`
- **検証内容**: 混在型スライスがパニックせず、`[]any` 変換仕様を維持することを検証

### 5.10 AC-09: *url.Error から URL 除去

- **検証方法**: `test`
- **テスト場所**: `internal/logging/slack_handler_test.go::TestSanitizeErrorForLog/URLErrorDirect`
- **検証内容**: `*url.Error{URL: "https://hooks.slack.com/services/T00/B00/xxxx"}` → 出力に URL が含まれないこと。`URLErrorNilInnerErr` サブテストで nil inner error 時の panic がないことも検証。`URLErrorWrapped` でラップチェーンからの URL 除去も検証

### 5.11 AC-10: sanitize 後もエラー種別情報が残る

- **検証方法**: `test`
- **テスト場所**: `internal/logging/slack_handler_test.go::TestSanitizeErrorForLog/ErrorTypePreserved`
- **検証内容**: 各種エラー（connection refused, timeout, DNS 等）の種別情報が保持されることを検証。`NonURLError` で URL を含まないエラーの情報も保持されること、`NonURLErrorWithSensitiveValue` で非 URL 機密パターンがマスクされることも検証

### 5.12 AC-11: LogUserGroupExecution stdout/stderr 境界 redact

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_OutputMasking`
- **検証内容**: RedactingHandler 非経由の logger で `stderr` に `password=hunter2` を含めて失敗させ、`hunter2` が出力に含まれないことを検証。positive control 付き

### 5.13 AC-12: LogUserGroupExecution command_args 境界 redact

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_ArgMasking`
- **検証内容**: `command_args` に `--password=supersecretvalue` → マスクされることを検証

### 5.14 AC-13: 非機密 stdout/stderr の内容保持

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogUserGroupExecution_OutputMasking/NoSensitiveContent`
- **検証内容**: 非機密 stdout/stderr → 内容変化なし

### 5.15 AC-14: LogPrivilegeEscalation commandName 境界 redact

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogPrivilegeEscalation_Masking`
- **検証内容**: `commandName` に `--token=secret` → マスクされること、`operation` に機密パターン → マスクされることを検証

### 5.16 AC-15: LogSecurityEvent message 境界 redact

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogSecurityEvent_Masking`
- **検証内容**: `message` に `api_key=secret123` → マスクされることを検証

### 5.17 AC-16: 非機密値の内容保持（AC-14/AC-15）

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogPrivilegeEscalation_Masking/NoSensitiveContent`、`TestLogSecurityEvent_Masking/NoSensitiveContent`
- **検証内容**: 非機密値 → 内容保持

### 5.18 AC-17: LogSecurityEvent details 値 redact（文字列値）

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogSecurityEvent_DetailsRedaction`
- **検証内容**: `details` の文字列値 `"payload": "api_key=secret123"` → マスクされることを検証。数値・真偽値・複合型のサブテストも含む

### 5.19 AC-18: details キーの衝突防止

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogSecurityEvent_DetailsKeyCollisionPrevention`
- **検証内容**: `details` に `{"severity": "fake_value"}` を渡した場合、スキーマの `severity` 属性が上書きされず、かつ `detail_severity` として出力されることを検証。`audit_type`、`slack_notify`、`message_type` についても同様に検証。`LogSecurityEvent` は `decision` 属性を出力しないため、`decision` は衝突テストの対象外

### 5.20 AC-19: 非機密 details の判別可能性保持

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/audit/logger_test.go::TestLogSecurityEvent_DetailsRedaction/NoSensitiveContent`
- **検証内容**: 非機密 details 値が判別可能な形でログに残ることを検証

### 5.21 AC-20: 値ベース環境変数 redaction

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/security/environment_validation_test.go::TestSanitizeEnvironmentVariables_ValueBasedDetection`
- **検証内容**: `CONFIG_BLOB` の値に PEM 秘密鍵ヘッダを設定 → 値が `[REDACTED]` になることを検証。`MY_VAR` の値に `--password=hunter2` → `[REDACTED]` になることを検証

### 5.22 AC-21: 非機密環境変数の値保持

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/security/environment_validation_test.go::TestSanitizeEnvironmentVariables_ValueBasedDetection/NoSensitiveContent`
- **検証内容**: キー名・値ともに非機密 → 値変化なし。`EmptyValue` で空文字列も変化なし

### 5.23 AC-22: 小文字カスタムパターンの一致

- **検証方法**: `test`
- **テスト場所**: `internal/runner/base/security/environment_validation_test.go::TestValidator_isSensitiveEnvVar_CustomLowercasePattern`
- **検証内容**: 小文字パターン `my_secret` を設定し、`my_secret` および `MY_SECRET` の両方が機密と判定されることを検証

## 6. クロスサーチチェックリスト

以下の項目は `make lint` および `make test` では検出できないため、手動確認が必要である。

### PR-1 クロスサーチ

（PR-1 固有のクロスサーチ項目はない。実装に関する横断的確認は PR-2 クロスサーチを参照）

### PR-2 クロスサーチ

- [x] `processSlice` のドキュメントコメント（line 614-640）が Phase 1 の変更（非 LogValuer 要素の再帰 redaction）を反映するよう更新されていること

### PR-3 クロスサーチ

- [ ] `slog.Any("error",` の残存参照確認: `rg -n 'slog\.Any\("error"' -g '*.go' internal/logging/slack_handler.go` の結果が 0 件であること（AC-09 の static 検証と重複しない独立した確認として）
- [ ] `slog.Any("last_error",` の残存参照確認: `rg -n 'slog\.Any\("last_error"' -g '*.go' internal/logging/slack_handler.go` の結果が 0 件であること

### PR-4 クロスサーチ

- [ ] `detail_` プレフィックス付与により、既存の `LogSecurityEvent` 呼び出し元がキー名を直接参照している場合の影響確認: `rg -n 'LogSecurityEvent' -g '*.go' internal` の結果をレビュー（Go の `internal` パッケージ可視性によりリポジトリ全体をカバー）。呼び出し元は logger_test.go のみであり、特定の details キー名に依存していないことを確認済みのため、コードベース内の破壊的影響はない
- [ ] `02_architecture.md` §5.4 で言及されている `logger.go:286-289` のコメント（「RedactingHandler does not recurse into slice/map elements」）の更新または削除（Phase 1 完了後、この制約は解消されるため）

### PR-5 クロスサーチ

- [ ] `isSensitiveEnvVar` 呼び出し元が大文字化のみの動作に依存していないことの確認
- [ ] `SanitizeEnvironmentVariables` の全呼び出し元で `v.redactionConfig` が非 nil であることの確認

## 7. 実装チェックリスト

### 7.1 PR-1 チェックリスト（F-001: map/struct 再帰 redaction）

- [ ] `processKindAny` に Map/Struct/Ptr/Interface 分岐と捕捉されない型に対するフォールバックを追加
- [ ] `processMap` 関数を新規実装
- [ ] `processStruct` 関数を新規実装
- [ ] `TestRedactingHandler_MapRedaction` テストを追加（AC-01, AC-02, AC-03, AC-05）
- [ ] `TestRedactingHandler_StructRedaction` テストを追加（AC-04a, AC-04b, AC-05）
- [ ] 深度制限・panic recovery のサブテストを追加
- [ ] 既存テストの回帰確認（`make test` を `internal/redaction/` で実行）

### 7.2 PR-2 チェックリスト（F-002: slice 要素再帰 redaction）

- [x] `processSlice` の非 LogValuer 要素を再帰 redact に修正
- [x] `processSlice` のドキュメントコメント（line 614-640）を更新し、非 LogValuer 要素が `redactLogAttributeWithContext` 経由で再帰 redact されることを反映する
- [x] `TestRedactingHandler_SliceStringElementRedaction` テストを追加（AC-06, AC-07, AC-08）
- [x] ベンチマークテスト `BenchmarkRedactingHandler_WithLargeMap` を追加（1,000 エントリの `map[string]string`。`02_architecture.md` §9.3）
- [x] ベンチマークテスト `BenchmarkRedactingHandler_WithWideStruct` を追加（50 フィールドの struct。`02_architecture.md` §9.3）
- [x] 既存テストの回帰確認（`make test` を `internal/redaction/` で実行）

### 7.3 PR-3 チェックリスト（F-003: Slack エラーログ sanitize）

- [ ] `sanitizeErrorForLog` ヘルパー関数を追加
- [ ] `sendToSlack` の 5 箇所の `slog.Any("error", err)` / `slog.Any("last_error", lastErr)` を `slog.String("error", sanitizeErrorForLog(err))` / `slog.String("last_error", sanitizeErrorForLog(lastErr))` に置換
- [ ] `TestSanitizeErrorForLog` テストを追加（AC-09, AC-10）
- [ ] クロスサーチ: `slog.Any("error",` と `slog.Any("last_error",` の slack_handler.go 内の残存確認

### 7.4 PR-4 チェックリスト（F-004/F-005/F-006: 監査ログ境界 redaction）

- [ ] `LogUserGroupExecution` の `command_args` に `argRedactor.RedactText` を適用
- [ ] `LogUserGroupExecution` の `expanded_command_args` に `argRedactor.RedactText` を適用
- [ ] `LogUserGroupExecution` の失敗時 `stdout` に `argRedactor.RedactText` を適用
- [ ] `LogUserGroupExecution` の失敗時 `stderr` に `argRedactor.RedactText` を適用
- [ ] `LogPrivilegeEscalation` の `operation` に `argRedactor.RedactText` を適用
- [ ] `LogPrivilegeEscalation` の `commandName` に `argRedactor.RedactText` を適用
- [ ] `LogSecurityEvent` の `message` に `argRedactor.RedactText` を適用
- [ ] `LogSecurityEvent` の `details` イテレーションを修正（プレフィックス付与、値の型別処理）
- [ ] `TestLogUserGroupExecution_OutputMasking` テストを追加（AC-11, AC-13）
- [ ] `TestLogUserGroupExecution_ArgMasking` テストを追加（AC-12, AC-13）
- [ ] `TestLogPrivilegeEscalation_Masking` テストを追加（AC-14, AC-16）
- [ ] `TestLogSecurityEvent_Masking` テストを追加（AC-15, AC-16）
- [ ] `TestLogSecurityEvent_DetailsRedaction` テストを追加（AC-17, AC-19）
- [ ] `TestLogSecurityEvent_DetailsKeyCollisionPrevention` テストを追加（AC-18）
- [ ] `logRiskProfileEntry` ヘルパーを再利用するか、必要な新規ヘルパーを同ファイルに追加
- [ ] クロスサーチ: `logger.go:286-289` のコメントを更新（Phase 1 により制約解消）

### 7.5 PR-5 チェックリスト（F-007: 環境変数サニタイズ拡張）

- [ ] `isSensitiveEnvValue` ヘルパー関数を追加（`v.redactionConfig` の nil チェックを含む）
- [ ] `SanitizeEnvironmentVariables` に値内容検査（`isSensitiveEnvValue`）を追加
- [ ] `isSensitiveEnvVar` の既存の正規表現マッチングで元の名前（大文字化前）も試行するよう修正
- [ ] `TestSanitizeEnvironmentVariables_ValueBasedDetection` テストを追加（AC-20, AC-21）
- [ ] `TestValidator_isSensitiveEnvVar_CustomLowercasePattern` テストを追加（AC-22）
- [ ] ベンチマークテスト `BenchmarkSanitizeEnvironmentVariables_WithLargeEnv` を追加（200 エントリ。`02_architecture.md` §9.3）
- [ ] 既存テストの回帰確認（`make test` を `internal/runner/base/security/` で実行）

### 7.6 最終検証チェックリスト（全 PR マージ後）

- [ ] PR-1 マージ済み（対象ステップ: 2.1.1 / 2.1.2 / 2.1.3 / 2.1.4）
- [ ] PR-2 マージ済み（対象ステップ: 2.1.5 / 2.1.6 / 2.1.7）
- [ ] PR-3 マージ済み（対象ステップ: 2.2.1 / 2.2.2 / 2.2.3）
- [ ] PR-4 マージ済み（対象ステップ: 2.3.1 / 2.3.2 / 2.3.3 / 2.3.4）
- [ ] PR-5 マージ済み（対象ステップ: 2.4.1 / 2.4.2 / 2.4.3 / 2.4.4）
- [ ] 全クロスサーチチェック項目を確認
- [ ] リリースノートに `detail_` プレフィックス変更を記載

## 8. リスク管理

| リスク | 影響 | 軽減策 |
|---|---|---|
| Phase 1 の `processKindAny` catch-all 変更により既存ログ出力の情報が欠落する | 低 | コードベース監査により Func/Chan/UnsafePointer 型の `slog.Any` 呼び出しは存在しないことを確認済み（`02_architecture.md` §4.3）。また、int/bool 等のプリミティブ型は `slog.AnyValue` により適切な Kind に解決されるため catch-all に到達しない |
| `detail_` プレフィックス変更が監視クエリを破壊する | 中 | リリースノートに明示的に記載。非本番環境での事前検証を推奨（`02_architecture.md` §9.4 参照）。`LogSecurityEvent` の外部呼び出し元は存在しないため、コードベース内の破壊的影響はない |
| 再帰 redaction のパフォーマンス劣化 | 低 | `maxRedactionDepth=10` が深度を制限。ログ出力はコマンド実行のクリティカルパスではない。ベンチマークテスト（§7.2、§7.5）により現状からの増加率を計測する |

## 9. 次のステップ

1. 本計画書のレビューおよび承認
2. Phase 1 の実装着手（RedactingHandler コア修正）
3. Phase 2〜4 の順次実装
4. 全フェーズ完了後、`make test && make lint` で最終確認
5. PR 作成とコードレビュー
6. リリースノートに `detail_` プレフィックス変更を記載
