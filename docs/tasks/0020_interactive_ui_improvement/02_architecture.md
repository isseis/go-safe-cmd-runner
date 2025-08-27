# 対話的UI改善 アーキテクチャ設計書

## 1. アーキテクチャ概要

### 1.1 設計原則
- **既存システムとの統合**: 現在の `internal/logging` パッケージを拡張
- **責任分離**: 対話モード検知、メッセージフォーマット、出力制御を分離
- **後方互換性**: 非対話的利用への影響を最小化
- **設定の一元化**: 既存の `--log-level` オプションを活用

### 1.2 システム構成
```mermaid
graph TB
    subgraph "Runner Application"
        CLI[CLI Parsing]
        Config[Config Loading]
        Main[Main Execution]
    end

    subgraph "Enhanced Logging System"
        RH[Redacted Handler]
        MH[MultiHandler]
        IH[Interactive Handler - NEW]
        CTH[Conditional Text Handler - MODIFIED]
        JH[JSON Handler - EXISTING]
        SH[Slack Handler - EXISTING]
    end

    subgraph "Handler Components"
        IMD[Interactive Mode Detector]
        MF[Message Formatter]
    end

    CLI --> Config
    Config --> Main
    Main --> RH
    RH --> MH
    MH --> IH
    MH --> CTH
    MH --> JH
    MH --> SH
    IH --> IMD
    IH --> MF
    CTH --> IMD
```

## 2. システム構成要素

### 2.1 新規パッケージ構成
```
internal/
├── logging/
│   ├── pre_execution_error.go         # 既存（修正）
│   ├── interactive_handler.go         # 新規
│   ├── conditional_text_handler.go    # 新規
│   ├── message_formatter.go           # 新規
│   ├── log_line_tracker.go           # 新規
│   └── message_templates.go           # 新規
└── terminal/
    ├── detector.go                    # 新規：対話性検出
    ├── color.go                       # 新規：カラー対応検出
    ├── preference.go                  # 新規：ユーザー設定管理
    └── capabilities.go                # 新規：端末機能統合
```

## 3. コンポーネント設計

### 3.1 Terminal Package (新規)
端末機能に関する責任を分離した専用パッケージ

#### 3.1.1 責任の分離
- **対話性検出**: CI環境、ターミナル判定
- **カラー対応検出**: 端末のカラー表示能力判定
- **ユーザー設定管理**: NO_COLOR, CLICOLOR等の環境変数処理
- **端末機能統合**: 上記機能の統合インターフェース

#### 3.1.2 Terminal Capabilities インターフェース
```go
// terminal/capabilities.go
package terminal

// Capabilities は端末の全体的な能力を表す統合インターフェース
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool
    GetColorProfile() ColorProfile
    HasExplicitUserPreference() bool
}

// DefaultCapabilities は標準的な端末機能検出実装
type DefaultCapabilities struct {
    detector    InteractiveDetector
    colorSupport ColorDetector
    userPref    UserPreference
}

func NewCapabilities(options ...Option) Capabilities {
    return &DefaultCapabilities{
        detector:     NewInteractiveDetector(),
        colorSupport: NewColorDetector(),
        userPref:     NewUserPreference(),
    }
}
```

#### 3.1.3 対話性検出 (detector.go)
```go
// terminal/detector.go
package terminal

import (
    "os"
    "golang.org/x/term"
)

// InteractiveDetector は実行環境の対話性を判定する
type InteractiveDetector interface {
    IsInteractive() bool
    IsTerminal() bool
    IsCIEnvironment() bool
}

type DefaultInteractiveDetector struct {
    forceInteractive *bool
    stdout           *os.File
    stderr           *os.File
}

func NewInteractiveDetector(options ...DetectorOption) InteractiveDetector {
    detector := &DefaultInteractiveDetector{
        stdout: os.Stdout,
        stderr: os.Stderr,
    }

    for _, opt := range options {
        opt(detector)
    }

    return detector
}

func (d *DefaultInteractiveDetector) IsInteractive() bool {
    // テスト用の強制設定が優先
    if d.forceInteractive != nil {
        return *d.forceInteractive
    }

    // CI環境の検出
    if d.IsCIEnvironment() {
        return false
    }

    // ターミナルかどうかの判定
    return d.IsTerminal()
}

func (d *DefaultInteractiveDetector) IsTerminal() bool {
    return term.IsTerminal(int(d.stdout.Fd()))
}

func (d *DefaultInteractiveDetector) IsCIEnvironment() bool {
    return os.Getenv("CI") != ""
}
```

