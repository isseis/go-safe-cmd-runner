# 詳細設計書: グループレベル環境変数設定削除

## 1. 概要

### 1.1 目的
本文書は、go-safe-cmd-runnerプロジェクトにおけるグループレベル環境変数設定（`CommandGroup.Env`）の完全削除に関する詳細な実装設計を記述する。アーキテクチャ設計書で定義された変更内容を、具体的なコード変更として詳細化する。

### 1.2 スコープ
- データ構造の変更詳細
- 処理関数の変更仕様
- エラーハンドリングの変更
- テストケースの変更方針
- サンプル設定ファイルの更新

## 2. データ構造の詳細変更

### 2.1 CommandGroup構造体の変更

#### 2.1.1 変更対象ファイル
- `internal/runner/runnertypes/config.go`

#### 2.1.2 変更内容
```go
// 変更前
type CommandGroup struct {
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    DependsOn    []string  `toml:"depends_on"`
    Template     string    `toml:"template"`
    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    Env          []string  `toml:"env"`           // 削除対象
    EnvAllowlist []string  `toml:"env_allowlist"` // 維持
}

// 変更後
type CommandGroup struct {
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    DependsOn    []string  `toml:"depends_on"`
    Template     string    `toml:"template"`
    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    // Envフィールドを完全削除
    EnvAllowlist []string  `toml:"env_allowlist"` // 維持
}
```

#### 2.1.3 影響範囲
- TOMLパーサーは自動的にenvフィールドを無視する
- 既存の設定ファイルでenvフィールドを使用している場合、警告なしに無視される
- EnvAllowlistフィールドは影響を受けない

## 3. 環境変数処理関数の詳細変更

### 3.1 ResolveGroupEnvironmentVars関数の変更

#### 3.1.1 変更対象ファイル
- `internal/runner/environment/filter.go`

#### 3.1.2 現在の実装（変更前）
```go
// ResolveGroupEnvironmentVars resolves environment variables for a specific group
func (f *Filter) ResolveGroupEnvironmentVars(group *runnertypes.CommandGroup, loadedEnvVars map[string]string) (map[string]string, error) {
    if group == nil {
        return nil, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
    }

    // Filter system environment variables
    filteredSystemEnv, err := f.FilterSystemEnvironment(group.EnvAllowlist)
    if err != nil {
        return nil, fmt.Errorf("failed to filter system environment: %w", err)
    }

    // Start with filtered system environment variables
    result := make(map[string]string)
    maps.Copy(result, filteredSystemEnv)

    // Add loaded environment variables from .env file (already filtered in LoadEnvironment)
    // These override system variables
    for variable, value := range loadedEnvVars {
        if f.isVariableAllowed(variable, group.EnvAllowlist) {
            result[variable] = value
        }
    }

    // Add group-level environment variables (these override both system and .env vars)
    for _, env := range group.Env {  // この処理ブロックを削除
        parts := strings.SplitN(env, "=", envSeparatorParts)
        if len(parts) != envSeparatorParts {
            continue
        }

        variable, value := parts[0], parts[1]

        // Validate environment variable name and value
        if err := f.ValidateEnvironmentVariable(variable, value); err != nil {
            slog.Warn("Group environment variable validation failed",
                "variable", variable,
                "group", group.Name,
                "error", err)
            continue
        }

        // Check if variable is allowed
        if f.isVariableAllowed(variable, group.EnvAllowlist) {
            result[variable] = value
        } else {
            slog.Warn("Group environment variable rejected by allowlist",
                "variable", variable,
                "group", group.Name)
        }
    }

    return result, nil
}
```

#### 3.1.3 変更後の実装
```go
// ResolveGroupEnvironmentVars resolves environment variables for a specific group
func (f *Filter) ResolveGroupEnvironmentVars(group *runnertypes.CommandGroup, loadedEnvVars map[string]string) (map[string]string, error) {
    if group == nil {
        return nil, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
    }

    // Filter system environment variables
    filteredSystemEnv, err := f.FilterSystemEnvironment(group.EnvAllowlist)
    if err != nil {
        return nil, fmt.Errorf("failed to filter system environment: %w", err)
    }

    // Start with filtered system environment variables
    result := make(map[string]string)
    maps.Copy(result, filteredSystemEnv)

    // Add loaded environment variables from .env file (already filtered in LoadEnvironment)
    // These override system variables
    for variable, value := range loadedEnvVars {
        if f.isVariableAllowed(variable, group.EnvAllowlist) {
            result[variable] = value
        }
    }

    // グループレベル環境変数処理を削除
    // 以前のgroup.Env処理ブロックを完全削除

    return result, nil
}
```

