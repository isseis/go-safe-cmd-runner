# 対話的UI改善 詳細仕様書 (Terminal Package 対応版)

## 1. 実装仕様

### 1.1 パッケージ構成
```
internal/
├── logging/
│   ├── pre_execution_error.go         # 既存（修正）
│   ├── interactive_handler.go         # 新規（terminalパッケージ使用）
│   ├── conditional_text_handler.go    # 新規（terminalパッケージ使用）
│   ├── message_formatter.go           # 新規
│   ├── log_line_tracker.go           # 新規
│   └── message_templates.go           # 新規
└── terminal/
    ├── detector.go                    # 新規：対話性検出
    ├── color.go                       # 新規：カラー対応検出
    ├── preference.go                  # 新規：ユーザー設定管理
    └── capabilities.go                # 新規：端末機能統合
```

## 2. Terminal Package インターフェース定義

### 2.1 Terminal Capabilities (統合インターフェース)
```go
// terminal/capabilities.go
package terminal

import "os"

// Capabilities は端末の全体的な能力を表す統合インターフェース
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool
    GetColorProfile() ColorProfile
    HasExplicitUserPreference() bool
}

// Option は Capabilities の設定オプション
type Option func(*DefaultCapabilities)

// DefaultCapabilities は標準的な端末機能検出実装
type DefaultCapabilities struct {
    detector    InteractiveDetector
    colorSupport ColorDetector
    userPref    UserPreference
    options     capabilitiesOptions
}

type capabilitiesOptions struct {
    forceInteractive *bool
    noColor          bool
}

func NewCapabilities(options ...Option) Capabilities {
    caps := &DefaultCapabilities{
        detector:     NewInteractiveDetector(),
        colorSupport: NewColorDetector(),
        userPref:     NewUserPreference(),
    }

    for _, opt := range options {
        opt(caps)
    }

    if caps.options.forceInteractive != nil {
        caps.detector = NewInteractiveDetector(WithForceInteractive(*caps.options.forceInteractive))
    }

    return caps
}

func WithForceInteractive(force bool) Option {
    return func(c *DefaultCapabilities) {
        c.options.forceInteractive = &force
    }
}

func WithNoColor(noColor bool) Option {
    return func(c *DefaultCapabilities) {
        c.options.noColor = noColor
    }
}

func (c *DefaultCapabilities) SupportsColor() bool {
    if c.options.noColor {
        return false
    }

    userPreference := c.userPref.WantsColor()
    if userPreference != nil {
        // CLICOLOR_FORCE=1 の場合は対話的でない環境でもカラーを有効にする
        if c.userPref.IsColorForced() {
            return *userPreference
        }

        if !c.IsInteractive() && !*userPreference {
            return false
        }
        return *userPreference
    }

    if !c.IsInteractive() {
        return false
    }

    termEnv := os.Getenv("TERM")
    if termEnv == "" || termEnv == "dumb" {
        return false
    }

    return c.colorSupport.SupportsColor()
}
```

### 2.2 Interactive Detector (対話性検出)
```go
// terminal/detector.go
package terminal

import (
    "os"
    "golang.org/x/term"
)

type InteractiveDetector interface {
    IsInteractive() bool
    IsTerminal() bool
    IsCIEnvironment() bool
}

type DetectorOption func(*DefaultInteractiveDetector)

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

func WithForceInteractive(force bool) DetectorOption {
    return func(d *DefaultInteractiveDetector) {
        d.forceInteractive = &force
    }
}

func (d *DefaultInteractiveDetector) IsInteractive() bool {
    if d.forceInteractive != nil {
        return *d.forceInteractive
    }

    if d.IsCIEnvironment() {
        return false
    }

    return d.IsTerminal()
}

func (d *DefaultInteractiveDetector) IsTerminal() bool {
    return term.IsTerminal(int(d.stdout.Fd()))
}

func (d *DefaultInteractiveDetector) IsCIEnvironment() bool {
    return os.Getenv("CI") != ""
}
```

### 2.3 Color Detector (カラー対応検出)
```go
// terminal/color.go
package terminal

import (
    "os"
    "strings"
)

type ColorDetector interface {
    SupportsColor() bool
    GetColorProfile() ColorProfile
    IsColorCapableTerminal() bool
}

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

    if d.colortermEnv == "truecolor" || d.colortermEnv == "24bit" {
        return ColorProfileTrueColor
    }

    if strings.Contains(d.termEnv, "256color") {
        return ColorProfile256
    }

    return ColorProfileBasic
}

func (d *DefaultColorDetector) IsColorCapableTerminal() bool {
    if d.colortermEnv == "truecolor" || d.colortermEnv == "24bit" {
        return true
    }

    colorTerminals := []string{
        "xterm", "xterm-color", "xterm-256color",
        "screen", "screen-256color",
        "tmux", "tmux-256color",
        "rxvt", "rxvt-unicode", "rxvt-256color",
        "linux", "cygwin", "konsole", "gnome", "vte",
    }

    for _, colorTerm := range colorTerminals {
        if strings.HasPrefix(d.termEnv, colorTerm) {
            return true
        }
    }

    if strings.HasPrefix(d.termEnv, "screen.") {
        suffix := strings.TrimPrefix(d.termEnv, "screen.")
        for _, colorTerm := range colorTerminals {
            if strings.HasPrefix(suffix, colorTerm) {
                return true
            }
        }
    }

    if strings.HasPrefix(d.termEnv, "tmux.") {
        suffix := strings.TrimPrefix(d.termEnv, "tmux.")
        for _, colorTerm := range colorTerminals {
            if strings.HasPrefix(suffix, colorTerm) {
                return true
            }
        }
    }

    colorSuffixes := []string{"-color", "-256color", "-88color", "color"}
    for _, suffix := range colorSuffixes {
        if strings.HasSuffix(d.termEnv, suffix) {
            return true
        }
    }

    return false
}
```