#### 3.1.4 カラー対応検出 (color.go)
```go
// terminal/color.go
package terminal

import (
    "os"
    "strings"
)

// ColorDetector は端末のカラー表示能力を判定する
type ColorDetector interface {
    SupportsColor() bool
    GetColorProfile() ColorProfile
    IsColorCapableTerminal() bool
}

// ColorProfile はカラー対応レベルを表す
type ColorProfile int

const (
    ColorProfileNone ColorProfile = iota
    ColorProfileBasic              // 8色
    ColorProfile256                // 256色
    ColorProfileTrueColor          // 1600万色
)

type DefaultColorDetector struct {
    termEnv      string
    colortermEnv string
}

func NewColorDetector() ColorDetector {
    return &DefaultColorDetector{
        termEnv:      os.Getenv("TERM"),
        colortermEnv: os.Getenv("COLORTERM"),
    }
}

func (d *DefaultColorDetector) SupportsColor() bool {
    return d.IsColorCapableTerminal()
}

func (d *DefaultColorDetector) GetColorProfile() ColorProfile {
    if !d.IsColorCapableTerminal() {
        return ColorProfileNone
    }

    // TrueColor対応の判定
    if d.colortermEnv == "truecolor" || d.colortermEnv == "24bit" {
        return ColorProfileTrueColor
    }

    // 256色対応の判定
    if strings.Contains(d.termEnv, "256color") {
        return ColorProfile256
    }

    // 基本カラー対応
    return ColorProfileBasic
}

func (d *DefaultColorDetector) IsColorCapableTerminal() bool {
    // COLORTERM環境変数による高度なカラー対応の判定
    if d.colortermEnv == "truecolor" || d.colortermEnv == "24bit" {
        return true
    }

    // カラー対応が確実なターミナル
    colorTerminals := []string{
        "xterm", "xterm-color", "xterm-256color",
        "screen", "screen-256color",
        "tmux", "tmux-256color",
        "rxvt", "rxvt-unicode", "rxvt-256color",
        "linux", "cygwin", "konsole", "gnome", "vte",
    }

    // 直接マッチの確認
    for _, colorTerm := range colorTerminals {
        if strings.HasPrefix(d.termEnv, colorTerm) {
            return true
        }
    }

    // 複合パターンの確認（screen.xterm-256color等）
    if strings.HasPrefix(d.termEnv, "screen.") {
        suffix := strings.TrimPrefix(d.termEnv, "screen.")
        for _, colorTerm := range colorTerminals {
            if strings.HasPrefix(suffix, colorTerm) {
                return true
            }
        }
    }

    // tmux複合パターンの確認
    if strings.HasPrefix(d.termEnv, "tmux.") {
        suffix := strings.TrimPrefix(d.termEnv, "tmux.")
        for _, colorTerm := range colorTerminals {
            if strings.HasPrefix(suffix, colorTerm) {
                return true
            }
        }
    }

    // カラーサフィックスの確認
    colorSuffixes := []string{"-color", "-256color", "-88color", "color"}
    for _, suffix := range colorSuffixes {
        if strings.HasSuffix(d.termEnv, suffix) {
            return true
        }
    }

    return false
}
```

