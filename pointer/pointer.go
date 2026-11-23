package pointer

// Ptr returns a pointer to value.
func Ptr[T any](value T) *T { return &value }

// Value returns the pointed-to value, or its zero value for nil.
func Value[T any](value *T) (zero T) {
	if value != nil {
		return *value
	}
	return zero
}
