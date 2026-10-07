package starlings

// Ref returns a pointer to value. It keeps optional request fields readable:
//
//	ModifyMember{Nick: starlings.Ref("new name")}
//
// Use nil when the field should be omitted. APIs that support an explicit JSON
// null provide a named Clear flag or helper so callers do not need **T.
func Ref[T any](value T) *T { return &value }
