# Dry-Run モードでの最終環境変数表示機能

## 概要

dry-runモードの`--dry-run-detail=full`オプション使用時に、各コマンドに実際に渡される最終的な環境変数とその値、および各変数の出所（Global/Group/Command/System）を表示する機能を追加する。

## 背景

### 現状の課題

現在のdry-runモードでは、以下の情報が表示される:

1. **`PrintFromEnvInheritance`** (グループレベル):
   - env_importの継承関係のみ表示
   - 内部変数名のリストのみで、実際の値は表示されない
   - グループレベルの情報のみ（コマンドレベルの情報は表示されない）

2. **`EnvironmentInfo`** (DetailLevelFull):
   - 環境変数の統計情報のみ表示
   - 変数名と使用コマンド数は表示されるが、値は表示されない
   - どのレベルから値が来ているかの情報がない

### ギャップ

**表示されない重要な情報**:
- コマンドに実際に渡される環境変数の**最終的な値**
- 各環境変数がどのレベル（Global/Group/Command/System）から来ているか
- 変数展開（`%{VAR}`）後の実際の値

これにより、以下のデバッグシナリオで困難が生じる:
- 環境変数の上書きが意図通りに動作しているか確認したい
- `%{VAR}`展開が正しく行われているか確認したい
- セキュリティ監査で実際にどの値が使われるか確認したい

## 目的

dry-runモードで以下を実現する:

1. **最終環境変数の可視化**: コマンドに渡される環境変数の最終的な値を表示
2. **変数の出所の明示**: 各環境変数がGlobal/Group/Command/Systemのどこから来ているかを明示
3. **デバッグ性の向上**: 変数展開や上書きの動作を確認可能にする

## 要件

### 機能要件

#### FR-1: PrintFinalEnvironmentの統合

既存の`debug.PrintFinalEnvironment`関数をdry-runモードに統合する。

**入力**:
- コマンドの最終環境変数マップ (`map[string]string`)
- RuntimeGlobal
- RuntimeGroup
- RuntimeCommand

**出力例**:
```
===== Final Process Environment =====

Environment variables (5):
  PATH=/usr/local/bin:/usr/bin:/bin
    (from Global)
  HOME=/home/testuser
    (from System (filtered by allowlist))
  APP_DIR=/opt/myapp
    (from Group[build])
  LOG_FILE=/opt/myapp/logs/app.log
    (from Command[run_tests])
  DEBUG=true
    (from Command[run_tests])
```

#### FR-2: DetailLevel統合

`--dry-run-detail=full`オプション使用時のみ、最終環境変数を表示する。

**動作**:
- `DetailLevelSummary`: 環境変数情報を表示しない
- `DetailLevelDetailed`: 統計情報のみ表示（既存の動作を維持）
- `DetailLevelFull`: 統計情報 + **最終環境変数の詳細**を表示

#### FR-3: コマンドレベルでの表示

各コマンドの実行前に、そのコマンドに渡される最終環境変数を表示する。

**表示タイミング**:
- DryRunManagerでコマンド分析を行う際
- コマンドごとに個別に表示

#### FR-4: 長い値の切り詰め

環境変数の値が長い場合、読みやすさのために切り詰める。

**仕様**:
- 最大表示長: 60文字
- 60文字を超える場合: 57文字 + "..." で表示
- 例: `VERY_LONG_PATH=/usr/local/very/long/path/that/exceeds/the/limit/an...`

#### FR-5: センシティブデータの考慮

`--show-sensitive`フラグに関わらず、dry-runモードではセキュリティ監査目的で全ての環境変数を表示する。

**理由**:
- dry-runモードは実行前の確認・監査が目的
- 実際にコマンドは実行されないため、値の表示はセキュリティリスクが低い
- 管理者がdry-runで実際の値を確認できることが重要

**注意事項**:
- 通常のログ出力には引き続きredaction機能が適用される
- dry-runの出力は標準出力に表示される

### 非機能要件

#### NFR-1: パフォーマンス

`PrintFinalEnvironment`の呼び出しがdry-runモードのパフォーマンスに大きな影響を与えないこと。

**基準**:
- 環境変数の数が100個の場合でも、表示処理は1ms以内に完了すること

#### NFR-2: 後方互換性

既存のdry-run出力フォーマットを維持し、新しい情報を追加のセクションとして表示すること。

**要件**:
- `DetailLevelSummary`と`DetailLevelDetailed`の出力は変更しない
- `DetailLevelFull`のみ新しいセクションを追加

#### NFR-3: テスタビリティ

新しい機能は単体テストと統合テストでカバーされること。

**要件**:
- `PrintFinalEnvironment`の出力内容を検証するテスト
- 各DetailLevelでの動作を検証するテスト
- 環境変数の出所判定ロジックを検証するテスト

## 実装範囲

### 範囲内

1. **`group_executor.go`の修正**:
   - dry-runモードかつ`DetailLevelFull`の場合に`PrintFinalEnvironment`を呼び出す
   - コマンド実行前（`executeCommandInGroup`内）に呼び出し

2. **テストの追加**:
   - 単体テスト: `debug.PrintFinalEnvironment`の出力検証
   - 統合テスト: dry-runモードでの表示内容検証

