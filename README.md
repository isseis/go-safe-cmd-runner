NOTE: This project is for evaluating vibe coding, and not for production use.

# go-safe-cmd-runner

A secure command runner designed for safe batch execution of commands with comprehensive security controls and resource management.

## Features

### Core Functionality
- **Group-based Command Execution**: Organize commands into logical groups with priority-based execution order
- **Security Controls**: Built-in validation for commands, file permissions, and environment variables
- **File Integrity Verification**: Hash-based verification to prevent tampering
- **Resource Management**: Automatic temporary directory creation and cleanup
- **Environment Variable Filtering**: Secure environment variable management with allowlists

### Resource Management Features
- **Temporary Directory Creation**: Automatically create temporary directories for command groups
- **Auto-cleanup**: Automatic cleanup of resources after group execution
- **Custom Working Directories**: Set specific working directories per group
- **Resource Isolation**: Each group can have isolated temporary resources

### Configuration Options

#### Group Configuration Fields
- `temp_dir`: Automatically create temporary directory for the group
- `cleanup`: Enable automatic cleanup of temporary resources
- `workdir`: Set custom working directory for the group
- `env_allowlist`: Group-level environment variable allowlist

#### Example Configuration
```toml
[[groups]]
name = "setup"
description = "Initial setup with temporary directory"
temp_dir = true       # Create temporary directory
cleanup = true        # Auto-cleanup after execution
workdir = "/tmp/work" # Custom working directory

[[groups.commands]]
name = "create_files"
cmd = "mkdir"
args = ["-p", "data", "logs"]
```

## Usage

### Basic Execution
```bash
go run cmd/runner/main.go -config sample/config.toml
```

### With Environment Variables
```bash
TEST_SUITE="integration" go run cmd/runner/main.go -config sample/config.toml
```

### Dry Run
```bash
go run cmd/runner/main.go -config sample/config.toml -dry-run
```

## Building

```bash
make build    # Build all binaries
make test     # Run tests
make lint     # Run linter
```

## セキュリティ制限事項

### ファイル改竄検出

**現在の状態**: 未実装

go-safe-cmd-runner は現在、以下のファイル改竄検出機能を提供していません：

1. **設定ファイルの改竄検出**
   - 設定ファイルが悪意を持って変更された場合の検出
   - 不正な設定変更による権限昇格の防止

2. **実行ファイルの改竄検出**
   - runner が呼び出すバイナリファイルの整合性検証
   - 悪意のあるバイナリへの置き換え検出

### 推奨される緩和策

1. 設定ファイルを root 所有に設定し、適切な権限を付与
2. 定期的な設定ファイルのバックアップとレビュー
3. システム監査ツールとの併用
