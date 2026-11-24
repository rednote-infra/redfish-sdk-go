package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

func TestConditionalTypedRead(t *testing.T) {
	var reads atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if reads.Add(1) > 1 {
			if r.Header.Get("If-None-Match") != "v1" {
				t.Error("missing validator")
			}
			w.Header().Set("ETag", "v2")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "v1")
		fmt.Fprint(w, `{"Id":"one","Name":"cached"}`)
	})
	first, err := Fetch[models.System](context.Background(), c, "/system", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fetch(context.Background(), c, "/system", first)
	if err != nil || !second.NotModified || second.ETag != "v2" || models.Value(second.Value.Name) != "cached" {
		t.Fatalf("conditional read: %+v %v", second, err)
	}
}

func TestBoundedCollectionConcurrency(t *testing.T) {
	var active, peak atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collection" {
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/a"},{"@odata.id":"/b"},{"@odata.id":"/c"}]}`)
			return
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		fmt.Fprintf(w, `{"@odata.id":%q,"Id":%q}`, r.URL.Path, r.URL.Path[1:])
	}, WithCollectionConcurrency(2))
	var result []models.System
	if err := c.GetCollection(context.Background(), "/collection", &result); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 || len(result) != 3 || result[0].ResourceID() != "a" || result[2].ResourceID() != "c" {
		t.Fatalf("peak=%d result=%v", peak.Load(), result)
	}
}

func TestRegistryUnsignedValidation(t *testing.T) {
	var registry models.AttributeRegistry
	if err := json.Unmarshal([]byte(`{"RegistryEntries":{"Attributes":[{"AttributeName":"Counter","Type":"Integer","LowerBound":0,"UpperBound":18446744073709551615}]}}`), &registry); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAttributeValues(&registry, Object{"Counter": uint64(^uint64(0))}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAttributeValues(&registry, Object{"Counter": int64(-1)}); err == nil {
		t.Fatal("accepted out of range value")
	}
}

func interopResources(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/redfish/v1/":
		fmt.Fprint(w, `{"Systems":{"@odata.id":"/systems"},"Managers":{"@odata.id":"/managers"}}`)
	case "/systems":
		fmt.Fprint(w, `{"Members":[{"@odata.id":"/system"}]}`)
	case "/managers":
		fmt.Fprint(w, `{"Members":[{"@odata.id":"/manager"}]}`)
	default:
		return false
	}
	return true
}

