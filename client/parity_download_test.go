package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

func TestExternalArtifactUsesSeparateCredentialFreeClient(t *testing.T) {
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"Authorization", wire.HeaderAuthToken, "Cookie"} {
			if r.Header.Get(header) != "" {
				t.Errorf("BMC credential forwarded: %s", header)
			}
		}
		fmt.Fprint(w, "sample archive")
	}))
	defer outside.Close()
	bmc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("external artifact must not contact the BMC") }))
	defer bmc.Close()
	jar, _ := cookiejar.New(nil)
	origin, _ := url.Parse(bmc.URL)
	jar.SetCookies(origin, []*http.Cookie{{Name: "bmc_cookie", Value: "sample-cookie"}})
	sdk, err := NewClient(bmc.URL, "sample-user", "sample-password", WithHTTPClient(&http.Client{Jar: jar}), WithSession("sample-token", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	data, err := sdk.DownloadBytes(context.Background(), outside.URL+"/artifact")
	if err != nil || string(data) != "sample archive" {
		t.Fatalf("memory download: %q %v", data, err)
	}
	path := filepath.Join(t.TempDir(), "new", "bundle.tgz")
	actual, err := sdk.Download(context.Background(), outside.URL+"/artifact", path)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(actual)
	if err != nil || string(data) != "sample archive" {
		t.Fatalf("file download: %v", err)
	}
}

func TestExternalTransportCannotReuseBMCClient(t *testing.T) {
	transport := &http.Client{}
	if sdk, err := NewClient("https://example.invalid", "", "", WithHTTPClient(transport), WithExternalHTTPClient(transport)); err == nil {
		sdk.Close()
		t.Fatal("external and BMC client must be distinct")
	}
}

func TestDiagnosticDataSupportsMemoryDownloads(t *testing.T) {
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "diagnostic bytes") })
	data, err := sdk.DownloadDiagnosticDataBytes(context.Background(), &DiagnosticTask{DownloadURI: "/artifact", Vendor: VendorGeneric})
	if err != nil || string(data) != "diagnostic bytes" {
		t.Fatalf("diagnostic memory download: %q %v", data, err)
	}
	provider := memoryDiagnosticProvider{}
	if err := sdk.Providers().RegisterLogCollect(VendorGeneric, provider); err != nil {
		t.Fatal(err)
	}
	sdk.cfg.vendor = VendorGeneric
	data, err = sdk.CollectAndDownloadDiagnosticDataBytes(context.Background(), CollectDownloadOptions{DisableReuse: true})
	if err != nil || string(data) != "provider bytes" {
		t.Fatalf("collect-and-download memory workflow: %q %v", data, err)
	}
}

type memoryDiagnosticProvider struct{}

func (memoryDiagnosticProvider) BuildBody(DiagnosticOptions) (Object, error)     { return Object{}, nil }
func (memoryDiagnosticProvider) SelectService([]models.Log) (*models.Log, error) { return nil, nil }
func (memoryDiagnosticProvider) Collect(context.Context, *Client, DiagnosticOptions) (*DiagnosticTask, error) {
	return &DiagnosticTask{Vendor: VendorGeneric}, nil
}
func (memoryDiagnosticProvider) Wait(context.Context, *Client, *DiagnosticTask, PollOptions) error {
	return nil
}
func (memoryDiagnosticProvider) Download(context.Context, *Client, *DiagnosticTask, string) (string, error) {
	return "", errors.New("memory workflow must not write a file")
}
func (memoryDiagnosticProvider) DownloadBytes(context.Context, *Client, *DiagnosticTask) ([]byte, error) {
	return []byte("provider bytes"), nil
}

func TestBMCArtifactRedirectSwitchesToCredentialFreeTransport(t *testing.T) {
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get(wire.HeaderAuthToken) != "" || r.Header.Get("Cookie") != "" {
			t.Error("artifact redirect leaked BMC credentials")
		}
		fmt.Fprint(w, "redirected archive")
	}))
	defer outside.Close()
	bmc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(wire.HeaderAuthToken) != "sample-token" {
			t.Error("initial BMC request was not authenticated")
		}
		w.Header().Set(wire.HeaderLocation, outside.URL+"/artifact")
		w.WriteHeader(http.StatusFound)
	}))
	defer bmc.Close()
	sdk, err := NewClient(bmc.URL, "sample-user", "sample-password", WithSession("sample-token", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.Close()
	data, err := sdk.DownloadBytes(context.Background(), "/redirect")
	if err != nil || string(data) != "redirected archive" {
		t.Fatalf("artifact redirect: %q %v", data, err)
	}
}
