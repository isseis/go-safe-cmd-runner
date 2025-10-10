# 詳細仕様書: Global・Groupレベル環境変数設定機能

## 1. 仕様概要

### 1.1 目的
Global・Groupレベルで環境変数を定義し、階層的なスコープとオーバーライド機構を提供する機能の詳細仕様を定義する。

### 1.2 適用範囲
本仕様書は以下のコンポーネントの実装に適用される:
- データ構造の拡張（GlobalConfig, CommandGroup）
- 環境変数展開処理（expansion.go）
- 既存の`VariableExpander`の活用

## 2. データ構造の詳細仕様

### 2.1 GlobalConfig構造体の拡張

#### 2.1.1 フィールド定義

**新規追加フィールド**:

| フィールド名 | 型 | TOMLタグ | 説明 | 必須 |
|------------|-----|---------|------|------|
| Env | []string | `toml:"env"` | Global環境変数定義（KEY=VALUE形式） | いいえ |
| ExpandedEnv | map[string]string | `toml:"-"` | 展開済みGlobal環境変数マップ | - |

#### 2.1.2 バリデーションルール

1. **KEY形式**:
   - 正規表現: `^[A-Za-z_][A-Za-z0-9_]*$`
   - 予約プレフィックス: `__RUNNER_` で始まる名前は禁止

2. **VALUE形式**:
   - 任意の文字列
   - `${VAR}`形式の変数参照をサポート
   - エスケープシーケンス: `\$` → `$`, `\\` → `\`（既存実装と同じ）

3. **KEY=VALUE形式のパース**:
   - 最初の`=`でKEYとVALUEに分割
   - `=`が存在しない場合はエラー

### 2.2 CommandGroup構造体の拡張

#### 2.2.1 フィールド定義

**新規追加フィールド**:

| フィールド名 | 型 | TOMLタグ | 説明 | 必須 |
|------------|-----|---------|------|------|
| Env | []string | `toml:"env"` | Group環境変数定義（KEY=VALUE形式） | いいえ |
| ExpandedEnv | map[string]string | `toml:"-"` | 展開済みGroup環境変数マップ | - |

#### 2.2.2 バリデーションルール

GlobalConfigと同じバリデーションルールを適用。

### 2.3 Command構造体

**変更なし**: 既存の`Env`と`ExpandedEnv`フィールドをそのまま使用。

## 3. 環境変数展開処理の詳細仕様

### 3.1 既存実装の活用

#### 3.1.1 VariableExpander（Task 0026で実装済み）

**既存のメソッド**:

| メソッド | シグネチャ | 本タスクでの活用 |
|---------|---------|-----------------|
| `ExpandString(value, envVars, allowlist, groupName, visited)` | 文字列内の変数を`envVars`マップから展開 | Global/Group.Envの展開、VerifyFilesの展開 |
| `ExpandStrings(texts, envVars, allowlist, groupName)` | 文字列配列の各要素を展開 | VerifyFilesの展開 |
| `ExpandCommandEnv(cmd, groupName, allowlist, baseEnv)` | Command.Envを展開（内部でbaseEnvとマージ） | 既存機能（Global+Group.ExpandedEnvをbaseEnvとして渡す） |
| `resolveVariable()` | 変数の解決（内部メソッド） | 既存機能（allowlistチェック、循環参照検出） |

**エスケープシーケンス処理**（既存実装）:
- `\$` → `$`: リテラルのドル記号
- `\\` → `\`: リテラルのバックスラッシュ
- その他の`\x`はエラー

**循環参照検出**（既存実装）:
- visited mapによる即時検出（変数参照時に検出、反復回数制限は不要）

### 3.2 新規展開関数の仕様

#### 3.2.1 ExpandGlobalEnv()

**シグネチャ**:
```go
func ExpandGlobalEnv(
    cfg *GlobalConfig,
    expander *VariableExpander,
) error
```

**処理フロー**:
```
1. 入力検証:
   - cfg.Env が nil または空 → ExpandedEnv = nil, return nil

