# Security Documentation

This directory contains security advisories, vulnerability reports, and mitigation documentation for the go-safe-cmd-runner project.

## Security Advisories

### Active Advisories

#### SSRF-001: External Link Verification Security Risk
- **Status**: ✅ Mitigated (2025-12-14)
- **Severity**: High
- **Component**: `scripts/verification/verify_links.go`
- **Description**: Server-Side Request Forgery vulnerability in external link checking
- **Documents**:
  - [Full Advisory](SSRF-001-external-link-verification.md) - Detailed vulnerability analysis
  - [Implementation Summary](SSRF-001-implementation-summary.md) - Mitigation implementation details
  - [CI Configuration Guide](ci-configuration-example.md) - Safe CI/CD setup

**Quick Summary**: The external link verification feature could be exploited to make arbitrary HTTP requests from CI environments. This has been mitigated with URL allowlisting, private IP blocking, and DNS rebinding protection.

**Action Required**:
- Review CI configuration to ensure `-e` flag is not used on untrusted branches
- See [ci-configuration-example.md](ci-configuration-example.md) for safe CI setup

## Security Scope and Limitations

### Out of Scope: ld.so.cache Tampering

Tampering with `/etc/ld.so.cache` is **outside the threat model** of this system.

**Rationale**: `/etc/ld.so.cache` is owned by root and writable only by root (via `ldconfig`).
An attacker capable of modifying it already has root privileges and can compromise the system
through far more direct means (e.g., replacing binaries directly, loading kernel modules).
Detecting ld.so.cache tampering would therefore provide no meaningful additional security.

**Mitigations in place**:
- `LD_LIBRARY_PATH` is **always cleared** from the child process environment before execution,
  regardless of how it was set (env_allowlist, vars, env_import, etc.).
- Setting `LD_LIBRARY_PATH` via `env_import` is rejected with an error at config load time.
- Dynamic library integrity is verified by **SHA-256 hash** of each recorded library file.

## Security Features

### Implemented Security Controls

The project implements multiple layers of security:

1. **Command Execution Security**
   - Command path validation
   - Environment variable isolation (allowlist-based)
   - `LD_LIBRARY_PATH` always cleared before execution
   - Working directory validation
   - Command injection prevention

2. **File System Security**
   - Symlink attack prevention (safefileio package)
   - Path traversal protection
   - File integrity verification (filevalidator package)
   - Dynamic library integrity via SHA-256 hash verification

3. **Network Security** (SSRF-001 Mitigation)
   - URL allowlisting for external requests
   - Private IP range blocking
   - DNS rebinding protection
   - Redirect validation

4. **Audit and Logging**
   - Security event logging
   - Audit trail for sensitive operations
   - Redaction of sensitive data

## Reporting Security Issues

### For Security Researchers

If you discover a security vulnerability in this project:

1. **DO NOT** open a public GitHub issue
2. **DO NOT** disclose the vulnerability publicly until it has been addressed
3. **DO** contact the maintainers privately with details:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested mitigation (if any)

### For Users

If you suspect a security issue in your deployment:

1. Check this directory for relevant advisories
2. Review the [SSRF-001 advisory](SSRF-001-external-link-verification.md) if using external link checking
3. Follow incident response procedures in the relevant advisory
4. Contact maintainers if you need assistance

## Security Best Practices

### General Guidelines

1. **Keep Updated**: Regularly update to the latest version to receive security fixes
2. **Review Configuration**: Periodically review TOML configuration files for security issues
3. **Limit Privileges**: Run with minimum required privileges
4. **Audit Logs**: Regularly review audit logs for suspicious activity
5. **Network Isolation**: Run in isolated environments when processing untrusted input

### CI/CD Security

When using verification tools in CI/CD:

1. **Never** enable external link checking (`-e`) for pull requests from forks
2. **Always** validate input from untrusted sources
3. **Restrict** network access for CI runners when possible
4. **Monitor** for suspicious HTTP requests in logs
5. **Follow** the [CI Configuration Guide](ci-configuration-example.md)

### Development Security

When contributing to the project:

1. **Test** security features thoroughly
2. **Use** the existing security helpers (safefileio, filevalidator)
3. **Validate** all external input
4. **Follow** the principle of least privilege
5. **Document** security assumptions and constraints

## Security Testing

### Running Security Tests

```bash
# Run all tests including security tests
make test

# Run security-specific tests for link verification
cd scripts/verification
go test -v verify_links.go verify_links_test.go

# Run with race detection
go test -race ./...
```

### Security Test Coverage

The project includes comprehensive security tests:

