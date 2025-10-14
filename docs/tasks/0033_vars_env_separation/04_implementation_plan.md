# 実装計画書: 内部変数とプロセス環境変数の分離

## 1. 実装概要

### 1.1 目的
内部変数（TOML展開用）とプロセス環境変数（子プロセス用）を明確に分離し、セキュリティを向上させる機能を段階的に実装する。要件定義書・アーキテクチャ設計書・詳細設計書に基づいた確実な開発を行う。

### 1.2 実装方針
- **テスト駆動開発（TDD）**: 各機能の実装前にテストを作成
- **段階的実装**: Phase 1から順次実装し、各Phaseで動作確認
- **破壊的変更の管理**: `${VAR}` 構文の完全廃止に伴う移行支援
- **セキュリティ優先**: allowlistチェックと循環参照検出の徹底

### 1.3 実装スコープ
本タスクで実装する機能:
- `FromEnv`/`Vars` フィールドの追加（Global, Group, Command レベル）
- 内部変数展開エンジン（`%{VAR}` 構文）
- `from_env` 処理（システム環境変数の取り込み、Override継承方式）
- `vars` 処理（内部変数の定義、階層的マージ）
- `env` フィールドでの内部変数参照
- `cmd`, `args`, `verify_files` での内部変数参照
- `${VAR}` 構文の完全廃止とエラー処理
- 自動設定内部変数（`%{__runner_datetime}`, `%{__runner_pid}`）
- エスケープシーケンス（`\%`, `\\`）
- 循環参照検出
- 移行ドキュメントの作成

### 1.4 重要な変更点
- **破壊的変更**: `${VAR}` 構文の完全廃止
- **新構文**: `%{VAR}` に統一
- **システム環境変数アクセス**: `from_env` 経由でのみ可能
- **env の役割変更**: 子プロセス環境変数専用（TOML展開では参照不可）

## 2. 実装フェーズ

### Phase 1: データ構造の拡張と基本バリデーション
**目的**: 新しいフィールドを追加し、基本的なパースとバリデーションを実装

#### 2.1.1 構造体定義の拡張
- [x] `internal/runner/runnertypes/config.go`を編集
  - [x] `GlobalConfig`に以下のフィールドを追加:
    ```go
    FromEnv []string `toml:"from_env"` // System env var import
    Vars    []string `toml:"vars"`     // Internal variable definitions
    ExpandedVars map[string]string `toml:"-"` // Expanded internal variables
    ```
  - [x] `CommandGroup`に以下のフィールドを追加:
    ```go
    FromEnv []string `toml:"from_env"` // System env var import (with inheritance)
    Vars    []string `toml:"vars"`     // Group-level internal variables
    ExpandedVars map[string]string `toml:"-"` // Expanded internal variables
    ```
  - [x] `Command`に以下のフィールドを追加:
    ```go
    Vars []string `toml:"vars"` // Command-level internal variables
    ExpandedVars map[string]string `toml:"-"` // Expanded internal variables
    ```

#### 2.1.2 変数名バリデーション関数の実装
- [x] `internal/runner/config/validation.go`を作成または拡張
  - [x] `validateVariableName(name string) error`関数を実装
    - **既存の `security.ValidateVariableName` を再利用** (POSIX準拠チェック済み)
    - 予約プレフィックスチェック: `__runner_` で始まる名前を拒否（ユーザー定義変数）
    - 空文字列チェック（`security.ValidateVariableName`内で実施済み）
  - [x] テスト: `validation_test.go`
    - [x] 正常な変数名のテスト（`home`, `user_path`, `MY_VAR`）
    - [x] 不正な変数名のテスト（数字始まり、ハイフン含む等）
    - [x] 予約プレフィックスのテスト（`__runner_foo`）
    - [x] 空文字列のテスト
  - **注意**: `security.ValidateVariableName` は既にPOSIX準拠と空文字列チェックを実装済み。
    `config.validateVariableName` は予約プレフィックスチェックを追加するラッパー関数として実装。

#### 2.1.3 エラー型の定義
- [x] `internal/runner/config/errors.go`を編集または作成
  - [x] 以下のエラー型を定義:
    ```go
    // ErrInvalidVariableName is returned when a variable name does not conform to POSIX naming rules.
    type ErrInvalidVariableName struct {
        Level        string // "global", "group", "command"
        Field        string // "from_env", "vars", "env"
        VariableName string
        Reason       string
    }

    // ErrReservedVariableName is returned when a variable name starts with reserved prefix.
    type ErrReservedVariableName struct {
        Level        string
        Field        string
        VariableName string
        Prefix       string
    }

    // ErrVariableNotInAllowlist is returned when from_env references a system env var not in env_allowlist.
    type ErrVariableNotInAllowlist struct {
        Level           string
        SystemVarName   string
        InternalVarName string
        Allowlist       []string
    }

    // ErrCircularReference is returned when circular variable reference is detected.
    type ErrCircularReference struct {
        Level        string
        Field        string
        VariableName string
        Chain        []string
    }

    // ErrUndefinedVariable is returned when %{VAR} references an undefined variable.
    type ErrUndefinedVariable struct {
        Level        string
        Field        string
        VariableName string
        Context      string
    }

    // ErrInvalidEscapeSequence is returned when an invalid escape sequence is found.
    type ErrInvalidEscapeSequence struct{
        Level    string
        Field    string
        Sequence string
        Context  string
    }
    ```

#### 2.1.4 TOMLパースのテスト
- [x] サンプルTOMLファイルを作成: `testdata/phase1_basic_vars.toml`
  ```toml
  [global]
  env_allowlist = ["HOME", "PATH"]
  from_env = ["home=HOME", "path=PATH"]
  vars = ["app_dir=/opt/myapp"]
  env = ["BASE_DIR=%{app_dir}"]

  [[groups]]
  name = "test_group"
  vars = ["log_dir=%{app_dir}/logs"]
  env = ["LOG_DIR=%{log_dir}"]

  [[groups.commands]]
  name = "test_cmd"
  vars = ["temp_file=%{log_dir}/temp.log"]
  cmd = "/bin/echo"
  args = ["%{temp_file}"]
  ```
- [x] `internal/runner/config/loader_test.go`でパーステスト
  - [x] Global.FromEnv, Global.Varsが正しくパースされることを確認
  - [x] Group.FromEnv, Group.Varsが正しくパースされることを確認
  - [x] Command.Varsが正しくパースされることを確認
  - [x] ExpandedVarsがnilまたは空であることを確認（まだ展開していない）

