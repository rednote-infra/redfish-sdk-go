package dell_test

import (
	"net/http"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/dell"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func TestSupportAssistFailureMessagesAndMissingArtifacts(t *testing.T) {
	policy := dell.LogCollectStrategy{}
	for _, body := range []string{`{"JobState":"Failed","Message":"  preserve this message!  "}`, `{"Oem":{"Dell":{"JobState":"CompletedWithErrors","Message":"  preserve this message!  "}}}`} {
		progress, err := policy.DecodeDiagnosticResponse(&sdkresponse.Response{StatusCode: http.StatusOK, Body: []byte(body)})
		if err != nil || !progress.Failed || progress.FailureMessage != "  preserve this message!  " {
			t.Fatalf("failure: %+v %v", progress, err)
		}
	}
	if _, err := policy.DecodeDiagnosticResponse(&sdkresponse.Response{StatusCode: http.StatusOK, Body: []byte(`{"JobState":"Completed"}`)}); err == nil {
		t.Fatal("completion without an archive must fail")
	}
	if _, err := policy.BuildBody(provider.DiagnosticOptions{OEMParameters: provider.Object{"ShareType": "CIFS"}}); err == nil {
		t.Fatal("bundle download cannot silently export to a remote share")
	}
}
func TestSupportAssistResolvesRelativeLocation(t *testing.T) {
	progress, err := (dell.LogCollectStrategy{}).DecodeDiagnosticResponse(&sdkresponse.Response{StatusCode: http.StatusNoContent, URL: "https://example.invalid/jobs/one", Header: http.Header{"Location": []string{"../archives/sacollect.zip"}}})
	if err != nil || !progress.Complete || progress.DownloadURI != "https://example.invalid/archives/sacollect.zip" {
		t.Fatalf("relative archive: %+v %v", progress, err)
	}
}
func TestMissingDellServiceDoesNotInventAPath(t *testing.T) {
	if _, err := dell.LCServiceURI(&models.Manager{}); err == nil {
		t.Fatal("missing service link was accepted")
	}
}
