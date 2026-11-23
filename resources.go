package redfish

import (
	"context"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func (c *Client) GetSystem(ctx context.Context, ids ...string) (*models.System, error) {
	return rootSelect[models.System](ctx, c, "Systems", ids)
}

func (c *Client) GetChassis(ctx context.Context, ids ...string) (*models.Chassis, error) {
	return rootSelect[models.Chassis](ctx, c, "Chassis", ids)
}

func (c *Client) GetManager(ctx context.Context, ids ...string) (*models.Manager, error) {
	return rootSelect[models.Manager](ctx, c, "Managers", ids)
}

func (c *Client) GetSystems(ctx context.Context) ([]models.System, error) {
	return rootCollection[models.System](ctx, c, "Systems")
}

func (c *Client) GetChassisCollection(ctx context.Context) ([]models.Chassis, error) {
	return rootCollection[models.Chassis](ctx, c, "Chassis")
}

func (c *Client) GetManagers(ctx context.Context) ([]models.Manager, error) {
	return rootCollection[models.Manager](ctx, c, "Managers")
}

func (c *Client) GetRegistries(ctx context.Context) ([]models.Registry, error) {
	return rootCollection[models.Registry](ctx, c, "Registries")
}

func (c *Client) GetAccountService(ctx context.Context) (*models.AccountService, error) {
	return rootResource[models.AccountService](ctx, c, "AccountService")
}

func (c *Client) GetSessionService(ctx context.Context) (*models.SessionService, error) {
	return rootResource[models.SessionService](ctx, c, "SessionService")
}

func (c *Client) GetEventService(ctx context.Context) (*models.EventService, error) {
	return rootResource[models.EventService](ctx, c, "EventService")
}

func (c *Client) GetTaskService(ctx context.Context) (*models.TaskService, error) {
	return rootResource[models.TaskService](ctx, c, "TaskService")
}

func (c *Client) GetUpdateService(ctx context.Context) (*models.UpdateService, error) {
	return rootResource[models.UpdateService](ctx, c, "UpdateService")
}

func (c *Client) GetCompositionService(ctx context.Context) (*models.CompositionService, error) {
	return rootResource[models.CompositionService](ctx, c, "CompositionService")
}

func (c *Client) GetPolicyService(ctx context.Context) (*models.PolicyService, error) {
	return rootResource[models.PolicyService](ctx, c, "PolicyService")
}

func (c *Client) GetTelemetryService(ctx context.Context) (*models.TelemetryService, error) {
	return rootResource[models.TelemetryService](ctx, c, "TelemetryService")
}

func (c *Client) GetProcessors(ctx context.Context, ids ...string) ([]models.Processor, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Processor](ctx, c, parent.Processors, "Processors")
}

func (c *Client) GetMemory(ctx context.Context, ids ...string) ([]models.Memory, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Memory](ctx, c, parent.Memory, "Memory")
}

func (c *Client) GetStorages(ctx context.Context, ids ...string) ([]models.Storage, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Storage](ctx, c, parent.Storage, "Storage")
}

func (c *Client) GetSystemLogServices(ctx context.Context, ids ...string) ([]models.Log, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Log](ctx, c, parent.LogServices, "LogServices")
}

func (c *Client) GetNetworkInterfaces(ctx context.Context, ids ...string) ([]models.NetworkInterface, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.NetworkInterface](ctx, c, parent.NetworkInterfaces, "NetworkInterfaces")
}

func (c *Client) GetMemoryDomains(ctx context.Context, ids ...string) ([]models.MemoryDomain, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.MemoryDomain](ctx, c, parent.MemoryDomains, "MemoryDomains")
}

func (c *Client) GetNetworkAdapters(ctx context.Context, ids ...string) ([]models.NetworkAdapter, error) {
	parent, err := c.GetChassis(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.NetworkAdapter](ctx, c, parent.NetworkAdapters, "NetworkAdapters")
}

func (c *Client) GetSensors(ctx context.Context, ids ...string) ([]models.Sensor, error) {
	parent, err := c.GetChassis(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Sensor](ctx, c, parent.Sensors, "Sensors")
}

func (c *Client) GetSerialInterfaces(ctx context.Context, ids ...string) ([]models.SerialInterface, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.SerialInterface](ctx, c, parent.SerialInterfaces, "SerialInterfaces")
}

func (c *Client) GetManagerLogServices(ctx context.Context, ids ...string) ([]models.Log, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Log](ctx, c, parent.LogServices, "LogServices")
}

func (c *Client) GetManagerEthernetInterfaces(ctx context.Context, ids ...string) ([]models.EthernetInterface, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.EthernetInterface](ctx, c, parent.EthernetInterfaces, "EthernetInterfaces")
}

func (c *Client) GetHostInterfaces(ctx context.Context, ids ...string) ([]models.HostInterface, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.HostInterface](ctx, c, parent.HostInterfaces, "HostInterfaces")
}

