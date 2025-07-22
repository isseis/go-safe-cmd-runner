# アーキテクチャ設計書: Allowlist機能の短期的改善

## 1. アーキテクチャ概要

### 1.1 現在のアーキテクチャ

```
┌─────────────────────────────────────────────────┐
│                Runner                           │
├─────────────────────────────────────────────────┤
│ resolveEnvironmentVars()                        │
│ ├── envFilter.ResolveGroupEnvironmentVars()     │
│ └── Command.Env 処理 + allowlistチェック        │ ← 問題箇所
└─────────────────────────────────────────────────┘
                          │
┌─────────────────────────────────────────────────┐
│            Environment Filter                   │
├─────────────────────────────────────────────────┤
│ ResolveGroupEnvironmentVars()                   │
│ ├── システム環境変数 + allowlistチェック        │
│ └── .envファイル変数 + allowlistチェック        │
│                                                 │
│ IsVariableAccessAllowed()                       │
│ └── isVariableAllowed() ← 問題の継承ロジック    │
└─────────────────────────────────────────────────┘
```

### 1.2 改善後のアーキテクチャ

```
┌─────────────────────────────────────────────────┐
│                Runner                           │
├─────────────────────────────────────────────────┤
│ resolveEnvironmentVars()                        │
│ ├── envFilter.ResolveGroupEnvironmentVars()     │
│ └── Command.Env 処理 (allowlistチェック除外)    │ ← 改善
└─────────────────────────────────────────────────┘
                          │
┌─────────────────────────────────────────────────┐
│            Environment Filter                   │
├─────────────────────────────────────────────────┤
│ ResolveGroupEnvironmentVars()                   │
│ ├── システム環境変数 + allowlistチェック        │
│ └── .envファイル変数 + allowlistチェック        │
│                                                 │
│ resolveAllowedVariable() ← 改善された継承ロジック│
│ ├── 明示的グループ設定 → グループのみ使用       │
│ ├── 明示的拒否 → グローバル無視                  │
│ └── 未定義 → グローバル継承                      │
│                                                 │
│ ValidateConfig() ← 新機能                       │
│ └── 設定検証ロジック                            │
└─────────────────────────────────────────────────┘
```

## 2. コンポーネント設計

### 2.1 Environment Filter の改善

#### 2.1.1 継承ロジックの明確化

**現在の問題**:
- `isVariableAllowed`関数でnil（未定義）と空スライス（明示的拒否）が区別されていない
- 継承パターンが不明確

**改善アプローチ**:
```go
// 新しい継承ロジック
func (f *Filter) resolveAllowedVariable(variable string, groupAllowlist []string, inheritanceMode InheritanceMode) bool {
    switch inheritanceMode {
    case InheritanceModeExplicit:    // ["VAR1", "VAR2"]
        return slices.Contains(groupAllowlist, variable)
    case InheritanceModeReject:      // []
        return false
    case InheritanceModeInherit:     // nil/undefined
        return f.globalAllowlist[variable]
    }
}
```

#### 2.1.2 継承モード定義

```go
type InheritanceMode int

const (
    InheritanceModeInherit InheritanceMode = iota  // nil - グローバル継承
    InheritanceModeExplicit                        // non-empty slice - 明示的設定
    InheritanceModeReject                          // empty slice - 明示的拒否
)
```

### 2.2 Runner の改善

#### 2.2.1 Command.Env 処理の分離

**現在の問題**:
```go
// Command.Env の変数もallowlistチェックされる
allowed := r.envFilter.IsVariableAccessAllowed(variable, group)
if !allowed {
    return nil, fmt.Errorf("failed to resolve variable %s: %w", variable, ErrVariableAccessDenied)
}
```

**改善アプローチ**:
```go
// Command.Env の変数はallowlistチェックを除外
func (r *Runner) processCommandEnvironment(cmd runnertypes.Command, envVars map[string]string, group *runnertypes.CommandGroup) error {
    for _, env := range cmd.Env {
        variable, value, ok := environment.ParseEnvVariable(env)
        if !ok {
            continue
        }

        // Command.Envはallowlistチェックを除外、基本検証のみ
        if err := r.envFilter.ValidateEnvironmentVariable(variable, value); err != nil {
            return fmt.Errorf("invalid command environment variable %s: %w", variable, err)
        }

        // 変数参照の解決（allowlistチェックは参照先で実行）
        resolvedValue, err := r.resolveVariableReferences(value, envVars, group)
        if err != nil {
            return fmt.Errorf("failed to resolve variable %s: %w", variable, err)
        }
        envVars[variable] = resolvedValue
    }
    return nil
}
```

