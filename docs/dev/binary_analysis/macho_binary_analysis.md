# Mach-O Binary Analysis: Technical Details

This document records the technical behavior specification and design rationale for Mach-O binary analysis on macOS.

ELF binary analysis is handled by `internal/security/elfanalyzer/`, whereas Mach-O binary analysis is implemented across the following three packages.

| Package | Role |
|---|---|
| `internal/security/machoanalyzer/` | Network symbol detection + arm64-only 2-pass syscall analysis |
| `internal/dynlib/machodylib/` | `LC_LOAD_DYLIB` recursive analysis (hash recording of dependency libraries) |
| `internal/libccache/` Mach-O side | `libsystem_kernel.dylib` syscall wrapper symbol cache |

## 1. Network Symbol Detection

### 1.1 Purpose and Supported Architectures

`StandardMachOAnalyzer` implements the `binaryanalyzer.BinaryAnalyzer` interface and determines whether a Mach-O binary imports network-related symbols such as socket, connect, and bind.
It supports both single-architecture Mach-O files and Fat binaries; symbol detection works for all architectures.

### 1.2 How Symbol Detection Works

macOS Mach-O binaries use the two-level namespace by default, so the library ordinal (bits[15:8]) for each symbol is stored in the `Desc` field.

**Normal path (Symtab available)**:

1. Use `Dysymtab` to obtain the range of undefined external symbols.
2. Extract the library ordinal from each symbol's `Desc` field and compare against the `LC_LOAD_DYLIB` list to determine whether the symbol originates from `libSystem`.
3. Apply network categorization to `libSystem`-derived symbols by matching against `binaryanalyzer.GetNetworkSymbols()`.

```
libSystem origin determination:
  1. ordinal is in range and the corresponding library is libSystem.B.dylib or libsystem_*.dylib
  2. OR flat namespace (all symbol ordinals are 0) AND
     libSystem is present in imported libraries
```

**Fallback path (Symtab unavailable)**:

For stripped binaries without `Symtab`, if `ImportedLibraries()` includes `libSystem`, all imported symbols are obtained via `ImportedSymbols()` and detected.

### 1.3 Handling of Fat Binaries

For Fat binaries, all slices (all architectures) are analyzed and the most severe risk is reported.
This prevents cross-architecture security bypass attacks where an arm64 slice appears safe while an x86_64 slice has network capabilities.

```
Severity priority: NetworkDetected > AnalysisError > NoNetworkSymbols

If any slice is NetworkDetected → report the entire binary as NetworkDetected
```

## 2. syscall Static Analysis (arm64 Only)

### 2.1 macOS syscall Convention

macOS arm64 binaries use the BSD syscall convention.
The following differences exist compared to the Linux ABI for ELF arm64.

| Item | macOS (Mach-O) | Linux (ELF) |
|---|---|---|
| syscall instruction | `SVC #0x80` (0xD4001001) | `SVC #0` (0xD4000001) |
| syscall number register | **X16** | W8/X8 |
| Go wrapper argument passing | **stack** (`[SP, #8]`) | register (X0) |

### 2.2 Analysis Flow Overview

```
ScanSyscallInfos (svc_scanner.go)
  ↓
  Fat binary: run analyzeArm64Slice on every arm64 slice
  Single Mach-O: run analyzeArm64Slice
  ↓
  analyzeArm64Slice
    ├── ParseMachoPclntab → obtain Go function address ranges
    ├── buildStubRanges  → construct ranges for known wrapper functions
    ├── Pass 1: collectSVCAddresses + scanSVCWithX16
    └── Pass 2: buildWrapperAddrs + scanGoWrapperCalls
```

### 2.3 pclntab Parsing

`ParseMachoPclntab` (`pclntab_macho.go`) parses the section in the Mach-O file that corresponds to `.gopclntab` and returns a map (`map[string]MachoPclntabFunc`) of Go function names to their entry and end addresses.

From this map, `buildStubRanges` constructs the address ranges of `knownMachoSyscallImpls` (`syscall.Syscall`, `syscall.RawSyscall`, etc.).
`SVC #0x80` instructions inside wrapper functions are skipped in both Pass 1 and Pass 2.

