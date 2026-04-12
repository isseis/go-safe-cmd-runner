# 0091: Slack webhook URL のホスト allowlist

## 背景

[0089_security_audit_fixes](../0089_security_audit_fixes/01_requirements.md) のセキュリティ検査所見 L2 を独立タスクとして切り出したもの。

[internal/logging/slack_handler.go:132-152](../../../internal/logging/slack_handler.go#L132-L152) の `validateWebhookURL` は HTTPS スキームのみを検査し、ホスト名を制限しない。環境変数 (`GSCR_SLACK_WEBHOOK_URL_*`) が改ざんされた場合に、任意ホストへのログ送信 (情報漏洩・SSRF) が成立しうる。

---

## 設計方針

webhook URL 自体はポスト権限を持つ機密情報であるため、TOML 設定ファイルには書けない。一方でオンプレミスの Slack 互換サービスに対応するため、許可ホストの一覧 (ポリシー情報) のみを TOML に記載し、URL 本体は引き続き環境変数で管理する。

- **ポリシー (TOML)**: `global.slack_allowed_hosts` に追加許可ホストを列挙する。TOML はハッシュ検証対象であるため、改ざんを検出可能。
- **秘密情報 (環境変数)**: `GSCR_SLACK_WEBHOOK_URL_SUCCESS` / `GSCR_SLACK_WEBHOOK_URL_ERROR` は従来どおり環境変数で管理する。
- **検証フロー**: 起動時に TOML から読んだ追加許可ホストをデフォルト allowlist に合算し、環境変数の URL ホストがその allowlist に含まれるかを検証する。

---

## 受け入れ条件

**設定スキーマ:**

- AC-L2-1: `GlobalSpec` に `SlackAllowedHosts []string` フィールド (`toml:"slack_allowed_hosts"`) を追加すること
- AC-L2-2: `SlackHandlerOptions` に `AllowedHosts []string` フィールドを追加すること

**検証ロジック:**

- AC-L2-3: `validateWebhookURL` (または上位の検証関数) にホスト allowlist 検査を追加し、デフォルトで `hooks.slack.com` のみを許可すること
- AC-L2-4: `AllowedHosts` に指定されたホストをデフォルト allowlist に追加して検証すること
- AC-L2-5: allowlist に含まれないホストへの URL は検証エラーを返すこと

**起動パイプライン:**

- AC-L2-6: bootstrap が `ConfigSpec.Global.SlackAllowedHosts` を `SlackHandlerOptions.AllowedHosts` に渡すこと

**テスト:**

- AC-L2-7: `hooks.slack.com` 以外のホスト (例: `evil.example.com`) がエラーになることを確認するユニットテストを追加すること
- AC-L2-8: `hooks.slack.com` の正当な URL が検証を通過することを確認するユニットテストを含めること
- AC-L2-9: `AllowedHosts` に登録したホストの URL が検証を通過することを確認するユニットテストを追加すること
- AC-L2-10: `GlobalSpec.SlackAllowedHosts` に設定したホストが `SlackHandlerOptions.AllowedHosts` に正しく渡されることを確認するユニットテストを追加すること
