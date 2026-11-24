package provider

import (
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// DiagnosticLayout describes resource and action differences, independent of HTTP execution.
type DiagnosticLayout struct {
	MonitorFromLocation      bool
	SystemScope              bool
	SelectService            bool
	CollectAction            string
	FallbackCollectAction    string
	UseManagerActions        bool
	ManagerServiceSuffix     string
	DownloadAction           string
	ProgressAction           string
	ProgressSuffix           string
	ProbeBeforeCollect       bool
	ReuseTasks               bool
	ReuseProgress            bool
	AllowTaskVisibilityDelay bool
}

// DiagnosticResourceStrategy describes the vendor resource layout used by the generic client.
type DiagnosticResourceStrategy interface{ DiagnosticLayout() DiagnosticLayout }

// DiagnosticProgress retains the vendor's terminal state and download metadata.
type DiagnosticProgress struct {
	Response         *sdkresponse.Response
	DownloadURI      string
	Complete, Failed bool
	// Reusable reports a running collection that can be safely reused.
	Reusable bool
	// CompletionWins preserves established SDK precedence for conflicting terminal indicators.
	CompletionWins  bool
	State           string
	Percentage      int
	PercentageValid bool
	Artifact        string
	DownloadBody    Object
	FailureMessage  string
}

// DiagnosticProgressReader decodes a vendor's actual progress response contract.
type DiagnosticProgressReader interface {
	DecodeProgress([]byte) (DiagnosticProgress, error)
}

// DiagnosticDownloadBuilder builds the vendor-specific download action body.
type DiagnosticDownloadBuilder interface{ BuildDownloadBody(Object) Object }

func (DefaultLogCollectStrategy) DiagnosticLayout() DiagnosticLayout {
	return DiagnosticLayout{CollectAction: wire.ActionCollectDiagnosticData, SelectService: true, ReuseTasks: true}
}

// DiagnosticServiceResolver follows a vendor-advertised service link.
type DiagnosticServiceResolver interface {
	DiagnosticServiceURI(*models.Manager) (string, error)
}

// DiagnosticArtifactSource exposes an existing binary artifact without triggering a collection POST.
type DiagnosticArtifactSource interface {
	DiagnosticServiceResolver
	DiagnosticArtifactURI([]byte, DiagnosticOptions) (string, error)
}

// DiagnosticResponseReader decodes monitor bodies and headers supplied by the service.
type DiagnosticResponseReader interface {
	DecodeDiagnosticResponse(*sdkresponse.Response) (DiagnosticProgress, error)
}
