package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

func TestZTEDiagnosticLifecycleRejectsStaleArchive(t *testing.T) {
	progress := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
		case "/managers":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/managers/self","LogServices":{"@odata.id":"/logs"}}]}`)
		case "/logs":
			fmt.Fprint(w, `{"Members":[],"Actions":{"Oem":{"#LogServices.Dump":{"target":"/dump"},"#LogServices.ExportLogs":{"target":"/export"}}}}`)
		case "/dump/Progress":
			progress++
			if progress <= 2 {
				fmt.Fprint(w, `{"State":"STATE_COMPLETED","Percentage":"100","TarPath":"/old.tgz"}`)
			} else if progress == 3 {
				fmt.Fprint(w, `{"State":"STATE_RUNNING","Percentage":"50","TarPath":""}`)
			} else {
				fmt.Fprint(w, `{"State":"STATE_COMPLETED","Percentage":"100","TarPath":"/new.tgz","Type":"AllLogs"}`)
			}
		case "/dump":
			var body Object
			json.NewDecoder(r.Body).Decode(&body)
			if body["Type"] != "AllLogs" {
				t.Error("wrong dump type")
			}
			fmt.Fprint(w, `{}`)
		case "/export":
			var body Object
			json.NewDecoder(r.Body).Decode(&body)
			if body["Content"] != "/new.tgz" {
				t.Errorf("stale archive: %v", body)
			}
			fmt.Fprint(w, "new archive")
		default:
			http.NotFound(w, r)
		}
	}, WithVendor(VendorZTE))
	path := filepath.Join(t.TempDir(), "zte.tgz")
	actual, err := c.CollectAndDownloadDiagnosticData(context.Background(), path, CollectDownloadOptions{Poll: PollOptions{Interval: time.Millisecond, Timeout: time.Second}, DisableReuse: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(actual)
	if err != nil || string(data) != "new archive" {
		t.Fatalf("archive: %s %v", data, err)
	}
}

func TestVendorDiagnosticBodies(t *testing.T) {
	for _, vendor := range []Vendor{VendorGeneric, VendorEnginetech, VendorInspur, VendorLenovo, VendorZTE, VendorH3C} {
		t.Run(string(vendor), func(t *testing.T) {
			body, err := NewProviderRegistry().LogCollect(vendor).BuildBody(DiagnosticOptions{})
			if err != nil {
				t.Fatal(err)
			}
			switch vendor {
			case VendorInspur, VendorLenovo:
				if len(body) != 0 {
					t.Error("CollectAllLog must send empty body")
				}
			case VendorZTE:
				if body["Type"] != "AllLogs" {
					t.Error("wrong ZTE body")
				}
			case VendorH3C:
				if body["Type"] != "URI" {
					t.Error("wrong H3C body")
				}
			case VendorEnginetech:
				if body["OEMDiagnosticDataType"] != "BlackBox" {
					t.Error("missing BlackBox type")
				}
			default:
				if body["DiagnosticDataType"] != "Manager" {
					t.Error("wrong standard body")
				}
			}
		})
	}
}

func TestStandardDiagnosticLifecycleFollowsTaskAndLogEntry(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
		case "/managers":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/manager","LogServices":{"@odata.id":"/logs"}}]}`)
		case "/logs":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/logs/diag"}]}`)
		case "/logs/diag":
			fmt.Fprint(w, `{"@odata.id":"/logs/diag","Actions":{"#LogService.CollectDiagnosticData":{"target":"/collect"}}}`)
		case "/collect":
			w.Header().Set(wire.HeaderLocation, "/monitor")
			w.WriteHeader(202)
			fmt.Fprint(w, `{"@odata.id":"/task/1","TaskState":"Running"}`)
		case "/monitor", "/task/1":
			fmt.Fprint(w, `{"@odata.id":"/task/1","TaskState":"Completed","Links":{"CreatedResources":[{"@odata.id":"/entries/1"}]}}`)
		case "/entries/1":
			fmt.Fprint(w, `{"@odata.id":"/entries/1","AdditionalDataURI":"/bundle"}`)
		case "/bundle":
			fmt.Fprint(w, "standard archive")
		default:
			http.NotFound(w, r)
		}
	}, WithVendor(VendorGeneric))
	path := filepath.Join(t.TempDir(), "bundle.tgz")
	actual, err := c.CollectAndDownloadDiagnosticData(context.Background(), path, CollectDownloadOptions{DisableReuse: true, Poll: PollOptions{Interval: time.Millisecond, Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(actual)
	if err != nil || string(data) != "standard archive" {
		t.Fatalf("bad standard artifact: %q %v", data, err)
	}
}
