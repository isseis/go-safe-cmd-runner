# 要件定義書: Global・Groupレベル環境変数設定機能

## 1. プロジェクト概要

### 1.1 プロジェクト名
Global・Groupレベル環境変数設定機能 (Global and Group Level Environment Variables)

### 1.2 プロジェクト目的
TOML設定ファイルのglobalおよびgroupレベルで環境変数を定義可能にし、階層的なスコープとオーバーライド機構を提供することで、設定の再利用性と保守性を向上させる。

### 1.3 背景と課題

**現在の状況**:
現在のgo-safe-cmd-runnerでは、環境変数の設定方法は以下の2通りに限定されている：
1. システム環境変数として runner 実行時に与える
2. TOML ファイルの command レベルで `env` フィールドに記述する

**課題**:
- **設定の重複**: 複数のコマンドで同じ環境変数を設定する場合、各コマンドで重複して記述する必要がある
- **保守性の低下**: 共通の環境変数を変更する際、すべてのコマンド定義を修正する必要がある
- **可読性の問題**: コマンド固有の設定と共通設定が混在し、設定ファイルが読みにくくなる
- **柔軟性の不足**: グループ単位での環境変数設定ができず、グループ内の全コマンドで共通の設定を共有できない

**既存機能**:
- Command.Env での環境変数展開機能（Task 0026で実装済み）
- 環境変数の allowlist 機能
- 環境変数の変数展開機能（`${VAR}` 形式のサポート）

## 2. 機能要件

### 2.1 基本機能

#### F001: Globalレベル環境変数設定
**概要**: TOML設定ファイルのglobalセクションで環境変数を定義する

**設定形式**:
```toml
[global]
env = ["VAR1=value1", "VAR2=value2"]
```

**スコープ**:
- すべてのグループとコマンドで参照可能
- システム環境変数を上書き

**例**:
```toml
[global]
env = ["BASE_DIR=/opt/app", "LOG_LEVEL=info"]

[[groups]]
name = "group1"
[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["${BASE_DIR}"]  # /opt/app が展開される
```

#### F002: Groupレベル環境変数設定
**概要**: CommandGroupで環境変数を定義する

**設定形式**:
```toml
[[groups]]
name = "database"
env = ["DB_HOST=localhost", "DB_PORT=5432"]

[[groups.commands]]
name = "connect"
cmd = "/usr/bin/psql"
args = ["-h", "${DB_HOST}", "-p", "${DB_PORT}"]
```

**スコープ**:
- そのグループ内のすべてのコマンドで参照可能
- globalレベルの環境変数を上書き

#### F003: Commandレベル環境変数設定（既存機能の維持）
**概要**: 既存のCommand.Envフィールドの動作を維持

**スコープ**:
- そのコマンド内でのみ有効
- groupレベルの環境変数を上書き

### 2.2 環境変数の優先順位

#### F004: スコープベースの優先順位
**優先順位（低 → 高）**:
1. システム環境変数
2. Global レベル (`global.env`)
3. Group レベル (`groups[].env`)
4. Command レベル (`groups[].commands[].env`)

**動作**:
- 下位レベルで定義された環境変数は、上位レベルの同名環境変数を上書きする
- 未定義の環境変数は上位レベルから継承される

**例**:
```toml
[global]
env = ["ENV=global", "COMMON=shared"]

[[groups]]
name = "group1"
env = ["ENV=group"]

[[groups.commands]]
name = "cmd1"
cmd = "/bin/printenv"
env = ["ENV=command"]

# 実行時の環境変数:
# ENV=command      (command レベルで上書き)
# COMMON=shared    (global レベルから継承)
```

### 2.3 環境変数の参照と展開

#### F005: 階層的な変数参照
**概要**: 各レベルで定義した環境変数を下位レベルで参照可能

