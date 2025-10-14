# 詳細設計書: 内部変数とプロセス環境変数の分離

## 1. データ構造設計

### 1.1 GlobalConfig の拡張

**変更前（Task 0031 の状態）**:
```go
type GlobalConfig struct {
    Timeout             int
    WorkDir             string
    LogLevel            string
    VerifyFiles         []string
    SkipStandardPaths   bool
    EnvAllowlist        []string
    MaxOutputSize       int64
    Env                 []string          // Task 0031 で追加
    ExpandedVerifyFiles []string          `toml:"-"`
    ExpandedEnv         map[string]string `toml:"-"` // Task 0031 で追加
}
```

**変更後（本タスク）**:
```go
type GlobalConfig struct {
    Timeout             int
    WorkDir             string
    LogLevel            string
    VerifyFiles         []string
    SkipStandardPaths   bool
    EnvAllowlist        []string
    MaxOutputSize       int64
    FromEnv             []string `toml:"from_env"` // 新規追加（KEY=VALUE 形式）
    Vars                []string `toml:"vars"`      // 新規追加（KEY=VALUE 形式）
    Env                 []string `toml:"env"`       // 既存（Task 0031、KEY=VALUE 形式）

    InternalVars        map[string]string `toml:"-"` // 新規追加（展開済み内部変数）
    ExpandedVerifyFiles []string          `toml:"-"`
    ExpandedEnv         map[string]string `toml:"-"` // 既存（Task 0031）
}
```

**フィールドの説明**:
- `FromEnv []string`: システム環境変数を内部変数にマッピング（`内部変数名=システム環境変数名` 形式）
- `Vars []string`: 内部変数の定義（`変数名=値` 形式、他の内部変数を参照可能）
- `Env []string`: 子プロセス環境変数の定義（`変数名=値` 形式、内部変数を参照可能）
- `InternalVars map[string]string`: FromEnv + Vars の展開結果
- `ExpandedEnv map[string]string`: 展開済み環境変数（子プロセス起動時に使用）

### 1.2 CommandGroup の拡張

**変更前（Task 0031 の状態）**:
```go
type CommandGroup struct {
    Name                string
    Description         string
    Priority            int
    TempDir             bool
    WorkDir             string
    Commands            []Command
    VerifyFiles         []string
    EnvAllowlist        []string
    Env                 []string          `toml:"env"` // Task 0031 で追加
    ExpandedVerifyFiles []string          `toml:"-"`
    ExpandedEnv         map[string]string `toml:"-"` // Task 0031 で追加
}
```

**変更後（本タスク）**:
```go
type CommandGroup struct {
    Name                string
    Description         string
    Priority            int
    TempDir             bool
    WorkDir             string
    Commands            []Command
    VerifyFiles         []string
    EnvAllowlist        []string
    FromEnv             []string `toml:"from_env"` // 新規追加（KEY=VALUE 形式）
    Vars                []string `toml:"vars"`      // 新規追加（KEY=VALUE 形式）
    Env                 []string `toml:"env"`       // 既存（Task 0031、KEY=VALUE 形式）

    InternalVars        map[string]string `toml:"-"` // 新規追加（展開済み内部変数）
    ExpandedVerifyFiles []string          `toml:"-"`
    ExpandedEnv         map[string]string `toml:"-"` // 既存（Task 0031）
}
```

### 1.3 Command 構造体（変更なし）

既存の `Command` 構造体は変更不要：
- `Env []string` は既存
- `ExpandedEnv map[string]string` は既存
- `ExpandedCmd string` は既存
- `ExpandedArgs []string` は既存

## 2. 処理フロー詳細

### 2.1 設定読み込み時の処理フロー

