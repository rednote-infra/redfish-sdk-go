package dell

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// Service implements Dell Lifecycle Controller operations using the supplied resource transport.
type Service struct{ client provider.ResourceClient }

// NewService binds Dell operations to an authenticated resource transport.
func NewService(client provider.ResourceClient) *Service { return &Service{client: client} }

// GetRemoteServicesAPIStatus follows the manager's Dell LC service link.
func (s *Service) GetRemoteServicesAPIStatus(ctx context.Context, managerID ...string) (*models.DellRemoteServicesAPIStatus, error) {
	manager, err := s.client.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	uri, err := LCServiceURI(manager)
	if err != nil {
		return nil, err
	}
	var service LCService
	if err := s.client.Get(ctx, uri, &service); err != nil {
		return nil, err
	}
	target, err := service.StatusActionURI()
	if err != nil {
		return nil, err
	}
	response, err := s.client.Post(ctx, target, struct{}{})
	if err != nil {
		return nil, err
	}
	var result models.DellRemoteServicesAPIStatus
	if err := response.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ExportSystemConfiguration starts the advertised OEM action without assuming an iDRAC path.
func (s *Service) ExportSystemConfiguration(ctx context.Context, request models.DellExportSystemConfigurationRequest, managerID ...string) (*models.RedfishResponse, error) {
	if err := ValidateExportSystemConfiguration(request); err != nil {
		return nil, err
	}
	manager, err := s.client.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	target, err := ExportActionURI(manager)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Post(ctx, target, request)
	if err != nil {
		return nil, err
	}
	return response.Operation()
}

// WaitForSystemConfiguration returns the XML/JSON content served by a LOCAL export monitor.
// HTTP 202 means pending; task failure never masquerades as exported configuration.
func (s *Service) WaitForSystemConfiguration(ctx context.Context, operation *models.RedfishResponse, options provider.PollSettings) ([]byte, error) {
	if operation == nil || models.Value(operation.TaskMonitor) == "" {
		return nil, errors.New("Dell export requires a task monitor")
	}
	ctx, cancel, interval, err := provider.NewPollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if initial := models.Value(operation.RetryAfter); initial != "" {
		if err := provider.WaitContext(ctx, provider.RetryDelay(http.Header{"Retry-After": []string{initial}}, 0, 0)); err != nil {
			return nil, err
		}
	}
	for {
		response, err := s.client.GetResponse(ctx, models.Value(operation.TaskMonitor))
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusAccepted {
			if IsSystemConfiguration(response.Body) {
				return bytes.Clone(response.Body), nil
			}
			var task models.Task
			if err := response.Decode(&task); err != nil {
				return nil, errors.New("Dell export response does not contain a configuration")
			}
			if err := s.exportFailure(&task, response); err != nil {
				return nil, err
			}
			return nil, errors.New("Dell export completed without configuration content")
		}
		var task models.Task
		if len(bytes.TrimSpace(response.Body)) > 0 {
			if err := response.Decode(&task); err != nil {
				return nil, err
			}
			if err := s.exportFailure(&task, response); err != nil {
				return nil, err
			}
		}
		if err := provider.WaitContext(ctx, provider.RetryDelay(response.Header, interval, 0)); err != nil {
			return nil, err
		}
	}
}

func (s *Service) exportFailure(task *models.Task, response *sdkresponse.Response) error {
	failure := sdkerrors.TaskFailure(task)
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
	return &sdkerrors.AsyncOperationError{Task: task, Response: response, Message: s.client.RedactKnownSecrets(message)}
}