**サポートする参照パターン**:
1. **Global → Group**: globalで定義した変数をgroupの`env`で参照
2. **Global → Command**: globalで定義した変数をcommandの`env`で参照
3. **Group → Command**: groupで定義した変数をcommandの`env`で参照
4. **同レベル内参照**: 同じレベル内での変数間参照

**変数参照の解決順序**:
`${VAR}` 形式の変数参照を解決する際、以下の順序で変数を探索する：

1. **Commandレベルでの展開時**:
   - Command.Env で定義された変数（最優先）
   - Group.Env で定義された変数
   - Global.Env で定義された変数
   - システム環境変数（最低優先）

2. **Groupレベルでの展開時**:
   - Group.Env で定義された変数（最優先）
   - Global.Env で定義された変数
   - システム環境変数（最低優先）
   - ※ Command.Env は参照不可（下位レベルは上位から参照できない）

3. **Globalレベルでの展開時**:
   - Global.Env で定義された変数（最優先）
   - システム環境変数（最低優先）
   - ※ Group.Env、Command.Env は参照不可（下位レベルは上位から参照できない）

**重要な制約**:
- 上位レベルから下位レベルの変数は参照できない（Global → Command は不可）
- 同じレベル内での変数参照は、定義順序に依存しない（展開アルゴリズムで解決）
- 同名変数が複数レベルに存在する場合、最も下位（優先度が高い）レベルの値を使用

**例1: Global → Group 参照**:
```toml
[global]
env = ["BASE=/opt", "APP_DIR=${BASE}/app"]

[[groups]]
name = "deploy"
env = ["DEPLOY_DIR=${APP_DIR}/deploy"]  # /opt/app/deploy に展開
# 解決順序: ${APP_DIR} → Group.Env(未定義) → Global.Env(/opt/app) ✓
```

**例2: Group → Command 参照**:
```toml
[[groups]]
name = "database"
env = ["DB_HOST=localhost"]

[[groups.commands]]
name = "backup"
env = ["BACKUP_FILE=/backup/${DB_HOST}.sql"]  # /backup/localhost.sql に展開
# 解決順序: ${DB_HOST} → Command.Env(未定義) → Group.Env(localhost) ✓
```

**例3: 複数レベルの参照と優先順位**:
```toml
[global]
env = ["ROOT=/data", "VAR=global"]

[[groups]]
name = "processing"
env = ["INPUT_DIR=${ROOT}/input", "VAR=group"]

[[groups.commands]]
name = "process"
env = ["OUTPUT_FILE=${INPUT_DIR}/result.txt", "VAR=command"]
# ${INPUT_DIR} の解決: Command.Env(未定義) → Group.Env(/data/input) ✓
# ${ROOT} の解決: Command.Env(未定義) → Group.Env(未定義) → Global.Env(/data) ✓
# VAR の最終値: command (Command.Envが最優先)
```

**例4: 同レベル内での相互参照**:
```toml
[global]
env = ["A=${B}", "B=value"]
# Global.Env内で展開: A=value, B=value
# 定義順序に関わらず正しく展開される（反復展開アルゴリズムによる）
```

#### F006: cmdとargsでの環境変数参照
**概要**: 既存機能（Task 0026）を活用し、すべてのレベルで定義された環境変数を`cmd`と`args`で参照可能

**例**:
```toml
[global]
env = ["TOOL_DIR=/usr/local/bin"]

[[groups]]
name = "tools"
env = ["PYTHON=${TOOL_DIR}/python3"]

[[groups.commands]]
name = "run_script"
cmd = "${PYTHON}"  # /usr/local/bin/python3 に展開
args = ["script.py"]
```

### 2.4 Group間の独立性

#### F007: Group間での環境変数非継承
**概要**: グループ間では環境変数を継承しない（dependencyによる依存関係とは無関係）

**動作**:
- GroupAの環境変数はGroupBに影響しない
- 各グループはglobalレベルからのみ環境変数を継承
- グループの依存関係（dependency）は実行順序のみを決定し、環境変数のスコープには影響しない

