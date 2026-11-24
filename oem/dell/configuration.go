package dell

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

// ValidateExportSystemConfiguration checks the documented export request contract.
func ValidateExportSystemConfiguration(request models.DellExportSystemConfigurationRequest) error {
	if strings.TrimSpace(request.ShareParameters.Target) == "" {
		return errors.New("ShareParameters.Target is required")
	}
	switch request.ExportFormat {
	case "", models.DellExportXML, models.DellExportJSON:
	default:
		return errors.New("unsupported Dell export format")
	}
	switch request.ExportUse {
	case "", models.DellExportDefault, models.DellExportClone, models.DellExportReplace:
	default:
		return errors.New("unsupported Dell export use")
	}
	switch request.IncludeInExport {
	case "", models.DellIncludeDefault, models.DellIncludeReadOnly, models.DellIncludePasswordHashes, models.DellIncludeCustomTelemetry:
	default:
		return errors.New("unsupported Dell export inclusion")
	}
	share := request.ShareParameters
	switch share.ShareType {
	case "", models.DellShareLocal:
	case models.DellShareNFS, models.DellShareCIFS, models.DellShareHTTP, models.DellShareHTTPS:
		if share.IPAddress == "" || share.ShareName == "" {
			return errors.New("network export requires IPAddress and ShareName")
		}
		if share.ShareType == models.DellShareCIFS && share.UserName == "" {
			return errors.New("CIFS export requires UserName")
		}
	default:
		return errors.New("unsupported Dell export share type")
	}
	return nil
}

// IsSystemConfiguration recognizes the XML or JSON configuration returned by an export monitor.
func IsSystemConfiguration(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("<")) {
		decoder := xml.NewDecoder(bytes.NewReader(trimmed))
		found := false
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return found
			}
			if err != nil {
				return false
			}
			if start, ok := token.(xml.StartElement); ok && !found {
				if start.Name.Local != "SystemConfiguration" {
					return false
				}
				found = true
			}
		}
	}
	var configuration struct {
		SystemConfiguration json.RawMessage `json:"SystemConfiguration"`
	}
	return json.Unmarshal(trimmed, &configuration) == nil && len(configuration.SystemConfiguration) > 0 && !bytes.Equal(configuration.SystemConfiguration, []byte("null"))
}
