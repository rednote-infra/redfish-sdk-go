package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnginetechDiagnosticUsesFreshBlackBoxLatest(t *testing.T) {
	for _, scenario := range []struct {
		name, entries, latest, status, wantError string
		logID                                    string
		inline                                   bool
	}{
		{name: "fresh file and memory downloads", entries: `,"Entries":{"@odata.id":"/opaque/archive-index"}`, latest: `{"AdditionalDataURI":"/fresh-bundle"}`, status: "OK"},
		{name: "explicit service URI with absent status", entries: `,"Entries":{"@odata.id":"/opaque/archive-index"}`, latest: `{"AdditionalDataURI":"/fresh-bundle"}`, logID: "/opaque/service"},
		{name: "missing entries", status: "OK", wantError: "Entries"},
		{name: "missing artifact", entries: `,"Entries":{"@odata.id":"/opaque/archive-index"}`, latest: `{}`, status: "OK", wantError: "AdditionalDataURI"},
		{name: "missing latest", entries: `,"Entries":{"@odata.id":"/opaque/archive-index"}`, status: "OK", wantError: "404"},
		{name: "warning is not success", entries: `,"Entries":{"@odata.id":"/opaque/archive-index"}`, status: "Warning", wantError: "Collection incomplete"},
		{name: "inline warning is not success", entries: `,"Entries":{"@odata.id":"/opaque/archive-index"}`, status: "Warning", inline: true, wantError: "Collection incomplete"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			posts, downloads, latestReads := 0, 0, 0
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/redfish/v1/":
					fmt.Fprint(w, `{"Systems":{"@odata.id":"/systems"}}`)
				case "/systems":
					fmt.Fprint(w, `{"Members":[{"@odata.id":"/system","Id":"Self","LogServices":{"@odata.id":"/logs"}}]}`)
				case "/logs":
					fmt.Fprint(w, `{"Members":[{"@odata.id":"/opaque/service"},{"@odata.id":"/other-service"}]}`)
				case "/opaque/service":
					fmt.Fprintf(w, `{"@odata.id":"/opaque/service","Id":"BlackBox","Actions":{"#LogService.CollectDiagnosticData":{"target":"/collect"}}%s}`, scenario.entries)
				case "/other-service":
					fmt.Fprint(w, `{"@odata.id":"/other-service","Id":"Other","Actions":{"#LogService.CollectDiagnosticData":{"target":"/wrong-collect"}}}`)
				case "/collect":
					if r.Method != http.MethodPost {
						t.Errorf("collect method: %s", r.Method)
					}
					var body struct {
						Kind    string `json:"DiagnosticDataType"`
						OEMKind string `json:"OEMDiagnosticDataType"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Kind != "OEM" || body.OEMKind != "BlackBox" {
						t.Errorf("collect body: %+v, %v", body, err)
					}
					posts++
					if scenario.inline {
						fmt.Fprintf(w, `{"TaskState":"Completed","TaskStatus":%q,"Messages":[{"Message":"Collection incomplete"}]}`, scenario.status)
						return
					}
					w.WriteHeader(http.StatusAccepted)
					fmt.Fprint(w, `{"@odata.id":"/tasks/1","TaskState":"Running"}`)
				case "/tasks/1":
					fmt.Fprintf(w, `{"@odata.id":"/tasks/1","TaskState":"Completed","TaskStatus":%q,"Messages":[{"Message":"Collection incomplete"}],"AdditionalDataURI":"/stale-bundle","Links":{"CreatedResources":[{"@odata.id":"/old-entry"}]}}`, scenario.status)
				case "/opaque/archive-index/Latest":
					latestReads++
					if scenario.status == "Warning" {
						t.Error("read Latest after failed collection")
					}
					if scenario.latest == "" {
						http.NotFound(w, r)
						return
					}
					fmt.Fprint(w, scenario.latest)
				case "/fresh-bundle":
					downloads++
					fmt.Fprintf(w, "fresh bundle %d", posts)
				default:
					t.Errorf("unexpected request (must not enumerate entries, reuse tasks, or follow stale artifacts): %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}, WithVendor(VendorEnginetech))
			options := CollectDownloadOptions{Diagnostic: DiagnosticOptions{LogID: scenario.logID}, Poll: PollOptions{Interval: time.Millisecond, Timeout: time.Second}}
			path := filepath.Join(t.TempDir(), "blackbox.tgz")
			_, err := c.CollectAndDownloadDiagnosticData(context.Background(), path, options)
			if scenario.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), scenario.wantError) {
					t.Fatalf("error: %v, want %q", err, scenario.wantError)
				}
				if scenario.status == "Warning" {
					var failure *AsyncOperationError
					if !errors.As(err, &failure) || failure.Task == nil || failure.Response == nil || err.Error() != scenario.wantError {
						t.Fatalf("lost task failure context or message: %#v", err)
					}
				}
				if downloads != 0 {
					t.Fatal("downloaded an invalid artifact")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "fresh bundle 1" {
				t.Fatalf("file download: %q %v", data, err)
			}
			data, err = c.CollectAndDownloadDiagnosticDataBytes(context.Background(), options)
			if err != nil || string(data) != "fresh bundle 2" {
				t.Fatalf("memory download: %q %v", data, err)
			}
			if posts != 2 || downloads != 2 || latestReads != 2 {
				t.Fatalf("not fresh: posts=%d downloads=%d Latest=%d", posts, downloads, latestReads)
			}
		})
	}
}
