# Slack 通知エラーハンドリングの設計

## 概要

go-safe-cmd-runner の実行は2つのフェーズに分かれています：

**フェーズ1（検証とログシステム初期化）**
- ファイルハッシュ検証（`internal/verification/manager.go`）
- TOML 設定解析（`internal/runner/config.go`）
- ログシステムのセットアップ

**フェーズ2（Slack ハンドラ登録）**
- 許可 Slack ウェブフックホストの読み込み
- Slack ハンドラの初期化（`internal/logging/slack_handler.go`）
- コマンド実行開始

この設計により、**フェーズ1で発生するエラー（設定ファイルハッシュ検証失敗、TOML 解析エラー）は Slack 通知チャネルに到達しません**。本ドキュメントはこの制限が生じる理由、代替案の検討、そして推奨される監視戦略を説明します。

## 問題の本質

### なぜ通知ができないのか

Slack ハンドラ登録は設定ファイルから許可 Slack ホストを読み込む必要があります：

```
設定ファイル読み込み失敗 → Slack 設定を読み込めない → Slack ハンドラ登録不可
```

この依存関係は**構造的な制約**です。設定ファイル自体が改ざんされていれば、その設定から Slack 通知先を読むことはできません。

### 発生するエラー例

- **ハッシュ検証失敗**: ファイルが改ざんされた場合
- **TOML 解析エラー**: 設定ファイルが壊れている場合
- **スキーマ検証エラー**: 設定フィールドが無効な場合

これらは**最も重要度が高い検出イベント**です。設定ファイルの改ざんはシステムの整合性を脅かす重大なセキュリティインシデントです。

## 代替案の検討

### 案1: ハードコード化されたフォールバック Slack 通知

設定ファイル読み込み失敗時に、ハードコードされた Slack ウェブフックホストを使用して通知する。

**メリット**
- フェーズ1エラーが Slack に到達する

**デメリット**
- ハードコード値の管理負荷増加
- 環境ごとの異なる Slack ホストに対応できない
- セキュリティリスク: 事前知識のない不正ホストへの誤送信防止が複雑
- コードベースに環境固有の設定が混在する
- テスト複雑度が著しく増加
- バイナリ配布時の再コンパイル必要

### 案2: 最小ペイロードの事前検証

許可ホストのリストを最小限の情報（例：既知の Slack IP レンジ）で事前検証。

**メリット**
- データ漏洩のリスク低減（やや）

**デメリット**
- Slack IP 範囲の変動に対応する必要あり
- 依然として2つの初期化パスが必要
- テスト複雑度が高い
- 保守負荷が増加
- 結局のところ Slack 手段に依存した通知はできない

### 案3: 外部監視メカニズムに依存（採用案）

起動失敗、ログ監視、ヘルスチェックなどの**外部監視メカニズム**でエラーを検出。

**メリット**
- 初期化フローはシンプルなまま
- 複雑な代替ロジック不要
- テスト複雑度が上がらない
- 他の障害（例：権限不足）との一貫した検出戦略
- 運用監視ツール（Prometheus, Grafana など）の標準パターン
- 柔軟で拡張性が高い

**デメリット**
- Slack 通知に即座には到達しない（ただしログには記録される）
- 外部監視の設定が必要

## 採用した設計：外部監視に依存

設定ファイルハッシュ検証エラーを**確実に検出**するため、以下のメカニズムを推奨します。

### 1. プロセス起動失敗の監視

**systemd を使用する場合:**
```bash
# サービスユニットで restart ポリシーを設定
[Service]
Restart=on-failure
RestartSec=10s
OnFailure=notify-admin@%n.service
```

起動失敗ユニットの監視：
```bash
systemctl status go-safe-cmd-runner.service
journalctl -u go-safe-cmd-runner.service -f
```

**cron を使用する場合:**
```bash
# cron 実行の異常終了(exit code != 0)を監視
# メール通知設定
MAILTO=admin@example.com
0 2 * * * /usr/local/bin/runner --config /etc/go-safe-cmd-runner/config.toml
```

### 2. ログファイル監視

**即座の検出:**
```bash
# ハッシュ検証エラーの即座の検出
tail -f /var/log/go-safe-cmd-runner/runner.log | grep -i "hash verification failed"
```

**ログ監視ツールの活用:**

Filebeat:
```yaml
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/go-safe-cmd-runner/runner.log
  fields:
    module: go-safe-cmd-runner
  processors:
    - add_kubernetes_metadata: ~

output.elasticsearch:
  hosts: ["elasticsearch:9200"]

output.logstash:
  hosts: ["logstash:5000"]
```

