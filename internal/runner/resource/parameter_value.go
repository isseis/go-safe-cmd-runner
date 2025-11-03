package resource

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
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
	case map[string]any:
		// Check if it's an environment map (all values are strings)
		envMap := make(map[string]string)
		allStrings := true
		for k, v := range val {
			if str, ok := v.(string); ok {
				envMap[k] = str
			} else {
				allStrings = false
				break
			}
		}
		if allStrings && len(envMap) > 0 {
			return NewEnvironmentValue(envMap)
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

// DurationValue represents a time.Duration parameter value
type DurationValue struct {
	value time.Duration
}

// NewDurationValue creates a new duration parameter value
func NewDurationValue(v time.Duration) ParameterValue {
	return DurationValue{value: v}
}

func (d DurationValue) String() string {
	return fmt.Sprintf("%v", d.value)
}

// Value implements ParameterValue interface
func (d DurationValue) Value() any {
	return d.value
}

// MarshalJSON implements json.Marshaler interface
func (d DurationValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.value)
}

// EnvironmentValue represents an environment variable map with control character escaping
type EnvironmentValue struct {
	value map[string]string
}

// NewEnvironmentValue creates a new environment parameter value
func NewEnvironmentValue(v map[string]string) ParameterValue {
	return EnvironmentValue{value: v}
}

func (e EnvironmentValue) String() string {
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
		fmt.Fprintf(&result, "%s=%s", k, escapeControlChars(e.value[k]))
	}
	return result.String()
}

// Value implements ParameterValue interface
func (e EnvironmentValue) Value() any {
	return e.value
}

// MarshalJSON implements json.Marshaler interface
func (e EnvironmentValue) MarshalJSON() ([]byte, error) {
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

// escapeControlChars escapes control characters in a string for safe display.
// This ensures terminal control characters don't corrupt the output.
//
// Uses unicode.IsControl to detect control characters, then escapes them using
// standard escape sequences (\n, \t, etc.) for common ones, or \xNN for others.
// Regular printable characters are left unchanged for readability.
func escapeControlChars(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r != ' ' && !unicode.IsControl(r) {
			// Not a space or a control character - output as-is
			result.WriteRune(r)
			continue
		}

		// Use standard escape sequences for common control characters
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\b':
			result.WriteString("\\b")
		case '\f':
			result.WriteString("\\f")
		case '\v':
			result.WriteString("\\v")
		case '\a':
			result.WriteString("\\a")
		default:
			// For space character and other control characters, use \xNN notation
			fmt.Fprintf(&result, "\\x%02x", r)
		}
	}
	return result.String()
}
