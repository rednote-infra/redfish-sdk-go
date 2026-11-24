package client

import "fmt"

var (
	_ fmt.Stringer              = (*Client)(nil)
	_ fmt.Formatter             = (*Client)(nil)
	_ LogCollectStrategy        = vendorLogCollect{}
	_ DiagnosticLifecycle       = vendorLogCollect{}
	_ DiagnosticBytesDownloader = vendorLogCollect{}
)
