# 実装計画書: Global・Groupレベル環境変数設定機能

## 1. 実装概要

### 1.1 目的
Global・Groupレベル環境変数設定機能を段階的に実装し、要件定義書・アーキテクチャ設計書・詳細仕様書に基づいた確実な開発を行う。

### 1.2 実装方針
- **テスト駆動開発（TDD）**: 各機能の実装前にテストを作成
- **段階的実装**: Phase 1から順次実装し、各Phaseで動作確認
- **後方互換性の維持**: 既存テストの継続的な実行
- **セキュリティ優先**: allowlistチェックと循環参照検出の徹底

### 1.3 実装スコープ
本タスクで実装する機能:
- Global.Env/Group.Envフィールドの追加
- Global/Group環境変数の展開処理
- VerifyFiles展開時のGlobal/Group.Env参照
- 階層的な変数参照と優先順位
- Allowlist統合（継承・上書き・全拒否）
- 循環参照検出
- エラーハンドリング

## 2. 実装フェーズ

### Phase 1: データ構造の拡張とバリデーション
**目的**: 新しいフィールドを追加し、基本的なパースとバリデーションを実装

#### 2.1.1 構造体定義の拡張
- [x] `internal/runner/runnertypes/config.go`を編集
  - [x] `GlobalConfig`に`Env []string`フィールドを追加
  - [x] `GlobalConfig`に`ExpandedEnv map[string]string`フィールドを追加（`toml:"-"`タグ付き）
  - [x] `CommandGroup`に`Env []string`フィールドを追加
  - [x] `CommandGroup`に`ExpandedEnv map[string]string`フィールドを追加（`toml:"-"`タグ付き）

#### 2.1.2 KEY名バリデーション関数の実装
- [x] `internal/runner/config/validation.go`を作成または拡張
  - [x] `validateEnvKey(key string) error`関数を実装
    - KEY形式チェック: `^[A-Za-z_][A-Za-z0-9_]*$`
    - 予約プレフィックスチェック: `__RUNNER_`で始まる名前を拒否
  - [x] テスト: `validation_test.go`
    - [x] 正常なKEY名のテスト
    - [x] 不正なKEY名のテスト（数字始まり、特殊文字含む等）
    - [x] 予約プレフィックスのテスト

#### 2.1.3 重複変数検出関数の実装
- [x] `internal/runner/config/validation.go`に追加
  - [x] `checkDuplicateKeys(envList []string, context string) error`関数を実装
    - `common.ParseEnvVariable()`でKEY=VALUEをパース
    - 重複キーを検出してエラーを返す
  - [x] テスト: `validation_test.go`
    - [x] 重複なしのテスト
    - [x] 重複ありのテスト
    - [x] 不正フォーマット（`=`なし）のテスト

#### 2.1.4 エラー型の定義
- [x] `internal/runner/config/errors.go`を編集または作成
  - [x] `ErrGlobalEnvExpansionFailed`エラー変数を定義
  - [x] `ErrGroupEnvExpansionFailed`エラー変数を定義
  - [x] `ErrDuplicateEnvVariable`エラー変数を定義

#### 2.1.5 TOMLパースのテスト
- [x] サンプルTOMLファイルを作成: `testdata/phase1_basic.toml`
  ```toml
  [global]
  env = ["VAR1=value1", "VAR2=value2"]

  [[groups]]
  name = "test_group"
  env = ["GROUP_VAR=group_value"]
  ```
- [x] `internal/runner/config/loader_test.go`でパーステスト
  - [x] Global.Envが正しくパースされることを確認
  - [x] Group.Envが正しくパースされることを確認
  - [x] ExpandedEnvがnilであることを確認（まだ展開していない）

#### 2.1.6 Phase 1の完了確認
- [x] すべての既存テストがPASS
- [x] Phase 1の新規テストがすべてPASS
- [x] `make lint`でエラーなし
- [ ] コミット: "Add Env/ExpandedEnv fields and validation for Global/Group levels"

---

### Phase 2: Global.Env展開の実装
**目的**: Global環境変数の展開処理を実装し、Global.VerifyFilesで参照可能にする

