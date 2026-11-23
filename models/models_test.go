package models

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPythonModelPayloads(t *testing.T) {
	data, err := os.ReadFile("../testdata/python_models.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]json.RawMessage
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	cases := map[string]any{
		"redfish_v1_Systems_1.json": &System{}, "redfish_v1_Systems_1_Processors_1.json": &Processor{}, "redfish_v1_Systems_1_Memory_CPU0_DIMMA0.json": &Memory{}, "redfish_v1_Systems_1_Storages.json": &Storage{}, "redfish_v1_Systems_1_Bios.json": &Bios{}, "redfish_v1_Chassis_1.json": &Chassis{}, "redfish_v1_Chassis_1_Drives_HDDPlaneDisk1.json": &Drive{}, "redfish_v1_Chassis_1_Power.json": &Power{}, "redfish_v1_Chassis_1_NetworkAdapters_NIC_RiserA_Slot3.json": &NetworkAdapter{}, "redfish_v1_Chassis_1_PCIeDevices_EthernetCard0.json": &PCIeDevice{}, "redfish_v1_Managers_1.json": &Manager{}, "redfish_v1_Managers_1_NetworkProtocol.json": &NetworkProtocol{}, "redfish_v1_Managers_1_EthernetInterfaces_example.json": &EthernetInterface{}, "redfish_v1_Systems_1_FruInfo.json": &Fru{}, "redfish_v1_Systems_1_LogServices_SEL.json": &Log{}, "redfish_v1_Systems_1_LogServices_SEL_Entries_1.json": &LogEntry{},
	}
	for name, model := range cases {
		t.Run(name, func(t *testing.T) {
			if err := json.Unmarshal(fixtures[name], model); err != nil {
				t.Fatal(err)
			}
			serialized, err := json.Marshal(model)
			if err != nil {
				t.Fatal(err)
			}
			fresh := reflect.New(reflect.TypeOf(model).Elem()).Interface()
			if err = json.Unmarshal(serialized, fresh); err != nil {
				t.Fatal(err)
			}
			second, err := json.Marshal(fresh)
			if err != nil {
				t.Fatal(err)
			}
			if string(second) != string(serialized) {
				t.Fatalf("round trip changed JSON: %s => %s", serialized, second)
			}
		})
	}
}
func TestUnknownFieldsAliasesAndZeroValues(t *testing.T) {
	var value UpdateService
	if err := json.Unmarshal([]byte(`{"MultiPartHttpPushUri":"/upload","ServiceEnabled":false,"Oem":{"NewVendor":{"Value":null}}}`), &value); err != nil {
		t.Fatal(err)
	}
	if value.MultiPartHTTPPushURI == nil || *value.MultiPartHTTPPushURI != "/upload" {
		t.Fatal("alternate wire alias was lost")
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	json.Unmarshal(data, &result)
	if result["ServiceEnabled"] != false || result["Oem"] == nil {
		t.Fatal("false or unknown extension was lost")
	}
	var id Entity
	if err = json.Unmarshal([]byte(`{"Id":42}`), &id); err != nil || string(*id.ID) != "42" {
		t.Fatalf("numeric identity: %v", err)
	}
}

func TestFetchedAccountPreservesFieldPresenceRecursively(t *testing.T) {
	var account Account
	if err := json.Unmarshal([]byte(`{"UserName":"tester","Enabled":null,"AccountTypes":[],"Oem":{"Vendor":{"Clear":null}}}`), &account); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err = json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if string(out["Enabled"]) != "null" || string(out["AccountTypes"]) != "[]" {
		t.Fatalf("explicit null or empty list was lost: %s", data)
	}
	if _, ok := out["Locked"]; ok {
		t.Fatal("an absent property was synthesized")
	}
}

func TestSensitiveRequestFormattingRetainsRawSerialization(t *testing.T) {
	password := "test-only-sensitive-value"
	request := AccountCreate{UserName: "tester", RoleID: "Administrator", Password: &password}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, request), password) {
			t.Fatal("sensitive DTO leaked a password")
		}
	}
	data, err := json.Marshal(request)
	if err != nil || !strings.Contains(string(data), password) {
		t.Fatal("display redaction changed the wire body")
	}
}

func TestObservedTaskMessageAndPercentageVariants(t *testing.T) {
	var task Task
	if err := json.Unmarshal([]byte(`{"@odata.id":"/task","TaskState":"Completed","Messages":{"MessageId":"Base.1.0.Success","Message":"Done"},"PercentComplete":"100%"}`), &task); err != nil {
		t.Fatal(err)
	}
	if len(task.Messages) != 1 || task.PercentComplete == nil || *task.PercentComplete != 100 {
		t.Fatalf("task variants were not normalized: %+v", task)
	}
}
