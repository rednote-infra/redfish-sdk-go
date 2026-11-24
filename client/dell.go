package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/dell"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// GetDellRemoteServicesAPIStatus follows the manager's Dell LC service link.
func (c *Client) GetDellRemoteServicesAPIStatus(ctx context.Context, managerID ...string) (*models.DellRemoteServicesAPIStatus, error) {
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	uri, err := dell.LCServiceURI(manager)
	if err != nil {
		return nil, err
	}
	var service dell.LCService
	if err := c.Get(ctx, uri, &service); err != nil {
		return nil, err
	}
	target, err := service.StatusActionURI()
	if err != nil {
		return nil, err
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
	if err := dell.ValidateExportSystemConfiguration(request); err != nil {
		return nil, err
	}
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	target, err := dell.ExportActionURI(manager)
	if err != nil {
		return nil, err
	}
	return c.postOperation(ctx, target, request)
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
			if dell.IsSystemConfiguration(response.Body) {
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