### 2.4 User Preference (ユーザー設定管理)
```go
// terminal/preference.go
package terminal

import "os"

type UserPreference interface {
    WantsColor() *bool
    HasExplicitPreference() bool
    IsColorForced() bool
    IsColorDisabled() bool
}

type DefaultUserPreference struct {
    noColorEnv      string
    cliColorEnv     string
    cliColorForceEnv string
}

func NewUserPreference() UserPreference {
    return &DefaultUserPreference{
        noColorEnv:      os.Getenv("NO_COLOR"),
        cliColorEnv:     os.Getenv("CLICOLOR"),
        cliColorForceEnv: os.Getenv("CLICOLOR_FORCE"),
    }
}

func (p *DefaultUserPreference) WantsColor() *bool {
    // CLICOLOR_FORCE=1 が設定されている場合は他の条件を無視してカラーを強制有効化
    if p.cliColorForceEnv == "1" {
        return &[]bool{true}[0]
    }

    // NO_COLOR環境変数による明示的な無効化（業界標準）
    if p.noColorEnv != "" {
        return &[]bool{false}[0]
    }

    // CLICOLOR環境変数によるユーザー希望の判定（Unix系標準）
    if p.cliColorEnv != "" {
        if p.cliColorEnv != "0" {
            return &[]bool{true}[0]
        } else {
            return &[]bool{false}[0]
        }
    }

    return nil
}

func (p *DefaultUserPreference) HasExplicitPreference() bool {
    return p.noColorEnv != "" || p.cliColorEnv != "" || p.cliColorForceEnv != ""
}

func (p *DefaultUserPreference) IsColorForced() bool {
    return p.cliColorForceEnv == "1" || (p.cliColorEnv != "" && p.cliColorEnv != "0")
}

func (p *DefaultUserPreference) IsColorDisabled() bool {
    return p.noColorEnv != "" || p.cliColorEnv == "0"
}
```

## 3. Logging Package の修正

### 3.1 Interactive Handler (terminalパッケージ対応版)
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

