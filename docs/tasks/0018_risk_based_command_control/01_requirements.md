# 要件定義書: Normal Mode リスクベースコマンド制御

## 1. 背景・課題

### 1.1 現在の状況
- **Dry-run mode**: セキュリティ分析（`security.AnalyzeCommandSecurity`）を実行し、危険なコマンドパターンを検出・分類
- **Normal execution mode**: セキュリティ分析を一切実行せず、コマンドを直接実行

### 1.2 特定された問題
- Normal modeでは以下の危険コマンドが無検証で実行される：
  - **High risk**: `rm -rf`, `sudo rm`, `format`, `mkfs`, `fdisk`, `dd if=`
  - **Medium risk**: `chmod 777`, `chown root`, `wget`, `curl`, `nc`, `netcat`
  - **深いシンボリックリンク攻撃**: 40段階超えのシンボリックリンクチェーン

### 1.3 セキュリティリスク
- 設定ファイル内の危険コマンドが警告なしに実行される
- システム破壊、データ消失、不正アクセスの可能性
- 監査証跡の不足

## 2. 要件概要

### 2.1 基本要件
**R1**: Normal execution modeにセキュリティ分析を統合し、dry-run modeと同等のセキュリティ検査を実行する

**R2**: High risk または Medium risk と判定されたコマンドは、明示的な許可設定なしに実行を拒否する

**R3**: バッチ処理環境での使用を前提とし、対話的な確認は行わない

### 2.2 機能要件

#### 2.2.1 セキュリティ分析統合
**F1**: Normal mode の `ExecuteCommand` 実行前に `security.AnalyzeCommandSecurity` を呼び出す

**F2**: シンボリックリンク深度チェック（MaxSymlinkDepth=40）を実行し、超過時は RiskLevelHigh として処理

**F3**: 既存の危険コマンドパターンマッチング機能を活用

#### 2.2.2 許可設定機能
**F4**: TOML設定ファイルの `groups.commands` セクションに `max_risk_level` フィールドを追加

**F5**: `max_risk_level` の値：
- `"high"`: High risk コマンドまで許可
- `"medium"`: Medium risk コマンドまで許可
- `"none"` または未設定: リスクのないコマンドのみ許可

**F6**: コマンドの実際のリスクレベルが設定された `max_risk_level` を超える場合、実行を拒否
ただし `privileged = true` が設定されている場合
特権コマンドの実行は明示的に許可されているものと見なす

#### 2.2.3 エラーハンドリング
**F7**: 実行拒否時は明確なエラーメッセージを表示：
- 検出されたリスクレベル
- 検出された危険パターン
- 必要な設定方法

**F8**: セキュリティログに実行拒否の詳細を記録

### 2.3 非機能要件

#### 2.3.1 互換性
**NF1**: 既存のTOMLファイルは `max_risk_level` 未設定でも動作する（デフォルト: リスクなしコマンドのみ）

**NF2**: Dry-run mode の動作に影響しない

#### 2.3.2 性能
**NF3**: セキュリティ分析による実行時間の増加は最小限に抑える

#### 2.3.3 保守性
**NF4**: 新しい危険パターンの追加が容易な設計を維持

## 3. 設定例

### 3.1 High risk コマンドの許可
```toml
[[groups.commands]]
name = "controlled_cleanup"
description = "Remove temporary files in controlled manner"
cmd = "rm"
args = ["-rf", "/tmp/app_temp_files"]
max_risk_level = "high"
```

### 3.2 Medium risk コマンドの許可
```toml
[[groups.commands]]
name = "download_data"
description = "Download configuration data from trusted source"
cmd = "wget"
args = ["https://trusted.example.com/config.json"]
max_risk_level = "medium"
```

### 3.3 安全なコマンド（設定不要）
```toml
[[groups.commands]]
name = "display_info"
description = "Display system information"
cmd = "echo"
args = ["System ready"]
# max_risk_level 未設定でも実行可能
```

## 4. 想定されるエラーメッセージ

```
Error: command_security_violation - Command execution blocked due to security risk
Details:
  Command: rm -rf /important/data
  Detected Risk Level: HIGH
  Detected Pattern: Recursive file removal
  Required Setting: Add 'max_risk_level = "high"' to command configuration
  Command Path: groups.basic_tests.commands.dangerous_cleanup
Run ID: 01K35WM4J8BBX09DY348H7JDEX
```

## 5. 実装対象範囲

### 5.1 修正対象ファイル
- `internal/runner/resource/normal_manager.go`: セキュリティ分析統合
- `internal/runner/config/config.go`: max_risk_level フィールド追加
- `internal/runner/config/validator.go`: 設定値検証
- 関連テストファイル

### 5.2 新規追加予定
- セキュリティ違反時の専用エラー型
- 設定値検証ロジック
- 統合テストケース

## 6. 受け入れ条件

### 6.1 機能テスト
- [ ] High risk コマンドが `max_risk_level = "high"` で実行可能
- [ ] High risk コマンドが `max_risk_level` 未設定で実行拒否
- [ ] Medium risk コマンドが `max_risk_level = "medium"` で実行可能
- [ ] Medium risk コマンドが `max_risk_level` 未設定で実行拒否
- [ ] 深いシンボリックリンクが適切に検出・拒否される
- [ ] 安全なコマンドは設定なしで実行可能

### 6.2 エラーハンドリング
- [ ] 適切なエラーメッセージが表示される
- [ ] セキュリティログが正しく出力される
- [ ] 実行拒否後にプロセスが適切に終了する

### 6.3 互換性
- [ ] 既存のTOMLファイルが変更なしで動作する
- [ ] Dry-run mode の動作に影響しない
- [ ] 既存のテストが全て通過する

## 7. 制約事項

### 7.1 設計制約
- 対話的な確認機能は実装しない
- 複雑なリスク評価ロジックは避け、既存のセキュリティ分析機能を活用
- 段階的導入機能（warn mode等）は実装しない

### 7.2 運用制約
- 設定ファイル作成者はセキュリティリスクを理解して `max_risk_level` を設定する責任を負う
- 危険コマンドの実行は明示的な設定が必要という原則を徹底

## 8. 今後の拡張可能性

### 8.1 将来的な機能拡張
- カスタム危険パターンの定義機能
- 実行時コンテキストを考慮したリスク評価
- 管理者承認ワークフローとの統合

### 8.2 監査・コンプライアンス
- セキュリティ違反の統計レポート
- コンプライアンス要件に対応した監査ログ