### 2.3 設定検証機能の追加

#### 2.3.1 設定検証アーキテクチャ

```
┌─────────────────────────────────────────────────┐
│              Config Validator                   │
├─────────────────────────────────────────────────┤
│ ValidateConfig()                                │
│ ├── ValidateGlobalAllowlist()                   │
│ ├── ValidateGroupAllowlists()                   │
│ ├── ValidateAllowlistConsistency()              │
│ └── GenerateValidationReport()                  │
└─────────────────────────────────────────────────┘
```

#### 2.3.2 検証タイミング

1. **設定読み込み時**: 基本的な構文検証
2. **Runner初期化時**: 詳細な妥当性検証
3. **実行前**: 環境変数参照の事前検証（オプション）

## 3. データフロー設計

### 3.1 環境変数解決フロー（改善後）

```
1. システム環境変数
   ↓ allowlistチェック（グループ→グローバル継承）
2. .envファイル変数
   ↓ allowlistチェック（グループ→グローバル継承）
3. Command.Env変数
   ↓ 基本検証のみ（allowlistチェック除外）
4. 変数参照解決
   ↓ 参照先の変数ソースに応じてallowlistチェック適用
   │  ├─ システム環境変数参照 → allowlistチェック適用
   │  ├─ .envファイル変数参照 → allowlistチェック適用
   │  └─ Command.Env変数参照 → allowlistチェック除外
5. 最終環境変数マップ
```

### 3.2 継承ロジックフロー（改善後）

```
グループのenv_allowlistフィールド
├─ 存在しない（nil） → グローバル設定を継承
├─ 空配列（[]）     → 明示的拒否（すべて拒否）
└─ 値あり（["VAR"]） → グループ設定のみ使用
```

## 4. セキュリティアーキテクチャ

### 4.1 セキュリティ境界

```
┌─────────────────────────┐
│   信頼できるソース      │
│   - Command.Env         │ ← allowlistチェック除外
│   - 設定ファイル        │
└─────────────────────────┘
             │
┌─────────────────────────┐
│   部分的信頼ソース      │
│   - システム環境変数    │ ← allowlistチェック適用
│   - .envファイル        │ ← allowlistチェック適用
└─────────────────────────┘
```

### 4.2 検証レイヤー

1. **構文検証**: 変数名・値の形式チェック
2. **セキュリティ検証**: 危険パターンの検出
3. **アクセス制御**: allowlistベースの制御
4. **参照検証**: 循環参照・未定義参照の検出

## 5. 実装戦略

### 5.1 Phase 1: 継承ロジックの改善

**対象ファイル**:
- `internal/runner/environment/filter.go`

**変更内容**:
1. `InheritanceMode` 型の追加
2. `resolveAllowedVariable` 関数の実装
3. `isVariableAllowed` の置き換え
4. デバッグログの追加

**影響範囲**: 最小限（内部実装のみ）

### 5.2 Phase 2: Command.Env の除外

**対象ファイル**:
- `internal/runner/runner.go`

**変更内容**:
1. `processCommandEnvironment` 関数の追加
2. `resolveEnvironmentVars` の分割
3. allowlistチェックロジックの分離

**影響範囲**: 中程度（Runner の内部フロー変更）

### 5.3 Phase 3: 設定検証機能

**対象ファイル**:
- `internal/runner/config/validator.go` (新規)
- `internal/runner/environment/filter.go`

**変更内容**:
1. `ConfigValidator` 構造体の追加
2. 各種検証関数の実装
3. 検証レポート機能

**影響範囲**: 最小限（新機能追加のみ）

## 6. 後方互換性の保証

### 6.1 設定ファイル形式

- 既存の `env_allowlist` 形式を完全保持
- 新しい動作は既存設定に対して非破壊的
- デフォルト動作の維持

### 6.2 API 互換性

- パブリックAPIの変更なし
- 内部実装の改善のみ
- 既存テストの互換性維持

