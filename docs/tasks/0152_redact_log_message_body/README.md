# タスク: RedactingHandler がログメッセージ本文を redact しない (Issue #859)

## Document Status

| Item | Value |
|---|---|
| Status | `approved` |
| Created | 2026-07-19 |
| Review date | 2026-07-19 |
| Reviewer | isseis |
| Comments | - |

## 位置づけ（フルセット文書を作らない理由）

このタスクは1箇所（`RedactingHandler.Handle`）のロジック欠落を直す小規模な修正であり、
新規機能や設計判断を伴わない。要件・対応方針・検証方法は下記で完結するため、
`docs/dev/developer_guide/requirements_process.md` が定める
`01_requirements.md` / `02_architecture.md` / `03_implementation_plan.md` の
フルセットではなく、本ファイル1本に要件・方針・進捗を統合する。

## 概要

- Issue: [#859](https://github.com/isseis/go-safe-cmd-runner/issues/859)
- 重大度: 🔴 High (H-2)
- 該当箇所:
  - `internal/redaction/redactor.go:401-414` (`RedactingHandler.Handle`)
  - `internal/logging/slack_handler.go:823-827` (`buildGenericMessage`, `r.Message` をそのまま使用)

`RedactingHandler.Handle` は `record.Attrs` を redact して `newRecord` に積み直しているが、
`record.Message`（ログメッセージ本文）はそのまま `slog.NewRecord` に渡され、redact されない。
`slog.Error(fmt.Sprintf("... %v", err))` のように機密情報（credential 入り URL・トークン等）を
メッセージ本文に埋め込むコードが存在すると、file/stderr に加え `slack_notify=true` 経由で
Slack へも平文で送出されうる。

`SlackHandler` は `multiHandler` 経由で `RedactingHandler` にラップされている
（`internal/runner/bootstrap/logger.go:178,249`）ため、`Handle` 内でメッセージ本文を
redact すれば file/stderr/Slack すべての出力経路を一箇所で塞げる。
`slack_handler.go` 側の個別修正は不要。

## 対応方針

`RedactingHandler.Handle` で `newRecord` 生成時に、`record.Message` にも
`r.config.RedactText()` を適用する。

```go
func (r *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redactedMessage := r.config.RedactText(record.Message)
	newRecord := slog.NewRecord(record.Time, record.Level, redactedMessage, record.PC)
	...
}
```

`RedactText` は文字列パターンマッチのみ（reflection なし）で panic しないため、
Attrs 側のような recover 付きラップは不要（既存の `redactLogAttributeWithContext` の
recover は reflection を伴う複合値の処理のためのものであり、`RedactText` には該当しない）。

## 受け入れ基準 (Acceptance Criteria)

- **AC-01**: メッセージ本文に key=value 形式の機密情報（例: `password=secret123`）を含む
  ログを `slog.Error`/`Info` 等で出力すると、`RedactingHandler` を通過した後の
  `record.Message` が redact されている（プレースホルダに置換されている）こと。
- **AC-02**: メッセージ本文に value 検出ベースの機密情報（例: AWS キー形式の文字列）を
  含む場合も同様に redact されること。
- **AC-03**: 機密情報を含まないメッセージ本文は変更されない（既存の非機密ログの表示が
  壊れない）こと。
- **AC-04**: Attrs の redaction（既存機能）が引き続き正しく動作すること（リグレッションなし）。

## テスト方針

- `internal/redaction/redactor_test.go` の `TestRedactingHandler_Handle`
  （`redactor.go:574`）にメッセージ本文の redaction を検証するケースを追加、または
  専用テスト `TestRedactingHandler_Handle_MessageRedaction` を新設する。
  - AC-01, AC-02, AC-03 をそれぞれケースとして用意する。
- 既存の `TestRedactingHandler_*` 系テストが全てパスすることで AC-04 を確認する。

## 実装チェックリスト

- [x] `internal/redaction/redactor.go` の `RedactingHandler.Handle` を修正し、
      `record.Message` に `RedactText` を適用する
- [x] `internal/redaction/redactor_test.go` にメッセージ本文 redaction のテストを追加
      (AC-01, AC-02, AC-03)
- [x] `make fmt` / `make test` / `make lint` を実行し、既存テストにリグレッションがないこと
      を確認する (AC-04)
- [ ] Issue #859 をクローズ（PR とリンク）

## Acceptance Criteria Verification

| AC | Test | Implementation | Verification |
|---|---|---|---|
| AC-01 | `internal/redaction/redactor_test.go::TestRedactingHandler_Handle_MessageRedaction`（新設済み） | `internal/redaction/redactor.go` `Handle` | key=value 形式メッセージが redact されることを確認 |
| AC-02 | `internal/redaction/redactor_test.go::TestRedactingHandler_Handle_MessageRedaction`（新設済み） | 同上 | value 検出ベースの機密情報が redact されることを確認 |
| AC-03 | 同上 | 同上 | 非機密メッセージが変更されないことを確認 |
| AC-04 | 既存 `TestRedactingHandler_*` 一式 | 同上 | `make test` 全体グリーン |

## 補足: Issue #861 との関係

Issue #861 は本件（メッセージ本文の redaction 漏れ）を含む横断的パターン P2
（redaction 境界の不統一）を扱っており、`slog.Any` の map/slice への再帰未対応や
audit ログ系メソッドの非対称性など、より広いスコープを持つ。本タスクは #859 の
スコープ（メッセージ本文のみ）に限定し、#861 は別タスクとして扱う。
