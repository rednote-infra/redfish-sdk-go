package lenovo_test

import (
	"github.com/rednote-infra/redfish-sdk-go/oem/lenovo"
	"testing"
)

func TestDiagnosticProgressReadsLenovoNamespace(t *testing.T) {
	policy := lenovo.LogCollectStrategy{}
	if policy.DiagnosticLayout().ProgressAction != "GetLogCollectProgress" {
		t.Fatal("incorrect progress action")
	}
	progress, err := policy.DecodeProgress([]byte(`{"Oem":{"Lenovo":{"Progress":100,"Status":5}}}`))
	if err != nil || !progress.Complete || progress.Failed {
		t.Fatalf("progress: %+v %v", progress, err)
	}
	progress, err = policy.DecodeProgress([]byte(`{"Oem":{"Public":{"Progress":100,"Status":1}}}`))
	if err != nil || !progress.Failed {
		t.Fatalf("failure: %+v %v", progress, err)
	}
}
