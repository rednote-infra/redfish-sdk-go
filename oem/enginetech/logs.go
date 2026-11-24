package enginetech

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

const (
	_blackBoxLogServiceID  = "BlackBox"
	_latestEntryID         = "Latest"
	_diagnosticDataTypeOEM = "OEM"
)

// LogCollectStrategy implements this vendor's diagnostic resource policy.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

var (
	_ provider.LogCollectStrategy         = LogCollectStrategy{}
	_ provider.DiagnosticArtifactResolver = LogCollectStrategy{}
	_ provider.DiagnosticTaskValidator    = LogCollectStrategy{}
)

func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	kind := options.DiagnosticDataType
	if kind == "" {
		kind = _diagnosticDataTypeOEM
	}
	body := provider.Object{wire.FieldDiagnosticDataType: kind, wire.FieldOEMDiagnosticDataType: _blackBoxLogServiceID}
	return provider.ApplyDiagnosticOptions(body, options), nil
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	// Latest is overwritten by every collection, so existing tasks must never be reused.
	return provider.DiagnosticLayout{SystemScope: true, CollectAction: wire.ActionCollectDiagnosticData, DefaultLogServiceID: _blackBoxLogServiceID}
}

// ResolveDiagnosticArtifact follows BlackBox's advertised Entries link to the fixed Latest entry.
func (LogCollectStrategy) ResolveDiagnosticArtifact(ctx context.Context, client provider.DiagnosticResourceClient, _ *models.Task) (string, error) {
	system, err := client.GetSystem(ctx)
	if err != nil {
		return "", err
	}
	if system.LogServices == nil || strings.TrimSpace(models.Value(system.LogServices.ODataID)) == "" {
		return "", errors.New("Enginetech System has no LogServices link")
	}
	service, err := client.GetLogService(ctx, models.Value(system.LogServices.ODataID), _blackBoxLogServiceID)
	if err != nil {
		return "", err
	}
	if service.Entries == nil || strings.TrimSpace(models.Value(service.Entries.ODataID)) == "" {
		return "", errors.New("Enginetech BlackBox LogService has no Entries link")
	}
	entryURI, err := url.JoinPath(models.Value(service.Entries.ODataID), _latestEntryID)
	if err != nil {
		return "", err
	}
	var entry models.LogEntry
	if err := client.Get(ctx, entryURI, &entry); err != nil {
		return "", err
	}
	if strings.TrimSpace(models.Value(entry.AdditionalDataURI)) == "" {
		return "", errors.New("Enginetech BlackBox entry " + entryURI + " has no AdditionalDataURI")
	}
	return models.Value(entry.AdditionalDataURI), nil
}

// ValidateDiagnosticTask requires successful completion before reading the mutable Latest entry.
func (LogCollectStrategy) ValidateDiagnosticTask(task *models.Task) error {
	state, status := models.Value(task.TaskState), models.Value(task.TaskStatus)
	if state == models.TaskCompleted && (status == "" || status == models.TaskStatusOK) {
		return nil
	}
	message := "Enginetech diagnostic task did not complete successfully: state=" + state + ", status=" + status
	for _, info := range task.Messages {
		if strings.TrimSpace(models.Value(info.Message)) != "" {
			message = models.Value(info.Message)
			break
		}
	}
	return &sdkerrors.AsyncOperationError{Task: task, Message: message}
}
