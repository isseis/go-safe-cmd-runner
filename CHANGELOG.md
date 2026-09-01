# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Breaking Changes

#### `runner`: `--run-id` accepted format limited to `^[A-Za-z0-9_-]{1,64}$`

The `--run-id` flag now only accepts values consisting of uppercase letters (`A-Z`), lowercase letters (`a-z`), digits (`0-9`), underscores (`_`), and hyphens (`-`) only, with a length of 1 to 64 characters. Values that do not match this format cause the process to exit with an error before execution begins.

**Affected scenarios:** CI and operational scripts that pass non-auto-generated values to `--run-id`. Check whether the value you pass fits the above format. Auto-generated ULIDs (26-character Crockford Base32) and the values recommended in the user documentation (`my-custom-run-001`, `gh-<GitHub Actions Run ID>`, `jenkins-<build number>`, `backup-<timestamp>`) all match the accepted format.

#### `verify`: fail-closed on hash directory permission violations

When a permission violation is detected on the hash directory or its ancestor directories, `verify` now exits with code 3 without verifying any target files. Violations confined to a target file's ancestor directories are still recorded as warnings and verification continues as before (exit code unchanged). No bypass flag is provided.

**Assessing impact before upgrading:**

Run the current version of `verify` against your target files and check standard error output for `TOCTOU permission check violation` warnings.

```bash
# Run verify with explicit hash directory and check for TOCTOU warnings
verify -d <hash-directory> <target-files> 2>&1 | grep "TOCTOU permission check violation"
```

(If you do not explicitly pass `-hash-dir`, specify the default `/usr/local/etc/go-safe-cmd-runner/hashes`.)

If this output is empty, the upgrade will have no impact. If there is output, check whether the `path` in the warning points to the hash directory or one of its ancestors. If a hash-directory-side violation exists, fix the hash directory permissions or move the hash directory to a path with appropriate permissions before upgrading. If the violation is on the target file side only, verification continues after upgrade.

#### `verify`: no longer creates the hash directory

`verify` no longer creates the specified hash directory when it does not exist. When it does not exist, `verify` exits with code 3 without verifying a single target file. Create hash records with the `record` command.

When the run ends without verifying a single file, the message on standard error contains an identification token indicating the cause, spelled `verify-error=<token>`. It distinguishes `hash_dir_not_found` (missing), `hash_dir_unreadable` (unreadable), `hash_dir_permission_violation` (permission violation), `path_resolution_failed` (path resolution failure), and `permission_checker_init_failed` (permission checker initialization failure). See the [verify command user guide](docs/user/verify_command.md) for the full list of tokens.

**Affected scenarios:** On hosts with monitoring rules that alert on "exit code 3 = possible tampering", the alert now fires even when the hash directory has merely not been created yet. Either split the monitoring rules by identification token, or create the hash records with `record` beforehand.

#### `record`: rejects a hash directory in a world-writable location

`record` now exits with an error when the hash directory is writable by anyone (world-writable). It refuses even when the sticky bit is set. Two cases are covered.

- **The hash directory already exists and is itself world-writable.** To correct this, run `chmod go-w <hash-directory>` or move the hash directory somewhere only you can write.
- **The hash directory does not exist and its creation site (the deepest existing ancestor of the specified path) is world-writable.** In this case the directory is not created. Once the directory exists, the world-writable refusal above no longer applies and only the ordinary ancestor permission check runs, which does treat the sticky bit as safe. So if that creation site has the sticky bit (`/tmp` and the like), you can still run `record` as before by creating the directory yourself first with `mkdir -m 700 -p <hash-directory>`. If it does not, creating the directory is not enough: the ancestor permission check rejects it all the same, so correct that ancestor with `chmod go-w` or choose a hash directory somewhere only you can write.

In both cases the reason is the same: while others can claim a name there, hash records for files that `record` has not processed could be pre-planted.

**Affected scenarios:** The default hash directory in production (`/usr/local/etc/go-safe-cmd-runner/hashes`) is not affected. Paths under `/tmp` or similar are.

#### Path resolution changes may surface new permission violations

The directory permission check now resolves the specified path to its real path (following symlinks) before performing the check. As a result, for hash directories and target files that were specified through a link, the ancestor directories on the real path, which were not inspected before, are now inspected.

**Assessing impact before upgrading:**

Apply `readlink -m` to the hash directory and the target files, and check the permissions of the ancestor directories of the resolved paths.

```bash
# Resolve the hash directory and the target file to their real paths (-m resolves paths that do not exist yet)
readlink -m "$HASH_DIR"
readlink -m "$TARGET_FILE"

# Walk the ancestors of the real path up to the root and check write permissions for others and the group
# readlink -f fails and returns empty when part of the path does not exist, so use -m
p=$(readlink -m "$HASH_DIR")
while [ -n "$p" ]; do
    ls -ld "$p" 2>/dev/null || echo "(not created yet) $p"
    [ "$p" = / ] && break
    p=$(dirname "$p")
done
```

If the real path is the same as the specified path, there is no impact. If it differs, check whether any of the listed ancestor directories grant write permission to others (`o+w`) or to a group that has members other than the owner (`g+w`), and if so, correct them with `chmod go-w`.

#### `groupmembership`: fail-closed on group-writable writes when enumeration is incomplete on non-CGO builds

The non-CGO build now determines, from the contents of `/etc/nsswitch.conf` and the platform, whether it can enumerate group members and primary-group membership without omission. When the enumeration cannot be determined to be complete, the write-safety check for group-writable files and directories (`CanUserSafelyWriteFile`) — which used to be evaluated more permissively — is now denied with an error (the read-safety check is unaffected).

**Impact:** Any host matching one of the following is affected.

