# go-safe-cmd-runner ユーザーガイド

go-safe-cmd-runner のユーザー向けドキュメントへようこそ。このガイドでは、コマンドラインツールの使用方法、設定ファイルの記述方法、セキュリティに関する情報を提供します。

## クイックナビゲーション

### 🚀 初めての方へ

初めて go-safe-cmd-runner を使用する場合は、以下の順序でドキュメントを読むことをお勧めします：

1. [プロジェクトREADME](../../README.ja.md) - 概要とセキュリティ機能
2. [runner コマンドガイド](#コマンドラインツール) - メインの実行コマンド
3. [TOML設定ファイルガイド](#設定ファイル) - 設定ファイルの記述方法
4. [record コマンドガイド](#コマンドラインツール) - ハッシュファイルの作成

### 📚 ドキュメント一覧

## コマンドラインツール

go-safe-cmd-runner は3つのコマンドラインツールを提供します。

### [runner コマンド](runner_command.ja.md) ⭐ 必読

メインの実行コマンド。TOML設定ファイルに基づいてコマンドを安全に実行します。

**主な機能:**
- セキュアなバッチ処理
- ドライラン機能
- リスクベースのセキュリティ制御
- 詳細なロギング
- カラー出力対応

**クイックスタート:**
```bash
# 基本的な実行
runner -config config.toml

# ドライラン（実行内容の確認）
runner -config config.toml -dry-run

# 設定ファイルの検証
runner -config config.toml -validate
```

**こんな時に:**
- コマンドを実行したい
- 設定ファイルを検証したい
- 実行前に動作を確認したい

[詳細はこちら →](runner_command.ja.md)

---

### [record コマンド](record_command.ja.md)

ファイルのSHA-256ハッシュ値を記録するコマンド。管理者向け。

**主な機能:**
- ファイル整合性のベースライン作成
- ハッシュファイルの管理
- 複数ファイルの一括記録対応

**クイックスタート:**
```bash
# ハッシュを記録
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 既存のハッシュを上書き
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

**こんな時に:**
- 初期セットアップ時
- ファイルを更新した後
- システムパッケージ更新後

[詳細はこちら →](record_command.ja.md)

---

### [verify コマンド](verify_command.ja.md)

ファイルの整合性を検証するコマンド。デバッグ・トラブルシューティング用。

**主な機能:**
- 個別ファイルの整合性確認
- 検証エラーの詳細調査
- バッチ検証対応

**クイックスタート:**
```bash
# ファイルを検証
verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 複数ファイルの検証
for file in /usr/local/bin/*.sh; do
    verify -file "$file" -hash-dir /path/to/hashes
done
```

**こんな時に:**
- 検証エラーの原因調査
- runner 実行前の事前確認
- 定期的な整合性チェック

[詳細はこちら →](verify_command.ja.md)

---

## 設定ファイル

### [TOML設定ファイル ユーザーガイド](toml_config/README.ja.md) ⭐ 必読

runner コマンドで使用する設定ファイルの詳細な記述方法を解説します。

**章立て:**

1. **[はじめに](toml_config/01_introduction.ja.md)**
   - TOML設定ファイルの概要
   - 基本構造

2. **[設定ファイルの階層構造](toml_config/02_hierarchy.ja.md)**
   - ルート・グローバル・グループ・コマンドレベル
   - 継承とオーバーライド

3. **[ルートレベル設定](toml_config/03_root_level.ja.md)**
   - `version` パラメータ

4. **[グローバルレベル設定](toml_config/04_global_level.ja.md)**
   - タイムアウト、ログレベル、環境変数許可リストなど
   - 全グループに適用されるデフォルト設定

5. **[グループレベル設定](toml_config/05_group_level.ja.md)**
   - グループ単位でのコマンド管理
   - リソース管理とセキュリティ設定

6. **[コマンドレベル設定](toml_config/06_command_level.ja.md)**
   - 個別コマンドの詳細設定
   - 実行ユーザー、リスクレベル、出力管理

7. **[変数展開機能](toml_config/07_variable_expansion.ja.md)**
   - `${VAR}` 形式の変数展開
   - 動的な設定構築

8. **[実践的な設定例](toml_config/08_practical_examples.ja.md)**
   - バックアップ、デプロイ、メンテナンスなどの実例

9. **[ベストプラクティス](toml_config/09_best_practices.ja.md)**
   - セキュリティ、保守性、パフォーマンスの向上

10. **[トラブルシューティング](toml_config/10_troubleshooting.ja.md)**
    - よくあるエラーと対処法

**クイックスタート:**
```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "backup"
description = "Database backup"

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
output = "backup.sql"
run_as_user = "postgres"
max_risk_level = "medium"
```

[詳細はこちら →](toml_config/README.ja.md)

---

## セキュリティ

### [セキュリティリスク評価](security-risk-assessment.ja.md)

コマンドのリスクレベルと評価基準について解説します。

**内容:**
- リスクレベルの定義（low, medium, high, critical）
- コマンドごとのリスク評価
- リスクベースの制御方法

**リスクレベル:**
- **Low**: 基本的な読み取り操作（ls, cat, grep）
- **Medium**: ファイル変更、パッケージ管理（cp, mv, apt）
- **High**: システム管理、破壊的操作（systemctl, rm -rf）
- **Critical**: 権限昇格（sudo, su）- 常にブロック

[詳細はこちら →](security-risk-assessment.ja.md)

---

## 実践的なワークフロー

### 典型的な使用フロー

```
1. 設定ファイルを作成
   └─ TOML設定ファイルガイドを参照

2. ハッシュ値を記録
   └─ record コマンドで実行ファイルと設定ファイルのハッシュを記録

3. 設定を検証
   └─ runner -config config.toml -validate

4. ドライランで確認
   └─ runner -config config.toml -dry-run

5. 本番実行
   └─ runner -config config.toml

6. トラブルシューティング（必要に応じて）
   └─ verify コマンドでファイル整合性を確認
```

### 初回セットアップ例

```bash
# 1. 設定ファイルを作成
cat > /etc/go-safe-cmd-runner/backup.toml << 'EOF'
version = "1.0"

[global]
timeout = 3600
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "backup"

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["-U", "postgres", "mydb"]
output = "/var/backups/db.sql"
run_as_user = "postgres"
EOF

# 2. ハッシュディレクトリを作成
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# 3. ハッシュを記録
sudo record -file /etc/go-safe-cmd-runner/backup.toml \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

sudo record -file /usr/bin/pg_dump \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 4. 設定を検証
runner -config /etc/go-safe-cmd-runner/backup.toml -validate

# 5. ドライランで確認
runner -config /etc/go-safe-cmd-runner/backup.toml -dry-run

# 6. 本番実行
runner -config /etc/go-safe-cmd-runner/backup.toml
```

---

## よくある質問（FAQ）

### Q: どのコマンドから使い始めればいいですか？

A: まず [runner コマンド](runner_command.ja.md) と [TOML設定ファイルガイド](toml_config/README.ja.md) を読むことをお勧めします。この2つでほとんどの使用ケースをカバーできます。

### Q: 設定ファイルのサンプルはありますか？

A: はい、プロジェクトの `sample/` ディレクトリに多数のサンプルがあります：
- `sample/comprehensive.toml` - 全機能を網羅
- `sample/variable_expansion_basic.toml` - 変数展開の基本
- `sample/output_capture_basic.toml` - 出力キャプチャの基本

詳細は [TOML設定ファイルガイド](toml_config/README.ja.md) を参照してください。

### Q: エラーが発生した場合はどうすればいいですか？

A: 以下の順序で確認してください：

1. **設定検証**: `runner -config config.toml -validate`
2. **ファイル検証**: `verify -file <path> -hash-dir <hash-dir>`
3. **デバッグログ**: `runner -config config.toml -log-level debug`
4. **トラブルシューティングガイド**:
   - [runner のトラブルシューティング](runner_command.ja.md#6-トラブルシューティング)
   - [TOML設定のトラブルシューティング](toml_config/10_troubleshooting.ja.md)

### Q: CI/CD環境で使用できますか？

A: はい、CI/CD環境に最適化されています。詳細は以下を参照：
- [runner コマンド - CI/CD環境での使用](runner_command.ja.md#55-cicd環境での使用)
- 環境変数による自動検出（CI, GITHUB_ACTIONS, JENKINS_URL など）
- `-quiet` フラグによる非インタラクティブモード

### Q: セキュリティ上の注意点は？

A: 主な注意点：
- 設定ファイルと実行バイナリは必ずハッシュ値を記録してください
- 環境変数は必要最小限のみ許可リストに追加してください
- リスクレベルを適切に設定してください
- 詳細は [セキュリティリスク評価](security-risk-assessment.ja.md) を参照

---

## 推奨される学習パス

### 🎯 初心者向け（1-2時間）

1. [プロジェクトREADME](../../README.ja.md) - 全体概要（15分）
2. [runner コマンド - 概要とクイックスタート](runner_command.ja.md#1-概要) - 基本操作（30分）
3. [TOML設定 - はじめに](toml_config/01_introduction.ja.md) - 設定の基本（15分）
4. [TOML設定 - 実践例](toml_config/08_practical_examples.ja.md) - サンプルで学習（30分）

### 🎓 中級者向け（3-4時間）

上記に加えて：

5. [runner コマンド - 全フラグ詳解](runner_command.ja.md#3-コマンドラインフラグ詳解) - 詳細オプション（1時間）
6. [TOML設定 - グローバル/グループ/コマンドレベル](toml_config/04_global_level.ja.md) - 階層的設定（1時間）
7. [TOML設定 - 変数展開](toml_config/07_variable_expansion.ja.md) - 高度な機能（30分）
8. [record/verify コマンド](record_command.ja.md) - ハッシュ管理（30分）

### 🚀 上級者向け（フル習得）

上記に加えて：

9. [TOML設定 - ベストプラクティス](toml_config/09_best_practices.ja.md) - 設計パターン
10. [セキュリティリスク評価](security-risk-assessment.ja.md) - セキュリティモデル
11. [開発者向けドキュメント](../dev/) - アーキテクチャとセキュリティ設計
12. [トラブルシューティング](toml_config/10_troubleshooting.ja.md) - 問題解決スキル

---

## その他のリソース

### プロジェクト情報

- [プロジェクトREADME](../../README.ja.md) - 概要、セキュリティ機能、インストール方法
- [GitHub リポジトリ](https://github.com/isseis/go-safe-cmd-runner/) - ソースコード、Issue、PR
- [LICENSE](../../LICENSE) - ライセンス情報

### 開発者向け

- [開発者向けドキュメント](../dev/) - アーキテクチャ、セキュリティ設計、開発ガイドライン
- [タスクドキュメント](../tasks/) - 開発タスクの要件定義と実装計画

### コミュニティ

- [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues) - バグ報告、機能要望、質問、アイデア共有

---

## ドキュメントへの貢献

ドキュメントの改善提案や誤りの指摘は歓迎します。以下の方法で貢献できます：

1. **Issueを作成**: [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues)
2. **Pull Requestを送信**: ドキュメントの修正や追加
3. **フィードバック**: 使いにくい点や不明瞭な説明を報告

 ドキュメント作成ガイドラインは [CLAUDE.md](../../CLAUDE.md) を参照してください。

---

**最終更新**: 2025-10-02
**バージョン**: 1.0
