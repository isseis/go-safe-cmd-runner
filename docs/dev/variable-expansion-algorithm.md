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

Variable expansion is implemented in different hierarchies depending on the expansion strategy:

#### Immediate Expansion (via `ExpandString`)

```
ExpandString (Public API)
  ↓
resolveAndExpand (Generate resolver from variable map)
  ↓
parseAndSubstitute (Core parsing and substitution logic)
```

| Function | Role | Visibility | Implementation |
|----------|------|------------|----------------|
| `ExpandString` | Public API (entry point) | public | [expansion.go](../../internal/runner/config/expansion.go) |
| `resolveAndExpand` | Generate resolver from variable map and expand recursively | private | [expansion.go](../../internal/runner/config/expansion.go) |
| `parseAndSubstitute` | Core logic for parsing, escape handling, and variable substitution | private | [expansion.go](../../internal/runner/config/expansion.go) |

#### Lazy Expansion (via `varExpander`)

```
varExpander.expandString (Entry point)
  ↓
varExpander.resolveVariable (Variable resolution and memoization)
  ↓
parseAndSubstitute (Core parsing and substitution logic)
```

| Function | Role | Visibility | Implementation |
|----------|------|------------|----------------|
| `varExpander.expandString` | Entry point (expansion of internal variables) | private | [expansion.go](../../internal/runner/config/expansion.go) |
| `varExpander.resolveVariable` | Variable resolution with lazy evaluation and memoization | private | [expansion.go](../../internal/runner/config/expansion.go) |
| `parseAndSubstitute` | Core logic for parsing, escape handling, and variable substitution (shared by both strategies) | private | [expansion.go](../../internal/runner/config/expansion.go) |

### Two Expansion Strategies

The implementation uses two different expansion strategies depending on the use case:

#### 1. Immediate Expansion (via `ExpandString`)

**Use case**: Expansion using already-expanded variables for environment variables, command arguments, etc.

**Characteristics**:
- Input: `expandedVars map[string]string` (all pre-expanded)
- Processing: Simply resolves variable references
- State: Stateless (no memoization needed)
- Performance: Fast (map lookup only)

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

#### 2. Lazy Expansion (via `varExpander`)

**Use case**: Processing variable definitions (`ProcessVars`)

**Characteristics**:
- Input: `rawVars map[string]interface{}` (unexpanded)
- Processing: Dynamically expands on demand (order-independent)
- State: Stateful (with memoization)
- Performance: Expansion cost on first access, cache hit on subsequent accesses

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

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

Processing flow of the `resolveVariable` method ([expansion.go](../../internal/runner/config/expansion.go)):

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

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

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

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

```go
if depth >= MaxRecursionDepth {
    return "", &ErrMaxRecursionDepthExceededDetail{...}
}
```

### 3. Variable Count Limit

Limits the number of variables per level to prevent DoS attacks.

**Limit**: `MaxVarsPerLevel = 1000`

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

### 4. Size Limits

Limits string length and array size to prevent memory exhaustion.

**Limits**:
- `MaxStringValueLen = 10KB`
- `MaxArrayElements = 1000`

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go), [expansion.go](../../internal/runner/config/expansion.go)

### 5. Variable Name Validation

Validates variable names to prevent injection attacks.

**Implementation**: Uses `security.ValidateVariableName()` ([expansion.go](../../internal/runner/config/expansion.go))

### 6. Type Safety

Prevents mixing of string and array variables.

**Rules**:
- Overriding string variable with array variable → error
- Overriding array variable with string variable → error
- Referencing array variable in string context → error

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go), [expansion.go](../../internal/runner/config/expansion.go)

## Escape Sequences

Supports escaping for literal use of variable expansion syntax.

**Supported escape sequences**:
- `\%` → `%` (percent sign)
- `\\` → `\` (backslash)

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

**Examples**:

```toml
[vars]
literal_percent = "100\\% complete"    # → "100% complete"
escaped_backslash = "path\\\\to\\\\file"  # → "path\to\file"
```

## Performance Optimizations

### 1. Memoization

Once a variable is expanded, it is cached for fast subsequent access.

**Implementation**: [expansion.go](../../internal/runner/config/expansion.go)

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
| `ErrUndefinedVariableDetail` | Reference to undefined variable | Variable resolution | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrCircularReferenceDetail` | Circular reference | Variable resolution | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrMaxRecursionDepthExceededDetail` | Recursion depth exceeded | Parsing | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrInvalidVariableNameDetail` | Invalid variable name | Parsing | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrUnclosedVariableReferenceDetail` | Unclosed `%{` | Parsing | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrInvalidEscapeSequenceDetail` | Invalid escape sequence | Parsing | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrTypeMismatchDetail` | Type mismatch (string⇔array) | Variable validation | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrArrayVariableInStringContextDetail` | Array variable in string context | Variable resolution | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrTooManyVariablesDetail` | Variable count exceeded | Variable validation | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrValueTooLongDetail` | String length exceeded | Variable validation | [expansion.go](../../internal/runner/config/expansion.go) |
| `ErrArrayTooLargeDetail` | Array size exceeded | Variable validation | [expansion.go](../../internal/runner/config/expansion.go) |

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