#### 2.1.5 Phase 1の完了確認
- [x] すべての既存テストがPASS
- [x] Phase 1の新規テストがすべてPASS
- [x] `make lint`でエラーなし
- [x] `make fmt`でフォーマット完了
- [x] コミット: "Add FromEnv/Vars/ExpandedVars fields and basic validation"

---

### Phase 2: 内部変数展開エンジンの実装
**目的**: `%{VAR}` 構文の展開処理を実装

- [x] Phase 2 completed: InternalVariableExpander implemented and tests
  passed (verified 2025-10-14). Stopped after Phase 2 as requested.

#### 2.2.1 InternalVariableExpander構造体の実装（テスト先行）
- [x] テスト作成: `internal/runner/config/expansion_test.go`
  - [x] `TestExpandString_Basic`: 基本的な変数展開
    ```go
    input: "prefix_%{var1}_suffix"
    vars: {"var1": "value1"}
    expected: "prefix_value1_suffix"
    ```
  - [x] `TestExpandString_Multiple`: 複数の変数展開
    ```go
    input: "%{var1}/%{var2}"
    vars: {"var1": "a", "var2": "b"}
    expected: "a/b"
    ```
  - [x] `TestExpandString_Nested`: ネストした展開
    ```go
    input: "%{var3}"
    vars: {"var1": "x", "var2": "%{var1}/y", "var3": "%{var2}/z"}
    expected: "x/y/z"
    ```
  - [x] `TestExpandString_UndefinedVariable`: 未定義変数エラー
  - [x] `TestExpandString_CircularReference`: 循環参照エラー
    ```go
    vars: {"A": "%{B}", "B": "%{A}"}
    expected: ErrCircularReference
    ```
  - [x] `TestExpandString_EscapeSequence`: エスケープ処理
    ```go
    input: "literal \%{var1} and \\path"
    expected: "literal %{var1} and \path"
    ```
  - [x] `TestExpandString_InvalidEscape`: 不正なエスケープエラー
    ```go
    input: "\$invalid"
    expected: ErrInvalidEscapeSequence
    ```
- [x] テスト実行で失敗を確認
- [x] コミット: "Add tests for InternalVariableExpander (TDD)"

