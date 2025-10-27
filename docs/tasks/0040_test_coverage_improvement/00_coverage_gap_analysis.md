# テストカバレッジギャップ分析

## 1. エグゼクティブサマリー

### 現状
- **現在のカバレッジ**: 77.8%
- **目標カバレッジ**: 85.0%
- **ギャップ**: 7.2ポイント
- **カバレッジ85%未満の関数数**: 206関数
- **カバレッジ0%の関数数**: 95関数

### 主な課題
1. CLIエントリポイントとブートストラップコードのテスト不足
2. ロギング・Slack通知機能のテスト不足
3. エラー型メソッド（Error, Unwrap等）のテスト不足
4. OS依存機能（openat2等）のテスト困難性
5. 特権操作関連のテスト困難性

## 2. パッケージ別カバレッジ詳細

### 2.1 カバレッジ0%のパッケージ（緊急対応必要）

| パッケージ | カバレッジ | 主な機能 | 優先度 |
|-----------|----------|---------|-------|
| `internal/cmdcommon` | 0.0% | CLI共通ユーティリティ | 高 |
| `internal/runner/cli` | 0.0% | CLIパーサー・バリデーション | 高 |
| `internal/runner/errors` | 0.0% | エラー分類・ロギング | 高 |
| `internal/runner/risk` | 0.0% | リスク評価 | 中 |

### 2.2 カバレッジ50%未満のパッケージ（要改善）

| パッケージ | カバレッジ | 主な機能 | 優先度 |
|-----------|----------|---------|-------|
| `internal/runner/bootstrap` | 11.5% | 初期化・セットアップ | 高 |
| `internal/runner/debug` | 16.4% | デバッグ機能 | 低 |
| `internal/logging` | 53.9% | ロギング・Slack通知 | 中 |

### 2.3 カバレッジ50-85%のパッケージ（改善推奨）

| パッケージ | カバレッジ | 主な機能 | 優先度 |
|-----------|----------|---------|-------|
| `internal/groupmembership` | 65.7% | グループメンバーシップ管理 | 中 |
| `internal/runner/privilege` | 62.0% | 特権管理 | 中 |
| `internal/runner/runnertypes` | 74.0% | 型定義・設定 | 中 |
| `internal/safefileio` | 76.4% | 安全なファイルI/O | 高 |
| `internal/runner/resource` | 78.4% | リソース管理 | 中 |
| `internal/runner/variable` | 80.0% | 変数管理 | 低 |
| `internal/filevalidator` | 82.9% | ファイル検証 | 中 |
| `internal/runner/environment` | 82.5% | 環境変数管理 | 中 |
| `internal/verification` | 83.2% | ファイル検証統合 | 中 |

### 2.4 カバレッジ85%以上のパッケージ（目標達成）

| パッケージ | カバレッジ | 主な機能 |
|-----------|----------|---------|
| `internal/runner` | 85.2% | コマンド実行統合 |
| `internal/runner/config` | 86.7% | 設定管理 |
| `internal/runner/executor` | 86.7% | コマンド実行 |
| `internal/runner/security` | 87.5% | セキュリティ検証 |
| `internal/filevalidator/encoding` | 90.9% | ファイル名エンコーディング |
| `internal/runner/output` | 90.5% | 出力管理 |
| `internal/common` | 92.5% | 共通ユーティリティ |
| `internal/runner/audit` | 92.4% | 監査ログ |
| `internal/redaction` | 92.7% | 機密情報マスキング |
| `internal/terminal` | 98.5% | ターミナル機能 |
| `internal/color` | 100.0% | 色出力 |

## 3. 未カバー関数の概要分類

### 3.1 CLIエントリポイント（5関数、優先度：最高）

**実装難易度**: 低
**カバレッジ向上への寄与**: 中（約0.8%）

| 関数 | ファイル | カバレッジ |
|-----|---------|----------|
| `ParseFlags` | `internal/cmdcommon/common.go:41` | 0.0% |
| `CreateValidator` | `internal/cmdcommon/common.go:75` | 0.0% |
| `PrintUsage` | `internal/cmdcommon/common.go:80` | 0.0% |
| `ParseDryRunDetailLevel` | `internal/runner/cli/output.go:8` | 0.0% |
| `ParseDryRunOutputFormat` | `internal/runner/cli/output.go:22` | 0.0% |

**テスト戦略**:
- 単純な単体テストで十分カバー可能
- flag パッケージのモック不要（os.Args の操作で対応可）
- 正常系・異常系のシンプルなテストケース

