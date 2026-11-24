package oem

import (
	"errors"
	"fmt"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const (
	_diagnosticManager       = "Manager"
	_diagnosticOEM           = "OEM"
	_diagnosticAllLogs       = wire.DiagnosticAllLogs
	_h3cSDSPath              = wire.H3CDiagnosticPath
	_xfusionContentDirectory = "/tmp/web"
)

// NewLogCollectStrategy selects a built-in log request and service policy.
func NewLogCollectStrategy(vendor Vendor) LogCollectStrategy {
	return logCollectStrategy{vendor: vendor}
}

type DiagnosticOptions struct {
	DiagnosticDataType, OEMDiagnosticDataType, LogID, ManagerID string
	OEMParameters                                               Object
}

type logCollectStrategy struct{ vendor Vendor }

func (p logCollectStrategy) BuildBody(options DiagnosticOptions) (Object, error) {
	kind := options.DiagnosticDataType
	body := Object{}
	switch p.vendor {
	case VendorInspur, VendorIEIT, VendorLenovo, VendorSmoothcompute:
	case VendorZTE:
		if kind == "" {
			kind = _diagnosticAllLogs
		}
		body[wire.FieldType] = kind
	case VendorH3C:
		body[wire.FieldType] = wire.DiagnosticOutputURI
		body[wire.FieldContent] = _h3cSDSPath
		if kind != "" {
			body["LogType"] = kind
		}
	case VendorXFusion:
		body[wire.FieldType] = wire.DiagnosticOutputURI
		body[wire.FieldContent] = fmt.Sprintf("%s/xf_sdk_%d.tar.gz", _xfusionContentDirectory, time.Now().UnixNano())
	case VendorEnginetech:
		if kind == "" {
			kind = _diagnosticOEM
		}
		body[wire.FieldDiagnosticDataType] = kind
		body[wire.FieldOEMDiagnosticDataType] = "BlackBox"
	default:
		if kind == "" {
			kind = _diagnosticManager
		}
		body[wire.FieldDiagnosticDataType] = kind
	}
	if options.OEMDiagnosticDataType != "" {
		body[wire.FieldOEMDiagnosticDataType] = options.OEMDiagnosticDataType
	}
	for key, value := range options.OEMParameters {
		body[key] = value
	}
	return body, nil
}

func (p logCollectStrategy) SelectService(services []models.Log) (*models.Log, error) {
	for _, service := range services {
		if service.Actions != nil {
			if _, err := sdkresponse.FindAction(service.Actions.Extra, wire.ActionCollectDiagnosticData); err == nil {
				return &service, nil
			}
		}
	}
	return nil, errors.New("no log service advertises CollectDiagnosticData")
}
