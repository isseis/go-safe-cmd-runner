# ログシステム再設計の詳細仕様書

## 1. 概要

本文書は`02_architecture.md`で定義されたアーキテクチャに基づき、ログシステム再設計の詳細設計と実装計画を提供します。これは実行毎ユニーク圧縮JSONログ、権限降格前の安全なファイル処理、同期ログ、MarkdownのSlackサマリー、機密情報墨消し、明確なスキーマ/バージョン管理を含む決定を反映しています。

## 2. ファイルとモジュール構造

実装されたログコンポーネントは以下の構造になっています。

- `internal/logging/`: ログユーティリティ用のパッケージ（標準`log`との混同を避けるため）
  - `multihandler.go`: `MultiHandler`の実装
  - `multihandler_test.go`: `MultiHandler`の単体テスト
  - `redactor.go`: 属性/ペイロード用の墨消しデコレーター
  - `redactor_test.go`: 墨消し機能の単体テスト
  - `safeopen.go`: 権限降格前に実行毎ログファイルを安全に開くヘルパー
  - `slack_handler.go`: Slack通知用のカスタムハンドラー
  - `pre_execution_error.go`: 実行前エラー処理のヘルパー関数
  - `pre_execution_error_test.go`: 実行前エラー処理の単体テスト

メインアプリケーションロジックを更新：

- `cmd/runner/main.go`: 新しいログシステムを初期化・設定するため変更される

## 3. `MultiHandler`実装（`internal/log/multihandler.go`）

`MultiHandler`はファンアウト配信とエラー集約を持つ`slog.Handler`インターフェースを実装します。

```go
package logging

import (
	"context"
	"log/slog"
)

// MultiHandlerは複数のハンドラーにログレコードをディスパッチするslog.Handler
type MultiHandler struct { handlers []slog.Handler }

// NewMultiHandlerは与えられたハンドラーをラップする新しいMultiHandlerを作成
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{
		handlers: handlers,
	}
}

// Enabledは指定レベルでハンドラーがレコードを処理するかどうかを報告
// 基盤ハンドラーの少なくとも1つが有効な場合にハンドラーは有効
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handleはすべての基盤ハンドラーに渡すことでログレコードを処理
func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
    var multiErr error
    for _, handler := range h.handlers {
        if handler.Enabled(ctx, r.Level) {
            if err := handler.Handle(ctx, r.Clone()); err != nil {
                // すべてのエラーを集約（最初のエラー + ラップ）
                if multiErr == nil { multiErr = err } else { multiErr = errors.Join(multiErr, err) }
            }
        }
    }
    return multiErr
}

// WithAttrsは指定属性を持つ新しいMultiHandlerを返す
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

// WithGroupは指定グループ名を持つ新しいMultiHandlerを返す
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}
```

## 4. `main.go`の修正（`cmd/runner/main.go`）

`main`関数は新しいログシステムを設定するため更新されます。

### 4.1. 設定とフラグ

設定優先度：CLIフラグ > 環境変数 > TOML設定 > デフォルト値。TOMLキーは`[logging]`以下。

```go
// main関数内、flag.Parse()前
var (
    // ... 既存フラグ
    logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
    logDir   = flag.String("log-dir", "", "Directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
)
```
`log-level`のデフォルトは`info`です。

### 4.2. ログ初期化ロジック

新しい関数`setupLoggerWithConfig`が作成され、`run`の開始時に呼び出されます。実行毎ファイル名`<hostname>_<timestamp>_<runid>.json`を生成し、安全に（シンボリックリンクなし）開き、ハンドラーを設定します。

