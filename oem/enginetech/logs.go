package enginetech

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

// LogCollectStrategy implements this vendor's diagnostic resource policy.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	kind := options.DiagnosticDataType
	if kind == "" {
		kind = "OEM"
	}
	body := provider.Object{wire.FieldDiagnosticDataType: kind, wire.FieldOEMDiagnosticDataType: "BlackBox"}
	return provider.ApplyDiagnosticOptions(body, options), nil
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{SystemScope: true, CollectAction: wire.ActionCollectDiagnosticData}
}
