package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

type identified interface {
	ResourceURI() string
	ResourceID() string
}

func selectItem[T identified](items []T, id string) (*T, error) {
	var selected *T
	for _, item := range items {
		uri := item.ResourceURI()
		if id == "" || id == item.ResourceID() || id == uri || memberMatches(item, id) || strings.TrimRight(uri, "/")[strings.LastIndex(strings.TrimRight(uri, "/"), "/")+1:] == id {
			if selected != nil {
				return nil, errors.New("resource selection is ambiguous; specify an identifier")
			}
			copy := item
			selected = &copy
		}
	}
	if selected == nil {
		return nil, sdkerrors.ErrNotFound
	}
	return selected, nil
}

func listLinks[T any](ctx context.Context, c *Client, links []models.Link) ([]T, error) {
	out := make([]T, 0)
	seen := map[string]bool{}
	for _, link := range links {
		uri, err := requireLink(&link, "@odata.id")
		if err != nil {
			return nil, err
		}
		if seen[uri] {
			continue
		}
		seen[uri] = true
		item, err := getResource[T](ctx, c, uri)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, nil
}

func (c *Client) GetVolumes(ctx context.Context, storageID string, systemID ...string) ([]models.Volume, error) {
	storages, err := c.GetStorages(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	storage, err := selectItem(storages, storageID)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Volume](ctx, c, storage.Volumes, "Volumes")
}

func (c *Client) GetPCIeDevices(ctx context.Context, chassisID ...string) ([]models.PCIeDevice, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	if len(chassis.PCIeDevices) == 0 {
		if chassis.Links != nil {
			return listLinks[models.PCIeDevice](ctx, c, chassis.Links.PCIeDevices)
		}
		return []models.PCIeDevice{}, nil
	}
	var link models.Link
	if json.Unmarshal(chassis.PCIeDevices, &link) == nil && models.Value(link.ODataID) != "" {
		items, err := linkedCollection[models.PCIeDevice](ctx, c, &link, "PCIeDevices")
		if errors.Is(err, sdkerrors.ErrNotFound) && chassis.Links != nil {
			return listLinks[models.PCIeDevice](ctx, c, chassis.Links.PCIeDevices)
		}
		return items, err
	}
	var links []models.Link
	if err = json.Unmarshal(chassis.PCIeDevices, &links); err != nil {
		return nil, err
	}
	return listLinks[models.PCIeDevice](ctx, c, links)
}

func (c *Client) GetPowerSupplyMetrics(ctx context.Context, id string, chassisID ...string) (*models.PowerSupplyMetrics, error) {
	supply, err := c.GetPowerSupply(ctx, id, chassisID...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.PowerSupplyMetrics](ctx, c, supply.Metrics, "Metrics")
}

func (c *Client) GetFan(ctx context.Context, chassisID ...string) ([]models.Fan, error) {
	thermal, err := c.GetThermal(ctx, chassisID...)
	if err == nil {
		return thermal.Fans, nil
	}
	if !errors.Is(err, sdkerrors.ErrNotFound) {
		return nil, err
	}
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	subsystem, err := linkedResource[models.ThermalSubsystem](ctx, c, chassis.ThermalSubsystem, "ThermalSubsystem")
	if err != nil {
		return nil, err
	}
	fans, err := linkedCollection[models.StandaloneFan](ctx, c, subsystem.Fans, "Fans")
	if err != nil {
		return nil, err
	}
	out := make([]models.Fan, 0, len(fans))
	for _, fan := range fans {
		item := models.Fan{ODataID: fan.ODataID, ID: fan.ID, Name: fan.Name, Status: fan.Status}
		item.SpeedPercent = fan.SpeedPercent
		out = append(out, item)
	}
	return out, nil
}

func (c *Client) SetIndicatorLED(ctx context.Context, state string, chassisID ...string) (*sdkresponse.Response, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	return c.patchWithETag(ctx, models.Value(chassis.ODataID), Object{"IndicatorLED": state})
}

func (c *Client) SetDriveIndicatorLED(ctx context.Context, uri, state string) (*sdkresponse.Response, error) {
	return c.patchWithETag(ctx, uri, Object{"IndicatorLED": state})
}

func (c *Client) SetDriveLocationIndicator(ctx context.Context, uri string, active bool) (*sdkresponse.Response, error) {
	return c.patchWithETag(ctx, uri, Object{"LocationIndicatorActive": active})
}

func (c *Client) DriveReset(ctx context.Context, uri, kind string) (*models.RedfishResponse, error) {
	actions, err := c.resourceActions(ctx, uri)
	if err != nil {
		return nil, err
	}
	action, err := findAction(actions, "Reset")
	if err != nil {
		return nil, err
	}
	return c.postOperation(ctx, action.Target, Object{wire.FieldResetType: kind})
}

func (c *Client) GetManufacturer(ctx context.Context, systemID ...string) (string, error) {
	system, err := c.GetSystem(ctx, systemID...)
	if err != nil {
		return "", err
	}
	return models.Value(system.Manufacturer), nil
}

func (c *Client) GetNetworkPorts(ctx context.Context, interfaceID string, systemID ...string) ([]models.NetworkPort, error) {
	interfaces, err := c.GetNetworkInterfaces(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if interfaceID != "" {
		item, err := selectItem(interfaces, interfaceID)
		if err != nil {
			return nil, err
		}
		interfaces = []models.NetworkInterface{*item}
	}
	var result []models.NetworkPort
	advertised := false
	for _, item := range interfaces {
		link := item.NetworkPorts
		if link == nil && item.Links != nil && item.Links.NetworkAdapter != nil {
			adapter, err := linkedResource[models.NetworkAdapter](ctx, c, item.Links.NetworkAdapter, "NetworkAdapter")
			if err != nil {
				return nil, err
			}
			link = adapter.NetworkPorts
		}
		if link == nil {
			continue
		}
		advertised = true
		ports, err := linkedCollection[models.NetworkPort](ctx, c, link, "NetworkPorts")
		if err != nil {
			return nil, err
		}
		result = append(result, ports...)
	}
	if !advertised {
		return nil, sdkerrors.ErrNotFound
	}
	return uniqueResources(result), nil
}

func (c *Client) GetNetworkPort(ctx context.Context, id, interfaceID string, systemID ...string) (*models.NetworkPort, error) {
	ports, err := c.GetNetworkPorts(ctx, interfaceID, systemID...)
	if err != nil {
		return nil, err
	}
	return selectItem(ports, id)
}

func (c *Client) GetMemoryChunks(ctx context.Context, domainID string, systemID ...string) ([]models.MemoryChunks, error) {
	domains, err := c.GetMemoryDomains(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if domainID != "" {
		domain, err := selectItem(domains, domainID)
		if err != nil {
			return nil, err
		}
		domains = []models.MemoryDomain{*domain}
	}
	var result []models.MemoryChunks
	advertised := false
	for _, domain := range domains {
		if domain.MemoryChunks == nil {
			continue
		}
		advertised = true
		chunks, err := linkedCollection[models.MemoryChunks](ctx, c, domain.MemoryChunks, "MemoryChunks")
		if err != nil {
			return nil, err
		}
		result = append(result, chunks...)
	}
	if !advertised {
		return nil, sdkerrors.ErrNotFound
	}
	return uniqueResources(result), nil
}

func (c *Client) GetMemoryChunk(ctx context.Context, id, domainID string, systemID ...string) (*models.MemoryChunks, error) {
	items, err := c.GetMemoryChunks(ctx, domainID, systemID...)
	if err != nil {
		return nil, err
	}
	return selectItem(items, id)
}

func (c *Client) GetMemoryMetrics(ctx context.Context, memoryID string, systemID ...string) (*models.MemoryMetrics, error) {
	items, err := c.GetMemory(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	item, err := selectItem(items, memoryID)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.MemoryMetrics](ctx, c, item.Metrics, "Metrics")
}

func resourcesFromLinks[T any](ctx context.Context, c *Client, links []models.Link) ([]T, error) {
	var items []T
	seen := map[string]bool{}
	for _, link := range links {
		uri, err := requireLink(&link, "@odata.id")
		if err != nil {
			return nil, err
		}
		if seen[uri] {
			continue
		}
		seen[uri] = true
		r, err := c.Request(ctx, "GET", uri, nil)
		if err != nil {
			return nil, err
		}
		var view struct {
			Members []json.RawMessage `json:"Members"`
		}
		if err = r.Decode(&view); err != nil {
			return nil, err
		}
		if view.Members != nil {
			collection, err := collection[T](ctx, c, uri)
			if err != nil {
				return nil, err
			}
			items = append(items, collection...)
		} else {
			var item T
			if err = r.Decode(&item); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func (c *Client) GetCoolingUnits(ctx context.Context, chassisID ...string) ([]models.CoolingUnit, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	if chassis.Links == nil || len(chassis.Links.CoolingUnits) == 0 {
		return nil, sdkerrors.ErrNotFound
	}
	return resourcesFromLinks[models.CoolingUnit](ctx, c, chassis.Links.CoolingUnits)
}

func (c *Client) GetCoolantConnectors(ctx context.Context, unitID string, chassisID ...string) ([]models.CoolantConnector, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	var links []models.Link
	if unitID == "" && chassis.ThermalSubsystem != nil {
		subsystem, err := linkedResource[models.ThermalSubsystem](ctx, c, chassis.ThermalSubsystem, "ThermalSubsystem")
		if err != nil {
			return nil, err
		}
		if subsystem.CoolantConnectors != nil {
			links = append(links, *subsystem.CoolantConnectors)
		}
	}
	var units []models.CoolingUnit
	if chassis.Links != nil && len(chassis.Links.CoolingUnits) > 0 {
		units, err = resourcesFromLinks[models.CoolingUnit](ctx, c, chassis.Links.CoolingUnits)
		if err != nil {
			return nil, err
		}
	}
	if unitID != "" {
		selected, err := selectItem(units, unitID)
		if err != nil {
			return nil, err
		}
		units = []models.CoolingUnit{*selected}
	}
	for _, unit := range units {
		if unit.PrimaryCoolantConnectors != nil {
			links = append(links, *unit.PrimaryCoolantConnectors)
		}
		if unit.SecondaryCoolantConnectors != nil {
			links = append(links, *unit.SecondaryCoolantConnectors)
		}
	}
	if len(links) == 0 {
		return nil, sdkerrors.ErrNotFound
	}
	return resourcesFromLinks[models.CoolantConnector](ctx, c, links)
}

func (c *Client) GetEnvironmentMetrics(ctx context.Context, unitID string, chassisID ...string) (*models.EnvironmentMetrics, error) {
	if unitID != "" {
		units, err := c.GetCoolingUnits(ctx, chassisID...)
		if err != nil {
			return nil, err
		}
		unit, err := selectItem(units, unitID)
		if err != nil {
			return nil, err
		}
		return linkedResource[models.EnvironmentMetrics](ctx, c, unit.EnvironmentMetrics, "EnvironmentMetrics")
	}
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.EnvironmentMetrics](ctx, c, chassis.EnvironmentMetrics, "EnvironmentMetrics")
}

func (c *Client) GetPowerSupplies(ctx context.Context, chassisID ...string) ([]models.PowerSupply, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	if chassis.PowerSubsystem != nil {
		subsystem, err := linkedResource[models.PowerSubsystem](ctx, c, chassis.PowerSubsystem, "PowerSubsystem")
		if err == nil && subsystem.PowerSupplies != nil {
			return linkedCollection[models.PowerSupply](ctx, c, subsystem.PowerSupplies, "PowerSupplies")
		}
		if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, err
		}
	}
	if chassis.Power == nil {
		return []models.PowerSupply{}, nil
	}
	power, err := linkedResource[models.Power](ctx, c, chassis.Power, "Power")
	if err != nil {
		return nil, err
	}
	return power.PowerSupplies, nil
}

func (c *Client) GetPowerSupply(ctx context.Context, id string, chassisID ...string) (*models.PowerSupply, error) {
	items, err := c.GetPowerSupplies(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	return selectItem(items, id)
}

func (c *Client) GetAllComponentsSummary(ctx context.Context, systemID, chassisID string) (Object, error) {
	fetches := []struct {
		name  string
		fetch func() (any, error)
	}{
		{name: "processors", fetch: func() (any, error) { return c.GetProcessors(ctx, systemID) }}, {name: "memory", fetch: func() (any, error) { return c.GetMemory(ctx, systemID) }}, {name: "storages", fetch: func() (any, error) { return c.GetStorages(ctx, systemID) }}, {name: "gpus", fetch: func() (any, error) { return c.GetGPUs(ctx, systemID) }}, {name: "drives", fetch: func() (any, error) { return c.GetDrives(ctx, chassisID) }}, {name: "network_adapters", fetch: func() (any, error) { return c.GetNetworkAdapters(ctx, chassisID) }}, {name: "pcie_devices", fetch: func() (any, error) { return c.GetPCIeDevices(ctx, chassisID) }}, {name: "power", fetch: func() (any, error) { return c.GetPower(ctx, chassisID) }}, {name: "thermal", fetch: func() (any, error) { return c.GetThermal(ctx, chassisID) }}, {name: "fans", fetch: func() (any, error) { return c.GetFan(ctx, chassisID) }}, {name: "power_supplies", fetch: func() (any, error) { return c.GetPowerSupplies(ctx, chassisID) }}, {name: "firmware_inventory", fetch: func() (any, error) { return c.GetFirmwareInventory(ctx) }}}
	result := Object{}
	for _, item := range fetches {
		value, err := item.fetch()
		if err != nil {
			return nil, err
		}
		result[item.name] = value
	}
	return result, nil
}

func requireIdentity(uri string) error {
	if strings.TrimSpace(uri) == "" {
		return fmt.Errorf("resource is missing @odata.id")
	}
	return nil
}

func memberMatches(item any, id string) bool {
	if id == "" {
		return false
	}
	switch value := item.(type) {
	case models.PowerSupply:
		return models.Value(value.MemberID) == id
	case models.Fan:
		return models.Value(value.MemberID) == id
	default:
		return false
	}
}

func (c *Client) GetDrives(ctx context.Context, chassisID ...string) ([]models.Drive, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	if chassis.Drives != nil {
		items, err := linkedCollection[models.Drive](ctx, c, chassis.Drives, "Drives")
		if err == nil && len(items) > 0 {
			return items, nil
		}
		if err != nil && !errors.Is(err, sdkerrors.ErrNotFound) {
			return nil, err
		}
	}
	if chassis.Links != nil && len(chassis.Links.Drives) > 0 {
		return resourcesFromLinks[models.Drive](ctx, c, chassis.Links.Drives)
	}
	return []models.Drive{}, nil
}

// GetStandaloneFans returns full modern Fan resources without projecting them onto legacy Thermal fans.
func (c *Client) GetStandaloneFans(ctx context.Context, chassisID ...string) ([]models.StandaloneFan, error) {
	chassis, err := c.GetChassis(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	subsystem, err := linkedResource[models.ThermalSubsystem](ctx, c, chassis.ThermalSubsystem, "ThermalSubsystem")
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.StandaloneFan](ctx, c, subsystem.Fans, "Fans")
}
