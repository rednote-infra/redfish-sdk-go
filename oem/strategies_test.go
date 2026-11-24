package oem_test

import (
	"encoding/json"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/client"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem"
)

func TestStrategiesRegisterWithClient(t *testing.T) {
	registry := client.NewProviderRegistry()
	if err := registry.RegisterExtractor(oem.VendorAMI, oem.NewExtractor(oem.VendorAMI)); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterUpdate(oem.VendorZTE, oem.NewUpdateStrategy(oem.VendorZTE)); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterLogCollect(oem.VendorH3C, oem.NewLogCollectStrategy(oem.VendorH3C)); err != nil {
		t.Fatal(err)
	}
	thermal := &models.Thermal{ODataID: models.Ptr("/thermal")}
	uri, err := registry.Extractor(oem.VendorAMI).InletHistoryURI(thermal)
	if err != nil || uri != "/thermal/Oem/Ami/InletHistoryTemperature" {
		t.Fatalf("extractor: %q %v", uri, err)
	}
	body, err := registry.Update(oem.VendorZTE).BuildBody(client.UpdateRequest{ImageURI: "https://images.invalid/firmware", PreserveConfig: models.Ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	public := body["Oem"].(models.Object)["Public"].(models.Object)
	if public["PreserveConf"] != false {
		t.Fatal("explicit false was lost")
	}
	body, err = registry.LogCollect(oem.VendorH3C).BuildBody(client.DiagnosticOptions{})
	if err != nil || body["Type"] != "URI" || body["Content"] != "/mnt/sdmmc0p1/sds/bmc/dump.tar.gz" {
		t.Fatalf("log policy: %v %v", body, err)
	}
}

func TestExtractorReadingsAreIndependent(t *testing.T) {
	reading := float64(27)
	fan := &models.Fan{SpeedPercent: &models.FanSpeedExcerpt{Reading: &reading}}
	ratio := oem.NewExtractor(oem.VendorGeneric).FanSpeedRatio(fan)
	if ratio == nil || *ratio != reading {
		t.Fatal("missing ratio")
	}
	*ratio = 99
	if reading != 27 {
		t.Fatal("extractor returned an aliased reading")
	}
}

func TestLogServiceSelectionValidatesAdvertisedActions(t *testing.T) {
	valid := models.Log{Actions: &models.LogActions{Extra: map[string]json.RawMessage{"#LogService.CollectDiagnosticData": json.RawMessage(`{"target":"/collect"}`)}}}
	provider := oem.NewLogCollectStrategy(oem.VendorGeneric)
	if _, err := provider.SelectService([]models.Log{valid}); err != nil {
		t.Fatal(err)
	}
	valid.Actions.Extra["#LogService.CollectDiagnosticData"] = json.RawMessage(`{"target":""}`)
	if _, err := provider.SelectService([]models.Log{valid}); err == nil {
		t.Fatal("empty action target was accepted")
	}
}

func TestIEITRetainsStandardUpdateAndReadingDefaults(t *testing.T) {
	body, err := oem.NewUpdateStrategy(oem.VendorIEIT).BuildBody(oem.UpdateRequest{ImageURI: "https://images.invalid/firmware", Username: "synthetic-user", PreserveConfig: models.Ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || body["ImageURI"] != "https://images.invalid/firmware" || body["TransferProtocol"] != "HTTP" {
		t.Fatalf("IEIT default changed: %v", body)
	}
	extensions := models.Oem{"Public": json.RawMessage(`{"temperature":42}`)}
	if value := oem.NewExtractor(oem.VendorIEIT).DriveTemperature(&models.Drive{OEM: &extensions}); value != nil {
		t.Fatal("IEIT must retain standard reading defaults")
	}
}