Fluentd:
```xml
<source>
  @type tail
  path /var/log/go-safe-cmd-runner/runner.log
  pos_file /var/log/runner.log.pos
  tag go-safe-cmd-runner
  <parse>
    @type json
  </parse>
</source>

<match go-safe-cmd-runner>
  @type copy
  <store>
    @type elasticsearch
    host elasticsearch
    port 9200
    logstash_format true
  </store>
  <store>
    @type slack
    webhook_url "#{ENV['SLACK_WEBHOOK_URL']}"
    message_keys message,level
  </store>
</match>
```

### 3. ヘルスチェック

定期実行スケジュールに異常がないか確認：

```bash
#!/bin/bash
# 最後の実行ログ日時が期待通りか
LAST_RUN=$(grep "process started" /var/log/go-safe-cmd-runner/runner.log | tail -1 | cut -d' ' -f1-2)
CURRENT_TIME=$(date +"%Y-%m-%d %H")
if [[ "$LAST_RUN" != "$CURRENT_TIME"* ]]; then
    echo "WARNING: Last run was not in the current hour"
    # Slack 通知を送信（別途設定）
fi

# 最近のログにハッシュ検証エラーがないか
if grep -i "hash verification failed" /var/log/go-safe-cmd-runner/runner.log | tail -24h > /dev/null; then
    echo "ALERT: Hash verification failures detected in past 24 hours"
    # Slack 通知を送信（別途設定）
fi
```

## 実装の現状

フェーズ1の各エラーは必ずログに記録されます：

**`internal/verification/manager.go`**
```go
func (m *Manager) Verify(...) error
// ハッシュ検証失敗時はエラーを返す
// 呼び出し側がログ記録（エラーの痕跡は失われない）
```

**`internal/runner/config.go`**
```go
func (c *Config) Load(...) error
// TOML 解析失敗時はエラーを返す
// ログシステムが初期化される前なため、標準エラー出力にも記録
```

**`internal/runner/executor.go`**
```go
func (e *Executor) Run(...) error
// フェーズ1の各エラーをログに記録
// 設定ファイル改ざん検出 → プロセス起動失敗 → 強力なシグナル
```

### 重要な保証

| 項目 | 保証 |
|------|------|
| **ログ記録** | ✓ エラーは必ずログファイルに記録される |
| **起動失敗** | ✓ 設定ファイルハッシュ検証失敗→プロセス起動失敗 |
| **外部監視可能性** | ✓ ログ監視、プロセス起動失敗監視で確実に検出可能 |

## トレードオフ分析

| 項目 | 現在の設計 | 代替案 |
|------|---------|-------|
| 初期化フローの複雑度 | ⭐⭐ (低) | ⭐⭐⭐⭐ (高) |
| テスト複雑度 | ⭐⭐ (低) | ⭐⭐⭐⭐ (高) |
| ログ記録 | ✓ (保証) | ✓ (保証) |
| Slack 即時通知 | ✗ (なし) | ✓ (あり) |
| 保守負荷 | ⭐⭐ (低) | ⭐⭐⭐⭐ (高) |
| 運用監視の柔軟性 | ✓ (良好) | ✓ (良好) |

## 設計判断の根拠

1. **複雑性対効果**: 代替案の実装・テスト・保守コストが著しく高い一方で、外部監視で同じ目的が実現できる。

2. **セキュリティのベストプラクティス**: 重大な設定ファイル改ざんの場合、プロセス起動そのものが失敗する。この「起動失敗」が最大の警告信号であり、Slack 非同期通知よりも有効。

3. **運用の柔軟性**: 外部監視メカニズムは、ホスト構成の変更に柔軟に対応でき、複数の検出パターン（ログ監視、ヘルスチェック、メトリクス監視）を組み合わせられる。

4. **一貫性**: 他の障害シナリオ（権限不足、ディスク容量不足など）との検出戦略を統一できる。

## 結論

**設定ファイルハッシュ検証エラーは**：
1. **必ずログに記録される** ← 証拠は失われない
2. **起動失敗を引き起こす** ← 強力で信頼性の高い検出シグナル
3. **外部監視で確実に検出可能** ← ログ監視、プロセス監視、ヘルスチェック等

したがって、**複雑な代替ロジックの導入コストより、シンプルな設計と外部監視の組み合わせの方が、運用効率が高い**と判断します。

この設計上のトレードオフは**受容可能であり、推奨される**です。

---

**関連課題**: [Issue #1018](https://github.com/isseis/go-safe-cmd-runner/issues/1018)
**実装参考**: [internal/logging/slack_async_delivery.ja.md](slack_async_delivery.ja.md)
