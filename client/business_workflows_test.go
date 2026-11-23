package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func assertWireBody(t *testing.T, r *http.Request, expected Object) {
	t.Helper()
	var actual any
	if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	var normalized any
	json.Unmarshal(data, &normalized)
	if !reflect.DeepEqual(actual, normalized) {
		t.Fatalf("wrong wire body for %s: got %v, expected %v", r.URL.Path, actual, normalized)
	}
}

func TestAccountEventAndPolicyWorkflows(t *testing.T) {
	ctx := context.Background()
	writes := 0
	graph := map[string]string{
		"/redfish/v1/":     `{"AccountService":{"@odata.id":"/account-service"},"EventService":{"@odata.id":"/event-service"},"PolicyService":{"@odata.id":"/policy-service"}}`,
		"/account-service": `{"Accounts":{"@odata.id":"/accounts"}}`, "/accounts": `{"Members":[{"@odata.id":"/accounts/one"}]}`, "/accounts/one": `{"@odata.id":"/accounts/one","UserName":"tester"}`,
		"/event-service": `{"@odata.id":"/event-service","Subscriptions":{"@odata.id":"/subscriptions"},"Actions":{"#EventService.SubmitTestEvent":{"target":"/test-event"}}}`, "/subscriptions": `{"Members":[{"@odata.id":"/subscriptions/one"}]}`, "/subscriptions/one": `{"@odata.id":"/subscriptions/one"}`,
		"/policy-service": `{"Policies":{"@odata.id":"/policies"}}`, "/policies": `{"Members":[{"@odata.id":"/policies/one"}]}`, "/policies/one": `{"@odata.id":"/policies/one"}`,
	}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if body, ok := graph[r.URL.Path]; ok {
				w.Header().Set(wire.HeaderETag, `"v1"`)
				fmt.Fprint(w, body)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		writes++
		switch r.Method + " " + r.URL.Path {
		case "POST /accounts":
			assertWireBody(t, r, Object{"UserName": "new-user", "RoleId": "ReadOnly", "Password": nil, "Enabled": false})
			w.Header().Set(wire.HeaderLocation, "/accounts/new")
			w.WriteHeader(201)
		case "PATCH /accounts/one":
			assertWireBody(t, r, Object{"Enabled": false, "EmailAddress": ""})
			if r.Header.Get(wire.HeaderIfMatch) != `"v1"` {
				t.Error("missing account concurrency validator")
			}
			w.WriteHeader(204)
		case "DELETE /accounts/one", "DELETE /subscriptions/one", "DELETE /policies/one":
			w.WriteHeader(204)
		case "POST /subscriptions":
			assertWireBody(t, r, Object{"Destination": "https://events.invalid/redfish", "Protocol": "Redfish"})
			w.Header().Set(wire.HeaderLocation, "/subscriptions/new")
			w.WriteHeader(201)
		case "POST /test-event":
			assertWireBody(t, r, Object{"Message": "Test message"})
			w.WriteHeader(204)
		case "POST /policies":
			var body Object
			json.NewDecoder(r.Body).Decode(&body)
			if body["Id"] != "new" || body["Enabled"] != false || body["Oem"] == nil {
				t.Error("policy fields were omitted")
			}
			w.Header().Set(wire.HeaderLocation, "/policies/new")
			w.WriteHeader(201)
		case "PATCH /policies/one":
			var body Object
			json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["Enabled"]; !ok {
				t.Error("policy enablement missing")
			}
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	})
	account, err := c.AddAccount(ctx, models.AccountCreate{UserName: "new-user", RoleID: "ReadOnly", Enabled: models.Ptr(false)})
	if err != nil || models.Value(account.ODataID) != "/accounts/new" {
		t.Fatalf("create account: %v", err)
	}
	if _, err = c.UpdateAccount(ctx, "tester", models.AccountPatch{Enabled: models.Ptr(false), EmailAddress: models.Ptr("")}); err != nil {
		t.Fatal(err)
	}
	if err = c.DeleteAccount(ctx, "tester"); err != nil {
		t.Fatal(err)
	}
	subscription, err := c.Subscribe(ctx, models.Subscription{Destination: models.Ptr("https://events.invalid/redfish")})
	if err != nil || models.Value(subscription.ODataID) != "/subscriptions/new" {
		t.Fatalf("subscribe: %v", err)
	}
	if err = c.DeleteSubscription(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SubmitTestEvent(ctx, models.EventRecord{Message: models.Ptr("Test message")}); err != nil {
		t.Fatal(err)
	}
	policy, err := c.CreatePolicy(ctx, PolicyCreate{ID: "new", Name: "New", TriggerCondition: models.PolicyCondition{Type: models.Ptr("Triggered")}, Response: []models.PolicyResponse{{ResponseAction: models.Ptr("LogEvent")}}, Enabled: models.Ptr(false), OEM: models.Ptr(Object{})})
	if err != nil || models.Value(policy.ODataID) != "/policies/new" {
		t.Fatalf("create policy: %v", err)
	}
	if _, err = c.EnablePolicy(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.DisablePolicy(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err = c.DeletePolicy(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if writes != 10 {
		t.Fatalf("unexpected write count: %d", writes)
	}
}

func TestSystemManagerAndConsoleActions(t *testing.T) {
	ctx := context.Background()
	writes := 0
	graph := map[string]string{
		"/redfish/v1/": `{"Systems":{"@odata.id":"/systems"},"Managers":{"@odata.id":"/managers"}}`, "/systems": `{"Members":[{"@odata.id":"/systems/one"}]}`,
		"/systems/one": `{"@odata.id":"/systems/one","PowerState":"On","Boot":{"BootSourceOverrideTarget@Redfish.AllowableValues":["Pxe"]},"SecureBoot":{"@odata.id":"/secure-boot"},"GraphicalConsole":{"HTML5":{"ServiceEnabled":true,"SupportsOneTimeURL":true},"VNC":{"ServiceEnabled":true,"SupportsOneTimePassword":true,"SupportsPersistentPassword":true}},"Actions":{"#ComputerSystem.Reset":{"target":"/reset","ResetType@Redfish.AllowableValues":["GracefulRestart"]},"#ComputerSystem.GraphicalConsoleGetOneTimeURL":{"target":"/console/url"},"#ComputerSystem.GraphicalConsoleSetOneTimePassword":{"target":"/console/temporary"},"#ComputerSystem.GraphicalConsoleSetPassword":{"target":"/console/persistent"}}}`,
		"/secure-boot": `{"@odata.id":"/secure-boot","Actions":{"#SecureBoot.ResetKeys":{"target":"/reset-keys","ResetKeysType@Redfish.AllowableValues":["DeletePK"]}}}`,
		"/managers":    `{"Members":[{"@odata.id":"/managers/one"}]}`, "/managers/one": `{"@odata.id":"/managers/one","SerialInterfaces":{"@odata.id":"/serial"},"VirtualMedia":{"@odata.id":"/media"},"Actions":{"#Manager.Reset":{"target":"/manager-reset"}}}`,
		"/serial/one": `{"@odata.id":"/serial/one"}`, "/serial": `{"Members":[{"@odata.id":"/serial/one","Id":"one"}]}`, "/media": `{"Members":[{"@odata.id":"/media/one"}]}`, "/media/one": `{"@odata.id":"/media/one","Actions":{"#VirtualMedia.InsertMedia":{"target":"/media/insert","TransferMethod@Redfish.AllowableValues":["Stream"]},"#VirtualMedia.EjectMedia":{"target":"/media/eject"}}}`,
	}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if body, ok := graph[r.URL.Path]; ok {
				w.Header().Set(wire.HeaderETag, `"v1"`)
				fmt.Fprint(w, body)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		writes++
		switch r.URL.Path {
		case "/systems/one":
			assertWireBody(t, r, Object{"Boot": Object{"BootSourceOverrideTarget": "Pxe", "BootSourceOverrideMode": "UEFI", "BootSourceOverrideEnabled": "Once"}})
		case "/reset":
			assertWireBody(t, r, Object{"ResetType": "GracefulRestart"})
		case "/secure-boot":
			assertWireBody(t, r, Object{"SecureBootEnable": false})
		case "/reset-keys":
			assertWireBody(t, r, Object{"ResetKeysType": "DeletePK"})
		case "/manager-reset":
			assertWireBody(t, r, Object{"ResetType": "ForceRestart"})
		case "/serial/one":
			assertWireBody(t, r, Object{"InterfaceEnabled": false})
		case "/media/insert":
			assertWireBody(t, r, Object{"Image": "https://images.invalid/os.iso", "Inserted": false, "TransferMethod": "Stream"})
		case "/media/eject":
			assertWireBody(t, r, Object{})
		case "/console/url":
			assertWireBody(t, r, Object{"ConsoleType": "HTML5", "ValidityPeriodSeconds": 60})
			fmt.Fprint(w, `{"ConsoleType":"HTML5","AccessMode":"ReadWrite","ConsoleURL":"https://console.invalid/temporary"}`)
			return
		case "/console/temporary":
			assertWireBody(t, r, Object{"ConsoleType": "VNC"})
			fmt.Fprint(w, `{"ConsoleType":"VNC","AccessMode":"ReadWrite","Password":"temporary"}`)
			return
		case "/console/persistent":
			assertWireBody(t, r, Object{"ConsoleType": "VNC", "Password": "test-only"})
		default:
			t.Errorf("unexpected action %s", r.URL.Path)
		}
		w.WriteHeader(204)
	})
	if _, err := c.ChangeBootSource(ctx, "Pxe", BootOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Reset(ctx, ResetGracefulRestart, ResetOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateSecureBoot(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResetSecureBootKeys(ctx, "DeletePK"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ResetManager(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateSerialInterface(ctx, "one", SerialInterfaceUpdate{InterfaceEnabled: models.Ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.InsertVirtualMedia(ctx, "one", models.VirtualMediaInsertMediaRequest{Image: "https://images.invalid/os.iso", Inserted: models.Ptr(false), TransferMethod: models.Ptr("Stream")}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EjectVirtualMedia(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	console, err := c.GraphicalConsoleGetOneTimeURL(ctx, ConsoleOptions{ConsoleType: models.Ptr(ConsoleHTML5), ValidityPeriodSeconds: models.Ptr(int64(60))})
	if err != nil || console.ConsoleURL != "https://console.invalid/temporary" {
		t.Fatalf("console URL: %v", err)
	}
	if _, err = c.GraphicalConsoleSetOneTimePassword(ctx, ConsoleOptions{ConsoleType: models.Ptr(ConsoleVNC)}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.GraphicalConsoleSetPassword(ctx, ConsoleVNC, "test-only"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Reset(ctx, ResetForceOff, ResetOptions{}); err == nil {
		t.Fatal("unsupported reset was sent")
	}
	if writes != 11 {
		t.Fatalf("unexpected write count: %d", writes)
	}
}
