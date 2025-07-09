# 詳細仕様書: 特権実行設定の統一化

## 1. 仕様概要

### 1.1 変更対象
- `internal/runner/runnertypes/config.go`の`Command`構造体
- `internal/runner/config/loader.go`の設定ローダー
- `sample/config.toml`の設定ファイル
- 関連するドキュメント

### 1.2 変更内容
1. **Phase 1**: 現状の明確化と警告機能
2. **Phase 2**: `user`フィールドの削除と統一化
3. **Phase 3**: 特権実行機能の実装（将来）

## 2. データ構造仕様

### 2.1 現在の構造体定義
```go
// internal/runner/runnertypes/config.go
type Command struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    Dir         string   `toml:"dir"`
    User        string   `toml:"user"`        // 削除対象
    Privileged  bool     `toml:"privileged"`  // 維持
    Timeout     int      `toml:"timeout"`
}
```

### 2.2 Phase 2後の構造体定義
```go
// internal/runner/runnertypes/config.go
type Command struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    Dir         string   `toml:"dir"`
    Privileged  bool     `toml:"privileged"`  // 統一的な権限制御
    Timeout     int      `toml:"timeout"`
}
```

## 3. 設定ファイル仕様

### 3.1 現在の設定例
```toml
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  user = "root"           # 削除対象
  privileged = true       # 維持
```

### 3.2 Phase 2後の設定例
```toml
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  privileged = true       # 統一的な権限制御
```

### 3.3 移行ルール
| 現在の設定 | 移行後の設定 | 動作 |
|-----------|-------------|------|
| `user = "root"` | `privileged = true` | 特権実行（将来実装） |
| `user = "other"` | 削除 | 警告メッセージ、通常実行 |
| `user = ""` | 削除 | 影響なし |
| `privileged = true` | `privileged = true` | 維持 |
| `privileged = false` | `privileged = false` | 維持 |

## 4. API仕様

### 4.1 設定ローダーの拡張

#### Phase 1: 警告機能の追加
```go
// internal/runner/config/loader.go

// validateDeprecatedFields checks for deprecated fields and logs warnings
func (l *Loader) validateDeprecatedFields(cfg *runnertypes.Config) error {
    var warnings []string

    for _, group := range cfg.Groups {
        for _, cmd := range group.Commands {
            if cmd.User != "" {
                warnings = append(warnings, fmt.Sprintf(
                    "command '%s': user field is deprecated and not implemented",
                    cmd.Name))
            }
            if cmd.Privileged {
                warnings = append(warnings, fmt.Sprintf(
                    "command '%s': privileged field is not yet implemented",
                    cmd.Name))
            }
        }
    }

    if len(warnings) > 0 {
        for _, warning := range warnings {
            log.Printf("Warning: %s", warning)
        }
    }

    return nil
}
```

#### Phase 2: 移行処理の追加
```go
// internal/runner/config/loader.go

// migrateDeprecatedFields converts deprecated user field to privileged
func (l *Loader) migrateDeprecatedFields(cfg *runnertypes.Config) error {
    for i, group := range cfg.Groups {
        for j, cmd := range group.Commands {
            if cmd.User == "root" {
                cfg.Groups[i].Commands[j].Privileged = true
                log.Printf("Migrated user='root' to privileged=true for command %s", cmd.Name)
            } else if cmd.User != "" {
                log.Printf("Warning: user field '%s' is ignored for command %s", cmd.User, cmd.Name)
            }
            // User フィールドはクリアする（Phase 2で削除）
        }
    }
    return nil
}
```

### 4.2 エラー型の定義
```go
// internal/runner/config/loader.go

// DeprecationWarning represents a warning for deprecated configuration
type DeprecationWarning struct {
    Field   string
    Command string
    Message string
}

func (w DeprecationWarning) Error() string {
    return fmt.Sprintf("deprecated field '%s' in command '%s': %s",
        w.Field, w.Command, w.Message)
}

// MigrationError represents an error during configuration migration
type MigrationError struct {
    Command string
    Reason  string
}

func (e MigrationError) Error() string {
    return fmt.Sprintf("migration failed for command '%s': %s",
        e.Command, e.Reason)
}
```

## 5. 実行時動作仕様

### 5.1 Phase 1: 警告表示
```
実行時ログ例:
2024/01/01 12:00:00 Warning: command 'system_restart': user field is deprecated and not implemented
2024/01/01 12:00:00 Warning: command 'system_restart': privileged field is not yet implemented
```

### 5.2 Phase 2: 移行処理
```
実行時ログ例:
2024/01/01 12:00:00 Migrated user='root' to privileged=true for command system_restart
2024/01/01 12:00:00 Warning: user field 'deploy' is ignored for command deploy_app
```