#### 3.1.4 変更の詳細
- **削除対象**: `group.Env`をイテレートするforループ全体
- **削除される処理**:
  - グループ環境変数のパース（`strings.SplitN`）
  - 環境変数の検証（`ValidateEnvironmentVariable`）
  - allowlist チェック（`isVariableAllowed`）
  - エラー・警告ログ出力
- **維持される処理**:
  - システム環境変数のフィルタリング
  - .envファイル環境変数の適用
  - EnvAllowlist によるアクセス制御

### 3.2 resolveEnvironmentVars関数への影響

#### 3.2.1 変更対象ファイル
- `internal/runner/runner.go`

#### 3.2.2 現在の実装（変更前）
```go
// resolveEnvironmentVars resolves environment variables for a command with group context
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command, group *runnertypes.CommandGroup) (map[string]string, error) {
    var envVars map[string]string
    var err error

    // Use the filter to resolve group environment variables with allowlist filtering
    if group != nil {
        envVars, err = r.envFilter.ResolveGroupEnvironmentVars(group, r.envVars)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve group environment variables: %w", err)
        }
    } else {
        // For commands without group context, use global allowlist and filter system environment variables
        // FilterSystemEnvironment will create and return a new map
        envVars, err = r.envFilter.FilterSystemEnvironment(nil)
        if err != nil {
            return nil, fmt.Errorf("failed to filter system environment variables: %w", err)
        }

        // Add loaded environment variables from .env file (already filtered in LoadEnvironment)
        maps.Copy(envVars, r.envVars)
    }

    // Add command-specific environment variables
    for _, env := range cmd.Env {
        parts := strings.SplitN(env, "=", envSeparatorParts)
        if len(parts) != envSeparatorParts {
            continue
        }

        variable, value := parts[0], parts[1]
        allowed := r.envFilter.IsVariableAccessAllowed(variable, group)
        if !allowed {
            logDeniedEnvironmentVariableAccess(group, variable, cmd)
            continue
        }
        // Resolve variable references in the value
        resolvedValue, err := r.resolveVariableReferences(value, envVars, group)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve variable %s: %w", variable, err)
        }
        envVars[variable] = resolvedValue
    }
    return envVars, nil
}
```

#### 3.2.3 変更後の実装
```go
// resolveEnvironmentVars resolves environment variables for a command with group context
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command, group *runnertypes.CommandGroup) (map[string]string, error) {
    var envVars map[string]string
    var err error

    // Use the filter to resolve group environment variables with allowlist filtering
    // この関数の動作は変更されないが、内部でgroup.Env処理が削除される
    if group != nil {
        envVars, err = r.envFilter.ResolveGroupEnvironmentVars(group, r.envVars)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve group environment variables: %w", err)
        }
    } else {
        // For commands without group context, use global allowlist and filter system environment variables
        // FilterSystemEnvironment will create and return a new map
        envVars, err = r.envFilter.FilterSystemEnvironment(nil)
        if err != nil {
            return nil, fmt.Errorf("failed to filter system environment variables: %w", err)
        }

        // Add loaded environment variables from .env file (already filtered in LoadEnvironment)
        maps.Copy(envVars, r.envVars)
    }

    // Add command-specific environment variables
    // この処理は変更なし（コマンドレベル環境変数は維持）
    for _, env := range cmd.Env {
        parts := strings.SplitN(env, "=", envSeparatorParts)
        if len(parts) != envSeparatorParts {
            continue
        }

        variable, value := parts[0], parts[1]
        allowed := r.envFilter.IsVariableAccessAllowed(variable, group)
        if !allowed {
            logDeniedEnvironmentVariableAccess(group, variable, cmd)
            continue
        }
        // Resolve variable references in the value
        resolvedValue, err := r.resolveVariableReferences(value, envVars, group)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve variable %s: %w", variable, err)
        }
        envVars[variable] = resolvedValue
    }
    return envVars, nil
}
```

#### 3.2.4 変更の詳細
- **関数シグネチャ**: 変更なし
- **処理ロジック**: `ResolveGroupEnvironmentVars`の内部変更により間接的に影響
- **直接的な変更**: なし（この関数自体のロジックは変更されない）

## 4. 変数参照解決機能の確認

