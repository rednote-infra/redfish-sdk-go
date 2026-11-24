package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func TestDetectHPEAndDell(t *testing.T) {
	cases := []struct {
		name         string
		manufacturer string
		extensions   *models.Oem
		want         Vendor
	}{
		{"HPE", "Hewlett Packard Enterprise", nil, VendorHPE}, {"HP legacy", "HP", nil, VendorHPE}, {"Dell", "Dell Inc.", nil, VendorDell}, {"Dell EMC", "Dell EMC", nil, VendorDell},
		{"Hpe namespace", "", &models.Oem{"Hpe": json.RawMessage(`{}`)}, VendorHPE}, {"Hp namespace", "", &models.Oem{"Hp": json.RawMessage(`{}`)}, VendorHPE}, {"Dell namespace", "", &models.Oem{"Dell": json.RawMessage(`{}`)}, VendorDell},
		{"unrelated", "Example ChpVendor", nil, VendorGeneric},
		{"similar name", "Modelling Labs", nil, VendorGeneric},
		{"HP Inc", "HP Inc.", nil, VendorHPE},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectVendor(tc.manufacturer, tc.extensions); got != tc.want {
				t.Fatalf("vendor=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestHPEAHSAdvertisedDownload(t *testing.T) {
	var posts atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			t.Error("AHS download must not trigger a collection POST")
		}
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
		case "/managers":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/ilo/custom"}]}`)
		case "/ilo/custom":
			fmt.Fprint(w, `{"@odata.id":"/ilo/custom","Oem":{"Hpe":{"Links":{"ActiveHealthSystem":{"@odata.id":"/health/advertised"}}}}}`)
		case "/health/advertised":
			fmt.Fprint(w, `{"Links":{"AHSLocation":{"extref":"/artifact/ahs?device=default"}}}`)
		case "/artifact/ahs":
			if r.URL.Query().Get("device") != "default" || r.URL.Query().Get("downloadAll") != "1" || r.URL.Query().Get("co_name") != "Example & Co" {
				t.Errorf("query changed: %s", r.URL.RawQuery)
			}
			if _, _, ok := r.BasicAuth(); !ok {
				t.Error("same-origin AHS download lost BMC authentication")
			}
			fmt.Fprint(w, "synthetic-ahs")
		default:
			http.NotFound(w, r)
		}
	}, WithVendor(VendorHPE))
	options := CollectDownloadOptions{Diagnostic: DiagnosticOptions{DownloadQuery: url.Values{"downloadAll": {"1"}, "co_name": {"Example & Co"}}}}
	data, err := c.CollectAndDownloadDiagnosticDataBytes(context.Background(), options)
	if err != nil || string(data) != "synthetic-ahs" || posts.Load() != 0 {
		t.Fatalf("AHS: %q %v", data, err)
	}
	path := filepath.Join(t.TempDir(), "ahs.log")
	if _, err := c.CollectAndDownloadDiagnosticData(context.Background(), path, options); err != nil {
		t.Fatal(err)
	}
	if saved, err := os.ReadFile(path); err != nil || string(saved) != "synthetic-ahs" {
		t.Fatalf("saved AHS: %q %v", saved, err)
	}
}

func TestDellSupportAssistJobs(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(fmt.Sprint(modern), func(t *testing.T) {
			var polls atomic.Int32
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/redfish/v1/":
					fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
				case "/managers":
					fmt.Fprint(w, `{"Members":[{"@odata.id":"/idrac/custom"}]}`)
				case "/idrac/custom":
					fmt.Fprint(w, `{"@odata.id":"/idrac/custom","Links":{"Oem":{"Dell":{"DellLCService":{"@odata.id":"/lc/advertised"}}}}}`)
				case "/lc/advertised":
					fmt.Fprint(w, `{"Actions":{"#DellLCService.SupportAssistCollection":{"target":"/support/collect"}}}`)
				case "/support/collect":
					var body struct {
						ShareType           string
						DataSelectorArrayIn []string
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.ShareType != "Local" || len(body.DataSelectorArrayIn) != 1 || body.DataSelectorArrayIn[0] != "HWData" {
						t.Errorf("wrong SA request: %+v", body)
					}
					w.Header().Set("Location", "/jobs/advertised")
					w.WriteHeader(http.StatusAccepted)
				case "/jobs/advertised":
					if polls.Add(1) == 1 {
						if modern {
							fmt.Fprint(w, `{"Oem":{"Dell":{"JobState":"Running","Message":"in progress"}}}`)
						} else {
							fmt.Fprint(w, `{"JobState":"Running","Message":"in progress"}`)
						}
						return
					}
					w.Header().Set("Location", "/artifacts/sacollect.zip")
					if modern {
						w.WriteHeader(http.StatusNoContent)
					} else {
						fmt.Fprint(w, `{"JobState":"Completed"}`)
					}
				case "/artifacts/sacollect.zip":
					fmt.Fprint(w, "synthetic-supportassist")
				default:
					http.NotFound(w, r)
				}
			}, WithVendor(VendorDell))
			data, err := c.CollectAndDownloadDiagnosticDataBytes(context.Background(), CollectDownloadOptions{Diagnostic: DiagnosticOptions{OEMParameters: Object{"DataSelectorArrayIn": []string{"HWData"}}}, Poll: PollOptions{Interval: time.Millisecond, Timeout: time.Second}})
			if err != nil || string(data) != "synthetic-supportassist" {
				t.Fatalf("SA: %q %v", data, err)
			}
		})
	}
}