- `GOOS` is other than `linux` (e.g. the distributed macOS binary).
- The `passwd`/`group` lines of `/etc/nsswitch.conf` name a source other than `files`/`systemd` (including `passwd: files sss`, the default on domain-joined hosts).
- `/etc/passwd`/`/etc/group` contains at least one line that cannot be parsed.

On these hosts, checks or writes that go through a group-writable path component — for example, a hash directory or configuration placed under a shared `0775 user:group` directory — will fail after upgrading.

**Steps to assess impact before upgrading:**

```bash
# 1. Check the NSS source configuration (whether anything other than files/systemd is present)
grep -E '^(passwd|group):' /etc/nsswitch.conf

# 2. Check for malformed lines in the user/group databases
getent passwd >/dev/null; echo "passwd: $?"
getent group >/dev/null; echo "group: $?"

# 3. Check whether any path component of the target path is group-writable
p=$(readlink -m <target-path>)
while [ -n "$p" ]; do
    ls -ld "$p"
    [ "$p" = / ] && break
    p=$(dirname "$p")
done
```

If 1 and 2 find no source other than `files`/`systemd` and no malformed lines, and 3 finds no group-writable path component, there is no impact after upgrading. If any of these apply, run `record`/`verify` once and check whether the startup warning `This build cannot enumerate every member of a group on this host` appears.

**Note for macOS:** since the impact list above already always applies when `GOOS` is other than `linux`, steps 1 and 2 do not need to be run on macOS. If you still want to run step 3 to check for group-writable path components, note that `readlink -m` is a GNU extension not available on macOS by default; use `greadlink -m` (from Homebrew's `coreutils`) or a short one-liner such as `python3 -c "import os,sys; print(os.path.realpath(sys.argv[1]))" <target-path>` instead.

**Remediation:** (a) rebuild with `CGO_ENABLED=1`. (b) remove the group-writable bit from the target path (e.g. `chmod 0755`). (c) if a malformed line is a formatting mistake, fix `/etc/passwd`/`/etc/group`.

**Rollback:** reverting to the previous release restores the old behavior. The hash file and configuration formats are unchanged, so no additional work is needed.

#### `groupmembership`: fail-closed on group-writable writes when enumeration is incomplete on CGO_ENABLED=1 builds

This pairs with the non-CGO item above: the same completeness check now also applies to a CGO build (a binary self-built with `CGO_ENABLED=1`). **All officially distributed binaries are built with `CGO_ENABLED=0`, so they are unaffected by this change.** Only hosts running a self-built binary with `CGO_ENABLED=1` are affected.

A CGO build resolves the user and group databases through libc's NSS lookup, but in an environment configured with SSSD's `enumerate = False` (the default) or `ignore_group_members = True`, libc's lookup can return a partial member set without returning an error. Previously the CGO build did not detect this and evaluated group membership more permissively than it actually was; from now on, when the enumeration cannot be determined to be complete, the write-safety check for group-writable files and directories is denied with an error (the read-safety check is unaffected).

**Impact:** A binary built with `CGO_ENABLED=1`, on a host matching one of the following, is affected.

- `GOOS` is other than `linux` (e.g. a macOS self-build).
- The `passwd`/`group` lines of `/etc/nsswitch.conf` name a source other than `files`/`systemd` (including `passwd: files sss`, the default on domain-joined hosts), a line has an unreadable form, a line is missing, or the file could not be read.

On these hosts, checks or writes that go through a group-writable path component not covered by the `isTrustedGroup` exemption will fail after upgrading.

**Steps to assess impact before upgrading:**

```bash
# 1. Check the NSS source configuration (whether anything other than files/systemd is present)
# Only the passwd and group lines matter -- a netgroup line does not affect the check
grep -E '^(passwd|group):' /etc/nsswitch.conf

# 2. Check whether any path component of the target path is group-writable
p=$(readlink -m <target-path>)
while [ -n "$p" ]; do
    ls -ld "$p"
    [ "$p" = / ] && break
    p=$(dirname "$p")
done
```

If your self-build uses `CGO_ENABLED=0`, this item does not apply. If you build with `CGO_ENABLED=1`, and step 1 finds no source other than `files`/`systemd` and step 2 finds no group-writable path component, there is no impact after upgrading. If either applies, run `record`/`verify` once and check whether the startup warning `This build cannot enumerate every member of a group on this host` (`user_database_source=nss`) appears.

**Remediation:** (a) remove the group-writable bit from the target path (`chmod g-w`). (b) configure both the `passwd` and `group` lines with only sources whose enumeration is exhaustive (`files`, `systemd`). **Rebuilding with `CGO_ENABLED=1` is not a remediation** -- it already meets that condition.

**Rollback:** rebuilding with `CGO_ENABLED=0` does not avoid this; you must use a version that does not include this change. The configuration and hash file formats are unchanged, so no additional work is needed.

### Changed

#### Log file name timestamps are now UTC

The timestamp in the log file names that `runner` creates under `-log-dir` (`<hostname>_<timestamp>_<run-id>.json`) now reads in UTC instead of the host local time. The format is unchanged (`YYYYMMDDThhmmssZ`), so old and new names are indistinguishable by shape.

**Affected scenarios:** On hosts in a timezone ahead of UTC, immediately after the migration the existing file names created in local time and the new file names created in UTC coexist, producing a period (lasting as long as the time offset) in which the lexicographic order of the file names does not match the actual chronological order. If you have scripts that process logs sorted by name, note that the order can come out wrong during that period.

#### Newly created hash directories now have 0700 permissions

Newly created hash directories now have `0700` permissions regardless of the path that creates them. `record` already created them with `0700`, but `verify` created them with `0750` (in this release `verify` no longer creates them at all), and the analysis store (`internal/fileanalysis`) also created directories with `0750`.

**Affected scenarios:** The permissions of existing hash directories are not changed. Directories created with `0750` remain as they are, so correct them manually with `chmod 0700 <hash-directory>` if needed. However, in a split-role deployment where the user who runs `record` differs from the user who runs `runner`, tightening them to `0700` would make `runner` unable to read the hashes. See the [record command user guide](docs/user/record_command.md) for how to configure that deployment.

#### The log text for directory permission violations has changed

The term `TOCTOU` has been removed from the text that reports the result of inspecting a directory's permissions, ownership, and path components. This inspection is a static audit that examines each directory once before execution; it is not a TOCTOU defense that compares an observation at the time of check against one at the time of use. The TOCTOU defense itself is provided by file opens that do not follow symlinks and by execution through a file descriptor without re-resolving the path, and that is unchanged.

- The WARN that `runner`, `verify`, and `record` emit per violation: `TOCTOU permission check violation` → `insecure directory permissions`. The `path` and `violation` attribute names and values are unchanged.
- The ERROR that `runner` emits when it aborts execution: `TOCTOU permission check failed: ...` → `directory permission audit failed: ...`.

The rules that determine a violation, the log levels, and the exit codes are unchanged. What is inspected and the results are unchanged as well; only the text has changed.

**Affected scenarios:** Monitoring rules and scripts that search or match logs by the above text are affected. Update them to the new text. Note that the procedure for assessing impact before upgrading, described in "`verify`: fail-closed on hash directory permission violations" in this release, is run on the version **before** the upgrade, so it works correctly with the old text written there.

### Security

#### `groupmembership`: malformed `/etc/group` / `/etc/passwd` lines are now logged

The non-CGO fallback implementation (`internal/groupmembership`) previously skipped malformed lines in `/etc/group` and `/etc/passwd` silently. It now emits a `slog.Warn` with the file name and line number attached, so a corrupted or hand-edited entry that hides group members can be detected in the logs (previously it silently degraded to a "zero members" verdict).

## [1.1.1] - 2026-08-03

### Breaking Changes

#### `record` / `verify`: SUDO_UID must refer to an existing user

When `record` or `verify` runs as root and uses the base UID from `SUDO_UID` for file-read permission checks, that UID is now verified to exist in the user database before use. Invocations with non-existent or unresolvable `SUDO_UID` values now fail immediately instead of silently adopting the unverified value.

**Affected scenarios:**
- Non-cgo builds with LDAP/SSSD-managed users
- Temporary user-database outages
- Stale `SUDO_UID` values from root's cron/systemd environment
- Container images with no `/etc/passwd`

**Workaround:** Remove `SUDO_UID` from the environment (`sudo env -u SUDO_UID record ...`), but note that this changes permission check behavior for group-writable files.

### Changed

#### `sudo runner`: base UID for file-read permission checks no longer reads `SUDO_UID`

`runner` no longer reads the `SUDO_UID` environment variable when determining the
base UID for file-read permission checks. Under `sudo runner`, the base UID
changes from the calling user to `0` (root), which may cause read denials on
group-writable files when root is not a member of the file's group.

The intended operation — regular users launching a setuid `runner` installed
with `install -m 4755` — is unaffected. Direct execution from root's cron is
also unaffected.

The behavior of `record` and `verify` is unchanged: they continue to use the
calling user's UID from `SUDO_UID` when run via sudo, preserving the existing
read-safety check semantics.

#### Permission checks no longer require a passwd entry for the process's own UID

Previously, if the process's real UID had no resolvable passwd entry (an NSS failure with
cgo enabled, or a missing/absent `/etc/passwd` entry with cgo disabled), permission checks
failed to determine the UID and denied file access outright, stopping execution
(fail-closed). The UID used for permission checks is now read directly from the kernel
(`os.Getuid()`) instead of through the passwd database, so this failure mode no longer
occurs and execution continues (fail-open) for this specific failure.