#### 3.1.5 ユーザー設定管理 (preference.go)
```go
// terminal/preference.go
package terminal

import "os"

// UserPreference はユーザーの明示的な設定を管理する
type UserPreference interface {
    WantsColor() *bool
    HasExplicitPreference() bool
    IsColorForced() bool
    IsColorDisabled() bool
}

type DefaultUserPreference struct {
    noColorEnv  string
    cliColorEnv string
}

func NewUserPreference() UserPreference {
    return &DefaultUserPreference{
        noColorEnv:  os.Getenv("NO_COLOR"),
        cliColorEnv: os.Getenv("CLICOLOR"),
    }
}

// WantsColor はユーザーの明示的なカラー設定希望を返す
// 戻り値: nil = 明示的な希望なし, true = カラー希望, false = カラー拒否
func (p *DefaultUserPreference) WantsColor() *bool {
    // NO_COLOR環境変数による明示的な無効化（業界標準）
    if p.noColorEnv != "" {
        return &[]bool{false}[0]
    }

    // CLICOLOR環境変数によるユーザー希望の判定（Unix系標準）
    if p.cliColorEnv != "" {
        // "0" 以外の場合はカラー希望
        if p.cliColorEnv != "0" {
            return &[]bool{true}[0]
        } else {
            return &[]bool{false}[0]
        }
    }

    // 明示的な希望がない場合はnilを返す
    return nil
}

func (p *DefaultUserPreference) HasExplicitPreference() bool {
    return p.noColorEnv != "" || p.cliColorEnv != ""
}

func (p *DefaultUserPreference) IsColorForced() bool {
    return p.cliColorEnv != "" && p.cliColorEnv != "0"
}

func (p *DefaultUserPreference) IsColorDisabled() bool {
    return p.noColorEnv != "" || p.cliColorEnv == "0"
}
```

### 3.2 Interactive Handler (logging package - 修正)
terminalパッケージを使用するように修正

#### 3.2.1 修正された実装
```go
// logging/interactive_handler.go
package logging

import (
    "context"
    "fmt"
    "io"
    "log/slog"
    "os"

    "internal/terminal"
)

type InteractiveHandler struct {
    capabilities terminal.Capabilities
    formatter    MessageFormatter
    level        slog.Level
    output       io.Writer
    logFilePath  string
    lineTracker  *LogLineTracker
}

func NewInteractiveHandler(level slog.Level, logFilePath string) *InteractiveHandler {
    return &InteractiveHandler{
        capabilities: terminal.NewCapabilities(),
        formatter:    NewMessageFormatter(),
        level:        level,
        output:       os.Stderr,
        logFilePath:  logFilePath,
        lineTracker:  globalLineTracker,
    }
}

func (h *InteractiveHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return h.capabilities.IsInteractive() && level >= h.level
}

func (h *InteractiveHandler) Handle(ctx context.Context, r slog.Record) error {
    if !h.capabilities.IsInteractive() {
        return nil
    }

    // カラー対応を考慮したフォーマッターの設定
    colorSupported := h.capabilities.SupportsColor()
    message := h.formatter.FormatRecordWithColor(r, colorSupported)

    // ログファイルヒント情報の追加
    if r.Level >= slog.LevelError && h.logFilePath != "" {
        estimatedLine := h.lineTracker.EstimateCurrentLine()
        logHint := h.formatLogFileHint(estimatedLine, colorSupported)
        message = message + "\n" + logHint
    }

    _, err := fmt.Fprintf(h.output, "%s\n", message)
    h.lineTracker.IncrementLine()

    return err
}

func (h *InteractiveHandler) formatLogFileHint(estimatedLine int, colorSupported bool) string {
    if colorSupported {
        return fmt.Sprintf("\033[90mDetails: %s (around line %d)\033[0m",
            h.logFilePath, estimatedLine)
    }
    return fmt.Sprintf("Details: %s (around line %d)", h.logFilePath, estimatedLine)
}
```

### 2.3 Log Line Tracker (新規)
ログファイルの行数を追跡してヒント情報を提供する機能

#### 2.3.1 責任
- ログファイルへの書き込み行数の概算追跡
- 複数ハンドラー間での行数同期
- メモリ効率的な行数管理

