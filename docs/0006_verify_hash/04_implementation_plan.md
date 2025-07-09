# 実装計画書: ファイル改竄検出機能の実装

## 1. 概要

本実装計画書では、ファイル改竄検出機能の段階的実装について詳細なスケジュールとタスクを定義する。

## 2. 実装フェーズと優先度

### Phase 1: 警告機能実装（優先度: 高）
**目標**: 現状の制限を明確化し、将来の実装準備

### Phase 2: 設定ファイル検証実装（優先度: 高）
**目標**: 設定ファイルの改竄検出機能の完全実装

### Phase 3: 実行ファイル検証実装（優先度: 低、将来）
**目標**: 包括的なファイル整合性検証システム

## 3. Phase 1 実装計画

### 3.1 タスク一覧

- [x] **Task 1.1**: config/loader.go に警告ログ追加
  - **担当**: 開発者
  - **工数**: 0.5日
  - **依存**: なし
  - **成果物**: 警告ログ実装

- [x] **Task 1.2**: README.md セキュリティセクション更新
  - **担当**: 開発者
  - **工数**: 0.5日
  - **依存**: Task 1.1
  - **成果物**: 更新されたREADME.md

- [x] **Task 1.3**: --verify-config オプションのヘルプ追加
  - **担当**: 開発者
  - **工数**: 0.5日
  - **依存**: なし
  - **成果物**: 更新されたヘルプメッセージ

- [x] **Task 1.4**: 警告機能のテスト作成
  - **担当**: 開発者
  - **工数**: 1日
  - **依存**: Task 1.1
  - **成果物**: テストケース

- [x] **Task 1.5**: ドキュメント更新
  - **担当**: 開発者
  - **工数**: 0.5日
  - **依存**: Task 1.2, 1.3
  - **成果物**: 完全なドキュメント

### 3.2 実装詳細

#### Task 1.1: config/loader.go 警告ログ追加

```go
// internal/runner/config/loader.go
func (l *Loader) LoadConfig(path string) (*runnertypes.Config, error) {
    // 警告ログの追加
    slog.Warn("Configuration file integrity verification is not implemented",
        "phase", "1",
        "security_risk", "Configuration files may be tampered without detection",
        "recommendation", "Enable verification in production environments")

    // 既存の処理...
}
```

#### Task 1.2: README.md セキュリティセクション

```markdown
## セキュリティ制限事項

### ファイル改竄検出

**現在の状態**: 未実装

go-safe-cmd-runner は現在、以下のファイル改竄検出機能を提供していません：

1. **設定ファイルの改竄検出**
   - 設定ファイルが悪意を持って変更された場合の検出
   - 不正な設定変更による権限昇格の防止

2. **実行ファイルの改竄検出**
   - runner が呼び出すバイナリファイルの整合性検証
   - 悪意のあるバイナリへの置き換え検出

### 推奨される緩和策

1. 設定ファイルを root 所有に設定し、適切な権限を付与
2. 定期的な設定ファイルのバックアップとレビュー
3. システム監査ツールとの併用
```

#### Task 1.3: ヘルプメッセージ更新

```go
// cmd/runner/main.go または適切な場所
var verifyConfigCmd = &cobra.Command{
    Use:   "verify-config",
    Short: "Verify configuration file integrity (not implemented)",
    Long: `Verify the integrity of configuration files using cryptographic hashes.

WARNING: This feature is not yet implemented. Configuration files are currently
not protected against tampering. Use appropriate file permissions and monitoring
tools to mitigate this security risk.`,
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Println("ERROR: Configuration verification is not yet implemented")
        fmt.Println("Current implementation phase: 1 (warning only)")
        os.Exit(1)
    },
}
```

### 3.3 Phase 1 検証基準

- [x] 警告ログが適切に出力される
- [x] README.md にセキュリティ制限が明記される
- [x] --verify-config オプションが存在し、未実装メッセージを表示
- [x] 既存機能に影響がない
- [x] すべてのテストが通過する

## 4. 設定ファイル検証機能実装計画

### 4.1 タスク分解

- [x] **Task 2.1**: verification パッケージ基盤実装
  - **工数**: 3日
  - **依存**: Phase 1 完了

- [x] **Task 2.2**: マニフェスト管理機能実装
  - **工数**: 2日
  - **依存**: Task 2.1