### 3.2 エラー型メソッド（27関数、優先度：中）

**実装難易度**: 低
**カバレッジ向上への寄与**: 低（約0.5%、ボイラープレートコード）

| 関数例 | ファイル | カバレッジ |
|-------|---------|----------|
| `Error()` | 各種エラー型 | 0.0% |
| `Unwrap()` | 各種エラー型 | 0.0% |

**テスト戦略**:
- エラーメッセージ生成の確認
- errors.Is/errors.As による型判定の確認
- 優先度は低いが、実装は簡単

### 3.3 ブートストラップ・初期化（3関数、優先度：高）

**実装難易度**: 中
**カバレッジ向上への寄与**: 中（約1.5%）

| 関数 | ファイル | カバレッジ |
|-----|---------|----------|
| `SetupLogging` | `internal/runner/bootstrap/environment.go:12` | 0.0% |
| `SetupLoggerWithConfig` | `internal/runner/bootstrap/logger.go:30` | 0.0% |
| `InitializeVerificationManager` | `internal/runner/bootstrap/verification.go:13` | 0.0% |

**テスト戦略**:
- 統合テストまたはE2Eテストで間接的にカバー
- 依存関係のモック化が必要
- 実際の初期化シーケンスを再現するテスト

### 3.4 ロギング・Slack通知（20関数、優先度：中）

**実装難易度**: 中～高
**カバレッジ向上への寄与**: 高（約2.5%）

**主な未カバー関数**:
- `internal/logging/slack_handler.go`: Slack通知機能（11関数）
- `internal/logging/safeopen.go`: 安全なファイルオープン（4関数）
- `internal/logging/message_formatter.go`: メッセージフォーマット（2関数）
- `internal/logging/pre_execution_error.go`: 実行前エラー処理（1関数）
- `internal/logging/multihandler.go`: 複数ハンドラ管理（1関数）

**テスト戦略**:
- **Slack通知**: HTTPクライアントのモック化、ネットワークI/Oのテスト
- **ファイルI/O**: ファイルシステムのモック使用
- **ログフォーマット**: 単体テストで十分

**課題**:
- ネットワークI/Oのモック化
- 外部サービス（Slack）への依存
- 非同期処理のテスト

### 3.5 バリデーション（15関数、優先度：中）

**実装難易度**: 中
**カバレッジ向上への寄与**: 中（約2.0%）

**主な低カバレッジ関数**:
- `ValidateEnvironmentVariable`: 0.0%
- `validateGroupWritePermissions`: 55.0%
- `validatePrivilegedCommand`: 57.1%
- `ValidateOutputPath`: 60.0%
- `validateFileHash`: 70.0%

**テスト戦略**:
- エッジケースの追加テスト
- セキュリティ関連の境界値テスト
- エラーパスのカバレッジ向上

### 3.6 I/O操作（9関数、優先度：高）

**実装難易度**: 高
**カバレッジ向上への寄与**: 中（約1.5%）

**主な低カバレッジ関数**:
- `IsOpenat2Available`: 0.0%（OS依存）
- `safeOpenFileInternal`: 60.7%
- `CanCurrentUserSafelyWriteFile`: 66.7%
- `CanCurrentUserSafelyReadFile`: 73.9%
- `OpenFileWithPrivileges`: 76.5%

**テスト戦略**:
- **OS依存機能**: ビルドタグによる環境分離、統合テストでの実機確認
- **特権操作**: モックを使った単体テスト + 統合テスト
- **ファイルシステム操作**: 既存のMockFileSystemを活用

**課題**:
- openat2 はLinux固有システムコール（カーネル5.6以降）
- 特権操作のテストには特別な環境が必要
- CI環境での実行制約

### 3.7 オプションビルダー（8関数、優先度：低）

**実装難易度**: 低
**カバレッジ向上への寄与**: 低（約0.3%）

**主な未カバー関数**:
- `WithExecutor`, `WithPrivilegeManager`, `WithAuditLogger` など
- 主にテスト用のオプション関数

**テスト戦略**:
- 既存のテストコードで使用することで自然にカバー
- 明示的な単体テストは不要

### 3.8 デバッグ機能（4関数、優先度：低）

**実装難易度**: 低
**カバレッジ向上への寄与**: 低（約0.5%）

