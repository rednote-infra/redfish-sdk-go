package ami_test

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/ami"
	"testing"
)

func TestHistoryUsesVendorResourceWithoutChangingDefault(t *testing.T) {
	thermal := &models.Thermal{ODataID: models.Ptr("/thermal"), InletHistoryTemperature: &models.Link{ODataID: models.Ptr("/standard-history")}}
	var standard provider.Extractor = provider.DefaultExtractor{}
	var vendor provider.Extractor = ami.Extractor{}
	actual, err := vendor.InletHistoryURI(thermal)
	if err != nil || actual != "/thermal/Oem/Ami/InletHistoryTemperature" {
		t.Fatalf("AMI history: %q %v", actual, err)
	}
	actual, err = standard.InletHistoryURI(thermal)
	if err != nil || actual != "/standard-history" {
		t.Fatalf("default history: %q %v", actual, err)
	}
}