### 4.1 維持される機能
- `resolveVariableReferences`関数は変更されない
- `${VAR}`形式の変数参照解決機能は完全に維持
- EnvAllowlistによる変数アクセス制御も維持

### 4.2 変数参照元の変更
```go
// 変更前: 4つのソースから変数参照可能
// 1. システム環境変数
// 2. .envファイル変数
// 3. グループ環境変数  ← 削除対象
// 4. コマンド環境変数

// 変更後: 3つのソースから変数参照可能
// 1. システム環境変数
// 2. .envファイル変数
// 3. コマンド環境変数
```

## 5. エラーハンドリングの詳細変更

### 5.1 削除されるエラーパターン

#### 5.1.1 グループ環境変数構文エラー
```go
// 削除される処理
if len(parts) != envSeparatorParts {
    continue  // グループ環境変数の構文エラー処理
}
```

#### 5.1.2 グループ環境変数検証エラー
```go
// 削除される処理
if err := f.ValidateEnvironmentVariable(variable, value); err != nil {
    slog.Warn("Group environment variable validation failed",
        "variable", variable,
        "group", group.Name,
        "error", err)
    continue
}
```

#### 5.1.3 グループ環境変数Allowlist違反
```go
// 削除される処理
if f.isVariableAllowed(variable, group.EnvAllowlist) {
    result[variable] = value
} else {
    slog.Warn("Group environment variable rejected by allowlist",
        "variable", variable,
        "group", group.Name)
}
```

### 5.2 維持されるエラーハンドリング

#### 5.2.1 コマンド環境変数関連
- コマンドレベル環境変数のAllowlist違反
- コマンドレベル環境変数の変数参照エラー
- コマンドレベル環境変数の検証エラー

#### 5.2.2 システム・.envファイル関連
- システム環境変数フィルタリングエラー
- .envファイル読み込みエラー
- 変数参照解決エラー

## 6. テストケースの変更方針

### 6.1 削除対象テストケース

#### 6.1.1 グループ環境変数処理テスト
```go
// 削除対象テストケースの例
func TestResolveGroupEnvironmentVars_GroupEnvProcessing(t *testing.T) {
    group := &runnertypes.CommandGroup{
        Name: "test_group",
        Env: []string{"GROUP_VAR=group_value"},  // 削除されたフィールド
        EnvAllowlist: []string{"GROUP_VAR"},
    }
    // このテストケースは削除
}
```

#### 6.1.2 グループ環境変数優先順位テスト
```go
// 削除対象テストケースの例
func TestEnvironmentVariablePriority_WithGroupEnv(t *testing.T) {
    // グループ環境変数の優先順位テスト
    // このテストケースは削除
}
```

### 6.2 更新が必要なテストケース

#### 6.2.1 統合テストケース
```go
// 変更前のテストケース
func TestRunner_resolveEnvironmentVars(t *testing.T) {
    config := &runnertypes.Config{
        Groups: []runnertypes.CommandGroup{{
            Name: "test_group",
            Env: []string{"GROUP_VAR=value"},  // この設定を削除
            EnvAllowlist: []string{"SYSTEM_VAR", "GROUP_VAR", "CMD_VAR"},
        }},
    }

    // テストロジックの更新が必要
}

// 変更後のテストケース
func TestRunner_resolveEnvironmentVars(t *testing.T) {
    config := &runnertypes.Config{
        Groups: []runnertypes.CommandGroup{{
            Name: "test_group",
            // Envフィールドを削除
            EnvAllowlist: []string{"SYSTEM_VAR", "CMD_VAR"},  // GROUP_VARを削除
        }},
    }

    // 期待結果の更新が必要
}
```

### 6.3 新規追加テストケース

#### 6.3.1 簡素化された処理フローテスト
```go
func TestResolveGroupEnvironmentVars_SimplifiedFlow(t *testing.T) {
    tests := []struct {
        name           string
        systemEnv      map[string]string
        envFileVars    map[string]string
        groupAllowlist []string
        expected       map[string]string
    }{
        {
            name: "system and env file variables only",
            systemEnv: map[string]string{
                "SYSTEM_VAR": "system_value",
                "OTHER_VAR":  "other_value",
            },
            envFileVars: map[string]string{
                "ENV_FILE_VAR": "env_file_value",
            },
            groupAllowlist: []string{"SYSTEM_VAR", "ENV_FILE_VAR"},
            expected: map[string]string{
                "SYSTEM_VAR":   "system_value",
                "ENV_FILE_VAR": "env_file_value",
            },
        },
    }

    // テスト実装
}
```

