package provider

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
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
	words := strings.FieldsFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if strings.Contains(value, "hewlett") || slices.Contains(words, string(VendorHPE)) || slices.Contains(words, "hp") {
		return VendorHPE
	}
	if slices.Contains(words, string(VendorDell)) {
		return VendorDell
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
			if strings.EqualFold(key, wire.OEMNamespaceHPE) || strings.EqualFold(key, wire.OEMNamespaceHP) {
				return VendorHPE
			}
			if strings.EqualFold(key, wire.OEMNamespaceDell) {
				return VendorDell
			}
			for _, variant := range variants {
				if strings.EqualFold(key, string(variant.vendor)) {
					return variant.vendor
				}
			}
		}
	}
	return VendorGeneric
}

// DetectThermalVendor selects the history-query policy from the resource's advertised OEM marker.
func DetectThermalVendor(thermal *models.Thermal) Vendor {
	if thermal.OEM != nil {
		var marker map[string]json.RawMessage
		if json.Unmarshal((*thermal.OEM)[wire.OEMNamespaceAMI], &marker) == nil && marker != nil {
			return VendorAMI
		}
	}
	return VendorGeneric
}