- [x] **Task 2.3**: 設定ファイル検証機能実装
  - **工数**: 3日
  - **依存**: Task 2.2

- [x] **Task 2.4**: main.go 統合
  - **工数**: 1日
  - **依存**: Task 2.3

- [x] **Task 2.5**: エラーハンドリング実装
  - **工数**: 2日
  - **依存**: Task 2.4

- [x] **Task 2.6**: 設定ファイル拡張
  - **工数**: 1日
  - **依存**: Task 2.1

- [x] **Task 2.7**: ユニットテスト作成
  - **工数**: 4日
  - **依存**: Task 2.1-2.6

- [x] **Task 2.8**: 統合テスト作成
  - **工数**: 3日
  - **依存**: Task 2.7

- [x] **Task 2.9**: パフォーマンステスト
  - **工数**: 1日
  - **依存**: Task 2.8

- [x] **Task 2.10**: ドキュメント作成
  - **工数**: 2日
  - **依存**: Task 2.9

### 4.2 実装完了状況

#### 実装完了済み（簡素化版）

**✅ Task 2.1 - verification パッケージ基盤**

```go
// internal/verification/manager.go
type Manager struct {
    config    *Config
    fs        common.FileSystem
    validator *filevalidator.Validator
    security  *security.Validator
}

// 実装メソッド:
// - NewManager()
// - NewManagerWithFS()
// - IsEnabled()
// - VerifyConfigFile()
// - ValidateHashDirectory()
```

**✅ Task 2.2 - マニフェスト管理**

```go
// filevalidator パッケージの既存機能を活用
// 独自マニフェスト実装は削除
```

**✅ Task 2.3 - 設定ファイル検証**

```go
// 実装メソッド:
// - VerifyConfigFile() (filevalidator.Verify()を活用)
// - ValidateHashDirectory() (security.Validatorを活用)
```

**✅ Task 2.4 - main.go 統合**

```go
// cmd/runner/main.go
func main() {
    // 1. 設定ファイル読み込み
    cfg, err := cfgLoader.LoadConfig(*configPath)

    // 2. 検証マネージャー初期化
    verificationManager, err := verification.NewManager(&cfg.Verification)

    // 3. 設定ファイル検証
    if err := verificationManager.VerifyConfigFile(*configPath); err != nil {
        return fmt.Errorf("config verification failed: %w", err)
    }

    // 4. 既存処理継続
    // ...
}
```

**✅ Task 2.5 - エラーハンドリング**

```go
// 静的エラーとError構造体による統合エラーハンドリング
type Error struct {
    Op       string
    Path     string
    Expected string
    Actual   string
    Err      error
}
```

**✅ Task 2.6 - 設定ファイル拡張**

```toml
# config.toml 拡張
[verification]
enabled = false  # シンプルなon/off制御
hash_directory = "/etc/go-safe-cmd-runner/hashes"
```

**✅ Task 2.7 - ユニットテスト**

```go
// テストファイル構成:
// - manager_test.go
// - config_test.go
// - errors_test.go
```

**✅ Task 2.8 - 統合テスト**
**✅ Task 2.9 - パフォーマンステスト**
**✅ Task 2.10 - ドキュメント作成**

### 4.3 実装チェックリスト（完了）

#### 基盤機能

- [x] `Manager` struct 定義（簡素化版）
- [x] `Config` struct 定義と妥当性検証
- [x] ファイルシステム抽象化対応
- [x] エラー型定義と適切なメッセージ

#### 検証機能

- [x] ハッシュディレクトリ権限検証
- [x] 設定ファイルハッシュ検証（filevalidator活用）
- [x] ファイル権限検証（security package連携）

#### 統合

- [x] main.go への組み込み
- [x] 設定ファイル読み込み後の検証実行
- [x] enabled設定による動作制御
- [x] 適切なログ出力

#### テスト

- [x] 正常系テスト（検証成功）
- [x] 異常系テスト（ハッシュ不一致）
- [x] エラーハンドリングテスト
- [x] MockFileSystem使用
- [x] パフォーマンステスト

## 5. 将来の拡張計画

### 5.1 概要

将来的には、実行ファイルや関連ファイルの改竄検出機能を実装する。

### 5.2 予想される拡張点

