# 対話的UI改善 実装計画書

## 1. 実装概要

本計画では、端末機能検出を分離したTerminalパッケージを作成し、既存のログシステムを改善します。CLICOLOR_FORCE環境変数のサポートも含めますが、今回はANSIエスケープシーケンスを使わないシンプルなカラー対応に限定し、高度なカラー機能は将来拡張として位置づけます。

## 2. 実装フェーズ

### Phase 1: Terminal Package の基盤実装
- [x] `internal/terminal/preference.go` - ユーザー設定管理（CLICOLOR_FORCE対応）
- [x] `internal/terminal/preference_test.go` - ユーザー設定のテスト
- [x] `internal/terminal/detector.go` - 対話性検出
- [x] `internal/terminal/detector_test.go` - 対話性検出のテスト
- [x] `internal/terminal/color.go` - カラー対応検出（シンプル実装）
- [x] `internal/terminal/color_test.go` - カラー対応検出のテスト

### Phase 2: Terminal Capabilities 統合インターフェース
- [ ] `internal/terminal/capabilities.go` - 統合インターフェース
- [ ] `internal/terminal/capabilities_test.go` - 統合機能のテスト

### Phase 3: Logging Package の改善
- [ ] `internal/logging/interactive_handler.go` - 対話的環境用ハンドラ
- [ ] `internal/logging/interactive_handler_test.go` - 対話的ハンドラのテスト
- [ ] `internal/logging/conditional_text_handler.go` - 条件付きテキストハンドラ
- [ ] `internal/logging/conditional_text_handler_test.go` - 条件付きハンドラのテスト
- [ ] `internal/logging/message_formatter.go` - メッセージフォーマッタ
- [ ] `internal/logging/message_formatter_test.go` - フォーマッタのテスト
- [ ] `internal/logging/log_line_tracker.go` - ログ行追跡
- [ ] `internal/logging/log_line_tracker_test.go` - 行追跡のテスト
- [ ] `internal/logging/message_templates.go` - メッセージテンプレート

### Phase 4: 既存システムとの統合
- [ ] `cmd/runner/main.go` の `setupLoggerWithConfig` 関数修正
- [ ] 既存ハンドラとの統合テスト
- [ ] エンドツーエンドテスト

### Phase 5: ドキュメントと最終検証
- [ ] README.md の更新
- [ ] 統合テストの実行
- [ ] パフォーマンステスト
- [ ] セキュリティレビュー

## 3. 詳細実装タスク

### 3.1 Phase 1: Terminal Package の基盤実装

#### 3.1.1 対話性検出 (`internal/terminal/detector.go`)
```go
type InteractiveDetector interface {
    IsInteractive() bool
    IsTerminal() bool
    IsCIEnvironment() bool
}
```

**実装要件:**
- [ ] CI環境の検出（CI, GITHUB_ACTIONS, JENKINS_URL, BUILD_NUMBER等）
- [ ] stdout/stderrのターミナル判定
- [ ] 強制対話モードオプション

**実装対象外**
- [ ] Windowsコンソール対応

**テスト要件:**
- [ ] 各CI環境での動作確認
- [ ] パイプ・リダイレクト環境でのテスト
- [ ] 強制対話モードのテスト

#### 3.1.2 カラー対応検出 (`internal/terminal/color.go`) - シンプル実装
```go
type ColorDetector interface {
    SupportsColor() bool  // 基本的なカラー対応のみ
}
```

**実装要件（シンプル版）:**
- [ ] 基本的なTERM環境変数の判定
- [ ] 一般的な端末での基本カラー対応検出
- [ ] 複雑なカラープロファイル判定は将来拡張として除外

**テスト要件:**
- [ ] 主要端末での基本動作確認
- [ ] 環境変数による制御のテスト

### 3.2 Phase 2: Terminal Capabilities 統合インターフェース

#### 3.2.1 統合機能 (`internal/terminal/capabilities.go`)
```go
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool  // シンプルなbool判定のみ
    HasExplicitUserPreference() bool
    // GetColorProfile() は将来拡張として除外
}
```

**実装要件:**
- [ ] 各コンポーネントの統合
- [ ] オプション駆動の設定
- [ ] ユーザー設定の優先度制御
- [ ] CLICOLOR_FORCE=1の特別処理

**重要な優先度ロジック:**
1. コマンドライン引数（最優先）
2. CLICOLOR_FORCE=1（他の条件を無視）
3. NO_COLOR環境変数
4. CLICOLOR環境変数
5. 端末機能自動検出

**テスト要件:**
- [ ] 環境変数の組み合わせテスト
- [ ] 優先度ロジックの確認
- [ ] オプション設定のテスト

### 3.3 Phase 3: Logging Package の改善

#### 3.3.1 対話的ハンドラ (`internal/logging/interactive_handler.go`)
**実装要件:**
- [ ] Terminalパッケージとの統合
- [ ] カラーメッセージのフォーマット
- [ ] エラーレベルでのログファイルヒント
- [ ] slog標準パターンの準拠（重複チェック除去）

**テスト要件:**
- [ ] カラー出力のテスト
- [ ] 非対話環境での無効化テスト
- [ ] ログファイルヒントのテスト

#### 3.3.2 条件付きテキストハンドラ (`internal/logging/conditional_text_handler.go`)
**実装要件:**
- [ ] 非対話環境での標準出力
- [ ] 対話環境での無効化
- [ ] 既存TextHandlerのラップ

#### 3.3.3 メッセージフォーマッタ (`internal/logging/message_formatter.go`) - カラーリング統合版
**実装要件:**
- [ ] `FormatRecordWithColor` - メインメッセージのフォーマット
- [ ] `FormatLogFileHint` - ログファイルヒントのフォーマット
- [ ] カラーリング操作の一元管理（将来拡張対応）
- [ ] レベル別の視覚的区別（記号やプレフィックスで対応）
- [ ] シンプル実装（ANSIエスケープシーケンス不使用）

