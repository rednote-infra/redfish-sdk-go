package redfish_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	redfish "github.com/rednote-infra/redfish-sdk-go"
	"github.com/rednote-infra/redfish-sdk-go/client"
	"github.com/rednote-infra/redfish-sdk-go/discovery"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/response"
)

var (
	_ *client.Client                         = (*redfish.Client)(nil)
	_ *response.Response                     = (*redfish.Response)(nil)
	_ *sdkerrors.Error                       = (*redfish.Error)(nil)
	_ *sdkerrors.TransportError              = (*redfish.TransportError)(nil)
	_ *discovery.DiscoveredService           = (*redfish.DiscoveredService)(nil)
	_ client.CollectionResult[models.System] = redfish.CollectionResult[models.System]{}
)

func TestLegacyAndClientImportsShareAPIAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"message":" Resource is unavailable. "}}`)
	}))
	t.Cleanup(server.Close)
	oldClient, err := redfish.NewClient(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { oldClient.Close() })
	newClient, err := client.NewClient(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { newClient.Close() })
	for _, sdk := range []*client.Client{oldClient, newClient} {
		_, err := sdk.GetRaw(context.Background(), "/missing")
		var legacyError *redfish.Error
		var sharedError *sdkerrors.Error
		if !errors.As(err, &legacyError) || !errors.As(err, &sharedError) {
			t.Fatalf("error identity changed: %T", err)
		}
		if !errors.Is(err, redfish.ErrNotFound) || !errors.Is(err, sdkerrors.ErrNotFound) {
			t.Fatalf("sentinel identity changed: %v", err)
		}
		if err.Error() != " Resource is unavailable. " {
			t.Fatalf("downstream message changed: %q", err.Error())
		}
	}
	if redfish.AuthenticationBasic != client.AuthenticationBasic || redfish.VendorAMI != client.VendorAMI {
		t.Fatal("public constants changed")
	}
	if redfish.Value(redfish.Ptr(false)) {
		t.Fatal("legacy generic helpers changed")
	}
}