The UID, GID, and permission bits used for the judgment, and the judgment rules
themselves, are unchanged.

Judgments for **group-writable files** still require a passwd entry, because they look up
group membership (`user.LookupId`) to determine who else is in the file's group. For those
files, a missing passwd entry continues to result in access being denied (fail-closed), as
before.

#### `record` / `verify`: startup UID verification and logging

Both commands now resolve the permission-check base UID once at startup. When `SUDO_UID` is adopted and differs from the real UID, a warning is emitted to the structured log (`log/slog`, i.e. standard error) once per process, naming the adopted UID, the real UID, and the user database consulted. Under `sudo` this is the normal case, so expect one such warning per `sudo record` or `sudo verify` run; it records which UID the permission check used, not a fault.

**Troubleshooting:** For detailed migration guidance for environments affected by the `SUDO_UID` existence requirement, see the Breaking Changes section.

## [1.0.0] - 2026-06-27

### Breaking Changes

#### `slack_allowed_host` Required for Slack Webhook Notifications

If `GSCR_SLACK_WEBHOOK_URL_SUCCESS` or `GSCR_SLACK_WEBHOOK_URL_ERROR` environment variables are set,
the `slack_allowed_host` field must now be configured in the `[global]` section.
Startup fails with a configuration error if Slack webhook environment variables are present but `slack_allowed_host` is not set.

**Migration:**
Add `slack_allowed_host` to the `[global]` section of your configuration:
```toml
[global]
slack_allowed_host = "hooks.slack.com"
```

#### File Record Schema Version: v16 → v17

The `detected_syscalls` field in file records has been restructured.
Records created by a previous version are incompatible with the current `verify` and `runner` commands.

**Migration:** Re-record all commands with `record --force`.

#### Risk Assessment Behavioral Changes

- **`risk_level = "unknown"` rejected**: Previously silently treated as 0; now fails with a configuration error at load time.
- **Uncertain binaries denied**: Commands whose binary analysis record is missing or cannot be read are now always denied at runtime, regardless of the configured `risk_level`. Previously they were allowed with a warning.
- **`systemctl status`/`show` reclassified to Medium**: These read-only subcommands are no longer assessed as High. Configurations that set `risk_level = "medium"` for these commands now work correctly.

