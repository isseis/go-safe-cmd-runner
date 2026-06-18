# Risk Assessment Guide

To correctly set `risk_level`, you need to understand how the runner calculates the risk of a command before execution. This document explains the risk calculation mechanism and how to verify the basis for your configuration.

## 1. How Risk Assessment Works

`risk_level` declares the **maximum** risk level permitted for a command. The runner automatically calculates the actual risk before execution and rejects the command if the calculated value exceeds `risk_level`.

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef decision fill:#fffde6,stroke:#b8860b,stroke-width:1px,color:#5a4000;
    classDef ok fill:#e6ffe6,stroke:#2e8b57,stroke-width:2px,color:#006400;
    classDef ng fill:#ffe6e6,stroke:#c00000,stroke-width:2px,color:#600000;

    BIN[("Executable")] --> REC["record command<br>(initial setup / update)"]
    REC --> STORE[("Analysis record<br>(hash DB)")]

    CFG[("TOML config<br>risk_level")] --> RUN
    STORE --> RUN["runner execution"]
    RUN --> EVAL1["① Command name & args evaluation<br>Calculated on every run"]
    RUN --> EVAL2["② Command profile factors<br>(privilege / network / data exfiltration / system modification)"]
    RUN --> EVAL3["③ Binary analysis result lookup<br>Reuses static analysis from record"]

    EVAL1 --> MAX["Maximum across all factors"]
    EVAL2 --> MAX
    EVAL3 --> MAX

    MAX --> CMP{"Calculated risk ≤ risk_level?"}
    CMP -->|"Yes"| OK["Execute"]
    CMP -->|"No"| NG["Reject"]

    class BIN,STORE,CFG data;
    class REC,RUN,EVAL1,EVAL2,EVAL3,MAX process;
    class CMP decision;
    class OK ok;
    class NG ng;
```

**Legend**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    D1[("Data")] --> P1["Process"]
    class D1 data
    class P1 process
```

Risk calculation draws on **several independent factors**: the command name and arguments, the command profile factors (privilege, network, data exfiltration, system modification), and the binary analysis result. The final effective risk is the **maximum value across all of these factors**, so any single high-risk factor — including a command profile factor — raises the result regardless of the others.

## 2. Risk Level Definitions

| Level | Meaning | Configurable |
|-------|---------|-------------|
| `low` | Read-only, no side effects | ✅ Yes (default) |
| `medium` | Network communication, file changes, system changes | ✅ Yes |
| `high` | Destructive operations, system/service changes, dynamic/arbitrary code execution | ✅ Yes |
| `critical` | Use of privilege-escalation commands (assigned automatically) | ❌ Not configurable — always blocked |

> `"critical"` cannot be written in TOML. It is assigned automatically when commands like `sudo`/`su`/`doas` are detected and always results in rejection.

## 3. Risk Calculation Rules

### 3.1 Command Name and Argument Evaluation (assessed on every run)

The runner matches commands by their resolved absolute path and basename (symbolic links are resolved), so both `rm` and `/usr/bin/rm` are recognized.

| Detected condition | Calculated risk |
|--------------------|----------------|
| Privilege-escalation commands: `sudo`/`su`/`doas`, etc. | `critical` |
| Destructive file operations: `rm -rf`, `dd`, `chmod -R 777`, etc. | `high` |
| Filesystem/partition tools: `mkfs`/`mkfs.*`, `fdisk`, etc. | `high` |
| Shells, interpreters, and build/task runners: `bash`/`sh`/`python`/`node`/`ruby`/`perl`/`make`/`cmake`/`gradle`, etc. | `high` |
| `systemctl` change verbs (`start`/`stop`/`restart`/`enable`/`disable`, etc.) | `high` |
| `service` (all actions, because it runs an unverified init script) | `high` |
| `systemctl` read-only verbs (`status`/`show`/`is-active`, etc.) | `medium` (floor) |
| Other system-modifying commands (`mount`/`crontab`/`chkconfig`, package install/remove such as `apt install`) | `medium` |
| Network commands: `curl`/`wget`/`ssh`/`scp`, etc. | `medium` |
| None of the above | `low` |

