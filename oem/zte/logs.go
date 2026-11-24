package zte

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
		kind = wire.DiagnosticAllLogs
	}
	body := provider.Object{wire.FieldType: kind}
	return provider.ApplyDiagnosticOptions(body, options), nil
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{CollectAction: wire.ActionDump, DownloadAction: wire.ActionExportLogs, ProgressSuffix: "/Progress", ProbeBeforeCollect: true, ReuseProgress: true}
}
