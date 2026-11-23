package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

type ApplyTime string

const (
	ApplyImmediate                  ApplyTime = "Immediate"
	ApplyOnReset                    ApplyTime = "OnReset"
	ApplyAtMaintenanceWindowStart   ApplyTime = "AtMaintenanceWindowStart"
	ApplyInMaintenanceWindowOnReset ApplyTime = "InMaintenanceWindowOnReset"
	BootModeUEFI                              = "UEFI"
	BootModeLegacy                            = "Legacy"
	BootOverrideOnce                          = "Once"
	BootOverrideContinuous                    = "Continuous"
	BootOverrideDisabled                      = "Disabled"
	ResetOn                                   = wire.PowerStateOn
	ResetForceOff                             = "ForceOff"
	ResetGracefulShutdown                     = "GracefulShutdown"
	ResetGracefulRestart                      = "GracefulRestart"
	ResetForceRestart                         = "ForceRestart"
	ResetPowerCycle                           = "PowerCycle"
)

type BIOSOptions struct {
	SystemID          string
	ApplyTime         ApplyTime
	MaintenanceWindow *models.MaintenanceWindow
}

func (c *Client) GetBIOS(ctx context.Context, systemID ...string) (*models.Bios, error) {
	return c.GetBios(ctx, systemID...)
}

