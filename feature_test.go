package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

func TestGPUDiscoveryMergesStandardTopology(t *testing.T) {
	payloads := map[string]string{
		"/redfish/v1/": `{"Systems":{"@odata.id":"/systems"}}`,
		"/systems":     `{"Members":[{"@odata.id":"/system/a","GraphicsControllers":{"@odata.id":"/graphics"},"Processors":{"@odata.id":"/processors"},"PCIeDevices":[{"@odata.id":"/pcie/gpu"}]}]}`,
		"/graphics":    `{"Members":[{"@odata.id":"/graphics/gpu","Name":"Accelerator","Links":{"Processors":[{"@odata.id":"/processors/gpu"}]}}]}`,
		"/processors":  `{"Members":[{"@odata.id":"/processors/gpu","ProcessorType":"GPU","Model":"Accelerator-X","TDPWatts":400,"Links":{"PCIeDevice":{"@odata.id":"/pcie/gpu"}}}]}`,
		"/pcie/gpu":    `{"@odata.id":"/pcie/gpu","DeviceType":"GPU","SerialNumber":"synthetic-gpu"}`,
	}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if payload, ok := payloads[r.URL.Path]; ok {
			fmt.Fprint(w, payload)
		} else {
			http.NotFound(w, r)
		}
	})
	gpus, err := c.GetGPUs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 1 || Value(gpus[0].Model) != "Accelerator-X" || len(gpus[0].SourceTypes) != 3 {
		t.Fatalf("GPU representations were not merged: %+v", gpus)
	}
}
func TestResourceDiscoveryBoundsFilteringAndExternalLinks(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			fmt.Fprint(w, `{"@odata.id":"/redfish/v1/","@odata.type":"#ServiceRoot.v1_0_0.ServiceRoot","Systems":{"@odata.id":"/systems"},"External":{"@odata.id":"https://external.invalid/redfish/v1/"}}`)
		} else {
			fmt.Fprint(w, `{"@odata.id":"/systems","@odata.type":"#ComputerSystem.v1_0_0.ComputerSystem","PowerState":"On","Root":{"@odata.id":"/redfish/v1/"}}`)
		}
	})
	index, err := c.DiscoverResources(context.Background(), DiscoveryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(index.Resources) != 2 || !index.IsPartial() {
		t.Fatalf("unexpected discovery: %+v", index)
	}
	if got := index.Find("ComputerSystem", Object{"PowerState": "On"}); len(got.Resources) != 1 {
		t.Fatal("type/property filter failed")
	}
}
func TestRegistryResolutionSelectsHighestErrataAndFormatsMessage(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Registries":{"@odata.id":"/registries"}}`)
		case "/registries":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/registry/old","Id":"Base.1.0.0","Registry":"Base.1.0.0","Location":[{"Language":"en","Uri":"/old"}]},{"@odata.id":"/registry/new","Id":"Base.1.0.2","Registry":"Base.1.0.2","Location":[{"Language":"en","Uri":"/new"}]}]}`)
		case "/new":
			fmt.Fprint(w, `{"RegistryPrefix":"Base","RegistryVersion":"1.0.2","Messages":{"Changed":{"Message":"Changed %1 to %2.","NumberOfArgs":2,"Severity":"OK"}}}`)
		default:
			t.Errorf("unexpected registry URI: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	result, err := c.ResolveMessage(context.Background(), "Base.1.0.Changed", []any{"Mode", "Auto"}, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "Changed Mode to Auto." || Value(result.MessageSeverity) != "OK" || result.RegistryVersion != "1.0.2" {
		t.Fatalf("wrong resolution: %+v", result)
	}
}
func TestAttributeValidationRejectsUnknownReadOnlyAndWrongTypes(t *testing.T) {
	data, err := os.ReadFile("testdata/attribute_registry.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry models.AttributeRegistry
	if err = json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if registry.RegistryEntries == nil {
		t.Fatal("fixture has no attributes")
	}
	for _, definition := range registry.RegistryEntries.Attributes {
		if definition.Type != nil && *definition.Type == "Integer" && !Value(definition.ReadOnly) {
			if err = ValidateAttributeValues(&registry, Object{definition.AttributeName: true}); err == nil {
				t.Fatal("bool was accepted as integer")
			}
		}
	}
	if err = ValidateAttributeValues(&registry, Object{"NotAdvertised": 1}); err == nil {
		t.Fatal("unknown attribute accepted")
	}
}
func TestAliasesRequireCapabilityAndUseStandardClearValue(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			fmt.Fprint(w, `{"ProtocolFeaturesSupported":{"Alias":true}}`)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set(HeaderETag, `"v1"`)
			fmt.Fprint(w, `{}`)
			return
		}
		var body Object
		json.NewDecoder(r.Body).Decode(&body)
		if body["@Redfish.Alias"] != "" || r.Header.Get(HeaderIfMatch) != `"v1"` {
			t.Error("incorrect alias clear")
		}
		w.WriteHeader(204)
	})
	uri, err := c.ResourceAliasURI(context.Background(), "/systems", "my-server")
	if err != nil || uri != "/systems/~my-server" {
		t.Fatalf("alias addressing: %s %v", uri, err)
	}
	if _, err = c.ResourceAliasURI(context.Background(), "/systems", "bad/alias"); err == nil {
		t.Fatal("unsafe alias accepted")
	}
	if _, err = c.ClearResourceAlias(context.Background(), "/system"); err != nil {
		t.Fatal(err)
	}
}
func TestRestrictedExplicitLoginRedactsAttemptedSecrets(t *testing.T) {
	attemptedPassword := "new\"password\\"
	token := "new-token"
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"Links":{"Sessions":{"@odata.id":"/sessions"}}}`)
			return
		}
		w.Header().Set(HeaderAuthToken, token)
		w.Header().Set(HeaderLocation, "/sessions/current")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Object{"Id": "current", "error": Object{"message": "change " + attemptedPassword + " " + token, "@Message.ExtendedInfo": []Object{{"MessageId": "Base.1.0.PasswordChangeRequired"}}}})
	})
	_, err := c.CreateSession(context.Background(), "new-user", attemptedPassword, true)
	if !errors.Is(err, ErrPasswordChangeRequired) {
		t.Fatalf("missing restriction error: %v", err)
	}
	for _, format := range []string{"%s", "%v", "%+v", "%#v"} {
		display := fmt.Sprintf(format, err)
		if strings.Contains(display, token) || strings.Contains(display, attemptedPassword) {
			t.Fatalf("credential leak: %s", display)
		}
	}
	var rf *Error
	if !errors.As(err, &rf) || !strings.Contains(string(rf.Body), token) {
		t.Fatal("raw response was modified")
	}
	if c.authenticationMode() != AuthenticationSession {
		t.Fatal("explicit login did not enable session refresh")
	}
}
func TestSSDPUsesRedfishALAndRejectsUnrelatedOrDuplicateHeaders(t *testing.T) {
	valid := "HTTP/1.1 200 OK\r\nST: " + ssdpServiceTarget + "\r\nUSN: uuid:12345678-1234-1234-1234-123456789abc::" + ssdpServiceTarget + "\r\nAL: <https://example.invalid/redfish/v1/>\r\nLOCATION: http://other.invalid/upnp.xml\r\n\r\n"
	service, err := ParseSSDPResponse([]byte(valid), "192.0.2.1")
	if err != nil || service.Origin() != "https://example.invalid" {
		t.Fatalf("discovery: %+v %v", service, err)
	}
	if _, err = ParseSSDPResponse([]byte(strings.Replace(valid, "AL:", "ST: duplicate\r\nAL:", 1)), ""); err == nil {
		t.Fatal("duplicate ST accepted")
	}
	if _, err = ParseSSDPResponse([]byte(strings.Replace(valid, "AL:", "X-AL:", 1)), ""); err == nil {
		t.Fatal("LOCATION was used instead of AL")
	}
}

func TestDeletionCannotBypassAdvertisedSubscriptionMembership(t *testing.T) {
	deletes := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes++
			w.WriteHeader(204)
			return
		}
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"EventService":{"@odata.id":"/events"}}`)
		case "/events":
			fmt.Fprint(w, `{"Subscriptions":{"@odata.id":"/subscriptions"}}`)
		case "/subscriptions":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/subscriptions/1","Id":"1"}]}`)
		default:
			fmt.Fprint(w, `{"@odata.id":"/systems/1"}`)
		}
	})
	err := c.DeleteSubscription(context.Background(), "/systems/1")
	if !errors.Is(err, ErrNotFound) || deletes != 0 {
		t.Fatalf("subscription selection bypassed membership: %d %v", deletes, err)
	}
}

func TestVirtualMediaFollowsOEMFallback(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Managers":{"@odata.id":"/managers"}}`)
		case "/managers":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/manager","Oem":{"Public":{"VirtualMedia":{"@odata.id":"/media"}}}}]}`)
		case "/media":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/media/cd","Id":"cd","Name":"Virtual CD"}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	media, err := c.GetVirtualMediaResource(context.Background(), "cd")
	if err != nil || Value(media.Name) != "Virtual CD" {
		t.Fatalf("OEM virtual media: %v", err)
	}
}
