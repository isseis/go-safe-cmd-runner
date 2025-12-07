# Variable Expansion Algorithm

This document explains the implementation details of the variable expansion algorithm in go-safe-cmd-runner.

## Overview

go-safe-cmd-runner provides functionality to expand variable references in the format `%{variable_name}` within configuration files. The algorithm has the following characteristics:

- **Order-independent expansion**: Expands correctly regardless of variable definition order
- **Lazy evaluation**: Expands only when needed
- **Memoization**: Caches and reuses expansion results
- **Circular reference detection**: Detects circular references to prevent infinite loops
- **Type safety**: Prevents mixing of string and array variables

## Architecture

### Core Function Hierarchy

Variable expansion is implemented in three layers of functions:

```
ExpandString (Public API)
  ↓
resolveAndExpand (Generate resolver from variable map)
  ↓
parseAndSubstitute (Core parsing and substitution logic)
```

| Function | Role | Visibility | Implementation |
|----------|------|------------|----------------|
| `ExpandString` | Public API (entry point) | public | [expansion.go:59-67](../../internal/runner/config/expansion.go#L59-L67) |
| `resolveAndExpand` | Generate resolver from variable map and expand recursively | private | [expansion.go:71-124](../../internal/runner/config/expansion.go#L71-L124) |
| `parseAndSubstitute` | Core logic for parsing, escape handling, and variable substitution | private | [expansion.go:141-241](../../internal/runner/config/expansion.go#L141-L241) |

### Two Expansion Strategies

The implementation uses two different expansion strategies depending on the use case:

#### 1. Immediate Expansion (via `ExpandString`)

**Use case**: Expansion using already-expanded variables for environment variables, command arguments, etc.

**Characteristics**:
- Input: `expandedVars map[string]string` (all pre-expanded)
- Processing: Simply resolves variable references
- State: Stateless (no memoization needed)
- Performance: Fast (map lookup only)

**Implementation**: [expansion.go:59-124](../../internal/runner/config/expansion.go#L59-L124)

#### 2. Lazy Expansion (via `varExpander`)

**Use case**: Processing variable definitions (`ProcessVars`)

**Characteristics**:
- Input: `rawVars map[string]interface{}` (unexpanded)
- Processing: Dynamically expands on demand (order-independent)
- State: Stateful (with memoization)
- Performance: Expansion cost on first access, cache hit on subsequent accesses

**Implementation**: [expansion.go:307-460](../../internal/runner/config/expansion.go#L307-L460)

## Lazy Expansion Details

### varExpander Internal Structure

```go
type varExpander struct {
    expandedVars      map[string]string       // ① Cache (expanded)
    expandedArrayVars map[string][]string     // ② Array variable cache
    rawVars           map[string]interface{}  // ③ Source (unexpanded)
    level             string                  // For error messages
}
```

Lazy evaluation is achieved by using three separate maps:

1. **expandedVars**: Already expanded variables (memoization cache)
2. **expandedArrayVars**: Expanded array variables
3. **rawVars**: Unexpanded variable definitions (raw data from TOML)

### Resolution Algorithm

Processing flow of the `resolveVariable` method ([expansion.go:370-460](../../internal/runner/config/expansion.go#L370-L460)):

```
1. Cache check
   ├─ expandedVars[varName] exists → return immediately (O(1))
   └─ not found → proceed

2. Array variable check
   ├─ expandedArrayVars[varName] exists → error (array variables cannot be used in string context)
   └─ not found → proceed

3. Get unexpanded variable
   ├─ rawVars[varName] exists → proceed
   └─ not found → error (undefined variable)

4. Dynamic expansion
   ├─ visited[varName] = struct{}{} (for circular reference detection)
   ├─ Recursively expand using parseAndSubstitute
   └─ delete(visited, varName) (unmark after expansion)

5. Memoization
   ├─ expandedVars[varName] = expanded (speeds up subsequent accesses)
   └─ return expanded
```

### Example: Order-Independent Expansion

#### TOML Configuration (defined in reverse order)

```toml
[vars]
config_path = "%{base_dir}/config.toml"  # defined before base_dir
log_path = "%{base_dir}/logs"
base_dir = "/opt/myapp"                   # defined later
```

#### Initial State

```go
rawVars = {
    "config_path": "%{base_dir}/config.toml",  // unexpanded
    "log_path": "%{base_dir}/logs",            // unexpanded
    "base_dir": "/opt/myapp"                   // unexpanded
}
expandedVars = {}  // empty
```

#### Expansion Process: Expanding `config_path`

```
1. expandString("config_path", ...)
   ↓
2. resolveVariable("config_path")
   ├─ expandedVars["config_path"] → not found
   ├─ rawVars["config_path"] → "%{base_dir}/config.toml"
   └─ parseAndSubstitute("%{base_dir}/config.toml", ...)
      ↓ Found %{base_dir}
3. resolveVariable("base_dir")  ← recursive call
   ├─ expandedVars["base_dir"] → not found
   ├─ rawVars["base_dir"] → "/opt/myapp"
   └─ parseAndSubstitute("/opt/myapp", ...)
      ↓ No variable references
      ✓ Expansion complete: "/opt/myapp"
   ├─ expandedVars["base_dir"] = "/opt/myapp"  ← memoization
   └─ return "/opt/myapp"
   ↓
4. Return and resume config_path expansion
   "/opt/myapp/config.toml"
   ├─ expandedVars["config_path"] = "/opt/myapp/config.toml"  ← memoization
   └─ return "/opt/myapp/config.toml"
```

#### State After Expansion

```go
expandedVars = {
    "base_dir": "/opt/myapp",                    // memoized
    "config_path": "/opt/myapp/config.toml"      // memoized
}
```

#### Next Expansion: `log_path` (fast)

```
1. resolveVariable("log_path")
   └─ parseAndSubstitute("%{base_dir}/logs", ...)
      ↓ Found %{base_dir}
2. resolveVariable("base_dir")
   ├─ expandedVars["base_dir"] → "/opt/myapp"  ← cache hit!
   └─ return "/opt/myapp"  ← no re-expansion needed
   ↓
3. "/opt/myapp/logs"
```

## Security Features

### 1. Circular Reference Detection

Detects circular references to prevent infinite loops.

**Detection method**: Track currently expanding variables using `visited` map

**Implementation**: [expansion.go:407-436](../../internal/runner/config/expansion.go#L407-L436)

```go
visited[varName] = struct{}{}  // Mark at expansion start
// ... expansion processing ...
delete(visited, varName)       // Unmark after completion
```

**Circular reference example**:

```toml
[vars]
A = "%{B}"
B = "%{C}"
C = "%{A}"  # circular!
```

**Detection process**:

```
resolveVariable("A")
  visited = {"A"}
  ↓ resolveVariable("B")
    visited = {"A", "B"}
    ↓ resolveVariable("C")
      visited = {"A", "B", "C"}
      ↓ resolveVariable("A")
        "A" already in visited → ErrCircularReferenceDetail
```

### 2. Recursion Depth Limit

Limits recursion depth to prevent stack overflow.

**Limit**: `MaxRecursionDepth = 100`

**Implementation**: [expansion.go:150-158](../../internal/runner/config/expansion.go#L150-L158)

```go
if depth >= MaxRecursionDepth {
    return "", &ErrMaxRecursionDepthExceededDetail{...}
}
```

### 3. Variable Count Limit

Limits the number of variables per level to prevent DoS attacks.

**Limit**: `MaxVarsPerLevel = 1000`

**Implementation**: [expansion.go:504-511](../../internal/runner/config/expansion.go#L504-L511)

### 4. Size Limits

Limits string length and array size to prevent memory exhaustion.

**Limits**:
- `MaxStringValueLen = 10KB`
- `MaxArrayElements = 1000`

**Implementation**: [expansion.go:598-605](../../internal/runner/config/expansion.go#L598-L605), [expansion.go:628-635](../../internal/runner/config/expansion.go#L628-L635)

### 5. Variable Name Validation

Validates variable names to prevent injection attacks.

**Implementation**: Uses `security.ValidateVariableName()` ([expansion.go:205-212](../../internal/runner/config/expansion.go#L205-L212))

### 6. Type Safety

Prevents mixing of string and array variables.

**Rules**:
- Overriding string variable with array variable → error
- Overriding array variable with string variable → error
- Referencing array variable in string context → error

**Implementation**: [expansion.go:588-595](../../internal/runner/config/expansion.go#L588-L595), [expansion.go:617-625](../../internal/runner/config/expansion.go#L617-L625)

## Escape Sequences

Supports escaping for literal use of variable expansion syntax.

**Supported escape sequences**:
- `\%` → `%` (percent sign)
- `\\` → `\` (backslash)

**Implementation**: [expansion.go:164-184](../../internal/runner/config/expansion.go#L164-L184)

**Examples**:

```toml
[vars]
literal_percent = "100\\% complete"    # → "100% complete"
escaped_backslash = "path\\\\to\\\\file"  # → "path\to\file"
```

## Performance Optimizations

### 1. Memoization

Once a variable is expanded, it is cached for fast subsequent access.

**Implementation**: [expansion.go:438-439](../../internal/runner/config/expansion.go#L438-L439)

```go
e.expandedVars[varName] = expanded
```

**Effect**: Multiple references to the same variable require expansion only once

### 2. Lazy Evaluation

Variables are not expanded until they are referenced.

**Effect**: Reduces expansion cost for unused variables

### 3. Order-Independent Expansion

No dependency on variable definition order eliminates the need for preprocessing such as topological sort.

**Effect**: Reduces overhead during configuration file loading

## Error Handling

Possible errors during variable expansion and their respective detection timing:

| Error Type | Description | Detection Timing | Implementation |
|-----------|-------------|------------------|----------------|
| `ErrUndefinedVariableDetail` | Reference to undefined variable | Variable resolution | [expansion.go:92-97](../../internal/runner/config/expansion.go#L92-L97) |
| `ErrCircularReferenceDetail` | Circular reference | Variable resolution | [expansion.go:215-221](../../internal/runner/config/expansion.go#L215-L221) |
| `ErrMaxRecursionDepthExceededDetail` | Recursion depth exceeded | Parsing | [expansion.go:151-157](../../internal/runner/config/expansion.go#L151-L157) |
| `ErrInvalidVariableNameDetail` | Invalid variable name | Parsing | [expansion.go:206-211](../../internal/runner/config/expansion.go#L206-L211) |
| `ErrUnclosedVariableReferenceDetail` | Unclosed `%{` | Parsing | [expansion.go:194-198](../../internal/runner/config/expansion.go#L194-L198) |
| `ErrInvalidEscapeSequenceDetail` | Invalid escape sequence | Parsing | [expansion.go:178-182](../../internal/runner/config/expansion.go#L178-L182) |
| `ErrTypeMismatchDetail` | Type mismatch (string⇔array) | Variable validation | [expansion.go:589-594](../../internal/runner/config/expansion.go#L589-L594) |
| `ErrArrayVariableInStringContextDetail` | Array variable in string context | Variable resolution | [expansion.go:383-388](../../internal/runner/config/expansion.go#L383-L388) |
| `ErrTooManyVariablesDetail` | Variable count exceeded | Variable validation | [expansion.go:505-510](../../internal/runner/config/expansion.go#L505-L510) |
| `ErrValueTooLongDetail` | String length exceeded | Variable validation | [expansion.go:599-604](../../internal/runner/config/expansion.go#L599-L604) |
| `ErrArrayTooLargeDetail` | Array size exceeded | Variable validation | [expansion.go:629-634](../../internal/runner/config/expansion.go#L629-L634) |

All errors include detailed context information (level, field, variable name, etc.) for easy debugging.

## Usage Examples

### Basic Variable References

```toml
[vars]
app_name = "myapp"
base_dir = "/opt/%{app_name}"
config_file = "%{base_dir}/config.toml"
```

Expansion result:
```
base_dir = "/opt/myapp"
config_file = "/opt/myapp/config.toml"
```

### Array Variables

```toml
[vars]
base_dir = "/opt/myapp"
paths = [
    "%{base_dir}/bin",
    "%{base_dir}/lib",
    "%{base_dir}/share"
]
```

Expansion result:
```
paths = ["/opt/myapp/bin", "/opt/myapp/lib", "/opt/myapp/share"]
```

### Environment Variable Import

```toml
[global]
env_allowed = ["HOME", "USER"]
env_import = ["home_dir=HOME", "current_user=USER"]

[vars]
user_config = "%{home_dir}/.config/myapp"
```

### Nested References

```toml
[vars]
env = "production"
region = "us-west"
cluster = "%{env}-%{region}"
endpoint = "https://%{cluster}.example.com/api"
```

Expansion result:
```
cluster = "production-us-west"
endpoint = "https://production-us-west.example.com/api"
```

## Summary

The variable expansion algorithm in go-safe-cmd-runner has the following characteristics:

1. **Flexibility**: Order-independent lazy evaluation
2. **Efficiency**: Speed up through memoization
3. **Safety**: Circular reference detection, size limits, type safety
4. **Clarity**: Detailed error messages

These design choices enable safe and efficient variable expansion even for complex configurations.