2. 環境変数マップの構築と重複チェック:
   envMap := make(map[string]string)
   for _, entry := range cfg.Env:
       key, value, ok := common.ParseEnvVariable(entry)  // 最初の'='で分割
       if !ok:
           return fmt.Errorf("invalid environment variable format: %s: %w", entry, environment.ErrMalformedEnvVariable)

       if err := validateKey(key); err != nil:  // KEY形式の検証
           return err

       // 重複キーのチェック
       if _, exists := envMap[key]; exists:
           return fmt.Errorf("duplicate environment variable %q in global.env", key)

       envMap[key] = value

3. 各変数の展開:
   for key, value := range envMap:
       if contains(value, "${"):
           // VariableExpander.ExpandString()を使用
           // Globalレベルでは envMap のみを参照（自己参照はシステム環境変数から）
           // 循環参照は内部でvisited mapにより即時検出される
           expanded, err := expander.ExpandString(
               value,
               envMap,              // envVars: 同レベルの変数マップ
               cfg.EnvAllowlist,    // allowlist
               "global",            // グループ名
               make(map[string]bool) // visited（各変数展開で新規作成）
           )
           if err != nil:
               return err
           envMap[key] = expanded

4. 結果の保存:
   cfg.ExpandedEnv = envMap
```

**エラー**:
- 未定義変数エラー（`ExpandString()`から）
- 循環参照エラー（`ExpandString()`のvisited mapで即時検出）
- Allowlist違反エラー（`ExpandString()`から）

#### 3.2.2 ExpandGroupEnv()

**シグネチャ**:
```go
func ExpandGroupEnv(
    group *CommandGroup,
    globalEnv map[string]string,
    globalAllowlist []string,
    expander *VariableExpander,
) error
```

**処理フロー**:
```
1. 有効なAllowlistの決定:
   effectiveAllowlist := globalAllowlist
   if group.EnvAllowlist != nil:
       effectiveAllowlist = group.EnvAllowlist

2. 入力検証:
   - group.Env が nil または空 → ExpandedEnv = nil, return nil

3. 環境変数マップの構築と重複チェック:
   envMap := make(map[string]string)
   for _, entry := range group.Env:
       key, value, ok := common.ParseEnvVariable(entry)
       if !ok:
           return fmt.Errorf("invalid environment variable format in group %s: %s: %w", group.Name, entry, environment.ErrMalformedEnvVariable)

       if err := validateKey(key); err != nil:
           return err

       // 重複キーのチェック
       if _, exists := envMap[key]; exists:
           return fmt.Errorf("duplicate environment variable %q in group %q", key, group.Name)

       envMap[key] = value

4. 各変数の展開:
   // globalEnvとenvMapをマージした環境で展開
   // ExpandString()は1つの環境変数マップしか受け取らないため、事前にマージする
   combinedEnv := make(map[string]string)
   maps.Copy(combinedEnv, globalEnv)  // Globalをコピー
   maps.Copy(combinedEnv, envMap)     // Groupをコピー（上書き）

   for key, value := range envMap:
       if contains(value, "${"):
           // 循環参照は内部でvisited mapにより即時検出される
           expanded, err := expander.ExpandString(
               value,
               combinedEnv,         // envVars: Global + Group の変数マップ
               effectiveAllowlist,
               "group:" + group.Name,
               make(map[string]bool) // visited（各変数展開で新規作成）
           )
           if err != nil:
               return err
           envMap[key] = expanded
           combinedEnv[key] = expanded  // 後続変数の参照用に更新

5. 結果の保存:
   group.ExpandedEnv = envMap
