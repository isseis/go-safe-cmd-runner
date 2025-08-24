# 要件定義書: Normal Mode リスクベースコマンド制御

## 1. 背景・課題

### 1.1 現在の状況
- **Dry-run mode**: セキュリティ分析（`security.AnalyzeCommandSecurity`）を実行し、危険なコマンドパターンを検出・分類
- **Normal execution mode**: セキュリティ分析を一切実行せず、コマンドを直接実行

### 1.2 特定された問題
- Normal modeでは以下の危険コマンドが無検証で実行される：
  - **High risk**: `rm -rf`, `format`, `mkfs`, `fdisk`, `dd if=`
  - **Medium risk**: `chmod 777`, `chown root`, `wget`, `curl`, `nc`, `netcat`
  - **特権昇格コマンド**: `sudo`, `su`, `doas`（一律禁止対象）
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
- `"critical"`: Critical risk コマンドまで許可（特権昇格コマンドを除く）
- `"high"`: High risk コマンドまで許可
- `"medium"`: Medium risk コマンドまで許可
- `"low"` または未設定: Low risk コマンドのみ許可

**F6**: コマンドの実際のリスクレベルが設定された `max_risk_level` を超える場合、実行を拒否

**F7**: 特権昇格コマンド（`sudo`, `su`, `doas`）は `max_risk_level` 設定に関わらず一律で実行を拒否
- 理由: `run_as_user`/`run_as_group` による安全な権限昇格メカニズムが既に提供されているため
- セキュリティ上の二重メカニズムを防ぎ、攻撃面を縮小

#### 2.2.3 拡張特権管理機能
**F8**: 詳細なユーザー・グループ指定機能
- `run_as_user`: 実行時のユーザー名またはUID指定（seteuid）
- `run_as_group`: 実行時のグループ名またはGID指定（setegid）
- rootのみでなく、任意のユーザー・グループでの実行を可能にする

**F9**: 最小権限の原則に基づく実行制御
- 必要最小限のユーザー・グループ権限での実行
- root権限が不要な場合の権限制限
- セキュリティ監査の強化

#### 2.2.4 エラーハンドリング
**F10**: 実行拒否時は明確なエラーメッセージを表示：
- 検出されたリスクレベル
- 検出された危険パターン
- 必要な設定方法

**F11**: セキュリティログに実行拒否の詳細を記録

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

### 3.4 特権コマンドの適切な使用方法
```toml
# ❌ 禁止: TOMLファイル内でのsudoコマンド
[[groups.commands]]
name = "privileged_task"
cmd = "sudo"
args = ["systemctl", "restart", "service"]
# このコマンドは実行前にリジェクトされる

# ✅ 推奨: run_as_user を使用
[[groups.commands]]
name = "restart_service"
cmd = "/bin/systemctl"
args = ["restart", "service"]
run_as_user = "root"
```

### 3.5 ユーザー/グループ指定付き実行
```toml
# ユーザー指定でのコマンド実行
[[groups.commands]]
name = "app_task"
cmd = "/app/bin/worker"
args = ["--process"]
run_as_user = "appuser"

# グループ指定でのコマンド実行
[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
run_as_group = "postgres"

# ユーザーとグループ両方を指定
[[groups.commands]]
name = "secure_task"
cmd = "/secure/bin/tool"
args = ["--config", "/etc/secure/config.json"]
run_as_user = "secure_user"
run_as_group = "secure_group"
max_risk_level = "medium"
name = "privileged_cleanup"  # このような設定は常に拒否される
cmd = "sudo"
args = ["rm", "-rf", "/tmp/files"]
max_risk_level = "high"  # この設定でも実行拒否
```

# ✅ 推奨: run_as_user による安全な権限昇格
```toml
[[groups.commands]]
name = "privileged_cleanup"
description = "Remove temporary files with elevated privileges"
cmd = "/bin/rm"
args = ["-rf", "/tmp/files"]
run_as_user = "root"  # 安全な権限昇格メカニズム
max_risk_level = "high"
```

## 4. 想定されるエラーメッセージ

### 4.1 一般的なリスクレベル超過
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

### 4.2 特権昇格コマンドの使用禁止
```
Error: privilege_escalation_prohibited - Privilege escalation commands are not allowed in TOML configuration
Details:
  Command: sudo rm -rf /tmp
  Detected Command: sudo
  Reason: Privilege escalation commands (sudo, su, doas) are prohibited in TOML files
  Alternative: Use 'run_as_user' setting for safe privilege escalation
  Command Path: groups.admin_tasks.commands.cleanup_temp
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
- [x] High risk コマンドが `max_risk_level = "high"` で実行可能（**実装完了** - Normal modeで完全実装）
- [x] High risk コマンドが `max_risk_level` 未設定で実行拒否（**実装完了** - Normal modeで完全実装）
- [x] Medium risk コマンドが `max_risk_level = "medium"` で実行可能（**実装完了** - Normal modeで完全実装）
- [x] Medium risk コマンドが `max_risk_level` 未設定で実行拒否（**実装完了** - Normal modeで完全実装）
- [x] 深いシンボリックリンクが適切に検出・拒否される
- [x] 安全なコマンドは設定なしで実行可能
- [x] **特権昇格コマンド（sudo, su, doas）が `max_risk_level` 設定に関わらず一律で実行拒否**（Critical riskとして分類され拒否）
- [x] **`run_as_user`/`run_as_group` 設定による安全な権限昇格が正常に動作**（設定構造体とDry-runで実装済み）

### 6.2 エラーハンドリング
- [x] 適切なエラーメッセージが表示される
- [x] セキュリティログが正しく出力される
- [x] 実行拒否後にプロセスが適切に終了する

### 6.3 互換性
- [x] 既存のTOMLファイルが変更なしで動作する
- [x] Dry-run mode の動作に影響しない
- [x] 既存のテストが全て通過する

## 7. 制約事項

### 7.1 設計制約
- 対話的な確認機能は実装しない
- 複雑なリスク評価ロジックは避け、既存のセキュリティ分析機能を活用
- 段階的導入機能（warn mode等）は実装しない

### 7.2 運用制約
- 設定ファイル作成者はセキュリティリスクを理解して `max_risk_level` を設定する責任を負う
- 危険コマンドの実行は明示的な設定が必要という原則を徹底
- **特権昇格が必要な場合は `run_as_user`/`run_as_group` 設定を使用し、TOMLファイル内での `sudo`/`su` コマンドは一切使用しない**

## 8. 今後の拡張可能性

### 8.1 将来的な機能拡張
- カスタム危険パターンの定義機能
- 実行時コンテキストを考慮したリスク評価
- 管理者承認ワークフローとの統合

### 8.2 監査・コンプライアンス
- セキュリティ違反の統計レポート
- コンプライアンス要件に対応した監査ログ
