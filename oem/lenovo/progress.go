package lenovo

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

const (
	_progressComplete = 100
	_statusSuccess    = 0
	_statusCollected  = 5
)

type progressFields struct {
	Progress *models.StringOrNumber `json:"Progress"`
	Status   *models.StringOrNumber `json:"Status"`
}
type progressResponse struct {
	OEM struct {
		Lenovo *progressFields `json:"Lenovo"`
		Public *progressFields `json:"Public"`
	} `json:"Oem"`
}

func (LogCollectStrategy) DecodeProgress(data []byte) (provider.DiagnosticProgress, error) {
	var raw progressResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return provider.DiagnosticProgress{}, err
	}
	fields := raw.OEM.Lenovo
	if fields == nil {
		fields = raw.OEM.Public
	}
	if fields == nil || fields.Progress == nil {
		return provider.DiagnosticProgress{}, errors.New("Lenovo progress response is missing Progress")
	}
	percentage, err := strconv.Atoi(string(*fields.Progress))
	if err != nil {
		return provider.DiagnosticProgress{}, err
	}
	result := provider.DiagnosticProgress{Percentage: percentage, PercentageValid: true, Complete: percentage >= _progressComplete}
	if result.Complete && fields.Status != nil {
		status, err := strconv.Atoi(string(*fields.Status))
		if err != nil {
			return provider.DiagnosticProgress{}, err
		}
		result.Failed = status != _statusSuccess && status != _statusCollected
		if result.Failed {
			result.FailureMessage = "Lenovo diagnostic collection failed"
		}
	}
	return result, nil
}
