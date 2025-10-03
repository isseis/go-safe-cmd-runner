# Chapter 10: Troubleshooting

## 10.1 Common Configuration Errors

### Version Specification Issues
```
Error: unsupported version "2.0"
```
**Solution**: Use version "1.0"
```toml
version = "1.0"  # Correct version
```

### Missing Required Fields
```
Error: group name is required
```
**Solution**: Ensure all required fields are present
```toml
[[groups]]
name = "example_group"  # Required field

[[groups.commands]]
name = "example_command"  # Required field
cmd = "/bin/echo"         # Required field
```

### Invalid File Paths
```
Error: command not found: /invalid/path
```
**Solution**: Use absolute paths to existing executables
```toml
[[groups.commands]]
name = "list"
cmd = "/bin/ls"  # Ensure path exists
args = ["-la"]
```

## 10.2 Environment Variable Issues

### Variable Not in Allowlist
```
Error: variable "UNDEFINED_VAR" not in allowlist
```
**Solution**: Add variable to allowlist
```toml
[global]
env_allowlist = ["PATH", "UNDEFINED_VAR"]  # Add required variable
```

### Variable Expansion Errors
```
Error: failed to expand variable "${MISSING_VAR}"
```
**Solution**: Ensure environment variable is set and allowed
```bash
export MISSING_VAR="value"
```

## 10.3 File Verification Issues

### Hash File Missing
```
Error: hash file not found for /usr/bin/command
```
**Solution**: Record hash using record command
```bash
record -file /usr/bin/command -hash-dir /path/to/hashes
```

### Hash Mismatch
```
Error: hash mismatch for /usr/bin/command
```
**Solution**: Re-record hash after file update
```bash
record -file /usr/bin/command -hash-dir /path/to/hashes -force
```

## 10.4 Permission Issues

### Insufficient Privileges
```
Error: permission denied
```
**Solution**: Check file permissions and user privileges
```bash
# Check file permissions
ls -l /path/to/file

# Ensure runner has appropriate setuid permissions
ls -l $(which runner)
```

## 10.5 Timeout Issues

### Command Timeout
```
Error: command timeout after 300 seconds
```
**Solution**: Increase timeout for long-running commands
```toml
[[groups.commands]]
name = "long_process"
cmd = "/opt/tools/long-process"
timeout = 3600  # Increase to 1 hour
```

## 10.6 Debug Techniques

### Configuration Validation
```bash
# Validate configuration syntax
runner -config config.toml -validate

# Check detailed validation output
runner -config config.toml -validate -log-level debug
```

### Dry Run Testing
```bash
# Test without execution
runner -config config.toml -dry-run

# Detailed dry run output
runner -config config.toml -dry-run -dry-run-detail full
```

### Environment Testing
```bash
# Check environment variables
env | grep VARIABLE_NAME

# Test variable expansion
runner -config config.toml -dry-run -dry-run-format json
```

## 10.7 Log Analysis

### Enable Debug Logging
```toml
[global]
log_level = "debug"  # Enable detailed logging
```

### Check System Logs
```bash
# Check syslog for runner messages
journalctl -u go-safe-cmd-runner

# Check specific log files
tail -f /var/log/go-safe-cmd-runner.log
```

## 10.8 Quick Fixes Checklist

- [ ] Configuration file has correct version
- [ ] All required fields are present
- [ ] File paths are absolute and exist
- [ ] Environment variables are in allowlist
- [ ] Hash files are recorded for all executables
- [ ] Timeouts are appropriate for operations
- [ ] Permissions are correctly set
- [ ] Working directories exist and are accessible

For complex issues, use the validation and dry-run features to diagnose problems before production deployment.