#### 2.2.2 InternalVariableExpander構造体の実装
- [x] `internal/runner/config/expansion.go`を編集
  - [x] `InternalVariableExpander`構造体を定義:
    ```go
    type InternalVariableExpander struct {
        logger *logging.Logger
    }

    func NewInternalVariableExpander(logger *logging.Logger) *InternalVariableExpander
    ```
  - [x] `ExpandString`メソッドを実装:
    ```go
    func (e *InternalVariableExpander) ExpandString(
        input string,
        internalVars map[string]string,
        level string,
        field string,
    ) (string, error)
    ```
    - [x] `%{VAR}` パターンのマッチング
    - [x] エスケープシーケンス処理（`\%` → `%`, `\\` → `\`）
    - [x] 無効なエスケープ検出（`\$`, `\x` など）
    - [x] 循環参照検出（visited map使用）
    - [x] 変数名バリデーション
    - [x] 未定義変数検出
  - [x] `expandStringRecursive`ヘルパー関数を実装
    - [x] visited mapで循環参照を追跡
    - [x] expansionChainでエラーメッセージ用のチェーンを記録
    - [x] 最大ネスト深度チェック（必要に応じて）

#### 2.2.3 InternalVariableExpanderのテスト実行
- [x] すべてのテストがPASS
- [x] エッジケースの追加テスト
  - [x] 空文字列の展開
  - [x] 変数名のみ（`%{var}`）
  - [x] 複雑なネスト（3階層以上）
- [x] コミット: "Implement InternalVariableExpander with %{VAR} syntax"

---

### Phase 3: from_env処理の実装
**目的**: システム環境変数を内部変数として取り込む機能を実装

#### 2.3.1 ProcessFromEnv()関数の実装（テスト先行）
- [x] テスト作成: `internal/runner/config/expansion_test.go`
  - [x] `TestProcessFromEnv_Basic`: 基本的な取り込み
    ```go
    fromEnv: ["home=HOME", "user=USER"]
    systemEnv: {"HOME": "/home/test", "USER": "testuser"}
    allowlist: ["HOME", "USER"]
    expected: {"home": "/home/test", "user": "testuser"}
    ```
  - [x] `TestProcessFromEnv_NotInAllowlist`: allowlist違反エラー
    ```go
    fromEnv: ["secret=SECRET"]
    allowlist: ["HOME"]  // SECRET not in allowlist
    expected: ErrVariableNotInAllowlist
    ```
  - [x] `TestProcessFromEnv_SystemVarNotSet`: システム変数が未設定
    ```go
    fromEnv: ["missing=MISSING_VAR"]
    systemEnv: {}
    allowlist: ["MISSING_VAR"]
    expected: {"missing": ""} with warning log
    ```
  - [x] `TestProcessFromEnv_InvalidInternalName`: 不正な内部変数名
    ```go
    fromEnv: ["123invalid=HOME"]
    expected: ErrInvalidVariableName
    ```
  - [x] `TestProcessFromEnv_ReservedPrefix`: 予約プレフィックス
    ```go
    fromEnv: ["__runner_home=HOME"]
    expected: ErrReservedVariableName
    ```
  - [x] `TestProcessFromEnv_InvalidFormat`: 不正なフォーマット
    ```go
    fromEnv: ["invalid_format"]  // no '='
    expected: error
    ```
- [x] テスト実行で失敗を確認
- [x] コミット: "Add tests for ProcessFromEnv (TDD)"

#### 2.3.2 ProcessFromEnv()関数の実装
- [x] `internal/runner/config/expansion.go`に追加
  - [x] `ProcessFromEnv`メソッドを実装:
    ```go
    func (e *InternalVariableExpander) ProcessFromEnv(
        fromEnv []string,
        envAllowlist []string,
        systemEnv map[string]string,
        level string,
    ) (map[string]string, error)
    ```
    - [x] allowlistマップの構築
    - [x] 各マッピングをパース（`internal_name=SYSTEM_VAR`）
    - [x] 内部変数名のバリデーション
    - [x] 予約プレフィックスチェック
    - [x] システム変数名のバリデーション
    - [x] allowlistチェック
    - [x] システム環境変数の値を取得（未設定時は空文字列 + 警告ログ）
    - [x] 結果マップに格納

#### 2.3.3 ProcessFromEnv()のテスト実行
- [x] すべてのテストがPASS
- [x] コミット: "Implement ProcessFromEnv for system env var import"

---

### Phase 4: vars処理の実装
**目的**: 内部変数の定義と展開を実装

#### 2.4.1 ProcessVars()関数の実装（テスト先行）
- [x] テスト作成: `internal/runner/config/expansion_test.go`
  - [x] `TestProcessVars_Basic`: 基本的な定義
    ```go
    vars: ["var1=value1", "var2=value2"]
    baseVars: {}
    expected: {"var1": "value1", "var2": "value2"}
    ```
  - [x] `TestProcessVars_ReferenceBase`: ベース変数の参照
    ```go
    vars: ["var2=%{var1}/sub"]
    baseVars: {"var1": "base"}
    expected: {"var1": "base", "var2": "base/sub"}
    ```
  - [x] `TestProcessVars_ReferenceOther`: 同レベル変数の参照
    ```go
    vars: ["var1=a", "var2=%{var1}/b", "var3=%{var2}/c"]
    expected: {"var1": "a", "var2": "a/b", "var3": "a/b/c"}
    ```
  - [x] `TestProcessVars_CircularReference`: 循環参照エラー（順次処理により未定義エラーとなる）
    ```go
    vars: ["A=%{B}", "B=%{A}"]
    expected: ErrUndefinedVariable (Bが未定義)
    ```
  - [x] `TestProcessVars_SelfReference`: 同名での拡張
    ```go
    vars: ["path=%{path}:/custom"]
    baseVars: {"path": "/usr/bin"}
    expected: {"path": "/usr/bin:/custom"}
    ```
  - [x] `TestProcessVars_InvalidFormat`: 不正なフォーマット
  - [x] `TestProcessVars_InvalidVariableName`: 不正な変数名
- [x] テスト実行で失敗を確認
- [x] コミット: "Add tests for ProcessVars (TDD)"

#### 2.4.2 ProcessVars()関数の実装
- [x] `internal/runner/config/expansion.go`に追加
  - [x] `ProcessVars`関数を実装:
    ```go
    func ProcessVars(
        vars []string,
        baseExpandedVars map[string]string,
        level string,
    ) (map[string]string, error)
    ```
    - [x] ベース変数をコピーして `result` マップを作成
    - [x] **第1パス（パースとバリデーション）**:
    - [x] `vars` 配列の全定義をパース
    - [x] 変数名のバリデーションを実行
    - [x] **第2パス（順次展開）**:
    - [x] 各変数を順番に展開し、`result` マップに追加
    - [x] 各変数は result マップ内の既存変数（baseVars + 前に定義された vars）を参照可能
- [x] `ErrInvalidVarsFormat` エラーを `errors.go` に追加

#### 2.4.3 ProcessVars()のテスト実行
- [x] すべてのテストがPASS
- [x] コミット: "Implement ProcessVars for internal variable definitions"

---

### Phase 5: env処理の拡張
**目的**: `env` フィールドで内部変数を参照可能にする

#### 2.5.1 ProcessEnv()関数の実装（テスト先行）
- [x] テスト作成: `internal/runner/config/expansion_test.go`
  - [x] `TestProcessEnv_Basic`: 基本的な展開
    ```go
    env: ["VAR1=value1", "VAR2=value2"]
    internalVars: {}
    expected: {"VAR1": "value1", "VAR2": "value2"}
    ```
  - [x] `TestProcessEnv_ReferenceInternalVars`: 内部変数の参照
    ```go
    env: ["BASE_DIR=%{app_dir}", "LOG_DIR=%{app_dir}/logs"]
    internalVars: {"app_dir": "/opt/myapp"}
    expected: {"BASE_DIR": "/opt/myapp", "LOG_DIR": "/opt/myapp/logs"}
    ```
  - [x] `TestProcessEnv_UndefinedInternalVar`: 未定義変数エラー
  - [x] `TestProcessEnv_InvalidEnvVarName`: 不正な環境変数名
- [x] テスト実行で失敗を確認
- [x] コミット: "Add tests for ProcessEnv (TDD)"

#### 2.5.2 ProcessEnv()関数の実装
- [x] `internal/runner/config/expansion.go`に追加
  - [x] `ProcessEnv`メソッドを実装:
    ```go
    func (e *InternalVariableExpander) ProcessEnv(
        env []string,
        internalVars map[string]string,
        level string,
    ) (map[string]string, error)
    ```
    - [x] 各環境変数定義をパース（`VAR=value`）
    - [x] 環境変数名のバリデーション（POSIX準拠）
    - [x] `ExpandString`で値を展開（internalVarsを使用）
    - [x] 結果マップに格納
    - [x] **注意**: env は他の env 変数を参照できない（internalVars のみ）

#### 2.5.3 ProcessEnv()のテスト実行
- [x] すべてのテストがPASS
- [x] コミット: "Implement ProcessEnv for environment variable expansion"

---

### Phase 6: Global設定処理の統合
**目的**: Global レベルでの from_env, vars, env の処理を統合

#### 2.6.1 expandGlobalConfig()関数の実装（テスト先行）
- [x] テスト作成: `internal/runner/config/expansion_test.go`
  - [x] `TestExpandGlobalConfig_Basic`: 基本的な展開フロー
    ```toml
    [global]
    env_allowlist = ["HOME"]
    from_env = ["home=HOME"]
    vars = ["app_dir=%{home}/app"]
    env = ["APP_DIR=%{app_dir}"]
    ```
  - [x] `TestExpandGlobalConfig_NoFromEnv`: from_env なし
  - [x] `TestExpandGlobalConfig_NoVars`: vars なし
  - [x] `TestExpandGlobalConfig_NoEnv`: env なし
  - [x] `TestExpandGlobalConfig_ComplexChain`: 複雑な参照チェーン
- [x] テスト実行で失敗を確認
- [x] コミット: "Add tests for expandGlobalConfig (TDD)"

#### 2.6.2 expandGlobalConfig()関数の実装
- [x] `internal/runner/config/expansion.go`に追加
  - [x] `ExpandGlobalConfig`関数を実装:
    ```go
    func ExpandGlobalConfig(
        global *runnertypes.GlobalConfig,
        filter *environment.Filter,
    ) error
    ```
    - [x] システム環境変数の取得（`filter.ParseSystemEnvironment`）
    - [x] `ProcessFromEnv`で Global.FromEnv を処理
    - [x] `ProcessVars`で Global.Vars を展開
    - [x] `Global.ExpandedVars`に結果を保存
    - [x] `ProcessEnv`で Global.Env を展開（ExpandedVars使用）
    - [x] `Global.ExpandedEnv`に結果を保存
    - [x] `ExpandString`で Global.VerifyFiles を展開（ExpandedVars使用）
    - [x] エラーハンドリング

#### 2.6.3 expandGlobalConfig()のテスト実行
- [x] すべてのテストがPASS
- [x] コミット: "Implement expandGlobalConfig for Global-level processing"

---

### Phase 7: Group設定処理の統合
**目的**: Group レベルでの from_env, vars, env の処理を統合（継承を含む）

#### 2.7.1 expandGroupConfig()関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestExpandGroupConfig_InheritFromEnv`: from_env 継承
    ```toml
    [global]
    env_allowlist = ["HOME", "PATH"]
    from_env = ["home=HOME", "path=PATH"]

    [[groups]]
    name = "inherit_group"
    # from_env 未定義 → Global を継承
    vars = ["config=%{home}/.config"]
    ```
  - [ ] `TestExpandGroupConfig_OverrideFromEnv`: from_env 上書き
    ```toml
    [global]
    from_env = ["home=HOME"]

    [[groups]]
    name = "override_group"
    env_allowlist = ["CUSTOM_VAR"]
    from_env = ["custom=CUSTOM_VAR"]
    # Global.from_env は無視される
    ```
  - [ ] `TestExpandGroupConfig_EmptyFromEnv`: from_env = []
  - [ ] `TestExpandGroupConfig_VarsMerge`: vars のマージ
  - [ ] `TestExpandGroupConfig_AllowlistInherit`: allowlist 継承
  - [ ] `TestExpandGroupConfig_AllowlistOverride`: allowlist 上書き
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for expandGroupConfig (TDD)"

