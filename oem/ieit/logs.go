package ieit

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

// LogCollectStrategy implements this vendor's diagnostic resource policy.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	body := provider.Object{}
	return provider.ApplyDiagnosticOptions(body, options), nil
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{CollectAction: wire.ActionCollectAllLog, DownloadAction: wire.ActionDownloadAllLog, ReuseTasks: true, AllowTaskVisibilityDelay: true}
}
