# Slice Type Conversion in Redaction

## Overview

The `RedactingHandler.processSlice()` method in the `internal/redaction` package converts all typed slices (`[]string`, `[]int`, `[]MyStruct`, etc.) to `[]any`. This document explains the reason for this behavior, its impact, and the recommended pattern to avoid slice type conversion.

## Slice Type Conversion Behavior

### Basic Type Conversion

RedactingHandler converts all slices to `[]any`:

```go
// Input: typed slice
stringSlice := []string{"alice", "bob", "charlie"}
attr := slog.Any("users", stringSlice)

// After processSlice processing
// Output: converted to []any
attr.Value.Any().([]string) // fails
attr.Value.Any().([]any)    // succeeds
```

### Processing Flow

1. **All slices are processed**: All slices go through `processSlice()` regardless of whether they contain LogValuer elements
2. **Create new []any slice**: A new slice of type `[]any` is created to store processed elements
3. **Process elements**:
   - LogValuer elements: Call `LogValue()` to resolve and apply redaction recursively
   - Non-LogValuer elements: Preserve as-is
4. **Return value**: Return as `slog.AnyValue(processedElements)` with type `[]any`

### Impact Scope

**Affected**:
- Type assertion: `value.Any().([]string)` fails, `value.Any().([]any)` is required
- Type information: Original type information (`[]string`, `[]int`, etc.) is lost

**Not affected**:
- Log output: Output results of JSON handler or text handler
- Semantic content: Actual values in the slice are all preserved
- Non-slice values: string, int, bool, etc. preserve original types

## Rationale for Design Decision

This design is considered appropriate for the following reasons:

1. **Purpose**: In logging systems, preservation of semantic content is more important than type preservation
2. **Handler implementation**: Standard slog handlers (JSON, text) do not require type information
3. **Simplicity**: Implementation is simple and easy to understand
4. **Consistency**: All slices are processed in the same way
5. **Performance**: Minimum overhead because reflection is not used

Type preservation implementation (using reflection) is technically possible, but complexity and overhead outweigh the benefits.

## Recommended Pattern When Type-Safe Processing Is Required

To avoid slice type conversion and achieve type-safe processing, the **wrapper type with LogValuer implementation using Group structure** pattern is recommended.

### CommandResults Implementation Example

For `[]CommandResult` slice processing, the following approach is adopted:

```go
// Type definition
type CommandResults []CommandResult

// LogValuer implementation: Structure entire slice using Group structure
func (c CommandResults) LogValue() slog.Value {
    attrs := make([]slog.Attr, len(c))
    for i, result := range c {
        attrs[i] = slog.Any(strconv.Itoa(i), result)
    }
    return slog.GroupValue(attrs...)
}
```

### Pattern Advantages

1. **Type safety**: Group structure is preserved even after going through RedactingHandler
2. **No type assertion required**: SlackHandler side can process directly as Group value
3. **Performance**: No complex type assertion or reflection required
4. **Consistency**: Structure does not change before and after redaction

### Usage Example

```go
// Log recording side (Runner)
results := runnertypes.CommandResults{
    {Command: "echo test", ExitCode: 0},
    {Command: "false", ExitCode: 1},
}
logger.Info("Execution summary", "results", results)

// Log processing side (SlackHandler)
// Can process directly as Group, no type assertion required
func (h *SlackHandler) Handle(ctx context.Context, record slog.Record) error {
    record.Attrs(func(attr slog.Attr) bool {
        if attr.Key == "results" && attr.Value.Kind() == slog.KindGroup {
            // Process directly as Group structure
            for _, a := range attr.Value.Group() {
                // Process each CommandResult
            }
        }
        return true
    })
}
```

## Testing

Type conversion behavior verification:
- [TestRedactingHandler_SliceTypeConversion in redactor_test.go](../../internal/redaction/redactor_test.go)

CommandResults pattern integration tests:
- Group structure preservation through RedactingHandler
- Sensitive data redaction in end-to-end

For details, see Task 0056 deliverables:
- [Architecture Design](../tasks/0056_command_results_type_safety/02_architecture.md)
- [Implementation Plan](../tasks/0056_command_results_type_safety/04_implementation_plan.md)

## References

- Implementation: [internal/redaction/redactor.go](../../internal/redaction/redactor.go)
  - `processSlice` function: Slice processing implementation
  - `processKindAny` function: slog.KindAny value processing
  - `processLogValuer` function: LogValuer element processing
- Tests: [internal/redaction/redactor_test.go](../../internal/redaction/redactor_test.go)
  - `TestRedactingHandler_SliceTypeConversion`: Type conversion behavior verification