```

**重要な注意点**:
- `globalEnv`は読み取り専用（変更しない）
- `group.ExpandedEnv`にはGroupレベルで定義した変数のみを保存（Globalとマージしない）
- 変数解決時は`combinedEnv`（Global + Group）をマージして使用
- `combinedEnv`は展開中に動的に更新され、後続の変数が先に展開された変数を参照できる

#### 3.2.3 ExpandCommandEnv()の拡張

**既存のシグネチャ**:
```go
func (p *VariableExpander) ExpandCommandEnv(
    cmd *Command,
    groupName string,
    groupEnvAllowList []string,
    baseEnv map[string]string,
) (map[string]string, error)
```

**拡張方針**:
- 既存の`ExpandCommandEnv()`を変更せず活用
- `baseEnv`パラメータに`Global.Env + Group.Env`をマージして渡す

**呼び出し側の実装**:
```go
// Config Loaderで実行
baseEnv := make(map[string]string)
maps.Copy(baseEnv, global.ExpandedEnv)  // Globalをコピー
maps.Copy(baseEnv, group.ExpandedEnv)   // Groupをコピー（上書き）

expandedEnv, err := expander.ExpandCommandEnv(
    cmd,
    group.Name,
    effectiveAllowlist,
    baseEnv,  // Global + Group
)
```

**自己参照の動作**:
- `PATH=/local:${PATH}` の場合
- `${PATH}`は`baseEnv`（Group.Env → Global.Env → システム環境変数）から解決
- Command.Env内の`PATH`定義自身は除外される（既存実装の動作）

### 3.3 VerifyFiles展開の拡張

#### 3.3.1 Global.VerifyFiles

**既存関数の活用**（Task 0030で実装済み）:
```go
func ExpandGlobalVerifyFiles(
    global *GlobalConfig,
    filter *Filter,
    expander *VariableExpander,
) error
```

**本タスクでの拡張**:
- 内部で呼び出される`expandVerifyFiles()`の引数に`envVars map[string]string`を追加
- `expandVerifyFiles()`内で`systemEnv`と`envVars`をマージしてから`ExpandString()`に渡す
- `ExpandGlobalVerifyFiles()`から`global.ExpandedEnv`を渡す

**拡張後のシグネチャ**:
```go
func expandVerifyFiles(
    paths []string,
    allowlist []string,
    level string,
    envVars map[string]string,  // 新規追加: Global/Group.ExpandedEnv
    filter *Filter,
    expander *VariableExpander,
) ([]string, error)
```

**拡張後の処理フロー**:
```
1. systemEnv を allowlist でフィルタリング
2. envVars と systemEnv をマージ（envVars が優先）
3. マージした環境でExpandString()を呼び出す
4. 変数解決順序（ExpandString内部）:
   - Global.ExpandedEnv[VAR]
   - システム環境変数[VAR] (allowlistチェック済み)
```

#### 3.3.2 Group.VerifyFiles

**既存関数の活用**（Task 0030で実装済み）:
```go
func ExpandGroupVerifyFiles(
    group *CommandGroup,
    filter *Filter,
    expander *VariableExpander,
) error
```

**本タスクでの拡張**:
- 拡張された`expandVerifyFiles()`に`Global.ExpandedEnv + Group.ExpandedEnv`を渡す
- `ExpandGroupVerifyFiles()`に`globalConfig *GlobalConfig`パラメータを追加

**拡張後のシグネチャ**:
```go
func ExpandGroupVerifyFiles(
    group *CommandGroup,
    globalConfig *GlobalConfig,  // 新規追加: Global.ExpandedEnvを取得するため
    filter *Filter,
    expander *VariableExpander,
) error
```

**拡張後の処理フロー**:
```
1. Global.ExpandedEnv と Group.ExpandedEnv をマージ
   combinedEnv := make(map[string]string)
   maps.Copy(combinedEnv, globalConfig.ExpandedEnv)
   maps.Copy(combinedEnv, group.ExpandedEnv)
2. expandVerifyFiles()に combinedEnv を渡す
3. 変数解決順序（expandVerifyFiles内部）:
   - Group.ExpandedEnv[VAR]
   - Global.ExpandedEnv[VAR]
   - システム環境変数[VAR] (allowlistチェック済み)
