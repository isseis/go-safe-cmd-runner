# ADR: Determining Expansion Order for Template Parameters and Variable References in Command Templates

## Status

Adopted (Not yet implemented)

## Context

In the command template feature (Task 0062), we need to determine the expansion order for template parameters (`${...}`) and existing variable references (`%{...}`).

### Background

The template feature requires two types of substitution processing:

1. **Template parameter expansion**: Replace `${param}`, `${?param}`, `${@list}` with parameter values
2. **Variable expansion**: Replace `%{variable}` with group-local variable values

In particular, when enabling variable references within params, the following configuration becomes possible:

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]

[[groups]]
name = "group1"
[groups.vars]
group_root = "/data/group1"

[[groups.commands]]
template = "restic_backup"
params.verbose_flags = ["-q"]
params.backup_path = "%{group_root}/volumes"
```

In this case, the question arises: at what timing should `%{group_root}` in `params.backup_path = "%{group_root}/volumes"` be expanded?

### Expansion Order Alternatives Considered

#### Option 1: Expand params first, then template expansion

```
Step 1: Expand %{...} within params
  params.backup_path = "%{group_root}/volumes" → "/data/group1/volumes"

Step 2: Template expansion
  args = ["${@verbose_flags}", "backup", "${backup_path}"]
       → ["-q", "backup", "/data/group1/volumes"]
```

**Advantages**:
- Intuitive: params are "values being passed", so it feels natural for values to be finalized before passing
- Easier to debug: when param values are output to logs, they are already expanded
- Error location is easy to identify: if a variable is undefined, the error occurs before template expansion, making it clear the problem is in params

**Disadvantages**:
- Implementation complexity: requires special logic to expand only param values (separate from normal `[[groups.commands]]` expansion)
- Inconsistent expansion order: other `[[groups.commands]]` fields are expanded later, but params being expanded first violates consistency
- Two-stage variable expansion: once in params, and if there are more `%{...}` references after template application, a second time
- Performance: variable expansion must be executed multiple times

#### Option 2: Perform template expansion first, then variable expansion

```
Step 1: Template expansion (substitute param values as-is)
  args = ["${@verbose_flags}", "backup", "${backup_path}"]
       → ["-q", "backup", "%{group_root}/volumes"]

Step 2: Variable expansion
  args = ["-q", "backup", "%{group_root}/volumes"]
       → ["-q", "backup", "/data/group1/volumes"]
```

**Advantages**:
- **Consistency**: all `%{...}` expansion happens at the same timing (variable expansion phase)
- **Simple implementation**: template expansion is pure string substitution (`${...}` → param values), variable expansion reuses existing logic
- **Performance**: variable expansion only once (on the final form after template expansion)
- **Clear processing flow**: "template expansion → variable expansion" is a simple one-way process
- **Compatibility with existing code**: can reuse existing `%{...}` expansion logic as-is

**Disadvantages**:
- Param values are unclear: during debugging, `params.backup_path = "%{group_root}/volumes"` is displayed, but the actual value is not visible
- Complex error messages: if a variable undefined error occurs, it is difficult to distinguish whether it originated from params or template definition
- Unintuitive: from the concept of "passing values via params", it feels unnatural that values are not finalized at the time of passing

#### Option 3: Hybrid (expand params, keep template definition unexpanded)

Similar to Option 1, but since `%{...}` in template definitions is already prohibited by NF-006, only params need to be considered.

**Advantages**: Same as Option 1

**Disadvantages**: Same as Option 1

#### Option 4: Prohibit %{...} within params as well

If users want to use variables, use normal `[[groups.commands]]` instead of templates.

**Advantages**:
- Simplest implementation
- Lower security risk

**Disadvantages**:
- Lack of flexibility: cannot use different variable values per group (template reusability decreases)
- Reduced usability: hardcoding required
- Violates YAGNI: desire to use variables within template params is a reasonable use case

## Adopted Option

**Option 2: Perform template expansion first, then variable expansion**

### Rationale for Adoption

1. **Lowest implementation cost**
   - Can reuse existing variable expansion logic (`internal/runner/config/expansion.go`)
   - Template expansion can be implemented as pure string substitution
   - No special timing control for variable expansion needed

2. **High consistency**
   - All `%{...}` expansion occurs at the same timing (variable expansion phase)
   - Processing flow is simple: "template expansion → variable expansion"

3. **Simple security verification**
   - Apply existing verification (command path validation, command injection detection, etc.) to the final expanded form
   - No need to verify intermediate state between template and variable expansion

4. **Performance**
   - Variable expansion only once (on final command definition after template expansion)

5. **Alignment with current design**
   - Variable references within template definitions are already prohibited by NF-006
   - Only need to consider `%{...}` usage within params

### Addressing Disadvantages

To address Option 2's disadvantages (debugging difficulty, complex error messages, unintuitive behavior), we will implement the following mitigations:

#### 1. Display expanded and unexpanded values in debug logs (NF-002)

```
DEBUG: Expanding template "restic_backup" with params: {backup_path: "%{group_root}/volumes"}
DEBUG: Template expansion result: args = ["backup", "%{group_root}/volumes"]
DEBUG: Variable expansion result: args = ["backup", "/data/group1/volumes"]
```

Output logs at both template expansion and variable expansion stages to allow tracking the state at each step.

#### 2. Visualization in dry-run mode (NF-007)

```
Group: group1
Command: restic_backup (from template)
  Template parameters:
    verbose_flags = ["-q"]
    backup_path = "%{group_root}/volumes" → "/data/group1/volumes"

  Expanded command:
    cmd: restic
    args: ["-q", "backup", "/data/group1/volumes"]