#### 2.2.1 ExpandGlobalEnv()関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestExpandGlobalEnv_Basic`: 基本的な展開
  - [ ] `TestExpandGlobalEnv_VariableReference`: Global.Env内での変数参照
  - [ ] `TestExpandGlobalEnv_SystemEnvReference`: システム環境変数参照
  - [ ] `TestExpandGlobalEnv_SelfReference`: 自己参照（`PATH=/custom:${PATH}`）
  - [ ] `TestExpandGlobalEnv_CircularReference`: 循環参照エラー
  - [ ] `TestExpandGlobalEnv_DuplicateKey`: 重複キーエラー
  - [ ] `TestExpandGlobalEnv_InvalidFormat`: 不正フォーマットエラー
  - [ ] `TestExpandGlobalEnv_AllowlistViolation`: allowlist違反エラー
  - [ ] `TestExpandGlobalEnv_Empty`: 空配列/nilの場合
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for ExpandGlobalEnv (TDD)"

#### 2.2.2 ExpandGlobalEnv()関数の実装
- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `ExpandGlobalEnv(cfg *GlobalConfig, expander *VariableExpander) error`を実装
    - [ ] 入力検証（nil/空チェック）
    - [ ] 重複キーチェック（`checkDuplicateKeys()`使用）
    - [ ] KEY名バリデーション（`validateEnvKey()`使用）
    - [ ] `common.ParseEnvVariable()`でKEY=VALUEをパース
    - [ ] `expander.ExpandString()`で各変数を展開
    - [ ] 結果を`cfg.ExpandedEnv`に保存
  - [ ] エラーハンドリング
    - [ ] 不正フォーマット → `ErrMalformedEnvVariable`をラップ
    - [ ] 重複キー → `ErrDuplicateEnvVariable`
    - [ ] 循環参照等 → `ErrGlobalEnvExpansionFailed`でラップ

#### 2.2.3 ExpandGlobalEnv()のテスト実行
- [ ] すべてのテストがPASS
- [ ] エッジケースの追加テスト
  - [ ] 特殊文字を含む値
  - [ ] エスケープシーケンス（`\$`, `\\`）
- [ ] コミット: "Implement ExpandGlobalEnv with variable expansion"

#### 2.2.4 Global.VerifyFiles展開の拡張（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestExpandGlobalVerifyFiles_WithGlobalEnv`: Global.Envを参照
  - [ ] `TestExpandGlobalVerifyFiles_SystemEnv`: システム環境変数を参照
  - [ ] `TestExpandGlobalVerifyFiles_Priority`: Global.Env > システム環境変数の優先順位
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for Global.VerifyFiles expansion with Global.Env (TDD)"

#### 2.2.5 expandVerifyFiles()の拡張
- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `expandVerifyFiles()`のシグネチャを拡張
    ```go
    func expandVerifyFiles(
        paths []string,
        allowlist []string,
        level string,
        envVars map[string]string,  // 新規追加
        filter *Filter,
        expander *VariableExpander,
    ) ([]string, error)
    ```
  - [ ] 実装の拡張
    - [ ] `filter.ResolveAllowlistConfiguration()`でシステム環境変数をフィルタリング
    - [ ] `envVars`とフィルタ済みシステム環境変数をマージ（`envVars`優先）
    - [ ] マージした環境で`expander.ExpandString()`を呼び出す

#### 2.2.6 ExpandGlobalVerifyFiles()の拡張
- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `ExpandGlobalVerifyFiles()`内で`expandVerifyFiles()`を呼び出す際に`global.ExpandedEnv`を渡す
  - [ ] `global.ExpandedEnv`がnilの場合は空mapを渡す

#### 2.2.7 Config Loaderの統合
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] TOMLパース後、`ExpandGlobalEnv()`を呼び出す
  - [ ] その後、`ExpandGlobalVerifyFiles()`を呼び出す
  - [ ] エラーハンドリング

