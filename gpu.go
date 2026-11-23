package redfish

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

// GPU discovery merges representations using topology links and usable serial numbers.
func (c *Client) GetGPUs(ctx context.Context, systemID ...string) ([]models.Gpu, error) {
	id, err := optionalID(systemID)
	if err != nil {
		return nil, err
	}
	var systems []models.System
	if id != "" {
		system, err := c.GetSystem(ctx, id)
		if err != nil {
			return nil, err
		}
		systems = []models.System{*system}
	} else {
		systems, err = c.GetSystems(ctx)
		if err != nil {
			return nil, err
		}
	}
	type candidate struct {
		gpu models.Gpu
		ids map[string]bool
	}
	var candidates []candidate
	add := func(gpu models.Gpu, source string, links []models.Link) {
		identities := map[string]bool{}
		uri := Value(gpu.ODataID)
		if uri != "" {
			identities[uri] = true
		}
		for _, link := range links {
			if id := Value(link.ODataID); id != "" {
				identities[id] = true
			}
		}
		if gpu.OEM != nil {
			serial := strings.ToLower(strings.TrimSpace(Value(gpu.OEM.SerialNumber)))
			if serial != "" && serial != "n/a" && serial != "not available" {
				identities["serial:"+serial] = true
			}
		}
		gpu.SourceTypes = []string{source}
		if uri != "" {
			gpu.SourceODataIDs = []string{uri}
		}
		selected := -1
		for i := 0; i < len(candidates); i++ {
			overlap := false
			for id := range identities {
				if candidates[i].ids[id] {
					overlap = true
					break
				}
			}
			if !overlap {
				continue
			}
			if selected < 0 {
				selected = i
				continue
			}
			other := candidates[i]
			mergeGPU(&candidates[selected].gpu, other.gpu)
			for id := range other.ids {
				identities[id] = true
			}
			candidates = append(candidates[:i], candidates[i+1:]...)
			i--
		}
		if selected < 0 {
			candidates = append(candidates, candidate{gpu, identities})
		} else {
			mergeGPU(&candidates[selected].gpu, gpu)
			for id := range identities {
				candidates[selected].ids[id] = true
			}
		}
	}
	enrich := func(gpu *models.Gpu, link *models.Link) error {
		if link == nil {
			return nil
		}
		metrics, err := linkedResource[models.EnvironmentMetrics](ctx, c, link, "EnvironmentMetrics")
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		}
		if metrics.PowerWatts != nil && metrics.PowerWatts.Reading != nil {
			gpu.PowerWatts = Ptr(fmt.Sprint(*metrics.PowerWatts.Reading))
		}
		return nil
	}
	devices := map[string]models.PCIeDevice{}
	for _, system := range systems {
		if system.GraphicsControllers != nil {
			controllers, err := linkedCollection[models.Gpu](ctx, c, system.GraphicsControllers, "GraphicsControllers")
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
			for _, gpu := range controllers {
				var links []models.Link
				if gpu.Links != nil {
					links = append(links, gpu.Links.PCIeFunctions...)
					links = append(links, gpu.Links.Processors...)
					if gpu.Links.PCIeDevice != nil {
						links = append(links, *gpu.Links.PCIeDevice)
					}
				}
				add(gpu, "GraphicsController", links)
			}
		}
		if system.Processors != nil {
			processors, err := linkedCollection[models.Processor](ctx, c, system.Processors, "Processors")
			if err != nil && !errors.Is(err, ErrNotFound) {
				return nil, err
			}
			for _, processor := range processors {
				kind := strings.ToLower(Value(processor.ProcessorType))
				if kind != "gpu" && !(kind == "accelerator" && strings.Contains(strings.ToLower(Value(processor.Name)+Value(processor.Model)), "gpu")) {
					continue
				}
				gpu := models.Gpu{ODataID: processor.ODataID, ID: processor.ID, Name: processor.Name, Manufacturer: processor.Manufacturer, Model: processor.Model, Version: processor.FirmwareVersion}
				if processor.SerialNumber != nil {
					gpu.OEM = &models.GpuOEM{SerialNumber: processor.SerialNumber}
				}
				if processor.TdpWatts != nil {
					gpu.PowerWatts = Ptr(fmt.Sprint(*processor.TdpWatts))
				}
				var links []models.Link
				if processor.Links != nil {
					links = append(links, processor.Links.PCIeFunctions...)
					if processor.Links.PCIeDevice != nil {
						links = append(links, *processor.Links.PCIeDevice)
					}
				}
				if err = enrich(&gpu, processor.EnvironmentMetrics); err != nil {
					return nil, err
				}
				if processor.AccelerationFunctions != nil {
					functions, err := linkedCollection[models.AccelerationFunction](ctx, c, processor.AccelerationFunctions, "AccelerationFunctions")
					if err != nil && !errors.Is(err, ErrNotFound) {
						return nil, err
					}
					for _, function := range functions {
						if function.ODataID != nil {
							links = append(links, models.Link{ODataID: function.ODataID})
						}
						if function.Links != nil {
							links = append(links, function.Links.PCIeFunctions...)
						}
						if gpu.Manufacturer == nil {
							gpu.Manufacturer = function.Manufacturer
						}
						if gpu.Version == nil {
							gpu.Version = function.Version
						}
						if gpu.PowerWatts == nil && function.PowerWatts != nil {
							gpu.PowerWatts = Ptr(fmt.Sprint(*function.PowerWatts))
						}
					}
				}
				add(gpu, "Processor", links)
			}
		}
		links := slices.Clone(system.PCIeDevices)
		if system.Links != nil {
			links = append(links, system.Links.PCIeDevices...)
		}
		for _, link := range links {
			device, err := linkedResource[models.PCIeDevice](ctx, c, &link, "PCIeDevice")
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return nil, err
			}
			devices[Value(device.ODataID)] = *device
		}
		if system.Links != nil {
			for _, chassisLink := range system.Links.Chassis {
				chassis, err := linkedResource[models.Chassis](ctx, c, &chassisLink, "Chassis")
				if err != nil {
					if errors.Is(err, ErrNotFound) {
						continue
					}
					return nil, err
				}
				items, err := c.GetPCIeDevices(ctx, Value(chassis.ODataID))
				if err != nil {
					if errors.Is(err, ErrNotFound) {
						continue
					}
					return nil, err
				}
				for _, device := range items {
					devices[Value(device.ODataID)] = device
				}
			}
		}
	}

	// Older BMCs omit the System-to-Chassis links but advertise chassis inventory at the root.
	needsChassisFallback := false
	for _, system := range systems {
		if system.Links == nil || len(system.Links.Chassis) == 0 {
			needsChassisFallback = true
			break
		}
	}
	if needsChassisFallback {
		chassis, err := c.GetChassisCollection(ctx)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		for _, item := range chassis {
			items, err := c.GetPCIeDevices(ctx, Value(item.ODataID))
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					continue
				}
				return nil, err
			}
			for _, device := range items {
				devices[Value(device.ODataID)] = device
			}
		}
	}
	keys := make([]string, 0, len(devices))
	for uri := range devices {
		keys = append(keys, uri)
	}
	slices.Sort(keys)
	for _, key := range keys {
		device := devices[key]
		isGPU := strings.EqualFold(Value(device.DeviceType), "gpu") || strings.Contains(strings.ToLower(Value(device.Name)), "gpu")
		var links []models.Link
		if device.Links != nil {
			links = append(links, device.Links.Processors...)
		}
		var functions []models.PCIeFunction
		if device.PCIeFunctions != nil {
			functions, err = linkedCollection[models.PCIeFunction](ctx, c, device.PCIeFunctions, "PCIeFunctions")
		} else if device.Links != nil {
			functions, err = listLinks[models.PCIeFunction](ctx, c, device.Links.PCIeFunctions)
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		for _, function := range functions {
			class := strings.ReplaceAll(strings.ToLower(Value(function.DeviceClass)), " ", "")
			if slices.Contains([]string{"displaycontroller", "processingaccelerators", "coprocessor"}, class) {
				isGPU = true
			}
			if function.ODataID != nil {
				links = append(links, models.Link{ODataID: function.ODataID})
			}
		}
		if !isGPU {
			continue
		}
		gpu := models.Gpu{ODataID: device.ODataID, ID: device.ID, Name: device.Name, Manufacturer: device.Manufacturer, Model: device.Model, Version: device.FirmwareVersion}
		if device.SerialNumber != nil {
			gpu.OEM = &models.GpuOEM{SerialNumber: device.SerialNumber}
		}
		if err = enrich(&gpu, device.EnvironmentMetrics); err != nil {
			return nil, err
		}
		add(gpu, "PCIeDevice", links)
	}
	out := make([]models.Gpu, 0, len(candidates))
	for _, candidate := range candidates {
		for id := range candidate.ids {
			if !strings.HasPrefix(id, "serial:") {
				candidate.gpu.RelatedResourceODataIDs = append(candidate.gpu.RelatedResourceODataIDs, id)
			}
		}
		slices.Sort(candidate.gpu.RelatedResourceODataIDs)
		out = append(out, candidate.gpu)
	}
	return out, nil
}
func mergeGPU(primary *models.Gpu, other models.Gpu) {
	if primary.Name == nil {
		primary.Name = other.Name
	}
	if primary.Manufacturer == nil {
		primary.Manufacturer = other.Manufacturer
	}
	if primary.Model == nil {
		primary.Model = other.Model
	}
	if primary.Version == nil {
		primary.Version = other.Version
	}
	if primary.PowerWatts == nil {
		primary.PowerWatts = other.PowerWatts
	}
	if primary.OEM == nil {
		primary.OEM = other.OEM
	}
	for _, source := range other.SourceTypes {
		if !slices.Contains(primary.SourceTypes, source) {
			primary.SourceTypes = append(primary.SourceTypes, source)
		}
	}
	for _, source := range other.SourceODataIDs {
		if !slices.Contains(primary.SourceODataIDs, source) {
			primary.SourceODataIDs = append(primary.SourceODataIDs, source)
		}
	}
}