- [ ] **実行ファイル検証**: コマンドバイナリのハッシュ検証
- [ ] **動的ライブラリ検証**: 依存ライブラリの整合性確認
- [ ] **参照ファイル検証**: 設定で指定された外部ファイル
- [ ] **キャッシュ機能**: ハッシュ計算結果のキャッシュ
- [ ] **増分検証**: 変更されたファイルのみの検証

## 6. リスク管理

### 6.1 技術リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| filevalidator パッケージの制約 | 中 | 事前調査と代替案検討 |
| パフォーマンス影響 | 中 | ベンチマークテストとプロファイリング |
| 既存機能への影響 | 高 | 段階的実装と十分なテスト |

### 6.2 運用リスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| ハッシュファイル管理の複雑化 | 中 | 詳細な運用ドキュメント作成 |
| 設定更新時の手順増加 | 中 | 自動化ツールの提供 |
| 権限設定ミス | 高 | 検証スクリプトとチェックリスト |

### 6.3 スケジュールリスク

| リスク | 影響度 | 対策 |
|--------|--------|------|
| テスト工数の過小評価 | 中 | バッファ期間の確保 |
| 統合時の予期しない問題 | 高 | 早期の統合テスト実施 |

## 7. 品質保証

### 7.1 コードレビュー

- **Phase 1**: 各タスク完了時に実施
- **Phase 2**: 週次レビューと最終レビュー
- **レビュー観点**: セキュリティ、パフォーマンス、保守性

### 7.2 テスト戦略

```
ユニットテスト
├─ 各パッケージ: 90%以上のカバレッジ
├─ 異常系テスト: 全エラーパターン
└─ MockFileSystem: 実ファイル操作なし

統合テスト
├─ エンドツーエンド: 実際のファイルを使用
├─ 権限テスト: 実際の権限設定
└─ パフォーマンス: 起動時間測定

セキュリティテスト
├─ 権限バイパステスト
├─ パス正規化テスト
└─ タイムオブチェック問題テスト
```

### 7.3 継続的インテグレーション

```yaml
# .github/workflows/test.yml 拡張例
- name: Run verification tests
  run: |
    go test -v ./internal/verification/...
    go test -v -race ./internal/verification/...
    go test -v -bench=. ./internal/verification/...
```

## 8. 成果物

### 8.1 Phase 1 成果物

- [x] 更新された config/loader.go
- [x] 更新された README.md
- [x] 更新されたヘルプメッセージ
- [x] Phase 1 テストケース
- [x] 警告機能ドキュメント

### 8.2 Phase 2 成果物

- [ ] verification パッケージ完全実装
- [ ] 統合された main.go
- [ ] 拡張された設定ファイル形式
- [ ] 包括的なテストスイート
- [ ] 運用ドキュメント
- [ ] パフォーマンス測定結果

## 9. 移行計画

### 9.1 既存システムへの影響

- **Phase 1**: 影響なし（警告のみ）
- **Phase 2**: 新規インストールのみ（既存は手動有効化）

### 9.2 段階的ロールアウト

```
1. 開発環境でのテスト
2. ステージング環境での検証
3. パイロット本番環境での限定運用
4. 全本番環境への展開
```

### 9.3 ロールバック計画

- Phase 1: 単純なコード変更のため、git revert で対応
- Phase 2: 設定ファイルで無効化可能な設計

## 10. 成功基準

### 10.1 Phase 1 成功基準

- [x] 警告メッセージが適切に表示される
- [x] README.md にセキュリティ制限が明記される
- [x] 既存機能に影響がない
- [x] 全テストが通過する

### 10.2 Phase 2 成功基準

- [ ] 設定ファイル改竄の検出と適切な終了
- [ ] ハッシュファイル権限の適切な検証
- [ ] 95%以上のテストカバレッジ
- [ ] 起動時間増加 < 100ms
- [ ] 包括的な運用ドキュメント

## 11. 完了後のアクション

### 11.1 モニタリング

- 検証機能の使用状況監視
- エラー発生率の追跡
- パフォーマンス影響の継続測定

### 11.2 保守計画

- 四半期ごとの機能レビュー
- セキュリティアップデートの適用
- ユーザーフィードバックの収集と対応

### 11.3 将来拡張への準備

- Phase 3 要件の詳細化
- 新しいハッシュアルゴリズムへの対応検討
- 分散環境での検証機能検討
