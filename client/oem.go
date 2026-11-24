package client

import (
	"context"
	"errors"
	"sync"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem"
)

type Vendor = provider.Vendor

const (
	VendorGeneric       = oem.VendorGeneric
	VendorAMI           = oem.VendorAMI
	VendorZTE           = oem.VendorZTE
	VendorInspur        = oem.VendorInspur
	VendorIEIT          = oem.VendorIEIT
	VendorH3C           = oem.VendorH3C
	VendorXFusion       = oem.VendorXFusion
	VendorLenovo        = oem.VendorLenovo
	VendorNettrix       = oem.VendorNettrix
	VendorEnginetech    = oem.VendorEnginetech
	VendorSmoothcompute = oem.VendorSmoothcompute
)

type Extractor = provider.Extractor
type UpdateStrategy = provider.UpdateStrategy
type LogCollectStrategy = provider.LogCollectStrategy

// Default strategies implement the standard Redfish resource behavior.
type DefaultExtractor = provider.DefaultExtractor
type DefaultUpdateStrategy = provider.DefaultUpdateStrategy
type DefaultLogCollectStrategy = provider.DefaultLogCollectStrategy

// ProviderRegistry is client-scoped. Registering a provider never changes another client.
type ProviderRegistry struct {
	loadMu         sync.Mutex
	options        ProviderRegistryOptions
	explicit       map[providerKey]bool
	discovered     bool
	discoveryError error
	entries        map[providerKey][]ProviderRegistration
	loaded         map[providerKey]providerInstance
	failures       map[providerKey]error
	mu             sync.RWMutex
	extractors     map[Vendor]Extractor
	updates        map[Vendor]UpdateStrategy
	logs           map[Vendor]LogCollectStrategy
}

func NewProviderRegistry() *ProviderRegistry {
	r := &ProviderRegistry{
		explicit:   map[providerKey]bool{},
		entries:    map[providerKey][]ProviderRegistration{},
		loaded:     map[providerKey]providerInstance{},
		failures:   map[providerKey]error{},
		extractors: map[Vendor]Extractor{},
		updates:    map[Vendor]UpdateStrategy{},
		logs:       map[Vendor]LogCollectStrategy{},
	}
	for _, vendor := range []Vendor{VendorGeneric, VendorAMI, VendorZTE, VendorInspur, VendorIEIT, VendorH3C, VendorXFusion, VendorLenovo, VendorNettrix, VendorEnginetech, VendorSmoothcompute} {
		r.extractors[vendor] = oem.NewExtractor(vendor)
		r.updates[vendor] = oem.NewUpdateStrategy(vendor)
		r.logs[vendor] = builtinLogCollect(vendor)
	}
	r.logs[VendorAMI] = builtinLogCollect(VendorGeneric)
	r.logs[VendorNettrix] = builtinLogCollect(VendorGeneric)
	return r
}

func (r *ProviderRegistry) Clone() *ProviderRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := NewProviderRegistryWithOptions(r.options)
	for key, value := range r.explicit {
		out.explicit[key] = value
	}
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
	vendor = providerVendor(vendor)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.explicitRegistration(OEMExtractor, vendor); err != nil {
		return err
	}
	r.extractors[vendor] = provider
	return nil
}

func (r *ProviderRegistry) RegisterUpdate(vendor Vendor, provider UpdateStrategy) error {
	if provider == nil {
		return errors.New("update strategy must not be nil")
	}
	vendor = providerVendor(vendor)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.explicitRegistration(OEMUpdate, vendor); err != nil {
		return err
	}
	r.updates[vendor] = provider
	return nil
}

func (r *ProviderRegistry) RegisterLogCollect(vendor Vendor, provider LogCollectStrategy) error {
	if provider == nil {
		return errors.New("log strategy must not be nil")
	}
	vendor = providerVendor(vendor)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.explicitRegistration(OEMLogCollect, vendor); err != nil {
		return err
	}
	r.logs[vendor] = provider
	return nil
}

// Extractor retains explicit/built-in lookup. ResolveExtractor also discovers optional providers.
func (r *ProviderRegistry) Extractor(vendor Vendor) Extractor {
	vendor = providerVendor(vendor)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.extractors[vendor]; p != nil {
		return p
	}
	return r.extractors[VendorGeneric]
}

// Update retains explicit/built-in lookup. ResolveUpdate also discovers optional providers.
func (r *ProviderRegistry) Update(vendor Vendor) UpdateStrategy {
	vendor = providerVendor(vendor)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.updates[vendor]; p != nil {
		return p
	}
	return r.updates[VendorGeneric]
}

// LogCollect retains explicit/built-in lookup. ResolveLogCollect also discovers optional providers.
func (r *ProviderRegistry) LogCollect(vendor Vendor) LogCollectStrategy {
	vendor = providerVendor(vendor)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p := r.logs[vendor]; p != nil {
		return p
	}
	return r.logs[VendorGeneric]
}

func (c *Client) Providers() *ProviderRegistry { return c.cfg.providers }

func DetectVendor(manufacturer string, extensions *models.Oem) Vendor {
	return provider.DetectVendor(manufacturer, extensions)
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
			return DetectVendor(models.Value(root.Vendor), root.OEM), nil
		}
		return VendorGeneric, err
	}
	return DetectVendor(models.Value(system.Manufacturer), system.OEM), nil
}

func (c *Client) GetInletHistoryTemperature(ctx context.Context, chassisID ...string) (*models.InletHistoryTemperature, error) {
	thermal, err := c.GetThermal(ctx, chassisID...)
	if err != nil {
		return nil, err
	}
	vendor := c.cfg.vendor
	if vendor == "" {
		vendor = provider.DetectThermalVendor(thermal)
	}
	provider, err := c.cfg.providers.ResolveExtractor(vendor)
	if err != nil {
		return nil, err
	}
	uri, err := provider.InletHistoryURI(thermal)
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
