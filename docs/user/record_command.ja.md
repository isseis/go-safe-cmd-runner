# record コマンド ユーザーガイド

ファイルのSHA-256ハッシュ値を記録するための `record` コマンドの使用方法を解説します。

## 目次

- [1. 概要](#1-概要)
- [2. 基本的な使い方](#2-基本的な使い方)
- [3. コマンドラインフラグ詳解](#3-コマンドラインフラグ詳解)
- [4. 実践例](#4-実践例)
- [5. トラブルシューティング](#5-トラブルシューティング)
- [6. 関連ドキュメント](#6-関連ドキュメント)

## 1. 概要

### 1.1 record コマンドとは

`record` コマンドは、ファイルのSHA-256ハッシュ値を計算し、ハッシュディレクトリに保存します。このハッシュ値は、後で `runner` コマンドや `verify` コマンドによってファイルの整合性を検証するために使用されます。

### 1.2 主な用途

- **セキュリティ**: 実行バイナリやスクリプトの改ざん検出
- **整合性保証**: 設定ファイルや環境ファイルの変更検出
- **監査**: ファイルのバージョン管理と追跡

### 1.3 動作の仕組み

```
1. ファイルのSHA-256ハッシュ値を計算
   ↓
2. ファイルパスをエンコードしてハッシュファイル名を生成
   ↓
3. ハッシュ値をハッシュディレクトリに保存
   ↓
4. 保存されたハッシュファイル名を表示
```

### 1.4 ハッシュファイルの命名規則

record コマンドは、ハイブリッドエンコーディング方式を使用してハッシュファイル名を生成します：

**短いパスの場合（置換エンコーディング）**

```
/usr/bin/backup.sh → ~usr~bin~backup.sh
/etc/config.toml   → ~etc~config.toml
```

**長いパスの場合（SHA-256フォールバック）**

```
/very/long/path/to/file.sh → AbCdEf123456.json
```

この方式により、ハッシュファイル名が人間に読みやすく、かつファイル名の長さ制限にも対応しています。

### 1.5 使用場面

- **初期セットアップ**: システム導入時に実行ファイルのハッシュを記録
- **ファイル更新後**: スクリプトや設定ファイルを更新した後にハッシュを再記録
- **定期更新**: システムパッケージ更新後にハッシュを更新

## 2. 基本的な使い方

### 2.1 最もシンプルな使用例

```bash
# カレントディレクトリにハッシュを記録
record -file /usr/bin/backup.sh
```

実行結果：
```
Recorded hash for /usr/bin/backup.sh in /home/user/~usr~bin~backup.sh
```

### 2.2 ハッシュディレクトリを指定

```bash
# 特定のディレクトリにハッシュを記録
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

実行結果：
```
Recorded hash for /usr/bin/backup.sh in /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

### 2.3 既存のハッシュを上書き

```bash
# 既存のハッシュファイルを強制的に上書き
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force
```

### 2.4 複数ファイルの一括記録

```bash
# スクリプトで複数ファイルを記録
for file in /usr/local/bin/*.sh; do
    record -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
done
```

## 3. コマンドラインフラグ詳解

### 3.1 `-file <path>` (必須)

**概要**

ハッシュ値を記録するファイルのパスを指定します。

**文法**

```bash
record -file <path>
```

**パラメータ**

- `<path>`: ハッシュを記録したいファイルへの絶対パスまたは相対パス（必須）

**使用例**

```bash
# 絶対パスで指定
record -file /usr/bin/backup.sh

# 相対パスで指定
record -file ./scripts/deploy.sh

# ホームディレクトリのファイル
record -file ~/bin/custom-script.sh
```

**注意事項**

- ファイルが存在しない場合はエラーになります
- シンボリックリンクの場合、リンク先のファイルのハッシュが記録されます
- ディレクトリは指定できません（ファイルのみ）

### 3.2 `-hash-dir <directory>` (オプション)

**概要**

ハッシュファイルを保存するディレクトリを指定します。指定しない場合はカレントディレクトリが使用されます。

**文法**

```bash
record -file <path> -hash-dir <directory>
```

**パラメータ**

- `<directory>`: ハッシュファイルを保存するディレクトリパス（省略可能）
- デフォルト: カレントディレクトリ

**使用例**

```bash
# 標準のハッシュディレクトリに保存
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# カスタムディレクトリに保存（テスト用）
record -file ./test.sh -hash-dir ./test-hashes

# 相対パスで指定
record -file /etc/config.toml -hash-dir ../hashes
```

**ディレクトリの自動作成**

指定したディレクトリが存在しない場合、自動的に作成されます（権限: 0750）。

```bash
# ディレクトリが存在しない場合でもOK
record -file /usr/bin/backup.sh -hash-dir /new/hash/directory
# /new/hash/directory が自動的に作成されます
```

**権限について**

- ハッシュディレクトリは 0750 権限で作成されます（所有者: rwx, グループ: r-x, その他: ---）
- ハッシュファイルは 0640 権限で作成されます（所有者: rw-, グループ: r--, その他: ---）

**本番環境での推奨設定**

```bash
# 本番環境では標準ディレクトリを使用
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# ハッシュを記録
sudo record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 3.3 `-force` (オプション)

**概要**

既存のハッシュファイルを強制的に上書きします。指定しない場合、既存のハッシュファイルが存在するとエラーになります。

**文法**

```bash
record -file <path> -hash-dir <directory> -force
```

**使用例**

**通常の動作（既存ファイルがあるとエラー）**

```bash
# 1回目は成功
record -file /usr/bin/backup.sh -hash-dir ./hashes

# 2回目はエラー
record -file /usr/bin/backup.sh -hash-dir ./hashes
# Error: hash file already exists: ./hashes/~usr~bin~backup.sh
```

**-force フラグを使用**

```bash
# 既存のハッシュファイルを上書き
record -file /usr/bin/backup.sh -hash-dir ./hashes -force
# Recorded hash for /usr/bin/backup.sh in ./hashes/~usr~bin~backup.sh
```

**ユースケース**

- **ファイル更新後**: スクリプトやバイナリを更新した後、ハッシュを再記録
- **強制再同期**: ハッシュファイルが破損または誤って削除された場合の復旧
- **バッチ更新**: 複数ファイルのハッシュを一括更新するスクリプト

**使用例：バッチ更新**

```bash
# 全スクリプトのハッシュを強制的に再記録
for file in /usr/local/bin/*.sh; do
    record -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force
done
```

**注意事項**

- `-force` フラグは既存のハッシュファイルを警告なしで上書きします
- 誤って重要なハッシュファイルを上書きしないよう注意してください
- 本番環境では、バックアップを取ってから使用することを推奨します

## 4. 実践例

### 4.1 初期セットアップ

**システム導入時のハッシュ記録**

```bash
#!/bin/bash
# setup-hashes.sh - 初期ハッシュ記録スクリプト

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

# ハッシュディレクトリの作成
sudo mkdir -p "$HASH_DIR"
sudo chown root:root "$HASH_DIR"
sudo chmod 755 "$HASH_DIR"

# 設定ファイルのハッシュを記録
echo "Recording configuration files..."
sudo record -file /etc/go-safe-cmd-runner/backup.toml -hash-dir "$HASH_DIR"
sudo record -file /etc/go-safe-cmd-runner/deploy.toml -hash-dir "$HASH_DIR"

# 実行スクリプトのハッシュを記録
echo "Recording executable scripts..."
sudo record -file /usr/local/bin/backup.sh -hash-dir "$HASH_DIR"
sudo record -file /usr/local/bin/deploy.sh -hash-dir "$HASH_DIR"
sudo record -file /usr/local/bin/cleanup.sh -hash-dir "$HASH_DIR"

# システムバイナリのハッシュを記録
echo "Recording system binaries..."
sudo record -file /usr/bin/rsync -hash-dir "$HASH_DIR"
sudo record -file /usr/bin/pg_dump -hash-dir "$HASH_DIR"

echo "Hash recording completed successfully!"
```

### 4.2 ファイル更新後のハッシュ再記録

**スクリプト更新時の手順**

```bash
# 1. バックアップを作成
sudo cp /usr/local/bin/backup.sh /usr/local/bin/backup.sh.bak

# 2. スクリプトを編集
sudo vim /usr/local/bin/backup.sh

# 3. ハッシュを再記録
sudo record -file /usr/local/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force

# 4. 動作確認
runner -config /etc/go-safe-cmd-runner/backup.toml -validate
```

### 4.3 複数ファイルの一括記録

**ディレクトリ内の全スクリプトを記録**

```bash
#!/bin/bash
# record-all-scripts.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
SCRIPT_DIR="/usr/local/bin"

# .sh ファイルを全て記録
for script in "$SCRIPT_DIR"/*.sh; do
    echo "Recording: $script"
    sudo record -file "$script" -hash-dir "$HASH_DIR" -force
done

echo "All scripts recorded successfully!"
```

**設定ファイルのリストから記録**

```bash
#!/bin/bash
# record-from-list.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
FILE_LIST="files-to-record.txt"

# ファイルリストの内容例:
# /usr/local/bin/backup.sh
# /usr/local/bin/deploy.sh
# /etc/config.toml

while IFS= read -r file; do
    # コメント行と空行をスキップ
    [[ "$file" =~ ^#.*$ ]] && continue
    [[ -z "$file" ]] && continue

    echo "Recording: $file"
    sudo record -file "$file" -hash-dir "$HASH_DIR" -force || {
        echo "Error recording: $file"
        exit 1
    }
done < "$FILE_LIST"

echo "All files recorded successfully!"
```

### 4.4 自動化とCI/CD統合

**GitHub Actionsでのハッシュ記録**

```yaml
name: Record File Hashes

on:
  push:
    branches: [main]
    paths:
      - 'scripts/**'
      - 'config/**'

jobs:
  record-hashes:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup record command
        run: |
          make build
          sudo install -o root -g root -m 0755 build/record /usr/local/bin/record

      - name: Create hash directory
        run: |
          sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
          sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

      - name: Record hashes for scripts
        run: |
          for script in scripts/*.sh; do
            sudo record -file "$script" \
              -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
              -force
          done

      - name: Record hashes for configs
        run: |
          for config in config/*.toml; do
            sudo record -file "$config" \
              -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
              -force
          done

      - name: Commit hash files
        run: |
          sudo cp /usr/local/etc/go-safe-cmd-runner/hashes/* ./hashes/
          git config user.name "GitHub Actions"
          git config user.email "actions@github.com"
          git add hashes/
          git commit -m "Update file hashes [skip ci]" || true
          git push
```

### 4.5 パッケージ更新後のハッシュ更新

**システムパッケージ更新時の手順**

```bash
#!/bin/bash
# update-system-hashes.sh - システム更新後のハッシュ再記録

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

# システムバイナリのリスト
BINARIES=(
    "/usr/bin/rsync"
    "/usr/bin/pg_dump"
    "/usr/bin/mysqldump"
    "/usr/bin/tar"
    "/usr/bin/gzip"
)

echo "Updating hashes for system binaries..."

for binary in "${BINARIES[@]}"; do
    if [[ -f "$binary" ]]; then
        echo "Recording: $binary"
        sudo record -file "$binary" -hash-dir "$HASH_DIR" -force
    else
        echo "Warning: $binary not found, skipping"
    fi
done

echo "Hash update completed!"
```

**cronで定期実行**

```bash
# crontab -e
# 毎週日曜日の深夜2時にシステムバイナリのハッシュを更新
0 2 * * 0 /usr/local/sbin/update-system-hashes.sh >> /var/log/hash-update.log 2>&1
```

### 4.6 テスト環境でのハッシュ管理

**テスト用の独立したハッシュディレクトリ**

```bash
#!/bin/bash
# test-setup.sh

TEST_HASH_DIR="./test-hashes"

# テスト用ハッシュディレクトリを作成
mkdir -p "$TEST_HASH_DIR"

# テストスクリプトのハッシュを記録
record -file ./test/test-script.sh -hash-dir "$TEST_HASH_DIR"
record -file ./test/test-config.toml -hash-dir "$TEST_HASH_DIR"

# テスト実行
runner -config ./test/test-config.toml -dry-run

echo "Test setup completed!"
```

## 5. トラブルシューティング

### 5.1 ファイルが見つからない

**エラーメッセージ**
```
Error: file not found: /usr/bin/backup.sh
```

**対処法**

```bash
# ファイルの存在確認
ls -l /usr/bin/backup.sh

# パスのタイプミスを確認
which backup.sh

# 相対パスの場合はカレントディレクトリを確認
pwd
```

### 5.2 権限エラー

**エラーメッセージ**
```
Error: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**対処法**

```bash
# ディレクトリの権限確認
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# 権限を修正（管理者権限が必要）
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# または sudo で record を実行
sudo record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 5.3 既存のハッシュファイルが存在

**エラーメッセージ**
```
Error: hash file already exists: /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

**対処法**

**方法1: -force フラグを使用**

```bash
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

**方法2: 既存のハッシュファイルを削除**

```bash
# ハッシュファイルを削除してから再記録
sudo rm /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
sudo record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

**方法3: バックアップを取ってから上書き**

```bash
# 既存のハッシュをバックアップ
sudo cp /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh \
       /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh.bak

# 強制的に上書き
sudo record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

### 5.4 シンボリックリンクのハッシュ記録

**動作**

シンボリックリンクを指定した場合、リンク先のファイルのハッシュが記録されます。

```bash
# シンボリックリンクを作成
ln -s /usr/local/bin/backup-v2.sh /usr/local/bin/backup.sh

# ハッシュを記録（リンク先のファイルのハッシュが記録される）
record -file /usr/local/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

**注意事項**

- ハッシュファイル名はシンボリックリンクのパスに基づいて生成されます
- リンク先のファイルが変更されても、ハッシュファイル名は変わりません
- リンク先が変更された場合、ハッシュを再記録する必要があります

### 5.5 ディレクトリを指定した場合

**エラーメッセージ**
```
Error: cannot record hash for directory: /usr/local/bin
```

**対処法**

ディレクトリ内の全ファイルのハッシュを記録したい場合は、ループを使用します：

```bash
# ディレクトリ内の全ファイルのハッシュを記録
for file in /usr/local/bin/*; do
    if [[ -f "$file" ]]; then
        record -file "$file" \
            -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
    fi
done
```

## 6. 関連ドキュメント

### コマンドラインツール

- [runner コマンドガイド](runner_command.ja.md) - メインの実行コマンド
- [verify コマンドガイド](verify_command.ja.md) - ファイル整合性の検証（デバッグ用）

### 設定ファイル

- [TOML設定ファイル ユーザーガイド](toml_config/README.ja.md)
  - [グローバルレベル設定](toml_config/04_global_level.ja.md) - `verify_files` パラメータ
  - [グループレベル設定](toml_config/05_group_level.ja.md) - グループごとのファイル検証

### プロジェクト情報

- [README.ja.md](../../README.ja.md) - プロジェクト概要
- [開発者向けドキュメント](../dev/) - ハッシュファイル命名規則の詳細

---

**最終更新**: 2025-10-02
