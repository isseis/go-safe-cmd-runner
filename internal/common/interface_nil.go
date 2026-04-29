package common

import "reflect"

// IsNilInterfaceValue reports whether v is nil, including typed-nil interface values.
//
// Example:
//   - var p *MyType = nil; IsNilInterfaceValue(p) == true
//   - var v any = p; IsNilInterfaceValue(v) == true
func IsNilInterfaceValue(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
