package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func newTestClient(t *testing.T, h http.HandlerFunc, options ...Option) (*Client, *httptest.Server) {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	c, err := NewClient(s.URL, "tester", "secret", options...)
	if err != nil {
		t.Fatal(err)
	}
	return c, s
}

func TestTypedResourcesFollowLinksAndPagination(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "tester" || p != "secret" {
			t.Error("missing Basic authentication")
		}
		switch r.URL.RequestURI() {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"Systems":{"@odata.id":"/systems"}}`)
		case "/systems":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/systems/a"}],"Members@odata.nextLink":"?page=2"}`)
		case "/systems?page=2":
			fmt.Fprint(w, `{"Members":[{"@odata.id":"/systems/b"}]}`)
		case "/systems/a":
			fmt.Fprint(w, `{"@odata.id":"/systems/a","Id":"a","Name":"Alpha","PowerState":"On"}`)
		case "/systems/b":
			fmt.Fprint(w, `{"@odata.id":"/systems/b","Id":"b","Name":"Beta"}`)
		default:
			http.NotFound(w, r)
		}
	})
	systems, err := c.GetSystems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(systems) != 2 || models.Value(systems[1].Name) != "Beta" {
		t.Fatalf("unexpected systems: %+v", systems)
	}
	if _, err = c.GetSystem(context.Background()); err == nil {
		t.Fatal("ambiguous selection must fail")
	}
	system, err := c.GetSystem(context.Background(), "b")
	if err != nil || models.Value(system.Name) != "Beta" {
		t.Fatalf("selection: %v %v", system, err)
	}
}

func TestMutationPreservesNullAndZeroValues(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, key := range []string{"Clear", "Enabled", "Count", "Name", "Items", "Options"} {
			if _, ok := body[key]; !ok {
				t.Errorf("missing %s", key)
			}
		}
		if body["Clear"] != nil || body["Enabled"] != false || body["Count"] != float64(0) {
			t.Errorf("changed values: %v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	_, err := c.Patch(context.Background(), "/resource", Object{"Clear": nil, "Enabled": false, "Count": 0, "Name": "", "Items": []string{}, "Options": Object{}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpstreamMessageIsUnchangedAndRawBodyAvailable(t *testing.T) {
	message := " Vendor rejected:  preserve punctuation! \n"
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Object{"error": Object{"code": "Base.Error", "message": message}})
	})
	_, err := c.GetRaw(context.Background(), "/resource")
	var rf *sdkerrors.Error
	if !errors.As(err, &rf) || rf.Error() != message || rf.StatusCode != 400 || len(rf.Body) == 0 {
		t.Fatalf("wrong error: %#v", err)
	}
}

func TestCrossOriginAndRedirectCannotLeakCredentials(t *testing.T) {
	var reached atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); t.Error("cross-origin target reached") }))
	defer other.Close()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) })
	if _, err := c.GetRaw(context.Background(), other.URL); err == nil {
		t.Fatal("cross-origin URL accepted")
	}
	if _, err := c.GetRaw(context.Background(), "/redirect"); err == nil {
		t.Fatal("redirect accepted")
	}
	if reached.Load() != 0 {
		t.Fatal("credentials leaked")
	}
}

func TestSessionRefreshIsSingleFlightAndCloseDeletesSession(t *testing.T) {
	var logins, deleted atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted.Add(1)
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/redfish/v1/" {
			fmt.Fprint(w, `{"Links":{"Sessions":{"@odata.id":"/sessions"}}}`)
			return
		}
		if r.URL.Path == "/sessions" {
			n := logins.Add(1)
			w.Header().Set(wire.HeaderAuthToken, fmt.Sprintf("token-%d", n))
			w.Header().Set("Location", "/sessions/current")
			w.WriteHeader(201)
			fmt.Fprint(w, `{"Id":"current"}`)
			return
		}
		if r.Header.Get(wire.HeaderAuthToken) == "token-1" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":{"message":"expired"}}`)
			return
		}
		fmt.Fprint(w, `{"Id":"ok"}`)
	}, WithAuthentication(AuthenticationSession))
	if err := c.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.GetRaw(context.Background(), "/protected"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if logins.Load() != 2 {
		t.Fatalf("login storm: %d", logins.Load())
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if deleted.Load() != 1 {
		t.Fatal("session not removed")
	}
}

func TestContextCancellationAndTLSDefaults(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer s.Close()
	c, err := NewClient(s.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.GetRaw(context.Background(), "/resource"); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	c, err = NewClient(s.URL, "", "", WithHTTPClient(s.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.GetRaw(context.Background(), "/resource"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.GetRaw(ctx, "/resource"); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost context cancellation: %v", err)
	}
}

func TestPaginationCycleAndMalformedCollection(t *testing.T) {
	for _, body := range []string{`{}`, `{"Members":[{}]}`, `{"Members":[],"Members@odata.nextLink":"/items"}`} {
		t.Run(body, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
			var out []Object
			if err := c.GetCollection(context.Background(), "/items", &out); err == nil {
				t.Fatal("invalid collection accepted")
			}
		})
	}
}

func TestRetryOnlySafeReads(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			var count int
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				count++
				if count == 1 {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, `{}`)
			}, WithRetry(RetryPolicy{MaxAttempts: 2, Backoff: time.Millisecond}))
			_, err := c.Request(context.Background(), method, "/resource", nil)
			if method == http.MethodGet && (err != nil || count != 2) {
				t.Fatalf("read retry failed: %d %v", count, err)
			}
			if method == http.MethodPost && (err == nil || count != 1) {
				t.Fatalf("unsafe mutation retried: %d %v", count, err)
			}
		})
	}
}

