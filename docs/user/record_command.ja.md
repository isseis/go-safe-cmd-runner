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
record /usr/bin/backup.sh
```

実行結果：
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: OK (~usr~bin~backup.sh)

Summary: 1 succeeded, 0 failed
```

### 2.2 ハッシュディレクトリを指定

```bash
# 特定のディレクトリにハッシュを記録
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# 短縮形を使用
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

実行結果：
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: OK (~usr~bin~backup.sh)

Summary: 1 succeeded, 0 failed
```

### 2.3 既存のハッシュを上書き

```bash
# 既存のハッシュファイルを強制的に上書き
record -force -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 2.4 複数ファイルの一括記録

```bash
# 複数ファイルを直接指定（推奨）
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh /usr/local/bin/deploy.sh

# ワイルドカードを使用
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

### 2.5 syscall 解析のデバッグ情報を含めて記録

```bash
# デバッグ情報（Occurrences, DeterminationStats）を含めて記録
record --debug-info -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh
```

## 3. コマンドラインフラグ詳解

### 3.1 ファイル指定（ポジショナル引数）

**概要**

ハッシュ値を記録するファイルをポジショナル引数として指定します。複数ファイルを同時に指定できます。

**文法**

```bash
record [flags] <file> [<file>...]
```

**パラメータ**

- `<file>`: ハッシュを記録したいファイルへの絶対パスまたは相対パス（1つ以上必須）

**使用例**

```bash
# 絶対パスで指定
record /usr/bin/backup.sh

# 相対パスで指定
record ./scripts/deploy.sh

# ホームディレクトリのファイル
record ~/bin/custom-script.sh

# 複数ファイルを指定
record /usr/bin/backup.sh /usr/bin/restore.sh

# ワイルドカードを使用
record /usr/local/bin/*.sh
```

**注意事項**

- ファイルが存在しない場合はエラーになります
- シンボリックリンクの場合、リンク先のファイルのハッシュが記録されます
- ディレクトリは指定できません（ファイルのみ）

### 3.2 `-hash-dir <directory>` / `-d <directory>` (オプション)

**概要**

ハッシュファイルを保存するディレクトリを指定します。指定しない場合はデフォルトのハッシュディレクトリ（`/usr/local/etc/go-safe-cmd-runner/hashes`）が使用されます。

**文法**

```bash
record -hash-dir <directory> <file>...
record -d <directory> <file>...
```

**パラメータ**

- `<directory>`: ハッシュファイルを保存するディレクトリパス（省略可能）
- デフォルト: `/usr/local/etc/go-safe-cmd-runner/hashes`

**使用例**

```bash
# 標準のハッシュディレクトリに保存
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# 短縮形を使用
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# カスタムディレクトリに保存（テスト用）
record -d ./test-hashes ./test.sh

# 相対パスで指定
record -d ../hashes /etc/config.toml
```

**ディレクトリの自動作成**

指定したディレクトリが存在しない場合、自動的に作成されます（権限: 0700）。作成はディレクトリ権限のチェックを通過したあとに行われます。チェックで違反が見つかった場合、`record` はディレクトリを作らずにエラー終了します。

さらに、指定したハッシュディレクトリが既に存在していてそれ自身が誰からでも書き込めるディレクトリ（world-writable）である場合、および、存在せずその作成先（指定したパスのうち実在する最深の祖先）が world-writable である場合は、sticky ビットの有無にかかわらずエラー終了します。後者ではディレクトリを作成しません。作成しようとする位置に他者があらかじめハッシュ記録を置ける状態では、`record` が処理していないファイルの記録が混ざりうるためです。既に存在する場合は `chmod go-w <ハッシュディレクトリ>` で是正するか、自分だけが書き込める場所へ移動してください。存在しない場合、作成先に sticky ビットが設定されていれば（`/tmp` などが該当します）、利用者自身が先にディレクトリを作成し（適切な所有者と権限を与えたうえで）あらためて `record` を実行できます。設定されていない場合は、ディレクトリを作成しても祖先ディレクトリの権限チェックで拒否されるため、`chmod go-w` でその祖先を是正するか、自分だけが書き込める場所をハッシュディレクトリに選んでください。