#### 2.2.8 統合テスト
- [ ] サンプルTOMLファイル: `testdata/phase2_global_env.toml`
  ```toml
  [global]
  env = ["BASE_DIR=/opt/app", "LOG_LEVEL=info"]
  env_allowlist = ["HOME"]
  verify_files = ["${BASE_DIR}/verify.sh", "${HOME}/script.sh"]

  [[groups]]
  name = "test_group"
  [[groups.commands]]
  name = "test_cmd"
  cmd = "/bin/echo"
  args = ["${BASE_DIR}"]
  ```
- [ ] 統合テスト: `internal/runner/config/loader_test.go`
  - [ ] Global.ExpandedEnvが正しく展開される
  - [ ] Global.VerifyFilesでGlobal.Envを参照できる
  - [ ] Command.Args内でGlobal.Envを参照できる

#### 2.2.9 Phase 2の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 2の新規テストがすべてPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Implement Global.Env expansion and integrate with VerifyFiles"

---

### Phase 3: Group.Env展開の実装
**目的**: Group環境変数の展開処理を実装し、allowlist継承を統合

#### 2.3.1 Allowlist決定関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/allowlist_test.go`
  - [ ] `TestDetermineEffectiveAllowlist_Inherit`: Group.EnvAllowlist == nil
  - [ ] `TestDetermineEffectiveAllowlist_Override`: Group.EnvAllowlist != nil
  - [ ] `TestDetermineEffectiveAllowlist_Reject`: Group.EnvAllowlist == []
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for allowlist inheritance (TDD)"

#### 2.3.2 Allowlist決定関数の実装
- [ ] `internal/runner/config/allowlist.go`を作成
  - [ ] `determineEffectiveAllowlist(group *CommandGroup, global *GlobalConfig) []string`を実装
    ```go
    func determineEffectiveAllowlist(group *CommandGroup, global *GlobalConfig) []string {
        if group.EnvAllowlist == nil {
            return global.EnvAllowlist
        }
        return group.EnvAllowlist
    }
    ```
- [ ] テストがPASSすることを確認
- [ ] コミット: "Implement allowlist inheritance logic"

#### 2.3.3 ExpandGroupEnv()関数の実装（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestExpandGroupEnv_Basic`: 基本的な展開
  - [ ] `TestExpandGroupEnv_ReferenceGlobal`: Global.Envを参照
  - [ ] `TestExpandGroupEnv_ReferenceSystemEnv`: システム環境変数を参照
  - [ ] `TestExpandGroupEnv_AllowlistInherit`: allowlist継承
  - [ ] `TestExpandGroupEnv_AllowlistOverride`: allowlist上書き
  - [ ] `TestExpandGroupEnv_AllowlistReject`: allowlist全拒否
  - [ ] `TestExpandGroupEnv_CircularReference`: 循環参照エラー
  - [ ] `TestExpandGroupEnv_DuplicateKey`: 重複キーエラー
  - [ ] `TestExpandGroupEnv_Empty`: 空配列/nilの場合
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for ExpandGroupEnv (TDD)"

#### 2.3.4 ExpandGroupEnv()関数の実装
- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `ExpandGroupEnv(group *CommandGroup, globalEnv map[string]string, globalAllowlist []string, expander *VariableExpander) error`を実装
    - [ ] 有効なallowlistの決定（`determineEffectiveAllowlist()`使用）
    - [ ] 入力検証（nil/空チェック）
    - [ ] 重複キーチェック
    - [ ] KEY名バリデーション
    - [ ] `globalEnv`と`group.Env`をマージした`combinedEnv`を作成
    - [ ] `expander.ExpandString()`で各変数を展開
    - [ ] 結果を`group.ExpandedEnv`に保存（Groupレベルの変数のみ）
  - [ ] エラーハンドリング

#### 2.3.5 ExpandGroupEnv()のテスト実行
- [ ] すべてのテストがPASS
- [ ] コミット: "Implement ExpandGroupEnv with Global.Env reference"

#### 2.3.6 Group.VerifyFiles展開の拡張（テスト先行）
- [ ] テスト作成: `internal/runner/config/expansion_test.go`
  - [ ] `TestExpandGroupVerifyFiles_WithGroupEnv`: Group.Envを参照
  - [ ] `TestExpandGroupVerifyFiles_WithGlobalEnv`: Global.Envを参照
  - [ ] `TestExpandGroupVerifyFiles_Priority`: Group.Env > Global.Env > システム環境変数
