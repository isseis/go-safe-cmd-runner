# pclntab (Program Counter Line Table) Maintenance Guide

## Overview

This document explains the parsing of the `.gopclntab` section (pclntab) in Go binaries, covering the procedures for handling Go version upgrades and relevant background information.

pclntab is an internal structure used by the Go runtime for stack trace generation and garbage collection; it is also used to recover function names and addresses from stripped binaries.

## pclntab Version History

| Go Version | pclntab Version | Magic Number | Key Changes |
|------------|-----------------|--------------|-------------|
| Go 1.2-1.15 | ver12 | `0xFFFFFFFB` | Initial format. All data stored in a single array |
| Go 1.16-1.17 | ver116 | `0xFFFFFFFA` | Table separation. Uses absolute pointers |
| Go 1.18-1.19 | ver118 | `0xFFFFFFF0` | Entry PC changed to 32-bit offset |
| Go 1.20-1.24 | ver120 | `0xFFFFFFF1` | pcHeader 80 bytes (with ftabOffset field) |
| **Go 1.25+** | **ver120** | **`0xFFFFFFF1`** | **pcHeader 72 bytes (ftabOffset removed). Currently supported** |

**Important**:
- The pclntab version does not always match the Go runtime version. For example, Go 1.19 uses ver118 (Go 1.18 format)
- Go 1.20-1.24 and Go 1.25+ use the same magic number (`0xFFFFFFF1`) but differ in header size (80 bytes vs 72 bytes)
- **This parser supports Go 1.25+ only**. All versions prior to Go 1.24 return `ErrUnsupportedPclntab`

## Header Structure

Minimum 8-byte header common to all versions:

```
Offset  Size  Content
0-3     4     Magic number (little-endian)
4-5     2     Padding (0x00, 0x00)
6       1     PC quantum (1, 2, or 4)
7       1     Pointer size (4 or 8)
8+      var   Version-specific data
```

**Notes**:
- Go 1.25+ requires a header of 72 bytes or more due to additional fields

### pcHeader Structure for Go 1.25+ (ver120)

```go
// pcHeader structure (Go 1.25+)
// Reference: https://go.dev/src/runtime/symtab.go
type pcHeader struct {
    magic          uint32  // offset 0x00: magic number
    pad1, pad2     uint8   // offset 0x04-0x05: padding
    minLC          uint8   // offset 0x06: minimum instruction size (PC quantum)
    ptrSize        uint8   // offset 0x07: pointer size (4 or 8)
    nfunc          int     // offset 0x08: number of functions
    nfiles         uint    // offset 0x10: number of file table entries
    textStart      uintptr // offset 0x18: base address for function entry PCs
    funcnameOffset uintptr // offset 0x20: offset to function name table
    cuOffset       uintptr // offset 0x28: offset to compilation unit table
    filetabOffset  uintptr // offset 0x30: offset to file table
    pctabOffset    uintptr // offset 0x38: offset to PC table
    pclnOffset     uintptr // offset 0x40: offset to pclntab data (also serves as functab)
}
```

**Notes**:
- The sizes of `nfunc` and `nfiles` depend on `ptrSize` (32-bit: 4 bytes, 64-bit: 8 bytes)
- In Go 1.25+, the `ftabOffset` field is removed and `pclnOffset` also serves as the functab offset
- Total header size: 72 bytes (0x48) for 64-bit
- In Go 1.20-1.24, the 80-byte (0x50) header included `ftabOffset`, but this parser does not support that format

## Procedures for Handling New Versions

### 1. Change Detection (Estimated time: 1-2 hours)

When a new Go major/minor version is released:

