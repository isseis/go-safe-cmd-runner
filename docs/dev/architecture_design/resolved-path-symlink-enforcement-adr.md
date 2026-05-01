# ADR: Enforcing ResolvedPath Constructor Constraints at Security Boundaries

## Status

Accepted (implemented)

## Context

### Background

`common.ResolvedPath` is a struct type with two constructors.

| Constructor | Behavior | Primary use |
|---|---|---|
| `NewResolvedPath` | Applies `filepath.EvalSymlinks` to all path components including the leaf | Access to existing files (`filevalidator`, `config/loader`, etc.) |
| `NewResolvedPathParentOnly` | Applies `EvalSymlinks` to the parent directory only; preserves the leaf (filename) as-is | New file creation, overwrite, atomic move destination, etc. |

### Security Precondition

The `safefileio` functions (`SafeWriteFile`, `SafeWriteFileOverwrite`, `SafeAtomicMoveFile`) are designed to **detect and reject** symlinks at the leaf position. This detection relies on `SafeOpenFile` using `openat2(RESOLVE_NO_SYMLINKS)`.

### Problem

These functions accept `common.ResolvedPath`, which can be created by either constructor. A value created with `NewResolvedPath` has its leaf symlink already resolved, so `SafeOpenFile`'s `RESOLVE_NO_SYMLINKS` check passes without detecting the original symlink.

**Concrete scenario:**

```
/tmp/link → /real/file  (attacker-controlled symlink)
```

```go
// Using NewResolvedPath
srcRP, _ := common.NewResolvedPath("/tmp/link")
// srcRP.path == "/real/file"  (leaf symlink already resolved)

SafeAtomicMoveFile(srcRP, dstRP, 0o600)
// SafeOpenFile opens "/real/file" directly
// openat2(RESOLVE_NO_SYMLINKS) succeeds because the path contains no symlinks
// → symlink detection does not trigger
```

```go
// Using NewResolvedPathParentOnly
srcRP, _ := common.NewResolvedPathParentOnly("/tmp/link")
// srcRP.path == "/tmp/link"  (leaf preserved as-is)

SafeAtomicMoveFile(srcRP, dstRP, 0o600)
// SafeOpenFile attempts to open "/tmp/link" with openat2(RESOLVE_NO_SYMLINKS)
// → leaf is a symlink, ErrIsSymlink is returned  ✓
```

### Required Constructor per Function

| Function | Parameter | Required constructor | Reason |
|---|---|---|---|
| `SafeWriteFile` | `filePath` | `NewResolvedPathParentOnly` only | Leaf symlink detection required |
| `SafeWriteFileOverwrite` | `filePath` | `NewResolvedPathParentOnly` only | Same |
| `SafeAtomicMoveFile` | `srcPath` | `NewResolvedPathParentOnly` only | Same |
| `SafeAtomicMoveFile` | `dstPath` | `NewResolvedPathParentOnly` only | Destination file may not yet exist |
| `SafeReadFile` | `filePath` | **Both are valid** | Called from `filevalidator` (`NewResolvedPath`) and `fileanalysis` (`NewResolvedPathParentOnly`) |

`SafeReadFile` is semantically valid with either constructor, so type-level enforcement cannot be applied to it.

---

## Options Considered

### Option A: Comment-only documentation (rejected)

Add a doc comment to `SafeAtomicMoveFile` and related functions stating that arguments must be created with `NewResolvedPathParentOnly`.

**Reason for rejection:** Misuse cannot be detected at compile time or at runtime; only code review provides a defense.

---

### Option B: Introduce a distinct `ParentOnlyResolvedPath` type

```go
type ParentOnlyResolvedPath struct { rp ResolvedPath }
func (p ParentOnlyResolvedPath) String() string             { return p.rp.String() }
func (p ParentOnlyResolvedPath) AsResolvedPath() ResolvedPath { return p.rp }

func NewResolvedPathParentOnly(path string) (ParentOnlyResolvedPath, error) { ... }
```

Change the parameter types of `SafeWriteFile`, `SafeWriteFileOverwrite`, and the `dstPath` of `SafeAtomicMoveFile` to `ParentOnlyResolvedPath`.

**Pros:**
- Misuse is a compile error
- Function signatures self-document intent

**Cons:**
- `SafeReadFile` remains unprotected because both constructors are valid for it (the most frequently called security-boundary function gains no benefit)
- `fileanalysis.Store.Load` requires an explicit `.AsResolvedPath()` conversion when passing to `SafeReadFile`, reducing readability at the call site
- Test helper `mustResolvedPath` changes its return type, requiring `.AsResolvedPath()` additions at 6–8 `SafeReadFile` call sites
- The `srcPath` of `SafeAtomicMoveFile` was previously documented as using `NewResolvedPath`; this contradiction must be resolved before proceeding
- Estimated change size: ~45 lines (including 6–8 conversion call sites)

---

### Option C: Add a `resolveMode` field to `ResolvedPath` (accepted)