- [ ] テスト実行で失敗を確認
- [ ] コミット: "Add tests for Group.VerifyFiles expansion (TDD)"

#### 2.3.7 ExpandGroupVerifyFiles()の拡張
- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `ExpandGroupVerifyFiles()`のシグネチャを拡張
    ```go
    func ExpandGroupVerifyFiles(
        group *CommandGroup,
        globalConfig *GlobalConfig,  // 新規追加
        filter *Filter,
        expander *VariableExpander,
    ) error
    ```
  - [ ] 実装の拡張
    - [ ] `Global.ExpandedEnv`と`Group.ExpandedEnv`をマージ
    - [ ] マージした環境を`expandVerifyFiles()`に渡す

#### 2.3.8 Config Loaderの統合
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] 各グループに対して`ExpandGroupEnv()`を呼び出す
  - [ ] その後、`ExpandGroupVerifyFiles()`を呼び出す（`globalConfig`を渡す）
  - [ ] エラーハンドリング

#### 2.3.9 統合テスト
- [ ] サンプルTOMLファイル: `testdata/phase3_group_env.toml`
  ```toml
  [global]
  env = ["BASE_DIR=/opt"]
  env_allowlist = ["HOME", "PATH"]

  [[groups]]
  name = "inherit_group"
  # env_allowlist未定義 → 継承
  env = ["APP_DIR=${BASE_DIR}/app"]
  verify_files = ["${APP_DIR}/verify.sh"]

  [[groups]]
  name = "override_group"
  env_allowlist = ["USER"]  # 上書き
  env = ["DATA_DIR=/data"]
  ```
- [ ] 統合テスト: `internal/runner/config/loader_test.go`
  - [ ] Group.ExpandedEnvが正しく展開される
  - [ ] Allowlist継承が正しく動作する
  - [ ] Group.VerifyFilesでGroup.EnvとGlobal.Envを参照できる

#### 2.3.10 Phase 3の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 3の新規テストがすべてPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Implement Group.Env expansion with allowlist inheritance"

---

### Phase 4: Command.Env展開の拡張とCmd/Args統合
**目的**: Command.Envの展開でGlobal/Group.Envを参照できるようにし、Cmd/Args展開と統合

#### 2.4.1 Command.Env展開の拡張（テスト先行）
- [ ] テスト作成: `internal/runner/environment/processor_test.go`
  - [ ] `TestExpandCommandEnv_WithGlobalEnv`: Global.Envを参照
  - [ ] `TestExpandCommandEnv_WithGroupEnv`: Group.Envを参照
  - [ ] `TestExpandCommandEnv_WithBothGlobalAndGroup`: 両方を参照
  - [ ] `TestExpandCommandEnv_Priority`: Command.Env > Group.Env > Global.Env
  - [ ] `TestExpandCommandEnv_SelfReference`: 自己参照
- [ ] テスト実行（既存実装で一部PASSする可能性あり）
- [ ] コミット: "Add tests for Command.Env with Global/Group.Env reference (TDD)"

#### 2.4.2 Config LoaderでのCommand.Env展開呼び出し
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] 各コマンドに対してCommand.Env展開を実行
    ```go
    // Global.Env + Group.Env をマージしてbaseEnvを作成
    baseEnv := make(map[string]string)
    maps.Copy(baseEnv, cfg.Global.ExpandedEnv)
    maps.Copy(baseEnv, group.ExpandedEnv)

    // 既存のExpandCommandEnv()を呼び出す
    expandedEnv, err := expander.ExpandCommandEnv(
        cmd,
        group.Name,
        effectiveAllowlist,
        baseEnv,  // Global + Group
    )
    if err != nil {
        return err
    }
    cmd.ExpandedEnv = expandedEnv
    ```
  - [ ] エラーハンドリング

#### 2.4.3 Cmd/Args展開の統合
- [ ] `internal/runner/config/loader.go`を編集
  - [ ] Command.Env展開後、既存のCmd/Args展開処理を呼び出す
  - [ ] 既存処理が`Command.ExpandedEnv`と`baseEnv`をマージして使用していることを確認

