# 実装計画書：タイムアウト設定仕様の明確化

## 1. 実装概要

### 1.1. 実装目標
- TOML設定における「未設定」と「明示的なゼロ」の区別を可能にする
- `timeout = 0` を無制限実行として明確に定義する
- 既存機能の後方互換性を維持しつつ、新機能を安全に導入する

### 1.2. 実装戦略
1. **段階的実装**: 型変更 → 解決ロジック → 実行制御 → テスト
2. **破壊的変更管理**: バージョン番号を明確にし、移行ガイドを提供
3. **リスク最小化**: 各段階でテストを実行し、問題を早期発見

## 2. 実装フェーズ計画

### Phase 1: 基盤構造変更 (1-2日)
- データ型定義の変更
- 基本的な型変換ロジック実装
- 基本バリデーション実装

### Phase 2: 解決ロジック実装 (1-2日)
- タイムアウト階層継承ロジック実装
- RuntimeGlobal, RuntimeCommand の更新
- タイムアウト解決アルゴリズム実装

### Phase 3: 実行制御実装 (2-3日)
- 無制限タイムアウト実行機能実装
- 監視・ログ機能実装
- セキュリティ機能強化

### Phase 4: テスト実装 (2-3日)
- 単体テスト実装
- 統合テスト実装
- E2Eテスト実装

### Phase 5: ドキュメント・最終化 (1-2日)
- ドキュメント更新
- サンプル設定更新
- 移行ガイド作成

## 3. 詳細実装計画

### 3.1. Phase 1: 基盤構造変更

#### 3.1.1. ファイル変更計画

**target file**: `internal/common/types.go` (新規作成)
```go
// TimeoutValue represents a timeout setting that can be unset, zero, or positive
type TimeoutValue *int

const (
    // DefaultTimeout is used when no timeout is explicitly set
    DefaultTimeout = 60 // seconds

    // MaxTimeout defines the maximum allowed timeout value (24 hours)
    MaxTimeout = 86400 // 24 hours in seconds
)

// Helper functions
func IntPtr(v int) *int { return &v }
func TimeoutPtr(v int) TimeoutValue { return &v }
```

**target file**: `internal/common/config.go` (修正)
- `GlobalSpec.Timeout` を `*int` に変更
- `CommandSpec.Timeout` を `*int` に変更

**target file**: `internal/common/validation.go` (新規作成)
```go
// ValidateTimeout validates timeout configuration
func ValidateTimeout(timeout *int, context string) error
// parseTimeoutValue converts TOML value to *int
func parseTimeoutValue(value interface{}) (*int, error)
```

#### 3.1.2. 実装チェックポイント
- [ ] 型定義が正しく作成されている
- [ ] 構造体フィールドが正しく更新されている
- [ ] バリデーション関数が動作する
- [ ] 既存のビルドが通る

### 3.2. Phase 2: 解決ロジック実装

#### 3.2.1. ファイル変更計画

**target file**: `internal/common/timeout_resolver.go` (新規作成)
```go
// ResolveTimeout resolves the effective timeout value from the hierarchy
func ResolveTimeout(cmdTimeout, groupTimeout, globalTimeout *int) int

// TimeoutResolutionContext provides context for timeout resolution
type TimeoutResolutionContext struct {
    CommandName string
    GroupName   string
    Level      string // "command", "group", "global", "default"
}
```

**target file**: `internal/common/runtime.go` (修正)
- `RuntimeGlobal.Timeout()` メソッド更新
- `RuntimeCommand` 構造体に `EffectiveTimeout` フィールド追加
- `NewRuntimeCommand` 関数でタイムアウト解決実装

#### 3.2.2. 実装チェックポイント
- [ ] タイムアウト解決アルゴリズムが正しく動作する
- [ ] 階層継承ロジックが仕様通りに実装されている
- [ ] RuntimeCommand にタイムアウト値が正しく設定される

### 3.3. Phase 3: 実行制御実装