**例**:
```toml
[[groups]]
name = "groupA"
env = ["VAR_A=valueA"]

[[groups]]
name = "groupB"
dependency = ["groupA"]  # 実行順序のみに影響
env = ["VAR_B=valueB"]

[[groups.commands]]
name = "cmdB"
cmd = "/bin/echo"
args = ["${VAR_A}"]  # エラー: VAR_Aは未定義（VAR_Bのみ参照可能）
```

## 3. 非機能要件

### 3.1 互換性要件

#### C001: 後方互換性の完全維持
**概要**: 既存の設定ファイルが変更なしで動作継続

**要件**:
- `global.env` および `groups[].env` が未定義の場合、従来通り動作
- 既存のCommand.Env処理に変更なし
- 設定ファイル構造に破壊的変更なし
- 既存のallowlist機能との完全互換

**検証**:
- すべての既存サンプルTOMLファイルが変更なしで動作すること
- 既存のテストケースがすべてPASSすること

### 3.2 セキュリティ要件

#### S001: allowlistとの統合
**概要**: 既存のallowlist機能と完全統合

**allowlist継承ルール（既存仕様）**:
- グループが `env_allowlist` を定義していない（nil）: globalのallowlistを**継承**
- グループが `env_allowlist` を明示的に定義: そのリストのみを使用（globalは継承しない = **上書き**）
- グループが `env_allowlist = []` を定義: すべての環境変数を**拒否**

**各レベルでのallowlistチェック**:
1. **Global.Envの展開時**:
   - チェック対象: `global.env_allowlist`
   - 未定義の場合: チェックなし（すべての環境変数を許可）
   - システム環境変数参照時のみチェック（Global.Env内の変数定義自体はチェック対象外）

2. **Group.Envの展開時**:
   - チェック対象: グループの**有効なallowlist**
   - 有効なallowlistの決定:
     - `group.env_allowlist` が未定義（nil）→ `global.env_allowlist` を継承
     - `group.env_allowlist` が定義済み → そのリストを使用（globalは無視）
     - `group.env_allowlist = []` → 空リスト（すべて拒否）
   - システム環境変数参照時のみチェック（Group.Env内の変数定義自体はチェック対象外）

3. **Command.Envの展開時**:
   - チェック対象: そのコマンドが所属するグループの**有効なallowlist**
   - Group.Envと同じルールでallowlistを決定
   - システム環境変数参照時のみチェック（Command.Env内の変数定義自体はチェック対象外）

**重要な注意点**:
- allowlistは「Union（結合）」ではなく「Override（上書き）」方式
- グループが独自のallowlistを定義すると、globalのallowlistは完全に無視される
- この仕様は既存のTask 0011で確立されており、本タスクでも維持

**例1: allowlist継承（group.env_allowlist未定義）**:
```toml
[global]
env_allowlist = ["GLOBAL_VAR", "COMMON_VAR"]
env = ["GLOBAL_VAR=global_value"]  # OK: global.env_allowlistでチェック

[[groups]]
name = "inherit_group"
# env_allowlist未定義 → globalを継承
env = ["COMMON_VAR=${GLOBAL_VAR}"]  # OK: 両方とも有効なallowlistに含まれる

[[groups.commands]]
name = "cmd1"
env = ["CMD_VAR=${COMMON_VAR}"]  # OK: COMMON_VARは有効なallowlist（継承したglobal）に含まれる
```

**例2: allowlist上書き（group.env_allowlist定義済み）**:
```toml
[global]
env_allowlist = ["GLOBAL_VAR"]
env = ["GLOBAL_VAR=value"]  # OK: global.env_allowlistでチェック

[[groups]]
name = "override_group"
env_allowlist = ["GROUP_VAR"]  # 明示的定義 → globalは無視
env = ["GROUP_VAR=value"]      # OK: 有効なallowlist（GROUP_VARのみ）に含まれる
env = ["INVALID=${GLOBAL_VAR}"]  # エラー: GLOBAL_VARは有効なallowlistに含まれない

[[groups.commands]]
name = "cmd2"
env = ["CMD_VAR=${GROUP_VAR}"]   # OK: GROUP_VARは有効なallowlist（明示的定義）に含まれる
env = ["BAD=${GLOBAL_VAR}"]      # エラー: GLOBAL_VARは有効なallowlistに含まれない
```

