# ELF DT_RPATH / DT_RUNPATH Inheritance Rules

This document is a reference that summarizes the search and inheritance behavior of `DT_RPATH` and `DT_RUNPATH` in Linux ld.so. It serves as the design rationale for the `dynlibanalysis` package.

## 1. Basic Differences

| Attribute | Scope | Inheritance | Order relative to LD_LIBRARY_PATH |
|-----------|-------|-------------|----------------------------------|
| `DT_RPATH` | Direct dependencies + **all transitive dependencies** | **Inherited** (subject to termination conditions described below) | `DT_RPATH` → LD_LIBRARY_PATH → ... |
| `DT_RUNPATH` | **Direct dependencies only** | Not inherited | LD_LIBRARY_PATH → `DT_RUNPATH` → ... |

> **Source**: `man 8 ld.so` — "DT_RUNPATH directories are searched only to find objects required by DT_NEEDED entries and do not apply to those objects' children, which must themselves have their own DT_RUNPATH entries. This is unlike DT_RPATH, which is applied to searches for all children in the dependency tree."

## 2. ld.so Search Order

The search order when ld.so resolves a soname (based on glibc implementation):

1. **DT_RPATH** — The `DT_RPATH` of the loading object (the ELF that "loads" the library being resolved). **Skipped** if the loading object has `DT_RUNPATH`
2. **Ancestor DT_RPATH inheritance chain** — Walks up to the loader (parent) → its loader (grandparent) → ... checking each one's `DT_RPATH`. **Terminated** when an ELF with `DT_RUNPATH` is encountered (see §3)
3. **LD_LIBRARY_PATH** — Environment variable (not used at record time for security reasons)
4. **DT_RUNPATH** — The `DT_RUNPATH` of the loading object
5. **/etc/ld.so.cache**
6. **Default paths** — `/lib`, `/usr/lib`, etc. (architecture-dependent)

> **Source**: glibc `elf/dl-load.c`, `_dl_map_object` implementation. "Unless the loading object has RUNPATH, the RPATH of the loading object is checked, then the RPATH of its loader (unless it has a RUNPATH), and so on until the end of the chain."

## 3. DT_RPATH Inheritance Chain Termination Rules

**Condition that triggers termination**:

> When the loading object (the ELF that loads the library being resolved) itself has `DT_RUNPATH`, the ancestor RPATH inheritance chain is terminated.

This reflects the glibc implementation behavior directly:
- If the loading object has `DT_RUNPATH`: skip its own `DT_RPATH` (Step 1) and skip walking up the ancestor RPATH chain (Step 2)
- If the loading object has no `DT_RUNPATH`: use its own `DT_RPATH` and continue walking up to the loader (parent). However, if the parent has `DT_RUNPATH`, stop there

### Concrete Examples

```
main(RPATH=/gp) → libA(no RPATH, no RUNPATH) → libB(RUNPATH=/b) → libC
```