```
1. TOML ファイルをパース
   ↓
2. Global の処理
   a. Global.from_env の処理
      - システム環境変数を Global.InternalVars にマッピング
      - env_allowlist チェック
   b. Global.vars の展開
      - Global.InternalVars（from_env の結果）を参照可能（%{VAR} のみ）
      - 結果を Global.InternalVars にマージ
      - 循環参照検出
   c. Global.env の展開
      - Global.InternalVars（%{VAR} のみ）を参照可能
      - ${VAR} 構文は使用不可
      - 結果を Global.ExpandedEnv に保存
      - env_allowlist チェック不要（内部変数のみ参照）
   d. Global.verify_files の展開
      - Global.InternalVars（%{VAR} のみ）を参照可能
      - 結果を Global.ExpandedVerifyFiles に保存
   ↓
3. 各 Group の処理（全グループに対して順次実行）
   a. Group.from_env の処理と継承判定
      - Group.from_env が定義されている場合:
        * システム環境変数を Group.InternalVars にマッピング
        * Global.from_env は継承しない（上書き）
        * グループの有効な env_allowlist でチェック
      - Group.from_env が未定義の場合:
        * Global.from_env の内容を Group.InternalVars にコピー（継承）
   b. Group.vars の展開
      - Group.InternalVars（from_env の結果または継承）と Global.InternalVars（vars）を参照可能
      - 結果を Group.InternalVars にマージ
      - 循環参照検出
   c. Group.env の展開
      - Group.InternalVars（%{VAR} のみ）を参照可能
      - ${VAR} 構文は使用不可
      - 結果を Group.ExpandedEnv に保存
      - env_allowlist チェック不要（内部変数のみ参照）
   d. Group.verify_files の展開
      - Group.InternalVars（%{VAR} のみ）を参照可能
      - 結果を Group.ExpandedVerifyFiles に保存
   ↓
4. 各 Command の処理（全コマンドに対して順次実行）
   a. Command.env の展開
      - 所属 Group.InternalVars（%{VAR} のみ）を参照可能
        * Group.from_env が定義されている場合: Group 固有の from_env のみ
        * Group.from_env が未定義の場合: Global.from_env を継承した内部変数
      - ${VAR} 構文は使用不可
      - 結果を Command.ExpandedEnv に保存
      - env_allowlist チェック不要（内部変数のみ参照）
   b. Command.cmd と Command.args の展開
      - 所属 Group.InternalVars（%{VAR} のみ）を参照可能（継承ルールに従う）
      - env で定義した変数は参照不可
      - 結果を Command.ExpandedCmd, Command.ExpandedArgs に保存
```

### 2.2 実行時の処理フロー

```
1. 子プロセス環境変数の構築
   - システム環境変数（env_allowlist でフィルタリング）
   - Global.ExpandedEnv
   - Group.ExpandedEnv
   - Command.ExpandedEnv
   - 上記を優先順位に従ってマージ
   ↓
2. 子プロセスの起動
   - Command.ExpandedCmd
   - Command.ExpandedArgs
   - 構築した環境変数
```

## 3. アルゴリズム設計

### 3.1 循環参照検出アルゴリズム

**重要**: 処理順序（from_env → vars → env）と階層構造（Global → Group → Command）により、各段階で参照するのは既に確定した値です。そのため、自己参照のための特別な処理は不要です。

**循環参照検出が必要なケース**:
- 同一レベル内での変数間の循環参照（例: vars 内で A → B → A）
- 同一変数名での完全な自己参照（例: `A=%{A}`）

**循環参照検出が不要なケース**:
- from_env と vars の間の参照（処理順序により既に確定）
- Global.vars と Group.vars の間の参照（階層構造により一方向のみ）
- 異なる名前の変数による値の拡張（例: `from_env = ["path=PATH"]` → `vars = ["path=/custom:%{path}"]`）

