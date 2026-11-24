package dell

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const (
	_collectAction          = "SupportAssistCollection"
	_shareType              = "ShareType"
	_localShare             = "Local"
	_jobCompleted           = "Completed"
	_jobCompletedWithErrors = "CompletedWithErrors"
	_jobFailed              = "Failed"
	_taskException          = "Exception"
	_taskCancelled          = "Cancelled"
	_taskKilled             = "Killed"
	_archiveExtension       = ".zip"
)

// LogCollectStrategy collects a local SupportAssist bundle through the advertised LC service.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	body := provider.Object{_shareType: _localShare}
	for key, value := range options.OEMParameters {
		body[key] = value
	}
	if body[_shareType] != _localShare {
		return nil, errors.New("SupportAssist bundle downloads require ShareType Local")
	}
	return body, nil
}
func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{CollectAction: _collectAction, MonitorFromLocation: true}
}
func (LogCollectStrategy) DiagnosticServiceURI(manager *models.Manager) (string, error) {
	return LCServiceURI(manager)
}

type jobFields struct {
	JobState string `json:"JobState"`
	Message  string `json:"Message"`
}
type jobResponse struct {
	jobFields
	TaskState string `json:"TaskState"`
	OEM       struct {
		Dell *jobFields `json:"Dell"`
	} `json:"Oem"`
}

// DecodeDiagnosticResponse handles both iDRAC9 jobs and iDRAC10 OEM job status.
func (LogCollectStrategy) DecodeDiagnosticResponse(response *sdkresponse.Response) (provider.DiagnosticProgress, error) {
	var raw jobResponse
	if len(strings.TrimSpace(string(response.Body))) > 0 {
		if err := json.Unmarshal(response.Body, &raw); err != nil {
			return provider.DiagnosticProgress{}, err
		}
	}
	fields := raw.jobFields
	if raw.OEM.Dell != nil {
		if raw.OEM.Dell.JobState != "" {
			fields.JobState = raw.OEM.Dell.JobState
		}
		if strings.TrimSpace(raw.OEM.Dell.Message) != "" {
			fields.Message = raw.OEM.Dell.Message
		}
	}
	state := fields.JobState
	if state == "" {
		state = raw.TaskState
	}
	result := provider.DiagnosticProgress{State: state}
	switch state {
	case _jobFailed, _jobCompletedWithErrors, _taskException, _taskCancelled, _taskKilled:
		result.Failed = true
		result.FailureMessage = fields.Message
		if strings.TrimSpace(result.FailureMessage) == "" {
			result.FailureMessage = "Dell SupportAssist collection failed"
		}
		return result, nil
	}
	location := response.Header.Get(wire.HeaderLocation)
	if location != "" {
		target, err := url.Parse(location)
		if err != nil {
			return result, err
		}
		terminal := state == _jobCompleted || response.StatusCode == http.StatusNoContent || strings.EqualFold(path.Ext(target.Path), _archiveExtension)
		if terminal {
			if response.URL != "" {
				base, err := url.Parse(response.URL)
				if err != nil {
					return result, err
				}
				target = base.ResolveReference(target)
			}
			result.Complete = true
			result.DownloadURI = target.String()
			return result, nil
		}
	}
	if state == _jobCompleted || response.StatusCode == http.StatusNoContent {
		return result, errors.New("Dell SupportAssist completed without an archive Location")
	}
	if state == "" {
		return result, errors.New("Dell SupportAssist response does not advertise JobState or archive Location")
	}
	return result, nil
}
