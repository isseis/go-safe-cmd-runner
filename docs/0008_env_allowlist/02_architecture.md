# アーキテクチャ設計書: 環境変数安全化機能の実装

## 1. 概要

本設計書では、go-safe-cmd-runnerにおける環境変数安全化機能の包括的実装アーキテクチャについて説明する。この機能により、明示的に許可された環境変数のみを使用することで、セキュリティリスクを大幅に軽減する。

### 1.1 設計原則

- **デフォルト拒否**: 明示的に許可されていない環境変数は一切使用しない
- **階層的フィルタリング**: global → groups → commands の順序で環境変数を継承・制御
- **明示的設定**: 設定ファイルですべての必要な環境変数を明確に定義
- **セキュリティファースト**: 環境変数の漏洩や汚染を防止
- **Breaking Change**: 後方互換性は考慮せず、セキュリティを最優先

## 2. システム全体アーキテクチャ

### 2.1 コンポーネント図

```
┌─────────────────────────────────────────┐
│              go-safe-cmd-runner         │
├─────────────────────────────────────────┤
│  main.go                                │
│  ├─ 1. Config Loading                   │
│  ├─ 2. Environment Setup               │  ← 変更
│  │  ├─ System Env Filtering           │  ← 新規
│  │  ├─ .env File Loading              │  ← 既存
│  │  └─ Environment Validation         │  ← 強化
│  ├─ 3. Groups Processing               │
│  │  └─ Group Environment Resolution   │  ← 変更
│  └─ 4. Command Execution               │
│     └─ Command Environment Resolution │  ← 変更
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│         Environment Management         │  ← 新規・拡張
├─────────────────────────────────────────┤
│  runner.Runner                          │
│  ├─ LoadEnvironment()                   │  ← 大幅変更
│  ├─ resolveEnvironmentVars()            │  ← 大幅変更
│  ├─ filterSystemEnvironment()           │  ← 新規
│  ├─ validateEnvironmentSecurity()       │  ← 新規
│  └─ logEnvironmentFiltering()           │  ← 新規
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│           設定管理コンポーネント          │  ← 拡張
├─────────────────────────────────────────┤
│  runnertypes.Config                     │
│  ├─ GlobalConfig.EnvAllowlist               │  ← 新規
│  └─ CommandGroup.EnvAllowlist               │  ← 新規
│                                         │
│  config.Loader                          │
│  ├─ LoadConfig()                        │  ← 既存
│  ├─ ValidateEnvAllowlistConfig()             │  ← 新規
│  └─ GetAllowedEnvironmentVars()         │  ← 新規
└─────────────────────────────────────────┘
```

### 2.2 データフロー

```
[起動] → [設定読み込み] → [環境変数フィルタリング] → [グループ処理] → [コマンド実行]
   │           │                      │                     │              │
   │           │                      │                     │              ├─ グループ1:
   │           │                      │                     │              │  ├─ グループ env_allowlist 適用
   │           │                      │                     │              │  ├─ コマンド1: env 解決・実行
   │           │                      │                     │              │  ├─ コマンド2: env 解決・実行
   │           │                      │                     │              │  └─ ...
   │           │                      │                     │              │
   │           │                      │                     ├─ グループ2:
   │           │                      │                     │  └─ ...
   │           │                      │                     │
   │           │                      ├─ システム環境変数
   │           │                      │  ├─ global.env_allowlist でフィルタ
   │           │                      │  └─ 許可されたもののみ取り込み
   │           │                      │
   │           │                      ├─ .env ファイル
   │           │                      │  └─ 無制限で取り込み（既存動作維持）
   │           │                      │
   │           │                      └─ 検証・ログ出力
   │           │
   │           ├─ global.env_allowlist 設定読み込み
   │           ├─ groups[].env_allowlist 設定読み込み
   │           └─ 設定検証
   │
   └─ env_allowlist 未定義時: 環境変数なしで実行
```

## 3. 詳細設計

### 3.1 設定ファイル拡張

#### 3.1.1 global セクション拡張

```toml
# config.toml
[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"

# 新規追加: 許可する環境変数
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "TERM",
    "TMPDIR"
]
```

#### 3.1.2 groups セクション拡張

```toml
[[groups]]
name = "web-server"
description = "Web server management"

# 新規追加: グループ固有の許可環境変数
# 注意: env_allowlistが定義されている場合、global.env_allowlistは無視される
env_allowlist = [
    "PATH",
    "HOME",
    "NODE_ENV",
    "PORT",
    "DATABASE_URL"
]

# 既存: コマンド定義（env 設定は既存通り）
[[groups.commands]]
name = "start_server"
cmd = "node"
args = ["server.js"]
env = ["DEBUG=app:*", "NODE_ENV=${NODE_ENV}"]  # ${NODE_ENV}はenv_allowlistで許可が必要

# 明示的拒否の例
[[groups]]
name = "secure_task"
env_allowlist = []  # 空リスト：環境変数を一切使用しない（globalも無視）

# グローバル継承の例
[[groups]]
name = "inherit_task"
# env_allowlist未定義：global.env_allowlistを継承
```

