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

const (
	DiagnosticAllLogs = "AllLogs"
	H3CDiagnosticPath = "/mnt/sdmmc0p1/sds/bmc/dump.tar.gz"
)

const (
	FieldSpeedRatio         = "SpeedRatio"
	FieldTemperatureCelsius = "TemperatureCelsius"
	FieldLegacyTemperature  = "temperature"
)

const ReadingUnitsPercent = "Percent"

const (
	OEMNamespaceHPE  = "Hpe"
	OEMNamespaceHP   = "Hp"
	OEMNamespaceDell = "Dell"
)