### 2.4 Pass 1: SVC #0x80 Scan and X16 Backward Scan

`collectSVCAddresses` traverses the `__TEXT,__text` section in 4-byte units and enumerates the virtual addresses of `SVC #0x80` (byte sequence `0x01 0x10 0x00 0xD4`).

`scanSVCWithX16` performs the following processing for each address.

1. Skip the address if it falls inside a known wrapper function body (Pass 2 handles those).
2. Backward-scan the instruction sequence immediately preceding `SVC #0x80` using `arm64util.BackwardScanX16` to search for an immediate load into X16.
3. If an immediate value is found, record it with `DeterminationMethod = "immediate"`.
4. If not found, record it with `DeterminationMethod = "direct_svc_0x80"` (a High Risk signal indicating that a direct syscall was observed even though the number could not be resolved).

```asm
; typical pattern detected by Pass 1 (non-Go binary)
MOV  X16, #4            ; load syscall number 4 (write) into X16
SVC  #0x80              ; invoke BSD syscall
```

### 2.5 Pass 2: Go Wrapper Calls and Stack ABI

Go's NOSPLIT assembly stubs such as `syscall.Syscall` / `syscall.RawSyscall` use the **stack-based ABI** rather than the calling convention.
The caller stores the trap number at `SP+8` (`trap+0(FP)`) before the BL instruction that calls the wrapper.

`scanGoWrapperCalls` performs the following steps.

1. Traverse `__TEXT,__text` in 4-byte units and compute the target address of each BL instruction.
2. Check whether the target matches a known wrapper address.
   One-level trampolines (`B <wrapper>`) are also resolved.
3. Skip BL instructions that originate from inside a stub body (internal calls are excluded).
4. Backward-scan the instruction sequence immediately preceding the BL using `arm64util.BackwardScanStackTrap` to search for a write to `[SP, #8]` (STR/STP) and resolve the immediate load into the source register.

```go
// typical pattern generated by Go caller
// stores trap number at SP+8 before calling syscall.Syscall
MOV  X0, #4             // trap number (write)
STR  X0, [SP, #8]       // store at trap+0(FP)
...
BL   _syscall.Syscall   // call Go wrapper
```

**Difference from ELF arm64**: In ELF arm64, Go wrappers such as `syscall.Syscall` use the register-based ABI and pass the first argument (syscall number) in the X0 register.
In Mach-O, NOSPLIT stubs use `[SP, #8]`.

## 3. Dynamic Library Dependency Analysis

### 3.1 Recursive Analysis via Breadth-First Search

`MachODynLibAnalyzer.Analyze` (`machodylib/analyzer.go`) recursively resolves `LC_LOAD_DYLIB` and `LC_LOAD_WEAK_DYLIB` dependencies using breadth-first search (BFS).

```
Analyze(binaryPath)
  ↓ initialize queue: enqueue direct dependencies of the binary
  ↓ BFS loop (maximum depth: MaxRecursionDepth = 20)
    ├── install name → real path conversion
    │     @rpath         → refer to LC_RPATH entries
    │     @loader_path   → directory of the Mach-O file
    │     @executable_path → directory of the binary
    ├── IsDyldSharedCacheLib → exclude dyld shared cache libraries (see below)
    ├── SHA256 hash computation
    ├── append LibEntry (SOName, Path, Hash)
    └── enqueue dependencies of the resolved library
```

### 3.2 Exclusion of dyld Shared Cache

Many macOS system libraries (such as `libSystem.B.dylib`) do not exist as individual files on disk; instead they are consolidated into the **dyld shared cache**.
For this reason, they must be excluded from hash verification.

`IsDyldSharedCacheLib` (`dyld_extractor_darwin.go`) identifies cache libraries by the following path patterns.

- Libraries under `/usr/lib/`
- Frameworks under `/System/Library/Frameworks/`
- Other paths that Apple includes in the dyld shared cache

Excluded libraries are not included in `LibEntry` and are not subject to hash verification at runner runtime.

### 3.3 Verification at Runner Runtime

