# Task 0036: LogLevel 型の導入 - 実装計画

## 1. 概要

LogLevel を専用のカスタム型として定義し、`encoding.TextUnmarshaler` インターフェースを実装することで、TOML パース時点でのバリデーションと型安全性を実現する。

## 2. 実装フェーズ

### フェーズ 1: 型定義とテスト (TDD)
- [ ] 1.1. テストファイルの作成
- [ ] 1.2. LogLevel 型の基本テスト追加 (失敗することを確認)
- [ ] 1.3. LogLevel 型と定数の定義
- [ ] 1.4. UnmarshalText() の実装
- [ ] 1.5. ToSlogLevel() の実装
- [ ] 1.6. String() の実装
- [ ] 1.7. テストの成功を確認
- [ ] 1.8. コミット: "feat: add LogLevel type with validation"

### フェーズ 2: 既存コードの更新
- [ ] 2.1. GlobalConfig.LogLevel の型変更
- [ ] 2.2. bootstrap.LoggerConfig.Level の型変更
- [ ] 2.3. cmd/runner/main.go のコマンドライン引数処理更新
- [ ] 2.4. 既存テストの更新
- [ ] 2.5. テストの成功を確認
- [ ] 2.6. コミット: "refactor: use LogLevel type in GlobalConfig"

### フェーズ 3: 統合テストと検証
- [ ] 3.1. 統合テストの実行 (make test)
- [ ] 3.2. エラーメッセージの手動確認
- [ ] 3.3. ドキュメントの更新
- [ ] 3.4. コミット: "docs: update for LogLevel type"
- [ ] 3.5. PR 作成とレビュー

## 3. 詳細実装計画

### 3.1. フェーズ 1: 型定義とテスト (TDD)

#### 3.1.1. テストファイルの作成

**ファイル**: `internal/runner/runnertypes/loglevel_test.go`

**内容**:
```go
package runnertypes

import (
	"log/slog"
	"testing"
)

// Test valid log levels
func TestLogLevel_UnmarshalText_ValidLevels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected LogLevel
	}{
		{"debug", "debug", LogLevelDebug},
		{"info", "info", LogLevelInfo},
		{"warn", "warn", LogLevelWarn},
		{"error", "error", LogLevelError},
		{"empty defaults to info", "", LogLevelInfo},
		{"uppercase DEBUG", "DEBUG", LogLevelDebug},
		{"uppercase INFO", "INFO", LogLevelInfo},
		{"uppercase WARN", "WARN", LogLevelWarn},
		{"uppercase ERROR", "ERROR", LogLevelError},
		{"mixed case Debug", "Debug", LogLevelDebug},
		{"mixed case Info", "Info", LogLevelInfo},
		{"mixed case Warn", "Warn", LogLevelWarn},
		{"mixed case Error", "Error", LogLevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var level LogLevel
			err := level.UnmarshalText([]byte(tt.input))
			if err != nil {
				t.Errorf("UnmarshalText() error = %v, want nil", err)
			}
			if level != tt.expected {
				t.Errorf("UnmarshalText() = %v, want %v", level, tt.expected)
			}
		})
	}
}

// Test invalid log levels
func TestLogLevel_UnmarshalText_InvalidLevels(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"typo", "debg"},
		{"unknown", "unknown"},
		{"number", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var level LogLevel
			err := level.UnmarshalText([]byte(tt.input))
			if err == nil {
				t.Errorf("UnmarshalText() error = nil, want error for input %q", tt.input)
			}
		})
	}
}

// Test ToSlogLevel conversion with valid levels
func TestLogLevel_ToSlogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected slog.Level
		wantErr  bool
	}{
		{"debug", LogLevelDebug, slog.LevelDebug, false},
		{"info", LogLevelInfo, slog.LevelInfo, false},
		{"warn", LogLevelWarn, slog.LevelWarn, false},
		{"error", LogLevelError, slog.LevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slogLevel, err := tt.level.ToSlogLevel()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToSlogLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if slogLevel != tt.expected {
				t.Errorf("ToSlogLevel() = %v, want %v", slogLevel, tt.expected)
			}
		})
	}
}

// Test ToSlogLevel conversion with invalid levels
// LogLevel is a string alias, so invalid values can be set directly without going through UnmarshalText
func TestLogLevel_ToSlogLevel_InvalidLevels(t *testing.T) {
	tests := []struct {
		name    string
		level   LogLevel
		wantErr bool
	}{
		{"typo dbg", LogLevel("dbg"), true},
		{"typo debg", LogLevel("debg"), true},
		{"unknown value", LogLevel("unknown"), true},
		{"numeric string", LogLevel("1"), true},
		{"uppercase DEBUG", LogLevel("DEBUG"), true},
		{"mixed case Debug", LogLevel("Debug"), true},
		{"whitespace", LogLevel(" debug"), true},
		{"empty string", LogLevel(""), false}, // empty string should work (defaults to info)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slogLevel, err := tt.level.ToSlogLevel()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToSlogLevel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// For invalid levels, we just check that an error was returned
			if tt.wantErr && err == nil {
				t.Errorf("ToSlogLevel() expected error for invalid level %q, got nil", tt.level)
			}
			// For empty string, it should default to info level
			if tt.level == LogLevel("") && !tt.wantErr && slogLevel != slog.LevelInfo {
				t.Errorf("ToSlogLevel() empty string should default to info, got %v", slogLevel)
			}
		})
	}
}

// Test String method
func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected string
	}{
		{"debug", LogLevelDebug, "debug"},
		{"info", LogLevelInfo, "info"},
		{"warn", LogLevelWarn, "warn"},
		{"error", LogLevelError, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}
```

