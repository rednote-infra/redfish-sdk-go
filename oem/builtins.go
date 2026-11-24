package oem

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/oem/ami"
	"github.com/rednote-infra/redfish-sdk-go/oem/enginetech"
	"github.com/rednote-infra/redfish-sdk-go/oem/h3c"
	"github.com/rednote-infra/redfish-sdk-go/oem/ieit"
	"github.com/rednote-infra/redfish-sdk-go/oem/inspur"
	"github.com/rednote-infra/redfish-sdk-go/oem/lenovo"
	"github.com/rednote-infra/redfish-sdk-go/oem/nettrix"
	"github.com/rednote-infra/redfish-sdk-go/oem/smoothcompute"
	"github.com/rednote-infra/redfish-sdk-go/oem/xfusion"
	"github.com/rednote-infra/redfish-sdk-go/oem/zte"
)

// NewExtractor selects the vendor implementation or the Client default.
func NewExtractor(vendor Vendor) Extractor {
	switch vendor {
	case VendorAMI:
		return ami.Extractor{}
	case VendorZTE:
		return zte.Extractor{}
	case VendorH3C:
		return h3c.Extractor{}
	case VendorXFusion:
		return xfusion.Extractor{}
	case VendorInspur:
		return inspur.Extractor{}
	case VendorLenovo:
		return lenovo.Extractor{}
	case VendorNettrix:
		return nettrix.Extractor{}
	}
	return provider.DefaultExtractor{}
}

// NewUpdateStrategy selects the vendor implementation or the Client default.
func NewUpdateStrategy(vendor Vendor) UpdateStrategy {
	switch vendor {
	case VendorInspur:
		return inspur.UpdateStrategy{}
	case VendorZTE:
		return zte.UpdateStrategy{}
	case VendorEnginetech:
		return enginetech.UpdateStrategy{}
	case VendorH3C:
		return h3c.UpdateStrategy{}
	case VendorNettrix:
		return nettrix.UpdateStrategy{}
	case VendorXFusion:
		return xfusion.UpdateStrategy{}
	case VendorLenovo:
		return lenovo.UpdateStrategy{}
	}
	return provider.DefaultUpdateStrategy{}
}

// NewLogCollectStrategy selects the vendor implementation or the Client default.
func NewLogCollectStrategy(vendor Vendor) LogCollectStrategy {
	switch vendor {
	case VendorZTE:
		return zte.LogCollectStrategy{}
	case VendorH3C:
		return h3c.LogCollectStrategy{}
	case VendorXFusion:
		return xfusion.LogCollectStrategy{}
	case VendorEnginetech:
		return enginetech.LogCollectStrategy{}
	case VendorInspur:
		return inspur.LogCollectStrategy{}
	case VendorIEIT:
		return ieit.LogCollectStrategy{}
	case VendorLenovo:
		return lenovo.LogCollectStrategy{}
	case VendorSmoothcompute:
		return smoothcompute.LogCollectStrategy{}
	}
	return provider.DefaultLogCollectStrategy{}
}
