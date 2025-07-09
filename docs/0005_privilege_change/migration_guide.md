# 移行ガイド: 特権実行設定の統一化

## 概要

go-safe-cmd-runnerのバージョンX.X.Xから、コマンドの権限制御設定が統一されました。この変更により：

- **削除**: `user`フィールドが削除されました
- **統一**: `privileged`フィールドで権限制御を統一
- **後方互換性なし**: 古い形式の設定ファイルはサポートされません

## 変更内容

### 設定構造体の変更

#### 変更前
```toml
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  user = "root"        # 削除されたフィールド
  privileged = true
```

#### 変更後
```toml
[[groups.commands]]
  name = "system_command"
  cmd = "systemctl"
  args = ["restart", "service"]
  privileged = true    # 統一的な権限制御
```

## 手動移行（必須）

### 移行ルール

| 旧設定 | 新設定 | 変換方法 |
|--------|--------|----------|
| `user = "root"` | `privileged = true` | 特権実行に変換 |
| `user = "other"` | 削除 | userフィールドを削除 |
| `user = ""` | 削除 | userフィールドを削除 |
| `privileged = true/false` | `privileged = true/false` | 変更なし |

### 重要な注意事項

**古い形式の設定ファイルはサポートされません。** `user`フィールドが含まれている設定ファイルは設定読み込み時にエラーとなります。

## 移行手順

以下の手順で設定ファイルを更新してください：

### ステップ1: 設定ファイルのバックアップ
```bash
cp config.toml config.toml.backup
```

### ステップ2: userフィールドの確認
```bash
grep -n "user.*=" config.toml
```

### ステップ3: 変更の実施

#### 例1: user="root"の場合
```toml
# 変更前
[[groups.commands]]
  name = "system_restart"
  cmd = "systemctl"
  args = ["restart", "myapp"]
  user = "root"
  privileged = false

# 変更後
[[groups.commands]]
  name = "system_restart"
  cmd = "systemctl"
  args = ["restart", "myapp"]
  privileged = true  # user="root" → privileged=true
```

#### 例2: user="other"の場合
```toml
# 変更前
[[groups.commands]]
  name = "deploy_app"
  cmd = "rsync"
  args = ["-av", "dist/", "/var/app/"]
  user = "deploy"
  privileged = false

# 変更後
[[groups.commands]]
  name = "deploy_app"
  cmd = "rsync"
  args = ["-av", "dist/", "/var/app/"]
  privileged = false  # userフィールドを削除
```

#### 例3: user=""の場合
```toml
# 変更前
[[groups.commands]]
  name = "build_app"
  cmd = "npm"
  args = ["run", "build"]
  user = ""
  privileged = false

# 変更後
[[groups.commands]]
  name = "build_app"
  cmd = "npm"
  args = ["run", "build"]
  privileged = false  # userフィールドを削除
```

### ステップ4: 設定の検証
```bash
# ドライランで設定ファイルの検証
./cmd/runner/main -config config.toml -dry-run

# エラーがないことを確認
echo $?  # 0であることを確認
```

## 移行の確認

### 設定ファイルの妥当性確認
```bash
# 設定ファイルの構文チェック
./cmd/runner/main -config config.toml -dry-run

# 成功例:
# [DRY RUN] Would execute the following groups:
# ...

# エラー例（userフィールドが含まれている場合）:
# Error: failed to parse config: ...
```

## よくある質問

### Q: 既存の設定ファイルはそのまま使用できますか？
A: いいえ、`user`フィールドが含まれている設定ファイルはエラーとなります。手動での移行が必須です。

### Q: user="root"以外の設定はどうなりますか？
A: 設定ファイル内のすべての`user`フィールドを削除する必要があります。

### Q: privilegedフィールドはいつ実装されますか？
A: Phase 3で実装予定です。現在は設定として受け入れられますが、実際の特権昇格は行われません。

### Q: 移行後にエラーが発生した場合は？
A: バックアップした設定ファイルに戻し、手動移行の手順を再確認してください。

## トラブルシューティング

### 設定ファイルの構文エラー
```bash
# TOML構文の検証
go run -c 'import "github.com/pelletier/go-toml/v2"; toml.Load("config.toml")'
```

### userフィールドが残っている場合
```bash
# userフィールドの確認
grep -n "user.*=" config.toml

# 見つかった場合は手動で削除
```

### 権限エラー
移行後も権限エラーが発生する場合：
1. 現在の実装では特権昇格は行われません
2. 手動でsudoを使用するか、Phase 3の実装を待ってください

## 関連ドキュメント

- [要件定義書](01_requirements.md)
- [アーキテクチャ設計書](02_architecture.md)
- [詳細仕様書](03_specification.md)
- [実装計画書](04_implementation_plan.md)

## サポート

移行に関する問題や質問がある場合は、以下のチャンネルでサポートを受けることができます：

- GitHub Issues: プロジェクトリポジトリ
- ドキュメント: `docs/0005_privilege_change/`ディレクトリ
- ログ出力: `--log-level=debug`オプションで詳細な情報を確認
