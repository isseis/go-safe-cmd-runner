# verify Command User Guide

This guide explains how to use the `verify` command to validate file integrity.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Basic Usage](#2-basic-usage)
- [3. Command-Line Flags Explained](#3-command-line-flags-explained)
- [4. Practical Examples](#4-practical-examples)
- [5. Troubleshooting](#5-troubleshooting)
- [6. Related Documents](#6-related-documents)

## 1. Overview

### 1.1 What is the verify command?

The `verify` command calculates the current SHA-256 hash of a file and compares it with a previously recorded hash value to verify the file's integrity.

### 1.2 Main Uses

- **Debugging**: Investigating the cause of file verification errors.
- **Manual Verification**: Individually checking the integrity of specific files.
- **Troubleshooting**: Identifying issues before running the `runner` command.
- **Auditing**: Confirming that files have not been tampered with.

### 1.3 How it works

```
1. Calculate the SHA-256 hash of the specified file.
   ↓
2. Search for the corresponding hash file in the hash directory.
   ↓
3. Compare the recorded hash with the current hash.
   ↓
4. Success if they match, error if they don't.
```

### 1.4 Relationship with the runner command

The `runner` command automatically performs file verification internally, but the `verify` command is useful in the following cases:

- **Pre-check**: To confirm there are no issues before running `runner`.
- **Error Investigation**: To check the details of a verification error.
- **Individual Verification**: To verify only specific files.

## 2. Basic Usage

### 2.1 Simplest Use Case

```bash
# Verify using the hash file in the current directory
verify -file /usr/bin/backup.sh
```

Output on success:
```
OK: /usr/bin/backup.sh
```

Output on failure:
```
Verification failed: hash mismatch
Expected: abc123def456...
Got:      def456abc123...
```

### 2.2 Specifying a Hash Directory

```bash
# Use hash files from a specific directory
verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 2.3 Verifying Multiple Files

```bash
# Verify multiple files with a script
for file in /usr/local/bin/*.sh; do
    echo "Verifying: $file"
    verify -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes || {
        echo "Verification failed for: $file"
    }
done
```

### 2.4 Checking with Exit Codes

```bash
# Determine the verification result by the exit code
if verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes; then
    echo "File is valid"
else
    echo "File verification failed"
    exit 1
fi
```

## 3. Command-Line Flags Explained

### 3.1 `-file <path>` (Required)

**Overview**

Specifies the path to the file to be verified.

**Syntax**

```bash
verify -file <path>
```

**Parameters**

- `<path>`: Absolute or relative path to the file you want to verify (required).

**Examples**

```bash
# Specify with an absolute path
verify -file /usr/bin/backup.sh

# Specify with a relative path
verify -file ./scripts/deploy.sh

# File in the home directory
verify -file ~/bin/custom-script.sh
```

**Notes**

- An error will occur if the file does not exist.
- An error will also occur if the corresponding hash file does not exist.
- In the case of a symbolic link, the linked file will be verified.

### 3.2 `-hash-dir <directory>` (Optional)

**Overview**

Specifies the directory where hash files are stored. If not specified, the current directory is used.

**Syntax**

```bash
verify -file <path> -hash-dir <directory>
```

**Parameters**

- `<directory>`: Path to the directory where hash files are stored (optional).
- Default: Current directory.

**Examples**

```bash
# Use the standard hash directory
verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# Use a custom directory (for testing)
verify -file ./test.sh -hash-dir ./test-hashes

# Specify with a relative path
verify -file /etc/config.toml -hash-dir ../hashes
```

**Hash File Search**

The `verify` command automatically generates and searches for the hash file name from the specified file path:

```bash
# For /usr/bin/backup.sh
# Hash file: <hash-dir>/~usr~bin~backup.sh

verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
# The file actually searched for:
# /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

**Notes**

- An error will occur if the hash directory does not exist.
- An error will also occur if the corresponding hash file is not found.
- Please specify the same hash directory as the `record` command.

## 4. Practical Examples

### 4.1 Pre-check before running runner

**Script to verify all files**

```bash
#!/bin/bash
# verify-all.sh - Pre-verification before running runner

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
CONFIG_FILE="/etc/go-safe-cmd-runner/backup.toml"

# Verify the configuration file
echo "Verifying configuration file..."
if ! verify -file "$CONFIG_FILE" -hash-dir "$HASH_DIR"; then
    echo "Error: Configuration file verification failed"
    exit 1
fi

# Extract and verify verify_files from TOML configuration (manually specified)
FILES=(
    "/usr/local/bin/backup.sh"
    "/usr/local/bin/cleanup.sh"
    "/usr/bin/rsync"
)

echo "Verifying executable files..."
for file in "${FILES[@]}"; do
    echo "  Checking: $file"
    if ! verify -file "$file" -hash-dir "$HASH_DIR"; then
        echo "  Error: Verification failed for $file"
        exit 1
    fi
done

echo "All files verified successfully!"
echo "You can now run: runner -config $CONFIG_FILE"
```

### 4.2 Investigating Verification Errors

**Getting detailed error information**

```bash
#!/bin/bash
# investigate-verification-failure.sh

FILE="/usr/bin/backup.sh"
HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

echo "=== File Verification Investigation ==="
echo "File: $FILE"
echo ""

# Check if the file exists
if [[ ! -f "$FILE" ]]; then
    echo "Error: File does not exist"
    exit 1
fi

# Display file information
echo "File information:"
ls -l "$FILE"
echo ""

# Calculate the current hash
echo "Current hash:"
sha256sum "$FILE"
echo ""

# Display the recorded hash
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
verify -file "$FILE" -hash-dir "$HASH_DIR"
```

### 4.3 Periodic Integrity Checks

**Periodic execution with cron**

```bash
#!/bin/bash
# periodic-integrity-check.sh - Periodic integrity check

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
LOG_FILE="/var/log/integrity-check.log"

# Record a timestamp in the log file
echo "=== Integrity Check: $(date) ===" >> "$LOG_FILE"

# List of critical files
CRITICAL_FILES=(
    "/usr/local/bin/backup.sh"
    "/usr/local/bin/deploy.sh"
    "/etc/go-safe-cmd-runner/backup.toml"
    "/usr/bin/rsync"
)

FAILED=0

for file in "${CRITICAL_FILES[@]}"; do
    if verify -file "$file" -hash-dir "$HASH_DIR" >> "$LOG_FILE" 2>&1; then
        echo "OK: $file" >> "$LOG_FILE"
    else
        echo "FAILED: $file" >> "$LOG_FILE"
        FAILED=1

        # Alert processing such as Slack notifications
        # send-alert.sh "$file verification failed"
    fi
done

if [[ $FAILED -eq 1 ]]; then
    echo "Integrity check failed. See $LOG_FILE for details" >&2
    exit 1
else
    echo "All files verified successfully" >> "$LOG_FILE"
fi
```

**crontab entry**

```bash
# crontab -e
# Run integrity check every day at 3 AM
0 3 * * * /usr/local/sbin/periodic-integrity-check.sh
```

### 4.4 Verification in CI/CD

**Example usage in GitHub Actions**

```yaml
name: Verify File Integrity

on:
  schedule:
    # Run every day at midnight
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
          verify -file config/backup.toml \
            -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

      - name: Verify scripts
        run: |
          for script in scripts/*.sh; do
            echo "Verifying: $script"
            verify -file "$script" \
              -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
          done

      - name: Report failure
        if: failure()
        run: |
          echo "File integrity verification failed!"
          echo "Some files may have been modified without updating hashes."
          exit 1
```

### 4.5 Pre-deployment Verification

**Deployment script integration**

```bash
#!/bin/bash
# deploy.sh - Deployment script

set -e  # Exit immediately on error

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
CONFIG_FILE="/etc/go-safe-cmd-runner/deploy.toml"

echo "=== Pre-deployment Verification ==="

# Verify the configuration file
echo "Verifying configuration file..."
if ! verify -file "$CONFIG_FILE" -hash-dir "$HASH_DIR"; then
    echo "Error: Configuration file verification failed"
    echo "Possible causes:"
    echo "  - Configuration file has been modified"
    echo "  - Hash file is outdated"
    echo "  - Hash file is missing"
    exit 1
fi

# Verify deployment scripts
echo "Verifying deployment scripts..."
SCRIPTS=(
    "/usr/local/bin/deploy-app.sh"
    "/usr/local/bin/migrate-db.sh"
    "/usr/local/bin/restart-services.sh"
)

for script in "${SCRIPTS[@]}"; do
    echo "  Checking: $script"
    if ! verify -file "$script" -hash-dir "$HASH_DIR"; then
        echo "  Error: Script verification failed"
        exit 1
    fi
done

echo "All verifications passed!"
echo ""
echo "=== Running Deployment ==="

# Run deployment
runner -config "$CONFIG_FILE" -log-dir /var/log/runner

echo "Deployment completed successfully!"
```

### 4.6 Batch Verification and Report Generation

**Verification script with detailed report**

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

    if verify -file "$file" -hash-dir "$HASH_DIR" 2>/dev/null; then
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

# Display the report
cat "$REPORT_FILE"

# Set exit code based on the result
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

### 5.1 File not found

**Error message**
```
Error: file not found: /usr/bin/backup.sh
```

**Solution**

```bash
# Check if the file exists
ls -l /usr/bin/backup.sh

# Check for typos in the path
which backup.sh

# If it's a symbolic link, check the destination
ls -lL /usr/bin/backup.sh
```

### 5.2 Hash file not found

**Error message**
```
Error: hash file not found
```

**Solution**

**Cause 1: Hash has not been recorded yet**

```bash
# Record the hash
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

**Cause 2: Specified the wrong hash directory**

```bash
# Search for the hash file
find /usr/local/etc/go-safe-cmd-runner -name "*backup.sh*"

# Re-verify with the correct directory
verify -file /usr/bin/backup.sh -hash-dir /path/to/correct/hash-dir
```

**Cause 3: Problem with the hash file name**

```bash
# Check the contents of the hash directory
ls -la /usr/local/etc/go-safe-cmd-runner/hashes/

# Check the expected hash file name
# /usr/bin/backup.sh → ~usr~bin~backup.sh
```

### 5.3 Hash mismatch

**Error message**
```
Verification failed: hash mismatch
Expected: abc123def456789...
Got:      def456abc123xyz...
```

**Causes and Solutions**

**Cause 1: The file has been updated**

```bash
# Check the file's modification date
ls -l /usr/bin/backup.sh

# If the file was intentionally updated, re-record the hash
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

**Cause 2: The file has been tampered with**

```bash
# Restore the file from a backup
sudo cp /backup/usr/bin/backup.sh /usr/bin/backup.sh
sudo chown root:root /usr/bin/backup.sh
sudo chmod 755 /usr/bin/backup.sh

# Re-run verification
verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

**Cause 3: The hash file is outdated**

```bash
# Check the hash file's date
HASH_FILE="/usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh"
ls -l "$HASH_FILE"

# Compare the dates of the file and the hash
echo "File:"; ls -l /usr/bin/backup.sh
echo "Hash:"; ls -l "$HASH_FILE"

# If the hash is old, re-record it
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

### 5.4 Permission error

**Error message**
```
Error: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**Solution**

```bash
# Check directory permissions
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# Add read permissions
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# Or run with sudo
sudo verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 5.5 Verifying Symbolic Links

**Behavior**

If you specify a symbolic link, the file it points to will be verified.

```bash
# Verifying a symbolic link
verify -file /usr/local/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# It will be compared with the hash of the linked file
```

**Notes**

- The hash file name is generated based on the path of the symbolic link.
- If the link destination changes, you need to re-record the hash.

```bash
# Check the link destination
ls -lL /usr/local/bin/backup.sh

# If the link destination has changed, re-record the hash
record -file /usr/local/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

### 5.6 Error Handling in Scripts

**Proper error handling using exit codes**

```bash
#!/bin/bash
# robust-verification.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
FILE="/usr/bin/backup.sh"

# Run verification and handle errors in detail
if verify -file "$FILE" -hash-dir "$HASH_DIR" 2>&1 | tee /tmp/verify-output.txt; then
    echo "Verification passed: $FILE"
else
    EXIT_CODE=$?
    echo "Verification failed: $FILE"
    echo "Exit code: $EXIT_CODE"
    echo "Output:"
    cat /tmp/verify-output.txt

    # Process based on the type of error
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

## 6. Related Documents

### Command-Line Tools

- [runner Command Guide](runner_command.md) - The main execution command
- [record Command Guide](record_command.md) - Creating hash files (for administrators)

### Configuration Files

- [TOML Configuration File User Guide](toml_config/README.md)
  - [Global Level Settings](toml_config/04_global_level.md) - `verify_files` parameter
  - [Group Level Settings](toml_config/05_group_level.md) - File verification per group
  - [Troubleshooting](toml_config/10_troubleshooting.md) - Dealing with verification errors

### Project Information

- [README.md](../../README.md) - Project overview
- [Developer Documentation](../dev/) - Details on the file verification architecture

---

**Last Updated**: 2025-10-03
