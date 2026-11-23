package errors

import "fmt"

var (
	_ error         = (*Error)(nil)
	_ error         = (*TransportError)(nil)
	_ error         = (*MissingAttributeError)(nil)
	_ fmt.Formatter = (*Error)(nil)
)
