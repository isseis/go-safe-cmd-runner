package resource

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// ParametersMap is a custom type for parameters that handles JSON marshaling/unmarshaling
type ParametersMap map[string]ParameterValue

// MarshalJSON implements JSON marshaling for ParametersMap
func (p ParametersMap) MarshalJSON() ([]byte, error) {
	// Convert to map[string]any using Value() method
	m := make(map[string]any, len(p))
	for k, v := range p {
		m[k] = v.Value()
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements JSON unmarshaling for ParametersMap
func (p *ParametersMap) UnmarshalJSON(data []byte) error {
	// First unmarshal to map[string]any
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	// Convert each value to appropriate ParameterValue type
	*p = make(ParametersMap, len(m))
	for k, v := range m {
		(*p)[k] = anyToParameterValue(v)
	}
	return nil
}

// anyToParameterValue converts an any value to the appropriate ParameterValue type
func anyToParameterValue(v any) ParameterValue {
	switch val := v.(type) {
	case string:
		return NewStringValue(val)
	case bool:
		return NewBoolValue(val)
	case float64:
		// JSON numbers are always float64
		// Check if it's actually an integer
		if val == float64(int64(val)) {
			return NewIntValue(int64(val))
		}
		return NewFloatValue(val)
	case int:
		return NewIntValue(int64(val))
	case int64:
		return NewIntValue(val)
	case []any:
		// Check if it's a string slice (all values are strings)
		strSlice := make([]string, 0, len(val))
		allStrings := true
		for _, v := range val {
			if str, ok := v.(string); ok {
				strSlice = append(strSlice, str)
			} else {
				allStrings = false
				break
			}
		}
		if allStrings {
			return NewStringSliceValue(strSlice)
		}
		// Otherwise treat as generic any value
		return NewAnyValue(val)
	case map[string]any:
		// Check if it's a string map (all values are strings)
		strMap := make(map[string]string)
		allStrings := true
		for k, v := range val {
			if str, ok := v.(string); ok {
				strMap[k] = str
			} else {
				allStrings = false
				break
			}
		}
		if allStrings {
			return NewStringMapValue(strMap)
		}
		// Otherwise treat as generic any value
		return NewAnyValue(val)
	default:
		// Fallback to AnyValue for unknown types
		return NewAnyValue(v)
	}
}

// ParameterValue is an interface for type-safe parameter values with custom string formatting
type ParameterValue interface {
	// String returns a human-readable string representation of the value
	// with appropriate escaping for control characters if needed
	String() string

	// Value returns the underlying value for JSON marshaling
	Value() any
}

// StringValue represents a string parameter value
type StringValue struct {
	value string
}

// NewStringValue creates a new string parameter value
func NewStringValue(v string) ParameterValue {
	return StringValue{value: v}
}

func (s StringValue) String() string {
	return s.value
}

// Value implements ParameterValue interface
func (s StringValue) Value() any {
	return s.value
}

// MarshalJSON implements json.Marshaler interface
func (s StringValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.value)
}

// BoolValue represents a boolean parameter value
type BoolValue struct {
	value bool
}

// NewBoolValue creates a new boolean parameter value
func NewBoolValue(v bool) ParameterValue {
	return BoolValue{value: v}
}

func (b BoolValue) String() string {
	return fmt.Sprintf("%v", b.value)
}

// Value implements ParameterValue interface
func (b BoolValue) Value() any {
	return b.value
}

// MarshalJSON implements json.Marshaler interface
func (b BoolValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.value)
}

// IntValue represents an integer parameter value
type IntValue struct {
	value int64
}

// NewIntValue creates a new integer parameter value
func NewIntValue(v int64) ParameterValue {
	return IntValue{value: v}
}

func (i IntValue) String() string {
	return fmt.Sprintf("%d", i.value)
}

// Value implements ParameterValue interface
func (i IntValue) Value() any {
	return i.value
}

// MarshalJSON implements json.Marshaler interface
func (i IntValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.value)
}

// FloatValue represents a float parameter value (used for timeout/duration)
type FloatValue struct {
	value float64
}

// NewFloatValue creates a new float parameter value
func NewFloatValue(v float64) ParameterValue {
	return FloatValue{value: v}
}

func (f FloatValue) String() string {
	return fmt.Sprintf("%v", f.value)
}

// Value implements ParameterValue interface
func (f FloatValue) Value() any {
	return f.value
}

// MarshalJSON implements json.Marshaler interface
func (f FloatValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.value)
}

// StringSliceValue represents a string slice parameter value
type StringSliceValue struct {
	value []string
}

// NewStringSliceValue creates a new string slice parameter value
func NewStringSliceValue(v []string) ParameterValue {
	return StringSliceValue{value: v}
}

func (s StringSliceValue) String() string {
	if len(s.value) == 0 {
		return "[]"
	}

	// Format as JSON array for readability
	var result strings.Builder
	result.WriteString("[")
	for i, v := range s.value {
		if i > 0 {
			result.WriteString(", ")
		}
		// Quote and escape each argument
		result.WriteString(fmt.Sprintf("%q", v))
	}
	result.WriteString("]")
	return result.String()
}

// Value implements ParameterValue interface
func (s StringSliceValue) Value() any {
	return s.value
}

// MarshalJSON implements json.Marshaler interface
func (s StringSliceValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.value)
}

// StringMapValue represents a variable map with control character escaping
type StringMapValue struct {
	value map[string]string
}

// NewStringMapValue creates a new string map parameter value
func NewStringMapValue(v map[string]string) ParameterValue {
	return StringMapValue{value: v}
}

func (e StringMapValue) String() string {
	if len(e.value) == 0 {
		return ""
	}

	// Sort keys for stable output
	keys := make([]string, 0, len(e.value))
	for k := range e.value {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var result strings.Builder
	first := true
	for _, k := range keys {
		if !first {
			result.WriteString(" ")
		}
		first = false
		fmt.Fprintf(&result, "%s=%s", k, common.EscapeControlChars(e.value[k]))
	}
	return result.String()
}

// Value implements ParameterValue interface
func (e StringMapValue) Value() any {
	return e.value
}

// MarshalJSON implements json.Marshaler interface
func (e StringMapValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.value)
}

// AnyValue represents an arbitrary parameter value (fallback for complex types)
type AnyValue struct {
	value any
}

// NewAnyValue creates a new any parameter value
func NewAnyValue(v any) ParameterValue {
	return AnyValue{value: v}
}

func (a AnyValue) String() string {
	return fmt.Sprintf("%v", a.value)
}

// Value implements ParameterValue interface
func (a AnyValue) Value() any {
	return a.value
}

// MarshalJSON implements json.Marshaler interface
func (a AnyValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.value)
}