```bash
# ディレクトリが存在しない場合でもOK（作成先が world-writable でなければ作成される）
record -d /new/hash/directory /usr/bin/backup.sh
# /new/hash/directory が自動的に作成されます
```

**権限について**

- ハッシュディレクトリは 0700 権限で作成されます（所有者: rwx, グループ: ---, その他: ---）
- ハッシュファイルは 0640 権限で作成されます（所有者: rw-, グループ: r--, その他: ---）

**本番環境での推奨設定**

```bash
# 本番環境では標準ディレクトリを使用
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 700 /usr/local/etc/go-safe-cmd-runner/hashes

# ハッシュを記録
sudo record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**record の実行者と runner の実行者が異なる場合（分離運用）**

上記の 0700 は、record を実行する管理者自身が runner も実行する構成を前提にしています。管理者が record でハッシュを記録し、より権限を限った別のユーザーが runner を実行する構成では、この設定のままでは runner が動きません。

runner は起動直後に実効 UID を実行者の実 UID へ降格し、以後のハッシュ照合をそのユーザーの権限で行います。したがって runner の実行者はハッシュディレクトリとハッシュファイルを読める必要があります。root 所有・0700 のままでは読めず、検証が全件失敗します。

この構成では、runner 実行者を含むグループを作り、グループに読み取りを許可してください。

```bash
# runner を実行するユーザーを含むグループ（例: gscr-runners）を作る
sudo groupadd gscr-runners
sudo usermod -aG gscr-runners batch-user

# ハッシュディレクトリの所有者は record 実行者のまま、グループだけを付け替える
sudo chgrp gscr-runners /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 750 /usr/local/etc/go-safe-cmd-runner/hashes
```

この 0750 は起動時の権限チェックを通ります。チェックが拒否するのは書き込みを許す設定（sticky bit の無い world-writable、および安全と判定されないグループへの group-writable）であり、グループへの読み取り許可は拒否しません。

グループに書き込みを与えてはいけません（`chmod 770` 等）。ハッシュDBへの書き込みは改ざんそのものを意味します。権限チェックは、所有者以外のメンバーがいるグループへの書き込み許可を拒否します（分離運用のグループはまさにこれに当たるため、`770` にすると runner が起動時に停止します）。

付け替えるのは所有者ではなくグループである点にも注意してください。所有者を runner 実行者にすると、その実行者が自分でハッシュを書き換えられるようになり、分離した意味が失われます。

### 3.3 `-force` (オプション)

**概要**

既存のハッシュファイルを強制的に上書きします。指定しない場合、既存のハッシュファイルが存在するとエラーになります。

**文法**

```bash
record -force [-hash-dir <directory>] <file>...
```

**使用例**

**通常の動作（既存ファイルがあるとエラー）**

```bash
# 1回目は成功
record -d ./hashes /usr/bin/backup.sh

# 2回目はエラー
record -d ./hashes /usr/bin/backup.sh
# Error: hash file already exists: ./hashes/~usr~bin~backup.sh
```

**-force フラグを使用**

```bash
# 既存のハッシュファイルを上書き
record -force -d ./hashes /usr/bin/backup.sh
```

**ユースケース**

- **ファイル更新後**: スクリプトやバイナリを更新した後、ハッシュを再記録
- **強制再同期**: ハッシュファイルが破損または誤って削除された場合の復旧
- **バッチ更新**: 複数ファイルのハッシュを一括更新するスクリプト

**使用例：バッチ更新**

```bash
# 全スクリプトのハッシュを強制的に再記録
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

**注意事項**

- `-force` フラグは既存のハッシュファイルの上書き専用です。権限違反チェックをバイパスするものではありません。ハッシュディレクトリの祖先ディレクトリに安全でない権限を検出した場合、`-force` の指定に関わらず record はハッシュ生成を拒否します。
- `-force` フラグは既存のハッシュファイルを警告なしで上書きします
- 誤って重要なハッシュファイルを上書きしないよう注意してください
- 本番環境では、バックアップを取ってから使用することを推奨します

### 3.4 `--debug-info` (オプション)

**概要**

syscall 解析結果にデバッグ情報（`Occurrences` フィールドおよび `DeterminationStats` フィールド）を含めて保存します。指定しない場合、これらのフィールドは保存されたハッシュレコードから除去されます。

