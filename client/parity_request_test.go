package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestExplicitRequestControlsPreserveLegacyDefaults(t *testing.T) {
	var writes atomic.Int32
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "/target")
			w.WriteHeader(http.StatusFound)
		case "/target":
			fmt.Fprint(w, `{"result":"ok"}`)
		case "/write":
			if writes.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				fmt.Fprint(w, `{}`)
			}
		case "/slow":
			time.Sleep(30 * time.Millisecond)
			fmt.Fprint(w, `{}`)
		}
	})
	if _, err := sdk.Request(context.Background(), http.MethodGet, "/redirect", nil); err == nil {
		t.Fatal("legacy redirect errors must remain compatible")
	}
	response, err := sdk.Request(context.Background(), http.MethodGet, "/redirect", nil, RequestOptions{ReturnRedirects: true})
	if err != nil || response.StatusCode != http.StatusFound {
		t.Fatalf("return redirect: %v", err)
	}
	response, err = sdk.Request(context.Background(), http.MethodGet, "/redirect", nil, RequestOptions{AllowRedirects: true})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("follow redirect: %v", err)
	}
	policy := RetryPolicy{MaxAttempts: 2, Safety: RetrySafetyIdempotent}
	if _, err = sdk.Request(context.Background(), http.MethodPatch, "/write", Object{"Enabled": true}, RequestOptions{Retry: &policy}); err != nil || writes.Load() != 2 {
		t.Fatalf("idempotent replay: writes=%d err=%v", writes.Load(), err)
	}
	_, err = sdk.Request(context.Background(), http.MethodGet, "/slow", nil, RequestOptions{ReadTimeout: 5 * time.Millisecond})
	if err == nil {
		t.Fatal("read timeout was ignored")
	}
	if _, err = sdk.Request(context.Background(), http.MethodGet, "/slow", nil); err != nil {
		t.Fatalf("timeout override changed the client: %v", err)
	}
}

func TestRedirectControlsCannotSendCredentialsToAnotherOrigin(t *testing.T) {
	var externalCalls atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { externalCalls.Add(1) }))
	defer outside.Close()
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", outside.URL)
		w.WriteHeader(http.StatusFound)
	})
	if _, err := sdk.Request(context.Background(), http.MethodGet, "/redirect", nil, RequestOptions{AllowRedirects: true}); err == nil || externalCalls.Load() != 0 {
		t.Fatal("external redirect was followed")
	}
}

type readTimeoutError struct{}

func (readTimeoutError) Error() string   { return "offline read timeout" }
func (readTimeoutError) Timeout() bool   { return true }
func (readTimeoutError) Temporary() bool { return true }

func TestReadTimeoutReplayRequiresExplicitPolicy(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, readTimeoutError{} })
	sdk, err := NewClient("https://example.invalid", "", "", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	policy := RetryPolicy{MaxAttempts: 2, RetryOnReadTimeout: true}
	_, err = sdk.Request(context.Background(), http.MethodGet, "/resource", nil, RequestOptions{Retry: &policy})
	var timeout readTimeoutError
	if !errors.As(err, &timeout) || calls != 2 {
		t.Fatalf("read timeout retry: calls=%d err=%v", calls, err)
	}
	calls = 0
	_, _ = sdk.Request(context.Background(), http.MethodPost, "/resource", Object{}, RequestOptions{Retry: &policy})
	if calls != 1 {
		t.Fatal("unsafe writes must not be replayed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConnectTimeoutOverrideDoesNotMutateTransport(t *testing.T) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("connect timeout did not set a dial deadline")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	sdk, err := NewClient("http://example.invalid", "", "", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	_, err = sdk.Request(context.Background(), http.MethodGet, "/resource", nil, RequestOptions{ConnectTimeout: time.Millisecond})
	if err == nil {
		t.Fatal("connect timeout was ignored")
	}
	if transport.ResponseHeaderTimeout != 0 {
		t.Fatal("caller-owned transport was modified")
	}
}
