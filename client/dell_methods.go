package client

import (
	"context"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/oem/dell"
)

// GetDellRemoteServicesAPIStatus forwards to the Dell OEM service.
func (c *Client) GetDellRemoteServicesAPIStatus(ctx context.Context, managerID ...string) (*models.DellRemoteServicesAPIStatus, error) {
	return dell.NewService(c.ResourceClient()).GetRemoteServicesAPIStatus(ctx, managerID...)
}

// ExportDellSystemConfiguration forwards to the Dell OEM service.
func (c *Client) ExportDellSystemConfiguration(ctx context.Context, request models.DellExportSystemConfigurationRequest, managerID ...string) (*models.RedfishResponse, error) {
	return dell.NewService(c.ResourceClient()).ExportSystemConfiguration(ctx, request, managerID...)
}

// WaitForDellSystemConfiguration forwards to the Dell OEM export monitor.
func (c *Client) WaitForDellSystemConfiguration(ctx context.Context, operation *models.RedfishResponse, options PollOptions) ([]byte, error) {
	return dell.NewService(c.ResourceClient()).WaitForSystemConfiguration(ctx, operation, provider.PollSettings{Interval: options.Interval, Timeout: options.Timeout, MaxTransientFailures: options.MaxTransientFailures})
}
