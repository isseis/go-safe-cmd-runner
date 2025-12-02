# record Command User Guide

This guide explains how to use the `record` command to record SHA-256 hash values of files.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Basic Usage](#2-basic-usage)
- [3. Command-Line Flags](#3-command-line-flags)
- [4. Practical Examples](#4-practical-examples)
- [5. Troubleshooting](#5-troubleshooting)
- [6. Related Documentation](#6-related-documentation)

## 1. Overview

### 1.1 What is the record Command

The `record` command calculates the SHA-256 hash value of a file and saves it to the hash directory. This hash value is later used by the `runner` command or `verify` command to verify file integrity.

### 1.2 Main Use Cases

- **Security**: Tampering detection for executable binaries and scripts
- **Integrity Assurance**: Change detection for configuration files and environment files
- **Auditing**: File version management and tracking

### 1.3 How It Works

```
1. Calculate SHA-256 hash value of the file
   ↓
2. Encode the file path to generate a hash file name
   ↓
3. Save the hash value to the hash directory
   ↓
4. Display the saved hash file name
```

### 1.4 Hash File Naming Convention

The record command uses a hybrid encoding scheme to generate hash file names:

**For Short Paths (Replacement Encoding)**

```
/usr/bin/backup.sh → ~usr~bin~backup.sh
/etc/config.toml   → ~etc~config.toml
```

**For Long Paths (SHA-256 Fallback)**

```
/very/long/path/to/file.sh → AbCdEf123456.json
```

This approach makes hash file names human-readable while also handling file name length limitations.

### 1.5 Usage Scenarios

- **Initial Setup**: Record hashes of executable files during system deployment
- **After File Updates**: Re-record hashes after updating scripts or configuration files
- **Regular Updates**: Update hashes after system package updates

## 2. Basic Usage

### 2.1 Simplest Usage Example

```bash
# Record hash to current directory
record /usr/bin/backup.sh
```

Output:
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: OK (~usr~bin~backup.sh)

Summary: 1 succeeded, 0 failed
```

### 2.2 Specify Hash Directory

```bash
# Record hash to a specific directory
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# Short form
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

Output:
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: OK (~usr~bin~backup.sh)

Summary: 1 succeeded, 0 failed
```

### 2.3 Overwrite Existing Hash

```bash
# Forcefully overwrite existing hash file
record -force -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 2.4 Batch Recording of Multiple Files

```bash
# Specify multiple files directly (recommended)
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh /usr/local/bin/deploy.sh

# Using wildcards
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

## 3. Command-Line Flags

### 3.1 File Specification (Positional Arguments)

**Overview**

Specifies the files whose hash values should be recorded as positional arguments. Multiple files can be specified simultaneously.

**Syntax**

```bash
record [flags] <file> [<file>...]
```

**Parameters**

- `<file>`: Absolute or relative path to the file for which to record the hash (one or more required)

**Usage Examples**

```bash
# Specify with absolute path
record /usr/bin/backup.sh

# Specify with relative path
record ./scripts/deploy.sh

# File in home directory
record ~/bin/custom-script.sh

# Specify multiple files
record /usr/bin/backup.sh /usr/bin/restore.sh

# Using wildcards
record /usr/local/bin/*.sh
```

**Notes**

- An error occurs if the file does not exist
- For symbolic links, the hash of the target file is recorded
- Directories cannot be specified (files only)

### 3.2 `-hash-dir <directory>` / `-d <directory>` (Optional)

**Overview**

Specifies the directory where the hash file should be saved. If not specified, the current directory is used.

**Syntax**

```bash
record -hash-dir <directory> <file>...
record -d <directory> <file>...
```

**Parameters**

- `<directory>`: Directory path where the hash file will be saved (optional)
- Default: Current directory

**Usage Examples**

```bash
# Save to standard hash directory
record -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# Short form
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh

# Save to custom directory (for testing)
record -d ./test-hashes ./test.sh

# Specify with relative path
record -d ../hashes /etc/config.toml
```

**Automatic Directory Creation**

If the specified directory does not exist, it will be created automatically (permissions: 0750).

```bash
# Works even if directory doesn't exist
record -d /new/hash/directory /usr/bin/backup.sh
# /new/hash/directory will be created automatically
```

**About Permissions**

- Hash directories are created with 0750 permissions (owner: rwx, group: r-x, others: ---)
- Hash files are created with 0640 permissions (owner: rw-, group: r--, others: ---)

**Recommended Settings for Production Environment**

```bash
# Use standard directory in production environment
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# Record hash
sudo record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 3.3 `-force` (Optional)

**Overview**

Forcefully overwrites an existing hash file. If not specified, an error occurs if an existing hash file is present.

**Syntax**

```bash
record -force [-hash-dir <directory>] <file>...
```

**Usage Examples**

**Normal Behavior (Error if Existing File Exists)**

```bash
# First time succeeds
record -d ./hashes /usr/bin/backup.sh

# Second time fails
record -d ./hashes /usr/bin/backup.sh
# Error: hash file already exists: ./hashes/~usr~bin~backup.sh
```

**Using -force Flag**

```bash
# Overwrite existing hash file
record -force -d ./hashes /usr/bin/backup.sh
```

**Use Cases**

- **After File Updates**: Re-record hashes after updating scripts or binaries
- **Forced Re-sync**: Recovery when hash files are corrupted or accidentally deleted
- **Batch Updates**: Scripts that update hashes of multiple files at once

**Usage Example: Batch Update**

```bash
# Forcefully re-record hashes of all scripts
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

**Notes**

- The `-force` flag overwrites existing hash files without warning
- Be careful not to accidentally overwrite important hash files
- In production environments, it is recommended to take backups before use

## 4. Practical Examples

### 4.1 Initial Setup

**Hash Recording During System Deployment**

```bash
#!/bin/bash
# setup-hashes.sh - Initial hash recording script

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

# Create hash directory
sudo mkdir -p "$HASH_DIR"
sudo chown root:root "$HASH_DIR"
sudo chmod 755 "$HASH_DIR"

# Record hashes of configuration files
echo "Recording configuration files..."
sudo record -d "$HASH_DIR" /etc/go-safe-cmd-runner/backup.toml /etc/go-safe-cmd-runner/deploy.toml

# Record hashes of executable scripts
echo "Recording executable scripts..."
sudo record -d "$HASH_DIR" /usr/local/bin/backup.sh /usr/local/bin/deploy.sh /usr/local/bin/cleanup.sh

# Record hashes of system binaries
echo "Recording system binaries..."
sudo record -d "$HASH_DIR" /usr/bin/rsync /usr/bin/pg_dump

echo "Hash recording completed successfully!"
```

### 4.2 Re-recording Hash After File Updates

**Procedure When Updating Scripts**

```bash
# 1. Create backup
sudo cp /usr/local/bin/backup.sh /usr/local/bin/backup.sh.bak

# 2. Edit script
sudo vim /usr/local/bin/backup.sh

# 3. Re-record hash
sudo record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh

# 4. Verify operation
runner -config /etc/go-safe-cmd-runner/backup.toml -dry-run
```

### 4.3 Batch Recording of Multiple Files

**Record All Scripts in a Directory**

```bash
#!/bin/bash
# record-all-scripts.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
SCRIPT_DIR="/usr/local/bin"

# Record all .sh files
echo "Recording scripts in $SCRIPT_DIR..."
sudo record -force -d "$HASH_DIR" "$SCRIPT_DIR"/*.sh

echo "All scripts recorded successfully!"
```

**Record from Configuration File List**

```bash
#!/bin/bash
# record-from-list.sh

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"
FILE_LIST="files-to-record.txt"

# Example contents of file list:
# /usr/local/bin/backup.sh
# /usr/local/bin/deploy.sh
# /etc/config.toml

# Read file list into array and batch record
mapfile -t FILES < <(grep -v '^#' "$FILE_LIST" | grep -v '^$')
if [[ ${#FILES[@]} -gt 0 ]]; then
    sudo record -force -d "$HASH_DIR" "${FILES[@]}"
fi

echo "All files recorded successfully!"
```

### 4.4 Automation and CI/CD Integration

**Hash Recording in GitHub Actions**

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

### 4.5 Hash Updates After Package Updates

**Procedure After System Package Updates**

```bash
#!/bin/bash
# update-system-hashes.sh - Re-record hashes after system updates

HASH_DIR="/usr/local/etc/go-safe-cmd-runner/hashes"

# List of system binaries
BINARIES=(
    "/usr/bin/rsync"
    "/usr/bin/pg_dump"
    "/usr/bin/mysqldump"
    "/usr/bin/tar"
    "/usr/bin/gzip"
)

# Filter to only existing binaries
EXISTING_BINARIES=()
for binary in "${BINARIES[@]}"; do
    if [[ -f "$binary" ]]; then
        EXISTING_BINARIES+=("$binary")
    else
        echo "Warning: $binary not found, skipping"
    fi
done

# Batch record hashes of existing binaries
if [[ ${#EXISTING_BINARIES[@]} -gt 0 ]]; then
    echo "Updating hashes for system binaries..."
    sudo record -force -d "$HASH_DIR" "${EXISTING_BINARIES[@]}"
fi

echo "Hash update completed!"
```

**Periodic Execution with cron**

```bash
# crontab -e
# Update hashes of system binaries every Sunday at 2:00 AM
0 2 * * 0 /usr/local/sbin/update-system-hashes.sh >> /var/log/hash-update.log 2>&1
```

### 4.6 Hash Management in Test Environment

**Independent Hash Directory for Testing**

```bash
#!/bin/bash
# test-setup.sh

TEST_HASH_DIR="./test-hashes"

# Create test hash directory
mkdir -p "$TEST_HASH_DIR"

# Record hashes of test scripts
record -d "$TEST_HASH_DIR" ./test/test-script.sh ./test/test-config.toml

# Run tests
runner -config ./test/test-config.toml -dry-run

echo "Test setup completed!"
```

## 5. Troubleshooting

### 5.1 File Not Found

**Error Message**
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Error recording hash for /usr/bin/backup.sh: file not found
```

**Solution**

```bash
# Check file existence
ls -l /usr/bin/backup.sh

# Check for typos in path
which backup.sh

# For relative paths, check current directory
pwd
```

### 5.2 Permission Error

**Error Message**
```
Error creating validator: permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**Solution**

```bash
# Check directory permissions
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# Fix permissions (requires administrator privileges)
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# Or run record with sudo
sudo record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 5.3 Existing Hash File Present

**Error Message**
```
Processing 1 file...
[1/1] /usr/bin/backup.sh: FAILED
Error recording hash for /usr/bin/backup.sh: hash file already exists
```

**Solution**

**Method 1: Use -force Flag**

```bash
record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**Method 2: Delete Existing Hash File**

```bash
# Delete hash file and re-record
sudo rm /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
sudo record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

**Method 3: Back Up Before Overwriting**

```bash
# Back up existing hash
sudo cp /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh \
       /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh.bak

# Forcefully overwrite
sudo record -force -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/bin/backup.sh
```

### 5.4 Recording Hash of Symbolic Links

**Behavior**

When a symbolic link is specified, the hash of the target file is recorded.

```bash
# Create symbolic link
ln -s /usr/local/bin/backup-v2.sh /usr/local/bin/backup.sh

# Record hash (hash of target file is recorded)
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/backup.sh
```

**Notes**

- The hash file name is generated based on the symbolic link path
- Even if the target file changes, the hash file name remains unchanged
- If the link target is changed, the hash needs to be re-recorded

### 5.5 Directory Specified

**Error Message**
```
Processing 1 file...
[1/1] /usr/local/bin: FAILED
Error recording hash for /usr/local/bin: cannot record hash for directory
```

**Solution**

To record hashes of all files in a directory, use wildcards:

```bash
# Record hashes of all files in directory
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*

# Or only specific extensions
record -d /usr/local/etc/go-safe-cmd-runner/hashes /usr/local/bin/*.sh
```

## 6. Related Documentation

### Command-Line Tools

- [runner Command Guide](runner_command.md) - Main execution command
- [verify Command Guide](verify_command.md) - File integrity verification (for debugging)

### Configuration Files

- [TOML Configuration File User Guide](toml_config/README.md)
  - [Global Level Configuration](toml_config/04_global_level.md) - `verify_files` parameter
  - [Group Level Configuration](toml_config/05_group_level.md) - Per-group file verification

### Project Information

- [README.md](../../README.md) - Project overview
- [Developer Documentation](../dev/) - Details of hash file naming conventions

---

**Last Updated**: 2025-10-02
