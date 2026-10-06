package starlings

import "reflect"

// typeName is used only in panic messages, so the reflection cost is
// irrelevant.
func typeName(v any) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}
