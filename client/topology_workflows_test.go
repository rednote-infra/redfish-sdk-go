package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

func hardwareGraph() map[string]string {
	return map[string]string{
		"/redfish/v1/": `{"Systems":{"@odata.id":"/systems"},"Chassis":{"@odata.id":"/chassis"},"Managers":{"@odata.id":"/managers"},"UpdateService":{"@odata.id":"/update"}}`,
		"/systems":     `{"Members":[{"@odata.id":"/system"}]}`,
		"/system":      `{"@odata.id":"/system","Id":"sys","Processors":{"@odata.id":"/processors"},"Memory":{"@odata.id":"/memory"},"MemoryDomains":{"@odata.id":"/domains"},"NetworkInterfaces":{"@odata.id":"/interfaces"},"Storage":{"@odata.id":"/storage"},"LogServices":{"@odata.id":"/logs"},"Links":{"Chassis":[{"@odata.id":"/chassis/tray"}]},"Oem":{"Public":{"Fru":{"@odata.id":"/fru"}}}}`,
		"/processors":  `{"Members":[{"@odata.id":"/processor"}]}`, "/processor": `{"@odata.id":"/processor","Id":"cpu","ProcessorType":"CPU","Ports":{"@odata.id":"/ports"}}`,
		"/ports": `{"Members":[{"@odata.id":"/port"}]}`, "/port": `{"@odata.id":"/port","Id":"port1","Metrics":{"@odata.id":"/port-metrics"},"Links":{"CXLLogicalDevices":[{"@odata.id":"/logical-device"}]},"Actions":{"#Port.ResetTransceivers":{"target":"/transceiver-reset"}}}`, "/port-metrics": `{"Id":"metrics"}`,
		"/memory": `{"Members":[{"@odata.id":"/dimm"}]}`, "/dimm": `{"@odata.id":"/dimm","Id":"mem","Metrics":{"@odata.id":"/memory-metrics"}}`, "/memory-metrics": `{"Id":"metrics"}`,
		"/domains": `{"Members":[{"@odata.id":"/domain"}]}`, "/domain": `{"@odata.id":"/domain","Id":"domain","MemoryChunks":{"@odata.id":"/chunks"}}`, "/chunks": `{"Members":[{"@odata.id":"/chunk"}]}`, "/chunk": `{"@odata.id":"/chunk","Id":"chunk"}`,
		"/interfaces": `{"Members":[{"@odata.id":"/interface"}]}`, "/interface": `{"@odata.id":"/interface","Id":"net","Links":{"NetworkAdapter":{"@odata.id":"/adapter"}}}`, "/adapter": `{"@odata.id":"/adapter","Id":"adapter","NetworkPorts":{"@odata.id":"/network-ports"}}`, "/network-ports": `{"Members":[{"@odata.id":"/network-port"}]}`, "/network-port": `{"@odata.id":"/network-port","Id":"net-port"}`,
		"/storage": `{"Members":[{"@odata.id":"/storage/one"}]}`, "/storage/one": `{"@odata.id":"/storage/one","Id":"storage","Volumes":{"@odata.id":"/volumes"}}`, "/volumes": `{"Members":[{"@odata.id":"/volume"}]}`, "/volume": `{"@odata.id":"/volume","Id":"volume"}`,
		"/chassis":         `{"Members":[{"@odata.id":"/chassis/tray"}]}`,
		"/chassis/tray":    `{"@odata.id":"/chassis/tray","Id":"tray","Power":{"@odata.id":"/legacy-power"},"PowerSubsystem":{"@odata.id":"/power-subsystem"},"Thermal":{"@odata.id":"/thermal"},"ThermalSubsystem":{"@odata.id":"/thermal-subsystem"},"Drives":{"@odata.id":"/drives"},"NetworkAdapters":{"@odata.id":"/adapters"},"PCIeDevices":{"@odata.id":"/devices"},"EnvironmentMetrics":{"@odata.id":"/environment"},"Links":{"CoolingUnits":[{"@odata.id":"/cooling-unit"}]},"Oem":{"xFusion":{"FruService":{"@odata.id":"/fru-collection"}}}}`,
		"/power-subsystem": `{"PowerSupplies":{"@odata.id":"/supplies"}}`, "/supplies": `{"Members":[{"@odata.id":"/supply"}]}`, "/supply": `{"@odata.id":"/supply","Id":"psu","MemberId":"psu-alias","Metrics":{"@odata.id":"/supply-metrics"}}`, "/supply-metrics": `{"Id":"metrics"}`,
		"/legacy-power": `{"PowerSupplies":[{"MemberId":"legacy-psu","PowerCapacityWatts":1000}]}`, "/thermal": `{"Fans":[{"MemberId":"fan1","Reading":5000,"MaxReadingRange":10000}]}`,
		"/thermal-subsystem": `{"Fans":{"@odata.id":"/modern-fans"},"CoolantConnectors":{"@odata.id":"/connectors"}}`, "/modern-fans": `{"Members":[{"@odata.id":"/modern-fan"}]}`, "/modern-fan": `{"@odata.id":"/modern-fan","Id":"fan1","SpeedPercent":{"Reading":50.5}}`,
		"/cooling-unit": `{"@odata.id":"/cooling-unit","Id":"unit","EnvironmentMetrics":{"@odata.id":"/environment"},"PrimaryCoolantConnectors":{"@odata.id":"/connectors"}}`, "/connectors": `{"Members":[{"@odata.id":"/connector"}]}`, "/connector": `{"@odata.id":"/connector","Id":"connector"}`, "/environment": `{"PowerWatts":{"Reading":400}}`,
		"/drives": `{"Members":[{"@odata.id":"/drive"}]}`, "/drive": `{"@odata.id":"/drive","Id":"drive","Actions":{"#Drive.Reset":{"target":"/drive-reset"}}}`, "/adapters": `{"Members":[{"@odata.id":"/adapter"}]}`,
		"/devices": `{"Members":[{"@odata.id":"/device"}]}`, "/device": `{"@odata.id":"/device","Id":"device","DeviceType":"SingleFunction","CXLLogicalDevices":{"@odata.id":"/logical-devices"},"Links":{"ConnectedPCIePorts":[{"@odata.id":"/port"}]}}`, "/logical-devices": `{"Members":[{"@odata.id":"/logical-device"}]}`, "/logical-device": `{"@odata.id":"/logical-device","Id":"logical"}`,
		"/managers": `{"Members":[{"@odata.id":"/manager"}]}`, "/manager": `{"@odata.id":"/manager","LogServices":{"@odata.id":"/logs"}}`, "/logs": `{"Members":[{"@odata.id":"/logs/sel"}]}`, "/logs/sel": `{"@odata.id":"/logs/sel","Id":"sel","Entries":{"@odata.id":"/entries"},"Actions":{"#LogService.ClearLog":{"target":"/clear-log"}}}`, "/entries": `{"Members":[{"@odata.id":"/entry","Id":"entry","Message":"Synthetic log"}]}`,
		"/update": `{"FirmwareInventory":{"@odata.id":"/firmware"}}`, "/firmware": `{"Members":[]}`,
		"/fru": `{"MainBoard":{"Manufacturer":"Example","PrettyName":"System board"}}`, "/fru-collection": `{"Members":[{"@odata.id":"/fru-collection/0"}]}`, "/fru-collection/0": `{"@odata.id":"/fru-collection/0","BoardInfo":{"Manufacturer":"Example","PrettyName":"Chassis board"}}`,
	}
}

