# グループレベルコマンド許可リスト - 詳細仕様書

## 1. 仕様概要

### 1.1 目的

グループレベルコマンド許可リスト機能の詳細な実装仕様を定義し、開発者が実装時に参照できる技術的な詳細を提供する。

### 1.2 適用範囲

- データ型の詳細仕様
- 関数のシグネチャと実装仕様
- バリデーションルール
- エラーハンドリング仕様
- テストケース

## 2. データ型仕様

### 2.1 GroupSpec の拡張

#### 2.1.1 型定義

**ファイル**: `internal/runner/runnertypes/spec.go`

```go
type GroupSpec struct {
    // ... 既存フィールド ...

    // CmdAllowed はこのグループで実行を許可する追加コマンドのリスト
    // 各要素は絶対パス（変数展開前）
    //
    // 空の場合の動作:
    //   - nil: フィールドが省略された（グループレベルの追加許可なし）
    //   - []: 空配列が明示的に指定された（追加許可なし、nil と同じ動作）
    //
    // 例:
    //   cmd_allowed = ["/home/user/bin/tool1", "%{home}/bin/tool2"]
    CmdAllowed []string `toml:"cmd_allowed"`
}
```

#### 2.1.2 フィールド仕様

| プロパティ | 型 | 必須 | デフォルト | 説明 |
|-----------|---|-----|----------|------|
| `CmdAllowed` | `[]string` | No | `nil` | グループレベルで許可するコマンドの絶対パス |

#### 2.1.3 値の制約

| 制約 | 説明 |
|-----|------|
| 絶対パス | 各要素は変数展開後に絶対パスでなければならない |
| 変数参照 | `%{variable}` 形式の変数参照を含むことができる |
| 重複 | 重複は許可されるが、実行時に重複除去される |
| 空文字列 | 空文字列は不可（バリデーションエラー） |

### 2.2 RuntimeGroup の拡張

#### 2.2.1 型定義

**ファイル**: `internal/runner/runnertypes/runtime.go`

```go
type RuntimeGroup struct {
    // ... 既存フィールド ...

    // ExpandedCmdAllowed は変数展開後の許可コマンドリスト
    //
    // 各要素は以下の処理が完了している:
    //   1. 変数展開: %{var} -> 実際の値
    //   2. 絶対パス検証: / で始まることを確認
    //   3. シンボリックリンク解決: filepath.EvalSymlinks
    //   4. パス正規化: filepath.Clean
    //
    // nil の場合:
    //   - GroupSpec.CmdAllowed が nil または空配列だった
    //   - グループレベルの追加許可は適用されない
    //
    // 例:
    //   ["/home/user/bin/tool1", "/usr/local/bin/node"]
    ExpandedCmdAllowed []string
}
```

#### 2.2.2 プロパティの特性

| プロパティ | 特性 | 説明 |
|-----------|-----|------|
| 展開済み | すべて変数展開済み | `%{var}` はすべて実際の値に置換 |
| 絶対パス | すべて絶対パス | `/` で始まる |
| 正規化済み | `filepath.Clean` 適用 | `.` や `..` は解決済み |
| シンボリックリンク解決 | 実体パスに解決 | シンボリックリンクは実ファイルパスに変換 |

## 3. 関数仕様

### 3.1 ExpandGroup の拡張

#### 3.1.1 シグネチャ

**ファイル**: `internal/runner/config/expansion.go`

```go
// ExpandGroup はグループ設定を展開し RuntimeGroup を生成する
//
// 変更点:
//   - GroupSpec.CmdAllowed を展開して RuntimeGroup.ExpandedCmdAllowed に格納
//   - 各パスの変数展開、絶対パス検証、シンボリックリンク解決を実行
//
// Parameters:
//   - spec: 展開元のグループ設定
//   - globalRuntime: グローバルランタイム設定（変数の継承元）
//
// Returns:
//   - *RuntimeGroup: 展開後のランタイムグループ
//   - error: 展開エラー（変数未定義、絶対パス違反など）
func (e *Expander) ExpandGroup(
    spec *GroupSpec,
    globalRuntime *RuntimeGlobal,
) (*RuntimeGroup, error)
```