func TestBIOSSkipUnchangedAndStrictCapabilities(t *testing.T) {
	var writes atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		if r.Method == http.MethodPatch {
			writes.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"@odata.id":"/system","Bios":{"@odata.id":"/bios"}}`)
		case "/bios":
			fmt.Fprint(w, `{"Attributes":{"Counter":131072,"Enabled":true,"Large":18446744073709551615},"@Redfish.Settings":{"SettingsObject":{"@odata.id":"/pending"}}}`)
		case "/pending":
			fmt.Fprint(w, `{"Attributes":{"Counter":131072}}`)
		default:
			http.NotFound(w, r)
		}
	})
	result, err := c.UpdateBIOS(context.Background(), Object{"Counter": "131072", "Enabled": "true", "Large": "18446744073709551615"}, BIOSOptions{SkipUnchanged: true})
	if err != nil || result == nil || !result.NoOp || writes.Load() != 0 {
		t.Fatalf("unchanged write: %v %v", result, err)
	}
	_, err = c.UpdateBIOS(context.Background(), Object{"Counter": 2}, BIOSOptions{ApplyTime: ApplyOnReset, RequireAdvertisedApplyTime: true})
	if err == nil || writes.Load() != 0 {
		t.Fatal("unadvertised apply time must fail before write")
	}
}

func TestBIOSValueEqualityExact(t *testing.T) {
	cases := []struct {
		a, b  any
		equal bool
	}{
		{json.Number("131072"), "131072", true}, {true, "true", true}, {json.Number("7"), "007", false}, {json.Number("16"), "0x10", false}, {json.Number("9007199254740993"), "9007199254740992", false}, {json.Number("18446744073709551615"), uint64(^uint64(0)), true},
	}
	for _, tc := range cases {
		if equivalentAttribute(tc.a, tc.b) != tc.equal {
			t.Fatalf("comparison %v %v", tc.a, tc.b)
		}
	}
}

func TestLenovoPatchVirtualMedia(t *testing.T) {
	var patches atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		switch r.URL.Path {
		case "/manager":
			fmt.Fprint(w, `{"@odata.id":"/manager","VirtualMedia":{"@odata.id":"/media"}}`)
		case "/media":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/media/one"}]}`)
		case "/media/one":
			if r.Method == http.MethodPatch {
				patches.Add(1)
				if r.Header.Get("If-Match") != "media-tag" {
					t.Error("wrong etag")
				}
				var body models.VirtualMediaInsertMediaRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if patches.Load() == 1 && (body.Image != "https://images.invalid/os.iso" || !models.Value(body.Inserted)) {
					t.Error("missing insert fields")
				}
				w.WriteHeader(http.StatusNoContent)
			} else {
				w.Header().Set("ETag", "media-tag")
				fmt.Fprint(w, `{"@odata.id":"/media/one","Id":"one"}`)
			}
		default:
			http.NotFound(w, r)
		}
	}, WithVendor(VendorLenovo))
	if _, err := c.InsertVirtualMedia(context.Background(), "one", models.VirtualMediaInsertMediaRequest{Image: "https://images.invalid/os.iso"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EjectVirtualMedia(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if patches.Load() != 2 {
		t.Fatal("missing patches")
	}
}

func TestBootAutoRoutesAdvertisedProperties(t *testing.T) {
	for _, allSettings := range []bool{false, true} {
		t.Run(fmt.Sprint(allSettings), func(t *testing.T) {
			var paths []string
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if interopResources(w, r) {
					return
				}
				if r.Method == http.MethodPatch {
					paths = append(paths, r.URL.Path)
					if r.Header.Get("If-Match") != r.URL.Path {
						t.Error("etag did not come from actual target")
					}
					var body struct{ Boot map[string]string }
					json.NewDecoder(r.Body).Decode(&body)
					for key := range body.Boot {
						if (allSettings || key == "BootSourceOverrideMode") != (r.URL.Path == "/pending") {
							t.Errorf("wrong destination for %s", key)
						}
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("ETag", r.URL.Path)
				if r.URL.Path == "/system" {
					fmt.Fprint(w, `{"@odata.id":"/system","Boot":{"BootSourceOverrideTarget":"Hdd","BootSourceOverrideMode":"UEFI","BootSourceOverrideEnabled":"Disabled"},"@Redfish.Settings":{"SettingsObject":{"@odata.id":"/pending"}}}`)
					return
				}
				if r.URL.Path == "/pending" {
					if allSettings {
						fmt.Fprint(w, `{"@odata.id":"/pending","Boot":{"BootSourceOverrideMode":"UEFI","BootSourceOverrideTarget":"Hdd","BootSourceOverrideEnabled":"Disabled"}}`)
					} else {
						fmt.Fprint(w, `{"@odata.id":"/pending","Boot":{"BootSourceOverrideMode":"UEFI"}}`)
					}
					return
				}
				http.NotFound(w, r)
			})
			if _, err := c.ChangeBootSource(context.Background(), "Pxe", BootOptions{Route: BootRouteAuto}); err != nil {
				t.Fatal(err)
			}
			expected := 2
			if allSettings {
				expected = 1
			}
			if len(paths) != expected {
				t.Fatalf("writes: %v", paths)
			}
		})
	}
}

func TestDellServicesAndExport(t *testing.T) {
	var polls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		switch r.URL.Path {
		case "/manager":
			fmt.Fprint(w, `{"@odata.id":"/manager","Links":{"Oem":{"Dell":{"DellLCService":{"@odata.id":"/lc"}}}},"Actions":{"Oem":{"#OemManager.ExportSystemConfiguration":{"target":"/export"}}}}`)
		case "/lc":
			fmt.Fprint(w, `{"Actions":{"#DellLCService.GetRemoteServicesAPIStatus":{"target":"/status"}}}`)
		case "/status":
			if r.Method != http.MethodPost {
				t.Error("wrong action method")
			}
			fmt.Fprint(w, `{"Status":"Ready","LCStatus":"Ready","RTStatus":"Ready","ServerStatus":"OutOfPOST","SEKMServiceStatus":"FutureState","TelemetryServiceStatus":"Ready"}`)
		case "/export":
			var body models.DellExportSystemConfigurationRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.ShareParameters.Target != "ALL" {
				t.Error("wrong export target")
			}
			w.Header().Set("Location", "/monitor")
			w.WriteHeader(http.StatusAccepted)
		case "/monitor":
			if polls.Add(1) == 1 {
				w.WriteHeader(http.StatusAccepted)
				fmt.Fprint(w, `{"TaskState":"Running"}`)
			} else {
				fmt.Fprint(w, `<SystemConfiguration><Component/></SystemConfiguration>`)
			}
		default:
			http.NotFound(w, r)
		}
	})
	status, err := c.GetDellRemoteServicesAPIStatus(context.Background())
	if err != nil || !status.IsReady() || status.SEKMServiceStatus != "FutureState" {
		t.Fatalf("status: %+v %v", status, err)
	}
	operation, err := c.ExportDellSystemConfiguration(context.Background(), models.DellExportSystemConfigurationRequest{ExportFormat: models.DellExportXML, ShareParameters: models.DellShareParameters{Target: "ALL"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.WaitForDellSystemConfiguration(context.Background(), operation, PollOptions{Interval: time.Millisecond, Timeout: time.Second})
	if err != nil || string(data) != `<SystemConfiguration><Component/></SystemConfiguration>` {
		t.Fatalf("export: %q %v", data, err)
	}
}

func TestSecureBootEnrollFollowsAdvertisedCollection(t *testing.T) {
	var writes atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"@odata.id":"/system","SecureBoot":{"@odata.id":"/secure"}}`)
		case "/secure":
			fmt.Fprint(w, `{"SecureBootDatabases":{"@odata.id":"/databases"}}`)
		case "/databases":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/db"}]}`)
		case "/db":
			fmt.Fprint(w, `{"@odata.id":"/db","Id":"db","Certificates":{"@odata.id":"/enroll"}}`)
		case "/enroll":
			if r.Method != http.MethodPost {
				t.Error("wrong enrollment method")
			}
			var body models.SecureBootCertificateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.CertificateString != "synthetic-certificate" || body.CertificateType != models.CertificatePEM {
				t.Error("wrong enrollment body")
			}
			writes.Add(1)
			w.Header().Set("Location", "/certificate/created")
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	})
	response, err := c.AddSecureBootCertificate(context.Background(), "db", models.SecureBootCertificateRequest{CertificateString: "synthetic-certificate", CertificateType: models.CertificatePEM})
	if err != nil || response.Header.Get("Location") != "/certificate/created" || writes.Load() != 1 {
		t.Fatalf("enrollment: %v %v", response, err)
	}
}

