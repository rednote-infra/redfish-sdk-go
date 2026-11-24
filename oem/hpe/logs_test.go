package hpe_test

import (
	"net/url"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/hpe"
)

func TestLegacyHPLinkAndDateQueryValidation(t *testing.T) {
	extensions := models.Oem{"Hp": []byte(`{"Links":{"ActiveHealthSystem":{"@odata.id":"/legacy/ahs"}}}`)}
	policy := hpe.LogCollectStrategy{}
	uri, err := policy.DiagnosticServiceURI(&models.Manager{OEM: &extensions})
	if err != nil || uri != "/legacy/ahs" {
		t.Fatalf("legacy AHS: %q %v", uri, err)
	}
	if _, err := policy.DiagnosticArtifactURI([]byte(`{"Links":{"AHSLocation":{"extref":"/ahs"}}}`), provider.DiagnosticOptions{DownloadQuery: url.Values{"from": {"2026-01-01"}}}); err == nil {
		t.Fatal("partial date range accepted")
	}
	if _, err := policy.DiagnosticArtifactURI([]byte(`{"Links":{}}`), provider.DiagnosticOptions{}); err == nil {
		t.Fatal("missing AHS artifact link accepted")
	}
	if _, err := policy.DiagnosticServiceURI(&models.Manager{}); err == nil {
		t.Fatal("missing AHS service link accepted")
	}
}