| 関数 | ファイル | カバレッジ |
|-----|---------|----------|
| `extractFromEnvVariables` | `internal/runner/debug/inheritance.go:102` | 0.0% |
| `findUnavailableVars` | `internal/runner/debug/inheritance.go:116` | 0.0% |
| `findRemovedAllowlistVars` | `internal/runner/debug/inheritance.go:133` | 0.0% |
| `PrintTrace` | `internal/runner/debug/trace.go:23` | 0.0% |

**テスト戦略**:
- デバッグ機能は本番コードパスではない
- 優先度は低いが、実装は簡単

### 3.9 その他（114関数、優先度：個別評価）

**実装難易度**: 混在
**カバレッジ向上への寄与**: 高（約5.0%）

多様な機能を含むため、個別に評価が必要。主な領域：
- キャッシュ管理
- 一時ディレクトリ管理
- 内部ヘルパー関数
- エッジケース処理

## 4. テスト実装難易度別分類

### 4.1 容易（Low）- 即座に実装可能

**推定工数**: 1-2日
**カバレッジ向上**: 約2.5%

- CLIパーサー・バリデーション（6関数）
- エラー型メソッド（27関数、ただし優先度低）
- デバッグ機能（4関数、優先度低）
- オプションビルダー（8関数、優先度低）

**アプローチ**:
- 標準的な単体テストで対応
- モック不要または最小限のモック
- テストケースも比較的単純

### 4.2 中程度（Medium）- モック・統合テストが必要

**推定工数**: 3-5日
**カバレッジ向上**: 約4.0%

- ブートストラップ・初期化（3関数）
- バリデーション（15関数）
- ロギング機能（Slack除く、9関数）

**アプローチ**:
- 既存のモックインフラストラクチャを活用
- 統合テストレベルでのカバレッジ
- 依存関係の適切な注入

### 4.3 困難（High）- 特殊な環境・大規模モックが必要

**推定工数**: 5-10日
**カバレッジ向上**: 約4.0%

- Slack通知機能（11関数）
- OS依存I/O操作（openat2関連）
- 特権操作関連

**アプローチ**:
- HTTPクライアントのモック実装
- ビルドタグによる環境分離
- 統合テスト環境の整備
- 一部機能はCI環境での制約あり

### 4.4 実用性が低い（Very Low Priority）

**推定工数**: -
**カバレッジ向上**: 約0.8%

- テストヘルパ関数（4関数、manager_testing.go等）
- 既に除外対象だが集計に含まれているもの

**アプローチ**:
- テスト対象から除外
- または自然にカバーされるのを待つ

## 5. カバレッジ向上戦略

### 5.1 短期目標（1-2週間）：80%達成

**目標カバレッジ**: 80%（+2.2ポイント）

**実施項目**:
1. **CLIエントリポイントのテスト実装**（優先度：最高）
   - `internal/cmdcommon`: 3関数
   - `internal/runner/cli`: 3関数
   - 推定カバレッジ向上: +0.8%

2. **ブートストラップコードのテスト実装**（優先度：高）
   - `internal/runner/bootstrap`: 3関数
   - 推定カバレッジ向上: +1.5%

3. **エラー関連のテスト実装**（優先度：中）
   - `internal/runner/errors`: 3関数
   - エラー型メソッド: 主要なもの10関数程度
   - 推定カバレッジ向上: +0.5%

**期待成果**: カバレッジ約80%達成

### 5.2 中期目標（3-4週間）：85%達成

**目標カバレッジ**: 85%（+7.2ポイント）

**実施項目**（短期目標に加えて）:
1. **バリデーション関数の補強**（優先度：高）
   - エッジケース・エラーパスの追加
   - 推定カバレッジ向上: +2.0%

2. **ロギング機能のテスト実装**（優先度：中）
   - ファイルI/O関連
   - フォーマット関連
   - 推定カバレッジ向上: +1.5%

3. **I/O操作のテスト補強**（優先度：高）
   - モック活用による単体テスト
   - 統合テストの追加
   - 推定カバレッジ向上: +1.5%

**期待成果**: 目標カバレッジ85%達成

### 5.3 長期目標（将来的に）：90%以上

**目標カバレッジ**: 90%以上

**実施項目**:
1. **Slack通知機能の完全テスト**
   - HTTPクライアントモックの実装
   - リトライ・エラーハンドリングのテスト
   - 推定カバレッジ向上: +2.0%

2. **OS依存機能のテスト環境整備**
   - openat2関連のテスト
   - 特権操作のテスト環境
   - 推定カバレッジ向上: +1.0%

