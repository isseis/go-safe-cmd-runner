# Slice Type Conversion Behavior in Redaction

## Overview

The `RedactingHandler.processSlice()` method in the `internal/redaction` package converts all typed slices (`[]string`, `[]int`, `[]MyStruct`, etc.) to `[]any`. This document explains the reason for this behavior, its impact, and the validity of this design.

## Current Behavior

### Type Conversion Details

```go
// Input: typed slice
stringSlice := []string{"alice", "bob", "charlie"}
attr := slog.Any("users", stringSlice)

// After processSlice processing
// Output: converted to []any
// attr.Value.Any().([]string) → fails
// attr.Value.Any().([]any)    → succeeds
```

### Implementation Points

For implementation details, see [the processSlice function in redactor.go](../../internal/redaction/redactor.go).

1. **All slices are processed**: The `processKindAny()` function ensures that all slices go through `processSlice()`, regardless of whether they contain LogValuer elements

2. **Creation of new []any slice**: A new slice of type `[]any` is created to store processed elements

3. **Element processing**:
   - LogValuer elements: Call `LogValue()` to resolve and apply redaction recursively
   - Non-LogValuer elements: Preserve as-is

4. **Return value**: Returns as `[]any` in `slog.AnyValue(processedElements)`

## Impact

### Affected Cases

1. **Type assertion failure**:
   ```go
   // Before processing
   value.Any().([]string) // succeeds

   // After processing
   value.Any().([]string) // fails
   value.Any().([]any)    // succeeds
   ```

2. **Type information loss**:
   - Information about the original type (`[]string`, `[]int`, etc.) is lost
   - The concrete type of slice elements is unified to `[]any`

### Unaffected Cases

1. **Log output**: JSON handlers and text handlers serialize regardless of slice element types, so output results are unaffected

2. **Semantic content**: All actual values of the slice are preserved

3. **Non-slice values**: Other types (string, int, bool, etc.) preserve their original types

## Alternative Considerations

### Option 1: Type Preservation Using Reflection

**Overview**: Use `reflect.MakeSlice()` to create a slice of the original type

**Advantages**:
- Preserves the original slice type
- Type assertions function

**Disadvantages**:
- Complex implementation
- Reflection performance overhead
- Requires type mismatch edge case handling
- High maintenance cost

### Option 2: Process Only When LogValuer Elements Exist

**Overview**: Return the original slice as-is when no LogValuer elements exist

**Advantages**:
- Preserves type in some cases
- Performance improvement

**Disadvantages**:
- Inconsistent behavior (behavior changes depending on presence of LogValuer)
- Complex testing
- Difficult to predict

### Option 3: Maintain Current Implementation (Recommended)

**Reason**:
1. **Characteristics as a logging system**: This system is for log output, and type information preservation is not important
2. **Simplicity**: Implementation is simple and easy to understand
3. **Consistency**: All slices are processed in the same way
4. **Performance**: Minimum overhead as it does not use Reflection
5. **Practicality**: Type assertions are rare in actual use cases

## Design Validity

### Why This Design Is Appropriate

1. **Purpose**: This is a logging system, and preservation of semantic content is more important than type preservation

2. **Handler implementation**: Standard slog handlers (JSON, text) do not require type information

3. **Security**: The primary purpose of redaction is protection of sensitive data; type preservation is secondary

4. **Maintainability**: Simple implementation reduces long-term maintenance cost

### Recommended Usage

```go
// ✓ Good example: use in log output
logger.Info("Users list", "users", slog.AnyValue(stringSlice))

// ✗ Bad example: dependence on type assertion
sliceValue := attr.Value.Any().([]string) // fails after redaction

// ✓ Good example: generic processing
sliceValue := attr.Value.Any().([]any)
for _, elem := range sliceValue {
    // process each element
}
```

## Testing

The type conversion behavior is verified in [TestRedactingHandler_SliceTypeConversion in redactor_test.go](../../internal/redaction/redactor_test.go).

Test cases:
1. Typed slice without LogValuer elements → converted to `[]any`
2. Slice with LogValuer elements → converted to `[]any`
3. Mixed-type slice → converted to `[]any`, semantic content is preserved

## Conclusion

The current `[]any` conversion is an appropriate design decision for the purpose as a logging system. Type information is lost, but all semantic content is preserved, and the implementation is simple and maintainable.

Technical methods to implement type preservation exist, but the complexity and overhead exceed the benefits in a logging system.

## Related Project

### CommandResults Type Safety Improvement Project

A project is in progress to fundamentally resolve the type conversion problem in log processing of `[]CommandResult`.

**Problem:**
- Slice type conversion by RedactingHandler requires complex type assertion in `extractCommandResults`
- Lack of type safety and performance overhead

**Solution:**
- Introduce `CommandResults` type that implements LogValuer for the entire slice
- Avoid slice type conversion using Group structure

**Details:**
- [Project requirements document](../tasks/0056_command_results_type_safety/01_requirements.md)
- [Architecture design](../tasks/0056_command_results_type_safety/02_architecture.md)

This project resolves the type conversion problem explained in this document for a specific use case (CommandResults).

## Resolution (Implemented)

Completed through Phase 3 of Task 0056 and reflected the following measures using CommandResults in production code.

1. **Introduction of CommandResults type**: Implemented LogValuer as an alias type of `[]CommandResult` to provide structured output for the entire slice.
2. **Update of Runner/SlackHandler**: Pass `CommandResults` at log occurrence points such as `logGroupExecutionSummary`, and the SlackHandler side processes only Group values.
3. **Expansion of Redaction/E2E testing**: Verified with new integration tests that Group structure is maintained even through RedactingHandler and that sensitive data is redacted end-to-end.

For details, refer to the deliverables of Task 0056:
- [Implementation plan](../tasks/0056_command_results_type_safety/04_implementation_plan.md)
- [Design/specification](../tasks/0056_command_results_type_safety/02_architecture.md)

## Reference

- Implementation: [internal/redaction/redactor.go](../../internal/redaction/redactor.go)
  - `processSlice` function: Implementation of slice processing
  - `processKindAny` function: Processing of slog.KindAny values
  - `processLogValuer` function: Processing of LogValuer elements
- Testing: [internal/redaction/redactor_test.go](../../internal/redaction/redactor_test.go)
  - `TestRedactingHandler_SliceTypeConversion`: Verification of type conversion behavior
