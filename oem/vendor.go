package oem

import (
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

// DetectVendor identifies known manufacturer names and OEM namespaces.
func DetectVendor(manufacturer string, oem *models.Oem) Vendor {
	value := strings.ToLower(manufacturer)
	variants := []struct {
		vendor   Vendor
		keywords []string
	}{
		{vendor: VendorEnginetech, keywords: []string{"enginetech", "\u5b89\u64ce"}}, {vendor: VendorInspur, keywords: []string{"inspur", "maginfra", "\u6d6a\u6f6e"}}, {vendor: VendorIEIT, keywords: []string{"ieit systems"}}, {vendor: VendorZTE, keywords: []string{"zte", "\u4e2d\u5174"}}, {vendor: VendorH3C, keywords: []string{"h3c", "\u65b0\u534e\u4e09"}}, {vendor: VendorNettrix, keywords: []string{"nettrix", "\u5b81\u7545"}}, {vendor: VendorXFusion, keywords: []string{"xfusion", "huawei", "\u8d85\u805a\u53d8"}}, {vendor: VendorLenovo, keywords: []string{"lenovo", "\u8054\u60f3"}}, {vendor: VendorSmoothcompute, keywords: []string{"smoothcompute", "\u987a\u7b97"}}, {vendor: VendorAMI, keywords: []string{"ami", "megarac"}},
	}
	for _, variant := range variants {
		for _, keyword := range variant.keywords {
			if strings.Contains(value, keyword) {
				return variant.vendor
			}
		}
	}
	if oem != nil {
		for key := range *oem {
			for _, variant := range variants {
				if strings.EqualFold(key, string(variant.vendor)) {
					return variant.vendor
				}
			}
		}
	}
	return VendorGeneric
}