#### 2.7.2 expandGroupConfig()関数の実装
- [ ] `internal/runner/config/loader.go`に追加
  - [ ] `expandGroupConfig`関数を実装:
    ```go
    func expandGroupConfig(
        group *runnertypes.CommandGroup,
        global *runnertypes.GlobalConfig,
        filter *environment.Filter,
        expander *InternalVariableExpander,
    ) error
    ```
    - [ ] from_env 継承判定:
      ```go
      if group.FromEnv == nil {
          // 未定義 → Global を継承
          baseInternalVars = copyMap(global.ExpandedVars)
      } else if len(group.FromEnv) == 0 {
          // 空配列 → 何も取り込まない
          baseInternalVars = make(map[string]string)
      } else {
          // 定義あり → Global.from_env を無視して上書き
          systemEnv := filter.ParseSystemEnvironment(nil)
          groupAllowlist := group.EnvAllowlist
          if groupAllowlist == nil {
              groupAllowlist = global.EnvAllowlist
          }
          baseInternalVars, err = expander.ProcessFromEnv(
              group.FromEnv, groupAllowlist, systemEnv, "group["+group.Name+"]")
      }
      ```
    - [ ] `ProcessVars`で Group.Vars を展開（baseInternalVars使用）
    - [ ] `Group.ExpandedVars`に結果を保存
    - [ ] `ProcessEnv`で Group.Env を展開（ExpandedVars使用）
    - [ ] `Group.ExpandedEnv`に結果を保存
    - [ ] `ExpandVerifyFiles`で Group.VerifyFiles を展開（ExpandedVars使用）
    - [ ] エラーハンドリング

#### 2.7.3 copyMap()ヘルパー関数の実装
- [ ] `internal/runner/config/loader.go`に追加
  ```go
  func copyMap(m map[string]string) map[string]string
  ```

#### 2.7.4 expandGroupConfig()のテスト実行
- [ ] すべてのテストがPASS
- [ ] コミット: "Implement expandGroupConfig with from_env inheritance"

---

### Phase 8: Command設定処理の統合
**目的**: Command レベルでの vars, env の処理を統合

#### 2.8.1 expandCommandConfig()関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestExpandCommandConfig_Basic`: 基本的な展開
    ```toml
    [[groups.commands]]
    name = "test"
    vars = ["temp=%{log_dir}/temp"]
    env = ["TEMP_DIR=%{temp}"]
    cmd = "%{temp}/script.sh"
    args = ["--log", "%{log_dir}"]
    ```
  - [ ] `TestExpandCommandConfig_InheritGroupVars`: Group.vars を参照
  - [ ] `TestExpandCommandConfig_InheritGlobalVars`: Global.vars を参照
  - [ ] `TestExpandCommandConfig_NoVars`: vars なし
  - [ ] `TestExpandCommandConfig_CmdExpansion`: cmd での展開
  - [ ] `TestExpandCommandConfig_ArgsExpansion`: args での展開
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for expandCommandConfig (TDD)"

#### 2.8.2 expandCommandConfig()関数の実装
- [ ] `internal/runner/config/loader.go`に追加
  - [ ] `expandCommandConfig`関数を実装:
    ```go
    func expandCommandConfig(
        cmd *runnertypes.Command,
        group *runnertypes.CommandGroup,
        expander *InternalVariableExpander,
    ) error
    ```
    - [ ] Group.ExpandedVars を継承（baseInternalVars）
    - [ ] `ProcessVars`で Command.Vars を展開
    - [ ] `Command.ExpandedVars`に結果を保存
    - [ ] `ProcessEnv`で Command.Env を展開
    - [ ] `Command.ExpandedEnv`に結果を保存
    - [ ] `ExpandString`で Command.Cmd を展開
    - [ ] `Command.ExpandedCmd`に結果を保存
    - [ ] 各 Command.Args を`ExpandString`で展開
    - [ ] `Command.ExpandedArgs`に結果を保存
    - [ ] エラーハンドリング

#### 2.8.3 expandCommandConfig()のテスト実行
- [ ] すべてのテストがPASS
- [ ] コミット: "Implement expandCommandConfig for Command-level processing"

---

### Phase 9: Config Loaderの統合
**目的**: 設定読み込みフローに変数展開処理を統合