3. **残存エッジケースの網羅**
   - その他の低カバレッジ関数
   - 推定カバレッジ向上: +2.0%

## 6. 実装上の注意事項

### 6.1 既存のテストインフラストラクチャ活用

プロジェクトには充実したモックインフラが存在：
- `internal/common/testing/mocks.go`: ファイルシステムモック
- `internal/runner/executor/testing/testify_mocks.go`: 実行モック
- `internal/runner/privilege/testing/mocks.go`: 特権管理モック
- `internal/verification/testing/mocks.go`: 検証モック

これらを積極的に活用し、新規モック作成は最小限に。

### 6.2 ビルドタグの適切な使用

既存のパターンに従う：
- `//go:build test`: テスト専用コード
- `//go:build !test`: 本番コード

### 6.3 統合テストとの適切なバランス

単体テストで無理にカバレッジを上げるより、統合テストで実際の動作を確認する方が有効な場合：
- ブートストラップシーケンス
- エンドツーエンドのコマンド実行
- セキュリティ検証フロー

### 6.4 CI環境の制約

以下の機能はCI環境でテスト困難：
- openat2システムコール（カーネルバージョン依存）
- 特権操作（root権限必要）
- Slack通知（外部サービス依存）

→ モック化またはローカル環境でのマニュアルテストで対応

## 7. リスク評価

### 7.1 技術的リスク

| リスク | 影響 | 確率 | 対策 |
|-------|-----|-----|-----|
| OS依存機能のテスト困難性 | 中 | 高 | ビルドタグ分離、統合テスト |
| モック実装の複雑化 | 中 | 中 | 既存モックの再利用 |
| テストメンテナンスコスト増加 | 低 | 中 | シンプルなテスト設計 |

### 7.2 スケジュールリスク

| リスク | 影響 | 確率 | 対策 |
|-------|-----|-----|-----|
| Slack通知テストの遅延 | 低 | 中 | 優先度を下げる |
| 統合テスト環境の問題 | 中 | 低 | 早期の環境確認 |

## 8. 推奨アクションプラン

### Phase 1（1週目）: Quick Wins
1. CLIパーサーのテスト実装
2. エラー型メソッドのテスト実装
3. カバレッジ: 77.8% → 79.5%

### Phase 2（2週目）: Core Infrastructure
1. ブートストラップコードのテスト実装
2. エラー分類・ロギングのテスト実装
3. カバレッジ: 79.5% → 82.0%

### Phase 3（3週目）: Validation & I/O
1. バリデーション関数の補強
2. I/O操作のテスト補強
3. カバレッジ: 82.0% → 85.0% ← **目標達成**

### Phase 4（4週目以降）: Advanced Features（オプション）
1. ロギング機能の完全カバー
2. Slack通知のテスト
3. OS依存機能のテスト環境整備
4. カバレッジ: 85.0% → 90.0%

## 9. 測定とモニタリング

### 9.1 カバレッジ測定コマンド

```bash
# 全体カバレッジ
go test -tags test -coverprofile=coverage.out -coverpkg=./internal/... ./internal/...
go tool cover -func=coverage.out | tail -1

# パッケージ別カバレッジ
go test -tags test -cover ./internal/...

# HTML形式のカバレッジレポート
go tool cover -html=coverage.out -o coverage.html
```

### 9.2 継続的なモニタリング

- 各PR でのカバレッジチェック
- カバレッジ低下の防止
- 新規コードには同時にテストを追加（TDD原則）

## 10. 未カバー関数の詳細分類

### 10.1 CLIエントリポイント (5関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 中 (+0.8%)
- **優先度**: 最高

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `CreateValidator` | `internal/cmdcommon/common.go` | 75 | 0.0% |
| `ParseDryRunDetailLevel` | `internal/runner/cli/output.go` | 8 | 0.0% |
| `ParseDryRunOutputFormat` | `internal/runner/cli/output.go` | 22 | 0.0% |
| `ParseFlags` | `internal/cmdcommon/common.go` | 41 | 0.0% |
| `PrintUsage` | `internal/cmdcommon/common.go` | 80 | 0.0% |

### 10.2 ブートストラップ・初期化 (4関数)

- **実装難易度**: 中
- **カバレッジ向上への寄与**: 中 (+1.5%)
- **優先度**: 高

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `InitializeVerificationManager` | `internal/runner/bootstrap/verification.go` | 13 | 0.0% |
| `InitializeVerificationManagerForTest` | `internal/runner/bootstrap/verification_test_helper.go` | 17 | 0.0% |
| `SetupLoggerWithConfig` | `internal/runner/bootstrap/logger.go` | 30 | 0.0% |
| `SetupLogging` | `internal/runner/bootstrap/environment.go` | 12 | 0.0% |

