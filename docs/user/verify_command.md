# verify Command User Guide

Guide to using the `verify` command for verifying file integrity.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Basic Usage](#2-basic-usage)
- [3. Command-Line Flags Reference](#3-command-line-flags-reference)
- [4. Practical Examples](#4-practical-examples)
- [5. Troubleshooting](#5-troubleshooting)
- [6. Related Documentation](#6-related-documentation)

## 1. Overview

### 1.1 What is the verify Command

The `verify` command calculates the current SHA-256 hash value of a file and verifies its integrity by comparing it with the pre-recorded hash value.

### 1.2 Main Use Cases

- **Debugging**: Investigating the cause of file verification errors
- **Manual Verification**: Individually checking the integrity of specific files
- **Troubleshooting**: Identifying issues before running the `runner` command
- **Auditing**: Confirming that files have not been tampered with

### 1.3 How It Works

```
1. Calculate SHA-256 hash value of the specified file
   ↓
2. Search for the corresponding hash file in the hash directory
   ↓
3. Compare the recorded hash value with the current hash value
   ↓
4. Success if matched, error if mismatched
```

### 1.4 Relationship with the runner Command

While the `runner` command automatically performs file verification internally, the `verify` command is useful in the following cases:

- **Pre-check**: Confirm there are no issues before running `runner`
- **Error Investigation**: Check details of verification errors
- **Individual Verification**: Verify only specific files

## 2. Basic Usage

### 2.1 Simplest Usage Example

```bash
# Verify using hash files in the current directory
verify /usr/bin/backup.sh
```

Output on success:
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: OK

Summary: 1 succeeded, 0 failed
```

Output on failure:
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: hash mismatch

Summary: 0 succeeded, 1 failed
```

### 2.2 Specifying Hash Directory

```bash
# Use hash files from a specific directory
verify -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# Short form
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 2.3 Verifying Multiple Files

```bash
# Specify multiple files directly (recommended)
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh /usr/local/bin/deploy.sh

# Using wildcards
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

### 2.4 Determining Results by Exit Code

```bash
# Determine verification results by exit code
if verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh; then
    echo "File is valid"
else
    echo "File verification failed"
    exit 1
fi
```

## 3. Command-Line Flags Reference

### 3.1 File Specification (Positional Arguments)

**Overview**

Specifies the files to verify as positional arguments. Multiple files can be specified simultaneously.

**Syntax**

```bash
verify [flags] <file> [<file>...]
```

**Parameters**

- `<file>`: Absolute or relative path to the file to verify (one or more required)

**Usage Examples**

```bash
# Specify with absolute path
verify /usr/bin/backup.sh

# Specify with relative path
verify ./scripts/deploy.sh

# File in home directory
verify ~/bin/custom-script.sh

# Specify multiple files
verify /usr/bin/backup.sh /usr/bin/restore.sh

# Using wildcards
verify /usr/local/bin/*.sh
```

**Notes**

- Error occurs if the file does not exist
- Error also occurs if the corresponding hash file does not exist
- For symbolic links, the target file is verified

### 3.2 `-hash-dir <directory>` / `-d <directory>` (Optional)

**Overview**

Specifies the directory where hash files are stored. If not specified, the current directory is used.

**Syntax**

```bash
verify -hash-dir <directory> <file>...
verify -d <directory> <file>...
```

**Parameters**

- `<directory>`: Directory path where hash files are stored (optional)
- Default: Current directory

**Usage Examples**

```bash
# Use standard hash directory
verify -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# Short form
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# Use custom directory (for testing)
verify -d ./test-hashes ./test.sh

# Specify with relative path
verify -d ../hashes /etc/config.toml
```

**Hash File Search**

The `verify` command automatically generates the hash filename from the specified file path and searches for it:

```bash
# For /usr/bin/backup.sh
# Hash file: <hash-dir>/~usr~bin~backup.sh

verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
# Actually searched file:
# /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

**Notes**

- Error occurs if the hash directory does not exist
- Error also occurs if the corresponding hash file is not found
- Specify the same hash directory as used with the `record` command

## 4. Practical Examples

### 4.1 Pre-check Before runner Execution

**Script to Verify All Files**

```bash
#!/bin/bash
# verify-all.sh - Pre-verification before runner execution

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
CONFIG_FILE="/etc/go-safe-cmd-runner/backup.toml"

# Batch verify configuration file and executable files
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

### 4.2 Investigating Verification Errors

**Getting Detailed Error Information**

```bash
#!/bin/bash
# investigate-verification-failure.sh

FILE="/usr/bin/backup.sh"
HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

echo "=== File Verification Investigation ==="
echo "File: $FILE"
echo ""

# Check file existence
if [[ ! -f "$FILE" ]]; then
    echo "Error: File does not exist"
    exit 1
fi

# Display file information
echo "File information:"
ls -l "$FILE"
echo ""

# Calculate current hash value
echo "Current hash:"
sha256sum "$FILE"
echo ""

# Display recorded hash value
HASH_FILE="${HASH_DIR}/~usr~bin~backup.sh"
echo "Recorded hash:"
if [[ -f "$HASH_FILE" ]]; then
    cat "$HASH_FILE"
    echo ""
else
    echo "Hash file not found: $HASH_FILE"
    exit 1
fi

# Run verification
echo "Running verification:"
verify -d "$HASH_DIR" "$FILE"
```

### 4.3 Periodic Integrity Check

**Run Periodically with cron**

```bash
#!/bin/bash
# periodic-integrity-check.sh - Periodic integrity check

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
LOG_FILE="/var/log/integrity-check.log"

# Record timestamp in log file
echo "=== Integrity Check: $(date) ===" >> "$LOG_FILE"

# List of critical files
CRITICAL_FILES=(
    "/usr/local/bin/backup.sh"
    "/usr/local/bin/deploy.sh"
    "/etc/go-safe-cmd-runner/backup.toml"
    "/usr/bin/rsync"
)

# Execute batch verification
if verify -d "$HASH_DIR" "${CRITICAL_FILES[@]}" >> "$LOG_FILE" 2>&1; then
    echo "All files verified successfully" >> "$LOG_FILE"
else
    echo "Integrity check failed. See $LOG_FILE for details" >&2
    # Alert handling such as Slack notification
    # send-alert.sh "Integrity check failed"
    exit 1
fi
```

**crontab Entry**

```bash
# crontab -e
# Run integrity check daily at 3:00 AM
0 3 * * * /usr/local/sbin/periodic-integrity-check.sh
```

### 4.4 Verification in CI/CD

**Usage Example in GitHub Actions**

```yaml
name: Verify File Integrity

on:
  schedule:
    # Run daily at midnight
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

### 4.5 Pre-deployment Verification

**Integration with Deployment Script**

```bash
#!/bin/bash
# deploy.sh - Deployment script

set -e  # Exit immediately on error

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
CONFIG_FILE="/etc/go-safe-cmd-runner/deploy.toml"

echo "=== Pre-deployment Verification ==="

# Batch verify configuration file and deployment scripts
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

# Execute deployment
runner -config "$CONFIG_FILE" -log-dir /var/log/runner

echo "Deployment completed successfully!"
```

### 4.6 Batch Verification with Report Generation

**Verification Script with Detailed Report**

```bash
#!/bin/bash
# batch-verify-with-report.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
REPORT_FILE="verification-report-$(date +%Y%m%d-%H%M%S).txt"

echo "=== File Integrity Verification Report ===" > "$REPORT_FILE"
echo "Date: $(date)" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# List of files to verify
mapfile -t FILES < <(find /usr/local/bin -name "*.sh")
FILES+=("/etc/go-safe-cmd-runner/backup.toml")
FILES+=("/etc/go-safe-cmd-runner/deploy.toml")

TOTAL=0
PASSED=0
FAILED=0

for file in "${FILES[@]}"; do
    TOTAL=$((TOTAL + 1))

    if verify -d "$HASH_DIR" "$file" 2>/dev/null; then
        echo "✓ PASS: $file" >> "$REPORT_FILE"
        PASSED=$((PASSED + 1))
    else
        echo "✗ FAIL: $file" >> "$REPORT_FILE"
        FAILED=$((FAILED + 1))

        # Record error details
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

# Summary
{
    echo ""
    echo "=== Summary ==="
    echo "Total files: $TOTAL"
    echo "Passed: $PASSED"
    echo "Failed: $FAILED"
} >> "$REPORT_FILE"

# Display report
cat "$REPORT_FILE"

# Set exit code based on results
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

## 5. Troubleshooting

### 5.1 File Not Found

**Error Message**
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: file not found
```

**Solutions**

```bash
# Check file existence
ls -l /usr/bin/backup.sh

# Check for typos in path
which backup.sh

# For symbolic links, check the target
ls -lL /usr/bin/backup.sh
```

### 5.2 Hash File Not Found

**Error Message**
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: hash file not found
```

**Solutions**

**Cause 1: Hash has not been recorded yet**

```bash
# Record hash
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**Cause 2: Wrong hash directory specified**

```bash
# Search for hash file
find /usr/local/etc/go-safe-cmd-runner -name "*backup.sh*"

# Verify again with correct directory
verify -d /path/to/correct/hash-dir /usr/bin/backup.sh
```

**Cause 3: Hash filename issue**

```bash
# Check contents of hash directory
ls -la /usr/local/etc/go-safe-cmd-runner/hashes/

# Check expected hash filename
# /usr/bin/backup.sh → ~usr~bin~backup.sh
```

### 5.3 Hash Mismatch

**Error Message**
```
Verifying 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Verification failed for /usr/bin/backup.sh: hash mismatch
```

**Causes and Solutions**

**Cause 1: File has been updated**

```bash
# Check file modification time
ls -l /usr/bin/backup.sh

# If file was intentionally updated, re-record hash
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**Cause 2: File has been tampered with**

```bash
# Restore from file backup
sudo cp /backup/usr/bin/backup.sh /usr/bin/backup.sh
sudo chown root:root /usr/bin/backup.sh
sudo chmod 755 /usr/bin/backup.sh

# Re-run verification
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**Cause 3: Hash file is outdated**

```bash
# Check hash file timestamp
HASH_FILE="/usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh"
ls -l "$HASH_FILE"

# Compare timestamps of file and hash
echo "File:"; ls -l /usr/bin/backup.sh
echo "Hash:"; ls -l "$HASH_FILE"

# Re-record if hash is outdated
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.4 Permission Error

**Error Message**
```
Error: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**Solutions**

```bash
# Check directory permissions
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# Add read permission
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# Or run with sudo
sudo verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.5 Symbolic Link Verification

**Behavior**

When a symbolic link is specified, the target file is verified.

```bash
# Verify symbolic link
verify -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh

# Compared with the target file's hash
```

**Notes**

- Hash filename is generated based on the symbolic link's path
- If the target changes, hash must be re-recorded

```bash
# Check link target
ls -lL /usr/local/bin/backup.sh

# Re-record hash if target has changed
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh
```

### 5.6 Error Handling in Scripts

**Proper Error Handling Using Exit Codes**

```bash
#!/bin/bash
# robust-verification.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
FILE="/usr/bin/backup.sh"

# Run verification and handle errors in detail
verify -d "$HASH_DIR" "$FILE" 2>&1 | tee /tmp/verify-output.txt
EXIT_CODE=${PIPESTATUS[0]}
if [[ $EXIT_CODE -eq 0 ]]; then
    echo "Verification passed: $FILE"
else
    echo "Verification failed: $FILE"
    echo "Exit code: $EXIT_CODE"
    echo "Output:"
    cat /tmp/verify-output.txt

    # Process based on error type
    if grep -q "file not found" /tmp/verify-output.txt; then
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

## 6. Related Documentation

### Command-Line Tools

- [runner Command Guide](runner_command.md) - Main execution command
- [record Command Guide](record_command.md) - Hash file creation (for administrators)

### Configuration Files

- [TOML Configuration File User Guide](toml_config/README.md)
  - [Global Level Configuration](toml_config/04_global_level.md) - `verify_files` parameter
  - [Group Level Configuration](toml_config/05_group_level.md) - File verification per group
  - [Troubleshooting](toml_config/11_troubleshooting.md) - Handling verification errors

### Project Information

- [README.md](../../README.md) - Project overview
- [Developer Documentation](../dev/) - File verification architecture details

---

**Last Updated**: 2025-10-02