#### 2.9.1 LoadConfig()の拡張
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] TOMLパース後に以下の処理を追加:
    ```go
    // Create expander
    expander := NewInternalVariableExpander(logger)

    // Expand Global config
    if err := expandGlobalConfig(&config.Global, filter, expander); err != nil {
        return nil, fmt.Errorf("failed to expand global config: %w", err)
    }

    // Expand each Group config
    for i := range config.Groups {
        group := &config.Groups[i]
        if err := expandGroupConfig(group, &config.Global, filter, expander); err != nil {
            return nil, fmt.Errorf("failed to expand group[%s] config: %w", group.Name, err)
        }

        // Expand each Command config
        for j := range group.Commands {
            cmd := &group.Commands[j]
            if err := expandCommandConfig(cmd, group, expander); err != nil {
                return nil, fmt.Errorf("failed to expand command[%s] config: %w", cmd.Name, err)
            }
        }
    }
    ```
  - [ ] エラーハンドリング

#### 2.9.2 統合テスト
- [ ] サンプルTOMLファイル: `testdata/phase9_integration.toml`
  ```toml
  [global]
  env_allowlist = ["HOME", "PATH"]
  from_env = ["home=HOME", "system_path=PATH"]
  vars = [
      "app_name=myapp",
      "app_dir=%{home}/%{app_name}",
      "data_dir=%{app_dir}/data"
  ]
  env = ["APP_DIR=%{app_dir}"]
  verify_files = ["%{app_dir}/verify.sh"]

  [[groups]]
  name = "processing"
  vars = [
      "input_dir=%{data_dir}/input",
      "output_dir=%{data_dir}/output"
  ]
  env = ["INPUT_DIR=%{input_dir}"]

  [[groups.commands]]
  name = "process_data"
  vars = ["temp_dir=%{input_dir}/temp"]
  cmd = "/usr/bin/process"
  args = ["--input", "%{input_dir}", "--temp", "%{temp_dir}"]
  env = ["TEMP_DIR=%{temp_dir}"]
  ```
- [ ] 統合テスト: `internal/runner/config/loader_test.go`
  - [ ] Global.ExpandedVarsが正しく展開される
  - [ ] Group.ExpandedVarsがGlobal.ExpandedVarsを継承
  - [ ] Command.ExpandedVarsがGroup/GlobalのExpandedVarsを継承
  - [ ] Command.ExpandedCmd, ExpandedArgsが正しく展開される
  - [ ] Command.ExpandedEnvが正しく展開される
  - [ ] Global/Group.VerifyFilesが正しく展開される

#### 2.9.3 Phase 9の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 9の新規テストがすべてPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Integrate variable expansion into config loader"

---

### Phase 10: 実行時環境変数の構築
**目的**: 子プロセス実行時の環境変数を構築

#### 2.10.1 BuildProcessEnvironment()関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/executor/environment_test.go`
  - [ ] `TestBuildProcessEnvironment_Basic`: 基本的なマージ
  - [ ] `TestBuildProcessEnvironment_Priority`: 優先順位のテスト
    - システム環境変数 < Global.env < Group.env < Command.env
  - [ ] `TestBuildProcessEnvironment_AllowlistFiltering`: allowlist フィルタリング
  - [ ] `TestBuildProcessEnvironment_EmptyEnv`: env が空の場合
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for BuildProcessEnvironment (TDD)"

#### 2.10.2 BuildProcessEnvironment()関数の実装
- [ ] `internal/runner/executor/environment.go`を作成または編集
  - [ ] `BuildProcessEnvironment`関数を実装:
    ```go
    func BuildProcessEnvironment(
        global *runnertypes.GlobalConfig,
        group *runnertypes.CommandGroup,
        cmd *runnertypes.Command,
        filter *environment.Filter,
    ) (map[string]string, error)
    ```
    - [ ] システム環境変数の取得（allowlist フィルタリング）
    - [ ] allowlist の決定（Group または Global）
    - [ ] 環境変数のマージ（優先順位順）:
      1. システム環境変数
      2. Global.ExpandedEnv
      3. Group.ExpandedEnv
      4. Command.ExpandedEnv
    - [ ] 結果マップを返す

#### 2.10.3 BuildProcessEnvironment()のテスト実行
- [ ] すべてのテストがPASS
- [ ] コミット: "Implement BuildProcessEnvironment for process env construction"

#### 2.10.4 Executorへの統合
- [ ] `internal/runner/executor/executor.go`を編集
  - [ ] `executeCommand()`内で`BuildProcessEnvironment()`を呼び出す
  - [ ] 構築した環境変数を`exec.Command`に設定
  - [ ] 既存の環境変数構築コードを削除または置き換え

#### 2.10.5 統合テスト
- [ ] E2Eテスト: `sample/`ディレクトリに新しいサンプルファイルを追加
- [ ] 実際にrunnerを実行して動作確認
- [ ] 子プロセスの環境変数が正しく設定されることを確認

#### 2.10.6 Phase 10の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 10の新規テストがすべてPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Integrate BuildProcessEnvironment into executor"

---

### Phase 11: 自動設定内部変数の実装
**目的**: `%{__runner_datetime}`, `%{__runner_pid}` を実装

#### 2.11.1 自動設定変数の生成関数実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestGenerateAutoVariables_DateTime`: 日時フォーマット確認
  - [ ] `TestGenerateAutoVariables_Pid`: PIDの取得確認
  - [ ] `TestGenerateAutoVariables_Immutable`: 実行中一定であることを確認
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for auto-generated variables (TDD)"

#### 2.11.2 自動設定変数の生成関数実装
- [ ] `internal/runner/config/expansion.go`に追加
  - [ ] `generateAutoVariables`関数を実装:
    ```go
    func generateAutoVariables() map[string]string {
        now := time.Now()
        return map[string]string{
            "__runner_datetime": now.Format("20060102_150405"),
            "__runner_pid":      strconv.Itoa(os.Getpid()),
        }
    }
    ```

#### 2.11.3 expandGlobalConfig()の拡張
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] `expandGlobalConfig()`の開始時に自動変数を生成
  - [ ] 自動変数を Global.ExpandedVars にマージ（最初に）
  - [ ] from_env, vars で上書き不可（予約プレフィックスチェックで保護）

#### 2.11.4 テストの実行
- [ ] すべてのテストがPASS
- [ ] 自動変数が cmd, args, env, verify_files で使用できることを確認
- [ ] コミット: "Implement auto-generated internal variables"

---

### Phase 12: ${VAR}構文の廃止とエラー処理
**目的**: `${VAR}` 構文の完全廃止とエラーメッセージの実装

#### 2.12.1 ${VAR}検出関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestDetectDollarSyntax_Found`: `${VAR}` を検出
  - [ ] `TestDetectDollarSyntax_NotFound`: `%{VAR}` のみ
  - [ ] `TestDetectDollarSyntax_Escaped`: `\${VAR}` は検出しない
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for ${VAR} syntax detection (TDD)"