## 7. ログ出力の変更

### 7.1 削除されるログメッセージ

#### 7.1.1 デバッグレベルログ
```go
// 削除されるログ
slog.Debug("Processing group environment variable",
    "variable", variable,
    "value", value,
    "group", group.Name)
```

#### 7.1.2 警告レベルログ
```go
// 削除される警告ログ
slog.Warn("Group environment variable validation failed", ...)
slog.Warn("Group environment variable rejected by allowlist", ...)
```

### 7.2 維持されるログメッセージ
- コマンド環境変数処理関連のログ
- システム環境変数フィルタリング関連のログ
- 変数参照解決関連のログ
- EnvAllowlistアクセス制御関連のログ

## 8. サンプル設定ファイルの更新

### 8.1 更新対象ファイル
- `sample/config.toml`
- `sample/test.toml`
- ドキュメント内のサンプル設定

### 8.2 変更例

#### 8.2.1 sample/config.toml の更新
```toml
# 変更前（現在グループ環境変数が使用されていない場合は変更不要）
[[groups]]
name = "example"
description = "Example group"
# 現在、グループレベルenvは使用されていないため、変更不要

# 万が一使用されている場合の変更例
# 変更前
[[groups]]
name = "build"
env = ["NODE_ENV=production"]  # 削除対象

[[groups.commands]]
name = "build_app"
env = ["BUILD_TARGET=web"]

# 変更後
[[groups]]
name = "build"
# グループレベルenvを削除

[[groups.commands]]
name = "build_app"
env = ["NODE_ENV=production", "BUILD_TARGET=web"]  # 統合
```

## 9. 移行ガイダンス

### 9.1 既存設定ファイルの識別
```bash
# グループ環境変数を使用している設定ファイルを検索
grep -n "env\s*=" sample/*.toml | grep -v "commands"
```

### 9.2 移行手順

#### 9.2.1 手動移行プロセス
1. グループレベル`env`設定を特定
2. 該当グループ内の全コマンドを確認
3. グループ`env`設定をコマンドレベル`env`に統合
4. 重複環境変数の整理
5. EnvAllowlist設定の確認・更新

#### 9.2.2 移行チェックリスト
```
□ グループレベルenv設定の削除
□ コマンドレベルへの環境変数統合
□ 重複環境変数の整理
□ EnvAllowlist設定の更新
□ 動作テストの実行
```

## 10. パフォーマンス測定方針

### 10.1 測定対象
- 環境変数解決処理時間
- メモリ使用量（設定ファイル読み込み後）
- 設定ファイルパース時間

### 10.2 測定方法
```go
func BenchmarkResolveEnvironmentVars(b *testing.B) {
    // 変更前後の処理時間を比較
    // グループ環境変数数: 0, 10, 50, 100
    // コマンド数: 1, 10, 50
}
```

## 11. 実装完了基準

### 11.1 コード変更完了基準
- [ ] `CommandGroup`構造体から`Env`フィールドの削除
- [ ] `ResolveGroupEnvironmentVars`関数からグループ環境変数処理の削除
- [ ] 関連テストケースの削除・更新
- [ ] サンプル設定ファイルの更新

### 11.2 品質基準
- [ ] 全ユニットテストのパス
- [ ] 統合テストのパス
- [ ] ベンチマークテストによる性能確認
- [ ] 既存機能（EnvAllowlist等）の動作確認

### 11.3 ドキュメント更新基準
- [ ] アーキテクチャ図の更新
- [ ] 設定ファイル仕様書の更新
- [ ] サンプル設定の更新
- [ ] 移行ガイドの作成

## 12. リスク軽減策

### 12.1 実装リスク対策
- **段階的実装**: データ構造→処理ロジック→テストの順で実装
- **既存機能保護**: EnvAllowlist等の既存機能は変更しない
- **十分なテスト**: 変更前後の動作比較テストを実施

### 12.2 運用リスク対策
- **移行支援**: 詳細な移行ガイドの提供
- **検証ツール**: 設定ファイルの構文チェック機能の活用
- **段階的展開**: 開発環境での十分な検証後の本格展開

## 13. 承認

本詳細設計書は、アーキテクチャ設計書に基づいて作成されており、グループレベル環境変数設定削除の具体的な実装方法を定義している。

技術レビュー: [要レビュー]
設計承認者: [プロジェクト責任者]
承認日: [承認日]