func NewInteractiveHandler(level slog.Level, logFilePath string, options ...terminal.Option) *InteractiveHandler {
    return &InteractiveHandler{
        capabilities: terminal.NewCapabilities(options...),
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
    // Enabled() メソッドで既に対話性がチェックされているため、ここでは重複チェック不要
    colorSupported := h.capabilities.SupportsColor()
    message := h.formatter.FormatRecordWithColor(r, colorSupported)

    if r.Level >= slog.LevelError && h.logFilePath != "" {
        estimatedLine := h.lineTracker.EstimateCurrentLine()
        logHint := h.formatter.FormatLogFileHint(h.logFilePath, estimatedLine, colorSupported)
        message = message + "\n" + logHint
    }

    _, err := fmt.Fprintf(h.output, "%s\n", message)
    h.lineTracker.IncrementLine()

    return err
}
```

### 3.2 Message Formatter (カラーリング統合版)
```go
// logging/message_formatter.go
package logging

import "log/slog"

type MessageFormatter interface {
    FormatRecordWithColor(r slog.Record, colorSupported bool) string
    FormatLogFileHint(logFilePath string, estimatedLine int, colorSupported bool) string
}

type DefaultMessageFormatter struct{}

func NewMessageFormatter() MessageFormatter {
    return &DefaultMessageFormatter{}
}

func (f *DefaultMessageFormatter) FormatRecordWithColor(r slog.Record, colorSupported bool) string {
    // シンプル実装：ANSIエスケープシーケンス不使用
    // レベル別にプレフィックス記号で視覚的区別
    levelPrefix := f.getLevelPrefix(r.Level)
    timestamp := r.Time.Format("15:04:05")
    return fmt.Sprintf("%s [%s] %s", levelPrefix, timestamp, r.Message)
}

func (f *DefaultMessageFormatter) FormatLogFileHint(logFilePath string, estimatedLine int, colorSupported bool) string {
    // 将来拡張：colorSupportedがtrueの場合はカラー対応
    // 今回はシンプル実装のため常に同じフォーマット
    return fmt.Sprintf("Details: %s (around line %d)", logFilePath, estimatedLine)
}

func (f *DefaultMessageFormatter) getLevelPrefix(level slog.Level) string {
    switch level {
    case slog.LevelDebug:
        return "[DEBUG]"
    case slog.LevelInfo:
        return "[INFO] "
    case slog.LevelWarn:
        return "[WARN] "
    case slog.LevelError:
        return "[ERROR]"
    default:
        return "[LOG]  "
    }
}
```

### 3.3 Conditional Text Handler (terminalパッケージ対応版)
```go
// logging/conditional_text_handler.go
package logging

import (
    "context"
    "log/slog"

    "internal/terminal"
)

type ConditionalTextHandler struct {
    textHandler  slog.Handler
    capabilities terminal.Capabilities
}

func NewConditionalTextHandler(textHandler slog.Handler, options ...terminal.Option) *ConditionalTextHandler {
    return &ConditionalTextHandler{
        textHandler:  textHandler,
        capabilities: terminal.NewCapabilities(options...),
    }
}

func (h *ConditionalTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
    return !h.capabilities.IsInteractive() && h.textHandler.Enabled(ctx, level)
}

func (h *ConditionalTextHandler) Handle(ctx context.Context, r slog.Record) error {
    if h.capabilities.IsInteractive() {
        return nil
    }
    return h.textHandler.Handle(ctx, r)
}
```

## 4. テスト仕様

### 4.1 Terminal Package のテスト
```go
func TestCapabilities(t *testing.T) {
    tests := []struct {
        name        string
        options     []terminal.Option
        envVars     map[string]string
        expected    bool
    }{
        {
            name:    "force_interactive_with_color",
            options: []terminal.Option{terminal.WithForceInteractive(true)},
            envVars: map[string]string{"TERM": "xterm-256color"},
            expected: true,
        },
        {
            name:    "no_color_flag",
            options: []terminal.Option{terminal.WithNoColor(true)},
            envVars: map[string]string{"TERM": "xterm-256color"},
            expected: false,
        },
        {
            name:    "clicolor_preference",
            options: []terminal.Option{terminal.WithForceInteractive(true)},
            envVars: map[string]string{"CLICOLOR": "1", "TERM": "unknown"},
            expected: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 環境変数設定
            for key, value := range tt.envVars {
                os.Setenv(key, value)
                defer os.Unsetenv(key)
            }

            caps := terminal.NewCapabilities(tt.options...)
            result := caps.SupportsColor()
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

## 5. システム統合

### 5.1 setupLogging の修正
```go
func setupLogging(config Config) error {
    level := parseLogLevel(config.Level)
    var handlers []slog.Handler

    // Terminal capabilities のオプション設定
    var terminalOptions []terminal.Option
    if config.ForceColor {
        // 将来的な拡張: --force-color オプション対応
    }
    if config.NoColor {
        terminalOptions = append(terminalOptions, terminal.WithNoColor(true))
    }

    // 1. ConditionalTextHandler（非対話的環境用）
    originalTextHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    conditionalTextHandler := NewConditionalTextHandler(originalTextHandler, terminalOptions...)
    handlers = append(handlers, conditionalTextHandler)

    // 2. InteractiveHandler（対話的環境用）
    logFilePath := ""
    if config.LogDir != "" {
        logFilePath = generateLogPath(config)
    }
    interactiveHandler := NewInteractiveHandler(level, logFilePath, terminalOptions...)
    handlers = append(handlers, interactiveHandler)

    // 3. JSONHandler（ログファイル出力）
    if config.LogDir != "" {
        logPath := generateLogPath(config)
        jsonHandler := createJSONHandler(logPath, level, config)
        handlers = append(handlers, jsonHandler)
    }

    // 4. SlackHandler（通知）
    if config.SlackWebhookURL != "" {
        slackHandler, err := NewSlackHandler(config.SlackWebhookURL, config.RunID)
        if err != nil {
            return err
        }
        handlers = append(handlers, slackHandler)
    }

    multiHandler, err := NewMultiHandler(handlers...)
    if err != nil {
        return err
    }

    logger := slog.New(NewRedactedHandler(multiHandler))
    slog.SetDefault(logger)
    return nil
}
```

## 6. 利点と改善点

### 6.1 Terminal Package の利点
1. **責任の明確な分離**: 端末機能とログ機能が分離
2. **再利用性**: 他のパッケージからも利用可能
3. **テスタビリティ**: 各機能の独立したテスト
4. **拡張性**: 新しい端末機能の追加が容易
5. **保守性**: 複雑なカラー検出ロジックが分離

### 6.2 設計改善点
1. **オプション駆動**: 設定の柔軟性向上
2. **インターフェース設計**: モック化とテストの容易性
3. **階層化**: Capabilities → Detector/ColorDetector/UserPreference
4. **型安全性**: ColorProfile等の型定義による安全性向上

### 6.3 将来の拡張可能性
1. **高度なカラー機能**: TrueColor対応の詳細制御
2. **プラットフォーム固有機能**: Windows/macOS固有の端末検出
3. **動的設定**: 実行時設定変更への対応
4. **プラグイン対応**: 外部端末検出ライブラリとの統合