**設計改善:**
- [ ] カラーリング操作をformatterに統一
- [ ] InteractiveHandler内での直接的なANSI操作を除去
- [ ] 一貫性のある責務分離

#### 3.3.4 ログ行追跡 (`internal/logging/log_line_tracker.go`)
**実装要件:**
- [ ] グローバルカウンター
- [ ] スレッドセーフ実装
- [ ] 推定行番号計算

### 3.4 Phase 4: 既存システムとの統合

#### 3.4.1 main.go の修正
**修正箇所:**
- [ ] `setupLoggerWithConfig`関数の改修
- [ ] ハンドラチェーンの構築
- [ ] Terminal optionsの設定

**実装要件:**
- [ ] 既存設定との互換性維持
- [ ] 段階的な移行サポート
- [ ] エラーハンドリングの改善

#### 3.4.2 統合テスト
**テストシナリオ:**
- [ ] 対話環境でのカラー出力
- [ ] 非対話環境での通常出力
- [ ] ログファイル出力の継続
- [ ] Slack通知の継続
- [ ] 環境変数の各組み合わせ

## 4. テスト戦略

### 4.1 単体テスト
- [ ] 各パッケージの独立テスト
- [ ] モックを使用した依存関係の分離
- [ ] 境界値テスト
- [ ] エラーケースのテスト

### 4.2 統合テスト
- [ ] ハンドラチェーン全体のテスト
- [ ] 実環境での動作確認
- [ ] CI環境での自動テスト

### 4.3 互換性テスト
- [ ] 既存設定ファイルとの互換性
- [ ] 既存コマンドライン引数との互換性
- [ ] 古い端末での動作確認

## 5. 品質保証

### 5.1 コード品質
- [ ] golangci-lintでの静的解析
- [ ] テストカバレッジ90%以上
- [ ] ベンチマークテストの実装
- [ ] メモリリークの確認

### 5.2 ドキュメント
- [ ] 各パッケージのGoDoc
- [ ] 使用例の追加
- [ ] 設定リファレンスの更新

### 5.3 セキュリティ
- [ ] 環境変数インジェクションの防止
- [ ] ログ出力での機密情報漏洩防止
- [ ] 権限エスカレーションの確認

## 6. 実装上の注意点

### 6.1 後方互換性
- [ ] 既存の動作を変更しない
- [ ] 新機能はオプトイン方式
- [ ] 段階的な移行パス提供

### 6.2 パフォーマンス
- [ ] 環境変数アクセスの最適化
- [ ] キャッシュ機能の活用
- [ ] 遅延初期化の検討

### 6.3 プラットフォーム対応
- [ ] Unix系OSでの動作確認
- [ ] Windowsでの基本動作確認
- [ ] 各種シェルでの動作確認

## 7. 完了条件

### 7.1 機能要件
- [ ] すべてのPhaseのタスク完了
- [ ] CLICOLOR_FORCE=1の正常動作
- [ ] 既存機能の継続動作
- [ ] 新しいカラー機能の動作

### 7.2 品質要件
- [ ] すべてのテストが通過
- [ ] コードカバレッジ90%以上
- [ ] 静的解析エラー0件
- [ ] パフォーマンス劣化なし

### 7.3 ドキュメント要件
- [ ] 仕様書の最新化
- [ ] APIドキュメントの完成
- [ ] 利用ガイドの作成

## 8. リスクと対策

### 8.1 技術リスク
**リスク:** 既存システムとの非互換
**対策:** 段階的実装と詳細なテスト

**リスク:** パフォーマンス劣化
**対策:** ベンチマークテストと最適化

### 8.2 スケジュールリスク
**リスク:** 実装の複雑さによる遅延
**対策:** フェーズ分割と優先度付け

### 8.3 品質リスク
**リスク:** 端末対応の不完全性
**対策:** 主要端末での徹底テスト

## 9. 実装スケジュール目安

- **Phase 1:** 2-3日（基盤実装）
- **Phase 2:** 1-2日（統合インターフェース）
- **Phase 3:** 3-4日（ログシステム改善）
- **Phase 4:** 2-3日（システム統合）
- **Phase 5:** 1-2日（最終検証）

**合計:** 約9-14日の実装期間

## 10. 将来拡張（今回スコープ外）

### 10.1 高度なカラー機能
**対象外とした理由:** API設計の複雑さとANSIエスケープシーケンス対応の工数
**将来実装予定:**
- [ ] ANSIエスケープシーケンスによる本格的なカラー出力
- [ ] カラープロファイル判定（8色/256色/TrueColor）
- [ ] レベル別カラーテーマ
- [ ] カスタムカラーテーマ対応
- [ ] プログレスバー・スピナー等のUI要素

### 10.2 高度な端末機能検出
- [ ] Windows固有のコンソール機能検出
- [ ] 端末サイズ・リサイズイベント対応
- [ ] キーボード入力対応（将来的な対話機能用）
- [ ] マウス入力対応

### 10.3 設定とカスタマイズ
- [ ] 設定ファイルによるカラーテーマ管理
- [ ] 実行時カラー設定変更
- [ ] ユーザー毎の設定永続化

### 10.4 パフォーマンス最適化
- [ ] 端末機能検出結果のキャッシュ
- [ ] 遅延初期化による起動速度改善
- [ ] 大量ログ出力時の最適化

## 11. 次のステップ

1. Phase 1の実装開始（preference.goから）
2. 各フェーズ完了後のレビューと検証
3. 統合テストの実行
4. 本番環境での動作確認
5. 将来拡張の詳細設計（必要に応じて）