**例3: allowlist完全拒否（group.env_allowlist = []）**:
```toml
[global]
env_allowlist = ["GLOBAL_VAR"]

[[groups]]
name = "reject_group"
env_allowlist = []  # すべて拒否
env = ["ANY_VAR=value"]  # エラー: システム環境変数を参照する場合は拒否される
                         # （定数値のみの定義は可能）
```

#### S002: 循環参照検出
**概要**: すべてのレベルで循環参照を検出

**検出対象**:
1. **同一レベル内**: `global.env`、`group.env`、`command.env` それぞれ内部での循環参照
2. **レベル間**: 異なるレベル間での循環参照（展開処理で自然に検出される）

**実装方式**: 既存の visited map 検出方式を継続使用

**システム環境変数の扱い**:
- システム環境変数の値は**リテラル文字列**として扱われる（再展開しない）
- システム環境変数 `B="${A}"` の場合、値は文字列 `"${A}"` として扱われる
- したがって、システム環境変数経由の循環参照は発生しない
- 例: `global.env = ["A=${B}"]` + システム環境変数 `B="${A}"` の場合
  - `${B}` はシステム環境変数から文字列 `"${A}"` を取得
  - これ以上展開されないため、結果は `A=${A}` （循環参照ではない）

**既存実装との整合性**:
- Task 0026 の `VariableExpander.resolveVariable()` はシステム環境変数の値を `os.LookupEnv()` で取得後、そのまま返す
- システム環境変数の値内の `${...}` は展開対象外
- この動作により、外部からの循環参照注入が防止され、セキュリティが保たれる

**例（エラーケース）**:
```toml
[global]
env = ["A=${B}", "B=${A}"]  # エラー: 循環参照（global.env内）

[[groups]]
env = ["X=${Y}", "Y=${X}"]  # エラー: 循環参照（group.env内）
```

**例（エラーにならないケース）**:
```bash
# システム環境変数
export SYS_VAR='${GLOBAL_VAR}'

# TOML設定
[global]
env = ["GLOBAL_VAR=${SYS_VAR}"]
# 結果: GLOBAL_VAR='${GLOBAL_VAR}' （リテラル文字列、循環参照ではない）
```

#### S003: 展開処理のセキュリティ
**概要**: 既存のセキュリティ制約を維持

**制約**:
- シェル実行は行わない
- グロブパターンは展開しない
- エスケープ機能は提供しない
- 展開後の値に対しても既存の検証を適用

### 3.3 性能要件

#### P001: 展開処理性能
**目標**:
- 環境変数1個あたりの展開時間: 1ms以下
- 設定ファイルの読み込み時間への影響: 10%以内
- メモリ使用量の増加: 環境変数定義の2倍以内

#### P002: スケーラビリティ
**制限**:
- Globalレベル環境変数: 最大100個
- Groupレベル環境変数: 最大100個/グループ
- Commandレベル環境変数: 最大100個/コマンド
- 変数展開のネスト深度: 最大10レベル

### 3.4 信頼性要件

#### R001: エラーハンドリング
**要件**:
- 環境変数定義のエラーは設定ファイル読み込み時に検出
- 循環参照、未定義変数、allowlist違反などで明確なエラーメッセージ
- エラー発生箇所（global/group/command、変数名）を明示

**エラーメッセージ例**:
```
Error: Undefined environment variable 'VAR1' referenced in global.env
Error: Circular reference detected in group 'database': DB_HOST -> DB_PORT -> DB_HOST
Error: Environment variable 'FORBIDDEN' not in allowlist (group: 'deploy')
```

