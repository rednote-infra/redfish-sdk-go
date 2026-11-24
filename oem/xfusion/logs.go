package xfusion

import (
	"fmt"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

// LogCollectStrategy implements this vendor's diagnostic resource policy.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	body := provider.Object{wire.FieldType: wire.DiagnosticOutputURI, wire.FieldContent: fmt.Sprintf("/tmp/web/xf_sdk_%d.tar.gz", time.Now().UnixNano())}
	return provider.ApplyDiagnosticOptions(body, options), nil
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{CollectAction: wire.ActionDump, FallbackCollectAction: "ExportLog", UseManagerActions: true, DownloadAction: wire.ActionGeneralDownload}
}

func (LogCollectStrategy) BuildDownloadBody(body provider.Object) provider.Object {
	return provider.Object{wire.FieldTransferProtocol: wire.TransferProtocolHTTPS, wire.FieldPath: body[wire.FieldContent]}
}