> Shells, interpreters, and build/task runners are `high` regardless of arguments, because they can execute arbitrary code (a script, an inline `-c`/`-e` snippet, or a build target).
> An unknown or unidentifiable `systemctl` subcommand is treated as `high` (fail-safe), never silently downgraded.

### 3.2 coreutils Single-Binary Classification

On distributions that ship coreutils as a single multi-call binary in a dedicated coreutils directory (for example the Rust coreutils binary at `/usr/lib/cargo/bin/coreutils` on Ubuntu 26.04+), every applet shares one executable and therefore one hash. For a command resolved to that directory, the risk is classified from the **subcommand** (applet) — including the `coreutils <applet> ...` multicall form — rather than from the shared binary's analysis signals.

| coreutils subcommand class | Calculated risk |
|----------------------------|----------------|
| Known-safe read-only / informational subcommands (`echo`, `cat`, `ls`, `mkdir`, ...) | `low` |
| Destructive subcommands (`rm`, `dd`, `shred`, `truncate`, ...), or any unknown/unidentifiable subcommand (fail-safe) | `high` |

Only subcommands on the curated safe list are `low`; everything else — including an unparseable multicall invocation that might hide a destructive applet — is `high`. There is no `medium` coreutils class. A binary carrying a setuid/setgid bit is also `high`. For such a verified coreutils binary, the binary-analysis dimension (§3.3) is suppressed for the safe subcommands so that, for example, `echo` stays `low` even though the shared multi-call binary links network or `exec` symbols. Hash verification is still required — suppression applies only to the binary-analysis signal, not to identity verification.

(This mechanism is specific to the unified coreutils directory. Other multi-call binaries such as BusyBox are not covered by it; they are evaluated by the general rules in §3.1 and §3.3.)

### 3.3 Binary Analysis Evaluation (static analysis at record time, result reused)

The executable binary is statically analyzed to determine which system calls and APIs it may invoke.

| Detected capability | Calculated risk | Reason |
|--------------------|----------------|--------|
| Socket APIs: `socket`/`connect`/`bind`/`accept`/`send`/`recv`, etc. | `medium` | May communicate over the network or IPC (any socket family) |
| DNS resolution APIs: `getaddrinfo`/`gethostbyname`, etc. | `medium` | May communicate over the network |
| Dynamic library loading: `dlopen`/`dlsym`/`dlvsym` | `high` | Can load and execute arbitrary code at runtime |
| Process creation: `execve`/`execveat` | `high` | Can launch arbitrary commands |
| Dynamic code execution: `mprotect`+`PROT_EXEC`/`pkey_mprotect` | `high` | Enables arbitrary code execution (e.g., JIT) |
| None of the above detected | `low` | |

**Analysis method**: On Linux, the ELF binary's dynamic symbol table (`.dynsym`) and machine instructions are scanned statically. On macOS, the equivalent Mach-O structures are analyzed. Shared libraries that the binary depends on are also analyzed recursively (OS ABI libraries such as libc are excluded).

### 3.4 Fail-Closed Behavior (unverifiable identity and inconsistencies)

The runner is fail-closed: a command whose binary identity cannot be confirmed is **denied** (it is not executed, regardless of `risk_level`), rather than being allowed to run at some risk level. Failures fall into two categories:

- **Deny (blocking)**: the command is rejected. This is reported in normal execution and previewed as a deny in dry-run.
- **Error**: a genuinely unexpected internal failure aborts the run with an error.