#### 2.12.2 ${VAR}検出関数の実装
- [ ] `internal/runner/config/expansion.go`に追加
  - [ ] `detectDeprecatedDollarSyntax`関数を実装:
    ```go
    func detectDeprecatedDollarSyntax(input string, level string, field string) error {
        // Pattern: ${...} but not \${...}
        pattern := regexp.MustCompile(`(?:[^\\]|^)\$\{[^}]+\}`)
        if pattern.MatchString(input) {
            return &ErrDeprecatedSyntax{
                Level:   level,
                Field:   field,
                Input:   input,
                Message: "${VAR} syntax is no longer supported. Use %{VAR} for internal variables.",
            }
        }
        return nil
    }
    ```

#### 2.12.3 ExpandString()への統合
- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `ExpandString()`の最初で`detectDeprecatedDollarSyntax()`を呼び出す
  - [ ] エラーが返された場合は早期リターン

#### 2.12.4 エラー型の追加
- [ ] `internal/runner/config/errors.go`に追加
  ```go
  type ErrDeprecatedSyntax struct {
      Level   string
      Field   string
      Input   string
      Message string
  }
  ```

#### 2.12.5 テストの実行
- [ ] すべてのテストがPASS
- [ ] `${VAR}` を使用した設定ファイルでエラーが発生することを確認
- [ ] エラーメッセージに移行のヒントが含まれることを確認
- [ ] コミット: "Implement ${VAR} syntax detection and deprecation error"

---

### Phase 13: デバッグ機能の実装
**目的**: dry-run での変数展開トレースを実装

#### 2.13.1 デバッグトレース構造体の実装
- [ ] `internal/runner/debug/trace.go`を作成
  - [ ] `VariableExpansionTrace`構造体を定義:
    ```go
    type VariableExpansionTrace struct {
        Level          string            // "global", "group[name]", "command[name]"
        Phase          string            // "from_env", "vars", "env", "cmd", "args"
        Input          string
        Output         string
        ReferencedVars []string
        ExpandedVars   map[string]string
        Errors         []error
    }
    ```
  - [ ] `PrintTrace(w io.Writer)`メソッドを実装

#### 2.13.2 from_env継承状態の表示関数実装
- [ ] `internal/runner/debug/inheritance.go`を作成
  - [ ] `PrintFromEnvInheritance`関数を実装:
    ```go
    func PrintFromEnvInheritance(
        w io.Writer,
        global *runnertypes.GlobalConfig,
        group *runnertypes.CommandGroup,
    )
    ```
    - [ ] Global.from_env の状態を表示
      - 取り込まれたシステム環境変数のリスト
      - 各変数のマッピング（`internal_name=SYSTEM_VAR`）
    - [ ] Group.from_env の継承/上書き/空配列の状態を明確に表示:
      - **`group.FromEnv == nil`の場合（継承）**:
        - メッセージ例: `"Inheriting from_env from Global (3 variables: home, path, user)"`
        - 継承された変数のリストを表示
      - **`len(group.FromEnv) == 0`の場合（明示的な無効化）**:
        - メッセージ例: `"Explicitly disabled from_env (no system env vars imported)"`
        - Globalからの継承がないことを明示
      - **`len(group.FromEnv) > 0`の場合（上書き）**:
        - メッセージ例: `"Overriding Global from_env with Group-specific configuration"`
        - Groupで新たに取り込まれる変数のリスト
        - Globalで定義されていたがGroupでは使用不可になる変数を警告表示
        - 例: `"Warning: Global variables (home, path) are not available in this group"`
    - [ ] 上書き時に Global から使用不可になる変数を警告
    - [ ] allowlistの継承/上書きも同様に表示
      - Groupでallowlistを上書きした場合の影響を表示

#### 2.13.3 最終環境変数の表示関数実装
- [ ] `internal/runner/debug/environment.go`を作成
  - [ ] `PrintFinalEnvironment`関数を実装:
    ```go
    func PrintFinalEnvironment(
        w io.Writer,
        envVars map[string]string,
        global *runnertypes.GlobalConfig,
        group *runnertypes.CommandGroup,
        cmd *runnertypes.Command,
    )
    ```
    - [ ] 環境変数をソート表示
    - [ ] 各変数の由来（system/global/group/command）を表示

#### 2.13.4 dry-runへの統合
- [ ] `internal/runner/runner.go`を編集
  - [ ] dry-run モード時に上記の関数を呼び出す
  - [ ] 変数展開のトレース情報を出力
  - [ ] 出力フォーマット例:
    ```
    ===== Variable Expansion Trace =====

    [Global Level]
    from_env: home=HOME, path=PATH (2 variables imported)
    vars: app_name=myapp, app_dir=%{home}/%{app_name} -> /home/user/myapp
    env: APP_DIR=%{app_dir} -> /home/user/myapp

    [Group: processing]
    from_env: Inheriting from Global (2 variables: home, path)
    vars: input_dir=%{app_dir}/input -> /home/user/myapp/input
    env: INPUT_DIR=%{input_dir} -> /home/user/myapp/input

    [Command: process_data]
    vars: temp_dir=%{input_dir}/temp -> /home/user/myapp/input/temp
    env: TEMP_DIR=%{temp_dir} -> /home/user/myapp/input/temp
    cmd: /usr/bin/process (no expansion)
    args[0]: --input
    args[1]: %{input_dir} -> /home/user/myapp/input

    ===== Final Process Environment =====
    APP_DIR=/home/user/myapp (from Global)
    INPUT_DIR=/home/user/myapp/input (from Group)
    PATH=/usr/bin:/bin (from system, filtered by allowlist)
    TEMP_DIR=/home/user/myapp/input/temp (from Command)
    ```

#### 2.13.5 継承動作の警告例の実装
- [ ] Group が from_env を上書きする場合の警告例:
  ```
  [Group: custom_group]
  from_env: Overriding Global from_env with Group-specific configuration
    - Group imports: custom=CUSTOM_VAR
    - Warning: Global variables (home, path) are NOT available in this group
    - These variables will be undefined: %{home}, %{path}
  ```
- [ ] このような警告により、ユーザーが意図しない変数の未定義エラーを事前に理解できる

