package models

import "encoding/json"

const (
	_extendedInfoSuffix    = "@Message.ExtendedInfo"
	_allowableValuesSuffix = "@Redfish.AllowableValues"
)

// PropertyExtendedInfo decodes the standard messages attached to one property.
// Absence returns nil; malformed advertised annotations remain explicit errors.
func PropertyExtendedInfo(extensions map[string]json.RawMessage, property string) ([]ExtendedInfo, error) {
	return propertyAnnotation[ExtendedInfo](extensions, property, _extendedInfoSuffix)
}

// PropertyAllowableValues decodes a property's typed advertised value list.
func PropertyAllowableValues[T any](extensions map[string]json.RawMessage, property string) ([]T, error) {
	return propertyAnnotation[T](extensions, property, _allowableValuesSuffix)
}
func propertyAnnotation[T any](extensions map[string]json.RawMessage, property, suffix string) ([]T, error) {
	raw, ok := extensions[property+suffix]
	if !ok {
		return nil, nil
	}
	var values []T
	err := json.Unmarshal(raw, &values)
	return values, err
}
