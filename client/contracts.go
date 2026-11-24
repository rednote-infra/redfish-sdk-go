package client

import "fmt"

var (
	_ fmt.Stringer              = (*Client)(nil)
	_ fmt.Formatter             = (*Client)(nil)
	_ Extractor                 = vendorExtractor{}
	_ UpdateStrategy            = vendorUpdate{}
	_ LogCollectStrategy        = vendorLogCollect{}
	_ DiagnosticLifecycle       = vendorLogCollect{}
	_ DiagnosticBytesDownloader = vendorLogCollect{}
)