### 10.3 エラー分類・ロギング (3関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.3%)
- **優先度**: 高

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `ClassifyVerificationError` | `internal/runner/errors/classification.go` | 8 | 0.0% |
| `LogClassifiedError` | `internal/runner/errors/logging.go` | 17 | 0.0% |
| `LogCriticalToStderr` | `internal/runner/errors/logging.go` | 11 | 0.0% |

### 10.4 エラー型メソッド (22関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.5%)
- **優先度**: 低

主にボイラープレートコードの`Error()`, `Unwrap()`, `Is()`メソッド。
優先度は低いが実装は容易。

### 10.5 Slack通知 (11関数)

- **実装難易度**: 高
- **カバレッジ向上への寄与**: 高 (+1.5%)
- **優先度**: 中

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `Enabled` | `internal/logging/slack_handler.go` | 135 | 0.0% |
| `GetSlackWebhookURL` | `internal/logging/slack_handler.go` | 105 | 0.0% |
| `Handle` | `internal/logging/slack_handler.go` | 140 | 0.0% |
| `buildCommandGroupSummary` | `internal/logging/slack_handler.go` | 229 | 0.0% |
| `buildGenericMessage` | `internal/logging/slack_handler.go` | 577 | 0.0% |
| `buildPreExecutionError` | `internal/logging/slack_handler.go` | 322 | 0.0% |
| `buildPrivilegeEscalationFailure` | `internal/logging/slack_handler.go` | 508 | 0.0% |
| `buildPrivilegedCommandFailure` | `internal/logging/slack_handler.go` | 439 | 0.0% |
| `buildSecurityAlert` | `internal/logging/slack_handler.go` | 374 | 0.0% |
| `generateBackoffIntervals` | `internal/logging/slack_handler.go` | 586 | 0.0% |
| `sendToSlack` | `internal/logging/slack_handler.go` | 596 | 0.0% |

**課題**: HTTPクライアントのモック化、ネットワークI/Oテスト、リトライロジックのテストが必要

### 10.6 ロギング - ファイルI/O (4関数)

- **実装難易度**: 中
- **カバレッジ向上への寄与**: 中 (+0.5%)
- **優先度**: 中

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `GenerateRunID` | `internal/logging/safeopen.go` | 57 | 0.0% |
| `NewSafeFileOpener` | `internal/logging/safeopen.go` | 32 | 0.0% |
| `OpenFile` | `internal/logging/safeopen.go` | 40 | 0.0% |
| `ValidateLogDir` | `internal/logging/safeopen.go` | 63 | 0.0% |

### 10.7 ロギング - フォーマット (3関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.2%)
- **優先度**: 低

主にメッセージフォーマッターとマルチハンドラーの補助機能。

### 10.8 バリデーション (15関数)

- **実装難易度**: 中
- **カバレッジ向上への寄与**: 高 (+2.0%)
- **優先度**: 高

主要な低カバレッジ関数：
- `ValidateEnvironmentVariable`: 0.0%
- `validateGroupWritePermissions`: 55.0%
- `validatePrivilegedCommand`: 57.1%
- `ValidateOutputPath`: 60.0%
- `validateFileHash`: 70.0%

**戦略**: 既存テストにエッジケース・エラーパスを追加

### 10.9 I/O操作 - OS依存 (2関数)

- **実装難易度**: 高
- **カバレッジ向上への寄与**: 低 (+0.3%)
- **優先度**: 低

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `IsOpenat2Available` | `internal/safefileio/safe_file.go` | 145 | 0.0% |
| `isOpenat2Available` | `internal/safefileio/safe_file.go` | 68 | 80.0% |

**課題**: Linux固有システムコール（カーネル5.6以降）、CI環境での制約

### 10.10 I/O操作 - 特権 (3関数)

- **実装難易度**: 高
- **カバレッジ向上への寄与**: 中 (+0.8%)
- **優先度**: 中

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `CanCurrentUserSafelyReadFile` | `internal/groupmembership/manager.go` | 316 | 73.9% |
| `CanCurrentUserSafelyWriteFile` | `internal/groupmembership/manager.go` | 281 | 66.7% |
| `OpenFileWithPrivileges` | `internal/filevalidator/privileged_file.go` | 14 | 76.5% |