#### R002: デバッグサポート
**要件**:
- 環境変数の展開過程をログ出力（デバッグレベル）
- 各レベルでの環境変数の最終的な値を確認可能
- 変数のオーバーライド履歴を追跡可能

### 3.5 保守性要件

#### M001: コード設計
**原則**:
- 既存の環境変数展開ロジック（Task 0026）を最大限再利用
- Global/Group/Commandの展開処理を統一的に扱う
- 設定読み込み時に一度だけ展開（実行時の再展開なし）

#### M002: テスト要件
**カバレッジ目標**:
- 単体テスト: 95%以上
- 統合テスト: 主要シナリオを100%カバー
- エッジケーステスト: 循環参照、未定義変数、allowlist違反など

## 4. 実装スコープ

### 4.1 スコープ内 (In Scope)

- [ ] `GlobalConfig` に `Env []string` フィールドを追加
- [ ] `CommandGroup` に `Env []string` フィールドを追加
- [ ] Global/Group/Commandの環境変数を階層的に展開する機能
- [ ] 環境変数の優先順位に基づくオーバーライド機能
- [ ] allowlistとの統合（各レベルでのチェック）
- [ ] 循環参照検出（すべてのレベル）
- [ ] 既存機能との統合テスト
- [ ] ドキュメント更新（設定ファイル仕様、ユーザーガイド）

### 4.2 スコープ外 (Out of Scope)

- 環境変数の削除・無効化機能（将来の拡張として検討）
- Group間での環境変数継承（dependency経由での継承）
- 環境変数の条件付き設定
- 環境変数のテンプレート機能
- 実行時の動的な環境変数変更

## 5. データ構造の変更

### 5.1 GlobalConfig の拡張

**変更前**:
```go
type GlobalConfig struct {
    Timeout           int
    WorkDir           string
    LogLevel          string
    VerifyFiles       []string
    SkipStandardPaths bool
    EnvAllowlist      []string
    MaxOutputSize     int64
    ExpandedVerifyFiles []string `toml:"-"`
}
```

**変更後**:
```go
type GlobalConfig struct {
    Timeout           int
    WorkDir           string
    LogLevel          string
    VerifyFiles       []string
    SkipStandardPaths bool
    EnvAllowlist      []string
    MaxOutputSize     int64
    Env               []string `toml:"env"` // 追加

    ExpandedVerifyFiles []string          `toml:"-"`
    ExpandedEnv         map[string]string `toml:"-"` // 追加
}
```

### 5.2 CommandGroup の拡張

**変更前**:
```go
type CommandGroup struct {
    Name        string
    Description string
    Priority    int
    TempDir     bool
    WorkDir     string
    Commands    []Command
    VerifyFiles []string
    EnvAllowlist []string
    ExpandedVerifyFiles []string `toml:"-"`
}
```

**変更後**:
```go
type CommandGroup struct {
    Name        string
    Description string
    Priority    int
    TempDir     bool
    WorkDir     string
    Commands    []Command
    VerifyFiles []string
    EnvAllowlist []string
    Env         []string `toml:"env"` // 追加

    ExpandedVerifyFiles []string          `toml:"-"`
    ExpandedEnv         map[string]string `toml:"-"` // 追加
}
```

### 5.3 Command 構造体（変更なし）

既存の `Command` 構造体は変更不要（`Env` と `ExpandedEnv` は既存）

## 6. 処理フロー

### 6.1 設定読み込み時の処理

```
1. TOMLファイルをパース
   ↓
2. Global環境変数の展開
   - global.env の展開（システム環境変数を参照可能）
   - allowlistチェック（global.env_allowlistに対して）
   - 循環参照検出
   ↓
3. 各Groupの環境変数展開
   - group.env の展開（global.ExpandedEnvを参照可能）
   - allowlistチェック（group有効allowlistに対して）
   - 循環参照検出
   ↓
4. 各Commandの環境変数展開
   - command.env の展開（global + group の ExpandedEnvを参照可能）
   - allowlistチェック
   - 循環参照検出
   ↓
5. Command.Cmd と Command.Args の展開
   - 既存機能（Task 0026）を使用
   - global + group + command の環境変数すべてを参照可能
   ↓
6. VerifyFiles パスの展開
   - 既存機能を使用
   - global + group + command の環境変数を参照可能
```