#### 2.3.2 実装方法
```go
type LogLineTracker struct {
    currentLine int
    mutex       sync.RWMutex
}

func NewLogLineTracker() *LogLineTracker {
    return &LogLineTracker{
        currentLine: 1,
    }
}

func (lt *LogLineTracker) IncrementLine() {
    lt.mutex.Lock()
    defer lt.mutex.Unlock()
    lt.currentLine++
}

func (lt *LogLineTracker) EstimateCurrentLine() int {
    lt.mutex.RLock()
    defer lt.mutex.RUnlock()
    return lt.currentLine
}

// グローバルなLine Trackerを共有（複数ハンドラー間で同期）
var globalLineTracker *LogLineTracker

func init() {
    globalLineTracker = NewLogLineTracker()
}
```

### 2.4 Message Formatter
ログメッセージの形式を決定する機能（共有コンポーネント）

#### 2.4.1 責任
- slog.Record から適切なメッセージフォーマットを生成
- ログ情報の構造化
- カラー表示の制御

#### 2.4.2 実装方法
```go
type MessageFormatter interface {
    FormatRecord(r slog.Record) string
}

type DefaultMessageFormatter struct {
    supportsColor bool
}

func (f *DefaultMessageFormatter) FormatRecord(r slog.Record) string {
    // slog.Record の Attrs から必要な情報を抽出
    var logType, message, component, runID string

    r.Attrs(func(a slog.Attr) bool {
        switch a.Key {
        case "error_type", "log_type":
            logType = a.Value.String()
        case "error_message", "message":
            message = a.Value.String()
        case "component":
            component = a.Value.String()
        case "run_id":
            runID = a.Value.String()
        }
        return true
    })

    return f.formatInteractiveMessage(logType, message, component)
}
```

## 3. データフロー

### 3.1 ログイベント発生時のフロー
```mermaid
flowchart TD
    A[Log Event Occurs] --> B[slog.Logger.Error/Warn/Info/Debug]
    B --> C[Redacted Handler]
    C --> D[MultiHandler]
    D --> E[Interactive Handler]
    D --> F[Conditional Text Handler]
    D --> G[JSON Handler]
    D --> H[Slack Handler]

    E --> E1{Is Interactive?}
    E1 -->|Yes| E2[Format for UI]
    E1 -->|No| E3[Skip]
    E2 --> E4[Output to stderr]

    F --> F1{Is Interactive?}
    F1 -->|No| F2[Output to stdout - existing format]
    F1 -->|Yes| F3[Skip]

    G --> G1[Log to JSON file - full details]
    H --> H1[Send Slack notification]
```

### 3.2 ログレベル制御フロー
```mermaid
flowchart TD
    A[slog Record] --> B[MultiHandler]
    B --> C[Interactive Handler.Enabled]
    B --> D[Conditional Text Handler.Enabled]
    B --> E[JSON Handler.Enabled]
    B --> F[Slack Handler.Enabled]

    C --> C1{Interactive && Level >= threshold?}
    C1 -->|Yes| C2[Format & Output to stderr]
    C1 -->|No| C3[Skip]

    D --> D1{!Interactive && Level >= threshold?}
    D1 -->|Yes| D2[Output to stdout - existing format]
    D1 -->|No| D3[Skip]

    E --> E1{Level >= threshold?}
    E1 -->|Yes| E2[Log to JSON file]
    E1 -->|No| E3[Skip]

    F --> F1{Level >= threshold?}
    F1 -->|Yes| F2[Send notification]
    F1 -->|No| F3[Skip]
```

## 4. 既存システムとの統合

### 4.1 既存ハンドラーとの関係

現在のログシステムは以下の3つのハンドラーを使用：
- **slog.NewTextHandler(os.Stdout)**: 人間読み取り用サマリー出力
- **slog.NewJSONHandler(logFile)**: 詳細ログをJSONファイルに出力
- **SlackHandler**: 通知送信

#### 4.1.1 既存TextHandlerの扱い
現在のTextHandlerは非対話的環境でも常にstdoutに出力するため、以下の方針で統合：

