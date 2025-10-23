# TOML設定フィールド名の移行ガイド

## 概要

このガイドでは、TOML設定ファイルのフィールド名変更に伴う移行手順を説明します。

### Breaking Change の背景

TOML設定ファイルのフィールド名を改善し、より分かりやすく一貫性のある命名に変更しました。この変更により、設定ファイルの可読性と保守性が向上します。

### 影響範囲

すべての既存TOML設定ファイルが影響を受けます。**手動での移行が必要**です。

## フィールド名変更一覧

### Global レベル

| 旧フィールド名 | 新フィールド名 | デフォルト値の変更 | 備考 |
|---------------|---------------|------------------|------|
| `skip_standard_paths` | `verify_standard_paths` | 動作は変更なし | 否定形から肯定形に変更 |
| `env` | `env_vars` | なし | 環境変数プレフィックス統一 |
| `env_allowlist` | `env_allowed` | なし | より短縮された名前 |
| `from_env` | `env_import` | なし | 環境変数プレフィックス統一 |
| `max_output_size` | `output_size_limit` | なし | より自然な語順 |

### Group レベル

| 旧フィールド名 | 新フィールド名 | デフォルト値の変更 | 備考 |
|---------------|---------------|------------------|------|
| `env` | `env_vars` | なし | 環境変数プレフィックス統一 |
| `env_allowlist` | `env_allowed` | なし | より短縮された名前 |
| `from_env` | `env_import` | なし | 環境変数プレフィックス統一 |

### Command レベル

| 旧フィールド名 | 新フィールド名 | デフォルト値の変更 | 備考 |
|---------------|---------------|------------------|------|
| `env` | `env_vars` | なし | 環境変数プレフィックス統一 |
| `from_env` | `env_import` | なし | 環境変数プレフィックス統一 |
| `max_risk_level` | `risk_level` | なし | より簡潔な名前 |
| `output` | `output_file` | なし | より明確な名前 |

## 重要な変更点

### 1. `skip_standard_paths` → `verify_standard_paths`

**最も重要な変更**: 否定形から肯定形への変更

#### 変更前
```toml
[global]
skip_standard_paths = false  # 標準パスを検証する（デフォルト）
skip_standard_paths = true   # 標準パスの検証をスキップ
```

#### 変更後
```toml
[global]
verify_standard_paths = true   # 標準パスを検証する（デフォルト）
verify_standard_paths = false  # 標準パスの検証をスキップ
```

#### デフォルト動作
- **動作は変更ありません**: デフォルトでは標準パスの検証が実行されます
- **フィールド名が明確になりました**: 何をするかが直感的に理解できます

### 2. 環境変数関連フィールドの統一

すべての環境変数関連フィールドが `env_` プレフィックスで統一されました：

```toml
# 変更前
env = ["VAR1", "VAR2"]
env_allowlist = ["ALLOWED_VAR"]
from_env = ["IMPORTED_VAR"]

# 変更後
env_vars = ["VAR1", "VAR2"]
env_allowed = ["ALLOWED_VAR"]
env_import = ["IMPORTED_VAR"]
```

## 移行手順

### 手順1: バックアップの作成

移行前に既存の設定ファイルをバックアップしてください：

```bash
# 単一ファイルの場合
cp config.toml config.toml.backup

# 複数ファイルの場合
find . -name "*.toml" -exec cp {} {}.backup \;
```

### 手順2: sed による一括置換

以下のsedコマンドで一括置換できます：

```bash
# Global レベルの置換
sed -i 's/skip_standard_paths = false/verify_standard_paths = true/g' *.toml
sed -i 's/skip_standard_paths = true/verify_standard_paths = false/g' *.toml
sed -i 's/^env = /env_vars = /g' *.toml
sed -i 's/^env_allowlist = /env_allowed = /g' *.toml
sed -i 's/^from_env = /env_import = /g' *.toml
sed -i 's/^max_output_size = /output_size_limit = /g' *.toml

# Group/Command レベルの置換
sed -i 's/max_risk_level = /risk_level = /g' *.toml
sed -i 's/^output = /output_file = /g' *.toml
```

