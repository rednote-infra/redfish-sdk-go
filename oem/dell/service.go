package dell

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

const (
	_exportAction = "#OemManager.ExportSystemConfiguration"
)

// LCServiceURI resolves the Lifecycle Controller service advertised by a manager.
func LCServiceURI(manager *models.Manager) (string, error) {
	var links struct {
		OEM struct {
			Dell struct {
				Service *models.Link `json:"DellLCService"`
			} `json:"Dell"`
		} `json:"Oem"`
	}
	if raw := manager.Extra["Links"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &links); err != nil {
			return "", err
		}
	}
	uri := ""
	if links.OEM.Dell.Service != nil {
		uri = models.Value(links.OEM.Dell.Service.ODataID)
	}
	if manager.OEM != nil {
		if raw, exists := (*manager.OEM)[wire.OEMNamespaceDell]; exists {
			var extension struct {
				Service *models.Link `json:"DellLCService"`
			}
			if err := json.Unmarshal(raw, &extension); err != nil {
				return "", err
			}
			if extension.Service != nil {
				alternative := models.Value(extension.Service.ODataID)
				if uri != "" && alternative != "" && uri != alternative {
					return "", errors.New("DellLCService link is ambiguous")
				}
				if alternative != "" {
					uri = alternative
				}
			}
		}
	}
	if strings.TrimSpace(uri) == "" {
		return "", fmt.Errorf("manager does not advertise DellLCService: %w", sdkerrors.ErrNotFound)
	}
	return uri, nil
}

// LCService contains known Lifecycle Controller action contracts.
type LCService struct {
	Actions struct {
		GetRemoteServicesAPIStatus models.ActionTarget `json:"#DellLCService.GetRemoteServicesAPIStatus"`
	} `json:"Actions"`
}

func (s LCService) StatusActionURI() (string, error) {
	uri := models.Value(s.Actions.GetRemoteServicesAPIStatus.Target)
	if strings.TrimSpace(uri) == "" {
		return "", errors.New("Dell LC service does not advertise status action")
	}
	return uri, nil
}
func ExportActionURI(manager *models.Manager) (string, error) {
	if manager.Actions == nil {
		return "", errors.New("manager does not advertise OEM actions")
	}
	var action models.ActionTarget
	if raw := manager.Actions.OEM[_exportAction]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &action); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(models.Value(action.Target)) == "" {
		return "", errors.New("manager does not advertise Dell export action")
	}
	return models.Value(action.Target), nil
}
