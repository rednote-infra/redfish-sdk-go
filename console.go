package redfish

import (
	"context"
	"errors"
	"slices"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

const (
	ConsoleVNC      = "VNC"
	ConsoleHTML5    = "HTML5"
	ConsoleRDP      = "RDP"
	ConsoleKVMIP    = "KVMIP"
	AccessReadWrite = "ReadWrite"
	AccessReadOnly  = "ReadOnly"
)

type ConsoleOptions struct {
	SystemID              string
	ConsoleType           *string
	AccessMode            *string
	Password              *string
	ValidityPeriodSeconds *int64
}

func (c *Client) consoleAction(ctx context.Context, options ConsoleOptions, name string) (*models.GraphicalConsoleAction, error) {
	system, err := c.GetSystem(ctx, options.SystemID)
	if err != nil {
		return nil, err
	}
	if system.GraphicalConsole == nil || system.Actions == nil {
		return nil, errors.New("system does not advertise graphical console")
	}
	var action *models.GraphicalConsoleAction
	switch name {
	case ActionGraphicalConsoleGetOneTimeURL:
		action = system.Actions.GraphicalConsoleGetOneTimeURL
	case ActionGraphicalConsoleSetOneTimePassword:
		action = system.Actions.GraphicalConsoleSetOneTimePassword
	case ActionGraphicalConsoleSetPassword:
		action = system.Actions.GraphicalConsoleSetPassword
	}
	if action == nil || Value(action.Target) == "" {
		return nil, errors.New("system does not advertise " + name)
	}
	if options.ConsoleType != nil {
		kind := *options.ConsoleType
		if err = c.validateActionParameter(ctx, Value(action.ActionInfo), FieldConsoleType, kind, action.ConsoleTypeAllowableValues); err != nil {
			return nil, err
		}
		var enabled *bool
		var modes []string
		advertised := false
		console := system.GraphicalConsole
		switch kind {
		case ConsoleVNC:
			if console.VNC != nil {
				advertised = true
				enabled = console.VNC.ServiceEnabled
				modes = console.VNC.SupportedAccessModes
				if name == ActionGraphicalConsoleSetOneTimePassword && console.VNC.SupportsOneTimePassword != nil && !*console.VNC.SupportsOneTimePassword {
					return nil, errors.New("console does not support one-time passwords")
				}
				if name == ActionGraphicalConsoleSetPassword && console.VNC.SupportsPersistentPassword != nil && !*console.VNC.SupportsPersistentPassword {
					return nil, errors.New("console does not support persistent passwords")
				}
			}
		case ConsoleHTML5:
			if console.HTML5 != nil {
				advertised = true
				enabled = console.HTML5.ServiceEnabled
				modes = console.HTML5.SupportedAccessModes
				if name == ActionGraphicalConsoleGetOneTimeURL && console.HTML5.SupportsOneTimeURL != nil && !*console.HTML5.SupportsOneTimeURL {
					return nil, errors.New("console does not support one-time URLs")
				}
			}
		case ConsoleRDP:
			if console.RDP != nil {
				advertised = true
				enabled = console.RDP.ServiceEnabled
				modes = console.RDP.SupportedAccessModes
			}
		case ConsoleKVMIP:
			if console.KVMIP != nil {
				advertised = true
				enabled = console.KVMIP.ServiceEnabled
				modes = console.KVMIP.SupportedAccessModes
			}
		}
		if !advertised {
			return nil, errors.New("console type is not advertised")
		}
		if enabled != nil && !*enabled {
			return nil, errors.New("console is disabled")
		}
		if modes == nil {
			modes = []string{AccessReadWrite}
		}
		if options.AccessMode != nil && !slices.Contains(modes, *options.AccessMode) {
			return nil, errors.New("console access mode is not supported")
		}
	}
	if options.AccessMode != nil {
		if err = c.validateActionParameter(ctx, Value(action.ActionInfo), FieldAccessMode, *options.AccessMode, action.AccessModeAllowableValues); err != nil {
			return nil, err
		}
	}
	if period := options.ValidityPeriodSeconds; period != nil {
		if *period < 1 {
			return nil, errors.New("validity period must be positive")
		}
		if action.ValidityPeriodSecondsAllowableValues != nil && !slices.Contains(action.ValidityPeriodSecondsAllowableValues, *period) {
			return nil, errors.New("validity period is not advertised")
		}
	}
	return action, nil
}
func consoleBody(options ConsoleOptions) Object {
	body := Object{}
	if options.ConsoleType != nil {
		body[FieldConsoleType] = *options.ConsoleType
	}
	if options.AccessMode != nil {
		body[FieldAccessMode] = *options.AccessMode
	}
	if options.Password != nil {
		body[FieldPassword] = *options.Password
	}
	if options.ValidityPeriodSeconds != nil {
		body[FieldValidityPeriodSeconds] = *options.ValidityPeriodSeconds
	}
	return body
}
func (c *Client) GraphicalConsoleGetOneTimeURL(ctx context.Context, options ConsoleOptions) (*models.GraphicalConsoleGetOneTimeURLResponse, error) {
	action, err := c.consoleAction(ctx, options, ActionGraphicalConsoleGetOneTimeURL)
	if err != nil {
		return nil, err
	}
	response, err := c.Post(ctx, Value(action.Target), consoleBody(options))
	if err != nil {
		return nil, err
	}
	var result models.GraphicalConsoleGetOneTimeURLResponse
	if err = response.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) GraphicalConsoleSetOneTimePassword(ctx context.Context, options ConsoleOptions) (*models.GraphicalConsoleSetOneTimePasswordResponse, error) {
	action, err := c.consoleAction(ctx, options, ActionGraphicalConsoleSetOneTimePassword)
	if err != nil {
		return nil, err
	}
	response, err := c.Post(ctx, Value(action.Target), consoleBody(options))
	if err != nil {
		return nil, err
	}
	var result models.GraphicalConsoleSetOneTimePasswordResponse
	if err = response.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) GraphicalConsoleSetPassword(ctx context.Context, consoleType, password string, systemID ...string) (*models.RedfishResponse, error) {
	id, err := optionalID(systemID)
	if err != nil {
		return nil, err
	}
	options := ConsoleOptions{SystemID: id, ConsoleType: &consoleType, Password: &password}
	action, err := c.consoleAction(ctx, options, ActionGraphicalConsoleSetPassword)
	if err != nil {
		return nil, err
	}
	return c.postOperation(ctx, Value(action.Target), consoleBody(options))
}