func TestSupermicroSyslogDiscovery(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		if r.URL.Path == "/manager" {
			fmt.Fprint(w, `{"@odata.id":"/manager","Oem":{"Supermicro":{"Syslog":{"@odata.id":"/syslog"}}}}`)
			return
		}
		if r.URL.Path == "/syslog" {
			fmt.Fprint(w, `{"EnableSyslog":false,"SyslogServer":"logs.example.invalid","SyslogPortNumber":514}`)
			return
		}
		http.NotFound(w, r)
	})
	service, err := c.GetSyslogService(context.Background())
	if err != nil || len(service.SyslogServers) != 1 || models.Value(service.SyslogServers[0].Port) != 514 {
		t.Fatalf("syslog: %+v %v", service, err)
	}
}

func TestDellExportFailureAndCancellation(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if failure {
					fmt.Fprint(w, `{"TaskState":"Exception","Messages":[{"Message":"export rejected"}]}`)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				fmt.Fprint(w, `{"TaskState":"Running"}`)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			_, err := c.WaitForDellSystemConfiguration(ctx, &models.RedfishResponse{TaskMonitor: models.Ptr("/monitor")}, PollOptions{Interval: time.Millisecond})
			if err == nil {
				t.Fatal("failed/running export cannot return success")
			}
			if failure && err.Error() != "export rejected" {
				t.Fatalf("downstream task message changed: %v", err)
			}
		})
	}
}

func TestFullModernFans(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Chassis":{"@odata.id":"/chassis"}}`)
		case "/chassis":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/chassis/one"}]}`)
		case "/chassis/one":
			fmt.Fprint(w, `{"@odata.id":"/chassis/one","ThermalSubsystem":{"@odata.id":"/thermal"}}`)
		case "/thermal":
			fmt.Fprint(w, `{"Fans":{"@odata.id":"/fans"}}`)
		case "/fans":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/fan"}]}`)
		case "/fan":
			fmt.Fprint(w, `{"@odata.id":"/fan","Name":"modern","HotPluggable":true,"SpeedPercent":{"Reading":43},"VendorMetadata":"preserved"}`)
		default:
			http.NotFound(w, r)
		}
	})
	fans, err := c.GetStandaloneFans(context.Background())
	if err != nil || len(fans) != 1 || string(fans[0].Extra["VendorMetadata"]) != `"preserved"` {
		t.Fatalf("fans: %v %v", fans, err)
	}
}

