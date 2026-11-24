package models

import (
	"encoding/json"
	"testing"
)

func TestNumericETagExact(t *testing.T) {
	for _, raw := range []string{`87422082150`, `18446744073709551615`} {
		var model System
		if err := json.Unmarshal([]byte(`{"@odata.etag":`+raw+`}`), &model); err != nil {
			t.Fatal(err)
		}
		if Value(model.ODataETag) != raw {
			t.Fatalf("etag: %v", model.ODataETag)
		}
	}
}

func TestRegistryLargeIntegerRoundTrip(t *testing.T) {
	var model AttributeDefinition
	raw := []byte(`{"AttributeName":"Counter","Type":"Integer","LowerBound":0,"UpperBound":18446744073709551615,"ScalarIncrement":1}`)
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	if model.ExactUpperBound().String() != "18446744073709551615" {
		t.Fatal("lost upper bound precision")
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	var again AttributeDefinition
	if err := json.Unmarshal(encoded, &again); err != nil {
		t.Fatal(err)
	}
	if again.ExactUpperBound().Cmp(model.ExactUpperBound()) != 0 {
		t.Fatal("round trip changed bound")
	}
}

func TestSupermicroSyslogTypedServers(t *testing.T) {
	var model SyslogService
	if err := json.Unmarshal([]byte(`{"EnableSyslog":true,"SyslogServer":[{"ServerIP":"logs.example.invalid","Port":514},{"ServerIP":null,"Port":null}]}`), &model); err != nil {
		t.Fatal(err)
	}
	if !Value(model.ServiceEnabled) || len(model.SyslogServers) != 2 || Value(model.SyslogServers[0].Address) != "logs.example.invalid" || !model.SyslogServers[1].NullFields["Address"] {
		t.Fatalf("normalization failed: %+v", model)
	}
}

func TestStorageOperationVariants(t *testing.T) {
	var drive Drive
	if err := json.Unmarshal([]byte(`{"Operations":[{"OperationName":"Rebuild","PercentageComplete":12,"AssociatedTask":[]},{"AssociatedTask":{"@odata.id":"/tasks/1"}}]}`), &drive); err != nil {
		t.Fatal(err)
	}
	if len(drive.Operations) != 2 || drive.Operations[0].AssociatedTask != nil || Value(drive.Operations[1].AssociatedTask.ODataID) != "/tasks/1" {
		t.Fatal("operation task links lost")
	}
	var controller StorageController
	if err := json.Unmarshal([]byte(`{"PCIeInterface":[]}`), &controller); err != nil {
		t.Fatal(err)
	}
	if controller.PCIeInterface != nil {
		t.Fatal("empty array should represent missing PCIe interface")
	}
	if err := json.Unmarshal([]byte(`{"PCIeInterface":[{"MaxPCIeType":"Gen4"}]}`), &controller); err == nil {
		t.Fatal("nonempty array must not be silently discarded")
	}
}

func TestPropertyAnnotations(t *testing.T) {
	var system System
	if err := json.Unmarshal([]byte(`{"PowerState@Message.ExtendedInfo":[{"Message":"temporarily unavailable"}],"PowerState@Redfish.AllowableValues":["On","Off"]}`), &system); err != nil {
		t.Fatal(err)
	}
	messages, err := PropertyExtendedInfo(system.Extra, "PowerState")
	if err != nil || len(messages) != 1 || Value(messages[0].Message) != "temporarily unavailable" {
		t.Fatalf("messages: %v %v", messages, err)
	}
	values, err := PropertyAllowableValues[string](system.Extra, "PowerState")
	if err != nil || len(values) != 2 || values[1] != "Off" {
		t.Fatalf("values: %v %v", values, err)
	}
}

func TestSupermicroRoundTripUsesOriginalWireShape(t *testing.T) {
	for _, raw := range []string{
		`{"EnableSyslog":true,"SyslogServer":[{"ServerIP":"old.invalid","Port":514},{"ServerIP":null,"Port":null}]}`,
		`{"EnableSyslog":true,"SyslogServer":"old.invalid","SyslogPortNumber":514}`,
	} {
		var model SyslogService
		if err := json.Unmarshal([]byte(raw), &model); err != nil {
			t.Fatal(err)
		}
		model.SyslogServers[0].Address = Ptr("new.invalid")
		data, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(data, &fields)
		if _, ok := fields["SyslogServers"]; ok {
			t.Fatalf("invented standard key: %s", data)
		}
		if _, ok := fields["ServiceEnabled"]; ok {
			t.Fatalf("invented enabled key: %s", data)
		}
		var again SyslogService
		if err := json.Unmarshal(data, &again); err != nil || Value(again.SyslogServers[0].Address) != "new.invalid" {
			t.Fatalf("round trip: %s %v", data, err)
		}
	}
}

func TestStorageVariantsRetainWireMetadata(t *testing.T) {
	var drive Drive
	if err := json.Unmarshal([]byte(`{"Operations":[{"AssociatedTask":[],"VendorProgress":"kept"}]}`), &drive); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(drive)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct{ Operations []map[string]json.RawMessage }
	json.Unmarshal(data, &raw)
	if string(raw.Operations[0]["AssociatedTask"]) != "[]" || string(raw.Operations[0]["VendorProgress"]) != `"kept"` {
		t.Fatalf("lost metadata: %s", data)
	}
	var controller StorageController
	json.Unmarshal([]byte(`{"PCIeInterface":[]}`), &controller)
	data, err = json.Marshal(controller)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(data, &fields)
	if string(fields["PCIeInterface"]) != "[]" {
		t.Fatalf("lost empty array: %s", data)
	}
}

func TestSupermicroNullServersRemainAbsent(t *testing.T) {
	var service SyslogService
	if err := json.Unmarshal([]byte(`{"EnableSyslog":null,"SyslogServer":null}`), &service); err != nil {
		t.Fatal(err)
	}
	if service.ServiceEnabled != nil || service.SyslogServers != nil {
		t.Fatal("invented values for null syslog fields")
	}
	data, err := json.Marshal(service)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(data, &fields)
	if string(fields["SyslogServer"]) != "null" {
		t.Fatalf("lost null: %s", data)
	}
}
