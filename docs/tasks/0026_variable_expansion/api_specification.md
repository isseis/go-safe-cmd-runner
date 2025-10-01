# API仕様書: コマンド・引数内環境変数展開機能

## 1. 概要

本ドキュメントは、go-safe-cmd-runnerの環境変数展開機能のAPI仕様を定義します。この機能は、TOML設定ファイル内のコマンド名（`cmd`）および引数（`args`）に記述された環境変数参照を実行時に展開します。

### 1.1 対象読者

- go-safe-cmd-runnerの開発者
- 内部APIを使用してカスタマイズを行う開発者
- 機能の保守・拡張を行う開発者

### 1.2 関連ドキュメント

- [ユーザーガイド](user_guide.md) - エンドユーザー向けの使用方法
- [アーキテクチャ設計書](02_architecture.md) - システム全体の設計
- [要件定義書](01_requirements.md) - 機能要件とセキュリティ要件

## 2. パッケージ構成

### 2.1 コアパッケージ

```
internal/runner/
├── environment/          # 環境変数処理パッケージ
│   ├── processor.go     # VariableExpander実装
│   ├── filter.go        # 環境変数フィルタリング
│   └── errors.go        # エラー定義
├── config/              # 設定管理パッケージ
│   ├── expansion.go     # コマンド展開統合
│   └── command.go       # Command構造体定義
└── security/            # セキュリティ検証パッケージ
    └── validator.go     # allowlist検証
```

## 3. 主要API

### 3.1 VariableExpander

**パッケージ**: `internal/runner/environment`

#### 3.1.1 構造体定義

```go
// VariableExpander handles variable expansion for command strings and environment maps.
type VariableExpander struct {
    filter    *Filter
    logger    *slog.Logger
    validator *security.Validator
}
```

**フィールド説明**:
- `filter`: 環境変数のフィルタリングとallowlist検証を行う
- `logger`: 構造化ログ出力用のロガー
- `validator`: セキュリティ検証用のキャッシュされたバリデータ

#### 3.1.2 コンストラクタ

```go
func NewVariableExpander(filter *Filter) *VariableExpander
```

**パラメータ**:
- `filter` (*Filter): 環境変数フィルタ（allowlist検証を含む）

**戻り値**:
- `*VariableExpander`: 初期化されたVariableExpanderインスタンス

**説明**:
- SecurityValidatorを内部でキャッシュ化し、性能を最適化
- デフォルト設定でValidatorを作成
- ロガーは"VariableExpander"コンポーネントとして初期化

**使用例**:
```go
filter := environment.NewFilter(allowlist, systemEnv)
expander := environment.NewVariableExpander(filter)
```

#### 3.1.3 ExpandCommandEnv

```go
func (p *VariableExpander) ExpandCommandEnv(
    cmd *runnertypes.Command,
    groupName string,
    groupEnvAllowList []string,
) (map[string]string, error)
```

**パラメータ**:
- `cmd` (*runnertypes.Command): 展開対象のコマンド
- `groupName` (string): グループ名（ログ出力用）
- `groupEnvAllowList` ([]string): グループレベルのallowlist

**戻り値**:
- `map[string]string`: 展開された環境変数マップ
- `error`: エラー（成功時はnil）

**説明**:
Command.Envブロックの環境変数を展開します。2パスアプローチを採用：
1. **第1パス**: すべての変数を未展開のままマップに追加
2. **第2パス**: 各変数の値を展開（相互参照に対応）

**エラー**:
- `ErrMalformedEnvVariable`: 環境変数の形式が不正
- `ErrCircularReference`: 循環参照を検出
- `ErrInvalidVariableName`: 変数名が不正
- セキュリティ検証エラー

**使用例**:
```go
env, err := expander.ExpandCommandEnv(cmd, "group1", allowlist)
if err != nil {
    return fmt.Errorf("failed to expand env: %w", err)
}
```