**文法**

```bash
record --debug-info [-hash-dir <directory>] <file>...
```

**デバッグ情報の内容**

- **`Occurrences`**: 各 syscall が検出されたバイナリ内の仮想アドレス一覧、および syscall 番号の判定方法（`DeterminationMethod`、`DeterminationDetail`）
- **`DeterminationStats`**: syscall 番号解決の統計（即値解決、レジスタコピーチェーン、分岐収束による解決数など）

**使用例**

```bash
# デバッグ情報を含めてハッシュを記録
record --debug-info -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh

# 記録されたデバッグ情報を確認（JSON で保存されたハッシュレコードを参照）
cat /usr/local/etc/go-safe-cmd-runner/hashes/~usr~local~bin~backup.sh.json | python3 -m json.tool | grep -A 20 '"occurrences"'
```

**ユースケース**

- **解析結果の調査**: `unknown:indirect_setting` などの警告が出たときに、どのアドレスで検出されたかを確認する
- **判定精度の確認**: `DeterminationStats` により、何件の syscall が即値・コピーチェーン・分岐収束で解決されたかを把握する
- **開発・デバッグ**: syscall 解析ロジックの動作確認

**注意事項**

- デバッグ情報はファイルサイズが増加します
- 通常の本番環境ではデバッグ情報は不要です（デフォルト: 除去）

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
sudo chmod 700 "$HASH_DIR"

# 設定ファイルのハッシュを記録
echo "Recording configuration files..."
sudo record -d "$HASH_DIR" /etc/go-safe-cmd-runner/backup.toml /etc/go-safe-cmd-runner/deploy.toml

# 実行スクリプトのハッシュを記録
echo "Recording executable scripts..."
sudo record -d "$HASH_DIR" /usr/local/bin/backup.sh /usr/local/bin/deploy.sh /usr/local/bin/cleanup.sh

# システムバイナリのハッシュを記録
echo "Recording system binaries..."
sudo record -d "$HASH_DIR" /usr/bin/rsync /usr/bin/pg_dump

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
sudo record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh

# 4. 動作確認
runner -config /etc/go-safe-cmd-runner/backup.toml -dry-run
```

### 4.3 複数ファイルの一括記録

**ディレクトリ内の全スクリプトを記録**

```bash
#!/bin/bash
# record-all-scripts.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
SCRIPT_DIR="/usr/local/bin"

