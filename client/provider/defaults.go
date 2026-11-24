package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// DefaultExtractor follows advertised links and standard Redfish readings.
type DefaultExtractor struct{}

func (DefaultExtractor) InletHistoryURI(thermal *models.Thermal) (string, error) {
	if thermal.InletHistoryTemperature == nil || strings.TrimSpace(models.Value(thermal.InletHistoryTemperature.ODataID)) == "" {
		return "", fmt.Errorf("resource does not advertise InletHistoryTemperature: %w", sdkerrors.ErrNotFound)
	}
	return models.Value(thermal.InletHistoryTemperature.ODataID), nil
}
func (DefaultExtractor) FanSpeedRatio(fan *models.Fan) *float64 {
	if reading := ReportedFanSpeed(fan); reading != nil {
		return reading
	}
	if fan.Reading != nil && fan.MaxReadingRange != nil && *fan.MaxReadingRange > 0 {
		return models.Ptr(math.Round(float64(*fan.Reading)/float64(*fan.MaxReadingRange)*1000) / 10)
	}
	return nil
}
func (DefaultExtractor) DriveTemperature(*models.Drive) *float64 { return nil }

// ReportedFanSpeed preserves a standard SpeedPercent reading when present.
func ReportedFanSpeed(fan *models.Fan) *float64 {
	if fan.SpeedPercent != nil && fan.SpeedPercent.Reading != nil {
		return models.Ptr(*fan.SpeedPercent.Reading)
	}
	return nil
}

// OEMNumber reads a genuinely dynamic vendor property without changing the source resource.
func OEMNumber(extensions *models.Oem, namespace, key string) *float64 {
	if extensions == nil {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal((*extensions)[namespace], &fields) != nil {
		return nil
	}
	var number float64
	if json.Unmarshal(fields[key], &number) == nil {
		return &number
	}
	var text string
	if json.Unmarshal(fields[key], &text) == nil && json.Unmarshal([]byte(text), &number) == nil {
		return &number
	}
	return nil
}

// DefaultUpdateStrategy builds the standard SimpleUpdate payload.
type DefaultUpdateStrategy struct{}

func (DefaultUpdateStrategy) BuildBody(request UpdateRequest) (Object, error) {
	if request.ImageURI == "" {
		return nil, errors.New("image URI must not be empty")
	}
	protocol := request.TransferProtocol
	if protocol == "" {
		protocol = wire.TransferProtocolHTTP
	}
	body := Object{wire.FieldImageURI: request.ImageURI, wire.FieldTransferProtocol: protocol}
	if len(request.Targets) > 0 {
		body[wire.FieldTargets] = request.Targets
	}
	return body, nil
}

// UpdateOptions copies only supported optional OEM request fields.
func UpdateOptions(request UpdateRequest, keys ...string) Object {
	result := Object{}
	for _, key := range keys {
		if value, ok := request.Options[key]; ok {
			result[key] = value
		}
	}
	return result
}

// AddUpdateCredentials includes supplied image source credentials.
func AddUpdateCredentials(body Object, request UpdateRequest) {
	if request.Username != "" {
		body[wire.FieldUsername] = request.Username
	}
	if request.Password != "" {
		body[wire.FieldPassword] = request.Password
	}
}

// AddPublicUpdateOptions adds a nonempty vendor Public namespace.
func AddPublicUpdateOptions(body, options Object) {
	if len(options) > 0 {
		body[wire.FieldOEM] = Object{wire.FieldPublic: options}
	}
}

// DefaultLogServiceSelector selects an advertised standard collection action.
type DefaultLogServiceSelector struct{}

func (DefaultLogServiceSelector) SelectService(services []models.Log) (*models.Log, error) {
	for _, service := range services {
		if service.Actions != nil {
			if _, err := sdkresponse.FindAction(service.Actions.Extra, wire.ActionCollectDiagnosticData); err == nil {
				return &service, nil
			}
		}
	}
	return nil, errors.New("no log service advertises CollectDiagnosticData")
}

// DefaultLogCollectStrategy uses standard manager diagnostic collection.
type DefaultLogCollectStrategy struct{ DefaultLogServiceSelector }

func (DefaultLogCollectStrategy) BuildBody(options DiagnosticOptions) (Object, error) {
	kind := options.DiagnosticDataType
	if kind == "" {
		kind = "Manager"
	}
	return ApplyDiagnosticOptions(Object{wire.FieldDiagnosticDataType: kind}, options), nil
}

// ApplyDiagnosticOptions applies caller-supplied OEM fields after strategy defaults.
func ApplyDiagnosticOptions(body Object, options DiagnosticOptions) Object {
	if options.OEMDiagnosticDataType != "" {
		body[wire.FieldOEMDiagnosticDataType] = options.OEMDiagnosticDataType
	}
	for key, value := range options.OEMParameters {
		body[key] = value
	}
	return body
}
