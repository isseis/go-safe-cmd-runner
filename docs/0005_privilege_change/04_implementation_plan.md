# 実装計画書: 特権実行設定の統一化

## 1. 実装概要

### 1.1 プロジェクト期間
- **Phase 1**: 即座実行可能（影響範囲: 最小）
- **Phase 2**: 次期バージョン（影響範囲: 中程度）
- **Phase 3**: 将来バージョン（影響範囲: 大）

### 1.2 実装方針
- 段階的実装により既存機能への影響を最小化
- 各フェーズで完全なテストを実施
- 明確な移行ガイドの提供

## 2. Phase 1: 現状の明確化と警告機能

### 2.1 実装項目
- [ ] ドキュメント更新（未実装状態の明記）
- [ ] 警告機能の実装
- [ ] 既存機能の動作確認
- [ ] テストケースの作成

### 2.2 具体的な実装手順

#### 2.2.1 設定ローダーの警告機能追加
```go
// internal/runner/config/loader.go に追加

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

// LoadConfigを修正
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
    // ... 既存のロード処理 ...

    // 警告チェック追加
    if err := l.validateDeprecatedFields(&cfg); err != nil {
        // エラーではなく警告として扱う
        log.Printf("Configuration warnings: %v", err)
    }

    return &cfg, nil
}
```

#### 2.2.2 sample/config.tomlの更新
```toml
# 各コマンドに未実装状態のコメントを追加
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  user = "root"        # WARNING: user field is not implemented
  privileged = true    # WARNING: privileged field is not implemented
```

#### 2.2.3 ドキュメント更新
- README.mdに未実装状態の説明を追加
- 設定ファイルの各フィールドの実装状況を明記

#### 2.2.4 テストケースの作成
```go
// internal/runner/config/loader_test.go に追加

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
            // ログ出力をキャプチャしてテスト
        })
    }
}
```

## 3. Phase 2: userフィールドの削除

### 3.1 実装項目
- [ ] 構造体からUserフィールドを削除
- [ ] 移行処理の実装
- [ ] 設定ファイルの更新
- [ ] テストケースの更新
- [ ] 移行ガイドの作成

### 3.2 具体的な実装手順

#### 3.2.1 構造体の変更
```go
// internal/runner/runnertypes/config.go

// 変更前
type Command struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    Dir         string   `toml:"dir"`
    User        string   `toml:"user"`        // 削除
    Privileged  bool     `toml:"privileged"`
    Timeout     int      `toml:"timeout"`
}

// 変更後
type Command struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    Dir         string   `toml:"dir"`
    Privileged  bool     `toml:"privileged"`
    Timeout     int      `toml:"timeout"`
}
```

#### 3.2.2 移行処理の実装
```go
// internal/runner/config/loader.go

// 移行用の一時的な構造体
type legacyCommand struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    Dir         string   `toml:"dir"`
    User        string   `toml:"user"`        // 移行処理用
    Privileged  bool     `toml:"privileged"`
    Timeout     int      `toml:"timeout"`
}

func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
    // ... 既存のロード処理 ...

    // 一時的にlegacyCommandでロード
    var legacyCfg struct {
        Version   string                    `toml:"version"`
        Global    runnertypes.GlobalConfig  `toml:"global"`
        Templates map[string]runnertypes.TemplateConfig `toml:"templates"`
        Groups    []struct {
            Name        string         `toml:"name"`
            Description string         `toml:"description"`
            Priority    int            `toml:"priority"`
            DependsOn   []string       `toml:"depends_on"`
            Template    string         `toml:"template"`
            Commands    []legacyCommand `toml:"commands"`
        } `toml:"groups"`
    }

    if err := toml.Unmarshal(data, &legacyCfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    // 新しい構造体に変換
    cfg := &runnertypes.Config{
        Version:   legacyCfg.Version,
        Global:    legacyCfg.Global,
        Templates: legacyCfg.Templates,
    }

    // コマンドの移行処理
    for _, group := range legacyCfg.Groups {
        newGroup := runnertypes.CommandGroup{
            Name:        group.Name,
            Description: group.Description,
            Priority:    group.Priority,
            DependsOn:   group.DependsOn,
            Template:    group.Template,
        }

        for _, cmd := range group.Commands {
            newCmd := runnertypes.Command{
                Name:        cmd.Name,
                Description: cmd.Description,
                Cmd:         cmd.Cmd,
                Args:        cmd.Args,
                Env:         cmd.Env,
                Dir:         cmd.Dir,
                Privileged:  cmd.Privileged,
                Timeout:     cmd.Timeout,
            }

            // 移行処理
            if cmd.User == "root" {
                newCmd.Privileged = true
                log.Printf("Migrated user='root' to privileged=true for command %s", cmd.Name)
            } else if cmd.User != "" {
                log.Printf("Warning: user field '%s' is ignored for command %s", cmd.User, cmd.Name)
            }

            newGroup.Commands = append(newGroup.Commands, newCmd)
        }

        cfg.Groups = append(cfg.Groups, newGroup)
    }

    // ... 残りの処理 ...

    return cfg, nil
}
```

#### 3.2.3 設定ファイルの更新
```toml
# sample/config.toml の更新

# 変更前
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  user = "root"
  privileged = true

# 変更後
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  privileged = true    # Unified privilege control
```