func TestSecretRedactionBeforeJSONEscaping(t *testing.T) {
	secret := "pass\"word\\line\n"
	payload := Object{"message": "echo " + secret, "Password": secret, "nested": []any{Object{"Token": "token"}}}
	safe, err := redaction.Redact(payload, secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(safe), "pass") || strings.Contains(string(safe), "token\"") {
		t.Fatalf("secret survived redaction: %s", safe)
	}
	if payload["Password"] != secret {
		t.Fatal("redaction modified raw payload")
	}
}

func TestBlankErrorMessageUsesFallback(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"  \n "}}`)
	})
	_, err := c.GetRaw(context.Background(), "/failure")
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("blank HTTP error: %v", err)
	}
}

func TestBrokenErrorBodyPreservesHTTPStatus(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		fmt.Fprint(connection, "HTTP/1.1 503 Service Unavailable\r\nTransfer-Encoding: chunked\r\n\r\n10\r\nbroken")
		connection.Close()
	})
	_, err := c.GetRaw(context.Background(), "/failure")
	if !sdkerrors.IsStatus(err, 503) {
		t.Fatalf("body read masked status: %v", err)
	}
}

func TestCachedRootCannotBeChangedByCallers(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"Systems":{"@odata.id":"/systems"},"ProtocolFeaturesSupported":{"Alias":true}}`)
	})
	first, err := c.Root(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	*first.Systems.ODataID = "/changed"
	*first.ProtocolFeaturesSupported.Alias = false
	second, err := c.Root(context.Background())
	if err != nil || models.Value(second.Systems.ODataID) != "/systems" || !models.Value(second.ProtocolFeaturesSupported.Alias) {
		t.Fatal("caller changed cached discovery data")
	}
}

func TestClientAndSensitiveModelsDoNotLeakCredentialsToFmt(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, c), "secret") {
			t.Fatal("client formatting leaked credentials")
		}
	}
}

func TestMutationErrorRedactsSubmittedPasswordWithoutChangingRawMessage(t *testing.T) {
	password := "file-server\\\"credential"
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(Object{"error": Object{"message": "rejected " + password}})
	})
	_, err := c.Post(context.Background(), "/update", Object{"Password": password})
	if err == nil || strings.Contains(err.Error(), password) {
		t.Fatalf("submitted password leaked: %v", err)
	}
	var rf *sdkerrors.Error
	if !errors.As(err, &rf) || rf.Message != "rejected "+password {
		t.Fatal("raw downstream message was changed")
	}
}

func TestSessionRefreshDoesNotReuseExpiredTokenForLoginDiscovery(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			if r.Header.Get(wire.HeaderAuthToken) == "expired" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"Links":{"Sessions":{"@odata.id":"/sessions"}}}`)
		case "/sessions":
			w.Header().Set(wire.HeaderAuthToken, "fresh")
			w.Header().Set(wire.HeaderLocation, "/sessions/fresh")
			w.WriteHeader(201)
		case "/protected":
			if r.Header.Get(wire.HeaderAuthToken) != "fresh" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"Id":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}, WithSession("expired", "/sessions/old"))
	if _, err := c.GetRaw(context.Background(), "/protected"); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitHeaderOnlySessionReturnsTokenWithoutSwitchingAuthentication(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"Links":{"Sessions":{"@odata.id":"/sessions"}}}`)
			return
		}
		w.Header().Set(wire.HeaderAuthToken, "created-token")
		w.Header().Set(wire.HeaderLocation, "/sessions/one")
		w.WriteHeader(201)
	})
	session, err := c.CreateSession(context.Background(), "tester", "test-only", false)
	if err != nil || models.Value(session.XAuthToken) != "created-token" || models.Value(session.ODataID) != "/sessions/one" {
		t.Fatalf("session metadata lost: %v", err)
	}
	if c.authenticationMode() != AuthenticationBasic {
		t.Fatal("explicit login switched authentication unexpectedly")
	}
}
