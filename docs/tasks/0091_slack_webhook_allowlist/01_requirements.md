# 0091: Slack webhook URL のホスト allowlist

## 背景

[0089_security_audit_fixes](../0089_security_audit_fixes/01_requirements.md) のセキュリティ検査所見 L2 を独立タスクとして切り出したもの。

[internal/logging/slack_handler.go:132-152](../../../internal/logging/slack_handler.go#L132-L152) の `validateWebhookURL` は HTTPS スキームのみを検査し、ホスト名を制限しない。環境変数 (`GSCR_SLACK_WEBHOOK_URL_*`) が改ざんされた場合に、任意ホストへのログ送信 (情報漏洩・SSRF) が成立しうる。

---

## 設計方針

webhook URL 自体はポスト権限を持つ機密情報であるため、TOML 設定ファイルには書けない。一方でオンプレミスの Slack 互換サービスに対応するため、許可ホストの一覧 (ポリシー情報) のみを TOML に記載し、URL 本体は引き続き環境変数で管理する。

- **ポリシー (TOML)**: `global.slack_allowed_hosts` に許可ホストを列挙する。TOML はハッシュ検証対象であるため、改ざんを検出可能。
- **秘密情報 (環境変数)**: `GSCR_SLACK_WEBHOOK_URL_SUCCESS` / `GSCR_SLACK_WEBHOOK_URL_ERROR` は従来どおり環境変数で管理する。
- **検証フロー**: 起動時に TOML から読んだ許可ホストを allowlist とし、環境変数の URL ホストがその allowlist に含まれるかを検証する。`slack_allowed_hosts` が未設定の場合は allowlist が空となり、Slack 通知機能は使用不可となる。
- **ホスト名比較**: `url.Hostname()` でポート番号を除いたホスト名を取得し、`strings.ToLower` で正規化したうえで allowlist と完全一致比較する。

---

## 受け入れ条件

**設定スキーマ:**

- AC-L2-1: `GlobalSpec` に `SlackAllowedHosts []string` フィールド (`toml:"slack_allowed_hosts"`) を追加すること
- AC-L2-2: `SlackHandlerOptions` に `AllowedHosts []string` フィールドを追加すること

**検証ロジック:**

- AC-L2-3: `validateWebhookURL` のシグネチャを `func validateWebhookURL(webhookURL string, allowedHosts []string) error` に変更し、ホスト allowlist 検査を追加すること
- AC-L2-4: allowlist との比較は `url.Hostname()` で取得したホスト名を `strings.ToLower` で正規化したうえで完全一致で行うこと
- AC-L2-5: `AllowedHosts` が空の場合、すべての URL はホスト検証エラーを返すこと
- AC-L2-6 (旧 AC-L2-5): allowlist に含まれないホストへの URL は検証エラーを返すこと

**起動パイプライン:**

- AC-L2-7 (旧 AC-L2-6): bootstrap が `ConfigSpec.Global.SlackAllowedHosts` を `SlackHandlerOptions.AllowedHosts` に渡すこと

**テスト:**

- AC-L2-8: `AllowedHosts` が空の場合に URL 検証がエラーを返すことを確認するユニットテストを追加すること
- AC-L2-9: allowlist に含まれないホスト (例: `evil.example.com`) がエラーになることを確認するユニットテストを追加すること
- AC-L2-10: `AllowedHosts` に登録したホスト (例: `hooks.slack.com`) の URL が検証を通過することを確認するユニットテストを含めること
- AC-L2-11: ホスト名比較が大文字/小文字を区別しないことを確認するユニットテストを追加すること (例: `HOOKS.SLACK.COM` が `hooks.slack.com` の allowlist で通過する)
- AC-L2-12: ポート番号付き URL (例: `https://hooks.slack.com:443/...`) がホスト名 `hooks.slack.com` の allowlist で正しく処理されることを確認するユニットテストを追加すること
- AC-L2-13: `GlobalSpec.SlackAllowedHosts` に設定したホストが `SlackHandlerOptions.AllowedHosts` に正しく渡されることを確認するユニットテストを追加すること
