# Slack 通知エラーハンドリングの設計

## 概要

go-safe-cmd-runner の実行は2つのフェーズに分かれています：

**フェーズ1（ログシステム初期化と設定ファイル検証）**
- ログシステムのセットアップ（`bootstrap.SetupLogging`）
- 設定ファイルのハッシュ検証と読み込み（`verification.Manager.VerifyAndReadConfigFile`）
- TOML 解析とスキーマ検証（`bootstrap.LoadAndPrepareConfig`）

**フェーズ2（Slack ハンドラ登録以降）**
- 設定から許可 Slack ウェブフックホストを読み込み、Slack ハンドラを登録（`bootstrap.SetupSlackLogging`）
- グローバル／グループ対象ファイルのハッシュ検証（`verification.Manager.VerifyGlobalFiles`）
- コマンド実行開始

この設計により、**フェーズ1で発生するエラー（設定ファイルのハッシュ検証失敗、TOML 解析エラー）は Slack 通知チャネルに到達しません**。本ドキュメントはこの制限が生じる理由、代替案の検討、そして推奨される監視戦略を説明します。

なお、到達しないのは**設定ファイル自体の検証エラーに限られます**。グローバル／グループ対象ファイルのハッシュ検証はフェーズ2で実行されるため、その失敗は通常どおり Slack に通知されます。

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
```ini
# サービスユニットで失敗時の通知ユニットを起動する
[Service]
Type=oneshot
OnFailure=notify-admin@%N.service
```

`OnFailure=` はユニットが failed 状態に入ったときにのみ起動します。`Restart=on-failure`
を併用すると、起動制限（`StartLimitBurst`）に達するまでユニットは再起動を繰り返して
failed 状態にならず、通知が発火しません。バッチ実行では `Restart=` を設定せず、
1回の失敗をそのまま failed として扱うのが確実です。

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

**ログファイルの形式（前提）**

ログファイルは実行ごとに新しいファイルとして作成され、名前は
`<ログディレクトリ>/<ホスト名>_<UTC タイムスタンプ>_<run_id>.json` です
（`internal/runner/bootstrap/logger.go`）。追記される単一の `runner.log` は存在しない
ため、監視は**ディレクトリ内のファイル群**を対象にします。中身は1行1レコードの JSON
（slog の `JSONHandler`）で、`time` / `level` / `msg` に加えて `hostname` / `pid` /
`run_id` / `schema_version` が付与されます。

**即座の検出:**
```bash
# 検証エラーの即座の検出（ログディレクトリ内の新規ファイルを追跡）
tail -F /var/log/go-safe-cmd-runner/*.json | grep -i "verification failed"
```

対象とする `msg` の実値は `Global file verification failed` および
`CRITICAL: Global file verification failed - program will terminate`
（`internal/verification/manager.go`）です。設定ファイル自体の検証・解析失敗は
`Failed to verify and read the configuration file` / `Failed to load the configuration`
（`internal/runner/bootstrap/config.go`）として記録されます。

**ログ監視ツールの活用:**

Filebeat（出力は1つだけ有効にできます。Elasticsearch と Logstash を同時に指定すると
起動時にエラーになります）:
```yaml
filebeat.inputs:
- type: filestream
  id: go-safe-cmd-runner
  enabled: true
  paths:
    - /var/log/go-safe-cmd-runner/*.json
  parsers:
    - ndjson:
        target: ""
  fields:
    module: go-safe-cmd-runner

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
```

Fluentd（`@type slack` には `fluent-plugin-slack` が必要です）:
```xml
<source>
  @type tail
  path /var/log/go-safe-cmd-runner/*.json
  pos_file /var/log/go-safe-cmd-runner.pos
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
set -euo pipefail
LOG_DIR=/var/log/go-safe-cmd-runner

# 最後の実行ログ日時が期待通りか。
# 実行開始のマーカーは logger 初期化時に出力される "Logger initialized"
# (internal/runner/bootstrap/logger.go)。ログは実行ごとに別ファイルなので、
# 最新のファイルを対象にする。
LATEST=$(ls -1t "$LOG_DIR"/*.json 2>/dev/null | head -1)
LAST_RUN=$(
    [[ -n "$LATEST" ]] &&
    jq -r 'select(.msg == "Logger initialized") | .time' "$LATEST" | tail -1
)
CURRENT_TIME=$(date -u +"%Y-%m-%dT%H")
if [[ -z "$LAST_RUN" || "$LAST_RUN" != "$CURRENT_TIME"* ]]; then
    echo "WARNING: Last run was not in the current hour"
    # Slack 通知を送信（別途設定）
fi

# 直近の実行に検証エラーがないか
if [[ -n "$LATEST" ]] && jq -e 'select(.level == "ERROR" and (.msg | test("verification failed"; "i")))' \
        "$LATEST" >/dev/null; then
    echo "ALERT: Verification failures detected"
    # Slack 通知を送信（別途設定）
fi
```

`.time` はログ出力時刻（UTC）なので、比較する現在時刻も `date -u` で揃えます。

## 実装の現状

ログシステムはフェーズ1の先頭（`bootstrap.SetupLogging`）で初期化されるため、
その後に発生する設定ファイル検証・解析エラーは必ずログファイルに記録されます。
Slack ハンドラだけが後から追加されます。

**`internal/verification/manager.go`**
```go
func (m *Manager) VerifyAndReadConfigFile(configPath string) ([]byte, error)
// 設定ファイルのハッシュ検証と読み込みを1ステップで行う（TOCTOU 回避）
// 検証失敗時はエラーを返す
```

**`internal/runner/bootstrap/config.go`**
```go
func LoadAndPrepareConfig(...) (*runnertypes.ConfigSpec, error)
// 検証失敗・TOML 解析失敗を PreExecutionError として返す
// 原因は Err に保持されるため errors.Is/As で判別できる
```

**`cmd/runner/main.go`**
```go
// SetupLogging → LoadAndPrepareConfig → SetupSlackLogging の順に実行する。
// LoadAndPrepareConfig のエラーは Slack ハンドラ登録前に返るため、
// ログファイルには残るが Slack には届かない。
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
**実装参考**: [slack_async_delivery.ja.md](slack_async_delivery.ja.md)
