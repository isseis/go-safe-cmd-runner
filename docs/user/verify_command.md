# verify Command User Guide# verify コマンド ユーザーガイド



This guide explains how to use the `verify` command for verifying file integrity.ファイルの整合性を検証するための `verify` コマンドの使用方法を解説します。



## Table of Contents## 目次



- [1. Overview](#1-overview)- [1. 概要](#1-概要)

- [2. Basic Usage](#2-basic-usage)- [2. 基本的な使い方](#2-基本的な使い方)

- [3. Command Line Flags Details](#3-command-line-flags-details)- [3. コマンドラインフラグ詳解](#3-コマンドラインフラグ詳解)

- [4. Practical Examples](#4-practical-examples)- [4. 実践例](#4-実践例)

- [5. Troubleshooting](#5-troubleshooting)- [5. トラブルシューティング](#5-トラブルシューティング)

- [6. Related Documentation](#6-related-documentation)- [6. 関連ドキュメント](#6-関連ドキュメント)



## 1. Overview## 1. 概要



### 1.1 What is the verify command?### 1.1 verify コマンドとは



The `verify` command calculates the current SHA-256 hash value of a file and compares it with a previously recorded hash value to verify file integrity.`verify` コマンドは、ファイルの現在のSHA-256ハッシュ値を計算し、事前に記録されたハッシュ値と比較してファイルの整合性を検証します。



### 1.2 Main Use Cases### 1.2 主な用途



- **Debugging**: Investigating causes of file verification errors- **デバッグ**: ファイル検証エラーの原因調査

- **Manual Verification**: Individually checking the integrity of specific files- **手動検証**: 特定のファイルの整合性を個別に確認

- **Troubleshooting**: Identifying problems before running the `runner` command- **トラブルシューティング**: `runner` コマンドの実行前に問題を特定

- **Auditing**: Confirming that files have not been tampered with- **監査**: ファイルが改ざんされていないことの確認



### 1.3 How it Works### 1.3 動作の仕組み



``````

1. Calculate SHA-256 hash value of the specified file1. 指定されたファイルのSHA-256ハッシュ値を計算

   ↓   ↓

2. Search for corresponding hash file in hash directory2. ハッシュディレクトリから対応するハッシュファイルを検索

   ↓   ↓

3. Compare recorded hash value with current hash value3. 記録されたハッシュ値と現在のハッシュ値を比較

   ↓   ↓

4. Success if match, error if mismatch4. 一致すれば成功、不一致ならエラー

``````



### 1.4 Relationship with runner Command### 1.4 runner コマンドとの関係



The `runner` command automatically performs file verification internally, but the `verify` command is useful in the following cases:`runner` コマンドは内部的に自動的にファイル検証を実行しますが、`verify` コマンドは以下の場合に便利です：



- **Pre-verification**: Check for problems before running `runner`- **事前確認**: `runner` 実行前に問題がないか確認

- **Error Investigation**: Examine details of verification errors- **エラー調査**: 検証エラーの詳細を確認

- **Individual Verification**: Verify specific files only- **個別検証**: 特定のファイルのみを検証



## 2. Basic Usage## 2. 基本的な使い方



### 2.1 Simplest Usage Example### 2.1 最もシンプルな使用例



```bash```bash

# Verify using hash files in current directory# カレントディレクトリのハッシュファイルを使用して検証

verify -file /usr/bin/backup.shverify -file /usr/bin/backup.sh

``````



Output on success:成功時の出力：

``````

OK: /usr/bin/backup.shOK: /usr/bin/backup.sh

``````



Output on failure:失敗時の出力：

``````

Verification failed: hash mismatchVerification failed: hash mismatch

Expected: abc123def456...Expected: abc123def456...

Got:      def456abc123...Got:      def456abc123...

``````



### 2.2 Specifying Hash Directory### 2.2 ハッシュディレクトリを指定



```bash```bash

# Use hash files from specific directory# 特定のディレクトリのハッシュファイルを使用

verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashesverify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

``````



### 2.3 Verifying Multiple Files### 2.3 複数ファイルの検証



```bash```bash

# Verify multiple files with script# スクリプトで複数ファイルを検証

for file in /usr/local/bin/*.sh; dofor file in /usr/local/bin/*.sh; do

    echo "Verifying: $file"    echo "Verifying: $file"

    verify -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes || {    verify -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes || {

        echo "Verification failed for: $file"        echo "Verification failed for: $file"

    }    }

donedone

``````



### 2.4 Checking Results with Exit Codes### 2.4 終了コードによる判定



```bash```bash

# Use exit codes to determine verification results# 終了コードで検証結果を判定

if verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes; thenif verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes; then

    echo "File is valid"    echo "File is valid"

elseelse

    echo "File verification failed"    echo "File verification failed"

    exit 1    exit 1

fifi

``````



## 3. Command Line Flags Details## 3. コマンドラインフラグ詳解



### 3.1 `-file <path>` (Required)### 3.1 `-file <path>` (必須)



**Overview****概要**



Specifies the path to the file to verify.検証するファイルのパスを指定します。



**Syntax****文法**



```bash```bash

verify -file <path>verify -file <path>

``````



**Parameters****パラメータ**



- `<path>`: Absolute or relative path to the file to verify (required)- `<path>`: 検証したいファイルへの絶対パスまたは相対パス（必須）



**Usage Examples****使用例**



```bash```bash

# Specify with absolute path# 絶対パスで指定

verify -file /usr/bin/backup.shverify -file /usr/bin/backup.sh



# Specify with relative path# 相対パスで指定

verify -file ./scripts/deploy.shverify -file ./scripts/deploy.sh



# Home directory file# ホームディレクトリのファイル

verify -file ~/bin/custom-script.shverify -file ~/bin/custom-script.sh

``````



**Notes****注意事項**



- Error occurs if file does not exist- ファイルが存在しない場合はエラーになります

- For symbolic links, the target file is verified- 対応するハッシュファイルが存在しない場合もエラーになります

- Directories cannot be specified (files only)- シンボリックリンクの場合、リンク先のファイルが検証されます



### 3.2 `-hash-dir <directory>` (Optional)### 3.2 `-hash-dir <directory>` (オプション)



**Overview****概要**



Specifies the directory containing hash files. If not specified, the current directory is used.ハッシュファイルが保存されているディレクトリを指定します。指定しない場合はカレントディレクトリが使用されます。



**Syntax****文法**



```bash```bash

verify -file <path> -hash-dir <directory>verify -file <path> -hash-dir <directory>

``````



**Parameters****パラメータ**



- `<directory>`: Directory path containing hash files- `<directory>`: ハッシュファイルが保存されているディレクトリパス（省略可能）

- デフォルト: カレントディレクトリ

**Usage Examples**

**使用例**

```bash

# System-wide hash directory```bash

verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes# 標準のハッシュディレクトリを使用

verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# User-specific hash directory

verify -file script.sh -hash-dir ~/.go-safe-cmd-runner/hashes# カスタムディレクトリを使用（テスト用）

verify -file ./test.sh -hash-dir ./test-hashes

# Current project hash directory

verify -file script.sh -hash-dir ./hash-files# 相対パスで指定

```verify -file /etc/config.toml -hash-dir ../hashes

```

**Notes**

**ハッシュファイルの検索**

- Directory must exist and contain the corresponding hash file

- Hash file must have been created using the `record` command`verify` コマンドは、指定されたファイルパスから自動的にハッシュファイル名を生成して検索します：



### 3.3 Return Values```bash

# /usr/bin/backup.sh の場合

**Success (Exit Code 0)**# ハッシュファイル: <hash-dir>/~usr~bin~backup.sh

- File exists and hash matches

verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

**Failure (Exit Code 1)**# 実際に検索されるファイル:

- File does not exist# /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh

- Hash file does not exist```

- Hash values do not match

- Permission errors**注意事項**



## 4. Practical Examples- ハッシュディレクトリが存在しない場合はエラーになります

- 対応するハッシュファイルが見つからない場合もエラーになります

### 4.1 Pre-execution Verification- `record` コマンドと同じハッシュディレクトリを指定してください



```bash## 4. 実践例

#!/bin/bash

# Verify all files before running production commands### 4.1 runner 実行前の事前確認



HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"**全ファイルの検証スクリプト**

CONFIG_FILE="/etc/go-safe-cmd-runner/production.toml"

```bash

echo "Verifying configuration file..."#!/bin/bash

if ! verify -file "$CONFIG_FILE" -hash-dir "$HASH_DIR"; then# verify-all.sh - runner実行前の事前検証

    echo "Configuration file verification failed"

    exit 1HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

fiCONFIG_FILE="/etc/go-safe-cmd-runner/backup.toml"



echo "Verifying executable binaries..."# 設定ファイルを検証

for binary in /usr/bin/pg_dump /usr/bin/rsync /usr/local/bin/backup.sh; doecho "Verifying configuration file..."

    echo "Checking: $binary"if ! verify -file "$CONFIG_FILE" -hash-dir "$HASH_DIR"; then

    if ! verify -file "$binary" -hash-dir "$HASH_DIR"; then    echo "Error: Configuration file verification failed"

        echo "Binary verification failed: $binary"    exit 1

        exit 1fi

    fi

done# TOML設定から verify_files を抽出して検証（手動で指定）

FILES=(

echo "All verifications passed. Proceeding with execution..."    "/usr/local/bin/backup.sh"

runner -config "$CONFIG_FILE"    "/usr/local/bin/cleanup.sh"

```    "/usr/bin/rsync"

)

### 4.2 System Integrity Check

echo "Verifying executable files..."

```bashfor file in "${FILES[@]}"; do

#!/bin/bash    echo "  Checking: $file"

# System integrity monitoring script    if ! verify -file "$file" -hash-dir "$HASH_DIR"; then

        echo "  Error: Verification failed for $file"

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"        exit 1

REPORT_FILE="/var/log/integrity-check.log"    fi

done

echo "=== Integrity Check Report $(date) ===" >> "$REPORT_FILE"

echo "All files verified successfully!"

# Check critical system binariesecho "You can now run: runner -config $CONFIG_FILE"

CRITICAL_FILES=(```

    "/usr/bin/sudo"

    "/usr/bin/ssh"### 4.2 検証エラーの調査

    "/usr/bin/scp"

    "/usr/local/bin/backup.sh"**詳細なエラー情報の取得**

    "/usr/local/bin/deploy.sh"

)```bash

#!/bin/bash

FAILED_COUNT=0# investigate-verification-failure.sh



for file in "${CRITICAL_FILES[@]}"; doFILE="/usr/bin/backup.sh"

    if [[ -f "$file" ]]; thenHASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

        if verify -file "$file" -hash-dir "$HASH_DIR" 2>/dev/null; then

            echo "OK: $file" >> "$REPORT_FILE"echo "=== File Verification Investigation ==="

        elseecho "File: $FILE"

            echo "FAILED: $file" >> "$REPORT_FILE"echo ""

            ((FAILED_COUNT++))

        fi# ファイルの存在確認

    elseif [[ ! -f "$FILE" ]]; then

        echo "MISSING: $file" >> "$REPORT_FILE"    echo "Error: File does not exist"

        ((FAILED_COUNT++))    exit 1

    fifi

done

# ファイル情報の表示

if [[ $FAILED_COUNT -gt 0 ]]; thenecho "File information:"

    echo "ALERT: $FAILED_COUNT file(s) failed verification" >> "$REPORT_FILE"ls -l "$FILE"

    # Send alert (e.g., email, Slack notification)echo ""

    echo "Security alert: File integrity check failed" | mail -s "Security Alert" admin@company.com

fi# 現在のハッシュ値を計算

echo "Current hash:"

echo "=== End Report ===" >> "$REPORT_FILE"sha256sum "$FILE"

```echo ""



### 4.3 Development Workflow Integration# 記録されたハッシュ値を表示

HASH_FILE="${HASH_DIR}/~usr~bin~backup.sh"

```bashecho "Recorded hash:"

#!/bin/bashif [[ -f "$HASH_FILE" ]]; then

# Development deployment verification    cat "$HASH_FILE"

    echo ""

SCRIPT_DIR="./scripts"else

HASH_DIR="./hashes"    echo "Hash file not found: $HASH_FILE"

    exit 1

echo "Verifying scripts before deployment..."fi



for script in "$SCRIPT_DIR"/*.sh; do# 検証を実行

    if [[ -f "$script" ]]; thenecho "Running verification:"

        basename_script=$(basename "$script")verify -file "$FILE" -hash-dir "$HASH_DIR"

        echo -n "Checking $basename_script... "```



        if verify -file "$script" -hash-dir "$HASH_DIR" 2>/dev/null; then### 4.3 定期的な整合性チェック

            echo "OK"

        else**cronで定期実行**

            echo "FAILED"

            echo "Error: $script has been modified but hash not updated"```bash

            echo "Please run: record -file $script -hash-dir $HASH_DIR -force"#!/bin/bash

            exit 1# periodic-integrity-check.sh - 定期的な整合性チェック

        fi

    fiHASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

doneLOG_FILE="/var/log/integrity-check.log"



echo "All scripts verified successfully"# ログファイルにタイムスタンプを記録

```echo "=== Integrity Check: $(date) ===" >> "$LOG_FILE"



### 4.4 CI/CD Pipeline Integration# 重要なファイルのリスト

CRITICAL_FILES=(

```yaml    "/usr/local/bin/backup.sh"

# .github/workflows/verify.yml    "/usr/local/bin/deploy.sh"

name: File Integrity Check    "/etc/go-safe-cmd-runner/backup.toml"

on:    "/usr/bin/rsync"

  pull_request:)

    paths:

    - 'scripts/**'FAILED=0



jobs:for file in "${CRITICAL_FILES[@]}"; do

  verify:    if verify -file "$file" -hash-dir "$HASH_DIR" >> "$LOG_FILE" 2>&1; then

    runs-on: ubuntu-latest        echo "OK: $file" >> "$LOG_FILE"

    steps:    else

    - uses: actions/checkout@v2        echo "FAILED: $file" >> "$LOG_FILE"

            FAILED=1

    - name: Verify critical scripts

      run: |        # Slack通知などの警告処理

        # Download hash files from secure storage        # send-alert.sh "$file verification failed"

        curl -H "Authorization: Bearer ${{ secrets.STORAGE_TOKEN }}" \    fi

             -o hashes.tar.gz \done

             "${{ secrets.HASH_STORAGE_URL }}"

        tar -xzf hashes.tar.gzif [[ $FAILED -eq 1 ]]; then

            echo "Integrity check failed. See $LOG_FILE for details" >&2

        # Verify each script    exit 1

        for script in scripts/*.sh; doelse

          if ! verify -file "$script" -hash-dir ./hashes; then    echo "All files verified successfully" >> "$LOG_FILE"

            echo "::error::Script verification failed: $script"fi

            exit 1```

          fi

        done**crontabエントリ**



    - name: Report success```bash

      run: echo "All scripts verified successfully"# crontab -e

```# 毎日午前3時に整合性チェックを実行

0 3 * * * /usr/local/sbin/periodic-integrity-check.sh

## 5. Troubleshooting```



### 5.1 Common Errors### 4.4 CI/CDでの検証



#### Hash File Not Found**GitHub Actionsでの使用例**



**Error Message:**```yaml

```name: Verify File Integrity

Error: hash file not found

```on:

  schedule:

**Causes and Solutions:**    # 毎日午前0時に実行

- **Hash not recorded**: Record hash using `record` command first    - cron: '0 0 * * *'

- **Wrong hash directory**: Check if `-hash-dir` parameter is correct  workflow_dispatch:

- **Filename encoding**: Check if file path encoding matches

jobs:

#### Hash Mismatch  verify:

    runs-on: ubuntu-latest

**Error Message:**    steps:

```      - uses: actions/checkout@v3

Verification failed: hash mismatch

Expected: abc123def456...      - name: Setup verify command

Got:      def456abc123...        run: |

```          make build

          sudo install -o root -g root -m 0755 build/verify /usr/local/bin/verify

**Causes and Solutions:**

- **File modified**: File has been changed after hash recording      - name: Restore hash files

- **File replaced**: File has been replaced with a different file        run: |

- **Symbolic link changed**: Target of symbolic link has changed          sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes

          sudo cp hashes/* /usr/local/etc/go-safe-cmd-runner/hashes/

**Investigation Steps:**

```bash      - name: Verify configuration files

# Check file modification time        run: |

stat /usr/bin/backup.sh          verify -file config/backup.toml \

            -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# Check current hash

sha256sum /usr/bin/backup.sh      - name: Verify scripts

        run: |

# Check recorded hash          for script in scripts/*.sh; do

cat /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh            echo "Verifying: $script"

```            verify -file "$script" \

              -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

#### Permission Denied          done



**Error Message:**      - name: Report failure

```        if: failure()

Error: permission denied: cannot read file        run: |

```          echo "File integrity verification failed!"

          echo "Some files may have been modified without updating hashes."

**Solution:**          exit 1

- Check file read permissions```

- Ensure sufficient privileges to access the file

- Check if file exists### 4.5 デプロイ前の検証



### 5.2 Debug Methods**デプロイスクリプト統合**



#### Verbose Output```bash

#!/bin/bash

```bash# deploy.sh - デプロイメントスクリプト

# Enable detailed logging (if supported)

verify -file /usr/bin/backup.sh -hash-dir ./hashes -verboseset -e  # エラーで即座に終了

```

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

#### Manual Hash ComparisonCONFIG_FILE="/etc/go-safe-cmd-runner/deploy.toml"



```bashecho "=== Pre-deployment Verification ==="

# Calculate current hash

CURRENT_HASH=$(sha256sum /usr/bin/backup.sh | cut -d' ' -f1)# 設定ファイルを検証

echo "Verifying configuration file..."

# Read recorded hashif ! verify -file "$CONFIG_FILE" -hash-dir "$HASH_DIR"; then

RECORDED_HASH=$(cat ./hashes/~usr~bin~backup.sh)    echo "Error: Configuration file verification failed"

    echo "Possible causes:"

# Compare    echo "  - Configuration file has been modified"

if [[ "$CURRENT_HASH" == "$RECORDED_HASH" ]]; then    echo "  - Hash file is outdated"

    echo "Hashes match"    echo "  - Hash file is missing"

else    exit 1

    echo "Hash mismatch"fi

    echo "Current:  $CURRENT_HASH"

    echo "Recorded: $RECORDED_HASH"# デプロイスクリプトを検証

fiecho "Verifying deployment scripts..."

```SCRIPTS=(

    "/usr/local/bin/deploy-app.sh"

### 5.3 Recovery Procedures    "/usr/local/bin/migrate-db.sh"

    "/usr/local/bin/restart-services.sh"

#### After File Update)



```bashfor script in "${SCRIPTS[@]}"; do

# If file was legitimately updated    echo "  Checking: $script"

record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force    if ! verify -file "$script" -hash-dir "$HASH_DIR"; then

        echo "  Error: Script verification failed"

# Verify new hash        exit 1

verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes    fi

```done



#### Batch Recoveryecho "All verifications passed!"

echo ""

```bashecho "=== Running Deployment ==="

# Re-record all files in directory

for file in /usr/local/bin/*.sh; do# デプロイを実行

    echo "Re-recording: $file"runner -config "$CONFIG_FILE" -log-dir /var/log/runner

    record -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force

doneecho "Deployment completed successfully!"

``````



## 6. Related Documentation### 4.6 バッチ検証とレポート生成



- [record Command Guide](record_command.md) - Hash file creation**詳細レポート付き検証スクリプト**

- [runner Command Guide](runner_command.md) - Main execution command

- [Hash File Naming ADR](../dev/hash-file-naming-adr.md) - Technical details of hash files```bash

- [Security Architecture](../dev/security-architecture.md) - Overall security design#!/bin/bash

- [Project README](../../README.md) - Installation and overview# batch-verify-with-report.sh

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
Error: file not found: /usr/bin/backup.sh
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
Error: hash file not found
```

**対処法**

**原因1: ハッシュがまだ記録されていない**

```bash
# ハッシュを記録
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

**原因2: 間違ったハッシュディレクトリを指定**

```bash
# ハッシュファイルを検索
find /usr/local/etc/go-safe-cmd-runner -name "*backup.sh*"

# 正しいディレクトリで再度検証
verify -file /usr/bin/backup.sh -hash-dir /path/to/correct/hash-dir
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
Verification failed: hash mismatch
Expected: abc123def456789...
Got:      def456abc123xyz...
```

**原因と対処法**

**原因1: ファイルが更新された**

```bash
# ファイルの更新日時を確認
ls -l /usr/bin/backup.sh

# ファイルが意図的に更新された場合、ハッシュを再記録
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

**原因2: ファイルが改ざんされた**

```bash
# ファイルのバックアップから復元
sudo cp /backup/usr/bin/backup.sh /usr/bin/backup.sh
sudo chown root:root /usr/bin/backup.sh
sudo chmod 755 /usr/bin/backup.sh

# 検証を再実行
verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
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
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

### 5.4 権限エラー

**エラーメッセージ**
```
Error: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**対処法**

```bash
# ディレクトリの権限確認
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# 読み取り権限を追加
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# またはsudoで実行
sudo verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 5.5 シンボリックリンクの検証

**動作**

シンボリックリンクを指定した場合、リンク先のファイルが検証されます。

```bash
# シンボリックリンクの検証
verify -file /usr/local/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# リンク先のファイルのハッシュと比較されます
```

**注意事項**

- ハッシュファイル名はシンボリックリンクのパスに基づいて生成されます
- リンク先が変更された場合、ハッシュの再記録が必要です

```bash
# リンク先を確認
ls -lL /usr/local/bin/backup.sh

# リンク先が変更された場合はハッシュを再記録
record -file /usr/local/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

### 5.6 スクリプトでのエラーハンドリング

**終了コードを使用した適切なエラーハンドリング**

```bash
#!/bin/bash
# robust-verification.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
FILE="/usr/bin/backup.sh"

# 検証を実行し、エラーを詳細に処理
if verify -file "$FILE" -hash-dir "$HASH_DIR" 2>&1 | tee /tmp/verify-output.txt; then
    echo "Verification passed: $FILE"
else
    EXIT_CODE=$?
    echo "Verification failed: $FILE"
    echo "Exit code: $EXIT_CODE"
    echo "Output:"
    cat /tmp/verify-output.txt

    # エラーの種類に応じた処理
    if grep -q "file not found" /tmp/verify-output.txt; then
        echo "Error: File does not exist"
    elif grep -q "hash file not found" /tmp/verify-output.txt; then
        echo "Error: Hash has not been recorded"
        echo "Run: record -file $FILE -hash-dir $HASH_DIR"
    elif grep -q "hash mismatch" /tmp/verify-output.txt; then
        echo "Error: File has been modified"
        echo "Current hash:"
        sha256sum "$FILE"
    fi

    exit 1
fi
```

## 6. 関連ドキュメント

### コマンドラインツール

- [runner コマンドガイド](runner_command.md) - メインの実行コマンド
- [record コマンドガイド](record_command.md) - ハッシュファイルの作成（管理者向け）

### 設定ファイル

- [TOML設定ファイル ユーザーガイド](toml_config/README.md)
  - [グローバルレベル設定](toml_config/04_global_level.md) - `verify_files` パラメータ
  - [グループレベル設定](toml_config/05_group_level.md) - グループごとのファイル検証
  - [トラブルシューティング](toml_config/10_troubleshooting.md) - 検証エラーの対処法

### プロジェクト情報

- [README.md](../../README.md) - プロジェクト概要
- [開発者向けドキュメント](../dev/) - ファイル検証アーキテクチャの詳細

---

**最終更新**: 2025-10-02
