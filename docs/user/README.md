# go-safe-cmd-runner User Guide

Welcome to the go-safe-cmd-runner user documentation. This guide provides information on how to use the command-line tools, write configuration files, and understand security features.

## Quick Navigation

### 🚀 For First-Time Users

If you are using go-safe-cmd-runner for the first time, we recommend reading the documentation in the following order:

1. [Project README](../../README.md) - Overview and security features
2. [runner Command Guide](#command-line-tools) - Main execution command
3. [TOML Configuration File Guide](#configuration-files) - How to write configuration files
4. [record Command Guide](#command-line-tools) - Creating hash files

### 📚 Documentation List

## Command-Line Tools

go-safe-cmd-runner provides three command-line tools.

### [runner Command](runner_command.md) ⭐ Must Read

The main execution command. Safely executes commands based on TOML configuration files.

**Key Features:**
- Secure batch processing
- Dry run functionality
- Risk-based security controls
- Detailed logging
- Color output support

**Quick Start:**
```bash
# Basic execution
runner -config config.toml

# Dry run (verify execution plan)
runner -config config.toml -dry-run

# Validate configuration file
runner -config config.toml -validate
```

**Use this when:**
- You want to execute commands
- You want to validate configuration files
- You want to verify behavior before execution

[Learn more →](runner_command.md)

---

### [record Command](record_command.md)

Command to record SHA-256 hash values of files. For administrators.

**Key Features:**
- Create file integrity baseline
- Manage hash files
- Support for batch recording of multiple files

**Quick Start:**
```bash
# Record hash
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# Overwrite existing hash
record -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes \
    -force
```

**Use this when:**
- During initial setup
- After updating files
- After system package updates

[Learn more →](record_command.md)

---

### [verify Command](verify_command.md)

Command to verify file integrity. For debugging and troubleshooting.

**Key Features:**
- Verify individual file integrity
- Detailed investigation of verification errors
- Support for batch verification

**Quick Start:**
```bash
# Verify a file
verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# Verify multiple files
for file in /usr/local/bin/*.sh; do
    verify -file "$file" -hash-dir /path/to/hashes
done
```

**Use this when:**
- Investigating causes of verification errors
- Pre-checking before runner execution
- Regular integrity checks

[Learn more →](verify_command.md)

---

## Configuration Files

### [TOML Configuration File User Guide](toml_config/README.md) ⭐ Must Read

Detailed explanation of how to write configuration files used by the runner command.

**Chapter Structure:**

1. **[Introduction](toml_config/01_introduction.md)**
   - Overview of TOML configuration files
   - Basic structure

2. **[Configuration File Hierarchy](toml_config/02_hierarchy.md)**
   - Root, Global, Group, and Command levels
   - Inheritance and override

3. **[Root Level Settings](toml_config/03_root_level.md)**
   - `version` parameter

4. **[Global Level Settings](toml_config/04_global_level.md)**
   - Timeout, log level, environment variable allowlist, etc.
   - Default settings applied to all groups

5. **[Group Level Settings](toml_config/05_group_level.md)**
   - Command management by group
   - Resource management and security settings

6. **[Command Level Settings](toml_config/06_command_level.md)**
   - Detailed settings for individual commands
   - Execution user, risk level, output management

7. **[Variable Expansion](toml_config/07_variable_expansion.md)**
   - Variable expansion in `%{VAR}` format
   - Dynamic configuration construction

8. **[Practical Examples](toml_config/08_practical_examples.md)**
   - Real-world examples: backup, deployment, maintenance, etc.

9. **[Best Practices](toml_config/09_best_practices.md)**
   - Improving security, maintainability, and performance

10. **[Troubleshooting](toml_config/10_troubleshooting.md)**
    - Common errors and solutions

**Quick Start:**
```toml
version = "1.0"

[global]
timeout = 3600
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "backup"
description = "Database backup"

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
output_file = "backup.sql"
run_as_user = "postgres"
risk_level = "medium"
```

[Learn more →](toml_config/README.md)

---

## Working Directory Configuration

### Default Behavior (Recommended)

If `workdir` is not specified at the group level, a temporary directory is automatically generated.
The temporary directory is automatically deleted after the group execution completes.

```toml
[[groups]]
name = "backup"

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
# Output to /tmp/scr-backup-XXXXXX/dump.sql
```

### Using a Fixed Directory

To use a fixed directory, specify `workdir` at the group level.

```toml
[[groups]]
name = "build"
workdir = "/opt/app"

[[groups.commands]]
name = "compile"
cmd = "make"
```

### Reserved Variable `%{__runner_workdir}`

Use `%{__runner_workdir}` at the command level to reference the working directory at execution time.

### Keeping Temporary Directories

To keep temporary directories for debugging purposes, use the `--keep-temp-dirs` flag.

```bash
$ ./runner --config backup.toml --keep-temp-dirs
```

---

## Security

### [Security Risk Assessment](security-risk-assessment.md)

Explains command risk levels and evaluation criteria.

**Contents:**
- Risk level definitions (low, medium, high, critical)
- Risk assessment by command
- Risk-based control methods

**Risk Levels:**
- **Low**: Basic read operations (ls, cat, grep)
- **Medium**: File modifications, package management (cp, mv, apt)
- **High**: System administration, destructive operations (systemctl, rm -rf)
- **Critical**: Privilege escalation (sudo, su) - Always blocked

[Learn more →](security-risk-assessment.md)

---

## Practical Workflows

### Typical Usage Flow

```
1. Create configuration file
   └─ Refer to TOML Configuration File Guide

2. Record hash values
   └─ Use record command to record hashes of executables and configuration files

3. Validate configuration
   └─ runner -config config.toml -validate

4. Verify with dry run
   └─ runner -config config.toml -dry-run

5. Production execution
   └─ runner -config config.toml

6. Troubleshooting (as needed)
   └─ Use verify command to check file integrity
```

### Initial Setup Example

```bash
# 1. Create configuration file
cat > /etc/go-safe-cmd-runner/backup.toml << 'EOF'
version = "1.0"

[global]
timeout = 3600
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "backup"

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["-U", "postgres", "mydb"]
output_file = "/var/backups/db.sql"
run_as_user = "postgres"
EOF

# 2. Create hash directory
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# 3. Record hashes
sudo record -file /etc/go-safe-cmd-runner/backup.toml \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

sudo record -file /usr/bin/pg_dump \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# 4. Validate configuration
runner -config /etc/go-safe-cmd-runner/backup.toml -validate

# 5. Verify with dry run
runner -config /etc/go-safe-cmd-runner/backup.toml -dry-run

# 6. Production execution
runner -config /etc/go-safe-cmd-runner/backup.toml
```

---

## Frequently Asked Questions (FAQ)

### Q: Which command should I start with?

A: We recommend starting with the [runner Command](runner_command.md) and [TOML Configuration File Guide](toml_config/README.md). These two cover most use cases.

### Q: Are there sample configuration files?

A: Yes, there are many samples in the project's `sample/` directory:
- `sample/comprehensive.toml` - Covers all features
- `sample/variable_expansion_basic.toml` - Basic variable expansion
- `sample/output_capture_basic.toml` - Basic output capture

See the [TOML Configuration File Guide](toml_config/README.md) for details.

### Q: What should I do if I encounter an error?

A: Check in the following order:

1. **Validate configuration**: `runner -config config.toml -validate`
2. **Verify files**: `verify -file <path> -hash-dir <hash-dir>`
3. **Debug logging**: `runner -config config.toml -log-level debug`
4. **Troubleshooting guides**:
   - [runner Troubleshooting](runner_command.md#6-troubleshooting)
   - [TOML Configuration Troubleshooting](toml_config/10_troubleshooting.md)

### Q: Can I use this in CI/CD environments?

A: Yes, it is optimized for CI/CD environments. See:
- [runner Command - Using in CI/CD Environments](runner_command.md#55-using-in-cicd-environments)
- Automatic detection via environment variables (CI, GITHUB_ACTIONS, JENKINS_URL, etc.)
- Non-interactive mode with `-quiet` flag

### Q: What are the security considerations?

A: Key considerations:
- Always record hash values for configuration files and executable binaries
- Only add necessary environment variables to the allowlist
- Set appropriate risk levels
- See [Security Risk Assessment](security-risk-assessment.md) for details

---

## Recommended Learning Path

### 🎯 For Beginners (1-2 hours)

1. [Project README](../../README.md) - Overall overview (15 min)
2. [runner Command - Overview and Quick Start](runner_command.md#1-overview) - Basic operations (30 min)
3. [TOML Configuration - Introduction](toml_config/01_introduction.md) - Configuration basics (15 min)
4. [TOML Configuration - Practical Examples](toml_config/08_practical_examples.md) - Learn from samples (30 min)

### 🎓 For Intermediate Users (3-4 hours)

In addition to the above:

5. [runner Command - All Flags Explained](runner_command.md#3-command-line-flags-explained) - Detailed options (1 hour)
6. [TOML Configuration - Global/Group/Command Levels](toml_config/04_global_level.md) - Hierarchical configuration (1 hour)
7. [TOML Configuration - Variable Expansion](toml_config/07_variable_expansion.md) - Advanced features (30 min)
8. [record/verify Commands](record_command.md) - Hash management (30 min)

### 🚀 For Advanced Users (Full Mastery)

In addition to the above:

9. [TOML Configuration - Best Practices](toml_config/09_best_practices.md) - Design patterns
10. [Security Risk Assessment](security-risk-assessment.md) - Security model
11. [Developer Documentation](../dev/) - Architecture and security design
12. [Troubleshooting](toml_config/10_troubleshooting.md) - Problem-solving skills

---

## Additional Resources

### Project Information

- [Project README](../../README.md) - Overview, security features, installation
- [GitHub Repository](https://github.com/isseis/go-safe-cmd-runner/) - Source code, Issues, PRs
- [LICENSE](../../LICENSE) - License information

### For Developers

- [Developer Documentation](../dev/) - Architecture, security design, development guidelines
- [Task Documentation](../tasks/) - Requirements definition and implementation plans for development tasks

### Community

- [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues) - Bug reports, feature requests, Questions, idea sharing

---

## Contributing to Documentation

We welcome suggestions for improving documentation and reporting errors. You can contribute in the following ways:

1. **Create an Issue**: [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues)
2. **Submit a Pull Request**: Fix or add to documentation
3. **Provide Feedback**: Report usability issues or unclear explanations

See [CLAUDE.md](../../CLAUDE.md) for documentation writing guidelines.

---

**Last Updated**: 2025-10-02
**Version**: 1.0
