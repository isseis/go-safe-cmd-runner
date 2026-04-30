# Technical Details of arm64 Instruction Decoding

This document records the technical operational specifications and design decisions
for arm64 instruction decoding in the syscall static analysis feature based on
ELF machine code analysis.

## 1. Overall Structure of Analysis

`SyscallAnalyzer` abstracts both x86_64 and arm64 architectures via the
`MachineCodeDecoder` interface. The arm64 implementation is handled by
the `ARM64Decoder` struct.

The analysis consists of two passes.

```
Pass 1: findSyscallInstructions
  → Forward scan of the .text section to enumerate SVC #0 instruction (0xD4000001) positions
  → For each SVC #0 instruction, resolve the syscall number via backwardScanForRegister

Pass 2: FindWrapperCalls (ARM64GoWrapperResolver)
  → Forward scan of the .text section to detect BL instructions targeting Go syscall wrappers
  → Backward scan of preceding instructions to resolve the first argument (X0/W0) value
```

If a SVC #0 instruction detected in Pass 1 is located inside a known wrapper
function or implementation function, it is skipped because the number cannot
be determined statically.

## 2. Behavior on Decode Failure

### 2.1 Alignment Guarantee from Fixed-Length Instructions

All arm64 instructions are a fixed 4 bytes in length. Therefore, the value
returned by `InstructionAlignment()` is always 4, and advancing by 4 bytes
on decode failure always preserves instruction boundary alignment.

```
Failure position: pos
Next attempt position: pos + 4
```

Unlike x86_64 variable-length instructions (1–15 bytes), arm64 is structurally
free from instruction boundary misalignment. There is no risk of re-synchronizing
to an incorrect instruction boundary after a decode failure.

### 2.2 Causes of Decode Failure

Because all instructions in the arm64 `.text` section are aligned at 4-byte
boundaries, decode failures arise primarily from the following causes.

- Undefined instruction encodings
- Instruction variants not yet supported by the arm64asm library
- Cases where data regions are interleaved with the `.text` section

Even in these cases, skipping by 4 bytes ensures that subsequent instruction
boundaries remain correct at all times.

### 2.3 False Positive Risk

Because arm64 uses fixed-length instructions, the false positive risk of
detecting SVC #0 due to instruction boundary misalignment—as can occur in
x86_64—does not exist. Even if a `0xD4000001` pattern appears in a non-SVC
context (e.g., coincidental match in a data region), the backward scan
attempts to resolve the syscall number, and an invalid SVC #0 instruction
will result in one of the following.

- No valid syscall number is found → treated as `unknown:*`, classified as High Risk
- A coincidentally valid number is found → false positive (theoretically possible
  but extremely rare in practice)

## 3. Syscall Number Resolution via Backward Scan

### 3.1 Overall Flow (backwardScanForSyscallNumber)

On arm64, unlike x86_64 which uses `backwardScanX86WithRegCopy`, the generic
function `backwardScanForRegister` is used. Register copy chain tracking and
CFG-based branch convergence analysis are not performed.

```
backwardScanForSyscallNumber
  → When the decoder is not ARM64Decoder: call backwardScanForRegister
    ↓
    [1] Window calculation: windowStart = syscallOffset - (maxBackwardScan × 4)
    [2] Decode instructions in the window via decodeWindow
    [3] Scan the instruction sequence from the end:
        - Control flow instruction → unknown:control_flow_boundary
        - Not a write to W8/X8 → skip
        - Immediate load to W8/X8 → retrieve immediate and return
        - Indirect write to W8/X8 → unknown:indirect_setting
    [4] Scan limit reached (maxBackwardScan = 50) → unknown:scan_limit_exceeded
    [5] All instructions in window consumed → unknown:window_exhausted
```

Because arm64 uses fixed-length instructions, the alternative window start
position search required by x86_64 to handle boundary misalignment after
decode failures is unnecessary.

### 3.2 Target Register for Scanning

The arm64 convention places the syscall number in the W8 or X8 register.
`WritesSyscallReg` determines whether the first operand of an instruction
writes to W8 or X8. Instructions with read-only first operands (STR, CMP,
CMN, TST, etc.) are excluded.

### 3.3 Validation of Valid Syscall Numbers

Even when an immediate value is obtained, values exceeding
`maxValidSyscallNumber = 500` or negative values are treated as
`unknown:indirect_setting`.

## 4. Immediate Load Encodings

On arm64, two encodings are recognized for instructions that set the syscall
number in the W8/X8 register.