#### File Path Trust-Zone Risk Elevation (Axis-2)

Write operations targeting trust-critical paths (`/etc`, `/usr`, `/lib`, `/boot`, `/var`, `/sbin`, device nodes, etc.) are now assessed as **High** risk, even when the operation itself (e.g., `cp`, `install`) was previously assessed as Medium.

**Migration:** Review commands that write to system paths. Add `risk_level = "high"` where needed, or restructure commands to write to a safe working directory first.

#### Strict Flag Validation for File Operation Commands

File operation commands (e.g., `ln`, `mkdir`, `chmod`) are now validated against their actual flag specifications. Commands referencing non-existent flags are assessed as **High** risk (fail-closed).

**Migration:** Remove invalid flags from command arguments or add `risk_level = "high"` if the risk is acceptable.

### Added

#### Slack Webhook URL Host Allowlist (`slack_allowed_host`)

New `[global]` field `slack_allowed_host` restricts Slack webhook notifications to a specific hostname, preventing SSRF attacks if environment variables are compromised.

**Configuration:**
```toml
[global]
slack_allowed_host = "hooks.slack.com"
```

- Startup rejects webhook URLs whose host does not match `slack_allowed_host`
- Logging initialization is now two-phase: console/file logging starts first, Slack logging is added after TOML validation succeeds

#### Risk Profile Audit Logging

Command executions now emit structured audit log entries containing the complete risk assessment result:
- `RiskAuditEntry` with correlation fields (ULID, command path, risk level, reason code)
- Reason codes identify the specific rule that determined the allow/deny decision
- Operand zone information for file operation commands (safe-zone, ordinary, trust-critical)
- Available in both normal and dry-run execution modes

#### Dry-Run Allow/Deny Preview

`--dry-run` mode now evaluates risk using the same `StandardEvaluator` as normal mode, accurately predicting whether each command would be allowed or denied at runtime. The previous divergent implementation could produce incorrect predictions.

#### File Path Trust-Zoning for File Operations (Axis-2)

File operation commands are now assessed based on the trust zone of their destination paths, in addition to the command's inherent risk (Axis-1):

- **Safe-zone** (`/tmp`, auto-generated working directories): destructive operations (e.g., `rm`) assessed as **Low**
- **Ordinary paths**: assessed as **Medium**
- **Trust-critical paths** (`/etc`, `/usr`, `/lib`, `/boot`, `/var`, `/sbin`, device nodes): write operations assessed as **High**

This enables legitimate maintenance scripts that clean up temporary files to use `risk_level = "low"` while ensuring writes to system paths remain tightly controlled.

#### Risk Reason Codes and Operand Zones in Audit Output

Risk audit log entries now include:
- `reason_code`: machine-readable code identifying the specific evaluation rule (e.g., `trust_boundary_write`, `privilege_escalation`, `unknown_binary`)
- `reason_family`: grouping of related reason codes
- `operand_zones`: per-operand path resolution and trust-zone classification for file operations

### Changed

#### `detected_syscalls` JSON Structure (Schema v17)

Syscall occurrences in file records are now grouped by syscall number. Each syscall number appears once with an `occurrences` array containing detection details, reducing record size for binaries that trigger the same syscall repeatedly.

**Old format:**
```json
"detected_syscalls": [
  {"name": "mprotect", "address": "0x1000", ...},
  {"name": "mprotect", "address": "0x2000", ...}
]
```

**New format:**
```json
"detected_syscalls": [
  {"name": "mprotect", "occurrences": [{"address": "0x1000"}, {"address": "0x2000"}]}
]
```

#### Risk Assessment Completeness and Accuracy

- All risk factors from command profiles (e.g., full command path `^/usr/bin/rm$` patterns) are now evaluated at runtime, not just basename matching.
- Symlink resolution failures in risk assessment now safely deny execution instead of silently bypassing checks.
- Dry-run mode uses the same risk evaluator as normal mode (single source of truth).

#### Flag Specification Aligned to Real CLI

Flag recognition for file operation commands is now derived from their actual CLI documentation (union of GNU coreutils and uutils). Flags not present in either reference are no longer recognized, preventing false-positive safe assessments.

#### Removal of `verify_standard_paths` Feature

The `verify_standard_paths` configuration field and all related code have been
completely removed. Hash verification now always runs for all commands,
regardless of their directory.

**Removed items:**
- `GlobalSpec.VerifyStandardPaths` field (TOML: `verify_standard_paths`)
- `DefaultVerifyStandardPaths` constant
- `DetermineVerifyStandardPaths()` function
- `RuntimeGlobal.SkipStandardPaths()` method
- `RuntimeCommand.SkipBinaryAnalysis` field
- `AnalysisOptions.VerifyStandardPaths` field
- `DryRunOptions.VerifyStandardPaths` field
- `PathResolver.ShouldSkipVerification()` method
- `shouldPerformHashValidation()` function
- `isStandardDirectory()` function and `StandardDirectories` variable
- `IsNetworkOperation()` `skipBinaryAnalysis` parameter
- `FileVerificationSummary.SkippedFiles` field
- `skipped_files` field from dry-run JSON output

**Migration:**
- Remove `verify_standard_paths = ...` from all TOML configuration files.
  The field is no longer recognized; configs containing it will fail to load
  with an "unknown field" error.
- Update any code that references the removed types or functions listed above.

#### Shebang Interpreter Tracking

Added shebang interpreter tracking to the record/verify pipeline, enabling integrity verification of script interpreters in addition to the scripts themselves.

