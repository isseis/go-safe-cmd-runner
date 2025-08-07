# アーキテクチャ設計書: 特権実行設定の統一化

## 1. アーキテクチャ概要

### 1.1 現在のアーキテクチャ
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Config File   │───▶│  Config Loader  │───▶│   Runner Types  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    Executor     │◀───│    Template     │◀───│     Command     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### 1.2 問題点
- **冗長性**: `Command`構造体に`User`と`Privileged`フィールドが併存
- **未実装**: `User`フィールドは実行時に使用されない
- **不整合**: `Privileged`フィールドも実際の特権昇格処理なし

### 1.3 改善後のアーキテクチャ
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Config File   │───▶│  Config Loader  │───▶│   Runner Types  │
│  (privileged)   │    │  (validation)   │    │  (privileged)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    Executor     │◀───│    Template     │◀───│     Command     │
│ (privilege impl)│    │  (privileged)   │    │  (privileged)   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 2. コンポーネント設計

### 2.1 設定構造体の変更

#### 現在の`Command`構造体
```go
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

#### 変更後の`Command`構造体
```go
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

### 2.2 設定ローダーの拡張

#### Phase 1: 警告機能の追加
```go
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
    // ... 既存のロード処理 ...

    // 未実装フィールドの警告
    if err := l.validateDeprecatedFields(&cfg); err != nil {
        log.Printf("Warning: %v", err)
    }

    return &cfg, nil
}

func (l *Loader) validateDeprecatedFields(cfg *runnertypes.Config) error {
    for _, group := range cfg.Groups {
        for _, cmd := range group.Commands {
            if cmd.User != "" {
                return fmt.Errorf("user field is deprecated and not implemented")
            }
            if cmd.Privileged {
                return fmt.Errorf("privileged field is not yet implemented")
            }
        }
    }
    return nil
}
```

#### Phase 2: 後方互換性の処理
```go
func (l *Loader) migrateDeprecatedFields(cfg *runnertypes.Config) error {
    for i, group := range cfg.Groups {
        for j, cmd := range group.Commands {
            // user="root" を privileged=true に変換
            if cmd.User == "root" {
                cfg.Groups[i].Commands[j].Privileged = true
                log.Printf("Migrated user='root' to privileged=true for command %s", cmd.Name)
            } else if cmd.User != "" {
                log.Printf("Warning: user field '%s' is ignored (only root was supported)", cmd.User)
            }
        }
    }
    return nil
}
```

### 2.3 実行エンジンの設計

#### Phase 3: 特権実行の実装（将来）
```go
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

## 3. データフロー

### 3.1 現在のフロー
```
Config File → Loader → Command{User, Privileged} → Executor (User無視)
```

### 3.2 Phase 1のフロー
```
Config File → Loader → Validation → Command{User, Privileged} → Executor
                           ↓
                    Warning Messages
```

### 3.3 Phase 2のフロー
```
Config File → Loader → Migration → Command{Privileged} → Executor
                           ↓
                    user="root" → privileged=true
```

### 3.4 Phase 3のフロー（将来）
```
Config File → Loader → Command{Privileged} → Executor → sudo/normal execution
```

## 4. セキュリティ考慮事項

### 4.1 現在の状態
- 特権実行機能は実装されていないため、セキュリティリスクは低い
- 設定ファイルの権限制御は既存の仕組みで保護されている

### 4.2 Phase 1-2の考慮事項
- 設定ファイルの後方互換性処理においてセキュリティ情報の漏洩防止
- 適切な警告メッセージの表示

### 4.3 Phase 3の考慮事項（将来）
- `sudo`実行時の適切な権限チェック
- 特権実行の監査ログ
- セキュリティポリシーの適用

## 5. エラーハンドリング

### 5.1 Phase 1: 警告メッセージ
```go
type DeprecationWarning struct {
    Field   string
    Command string
    Message string
}

func (w DeprecationWarning) Error() string {
    return fmt.Sprintf("deprecated field '%s' in command '%s': %s", w.Field, w.Command, w.Message)
}
```

### 5.2 Phase 2: 移行エラー
```go
type MigrationError struct {
    Command string
    Reason  string
}

func (e MigrationError) Error() string {
    return fmt.Sprintf("migration failed for command '%s': %s", e.Command, e.Reason)
}
```

### 5.3 Phase 3: 特権実行エラー（将来）
```go
type PrivilegeError struct {
    Command string
    Reason  string
}

func (e PrivilegeError) Error() string {
    return fmt.Sprintf("privilege execution failed for command '%s': %s", e.Command, e.Reason)
}
```

## 6. テスト戦略

### 6.1 Phase 1: 警告機能のテスト
- 非推奨フィールド使用時の警告メッセージ確認
- 既存機能の正常動作確認

### 6.2 Phase 2: 移行機能のテスト
- `user="root"` → `privileged=true`の変換テスト
- 後方互換性のテスト
- エラーハンドリングのテスト

### 6.3 Phase 3: 特権実行のテスト（将来）
- 特権実行の動作確認
- セキュリティテスト
- パフォーマンステスト

## 7. 依存関係

### 7.1 内部依存
- `internal/runner/runnertypes`: 構造体定義の変更
- `internal/runner/config`: 設定ローダーの拡張
- `internal/runner/executor`: 実行エンジンの将来的な拡張

### 7.2 外部依存
- `github.com/pelletier/go-toml/v2`: TOML解析（変更なし）
- システムの`sudo`コマンド: Phase 3での特権実行（将来）

## 8. 監視・運用

### 8.1 ログ出力
- 非推奨フィールドの使用警告
- 移行処理の実行ログ
- 特権実行の監査ログ（将来）

### 8.2 メトリクス
- 非推奨フィールドの使用頻度
- 移行成功/失敗の統計
- 特権実行の頻度と成功率（将来）

## 9. 移行計画

### 9.1 Phase 1: 即座実行可能
- 既存機能への影響なし
- 警告機能の追加のみ

### 9.2 Phase 2: 次期バージョン
- 構造体の変更を伴う
- 設定ファイルの更新が必要

### 9.3 Phase 3: 将来バージョン
- 新機能の実装
- セキュリティテストが必要
