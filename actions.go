package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Action struct {
	Target     string                     `json:"target"`
	ActionInfo string                     `json:"@Redfish.ActionInfo"`
	Parameters map[string]json.RawMessage `json:"-"`
}

func findAction(actions map[string]json.RawMessage, name string) (*Action, error) {
	var found *Action
	var walk func(map[string]json.RawMessage) error
	walk = func(container map[string]json.RawMessage) error {
		for key, raw := range container {
			if strings.HasPrefix(key, "#") && strings.HasSuffix(key, "."+name) {
				var action Action
				if err := json.Unmarshal(raw, &action); err != nil {
					return err
				}
				if strings.TrimSpace(action.Target) == "" {
					return errors.New("advertised action is missing target")
				}
				if found != nil {
					return errors.New("advertised action is ambiguous")
				}
				action.Parameters = map[string]json.RawMessage{}
				if err := json.Unmarshal(raw, &action.Parameters); err != nil {
					return err
				}
				found = &action
			} else if !strings.HasPrefix(key, "#") {
				var nested map[string]json.RawMessage
				if json.Unmarshal(raw, &nested) == nil {
					if err := walk(nested); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(actions); err != nil {
		return nil, err
	}
	if found == nil {
		return nil, errors.New("resource does not advertise " + name + " action")
	}
	return found, nil
}

// ActionInfo is fetched only when inline allowable values are unavailable.
func (c *Client) validateActionParameter(ctx context.Context, infoURI, name, value string, allowed []string) error {
	if allowed == nil && infoURI != "" {
		var info struct {
			Parameters []struct {
				Name            string   `json:"Name"`
				AllowableValues []string `json:"AllowableValues"`
			} `json:"Parameters"`
		}
		if err := c.Get(ctx, infoURI, &info); err != nil {
			return err
		}
		for _, parameter := range info.Parameters {
			if parameter.Name == name {
				allowed = parameter.AllowableValues
				break
			}
		}
	}
	if allowed != nil && !slices.Contains(allowed, value) {
		return fmt.Errorf("%s %q is not in advertised allowable values", name, value)
	}
	return nil
}
func (c *Client) resourceActions(ctx context.Context, uri string) (map[string]json.RawMessage, error) {
	var document struct {
		Actions map[string]json.RawMessage `json:"Actions"`
	}
	if err := c.Get(ctx, uri, &document); err != nil {
		return nil, err
	}
	return document.Actions, nil
}