#### 3.3.1. ファイル変更計画

**target file**: `internal/runner/executor.go` (修正)
```go
// ExecuteWithTimeout executes a command with the specified timeout
func ExecuteWithTimeout(cmd *exec.Cmd, timeout int) error

// MonitorUnlimitedExecution monitors commands running without timeout
func MonitorUnlimitedExecution(cmd *exec.Cmd, cmdName string) context.CancelFunc
```

**target file**: `internal/runner/monitor.go` (新規作成)
```go
// UnlimitedExecutionMonitor tracks commands running without timeout
type UnlimitedExecutionMonitor struct {
    processes map[int]*ProcessInfo
    mutex     sync.RWMutex
}

type ProcessInfo struct {
    CommandName string
    StartTime   time.Time
    PID         int
}
```

**target file**: `internal/logging/security.go` (修正)
```go
// SecurityLogger logs security-relevant timeout events
type SecurityLogger struct {
    logger *log.Logger
}

func (s *SecurityLogger) LogUnlimitedExecution(cmdName string, user string)
func (s *SecurityLogger) LogLongRunningProcess(cmdName string, duration time.Duration, pid int)
```

#### 3.3.2. 実装チェックポイント
- [ ] 無制限タイムアウト実行が正しく動作する
- [ ] 長時間実行プロセスの監視が機能する
- [ ] セキュリティログが適切に出力される
- [ ] リソース使用量が適切に管理されている

### 3.4. Phase 4: テスト実装

#### 3.4.1. テストファイル構成

**target file**: `internal/common/timeout_test.go` (新規作成)
- 型変換テスト
- バリデーションテスト
- タイムアウト解決テスト

**target file**: `internal/runner/executor_test.go` (修正)
- 無制限実行テスト
- タイムアウト制御テスト

**target file**: `cmd/runner/integration_timeout_test.go` (新規作成)
- E2Eタイムアウト動作テスト
- 設定ファイル読み込みテスト

**target file**: `test/security/timeout_security_test.go` (新規作成)
- セキュリティ監視テスト
- 長時間実行検出テスト

#### 3.4.2. テストケース詳細

```go
// 単体テスト例
func TestTimeoutParsing(t *testing.T) {
    tests := []struct {
        name     string
        toml     string
        expected *int
        wantErr  bool
    }{
        {"unset timeout", "", nil, false},
        {"zero timeout", "timeout = 0", intPtr(0), false},
        {"positive timeout", "timeout = 300", intPtr(300), false},
        {"negative timeout", "timeout = -1", nil, true},
        {"overflow timeout", "timeout = 90000", nil, true},
    }
    // テスト実装...
}

// E2Eテスト例
func TestE2ETimeoutBehavior(t *testing.T) {
    tests := []struct {
        name           string
        config         string
        command        string
        expectTimeout  bool
        expectDuration time.Duration
    }{
        {
            name: "default timeout",
            config: `
                version = "1.0"
                [[groups]]
                name = "test"
                [[groups.commands]]
                name = "sleep"
                cmd = "/bin/sleep"
                args = ["90"]
            `,
            command:        "sleep",
            expectTimeout:  true,
            expectDuration: 60 * time.Second,
        },
        // 他のテストケース...
    }
    // テスト実装...
}
```

#### 3.4.3. テスト実装チェックポイント
- [ ] 単体テストカバレッジ95%以上
- [ ] 統合テストが全てパス
- [ ] E2Eテストで実際の動作確認
- [ ] 性能回帰テストでオーバーヘッド確認
- [ ] セキュリティテストで監視機能確認

### 3.5. Phase 5: ドキュメント・最終化

#### 3.5.1. ドキュメント更新計画

**target file**: `docs/user/04_global_level.md`
- タイムアウト設定の説明更新
- 破壊的変更の警告追加
- 新しい動作の詳細説明

**target file**: `docs/user/06_command_level.md`
- コマンドレベルタイムアウトの説明更新
- 継承ロジックの説明追加

