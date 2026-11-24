package dell_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/dell"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

type serviceTransport struct {
	polls   int
	failure error
}

func (t *serviceTransport) GetManager(context.Context, ...string) (*models.Manager, error) {
	var manager models.Manager
	err := json.Unmarshal([]byte(`{"Links":{"Oem":{"Dell":{"DellLCService":{"@odata.id":"/lc"}}}},"Actions":{"Oem":{"#OemManager.ExportSystemConfiguration":{"target":"/export"}}}}`), &manager)
	return &manager, err
}
func (t *serviceTransport) Get(_ context.Context, uri string, dest any) error {
	if t.failure != nil {
		return t.failure
	}
	if uri != "/lc" {
		return errors.New("unexpected service URI")
	}
	return json.Unmarshal([]byte(`{"Actions":{"#DellLCService.GetRemoteServicesAPIStatus":{"target":"/status"}}}`), dest)
}
func (t *serviceTransport) Post(_ context.Context, uri string, _ any) (*sdkresponse.Response, error) {
	switch uri {
	case "/status":
		return &sdkresponse.Response{StatusCode: http.StatusOK, Body: []byte(`{"Status":"Ready"}`)}, nil
	case "/export":
		return &sdkresponse.Response{StatusCode: http.StatusAccepted, Header: http.Header{"Location": []string{"/monitor"}}}, nil
	}
	return nil, errors.New("unexpected action URI")
}
func (t *serviceTransport) GetResponse(context.Context, string) (*sdkresponse.Response, error) {
	t.polls++
	if t.polls == 1 {
		return &sdkresponse.Response{StatusCode: http.StatusAccepted, Body: []byte(`{"TaskState":"Running"}`)}, nil
	}
	return &sdkresponse.Response{StatusCode: http.StatusOK, Body: []byte(`<SystemConfiguration/>`)}, nil
}
func (t *serviceTransport) RedactKnownSecrets(message string) string {
	return strings.ReplaceAll(message, "synthetic-secret", "[REDACTED]")
}

func TestDellServiceOwnsStatusExportAndPolling(t *testing.T) {
	transport := &serviceTransport{}
	service := dell.NewService(transport)
	status, err := service.GetRemoteServicesAPIStatus(context.Background())
	if err != nil || !status.IsReady() {
		t.Fatalf("LC status: %+v %v", status, err)
	}
	operation, err := service.ExportSystemConfiguration(context.Background(), models.DellExportSystemConfigurationRequest{ShareParameters: models.DellShareParameters{Target: "ALL"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := service.WaitForSystemConfiguration(context.Background(), operation, provider.PollSettings{Interval: time.Millisecond, Timeout: time.Second})
	if err != nil || string(data) != `<SystemConfiguration/>` || transport.polls != 2 {
		t.Fatalf("export: %q %v polls=%d", data, err, transport.polls)
	}
}
func TestDellServicePreservesTransportFailure(t *testing.T) {
	cause := errors.New("  original downstream message  ")
	_, err := dell.NewService(&serviceTransport{failure: cause}).GetRemoteServicesAPIStatus(context.Background())
	if err != cause {
		t.Fatalf("transport error was replaced: %v", err)
	}
}