| Resolving | Loading object | Search paths used | Reason |
|-----------|---------------|-------------------|--------|
| libA | main | /gp (main's RPATH) | main has no RUNPATH → uses its own RPATH |
| libB | libA | /gp (inherited from main) | libA has no RPATH/RUNPATH → walks up to loader (main) |
| libC | libB | /b (libB's RUNPATH) | libB has RUNPATH → skips its own RPATH (none) and ancestor chain |

```
grandparent(RPATH=/gp) → parent(RPATH=/p, no RUNPATH) → child(RUNPATH=/c) → grandchild
```

| Resolving | Loading object | Search paths used | Reason |
|-----------|---------------|-------------------|--------|
| parent | grandparent | /gp | grandparent has no RUNPATH |
| child | parent | /p, /gp (inherited) | parent has no RUNPATH → uses /p, then walks up to grandparent's /gp |
| grandchild | child | /c only | child has RUNPATH → ancestor chain (/p, /gp) is not used |

## 4. $ORIGIN Expansion

`$ORIGIN` expands to the **directory of the ELF file that defines the `DT_RPATH`/`DT_RUNPATH` entry** containing `$ORIGIN`.

For inherited RPATH entries, `$ORIGIN` expands to the **directory of the ELF that originally defined that entry** (= `OriginDir`), not the directory of the loading object.

```
/app/bin/main (RPATH=$ORIGIN/../lib) → /app/lib/libA.so → /app/lib/libB.so
```

When resolving libB, `$ORIGIN` expands to main's directory `/app/bin`, so the search path becomes `/app/bin/../lib` = `/app/lib`.

## 5. Coexistence of DT_RPATH and DT_RUNPATH

When both are present in the same ELF, `DT_RUNPATH` takes priority and `DT_RPATH` is ignored.

In the glibc implementation, `DT_RPATH` is only consulted when `DT_RUNPATH` is absent.

## 6. Design Mapping in the dynlibanalysis Package

How the above rules are expressed in `ResolveContext`:

| ld.so rule | `ResolveContext` representation |
|------------|--------------------------------|
| Own RPATH (when no RUNPATH) | `OwnRPATH` |
| Own RUNPATH | `OwnRUNPATH` |
| Ancestor RPATH inheritance chain | `InheritedRPATH []ExpandedRPATHEntry` (each entry carries `OriginDir`) |
| RUNPATH overrides RPATH | `NewRootContext`/`NewChildContext` set `OwnRUNPATH` and `OwnRPATH` mutually exclusively |
| Loading object with RUNPATH terminates inheritance | `NewChildContext` leaves `InheritedRPATH = nil` when `childRUNPATH` is non-empty |

### Termination Logic in NewChildContext

```go
if len(childRUNPATH) > 0 {
    child.OwnRUNPATH = childRUNPATH
    // InheritedRPATH remains nil (chain terminated)
} else {
    // Accumulate parent's and ancestor's RPATH into InheritedRPATH
}
```

This corresponds to glibc's behavior of not using ancestor RPATHs when resolving a child's DT_NEEDED entries, given that the child has RUNPATH. Discarding `c.InheritedRPATH` (the ancestor RPATH already accumulated by the parent) is correct, because glibc stops walking the entire loader chain the moment the loading object (child) has RUNPATH.

### Search Order in Resolve()

```
1. OwnRPATH      (only when OwnRUNPATH is absent)
2. InheritedRPATH
3. LD_LIBRARY_PATH (runner time only)
4. OwnRUNPATH
5. /etc/ld.so.cache
6. Default paths
```

`OwnRUNPATH` appearing at Step 4 reflects ld.so's search order (RPATH → LD_LIBRARY_PATH → RUNPATH → cache → default).

## 7. Common Misconceptions

### Misconception 1: "Even if a child has RUNPATH, ancestor RPATHs are still used for resolving the child's own dependencies"

**Incorrect.** glibc does not walk the loader RPATH chain when the loading object (child) has RUNPATH. Setting `InheritedRPATH = nil` is correct.

### Misconception 2: "Chain termination occurs when any ancestor has RUNPATH"

**Incorrect.** Termination is determined by whether the **loading object itself** has RUNPATH, not its ancestors. Even if an ancestor has RUNPATH, the chain continues for ELFs downstream that do not have RUNPATH as their loading object.

```
main(RPATH=/gp) → libA(RUNPATH=/a) → libB(no RPATH, no RUNPATH) → libC
```

- When libB is the loading object: libB has no RUNPATH → walks up to loader (libA)
  - libA has RUNPATH → chain terminates at libA
  - libA's RUNPATH (/a) applies only to libA's **direct dependencies (libB)** and is not used for libC
  - main's /gp is **not used** for resolving libC either
- Therefore, search paths for libC: no RPATH/RUNPATH → LD_LIBRARY_PATH → /etc/ld.so.cache → default paths

### Misconception 3: "DT_RUNPATH uses the same search order as DT_RPATH but just isn't inherited"

**Incorrect.** The search order is also different. `DT_RPATH` is searched **before** `LD_LIBRARY_PATH`, but `DT_RUNPATH` is searched **after** `LD_LIBRARY_PATH`. This is an important distinction for `LD_LIBRARY_PATH` hijack detection.

## 8. References

- `man 8 ld.so` (Linux manual page): https://man7.org/linux/man-pages/man8/ld.so.8.html
- glibc source: `_dl_map_object` function in `elf/dl-load.c`
- Implementation: [`internal/dynlibanalysis/resolver_context.go`](../../internal/dynlibanalysis/resolver_context.go)
- Implementation: [`internal/dynlibanalysis/resolver.go`](../../internal/dynlibanalysis/resolver.go)
- Specification: [`docs/tasks/0074_elf_dynlib_integrity/03_detailed_specification.md`](../tasks/0074_elf_dynlib_integrity/03_detailed_specification.md) §3.3, §3.4