```go
// 同一レベル内での循環参照のみを検出
func detectCircularReference(vars map[string]string, visited map[string]bool,
                           current string, path []string) error {
    if visited[current] {
        return fmt.Errorf("circular reference detected: %s -> %s",
                         strings.Join(path, " -> "), current)
    }

    visited[current] = true

    // 変数値内の %{VAR} 参照を解析
    refs := extractVariableReferences(vars[current])
    for _, ref := range refs {
        // 同一マップ内の変数のみチェック
        if _, exists := vars[ref]; exists {
            if err := detectCircularReference(vars, visited, ref,
                                            append(path, current)); err != nil {
                return err
            }
        }
    }

    delete(visited, current)
    return nil
}
```

### 3.2 変数展開アルゴリズム

**処理方針**:
1. 処理順序に従って段階的に展開（from_env → vars → env）
2. 各段階で参照できるのは既に確定した変数のみ
3. 循環参照チェックは同一マップ内のみ

```go
func expandVariable(value string, availableVars map[string]string,
                   visited map[string]bool, maxDepth int) (string, error) {
    if maxDepth <= 0 {
        return "", fmt.Errorf("maximum expansion depth exceeded")
    }

    result := value
    iterations := 0
    maxIterations := 15 // 無限ループ防止

    for iterations < maxIterations {
        refs := extractVariableReferences(result)
        if len(refs) == 0 {
            break // 展開完了
        }

        changed := false
        for _, ref := range refs {
            // 循環参照チェック（visited マップ）
            if visited[ref] {
                return "", fmt.Errorf("circular reference: %s", ref)
            }

            // 利用可能な変数から値を取得
            if varValue, exists := availableVars[ref]; exists {
                visited[ref] = true
                expanded, err := expandVariable(varValue, availableVars, visited, maxDepth-1)
                delete(visited, ref)

                if err != nil {
                    return "", err
                }

                result = strings.ReplaceAll(result, "%{"+ref+"}", expanded)
                changed = true
            } else {
                return "", fmt.Errorf("undefined variable: %s", ref)
            }
        }

        if !changed {
            break
        }
        iterations++
    }

    if iterations >= maxIterations {
        return "", fmt.Errorf("maximum iterations exceeded (possible infinite loop)")
    }

    return result, nil
}
```

**availableVars の構成**:
- **Global.vars 展開時**: Global.from_env の結果のみ
- **Group.vars 展開時**: Global.from_env（または Group.from_env）+ Global.vars
- **env 展開時**: 該当レベルの InternalVars（from_env + vars の結果）

## 4. インターフェース設計

### 4.1 変数展開インターフェース

```go
type VariableExpander interface {
    ExpandString(value string, context VariableContext) (string, error)
    ExpandStringSlice(values []string, context VariableContext) ([]string, error)
    ValidateReferences(value string, context VariableContext) error
}

type VariableContext interface {
    GetVariable(name string) (string, bool)
    GetAllVariables() map[string]string
    IsSystemVariable(name string) bool
    IsAllowed(name string) bool
}
```

### 4.2 環境変数プロセッサインターフェース

```go
type EnvironmentProcessor interface {
    ProcessGlobalEnv(global *GlobalConfig) error
    ProcessGroupEnv(group *CommandGroup, global *GlobalConfig) error
    ProcessCommandEnv(cmd *Command, group *CommandGroup, global *GlobalConfig) error
    BuildEnvironment(cmd *Command, group *CommandGroup, global *GlobalConfig) (map[string]string, error)
}
```

## 5. エラー処理設計

### 5.1 エラー型定義

```go
type VariableError struct {
    Type     ErrorType
    Variable string
    Location string
    Message  string
    Cause    error
}

type ErrorType int

const (
    ErrorUndefinedVariable ErrorType = iota
    ErrorCircularReference
    ErrorAllowlistViolation
    ErrorSyntaxError
    ErrorExpansionDepthExceeded
)
```

### 5.2 エラーメッセージ設計

- **詳細な位置情報**: global/group/command レベルでの正確なエラー位置
- **修正提案**: 具体的な修正方法の提示
- **関連情報**: 循環参照の場合は参照チェーンの表示