- ✅ SSRF attack vector tests (50+ cases)
- ✅ Path traversal protection tests
- ✅ Symlink attack prevention tests
- ✅ File integrity validation tests
- ✅ Command injection prevention tests
- ✅ Environment variable isolation tests

## Vulnerability Disclosure Policy

### Timeline

1. **Day 0**: Vulnerability reported privately
2. **Day 1-7**: Initial assessment and confirmation
3. **Day 7-30**: Development and testing of fix
4. **Day 30**: Release of patched version
5. **Day 30-90**: Public disclosure (coordinated with reporter)

### Credit

We acknowledge security researchers who responsibly disclose vulnerabilities:
- Name listed in advisory (if desired)
- Credit in release notes
- Entry in this document

## Security Changelog

### 2025-12-14: SSRF-001 Mitigation
- Implemented URL allowlisting for external link checking
- Added private IP range blocking
- Implemented DNS rebinding protection
- Created comprehensive security tests (50+ test cases)
- Documented safe CI/CD configuration patterns

## Additional Resources

### External References

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE - Common Weakness Enumeration](https://cwe.mitre.org/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)

### Project Security Documentation

- [CLAUDE.md](../../CLAUDE.md) - Project architecture and security patterns
- [Architecture Documentation](../tasks/) - Detailed design documents
- [Configuration Examples](../user/) - Secure configuration examples

## Contact

For security-related questions or concerns, please contact the project maintainers through GitHub.

---

## Operational Requirements for TOCTOU Attacks

### Overview

This system inspects the integrity of command binaries and hash files before execution using
`ValidateDirectoryPermissions` / `validateCompletePath`. However, if a third party can write to
the inspected directories or their parent directories, a TOCTOU (Time-of-Check to Time-of-Use)
attack — where files are swapped **after hash verification but before command execution** — may
be possible.

Minimize this risk by satisfying the following operational requirements.

### Permission Requirements for Target Directories

Binaries specified in `verify_files` and `commands`, and the hash directory specified by
`--hash-dir`, must be placed under directories that satisfy the permission conditions below.
The conditions apply not only to the target directory itself but also to
**all parent directories up to the root**.

| Condition | Details |
|-----------|---------|
| **No other-write** | The `other` permission must not have the write bit (`o+w`) set. Exception: directories with the sticky bit set (e.g., `/tmp`) |
| **Restricted group-write** | If the `group` permission has the write bit (`g+w`) set, the directory owner must be root, or the executing user must be the sole member of that group |
| **Restricted owner-write** | If the `owner` permission has the write bit (`u+w`) set, the directory owner must be root or the executing user themselves |
| **Validated on resolved path** | The path of the target directory is validated based on the real path after symlink resolution; the real path and all parent directories up to the root must satisfy the above requirements |

### Hash Directory Requirements

The directory specified by `--hash-dir` itself, and all parent directories up to the root,
must also satisfy the above permission requirements.

The default hash directory is `/usr/local/etc/go-safe-cmd-runner/hashes`.
The parent directories of this path (`/usr/local/etc/go-safe-cmd-runner`, `/usr/local/etc`,
`/usr/local`, `/usr`, `/`) must also all be managed to satisfy the above requirements.

### Automatic Inspection

The `runner` command automatically inspects the above conditions at startup.

- **If a violation is detected**: `runner` exits with an error without starting command execution.
- **For `record` / `verify` commands**: Processing continues with a warning log even if a violation is detected.

Directories that do not exist are skipped as they are not yet attack targets. After creating a
directory, set its permissions appropriately.

**Directories subject to automatic inspection:**

| Configuration Item | Inspection Scope |
|--------------------|-----------------|
| Each file in `verify_files` | Direct parent directory + all ancestor directories up to the root |
| Each command in `commands` | Direct parent directory + all ancestor directories up to the root |
| `--hash-dir` | The hash directory itself + all ancestor directories up to the root |

In all cases, all ancestors are inspected because write access to an ancestor directory allows
replacing an intermediate directory via rename, enabling a TOCTOU attack on the direct parent
directory.

### Recommended Configuration

```bash
# Example recommended permission settings for the hash directory
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
```

It is recommended to place command binaries in directories owned by root with no other-write
permission, such as `/usr/local/bin` or `/usr/bin`.

### Related

- As a medium- to long-term TOCTOU countermeasure, runtime integrity verification using `fexecve` is under consideration ([0090_toctou_fexecve](../tasks/0090_toctou_fexecve/00_analysis.md)).

---

**Last Updated**: 2025-12-14
**Next Review**: 2026-01-14 (monthly review recommended)
