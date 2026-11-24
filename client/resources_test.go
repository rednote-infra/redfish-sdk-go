package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
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
	graph[wire.DefaultServiceRoot] = root
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
		{name: "GetSystem", path: "/resource/system", fetch: func(c *Client) (any, error) { return c.GetSystem(context.Background()) }},
		{name: "GetChassis", path: "/resource/chassis", fetch: func(c *Client) (any, error) { return c.GetChassis(context.Background()) }},
		{name: "GetManager", path: "/resource/manager", fetch: func(c *Client) (any, error) { return c.GetManager(context.Background()) }},
		{name: "GetAccountService", path: "/resource/account", fetch: func(c *Client) (any, error) { return c.GetAccountService(context.Background()) }},
		{name: "GetEventService", path: "/resource/event", fetch: func(c *Client) (any, error) { return c.GetEventService(context.Background()) }},
		{name: "GetTaskService", path: "/resource/task", fetch: func(c *Client) (any, error) { return c.GetTaskService(context.Background()) }},
		{name: "GetUpdateService", path: "/resource/update", fetch: func(c *Client) (any, error) { return c.GetUpdateService(context.Background()) }},
		{name: "GetCompositionService", path: "/resource/composition", fetch: func(c *Client) (any, error) { return c.GetCompositionService(context.Background()) }},
		{name: "GetPolicyService", path: "/resource/policy", fetch: func(c *Client) (any, error) { return c.GetPolicyService(context.Background()) }},
		{name: "GetTelemetryService", path: "/resource/telemetry", fetch: func(c *Client) (any, error) { return c.GetTelemetryService(context.Background()) }},
		{name: "GetProcessors", path: "/catalog/system/Processors/member", fetch: func(c *Client) (any, error) { return c.GetProcessors(context.Background()) }},
		{name: "GetMemory", path: "/catalog/system/Memory/member", fetch: func(c *Client) (any, error) { return c.GetMemory(context.Background()) }},
		{name: "GetStorages", path: "/catalog/system/Storage/member", fetch: func(c *Client) (any, error) { return c.GetStorages(context.Background()) }},
		{name: "GetSystemLogServices", path: "/catalog/system/LogServices/member", fetch: func(c *Client) (any, error) { return c.GetSystemLogServices(context.Background()) }},
		{name: "GetNetworkInterfaces", path: "/catalog/system/NetworkInterfaces/member", fetch: func(c *Client) (any, error) { return c.GetNetworkInterfaces(context.Background()) }},
		{name: "GetMemoryDomains", path: "/catalog/system/MemoryDomains/member", fetch: func(c *Client) (any, error) { return c.GetMemoryDomains(context.Background()) }},
		{name: "GetDrives", path: "/catalog/chassis/Drives/member", fetch: func(c *Client) (any, error) { return c.GetDrives(context.Background()) }},
		{name: "GetNetworkAdapters", path: "/catalog/chassis/NetworkAdapters/member", fetch: func(c *Client) (any, error) { return c.GetNetworkAdapters(context.Background()) }},
		{name: "GetSensors", path: "/catalog/chassis/Sensors/member", fetch: func(c *Client) (any, error) { return c.GetSensors(context.Background()) }},
		{name: "GetSerialInterfaces", path: "/catalog/manager/SerialInterfaces/member", fetch: func(c *Client) (any, error) { return c.GetSerialInterfaces(context.Background()) }},
		{name: "GetManagerLogServices", path: "/catalog/manager/LogServices/member", fetch: func(c *Client) (any, error) { return c.GetManagerLogServices(context.Background()) }},
		{name: "GetManagerEthernetInterfaces", path: "/catalog/manager/EthernetInterfaces/member", fetch: func(c *Client) (any, error) { return c.GetManagerEthernetInterfaces(context.Background()) }},
		{name: "GetHostInterfaces", path: "/catalog/manager/HostInterfaces/member", fetch: func(c *Client) (any, error) { return c.GetHostInterfaces(context.Background()) }},
		{name: "GetVirtualMedia", path: "/catalog/manager/VirtualMedia/member", fetch: func(c *Client) (any, error) { return c.GetVirtualMedia(context.Background()) }},
		{name: "GetAccounts", path: "/catalog/account/Accounts/member", fetch: func(c *Client) (any, error) { return c.GetAccounts(context.Background()) }},
		{name: "GetRoles", path: "/catalog/account/Roles/member", fetch: func(c *Client) (any, error) { return c.GetRoles(context.Background()) }},
		{name: "GetSubscriptions", path: "/catalog/event/Subscriptions/member", fetch: func(c *Client) (any, error) { return c.GetSubscriptions(context.Background()) }},
		{name: "GetTasks", path: "/catalog/task/Tasks/member", fetch: func(c *Client) (any, error) { return c.GetTasks(context.Background()) }},
		{name: "GetFirmwareInventory", path: "/catalog/update/FirmwareInventory/member", fetch: func(c *Client) (any, error) { return c.GetFirmwareInventory(context.Background()) }},
		{name: "GetSoftwareInventory", path: "/catalog/update/SoftwareInventory/member", fetch: func(c *Client) (any, error) { return c.GetSoftwareInventory(context.Background()) }},
		{name: "GetClientCertificates", path: "/catalog/update/ClientCertificates/member", fetch: func(c *Client) (any, error) { return c.GetClientCertificates(context.Background()) }},
		{name: "GetResourceBlocks", path: "/catalog/composition/ResourceBlocks/member", fetch: func(c *Client) (any, error) { return c.GetResourceBlocks(context.Background()) }},
		{name: "GetResourceZones", path: "/catalog/composition/ResourceZones/member", fetch: func(c *Client) (any, error) { return c.GetResourceZones(context.Background()) }},
		{name: "GetPolicies", path: "/catalog/policy/Policies/member", fetch: func(c *Client) (any, error) { return c.GetPolicies(context.Background()) }},
		{name: "GetMetricDefinitions", path: "/catalog/telemetry/MetricDefinitions/member", fetch: func(c *Client) (any, error) { return c.GetMetricDefinitions(context.Background()) }},
		{name: "GetMetricReportDefinitions", path: "/catalog/telemetry/MetricReportDefinitions/member", fetch: func(c *Client) (any, error) { return c.GetMetricReportDefinitions(context.Background()) }},
		{name: "GetMetricReports", path: "/catalog/telemetry/MetricReports/member", fetch: func(c *Client) (any, error) { return c.GetMetricReports(context.Background()) }},
		{name: "GetTelemetryTriggers", path: "/catalog/telemetry/Triggers/member", fetch: func(c *Client) (any, error) { return c.GetTelemetryTriggers(context.Background()) }},
		{name: "GetPowerSupplies", path: "/catalog/power-subsystem/PowerSupplies/member", fetch: func(c *Client) (any, error) { return c.GetPowerSupplies(context.Background()) }},
		{name: "GetBIOS", path: "/resource/bios", fetch: func(c *Client) (any, error) { return c.GetBIOS(context.Background()) }},
		{name: "GetSecureBoot", path: "/resource/secure-boot", fetch: func(c *Client) (any, error) { return c.GetSecureBoot(context.Background()) }},
		{name: "GetPower", path: "/resource/power", fetch: func(c *Client) (any, error) { return c.GetPower(context.Background()) }},
		{name: "GetPowerSubsystem", path: "/resource/power-subsystem", fetch: func(c *Client) (any, error) { return c.GetPowerSubsystem(context.Background()) }},
		{name: "GetThermal", path: "/resource/thermal", fetch: func(c *Client) (any, error) { return c.GetThermal(context.Background()) }},
		{name: "GetNetworkProtocol", path: "/resource/network-protocol", fetch: func(c *Client) (any, error) { return c.GetNetworkProtocol(context.Background()) }},
		{name: "GetUpdateServiceCapabilities", path: "/resource/update-capabilities", fetch: func(c *Client) (any, error) { return c.GetUpdateServiceCapabilities(context.Background()) }},
		{name: "GetMetricReportLog", path: "/resource/metric-log", fetch: func(c *Client) (any, error) { return c.GetMetricReportLog(context.Background()) }},
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