**Features:**
- New `shebang` package parses shebang lines (`#!`) from script files
- Shebang interpreter path is resolved and recorded alongside the command during `record` phase
- `VerifyCommandShebangInterpreter` verifies the recorded interpreter at `verify` time
- Detects symlink redirection attacks on shebang interpreters (schema v12)
- Supports env-form shebangs (`#!/usr/bin/env python3`) by resolving via `PATH`
- Rejects relative entries in `PATH` to prevent working-directory–dependent resolution
- Integrated into the group executor verification pipeline

**Security Considerations:**
- Uses `safefileio.FileSystem` for symlink-safe file access during parsing
- Symlink redirection of interpreter path is treated as a verification failure
- Non-recursive shebang validation prevents infinite interpreter chains

**New Package:**
- `internal/shebang`: Shebang line parsing and interpreter resolution

#### ELF Dynamic Symbol Analysis for Network Detection

Added ELF binary analysis capability to improve network operation detection for unknown commands.

**Features:**
- Analyzes `.dynsym` section of ELF binaries to detect network-related symbols
- Detects Socket API (socket, connect, bind, listen, etc.)
- Detects DNS resolution functions (getaddrinfo, gethostbyname, etc.)
- Detects HTTP libraries (libcurl functions)
- Detects TLS/SSL libraries (OpenSSL, GnuTLS)
- Gracefully handles non-ELF files, static binaries, and analysis errors

**Security Considerations:**
- Uses safefileio for symlink attack prevention
- TOCTOU protection via kernel-level path validation (openat2) where available
- Fail-safe behavior: analysis errors treated as potential network operations
- File size limit (1GB) to prevent resource exhaustion

**Integration:**
- `IsNetworkOperation()` now performs ELF analysis for unknown commands
- Profile-based detection takes precedence over ELF analysis
- Static binaries (like Go binaries) identified and handled separately

**Performance:**
- Average analysis time: <15 microseconds per binary
- Negligible memory overhead

**New Package:**
- `internal/runner/security/elfanalyzer`: Standalone ELF analysis package

#### Template Inheritance Enhancement

Extended command template functionality to support inheritance and merging of additional fields.

**New Inheritable Fields:**
- **WorkDir**: Working directory path
  - Inheritance model: Override (command-level value takes precedence if specified)
  - `nil`: Field not specified, inheritable from template
  - Empty string: Explicitly set to current directory
  - Non-empty: Use specified absolute path
- **OutputFile**: Output file path for command output capture
  - Inheritance model: Override (command-level value takes precedence if specified)
  - `nil`: Field not specified, inheritable from template
  - Non-empty: Use specified file path
- **EnvImport**: List of environment variables to import as internal variables
  - Inheritance model: Union merge (combines template and command-level lists)
  - Duplicates are automatically removed
  - All variables must be in the `env_allowed` list
- **Vars**: Internal variable definitions
  - Inheritance model: Map merge (command-level variables override template variables with same key)
  - Template variables are inherited first
  - Command-level variables override conflicting keys

**Benefits:**
- Reduce configuration duplication by defining common fields in templates
- Flexible inheritance models for different field types
- Maintain backward compatibility with existing configurations

**Example:**
```toml
[command_templates.build_template]
cmd = "make"
workdir = "/workspace"
env_import = ["cc=CC", "cxx=CXX"]

[command_templates.build_template.vars]
optimization = "O2"

[[groups.commands]]
name = "build-debug"
template = "build_template"
args = ["debug"]
# Inherits: workdir="/workspace", env_import=["cc=CC", "cxx=CXX"], vars={optimization: "O2"}

[[groups.commands]]
name = "build-release"
template = "build_template"
args = ["release"]
workdir = "/opt/build"  # Overrides template workdir
env_import = ["ldflags=LDFLAGS"]  # Merges with template: ["cc=CC", "cxx=CXX", "ldflags=LDFLAGS"]

[groups.commands.vars]
optimization = "O3"  # Overrides template variable
```

#### Variable Scope and Naming Conventions

Added strict naming conventions for user-defined variables to enforce scope separation and prevent configuration errors.

**Features:**
- **Global Variables**: Must start with uppercase letter (A-Z)
  - Defined in `[global.vars]` section
  - Available across all groups and commands
  - Example: `BackupDir`, `MaxRetries`, `Environment`
- **Local Variables**: Must start with lowercase letter (a-z) or underscore (_)
  - Defined in `[groups.vars]` or `[groups.commands.vars]` sections
  - Only available within their scope
  - Example: `backup_date`, `_temp_file`, `retry_count`
- **Reserved Prefix**: Variable names starting with `__` (double underscore) are reserved for system use
- **Validation**: Scope violations are detected at configuration load time with clear error messages

**Benefits:**
- Variable scope is immediately recognizable from the variable name
- Prevents accidental scope misuse
- Improves configuration maintainability
- Enables future optimizations (template variable references can only use global variables)

**Example:**
```toml
# Global variables (uppercase)
[global.vars]
BackupDir = "/data/backups"
MaxRetries = "3"

[[groups]]
name = "backup"

# Local variables (lowercase or underscore)
[groups.vars]
backup_date = "20250101"
_temp_file = "/tmp/backup.tmp"

[[groups.commands]]
name = "database_backup"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases", "--result-file=%{BackupDir}/db-%{backup_date}.sql"]
```

**Migration Guide:**
- Review all variable definitions in your configuration files
- Rename global variables to start with uppercase letters
- Rename local variables to start with lowercase letters or underscore
- The system will report clear error messages if naming violations are detected

**Documentation:**
- Updated: `docs/user/toml_config/08_variable_expansion.ja.md`
- Sample files updated to follow new naming conventions

#### Command Templates

Added reusable command templates with parameter substitution to reduce configuration duplication and improve maintainability.

**Features:**
- Define command templates in `[command_templates.template_name]` sections
- Reference templates using `template` field in command definitions
- Three parameter types:
  - `${param}`: Required parameter (error if missing)
  - `${?param}`: Optional parameter (omitted if empty)
  - `${@param}`: Array parameter (expanded into multiple arguments)
