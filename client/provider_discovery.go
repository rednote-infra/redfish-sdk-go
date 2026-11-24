package client

import (
	"context"
	"errors"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

type OEMCategory string

const (
	OEMExtractor  OEMCategory = "extractor"
	OEMUpdate     OEMCategory = "update"
	OEMLogCollect OEMCategory = "log_collect"
)

// ProviderRegistration exposes typed factories; only the selected vendor is initialized.
type ProviderRegistration struct {
	Category       OEMCategory
	Vendor         Vendor
	LoadExtractor  func() (Extractor, error)
	LoadUpdate     func() (UpdateStrategy, error)
	LoadLogCollect func() (LogCollectStrategy, error)
}

type ProviderDiscovery func() ([]ProviderRegistration, error)

type ProviderRegistryOptions struct {
	Discover              ProviderDiscovery
	AllowBuiltinOverrides bool
}

type providerKey struct {
	category OEMCategory
	vendor   Vendor
}

type providerInstance struct {
	extractor Extractor
	update    UpdateStrategy
	logs      LogCollectStrategy
}

// UpdateExecutor optionally replaces action discovery, request construction, and execution.
type UpdateExecutor interface {
	Execute(context.Context, *Client, UpdateRequest) (*models.RedfishResponse, error)
}

func providerVendor(vendor Vendor) Vendor {
	return Vendor(strings.ToLower(strings.TrimSpace(string(vendor))))
}

func NewProviderRegistryWithOptions(options ProviderRegistryOptions) *ProviderRegistry {
	registry := NewProviderRegistry()
	registry.options = options
	return registry
}

func (r *ProviderRegistry) explicitRegistration(category OEMCategory, vendor Vendor) error {
	if vendor == "" {
		return errors.New("OEM vendor must not be empty")
	}
	key := providerKey{category: category, vendor: vendor}
	if r.explicit[key] {
		return &sdkerrors.ProviderConflictError{Category: string(category), Vendor: string(vendor)}
	}
	r.explicit[key] = true
	return nil
}

func (r *ProviderRegistry) builtin(key providerKey) (providerInstance, bool) {
	result := providerInstance{extractor: r.extractors[key.vendor], update: r.updates[key.vendor], logs: r.logs[key.vendor]}
	switch key.category {
	case OEMExtractor:
		return result, result.extractor != nil
	case OEMUpdate:
		return result, result.update != nil
	case OEMLogCollect:
		return result, result.logs != nil
	}
	return result, false
}

// resolve serializes discovery and factory initialization; registration uses its own lock.
func (r *ProviderRegistry) resolve(category OEMCategory, vendor Vendor) (providerInstance, error) {
	vendor = providerVendor(vendor)
	if vendor == "" {
		return providerInstance{}, errors.New("OEM vendor must not be empty")
	}
	key := providerKey{category: category, vendor: vendor}
	r.loadMu.Lock()
	defer r.loadMu.Unlock()
	r.mu.RLock()
	builtin, known := r.builtin(key)
	explicit := r.explicit[key]
	options := r.options
	r.mu.RUnlock()
	if explicit || (known && !options.AllowBuiltinOverrides) {
		return builtin, nil
	}
	if !r.discovered {
		r.discovered = true
		if options.Discover != nil {
			registrations, err := options.Discover()
			r.discoveryError = err
			for _, registration := range registrations {
				registration.Vendor = providerVendor(registration.Vendor)
				discoveredKey := providerKey{category: registration.Category, vendor: registration.Vendor}
				r.entries[discoveredKey] = append(r.entries[discoveredKey], registration)
			}
		}
	}
	if r.discoveryError != nil {
		if known {
			return builtin, nil
		}
		return providerInstance{}, &sdkerrors.ProviderLoadError{Category: string(category), Vendor: string(vendor), Cause: r.discoveryError}
	}
	if err := r.failures[key]; err != nil {
		return providerInstance{}, err
	}
	if cached, ok := r.loaded[key]; ok {
		return cached, nil
	}
	registrations := r.entries[key]
	if len(registrations) > 1 {
		err := &sdkerrors.ProviderConflictError{Category: string(category), Vendor: string(vendor)}
		r.failures[key] = err
		return providerInstance{}, err
	}
	if len(registrations) == 1 {
		registration := registrations[0]
		var result providerInstance
		var err error
		switch category {
		case OEMExtractor:
			if registration.LoadExtractor != nil {
				result.extractor, err = registration.LoadExtractor()
			}
			if err == nil && result.extractor == nil {
				err = errors.New("extractor factory returned no provider")
			}
		case OEMUpdate:
			if registration.LoadUpdate != nil {
				result.update, err = registration.LoadUpdate()
			}
			if err == nil && result.update == nil {
				err = errors.New("update factory returned no provider")
			}
		case OEMLogCollect:
			if registration.LoadLogCollect != nil {
				result.logs, err = registration.LoadLogCollect()
			}
			if err == nil && result.logs == nil {
				err = errors.New("log factory returned no provider")
			}
		}
		if err != nil {
			failure := &sdkerrors.ProviderLoadError{Category: string(category), Vendor: string(vendor), Cause: err}
			r.failures[key] = failure
			return providerInstance{}, failure
		}
		// A factory can register an explicit provider; explicit precedence still wins.
		r.mu.RLock()
		explicit = r.explicit[key]
		override, _ := r.builtin(key)
		r.mu.RUnlock()
		if explicit {
			return override, nil
		}
		r.loaded[key] = result
		return result, nil
	}
	if known {
		return builtin, nil
	}
	r.mu.RLock()
	fallback, _ := r.builtin(providerKey{category: category, vendor: VendorGeneric})
	r.mu.RUnlock()
	return fallback, nil
}

func (r *ProviderRegistry) ResolveExtractor(vendor Vendor) (Extractor, error) {
	instance, err := r.resolve(OEMExtractor, vendor)
	return instance.extractor, err
}

func (r *ProviderRegistry) ResolveUpdate(vendor Vendor) (UpdateStrategy, error) {
	instance, err := r.resolve(OEMUpdate, vendor)
	return instance.update, err
}

func (r *ProviderRegistry) ResolveLogCollect(vendor Vendor) (LogCollectStrategy, error) {
	instance, err := r.resolve(OEMLogCollect, vendor)
	return instance.logs, err
}
