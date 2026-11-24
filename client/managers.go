package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func (c *Client) ResetManager(ctx context.Context, resetType string, managerID ...string) (*models.RedfishResponse, error) {
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	if resetType == "" {
		resetType = ResetForceRestart
	}
	if manager.Actions == nil || manager.Actions.ManagerReset == nil {
		return nil, errors.New("manager does not advertise reset action")
	}
	action := manager.Actions.ManagerReset
	if err = c.validateActionParameter(ctx, models.Value(action.ActionInfo), wire.FieldResetType, resetType, action.ResetTypeAllowableValues); err != nil {
		return nil, err
	}
	return c.postOperation(ctx, models.Value(action.Target), Object{wire.FieldResetType: resetType})
}

type SerialInterfaceUpdate struct {
	InterfaceEnabled *bool   `json:"InterfaceEnabled,omitempty"`
	BitRate          *string `json:"BitRate,omitempty"`
	DataBits         *string `json:"DataBits,omitempty"`
	StopBits         *string `json:"StopBits,omitempty"`
	Parity           *string `json:"Parity,omitempty"`
	FlowControl      *string `json:"FlowControl,omitempty"`
}

func (c *Client) UpdateSerialInterface(ctx context.Context, id string, patch SerialInterfaceUpdate, managerID ...string) (*sdkresponse.Response, error) {
	resource, err := c.GetSerialInterface(ctx, id, managerID...)
	if err != nil {
		return nil, err
	}
	if patch == (SerialInterfaceUpdate{}) {
		return nil, errors.New("serial interface update must not be empty")
	}
	return c.patchWithETag(ctx, models.Value(resource.ODataID), patch)
}