#### 2.4.4 統合テスト
- [ ] サンプルTOMLファイル: `testdata/phase4_command_env.toml`
  ```toml
  [global]
  env = ["BASE_DIR=/opt"]

  [[groups]]
  name = "app_group"
  env = ["APP_DIR=${BASE_DIR}/myapp"]

  [[groups.commands]]
  name = "run_app"
  cmd = "${APP_DIR}/bin/server"
  args = ["--log", "${LOG_DIR}/app.log"]
  env = ["LOG_DIR=${APP_DIR}/logs"]
  ```
- [ ] 統合テスト: `internal/runner/config/loader_test.go`
  - [ ] Command.ExpandedEnvが正しく展開される
  - [ ] CmdでGroup.Envを参照できる
  - [ ] ArgsでCommand.Envを参照できる
  - [ ] 優先順位が正しい

#### 2.4.5 Phase 4の完了確認
- [ ] すべての既存テストがPASS
- [ ] Phase 4の新規テストがすべてPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Integrate Command.Env expansion with Global/Group.Env"

---

### Phase 5: エラーハンドリングとエッジケースの強化
**目的**: エラーメッセージの改善とエッジケースのテストを追加

#### 2.5.1 エラーメッセージの改善
**参照**: アーキテクチャドキュメント（[02_architecture.md](02_architecture.md)）セクション 5.3 のエラーメッセージ例に準拠

- [ ] `internal/runner/config/expansion.go`を編集
  - [ ] `ExpandGlobalEnv()`のエラーメッセージを詳細化
    - [ ] 変数名、コンテキスト（global.env）を含める
    - [ ] 重複定義エラー: 最初の定義と重複した定義の両方を表示
      ```
      Error: Duplicate environment variable definition 'PATH'
      Context: global.env
      First definition: PATH=/usr/local/bin
      Duplicate definition: PATH=/opt/bin
      ```
    - [ ] 未定義変数エラー: 利用可能な変数リストを表示
      ```
      Error: Undefined environment variable 'VAR_NAME'
      Context: global.env
      Available variables:
        - Global.Env: [VAR1, VAR2]
        - System (allowlist): [HOME, PATH, USER]
      ```
  - [ ] `ExpandGroupEnv()`のエラーメッセージを詳細化
    - [ ] 変数名、コンテキスト（group名）を含める
    - [ ] 未定義変数エラー: Global/Group/Systemレベルの利用可能な変数を表示
      ```
      Error: Undefined environment variable 'DB_HOST'
      Context: group.env:database
      Available variables:
        - Global.Env: [BASE_DIR, LOG_LEVEL]
        - Group.Env: [DB_PORT, DB_NAME]
        - System (allowlist): [HOME, PATH, USER]
      ```
    - [ ] Allowlist違反エラー: 有効なallowlistと参照箇所を表示
      ```
      Error: Environment variable 'SECRET_KEY' not in allowlist
      Effective allowlist: [HOME, PATH, USER]
      Referenced in: group.env:production
      ```
  - [ ] ラッピング時にコンテキスト情報を追加

#### 2.5.2 エッジケーステストの確認と追加
**注記**: `internal/runner/environment/processor_test.go`に既存のエッジケーステストあり：
- ✅ エスケープシーケンス: `TestVariableExpander_EscapeSequences`
- ✅ 複数の変数参照: `TestVariableExpander_ResolveVariableReferences` (line 205)
- ✅ 無効な変数形式: `TestVariableExpander_ResolveVariableReferences` (lines 249-291)
- ✅ 循環参照: `TestVariableExpander_ResolveVariableReferences_CircularReferences`

**追加が必要なテストケース**:
- [ ] 既存テストカバレッジを確認: `internal/runner/environment/processor_test.go`
- [ ] Global/Group.Env特有のエッジケーステストを`internal/runner/config/expansion_test.go`に追加（必要に応じて）:
  - [ ] `TestExpandGlobalEnv_EmptyValue`: Global.Envで空文字列の値（既存で未カバーの場合）
  - [ ] `TestExpandGlobalEnv_SpecialCharacters`: URL、パス、特殊文字を含む値（既存で未カバーの場合）
  - [ ] `TestExpandGroupEnv_LongValue`: 長い値（既存で未カバーの場合）
  - [ ] `TestExpandGlobalEnv_UnicodeCharacters`: Unicode文字を含む値（既存で未カバーの場合）