#### 3.2.4 移行ガイドの作成
```markdown
# 移行ガイド

## 設定ファイルの更新方法

### 自動移行
システムが自動的に以下の変換を行います：
- `user = "root"` → `privileged = true`
- `user = "other"` → 削除（警告表示）

### 手動更新推奨
自動移行に依存せず、以下の手順で手動更新することを推奨します：

1. 設定ファイルをバックアップ
2. `user = "root"`を`privileged = true`に変更
3. 他の`user`フィールドを削除
4. 設定ファイルをテスト

### 例
```toml
# 変更前
[[groups.commands]]
  name = "system_restart"
  cmd = "systemctl"
  user = "root"
  privileged = false

# 変更後
[[groups.commands]]
  name = "system_restart"
  cmd = "systemctl"
  privileged = true
```
```

## 4. Phase 3: 特権実行機能の実装（将来）

### 4.1 実装項目（参考）
- [ ] 実行エンジンでの特権実行機能
- [ ] セキュリティ制御の実装
- [ ] 監査ログの実装
- [ ] 包括的なテスト

### 4.2 実装概要（参考）
```go
// internal/runner/executor/executor.go

func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // ... 既存のバリデーション ...

    var execCmd *exec.Cmd

    if cmd.Privileged {
        // 特権実行の場合
        execCmd = e.createPrivilegedCommand(ctx, cmd)
    } else {
        // 通常実行の場合
        execCmd = exec.CommandContext(ctx, path, cmd.Args...)
    }

    // ... 残りの実行処理 ...
}

func (e *DefaultExecutor) createPrivilegedCommand(ctx context.Context, cmd runnertypes.Command) *exec.Cmd {
    // sudoを使用した特権実行
    args := append([]string{cmd.Cmd}, cmd.Args...)
    return exec.CommandContext(ctx, "sudo", args...)
}
```

## 5. 実装チェックリスト

### 5.1 Phase 1チェックリスト
- [ ] validateDeprecatedFields関数の実装
- [ ] LoadConfig関数の更新
- [ ] ログ出力の実装
- [ ] sample/config.tomlの警告コメント追加
- [ ] README.mdの更新
- [ ] テストケースの作成
- [ ] 全テストの実行と通過確認
- [ ] 動作テストの実施

### 5.2 Phase 2チェックリスト
- [ ] Command構造体からUserフィールドを削除
- [ ] 移行処理の実装
- [ ] legacyCommand構造体の作成
- [ ] 設定ロード処理の更新
- [ ] sample/config.tomlの更新
- [ ] 移行ガイドの作成
- [ ] 新しいテストケースの作成
- [ ] 全テストの実行と通過確認
- [ ] 移行テストの実施
- [ ] ドキュメントの更新

### 5.3 品質管理チェックリスト
- [ ] コードレビューの実施
- [ ] 静的解析の実行
- [ ] カバレッジテストの実行
- [ ] 性能テストの実行
- [ ] セキュリティチェックの実行
- [ ] ドキュメントの校正

## 6. リスク対策

### 6.1 実装リスク
| リスク | 対策 |
|--------|------|
| 既存機能の破綻 | 段階的実装とテスト |
| 移行処理の失敗 | 十分なテストケース作成 |
| 設定ファイルの不整合 | バリデーション強化 |

### 6.2 運用リスク
| リスク | 対策 |
|--------|------|
| ユーザーの混乱 | 明確な移行ガイド |
| 設定の誤解 | 詳細なドキュメント |
| 後方互換性の問題 | 段階的移行 |

## 7. 成功指標

### 7.1 Phase 1成功指標
- [ ] 全テストケースが通過する
- [ ] 警告メッセージが適切に表示される
- [ ] 既存機能が正常に動作する
- [ ] ドキュメントが更新されている

### 7.2 Phase 2成功指標
- [ ] 全テストケースが通過する
- [ ] 移行処理が正常に動作する
- [ ] 設定ファイルが更新されている
- [ ] 移行ガイドが完成している

## 8. 実装スケジュール

### 8.1 Phase 1（1-2日）
- Day 1: 警告機能の実装
- Day 2: テストとドキュメント更新

### 8.2 Phase 2（3-5日）
- Day 1: 構造体の変更
- Day 2: 移行処理の実装
- Day 3: 設定ファイルの更新
- Day 4: テストケースの作成
- Day 5: 移行ガイドの作成

### 8.3 品質管理（1-2日）
- コードレビュー
- 包括的テスト
- ドキュメント校正

## 9. 承認プロセス

### 9.1 Phase 1承認
- [ ] 実装コードのレビュー
- [ ] テスト結果の確認
- [ ] ドキュメントの確認
- [ ] 承認者の承認

### 9.2 Phase 2承認
- [ ] 実装コードのレビュー
- [ ] 移行テストの結果確認
- [ ] 移行ガイドの確認
- [ ] 承認者の承認

## 10. 実装開始

実装は以下の順序で開始する：

1. **Phase 1の実装**: 警告機能の追加
2. **Phase 1のテスト**: 全機能の動作確認
3. **Phase 2の実装**: userフィールドの削除
4. **Phase 2のテスト**: 移行機能の動作確認
5. **ドキュメント最終化**: 全ドキュメントの更新
6. **最終テスト**: 包括的な動作確認

承認者: [プロジェクト責任者]
実装開始日: [実装開始日]