- Escape sequences: `\$` for literal dollar sign
- Variable expansion (`%{var}`) allowed in `params` values
- Security constraint: `%{var}` syntax forbidden in template definitions

**Example:**
```toml
# Define template
[command_templates.restic_backup]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

# Use template
[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
flags = ["-v", "--exclude-caches"]
path = "/data/volumes"
repo = "/backup/repo"
```

**Type Definitions:**
- Added `CommandTemplate` struct in `runnertypes/spec.go`
- Added `CommandTemplates map[string]CommandTemplate` field to `ConfigSpec`
- Added `Template string` and `Params map[string]interface{}` fields to `CommandSpec`

**Documentation:**
- User guide: `docs/user/command_templates.md`
- Sample configuration: `sample/command_template_example.toml`

#### ResolvedPath Type Safety and safefileio API Migration

Strengthened the `common.ResolvedPath` type to carry symlink-resolution semantics, and migrated the `safefileio` public API to require `ResolvedPath` arguments.

**ResolvedPath struct conversion:**
- `ResolvedPath` is now a struct (was a type alias) carrying a `resolveMode` field
- Two constructors enforce distinct semantics at construction time:
  - `NewResolvedPath(path)`: full symlink resolution — all components including the final element are resolved
  - `NewResolvedPathParentOnly(path)`: parent-only resolution — only the parent directory is resolved; the final component may not yet exist (formerly `NewResolvedPathForNew`)
- `IsParentOnly()` method allows callers to query which mode was used
- Write-boundary functions (`SafeWriteFile`, `SafeWriteFileOverwrite`, `SafeAtomicMoveFile`) enforce `IsParentOnly()` at call time, returning an error if a full-resolve path is supplied

**Security fixes included in this work:**
- Fixed TOCTOU race in `atomicMoveFileCore`: `fchmod` is now called via the open file handle instead of a path-based `chmod`, eliminating a race window
- Removed `NewResolvedPathAbsOnly` to preserve the `ResolvedPath` type guarantee
- Fixed security regression in `osFS.AtomicMoveFile` FileSystem bridge

**safefileio public API migration:**
- All public functions in `safefileio` now accept `common.ResolvedPath` instead of plain `string`
- Callers in `fileanalysis`, `filevalidator`, and config loader updated accordingly
- `pathencoding.Encode` had redundant `IsAbs`/`filepath.Clean` checks removed (guaranteed by `ResolvedPath`)

**Rename:**
- `NewResolvedPathForNew` → `NewResolvedPathParentOnly` (intent is clearer)

#### Go Toolchain Upgrade

- Upgraded Go to **1.26.2** (from 1.23.10)
- Upgraded `golangci-lint` to **v2.11.4**
- CI pinned to the Go version declared in `go.mod` via `go-version-file`

#### WorkDir and OutputFile Type Changes

**What Changed:**
The `workdir` field in `CommandTemplate` and `CommandSpec` has been changed from `string` to `*string` (pointer type) to support proper inheritance semantics.

**Behavior:**
- `nil`: Field not specified, can inherit from template
- Empty string pointer (`""`): Explicitly set to use current directory
- Non-nil with value: Use the specified absolute path

**Impact:**
- Existing configurations continue to work without modification
- TOML parser automatically converts string values to pointers
- Code referencing `WorkDir` must handle nil case: `if cmdSpec.WorkDir == nil || *cmdSpec.WorkDir == "" { ... }`

**Benefits:**
- Enables distinction between "not specified" (nil) and "explicitly empty" ("")
- Consistent with other pointer-type fields like `Timeout`, `OutputSizeLimit`, `RiskLevel`
- Required for proper template inheritance support

#### BREAKING CHANGE: Vars Configuration Format

**What Changed:**
The `vars` field configuration format has been changed from an array of strings to a TOML table format.

**Old Format (Deprecated):**
```toml
[global]
vars = [
    "app_dir=/opt/myapp",
    "config_file=%{app_dir}/config.yml"
]

[[groups]]
name = "example"
vars = ["backup_dir=/var/backups"]

[[groups.commands]]
name = "backup"
vars = ["timestamp=20250114"]
```

**New Format (Required):**
```toml
[global.vars]
app_dir = "/opt/myapp"
config_file = "%{app_dir}/config.yml"

[[groups]]
name = "example"

[groups.vars]
backup_dir = "/var/backups"

[[groups.commands]]
name = "backup"

[groups.commands.vars]
timestamp = "20250114"
```

**Migration Guide:**
1. Global level: Change `[global] vars = [...]` to `[global.vars]` table
2. Group level: Change `[[groups]] vars = [...]` to `[groups.vars]` table
3. Command level: Change `[[groups.commands]] vars = [...]` to `[groups.commands.vars]` table
4. Convert each `"key=value"` array entry to `key = "value"` table entry

**Why This Change:**
- Improved TOML compliance and readability
- Better support for complex value types (strings, arrays, nested tables)
- Consistent with standard TOML practices
- Easier validation and error reporting

#### Group-Level Command Allowlist (`cmd_allowed`)

Added the ability to define group-specific allowed commands that are not covered by hardcoded global patterns. This feature enables finer-grained security control.

**Hardcoded Global Patterns** (not configurable from TOML):
```
^/bin/.*
^/usr/bin/.*
^/usr/sbin/.*
^/usr/local/bin/.*
```

**Features:**
- `cmd_allowed` field in `[[groups]]` sections for per-group command allowlists
- Variable expansion support (`%{variable}`) for flexible path configuration
- OR condition evaluation: commands pass if they match EITHER hardcoded global patterns OR group-level list
- Symlink resolution and path normalization for security
- Absolute path requirement prevents path traversal attacks
- All other security checks (permissions, risk assessment) remain active
- Global patterns are hardcoded for security (cannot be configured from TOML)

