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

## TOCTOU 攻撃に対する運用要件

### 概要

本システムは、実行前にコマンドバイナリおよびハッシュファイルの整合性を `ValidateDirectoryPermissions` /
`validateCompletePath` を用いて検査します。ただし、検査対象のディレクトリやその親ディレクトリに
第三者が書き込み可能な場合、**ハッシュ検証後・コマンド実行前**の間でファイルを差し替えられる
TOCTOU (Time-of-Check to Time-of-Use) 攻撃が成立する可能性があります。

以下の運用要件を満たすことで、このリスクを最小化してください。

### 対象ディレクトリのパーミッション要件

`verify_files`・`commands` で指定するバイナリ、および `--hash-dir` で指定するハッシュディレクトリは、
下記のパーミッション条件を満たすディレクトリ配下に配置してください。条件は対象ディレクトリ自身だけでなく、
**ルートまでのすべての親ディレクトリ**に適用されます。

| 条件 | 詳細 |
|------|------|
| **other 書込不可** | `other` パーミッションに書込ビット (`o+w`) がないこと。ただし sticky bit が設定されているディレクトリ (`/tmp` 等) は例外 |
| **group 書込制約** | `group` パーミッションに書込ビット (`g+w`) がある場合、ディレクトリの所有者が root であるか、実行ユーザが当該グループの唯一のメンバであること |
| **owner 書込制約** | `owner` パーミッションに書込ビット (`u+w`) がある場合、ディレクトリの所有者が root または実行ユーザ自身であること |
| **シンボリックリンク禁止** | 対象ディレクトリへのパスの途中にシンボリックリンクが含まれないこと |

### ハッシュディレクトリの要件

`--hash-dir` で指定するディレクトリ自体、およびそのルートまでのすべての親ディレクトリも
上記のパーミッション要件を満たす必要があります。

デフォルトのハッシュディレクトリは `/usr/local/etc/go-safe-cmd-runner/hashes` です。
このパスの親ディレクトリ (`/usr/local/etc/go-safe-cmd-runner`, `/usr/local/etc`,
`/usr/local`, `/usr`, `/`) もすべて上記要件を満たすよう管理してください。

### 自動検査について

`runner` コマンドは起動時に上記条件を自動的に検査します。

- **違反が検出された場合**: `runner` はコマンド実行を開始せずエラー終了します。
- **`record` / `verify` コマンドの場合**: 違反が検出されても警告ログを出力して処理を継続します。

存在しないディレクトリはまだ攻撃対象にならないためスキップされます。ディレクトリ作成後は
パーミッションを適切に設定してください。

### 推奨設定

```bash
# ハッシュディレクトリの推奨パーミッション設定例
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
```

コマンドバイナリは `/usr/local/bin` や `/usr/bin` など、root 所有かつ other 書込なしのディレクトリに配置することを推奨します。

### 関連

- 中長期的な TOCTOU 対策として `fexecve` を使った実行時整合性検証 ([0090_toctou_fexecve](../tasks/0090_toctou_fexecve/00_analysis.md)) を検討中です。

---

**Last Updated**: 2025-12-14
**Next Review**: 2026-01-14 (monthly review recommended)
