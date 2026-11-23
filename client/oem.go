package client

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

type Vendor string

const (
	VendorGeneric       Vendor = "generic"
	VendorAMI           Vendor = "ami"
	VendorZTE           Vendor = "zte"
	VendorInspur        Vendor = "inspur"
	VendorIEIT          Vendor = "ieit"
	VendorH3C           Vendor = "h3c"
	VendorXFusion       Vendor = "xfusion"
	VendorLenovo        Vendor = "lenovo"
	VendorNettrix       Vendor = "nettrix"
	VendorEnginetech    Vendor = "enginetech"
	VendorSmoothcompute Vendor = "smoothcompute"
)

// Extractor isolates OEM readings and resource layouts from the standard APIs.
type Extractor interface {
	InletHistoryURI(*models.Thermal) (string, error)
	FanSpeedRatio(*models.Fan) *float64
	DriveTemperature(*models.Drive) *float64
}

type UpdateStrategy interface {
	BuildBody(UpdateRequest) (Object, error)
}

type LogCollectStrategy interface {
	BuildBody(DiagnosticOptions) (Object, error)
	SelectService([]models.Log) (*models.Log, error)
}

// ProviderRegistry is client-scoped. Registering a provider never changes another client.
type ProviderRegistry struct {
	mu         sync.RWMutex
	extractors map[Vendor]Extractor
	updates    map[Vendor]UpdateStrategy
	logs       map[Vendor]LogCollectStrategy
}

func NewProviderRegistry() *ProviderRegistry {
	r := &ProviderRegistry{extractors: map[Vendor]Extractor{}, updates: map[Vendor]UpdateStrategy{}, logs: map[Vendor]LogCollectStrategy{}}
	for _, vendor := range []Vendor{VendorGeneric, VendorAMI, VendorZTE, VendorInspur, VendorIEIT, VendorH3C, VendorXFusion, VendorLenovo, VendorNettrix, VendorEnginetech, VendorSmoothcompute} {
		r.extractors[vendor] = vendorExtractor{vendor: vendor}
		r.updates[vendor] = vendorUpdate{vendor: vendor}
		r.logs[vendor] = vendorLogCollect{vendor: vendor}
	}
	r.updates[VendorIEIT] = vendorUpdate{vendor: VendorGeneric}
	r.extractors[VendorIEIT] = vendorExtractor{vendor: VendorGeneric}
	r.logs[VendorAMI] = vendorLogCollect{vendor: VendorGeneric}
	r.logs[VendorNettrix] = vendorLogCollect{vendor: VendorGeneric}
	return r
}

func (r *ProviderRegistry) Clone() *ProviderRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := NewProviderRegistry()
	for k, v := range r.extractors {
		out.extractors[k] = v
	}
	for k, v := range r.updates {
		out.updates[k] = v
	}
	for k, v := range r.logs {
		out.logs[k] = v
	}
	return out
}