func (c *Client) GetVirtualMedia(ctx context.Context, managerID ...string) ([]models.VirtualMedia, error) {
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	link := manager.VirtualMedia
	if link == nil && manager.OEM != nil {
		uri, err := findOEMLink(*manager.OEM, "VirtualMedia")
		if err != nil {
			return nil, err
		}
		link = &models.Link{ODataID: &uri}
	}
	return linkedCollection[models.VirtualMedia](ctx, c, link, "VirtualMedia")
}

func (c *Client) GetAccounts(ctx context.Context) ([]models.Account, error) {
	parent, err := c.GetAccountService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Account](ctx, c, parent.Accounts, "Accounts")
}

func (c *Client) GetRoles(ctx context.Context) ([]models.Role, error) {
	parent, err := c.GetAccountService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Role](ctx, c, parent.Roles, "Roles")
}

func (c *Client) GetSubscriptions(ctx context.Context) ([]models.Subscription, error) {
	parent, err := c.GetEventService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Subscription](ctx, c, parent.Subscriptions, "Subscriptions")
}

func (c *Client) GetTasks(ctx context.Context) ([]models.Task, error) {
	parent, err := c.GetTaskService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Task](ctx, c, parent.Tasks, "Tasks")
}

func (c *Client) GetFirmwareInventory(ctx context.Context) ([]models.FirmwareInventory, error) {
	parent, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.FirmwareInventory](ctx, c, parent.FirmwareInventory, "FirmwareInventory")
}

func (c *Client) GetSoftwareInventory(ctx context.Context) ([]models.SoftwareInventory, error) {
	parent, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.SoftwareInventory](ctx, c, parent.SoftwareInventory, "SoftwareInventory")
}

func (c *Client) GetClientCertificates(ctx context.Context) ([]models.ClientCertificate, error) {
	parent, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.ClientCertificate](ctx, c, parent.ClientCertificates, "ClientCertificates")
}

func (c *Client) GetResourceBlocks(ctx context.Context) ([]models.ResourceBlock, error) {
	parent, err := c.GetCompositionService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.ResourceBlock](ctx, c, parent.ResourceBlocks, "ResourceBlocks")
}

func (c *Client) GetResourceZones(ctx context.Context) ([]models.ResourceZone, error) {
	parent, err := c.GetCompositionService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.ResourceZone](ctx, c, parent.ResourceZones, "ResourceZones")
}

func (c *Client) GetPolicies(ctx context.Context) ([]models.Policy, error) {
	parent, err := c.GetPolicyService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Policy](ctx, c, parent.Policies, "Policies")
}

func (c *Client) GetMetricDefinitions(ctx context.Context) ([]models.MetricDefinition, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.MetricDefinition](ctx, c, parent.MetricDefinitions, "MetricDefinitions")
}

func (c *Client) GetMetricReportDefinitions(ctx context.Context) ([]models.MetricReportDefinition, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.MetricReportDefinition](ctx, c, parent.MetricReportDefinitions, "MetricReportDefinitions")
}

func (c *Client) GetMetricReports(ctx context.Context) ([]models.MetricReport, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.MetricReport](ctx, c, parent.MetricReports, "MetricReports")
}

func (c *Client) GetTelemetryTriggers(ctx context.Context) ([]models.Triggers, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.Triggers](ctx, c, parent.Triggers, "Triggers")
}

func (c *Client) GetBios(ctx context.Context, ids ...string) (*models.Bios, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.Bios](ctx, c, parent.BIOS, "BIOS")
}

func (c *Client) GetSecureBoot(ctx context.Context, ids ...string) (*models.SecureBoot, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.SecureBoot](ctx, c, parent.SecureBoot, "SecureBoot")
}

func (c *Client) GetPower(ctx context.Context, ids ...string) (*models.Power, error) {
	parent, err := c.GetChassis(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.Power](ctx, c, parent.Power, "Power")
}

func (c *Client) GetPowerSubsystem(ctx context.Context, ids ...string) (*models.PowerSubsystem, error) {
	parent, err := c.GetChassis(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.PowerSubsystem](ctx, c, parent.PowerSubsystem, "PowerSubsystem")
}

func (c *Client) GetThermal(ctx context.Context, ids ...string) (*models.Thermal, error) {
	parent, err := c.GetChassis(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.Thermal](ctx, c, parent.Thermal, "Thermal")
}

func (c *Client) GetNetworkProtocol(ctx context.Context, ids ...string) (*models.NetworkProtocol, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.NetworkProtocol](ctx, c, parent.NetworkProtocol, "NetworkProtocol")
}

func (c *Client) GetUpdateServiceCapabilities(ctx context.Context) (*models.UpdateServiceCapabilities, error) {
	parent, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.UpdateServiceCapabilities](ctx, c, parent.UpdateServiceCapabilities, "UpdateServiceCapabilities")
}

