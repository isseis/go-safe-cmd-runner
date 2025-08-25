# 要件定義書: セキュリティ検証メカニズムの統一

## 1. 背景・課題

### 1.1 現在の状況
- **PathResolver.ValidateCommand**: ホワイトリストベースのコマンド許可検証（`internal/verification/path_resolver.go`）
- **リスクレベル評価システム**: リスクベースアクセス制御（`internal/runner/security/command_analysis.go`）
- **二重検証メカニズム*### 7.1 Phase 1: 基盤整備 (3週間)
- 統合リスクベース検証エンジンの設計・実装
- PathResolver への統合
- ハードコーディングされたリスクレベル計算ルールの実装
- 基本テストケースの作成るモジュールで独立したセキュリティ検証を実行

### 1.2 特定された問題
- **機能重複**:
  - PathResolver: パスベースのホワイトリスト検証（ハードコーディング）
  - Risk Evaluator: パターンマッチングベースのリスク分析
  - 両方とも本質的にはコマンドの安全性を検証
- **アーキテクチャの複雑性**:
  - verification module と runner module で異なるセキュリティ基準
  - セキュリティロジックの分散（ハードコードされたパス許可 + リスクレベル評価）
  - 設定変更不可能なホワイトリスト
- **制御の非一貫性**:
  - verification フェーズ: パスベースホワイトリスト（`/bin/*`, `/usr/bin/*` 等）
  - execution フェーズ: リスクレベルベース（段階的制御）

### 1.3 セキュリティ・運用上の課題
- **セキュリティ基準の不統一**: 異なるフェーズで異なる判定基準
- **設定複雑性**: 管理者が2つの異なる検証システムを理解・設定する必要
- **監査の困難性**: セキュリティ判定ロジックが複数箇所に分散
- **機能拡張の困難性**: 新しいセキュリティ要件を2箇所で実装する必要

## 2. 要件概要

### 2.1 基本要件
**R1**: PathResolver.ValidateCommand とリスクレベル評価システムを統一し、単一のリスクベースセキュリティ検証メカニズムに集約する

**R2**: 現状のハードコーディングされたホワイトリストルール（`/bin/*`, `/usr/bin/*`, `/usr/sbin/*`, `/usr/local/bin/*`）をリスクベース計算ルール（ハードコーディング）に変換する

**R3**: リスクベース制御を唯一の検証方式とし、より柔軟で細かなセキュリティ制御を提供する

### 2.2 機能要件

#### 2.2.1 統一セキュリティ検証システム
**F1**: PathResolver.ValidateCommand を拡張し、リスクレベル評価に完全統合

**F2**: ハードコーディングされたホワイトリストルールの変換：
- 現状の正規表現パターンをリスクレベル計算ルール（ハードコーディング）に変換
- パス単位での直接的なリスクレベル決定機能

**F3**: リスクベース検証のみの運用（ホワイトリスト廃止）

#### 2.2.2 リスクベース検証の標準化
**F4**: `security.AnalyzeCommandSecurity` を全セキュリティ検証の基盤として使用

**F5**: verification フェーズでのリスクレベル制限設定：
- `max_risk_level`: verification での制限レベル（デフォルト: "medium"）
- 従来のホワイトリスト許可コマンドは None/Low レベルで自動許可

**F6**: ハードコーディングされたパス→リスクベース変換ルール：
- `/bin/*`, `/usr/bin/*` コマンド: デフォルト Low レベル（実行許可）
- `/usr/sbin/*`, `/sbin/*` コマンド: デフォルト Medium レベル（システム管理）
- `/usr/local/bin/*` コマンド: デフォルト Low レベル（カスタムツール）
- 明示的リスクレベル指定コマンド:
  - `/usr/bin/sudo`, `/bin/su`: Critical レベル（特権昇格）
  - `/usr/bin/curl`, `/usr/bin/wget`: Medium レベル（ネットワーク）
  - `/usr/sbin/systemctl`, `/usr/sbin/service`: High レベル（システム制御）
  - `/bin/rm`, `/usr/bin/dd`: High レベル（破壊的操作）

#### 2.2.3 設定システムの統合
**F7**: リスクレベル制限設定：
既存のコマンドレベルの設定を利用。新規開発なし。

**F8**: 設定値の検証とエラーハンドリング

**F9**: ハードコーディングされたリスクレベル計算システム

### 2.3 非機能要件

#### 2.3.1 互換性
**NF1**: 現状のハードコーディングされたホワイトリストで許可されていたコマンドは、リスクベースシステムでも同等以上のアクセスレベルを保証

**NF2**: 既存の API インターフェースを変更せず、内部実装のみ変更

**NF3**: 既存のテストケースは全て通過する（リスクベース基準で）

#### 2.3.2 性能
**NF4**: リスクレベル評価による性能劣化は 5% 以内に抑制

#### 2.3.3 保守性
**NF6**: セキュリティロジックの一元化により保守性を向上

**NF7**: 新しいセキュリティパターンの追加が容易な設計

#### 2.3.4 監査性
**NF8**: 統一されたセキュリティログ出力

**NF9**: セキュリティ判定根拠の明確な記録

## 3. 実装概要

### 3.1 ハードコーディングされたリスクレベル計算
システム内で直接コード化されたロジックによりリスクレベルを決定：

