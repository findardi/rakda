// Package ptr holds the one pointer helper every domain service needs.
package ptr

// Deref returns *v, or the zero value when v is nil.
func Deref[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}