#### 3.1.2 実装詳細

```go
func (e *Expander) ExpandGroup(
    spec *GroupSpec,
    globalRuntime *RuntimeGlobal,
) (*RuntimeGroup, error) {
    runtime := &RuntimeGroup{Spec: spec}

    // ... 既存の変数展開処理（env_import, vars, env_vars など） ...

    // cmd_allowed の展開
    if spec.CmdAllowed != nil && len(spec.CmdAllowed) > 0 {
        expandedCmdAllowed, err := e.expandCmdAllowed(
            spec.CmdAllowed,
            runtime.ExpandedVars,
            spec.Name,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to expand cmd_allowed for group[%s]: %w", spec.Name, err)
        }
        runtime.ExpandedCmdAllowed = expandedCmdAllowed
    }

    // ... 既存の処理（VerifyFiles, WorkDir など） ...

    return runtime, nil
}
```

### 3.2 expandCmdAllowed 関数

#### 3.2.1 シグネチャ

```go
// expandCmdAllowed は cmd_allowed リストを展開する
//
// 処理内容:
//   1. 各パスの変数展開
//   2. 絶対パス検証
//   3. シンボリックリンク解決
//   4. パス正規化
//   5. 重複除去
//
// Parameters:
//   - rawPaths: 展開前のパスリスト（変数参照を含む可能性）
//   - vars: 変数マップ（%{key} -> value）
//   - groupName: グループ名（エラーメッセージ用）
//
// Returns:
//   - []string: 展開・正規化後のパスリスト
//   - error: 展開エラー
func (e *Expander) expandCmdAllowed(
    rawPaths []string,
    vars map[string]string,
    groupName string,
) ([]string, error)
```

#### 3.2.2 実装仕様

```go
func (e *Expander) expandCmdAllowed(
    rawPaths []string,
    vars map[string]string,
    groupName string,
) ([]string, error) {
    result := make([]string, 0, len(rawPaths))
    seen := make(map[string]bool) // 重複除去用

    for i, rawPath := range rawPaths {
        // 1. 空文字列チェック
        if rawPath == "" {
            return nil, fmt.Errorf("group[%s] cmd_allowed[%d]: path cannot be empty", groupName, i)
        }

        // 2. 変数展開
        expanded, err := e.expandString(rawPath, vars)
        if err != nil {
            return nil, fmt.Errorf("group[%s] cmd_allowed[%d] %s: %w", groupName, i, rawPath, err)
        }

        // 3. 絶対パス検証
        if !filepath.IsAbs(expanded) {
            return nil, &InvalidPathError{
                Path:   expanded,
                Reason: "cmd_allowed paths must be absolute (start with '/')",
            }
        }

        // 4. パス長検証
        if len(expanded) > e.config.MaxPathLength {
            return nil, &InvalidPathError{
                Path:   expanded,
                Reason: fmt.Sprintf("path length exceeds maximum (%d)", e.config.MaxPathLength),
            }
        }

        // 5. シンボリックリンク解決と正規化
        normalized, err := filepath.EvalSymlinks(expanded)
        if err != nil {
            return nil, fmt.Errorf("group[%s] cmd_allowed[%d] %s: failed to resolve path: %w", groupName, i, expanded, err)
        }

        // 6. 重複チェックと追加
        if !seen[normalized] {
            result = append(result, normalized)
            seen[normalized] = true
        }
    }

    return result, nil
}
```

### 3.3 ValidateCommandAllowed の拡張

#### 3.3.1 シグネチャ

**ファイル**: `internal/runner/security/validator.go`

```go
// ValidateCommandAllowed は、コマンドが実行許可されているかを検証する
//
// 検証ロジック:
//   1. AllowedCommands の正規表現パターンにマッチするかチェック
//      - マッチした場合: OK (return nil)
//   2. groupCmdAllowed が提供されている場合、リストに含まれるかチェック
//      - 含まれる場合: OK (return nil)
//   3. どちらもマッチしない場合: ErrCommandNotAllowed
//
// Parameters:
//   - cmdPath: 実行するコマンドのパス（絶対パス、変数展開済み）
//   - groupCmdAllowed: グループレベルの許可コマンドリスト（nil 可）
//
// Returns:
//   - error: 許可されていない場合は ErrCommandNotAllowed
func (v *Validator) ValidateCommandAllowed(
    cmdPath string,
    groupCmdAllowed []string,
) error
```

