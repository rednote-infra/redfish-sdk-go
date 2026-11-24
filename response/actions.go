package response

import (
	"encoding/json"
	"errors"
	"strings"
)

// Action retains an advertised action target, ActionInfo URI, and parameter metadata.
type Action struct {
	Target     string                     `json:"target"`
	ActionInfo string                     `json:"@Redfish.ActionInfo"`
	Parameters map[string]json.RawMessage `json:"-"`
}

// FindAction resolves one advertised action, rejecting missing targets and ambiguous matches.
func FindAction(actions map[string]json.RawMessage, name string) (*Action, error) {
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
