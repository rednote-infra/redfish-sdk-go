package zte_test

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/oem/zte"
	"testing"
)

func TestDiagnosticUsesAdvertisedDumpAndExportActions(t *testing.T) {
	policy := zte.LogCollectStrategy{}
	layout := policy.DiagnosticLayout()
	if layout.CollectAction != "Dump" || layout.DownloadAction != "ExportLogs" || layout.ProgressSuffix != "/Progress" || !layout.ProbeBeforeCollect {
		t.Fatalf("layout: %+v", layout)
	}
	var reader provider.DiagnosticProgressReader = policy
	progress, err := reader.DecodeProgress([]byte(`{"State":"STATE_COMPLETED","Percentage":"100","TarPath":"/synthetic/log.tar","Type":"AllLogs"}`))
	if err != nil || !progress.Complete || progress.DownloadBody["Content"] != "/synthetic/log.tar" {
		t.Fatalf("progress: %+v %v", progress, err)
	}
}

func TestUnknownErrorStateIsNotReusable(t *testing.T) {
	progress, err := (zte.LogCollectStrategy{}).DecodeProgress([]byte(`{"State":"VENDOR_ERROR_PENDING","Percentage":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if progress.Reusable {
		t.Fatal("error state must not be reused")
	}
}