### 4.1 MOV W8/X8, #imm (MOVZ Normalization)

The compiler generates 16-bit immediates as MOVZ instructions, and the
arm64asm library normalizes these to `MOV Wn/Xn, #imm`. `IsSyscallNumImm`
checks for the `arm64asm.MOV` opcode with a W8 or X8 destination and
retrieves the immediate value.

```asm
MOV W8, #198    ; Generated as MOVZ W8, #198; arm64asm normalizes to MOV
SVC #0
```

### 4.2 ORR W8/X8, WZR/XZR, #imm (Bitmask Immediate Form)

Constants that cannot be represented by MOVZ but fit the ARM64 bitmask
immediate format may be generated by the compiler as
`ORR Wn/Xn, WZR/XZR, #imm`. This is functionally equivalent to
`MOV Wn, #imm`. The `arm64OrrZeroRegImm` function detects this pattern.

```asm
ORR X8, XZR, #imm   ; Functionally equivalent to MOV X8, #imm
SVC #0
```

## 5. Global Variable Resolution via ADRP+LDR

### 5.1 Target Pattern

In some cases, Go's syscall package stores syscall numbers in package-level
variables. On arm64, the following ADRP+LDR instruction pair is generated.

```asm
ADRP Xn, <page>          ; Load page base address into Xn
LDR  X0/W0, [Xn, #offset] ; Load value from within-page offset
BL   <wrapper>
```

### 5.2 Behavior of ResolveFirstArgGlobal

`ARM64Decoder.ResolveFirstArgGlobal` is called during the backward scan in
Pass 2 (Go wrapper analysis) and resolves the value through the following steps.

1. Confirm the instruction is in the form `LDR X0/W0, [Xn, #offset]`
   (unsigned offset addressing mode).
2. Extract the `imm12` field from the encoding and shift it according to the
   register size to compute the byte offset.
   - X0 (64-bit): `offset = imm12 << 3`
   - W0 (32-bit): `offset = imm12 << 2`
3. Backward scan of preceding instructions up to `arm64ADRPBacktrackLimit = 4`
   instructions to find an ADRP instruction targeting the same base register.
4. Compute the effective address of the ADRP instruction:
   `(instOffset & ~0xFFF) + sign_extend(pcrel)`.
5. Read the value at that address from ELF sections registered via
   `SetDataSections` (`.noptrdata`, `.rodata`, `.data`), as a
   little-endian integer (4 bytes for W0, 8 bytes for X0).

### 5.3 Differences from RIP-Relative Addressing

While x86_64 uses `MOV RAX, [RIP + disp32]` in RIP-relative form,
arm64 requires a two-instruction ADRP+LDR pair. Consequently, resolution
on the arm64 side requires a backward scan of preceding instructions rather
than decoding a single instruction.

## 6. Transparent Wrapper Detection

### 6.1 What Are Transparent Wrappers

In the Go runtime, functions may be generated that further wrap wrappers such
as `syscall.Syscall`. These "transparent wrappers" have a structure that
receives arguments via the stack, calls an internal helper, saves the return
value to the stack, and ultimately calls a known wrapper function.

### 6.2 Behavior of discoverTransparentWrappers

`ARM64GoWrapperResolver.discoverTransparentWrappers` searches for the following
structural pattern using a sliding window.

```
[Prologue] STR X30, [SP, #-n]!        ← Function entry (save return address)
...
[Stack save] STR X0, [SP, #offset]    ← Spill argument to stack
...
[Helper call] BL <helper>             ← CALL to a non-wrapper function
...
[Stack reload] LDR X0, [SP, #offset]  ← Reload argument from stack
...
[Wrapper call] BL <known wrapper>     ← CALL to a known wrapper
[Tail] RET                            ← Function return
```

The window size is controlled by the following constants.

| Constant | Value | Meaning |
|---|---|---|
| `arm64ReloadSearchWindow` | 8 instructions | Search range for stack reload |
| `arm64HelperSearchWindow` | 15 instructions | Search range for helper call |
| `arm64SaveSearchWindow` | 15 instructions | Search range for stack save |
| `arm64PrologueSearchWindow` | 6 instructions | Search range for prologue |
| `arm64FunctionTailSearchSpan` | 24 instructions | Lookahead range for RET detection |

Detected transparent wrappers are added to `wrapperAddrs`, and their ranges
are registered in `wrapperRanges`. These are also used in the skip determination
for direct SVC #0 instructions in Pass 1 (`IsInsideWrapper`).

### 6.3 Efficiency of the Sliding Window

