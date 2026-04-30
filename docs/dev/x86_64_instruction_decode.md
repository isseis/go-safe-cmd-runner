# x86_64 Instruction Decode: Technical Details

This document records the technical behavior specification and design rationale
for x86_64 instruction decode in the syscall static analysis feature based on ELF machine code analysis.

## 1. Analysis Overview

`SyscallAnalyzer` abstracts both x86_64 and arm64 architectures via the `MachineCodeDecoder`
interface. The x86_64 implementation is provided by the `X86Decoder` struct.

Analysis consists of two passes.

```
Pass 1: findSyscallInstructions
  → forward-scans the .text section to enumerate SYSCALL instruction (0F 05) positions
  → resolves the syscall number for each SYSCALL instruction via backwardScanX86WithRegCopy

Pass 2: FindWrapperCalls (X86GoWrapperResolver)
  → forward-scans the .text section to detect CALL instructions to Go syscall wrappers
  → backward-scans the preceding instruction sequence to resolve the first argument (RAX/EAX) value
```

SYSCALL instructions detected in Pass 1 that fall within the body of known wrapper or
implementation functions are skipped, because the number cannot be determined statically.

## 2. Behavior on Decode Failure

### 2.1 Retry by 1-byte Skip

When instruction decode fails, the position advances by the value returned by
`InstructionAlignment()` (which is 1 for x86_64), and decode is retried.

```
failure position:      pos
next attempt position: pos + 1
```

This is a constraint arising from x86_64 variable-length instructions (1 to 15 bytes).
Because there is no reliable way to identify the next correct instruction boundary
with variable-length instructions, the design advances 1 byte at a time to search
for the next decodable position.

### 2.2 Resynchronization Mechanism

Because the `.text` section is normally composed almost entirely of valid instructions,
it resynchronizes to a correct instruction boundary within a few bytes after a decode failure.

```
Example: when instruction boundaries are shifted
Actual instruction sequence: [5-byte instruction][3-byte instruction][2-byte instruction]
Shifted start:                    ^start here
              → after approximately 1-3 decode failures, a correct instruction boundary is reached
```

In practice, resynchronization typically completes within 1 to 3 bytes for x86_64 code.
The worst case (15 bytes of invalid decodes) is rare, and correctness is not affected
once subsequent instructions are decoded.

### 2.3 False Positive Risk

If instruction boundaries shift during the resynchronization process after a decode failure,
and the byte pattern `0F 05` appears incidentally within a data region,
it may be incorrectly detected as a SYSCALL instruction.

However, since backward scan also attempts syscall number analysis in such cases,
an invalid SYSCALL instruction results in one of the following.

- No plausible syscall number found → classified as High Risk as `unknown:*`
- Incidentally appears to be a plausible number → false positive (a theoretical risk, but extremely rare in practice)

## 3. Window-based Decoding

### 3.1 Role of decodeWindow

In the backward scan of Pass 1, the byte sequence immediately preceding the SYSCALL
instruction is forward-decoded to reconstruct the instruction sequence.
Instead of decoding the entire section on each iteration, only the
`maxBackwardScan × maxInstructionLength` bytes immediately preceding the SYSCALL instruction
are extracted as a window (`decodeWindow`).

```
defaultMaxBackwardScan  = 50 instructions
maxInstructionLength    = 15 bytes (x86_64 instruction length limit)
window size (maximum)   = 50 × 15 = 750 bytes
```

### 3.2 Misalignment of Window Start Position

The window start position is computed by subtracting a fixed byte count, and may not
align with an instruction boundary.
For this reason, decode failures may occur near the beginning of the window; however,
since backward scan proceeds from the end of the instruction sequence
(immediately before the SYSCALL), the practical impact is small.

## 4. Syscall Number Resolution via Backward Scan

### 4.1 Overall Flow (backwardScanX86WithRegCopy)

```
backwardScanX86WithRegCopy
  ↓
  [1] backwardScanX86WithWindow (initial window)
       → result is determined or complete failure → return
       → if x86_copy_chain_unresolved:
           [2] resolveX86CopyChainTailConsensus (tail consensus search)
                → success → return
           [3] if decode failures occurred: search for alternative window start positions
                for candidateStart in [windowStart+1 .. windowStart+maxInstructionLength]:
                    backwardScanX86WithWindow(candidateStart)
                    → immediate → return
```

### 4.2 Single-window Scan (backwardScanX86WithWindow)

`scanX86SyscallRegInBlock` traverses the instruction sequence from the end and accumulates
results in `x86ScanResult`.

| Field | Meaning |
|---|---|
| `foundImmediate` | an immediate load instruction into RAX was found |
| `immediateValue` | the immediate value found |
| `sawRegCopy` | passed through a register copy (e.g., `MOV EAX, EDX`) |
| `hitControlBoundary` | reached a control flow instruction (JMP / CALL / RET, etc.) |
| `needPredResolution` | a control flow boundary was reached during register copy chain tracking |
| `hasCopyInstAddr` / `copyInstAddr` | address of the copy instruction |
| `indirectSetting` | indirect write detected (memory reference, indirect register, etc.) |
| `targetReg` | the register currently being tracked (updated on each copy) |

When a register copy (e.g., `MOV EAX, EDX`) is detected during scan, `targetReg` is
switched to the source register and tracking continues.

### 4.3 CFG-based Branch Convergence Analysis (resolveX86RegAcrossPreds)

When the result is `x86_copy_chain_unresolved` (i.e., a control flow boundary was reached
during register copy chain tracking), the following processing is performed.

