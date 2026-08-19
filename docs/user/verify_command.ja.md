# verify コマンド ユーザーガイド

ファイルの整合性を検証するための `verify` コマンドの使用方法を解説します。

## 目次

- [1. 概要](#1-概要)
- [2. 基本的な使い方](#2-基本的な使い方)
- [3. コマンドラインフラグ詳解](#3-コマンドラインフラグ詳解)
- [4. 実践例](#4-実践例)
- [5. トラブルシューティング](#5-トラブルシューティング)
- [6. 関連ドキュメント](#6-関連ドキュメント)

## 1. 概要

### 1.1 verify コマンドとは

`verify` コマンドは、ファイルの現在のSHA-256ハッシュ値を計算し、事前に記録されたハッシュ値と比較してファイルの整合性を検証します。

### 1.2 主な用途

- **デバッグ**: ファイル検証エラーの原因調査
- **手動検証**: 特定のファイルの整合性を個別に確認
- **トラブルシューティング**: `runner` コマンドの実行前に問題を特定
- **監査**: ファイルが改ざんされていないことの確認

### 1.3 動作の仕組み

```
1. 指定されたファイルのSHA-256ハッシュ値を計算
   ↓
2. ハッシュディレクトリから対応するハッシュファイルを検索
   ↓
3. 記録されたハッシュ値と現在のハッシュ値を比較
   ↓
4. 一致すれば成功、不一致ならエラー
```

### 1.4 runner コマンドとの関係

`runner` コマンドは内部的に自動的にファイル検証を実行しますが、`verify` コマンドは以下の場合に便利です：

- **事前確認**: `runner` 実行前に問題がないか確認
- **エラー調査**: 検証エラーの詳細を確認
- **個別検証**: 特定のファイルのみを検証

## 2. 基本的な使い方

### 2.1 最もシンプルな使用例

```bash
# カレントディレクトリのハッシュファイルを使用して検証
verify /usr/bin/backup.sh
```

成功時の出力：
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: OK

Summary: 1 succeeded, 0 failed
```

失敗時の出力：
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: hash mismatch

Summary: 0 succeeded, 1 failed
```

### 2.2 ハッシュディレクトリを指定

```bash
# 特定のディレクトリのハッシュファイルを使用
verify -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# 短縮形を使用
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 2.3 複数ファイルの検証

```bash
# 複数ファイルを直接指定（推奨）
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh /usr/local/bin/deploy.sh

# ワイルドカードを使用
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

### 2.4 終了コードによる判定

`verify` は以下の終了コードを返します。

| 終了コード | 意味 |
|---|---|
| 0 | 全ファイルの検証が成功 |
| 1 | 引数エラー、バリデータ生成失敗、ハッシュディレクトリのパスがディレクトリでない、または1件以上のファイルで検証が失敗 |
| 2 | 予期しない異常終了（Goランタイムが未捕捉panicに対して使用）。`verify` が明示的に返すことはない |
| 3 | 次のいずれかにより、検証結果が信頼できない状態で起動した。(a) ハッシュディレクトリまたはその祖先のディレクトリ権限違反、(b) ハッシュディレクトリの不在または読み取り不能、(c) 権限チェッカの初期化失敗、(d) ハッシュディレクトリのパス解決の失敗。いずれの場合も対象ファイルを1件も検証していない |

対象ファイルの祖先ディレクトリのみに違反が検出され、ハッシュディレクトリ側に違反がない場合、`verify` は終了コード 3 を返しません。この違反は警告として記録され、検証は継続します（終了コードは各ファイルの検証結果に依存します）。バイパス手段は用意されていません。ハッシュディレクトリ側の違反が検出された場合は、ハッシュディレクトリの権限を修正するか、権限の適切なパスへハッシュディレクトリを移動してください。

`verify` はハッシュディレクトリを作成しません。ハッシュディレクトリが存在しない場合は終了コード 3（`hash_dir_not_found`）で終了するため、先に `record` コマンドでハッシュを記録してください。

#### 識別トークン

検証を1件も実施せずに終了した場合、標準エラー出力のメッセージに `verify-error=<トークン>` の形で原因を表す固定文字列が含まれます。終了コードだけでは原因を分けられないため、スクリプトから原因を判別する場合はメッセージの地の文ではなくこのトークンを参照してください。

| トークン | 原因 | 対処 | 終了コード |
|---|---|---|---|
| `hash_dir_permission_violation` | ハッシュディレクトリまたはその祖先に、他者から書き込める権限が設定されている | 該当ディレクトリの権限を修正するか、権限の適切なパスへハッシュディレクトリを移動する | 3 |
| `path_resolution_failed` | ハッシュディレクトリのパスを解決できず、権限を検査したディレクトリが実際に読むディレクトリと一致すると保証できない | 指定したパスと、その途中にあるシンボリックリンクを確認する | 3 |
| `hash_dir_not_found` | ハッシュディレクトリが存在しない | `record` コマンドでハッシュを記録するか、正しいハッシュディレクトリを `-hash-dir` で指定する | 3 |
| `hash_dir_unreadable` | ハッシュディレクトリは存在するが、記録を読み取れない | ディレクトリの権限と実行ユーザーを確認する | 3 |
| `permission_checker_init_failed` | ディレクトリ権限監査に用いるチェッカを初期化できない | 標準エラー出力に併記される原因を確認する | 3 |
| `hash_dir_not_a_directory` | `-hash-dir` に指定したパスがディレクトリではない | 指定したパスを確認する | 1 |
| `invalid_arguments` | 検証対象のファイルが指定されていない、または引数を解析できない | コマンドラインを確認する | 1 |
| `permission_check_uid_unresolved` | 権限チェックの基準UIDを確定できない | `SUDO_UID` の値を確認する（5.7 節を参照） | 1 |
| `validator_init_failed` | バリデータを構築できない | 標準エラー出力に併記される原因を確認する | 1 |

```bash
# 終了コードで検証結果を判定
if verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh; then
    echo "File is valid"
else
    echo "File verification failed"
    exit 1
fi
```

## 3. コマンドラインフラグ詳解

### 3.1 ファイル指定（ポジショナル引数）

**概要**

検証するファイルをポジショナル引数として指定します。複数ファイルを同時に指定できます。

**文法**

```bash
verify [flags] <file> [<file>...]
```

**パラメータ**

- `<file>`: 検証したいファイルへの絶対パスまたは相対パス（1つ以上必須）

**使用例**

```bash
# 絶対パスで指定
verify /usr/bin/backup.sh

# 相対パスで指定
verify ./scripts/deploy.sh

# ホームディレクトリのファイル
verify ~/bin/custom-script.sh

# 複数ファイルを指定
verify /usr/bin/backup.sh /usr/bin/restore.sh

# ワイルドカードを使用
verify /usr/local/bin/*.sh
```

**注意事項**

- ファイルが存在しない場合はエラーになります
- 対応するハッシュファイルが存在しない場合もエラーになります
- シンボリックリンクの場合、リンク先のファイルが検証されます

### 3.2 `-hash-dir <directory>` / `-d <directory>` (オプション)

**概要**

ハッシュファイルが保存されているディレクトリを指定します。指定しない場合はカレントディレクトリが使用されます。

**文法**

```bash
verify -hash-dir <directory> <file>...
verify -d <directory> <file>...
```

**パラメータ**

- `<directory>`: ハッシュファイルが保存されているディレクトリパス（省略可能）
- デフォルト: カレントディレクトリ

**使用例**

```bash
# 標準のハッシュディレクトリを使用
verify -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# 短縮形を使用
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# カスタムディレクトリを使用（テスト用）
verify -d ./test-hashes ./test.sh

# 相対パスで指定
verify -d ../hashes /etc/config.toml
```

**ハッシュファイルの検索**

`verify` コマンドは、指定されたファイルパスから自動的にハッシュファイル名を生成して検索します：

```bash
# /usr/bin/backup.sh の場合
# ハッシュファイル: <hash-dir>/~usr~bin~backup.sh

verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
# 実際に検索されるファイル:
# /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

**注意事項**

- ハッシュディレクトリが存在しない場合は終了コード 3（`hash_dir_not_found`）で終了します。`verify` はハッシュディレクトリを作成しないため、先に `record` コマンドでハッシュを記録してください
- 対応するハッシュファイルが見つからない場合もエラーになります
- `record` コマンドと同じハッシュディレクトリを指定してください

## 4. 実践例

### 4.1 runner 実行前の事前確認

**全ファイルの検証スクリプト**

```bash
#!/bin/bash
# verify-all.sh - runner実行前の事前検証

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
CONFIG_FILE="/etc/go-safe-cmd-runner/backup.toml"

# 設定ファイルと実行ファイルを一括検証
echo "Verifying all files..."
if ! verify -d "$HASH_DIR" "$CONFIG_FILE" \
    /usr/local/bin/backup.sh \
    /usr/local/bin/cleanup.sh \
    /usr/bin/rsync; then
    echo "Error: Verification failed"
    exit 1
fi

echo "All files verified successfully!"
echo "You can now run: runner -config $CONFIG_FILE"
```

### 4.2 検証エラーの調査

**詳細なエラー情報の取得**

```bash
#!/bin/bash
# investigate-verification-failure.sh

FILE="/usr/bin/backup.sh"
HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

echo "=== File Verification Investigation ==="
echo "File: $FILE"
echo ""

# ファイルの存在確認
if [[ ! -f "$FILE" ]]; then
    echo "Error: File does not exist"
    exit 1
fi

# ファイル情報の表示
echo "File information:"
ls -l "$FILE"
echo ""

# 現在のハッシュ値を計算
echo "Current hash:"
sha256sum "$FILE"
echo ""

# 記録されたハッシュ値を表示
HASH_FILE="${HASH_DIR}/~usr~bin~backup.sh"
echo "Recorded hash:"
if [[ -f "$HASH_FILE" ]]; then
    cat "$HASH_FILE"
    echo ""
else
    echo "Hash file not found: $HASH_FILE"
    exit 1
fi

# 検証を実行
echo "Running verification:"
verify -d "$HASH_DIR" "$FILE"
```

### 4.3 定期的な整合性チェック

**cronで定期実行**

```bash
#!/bin/bash
# periodic-integrity-check.sh - 定期的な整合性チェック

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
LOG_FILE="/var/log/integrity-check.log"

# ログファイルにタイムスタンプを記録
echo "=== Integrity Check: $(date) ===" >> "$LOG_FILE"

# 重要なファイルのリスト
CRITICAL_FILES=(
    "/usr/local/bin/backup.sh"
    "/usr/local/bin/deploy.sh"
    "/etc/go-safe-cmd-runner/backup.toml"
    "/usr/bin/rsync"
)

# 一括検証を実行
if verify -d "$HASH_DIR" "${CRITICAL_FILES[@]}" >> "$LOG_FILE" 2>&1; then
    echo "All files verified successfully" >> "$LOG_FILE"
else
    echo "Integrity check failed. See $LOG_FILE for details" >&2
    # Slack通知などの警告処理
    # send-alert.sh "Integrity check failed"
    exit 1
fi
```

**crontabエントリ**

```bash
# crontab -e
# 毎日午前3時に整合性チェックを実行
0 3 * * * /usr/local/sbin/periodic-integrity-check.sh
```

### 4.4 CI/CDでの検証

**GitHub Actionsでの使用例**

```yaml
name: Verify File Integrity

on:
  schedule:
    # 毎日午前0時に実行
    - cron: '0 0 * * *'
  workflow_dispatch:

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup verify command
        run: |
          make build
          sudo install -o root -g root -m 0755 build/verify /usr/local/bin/verify

      - name: Restore hash files
        run: |
          sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
          sudo cp hashes/* /usr/local/etc/go-safe-cmd-runner/hashes/

      - name: Verify configuration files
        run: |
          verify -d /usr/local/etc/go-safe-cmd-runner/hashes config/*.toml

      - name: Verify scripts
        run: |
          verify -d /usr/local/etc/go-safe-cmd-runner/hashes scripts/*.sh

      - name: Report failure
        if: failure()
        run: |
          echo "File integrity verification failed!"
          echo "Some files may have been modified without updating hashes."
          exit 1
```

### 4.5 デプロイ前の検証

**デプロイスクリプト統合**

```bash
#!/bin/bash
# deploy.sh - デプロイメントスクリプト

set -e  # エラーで即座に終了

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
CONFIG_FILE="/etc/go-safe-cmd-runner/deploy.toml"

echo "=== Pre-deployment Verification ==="

# 設定ファイルとデプロイスクリプトを一括検証
echo "Verifying all files..."
if ! verify -d "$HASH_DIR" "$CONFIG_FILE" \
    /usr/local/bin/deploy-app.sh \
    /usr/local/bin/migrate-db.sh \
    /usr/local/bin/restart-services.sh; then
    echo "Error: Verification failed"
    echo "Possible causes:"
    echo "  - Files have been modified"
    echo "  - Hash files are outdated"
    echo "  - Hash files are missing"
    exit 1
fi

echo "All verifications passed!"
echo ""
echo "=== Running Deployment ==="

# デプロイを実行
runner -config "$CONFIG_FILE" -log-dir /var/log/runner

echo "Deployment completed successfully!"
```

### 4.6 バッチ検証とレポート生成

**詳細レポート付き検証スクリプト**

```bash
#!/bin/bash
# batch-verify-with-report.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
REPORT_FILE="verification-report-$(date +%Y%m%d-%H%M%S).txt"

echo "=== File Integrity Verification Report ===" > "$REPORT_FILE"
echo "Date: $(date)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# 検証対象ファイルのリスト
mapfile -t FILES < <(find /usr/local/bin -name "*.sh")
FILES+=("/etc/go-safe-cmd-runner/backup.toml")
FILES+=("/etc/go-safe-cmd-runner/deploy.toml")

TOTAL=0
PASSED=0
FAILED=0

for file in "${FILES[@]}"; do
    TOTAL=$((TOTAL + 1))

    if verify -file "$file" -hash-dir "$HASH_DIR" 2>/dev/null; then
        echo "✓ PASS: $file" >> "$REPORT_FILE"
        PASSED=$((PASSED + 1))
    else
        echo "✗ FAIL: $file" >> "$REPORT_FILE"
        FAILED=$((FAILED + 1))

        # エラー詳細を記録
        {
            echo "  Current hash: $(sha256sum "$file" | cut -d' ' -f1)"
            HASH_FILE="${HASH_DIR}/$(echo "$file" | sed 's|/|~|g')"
            if [[ -f "$HASH_FILE" ]]; then
                echo "  Recorded hash: $(cat "$HASH_FILE")"
            else
                echo "  Recorded hash: (not found)"
            fi
            echo ""
        } >> "$REPORT_FILE"
    fi
done

# サマリー
{
    echo ""
    echo "=== Summary ==="
    echo "Total files: $TOTAL"
    echo "Passed: $PASSED"
    echo "Failed: $FAILED"
} >> "$REPORT_FILE"

# レポートを表示
cat "$REPORT_FILE"

# 結果に応じて終了コードを設定
if [[ $FAILED -gt 0 ]]; then
    echo ""
    echo "Verification failed. See $REPORT_FILE for details."
    exit 1
else
    echo ""
    echo "All files verified successfully!"
    exit 0
fi
```

## 5. トラブルシューティング

### 5.1 ファイルが見つからない

**エラーメッセージ**
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: file not found
```

**対処法**

```bash
# ファイルの存在確認
ls -l /usr/bin/backup.sh

# パスのタイプミスを確認
which backup.sh

# シンボリックリンクの場合、リンク先を確認
ls -lL /usr/bin/backup.sh
```

### 5.2 ハッシュファイルが見つからない

**エラーメッセージ**
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: hash file not found
```

**対処法**

**原因1: ハッシュがまだ記録されていない**

```bash
# ハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**原因2: 間違ったハッシュディレクトリを指定**

```bash
# ハッシュファイルを検索
find /usr/local/etc/go-safe-cmd-runner -name "*backup.sh*"

# 正しいディレクトリで再度検証
verify -d /path/to/correct/hash-dir /usr/bin/backup.sh
```

**原因3: ハッシュファイル名の問題**

```bash
# ハッシュディレクトリの内容を確認
ls -la /usr/local/etc/go-safe-cmd-runner/hashes/

# 期待されるハッシュファイル名を確認
# /usr/bin/backup.sh → ~usr~bin~backup.sh
```

### 5.3 ハッシュ値の不一致

**エラーメッセージ**
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: hash mismatch
```

**原因と対処法**

**原因1: ファイルが更新された**

```bash
# ファイルの更新日時を確認
ls -l /usr/bin/backup.sh

# ファイルが意図的に更新された場合、ハッシュを再記録
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**原因2: ファイルが改ざんされた**

```bash
# ファイルのバックアップから復元
sudo cp /backup/usr/bin/backup.sh /usr/bin/backup.sh
sudo chown root:root /usr/bin/backup.sh
sudo chmod 755 /usr/bin/backup.sh

# 検証を再実行
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**原因3: ハッシュファイルが古い**

```bash
# ハッシュファイルの日時を確認
HASH_FILE="/usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh"
ls -l "$HASH_FILE"

# ファイルとハッシュの日時を比較
echo "File:"; ls -l /usr/bin/backup.sh
echo "Hash:"; ls -l "$HASH_FILE"

# ハッシュが古い場合は再記録
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.4 権限エラー

**エラーメッセージ**
```
Error creating validator: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**対処法**

```bash
# ディレクトリの権限確認
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# 読み取り権限を追加
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# またはsudoで実行
sudo verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.5 シンボリックリンクの検証

**動作**

シンボリックリンクを指定した場合、リンク先のファイルが検証されます。

```bash
# シンボリックリンクの検証
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh

# リンク先のファイルのハッシュと比較されます
```

**注意事項**

- ハッシュファイル名はシンボリックリンクのパスに基づいて生成されます
- リンク先が変更された場合、ハッシュの再記録が必要です

```bash
# リンク先を確認
ls -lL /usr/local/bin/backup.sh

# リンク先が変更された場合はハッシュを再記録
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh
```

### 5.6 スクリプトでのエラーハンドリング

**終了コードを使用した適切なエラーハンドリング**

```bash
#!/bin/bash
# robust-verification.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
FILE="/usr/bin/backup.sh"

# 検証を実行し、エラーを詳細に処理
verify -d "$HASH_DIR" "$FILE" 2>&1 | tee /tmp/verify-output.txt
EXIT_CODE=${PIPESTATUS[0]}
if [[ $EXIT_CODE -eq 0 ]]; then
    echo "Verification passed: $FILE"
elif [[ $EXIT_CODE -eq 3 ]]; then
    echo "No file was verified: $FILE"
    # 原因は識別トークンで分ける。メッセージの地の文には依存しない。
    if grep -q "verify-error=hash_dir_not_found" /tmp/verify-output.txt; then
        echo "Error: The hash directory does not exist."
        echo "Record hashes first: record -d $HASH_DIR $FILE"
    elif grep -q "verify-error=hash_dir_unreadable" /tmp/verify-output.txt; then
        echo "Error: The hash directory exists but its records cannot be read."
        echo "Check its permissions and the invoking user."
    elif grep -q "verify-error=hash_dir_permission_violation" /tmp/verify-output.txt; then
        echo "Error: The hash directory or its ancestor directories are writable by others."
        echo "Fix directory permissions and re-run."
    elif grep -q "verify-error=path_resolution_failed" /tmp/verify-output.txt; then
        echo "Error: The hash directory path could not be resolved."
        echo "Check the path and any symlinks along it."
    elif grep -q "verify-error=permission_checker_init_failed" /tmp/verify-output.txt; then
        echo "Error: The directory permission checker could not be initialized."
    else
        echo "Error: Unrecognized cause."
    fi
    echo "Output:"
    cat /tmp/verify-output.txt
    exit 3
else
    echo "Verification failed: $FILE"
    echo "Exit code: $EXIT_CODE"
    echo "Output:"
    cat /tmp/verify-output.txt

    # エラーの種類に応じた処理
    if grep -q "verify-error=hash_dir_not_a_directory" /tmp/verify-output.txt; then
        echo "Error: The hash directory path is not a directory"
    elif grep -q "file not found" /tmp/verify-output.txt; then
        echo "Error: File does not exist"
    elif grep -q "hash file not found" /tmp/verify-output.txt; then
        echo "Error: Hash has not been recorded"
        echo "Run: record -d $HASH_DIR $FILE"
    elif grep -q "hash mismatch" /tmp/verify-output.txt; then
        echo "Error: File has been modified"
        echo "Current hash:"
        sha256sum "$FILE"
    fi

    exit 1
fi
```

### 5.7 SUDO_UID の実在確認に失敗する

**症状**

`sudo verify` を実行した際、`SUDO_UID` 環境変数が指す UID が実在するユーザーでない場合、対象ファイルを1件も処理せずに起動時に失敗します。指定したファイルの一部だけが検証される、といった中途半端な結果は生じません。

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
| 非 CGO ビルドの `verify` で LDAP/SSSD 管理のユーザーを対象としている | `sudo` は NSS 経由でユーザーを解決しますが、非 CGO ビルドは `/etc/passwd` しか参照しません。CGO 有効でビルドし直してください。エラーメッセージの `user_database_source=passwd-file` が手がかりになります |
| ユーザーデータベースの一時障害（LDAP/SSSD の停止、ネットワーク断） | 復旧後に再実行してください。`failed to verify that SUDO_UID refers to an existing user` で判別できます |
| root の cron・systemd unit に `SUDO_UID` が残留し、その UID が既に削除されている | 残留値が実在しない UID を指しています。`SUDO_UID` を環境から除いてください。この失敗は意図した挙動であり、`SUDO_UID` の残留を検出するためのものです |
| `/etc/passwd` が無いか、ほぼ空のコンテナイメージ（`scratch` 系、一部の distroless）を非 CGO ビルドの root で実行し、`SUDO_UID` が引き継がれている | 参照先にユーザーが1件もありません。`SUDO_UID` が `0` の場合も失敗します。`SUDO_UID` を環境から除くか、`/etc/passwd` を用意してください |

いずれの環境でも、環境変数を除いて実行する方法があります。

```bash
sudo env -u SUDO_UID verify ...
```

ただしこれは判定条件を等価に保つ回避策ではありません。`SUDO_UID` を除くと基準UIDが `0` になるため、グループ書き込み可能なファイルについては「root がそのグループに属するか」で判定され、多くの場合それまで通っていた読み取りが通らなくなります。判定は緩む側ではなく厳しい側へ動くため、検証の実行自体が失敗する可能性があります。

**正常運用でも出力される警告**

`SUDO_UID` の採用によって基準UIDが実 UID と異なる値になった場合、警告が標準エラー出力へ1回記録されます。この警告は起動時に出力されるため、`sudo verify` の通常運用では実行1回につき必ず1件出力されます。cron や systemd unit から実行する場合は、この警告を捉えるため標準エラー出力を保存してください。

出力例を次に示します（既定のログハンドラが行頭に付与する時刻は省略しています）。

```text
WARN Permission check UID taken from SUDO_UID instead of the real UID; if this process was not started via sudo, SUDO_UID may be a stale value inherited from the environment permission_check_uid=1000 real_uid=0 source_env_var=SUDO_UID permission_check_uid_policy=sudo-uid-aware user_database_source=nss
```

## 6. 関連ドキュメント

### コマンドラインツール

- [runner コマンドガイド](runner_command.ja.md) - メインの実行コマンド
- [record コマンドガイド](record_command.ja.md) - ハッシュファイルの作成（管理者向け）

### 設定ファイル

- [TOML設定ファイル ユーザーガイド](toml_config/README.ja.md)
  - [グローバルレベル設定](toml_config/04_global_level.ja.md) - `verify_files` パラメータ
  - [グループレベル設定](toml_config/05_group_level.ja.md) - グループごとのファイル検証
  - [トラブルシューティング](toml_config/11_troubleshooting.ja.md) - 検証エラーの対処法

### プロジェクト情報

- [README.ja.md](../../README.ja.md) - プロジェクト概要
- [開発者向けドキュメント](../dev/) - ファイル検証アーキテクチャの詳細

---

**最終更新**: 2026-08-02
