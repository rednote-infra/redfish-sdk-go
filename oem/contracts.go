// Package oem implements vendor strategies independently of the HTTP client.
package oem

import "github.com/rednote-infra/redfish-sdk-go/models"

type Object = models.Object

type Vendor string

const (
	VendorGeneric       Vendor = "generic"
	VendorAMI           Vendor = "ami"
	VendorZTE           Vendor = "zte"
	VendorInspur        Vendor = "inspur"
	VendorIEIT          Vendor = "ieit"
	VendorH3C           Vendor = "h3c"
	VendorXFusion       Vendor = "xfusion"
	VendorLenovo        Vendor = "lenovo"
	VendorNettrix       Vendor = "nettrix"
	VendorEnginetech    Vendor = "enginetech"
	VendorSmoothcompute Vendor = "smoothcompute"
)

// Extractor isolates OEM readings and resource layouts from the standard APIs.
type Extractor interface {
	InletHistoryURI(*models.Thermal) (string, error)
	FanSpeedRatio(*models.Fan) *float64
	DriveTemperature(*models.Drive) *float64
}

type UpdateStrategy interface {
	BuildBody(UpdateRequest) (Object, error)
}

type LogCollectStrategy interface {
	BuildBody(DiagnosticOptions) (Object, error)
	SelectService([]models.Log) (*models.Log, error)
}

var (
	_ Extractor          = extractor{}
	_ UpdateStrategy     = updateStrategy{}
	_ LogCollectStrategy = logCollectStrategy{}
)