- [ ] 既存の`VariableExpander`がGlobal/Group.Envで正しく動作することを確認
- [ ] 必要に応じて実装を修正
- [ ] コミット: "Verify edge case coverage and add Global/Group-specific tests if needed"

#### 2.5.3 循環参照検出の確認
**注記**: `internal/runner/environment/processor_test.go`に既存の循環参照テストあり：
- ✅ 直接的自己参照: `TestVariableExpander_ResolveVariableReferences_CircularReferences` (line 329)
- ✅ 間接的循環参照: `TestVariableExpander_ResolveVariableReferences_CircularReferences` (lines 340, 351)
- ✅ 深いが循環しない参照チェーン: `TestVariableExpander_ResolveVariableReferences_CircularReferences` (line 362)

**確認タスク**:
- [ ] 既存の循環参照検出がGlobal.Env内で正しく動作することを確認
- [ ] 既存の循環参照検出がGroup.Env内で正しく動作することを確認
- [ ] 自己参照（`PATH=/custom:${PATH}`）がGlobal/Groupレベルで正しく動作することを確認
  - Global.Env内の`PATH`がシステム環境変数の`PATH`を参照する場合
  - Group.Env内の`PATH`がGlobal.ExpandedEnvの`PATH`を参照する場合
- [ ] 必要に応じてGlobal/Group特有のテストケースを追加
- [ ] コミット: "Verify circular reference detection for Global/Group.Env"

#### 2.5.4 Allowlist違反のテスト強化
- [ ] テスト作成: `internal/runner/config/allowlist_violation_test.go`
  - [ ] `TestAllowlistViolation_Global`: Global.Envでの違反
  - [ ] `TestAllowlistViolation_Group`: Group.Envでの違反
  - [ ] `TestAllowlistViolation_Command`: Command.Envでの違反
  - [ ] `TestAllowlistViolation_VerifyFiles`: VerifyFilesでの違反
  - [ ] `TestAllowlistViolation_EmptyAllowlist`: 空allowlistでの全拒否
- [ ] エラーメッセージが適切であることを確認
- [ ] コミット: "Add comprehensive allowlist violation tests"

#### 2.5.5 Phase 5の完了確認
- [ ] すべてのテストがPASS
- [ ] `make lint`でエラーなし
- [ ] コミット: "Complete error handling and edge case testing"

---

### Phase 6: 統合テストと互換性確認
**目的**: 全機能の統合テストと既存機能との互換性確認

#### 2.6.1 E2Eテストの作成
- [ ] サンプルTOMLファイル: `testdata/e2e_complete.toml`
  ```toml
  [global]
  env = [
      "BASE_DIR=/opt/app",
      "LOG_LEVEL=info",
      "PATH=/opt/tools/bin:${PATH}"
  ]
  env_allowlist = ["HOME", "USER", "PATH"]
  verify_files = ["${BASE_DIR}/verify.sh"]

  [[groups]]
  name = "database"
  env = [
      "DB_HOST=localhost",
      "DB_PORT=5432",
      "DB_DATA=${BASE_DIR}/db-data"
  ]
  verify_files = ["${DB_DATA}/schema.sql"]

  [[groups.commands]]
  name = "migrate"
  cmd = "${BASE_DIR}/bin/migrate"
  args = ["-h", "${DB_HOST}", "-p", "${DB_PORT}"]
  env = ["MIGRATION_DIR=${DB_DATA}/migrations"]

  [[groups]]
  name = "web"
  env_allowlist = ["PORT"]  # 上書き
  env = ["WEB_DIR=${BASE_DIR}/web"]

  [[groups.commands]]
  name = "start"
  cmd = "${WEB_DIR}/server"
  args = ["--port", "${PORT}"]
  ```