func (c *Client) GetMetricReportLog(ctx context.Context) (*models.Log, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.Log](ctx, c, parent.LogService, "LogService")
}

func (c *Client) GetProcessor(ctx context.Context, id string, ids ...string) (*models.Processor, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Processors, "Processors")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Processor](ctx, c, uri, id)
}

func (c *Client) GetMemoryDevice(ctx context.Context, id string, ids ...string) (*models.Memory, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Memory, "Memory")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Memory](ctx, c, uri, id)
}

func (c *Client) GetMemoryDomain(ctx context.Context, id string, ids ...string) (*models.MemoryDomain, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.MemoryDomains, "MemoryDomains")
	if err != nil {
		return nil, err
	}
	return selectResource[models.MemoryDomain](ctx, c, uri, id)
}

func (c *Client) GetSensor(ctx context.Context, id string, ids ...string) (*models.Sensor, error) {
	parent, err := c.GetChassis(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Sensors, "Sensors")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Sensor](ctx, c, uri, id)
}

func (c *Client) GetSerialInterface(ctx context.Context, id string, ids ...string) (*models.SerialInterface, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.SerialInterfaces, "SerialInterfaces")
	if err != nil {
		return nil, err
	}
	return selectResource[models.SerialInterface](ctx, c, uri, id)
}

func (c *Client) GetVirtualMediaResource(ctx context.Context, id string, managerID ...string) (*models.VirtualMedia, error) {
	items, err := c.GetVirtualMedia(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	return selectItem(items, id)
}

func (c *Client) GetSystemLogService(ctx context.Context, id string, ids ...string) (*models.Log, error) {
	parent, err := c.GetSystem(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.LogServices, "LogServices")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Log](ctx, c, uri, id)
}

func (c *Client) GetManagerLogService(ctx context.Context, id string, ids ...string) (*models.Log, error) {
	parent, err := c.GetManager(ctx, ids...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.LogServices, "LogServices")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Log](ctx, c, uri, id)
}

func (c *Client) GetSubscription(ctx context.Context, id string) (*models.Subscription, error) {
	parent, err := c.GetEventService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Subscriptions, "Subscriptions")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Subscription](ctx, c, uri, id)
}

func (c *Client) GetTask(ctx context.Context, id string) (*models.Task, error) {
	parent, err := c.GetTaskService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Tasks, "Tasks")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Task](ctx, c, uri, id)
}

func (c *Client) GetResourceBlock(ctx context.Context, id string) (*models.ResourceBlock, error) {
	parent, err := c.GetCompositionService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.ResourceBlocks, "ResourceBlocks")
	if err != nil {
		return nil, err
	}
	return selectResource[models.ResourceBlock](ctx, c, uri, id)
}

func (c *Client) GetResourceZone(ctx context.Context, id string) (*models.ResourceZone, error) {
	parent, err := c.GetCompositionService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.ResourceZones, "ResourceZones")
	if err != nil {
		return nil, err
	}
	return selectResource[models.ResourceZone](ctx, c, uri, id)
}

func (c *Client) GetPolicy(ctx context.Context, id string) (*models.Policy, error) {
	parent, err := c.GetPolicyService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Policies, "Policies")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Policy](ctx, c, uri, id)
}

func (c *Client) GetMetricDefinition(ctx context.Context, id string) (*models.MetricDefinition, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.MetricDefinitions, "MetricDefinitions")
	if err != nil {
		return nil, err
	}
	return selectResource[models.MetricDefinition](ctx, c, uri, id)
}

func (c *Client) GetMetricReportDefinition(ctx context.Context, id string) (*models.MetricReportDefinition, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.MetricReportDefinitions, "MetricReportDefinitions")
	if err != nil {
		return nil, err
	}
	return selectResource[models.MetricReportDefinition](ctx, c, uri, id)
}

func (c *Client) GetMetricReport(ctx context.Context, id string) (*models.MetricReport, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.MetricReports, "MetricReports")
	if err != nil {
		return nil, err
	}
	return selectResource[models.MetricReport](ctx, c, uri, id)
}

func (c *Client) GetTelemetryTrigger(ctx context.Context, id string) (*models.Triggers, error) {
	parent, err := c.GetTelemetryService(ctx)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(parent.Triggers, "Triggers")
	if err != nil {
		return nil, err
	}
	return selectResource[models.Triggers](ctx, c, uri, id)
}

func (c *Client) GetDrive(ctx context.Context, uri string) (*models.Drive, error) {
	return getResource[models.Drive](ctx, c, uri)
}

func (c *Client) GetPCIeDevice(ctx context.Context, uri string) (*models.PCIeDevice, error) {
	return getResource[models.PCIeDevice](ctx, c, uri)
}