func TestCollectionFailuresAndCancellation(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collection" {
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/slow"},{"@odata.id":"/fail"},{"@odata.id":"/ok"}]}`)
			return
		}
		if r.URL.Path == "/slow" {
			time.Sleep(15 * time.Millisecond)
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/fail" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"@odata.id":"/ok","Id":"ok"}`)
	}, WithCollectionConcurrency(2))
	result, err := Collect[models.System](context.Background(), c, "/collection")
	if err != nil || len(result.Failures) != 2 || len(result.Members) != 1 || !strings.HasSuffix(result.Failures[0].URI, "/slow") || !strings.HasSuffix(result.Failures[1].URI, "/fail") {
		t.Fatalf("unstable failures: %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var members []models.System
	if err := c.GetCollection(ctx, "/collection", &members); err == nil {
		t.Fatal("canceled collection succeeded")
	}
}

func TestBIOSPendingPrecedenceAndPayloadType(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		if r.Method == http.MethodPatch {
			var body struct{ Attributes map[string]json.RawMessage }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body.Attributes) != 1 || string(body.Attributes["Counter"]) != `"2"` {
				t.Errorf("changed caller payload: %v", body.Attributes)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"@odata.id":"/system","Bios":{"@odata.id":"/bios"}}`)
		case "/bios":
			fmt.Fprint(w, `{"Attributes":{"Counter":2,"Keep":1},"@Redfish.Settings":{"SettingsObject":{"@odata.id":"/pending"}}}`)
		case "/pending":
			fmt.Fprint(w, `{"Attributes":{"Counter":1}}`)
		default:
			http.NotFound(w, r)
		}
	})
	result, err := c.UpdateBIOS(context.Background(), Object{"Counter": "2", "Keep": "1"}, BIOSOptions{SkipUnchanged: true})
	if err != nil || result.NoOp {
		t.Fatalf("pending difference was skipped: %v %v", result, err)
	}
}

func TestBootPartialFailureIsInspectable(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		if r.Method == http.MethodPatch {
			if r.URL.Path == "/pending" {
				w.WriteHeader(http.StatusPreconditionFailed)
				fmt.Fprint(w, `{"error":{"message":"settings changed"}}`)
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
			return
		}
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"@odata.id":"/system","Boot":{},"@Redfish.Settings":{"SettingsObject":{"@odata.id":"/pending"}}}`)
		case "/pending":
			fmt.Fprint(w, `{"@odata.id":"/pending","Boot":{"BootSourceOverrideMode":"UEFI"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	_, err := c.ChangeBootSource(context.Background(), "Pxe", BootOptions{Route: BootRouteAuto})
	var update *BootUpdateError
	if !errors.As(err, &update) || len(update.CompletedURIs) != 1 || update.CompletedURIs[0] != "/system" || err.Error() != "settings changed" {
		t.Fatalf("lost partial update context: %v", err)
	}
}

func TestConditionalReadDoesNotHideRedirectsOrUnsolicited304(t *testing.T) {
	for _, status := range []int{http.StatusNotModified, http.StatusFound} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		if _, err := Fetch[models.System](context.Background(), c, "/system", nil); err == nil {
			t.Fatalf("accepted status %d without cache", status)
		}
	}
}

func TestBIOSComparisonUsesSameSnapshotETag(t *testing.T) {
	var reads atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if interopResources(w, r) {
			return
		}
		switch r.URL.Path {
		case "/system":
			fmt.Fprint(w, `{"@odata.id":"/system","Bios":{"@odata.id":"/bios"}}`)
		case "/bios":
			fmt.Fprint(w, `{"@Redfish.Settings":{"SettingsObject":{"@odata.id":"/pending"}}}`)
		case "/pending":
			if r.Method == http.MethodPatch {
				if r.Header.Get("If-Match") != "snapshot-1" {
					t.Error("comparison validator was replaced by a newer snapshot")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			reads.Add(1)
			w.Header().Set("ETag", fmt.Sprintf("snapshot-%d", reads.Load()))
			fmt.Fprint(w, `{"Attributes":{"Counter":1}}`)
		default:
			http.NotFound(w, r)
		}
	})
	if _, err := c.UpdateBIOS(context.Background(), Object{"Counter": 2}, BIOSOptions{SkipUnchanged: true}); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 {
		t.Fatal("comparison and mutation must share one snapshot")
	}
}