### 5.3 Phase 3: 特権実行（将来）
```go
// internal/runner/executor/executor.go

func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // ... 既存のバリデーション ...

    var execCmd *exec.Cmd

    if cmd.Privileged {
        // 特権実行: sudo を使用
        sudoArgs := append([]string{cmd.Cmd}, cmd.Args...)
        execCmd = exec.CommandContext(ctx, "sudo", sudoArgs...)
    } else {
        // 通常実行
        execCmd = exec.CommandContext(ctx, path, cmd.Args...)
    }

    // ... 残りの実行処理 ...
}
```

## 6. バリデーション仕様

### 6.1 Phase 1: 非推奨フィールドの検出
```go
func validateCommand(cmd runnertypes.Command) []ValidationError {
    var errors []ValidationError

    if cmd.User != "" {
        errors = append(errors, ValidationError{
            Type:    "deprecation",
            Field:   "user",
            Message: "user field is deprecated and not implemented",
        })
    }

    if cmd.Privileged {
        errors = append(errors, ValidationError{
            Type:    "unimplemented",
            Field:   "privileged",
            Message: "privileged field is not yet implemented",
        })
    }

    return errors
}
```

### 6.2 Phase 2: 移行後の検証
```go
func validateMigratedCommand(cmd runnertypes.Command) []ValidationError {
    var errors []ValidationError

    // user フィールドが削除されているかチェック
    // (構造体レベルで削除されるため、コンパイル時エラー)

    // privileged フィールドの妥当性チェック
    // (将来の実装に向けた準備)

    return errors
}
```

## 7. テスト仕様

### 7.1 Phase 1: 警告機能のテスト
```go
func TestDeprecationWarnings(t *testing.T) {
    tests := []struct {
        name     string
        config   string
        expected []string
    }{
        {
            name: "user field warning",
            config: `
                [[groups]]
                name = "test"
                [[groups.commands]]
                name = "cmd"
                cmd = "echo"
                user = "root"
            `,
            expected: []string{"user field is deprecated"},
        },
        {
            name: "privileged field warning",
            config: `
                [[groups]]
                name = "test"
                [[groups.commands]]
                name = "cmd"
                cmd = "echo"
                privileged = true
            `,
            expected: []string{"privileged field is not yet implemented"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト実装
        })
    }
}
```

### 7.2 Phase 2: 移行機能のテスト
```go
func TestMigration(t *testing.T) {
    tests := []struct {
        name     string
        input    runnertypes.Command
        expected runnertypes.Command
    }{
        {
            name: "user root to privileged",
            input: runnertypes.Command{
                Name: "test",
                Cmd:  "echo",
                User: "root",
                Privileged: false,
            },
            expected: runnertypes.Command{
                Name: "test",
                Cmd:  "echo",
                Privileged: true,
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト実装
        })
    }
}
```

## 8. 後方互換性仕様

### 8.1 Phase 1: 既存設定の動作保証
- `user`フィールドがある設定ファイルでも正常に動作
- 警告メッセージの表示のみ、機能は変更なし

### 8.2 Phase 2: 段階的移行
- 設定ロード時に`user="root"`を`privileged=true`に自動変換
- 他の`user`値は警告のみで無視
- 既存の`privileged`設定は維持

### 8.3 移行ガイドの提供
```markdown
# 移行ガイド

## 設定ファイルの更新

### 変更前
```toml
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  user = "root"
  privileged = false
```

### 変更後
```toml
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  privileged = true
```

## 自動移行
- `user = "root"` → `privileged = true`
- `user = "other"` → 削除（警告表示）
- `user = ""` → 削除（影響なし）
```

## 9. ドキュメント更新仕様

### 9.1 README.mdの更新
- 新しい設定方法の説明
- 移行ガイドへのリンク
- 非推奨フィールドの注意事項

### 9.2 設定ファイルサンプルの更新
- `sample/config.toml`から`user`フィールドを削除
- `privileged`フィールドの使用例を追加
- 適切なコメントの追加

### 9.3 APIドキュメントの更新
- 構造体定義の更新
- 新しいバリデーション仕様の説明
- エラーハンドリングの仕様

## 10. 実装優先度

### 10.1 High Priority (Phase 1)
1. 警告機能の実装
2. 既存機能の動作確認
3. ドキュメントの更新

### 10.2 Medium Priority (Phase 2)
1. 構造体からの`User`フィールド削除
2. 移行処理の実装
3. 設定ファイルの更新
4. テストケースの作成

### 10.3 Low Priority (Phase 3 - 将来)
1. 特権実行機能の実装
2. セキュリティ機能の追加
3. 監査ログの実装

## 11. 制約事項

### 11.1 技術的制約
- 既存のテストケースが全て通過すること
- 設定ファイルの構文が有効なTOMLであること
- Go 1.23.10以上での動作保証

### 11.2 機能制約
- Phase 1では実際の特権実行は行わない
- Phase 2では構造体の変更を伴う
- Phase 3は将来の実装であり、現在の対象外

### 11.3 互換性制約
- 段階的移行により既存設定の動作を保証
- 適切な警告メッセージの提供
- 明確な移行ガイドの提供