1. **Checking magic numbers**
   - Check Go source code [`src/debug/gosym/pclntab.go`](https://go.dev/src/debug/gosym/pclntab.go)
   - Check whether new magic number constants have been added

2. **Checking runtime structure**
   - Check the `pcHeader` struct in [`src/runtime/symtab.go`](https://go.dev/src/runtime/symtab.go)
   - Check for added, changed, or removed fields

3. **When there are no changes**
   - The pclntab version often lags behind the runtime version
   - For example, Go 1.21, 1.22, and 1.23 continued to use ver120 (Go 1.20 format)
   - In this case, no action is required

### 2. Parser Modification (Estimated time: 2-4 hours)

When changes are detected:

1. **Adding magic number constants**
   ```go
   const (
       pclntabMagicGo120 = 0xFFFFFFF1  // Go 1.20+ (currently supported)
       pclntabMagicGoXXX = 0xXXXXXXXX  // newly added
   )
   ```

2. **Adding parse functions**
   ```go
   func (p *pclntabParser) parseGoXXX(data []byte) error {
       // parsing logic specific to the new version
   }
   ```

3. **Updating the switch statement**
   ```go
   switch magic {
   case pclntabMagicGoXXX:
       return p.parseGoXXX(data)
   // ... existing cases
   }
   ```

### 3. Adding Tests (Estimated time: 2-3 hours)

1. **Creating test binaries**
   - Prepare test binaries compiled with the new version of Go
   - Also prepare a stripped version

2. **Adding unit tests**
   - Verify that parsing of the new version's pclntab succeeds
   - Verify that function names and addresses are correctly extracted

### 4. Updating Documentation (Estimated time: 30 minutes)

1. Update the version history table in this document
2. Update the magic number list in the requirements document (`docs/tasks/0070_elf_syscall_analysis/01_requirements.md`)

## Estimated Maintenance Cost

| Scenario | Working Time | Frequency |
|----------|-------------|-----------|
| No changes (verification only) | 1-2 hours | approximately 70% of Go major releases |
| Minor changes (offset adjustments, etc.) | 4-6 hours | approximately 20% of Go major releases |
| Major structural changes | 1-2 days | approximately 10% of Go major releases |

**Notes**:
- Go's major releases are approximately every 6 months (February and August)
- Major structural changes to pclntab occur approximately once every 2-3 years (occurred in Go 1.16, 1.18, 1.20)
- For minor changes, the change diff in the official `debug/gosym` package can be used as a reference

## Reference Resources

### Official Go Source Code

- [debug/gosym/pclntab.go](https://go.dev/src/debug/gosym/pclntab.go) - Official implementation of the pclntab parser
- [runtime/symtab.go](https://go.dev/src/runtime/symtab.go) - pclntab structure definition in the runtime
- [cmd/link/internal/ld/pcln.go](https://go.dev/src/cmd/link/internal/ld/pcln.go) - pclntab generation in the linker

### External Tools and Documentation

- [GoReSym](https://github.com/mandiant/GoReSym) - Mandiant's Go symbol recovery tool. Useful as an implementation reference for multi-version support
- [Go 1.2 Runtime Symbol Information](https://docs.google.com/document/d/1lyPIbmsYbXnpNj57a261hgOYVpNRcgydurVQIyZOz_o/pub) - The original design document for pclntab (as of Go 1.2)
- [Golang Internals: Symbol Recovery](https://cloud.google.com/blog/topics/threat-intelligence/golang-internals-symbol-recovery) - Detailed explanation on the Google Cloud Blog

## Consideration of Alternative Approaches

If maintenance cost of pclntab becomes an issue, the following alternative approaches can be considered:

### 1. Using the debug/gosym Package

A method of directly using Go's standard library `debug/gosym` package.

**Advantages**:
- Automatically handles new versions with Go core updates
- Maintenance cost is nearly zero

**Disadvantages**:
- Requires `.gosymtab` section for stripped binaries (empty since Go 1.3)
- Cannot handle stripped binaries on its own

### 2. Using GoReSym as a Library

A method of referencing [GoReSym](https://github.com/mandiant/GoReSym)'s code or using it as a library.

**Advantages**:
- Supports multiple versions
- Partially supports stripped binaries and obfuscated binaries
- Actively maintained

**Disadvantages**:
- Increases external dependencies
- License check required (Apache 2.0)

### 3. Limiting Supported Versions

A method of supporting only the current version (Go 1.25+) and returning `ErrUnsupportedPclntab` for older versions. This is the currently adopted approach.

**Advantages**:
- Simplifies implementation and maintenance
- Eliminates untested code paths
- In practice, binaries built with older versions of Go are a minority

**Disadvantages**:
- Binaries from Go 1.24 and earlier cannot be parsed (result in an error)

## Related Files

- `internal/runner/security/elfanalyzer/pclntab_parser.go` - pclntab parser implementation
- `internal/runner/security/elfanalyzer/go_wrapper_resolver.go` - Go wrapper analysis using pclntab
- `docs/tasks/0070_elf_syscall_analysis/03_detailed_specification.md` - detailed design
- `docs/tasks/0077_cgo_binary_network_detection/05_requirements_pclntab_symtab_free.md` - pclntab address shift issue in CGO binaries and requirements for eliminating `.symtab` dependency (starting point for investigation if the issue recurs in CGO binaries). Note that that document targets magic = `0xfffffff1` (Go 1.20+), but this is because parsing is delegated to `debug/gosym` and the pcHeader structure is not directly analyzed. The "Go 1.25+ only support" in this document refers to the pcHeader structure (presence or absence of ftabOffset), and these are at different layers.
- `docs/tasks/0077_cgo_binary_network_detection/ac1_verification_result_x86_64.md` - measured address shift data for x86_64 CGO binaries (−0x100)