**課題**: 特権操作のテストには特別な環境が必要

### 10.11 I/O操作 - 標準 (4関数)

- **実装難易度**: 中
- **カバレッジ向上への寄与**: 低 (+0.4%)
- **優先度**: 中

主に`safefileio`パッケージの内部関数とエラーパス。既存のモックファイルシステムで対応可能。

### 10.12 オプションビルダー (8関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.3%)
- **優先度**: 低

`With*`関数群。既存テストで使用することで自然にカバー可能。

### 10.13 デバッグ機能 (4関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.5%)
- **優先度**: 低

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `PrintTrace` | `internal/runner/debug/trace.go` | 23 | 0.0% |
| `extractFromEnvVariables` | `internal/runner/debug/inheritance.go` | 102 | 0.0% |
| `findRemovedAllowlistVars` | `internal/runner/debug/inheritance.go` | 133 | 0.0% |
| `findUnavailableVars` | `internal/runner/debug/inheritance.go` | 116 | 0.0% |

### 10.14 キャッシュ管理 (1関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.1%)
- **優先度**: 低

| 関数名 | ファイル | 行 | カバレッジ |
|--------|---------|-----|-----------|
| `clearExpiredCache` | `internal/groupmembership/manager.go` | 444 | 0.0% |

### 10.15 一時ディレクトリ管理 (10関数)

- **実装難易度**: 低
- **カバレッジ向上への寄与**: 低 (+0.2%)
- **優先度**: 低

主に`CreateTempDir`, `CleanupTempDir`, `CleanupAllTempDirs`系の関数。
モックファイルシステムで容易にテスト可能。

### 10.16 その他 (95関数)

- **実装難易度**: 混在
- **カバレッジ向上への寄与**: 中 (+2.0%)
- **優先度**: 個別評価

多様な機能を含むため個別評価が必要。主な領域：
- 内部ヘルパー関数
- エッジケース処理
- エラーハンドリング
- 統合テストで自然にカバーされる可能性のある関数

---

## 11. 実装戦略の詳細

### Phase 1: Quick Wins（1週目）

**目標**: 77.8% → 79.5% (+1.7ポイント)
**推定工数**: 2-3日

#### 実施内容

1. **CLIエントリポイントのテスト**（5関数、+0.8%）

   新規ファイル:
   - `internal/cmdcommon/common_test.go`
   - `internal/runner/cli/output_test.go`
   - `internal/runner/cli/validation_test.go`

   テストケース:
   - `TestParseFlags`: 正常系、引数不足、デフォルト値
   - `TestCreateValidator`: バリデータ生成
   - `TestPrintUsage`: 使用方法表示
   - `TestParseDryRunDetailLevel`: 各レベルのパース
   - `TestParseDryRunOutputFormat`: 各フォーマットのパース
   - `TestValidateConfigCommand`: 設定コマンドバリデーション

2. **エラー分類・ロギングのテスト**（3関数、+0.3%）

   新規ファイル:
   - `internal/runner/errors/classification_test.go`
   - `internal/runner/errors/logging_test.go`

   テストケース:
   - `TestClassifyVerificationError`: エラー分類
   - `TestLogCriticalToStderr`: stderr出力
   - `TestLogClassifiedError`: 分類済みエラーログ

3. **エラー型メソッドのテスト**（主要10関数、+0.3%）

   拡張ファイル:
   - `internal/runner/config/errors_test.go`
   - `internal/runner/runnertypes/errors_test.go`
   - `internal/common/timeout_test.go`

   テストアプローチ:
   - `errors.Is()` / `errors.As()` による型判定
   - エラーチェーンの確認

4. **キャッシュ管理のテスト**（1関数、+0.1%）
   - `internal/groupmembership/manager_test.go` を拡張
   - `TestClearExpiredCache`: キャッシュ有効期限

5. **一時ディレクトリ管理のテスト**（簡易版、+0.2%）
   - モックファイルシステムを活用
   - `TempDir()` などの簡単な関数

**成果物**: 新規5-6ファイル、カバレッジ79.5%

---

### Phase 2: Core Infrastructure（2週目）

**目標**: 79.5% → 82.0% (+2.5ポイント)
**推定工数**: 3-4日

#### 実施内容