**Configuration Example:**
```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "custom_build"
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "/opt/myapp/bin/processor"
]

[[groups.commands]]
name = "run_custom"
cmd = "%{home}/bin/custom_tool"
args = ["--verbose"]
```

**Sample File:** See `sample/group_cmd_allowed.toml` for complete examples.

#### File Verification in Dry-Run Mode

Dry-run mode now performs file verification checks, providing visibility into the integrity status of configuration files, global files, group files, and executables without interrupting execution.

**Features:**
- File verification enabled in dry-run mode with warn-only behavior
- Verification results included in dry-run output (TEXT and JSON formats)
- No verification failures cause dry-run to exit (exit code always 0)
- Detailed verification summary showing:
  - Total files verified
  - Hash directory status
  - Verification failures with severity levels (INFO/WARN/ERROR)
  - Context information for each file (config, global, group, env)
  - Security risk assessment for failures

**Verification Failure Reasons:**
- Hash directory not found (INFO level)
- Hash file not found (WARN level)
- Hash mismatch (ERROR level - potential tampering)
- File read error (ERROR level)
- Permission denied (ERROR level)

**Example Output (TEXT):**
```
=== FILE VERIFICATION ===
Hash Directory: /usr/local/etc/go-safe-cmd-runner/hashes
  Exists: true
  Validated: true
Total Files: 2
  Verified: 0
  Skipped: 0
  Failed: 2
Duration: 3.469ms

Failures:
1. [WARN] /tmp/test-config.toml
   Reason: Hash file not found
   Context: config
   Message: hash file not found
2. [WARN] /bin/echo
   Reason: Hash file not found
   Context: group:test_group
   Message: hash file not found
```

**Example Output (JSON):**
```json
{
  "file_verification": {
    "total_files": 2,
    "verified_files": 0,
    "skipped_files": 0,
    "failed_files": 2,
    "duration": 3469483,
    "hash_dir_status": {
      "path": "/usr/local/etc/go-safe-cmd-runner/hashes",
      "exists": true,
      "validated": true
    },
    "failures": [
      {
        "path": "/tmp/test-config.toml",
        "context": "config",
        "reason": "hash_file_not_found",
        "message": "hash file not found",
        "level": "warn"
      },
      {
        "path": "/bin/echo",
        "context": "group:test_group",
        "reason": "hash_file_not_found",
        "message": "hash file not found",
        "level": "warn"
      }
    ]
  }
}
```

**Side-Effect Guarantees:**
- Dry-run mode remains side-effect free
- Only read-only operations performed (file and hash reading)
- No files written or modified
- No network communication
- Exit code always 0 regardless of verification failures

**Documentation:**
- Verification behavior documented in implementation plan

#### JSON Format Output for Dry-Run Mode

Dry-run mode now supports JSON format output with comprehensive debug information, enabling machine processing and automated analysis of execution plans.

**Features:**
- New `--dry-run-format=json` flag for JSON output (default: text)
- Debug information included in JSON output based on detail level:
  - `summary`: No debug information
  - `detailed`: Basic debug information (environment inheritance, final environment)
  - `full`: Complete debug information with diff analysis
- Environment variable inheritance analysis showing:
  - Global and group-level configuration
  - Inheritance mode (inherit/explicit/reject)
  - Inherited variables list
  - Removed allowlist variables
  - Unavailable env_import variables
- Final environment variables with source tracking
- Logs output to stderr in JSON mode to keep stdout clean for piping

**JSON Schema:**
- `ResourceAnalysis` objects with `debug_info` field
- `InheritanceAnalysis` for environment variable inheritance details
- `FinalEnvironment` with per-variable source tracking
- `InheritanceMode` JSON serialization (inherit/explicit/reject)

**Example Usage:**
```bash
# JSON output with full debug information
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full

# Pipe to jq for analysis
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | jq '.'

# Extract debug information
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info != null) | .debug_info'
```

**Documentation:**
- See `docs/user/dry_run_json_schema.md` for complete JSON schema reference
- See `docs/user/runner_command.md` for usage examples

#### Final Environment Variable Display in Dry-Run Mode

When using `--dry-run-detail=full`, the final environment variables for each command are now displayed with their origin information.

**Features:**
- Display final environment variables before each command execution in dry-run mode
- Show the origin of each variable (System, Global, Group, Command)
- Long values are truncated to 60 characters for readability
- Sensitive information (passwords, tokens, secrets) is masked by default as `[REDACTED]`

**New Flag:**
- `--show-sensitive`: Explicitly show sensitive environment variable values in plain text (use with caution)
  - Default: sensitive values are masked
  - Security warning: do not use in production or CI/CD environments

**Example Output:**
```
===== Final Process Environment =====

Environment variables (5):
  PATH=/usr/local/bin:/usr/bin:/bin
    (from Global)
  HOME=/home/testuser
    (from System (filtered by allowlist))
  APP_DIR=/opt/myapp
    (from Group[build])
  DB_PASSWORD=[REDACTED]
    (from Global)
  LOG_FILE=/opt/myapp/logs/app.log
    (from Command[run_tests])
```

**Performance:**
- The overhead for displaying the final environment in dry-run mode is negligible (less than 10% in tests), ensuring minimal impact on performance.

#### Timeout Behavior Change

**BREAKING**: `timeout = 0` now means unlimited execution (previously defaulted to 60 seconds)

- **Before**: `timeout = 0` was treated as invalid (not accepted)
- **After**: `timeout = 0` explicitly means unlimited execution time (no timeout)

**Migration Required**: Review all `timeout = 0` settings in existing configuration files.

#### TOML Field Renaming

All TOML configuration field names have been updated to improve clarity and consistency.

**Migration Required**: Existing configuration files must be manually updated.

##### Field Name Mapping