**target file**: `docs/migration/v2.0.0_timeout_changes.md` (新規作成)
- 破壊的変更の詳細説明
- 移行手順の明確化
- Before/Afterの例示

**target file**: `sample/timeout_examples.toml` (新規作成)
- 各種タイムアウト設定のサンプル
- ベストプラクティスの例示

#### 3.5.2. サンプル設定更新

既存のサンプルファイルを確認し、新しいタイムアウト仕様に合わせて更新：
- `sample/comprehensive.toml`
- `sample/starter.toml`
- その他必要に応じて

#### 3.5.3. CHANGELOG更新

**target file**: `CHANGELOG.md`
```markdown
## [2.0.0] - 2025-10-XX

### Breaking Changes
- **BREAKING**: `timeout = 0` now means unlimited execution (previously defaulted to 60 seconds)
- **BREAKING**: Unset timeout still defaults to 60 seconds

### Added
- Support for unlimited command execution with `timeout = 0`
- Enhanced timeout hierarchy resolution
- Security monitoring for unlimited execution commands
- Long-running process detection and logging

### Changed
- Timeout configuration now uses nullable integers for better control
- Improved error messages for timeout configuration errors

### Migration Guide
See `docs/migration/v2.0.0_timeout_changes.md` for detailed migration instructions.
```

## 4. リスク管理・品質保証

### 4.1. 破壊的変更管理

#### 4.1.1. バージョニング戦略
- セマンティックバージョニング採用
- メジャーバージョンアップ（v2.0.0）
- 十分な告知期間設定

#### 4.1.2. 移行支援策
- 詳細な移行ガイド提供
- 設定ファイル検証ツール提供
- サンプル設定の充実

### 4.2. 性能影響評価

#### 4.2.1. ベンチマーク実施
- 設定読み込み性能測定
- タイムアウト解決性能測定
- メモリ使用量測定

#### 4.2.2. 性能要件
- 設定読み込み: 5%以内の性能劣化
- タイムアウト解決: 1マイクロ秒以内
- メモリ増加: 構造体あたり8バイト（許容範囲内）

### 4.3. セキュリティ考慮事項

#### 4.3.1. 無制限実行の監視
- 長時間実行プロセスの自動検出
- セキュリティログの出力
- リソース使用量の監視

#### 4.3.2. DoS攻撃対策
- 同時無制限実行数の制限検討
- プロセス監視による異常検出
- 管理者への通知機能

## 5. 実装スケジュール

### 5.1. 週次スケジュール

#### Week 1 (Oct 23-29, 2025)
- **Day 1-2**: Phase 1 - 基盤構造変更
- **Day 3-4**: Phase 2 - 解決ロジック実装
- **Day 5**: 中間レビュー・調整

#### Week 2 (Oct 30 - Nov 5, 2025)
- **Day 1-3**: Phase 3 - 実行制御実装
- **Day 4-5**: Phase 4開始 - 単体テスト実装

#### Week 3 (Nov 6-12, 2025)
- **Day 1-2**: Phase 4継続 - 統合・E2Eテスト
- **Day 3-4**: Phase 5 - ドキュメント更新
- **Day 5**: 最終レビュー・リリース準備

### 5.2. マイルストーン

| マイルストーン | 完了日 | 成果物 |
|----------------|---------|---------|
| M1: 基盤構造完了 | Oct 25 | 型定義、基本バリデーション |
| M2: 解決ロジック完了 | Oct 27 | タイムアウト解決機能 |
| M3: 実行制御完了 | Oct 31 | 無制限実行、監視機能 |
| M4: テスト完了 | Nov 8 | 全テスト実装・検証 |
| M5: リリース準備完了 | Nov 12 | ドキュメント、移行ガイド |

## 6. 品質ゲート

### 6.1. 各フェーズの完了基準

#### Phase 1 完了基準
- [ ] 全ての既存テストがパス
- [ ] 新しい型定義がコンパイルエラーなく動作
- [ ] バリデーション機能が正常動作