When recorded `DynLibDeps` are present, the runner's `VerifyCommandDynLibDeps` matches the hash of each library against the actual file on disk.
For dynamically linked binaries whose `DynLibDeps` are not recorded, re-recording is required (`ErrDynLibDepsRequired`).

dyld shared cache libraries are not recorded and therefore their changes are not subject to verification.
This is an intentional design exclusion to allow the runner to start even when the cache is replaced by a macOS system update.

## 4. libsystem_kernel.dylib Wrapper Cache

### 4.1 Role

Go wrappers such as `syscall.Syscall` ultimately call `_syscallname` functions (e.g., `_write`, `_socket`) inside `libsystem_kernel.dylib` via BL.
These functions are syscall wrappers that contain `SVC #0x80`.

`MachoLibSystemAnalyzer` (`libccache/macho_analyzer.go`) scans the `__TEXT,__text` section of `libsystem_kernel.dylib` and enumerates functions that contain `SVC #0x80` as `WrapperEntry{Name, Number}`.

### 4.2 Cache Mechanism

`MachoLibSystemCacheManager` (`libccache/macho_cache.go`) caches the analysis results under the hash directory of the record command.
The cache key is the library path and SHA256 hash; a cache miss occurs when the library is updated, triggering re-analysis.

This limits the analysis cost of `libsystem_kernel.dylib` (full-function SVC scan) to the first record invocation only.

### 4.3 Relationship with libccache

The ELF-side `LibcCacheManager` (for Linux glibc) and the Mach-O-side `MachoLibSystemCacheManager` share the same cache directory and the same schema (`LibcCacheFile`).
However, their implementations are independent in terms of the target library and the arm64-only constraint.

## 5. Key Differences from ELF arm64

| Item | Mach-O (machoanalyzer) | ELF (elfanalyzer) |
|---|---|---|
| syscall instruction | `SVC #0x80` (0xD4001001) | `SVC #0` (0xD4000001) |
| syscall number register | X16 | W8/X8 |
| Go wrapper arguments | stack `[SP, #8]` | register X0 |
| pclntab parsing | `ParseMachoPclntab` (Mach-O specific) | `parsePclntabFuncs` (.gopclntab ELF section) |
| dynamic library analysis | `machodylib` (LC_LOAD_DYLIB + dyld cache exclusion) | `elfdynlib` (DT_NEEDED + ldconfig cache) |
| libc cache | `MachoLibSystemCacheManager` (libsystem_kernel.dylib) | `LibcCacheManager` (glibc) |
| Fat binary | analyzes all slices, reports worst case | not applicable (ELF is single-architecture) |
| trampoline resolution | resolves one-level B stub | none |

## 6. Design Rationale

### 6.1 Reason for Keeping machoanalyzer as a Separate Package from elfanalyzer

ELF and Mach-O are fundamentally different in binary format, syscall convention, and library system, so there is little code that can be shared and integration would increase complexity.
In addition, macOS-specific features depend on Darwin-specific APIs (`debug/macho`, dyld cache), so separating them from ELF analysis enables support for both platforms without build tags.

### 6.2 Reason for Excluding dyld Shared Cache

The dyld shared cache is updated by macOS system updates but does not exist as individual files, so SHA256 hashes cannot be computed.
Furthermore, the contents of the dyld shared cache cannot be modified from user space, so the practical benefit of subjecting it to integrity verification is limited.
On the other hand, non-cache libraries (such as custom frameworks via `@rpath`) can potentially be replaced by an attacker and are therefore subject to hash verification.

### 6.3 Reason Pass 2 Uses the Stack ABI

Go's NOSPLIT assembly stubs such as `syscall.Syscall` maintain the old stack ABI rather than the arm64 Go register ABI (introduced in Go 1.17).
This is because NOSPLIT stubs do not switch the goroutine stack, so the old ABI must be maintained for compatibility with the Go runtime's stack management.
Therefore, the backward scan in Pass 2 must track `[SP, #8]` rather than X0.

### 6.4 Analyzing All Slices of Fat Binaries

A malicious developer could make the arm64 slice appear safe while embedding network capabilities or dangerous syscalls in the x86_64 slice (cross-slice attack).
Analyzing all slices and reporting the worst case prevents this bypass.
