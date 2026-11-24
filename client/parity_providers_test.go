package client

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem"
)

func TestProviderDiscoveryIsLazyAndLoadsSelectedFactoryOnce(t *testing.T) {
	var discovered, loaded atomic.Int32
	provider := oem.NewExtractor(VendorAMI)
	registry := NewProviderRegistryWithOptions(ProviderRegistryOptions{Discover: func() ([]ProviderRegistration, error) {
		discovered.Add(1)
		return []ProviderRegistration{{Category: OEMExtractor, Vendor: "CUSTOM", LoadExtractor: func() (Extractor, error) { loaded.Add(1); return provider, nil }}}, nil
	}})
	if discovered.Load() != 0 {
		t.Fatal("provider discovery was eager")
	}
	if _, err := registry.ResolveExtractor(VendorAMI); err != nil || discovered.Load() != 0 {
		t.Fatal("built-in lookup should not discover plugins")
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := registry.ResolveExtractor("custom"); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if discovered.Load() != 1 || loaded.Load() != 1 {
		t.Fatal("provider discovery/factory ran more than once")
	}
	if err := registry.RegisterExtractor("custom", provider); err != nil {
		t.Fatal(err)
	}
	var conflict *sdkerrors.ProviderConflictError
	if err := registry.RegisterExtractor("CUSTOM", provider); !errors.As(err, &conflict) {
		t.Fatalf("duplicate explicit provider: %v", err)
	}
}

func TestProviderConflictsAndFailuresAreIsolated(t *testing.T) {
	load := func() (Extractor, error) { t.Error("conflicting factory must not execute"); return nil, nil }
	registry := NewProviderRegistryWithOptions(ProviderRegistryOptions{Discover: func() ([]ProviderRegistration, error) {
		return []ProviderRegistration{{Category: OEMExtractor, Vendor: "custom", LoadExtractor: load}, {Category: OEMExtractor, Vendor: "custom", LoadExtractor: load}}, nil
	}})
	var conflict *sdkerrors.ProviderConflictError
	if _, err := registry.ResolveExtractor("custom"); !errors.As(err, &conflict) {
		t.Fatalf("discovery conflict: %v", err)
	}
	if _, err := registry.ResolveExtractor(VendorGeneric); err != nil {
		t.Fatal("one vendor conflict affected a built-in")
	}
	cause := errors.New("provider initialization failed")
	failed := NewProviderRegistryWithOptions(ProviderRegistryOptions{Discover: func() ([]ProviderRegistration, error) { return nil, cause }})
	var loadError *sdkerrors.ProviderLoadError
	if _, err := failed.ResolveExtractor("unknown"); !errors.As(err, &loadError) || !errors.Is(err, cause) {
		t.Fatalf("factory error identity lost: %v", err)
	}
}

type completeUpdateProvider struct{ called *bool }

func (p completeUpdateProvider) BuildBody(UpdateRequest) (Object, error) {
	return nil, errors.New("body builder must not execute")
}
func (p completeUpdateProvider) Execute(context.Context, *Client, UpdateRequest) (*models.RedfishResponse, error) {
	*p.called = true
	return &models.RedfishResponse{HTTPStatus: models.Ptr(int64(http.StatusAccepted))}, nil
}

func TestUpdateProviderCanReplaceCompleteWorkflow(t *testing.T) {
	called := false
	registry := NewProviderRegistry()
	if err := registry.RegisterUpdate("custom", completeUpdateProvider{called: &called}); err != nil {
		t.Fatal(err)
	}
	sdk, err := NewClient("https://example.invalid", "", "", WithProviders(registry))
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	result, err := sdk.SimpleUpdate(context.Background(), UpdateRequest{Vendor: "custom"})
	if err != nil || !called || models.Value(result.HTTPStatus) != http.StatusAccepted {
		t.Fatalf("custom lifecycle was ignored: %v", err)
	}
}

func TestProviderOverridePriorityIsExplicitAndClientScoped(t *testing.T) {
	discovered := func() ([]ProviderRegistration, error) {
		return []ProviderRegistration{{Category: OEMExtractor, Vendor: VendorAMI, LoadExtractor: func() (Extractor, error) { return oem.NewExtractor(VendorGeneric), nil }}}, nil
	}
	registry := NewProviderRegistryWithOptions(ProviderRegistryOptions{Discover: discovered, AllowBuiltinOverrides: true})
	copy := registry.Clone()
	provider, err := registry.ResolveExtractor(VendorAMI)
	if err != nil {
		t.Fatal(err)
	}
	thermal := &models.Thermal{ODataID: models.Ptr("/thermal"), InletHistoryTemperature: &models.Link{ODataID: models.Ptr("/legacy")}}
	if uri, err := provider.InletHistoryURI(thermal); err != nil || uri != "/legacy" {
		t.Fatal("discovered provider did not override built-in")
	}
	if err := registry.RegisterExtractor(VendorAMI, oem.NewExtractor(VendorAMI)); err != nil {
		t.Fatal(err)
	}
	provider, err = registry.ResolveExtractor(VendorAMI)
	if err != nil {
		t.Fatal(err)
	}
	if uri, _ := provider.InletHistoryURI(thermal); uri == "/legacy" {
		t.Fatal("explicit provider did not take precedence")
	}
	other, err := copy.ResolveExtractor(VendorAMI)
	if err != nil {
		t.Fatal(err)
	}
	if uri, _ := other.InletHistoryURI(thermal); uri != "/legacy" {
		t.Fatal("registration leaked into a cloned registry")
	}
}