## 6. 設定ファイル検証

### 6.1 検証ルール

1. **構文検証**: TOML 構文の正確性
2. **スキーマ検証**: フィールドの型と必須項目
3. **セキュリティ検証**: allowlist との整合性
4. **参照整合性**: 変数参照の妥当性
5. **循環参照検証**: 変数間の循環参照チェック

### 6.2 検証アルゴリズム

```go
func ValidateConfiguration(config *Config) []ValidationError {
    var errors []ValidationError

    // 1. 構文検証
    errors = append(errors, validateSyntax(config)...)

    // 2. allowlist 検証
    errors = append(errors, validateAllowlist(config)...)

    // 3. 変数参照検証
    errors = append(errors, validateVariableReferences(config)...)

    // 4. 循環参照検証
    errors = append(errors, validateCircularReferences(config)...)

    return errors
}
```

## 7. 性能最適化

### 7.1 キャッシュ戦略

- **展開結果キャッシュ**: 一度展開した変数値のキャッシュ
- **参照関係キャッシュ**: 変数間の依存関係のキャッシュ
- **環境変数マップキャッシュ**: 構築済み環境変数の再利用

### 7.2 遅延評価

- **段階的展開**: 必要になるまで変数展開を遅延
- **オンデマンド構築**: 実行時の環境変数構築

## 8. エスケープシーケンス処理の変更

### 8.1 変更の必要性

**`${VAR}` 構文の廃止に伴う変更**:
- 本タスクで `${VAR}` 構文が完全に廃止される
- `$` 記号は特殊文字ではなくなる
- `\$` エスケープシーケンスは不要になる

### 8.2 変更対象

**`internal/runner/environment/processor.go`**:
```go
// 変更前（Task 0026 の実装）
func (p *VariableExpander) handleEscapeSequence(...) (int, error) {
    // ...
    nextChar := inputChars[i+1]
    if nextChar == '$' || nextChar == '\\' {  // $ と \ をエスケープ
        result.WriteRune(nextChar)
        return i + 2, nil
    }
    // ...
}

// 変更後（本タスク）
func (p *VariableExpander) handleEscapeSequence(...) (int, error) {
    // ...
    nextChar := inputChars[i+1]
    if nextChar == '%' || nextChar == '\\' {  // % と \ をエスケープ
        result.WriteRune(nextChar)
        return i + 2, nil
    }
    // ...
}
```

### 8.3 テストの更新

**変更が必要なテストケース**:
- `internal/runner/config/expansion_test.go`: エスケープシーケンスのテスト
- `internal/runner/environment/processor_test.go`: エスケープ処理のテスト

**更新例**:
```go
// 変更前
{
    name: "escape sequence handling",
    args: []string{"\\${HOME}", "${MESSAGE}"},
    expectedArgs: []string{"${HOME}", "hello"},
}

// 変更後
{
    name: "escape sequence handling",
    args: []string{"\\%{home}", "%{message}"},
    expectedArgs: []string{"%{home}", "hello"},
}
```

## 9. テスト設計

### 9.1 単体テスト

- **変数展開**: 各種パターンでの正常な展開
- **循環参照**: 循環参照の正確な検出
- **エラーハンドリング**: 各種エラー条件の適切な処理
- **エスケープシーケンス**: `\%` と `\\` のエスケープ処理

### 9.2 統合テスト

- **設定読み込み**: 複雑な設定ファイルの正常な処理
- **実行フロー**: end-to-end での動作確認
- **セキュリティ**: allowlist 機能の確実な動作
- **後方互換性**: `${VAR}` 構文の検出とエラー処理

### 9.3 性能テスト

- **スケーラビリティ**: 大量の変数での性能測定
- **メモリ使用量**: メモリ効率の測定
- **レスポンス性能**: 設定読み込み時間の測定
