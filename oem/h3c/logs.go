package h3c

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

// LogCollectStrategy implements this vendor's diagnostic resource policy.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	body := provider.Object{wire.FieldType: wire.DiagnosticOutputURI, wire.FieldContent: wire.H3CDiagnosticPath}
	if options.DiagnosticDataType != "" {
		body["LogType"] = options.DiagnosticDataType
	}
	return provider.ApplyDiagnosticOptions(body, options), nil
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{CollectAction: wire.ActionDump, UseManagerActions: true, DownloadAction: wire.ActionGeneralDownload}
}

func (LogCollectStrategy) BuildDownloadBody(body provider.Object) provider.Object {
	return provider.Object{wire.FieldTransferProtocol: wire.TransferProtocolHTTPS, wire.FieldContent: wire.H3CDiagnosticPath}
}
