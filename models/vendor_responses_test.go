package models

import (
	"encoding/json"
	"testing"
)

func TestVendorScalarAndLocationContracts(t *testing.T) {
	var server SyslogServer
	if err := json.Unmarshal([]byte(`{"MemberId":"1","Enabled":"Enabled","Port":"","LogType":["Audit"],"VendorExtension":7}`), &server); err != nil {
		t.Fatal(err)
	}
	if Value(server.MemberID) != 1 || !Value(server.Enabled) || server.Port != nil || server.Extra["VendorExtension"] == nil {
		t.Fatalf("scalar compatibility or extension retention failed: %+v", server)
	}
	var service SyslogService
	if err := json.Unmarshal([]byte(`{"ServiceEnabled":"Disabled"}`), &service); err != nil || Value(service.ServiceEnabled) {
		t.Fatalf("service switch: %v", err)
	}
	var drive Drive
	if err := json.Unmarshal([]byte(`{"Location":{"PartLocation":{"ServiceLabel":"Bay1"}},"LocationIndicatorActive":true}`), &drive); err != nil {
		t.Fatal(err)
	}
	if len(drive.Location) != 1 || Value(drive.IndicatorLed) != "Lit" {
		t.Fatalf("drive compatibility failed: %+v", drive)
	}
	var cpu Processor
	if err := json.Unmarshal([]byte(`{"ProcessorId":{"EffectiveFamily":6,"EffectiveModel":85,"Step":7},"Oem":{"BMC":{"SerialNumber":"cpu-example"}}}`), &cpu); err != nil {
		t.Fatal(err)
	}
	if Value(cpu.ProcessorID.EffectiveFamily) != "6" || Value(cpu.SerialNumber) != "cpu-example" {
		t.Fatalf("CPU compatibility failed: %+v", cpu)
	}
}

func TestVendorNormalizationPreservesExplicitValuesAndNulls(t *testing.T) {
	for _, payload := range []string{
		`{"SerialNumber":null,"Oem":{"BMC":{"SerialNumber":"fallback"}}}`,
		`{"SerialNumber":"","Oem":{"BMC":{"SerialNumber":"fallback"}}}`,
	} {
		var cpu Processor
		if err := json.Unmarshal([]byte(payload), &cpu); err != nil {
			t.Fatal(err)
		}
		if Value(cpu.SerialNumber) != "" {
			t.Fatal("explicit serial number must take precedence")
		}
	}
	var drive Drive
	if err := json.Unmarshal([]byte(`{"IndicatorLED":null,"LocationIndicatorActive":true}`), &drive); err != nil {
		t.Fatal(err)
	}
	if drive.IndicatorLed != nil {
		t.Fatal("explicit null must not be replaced")
	}
	var server SyslogServer
	if err := json.Unmarshal([]byte(`{"Enabled":"unknown","Port":null}`), &server); err != nil || server.Enabled != nil {
		t.Fatalf("unknown optional switch: %v", err)
	}
	data, err := json.Marshal(server)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	json.Unmarshal(data, &body)
	if string(body["Port"]) != "null" {
		t.Fatal("explicit null port was lost")
	}
}

func TestDerivedIndicatorDoesNotInventMutationFields(t *testing.T) {
	var drive Drive
	if err := json.Unmarshal([]byte(`{"LocationIndicatorActive":true}`), &drive); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(drive)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	json.Unmarshal(data, &body)
	if body["IndicatorLED"] != nil {
		t.Fatal("a derived compatibility view must not become a mutation field")
	}
	drive.IndicatorLed = Ptr("Off")
	data, err = json.Marshal(drive)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &body)
	if string(body["IndicatorLED"]) != `"Off"` {
		t.Fatal("an explicit legacy indicator assignment must remain writable")
	}
}

func TestSuppliedNormalizedScalarsRemainPresent(t *testing.T) {
	var server SyslogServer
	if err := json.Unmarshal([]byte(`{"Port":"","Enabled":"unknown"}`), &server); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(server)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	json.Unmarshal(data, &body)
	if string(body["Port"]) != "null" || string(body["Enabled"]) != "null" {
		t.Fatalf("normalization lost supplied-field presence: %s", data)
	}
}
