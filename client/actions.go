package client

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

type Action = sdkresponse.Action

func findAction(actions map[string]json.RawMessage, name string) (*Action, error) {
	return sdkresponse.FindAction(actions, name)
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
