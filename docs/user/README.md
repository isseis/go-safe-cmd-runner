# go-safe-cmd-runner User Guide

Welcome to the user documentation for go-safe-cmd-runner. This guide provides information on how to use command-line tools, write configuration files, and security-related information.

## Quick Navigation

### 🚀 For First-Time Users

If you are using go-safe-cmd-runner for the first time, we recommend reading the documentation in the following order:

1. [Project README](../../README.md) - Overview and security features
2. [runner command guide](#command-line-tools) - Main execution command
3. [TOML configuration file guide](#configuration-files) - How to write configuration files
4. [record command guide](#command-line-tools) - Creating hash files

### 📚 Documentation List

## Command Line Tools

go-safe-cmd-runner provides three command-line tools.

### [runner command](runner_command.md) ⭐ Must Read

Main execution command. Safely executes commands based on TOML configuration files.

**Key Features:**
- Secure batch processing
- Dry-run functionality
- Risk-based security controls
- Detailed logging
- Color output support

**Quick Start:**
```bash
# Basic execution
runner -config config.toml

# Dry run (check execution content)
runner -config config.toml -dry-run

# Validate configuration file
runner -config config.toml -validate
```

**When to use:**
- Want to execute commands
- Want to validate configuration files
- Want to check behavior before execution

[Details here →](runner_command.md)

---

### [record command](record_command.md)

Command to record SHA-256 hash values of files. For administrators.

**Key Features:**
- Create file integrity baselines
- Hash file management
- Batch recording support for multiple files

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

**When to use:**
- During initial setup
- After file updates
- After system package updates

[Details here →](record_command.md)

---

### [verify Command Guide](verify_command.md)

Command to verify file integrity. For debugging and troubleshooting.

**Key Features:**
- Individual file integrity verification
- Detailed investigation of verification errors
- Batch verification support

**Quick Start:**
```bash
# Verify file
verify -file /usr/bin/backup.sh \
    -hash-dir /usr/local/etc/go-safe-cmd-runner/hashes

# Verify multiple files
for file in /usr/local/bin/*.sh; do
    verify -file "$file" -hash-dir /path/to/hashes
done
```

**When to use:**
- Investigating verification error causes
- Pre-check before runner execution
- Regular integrity checks

[Details here →](verify_command.md)

---

## Configuration Files

### [TOML Configuration File Guide](toml_config/README.md) ⭐ Must Read

Detailed explanation of how to write configuration files used by the runner command.

**Chapters:**

1. **[Introduction](toml_config/01_introduction.md)**
   - Overview of TOML configuration files
   - Basic structure

2. **[Configuration File Hierarchy](toml_config/02_hierarchy.md)**
   - Root, global, group, and command levels
   - Inheritance and overrides

3. **[Root Level Settings](toml_config/03_root_level.md)**
   - `version` parameter

4. **[Global Level Settings](toml_config/04_global_level.md)**
   - Timeout, log level, environment variable allowlists, etc.
   - Default settings applied to all groups

5. **[Group Level Settings](toml_config/05_group_level.md)**
   - Group-based command management
   - Resource management and security settings

6. **[Command Level Settings](toml_config/06_command_level.md)**
   - Detailed settings for individual commands
   - Execution user, risk level, output management

7. **[Variable Expansion Features](toml_config/07_variable_expansion.md)**
   - `${VAR}` format variable expansion
   - Dynamic configuration construction

8. **[Practical Configuration Examples](toml_config/08_practical_examples.md)**
   - Real examples for backup, deployment, maintenance, etc.

9. **[Best Practices](toml_config/09_best_practices.md)**
   - Improving security, maintainability, and performance

10. **[Troubleshooting](toml_config/10_troubleshooting.md)**
    - Common errors and solutions

**Quick Start:**
```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "backup"
description = "Database backup"

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
output = "backup.sql"
run_as_user = "postgres"
max_risk_level = "medium"
```

[Details here →](toml_config/README.md)

---

## Security

### [Security Risk Assessment](security-risk-assessment.md)

Explains risk levels and evaluation criteria for commands.

**Contents:**
- Risk level definitions (low, medium, high, critical)
- Risk evaluation per command
- Risk-based control methods

**Risk Levels:**
- **Low**: Basic read operations (ls, cat, grep)
- **Medium**: File modifications, package management (cp, mv, apt)
- **High**: System administration, destructive operations (systemctl, rm -rf)
- **Critical**: Privilege escalation (sudo, su) - Always blocked

[Details here →](security-risk-assessment.md)

---

## Practical Workflows

### Typical Usage Flow

```
1. Create configuration file
   └─ Refer to TOML Configuration File Guide

2. Record hash values
   └─ Record hashes of executables and config files using record command

3. Validate configuration
   └─ runner -config config.toml -validate

4. Check with dry run
   └─ runner -config config.toml -dry-run

5. Production execution
   └─ runner -config config.toml

6. Troubleshooting (if needed)
   └─ Check file integrity with verify command
```

### Initial Setup Example

```bash
# 1. Create configuration file
cat > /etc/go-safe-cmd-runner/backup.toml << 'EOF'
version = "1.0"

[global]
timeout = 3600
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "backup"

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["-U", "postgres", "mydb"]
output = "/var/backups/db.sql"
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

# 5. Check with dry run
runner -config /etc/go-safe-cmd-runner/backup.toml -dry-run

# 6. Production execution
runner -config /etc/go-safe-cmd-runner/backup.toml
```

---

## Frequently Asked Questions (FAQ)

### Q: Which command should I start with?

A: We recommend first reading the [runner command](runner_command.md) and [TOML Configuration File Guide](toml_config/README.md). These two cover most use cases.

### Q: Are there sample configuration files?

A: Yes, there are many samples in the project's `sample/` directory:
- `sample/comprehensive.toml` - Comprehensive coverage of all features
- `sample/variable_expansion_basic.toml` - Variable expansion basics
- `sample/output_capture_basic.toml` - Output capture basics

For details, refer to the [TOML Configuration File Guide](toml_config/README.md).

### Q: What should I do if an error occurs?

A: Please check in the following order:

1. **Configuration validation**: `runner -config config.toml -validate`
2. **File verification**: `verify -file <path> -hash-dir <hash-dir>`
3. **Debug logging**: `runner -config config.toml -log-level debug`
4. **Troubleshooting guides**:
   - [runner troubleshooting](runner_command.md#6-troubleshooting)
   - [TOML configuration troubleshooting](toml_config/10_troubleshooting.md)

### Q: Can it be used in CI/CD environments?

A: Yes, it's optimized for CI/CD environments. For details, refer to:
- [runner command - Usage in CI/CD environments](runner_command.md#55-usage-in-cicd-environments)
- Automatic detection via environment variables (CI, GITHUB_ACTIONS, JENKINS_URL, etc.)
- Non-interactive mode with `-quiet` flag

### Q: What are the security considerations?

A: Main considerations:
- Always record hash values for configuration files and executable binaries
- Add only the minimum necessary environment variables to the allowlist
- Set risk levels appropriately
- For details, refer to [Security Risk Assessment](security-risk-assessment.md)

---

## Recommended Learning Path

### 🎯 For Beginners (1-2 hours)

1. [Project README](../../README.md) - Overall overview (15 minutes)
2. [runner command - Overview and Quick Start](runner_command.md#1-overview) - Basic operations (30 minutes)
3. [TOML Configuration - Introduction](toml_config/01_introduction.md) - Configuration basics (15 minutes)
4. [TOML Configuration - Practical Examples](toml_config/08_practical_examples.md) - Learning with samples (30 minutes)

### 🎓 For Intermediate Users (3-4 hours)

In addition to the above:

5. [runner command - All Flags Detailed](runner_command.md#3-command-line-flags-detailed) - Detailed options (1 hour)
6. [TOML Configuration - Global/Group/Command Levels](toml_config/04_global_level.md) - Hierarchical configuration (1 hour)
7. [TOML Configuration - Variable Expansion](toml_config/07_variable_expansion.md) - Advanced features (30 minutes)
8. [record/verify commands](record_command.md) - Hash management (30 minutes)

### 🚀 For Advanced Users (Full Mastery)

In addition to the above:

9. [TOML Configuration - Best Practices](toml_config/09_best_practices.md) - Design patterns
10. [Security Risk Assessment](security-risk-assessment.md) - Security model
11. [Developer Documentation](../dev/) - Architecture and security design
12. [Troubleshooting](toml_config/10_troubleshooting.md) - Problem-solving skills

---

## Other Resources

### Project Information

- [Project README](../../README.md) - Overview, security features, installation methods
- [GitHub Repository](https://github.com/isseis/go-safe-cmd-runner/) - Source code, Issues, PRs
- [LICENSE](../../LICENSE) - License information

### For Developers

- [Developer Documentation](../dev/) - Architecture, security design, development guidelines
- [Task Documentation](../tasks/) - Development task requirements and implementation plans

### Community

- [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues) - Bug reports, feature requests
- [GitHub Discussions](https://github.com/isseis/go-safe-cmd-runner/discussions) - Questions, idea sharing

---

## Contributing to Documentation

We welcome suggestions for improving documentation or pointing out errors. You can contribute in the following ways:

1. **Create Issues**: [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues)
2. **Submit Pull Requests**: Documentation fixes or additions
3. **Feedback**: Report usability issues or unclear explanations

For documentation creation guidelines, refer to [CLAUDE.md](../../CLAUDE.md).

---

**Last Updated**: 2025-10-02
**Version**: 1.0