```go
// 既存のログシステムの修正
func setupLogging(config Config) error {
    level := parseLogLevel(config.Level)
    var handlers []slog.Handler

    // 1. 既存のTextHandler を ConditionalTextHandler で置き換え
    // （非対話的環境でstdoutに出力、対話的環境では出力しない）
    originalTextHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    conditionalTextHandler := NewConditionalTextHandler(originalTextHandler)
    handlers = append(handlers, conditionalTextHandler)

    // 2. 新しいInteractiveHandler を追加
    // （対話的環境でstderrに出力、非対話的環境では出力しない）
    logFilePath := ""
    if config.LogDir != "" {
        logFilePath = generateLogPath(config) // ログファイルパスを共有
    }
    interactiveHandler := NewInteractiveHandler(level, logFilePath)
    handlers = append(handlers, interactiveHandler)

    // 3. 既存のJSONHandler を維持（ファイル出力）
    if config.LogDir != "" {
        logPath := generateLogPath(config)
        jsonHandler := createJSONHandler(logPath, level, config)
        handlers = append(handlers, jsonHandler)
    }

    // 4. 既存のSlackHandler を維持
    if config.SlackWebhookURL != "" {
        slackHandler, err := logging.NewSlackHandler(config.SlackWebhookURL, config.RunID)
        if err != nil {
            return err
        }
        handlers = append(handlers, slackHandler)
    }

    // MultiHandlerで統合
    multiHandler, err := NewMultiHandler(handlers...)
    if err != nil {
        return err
    }

    logger := slog.New(redaction.NewRedactedHandler(multiHandler))
    slog.SetDefault(logger)
    return nil
}

// ConditionalTextHandler の実装
func NewConditionalTextHandler(textHandler slog.Handler) *ConditionalTextHandler {
    return &ConditionalTextHandler{
        textHandler: textHandler,
        detector:    NewInteractiveDetector(nil, false),
    }
}
```

#### 4.1.2 ConditionalTextHandler の完全実装
上記4.1.1で参照される ConditionalTextHandler の完全実装：

```go
// ConditionalTextHandler は対話モードに応じてTextHandlerの出力を制御する
type ConditionalTextHandler struct {
    textHandler slog.Handler
    detector    InteractiveDetector
}

func NewConditionalTextHandler(textHandler slog.Handler) *ConditionalTextHandler {
    return &ConditionalTextHandler{
        textHandler: textHandler,
        detector:    NewInteractiveDetector(nil, false),
    }
}

func (h *ConditionalTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
    // 非対話的環境でのみ有効
    return !h.detector.IsInteractive() && h.textHandler.Enabled(ctx, level)
}

func (h *ConditionalTextHandler) Handle(ctx context.Context, r slog.Record) error {
    if h.detector.IsInteractive() {
        return nil // 対話的環境では何もしない
    }
    return h.textHandler.Handle(ctx, r)
}

func (h *ConditionalTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &ConditionalTextHandler{
        textHandler: h.textHandler.WithAttrs(attrs),
        detector:    h.detector,
    }
}

func (h *ConditionalTextHandler) WithGroup(name string) slog.Handler {
    return &ConditionalTextHandler{
        textHandler: h.textHandler.WithGroup(name),
        detector:    h.detector,
    }
}
```

#### 4.1.3 現在のmain.goでの実装箇所
```go
// cmd/runner/main.go の該当部分（line 462付近）の修正
// 変更前:
// textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//     Level: slog.LevelInfo,
// })
// handlers = append(handlers, textHandler)

// 変更後:
originalTextHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})
conditionalTextHandler := logging.NewConditionalTextHandler(originalTextHandler)
handlers = append(handlers, conditionalTextHandler)

// 新しいInteractiveHandlerを追加（ログファイルパス付き）
logFilePath := ""
if config.LogDir != "" {
    logFilePath = filepath.Join(config.LogDir, fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, config.RunID))
}
interactiveHandler := logging.NewInteractiveHandler(slogLevel, logFilePath)
handlers = append(handlers, interactiveHandler)
```

#### 4.1.4 ログファイルヒント出力例
```
Config file parsing failed at line 15: expected key but found ']'. Check TOML syntax
Details: /var/log/runner/host_20250126_143022_01K3M7.json (around line 45)

Cannot access config file '/opt/app/config.toml': permission denied. Check file permissions
Details: /var/log/runner/host_20250126_143022_01K3M7.json (around line 52)
```