### 手順3: 手動確認

sed による一括置換後、以下を手動で確認してください：

1. **`skip_standard_paths`の値の反転**:
   - `skip_standard_paths = false` → `verify_standard_paths = true`
   - `skip_standard_paths = true` → `verify_standard_paths = false`

2. **コメント内のフィールド名**: sedはコメント内のフィールド名を変更しない場合があります

3. **インデントや空白**: TOML構造が正しく保持されているかの確認

### 手順4: 設定ファイルの検証

移行後、設定ファイルが正しく動作することを確認：

```bash
# Dry-run での動作確認
./build/runner --config config.toml --dry-run

# 構文エラーの確認
./build/runner --config config.toml --validate-only
```

## 移行例

### 完全な移行例

#### 変更前
```toml
[global]
skip_standard_paths = false
env = ["GLOBAL_VAR"]
env_allowlist = ["ALLOWED_*"]
from_env = ["PATH"]
max_output_size = 1024

[[groups]]
name = "example"
env = ["GROUP_VAR"]
env_allowlist = ["GROUP_*"]
from_env = ["HOME"]

  [[groups.commands]]
  name = "test"
  cmd = ["echo", "test"]
  env = ["CMD_VAR"]
  from_env = ["USER"]
  max_risk_level = "medium"
  output = "test.log"
```

#### 変更後
```toml
[global]
verify_standard_paths = true
env_vars = ["GLOBAL_VAR"]
env_allowed = ["ALLOWED_*"]
env_import = ["PATH"]
output_size_limit = 1024

[[groups]]
name = "example"
env_vars = ["GROUP_VAR"]
env_allowed = ["GROUP_*"]
env_import = ["HOME"]

  [[groups.commands]]
  name = "test"
  cmd = ["echo", "test"]
  env_vars = ["CMD_VAR"]
  env_import = ["USER"]
  risk_level = "medium"
  output_file = "test.log"
```

## よくある質問（FAQ）

### Q1: デフォルト動作は変わりますか？

A1: いいえ、デフォルト動作は変更ありません。`skip_standard_paths`のデフォルトは`false`（検証実行）でしたが、`verify_standard_paths`のデフォルトは`true`（検証実行）です。

### Q2: 古いフィールド名を使うとどうなりますか？

A2: エラーになります。新しいバージョンでは古いフィールド名は認識されません。

### Q3: 段階的に移行できますか？

A3: いいえ、この変更はBreaking Changeのため、すべてのフィールドを一度に移行する必要があります。

### Q4: 移行支援ツールはありますか？

A4: 現在、専用の移行ツールはありません。このガイドのsedコマンドを使用してください。

### Q5: 設定ファイルが大量にある場合は？

A5: 以下のように複数ファイルを一括処理できます：

```bash
# 特定ディレクトリ内のすべての.tomlファイル
find /path/to/configs -name "*.toml" -exec sed -i 's/old/new/g' {} \;

# 再帰的に処理
find . -name "*.toml" -type f -exec sed -i 's/old/new/g' {} \;
```

## トラブルシューティング

### 問題: 設定ファイルの読み込みエラー

**症状**:
```
Error: unknown field 'env' in TOML
```

**解決方法**:
古いフィールド名が残っています。このガイドの移行手順を再度実行してください。

### 問題: 値の反転忘れ

**症状**:
標準パスの検証動作が期待と逆になる

**解決方法**:
`skip_standard_paths`と`verify_standard_paths`の値の関係を確認：
- `skip_standard_paths = false` → `verify_standard_paths = true`
- `skip_standard_paths = true` → `verify_standard_paths = false`

### 問題: 一部のファイルが移行されていない

**解決方法**:
以下のコマンドで古いフィールド名が残っているファイルを検索：

```bash
# 古いフィールド名を含むファイルを検索
grep -r "skip_standard_paths\|^env =\|env_allowlist\|from_env\|max_output_size\|max_risk_level\|^output =" *.toml
```

## サポート

移行に関して問題が発生した場合は、以下のリソースを参照してください：

- [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues)
- [ユーザーガイド](../user/)
- [設定リファレンス](../user/toml_config/)
