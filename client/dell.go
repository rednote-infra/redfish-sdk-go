package client

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const (
	_dellLCStatusAction = "#DellLCService.GetRemoteServicesAPIStatus"
	_dellExportAction   = "#OemManager.ExportSystemConfiguration"
)

// GetDellRemoteServicesAPIStatus follows the manager's Dell LC service link.
func (c *Client) GetDellRemoteServicesAPIStatus(ctx context.Context, managerID ...string) (*models.DellRemoteServicesAPIStatus, error) {
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	var links struct {
		OEM struct {
			Dell struct {
				LCService *models.Link `json:"DellLCService"`
			} `json:"Dell"`
		} `json:"Oem"`
	}
	if err := json.Unmarshal(manager.Extra["Links"], &links); err != nil {
		return nil, err
	}
	uri, err := requireLink(links.OEM.Dell.LCService, "DellLCService")
	if err != nil {
		return nil, err
	}
	var service struct {
		Actions map[string]models.ActionTarget `json:"Actions"`
	}
	if err := c.Get(ctx, uri, &service); err != nil {
		return nil, err
	}
	action := service.Actions[_dellLCStatusAction]
	target := models.Value(action.Target)
	if target == "" {
		return nil, errors.New("Dell LC service does not advertise status action")
	}
	response, err := c.Post(ctx, target, struct{}{})
	if err != nil {
		return nil, err
	}
	var result models.DellRemoteServicesAPIStatus
	if err := response.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ExportDellSystemConfiguration starts the advertised OEM action without assuming an iDRAC path.
func (c *Client) ExportDellSystemConfiguration(ctx context.Context, request models.DellExportSystemConfigurationRequest, managerID ...string) (*models.RedfishResponse, error) {
	if strings.TrimSpace(request.ShareParameters.Target) == "" {
		return nil, errors.New("ShareParameters.Target is required")
	}
	switch request.ExportFormat {
	case "", models.DellExportXML, models.DellExportJSON:
	default:
		return nil, errors.New("unsupported Dell export format")
	}
	switch request.ExportUse {
	case "", models.DellExportDefault, models.DellExportClone, models.DellExportReplace:
	default:
		return nil, errors.New("unsupported Dell export use")
	}
	switch request.IncludeInExport {
	case "", models.DellIncludeDefault, models.DellIncludeReadOnly, models.DellIncludePasswordHashes, models.DellIncludeCustomTelemetry:
	default:
		return nil, errors.New("unsupported Dell export inclusion")
	}
	share := request.ShareParameters
	switch share.ShareType {
	case "", models.DellShareLocal:
	case models.DellShareNFS, models.DellShareCIFS, models.DellShareHTTP, models.DellShareHTTPS:
		if share.IPAddress == "" || share.ShareName == "" {
			return nil, errors.New("network export requires IPAddress and ShareName")
		}
		if share.ShareType == models.DellShareCIFS && share.UserName == "" {
			return nil, errors.New("CIFS export requires UserName")
		}
	default:
		return nil, errors.New("unsupported Dell export share type")
	}
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	if manager.Actions == nil {
		return nil, errors.New("manager does not advertise OEM actions")
	}
	var action models.ActionTarget
	if raw := manager.Actions.OEM[_dellExportAction]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &action); err != nil {
			return nil, err
		}
	}
	if models.Value(action.Target) == "" {
		return nil, errors.New("manager does not advertise Dell export action")
	}
	return c.postOperation(ctx, models.Value(action.Target), request)
}

// WaitForDellSystemConfiguration returns the XML/JSON content served by a LOCAL export monitor.
// HTTP 202 means pending; task failure never masquerades as exported configuration.
func (c *Client) WaitForDellSystemConfiguration(ctx context.Context, operation *models.RedfishResponse, options PollOptions) ([]byte, error) {
	if operation == nil || models.Value(operation.TaskMonitor) == "" {
		return nil, errors.New("Dell export requires a task monitor")
	}
	ctx, cancel, interval, err := pollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if initial := models.Value(operation.RetryAfter); initial != "" {
		if err := waitContext(ctx, retryDelay(http.Header{"Retry-After": []string{initial}}, 0, 0)); err != nil {
			return nil, err
		}
	}
	for {
		response, err := c.Request(ctx, http.MethodGet, models.Value(operation.TaskMonitor), nil)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusAccepted {
			if isDellConfiguration(response.Body) {
				return bytes.Clone(response.Body), nil
			}
			var task models.Task
			if err := response.Decode(&task); err != nil {
				return nil, errors.New("Dell export response does not contain a configuration")
			}
			if err := c.dellExportFailure(&task, response); err != nil {
				return nil, err
			}
			return nil, errors.New("Dell export completed without configuration content")
		}
		var task models.Task
		if len(bytes.TrimSpace(response.Body)) > 0 {
			if err := response.Decode(&task); err != nil {
				return nil, err
			}
			if err := c.dellExportFailure(&task, response); err != nil {
				return nil, err
			}
		}
		if err := waitContext(ctx, retryDelay(response.Header, interval, 0)); err != nil {
			return nil, err
		}
	}
}
func isDellConfiguration(data []byte) bool {
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

func (c *Client) dellExportFailure(task *models.Task, response *sdkresponse.Response) error {
	failure := taskFailure(task)
	if failure == nil {
		return nil
	}
	message := failure.Error()
	for _, entry := range task.Messages {
		if text := models.Value(entry.Message); strings.TrimSpace(text) != "" {
			message = text
			break
		}
	}
	return &AsyncOperationError{Task: task, Response: response, Message: redaction.String(message, c.passwordValue(), c.tokenSnapshot())}
}
