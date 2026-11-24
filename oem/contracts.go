// Package oem retains constructors for vendor resource implementations.
// Interfaces and standard behavior are defined in client/provider.
package oem

import "github.com/rednote-infra/redfish-sdk-go/client/provider"

type Object = provider.Object
type Vendor = provider.Vendor
type Extractor = provider.Extractor
type UpdateStrategy = provider.UpdateStrategy
type LogCollectStrategy = provider.LogCollectStrategy
type UpdateRequest = provider.UpdateRequest
type DiagnosticOptions = provider.DiagnosticOptions

const (
	VendorGeneric       = provider.VendorGeneric
	VendorAMI           = provider.VendorAMI
	VendorZTE           = provider.VendorZTE
	VendorInspur        = provider.VendorInspur
	VendorIEIT          = provider.VendorIEIT
	VendorH3C           = provider.VendorH3C
	VendorXFusion       = provider.VendorXFusion
	VendorLenovo        = provider.VendorLenovo
	VendorNettrix       = provider.VendorNettrix
	VendorEnginetech    = provider.VendorEnginetech
	VendorSmoothcompute = provider.VendorSmoothcompute
)
