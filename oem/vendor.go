package oem

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func DetectVendor(manufacturer string, extensions *models.Oem) Vendor {
	return provider.DetectVendor(manufacturer, extensions)
}