#### 3.3.2 実装仕様

```go
func (v *Validator) ValidateCommandAllowed(
    cmdPath string,
    groupCmdAllowed []string,
) error {
    // 入力検証
    if cmdPath == "" {
        return fmt.Errorf("command path cannot be empty")
    }

    // 1. AllowedCommands パターンマッチ
    for _, pattern := range v.config.AllowedCommands {
        re, err := regexp.Compile(pattern)
        if err != nil {
            return fmt.Errorf("invalid allowed command pattern %s: %w", pattern, err)
        }

        if re.MatchString(cmdPath) {
            return nil // OK: AllowedCommands にマッチ
        }
    }

    // 2. cmd_allowed リストチェック
    if len(groupCmdAllowed) > 0 {
        // コマンドパスを正規化（シンボリックリンク解決）
        normalizedCmd, err := filepath.EvalSymlinks(cmdPath)
        if err != nil {
            return fmt.Errorf("failed to resolve command path %s: %w", cmdPath, err)
        }

        for _, allowed := range groupCmdAllowed {
            if normalizedCmd == allowed {
                return nil // OK: cmd_allowed にマッチ
            }
        }
    }

    // 3. どちらもマッチしない場合はエラー
    return &CommandNotAllowedError{
        CommandPath:     cmdPath,
        AllowedPatterns: v.config.AllowedCommands,
        GroupCmdAllowed: groupCmdAllowed,
    }
}
```

## 4. エラー仕様

### 4.1 InvalidPathError

#### 4.1.1 型定義

**ファイル**: `internal/runner/config/errors.go`

```go
// InvalidPathError は、パスが不正な場合のエラー
type InvalidPathError struct {
    Path   string // 不正なパス
    Reason string // 理由
}

func (e *InvalidPathError) Error() string {
    return fmt.Sprintf("invalid path %s: %s", e.Path, e.Reason)
}

func (e *InvalidPathError) Is(target error) bool {
    _, ok := target.(*InvalidPathError)
    return ok
}
```

#### 4.1.2 使用例

```go
// 相対パスの場合
&InvalidPathError{
    Path:   "bin/tool",
    Reason: "cmd_allowed paths must be absolute (start with '/')",
}

// パス長超過の場合
&InvalidPathError{
    Path:   "/very/long/path/...",
    Reason: "path length exceeds maximum (4096)",
}
```

### 4.2 CommandNotAllowedError の拡張

#### 4.2.1 型定義

**ファイル**: `internal/runner/security/errors.go`

```go
// CommandNotAllowedError は、コマンドが許可されていない場合のエラー
type CommandNotAllowedError struct {
    CommandPath     string   // 実行しようとしたコマンドのパス
    AllowedPatterns []string // AllowedCommands パターン
    GroupCmdAllowed []string // グループレベルの cmd_allowed リスト
}

func (e *CommandNotAllowedError) Error() string {
    var buf strings.Builder
    buf.WriteString(fmt.Sprintf("command not allowed: %s\n", e.CommandPath))

    // AllowedCommands の情報
    buf.WriteString("  - Not matched by global allowed_commands patterns:\n")
    for _, pattern := range e.AllowedPatterns {
        buf.WriteString(fmt.Sprintf("      %s\n", pattern))
    }

    // cmd_allowed の情報
    if len(e.GroupCmdAllowed) > 0 {
        buf.WriteString("  - Not in group-level cmd_allowed list:\n")
        for _, allowed := range e.GroupCmdAllowed {
            buf.WriteString(fmt.Sprintf("      %s\n", allowed))
        }
    } else {
        buf.WriteString("  - Group-level cmd_allowed is not configured\n")
    }

    return buf.String()
}

func (e *CommandNotAllowedError) Is(target error) bool {
    return target == ErrCommandNotAllowed
}

func (e *CommandNotAllowedError) Unwrap() error {
    return ErrCommandNotAllowed
}
```