```go
// setupLoggerWithConfig initializes the logging system with all handlers atomically
func setupLoggerWithConfig(config LoggerConfig) error {
    hostname, err := os.Hostname()
    if err != nil {
        hostname = "unknown-host"
    }
    timestamp := time.Now().Format("20060102T150405Z")

    var handlers []slog.Handler
    var invalidLogLevel bool

    // 1. Human-readable summary handler (to stdout)
    textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    handlers = append(handlers, textHandler)

    // 2. Machine-readable log handler (to file, per-run auto-named)
    if config.LogDir != "" {
        // Validate log directory
        if err := logging.ValidateLogDir(config.LogDir); err != nil {
            return fmt.Errorf("invalid log directory: %w", err)
        }

        logPath := filepath.Join(config.LogDir, fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID))
        fileOpener := logging.NewSafeFileOpener()
        logF, err := fileOpener.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, logFilePerm)
        if err != nil {
            return fmt.Errorf("failed to open log file: %w", err)
        }

        var slogLevel slog.Level
        if err := slogLevel.UnmarshalText([]byte(config.Level)); err != nil {
            slogLevel = slog.LevelInfo // Default to info on parse error
            invalidLogLevel = true
        }

        jsonHandler := slog.NewJSONHandler(logF, &slog.HandlerOptions{
            Level: slogLevel,
        })

        // Attach common attributes
        enrichedHandler := jsonHandler.WithAttrs([]slog.Attr{
            slog.String("hostname", hostname),
            slog.Int("pid", os.Getpid()),
            slog.Int("schema_version", 1),
            slog.String("run_id", config.RunID),
        })
        handlers = append(handlers, enrichedHandler)
    }

    // 3. Slack notification handler (optional)
    if config.SlackWebhookURL != "" {
        slackHandler := logging.NewSlackHandler(config.SlackWebhookURL, config.RunID)
        handlers = append(handlers, slackHandler)
    }

    // Create MultiHandler with redaction
    multiHandler := logging.NewMultiHandler(handlers...)
    redactedHandler := redaction.NewRedactingHandler(multiHandler, nil)

    // Set as default logger
    logger := slog.New(redactedHandler)
    slog.SetDefault(logger)

    slog.Info("Logger initialized",
        "log-level", config.Level,
        "log-dir", config.LogDir,
        "run_id", config.RunID,
        "hostname", hostname,
        "slack_enabled", config.SlackWebhookURL != "")

    // Warn about invalid log level after logger is properly set up
    if invalidLogLevel {
        slog.Warn("Invalid log level provided, defaulting to INFO", "provided", config.Level)
    }

    return nil
}

```

### 4.3. `log`パッケージ呼び出しの置換

標準`log`パッケージ（`log.Printf`、`log.Fatalf`など）へのすべての既存呼び出しは対応する`slog`呼び出しに置き換える必要があります。

- `log.Printf(...)` -> `slog.Info(...)`または`slog.Debug(...)`
- `log.Fatalf(...)` -> `slog.Error(...); os.Exit(1)`

**置換例:**
```go
// 置換前
if err != nil {
    log.Fatalf("Failed to drop privileges: %v", err)
}

// 置換後
if err != nil {
    slog.Error("Failed to drop privileges", "error", err)
    os.Exit(1)
}
```

## 5. テスト計画

- **`MultiHandler`の単体テスト**:
  - `Enabled()`が任意のハンドラーが有効な場合にtrueを返すことをテスト
  - `Handle()`がすべての有効なハンドラーで`Handle`を呼び出すことをテスト
  - `WithAttrs()`と`WithGroup()`が属性/グループをすべてのハンドラーに正しく伝搬することをテスト
- **実行前エラーハンドリングのテスト**:
  - 設定解析失敗、ファイルアクセス失敗、権限エラーなどの実行前エラーケース
  - 実行前エラー時のログ記録とSlack通知の確認
  - ユーザー中断（SIGINT等）時の適切な処理
  - 実行前エラー時のRUN_SUMMARYフォーマット（`status=pre_execution_error`）
- **`main_test.go`での統合テスト**:
    - `--log-level`と`--log-dir`の組み合わせで`runner`を実行
    - stdoutにタイトル行と`RUN_SUMMARY`が含まれることを確認
    - 実行毎JSONファイルが`0600`権限で存在することを確認
    - 墨消しの確認：機密情報が`***`で置換される
    - テストダブル（HTTPサーバー）で429/5xxを含むSlackリトライロジックを確認
    - 実行前エラー時のSlack通知内容とフォーマットの確認
    - レーステストと高負荷テスト

## 6. 対象外（文書化された将来作業）
- 動的ログレベル再読み込み（SIGHUP）
- Trace/Span ID伝搬（OpenTelemetry）
- ローテーション/保持ポリシー（外部ツール）

この詳細仕様書はすべての要件が満たされることを保証し、実装への明確な道筋を提供します。