#### Phase 2 完了基準
- [ ] タイムアウト解決ロジックが仕様通り動作
- [ ] RuntimeCommand にタイムアウト値が正しく設定
- [ ] 階層継承テストが全てパス

#### Phase 3 完了基準
- [ ] 無制限実行が正常動作
- [ ] 監視機能が適切に動作
- [ ] セキュリティログが正しく出力

#### Phase 4 完了基準
- [ ] 単体テストカバレッジ95%以上
- [ ] 全ての統合テストがパス
- [ ] E2Eテストで実動作確認完了
- [ ] 性能要件達成確認

#### Phase 5 完了基準
- [ ] 全ドキュメント更新完了
- [ ] 移行ガイド作成完了
- [ ] サンプル設定更新完了
- [ ] CHANGELOGエントリ作成完了

### 6.2. 最終リリース基準

- [ ] 全テスト 100% パス
- [ ] 性能回帰なし（5%以内）
- [ ] セキュリティレビュー完了
- [ ] ドキュメントレビュー完了
- [ ] 移行ガイド検証完了

## 7. チーム体制・責任分担

### 7.1. 実装責任者
- **主実装者**: コア機能実装、アーキテクチャ設計
- **テスト責任者**: テスト戦略、テスト実装
- **ドキュメント責任者**: ユーザードキュメント、移行ガイド

### 7.2. レビュープロセス
- **コードレビュー**: 全PRに対して実施
- **設計レビュー**: 各フェーズ完了時に実施
- **セキュリティレビュー**: Phase 3完了時に実施
- **最終レビュー**: リリース前の統合レビュー

## 8. リスク対応計画

### 8.1. 技術リスク

#### 高リスク: TOML パーサーの型変換問題
- **影響**: 設定読み込み失敗
- **対策**: 十分なテストケース作成、型変換ロジックの慎重実装
- **コンティンジェンシー**: パーサー部分の分離実装

#### 中リスク: 無制限実行のリソース問題
- **影響**: システムリソース枯渇
- **対策**: 監視機能の強化、適切な警告実装
- **コンティンジェンシー**: 緊急停止機能の実装

#### 低リスク: 性能劣化
- **影響**: 実行速度低下
- **対策**: 継続的な性能測定、最適化実装
- **コンティンジェンシー**: 性能クリティカル部分の最適化

### 8.2. スケジュールリスク

#### テスト実装遅延リスク
- **対策**: 早期のテスト計画策定、並行実装
- **コンティンジェンシー**: 最小限テストでの限定リリース

#### ドキュメント作成遅延リスク
- **対策**: 実装と並行したドキュメント作成
- **コンティンジェンシー**: コミュニティによる追加ドキュメント作成

## 9. 成功指標

### 9.1. 技術指標
- [ ] 既存機能の100%後方互換性維持
- [ ] 新機能の95%以上テストカバレッジ
- [ ] 5%以内の性能劣化
- [ ] ゼロクリティカルバグ

### 9.2. ユーザビリティ指標
- [ ] 明確な移行ガイド提供
- [ ] 充実したサンプル設定提供
- [ ] わかりやすいエラーメッセージ
- [ ] 包括的なドキュメント更新

### 9.3. セキュリティ指標
- [ ] 無制限実行の適切な監視
- [ ] セキュリティイベントの適切なログ出力
- [ ] リソース使用量の適切な制御

## 10. 今後の展開

### 10.1. 将来機能拡張
- グループレベルタイムアウト設定
- 動的タイムアウト調整機能
- タイムアウト統計・分析機能

### 10.2. 保守計画
- 定期的な性能監視
- セキュリティ機能の継続改善
- ユーザーフィードバックの収集・反映

---

このドキュメントは実装開始前の最終確認として、関係者全員でレビューを実施し、必要に応じて調整を行う。実装中も定期的にこの計画を参照し、進捗管理と品質確保に努める。