#### 2.13.6 Phase 13の完了確認
- [ ] dry-run で詳細な変数展開情報が表示されることを確認
- [ ] from_env の継承/上書き/無効化が明確に表示されることを確認
- [ ] 警告メッセージが適切に表示されることを確認
- [ ] コミット: "Implement debug trace for variable expansion with inheritance visibility"

---

### Phase 14: 移行ドキュメントの作成
**目的**: ユーザーが既存設定を新仕様に移行するためのドキュメントを作成

#### 2.14.1 構文早見表の作成
- [ ] `docs/migration/0033_cheatsheet.md`を作成
  - [ ] フィールド別の構文サポート表
  - [ ] 参照可能な変数の一覧
  - [ ] 例: `| cmd | %{VAR} | ✓ | from_env, vars |`

#### 2.14.2 変換例集の作成
- [ ] `docs/migration/0033_examples.md`を作成
  - [ ] PATH拡張の例（変更前 → 変更後）
  - [ ] HOMEディレクトリの使用例
  - [ ] 秘密情報の受け渡し例
  - [ ] env変数をTOML展開で使う例（varsへの移行）
  - [ ] 複雑な階層的設定の例
  - [ ] 自動設定変数の変更例（Task 0028からの移行）
  - [ ] **from_env 継承パターンの例**:
    - **例1: 継承する場合（from_env 未定義）**:
      ```toml
      [global]
      from_env = ["home=HOME"]
      [[groups]]
      name = "inherit"
      # from_env 未定義 → Global を継承
      vars = ["config=%{home}/.config"]  # OK
      ```
    - **例2: 上書きする場合（from_env 定義）**:
      ```toml
      [global]
      from_env = ["home=HOME"]
      [[groups]]
      name = "override"
      from_env = ["custom=CUSTOM"]
      vars = ["config=%{home}/.config"]  # ERROR: %{home} は未定義
      ```
    - **例3: 無効化する場合（from_env = []）**:
      ```toml
      [global]
      from_env = ["home=HOME"]
      [[groups]]
      name = "no_env"
      from_env = []  # 明示的に無効化
      vars = ["static=/opt/app"]  # システム環境変数を使わない
      ```

#### 2.14.3 FAQの作成
- [ ] `docs/migration/0033_faq.md`を作成
  - [ ] `${VAR}` 構文エラーの対処法
  - [ ] 未定義変数エラーの対処法
  - [ ] allowlist違反エラーの対処法
  - [ ] env変数を参照できないエラーの対処法
  - [ ] 自動設定変数が環境変数に設定されない問題
  - [ ] **from_env の継承ルールについて**:
    - Q: "Groupで from_env を定義したら、Globalの変数が使えなくなった"
    - A: Override継承方式により、Group.from_env を定義するとGlobal.from_env は完全に無視されます。dry-runで警告を確認してください。
  - [ ] **from_env の明示的な無効化について**:
    - Q: "Groupでシステム環境変数を一切使いたくない"
    - A: `from_env = []` と明示的に空配列を指定してください。これによりGlobalからの継承も無効化されます。
  - [ ] **from_env と allowlist の関係について**:
    - Q: "Group.from_env を定義したら allowlist エラーが出た"
    - A: Group.from_env で参照するシステム環境変数は、Group.env_allowlist（未定義ならGlobal.env_allowlist）に含まれている必要があります。

#### 2.14.4 移行チェックリストの作成
- [ ] `docs/migration/0033_checklist.md`を作成
  - [ ] すべての `${VAR}` を検索
  - [ ] システム環境変数の特定
  - [ ] `${__RUNNER_*}` 自動設定変数の特定
  - [ ] `env_allowlist` への追加
  - [ ] `from_env` での取り込み
  - [ ] `${VAR}` を `%{VAR}` に変更
  - [ ] `${__RUNNER_*}` を `%{__runner_*}` に変更
  - [ ] `env` で定義していた変数の `vars` への移動
  - [ ] テスト実行

#### 2.14.5 移行ガイド本体の作成
- [ ] `docs/migration/0033_vars_env_separation.md`を作成
  - [ ] 変更の概要
  - [ ] 移行が必要な理由
  - [ ] 段階的な移行手順
  - [ ] 上記のドキュメントへのリンク

#### 2.14.6 READMEの更新
- [ ] `README.md`を編集
  - [ ] 移行ガイドへのリンクを追加
  - [ ] 新しい変数システムの簡単な説明を追加

#### 2.14.7 Phase 14の完了確認
- [ ] すべての移行ドキュメントが作成されている
- [ ] ドキュメント内のリンクが正しい
- [ ] コミット: "Add migration documentation for vars/env separation"

---

### Phase 15: サンプルファイルの更新
**目的**: 既存のサンプルファイルを新仕様に更新

#### 2.15.1 既存サンプルファイルの特定
- [ ] `sample/`ディレクトリ内のすべての`.toml`ファイルをリストアップ
- [ ] 各ファイルで`${VAR}`構文を使用しているか確認

#### 2.15.2 サンプルファイルの更新
- [ ] 各サンプルファイルを新仕様に変更:
  - [ ] `${VAR}` を `%{VAR}` に変更
  - [ ] システム環境変数の使用箇所で `from_env` を追加
  - [ ] `env_allowlist` の追加
  - [ ] 必要に応じて `vars` の追加
  - [ ] コメントで新仕様の使い方を説明

#### 2.15.3 新しいサンプルファイルの作成
- [ ] `sample/vars_env_separation.toml`を作成
  - [ ] 内部変数システムのデモ
  - [ ] from_env の使用例
  - [ ] 階層的な変数参照の例
  - [ ] PATH拡張の例
  - [ ] 自動設定変数の使用例

#### 2.15.4 Phase 15の完了確認
- [ ] すべてのサンプルファイルが新仕様に準拠
- [ ] サンプルファイルが正常に読み込めることを確認
- [ ] コミット: "Update sample files to new vars/env separation syntax"

---

### Phase 16: 最終テストと統合
**目的**: 全機能の統合テストと性能確認

#### 2.16.1 包括的な統合テスト
- [ ] `internal/runner/config/integration_test.go`を作成
  - [ ] 複雑な設定ファイルでの動作確認
  - [ ] 全レベル（Global/Group/Command）での変数展開
  - [ ] from_env 継承の全パターン
  - [ ] allowlist 継承の全パターン
  - [ ] エラーケースの網羅的テスト

#### 2.16.2 性能テスト
- [ ] `internal/runner/config/expansion_bench_test.go`を作成
  - [ ] 変数展開のベンチマーク
  - [ ] 大量の変数でのスケーラビリティテスト
  - [ ] メモリ使用量の測定