3. **ドキュメント更新**:
   - `--dry-run-detail=full`の説明を更新
   - 最終環境変数表示機能について記載

### 範囲外

以下は本タスクの範囲外とする:

1. **JSON形式での出力**: 現在はテキスト形式のみサポート
2. **環境変数のフィルタリング**: 全ての環境変数を表示
3. **変数の値の検証**: 表示のみで、値の妥当性検証は行わない
4. **履歴管理**: 過去のdry-run結果との比較機能は提供しない

## ユースケース

### UC-1: 変数上書きの確認

**シナリオ**:
開発者がGroup/Commandレベルでの環境変数上書きが意図通りに動作しているか確認したい。

**手順**:
1. `--dry-run --dry-run-detail=full`でrunnerを実行
2. 各コマンドの最終環境変数セクションを確認
3. 上書きされた変数の値と出所を確認

**期待結果**:
```
===== Final Process Environment =====

Environment variables (3):
  DB_HOST=production.db.example.com
    (from Global)
  DB_PORT=5432
    (from Group[deploy])
  DB_NAME=test_db
    (from Command[run_migration])
```

### UC-2: 変数展開の確認

**シナリオ**:
開発者が`%{VAR}`形式の変数展開が正しく行われているか確認したい。

**手順**:
1. `vars`で`BASE=/opt/app`を定義
2. `env`で`LOG_DIR=%{BASE}/logs`を定義
3. `--dry-run --dry-run-detail=full`で実行

**期待結果**:
```
Environment variables (2):
  BASE=/opt/app
    (from Command[setup])
  LOG_DIR=/opt/app/logs
    (from Command[setup])
```

### UC-3: セキュリティ監査

**シナリオ**:
セキュリティ監査者が、本番環境で実行前に実際にどの環境変数が使用されるか確認したい。

**手順**:
1. 本番環境の設定ファイルを使用
2. `--dry-run --dry-run-detail=full`で実行
3. センシティブな値（APIキーなど）が正しく設定されているか確認

**期待結果**:
全ての環境変数の最終的な値と出所が表示され、意図しない値が使われていないことを確認できる。

## 制約事項

### 技術的制約

1. **既存コードの利用**: `debug.PrintFinalEnvironment`を最大限活用し、新規コード量を最小化
2. **出力先**: 標準出力（os.Stdout）に出力
3. **フォーマット**: テキスト形式のみ（JSON形式は将来的な拡張として検討）

### ビジネス制約

1. **後方互換性**: 既存のdry-run動作を変更しない
2. **デフォルト動作**: デフォルト（`--dry-run-detail=detailed`）では表示しない

## 成功基準

以下の基準を全て満たすこと:

1. **機能の完全性**:
   - [ ] `--dry-run-detail=full`で最終環境変数が表示される
   - [ ] 各環境変数の出所（Global/Group/Command/System）が正しく表示される
   - [ ] 60文字を超える値が適切に切り詰められる

2. **品質**:
   - [ ] 全ての単体テストがパス
   - [ ] 全ての統合テストがパス
   - [ ] lintエラーが0件

3. **ドキュメント**:
   - [ ] 要件定義書が作成されている（本ドキュメント）
   - [ ] アーキテクチャー設計書が作成されている
   - [ ] 実装計画書が作成されている

4. **パフォーマンス**:
   - [ ] 環境変数100個の表示が1ms以内に完了

## リスクと対策

### リスク1: パフォーマンスへの影響

**リスク**: 環境変数の表示処理がdry-runのパフォーマンスに影響を与える

**対策**:
- `DetailLevelFull`でのみ実行（デフォルトでは実行されない）
- 環境変数のソート処理を最適化
- ベンチマークテストで性能を測定

**影響度**: 低
**発生確率**: 低

### リスク2: センシティブデータの漏洩

**リスク**: 環境変数にセンシティブな値が含まれている場合、標準出力に表示される

**対策**:
- dry-runは実行前の確認・監査が目的であることをドキュメントで明記
- 本番環境では慎重に使用することを推奨
- 将来的に`--show-sensitive`フラグとの統合を検討

**影響度**: 中
**発生確率**: 中

### リスク3: 後方互換性の破損

**リスク**: 既存のdry-run出力パーサーが新しい出力フォーマットで動作しない

**対策**:
- `DetailLevelFull`のみで表示（既存の`detailed`には影響なし）
- 新しいセクションとして追加（既存セクションは変更しない）
- 統合テストで既存の出力フォーマットを検証

**影響度**: 低
**発生確率**: 低

## 将来的な拡張

本タスクでは実装しないが、将来的に検討すべき拡張機能:

1. **JSON形式での出力**: `--dry-run-format=json`でJSON形式をサポート
2. **環境変数のフィルタリング**: 特定の変数のみ表示するオプション
3. **変数の差分表示**: 前回のdry-run結果との差分を表示
4. **変数の依存関係グラフ**: 変数展開の依存関係を可視化
5. **--show-sensitive統合**: センシティブデータの表示制御を統一

## 参考資料

- debug.PrintFinalEnvironment実装
- [DryRunManager実装](../../../internal/runner/resource/dryrun_manager.go)
- [TextFormatter実装](../../../internal/runner/resource/formatter.go)
- [DetailLevel定義](../../../internal/runner/resource/types.go)
