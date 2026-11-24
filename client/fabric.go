package client

import (
	"context"
	"errors"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func (c *Client) GetPorts(ctx context.Context, ownerURI string) ([]models.Port, error) {
	var owner struct {
		Ports *models.Link `json:"Ports"`
	}
	if err := c.Get(ctx, ownerURI, &owner); err != nil {
		return nil, err
	}
	return linkedCollection[models.Port](ctx, c, owner.Ports, "Ports")
}

func (c *Client) GetPort(ctx context.Context, id, ownerURI string) (*models.Port, error) {
	ports, err := c.GetPorts(ctx, ownerURI)
	if err != nil {
		return nil, err
	}
	return selectItem(ports, id)
}

func (c *Client) GetProcessorPorts(ctx context.Context, processorID string, systemID ...string) ([]models.Port, error) {
	processors, err := c.GetProcessors(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if processorID != "" {
		selected, err := selectItem(processors, processorID)
		if err != nil {
			return nil, err
		}
		processors = []models.Processor{*selected}
	}
	var result []models.Port
	advertised := false
	for _, processor := range processors {
		if processor.Ports != nil {
			advertised = true
			ports, err := linkedCollection[models.Port](ctx, c, processor.Ports, "Ports")
			if err != nil {
				return nil, err
			}
			result = append(result, ports...)
		}
	}
	if !advertised {
		return nil, sdkerrors.ErrNotFound
	}
	return uniqueResources(result), nil
}

func uniqueResources[T identified](items []T) []T {
	out := make([]T, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		key := item.ResourceURI()
		if key != "" && seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func (c *Client) GetConnectedPCIePorts(ctx context.Context, deviceID string, chassisID ...string) ([]models.Port, error) {
	devices, err := c.GetPCIeDevices(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	device, err := selectItem(devices, deviceID)
	if err != nil {
		return nil, err
	}
	if device.Links == nil || len(device.Links.ConnectedPCIePorts) == 0 {
		return nil, sdkerrors.ErrNotFound
	}
	return listLinks[models.Port](ctx, c, device.Links.ConnectedPCIePorts)
}

type PortSelector struct{ PortID, ProcessorID, PCIeDeviceID, OwnerURI, SystemID, ChassisID string }

func (c *Client) selectPort(ctx context.Context, selector PortSelector) (*models.Port, error) {
	count := 0
	for _, value := range []string{selector.ProcessorID, selector.PCIeDeviceID, selector.OwnerURI} {
		if value != "" {
			count++
		}
	}
	if count > 1 {
		return nil, errors.New("specify only one port owner")
	}
	var ports []models.Port
	var err error
	switch {
	case selector.OwnerURI != "":
		ports, err = c.GetPorts(ctx, selector.OwnerURI)
	case selector.PCIeDeviceID != "":
		ports, err = c.GetConnectedPCIePorts(ctx, selector.PCIeDeviceID, selector.ChassisID)
	default:
		ports, err = c.GetProcessorPorts(ctx, selector.ProcessorID, selector.SystemID)
	}
	if err != nil {
		return nil, err
	}
	return selectItem(ports, selector.PortID)
}

func (c *Client) GetPortMetrics(ctx context.Context, selector PortSelector) (*models.PortMetrics, error) {
	port, err := c.selectPort(ctx, selector)
	if err != nil {
		return nil, err
	}
	return linkedResource[models.PortMetrics](ctx, c, port.Metrics, "Metrics")
}

func (c *Client) ResetPortTransceivers(ctx context.Context, portID, ownerURI string) (*models.RedfishResponse, error) {
	port, err := c.GetPort(ctx, portID, ownerURI)
	if err != nil {
		return nil, err
	}
	if port.Actions == nil || port.Actions.ResetTransceivers == nil {
		return nil, errors.New("port does not advertise ResetTransceivers")
	}
	return c.postOperation(ctx, models.Value(port.Actions.ResetTransceivers.Target), Object{})
}

func (c *Client) GetCXLLogicalDevices(ctx context.Context, deviceID string, chassisID ...string) ([]models.CXLLogicalDevice, error) {
	devices, err := c.GetPCIeDevices(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	device, err := selectItem(devices, deviceID)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.CXLLogicalDevice](ctx, c, device.CXLLogicalDevices, "CXLLogicalDevices")
}

func (c *Client) GetPortCXLLogicalDevices(ctx context.Context, selector PortSelector) ([]models.CXLLogicalDevice, error) {
	port, err := c.selectPort(ctx, selector)
	if err != nil {
		return nil, err
	}
	if port.Links == nil || len(port.Links.CXLLogicalDevices) == 0 {
		return nil, sdkerrors.ErrNotFound
	}
	return listLinks[models.CXLLogicalDevice](ctx, c, port.Links.CXLLogicalDevices)
}