#### 2.16.3 既存機能との互換性確認
- [ ] すべての既存テストが PASS することを確認
- [ ] 既存の機能が正常に動作することを確認
- [ ] 回帰テストの実行

#### 2.16.4 エラーメッセージの検証
- [ ] すべてのエラーケースで適切なエラーメッセージが表示されることを確認
- [ ] エラーメッセージに修正のヒントが含まれることを確認

#### 2.16.5 ドキュメントの最終レビュー
- [ ] すべてのドキュメントが正確であることを確認
- [ ] コードスニペットが実際に動作することを確認
- [ ] リンクが切れていないことを確認

#### 2.16.6 Phase 16の完了確認
- [ ] すべてのテストが PASS
- [ ] `make lint` でエラーなし
- [ ] `make fmt` でフォーマット完了
- [ ] 性能要件を満たしている（変数展開 < 1ms、読み込み時間増加 < 10%）
- [ ] コミット: "Complete integration and final testing"

---

## 3. リスク管理

### 3.1 技術リスク

| リスク | 影響 | 対策 | 担当Phase |
|-------|------|------|----------|
| 既存の環境変数展開処理との干渉 | 高 | 段階的な実装とテスト | Phase 2-10 |
| 複雑な変数参照での性能劣化 | 中 | ベンチマークテストで監視 | Phase 16 |
| 循環参照検出の漏れ | 高 | 包括的なテストケース | Phase 2, 4 |

### 3.2 後方互換性リスク

| リスク | 影響 | 対策 | 担当Phase |
|-------|------|------|----------|
| `${VAR}` 構文の廃止による既存設定の破壊 | 高 | 詳細な移行ドキュメント、エラーメッセージでのヒント提供 | Phase 12, 14 |
| システム環境変数の直接参照不可 | 高 | 移行例の充実、FAQ の作成 | Phase 14 |
| env で定義した変数の TOML 展開での参照不可 | 中 | 変換例の提供 | Phase 14 |

### 3.3 セキュリティリスク

| リスク | 影響 | 対策 | 担当Phase |
|-------|------|------|----------|
| allowlist のバイパス | 高 | from_env と env での厳格な allowlist チェック | Phase 3, 5 |
| 循環参照による無限ループ | 中 | visited map による検出、最大反復回数制限 | Phase 2, 4 |
| 予約変数名の衝突 | 低 | 予約プレフィックスチェック | Phase 1, 3, 4 |

## 4. 品質保証

### 4.1 テスト戦略

#### 4.1.1 単体テスト
- [ ] すべての新規関数にテストを作成
- [ ] カバレッジ目標: 95%以上
- [ ] エッジケースの網羅

#### 4.1.2 統合テスト
- [ ] 各Phaseで統合テストを実施
- [ ] 全レベル（Global/Group/Command）での動作確認
- [ ] エラーケースの確認

#### 4.1.3 回帰テスト
- [ ] 既存テストの継続的な実行
- [ ] 既存機能への影響確認

#### 4.1.4 E2Eテスト
- [ ] 実際の設定ファイルでの動作確認
- [ ] dry-run でのデバッグ情報確認
- [ ] runner の実行確認

### 4.2 性能目標

| 指標 | 目標 | 測定方法 |
|------|------|----------|
| 変数展開処理時間 | 1ms/変数以下 | ベンチマークテスト |
| 設定読み込み時間増加 | 10%以内 | ベンチマークテスト |
| メモリ使用量増加 | 変数定義の2倍以内 | メモリプロファイル |

### 4.3 コード品質

- [ ] `make lint` でエラーなし
- [ ] `make fmt` でフォーマット完了
- [ ] コードレビュー（自己レビュー）
- [ ] ドキュメントの正確性確認

## 5. 完了基準

### 5.1 機能的完了基準
- [ ] すべてのPhaseが完了
- [ ] すべてのテストが PASS
- [ ] 移行ドキュメントが完成
- [ ] サンプルファイルが新仕様に準拠

### 5.2 品質完了基準
- [ ] テストカバレッジ 95%以上
- [ ] 性能目標を達成
- [ ] `make lint` でエラーなし
- [ ] すべてのエラーケースで適切なエラーメッセージが表示

### 5.3 ドキュメント完了基準
- [ ] 構文早見表の作成完了
- [ ] 変換例集の作成完了（5つ以上の例）
- [ ] FAQ の作成完了（6つ以上の Q&A）
- [ ] 移行チェックリストの作成完了
- [ ] 移行ガイド本体の作成完了
- [ ] README への移行ガイドリンク追加

## 6. スケジュール概算

| Phase | 概算工数 | 累計工数 |
|-------|---------|----------|
| Phase 1: データ構造の拡張 | 0.5日 | 0.5日 |
| Phase 2: 内部変数展開エンジン | 1日 | 1.5日 |
| Phase 3: from_env 処理 | 1日 | 2.5日 |
| Phase 4: vars 処理 | 1日 | 3.5日 |
| Phase 5: env 処理の拡張 | 0.5日 | 4日 |
| Phase 6: Global 設定処理 | 1日 | 5日 |
| Phase 7: Group 設定処理 | 1.5日 | 6.5日 |
| Phase 8: Command 設定処理 | 1日 | 7.5日 |
| Phase 9: Config Loader 統合 | 1日 | 8.5日 |
| Phase 10: 実行時環境変数構築 | 1日 | 9.5日 |
| Phase 11: 自動設定内部変数 | 0.5日 | 10日 |
| Phase 12: ${VAR}構文の廃止 | 0.5日 | 10.5日 |
| Phase 13: デバッグ機能 | 1日 | 11.5日 |
| Phase 14: 移行ドキュメント | 2日 | 13.5日 |
| Phase 15: サンプルファイル更新 | 0.5日 | 14日 |
| Phase 16: 最終テストと統合 | 1日 | 15日 |
| **合計** | **15日** | - |

**注意**: 上記は概算であり、実際の工数は変動する可能性があります。

## 7. 次のステップ

実装を開始する前に:
1. [ ] 本実装計画書のレビュー
2. [ ] 要件定義書、アーキテクチャ設計書、詳細設計書の再確認
3. [ ] 開発環境の準備
4. [ ] Phase 1 の開始

実装開始後:
1. 各Phaseの完了時にチェックボックスをマーク
2. 問題が発生した場合は本ドキュメントを更新
3. 実装の進捗を記録
