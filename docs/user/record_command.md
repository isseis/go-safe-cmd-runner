# record Command User Guide

This guide explains how to use the `record` command for recording SHA-256 hash values of files.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Basic Usage](#2-basic-usage)
- [3. Command Line Flags Details](#3-command-line-flags-details)
- [4. Practical Examples](#4-practical-examples)
- [5. Troubleshooting](#5-troubleshooting)
- [6. Related Documentation](#6-related-documentation)

## 1. Overview

### 1.1 What is the record command?

The `record` command calculates SHA-256 hash values of files and saves them to a hash directory. These hash values are later used by the `runner` and `verify` commands to verify file integrity.

### 1.2 Main Use Cases

- **Security**: Detection of tampering with executable binaries and scripts
- **Integrity Assurance**: Detection of changes in configuration and environment files
- **Auditing**: File version management and tracking

### 1.3 How it Works

```
1. Calculate SHA-256 hash value of the file
   ↓
2. Encode file path to generate hash filename
   ↓
3. Save hash value to hash directory
   ↓
4. Display the saved hash filename
```

### 1.4 Hash File Naming Convention

The record command uses a hybrid encoding scheme to generate hash filenames:

**For Short Paths (Substitution Encoding)**

```
/usr/bin/backup.sh → ~usr~bin~backup.sh
/etc/config.toml   → ~etc~config.toml
```

**For Long Paths (SHA-256 Fallback)**

```
/very/long/path/to/file.sh → AbCdEf123456.json
```

This method ensures hash filenames are human-readable while also accommodating filename length restrictions.

### 1.5 Usage Scenarios

- **Initial Setup**: Record hashes of executable files during system deployment
- **After File Updates**: Re-record hashes after updating scripts or configuration files
- **Regular Updates**: Update hashes after system package updates

## 2. Basic Usage

### 2.1 Simplest Usage Example

```bash
# Record hash in current directory
record -file /usr/bin/backup.sh
```

Output:
```
Recorded hash for /usr/bin/backup.sh in /home/user/~usr~bin~backup.sh
```

### 2.2 Specifying Hash Directory

```bash
# Record hash in specific directory
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

Output:
```
Recorded hash for /usr/bin/backup.sh in /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

### 2.3 Overwriting Existing Hash

```bash
# Force overwrite existing hash file
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force
```

### 2.4 Batch Recording Multiple Files

```bash
# Record multiple files with script
for file in /usr/local/bin/*.sh; do
    record -file "$file" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
done
```

## 3. Command Line Flags Details

### 3.1 `-file <path>` (Required)

**Overview**

Specifies the path to the file for which to record a hash value.

**Syntax**

```bash
record -file <path>
```

**Parameters**

- `<path>`: Absolute or relative path to the file for hash recording (required)

**Usage Examples**

```bash
# Specify with absolute path
record -file /usr/bin/backup.sh

# Specify with relative path
record -file ./scripts/deploy.sh

# Home directory file
record -file ~/bin/custom-script.sh
```

**Notes**

- Error occurs if file does not exist
- For symbolic links, the hash of the target file is recorded
- Directories cannot be specified (files only)

### 3.2 `-hash-dir <directory>` (Optional)

**Overview**

Specifies the directory to save hash files. If not specified, the current directory is used.

**Syntax**

```bash
record -file <path> -hash-dir <directory>
```

**Parameters**

- `<directory>`: Directory path to save hash files

**Usage Examples**

```bash
# System-wide hash directory
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# User-specific hash directory
record -file script.sh -hash-dir ~/.go-safe-cmd-runner/hashes

# Create directory if it doesn't exist
record -file script.sh -hash-dir ./hash-files
```

**Notes**

- Directory is created automatically if it doesn't exist
- Requires write permissions for the directory

### 3.3 `-force` (Optional)

**Overview**

Forces overwriting of existing hash files without confirmation.

**Syntax**

```bash
record -file <path> -force
```

**Usage Examples**

```bash
# Overwrite existing hash file
record -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes -force

# Update after file modification
record -file updated-script.sh -force
```

**Use Cases**

- **File Updates**: After updating executable files or scripts
- **Batch Operations**: When processing multiple files in scripts
- **Development**: During frequent file modifications

**Notes**

- Without this flag, an error occurs if hash file already exists
- Use carefully to avoid accidental overwriting

## 4. Practical Examples

### 4.1 Initial System Setup

```bash
#!/bin/bash
# System initialization script

# Create hash directory
mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes

# Record system binaries
for binary in /usr/bin/pg_dump /usr/bin/mysqldump /usr/bin/rsync; do
    if [[ -x "$binary" ]]; then
        record -file "$binary" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
        echo "Recorded: $binary"
    fi
done

# Record custom scripts
for script in /usr/local/bin/*.sh; do
    record -file "$script" -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
    echo "Recorded: $script"
done
```

### 4.2 Configuration File Management

```bash
# Record configuration files
record -file /etc/go-safe-cmd-runner/backup.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
record -file /etc/go-safe-cmd-runner/deploy.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
record -file /etc/go-safe-cmd-runner/maintenance.toml -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes
```

### 4.3 Development Workflow

```bash
#!/bin/bash
# Development script update workflow

SCRIPT_FILE="./deploy.sh"
HASH_DIR="~/.go-safe-cmd-runner/hashes"

# Edit script
vim "$SCRIPT_FILE"

# Test script
bash "$SCRIPT_FILE" --dry-run

# Record new hash after confirmation
if [[ $? -eq 0 ]]; then
    record -file "$SCRIPT_FILE" -hash-dir "$HASH_DIR" -force
    echo "Hash updated for $SCRIPT_FILE"
else
    echo "Script test failed, hash not updated"
fi
```

### 4.4 CI/CD Integration

```yaml
# .github/workflows/deploy.yml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v2

    - name: Record deployment script hash
      run: |
        record -file ./scripts/deploy.sh -hash-dir ./hashes -force

    - name: Upload hash files
      run: |
        scp ./hashes/* user@server:/usr/local/etc/go-safe-cmd-runner/hashes/
```

## 5. Troubleshooting

### 5.1 Common Errors

#### File Not Found

**Error Message:**
```
Error: failed to read file: open /path/to/file: no such file or directory
```

**Solution:**
- Check if the file path is correct
- Ensure the file exists
- Verify read permissions

#### Hash File Already Exists

**Error Message:**
```
Error: hash file already exists: /path/to/hashdir/~usr~bin~script.sh
```

**Solution:**
- Use `-force` flag to overwrite
- Remove existing hash file manually
- Choose a different hash directory

#### Permission Denied

**Error Message:**
```
Error: permission denied: cannot write to hash directory
```

**Solution:**
- Check write permissions for hash directory
- Create directory with appropriate permissions
- Use a directory you have write access to

### 5.2 Verification of Recorded Hash

```bash
# Check if hash was recorded correctly
verify -file /usr/bin/backup.sh -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# List hash files
ls -la /usr/local/etc/go-safe-cmd-runner/hashes/

# View hash file content
cat /usr/local/etc/go-safe-cmd-runner/hashes/~usr~bin~backup.sh
```

### 5.3 Debug Options

```bash
# Enable verbose output (if supported)
record -file script.sh -hash-dir ./hashes -verbose

# Check file information before recording
file /usr/bin/backup.sh
sha256sum /usr/bin/backup.sh
```

## 6. Related Documentation

- [runner Command Guide](runner_command.md) - Main execution command
- [verify Command Guide](verify_command.md) - File verification
- [Hash File Naming ADR](../dev/hash-file-naming-adr.md) - Technical details of naming convention
- [Security Architecture](../dev/security-architecture.md) - Overall security design
- [Project README](../../README.md) - Installation and overview