func (r *ProviderRegistry) RegisterExtractor(vendor Vendor, provider Extractor) error {
	if provider == nil {
		return errors.New("extractor must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extractors[vendor] = provider
	return nil
}

func (r *ProviderRegistry) RegisterUpdate(vendor Vendor, provider UpdateStrategy) error {
	if provider == nil {
		return errors.New("update strategy must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates[vendor] = provider
	return nil
}

func (r *ProviderRegistry) RegisterLogCollect(vendor Vendor, provider LogCollectStrategy) error {
	if provider == nil {
		return errors.New("log strategy must not be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs[vendor] = provider
	return nil
}

func (r *ProviderRegistry) Extractor(vendor Vendor) Extractor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.extractors[vendor]; p != nil {
		return p
	}
	return r.extractors[VendorGeneric]
}

func (r *ProviderRegistry) Update(vendor Vendor) UpdateStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.updates[vendor]; p != nil {
		return p
	}
	return r.updates[VendorGeneric]
}

func (r *ProviderRegistry) LogCollect(vendor Vendor) LogCollectStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.logs[vendor]; p != nil {
		return p
	}
	return r.logs[VendorGeneric]
}

func (c *Client) Providers() *ProviderRegistry { return c.cfg.providers }

func DetectVendor(manufacturer string, oem *models.Oem) Vendor {
	value := strings.ToLower(manufacturer)
	variants := []struct {
		vendor   Vendor
		keywords []string
	}{
		{vendor: VendorEnginetech, keywords: []string{"enginetech", "\u5b89\u64ce"}}, {vendor: VendorInspur, keywords: []string{"inspur", "maginfra", "\u6d6a\u6f6e"}}, {vendor: VendorIEIT, keywords: []string{"ieit systems"}}, {vendor: VendorZTE, keywords: []string{"zte", "\u4e2d\u5174"}}, {vendor: VendorH3C, keywords: []string{"h3c", "\u65b0\u534e\u4e09"}}, {vendor: VendorNettrix, keywords: []string{"nettrix", "\u5b81\u7545"}}, {vendor: VendorXFusion, keywords: []string{"xfusion", "huawei", "\u8d85\u805a\u53d8"}}, {vendor: VendorLenovo, keywords: []string{"lenovo", "\u8054\u60f3"}}, {vendor: VendorSmoothcompute, keywords: []string{"smoothcompute", "\u987a\u7b97"}}, {vendor: VendorAMI, keywords: []string{"ami", "megarac"}},
	}
	for _, variant := range variants {
		for _, keyword := range variant.keywords {
			if strings.Contains(value, keyword) {
				return variant.vendor
			}
		}
	}
	if oem != nil {
		for key := range *oem {
			for _, variant := range variants {
				if strings.EqualFold(key, string(variant.vendor)) {
					return variant.vendor
				}
			}
		}
	}
	return VendorGeneric
}

func (c *Client) vendor(ctx context.Context) (Vendor, error) {
	if c.cfg.vendor != "" {
		return c.cfg.vendor, nil
	}
	system, err := c.GetSystem(ctx)
	if err != nil {
		if errors.Is(err, sdkerrors.ErrNotFound) {
			root, rootErr := c.Root(ctx)
			if rootErr != nil {
				return VendorGeneric, rootErr
			}
			return DetectVendor(pointer.Value(root.Vendor), root.OEM), nil
		}
		return VendorGeneric, err
	}
	return DetectVendor(pointer.Value(system.Manufacturer), system.OEM), nil
}

type vendorExtractor struct{ vendor Vendor }

func (p vendorExtractor) InletHistoryURI(thermal *models.Thermal) (string, error) {
	if p.vendor == VendorAMI {
		if pointer.Value(thermal.ODataID) == "" {
			return "", errors.New("Thermal is missing @odata.id")
		}
		return strings.TrimRight(pointer.Value(thermal.ODataID), "/") + "/Oem/Ami/InletHistoryTemperature", nil
	}
	return requireLink(thermal.InletHistoryTemperature, "InletHistoryTemperature")
}

func (p vendorExtractor) FanSpeedRatio(fan *models.Fan) *float64 {
	if fan.SpeedPercent != nil && fan.SpeedPercent.Reading != nil {
		return clonePointer(fan.SpeedPercent.Reading)
	}
	derive := func() *float64 {
		if fan.Reading != nil && fan.MaxReadingRange != nil && *fan.MaxReadingRange > 0 {
			return pointer.Ptr(math.Round(float64(*fan.Reading)/float64(*fan.MaxReadingRange)*1000) / 10)
		}
		return nil
	}
	switch p.vendor {
	case VendorXFusion:
		return oemNumber(fan.OEM, wire.OEMNamespaceXFusion, "SpeedRatio")
	case VendorZTE, VendorH3C:
		return oemNumber(fan.OEM, wire.FieldPublic, "SpeedRatio")
	case VendorInspur, VendorIEIT:
		primary := oemNumber(fan.OEM, wire.OEMNamespaceCustom, "SpeedRatio")
		if primary != nil && *primary != 0 {
			return primary
		}
		fallback := oemNumber(fan.OEM, wire.FieldPublic, "SpeedRatio")
		if fallback != nil {
			return fallback
		}
		return primary
	case VendorNettrix:
		ratio := oemNumber(fan.OEM, wire.FieldPublic, "SpeedRatio")
		if ratio != nil && *ratio != 0 {
			return ratio
		}
		return derive()
	default:
		return derive()
	}
}

func (p vendorExtractor) DriveTemperature(drive *models.Drive) *float64 {
	switch p.vendor {
	case VendorXFusion:
		return oemNumber(drive.OEM, wire.OEMNamespaceXFusion, "TemperatureCelsius")
	case VendorZTE, VendorH3C:
		return oemNumber(drive.OEM, wire.FieldPublic, "TemperatureCelsius")
	case VendorInspur, VendorIEIT, VendorLenovo:
		return oemNumber(drive.OEM, wire.FieldPublic, "temperature")
	case VendorNettrix:
		return drive.DiskTemperatureCelsius
	default:
		return nil
	}
}

func oemNumber(oem *models.Oem, namespace, key string) *float64 {
	if oem == nil {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal((*oem)[namespace], &fields) != nil {
		return nil
	}
	var number float64
	if json.Unmarshal(fields[key], &number) == nil {
		return &number
	}
	var text string
	if json.Unmarshal(fields[key], &text) == nil {
		if json.Unmarshal([]byte(text), &number) == nil {
			return &number
		}
	}
	return nil
}

func (c *Client) GetInletHistoryTemperature(ctx context.Context, chassisID ...string) (*models.InletHistoryTemperature, error) {
	thermal, err := c.GetThermal(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	vendor := c.cfg.vendor
	if vendor == "" {
		vendor = VendorGeneric
		if thermal.OEM != nil {
			var marker map[string]json.RawMessage
			if json.Unmarshal((*thermal.OEM)[wire.OEMNamespaceAMI], &marker) == nil && marker != nil {
				vendor = VendorAMI
			}
		}
	}
	uri, err := c.cfg.providers.Extractor(vendor).InletHistoryURI(thermal)
	if errors.Is(err, sdkerrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := getResource[models.InletHistoryTemperature](ctx, c, uri)
	if sdkerrors.IsStatus(err, 404) {
		return nil, nil
	}
	return result, err
}
