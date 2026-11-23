package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

func TestBIOSUsesPendingSettingsETagAndSchedule(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Systems":{"@odata.id":"/systems"}}`)
		case "/systems":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/systems/a","Id":"a","Bios":{"@odata.id":"/bios"}}]}`)
		case "/bios":
			fmt.Fprint(w, `{"@odata.id":"/bios","@Redfish.Settings":{"SettingsObject":{"@odata.id":"/pending"},"ETag":"old-active"}}`)
		case "/pending":
			if r.Method == http.MethodGet {
				w.Header().Set(wire.HeaderETag, `W/"pending"`)
				fmt.Fprint(w, `{"@odata.id":"/pending"}`)
				return
			}
			if r.Header.Get(wire.HeaderIfMatch) != `W/"pending"` {
				t.Error("wrong ETag")
			}
			var body Object
			json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["Attributes"]; !ok {
				t.Error("missing attributes")
			}
			if _, ok := body["@Redfish.SettingsApplyTime"]; !ok {
				t.Error("missing apply time")
			}
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	})
	if _, err := c.UpdateBIOS(context.Background(), Object{"BootMode": "UEFI"}, BIOSOptions{ApplyTime: ApplyOnReset}); err != nil {
		t.Fatal(err)
	}
}

func TestChangeSetPreflightsAllETagsBeforeWriting(t *testing.T) {
	var writes int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			writes++
			w.WriteHeader(204)
			return
		}
		etag := `"v1"`
		if r.URL.Path == "/b" {
			etag = `"v2"`
		}
		w.Header().Set(wire.HeaderETag, etag)
		fmt.Fprint(w, `{}`)
	})
	changes := c.ChangeSet()
	if err := changes.Stage(http.MethodPatch, "/a", Object{"Enabled": false}, ChangeOptions{ETag: `"v1"`}); err != nil {
		t.Fatal(err)
	}
	if err := changes.Stage(http.MethodPatch, "/b", Object{"Enabled": true}, ChangeOptions{ETag: `"v1"`}); err != nil {
		t.Fatal(err)
	}
	result, err := changes.Commit(context.Background(), CommitOptions{})
	if err == nil || writes != 0 || result.Successful() {
		t.Fatalf("preflight did not block: %d %+v %v", writes, result, err)
	}
}

func TestChangeSetDryRunAndNull(t *testing.T) {
	var writes int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
		}
		fmt.Fprint(w, `{}`)
	})
	changes := c.ChangeSet()
	if err := changes.Stage(http.MethodPatch, "/a", Object{"Value": nil, "Password": "sensitive"}, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	plan, err := changes.Plan(false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(plan)
	if string(data) == "" || containsSecret(data, "sensitive") {
		t.Fatal("plan leaks credentials")
	}
	if _, err := changes.Commit(context.Background(), CommitOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatal("dry run wrote data")
	}
}

func containsSecret(data []byte, secret string) bool {
	var value any
	json.Unmarshal(data, &value)
	return string(data) != "" && string(data) == secret || bytesContains(data, []byte(secret))
}

func bytesContains(data, part []byte) bool {
	for i := 0; i+len(part) <= len(data); i++ {
		if string(data[i:i+len(part)]) == string(part) {
			return true
		}
	}
	return false
}

func TestTaskPollAndTerminalFailure(t *testing.T) {
	for _, state := range []string{TaskCompleted, TaskException} {
		t.Run(state, func(t *testing.T) {
			calls := 0
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					fmt.Fprint(w, `{"@odata.id":"/task","TaskState":"Running"}`)
				} else {
					fmt.Fprintf(w, `{"@odata.id":"/task","TaskState":%q}`, state)
				}
			})
			task, err := c.WaitForTask(context.Background(), "/task", PollOptions{Interval: time.Millisecond, Timeout: time.Second})
			if state == TaskCompleted && (err != nil || pointer.Value(task.TaskState) != state) {
				t.Fatalf("completion: %v", err)
			}
			var async *AsyncOperationError
			if state == TaskException && !errors.As(err, &async) {
				t.Fatalf("missing failure: %v", err)
			}
		})
	}
}