func (c *Client) GetBIOSSettings(ctx context.Context, systemID ...string) (*models.Bios, error) {
	bios, err := c.GetBIOS(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if bios.Settings == nil {
		return nil, errors.New("BIOS does not advertise pending settings")
	}
	return linkedResource[models.Bios](ctx, c, bios.Settings.SettingsObject, "SettingsObject")
}

func (c *Client) GetBiosSettings(ctx context.Context, systemID ...string) (*models.Bios, error) {
	return c.GetBIOSSettings(ctx, systemID...)
}

func (c *Client) UpdateBIOS(ctx context.Context, attributes Object, options BIOSOptions) (*sdkresponse.Response, error) {
	if len(attributes) == 0 {
		return nil, errors.New("BIOS attributes must not be empty")
	}
	bios, err := c.GetBIOS(ctx, options.SystemID)
	if err != nil {
		return nil, err
	}
	if bios.Settings == nil {
		return nil, errors.New("BIOS does not advertise pending settings")
	}
	uri, err := requireLink(bios.Settings.SettingsObject, "SettingsObject")
	if err != nil {
		return nil, err
	}
	body := Object{wire.FieldAttributes: attributes}
	if options.ApplyTime != "" {
		allowed := bios.Settings.SupportedApplyTimes
		if len(allowed) > 0 && !slices.Contains(allowed, string(options.ApplyTime)) {
			return nil, errors.New("apply time is not advertised by BIOS")
		}
		schedule := Object{wire.FieldApplyTime: options.ApplyTime}
		switch options.ApplyTime {
		case ApplyImmediate, ApplyOnReset:
			if options.MaintenanceWindow != nil {
				return nil, errors.New("maintenance window is incompatible with apply time")
			}
		case ApplyAtMaintenanceWindowStart, ApplyInMaintenanceWindowOnReset:
			if options.MaintenanceWindow == nil {
				return nil, errors.New("maintenance apply time requires a window")
			}
		default:
			return nil, errors.New("unsupported settings apply time")
		}
		if window := options.MaintenanceWindow; window != nil {
			if window.MaintenanceWindowDurationInSeconds < 0 {
				return nil, errors.New("maintenance window duration must not be negative")
			}
			schedule["MaintenanceWindowStartTime"] = window.MaintenanceWindowStartTime
			schedule["MaintenanceWindowDurationInSeconds"] = window.MaintenanceWindowDurationInSeconds
		}
		body["@Redfish.SettingsApplyTime"] = schedule
	} else if options.MaintenanceWindow != nil {
		return nil, errors.New("maintenance window requires apply time")
	}
	return c.patchWithETag(ctx, uri, body)
}

func (c *Client) patchWithETag(ctx context.Context, uri string, body any) (*sdkresponse.Response, error) {
	tag, err := c.GetETag(ctx, uri)
	if err != nil {
		return nil, err
	}
	headers := http.Header{}
	if tag != "" {
		headers.Set(wire.HeaderIfMatch, tag)
	}
	return c.Patch(ctx, uri, body, RequestOptions{Headers: headers})
}

func (c *Client) GetBootOptions(ctx context.Context, systemID ...string) ([]models.BootOption, error) {
	system, err := c.GetSystem(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if system.Boot == nil {
		return nil, errors.New("system does not advertise Boot")
	}
	return linkedCollection[models.BootOption](ctx, c, system.Boot.BootOptions, "BootOptions")
}

func (c *Client) GetBootOption(ctx context.Context, id string, systemID ...string) (*models.BootOption, error) {
	options, err := c.GetBootOptions(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	return selectItem(options, id)
}

func (c *Client) SetBootOptionEnabled(ctx context.Context, id string, enabled bool, systemID ...string) (*sdkresponse.Response, error) {
	option, err := c.GetBootOption(ctx, id, systemID...)
	if err != nil {
		return nil, err
	}
	return c.patchWithETag(ctx, models.Value(option.ODataID), Object{"BootOptionEnabled": enabled})
}

type BootOptions struct{ SystemID, Mode, Enabled string }

func (c *Client) ChangeBootSource(ctx context.Context, target string, options BootOptions) (*sdkresponse.Response, error) {
	system, err := c.GetSystem(ctx, options.SystemID)
	if err != nil {
		return nil, err
	}
	if system.Boot != nil && system.Boot.AllowableValues != nil && !slices.Contains(system.Boot.AllowableValues, target) {
		return nil, fmt.Errorf("boot target %q is not advertised", target)
	}
	mode, enabled := options.Mode, options.Enabled
	if mode == "" {
		mode = BootModeUEFI
	}
	if enabled == "" {
		enabled = BootOverrideOnce
	}
	if mode != BootModeUEFI && mode != BootModeLegacy {
		return nil, errors.New("unsupported boot override mode")
	}
	if !slices.Contains([]string{BootOverrideOnce, BootOverrideContinuous, BootOverrideDisabled}, enabled) {
		return nil, errors.New("unsupported boot override enablement")
	}
	return c.patchWithETag(ctx, models.Value(system.ODataID), Object{wire.FieldBoot: Object{"BootSourceOverrideTarget": target, "BootSourceOverrideMode": mode, "BootSourceOverrideEnabled": enabled}})
}

type ResetOptions struct {
	SystemID            string
	SkipPowerStateCheck bool
}

func (c *Client) Reset(ctx context.Context, resetType string, options ResetOptions) (*models.RedfishResponse, error) {
	system, err := c.GetSystem(ctx, options.SystemID)
	if err != nil {
		return nil, err
	}
	if system.Actions == nil || system.Actions.ComputerSystemReset == nil {
		return nil, errors.New("system does not advertise reset action")
	}
	action := system.Actions.ComputerSystemReset
	if err = c.validateActionParameter(ctx, models.Value(action.ActionInfo), wire.FieldResetType, resetType, action.ResetTypeAllowableValues); err != nil {
		return nil, err
	}
	if !options.SkipPowerStateCheck {
		power := models.Value(system.PowerState)
		if power != "" {
			allowed := map[string][]string{ResetOn: {wire.PowerStateOff}, ResetForceOff: {wire.PowerStateOn, wire.PowerStatePoweringOn, wire.PowerStatePoweringOff}, ResetGracefulShutdown: {wire.PowerStateOn}, ResetGracefulRestart: {wire.PowerStateOn}, ResetForceRestart: {wire.PowerStateOn}, ResetPowerCycle: {wire.PowerStateOn, wire.PowerStateOff}}
			if states, ok := allowed[resetType]; ok && !slices.Contains(states, power) {
				return nil, errors.New("reset type is incompatible with current power state")
			}
		}
	}
	return c.postOperation(ctx, models.Value(action.Target), Object{wire.FieldResetType: resetType})
}

func (c *Client) postOperation(ctx context.Context, uri string, body any) (*models.RedfishResponse, error) {
	if uri == "" {
		return nil, errors.New("advertised action is missing target")
	}
	response, err := c.Post(ctx, uri, body)
	if err != nil {
		return nil, err
	}
	return response.Operation()
}

func (c *Client) UpdateSecureBoot(ctx context.Context, enabled bool, systemID ...string) (*sdkresponse.Response, error) {
	resource, err := c.GetSecureBoot(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	if raw := resource.Extra["SecureBootEnable@Redfish.AllowableValues"]; raw != nil {
		var allowed []bool
		if err = json.Unmarshal(raw, &allowed); err != nil {
			return nil, err
		}
		if !slices.Contains(allowed, enabled) {
			return nil, errors.New("secure boot enablement is not advertised")
		}
	}
	return c.patchWithETag(ctx, models.Value(resource.ODataID), Object{"SecureBootEnable": enabled})
}

func (c *Client) ResetSecureBootKeys(ctx context.Context, kind string, systemID ...string) (*models.RedfishResponse, error) {
	resource, err := c.GetSecureBoot(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	actions, err := c.resourceActions(ctx, models.Value(resource.ODataID))
	if err != nil {
		return nil, err
	}
	action, err := findAction(actions, "ResetKeys")
	if err != nil {
		return nil, err
	}
	var allowed []string
	if raw := action.Parameters["ResetKeysType@Redfish.AllowableValues"]; raw != nil {
		if err = json.Unmarshal(raw, &allowed); err != nil {
			return nil, err
		}
	}
	if allowed == nil && action.ActionInfo == "" {
		allowed = []string{"ResetAllKeysToDefault", "DeleteAllKeys", "DeletePK"}
	}
	if err = c.validateActionParameter(ctx, action.ActionInfo, wire.FieldResetKeysType, kind, allowed); err != nil {
		return nil, err
	}
	return c.postOperation(ctx, action.Target, Object{wire.FieldResetKeysType: kind})
}