1. **ブートストラップコードのテスト**（3関数、+1.5%）

   新規ファイル:
   - `internal/runner/bootstrap/environment_test.go`
   - `internal/runner/bootstrap/logger_test.go`
   - `internal/runner/bootstrap/verification_test.go`

   テストケース:
   - `TestSetupLogging`: ロギング初期化
   - `TestSetupLoggerWithConfig`: 設定ベースのロガー初期化
   - `TestInitializeVerificationManager`: 検証マネージャー初期化

   テストアプローチ:
   - 統合テストレベル: 実際の初期化フローを再現
   - モックを最小限に
   - エラーハンドリングの確認

2. **ロギング - ファイルI/Oのテスト**（4関数、+0.5%）

   新規ファイル:
   - `internal/logging/safeopen_test.go`

   テストケース:
   - `TestNewSafeFileOpener`: ファイルオープナー作成
   - `TestOpenFile`: 安全なファイルオープン
   - `TestGenerateRunID`: RunID生成
   - `TestValidateLogDir`: ログディレクトリ検証

   テストアプローチ:
   - モックファイルシステム使用
   - パーミッションエラーのシミュレート

3. **ロギング - フォーマットのテスト**（2関数、+0.2%）

   拡張ファイル:
   - `internal/logging/message_formatter_test.go`
   - `internal/logging/multihandler_test.go`

4. **オプションビルダーのテスト**（主要5関数、+0.3%）

   拡張ファイル:
   - `internal/runner/runner_test.go`
   - 既存テストで`With*`関数を明示的に使用

**成果物**: 新規5-6ファイル、カバレッジ82.0%

---

### Phase 3: Validation & I/O（3週目）

**目標**: 82.0% → 85.0% (+3.0ポイント)
**推定工数**: 4-5日

#### 実施内容

1. **バリデーション関数の補強**（15関数、+2.0%）

   拡張ファイル（エッジケース追加）:
   - `internal/runner/environment/filter_test.go`
     - `TestValidateEnvironmentVariable`: 無効な文字、長すぎる値
   - `internal/runner/security/file_validation_test.go`
     - `TestValidateGroupWritePermissions`: グループ権限境界値
   - `internal/runner/executor/executor_test.go`
     - `TestValidatePrivilegedCommand`: セキュリティ違反ケース
   - `internal/runner/resource/normal_manager_test.go`
     - `TestValidateOutputPath`: パストラバーサル、シンボリックリンク
   - `internal/runner/security/hash_validation_test.go`
     - `TestValidateFileHash`: エラーパス
   - `internal/filevalidator/hash_manifest_test.go`
     - `TestValidateHashManifest`: マニフェスト検証
   - `internal/runner/config/validator_test.go`
     - `TestNewConfigValidator`: 設定バリデータ
   - `internal/groupmembership/manager_test.go`
     - `TestValidateRequestedPermissions`: パーミッション検証

   テストアプローチ:
   - 境界値テスト
   - セキュリティテスト
   - エラーパスの網羅

2. **I/O操作 - 標準のテスト補強**（6関数、+0.4%）

   拡張ファイル:
   - `internal/safefileio/safe_file_test.go`
     - `TestSafeOpenFileInternal`: エラーパス
   - `internal/groupmembership/manager_test.go`
     - `TestCanCurrentUserSafelyWriteFile`: 書き込み権限チェック
     - `TestCanCurrentUserSafelyReadFile`: 読み取り権限チェック
   - `internal/runner/output/file_test.go`
     - `TestWriteToTemp`, `TestCreateTempFile`, `TestRemoveTemp`
   - `internal/filevalidator/privileged_file_test.go`
     - `TestOpenFileWithPrivileges`: 特権ファイルオープン

3. **デバッグ機能のテスト**（4関数、+0.5%）

   新規ファイル:
   - `internal/runner/debug/inheritance_test.go`
   - `internal/runner/debug/trace_test.go`

   テストケース:
   - `TestExtractFromEnvVariables`: 環境変数抽出
   - `TestFindUnavailableVars`: 利用不可変数検出
   - `TestFindRemovedAllowlistVars`: 削除変数検出
   - `TestPrintTrace`: トレース出力

4. **一時ディレクトリ管理の完全カバー**（残り、+0.1%）

   拡張ファイル:
   - `internal/runner/resource/default_manager_test.go`
   - `internal/runner/resource/normal_manager_test.go`
   - `internal/runner/resource/dryrun_manager_test.go`
   - `internal/runner/executor/executor_test.go`

**成果物**: 既存テストの大幅拡張、**カバレッジ85.0%達成** ← **目標達成！**

---