**手順**:
1. ファイルを作成
2. テストを実行 (`go test -tags test -v ./internal/runner/runnertypes`)
3. コンパイルエラーまたはテスト失敗を確認 (LogLevel 型がまだ定義されていないため)

#### 3.1.2. LogLevel 型の定義

**ファイル**: `internal/runner/runnertypes/config.go`

**追加位置**: `GlobalConfig` 定義の前 (行 14 付近)

**内容**:
```go
// LogLevel represents the logging level for the application.
// Valid values: debug, info, warn, error
type LogLevel string

const (
	// LogLevelDebug enables debug-level logging
	LogLevelDebug LogLevel = "debug"

	// LogLevelInfo enables info-level logging (default)
	LogLevelInfo LogLevel = "info"

	// LogLevelWarn enables warning-level logging
	LogLevelWarn LogLevel = "warn"

	// LogLevelError enables error-level logging only
	LogLevelError LogLevel = "error"
)

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// This enables validation during TOML parsing.
func (l *LogLevel) UnmarshalText(text []byte) error {
	s := strings.ToLower(string(text))
	switch LogLevel(s) {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		*l = LogLevel(s)
		return nil
	case "":
		// Empty string defaults to info level
		*l = LogLevelInfo
		return nil
	default:
		return fmt.Errorf("invalid log level %q: must be one of: debug, info, warn, error", string(text))
	}
}

// ToSlogLevel converts LogLevel to slog.Level for use with the slog package.
func (l LogLevel) ToSlogLevel() (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(l)); err != nil {
		return slog.LevelInfo, fmt.Errorf("failed to convert log level %q to slog.Level: %w", l, err)
	}
	return level, nil
}

// String returns the string representation of LogLevel.
func (l LogLevel) String() string {
	return string(l)
}
```

**手順**:
1. 上記コードを `config.go` に追加
2. import に `log/slog` および `strings` を追加
3. テストを実行 (`go test -tags test -v ./internal/runner/runnertypes`)
4. すべてのテストが成功することを確認
5. リンターを実行 (`make lint`)
6. フォーマッターを実行 (`make fmt`)

#### 3.1.3. コミット

```bash
git add internal/runner/runnertypes/config.go
git add internal/runner/runnertypes/loglevel_test.go
git commit -m "feat: add LogLevel type with validation

Add LogLevel custom type with the following features:
- Four log levels: debug, info, warn, error
- UnmarshalText() for TOML validation
- ToSlogLevel() for conversion to slog.Level
- String() for string representation
- Comprehensive unit tests covering valid/invalid inputs

This enables early error detection during TOML parsing and
improves type safety by using constants instead of string literals.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

### 3.2. フェーズ 2: 既存コードの更新

#### 3.2.1. GlobalConfig.LogLevel の型変更

**ファイル**: `internal/runner/runnertypes/config.go`

**変更箇所**: GlobalConfig 構造体 (現在の行 25)

**変更内容**:
```go
// Before:
LogLevel string `toml:"log_level"` // Log level (debug, info, warn, error)

// After:
LogLevel LogLevel `toml:"log_level"` // Log level (debug, info, warn, error)
```

**手順**:
1. フィールドの型を `string` から `LogLevel` に変更
2. 必要に応じてコメントを更新

#### 3.2.2. bootstrap.LoggerConfig.Level の型変更

**ファイル**: `internal/runner/bootstrap/logger.go`

**変更箇所 1**: LoggerConfig 構造体 (現在の行 22)

**変更内容**:
```go
// Before:
Level string

// After:
Level LogLevel
```

**変更箇所 2**: SetupLoggerWithConfig 関数 (現在の行 40-44)

**変更内容**:
```go
// Before:
var slogLevel slog.Level
if err := slogLevel.UnmarshalText([]byte(config.Level)); err != nil {
    slogLevel = slog.LevelInfo // Default to info on parse error
    invalidLogLevel = true
}