#### 4.2.2 エラーメッセージ例

```
command not allowed: /home/user/bin/unknown_tool
  - Not matched by global allowed_commands patterns:
      ^/bin/.*
      ^/usr/bin/.*
  - Not in group-level cmd_allowed list:
      /home/user/bin/custom_tool
      /opt/myapp/bin/processor
```

## 5. バリデーション仕様

### 5.1 設定ロード時のバリデーション

#### 5.1.1 検証項目

| 項目 | 検証内容 | エラー型 |
|-----|---------|---------|
| 空文字列 | `""` が含まれていないか | `InvalidPathError` |
| 絶対パス | 展開後のパスが `/` で始まるか | `InvalidPathError` |
| パス長 | `MaxPathLength` を超えていないか | `InvalidPathError` |
| 不正文字 | NULL文字などを含まないか | `InvalidPathError` |

#### 5.1.2 検証タイミング

- **タイミング**: `Expander.ExpandGroup()` 実行時
- **場所**: 変数展開直後
- **結果**: エラーの場合は `RuntimeGroup` の生成を中断

### 5.2 実行時のバリデーション

#### 5.2.1 検証項目

| 項目 | 検証内容 | エラー型 |
|-----|---------|---------|
| パターンマッチ | AllowedCommands にマッチするか | `CommandNotAllowedError` |
| リストマッチ | cmd_allowed に含まれるか | `CommandNotAllowedError` |

#### 5.2.2 検証ロジック

```
IF cmdPath matches any AllowedCommands pattern THEN
    RETURN OK
ELSE IF cmdPath is in groupCmdAllowed list THEN
    RETURN OK
ELSE
    RETURN CommandNotAllowedError
END IF
```

## 6. シンボリックリンク解決仕様

### 6.1 解決タイミング

| タイミング | 場所 | 目的 |
|-----------|-----|------|
| 設定ロード時 | `expandCmdAllowed()` | cmd_allowed の正規化 |
| コマンド検証時 | `ValidateCommandAllowed()` | 実行コマンドの正規化 |

### 6.2 エラーハンドリング

#### 6.2.1 ファイル不存在時の処理

**状況**: `filepath.EvalSymlinks()` が `ENOENT` エラーを返す

**動作**:
- **設定ロード時**: エラーを返す（設定ロード時点で実在するファイルのみ許可）
- **コマンド検証時**: エラーを返す（実行時点で実在するファイルのみ許可）

**理由**: 存在しないファイルを許可リストに含めることはセキュリティリスクとなるため

#### 6.2.2 その他のエラー

**状況**: パーミッション不足など、`ENOENT` 以外のエラー

**動作**: エラーを返す（シンボリックリンク解決に失敗した場合は実行を許可しない）

### 6.3 比較ロジック

```go
// cmd_allowed に "/usr/local/bin/node" が設定されている
// 実行コマンドが "/usr/local/bin/node" (シンボリックリンク)

// 設定ロード時
allowedPath := "/usr/local/bin/node"
resolved, _ := filepath.EvalSymlinks(allowedPath)
// resolved = "/usr/bin/node" (実体パス)

// コマンド検証時
cmdPath := "/usr/local/bin/node"
resolvedCmd, _ := filepath.EvalSymlinks(cmdPath)
// resolvedCmd = "/usr/bin/node" (実体パス)

// 比較
if resolvedCmd == resolved {
    // マッチ!
}
```

## 7. テストケース仕様

### 7.1 ユニットテスト: TOML パース

#### 7.1.1 正常系

| テストケース | 入力 | 期待値 |
|------------|-----|-------|
| 単一パス | `cmd_allowed = ["/bin/tool"]` | `[]string{"/bin/tool"}` |
| 複数パス | `cmd_allowed = ["/bin/a", "/bin/b"]` | `[]string{"/bin/a", "/bin/b"}` |
| 変数参照 | `cmd_allowed = ["%{home}/bin/tool"]` | パース成功（展開は後続処理） |
| 空配列 | `cmd_allowed = []` | `[]string{}` |
| フィールド省略 | `cmd_allowed` なし | `nil` |