### 6.3 動作互換性

- 既存の環境変数解決動作を保持
- エラーメッセージの改善（破壊的でない）
- ログレベルの適切な設定

## 7. テストアーキテクチャ

### 7.1 テスト分類

```
Unit Tests
├── 継承ロジック単体テスト
├── 設定検証単体テスト
└── 環境変数処理単体テスト

Integration Tests
├── 全体的な環境変数解決テスト
├── Command.Env除外テスト
└── 設定検証統合テスト

Regression Tests
├── 既存動作の回帰テスト
└── パフォーマンス回帰テスト
```

### 7.2 テストカバレッジ目標

- Unit Tests: 95%以上
- Integration Tests: 主要フローの100%
- Edge Cases: 異常系の網羅

## 8. 監視・ログアーキテクチャ

### 8.1 ログレベル設計

```go
// DEBUG: 詳細な処理フロー
slog.Debug("Resolving variable", "variable", variable, "inheritance_mode", mode)

// INFO: 重要な状態変化
slog.Info("Applied group allowlist", "group", group.Name, "count", len(allowlist))

// WARN: 注意が必要な状況
slog.Warn("Variable access denied", "variable", variable, "reason", "not in allowlist")

// ERROR: エラー状況
slog.Error("Configuration validation failed", "error", err)
```

### 8.2 監視メトリクス

- 環境変数解決の成功/失敗数
- allowlistチェックの拒否数
- 設定検証エラー数
- パフォーマンスメトリクス

## 9. パフォーマンス考慮事項

### 9.1 最適化ポイント

1. **allowlistマップ**: O(1)検索の維持
2. **継承判定**: 最小限の条件分岐
3. **ログ出力**: 条件付きログの活用
4. **メモリ使用**: 不要なアロケーションの回避

### 9.2 パフォーマンス目標

- 環境変数解決時間: 現在と同等以下
- メモリ使用量: 10%以下の増加
- CPU使用量: 現在と同等

## 10. エラーハンドリングアーキテクチャ

### 10.1 エラー分類

```go
// 設定エラー
var (
    ErrInvalidAllowlistConfig = errors.New("invalid allowlist configuration")
    ErrAllowlistConsistency   = errors.New("allowlist consistency check failed")
)

// 実行時エラー
var (
    ErrCommandEnvValidation = errors.New("command environment validation failed")
    ErrInheritanceResolution = errors.New("inheritance resolution failed")
)
```

### 10.2 エラー回復戦略

1. **設定エラー**: 早期失敗（fail-fast）
2. **実行時エラー**: 詳細なコンテキスト情報付きエラー
3. **部分的失敗**: 可能な限り処理を継続

## 11. 次のステップ

このアーキテクチャ設計書に基づき、次の詳細設計書で以下を定義する：

1. **詳細なAPI設計**: 関数シグネチャ、データ構造
2. **実装詳細**: アルゴリズム、エラーハンドリング
3. **テスト仕様**: テストケース、テストデータ
4. **移行計画**: 段階的実装の詳細手順

## 12. リスク分析と対策

### 12.1 実装リスク

**高リスク**: 既存動作の破壊
- **対策**: 詳細な回帰テスト、段階的実装

**中リスク**: パフォーマンス劣化
- **対策**: ベンチマークテスト、プロファイリング

**低リスク**: 新機能の複雑さ
- **対策**: シンプルな設計、明確な文書化

### 12.2 運用リスク

**高リスク**: 設定ミス
- **対策**: 詳細な検証機能、分かりやすいエラーメッセージ

**中リスク**: デバッグの困難さ
- **対策**: 詳細なログ出力、診断機能の追加

## 13. 成功基準

### 13.1 技術的成功基準

- [ ] 既存テストがすべてパス
- [ ] 新機能のテストカバレッジ80%以上
- [ ] パフォーマンス劣化なし
- [ ] メモリリークなし

### 13.2 機能的成功基準

- [ ] 継承ロジックが明確化されている
- [ ] Command.Envがallowlistチェックから除外されている
- [ ] 設定検証が正常に動作している
- [ ] エラーメッセージが改善されている

### 13.3 非機能的成功基準

- [ ] コードの可読性向上
- [ ] 保守性の向上
- [ ] ドキュメントの完全性
- [ ] ユーザビリティの向上