```

## 4. Allowlistの詳細仕様

### 4.1 Allowlist継承ルール（既存仕様）

**判定ロジック**:
```go
func determineEffectiveAllowlist(group *CommandGroup, global *GlobalConfig) []string {
    if group.EnvAllowlist == nil {
        // nilスライス（TOML未定義）→ 継承
        return global.EnvAllowlist
    } else {
        // 空配列を含む定義済みスライス → 上書き
        return group.EnvAllowlist
    }
}
```

**3つのパターン**:

1. **継承**（`group.env_allowlist`未定義）:
```toml
[global]
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "group1"
# env_allowlist未定義 → globalを継承
# 有効なallowlist: ["PATH", "HOME"]
```

2. **上書き**（`group.env_allowlist`定義済み）:
```toml
[global]
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "group2"
env_allowlist = ["USER"]
# 有効なallowlist: ["USER"] のみ（PATHとHOMEは使用不可）
```

3. **全拒否**（`group.env_allowlist = []`）:
```toml
[[groups]]
name = "group3"
env_allowlist = []
# 有効なallowlist: [] （すべてのシステム環境変数を拒否）
```

### 4.2 Allowlistチェックの適用

**チェック対象**:
- システム環境変数参照時のみ

**チェック不要**:
- Global.ExpandedEnv, Group.ExpandedEnv, Command.ExpandedEnv で定義された変数

**理由**:
- 設定ファイル内で定義された変数は信頼できる
- システム環境変数は外部から注入される可能性があるため制御が必要

## 5. エラー処理の詳細仕様

### 5.1 エラー型

#### 5.1.1 既存エラーの再利用（internal/runner/environment）

- `ErrVariableNotFound`: 未定義変数
- `ErrCircularReference`: 循環参照
- `ErrNotInAllowlist`: allowlist違反
- `ErrInvalidVariableFormat`: 不正な変数形式
- `ErrInvalidEscapeSequence`: 無効なエスケープシーケンス
- `ErrMalformedEnvVariable`: 不正な環境変数フォーマット（`=`が無い、KEY部分が空）

#### 5.1.2 新規エラー（internal/runner/config）

- `ErrGlobalEnvExpansionFailed`: Global.Env展開エラー
- `ErrGroupEnvExpansionFailed`: Group.Env展開エラー
- `ErrDuplicateEnvVariable`: 重複する環境変数定義

**エラーラッピング**:
```go
if err := ExpandGlobalEnv(cfg, expander); err != nil {
    return fmt.Errorf("%w: %v", ErrGlobalEnvExpansionFailed, err)
}
```

### 5.2 エラーメッセージ

**未定義変数エラー**:
```
Error: Undefined environment variable 'DB_HOST'
Context: group.env:database
```

**Allowlist違反エラー**:
```
Error: Environment variable 'SECRET_KEY' not in allowlist
Effective allowlist: [HOME, PATH, USER]
Referenced in: group.env:production
```

**循環参照エラー**:
```
Error: Circular reference detected in global.env
Variable 'VAR_A' references itself through a chain of dependencies
```

**重複変数定義エラー**:
```
Error: duplicate environment variable "BASE" in global.env
```

```
Error: duplicate environment variable "APP_DIR" in group "production"
```

**不正なフォーマットエラー**:
```
Error: invalid env format (missing '='): INVALID_ENTRY
```

## 6. 実装の詳細

### 6.1 処理順序

**Config Loaderでの呼び出し順序**:
```go
func processConfig(cfg *Config, filter *Filter, expander *VariableExpander) error {
    // 1. Global.Env展開
    if err := ExpandGlobalEnv(&cfg.Global, expander); err != nil {
        return err
    }

    // 2. Global.VerifyFiles展開
    if err := ExpandGlobalVerifyFiles(&cfg.Global, filter, expander); err != nil {
        return err
    }

    // 3-4. 各GroupのEnvとVerifyFiles展開
    for i := range cfg.Groups {
        group := &cfg.Groups[i]

        // 3. Group.Env展開
        if err := ExpandGroupEnv(
            group,
            cfg.Global.ExpandedEnv,
            cfg.Global.EnvAllowlist,
            expander,
        ); err != nil {
            return err
        }

        // 4. Group.VerifyFiles展開
        if err := ExpandGroupVerifyFiles(group, &cfg.Global, filter, expander); err != nil {
            return err
        }

        // 5-6. 各CommandのEnvとCmd/Args展開
        for j := range group.Commands {
            cmd := &group.Commands[j]
            effectiveAllowlist := determineEffectiveAllowlist(group, &cfg.Global)

            // 5. Command.Env展開
            baseEnvForCmd := make(map[string]string)
            maps.Copy(baseEnvForCmd, cfg.Global.ExpandedEnv)
            maps.Copy(baseEnvForCmd, group.ExpandedEnv)

            expandedEnv, err := expander.ExpandCommandEnv(
                cmd,
                group.Name,
                effectiveAllowlist,
                baseEnvForCmd,
            )
            if err != nil {
                return err
            }
            cmd.ExpandedEnv = expandedEnv

            // 6. Cmd/Args展開（既存処理）
            // ...
        }
    }

    return nil
}
```

### 6.2 KEY=VALUE形式のパース

**既存関数の活用**:
```go
// internal/common/env.go (既存実装)
func ParseEnvVariable(env string) (key, value string, ok bool)
```

**使用例**:
```go
key, value, ok := common.ParseEnvVariable(entry)
if !ok {
    return fmt.Errorf("invalid environment variable format: %s: %w", entry, environment.ErrMalformedEnvVariable)
}
```

### 6.3 KEY名の検証

**validateKey()関数**:
```go
func validateKey(key string) error {
    // 予約プレフィックスチェック
    if strings.HasPrefix(key, "__RUNNER_") {
        return fmt.Errorf("environment variable %q uses reserved prefix", key)
    }

    // KEY形式チェック
    if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(key) {
        return fmt.Errorf("invalid environment variable name: %s", key)
    }

    return nil
}
```

## 7. テストケースの詳細仕様

### 7.1 ユニットテストケース

#### 7.1.1 Global.Env展開テスト

**TC-G-001: 基本的な展開**:
```go
input := []string{"VAR1=value1", "VAR2=value2"}
expected := map[string]string{"VAR1": "value1", "VAR2": "value2"}
```

**TC-G-002: 変数参照**:
```go
input := []string{"BASE=/opt", "APP=${BASE}/app"}
expected := map[string]string{"BASE": "/opt", "APP": "/opt/app"}
```

**TC-G-003: 自己参照**:
```go
// システム環境: PATH=/usr/bin
// Allowlist: ["PATH"]
input := []string{"PATH=/opt/bin:${PATH}"}
expected := map[string]string{"PATH": "/opt/bin:/usr/bin"}
```

**TC-G-004: 循環参照エラー**:
```go
input := []string{"A=${B}", "B=${A}"}
expectedError := ErrCircularReference
```

**TC-G-005: 重複キーエラー**:
```go
input := []string{"BASE=/opt", "BASE=/usr"}
expectedError := ErrDuplicateEnvVariable
```

**TC-G-006: 不正なフォーマットエラー**:
```go
input := []string{"INVALID_ENTRY"}  // '='が無い
expectedError := environment.ErrMalformedEnvVariable
```

#### 7.1.2 Group.Env展開テスト

**TC-GR-001: Global.Env参照**:
```go
globalEnv := map[string]string{"BASE": "/opt"}
input := []string{"APP=${BASE}/app"}
expected := map[string]string{"APP": "/opt/app"}
```

**TC-GR-002: Allowlist継承**:
```go
globalAllowlist := []string{"PATH"}
groupAllowlist := nil  // 継承
// PATHの参照が可能
```

**TC-GR-003: Allowlist上書き**:
```go
globalAllowlist := []string{"PATH"}
groupAllowlist := []string{"HOME"}
// PATHの参照は不可、HOMEのみ可能
```

**TC-GR-004: 重複キーエラー**:
```go
input := []string{"APP=/opt/app1", "APP=/opt/app2"}
expectedError := ErrDuplicateEnvVariable
```

#### 7.1.3 Command.Env展開テスト

**TC-C-001: 複数レベル参照**:
```go
globalEnv := map[string]string{"BASE": "/opt"}
groupEnv := map[string]string{"APP": "/opt/app"}
input := []string{"LOG=${APP}/logs"}
expected := map[string]string{"LOG": "/opt/app/logs"}
```

### 7.2 統合テストケース

#### 7.2.1 階層的展開テスト

```toml
[global]
env = ["BASE=/opt"]
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "app"
env = ["APP=${BASE}/myapp"]