func findOEMLink(container map[string]json.RawMessage, key string) (string, error) {
	var found string
	var walk func(map[string]json.RawMessage) error
	walk = func(fields map[string]json.RawMessage) error {
		for name, raw := range fields {
			if strings.EqualFold(name, key) {
				var direct string
				if json.Unmarshal(raw, &direct) == nil && direct != "" {
					if found != "" && found != direct {
						return errors.New("OEM service link is ambiguous")
					}
					found = direct
					continue
				}
				var link models.Link
				if json.Unmarshal(raw, &link) == nil && models.Value(link.ODataID) != "" {
					if found != "" && found != models.Value(link.ODataID) {
						return errors.New("OEM service link is ambiguous")
					}
					found = models.Value(link.ODataID)
				}
			}
		}
		if found != "" {
			return nil
		}
		for _, raw := range fields {
			var nested map[string]json.RawMessage
			if json.Unmarshal(raw, &nested) == nil && nested != nil {
				if err := walk(nested); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(container); err != nil {
		return "", err
	}
	if found == "" {
		return "", sdkerrors.ErrNotFound
	}
	return found, nil
}

func managerOEMService[T any](ctx context.Context, c *Client, key string, managerID []string) (*T, error) {
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	uri, err := findOEMLink(manager.Extra, key)
	if errors.Is(err, sdkerrors.ErrNotFound) && manager.OEM != nil {
		uri, err = findOEMLink(*manager.OEM, key)
	}
	if errors.Is(err, sdkerrors.ErrNotFound) && key == "SyslogService" {
		uri, err = findOEMLink(manager.Extra, "Syslog")
		if errors.Is(err, sdkerrors.ErrNotFound) && manager.OEM != nil {
			uri, err = findOEMLink(*manager.OEM, "Syslog")
		}
	}
	if errors.Is(err, sdkerrors.ErrNotFound) && key == "VncService" && manager.OEM != nil {
		uri, err = findOEMLink(*manager.OEM, "RfbService")
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return getResource[T](ctx, c, uri)
}

func (c *Client) GetKVMService(ctx context.Context, managerID ...string) (*models.KvmService, error) {
	return managerOEMService[models.KvmService](ctx, c, "KvmService", managerID)
}

func (c *Client) GetNTPService(ctx context.Context, managerID ...string) (*models.NtpService, error) {
	return managerOEMService[models.NtpService](ctx, c, "NtpService", managerID)
}

func (c *Client) GetSyslogService(ctx context.Context, managerID ...string) (*models.SyslogService, error) {
	return managerOEMService[models.SyslogService](ctx, c, "SyslogService", managerID)
}

func (c *Client) GetSNMPService(ctx context.Context, managerID ...string) (*models.SnmpService, error) {
	return managerOEMService[models.SnmpService](ctx, c, "SnmpService", managerID)
}

func (c *Client) GetLLDPService(ctx context.Context, managerID ...string) (*models.LldpService, error) {
	return managerOEMService[models.LldpService](ctx, c, "LldpService", managerID)
}

func (c *Client) GetDNSService(ctx context.Context, managerID ...string) (*models.DnsService, error) {
	return managerOEMService[models.DnsService](ctx, c, "DnsService", managerID)
}

func (c *Client) GetVNCService(ctx context.Context, managerID ...string) (*models.VncService, error) {
	return managerOEMService[models.VncService](ctx, c, "VncService", managerID)
}

func (c *Client) GetSecurityService(ctx context.Context, managerID ...string) (*models.SecurityService, error) {
	resource, err := managerOEMService[models.SecurityService](ctx, c, "SecurityService", managerID)
	if !errors.Is(err, sdkerrors.ErrNotFound) || c.cfg.vendor != VendorZTE {
		return resource, err
	}
	manager, err := c.GetManager(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	if err = requireIdentity(models.Value(manager.ODataID)); err != nil {
		return nil, err
	}
	return getResource[models.SecurityService](ctx, c, strings.TrimRight(models.Value(manager.ODataID), "/")+"/SecurityService")
}

func (c *Client) GetHTTPSCert(ctx context.Context, managerID ...string) (*models.HttpsCert, error) {
	service, err := c.GetSecurityService(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	if service.Links != nil && service.Links.HTTPSCert != nil {
		return linkedResource[models.HttpsCert](ctx, c, service.Links.HTTPSCert, "HttpsCert")
	}
	uri, err := findOEMLink(service.Extra, "HttpsCert")
	if err != nil {
		return nil, err
	}
	return getResource[models.HttpsCert](ctx, c, uri)
}

func (c *Client) GetHTTPSCertificates(ctx context.Context, managerID ...string) ([]models.Link, error) {
	protocol, err := c.GetNetworkProtocol(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	if protocol.HTTPS == nil {
		return nil, sdkerrors.ErrNotFound
	}
	return linkedCollection[models.Link](ctx, c, protocol.HTTPS.Certificates, "Certificates")
}

func (c *Client) GetFirewallRules(ctx context.Context, managerID ...string) (*models.FirewallRules, error) {
	resource, err := managerOEMService[models.FirewallRules](ctx, c, "FirewallRules", managerID)
	if !errors.Is(err, sdkerrors.ErrNotFound) {
		return resource, err
	}
	service, err := c.GetSecurityService(ctx, managerID...)
	if err != nil {
		return nil, err
	}
	uri, err := findOEMLink(service.Extra, "FirewallRules")
	if err != nil {
		return nil, err
	}
	return getResource[models.FirewallRules](ctx, c, uri)
}

func (c *Client) GetSOLSource(ctx context.Context, managerID ...string) (*models.SolSourceControlInfo, error) {
	return managerOEMService[models.SolSourceControlInfo](ctx, c, "SolSourceControlInfo", managerID)
}

func (c *Client) InsertVirtualMedia(ctx context.Context, mediaID string, request models.VirtualMediaInsertMediaRequest, managerID ...string) (*models.RedfishResponse, error) {
	resource, err := c.GetVirtualMediaResource(ctx, mediaID, managerID...)
	if err != nil {
		return nil, err
	}
	if resource.Actions == nil || resource.Actions.InsertMedia == nil {
		if c.cfg.vendor != VendorLenovo {
			return nil, errors.New("virtual media does not advertise InsertMedia")
		}
		if request.TransferMethod != nil || request.TransferProtocolType != nil || request.HTTPHeaders != nil || len(request.Extra) > 0 {
			return nil, errors.New("PATCH virtual media does not support action-only parameters")
		}
		if request.Inserted == nil {
			request.Inserted = models.Ptr(true)
		}
		response, err := c.patchWithETag(ctx, models.Value(resource.ODataID), request)
		if err != nil {
			return nil, err
		}
		return response.Operation()
	}
	action := resource.Actions.InsertMedia
	if request.TransferMethod != nil {
		if err = c.validateActionParameter(ctx, models.Value(action.ActionInfo), "TransferMethod", *request.TransferMethod, action.TransferMethodAllowableValues); err != nil {
			return nil, err
		}
	}
	if request.TransferProtocolType != nil {
		if err = c.validateActionParameter(ctx, models.Value(action.ActionInfo), "TransferProtocolType", *request.TransferProtocolType, action.TransferProtocolTypeAllowableValues); err != nil {
			return nil, err
		}
	}
	return c.postOperation(ctx, models.Value(action.Target), request)
}

func (c *Client) EjectVirtualMedia(ctx context.Context, mediaID string, managerID ...string) (*models.RedfishResponse, error) {
	resource, err := c.GetVirtualMediaResource(ctx, mediaID, managerID...)
	if err != nil {
		return nil, err
	}
	if resource.Actions == nil || resource.Actions.EjectMedia == nil {
		if c.cfg.vendor != VendorLenovo {
			return nil, errors.New("virtual media does not advertise EjectMedia")
		}
		response, err := c.patchWithETag(ctx, models.Value(resource.ODataID), Object{wire.FieldInserted: false, wire.FieldImage: ""})
		if err != nil {
			return nil, err
		}
		return response.Operation()
	}
	return c.postOperation(ctx, models.Value(resource.Actions.EjectMedia.Target), Object{})
}

func (c *Client) GetSystemLogEntries(ctx context.Context, logID string, systemID ...string) ([]models.LogEntry, error) {
	service, err := c.GetSystemLogService(ctx, logID, systemID...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.LogEntry](ctx, c, service.Entries, "Entries")
}

func (c *Client) GetManagerLogEntries(ctx context.Context, logID string, managerID ...string) ([]models.LogEntry, error) {
	service, err := c.GetManagerLogService(ctx, logID, managerID...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.LogEntry](ctx, c, service.Entries, "Entries")
}

func (c *Client) GetMetricReportLogEntries(ctx context.Context) ([]models.LogEntry, error) {
	service, err := c.GetMetricReportLog(ctx)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.LogEntry](ctx, c, service.Entries, "Entries")
}

func (c *Client) clearLog(ctx context.Context, service *models.Log) (*models.RedfishResponse, error) {
	if service.Actions == nil || service.Actions.ClearLog == nil {
		return nil, errors.New("log service does not advertise ClearLog")
	}
	return c.postOperation(ctx, models.Value(service.Actions.ClearLog.Target), Object{})
}

func (c *Client) ClearSystemLog(ctx context.Context, logID string, systemID ...string) (*models.RedfishResponse, error) {
	service, err := c.GetSystemLogService(ctx, logID, systemID...)
	if err != nil {
		return nil, err
	}
	return c.clearLog(ctx, service)
}

func (c *Client) ClearManagerLog(ctx context.Context, logID string, managerID ...string) (*models.RedfishResponse, error) {
	service, err := c.GetManagerLogService(ctx, logID, managerID...)
	if err != nil {
		return nil, err
	}
	return c.clearLog(ctx, service)
}
