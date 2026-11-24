package wire

const (
	DefaultServiceRoot = "/redfish/v1/"
	HeaderAuthToken    = "X-Auth-Token"
	HeaderIfMatch      = "If-Match"
	HeaderETag         = "ETag"
	HeaderLocation     = "Location"
	HeaderRetryAfter   = "Retry-After"
	HeaderODataVersion = "OData-Version"
)

const ODataVersion = "4.0"

const (
	FieldBootTarget  = "BootSourceOverrideTarget"
	FieldBootMode    = "BootSourceOverrideMode"
	FieldBootEnabled = "BootSourceOverrideEnabled"
	FieldImage       = "Image"
	FieldInserted    = "Inserted"
)