```

Display both expanded and unexpanded values in `--dry-run` mode so users can verify results in advance.

#### 3. Add context information to error messages (NF-002)

```
Error: variable 'group_root' is not defined in group 'group1',
       referenced by template parameter 'backup_path' in template 'restic_backup' (command #2)

Hint: The parameter 'backup_path' was set to "%{group_root}/volumes"
      Please define 'group_root' in [groups.vars] section or fix the parameter value.
```

Include the following information in variable undefined errors:
- Which params field the reference originates from
- Group name, command number, template name
- The param value (unexpanded expression)
- Hints for correction

#### 4. Provide mental model in documentation

Clarify the concept that params pass an "expression" rather than a "value":
- `params.backup_path = "%{group_root}/volumes"` is passing an "expression"
- Template expansion is "expression substitution"
- Variable expansion is "expression evaluation"

## Implementation Impact

### Implementation order for expansion processing

1. **Template expansion** (`internal/runner/config/template_expansion.go`)
   - Replace `${...}`, `${?...}`, `${@...}` with param values
   - Preserve `%{...}` within param values as-is (treat as string)
   - Generate normal `CommandSpec` as result

2. **Variable expansion** (`internal/runner/config/expansion.go`)
   - Use existing variable expansion logic
   - Process both template-derived and normal commands the same way

3. **Security verification** (`internal/runner/security/`)
   - Execute on final command definition after variable expansion
   - Apply existing verification logic as-is

### Performance considerations

- Template expansion: once only at configuration file load time
- Variable expansion: same as before, once only at configuration file load time
- No runtime overhead

## Security Considerations

### NF-006: Prohibit variable references within template definitions

If template definitions (`command_templates` section) in `cmd`, `args`, `env`, `workdir` contain `%{`, reject as error.

**Rationale**:
1. Templates are reused across multiple groups
2. The same variable name may have different meanings in different contexts
3. A variable reference safe in group A could reference sensitive information in group B

### Allow variable references within params

Allow `%{...}` usage within params (for group-local variable references).

**Rationale**:
1. Each command definition explicitly references variables
2. Which variables are used is clear
3. Context (group) is explicit

### Security verification after expansion (NF-005)

After template expansion and variable expansion, verify the final command definition:
- Command path verification (`cmd_allowed` / `AllowedCommands`)
- Command injection detection
- Path traversal detection
- Environment variable verification

## Related Requirements

- **F-006**: Expansion timing (`docs/tasks/0062_command_templates/01_requirements.md`)
- **NF-002**: Clear error messages and debug information
- **NF-005**: Security verification after expansion
- **NF-006**: Security boundaries for variable references
- **NF-007**: Template expansion visualization in dry-run mode

## References

- `docs/tasks/0062_command_templates/01_requirements.md` - Command template feature requirements document
- `internal/runner/config/expansion.go` - Existing variable expansion logic
- `internal/runner/security/validator.go` - Security verification

## Decision date

2025-12-08

## Decision maker

Design review (requirements definition phase)
