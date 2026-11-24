package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"slices"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func decodeAttributes(raw json.RawMessage) (map[string]any, error) {
	result := map[string]any{}
	if len(raw) == 0 {
		return result, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	err := decoder.Decode(&result)
	return result, err
}
func changedBIOSAttributes(desired Object, currentRaw, pendingRaw json.RawMessage) (Object, error) {
	current, err := decodeAttributes(currentRaw)
	if err != nil {
		return nil, err
	}
	pending, err := decodeAttributes(pendingRaw)
	if err != nil {
		return nil, err
	}
	result := Object{}
	for key, value := range desired {
		actual, exists := pending[key]
		if !exists {
			actual, exists = current[key]
		}
		if !exists || !equivalentAttribute(actual, value) {
			result[key] = value
		}
	}
	return result, nil
}

// Comparison only: caller values are never coerced in the mutation payload.
func equivalentAttribute(a, b any) bool {
	normalize := func(value any) any {
		if text, ok := value.(string); ok {
			var decoded any
			decoder := json.NewDecoder(bytes.NewBufferString(text))
			decoder.UseNumber()
			if json.Valid([]byte(text)) && decoder.Decode(&decoded) == nil {
				switch decoded.(type) {
				case bool, json.Number:
					return decoded
				}
			}
			return value
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return value
		}
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		if decoder.Decode(&decoded) != nil {
			return value
		}
		return decoded
	}
	// Distinct textual values remain distinct (serial numbers and enum names).
	if _, ok := a.(string); ok {
		if _, both := b.(string); both {
			return a == b
		}
	}
	a, b = normalize(a), normalize(b)
	x, xok := a.(json.Number)
	y, yok := b.(json.Number)
	if xok && yok {
		left, lok := new(big.Rat).SetString(x.String())
		right, rok := new(big.Rat).SetString(y.String())
		return lok && rok && left.Cmp(right) == 0
	}
	return reflect.DeepEqual(a, b)
}

func (c *Client) updateBoot(ctx context.Context, system *models.System, fields Object, route BootRoute) (*sdkresponse.Response, error) {
	currentURI := models.Value(system.ODataID)
	if route == BootRouteCurrent {
		return c.patchWithETag(ctx, currentURI, Object{wire.FieldBoot: fields})
	}
	if route != BootRouteAuto && route != BootRouteSettings {
		return nil, errors.New("unsupported boot routing mode")
	}
	if system.Settings == nil {
		if route == BootRouteSettings {
			return nil, errors.New("system does not advertise pending settings")
		}
		if err := validateBootTarget(system.Boot, fields); err != nil {
			return nil, err
		}
		return c.patchWithETag(ctx, currentURI, Object{wire.FieldBoot: fields})
	}
	settingsURI, err := requireLink(system.Settings.SettingsObject, "SettingsObject")
	if err != nil {
		return nil, err
	}
	pending := &models.System{}
	err = c.Get(ctx, settingsURI, pending)
	if err != nil {
		return nil, err
	}
	currentFields, pendingFields := Object{}, Object{}
	for key, value := range fields {
		present := false
		if pending.Boot != nil {
			switch key {
			case wire.FieldBootTarget:
				present = pending.Boot.BootSourceOverrideTarget != nil || pending.Boot.NullFields[key]
			case wire.FieldBootMode:
				present = pending.Boot.BootSourceOverrideMode != nil || pending.Boot.NullFields[key]
			case wire.FieldBootEnabled:
				present = pending.Boot.BootSourceOverrideEnabled != nil || pending.Boot.NullFields[key]
			}
		}
		if present {
			pendingFields[key] = value
		} else {
			if route == BootRouteSettings {
				return nil, errors.New("pending settings do not advertise requested boot property")
			}
			currentFields[key] = value
		}
	}
	if err := validateBootTarget(system.Boot, currentFields); err != nil {
		return nil, err
	}
	if err := validateBootTarget(pending.Boot, pendingFields); err != nil {
		return nil, err
	}
	var response *sdkresponse.Response
	completed := []string{}
	for _, destination := range []struct {
		uri    string
		fields Object
	}{{currentURI, currentFields}, {settingsURI, pendingFields}} {
		if len(destination.fields) == 0 {
			continue
		}
		response, err = c.patchWithETag(ctx, destination.uri, Object{wire.FieldBoot: destination.fields})
		if err != nil {
			return nil, &BootUpdateError{CompletedURIs: completed, Err: err}
		}
		completed = append(completed, destination.uri)
	}
	return response, nil
}

func validateBootTarget(boot *models.Boot, fields Object) error {
	target, hasTarget := fields[wire.FieldBootTarget].(string)
	if hasTarget && boot != nil && boot.AllowableValues != nil && !slices.Contains(boot.AllowableValues, target) {
		return fmt.Errorf("boot target %q is not advertised", target)
	}
	return nil
}
