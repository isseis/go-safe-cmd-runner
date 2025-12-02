# Configuration Inheritance and Merging Behavior

This document explains in detail the inheritance and merging behavior of configuration items across hierarchy levels in go-safe-cmd-runner.

## Overview

The runner configuration is divided into the following 4 hierarchy levels:

1. **Runtime** - Environment variables at runner invocation time, etc.
2. **Global Section** - The `[global]` section in TOML
3. **Groups Section** - The `[[groups]]` section in TOML
4. **Commands Section** - The `[[groups.commands]]` section in TOML

The inheritance and merging behavior differs depending on the configuration item across these hierarchy levels.

## Configuration Item Inheritance and Merging Behavior Comparison Table

Configuration items are organized into **single-value items** and **multi-value items**. Single-value items can only Override when set at multiple levels, while multi-value items have choices such as Union or Override.

### Single-Value Items

Single-value items always exhibit Override behavior when set at multiple levels.

| Configuration Item | Global | Group | Command | Inheritance/Merging Behavior | Notes |
|-------------------|--------|-------|---------|------------------------------|-------|
| **timeout** | ✓ | - | ✓ | **Override**: Uses Command.Timeout if > 0, otherwise uses Global.Timeout | Can be overridden at Command level<br>Implementation: [runner.go:582-586](../../internal/runner/runner.go#L582-L586) |
| **workdir** | ✓ | ✓ | ✓ | **Override**: Global.WorkDir is set only when Command.Dir is empty string | Group.WorkDir is for temp_dir purpose<br>Command.Dir is used at execution time<br>Implementation: [runner.go:526-528](../../internal/runner/runner.go#L526-L528) |
| **max_output_size** | ✓ | - | - | **Global only**: Only Global.MaxOutputSize can be defined | Not supported at Command or Group level |
| **skip_standard_paths** | ✓ | - | - | **Global only**: Only Global.SkipStandardPaths can be defined | Not supported at Command or Group level |
| **risk_level** | - | - | ✓ | **Command only**: Only Command.RiskLevel can be defined | Not supported at Global or Group level |
| **run_as_user** | - | - | ✓ | **Command only**: Only Command.RunAsUser can be defined | Not supported at Global or Group level |
| **run_as_group** | - | - | ✓ | **Command only**: Only Command.RunAsGroup can be defined | Not supported at Global or Group level |
| **output** | - | - | ✓ | **Command only**: Only Command.Output can be defined | Not supported at Global or Group level |

### Multi-Value Items

Multi-value items have choices between Union or Override. In the current implementation, the behavior differs by item.

| Configuration Item | Global | Group | Command | Inheritance/Merging Behavior | Notes |
|-------------------|--------|-------|---------|------------------------------|-------|
| **env_vars** | - | - | ✓ | **Independent (no cross-level merging)**: Only environment variables defined in Command.Env are used. Independent between multiple Commands | Each Command has its own env_vars<br>Independent behavior, not Union |
| **env_allowlist** | ✓ | ✓ | - | **Inherit/Override/Prohibit**: <br>• Group.EnvAllowlist is `nil` → Inherit (inherits Global)<br>• Group.EnvAllowlist is `[]` → Prohibit (deny all)<br>• Group.EnvAllowlist is `["VAR1", ...]` → Override (uses only Group value) | 3 inheritance modes<br>**Override adopted, not Union**<br>Implementation: [filter.go:141-153](../../internal/runner/environment/filter.go#L141-L153)<br>Type definition: [config.go:121-135](../../internal/runner/runnertypes/config.go#L121-L135) |
| **verify_files** | ✓ | ✓ | - | **Effective Union**: Managed independently at Global and Group, but at runtime both verifications must succeed. Global failure → program exits, Group failure → group skipped | Effectively Union behavior from user perspective<br>Success of both verifications is prerequisite for Group execution<br>Implementation: [main.go:129-133](../../cmd/runner/main.go#L129-L133), [runner.go:406-417](../../internal/runner/runner.go#L406-L417) |

#### Design Principles for Multi-Value Items

**Reasons for not adopting Union for multi-value items**:

1. **env_allowlist**: For security reasons, explicit control is required. With Union, unintended environment variables could be inherited. Override enables strict control at the Group level.
2. **env_vars**: By having each Command possess its own set of environment variables, independence between Commands is ensured.
3. **verify_files**: Since verification targets differ between Global and Group, independent management is appropriate.

## Special Notes

### 1. Handling of Runtime Environment Variables

- OS environment variables at runner invocation time are filtered by `env_allowlist` and made available at Command execution
- `Filter.ResolveGroupEnvironmentVars()` filters system environment variables based on Group-level `env_allowlist`
- Implementation: [filter.go:114-139](../../internal/runner/environment/filter.go#L114-L139)

### 2. Auto Variables

- Automatically generated internal variables such as `__runner_datetime`, `__runner_pid`, `__runner_workdir` take priority over user-defined variables
- These are internal variables, not environment variables (referenced in the format `%{__runner_datetime}`)
- Implementation: [variable/auto.go](../../internal/runner/variable/auto.go)

### 3. env_allowlist Inheritance Mode Details

Three inheritance modes defined in [config.go:120-136](../../internal/runner/runnertypes/config.go#L120-L136):

#### InheritanceModeInherit (Inherit Mode)

- **Condition**: `env_allowlist` field is undefined in Group (not written in TOML)
- **Behavior**: Inherits Global's allowlist
- **Usage Example**:
  ```toml
  [global]
  env_allowlist = ["PATH", "HOME"]

  [[groups]]
  name = "example"
  # env_allowlist not specified → inherits ["PATH", "HOME"] from Global
  ```

#### InheritanceModeExplicit (Explicit Mode)

- **Condition**: `env_allowlist = ["VAR1", "VAR2"]` specified with values in Group
- **Behavior**: Uses only Group's allowlist (Global is ignored)
- **Usage Example**:
  ```toml
  [global]
  env_allowlist = ["PATH", "HOME"]

  [[groups]]
  name = "example"
  env_allowlist = ["USER", "LANG"]  # Ignores Global, uses only this value
  ```

#### InheritanceModeReject (Reject Mode)

- **Condition**: `env_allowlist = []` specified as empty array in Group
- **Behavior**: Denies all environment variable access
- **Usage Example**:
  ```toml
  [global]
  env_allowlist = ["PATH", "HOME"]

  [[groups]]
  name = "example"
  env_allowlist = []  # Denies all environment variable access
  ```

### 4. verify_files Runtime Verification Behavior

#### 4.1 Execution Flow

verify_files verification is executed in the following order:

1. **Global Verification** ([main.go:137-145](../../cmd/runner/main.go#L137-L145))
   - Verifies all files in Global.VerifyFiles at program start
   - **Verification failure → entire program exits**

2. **Group Verification** ([runner.go:406-417](../../internal/runner/runner.go#L406-L417))
   - Verifies all files in Group.VerifyFiles before each group execution
   - **Verification failure → corresponding group is skipped, other groups continue execution**

#### 4.2 Behavior from User Perspective

When verify_files is set in both Global and Group:

- **Both verifications succeed** → commands in the group are executed
- **Global fails** → entire program exits (Group verification is not even performed)
- **Global succeeds, Group fails** → commands in the corresponding group are not executed

This results in effectively **Union**-like behavior.

#### 4.3 Variable Expansion

When paths in verify_files contain environment variables, the allowlist used for expansion differs by hierarchy level.

#### Global Level

- **Allowlist used**: `Global.EnvAllowlist`
- **Implementation**: [expansion.go:194-216](../../internal/runner/config/expansion.go#L194-L216)
- **Example**:
  ```toml
  [global]
  env_allowlist = ["HOME"]
  verify_files = ["${HOME}/.config/app.conf"]  # HOME can be used
  ```

#### Group Level

- **Allowlist used**: Determined according to Group's `env_allowlist` inheritance rules (`InheritanceMode`)
- **Implementation**: [expansion.go:218-247](../../internal/runner/config/expansion.go#L218-L247)
- **Example**:
  ```toml
  [global]
  env_allowlist = ["HOME", "USER"]

  [[groups]]
  name = "example1"
  # env_allowlist not specified → inherits ["HOME", "USER"] from Global
  verify_files = ["${HOME}/.local/bin/app"]  # HOME, USER can be used

  [[groups]]
  name = "example2"
  env_allowlist = ["USER"]  # Ignores Global
  verify_files = ["${USER}.conf"]  # Only USER can be used

  [[groups]]
  name = "example3"
  env_allowlist = []  # Denies all
  verify_files = ["app.conf"]  # Variable expansion not possible (error)
  ```

### 5. timeout Priority

- Command.Timeout greater than 0 → Command.Timeout is used
- Command.Timeout is 0 or less → Global.Timeout is used
- Implementation: [runner.go:582-586](../../internal/runner/runner.go#L582-L586)

```go
timeout := time.Duration(r.config.Global.Timeout) * time.Second
if cmd.Timeout > 0 {
    timeout = time.Duration(cmd.Timeout) * time.Second
}
```

### 6. workdir Priority

- Command.Dir is not empty string → Command.Dir is used
- Command.Dir is empty string → Global.WorkDir is set
- Implementation: [runner.go:526-528](../../internal/runner/runner.go#L526-L528)

```go
if cmd.Dir == "" {
    cmd.Dir = r.config.Global.WorkDir
}
```

Note: Group.WorkDir is used for the temp_dir feature, but does not directly affect the Command execution directory.

## Summary

### Classification by Value Type

Configuration items are broadly classified into **single-value items** and **multi-value items**:

- **Single-value items** (timeout, workdir, etc.): Always Override behavior when set at multiple levels
- **Multi-value items** (env_vars, env_allowlist, verify_files): Union or Override choices are possible, but current implementation adopts **Override or Independent** for all

### Inheritance and Merging Behavior Patterns

The inheritance and merging behavior of configuration items is not uniform; the following patterns exist by item:

1. **Override Pattern** (timeout, workdir): Lower level overrides upper level
2. **Independent Pattern** (env_vars): Managed independently at each level (no cross-level merging)
3. **Effective Union Pattern** (verify_files): Configuration is managed independently, but at runtime both verifications must succeed
4. **Inherit/Override/Prohibit Pattern** (env_allowlist): Flexible control with 3 inheritance modes
5. **Single-level Pattern** (max_output_size, etc.): Can only be defined at specific levels

### Design Philosophy

**Reasons for not adopting Union for multi-value items**:

1. **Explicitness of Security**: Using Union for env_allowlist could unintentionally inherit upper-level settings, creating security risks. Override enables strict control at the Group level.
2. **Command Independence**: By having each Command possess its own environment variables with env_vars, unintended influence between Commands is prevented.
3. **Clarity of Verification Targets**: By managing verification targets for Global and Group independently with verify_files, each scope is clarified. However, at runtime it functions as **Effective Union**, with success of both verifications being a prerequisite for command execution.

This design achieves a balance between security requirements and flexibility. Particularly for verify_files, by keeping configuration management independent while making runtime security guarantees strict (Union-like behavior), robustness is ensured.