| Level | Old Field Name | New Field Name | Default Value Change |
|-------|----------------|----------------|---------------------|
| Global | `skip_standard_paths` | `verify_standard_paths` | `false` (verify) → `true` (verify) |
| Global | `env` | `env_vars` | - |
| Global | `env_allowlist` | `env_allowed` | - |
| Global | `from_env` | `env_import` | - |
| Global | `max_output_size` | `output_size_limit` | - |
| Group | `env` | `env_vars` | - |
| Group | `env_allowlist` | `env_allowed` | - |
| Group | `from_env` | `env_import` | - |
| Command | `env` | `env_vars` | - |
| Command | `from_env` | `env_import` | - |
| Command | `max_risk_level` | `risk_level` | - |
| Command | `output` | `output_file` | - |

##### Key Changes

1. **Positive Naming**: `skip_standard_paths` → `verify_standard_paths`
   - Old: `skip_standard_paths = false` (default: verify standard paths)
   - New: `verify_standard_paths = true` (default: verify standard paths)
   - **Default behavior unchanged (verification continues), but field name is now clearer**

2. **Environment Variable Prefix Unification**: All environment-related fields now use `env_` prefix
   - `env` → `env_vars`
   - `env_allowlist` → `env_allowed`
   - `from_env` → `env_import`

3. **Natural Word Order**: `max_output_size` → `output_size_limit`

4. **Clarity**: `output` → `output_file`, `max_risk_level` → `risk_level`

#### Working Directory Specification Redesign

**Working directory specification redesign**: Simplified working directory configuration with automatic temporary directory support
- **Removed `Global.WorkDir` field**: Global-level working directory configuration is no longer supported
- **Removed `Group.TempDir` field**: Replaced with automatic temporary directory generation when `workdir` is not specified
- **Renamed `Command.Dir` to `Command.WorkDir`**: Command-level directory specification now uses `workdir` field
- **Default behavior change**: Groups without `workdir` now automatically generate temporary directories instead of using current directory
- **Automatic cleanup**: Temporary directories are automatically deleted after group execution (unless `--keep-temp-dirs` is specified)

- Support for unlimited command execution with `timeout = 0`
- Enhanced timeout hierarchy resolution (command → global → system default)
- Security monitoring for unlimited execution commands
- Long-running process detection and logging
- Comprehensive timeout examples in `sample/timeout_examples.toml`
- Migration guide for timeout changes
- **`__runner_workdir` reserved variable**: New automatic variable that references the runtime working directory for command execution
- **`--keep-temp-dirs` flag**: New command-line flag to preserve temporary directories after execution for debugging purposes
- **Automatic temporary directory generation**: Groups without specified `workdir` now automatically generate temporary directories
- **Dry-run mode support for temporary directories**: Dry-run mode now uses virtual paths for temporary directories
- **verify_files Variable Expansion**: Environment variable expansion support for `verify_files` fields in both global and group configurations
  - Global-level `verify_files` can now use environment variables (e.g., `${HOME}/config.toml`)
  - Group-level `verify_files` can now use environment variables with allowlist inheritance
  - Support for multiple variables in a single path (e.g., `${BASE}/${ENV}/config.toml`)
  - Comprehensive error handling with detailed error messages
  - Security controls through `env_allowlist` validation
  - Circular reference detection for environment variables
  - Sample configuration: `sample/verify_files_expansion.toml`
  - Documentation: Added section 7.11 to variable expansion user guide

- Timeout configuration now uses nullable integers for better control
- Improved timeout resolution logic with clear inheritance hierarchy
- Enhanced error messages for timeout configuration errors
- Updated documentation with breaking change notices and examples
- Configuration loading now automatically expands environment variables in `verify_files` fields
- Verification manager now uses expanded file paths for all verification operations

### Security

- Added security logging for unlimited timeout executions
- Implemented monitoring for long-running processes
- Enhanced resource usage tracking for unlimited execution commands

### Technical Details

- New fields: `GlobalConfig.ExpandedVerifyFiles` and `CommandGroup.ExpandedVerifyFiles`
- New functions: `ExpandGlobalVerifyFiles()` and `ExpandGroupVerifyFiles()` in config package
- New error types: `VerifyFilesExpansionError` with sentinel errors for better error handling
- Exported `ResolveAllowlistConfiguration()` method in environment package for reusability
- Integration with existing `Filter` and `VariableExpander` infrastructure from task 0026

### Migration Guide

#### Timeout Configuration

For detailed migration instructions, see the timeout configuration documentation.

#### TOML Field Renaming

See [Migration Guide](docs/migration/toml_field_renaming.en.md) for detailed instructions.

#### Working Directory Configuration

Existing TOML configuration files must be updated as follows:

1. **Remove `[global]` section `workdir`**:
   ```toml
   # Before (will cause error)
   [global]
   workdir = "/tmp"

   # After
   [global]
   # workdir field removed
   ```

2. **Remove `[[groups]]` section `temp_dir`**:
   ```toml
   # Before (will cause error)
   [[groups]]
   name = "backup"
   temp_dir = true

   # After (automatic temporary directory)
   [[groups]]
   name = "backup"
   # temp_dir field removed - automatic temporary directory will be created
   ```

3. **Change `[[groups.commands]]` `dir` to `workdir`**:
   ```toml
   # Before (will cause error)
   [[groups.commands]]
   name = "backup"
   cmd = "pg_dump"
   dir = "/var/backups"

   # After
   [[groups.commands]]
   name = "backup"
   cmd = "pg_dump"
   workdir = "/var/backups"
   ```

4. **Use `%{__runner_workdir}` variable** for dynamic path references:
   ```toml
   [[groups]]
   name = "backup"
   # No workdir specified - automatic temporary directory

   [[groups.commands]]
   name = "dump"
   cmd = "pg_dump"
   args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
   ```

## [Previous Releases]

(Previous release notes will be added here when available)