### 3.2 新規コンポーネント設計

#### 3.2.1 設定構造体拡張

```go
// internal/runner/runnertypes/config.go 拡張
type GlobalConfig struct {
    // 既存フィールド
    Timeout   int         `toml:"timeout"`
    WorkDir   string      `toml:"workdir"`
    LogLevel  string      `toml:"log_level"`

    // 新規追加
    EnvAllowlist   []string    `toml:"env_allowlist"`  // 許可する環境変数一覧
}

type CommandGroup struct {
    // 既存フィールド
    Name        string    `toml:"name"`
    Description string    `toml:"description"`
    Commands    []Command `toml:"commands"`

    // 新規追加
    EnvAllowlist     []string  `toml:"env_allowlist"`  // グループ固有の許可環境変数
}
```

#### 3.2.2 環境変数フィルタリング機能

```go
// internal/runner/runner.go 拡張
type Runner struct {
    config         *runnertypes.Config
    envVars        map[string]string    // フィルタリング済み環境変数
    templateEngine *template.Engine
    validator      *security.Validator

    // 新規追加
    allowedGlobalEnvAllowlist map[string]bool  // グローバル許可リスト（高速検索用）
}

// 新規メソッド
func (r *Runner) filterSystemEnvironment(allowedVars []string) map[string]string
func (r *Runner) validateEnvironmentSecurity(envVars map[string]string) error
func (r *Runner) logEnvironmentFiltering(allowed, filtered []string)
func (r *Runner) resolveGroupEnvironmentVars(group runnertypes.CommandGroup, baseEnv map[string]string) (map[string]string, error)

// 大幅変更メソッド
func (r *Runner) LoadEnvironment(envFile string, loadSystemEnv bool) error
func (r *Runner) resolveEnvironmentVars(cmd runnertypes.Command, groupEnv map[string]string) (map[string]string, error)
```

#### 3.2.3 環境変数検証とログ機能

```go
// internal/runner/environment/filter.go 新規パッケージ
package environment

type Filter struct {
    allowedVars    map[string]bool
    logger         *log.Logger
    securityLogger *log.Logger
}

func NewFilter(allowedVars []string) *Filter
func (f *Filter) FilterSystemEnvironment() map[string]string
func (f *Filter) ValidateVariableReference(varName string) error
func (f *Filter) LogFilteringResult(original, filtered int)

// 環境変数名の検証
func (f *Filter) ValidateVariableName(name string) error {
    // 英数字とアンダースコアのみ許可
    // 最大長制限
    // 予約名チェック
}

// 環境変数値の検証
func (f *Filter) ValidateVariableValue(value string) error {
    // 最大長制限
    // 危険な文字列パターンチェック
    // エンコーディング検証
}
```

### 3.3 処理フロー設計

#### 3.3.1 起動時環境変数処理フロー

```
1. 設定ファイル読み込み
   ├─ global.env_allowlist 取得
   └─ groups[].env_allowlist 取得

2. 設定検証
   ├─ env_allowlist が定義されている場合は妥当性検証
   ├─ 環境変数名の妥当性検証
   └─ 重複チェック

3. システム環境変数フィルタリング
   ├─ os.Environ() から全環境変数取得
   ├─ global.env_allowlist と照合（未定義時は空リスト扱い）
   ├─ 許可されたもののみ抽出
   └─ フィルタリング結果をログ出力

4. .env ファイル読み込み（既存機能）
   ├─ ファイル存在チェック
   ├─ 権限検証
   └─ 環境変数取り込み（上書き）

5. 最終環境変数マップ構築
   ├─ システム環境変数（フィルタ済み）
   ├─ .env ファイル変数（上書き）
   └─ セキュリティ検証
```

#### 3.3.2 グループ実行時環境変数処理フロー

```
グループ実行開始
├─ 1. ベース環境変数準備
│  ├─ グローバル環境変数（フィルタ済み）
│  └─ .env ファイル変数
│
├─ 2. グループ環境変数フィルタリング
│  ├─ groups[i].env_allowlist が定義されているかチェック
│  │  ├─ 定義あり: その設定のみ使用（globalは無視）
│  │  └─ 未定義: global.env_allowlist を継承
│  ├─ ベース環境変数からフィルタリング
│  └─ グループ専用環境変数マップ作成
│
└─ 3. コマンド実行ループ
   └─ 各コマンドで環境変数解決
      ├─ グループ環境変数（ベース）
      ├─ テンプレート env 設定適用
      ├─ コマンド固有 env 設定適用
      ├─ 変数参照解決（${VAR}）
      ├─ 許可チェック（参照される変数がenv_allowlistに含まれるか）
      └─ 実行環境変数マップ完成
```

### 3.4 エラーハンドリング戦略

#### 3.4.1 エラー分類と対応