- [ ] E2Eテスト: `internal/runner/config/loader_e2e_test.go`
  - [ ] すべてのレベルで環境変数が正しく展開される
  - [ ] 優先順位が正しく適用される
  - [ ] Allowlist継承/上書きが正しく動作する
  - [ ] VerifyFilesで環境変数を参照できる
  - [ ] Cmd/Argsで環境変数を参照できる

#### 2.6.2 後方互換性テスト
- [ ] 既存のサンプルTOMLファイルをすべて実行
  - [ ] `sample/`ディレクトリ内のすべてのファイル
  - [ ] Global.Env/Group.Envが未定義でも動作すること
  - [ ] 既存の動作に変更がないこと
- [ ] 既存のテストスイート全体を実行
  - [ ] `make test`で全テストPASS
  - [ ] カバレッジレポートを確認

#### 2.6.3 パフォーマンステスト
- [ ] ベンチマークテスト: `internal/runner/config/expansion_bench_test.go`
  - [ ] `BenchmarkExpandGlobalEnv`: Global.Env展開の性能
  - [ ] `BenchmarkExpandGroupEnv`: Group.Env展開の性能
  - [ ] `BenchmarkExpandCommandEnv`: Command.Env展開の性能
  - [ ] `BenchmarkLoadConfigWithEnvs`: `global.env`と`group.env`を含む複雑な設定ファイルの読み込み性能を測定
- [ ] 性能要件の確認
  - [ ] 環境変数1個あたりの展開時間 < 1ms
  - [ ] 設定読み込み時間の増加 < 10%

#### 2.6.4 Phase 6の完了確認
- [ ] すべてのE2EテストがPASS
- [ ] すべての既存テストがPASS
- [ ] 性能要件を満たしている
- [ ] `make lint`でエラーなし
- [ ] コミット: "Add E2E tests and verify backward compatibility"

---

### Phase 7: ドキュメント更新とレビュー
**目的**: ドキュメントの更新と最終レビュー

#### 2.7.1 Task 0030ドキュメントの更新
- [ ] `docs/tasks/0030_verify_files_env/01_requirements.md`に注記を追加
  ```markdown
  注: Task 0031 (Global/Group Level Environment Variables) の実装により、
  verify_files の展開で Global.Env および Group.Env も参照可能になった。
  詳細は Task 0031 の要件定義書を参照。
  ```

#### 2.7.2 ユーザーガイドの更新
- [ ] `docs/user-guide/`に新機能のガイドを作成または更新
  - [ ] Global.Env/Group.Envの使用方法
  - [ ] 変数参照のパターンと優先順位
  - [ ] Allowlist継承/上書きの説明
  - [ ] 使用例とベストプラクティス

#### 2.7.3 設定ファイル仕様の更新
- [ ] `docs/config-spec/`でTOML仕様を更新
  - [ ] `global.env`フィールドの説明
  - [ ] `groups[].env`フィールドの説明
  - [ ] 変数展開のルール
  - [ ] エスケープシーケンス

#### 2.7.4 サンプルファイルの追加
- [ ] `sample/`ディレクトリにサンプルファイルを追加
  - [ ] `sample/global_group_env_basic.toml`: 基本的な使用例
  - [ ] `sample/global_group_env_advanced.toml`: 高度な使用例（allowlist継承/上書き）

#### 2.7.5 最終レビュー
- [ ] コード全体のレビュー
  - [ ] 命名規則の一貫性
  - [ ] エラーハンドリングの適切性
  - [ ] コメントの充実度
- [ ] テストカバレッジの確認
  - [ ] 単体テストカバレッジ > 95%
  - [ ] 統合テスト全パターンカバー
- [ ] ドキュメントの完全性確認

#### 2.7.6 Phase 7の完了確認
- [ ] すべてのドキュメントが更新されている
- [ ] サンプルファイルが動作する
- [ ] レビューが完了している
- [ ] コミット: "Update documentation for Global/Group env feature"

---

## 3. 完了基準