### Phase 4: Advanced Features（4週目以降、オプション）

**目標**: 85.0% → 90.0% (+5.0ポイント)
**推定工数**: 7-10日

#### 実施内容

1. **Slack通知の完全テスト**（11関数、+1.5%）

   大幅拡張:
   - `internal/logging/slack_handler_test.go`

   必要な準備:
   - HTTPクライアントのモック実装
   - `httptest.Server` によるモックWebhook
   - リトライロジックのテスト

   テストケース:
   - `TestGetSlackWebhookURL`: URL取得
   - `TestEnabled`: ハンドラ有効化判定
   - `TestHandle`: メッセージハンドリング
   - `TestBuildCommandGroupSummary`: サマリー構築
   - `TestBuildPreExecutionError`: 実行前エラー
   - `TestBuildPrivilegedCommandFailure`: 特権コマンド失敗
   - `TestBuildPrivilegeEscalationFailure`: 権限昇格失敗
   - `TestBuildSecurityAlert`: セキュリティアラート
   - `TestBuildGenericMessage`: 汎用メッセージ
   - `TestGenerateBackoffIntervals`: バックオフ間隔
   - `TestSendToSlack`: Slack送信（モック）

   テストアプローチ:
   - `httptest.Server` でモックWebhookサーバー
   - リトライロジックのタイムアウトテスト
   - JSONペイロードの検証

2. **I/O操作 - OS依存のテスト**（2関数、+0.3%）

   新規ファイル（ビルドタグ使用）:
   - `internal/safefileio/safe_file_openat2_test.go`

   テストケース:
   - `TestIsOpenat2Available`: openat2利用可否判定
   - `TestSafeOpenFileWithOpenat2`: openat2を使った安全なオープン

   テストアプローチ:
   - ビルドタグ `//go:build linux && !android`
   - カーネルバージョンチェック
   - CI環境では条件付きスキップ
   - ローカル環境でのマニュアルテスト

3. **I/O操作 - 特権のテスト環境整備**（3関数、+0.8%）

   新規ファイル:
   - `internal/groupmembership/manager_privileged_test.go`
   - `internal/filevalidator/privileged_file_integration_test.go`

   必要な準備:
   - Dockerfileの作成
   - CI設定の更新
   - テスト用ユーザー/グループ作成スクリプト

   テストアプローチ:
   - Docker環境での統合テスト
   - モックによる単体テスト（既存）
   - CI環境での特権テスト用コンテナ

4. **その他の低カバレッジ関数**（主要20関数、+2.4%）
   - 個別評価して優先度決定
   - 統合テストで自然にカバーされるものを確認

**成果物**: HTTPモックインフラ、OS依存テスト環境、特権テスト環境、カバレッジ90.0%

---

### 各Phaseの進め方

#### テスト実装の原則

1. **TDDアプローチ**: 期待動作は既存コードから把握
2. **既存インフラの最大活用**: モックは既存のものを優先使用
3. **シンプルさの維持**: 複雑なテストは避ける
4. **エラーパスの重視**: セキュリティ関連は特に重要
5. **統合テストとのバランス**: 適切な場合は統合テストで

#### 各Phaseの完了条件

- 目標カバレッジ達成
- 全テストがパス（既存テスト含む）
- `make lint` がクリーン
- `make fmt` 実行済み
- コードレビュー完了（必要に応じて）

#### リスク管理

**技術的リスク**:
- Phase 4のHTTPモック実装が予想より複雑
  → `httptest` ライブラリの活用で軽減

**スケジュールリスク**:
- テスト実装が予定より時間がかかる
  → Phase 3まで（85%）を最優先、Phase 4は余裕があれば

**品質リスク**:
- テストコードの保守性低下
  → コードレビューで品質担保、リファクタリングを恐れない

---

## 12. まとめ

現在のカバレッジ77.8%から目標の85%まで、約7.2ポイントの向上が必要。主な課題は：

1. **CLIとブートストラップコード**のテスト不足（最優先）
2. **ロギング・Slack通知**のテスト不足
3. **OS依存・特権操作**のテスト困難性

段階的なアプローチにより、3-4週間で目標達成が可能。短期的にはシンプルな単体テストでQuick Winsを狙い、中期的には統合テストとモックの活用で段階的にカバレッジを向上させる戦略を推奨。

長期的には90%以上を目指すが、そのためにはテスト環境の整備とHTTPモック等の追加インフラが必要。コスト対効果を考慮し、85%達成後に再評価することを推奨する。
