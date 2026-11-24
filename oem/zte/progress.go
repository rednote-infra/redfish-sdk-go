package zte

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

const (
	_stateCompleted = "STATE_COMPLETED"
	_stateSuccess   = "STATE_SUCCESS"
	_stateSucceeded = "STATE_SUCCESSED"
	_stateFinish    = "STATE_FINISH"
	_stateFailed    = "STATE_FAILED"
	_stateError     = "STATE_ERROR"
)

type progressResponse struct {
	State      string                `json:"State"`
	Percentage models.StringOrNumber `json:"Percentage"`
	TarPath    string                `json:"TarPath"`
	Type       string                `json:"Type"`
}

func (LogCollectStrategy) DecodeProgress(data []byte) (provider.DiagnosticProgress, error) {
	var raw progressResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return provider.DiagnosticProgress{}, err
	}
	percentage, err := strconv.Atoi(string(raw.Percentage))
	result := provider.DiagnosticProgress{State: raw.State, Percentage: percentage, PercentageValid: err == nil, Artifact: raw.TarPath}
	result.Complete = slices.Contains([]string{_stateCompleted, _stateSuccess, _stateSucceeded, _stateFinish}, raw.State) || percentage >= 100
	result.Failed = raw.State == _stateFailed || raw.State == _stateError
	result.CompletionWins = true
	result.Reusable = result.PercentageValid && raw.State != "" && !strings.Contains(raw.State, "FAILED") && !strings.Contains(raw.State, "ERROR") && !result.Complete
	if result.Failed {
		result.FailureMessage = "ZTE diagnostic collection failed: " + raw.State
	}
	if result.Complete && raw.TarPath != "" {
		kind := raw.Type
		if kind == "" {
			kind = wire.DiagnosticAllLogs
		}
		result.DownloadBody = provider.Object{wire.FieldType: kind, wire.FieldContent: raw.TarPath}
	}
	return result, nil
}

// ValidateArtifact verifies the completed progress resource's required download field.
func (LogCollectStrategy) ValidateArtifact(progress provider.DiagnosticProgress) error {
	if progress.Complete && progress.Artifact == "" {
		return errors.New("ZTE collection completed without TarPath")
	}
	return nil
}