```go
type resolveMode int
const (
    resolveModeFull       resolveMode = iota + 1 // set by NewResolvedPath; iota+1 makes the zero value (0) an invalid sentinel not assigned by either constructor
    resolveModeParentOnly                         // set by NewResolvedPathParentOnly
)

type ResolvedPath struct {
    path string
    mode resolveMode
}
```

Each constructor sets `mode`; enforcement is placed at security boundaries via
an exported `ResolvedPath` method, `IsParentOnly() bool`.

```go
// Inside safeAtomicMoveFileWithFS
if !srcPath.IsParentOnly() {
    return fmt.Errorf("%w: srcPath must use NewResolvedPathParentOnly", ErrInvalidFilePath)
}
if !dstPath.IsParentOnly() {
    return fmt.Errorf("%w: dstPath must use NewResolvedPathParentOnly", ErrInvalidFilePath)
}
```

Adding the same assertion to `safeWriteFileCommon` protects `SafeWriteFile` and `SafeWriteFileOverwrite` as well.

**Pros:**
- No changes to existing function signatures → zero impact on existing callers
- No conversion boilerplate
- `SafeWriteFile`, `SafeWriteFileOverwrite`, and `SafeAtomicMoveFile` are all protected uniformly
- The zero value of `mode` (0) is an invalid sentinel not set by either constructor; `IsParentOnly()` returns `false` for it, so `ResolvedPath{}` is rejected by write-family boundary assertions (and also by the empty-path check)
- Estimated change size: ~25 lines (no modifications to existing call sites)

**Cons:**
- Runtime check only; misuse is not detected at compile time
- `ResolvedPath` gains hidden state (same type, different behavior depending on how it was constructed)
- The `mode` field is unexported; tests must verify behavior indirectly

---

## Decision: Option C

### Rationale

**Asymmetric protection coverage:** `SafeReadFile` is validly called with either constructor, so even under Option B it remains outside type-level enforcement. Because the most frequently called security-boundary function gains no benefit, Option B's primary advantage—compile-time protection—becomes limited in scope.

**Conversion boilerplate trade-off:** Option B requires `.AsResolvedPath()` conversions in `fileanalysis.Store.Load` and at 6–8 `SafeReadFile` call sites in tests. The additional friction introduced by splitting the type outweighs the benefit of compile-time enforcement.

**Alignment with YAGNI:** At the time of this decision, the production callers of `SafeWriteFile` and `SafeAtomicMoveFile` are few (two call sites in `fileanalysis`), and both already use `NewResolvedPathParentOnly` correctly. Any misuse would surface immediately as a runtime error.

**Zero-value safety:** Using `resolveModeFull = iota + 1` makes the zero value of `mode` (0) an invalid sentinel that neither constructor assigns. `IsParentOnly()` returns `false` for the zero value, so passing an uninitialized `ResolvedPath{}` to any write-family function is rejected with `ErrInvalidFilePath` by the mode assertion. Additionally, `ResolvedPath{}` has an empty `path`, so the empty-path check (`absPath == ""` → `ErrInvalidFilePath`) may fire first. Either way, the zero value is safely rejected.

### Supersession of Prior Specification

`docs/tasks/0085_safefileio_resolved_path_api/01_requirements.md` (FR-5.1, FR-6.4) previously specified that the `srcPath` of `SafeAtomicMoveFile` should use `NewResolvedPath` because the source file already exists.

That specification was a security mistake. Pre-resolving the leaf symlink with `NewResolvedPath` causes `SafeOpenFile`'s `openat2(RESOLVE_NO_SYMLINKS)` check to receive a path with no symlink, silently bypassing detection (see the "Problem" section of this ADR). Whether the file exists is irrelevant; the leaf-symlink check must be preserved for `srcPath` as well.

**This ADR supersedes that prior specification: `NewResolvedPathParentOnly` is required for `srcPath` of `SafeAtomicMoveFile`.** The current implementation (test helper `mustResolvedPath` using `NewResolvedPathParentOnly`) already reflects the correct behavior.

### Implementation Scope

1. `internal/common/filesystem.go`
   - Add `resolveMode` type
   - Add `mode` field to `ResolvedPath`
   - `NewResolvedPath`: set `resolveModeFull`
   - `NewResolvedPathParentOnly`: set `resolveModeParentOnly`
    - Add `IsParentOnly() bool` method (to avoid external packages directly accessing `mode`)

2. `internal/safefileio/safe_file.go`
    - `safeAtomicMoveFileWithFS`: add assertions on `srcPath.IsParentOnly()` and `dstPath.IsParentOnly()`
    - `safeWriteFileCommon`: add assertion on `filePath.IsParentOnly()`

   - `SafeReadFile` / `SafeReadFileWithFS`: no mode assertion added (both constructors are valid callers)

3. New tests
    - Verify that passing a `ResolvedPath` created with `NewResolvedPath` to `SafeWriteFile`, `SafeWriteFileOverwrite`, and `SafeAtomicMoveFile` returns `ErrInvalidFilePath`

### Future Path

This decision does not preclude migrating to Option B (distinct type) if production callers of the write-family functions grow in number and misuse risk increases. However, migrating to Option B is not a pure additive change: it requires removing the `mode` field and all mode assertions introduced by Option C before adding the new type.