### 3.1 機能的完了基準
- [ ] Global環境変数の定義と展開が正常動作
- [ ] Group環境変数の定義と展開が正常動作
- [ ] 環境変数の優先順位（System < Global < Group < Command）が正しく動作
- [ ] 階層的な変数参照（Global→Group→Command）が正常動作
- [ ] Group間で環境変数が継承されないことを確認
- [ ] Allowlistとの統合が正常動作（継承・上書き・全拒否）
- [ ] 循環参照の検出が正常動作
- [ ] Global.VerifyFilesでGlobal.Envを参照可能
- [ ] Group.VerifyFilesでGlobal.EnvとGroup.Envを参照可能
- [ ] 既存機能（Command.Env、変数展開）との統合が正常動作

### 3.2 互換性完了基準
- [ ] すべての既存サンプルTOMLファイルが変更なしで動作
- [ ] 既存のテストケースがすべてPASS
- [ ] `global.env`と`group.env`が未定義の設定ファイルで動作変更なし

### 3.3 性能完了基準
- [ ] 環境変数展開処理時間が1ms/変数以下
- [ ] 設定ファイル読み込み時間の増加が10%以内
- [ ] メモリ使用量の増加が環境変数定義の2倍以内

### 3.4 品質完了基準
- [ ] 単体テストカバレッジ95%以上
- [ ] 統合テスト全パターンPASS
- [ ] エッジケーステスト（循環参照、未定義変数等）全PASS
- [ ] `make lint`でエラーなし
- [ ] `make test`で全テストPASS

### 3.5 ドキュメント完了基準
- [ ] ユーザーガイド更新完了
- [ ] 設定ファイル仕様更新完了
- [ ] Task 0030への注記追加完了
- [ ] サンプルファイル追加完了

---

## 4. リスク管理

### 4.1 技術リスク

**リスク**: 既存の環境変数展開処理との競合
- **影響度**: 中
- **対策**:
  - [ ] Phase 2-4で段階的に統合
  - [ ] 各Phaseで既存テストを実行
  - [ ] 問題発生時は即座にロールバック

**リスク**: 複雑な変数参照での性能劣化
- **影響度**: 低
- **対策**:
  - [ ] Phase 6でベンチマークテスト実施
  - [ ] 性能要件を満たさない場合は最適化

**リスク**: Allowlist継承ロジックのバグ
- **影響度**: 高（セキュリティ影響）
- **対策**:
  - [ ] Phase 3で徹底的にテスト
  - [ ] 3パターン（継承・上書き・全拒否）をすべてカバー

### 4.2 スケジュールリスク

**リスク**: テスト作成に予想以上の時間がかかる
- **影響度**: 中
- **対策**:
  - [ ] テストケースの優先順位付け
  - [ ] 重要なテストから先に実装

**リスク**: 既存テストの修正が必要になる
- **影響度**: 低
- **対策**:
  - [ ] 後方互換性を最優先
  - [ ] 既存テストの変更は最小限に

---

## 5. 実装チェックリスト

### 5.1 実装前の準備
- [ ] 要件定義書の理解
- [ ] アーキテクチャ設計書の理解
- [ ] 詳細仕様書の理解
- [ ] 既存コードの調査（VariableExpander, expansion.go, loader.go）

### 5.2 各Phaseの実装
- [ ] Phase 1: データ構造の拡張（完了）
- [ ] Phase 2: Global.Env展開（完了）
- [ ] Phase 3: Group.Env展開（完了）
- [ ] Phase 4: Command.Env拡張（完了）
- [ ] Phase 5: エラーハンドリング強化（完了）
- [ ] Phase 6: 統合テストと互換性確認（完了）
- [ ] Phase 7: ドキュメント更新（完了）

### 5.3 最終確認
- [ ] すべての完了基準を満たしている
- [ ] すべてのリスクが軽減されている
- [ ] ドキュメントが完全である
- [ ] コードレビューが完了している

---

## 6. まとめ

本実装計画書は、Global・Groupレベル環境変数設定機能を7つのPhaseに分けて段階的に実装するための詳細な手順を定義している。

**実装の要点**:
1. **TDD原則**: すべての機能でテスト先行開発
2. **段階的実装**: Phase 1から順次実装し、各Phaseで動作確認
3. **後方互換性**: 既存テストの継続的な実行で互換性を保証
4. **セキュリティ優先**: Allowlistと循環参照検出を徹底

各Phaseの完了時には必ずチェックボックスをマークし、進捗を可視化すること。
