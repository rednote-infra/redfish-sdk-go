package hpe

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// LogCollectStrategy resolves the existing iLO Active Health System download resource.
type LogCollectStrategy struct {
	provider.DefaultLogServiceSelector
}

func (LogCollectStrategy) DiagnosticLayout() provider.DiagnosticLayout {
	return provider.DiagnosticLayout{}
}
func (LogCollectStrategy) BuildBody(options provider.DiagnosticOptions) (provider.Object, error) {
	if len(options.OEMParameters) > 0 {
		return nil, errors.New("HPE AHS download parameters must use DownloadQuery")
	}
	return provider.Object{}, nil
}

// DiagnosticServiceURI follows the HPE manager OEM link without assuming a manager ID.
func (LogCollectStrategy) DiagnosticServiceURI(manager *models.Manager) (string, error) {
	if manager.OEM == nil {
		return "", fmt.Errorf("manager does not advertise ActiveHealthSystem: %w", sdkerrors.ErrNotFound)
	}
	var result string
	for _, namespace := range []string{wire.OEMNamespaceHPE, wire.OEMNamespaceHP} {
		raw, exists := (*manager.OEM)[namespace]
		if !exists {
			continue
		}
		var extension struct {
			Links struct {
				ActiveHealthSystem *models.Link `json:"ActiveHealthSystem"`
			} `json:"Links"`
		}
		if err := json.Unmarshal(raw, &extension); err != nil {
			return "", err
		}
		if extension.Links.ActiveHealthSystem == nil {
			continue
		}
		uri := strings.TrimSpace(models.Value(extension.Links.ActiveHealthSystem.ODataID))
		if result != "" && uri != "" && result != uri {
			return "", errors.New("ActiveHealthSystem link is ambiguous")
		}
		if uri != "" {
			result = uri
		}
	}
	if result == "" {
		return "", fmt.Errorf("manager does not advertise ActiveHealthSystem: %w", sdkerrors.ErrNotFound)
	}
	return result, nil
}

// DiagnosticArtifactURI decodes the documented AHS link and preserves caller query values.
func (LogCollectStrategy) DiagnosticArtifactURI(data []byte, options provider.DiagnosticOptions) (string, error) {
	var resource struct {
		Links struct {
			AHSLocation struct {
				ExternalRef string `json:"extref"`
				ODataID     string `json:"@odata.id"`
			} `json:"AHSLocation"`
		} `json:"Links"`
	}
	if err := json.Unmarshal(data, &resource); err != nil {
		return "", err
	}
	uri := resource.Links.AHSLocation.ExternalRef
	if uri == "" {
		uri = resource.Links.AHSLocation.ODataID
	}
	if strings.TrimSpace(uri) == "" {
		return "", fmt.Errorf("AHS resource does not advertise AHSLocation: %w", sdkerrors.ErrNotFound)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for key, values := range options.DownloadQuery {
		query[key] = append([]string(nil), values...)
	}
	if (query.Get("from") == "") != (query.Get("to") == "") {
		return "", errors.New("AHS from and to query parameters must be supplied together")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