### 4.2 現在の `HandlePreExecutionError` の修正
```go
// 既存の関数はそのまま維持し、slogに委託
func HandlePreExecutionError(errorType ErrorType, errorMsg, component, runID string) {
    // 従来のstderr出力を削除し、slogに委託
    slog.Error("Pre-execution error occurred",
        "log_type", string(errorType),
        "message", errorMsg,
        "component", component,
        "run_id", runID,
        "slack_notify", true,
        "message_type", "pre_execution_error",
    )

    // RUN_SUMMARYは互換性のため維持
    fmt.Printf("RUN_SUMMARY run_id=%s exit_code=1 status=pre_execution_error duration_ms=0 verified=0 skipped=0 failed=0 warnings=0 errors=1\n", runID)
}
```

### 4.3 統合方針まとめ

#### 4.3.1 ハンドラーの役割分担
- **Interactive Handler**: 対話的環境でのユーザーフレンドリーなログメッセージ（stderr） + ログファイルヒント
- **Conditional Text Handler**: 非対話的環境での従来のテキスト出力（stdout）
- **JSON Handler**: 詳細ログの永続化（ファイル）+ 行数追跡
- **Slack Handler**: リアルタイム通知

#### 4.3.2 後方互換性の保証
1. 非対話的環境では既存のTextHandler出力を維持
2. `HandlePreExecutionError` のAPIは変更せず
3. RUN_SUMMARY 出力は互換性のため継続
4. ログファイル形式は変更なし

#### 4.3.3 段階的移行戦略
1. **Phase 1**: LogLineTrackerとInteractiveHandlerを追加
2. **Phase 2**: ConditionalTextHandlerを追加して既存TextHandlerを置き換え
3. **Phase 3**: ログファイルヒント機能を有効化
4. **Phase 4**: `HandlePreExecutionError` をslog委託に変更
5. **Phase 5**: 必要に応じてRUN_SUMMARY出力の整理

#### 4.3.4 ログファイルヒント機能の制御
- **表示条件**: エラーレベル以上 && 対話的環境 && ログファイルが設定済み
- **行数精度**: 概算値（±5行程度の誤差を許容）
- **パフォーマンス**: 行数追跡のオーバーヘッドは最小限

## 5. エラーハンドリング

### 5.1 後退戦略（Fallback Strategy）
対話モード検知やメッセージフォーマットに失敗した場合の対応

```go
func (c *DefaultOutputController) HandlePreExecutionError(ctx context.Context, errorType ErrorType, details ErrorDetails) {
    defer func() {
        if r := recover(); r != nil {
            // フォールバック: 既存の形式でエラーを出力
            fmt.Fprintf(os.Stderr, "Error: %s - %s (run_id: %s)\n",
                errorType, details.Message, details.RunID)
        }
    }()

    // 通常の処理
    if c.detector.IsInteractive() {
        c.handleInteractiveError(errorType, details)
    } else {
        c.handleMachineError(errorType, details)
    }
}
```

### 5.2 テスト容易性
```go
type MockInteractiveDetector struct {
    interactive bool
    color       bool
}

func (m *MockInteractiveDetector) IsInteractive() bool { return m.interactive }
func (m *MockInteractiveDetector) SupportsColor() bool { return m.color }
```

## 6. パフォーマンス考慮事項

### 6.1 初期化コスト
- Interactive Detector の初期化は一度のみ実行
- メッセージテンプレートは事前にコンパイル済み状態で保持
- ログレベル判定の最適化

### 6.2 メモリ使用量
- エラー詳細情報の構造化によるメモリ使用量増加は最小限
- テンプレートエンジンは軽量な実装を選択
- Context map の使用は必要最小限に限定

## 7. 将来拡張性

### 7.1 多言語対応
```go
type MessageLocalizer interface {
    LocalizeMessage(errorType ErrorType, details ErrorDetails, locale string) string
}
```

### 7.2 カスタムフォーマッター
```go
type CustomMessageFormatter struct {
    templates map[ErrorType]string
}
```

### 7.3 プラグイン対応
```go
type FormatterPlugin interface {
    Name() string
    FormatMessage(errorType ErrorType, details ErrorDetails) string
}
```
