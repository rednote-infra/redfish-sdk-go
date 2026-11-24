package errors

import "fmt"

// ProviderConflictError identifies duplicate explicit or discovered OEM providers.
type ProviderConflictError struct{ Category, Vendor string }

func (e *ProviderConflictError) Error() string {
	return fmt.Sprintf("Duplicate OEM provider: %s/%s", e.Category, e.Vendor)
}

// ProviderLoadError retains the selected factory's original failure.
type ProviderLoadError struct {
	Category, Vendor string
	Cause            error
}

func (e *ProviderLoadError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return fmt.Sprintf("Cannot load OEM provider: %s/%s", e.Category, e.Vendor)
}

func (e *ProviderLoadError) Unwrap() error { return e.Cause }

var (
	_ error = (*ProviderConflictError)(nil)
	_ error = (*ProviderLoadError)(nil)
)
