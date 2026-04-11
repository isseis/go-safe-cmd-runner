# L2: Slack webhook URL の宛先 allowlist なし

- **重大度**: 🟠 Low
- **領域**: ログ通知 (`internal/logging`)
- **影響コマンド**: `record`, `verify`, `runner`

## 問題

[internal/logging/slack_handler.go:132-152](../../../internal/logging/slack_handler.go#L132-L152) の `validateWebhookURL` は以下のチェックのみを行う:

1. URL パース成功
2. スキームが `https`

ホスト名に対する allowlist は存在せず、`https://` で始まる任意の URL を宛先にできる。

## 影響

### 攻撃シナリオ

- 攻撃者が設定ファイルを改変できる場合 (ハッシュ検証をすり抜けた場合)、webhook URL を攻撃者制御のサーバに書き換えることで以下が可能:
  - ログ通知の内容 (実行コマンド、エラーメッセージ、タイムスタンプ等) を攻撃者サーバに送出し情報収集。
  - 外向き HTTP リクエストを強制することで SSRF 的な挙動 (内部ネットワークのメタデータサーバ `169.254.169.254` 等への到達) を誘発。

### 緩和要因

- 設定ファイルはハッシュ検証済みのため、攻撃者が自由に書き換えられる前提は本来成立しない。
- HTTPS 強制により MITM は防がれる。
- ログ内容には認証情報が **基本的に含まれない** (redaction 機構あり)。しかし redaction は完全ではない可能性がある。
- 通知先が `hooks.slack.com` 以外になる運用は通常ありえないため、allowlist 化のコストは低い。

## 修正方針

### 案 A (推奨): ホスト allowlist

デフォルトで `hooks.slack.com` のみ許可。運用上必要なら config で allowlist を拡張可能。

```go
var defaultAllowedHosts = map[string]bool{
    "hooks.slack.com": true,
}

func validateWebhookURL(raw string, extra []string) error {
    u, err := url.Parse(raw)
    if err != nil { return err }
    if u.Scheme != "https" { return ErrNonHTTPS }
    host := u.Hostname()
    if defaultAllowedHosts[host] { return nil }
    for _, h := range extra {
        if host == h { return nil }
    }
    return fmt.Errorf("webhook host %q not in allowlist", host)
}
```

### 案 B: プライベート IP 拒否

ホスト名から解決される IP がプライベート (RFC1918, loopback, link-local) の場合を拒否。SSRF 緩和になる。

- ただし「DNS rebinding」対策にはならないため、案 A の明示 allowlist の方が堅牢。

## 参考箇所

- [internal/logging/slack_handler.go:132-152](../../../internal/logging/slack_handler.go#L132-L152) — 現状の `validateWebhookURL`