```go
// 新規エラー定義
var (
    ErrInvalidVariableName   = errors.New("invalid environment variable name")
    ErrVariableNotAllowed    = errors.New("environment variable not in allowed list")
    ErrVariableValueTooLong  = errors.New("environment variable value too long")
    ErrCircularReference     = errors.New("circular reference in environment variable")
)

// エラー処理方針
1. env_allowlist未定義: 環境変数を一切引き継がない（正常動作）
2. 許可されていない変数参照: アプリケーション終了
3. 変数名・値の妥当性エラー: アプリケーション終了
4. 循環参照エラー: アプリケーション終了
```

#### 3.4.2 ログ出力仕様

```go
// セキュリティ監査ログ
log.Printf("SECURITY: Environment filtering applied - allowed: %d, filtered: %d", allowedCount, totalCount)
log.Printf("SECURITY: Denied environment variable reference: %s", varName)

// デバッグログ
log.Printf("DEBUG: Global allowed env vars: %v", globalEnvAllowlist)
log.Printf("DEBUG: Group '%s' allowed env vars: %v", groupName, groupEnvAllowlist)
log.Printf("DEBUG: Final environment for command '%s': %v", cmdName, sanitizedEnv)

// エラーログ
log.Printf("ERROR: Environment variable '%s' not allowed in group '%s'", varName, groupName)
```

## 4. セキュリティ考慮事項

### 4.1 攻撃ベクトルと対策

#### 4.1.1 環境変数汚染攻撃

**攻撃**: 悪意のある環境変数を設定してコマンド動作を変更
**対策**: env_allowlistで明示的に許可された変数のみ使用

#### 4.1.2 設定ファイル改ざん攻撃

**攻撃**: env_allowlist設定を改ざんして不正な環境変数を許可
**対策**: 既存のファイル改ざん検出機能でconfig.tomlを保護

#### 4.1.3 変数参照注入攻撃

**攻撃**: ${MALICIOUS_VAR}形式の参照で不正な変数を注入
**対策**: 参照される変数もenv_allowlistチェック対象とする

### 4.2 データ保護

#### 4.2.1 機密情報の取り扱い

- パスワードやAPIキーなどの機密情報は.envファイル経由で管理
- システム環境変数経由での機密情報取得は禁止
- ログ出力時の機密情報マスキング

#### 4.2.2 環境変数値の検証

- 最大長制限（デフォルト: 4096文字）
- 危険な文字列パターンの検出（改行、制御文字等）
- UTF-8エンコーディング検証

## 5. パフォーマンス設計

### 5.1 最適化戦略

#### 5.1.1 フィルタリング処理の最適化

```go
// O(1)検索のためのmap使用
type Runner struct {
    allowedGlobalEnvAllowlist map[string]bool  // 事前構築
    allowedGroupEnvAllowlist  map[string]map[string]bool  // グループ別事前構築
}

// 一度だけ構築、複数回利用
func (r *Runner) buildAllowedVarsMaps() {
    // 起動時に一度だけ実行
}
```

#### 5.1.2 メモリ使用量最適化

- 大量の環境変数がある場合の効率的な処理
- 不要な環境変数の早期解放
- 文字列の効率的なコピー処理

### 5.2 ベンチマーク目標

- 環境変数フィルタリング: < 10ms (1000変数時)
- 変数参照解決: < 5ms (100参照時)
- メモリオーバーヘッド: < 1MB 追加

## 6. 移行戦略

### 6.1 Breaking Change の管理

#### 6.1.1 段階的移行アプローチ

```
Phase 1: 検証モード
├─ env_allowlist設定の検証のみ実行
├─ 警告ログ出力
└─ 従来動作継続

Phase 2: 移行期間
├─ env_allowlist未定義時は警告ログ出力（環境変数なしで実行）
├─ 定義済み環境では新動作適用
└─ 移行ガイド提供

Phase 3: 完全移行
├─ env_allowlist未定義時は環境変数なしで実行
├─ セキュリティ強化完了
└─ ゼロトラスト環境変数モデル確立
```

#### 6.1.2 移行支援ツール

```bash
# 既存環境の環境変数使用状況分析ツール
go run tools/env-analyzer.go -config config.toml

# 出力例
# Recommended env_allowlist for global:
# env_allowlist = ["PATH", "HOME", "USER", "LANG"]
#
# Recommended env_allowlist for group 'web-server':
# env_allowlist = ["PATH", "HOME", "NODE_ENV", "PORT"]
```

## 7. テスト戦略

### 7.1 単体テスト

- 環境変数フィルタリング機能のテスト
- 変数参照解決のテスト
- エラーハンドリングのテスト
- セキュリティ検証のテスト

### 7.2 統合テスト

- 設定ファイル読み込みからコマンド実行までの一連フロー
- 複数グループでの環境変数継承
- エラー時の適切な終了処理

### 7.3 セキュリティテスト

- 不正な環境変数注入の検証
- 設定ファイル改ざん時の動作確認
- 権限昇格攻撃の防止確認