func graphHandler(t *testing.T, graph map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			switch r.URL.Path {
			case "/transceiver-reset", "/clear-log":
				assertWireBody(t, r, Object{})
			case "/drive-reset":
				assertWireBody(t, r, Object{"ResetType": "PowerCycle"})
			case "/chassis/tray", "/drive":
				var body Object
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
			default:
				t.Errorf("unexpected mutation %s", r.URL.Path)
			}
			w.WriteHeader(204)
			return
		}
		if body, ok := graph[r.URL.Path]; ok {
			w.Header().Set(wire.HeaderETag, `"v1"`)
			fmt.Fprint(w, body)
		} else {
			http.NotFound(w, r)
		}
	}
}

func TestHardwareCoolingFabricAndFRUWorkflows(t *testing.T) {
	c, _ := newTestClient(t, graphHandler(t, hardwareGraph()), WithVendor(VendorGeneric))
	ctx := context.Background()
	checks := []struct {
		name  string
		fetch func() (any, error)
	}{
		{name: "volumes", fetch: func() (any, error) { return c.GetVolumes(ctx, "storage") }},
		{name: "memory chunks", fetch: func() (any, error) { return c.GetMemoryChunks(ctx, "") }}, {name: "memory chunk", fetch: func() (any, error) { return c.GetMemoryChunk(ctx, "chunk", "") }}, {name: "memory metrics", fetch: func() (any, error) { return c.GetMemoryMetrics(ctx, "mem") }},
		{name: "network ports through adapter", fetch: func() (any, error) { return c.GetNetworkPorts(ctx, "") }}, {name: "network port", fetch: func() (any, error) { return c.GetNetworkPort(ctx, "net-port", "") }},
		{name: "processor ports", fetch: func() (any, error) { return c.GetProcessorPorts(ctx, "cpu") }}, {name: "owner ports", fetch: func() (any, error) { return c.GetPorts(ctx, "/processor") }}, {name: "port", fetch: func() (any, error) { return c.GetPort(ctx, "port1", "/processor") }}, {name: "connected ports", fetch: func() (any, error) { return c.GetConnectedPCIePorts(ctx, "device") }},
		{name: "port metrics", fetch: func() (any, error) {
			return c.GetPortMetrics(ctx, PortSelector{PortID: "port1", OwnerURI: "/processor"})
		}}, {name: "CXL devices", fetch: func() (any, error) { return c.GetCXLLogicalDevices(ctx, "device") }}, {name: "port CXL devices", fetch: func() (any, error) {
			return c.GetPortCXLLogicalDevices(ctx, PortSelector{PortID: "port1", OwnerURI: "/processor"})
		}},
		{name: "cooling units", fetch: func() (any, error) { return c.GetCoolingUnits(ctx) }}, {name: "coolant connectors", fetch: func() (any, error) { return c.GetCoolantConnectors(ctx, "") }}, {name: "unit coolant connectors", fetch: func() (any, error) { return c.GetCoolantConnectors(ctx, "unit") }}, {name: "environment", fetch: func() (any, error) { return c.GetEnvironmentMetrics(ctx, "") }}, {name: "unit environment", fetch: func() (any, error) { return c.GetEnvironmentMetrics(ctx, "unit") }},
		{name: "power supply alias", fetch: func() (any, error) { return c.GetPowerSupply(ctx, "psu-alias") }}, {name: "power supply metrics", fetch: func() (any, error) { return c.GetPowerSupplyMetrics(ctx, "psu") }}, {name: "fans", fetch: func() (any, error) { return c.GetFan(ctx) }},
		{name: "FRU", fetch: func() (any, error) { return c.GetSystemFRU(ctx) }}, {name: "FRU service", fetch: func() (any, error) { return c.GetFRUService(ctx) }}, {name: "baseboard FRU", fetch: func() (any, error) { return c.GetBaseboardFRU(ctx) }}, {name: "mainboard", fetch: func() (any, error) { return c.GetMainboard(ctx, "", "") }},
		{name: "system log entries", fetch: func() (any, error) { return c.GetSystemLogEntries(ctx, "sel") }}, {name: "manager log entries", fetch: func() (any, error) { return c.GetManagerLogEntries(ctx, "sel") }}, {name: "summary", fetch: func() (any, error) { return c.GetAllComponentsSummary(ctx, "", "") }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			result, err := check.fetch()
			if err != nil || result == nil {
				t.Fatalf("%s: %v", check.name, err)
			}
		})
	}
	if _, err := c.ResetPortTransceivers(ctx, "port1", "/processor"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DriveReset(ctx, "/drive", "PowerCycle"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetIndicatorLED(ctx, "Blinking"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetDriveIndicatorLED(ctx, "/drive", "Blinking"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetDriveLocationIndicator(ctx, "/drive", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClearSystemLog(ctx, "sel"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ClearManagerLog(ctx, "sel"); err != nil {
		t.Fatal(err)
	}
	if uri, err := c.GetODataID(ctx, models.RedfishResourceProcessors); err != nil || uri != "/processors" {
		t.Fatalf("resource key lookup: %q %v", uri, err)
	}
}

func TestLegacyPowerAndModernFanFallbacks(t *testing.T) {
	for _, variant := range []string{"legacy-power", "modern-fan"} {
		t.Run(variant, func(t *testing.T) {
			graph := hardwareGraph()
			var chassis Object
			if err := json.Unmarshal([]byte(graph["/chassis/tray"]), &chassis); err != nil {
				t.Fatal(err)
			}
			if variant == "legacy-power" {
				delete(chassis, "PowerSubsystem")
			} else {
				delete(chassis, "Thermal")
			}
			body, _ := json.Marshal(chassis)
			graph["/chassis/tray"] = string(body)
			c, _ := newTestClient(t, graphHandler(t, graph))
			if variant == "legacy-power" {
				supply, err := c.GetPowerSupply(context.Background(), "legacy-psu")
				if err != nil || pointer.Value(supply.PowerCapacityWatts) != 1000 {
					t.Fatalf("legacy PSU fallback: %v", err)
				}
			} else {
				fans, err := c.GetFan(context.Background())
				if err != nil || len(fans) != 1 {
					t.Fatalf("modern fan fallback: %v", err)
				}
				ratio := NewProviderRegistry().Extractor(VendorGeneric).FanSpeedRatio(&fans[0])
				if ratio == nil || *ratio != 50.5 {
					t.Fatal("modern fan precision was lost")
				}
			}
		})
	}
}

func TestMainboardFallsBackToChassisBoardInfo(t *testing.T) {
	graph := hardwareGraph()
	var system Object
	if err := json.Unmarshal([]byte(graph["/system"]), &system); err != nil {
		t.Fatal(err)
	}
	delete(system, "Oem")
	body, _ := json.Marshal(system)
	graph["/system"] = string(body)
	client, _ := newTestClient(t, graphHandler(t, graph))
	board, err := client.GetMainboard(context.Background(), "", "")
	if err != nil || board == nil || pointer.Value(board.PrettyName) != "Chassis board" {
		t.Fatalf("chassis board fallback: %+v %v", board, err)
	}
}