Rather than holding the entire section as an array in memory, the analysis
slides a window of fixed size (up to 69 instructions), achieving O(1) memory
usage with respect to section size while covering the entire section.

## 7. Determination Methods and Detail Codes

### 7.1 DeterminationMethod Constants

The same constants as x86_64 are used (no arm64-specific detail codes exist).

| Constant | Meaning |
|---|---|
| `immediate` | Determined directly from an immediate value |
| `go_wrapper` | Determined from the argument of a Go wrapper call |
| `unknown:decode_failed` | Unknown due to instruction decode failure |
| `unknown:control_flow_boundary` | Unknown because a control flow boundary was reached |
| `unknown:indirect_setting` | Unknown due to indirect setting (memory reference, indirect register, etc.) |
| `unknown:scan_limit_exceeded` | Scan step limit (`defaultMaxBackwardScan = 50`) reached |
| `unknown:window_exhausted` | All instructions in the window were consumed without a result |
| `unknown:invalid_offset` | Unknown because the SVC #0 instruction offset is invalid |

### 7.2 Detail Codes Specific to arm64

Because arm64 does not perform register copy chain tracking or CFG branch
convergence analysis, the x86_64-specific detail codes (`x86_copy_chain`,
`x86_branch_converged`, etc.) are not used. The `DeterminationDetail` field
is set only in the `invalid_offset` case.

## 8. Rationale for Design Decisions

### 8.1 Simplification of Design via Fixed-Length Instructions

arm64's fixed-length instructions (4 bytes) substantially simplify the
implementation of instruction decoding compared to x86_64's variable-length
instructions (1–15 bytes).

1. **Re-synchronization on decode failure is unnecessary**: Advancing by
   4 bytes always reaches a correct instruction boundary, eliminating the
   need for the "alternative window start position search" required by x86_64.
2. **No false positive risk from boundary misalignment**: In x86_64, there
   is a risk of falsely detecting `0F 05` during re-synchronization after a
   decode failure. This risk does not structurally exist in arm64.

### 8.2 Reason for Omitting Copy Chain and CFG Analysis

The arm64 compiler (in particular, Go) typically generates a pattern that
loads the syscall number directly into W8/X8 as an immediate value. The
chaining register copy chain patterns seen in x86_64 (e.g., `MOV EAX, EDX`)
are rarely observed in arm64, so these complex analysis mechanisms are omitted.

### 8.3 Reason for Not Classifying Decode Failures as High Risk

The same reasoning as x86_64 applies.

1. **Pass 1 targets direct SVC #0 instructions**: decode failure has limited
   impact on the detection of SVC #0 instructions themselves. SVC #0 is a
   fixed 4-byte value (`0xD4000001`), and decode failure rarely affects the
   detection accuracy of this pattern directly.

2. **Decode failures are rarer on arm64 than on x86_64** due to fixed-length
   instructions, and the `.text` section is normally composed of valid instructions.

3. **Decode failures in Pass 2 (Go wrapper analysis)** do not necessarily
   mean a syscall wrapper call is missed. BL instructions are decoded normally
   in most cases, and surrounding decode failures are unlikely to affect
   resolution of the BL target.

### 8.4 Consistency with the Design Principle of Failing Safe

This design treats "syscall numbers that cannot be detected" as High Risk
(FR-3.1.4).

- **Decode failure**: The case where "the instruction itself cannot be recognized"
- **Unknown syscall number**: The case where "the SVC #0 instruction was recognized
  but its number cannot be identified"

These two are distinct problems and are treated separately.

- When a SVC #0 instruction is successfully decoded but the number is unknown → **High Risk**
- When decoding itself fails → **Log output only** (does not affect risk classification)

Results with `unknown:*` methods (SVC #0 instructions with unknown numbers)
are treated as High Risk (§8.5 / §9.1.2).
The decode failure count does not affect risk classification.

### 8.5 Visualization of Decode Failures

Decode failures are collected as statistics in the `DecodeStatistics` struct
and made visible through the following log output.

- **Individual log**: Emitted via `slog.Debug` in both Pass 1
  (`findSyscallInstructions`) and Pass 2 (`FindWrapperCalls`). The number of
  log entries is limited to `MaxDecodeFailureLogs` (= 10) for each pass.
- **Summary log**: Emitted via `slog.Debug` in the record command, including
  the file path, total decode failure count (Pass 1 + Pass 2 combined),
  and total bytes analyzed.

This enables investigation of binaries with frequent decode failures, and
allows for improvement of the analysis logic or manual verification of the
target binary as needed.