1. `buildX86Successors` constructs a successor graph from the instruction sequence within the window.
2. `resolveX86RegAcrossPreds` executes forward dataflow analysis using the worklist algorithm.
3. The input state of each instruction is propagated as a map of register values; at branch
   points, states are merged by intersection (known if both sides have matching values, unknown otherwise).
4. Resolution succeeds if the input state at the virtual node (`virtualEnd`) of the SYSCALL
   instruction is known.

When the same value converges from multiple predecessor nodes, `DeterminationDetailX86BranchConverged`
is assigned.

### 4.4 Tail Consensus Search (resolveX86CopyChainTailConsensus)

`backwardScanX86WithWindow` is executed while shifting the candidate start position 1 byte at a
time within the range of 128 bytes immediately preceding the SYSCALL instruction; if all cases
that resolved as `immediate` return the same value, that value is adopted.
If any candidate yields a differing value, the search fails (to avoid misanalysis).

### 4.5 Validation of Valid Syscall Numbers

Even when an immediate value is obtained, values exceeding `maxValidSyscallNumber = 500` or
negative values are treated as `unknown:indirect_setting`.

## 5. RIP-relative Global Variable Resolution

### 5.1 Target Pattern

In Go's syscall package, there are cases where the syscall number is stored in a package-level
variable (e.g., `forkAndExecInChild1`, which references `syscall.fcntl64Syscall`).
This pattern generates the following instruction.

```asm
MOV RAX, [RIP + disp32]    ; load RAX via RIP-relative memory reference
SYSCALL
```

### 5.2 Behavior of ResolveFirstArgGlobal

`X86Decoder.ResolveFirstArgGlobal` is called during the backward scan of Pass 2 (Go wrapper
analysis) and resolves the value by the following steps.

1. Checks whether the instruction is in `MOV RAX/EAX, [RIP + disp32]` form.
2. Computes the effective address `nextPC + sign_extend(disp32)` via `x86RIPRelAddr`
   (because `x86asm` stores disp32 as zero-extended, it is reinterpreted as `int32`
   to recover the sign-extended value).
3. Reads the value at that address from the ELF sections registered via `SetDataSections`
   (`.noptrdata`, `.rodata`, `.data`), as a 4-byte or 8-byte little-endian integer.

## 6. Determination Methods and Detail Codes

### 6.1 DeterminationMethod Constants

| Constant | Meaning |
|---|---|
| `immediate` | determined directly from an immediate value |
| `go_wrapper` | determined from the argument of a Go wrapper call |
| `unknown:decode_failed` | unknown due to instruction decode failure |
| `unknown:control_flow_boundary` | unknown because a control flow boundary was reached |
| `unknown:indirect_setting` | unknown due to indirect setting (memory reference, indirect register, etc.) |
| `unknown:scan_limit_exceeded` | scan step limit reached (`defaultMaxBackwardScan = 50`) |
| `unknown:window_exhausted` | all instructions in the window were consumed without finding the value |
| `unknown:invalid_offset` | the SYSCALL instruction offset is invalid |

### 6.2 DeterminationDetail Constants (x86_64-specific)

| Constant | Meaning |
|---|---|
| `x86_copy_chain` | resolved via register copy chain |
| `x86_branch_converged` | resolved via CFG analysis at a branch convergence point |
| `x86_copy_chain_unresolved` | copy chain tracking ended without resolution |
| `x86_indirect_write` | indirect write detected (detail for `unknown:indirect_setting`) |

## 7. Design Rationale

### 7.1 Reason for Not Treating Decode Failures as High Risk

1. **The target of Pass 1 analysis is direct SYSCALL instructions**, and decode failures are
   unlikely to affect detection of the SYSCALL instructions themselves. The SYSCALL instruction
   is a fixed 2-byte sequence `0F 05`, and decode failures are unlikely to directly affect
   the detection accuracy of this 2-byte pattern.

2. **Cases where decode failures occur frequently are rare**, and excessive High Risk
   classifications would degrade practical usability. The `.text` section is normally composed
   of valid instructions, and decode failures are limited to special cases such as interleaved
   data regions or alignment mismatches.

3. **Decode failures in Pass 2 (Go wrapper analysis)** do not necessarily mean that syscall
   wrapper calls are missed. CALL instructions are typically decoded correctly, and it is
   unlikely that surrounding decode failures will affect resolution of CALL targets.

### 7.2 Consistency with the Fail-safe Design Principle

This design treats "syscall numbers that cannot be detected" as High Risk (FR-3.1.4).

- **Decode failure**: the case where the instruction itself cannot be recognized
- **Syscall number unknown**: the case where the SYSCALL instruction was recognized but the number cannot be identified

These two are problems of different nature and are treated distinctly.

- SYSCALL instruction decoded successfully but number unknown → **High Risk**
- Decode failure itself → **log output only** (does not affect risk classification)

Results with `unknown:*` method (SYSCALL instructions whose number is unknown) are treated
as High Risk.
The decode failure count does not affect risk classification.

### 7.3 Visualization of Decode Failures

Decode failures are collected as statistics in the `DecodeStatistics` struct and visualized
via the following log output.

- **Per-failure log**: output via `slog.Debug` in both Pass 1 (`findSyscallInstructions`) and
  Pass 2 (`FindWrapperCalls`). The number of log entries is limited to `MaxDecodeFailureLogs`
  (= 10) for each of Pass 1 and Pass 2.
- **Summary log**: the record command outputs, via `slog.Debug`, the file path, the total
  decode failure count (the sum of Pass 1 and Pass 2), and the number of bytes analyzed.

This enables investigation of binaries in which decode failures occur frequently, and allows
improvement of the analysis logic or manual verification of the target binary as needed.