#### 3.1.4 ExpandString

```go
func (p *VariableExpander) ExpandString(
    value string,
    envVars map[string]string,
    allowlist []string,
    groupName string,
    visited map[string]bool,
) (string, error)
```

**パラメータ**:
- `value` (string): 展開対象の文字列
- `envVars` (map[string]string): 環境変数マップ
- `allowlist` ([]string): 許可された環境変数のリスト
- `groupName` (string): グループ名（ログ出力用）
- `visited` (map[string]bool): 循環参照検出用の訪問済み変数マップ

**戻り値**:
- `string`: 展開後の文字列
- `error`: エラー（成功時はnil）

**説明**:
単一の文字列内の`${VAR}`形式の変数参照を展開します。
- エスケープシーケンス（`\$`, `\\`）をサポート
- 再帰的な変数展開に対応
- visited mapによる循環参照検出

**処理アルゴリズム**:
1. 文字列を1文字ずつスキャン
2. `\`を検出 → エスケープシーケンス処理
3. `$`を検出 → 変数展開処理
4. その他 → そのまま出力

**エラー**:
- `ErrInvalidEscapeSequence`: 不正なエスケープシーケンス
- `ErrUnclosedVariable`: 閉じていない変数参照（`${VAR`のような）
- `ErrInvalidVariableFormat`: 不正な変数形式（`$VAR`のような）
- `ErrCircularReference`: 循環参照を検出
- `ErrVariableNotAllowed`: allowlistに含まれていない変数
- `ErrVariableNotFound`: 変数が定義されていない

**使用例**:
```go
visited := make(map[string]bool)
expanded, err := expander.ExpandString(
    "${HOME}/bin/tool",
    env,
    allowlist,
    "group1",
    visited,
)
// expanded = "/home/user/bin/tool"
```

#### 3.1.5 ExpandStrings

```go
func (p *VariableExpander) ExpandStrings(
    values []string,
    envVars map[string]string,
    allowlist []string,
    groupName string,
) ([]string, error)
```

**パラメータ**:
- `values` ([]string): 展開対象の文字列スライス
- `envVars` (map[string]string): 環境変数マップ
- `allowlist` ([]string): 許可された環境変数のリスト
- `groupName` (string): グループ名（ログ出力用）

**戻り値**:
- `[]string`: 展開後の文字列スライス
- `error`: エラー（成功時はnil）

**説明**:
複数の文字列を一括展開します。各文字列は独立して展開され、それぞれに新しいvisited mapが使用されます。

**使用例**:
```go
args := []string{"--input", "${INPUT_FILE}", "--output", "${OUTPUT_FILE}"}
expandedArgs, err := expander.ExpandStrings(args, env, allowlist, "group1")
// expandedArgs = ["--input", "/data/input.txt", "--output", "/data/output.txt"]
```

### 3.2 Config Expansion API

**パッケージ**: `internal/runner/config`

#### 3.2.1 ExpandCommand

```go
func ExpandCommand(
    cmd *runnertypes.Command,
    expander *environment.VariableExpander,
    allowlist []string,
    groupName string,
) (string, []string, map[string]string, error)
```

**パラメータ**:
- `cmd` (*runnertypes.Command): 展開対象のコマンド
- `expander` (*environment.VariableExpander): 変数展開エンジン
- `allowlist` ([]string): 許可された環境変数のリスト
- `groupName` (string): グループ名

**戻り値**:
- `string`: 展開されたコマンド名（cmd）
- `[]string`: 展開された引数リスト（args）
- `map[string]string`: 展開された環境変数マップ
- `error`: エラー（成功時はnil）

**説明**:
コマンド全体（Cmd, Args, Env）を展開します。以下の順序で処理：
1. Command.Envの展開
2. コマンド名（cmd）の展開
3. 引数（args）の展開

**使用例**:
```go
expander := environment.NewVariableExpander(filter)
expandedCmd, expandedArgs, env, err := config.ExpandCommand(
    cmd,
    expander,
    allowlist,
    "group1",
)
if err != nil {
    return fmt.Errorf("command expansion failed: %w", err)
}
```

## 4. エラー型

### 4.1 エラー定義

**パッケージ**: `internal/runner/environment`

```go
var (
    // ErrCircularReference is returned when a circular variable reference is detected.
    ErrCircularReference = errors.New("circular variable reference detected")

    // ErrInvalidEscapeSequence is returned when an invalid escape sequence is detected.
    ErrInvalidEscapeSequence = errors.New("invalid escape sequence (only \\$ and \\\\ are allowed)")

    // ErrUnclosedVariable is returned when a variable expansion is not properly closed.
    ErrUnclosedVariable = errors.New("unclosed variable reference (missing closing '}')")

    // ErrInvalidVariableFormat is returned when $ is found but not followed by valid variable syntax.
    ErrInvalidVariableFormat = errors.New("invalid variable format (use ${VAR} syntax)")
)
```

### 4.2 エラーハンドリング

**エラーチェック**:
```go
if err != nil {
    if errors.Is(err, environment.ErrCircularReference) {
        // 循環参照エラーの処理
    } else if errors.Is(err, environment.ErrInvalidEscapeSequence) {
        // エスケープシーケンスエラーの処理
    }
    // その他のエラー処理
}
```

**エラーメッセージ**:
すべてのエラーには詳細なコンテキスト情報が含まれます：
- 変数名
- 位置情報
- グループ名
- コマンド名

## 5. データ型

### 5.1 Command構造体

**パッケージ**: `internal/runner/runnertypes`

```go
type Command struct {
    Name        string   // コマンド識別名
    Description string   // コマンドの説明
    Cmd         string   // 実行するコマンドパス（変数展開可能）
    Args        []string // コマンド引数（各要素で変数展開可能）
    Env         []string // コマンド固有の環境変数（"KEY=VALUE"形式）
    WorkDir     string   // 作業ディレクトリ
    Timeout     int      // タイムアウト（秒）
    // ... その他のフィールド
}
```

### 5.2 環境変数マップ

環境変数は`map[string]string`として表現されます：
```go
env := map[string]string{
    "HOME": "/home/user",
    "PATH": "/usr/local/bin:/usr/bin",
    "CUSTOM_VAR": "custom_value",
}
```

### 5.3 visited マップ

循環参照検出のための訪問済み変数を追跡：
```go
visited := map[string]bool{
    "VAR1": true,  // VAR1は展開中
    "VAR2": true,  // VAR2は展開中
}
```

## 6. 使用パターン

### 6.1 基本的な使用フロー

```go
// 1. Filterの作成
filter := environment.NewFilter(allowlist, systemEnv)

// 2. VariableExpanderの作成
expander := environment.NewVariableExpander(filter)

// 3. コマンドの展開
expandedCmd, expandedArgs, env, err := config.ExpandCommand(
    cmd,
    expander,
    allowlist,
    groupName,
)
if err != nil {
    return fmt.Errorf("expansion failed: %w", err)
}

// 4. 展開結果の使用
executor.Execute(expandedCmd, expandedArgs, env)
```

### 6.2 個別文字列の展開

```go
expander := environment.NewVariableExpander(filter)

// 環境変数マップの準備
env := map[string]string{
    "HOME": "/home/user",
    "USER": "alice",
}

// 単一文字列の展開
visited := make(map[string]bool)
expanded, err := expander.ExpandString(
    "${HOME}/workspace/${USER}",
    env,
    allowlist,
    "group1",
    visited,
)
// expanded = "/home/user/workspace/alice"
```

### 6.3 複数文字列の一括展開

```go
values := []string{
    "--config", "${HOME}/.config/app.conf",
    "--user", "${USER}",
    "--output", "${OUTPUT_DIR}/result.txt",
}

expandedValues, err := expander.ExpandStrings(
    values,
    env,
    allowlist,
    "group1",
)
```

### 6.4 エスケープシーケンスの処理

```go
// リテラルの$記号を含む文字列
input := "Price is \\$100"
expanded, _ := expander.ExpandString(input, env, allowlist, "group1", visited)
// expanded = "Price is $100"

// バックスラッシュのエスケープ
input = "Path: \\\\${HOME}"
expanded, _ := expander.ExpandString(input, env, allowlist, "group1", visited)
// expanded = "Path: \\/home/user"
```

## 7. セキュリティ考慮事項

### 7.1 allowlist検証

すべての変数参照はallowlist検証を通過する必要があります：

```go
// Command.Envで定義された変数は自動的に許可
cmd.Env = []string{"MY_VAR=value"}  // MY_VARは検証不要

// システム環境変数はallowlistが必要
allowlist := []string{"HOME", "USER", "PATH"}
```

### 7.2 循環参照検出

visited mapによって循環参照を自動的に検出：

```go
env := map[string]string{
    "A": "${B}",
    "B": "${A}",  // 循環参照
}

_, err := expander.ExpandString("${A}", env, allowlist, "group1", visited)
// err = ErrCircularReference
```

### 7.3 変数名の検証

変数名は以下の条件を満たす必要があります：
- 英数字とアンダースコアのみ
- 最初の文字は英字またはアンダースコア
- 空文字列は不可

```go
// 正しい変数名
"MY_VAR", "HOME", "PATH", "_INTERNAL"

// 不正な変数名
"123VAR"  // 数字で始まる
"MY-VAR"  // ハイフンを含む
""        // 空文字列
```

## 8. 性能特性

### 8.1 計算量

- **ExpandString**: O(n)（nは文字列長）
- **ExpandStrings**: O(m * n)（mは文字列数、nは平均文字列長）
- **循環参照検出**: O(1)（visited mapによる）

### 8.2 メモリ使用量

- **visited map**: O(d)（dは展開深度）
- **result buffer**: O(n)（nは出力文字列長）
- **キャッシュされたValidator**: O(1)（再利用）

### 8.3 最適化

以下の最適化が実装されています：
1. **SecurityValidatorのキャッシュ化**: 正規表現の再コンパイルを回避
2. **strings.Builder使用**: 文字列結合の効率化
3. **visited map再利用**: メモリアロケーションの削減

## 9. テスト戦略

### 9.1 単体テスト

各APIメソッドに対して以下のテストを実施：
- 正常系：基本的な変数展開
- 異常系：エラーケース（循環参照、不正な形式等）
- 境界値：空文字列、長大な文字列、深いネスト

### 9.2 統合テスト

実際のコマンド実行を含む統合テスト：
- Config Parserとの統合
- Security Validatorとの統合
- 実際のTOMLファイルによるテスト

### 9.3 ベンチマーク

性能要件の検証：
- 処理時間：1ms/要素以下
- メモリ使用量：展開前の2倍以下
- CPU使用率：5%以下の増加

## 10. 変更履歴

| バージョン | 日付 | 変更内容 | 担当者 |
|----------|------|---------|--------|
| 1.0.0 | 2025-10-01 | 初版作成 | - |

## 11. 参考資料

### 11.1 内部ドキュメント
- [ユーザーガイド](user_guide.md)
- [アーキテクチャ設計書](02_architecture.md)
- [要件定義書](01_requirements.md)
- [実装計画書](04_implementation_plan.md)

### 11.2 コードリファレンス
- `internal/runner/environment/processor.go`
- `internal/runner/config/expansion.go`
- `internal/runner/security/validator.go`

### 11.3 テストコード
- `internal/runner/environment/processor_test.go`
- `internal/runner/config/expansion_test.go`
- `internal/runner/config/expansion_benchmark_test.go`