func TestHPEAndDellAutomaticFirmwareStrategy(t *testing.T) {
	for _, manufacturer := range []string{"Hewlett Packard Enterprise", "Dell Inc."} {
		t.Run(manufacturer, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/redfish/v1/":
					fmt.Fprint(w, `{"Systems":{"@odata.id":"/systems"},"UpdateService":{"@odata.id":"/updates"}}`)
				case "/systems":
					fmt.Fprint(w, `{"Members":[{"@odata.id":"/system"}]}`)
				case "/system":
					fmt.Fprintf(w, `{"@odata.id":"/system","Manufacturer":%q}`, manufacturer)
				case "/updates":
					fmt.Fprint(w, `{"Actions":{"#UpdateService.SimpleUpdate":{"target":"/firmware/advertised"}}}`)
				case "/firmware/advertised":
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if manufacturer == "Hewlett Packard Enterprise" {
						if string(body["TPMOverrideFlag"]) != "false" {
							t.Error("HPE firmware policy was not selected")
						}
					} else {
						if string(body["Username"]) != `"source-user"` {
							t.Error("Dell firmware policy was not selected")
						}
					}
					w.Header().Set("Location", "/task/firmware")
					w.WriteHeader(http.StatusAccepted)
				default:
					http.NotFound(w, r)
				}
			})
			response, err := c.SimpleUpdate(context.Background(), UpdateRequest{ImageURI: "https://images.invalid/fw.bin", Username: "source-user", Options: Object{"TPMOverrideFlag": false}})
			if err != nil || models.Value(response.TaskMonitor) != "/task/firmware" {
				t.Fatalf("firmware operation: %v %v", response, err)
			}
		})
	}
}

func TestDellJobFailurePreservesDownstreamMessage(t *testing.T) {
	const message = "  SupportAssist failed: preserve punctuation!  "
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/job" {
			fmt.Fprintf(w, `{"Oem":{"Dell":{"JobState":"Failed","Message":%q}}}`, message)
			return
		}
		http.NotFound(w, r)
	}, WithVendor(VendorDell))
	task := &DiagnosticTask{Vendor: VendorDell, Task: &models.Task{TaskState: models.Ptr(TaskRunning)}, ProgressURI: "/job"}
	err := c.WaitForDiagnosticData(context.Background(), task, PollOptions{Interval: time.Millisecond, Timeout: time.Second})
	if err == nil || err.Error() != message {
		t.Fatalf("downstream message changed: %v", err)
	}
}

func TestHPEExternalArtifactDoesNotReceiveBMCCredentials(t *testing.T) {
	artifact := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Auth-Token") != "" || r.Header.Get("Cookie") != "" {
			t.Error("external AHS download received BMC credentials")
		}
		fmt.Fprint(w, "external-ahs")
	}))
	defer artifact.Close()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
		case "/managers":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/manager"}]}`)
		case "/manager":
			fmt.Fprint(w, `{"Oem":{"Hpe":{"Links":{"ActiveHealthSystem":{"@odata.id":"/ahs"}}}}}`)
		case "/ahs":
			fmt.Fprintf(w, `{"Links":{"AHSLocation":{"extref":%q}}}`, artifact.URL+"/download")
		default:
			http.NotFound(w, r)
		}
	}, WithVendor(VendorHPE), WithSession("synthetic-session-token", "/session"))
	data, err := c.CollectAndDownloadDiagnosticDataBytes(context.Background(), CollectDownloadOptions{})
	if err != nil || string(data) != "external-ahs" {
		t.Fatalf("external AHS: %q %v", data, err)
	}
}

func TestDellMonitorHonorsRetryAfterAndCancellation(t *testing.T) {
	var reads atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		w.Header().Set("Location", "/archives/sacollect.zip")
		w.WriteHeader(http.StatusNoContent)
	}, WithVendor(VendorDell))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	task := &DiagnosticTask{Vendor: VendorDell, Task: &models.Task{TaskState: models.Ptr(TaskRunning)}, ProgressURI: "/job", Response: &sdkresponse.Response{StatusCode: http.StatusAccepted, Header: http.Header{"Retry-After": []string{"1"}}}}
	err := c.WaitForDiagnosticData(ctx, task, PollOptions{Interval: time.Millisecond, Timeout: time.Second})
	if !errors.Is(err, context.DeadlineExceeded) || reads.Load() != 0 {
		t.Fatalf("Retry-After ignored: reads=%d err=%v", reads.Load(), err)
	}
}