func TestVendorUpdateBodies(t *testing.T) {
	tests := []struct {
		vendor         Vendor
		key, namespace string
	}{{vendor: VendorNettrix, key: "SaveConfig", namespace: ""}, {vendor: VendorXFusion, key: "PreserveConfig", namespace: ""}, {vendor: VendorH3C, key: "Preserve", namespace: "Public"}, {vendor: VendorZTE, key: "PreserveConf", namespace: "Public"}, {vendor: VendorInspur, key: "PreserveConf", namespace: "Public"}}
	for _, test := range tests {
		t.Run(string(test.vendor), func(t *testing.T) {
			body, err := NewProviderRegistry().Update(test.vendor).BuildBody(UpdateRequest{ImageURI: "https://images.invalid/fw.bin", PreserveConfig: pointer.Ptr(false)})
			if err != nil {
				t.Fatal(err)
			}
			var value any = body[test.key]
			if test.namespace != "" {
				value = body["Oem"].(Object)[test.namespace].(Object)[test.key]
			}
			if value == nil {
				t.Fatalf("missing preservation option: %v", body)
			}
		})
	}
}

func TestAMIDefaultAndExplicitStrategies(t *testing.T) {
	for _, vendor := range []Vendor{VendorGeneric, VendorAMI} {
		t.Run(string(vendor), func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/redfish/v1/":
					fmt.Fprint(w, `{"Chassis":{"@odata.id":"/chassis"}}`)
				case "/chassis":
					fmt.Fprint(w, `{"Members":[{"@odata.id":"/chassis/tray","Thermal":{"@odata.id":"/thermal"}}]}`)
				case "/thermal":
					fmt.Fprint(w, `{"@odata.id":"/thermal","InletHistoryTemperature":{"@odata.id":"/legacy"}}`)
				case "/legacy":
					if vendor != VendorGeneric {
						t.Error("AMI used default link")
					}
					fmt.Fprint(w, `{"HistoricalInletTemp":[{"avg":20}]}`)
				case "/thermal/Oem/Ami/InletHistoryTemperature":
					if vendor != VendorAMI {
						t.Error("default used AMI link")
					}
					fmt.Fprint(w, `{"HistoricalInletTemp":[{"avg":21}]}`)
				default:
					http.NotFound(w, r)
				}
			}, WithVendor(vendor))
			result, err := c.GetInletHistoryTemperature(context.Background())
			if err != nil || len(result.HistoricalInletTemp) != 1 {
				t.Fatalf("history: %v %v", result, err)
			}
		})
	}
}

func TestChangeSetDryRunSnapshotCannotMutateStagedBody(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) })
	changes := c.ChangeSet()
	if err := changes.Stage(http.MethodPatch, "/a", Object{"Value": "original"}, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	preview, err := changes.Commit(context.Background(), CommitOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := range preview.Operations[0].Operation.Body {
		preview.Operations[0].Operation.Body[i] = ' '
	}
	if !json.Valid(changes.Operations()[0].Body) {
		t.Fatal("dry-run result modified the immutable staged request")
	}
}

func TestChangeSetIfNoneMatchPreflightsBeforeWriting(t *testing.T) {
	writes := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{}`)
			return
		}
		writes++
		w.WriteHeader(204)
	})
	changes := c.ChangeSet()
	if err := changes.Stage(http.MethodPost, "/new", Object{"Name": "new"}, ChangeOptions{IfNoneMatch: "*"}); err != nil {
		t.Fatal(err)
	}
	_, err := changes.Commit(context.Background(), CommitOptions{})
	if !errors.Is(err, sdkerrors.ErrPreconditionFailed) || writes != 0 {
		t.Fatalf("If-None-Match failed to protect the plan: %d %v", writes, err)
	}
}

func TestIEITTaskVisibilityDelayIsScopedAndBounded(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"code":17017}`)
			return
		}
		fmt.Fprint(w, `{"@odata.id":"/task","TaskState":"Completed"}`)
	})
	_, err := c.WaitForTask(context.Background(), "/task", PollOptions{AllowTaskVisibilityDelay: true, Interval: time.Millisecond, Timeout: time.Second})
	if err != nil || calls != 2 {
		t.Fatalf("visibility recovery failed: %d %v", calls, err)
	}
}

func TestTaskRecoveryRetriesOnlyFailedReads(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Length", "64")
			fmt.Fprint(w, `{"TaskState":`)
			return
		}
		fmt.Fprint(w, `{"@odata.id":"/task","TaskState":"Completed"}`)
	})
	task, err := c.WaitForTask(context.Background(), "/task", PollOptions{MaxTransientFailures: 1, Interval: time.Millisecond, Timeout: time.Second})
	if err != nil || task == nil || calls != 2 {
		t.Fatalf("read recovery failed: %d %v", calls, err)
	}
}