| Condition | Behavior |
|-----------|---------|
| Binary analysis / file verification disabled in this configuration | **Deny** (blocking; the binary's identity cannot be confirmed) |
| Binary hash not computed (identity unverified) | **Deny** (blocking) |
| Analysis record does not exist | **Deny** (blocking) |
| Binary hash does not match the record | **Deny** (blocking) |
| Analysis record schema version is outdated | **Deny** (blocking) |
| Analysis result is inconclusive | **Deny** (blocking) |
| Symbolic-link resolution fails (cannot resolve the real target) | **Deny** (blocking) |
| Unexpected record load error | **Error** (run aborts) |

> A blocking deny is independent of `risk_level`: even `risk_level = "high"` does not permit a command whose identity could not be verified. This is intentional — the runner must not execute a binary it cannot confirm.

## 4. How to Verify the Calculated Risk

Use `record --debug-info` to examine the analysis basis for your `risk_level` setting.

```bash
# Record with detailed analysis information
record --debug-info -d /path/to/hashes /usr/bin/mycommand

# Check the actual calculated risk via dry run
runner -config config.toml -dry-run
```

With `--debug-info`, the analysis record includes:

- Detected network API symbols and their origin (main binary or dependency library)
- Detected syscall numbers
- Analysis determination basis (`determination_stats`)

Dry-run also previews the allow/deny decision: it runs the same read-only evaluation as normal execution and reports, for each command, whether it would be allowed or denied (including blocking denies for unverifiable binaries).

## 5. Guidelines for Setting risk_level

### Principles

- **Least privilege**: Set the minimum risk level required for the actual behavior.
- **Explicit configuration**: Do not rely on the default (`low`); document your intent.

### When binary analysis detects network usage

When binary analysis calculates `medium`, you must set `risk_level` to `"medium"` or higher — the runner will reject the command with any lower setting. Use `record --debug-info` to inspect what was detected, then decide:

| Situation | Action |
|-----------|--------|
| Command that genuinely uses the network (wget, curl, etc.) | Set `"medium"` |
| Command that has network APIs but does not use them in practice | Set `"medium"` (required; a lower value causes rejection) |
| Believed to be a false positive | Report to the development team for investigation. Use `"medium"` until the investigation concludes |

### Configuration examples

```toml
# Read-only system query (medium)
[[groups.commands]]
name = "show_status"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp"]
risk_level = "medium"     # systemctl status is read-only but not low
                          # (it can expose unit configuration); "low" would be rejected

# Network communication (medium)
[[groups.commands]]
name = "fetch_config"
cmd = "/usr/bin/curl"
args = ["-o", "/etc/myapp/config.json", "https://config.example.com/config.json"]
risk_level = "medium"     # curl uses network APIs → medium

# Dynamic loading (high)
[[groups.commands]]
name = "run_plugin"
cmd = "/usr/local/bin/plugin-runner"
args = ["--plugin", "myplugin.so"]
risk_level = "high"       # dlopen for dynamic loading → high

# Package installation (medium)
[[groups.commands]]
name = "install_deps"
cmd = "/usr/bin/apt-get"
args = ["install", "-y", "libfoo"]
run_as_user = "root"
risk_level = "medium"     # apt install/remove is system modification → medium
```

## 6. Frequently Asked Questions

### Q: What happens if I omit risk_level?

The default value `"low"` is used. If binary analysis detects network communication, the calculated risk is `"medium"`, which exceeds `"low"`, so execution is rejected. For commands that use network communication, explicitly set `"medium"`.

### Q: Can I set risk_level to "critical"?

No. `"critical"` cannot be set in TOML (it causes a startup error). The `critical` level is assigned automatically when privilege-escalation commands such as `sudo`/`su` are detected, and always results in rejection.

### Q: Can I set risk_level to "unknown"?

No. `risk_level = "unknown"` is rejected as a configuration error at startup. Use one of `"low"`, `"medium"`, or `"high"` (or omit the key to default to `"low"`).

### Q: The runner says the analysis record is not found

You may not have recorded the hash with the `record` command. Record the hash of the executable:

```bash
record -d /path/to/hashes /usr/bin/mycommand
```

Re-recording is required after system package updates.

## 7. Threat Model and Limitations

Understanding what the risk assessment does and does not protect against is essential for configuring it correctly.

- **Blocklist approach**: Command-name and argument evaluation (§3.1) is a **blocklist**: it recognizes known dangerous commands and patterns and raises their risk. A command that is not on any list is treated as `low` by that dimension. The blocklist is therefore not exhaustive by itself.
- **Allowlist and hash pinning are the primary control**: The blocklist is a backstop, not the main defense. The runner's primary guarantee comes from the **allowlist of permitted commands plus hash pinning** (the recorded analysis record): only verified binaries whose hash matches the record are executed (§3.4). New or unknown attack vectors are contained by this requirement — an unverifiable binary is denied regardless of `risk_level`.
- **Basename matching has limits**: Detection matches by basename and resolved symbolic links. It uses **exact name matching, not partial (substring) matching** — `lsrm` is not treated as `rm`, and `systemctl-helper` is not treated as `systemctl`. Conversely, a renamed copy of a dangerous binary at a different basename is matched only after symbolic-link resolution and hash verification, not by name alone.
- **`output_file` is out of scope**: The risk assessment evaluates the command being executed. Output redirection targets configured via `output_file` are not part of this risk calculation; protect them through the surrounding configuration and filesystem permissions.
- **Hard links and path substitution**: Because hash pinning binds to file content, a hard link to a verified binary has the same content and hash. Path substitution (replacing the file at a path after verification) is closed by binding execution to the verified file (TOCTOU-safe execution), not to the path name.
- **Relationship to privilege/root controls**: The risk assessment is independent of, and complementary to, the runner's user/group switching and root-handling controls. Running a command as `root` does not by itself change the calculated risk level; privilege escalation is detected separately (the `sudo`/`su`/`doas` tokens → `critical`). When a command name has more than one applicable rule, the **highest** resulting risk wins (the effective risk is the maximum across all factors), so a more specific dangerous classification is never lowered by a more general one.

## 8. Migration Notes

If you are upgrading from an earlier version, several commands are now evaluated at a higher risk than before. Review your existing `risk_level` settings against the following changes and use `--dry-run` to confirm before deploying:

- **AI service commands** (`claude`, `gemini`, etc.): now `high` (previously `medium`), because they always communicate with an external API and may exfiltrate data.
- **`systemctl` change verbs** (`start`/`stop`/`restart`/`enable`/`disable`, ...): now `high`. Read-only verbs (`status`/`show`, ...) are evaluated at a floor above low (see the table in §3.1).
- **`service`**: now `high` for all actions (it runs an unverified init script).
- **Destructive operations by absolute path** (`/usr/bin/rm -rf ...`, etc.): now detected the same as by basename — `high`.
- **Shells, interpreters, and build/task runners** (`bash`/`python`/`node`/`make`, ...): now `high` regardless of arguments (arbitrary code execution).
- **Package script runners** (`npm run`/`npx`/`yarn <script>`/`pnpm run`): `high`.
- **`risk_level = "unknown"`**: now rejected as a configuration error (previously accepted). Use `low`/`medium`/`high`.
- **Disabled binary analysis / file verification**: now a blocking deny (previously allowed to continue). A binary whose identity cannot be confirmed is not executed.
- **Wrapper commands** (`env`/`timeout`/`nice`, ...): the inner command executed through a wrapper is now evaluated as a flat **High** regardless of the inner command's content (even a harmless inner is not executed unless you explicitly set `risk_level = "high"`). When a privilege-escalation token (`sudo`/`su`/`doas`) appears as the inner command (including inside a nested wrapper), it is **Critical** and always denied; a subset of forms (loader-control environment variables `LD_*`/`DYLD_*`, working-directory change `env -C`, an uninterpretable `env -S`, `find`/`xargs` child-process execution, direct dynamic-loader invocation, helper execution such as `rsync -e`/`tar --to-command`, a wrapper whose inner command cannot be extracted, exceeding the nesting-depth limit, and symlink-resolution failure) remains **Blocking** and is denied (these are not relaxed to High). The **inner command is not automatically hash-verified or identity-bound** (it is logged in the audit chain, but that does not pin its identity). To pin the inner command's identity, `record` its path and register it explicitly in `verify_files` (but mind the residual risk below).
  - **Residual risk (TOCTOU)**: an inner command of a wrapper you opt into with `risk_level = "high"` is not fd-bound or identity-bound by the runner at execution time. Registering it in `verify_files` only adds a startup-time hash check (verification as an additional file); it does not pin the actual object the wrapper resolves and execs at run time. Because a wrapper binary (`env`, etc.) resolves the path itself and execs it, the verified file and the object actually executed may differ (e.g. `env mytool`), so there is no protection against a swap between check and exec (TOCTOU). This is the same residual limitation as `find`/`xargs` child-process execution.
- **Package managers**: system-modification examples no longer single out individual tool names; package managers are covered generically (`apt`/`yum`/`dnf`/... install/remove → `medium`).
