package hpe_test

import (
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/hpe"
)

func TestPercentFanReadingAndFirmware(t *testing.T) {
	fan := &models.Fan{Reading: models.Ptr(int64(28)), ReadingUnits: models.Ptr("Percent")}
	if value := (hpe.Extractor{}).FanSpeedRatio(fan); value == nil || *value != 28 {
		t.Fatalf("fan percent: %v", value)
	}
	body, err := (hpe.UpdateStrategy{}).BuildBody(provider.UpdateRequest{ImageURI: "https://images.invalid/fw.bin", Options: provider.Object{"TPMOverrideFlag": false}})
	if err != nil || body["TPMOverrideFlag"] != false || body["ImageURI"] != "https://images.invalid/fw.bin" {
		t.Fatalf("firmware: %v %v", body, err)
	}
}