[[groups.commands]]
name = "run"
cmd = "${APP}/bin/server"
env = ["LOG=${APP}/logs"]
```

**期待結果**:
- `Global.ExpandedEnv`: `{BASE: "/opt"}`
- `Group.ExpandedEnv`: `{APP: "/opt/myapp"}`
- `Command.ExpandedEnv`: `{LOG: "/opt/myapp/logs"}`
- `Command.ExpandedCmd`: `"/opt/myapp/bin/server"`

### 7.3 エッジケーステスト

**TC-EDGE-001: 特殊文字を含む値**:
```go
input := []string{"URL=https://example.com?a=1&b=2"}
expected := map[string]string{"URL": "https://example.com?a=1&b=2"}
```

**TC-EDGE-002: エスケープシーケンス**:
```go
input := []string{"PRICE=\\$100", "PATH=C:\\\\Windows"}
expected := map[string]string{"PRICE": "$100", "PATH": "C:\\Windows"}
```

## 8. 実装上の注意事項

### 8.1 既存コードとの統合

**修正が必要なファイル**:

1. **internal/runner/runnertypes/config.go**:
   - `GlobalConfig`と`CommandGroup`の構造体定義に`Env`と`ExpandedEnv`を追加

2. **internal/runner/config/expansion.go**:
   - `ExpandGlobalEnv()`関数を追加
   - `ExpandGroupEnv()`関数を追加
   - `expandVerifyFiles()`関数に`envVars`パラメータを追加
   - `ExpandGlobalVerifyFiles()`と`ExpandGroupVerifyFiles()`を拡張

3. **internal/runner/config/loader.go**:
   - `processConfig()`で新規展開関数を呼び出し

**修正が不要なファイル**:

1. **internal/runner/environment/processor.go**:
   - `ExpandString()`, `ExpandStrings()`, `ExpandCommandEnv()`は変更不要
   - `resolveVariable()`は変更不要

2. **internal/runner/executor/**:
   - 展開済みデータを使用するため変更不要

### 8.2 後方互換性の確保

**確認ポイント**:
1. `Global.Env`と`Group.Env`が未定義（nilまたは空配列）の場合、既存動作と同一
2. 既存のサンプルTOMLファイルがすべて動作
3. 既存のテストケースがすべてPASS

**互換性テスト**:
```go
// Global.Env未定義の場合
cfg := &GlobalConfig{
    Env: nil,  // または []string{}
}
// ExpandGlobalEnv()はエラーを返さず、ExpandedEnvはnilのまま
```

### 8.3 ドキュメント更新

**更新が必要なドキュメント**:
1. ユーザーガイド: Global/Group環境変数の使用方法
2. 設定ファイル仕様: TOMLフォーマットの拡張
3. Task 0030の要件定義書: VerifyFiles展開でGlobal.Env/Group.Envも参照可能になった旨の注記

## 9. まとめ

本仕様書は、Global・Groupレベル環境変数設定機能の実装に必要な詳細仕様を定義している。

**設計の要点**:
1. **既存実装の最大活用**: `VariableExpander`の既存メソッドを活用
2. **シンプルな実装**: 複雑な新規ロジックは追加せず、既存機能の組み合わせで実現
3. **後方互換性**: 新フィールドはオプショナル、既存動作を維持

実装時は本仕様書に従い、既存機能との互換性を保ちながら、段階的に機能を追加していくこと。