// After:
slogLevel, err := config.Level.ToSlogLevel()
if err != nil {
    slogLevel = slog.LevelInfo // Default to info on parse error
    invalidLogLevel = true
}
```

**変更箇所 3**: import 文の更新

**変更内容**:
```go
import (
    // ... existing imports ...
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)
```

**手順**:
1. LoggerConfig.Level の型を変更
2. SetupLoggerWithConfig 関数内の変換処理を `ToSlogLevel()` を使用するよう変更
3. import に `runnertypes` を追加
4. テストを実行 (`go test -tags test -v ./internal/runner/bootstrap`)

#### 3.2.3. cmd/runner/main.go の更新

**ファイル**: `cmd/runner/main.go`

**変更箇所**: run 関数内のコマンドライン引数処理 (現在の行 213-215)

**変更内容**:
```go
// Before:
if *logLevel != "" {
    cfg.Global.LogLevel = *logLevel
}

// After:
if *logLevel != "" {
    var level runnertypes.LogLevel
    if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
        return &logging.PreExecutionError{
            Type:      logging.ErrorTypeConfigError,
            Message:   fmt.Sprintf("Invalid log level %q: %v", *logLevel, err),
            Component: "main",
            RunID:     runID,
        }
    }
    cfg.Global.LogLevel = level
}
```

**手順**:
1. コマンドライン引数の処理を更新
2. エラーハンドリングを追加
3. テストを実行 (`go test -tags test -v ./cmd/runner`)

#### 3.2.4. 既存テストの更新

**影響を受けるテストファイル**:
- `internal/runner/runnertypes/config_test.go`
- `internal/runner/bootstrap/logger_test.go`
- `internal/runner/runner_test.go`

**更新内容**:
- `string` リテラルの代わりに `LogLevel` 定数を使用
- 必要に応じてテストケースを追加

**手順**:
1. 各テストファイルを確認
2. `string` リテラルを `LogLevel` 定数に置き換え
3. テストを実行して成功を確認

#### 3.2.5. コミット

```bash
git add internal/runner/runnertypes/config.go
git add internal/runner/bootstrap/logger.go
git add cmd/runner/main.go
git add internal/runner/runnertypes/config_test.go
git add internal/runner/bootstrap/logger_test.go
git add internal/runner/runner_test.go
git commit -m "refactor: use LogLevel type in GlobalConfig and related code

Update GlobalConfig.LogLevel from string to LogLevel type:
- Change GlobalConfig.LogLevel field type
- Update bootstrap.LoggerConfig.Level type
- Update cmd/runner/main.go command-line argument handling
- Update existing tests to use LogLevel constants

This provides compile-time type safety and validates log levels
during TOML parsing instead of at runtime.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

### 3.3. フェーズ 3: 統合テストと検証

#### 3.3.1. 統合テストの実行

**手順**:
1. 全テストを実行
   ```bash
   make test
   ```
2. ビルドを実行
   ```bash
   make build
   ```
3. リンターを実行
   ```bash
   make lint
   ```

#### 3.3.2. エラーメッセージの手動確認

**テストケース 1**: 無効なログレベルを含む TOML ファイル

**ファイル**: `/tmp/test_invalid_loglevel.toml`
```toml
version = "1"

[global]
log_level = "debg"
timeout = 300
workdir = "/tmp"
```

**実行**:
```bash
./build/runner -config /tmp/test_invalid_loglevel.toml
```

**期待される動作**:
- アプリケーションが起動せず、エラーメッセージが表示される
- エラーメッセージに以下が含まれる:
  - 無効な値 "debg"
  - 有効な値のリスト (debug, info, warn, error)

**テストケース 2**: 有効なログレベル

**ファイル**: `/tmp/test_valid_loglevel.toml`
```toml
version = "1"

[global]
log_level = "debug"
timeout = 300
workdir = "/tmp"
```

**実行**:
```bash
./build/runner -config /tmp/test_valid_loglevel.toml
```

**期待される動作**:
- アプリケーションが正常に起動
- ログレベルが debug に設定される

**テストケース 3**: コマンドライン引数での無効なログレベル

**実行**:
```bash
./build/runner -config /tmp/test_valid_loglevel.toml -log-level debg
```

**期待される動作**:
- アプリケーションが起動せず、エラーメッセージが表示される

#### 3.3.3. ドキュメントの更新

**更新が必要なドキュメント**:
1. ユーザー向けドキュメント (存在する場合)
2. コード内のコメント (既に更新済み)

#### 3.3.4. コミット