#### 7.1.2 異常系

| テストケース | 入力 | 期待エラー |
|------------|-----|-----------|
| 型不一致 | `cmd_allowed = "/bin/tool"` | パースエラー |
| NULL含む | `cmd_allowed = ["/bin/tool\x00"]` | バリデーションエラー |

### 7.2 ユニットテスト: 変数展開

#### 7.2.1 正常系

| テストケース | 入力 | 変数 | 期待値 |
|------------|-----|-----|-------|
| 単一変数 | `%{home}/bin/tool` | `home=/home/user` | `/home/user/bin/tool` |
| 複数変数 | `%{root}/%{app}/bin` | `root=/opt, app=myapp` | `/opt/myapp/bin` |
| 変数なし | `/bin/tool` | - | `/bin/tool` |

#### 7.2.2 異常系

| テストケース | 入力 | 変数 | 期待エラー |
|------------|-----|-----|-----------|
| 未定義変数 | `%{undefined}/bin` | - | 変数未定義エラー |
| 相対パス | `bin/tool` | - | `InvalidPathError` |
| 空文字列 | `""` | - | `InvalidPathError` |
| パス長超過 | `/very/long/path...` (4097文字) | - | `InvalidPathError` |

### 7.3 ユニットテスト: コマンド許可チェック

#### 7.3.1 AllowedCommands マッチ

| テストケース | コマンド | AllowedCommands | cmd_allowed | 期待結果 |
|------------|---------|----------------|-------------|---------|
| パターンマッチ | `/bin/echo` | `["^/bin/.*"]` | `nil` | OK |
| 複数パターン | `/usr/bin/cat` | `["^/bin/.*", "^/usr/bin/.*"]` | `nil` | OK |

#### 7.3.2 cmd_allowed マッチ

| テストケース | コマンド | AllowedCommands | cmd_allowed | 期待結果 |
|------------|---------|----------------|-------------|---------|
| 完全一致 | `/home/user/bin/tool` | `[]` | `["/home/user/bin/tool"]` | OK |
| リスト複数 | `/opt/app/bin` | `[]` | `["/home/user/bin", "/opt/app/bin"]` | OK |

#### 7.3.3 OR 条件

| テストケース | コマンド | AllowedCommands | cmd_allowed | 期待結果 |
|------------|---------|----------------|-------------|---------|
| 両方マッチ | `/bin/echo` | `["^/bin/.*"]` | `["/bin/echo"]` | OK |
| 片方のみ（パターン） | `/bin/echo` | `["^/bin/.*"]` | `["/bin/cat"]` | OK |
| 片方のみ（リスト） | `/home/user/tool` | `["^/bin/.*"]` | `["/home/user/tool"]` | OK |

#### 7.3.4 エラーケース

| テストケース | コマンド | AllowedCommands | cmd_allowed | 期待結果 |
|------------|---------|----------------|-------------|---------|
| 両方不一致 | `/tmp/unknown` | `["^/bin/.*"]` | `["/home/user/tool"]` | `CommandNotAllowedError` |
| リスト空 | `/tmp/unknown` | `["^/bin/.*"]` | `[]` | `CommandNotAllowedError` |

### 7.4 統合テスト

#### 7.4.1 エンドツーエンドテスト

**シナリオ 1**: グループレベル cmd_allowed でコマンド実行

```toml
allowed_commands = ["^/bin/.*"]

[[groups]]
name = "build"
env_import = ["home=HOME"]
cmd_allowed = ["%{home}/bin/custom_tool"]

[[groups.commands]]
name = "run"
cmd = "%{home}/bin/custom_tool"
args = ["--verbose"]
```

**期待動作**:
1. `cmd_allowed` が `/home/user/bin/custom_tool` に展開される
2. コマンド実行時に `ValidateCommandAllowed` が呼ばれる
3. AllowedCommands にマッチしないが、cmd_allowed にマッチする
4. コマンドが正常に実行される

**シナリオ 2**: AllowedCommands のみでコマンド実行（既存動作）