### 6.2 実行時の処理

```
1. 展開済みの ExpandedEnv を使用（再展開なし）
   ↓
2. 既存の実行ロジックを使用
   - ExpandedCmd
   - ExpandedArgs
   - ExpandedEnv (global + group + command のマージ済み)
```

## 7. 成功基準

### 7.1 機能的成功基準
- [ ] Global環境変数の定義と展開が正常動作
- [ ] Group環境変数の定義と展開が正常動作
- [ ] 環境変数の優先順位（System < Global < Group < Command）が正しく動作
- [ ] 階層的な変数参照（Global→Group→Command）が正常動作
- [ ] Group間で環境変数が継承されないことを確認
- [ ] allowlistとの統合が正常動作
- [ ] 循環参照の検出が正常動作
- [ ] 既存機能（Command.Env、変数展開）との統合が正常動作

### 7.2 互換性成功基準
- [ ] すべての既存サンプルTOMLファイルが変更なしで動作
- [ ] 既存のテストケースがすべてPASS
- [ ] `global.env` と `group.env` が未定義の設定ファイルで動作変更なし

### 7.3 性能成功基準
- [ ] 環境変数展開処理時間が1ms/変数以下
- [ ] 設定ファイル読み込み時間の増加が10%以内
- [ ] メモリ使用量の増加が環境変数定義の2倍以内

### 7.4 品質成功基準
- [ ] 単体テストカバレッジ95%以上
- [ ] 統合テスト全パターンPASS
- [ ] エッジケーステスト（循環参照、未定義変数等）全PASS

## 8. リスク分析

### 8.1 技術リスク

**リスク**: 既存の環境変数展開処理との競合
**影響**: 中
**対策**:
- 既存のexpansion.goを拡張して統一的に処理
- 段階的な実装とテスト
- 既存テストの継続実行

**リスク**: 複雑な変数参照での性能劣化
**影響**: 低
**対策**:
- 設定読み込み時に一度だけ展開（実行時の再展開なし）
- ベンチマークテストで性能を監視
- 必要に応じてキャッシング機構を導入

### 8.2 セキュリティリスク

**リスク**: allowlistのバイパス
**影響**: 高
**対策**:
- 各レベルで厳格なallowlistチェック
- セキュリティ監査テストの実施
- 既存のallowlist検証ロジックの再利用

**リスク**: 循環参照による無限ループ
**影響**: 中
**対策**:
- 既存の反復制限方式（最大15回）を使用
- すべてのレベルで循環参照検出
- エラー時の明確なメッセージ

### 8.3 互換性リスク

**リスク**: 既存設定ファイルの動作変更
**影響**: 高
**対策**:
- すべての既存サンプルファイルでの回帰テスト
- 新フィールドはオプショナル（未定義時は従来動作）
- 詳細な移行ガイドの提供

## 9. 関連資料

### 9.1 関連タスク
- Task 0026: コマンド・引数内環境変数展開機能（Command.Cmd/Args展開の基盤）
- Task 0008: env_allowlist機能（allowlist検証の基盤）
- Task 0030: verify_files内環境変数展開（VerifyFiles展開の基盤）

### 9.2 参考実装
- `internal/runner/config/expansion.go`: 既存の環境変数展開実装
- `internal/runner/environment/processor.go`: Command.Env処理
- `internal/runner/runnertypes/config.go`: 設定データ構造

### 9.3 参考ドキュメント
- `docs/dev/config-inheritance-behavior.ja.md`: allowlist継承動作の仕様
- TOML v1.0.0 仕様
- go-safe-cmd-runner セキュリティポリシー
