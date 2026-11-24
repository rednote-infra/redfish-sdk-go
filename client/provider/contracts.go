// Package provider defines resource strategies and standard behavior used by Client and OEM implementations.
package provider

import (
	"net/url"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

type Object = models.Object

type Vendor string

const (
	VendorHPE           Vendor = "hpe"
	VendorDell          Vendor = "dell"
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

type UpdateRequest struct {
	ImageURI           string
	TransferProtocol   string
	Targets            []string
	Username, Password string
	PreserveConfig     *bool
	Options            Object
	Parameters         *models.UpdateParameters
	Vendor             Vendor
}

type DiagnosticOptions struct {
	// DownloadQuery supplies query parameters for an advertised read-only diagnostic artifact.
	DownloadQuery                                               url.Values
	DiagnosticDataType, OEMDiagnosticDataType, LogID, ManagerID string
	OEMParameters                                               Object
}