```toml
allowed_commands = ["^/bin/.*"]

[[groups]]
name = "test"
# cmd_allowed なし

[[groups.commands]]
name = "echo"
cmd = "/bin/echo"
args = ["hello"]
```

**期待動作**:
1. `cmd_allowed` は nil
2. AllowedCommands パターンにマッチ
3. コマンドが正常に実行される

### 7.5 セキュリティテスト

#### 7.5.1 パストラバーサル攻撃

**テストケース**:
```toml
cmd_allowed = ["../../etc/passwd"]
```

**期待動作**: `InvalidPathError` （相対パス）

#### 7.5.2 シンボリックリンク攻撃

**テストケース**:
```bash
# /tmp/evil -> /etc/passwd (シンボリックリンク)
```

```toml
cmd_allowed = ["/tmp/evil"]
```

**期待動作**:
- シンボリックリンクが `/etc/passwd` に解決される
- セキュリティバリデーションで拒否される（他のチェックによる）

#### 7.5.3 他のセキュリティチェックのバイパス検証

**テストケース**:
```bash
# /home/user/bin/tool (world-writable)
chmod 777 /home/user/bin/tool
```

```toml
cmd_allowed = ["/home/user/bin/tool"]
```

**期待動作**:
- `cmd_allowed` でパターンマッチはバイパス
- ファイルパーミッション検証で拒否される

## 8. パフォーマンス要件

### 8.1 コマンド許可チェック

**要件**: 1コマンドあたり 1ms 未満

**計測ポイント**: `ValidateCommandAllowed()` の実行時間

**最適化**:
- AllowedCommands: 正規表現の事前コンパイル（既存）
- cmd_allowed: 線形探索（O(n)、n は通常 1-10 程度なので十分高速）

### 8.2 変数展開

**要件**: 1グループあたり 10ms 未満

**計測ポイント**: `expandCmdAllowed()` の実行時間

**最適化**:
- 重複除去は O(n) で実行（マップ使用）
- シンボリックリンク解決のキャッシュは初期実装では行わない

## 9. 実装チェックリスト

### 9.1 データ構造

- [ ] `GroupSpec` に `CmdAllowed` フィールド追加
- [ ] `RuntimeGroup` に `ExpandedCmdAllowed` フィールド追加

### 9.2 設定ロードと展開

- [ ] TOML パース機能（`cmd_allowed` フィールド）
- [ ] `expandCmdAllowed()` 関数実装
- [ ] 変数展開ロジック
- [ ] 絶対パスバリデーション
- [ ] シンボリックリンク解決
- [ ] 重複除去

### 9.3 セキュリティバリデーション

- [ ] `ValidateCommandAllowed()` の拡張
- [ ] AllowedCommands チェック
- [ ] cmd_allowed チェック
- [ ] OR 条件の実装

### 9.4 エラーハンドリング

- [ ] `InvalidPathError` 実装
- [ ] `CommandNotAllowedError` 拡張
- [ ] エラーメッセージの改善

### 9.5 テスト

- [ ] ユニットテスト: TOML パース
- [ ] ユニットテスト: 変数展開
- [ ] ユニットテスト: バリデーション
- [ ] ユニットテスト: コマンド許可チェック
- [ ] 統合テスト: エンドツーエンド
- [ ] セキュリティテスト: 攻撃シナリオ

## 10. 参照

### 10.1 関連ファイル

- `internal/runner/runnertypes/spec.go`: GroupSpec 定義
- `internal/runner/runnertypes/runtime.go`: RuntimeGroup 定義
- `internal/runner/config/expansion.go`: 変数展開ロジック
- `internal/runner/config/validation.go`: バリデーションロジック
- `internal/runner/security/validator.go`: セキュリティ検証

### 10.2 関連ドキュメント

- 要件定義書: `01_requirements.md`
- アーキテクチャ設計書: `02_architecture.md`
- 実装計画: `04_implementation_plan.md`

---

**文書バージョン**: 1.0
**作成日**: 2025-11-25
**承認日**: [未承認]
**次回レビュー予定**: [実装完了後]