# .sh ファイルを全て記録
echo "Recording scripts in $SCRIPT_DIR..."
sudo record -force -d "$HASH_DIR" "$SCRIPT_DIR"/*.sh

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

# ファイルリストを配列に読み込んで一括記録
mapfile -t FILES < <(grep -v '^#' "$FILE_LIST" | grep -v '^$')
if [[ ${#FILES[@]} -gt 0 ]]; then
    sudo record -force -d "$HASH_DIR" "${FILES[@]}"
fi

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
          sudo chmod 700 /usr/local/etc/go-safe-cmd-runner/hashes

      - name: Record hashes for scripts
        run: |
          sudo record -force -d /usr/local/etc/go-safe-cmd-runner/hashes scripts/*.sh

      - name: Record hashes for configs
        run: |
          sudo record -force -d /usr/local/etc/go-safe-cmd-runner/hashes config/*.toml

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

# 存在するバイナリのみをフィルタリング
EXISTING_BINARIES=()
for binary in "${BINARIES[@]}"; do
    if [[ -f "$binary" ]]; then
        EXISTING_BINARIES+=("$binary")
    else
        echo "Warning: $binary not found, skipping"
    fi
done

# 存在するバイナリのハッシュを一括記録
if [[ ${#EXISTING_BINARIES[@]} -gt 0 ]]; then
    echo "Updating hashes for system binaries..."
    sudo record -force -d "$HASH_DIR" "${EXISTING_BINARIES[@]}"
fi

echo "Hash update completed!"
```

**注意事項: ハッシュ更新を自動化してはならない**

ハッシュの更新を cron などで自動化することは**危険**です。

- record コマンドの目的は「意図した状態のハッシュを記録し、後でその状態と一致するか検証する」ことです
- ハッシュの更新を自動化すると、攻撃者がバイナリを改ざんした場合でもその変更が自動的にハッシュに取り込まれ、改ざんを見逃します
- ハッシュの更新は必ずパッケージ更新の内容を人間が確認した後に、手動で実行してください

### 4.6 テスト環境でのハッシュ管理

**テスト用の独立したハッシュディレクトリ**

```bash
#!/bin/bash
# test-setup.sh

TEST_HASH_DIR="./test-hashes"

# テスト用ハッシュディレクトリを作成
mkdir -p "$TEST_HASH_DIR"

# テストスクリプトのハッシュを記録
record -d "$TEST_HASH_DIR" ./test/test-script.sh ./test/test-config.toml

# テスト実行
runner -config ./test/test-config.toml -dry-run

echo "Test setup completed!"
```

### 4.7 セキュリティ: record は信頼できる管理者権限・クリーンな環境で実行すること

record は、ハッシュベースのファイル整合性検証における**信頼の起点**です。record が生成するハッシュDBは、`runner` コマンドや `verify` コマンドが改ざんを検出するための土台になります。この信頼を維持するため、**record は信頼できる管理者権限・クリーンな環境で実行してください**。具体的には次の点を守ってください。

- **record は root またはハッシュディレクトリへの排他的アクセス権を持つ専用の管理者アカウントで実行してください。**
- **ハッシュディレクトリおよびその祖先ディレクトリすべてに安全な権限（所有者のみ書き込み可能で、グループ・その他への書き込みを許さない）を設定してください。** record は実行のたびにこれを強制します。ハッシュディレクトリの祖先チェーンに sticky bit のない world-writable、または安全と判定されない group-writable なディレクトリを検出した場合、ハッシュ生成を拒否します（フェイルクローズド、非ゼロ終了）。ハッシュディレクトリ自身とその作成先については、sticky bit の有無にかかわらず world-writable であれば拒否します（§3.2 参照）。
- **信頼できないディレクトリ（`/tmp` や、管理者以外もアクセスできる共有ボリューム等）で record を実行しないでください。**
- **権限違反を無視して続行するバイパスフラグは存在しません。** `-force` フラグは既存のハッシュファイルの上書きのみを制御し、セキュリティチェックには影響しません。
- **`0o750` 権限のハッシュディレクトリを使っていた環境からアップグレードする場合:** `os.MkdirAll` は既存ディレクトリの権限を変更しません。既存のハッシュディレクトリは `chmod 0700 <ハッシュディレクトリ>` で手動で是正してください。ただし、record の実行者と runner の実行者が異なる分離運用（§3.2 参照）では狭めないでください。その構成では runner 実行者がハッシュを読めなくなり、検証が全件失敗します。0750 のまま、グループが runner 実行者のグループになっていること、およびグループに書き込みが無いことを確認してください。

## 5. トラブルシューティング

### 5.1 ファイルが見つからない

**エラーメッセージ**
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Error recording hash for /usr/bin/backup.sh: file not found
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
Error creating validator: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**対処法**

```bash
# ディレクトリの権限確認
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# 権限を修正（管理者権限が必要）
sudo chmod 700 /usr/local/etc/go-safe-cmd-runner/hashes

# または sudo で record を実行
sudo record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.3 既存のハッシュファイルが存在

**エラーメッセージ**
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Error recording hash for /usr/bin/backup.sh: hash file already exists
```

**対処法**

**方法1: -force フラグを使用**

```bash
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**方法2: 既存のハッシュファイルを削除**

```bash
# ハッシュファイルを削除してから再記録
sudo rm /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
sudo record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**方法3: バックアップを取ってから上書き**

```bash
# 既存のハッシュをバックアップ
sudo cp /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh \
       /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh.bak

# 強制的に上書き
sudo record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.4 シンボリックリンクのハッシュ記録

**動作**

シンボリックリンクを指定した場合、リンク先のファイルのハッシュが記録されます。

```bash
# シンボリックリンクを作成
ln -s /usr/local/bin/backup-v2.sh /usr/local/bin/backup.sh

# ハッシュを記録（リンク先のファイルのハッシュが記録される）
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh
```

**注意事項**

- ハッシュファイル名はシンボリックリンクのパスに基づいて生成されます
- リンク先のファイルが変更されても、ハッシュファイル名は変わりません
- リンク先が変更された場合、ハッシュを再記録する必要があります

### 5.5 ディレクトリを指定した場合

**エラーメッセージ**
```
Processing 1 file...
[1/1] /usr/local/bin: FAILED
Error recording hash for /usr/local/bin: cannot record hash for directory
```

**対処法**

ディレクトリ内の全ファイルのハッシュを記録したい場合は、ワイルドカードを使用します：

```bash
# ディレクトリ内の全ファイルのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*

# または特定の拡張子のみ
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

### 5.6 SUDO_UID の実在確認に失敗する

**症状**

`sudo record` を実行した際、`SUDO_UID` 環境変数が指す UID が実在するユーザーでない場合、対象ファイルを1件も処理せずに起動時に失敗します。指定したファイルの一部だけが記録される、といった中途半端な結果は生じません。

**エラーメッセージ**

実在確認の失敗には次の2種類があります。

```text
Error: SUDO_UID 9999 does not exist in the user database (user_database_source=nss); check whether SUDO_UID is a stale value inherited from the environment, then re-run from an interactive sudo session: SUDO_UID does not refer to an existing user: user: unknown userid 9999
```

上記は「`SUDO_UID` が実在しないユーザーを指している」場合です。メッセージにセンチネル文言 `SUDO_UID does not refer to an existing user` が含まれます。設定の誤りまたは残留であり、同じ環境のまま再実行しても解消しません。残留または誤った `SUDO_UID` を取り除いたうえで、対話的な sudo セッションから再実行してください。

```text
Error: could not verify SUDO_UID 9999 against the user database (user_database_source=nss); check the state of the user database, then re-run: failed to verify that SUDO_UID refers to an existing user: user: lookup userid 9999: ...
```

上記は「ユーザーデータベースへの照会そのものが失敗した」場合です。メッセージにセンチネル文言 `failed to verify that SUDO_UID refers to an existing user` が含まれます。ユーザーデータベースの一時的な障害である可能性があり、再実行が有効な場合があります。

どちらのメッセージにも `user_database_source=` が含まれ、`nss`（CGO 有効ビルド）または `passwd-file`（CGO 無効ビルド）で参照先のユーザーデータベースを判別できます。`user_database_source` は採用事実の記録の属性名でもあり、エラーメッセージ内でも同じ綴りで現れます。

**対処法**

| 環境 | 対処 |
|---|---|
| 非 CGO ビルドの `record` で LDAP/SSSD 管理のユーザーを対象としている | `sudo` は NSS 経由でユーザーを解決しますが、非 CGO ビルドは `/etc/passwd` しか参照しません。CGO 有効でビルドし直してください。エラーメッセージの `user_database_source=passwd-file` が手がかりになります |
| ユーザーデータベースの一時障害（LDAP/SSSD の停止、ネットワーク断） | 復旧後に再実行してください。`failed to verify that SUDO_UID refers to an existing user` で判別できます |
| root の cron・systemd unit に `SUDO_UID` が残留し、その UID が既に削除されている | 残留値が実在しない UID を指しています。`SUDO_UID` を環境から除いてください。この失敗は意図した挙動であり、`SUDO_UID` の残留を検出するためのものです |
| `/etc/passwd` が無いか、ほぼ空のコンテナイメージ（`scratch` 系、一部の distroless）を非 CGO ビルドの root で実行し、`SUDO_UID` が引き継がれている | 参照先にユーザーが1件もありません。`SUDO_UID` が `0` の場合も失敗します。`SUDO_UID` を環境から除くか、`/etc/passwd` を用意してください |

いずれの環境でも、環境変数を除いて実行する方法があります。

```bash
sudo env -u SUDO_UID record ...
```

ただしこれは判定条件を等価に保つ回避策ではありません。`SUDO_UID` を除くと基準UIDが `0` になるため、グループ書き込み可能なファイルについては「root がそのグループに属するか」で判定され、多くの場合それまで通っていた読み取りが通らなくなります。判定は緩む側ではなく厳しい側へ動くため、記録の生成自体が失敗する可能性があります。

**正常運用でも出力される警告**

`SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった場合、警告が標準エラー出力へ1回記録されます。この警告は起動時に出力されるため、`sudo record` の通常運用では実行1回につき必ず1件出力されます。cron や systemd unit から実行する場合は、この警告を捉えるため標準エラー出力を保存してください。

出力例を次に示します（既定のログハンドラが行頭に付与する時刻は省略しています）。

```text
WARN Permission check UID taken from SUDO_UID instead of the real UID; if this process was not started via sudo, SUDO_UID may be a stale value inherited from the environment permission_check_uid=1000 real_uid=0 source_env_var=SUDO_UID permission_check_uid_policy=sudo-uid-aware user_database_source=nss
```

### 5.7 group-writable なファイルの書き込みが拒否される（列挙不完全）

**症状**

ハッシュディレクトリ、または記録対象ファイルの祖先ディレクトリの構成要素が group-writable であり、かつこのホスト上でグループメンバーの列挙が完全であることを確認できない場合、その書き込み安全性判定が拒否され、`record` は対象ファイルを1件も記録せずに終了します。この判定は以前は「実際より緩く評価される」ことで許可されていましたが、現在は拒否されます。5.6 の `SUDO_UID` の実在確認とは異なる判定です。

判定が見るのは `/etc/nsswitch.conf` の `passwd`・`group` の2行だけであり、netgroup 行は判定に影響しません。Ubuntu の既定である `netgroup: nis` を見て自ホストが該当すると誤認しないでください——ネットグループは GID を持たず、グループメンバーの列挙には関与しません。

**エラーメッセージ（非 CGO ビルド、`CGO_ENABLED=0`）**

以下の3例はいずれも非 CGO ビルドのものです。メッセージの `user_database_source=passwd-file` が目印になります。拒否の原因は3種類あり、いずれもセンチネル文言 `group member enumeration is incomplete` と、原因を示す `cause=` 属性を含みます。

```text
Error: cannot confirm the members of group GID 1000: /etc/nsswitch.conf names a user database source this build cannot consult, or could not be read (user_database_source=passwd-file, cause=nss-sources); check the passwd and group lines of /etc/nsswitch.conf, then rebuild with CGO_ENABLED=1 so that the configured sources are consulted: group member enumeration is incomplete
```

`cause=nss-sources` は NSS 環境が原因です。`/etc/nsswitch.conf` の `passwd`・`group` 行が `files`・`systemd` 以外のソース（LDAP/SSSD 等）を含む、または同ファイルの読み取りに失敗しています。

```text
Error: cannot confirm the members of group GID 1000: a line of the user database files could not be parsed and was skipped, so the members listed there are unknown (user_database_source=passwd-file, cause=malformed-line, detail=...); check the reported line: correct it if its format is wrong, or, if it is a NIS compatibility entry (a line starting with + or -), rebuild with CGO_ENABLED=1: group member enumeration is incomplete
```

`cause=malformed-line` は `/etc/passwd`・`/etc/group` の不正行が原因です。この場合は標準エラー出力に別途 `slog.Warn` で該当ファイル名・行番号が記録されます。**この原因は CGO ビルドでは発生しません**——CGO ビルドはファイルを自ら走査せず、libc の NSS lookup を経由するためです。

```text
Error: cannot confirm the members of group GID 1000: this build cannot enumerate all members of a group on this platform (user_database_source=passwd-file, cause=unsupported-platform); rebuild with CGO_ENABLED=1 so that group members are resolved through the platform's own user database via libc: group member enumeration is incomplete
```

`cause=unsupported-platform` は macOS 配布バイナリが原因です。非 CGO ビルドの `darwin` バイナリは、`/etc/nsswitch.conf` を持たないため常に不完全と判定されます。

**対処法（非 CGO ビルド）**

| 原因（`cause=`） | 対処 |
|---|---|
| `nss-sources`（NSS 環境） | `CGO_ENABLED=1` でビルドし直すか、対象パスの group-writable ビットを外す（`chmod g-w`） |
| `malformed-line`（不正行） | 警告ログが指す行を修正する。NIS 互換行（`+`・`-` で始まる）であれば `CGO_ENABLED=1` でビルドし直す |
| `unsupported-platform`（macOS） | `CGO_ENABLED=1` でセルフビルドするか、対象パスの group-writable ビットを外す（`chmod g-w`） |

**エラーメッセージ（CGO ビルド、`CGO_ENABLED=1`）**

以下はセルフビルドした CGO ビルドのものです。メッセージの `user_database_source=nss` が目印になります。CGO ビルドは libc の NSS lookup を経由してユーザー・グループデータベースを解決するため、`cause=malformed-line` は生じません。

```text
Error: cannot confirm the members of group GID 1000: /etc/nsswitch.conf does not establish that every member of a group is enumerated: a source it names gives no guarantee of exhaustive enumeration (SSSD returns no directory users under enumerate = False, and no explicit members under ignore_group_members = True), a line it needs is missing or could not be read as written, or the file could not be read; the detail says which (user_database_source=nss, cause=nss-sources, detail=passwd: sss); clear the group-writable bit on the path (chmod g-w), or configure the passwd and group lines with only sources whose enumeration is exhaustive (files, systemd): group member enumeration is incomplete
```

`cause=nss-sources` は、`/etc/nsswitch.conf` が設定するソース（SSSD 等）が網羅的な列挙を保証しないこと、あるいは行の形が読めない・行が無い・ファイルが読めないことが原因です。どれに当たるかは `detail` が示します。

```text
Error: cannot confirm the members of group GID 1000: this platform offers no way to determine how its user database is configured, so a group's member list cannot be confirmed to cover every member (user_database_source=nss, cause=unsupported-platform); clear the group-writable bit on the path (chmod g-w): group member enumeration is incomplete
```

`cause=unsupported-platform` は `GOOS` が `linux` 以外の CGO ビルド（macOS でのセルフビルド等）が原因です。

**対処法（CGO ビルド）**

**`CGO_ENABLED=1` でのビルドは、CGO ビルドに対する回復手段になりません**——既にその条件を満たしているためです。

| 原因（`cause=`） | 対処 |
|---|---|
| `nss-sources`（NSS 環境） | 対象パスの group-writable ビットを外す（`chmod g-w`）。または `passwd`・`group` の両行を、列挙が網羅的なソース（`files`・`systemd`）のみで構成する |
| `unsupported-platform`（`linux` 以外） | 対象パスの group-writable ビットを外す（`chmod g-w`） |

**事前の検知**

列挙が不完全と判定されると、プロセス起動時に一度だけ次の警告が標準エラー出力に出ます。実際に拒否が起きる前に出力されるため、対象ホストで書き込み対象を含まない試験的な `record` 実行を1回行えば、拒否が起きるかどうかを事前に検知できます。ただし `record` は警告を出しても実行を止めずハッシュファイルの書き込みへ進むため、実行前の事前確認そのものには次項のとおり `verify` を使ってください。

```text
WARN This build cannot enumerate every member of a group on this host user_database_source=passwd-file cause=nss-sources detail=...
```

事前確認には `verify` を用いてください。`verify` は起動時に完全性判定を確定させ、読み取りのみを行うため、ハッシュファイルを書き換えずに拒否の有無を確認できます。

**`record` が途中で拒否された場合の復旧手順**

`record` は複数ファイルを1回の実行で処理しますが、拒否は起動時（最初の group-writable な構成要素に到達した時点）で発生するため、通常はどのファイルも記録されずに終了します。ただし復旧の手順としては、途中まで記録が進んでいる可能性を前提に確認してください。

1. ハッシュディレクトリの内容と、記録対象として指定した一覧を突き合わせ、既にハッシュファイルが存在するものと存在しないものを確認する。
2. 上記の対処法（原因に応じた group-writable ビットの除去、または `passwd`・`group` 行の構成変更）を適用する。
3. `record` を再実行する。`-force` を付けなければ、既に記録済みのファイルは上書きされずスキップされる（5.3 参照）。

**runner の実行前検証での拒否について**

`internal/security` のディレクトリ権限検査（`runner` の実行前検証）でこの拒否が起きた場合、`slog` による構造化された記録は出ません。返るのはエラー文字列のみであり、どのディレクトリで拒否されたかはエラー本文（`cannot confirm the members of group GID <gid>: ...`）から読み取る必要があります。

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

**最終更新**: 2026-08-02
