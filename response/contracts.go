package response

import "fmt"

var (
	_ fmt.Stringer  = (*Response)(nil)
	_ fmt.Formatter = (*Response)(nil)
)
