package client

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

func TestUpdateWireBodiesMatchPythonSDK(t *testing.T) {
	data, err := os.ReadFile("../testdata/python_update_bodies.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Vendor   Vendor `json:"vendor"`
		Preserve *bool  `json:"preserve_config"`
		Expected Object `json:"expected"`
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(string(test.Vendor), func(t *testing.T) {
			body, err := NewProviderRegistry().Update(test.Vendor).BuildBody(UpdateRequest{ImageURI: "https://images.invalid/fw.bin", TransferProtocol: "HTTPS", Targets: []string{"/components/bmc"}, PreserveConfig: test.Preserve})
			if err != nil {
				t.Fatal(err)
			}
			if !jsonEqual(body, test.Expected) {
				t.Fatalf("wire contract differs from Python:\nGo: %v\nPython: %v", body, test.Expected)
			}
		})
	}
}

func TestExtractorsMatchObservedLayouts(t *testing.T) {
	oem := models.Oem{"Public": json.RawMessage(`{"SpeedRatio":33,"temperature":42,"TemperatureCelsius":41}`), "xFusion": json.RawMessage(`{"SpeedRatio":35,"TemperatureCelsius":44}`), "gOemCustomeString": json.RawMessage(`{"SpeedRatio":0}`)}
	fan := &models.Fan{OEM: &oem, Reading: models.Ptr(int64(3000)), MaxReadingRange: models.Ptr(int64(6000))}
	drive := &models.Drive{OEM: &oem, DiskTemperatureCelsius: models.Ptr(float64(45))}
	cases := []struct {
		vendor             Vendor
		ratio, temperature *float64
	}{{vendor: VendorGeneric, ratio: models.Ptr(float64(50)), temperature: nil}, {vendor: VendorInspur, ratio: models.Ptr(float64(33)), temperature: models.Ptr(float64(42))}, {vendor: VendorH3C, ratio: models.Ptr(float64(33)), temperature: models.Ptr(float64(41))}, {vendor: VendorXFusion, ratio: models.Ptr(float64(35)), temperature: models.Ptr(float64(44))}, {vendor: VendorLenovo, ratio: models.Ptr(float64(50)), temperature: models.Ptr(float64(42))}, {vendor: VendorNettrix, ratio: models.Ptr(float64(33)), temperature: models.Ptr(float64(45))}}
	for _, test := range cases {
		t.Run(string(test.vendor), func(t *testing.T) {
			provider := NewProviderRegistry().Extractor(test.vendor)
			if actual := provider.FanSpeedRatio(fan); actual == nil || *actual != *test.ratio {
				t.Fatalf("wrong fan ratio: %v", actual)
			}
			actual := provider.DriveTemperature(drive)
			if (actual == nil) != (test.temperature == nil) || (actual != nil && *actual != *test.temperature) {
				t.Fatalf("wrong drive temperature: %v", actual)
			}
		})
	}
}

func TestProviderRegistriesAreClientScoped(t *testing.T) {
	first := NewProviderRegistry()
	second := first.Clone()
	if err := first.RegisterExtractor(VendorGeneric, vendorExtractor{vendor: VendorAMI}); err != nil {
		t.Fatal(err)
	}
	thermal := &models.Thermal{ODataID: models.Ptr("/thermal"), InletHistoryTemperature: &models.Link{ODataID: models.Ptr("/legacy")}}
	uri, err := second.Extractor(VendorGeneric).InletHistoryURI(thermal)
	if err != nil || uri != "/legacy" {
		t.Fatal("provider registration leaked into another registry")
	}
}
