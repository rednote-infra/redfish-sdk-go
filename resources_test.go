package redfish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// The fixture deliberately uses unrelated URIs to catch hard-coded resource paths.
func TestPublicResourceGettersUseAdvertisedURIs(t *testing.T) {
	graph := map[string]Object{}
	root := Object{}
	link := func(uri string) Object { return Object{"@odata.id": uri} }
	addResource := func(uri string) Object {
		if existing := graph[uri]; existing != nil {
			return existing
		}
		value := Object{"@odata.id": uri, "Id": "one", "Name": "Synthetic resource"}
		graph[uri] = value
		return value
	}
	addCollection := func(uri, memberURI string) {
		graph[uri] = Object{"Members": []Object{link(memberURI)}}
		addResource(memberURI)
	}
	addCollection("/catalog/system", "/resource/system")
	root["Systems"] = link("/catalog/system")
	addCollection("/catalog/chassis", "/resource/chassis")
	root["Chassis"] = link("/catalog/chassis")
	addCollection("/catalog/manager", "/resource/manager")
	root["Managers"] = link("/catalog/manager")
	addResource("/resource/account")
	root["AccountService"] = link("/resource/account")
	addResource("/resource/event")
	root["EventService"] = link("/resource/event")
	addResource("/resource/task")
	root["TaskService"] = link("/resource/task")
	addResource("/resource/update")
	root["UpdateService"] = link("/resource/update")
	addResource("/resource/composition")
	root["CompositionService"] = link("/resource/composition")
	addResource("/resource/policy")
	root["PolicyService"] = link("/resource/policy")
	addResource("/resource/telemetry")
	root["TelemetryService"] = link("/resource/telemetry")
	addResource("/resource/system")["Processors"] = link("/catalog/system/Processors")
	addCollection("/catalog/system/Processors", "/catalog/system/Processors/member")
	addResource("/resource/system")["Memory"] = link("/catalog/system/Memory")
	addCollection("/catalog/system/Memory", "/catalog/system/Memory/member")
	addResource("/resource/system")["Storage"] = link("/catalog/system/Storage")
	addCollection("/catalog/system/Storage", "/catalog/system/Storage/member")
	addResource("/resource/system")["LogServices"] = link("/catalog/system/LogServices")
	addCollection("/catalog/system/LogServices", "/catalog/system/LogServices/member")
	addResource("/resource/system")["NetworkInterfaces"] = link("/catalog/system/NetworkInterfaces")
	addCollection("/catalog/system/NetworkInterfaces", "/catalog/system/NetworkInterfaces/member")
	addResource("/resource/system")["MemoryDomains"] = link("/catalog/system/MemoryDomains")
	addCollection("/catalog/system/MemoryDomains", "/catalog/system/MemoryDomains/member")
	addResource("/resource/chassis")["Drives"] = link("/catalog/chassis/Drives")
	addCollection("/catalog/chassis/Drives", "/catalog/chassis/Drives/member")
	addResource("/resource/chassis")["NetworkAdapters"] = link("/catalog/chassis/NetworkAdapters")
	addCollection("/catalog/chassis/NetworkAdapters", "/catalog/chassis/NetworkAdapters/member")
	addResource("/resource/chassis")["Sensors"] = link("/catalog/chassis/Sensors")
	addCollection("/catalog/chassis/Sensors", "/catalog/chassis/Sensors/member")
	addResource("/resource/manager")["SerialInterfaces"] = link("/catalog/manager/SerialInterfaces")
	addCollection("/catalog/manager/SerialInterfaces", "/catalog/manager/SerialInterfaces/member")
	addResource("/resource/manager")["LogServices"] = link("/catalog/manager/LogServices")
	addCollection("/catalog/manager/LogServices", "/catalog/manager/LogServices/member")
	addResource("/resource/manager")["EthernetInterfaces"] = link("/catalog/manager/EthernetInterfaces")
	addCollection("/catalog/manager/EthernetInterfaces", "/catalog/manager/EthernetInterfaces/member")
	addResource("/resource/manager")["HostInterfaces"] = link("/catalog/manager/HostInterfaces")
	addCollection("/catalog/manager/HostInterfaces", "/catalog/manager/HostInterfaces/member")
	addResource("/resource/manager")["VirtualMedia"] = link("/catalog/manager/VirtualMedia")
	addCollection("/catalog/manager/VirtualMedia", "/catalog/manager/VirtualMedia/member")
	addResource("/resource/account")["Accounts"] = link("/catalog/account/Accounts")
	addCollection("/catalog/account/Accounts", "/catalog/account/Accounts/member")
	addResource("/resource/account")["Roles"] = link("/catalog/account/Roles")
	addCollection("/catalog/account/Roles", "/catalog/account/Roles/member")
	addResource("/resource/event")["Subscriptions"] = link("/catalog/event/Subscriptions")
	addCollection("/catalog/event/Subscriptions", "/catalog/event/Subscriptions/member")
	addResource("/resource/task")["Tasks"] = link("/catalog/task/Tasks")
	addCollection("/catalog/task/Tasks", "/catalog/task/Tasks/member")
	addResource("/resource/update")["FirmwareInventory"] = link("/catalog/update/FirmwareInventory")
	addCollection("/catalog/update/FirmwareInventory", "/catalog/update/FirmwareInventory/member")
	addResource("/resource/update")["SoftwareInventory"] = link("/catalog/update/SoftwareInventory")
	addCollection("/catalog/update/SoftwareInventory", "/catalog/update/SoftwareInventory/member")
	addResource("/resource/update")["ClientCertificates"] = link("/catalog/update/ClientCertificates")
	addCollection("/catalog/update/ClientCertificates", "/catalog/update/ClientCertificates/member")
	addResource("/resource/composition")["ResourceBlocks"] = link("/catalog/composition/ResourceBlocks")
	addCollection("/catalog/composition/ResourceBlocks", "/catalog/composition/ResourceBlocks/member")
	addResource("/resource/composition")["ResourceZones"] = link("/catalog/composition/ResourceZones")
	addCollection("/catalog/composition/ResourceZones", "/catalog/composition/ResourceZones/member")
	addResource("/resource/policy")["Policies"] = link("/catalog/policy/Policies")
	addCollection("/catalog/policy/Policies", "/catalog/policy/Policies/member")
	addResource("/resource/telemetry")["MetricDefinitions"] = link("/catalog/telemetry/MetricDefinitions")
	addCollection("/catalog/telemetry/MetricDefinitions", "/catalog/telemetry/MetricDefinitions/member")
	addResource("/resource/telemetry")["MetricReportDefinitions"] = link("/catalog/telemetry/MetricReportDefinitions")
	addCollection("/catalog/telemetry/MetricReportDefinitions", "/catalog/telemetry/MetricReportDefinitions/member")
	addResource("/resource/telemetry")["MetricReports"] = link("/catalog/telemetry/MetricReports")
	addCollection("/catalog/telemetry/MetricReports", "/catalog/telemetry/MetricReports/member")
	addResource("/resource/telemetry")["Triggers"] = link("/catalog/telemetry/Triggers")
	addCollection("/catalog/telemetry/Triggers", "/catalog/telemetry/Triggers/member")
	addResource("/resource/power-subsystem")["PowerSupplies"] = link("/catalog/power-subsystem/PowerSupplies")
	addCollection("/catalog/power-subsystem/PowerSupplies", "/catalog/power-subsystem/PowerSupplies/member")
	addResource("/resource/system")["Bios"] = link("/resource/bios")
	addResource("/resource/bios")
	addResource("/resource/system")["SecureBoot"] = link("/resource/secure-boot")
	addResource("/resource/secure-boot")
	addResource("/resource/chassis")["Power"] = link("/resource/power")
	addResource("/resource/power")
	addResource("/resource/chassis")["PowerSubsystem"] = link("/resource/power-subsystem")
	addResource("/resource/power-subsystem")
	addResource("/resource/chassis")["Thermal"] = link("/resource/thermal")
	addResource("/resource/thermal")
	addResource("/resource/manager")["NetworkProtocol"] = link("/resource/network-protocol")
	addResource("/resource/network-protocol")
	addResource("/resource/update")["UpdateServiceCapabilities"] = link("/resource/update-capabilities")
	addResource("/resource/update-capabilities")
	addResource("/resource/telemetry")["LogService"] = link("/resource/metric-log")
	addResource("/resource/metric-log")
	addResource("/catalog/task/Tasks/member")["TaskState"] = "Running"
	graph[DefaultServiceRoot] = root
	var lastURI string
	handler := func(w http.ResponseWriter, r *http.Request) {
		lastURI = r.URL.Path
		if value, ok := graph[r.URL.Path]; ok {
			if err := json.NewEncoder(w).Encode(value); err != nil {
				t.Error(err)
			}
		} else {
			t.Errorf("unexpected resource request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}
	cases := []struct {
		name, path string
		fetch      func(*Client) (any, error)
	}{
		{"GetSystem", "/resource/system", func(c *Client) (any, error) { return c.GetSystem(context.Background()) }},
		{"GetChassis", "/resource/chassis", func(c *Client) (any, error) { return c.GetChassis(context.Background()) }},
		{"GetManager", "/resource/manager", func(c *Client) (any, error) { return c.GetManager(context.Background()) }},
		{"GetAccountService", "/resource/account", func(c *Client) (any, error) { return c.GetAccountService(context.Background()) }},
		{"GetEventService", "/resource/event", func(c *Client) (any, error) { return c.GetEventService(context.Background()) }},
		{"GetTaskService", "/resource/task", func(c *Client) (any, error) { return c.GetTaskService(context.Background()) }},
		{"GetUpdateService", "/resource/update", func(c *Client) (any, error) { return c.GetUpdateService(context.Background()) }},
		{"GetCompositionService", "/resource/composition", func(c *Client) (any, error) { return c.GetCompositionService(context.Background()) }},
		{"GetPolicyService", "/resource/policy", func(c *Client) (any, error) { return c.GetPolicyService(context.Background()) }},
		{"GetTelemetryService", "/resource/telemetry", func(c *Client) (any, error) { return c.GetTelemetryService(context.Background()) }},
		{"GetProcessors", "/catalog/system/Processors/member", func(c *Client) (any, error) { return c.GetProcessors(context.Background()) }},
		{"GetMemory", "/catalog/system/Memory/member", func(c *Client) (any, error) { return c.GetMemory(context.Background()) }},
		{"GetStorages", "/catalog/system/Storage/member", func(c *Client) (any, error) { return c.GetStorages(context.Background()) }},
		{"GetSystemLogServices", "/catalog/system/LogServices/member", func(c *Client) (any, error) { return c.GetSystemLogServices(context.Background()) }},
		{"GetNetworkInterfaces", "/catalog/system/NetworkInterfaces/member", func(c *Client) (any, error) { return c.GetNetworkInterfaces(context.Background()) }},
		{"GetMemoryDomains", "/catalog/system/MemoryDomains/member", func(c *Client) (any, error) { return c.GetMemoryDomains(context.Background()) }},
		{"GetDrives", "/catalog/chassis/Drives/member", func(c *Client) (any, error) { return c.GetDrives(context.Background()) }},
		{"GetNetworkAdapters", "/catalog/chassis/NetworkAdapters/member", func(c *Client) (any, error) { return c.GetNetworkAdapters(context.Background()) }},
		{"GetSensors", "/catalog/chassis/Sensors/member", func(c *Client) (any, error) { return c.GetSensors(context.Background()) }},
		{"GetSerialInterfaces", "/catalog/manager/SerialInterfaces/member", func(c *Client) (any, error) { return c.GetSerialInterfaces(context.Background()) }},
		{"GetManagerLogServices", "/catalog/manager/LogServices/member", func(c *Client) (any, error) { return c.GetManagerLogServices(context.Background()) }},
		{"GetManagerEthernetInterfaces", "/catalog/manager/EthernetInterfaces/member", func(c *Client) (any, error) { return c.GetManagerEthernetInterfaces(context.Background()) }},
		{"GetHostInterfaces", "/catalog/manager/HostInterfaces/member", func(c *Client) (any, error) { return c.GetHostInterfaces(context.Background()) }},
		{"GetVirtualMedia", "/catalog/manager/VirtualMedia/member", func(c *Client) (any, error) { return c.GetVirtualMedia(context.Background()) }},
		{"GetAccounts", "/catalog/account/Accounts/member", func(c *Client) (any, error) { return c.GetAccounts(context.Background()) }},
		{"GetRoles", "/catalog/account/Roles/member", func(c *Client) (any, error) { return c.GetRoles(context.Background()) }},
		{"GetSubscriptions", "/catalog/event/Subscriptions/member", func(c *Client) (any, error) { return c.GetSubscriptions(context.Background()) }},
		{"GetTasks", "/catalog/task/Tasks/member", func(c *Client) (any, error) { return c.GetTasks(context.Background()) }},
		{"GetFirmwareInventory", "/catalog/update/FirmwareInventory/member", func(c *Client) (any, error) { return c.GetFirmwareInventory(context.Background()) }},
		{"GetSoftwareInventory", "/catalog/update/SoftwareInventory/member", func(c *Client) (any, error) { return c.GetSoftwareInventory(context.Background()) }},
		{"GetClientCertificates", "/catalog/update/ClientCertificates/member", func(c *Client) (any, error) { return c.GetClientCertificates(context.Background()) }},
		{"GetResourceBlocks", "/catalog/composition/ResourceBlocks/member", func(c *Client) (any, error) { return c.GetResourceBlocks(context.Background()) }},
		{"GetResourceZones", "/catalog/composition/ResourceZones/member", func(c *Client) (any, error) { return c.GetResourceZones(context.Background()) }},
		{"GetPolicies", "/catalog/policy/Policies/member", func(c *Client) (any, error) { return c.GetPolicies(context.Background()) }},
		{"GetMetricDefinitions", "/catalog/telemetry/MetricDefinitions/member", func(c *Client) (any, error) { return c.GetMetricDefinitions(context.Background()) }},
		{"GetMetricReportDefinitions", "/catalog/telemetry/MetricReportDefinitions/member", func(c *Client) (any, error) { return c.GetMetricReportDefinitions(context.Background()) }},
		{"GetMetricReports", "/catalog/telemetry/MetricReports/member", func(c *Client) (any, error) { return c.GetMetricReports(context.Background()) }},
		{"GetTelemetryTriggers", "/catalog/telemetry/Triggers/member", func(c *Client) (any, error) { return c.GetTelemetryTriggers(context.Background()) }},
		{"GetPowerSupplies", "/catalog/power-subsystem/PowerSupplies/member", func(c *Client) (any, error) { return c.GetPowerSupplies(context.Background()) }},
		{"GetBIOS", "/resource/bios", func(c *Client) (any, error) { return c.GetBIOS(context.Background()) }},
		{"GetSecureBoot", "/resource/secure-boot", func(c *Client) (any, error) { return c.GetSecureBoot(context.Background()) }},
		{"GetPower", "/resource/power", func(c *Client) (any, error) { return c.GetPower(context.Background()) }},
		{"GetPowerSubsystem", "/resource/power-subsystem", func(c *Client) (any, error) { return c.GetPowerSubsystem(context.Background()) }},
		{"GetThermal", "/resource/thermal", func(c *Client) (any, error) { return c.GetThermal(context.Background()) }},
		{"GetNetworkProtocol", "/resource/network-protocol", func(c *Client) (any, error) { return c.GetNetworkProtocol(context.Background()) }},
		{"GetUpdateServiceCapabilities", "/resource/update-capabilities", func(c *Client) (any, error) { return c.GetUpdateServiceCapabilities(context.Background()) }},
		{"GetMetricReportLog", "/resource/metric-log", func(c *Client) (any, error) { return c.GetMetricReportLog(context.Background()) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newTestClient(t, handler)
			lastURI = ""
			result, err := test.fetch(client)
			if err != nil {
				t.Fatal(err)
			}
			if lastURI != test.path {
				t.Fatalf("expected %s, requested %s", test.path, lastURI)
			}
			if result == nil {
				t.Fatal("resource result is nil")
			}
			_ = fmt.Sprint(result)
		})
	}
}
