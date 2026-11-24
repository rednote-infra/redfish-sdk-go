package client

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
)

type alternateLogResource struct {
	provider.DefaultLogCollectStrategy
}

func (alternateLogResource) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{ManagerServiceSuffix: "/VendorResource", CollectAction: "CreateBundle", DownloadAction: "FetchBundle"}
}
func TestVendorResourcePolicyControlsDiscoveryAndDownload(t *testing.T) {
	registry := NewProviderRegistry()
	const vendor Vendor = "alternate-vendor"
	if err := registry.RegisterLogCollect(vendor, alternateLogResource{}); err != nil {
		t.Fatal(err)
	}
	var reads, collects, downloads atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
		case "/managers":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/manager"}]}`)
		case "/manager":
			fmt.Fprint(w, `{"@odata.id":"/manager"}`)
		case "/manager/VendorResource":
			reads.Add(1)
			fmt.Fprint(w, `{"Actions":{"#Vendor.CreateBundle":{"target":"/create"},"#Vendor.FetchBundle":{"target":"/fetch"}}}`)
		case "/create":
			collects.Add(1)
			if r.Method != http.MethodPost {
				t.Error("wrong collection method")
			}
			fmt.Fprint(w, `{"TaskState":"Completed"}`)
		case "/fetch":
			downloads.Add(1)
			if r.Method != http.MethodPost {
				t.Error("wrong download method")
			}
			fmt.Fprint(w, "synthetic-bundle")
		default:
			http.NotFound(w, r)
		}
	}, WithVendor(vendor), WithProviders(registry))
	data, err := c.CollectAndDownloadDiagnosticDataBytes(context.Background(), CollectDownloadOptions{DisableReuse: true})
	if err != nil || string(data) != "synthetic-bundle" || reads.Load() != 1 || collects.Load() != 1 || downloads.Load() != 1 {
		t.Fatalf("resource policy: %q %v reads=%d collects=%d downloads=%d", data, err, reads.Load(), collects.Load(), downloads.Load())
	}
}