```go
// パスベースの基本ルール
func calculateDefaultRiskLevel(cmdPath string) RiskLevel {
    if strings.HasPrefix(cmdPath, "/bin/") || strings.HasPrefix(cmdPath, "/usr/bin/") {
        return RiskLevelLow
    }
    if strings.HasPrefix(cmdPath, "/usr/sbin/") || strings.HasPrefix(cmdPath, "/sbin/") {
        return RiskLevelMedium
    }
    if strings.HasPrefix(cmdPath, "/usr/local/bin/") {
        return RiskLevelLow
    }
    return RiskLevelHigh
}

// 明示的リスクレベル指定（優先）
var explicitRiskLevels = map[string]RiskLevel{
    "/usr/bin/sudo":       RiskLevelCritical,
    "/usr/bin/curl":       RiskLevelMedium,
    "/usr/sbin/systemctl": RiskLevelHigh,
    "/bin/rm":             RiskLevelHigh,
}
```

### 3.2 最小限の設定
既存のコマンドレベル `max_risk_level` 設定のみを活用し、新規設定システムは開発しない。

## 4. 想定されるエラーメッセージ

### 4.1 リスクレベル超過（verification フェーズ）
```
Error: command_verification_failed - Command blocked during path resolution
Details:
  Command: /usr/bin/wget
  Detected Risk Level: MEDIUM
  Max Allowed Risk Level: LOW
  Phase: Path Resolution
  Suggestion: Adjust max_risk_level in security configuration
```

### 4.2 設定エラー
```
Error: invalid_security_configuration - Invalid risk level specification
Details:
  Setting: max_risk_level = "invalid_level"
  Valid Values: none, low, medium, high, critical
```
  Valid Options: "whitelist", "risk_based"
  Location: security.validation_mode
```

## 5. 実装対象範囲

### 5.1 修正対象ファイル
- `internal/verification/path_resolver.go`: 統合セキュリティ検証の実装
- `internal/runner/security/validator.go`: 設定統合と互換性レイヤー
- `internal/runner/security/command_analysis.go`: API インターフェース調整
- `internal/runner/security/config.go`: 統合設定構造体
- 関連テストファイル群

### 5.2 新規追加予定
- `internal/security/unified_validator.go`: 統一検証エンジン
- 統合テストスイート

### 5.3 ドキュメント更新
- セキュリティ設定ガイド
- API リファレンスの更新
- トラブルシューティングガイド

## 6. 受け入れ条件

### 6.1 機能テスト
- [ ] リスクベースモードで適切なリスクレベル制御が動作する
- [ ] 設定検証が適切にエラーを検出する

### 6.2 互換性テスト
- [ ] 既存のテストケースが全て通過する
- [ ] 性能劣化が許容範囲内である

### 6.3 セキュリティテスト
- [ ] 危険コマンドが適切にブロックされる
- [ ] 安全なコマンドが正常に実行される
- [ ] 特権昇格コマンドが確実にブロックされる
- [ ] 設定による制御が正しく機能する

### 6.4 運用テスト
- [ ] エラーメッセージが理解しやすい
- [ ] ログ出力が統一され監査可能である
- [ ] ドキュメントが設定手順を正確に説明している

## 7. 実装フェーズ

### 7.1 Phase 1: 基盤整備 (3週間)
- 統一リスクベース検証エンジンの設計・実装
- PathResolver への統合
- ホワイトリストのハードコーディングされたロジック→リスクベース変換
- 基本テストケースの作成

### 7.2 Phase 2: 統合・最適化 (3週間)
- 包括的テストスイート
- ドキュメント整備
- エラーハンドリングの改善

### 7.3 Phase 3: 最終調整 (2週間)
- 最終統合テスト
- プロダクション準備

## 8. リスク分析

### 8.1 技術的リスク
- **性能劣化**: リスクレベル評価の処理コスト増加
  - *対策*: 評価結果のキャッシュ、最適化された実装
- **互換性問題**: ハードコーディングルール変換による予期しない動作変更
  - *対策*: 包括的テスト、詳細な互換性検証

### 8.2 運用リスク
- **一括変換リスク**: ハードコーディングルール変更による一括変更の影響
  - *対策*: 段階的テスト、詳細な検証手順、緊急時の復旧計画

### 8.3 セキュリティリスク
- **設定変更時の脆弱性**: 新システム導入時のセキュリティホール
  - *対策*: 厳密な検証、包括的テスト

## 9. 成功指標

### 9.1 技術指標
- セキュリティロジックの一元化達成率: 100%
- ハードコーディングルール変換成功率: 100%
- 性能劣化の抑制: 5% 以内

### 9.2 運用指標
- 互換性維持率: 100%（現状許可コマンドの継続許可）
- セキュリティインシデント発生率: 0%
- 設定エラーの削減: 60% 以上（統合システム化による）

### 9.3 保守性指標
- セキュリティパターン追加の工数削減: 70% 以上（単一システム化）
- テストケース保守工数の削減: 50% 以上
- ドキュメント一元化による情報検索効率向上: 40% 以上

## 10. 将来拡張性

### 10.1 パフォーマンス改善
- コマンドリスク評価結果のキャッシュシステム
- 頻繁に使用されるコマンドの評価最適化
- メモリ効率的なリスクレベル計算

### 10.2 セキュリティ機能拡張
- カスタムリスクパターンの定義機能
- 動的リスクレベル調整（実行コンテキスト依存）
- 機械学習ベースの異常検知統合

### 10.3 運用機能拡張
- リアルタイムセキュリティポリシー更新
- 分散システム間でのセキュリティポリシー同期
- コンプライアンス要件への自動対応

### 10.4 監査・分析機能
- セキュリティ決定の詳細分析ダッシュボード
- リスクトレンド分析とアラート
- 自動セキュリティレポート生成

この統合により、go-safe-cmd-runner のセキュリティアーキテクチャが大幅に簡素化され、より強力で保守しやすいシステムに進化します。