```bash
git add docs/
git commit -m "docs: update documentation for LogLevel type

Update user-facing documentation to reflect LogLevel type:
- Valid log level values (debug, info, warn, error)
- Error messages for invalid values
- Default behavior (info)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

#### 3.3.5. PR 作成

**PR タイトル**: `feat: introduce LogLevel type with TOML validation`

**PR 説明**:
```markdown
## Summary
Introduces a custom `LogLevel` type to replace the string-based log level configuration. This provides early error detection during TOML parsing and improves type safety.

## Changes
- Add `LogLevel` type with four constants: `LogLevelDebug`, `LogLevelInfo`, `LogLevelWarn`, `LogLevelError`
- Implement `encoding.TextUnmarshaler` interface for TOML validation
- Add `ToSlogLevel()` method for conversion to `slog.Level`
- Update `GlobalConfig.LogLevel` field type
- Update `bootstrap.LoggerConfig.Level` field type
- Update command-line argument handling in `cmd/runner/main.go`
- Add comprehensive unit tests

## Benefits
1. **Early error detection**: Invalid log levels are detected during TOML parsing, not at runtime
2. **Type safety**: Use constants instead of string literals, enabling compile-time checks
3. **Better error messages**: Clear error messages with valid value lists
4. **Consistency**: Follows the same pattern as `RiskLevel` type

## Testing
- [x] Unit tests for `LogLevel` type (100% coverage)
- [x] Updated existing tests to use new type
- [x] Manual testing with invalid log levels
- [x] Integration tests pass

## Backward Compatibility
- ✅ TOML configuration format unchanged
- ✅ Existing valid configuration files work without modification
- ✅ Default behavior (info level) maintained
```

## 4. チェックリスト

### 開発前
- [ ] 要件定義書のレビュー
- [ ] アーキテクチャ設計書のレビュー
- [ ] 実装計画書のレビュー

### フェーズ 1
- [ ] テストファイルの作成
- [ ] LogLevel 型のテスト追加 (TDD)
- [ ] LogLevel 型の実装
- [ ] テスト成功確認
- [ ] コミット

### フェーズ 2
- [ ] GlobalConfig.LogLevel の型変更
- [ ] bootstrap.LoggerConfig の更新
- [ ] cmd/runner/main.go の更新
- [ ] 既存テストの更新
- [ ] テスト成功確認
- [ ] コミット

### フェーズ 3
- [ ] 統合テスト実行 (make test)
- [ ] ビルド確認 (make build)
- [ ] リンター実行 (make lint)
- [ ] エラーメッセージの手動確認
- [ ] ドキュメント更新
- [ ] コミット
- [ ] PR 作成

### レビュー後
- [ ] レビューコメントの対応
- [ ] 最終テスト
- [ ] マージ

## 5. トラブルシューティング

### 問題 1: TOML パーサーが UnmarshalText を呼ばない

**症状**: バリデーションが実行されない

**原因**: ポインタレシーバーが正しく実装されていない

**解決策**:
```go
// 正しい実装
func (l *LogLevel) UnmarshalText(text []byte) error { ... }

// 間違った実装
func (l LogLevel) UnmarshalText(text []byte) error { ... }
```

### 問題 2: 既存テストが失敗する

**症状**: 型の不一致エラー

**原因**: 文字列リテラルが使用されている

**解決策**: 文字列リテラルを LogLevel 定数に置き換える
```go
// Before:
cfg.Global.LogLevel = "debug"

// After:
cfg.Global.LogLevel = runnertypes.LogLevelDebug
```

### 問題 3: コマンドライン引数の処理でパニック

**症状**: nil ポインタ参照

**原因**: LogLevel のゼロ値が空文字列

**解決策**: UnmarshalText で空文字列を適切に処理 (既に実装済み)

## 6. 完了条件

以下のすべてが満たされた場合、このタスクは完了とする:

1. ✅ LogLevel 型が定義され、すべてのメソッドが実装されている
2. ✅ 単体テストのカバレッジが 100%
3. ✅ 既存のすべてのテストが成功
4. ✅ リンターエラーなし
5. ✅ 統合テストが成功
6. ✅ 手動テストで無効なログレベルがエラーになることを確認
7. ✅ ドキュメントが更新されている
8. ✅ PR が作成され、レビュー済み
9. ✅ マージ済み

## 7. 推定工数

| フェーズ | 工数 |
|---------|------|
| フェーズ 1: 型定義とテスト | 1-2 時間 |
| フェーズ 2: 既存コードの更新 | 2-3 時間 |
| フェーズ 3: 統合テストと検証 | 1-2 時間 |
| **合計** | **4-7 時間** |

## 8. 参考資料

- [encoding.TextUnmarshaler interface](https://pkg.go.dev/encoding#TextUnmarshaler)
- [log/slog package](https://pkg.go.dev/log/slog)
- [go-toml/v2 documentation](https://github.com/pelletier/go-toml)
- 既存の RiskLevel 実装: `internal/runner/runnertypes/config.go:163-234`
